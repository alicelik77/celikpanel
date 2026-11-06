package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/fileaccess"
	"github.com/alicelik/celikpanel/internal/passwordpolicy"
	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// The owner's password rule (6 Nov 2026) at every endpoint that sets a new
// password: a refusal is 400 PASSWORD_WEAK with the first broken rule as its
// reason and min/max as vars, and it never carries the password.

const (
	// Long enough for every field, but no uppercase letter or special.
	weakLowerOnly = "abcdefghijklmnop"
	// Too short for every field.
	weakTooShort = "Ab-1"
)

func requirePasswordRefusal(t *testing.T, recorder *httptest.ResponseRecorder, reason, minChars, maxBytes string, secret string) {
	t.Helper()
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", recorder.Code, recorder.Body.String())
	}
	var body apiErrorBody
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", recorder.Body.String(), err)
	}
	if body.Code != errCodePasswordWeak || body.Reason != reason {
		t.Fatalf("code=%q reason=%q, want %s/%s; body=%s", body.Code, body.Reason, errCodePasswordWeak, reason, recorder.Body.String())
	}
	if body.Vars["min"] != minChars || body.Vars["max"] != maxBytes {
		t.Fatalf("vars = %v, want min=%s max=%s", body.Vars, minChars, maxBytes)
	}
	if rules := strings.Split(body.Vars["rules"], ","); len(body.Details) != 0 || rules[0] != reason || body.Error == "" {
		t.Fatalf("details/error: %+v", body)
	}
	if secret != "" && strings.Contains(recorder.Body.String(), secret) {
		t.Fatalf("the refusal carries the password: %s", recorder.Body.String())
	}
}

func TestPasswordRefusalIsTheCodedEnvelope(t *testing.T) {
	refusal := passwordpolicy.CheckField("abcdefgh", "mailbox")
	if refusal == nil {
		t.Fatal("a lowercase-only password was accepted")
	}
	recorder := httptest.NewRecorder()
	writePasswordRefusal(recorder, refusal)
	requirePasswordRefusal(t, recorder, passwordpolicy.RuleNeedsUpper, "8", "1024", "abcdefgh")
	var body apiErrorBody
	_ = json.Unmarshal(recorder.Body.Bytes(), &body)
	if body.Vars["rules"] != "needs_upper,needs_special" || body.Error != refusal.Error() {
		t.Fatalf("body = %+v", body)
	}
	// Carried up an error chain, it is still found and written typed.
	wrapped := fmt.Errorf("validate: %w", error(refusal))
	recorder = httptest.NewRecorder()
	if !writePasswordRefusalIf(recorder, wrapped) {
		t.Fatal("a wrapped refusal was not found")
	}
	requirePasswordRefusal(t, recorder, passwordpolicy.RuleNeedsUpper, "8", "1024", "")
	if writePasswordRefusalIf(httptest.NewRecorder(), errors.New("other")) {
		t.Fatal("an unrelated error was answered as a password refusal")
	}
}

// The policy windows are the ones the services and the Agent already enforce
// on the wire, so a password the panel accepts is never refused further on.
func TestPasswordFieldsMatchTheWireBounds(t *testing.T) {
	check := func(field string, minChars, maxBytes int) {
		t.Helper()
		got := passwordpolicy.Fields[field]
		if got.MinChars != minChars || got.MaxBytes != maxBytes {
			t.Fatalf("%s = %+v, want %d..%d", field, got, minChars, maxBytes)
		}
	}
	check("mailbox", transport.MinMailboxPasswordBytes, transport.MaxMailboxPasswordBytes)
	check("database_user", transport.MinDatabaseUserPasswordBytes, transport.MaxDatabaseUserPasswordBytes)
	check("account", 8, 1024)
	check("file_access", 8, 128)
	check("site_protect", 8, 72)
	check("service_app_owner", 12, 128)

	longest := "Ab-" + strings.Repeat("x", 125)
	if passwordpolicy.CheckField(longest, "file_access") != nil || fileaccess.ValidatePassword(longest) != nil {
		t.Fatal("a 128-byte file access password is refused somewhere")
	}
	if passwordpolicy.CheckField("Ab-defgh", "mailbox") != nil || transport.ValidateMailboxPassword("Ab-defgh") != nil {
		t.Fatal("a compliant 8-character mailbox password is refused somewhere")
	}
}

func TestMailboxCreateAndChangeRefuseAWeakPassword(t *testing.T) {
	agent := &mailMutationPanelAgent{addApplied: true, passwordApplied: true}
	panel, domainID := newMailMutationPanel(t, agent)
	for _, test := range []struct{ password, reason string }{
		{weakLowerOnly, passwordpolicy.RuleNeedsUpper},
		{weakTooShort, passwordpolicy.RuleTooShort},
		{"ABCDEFGH-1", passwordpolicy.RuleNeedsLower},
		{"Abcdefgh1", passwordpolicy.RuleNeedsSpecial},
	} {
		body, _ := json.Marshal(map[string]any{"address": "user@example.com", "password": test.password, "quota_mb": 100})
		recorder := httptest.NewRecorder()
		panel.handleAddEmailAccount(recorder, httptest.NewRequest(http.MethodPost, "/mail/accounts", bytes.NewReader(body)), domainID)
		requirePasswordRefusal(t, recorder, test.reason, "8", "1024", test.password)
	}
	if calls := agent.addCallCount(); calls != 0 {
		t.Fatalf("the Agent was asked %d times", calls)
	}

	recorder := httptest.NewRecorder()
	panel.handleAddEmailAccount(recorder, httptest.NewRequest(http.MethodPost, "/mail/accounts",
		strings.NewReader(`{"address":"user@example.com","password":"Mailbox-pass1","quota_mb":100}`)), domainID)
	if recorder.Code != http.StatusOK || agent.addCallCount() != 1 {
		t.Fatalf("a compliant password: status=%d calls=%d body=%s", recorder.Code, agent.addCallCount(), recorder.Body.String())
	}

	var id int
	if err := panel.db.GetDB().QueryRow(`SELECT id FROM email_accounts WHERE address = 'user@example.com'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	recorder = httptest.NewRecorder()
	panel.handleUpdateEmailPassword(recorder, httptest.NewRequest(http.MethodPut, "/mail/accounts/password",
		strings.NewReader(fmt.Sprintf(`{"id":%d,"new_password":%q}`, id, weakLowerOnly))), domainID)
	requirePasswordRefusal(t, recorder, passwordpolicy.RuleNeedsUpper, "8", "1024", weakLowerOnly)
	if calls := agent.passwordCallState(); len(calls) != 0 {
		t.Fatalf("the Agent received a weak password change: %d", len(calls))
	}
}

func TestDatabaseUserPasswordsRefuseAWeakPassword(t *testing.T) {
	// Change: refused before anything is read.
	for _, test := range []struct{ password, reason string }{
		{weakLowerOnly, passwordpolicy.RuleNeedsUpper},
		{"Abcdef-1", passwordpolicy.RuleTooShort},
		{"Ab-" + strings.Repeat("x", 126), passwordpolicy.RuleTooLong},
	} {
		body, _ := json.Marshal(map[string]string{"password": test.password})
		recorder := httptest.NewRecorder()
		(&Panel{}).handleSetDatabaseUserPassword(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/database-users/1/password", bytes.NewReader(body)), 1)
		requirePasswordRefusal(t, recorder, test.reason, "12", "128", test.password)
	}
	if checkDatabaseUserPassword("Database-pass1") != nil {
		t.Fatal("a compliant database password was refused")
	}

	// Create with a new user, and create a user: nothing reaches the engine.
	fixture := newDatabaseV2SecurityFixture(t)
	driver := useRecordingDatabaseDriver(t)
	before := countDatabaseSecurityRows(t, fixture.sql)
	recorder := httptest.NewRecorder()
	fixture.panel.handleCreateDatabaseV2(recorder, fixture.request(fmt.Sprintf(
		`{"database_name":"newdb","domain_id":%d,"new_username":"new_user","new_password":%q}`, dbSecurityDomain, weakLowerOnly)))
	requirePasswordRefusal(t, recorder, passwordpolicy.RuleNeedsUpper, "12", "128", weakLowerOnly)

	recorder = httptest.NewRecorder()
	fixture.panel.handleCreateDatabaseV2User(recorder, fixture.requestAsOwner(http.MethodPost,
		fmt.Sprintf(`/api/v1/database-servers/%d/users`, dbSecurityServer),
		fmt.Sprintf(`{"username":"standalone","password":%q}`, weakTooShort)))
	requirePasswordRefusal(t, recorder, passwordpolicy.RuleTooShort, "12", "128", weakTooShort)
	requireDatabaseDriverEvents(t, driver)
	if after := countDatabaseSecurityRows(t, fixture.sql); after != before {
		t.Fatalf("a refused password changed rows: %v -> %v", before, after)
	}

	// A user made with the database on a domain's page.
	recorder = httptest.NewRecorder()
	(&Panel{}).handleCreateDatabase(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/domains/1/databases",
		strings.NewReader(fmt.Sprintf(`{"name":"shop","type":"mysql","password":%q}`, weakLowerOnly))), 1)
	requirePasswordRefusal(t, recorder, passwordpolicy.RuleNeedsUpper, "12", "128", weakLowerOnly)
}

func TestAccountPasswordsRefuseAWeakPassword(t *testing.T) {
	admin := &Caller{ID: 1, Role: roleAdmin}
	for _, test := range []struct{ password, reason string }{
		{weakLowerOnly, passwordpolicy.RuleNeedsUpper},
		{weakTooShort, passwordpolicy.RuleTooShort},
	} {
		payload := fmt.Sprintf(`{"username":"newcustomer","email":"a@example.com","password":%q,"role":"customer"}`, test.password)
		recorder := httptest.NewRecorder()
		(&Panel{}).handleCreateUser(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(payload)), admin)
		requirePasswordRefusal(t, recorder, test.reason, "8", "1024", test.password)

		recorder = httptest.NewRecorder()
		(&Panel{}).handleUpdateUser(recorder, httptest.NewRequest(http.MethodPut, "/api/v1/users/2",
			strings.NewReader(fmt.Sprintf(`{"password":%q}`, test.password))), &core.User{ID: 2, Username: "someone"})
		requirePasswordRefusal(t, recorder, test.reason, "8", "1024", test.password)

		// Changing one's own: only the new password meets the rule.
		recorder = httptest.NewRecorder()
		(&Panel{}).handleChangeOwnPassword(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/auth/password",
			strings.NewReader(fmt.Sprintf(`{"current_password":"old","new_password":%q}`, test.password))))
		requirePasswordRefusal(t, recorder, test.reason, "8", "1024", test.password)
	}
	if checkAccountPassword("Account-pass") != nil {
		t.Fatal("a compliant account password was refused")
	}
}

func TestTeamMemberPasswordsRefuseAWeakPassword(t *testing.T) {
	p := newDNSPanelForTest(t)
	ownerID := createTeamMemberHandlerUser(t, p, roleCustomer, core.AccountTypeAccount, nil, "policy-owner")
	subscriptionID, domainID := createTeamMemberHandlerScope(t, p, ownerID, "policy-owner")
	caller := &Caller{ID: ownerID, Role: roleCustomer, AccountType: core.AccountTypeAccount, CustomerID: ownerID}

	weakBody := strings.Replace(teamMemberCreateBody(nil, "policy-member", subscriptionID, domainID),
		`"password":"Handler-password"`, `"password":"`+weakLowerOnly+`"`, 1)
	if !strings.Contains(weakBody, weakLowerOnly) {
		t.Fatalf("fixture body changed: %s", weakBody)
	}
	recorder := httptest.NewRecorder()
	p.handleTeamMembers(recorder, teamMemberHandlerRequest(http.MethodPost, "/api/v1/team-members", weakBody, caller))
	requirePasswordRefusal(t, recorder, passwordpolicy.RuleNeedsUpper, "8", "1024", weakLowerOnly)

	recorder = httptest.NewRecorder()
	p.handleTeamMembers(recorder, teamMemberHandlerRequest(http.MethodPost, "/api/v1/team-members",
		teamMemberCreateBody(nil, "policy-member", subscriptionID, domainID), caller))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("a compliant password: %d %s", recorder.Code, recorder.Body.String())
	}
	member := decodeTeamMemberHandlerResponse(t, recorder)

	recorder = httptest.NewRecorder()
	p.handleTeamMemberByID(recorder, teamMemberHandlerRequest(http.MethodPut,
		fmt.Sprintf("/api/v1/team-members/%d", member.ID), fmt.Sprintf(`{"password":%q}`, weakTooShort), caller))
	requirePasswordRefusal(t, recorder, passwordpolicy.RuleTooShort, "8", "1024", weakTooShort)

	// The validators return the typed refusal itself.
	var refusal *passwordpolicy.Error
	if err := validateTeamMemberIdentity("valid-name", "a@example.com", weakLowerOnly, "active", true); !errors.As(err, &refusal) {
		t.Fatalf("identity: %v", err)
	}
	if err := validateTeamMemberIdentity("valid-name", "a@example.com", "", "active", false); err != nil {
		t.Fatalf("an invited member's password is not checked: %v", err)
	}
}

func TestAdminCLIRefusesAWeakPasswordAndNamesTheRule(t *testing.T) {
	err := validateAdminCredentials(adminCredentials{username: "first-admin", email: "admin@example.test", password: weakLowerOnly})
	if err == nil || !strings.Contains(err.Error(), "uppercase") || strings.Contains(err.Error(), weakLowerOnly) {
		t.Fatalf("weak admin password: %v", err)
	}
	if err := validateAdminCredentials(adminCredentials{username: "first-admin", email: "admin@example.test", password: "Admin-pass1"}); err != nil {
		t.Fatalf("compliant admin password: %v", err)
	}

	answers := []string{weakLowerOnly, "Admin-pass1", "Admin-pass1"}
	var out bytes.Buffer
	got, err := readAndConfirmPassword(func(string) (string, error) {
		answer := answers[0]
		answers = answers[1:]
		return answer, nil
	}, &out)
	if err != nil || got != "Admin-pass1" {
		t.Fatalf("got %q, %v", got, err)
	}
	if !strings.Contains(out.String(), "uppercase letter") || !strings.Contains(out.String(), "büyük harf") ||
		strings.Contains(out.String(), weakLowerOnly) {
		t.Fatalf("prompt output: %q", out.String())
	}
}

func TestFileAccessPasswordRefusesAWeakPassword(t *testing.T) {
	agent := &fileAccessPanelAgent{capabilities: []string{transport.AgentCapabilityFileAccessV1}}
	panel := newFileAccessPanel(t, agent)
	response := fileAccessRequest(panel, http.MethodPut, 7103, false, fmt.Sprintf(`{"enabled":true,"password":%q}`, weakLowerOnly))
	requirePasswordRefusal(t, response, passwordpolicy.RuleNeedsUpper, "8", "128", weakLowerOnly)
	response = fileAccessRequest(panel, http.MethodPut, 7103, false, fmt.Sprintf(`{"enabled":true,"password":%q}`, weakTooShort))
	requirePasswordRefusal(t, response, passwordpolicy.RuleTooShort, "8", "128", weakTooShort)
	if applied := agent.applied(); len(applied) != 0 {
		t.Fatalf("the Agent applied %d changes", len(applied))
	}
}

func TestProtectedFolderPasswordRefusesAWeakPassword(t *testing.T) {
	p, domainID, owner, applied := newSiteFeatureFixture(t)
	r := featureRequest(p, domainID, owner, http.MethodPost, "protected-dirs",
		fmt.Sprintf(`{"path":"/private/","username":"alice","password":%q}`, weakLowerOnly))
	requirePasswordRefusal(t, r, passwordpolicy.RuleNeedsUpper, "8", "72", weakLowerOnly)
	r = featureRequest(p, domainID, owner, http.MethodPost, "protected-dirs",
		`{"path":"/private/","username":"alice","password":"Ab-`+strings.Repeat("x", 70)+`"}`)
	requirePasswordRefusal(t, r, passwordpolicy.RuleTooLong, "8", "72", "")
	r = featureRequest(p, domainID, owner, http.MethodPost, "protected-dirs",
		`{"path":"/private/","username":"alice","password":"short"}`)
	requirePasswordRefusal(t, r, passwordpolicy.RuleTooShort, "8", "72", "")
	if len(applied.requests) != 0 {
		t.Fatal("a refused password reached the web server")
	}
	if _, err := hashSiteProtectPassword("Folder-pass1"); err != nil {
		t.Fatalf("a compliant password: %v", err)
	}
}

func TestServiceAppOwnerPasswordRefusesAWeakPassword(t *testing.T) {
	f := newServiceAppFixture(t)
	f.offer(t, "n8n")
	path := "/api/v1/domains/9903/service-apps/install"
	body := strings.Replace(n8nInstallBody, `"owner_password":"Correct horse battery"`, `"owner_password":"`+weakLowerOnly+`"`, 1)
	if !strings.Contains(body, weakLowerOnly) {
		t.Fatalf("fixture body changed: %s", body)
	}
	requirePasswordRefusal(t, f.as(t, 9704, http.MethodPost, path, body), passwordpolicy.RuleNeedsUpper, "12", "128", weakLowerOnly)
	f.native.mu.Lock()
	prepares := len(f.native.prepares)
	f.native.mu.Unlock()
	if prepares != 0 {
		t.Fatalf("a refused owner password started %d installs", prepares)
	}
	if refusal, ok := checkOwnerPassword("Owner-password1"); refusal != nil || !ok {
		t.Fatalf("a compliant owner password: %v %v", refusal, ok)
	}
	if refusal, _ := checkOwnerPassword("Owner-pass1"); refusal == nil || refusal.Reason() != passwordpolicy.RuleTooShort {
		t.Fatalf("an 11-character owner password: %v", refusal)
	}
}

// Passwords CelikPanel makes up and shows meet the rule themselves.
func TestGeneratedPasswordsMeetTheRule(t *testing.T) {
	for i := 0; i < 200; i++ {
		db, err := randomDBPassword()
		if err != nil || len(db) != 24 || checkDatabaseUserPassword(db) != nil {
			t.Fatalf("randomDBPassword %q: %v", db, err)
		}
		admin, err := generateDatabaseAdminPassword()
		if err != nil || len(admin) != databaseAdminAccountPasswordLength || passwordpolicy.Check(admin, 12, 128) != nil ||
			strings.Trim(admin, databaseAdminAccountAlphabet) != "" {
			t.Fatalf("generateDatabaseAdminPassword %q: %v", admin, err)
		}
		minted, err := services.GeneratePassword(16)
		if err != nil || len(minted) != 16 || checkDatabaseUserPassword(minted) != nil {
			t.Fatalf("services.GeneratePassword %q: %v", minted, err)
		}
	}
}
