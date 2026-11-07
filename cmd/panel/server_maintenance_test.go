package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.128 ops, G29: Server → Maintenance for the administrator.

func maintenanceCustomerRequest(method, target, body string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	return req.WithContext(context.WithValue(req.Context(), callerKey, &Caller{ID: 99, Role: roleCustomer}))
}

func TestServerMaintenanceRoutesAreTheAdministrators(t *testing.T) {
	panel := &Panel{}
	for _, item := range []struct {
		method, path string
		handle       func(http.ResponseWriter, *http.Request)
	}{
		{http.MethodGet, serverClockPath, panel.handleServerClock},
		{http.MethodGet, serverProcessesPath, panel.handleServerProcesses},
		{http.MethodPost, serverRebootPath, panel.handleServerReboot},
	} {
		recorder := httptest.NewRecorder()
		item.handle(recorder, maintenanceCustomerRequest(item.method, item.path, `{}`))
		if recorder.Code != http.StatusForbidden || recorder.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%s: a customer got %d", item.path, recorder.Code)
		}
	}
}

func TestServerClockSaysTheZoneTheBootAndWhetherItIsSynchronised(t *testing.T) {
	dir := t.TempDir()
	zoneinfo := filepath.Join(dir, "usr", "share", "zoneinfo", "Europe")
	if err := os.MkdirAll(zoneinfo, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(zoneinfo, "Istanbul"), []byte("TZif"), 0o644); err != nil {
		t.Fatal(err)
	}
	localtime := filepath.Join(dir, "localtime")
	if err := os.Symlink(filepath.Join(zoneinfo, "Istanbul"), localtime); err != nil {
		t.Fatal(err)
	}
	bootID := filepath.Join(dir, "boot_id")
	_ = os.WriteFile(bootID, []byte("0a1b2c3d-0000-4000-8000-123456789abc\n"), 0o644)
	uptime := filepath.Join(dir, "uptime")
	_ = os.WriteFile(uptime, []byte("3600.50 7000.00\n"), 0o644)
	restore := []func(){}
	set := func(target *string, value string) {
		previous := *target
		*target = value
		restore = append(restore, func() { *target = previous })
	}
	set(&serverLocaltimePath, localtime)
	set(&serverTimezonePath, filepath.Join(dir, "missing"))
	set(&serverBootIDPath, bootID)
	set(&serverUptimePath, uptime)
	previousNow, previousSync := serverClockNow, serverClockSync
	fixed := time.Date(2026, 11, 7, 12, 0, 0, 0, time.FixedZone("+03", 3*3600))
	serverClockNow = func() time.Time { return fixed }
	serverClockSync = func() clockSync { return clockSync{State: "not_synchronised"} }
	t.Cleanup(func() {
		for _, undo := range restore {
			undo()
		}
		serverClockNow, serverClockSync = previousNow, previousSync
	})

	panel := &Panel{}
	recorder := httptest.NewRecorder()
	panel.handleServerClock(recorder, serviceStateAtomicRequest(http.MethodGet, serverClockPath, "", 1))
	var view serverClockView
	if err := json.Unmarshal(recorder.Body.Bytes(), &view); err != nil || recorder.Code != http.StatusOK {
		t.Fatalf("clock: %d %s", recorder.Code, recorder.Body.String())
	}
	if view.TimeZone != "Europe/Istanbul" || view.UTCOffset != "+03:00" || view.Sync != "not_synchronised" ||
		view.BootID != "0a1b2c3d-0000-4000-8000-123456789abc" || view.BootedAt != "2026-11-07T10:59:59+03:00" {
		t.Fatalf("clock view: %+v", view)
	}
	if formatUTCOffset(-(5*3600 + 30*60)) != "-05:30" {
		t.Fatal("negative offset")
	}
	// A clock that cannot be read is unknown, never "synchronised".
	serverClockSync = func() clockSync { return clockSync{State: "unknown"} }
	serverBootIDPath = filepath.Join(dir, "missing")
	view = readServerClock()
	if view.Sync != "unknown" || view.BootID != "" {
		t.Fatalf("unknown: %+v", view)
	}
}

func TestServerProcessesKeepTheShortNameOnlyAndSayWhenHidden(t *testing.T) {
	root := t.TempDir()
	write := func(pid, stat string) {
		if err := os.MkdirAll(filepath.Join(root, pid), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, pid, "stat"), []byte(stat), 0o644); err != nil {
			t.Fatal(err)
		}
		_ = os.WriteFile(filepath.Join(root, pid, "cmdline"), []byte("mysqld\x00--password=secret"), 0o644)
	}
	statLine := func(pid, name string, ticks, rss int) string {
		fields := make([]string, 22)
		for i := range fields {
			fields[i] = "0"
		}
		fields[11] = strconv.Itoa(ticks)
		fields[21] = strconv.Itoa(rss)
		return pid + " (" + name + ") " + strings.Join(fields, " ")
	}
	write("1", statLine("1", "systemd", 10, 100))
	write("20", statLine("20", "mysqld", 10, 900000))
	previousRoot, previousSleep := machineProcRoot, serverProcessSleep
	machineProcRoot = root
	serverProcessSleep = func(time.Duration) { write("20", statLine("20", "mysqld", 60, 900000)) }
	t.Cleanup(func() { machineProcRoot, serverProcessSleep = previousRoot, previousSleep })

	panel := &Panel{}
	recorder := httptest.NewRecorder()
	panel.handleServerProcesses(recorder, serviceStateAtomicRequest(http.MethodGet, serverProcessesPath, "", 1))
	body := recorder.Body.String()
	if recorder.Code != http.StatusOK || strings.Contains(body, "password") || strings.Contains(body, "secret") {
		t.Fatalf("processes: %d %s", recorder.Code, body)
	}
	var view serverProcessesView
	if err := json.Unmarshal(recorder.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.State != "read" || view.Total != 2 || len(view.ByCPU) != 2 || view.ByCPU[0].Name != "mysqld" || view.ByMemory[0].Name != "mysqld" {
		t.Fatalf("view: %+v", view)
	}

	// hidepid: process 1 is not readable, so the list would name only the
	// panel's own processes; the answer says hidden.
	_ = os.RemoveAll(filepath.Join(root, "1"))
	if hidden := readServerProcesses(); hidden.State != "hidden" || len(hidden.ByCPU) != 0 {
		t.Fatalf("hidden: %+v", hidden)
	}
}

func rebootTestCall(t *testing.T, panel *Panel, userID int, answer transport.ServerRebootResponse, err error) (*httptest.ResponseRecorder, int) {
	t.Helper()
	calls := 0
	previous := callServerRebootAgent
	callServerRebootAgent = func(*Panel, context.Context, *transport.ServerRebootRequest) (transport.ServerRebootResponse, error) {
		calls++
		return answer, err
	}
	t.Cleanup(func() { callServerRebootAgent = previous })
	recorder := httptest.NewRecorder()
	panel.handleServerReboot(recorder, serviceStateAtomicRequest(http.MethodPost, serverRebootPath, `{}`, userID))
	callServerRebootAgent = previous
	return recorder, calls
}

func TestServerRebootIsRefusedWhileWorkRunsAndAskedOnceWhenIdle(t *testing.T) {
	panel, _, database, userID := newServiceStateAtomicPanel(t)

	// Scheduled: 202 with the delay; the audit row is written first.
	recorder, calls := rebootTestCall(t, panel, userID, transport.ServerRebootResponse{Outcome: transport.ServerRebootScheduled, DelaySeconds: 3}, nil)
	if recorder.Code != http.StatusAccepted || calls != 1 || !strings.Contains(recorder.Body.String(), `"state":"restarting"`) {
		t.Fatalf("scheduled: %d %s (calls %d)", recorder.Code, recorder.Body.String(), calls)
	}
	var audited int
	_ = database.GetDB().QueryRow(`SELECT count(*) FROM audit_logs WHERE action = 'server.reboot'`).Scan(&audited)
	if audited != 1 {
		t.Fatalf("audit rows: %d", audited)
	}

	// The Agent's refusals are said with their reason and change nothing.
	for outcome, words := range map[string]string{
		transport.ServerRebootBusyPackageManager: "package manager",
		transport.ServerRebootBusyUpdate:         "CelikPanel update",
		transport.ServerRebootBusyMutation:       "server change",
	} {
		recorder, _ = rebootTestCall(t, panel, userID, transport.ServerRebootResponse{Outcome: outcome}, nil)
		if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), words) || !strings.Contains(recorder.Body.String(), serverRebootBusyCode) {
			t.Fatalf("%s: %d %s", outcome, recorder.Code, recorder.Body.String())
		}
	}
	recorder, _ = rebootTestCall(t, panel, userID, transport.ServerRebootResponse{Outcome: transport.ServerRebootUnavailable}, nil)
	if recorder.Code != http.StatusServiceUnavailable || !strings.Contains(recorder.Body.String(), "sudo systemctl reboot") {
		t.Fatalf("unavailable: %d %s", recorder.Code, recorder.Body.String())
	}
	// No answer: the outcome is unknown, said so (D-024).
	recorder, _ = rebootTestCall(t, panel, userID, transport.ServerRebootResponse{}, errors.New("connection reset"))
	if recorder.Code != http.StatusBadGateway || !strings.Contains(recorder.Body.String(), serverRebootUnknownCode) || !strings.Contains(recorder.Body.String(), "not known") {
		t.Fatalf("unknown: %d %s", recorder.Code, recorder.Body.String())
	}

	// A domain deletion running: refused before the Agent is asked.
	end := panel.deletionsRunning.begin(5)
	recorder, calls = rebootTestCall(t, panel, userID, transport.ServerRebootResponse{Outcome: transport.ServerRebootScheduled}, nil)
	end()
	if recorder.Code != http.StatusConflict || calls != 0 || !strings.Contains(recorder.Body.String(), "is running now") {
		t.Fatalf("busy: %d %s (calls %d)", recorder.Code, recorder.Body.String(), calls)
	}

	// The panel's own change lock held: refused, the Agent is not asked.
	panel.serviceMutationMu.Lock()
	recorder, calls = rebootTestCall(t, panel, userID, transport.ServerRebootResponse{Outcome: transport.ServerRebootScheduled}, nil)
	panel.serviceMutationMu.Unlock()
	if recorder.Code != http.StatusConflict || calls != 0 {
		t.Fatalf("locked: %d (calls %d)", recorder.Code, calls)
	}

	// A body with anything in it is refused.
	recorder = httptest.NewRecorder()
	panel.handleServerReboot(recorder, serviceStateAtomicRequest(http.MethodPost, serverRebootPath, `{"shutdown":true}`, userID))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("unknown field: %d", recorder.Code)
	}
}

func TestServerRebootBusyQueriesReadEveryTable(t *testing.T) {
	panel, _, _, _ := newServiceStateAtomicPanel(t)
	what, err := panel.serverRebootBlocker(context.Background(), "")
	if err != nil || what != "" {
		t.Fatalf("idle panel: %q %v", what, err)
	}
}

func TestServerRebootRouteAlwaysNeedsAnIdentity(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, serverRebootPath, nil)
	route, ok := operationStepRouteFor(request)
	if !ok || route.optional || route.pattern != serverRebootPath {
		t.Fatalf("route: %+v %v", route, ok)
	}
}
