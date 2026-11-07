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

type ftpLoginsPanelAgent struct {
	verifiedAPTAgentRPCFixture

	mu      sync.Mutex
	answer  transport.ApplyFTPLoginResponse
	applies []transport.ApplyFTPLoginRequest
}

func (a *ftpLoginsPanelAgent) Version(_ *transport.Empty, response *transport.AgentVersionResponse) error {
	response.Capabilities = []string{transport.AgentCapabilityFileAccessV1, transport.AgentCapabilityFTPLoginsV1}
	return nil
}

func (a *ftpLoginsPanelAgent) FTPLoginsStateV1(_ *transport.FTPLoginsStateRequest, response *transport.FTPLoginsStateResponse) error {
	*response = transport.FTPLoginsStateResponse{Logins: []transport.FTPLoginState{}, SSHDPresent: true, SSHDropIn: "managed", SSHPorts: []int{2222}}
	return nil
}

func (a *ftpLoginsPanelAgent) ApplyFTPLoginV1(request *transport.ApplyFTPLoginRequest, response *transport.ApplyFTPLoginResponse) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.applies = append(a.applies, *request)
	*response = a.answer
	return nil
}

func (a *ftpLoginsPanelAgent) applied() []transport.ApplyFTPLoginRequest {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]transport.ApplyFTPLoginRequest(nil), a.applies...)
}

func newFTPLoginsPanel(t *testing.T, agent *ftpLoginsPanelAgent) *Panel {
	t.Helper()
	database, err := openTemplatePanelDatabase(filepath.Join(t.TempDir(), "panel.sqlite"))
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(database.Close)
	if _, err := database.GetDB().Exec(`
		INSERT INTO users (id, username, password_hash, email, role)
		VALUES (7301, 'ftp-admin', 'x', 'ftp-admin@example.test', 'admin');
		INSERT INTO subscriptions (id, owner_id, name, status)
		VALUES (7302, 7301, 'Files', 'active'), (7310, 7301, 'Suspended', 'suspended');
		INSERT INTO domains (id, subscription_id, name, status) VALUES
			(7303, 7302, 'example.com', 'active'),
			(7304, 7302, 'shop.example.com', 'active'),
			(7311, 7310, 'paused.test', 'active');
		INSERT INTO sites (id, domain_id, document_root, project_type) VALUES
			(7401, 7303, '/var/www/celikpanel/subscriptions/7302/sites/7303/public_html', 'php'),
			(7402, 7304, '/var/www/celikpanel/subscriptions/7302/sites/7304/public_html', 'php'),
			(7411, 7311, '/var/www/celikpanel/subscriptions/7310/sites/7311/public_html', 'php');
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

func ftpLoginCall(panel *Panel, method, path string, domainID int, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request = request.WithContext(context.WithValue(request.Context(), callerKey, &Caller{ID: 7301, Role: roleAdmin, AccountType: core.AccountTypeAccount}))
	response := httptest.NewRecorder()
	panel.handleDomainFTPLogins(response, request, domainID)
	return response
}

// withFTPCeiling replaces the plan's count for one test.
func withFTPCeiling(t *testing.T, ceiling int) {
	t.Helper()
	previous := ftpLoginsCeiling
	ftpLoginsCeiling = func(*Panel, context.Context, int) (ftpLoginLimit, error) {
		value := ceiling
		return ftpLoginLimit{Ceiling: &value, SetBy: "plan"}, nil
	}
	t.Cleanup(func() { ftpLoginsCeiling = previous })
}

const ftpStrongPassword = "Strong-pass-1"

func TestFTPLoginAddRecordsTheRowAndAsksTheAgent(t *testing.T) {
	agent := &ftpLoginsPanelAgent{answer: transport.ApplyFTPLoginResponse{Applied: true}}
	panel := newFTPLoginsPanel(t, agent)
	withFTPCeiling(t, 2)
	previous := buildCommit
	buildCommit = "ftp-login-test"
	t.Cleanup(func() { buildCommit = previous })

	response := ftpLoginCall(panel, http.MethodPost, "/api/v1/domains/7303/access/logins", 7303,
		`{"login":"anna-uploads","folder":"public_html/uploads/","password":"`+ftpStrongPassword+`"}`)
	if response.Code != http.StatusOK {
		t.Fatalf("add: %d %s", response.Code, response.Body.String())
	}
	calls := agent.applied()
	if len(calls) != 1 {
		t.Fatalf("agent calls %d", len(calls))
	}
	call := calls[0]
	if call.Action != transport.FTPLoginAdd || call.Login != "anna-uploads" || call.Folder != "/public_html/uploads" ||
		call.Password != ftpStrongPassword || call.Site.Username != "example_com" || call.Site.SubscriptionID != 7302 ||
		call.Site.DomainID != 7303 || call.ExpectedBuildCommit != "ftp-login-test" {
		t.Fatalf("agent request %+v", call)
	}
	var folder string
	if err := panel.db.GetDB().QueryRow(`SELECT folder FROM ftp_logins WHERE login = 'anna-uploads' AND domain_id = 7303`).Scan(&folder); err != nil || folder != "/public_html/uploads" {
		t.Fatalf("row %q %v", folder, err)
	}
	var action, details string
	if err := panel.db.GetDB().QueryRow(`SELECT action, COALESCE(details, '') FROM audit_logs ORDER BY id DESC LIMIT 1`).Scan(&action, &details); err != nil || action != "ftp_login.add" {
		t.Fatalf("audit %q %v", action, err)
	}
	if strings.Contains(details, ftpStrongPassword) || strings.Contains(response.Body.String(), ftpStrongPassword) {
		t.Fatal("the password left the request")
	}

	// The same name again is refused before the Agent is asked.
	response = ftpLoginCall(panel, http.MethodPost, "/api/v1/domains/7304/access/logins", 7304,
		`{"login":"anna-uploads","folder":"/","password":"`+ftpStrongPassword+`"}`)
	if response.Code != http.StatusConflict || decodeCode(t, response).Code != errCodeFTPLoginNameTaken || len(agent.applied()) != 1 {
		t.Fatalf("duplicate name: %d %s", response.Code, response.Body.String())
	}
}

func TestFTPLoginLimitIsCountedAcrossTheSubscription(t *testing.T) {
	agent := &ftpLoginsPanelAgent{answer: transport.ApplyFTPLoginResponse{Applied: true}}
	panel := newFTPLoginsPanel(t, agent)
	withFTPCeiling(t, 1)
	if response := ftpLoginCall(panel, http.MethodPost, "/api/v1/domains/7303/access/logins", 7303,
		`{"login":"anna-uploads","folder":"/","password":"`+ftpStrongPassword+`"}`); response.Code != http.StatusOK {
		t.Fatalf("first: %d %s", response.Code, response.Body.String())
	}
	// Another site of the same subscription: the plan's count is reached.
	response := ftpLoginCall(panel, http.MethodPost, "/api/v1/domains/7304/access/logins", 7304,
		`{"login":"bob-web","folder":"/","password":"`+ftpStrongPassword+`"}`)
	if response.Code != http.StatusConflict || decodeCode(t, response).Code != errCodeFTPLoginLimit {
		t.Fatalf("over the limit: %d %s", response.Code, response.Body.String())
	}
	if len(agent.applied()) != 1 {
		t.Fatal("the Agent was asked beyond the limit")
	}
	var rows int
	_ = panel.db.GetDB().QueryRow(`SELECT COUNT(*) FROM ftp_logins`).Scan(&rows)
	if rows != 1 {
		t.Fatalf("rows %d", rows)
	}
}

func TestFTPLoginRefusalRemovesTheRowAndKeepsTheReason(t *testing.T) {
	agent := &ftpLoginsPanelAgent{answer: transport.ApplyFTPLoginResponse{Refusal: transport.FTPLoginRefusedFolder,
		Detail: "the folder /missing cannot be used: it is missing"}}
	panel := newFTPLoginsPanel(t, agent)
	withFTPCeiling(t, 5)
	response := ftpLoginCall(panel, http.MethodPost, "/api/v1/domains/7303/access/logins", 7303,
		`{"login":"anna-uploads","folder":"missing","password":"`+ftpStrongPassword+`"}`)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("refusal: %d %s", response.Code, response.Body.String())
	}
	body := decodeCode(t, response)
	if body.Code != errCodeFTPLoginRefused || body.Reason != transport.FTPLoginRefusedFolder || !strings.Contains(body.Error, "missing") {
		t.Fatalf("refusal body %+v", body)
	}
	var rows int
	_ = panel.db.GetDB().QueryRow(`SELECT COUNT(*) FROM ftp_logins`).Scan(&rows)
	if rows != 0 {
		t.Fatalf("the row outlived the refusal: %d", rows)
	}
}

func TestFTPLoginChecksBeforeAnythingChanges(t *testing.T) {
	agent := &ftpLoginsPanelAgent{answer: transport.ApplyFTPLoginResponse{Applied: true}}
	panel := newFTPLoginsPanel(t, agent)
	withFTPCeiling(t, 5)
	cases := []struct {
		name, body string
		domain     int
		code       string
	}{
		{"no hyphen", `{"login":"anna","folder":"/","password":"` + ftpStrongPassword + `"}`, 7303, errCodeFTPLoginInvalid},
		{"dot dot", `{"login":"anna-up","folder":"../x","password":"` + ftpStrongPassword + `"}`, 7303, errCodeFTPLoginInvalid},
		{"weak password", `{"login":"anna-up","folder":"/","password":"weak"}`, 7303, "PASSWORD_WEAK"},
		{"suspended", `{"login":"anna-up","folder":"/","password":"` + ftpStrongPassword + `"}`, 7311, errCodeFileAccessSubscriptionInactive},
	}
	for _, c := range cases {
		response := ftpLoginCall(panel, http.MethodPost, "/api/v1/domains/x/access/logins", c.domain, c.body)
		if response.Code < 400 || decodeCode(t, response).Code != c.code {
			t.Fatalf("%s: %d %s", c.name, response.Code, response.Body.String())
		}
	}
	if len(agent.applied()) != 0 {
		t.Fatal("the Agent was asked")
	}
	// Removing is always allowed, also in a suspended subscription.
	response := ftpLoginCall(panel, http.MethodDelete, "/api/v1/domains/7311/access/logins/anna-up", 7311, "")
	if response.Code != http.StatusOK || len(agent.applied()) != 1 || agent.applied()[0].Action != transport.FTPLoginRemove {
		t.Fatalf("remove in a suspended subscription: %d %s", response.Code, response.Body.String())
	}
}

func TestFTPLoginRoutesAndDigest(t *testing.T) {
	for path, want := range map[string]bool{
		"/api/v1/domains/7/access/logins":          true,
		"/api/v1/domains/7/access/logins/anna-up":  true,
		"/api/v1/domains/7/access/logins/anna":     false,
		"/api/v1/domains/7/access/logins/../x":     false,
		"/api/v1/domains/7/access/logins/a-b/more": false,
	} {
		method := http.MethodDelete
		if strings.HasSuffix(path, "/logins") {
			method = http.MethodPost
		}
		match, ok := matchDomainSubroute(httptest.NewRequest(method, path, nil))
		if ok != want || (ok && match.kind != "ftp-logins") {
			t.Fatalf("%s: %v %+v", path, ok, match)
		}
		if ok {
			if route, guarded := operationStepRouteFor(httptest.NewRequest(method, path, nil)); !guarded || route.digest == nil {
				t.Fatalf("%s is not guarded with a digest", path)
			}
		}
	}
	digest := string(requestIdentityFTPLoginDigest([]byte(`{"login":"anna-up","password":"` + ftpStrongPassword + `"}`)))
	var back map[string]any
	if strings.Contains(digest, ftpStrongPassword) || json.Unmarshal([]byte(digest), &back) != nil || back["login"] != "anna-up" {
		t.Fatalf("digest %q", digest)
	}
}
