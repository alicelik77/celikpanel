package main

import (
	"context"
	"net"
	"net/http"
	"net/rpc"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

func TestDomainDeletionFinishDelaySchedule(t *testing.T) {
	want := map[int]time.Duration{1: 5 * time.Minute, 2: 15 * time.Minute, 3: time.Hour, 4: 6 * time.Hour, 9: 6 * time.Hour}
	for failures, delay := range want {
		if got := domainDeletionFinishDelay(failures); got != delay {
			t.Errorf("after %d failures: %s, want %s", failures, got, delay)
		}
	}
}

// certFinishAgent is the pending-deletion fake Agent plus the certificate
// removal the finish runs, failing while fail is set.
type certFinishAgent struct {
	*dnsZoneV3TestAgent
	fail  atomic.Bool
	calls atomic.Int32
}

func (a *certFinishAgent) DeleteCertLineage(_ *transport.DeleteCertLineageRequest, resp *transport.DeleteCertLineageResponse) error {
	a.calls.Add(1)
	if a.fail.Load() {
		resp.Error = "certbot could not remove the lineage"
		return nil
	}
	resp.Deleted = true
	return nil
}

func attachCertFinishAgent(t *testing.T, panel *Panel, agent *certFinishAgent) {
	t.Helper()
	server := rpc.NewServer()
	if err := server.RegisterName("Agent", agent); err != nil {
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
	rawClient, err := connector(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	panel.agentClient = transport.NewReconnectingClientWithContextConnector(rawClient, connector)
	t.Cleanup(func() { _ = rawClient.Close() })
}

// A finish that keeps failing after the DNS removal is confirmed: attempts
// after 5 min, 15 min, 1 h, then every 6 h; from the third failure the
// status names the stage, the error line and the retry, and one notice is
// raised for the run; status reads change nothing; a person's retry starts
// the backoff again, and the deletion finishes once the cause is gone.
func TestDomainDeletionFinishBacksOffNamesTheFailureNotifiesOnceAndRetryResets(t *testing.T) {
	f := newPendingDeletionFixture(t, "finish-fails.example")
	db := f.panel.db.GetDB()
	if _, err := db.Exec(`
		INSERT INTO ssl_certificates (
			domain_id, type, cert_path, key_path, chain_path, issuer, subject, issued_at, expires_at,
			auto_renew, secure_mail, status, lineage_name
		) VALUES (?, 'letsencrypt', '/certs/cert.pem', '/certs/key.pem', '', 'Test CA', ?, '2026-01-01T00:00:00Z', '2027-01-01T00:00:00Z', 1, 0, 'active', ?)`,
		f.domainID, f.domain, f.domain); err != nil {
		t.Fatal(err)
	}
	if err := f.panel.syncZoneToDNS(context.Background(), f.domain, false); err != nil {
		t.Fatalf("creation: %v", err)
	}
	agent := &certFinishAgent{dnsZoneV3TestAgent: f.agent}
	agent.fail.Store(true)
	attachCertFinishAgent(t, f.panel, agent)

	var mu sync.Mutex
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	saved := domainDeletionFinishNow
	domainDeletionFinishNow = func() time.Time { mu.Lock(); defer mu.Unlock(); return now }
	t.Cleanup(func() { domainDeletionFinishNow = saved })
	advance := func(d time.Duration) { mu.Lock(); now = now.Add(d); mu.Unlock() }
	record := func() (domainDeletionFinishRecord, bool) {
		return readDomainDeletionFinishRecord(context.Background(), db, f.domainID, f.domain)
	}
	reconcile := func() { f.panel.reconcileLocalDNSDeletionsOnce(context.Background()) }
	notices := func() int {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM account_notices WHERE kind = ?`, noticeDomainDeletionStuck).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	// The request itself stops at the certificates: failure 1.
	if code, answer := f.delete(t); code != http.StatusAccepted || answer.Stage != "certificate_cleanup" {
		t.Fatalf("delete = %d %+v", code, answer)
	}
	if r, ok := record(); !ok || r.Failures != 1 || r.Stage != "certificate_cleanup" {
		t.Fatalf("after the request: %+v %v", r, ok)
	}
	if _, body := readDeletionStatusForTest(t, f.panel, f.domainID); body.Reason != domainDeletionDNSRemovalConfirmedReason || !strings.Contains(body.Message, "Next attempt: 2026-11-07 12:05 UTC") {
		t.Fatalf("status after one failure = %+v", body)
	}
	// Not due yet: no attempt.
	reconcile()
	if agent.calls.Load() != 1 {
		t.Fatalf("attempted before its pause: %d calls", agent.calls.Load())
	}
	steps := []struct {
		wait     time.Duration
		failures int
	}{{5 * time.Minute, 2}, {15 * time.Minute, 3}, {time.Hour, 4}}
	for _, step := range steps {
		advance(step.wait - time.Second)
		reconcile()
		if r, _ := record(); r.Failures != step.failures-1 {
			t.Fatalf("attempted %s early: %+v", time.Second, r)
		}
		advance(time.Second)
		reconcile()
		if r, _ := record(); r.Failures != step.failures {
			t.Fatalf("after %s: %+v", step.wait, r)
		}
	}
	people := notices()
	if people == 0 {
		t.Fatal("no notice when the finish was clearly failing")
	}
	// Then every 6 hours, and no further notice.
	advance(6*time.Hour - time.Minute)
	reconcile()
	if r, _ := record(); r.Failures != 4 {
		t.Fatalf("attempted before 6 h: %+v", r)
	}
	advance(time.Minute)
	reconcile()
	if r, _ := record(); r.Failures != 5 || notices() != people {
		t.Fatalf("after 6 h: %+v notices %d->%d", r, people, notices())
	}

	// The status names the failure; reads change nothing.
	var before string
	_ = db.QueryRow(`SELECT value FROM panel_settings WHERE key = ?`, domainDeletionFinishKey(f.domainID)).Scan(&before)
	calls := agent.calls.Load()
	body := f.pollStatusChangesNothing(t)
	var after string
	_ = db.QueryRow(`SELECT value FROM panel_settings WHERE key = ?`, domainDeletionFinishKey(f.domainID)).Scan(&after)
	if body.Reason != domainDeletionFinishFailedReason || body.Stage != "certificate_cleanup" ||
		!strings.Contains(body.ErrorLine, "certbot") || !strings.Contains(body.Message, "Retry this deletion") ||
		before != after || agent.calls.Load() != calls {
		t.Fatalf("clearly failing status = %+v (record changed %v, calls %d->%d)", body, before != after, calls, agent.calls.Load())
	}

	// The person's retry runs again (still failing): the backoff restarts.
	if code, answer := f.delete(t); code != http.StatusAccepted || answer.Stage != "certificate_cleanup" {
		t.Fatalf("retry = %d %+v", code, answer)
	}
	if r, ok := record(); !ok || r.Failures != 1 || r.NoticeRaised {
		t.Fatalf("the retry did not restart the backoff: %+v", r)
	}
	// The cause is fixed: the next attempt finishes the deletion.
	agent.fail.Store(false)
	advance(5 * time.Minute)
	reconcile()
	if f.domainLeft(t) {
		t.Fatal("the deletion did not finish once the cause was gone")
	}
	if _, ok := record(); ok {
		t.Fatal("the finishing record outlived the deletion")
	}
}
