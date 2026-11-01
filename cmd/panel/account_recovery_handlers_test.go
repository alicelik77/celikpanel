package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/quotedprintable"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/auth"
)

// Password reset by email, invitations and address confirmation (P6). Every
// request goes through the chain main builds: the cross-origin check, the
// session check (public routes skip it), then the handlers.

type recoveryMail struct {
	from, to string
	body     string
}

type recoveryFixture struct {
	authzMatrixFixture
	handler http.Handler
	mu      sync.Mutex
	mails   []recoveryMail
	arrived chan struct{}
	sendErr error
	probeOK bool
}

func newRecoveryFixture(t *testing.T) *recoveryFixture {
	t.Helper()
	base := newAuthzMatrixFixture(t)
	seedAdditionalUserSession(t, &base)
	f := &recoveryFixture{authzMatrixFixture: base, arrived: make(chan struct{}, 32), probeOK: true}
	f.panel.loginLimiter = newRateLimiter(50, time.Minute)

	oldHost, oldProbe, oldSend, oldNow := accountLinkPanelHost, accountMailProbe, accountMailSend, accountLinkNow
	t.Cleanup(func() {
		accountLinkPanelHost, accountMailProbe, accountMailSend, accountLinkNow = oldHost, oldProbe, oldSend, oldNow
		resetAccountMailCache()
		resetAccountRecoveryLimiters()
	})
	accountLinkPanelHost = func() string { return "panel.example.test" }
	accountMailProbe = func(context.Context) error {
		f.mu.Lock()
		defer f.mu.Unlock()
		if !f.probeOK {
			return errors.New("connection refused")
		}
		return nil
	}
	accountMailSend = func(_ context.Context, from, to string, message []byte) error {
		f.mu.Lock()
		err := f.sendErr
		if err == nil {
			f.mails = append(f.mails, recoveryMail{from: from, to: to, body: decodeRecoveryMail(t, message)})
		}
		f.mu.Unlock()
		if err == nil {
			f.arrived <- struct{}{}
		}
		return err
	}
	resetAccountMailCache()
	resetAccountRecoveryLimiters()

	mux := http.NewServeMux()
	f.panel.registerAccountRecoveryRoutes(mux)
	mux.HandleFunc("/api/v1/users", f.panel.handleUsers)
	mux.HandleFunc("/api/v1/team-members", f.panel.handleTeamMembers)
	f.handler = csrfProtect(f.panel.requireAuth(mux))
	return f
}

func decodeRecoveryMail(t *testing.T, message []byte) string {
	t.Helper()
	_, body, found := strings.Cut(string(message), "\r\n\r\n")
	if !found {
		t.Fatalf("message without a body: %q", message)
	}
	decoded, err := io.ReadAll(quotedprintable.NewReader(strings.NewReader(body)))
	if err != nil {
		t.Fatal(err)
	}
	return string(decoded)
}

func (f *recoveryFixture) setMail(probeOK bool, sendErr error) {
	f.mu.Lock()
	f.probeOK, f.sendErr = probeOK, sendErr
	f.mu.Unlock()
	resetAccountMailCache()
}

func (f *recoveryFixture) do(t *testing.T, method, path, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	return f.doFrom(t, method, path, token, body, "192.0.2.10:4000")
}

func (f *recoveryFixture) doFrom(t *testing.T, method, path, token, body, remote string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Host = "panel.example.test"
	request.RemoteAddr = remote
	request.Header.Set("Origin", "https://panel.example.test")
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	}
	recorder := httptest.NewRecorder()
	f.handler.ServeHTTP(recorder, request)
	return recorder
}

// waitMail waits for the next handed-over message (the reset is sent after
// the answer).
func (f *recoveryFixture) waitMail(t *testing.T) recoveryMail {
	t.Helper()
	select {
	case <-f.arrived:
	case <-time.After(5 * time.Second):
		t.Fatal("no message was handed over")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.mails[len(f.mails)-1]
}

func (f *recoveryFixture) noMail(t *testing.T) {
	t.Helper()
	select {
	case <-f.arrived:
		t.Fatal("a message was handed over")
	case <-time.After(300 * time.Millisecond):
	}
}

func (f *recoveryFixture) verify(t *testing.T, userID int) {
	t.Helper()
	if _, err := f.panel.db.GetDB().Exec(`UPDATE users SET email_verified_at = '2026-10-01T00:00:00Z' WHERE id = ?`, userID); err != nil {
		t.Fatal(err)
	}
}

var recoveryTokenPattern = regexp.MustCompile(`https://panel\.example\.test:(\d+)(/account-link/[a-z]+)#t=([A-Za-z0-9_-]{43})`)

func recoveryToken(t *testing.T, mail recoveryMail, wantPath string) string {
	t.Helper()
	match := recoveryTokenPattern.FindStringSubmatch(mail.body)
	if match == nil {
		t.Fatalf("no link in message: %q", mail.body)
	}
	if match[1] != recoveryItoa(panelPort()) {
		t.Fatalf("link port = %s, want the configured panel port %d", match[1], panelPort())
	}
	if match[2] != wantPath {
		t.Fatalf("link path = %s, want %s", match[2], wantPath)
	}
	return match[3]
}

func recoveryItoa(n int) string {
	raw, _ := json.Marshal(n)
	return string(raw)
}

func recoveryBody(values map[string]string) string {
	raw, _ := json.Marshal(values)
	return string(raw)
}

func (f *recoveryFixture) sessionCount(t *testing.T, userID int) int {
	t.Helper()
	var count int
	if err := f.panel.db.GetDB().QueryRow(`SELECT COUNT(*) FROM sessions WHERE user_id = ?`, userID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func (f *recoveryFixture) auditCount(t *testing.T, action string, userID int) int {
	t.Helper()
	var count int
	if err := f.panel.db.GetDB().QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action = ? AND resource_id = ?`, action, userID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func (f *recoveryFixture) passwordIs(t *testing.T, userID int, password string) bool {
	t.Helper()
	var hash string
	if err := f.panel.db.GetDB().QueryRow(`SELECT password_hash FROM users WHERE id = ?`, userID).Scan(&hash); err != nil {
		t.Fatal(err)
	}
	ok, _ := auth.VerifyPassword(password, hash)
	return ok
}

func TestRecoveryRequestAnswersTheSameForEveryName(t *testing.T) {
	f := newRecoveryFixture(t)
	f.verify(t, authzMatrixCustomerID)
	f.verify(t, authzMatrixSuspendedID)

	var bodies []string
	for i, identifier := range []string{
		"nobody-here",                  // unknown
		"matrix-suspended",             // suspended (address confirmed)
		"matrix-outsider",              // address never confirmed
		"matrix-customer",              // the one that gets a message
		"MATRIX-CUSTOMER@example.test", // the same account by address
	} {
		res := f.doFrom(t, http.MethodPost, recoveryRequestPath, "",
			recoveryBody(map[string]string{"identifier": identifier, "lang": "en"}), "192.0.2."+recoveryItoa(20+i)+":1")
		if res.Code != http.StatusAccepted {
			t.Fatalf("%s: status %d %s", identifier, res.Code, res.Body)
		}
		bodies = append(bodies, res.Body.String())
		if identifier == "matrix-customer" || strings.Contains(identifier, "@") {
			mail := f.waitMail(t)
			if mail.to != "matrix-customer@example.test" || mail.from != "no-reply@panel.example.test" {
				t.Fatalf("message %s -> %s", mail.from, mail.to)
			}
		} else {
			f.noMail(t)
		}
	}
	for _, body := range bodies[1:] {
		if body != bodies[0] {
			t.Fatalf("answers differ: %q vs %q", body, bodies[0])
		}
	}
	if got := f.auditCount(t, "account.password_reset.requested", authzMatrixCustomerID); got != 2 {
		t.Fatalf("requested audit rows = %d", got)
	}
}

func TestRecoveryResetLifecycle(t *testing.T) {
	f := newRecoveryFixture(t)
	f.verify(t, authzMatrixCustomerID)
	if f.sessionCount(t, authzMatrixCustomerID) == 0 {
		t.Fatal("fixture has no session to drop")
	}

	f.do(t, http.MethodPost, recoveryRequestPath, "", recoveryBody(map[string]string{"identifier": "matrix-customer", "lang": "tr"}))
	mail := f.waitMail(t)
	if !strings.Contains(mail.body, "parola sıfırlama") {
		t.Fatalf("Turkish text expected: %q", mail.body)
	}
	token := recoveryToken(t, mail, "/account-link/reset")

	// Hash at rest: the token itself is nowhere in the database.
	var stored string
	if err := f.panel.db.GetDB().QueryRow(`SELECT token_hash FROM account_links WHERE purpose = 'password_reset' AND user_id = ?`,
		authzMatrixCustomerID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == token || stored != hashAccountLinkToken(token) {
		t.Fatalf("stored %q for token %q", stored, token)
	}

	res := f.do(t, http.MethodPost, recoveryLinkPath, "", recoveryBody(map[string]string{"token": token, "purpose": "password_reset"}))
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), `"matrix-customer"`) {
		t.Fatalf("link check: %d %s", res.Code, res.Body)
	}
	// The same token is not an invitation.
	res = f.do(t, http.MethodPost, recoveryAcceptInvitePath, "", recoveryBody(map[string]string{"token": token, "new_password": "brand new secret"}))
	if res.Code != http.StatusGone {
		t.Fatalf("reset token accepted as invitation: %d", res.Code)
	}
	res = f.do(t, http.MethodPost, recoveryResetPath, "", recoveryBody(map[string]string{"token": token, "new_password": "short"}))
	if res.Code != http.StatusBadRequest || !strings.Contains(res.Body.String(), errCodeAccountPasswordShort) {
		t.Fatalf("short password: %d %s", res.Code, res.Body)
	}
	res = f.do(t, http.MethodPost, recoveryResetPath, "", recoveryBody(map[string]string{"token": token, "new_password": "brand new secret"}))
	if res.Code != http.StatusOK {
		t.Fatalf("reset: %d %s", res.Code, res.Body)
	}
	if !f.passwordIs(t, authzMatrixCustomerID, "brand new secret") {
		t.Fatal("password not set")
	}
	if got := f.sessionCount(t, authzMatrixCustomerID); got != 0 {
		t.Fatalf("%d sessions survived the reset", got)
	}
	if f.sessionCount(t, authzMatrixResellerID) == 0 {
		t.Fatal("another account's session was dropped")
	}
	// Single use.
	res = f.do(t, http.MethodPost, recoveryResetPath, "", recoveryBody(map[string]string{"token": token, "new_password": "another secret 2"}))
	if res.Code != http.StatusGone || !strings.Contains(res.Body.String(), errCodeAccountLinkInvalid) {
		t.Fatalf("second use: %d %s", res.Code, res.Body)
	}
	if f.auditCount(t, "account.password_reset.completed", authzMatrixCustomerID) != 1 {
		t.Fatal("completion not audited")
	}
}

func TestRecoveryResetLinkExpiresAndFollowsTheAccount(t *testing.T) {
	f := newRecoveryFixture(t)
	f.verify(t, authzMatrixCustomerID)
	request := func() string {
		f.do(t, http.MethodPost, recoveryRequestPath, "", recoveryBody(map[string]string{"identifier": "matrix-customer"}))
		return recoveryToken(t, f.waitMail(t), "/account-link/reset")
	}
	reset := func(token string) int {
		return f.do(t, http.MethodPost, recoveryResetPath, "", recoveryBody(map[string]string{"token": token, "new_password": "brand new secret"})).Code
	}

	// Expired after 30 minutes.
	token := request()
	start := time.Now().UTC()
	accountLinkNow = func() time.Time { return start.Add(31 * time.Minute) }
	if code := reset(token); code != http.StatusGone {
		t.Fatalf("expired link: %d", code)
	}
	accountLinkNow = func() time.Time { return time.Now().UTC() }

	// A newer link voids the older one.
	first := request()
	second := request()
	if code := reset(first); code != http.StatusGone {
		t.Fatalf("older link: %d", code)
	}

	// A password change another way voids the open link.
	if _, err := f.panel.db.GetDB().Exec(`UPDATE users SET auth_epoch = auth_epoch + 1 WHERE id = ?`, authzMatrixCustomerID); err != nil {
		t.Fatal(err)
	}
	if code := reset(second); code != http.StatusGone {
		t.Fatalf("link after a password change: %d", code)
	}
	// A suspended account cannot use a link.
	resetAccountRecoveryLimiters()
	third := request()
	if _, err := f.panel.db.GetDB().Exec(`UPDATE users SET status = 'suspended' WHERE id = ?`, authzMatrixCustomerID); err != nil {
		t.Fatal(err)
	}
	if code := reset(third); code != http.StatusGone {
		t.Fatalf("link of a suspended account: %d", code)
	}
	// Malformed tokens never reach a lookup.
	if code := reset("not-a-token"); code != http.StatusGone {
		t.Fatalf("malformed token: %d", code)
	}
}

func TestRecoveryRequestLimits(t *testing.T) {
	f := newRecoveryFixture(t)
	f.verify(t, authzMatrixCustomerID)
	// Per account: the fourth request within the hour answers the same and
	// sends nothing.
	for i := 0; i < 4; i++ {
		res := f.doFrom(t, http.MethodPost, recoveryRequestPath, "",
			recoveryBody(map[string]string{"identifier": "matrix-customer"}), "198.51.100."+recoveryItoa(i+1)+":1")
		if res.Code != http.StatusAccepted {
			t.Fatalf("request %d: %d", i, res.Code)
		}
		if i < 3 {
			f.waitMail(t)
		} else {
			f.noMail(t)
		}
	}
	// Per client address: the sixth request is refused, whatever the name.
	for i := 0; i < 6; i++ {
		res := f.doFrom(t, http.MethodPost, recoveryRequestPath, "",
			recoveryBody(map[string]string{"identifier": "nobody-" + recoveryItoa(i)}), "203.0.113.9:1")
		want := http.StatusAccepted
		if i == 5 {
			want = http.StatusTooManyRequests
		}
		if res.Code != want {
			t.Fatalf("address request %d: %d", i, res.Code)
		}
	}
}

func TestRecoveryWithoutMailGivesGuidance(t *testing.T) {
	f := newRecoveryFixture(t)
	f.verify(t, authzMatrixCustomerID)
	f.setMail(false, nil)

	res := f.do(t, http.MethodGet, recoveryMailStatusPath, "", "")
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), `"available":false`) ||
		!strings.Contains(res.Body.String(), accountMailReasonNoMailServer) {
		t.Fatalf("status: %d %s", res.Code, res.Body)
	}
	res = f.do(t, http.MethodPost, recoveryRequestPath, "", recoveryBody(map[string]string{"identifier": "nobody"}))
	if res.Code != http.StatusServiceUnavailable || !strings.Contains(res.Body.String(), errCodeAccountMailUnavailable) {
		t.Fatalf("request without mail: %d %s", res.Code, res.Body)
	}
	accountLinkPanelHost = func() string { return "" }
	res = f.do(t, http.MethodGet, recoveryMailStatusPath, "", "")
	if !strings.Contains(res.Body.String(), accountMailReasonNoPanelAddress) {
		t.Fatalf("no panel address: %s", res.Body)
	}
}

func TestInvitationByResellerIsAcceptedOnce(t *testing.T) {
	f := newRecoveryFixture(t)
	reseller := f.tokens[authzMatrixResellerID]

	res := f.do(t, http.MethodPost, "/api/v1/users", reseller,
		`{"username":"invited-one","email":"invited-one@example.test","role":"customer","invite":true,"password":"chosen by reseller"}`)
	if res.Code != http.StatusBadRequest || !strings.Contains(res.Body.String(), errCodeInvitationPassword) {
		t.Fatalf("invite with a password: %d %s", res.Code, res.Body)
	}
	res = f.do(t, http.MethodPost, "/api/v1/users", reseller,
		`{"username":"invited-one","email":"invited-one@example.test","role":"customer","invite":true,"lang":"en"}`)
	if res.Code != http.StatusOK {
		t.Fatalf("create: %d %s", res.Code, res.Body)
	}
	var created struct {
		ID         int               `json:"id"`
		Invitation invitationOutcome `json:"invitation"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Invitation.Delivery != "sent" || created.Invitation.LinkPath != "" {
		t.Fatalf("invitation outcome %+v", created.Invitation)
	}
	mail := f.waitMail(t)
	if mail.to != "invited-one@example.test" || !strings.Contains(mail.body, "matrix-reseller created") {
		t.Fatalf("invitation message: %+v", mail)
	}
	token := recoveryToken(t, mail, "/account-link/invitation")

	// The invited account cannot be reset by mail while invited.
	f.verify(t, created.ID)
	f.do(t, http.MethodPost, recoveryRequestPath, "", recoveryBody(map[string]string{"identifier": "invited-one"}))
	f.noMail(t)
	if _, err := f.panel.db.GetDB().Exec(`UPDATE users SET email_verified_at = NULL WHERE id = ?`, created.ID); err != nil {
		t.Fatal(err)
	}

	// The list: the reseller sees it, an unrelated customer does not.
	res = f.do(t, http.MethodGet, invitationsPath, reseller, "")
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), `"invited-one"`) || !strings.Contains(res.Body.String(), `"pending"`) {
		t.Fatalf("reseller list: %d %s", res.Code, res.Body)
	}
	res = f.do(t, http.MethodGet, invitationsPath, f.tokens[authzMatrixOutsiderID], "")
	if strings.Contains(res.Body.String(), "invited-one") {
		t.Fatalf("outsider sees the invitation: %s", res.Body)
	}
	res = f.do(t, http.MethodGet, invitationsPath, f.tokens[authzMatrixAdditionalUserID], "")
	if res.Code != http.StatusForbidden {
		t.Fatalf("additional user list: %d", res.Code)
	}

	res = f.do(t, http.MethodPost, recoveryAcceptInvitePath, "", recoveryBody(map[string]string{"token": token, "new_password": "my own password"}))
	if res.Code != http.StatusOK {
		t.Fatalf("accept: %d %s", res.Code, res.Body)
	}
	if !f.passwordIs(t, created.ID, "my own password") {
		t.Fatal("password not set")
	}
	var verified *string
	if err := f.panel.db.GetDB().QueryRow(`SELECT email_verified_at FROM users WHERE id = ?`, created.ID).Scan(&verified); err != nil || verified == nil {
		t.Fatalf("address not confirmed by an emailed invitation: %v", err)
	}
	res = f.do(t, http.MethodPost, recoveryAcceptInvitePath, "", recoveryBody(map[string]string{"token": token, "new_password": "my own password 2"}))
	if res.Code != http.StatusGone {
		t.Fatalf("second accept: %d", res.Code)
	}
	if f.auditCount(t, "account.invitation.accepted", created.ID) != 1 || f.auditCount(t, "account.invitation.sent", created.ID) != 1 {
		t.Fatal("invitation not audited")
	}
}

func TestInvitationWithoutMailResendAndRevoke(t *testing.T) {
	f := newRecoveryFixture(t)
	f.setMail(false, nil)
	admin := f.tokens[authzMatrixAdminID]
	res := f.do(t, http.MethodPost, "/api/v1/users", admin,
		`{"username":"invited-two","email":"invited-two@example.test","role":"reseller","invite":true}`)
	var created struct {
		ID         int               `json:"id"`
		Invitation invitationOutcome `json:"invitation"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &created); err != nil || res.Code != http.StatusOK {
		t.Fatalf("create: %d %s", res.Code, res.Body)
	}
	out := created.Invitation
	if out.Delivery != "link" || out.Reason != accountMailReasonNoMailServer || !strings.HasPrefix(out.LinkPath, "/account-link/invitation#t=") {
		t.Fatalf("outcome without mail: %+v", out)
	}
	first := strings.TrimPrefix(out.LinkPath, "/account-link/invitation#t=")

	// Not the inviter's: the same 404 as a missing one.
	res = f.do(t, http.MethodPost, invitationsPath+"/"+recoveryItoa(int(out.ID))+"/resend", f.tokens[authzMatrixResellerID], `{"delivery":"link"}`)
	if res.Code != http.StatusNotFound {
		t.Fatalf("foreign resend: %d", res.Code)
	}
	f.setMail(true, nil)
	res = f.do(t, http.MethodPost, invitationsPath+"/"+recoveryItoa(int(out.ID))+"/resend", admin, `{"delivery":"email","lang":"tr"}`)
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), `"sent"`) {
		t.Fatalf("resend: %d %s", res.Code, res.Body)
	}
	second := recoveryToken(t, f.waitMail(t), "/account-link/invitation")
	if code := f.do(t, http.MethodPost, recoveryLinkPath, "", recoveryBody(map[string]string{"token": first, "purpose": "invitation"})).Code; code != http.StatusGone {
		t.Fatalf("old link after resend: %d", code)
	}

	var linkID int64
	if err := f.panel.db.GetDB().QueryRow(`SELECT id FROM account_links WHERE token_hash = ?`, hashAccountLinkToken(second)).Scan(&linkID); err != nil {
		t.Fatal(err)
	}
	res = f.do(t, http.MethodDelete, invitationsPath+"/"+recoveryItoa(int(linkID)), admin, "")
	if res.Code != http.StatusNoContent {
		t.Fatalf("revoke: %d %s", res.Code, res.Body)
	}
	if code := f.do(t, http.MethodPost, recoveryAcceptInvitePath, "", recoveryBody(map[string]string{"token": second, "new_password": "my own password"})).Code; code != http.StatusGone {
		t.Fatalf("revoked link: %d", code)
	}
	if f.auditCount(t, "account.invitation.revoked", created.ID) != 1 {
		t.Fatal("revoke not audited")
	}

	// A mail system that refuses the message: the inviter gets the copy.
	f.setMail(true, errors.New("550 refused"))
	res = f.do(t, http.MethodPost, "/api/v1/team-members", f.tokens[authzMatrixCustomerID],
		`{"username":"invited-member","email":"invited-member@example.test","invite":true,"access":{"subscription_permissions":[],"domain_permissions":[]}}`)
	if res.Code != http.StatusCreated || !strings.Contains(res.Body.String(), `"send_failed"`) || !strings.Contains(res.Body.String(), `"link_path"`) {
		t.Fatalf("team member invitation with a refused message: %d %s", res.Code, res.Body)
	}
}

func TestEmailVerificationEnablesReset(t *testing.T) {
	f := newRecoveryFixture(t)
	customer := f.tokens[authzMatrixCustomerID]

	res := f.do(t, http.MethodGet, ownEmailVerificationPath, customer, "")
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), `"email_verified":false`) {
		t.Fatalf("state: %d %s", res.Code, res.Body)
	}
	res = f.do(t, http.MethodPost, ownEmailVerificationPath, customer, `{"lang":"en"}`)
	if res.Code != http.StatusOK {
		t.Fatalf("send: %d %s", res.Code, res.Body)
	}
	token := recoveryToken(t, f.waitMail(t), "/account-link/verify")
	// A verification token is not a reset token.
	if code := f.do(t, http.MethodPost, recoveryResetPath, "", recoveryBody(map[string]string{"token": token, "new_password": "brand new secret"})).Code; code != http.StatusGone {
		t.Fatalf("verify token used for reset: %d", code)
	}
	res = f.do(t, http.MethodPost, recoveryVerifyEmailPath, "", recoveryBody(map[string]string{"token": token}))
	if res.Code != http.StatusOK {
		t.Fatalf("verify: %d %s", res.Code, res.Body)
	}
	res = f.do(t, http.MethodGet, ownEmailVerificationPath, customer, "")
	if !strings.Contains(res.Body.String(), `"email_verified":true`) {
		t.Fatalf("after verify: %s", res.Body)
	}
	res = f.do(t, http.MethodPost, ownEmailVerificationPath, customer, `{}`)
	if res.Code != http.StatusConflict {
		t.Fatalf("already verified: %d", res.Code)
	}
	f.do(t, http.MethodPost, recoveryRequestPath, "", recoveryBody(map[string]string{"identifier": "matrix-customer"}))
	f.waitMail(t)

	// Any change of the address makes it unconfirmed again (migration 047).
	if _, err := f.panel.db.GetDB().Exec(`UPDATE users SET email = 'changed@example.test' WHERE id = ?`, authzMatrixCustomerID); err != nil {
		t.Fatal(err)
	}
	res = f.do(t, http.MethodGet, ownEmailVerificationPath, customer, "")
	if !strings.Contains(res.Body.String(), `"email_verified":false`) {
		t.Fatalf("after address change: %s", res.Body)
	}
	// An unchanged write (the repository writes every column) keeps it.
	f.verify(t, authzMatrixCustomerID)
	if _, err := f.panel.db.GetDB().Exec(`UPDATE users SET email = email, username = username WHERE id = ?`, authzMatrixCustomerID); err != nil {
		t.Fatal(err)
	}
	res = f.do(t, http.MethodGet, ownEmailVerificationPath, customer, "")
	if !strings.Contains(res.Body.String(), `"email_verified":true`) {
		t.Fatalf("after unchanged write: %s", res.Body)
	}
}

func TestAccountMailMessageHasNoInjectableHeaders(t *testing.T) {
	setup := accountMailSetup{from: "no-reply@example.test", domain: "example.test", host: "panel.example.test"}
	setup.Available = true
	if _, err := buildAccountMail(setup, "a@example.test\r\nBcc: x@example.test", accountMailContent{kind: accountMailReset}, time.Now()); err == nil {
		t.Fatal("a recipient with a line break was accepted")
	}
	message, err := buildAccountMail(setup, "a@example.test", accountMailContent{kind: accountMailInvitation, lang: "tr", username: "u", inviter: "i", link: "https://x/y"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	head, _, _ := strings.Cut(string(message), "\r\n\r\n")
	if !strings.Contains(head, "Subject: =?utf-8?q?") || !strings.Contains(head, "Auto-Submitted: auto-generated") {
		t.Fatalf("headers: %s", head)
	}
}
