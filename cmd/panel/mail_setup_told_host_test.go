package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.139 (each mailbox's connection settings): the mail server name of a
// domain's setup answer is the name its mail programs are told, whatever its
// site answers the set-up requests with. A subdomain, a domain without a site
// and a domain whose set-up name is in use were told mail.<domain>, a name this
// server neither chose nor serves with a certificate. Only mail not on this
// server and no mail name of the server keep that stand-in, and then the
// answer's autoconfig carries no mail_host, so the screens say it is not
// confirmed. Read through the production route (handleDomainMail).
//
// Bir alan adının kurulum yanıtındaki posta sunucusu adı, sitesinin yanıtından
// bağımsız olarak posta programlarına söylenen addır.
func TestMailSetupTellsTheServedMailNameWhateverTheSetupReason(t *testing.T) {
	stubMailSetupReads(t)
	f := newAutoconfigFixture(t, true)
	ctx := context.Background()
	exec := func(stmt string) {
		t.Helper()
		if _, err := f.p.db.GetDB().Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}

	// An ordinary PHP domain, a subdomain and a domain without a site: the
	// server's own mail name, and the window has nothing to warn about.
	expectMailSetupHost(t, f.p, 9801, "mail.server.example", "", true)
	expectMailSetupHost(t, f.p, 9802, "mail.server.example", mailAutoconfigReasonSubdomain, true)
	expectMailSetupHost(t, f.p, 9803, "mail.server.example", mailAutoconfigReasonNoSite, true)

	// The server's mail name not set: the stand-in, unconfirmed.
	saved := readMailTLSHostname
	t.Cleanup(func() { readMailTLSHostname = saved })
	readMailTLSHostname = func() (string, error) { return "frankfurt", nil }
	exec(`DELETE FROM panel_settings WHERE key = 'mail_hostname'`)
	expectMailSetupHost(t, f.p, 9801, "mail.example.com", mailAutoconfigReasonMailHost, false)
	expectMailSetupHost(t, f.p, 9802, "mail.shop.example.com", mailAutoconfigReasonSubdomain, false)
	if err := f.p.setSetting(ctx, settingMailHostname, "mail.server.example"); err != nil {
		t.Fatal(err)
	}

	// A set-up name held by another domain's alias: still the server's mail name.
	exec(`INSERT INTO domain_aliases (domain_id, alias) VALUES (9803, 'autodiscover.example.com')`)
	expectMailSetupHost(t, f.p, 9801, "mail.server.example", mailAutoconfigReasonNameTaken, true)

	// mail.<domain> chosen and served with the domain's certificate: that name.
	exec(`INSERT INTO ssl_certificates (domain_id, type, cert_path, key_path, expires_at, status, secure_mail) VALUES (9801, 'letsencrypt', '/c', '/k', '2099-01-01T00:00:00Z', 'active', 1)`)
	exec(`UPDATE domains SET mail_clients_use_own_name = 1 WHERE id = 9801`)
	expectMailSetupHost(t, f.p, 9801, "mail.example.com", mailAutoconfigReasonNameTaken, true)
	// Chosen but no longer served: the server's mail name again.
	exec(`UPDATE ssl_certificates SET secure_mail = 0 WHERE domain_id = 9801`)
	expectMailSetupHost(t, f.p, 9801, "mail.server.example", mailAutoconfigReasonNameTaken, true)
}

func TestMailSetupWithoutMailHereKeepsTheUnconfirmedStandIn(t *testing.T) {
	stubMailSetupReads(t)
	f := newAutoconfigFixture(t, false)
	expectMailSetupHost(t, f.p, 9801, "mail.example.com", mailAutoconfigReasonMailNotHere, false)
	// A subdomain keeps its own reason; no name is confirmed for it either.
	expectMailSetupHost(t, f.p, 9802, "mail.shop.example.com", mailAutoconfigReasonSubdomain, false)
}

// stubMailSetupReads keeps the setup answer's other reads on this machine:
// the set-up probes answer "not served" and the certificate overview's Agent
// reads fail, as without an Agent.
func stubMailSetupReads(t *testing.T) {
	t.Helper()
	savedHTTP, savedTLS := mailAutoconfigProbeHTTP, mailAutoconfigProbeTLS
	savedDefault, savedMail := readDefaultHTTPSForOverview, readMailHostCertificateForOverview
	t.Cleanup(func() {
		mailAutoconfigProbeHTTP, mailAutoconfigProbeTLS = savedHTTP, savedTLS
		readDefaultHTTPSForOverview, readMailHostCertificateForOverview = savedDefault, savedMail
	})
	mailAutoconfigProbeHTTP = func(context.Context, string) (bool, error) { return false, nil }
	mailAutoconfigProbeTLS = func(context.Context, string) (bool, error) { return false, nil }
	readDefaultHTTPSForOverview = func(*Panel, context.Context) (core.NginxDefaultHTTPS, error) {
		return core.NginxDefaultHTTPS{}, errors.New("no Agent in this test")
	}
	readMailHostCertificateForOverview = func(*Panel, context.Context, string) (transport.MailHostCertificateStatusResponse, error) {
		return transport.MailHostCertificateStatusResponse{}, errors.New("no Agent in this test")
	}
}

// expectMailSetupHost reads GET /api/v1/domains/{id}/mail/setup and checks
// that all three protocols name host, with the set-up reason, and that
// autoconfig.mail_host is host exactly when the name is confirmed.
func expectMailSetupHost(t *testing.T, p *Panel, domainID int, host, reason string, confirmed bool) {
	t.Helper()
	recorder := httptest.NewRecorder()
	p.handleDomainMail(recorder, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/domains/%d/mail/setup", domainID), nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("domain %d: status %d: %s", domainID, recorder.Code, recorder.Body)
	}
	var answer mailClientSetupResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}
	if answer.MailHost != host || answer.IMAP.Host != host || answer.POP3.Host != host || answer.SMTP.Host != host {
		t.Fatalf("domain %d: hosts %q %q %q %q, want %q", domainID, answer.MailHost, answer.IMAP.Host, answer.POP3.Host, answer.SMTP.Host, host)
	}
	if answer.Autoconfig == nil || answer.Autoconfig.Reason != reason {
		t.Fatalf("domain %d: autoconfig %+v, want reason %q", domainID, answer.Autoconfig, reason)
	}
	want := ""
	if confirmed {
		want = host
	}
	if answer.Autoconfig.MailHost != want {
		t.Fatalf("domain %d: autoconfig.mail_host %q, want %q", domainID, answer.Autoconfig.MailHost, want)
	}
}
