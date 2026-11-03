package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// installN8N installs n8n on outsider.matrix.test through the real operation.
func installN8N(t *testing.T, f serviceAppFixture) int {
	t.Helper()
	f.offer(t, "n8n")
	wantStatus(t, f.as(t, 9704, http.MethodPost, "/api/v1/domains/9903/service-apps/install", n8nInstallBody), http.StatusAccepted, "")
	if op := f.waitOperation(t, "op-n8n-0001"); op.State != "succeeded" {
		t.Fatalf("install: %+v", op)
	}
	var id int
	if err := f.panel.db.GetDB().QueryRow(`SELECT id FROM service_app_installs WHERE domain_id = 9903`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func adminSandbox(t *testing.T, f serviceAppFixture) serviceAppSandboxView {
	t.Helper()
	rec := f.as(t, 9701, http.MethodGet, serviceAppsAdminPath, "")
	wantStatus(t, rec, http.StatusOK, "")
	var page struct {
		Sandbox serviceAppSandboxView `json:"sandbox"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	return page.Sandbox
}

// TestServiceAppSandboxRefreshIsExplicitAndOnce: a running alpha.118 unit is
// flagged, never rewritten by a read; the administrator's action restarts it
// once through the durable operation, a repeated request answers the first
// answer, and a count that changed refuses.
func TestServiceAppSandboxRefreshIsExplicitAndOnce(t *testing.T) {
	f := newServiceAppFixture(t)
	id := installN8N(t, f)
	f.native.mu.Lock()
	f.native.generations = map[int]int{id: 1}
	activatesBefore := f.native.activates
	f.native.mu.Unlock()

	view := adminSandbox(t, f)
	if !view.Known || view.NeedsRefresh != 1 || view.Running != 1 || view.Level != "full" {
		t.Fatalf("view = %+v", view)
	}
	rec := f.as(t, 9704, http.MethodGet, "/api/v1/domains/9903/service-apps", "")
	if !strings.Contains(rec.Body.String(), `"needs_sandbox":true`) {
		t.Fatalf("the domain page lacks the flag: %s", rec.Body.String())
	}
	f.native.mu.Lock()
	if f.native.activates != activatesBefore {
		t.Fatal("a read rewrote the unit")
	}
	f.native.mu.Unlock()

	wantStatus(t, f.as(t, 9703, http.MethodPost, serviceAppsAdminPath+"/sandbox-refresh", `{"request_id":"refresh-0001","expected":1}`), http.StatusForbidden, "")
	wantStatus(t, f.as(t, 9701, http.MethodPost, serviceAppsAdminPath+"/sandbox-refresh", `{"request_id":"refresh-0001","expected":2}`), http.StatusConflict, errCodeServiceApp)
	rec = f.as(t, 9701, http.MethodPost, serviceAppsAdminPath+"/sandbox-refresh", `{"request_id":"refresh-0002","expected":1}`)
	wantStatus(t, rec, http.StatusOK, "")
	var result serviceAppSandboxRefreshResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil || len(result.Started) != 1 || result.Started[0] != "outsider.matrix.test" {
		t.Fatalf("result = %+v (%v)", result, err)
	}
	var opID, message string
	if err := f.panel.db.GetDB().QueryRow(`SELECT id, message FROM service_app_operations WHERE install_id = ? AND kind = 'update' ORDER BY created_at DESC LIMIT 1`, id).Scan(&opID, &message); err != nil {
		t.Fatal(err)
	}
	if op := f.waitOperation(t, opID); op.State != "succeeded" {
		t.Fatalf("refresh operation = %+v", op)
	}
	// The lost answer: the same request answers again and starts nothing.
	rec = f.as(t, 9701, http.MethodPost, serviceAppsAdminPath+"/sandbox-refresh", `{"request_id":"refresh-0002","expected":1}`)
	wantStatus(t, rec, http.StatusOK, "")
	var count int
	_ = f.panel.db.GetDB().QueryRow(`SELECT COUNT(*) FROM service_app_operations WHERE install_id = ? AND kind = 'update'`, id).Scan(&count)
	if count != 1 {
		t.Fatalf("operations after a repeat = %d", count)
	}
	if v := adminSandbox(t, f); v.NeedsRefresh != 0 {
		t.Fatalf("after the refresh = %+v", v)
	}
	_ = f.panel.db.GetDB().QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action = 'service_app.sandbox_refresh'`).Scan(&count)
	if count != 1 {
		t.Fatalf("audit rows = %d", count)
	}
	f.native.mu.Lock()
	defer f.native.mu.Unlock()
	if f.native.activates != activatesBefore+1 {
		t.Fatalf("activates = %d, want one more than %d", f.native.activates, activatesBefore)
	}
}

func TestServiceAppSandboxRefreshKeepsAStoppedAppStopped(t *testing.T) {
	f := newServiceAppFixture(t)
	id := installN8N(t, f)
	f.exec(t, `UPDATE service_app_installs SET state = 'stopped' WHERE id = ?`, id)
	f.native.mu.Lock()
	f.native.generations = map[int]int{id: 1}
	f.native.units[id] = transport.AppUnitState{Present: true}
	f.native.mu.Unlock()
	rec := f.as(t, 9701, http.MethodPost, serviceAppsAdminPath+"/sandbox-refresh", `{"request_id":"refresh-0003","expected":1}`)
	wantStatus(t, rec, http.StatusOK, "")
	if !strings.Contains(rec.Body.String(), `"rewritten":["outsider.matrix.test"]`) {
		t.Fatalf("result = %s", rec.Body.String())
	}
	f.native.mu.Lock()
	defer f.native.mu.Unlock()
	if f.native.units[id].Active {
		t.Fatal("a stopped app was started")
	}
}

func TestServiceAppSandboxProofFailureStopsAndDoesNotPublish(t *testing.T) {
	f := newServiceAppFixture(t)
	f.native.proof = &transport.SandboxProof{Code: "not_in_smtp_set", Detail: "celiksvc-1.service is not in the SMTP block set"}
	f.offer(t, "n8n")
	wantStatus(t, f.as(t, 9704, http.MethodPost, "/api/v1/domains/9903/service-apps/install", n8nInstallBody), http.StatusAccepted, "")
	op := f.waitOperation(t, "op-n8n-0001")
	if op.State != "failed" || op.Reason != "sandbox_proof" || !strings.Contains(op.Message, "not_in_smtp_set") {
		t.Fatalf("operation = %+v", op)
	}
	f.native.mu.Lock()
	defer f.native.mu.Unlock()
	if len(f.native.controls) != 1 || f.native.controls[0] != "stop" || len(f.native.renders) != 0 {
		t.Fatalf("an unproved app must be stopped and not published: controls %v renders %v", f.native.controls, f.native.renders)
	}
}
