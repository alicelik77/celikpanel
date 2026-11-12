package main

import (
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Final alpha.113 acceptance run 3: the first, transient answer of a paired
// deletion's peer proof (dns_peer_inspection_unknown) was the state a read
// saw while the same request was about to check again, and its guidance
// named people who must act. A read during the request says the deletion is
// running and re-checked automatically; once the request has ended the
// reviewed reason and its actions are shown as before.
func TestDomainDeletionStatusSaysRunningWhileTheRequestRuns(t *testing.T) {
	p := newDNSPanelForTest(t)
	id, lease := seedExactDeletionStatusForEngine(t, p, transport.DNSEnginePowerDNS)
	agent := newDNSZoneV3TestAgent()
	agent.durableMutationRPCFixture.jobs = map[string]*ServiceOperationMutationJob{}
	phase, err := dnsZoneSyncV3PendingPhase(lease.identity())
	if err != nil {
		t.Fatal(err)
	}
	agent.durableMutationRPCFixture.jobs[lease.RequestID] = &ServiceOperationMutationJob{
		RequestID: lease.RequestID, OwnerID: lease.OwnerID,
		Kind: "dns_zone_sync", Target: lease.ZoneName, PackageName: lease.Qualifier,
		Status: agentMutationPending, Phase: phase,
		ErrorCode: transport.DNSPeerPendingInspectionUnknown,
	}
	attachDNSZoneV3TestAgent(t, p, agent)

	end := p.deletionsRunning.begin(id)
	status, body := readDeletionStatusForTest(t, p, id)
	if status != http.StatusOK || body.Status != domainDeletionPendingStatus ||
		body.Stage != domainDeletionRunningStage || body.Reason != domainDeletionRunningReason {
		t.Fatalf("running deletion status=%d body=%+v", status, body)
	}
	for _, want := range []string{"running now", "checked automatically", "Nobody needs to do anything", "check again in a minute"} {
		if !strings.Contains(body.Message, want) {
			t.Fatalf("running message lacks %q: %s", want, body.Message)
		}
	}
	if strings.Contains(body.Message, "must check") || strings.Contains(body.Message, "SSH") {
		t.Fatalf("running message escalates: %s", body.Message)
	}
	// Final acceptance run 4: a standalone server has no backup DNS server,
	// so the running text, shown on every server, never names one.
	if strings.Contains(strings.ToLower(body.Message), "backup") {
		t.Fatalf("running message names a backup DNS server: %s", body.Message)
	}
	// Another domain is not affected.
	if p.deletionsRunning.isRunning(id + 1) {
		t.Fatal("another domain reported running")
	}

	end()
	end() // idempotent
	_, body = readDeletionStatusForTest(t, p, id)
	if body.Reason != transport.DNSPeerPendingInspectionUnknown || body.Stage != "dns_cleanup" {
		t.Fatalf("after the request the reviewed reason must be shown: %+v", body)
	}

	// Two overlapping requests: running until both ended.
	first, second := p.deletionsRunning.begin(id), p.deletionsRunning.begin(id)
	first()
	if !p.deletionsRunning.isRunning(id) {
		t.Fatal("ended while a request still runs")
	}
	second()
	if p.deletionsRunning.isRunning(id) {
		t.Fatal("still running after both requests ended")
	}
}

// Owner's order, 6 Nov 2026: the progress window shows the step a deletion
// is at while the request runs (mail settings, website, DNS, certificates,
// finishing). The status read names it in `step`; it only reads, and a
// domain whose request ended keeps nothing.
func TestDomainDeletionStatusNamesTheLiveStepWhileTheRequestRuns(t *testing.T) {
	p := newDNSPanelForTest(t)
	id, _ := seedExactDeletionStatusForEngine(t, p, transport.DNSEnginePowerDNS)

	// No running request: a stray step is not kept.
	p.deletionsRunning.setStep(id, "site_cleanup")
	if got := p.deletionsRunning.step(id); got != "" {
		t.Fatalf("step kept without a running request: %q", got)
	}

	end := p.deletionsRunning.begin(id)
	_, body := readDeletionStatusForTest(t, p, id)
	if body.Stage != domainDeletionRunningStage || body.Reason != domainDeletionRunningReason || body.Step != "" {
		t.Fatalf("before the first stage: %+v", body)
	}
	for _, stage := range []string{"mail_tls_cleanup", domainDeletionStageMailRuntime, "site_cleanup", "dns_cleanup", "certificate_cleanup", "ledger_delete"} {
		p.deletionsRunning.setStep(id, stage)
		status, body := readDeletionStatusForTest(t, p, id)
		if status != http.StatusOK || body.Stage != domainDeletionRunningStage ||
			body.Reason != domainDeletionRunningReason || body.Step != stage {
			t.Fatalf("stage %s: status=%d body=%+v", stage, status, body)
		}
		// Reading twice changes nothing.
		if _, again := readDeletionStatusForTest(t, p, id); again != body {
			t.Fatalf("a second read differs: %+v vs %+v", again, body)
		}
	}
	if p.deletionsRunning.step(id+1) != "" {
		t.Fatal("another domain reports a step")
	}
	end()
	if got := p.deletionsRunning.step(id); got != "" {
		t.Fatalf("step kept after the request ended: %q", got)
	}
	if _, body := readDeletionStatusForTest(t, p, id); body.Step != "" {
		t.Fatalf("ended request still names a step: %+v", body)
	}
}

// The delete handler records each stage before it runs it, in the order the
// progress window lists them.
func TestDomainDeleteHandlerRecordsStagesInOrder(t *testing.T) {
	source, err := os.ReadFile("domain_handlers.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	pairs := [][2]string{
		{`step("mail_tls_cleanup")`, `p.prepareDomainMailTLSRemoval(ctx, domainID)`},
		{`step(domainDeletionStageMailRuntime)`, `p.runDomainDeletionMailStageLocked(ctx, domainID, domain.Name)`},
		{`step(domainDeletionStageMailLists)`, `p.runDomainDeletionMailListsStage(ctx)`},
		{`step("site_cleanup")`, `"Agent.DeleteSite"`},
		{`step(domainDeletionStageDatabases)`, `p.runDomainDeletionDatabaseStage(ctx, domainID, domain.Name, plan)`},
		{`step("dns_cleanup")`, `p.removeDomainDNSForDeletion(ctx, domain.Name, parentDomainName)`},
		{`step("certificate_cleanup")`, `p.removeManagedDomainCertificates(`},
		{`step("ledger_delete")`, `p.finalizeDomainDeletion(ctx, domainID)`},
	}
	last := 0
	for _, pair := range pairs {
		at, call := strings.Index(text, pair[0]), strings.Index(text, pair[1])
		if at < 0 || call < 0 || at > call || at < last {
			t.Fatalf("%s must be recorded before %s and after the previous stage (at=%d call=%d last=%d)", pair[0], pair[1], at, call, last)
		}
		last = call
	}
}
