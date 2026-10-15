package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

func markServerSetupReadyForTest(t *testing.T, f serviceOperationTestFixture) serverSetupState {
	t.Helper()
	if _, err := f.database.GetDB().Exec(`UPDATE server_setup_state SET status='ready',completed_at='2026-10-15T10:00:00Z' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	state, err := f.panel.loadServerSetup(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func postSetupExtendForTest(t *testing.T, f serviceOperationTestFixture, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	raw, _ := json.Marshal(body)
	w := httptest.NewRecorder()
	f.panel.handleServerSetupExtend(w, serviceOperationAdminRequest(t, http.MethodPost, "/api/v1/setup/extend", string(raw), f.userID))
	return w
}

func setupErrorCode(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	return body.Code
}

func TestServerSetupExtendKeepsTheCompletedSetupAndItsBase(t *testing.T) {
	f, _ := setupOperationFixture(t)
	ready := markServerSetupReadyForTest(t, f)
	// Negative control: a completed setup is neither an extension nor required.
	if ready.Extension || ready.Required || ready.ExtensionBase != nil {
		t.Fatalf("completed setup read as an extension: %+v", ready)
	}
	w := postSetupExtendForTest(t, f, map[string]any{"revision": ready.Revision, "action": "begin"})
	if w.Code != http.StatusOK {
		t.Fatalf("begin: %d %s", w.Code, w.Body.String())
	}
	state, err := f.panel.loadServerSetup(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != "draft" || !state.Extension || state.Required || state.Revision != ready.Revision+1 || state.CompletedAt != ready.CompletedAt {
		t.Fatalf("extension state: %+v", state)
	}
	if state.ExtensionBase == nil || !serverSetupDraftsEqual(*state.ExtensionBase, ready.Draft) || !serverSetupDraftsEqual(state.Draft, ready.Draft) {
		t.Fatalf("extension base: %+v", state.ExtensionBase)
	}
	var answer serverSetupState
	if err := json.Unmarshal(w.Body.Bytes(), &answer); err != nil || !answer.Extension || answer.ExtensionBase == nil {
		t.Fatalf("answer: %s", w.Body.String())
	}
	if f.agent.installCalls.Load() != 0 || len(f.agent.capturedMutationEvents()) != 0 {
		t.Fatal("beginning an extension changed the host")
	}

	// The owner adds something, then stops: the completed draft comes back.
	changed := state.Draft
	changed.Purpose = "web_mail"
	changed.MailHostname = "mail.example.test"
	state, err = f.panel.saveServerSetupDraft(context.Background(), state.Revision, changed)
	if err != nil || !state.Extension || state.ExtensionBase == nil || state.ExtensionBase.Purpose != "web" {
		t.Fatalf("draft saved during extension: %+v %v", state, err)
	}
	w = postSetupExtendForTest(t, f, map[string]any{"revision": state.Revision, "action": "cancel"})
	if w.Code != http.StatusOK {
		t.Fatalf("cancel: %d %s", w.Code, w.Body.String())
	}
	state, err = f.panel.loadServerSetup(context.Background())
	if err != nil || state.Status != "ready" || state.Extension || state.Draft.Purpose != "web" || state.Draft.MailHostname != "" || state.CompletedAt != ready.CompletedAt {
		t.Fatalf("stopped extension: %+v %v", state, err)
	}
	if f.panel.setting(context.Background(), settingServerSetupExtensionBase) != "" {
		t.Fatal("stopped extension left its base behind")
	}
}

func TestServerSetupExtendRefusals(t *testing.T) {
	t.Run("first setup not finished", func(t *testing.T) {
		f, state := setupOperationFixture(t)
		w := postSetupExtendForTest(t, f, map[string]any{"revision": state.Revision, "action": "begin"})
		if w.Code != http.StatusConflict || setupErrorCode(t, w) != errCodeServerSetupExtendNotReady {
			t.Fatalf("not ready: %d %s", w.Code, w.Body.String())
		}
		// The same request on a completed setup is accepted (control).
		ready := markServerSetupReadyForTest(t, f)
		if w := postSetupExtendForTest(t, f, map[string]any{"revision": ready.Revision, "action": "begin"}); w.Code != http.StatusOK {
			t.Fatalf("control: %d %s", w.Code, w.Body.String())
		}
	})
	t.Run("stale revision", func(t *testing.T) {
		f, _ := setupOperationFixture(t)
		ready := markServerSetupReadyForTest(t, f)
		w := postSetupExtendForTest(t, f, map[string]any{"revision": ready.Revision - 1, "action": "begin"})
		if w.Code != http.StatusConflict || setupErrorCode(t, w) != "setup_conflict" {
			t.Fatalf("stale: %d %s", w.Code, w.Body.String())
		}
	})
	t.Run("setup run active", func(t *testing.T) {
		f, _ := setupOperationFixture(t)
		ready := markServerSetupReadyForTest(t, f)
		plan := saveSetupPlanForTest(t, f, ready)
		if _, err := f.database.GetDB().Exec(`INSERT INTO server_setup_executions(id,request_id,plan_id,status,execution_json,created_at,updated_at) VALUES(?,?,?,'running','{}','now','now')`, strings.Repeat("5", 32), strings.Repeat("5", 32), plan.ID); err != nil {
			t.Fatal(err)
		}
		w := postSetupExtendForTest(t, f, map[string]any{"revision": ready.Revision, "action": "begin"})
		if w.Code != http.StatusConflict || setupErrorCode(t, w) != "server_setup_busy" {
			t.Fatalf("busy: %d %s", w.Code, w.Body.String())
		}
		state, _ := f.panel.loadServerSetup(context.Background())
		if state.Status != "ready" || state.Revision != ready.Revision {
			t.Fatalf("busy refusal changed the state: %+v", state)
		}
	})
	t.Run("stop without extension", func(t *testing.T) {
		f, _ := setupOperationFixture(t)
		ready := markServerSetupReadyForTest(t, f)
		w := postSetupExtendForTest(t, f, map[string]any{"revision": ready.Revision, "action": "cancel"})
		if w.Code != http.StatusConflict || setupErrorCode(t, w) != errCodeServerSetupExtendNotActive {
			t.Fatalf("cancel: %d %s", w.Code, w.Body.String())
		}
	})
	t.Run("invalid action", func(t *testing.T) {
		f, _ := setupOperationFixture(t)
		ready := markServerSetupReadyForTest(t, f)
		if w := postSetupExtendForTest(t, f, map[string]any{"revision": ready.Revision, "action": "remove"}); w.Code != http.StatusBadRequest {
			t.Fatalf("invalid action: %d", w.Code)
		}
	})
}

func TestServerSetupExtendAddsAMailProfileWithItsHostName(t *testing.T) {
	f, _ := setupOperationFixture(t)
	ready := markServerSetupReadyForTest(t, f)
	// No name given and none saved: the dialog has to ask for one.
	w := postSetupExtendForTest(t, f, map[string]any{"revision": ready.Revision, "action": "begin", "mail_profile": "core-mail"})
	if w.Code != http.StatusBadRequest || setupErrorCode(t, w) != errCodeMailProfileServerHostnameRequired {
		t.Fatalf("missing name: %d %s", w.Code, w.Body.String())
	}
	w = postSetupExtendForTest(t, f, map[string]any{"revision": ready.Revision, "action": "begin", "mail_profile": "core-mail", "mail_hostname": "not a name"})
	if w.Code != http.StatusBadRequest || setupErrorCode(t, w) != errCodeMailProfileServerHostnameInvalid {
		t.Fatalf("invalid name: %d %s", w.Code, w.Body.String())
	}
	if state, _ := f.panel.loadServerSetup(context.Background()); state.Status != "ready" || state.Revision != ready.Revision {
		t.Fatalf("refused request changed the state: %+v", state)
	}
	if err := f.panel.setSetting(context.Background(), settingMailHostname, "mail.example.test"); err != nil {
		t.Fatal(err)
	}
	w = postSetupExtendForTest(t, f, map[string]any{"revision": ready.Revision, "action": "begin", "mail_profile": "core-mail"})
	if w.Code != http.StatusOK {
		t.Fatalf("saved name: %d %s", w.Code, w.Body.String())
	}
	state, _ := f.panel.loadServerSetup(context.Background())
	if state.Draft.MailHostname != "mail.example.test" || !slices.Equal(serverSetupMailProfileIDs(state.Draft), []string{"core-mail"}) {
		t.Fatalf("mail draft: %+v", state.Draft)
	}
	// A second request on the extension corrects the name only.
	w = postSetupExtendForTest(t, f, map[string]any{"revision": state.Revision, "action": "begin", "mail_profile": "core-mail", "mail_hostname": "Smtp.Example.Test."})
	if w.Code != http.StatusOK {
		t.Fatalf("rename: %d %s", w.Code, w.Body.String())
	}
	state, _ = f.panel.loadServerSetup(context.Background())
	if state.Draft.MailHostname != "smtp.example.test" || state.ExtensionBase == nil || state.ExtensionBase.MailHostname != "" {
		t.Fatalf("renamed draft: %+v base %+v", state.Draft, state.ExtensionBase)
	}
}

func TestServerSetupDraftWithMailProfile(t *testing.T) {
	web := serverSetupDraft{Purpose: "web", DNSMode: "external", Database: "mariadb"}
	core, err := serverSetupDraftWithMailProfile(web, "core-mail")
	if err != nil || core.Customization == nil || !slices.Equal(core.Customization.Components, []string{"dovecot", "mariadb", "nginx", "php-fpm", "postfix"}) {
		t.Fatalf("web + core mail: %+v %v", core.Customization, err)
	}
	if got := serverSetupMailProfileIDs(core); !slices.Equal(got, []string{"core-mail"}) {
		t.Fatalf("profiles: %v", got)
	}
	webmail, _ := serverSetupDraftWithMailProfile(core, "webmail")
	if got := serverSetupMailProfileIDs(webmail); !slices.Equal(got, []string{"webmail"}) {
		t.Fatalf("webmail profiles: %v", got)
	}
	// Already covered: unchanged (the run repairs the profile).
	full := serverSetupDraft{Purpose: "web_mail", DNSMode: "external", Database: "mariadb"}
	same, err := serverSetupDraftWithMailProfile(full, "core-mail")
	if err != nil || same.Customization != nil || same.Purpose != "web_mail" {
		t.Fatalf("covered profile changed the draft: %+v %v", same, err)
	}
	dns := serverSetupDraft{Purpose: "dns", DNSMode: "local", DNSEngine: "pdns", DNSRole: "secondary"}
	mail, err := serverSetupDraftWithMailProfile(dns, "protected-mail")
	if err != nil || mail.Purpose != "custom" || !slices.Equal(mail.Customization.Components, []string{"dovecot", "postfix", "rspamd"}) {
		t.Fatalf("dns + protected mail: %+v %v", mail, err)
	}
	if _, err := serverSetupDraftWithMailProfile(web, "nothing"); err == nil {
		t.Fatal("unknown profile accepted")
	}
}

func TestServerSetupExtensionMarkerFollowsTheCompletionRecord(t *testing.T) {
	f, state := setupOperationFixture(t)
	// A first setup in draft is required, not an extension (control).
	if state.Extension || !state.Required {
		t.Fatalf("first setup: %+v", state)
	}
	ready := markServerSetupReadyForTest(t, f)
	// A draft saved over a completed setup (the existing PUT) extends it too.
	next := ready.Draft
	next.NodeVersion = "22.11.0"
	state, err := f.panel.saveServerSetupDraft(context.Background(), ready.Revision, next)
	if err != nil || !state.Extension || state.Required || state.ExtensionBase == nil || state.ExtensionBase.NodeVersion != "" {
		t.Fatalf("PUT over a completed setup: %+v %v", state, err)
	}
	// Completing again ends the extension and clears its base.
	f.panel.serverSetupProbe = func(context.Context, serverSetupDraft) ([]serverSetupCheck, error) {
		return []serverSetupCheck{{ID: "panel_https", State: "ready", Code: "ready"}}, nil
	}
	if err := f.panel.completeServerSetup(context.Background(), state.Revision); err != nil {
		t.Fatal(err)
	}
	state, _ = f.panel.loadServerSetup(context.Background())
	if state.Status != "ready" || state.Extension || f.panel.setting(context.Background(), settingServerSetupExtensionBase) != "" {
		t.Fatalf("completed extension: %+v", state)
	}
}

// Reopening the plan of an extension run keeps it an extension: the completion
// record survives, so the server is never sent back into a first setup.
func TestServerSetupReviseKeepsAnExtension(t *testing.T) {
	f, _ := setupOperationFixture(t)
	ready := markServerSetupReadyForTest(t, f)
	if w := postSetupExtendForTest(t, f, map[string]any{"revision": ready.Revision, "action": "begin"}); w.Code != http.StatusOK {
		t.Fatalf("begin: %d %s", w.Code, w.Body.String())
	}
	state, _ := f.panel.loadServerSetup(context.Background())
	plan := saveSetupPlanForTest(t, f, state)
	execution := serverSetupExecution{ID: strings.Repeat("4", 32), RequestID: strings.Repeat("4", 32), PlanID: plan.ID, Status: "waiting", Phase: "verification"}
	for _, step := range plan.Steps {
		execution.Steps = append(execution.Steps, serverSetupExecutionStep{serverSetupPlanStep: step, Status: "succeeded", RequestID: serverSetupID(execution.ID, step.ID, "request"), OwnerID: serverSetupID(execution.ID, step.ID, "owner")})
	}
	execution.Steps[len(execution.Steps)-1].Status = "pending"
	raw, _ := json.Marshal(execution)
	if _, err := f.database.GetDB().Exec(`INSERT INTO server_setup_executions(id,request_id,plan_id,status,execution_json,created_at,updated_at) VALUES(?,?,?,'waiting',?,'now','now')`, execution.ID, execution.RequestID, plan.ID, string(raw)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.database.GetDB().Exec(`UPDATE server_setup_state SET status='waiting' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	request, _ := json.Marshal(map[string]any{"revision": state.Revision, "execution_id": execution.ID})
	w := httptest.NewRecorder()
	f.panel.handleServerSetupRevise(w, serviceOperationAdminRequest(t, http.MethodPost, "/api/v1/setup/revise", string(request), f.userID))
	if w.Code != http.StatusOK {
		t.Fatalf("revise: %d %s", w.Code, w.Body.String())
	}
	state, err := f.panel.loadServerSetup(context.Background())
	if err != nil || state.Status != "draft" || !state.Extension || state.Required || state.CompletedAt != ready.CompletedAt {
		t.Fatalf("reopened extension: %+v %v", state, err)
	}
}

// set18 D1 (2026-10-16): while a completed setup is being extended and the
// extension has not started, the latest run is the completed setup's own
// succeeded run. The operation read says so (superseded), so the page shows
// the extension's wizard instead of "completion could not be verified".
func TestServerSetupOperationMarksTheCompletedRunSupersededByAnExtension(t *testing.T) {
	f, state := setupOperationFixture(t)
	plan := saveSetupPlanForTest(t, f, state)
	id := strings.Repeat("6", 32)
	execution := serverSetupExecution{ID: id, RequestID: id, PlanID: plan.ID, Status: "succeeded", Phase: "complete"}
	for _, step := range plan.Steps {
		execution.Steps = append(execution.Steps, serverSetupExecutionStep{serverSetupPlanStep: step, Status: "succeeded", RequestID: serverSetupID(id, step.ID, "request"), OwnerID: serverSetupID(id, step.ID, "owner")})
	}
	raw, _ := json.Marshal(execution)
	if _, err := f.database.GetDB().Exec(`INSERT INTO server_setup_executions(id,request_id,plan_id,status,execution_json,created_at,updated_at) VALUES(?,?,?,'succeeded',?,'now','now')`, id, id, plan.ID, string(raw)); err != nil {
		t.Fatal(err)
	}
	read := func(label string, want bool) {
		t.Helper()
		for _, query := range []string{"", "?request_id=" + id} {
			response, body := readSetupExecutionContextResponse(t, f, query)
			if response.serverSetupExecution == nil || response.ID != id || response.Status != "succeeded" {
				t.Fatalf("%s%s: the run itself must stay readable: %s", label, query, body)
			}
			if response.Superseded != want || strings.Contains(body, `"superseded"`) != want {
				t.Fatalf("%s%s: superseded=%v, want %v: %s", label, query, response.Superseded, want, body)
			}
		}
	}
	// Negative control: a first setup whose run succeeded but whose state is
	// not ready is a completion still to verify, never superseded.
	read("first setup not ready", false)
	ready := markServerSetupReadyForTest(t, f)
	read("completed setup", false)
	if w := postSetupExtendForTest(t, f, map[string]any{"revision": ready.Revision, "action": "begin"}); w.Code != http.StatusOK {
		t.Fatalf("begin: %d %s", w.Code, w.Body.String())
	}
	read("extension in draft", true)
	extending, _ := f.panel.loadServerSetup(context.Background())
	if w := postSetupExtendForTest(t, f, map[string]any{"revision": extending.Revision, "action": "cancel"}); w.Code != http.StatusOK {
		t.Fatalf("cancel: %d %s", w.Code, w.Body.String())
	}
	read("extension stopped", false)
	again, _ := f.panel.loadServerSetup(context.Background())
	if w := postSetupExtendForTest(t, f, map[string]any{"revision": again.Revision, "action": "begin"}); w.Code != http.StatusOK {
		t.Fatalf("begin again: %d %s", w.Code, w.Body.String())
	}
	read("extension in draft again", true)
	// Once the extension starts the state is running: whatever run the read
	// answers is not flagged (the page shows the extension's own run).
	if _, err := f.database.GetDB().Exec(`UPDATE server_setup_state SET status='running' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	read("extension running", false)
	if f.agent.installCalls.Load() != 0 {
		t.Fatal("reading the operation changed the host")
	}
}
