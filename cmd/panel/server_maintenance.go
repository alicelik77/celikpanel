package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Server → Maintenance for the administrator (alpha.128, G29):
//
//   - GET  /api/v1/maintenance/clock      the server's time, time zone and
//     whether the clock is synchronised (read by the panel itself, no Agent);
//   - GET  /api/v1/maintenance/processes  the busiest processes by CPU and by
//     memory, from /proc as the monitoring sampler reads it: the kernel's
//     short name and the owner only, never a command line;
//   - POST /api/v1/maintenance/reboot     restart the operating system, under
//     a request identity whose steps the progress window follows.
//
// Every route is the administrator's (handler check, like the restarts
// card). Reads change nothing. The reboot is refused while any change runs
// (D-024: the reason, who acts, the next step); the Agent proves the host
// idle again under the host lock and keeps that lock until the machine goes
// down. An Agent that does not answer leaves the outcome unknown, said so.
// Nothing here is needed to run the server: the owner reboots, reads the
// clock and the processes with the operating system's own tools (D-022).
//
// Sunucu → Bakım: saat ve saat dilimi, en yoğun işlemler (yalnız kısa ad ve
// sahip) ve sunucuyu yeniden başlatma. Okumalar hiçbir şey değiştirmez;
// yeniden başlatma bir değişiklik sürerken reddedilir.

const (
	serverClockPath     = "/api/v1/maintenance/clock"
	serverProcessesPath = "/api/v1/maintenance/processes"
	serverRebootPath    = "/api/v1/maintenance/reboot"

	serverProcessTop        = 15
	serverProcessMeasure    = time.Second
	serverRebootAgentBound  = 30 * time.Second
	serverRebootBusyCode    = "SERVER_REBOOT_BUSY"
	serverRebootUnknownCode = "SERVER_REBOOT_UNKNOWN"
	serverRebootRefusedCode = "SERVER_REBOOT_REFUSED"
	serverRebootInvalidCode = "SERVER_REBOOT_INVALID"
)

// requestIdentityRouteServerReboot: the reboot runs once per identity; the
// page always sends one (operation_step_routes.go).
var requestIdentityRouteServerReboot = requestIdentityRoute{
	pattern: serverRebootPath, timeout: serverRebootAgentBound + time.Minute,
}

// Readers, replaced by tests.
var (
	serverBootIDPath    = "/proc/sys/kernel/random/boot_id"
	serverUptimePath    = "/proc/uptime"
	serverLocaltimePath = "/etc/localtime"
	serverTimezonePath  = "/etc/timezone"
	serverClockNow      = time.Now
	serverClockSync     = readClockSync
	serverProcessSleep  = time.Sleep
)

func requireMaintenanceAdmin(w http.ResponseWriter, r *http.Request) bool {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if caller := currentCaller(r); caller == nil || !caller.hasAccountRole(roleAdmin) {
		writeCodedError(w, http.StatusForbidden, "ADMIN_ONLY", "Only the administrator can use server maintenance. Nothing was changed.", "")
		return false
	}
	return true
}

// --- Clock ---------------------------------------------------------------

// clockSync is what the kernel says about the clock's discipline.
type clockSync struct {
	// State: synchronised, not_synchronised or unknown.
	State string
	// MaxErrorMS is the kernel's maximum error estimate, when known.
	MaxErrorMS *int64
}

type serverClockView struct {
	Now          string `json:"now"`
	TimeZone     string `json:"time_zone"`
	UTCOffset    string `json:"utc_offset"`
	Abbreviation string `json:"abbreviation,omitempty"`
	Sync         string `json:"sync"`
	MaxErrorMS   *int64 `json:"max_error_ms,omitempty"`
	BootID       string `json:"boot_id,omitempty"`
	BootedAt     string `json:"booted_at,omitempty"`
}

func (p *Panel) handleServerClock(w http.ResponseWriter, r *http.Request) {
	if !requireMaintenanceAdmin(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		rejectRouteMethod(w, []string{http.MethodGet})
		return
	}
	_ = json.NewEncoder(w).Encode(readServerClock())
}

func readServerClock() serverClockView {
	now := serverClockNow()
	abbreviation, offsetSeconds := now.Zone()
	sync := serverClockSync()
	view := serverClockView{
		Now:          now.Format(time.RFC3339),
		TimeZone:     readServerTimeZone(),
		UTCOffset:    formatUTCOffset(offsetSeconds),
		Abbreviation: abbreviation,
		Sync:         sync.State,
		MaxErrorMS:   sync.MaxErrorMS,
		BootID:       readServerBootID(),
	}
	if uptime, ok := readServerUptime(); ok {
		view.BootedAt = now.Add(-uptime).Truncate(time.Second).Format(time.RFC3339)
	}
	return view
}

func formatUTCOffset(seconds int) string {
	sign := "+"
	if seconds < 0 {
		sign, seconds = "-", -seconds
	}
	return sign + twoDigits(seconds/3600) + ":" + twoDigits(seconds%3600/60)
}

func twoDigits(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}

// readServerTimeZone names the zone /etc/localtime points to (the name
// timedatectl shows), else /etc/timezone, else "".
func readServerTimeZone() string {
	if target, err := os.Readlink(serverLocaltimePath); err == nil {
		if _, name, found := strings.Cut(filepath.ToSlash(target), "zoneinfo/"); found && validTimeZoneName(name) {
			return name
		}
	}
	if data, err := os.ReadFile(serverTimezonePath); err == nil {
		if name := strings.TrimSpace(string(data)); validTimeZoneName(name) {
			return name
		}
	}
	return ""
}

func validTimeZoneName(name string) bool {
	if name == "" || len(name) > 64 || strings.Contains(name, "..") {
		return false
	}
	for _, c := range name {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("/_-+", c)) {
			return false
		}
	}
	return true
}

func readServerBootID() string {
	data, err := os.ReadFile(serverBootIDPath)
	if err != nil {
		return ""
	}
	id := strings.TrimSpace(string(data))
	if len(id) != 36 {
		return ""
	}
	for _, c := range id {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c == '-') {
			return ""
		}
	}
	return id
}

func readServerUptime() (time.Duration, bool) {
	data, err := os.ReadFile(serverUptimePath)
	if err != nil {
		return 0, false
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0, false
	}
	seconds, err := strconv.ParseFloat(fields[0], 64)
	if err != nil || seconds < 0 {
		return 0, false
	}
	return time.Duration(seconds * float64(time.Second)), true
}

// --- Processes -----------------------------------------------------------

type serverProcessesView struct {
	// State: read, or hidden (/proc shows the panel only its own processes).
	State           string            `json:"state"`
	MeasuredSeconds float64           `json:"measured_seconds,omitempty"`
	VCPUs           int               `json:"vcpus,omitempty"`
	Total           int               `json:"total,omitempty"`
	ByCPU           []snapshotProcess `json:"by_cpu"`
	ByMemory        []snapshotProcess `json:"by_memory"`
}

func (p *Panel) handleServerProcesses(w http.ResponseWriter, r *http.Request) {
	if !requireMaintenanceAdmin(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		rejectRouteMethod(w, []string{http.MethodGet})
		return
	}
	_ = json.NewEncoder(w).Encode(readServerProcesses())
}

// readServerProcesses takes two readings one second apart. Only the short
// name and the owner are kept, as the spike snapshots do.
func readServerProcesses() serverProcessesView {
	view := serverProcessesView{ByCPU: []snapshotProcess{}, ByMemory: []snapshotProcess{}}
	before, ok := readProcesses()
	if !ok {
		view.State = "hidden"
		return view
	}
	started := time.Now()
	serverProcessSleep(serverProcessMeasure)
	after, ok := readProcesses()
	if !ok {
		view.State = "hidden"
		return view
	}
	seconds := time.Since(started).Seconds()
	vcpus := vcpuCount()
	view.State = "read"
	view.MeasuredSeconds = round1(seconds)
	view.VCPUs = vcpus
	view.Total = len(after)
	view.ByCPU = rankProcesses(before, after, seconds, vcpus, "cpu", serverProcessTop)
	view.ByMemory = rankProcesses(before, after, seconds, vcpus, "memory", serverProcessTop)
	return view
}

// --- Reboot --------------------------------------------------------------

type serverRebootAnswer struct {
	State        string `json:"state"`
	DelaySeconds int    `json:"delay_seconds"`
	// BootID is this boot's identity: after the panel answers again the
	// screen compares it with the new one to know the server did restart.
	BootID string `json:"boot_id,omitempty"`
}

var callServerRebootAgent = func(p *Panel, ctx context.Context, request *transport.ServerRebootRequest) (transport.ServerRebootResponse, error) {
	callCtx, cancel := context.WithTimeout(ctx, serverRebootAgentBound)
	defer cancel()
	var response transport.ServerRebootResponse
	err := p.callAgentContext(callCtx, "Agent.ServerRebootV1", request, &response)
	return response, err
}

const serverRebootOwnerCommand = " If you are sure nothing is running, you, the owner, can also restart it from a terminal with `sudo systemctl reboot`."

func (p *Panel) handleServerReboot(w http.ResponseWriter, r *http.Request) {
	if !requireMaintenanceAdmin(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		rejectRouteMethod(w, []string{http.MethodPost})
		return
	}
	var req struct{}
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeCodedError(w, http.StatusBadRequest, serverRebootInvalidCode, "The request was not understood, so the server was not restarted. Reload the page and try again.", "")
		return
	}
	ctx := r.Context()
	reportOperationPlan(ctx, "check", "record", "schedule")
	reportOperationStep(ctx, "check")
	// The panel's own changes cannot start while this runs.
	if !p.serviceMutationMu.TryLock() {
		writeServerRebootBusy(w, "another change in this panel")
		return
	}
	defer p.serviceMutationMu.Unlock()
	if what, err := p.serverRebootBlocker(ctx, requestIdentityFromContext(ctx)); err != nil {
		log.Printf("server reboot: the running work could not be read: %v", err)
		writeCodedError(w, http.StatusServiceUnavailable, serverRebootRefusedCode,
			"The server was not restarted: the panel could not check whether a change is running, so it did not risk interrupting one. Nothing was changed. Try again in a minute."+serverRebootOwnerCommand, "")
		return
	} else if what != "" {
		writeServerRebootBusy(w, what)
		return
	}

	reportOperationStep(ctx, "record")
	bootID := readServerBootID()
	p.auditDetailed(r, "server.reboot", "server", 0, map[string]any{"boot_id": bootID})

	reportOperationStep(ctx, "schedule")
	response, err := callServerRebootAgent(p, ctx, &transport.ServerRebootRequest{
		ExpectedBuildCommit: strings.TrimSpace(buildCommit),
		RequestID:           requestIdentityFromContext(ctx),
	})
	if err != nil {
		log.Printf("server reboot: the Agent did not answer: %v", err)
		writeCodedError(w, http.StatusBadGateway, serverRebootUnknownCode,
			"The CelikPanel Agent did not answer, so whether the server is restarting is not known. Nobody needs to act yet: wait two minutes and reload this page. If the server did not restart, check that the agent runs (`sudo systemctl status celikpanel-agent`), then try again.", "")
		return
	}
	switch response.Outcome {
	case transport.ServerRebootScheduled, transport.ServerRebootAlreadyScheduled:
		delay := response.DelaySeconds
		if delay <= 0 {
			delay = 3
		}
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(serverRebootAnswer{State: "restarting", DelaySeconds: delay, BootID: bootID})
	case transport.ServerRebootBusyMutation:
		writeServerRebootBusy(w, "a server change")
	case transport.ServerRebootBusyPackageManager:
		writeServerRebootBusy(w, "the package manager (installing or updating software)")
	case transport.ServerRebootBusyUpdate:
		writeServerRebootBusy(w, "a CelikPanel update")
	default:
		detail := strings.TrimSpace(response.Detail)
		if detail == "" {
			detail = strings.TrimSpace(response.Error)
		}
		log.Printf("server reboot: the Agent refused: %s %s", response.Outcome, detail)
		writeCodedError(w, http.StatusServiceUnavailable, serverRebootRefusedCode,
			"The server was not restarted: the CelikPanel Agent could not confirm that nothing is running, so it did not risk interrupting a change. Nothing was changed. Try again in a minute."+serverRebootOwnerCommand, "")
	}
}

func writeServerRebootBusy(w http.ResponseWriter, what string) {
	writeCodedError(w, http.StatusConflict, serverRebootBusyCode,
		"The server was not restarted because "+what+" is running now; restarting would cut it off. Nothing was changed. Nobody needs to act: wait until it has finished, then restart again."+serverRebootOwnerCommand, "")
}

// serverRebootBusyChecks: the long work kept in the panel's tables. A row
// in one of these states is running now.
var serverRebootBusyChecks = []struct{ what, query string }{
	{"a site application install or removal", `SELECT count(*) FROM app_catalog_operations WHERE status IN ('database_creating','files_installing','setting_up','removing')`},
	{"a site application install", `SELECT count(*) FROM application_install_operations WHERE status IN ('database_creating','files_installing')`},
	{"a WordPress update, restore or copy", `SELECT count(*) FROM wordpress_operations WHERE status IN ('queued','backing_up','updating','restoring','cloning')`},
	{"a malware scan", `SELECT count(*) FROM malware_scans WHERE status IN ('queued','running')`},
	{"a remote backup transfer", `SELECT count(*) FROM remote_backup_operations WHERE state IN ('queued','running')`},
	{"a suspension change", `SELECT count(*) FROM suspension_operations WHERE state = 'running'`},
	{"a security emergency action", `SELECT count(*) FROM security_emergency_operations WHERE state = 'running'`},
	{"an AI or automation app change", `SELECT count(*) FROM service_app_operations WHERE state = 'running'`},
	{"a runtime app change", `SELECT count(*) FROM runtime_app_operations WHERE state = 'running'`},
}

// serverRebootBlocker names the work that is running now, or "". The caller
// holds serviceMutationMu. ownID is the reboot request's own identity.
func (p *Panel) serverRebootBlocker(ctx context.Context, ownID string) (string, error) {
	if active, err := p.activeServiceOperation(ctx); err != nil {
		return "", err
	} else if active != nil {
		return "a service operation (" + active.Kind + ")", nil
	}
	if p.requestIdentities != nil && p.requestIdentities.othersInFlight(ownID) > 0 {
		return "another change sent from the panel (a backup, restore, certificate or similar)", nil
	}
	if p.deletionsRunning.anyRunning() {
		return "a domain deletion", nil
	}
	if p.admittedPanelUpdateInProgress(ctx) {
		return "a CelikPanel update", nil
	}
	if mutating, err := p.serverSetupExecutionMutating(ctx); err != nil {
		return "", err
	} else if mutating {
		return "server setup", nil
	}
	database := p.db.GetDB()
	cutoff := time.Now().UTC().Add(-6 * time.Hour).Format(time.RFC3339)
	var backups int
	if err := database.QueryRowContext(ctx, `SELECT count(*) FROM backup_schedules WHERE enabled = 1 AND last_status = 'running' AND last_attempt >= ?`, cutoff).Scan(&backups); err != nil {
		return "", err
	}
	if backups > 0 {
		return "a scheduled backup", nil
	}
	for _, check := range serverRebootBusyChecks {
		var count int
		if err := database.QueryRowContext(ctx, check.query).Scan(&count); err != nil {
			return "", errors.Join(errors.New(check.what), err)
		}
		if count > 0 {
			return check.what, nil
		}
	}
	return "", nil
}

// othersInFlight counts the guarded changes this process runs now, other
// than ownID.
func (g *requestIdentityGuard) othersInFlight(ownID string) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	count := 0
	for id := range g.inFlight {
		if id != ownID {
			count++
		}
	}
	return count
}

// anyRunning: a domain deletion request runs in this process now.
func (r *domainDeletionsRunning) anyRunning() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.running) > 0
}
