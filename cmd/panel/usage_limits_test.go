package main

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Plan limits made real (alpha.118, D-044), on the suspension fixture:
// admin 9701, reseller 9702 (subscription 9801, domain 9901), its customer
// 9703 (subscription 9802, domain 9902 with a site), admin-managed customer
// 9704 (subscription 9803).

type fakeTrafficAgent struct {
	epoch   string
	batches []transport.TrafficBatch
	calls   int
}

// collect behaves like the Agent: confirmed batches are dropped, the rest
// is answered.
func (f *fakeTrafficAgent) collect(_ context.Context, req transport.TrafficCollectRequest) (transport.TrafficCollectResponse, error) {
	f.calls++
	if req.Epoch == f.epoch {
		kept := f.batches[:0]
		for _, b := range f.batches {
			if b.Seq > req.AfterSeq {
				kept = append(kept, b)
			}
		}
		f.batches = kept
	}
	resp := transport.TrafficCollectResponse{Epoch: f.epoch}
	for _, b := range f.batches {
		if req.Epoch != f.epoch || b.Seq > req.AfterSeq {
			resp.Batches = append(resp.Batches, b)
		}
	}
	for _, d := range req.Domains {
		resp.Sources = append(resp.Sources, transport.TrafficSourceState{DomainID: d.DomainID, State: transport.TrafficSourceMeasuring})
	}
	return resp, nil
}

type usageFixture struct {
	suspensionFixture
	traffic  *fakeTrafficAgent
	rendered map[int][]bool // domain -> traffic limited at each render
	now      time.Time
}

func newUsageFixture(t *testing.T) *usageFixture {
	t.Helper()
	f := &usageFixture{suspensionFixture: newSuspensionFixture(t), traffic: &fakeTrafficAgent{epoch: "e1"}, rendered: map[int][]bool{}}
	f.now = time.Date(2026, 11, 15, 12, 0, 0, 0, time.Local)
	previous := usageNow
	usageNow = func() time.Time { return f.now }
	t.Cleanup(func() { usageNow = previous })
	f.panel.trafficCollector = f.traffic
	f.panel.trafficRender = func(ctx context.Context, domainID int) error {
		var sub int
		if err := f.panel.db.GetDB().QueryRowContext(ctx, `SELECT subscription_id FROM domains WHERE id = ?`, domainID).Scan(&sub); err != nil {
			return err
		}
		limited, err := subscriptionTrafficLimited(ctx, f.panel.db.GetDB(), sub)
		f.rendered[domainID] = append(f.rendered[domainID], limited)
		return err
	}
	f.panel.diskMeasurer = &fakeDiskMeasurer{bytes: map[int]int64{}}
	f.panel.mailQuota = func(context.Context) (transport.MailQuotaReconcileResponse, error) {
		return transport.MailQuotaReconcileResponse{State: transport.MailQuotaEnforced}, nil
	}
	// The reseller's own domain gets a site too.
	f.exec(t, `INSERT INTO sites (id, domain_id, document_root) VALUES (9931, 9901, '/srv/r')`)
	f.exec(t, `UPDATE subscriptions SET bandwidth_quota_mb = 0, disk_quota_mb = 0`)
	return f
}

func (f *usageFixture) addTraffic(seq int64, domainID int, day string, bytes int64) {
	f.traffic.batches = append(f.traffic.batches, transport.TrafficBatch{Seq: seq, Items: []transport.TrafficDayBytes{{DomainID: domainID, Day: day, BytesOut: bytes, Requests: 1}}})
}

func (f *usageFixture) month(t *testing.T, sub int) int64 {
	t.Helper()
	used, err := subscriptionTrafficMonth(context.Background(), f.panel.db.GetDB(), sub, f.now)
	if err != nil {
		t.Fatal(err)
	}
	return used
}

func (f *usageFixture) notices(t *testing.T, kind string) []int {
	t.Helper()
	rows, err := f.panel.db.GetDB().Query(`SELECT user_id FROM usage_notices WHERE kind = ? ORDER BY user_id`, kind)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var users []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		users = append(users, id)
	}
	return users
}

func sameInts(a []int, b ...int) bool {
	if len(a) != len(b) {
		return false
	}
	sort.Ints(a)
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestTrafficBatchesAreCountedOnceAcrossLostAnswersAndANewAgentStore(t *testing.T) {
	f := newUsageFixture(t)
	ctx := context.Background()
	f.addTraffic(1, 9902, "2026-11-15", 1000)
	f.addTraffic(2, 9902, "2026-11-16", 500)
	if err := f.panel.collectTraffic(ctx); err != nil {
		t.Fatal(err)
	}
	if got := f.month(t, 9802); got != 1500 {
		t.Fatalf("month = %d, want 1500", got)
	}
	// The Panel committed but its next request was lost: the same answer is
	// committed again and changes nothing.
	resp := transport.TrafficCollectResponse{Epoch: "e1", Batches: []transport.TrafficBatch{
		{Seq: 1, Items: []transport.TrafficDayBytes{{DomainID: 9902, Day: "2026-11-15", BytesOut: 1000}}},
		{Seq: 2, Items: []transport.TrafficDayBytes{{DomainID: 9902, Day: "2026-11-16", BytesOut: 500}}},
	}}
	if err := f.panel.commitTrafficBatches(ctx, "e1", 2, resp); err != nil {
		t.Fatal(err)
	}
	if got := f.month(t, 9802); got != 1500 {
		t.Fatalf("a repeated batch was counted twice: %d", got)
	}
	// The Agent's store is new (reinstalled): its sequence starts again.
	f.traffic.epoch = "e2"
	f.traffic.batches = nil
	f.addTraffic(1, 9902, "2026-11-17", 7)
	if err := f.panel.collectTraffic(ctx); err != nil {
		t.Fatal(err)
	}
	if got := f.month(t, 9802); got != 1507 {
		t.Fatalf("after a new store: %d", got)
	}
	// Last month's bytes do not count this month; a deleted site's are dropped.
	f.addTraffic(2, 9902, "2026-10-31", 99999)
	f.addTraffic(3, 424242, "2026-11-17", 5)
	if err := f.panel.collectTraffic(ctx); err != nil {
		t.Fatal(err)
	}
	if got := f.month(t, 9802); got != 1507 {
		t.Fatalf("other month counted: %d", got)
	}
	var state string
	if err := f.panel.db.GetDB().QueryRow(`SELECT state FROM traffic_sources WHERE domain_id = 9902`).Scan(&state); err != nil || state != "measuring" {
		t.Fatalf("source state = %q, %v", state, err)
	}
}

func TestTrafficLimitClosesOnlyTheWebsitesAndReopensByItself(t *testing.T) {
	f := newUsageFixture(t)
	ctx := context.Background()
	f.exec(t, `UPDATE subscriptions SET bandwidth_quota_mb = 1 WHERE id = 9802`)
	f.addTraffic(1, 9902, "2026-11-10", 900*1024)
	f.panel.runUsageRound(ctx)
	if users := f.notices(t, noticeTraffic80); !sameInts(users, 9701, 9702, 9703) {
		t.Fatalf("80%% notices to %v", users)
	}
	if limited, _ := subscriptionTrafficLimited(ctx, f.panel.db.GetDB(), 9802); limited {
		t.Fatal("blocked at 80%")
	}

	f.addTraffic(2, 9902, "2026-11-11", 200*1024)
	f.panel.runUsageRound(ctx)
	if limited, _ := subscriptionTrafficLimited(ctx, f.panel.db.GetDB(), 9802); !limited {
		t.Fatal("not blocked at 100%")
	}
	if got := f.rendered[9902]; len(got) != 1 || !got[0] {
		t.Fatalf("renders = %v", got)
	}
	if users := f.notices(t, noticeTraffic100); !sameInts(users, 9701, 9702, 9703) {
		t.Fatalf("100%% notices to %v", users)
	}
	// Web only: the subscription is not suspended, mail and databases work.
	if f.status(t, 9802) != "active" || f.count(t, `SELECT COUNT(*) FROM suspension_operations WHERE subscription_id = 9802`) != 0 {
		t.Fatal("the traffic limit suspended the whole subscription")
	}
	// Another round neither renders again nor sends another notice.
	f.panel.runUsageRound(ctx)
	if len(f.rendered[9902]) != 1 || len(f.notices(t, noticeTraffic100)) != 3 {
		t.Fatal("a second round repeated the change")
	}

	// The limit is raised: the websites open within one round.
	f.exec(t, `UPDATE subscriptions SET bandwidth_quota_mb = 10 WHERE id = 9802`)
	f.panel.runUsageRound(ctx)
	if got := f.rendered[9902]; len(got) != 2 || got[1] {
		t.Fatalf("renders after raise = %v", got)
	}
	if f.count(t, `SELECT COUNT(*) FROM traffic_web_blocks`) != 0 {
		t.Fatal("a released block stayed")
	}

	// Full again, then the 1st of the month opens the websites.
	f.exec(t, `UPDATE subscriptions SET bandwidth_quota_mb = 1 WHERE id = 9802`)
	f.panel.runUsageRound(ctx)
	if limited, _ := subscriptionTrafficLimited(ctx, f.panel.db.GetDB(), 9802); !limited {
		t.Fatal("not blocked again")
	}
	f.now = time.Date(2026, 12, 1, 0, 5, 0, 0, time.Local)
	f.panel.runUsageRound(ctx)
	if limited, _ := subscriptionTrafficLimited(ctx, f.panel.db.GetDB(), 9802); limited {
		t.Fatal("still blocked in the new month")
	}
	if got := f.rendered[9902]; got[len(got)-1] {
		t.Fatalf("the last render still shows the page: %v", got)
	}
}

func TestResellerTrafficPoolLimitsTheRealTotalOfItsCustomers(t *testing.T) {
	f := newUsageFixture(t)
	ctx := context.Background()
	// The reseller hands out more than its pool; only real use counts (K2).
	f.exec(t, `INSERT INTO reseller_pools (reseller_id, traffic_quota_mb) VALUES (9702, 2)`)
	f.exec(t, `UPDATE subscriptions SET bandwidth_quota_mb = 5 WHERE id IN (9801, 9802)`)
	f.addTraffic(1, 9901, "2026-11-10", 900*1024)
	f.addTraffic(2, 9902, "2026-11-10", 900*1024)
	f.panel.runUsageRound(ctx)
	if users := f.notices(t, noticePoolTraffic80); !sameInts(users, 9701, 9702) {
		t.Fatalf("pool 80%% notices to %v", users)
	}
	f.addTraffic(3, 9902, "2026-11-11", 300*1024)
	f.panel.runUsageRound(ctx)
	for _, sub := range []int{9801, 9802} {
		if limited, _ := subscriptionTrafficLimited(ctx, f.panel.db.GetDB(), sub); !limited {
			t.Fatalf("subscription %d not blocked by the full pool", sub)
		}
	}
	var cause string
	if err := f.panel.db.GetDB().QueryRow(`SELECT cause FROM traffic_web_blocks WHERE subscription_id = 9802`).Scan(&cause); err != nil || cause != trafficCausePool {
		t.Fatalf("cause = %q, %v", cause, err)
	}
	if users := f.notices(t, noticeTraffic100); !sameInts(users, 9703) {
		t.Fatalf("customer notices = %v", users)
	}
	// A subscription outside the tree is not touched.
	if limited, _ := subscriptionTrafficLimited(ctx, f.panel.db.GetDB(), 9803); limited {
		t.Fatal("a subscription outside the pool was blocked")
	}
	// The pool is raised: everything opens.
	f.exec(t, `UPDATE reseller_pools SET traffic_quota_mb = 100 WHERE reseller_id = 9702`)
	f.panel.runUsageRound(ctx)
	if f.count(t, `SELECT COUNT(*) FROM traffic_web_blocks`) != 0 {
		t.Fatal("the raised pool left blocks")
	}
}

type fakeDiskMeasurer struct {
	bytes map[int]int64
	err   error
}

func (m *fakeDiskMeasurer) measure(_ context.Context, req transport.SubscriptionDiskUsageRequest) (transport.SubscriptionDiskUsageResponse, error) {
	if m.err != nil {
		return transport.SubscriptionDiskUsageResponse{}, m.err
	}
	return transport.SubscriptionDiskUsageResponse{WebBytes: m.bytes[req.SubscriptionID], Method: "walk", FreePercent: 50}, nil
}

func TestDiskOverTheLimitForSevenDaysSuspendsAndResumesWithoutUndoingOtherCauses(t *testing.T) {
	f := newUsageFixture(t)
	ctx := context.Background()
	disk := &fakeDiskMeasurer{bytes: map[int]int64{9802: 3 * mib}}
	f.panel.diskMeasurer = disk
	f.exec(t, `UPDATE subscriptions SET disk_quota_mb = 2 WHERE id = 9802`)
	f.panel.runUsageRound(ctx)

	// Day 0: full. Nothing new, no upload; the sites keep running.
	if f.status(t, 9802) != "active" {
		t.Fatal("suspended at once")
	}
	var exceeded *quotaExceededError
	if err := f.panel.checkSubscriptionQuota(ctx, 9802, quotaMail); !errors.As(err, &exceeded) || exceeded.Kind != quotaDisk {
		t.Fatalf("a mailbox was allowed on a full disk: %v", err)
	}
	if blocked, _ := subscriptionUploadsBlocked(ctx, f.panel.db.GetDB(), 9802); !blocked {
		t.Fatal("uploads are not blocked")
	}
	if users := f.notices(t, noticeDisk100); !sameInts(users, 9701, 9702, 9703) {
		t.Fatalf("disk 100%% notices to %v", users)
	}

	// Day 5: the countdown notice; still running.
	f.now = f.now.Add(5*24*time.Hour + time.Hour)
	f.panel.runUsageRound(ctx)
	if f.status(t, 9802) != "active" || len(f.notices(t, noticeDiskCountdown)) != 3 {
		t.Fatalf("day 5: status %s, countdown %v", f.status(t, 9802), f.notices(t, noticeDiskCountdown))
	}

	// Day 7: suspended with the disk cause.
	f.now = f.now.Add(2 * 24 * time.Hour)
	f.panel.runUsageRound(ctx)
	f.run(t)
	if f.status(t, 9802) != "suspended" {
		t.Fatal("not suspended after 7 days")
	}
	row, err := loadSuspensionRow(ctx, f.panel.db.GetDB(), 9802)
	if err != nil || !row.Causes.Disk || row.Causes.Manual {
		t.Fatalf("causes = %+v, %v", row.Causes, err)
	}
	var reason string
	_ = f.panel.db.GetDB().QueryRow(`SELECT COALESCE(suspend_reason, '') FROM subscriptions WHERE id = 9802`).Scan(&reason)
	if reason != suspendReasonDisk {
		t.Fatalf("suspend_reason = %q", reason)
	}

	// The administrator also suspends it by hand. Usage then goes under the
	// limit: the disk cause clears, the manual one stays.
	wantStatus(t, f.as(t, 9701, http.MethodPost, "/api/v1/subscriptions/9802/suspension", `{"action":"suspend","reason":"unpaid"}`), http.StatusAccepted, "")
	f.run(t)
	disk.bytes[9802] = mib
	f.panel.requestDiskMeasurement(ctx, 9802)
	f.now = f.now.Add(time.Minute)
	f.panel.runUsageRound(ctx)
	f.run(t)
	row, _ = loadSuspensionRow(ctx, f.panel.db.GetDB(), 9802)
	if row.Causes.Disk || !row.Causes.Manual || f.status(t, 9802) != "suspended" {
		t.Fatalf("after going under: causes %+v status %s", row.Causes, f.status(t, 9802))
	}
	if blocked, _ := subscriptionUploadsBlocked(ctx, f.panel.db.GetDB(), 9802); blocked {
		t.Fatal("uploads still blocked under the limit")
	}
	wantStatus(t, f.as(t, 9701, http.MethodPost, "/api/v1/subscriptions/9802/suspension", `{"action":"resume"}`), http.StatusAccepted, "")
	f.run(t)
	if f.status(t, 9802) != "active" {
		t.Fatal("not resumed")
	}
}

func TestOperatorResumeOfADiskSuspensionGivesSevenMoreDays(t *testing.T) {
	f := newUsageFixture(t)
	ctx := context.Background()
	f.panel.diskMeasurer = &fakeDiskMeasurer{bytes: map[int]int64{9802: 3 * mib}}
	f.exec(t, `UPDATE subscriptions SET disk_quota_mb = 2 WHERE id = 9802`)
	f.panel.runUsageRound(ctx)
	f.now = f.now.Add(7*24*time.Hour + time.Hour)
	f.panel.runUsageRound(ctx)
	f.run(t)
	if f.status(t, 9802) != "suspended" {
		t.Fatal("not suspended")
	}
	v := f.view(t, 9802)
	if !v.CanResume || !v.Causes.Disk {
		t.Fatalf("view = %+v", v)
	}
	// The resume is the operator's decision; the countdown starts again.
	restartAt := f.now
	wantStatus(t, f.as(t, 9701, http.MethodPost, "/api/v1/subscriptions/9802/suspension", `{"action":"resume"}`), http.StatusAccepted, "")
	f.run(t)
	f.now = restartAt.Add(time.Hour)
	f.panel.runUsageRound(ctx)
	f.run(t)
	if f.status(t, 9802) != "active" {
		t.Fatal("suspended again at once after the operator's resume")
	}
	f.now = restartAt.Add(7*24*time.Hour + time.Hour)
	f.panel.runUsageRound(ctx)
	f.run(t)
	if f.status(t, 9802) != "suspended" {
		t.Fatal("the new 7-day period did not end in a suspension")
	}
}

func TestDiskMeasurementFailureIsUnknownNotEmpty(t *testing.T) {
	f := newUsageFixture(t)
	ctx := context.Background()
	f.panel.diskMeasurer = &fakeDiskMeasurer{err: errors.New("agent unreachable")}
	f.exec(t, `UPDATE subscriptions SET disk_quota_mb = 2 WHERE id = 9802`)
	f.panel.runUsageRound(ctx)
	view, err := loadSubscriptionDiskView(ctx, f.panel.db.GetDB(), 9802)
	if err != nil || view.State != "unknown" || view.Detail == "" {
		t.Fatalf("view = %+v, %v", view, err)
	}
	if blocked, _ := subscriptionUploadsBlocked(ctx, f.panel.db.GetDB(), 9802); blocked {
		t.Fatal("an unknown measurement blocked uploads")
	}
}

func TestUsageNoticesAreReadOnlyByTheirRecipient(t *testing.T) {
	f := newUsageFixture(t)
	ctx := context.Background()
	f.exec(t, `UPDATE subscriptions SET bandwidth_quota_mb = 1 WHERE id = 9802`)
	f.addTraffic(1, 9902, "2026-11-10", 2*mib)
	f.panel.runUsageRound(ctx)
	mux := http.NewServeMux()
	mux.HandleFunc(usageNoticesPath, f.panel.handleUsageNotices)
	mux.HandleFunc(usageNoticesPath+"/", f.panel.handleUsageNotices)
	f.handler = f.panel.requireAuth(mux)
	rec := f.as(t, 9703, http.MethodGet, usageNoticesPath, "")
	wantStatus(t, rec, http.StatusOK, "")
	var id int
	if err := f.panel.db.GetDB().QueryRow(`SELECT id FROM usage_notices WHERE user_id = 9703 AND kind = ?`, noticeTraffic100).Scan(&id); err != nil {
		t.Fatal(err)
	}
	wantStatus(t, f.as(t, 9704, http.MethodPost, usageNoticesPath+"/"+strconv.Itoa(id)+"/seen", ""), http.StatusNotFound, "")
	wantStatus(t, f.as(t, 9703, http.MethodPost, usageNoticesPath+"/"+strconv.Itoa(id)+"/seen", ""), http.StatusOK, "")
}

func TestFullDiskClosesFTPSFTPAndPHPUploadsAndOpensThemAgain(t *testing.T) {
	f := newUsageFixture(t)
	ctx := context.Background()
	disk := &fakeDiskMeasurer{bytes: map[int]int64{9802: 3 * mib}}
	f.panel.diskMeasurer = disk
	f.exec(t, `UPDATE subscriptions SET disk_quota_mb = 2 WHERE id = 9802`)
	type call struct {
		sub     int
		blocked bool
	}
	var calls []call
	fail := true
	f.panel.uploadBlockNative = func(_ context.Context, sub int, blocked bool) (string, error) {
		calls = append(calls, call{sub, blocked})
		if fail {
			return "", errors.New("agent unreachable")
		}
		return "ssh_open", nil
	}
	f.panel.runUsageRound(ctx)
	view, _ := loadSubscriptionDiskView(ctx, f.panel.db.GetDB(), 9802)
	if len(calls) != 1 || !calls[0].blocked || view.UploadsApplied || !strings.HasPrefix(view.UploadsNote, "error:") {
		t.Fatalf("first round: calls %v view %+v", calls, view)
	}
	// The failure is retried next round, then the state holds.
	fail = false
	f.panel.runUsageRound(ctx)
	view, _ = loadSubscriptionDiskView(ctx, f.panel.db.GetDB(), 9802)
	if len(calls) != 2 || !view.UploadsApplied || view.UploadsNote != "ssh_open" {
		t.Fatalf("retry: calls %v view %+v", calls, view)
	}
	f.panel.runUsageRound(ctx)
	if len(calls) != 2 {
		t.Fatalf("an applied state was applied again: %v", calls)
	}
	// Under the limit: uploads open again by themselves.
	disk.bytes[9802] = mib
	f.panel.requestDiskMeasurement(ctx, 9802)
	f.panel.runUsageRound(ctx)
	if len(calls) != 3 || calls[2].blocked {
		t.Fatalf("not reopened: %v", calls)
	}
}
