package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// inspectionAgents stands in for the two Agents of the automatic deletion
// check: the main's primary steps and the backup's secondary steps, with
// the idempotence the real Agent has (cmd/agent tests cover the native side).
type inspectionAgents struct {
	mu sync.Mutex
	// main
	prepares, activates []transport.DNSPeerInspectionPrimaryRequest
	credential, public  string
	active              bool
	activeHostKey       string
	mainExisting        bool
	mainFailOnce        error
	probes              []string // consumed one per activation answer
	// backup
	checks, installs []transport.DNSPeerInspectionSecondaryRequest
	checkState       string
	checkPorts       []int
	installFailOnce  error
	installedKey     string
}

const inspectionHostKey = "1111111111111111111111111111111111111111111111111111111111111111"
const inspectionPublicKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOx0Cx3yZ6eXn0Qd3B1sJ8yQnQ6m3u5W9k4bH2pY7d1F"

func (a *inspectionAgents) main(_ context.Context, method string, args, reply any) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if method != "Agent.DNSPeerInspectionPrimaryV1" {
		return errors.New("unexpected " + method)
	}
	req := *args.(*transport.DNSPeerInspectionPrimaryRequest)
	resp := reply.(*transport.DNSPeerInspectionPrimaryResponse)
	if a.mainFailOnce != nil {
		err := a.mainFailOnce
		a.mainFailOnce = nil
		return err
	}
	resp.LocalIP, resp.PeerIP = "198.51.100.20", "203.0.113.10"
	if a.mainExisting {
		resp.State, resp.CredentialID, resp.Probe = transport.DNSPeerInspectionExisting, strings.Repeat("e", 32), "reachable"
		return nil
	}
	probe := func() string {
		if len(a.probes) == 0 {
			return "reachable"
		}
		next := a.probes[0]
		a.probes = a.probes[1:]
		return next
	}
	switch req.Action {
	case transport.DNSPeerInspectionActionPrepare:
		a.prepares = append(a.prepares, req)
		if a.active && req.CredentialID == a.credential {
			resp.State, resp.CredentialID, resp.Probe = transport.DNSPeerInspectionActive, a.credential, probe()
			return nil
		}
		if a.credential == "" || req.CredentialID != a.credential {
			a.credential, a.public = strings.Repeat("a", 31)+string(rune('0'+len(a.prepares))), inspectionPublicKey
		}
		resp.State, resp.CredentialID, resp.PublicKey = transport.DNSPeerInspectionPrepared, a.credential, a.public
	case transport.DNSPeerInspectionActionActivate:
		a.activates = append(a.activates, req)
		if req.CredentialID != a.credential {
			resp.State, resp.Reason = transport.DNSPeerInspectionFailed, transport.DNSPeerInspectionReasonCredentialStale
			return nil
		}
		a.active, a.activeHostKey = true, req.HostKeySHA256
		resp.State, resp.CredentialID, resp.Probe = transport.DNSPeerInspectionActive, a.credential, probe()
	}
	return nil
}

func (a *inspectionAgents) backup(_ context.Context, method string, args, reply any) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if method != "Agent.DNSPeerInspectionSecondaryV1" {
		return errors.New("unexpected " + method)
	}
	req := *args.(*transport.DNSPeerInspectionSecondaryRequest)
	resp := reply.(*transport.DNSPeerInspectionSecondaryResponse)
	resp.Engine, resp.PrimaryIP, resp.LocalIP = "pdns", "198.51.100.20", "203.0.113.10"
	switch req.Action {
	case transport.DNSPeerInspectionActionCheck:
		a.checks = append(a.checks, req)
		resp.State, resp.SSHPorts = a.checkState, a.checkPorts
		if resp.State == "" {
			resp.State = transport.DNSPeerInspectionReady
		}
	case transport.DNSPeerInspectionActionInstall:
		a.installs = append(a.installs, req)
		if a.installFailOnce != nil {
			err := a.installFailOnce
			a.installFailOnce = nil
			return err
		}
		a.installedKey = req.PublicKey
		resp.State, resp.HostKeySHA256 = transport.DNSPeerInspectionInstalled, inspectionHostKey
	}
	return nil
}

func inspectionPair(t *testing.T) (*peerPair, serverSetupPlan, *serverSetupExecution, *inspectionAgents) {
	t.Helper()
	pair, plan, second := boundPeerPair(t)
	agents := &inspectionAgents{}
	pair.first.panel.serverSetupPeerInspectionCall = agents.main
	pair.second.panel.serverSetupPeerInspectionCall = agents.backup
	return pair, plan, second, agents
}

func inspectionRecordOf(t *testing.T, f serviceOperationTestFixture) serverSetupPeerInspectionRecord {
	t.Helper()
	record, found, err := f.panel.readServerSetupPeerInspection(context.Background())
	if err != nil || !found {
		t.Fatalf("inspection record: %v %v", found, err)
	}
	return record
}

// The owner's pair: the backup's completion sets up the deletion check with
// the main over the setup link (prepare, install, activate pinned to the
// backup's host key), then reports ready. Both setup pages say it is set
// up; no secret is in either record or view.
func TestServerSetupPeerInspectionIsSetUpWhenTheBackupCompletes(t *testing.T) {
	pair, plan, second, agents := inspectionPair(t)
	completeWithChecks(t, pair.first, pair.plan, pair.execution)
	completeWithChecks(t, pair.second, plan, second)
	if second.Status != "succeeded" || pair.execution.Status != "succeeded" {
		t.Fatalf("both complete: %s %+v / %s %+v", second.Status, second.Error, pair.execution.Status, pair.execution.Error)
	}
	if len(agents.prepares) != 1 || len(agents.installs) != 1 || len(agents.activates) != 1 ||
		agents.installedKey != inspectionPublicKey || agents.activeHostKey != inspectionHostKey ||
		agents.activates[0].Engine != "pdns" || agents.activates[0].CredentialID != agents.credential {
		t.Fatalf("steps: %+v %+v %+v", agents.prepares, agents.installs, agents.activates)
	}
	link, _ := pair.second.panel.readServerSetupLink(context.Background())
	for _, f := range []serviceOperationTestFixture{pair.first, pair.second} {
		record := inspectionRecordOf(t, f)
		if record.Outcome != peerInspectionOutcomeReady || record.Wait != "" {
			t.Fatalf("record: %+v", record)
		}
		view := peerViewOf(t, f)
		if view.Inspection == nil || view.Inspection.Code != "ready" || view.Inspection.State != "ready" ||
			view.Inspection.Vars["main"] != "198.51.100.20" || view.Inspection.Vars["backup"] != "203.0.113.10" {
			t.Fatalf("view: %+v", view.Inspection)
		}
		raw, _ := json.Marshal(record)
		viewRaw, _ := json.Marshal(view)
		for _, secret := range []string{link.LinkCredential, serverSetupReverseCredential(link.LinkCredential), "PRIVATE KEY"} {
			if secret != "" && (strings.Contains(string(raw), secret) || strings.Contains(string(viewRaw), secret)) {
				t.Fatal("a secret reached the record or the view")
			}
		}
	}
	// After the report the link has ended: asking again changes nothing.
	state, err := pair.second.panel.loadServerSetup(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := pair.second.panel.enrollServerSetupPeerInspection(context.Background(), state.Draft); err != nil {
		t.Fatal(err)
	}
	if len(agents.prepares) != 1 || len(agents.installs) != 1 || len(agents.checks) != 1 {
		t.Fatal("the automation ran again after it finished")
	}
}

// Interrupted at each side, the same completion continues the same
// enrollment: the main's Agent did not answer (a wait, the link stays open),
// then the backup's Agent did not answer the install; the retry reuses the
// prepared key, installs once and activates once.
func TestServerSetupPeerInspectionResumesAfterAnInterruption(t *testing.T) {
	pair, plan, second, agents := inspectionPair(t)
	completeWithChecks(t, pair.first, pair.plan, pair.execution)
	agents.mainFailOnce = errors.New("connection refused")
	completeWithChecks(t, pair.second, plan, second)
	if second.Status != "waiting" || second.Error == nil || second.Error.Code != "server_setup_peer_inspection_main_agent" ||
		!strings.Contains(second.Error.Message, "tries again by itself") {
		t.Fatalf("main Agent wait: %s %+v", second.Status, second.Error)
	}
	if join := firstJoinRecord(t, pair); join.PeerReadyAt != "" || join.LinkCredentialHash == "" {
		t.Fatal("the backup reported ready before the check was set up")
	}
	if view := peerViewOf(t, pair.second); view.Inspection == nil || view.Inspection.Code != "main_agent" || view.Inspection.State != "working" {
		t.Fatalf("waiting view: %+v", view.Inspection)
	}
	agents.installFailOnce = errors.New("agent restarting")
	completeWithChecks(t, pair.second, plan, second)
	if second.Status != "waiting" || second.Error.Code != "server_setup_peer_inspection_agent" {
		t.Fatalf("backup Agent wait: %s %+v", second.Status, second.Error)
	}
	prepared := inspectionRecordOf(t, pair.second).CredentialID
	completeWithChecks(t, pair.second, plan, second)
	if second.Status != "succeeded" {
		t.Fatalf("after the retry: %s %+v", second.Status, second.Error)
	}
	if len(agents.prepares) != 2 || agents.prepares[1].CredentialID != prepared || len(agents.activates) != 1 || agents.activeHostKey != inspectionHostKey {
		t.Fatalf("retry did not continue the same enrollment: %+v %+v", agents.prepares, agents.activates)
	}
	if record := inspectionRecordOf(t, pair.second); record.Outcome != peerInspectionOutcomeReady {
		t.Fatalf("record: %+v", record)
	}
}

// An enrollment the owner made earlier is kept: the main reports it as
// existing, nothing is installed, and the backup reports ready.
func TestServerSetupPeerInspectionKeepsAnExistingOwnerEnrollment(t *testing.T) {
	pair, plan, second, agents := inspectionPair(t)
	agents.mainExisting = true
	completeWithChecks(t, pair.first, pair.plan, pair.execution)
	completeWithChecks(t, pair.second, plan, second)
	if second.Status != "succeeded" || len(agents.installs) != 0 {
		t.Fatalf("existing enrollment: %s %+v installs=%d", second.Status, second.Error, len(agents.installs))
	}
	for _, f := range []serviceOperationTestFixture{pair.first, pair.second} {
		if view := peerViewOf(t, f); view.Inspection == nil || view.Inspection.Code != "existing" {
			t.Fatalf("view: %+v", view.Inspection)
		}
	}
}

// Owner state on the backup (another channel, revoked, a changed sshd_config)
// is final: the main is never asked, nothing changes, the backup reports.
func TestServerSetupPeerInspectionReportsOwnerStateOnTheBackup(t *testing.T) {
	for _, state := range []string{transport.DNSPeerInspectionOwnerConflict, transport.DNSPeerInspectionRevoked, transport.DNSPeerInspectionInspectorUnavailable} {
		pair, plan, second, agents := inspectionPair(t)
		agents.checkState = state
		completeWithChecks(t, pair.first, pair.plan, pair.execution)
		completeWithChecks(t, pair.second, plan, second)
		if second.Status != "succeeded" || len(agents.prepares) != 0 || len(agents.installs) != 0 {
			t.Fatalf("%s: %s %+v", state, second.Status, second.Error)
		}
		view := peerViewOf(t, pair.second)
		if view.Inspection == nil || view.Inspection.State != "attention" || view.Inspection.Message == "" {
			t.Fatalf("%s view: %+v", state, view.Inspection)
		}
	}
}

// SSH on another port than 22 is named with the port; once the owner adds
// port 22 the same completion goes on by itself.
func TestServerSetupPeerInspectionWaitsForSSHOnPort22(t *testing.T) {
	pair, plan, second, agents := inspectionPair(t)
	agents.checkState, agents.checkPorts = transport.DNSPeerInspectionSSHPort, []int{2222}
	completeWithChecks(t, pair.first, pair.plan, pair.execution)
	completeWithChecks(t, pair.second, plan, second)
	if second.Status != "waiting" || second.Error.Code != "server_setup_peer_inspection_ssh_port" || !strings.Contains(second.Error.Message, "2222") ||
		!strings.Contains(second.Error.Message, "Stop waiting for the other server") {
		t.Fatalf("ssh port wait: %s %+v", second.Status, second.Error)
	}
	if view := peerViewOf(t, pair.second); view.Inspection == nil || view.Inspection.Code != "ssh_port" || view.Inspection.Vars["ports"] != "2222" {
		t.Fatalf("ssh port view: %+v", view.Inspection)
	}
	agents.checkState, agents.checkPorts = "", nil
	completeWithChecks(t, pair.second, plan, second)
	if second.Status != "succeeded" || len(agents.activates) != 1 {
		t.Fatalf("after port 22: %s %+v", second.Status, second.Error)
	}
}

// The main could not reach the backup's SSH after activating: a wait on the
// backup with the concrete action, shown on the main too; the next pass
// probes again (no second activation) and finishes.
func TestServerSetupPeerInspectionWaitsUntilTheMainReachesTheBackupSSH(t *testing.T) {
	pair, plan, second, agents := inspectionPair(t)
	agents.probes = []string{"unreachable"}
	completeWithChecks(t, pair.first, pair.plan, pair.execution)
	completeWithChecks(t, pair.second, plan, second)
	if second.Status != "waiting" || second.Error.Code != "server_setup_peer_inspection_ssh_unreachable" ||
		!strings.Contains(second.Error.Message, "TCP port 22") || !strings.Contains(second.Error.Message, "203.0.113.10") {
		t.Fatalf("unreachable wait: %s %+v", second.Status, second.Error)
	}
	if view := peerViewOf(t, pair.first); view.Inspection == nil || view.Inspection.Code != "ssh_unreachable" {
		t.Fatalf("main view: %+v", view.Inspection)
	}
	completeWithChecks(t, pair.second, plan, second)
	if second.Status != "succeeded" || len(agents.activates) != 1 || len(agents.installs) != 1 {
		t.Fatalf("after the firewall fix: %s %+v activates=%d installs=%d", second.Status, second.Error, len(agents.activates), len(agents.installs))
	}
}

// Agents without the automation (an older Agent): a known final state, the
// owner tool remains, and the backup's report is not held.
func TestServerSetupPeerInspectionWithoutTheAgentStepIsUnsupported(t *testing.T) {
	pair, plan, second := boundPeerPair(t)
	completeWithChecks(t, pair.first, pair.plan, pair.execution)
	completeWithChecks(t, pair.second, plan, second)
	if second.Status != "succeeded" {
		t.Fatalf("report held: %s %+v", second.Status, second.Error)
	}
	if view := peerViewOf(t, pair.second); view.Inspection == nil || view.Inspection.Code != "unsupported" {
		t.Fatalf("view: %+v", view.Inspection)
	}
}
