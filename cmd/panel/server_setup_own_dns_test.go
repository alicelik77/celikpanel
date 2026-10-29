package main

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/core"
)

// Beta-1 re-run P9: after the P4 rejoin the backup's new review was refused
// with server_setup_existing_dns_requires_migration because of the DNS
// software its own earlier run had installed. That engine is recorded as
// this setup's own (a committed first install by a succeeded DNS step), so
// it is never "existing DNS"; a plan naming another engine is told so with
// its own code, and an engine without that record still needs migration.
func TestServerSetupOwnDNSIsNotExistingDNSThatNeedsMigration(t *testing.T) {
	agent := newDNSEngineTestAgent()
	x := newSetupDNSOutcomeFixture(t, setupDNSTestDraft(), agent)
	x.poll(t)
	if x.execution.Steps[0].Status != "succeeded" {
		t.Fatalf("the earlier run's DNS step: %+v", x.execution)
	}
	ctx := context.Background()
	state, err := readDNSEngineDBState(ctx, x.f.database.GetDB())
	if err != nil || state.ActiveEngine != "bind" {
		t.Fatalf("installed: %+v %v", state, err)
	}
	// That run stopped later (in the VM at its publisher step); the owner
	// reviews a new plan that waits for its second server again.
	if _, err := x.f.database.GetDB().Exec(`UPDATE server_setup_executions SET status='failed'`); err != nil {
		t.Fatal(err)
	}
	if _, err := x.f.database.GetDB().Exec(`UPDATE server_setup_state SET status='draft' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	review := func(change func(*serverSetupDraft)) []string {
		t.Helper()
		setup, err := x.f.panel.loadServerSetup(ctx)
		if err != nil {
			t.Fatal(err)
		}
		draft := setup.Draft
		change(&draft)
		setup, err = x.f.panel.saveServerSetupDraft(ctx, setup.Revision, draft)
		if err != nil {
			t.Fatal(err)
		}
		plan, err := x.f.panel.buildServerSetupPlan(ctx, setup, serviceOperationActor{UserID: x.f.userID})
		if err != nil {
			t.Fatal(err)
		}
		return plan.Blockers
	}
	joinPending := func(engine string) func(*serverSetupDraft) {
		return func(d *serverSetupDraft) {
			d.ServerCount, d.MainDomain, d.PeerIP, d.PeerNS, d.DNSEngine = 2, "example.test", "", "", engine
		}
	}
	if blockers := review(joinPending("bind")); slices.Contains(blockers, "server_setup_existing_dns_requires_migration") || slices.Contains(blockers, serverSetupDNSInstalledDiffersCode) {
		t.Fatalf("the setup's own BIND was treated as existing DNS: %v", blockers)
	}
	if blockers := review(joinPending("pdns")); !slices.Contains(blockers, serverSetupDNSInstalledDiffersCode) || slices.Contains(blockers, "server_setup_existing_dns_requires_migration") {
		t.Fatalf("another engine than the setup's own: %v", blockers)
	}
	// With the peer known (the plain paired review), a different address is
	// the same own-engine state, not a migration.
	if blockers := review(func(d *serverSetupDraft) {
		*d = setupDNSTestDraft()
		d.PanelDomain, d.PeerIP = "panel.example.test", "192.0.2.3"
	}); !slices.Contains(blockers, serverSetupDNSInstalledDiffersCode) || slices.Contains(blockers, "server_setup_existing_dns_requires_migration") {
		t.Fatalf("own engine, other peer: %v", blockers)
	}
	// The new run's DNS step, bound to the same peer, adopts it: nothing is
	// installed again.
	if err := x.f.panel.startServerSetupDNS(ctx, setupDNSTestDraft(), strings.Repeat("4", 32), serviceOperationActor{UserID: x.f.userID}); err != nil {
		t.Fatalf("the new run did not adopt the setup's own engine: %v", err)
	}
	if calls := x.switchCalls(); calls != 1 {
		t.Fatalf("host switch calls: %d", calls)
	}
	// A rejoin bound another second server: the DNS step says the coded
	// state, never the generic "existing DNS ownership" (review fix, item 5).
	moved := setupDNSTestDraft()
	moved.PeerIP = "192.0.2.3"
	err = x.f.panel.startServerSetupDNS(ctx, moved, strings.Repeat("3", 32), serviceOperationActor{UserID: x.f.userID})
	var differs *serverSetupChildFailure
	if !errors.As(err, &differs) || differs.Code != serverSetupDNSInstalledDiffersCode || !strings.Contains(differs.Message, "Review a revised plan") {
		t.Fatalf("own engine, other bound peer: %v", err)
	}
	if calls := x.switchCalls(); calls != 1 {
		t.Fatalf("host switch calls: %d", calls)
	}
	// Without that record (no setup run installed it) it is existing DNS.
	if _, err := x.f.database.GetDB().Exec(`DELETE FROM server_setup_executions`); err != nil {
		t.Fatal(err)
	}
	if blockers := review(joinPending("bind")); !slices.Contains(blockers, "server_setup_existing_dns_requires_migration") {
		t.Fatalf("an engine without the setup's record: %v", blockers)
	}
}

// The DNS software step says the same: the other engine is not offered
// because of the setup's own install, not because of foreign DNS.
func TestServerSetupEngineChoicesNameTheSetupsOwnEngine(t *testing.T) {
	for _, own := range []bool{false, true} {
		host := core.ManagedServiceHostProfile{PackageFamily: "apt", DistroFamily: "debian", ServiceManager: "systemd", DistroID: "debian", VersionID: "13", Architecture: "amd64"}
		choices := serverSetupDNSEngineChoices(serverSetupCatalogInputs{Host: host, InventoryReady: true, DNSState: &dnsEngineDBState{ActiveEngine: "pdns"}, OwnDNSEngine: own})
		want := "server_setup_existing_dns_requires_migration"
		if own {
			want = serverSetupDNSInstalledDiffersCode
		}
		for _, choice := range choices {
			if choice.ID == "bind" && (choice.Offered || choice.Reason != want) {
				t.Fatalf("own=%v: %+v", own, choice)
			}
		}
	}
}
