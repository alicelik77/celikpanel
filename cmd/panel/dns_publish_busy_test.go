package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// E3 (alpha.114 lab): a customer's DNS record was saved, its publication hit
// "database is locked" while automatic DKIM wrote the same zone, and the
// customer saw HTTP 500 (a retry then said the record exists). A saved change
// now answers success with publication "publishing", no mutation is started
// from the request, and the reconciler publishes the zone by itself.
func TestDNSChangeSavedWhileTheDatabaseIsBusyIsPublishingThenPublishedByItself(t *testing.T) {
	panel, agent, _, scheduler := newPendingPublicationPanel(t, "busy.example")
	ctx := context.Background()
	pool := panel.db.GetDB()
	if _, err := pool.Exec(`
		INSERT INTO pdns_records (domain_id, name, type, content, ttl)
		SELECT id, 'api.busy.example', 'A', '192.0.2.44', 300 FROM pdns_domains WHERE name = 'busy.example'`); err != nil {
		t.Fatal(err)
	}

	// Another DNS writer holds the database for longer than busy_timeout.
	holder, err := beginDNSWriteTx(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	publication, ok := panel.publishDNSMutation(recorder, ctx, "busy.example")
	_ = holder.Rollback()
	if !ok || publication != dnsPublicationPublishing || recorder.Code != http.StatusOK || recorder.Body.Len() != 0 {
		t.Fatalf("busy publication: %q ok=%v status=%d body=%s", publication, ok, recorder.Code, recorder.Body.String())
	}
	if syncs, _ := pendingAgentCalls(agent); len(syncs) != 0 {
		t.Fatalf("a mutation was started while the database was busy: %d", len(syncs))
	}
	if leaseCount(t, panel) != 0 {
		t.Fatal("a lease was left by the rolled-back publication")
	}
	scheduler.mu.Lock()
	armed := scheduler.republishArmed
	scheduler.mu.Unlock()
	if !armed {
		t.Fatal("the reconciler was not asked to publish the saved change")
	}

	panel.republishHeldDNSZones(ctx)
	syncs, _ := pendingAgentCalls(agent)
	if len(syncs) != 1 || syncs[0].Domain != "busy.example" {
		t.Fatalf("republished: %+v", syncs)
	}
	found := false
	for _, record := range syncs[0].Records {
		found = found || (record.Name == "api.busy.example" && record.Content == "192.0.2.44")
	}
	if !found {
		t.Fatalf("the saved record was not published: %+v", syncs[0].Records)
	}
	state, err := readDNSZoneSyncState(ctx, pool, "busy.example")
	if err != nil || state.Status != "applied" || state.AppliedGeneration != state.DesiredGeneration {
		t.Fatalf("state=%+v err=%v", state, err)
	}
}
