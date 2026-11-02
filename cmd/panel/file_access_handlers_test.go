package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/rpc"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

type fileAccessPanelAgent struct {
	verifiedAPTAgentRPCFixture

	mu           sync.Mutex
	capabilities []string
	enabled      bool
	answer       transport.ApplyFileAccessResponse
	applies      []transport.ApplyFileAccessRequest
	previews     []transport.FileAccessPreviewRequest
}

func (a *fileAccessPanelAgent) Version(_ *transport.Empty, response *transport.AgentVersionResponse) error {
	response.Capabilities = append([]string(nil), a.capabilities...)
	return nil
}

func (a *fileAccessPanelAgent) FileAccessStateV1(request *transport.FileAccessStateRequest, response *transport.FileAccessStateResponse) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	response.Account = transport.FileAccessAccount{Username: request.Site.Username, Exists: true, HomeMatches: true, Enabled: a.enabled}
	response.Server = transport.FileAccessServer{SSHDPresent: true, SSHPorts: []int{2222}, FTPCertificate: transport.FileAccessCertMailHost, FTPCertificateName: "mail.example.com"}
	return nil
}

func (a *fileAccessPanelAgent) FileAccessPreviewV1(request *transport.FileAccessPreviewRequest, response *transport.ApplyFileAccessResponse) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.previews = append(a.previews, *request)
	*response = transport.ApplyFileAccessResponse{Changes: []transport.FileAccessChange{{Kind: "file", Target: "x", Action: "create"}}}
	return nil
}

func (a *fileAccessPanelAgent) ApplyFileAccessV1(request *transport.ApplyFileAccessRequest, response *transport.ApplyFileAccessResponse) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.applies = append(a.applies, *request)
	*response = a.answer
	return nil
}

func (a *fileAccessPanelAgent) applied() []transport.ApplyFileAccessRequest {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]transport.ApplyFileAccessRequest(nil), a.applies...)
}

func newFileAccessPanel(t *testing.T, agent *fileAccessPanelAgent) *Panel {
	t.Helper()
	database, err := openTemplatePanelDatabase(filepath.Join(t.TempDir(), "panel.sqlite"))
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(database.Close)
	if _, err := database.GetDB().Exec(`
		INSERT INTO users (id, username, password_hash, email, role)
		VALUES (7101, 'files-admin', 'x', 'files-admin@example.test', 'admin');
		INSERT INTO subscriptions (id, owner_id, name, status)
		VALUES (7102, 7101, 'Files', 'active'), (7110, 7101, 'Suspended', 'suspended');
		INSERT INTO domains (id, subscription_id, name, status) VALUES
			(7103, 7102, 'example.com', 'active'),
			(7104, 7102, 'shop.example.com', 'active'),
			(7105, 7102, 'dns-only.test', 'active'),
			(7111, 7110, 'paused.test', 'active');
		INSERT INTO sites (id, domain_id, document_root, project_type) VALUES
			(7201, 7103, '/var/www/celikpanel/subscriptions/7102/sites/7103/public_html', 'php'),
			(7202, 7104, '/var/www/celikpanel/subscriptions/7102/sites/7104/public_html', 'static'),
			(7203, 7105, '', 'dnsonly'),
			(7211, 7111, '/var/www/celikpanel/subscriptions/7110/sites/7111/public_html', 'php');
	`); err != nil {
		t.Fatalf("seed fixture: %v", err)
	}
	server := rpc.NewServer()
	if err := server.RegisterName("Agent", agent); err != nil {
		t.Fatal(err)
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
	t.Cleanup(func() { _ = client.Close() })
	return &Panel{db: database, pkgFamilyVal: "apt", agentClient: transport.NewReconnectingClientWithContextConnector(client, connector)}
}

func fileAccessRequest(panel *Panel, method string, domainID int, preview bool, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, "/api/v1/domains/x/access", strings.NewReader(body))
	request = request.WithContext(context.WithValue(request.Context(), callerKey, &Caller{ID: 7101, Role: roleAdmin, AccountType: core.AccountTypeAccount}))
	response := httptest.NewRecorder()
	panel.handleDomainFileAccess(response, request, domainID, preview)
	return response
}

func decodeCode(t *testing.T, response *httptest.ResponseRecorder) apiErrorBody {
	t.Helper()
	var body apiErrorBody
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", response.Body.String(), err)
	}
	return body
}

func TestFileAccessEnableSendsTheSiteAccountAndReconcilesTheSubscription(t *testing.T) {
	agent := &fileAccessPanelAgent{
		capabilities: []string{transport.AgentCapabilityFileAccessV1},
		answer:       transport.ApplyFileAccessResponse{Applied: true, State: transport.FileAccessStateResponse{Account: transport.FileAccessAccount{Enabled: true}}},
	}
	panel := newFileAccessPanel(t, agent)
	previous := buildCommit
	buildCommit = "file-access-test"
	t.Cleanup(func() { buildCommit = previous })

	response := fileAccessRequest(panel, http.MethodPut, 7103, false, `{"enabled":true,"ssh":true,"password":"correct horse battery"}`)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	calls := agent.applied()
	if len(calls) != 1 {
		t.Fatalf("applies %+v", calls)
	}
	call := calls[0]
	if call.ExpectedBuildCommit != "file-access-test" || call.Site.Username != "example_com" || call.Site.SubscriptionID != 7102 ||
		call.Site.DomainID != 7103 || !call.Enabled || !call.SSH || call.Password != "correct horse battery" {
		t.Fatalf("apply %+v", call)
	}
	// The subscription's other site is reconciled; the DNS-only domain has
	// no account and the other subscription is never named.
	if len(call.Reconcile) != 1 || call.Reconcile[0].Username != "shop_example_com" || call.Reconcile[0].DomainID != 7104 {
		t.Fatalf("reconcile %+v", call.Reconcile)
	}
	if strings.Contains(response.Body.String(), "correct horse") {
		t.Fatal("the password came back")
	}
	var action string
	if err := panel.db.GetDB().QueryRow(`SELECT action FROM audit_logs ORDER BY id DESC LIMIT 1`).Scan(&action); err != nil || action != "file_access.enable" {
		t.Fatalf("audit %q %v", action, err)
	}
	var details string
	_ = panel.db.GetDB().QueryRow(`SELECT COALESCE(details, '') FROM audit_logs ORDER BY id DESC LIMIT 1`).Scan(&details)
	if strings.Contains(details, "correct horse") {
		t.Fatal("the audit trail carries the password")
	}
}

func TestFileAccessGetShowsConnectionByTheCertificateName(t *testing.T) {
	agent := &fileAccessPanelAgent{capabilities: []string{transport.AgentCapabilityFileAccessV1}}
	panel := newFileAccessPanel(t, agent)
	response := fileAccessRequest(panel, http.MethodGet, 7103, false, "")
	var view fileAccessView
	if err := json.Unmarshal(response.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if !view.Available || view.State == nil || view.Connection.FTPHost != "mail.example.com" || view.Connection.SFTPHost != "example.com" ||
		view.Connection.SFTPPort != 2222 || view.Connection.StartDir != "/7103" || view.Connection.Username != "example_com" ||
		!view.Rights.FileAccess {
		t.Fatalf("view %+v (%s)", view, response.Body.String())
	}
}

func TestFileAccessRefusals(t *testing.T) {
	agent := &fileAccessPanelAgent{capabilities: []string{transport.AgentCapabilityFileAccessV1}}
	panel := newFileAccessPanel(t, agent)
	cases := []struct {
		name     string
		domainID int
		body     string
		status   int
		code     string
	}{
		{"suspended subscription", 7111, `{"enabled":true}`, http.StatusConflict, errCodeFileAccessSubscriptionInactive},
		{"dns-only domain", 7105, `{"enabled":true}`, http.StatusConflict, errCodeFileAccessUnavailable},
		{"short password", 7103, `{"enabled":true,"password":"short"}`, http.StatusBadRequest, errCodeFileAccessInvalid},
		{"bad key", 7103, `{"enabled":true,"add_keys":["not a key"]}`, http.StatusBadRequest, errCodeFileAccessInvalid},
		{"password while off", 7103, `{"enabled":false,"password":"long enough pw"}`, http.StatusBadRequest, errCodeFileAccessInvalid},
		{"unknown field", 7103, `{"enabled":true,"shell":"/bin/sh"}`, http.StatusBadRequest, errCodeFileAccessInvalid},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			response := fileAccessRequest(panel, http.MethodPut, c.domainID, false, c.body)
			if response.Code != c.status || decodeCode(t, response).Code != c.code {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
	if len(agent.applied()) != 0 {
		t.Fatalf("a refused change reached the Agent: %+v", agent.applied())
	}
	// Closing access stays possible on a suspended subscription.
	agent.answer = transport.ApplyFileAccessResponse{Applied: true}
	if response := fileAccessRequest(panel, http.MethodPut, 7111, false, `{"enabled":false}`); response.Code != http.StatusOK {
		t.Fatalf("disable on suspended: %d %s", response.Code, response.Body.String())
	}
}

func TestFileAccessOldAgentIsAnUpgradeNotARefusal(t *testing.T) {
	agent := &fileAccessPanelAgent{}
	panel := newFileAccessPanel(t, agent)
	response := fileAccessRequest(panel, http.MethodPut, 7103, false, `{"enabled":true}`)
	if response.Code != http.StatusConflict || decodeCode(t, response).Code != errCodeFileAccessAgentUpgrade {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	get := fileAccessRequest(panel, http.MethodGet, 7103, false, "")
	var view fileAccessView
	if err := json.Unmarshal(get.Body.Bytes(), &view); err != nil || view.StateError != "agent_upgrade_required" || view.State != nil {
		t.Fatalf("view %+v %v", view, err)
	}
}

func TestFileAccessAgentRefusalIsCoded(t *testing.T) {
	agent := &fileAccessPanelAgent{
		capabilities: []string{transport.AgentCapabilityFileAccessV1},
		answer:       transport.ApplyFileAccessResponse{Refusal: transport.FileAccessRefusedSSHDOwner, Detail: "kept"},
	}
	panel := newFileAccessPanel(t, agent)
	response := fileAccessRequest(panel, http.MethodPut, 7103, false, `{"enabled":true}`)
	body := decodeCode(t, response)
	if response.Code != http.StatusConflict || body.Code != errCodeFileAccessRefused || body.Reason != transport.FileAccessRefusedSSHDOwner {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestFileAccessPreviewNeverCarriesThePassword(t *testing.T) {
	agent := &fileAccessPanelAgent{capabilities: []string{transport.AgentCapabilityFileAccessV1}}
	panel := newFileAccessPanel(t, agent)
	response := fileAccessRequest(panel, http.MethodPost, 7103, true, `{"enabled":true,"password":"correct horse battery"}`)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	agent.mu.Lock()
	defer agent.mu.Unlock()
	if len(agent.previews) != 1 || agent.previews[0].Change.Password != "" || len(agent.applies) != 0 {
		t.Fatalf("previews %+v applies %+v", agent.previews, agent.applies)
	}
}

func TestFileAccessRoutesAreOwnerOnly(t *testing.T) {
	for _, path := range []string{"/api/v1/domains/5/access", "/api/v1/domains/5/access/preview"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		match, ok := matchDomainSubroute(request)
		if !ok || !strings.HasPrefix(match.kind, "file-access") {
			t.Fatalf("%s: %+v %v", path, match, ok)
		}
		for _, method := range match.methods {
			if _, mapped := teamMemberDomainRequirementFor(match.kind, method); mapped {
				t.Fatalf("%s %s is open to additional users", method, path)
			}
		}
	}
}
