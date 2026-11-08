package main

import (
	"context"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/dbremote"
	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.130 lab (release recovery): the remote access loop takes no
// service-mutation lease at panel start.

func dbrStartWindow(t *testing.T, quiet time.Duration, hostQuiet bool) {
	t.Helper()
	previousQuiet, previousHost := databaseRemoteStartupQuiet, databaseRemoteHostQuiet
	databaseRemoteStartupQuiet = quiet
	databaseRemoteHostQuiet = func(*Panel, context.Context) bool { return hostQuiet }
	t.Cleanup(func() { databaseRemoteStartupQuiet, databaseRemoteHostQuiet = previousQuiet, previousHost })
}

func dbrStatus(t *testing.T, answer func(transport.DatabaseRemoteStatusRequest) transport.DatabaseRemoteResponse) {
	t.Helper()
	previous := databaseRemoteStatusFromAgent
	databaseRemoteStatusFromAgent = func(_ *Panel, _ context.Context, req transport.DatabaseRemoteStatusRequest) (transport.DatabaseRemoteResponse, error) {
		return answer(req), nil
	}
	t.Cleanup(func() { databaseRemoteStatusFromAgent = previous })
}

// runLoopBriefly starts the sync loop, lets its start-up run finish and
// stops it.
func runLoopBriefly(t *testing.T, p *Panel) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := p.startDatabaseRemoteSync(ctx)
	time.Sleep(300 * time.Millisecond)
	cancel()
	<-done
}

func (r *databaseRemoteRecorder) counts() (int, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.requests), len(r.narrows)
}

// Zero rules and nothing in the filter: nothing at all is sent at start,
// even when the loop would be allowed to mutate.
func TestDatabaseRemoteStartSendsNothingWithZeroRules(t *testing.T) {
	p := newDatabaseRemotePanel(t)
	recorder := &databaseRemoteRecorder{}
	recorder.install(t)
	dbrStartWindow(t, 0, true)
	dbrStatus(t, func(transport.DatabaseRemoteStatusRequest) transport.DatabaseRemoteResponse {
		return transport.DatabaseRemoteResponse{NFTAvailable: true}
	})
	runLoopBriefly(t, p)
	if applies, narrows := recorder.counts(); applies != 0 || narrows != 0 {
		t.Fatalf("start sent %d applies and %d narrowings with no rule", applies, narrows)
	}
}

func dbrSeededRule(t *testing.T, p *Panel) []transport.DatabaseRemoteTarget {
	t.Helper()
	dbrOffer(t, p, true)
	if _, err := p.db.GetDB().Exec(`INSERT INTO database_remote_rules (database_id, user_id, address) VALUES (7530, 7540, '203.0.113.7/32')`); err != nil {
		t.Fatal(err)
	}
	desired, err := p.databaseRemoteDesired(context.Background())
	if err != nil || len(desired) != 1 {
		t.Fatalf("desired = %+v, %v", desired, err)
	}
	return desired
}

func dbrRecordedStatus(desired []transport.DatabaseRemoteTarget, digest string) func(transport.DatabaseRemoteStatusRequest) transport.DatabaseRemoteResponse {
	return func(transport.DatabaseRemoteStatusRequest) transport.DatabaseRemoteResponse {
		return transport.DatabaseRemoteResponse{NFTAvailable: true, FilterLoaded: true, FilterPorts: []int{3306}, AppliedDigest: digest,
			Targets: []transport.DatabaseRemoteTargetResult{{Engine: "mysql", Port: 3306, State: transport.DatabaseRemoteStateOpen,
				Active: append([]transport.DatabaseRemoteRule{{Database: "a_shop", User: "a_user", Address: "198.51.100.1/32"}}, desired[0].Rules...)}}}
	}
}

// The Agent's record carries exactly the wanted set: the digest is seeded
// from the status, the ports in force are known, and nothing is sent.
func TestDatabaseRemoteDigestSeededFromStatus(t *testing.T) {
	p := newDatabaseRemotePanel(t)
	recorder := &databaseRemoteRecorder{}
	recorder.install(t)
	desired := dbrSeededRule(t, p)
	digest := dbremote.RequestDigest([]dbremote.Target{{Engine: "mysql", Port: 3306, Rules: fromTransportRemoteRules(desired[0].Rules)}}, false)
	dbrStatus(t, dbrRecordedStatus(desired, digest))
	if !p.seedDatabaseRemoteFromStatus(context.Background()) {
		t.Fatal("a matching record was not recognised")
	}
	if ports := databaseRemoteFilteredOpenPorts(); len(ports) != 1 || ports[0] != 3306 {
		t.Fatalf("open ports after seeding: %v", ports)
	}
	last, _, _, _ := databaseRemoteLast("mysql", 3306)
	if len(last.Active) != 1 || last.Active[0].Address != "203.0.113.7/32" {
		t.Fatalf("a rule that is no longer wanted counts as in force: %+v", last.Active)
	}
	if _, sent, err := p.applyDatabaseRemote(context.Background(), false); err != nil || sent {
		t.Fatalf("an unchanged set was sent again: %v %v", sent, err)
	}
	if applies, narrows := recorder.counts(); applies != 0 || narrows != 0 {
		t.Fatalf("sent %d applies, %d narrowings", applies, narrows)
	}
}

// The record differs (a rule changed while the panel was down): within the
// start-up window nothing takes a lease, the filter is narrowed (lease-free)
// and the full apply waits; after the window, on a quiet server, it runs.
func TestDatabaseRemoteStartWaitsForTheWindowButClosesAtOnce(t *testing.T) {
	p := newDatabaseRemotePanel(t)
	recorder := &databaseRemoteRecorder{}
	recorder.install(t)
	desired := dbrSeededRule(t, p)
	dbrStatus(t, dbrRecordedStatus(desired, "an older set"))
	dbrStartWindow(t, time.Hour, true)
	runLoopBriefly(t, p)
	if applies, narrows := recorder.counts(); applies != 0 || narrows != 1 {
		t.Fatalf("in the window: %d applies (want 0), %d narrowings (want 1)", applies, narrows)
	}

	// After the window but with the server busy: still no lease.
	databaseRemoteMemory = &databaseRemoteState{kick: make(chan struct{}, 1)}
	dbrStartWindow(t, 0, false)
	runLoopBriefly(t, p)
	if applies, _ := recorder.counts(); applies != 0 {
		t.Fatalf("a busy server got %d applies", applies)
	}

	// Quiet: the full apply runs.
	databaseRemoteMemory = &databaseRemoteState{kick: make(chan struct{}, 1)}
	dbrStartWindow(t, 0, true)
	runLoopBriefly(t, p)
	if applies, _ := recorder.counts(); applies != 1 {
		t.Fatalf("a quiet server after the window got %d applies", applies)
	}
}
