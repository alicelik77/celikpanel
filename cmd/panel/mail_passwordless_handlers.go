package main

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Passwordless mail, phase 1 (alpha.124; owner decision of 4 Nov 2026:
// "parolasız posta kesinlikle yasak. spam sorunu olabilir."). The next
// release closes mail sent from this server without a mailbox's password.
// Before that, the administrator sees which sites used it in the last 30
// days (the Agent reads Postfix's own log, read-only) and can make a sending
// mailbox for each. Nothing on the server changes here.
//
// Parolasız posta, 1. aşama: bir sonraki sürüm kapatır; önce yönetici son 30
// günde hangi sitelerin kullandığını görür ve her biri için bir gönderim posta
// kutusu oluşturabilir. Burada sunucuda hiçbir şey değişmez.

const passwordlessMailPath = "/api/v1/mail/passwordless"

// passwordlessSenderLocalPart is the mailbox the "Create a sending mailbox"
// button makes for a site's domain. Submission checks that the From address
// is the mailbox itself (reject_sender_login_mismatch), so the site sends as
// this address.
const passwordlessSenderLocalPart = "noreply"

// Ways a site sent without a password.
const (
	passwordlessWaySendmail = "sendmail" // PHP mail(), cron, the sendmail command
	passwordlessWaySMTP     = "smtp"     // SMTP to port 25 on this server, no password
)

type passwordlessMailSite struct {
	DomainID int      `json:"domain_id"`
	Domain   string   `json:"domain"`
	Ways     []string `json:"ways"`
	Messages int      `json:"messages"`
	Refused  int      `json:"refused"`
	Last     string   `json:"last,omitempty"`
	// SenderMailbox: noreply@domain when it exists (made by the button or by
	// hand); the screen then shows the settings instead of the button.
	SenderMailbox string `json:"sender_mailbox,omitempty"`
}

// passwordlessMailOther is a sender that is not a site of this panel: a Unix
// account or an envelope sender address the panel cannot place.
type passwordlessMailOther struct {
	Label    string `json:"label"`
	Way      string `json:"way"`
	Messages int    `json:"messages"`
	Refused  int    `json:"refused"`
	Last     string `json:"last,omitempty"`
}

type passwordlessMailView struct {
	// State: "unknown" (the log could not be read), "none" or "found".
	State        string                  `json:"state"`
	Reason       string                  `json:"reason,omitempty"`
	WindowDays   int                     `json:"window_days"`
	CoveredSince string                  `json:"covered_since,omitempty"`
	Incomplete   bool                    `json:"incomplete,omitempty"`
	Sites        []passwordlessMailSite  `json:"sites"`
	Others       []passwordlessMailOther `json:"others"`
	// Enforcement: whether the server refuses it now (phase 2, alpha.125).
	Enforcement *transport.PasswordlessMailEnforcement `json:"enforcement,omitempty"`
}

type passwordlessDomainRow struct {
	id   int
	name string
}

func (p *Panel) handlePasswordlessMail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		rejectRouteMethod(w, []string{http.MethodGet})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	var report transport.PasswordlessMailReportResponse
	if err := p.callAgentContext(r.Context(), "Agent.PasswordlessMailReportV1", &transport.Empty{}, &report); err != nil {
		writeServerError(w, err)
		return
	}
	domains, err := p.passwordlessDomains(r.Context())
	if err != nil {
		writeServerError(w, err)
		return
	}
	view := buildPasswordlessMailView(report, domains, p.passwordlessOwnSender(r.Context()))
	if err := p.passwordlessSenderMailboxes(r.Context(), view.Sites); err != nil {
		writeServerError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(view)
}

func (p *Panel) passwordlessDomains(ctx context.Context) ([]passwordlessDomainRow, error) {
	rows, err := p.db.GetDB().QueryContext(ctx, "SELECT id, name FROM domains ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []passwordlessDomainRow
	for rows.Next() {
		var row passwordlessDomainRow
		if err := rows.Scan(&row.id, &row.name); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// passwordlessOwnSender is the panel's own account-mail sender (password
// resets, invitations): it is not a site.
func (p *Panel) passwordlessOwnSender(ctx context.Context) string {
	host := strings.TrimSpace(accountLinkPanelHost())
	if domain := p.accountMailSenderDomain(ctx, host); domain != "" {
		return "no-reply@" + strings.ToLower(domain)
	}
	return ""
}

func (p *Panel) passwordlessSenderMailboxes(ctx context.Context, sites []passwordlessMailSite) error {
	for i := range sites {
		address := passwordlessSenderLocalPart + "@" + sites[i].Domain
		var found int
		err := p.db.GetDB().QueryRowContext(ctx,
			"SELECT COUNT(*) FROM email_accounts WHERE domain_id = ? AND lower(address) = ?",
			sites[i].DomainID, address).Scan(&found)
		if err != nil {
			return err
		}
		if found > 0 {
			sites[i].SenderMailbox = address
		}
	}
	return nil
}

// buildPasswordlessMailView places the Agent's senders on the panel's domains:
// a Unix account by the site user name derived from the domain
// (services.SiteUsername), an SMTP sender by its address's domain.
func buildPasswordlessMailView(report transport.PasswordlessMailReportResponse, domains []passwordlessDomainRow, ownSender string) passwordlessMailView {
	view := passwordlessMailView{
		WindowDays: transport.PasswordlessMailWindowDays,
		Sites:      []passwordlessMailSite{},
		Others:     []passwordlessMailOther{},
	}
	view.Enforcement = report.Enforcement
	if !report.Read {
		view.State = "unknown"
		view.Reason = report.Error
		return view
	}
	view.CoveredSince = report.CoveredSince
	view.Incomplete = report.Incomplete
	byUser := map[string]passwordlessDomainRow{}
	byName := map[string]passwordlessDomainRow{}
	for _, domain := range domains {
		name := strings.ToLower(domain.name)
		byName[name] = domain
		byUser[services.SiteUsername(name)] = domain
	}
	sites := map[int]*passwordlessMailSite{}
	site := func(domain passwordlessDomainRow) *passwordlessMailSite {
		entry := sites[domain.id]
		if entry == nil {
			entry = &passwordlessMailSite{DomainID: domain.id, Domain: strings.ToLower(domain.name), Ways: []string{}}
			sites[domain.id] = entry
		}
		return entry
	}
	add := func(entry *passwordlessMailSite, way string, messages, refused int, last string) {
		if !containsString(entry.Ways, way) {
			entry.Ways = append(entry.Ways, way)
		}
		entry.Messages += messages
		entry.Refused += refused
		if last > entry.Last {
			entry.Last = last
		}
	}
	for _, sender := range report.LocalSenders {
		if sender.System {
			continue
		}
		if domain, ok := byUser[sender.User]; ok && sender.User != "" {
			add(site(domain), passwordlessWaySendmail, sender.Messages, sender.Refused, sender.Last)
			continue
		}
		label := sender.User
		if label == "" {
			label = "uid " + strconv.Itoa(sender.UID)
		}
		view.Others = append(view.Others, passwordlessMailOther{Label: label, Way: passwordlessWaySendmail,
			Messages: sender.Messages, Refused: sender.Refused, Last: sender.Last})
	}
	for _, sender := range report.LoopbackSenders {
		address := strings.ToLower(sender.Sender)
		if address == "" || (ownSender != "" && address == ownSender) {
			continue
		}
		at := strings.LastIndexByte(address, '@')
		if domain, ok := byName[address[at+1:]]; ok && at >= 0 {
			add(site(domain), passwordlessWaySMTP, sender.Messages, sender.Refused, sender.Last)
			continue
		}
		view.Others = append(view.Others, passwordlessMailOther{Label: address, Way: passwordlessWaySMTP,
			Messages: sender.Messages, Refused: sender.Refused, Last: sender.Last})
	}
	for _, entry := range sites {
		view.Sites = append(view.Sites, *entry)
	}
	sort.Slice(view.Sites, func(i, j int) bool { return view.Sites[i].Domain < view.Sites[j].Domain })
	view.State = "none"
	if len(view.Sites) > 0 || len(view.Others) > 0 {
		view.State = "found"
	}
	return view
}
