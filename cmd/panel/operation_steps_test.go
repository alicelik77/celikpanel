package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/alicelik/celikpanel/internal/core"
)

// The shared operation-step store (alpha.127, owner's order of 6 Nov 2026:
// every operation says what it is doing while it does it).

// recordOperationSteps collects every step handlers report until the test ends.
func recordOperationSteps(t *testing.T) func(id string) []string {
	t.Helper()
	var mu sync.Mutex
	seen := map[string][]string{}
	operationStepSeen = func(id, step string) {
		mu.Lock()
		defer mu.Unlock()
		seen[id] = append(seen[id], step)
	}
	t.Cleanup(func() { operationStepSeen = nil })
	return func(id string) []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), seen[id]...)
	}
}

func readOperationProgress(t *testing.T, p *Panel, id string, caller *Caller) (int, operationProgressView, apiErrorBody) {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, operationProgressPath+id, nil)
	if caller != nil {
		request = request.WithContext(context.WithValue(request.Context(), callerKey, caller))
	}
	recorder := httptest.NewRecorder()
	p.handleOperationProgress(recorder, request)
	var view operationProgressView
	var refusal apiErrorBody
	if recorder.Code == http.StatusOK {
		if err := json.Unmarshal(recorder.Body.Bytes(), &view); err != nil {
			t.Fatalf("progress answer: %v (%s)", err, recorder.Body.String())
		}
	} else {
		_ = json.Unmarshal(recorder.Body.Bytes(), &refusal)
	}
	return recorder.Code, view, refusal
}

// While a change runs, its sender reads the plan and the step it is at; nobody
// else can; afterwards the answer and the step it ended at are read back, and
// no read ever runs the handler again.
func TestOperationProgressShowsTheLiveStepToItsSenderOnly(t *testing.T) {
	guard, database := newRequestIdentityTestGuard(t)
	panel := &Panel{requestIdentities: guard}
	_ = database
	atStep := make(chan struct{})
	release := make(chan struct{})
	var runs int
	handler := guard.wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		runs++
		reportOperationPlan(r.Context(), "check", "restore", "finish", "Not A Step!")
		reportOperationStep(r.Context(), "check")
		reportOperationStep(r.Context(), "restore")
		close(atStep)
		<-release
		reportOperationStep(r.Context(), "finish")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	id := requestIdentityTestID(41)
	owner := requestIdentityTestAdmin()
	done := make(chan *httptest.ResponseRecorder)
	go func() {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, requestIdentityTestRequest("/api/v1/domains/3/backups/restore", `{}`, id, owner))
		done <- recorder
	}()
	<-atStep

	code, view, _ := readOperationProgress(t, panel, id, owner)
	if code != http.StatusOK || view.State != operationProgressRunning || view.Step != "restore" || view.StepStartedAt == "" {
		t.Fatalf("running read: %d %+v", code, view)
	}
	if !reflect.DeepEqual(view.Plan, []string{"check", "restore", "finish"}) {
		t.Fatalf("plan=%v: a malformed step name must never reach the screen", view.Plan)
	}
	other := &Caller{ID: owner.ID + 1, Role: roleAdmin}
	if code, _, refusal := readOperationProgress(t, panel, id, other); code != http.StatusNotFound || refusal.Code != errCodeOperationNotFound {
		t.Fatalf("another person read the change: %d %+v", code, refusal)
	}
	if code, _, _ := readOperationProgress(t, panel, id, nil); code != http.StatusUnauthorized {
		t.Fatalf("anonymous read: %d", code)
	}

	close(release)
	answer := <-done
	if answer.Code != http.StatusOK || answer.Header().Get(operationStepHeader) != "finish" {
		t.Fatalf("answer %d, step header %q", answer.Code, answer.Header().Get(operationStepHeader))
	}
	code, view, _ = readOperationProgress(t, panel, id, owner)
	if code != http.StatusOK || view.State != operationProgressDone || view.ResponseStatus != http.StatusOK ||
		view.Step != "finish" || string(view.Answer) != `{"success":true}` {
		t.Fatalf("ended read: %d %+v answer=%s", code, view, view.Answer)
	}
	for i := 0; i < 3; i++ {
		readOperationProgress(t, panel, id, owner)
	}
	if runs != 1 {
		t.Fatalf("the change ran %d times; reads must never run it", runs)
	}
	if code, _, _ := readOperationProgress(t, panel, requestIdentityTestID(99), owner); code != http.StatusNotFound {
		t.Fatalf("unknown identity: %d", code)
	}
	if code, _, _ := readOperationProgress(t, panel, "not-an-identity", owner); code != http.StatusNotFound {
		t.Fatalf("malformed identity: %d", code)
	}
}

// A row a previous process left `running` is an unknown result. The read says
// so and leaves the row exactly as it was (reads never write).
func TestOperationProgressCallsAnOrphanedRunUnknownWithoutChangingIt(t *testing.T) {
	guard, database := newRequestIdentityTestGuard(t)
	panel := &Panel{requestIdentities: guard}
	id := requestIdentityTestID(42)
	if _, err := database.Exec(`
		INSERT INTO request_identities (id, actor_user_id, method, route, request_sha256, status, created_at, expires_at)
		VALUES (?, 7, 'POST', '/api/v1/domains/{id}/ssl/letsencrypt', ?, 'running', 1, 9999999999)`, id, strings.Repeat("0", 64)); err != nil {
		t.Fatal(err)
	}
	code, view, _ := readOperationProgress(t, panel, id, requestIdentityTestAdmin())
	if code != http.StatusOK || view.State != operationProgressUnknown {
		t.Fatalf("orphaned read: %d %+v", code, view)
	}
	var status string
	if err := database.QueryRow(`SELECT status FROM request_identities WHERE id = ?`, id).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != requestIdentityRunning {
		t.Fatalf("the read rewrote the row to %q", status)
	}
}

// An answer that carries a one-time secret is never handed back by a read.
func TestOperationProgressNeverReturnsANotRetainedAnswer(t *testing.T) {
	guard, _ := newRequestIdentityTestGuard(t)
	panel := &Panel{requestIdentities: guard}
	handler := guard.wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reportOperationStep(r.Context(), "create")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"password":"one-time"}`))
	}))
	id := requestIdentityTestID(43)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, requestIdentityTestRequest("/api/v1/vpn/peers", `{}`, id, requestIdentityTestAdmin()))
	code, view, _ := readOperationProgress(t, panel, id, requestIdentityTestAdmin())
	if code != http.StatusOK || view.State != operationProgressDone || len(view.Answer) != 0 || view.Step != "create" {
		t.Fatalf("secret route read: %d %+v answer=%s", code, view, view.Answer)
	}
}

// Changing a site's PHP version is guarded when the page sends an identity
// (and runs as before without one), and says its steps.
func TestPHPVersionChangeReportsItsStepsUnderItsIdentity(t *testing.T) {
	steps := recordOperationSteps(t)
	fixture := newDomainPHPHandlerFixture(t, []core.ServiceInstance{{Version: "8.3", Managed: true}, {Version: "8.4", Managed: true}})
	guard := newRequestIdentityGuard(fixture.panel.db.GetDB())
	fixture.panel.requestIdentities = guard
	handler := guard.wrap(http.HandlerFunc(fixture.panel.handleDomainSubroute))
	owner := domainPHPTestOwnerCaller()
	path := fmt.Sprintf("/api/v1/domains/%d/php", domainPHPTestDomainID)

	// Same version: check, then save.
	id := requestIdentityTestID(51)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, requestIdentityTestRequest(path, `{"php_version":"8.3"}`, id, owner))
	if recorder.Code != http.StatusOK {
		t.Fatalf("same-version save: %d %s", recorder.Code, recorder.Body.String())
	}
	if got := steps(id); !reflect.DeepEqual(got, []string{"check", "save"}) {
		t.Fatalf("steps=%v", got)
	}
	if _, view, _ := readOperationProgress(t, fixture.panel, id, owner); !reflect.DeepEqual(view.Plan, []string{"check", "save"}) || view.State != operationProgressDone {
		t.Fatalf("progress=%+v", view)
	}

	// A version the server does not run is refused at the check: the answer
	// names that step, and nothing was switched.
	id = requestIdentityTestID(52)
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, requestIdentityTestRequest(path, `{"php_version":"8.5"}`, id, owner))
	if recorder.Code != http.StatusBadRequest || recorder.Header().Get(operationStepHeader) != "check" {
		t.Fatalf("refusal: %d step=%q", recorder.Code, recorder.Header().Get(operationStepHeader))
	}
	if _, _, migrate := fixture.agent.callCounts(); migrate != 0 {
		t.Fatalf("a refused change migrated the pool %d times", migrate)
	}

	// Without the header the route runs as before and keeps no row.
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, requestIdentityTestRequest(path, `{"php_version":"8.3"}`, "", owner))
	if recorder.Code != http.StatusOK {
		t.Fatalf("unguarded save: %d", recorder.Code)
	}
	var rows int
	if err := fixture.panel.db.GetDB().QueryRow(`SELECT COUNT(*) FROM request_identities`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 2 {
		t.Fatalf("rows=%d, want only the two identified changes", rows)
	}
}

// Backup restore, backup creation and the certificate request report their
// real steps in order, through the guard the screens use.
func TestGuardedChangesReportTheirStepsInOrder(t *testing.T) {
	cases := newRequestIdentityRouteCases()
	for name, want := range map[string][]string{
		"POST domains/{id}/backups/restore":           {"check", "inspect", "restore", "finish"},
		"POST domains/{id}/backups":                   {"check", "create", "finish"},
		"POST domains/{id}/ssl/letsencrypt (reissue)": {"check", "dns_check", "prepare_validation", "acme", "verify", "install", "reload"},
	} {
		t.Run(name, func(t *testing.T) {
			steps := recordOperationSteps(t)
			c := cases[name](t)
			id := requestIdentityTestID(60 + len(name))
			answer := c.send(c.handler, id)
			if answer.Code != c.wantStatus {
				t.Fatalf("status=%d body=%s", answer.Code, answer.Body.String())
			}
			if got := steps(id); !reflect.DeepEqual(got, want) {
				t.Fatalf("steps=%v, want %v", got, want)
			}
			if got := answer.Header().Get(operationStepHeader); got != want[len(want)-1] {
				t.Fatalf("answer names step %q, want %q", got, want[len(want)-1])
			}
		})
	}
}

// A restore the Agent refuses names the step it stopped at.
func TestARefusedRestoreNamesTheStepItStoppedAt(t *testing.T) {
	steps := recordOperationSteps(t)
	f := newPanelBackupFixture(t)
	const backupName = "files-20261009T120000.000000000Z-0123456789abcdef.cpbak"
	f.agent.inspectResp.Success = false
	f.agent.inspectResp.Error = "backup archive is damaged"
	guard := newRequestIdentityGuard(f.panel.db.GetDB())
	handler := guard.wrap(requestIdentityRoutesMux(f.panel))
	var userID int
	if err := f.panel.db.GetDB().QueryRow(`SELECT id FROM users ORDER BY id LIMIT 1`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	id := requestIdentityTestID(70)
	request := requestIdentityTestRequest(fmt.Sprintf("/api/v1/domains/%d/backups/restore", f.domainID),
		`{"backup_name":"`+backupName+`"}`, id, &Caller{ID: userID, Role: roleAdmin})
	request.Header.Set("Origin", "https://"+request.Host)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code < 400 || recorder.Header().Get(operationStepHeader) != "inspect" {
		t.Fatalf("refused restore: %d step=%q body=%s", recorder.Code, recorder.Header().Get(operationStepHeader), recorder.Body.String())
	}
	if got := steps(id); !reflect.DeepEqual(got, []string{"check", "inspect"}) {
		t.Fatalf("steps=%v", got)
	}
}
