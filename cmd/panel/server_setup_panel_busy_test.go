package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

// panelBusyErrorForTest makes the Panel's own database answer busy exactly as
// in the lab: a DEFERRED transaction that read first meets another
// connection's write lock at its first write, and SQLite answers at once.
func panelBusyErrorForTest(t *testing.T, pool *sql.DB) error {
	t.Helper()
	ctx := context.Background()
	holder, err := beginDNSWriteTx(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Rollback()
	deferred, err := pool.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer deferred.Rollback()
	var count int
	if err := deferred.QueryRow(`SELECT count(*) FROM panel_settings`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	_, err = deferred.Exec(`INSERT INTO panel_settings (key, value) VALUES ('zz_busy_probe', 'x')`)
	if err == nil || time.Since(started) > 2*time.Second {
		t.Fatalf("no immediate busy answer: %v after %s", err, time.Since(started))
	}
	return err
}

func TestPanelDatabaseBusyIsOnlyThePanelsOwnDatabase(t *testing.T) {
	p := newDNSPanelForTest(t)
	busy := panelBusyErrorForTest(t, p.db.GetDB())
	if !panelDatabaseBusy(busy) || !panelDatabaseBusy(fmt.Errorf("persist DNS operation: %w", busy)) {
		t.Fatalf("the driver's own busy answer was not recognised: %v", busy)
	}
	for _, err := range []error{
		nil,
		// An Agent's or a remote panel's message is text, not this database.
		errors.New("database is locked (5) (SQLITE_BUSY)"),
		codedSQLiteError(5),
		fmt.Errorf("persist: %v", busy), // the type is lost: not claimed
		errors.New("zone example.test has no SOA"),
	} {
		if panelDatabaseBusy(err) {
			t.Fatalf("classified as the panel's own busy database: %v", err)
		}
	}
}

func TestServerSetupPanelBusyBacksOff(t *testing.T) {
	want := []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 20 * time.Second, 20 * time.Second}
	for index, delay := range want {
		if got := serverSetupPanelBusyDelay(index + 1); got != delay {
			t.Fatalf("attempt %d: %s, want %s", index+1, got, delay)
		}
	}
	waiting := &serverSetupExecution{Status: "waiting", PanelBusyAttempts: 3, Error: &serviceOperationError{Code: serverSetupPanelBusyCode}}
	if serverSetupRunnerPause(waiting) != 8*time.Second {
		t.Fatalf("busy wait pause: %s", serverSetupRunnerPause(waiting))
	}
	waiting.Error.Code = serverSetupHostStartingCode
	if serverSetupRunnerPause(waiting) != serverSetupWaitingInterval {
		t.Fatal("another wait lost its usual cadence")
	}
}

// alpha.123 lab (Ubuntu 22.04): a busy panel database during a setup step
// is a wait with its own guidance, never the step's verified failure (the
// DNS guidance) and never a released draft. The same step goes on once the
// database answers; past the bound it stops with the panel's own reason.
func TestServerSetupStepWaitsWhileThePanelDatabaseIsBusy(t *testing.T) {
	f, state := setupOperationFixture(t)
	plan := saveSetupPlanForTest(t, f, state)
	serverSetupRunners.Store(f.panel, true)
	t.Cleanup(func() { serverSetupRunners.Delete(f.panel) })
	response := postSetupStartForTest(t, f, plan.ID, strings.Repeat("9", 32))
	var execution serverSetupExecution
	if response.Code != 202 || json.Unmarshal(response.Body.Bytes(), &execution) != nil {
		t.Fatalf("start %d %s", response.Code, response.Body.String())
	}
	pool := f.database.GetDB()
	busy, calls := true, 0
	f.panel.serverSetupStepOverride = func(_ context.Context, step serverSetupExecutionStep) (bool, error, bool) {
		if step.ID != execution.Steps[0].ID {
			return false, nil, false
		}
		calls++
		if busy {
			return false, panelBusyErrorForTest(t, pool), true
		}
		return true, nil, true
	}
	for pass := 1; pass <= 2; pass++ {
		if _, err := f.panel.advanceServerSetupExecution(plan, &execution); err != nil {
			t.Fatal(err)
		}
		if execution.Status != "waiting" || execution.Steps[0].Status != "running" || execution.Error == nil ||
			execution.Error.Code != serverSetupPanelBusyCode || execution.PanelBusyAttempts != pass {
			t.Fatalf("pass %d: a busy panel database failed the step: %s %+v attempts=%d", pass, execution.Status, execution.Error, execution.PanelBusyAttempts)
		}
		if !strings.Contains(execution.Error.Message, "Nobody needs to act") || strings.Contains(execution.Error.Message, "DNS") {
			t.Fatalf("the wait's guidance: %q", execution.Error.Message)
		}
		var status, raw string
		if err := pool.QueryRow(`SELECT status, execution_json FROM server_setup_executions WHERE id=?`, execution.ID).Scan(&status, &raw); err != nil || status != "waiting" || !strings.Contains(raw, serverSetupPanelBusyCode) {
			t.Fatalf("pass %d stored %s %s %v", pass, status, raw, err)
		}
	}
	busy = false
	if _, err := f.panel.advanceServerSetupExecution(plan, &execution); err != nil {
		t.Fatal(err)
	}
	if execution.Steps[0].Status != "succeeded" || execution.PanelBusyAttempts != 0 || calls != 3 {
		t.Fatalf("the step did not go on: %+v calls=%d", execution, calls)
	}

	// Past the bound: the panel's own reason and next action.
	f.panel.serverSetupStepOverride = func(_ context.Context, step serverSetupExecutionStep) (bool, error, bool) {
		return false, panelBusyErrorForTest(t, pool), true
	}
	execution.Status = "waiting"
	execution.Error = &serviceOperationError{Code: serverSetupPanelBusyCode}
	execution.PanelBusyAttempts = serverSetupPanelBusyAttempts
	if _, err := f.panel.advanceServerSetupExecution(plan, &execution); err != nil {
		t.Fatal(err)
	}
	if execution.Status != "failed" || execution.Error == nil || execution.Error.Code != serverSetupPanelStayedBusyCode ||
		!strings.Contains(execution.Error.Message, "systemctl restart celikpanel-panel") {
		t.Fatalf("past the bound: %s %+v", execution.Status, execution.Error)
	}
}

// The lab's DNS step itself: another panel writer holds the database while
// the step stages its identity and records its operation. The step waits
// for the lock (busy_timeout now applies) and installs once, instead of
// answering SQLITE_BUSY at once.
func TestSetupDNSStepWaitsOutAnotherPanelWriter(t *testing.T) {
	p := newDNSPanelForTest(t)
	seedSetupDNSOwner(t, p)
	agent := newDNSEngineTestAgent()
	attachDNSEngineTestAgent(t, p, agent)
	t.Setenv("CELIKPANEL_SERVER_IP", "192.0.2.1")
	ctx := context.Background()
	holder, err := beginDNSWriteTx(ctx, p.db.GetDB())
	if err != nil {
		t.Fatal(err)
	}
	released := make(chan error, 1)
	go func() {
		time.Sleep(400 * time.Millisecond)
		released <- holder.Commit()
	}()
	if err := p.startServerSetupDNS(ctx, setupDNSTestDraft(), strings.Repeat("e", 32), serviceOperationActor{UserID: 1}); err != nil {
		t.Fatalf("a short write lock of another panel writer failed the DNS step: %v", err)
	}
	if err := <-released; err != nil {
		t.Fatal(err)
	}
	agent.mu.Lock()
	calls := agent.switchCalls
	agent.mu.Unlock()
	if calls != 1 {
		t.Fatalf("installs=%d", calls)
	}
}

// alpha.123 lab: PowerDNS was refused as the main DNS server on Debian 12
// and Ubuntu 22.04 but still offered as the backup there. One rule now
// decides both roles on the same data.
func TestServerSetupCatalogRefusesPowerDNSInBothRolesWhereUntested(t *testing.T) {
	profile := func(distro, version string) core.ManagedServiceHostProfile {
		family := "apt"
		if distro == "arch" {
			family = "pacman"
		}
		return core.ManagedServiceHostProfile{PackageFamily: family, DistroID: distro, VersionID: version, Architecture: "amd64"}
	}
	engine := func(catalog serverSetupComponentCatalog, id, role string) serverSetupDNSEngineChoice {
		for _, choice := range catalog.DNSEngines {
			if choice.ID == id && choice.Role == role {
				return choice
			}
		}
		t.Fatalf("missing %s/%s", id, role)
		return serverSetupDNSEngineChoice{}
	}
	for _, test := range []struct {
		distro, version string
		main, backup    string // "" offered, else the reason
	}{
		{"debian", "12", serverSetupPDNSUntestedSystemCode, serverSetupPDNSUntestedSystemCode},
		{"ubuntu", "22.04", serverSetupPDNSUntestedSystemCode, serverSetupPDNSUntestedSystemCode},
		{"ubuntu", "24.04", "server_setup_pdns_main_untested_system", ""},
		{"debian", "13", "", ""},
		{"arch", "", "", ""},
	} {
		catalog := buildServerSetupComponentCatalogFrom(serverSetupCatalogInputs{Host: profile(test.distro, test.version), InventoryReady: true, DNSState: &dnsEngineDBState{},
			PDNSCandidate: &transport.PDNSFreshPrimaryCandidateResponse{State: "unknown"}})
		for role, want := range map[string]string{"main": test.main, "backup": test.backup} {
			choice := engine(catalog, "pdns", role)
			if choice.Offered != (want == "") || choice.Reason != want {
				t.Fatalf("%s %s PowerDNS %s: %+v, want %q", test.distro, test.version, role, choice, want)
			}
			if bind := engine(catalog, "bind", role); !bind.Offered {
				t.Fatalf("%s %s BIND %s refused: %+v", test.distro, test.version, role, bind)
			}
		}
	}
}

// The review refuses the backup role with the catalogue's reason.
func TestServerSetupBuildRefusesPowerDNSBackupOnUntestedSystem(t *testing.T) {
	f, state := setupOperationFixture(t)
	setServerSetupTestHost(f.panel, "ubuntu", "22.04")
	caps := []string{transport.AgentCapabilityMailHostCertificateV1, transport.AgentCapabilityMailHostCertificateStageV1, transport.AgentCapabilityMailTLSSyncV2, transport.AgentCapabilityFirewallApplyV2, transport.AgentCapabilityPanelCertificateIssueV2}
	f.agent.versionCapabilities = &caps
	state.Draft.Purpose, state.Draft.DNSMode, state.Draft.DNSEngine, state.Draft.DNSRole = "web", "local", "pdns", "secondary"
	state.Draft.NS1, state.Draft.NS2, state.Draft.PeerNS = "ns1.example.test", "ns2.example.test", "ns1.example.test"
	state.Draft.LocalIP, state.Draft.PeerIP = "2.25.80.4", "72.62.38.15"
	plan, err := f.panel.buildServerSetupPlan(context.Background(), state, serviceOperationActor{UserID: f.userID})
	if err != nil {
		t.Fatal(err)
	}
	if plan.CanStart || !slices.Contains(plan.Blockers, serverSetupPDNSUntestedSystemCode) {
		t.Fatalf("PowerDNS backup on Ubuntu 22.04 was not refused at review: %v", plan.Blockers)
	}
}

// Server -> DNS and the join by code use the same rule; the setup DNS step
// refuses a plan reviewed before it with its own guidance.
func TestPowerDNSInstallPathsShareTheSystemRule(t *testing.T) {
	debian12 := core.ManagedServiceHostProfile{PackageFamily: "apt", DistroID: "debian", VersionID: "12", Architecture: "amd64"}
	debian13 := debian12
	debian13.VersionID = "13"
	fresh := dnsEngineSnapshot{Topology: transport.DNSTopologyStandalone, State: dnsEngineStateUnconfigured}
	if code := pdnsUntestedSystemBlocker(debian12, fresh, transport.DNSEnginePowerDNS); code != "pdns_untested_system" {
		t.Fatalf("Server -> DNS PowerDNS install on Debian 12: %q", code)
	}
	if code := pdnsUntestedSystemBlocker(debian13, fresh, transport.DNSEnginePowerDNS); code != "" {
		t.Fatalf("Debian 13: %q", code)
	}
	if code := pdnsUntestedSystemBlocker(debian12, fresh, transport.DNSEngineBIND); code != "" {
		t.Fatalf("BIND on Debian 12: %q", code)
	}
	pdns := serverSetupDraft{DNSMode: setupDNSModeLocal, DNSEngine: "pdns"}
	for system, refused := range map[string]bool{"ubuntu 22.04": true, "debian 12": true, "debian 13": false, "ubuntu 24.04": false, "arch": false, "": false} {
		if got := serverSetupJoinPDNSUntested(pdns, system); got != refused {
			t.Fatalf("join from %q: refused=%v", system, got)
		}
	}
	if serverSetupJoinPDNSUntested(serverSetupDraft{DNSMode: setupDNSModeLocal, DNSEngine: "bind"}, "ubuntu 22.04") {
		t.Fatal("a BIND setup refused an Ubuntu 22.04 second server")
	}
	failure := serverSetupFailureForStep(serverSetupExecutionStep{serverSetupPlanStep: serverSetupPlanStep{Kind: "dns"}}, errServerSetupPDNSUntestedSystem)
	if failure.Code != serverSetupPDNSUntestedSystemCode || !strings.Contains(failure.Message, "select BIND") {
		t.Fatalf("the DNS step's refusal: %+v", failure)
	}
}
