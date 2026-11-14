package main

import (
	"context"
	"log"
	"strings"
	"time"
)

// mail.<domain> in a domain's certificate by default (alpha.134,
// docs/CERTIFICATE-COVERAGE.md §3).
//
// A certificate request for a domain whose mail is on this server includes
// mail.<domain> without a tick when the public DNS shows the name here, and
// mail then serves the certificate for that name. Only serving changes: the
// server's own mail name keeps its certificate on every port and the name
// mail programs are told stays as it was (mailClientHost), so nothing that
// works starts to warn. The person can leave the name out (exclude_mail).
//
// Alan adının posta adı varsayılan olarak sertifikaya girer, ad bu sunucuyu
// gösteriyorsa; posta yalnız o ad için bu sertifikayı sunar.

// What happened to mail.<domain> that the person did not ask for, in the
// answer of a certificate request ("mail_note").
const (
	// certificateMailNoteSiteFileKept: the site's file is the owner's, so the
	// name could not be validated and was left out.
	certificateMailNoteSiteFileKept = "site_file_kept"
	// certificateMailNoteNotSwitched: the certificate carries the name, but
	// mail keeps the server's certificate for it (mail could not take it, or
	// the server has no mail name yet). Nothing else changed for mail.
	certificateMailNoteNotSwitched = "mail_not_switched"
)

// defaultMailNameState asks the public DNS where mail.<domain> points.
// Swapped by tests.
var defaultMailNameState = func(ctx context.Context, name string) string {
	checkCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	checks := checkCertificateNames(checkCtx, nil, name)
	if len(checks) != 1 {
		return certificateNameUnknown
	}
	return checks[0].State
}

// defaultMailCertificateName is mail.<domain> when a request should carry it
// without being asked: mail (Postfix and Dovecot) is on this server and the
// public DNS shows the name here. Otherwise "": a name that points elsewhere,
// has no address or could not be checked is not included, and the request
// goes on (the coverage list says why).
func (p *Panel) defaultMailCertificateName(ctx context.Context, domain string) string {
	if !mailAutoconfigMailHere(p, ctx) {
		return ""
	}
	name, err := mailCertificateHostname(domain)
	if err != nil {
		return ""
	}
	if defaultMailNameState(ctx, name) != certificateNameHere {
		return ""
	}
	return name
}

func removeCertificateName(names []string, name string) []string {
	out := make([]string, 0, len(names))
	for _, candidate := range names {
		if !strings.EqualFold(strings.TrimSuffix(candidate, "."), strings.TrimSuffix(name, ".")) {
			out = append(out, candidate)
		}
	}
	return out
}

// restoreMailBeforeServing puts mail back as it was before a certificate
// request asked it to start serving mail.<domain>, after mail said a known no
// (cause). It reports false when mail could not be put back; the caller then
// keeps the requested state and its "retry" (the existing pending state).
func (p *Panel) restoreMailBeforeServing(domainID int, cause error) bool {
	rollbackCtx, cancel := sslCompensationContext()
	defer cancel()
	log.Printf("SSL issue domain %d: mail could not start serving the domain's mail name: %v; mail keeps the server's certificate", domainID, cause)
	if _, err := p.db.GetDB().ExecContext(rollbackCtx, `
		UPDATE ssl_certificates SET secure_mail = 0, updated_at = datetime('now')
		WHERE domain_id = ? AND status = 'active'`, domainID); err != nil {
		log.Printf("SSL issue domain %d: restore mail ledger: %v", domainID, err)
		return false
	}
	if err := p.resyncMailTLS(rollbackCtx); err != nil {
		log.Printf("SSL issue domain %d: publish mail as it was: %v", domainID, err)
		if _, reinstateErr := p.db.GetDB().ExecContext(rollbackCtx, `
			UPDATE ssl_certificates SET secure_mail = 1, updated_at = datetime('now')
			WHERE domain_id = ? AND status = 'active'`, domainID); reinstateErr != nil {
			log.Printf("SSL issue domain %d: reinstate requested mail ledger: %v", domainID, reinstateErr)
		}
		return false
	}
	return true
}

// addCertificateMailAnswer says in a successful request's answer whether the
// certificate carries mail.<domain>, whether mail serves it, and the note.
func addCertificateMailAnswer(answer map[string]any, mailName string, served bool, note string) {
	if mailName != "" {
		answer["mail_name"] = mailName
	}
	answer["mail_served"] = served
	if note != "" {
		answer["mail_note"] = note
	}
}
