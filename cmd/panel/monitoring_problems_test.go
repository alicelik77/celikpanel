package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

func sustainSeries(now time.Time, minutes int, value func(i int) float64) []sustainRow {
	var rows []sustainRow
	count := minutes * 2
	for i := 0; i < count; i++ {
		v := value(i)
		rows = append(rows, sustainRow{at: now.Add(-time.Duration(count-1-i) * metricsInterval), cpu: v, memPercent: v, hasMemTotal: true})
	}
	return rows
}

func TestSustainedNeedsTheWholeTenMinutesAtOrAboveNinety(t *testing.T) {
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	cases := map[string]struct {
		rows []sustainRow
		want bool
	}{
		"twelve minutes at 95":   {sustainSeries(now, 12, func(int) float64 { return 95 }), true},
		"one dip to 40":          {sustainSeries(now, 12, func(i int) float64 { return map[bool]float64{true: 40, false: 95}[i == 20] }), false},
		"only five minutes":      {sustainSeries(now, 5, func(int) float64 { return 99 }), false},
		"a short burst at 100":   {sustainSeries(now, 12, func(i int) float64 { return map[bool]float64{true: 100, false: 3}[i > 20] }), false},
		"exactly ninety":         {sustainSeries(now, 11, func(int) float64 { return 90 }), true},
		"no rows":                {nil, false},
		"stopped writing 5m ago": {sustainSeries(now.Add(-5*time.Minute), 20, func(int) float64 { return 99 }), false},
	}
	for name, c := range cases {
		if got := sustainedAbove(c.rows, now, problemSustainedWindow, problemSustainedLevel, sustainCPU); got != c.want {
			t.Fatalf("%s: sustained = %v, want %v", name, got, c.want)
		}
	}
	// A gap inside the window is not guessed over.
	rows := sustainSeries(now, 12, func(int) float64 { return 95 })
	gapped := append(append([]sustainRow{}, rows[:8]...), rows[14:]...)
	if sustainedAbove(gapped, now, problemSustainedWindow, problemSustainedLevel, sustainCPU) {
		t.Fatal("a three-minute gap was treated as sustained load")
	}
	// Since: the first row of the stretch at or above the level.
	series := sustainSeries(now, 30, func(i int) float64 { return map[bool]float64{true: 95, false: 20}[i >= 30] })
	if since := sinceAbove(series, problemSustainedLevel, sustainCPU); !since.Equal(series[30].at) {
		t.Fatalf("since = %v, want %v", since, series[30].at)
	}
}

func TestSustainedProblemsReadOnlyIntervalAverages(t *testing.T) {
	_, db := metricsTestDB(t)
	now := time.Now().UTC().Truncate(time.Second)
	// Old snapshot rows (no cpu_peak) at 100 % prove nothing.
	insertTestSamples(t, db, now.Add(-12*time.Minute), 24, metricsInterval, func(int) float64 { return 100 }, false)
	problems, unknown := sustainedProblems(context.Background(), db, now)
	if len(problems) != 0 || unknown != nil {
		t.Fatalf("snapshot rows raised %+v %+v", problems, unknown)
	}
	if _, err := db.Exec(`DELETE FROM metrics_samples`); err != nil {
		t.Fatal(err)
	}
	insertTestSamples(t, db, now.Add(-12*time.Minute), 24, metricsInterval, func(int) float64 { return 97 }, true)
	if _, err := db.Exec(`UPDATE metrics_samples SET mem_used = 3000`); err != nil {
		t.Fatal(err)
	}
	problems, _ = sustainedProblems(context.Background(), db, now)
	if len(problems) != 1 || problems[0].Kind != "cpu_sustained" || problems[0].Value != 97 || problems[0].Since == "" ||
		problems[0].Actor != "server_owner" {
		t.Fatalf("cpu at 97 %% for 12 minutes: %+v", problems)
	}
	if _, err := db.Exec(`UPDATE metrics_samples SET mem_used = 3800`); err != nil {
		t.Fatal(err)
	}
	problems, _ = sustainedProblems(context.Background(), db, now)
	if len(problems) != 2 || problems[1].Kind != "memory_sustained" || problems[1].Value != 95 {
		t.Fatalf("memory at 95 %%: %+v", problems)
	}
}

func TestDiskProblemThresholds(t *testing.T) {
	now := time.Now()
	if problems, unknown := diskProblems(84, 100, now); len(problems) != 0 || unknown != nil {
		t.Fatalf("84 %%: %+v", problems)
	}
	if problems, _ := diskProblems(85, 100, now); len(problems) != 1 || problems[0].Severity != "warning" || problems[0].Action.To != "/domains" {
		t.Fatalf("85 %%: %+v", problems)
	}
	if problems, _ := diskProblems(95, 100, now); len(problems) != 1 || problems[0].Severity != "critical" {
		t.Fatalf("95 %%: %+v", problems)
	}
	if _, unknown := diskProblems(0, 0, now); unknown == nil || unknown.Source != "disk" {
		t.Fatal("an unreadable disk was not reported as not checked")
	}
}

func TestCertificateProblemsNameExpiringFailedAndExpired(t *testing.T) {
	p, _, certificateID := newSSLStateFixture(t)
	db := p.db.GetDB()
	now := time.Now().UTC()
	set := func(days int, renewal any) {
		t.Helper()
		if _, err := db.Exec(`UPDATE ssl_certificates SET expires_at = ?, renewal_status = ? WHERE id = ?`,
			now.Add(time.Duration(days)*24*time.Hour+time.Hour).Format(time.RFC3339), renewal, certificateID); err != nil {
			t.Fatal(err)
		}
	}
	cases := []struct {
		days     int
		renewal  any
		kind     string
		severity string
	}{
		{40, nil, "", ""},
		{10, nil, "certificate_expiring", "warning"},
		{2, nil, "certificate_expiring", "critical"},
		{40, "failed", "certificate_renewal_failed", "warning"},
		{40, nativeRenewalStatusPrefix + transport.SiteRenewalSchedulerMissing, "certificate_renewal_failed", "warning"},
		{40, nativeRenewalStatusPrefix + transport.SiteRenewalUnknown, "", ""},
		{-3, "failed", "certificate_expired", "critical"},
		{5, sslWaitingForOwner, "", ""},
	}
	for _, c := range cases {
		set(c.days, c.renewal)
		problems, unknown := certificateProblems(context.Background(), db, now)
		if unknown != nil {
			t.Fatalf("%+v: unknown %+v", c, unknown)
		}
		if c.kind == "" {
			if len(problems) != 0 {
				t.Fatalf("%+v raised %+v", c, problems)
			}
			continue
		}
		if len(problems) != 1 || problems[0].Kind != c.kind || problems[0].Severity != c.severity ||
			problems[0].Action.To != "/domains/ssl-state.example?tab=ssl" {
			t.Fatalf("%+v: %+v", c, problems)
		}
	}
}

func TestSecurityUpdateProblemsNeverClaimNoneWhenUnknown(t *testing.T) {
	known := func(security int, securityState string) osUpdatesSummaryView {
		return osUpdatesSummaryView{State: "ready", CheckedAt: "2026-11-05T10:00:00Z",
			Pending: &transport.OSUpdatesPending{State: "known", Total: 9, Security: security, SecurityState: securityState}}
	}
	if problems, unknown := securityUpdateProblems(known(3, "")); len(problems) != 1 || unknown != nil || problems[0].Value != 3 || problems[0].Action.To != "/security" {
		t.Fatalf("three security updates: %+v %+v", problems, unknown)
	}
	if problems, unknown := securityUpdateProblems(known(0, "")); len(problems) != 0 || unknown != nil {
		t.Fatal("none waiting raised something")
	}
	for name, view := range map[string]osUpdatesSummaryView{
		"checking":          {State: "checking"},
		"failed":            {State: "unknown"},
		"lock busy":         {State: "ready", Pending: &transport.OSUpdatesPending{State: "busy"}},
		"arch-audit absent": known(0, "arch_audit_missing"),
	} {
		if problems, unknown := securityUpdateProblems(view); len(problems) != 0 || unknown == nil {
			t.Fatalf("%s: %+v %+v", name, problems, unknown)
		}
	}
}

func TestMailQueueIsReadInTheBackgroundAndOnlyWherePostfixIsInstalled(t *testing.T) {
	restoreRead, restoreStart := readMailQueueSummary, startMailQueueRead
	mailQueueProblemReading = mailQueueReading{}
	t.Cleanup(func() {
		readMailQueueSummary, startMailQueueRead = restoreRead, restoreStart
		mailQueueProblemReading = mailQueueReading{}
	})
	reads := 0
	deferred := 120
	var failure error
	readMailQueueSummary = func(*Panel, context.Context) (core.PostfixQueueResult, error) {
		reads++
		return core.PostfixQueueResult{Installed: true, Summary: core.PostfixSummary{Deferred: deferred}}, failure
	}
	var pending func()
	startMailQueueRead = func(run func()) { pending = run }
	p := &Panel{}
	now := time.Now()

	if problems, unknown := p.mailQueueProblems([]serviceObservation{{ID: "nginx", IsInstalled: true}}, now); len(problems) != 0 || unknown != nil || pending != nil {
		t.Fatal("the queue was read where Postfix is not installed")
	}
	postfix := []serviceObservation{{ID: "postfix", IsInstalled: true}}
	if _, unknown := p.mailQueueProblems(postfix, now); unknown == nil || unknown.Reason != "checking" || pending == nil {
		t.Fatalf("first look: %+v", unknown)
	}
	// Polling while the read runs starts no second read.
	run := pending
	pending = nil
	p.mailQueueProblems(postfix, now)
	if pending != nil {
		t.Fatal("a second read started while one was running")
	}
	run()
	problems, unknown := p.mailQueueProblems(postfix, time.Now())
	if unknown != nil || len(problems) != 1 || problems[0].Value != 120 || problems[0].Action.To != "/components/postfix" {
		t.Fatalf("120 deferred: %+v %+v", problems, unknown)
	}
	if reads != 1 || pending != nil {
		t.Fatalf("a fresh reading was read again (%d reads)", reads)
	}
	// A failed first reading says unreadable, never "empty".
	mailQueueProblemReading = mailQueueReading{}
	failure = errors.New("agent unreachable")
	p.mailQueueProblems(postfix, time.Now())
	pending()
	if _, unknown := p.mailQueueProblems(postfix, time.Now()); unknown == nil || unknown.Reason != "unreadable" {
		t.Fatalf("failed read: %+v", unknown)
	}
	// 50 or fewer deferred is not a problem.
	mailQueueProblemReading = mailQueueReading{}
	failure, deferred = nil, 50
	p.mailQueueProblems(postfix, time.Now())
	pending()
	if problems, unknown := p.mailQueueProblems(postfix, time.Now()); len(problems) != 0 || unknown != nil {
		t.Fatalf("50 deferred: %+v %+v", problems, unknown)
	}
}

func TestServiceProblemsListOnlyStoppedDaemonsWithTheirUnit(t *testing.T) {
	observations := []serviceObservation{
		{ID: "nginx", IsInstalled: true, Status: "active (running)", Unit: "nginx"},
		{ID: "bind", IsInstalled: true, Status: "failed", Unit: "named"},
		{ID: "postfix", IsInstalled: false, Status: "inactive (dead)"},
	}
	problems, unknown := serviceProblems(observations, core.ManagedServiceHostProfile{PackageFamily: "apt"}, time.Now(), nil, time.Now())
	if unknown != nil {
		t.Fatalf("a fresh scan is not checked: %+v", unknown)
	}
	if len(problems) != 1 || problems[0].Subject != "bind" || problems[0].Unit != "named" ||
		problems[0].Action.Kind != "start_service" || problems[0].Severity != "critical" {
		t.Fatalf("problems = %+v", problems)
	}
}

func TestProblemsEndpointReportsWhatItCouldNotCheck(t *testing.T) {
	p, db := metricsTestDB(t)
	restoreDisk, restoreUpdates := monitoringReadDisk, monitoringSecurityUpdates
	t.Cleanup(func() { monitoringReadDisk, monitoringSecurityUpdates = restoreDisk, restoreUpdates })
	monitoringReadDisk = func(string) (uint64, uint64) { return 96, 100 }
	monitoringSecurityUpdates = func(*Panel) osUpdatesSummaryView {
		return osUpdatesSummaryView{State: "ready", Pending: &transport.OSUpdatesPending{State: "known", Security: 2}}
	}
	_ = db
	w := httptest.NewRecorder()
	p.handleMonitoringProblems(w, httptest.NewRequest(http.MethodGet, "/api/v1/metrics/problems", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
	var report monitoringProblemsReport
	if err := json.Unmarshal(w.Body.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Problems) != 2 || report.Problems[0].Kind != "disk_full" || report.Problems[0].Severity != "critical" ||
		report.Problems[1].Kind != "security_updates" {
		t.Fatalf("problems = %+v", report.Problems)
	}
	// No component scan has run on this fresh database.
	if len(report.Unknown) != 1 || report.Unknown[0].Source != "services" || report.Unknown[0].Reason != "not_scanned" {
		t.Fatalf("unknown = %+v", report.Unknown)
	}
	w = httptest.NewRecorder()
	p.handleMonitoringProblems(w, httptest.NewRequest(http.MethodPost, "/api/v1/metrics/problems", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("a POST was answered %d", w.Code)
	}
}
