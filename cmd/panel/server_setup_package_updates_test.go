package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
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
