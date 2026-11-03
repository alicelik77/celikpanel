package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A name behind a proxy or CDN reads as another address, yet the proxy can
// forward the challenge here. With the owner's explicit allow_not_pointed the
// not-pointed refusal is skipped (and only that one), the override is audited,
// and a failure is the authority's own answer.
func TestCertificateRequestAllowNotPointedSkipsOnlyThatRefusalAndIsAudited(t *testing.T) {
	proxied := byNameResolver{"example.test": {"198.51.100.9"}, "www.example.test": {"198.51.100.9"}}

	t.Run("without the flag it still refuses", func(t *testing.T) {
		p, domainID := newIncludeMailFixture(t, "example.test")
		agent := &siteCertAgent{}
		attachSiteCertAgent(t, p, agent)
		withCertificateDNS(t, []string{certNamesServerIP}, proxied)
		recorder := httptest.NewRecorder()
		p.handleIssueLetsEncrypt(recorder, siteCertRequest(domainID, siteCertUserID(t, p),
			`{"auto_renew":true,"allow_not_pointed":false}`))
		if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), errCodeCertificateDomainNotPointed) {
			t.Fatalf("answer = %d %s", recorder.Code, recorder.Body.String())
		}
		if got := agent.askedEmails(); len(got) != 0 {
			t.Fatalf("agent asked %d times", len(got))
		}
	})

	t.Run("with the flag the request goes ahead and the override is audited", func(t *testing.T) {
		p, domainID := newIncludeMailFixture(t, "example.test")
		agent := &siteCertAgent{}
		attachSiteCertAgent(t, p, agent)
		withCertificateDNS(t, []string{certNamesServerIP}, proxied)
		recorder := httptest.NewRecorder()
		p.handleIssueLetsEncrypt(recorder, siteCertRequest(domainID, siteCertUserID(t, p),
			`{"auto_renew":true,"allow_not_pointed":true}`))
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, body=%s", recorder.Code, recorder.Body.String())
		}
		if got := agent.askedEmails(); len(got) != 1 {
			t.Fatalf("agent asked %d times, want 1", len(got))
		}
		var details string
		if err := p.db.GetDB().QueryRow(
			`SELECT COALESCE(details, '') FROM audit_logs WHERE action = 'ssl.issue.not_pointed_allowed' AND resource_id = ?`, domainID,
		).Scan(&details); err != nil {
			t.Fatalf("the override is not audited: %v", err)
		}
		if !strings.Contains(details, "example.test") || !strings.Contains(details, "www.example.test") {
			t.Fatalf("audit details = %s", details)
		}
	})

	t.Run("with the flag a failure is the authority's own answer, not the refusal", func(t *testing.T) {
		p, domainID := newIncludeMailFixture(t, "example.test")
		agent := &siteCertAgent{fail: "validation"}
		attachSiteCertAgent(t, p, agent)
		withCertificateDNS(t, []string{certNamesServerIP}, proxied)
		recorder := httptest.NewRecorder()
		p.handleIssueLetsEncrypt(recorder, siteCertRequest(domainID, siteCertUserID(t, p),
			`{"auto_renew":true,"allow_not_pointed":true}`))
		body := recorder.Body.String()
		if strings.Contains(body, errCodeCertificateDomainNotPointed) || !strings.Contains(body, "CERTIFICATE_ISSUE_FAILED") {
			t.Fatalf("answer = %d %s", recorder.Code, body)
		}
	})

	t.Run("with the flag nothing else is skipped", func(t *testing.T) {
		p, domainID := newIncludeMailFixture(t, "example.test")
		agent := &siteCertAgent{}
		attachSiteCertAgent(t, p, agent)
		withCertificateDNS(t, []string{certNamesServerIP}, proxied)
		if _, err := p.db.GetDB().Exec(`UPDATE users SET email = '' WHERE id = ?`, siteCertUserID(t, p)); err != nil {
			t.Fatal(err)
		}
		recorder := httptest.NewRecorder()
		p.handleIssueLetsEncrypt(recorder, siteCertRequest(domainID, siteCertUserID(t, p),
			`{"auto_renew":true,"allow_not_pointed":true}`))
		if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), errCodeSSLContactEmailRequired) {
			t.Fatalf("answer = %d %s", recorder.Code, recorder.Body.String())
		}
	})
}
