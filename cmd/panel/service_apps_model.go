package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/serviceapps"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Service apps (alpha.118, docs/AI-APPS.md): the Panel's record of AI and
// automation applications that run as a site's long-running service. The
// rights gate is D-042's: the permission "service_apps" and the offering
// app:<id>, which stays closed until the administrator offers it after reading
// its risk summary; a reseller and a plan can only narrow. The server enforces
// all of it; the screens only show it.
//
// Hizmet uygulamaları: sitenin uzun süre çalışan hizmeti olarak çalışan yapay
// zekâ ve otomasyon uygulamalarının Panel kaydı. Yönetici risk özetini okuyup
// sunmadıkça uygulama kapalıdır.

const (
	serviceAppsAdminPath = "/api/v1/admin/service-apps"

	errCodeServiceApp = "SERVICE_APP"
	// Install states.
	serviceAppInstalling = "installing"
	serviceAppRunning    = "running"
	serviceAppStopped    = "stopped"
	serviceAppFailed     = "failed"
	serviceAppRemoving   = "removing"
)

// serviceAppOfferingApp: the catalog app an offering id names.
func serviceAppOfferingApp(offeringID string) (string, bool) {
	appID, ok := strings.CutPrefix(offeringID, "app:")
	if !ok || !serviceapps.Known(appID) {
		return "", false
	}
	return appID, true
}

// serviceAppCandidateOfferings are the service apps a plan or a rights set
// can name (the installable catalog entries).
func serviceAppCandidateOfferings() []string {
	out := []string{}
	for _, app := range serviceapps.Catalog() {
		if app.Installable() {
			out = append(out, serviceapps.OfferingID(app.ID))
		}
	}
	return out
}

// serviceAppOffering is one row of the administrator's offered layer.
type serviceAppOffering struct {
	AppID      string
	Offered    bool
	RiskDigest string
	Limits     transport.ServiceAppLimits
	Held       bool
	HeldReason string
	UpdatedAt  string
}

func (p *Panel) loadServiceAppOfferings(ctx context.Context) (map[string]serviceAppOffering, error) {
	rows, err := p.db.GetDB().QueryContext(ctx, `
		SELECT app_id, offered, risk_digest, memory_mb, cpu_percent, tasks_max, held, held_reason, updated_at
		FROM service_app_offerings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]serviceAppOffering{}
	for rows.Next() {
		var o serviceAppOffering
		var offered, held int
		if err := rows.Scan(&o.AppID, &offered, &o.RiskDigest, &o.Limits.MemoryMB, &o.Limits.CPUPercent,
			&o.Limits.TasksMax, &held, &o.HeldReason, &o.UpdatedAt); err != nil {
			return nil, err
		}
		o.Offered, o.Held = offered == 1, held == 1
		out[o.AppID] = o
	}
	return out, rows.Err()
}

// loadOfferedServiceApps is the admin layer: offered, not held, still
// installable in this release and offered with the risk summary this release
// shows (a changed summary asks the administrator again).
func (p *Panel) loadOfferedServiceApps(ctx context.Context) (map[string]bool, error) {
	offerings, err := p.loadServiceAppOfferings(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for id, o := range offerings {
		app, ok := serviceapps.Lookup(id)
		if ok && app.Installable() && o.Offered && !o.Held && o.RiskDigest == app.RiskDigest() {
			out[id] = true
		}
	}
	return out, nil
}

// serviceAppInstall is one row of service_app_installs.
type serviceAppInstall struct {
	ID                  int
	SubscriptionID      int
	DomainID            int
	SiteID              int
	Domain              string
	AppID               string
	Version             string
	TargetVersion       string
	Port                int
	AuxPort             int
	State               string
	PreviousProjectType string
	PreviousAppPort     sql.NullInt64
	CreatedAt           string
}

const serviceAppInstallColumns = `i.id, i.subscription_id, i.domain_id, i.site_id, d.name, i.app_id, i.version,
	i.target_version, i.port, i.aux_port, i.state, i.previous_project_type, i.previous_app_port, i.created_at`

func scanServiceAppInstall(row interface{ Scan(...any) error }) (serviceAppInstall, error) {
	var in serviceAppInstall
	err := row.Scan(&in.ID, &in.SubscriptionID, &in.DomainID, &in.SiteID, &in.Domain, &in.AppID, &in.Version,
		&in.TargetVersion, &in.Port, &in.AuxPort, &in.State, &in.PreviousProjectType, &in.PreviousAppPort, &in.CreatedAt)
	return in, err
}

func (p *Panel) serviceAppInstallForDomain(ctx context.Context, domainID int) (serviceAppInstall, bool, error) {
	in, err := scanServiceAppInstall(p.db.GetDB().QueryRowContext(ctx, `
		SELECT `+serviceAppInstallColumns+` FROM service_app_installs i JOIN domains d ON d.id = i.domain_id
		WHERE i.domain_id = ?`, domainID))
	if errors.Is(err, sql.ErrNoRows) {
		return serviceAppInstall{}, false, nil
	}
	return in, err == nil, err
}

func (p *Panel) serviceAppInstallByID(ctx context.Context, id int) (serviceAppInstall, error) {
	return scanServiceAppInstall(p.db.GetDB().QueryRowContext(ctx, `
		SELECT `+serviceAppInstallColumns+` FROM service_app_installs i JOIN domains d ON d.id = i.domain_id
		WHERE i.id = ?`, id))
}

func (in serviceAppInstall) target() transport.ServiceAppTarget {
	return transport.ServiceAppTarget{InstallID: in.ID, SiteID: in.SiteID, SubscriptionID: in.SubscriptionID,
		DomainID: in.DomainID, Domain: in.Domain, AppID: in.AppID}
}

// serviceAppOperation is one row of service_app_operations.
type serviceAppOperation struct {
	ID         string `json:"id"`
	InstallID  int    `json:"install_id"`
	Kind       string `json:"kind"`
	State      string `json:"state"`
	Step       string `json:"step"`
	Class      string `json:"class,omitempty"`
	Reason     string `json:"reason,omitempty"`
	Message    string `json:"message,omitempty"`
	RemoveData bool   `json:"remove_data,omitempty"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

func scanServiceAppOperation(row interface{ Scan(...any) error }) (serviceAppOperation, error) {
	var op serviceAppOperation
	var removeData int
	err := row.Scan(&op.ID, &op.InstallID, &op.Kind, &op.State, &op.Step, &op.Class, &op.Reason, &op.Message,
		&removeData, &op.CreatedAt, &op.UpdatedAt)
	op.RemoveData = removeData == 1
	return op, err
}

const serviceAppOperationColumns = `id, install_id, kind, state, step, class, reason, message, remove_data, created_at, updated_at`

func (p *Panel) serviceAppOperationByID(ctx context.Context, id string) (serviceAppOperation, error) {
	return scanServiceAppOperation(p.db.GetDB().QueryRowContext(ctx,
		`SELECT `+serviceAppOperationColumns+` FROM service_app_operations WHERE id = ?`, id))
}

// latestServiceAppOperation: the newest operation of an install (running
// first), or found false.
func (p *Panel) latestServiceAppOperation(ctx context.Context, installID int) (serviceAppOperation, bool, error) {
	op, err := scanServiceAppOperation(p.db.GetDB().QueryRowContext(ctx, `
		SELECT `+serviceAppOperationColumns+` FROM service_app_operations WHERE install_id = ?
		ORDER BY (state = 'running') DESC, created_at DESC, rowid DESC LIMIT 1`, installID))
	if errors.Is(err, sql.ErrNoRows) {
		return serviceAppOperation{}, false, nil
	}
	return op, err == nil, err
}

func (p *Panel) setServiceAppStep(ctx context.Context, opID, step string) error {
	_, err := p.db.GetDB().ExecContext(ctx, `
		UPDATE service_app_operations SET step = ?, updated_at = datetime('now')
		WHERE id = ? AND state = 'running'`, step, opID)
	return err
}

// failServiceAppOperation ends an operation with its class, reason code and
// the server's own (already redacted) words. The install row says failed for
// an install or update, so the screen shows what to do next.
func (p *Panel) failServiceAppOperation(ctx context.Context, op serviceAppOperation, class, reason, message string) {
	if class == "" {
		class = transport.ServiceAppUnknown
	}
	if len(message) > 1900 {
		message = message[:1900]
	}
	db := p.db.GetDB()
	_, _ = db.ExecContext(ctx, `
		UPDATE service_app_operations SET state = 'failed', class = ?, reason = ?, message = ?, updated_at = datetime('now')
		WHERE id = ? AND state = 'running'`, class, reason, message, op.ID)
	switch op.Kind {
	case "install":
		_, _ = db.ExecContext(ctx, `UPDATE service_app_installs SET state = 'failed', updated_at = datetime('now') WHERE id = ? AND version = ''`, op.InstallID)
	case "uninstall":
		// The removal stopped part way: the install stays listed so it can
		// be retried; what runs is decided by the unit's own state.
		_, _ = db.ExecContext(ctx, `UPDATE service_app_installs SET state = 'failed', updated_at = datetime('now') WHERE id = ?`, op.InstallID)
	}
}

func (p *Panel) succeedServiceAppOperation(ctx context.Context, opID, message string) error {
	_, err := p.db.GetDB().ExecContext(ctx, `
		UPDATE service_app_operations SET state = 'succeeded', step = 'done', class = '', reason = '', message = ?, updated_at = datetime('now')
		WHERE id = ? AND state = 'running'`, message, opID)
	return err
}

// allocateServiceAppPorts takes the lowest free even loopback port of the
// range (and the next one for an app with a second port), never one that a
// Node.js site or another install holds. The Agent proves at start that the
// port is not taken by another process.
func allocateServiceAppPorts(ctx context.Context, q sqlQueryer) (int, int, error) {
	taken := map[int]bool{}
	rows, err := q.QueryContext(ctx, `
		SELECT port FROM service_app_installs UNION SELECT aux_port FROM service_app_installs WHERE aux_port <> 0
		UNION SELECT app_port FROM sites WHERE app_port IS NOT NULL`)
	if err != nil {
		return 0, 0, err
	}
	for rows.Next() {
		var port sql.NullInt64
		if err := rows.Scan(&port); err != nil {
			rows.Close()
			return 0, 0, err
		}
		if port.Valid {
			taken[int(port.Int64)] = true
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, 0, err
	}
	for port := serviceapps.PortMin; port <= serviceapps.PortMax; port += 2 {
		if !taken[port] && !taken[port+1] {
			return port, port + 1, nil
		}
	}
	return 0, 0, errors.New("no free loopback port is left for service apps")
}

// serviceAppNative is what the Panel asks of the server. Production uses the
// Agent; tests replace it with serviceAppNativeOverride.
type serviceAppNative interface {
	probe(ctx context.Context) (transport.ServiceAppProbeResponse, error)
	prepare(ctx context.Context, req transport.ServiceAppPrepareRequest) (transport.ServiceAppStepResponse, error)
	status(ctx context.Context, req transport.ServiceAppStatusRequest) (transport.ServiceAppStatusResponse, error)
	activate(ctx context.Context, req transport.ServiceAppActivateRequest) (transport.ServiceAppStepResponse, error)
	finalize(ctx context.Context, req transport.ServiceAppActivateRequest) (transport.ServiceAppStepResponse, error)
	control(ctx context.Context, req transport.ServiceAppControlRequest) (transport.ServiceAppStepResponse, error)
	logs(ctx context.Context, req transport.ServiceAppLogsRequest) (transport.ServiceAppLogsResponse, error)
	secrets(ctx context.Context, req transport.ServiceAppSecretsRequest) (transport.ServiceAppStepResponse, error)
	remove(ctx context.Context, req transport.ServiceAppRemoveRequest) (transport.ServiceAppRemoveResponse, error)
	unitState(ctx context.Context, installID int) (transport.ServiceAppUnitStateResponse, error)
	setSuspension(ctx context.Context, req transport.SetServiceAppSuspensionRequest) (transport.SetAppUnitSuspensionResponse, error)
	renderSite(ctx context.Context, domainID int) error
	// sandboxCaps reads the site sandbox (alpha.119); it changes nothing.
	sandboxCaps(ctx context.Context) (transport.SandboxCapsResponse, error)
}

var serviceAppNativeOverride serviceAppNative

// serviceAppPollInterval is how often a running operation reads the server.
var serviceAppPollInterval = 10 * time.Second

func (p *Panel) serviceAppNativeOps() serviceAppNative {
	if serviceAppNativeOverride != nil {
		return serviceAppNativeOverride
	}
	return panelServiceAppNative{p: p}
}

type panelServiceAppNative struct{ p *Panel }

func (n panelServiceAppNative) call(ctx context.Context, method string, req, resp any) error {
	return n.p.callAgentContext(ctx, method, req, resp)
}

func (n panelServiceAppNative) probe(ctx context.Context) (transport.ServiceAppProbeResponse, error) {
	var resp transport.ServiceAppProbeResponse
	return resp, n.call(ctx, "Agent.ServiceAppProbeV1", &transport.Empty{}, &resp)
}

func (n panelServiceAppNative) prepare(ctx context.Context, req transport.ServiceAppPrepareRequest) (transport.ServiceAppStepResponse, error) {
	req.ExpectedBuildCommit = strings.TrimSpace(buildCommit)
	var resp transport.ServiceAppStepResponse
	return resp, n.call(ctx, "Agent.ServiceAppPrepareV1", &req, &resp)
}

func (n panelServiceAppNative) status(ctx context.Context, req transport.ServiceAppStatusRequest) (transport.ServiceAppStatusResponse, error) {
	var resp transport.ServiceAppStatusResponse
	return resp, n.call(ctx, "Agent.ServiceAppStatusV1", &req, &resp)
}

func (n panelServiceAppNative) activate(ctx context.Context, req transport.ServiceAppActivateRequest) (transport.ServiceAppStepResponse, error) {
	req.ExpectedBuildCommit = strings.TrimSpace(buildCommit)
	var resp transport.ServiceAppStepResponse
	return resp, n.call(ctx, "Agent.ServiceAppActivateV1", &req, &resp)
}

func (n panelServiceAppNative) finalize(ctx context.Context, req transport.ServiceAppActivateRequest) (transport.ServiceAppStepResponse, error) {
	req.ExpectedBuildCommit = strings.TrimSpace(buildCommit)
	var resp transport.ServiceAppStepResponse
	return resp, n.call(ctx, "Agent.ServiceAppFinalizeV1", &req, &resp)
}

func (n panelServiceAppNative) control(ctx context.Context, req transport.ServiceAppControlRequest) (transport.ServiceAppStepResponse, error) {
	req.ExpectedBuildCommit = strings.TrimSpace(buildCommit)
	var resp transport.ServiceAppStepResponse
	return resp, n.call(ctx, "Agent.ServiceAppControlV1", &req, &resp)
}

func (n panelServiceAppNative) logs(ctx context.Context, req transport.ServiceAppLogsRequest) (transport.ServiceAppLogsResponse, error) {
	var resp transport.ServiceAppLogsResponse
	return resp, n.call(ctx, "Agent.ServiceAppLogsV1", &req, &resp)
}

func (n panelServiceAppNative) secrets(ctx context.Context, req transport.ServiceAppSecretsRequest) (transport.ServiceAppStepResponse, error) {
	req.ExpectedBuildCommit = strings.TrimSpace(buildCommit)
	var resp transport.ServiceAppStepResponse
	return resp, n.call(ctx, "Agent.ServiceAppSecretsV1", &req, &resp)
}

func (n panelServiceAppNative) remove(ctx context.Context, req transport.ServiceAppRemoveRequest) (transport.ServiceAppRemoveResponse, error) {
	req.ExpectedBuildCommit = strings.TrimSpace(buildCommit)
	var resp transport.ServiceAppRemoveResponse
	return resp, n.call(ctx, "Agent.ServiceAppRemoveV1", &req, &resp)
}

func (n panelServiceAppNative) unitState(ctx context.Context, installID int) (transport.ServiceAppUnitStateResponse, error) {
	var resp transport.ServiceAppUnitStateResponse
	return resp, n.call(ctx, "Agent.ServiceAppUnitStateV1", &transport.ServiceAppUnitStateRequest{InstallID: installID}, &resp)
}

func (n panelServiceAppNative) setSuspension(ctx context.Context, req transport.SetServiceAppSuspensionRequest) (transport.SetAppUnitSuspensionResponse, error) {
	req.ExpectedBuildCommit = strings.TrimSpace(buildCommit)
	var resp transport.SetAppUnitSuspensionResponse
	return resp, n.call(ctx, "Agent.SetServiceAppSuspensionV1", &req, &resp)
}

func (n panelServiceAppNative) sandboxCaps(ctx context.Context) (transport.SandboxCapsResponse, error) {
	var resp transport.SandboxCapsResponse
	return resp, n.call(ctx, "Agent.SandboxCapsV1", &transport.Empty{}, &resp)
}

func (n panelServiceAppNative) renderSite(ctx context.Context, domainID int) error {
	return n.p.applyVhostForDomain(ctx, domainID)
}

// serviceAppLimitsFor: the administrator's limits of an app (the catalog
// defaults when it has no offering row yet).
func serviceAppLimitsFor(app serviceapps.App, offerings map[string]serviceAppOffering) transport.ServiceAppLimits {
	if o, ok := offerings[app.ID]; ok && o.Limits.MemoryMB > 0 {
		return o.Limits
	}
	return transport.ServiceAppLimits{MemoryMB: app.Defaults.MemoryMB, CPUPercent: app.Defaults.CPUPercent, TasksMax: app.Defaults.TasksMax}
}

func serviceAppLabel(in serviceAppInstall) string {
	name := in.AppID
	if app, ok := serviceapps.Lookup(in.AppID); ok {
		name = app.Name
	}
	return fmt.Sprintf("%s on %s", name, in.Domain)
}
