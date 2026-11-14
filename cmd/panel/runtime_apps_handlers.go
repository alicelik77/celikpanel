package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/siteruntime"
	"github.com/alicelik/celikpanel/internal/sitesandbox"
	"github.com/alicelik/celikpanel/internal/transport"
)

// The domain's "Programs" screen and its actions. Every refusal names why (a
// reason code the screen words in English and Turkish), who acts and what to
// do next. Secret values go to the server once and are never shown again.
//
// Alan adının "Programlar" ekranı ve eylemleri. Her ret nedenini, kimin ne
// yapacağını söyler; gizli değerler bir kez gider, bir daha görünmez.

func writeRuntimeAppRefusal(w http.ResponseWriter, status int, reason, message string, vars map[string]string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(apiErrorBody{Error: message, Code: errCodeRuntimeApp, Reason: reason, Vars: vars})
}

func (p *Panel) handleDomainRuntimeApps(w http.ResponseWriter, r *http.Request, domainID int, kind string) {
	switch kind {
	case "runtime-apps":
		p.writeRuntimeAppsView(w, r, domainID)
	case "runtime-apps-create":
		p.handleRuntimeAppCreate(w, r, domainID)
	case "runtime-apps-settings":
		p.handleRuntimeAppSettings(w, r, domainID)
	case "runtime-apps-deploy":
		p.handleRuntimeAppDeploy(w, r, domainID)
	case "runtime-apps-control":
		p.handleRuntimeAppControl(w, r, domainID)
	case "runtime-apps-env":
		p.handleRuntimeAppEnv(w, r, domainID)
	case "runtime-apps-logs":
		p.handleRuntimeAppLogs(w, r, domainID)
	case "runtime-apps-remove":
		p.handleRuntimeAppRemove(w, r, domainID)
	default:
		http.NotFound(w, r)
	}
}

// runtimeHostLevel reads the host's sandbox level (read-only, cached by the
// Agent for a minute).
func (p *Panel) runtimeHostLevel(ctx context.Context) (transport.SandboxCapsResponse, error) {
	callCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	caps, err := p.runtimeAppNativeOps().sandboxCaps(callCtx)
	if err == nil && caps.Error != "" {
		err = errors.New(caps.Error)
	}
	return caps, err
}

// runtimeLevelRefusal: "" when the offering may run on a host at this level;
// otherwise why not (a reduced host whose every gap the administrator has
// not accepted for the offering).
func runtimeLevelRefusal(caps transport.SandboxCapsResponse, o runtimeOffering) string {
	if caps.Level != string(sitesandbox.LevelReduced) {
		return ""
	}
	gaps := make([]sitesandbox.Gap, len(caps.Gaps))
	for i, g := range caps.Gaps {
		gaps[i] = sitesandbox.Gap(g)
	}
	if o.AcceptedLevel == string(sitesandbox.LevelReduced) && siteruntime.GapsCovered(gaps, o.AcceptedGaps) {
		return ""
	}
	return strings.Join(caps.Gaps, ", ")
}

// runtimeKindRefusal is the layer check of new use (create, settings,
// deploy): the security hold, the administrator's offer (with the risk digest
// of this release), the host's level against what was accepted, the
// permission and the offering at the reseller and subscription layers. It
// returns the HTTP status, reason, message and vars ("" reason = allowed).
type runtimeRefusal struct {
	status  int
	reason  string
	message string
	vars    map[string]string
	// rights: the refusal is a rights layer one, written by writeNotOffered
	// or writeRightNotGranted.
	layer      string
	permission bool
	e          *effectiveRights
	// offeringID is the offering refused at a rights layer.
	offeringID string
}

func (p *Panel) runtimeKindRefusal(ctx context.Context, r *http.Request, kind siteruntime.Kind, subID int) (*runtimeRefusal, error) {
	if !siteruntime.Implemented(kind) {
		return &runtimeRefusal{status: http.StatusConflict, reason: "kind_unavailable",
			message: "Nothing was changed: this kind of app cannot run in this release."}, nil
	}
	// Installed before offered: a Python app needs the server's own Python
	// with its venv module, which the administrator installs in Components.
	offeringID, py := p.runtimeKindOffering(ctx, kind)
	if kind == siteruntime.KindPython && (py == nil || !py.Ready || offeringID == "") {
		return runtimePythonMissingRefusal(py), nil
	}
	offerings, err := p.loadRuntimeOfferings(ctx)
	if err != nil {
		return nil, err
	}
	o, has := offerings[offeringID]
	if has && o.Held {
		return &runtimeRefusal{status: http.StatusConflict, reason: "held",
			message: "Nothing was changed: the administrator has put " + runtimeKindName(kind) + " on a security hold. They can be deployed or started again when the administrator lifts the hold."}, nil
	}
	e, err := p.serviceAppRights(ctx, r, subID)
	if err != nil {
		return nil, err
	}
	if !has || !runtimeOfferOpen(o) {
		return &runtimeRefusal{layer: rightsLayerAdministrator, e: e, offeringID: offeringID}, nil
	}
	if layer := e.permissionRefusal(siteruntime.Permission(kind)); layer != "" {
		return &runtimeRefusal{layer: layer, permission: true, e: e}, nil
	}
	if layer := e.offeringRefusal(offeringID); layer != "" {
		return &runtimeRefusal{layer: layer, e: e, offeringID: offeringID}, nil
	}
	caps, err := p.runtimeHostLevel(ctx)
	if err != nil {
		return &runtimeRefusal{status: http.StatusBadGateway, reason: "agent_unreachable",
			message: "Nothing was changed: the server did not say which sandbox level it enforces. Try again in a minute."}, nil
	}
	if gaps := runtimeLevelRefusal(caps, o); gaps != "" {
		return &runtimeRefusal{status: http.StatusConflict, reason: "sandbox_reduced",
			message: "Nothing was changed: this server does not enforce the full sandbox (" + gaps + "), and the administrator has not accepted these gaps for " + runtimeKindName(kind) + ". The administrator reviews them in the Store's catalog management.",
			vars:    map[string]string{"gaps": gaps}}, nil
	}
	return nil, nil
}

func writeRuntimeRefusal(w http.ResponseWriter, refusal *runtimeRefusal, kind siteruntime.Kind) {
	switch {
	case refusal.e != nil && refusal.permission:
		writeRightNotGranted(w, refusal.e, siteruntime.Permission(kind), refusal.layer)
	case refusal.e != nil:
		name := "Uploaded programs"
		if kind == siteruntime.KindPython {
			name = "Python apps"
		}
		writeNotOffered(w, refusal.e, refusal.offeringID, name, refusal.layer)
	default:
		writeRuntimeAppRefusal(w, refusal.status, refusal.reason, refusal.message, refusal.vars)
	}
}

// runtimePythonMissingRefusal: the server's own Python cannot build apps
// (not installed, no venv module, another Python, or no pins in this
// release). The administrator acts; the detail says what is missing.
func runtimePythonMissingRefusal(py *transport.RuntimePythonResponse) *runtimeRefusal {
	detail := "the server did not say which Python it has"
	if py != nil && py.Detail != "" {
		detail = py.Detail
	} else if py != nil && py.Error != "" {
		detail = py.Error
	}
	return &runtimeRefusal{status: http.StatusConflict, reason: siteruntime.PythonRuntimeMissing,
		message: "Nothing was changed: Python apps need this server's own Python with its venv module (" + detail + "). The administrator installs Python in Components.",
		vars:    map[string]string{"detail": detail}}
}

// refuseRuntimeNewUse applies every gate of new use for a domain's app: the
// site, the suspension, the rights layers, the host level and the disk
// (D-044). It writes the refusal and returns true when refused.
func (p *Panel) refuseRuntimeNewUse(w http.ResponseWriter, r *http.Request, kind siteruntime.Kind, site serviceAppSite) bool {
	ctx := r.Context()
	if site.SiteID <= 0 || site.ProjectType == "dnsonly" {
		writeRuntimeAppRefusal(w, http.StatusConflict, "no_site",
			"Nothing was changed: this domain has no website on this server. Add hosting to the domain first.", nil)
		return true
	}
	if site.Suspended {
		writeRuntimeAppRefusal(w, http.StatusConflict, "suspended",
			"Nothing was changed: this subscription is suspended. Programs can be deployed again when it is resumed.", nil)
		return true
	}
	refusal, err := p.runtimeKindRefusal(ctx, r, kind, site.SubscriptionID)
	if err != nil {
		writeServerError(w, err)
		return true
	}
	if refusal != nil {
		writeRuntimeRefusal(w, refusal, kind)
		return true
	}
	if err := p.checkSubscriptionQuota(ctx, site.SubscriptionID, quotaDisk); err != nil {
		writeQuotaRefusal(w, err)
		return true
	}
	return false
}

type runtimeKindView struct {
	Kind              string             `json:"kind"`
	Implemented       bool               `json:"implemented"`
	OfferingID        string             `json:"offering_id"`
	Offered           bool               `json:"offered"`
	RefusedBy         string             `json:"refused_by,omitempty"`
	Grantor           string             `json:"grantor,omitempty"`
	PermissionGranted bool               `json:"permission_granted"`
	PermissionRefused string             `json:"permission_refused_by,omitempty"`
	Held              bool               `json:"held"`
	Risk              siteruntime.Risk   `json:"risk"`
	Limits            siteruntime.Limits `json:"limits"`
	AcceptedLevel     string             `json:"accepted_level"`
	AcceptedGaps      []string           `json:"accepted_gaps"`
	// Python is the server's Python for the Python kind.
	Python *runtimePythonView `json:"python,omitempty"`
}

// runtimePythonView is the distribution Python as the screens show it.
type runtimePythonView struct {
	Minor        string   `json:"minor,omitempty"`
	Ready        bool     `json:"ready"`
	Detail       string   `json:"detail,omitempty"`
	Packages     []string `json:"packages,omitempty"`
	MaxWorkers   int      `json:"max_workers"`
	SourceBuilds bool     `json:"source_builds"`
}

func pythonView(py *transport.RuntimePythonResponse, limits siteruntime.Limits, sourceBuilds bool) *runtimePythonView {
	v := &runtimePythonView{MaxWorkers: siteruntime.MaxWorkers(limits.MemoryMB), SourceBuilds: sourceBuilds}
	if py != nil {
		v.Minor, v.Ready, v.Detail, v.Packages = py.Minor, py.Ready, py.Detail, py.Packages
		if v.Detail == "" && py.Error != "" {
			v.Detail = py.Error
		}
	}
	return v
}

type runtimeEnvView struct {
	Key    string `json:"key"`
	Value  string `json:"value,omitempty"`
	Secret bool   `json:"secret"`
}

type runtimeAppView struct {
	ID         int              `json:"id"`
	Kind       string           `json:"kind"`
	Entry      string           `json:"entry"`
	Args       []string         `json:"args"`
	HealthPath string           `json:"health_path"`
	Port       int              `json:"port"`
	State      string           `json:"state"`
	Deployed   bool             `json:"deployed"`
	URL        string           `json:"url"`
	Folder     string           `json:"folder"`
	UploadDir  string           `json:"upload_dir"`
	Unit       string           `json:"unit"`
	Env        []runtimeEnvView `json:"env"`
	// Python: the Python app's settings (nil for a program).
	Python *siteruntime.PythonSettings `json:"python,omitempty"`
	// Status is the server's read; StatusUnknown when it did not answer.
	Status        *transport.RuntimeAppStatusResponse `json:"status,omitempty"`
	StatusUnknown bool                                `json:"status_unknown,omitempty"`
	// States are the D-024 states that apply now, most blocking first
	// (runtime.state.<code> on screen).
	States    []string `json:"states"`
	CreatedAt string   `json:"created_at"`
}

type runtimeAppsDomainView struct {
	Domain      string               `json:"domain"`
	Suspended   bool                 `json:"suspended"`
	HasSite     bool                 `json:"has_site"`
	ProjectType string               `json:"project_type"`
	Certificate bool                 `json:"certificate"`
	DiskFull    bool                 `json:"disk_full"`
	Sandbox     serviceAppSandboxLvl `json:"sandbox"`
	Kinds       []runtimeKindView    `json:"kinds"`
	App         *runtimeAppView      `json:"app"`
	Operation   *runtimeAppOperation `json:"operation"`
	PortRange   [2]int               `json:"port_range"`
	// Rechecking (alpha.127): the host's sandbox level is its last verified
	// answer while one check runs again; the screen reads again shortly.
	Rechecking bool `json:"rechecking,omitempty"`
}

// serviceAppSandboxLvl is the host level as the domain screen shows it.
type serviceAppSandboxLvl struct {
	Known bool     `json:"known"`
	Level string   `json:"level,omitempty"`
	Gaps  []string `json:"gaps"`
}

// runtimeGuidanceInput is what decides the states of an app.
type runtimeGuidanceInput struct {
	AgentUnreachable bool
	Suspended        bool
	Held             bool
	NotOffered       bool
	SandboxReduced   bool
	NoCertificate    bool
	DiskFull         bool
	Status           *transport.RuntimeAppStatusResponse
}

// runtimeAppStates lists the D-024 states of an app, most blocking first.
// Each has, on screen, the reason, who acts, the next action and how work
// resumes. A status the server did not give is agent_unreachable (unknown),
// never a guess.
func runtimeAppStates(in runtimeGuidanceInput) []string {
	out := []string{}
	add := func(cond bool, code string) {
		if cond {
			out = append(out, code)
		}
	}
	add(in.AgentUnreachable, "agent_unreachable")
	add(in.Suspended, "suspended")
	add(in.Held, "held")
	add(in.NotOffered, "not_offered")
	add(in.SandboxReduced, "sandbox_reduced")
	add(in.NoCertificate, "no_certificate")
	add(in.DiskFull, "disk_full")
	s := in.Status
	if s == nil || in.AgentUnreachable {
		return out
	}
	if s.Entry != nil && s.Entry.Code != siteruntime.EntryOK && s.Entry.Code != "" {
		code := s.Entry.Code
		switch {
		case code == siteruntime.EntryInvalid:
			code = siteruntime.EntryMissing
		case runtimePythonEntryState(code):
			// The Python kind's own states: the server's Python, app/src,
			// the environment, a refused requirements line.
		case !strings.HasPrefix(code, "entry_") && code != siteruntime.EntryFolderNotOwned && code != siteruntime.EntryFolderNotReal:
			// The account or the request could not be resolved: the
			// entry's state is unknown, not a verdict.
			code = siteruntime.EntryUnreadable
		}
		out = append(out, code)
	}
	add(s.StartLimitHit, "restart_loop")
	add(s.OOMKills > 0 || s.Result == "oom-kill", "oom")
	add(len(s.Exposed) > 0, "listens_elsewhere")
	add(s.Squatted, "port_taken")
	add(s.Unit.Active && !s.Serving && !s.Squatted && len(s.Exposed) == 0, "not_listening")
	add(s.Health.Checked && !s.Health.OK && s.Serving, "health_failing")
	return out
}

// runtimePythonEntryState: a Python check code that is a state of its own.
func runtimePythonEntryState(code string) bool {
	switch code {
	case siteruntime.PythonRuntimeMissing, siteruntime.PythonSrcMissing, siteruntime.PythonVenvMissing, siteruntime.RequirementsRefused:
		return true
	}
	return false
}

func (p *Panel) writeRuntimeAppsView(w http.ResponseWriter, r *http.Request, domainID int) {
	ctx := r.Context()
	site, found, err := p.serviceAppSiteFor(ctx, domainID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if !found {
		writeClientError(w, http.StatusNotFound, "domain not found")
		return
	}
	view := runtimeAppsDomainView{Domain: site.Domain, Suspended: site.Suspended, HasSite: site.SiteID > 0 && site.ProjectType != "dnsonly",
		ProjectType: site.ProjectType, Certificate: site.Certificate, Kinds: []runtimeKindView{},
		Sandbox: serviceAppSandboxLvl{Gaps: []string{}}, PortRange: [2]int{siteruntime.PortMin, siteruntime.PortMax}}
	quotaErr := p.checkSubscriptionQuota(ctx, site.SubscriptionID, quotaDisk)
	var exceeded *quotaExceededError
	view.DiskFull = errors.As(quotaErr, &exceeded)
	e, err := p.serviceAppRights(ctx, r, site.SubscriptionID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	offerings, err := p.loadRuntimeOfferings(ctx)
	if err != nil {
		writeServerError(w, err)
		return
	}
	// A read: the host's sandbox level is the last verified answer
	// (display_checks.go); every change asks the Agent itself.
	caps, capsView, capsErr := p.displayChecks.ready().sandbox.get(ctx, "sandbox", p.runtimeHostLevel)
	view.Rechecking = capsErr == nil && capsView.Rechecking
	if capsErr == nil {
		view.Sandbox = serviceAppSandboxLvl{Known: true, Level: caps.Level, Gaps: caps.Gaps}
		if view.Sandbox.Gaps == nil {
			view.Sandbox.Gaps = []string{}
		}
	}
	reducedByKind := map[siteruntime.Kind]bool{}
	offeredByKind := map[siteruntime.Kind]bool{}
	heldByKind := map[siteruntime.Kind]bool{}
	for _, kind := range siteruntime.Kinds {
		id, py := p.runtimeKindOffering(ctx, kind)
		if id == "" && kind != siteruntime.KindPython {
			continue
		}
		o, has := offerings[id]
		if id == "" {
			o, has = runtimeOffering{}, false
		}
		limits := runtimeLimitsFor(kind, o, has)
		sourceBuilds := has && runtimeOfferSourceBuilds(o)
		kv := runtimeKindView{Kind: string(kind), Implemented: siteruntime.Implemented(kind), OfferingID: id, Held: o.Held,
			Risk: siteruntime.RiskOfOffer(kind, sourceBuilds), Limits: limits, AcceptedLevel: "full", AcceptedGaps: []string{}}
		if kind == siteruntime.KindPython {
			kv.Python = pythonView(py, limits, sourceBuilds)
		}
		if has && o.AcceptedLevel != "" {
			kv.AcceptedLevel, kv.AcceptedGaps = o.AcceptedLevel, o.AcceptedGaps
		}
		if layer := e.permissionRefusal(siteruntime.Permission(kind)); layer == "" {
			kv.PermissionGranted = true
		} else {
			kv.PermissionRefused = layer
		}
		switch {
		case !has || !runtimeOfferOpen(o):
			kv.RefusedBy, kv.Grantor = rightsLayerAdministrator, "the administrator"
		default:
			if layer := e.offeringRefusal(id); layer != "" {
				kv.RefusedBy, kv.Grantor = layer, e.grantor(layer)
			} else {
				kv.Offered = true
			}
		}
		if kv.PermissionRefused != "" && kv.RefusedBy == "" {
			kv.RefusedBy, kv.Grantor = kv.PermissionRefused, e.grantor(kv.PermissionRefused)
		}
		offeredByKind[kind] = kv.Offered && kv.PermissionGranted
		heldByKind[kind] = o.Held
		reducedByKind[kind] = capsErr == nil && runtimeLevelRefusal(caps, o) != ""
		view.Kinds = append(view.Kinds, kv)
	}
	a, has, err := p.runtimeAppForDomain(ctx, domainID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if has {
		av := &runtimeAppView{ID: a.ID, Kind: string(a.Kind), Entry: a.Entry, Args: a.Args, HealthPath: a.HealthPath, Port: a.Port,
			State: a.State, Deployed: a.Deployed, URL: "https://" + a.Domain + "/", UploadDir: "app/bin",
			Unit: siteruntime.UnitName(a.ID), Env: []runtimeEnvView{}, CreatedAt: a.CreatedAt}
		if a.Kind == siteruntime.KindPython {
			settings, _ := siteruntime.PythonSettingsFrom(a.Entry, a.Args)
			av.Python, av.UploadDir, av.Args = &settings, "app/src", []string{}
		}
		if layout, err := siteruntime.LayoutFor(a.SubscriptionID, a.DomainID); err == nil {
			av.Folder = layout.Root
		}
		for key, value := range a.EnvPublic {
			av.Env = append(av.Env, runtimeEnvView{Key: key, Value: value})
		}
		for _, key := range a.EnvSecretKeys {
			av.Env = append(av.Env, runtimeEnvView{Key: key, Secret: true})
		}
		sort.Slice(av.Env, func(i, j int) bool { return av.Env[i].Key < av.Env[j].Key })
		o, oHas := offerings[a.OfferingID]
		req := transport.RuntimeAppStatusRequest{Target: a.target(), Spec: a.spec(o, oHas), Entry: true}
		// One health GET only when the person asked for a fresh read, never
		// from the screen's polling.
		if r.URL.Query().Get("health") == "1" {
			req.HealthPath = a.HealthPath
		}
		statusCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
		status, err := p.runtimeAppNativeOps().status(statusCtx, req)
		cancel()
		if err != nil || status.Error != "" {
			av.StatusUnknown = true
		} else {
			av.Status = &status
		}
		av.States = runtimeAppStates(runtimeGuidanceInput{
			AgentUnreachable: av.StatusUnknown, Suspended: site.Suspended, Held: heldByKind[a.Kind],
			NotOffered: !offeredByKind[a.Kind], SandboxReduced: reducedByKind[a.Kind], NoCertificate: !site.Certificate,
			DiskFull: view.DiskFull, Status: av.Status,
		})
		view.App = av
		if op, ok, err := p.latestRuntimeAppOperation(ctx, a.ID); err == nil && ok {
			view.Operation = &op
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(view)
}

type runtimeAppSettingsRequest struct {
	Kind       string   `json:"kind"`
	Entry      string   `json:"entry"`
	Args       []string `json:"args"`
	HealthPath string   `json:"health_path"`
	// The Python kind: Entry is "module:callable"; Server is wsgi or asgi.
	Server  string `json:"server"`
	Workers int    `json:"workers"`
}

// validatePythonSettings checks a Python app's settings against the
// offering's memory limit and turns them into the record's entry and
// argument tokens.
func validatePythonSettings(w http.ResponseWriter, limits siteruntime.Limits, req *runtimeAppSettingsRequest) bool {
	settings := siteruntime.PythonSettings{Server: strings.TrimSpace(req.Server), Callable: strings.TrimSpace(req.Entry), Workers: req.Workers}
	if settings.Workers == 0 {
		settings.Workers = 1
	}
	if err := settings.Validate(limits); err != nil {
		writeRuntimeAppRefusal(w, http.StatusBadRequest, "python_settings", "Nothing was changed: "+err.Error()+".",
			map[string]string{"max": strconv.Itoa(siteruntime.MaxWorkers(limits.MemoryMB))})
		return false
	}
	if req.HealthPath == "" {
		req.HealthPath = "/"
	}
	if !siteruntime.ValidHealthPath(req.HealthPath) {
		writeRuntimeAppRefusal(w, http.StatusBadRequest, "health_path_invalid",
			"Nothing was changed: the health path starts with / and holds only letters, digits and . _ ~ / -.", nil)
		return false
	}
	req.Entry, req.Args = settings.Callable, settings.Args()
	return true
}

// runtimeSettingsFor validates a create or settings request of a kind:
// the program's entry and arguments, or the Python app's settings. It
// returns the offering the kind is gated by on this server.
func (p *Panel) runtimeSettingsFor(w http.ResponseWriter, r *http.Request, kind siteruntime.Kind, layout siteruntime.Layout, req *runtimeAppSettingsRequest) (string, bool) {
	offeringID, _ := p.runtimeKindOffering(r.Context(), kind)
	if kind != siteruntime.KindPython {
		return offeringID, validateRuntimeSettings(w, layout, req)
	}
	offerings, err := p.loadRuntimeOfferings(r.Context())
	if err != nil {
		writeServerError(w, err)
		return "", false
	}
	o, has := offerings[offeringID]
	return offeringID, validatePythonSettings(w, runtimeLimitsFor(kind, o, has), req)
}

// validateRuntimeSettings checks the entry, arguments and health path.
func validateRuntimeSettings(w http.ResponseWriter, layout siteruntime.Layout, req *runtimeAppSettingsRequest) bool {
	req.Entry = strings.TrimPrefix(strings.TrimSpace(req.Entry), "bin/")
	if _, err := siteruntime.ValidEntry(layout, req.Entry); err != nil {
		writeRuntimeAppRefusal(w, http.StatusBadRequest, "entry_invalid", "Nothing was changed: "+err.Error()+".", nil)
		return false
	}
	if req.Args == nil {
		req.Args = []string{}
	}
	if err := siteruntime.ValidateArgs(req.Args); err != nil {
		writeRuntimeAppRefusal(w, http.StatusBadRequest, "args_invalid", "Nothing was changed: "+err.Error()+".", nil)
		return false
	}
	if req.HealthPath == "" {
		req.HealthPath = "/"
	}
	if !siteruntime.ValidHealthPath(req.HealthPath) {
		writeRuntimeAppRefusal(w, http.StatusBadRequest, "health_path_invalid",
			"Nothing was changed: the health path starts with / and holds only letters, digits and . _ ~ / -.", nil)
		return false
	}
	return true
}

func (p *Panel) handleRuntimeAppCreate(w http.ResponseWriter, r *http.Request, domainID int) {
	var req runtimeAppSettingsRequest
	if !decodeServiceAppBody(w, r, &req) {
		return
	}
	ctx := r.Context()
	kind := siteruntime.Kind(req.Kind)
	if req.Kind == "" {
		kind = siteruntime.KindProgram
	}
	if !siteruntime.Known(kind) {
		writeClientError(w, http.StatusBadRequest, "unknown kind")
		return
	}
	site, found, err := p.serviceAppSiteFor(ctx, domainID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if !found {
		writeClientError(w, http.StatusNotFound, "domain not found")
		return
	}
	if p.refuseRuntimeNewUse(w, r, kind, site) {
		return
	}
	layout, err := siteruntime.LayoutFor(site.SubscriptionID, domainID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	offeringID, valid := p.runtimeSettingsFor(w, r, kind, layout, &req)
	if !valid {
		return
	}
	if _, has, err := p.serviceAppInstallForDomain(ctx, domainID); err != nil {
		writeServerError(w, err)
		return
	} else if has || site.ProjectType == "node" || site.ProjectType == "service_app" {
		writeRuntimeAppRefusal(w, http.StatusConflict, "site_type",
			"Nothing was changed: this domain already runs a Node.js application or an AI app. Use a separate domain or subdomain for the program.", nil)
		return
	}
	if message, err := p.refuseProjectTypeWithSiteFeatures(ctx, domainID, "runtime_app"); err != nil {
		writeServerError(w, err)
		return
	} else if message != "" {
		writeRuntimeAppRefusal(w, http.StatusConflict, "site_features", message, nil)
		return
	}
	args, _ := json.Marshal(req.Args)
	tx, err := p.db.GetDB().BeginTx(ctx, nil)
	if err != nil {
		writeServerError(w, err)
		return
	}
	defer func() { _ = tx.Rollback() }()
	var existing int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM runtime_apps WHERE domain_id = ?`, domainID).Scan(&existing); err != nil {
		writeServerError(w, err)
		return
	}
	if existing > 0 {
		writeRuntimeAppRefusal(w, http.StatusConflict, "already_created",
			"Nothing was changed: this domain already has a program. Change its settings, or remove it first.", nil)
		return
	}
	port, err := allocateRuntimeAppPort(ctx, tx)
	if err != nil {
		writeRuntimeAppRefusal(w, http.StatusConflict, "port_taken", "Nothing was changed: "+err.Error()+". The administrator removes unused apps.", nil)
		return
	}
	previous := site.ProjectType
	if previous == "" {
		previous = "php"
	}
	res, err := tx.ExecContext(ctx, `
		INSERT INTO runtime_apps (subscription_id, domain_id, site_id, kind, offering_id, entry, args, health_path, port,
		       previous_project_type, previous_app_port, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		site.SubscriptionID, domainID, site.SiteID, string(kind), offeringID, req.Entry, string(args), req.HealthPath, port,
		previous, site.AppPort, nullablePositiveInt(callerID(currentCaller(r))))
	if err != nil {
		writeServerError(w, err)
		return
	}
	id, _ := res.LastInsertId()
	if err := tx.Commit(); err != nil {
		writeServerError(w, err)
		return
	}
	p.auditDetailed(r, "runtime.create", "domain", domainID, map[string]any{"app": id, "kind": string(kind), "entry": req.Entry, "port": port})
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "port": port})
}

// runtimeAppForAction loads the domain's app and refuses while an operation
// of it runs.
func (p *Panel) runtimeAppForAction(w http.ResponseWriter, r *http.Request, domainID int) (runtimeApp, bool) {
	a, has, err := p.runtimeAppForDomain(r.Context(), domainID)
	if err != nil {
		writeServerError(w, err)
		return a, false
	}
	if !has {
		writeRuntimeAppRefusal(w, http.StatusNotFound, "not_created", "This domain has no program yet.", nil)
		return a, false
	}
	if op, ok, err := p.latestRuntimeAppOperation(r.Context(), a.ID); err == nil && ok && op.State == "running" {
		writeRuntimeAppRefusal(w, http.StatusConflict, "busy",
			"Nothing was changed: the "+op.Kind+" of this program is still running. It continues by itself; wait for it to finish.", nil)
		return a, false
	}
	return a, true
}

func (p *Panel) handleRuntimeAppSettings(w http.ResponseWriter, r *http.Request, domainID int) {
	var req runtimeAppSettingsRequest
	if !decodeServiceAppBody(w, r, &req) {
		return
	}
	ctx := r.Context()
	a, ok := p.runtimeAppForAction(w, r, domainID)
	if !ok {
		return
	}
	site, _, err := p.serviceAppSiteFor(ctx, domainID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if p.refuseRuntimeNewUse(w, r, a.Kind, site) {
		return
	}
	layout, err := siteruntime.LayoutFor(a.SubscriptionID, a.DomainID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	offeringID, valid := p.runtimeSettingsFor(w, r, a.Kind, layout, &req)
	if !valid {
		return
	}
	args, _ := json.Marshal(req.Args)
	// A Python app follows the server's Python: after a system upgrade
	// (Debian 12 to 13) saving the settings moves it to the offering of
	// the new Python, which the gate above checked.
	if offeringID == "" {
		offeringID = a.OfferingID
	}
	if _, err := p.db.GetDB().ExecContext(ctx, `UPDATE runtime_apps SET entry = ?, args = ?, health_path = ?, offering_id = ?, updated_at = datetime('now') WHERE id = ?`,
		req.Entry, string(args), req.HealthPath, offeringID, a.ID); err != nil {
		writeServerError(w, err)
		return
	}
	p.auditDetailed(r, "runtime.settings", "domain", domainID, map[string]any{"app": a.ID, "entry": req.Entry, "args": len(req.Args), "health_path": req.HealthPath})
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"id": a.ID, "applies": "next_deploy"})
}

func writeRuntimeAppAccepted(w http.ResponseWriter, status int, op runtimeAppOperation) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"operation": op})
}

// runtimeLostAnswer answers a repeated operation id with the first answer
// (a lost answer): nothing starts twice. It returns true when it answered.
func (p *Panel) runtimeLostAnswer(w http.ResponseWriter, r *http.Request, domainID int, opID string) bool {
	if opID == "" {
		return false
	}
	op, err := p.runtimeAppOperationByID(r.Context(), opID)
	if err != nil {
		return false
	}
	if a, err := p.runtimeAppByID(r.Context(), op.AppID); err == nil && a.DomainID == domainID {
		writeRuntimeAppAccepted(w, http.StatusOK, op)
		return true
	}
	writeClientError(w, http.StatusConflict, "this operation id belongs to another request")
	return true
}

func (p *Panel) handleRuntimeAppDeploy(w http.ResponseWriter, r *http.Request, domainID int) {
	var req struct {
		OperationID string `json:"operation_id"`
	}
	if !decodeServiceAppBody(w, r, &req) {
		return
	}
	if req.OperationID != "" && !serviceAppOperationIDPattern.MatchString(req.OperationID) {
		writeClientError(w, http.StatusBadRequest, "invalid operation id")
		return
	}
	ctx := r.Context()
	if p.runtimeLostAnswer(w, r, domainID, req.OperationID) {
		return
	}
	a, ok := p.runtimeAppForAction(w, r, domainID)
	if !ok {
		return
	}
	site, _, err := p.serviceAppSiteFor(ctx, domainID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if p.refuseRuntimeNewUse(w, r, a.Kind, site) {
		return
	}
	if a.Kind == siteruntime.KindPython {
		if current, _ := p.runtimeKindOffering(ctx, a.Kind); current != a.OfferingID {
			writeRuntimeAppRefusal(w, http.StatusConflict, "python_moved",
				"Nothing was changed: this app was set up for "+strings.TrimPrefix(a.OfferingID, "component:")+", and this server's Python is now different. Open the app's settings and save them once to move it to the server's Python, then deploy.",
				map[string]string{"offering": a.OfferingID})
			return
		}
	}
	if !site.Certificate {
		writeRuntimeAppRefusal(w, http.StatusConflict, "no_certificate",
			"Nothing was changed: a program is served only over HTTPS, and "+site.Domain+" has no certificate yet. Get a free certificate on the domain's SSL page, then deploy.",
			map[string]string{"domain": site.Domain})
		return
	}
	opID := req.OperationID
	if opID == "" {
		if opID, err = newServiceOperationID(); err != nil {
			writeServerError(w, err)
			return
		}
	}
	db := p.db.GetDB()
	if _, err := db.ExecContext(ctx, `INSERT INTO runtime_app_operations (id, app_id, kind, step, actor_id) VALUES (?, ?, 'deploy', 'validate', ?)`,
		opID, a.ID, nullablePositiveInt(callerID(currentCaller(r)))); err != nil {
		writeRuntimeAppRefusal(w, http.StatusConflict, "busy", "Nothing was changed: another operation of this program is still running.", nil)
		return
	}
	_, _ = db.ExecContext(ctx, `UPDATE runtime_apps SET state = 'deploying', updated_at = datetime('now') WHERE id = ?`, a.ID)
	p.auditDetailed(r, "runtime.deploy", "domain", domainID, map[string]any{"app": a.ID, "kind": string(a.Kind), "entry": a.Entry, "operation": opID})
	op, _ := p.runtimeAppOperationByID(ctx, opID)
	p.startRuntimeAppOperation(opID)
	writeRuntimeAppAccepted(w, http.StatusAccepted, op)
}

func (p *Panel) handleRuntimeAppControl(w http.ResponseWriter, r *http.Request, domainID int) {
	var req struct {
		Action string `json:"action"`
	}
	if !decodeServiceAppBody(w, r, &req) {
		return
	}
	if req.Action != "start" && req.Action != "stop" && req.Action != "restart" {
		writeClientError(w, http.StatusBadRequest, "action must be start, stop or restart")
		return
	}
	ctx := r.Context()
	a, ok := p.runtimeAppForAction(w, r, domainID)
	if !ok {
		return
	}
	if !a.Deployed && req.Action != "stop" {
		writeRuntimeAppRefusal(w, http.StatusConflict, "not_deployed", "The program is not deployed yet; deploy it first.", nil)
		return
	}
	// Stop is never refused (D-042 P5). Start and restart are refused while
	// held or suspended, and on a host whose sandbox level the
	// administrator did not accept.
	if req.Action != "stop" {
		reason, message, vars, err := p.runtimeStartRefusal(ctx, a)
		if err != nil {
			writeServerError(w, err)
			return
		}
		if reason != "" {
			writeRuntimeAppRefusal(w, http.StatusConflict, reason, message, vars)
			return
		}
	}
	resp, err := p.runtimeAppNativeOps().control(ctx, transport.RuntimeAppControlRequest{Target: a.target(), Action: req.Action})
	if err != nil {
		writeRuntimeAppRefusal(w, http.StatusBadGateway, "agent_unreachable", "The server did not answer, so the result is unknown. Read the status again before you retry.", nil)
		return
	}
	if !resp.Done {
		writeRuntimeAppRefusal(w, http.StatusConflict, resp.Reason, resp.Error, nil)
		return
	}
	if !a.Deployed && req.Action == "stop" {
		// Stop is never refused, and an app that was never deployed has
		// nothing running (the Agent stopped whatever a failed deploy may
		// have left). Its state stays as it is; the answer says why
		// nothing changed and what starts it (D-024).
		p.auditDetailed(r, "runtime.stop", "domain", domainID, map[string]any{"app": a.ID, "not_deployed": true})
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"state": a.State, "reason": "not_deployed",
			"message": "Nothing to stop: the program is not deployed yet, so it does not run. Deploy it to start it."})
		return
	}
	state := runtimeAppRunning
	if req.Action == "stop" {
		state = runtimeAppStopped
	}
	_, _ = p.db.GetDB().ExecContext(ctx, `UPDATE runtime_apps SET state = ?, updated_at = datetime('now') WHERE id = ?`, state, a.ID)
	details := map[string]any{"app": a.ID}
	switch req.Action {
	case "start":
		p.auditDetailed(r, "runtime.start", "domain", domainID, details)
	case "stop":
		p.auditDetailed(r, "runtime.stop", "domain", domainID, details)
	default:
		p.auditDetailed(r, "runtime.restart", "domain", domainID, details)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"state": state})
}

// runtimeStartRefusal is why a program may not be started now (the gates of
// the Programs tab's start, also used by a backup restore that stopped the
// program): a security hold, a suspended subscription, or a sandbox level the
// administrator did not accept. reason is "" when it may start; err is a
// read that failed.
func (p *Panel) runtimeStartRefusal(ctx context.Context, a runtimeApp) (reason, message string, vars map[string]string, err error) {
	offerings, err := p.loadRuntimeOfferings(ctx)
	if err != nil {
		return "", "", nil, err
	}
	o, has := offerings[a.OfferingID]
	if has && o.Held {
		return "held", "Nothing was changed: the administrator has put " + runtimeKindName(a.Kind) + " on a security hold. It can be started again when the hold is lifted.", nil, nil
	}
	if suspended, err := p.subscriptionSuspended(ctx, a.SubscriptionID); err != nil {
		return "", "", nil, err
	} else if suspended {
		return "suspended", "Nothing was changed: this subscription is suspended.", nil, nil
	}
	if caps, err := p.runtimeHostLevel(ctx); err == nil {
		if gaps := runtimeLevelRefusal(caps, o); gaps != "" {
			return "sandbox_reduced", "Nothing was changed: this server does not enforce the full sandbox (" + gaps + ") and the administrator has not accepted these gaps.", map[string]string{"gaps": gaps}, nil
		}
	}
	return "", "", nil, nil
}

func (p *Panel) handleRuntimeAppEnv(w http.ResponseWriter, r *http.Request, domainID int) {
	var req struct {
		Entries []transport.RuntimeAppEnvEntry `json:"entries"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 512<<10)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeClientError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	ctx := r.Context()
	a, ok := p.runtimeAppForAction(w, r, domainID)
	if !ok {
		return
	}
	known := map[string]bool{}
	for _, key := range a.EnvSecretKeys {
		known[key] = true
	}
	entries := make([]siteruntime.EnvEntry, len(req.Entries))
	public, secretKeys := map[string]string{}, []string{}
	for i, e := range req.Entries {
		e.Key = strings.TrimSpace(e.Key)
		req.Entries[i].Key = e.Key
		entries[i] = siteruntime.EnvEntry{Key: e.Key, Value: e.Value, Secret: e.Secret}
		if e.Secret && e.Value == "" && !known[e.Key] {
			writeRuntimeAppRefusal(w, http.StatusBadRequest, "env_values", "Nothing was changed: the secret "+e.Key+" needs a value the first time.", nil)
			return
		}
		if e.Secret {
			secretKeys = append(secretKeys, e.Key)
		} else {
			public[e.Key] = e.Value
		}
	}
	if err := siteruntime.ValidateEnv(entries); err != nil {
		writeRuntimeAppRefusal(w, http.StatusBadRequest, "env_values", "Nothing was changed: "+err.Error()+".", nil)
		return
	}
	previous := append([]string{}, a.EnvSecretKeys...)
	for key := range a.EnvPublic {
		previous = append(previous, key)
	}
	resp, err := p.runtimeAppNativeOps().env(ctx, transport.RuntimeAppEnvRequest{Target: a.target(), Entries: req.Entries,
		Restart: a.Deployed, PreviousKeys: previous})
	if err != nil {
		writeRuntimeAppRefusal(w, http.StatusBadGateway, "agent_unreachable",
			"The server did not answer, so it is unknown whether the values were saved. Enter them again.", nil)
		return
	}
	if !resp.Done {
		writeRuntimeAppRefusal(w, http.StatusConflict, resp.Reason, resp.Error, nil)
		return
	}
	sort.Strings(secretKeys)
	publicJSON, _ := json.Marshal(public)
	secretJSON, _ := json.Marshal(secretKeys)
	if _, err := p.db.GetDB().ExecContext(ctx, `UPDATE runtime_apps SET env_public = ?, env_secret_keys = ?, updated_at = datetime('now') WHERE id = ?`,
		string(publicJSON), string(secretJSON), a.ID); err != nil {
		writeServerError(w, err)
		return
	}
	keys := make([]string, 0, len(req.Entries))
	for _, e := range req.Entries {
		keys = append(keys, e.Key)
	}
	sort.Strings(keys)
	// The audit names the keys, never a value.
	p.auditDetailed(r, "runtime.env", "domain", domainID, map[string]any{"app": a.ID, "keys": keys})
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"keys": keys})
}

func (p *Panel) handleRuntimeAppLogs(w http.ResponseWriter, r *http.Request, domainID int) {
	a, has, err := p.runtimeAppForDomain(r.Context(), domainID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if !has {
		writeRuntimeAppRefusal(w, http.StatusNotFound, "not_created", "This domain has no program yet.", nil)
		return
	}
	lines, _ := strconv.Atoi(r.URL.Query().Get("lines"))
	if lines <= 0 || lines > 500 {
		lines = 200
	}
	resp, err := p.runtimeAppNativeOps().logs(r.Context(), transport.RuntimeAppLogsRequest{Target: a.target(), Lines: lines, SecretKeys: a.EnvSecretKeys})
	if err != nil || resp.Error != "" {
		writeRuntimeAppRefusal(w, http.StatusBadGateway, "logs_unavailable", "The log could not be read from the server just now. Try again.", nil)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"lines": resp.Lines})
}

func (p *Panel) handleRuntimeAppRemove(w http.ResponseWriter, r *http.Request, domainID int) {
	var req struct {
		OperationID   string `json:"operation_id"`
		RemoveFiles   bool   `json:"remove_files"`
		ConfirmDomain string `json:"confirm_domain"`
	}
	if !decodeServiceAppBody(w, r, &req) {
		return
	}
	if req.OperationID != "" && !serviceAppOperationIDPattern.MatchString(req.OperationID) {
		writeClientError(w, http.StatusBadRequest, "invalid operation id")
		return
	}
	ctx := r.Context()
	if p.runtimeLostAnswer(w, r, domainID, req.OperationID) {
		return
	}
	a, ok := p.runtimeAppForAction(w, r, domainID)
	if !ok {
		return
	}
	if req.RemoveFiles && !strings.EqualFold(strings.TrimSpace(req.ConfirmDomain), a.Domain) {
		writeRuntimeAppRefusal(w, http.StatusBadRequest, "confirm_files",
			"To remove the uploaded files and data too, type the domain name "+a.Domain+". Without it, the app folder is kept.", map[string]string{"domain": a.Domain})
		return
	}
	opID := req.OperationID
	var err error
	if opID == "" {
		if opID, err = newServiceOperationID(); err != nil {
			writeServerError(w, err)
			return
		}
	}
	removeFiles := 0
	if req.RemoveFiles {
		removeFiles = 1
	}
	db := p.db.GetDB()
	if _, err := db.ExecContext(ctx, `INSERT INTO runtime_app_operations (id, app_id, kind, step, remove_files, actor_id) VALUES (?, ?, 'remove', 'unpublish', ?, ?)`,
		opID, a.ID, removeFiles, nullablePositiveInt(callerID(currentCaller(r)))); err != nil {
		writeRuntimeAppRefusal(w, http.StatusConflict, "busy", "Nothing was changed: another operation of this program is still running.", nil)
		return
	}
	_, _ = db.ExecContext(ctx, `UPDATE runtime_apps SET state = 'removing', updated_at = datetime('now') WHERE id = ?`, a.ID)
	p.auditDetailed(r, "runtime.remove", "domain", domainID, map[string]any{"app": a.ID, "remove_files": req.RemoveFiles, "operation": opID})
	op, _ := p.runtimeAppOperationByID(ctx, opID)
	p.startRuntimeAppOperation(opID)
	writeRuntimeAppAccepted(w, http.StatusAccepted, op)
}

// refuseHostingChangeWithRuntimeApp: a domain that serves a program keeps its
// type until the program is removed.
func (p *Panel) refuseHostingChangeWithRuntimeApp(w http.ResponseWriter, r *http.Request, domainID int) bool {
	a, has, err := p.runtimeAppForDomain(r.Context(), domainID)
	if err != nil {
		writeServerError(w, err)
		return true
	}
	if !has {
		return false
	}
	writeRuntimeAppRefusal(w, http.StatusConflict, "app_created",
		"Nothing was changed: "+runtimeAppLabel(a)+" uses this domain. Remove it on the Programs tab first, then change the site type.", nil)
	return true
}

// refuseDomainDeleteWithRuntimeApp: the same before a domain is deleted.
func (p *Panel) refuseDomainDeleteWithRuntimeApp(w http.ResponseWriter, r *http.Request, domainID int) bool {
	a, has, err := p.runtimeAppForDomain(r.Context(), domainID)
	if err != nil {
		writeServerError(w, err)
		return true
	}
	if !has {
		return false
	}
	writeRuntimeAppRefusal(w, http.StatusConflict, "app_created",
		"Nothing was deleted: "+runtimeAppLabel(a)+" runs on this domain. Remove it on the Programs tab first (choose whether to keep its files), then delete the domain.", nil)
	return true
}
