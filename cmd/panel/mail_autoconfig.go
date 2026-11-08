package main

import (
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Mail programs that set themselves up (alpha.130; docs/MAIL-AUTOCONFIG.md).
//
// A top-level domain whose mail is on this server gets, in its own site
// file, the answers Thunderbird (autoconfig) and Outlook (Autodiscover) ask
// for: the server's mail name, IMAP 993 over TLS, sending on 465 (TLS) and
// 587 (STARTTLS), never a password. The names autoconfig.<domain> and
// autodiscover.<domain> are CNAMEs to mail.<domain> (written with every new
// zone; for an existing zone with one press) and an _autodiscover._tcp SRV
// points Outlook at autodiscover.<domain>:443. The certificate of the site
// carries the two names when the public DNS shows them here; otherwise they
// are left out with a note and the certificate is issued as before.
//
//	GET  /api/v1/domains/{id}/mail/setup        includes "autoconfig": state,
//	                                            what is missing and who acts
//	POST /api/v1/domains/{id}/mail/autoconfig   writes the site file again and
//	                                            adds the missing DNS records
//	                                            (steps: site, dns)
//
// Posta programları kendini ayarlar: yanıtlar sitenin dosyasındadır; DNS
// kayıtları yeni bölgeyle ya da tek düğmeyle eklenir; sertifika adları ancak
// bu sunucuyu gösteriyorsa alır.

const (
	mailAutoconfigOn          = "on"
	mailAutoconfigPartial     = "partial"
	mailAutoconfigOff         = "off"
	mailAutoconfigUnavailable = "unavailable"

	mailAutoconfigReasonSubdomain   = "subdomain"
	mailAutoconfigReasonMailNotHere = "mail_not_here"
	mailAutoconfigReasonNameTaken   = "name_taken"
	mailAutoconfigReasonNoSite      = "no_site"
	mailAutoconfigReasonMailHost    = "mail_host_unknown"
	mailAutoconfigReasonSuspended   = "suspended"
)

// mailAutoconfigMailHere: Postfix and Dovecot are on this server, read from
// the last component scan (no host command). Swapped by tests.
var mailAutoconfigMailHere = func(p *Panel, ctx context.Context) bool {
	installed := p.cachedInstalledSet(ctx)
	return installed["postfix"] && installed["dovecot"]
}

// mailAutoconfigPlan says whether a domain's site file answers mail programs.
type mailAutoconfigPlan struct {
	available bool
	reason    string
	mailHost  string
	names     []string
}

// mailAutoconfigFor decides it from the panel's own records only; it never
// asks the network, so every render of a site decides the same.
func (p *Panel) mailAutoconfigFor(ctx context.Context, domainID int) (mailAutoconfigPlan, error) {
	var (
		name        string
		parent      sql.NullInt64
		projectType sql.NullString
		siteID      sql.NullInt64
	)
	err := p.db.GetDB().QueryRowContext(ctx, `
		SELECT d.name, d.parent_domain_id, s.project_type, s.id
		FROM domains d LEFT JOIN sites s ON s.domain_id = d.id
		WHERE d.id = ?`, domainID).Scan(&name, &parent, &projectType, &siteID)
	if err != nil {
		return mailAutoconfigPlan{}, err
	}
	name = strings.ToLower(strings.TrimSuffix(name, "."))
	plan := mailAutoconfigPlan{names: services.MailAutoconfigNames(name)}
	switch {
	case parent.Valid:
		plan.reason = mailAutoconfigReasonSubdomain
		return plan, nil
	case !siteID.Valid || projectType.String == "dnsonly":
		plan.reason = mailAutoconfigReasonNoSite
		return plan, nil
	}
	if !mailAutoconfigMailHere(p, ctx) {
		plan.reason = mailAutoconfigReasonMailNotHere
		return plan, nil
	}
	host, err := p.mailTLSHostname(ctx)
	if err != nil {
		plan.reason = mailAutoconfigReasonMailHost
		return plan, nil
	}
	var taken int
	if err := p.db.GetDB().QueryRowContext(ctx, `
		SELECT (SELECT COUNT(*) FROM domains WHERE lower(name) IN (?, ?))
		     + (SELECT COUNT(*) FROM domain_aliases WHERE lower(alias) IN (?, ?))`,
		plan.names[0], plan.names[1], plan.names[0], plan.names[1]).Scan(&taken); err != nil {
		return mailAutoconfigPlan{}, err
	}
	if webmail, ok := p.webmailAddress(ctx); ok {
		for _, n := range plan.names {
			if strings.EqualFold(webmail.Host, n) {
				taken++
			}
		}
	}
	if taken > 0 {
		plan.reason = mailAutoconfigReasonNameTaken
		return plan, nil
	}
	// The domain's own mail name when its certificate carries it for mail
	// (Dovecot and Postfix then show that certificate for it); otherwise the
	// server's mail name, which always has one.
	var secureMail int
	if err := p.db.GetDB().QueryRowContext(ctx, `
		SELECT COUNT(*) FROM ssl_certificates WHERE domain_id = ? AND status = 'active' AND secure_mail = 1`,
		domainID).Scan(&secureMail); err != nil {
		return mailAutoconfigPlan{}, err
	}
	if secureMail > 0 {
		host = "mail." + name
	}
	plan.available, plan.mailHost = true, host
	return plan, nil
}

// mailAutoconfigRequest is the render's part: nil when the domain's site
// file does not answer mail programs. A failure to decide is logged and
// leaves the answers out; it never stops the site's render.
func (p *Panel) mailAutoconfigRequest(ctx context.Context, domainID int, certificateNames []string) *transport.MailAutoconfigRequest {
	plan, err := p.mailAutoconfigFor(ctx, domainID)
	if err != nil {
		log.Printf("[mail-autoconfig] domain %d: %v; the site file is written without the mail set-up answers", domainID, err)
		return nil
	}
	if !plan.available {
		return nil
	}
	covered := len(certificateNames) > 0
	for _, name := range plan.names {
		covered = covered && certificateCoversHostname(certificateNames, name)
	}
	return &transport.MailAutoconfigRequest{MailHost: plan.mailHost, TLS: covered}
}

// mailAutoconfigCertificateNames are the set-up names a certificate request
// adds: only those the public DNS shows here. The others are left out with a
// plain note, and the certificate is requested as before.
func (p *Panel) mailAutoconfigCertificateNames(ctx context.Context, domainID int) ([]string, string) {
	plan, err := p.mailAutoconfigFor(ctx, domainID)
	if err != nil || !plan.available {
		return nil, ""
	}
	checkCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	checks := checkCertificateNames(checkCtx, plan.names, "")
	cancel()
	var here, skipped []string
	for _, check := range checks {
		if check.State == certificateNameHere {
			here = append(here, check.Name)
		} else {
			skipped = append(skipped, check.Name)
		}
	}
	if len(skipped) == 0 {
		return here, ""
	}
	return here, strings.Join(skipped, " and ") + " do not point to this server yet, so they were left out of the certificate. Mail programs still set themselves up over plain HTTP; once the names point here, request the certificate again to add them."
}

// ---- DNS records --------------------------------------------------------------

type mailAutoconfigRecord struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Content string `json:"content"`
	Prio    int    `json:"prio,omitempty"`
	// State: present, missing, different (another record holds the name;
	// it is never replaced) or unknown (the zone is not on this server).
	State string `json:"state"`
}

// mailAutoconfigRecords are the records mail programs look for.
func mailAutoconfigRecords(domain, target string) []mailAutoconfigRecord {
	names := services.MailAutoconfigNames(domain)
	return []mailAutoconfigRecord{
		{Name: names[0], Type: "CNAME", Content: target},
		{Name: names[1], Type: "CNAME", Content: target},
		{Name: "_autodiscover._tcp." + domain, Type: "SRV", Content: "0 443 " + names[1], Prio: 0},
	}
}

// mailAutoconfigZoneState reads the records of a domain's zone on this
// server. A name that already holds another record is "different" and is
// never replaced (a CNAME cannot share its name).
func (p *Panel) mailAutoconfigZoneState(ctx context.Context, zoneID int, records []mailAutoconfigRecord) ([]mailAutoconfigRecord, error) {
	out := make([]mailAutoconfigRecord, 0, len(records))
	for _, record := range records {
		rows, err := p.db.GetDB().QueryContext(ctx, `SELECT type, content FROM pdns_records WHERE domain_id = ? AND lower(name) = ?`, zoneID, record.Name)
		if err != nil {
			return nil, err
		}
		state := "missing"
		for rows.Next() {
			var typ, content string
			if err := rows.Scan(&typ, &content); err != nil {
				rows.Close()
				return nil, err
			}
			switch {
			case strings.EqualFold(typ, record.Type) && strings.EqualFold(strings.TrimSuffix(content, "."), record.Content):
				state = "present"
			case state != "present" && (record.Type == "CNAME" || strings.EqualFold(typ, "CNAME") || strings.EqualFold(typ, record.Type)):
				state = "different"
			}
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
		record.State = state
		out = append(out, record)
	}
	return out, nil
}

// mailAutoconfigTarget is the name the CNAMEs point to: mail.<domain> when
// the zone has it (every zone the panel made does), else the server's mail
// name.
func (p *Panel) mailAutoconfigTarget(ctx context.Context, zoneID int, domain, mailHost string) string {
	var count int
	_ = p.db.GetDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM pdns_records WHERE domain_id = ? AND lower(name) = ? AND type IN ('A', 'AAAA', 'CNAME')`,
		zoneID, "mail."+domain).Scan(&count)
	if count > 0 || mailHost == "" {
		return "mail." + domain
	}
	return mailHost
}

// ---- The view on the mailbox screen -------------------------------------------

type mailAutoconfigView struct {
	State  string   `json:"state"`
	Reason string   `json:"reason,omitempty"`
	Names  []string `json:"names"`
	// MailHost is the mail server name the programs are told (also shown
	// in the manual settings).
	MailHost string `json:"mail_host,omitempty"`
	// Served: this server's web server answers both names now (plain HTTP),
	// Secure: and over HTTPS with a certificate for them (yes, no, unknown).
	Served string `json:"served"`
	Secure string `json:"secure"`
	// DNS: local (this server's zone), remote (another CelikPanel holds it)
	// or external (the owner's DNS provider).
	DNS     string                 `json:"dns"`
	Records []mailAutoconfigRecord `json:"records"`
	// CanFix: one press here can finish it (write the site file again, add
	// the missing records). False when only someone else can act.
	CanFix bool `json:"can_fix"`
}

// Swapped by tests: the local reads of what this server's nginx answers.
var (
	mailAutoconfigProbeHTTP = func(ctx context.Context, host string) (bool, error) {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1/mail/config-v1.1.xml", nil)
		if err != nil {
			return false, err
		}
		request.Host = host
		client := &http.Client{Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		response, err := client.Do(request)
		if err != nil {
			return false, err
		}
		defer response.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(response.Body, 8192))
		return response.StatusCode == http.StatusOK && strings.Contains(string(body), "<clientConfig"), nil
	}
	mailAutoconfigProbeTLS = func(ctx context.Context, host string) (bool, error) {
		dialer := &tls.Dialer{NetDialer: &net.Dialer{Timeout: 3 * time.Second}, Config: &tls.Config{ServerName: host, InsecureSkipVerify: true}}
		probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		conn, err := dialer.DialContext(probeCtx, "tcp", "127.0.0.1:443")
		if err != nil {
			return false, err
		}
		defer conn.Close()
		state := conn.(*tls.Conn).ConnectionState()
		if len(state.PeerCertificates) == 0 {
			return false, nil
		}
		leaf := state.PeerCertificates[0]
		return leaf.VerifyHostname(host) == nil && time.Now().Before(leaf.NotAfter), nil
	}
)

func probeBoth(ctx context.Context, names []string, probe func(context.Context, string) (bool, error)) string {
	for _, name := range names {
		ok, err := probe(ctx, name)
		if err != nil {
			return "unknown"
		}
		if !ok {
			return "no"
		}
	}
	return "yes"
}

func (p *Panel) mailAutoconfigView(ctx context.Context, domainID int, domain string) mailAutoconfigView {
	view := mailAutoconfigView{State: mailAutoconfigUnavailable, Names: services.MailAutoconfigNames(domain), Served: "unknown", Secure: "unknown", Records: []mailAutoconfigRecord{}}
	plan, err := p.mailAutoconfigFor(ctx, domainID)
	if err != nil {
		log.Printf("[mail-autoconfig] domain %d: %v", domainID, err)
		view.Reason = "unknown"
		return view
	}
	if !plan.available {
		view.Reason = plan.reason
		return view
	}
	view.MailHost = plan.mailHost
	if suspended, err := p.domainSubscriptionSuspended(ctx, domainID); err == nil && suspended {
		view.Reason = mailAutoconfigReasonSuspended
		return view
	}
	view.Served = probeBoth(ctx, plan.names, mailAutoconfigProbeHTTP)
	if view.Served == "yes" {
		view.Secure = probeBoth(ctx, plan.names, mailAutoconfigProbeTLS)
	} else if view.Served == "no" {
		view.Secure = "no"
	}

	mode, err := p.domainDNSManagementMode(ctx, domain)
	if err != nil {
		mode = ""
	}
	recordsReady := false
	switch mode {
	case setupDNSModeLocal:
		view.DNS = "local"
		zoneID, zoneName, found, err := p.mailAuthZone(ctx, domain)
		if err == nil && found && zoneName == domain {
			records, err := p.mailAutoconfigZoneState(ctx, zoneID, mailAutoconfigRecords(domain, p.mailAutoconfigTarget(ctx, zoneID, domain, plan.mailHost)))
			if err == nil {
				view.Records = records
				recordsReady = true
				for _, record := range records {
					recordsReady = recordsReady && record.State == "present"
					view.CanFix = view.CanFix || record.State == "missing"
				}
			}
		}
	case setupDNSModeExisting:
		view.DNS = "remote"
	default:
		view.DNS = "external"
	}
	if view.DNS != "local" {
		for _, record := range mailAutoconfigRecords(domain, "mail."+domain) {
			record.State = "unknown"
			view.Records = append(view.Records, record)
		}
	}
	view.CanFix = view.CanFix || view.Served == "no"
	switch {
	case view.Served == "yes" && view.Secure == "yes" && (recordsReady || view.DNS != "local"):
		view.State = mailAutoconfigOn
	case view.Served == "no" && !recordsReady:
		view.State = mailAutoconfigOff
	default:
		view.State = mailAutoconfigPartial
	}
	return view
}

// domainSubscriptionSuspended: a suspended subscription's site serves only
// its suspended page, so its mail programs cannot set themselves up.
func (p *Panel) domainSubscriptionSuspended(ctx context.Context, domainID int) (bool, error) {
	var subscriptionID int
	if err := p.db.GetDB().QueryRowContext(ctx, `SELECT subscription_id FROM domains WHERE id = ?`, domainID).Scan(&subscriptionID); err != nil {
		return false, err
	}
	return p.subscriptionSuspended(ctx, subscriptionID)
}

// ---- Turning it on ------------------------------------------------------------

// handleMailAutoconfig writes the site file again (it then carries the
// answers) and adds the missing DNS records on this server's zone. A name
// that holds another record is never replaced; it is reported.
func (p *Panel) handleMailAutoconfig(w http.ResponseWriter, r *http.Request, domainID int) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeClientError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	ctx := r.Context()
	domain, err := p.domainNameByIDStrict(ctx, domainID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	reportOperationPlan(ctx, "check", "site", "dns")
	reportOperationStep(ctx, "check")
	plan, err := p.mailAutoconfigFor(ctx, domainID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if !plan.available {
		writeCodedErrorVars(w, http.StatusConflict, "MAIL_AUTOCONFIG_UNAVAILABLE",
			"mail programs cannot set themselves up for this domain ("+plan.reason+"); nothing was changed", "", nil, map[string]string{"reason": plan.reason})
		return
	}

	reportOperationStep(ctx, "site")
	if err := p.applyVhostForDomain(ctx, domainID); err != nil {
		var held *siteFileHeldError
		if errors.As(err, &held) {
			writeCodedErrorVars(w, http.StatusConflict, "MAIL_AUTOCONFIG_SITE_FILE_KEPT",
				"the site's web server file was changed by the server owner, so CelikPanel kept it and did not add the mail set-up answers; nothing was changed. The owner keeps the file as it is or lets CelikPanel write it on the site's Configuration file page", "", nil, map[string]string{"reason": held.File.Reason})
			return
		}
		writeServerError(w, err)
		return
	}
	p.audit(r, "mail.autoconfig.site", "domain", domainID)

	reportOperationStep(ctx, "dns")
	answer := map[string]any{"success": true}
	mode, err := p.domainDNSManagementMode(ctx, domain)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if mode != setupDNSModeLocal {
		answer["dns"] = "not_here"
		_ = json.NewEncoder(w).Encode(answer)
		return
	}
	zoneID, zoneName, found, err := p.mailAuthZone(ctx, domain)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if !found || zoneName != domain {
		answer["dns"] = "no_zone"
		_ = json.NewEncoder(w).Encode(answer)
		return
	}
	records, err := p.mailAutoconfigZoneState(ctx, zoneID, mailAutoconfigRecords(domain, p.mailAutoconfigTarget(ctx, zoneID, domain, plan.mailHost)))
	if err != nil {
		writeServerError(w, err)
		return
	}
	var missing []mailAutoconfigRecord
	var different []string
	for _, record := range records {
		switch record.State {
		case "missing":
			missing = append(missing, record)
		case "different":
			different = append(different, record.Name)
		}
	}
	if len(different) > 0 {
		answer["different"] = different
	}
	if len(missing) == 0 {
		answer["dns"] = "present"
		_ = json.NewEncoder(w).Encode(answer)
		return
	}
	if _, ready := p.requireActiveDNSPublisherForMutation(w, ctx); !ready {
		return
	}
	tx, err := beginDNSWriteTx(ctx, p.db.GetDB())
	if err != nil {
		writeServerError(w, err)
		return
	}
	defer tx.Rollback()
	for _, record := range missing {
		var prio any
		if record.Type == "SRV" {
			prio = record.Prio
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO pdns_records (domain_id, name, type, content, ttl, prio) VALUES (?, ?, ?, ?, 3600, ?)`,
			zoneID, record.Name, record.Type, record.Content, prio); err != nil {
			writeServerError(w, err)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		writeServerError(w, err)
		return
	}
	p.audit(r, "mail.autoconfig.dns", "domain", domainID)
	publication, ok := p.publishDNSMutation(w, ctx, zoneName)
	if !ok {
		return
	}
	answer["dns"] = "added"
	answer["publication"] = publication
	_ = json.NewEncoder(w).Encode(answer)
}
