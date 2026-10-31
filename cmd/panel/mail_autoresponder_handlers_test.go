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

type autoresponderPanelAgent struct {
	verifiedAPTAgentRPCFixture

	mu       sync.Mutex
	answer   transport.MailAutoresponderResponse
	setCalls []transport.SetMailAutoresponderRequest
}

func (a *autoresponderPanelAgent) SetMailAutoresponder(request *transport.SetMailAutoresponderRequest, response *transport.MailAutoresponderResponse) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.setCalls = append(a.setCalls, *request)
	*response = a.answer
	return nil
}

func (a *autoresponderPanelAgent) MailAutoresponderState(_ *transport.MailAutoresponderStateRequest, response *transport.MailAutoresponderResponse) error {
	*response = transport.MailAutoresponderResponse{Script: transport.AutoresponderScriptNone, SieveAvailable: true}
	return nil
}

func (a *autoresponderPanelAgent) calls() []transport.SetMailAutoresponderRequest {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]transport.SetMailAutoresponderRequest(nil), a.setCalls...)
}

func newAutoresponderPanel(t *testing.T, agent *autoresponderPanelAgent) (*Panel, int) {
	t.Helper()
	database, err := openTemplatePanelDatabase(filepath.Join(t.TempDir(), "panel.sqlite"))
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(database.Close)
	if _, err := database.GetDB().Exec(`
		INSERT INTO users (id, username, password_hash, email, role)
		VALUES (6101, 'mail-admin', 'x', 'mail-admin@example.test', 'admin');
		INSERT INTO subscriptions (id, owner_id, name, max_email_accounts, status)
		VALUES (6102, 6101, 'Mail subscription', 50, 'active');
		INSERT INTO domains (id, subscription_id, name, status)
		VALUES (6103, 6102, 'example.com', 'active'), (6104, 6102, 'other.test', 'active');
		INSERT INTO email_accounts (id, domain_id, address, password_hash, quota_mb)
		VALUES (6201, 6103, 'me@example.com', 'x', 100), (6202, 6104, 'you@other.test', 'x', 100);
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
	return &Panel{
		db:           database,
		pkgFamilyVal: "apt",
		agentClient:  transport.NewReconnectingClientWithContextConnector(client, connector),
	}, 6103
}

func putAutoresponder(panel *Panel, domainID int, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPut, "/api/v1/domains/6103/mail/autoresponder", strings.NewReader(body))
	request = request.WithContext(context.WithValue(request.Context(), callerKey, &Caller{
		ID: 6101, Role: roleAdmin, AccountType: core.AccountTypeAccount,
	}))
	response := httptest.NewRecorder()
	panel.handleMailAutoresponder(response, request, domainID)
	return response
}

func TestMailAutoresponderSaveStoresOnlyWhatTheAgentApplied(t *testing.T) {
	agent := &autoresponderPanelAgent{answer: transport.MailAutoresponderResponse{Applied: true, Script: transport.AutoresponderScriptManaged, SieveAvailable: true}}
	panel, domainID := newAutoresponderPanel(t, agent)
	previous := buildCommit
	buildCommit = "autoresponder-test"
	t.Cleanup(func() { buildCommit = previous })

	response := putAutoresponder(panel, domainID, `{"id":6201,"enabled":true,"subject":"Away","message":"Back soon","start_date":"2026-11-01","end_date":"2026-11-09","interval_days":3,"addresses":["Info@example.com"]}`)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	calls := agent.calls()
	if len(calls) != 1 || calls[0].Email != "me@example.com" || calls[0].ExpectedBuildCommit != "autoresponder-test" ||
		calls[0].Autoresponder.Addresses[0] != "info@example.com" || calls[0].Autoresponder.IntervalDays != 3 {
		t.Fatalf("agent calls: %+v", calls)
	}
	var enabled, days int
	var addresses, start string
	if err := panel.db.GetDB().QueryRow(`SELECT enabled, interval_days, addresses, start_date FROM email_autoresponders WHERE account_id = 6201`).
		Scan(&enabled, &days, &addresses, &start); err != nil {
		t.Fatal(err)
	}
	if enabled != 1 || days != 3 || addresses != `["info@example.com"]` || start != "2026-11-01" {
		t.Fatalf("stored %d %d %s %s", enabled, days, addresses, start)
	}

	// GET returns the stored setting and the Agent's reading.
	request := httptest.NewRequest(http.MethodGet, "/api/v1/domains/6103/mail/autoresponder?id=6201", nil)
	got := httptest.NewRecorder()
	panel.handleMailAutoresponder(got, request, domainID)
	var view mailAutoresponderView
	if err := json.Unmarshal(got.Body.Bytes(), &view); err != nil || !view.Stored || !view.Setting.Enabled ||
		view.State == nil || view.State.Script != transport.AutoresponderScriptNone {
		t.Fatalf("view %+v %v (%s)", view, err, got.Body.String())
	}
}

func TestMailAutoresponderRefusalIsCodedAndStoresNothing(t *testing.T) {
	agent := &autoresponderPanelAgent{answer: transport.MailAutoresponderResponse{
		Refusal: transport.AutoresponderRefusedOwnerScript, Detail: "owner script", Script: transport.AutoresponderScriptOwner,
	}}
	panel, domainID := newAutoresponderPanel(t, agent)
	response := putAutoresponder(panel, domainID, `{"id":6201,"enabled":true,"subject":"Away","message":"x"}`)
	if response.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var body apiErrorBody
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body.Code != errCodeMailAutoresponderRefused || body.Reason != transport.AutoresponderRefusedOwnerScript {
		t.Fatalf("body %+v %v", body, err)
	}
	var rows int
	if err := panel.db.GetDB().QueryRow(`SELECT COUNT(*) FROM email_autoresponders`).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("rows=%d %v", rows, err)
	}
}

func TestMailAutoresponderRejectsBadInputAndOtherDomainsBeforeTheAgent(t *testing.T) {
	agent := &autoresponderPanelAgent{answer: transport.MailAutoresponderResponse{Applied: true}}
	panel, domainID := newAutoresponderPanel(t, agent)
	for body, want := range map[string]int{
		`{"id":6202,"enabled":true,"subject":"a","message":"b"}`:                         http.StatusNotFound,
		`{"id":6201,"enabled":true,"subject":"a","message":""}`:                          http.StatusBadRequest,
		`{"id":6201,"enabled":true,"subject":"a\nBcc: x@y","message":"b"}`:               http.StatusBadRequest,
		`{"id":6201,"enabled":true,"subject":"a","message":"b","interval_days":99}`:      http.StatusBadRequest,
		`{"id":6201,"enabled":true,"subject":"a","message":"b","end_date":"2026-13-40"}`: http.StatusBadRequest,
		`{"id":6201,"enabled":true,"subject":"a","message":"b","unknown":1}`:             http.StatusBadRequest,
	} {
		if response := putAutoresponder(panel, domainID, body); response.Code != want {
			t.Errorf("%s: status=%d want %d (%s)", body, response.Code, want, response.Body.String())
		}
	}
	if calls := agent.calls(); len(calls) != 0 {
		t.Fatalf("the Agent was asked: %+v", calls)
	}
}

func TestMailAutoresponderRouteIsAMailSubroute(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		request := httptest.NewRequest(method, "/api/v1/domains/6103/mail/autoresponder", nil)
		match, ok := matchDomainSubroute(request)
		if !ok || match.kind != "mail" || match.domainID != 6103 || !routeAllows(method, match.methods) {
			t.Fatalf("%s: %+v %v", method, match, ok)
		}
	}
	request := httptest.NewRequest(http.MethodDelete, "/api/v1/domains/6103/mail/autoresponder", nil)
	if match, ok := matchDomainSubroute(request); ok && routeAllows(http.MethodDelete, match.methods) {
		t.Fatal("DELETE is allowed")
	}
}
