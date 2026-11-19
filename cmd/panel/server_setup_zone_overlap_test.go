package main

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// alpha.140: the setup review's zone-wide rule (unchanged) refuses a main
// domain whose zone has another hosted zone inside it or around it. The
// review and the peer join now say that cause in its own words: it is not a
// record clash and not about the second server's names (the lab primary
// hosted customer zones such as h4.dr.test under dr.test).
func TestServerSetupZoneOverlapIsSaidAsItsOwnCause(t *testing.T) {
	pair, binding := bindJoinedPair(t)
	ctx := context.Background()
	f := pair.first
	publishJoinedZone(t, f, pair.plan.ID, binding.InfrastructureDNS.Records)
	if _, err := f.database.GetDB().Exec(`UPDATE server_setup_executions SET status='failed' WHERE id=?`, pair.execution.ID); err != nil {
		t.Fatal(err)
	}
	f.panel.releaseFailedServerSetupDraft(ctx, pair.plan)
	state, err := f.panel.loadServerSetup(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Control: without another zone the plan can start.
	review, err := f.panel.buildServerSetupPlan(ctx, state, serviceOperationActor{UserID: f.userID})
	if err != nil || !review.CanStart {
		t.Fatalf("control: can_start=%v blockers=%v err=%v", review.CanStart, review.Blockers, err)
	}

	// A customer's domain under the main domain gets its own zone.
	if _, err := f.database.GetDB().Exec(`INSERT INTO pdns_domains(name,type) VALUES('shop.example.test','MASTER')`); err != nil {
		t.Fatal(err)
	}
	review, err = f.panel.buildServerSetupPlan(ctx, state, serviceOperationActor{UserID: f.userID})
	if err != nil {
		t.Fatal(err)
	}
	overlap := ""
	for _, blocker := range review.Blockers {
		if strings.HasPrefix(blocker, "server_setup_infrastructure_dns_zone_overlap:") {
			overlap = blocker
		}
	}
	if review.CanStart || overlap == "" || infrastructureConflict(review.Blockers) != "" {
		t.Fatalf("the overlap is not its own blocker: can_start=%v blockers=%v", review.CanStart, review.Blockers)
	}

	// The run's peer join says the same cause, not "change the second
	// server's names".
	executionID := strings.Repeat("9", 32)
	if _, err := f.database.GetDB().Exec(`INSERT INTO server_setup_executions(id,request_id,plan_id,status,execution_json,created_at,updated_at) VALUES(?,?,?,'running','{}','now','now')`,
		executionID, executionID, pair.plan.ID); err != nil { // a blocked plan is not stored; the run row needs a stored one
		t.Fatal(err)
	}
	done, err := f.panel.runServerSetupPeerJoin(ctx, review, executionID)
	var wait *serverSetupPeerWait
	if done || !errors.As(err, &wait) || wait.Code != "server_setup_peer_zone_overlap" {
		t.Fatalf("peer join: done=%v err=%v", done, err)
	}
	if strings.Contains(wait.Message, "Change them") || !strings.Contains(wait.Message, "example.test") || !strings.Contains(wait.Message, "Who acts: this server's administrator") {
		t.Fatalf("peer join sentence: %s", wait.Message)
	}

	// Control: a record clash at a name stays the conflict it was.
	if _, err := f.database.GetDB().Exec(`DELETE FROM pdns_domains WHERE name='shop.example.test'`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.database.GetDB().Exec(`UPDATE pdns_records SET content='198.51.100.99' WHERE name='ns2.example.test' AND type='A'`); err != nil {
		t.Fatal(err)
	}
	review, err = f.panel.buildServerSetupPlan(ctx, state, serviceOperationActor{UserID: f.userID})
	if err != nil {
		t.Fatal(err)
	}
	if conflict := infrastructureConflict(review.Blockers); conflict == "" || strings.Contains(strings.Join(review.Blockers, " "), "zone_overlap") {
		t.Fatalf("a record clash is no longer a conflict: %v", review.Blockers)
	}
}
