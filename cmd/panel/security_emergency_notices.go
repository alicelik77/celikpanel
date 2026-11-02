package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
)

// Security notices (alpha.117, D-043). Each affected customer and its
// reseller is told what happened, why (the administrator's reason), what
// happened to their sites, what to do, with a one-click switch to the
// recommended alternative. The notice stays in the panel until it is marked
// as seen, and is handed to the server's own mail system when the panel's
// account mail path is available (account_mail.go); without it the notice
// is in the panel only, and the administrator's screen says so. A missing
// mail system never fails the emergency.
//
// Güvenlik bildirimleri: panelde görülene dek durur; posta yolu varsa
// e-postayla da gider. Posta yokluğu acil durumu asla başarısız kılmaz.

const securityNoticesPath = "/api/v1/security-notices"

type securityNoticeSite struct {
	DomainID    int    `json:"domain_id"`
	Domain      string `json:"domain"`
	Customer    string `json:"customer,omitempty"`
	Maintenance bool   `json:"maintenance"`
}

type securityNoticeBody struct {
	Component   string               `json:"component"`
	Version     string               `json:"version"`
	Name        string               `json:"name"`
	Reason      string               `json:"reason"`
	Stage       string               `json:"stage"`
	Recommended string               `json:"recommended,omitempty"`
	Sites       []securityNoticeSite `json:"sites"`
}

// createEmergencyNotices writes one notice per affected customer and
// reseller (a reseller that also owns sites gets one notice for both). A
// notice already there for the same emergency and kind is renewed: unseen
// again, and its mail pending again.
func createEmergencyNotices(ctx context.Context, tx sqlQueryer, e securityEmergency, sites []emergencySite, kind string) error {
	type recipient struct {
		audience string
		sites    []securityNoticeSite
	}
	recipients := map[int]*recipient{}
	add := func(userID int, audience string, site securityNoticeSite) {
		if userID <= 0 {
			return
		}
		r := recipients[userID]
		if r == nil {
			r = &recipient{audience: audience}
			recipients[userID] = r
		}
		if audience == "reseller" {
			r.audience = "reseller"
		}
		r.sites = append(r.sites, site)
	}
	for _, site := range sites {
		entry := securityNoticeSite{DomainID: site.DomainID, Domain: site.Domain, Maintenance: kind == "emergency"}
		add(site.CustomerID, "customer", entry)
		if site.ResellerID > 0 && site.ResellerID != site.CustomerID {
			entry.Customer = site.Customer
			add(site.ResellerID, "reseller", entry)
		}
	}
	for userID, r := range recipients {
		body, err := json.Marshal(securityNoticeBody{Component: e.Component, Version: e.Version, Name: e.Name, Reason: e.Reason,
			Stage: e.Stage, Recommended: e.Recommended, Sites: r.sites})
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO security_notices (emergency_id, user_id, kind, audience, body) VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(emergency_id, user_id, kind) DO UPDATE SET audience = excluded.audience, body = excluded.body,
				mail_state = 'pending', mail_detail = NULL, seen_at = NULL, created_at = datetime('now')`,
			e.ID, userID, kind, r.audience, string(body)); err != nil {
			return err
		}
	}
	// A resolved notice ends the open emergency notice of the same people.
	if kind == "resolved" {
		if _, err := tx.ExecContext(ctx, `UPDATE security_notices SET seen_at = COALESCE(seen_at, datetime('now'))
			WHERE emergency_id = ? AND kind = 'emergency'`, e.ID); err != nil {
			return err
		}
	}
	return nil
}

// securityNoticeMail writes the bilingual mail of one notice.
func securityNoticeMail(kind, audience string, body securityNoticeBody, baseURL string) (string, string) {
	domains := []string{}
	for _, site := range body.Sites {
		if site.Customer != "" {
			domains = append(domains, "  - "+site.Domain+" ("+site.Customer+")")
		} else {
			domains = append(domains, "  - "+site.Domain)
		}
	}
	list := strings.Join(domains, "\n")
	if kind == "resolved" {
		subject := "Resolved: " + body.Name + " / Çözüldü: " + body.Name
		return subject, strings.Join([]string{
			"The security problem in " + body.Name + " on this server is resolved; the sites below show their own pages again:",
			list,
			"",
			"There is nothing you need to do.",
			"",
			"---",
			"",
			"Bu sunucudaki " + body.Name + " güvenlik sorunu giderildi; aşağıdaki siteler yeniden kendi sayfalarını gösteriyor:",
			list,
			"",
			"Yapmanız gereken bir şey yok.",
			"",
			baseURL,
		}, "\n")
	}
	subject := "Security notice: " + body.Name + " / Güvenlik bildirimi: " + body.Name
	whatToDo := "Switch the site to another version: sign in to the panel; the notice at the top has a one-click switch."
	whatToDoTR := "Siteyi başka bir sürüme geçirin: panele girin; üstteki bildirimde tek tıkla geçiş düğmesi var."
	if body.Recommended != "" {
		whatToDo = "Switch the site to " + emergencyComponents[body.Component].Name + " " + body.Recommended + ": sign in to the panel; the notice at the top has a one-click switch."
		whatToDoTR = "Siteyi " + emergencyComponents[body.Component].Name + " " + body.Recommended + " sürümüne geçirin: panele girin; üstteki bildirimde tek tıkla geçiş düğmesi var."
	}
	if body.Version == "" {
		whatToDo = "Nothing is deleted; the sites come back when the administrator re-enables " + body.Name + ". Ask your provider if you need help."
		whatToDoTR = "Hiçbir şey silinmedi; yönetici " + body.Name + " hizmetini yeniden açınca siteler geri gelir. Yardım gerekirse sağlayıcınıza başvurun."
	}
	who := "Your sites"
	whoTR := "Siteleriniz"
	if audience == "reseller" {
		who = "Sites of your customers (and yours)"
		whoTR = "Müşterilerinizin (ve sizin) siteleriniz"
	}
	return subject, strings.Join([]string{
		"The administrator stopped " + body.Name + " on this server because of a security problem.",
		"Reason: " + body.Reason,
		"",
		who + " that use it now show \"This site is temporarily under maintenance\":",
		list,
		"",
		"Nothing was deleted: files, databases and mailboxes stay.",
		"What to do: " + whatToDo,
		"",
		"---",
		"",
		"Yönetici bu sunucudaki " + body.Name + " hizmetini bir güvenlik sorunu nedeniyle durdurdu.",
		"Neden: " + body.Reason,
		"",
		whoTR + " şimdi \"Bu site geçici olarak bakımdadır\" sayfasını gösteriyor:",
		list,
		"",
		"Hiçbir şey silinmedi: dosyalar, veritabanları ve posta kutuları duruyor.",
		"Ne yapmalı: " + whatToDoTR,
		"",
		baseURL,
	}, "\n")
}

// sendPendingSecurityNotices hands every pending notice to the mail system,
// or records why it could not.
func (p *Panel) sendPendingSecurityNotices(ctx context.Context) error {
	db := p.db.GetDB()
	rows, err := db.QueryContext(ctx, `
		SELECT n.id, n.kind, n.audience, n.body, COALESCE(u.email, '')
		FROM security_notices n LEFT JOIN users u ON u.id = n.user_id
		WHERE n.mail_state = 'pending' ORDER BY n.id LIMIT 200`)
	if err != nil {
		return err
	}
	type pending struct {
		id                          int
		kind, audience, body, email string
	}
	var items []pending
	for rows.Next() {
		var item pending
		if err := rows.Scan(&item.id, &item.kind, &item.audience, &item.body, &item.email); err != nil {
			rows.Close()
			return err
		}
		items = append(items, item)
	}
	rows.Close()
	if len(items) == 0 {
		return rows.Err()
	}
	setup := p.accountMailSetup(ctx)
	mark := func(id int, state, detail string) {
		if _, err := db.ExecContext(ctx, `UPDATE security_notices SET mail_state = ?, mail_detail = ? WHERE id = ?`, state, nullIfEmpty(detail), id); err != nil {
			log.Printf("[emergency] notice %d mail state: %v", id, err)
		}
	}
	for _, item := range items {
		if !setup.Available {
			mark(item.id, "not_available", setup.Reason)
			continue
		}
		if strings.TrimSpace(item.email) == "" {
			mark(item.id, "no_address", "")
			continue
		}
		var body securityNoticeBody
		if err := json.Unmarshal([]byte(item.body), &body); err != nil {
			mark(item.id, "failed", "the notice could not be read")
			continue
		}
		subject, text := securityNoticeMail(item.kind, item.audience, body, setup.baseURL)
		if err := p.sendAccountMail(ctx, setup, strings.TrimSpace(item.email), accountMailContent{kind: accountMailSecurityNotice, subject: subject, body: text}); err != nil {
			mark(item.id, "failed", boundedAgentDiagnostic(err.Error()))
			continue
		}
		mark(item.id, "sent", "")
	}
	return nil
}

// securityNoticeView is one notice as its reader sees it.
type securityNoticeView struct {
	ID        int                `json:"id"`
	Kind      string             `json:"kind"`
	Audience  string             `json:"audience"`
	CreatedAt string             `json:"created_at"`
	Body      securityNoticeBody `json:"body"`
	// Sites still under maintenance, with the one-click switch when an
	// alternative is recommended.
	Actions []securityNoticeAction `json:"actions"`
}

type securityNoticeAction struct {
	DomainID int    `json:"domain_id"`
	Domain   string `json:"domain"`
	Kind     string `json:"kind"` // php | node
	To       string `json:"to"`
}

// handleSecurityNotices: GET the caller's unseen notices; POST
// /{id}/seen marks one as seen. Only the notice's own reader sees it.
func (p *Panel) handleSecurityNotices(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	caller := currentCaller(r)
	if caller == nil {
		writeClientError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	db := p.db.GetDB()
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, securityNoticesPath), "/")
	if rest == "" && r.Method == http.MethodGet {
		rows, err := db.QueryContext(r.Context(), `SELECT id, kind, audience, body, created_at FROM security_notices
			WHERE user_id = ? AND seen_at IS NULL ORDER BY created_at DESC, id DESC LIMIT 50`, caller.ID)
		if err != nil {
			writeServerError(w, err)
			return
		}
		views := []securityNoticeView{}
		for rows.Next() {
			var v securityNoticeView
			var raw string
			if err := rows.Scan(&v.ID, &v.Kind, &v.Audience, &raw, &v.CreatedAt); err != nil {
				rows.Close()
				writeServerError(w, err)
				return
			}
			if json.Unmarshal([]byte(raw), &v.Body) != nil {
				continue
			}
			views = append(views, v)
		}
		rows.Close()
		for i := range views {
			views[i].Actions = p.securityNoticeActions(r.Context(), views[i])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"notices": views})
		return
	}
	idText, action, _ := strings.Cut(rest, "/")
	id, err := strconv.Atoi(idText)
	if err != nil || id <= 0 || action != "seen" || r.Method != http.MethodPost {
		writeClientError(w, http.StatusNotFound, "not found")
		return
	}
	result, err := db.ExecContext(r.Context(), `UPDATE security_notices SET seen_at = COALESCE(seen_at, datetime('now')) WHERE id = ? AND user_id = ?`, id, caller.ID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if n, _ := result.RowsAffected(); n != 1 {
		writeClientError(w, http.StatusNotFound, "not found")
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]bool{"seen": true})
}

// securityNoticeActions: the sites of a notice that still show the
// maintenance page, each with the switch to the recommended version.
func (p *Panel) securityNoticeActions(ctx context.Context, v securityNoticeView) []securityNoticeAction {
	actions := []securityNoticeAction{}
	if v.Kind != "emergency" || v.Body.Recommended == "" || (v.Body.Component != "php" && v.Body.Component != "node") {
		return actions
	}
	for _, site := range v.Body.Sites {
		under, err := siteUnderEmergencyMaintenance(ctx, p.db.GetDB(), site.DomainID)
		if err != nil || !under {
			continue
		}
		actions = append(actions, securityNoticeAction{DomainID: site.DomainID, Domain: site.Domain, Kind: v.Body.Component, To: v.Body.Recommended})
	}
	return actions
}

// emergencyNoticeSummary is the administrator's view of the notices.
type emergencyNoticeSummary struct {
	Total        int    `json:"total"`
	Seen         int    `json:"seen"`
	MailSent     int    `json:"mail_sent"`
	MailPending  int    `json:"mail_pending"`
	MailFailed   int    `json:"mail_failed"`
	NoAddress    int    `json:"no_address"`
	NotAvailable int    `json:"mail_not_available"`
	MailReason   string `json:"mail_reason,omitempty"`
}

func loadEmergencyNoticeSummary(ctx context.Context, q sqlQueryer, emergencyID string) (emergencyNoticeSummary, error) {
	var s emergencyNoticeSummary
	var reason sql.NullString
	err := q.QueryRowContext(ctx, `
		SELECT COUNT(*), COALESCE(SUM(seen_at IS NOT NULL), 0),
		       COALESCE(SUM(mail_state = 'sent'), 0), COALESCE(SUM(mail_state = 'pending'), 0),
		       COALESCE(SUM(mail_state = 'failed'), 0), COALESCE(SUM(mail_state = 'no_address'), 0),
		       COALESCE(SUM(mail_state = 'not_available'), 0),
		       (SELECT mail_detail FROM security_notices WHERE emergency_id = ? AND mail_state = 'not_available' ORDER BY id DESC LIMIT 1)
		FROM security_notices WHERE emergency_id = ?`, emergencyID, emergencyID).
		Scan(&s.Total, &s.Seen, &s.MailSent, &s.MailPending, &s.MailFailed, &s.NoAddress, &s.NotAvailable, &reason)
	if errors.Is(err, sql.ErrNoRows) {
		return s, nil
	}
	s.MailReason = reason.String
	return s, err
}

func emergencyMailReasonText(reason string) string {
	switch reason {
	case accountMailReasonNoPanelAddress:
		return "the panel has no public address for links"
	case accountMailReasonNoSender:
		return "no sender domain is known"
	case accountMailReasonNoMailServer:
		return "no mail system answers on this server"
	}
	return fmt.Sprintf("mail is not available (%s)", reason)
}
