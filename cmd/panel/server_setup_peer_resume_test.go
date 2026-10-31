package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// D11 (final alpha.113 acceptance run 5, 31 Oct 2026): after a step failed
// on a paired main, the owner's documented path ("review the plan again",
// then start) was refused: the review checked the zone's records for the
// second server against a stand-in address, the zone already held ns2 with
// the joined server's real address, and the plan answered
// server_setup_infrastructure_dns_conflict. The review now uses the second
// server that joined this draft, so the owner can resume; an owner's own
// change of those records stays a conflict.

// bindJoinedPair runs the first server to its join step and binds the
// second server's names, as TestServerSetupPeerNamesBindTheSecondServerOnceAcrossRestarts.
func bindJoinedPair(t *testing.T) (*peerPair, *serverSetupPeerBinding) {
	t.Helper()
	pair := newPeerPair(t, joinFirstDraft())
	view := issueJoinCode(t, pair.first)
	if answer := pair.connect(t, view.Code); answer.State != "ok" {
		t.Fatalf("connect: %+v", answer)
	}
	for index, step := range pair.execution.Steps {
		if step.Kind == "peer_join" {
			break
		}
		pair.execution.Steps[index].Status = "succeeded"
	}
	if _, err := pair.first.panel.advanceServerSetupExecution(pair.plan, pair.execution); err != nil {
		t.Fatal(err)
	}
	_, plan := pair.secondReview(t, nil)
	step := serverSetupExecutionStep{serverSetupPlanStep: plan.Steps[0], RequestID: strings.Repeat("5", 32), OwnerID: strings.Repeat("6", 32)}
	if done, err := pair.second.panel.runServerSetupPeerNames(context.Background(), plan, step); !done || err != nil {
		t.Fatalf("names: %v %v", done, err)
	}
	if _, err := pair.first.panel.advanceServerSetupExecution(pair.plan, pair.execution); err != nil {
		t.Fatal(err)
	}
	binding, err := pair.first.panel.readServerSetupPeerBinding(context.Background(), pair.plan, pair.execution.ID)
	if err != nil || binding == nil || binding.InfrastructureDNS == nil {
		t.Fatalf("binding: %+v %v", binding, err)
	}
	return pair, binding
}

// publishJoinedZone writes the setup zone as the infrastructure DNS step
// left it: the records of the bound plan, owned by setup.
func publishJoinedZone(t *testing.T, f serviceOperationTestFixture, planID string, records []serverSetupInfrastructureDNSRecord) {
	t.Helper()
	db := f.database.GetDB()
	result, err := db.Exec(`INSERT INTO pdns_domains(name,type) VALUES('example.test','MASTER')`)
	if err != nil {
		t.Fatal(err)
	}
	zoneID, _ := result.LastInsertId()
	owner, _ := json.Marshal(serverSetupInfrastructureDNSOwner{Zone: "example.test", ZoneID: zoneID, PlanID: planID})
	if _, err := db.Exec(`INSERT INTO panel_settings(key,value) VALUES(?,?)`, serverSetupInfrastructureDNSOwnerKey("example.test"), string(owner)); err != nil {
		t.Fatal(err)
	}
	for _, record := range records {
		if _, err := db.Exec(`INSERT INTO pdns_records(domain_id,name,type,content,ttl,prio,disabled,auth) VALUES(?,?,?,?,?,?,0,1)`,
			zoneID, record.Name, record.Type, record.Content, record.TTL, record.Priority); err != nil {
			t.Fatal(err)
		}
	}
}

func infrastructureConflict(blockers []string) string {
	for _, blocker := range blockers {
		if strings.HasPrefix(blocker, "server_setup_infrastructure_dns_conflict") {
			return blocker
		}
	}
	return ""
}

func TestServerSetupPairedMainCanReviewAndResumeAfterAFailedStep(t *testing.T) {
	pair, binding := bindJoinedPair(t)
	ctx := context.Background()
	f := pair.first
	var ns2 string
	for _, record := range binding.InfrastructureDNS.Records {
		if record.Name == "ns2.example.test" && record.Type == "A" {
			ns2 = record.Content
		}
	}
	if ns2 != "198.51.100.20" {
		t.Fatalf("the bound plan does not give ns2 the joined address: %+v", binding.InfrastructureDNS.Records)
	}
	publishJoinedZone(t, f, pair.plan.ID, binding.InfrastructureDNS.Records)
	// A later step failed (D10's mail DNS step in run 5): the run ended.
	if _, err := f.database.GetDB().Exec(`UPDATE server_setup_executions SET status='failed' WHERE id=?`, pair.execution.ID); err != nil {
		t.Fatal(err)
	}
	f.panel.releaseFailedServerSetupDraft(ctx, pair.plan)

	// The owner's path: review the plan again. The draft still has no
	// peer address of its own (it lives in the join), and the review uses
	// the joined server's address, so the zone's ns2 is the plan's ns2.
	state, err := f.panel.loadServerSetup(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if state.Draft.PeerIP != "" {
		t.Fatalf("fixture: the draft holds a peer address: %q", state.Draft.PeerIP)
	}
	review, err := f.panel.buildServerSetupPlan(ctx, state, serviceOperationActor{UserID: f.userID})
	if err != nil {
		t.Fatal(err)
	}
	if conflict := infrastructureConflict(review.Blockers); conflict != "" || !review.CanStart {
		t.Fatalf("the paired main cannot resume: can_start=%v blockers=%v", review.CanStart, review.Blockers)
	}
	if !serverSetupPlanHasStep(review, "peer_join") || !serverSetupPlanHasStep(review, "infrastructure_dns") {
		t.Fatalf("the revised plan lost its join or zone step: %+v", review.Steps)
	}

	// The revised run binds the same second server again, without a new
	// code, and its zone records are kept as they are.
	executionID := strings.Repeat("8", 32)
	if _, err := f.database.GetDB().Exec(`INSERT INTO server_setup_executions(id,request_id,plan_id,status,execution_json,created_at,updated_at) VALUES(?,?,?,'running','{}','now','now')`,
		executionID, executionID, review.ID); err != nil {
		t.Fatal(err)
	}
	if done, err := f.panel.runServerSetupPeerJoin(ctx, review, executionID); !done || err != nil {
		t.Fatalf("the revised run did not bind the joined server: %v %v", done, err)
	}
	rebound, err := f.panel.readServerSetupPeerBinding(ctx, review, executionID)
	if err != nil || rebound == nil || rebound.PeerIPv4 != "198.51.100.20" || rebound.JoinRequestID != binding.JoinRequestID {
		t.Fatalf("rebinding: %+v %v", rebound, err)
	}
	for _, record := range rebound.InfrastructureDNS.Records {
		if record.Type == "A" && record.Action != "keep" {
			t.Fatalf("an existing address record would be written again: %+v", record)
		}
	}

	// The owner's own change of the second server's address in the zone is
	// still a conflict: the review never overwrites it.
	if _, err := f.database.GetDB().Exec(`UPDATE pdns_records SET content='198.51.100.99' WHERE name='ns2.example.test' AND type='A'`); err != nil {
		t.Fatal(err)
	}
	review, err = f.panel.buildServerSetupPlan(ctx, state, serviceOperationActor{UserID: f.userID})
	if err != nil {
		t.Fatal(err)
	}
	if conflict := infrastructureConflict(review.Blockers); review.CanStart || !strings.Contains(conflict, "ns2.example.test") {
		t.Fatalf("an owner's change of ns2 was not a conflict: can_start=%v blockers=%v", review.CanStart, review.Blockers)
	}
}

// Before the second server joins, the review keeps its stand-in address.
func TestServerSetupReviewBeforeTheJoinUsesNoJoinedServer(t *testing.T) {
	f, _, _ := joinFirstFixture(t, joinFirstDraft())
	state, err := f.panel.loadServerSetup(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	joined, err := f.panel.serverSetupJoinedReviewBinding(context.Background(), state.Draft)
	if err != nil || joined != nil {
		t.Fatalf("a review before any join used a joined server: %+v %v", joined, err)
	}
}
