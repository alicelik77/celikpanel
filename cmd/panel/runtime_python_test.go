package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/siteruntime"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Python apps (alpha.119, W5) on the runtime-app engine: installed before
// offered (the server's own Python with its venv module), closed until the
// administrator offers component:python:<minor>, the python_apps permission,
// the build step before render, a failed build that changes nothing that
// runs, and the one-step rollback of a redeploy that does not come up.

func (f runtimeFixture) offerPython(t *testing.T, sourceBuilds bool) *http.Response {
	t.Helper()
	body := `{"risk_digest":"` + siteruntime.RiskDigestFor(siteruntime.KindPython, sourceBuilds) + `"`
	if sourceBuilds {
		body += `,"source_builds":true`
	}
	return f.as(t, 9701, http.MethodPost, runtimeAppsAdminPath+"/python/offer", body+`}`).Result()
}

const pythonCreate = `{"kind":"python","entry":"main:app","server":"asgi","workers":2,"health_path":"/healthz"}`

func (f runtimeFixture) deployPython(t *testing.T, id string) runtimeAppOperation {
	t.Helper()
	wantStatus(t, f.as(t, 9703, http.MethodPost, runtimePath+"/deploy", `{"operation_id":"`+id+`"}`), http.StatusAccepted, "")
	return f.waitRuntimeOperation(t, id)
}

func TestPythonAppsNeedTheServersPythonThenAnOffer(t *testing.T) {
	f := newRuntimeFixture(t)
	// Not installed: nothing can be offered or created, and the
	// administrator is named.
	rec := f.as(t, 9703, http.MethodPost, runtimePath+"/create", pythonCreate)
	wantStatus(t, rec, http.StatusConflict, errCodeRuntimeApp)
	if !strings.Contains(rec.Body.String(), `"reason":"runtime_missing"`) || !strings.Contains(rec.Body.String(), "venv module") {
		t.Fatalf("%s", rec.Body.String())
	}
	if f.offerPython(t, false).StatusCode != http.StatusConflict {
		t.Fatal("Python was offered before it was installed")
	}
	f.rt.mu.Lock()
	f.rt.pyReady = true
	f.rt.mu.Unlock()
	// Installed, not offered: closed by the administrator layer.
	wantStatus(t, f.as(t, 9703, http.MethodPost, runtimePath+"/create", pythonCreate), http.StatusConflict, errCodeNotOffered)
	if f.as(t, 9701, http.MethodPost, runtimeAppsAdminPath+"/python/offer", `{"risk_digest":"`+siteruntime.RiskDigest(siteruntime.KindPython)+`","source_builds":true}`).Code != http.StatusConflict {
		t.Fatal("source builds were allowed with the digest of a summary that does not name them")
	}
	if f.offerPython(t, false).StatusCode != http.StatusOK {
		t.Fatal("offer")
	}
	offerings, _ := f.panel.loadRuntimeOfferings(context.Background())
	o, ok := offerings["component:python:3.13"]
	if !ok || !runtimeOfferOpen(o) || runtimeOfferSourceBuilds(o) || o.Kind != siteruntime.KindPython {
		t.Fatalf("offering %+v", offerings)
	}
	// The permission is python_apps: a reseller that keeps program_apps
	// only refuses Python apps.
	reseller := newRightsSet(false, true, permissionItemHead+rightProgramApps)
	if err := saveRightsSet(context.Background(), f.panel.db.GetDB(), rightsOwnerReseller, 9702, reseller); err != nil {
		t.Fatal(err)
	}
	wantStatus(t, f.as(t, 9703, http.MethodPost, runtimePath+"/create", pythonCreate), http.StatusForbidden, errCodeRightNotGranted)
	if err := saveRightsSet(context.Background(), f.panel.db.GetDB(), rightsOwnerReseller, 9702, newRightsSet(true, true)); err != nil {
		t.Fatal(err)
	}
	// Settings are checked: module:callable, the server, workers capped by memory.
	for _, bad := range []string{
		`{"kind":"python","entry":"main:create_app()","server":"asgi"}`,
		`{"kind":"python","entry":"main","server":"asgi"}`,
		`{"kind":"python","entry":"main:app","server":"cgi"}`,
		`{"kind":"python","entry":"main:app","server":"wsgi","workers":5}`,
		`{"kind":"python","entry":"main:app;rm","server":"wsgi"}`,
		`{"kind":"python","entry":"main:app","server":"wsgi","health_path":"x"}`,
	} {
		wantStatus(t, f.as(t, 9703, http.MethodPost, runtimePath+"/create", bad), http.StatusBadRequest, errCodeRuntimeApp)
	}
	wantStatus(t, f.as(t, 9703, http.MethodPost, runtimePath+"/create", pythonCreate), http.StatusCreated, "")
	a, _, _ := f.panel.runtimeAppForDomain(context.Background(), 9902)
	if a.Kind != siteruntime.KindPython || a.OfferingID != "component:python:3.13" || a.Entry != "main:app" || strings.Join(a.Args, ",") != "server=asgi,workers=2" {
		t.Fatalf("app %+v", a)
	}
	v := f.view(t, 9703)
	if v.App == nil || v.App.Python == nil || v.App.Python.Server != "asgi" || v.App.Python.Workers != 2 || v.App.UploadDir != "app/src" || len(v.App.Args) != 0 {
		t.Fatalf("view %+v", v.App)
	}
	if v.Kinds[1].Python == nil || v.Kinds[1].Python.MaxWorkers != 4 || !v.Kinds[1].Offered {
		t.Fatalf("python kind %+v", v.Kinds[1])
	}
	// The administrator view names both summaries.
	var admin struct {
		Kinds []runtimeAdminEntry `json:"kinds"`
	}
	_ = json.Unmarshal(f.as(t, 9701, http.MethodGet, runtimeAppsAdminPath, "").Body.Bytes(), &admin)
	if len(admin.Kinds) != 2 || admin.Kinds[1].OfferingID != "component:python:3.13" || admin.Kinds[1].RiskDigestSourceBuilds == admin.Kinds[1].RiskDigest ||
		!containsString(admin.Kinds[1].RiskSourceBuilds.Remains, "source_builds") || containsString(admin.Kinds[1].Risk.Remains, "source_builds") || !admin.Kinds[1].Offered {
		t.Fatalf("admin %+v", admin.Kinds)
	}
}

func TestPythonDeployBuildsThenRendersTheBuiltEnvironment(t *testing.T) {
	f := newRuntimeFixture(t)
	f.rt.pyReady = true
	f.offerPython(t, true)
	wantStatus(t, f.as(t, 9703, http.MethodPost, runtimePath+"/create", pythonCreate), http.StatusCreated, "")
	op := f.deployPython(t, "op-python-0001")
	if op.State != "succeeded" {
		t.Fatalf("deploy %+v %v", op, f.rt.calls)
	}
	build, render := -1, -1
	for i, c := range f.rt.calls {
		if c == "build" && build < 0 {
			build = i
		}
		if c == "render" && render < 0 {
			render = i
		}
	}
	if build < 0 || render < build {
		t.Fatalf("the build must come before the render: %v", f.rt.calls)
	}
	if len(f.rt.buildReqs) != 1 || f.rt.buildReqs[0].Spec.Python == nil || !f.rt.buildReqs[0].Spec.Python.SourceBuilds ||
		f.rt.buildReqs[0].Spec.Python.Server != "asgi" || f.rt.buildReqs[0].Spec.Entry != "main:app" {
		t.Fatalf("build %+v", f.rt.buildReqs)
	}
	if strings.Join(intsToStrings(f.rt.renderVenv), ",") != "1" {
		t.Fatalf("rendered %v", f.rt.renderVenv)
	}
	if !containsString(f.rt.calls, "site") {
		t.Fatal("not published")
	}
}

func TestPythonBuildFailureChangesNothingThatRuns(t *testing.T) {
	f := newRuntimeFixture(t)
	f.rt.pyReady = true
	f.offerPython(t, false)
	wantStatus(t, f.as(t, 9703, http.MethodPost, runtimePath+"/create", pythonCreate), http.StatusCreated, "")
	if op := f.deployPython(t, "op-python-0002"); op.State != "succeeded" {
		t.Fatalf("first deploy %+v", op)
	}
	f.rt.mu.Lock()
	f.rt.buildFail = siteruntime.WheelUnavailable
	f.rt.calls = nil
	f.rt.mu.Unlock()
	op := f.deployPython(t, "op-python-0003")
	if op.State != "failed" || op.Reason != siteruntime.WheelUnavailable || op.Class != transport.RuntimeAppVerifiedFailure || !strings.Contains(op.Message, "numpy") {
		t.Fatalf("op %+v", op)
	}
	for _, c := range f.rt.calls {
		if c == "render" || strings.HasPrefix(c, "control:") || c == "site" {
			t.Fatalf("a failed build touched the running app: %v", f.rt.calls)
		}
	}
	a, _, _ := f.panel.runtimeAppForDomain(context.Background(), 9902)
	if !a.Deployed || f.rt.venvs[a.ID] != 1 {
		t.Fatalf("app %+v venv %d", a, f.rt.venvs[a.ID])
	}
}

func TestPythonRedeployThatDoesNotComeUpGoesBackOneStep(t *testing.T) {
	f := newRuntimeFixture(t)
	f.rt.pyReady = true
	f.offerPython(t, false)
	wantStatus(t, f.as(t, 9703, http.MethodPost, runtimePath+"/create", pythonCreate), http.StatusCreated, "")
	if op := f.deployPython(t, "op-python-0004"); op.State != "succeeded" {
		t.Fatalf("first deploy %+v", op)
	}
	f.rt.mu.Lock()
	f.rt.healthBad = true
	f.rt.mu.Unlock()
	op := f.deployPython(t, "op-python-0005")
	if op.State != "failed" || op.Reason != "health_failing" || !strings.Contains(op.Message, "previous environment (venv-1)") {
		t.Fatalf("op %+v", op)
	}
	if got := strings.Join(intsToStrings(f.rt.renderVenv), ","); got != "1,2,1" {
		t.Fatalf("renders %s", got)
	}
	// A first deploy has nothing to go back to.
	g := newRuntimeFixture(t)
	g.rt.pyReady, g.rt.healthBad = true, true
	g.offerPython(t, false)
	wantStatus(t, g.as(t, 9703, http.MethodPost, runtimePath+"/create", pythonCreate), http.StatusCreated, "")
	if op := g.deployPython(t, "op-python-0006"); op.State != "failed" || strings.Contains(op.Message, "previous environment") {
		t.Fatalf("first deploy %+v", op)
	}
	if got := strings.Join(intsToStrings(g.rt.renderVenv), ","); got != "1" {
		t.Fatalf("renders %s", got)
	}
}

func TestPythonOfferingIsClosedUntilOfferedInTheRightsModel(t *testing.T) {
	layer := adminOfferLayer{hidden: map[string]bool{}, runtimes: map[string]bool{}}
	if layer.offers("component:python:3.13") {
		t.Fatal("a Python line is open before the administrator offers it")
	}
	layer.runtimes["component:python:3.13"] = true
	if !layer.offers("component:python:3.13") || layer.offers("component:python:3.11") {
		t.Fatal("only the offered line opens")
	}
	layer.hidden["component:python"] = true
	if layer.offers("component:python:3.13") {
		t.Fatal("a hidden component stays closed")
	}
	if k, ok := siteruntime.KindOfOffering("component:python:3.13"); !ok || k != siteruntime.KindPython {
		t.Fatal("kind of offering")
	}
}

func intsToStrings(list []int) []string {
	out := make([]string, len(list))
	for i, n := range list {
		out[i] = strconv.Itoa(n)
	}
	return out
}
