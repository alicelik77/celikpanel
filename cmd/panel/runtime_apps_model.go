package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/siteruntime"
	"github.com/alicelik/celikpanel/internal/sitesandbox"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Runtime apps (alpha.119, docs/RUNTIME-APPS.md): the Panel's record of a
// customer's own program run as a sandboxed site service. The rights gate is
// D-042's: the permission program_apps and the offering runtime:program, which
// stays closed until the administrator offers it after reading its risk
// summary (and, on a host that enforces less than the full sandbox, after
// accepting the named gaps); a reseller and a plan can only narrow. The server
// enforces every layer on create and deploy; stop, logs and removal are never
// refused (D-042 P5).
//
// Çalışma ortamı uygulamaları: müşterinin kendi programının Panel kaydı.
// Yönetici risk özetini okuyup sunmadıkça kapalıdır; sunucu her katmanı
// oluşturma ve yayımlamada denetler; durdurma, günlük ve kaldırma reddedilmez.

const (
	runtimeAppsAdminPath = "/api/v1/admin/runtime-apps"
	errCodeRuntimeApp    = "RUNTIME_APP"

	// App states.
	runtimeAppCreated   = "created"
	runtimeAppDeploying = "deploying"
	runtimeAppRunning   = "running"
	runtimeAppStopped   = "stopped"
	runtimeAppFailed    = "failed"
	runtimeAppRemoving  = "removing"
)

// rightProgramApps (alpha.119): running an uploaded program as the site's
// service; rightPythonApps: running a Python web app the same way. Each kind
// is also gated by its offering (runtime:program, component:python:<minor>).
const (
	rightProgramApps = "program_apps"
	rightPythonApps  = "python_apps"
)

// runtimeOffering is one row of the administrator's offered layer.
type runtimeOffering struct {
	OfferingID    string
	Kind          siteruntime.Kind
	Offered       bool
	RiskDigest    string
	Limits        siteruntime.Limits
	AcceptedLevel string
	AcceptedGaps  []string
	Held          bool
	HeldReason    string
	UpdatedAt     string
}

func splitGaps(text string) []string {
	out := []string{}
	for _, g := range strings.Split(text, ",") {
		if g = strings.TrimSpace(g); g != "" {
			out = append(out, g)
		}
	}
	return out
}

func (p *Panel) loadRuntimeOfferings(ctx context.Context) (map[string]runtimeOffering, error) {
	rows, err := p.db.GetDB().QueryContext(ctx, `
		SELECT offering_id, kind, offered, risk_digest, memory_mb, cpu_percent, tasks_max, accepted_level, accepted_gaps,
		       held, held_reason, updated_at
		FROM runtime_offerings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]runtimeOffering{}
	for rows.Next() {
		var o runtimeOffering
		var kind, gaps string
		var offered, held int
		if err := rows.Scan(&o.OfferingID, &kind, &offered, &o.RiskDigest, &o.Limits.MemoryMB, &o.Limits.CPUPercent,
			&o.Limits.Tasks, &o.AcceptedLevel, &gaps, &held, &o.HeldReason, &o.UpdatedAt); err != nil {
			return nil, err
		}
		o.Kind, o.Offered, o.Held, o.AcceptedGaps = siteruntime.Kind(kind), offered == 1, held == 1, splitGaps(gaps)
		out[o.OfferingID] = o
	}
	return out, rows.Err()
}

// runtimeOfferOpen is the administrator layer of one offering: offered, not
// held, its kind implemented and offered with the risk summary this release
// shows (a changed summary asks the administrator again).
func runtimeOfferOpen(o runtimeOffering) bool {
	return o.Offered && !o.Held && siteruntime.Implemented(o.Kind) &&
		(o.RiskDigest == siteruntime.RiskDigest(o.Kind) || runtimeOfferSourceBuilds(o))
}

// runtimeOfferSourceBuilds: the administrator offered the Python kind with
// source builds allowed. The stored risk digest records it (the summary the
// administrator agreed to names source builds), so 063 needs no column.
func runtimeOfferSourceBuilds(o runtimeOffering) bool {
	return o.Kind == siteruntime.KindPython && o.RiskDigest == siteruntime.RiskDigestFor(siteruntime.KindPython, true)
}

// loadOfferedRuntimes is the admin layer for the rights model.
func (p *Panel) loadOfferedRuntimes(ctx context.Context) (map[string]bool, error) {
	offerings, err := p.loadRuntimeOfferings(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for id, o := range offerings {
		if runtimeOfferOpen(o) {
			out[id] = true
		}
	}
	return out, nil
}

// runtimeCandidateOfferings are the runtime offerings a plan or a rights set
// can name.
func runtimeCandidateOfferings() []string {
	out := []string{}
	for _, k := range siteruntime.Kinds {
		if id := siteruntime.OfferingID(k); id != "" && siteruntime.Implemented(k) {
			out = append(out, id)
		}
	}
	return out
}

// runtimeLimitsFor: the administrator's limits (the defaults before a row).
func runtimeLimitsFor(kind siteruntime.Kind, o runtimeOffering, ok bool) siteruntime.Limits {
	if ok && o.Limits.MemoryMB > 0 {
		return o.Limits
	}
	return siteruntime.DefaultLimits(kind)
}

// runtimeApp is one row of runtime_apps.
type runtimeApp struct {
	ID                  int
	SubscriptionID      int
	DomainID            int
	SiteID              int
	Domain              string
	Kind                siteruntime.Kind
	OfferingID          string
	Entry               string
	Args                []string
	EnvPublic           map[string]string
	EnvSecretKeys       []string
	HealthPath          string
	Port                int
	State               string
	Deployed            bool
	PreviousProjectType string
	PreviousAppPort     sql.NullInt64
	CreatedAt           string
}

const runtimeAppColumns = `a.id, a.subscription_id, a.domain_id, a.site_id, d.name, a.kind, a.offering_id, a.entry, a.args,
	a.env_public, a.env_secret_keys, a.health_path, a.port, a.state, a.deployed, a.previous_project_type, a.previous_app_port, a.created_at`

func scanRuntimeApp(row interface{ Scan(...any) error }) (runtimeApp, error) {
	var a runtimeApp
	var kind, args, envPublic, secretKeys string
	var deployed int
	err := row.Scan(&a.ID, &a.SubscriptionID, &a.DomainID, &a.SiteID, &a.Domain, &kind, &a.OfferingID, &a.Entry, &args,
		&envPublic, &secretKeys, &a.HealthPath, &a.Port, &a.State, &deployed, &a.PreviousProjectType, &a.PreviousAppPort, &a.CreatedAt)
	if err != nil {
		return a, err
	}
	a.Kind, a.Deployed = siteruntime.Kind(kind), deployed == 1
	a.Args, a.EnvPublic, a.EnvSecretKeys = []string{}, map[string]string{}, []string{}
	_ = json.Unmarshal([]byte(args), &a.Args)
	_ = json.Unmarshal([]byte(envPublic), &a.EnvPublic)
	_ = json.Unmarshal([]byte(secretKeys), &a.EnvSecretKeys)
	return a, nil
}

func (p *Panel) runtimeAppForDomain(ctx context.Context, domainID int) (runtimeApp, bool, error) {
	a, err := scanRuntimeApp(p.db.GetDB().QueryRowContext(ctx, `
		SELECT `+runtimeAppColumns+` FROM runtime_apps a JOIN domains d ON d.id = a.domain_id WHERE a.domain_id = ?`, domainID))
	if errors.Is(err, sql.ErrNoRows) {
		return runtimeApp{}, false, nil
	}
	return a, err == nil, err
}

func (p *Panel) runtimeAppByID(ctx context.Context, id int) (runtimeApp, error) {
	return scanRuntimeApp(p.db.GetDB().QueryRowContext(ctx, `
		SELECT `+runtimeAppColumns+` FROM runtime_apps a JOIN domains d ON d.id = a.domain_id WHERE a.id = ?`, id))
}

func (a runtimeApp) target() transport.RuntimeAppTarget {
	return transport.RuntimeAppTarget{AppID: a.ID, Kind: string(a.Kind), SiteID: a.SiteID, SubscriptionID: a.SubscriptionID,
		DomainID: a.DomainID, Domain: a.Domain}
}

// spec is the unit's description for an offering's limits and level.
func (a runtimeApp) spec(o runtimeOffering, ok bool) transport.RuntimeAppSpec {
	limits := runtimeLimitsFor(a.Kind, o, ok)
	level := string(sitesandbox.LevelFull)
	var gaps []string
	if ok && o.AcceptedLevel == string(sitesandbox.LevelReduced) {
		level, gaps = string(sitesandbox.LevelReduced), o.AcceptedGaps
	}
	spec := transport.RuntimeAppSpec{Entry: a.Entry, Args: a.Args, Port: a.Port, Level: level, AcceptedGaps: gaps,
		Limits: transport.RuntimeAppLimits{MemoryMB: limits.MemoryMB, CPUPercent: limits.CPUPercent, TasksMax: limits.Tasks}}
	if a.Kind == siteruntime.KindPython {
		// The record keeps the callable in entry and the server and
		// workers as two argument tokens (siteruntime.PythonSettings).
		settings, _ := siteruntime.PythonSettingsFrom(a.Entry, a.Args)
		spec.Args = []string{}
		spec.Python = &transport.RuntimeAppPython{Server: settings.Server, Workers: settings.Workers,
			SourceBuilds: ok && runtimeOfferSourceBuilds(o)}
	}
	return spec
}

// envEntries is the full environment the Agent is sent: public values and
// the secret keys (their values stay on the server).
func (a runtimeApp) envEntries() []transport.RuntimeAppEnvEntry {
	out := []transport.RuntimeAppEnvEntry{}
	for key, value := range a.EnvPublic {
		out = append(out, transport.RuntimeAppEnvEntry{Key: key, Value: value})
	}
	for _, key := range a.EnvSecretKeys {
		out = append(out, transport.RuntimeAppEnvEntry{Key: key, Secret: true})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// runtimeAppOperation is one row of runtime_app_operations.
type runtimeAppOperation struct {
	ID          string `json:"id"`
	AppID       int    `json:"app_id"`
	Kind        string `json:"kind"`
	State       string `json:"state"`
	Step        string `json:"step"`
	Class       string `json:"class,omitempty"`
	Reason      string `json:"reason,omitempty"`
	Message     string `json:"message,omitempty"`
	RemoveFiles bool   `json:"remove_files,omitempty"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

const runtimeAppOperationColumns = `id, app_id, kind, state, step, class, reason, message, remove_files, created_at, updated_at`

func scanRuntimeAppOperation(row interface{ Scan(...any) error }) (runtimeAppOperation, error) {
	var op runtimeAppOperation
	var removeFiles int
	err := row.Scan(&op.ID, &op.AppID, &op.Kind, &op.State, &op.Step, &op.Class, &op.Reason, &op.Message,
		&removeFiles, &op.CreatedAt, &op.UpdatedAt)
	op.RemoveFiles = removeFiles == 1
	return op, err
}

func (p *Panel) runtimeAppOperationByID(ctx context.Context, id string) (runtimeAppOperation, error) {
	return scanRuntimeAppOperation(p.db.GetDB().QueryRowContext(ctx,
		`SELECT `+runtimeAppOperationColumns+` FROM runtime_app_operations WHERE id = ?`, id))
}

func (p *Panel) latestRuntimeAppOperation(ctx context.Context, appID int) (runtimeAppOperation, bool, error) {
	op, err := scanRuntimeAppOperation(p.db.GetDB().QueryRowContext(ctx, `
		SELECT `+runtimeAppOperationColumns+` FROM runtime_app_operations WHERE app_id = ?
		ORDER BY (state = 'running') DESC, created_at DESC, rowid DESC LIMIT 1`, appID))
	if errors.Is(err, sql.ErrNoRows) {
		return runtimeAppOperation{}, false, nil
	}
	return op, err == nil, err
}

func (p *Panel) setRuntimeAppStep(ctx context.Context, opID, step string) error {
	_, err := p.db.GetDB().ExecContext(ctx, `
		UPDATE runtime_app_operations SET step = ?, updated_at = datetime('now') WHERE id = ? AND state = 'running'`, step, opID)
	return err
}

// failRuntimeAppOperation ends an operation with its class, reason and the
// server's (already redacted) words; the app row says failed so the screen
// shows what to do next.
func (p *Panel) failRuntimeAppOperation(ctx context.Context, op runtimeAppOperation, class, reason, message string) {
	if class == "" {
		class = transport.RuntimeAppUnknown
	}
	if len(message) > 1900 {
		message = message[:1900]
	}
	db := p.db.GetDB()
	_, _ = db.ExecContext(ctx, `
		UPDATE runtime_app_operations SET state = 'failed', class = ?, reason = ?, message = ?, updated_at = datetime('now')
		WHERE id = ? AND state = 'running'`, class, reason, message, op.ID)
	_, _ = db.ExecContext(ctx, `UPDATE runtime_apps SET state = 'failed', updated_at = datetime('now') WHERE id = ?`, op.AppID)
}

func (p *Panel) succeedRuntimeAppOperation(ctx context.Context, opID, message string) error {
	_, err := p.db.GetDB().ExecContext(ctx, `
		UPDATE runtime_app_operations SET state = 'succeeded', step = 'done', class = '', reason = '', message = ?, updated_at = datetime('now')
		WHERE id = ? AND state = 'running'`, message, opID)
	return err
}

// allocateRuntimeAppPort takes the lowest free port of the runtime range that
// no other runtime app, service app or Node.js site holds.
func allocateRuntimeAppPort(ctx context.Context, q sqlQueryer) (int, error) {
	taken := map[int]bool{}
	rows, err := q.QueryContext(ctx, `
		SELECT port FROM runtime_apps
		UNION SELECT port FROM service_app_installs UNION SELECT aux_port FROM service_app_installs WHERE aux_port <> 0
		UNION SELECT app_port FROM sites WHERE app_port IS NOT NULL`)
	if err != nil {
		return 0, err
	}
	for rows.Next() {
		var port sql.NullInt64
		if err := rows.Scan(&port); err != nil {
			rows.Close()
			return 0, err
		}
		if port.Valid {
			taken[int(port.Int64)] = true
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	return siteruntime.AllocatePort(taken)
}

// runtimeAppNative is what the Panel asks of the server. Production uses the
// Agent; tests replace it with runtimeAppNativeOverride.
type runtimeAppNative interface {
	check(ctx context.Context, req transport.RuntimeAppCheckRequest) (transport.RuntimeAppCheckResponse, error)
	render(ctx context.Context, req transport.RuntimeAppRenderRequest) (transport.RuntimeAppRenderResponse, error)
	control(ctx context.Context, req transport.RuntimeAppControlRequest) (transport.RuntimeAppStepResponse, error)
	status(ctx context.Context, req transport.RuntimeAppStatusRequest) (transport.RuntimeAppStatusResponse, error)
	env(ctx context.Context, req transport.RuntimeAppEnvRequest) (transport.RuntimeAppStepResponse, error)
	logs(ctx context.Context, req transport.RuntimeAppLogsRequest) (transport.RuntimeAppLogsResponse, error)
	remove(ctx context.Context, req transport.RuntimeAppRemoveRequest) (transport.RuntimeAppRemoveResponse, error)
	unitState(ctx context.Context, appID int) (transport.ServiceAppUnitStateResponse, error)
	setSuspension(ctx context.Context, req transport.SetRuntimeAppSuspensionRequest) (transport.SetAppUnitSuspensionResponse, error)
	renderSite(ctx context.Context, domainID int) error
	sandboxCaps(ctx context.Context) (transport.SandboxCapsResponse, error)
	// The Python kind: the server's distribution Python, and the build of
	// an app's environment.
	python(ctx context.Context) (transport.RuntimePythonResponse, error)
	build(ctx context.Context, req transport.RuntimeAppBuildRequest) (transport.RuntimeAppBuildResponse, error)
	buildAdvance(ctx context.Context, req transport.RuntimeAppBuildAdvanceRequest) (transport.RuntimeAppBuildResponse, error)
}

var runtimeAppNativeOverride runtimeAppNative

// runtimeAppPollInterval is how often a running operation reads the server.
var runtimeAppPollInterval = 3 * time.Second

func (p *Panel) runtimeAppNativeOps() runtimeAppNative {
	if runtimeAppNativeOverride != nil {
		return runtimeAppNativeOverride
	}
	return panelRuntimeAppNative{p: p}
}

type panelRuntimeAppNative struct{ p *Panel }

func (n panelRuntimeAppNative) call(ctx context.Context, method string, req, resp any) error {
	return n.p.callAgentContext(ctx, method, req, resp)
}

func (n panelRuntimeAppNative) check(ctx context.Context, req transport.RuntimeAppCheckRequest) (transport.RuntimeAppCheckResponse, error) {
	var resp transport.RuntimeAppCheckResponse
	return resp, n.call(ctx, "Agent.RuntimeAppCheckV1", &req, &resp)
}

func (n panelRuntimeAppNative) render(ctx context.Context, req transport.RuntimeAppRenderRequest) (transport.RuntimeAppRenderResponse, error) {
	req.ExpectedBuildCommit = strings.TrimSpace(buildCommit)
	var resp transport.RuntimeAppRenderResponse
	return resp, n.call(ctx, "Agent.RuntimeAppRenderV1", &req, &resp)
}

func (n panelRuntimeAppNative) control(ctx context.Context, req transport.RuntimeAppControlRequest) (transport.RuntimeAppStepResponse, error) {
	req.ExpectedBuildCommit = strings.TrimSpace(buildCommit)
	var resp transport.RuntimeAppStepResponse
	return resp, n.call(ctx, "Agent.RuntimeAppControlV1", &req, &resp)
}

func (n panelRuntimeAppNative) status(ctx context.Context, req transport.RuntimeAppStatusRequest) (transport.RuntimeAppStatusResponse, error) {
	var resp transport.RuntimeAppStatusResponse
	return resp, n.call(ctx, "Agent.RuntimeAppStatusV1", &req, &resp)
}

func (n panelRuntimeAppNative) env(ctx context.Context, req transport.RuntimeAppEnvRequest) (transport.RuntimeAppStepResponse, error) {
	req.ExpectedBuildCommit = strings.TrimSpace(buildCommit)
	var resp transport.RuntimeAppStepResponse
	return resp, n.call(ctx, "Agent.RuntimeAppEnvV1", &req, &resp)
}

func (n panelRuntimeAppNative) logs(ctx context.Context, req transport.RuntimeAppLogsRequest) (transport.RuntimeAppLogsResponse, error) {
	var resp transport.RuntimeAppLogsResponse
	return resp, n.call(ctx, "Agent.RuntimeAppLogsV1", &req, &resp)
}

func (n panelRuntimeAppNative) remove(ctx context.Context, req transport.RuntimeAppRemoveRequest) (transport.RuntimeAppRemoveResponse, error) {
	req.ExpectedBuildCommit = strings.TrimSpace(buildCommit)
	var resp transport.RuntimeAppRemoveResponse
	return resp, n.call(ctx, "Agent.RuntimeAppRemoveV1", &req, &resp)
}

func (n panelRuntimeAppNative) unitState(ctx context.Context, appID int) (transport.ServiceAppUnitStateResponse, error) {
	var resp transport.ServiceAppUnitStateResponse
	return resp, n.call(ctx, "Agent.RuntimeAppUnitStateV1", &transport.RuntimeAppUnitStateRequest{AppID: appID}, &resp)
}

func (n panelRuntimeAppNative) setSuspension(ctx context.Context, req transport.SetRuntimeAppSuspensionRequest) (transport.SetAppUnitSuspensionResponse, error) {
	req.ExpectedBuildCommit = strings.TrimSpace(buildCommit)
	var resp transport.SetAppUnitSuspensionResponse
	return resp, n.call(ctx, "Agent.SetRuntimeAppSuspensionV1", &req, &resp)
}

func (n panelRuntimeAppNative) renderSite(ctx context.Context, domainID int) error {
	return n.p.applyVhostForDomain(ctx, domainID)
}

func (n panelRuntimeAppNative) sandboxCaps(ctx context.Context) (transport.SandboxCapsResponse, error) {
	var resp transport.SandboxCapsResponse
	return resp, n.call(ctx, "Agent.SandboxCapsV1", &transport.Empty{}, &resp)
}

func (n panelRuntimeAppNative) python(ctx context.Context) (transport.RuntimePythonResponse, error) {
	var resp transport.RuntimePythonResponse
	return resp, n.call(ctx, "Agent.RuntimePythonV1", &transport.Empty{}, &resp)
}

func (n panelRuntimeAppNative) build(ctx context.Context, req transport.RuntimeAppBuildRequest) (transport.RuntimeAppBuildResponse, error) {
	req.ExpectedBuildCommit = strings.TrimSpace(buildCommit)
	var resp transport.RuntimeAppBuildResponse
	return resp, n.call(ctx, "Agent.RuntimeAppBuildV1", &req, &resp)
}

func (n panelRuntimeAppNative) buildAdvance(ctx context.Context, req transport.RuntimeAppBuildAdvanceRequest) (transport.RuntimeAppBuildResponse, error) {
	req.ExpectedBuildCommit = strings.TrimSpace(buildCommit)
	var resp transport.RuntimeAppBuildResponse
	return resp, n.call(ctx, "Agent.RuntimeAppBuildAdvanceV1", &req, &resp)
}

func runtimeAppLabel(a runtimeApp) string {
	if a.Kind == siteruntime.KindPython {
		return "the Python app on " + a.Domain
	}
	return "the uploaded program on " + a.Domain
}

// runtimeKindName is a kind in a sentence ("uploaded programs").
func runtimeKindName(k siteruntime.Kind) string {
	if k == siteruntime.KindPython {
		return "Python apps"
	}
	return "uploaded programs"
}

// runtimeKindOffering is the offering a kind is gated by on this server:
// runtime:program, or component:python:<minor> for the distribution Python
// the Agent reports. py is the Agent's answer for the Python kind (nil for a
// program); id is "" when the server's Python is unknown.
func (p *Panel) runtimeKindOffering(ctx context.Context, kind siteruntime.Kind) (string, *transport.RuntimePythonResponse) {
	if kind != siteruntime.KindPython {
		return siteruntime.OfferingID(kind), nil
	}
	callCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	py, err := p.runtimeAppNativeOps().python(callCtx)
	if err != nil {
		return "", &transport.RuntimePythonResponse{Reason: siteruntime.PythonRuntimeMissing, Error: "the server did not say which Python it has: " + err.Error()}
	}
	if py.Minor == "" || !siteruntime.ValidPythonMinor(py.Minor) {
		return "", &py
	}
	return siteruntime.PythonOffering(py.Minor), &py
}
