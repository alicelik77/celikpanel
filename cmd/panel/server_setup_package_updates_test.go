package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Beta-1 VM cell a: a fresh Debian 13 refused MariaDB in setup because its
// install brought point updates of installed base packages. The review lists
// them ("Distribution packages also updated"), the one setup confirmation
// covers them, and the setup's install step tells the Agent so; a new item at
// start needs a new review; an unread simulation stays "unknown".
func TestServerSetupReviewListsDistributionPointUpdatesAndTheStepAcceptsThem(t *testing.T) {
	f, state := setupOperationFixture(t)
	point := []transport.NotableInstall{
		{Package: "perl-base", Reason: "upgrade", From: "5.40.1-6", To: "5.40.1-6+deb13u1", PointUpdate: true},
		{Package: "libpcre2-8-0", Reason: "upgrade", From: "10.46-1~deb13u1", To: "10.46-1~deb13u3", PointUpdate: true},
	}
	var previewed []string
	notable := append([]transport.NotableInstall{{Package: "apache2", Reason: "apache"}}, point...)
	f.panel.serverSetupInstallPreview = func(_ context.Context, request transport.ServiceInstallPreviewRequest) (transport.ServiceInstallPreviewResponse, error) {
		for _, step := range request.Steps {
			previewed = append(previewed, step.ID)
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
	if f.agent.installCalls.Load() != 0 {
		t.Fatal("the review installed something")
	}
	// The list is not part of the plan identity; an unread one is unknown.
	f.panel.serverSetupInstallPreview = func(context.Context, transport.ServiceInstallPreviewRequest) (transport.ServiceInstallPreviewResponse, error) {
		return transport.ServiceInstallPreviewResponse{}, errors.New("agent unavailable")
	}
	unread, err := f.panel.buildServerSetupPlan(context.Background(), state, serviceOperationActor{UserID: f.userID})
	if err != nil || unread.ID != plan.ID || unread.PackageUpdatesState != "unknown" || len(unread.PackageUpdates) != 0 {
		t.Fatalf("unread: %v %q %+v", err, unread.PackageUpdatesState, unread.PackageUpdates)
	}
	if !serverSetupPackageUpdatesCovered(plan, unread) || !serverSetupPackageUpdatesCovered(plan, plan) {
		t.Fatal("the same or an unread list needs no new review")
	}
	// A point update the review did not list makes the start stale.
	notable = append(notable, transport.NotableInstall{Package: "libssl3", Reason: "upgrade", From: "3.5.1-1", To: "3.5.1-1+deb13u1", PointUpdate: true})
	f.panel.serverSetupInstallPreview = func(context.Context, transport.ServiceInstallPreviewRequest) (transport.ServiceInstallPreviewResponse, error) {
		return transport.ServiceInstallPreviewResponse{State: "known", Notable: notable}, nil
	}
	if w := postSetupStartForTest(t, f, plan.ID, strings.Repeat("d", 32)); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "server_setup_review_stale") {
		t.Fatalf("an unlisted update started: %d %s", w.Code, w.Body.String())
	}
	// The setup's component step carries the confirmation to the Agent.
	step := serverSetupExecutionStep{serverSetupPlanStep: serverSetupPlanStep{ID: "01-service", Kind: "service", Target: "mariadb"}, RequestID: strings.Repeat("e", 32), OwnerID: strings.Repeat("f", 32)}
	op, err := f.panel.ensureServerSetupChild(context.Background(), plan, step)
	if err != nil {
		t.Fatal(err)
	}
	// The fake host does not run MariaDB afterwards; only the request matters.
	waitForServiceOperation(t, f.panel, f.userID, op.ID, "failed")
	f.agent.mu.Lock()
	defer f.agent.mu.Unlock()
	if len(f.agent.installRequests) != 1 || !f.agent.installRequests[0].AcceptPointUpdates || !f.agent.installRequests[0].CheckInstalls {
		t.Fatalf("install requests: %+v", f.agent.installRequests)
	}
}

// A component installed from Components never carries the setup's
// confirmation: the browser cannot set it.
func TestServiceInstallRequestCannotCarryTheSetupConfirmation(t *testing.T) {
	var req serviceInstallRequest
	if err := json.Unmarshal([]byte(`{"service_id":"mariadb","accept_point_updates":true,"AcceptPointUpdates":true}`), &req); err != nil || req.AcceptPointUpdates {
		t.Fatalf("decoded %+v %v", req, err)
	}
}
