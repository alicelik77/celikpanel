package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
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
		// The answer carries no address: the guard keeps it for 24 hours.
		if strings.TrimSpace(recorder.Body.String()) != `{"success":true}` {
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

// The email change and the password change share ONE re-authentication limit
// per user and client address (reauthAttemptAction): a stolen session gets one
// budget of password guesses across both routes, not one per route.
func TestOwnEmailAndPasswordChangeShareOneReauthenticationLimit(t *testing.T) {
	f := newOwnEmailFixture(t)
	f.panel.loginLimiter = newRateLimiter(2, time.Minute)
	id := authzMatrixResellerID
	before := f.emailOf(t, id)
	// One wrong guess on the password change...
	passwordGuess := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password", strings.NewReader(
		`{"current_password":"guess 1","new_password":"attacker supplied password"}`))
	passwordGuess = passwordGuess.WithContext(context.WithValue(passwordGuess.Context(), callerKey, &Caller{ID: id, Role: roleReseller}))
	guessed := httptest.NewRecorder()
	f.panel.handleChangeOwnPassword(guessed, passwordGuess)
	if guessed.Code != http.StatusForbidden {
		t.Fatalf("password guess: %d %s", guessed.Code, guessed.Body.String())
	}
	// ...and one on the email change use up the same budget of two.
	recorder := f.send(t, id, ownEmailSend{body: ownEmailBody("guess@example.test", "guess 2")})
	if recorder.Code != http.StatusForbidden || ownEmailCode(t, recorder) != errCodeOwnEmailWrongPassword {
		t.Fatalf("email guess: %d %s", recorder.Code, recorder.Body.String())
	}
	recorder = f.send(t, id, ownEmailSend{body: ownEmailBody("guess@example.test", ownEmailTestPassword)})
	if recorder.Code != http.StatusTooManyRequests || ownEmailCode(t, recorder) != errCodeOwnEmailTooManyAttempts {
		t.Fatalf("after two wrong passwords: %d %s", recorder.Code, recorder.Body.String())
	}
	if source, err := os.ReadFile("account_handlers.go"); err != nil ||
		!strings.Contains(string(source), "p.allowSensitiveAuthAttempt(r, userID, reauthAttemptAction)") {
		t.Fatal("the password change does not use the shared re-authentication limit")
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

// The refusals are told in this order: an unreadable request, an address the
// panel cannot use, then the password; whether the address is already yours or
// another account's only after the password was proven.
func TestOwnEmailWrongPasswordIsToldBeforeAnythingAboutTheAddress(t *testing.T) {
	f := newOwnEmailFixture(t)
	id := authzMatrixCustomerID
	before := f.emailOf(t, id)
	for _, address := range []string{"matrix-reseller@example.test", "MATRIX-ADMIN@example.test", before} {
		recorder := f.send(t, id, ownEmailSend{body: ownEmailBody(address, "not the password")})
		if recorder.Code != http.StatusForbidden || ownEmailCode(t, recorder) != errCodeOwnEmailWrongPassword {
			t.Fatalf("%s with a wrong password: %d %s", address, recorder.Code, recorder.Body.String())
		}
	}
	if got := f.emailOf(t, id); got != before {
		t.Fatalf("address changed to %q", got)
	}
}

// Keys are matched by their exact bytes. Go's JSON decoding would match
// "Current_Password" or "current_paſsword" (ſ folds to s) to the password
// field; such a body is refused, and the stored digest is of the address alone.
func TestOwnEmailBodyKeysAreExactAndTheDigestHoldsOnlyTheAddress(t *testing.T) {
	f := newOwnEmailFixture(t)
	id := authzMatrixAdminID
	before := f.emailOf(t, id)
	bodies := []string{
		`{"email":"x@example.test","Current_Password":"` + ownEmailTestPassword + `"}`,
		`{"email":"x@example.test","current_paſsword":"` + ownEmailTestPassword + `"}`,
		`{"EMAIL":"x@example.test","current_password":"` + ownEmailTestPassword + `"}`,
		`{"email":"x@example.test","current_password":"` + ownEmailTestPassword + `","user_id":9702}`,
		`{"email":"x@example.test","current_password":"` + ownEmailTestPassword + `","id":9702}`,
		`{"email":"x@example.test","current_password":7}`,
		`["x@example.test"]`,
		`null`,
	}
	for _, body := range bodies {
		identity := newOwnEmailTestID(t)
		recorder := f.send(t, id, ownEmailSend{id: identity, body: body})
		if recorder.Code != http.StatusBadRequest || ownEmailCode(t, recorder) != errCodeOwnEmailRequestInvalid {
			t.Fatalf("%s: %d %s", body, recorder.Code, recorder.Body.String())
		}
		digest := requestIdentityEmailDigest([]byte(body))
		if strings.Contains(string(digest), "password") || strings.Contains(string(digest), ownEmailTestPassword) {
			t.Fatalf("%s: digest input %q holds the password", body, digest)
		}
		var stored string
		if err := f.panel.db.GetDB().QueryRow(`SELECT request_sha256 FROM request_identities WHERE id = ?`, identity).Scan(&stored); err != nil {
			t.Fatal(err)
		}
		if stored != requestIdentityHash(http.MethodPut, ownEmailPath, "", []byte("unreadable")) {
			t.Fatalf("%s: the stored digest is not the constant of an unreadable request", body)
		}
	}
	if got := f.emailOf(t, id); got != before {
		t.Fatalf("a refused body changed the address to %q", got)
	}
	// A readable body is digested as the address alone, re-encoded.
	readable := ownEmailBody("x@example.test", ownEmailTestPassword)
	if got := string(requestIdentityEmailDigest([]byte(readable))); got != `{"email":"x@example.test"}` {
		t.Fatalf("digest input = %s", got)
	}
}

// One identity, one answer: a later arrival with another password (right or
// wrong) gets the stored answer and is never checked again.
func TestOwnEmailSameIdentityWithAnotherPasswordGetsTheStoredAnswer(t *testing.T) {
	f := newOwnEmailFixture(t)
	id := authzMatrixResellerID
	identity := strings.Repeat("f", 32)
	first := f.send(t, id, ownEmailSend{id: identity, body: ownEmailBody("stored@example.test", ownEmailTestPassword)})
	if first.Code != http.StatusOK {
		t.Fatalf("first: %d %s", first.Code, first.Body.String())
	}
	again := f.send(t, id, ownEmailSend{id: identity, body: ownEmailBody("stored@example.test", "a wrong password")})
	if again.Code != http.StatusOK || again.Header().Get(requestIdentityReplayedHeader) != "1" || again.Body.String() != first.Body.String() {
		t.Fatalf("replay with another password: %d %q %s", again.Code, again.Header().Get(requestIdentityReplayedHeader), again.Body.String())
	}
	// A wrong-password answer is stored too, and a right password under that
	// identity does not turn it into a change.
	wrongID := strings.Repeat("d", 32)
	wrong := f.send(t, id, ownEmailSend{id: wrongID, body: ownEmailBody("later@example.test", "a wrong password")})
	right := f.send(t, id, ownEmailSend{id: wrongID, body: ownEmailBody("later@example.test", ownEmailTestPassword)})
	if wrong.Code != http.StatusForbidden || right.Code != http.StatusForbidden || right.Header().Get(requestIdentityReplayedHeader) != "1" {
		t.Fatalf("stored refusal: %d then %d %s", wrong.Code, right.Code, right.Body.String())
	}
	if got := f.emailOf(t, id); got != "stored@example.test" {
		t.Fatalf("address = %q", got)
	}
	if got := f.auditCount(t, id); got != 1 {
		t.Fatalf("%d audit rows", got)
	}
}

// An administrator who impersonates a customer holds the customer's session;
// the customer's address changes only with the customer's own password.
func TestOwnEmailImpersonatingAdministratorNeedsTheCustomersPassword(t *testing.T) {
	f := newOwnEmailFixture(t)
	customerHash, err := auth.HashPassword("the customer's own password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.panel.db.GetDB().Exec(`UPDATE users SET password_hash = ? WHERE id = ?`, customerHash, authzMatrixCustomerID); err != nil {
		t.Fatal(err)
	}
	before := f.emailOf(t, authzMatrixCustomerID)
	// The impersonated session is the customer's; the administrator knows only
	// the administrator's password.
	recorder := f.send(t, authzMatrixCustomerID, ownEmailSend{body: ownEmailBody("taken-over@example.test", ownEmailTestPassword)})
	if recorder.Code != http.StatusForbidden || ownEmailCode(t, recorder) != errCodeOwnEmailWrongPassword {
		t.Fatalf("administrator's password on the customer's session: %d %s", recorder.Code, recorder.Body.String())
	}
	if got := f.emailOf(t, authzMatrixCustomerID); got != before {
		t.Fatalf("the customer's address changed to %q", got)
	}
	if got := f.emailOf(t, authzMatrixAdminID); got != "matrix-admin@example.test" {
		t.Fatalf("the administrator's address changed to %q", got)
	}
}

// The panel certificate's gate is the certificate request's own validator: an
// address mail.ParseAddress accepts but the contact check refuses gets the
// coded refusal with the My account action, and nothing is recorded.
func TestPanelCertificateRefusesEveryContactTheCertificateWouldRefuse(t *testing.T) {
	for i, address := range []string{"admin@localhost", "root@myhost", "ünicode@example.test", "admin@192.0.2.1", "Admin <admin@example.test>"} {
		agent := &firewallSyncTestAgent{
			status:        FirewallStatusResp{Enabled: true, EngineAvailable: true},
			issueResponse: transport.IssuePanelCertificateResponse{Issued: true},
		}
		panel, database := newPanelCertificateSagaTestPanel(t, agent)
		result, err := database.GetDB().Exec(
			"INSERT INTO users (username,password_hash,email,role) VALUES (?,?,?,?)",
			fmt.Sprintf("cert-contact-%d", i), "x", address, roleAdmin,
		)
		if err != nil {
			t.Fatal(err)
		}
		adminID, _ := result.LastInsertId()
		request := httptest.NewRequest(http.MethodPost, "/api/v1/settings/panel-certificate",
			panelCertificateSagaRequestBody(t, fmt.Sprintf("%032x", 0x5100+i)))
		request = request.WithContext(context.WithValue(request.Context(), callerKey, &Caller{ID: int(adminID), Role: roleAdmin}))
		recorder := httptest.NewRecorder()
		panel.handlePanelCertificate(recorder, request)
		var refusal apiErrorBody
		_ = json.Unmarshal(recorder.Body.Bytes(), &refusal)
		if recorder.Code != http.StatusConflict || refusal.Code != "panel_certificate_contact_email_invalid" || refusal.Action != ownEmailAccountAction {
			t.Fatalf("%s: %d %s", address, recorder.Code, recorder.Body.String())
		}
		var rows int
		if err := database.GetDB().QueryRow("SELECT COUNT(*) FROM service_operations").Scan(&rows); err != nil || rows != 0 {
			t.Fatalf("%s: %d operation rows (%v)", address, rows, err)
		}
	}
}

// The setup plan's contact blocker uses the same validator.
func TestServerSetupPlanBlocksEveryContactTheCertificateWouldRefuse(t *testing.T) {
	for _, c := range []struct {
		address string
		blocked bool
	}{
		{"admin@example.test", false},
		{"admin@localhost", true},
		{"root@myhost", true},
		{"ünicode@example.test", true},
	} {
		f, state := setupOperationFixture(t)
		if _, err := f.database.GetDB().Exec(`UPDATE users SET email = ? WHERE id = ?`, c.address, f.userID); err != nil {
			t.Fatal(err)
		}
		plan, err := f.panel.buildServerSetupPlan(context.Background(), state, serviceOperationActor{UserID: f.userID})
		if err != nil {
			t.Fatal(err)
		}
		if got := slices.Contains(plan.Blockers, "panel_certificate_contact_email_invalid"); got != c.blocked {
			t.Fatalf("%s: contact blocker = %v, blockers %v", c.address, got, plan.Blockers)
		}
	}
}

// A key given twice keeps its last value, in the handler and in the digest
// alike: the address the digest names is the address that is written.
func TestOwnEmailDuplicateKeysAreReadTheSameByTheHandlerAndTheDigest(t *testing.T) {
	f := newOwnEmailFixture(t)
	id := authzMatrixAdminID
	body := `{"email":"first@example.test","email":"last@example.test","current_password":"` + ownEmailTestPassword + `"}`
	if got := string(requestIdentityEmailDigest([]byte(body))); got != `{"email":"last@example.test"}` {
		t.Fatalf("digest input = %s", got)
	}
	recorder := f.send(t, id, ownEmailSend{body: body})
	if recorder.Code != http.StatusOK || f.emailOf(t, id) != "last@example.test" {
		t.Fatalf("duplicate keys: %d %s, stored %q", recorder.Code, recorder.Body.String(), f.emailOf(t, id))
	}
}
