package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/auth"
	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/repositories"
	"github.com/alicelik/celikpanel/internal/transport"
)

// The signed-in person's own email change (v0.1.0-alpha.99). Every request
// goes through the chain main builds: the cross-origin check, the session and
// licence check, the request-identity guard (D-029), then the handler.
// Kişinin kendi e-postasını değiştirmesi; her istek main'in kurduğu zincirden
// geçer.

const ownEmailTestPassword = "own email password 99"

type ownEmailFixture struct {
	authzMatrixFixture
	handler http.Handler
}

func newOwnEmailFixture(t *testing.T) *ownEmailFixture {
	t.Helper()
	base := newAuthzMatrixFixture(t)
	seedAdditionalUserSession(t, &base)
	hash, err := auth.HashPassword(ownEmailTestPassword)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := base.panel.db.GetDB().Exec(`UPDATE users SET password_hash = ?`, hash); err != nil {
		t.Fatal(err)
	}
	base.panel.loginLimiter = newRateLimiter(20, time.Minute)
	base.panel.requestIdentities = newRequestIdentityGuard(base.panel.db.GetDB())
	mux := http.NewServeMux()
	mux.HandleFunc(ownEmailPath, base.panel.handleChangeOwnEmail)
	return &ownEmailFixture{
		authzMatrixFixture: base,
		handler:            csrfProtect(base.panel.requireAuth(base.panel.requestIdentities.wrap(mux))),
	}
}

type ownEmailSend struct {
	method string
	id     string
	body   string
	origin string
	noID   bool
}

func (f *ownEmailFixture) send(t *testing.T, userID int, s ownEmailSend) *httptest.ResponseRecorder {
	t.Helper()
	method := s.method
	if method == "" {
		method = http.MethodPut
	}
	request := httptest.NewRequest(method, ownEmailPath, strings.NewReader(s.body))
	request.Host = "panel.example.test"
	origin := s.origin
	if origin == "" {
		origin = "https://panel.example.test"
	}
	if origin != "-" {
		request.Header.Set("Origin", origin)
	}
	request.Header.Set("Content-Type", "application/json")
	if !s.noID {
		id := s.id
		if id == "" {
			id = newOwnEmailTestID(t)
		}
		request.Header.Set(requestIdentityHeader, id)
	}
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: f.tokens[userID]})
	recorder := httptest.NewRecorder()
	f.handler.ServeHTTP(recorder, request)
	return recorder
}

var ownEmailTestIDs int

func newOwnEmailTestID(t *testing.T) string {
	t.Helper()
	ownEmailTestIDs++
	return fmt.Sprintf("%032x", 0x99000+ownEmailTestIDs)
}

func ownEmailBody(address, password string) string {
	raw, _ := json.Marshal(map[string]string{"email": address, "current_password": password})
	return string(raw)
}

func (f *ownEmailFixture) emailOf(t *testing.T, userID int) string {
	t.Helper()
	var email string
	if err := f.panel.db.GetDB().QueryRow(`SELECT email FROM users WHERE id = ?`, userID).Scan(&email); err != nil {
		t.Fatal(err)
	}
	return email
}

func (f *ownEmailFixture) auditCount(t *testing.T, userID int) int {
	t.Helper()
	var count int
	if err := f.panel.db.GetDB().QueryRow(
		`SELECT COUNT(*) FROM audit_logs WHERE action = 'account.email.changed' AND user_id = ?`, userID,
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func ownEmailCode(t *testing.T, recorder *httptest.ResponseRecorder) string {
	t.Helper()
	var body apiErrorBody
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("answer is not a coded JSON refusal: %d %s", recorder.Code, recorder.Body.String())
	}
	return body.Code
}

func TestOwnEmailChangeForEveryRoleKeepsSessionsAndAudits(t *testing.T) {
	f := newOwnEmailFixture(t)
	for _, id := range []int{authzMatrixAdminID, authzMatrixResellerID, authzMatrixCustomerID, authzMatrixAdditionalUserID} {
		raw := fmt.Sprintf("  New.Owner%d@Example.TEST ", id)
		want := fmt.Sprintf("New.Owner%d@example.test", id)
		recorder := f.send(t, id, ownEmailSend{body: ownEmailBody(raw, ownEmailTestPassword)})
		if recorder.Code != http.StatusOK {
			t.Fatalf("user %d: status %d %s", id, recorder.Code, recorder.Body.String())
		}
		var answer struct {
			Success bool   `json:"success"`
			Email   string `json:"email"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil || !answer.Success || answer.Email != want {
			t.Fatalf("user %d: answer %s", id, recorder.Body.String())
		}
		if got := f.emailOf(t, id); got != want {
			t.Fatalf("user %d: stored %q, want %q", id, got, want)
		}
		// The address is not a sign-in credential: the session goes on.
		if _, err := f.panel.sessions.Validate(context.Background(), f.tokens[id]); err != nil {
			t.Fatalf("user %d: session ended by an email change: %v", id, err)
		}
		if got := f.auditCount(t, id); got != 1 {
			t.Fatalf("user %d: %d audit rows", id, got)
		}
	}
	// The trail names the account, never the address.
	var leaked int
	if err := f.panel.db.GetDB().QueryRow(
		`SELECT COUNT(*) FROM audit_logs WHERE action LIKE '%@%' OR action LIKE '%example%'`,
	).Scan(&leaked); err != nil {
		t.Fatal(err)
	}
	if leaked != 0 {
		t.Fatalf("%d audit rows carry an address", leaked)
	}
}

func TestOwnEmailChangeRefusalsChangeNothing(t *testing.T) {
	f := newOwnEmailFixture(t)
	id := authzMatrixCustomerID
	before := f.emailOf(t, id)
	for _, c := range []struct {
		name   string
		send   ownEmailSend
		status int
		code   string
	}{
		{"invalid address", ownEmailSend{body: ownEmailBody("not an address", ownEmailTestPassword)}, http.StatusBadRequest, errCodeOwnEmailInvalid},
		{"no domain dot", ownEmailSend{body: ownEmailBody("owner@localhost", ownEmailTestPassword)}, http.StatusBadRequest, errCodeOwnEmailInvalid},
		{"display name", ownEmailSend{body: ownEmailBody("Owner <owner@example.test>", ownEmailTestPassword)}, http.StatusBadRequest, errCodeOwnEmailInvalid},
		{"too long", ownEmailSend{body: ownEmailBody(strings.Repeat("a", 65)+"@example.test", ownEmailTestPassword)}, http.StatusBadRequest, errCodeOwnEmailInvalid},
		{"same address", ownEmailSend{body: ownEmailBody(" "+before+" ", ownEmailTestPassword)}, http.StatusConflict, errCodeOwnEmailUnchanged},
		{"taken", ownEmailSend{body: ownEmailBody("matrix-reseller@example.test", ownEmailTestPassword)}, http.StatusConflict, errCodeOwnEmailTaken},
		{"taken in other letters", ownEmailSend{body: ownEmailBody("MATRIX-Admin@Example.Test", ownEmailTestPassword)}, http.StatusConflict, errCodeOwnEmailTaken},
		{"wrong password", ownEmailSend{body: ownEmailBody("fresh@example.test", "not the password")}, http.StatusForbidden, errCodeOwnEmailWrongPassword},
		{"no password", ownEmailSend{body: `{"email":"fresh@example.test"}`}, http.StatusForbidden, errCodeOwnEmailWrongPassword},
		{"unknown field", ownEmailSend{body: `{"email":"fresh@example.test","current_password":"` + ownEmailTestPassword + `","role":"admin"}`}, http.StatusBadRequest, errCodeOwnEmailRequestInvalid},
		{"two objects", ownEmailSend{body: ownEmailBody("fresh@example.test", ownEmailTestPassword) + `{}`}, http.StatusBadRequest, errCodeOwnEmailRequestInvalid},
		{"no identity", ownEmailSend{body: ownEmailBody("fresh@example.test", ownEmailTestPassword), noID: true}, http.StatusPreconditionRequired, errCodeRequestIDRequired},
		{"bad identity", ownEmailSend{body: ownEmailBody("fresh@example.test", ownEmailTestPassword), id: "NOT-HEX"}, http.StatusBadRequest, errCodeRequestIDRequired},
		{"POST", ownEmailSend{method: http.MethodPost, body: ownEmailBody("fresh@example.test", ownEmailTestPassword)}, http.StatusMethodNotAllowed, ""},
	} {
		recorder := f.send(t, id, c.send)
		if recorder.Code != c.status {
			t.Fatalf("%s: status %d, want %d: %s", c.name, recorder.Code, c.status, recorder.Body.String())
		}
		if c.code != "" {
			if got := ownEmailCode(t, recorder); got != c.code {
				t.Fatalf("%s: code %q, want %q", c.name, got, c.code)
			}
		}
		if got := f.emailOf(t, id); got != before {
			t.Fatalf("%s: address changed to %q", c.name, got)
		}
	}
	if got := f.auditCount(t, id); got != 0 {
		t.Fatalf("refusals wrote %d audit rows", got)
	}
}

func TestOwnEmailChangeIsSameOriginOnly(t *testing.T) {
	f := newOwnEmailFixture(t)
	before := f.emailOf(t, authzMatrixAdminID)
	for _, origin := range []string{"https://evil.example.net", "-"} {
		recorder := f.send(t, authzMatrixAdminID, ownEmailSend{
			body: ownEmailBody("csrf@example.test", ownEmailTestPassword), origin: origin,
		})
		if recorder.Code != http.StatusForbidden || !strings.Contains(recorder.Body.String(), "cross-origin") {
			t.Fatalf("origin %q: %d %s", origin, recorder.Code, recorder.Body.String())
		}
	}
	if got := f.emailOf(t, authzMatrixAdminID); got != before {
		t.Fatalf("cross-origin request changed the address to %q", got)
	}
}

func TestOwnEmailWrongPasswordsCountTowardTheSameLockout(t *testing.T) {
	f := newOwnEmailFixture(t)
	f.panel.loginLimiter = newRateLimiter(2, time.Minute)
	id := authzMatrixResellerID
	before := f.emailOf(t, id)
	for attempt := 1; attempt <= 2; attempt++ {
		recorder := f.send(t, id, ownEmailSend{body: ownEmailBody("guess@example.test", fmt.Sprintf("guess %d", attempt))})
		if recorder.Code != http.StatusForbidden || ownEmailCode(t, recorder) != errCodeOwnEmailWrongPassword {
			t.Fatalf("attempt %d: %d %s", attempt, recorder.Code, recorder.Body.String())
		}
	}
	recorder := f.send(t, id, ownEmailSend{body: ownEmailBody("guess@example.test", ownEmailTestPassword)})
	if recorder.Code != http.StatusTooManyRequests || ownEmailCode(t, recorder) != errCodeOwnEmailTooManyAttempts {
		t.Fatalf("after two wrong passwords: %d %s", recorder.Code, recorder.Body.String())
	}
	if got := f.emailOf(t, id); got != before {
		t.Fatalf("locked-out request changed the address to %q", got)
	}
	// Another person's attempts are counted apart.
	other := f.send(t, authzMatrixCustomerID, ownEmailSend{body: ownEmailBody("other@example.test", ownEmailTestPassword)})
	if other.Code != http.StatusOK {
		t.Fatalf("another account was locked out too: %d %s", other.Code, other.Body.String())
	}
}

func TestOwnEmailChangeReplaysOneIdentityAndNeverChangesTwice(t *testing.T) {
	f := newOwnEmailFixture(t)
	id := authzMatrixAdminID
	identity := strings.Repeat("e", 32)
	body := ownEmailBody("replay@example.test", ownEmailTestPassword)

	first := f.send(t, id, ownEmailSend{id: identity, body: body})
	if first.Code != http.StatusOK {
		t.Fatalf("first: %d %s", first.Code, first.Body.String())
	}
	// The same action arriving again (a browser's own re-send, a page asking
	// after a lost answer) is answered from the first arrival.
	again := f.send(t, id, ownEmailSend{id: identity, body: body})
	if again.Code != http.StatusOK || again.Header().Get(requestIdentityReplayedHeader) != "1" ||
		again.Body.String() != first.Body.String() {
		t.Fatalf("replay: %d %q %s", again.Code, again.Header().Get(requestIdentityReplayedHeader), again.Body.String())
	}
	if got := f.auditCount(t, id); got != 1 {
		t.Fatalf("one action wrote %d audit rows", got)
	}
	// The identity of one change cannot carry another address.
	reused := f.send(t, id, ownEmailSend{id: identity, body: ownEmailBody("other@example.test", ownEmailTestPassword)})
	if reused.Code != http.StatusConflict || ownEmailCode(t, reused) != errCodeRequestIDReused {
		t.Fatalf("reused identity: %d %s", reused.Code, reused.Body.String())
	}
	if got := f.emailOf(t, id); got != "replay@example.test" {
		t.Fatalf("reused identity changed the address to %q", got)
	}
	// A new action is a new identity and runs: the same address is now the
	// current one, a different one is changed once more.
	same := f.send(t, id, ownEmailSend{body: body})
	if same.Code != http.StatusConflict || ownEmailCode(t, same) != errCodeOwnEmailUnchanged {
		t.Fatalf("new identity, same address: %d %s", same.Code, same.Body.String())
	}
	next := f.send(t, id, ownEmailSend{body: ownEmailBody("second@example.test", ownEmailTestPassword)})
	if next.Code != http.StatusOK || f.emailOf(t, id) != "second@example.test" {
		t.Fatalf("new identity, new address: %d %s", next.Code, next.Body.String())
	}
	if got := f.auditCount(t, id); got != 2 {
		t.Fatalf("two changes wrote %d audit rows", got)
	}

	// The stored digest leaves the password out.
	var stored string
	if err := f.panel.db.GetDB().QueryRow(
		`SELECT request_sha256 FROM request_identities WHERE id = ?`, identity,
	).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == requestIdentityHash(http.MethodPut, ownEmailPath, "", []byte(body)) {
		t.Fatal("the stored digest covers the password")
	}
	if stored != requestIdentityHash(http.MethodPut, ownEmailPath, "", requestIdentityEmailDigest([]byte(body))) {
		t.Fatal("the stored digest is not the digest of the request without its password")
	}
}

func TestOwnEmailChangeNeverOverwritesAChangeMadeElsewhere(t *testing.T) {
	f := newOwnEmailFixture(t)
	id := authzMatrixCustomerID
	current, err := f.panel.users.GetByID(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	// The handler reads the account as it was before another change landed.
	stale := *current
	stale.Email = "earlier@example.test"
	f.panel.users = &authzUserRepositoryOverride{
		UserRepository: f.panel.users,
		users:          map[int]*core.User{id: &stale},
	}
	recorder := f.send(t, id, ownEmailSend{body: ownEmailBody("mine@example.test", ownEmailTestPassword)})
	if recorder.Code != http.StatusConflict || ownEmailCode(t, recorder) != errCodeOwnEmailChangedElsewhere {
		t.Fatalf("stale read: %d %s", recorder.Code, recorder.Body.String())
	}
	if got := f.emailOf(t, id); got != current.Email {
		t.Fatalf("a change made elsewhere was overwritten with %q", got)
	}
}

func TestOwnEmailHandlerRunsOnlyBehindTheGuard(t *testing.T) {
	panel, _, user := newTOTPSecurityPanel(t)
	request := httptest.NewRequest(http.MethodPut, ownEmailPath,
		strings.NewReader(ownEmailBody("unguarded@example.test", "correct horse battery staple")))
	request = request.WithContext(context.WithValue(request.Context(), callerKey, &Caller{ID: user.ID, Role: roleCustomer}))
	recorder := httptest.NewRecorder()
	panel.handleChangeOwnEmail(recorder, request)
	if recorder.Code != http.StatusPreconditionRequired || ownEmailCode(t, recorder) != errCodeRequestIDRequired {
		t.Fatalf("unguarded call: %d %s", recorder.Code, recorder.Body.String())
	}
	stored, err := panel.users.GetByID(context.Background(), user.ID)
	if err != nil || stored.Email != user.Email {
		t.Fatalf("unguarded call changed the address: %+v %v", stored, err)
	}
}

func TestOwnEmailRouteGatesFollowThePasswordChange(t *testing.T) {
	put := httptest.NewRequest(http.MethodPut, ownEmailPath, nil)
	route, ok := requestIdentityRouteFor(put)
	if !ok || route.pattern != ownEmailPath || route.digest == nil {
		t.Fatalf("the guard does not protect PUT %s: %+v %v", ownEmailPath, route, ok)
	}
	if _, ok := requestIdentityRouteFor(httptest.NewRequest(http.MethodPost, ownEmailPath, nil)); ok {
		t.Fatal("the guard names POST on the email route")
	}
	// Every role with a session reaches it, additional users included, as the
	// password change; the licence gate is unchanged (it is not an identity
	// recovery route).
	if isAdminOnlyPath(ownEmailPath) || isAdditionalUserRestrictedPath(ownEmailPath) {
		t.Fatal("the email route is restricted by role")
	}
	if licenseRecoveryRequest(put) {
		t.Fatal("the licence gate's recovery list changed")
	}
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(source), "http.HandleFunc(ownEmailPath, panel.handleChangeOwnEmail)") {
		t.Fatal("main.go does not register the email route")
	}
}

// The panel certificate request reads the administrator's address when it is
// made: a refusal sends the person to My account, and after the change the
// next request carries the new address. Nothing is requested in between.
func TestPanelCertificateReadsTheOwnEmailAtRequestTime(t *testing.T) {
	statusStarted := make(chan struct{})
	releaseStatus := make(chan struct{})
	released := false
	release := func() {
		if !released {
			released = true
			close(releaseStatus)
		}
	}
	t.Cleanup(release)
	agent := &firewallSyncTestAgent{
		status:        FirewallStatusResp{Enabled: true, EngineAvailable: true},
		statusStarted: statusStarted,
		releaseStatus: releaseStatus,
		issueResponse: transport.IssuePanelCertificateResponse{Issued: true},
	}
	panel, database := newPanelCertificateSagaTestPanel(t, agent)
	hash, err := auth.HashPassword(ownEmailTestPassword)
	if err != nil {
		t.Fatal(err)
	}
	result, err := database.GetDB().Exec(
		"INSERT INTO users (username,password_hash,email,role) VALUES (?,?,?,?)",
		"cert-email-admin", hash, "not an address", roleAdmin,
	)
	if err != nil {
		t.Fatal(err)
	}
	adminID64, _ := result.LastInsertId()
	adminID := int(adminID64)
	panel.users = repositories.NewPostgresUserRepository(database.GetDB())
	panel.loginLimiter = newRateLimiter(20, time.Minute)
	panel.requestIdentities = newRequestIdentityGuard(database.GetDB())
	withAdmin := func(request *http.Request) *http.Request {
		return request.WithContext(context.WithValue(request.Context(), callerKey, &Caller{ID: adminID, Role: roleAdmin}))
	}
	requestCertificate := func(requestID string) *httptest.ResponseRecorder {
		request := withAdmin(httptest.NewRequest(http.MethodPost, "/api/v1/settings/panel-certificate",
			panelCertificateSagaRequestBody(t, requestID)))
		recorder := httptest.NewRecorder()
		panel.handlePanelCertificate(recorder, request)
		return recorder
	}

	refused := requestCertificate(strings.Repeat("7", 32))
	var refusal apiErrorBody
	_ = json.Unmarshal(refused.Body.Bytes(), &refusal)
	if refused.Code != http.StatusConflict || refusal.Code != "panel_certificate_contact_email_invalid" ||
		refusal.Action != ownEmailAccountAction {
		t.Fatalf("invalid contact: %d %s", refused.Code, refused.Body.String())
	}
	var rows int
	if err := database.GetDB().QueryRow("SELECT COUNT(*) FROM service_operations").Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Fatalf("a refused request left %d operation rows", rows)
	}

	mux := http.NewServeMux()
	mux.HandleFunc(ownEmailPath, panel.handleChangeOwnEmail)
	change := withAdmin(httptest.NewRequest(http.MethodPut, ownEmailPath,
		strings.NewReader(ownEmailBody("Owner@Example.TEST", ownEmailTestPassword))))
	change.Header.Set(requestIdentityHeader, strings.Repeat("8", 32))
	changed := httptest.NewRecorder()
	panel.requestIdentities.wrap(mux).ServeHTTP(changed, change)
	if changed.Code != http.StatusOK {
		t.Fatalf("email change: %d %s", changed.Code, changed.Body.String())
	}
	agent.mu.Lock()
	issued := agent.issueCalls
	agent.mu.Unlock()
	if issued != 0 {
		t.Fatalf("the email change requested %d certificates", issued)
	}

	requestID := strings.Repeat("9", 32)
	accepted := requestCertificate(requestID)
	if accepted.Code != http.StatusAccepted {
		t.Fatalf("after the change: %d %s", accepted.Code, accepted.Body.String())
	}
	// Read the durable request while the saga waits at its firewall preflight.
	select {
	case <-statusStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("the certificate request did not reach its preflight")
	}
	op, err := panel.serviceOperationByRequestID(context.Background(), requestID)
	if err != nil {
		t.Fatal(err)
	}
	data, err := decodePanelCertificateSagaData(op)
	if err != nil || data.Email != "Owner@example.test" {
		t.Fatalf("the request carries %q (err %v), want the new address", data.Email, err)
	}
	release()
	waitPanelCertificateOperation(t, panel, requestID, serviceOperationSucceeded)
}
