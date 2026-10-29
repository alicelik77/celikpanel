package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// Beta-1 re-run P8: the backup DNS server is the first server (Option A).
// After the P4 rejoin its run bound the new join and went on; at its
// dns_publisher step it failed with the generic server_setup_step_failed
// ("server setup changed; reload the current plan"). The cause was the
// publisher gate: the reviewed step had no endpoint (it was reviewed before
// the second server joined) and was compared with the bound endpoint. The
// run now adopts the replacing join's plan through that gate; a join
// replaced under a run that used its binding stops with a verified state.
func TestServerSetupBackupRejoinGoesThroughTheDNSPublisherGate(t *testing.T) {
	previous := probeServerSetupPrimaryCatalogSOA
	t.Cleanup(func() { probeServerSetupPrimaryCatalogSOA = previous })
	probeServerSetupPrimaryCatalogSOA = func(context.Context, string, string) (uint32, error) {
		return 0, errors.New("the main DNS server is not ready")
	}
	first := joinFirstDraft()
	first.MainDNSServer = "peer"
	pair := newPeerPair(t, first)
	publisher := -1
	for index, step := range pair.plan.Steps {
		if step.Kind == "dns_publisher" {
			publisher = index
		}
	}
	if publisher < 0 || pair.plan.Steps[publisher].Target != "" {
		t.Fatalf("the backup's reviewed publisher step: %+v", pair.plan.Steps)
	}
	firstAdvance := func() {
		t.Helper()
		t.Setenv("CELIKPANEL_SERVER_IP", "203.0.113.10")
		if _, err := pair.first.panel.advanceServerSetupExecution(pair.plan, pair.execution); err != nil {
			t.Fatal(err)
		}
	}
	secondReview := func(change func(*serverSetupDraft)) serverSetupPlan {
		t.Setenv("CELIKPANEL_SERVER_IP", "198.51.100.20")
		_, plan := pair.secondReview(t, change)
		return plan
	}
	if answer := pair.connect(t, issueJoinCode(t, pair.first).Code); answer.State != "ok" {
		t.Fatalf("join: %+v", answer)
	}
	plan := secondReview(func(d *serverSetupDraft) { d.DNSEngine = "bind" })
	step := serverSetupExecutionStep{serverSetupPlanStep: plan.Steps[0], RequestID: strings.Repeat("5", 32), OwnerID: strings.Repeat("6", 32)}
	if done, err := pair.second.panel.runServerSetupPeerNames(context.Background(), plan, step); !done || err != nil {
		t.Fatalf("names: %v %v", done, err)
	}
	joinIndex := pair.advanceToJoin(t)
	// The main's BIND rolled back; the owner chose PowerDNS there; the
	// first server's owner made a new code and the main joined again.
	changed := secondReview(func(d *serverSetupDraft) { d.DNSEngine = "pdns" })
	step.serverSetupPlanStep = changed.Steps[0]
	if _, err := pair.second.panel.runServerSetupPeerNames(context.Background(), changed, step); err == nil {
		t.Fatal("changed answers were taken into the earlier join")
	}
	code := issueJoinCode(t, pair.first)
	if _, err := pair.second.database.GetDB().Exec(`UPDATE server_setup_executions SET status='failed'`); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CELIKPANEL_SERVER_IP", "198.51.100.20")
	if answer := pair.connect(t, code.Code); answer.State != "ok" {
		t.Fatalf("join again: %+v", answer)
	}
	changed = secondReview(func(d *serverSetupDraft) { d.DNSEngine = "pdns" })
	step.serverSetupPlanStep, step.RequestID = changed.Steps[0], strings.Repeat("7", 32)
	if done, err := pair.second.panel.runServerSetupPeerNames(context.Background(), changed, step); !done || err != nil {
		t.Fatalf("new answers: %v %v", done, err)
	}
	firstAdvance()
	firstAdvance()
	rebound, err := pair.first.panel.readServerSetupPeerBinding(context.Background(), pair.plan, pair.execution.ID)
	if err != nil || rebound == nil || rebound.PeerDNSEngine != "pdns" || pair.execution.Steps[joinIndex].Status != "succeeded" {
		t.Fatalf("the run adopted the new join: %+v %v %+v", rebound, err, pair.execution.Error)
	}
	// The run reaches its publisher gate (the steps before it are done, as
	// in the VM run) and the publishing connection is ready.
	for index := range pair.execution.Steps {
		switch {
		case index < publisher:
			pair.execution.Steps[index].Status = "succeeded"
		case index == publisher:
			pair.execution.Steps[index].Status = "running"
		}
	}
	pair.execution.Status, pair.execution.Phase = "waiting", "dns_publisher"
	raw, _ := json.Marshal(pair.execution)
	db := pair.first.database.GetDB()
	if _, err := db.Exec(`UPDATE server_setup_executions SET status='waiting',execution_json=? WHERE id=?`, string(raw), pair.execution.ID); err != nil {
		t.Fatal(err)
	}
	effective, err := pair.first.panel.effectiveServerSetupPlan(context.Background(), pair.plan, pair.execution.ID)
	if err != nil || !effective.peerBound || effective.Draft.DNSPublisherEndpoint == "" {
		t.Fatalf("bound plan: %+v %v", effective.Draft, err)
	}
	connection := strings.Repeat("c", 32)
	if _, err := db.Exec(`INSERT INTO remote_dns_connections(id,endpoint,label,credential,enrollment_code,status,nameservers_json,created_at) VALUES(?,?,?,?,'','ready','[]','now')`, connection, effective.Draft.DNSPublisherEndpoint, "main", strings.Repeat("d", 64)); err != nil {
		t.Fatal(err)
	}
	if err := writeServerSetupJSONSetting(context.Background(), db, serverSetupDNSPublisherKey(pair.execution.ID), map[string]string{"execution_id": pair.execution.ID}); err != nil {
		t.Fatal(err)
	}
	if !serverSetupAtPublisherGate(effective, *pair.execution) {
		t.Fatal("the adopted plan is not at its publisher gate")
	}
	if err := pair.first.panel.commitServerSetupDNSPublisherDefault(context.Background(), effective, pair.execution.ID, connection); err != nil {
		t.Fatalf("the publisher gate refused the adopted plan: %v", err)
	}
	if mode, _ := pair.first.panel.setupDNSManagementMode(context.Background()); mode != setupDNSModeExisting {
		t.Fatalf("publishing default: %q", mode)
	}
	// Had a newer join replaced the binding under this run, the run would
	// stop with its own verified state, never the generic failure.
	if rejoined, err := pair.first.panel.serverSetupRejoinedUnderRun(context.Background(), pair.plan, pair.execution.ID); err != nil || rejoined {
		t.Fatalf("the adopted binding is current: %v %v", rejoined, err)
	}
	var join serverSetupJoinRecord
	if _, err := readServerSetupJSONSetting(context.Background(), db, serverSetupJoinKey, &join); err != nil {
		t.Fatal(err)
	}
	join.RequestID = strings.Repeat("9", 32)
	if err := writeServerSetupJSONSetting(context.Background(), db, serverSetupJoinKey, join); err != nil {
		t.Fatal(err)
	}
	if rejoined, err := pair.first.panel.serverSetupRejoinedUnderRun(context.Background(), pair.plan, pair.execution.ID); err != nil || !rejoined {
		t.Fatalf("a replaced join under the run: %v %v", rejoined, err)
	}
	failure := serverSetupFailureForStep(pair.execution.Steps[publisher], serverSetupPlanChangedByRejoin())
	if failure.Code != "server_setup_plan_changed_by_rejoin" || !strings.Contains(failure.Message, "Review a revised plan") {
		t.Fatalf("stop state: %+v", failure)
	}
}
