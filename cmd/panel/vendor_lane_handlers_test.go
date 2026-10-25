package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.108 (D-038 amendment "two lanes"): a vendor repository is enabled
// only through its shown-and-confirmed lane; a plain enable is refused for
// it; customers never reach it; every outcome is audited.

func TestAVendorLaneIsShownFirstAndWrittenOnlyAsConfirmed(t *testing.T) {
	panel, _, database, userID := newServiceStateAtomicPanel(t)
	restoreRead, restoreWrite := readVendorLanePlan, enableVendorLaneThroughAgent
	t.Cleanup(func() { readVendorLanePlan, enableVendorLaneThroughAgent = restoreRead, restoreWrite })
	readVendorLanePlan = func(_ *Panel, _ context.Context, repoID string) (transport.VendorLanePlan, error) {
		return transport.VendorLanePlan{RepoID: repoID, Origin: "deb.sury.org", Ready: true, SHA256: strings.Repeat("b", 64), Steps: []transport.VendorLaneStep{
			{Step: "repository", State: "done"}, {Step: "confinement", State: "todo", Text: "Package: *\n", SHA256: strings.Repeat("c", 64)},
			{Step: "updates", State: "todo", Text: "Unattended-Upgrade::Origins-Pattern {\n};\n", SHA256: strings.Repeat("d", 64)},
		}}, nil
	}
	recorder := httptest.NewRecorder()
	panel.handleVendorLane(recorder, serviceStateAtomicRequest(http.MethodGet, vendorLanePath+"?service_id=php-fpm", "", userID))
	if body := recorder.Body.String(); !strings.Contains(body, `"state":"read"`) || !strings.Contains(body, `"step":"confinement"`) {
		t.Fatalf("plan = %d %s", recorder.Code, body)
	}

	called := ""
	enableVendorLaneThroughAgent = func(_ *Panel, _ context.Context, repoID, expected string) (transport.VendorLaneResponse, error) {
		called = repoID + ":" + expected
		return transport.VendorLaneResponse{Completed: []string{"confinement", "updates"}, Effective: true, Enabled: true}, nil
	}
	// Without the confirmed plan nothing runs.
	recorder = httptest.NewRecorder()
	panel.handleVendorLane(recorder, serviceStateAtomicRequest(http.MethodPost, vendorLanePath, `{"service_id":"php-fpm"}`, userID))
	if recorder.Code != http.StatusBadRequest || called != "" {
		t.Fatalf("unconfirmed = %d %s %q", recorder.Code, recorder.Body.String(), called)
	}
	// A component without a lane is refused.
	recorder = httptest.NewRecorder()
	panel.handleVendorLane(recorder, serviceStateAtomicRequest(http.MethodPost, vendorLanePath, `{"service_id":"nginx","expected_sha256":"`+strings.Repeat("b", 64)+`"}`, userID))
	if recorder.Code != http.StatusBadRequest || called != "" {
		t.Fatalf("no lane = %d %s", recorder.Code, recorder.Body.String())
	}
	recorder = httptest.NewRecorder()
	panel.handleVendorLane(recorder, serviceStateAtomicRequest(http.MethodPost, vendorLanePath, `{"service_id":"php-fpm","expected_sha256":"`+strings.Repeat("b", 64)+`"}`, userID))
	if recorder.Code != http.StatusOK || called != "sury:"+strings.Repeat("b", 64) || !strings.Contains(recorder.Body.String(), `"completed":["confinement","updates"]`) {
		t.Fatalf("lane = %d %s %q", recorder.Code, recorder.Body.String(), called)
	}
	var action, details string
	if err := database.GetDB().QueryRow(`SELECT action, COALESCE(details,'') FROM audit_logs ORDER BY id DESC LIMIT 1`).Scan(&action, &details); err != nil {
		t.Fatal(err)
	}
	if action != "repo.lane:sury — confinement,updates" || !strings.Contains(details, `"confirmed_sha256"`) || !strings.Contains(details, `"origin":"deb.sury.org"`) {
		t.Fatalf("audit = %q %s", action, details)
	}
	// A refusal is coded, audited and says which steps stay.
	enableVendorLaneThroughAgent = func(*Panel, context.Context, string, string) (transport.VendorLaneResponse, error) {
		return transport.VendorLaneResponse{ErrorCode: "LANE_PARTIAL", Error: "the automatic updates drop-in could not be written", Completed: []string{"confinement"}}, nil
	}
	recorder = httptest.NewRecorder()
	panel.handleVendorLane(recorder, serviceStateAtomicRequest(http.MethodPost, vendorLanePath, `{"service_id":"php-fpm","expected_sha256":"`+strings.Repeat("b", 64)+`"}`, userID))
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), `"code":"LANE_PARTIAL"`) || !strings.Contains(recorder.Body.String(), `"completed":["confinement"]`) {
		t.Fatalf("partial = %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestAVendorRepositoryIsNeverEnabledAloneAndCustomersNeverReachIt(t *testing.T) {
	panel, _, _, userID := newServiceStateAtomicPanel(t)
	recorder := httptest.NewRecorder()
	panel.handleRepo(recorder, serviceStateAtomicRequest(http.MethodPost, "/api/v1/repo", `{"service_id":"postgresql","action":"enable"}`, userID))
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), "REPO_LANE_REQUIRED") {
		t.Fatalf("plain enable = %d %s", recorder.Code, recorder.Body.String())
	}
	customer := func(method, target, body string) *http.Request {
		req := httptest.NewRequest(method, target, strings.NewReader(body))
		return req.WithContext(context.WithValue(req.Context(), callerKey, &Caller{ID: userID, Role: roleCustomer}))
	}
	for _, req := range []*http.Request{
		customer(http.MethodGet, vendorLanePath+"?service_id=php-fpm", ""),
		customer(http.MethodPost, vendorLanePath, `{"service_id":"php-fpm","expected_sha256":"`+strings.Repeat("b", 64)+`"}`),
	} {
		recorder := httptest.NewRecorder()
		panel.handleVendorLane(recorder, req)
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("customer %s = %d", req.Method, recorder.Code)
		}
	}
	recorder = httptest.NewRecorder()
	panel.handleRepo(recorder, customer(http.MethodPost, "/api/v1/repo", `{"service_id":"php-fpm","action":"disable"}`))
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("customer disable = %d", recorder.Code)
	}
	if !isAdminOnlyPath(vendorLanePath) {
		t.Fatal("the lane route is not an administrator route")
	}
}
