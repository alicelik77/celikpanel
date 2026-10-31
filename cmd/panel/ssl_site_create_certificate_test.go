package main

import (
	"context"
	"database/sql"
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

	"github.com/alicelik/celikpanel/internal/transport"
)

// Add domain with "Enable SSL" creates the site and then sends ONE request to
// the route the SSL/TLS page uses (web/src/lib/siteCertificateRequest.ts). The
// request carries no contact address: the server uses the signed-in account's
// own, refuses with a pointer to My account when there is none, and answers
// with the same codes the SSL page already shows. These tests pin the server
// half of that flow with the real handler and a fake Agent (no certbot, no CA).

type SiteCertIssueRequest struct {
	Domain  string
	Aliases []string
	Email   string
}

type SiteCertIssueResponse struct {
	Success       bool
	CertPath      string
	KeyPath       string
	ChainPath     string
	ExpiresAt     time.Time
	LineageName   string
	Error         string
	Failure       string
	FailureDetail string
}

// siteCertAgent answers like the Agent: it records what it was asked and
// either issues or reports a classified certbot failure.
type siteCertAgent struct {
	includeMailRPCAgent

	issueMu sync.Mutex
	emails  []string
	fail    string
}

func (a *siteCertAgent) IssueLetsEncryptCertificate(
	req *SiteCertIssueRequest,
	resp *SiteCertIssueResponse,
) error {
	a.issueMu.Lock()
	a.emails = append(a.emails, req.Email)
	fail := a.fail
	a.issueMu.Unlock()
	if fail != "" {
		*resp = SiteCertIssueResponse{Error: "certbot: " + fail, Failure: fail, FailureDetail: "Timeout during connect (likely firewall problem)"}
		return nil
	}
	a.includeMailRPCAgent.mu.Lock()
	a.includeMailRPCAgent.issuedNames = append([]string{req.Domain}, req.Aliases...)
	a.includeMailRPCAgent.mu.Unlock()
	*resp = SiteCertIssueResponse{
		Success:     true,
		CertPath:    "/certificates/example/fullchain.pem",
		KeyPath:     "/certificates/example/privkey.pem",
		ChainPath:   "/certificates/example/chain.pem",
		ExpiresAt:   time.Now().UTC().Add(90 * 24 * time.Hour),
		LineageName: req.Domain,
	}
	return nil
}

// attachSiteCertAgent is attachIncludeMailAgent for an Agent of this file's type.
func attachSiteCertAgent(t *testing.T, p *Panel, agent *siteCertAgent) {
	t.Helper()
	p.pkgFamilyVal = "apt"
	server := rpc.NewServer()
	if err := server.RegisterName("Agent", agent); err != nil {
		t.Fatalf("register certificate agent: %v", err)
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
		t.Fatalf("connect certificate agent: %v", err)
	}
	p.agentClient = transport.NewReconnectingClientWithContextConnector(client, connector)
	t.Cleanup(func() { _ = client.Close() })
}

func (a *siteCertAgent) askedEmails() []string {
	a.issueMu.Lock()
	defer a.issueMu.Unlock()
	return append([]string(nil), a.emails...)
}

func siteCertRequest(domainID int, userID int, body string) *http.Request {
	request := httptest.NewRequest(
		http.MethodPost,
		fmt.Sprintf("/api/v1/domains/%d/ssl/letsencrypt", domainID),
		strings.NewReader(body),
	)
	if userID > 0 {
		request = request.WithContext(context.WithValue(
			request.Context(), callerKey, &Caller{ID: userID, Role: roleCustomer},
		))
	}
	return request
}

func siteCertUserID(t *testing.T, p *Panel) int {
	t.Helper()
	var id int
	if err := p.db.GetDB().QueryRow(`SELECT id FROM users ORDER BY id LIMIT 1`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// siteCertListedSSL is the SSL value the domain list reports for a site.
func siteCertListedSSL(t *testing.T, p *Panel, domainID int) bool {
	t.Helper()
	var php, projectType sql.NullString
	var ssl bool
	var disk, traffic int64
	if err := p.db.GetDB().QueryRow(domainSiteInfoQuery, domainID).Scan(&php, &ssl, &projectType, &disk, &traffic); err != nil {
		t.Fatal(err)
	}
	return ssl
}

func siteCertSiteState(t *testing.T, p *Panel, domainID int) (sslEnabled bool, activeCertificates int) {
	t.Helper()
	db := p.db.GetDB()
	if err := db.QueryRow(`SELECT COALESCE(ssl_enabled, 0) FROM sites WHERE domain_id = ?`, domainID).Scan(&sslEnabled); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM ssl_certificates WHERE domain_id = ? AND status = 'active'`, domainID).Scan(&activeCertificates); err != nil {
		t.Fatal(err)
	}
	return sslEnabled, activeCertificates
}

const siteCertDefaultBody = `{"auto_renew":true,"include_mail":false,"provider":"letsencrypt","reissue":false}`

func TestSiteCreateCertificateRequestUsesTheAccountEmailAndIssuesOnce(t *testing.T) {
	p, domainID := newIncludeMailFixture(t, "example.test")
	agent := &siteCertAgent{}
	attachSiteCertAgent(t, p, agent)
	userID := siteCertUserID(t, p)

	// A site created with SSL requested has no certificate and says so.
	if enabled, active := siteCertSiteState(t, p, domainID); enabled || active != 0 {
		t.Fatalf("before the request: ssl_enabled=%v active certificates=%d", enabled, active)
	}

	recorder := httptest.NewRecorder()
	p.handleIssueLetsEncrypt(recorder, siteCertRequest(domainID, userID, siteCertDefaultBody))
	if recorder.Code != http.StatusOK {
		t.Fatalf("issue status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
	var answer map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil || answer["status"] != "success" {
		t.Fatalf("issue answer = %s (%v)", recorder.Body.String(), err)
	}
	if got := agent.askedEmails(); len(got) != 1 || got[0] != "owner@example.test" {
		t.Fatalf("contact addresses sent to the certificate agent = %#v, want the account's own", got)
	}
	// The names are the SSL page's: the domain and its www name, not mail.
	agent.includeMailRPCAgent.mu.Lock()
	issued := append([]string(nil), agent.includeMailRPCAgent.issuedNames...)
	agent.includeMailRPCAgent.mu.Unlock()
	if len(issued) != 2 || issued[0] != "example.test" || issued[1] != "www.example.test" {
		t.Fatalf("names requested = %#v", issued)
	}
	// Only now does the site claim a certificate.
	if enabled, active := siteCertSiteState(t, p, domainID); !enabled || active != 1 {
		t.Fatalf("after the request: ssl_enabled=%v active certificates=%d, want true/1", enabled, active)
	}

	// The same request again (a retried click, a replay the identity guard did
	// not catch) must not ask the certificate authority a second time.
	again := httptest.NewRecorder()
	p.handleIssueLetsEncrypt(again, siteCertRequest(domainID, userID, siteCertDefaultBody))
	if again.Code != http.StatusConflict || !strings.Contains(again.Body.String(), "active certificate already exists") {
		t.Fatalf("repeat status = %d, body=%s", again.Code, again.Body.String())
	}
	if got := agent.askedEmails(); len(got) != 1 {
		t.Fatalf("certificate authority asked %d times, want 1", len(got))
	}
}

func TestSiteCreateCertificateRequestTypedEmailWins(t *testing.T) {
	p, domainID := newIncludeMailFixture(t, "example.test")
	agent := &siteCertAgent{}
	attachSiteCertAgent(t, p, agent)
	userID := siteCertUserID(t, p)

	recorder := httptest.NewRecorder()
	p.handleIssueLetsEncrypt(recorder, siteCertRequest(domainID, userID,
		`{"email":"typed@example.test","auto_renew":true}`))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
	if got := agent.askedEmails(); len(got) != 1 || got[0] != "typed@example.test" {
		t.Fatalf("contact addresses = %#v, want the typed one", got)
	}
}

func TestSiteCreateCertificateRequestWithoutAUsableContactIsRefusedBeforeAnything(t *testing.T) {
	for _, stored := range []string{"", "admin@localhost", "not an address"} {
		t.Run(fmt.Sprintf("account email %q", stored), func(t *testing.T) {
			p, domainID := newIncludeMailFixture(t, "example.test")
			agent := &siteCertAgent{}
			attachSiteCertAgent(t, p, agent)
			userID := siteCertUserID(t, p)
			if _, err := p.db.GetDB().Exec(`UPDATE users SET email = ? WHERE id = ?`, stored, userID); err != nil {
				t.Fatal(err)
			}

			recorder := httptest.NewRecorder()
			p.handleIssueLetsEncrypt(recorder, siteCertRequest(domainID, userID, siteCertDefaultBody))
			var refusal apiErrorBody
			if err := json.Unmarshal(recorder.Body.Bytes(), &refusal); err != nil {
				t.Fatal(err)
			}
			if recorder.Code != http.StatusConflict || refusal.Code != errCodeSSLContactEmailRequired ||
				refusal.Action != ownEmailAccountAction {
				t.Fatalf("answer = %d %+v", recorder.Code, refusal)
			}
			if !strings.Contains(refusal.Error, "nothing was requested") {
				t.Fatalf("the refusal does not say that nothing was requested: %q", refusal.Error)
			}
			if got := agent.askedEmails(); len(got) != 0 {
				t.Fatalf("certificate agent was asked %d times", len(got))
			}
			agent.includeMailRPCAgent.mu.Lock()
			applied := len(agent.includeMailRPCAgent.applyCalls)
			agent.includeMailRPCAgent.mu.Unlock()
			if applied != 0 {
				t.Fatalf("web server configuration was touched %d times for a refused request", applied)
			}
			if enabled, active := siteCertSiteState(t, p, domainID); enabled || active != 0 {
				t.Fatalf("refused request changed the site: ssl_enabled=%v active=%d", enabled, active)
			}
		})
	}
}

func TestSiteCreateCertificateRequestWithoutACallerIsRefused(t *testing.T) {
	p, domainID := newIncludeMailFixture(t, "example.test")
	agent := &siteCertAgent{}
	attachSiteCertAgent(t, p, agent)

	recorder := httptest.NewRecorder()
	p.handleIssueLetsEncrypt(recorder, siteCertRequest(domainID, 0, siteCertDefaultBody))
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), errCodeSSLContactEmailRequired) {
		t.Fatalf("answer = %d %s", recorder.Code, recorder.Body.String())
	}
	if got := agent.askedEmails(); len(got) != 0 {
		t.Fatalf("certificate agent was asked %d times", len(got))
	}
}

func TestSiteCreateCertificateFailureLeavesTheSiteHonestAndNamesTheCause(t *testing.T) {
	for _, kind := range []string{
		transport.CertificateFailureValidation,
		transport.CertificateFailureAuthorityUnreachable,
		transport.CertificateFailureRateLimited,
		transport.CertificateFailureTimeout,
		transport.CertificateFailureTool,
	} {
		t.Run(kind, func(t *testing.T) {
			p, domainID := newIncludeMailFixture(t, "example.test")
			agent := &siteCertAgent{fail: kind}
			attachSiteCertAgent(t, p, agent)
			userID := siteCertUserID(t, p)

			recorder := httptest.NewRecorder()
			p.handleIssueLetsEncrypt(recorder, siteCertRequest(domainID, userID, siteCertDefaultBody))
			var failure apiErrorBody
			if err := json.Unmarshal(recorder.Body.Bytes(), &failure); err != nil {
				t.Fatal(err)
			}
			if recorder.Code != http.StatusBadGateway || failure.Code != errCodeCertificateIssueFailed || failure.Reason != kind {
				t.Fatalf("answer = %d %+v", recorder.Code, failure)
			}
			if !strings.Contains(failure.Error, "Nothing asks again automatically") {
				t.Fatalf("the failure does not say how work resumes: %q", failure.Error)
			}
			// The site is untouched: no certificate row, and the flag that
			// means "a certificate is served" stays off.
			if enabled, active := siteCertSiteState(t, p, domainID); enabled || active != 0 {
				t.Fatalf("after a failed request: ssl_enabled=%v active=%d, want false/0", enabled, active)
			}
			// The failed run is one run: nothing retried it.
			if got := agent.askedEmails(); len(got) != 1 {
				t.Fatalf("certificate authority asked %d times, want 1", len(got))
			}
			// The domain list reads the same truth.
			if siteCertListedSSL(t, p, domainID) {
				t.Fatal("the domain list reports SSL for a site without a certificate")
			}
		})
	}
}

// A site row that says SSL on while no certificate is active (what Add domain
// used to leave behind) is reported as off by the list, without a migration.
func TestDomainListReportsSSLOnlyForAnActiveCertificate(t *testing.T) {
	p, domainID := newIncludeMailFixture(t, "example.test")
	db := p.db.GetDB()
	if _, err := db.Exec(`UPDATE sites SET ssl_enabled = true WHERE domain_id = ?`, domainID); err != nil {
		t.Fatal(err)
	}
	if siteCertListedSSL(t, p, domainID) {
		t.Fatal("list reports SSL on for a flag without any certificate")
	}
	if _, err := db.Exec(`
		INSERT INTO ssl_certificates (domain_id, type, cert_path, key_path, issuer, subject, issued_at, expires_at, status)
		VALUES (?, 'letsencrypt', '/c/fullchain.pem', '/c/privkey.pem', 'x', 'example.test', '2026-01-01T00:00:00Z', '2027-01-01T00:00:00Z', 'active')`,
		domainID); err != nil {
		t.Fatal(err)
	}
	if !siteCertListedSSL(t, p, domainID) {
		t.Fatal("list reports SSL off although a certificate is active")
	}
}
