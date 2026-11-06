package main

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"testing"

	"github.com/alicelik/celikpanel/internal/sitesandbox"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Node.js sites in the site sandbox (alpha.127): the administrator's page
// reads; only the one action moves, once, with the expected count.

type fakeNodeSandbox struct {
	mu       sync.Mutex
	units    map[int]transport.SandboxUnit
	applied  []transport.AppApplyRequest
	putBack  map[int]bool
	capReads int
}

func newNodeSandboxFixture(t *testing.T) (serviceAppFixture, *fakeNodeSandbox) {
	t.Helper()
	f := newServiceAppFixture(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/domains/", f.panel.handleDomainSubroute)
	mux.HandleFunc("/api/v1/runtimes/node/", f.panel.handleNodeRuntimeSub)
	f.handler = f.panel.requireAuth(mux)
	// 9902: moves (site account, running); 9903: shared web account.
	f.exec(t, `UPDATE sites SET project_type = 'node', app_port = 3001, start_command = 'npm start', runtime_version = '22.11.0', app_run_as = 'site' WHERE id = 9940`)
	f.exec(t, `UPDATE sites SET project_type = 'node', app_port = 3002, start_command = 'node app.js', runtime_version = '22.11.0' WHERE id = 9941`)
	fake := &fakeNodeSandbox{putBack: map[int]bool{}, units: map[int]transport.SandboxUnit{
		9940: {Kind: "node", SiteID: 9940, Unit: "celikapp-9940.service", File: "ours", Generation: 0, Active: "active"},
		9941: {Kind: "node", SiteID: 9941, Unit: "celikapp-9941.service", File: "ours", Generation: 0, Active: "active"},
	}}
	oldCaps, oldApply := nodeSandboxCaps, nodeSandboxApply
	t.Cleanup(func() { nodeSandboxCaps, nodeSandboxApply = oldCaps, oldApply })
	nodeSandboxCaps = func(*Panel, context.Context) (transport.SandboxCapsResponse, error) {
		fake.mu.Lock()
		defer fake.mu.Unlock()
		fake.capReads++
		resp := transport.SandboxCapsResponse{Level: "reduced", Gaps: []string{"bpf_missing"}, Generation: sitesandbox.Generation}
		for _, u := range fake.units {
			resp.Units = append(resp.Units, u)
		}
		return resp, nil
	}
	nodeSandboxApply = func(_ *Panel, _ context.Context, req *transport.AppApplyRequest) (transport.AppApplyResponse, error) {
		fake.mu.Lock()
		defer fake.mu.Unlock()
		fake.applied = append(fake.applied, *req)
		if fake.putBack[req.SiteID] {
			return transport.AppApplyResponse{Unit: "celikapp", Error: "put back",
				Sandbox: &transport.NodeSandboxResult{PutBack: true, Reason: "not_running"}}, nil
		}
		u := fake.units[req.SiteID]
		u.Generation = sitesandbox.Generation
		fake.units[req.SiteID] = u
		return transport.AppApplyResponse{Unit: "celikapp", Sandbox: &transport.NodeSandboxResult{Generation: sitesandbox.Generation, Level: "reduced"}}, nil
	}
	return f, fake
}

func nodeSandboxPage(t *testing.T, f serviceAppFixture) nodeSandboxView {
	t.Helper()
	rec := f.as(t, 9701, http.MethodGet, "/api/v1/runtimes/node/sandbox", "")
	wantStatus(t, rec, http.StatusOK, "")
	var view nodeSandboxView
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	return view
}

func TestNodeSandboxMoveIsExplicitAndOnce(t *testing.T) {
	f, fake := newNodeSandboxFixture(t)
	view := nodeSandboxPage(t, f)
	if !view.Known || len(view.NeedsMove) != 1 || view.Running != 1 || len(view.SharedAccount) != 1 || view.Level != "reduced" || view.Gaps[0] != "bpf_missing" {
		t.Fatalf("view = %+v", view)
	}
	if len(fake.applied) != 0 {
		t.Fatal("a read moved a site")
	}
	// Only the administrator; the count must hold.
	wantStatus(t, f.as(t, 9703, http.MethodPost, "/api/v1/runtimes/node/sandbox-refresh", `{"request_id":"node-move-0001","expected":1}`), http.StatusForbidden, "")
	wantStatus(t, f.as(t, 9701, http.MethodPost, "/api/v1/runtimes/node/sandbox-refresh", `{"request_id":"node-move-0001","expected":2}`), http.StatusConflict, errCodeServiceApp)
	if len(fake.applied) != 0 {
		t.Fatal("a refused request moved a site")
	}
	rec := f.as(t, 9701, http.MethodPost, "/api/v1/runtimes/node/sandbox-refresh", `{"request_id":"node-move-0002","expected":1}`)
	wantStatus(t, rec, http.StatusOK, "")
	var result nodeSandboxResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil || len(result.Moved) != 1 || len(result.PutBack) != 0 {
		t.Fatalf("result = %+v (%v)", result, err)
	}
	if len(fake.applied) != 1 || !fake.applied[0].SandboxMove || fake.applied[0].NoStart || fake.applied[0].SiteID != 9940 || fake.applied[0].RunAsUser == "www-data" {
		t.Fatalf("applied = %+v", fake.applied)
	}
	// The lost answer: the same id answers again and moves nothing.
	wantStatus(t, f.as(t, 9701, http.MethodPost, "/api/v1/runtimes/node/sandbox-refresh", `{"request_id":"node-move-0002","expected":1}`), http.StatusOK, "")
	if len(fake.applied) != 1 {
		t.Fatalf("a repeat moved again: %d", len(fake.applied))
	}
	if v := nodeSandboxPage(t, f); len(v.NeedsMove) != 0 || v.Sandboxed != 1 || len(v.SharedAccount) != 1 {
		t.Fatalf("after = %+v", v)
	}
	var count int
	_ = f.panel.db.GetDB().QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action = 'site.node_sandbox_refresh'`).Scan(&count)
	if count != 1 {
		t.Fatalf("audit rows = %d", count)
	}
}

func TestNodeSandboxMoveStoppedEditedAndPutBack(t *testing.T) {
	f, fake := newNodeSandboxFixture(t)
	f.exec(t, `UPDATE sites SET app_run_as = 'site' WHERE id = 9941`)
	fake.mu.Lock()
	u := fake.units[9941]
	u.Active = "inactive"
	fake.units[9941] = u
	fake.putBack[9940] = true
	fake.mu.Unlock()
	rec := f.as(t, 9701, http.MethodPost, "/api/v1/runtimes/node/sandbox-refresh", `{"request_id":"node-move-0003","expected":2}`)
	wantStatus(t, rec, http.StatusOK, "")
	var result nodeSandboxResult
	_ = json.Unmarshal(rec.Body.Bytes(), &result)
	if len(result.Rewritten) != 1 || len(result.PutBack) != 1 || result.PutBack[0].Reason != "not_running" || len(result.Moved) != 0 {
		t.Fatalf("result = %+v", result)
	}
	for _, req := range fake.applied {
		if req.SiteID == 9941 && !req.NoStart {
			t.Fatal("a stopped application was asked to start")
		}
	}
	// An owner-changed unit is listed, never moved.
	fake.mu.Lock()
	u = fake.units[9940]
	u.File, u.Generation = "edited", 0
	fake.units[9940] = u
	fake.mu.Unlock()
	if v := nodeSandboxPage(t, f); len(v.Edited) != 1 || len(v.NeedsMove) != 0 {
		t.Fatalf("edited = %+v", v)
	}
}
