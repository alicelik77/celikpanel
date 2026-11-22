package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/rpc"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.143 lab, items 2 and 6 (alpha.144): a change the person started is
// not cancelled or lost because the page was left. Each test leaves the page
// (the browser's connection ends) while the Agent works; the Agent then
// finishes. The Panel's record must say what the server did, and the change
// is never run twice. On the code before alpha.144 the handler ran on the
// connection's context: the Agent's answer was taken as lost and the record
// stayed behind (PostgreSQL: "sent, not yet confirmed"; mail: a mailbox on
// the server without a row, a reply without its copy).

// leaveDuring sends request through handler, ends its connection once
// entered is closed, then lets the Agent finish (release) and waits for the
// handler.
func leaveDuring(t *testing.T, handler http.Handler, request *http.Request, entered, release chan struct{}) {
	t.Helper()
	connection, hangUp := context.WithCancel(request.Context())
	request = request.WithContext(connection)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		handler.ServeHTTP(httptest.NewRecorder(), request)
	}()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("the Agent was never asked")
	}
	hangUp()
	// The page is gone; the Agent finishes its work after that.
	time.Sleep(50 * time.Millisecond)
	close(release)
	select {
	case <-finished:
	case <-time.After(20 * time.Second):
		t.Fatal("the handler did not end")
	}
}

// Item 2: an administrator's close of a PostgreSQL database, the page left
// while it runs.
func TestA144PGPublicCloseIsNotLostWhenThePageIsLeft(t *testing.T) {
	f := pgPublicFixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	saved := pgPublicAgent
	pgPublicAgent = func(p *Panel, ctx context.Context, method string, req, resp any) error {
		if method != "Agent.PostgreSQLPublicAccessCloseV1" {
			return f.agent.call(p, ctx, method, req, resp)
		}
		once.Do(func() { close(entered) })
		<-release
		// The Agent does the close; the Panel's call, like the real one,
		// returns the context's error when the context ended meanwhile.
		err := f.agent.call(p, ctx, method, req, resp)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
	t.Cleanup(func() { pgPublicAgent = saved })

	guard := newRequestIdentityGuard(f.panel.db.GetDB())
	mux := http.NewServeMux()
	mux.HandleFunc(pgPublicPath, f.panel.handlePostgreSQLPublicAccess)
	handler := f.panel.requireAuth(guard.wrap(mux))
	raw, _ := json.Marshal(map[string]any{"action": "close", "database_id": 9982, "acl": "", "roles": []string{"admin_user", "other_customer"}})
	request := httptest.NewRequest(http.MethodPost, pgPublicPath, bytes.NewReader(raw))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(requestIdentityHeader, requestIdentityTestID(144201))
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: f.tokens["admin"]})
	leaveDuring(t, handler, request, entered, release)

	marker, err := f.panel.readPGPublicMarker(context.Background(), 9982)
	if err != nil || marker == nil || marker.State != pgPublicClosed || marker.FailureCode == "lost" {
		t.Fatalf("the close finished on the server, but the panel's record is %+v (%v)", marker, err)
	}
	if n := f.agent.closesOf("foreign_db"); n != 1 {
		t.Fatalf("closes sent: %d", n)
	}
	// Whoever opens the page again reads the real result.
	read := pgPublicPost(t, f.visibleDatabasesFixture, "admin", nil)
	var view pgPublicView
	if err := json.Unmarshal(read.Body.Bytes(), &view); err != nil || pgPublicEntryOf(view.Closed, "foreign_db") == nil {
		t.Fatalf("the page read again: %s (%v)", read.Body.String(), err)
	}
	// The same request arriving again is answered from its record, not sent
	// again.
	again := httptest.NewRequest(http.MethodPost, pgPublicPath, bytes.NewReader(raw))
	again.Header.Set("Content-Type", "application/json")
	again.Header.Set(requestIdentityHeader, requestIdentityTestID(144201))
	again.AddCookie(&http.Cookie{Name: sessionCookieName, Value: f.tokens["admin"]})
	replay := httptest.NewRecorder()
	handler.ServeHTTP(replay, again)
	if replay.Code != http.StatusOK || replay.Header().Get(requestIdentityReplayedHeader) != "1" || f.agent.closesOf("foreign_db") != 1 {
		t.Fatalf("replay: %d %s (closes %d)", replay.Code, replay.Body.String(), f.agent.closesOf("foreign_db"))
	}
}

// a144MailAgent holds a mailbox creation until released and answers that a
// mailbox already exists for the addresses the server holds.
type a144MailAgent struct {
	mailMutationPanelAgent
	entered, release chan struct{}
	enteredOnce      sync.Once
	onServer         map[string]transport.MailboxOnServerResponse
	reads            int
}

func (a *a144MailAgent) AddMailAccount(request *transport.MailAccount, response *transport.MailMutationResponse) error {
	if a.entered != nil {
		a.enteredOnce.Do(func() { close(a.entered) })
		<-a.release
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.addCalls++
	if _, held := a.onServer[request.Email]; held {
		return errors.New(transport.MailAccountExistsOnServer)
	}
	if a.onServer == nil {
		a.onServer = map[string]transport.MailboxOnServerResponse{}
	}
	a.onServer[request.Email] = transport.MailboxOnServerResponse{InUsers: true, InMap: true, OwnFolder: true, QuotaMB: request.QuotaMB}
	response.Applied = true
	return nil
}

func (a *a144MailAgent) MailboxOnServerV1(request *transport.MailboxOnServerRequest, response *transport.MailboxOnServerResponse) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.reads++
	*response = a.onServer[request.Email]
	return nil
}

func (a *a144MailAgent) adds() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.addCalls
}

func newA144MailPanel(t *testing.T, agent *a144MailAgent) (*Panel, int) {
	t.Helper()
	panel, domainID := newMailMutationPanel(t, &agent.mailMutationPanelAgent)
	panel.agentClient = a144AgentClient(t, agent)
	if _, err := panel.db.GetDB().Exec(`
		INSERT INTO users (id, username, password_hash, email, role)
		VALUES (6111, 'mail-customer', 'x', 'mail-customer@example.test', 'customer')`); err != nil {
		t.Fatal(err)
	}
	return panel, domainID
}

func a144MailRequest(method, path, body string, caller *Caller, id string) *http.Request {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if id != "" {
		request.Header.Set(requestIdentityHeader, id)
	}
	return request.WithContext(context.WithValue(request.Context(), callerKey, caller))
}

var (
	a144Admin    = &Caller{ID: 6101, Role: roleAdmin, AccountType: core.AccountTypeAccount}
	a144Customer = &Caller{ID: 6111, Role: roleCustomer, AccountType: core.AccountTypeAccount}
)

func a144MailHandler(panel *Panel) http.Handler {
	return newRequestIdentityGuard(panel.db.GetDB()).wrap(http.HandlerFunc(panel.handleDomainMail))
}

// Item 6a: creating a mailbox, the page left while the Agent creates it.
func TestA144MailboxCreatedWhileThePageIsLeftHasItsRow(t *testing.T) {
	agent := &a144MailAgent{entered: make(chan struct{}), release: make(chan struct{})}
	panel, domainID := newA144MailPanel(t, agent)
	handler := a144MailHandler(panel)
	path := "/api/v1/domains/" + strconv.Itoa(domainID) + "/mail/accounts"
	leaveDuring(t, handler, a144MailRequest(http.MethodPost, path, `{"address":"satis@example.com","password":"Long-enough-9","quota_mb":200}`,
		a144Admin, requestIdentityTestID(144601)), agent.entered, agent.release)
	var quota int
	if err := panel.db.GetDB().QueryRow(`SELECT quota_mb FROM email_accounts WHERE address = 'satis@example.com'`).Scan(&quota); err != nil || quota != 200 {
		t.Fatalf("the mailbox exists on the server, but the panel's row: quota=%d err=%v", quota, err)
	}
	if agent.adds() != 1 {
		t.Fatalf("Agent creations: %d", agent.adds())
	}
	row := readRequestIdentityRow(t, panel.db.GetDB(), requestIdentityTestID(144601))
	if row.status != requestIdentityDone || row.code != http.StatusOK {
		t.Fatalf("request record: %+v", row)
	}
	// The stored digest of the request leaves the password out.
	var digest string
	if err := panel.db.GetDB().QueryRow(`SELECT request_sha256 FROM request_identities WHERE id = ?`, requestIdentityTestID(144601)).Scan(&digest); err != nil {
		t.Fatal(err)
	}
	if digest == requestIdentityHash(http.MethodPost, path, "", []byte(`{"address":"satis@example.com","password":"Long-enough-9","quota_mb":200}`)) {
		t.Fatal("the stored digest covers the password")
	}
}

// Item 6a: saving an out-of-office reply, the page left while the Agent
// writes it.
func TestA144AutoresponderSavedWhileThePageIsLeftHasItsCopy(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	agent := &autoresponderPanelAgent{}
	agent.respond = func(*transport.SetMailAutoresponderRequest) (transport.MailAutoresponderResponse, error) {
		once.Do(func() { close(entered) })
		<-release
		return transport.MailAutoresponderResponse{Applied: true, Script: transport.AutoresponderScriptManaged, SieveAvailable: true}, nil
	}
	panel, domainID := newAutoresponderPanel(t, agent)
	handler := newRequestIdentityGuard(panel.db.GetDB()).wrap(http.HandlerFunc(panel.handleDomainMail))
	leaveDuring(t, handler, a144MailRequest(http.MethodPut, "/api/v1/domains/"+strconv.Itoa(domainID)+"/mail/autoresponder",
		`{"id":6201,"enabled":true,"subject":"Away","message":"Back soon","interval_days":1}`, a144Admin, requestIdentityTestID(144602)), entered, release)
	var enabled int
	var subject string
	if err := panel.db.GetDB().QueryRow(`SELECT enabled, subject FROM email_autoresponders WHERE account_id = 6201`).Scan(&enabled, &subject); err != nil || enabled != 1 || subject != "Away" {
		t.Fatalf("the reply is on the server, but the panel's copy: enabled=%d subject=%q err=%v", enabled, subject, err)
	}
	if n := len(agent.calls()); n != 1 {
		t.Fatalf("Agent writes: %d", n)
	}
}

// Item 6b: a mailbox the server holds without a row is never an internal
// error. The administrator is offered the adoption; a customer is told to
// ask the administrator; nothing was changed.
func TestA144MailboxOnTheServerWithoutARowIsSaidPlainly(t *testing.T) {
	agent := &a144MailAgent{onServer: map[string]transport.MailboxOnServerResponse{
		"satis@example.com": {InUsers: true, InMap: true, OwnFolder: true, QuotaMB: 300},
	}}
	panel, domainID := newA144MailPanel(t, agent)
	handler := a144MailHandler(panel)
	path := "/api/v1/domains/" + strconv.Itoa(domainID) + "/mail/accounts"
	for _, c := range []struct {
		caller *Caller
		reason string
		says   string
	}{
		{a144Admin, "admin", "Adopt it into this domain"},
		{a144Customer, "", "Ask the server's administrator"},
	} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, a144MailRequest(http.MethodPost, path, `{"address":"satis","password":"Long-enough-9","quota_mb":200}`, c.caller, ""))
		var body apiErrorBody
		_ = json.Unmarshal(recorder.Body.Bytes(), &body)
		if recorder.Code != http.StatusConflict || body.Code != errCodeMailAccountOnServerOnly || body.Reason != c.reason ||
			body.Vars["address"] != "satis@example.com" || !strings.Contains(body.Error, c.says) || !strings.Contains(body.Error, "nothing was changed") {
			t.Fatalf("role %s: %d %s", c.caller.Role, recorder.Code, recorder.Body.String())
		}
	}
	var count int
	if err := panel.db.GetDB().QueryRow(`SELECT COUNT(*) FROM email_accounts`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("rows after the refusals: %d (%v)", count, err)
	}
}

// Item 6b: the administrator's adoption reads the server and writes the row
// with the size limit found there; it refuses what it does not record by
// itself, and only an administrator may do it.
func TestA144AdministratorAdoptsAMailboxFoundOnTheServer(t *testing.T) {
	agent := &a144MailAgent{onServer: map[string]transport.MailboxOnServerResponse{
		"satis@example.com":   {InUsers: true, InMap: true, OwnFolder: true, QuotaMB: 300},
		"half@example.com":    {InUsers: true},
		"owners@example.com":  {InUsers: true, InMap: true, OwnFolder: false, QuotaMB: 100},
		"nolimit@example.com": {InUsers: true, InMap: true, OwnFolder: true},
	}}
	panel, domainID := newA144MailPanel(t, agent)
	handler := a144MailHandler(panel)
	path := "/api/v1/domains/" + strconv.Itoa(domainID) + "/mail/accounts/adopt"
	adopt := func(caller *Caller, address string) (*httptest.ResponseRecorder, apiErrorBody) {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, a144MailRequest(http.MethodPost, path, `{"address":"`+address+`"}`, caller, ""))
		var body apiErrorBody
		_ = json.Unmarshal(recorder.Body.Bytes(), &body)
		return recorder, body
	}
	if recorder, _ := adopt(a144Customer, "satis@example.com"); recorder.Code != http.StatusForbidden || agent.reads != 0 {
		t.Fatalf("a customer's adoption: %d %s (reads %d)", recorder.Code, recorder.Body.String(), agent.reads)
	}
	for address, reason := range map[string]string{
		"gone@example.com":    mailAdoptGone,
		"half@example.com":    mailAdoptIncomplete,
		"owners@example.com":  mailAdoptElsewhere,
		"nolimit@example.com": mailAdoptElsewhere,
	} {
		if recorder, body := adopt(a144Admin, address); recorder.Code != http.StatusConflict || body.Code != errCodeMailAccountAdoptRefused || body.Reason != reason {
			t.Fatalf("%s: %d %s", address, recorder.Code, recorder.Body.String())
		}
	}
	// Another domain's mailbox cannot be named through this domain.
	if recorder, _ := adopt(a144Admin, "satis@other.test"); recorder.Code != http.StatusBadRequest {
		t.Fatalf("an address of another domain: %d %s", recorder.Code, recorder.Body.String())
	}
	recorder, _ := adopt(a144Admin, "satis@example.com")
	if recorder.Code != http.StatusOK {
		t.Fatalf("adoption: %d %s", recorder.Code, recorder.Body.String())
	}
	var quota, owner int
	if err := panel.db.GetDB().QueryRow(`SELECT quota_mb, domain_id FROM email_accounts WHERE address = 'satis@example.com'`).Scan(&quota, &owner); err != nil || quota != 300 || owner != domainID {
		t.Fatalf("adopted row: quota=%d domain=%d err=%v", quota, owner, err)
	}
	if agent.adds() != 0 {
		t.Fatalf("the adoption created a mailbox on the server (%d calls)", agent.adds())
	}
	// Adopted once: a second adoption is the ordinary "already exists".
	if recorder, body := adopt(a144Admin, "satis@example.com"); recorder.Code != http.StatusConflict || body.Code != errCodeMailAccountExists {
		t.Fatalf("second adoption: %d %s", recorder.Code, recorder.Body.String())
	}
}

// a144AgentClient serves agent as the Panel's Agent.
func a144AgentClient(t *testing.T, agent any) *transport.ReconnectingClient {
	t.Helper()
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
	return transport.NewReconnectingClientWithContextConnector(client, connector)
}
