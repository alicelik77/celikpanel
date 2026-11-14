package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/rpc"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/hostingpath"
	"github.com/alicelik/celikpanel/internal/transport"
)

// mail.<domain> in the certificate by default, served by mail for that name
// only (docs/CERTIFICATE-COVERAGE.md §3, §4).

type coverMailAgent struct {
	*mailTLSIsolationRPCAgent

	issueMu      sync.Mutex
	domainID     int
	issued       []string
	issueAliases [][]string
	issueError   string
	vhostWrites  int
}

func (a *coverMailAgent) IssueLetsEncryptCertificate(req *IncludeMailIssueRequest, resp *IncludeMailIssueResponse) error {
	a.issueMu.Lock()
	defer a.issueMu.Unlock()
	a.issueAliases = append(a.issueAliases, append([]string(nil), req.Aliases...))
	if a.issueError != "" {
		resp.Error = a.issueError
		return nil
	}
	a.issued = append([]string{req.Domain}, req.Aliases...)
	lineage := fmt.Sprintf("celikpanel-site-%d-%s", a.domainID, strings.Repeat("ab", 12))
	fullchain, privkey, chain := transport.NativeSiteLineagePaths(lineage)
	*resp = IncludeMailIssueResponse{
		Success: true, CertPath: fullchain, KeyPath: privkey, ChainPath: chain,
		ExpiresAt: time.Now().UTC().Add(90 * 24 * time.Hour),
		DNSNames:  append([]string(nil), a.issued...), LineageName: lineage,
	}
	return nil
}

func (a *coverMailAgent) InspectInstalledCertificate(_ *MailTLSInspectRPCRequest, resp *MailTLSInspectRPCResponse) error {
	a.issueMu.Lock()
	defer a.issueMu.Unlock()
	*resp = MailTLSInspectRPCResponse{Valid: true, Trusted: true, TrustChecked: true, DNSNames: append([]string(nil), a.issued...)}
	return nil
}

func (a *coverMailAgent) ApplyVhost(_ *IncludeMailApplyVhostRequest, _ *IncludeMailApplyVhostResponse) error {
	a.issueMu.Lock()
	defer a.issueMu.Unlock()
	a.vhostWrites++
	return nil
}

func (a *coverMailAgent) siteWrites() int {
	a.issueMu.Lock()
	defer a.issueMu.Unlock()
	return a.vhostWrites
}

func newCoverMailFixture(t *testing.T, mailNameState string) (*Panel, int, *coverMailAgent) {
	t.Helper()
	p, subscriptionID := newMailTLSIsolationFixture(t)
	db := p.db.GetDB()
	result, err := db.Exec(`INSERT INTO domains (subscription_id, name, status) VALUES (?, 'example.test', 'active')`, subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	domainID64, _ := result.LastInsertId()
	domainID := int(domainID64)
	root, err := hostingpath.DocumentRoot(int(subscriptionID), domainID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO sites (domain_id, document_root, project_type, ssl_enabled, ssl_type)
		VALUES (?, ?, 'static', false, 'none')`, domainID, root); err != nil {
		t.Fatal(err)
	}
	agent := &coverMailAgent{
		mailTLSIsolationRPCAgent: &mailTLSIsolationRPCAgent{serviceOperationTestAgent: newServiceOperationTestAgent()},
		domainID:                 domainID,
	}
	agent.terminalizeSync = true
	attachCoverMailAgent(t, p, agent)
	savedHere, savedState := mailAutoconfigMailHere, defaultMailNameState
	t.Cleanup(func() { mailAutoconfigMailHere, defaultMailNameState = savedHere, savedState })
	mailAutoconfigMailHere = func(*Panel, context.Context) bool { return true }
	defaultMailNameState = func(context.Context, string) string { return mailNameState }
	return p, domainID, agent
}

// attachCoverMailAgent is attachMailTLSIsolationAgent with this test's
// outer fake registered, so its issue and inspect methods answer.
func attachCoverMailAgent(t *testing.T, p *Panel, agent *coverMailAgent) {
	t.Helper()
	ensureActiveDNSEngineForTest(t, p, transport.DNSEnginePowerDNS)
	p.pkgFamilyVal = "apt"
	previousHostname := readMailTLSHostname
	readMailTLSHostname = func() (string, error) { return "MAIL.PANEL.TEST.", nil }
	server := rpc.NewServer()
	if err := server.RegisterName("Agent", agent); err != nil {
		t.Fatalf("register cover-mail agent: %v", err)
	}
	connector := func(ctx context.Context) (*rpc.Client, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		serverConn, clientConn := net.Pipe()
		go server.ServeConn(serverConn)
		return rpc.NewClient(clientConn), nil
	}
	client, err := connector(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	p.agentClient = transport.NewReconnectingClientWithContextConnector(client, connector)
	t.Cleanup(func() {
		readMailTLSHostname = previousHostname
		_ = client.Close()
	})
}

func issueCertificateForTest(t *testing.T, p *Panel, domainID int, body string) (int, map[string]any) {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/domains/%d/ssl/letsencrypt", domainID), strings.NewReader(body))
	recorder := httptest.NewRecorder()
	p.handleIssueLetsEncrypt(recorder, request)
	answer := map[string]any{}
	_ = json.Unmarshal(recorder.Body.Bytes(), &answer)
	return recorder.Code, answer
}

func secureMailOf(t *testing.T, p *Panel, domainID int) (int, int) {
	t.Helper()
	var active, secure int
	if err := p.db.GetDB().QueryRow(`SELECT COUNT(*), COALESCE(MAX(secure_mail), 0) FROM ssl_certificates WHERE domain_id = ? AND status = 'active'`, domainID).
		Scan(&active, &secure); err != nil {
		t.Fatal(err)
	}
	return active, secure
}

func TestCertificateRequestIncludesTheMailNameByDefaultAndMailServesIt(t *testing.T) {
	p, domainID, agent := newCoverMailFixture(t, certificateNameHere)
	code, answer := issueCertificateForTest(t, p, domainID, `{"email":"admin@example.test","auto_renew":true}`)
	if code != http.StatusOK {
		t.Fatalf("issue = %d %v", code, answer)
	}
	if got := agent.issueAliases[0]; !equalStringSlices(got, []string{"www.example.test", "mail.example.test"}) {
		t.Fatalf("names asked for = %v", got)
	}
	if answer["mail_name"] != "mail.example.test" || answer["mail_served"] != true || answer["mail_note"] != nil {
		t.Fatalf("answer = %v", answer)
	}
	if _, secure := secureMailOf(t, p, domainID); secure != 1 {
		t.Fatal("mail does not serve the certificate for mail.example.test")
	}
	agent.mu.Lock()
	last := agent.syncCalls[len(agent.syncCalls)-1]
	agent.mu.Unlock()
	// The server's own mail name keeps the default certificate; only
	// mail.<domain> (and the domain) get the site's certbot live files.
	if last.Myhostname != "mail.panel.test" || len(last.SNI) != 1 ||
		strings.Join(last.SNI[0].Names, ",") != "example.test,mail.example.test" ||
		!strings.HasPrefix(last.SNI[0].CertPath, "/etc/letsencrypt/live/celikpanel-site-") {
		t.Fatalf("mail TLS snapshot = %+v", last)
	}
	// What programs are told does not change.
	if told, err := p.mailClientHost(context.Background(), domainID, "example.test", "mail.panel.test"); err != nil || told != "mail.panel.test" {
		t.Fatalf("told = %q %v", told, err)
	}
}

func TestCertificateRequestLeavesTheMailNameOutWhenAskedOrNotHere(t *testing.T) {
	for _, tc := range []struct {
		name, state, body string
	}{
		{"left out by the person", certificateNameHere, `{"email":"admin@example.test","auto_renew":true,"exclude_mail":true}`},
		{"points elsewhere", certificateNameElsewhere, `{"email":"admin@example.test","auto_renew":true}`},
		{"not known", certificateNameUnknown, `{"email":"admin@example.test","auto_renew":true}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, domainID, agent := newCoverMailFixture(t, tc.state)
			code, answer := issueCertificateForTest(t, p, domainID, tc.body)
			if code != http.StatusOK {
				t.Fatalf("issue = %d %v", code, answer)
			}
			if got := agent.issueAliases[0]; !equalStringSlices(got, []string{"www.example.test"}) {
				t.Fatalf("names asked for = %v", got)
			}
			if _, secure := secureMailOf(t, p, domainID); secure != 0 || answer["mail_served"] != false {
				t.Fatalf("mail serves a certificate without the name: %v", answer)
			}
		})
	}
	t.Run("mail is not on this server", func(t *testing.T) {
		p, domainID, agent := newCoverMailFixture(t, certificateNameHere)
		mailAutoconfigMailHere = func(*Panel, context.Context) bool { return false }
		if code, answer := issueCertificateForTest(t, p, domainID, `{"email":"admin@example.test","auto_renew":true}`); code != http.StatusOK {
			t.Fatalf("issue = %d %v", code, answer)
		}
		if got := agent.issueAliases[0]; !equalStringSlices(got, []string{"www.example.test"}) {
			t.Fatalf("names asked for = %v", got)
		}
	})
}

// Mail says a known no to serving the new name: mail goes back to how it was,
// the certificate stays, and the answer says so (no half state, no "retry"
// state for something the person did not ask for).
func TestCertificateRequestKeepsMailAsItWasWhenMailCannotServeTheName(t *testing.T) {
	p, domainID, agent := newCoverMailFixture(t, certificateNameHere)
	agent.mu.Lock()
	agent.secureMailErrorOnce = "postfix rejected the SNI map"
	agent.mu.Unlock()
	code, answer := issueCertificateForTest(t, p, domainID, `{"email":"admin@example.test","auto_renew":true}`)
	if code != http.StatusOK || answer["mail_note"] != certificateMailNoteNotSwitched ||
		answer["mail_served"] != false || answer["mail_name"] != "mail.example.test" {
		t.Fatalf("issue = %d %v", code, answer)
	}
	active, secure := secureMailOf(t, p, domainID)
	if active != 1 || secure != 0 {
		t.Fatalf("after a refused mail switch: active=%d secure=%d", active, secure)
	}
	var renewal string
	if err := p.db.GetDB().QueryRow(`SELECT COALESCE(renewal_status, '') FROM ssl_certificates WHERE domain_id = ? AND status = 'active'`, domainID).Scan(&renewal); err != nil {
		t.Fatal(err)
	}
	if renewal == sslPendingDependents {
		t.Fatal("the certificate was left waiting for a mail step the person did not ask for")
	}
	agent.mu.Lock()
	last := agent.syncCalls[len(agent.syncCalls)-1]
	agent.mu.Unlock()
	if len(last.SNI) != 0 {
		t.Fatalf("the last published mail snapshot still names the domain: %+v", last.SNI)
	}
}

// "Posta adını da kapsa" on an existing certificate without the name: one
// request again. If the authority refuses, the old certificate and mail stay
// exactly as they were.
func TestCoverMailNameRequestAgainFailureChangesNothing(t *testing.T) {
	p, domainID, agent := newCoverMailFixture(t, certificateNameHere)
	if code, answer := issueCertificateForTest(t, p, domainID, `{"email":"admin@example.test","auto_renew":true,"exclude_mail":true}`); code != http.StatusOK {
		t.Fatalf("first issue = %d %v", code, answer)
	}
	var before string
	if err := p.db.GetDB().QueryRow(`SELECT id || cert_path || secure_mail FROM ssl_certificates WHERE domain_id = ? AND status = 'active'`, domainID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	agent.issueMu.Lock()
	agent.issueError = "urn:ietf:params:acme:error:unauthorized: mail.example.test: invalid response"
	agent.issueMu.Unlock()
	code, _ := issueCertificateForTest(t, p, domainID, `{"email":"admin@example.test","auto_renew":true,"reissue":true}`)
	if code == http.StatusOK {
		t.Fatal("a refused request answered success")
	}
	if got := agent.issueAliases[1]; !equalStringSlices(got, []string{"www.example.test", "mail.example.test"}) {
		t.Fatalf("names asked for again = %v", got)
	}
	var after string
	if err := p.db.GetDB().QueryRow(`SELECT id || cert_path || secure_mail FROM ssl_certificates WHERE domain_id = ? AND status = 'active'`, domainID).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("certificate changed by a refused request: %q → %q", before, after)
	}
	// The same press that succeeds: the name is in and mail serves it.
	agent.issueMu.Lock()
	agent.issueError = ""
	agent.issueMu.Unlock()
	if code, answer := issueCertificateForTest(t, p, domainID, `{"email":"admin@example.test","auto_renew":true,"reissue":true}`); code != http.StatusOK || answer["mail_served"] != true {
		t.Fatalf("request again = %d %v", code, answer)
	}
	if _, secure := secureMailOf(t, p, domainID); secure != 1 {
		t.Fatal("mail does not serve the renewed certificate")
	}
}

// The switch "secure mail with this certificate" means serving since
// alpha.134. For a domain that chose its own mail name before, it also
// changes what programs are told, so the site file (the auto-setup answers)
// is written again in the same operation; for any other domain it is not.
func TestMailSwitchRewritesTheAutoSetupAnswersOnlyWhenWhatProgramsAreToldChanges(t *testing.T) {
	p, domainID, agent := newCoverMailFixture(t, certificateNameHere)
	if code, answer := issueCertificateForTest(t, p, domainID, `{"email":"admin@example.test","auto_renew":true}`); code != http.StatusOK || answer["mail_served"] != true {
		t.Fatalf("issue = %d %v", code, answer)
	}
	toggle := func(on bool) int {
		t.Helper()
		request := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/domains/%d/ssl/mail", domainID), strings.NewReader(fmt.Sprintf(`{"secure_mail":%t}`, on)))
		recorder := httptest.NewRecorder()
		p.handleDomainSSLMail(recorder, request, domainID)
		return recorder.Code
	}
	before := agent.siteWrites()
	if code := toggle(false); code != http.StatusOK {
		t.Fatalf("off = %d", code)
	}
	if agent.siteWrites() != before {
		t.Fatal("the site file was written although what programs are told did not change")
	}
	if code := toggle(true); code != http.StatusOK {
		t.Fatalf("on = %d", code)
	}
	if _, err := p.db.GetDB().Exec(`UPDATE domains SET mail_clients_use_own_name = 1 WHERE id = ?`, domainID); err != nil {
		t.Fatal(err)
	}
	before = agent.siteWrites()
	if code := toggle(false); code != http.StatusOK {
		t.Fatalf("off for a domain of the old choice = %d", code)
	}
	if agent.siteWrites() != before+1 {
		t.Fatalf("site writes = %d, want %d", agent.siteWrites(), before+1)
	}
	if told, _ := p.mailClientHost(context.Background(), domainID, "example.test", "mail.panel.test"); told != "mail.panel.test" {
		t.Fatalf("told after serving stopped = %q", told)
	}
}

// Renewal keeps the names a certificate covers: a certificate the panel still
// renews itself (an earlier release's) that carries mail.<domain> validates
// the name again and is accepted only when the renewed certificate still
// carries it. A renewed certificate without it is a failed renewal and the
// certificate in use stays. (Certificates of this release are renewed by
// certbot's own lineage, which keeps its names without the panel.)
func TestARenewalKeepsTheMailNameTheCertificateCovers(t *testing.T) {
	for _, tc := range []struct {
		name    string
		drops   bool
		renewed bool
	}{
		{name: "the renewed certificate keeps the mail name", renewed: true},
		{name: "a renewed certificate without it is refused", drops: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, domainID, certificateID, agent := newValidationRestoreRenewalFixture(t, "exact")
			agent.mailInCertificate, agent.renewedDropsMail = true, tc.drops
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			p.renewLetsEncrypt(ctx, certificateID, domainID, "ssl-state.example")
			calls := validationRestoreApplyCalls(agent)
			if len(calls) == 0 || !equalStringSlices(calls[0].ACMEChallengeNames, []string{"mail.ssl-state.example"}) {
				t.Fatalf("the renewal did not validate the mail name: %+v", calls)
			}
			var activeID int
			var status string
			if err := p.db.GetDB().QueryRow(`
				SELECT id, COALESCE(renewal_status, '') FROM ssl_certificates
				WHERE domain_id = ? AND status = 'active'`, domainID).Scan(&activeID, &status); err != nil {
				t.Fatal(err)
			}
			if tc.renewed && activeID == certificateID {
				t.Fatalf("the renewed certificate was not taken (status %q)", status)
			}
			if !tc.renewed && (activeID != certificateID || status != "failed") {
				t.Fatalf("a renewal that lost the mail name: active %d (was %d), status %q", activeID, certificateID, status)
			}
		})
	}
}
