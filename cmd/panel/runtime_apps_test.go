package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/siteruntime"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Runtime apps (alpha.119). The fixture is the authz matrix (see
// service_apps_test.go): reseller 9702's customer 9703 owns subscription 9802
// and domain 9902 (customer.matrix.test, with a certificate). The server side
// is a fake that behaves like the Agent: the entry check passes unless told,
// render writes a unit, restart starts it serving on loopback (or exposed, or
// failing its proof or health, when told).

type fakeRuntimeNative struct {
	mu        sync.Mutex
	level     string
	gaps      []string
	entryCode string
	units     map[int]transport.AppUnitState
	exposed   bool
	proofBad  bool
	healthBad bool
	calls     []string
	renders   []int
	envs      []transport.RuntimeAppEnvRequest
	removed   []transport.RuntimeAppRemoveRequest
}

func newFakeRuntimeNative() *fakeRuntimeNative {
	return &fakeRuntimeNative{level: "full", entryCode: siteruntime.EntryOK, units: map[int]transport.AppUnitState{}}
}

func (f *fakeRuntimeNative) note(call string) {
	f.calls = append(f.calls, call)
}

func (f *fakeRuntimeNative) mutations() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if c != "status" && c != "logs" && c != "caps" && c != "unitState" {
			n++
		}
	}
	return n
}

func (f *fakeRuntimeNative) check(_ context.Context, req transport.RuntimeAppCheckRequest) (transport.RuntimeAppCheckResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.note("check")
	resp := transport.RuntimeAppCheckResponse{Entry: transport.RuntimeAppEntryCheck{Code: f.entryCode}, Level: f.level}
	if f.entryCode != siteruntime.EntryOK {
		resp.Class, resp.Reason, resp.Error = transport.RuntimeAppPrerequisite, f.entryCode, "the entry is not ready"
		return resp, nil
	}
	resp.Done = true
	return resp, nil
}

func (f *fakeRuntimeNative) render(_ context.Context, req transport.RuntimeAppRenderRequest) (transport.RuntimeAppRenderResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.note("render")
	if _, ok := f.units[req.Target.AppID]; !ok {
		f.units[req.Target.AppID] = transport.AppUnitState{Present: true}
	}
	return transport.RuntimeAppRenderResponse{RuntimeAppStepResponse: transport.RuntimeAppStepResponse{Done: true}, UnitFile: "written"}, nil
}

func (f *fakeRuntimeNative) control(_ context.Context, req transport.RuntimeAppControlRequest) (transport.RuntimeAppStepResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.note("control:" + req.Action)
	unit := f.units[req.Target.AppID]
	unit.Present, unit.Active, unit.Enabled = true, req.Action != "stop", req.Action != "stop"
	f.units[req.Target.AppID] = unit
	return transport.RuntimeAppStepResponse{Done: true}, nil
}

func (f *fakeRuntimeNative) status(_ context.Context, req transport.RuntimeAppStatusRequest) (transport.RuntimeAppStatusResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.note("status")
	unit := f.units[req.Target.AppID]
	resp := transport.RuntimeAppStatusResponse{Unit: unit, UnitFile: "managed_unchanged", Level: f.level, Gaps: f.gaps}
	if req.Entry {
		resp.Entry = &transport.RuntimeAppEntryCheck{Code: f.entryCode}
	}
	if unit.Active {
		resp.ActiveState = "active"
		if f.exposed {
			resp.Exposed = []string{"0.0.0.0:30000"}
		} else {
			resp.Serving = true
		}
		if req.Proof {
			resp.Proof = &transport.SandboxProof{OK: !f.proofBad, Code: "ok"}
			if f.proofBad {
				resp.Proof.Code = "property_mismatch"
			}
		}
		if req.HealthPath != "" && resp.Serving {
			resp.Health = transport.RuntimeAppHealth{Checked: true, Status: 200, OK: !f.healthBad}
			if f.healthBad {
				resp.Health.Status = 502
			}
		}
	}
	return resp, nil
}

func (f *fakeRuntimeNative) env(_ context.Context, req transport.RuntimeAppEnvRequest) (transport.RuntimeAppStepResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.note("env")
	f.envs = append(f.envs, req)
	return transport.RuntimeAppStepResponse{Done: true}, nil
}

func (f *fakeRuntimeNative) logs(context.Context, transport.RuntimeAppLogsRequest) (transport.RuntimeAppLogsResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.note("logs")
	return transport.RuntimeAppLogsResponse{Lines: []string{"listening"}}, nil
}

func (f *fakeRuntimeNative) remove(_ context.Context, req transport.RuntimeAppRemoveRequest) (transport.RuntimeAppRemoveResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.note("remove")
	f.removed = append(f.removed, req)
	delete(f.units, req.Target.AppID)
	return transport.RuntimeAppRemoveResponse{RuntimeAppStepResponse: transport.RuntimeAppStepResponse{Done: true}}, nil
}

func (f *fakeRuntimeNative) unitState(_ context.Context, id int) (transport.ServiceAppUnitStateResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.note("unitState")
	return transport.ServiceAppUnitStateResponse{State: f.units[id]}, nil
}

func (f *fakeRuntimeNative) setSuspension(_ context.Context, req transport.SetRuntimeAppSuspensionRequest) (transport.SetAppUnitSuspensionResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.note("suspension")
	var resp transport.SetAppUnitSuspensionResponse
	current := f.units[req.AppID]
	target := transport.AppUnitState{Present: true}
	from := req.Before
	if !req.Suspend {
		target, from = req.Before, transport.AppUnitState{Present: true}
	}
	applied, change, conflict := casDecide(current, target, from)
	resp.Applied, resp.Changed, resp.Conflict = applied, change, conflict
	if change {
		f.units[req.AppID] = target
	}
	resp.State = f.units[req.AppID]
	return resp, nil
}

func (f *fakeRuntimeNative) renderSite(_ context.Context, domainID int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.note("site")
	f.renders = append(f.renders, domainID)
	return nil
}

func (f *fakeRuntimeNative) sandboxCaps(context.Context) (transport.SandboxCapsResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.note("caps")
	gaps := append([]string{}, f.gaps...)
	return transport.SandboxCapsResponse{Level: f.level, Gaps: gaps}, nil
}

type runtimeFixture struct {
	serviceAppFixture
	rt *fakeRuntimeNative
}

func newRuntimeFixture(t *testing.T) runtimeFixture {
	t.Helper()
	base := newServiceAppFixture(t)
	rt := newFakeRuntimeNative()
	runtimeAppNativeOverride = rt
	oldPoll, oldStart, oldHealth := runtimeAppPollInterval, runtimeAppStartTimeout, runtimeAppHealthTimeout
	runtimeAppPollInterval, runtimeAppStartTimeout, runtimeAppHealthTimeout = 2*time.Millisecond, 200*time.Millisecond, 200*time.Millisecond
	t.Cleanup(func() {
		runtimeAppNativeOverride = nil
		runtimeAppPollInterval, runtimeAppStartTimeout, runtimeAppHealthTimeout = oldPoll, oldStart, oldHealth
	})
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/domains/", base.panel.handleDomainSubroute)
	mux.HandleFunc(runtimeAppsAdminPath, base.panel.handleRuntimeAppsAdmin)
	mux.HandleFunc(runtimeAppsAdminPath+"/", base.panel.handleRuntimeAppsAdmin)
	base.handler = base.panel.requireAuth(mux)
	return runtimeFixture{serviceAppFixture: base, rt: rt}
}

const runtimePath = "/api/v1/domains/9902/runtime-apps"

func (f runtimeFixture) offerRuntime(t *testing.T, gaps string) *http.Response {
	t.Helper()
	rec := f.as(t, 9701, http.MethodPost, runtimeAppsAdminPath+"/program/offer",
		`{"risk_digest":"`+siteruntime.RiskDigest(siteruntime.KindProgram)+`","accepted_gaps":[`+gaps+`]}`)
	return rec.Result()
}

func (f runtimeFixture) waitRuntimeOperation(t *testing.T, id string) runtimeAppOperation {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		op, err := f.panel.runtimeAppOperationByID(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if op.State != "running" {
			return op
		}
		if time.Now().After(deadline) {
			t.Fatalf("operation still running at step %s", op.Step)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func (f runtimeFixture) create(t *testing.T) {
	t.Helper()
	wantStatus(t, f.as(t, 9703, http.MethodPost, runtimePath+"/create", `{"kind":"program","entry":"server","args":["-port","{port}"]}`), http.StatusCreated, "")
}

func (f runtimeFixture) view(t *testing.T, user int) runtimeAppsDomainView {
	t.Helper()
	rec := f.as(t, user, http.MethodGet, runtimePath, "")
	wantStatus(t, rec, http.StatusOK, "")
	var v runtimeAppsDomainView
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestRuntimeAppsAreClosedUntilOfferedAndLayersOnlyNarrow(t *testing.T) {
	f := newRuntimeFixture(t)
	v := f.view(t, 9703)
	if len(v.Kinds) != 1 || v.Kinds[0].Kind != "program" || v.Kinds[0].Offered || v.Kinds[0].RefusedBy != rightsLayerAdministrator {
		t.Fatalf("kinds before an offer: %+v", v.Kinds)
	}
	body := `{"kind":"program","entry":"server"}`
	wantStatus(t, f.as(t, 9703, http.MethodPost, runtimePath+"/create", body), http.StatusConflict, errCodeNotOffered)
	// The administrator acting for the account is refused by the same
	// administrator layer: nothing runs before it is offered.
	wantStatus(t, f.as(t, 9701, http.MethodPost, runtimePath+"/create", body), http.StatusConflict, errCodeNotOffered)
	wantStatus(t, f.as(t, 9703, http.MethodGet, runtimeAppsAdminPath, ""), http.StatusForbidden, "")
	wantStatus(t, f.as(t, 9701, http.MethodPost, runtimeAppsAdminPath+"/program/offer", `{"risk_digest":"stale"}`), http.StatusConflict, errCodeRuntimeApp)
	wantStatus(t, f.as(t, 9701, http.MethodPost, runtimeAppsAdminPath+"/python/offer", `{}`), http.StatusNotFound, "")
	wantStatus(t, f.as(t, 9701, http.MethodPost, runtimeAppsAdminPath+"/program/offer",
		`{"risk_digest":"`+siteruntime.RiskDigest(siteruntime.KindProgram)+`","memory_mb":1}`), http.StatusBadRequest, errCodeRuntimeApp)
	if f.offerRuntime(t, "").StatusCode != http.StatusOK {
		t.Fatal("offer")
	}

	// The reseller narrows the offering, then the permission.
	reseller := newRightsSet(true, false, "component:php-fpm:8.3")
	if err := saveRightsSet(context.Background(), f.panel.db.GetDB(), rightsOwnerReseller, 9702, reseller); err != nil {
		t.Fatal(err)
	}
	rec := f.as(t, 9703, http.MethodPost, runtimePath+"/create", body)
	wantStatus(t, rec, http.StatusConflict, errCodeNotOffered)
	if !strings.Contains(rec.Body.String(), `"reason":"reseller"`) {
		t.Fatalf("the reseller layer must be named: %s", rec.Body.String())
	}
	reseller = newRightsSet(false, true, permissionItemHead+rightMail)
	if err := saveRightsSet(context.Background(), f.panel.db.GetDB(), rightsOwnerReseller, 9702, reseller); err != nil {
		t.Fatal(err)
	}
	wantStatus(t, f.as(t, 9703, http.MethodPost, runtimePath+"/create", body), http.StatusForbidden, errCodeRightNotGranted)
	reseller = newRightsSet(true, true)
	if err := saveRightsSet(context.Background(), f.panel.db.GetDB(), rightsOwnerReseller, 9702, reseller); err != nil {
		t.Fatal(err)
	}

	// Invalid settings are refused with their reasons; nothing is stored.
	for _, bad := range []string{`{"entry":"../../bin/sh"}`, `{"entry":"server","args":["$HOME"]}`, `{"entry":"server","args":["a b"]}`, `{"entry":"server","health_path":"x"}`} {
		wantStatus(t, f.as(t, 9703, http.MethodPost, runtimePath+"/create", bad), http.StatusBadRequest, errCodeRuntimeApp)
	}
	f.create(t)
	wantStatus(t, f.as(t, 9703, http.MethodPost, runtimePath+"/create", body), http.StatusConflict, errCodeRuntimeApp)
	a, _, _ := f.panel.runtimeAppForDomain(context.Background(), 9902)
	if !siteruntime.ValidPort(a.Port) || a.Entry != "server" || a.State != runtimeAppCreated {
		t.Fatalf("app %+v", a)
	}

	// A withdrawn offer refuses new use; what exists stays readable,
	// stoppable and removable.
	wantStatus(t, f.as(t, 9701, http.MethodPost, runtimeAppsAdminPath+"/program/withdraw", `{}`), http.StatusOK, "")
	wantStatus(t, f.as(t, 9703, http.MethodPost, runtimePath+"/deploy", `{"operation_id":"op-runtime-01"}`), http.StatusConflict, errCodeNotOffered)
	v = f.view(t, 9703)
	if v.App == nil || !containsString(v.App.States, "not_offered") {
		t.Fatalf("states %+v", v.App)
	}
	wantStatus(t, f.as(t, 9703, http.MethodPost, runtimePath+"/control", `{"action":"stop"}`), http.StatusOK, "")
	wantStatus(t, f.as(t, 9703, http.MethodGet, runtimePath+"/logs", ""), http.StatusOK, "")
}

func TestRuntimeOfferOnAReducedHostNeedsEveryGapAccepted(t *testing.T) {
	f := newRuntimeFixture(t)
	f.rt.level, f.rt.gaps = "reduced", []string{"bpf_missing"}
	if f.offerRuntime(t, "").StatusCode != http.StatusConflict {
		t.Fatal("a reduced host was offered without accepting its gap")
	}
	if f.offerRuntime(t, `"nft_cgroup_missing"`).StatusCode != http.StatusConflict {
		t.Fatal("another gap's acceptance passed")
	}
	if f.offerRuntime(t, `"made_up"`).StatusCode != http.StatusConflict {
		t.Fatal("an unknown gap passed")
	}
	if f.offerRuntime(t, `"bpf_missing"`).StatusCode != http.StatusOK {
		t.Fatal("the accepted gap was refused")
	}
	o, _ := f.panel.loadRuntimeOfferings(context.Background())
	if got := o[siteruntime.OfferingProgram]; got.AcceptedLevel != "reduced" || strings.Join(got.AcceptedGaps, ",") != "bpf_missing" {
		t.Fatalf("offering %+v", got)
	}
	f.create(t)
	// The host loses another protection: nothing new starts until the
	// administrator accepts that gap too.
	f.rt.mu.Lock()
	f.rt.gaps = []string{"bpf_missing", "nft_cgroup_missing"}
	f.rt.mu.Unlock()
	rec := f.as(t, 9703, http.MethodPost, runtimePath+"/deploy", `{"operation_id":"op-runtime-02"}`)
	wantStatus(t, rec, http.StatusConflict, errCodeRuntimeApp)
	if !strings.Contains(rec.Body.String(), `"reason":"sandbox_reduced"`) {
		t.Fatalf("%s", rec.Body.String())
	}
	if v := f.view(t, 9703); !containsString(v.App.States, "sandbox_reduced") {
		t.Fatalf("states %v", v.App.States)
	}
	var admin struct {
		Kinds []runtimeAdminEntry `json:"kinds"`
	}
	recAdmin := f.as(t, 9701, http.MethodGet, runtimeAppsAdminPath, "")
	_ = json.Unmarshal(recAdmin.Body.Bytes(), &admin)
	if len(admin.Kinds) == 0 || !admin.Kinds[0].LevelGap {
		t.Fatalf("admin view: %s", recAdmin.Body.String())
	}
}

func TestRuntimeDeployIsOneDurableOperationAndPollingNeverMutates(t *testing.T) {
	f := newRuntimeFixture(t)
	f.offerRuntime(t, "")
	f.create(t)
	before := f.rt.mutations()
	for i := 0; i < 3; i++ {
		f.view(t, 9703)
	}
	if f.rt.mutations() != before {
		t.Fatalf("a status read changed something: %v", f.rt.calls)
	}
	rec := f.as(t, 9703, http.MethodPost, runtimePath+"/deploy", `{"operation_id":"op-runtime-03"}`)
	wantStatus(t, rec, http.StatusAccepted, "")
	// The answer was lost: the same id again answers the same operation.
	wantStatus(t, f.as(t, 9703, http.MethodPost, runtimePath+"/deploy", `{"operation_id":"op-runtime-03"}`), http.StatusOK, "")
	op := f.waitRuntimeOperation(t, "op-runtime-03")
	if op.State != "succeeded" {
		t.Fatalf("deploy: %+v", op)
	}
	renders := 0
	for _, c := range f.rt.calls {
		if c == "render" {
			renders++
		}
	}
	if renders != 1 {
		t.Fatalf("rendered %d times: %v", renders, f.rt.calls)
	}
	var projectType string
	var appPort int
	if err := f.panel.db.GetDB().QueryRow(`SELECT project_type, app_port FROM sites WHERE id = 9940`).Scan(&projectType, &appPort); err != nil {
		t.Fatal(err)
	}
	a, _, _ := f.panel.runtimeAppForDomain(context.Background(), 9902)
	if projectType != "runtime_app" || appPort != a.Port || !a.Deployed || a.State != runtimeAppRunning {
		t.Fatalf("published %s %d, app %+v", projectType, appPort, a)
	}
	// One operation per app: a running one refuses a second deploy.
	f.exec(t, `INSERT INTO runtime_app_operations (id, app_id, kind, step) VALUES ('op-runtime-held', ?, 'deploy', 'health')`, a.ID)
	rec = f.as(t, 9703, http.MethodPost, runtimePath+"/deploy", `{"operation_id":"op-runtime-04"}`)
	wantStatus(t, rec, http.StatusConflict, errCodeRuntimeApp)
	if !strings.Contains(rec.Body.String(), `"reason":"busy"`) {
		t.Fatalf("%s", rec.Body.String())
	}
	f.exec(t, `UPDATE runtime_app_operations SET state = 'failed' WHERE id = 'op-runtime-held'`)

	// Removal puts the site back and keeps the files unless asked.
	wantStatus(t, f.as(t, 9703, http.MethodPost, runtimePath+"/remove", `{"operation_id":"op-runtime-05","remove_files":true}`), http.StatusBadRequest, errCodeRuntimeApp)
	wantStatus(t, f.as(t, 9703, http.MethodPost, runtimePath+"/remove", `{"operation_id":"op-runtime-05"}`), http.StatusAccepted, "")
	if op := f.waitRuntimeOperation(t, "op-runtime-05"); op.State != "succeeded" {
		t.Fatalf("remove: %+v", op)
	}
	if err := f.panel.db.GetDB().QueryRow(`SELECT project_type FROM sites WHERE id = 9940`).Scan(&projectType); err != nil || projectType != "php" {
		t.Fatalf("site type after removal: %s %v", projectType, err)
	}
	if len(f.rt.removed) != 1 || f.rt.removed[0].RemoveFiles {
		t.Fatalf("removed %+v", f.rt.removed)
	}
}

func TestRuntimeDeployFailuresStopTheProgramAndSayWhy(t *testing.T) {
	cases := []struct {
		name    string
		set     func(*fakeRuntimeNative)
		reason  string
		class   string
		stopped bool
	}{
		{"entry missing", func(n *fakeRuntimeNative) { n.entryCode = siteruntime.EntryMissing }, siteruntime.EntryMissing, transport.RuntimeAppPrerequisite, false},
		{"listens elsewhere", func(n *fakeRuntimeNative) { n.exposed = true }, "listens_elsewhere", transport.RuntimeAppVerifiedFailure, true},
		{"proof", func(n *fakeRuntimeNative) { n.proofBad = true }, "sandbox_proof", transport.RuntimeAppVerifiedFailure, true},
		{"health", func(n *fakeRuntimeNative) { n.healthBad = true }, "health_failing", transport.RuntimeAppVerifiedFailure, false},
	}
	for i, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newRuntimeFixture(t)
			f.offerRuntime(t, "")
			f.create(t)
			c.set(f.rt)
			id := "op-runtime-f" + string(rune('a'+i)) + "000000"
			wantStatus(t, f.as(t, 9703, http.MethodPost, runtimePath+"/deploy", `{"operation_id":"`+id+`"}`), http.StatusAccepted, "")
			op := f.waitRuntimeOperation(t, id)
			if op.State != "failed" || op.Reason != c.reason || op.Class != c.class || op.Message == "" {
				t.Fatalf("op %+v", op)
			}
			stopped := containsString(f.rt.calls, "control:stop")
			if stopped != c.stopped {
				t.Fatalf("stopped=%v: %v", stopped, f.rt.calls)
			}
			if containsString(f.rt.calls, "site") {
				t.Fatal("a failed deploy published the site")
			}
		})
	}
}

func TestRuntimeHoldStopsEveryProgramAndRefusesStartButNeverStop(t *testing.T) {
	f := newRuntimeFixture(t)
	f.offerRuntime(t, "")
	f.create(t)
	wantStatus(t, f.as(t, 9703, http.MethodPost, runtimePath+"/deploy", `{"operation_id":"op-runtime-06"}`), http.StatusAccepted, "")
	f.waitRuntimeOperation(t, "op-runtime-06")
	wantStatus(t, f.as(t, 9701, http.MethodPost, runtimeAppsAdminPath+"/program/hold", `{}`), http.StatusBadRequest, errCodeRuntimeApp)
	rec := f.as(t, 9701, http.MethodPost, runtimeAppsAdminPath+"/program/hold", `{"reason":"advisory 2026-11"}`)
	wantStatus(t, rec, http.StatusOK, "")
	if !strings.Contains(rec.Body.String(), "customer.matrix.test") {
		t.Fatalf("hold: %s", rec.Body.String())
	}
	a, _, _ := f.panel.runtimeAppForDomain(context.Background(), 9902)
	if f.rt.units[a.ID].Active || a.State != runtimeAppStopped {
		t.Fatalf("held app still runs: %+v %+v", f.rt.units[a.ID], a)
	}
	for _, action := range []string{"start", "restart"} {
		rec = f.as(t, 9703, http.MethodPost, runtimePath+"/control", `{"action":"`+action+`"}`)
		if !strings.Contains(rec.Body.String(), `"reason":"held"`) {
			t.Fatalf("%s while held: %s", action, rec.Body.String())
		}
	}
	wantStatus(t, f.as(t, 9703, http.MethodPost, runtimePath+"/control", `{"action":"stop"}`), http.StatusOK, "")
	if v := f.view(t, 9703); !containsString(v.App.States, "held") {
		t.Fatalf("states %v", v.App.States)
	}
	wantStatus(t, f.as(t, 9701, http.MethodPost, runtimeAppsAdminPath+"/program/release", `{}`), http.StatusOK, "")
	if f.rt.units[a.ID].Active {
		t.Fatal("lifting the hold started a program")
	}
	wantStatus(t, f.as(t, 9703, http.MethodPost, runtimePath+"/control", `{"action":"start"}`), http.StatusOK, "")
}

func TestRuntimeEnvSendsSecretsOnceAndAuditsKeysOnly(t *testing.T) {
	f := newRuntimeFixture(t)
	f.offerRuntime(t, "")
	f.create(t)
	const secret = "tok-very-secret-value"
	wantStatus(t, f.as(t, 9703, http.MethodPut, runtimePath+"/env",
		`{"entries":[{"key":"LOG_LEVEL","value":"info"},{"key":"API_TOKEN","value":"`+secret+`","secret":true}]}`), http.StatusOK, "")
	for _, bad := range []string{
		`{"entries":[{"key":"LD_PRELOAD","value":"/x.so"}]}`,
		`{"entries":[{"key":"PORT","value":"80"}]}`,
		`{"entries":[{"key":"SHORT","value":"abc","secret":true}]}`,
		`{"entries":[{"key":"NEW_SECRET","secret":true}]}`,
	} {
		wantStatus(t, f.as(t, 9703, http.MethodPut, runtimePath+"/env", bad), http.StatusBadRequest, errCodeRuntimeApp)
	}
	// A kept secret goes back without its value.
	wantStatus(t, f.as(t, 9703, http.MethodPut, runtimePath+"/env",
		`{"entries":[{"key":"LOG_LEVEL","value":"debug"},{"key":"API_TOKEN","secret":true}]}`), http.StatusOK, "")
	last := f.rt.envs[len(f.rt.envs)-1]
	if strings.Join(last.PreviousKeys, ",") == "" || !containsString(last.PreviousKeys, "API_TOKEN") {
		t.Fatalf("previous keys %v", last.PreviousKeys)
	}
	var stored string
	if err := f.panel.db.GetDB().QueryRow(`SELECT env_public || env_secret_keys FROM runtime_apps WHERE domain_id = 9902`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	var audit string
	if err := f.panel.db.GetDB().QueryRow(`SELECT COALESCE(GROUP_CONCAT(COALESCE(details, '')), '') FROM audit_logs`).Scan(&audit); err != nil {
		t.Fatal(err)
	}
	rec := f.as(t, 9703, http.MethodGet, runtimePath, "")
	for name, text := range map[string]string{"database": stored, "audit": audit, "view": rec.Body.String()} {
		if strings.Contains(text, secret) {
			t.Fatalf("the secret reached the %s", name)
		}
	}
	if !strings.Contains(audit, "API_TOKEN") || !strings.Contains(stored, "API_TOKEN") {
		t.Fatalf("keys missing: %s / %s", audit, stored)
	}
}

func TestRuntimeAppSuspensionStopsAndResumeRestores(t *testing.T) {
	f := newSuspensionFixture(t)
	native := newFakeRuntimeNative()
	runtimeAppNativeOverride = native
	t.Cleanup(func() { runtimeAppNativeOverride = nil })
	f.exec(t, `INSERT INTO runtime_apps (id, subscription_id, domain_id, site_id, kind, offering_id, entry, port, state, deployed)
		VALUES (55, 9802, 9902, 9930, 'program', 'runtime:program', 'server', 30000, 'running', 1)`)
	native.units[55] = transport.AppUnitState{Present: true, Active: true, Enabled: true}
	v := f.view(t, 9802)
	if !strings.Contains(strings.Join(v.Preview.Apps, ","), "uploaded program on customer.matrix.test") {
		t.Fatalf("preview: %v", v.Preview.Apps)
	}
	wantStatus(t, f.as(t, 9701, http.MethodPost, "/api/v1/subscriptions/9802/suspension", `{"action":"suspend","reason":"unpaid"}`), http.StatusAccepted, "")
	f.run(t)
	if native.units[55] != (transport.AppUnitState{Present: true}) {
		t.Fatalf("suspended unit: %+v", native.units[55])
	}
	wantStatus(t, f.as(t, 9701, http.MethodPost, "/api/v1/subscriptions/9802/suspension", `{"action":"resume"}`), http.StatusAccepted, "")
	f.run(t)
	if native.units[55] != (transport.AppUnitState{Present: true, Active: true, Enabled: true}) {
		t.Fatalf("resumed unit: %+v", native.units[55])
	}
}

func TestRuntimeAppStatesNameEveryProblemMostBlockingFirst(t *testing.T) {
	if got := runtimeAppStates(runtimeGuidanceInput{AgentUnreachable: true, Status: &transport.RuntimeAppStatusResponse{StartLimitHit: true}}); strings.Join(got, ",") != "agent_unreachable" {
		t.Fatalf("an unknown status must not guess: %v", got)
	}
	got := runtimeAppStates(runtimeGuidanceInput{Suspended: true, Held: true, NoCertificate: true, DiskFull: true,
		Status: &transport.RuntimeAppStatusResponse{Entry: &transport.RuntimeAppEntryCheck{Code: "entry_not_executable"},
			StartLimitHit: true, OOMKills: 1}})
	if strings.Join(got, ",") != "suspended,held,no_certificate,disk_full,entry_not_executable,restart_loop,oom" {
		t.Fatalf("%v", got)
	}
	cases := map[string]transport.RuntimeAppStatusResponse{
		"listens_elsewhere": {Unit: transport.AppUnitState{Active: true}, Exposed: []string{"0.0.0.0:30000"}},
		"port_taken":        {Unit: transport.AppUnitState{Active: true}, Squatted: true},
		"not_listening":     {Unit: transport.AppUnitState{Active: true}},
		"health_failing":    {Unit: transport.AppUnitState{Active: true}, Serving: true, Health: transport.RuntimeAppHealth{Checked: true, Status: 500}},
		"entry_missing":     {Entry: &transport.RuntimeAppEntryCheck{Code: siteruntime.EntryInvalid}},
		"entry_unreadable":  {Entry: &transport.RuntimeAppEntryCheck{Code: "site_account"}},
		"oom":               {Result: "oom-kill"},
	}
	for want, status := range cases {
		s := status
		if got := runtimeAppStates(runtimeGuidanceInput{Status: &s}); strings.Join(got, ",") != want {
			t.Errorf("%s: %v", want, got)
		}
	}
	if got := runtimeAppStates(runtimeGuidanceInput{Status: &transport.RuntimeAppStatusResponse{Unit: transport.AppUnitState{Active: true}, Serving: true,
		Health: transport.RuntimeAppHealth{Checked: true, Status: 200, OK: true}, Entry: &transport.RuntimeAppEntryCheck{Code: "ok"}}}); len(got) != 0 {
		t.Fatalf("a healthy app has states: %v", got)
	}
}

func TestRuntimeOfferingIDIsARightsItem(t *testing.T) {
	if !validOfferingID("runtime:program") || validOfferingID("runtime:node") || !knownPermission(rightProgramApps) {
		t.Fatal("rights ids")
	}
	if !containsString(runtimeCandidateOfferings(), "runtime:program") {
		t.Fatal("candidates")
	}
	if domainSubrouteRightFor("runtime-apps-deploy", http.MethodPost) != rightProgramApps ||
		domainSubrouteRightFor("runtime-apps-remove", http.MethodPost) != "" ||
		domainSubrouteRightFor("runtime-apps-control", http.MethodPost) != "" {
		t.Fatal("subroute rights")
	}
}
