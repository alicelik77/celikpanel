package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/alicelik/celikpanel/internal/core"
	paneldb "github.com/alicelik/celikpanel/internal/db"
)

// withCaller makes the request a signed-in customer's own.
func withCaller(request *http.Request, userID int) *http.Request {
	return request.WithContext(context.WithValue(request.Context(), callerKey, &Caller{
		ID: userID, Role: roleCustomer, AccountType: core.AccountTypeAccount, CustomerID: userID,
	}))
}

const recoveryTestPassword = `correct horse battery staple`

var recoveryCodeShape = regexp.MustCompile(`^[A-HJ-NP-Z2-9]{4}(-[A-HJ-NP-Z2-9]{4}){3}$`)

type recoveryEnabled struct {
	panel  *Panel
	db     *paneldb.SQLiteDB
	userID int
	secret string
	codes  []string
}

// enableTwoFactorWithRecoveryCodes goes through the real setup and enable
// handlers, so the codes are the ones a person is shown.
func enableTwoFactorWithRecoveryCodes(t *testing.T) recoveryEnabled {
	t.Helper()
	isolatePendingLogins(t)
	panel, database, user := newTOTPSecurityPanel(t)
	setup := httptest.NewRecorder()
	panel.handle2FASetup(setup, httptest.NewRequest(http.MethodPost, `/api/v1/auth/2fa/setup`,
		strings.NewReader(fmt.Sprintf(`{"password":%q}`, recoveryTestPassword))), user.ID)
	if setup.Code != http.StatusOK {
		t.Fatalf(`setup status = %d; body=%s`, setup.Code, setup.Body.String())
	}
	var setupBody struct {
		Secret string `json:"secret"`
	}
	if err := json.NewDecoder(setup.Body).Decode(&setupBody); err != nil {
		t.Fatal(err)
	}
	enable := httptest.NewRecorder()
	panel.handle2FAEnable(enable, httptest.NewRequest(http.MethodPost, `/api/v1/auth/2fa/enable`,
		strings.NewReader(fmt.Sprintf(`{"password":%q,"code":%q}`, recoveryTestPassword, totpCodeForSecurityTest(t, setupBody.Secret)))), user.ID)
	if enable.Code != http.StatusOK {
		t.Fatalf(`enable status = %d; body=%s`, enable.Code, enable.Body.String())
	}
	var enableBody struct {
		Codes     []string `json:"recovery_codes"`
		Remaining int      `json:"recovery_remaining"`
		Total     int      `json:"recovery_total"`
	}
	if err := json.NewDecoder(enable.Body).Decode(&enableBody); err != nil {
		t.Fatal(err)
	}
	if len(enableBody.Codes) != recoveryCodeCount || enableBody.Remaining != 10 || enableBody.Total != 10 {
		t.Fatalf(`enable gave %d codes (remaining %d, total %d), want 10`, len(enableBody.Codes), enableBody.Remaining, enableBody.Total)
	}
	return recoveryEnabled{panel: panel, db: database, userID: user.ID, secret: setupBody.Secret, codes: enableBody.Codes}
}

func (e recoveryEnabled) signInWithRecovery(t *testing.T, code string) *httptest.ResponseRecorder {
	t.Helper()
	state, err := e.panel.userAuthState(t.Context(), e.userID)
	if err != nil {
		t.Fatal(err)
	}
	token, err := newPendingToken(state)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	e.panel.handleLoginTOTP(recorder, httptest.NewRequest(http.MethodPost, `/api/v1/auth/login/totp`,
		strings.NewReader(fmt.Sprintf(`{"pending_token":%q,"recovery_code":%q}`, token, code))))
	return recorder
}

func (e recoveryEnabled) counts(t *testing.T) (remaining, total int) {
	t.Helper()
	remaining, total, err := e.panel.recoveryCodeCounts(t.Context(), e.userID)
	if err != nil {
		t.Fatal(err)
	}
	return remaining, total
}

func (e recoveryEnabled) auditCount(t *testing.T, action string) int {
	t.Helper()
	var count int
	if err := e.db.GetDB().QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action LIKE ?`, action+`%`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestEnablingTwoFactorMakesTenSingleUseCodesStoredOnlyAsDigests(t *testing.T) {
	e := enableTwoFactorWithRecoveryCodes(t)
	seen := map[string]bool{}
	for _, code := range e.codes {
		if !recoveryCodeShape.MatchString(code) {
			t.Fatalf(`code %q does not have the shape XXXX-XXXX-XXXX-XXXX`, code)
		}
		if seen[code] {
			t.Fatalf(`code %q was made twice`, code)
		}
		seen[code] = true
	}
	if remaining, total := e.counts(t); remaining != 10 || total != 10 {
		t.Fatalf(`counts = %d/%d, want 10/10`, remaining, total)
	}
	rows, err := e.db.GetDB().Query(`SELECT code_hash FROM two_factor_recovery_codes WHERE user_id = ?`, e.userID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var hash string
		if err := rows.Scan(&hash); err != nil {
			t.Fatal(err)
		}
		if len(hash) != 64 || seen[hash] {
			t.Fatalf(`stored value %q is not a digest`, hash)
		}
		for _, code := range e.codes {
			normalized, _ := normalizeRecoveryCode(code)
			if hash == normalized || strings.Contains(hash, strings.ToLower(normalized)) {
				t.Fatalf(`a plain code is stored`)
			}
		}
	}
	if e.auditCount(t, `2fa.recovery.generate`) != 1 {
		t.Fatal(`making the codes is not audited`)
	}
}

func TestRecoveryCodeSignsInOnceAndEachUseIsAudited(t *testing.T) {
	e := enableTwoFactorWithRecoveryCodes(t)
	code := e.codes[0]

	first := e.signInWithRecovery(t, code)
	if first.Code != http.StatusOK {
		t.Fatalf(`recovery sign-in status = %d; body=%s`, first.Code, first.Body.String())
	}
	token := sessionTokenFromRecorder(t, first)
	requireSessionValidForUser(t, e.panel, token, e.userID)
	if remaining, total := e.counts(t); remaining != 9 || total != 10 {
		t.Fatalf(`counts after one use = %d/%d, want 9/10`, remaining, total)
	}
	if e.auditCount(t, `auth.login.2fa.recovery`) != 1 {
		t.Fatal(`the use of a recovery code is not audited`)
	}

	// The same code does not work a second time, in any spelling.
	for _, again := range []string{code, strings.ToLower(code), strings.ReplaceAll(code, `-`, ``), ` ` + code + ` `} {
		if second := e.signInWithRecovery(t, again); second.Code != http.StatusUnauthorized {
			t.Fatalf(`a spent code (%q) gave status %d, want 401`, again, second.Code)
		}
	}
	if remaining, _ := e.counts(t); remaining != 9 {
		t.Fatalf(`a refused attempt changed the count to %d`, remaining)
	}

	// Another code of the set still works, typed in lower case without dashes.
	other := strings.ToLower(strings.ReplaceAll(e.codes[1], `-`, ``))
	if third := e.signInWithRecovery(t, other); third.Code != http.StatusOK {
		t.Fatalf(`second code status = %d; body=%s`, third.Code, third.Body.String())
	}
	if remaining, _ := e.counts(t); remaining != 8 {
		t.Fatalf(`remaining = %d, want 8`, remaining)
	}
	if e.auditCount(t, `auth.login.2fa.recovery`) != 2 {
		t.Fatal(`the second use is not audited`)
	}
}

func TestRecoveryCodeRefusesWhatIsNotACodeOfTheAccount(t *testing.T) {
	e := enableTwoFactorWithRecoveryCodes(t)
	for _, wrong := range []string{
		`AAAA-AAAA-AAAA-AAAA`, `0000-0000-0000-0000`, `IIII-OOOO-IIII-OOOO`, `short`,
		e.codes[0][:len(e.codes[0])-1], e.codes[0] + `A`, `123456`, `'; DROP TABLE users;--`,
	} {
		if recorder := e.signInWithRecovery(t, wrong); recorder.Code != http.StatusUnauthorized {
			t.Fatalf(`%q gave status %d, want 401`, wrong, recorder.Code)
		}
	}
	if remaining, _ := e.counts(t); remaining != 10 {
		t.Fatalf(`wrong codes spent %d codes`, 10-remaining)
	}
}

func TestRecoveryCodeIsSpentByOnlyOneOfTwoSimultaneousUses(t *testing.T) {
	e := enableTwoFactorWithRecoveryCodes(t)
	var wg sync.WaitGroup
	results := make(chan bool, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			spent, err := e.panel.consumeRecoveryCode(t.Context(), e.userID, e.codes[3])
			results <- err == nil && spent
		}()
	}
	wg.Wait()
	close(results)
	won := 0
	for spent := range results {
		if spent {
			won++
		}
	}
	if won != 1 {
		t.Fatalf(`%d simultaneous uses spent the code, want exactly 1`, won)
	}
}

func TestRegeneratingRecoveryCodesNeedsPasswordAndAuthenticatorCodeAndKillsTheOldSet(t *testing.T) {
	e := enableTwoFactorWithRecoveryCodes(t)
	// One code is spent first, so the new set visibly starts at 10 again.
	if recorder := e.signInWithRecovery(t, e.codes[0]); recorder.Code != http.StatusOK {
		t.Fatalf(`sign-in status = %d`, recorder.Code)
	}

	post := func(body string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		e.panel.handle2FA(recorder, withCaller(httptest.NewRequest(http.MethodPost,
			`/api/v1/auth/2fa/recovery/regenerate`, strings.NewReader(body)), e.userID))
		return recorder
	}
	for name, body := range map[string]string{
		`wrong password`: fmt.Sprintf(`{"password":"nope","code":%q}`, totpCodeForSecurityTest(t, e.secret)),
		`wrong code`:     fmt.Sprintf(`{"password":%q,"code":"000000"}`, recoveryTestPassword),
		`no code`:        fmt.Sprintf(`{"password":%q}`, recoveryTestPassword),
		`a recovery code instead of the authenticator code`: fmt.Sprintf(`{"password":%q,"code":%q}`, recoveryTestPassword, e.codes[1]),
	} {
		if recorder := post(body); recorder.Code != http.StatusUnauthorized {
			t.Fatalf(`%s: status = %d, want 401; body=%s`, name, recorder.Code, recorder.Body.String())
		}
	}
	if remaining, _ := e.counts(t); remaining != 9 {
		t.Fatalf(`a refused regeneration changed the count to %d`, remaining)
	}

	recorder := post(fmt.Sprintf(`{"password":%q,"code":%q}`, recoveryTestPassword, totpCodeForSecurityTest(t, e.secret)))
	if recorder.Code != http.StatusOK {
		t.Fatalf(`regenerate status = %d; body=%s`, recorder.Code, recorder.Body.String())
	}
	var body struct {
		Codes []string `json:"recovery_codes"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Codes) != 10 {
		t.Fatalf(`new set has %d codes`, len(body.Codes))
	}
	if remaining, total := e.counts(t); remaining != 10 || total != 10 {
		t.Fatalf(`counts after regeneration = %d/%d, want 10/10`, remaining, total)
	}
	if e.auditCount(t, `2fa.recovery.regenerate`) != 1 {
		t.Fatal(`regenerating is not audited`)
	}
	// Old unspent codes no longer work; new ones do.
	if recorder := e.signInWithRecovery(t, e.codes[2]); recorder.Code != http.StatusUnauthorized {
		t.Fatalf(`an old code gave status %d, want 401`, recorder.Code)
	}
	if recorder := e.signInWithRecovery(t, body.Codes[0]); recorder.Code != http.StatusOK {
		t.Fatalf(`a new code gave status %d; body=%s`, recorder.Code, recorder.Body.String())
	}
}

func TestStatusShowsTheRemainingCodesAndDisablingRemovesThem(t *testing.T) {
	e := enableTwoFactorWithRecoveryCodes(t)
	if recorder := e.signInWithRecovery(t, e.codes[0]); recorder.Code != http.StatusOK {
		t.Fatalf(`sign-in status = %d`, recorder.Code)
	}
	status := httptest.NewRecorder()
	e.panel.handle2FA(status, withCaller(httptest.NewRequest(http.MethodGet, `/api/v1/auth/2fa/status`, nil), e.userID))
	var body struct {
		Enabled   bool `json:"enabled"`
		Remaining *int `json:"recovery_remaining"`
		Total     *int `json:"recovery_total"`
	}
	if err := json.NewDecoder(status.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if !body.Enabled || body.Remaining == nil || *body.Remaining != 9 || body.Total == nil || *body.Total != 10 {
		t.Fatalf(`status = %+v, want enabled with 9 of 10 left`, body)
	}

	disable := httptest.NewRecorder()
	e.panel.handle2FADisable(disable, httptest.NewRequest(http.MethodPost, `/api/v1/auth/2fa/disable`,
		strings.NewReader(fmt.Sprintf(`{"password":%q,"code":%q}`, recoveryTestPassword, totpCodeForSecurityTest(t, e.secret)))), e.userID)
	if disable.Code != http.StatusOK {
		t.Fatalf(`disable status = %d; body=%s`, disable.Code, disable.Body.String())
	}
	var rows int
	if err := e.db.GetDB().QueryRow(`SELECT COUNT(*) FROM two_factor_recovery_codes WHERE user_id = ?`, e.userID).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Fatalf(`disabling left %d recovery codes`, rows)
	}
	// With 2FA off, a recovery code opens nothing.
	if recorder := e.signInWithRecovery(t, e.codes[1]); recorder.Code == http.StatusOK {
		t.Fatal(`a recovery code signed in after 2FA was turned off`)
	}
	// Regeneration is for an account whose 2FA is on.
	regen := httptest.NewRecorder()
	e.panel.handle2FARecoveryRegenerate(regen, httptest.NewRequest(http.MethodPost, `/api/v1/auth/2fa/recovery/regenerate`,
		strings.NewReader(fmt.Sprintf(`{"password":%q,"code":"000000"}`, recoveryTestPassword))), e.userID)
	if regen.Code != http.StatusConflict {
		t.Fatalf(`regenerate with 2FA off gave status %d, want 409`, regen.Code)
	}
}

func TestRecoveryCodeSpellings(t *testing.T) {
	for raw, want := range map[string]string{
		`ABCD-EFGH-JKLM-NPQR`: `ABCDEFGHJKLMNPQR`,
		`abcd efgh jklm npqr`: `ABCDEFGHJKLMNPQR`,
		` abcdefghjklmnpqr `:  `ABCDEFGHJKLMNPQR`,
	} {
		got, ok := normalizeRecoveryCode(raw)
		if !ok || got != want {
			t.Fatalf(`normalize(%q) = %q, %v; want %q`, raw, got, ok, want)
		}
	}
	for _, raw := range []string{``, `ABCD`, `ABCD-EFGH-JKLM-NPQ0`, `ABCD-EFGH-JKLM-NPQI`, `ABCD-EFGH-JKLM-NPQRS`, "ABCD\nEFGH-JKLM-NPQR", `ÄBCD-EFGH-JKLM-NPQR`} {
		if got, ok := normalizeRecoveryCode(raw); ok {
			t.Fatalf(`normalize(%q) accepted as %q`, raw, got)
		}
	}
}
