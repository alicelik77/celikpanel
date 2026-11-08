package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/rpc"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Mailbox recovery (alpha.130; docs/MAILBOX-RECOVERY-REVIEW.md): the
// review's conditions driven through the real handlers, the real database and
// an RPC Agent double.

type recoveryAgent struct {
	verifiedAPTAgentRPCFixture

	mu          sync.Mutex
	fingerprint string
	disabled    bool
	setAnswer   transport.SetMailPasswordIfUnchangedResponse
	setCalls    []transport.SetMailPasswordIfUnchangedRequest
	kicked      []string
}

func (a *recoveryAgent) MailPasswordFingerprintV1(req *transport.MailPasswordFingerprintRequest, resp *transport.MailPasswordFingerprintResponse) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	*resp = transport.MailPasswordFingerprintResponse{Found: true, LoginDisabled: a.disabled, Fingerprint: a.fingerprint}
	return nil
}

func (a *recoveryAgent) SetMailPasswordIfUnchangedV1(req *transport.SetMailPasswordIfUnchangedRequest, resp *transport.SetMailPasswordIfUnchangedResponse) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.setCalls = append(a.setCalls, *req)
	answer := a.setAnswer
	if answer == (transport.SetMailPasswordIfUnchangedResponse{}) {
		if req.ExpectedFingerprint == a.fingerprint {
			answer.Applied = true
			a.fingerprint = "sha256:" + strings.Repeat("e", 64)
		} else {
			answer.Changed = true
		}
	}
	*resp = answer
	return nil
}

func (a *recoveryAgent) EndMailSessionsV1(req *transport.EndMailSessionsRequest, resp *transport.EndMailSessionsResponse) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.kicked = append(a.kicked, req.Email)
	resp.Applied, resp.Ended = true, true
	return nil
}

type recoveryTestMail struct{ to, body string }

type mailboxRecoveryFixture struct {
	panel *Panel
	agent *recoveryAgent
	mu    sync.Mutex
	mails []recoveryTestMail
	in    chan struct{}
}

const (
	mrecDomainID  = 6603
	mrecAccountID = 6604
)

func newMailboxRecoveryFixture(t *testing.T) *mailboxRecoveryFixture {
	t.Helper()
	database, err := openTemplatePanelDatabase(filepath.Join(t.TempDir(), "panel.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	if _, err := database.GetDB().Exec(`
		INSERT INTO users (id, username, password_hash, email, role) VALUES (6601, 'mrec-customer', 'x', 'c@example.test', 'customer');
		INSERT INTO users (id, username, password_hash, email, role) VALUES (6611, 'mrec-other', 'x', 'o@example.test', 'customer');
		INSERT INTO subscriptions (id, owner_id, name, status) VALUES (6602, 6601, 'S', 'active');
		INSERT INTO subscriptions (id, owner_id, name, status) VALUES (6612, 6611, 'O', 'active');
		INSERT INTO domains (id, subscription_id, name, status) VALUES (6603, 6602, 'example.com', 'active');
		INSERT INTO email_accounts (id, domain_id, address, password_hash, quota_mb) VALUES (6604, 6603, 'anna@example.com', 'managed-by-agent', 100);`); err != nil {
		t.Fatal(err)
	}
	agent := &recoveryAgent{fingerprint: "sha256:" + strings.Repeat("a", 64)}
	server := rpc.NewServer()
	if err := server.RegisterName("Agent", agent); err != nil {
		t.Fatal(err)
	}
	connector := func(ctx context.Context) (*rpc.Client, error) {
		serverConn, clientConn := net.Pipe()
		go server.ServeConn(serverConn)
		return rpc.NewClient(clientConn), nil
	}
	client, _ := connector(context.Background())
	t.Cleanup(func() { _ = client.Close() })
	f := &mailboxRecoveryFixture{agent: agent, in: make(chan struct{}, 32)}
	f.panel = &Panel{
		db: database, pkgFamilyVal: "apt",
		agentClient: transport.NewReconnectingClientWithContextConnector(client, connector),
	}

	oldHost, oldProbe, oldSend, oldCommit := accountLinkPanelHost, accountMailProbe, accountMailSend, buildCommit
	t.Cleanup(func() {
		accountLinkPanelHost, accountMailProbe, accountMailSend, buildCommit = oldHost, oldProbe, oldSend, oldCommit
		resetAccountMailCache()
		resetMailboxRecoveryLimiters()
	})
	buildCommit = "mailbox-recovery-test"
	accountLinkPanelHost = func() string { return "panel.example.test" }
	accountMailProbe = func(context.Context) error { return nil }
	accountMailSend = func(_ context.Context, _, to string, message []byte) error {
		f.mu.Lock()
		f.mails = append(f.mails, recoveryTestMail{to: to, body: decodeRecoveryMail(t, message)})
		f.mu.Unlock()
		f.in <- struct{}{}
		return nil
	}
	resetAccountMailCache()
	resetMailboxRecoveryLimiters()
	return f
}

func (f *mailboxRecoveryFixture) waitMail(t *testing.T) recoveryTestMail {
	t.Helper()
	select {
	case <-f.in:
	case <-time.After(5 * time.Second):
		t.Fatal("no message was handed over")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.mails[len(f.mails)-1]
}

func (f *mailboxRecoveryFixture) noMail(t *testing.T) {
	t.Helper()
	select {
	case <-f.in:
		f.mu.Lock()
		defer f.mu.Unlock()
		t.Fatalf("a message was handed over: %+v", f.mails[len(f.mails)-1])
	case <-time.After(300 * time.Millisecond):
	}
}

func (f *mailboxRecoveryFixture) manage(t *testing.T, method, query, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, "/api/v1/domains/6603/mail/accounts/recovery"+query, strings.NewReader(body))
	request = request.WithContext(context.WithValue(request.Context(), callerKey, &Caller{
		ID: 6601, Role: roleCustomer, AccountType: core.AccountTypeAccount,
	}))
	recorder := httptest.NewRecorder()
	f.panel.handleMailboxRecovery(recorder, request, mrecDomainID)
	return recorder
}

func (f *mailboxRecoveryFixture) public(t *testing.T, handler func(http.ResponseWriter, *http.Request), body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/mailbox-recovery/x", strings.NewReader(body))
	request.RemoteAddr = "192.0.2.20:5000"
	recorder := httptest.NewRecorder()
	handler(recorder, request)
	return recorder
}

var mailLinkToken = regexp.MustCompile(`#t=([A-Za-z0-9_-]{43})`)

func tokenIn(t *testing.T, body string) string {
	t.Helper()
	match := mailLinkToken.FindStringSubmatch(body)
	if match == nil {
		t.Fatalf("no link in %q", body)
	}
	return match[1]
}

func recoveryState(t *testing.T, f *mailboxRecoveryFixture) mailboxRecoveryView {
	t.Helper()
	response := f.manage(t, http.MethodGet, "?id=6604", "")
	if response.Code != http.StatusOK {
		t.Fatalf("GET status=%d body=%s", response.Code, response.Body.String())
	}
	var view mailboxRecoveryView
	if err := json.Unmarshal(response.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	return view
}

// setAndConfirm sets anna's recovery address and opens its confirmation link.
func setAndConfirm(t *testing.T, f *mailboxRecoveryFixture, address string) {
	t.Helper()
	response := f.manage(t, http.MethodPut, "", `{"id":6604,"address":"`+address+`","lang":"en"}`)
	if response.Code != http.StatusOK {
		t.Fatalf("PUT status=%d body=%s", response.Code, response.Body.String())
	}
	confirm := f.waitMail(t)
	if confirm.to != strings.ToLower(address) || !strings.Contains(confirm.body, "/account-link/mailbox-confirm#t=") {
		t.Fatalf("confirmation mail = %+v", confirm)
	}
	if got := recoveryState(t, f); got.State != mailboxRecoveryStatePending {
		t.Fatalf("state before confirmation = %+v", got)
	}
	if response := f.public(t, f.panel.handleMailboxRecoveryConfirm, `{"token":"`+tokenIn(t, confirm.body)+`"}`); response.Code != http.StatusOK {
		t.Fatalf("confirm status=%d body=%s", response.Code, response.Body.String())
	}
	notice := f.waitMail(t)
	if notice.to != "anna@example.com" || !strings.Contains(notice.body, "now has a recovery address") ||
		strings.Contains(notice.body, strings.ToLower(address)) {
		t.Fatalf("mailbox notice = %+v (the address must be masked)", notice)
	}
}

func TestMailboxRecoveryWholePath(t *testing.T) {
	f := newMailboxRecoveryFixture(t)
	setAndConfirm(t, f, "Anna.Private@Elsewhere.test")
	if got := recoveryState(t, f); got.State != mailboxRecoveryStateConfirmed || got.Address != "anna.private@elsewhere.test" {
		t.Fatalf("state after confirmation = %+v", got)
	}

	// Forgot the password: the same answer, the link after it.
	request := f.public(t, f.panel.handleMailboxRecoveryRequest, `{"address":"ANNA@example.com","lang":"tr"}`)
	if request.Code != http.StatusAccepted || strings.TrimSpace(request.Body.String()) != `{"accepted":true}` {
		t.Fatalf("request status=%d body=%s", request.Code, request.Body.String())
	}
	reset := f.waitMail(t)
	if reset.to != "anna.private@elsewhere.test" || !strings.Contains(reset.body, "/account-link/mailbox-reset#t=") ||
		!strings.Contains(reset.body, "30 dakika") {
		t.Fatalf("reset mail = %+v", reset)
	}
	token := tokenIn(t, reset.body)
	if link := f.public(t, f.panel.handleMailboxRecoveryLink, `{"token":"`+token+`"}`); link.Code != http.StatusOK ||
		!strings.Contains(link.Body.String(), "anna@example.com") {
		t.Fatalf("link status=%d body=%s", link.Code, link.Body.String())
	}
	// The password rule first; the link stays usable after a weak one.
	if weak := f.public(t, f.panel.handleMailboxRecoveryReset, `{"token":"`+token+`","new_password":"short"}`); weak.Code != http.StatusBadRequest ||
		!strings.Contains(weak.Body.String(), "PASSWORD_WEAK") {
		t.Fatalf("weak password status=%d body=%s", weak.Code, weak.Body.String())
	}
	done := f.public(t, f.panel.handleMailboxRecoveryReset, `{"token":"`+token+`","new_password":"Fresh-Mailbox-Pass-7!"}`)
	if done.Code != http.StatusOK || strings.Contains(done.Body.String(), "Fresh-Mailbox") {
		t.Fatalf("reset status=%d body=%s", done.Code, done.Body.String())
	}
	f.agent.mu.Lock()
	calls, kicked := append([]transport.SetMailPasswordIfUnchangedRequest(nil), f.agent.setCalls...), append([]string(nil), f.agent.kicked...)
	f.agent.mu.Unlock()
	if len(calls) != 1 || calls[0].Email != "anna@example.com" || calls[0].ExpectedFingerprint != "sha256:"+strings.Repeat("a", 64) ||
		calls[0].ExpectedBuildCommit != "mailbox-recovery-test" {
		t.Fatalf("Agent calls = %+v", calls)
	}
	if len(kicked) != 1 || kicked[0] != "anna@example.com" {
		t.Fatalf("sessions ended for %v", kicked)
	}
	told := f.waitMail(t)
	if told.to != "anna@example.com" || !strings.Contains(told.body, "was changed with a link") {
		t.Fatalf("reset notice = %+v", told)
	}
	// Single use.
	if again := f.public(t, f.panel.handleMailboxRecoveryReset, `{"token":"`+token+`","new_password":"Fresh-Mailbox-Pass-8!"}`); again.Code != http.StatusGone {
		t.Fatalf("second use status=%d body=%s", again.Code, again.Body.String())
	}
}

func TestMailboxRecoveryRequestAnswersTheSameAndSendsNothingWithoutAConfirmedAddress(t *testing.T) {
	f := newMailboxRecoveryFixture(t)
	for _, address := range []string{"nobody@example.com", "anna@example.com"} {
		response := f.public(t, f.panel.handleMailboxRecoveryRequest, `{"address":"`+address+`"}`)
		if response.Code != http.StatusAccepted || strings.TrimSpace(response.Body.String()) != `{"accepted":true}` {
			t.Fatalf("%s: status=%d body=%s", address, response.Code, response.Body.String())
		}
	}
	f.noMail(t)
	// A pending (unconfirmed) address receives the confirmation only.
	if response := f.manage(t, http.MethodPut, "", `{"id":6604,"address":"anna@elsewhere.test"}`); response.Code != http.StatusOK {
		t.Fatalf("PUT status=%d", response.Code)
	}
	f.waitMail(t)
	f.public(t, f.panel.handleMailboxRecoveryRequest, `{"address":"anna@example.com"}`)
	f.noMail(t)
}

func TestMailboxRecoveryRefusesBadAddresses(t *testing.T) {
	f := newMailboxRecoveryFixture(t)
	for body, code := range map[string]string{
		`{"id":6604,"address":"anna@example.com"}`: errCodeMailboxRecoverySelf,
		`{"id":6604,"address":"not an address"}`:   errCodeMailboxRecoveryInvalidAddress,
	} {
		response := f.manage(t, http.MethodPut, "", body)
		if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), code) {
			t.Fatalf("%s: status=%d body=%s", body, response.Code, response.Body.String())
		}
	}
	f.noMail(t)
	if got := recoveryState(t, f); got.State != mailboxRecoveryStateNone {
		t.Fatalf("state = %+v", got)
	}
}

func TestMailboxRecoverySameConfirmedAddressChangesNothing(t *testing.T) {
	f := newMailboxRecoveryFixture(t)
	setAndConfirm(t, f, "anna@elsewhere.test")
	response := f.manage(t, http.MethodPut, "", `{"id":6604,"address":"anna@elsewhere.test"}`)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"unchanged":true`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	f.noMail(t)
	removed := f.manage(t, http.MethodDelete, "?id=6604", "")
	if removed.Code != http.StatusOK || !strings.Contains(removed.Body.String(), `"removed":true`) {
		t.Fatalf("DELETE status=%d body=%s", removed.Code, removed.Body.String())
	}
	if again := f.manage(t, http.MethodDelete, "?id=6604", ""); !strings.Contains(again.Body.String(), `"removed":false`) {
		t.Fatalf("second DELETE body=%s", again.Body.String())
	}
}

func TestMailboxRecoveryTransferVoidsTheAddressAndItsLinks(t *testing.T) {
	f := newMailboxRecoveryFixture(t)
	setAndConfirm(t, f, "anna@elsewhere.test")
	f.public(t, f.panel.handleMailboxRecoveryRequest, `{"address":"anna@example.com"}`)
	token := tokenIn(t, f.waitMail(t).body)
	if _, err := f.panel.db.GetDB().Exec(`UPDATE domains SET subscription_id = 6612 WHERE id = 6603`); err != nil {
		t.Fatal(err)
	}
	if got := recoveryState(t, f); got.State != mailboxRecoveryStateVoid || got.VoidReason != mailboxRecoveryVoidTransferred {
		t.Fatalf("state after transfer = %+v", got)
	}
	if response := f.public(t, f.panel.handleMailboxRecoveryReset, `{"token":"`+token+`","new_password":"Fresh-Mailbox-Pass-7!"}`); response.Code != http.StatusGone {
		t.Fatalf("reset after transfer status=%d body=%s", response.Code, response.Body.String())
	}
	f.public(t, f.panel.handleMailboxRecoveryRequest, `{"address":"anna@example.com"}`)
	f.noMail(t)
	f.agent.mu.Lock()
	defer f.agent.mu.Unlock()
	if len(f.agent.setCalls) != 0 {
		t.Fatalf("the Agent was asked after a transfer: %+v", f.agent.setCalls)
	}
}

func TestMailboxRecoveryLocalDomainAddedAgainVoidsTheAddress(t *testing.T) {
	f := newMailboxRecoveryFixture(t)
	if _, err := f.panel.db.GetDB().Exec(`INSERT INTO domains (id, subscription_id, name, status) VALUES (6620, 6612, 'local.test', 'active')`); err != nil {
		t.Fatal(err)
	}
	setAndConfirm(t, f, "anna@local.test")
	if _, err := f.panel.db.GetDB().Exec(`DELETE FROM domains WHERE id = 6620;
		INSERT INTO domains (id, subscription_id, name, status) VALUES (6621, 6612, 'local.test', 'active')`); err != nil {
		t.Fatal(err)
	}
	if got := recoveryState(t, f); got.State != mailboxRecoveryStateVoid || got.VoidReason != mailboxRecoveryVoidLocalDomain {
		t.Fatalf("state = %+v", got)
	}
	f.public(t, f.panel.handleMailboxRecoveryRequest, `{"address":"anna@example.com"}`)
	f.noMail(t)
}

func TestMailboxRecoverySuspensionAndLoginDisabledSendNothing(t *testing.T) {
	f := newMailboxRecoveryFixture(t)
	setAndConfirm(t, f, "anna@elsewhere.test")
	if _, err := f.panel.db.GetDB().Exec(`UPDATE subscriptions SET status = 'suspended' WHERE id = 6602`); err != nil {
		t.Fatal(err)
	}
	f.public(t, f.panel.handleMailboxRecoveryRequest, `{"address":"anna@example.com"}`)
	f.noMail(t)
	if _, err := f.panel.db.GetDB().Exec(`UPDATE subscriptions SET status = 'active' WHERE id = 6602`); err != nil {
		t.Fatal(err)
	}
	f.agent.mu.Lock()
	f.agent.disabled = true
	f.agent.mu.Unlock()
	resetMailboxRecoveryLimiters()
	f.public(t, f.panel.handleMailboxRecoveryRequest, `{"address":"anna@example.com"}`)
	f.noMail(t)
}

func TestMailboxRecoveryBusyAgentGivesTheLinkBackAndAChangedPasswordVoidsIt(t *testing.T) {
	f := newMailboxRecoveryFixture(t)
	setAndConfirm(t, f, "anna@elsewhere.test")
	f.public(t, f.panel.handleMailboxRecoveryRequest, `{"address":"anna@example.com"}`)
	token := tokenIn(t, f.waitMail(t).body)
	f.agent.mu.Lock()
	f.agent.setAnswer = transport.SetMailPasswordIfUnchangedResponse{Error: transport.MailConfigurationBusy}
	f.agent.mu.Unlock()
	busy := f.public(t, f.panel.handleMailboxRecoveryReset, `{"token":"`+token+`","new_password":"Fresh-Mailbox-Pass-7!"}`)
	if busy.Code != http.StatusServiceUnavailable || !strings.Contains(busy.Body.String(), errCodeMailboxRecoveryBusy) {
		t.Fatalf("busy status=%d body=%s", busy.Code, busy.Body.String())
	}
	// The password changed elsewhere meanwhile: the fingerprint differs.
	f.agent.mu.Lock()
	f.agent.setAnswer = transport.SetMailPasswordIfUnchangedResponse{}
	f.agent.fingerprint = "sha256:" + strings.Repeat("b", 64)
	f.agent.mu.Unlock()
	changed := f.public(t, f.panel.handleMailboxRecoveryReset, `{"token":"`+token+`","new_password":"Fresh-Mailbox-Pass-7!"}`)
	if changed.Code != http.StatusGone {
		t.Fatalf("changed status=%d body=%s", changed.Code, changed.Body.String())
	}
	f.agent.mu.Lock()
	defer f.agent.mu.Unlock()
	if len(f.agent.setCalls) != 2 || len(f.agent.kicked) != 0 {
		t.Fatalf("calls=%d kicked=%v", len(f.agent.setCalls), f.agent.kicked)
	}
}

func TestMailboxRecoveryKeepsAManualPause(t *testing.T) {
	f := newMailboxRecoveryFixture(t)
	setAndConfirm(t, f, "anna@elsewhere.test")
	if _, err := f.panel.db.GetDB().Exec(`INSERT INTO mailbox_outgoing (email_account_id, paused_at, paused_reason) VALUES (6604, datetime('now'), 'manual')`); err != nil {
		t.Fatal(err)
	}
	f.public(t, f.panel.handleMailboxRecoveryRequest, `{"address":"anna@example.com"}`)
	token := tokenIn(t, f.waitMail(t).body)
	done := f.public(t, f.panel.handleMailboxRecoveryReset, `{"token":"`+token+`","new_password":"Fresh-Mailbox-Pass-7!"}`)
	if done.Code != http.StatusOK || !strings.Contains(done.Body.String(), `"sending_paused":true`) {
		t.Fatalf("status=%d body=%s", done.Code, done.Body.String())
	}
	var paused string
	if err := f.panel.db.GetDB().QueryRow(`SELECT COALESCE(paused_reason, '') FROM mailbox_outgoing WHERE email_account_id = 6604`).Scan(&paused); err != nil || paused != "manual" {
		t.Fatalf("pause after reset = %q, %v", paused, err)
	}
}

func TestMailboxRecoveryLinksAreHashedAndMalformedTokensRefused(t *testing.T) {
	f := newMailboxRecoveryFixture(t)
	setAndConfirm(t, f, "anna@elsewhere.test")
	f.public(t, f.panel.handleMailboxRecoveryRequest, `{"address":"anna@example.com"}`)
	token := tokenIn(t, f.waitMail(t).body)
	var stored int
	if err := f.panel.db.GetDB().QueryRow(`SELECT COUNT(*) FROM mailbox_recovery_links WHERE token_hash = ? OR token_hash = ?`,
		token, hashAccountLinkToken(token)).Scan(&stored); err != nil || stored != 1 {
		t.Fatalf("stored=%d err=%v", stored, err)
	}
	var raw int
	_ = f.panel.db.GetDB().QueryRow(`SELECT COUNT(*) FROM mailbox_recovery_links WHERE token_hash = ?`, token).Scan(&raw)
	if raw != 0 {
		t.Fatal("the raw token is stored")
	}
	for _, body := range []string{`{"token":"short"}`, `{"token":"` + strings.Repeat("A", 43) + `"}`} {
		if response := f.public(t, f.panel.handleMailboxRecoveryLink, body); response.Code != http.StatusGone {
			t.Fatalf("%s: status=%d", body, response.Code)
		}
	}
	if response := f.public(t, f.panel.handleMailboxRecoveryConfirm, `{"token":"`+token+`"}`); response.Code != http.StatusGone {
		t.Fatalf("a reset token confirmed an address: %d", response.Code)
	}
}

func TestMailboxRecoveryPublicRoutesAreExactlyListed(t *testing.T) {
	for _, path := range []string{mailboxRecoveryRequestPath, mailboxRecoveryConfirmPath, mailboxRecoveryLinkPath, mailboxRecoveryResetPath} {
		if !isPublicPath(httptest.NewRequest(http.MethodPost, path, nil)) {
			t.Fatalf("%s is not public", path)
		}
		if !strings.HasPrefix(path, "/api/v1/auth/mailbox-recovery/") {
			t.Fatalf("unexpected path %s", path)
		}
	}
	if isPublicPath(httptest.NewRequest(http.MethodGet, "/api/v1/domains/1/mail/accounts/recovery", nil)) {
		t.Fatal("the manager's route is public")
	}
}

func TestMaskedRecoveryAddress(t *testing.T) {
	for in, want := range map[string]string{"anna@elsewhere.test": "a***@elsewhere.test", "x@y.test": "x***@y.test", "bad": "***"} {
		if got := maskedRecoveryAddress(in); got != want {
			t.Fatalf("%s: %s", in, got)
		}
	}
}
