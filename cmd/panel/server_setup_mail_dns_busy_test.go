package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/rpc"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// D10 (final alpha.113 acceptance run 5, 31 Oct 2026): on the pair main the
// setup step 15-mail_dns saved the main domain's DKIM record, and the zone's
// publication was refused at admission because another change briefly held
// the host. The step reported that refusal as a verified failure
// (server_setup_mail_dns_publish_failed), the record stayed unpublished and
// nothing retried it. A busy refusal now makes the step wait (bounded, with
// its reason), the next pass publishes the same record, and the saved change
// is also published by the DNS reconciler by itself.

type mailDNSBusyTestAgent struct {
	*dnsZoneV3TestAgent
	signMu   sync.Mutex
	signings int
}

func (a *mailDNSBusyTestAgent) EnsureDKIMKey(req *transport.DKIMEnsureRequest, resp *transport.DKIMEnsureResponse) error {
	*resp = transport.DKIMEnsureResponse{PublicKeyB64: "MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAbusytestkey"}
	return nil
}

func (a *mailDNSBusyTestAgent) ConfigureDKIMSigning(_ *transport.ServiceMutationRequest, resp *transport.ConfigureDKIMSigningResponse) error {
	a.signMu.Lock()
	a.signings++
	a.signMu.Unlock()
	*resp = transport.ConfigureDKIMSigningResponse{Configured: true, Domains: 1}
	return nil
}

func (a *mailDNSBusyTestAgent) signingCount() int {
	a.signMu.Lock()
	defer a.signMu.Unlock()
	return a.signings
}

type mailDNSBusyFixture struct {
	panel     *Panel
	agent     *mailDNSBusyTestAgent
	clock     *pendingPublicationClock
	scheduler *dnsPendingPublicationScheduler
	plan      serverSetupPlan
	step      serverSetupExecutionStep
	hold      bool
	holder    *ServiceOperationMutationJob
}

func newMailDNSBusyFixture(t *testing.T) *mailDNSBusyFixture {
	t.Helper()
	panel, v3, clock, scheduler := newPendingPublicationPanel(t, "busy.example")
	panel.license = testPanelLicense(t, "active")
	f := &mailDNSBusyFixture{panel: panel, agent: &mailDNSBusyTestAgent{dnsZoneV3TestAgent: v3}, clock: clock, scheduler: scheduler}
	f.holder = &ServiceOperationMutationJob{RequestID: strings.Repeat("e", 32), OwnerID: strings.Repeat("f", 32),
		Kind: "dkim_signing_configure", Target: "opendkim", Status: agentMutationRunning, Phase: "starting", Attempt: 1}
	v3.busyRefusal = func(req *ServiceOperationMutationBeginRequest) (string, *ServiceOperationMutationJob, bool) {
		if f.hold && req.Kind == "dns_zone_sync" {
			return transport.HostMutationReasonAgentMutation, f.holder, true
		}
		return "", nil, false
	}
	server := rpc.NewServer()
	if err := server.RegisterName("Agent", f.agent); err != nil {
		t.Fatal(err)
	}
	connector := func(ctx context.Context) (*rpc.Client, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		serverConn, clientConn := net.Pipe()
		go server.ServeConn(serverConn)
		return rpc.NewClient(clientConn), nil
	}
	raw, err := connector(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	panel.agentClient = transport.NewReconnectingClientWithContextConnector(raw, connector)
	t.Cleanup(func() { _ = raw.Close() })

	draft := serverSetupDraft{Purpose: "web_mail", PanelDomain: "panel.busy.example", MainDomain: "busy.example",
		MailHostname: "mail.busy.example", DNSMode: setupDNSModeLocal, DNSRole: transport.DNSPairRolePrimary,
		InfrastructureDNS: &serverSetupInfrastructureDNSDraft{Zone: "busy.example"}}
	if !serverSetupNeedsMailDNSStep(draft) {
		t.Fatal("fixture draft has no mail DNS step")
	}
	f.plan = serverSetupPlan{Version: serverSetupPlanVersion, Draft: draft, Purpose: draft.Purpose}
	requestID, _ := newServiceOperationID()
	ownerID, _ := newServiceOperationID()
	f.step = serverSetupExecutionStep{serverSetupPlanStep: serverSetupPlanStep{ID: "15-mail_dns", Kind: "mail_dns", Target: "busy.example"},
		Status: "running", RequestID: requestID, OwnerID: ownerID}
	return f
}

func (f *mailDNSBusyFixture) zone(t *testing.T) dnsZoneSyncState {
	t.Helper()
	state, err := readDNSZoneSyncState(context.Background(), f.panel.db.GetDB(), "busy.example")
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func (f *mailDNSBusyFixture) dkimRecords(t *testing.T) int {
	t.Helper()
	var count int
	if err := f.panel.db.GetDB().QueryRow(`SELECT count(*) FROM pdns_records WHERE type='TXT' AND name=?`, dkimSelector+"._domainkey.busy.example").Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestSetupMailDNSWaitsForABusyHostAndPublishesTheSameRecord(t *testing.T) {
	f := newMailDNSBusyFixture(t)
	ctx := context.Background()
	f.hold = true

	done, err := f.panel.runServerSetupMailDNS(ctx, f.plan, f.step)
	var busy *serverSetupZonePublishBusyError
	if done || !errors.As(err, &busy) {
		t.Fatalf("busy publication: done=%v err=%v, want the busy wait error", done, err)
	}
	var failure *serverSetupChildFailure
	if errors.As(err, &failure) {
		t.Fatalf("a busy refusal was reported as a verified failure: %v", failure)
	}
	if detail := hostMutationBusyLogDetail(err); !strings.Contains(detail, "agent_mutation_active") || !strings.Contains(detail, `held_by="dkim_signing_configure opendkim"`) {
		t.Fatalf("log detail does not name the holder: %q", detail)
	}
	// The runner's mapping: a wait with its own reason, not a failure.
	mapped := f.panel.serverSetupAgentRequestRefusedBusy(ctx, &f.step, err)
	var wait *serverSetupPeerWait
	if !errors.As(mapped, &wait) || wait.Code != serverSetupHostBusyCode {
		t.Fatalf("mapped=%v, want the host busy wait", mapped)
	}
	for _, want := range []string{"Another change is running", "the DKIM record is saved in the zone busy.example", "Nobody needs to act", "by itself"} {
		if !strings.Contains(wait.Message, want) {
			t.Fatalf("wait message lacks %q: %s", want, wait.Message)
		}
	}
	if f.step.BusyAttempts != 1 || f.step.BusySince == "" {
		t.Fatalf("busy attempt not recorded on the step: %+v", f.step)
	}
	state := f.zone(t)
	if state.Status != "error" || !dnsZoneSyncStateRefusedBusy(state) || f.dkimRecords(t) != 1 {
		t.Fatalf("zone after the busy refusal = %+v records=%d", state, f.dkimRecords(t))
	}
	f.scheduler.mu.Lock()
	armed := f.scheduler.republishArmed
	f.scheduler.mu.Unlock()
	if !armed {
		t.Fatal("a busy refusal did not arm the automatic republication")
	}

	// The host is free: the next pass runs the same step and publishes. The
	// key is reused, signing is not configured a second time (its recorded
	// operation succeeded) and the same record is kept.
	f.hold = false
	done, err = f.panel.runServerSetupMailDNS(ctx, f.plan, f.step)
	if err != nil || !done {
		t.Fatalf("pass after the host freed: done=%v err=%v", done, err)
	}
	if got := f.agent.signingCount(); got != 1 {
		t.Fatalf("signing configured %d times, want once", got)
	}
	state = f.zone(t)
	if state.Status != "applied" || state.AppliedGeneration != state.DesiredGeneration || f.dkimRecords(t) != 1 {
		t.Fatalf("zone after the retry = %+v records=%d", state, f.dkimRecords(t))
	}
}

func TestSetupZonePublishBusyEndsPastItsBoundNamingTheCause(t *testing.T) {
	p := &Panel{}
	step := serverSetupExecutionStep{serverSetupPlanStep: serverSetupPlanStep{ID: "15-mail_dns", Kind: "mail_dns"},
		BusySince: time.Now().Add(-serverSetupHostBusyBound - time.Minute).UTC().Format(time.RFC3339Nano), BusyAttempts: 40}
	err := &serverSetupZonePublishBusyError{Zone: "busy.example", Record: "the DKIM record",
		Err: &dnsAgentPublicationError{Err: &hostMutationBusyError{reason: transport.HostMutationReasonPackageManager}}}
	mapped := p.serverSetupAgentRequestRefusedBusy(context.Background(), &step, err)
	var failure *serverSetupChildFailure
	if !errors.As(mapped, &failure) || failure.Code != serverSetupHostStayedBusyCode {
		t.Fatalf("past the bound: %v", mapped)
	}
	for _, want := range []string{"more than 30 minutes", "package manager", "nothing was published", "Review a revised plan"} {
		if !strings.Contains(failure.Message, want) {
			t.Fatalf("failure lacks %q: %s", want, failure.Message)
		}
	}
}

// Only a pure admission refusal is a busy wait. An outcome that is not known
// (a lost reply, an aggregate, a terminal-uncertain mutation) is not.
func TestDNSPublicationRefusedBusyIsOnlyThePureRefusal(t *testing.T) {
	busy := &hostMutationBusyError{reason: transport.HostMutationReasonAgentMutation}
	if !dnsPublicationRefusedBusy(&dnsAgentPublicationError{Err: busy}) {
		t.Fatal("the pure refusal is not a busy refusal")
	}
	for _, err := range []error{
		nil,
		errors.New("agent did not confirm the exact DNS publication"),
		&dnsAgentPublicationError{Err: errors.Join(busy, errors.New("DNS V3 publication terminal status is ambiguous"))},
		&agentMutationTerminalUncertainError{kind: "dns_zone_sync", err: busy},
		fmt.Errorf("%w: %w", errDNSZoneV3PropagationDeferred, busy),
	} {
		if dnsPublicationRefusedBusy(err) {
			t.Fatalf("%v was treated as a pure busy refusal", err)
		}
	}
}

// The saved change is also published by itself when no setup step runs
// again (for example a change from Domains refused while the host was busy).
func TestDNSBusyRefusedZoneIsRepublishedByTheReconciler(t *testing.T) {
	f := newMailDNSBusyFixture(t)
	ctx := context.Background()
	f.hold = true
	err := f.panel.syncZoneToDNS(ctx, "busy.example", false)
	if !dnsPublicationRefusedBusy(err) {
		t.Fatalf("publication while busy: %v", err)
	}
	if !f.panel.dnsBusyRefusedZonesRemain(ctx) {
		t.Fatal("the refused zone is not found for a republication after a restart")
	}
	// Still busy: the reconciler tries, is refused again and backs off.
	f.clock.Advance(dnsPendingPublicationMaxDelay)
	f.panel.reconcilePendingDNSPublicationsOnce(ctx)
	if state := f.zone(t); state.Status != "error" {
		t.Fatalf("zone while still busy = %+v", state)
	}
	f.scheduler.mu.Lock()
	attempts := f.scheduler.republishAttempts
	f.scheduler.mu.Unlock()
	if attempts == 0 {
		t.Fatal("a refusal while still busy did not back off")
	}
	f.hold = false
	f.clock.Advance(dnsPendingPublicationMaxDelay)
	f.panel.reconcilePendingDNSPublicationsOnce(ctx)
	if state := f.zone(t); state.Status != "applied" || state.AppliedGeneration != state.DesiredGeneration {
		t.Fatalf("zone after the host freed = %+v", state)
	}
	if f.panel.dnsBusyRefusedZonesRemain(ctx) {
		t.Fatal("a published zone still counts as refused")
	}
}

// Automatic DKIM starts no host work while server setup is changing the
// server: its pending records say they wait for another change, and the
// work runs once setup has stopped changing the server (D10).
func TestAutomaticDKIMWaitsWhileServerSetupChangesTheServer(t *testing.T) {
	f := newDKIMAutoFixture(t)
	seedStrictDNSZone(t, f.p, "held.example")
	id := f.domain(t, "held.example", "local", 0, true)
	f.queue(t, id, "held.example", dkimTriggerMailProfile)
	execution := serverSetupExecution{ID: "exec-dkim-hold", RequestID: "exec-dkim-hold", PlanID: "plan-dkim-hold", Status: "running", Phase: "mail_dns",
		Steps: []serverSetupExecutionStep{{serverSetupPlanStep: serverSetupPlanStep{ID: "15-mail_dns", Kind: "mail_dns"}, Status: "running"}}}
	raw, _ := json.Marshal(execution)
	db := f.p.db.GetDB()
	if _, err := db.Exec(`INSERT INTO server_setup_plans(id,revision,plan_json,requested_by,created_at) VALUES(?,?,?,?,?)`, execution.PlanID, 1, `{}`, 7001, "now"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO server_setup_executions(id,request_id,plan_id,status,execution_json,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`,
		execution.ID, execution.RequestID, execution.PlanID, execution.Status, string(raw), "now", "now"); err != nil {
		t.Fatal(err)
	}
	if !f.p.automaticDKIMHeldBySetup(context.Background()) {
		t.Fatal("automatic DKIM was not held while setup changes the server")
	}
	if record, _ := f.record(t, id, "held.example"); record.State != dkimAutoWaiting || record.Reason != dkimReasonHostBusy {
		t.Fatalf("held record = %+v, want waiting for another change", record)
	}
	if f.agent.signed["held.example"] || len(f.txt(t, dkimSelector+"._domainkey.held.example")) != 0 {
		t.Fatal("automatic DKIM changed the host while setup ran")
	}
	if _, err := db.Exec(`UPDATE server_setup_executions SET status='succeeded' WHERE id=?`, execution.ID); err != nil {
		t.Fatal(err)
	}
	if f.p.automaticDKIMHeldBySetup(context.Background()) {
		t.Fatal("automatic DKIM still held after setup stopped changing the server")
	}
	if retry := f.pass(t); retry {
		t.Fatal("the pass after setup still waits")
	}
	if _, ok := f.record(t, id, "held.example"); ok || !f.agent.signed["held.example"] {
		t.Fatal("the held work did not run after setup")
	}
}
