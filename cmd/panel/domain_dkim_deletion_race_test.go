package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// D6 (final alpha.113 acceptance run 3): a DNS-only zone created and deleted
// at once. The automatic DKIM runner made the key, the deletion marked the
// domain and removed the key and the zone, and the runner's TXT then failed
// on the zone's foreign key and left a "failed" record of the deleted
// domain. The automatic work must stop quietly instead, leave nothing behind
// and never publish into a zone being deleted.

// deleteDomainForDKIMRace does to DKIM state what the deletion does, in its
// order: mark under the mail lock, remove the key and the automatic record
// under it, then (optionally) remove the zone.
func (f *dkimAutoFixture) deleteDomainForDKIMRace(t *testing.T, id int, name string, removeZone bool) {
	t.Helper()
	ctx := context.Background()
	f.p.mailMutationMu.Lock()
	if _, err := f.p.markDomainDeletionPending(ctx, id); err != nil {
		f.p.mailMutationMu.Unlock()
		t.Fatalf("mark deletion: %v", err)
	}
	f.p.removeDomainDKIMLocked(ctx, id, name)
	f.p.mailMutationMu.Unlock()
	if removeZone {
		if _, err := f.p.db.GetDB().Exec(`DELETE FROM pdns_domains WHERE name = ?`, name); err != nil {
			t.Fatalf("remove zone: %v", err)
		}
	}
}

func (f *dkimAutoFixture) automaticRecordCount(t *testing.T, id int) int {
	t.Helper()
	var left int
	if err := f.p.db.GetDB().QueryRow(`SELECT count(*) FROM panel_settings WHERE key = ?`, dkimAutomaticKey(id)).Scan(&left); err != nil {
		t.Fatal(err)
	}
	return left
}

func TestAutomaticDKIMStopsQuietlyWhenTheDomainIsDeletedDuringThePass(t *testing.T) {
	for _, removeZone := range []bool{true, false} {
		name := "gone.example"
		if !removeZone {
			// The deletion has marked the domain but not reached its DNS
			// step yet: the zone is still there.
			name = "marked.example"
		}
		t.Run(name, func(t *testing.T) {
			f := newDKIMAutoFixture(t)
			seedStrictDNSZone(t, f.p, name)
			id := f.domain(t, name, "local", 0, true)
			f.queue(t, id, name, dkimTriggerDomainCreated)
			// The deletion runs while the runner's signing step restarts
			// OpenDKIM: after the key was made, before the TXT is written.
			f.agent.onConfigure = func() { f.deleteDomainForDKIMRace(t, id, name, removeZone) }
			syncs := len(f.agent.syncCalls)
			if retry := f.pass(t); retry {
				t.Fatal("a deleted domain was left waiting")
			}
			if f.agent.ensureCalls != 1 || f.agent.removeCalls != 1 {
				t.Fatalf("ensure=%d remove=%d, want the key made once and removed by the deletion", f.agent.ensureCalls, f.agent.removeCalls)
			}
			if _, ok := f.agent.keys[name]; ok {
				t.Fatal("the key made in the race was left for the deleted domain")
			}
			if got := f.txt(t, "celik._domainkey."+name); len(got) != 0 {
				t.Fatalf("a TXT was written into a zone being deleted: %q", got)
			}
			if len(f.agent.syncCalls) != syncs {
				t.Fatalf("a zone being deleted was published: %v", f.agent.syncCalls[syncs:])
			}
			if f.automaticRecordCount(t, id) != 0 {
				t.Fatal("an automatic DKIM record was left for the deleted domain")
			}
		})
	}
}

// Once the deletion has marked the domain, the runner's key step makes no
// key (the existing lock order) and nothing is recorded.
func TestAutomaticDKIMMakesNoKeyForADomainMarkedBeforeThePass(t *testing.T) {
	f := newDKIMAutoFixture(t)
	seedStrictDNSZone(t, f.p, "early.example")
	id := f.domain(t, "early.example", "local", 0, true)
	f.queue(t, id, "early.example", dkimTriggerDomainCreated)
	f.deleteDomainForDKIMRace(t, id, "early.example", false)
	f.pass(t)
	if f.agent.ensureCalls != 0 || len(f.agent.syncCalls) != 0 {
		t.Fatalf("work started for a domain being deleted: ensure=%d syncs=%v", f.agent.ensureCalls, f.agent.syncCalls)
	}
	if f.automaticRecordCount(t, id) != 0 {
		t.Fatal("record kept")
	}
}

func TestAutomaticDKIMNeverRecordsStateForADeletedDomain(t *testing.T) {
	f := newDKIMAutoFixture(t)
	id := f.domain(t, "late.example", "external", 0, true)
	f.queue(t, id, "late.example", dkimTriggerDomainCreated)
	record, ok := f.record(t, id, "late.example")
	if !ok {
		t.Fatal("record not queued")
	}
	f.deleteDomainForDKIMRace(t, id, "late.example", false)
	// A late answer of the runner (here the acceptance's failure) must not
	// bring the record back.
	f.p.recordAutomaticDKIM(context.Background(), record, dkimAutoFailed, dkimReasonDNSPublish,
		"constraint failed: FOREIGN KEY constraint failed (787)")
	if f.automaticRecordCount(t, id) != 0 {
		t.Fatal("a failure was recorded for a domain being deleted")
	}
	if _, err := f.p.requestAutomaticDKIM(context.Background(), id, "late.example", dkimTriggerRetry, false); !errors.Is(err, errDKIMAutomaticDomainGone) {
		t.Fatalf("queueing work for a domain being deleted: %v", err)
	}
	if f.automaticRecordCount(t, id) != 0 {
		t.Fatal("work was queued for a domain being deleted")
	}
	// "Try again" on a domain being deleted is refused before it queues.
	if recorder := f.retry(t, id); recorder.Code == 202 {
		t.Fatalf("retry accepted for a domain being deleted: %s", recorder.Body.String())
	}
}

func TestAutomaticDKIMRemovesRecordsOfDeletedDomainsAtStart(t *testing.T) {
	f := newDKIMAutoFixture(t)
	live := f.domain(t, "live.example", "external", 0, true)
	marked := f.domain(t, "half.example", "external", 0, true)
	ctx := context.Background()
	// A failed record kept for a live domain stays (the card shows it).
	if err := f.p.writeDKIMAutomaticRecord(ctx, dkimAutomaticRecord{DomainID: live, Domain: "live.example",
		Trigger: dkimTriggerDomainCreated, State: dkimAutoFailed, Reason: dkimReasonKeyFailed}); err != nil {
		t.Fatal(err)
	}
	// Left by a release before this fix: a deleted domain's failed record,
	// one whose id now names no domain of that name, and one of a domain
	// whose deletion is pending.
	stale := func(id int, domain string) {
		raw, _ := json.Marshal(dkimAutomaticRecord{Version: dkimAutomaticVersion, DomainID: id, Domain: domain,
			Trigger: dkimTriggerDomainCreated, State: dkimAutoFailed, Reason: dkimReasonDNSPublish,
			ErrorLine: "constraint failed: FOREIGN KEY constraint failed (787)", RecordedAt: "2026-10-30T21:12:58Z"})
		if err := f.p.setSetting(ctx, dkimAutomaticKey(id), string(raw)); err != nil {
			t.Fatal(err)
		}
	}
	stale(9999, "r3gone113.test")
	stale(live, "renamed.example") // overwrites the live record with a foreign name
	stale(marked, "half.example")
	if _, err := f.p.db.GetDB().Exec(`INSERT INTO domain_deletion_operations (domain_id, previous_status) VALUES (?, 'active')`, marked); err != nil {
		t.Fatal(err)
	}
	other := f.domain(t, "kept.example", "external", 0, true)
	if err := f.p.writeDKIMAutomaticRecord(ctx, dkimAutomaticRecord{DomainID: other, Domain: "kept.example",
		Trigger: dkimTriggerDomainCreated, State: dkimAutoBlocked, Reason: dkimReasonDNSNotReady}); err != nil {
		t.Fatal(err)
	}
	if err := f.p.setSetting(ctx, dkimAutomaticKeyPrefix+"77", "not json"); err != nil {
		t.Fatal(err)
	}

	f.p.runAutomaticDKIMPass(ctx, true)

	rows, err := f.p.db.GetDB().Query(`SELECT key FROM panel_settings WHERE key LIKE ? ORDER BY key`, dkimAutomaticKeyPrefix+"%")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			t.Fatal(err)
		}
		keys = append(keys, key)
	}
	want := []string{dkimAutomaticKey(other), dkimAutomaticKeyPrefix + "77"} // "7104" sorts before "77"
	if strings.Join(keys, ",") != strings.Join(want, ",") {
		t.Fatalf("records after start = %v, want %v", keys, want)
	}
	if f.agent.ensureCalls != 0 {
		t.Fatal("start made a key")
	}
}
