package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Per-domain mail service options (alpha.128, gap G25).
//
//	GET /api/v1/domains/{id}/mail/options
//	PUT /api/v1/domains/{id}/mail/options   {"incoming_off": bool, "unknown_reply": "…"}
//
// "Send only" refuses mail from other servers for the domain (550 5.7.1);
// its mailboxes keep sending and keep their mail. "Unknown addresses" answers
// mail for an address the domain does not have with the owner's own text
// (550 5.1.1 …) instead of Postfix's; not offered while a catch-all forward
// takes such mail. The whole map is sent to the Agent after each change and
// after a mailbox or forwarding of such a domain changes (the map lists the
// domain's known addresses). Nothing is sent while no domain uses an option.
//
// Alan adı başına posta seçenekleri: "yalnız gönderim" ve "bilinmeyen adrese
// kendi iletimizle ret". Posta kutuları ve postaları hiç değişmez.

const (
	domainOptionsSendOnlyReply = "550 5.7.1 This domain does not receive mail on this server"
	errCodeMailOptionsCatchAll = "MAIL_OPTIONS_CATCH_ALL"
)

type domainMailOptions struct {
	IncomingOff  bool   `json:"incoming_off"`
	UnknownReply string `json:"unknown_reply"`
}

var domainOptionsApply = struct {
	sync.Mutex
	hash string
}{}

// domainOptionsAgent is replaced by tests.
var domainOptionsAgent = func(p *Panel, ctx context.Context, entries []transport.MailDomainOptionsEntry) (transport.SetMailDomainOptionsResponse, error) {
	var resp transport.SetMailDomainOptionsResponse
	err := p.callAgentContext(ctx, "Agent.SetMailDomainOptionsV1", &transport.SetMailDomainOptionsRequest{
		ExpectedBuildCommit: strings.TrimSpace(buildCommit), Entries: entries,
	}, &resp)
	return resp, err
}

// cleanUnknownReply keeps a refusal text to printable ASCII without the map's
// separators; an empty result means "Postfix's own reply".
func cleanUnknownReply(text string) (string, error) {
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return "", nil
	}
	if len(text) > transport.MailDomainOptionsMaxReply {
		return "", fmt.Errorf("the reply is longer than %d characters", transport.MailDomainOptionsMaxReply)
	}
	for _, r := range text {
		if r < 0x20 || r > 0x7e {
			return "", errors.New("the reply may hold only plain letters, digits and punctuation (no accented letters, so every mail server can show it)")
		}
	}
	return text, nil
}

// domainOptionsEntries builds the whole map from the database.
func domainOptionsEntries(ctx context.Context, db *sql.DB) ([]transport.MailDomainOptionsEntry, int, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT d.id, lower(d.name), o.incoming_off, COALESCE(o.unknown_reply, '')
		FROM domain_mail_options o JOIN domains d ON d.id = o.domain_id
		WHERE (o.incoming_off = 1 OR COALESCE(o.unknown_reply, '') <> '')
		  AND NOT EXISTS (SELECT 1 FROM domain_deletion_operations op WHERE op.domain_id = d.id)`)
	if err != nil {
		return nil, 0, err
	}
	type domainRow struct {
		id          int
		name, reply string
		off         bool
	}
	var domains []domainRow
	for rows.Next() {
		var d domainRow
		var off int
		if err := rows.Scan(&d.id, &d.name, &off, &d.reply); err != nil {
			rows.Close()
			return nil, 0, err
		}
		d.off = off == 1
		domains = append(domains, d)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	var entries []transport.MailDomainOptionsEntry
	for _, d := range domains {
		if d.off {
			entries = append(entries, transport.MailDomainOptionsEntry{Key: d.name, Action: domainOptionsSendOnlyReply})
			continue
		}
		known := map[string]bool{}
		addRows, err := db.QueryContext(ctx, `
			SELECT lower(address) FROM email_accounts WHERE domain_id = ?
			UNION SELECT lower(source) FROM email_forwardings WHERE domain_id = ?`, d.id, d.id)
		if err != nil {
			return nil, 0, err
		}
		for addRows.Next() {
			var address string
			if err := addRows.Scan(&address); err != nil {
				addRows.Close()
				return nil, 0, err
			}
			if strings.HasSuffix(address, "@"+d.name) && !strings.HasPrefix(address, "@") {
				known[address] = true
			}
		}
		addRows.Close()
		// Postfix's own addresses for a domain always receive.
		for _, local := range []string{"postmaster", "abuse"} {
			known[local+"@"+d.name] = true
		}
		addresses := make([]string, 0, len(known))
		for address := range known {
			addresses = append(addresses, address)
		}
		sort.Strings(addresses)
		for _, address := range addresses {
			entries = append(entries, transport.MailDomainOptionsEntry{Key: address, Action: "DUNNO"})
		}
		entries = append(entries, transport.MailDomainOptionsEntry{Key: d.name, Action: "550 5.1.1 " + d.reply})
	}
	return entries, len(domains), nil
}

// applyDomainMailOptions sends the whole map when it differs from the last
// one sent (force: an explicit save). Never-used options send nothing.
func (p *Panel) applyDomainMailOptions(ctx context.Context, force bool) (transport.SetMailDomainOptionsResponse, bool, error) {
	domainOptionsApply.Lock()
	defer domainOptionsApply.Unlock()
	entries, _, err := domainOptionsEntries(ctx, p.db.GetDB())
	if err != nil {
		return transport.SetMailDomainOptionsResponse{}, false, err
	}
	raw, _ := json.Marshal(entries)
	sum := sha256.Sum256(raw)
	hash := hex.EncodeToString(sum[:])
	if !force && (hash == domainOptionsApply.hash || (domainOptionsApply.hash == "" && len(entries) == 0 && !p.domainOptionsEverUsed(ctx))) {
		return transport.SetMailDomainOptionsResponse{}, false, nil
	}
	resp, err := domainOptionsAgent(p, ctx, entries)
	if err != nil {
		return resp, true, err
	}
	if resp.Applied {
		domainOptionsApply.hash = hash
	}
	return resp, true, nil
}

func (p *Panel) domainOptionsEverUsed(ctx context.Context) bool {
	var n int
	_ = p.db.GetDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM domain_mail_options`).Scan(&n)
	return n > 0
}

// requestDomainMailOptionsApply follows a mailbox or forwarding change of a
// domain that uses the unknown-address reply, without blocking the caller.
func (p *Panel) requestDomainMailOptionsApply(ctx context.Context) {
	if !p.domainOptionsEverUsed(ctx) {
		return
	}
	go func() {
		if _, _, err := p.applyDomainMailOptions(context.WithoutCancel(ctx), false); err != nil {
			log.Printf("[mail-options] apply: %v", err)
		}
	}()
}

func (p *Panel) handleDomainMailOptions(w http.ResponseWriter, r *http.Request, domainID int) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	ctx := r.Context()
	db := p.db.GetDB()
	var catchAll string
	_ = db.QueryRowContext(ctx, `SELECT destination FROM mail_catch_all WHERE domain_id = ?`, domainID).Scan(&catchAll)
	switch r.Method {
	case http.MethodGet:
		var options domainMailOptions
		var off int
		var reply sql.NullString
		err := db.QueryRowContext(ctx, `SELECT incoming_off, unknown_reply FROM domain_mail_options WHERE domain_id = ?`, domainID).Scan(&off, &reply)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			writeServerError(w, err)
			return
		}
		options.IncomingOff, options.UnknownReply = off == 1, reply.String
		_ = json.NewEncoder(w).Encode(map[string]any{"options": options, "catch_all": catchAll != ""})
	case http.MethodPut:
		var request domainMailOptions
		if err := decodeStrictJSON(w, r, &request); err != nil {
			writeClientError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		reply, err := cleanUnknownReply(request.UnknownReply)
		if err != nil {
			writeClientError(w, http.StatusBadRequest, err.Error())
			return
		}
		if reply != "" && catchAll != "" && !request.IncomingOff {
			writeCodedError(w, http.StatusConflict, errCodeMailOptionsCatchAll,
				"This domain forwards mail for unknown addresses to "+catchAll+" (catch-all), so no unknown address is refused. Remove the catch-all first, or leave the reply empty.", "")
			return
		}
		reportOperationPlan(ctx, "save", "apply")
		reportOperationStep(ctx, "save")
		if _, err := db.ExecContext(ctx, `
			INSERT INTO domain_mail_options (domain_id, incoming_off, unknown_reply, updated_at) VALUES (?, ?, ?, datetime('now'))
			ON CONFLICT(domain_id) DO UPDATE SET incoming_off = excluded.incoming_off, unknown_reply = excluded.unknown_reply,
			    updated_at = excluded.updated_at`, domainID, boolInt(request.IncomingOff), nullIfEmpty(reply)); err != nil {
			writeServerError(w, err)
			return
		}
		p.audit(r, "mail.domain.options", "domain", domainID)
		reportOperationStep(ctx, "apply")
		resp, _, err := p.applyDomainMailOptions(ctx, true)
		if err != nil {
			writeServerError(w, errors.New("the options are saved, but the server could not be asked; it is asked again with the next mail change"))
			return
		}
		if !resp.Applied {
			writeClientError(w, http.StatusConflict, "the options are saved, but the server did not take them: "+boundedAgentDiagnostic(resp.Error))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"saved": true, "applied": true})
	default:
		w.Header().Set("Allow", "GET, PUT")
		writeClientError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
