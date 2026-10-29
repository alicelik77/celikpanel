package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Beta-1 VM cell a: a fresh Debian 13 refused MariaDB in setup because its
// install brought point updates of installed base packages. The review lists
// them ("Distribution packages also updated"), the one setup confirmation
// covers exactly those (package=version), and a component step asks again
// before it installs: an update the review did not list, or a read that
// failed, waits and installs nothing.
func TestServerSetupReviewListsDistributionPointUpdatesAndTheStepAcceptsThem(t *testing.T) {
	f, state := setupOperationFixture(t)
	point := []transport.NotableInstall{
		{Package: "perl-base", Reason: "upgrade", From: "5.40.1-6", To: "5.40.1-6+deb13u1", PointUpdate: true},
		{Package: "libpcre2-8-0", Reason: "upgrade", From: "10.46-1~deb13u1", To: "10.46-1~deb13u3", PointUpdate: true},
	}
	var previewed []string
	notable := append([]transport.NotableInstall{{Package: "apache2", Reason: "apache"}}, point...)
	previewErr := error(nil)
	f.panel.serverSetupInstallPreview = func(_ context.Context, request transport.ServiceInstallPreviewRequest) (transport.ServiceInstallPreviewResponse, error) {
		for _, step := range request.Steps {
			previewed = append(previewed, step.ID)
		}
		if previewErr != nil {
			return transport.ServiceInstallPreviewResponse{}, previewErr
		}
		return transport.ServiceInstallPreviewResponse{State: "known", Notable: notable}, nil
	}
	plan := saveSetupPlanForTest(t, f, state)
	if !strings.Contains(strings.Join(previewed, ","), "mariadb") {
		t.Fatalf("the review did not read the component installs: %v", previewed)
	}
	if plan.PackageUpdatesState != "known" || len(plan.PackageUpdates) != 2 || plan.PackageUpdates[0].Package != "libpcre2-8-0" || plan.PackageUpdates[1].To != "5.40.1-6+deb13u1" {
		t.Fatalf("listed: %q %+v", plan.PackageUpdatesState, plan.PackageUpdates)
	}
	if want := []string{"libpcre2-8-0=10.46-1~deb13u3", "perl-base=5.40.1-6+deb13u1"}; !slices.Equal(serverSetupAcceptedPointUpdates(plan), want) {
		t.Fatalf("accepted: %v", serverSetupAcceptedPointUpdates(plan))
	}
	if f.agent.installCalls.Load() != 0 {
		t.Fatal("the review installed something")
	}
	// The list is not part of the plan identity; an unread one is unknown
	// and accepts nothing.
	previewErr = errors.New("agent unavailable")
	unread, err := f.panel.buildServerSetupPlan(context.Background(), state, serviceOperationActor{UserID: f.userID})
	if err != nil || unread.ID != plan.ID || unread.PackageUpdatesState != "unknown" || len(unread.PackageUpdates) != 0 || serverSetupAcceptedPointUpdates(unread) != nil {
		t.Fatalf("unread: %v %q %+v", err, unread.PackageUpdatesState, unread.PackageUpdates)
	}
	previewErr = nil
	step := serverSetupExecutionStep{serverSetupPlanStep: serverSetupPlanStep{ID: "01-service", Kind: "service", Target: "mariadb"}, RequestID: strings.Repeat("e", 32), OwnerID: strings.Repeat("f", 32)}
	waitCode := func(err error) string {
		var wait *serverSetupPeerWait
		if errors.As(err, &wait) {
			return wait.Code
		}
		return ""
	}
	// A read that fails at the step: unknown, nothing installed.
	previewErr = errors.New("package lock busy")
	if _, err := f.panel.ensureServerSetupChild(context.Background(), plan, step); waitCode(err) != "server_setup_distribution_updates_unread" {
		t.Fatalf("unread at the step: %v", err)
	}
	previewErr = nil
	// The unread review's plan accepts nothing: any point update waits.
	if _, err := f.panel.ensureServerSetupChild(context.Background(), unread, step); waitCode(err) != "server_setup_distribution_updates_changed" {
		t.Fatalf("unread review at the step: %v", err)
	}
	// A point update that appeared after the review (days later): a
	// verified state for the owner; no operation, nothing installed.
	notable = append(notable, transport.NotableInstall{Package: "libssl3t64", Reason: "upgrade", From: "3.5.7-1~deb13u2", To: "3.5.7-1~deb13u3", PointUpdate: true})
	if _, err := f.panel.ensureServerSetupChild(context.Background(), plan, step); waitCode(err) != "server_setup_distribution_updates_changed" {
		t.Fatalf("new update at the step: %v", err)
	}
	if w := postSetupStartForTest(t, f, plan.ID, strings.Repeat("d", 32)); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "server_setup_review_stale") {
		t.Fatalf("an unlisted update started: %d %s", w.Code, w.Body.String())
	}
	if f.agent.installCalls.Load() != 0 {
		t.Fatal("a wait installed something")
	}
	if active, _ := f.panel.activeServiceOperation(context.Background()); active != nil {
		t.Fatalf("a wait created an operation: %+v", active)
	}
	// The listed ones only: the step carries them exactly to the Agent.
	notable = notable[:len(notable)-1]
	op, err := f.panel.ensureServerSetupChild(context.Background(), plan, step)
	if err != nil {
		t.Fatal(err)
	}
	// The fake host does not run MariaDB afterwards; only the request matters.
	waitForServiceOperation(t, f.panel, f.userID, op.ID, "failed")
	f.agent.mu.Lock()
	defer f.agent.mu.Unlock()
	if len(f.agent.installRequests) != 1 || !slices.Equal(f.agent.installRequests[0].AcceptedPointUpdates, serverSetupAcceptedPointUpdates(plan)) || !f.agent.installRequests[0].CheckInstalls {
		t.Fatalf("install requests: %+v", f.agent.installRequests)
	}
}

// A component installed from Components never carries the setup's
// confirmation: the browser cannot set it.
func TestServiceInstallRequestCannotCarryTheSetupConfirmation(t *testing.T) {
	var req serviceInstallRequest
	if err := json.Unmarshal([]byte(`{"service_id":"mariadb","accepted_point_updates":["perl-base=5.40.1-6+deb13u1"],"AcceptedPointUpdates":["x=1"]}`), &req); err != nil || req.AcceptedPointUpdates != nil {
		t.Fatalf("decoded %+v %v", req, err)
	}
}

// Beta-1 re-run P5: the review read "unknown" (the package manager was busy)
// and setup started anyway. An unread list holds the start, in the plan the
// web reads (can_start) and in the start handler a client cannot bypass;
// refreshing the review reads it again and the same plan can then start.
func TestServerSetupUnreadPackageUpdatesHoldTheStartUntilTheReviewIsRefreshed(t *testing.T) {
	f, state := setupOperationFixture(t)
	busy := true
	f.panel.serverSetupInstallPreview = func(context.Context, transport.ServiceInstallPreviewRequest) (transport.ServiceInstallPreviewResponse, error) {
		if busy {
			return transport.ServiceInstallPreviewResponse{}, errors.New("could not get lock /var/lib/dpkg/lock-frontend")
		}
		return transport.ServiceInstallPreviewResponse{State: "known"}, nil
	}
	serverSetupRunners.Store(f.panel, true)
	t.Cleanup(func() { serverSetupRunners.Delete(f.panel) })
	review := func() serverSetupPlan {
		w := httptest.NewRecorder()
		body, _ := json.Marshal(map[string]any{"revision": state.Revision})
		f.panel.handleServerSetupPlan(w, serviceOperationAdminRequest(t, http.MethodPost, "/api/v1/setup/plan", string(body), f.userID))
		var plan serverSetupPlan
		if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &plan) != nil {
			t.Fatalf("review: %d %s", w.Code, w.Body.String())
		}
		return plan
	}
	unread := review()
	if unread.PackageUpdatesState != "unknown" || unread.CanStart || len(unread.Blockers) != 0 {
		t.Fatalf("an unread review could start: %q %v %v", unread.PackageUpdatesState, unread.CanStart, unread.Blockers)
	}
	w := postSetupStartForTest(t, f, unread.ID, strings.Repeat("d", 32))
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), serverSetupPackageUpdatesUnreadCode) {
		t.Fatalf("the start handler accepted an unread review: %d %s", w.Code, w.Body.String())
	}
	if execution, err := f.panel.latestServerSetupExecution(context.Background()); err != nil || execution != nil {
		t.Fatalf("an execution was created: %+v %v", execution, err)
	}
	if f.agent.installCalls.Load() != 0 {
		t.Fatal("the refused start installed something")
	}
	// Refreshing the review is a read: the same plan, now read.
	busy = false
	read := review()
	if read.ID != unread.ID || read.PackageUpdatesState != "known" || !read.CanStart {
		t.Fatalf("refreshed review: %s/%s %q %v", read.ID, unread.ID, read.PackageUpdatesState, read.CanStart)
	}
	if w := postSetupStartForTest(t, f, read.ID, strings.Repeat("e", 32)); w.Code != http.StatusAccepted {
		t.Fatalf("the read review did not start: %d %s", w.Code, w.Body.String())
	}
}

// Beta-1 re-run P5: a step waited on server_setup_distribution_updates_changed
// and told the owner to open the review, but reopening was refused for the
// phase "service". The owner now reviews the new list from that wait and
// confirms it; the same execution continues from that step, nothing already
// done is repeated, and the install carries the confirmed update.
func TestServerSetupDistributionUpdatesWaitIsConfirmedAndTheSameRunContinues(t *testing.T) {
	f, state := setupOperationFixture(t)
	var notable []transport.NotableInstall
	f.panel.serverSetupInstallPreview = func(context.Context, transport.ServiceInstallPreviewRequest) (transport.ServiceInstallPreviewResponse, error) {
		return transport.ServiceInstallPreviewResponse{State: "known", Notable: notable}, nil
	}
	plan := saveSetupPlanForTest(t, f, state)
	if plan.PackageUpdatesState != "known" || len(plan.PackageUpdates) != 0 {
		t.Fatalf("review: %q %+v", plan.PackageUpdatesState, plan.PackageUpdates)
	}
	serverSetupRunners.Store(f.panel, true)
	t.Cleanup(func() { serverSetupRunners.Delete(f.panel) })
	response := postSetupStartForTest(t, f, plan.ID, strings.Repeat("9", 32))
	if response.Code != http.StatusAccepted {
		t.Fatalf("start %d %s", response.Code, response.Body.String())
	}
	var execution serverSetupExecution
	if err := json.Unmarshal(response.Body.Bytes(), &execution); err != nil {
		t.Fatal(err)
	}
	// Days later a point update appears; the first component waits.
	update := transport.NotableInstall{Package: "libssl3t64", Reason: "upgrade", From: "3.5.7-1~deb13u2", To: "3.5.7-1~deb13u3", PointUpdate: true}
	notable = []transport.NotableInstall{update}
	for pass := 0; pass < 4 && execution.Status != "waiting"; pass++ {
		if _, err := f.panel.advanceServerSetupExecution(plan, &execution); err != nil {
			t.Fatal(err)
		}
	}
	index := serverSetupPackageUpdatesWaitingStep(execution)
	if execution.Status != "waiting" || execution.Error == nil || execution.Error.Code != "server_setup_distribution_updates_changed" || index < 0 {
		t.Fatalf("no distribution-update wait: %+v", execution)
	}
	if f.agent.installCalls.Load() != 0 {
		t.Fatal("the wait installed something")
	}
	call := func(body map[string]any) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(body)
		w := httptest.NewRecorder()
		f.panel.handleServerSetupPackageUpdates(w, serviceOperationAdminRequest(t, http.MethodPost, "/api/v1/setup/package-updates", string(raw), f.userID))
		return w
	}
	// The owner reviews: a read, the new list.
	w := call(map[string]any{"execution_id": execution.ID, "action": "review"})
	var reviewed serverSetupPackageUpdatesResponse
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &reviewed) != nil || reviewed.State != "known" || len(reviewed.Updates) != 1 || reviewed.Updates[0].To != update.To {
		t.Fatalf("review: %d %s", w.Code, w.Body.String())
	}
	if confirmed, _ := f.panel.readServerSetupRunPackageUpdates(context.Background(), execution.ID); len(confirmed) != 0 {
		t.Fatalf("the review confirmed something: %+v", confirmed)
	}
	// A confirmation that does not cover what the package manager lists now
	// confirms nothing.
	if w := call(map[string]any{"execution_id": execution.ID, "action": "confirm", "updates": []transport.NotableInstall{}}); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "server_setup_review_stale") {
		t.Fatalf("an empty confirmation: %d %s", w.Code, w.Body.String())
	}
	w = call(map[string]any{"execution_id": execution.ID, "action": "confirm", "updates": reviewed.Updates})
	if w.Code != http.StatusOK {
		t.Fatalf("confirm: %d %s", w.Code, w.Body.String())
	}
	saved, err := f.panel.latestServerSetupExecution(context.Background())
	if err != nil || saved.ID != execution.ID || saved.Status != "running" || saved.Error != nil {
		t.Fatalf("the same run did not continue: %+v %v", saved, err)
	}
	// Not waiting any more: the actions are refused, nothing is overwritten.
	if w := call(map[string]any{"execution_id": execution.ID, "action": "confirm", "updates": reviewed.Updates}); w.Code != http.StatusConflict {
		t.Fatalf("a confirmation outside the wait: %d %s", w.Code, w.Body.String())
	}
	execution = *saved
	waiting := execution.Steps[index]
	if _, err := f.panel.advanceServerSetupExecution(plan, &execution); err != nil {
		t.Fatal(err)
	}
	step := execution.Steps[index]
	if step.ID != waiting.ID || step.RequestID != waiting.RequestID || step.OperationID == "" {
		t.Fatalf("the waiting step did not continue: %+v", step)
	}
	waitForServiceOperation(t, f.panel, f.userID, step.OperationID, "failed")
	f.agent.mu.Lock()
	defer f.agent.mu.Unlock()
	if len(f.agent.installRequests) != 1 || !slices.Contains(f.agent.installRequests[0].AcceptedPointUpdates, "libssl3t64=3.5.7-1~deb13u3") {
		t.Fatalf("install requests: %+v", f.agent.installRequests)
	}
}
