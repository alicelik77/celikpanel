package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestBusyPercentIsTheShareOfNonIdleTime(t *testing.T) {
	cases := []struct {
		prev, cur cpuTimes
		want      float64
		ok        bool
	}{
		{cpuTimes{idle: 100, total: 200}, cpuTimes{idle: 150, total: 300}, 50, true},
		{cpuTimes{idle: 100, total: 200}, cpuTimes{idle: 200, total: 300}, 0, true},
		{cpuTimes{idle: 100, total: 200}, cpuTimes{idle: 100, total: 300}, 100, true},
		{cpuTimes{idle: 0, total: 0}, cpuTimes{idle: 2, total: 3}, 33.3, true},
		// No time passed, or the counters went back: nothing is invented.
		{cpuTimes{idle: 100, total: 200}, cpuTimes{idle: 100, total: 200}, 0, false},
		{cpuTimes{idle: 100, total: 200}, cpuTimes{idle: 10, total: 50}, 0, false},
	}
	for _, c := range cases {
		got, ok := busyPercent(c.prev, c.cur)
		if got != c.want || ok != c.ok {
			t.Fatalf("busyPercent(%v, %v) = %v, %v; want %v, %v", c.prev, c.cur, got, ok, c.want, c.ok)
		}
	}
}

// The watchdog case from the audit: an idle 2-vCPU machine with one
// one-second burst at 100 % inside a 30 s interval. The stored figure is the
// interval average (about 3 %), the burst survives only as the short peak.
func TestIntervalAverageIsNotAliasedByAShortBurstAndKeepsItAsShortPeak(t *testing.T) {
	s := &cpuIntervalSampler{}
	at := time.Unix(1_800_000_000, 0)
	// 2 vCPU at 100 Hz: 200 jiffies a second.
	cur := cpuTimes{idle: 10_000, total: 20_000}
	s.observe(cur, at)
	if _, _, ok := s.closeInterval(cur); ok {
		t.Fatal("the very first interval wrote an average with nothing to compare")
	}
	for second := 1; second <= 30; second++ {
		busy := uint64(2) // 1 % of 200
		if second == 15 {
			busy = 200 // both vCPUs busy for one second
		}
		cur = cpuTimes{idle: cur.idle + 200 - busy, total: cur.total + 200}
		s.observe(cur, at.Add(time.Duration(second)*time.Second))
	}
	average, peak, ok := s.closeInterval(cur)
	if !ok || peak == nil {
		t.Fatalf("no average: %v %v", ok, peak)
	}
	if average < 3 || average > 4.5 {
		t.Fatalf("interval average = %v, want about 4 %% (one busy second in thirty)", average)
	}
	if *peak != 100 {
		t.Fatalf("short peak = %v, want 100", *peak)
	}
	if value, ok := s.live(at.Add(31*time.Second), 3*time.Second); !ok || value != 1 {
		t.Fatalf("live value = %v %v, want the latest one-second value 1", value, ok)
	}
	if _, ok := s.live(at.Add(time.Minute), 3*time.Second); ok {
		t.Fatal("a stale live value was used")
	}
	// The next interval starts where this one ended, with a fresh peak.
	cur = cpuTimes{idle: cur.idle + 198, total: cur.total + 200}
	s.observe(cur, at.Add(31*time.Second))
	average, peak, ok = s.closeInterval(cur)
	if !ok || average != 1 || peak == nil || *peak != 1 {
		t.Fatalf("next interval = %v %v %v", average, peak, ok)
	}
}

func metricsTestDB(t *testing.T) (*Panel, *sql.DB) {
	t.Helper()
	p := newDNSPanelForTest(t)
	return p, p.db.GetDB()
}

func insertTestSamples(t *testing.T, db *sql.DB, from time.Time, count int, step time.Duration, cpu func(i int) float64, withPeak bool) {
	t.Helper()
	for i := 0; i < count; i++ {
		var peak *float64
		if withPeak {
			value := cpu(i) + 5
			peak = &value
		}
		if err := insertMetricsSample(context.Background(), db, metricsRow{
			at: from.Add(time.Duration(i) * step), cpu: cpu(i), cpuPeak: peak,
			memUsed: 1000 + int64(i), memTotal: 4000, diskUsed: 50, diskTotal: 100, load1: 0.5,
		}); err != nil {
			t.Fatal(err)
		}
	}
}

func countRows(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// The range and prune bug: rows are RFC 3339 ("…T…Z") and were compared
// with datetime('now', …) ("… …"), which sorts before them, so a whole extra
// day was returned and kept. The cutoff is now the same text form.
func TestPruneAndRangesCompareTheSameTimeText(t *testing.T) {
	_, db := metricsTestDB(t)
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	// Every 30 minutes for 72 hours.
	insertTestSamples(t, db, now.Add(-72*time.Hour), 144, 30*time.Minute, func(int) float64 { return 10 }, true)

	points, err := readMetricsHistory(context.Background(), db, metricsRanges[2], now)
	if err != nil {
		t.Fatal(err)
	}
	if len(points) == 0 || points[0].TS < metricsTime(now.Add(-24*time.Hour-4*time.Minute)) {
		t.Fatalf("24h answered from %v, before the range", points[0].TS)
	}
	if len(points) != 48 {
		t.Fatalf("24h held %d half-hour samples, want 48", len(points))
	}

	if err := pruneMetrics(context.Background(), db, now); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, db, `SELECT COUNT(*) FROM metrics_samples WHERE ts < ?`, metricsTime(now.Add(-48*time.Hour))); n != 0 {
		t.Fatalf("%d samples older than 48 hours survived the prune", n)
	}
	if n := countRows(t, db, `SELECT COUNT(*) FROM metrics_samples`); n != 96 {
		t.Fatalf("prune kept %d samples, want the 96 of the last 48 hours", n)
	}
}

func TestRollupsAreIdempotentCatchUpAndAreKeptSevenAndThirtyDays(t *testing.T) {
	_, db := metricsTestDB(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 5, 12, 2, 0, 0, time.UTC)
	// Two hours of 30 s samples; CPU 0 in the first half of each 5-minute
	// bucket and 20 in the second half.
	start := now.Add(-2 * time.Hour)
	insertTestSamples(t, db, start, 240, metricsInterval, func(i int) float64 {
		if i%10 < 5 {
			return 0
		}
		return 20
	}, true)
	for run := 0; run < 2; run++ {
		if err := rollUpMetrics(ctx, db, now); err != nil {
			t.Fatal(err)
		}
	}
	// 10:02 → 12:00 holds complete five-minute buckets 10:05 … 11:55 and the
	// partial 10:00 one; 12:00 is in progress and not written yet.
	if n := countRows(t, db, `SELECT COUNT(*) FROM metrics_rollups WHERE resolution = 300`); n != 24 {
		t.Fatalf("%d five-minute buckets, want 24", n)
	}
	if n := countRows(t, db, `SELECT COUNT(*) FROM metrics_rollups WHERE resolution = 300 AND bucket = ?`, "2026-11-05T12:00:00Z"); n != 0 {
		t.Fatal("the bucket in progress was written")
	}
	var samples int
	var avg, maxCPU, peak float64
	if err := db.QueryRow(`SELECT samples, cpu_avg, cpu_max, cpu_peak FROM metrics_rollups WHERE resolution = 300 AND bucket = '2026-11-05T11:00:00Z'`).
		Scan(&samples, &avg, &maxCPU, &peak); err != nil {
		t.Fatal(err)
	}
	if samples != 10 || avg != 10 || maxCPU != 20 || peak != 25 {
		t.Fatalf("bucket = samples %d avg %v max %v peak %v; want 10, 10, 20, 25", samples, avg, maxCPU, peak)
	}
	// One-hour buckets: 10:00 (partial) and 11:00; 12:00 is in progress.
	if n := countRows(t, db, `SELECT COUNT(*) FROM metrics_rollups WHERE resolution = 3600`); n != 2 {
		t.Fatalf("%d hour buckets, want 2", n)
	}
	// Later the run catches up from its newest bucket.
	later := now.Add(10 * time.Minute)
	insertTestSamples(t, db, now, 20, metricsInterval, func(int) float64 { return 50 }, true)
	if err := rollUpMetrics(ctx, db, later); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, db, `SELECT COUNT(*) FROM metrics_rollups WHERE resolution = 300`); n != 26 {
		t.Fatalf("catch-up wrote %d five-minute buckets in all, want 26", n)
	}

	// Retention: five-minute buckets 7 days, hour buckets 30 days.
	for _, row := range []struct {
		resolution int
		at         time.Time
	}{
		{300, now.Add(-8 * 24 * time.Hour)}, {300, now.Add(-6 * 24 * time.Hour)},
		{3600, now.Add(-31 * 24 * time.Hour)}, {3600, now.Add(-29 * 24 * time.Hour)},
	} {
		if _, err := db.Exec(`INSERT INTO metrics_rollups VALUES (?, ?, 1, 1, 1, NULL, 1, 1, 1, 1, 1, 1, 1)`,
			row.resolution, metricsTime(floorTime(row.at, int64(row.resolution)))); err != nil {
			t.Fatal(err)
		}
	}
	if err := pruneMetrics(ctx, db, now); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, db, `SELECT COUNT(*) FROM metrics_rollups WHERE resolution = 300 AND bucket < ?`, metricsTime(now.Add(-7*24*time.Hour))); n != 0 {
		t.Fatal("a five-minute bucket older than 7 days survived")
	}
	if n := countRows(t, db, `SELECT COUNT(*) FROM metrics_rollups WHERE bucket = ?`, metricsTime(floorTime(now.Add(-6*24*time.Hour), 300))); n != 1 {
		t.Fatal("a six-day-old five-minute bucket was pruned")
	}
	if n := countRows(t, db, `SELECT COUNT(*) FROM metrics_rollups WHERE resolution = 3600 AND bucket < ?`, metricsTime(now.Add(-30*24*time.Hour-2*time.Hour))); n != 0 {
		t.Fatal("an hour bucket older than 30 days survived")
	}
	if n := countRows(t, db, `SELECT COUNT(*) FROM metrics_rollups WHERE bucket = ?`, metricsTime(floorTime(now.Add(-29*24*time.Hour), 3600))); n != 1 {
		t.Fatal("a 29-day-old hour bucket was pruned")
	}
}

func TestEveryRangeAnswersAtMost360PointsWithAverageAndMaximum(t *testing.T) {
	_, db := metricsTestDB(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	// 48 hours of raw samples: CPU alternating 10 and 30.
	insertTestSamples(t, db, now.Add(-48*time.Hour), 5760, metricsInterval, func(i int) float64 {
		return float64(10 + 20*(i%2))
	}, true)
	// 30 days of hour buckets and 7 days of five-minute buckets.
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for at := floorTime(now.Add(-30*24*time.Hour), 3600); at.Before(now.Add(-48 * time.Hour)); at = at.Add(time.Hour) {
		if _, err := tx.Exec(`INSERT INTO metrics_rollups VALUES (3600, ?, 120, 20, 30, 80, 1, 2, 4, 1, 9, 0.1, 0.2)`, metricsTime(at)); err != nil {
			t.Fatal(err)
		}
	}
	for at := floorTime(now.Add(-7*24*time.Hour), 300); at.Before(now.Add(-48 * time.Hour)); at = at.Add(5 * time.Minute) {
		if _, err := tx.Exec(`INSERT INTO metrics_rollups VALUES (300, ?, 10, 20, 30, 80, 1, 2, 4, 1, 9, 0.1, 0.2)`, metricsTime(at)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	for _, rng := range metricsRanges {
		points, err := readMetricsHistory(ctx, db, rng, now)
		if err != nil {
			t.Fatal(err)
		}
		if len(points) == 0 || len(points) > metricsMaxPoints {
			t.Fatalf("%s answered %d points", rng.Key, len(points))
		}
		last := points[len(points)-1]
		// The chart reaches the present: the raw tail after the newest
		// rollup is part of every range.
		if last.TS < metricsTime(now.Add(-time.Duration(metricsStep(rng))*time.Second-time.Minute)) {
			t.Fatalf("%s ends at %s", rng.Key, last.TS)
		}
		for _, point := range points {
			if point.CPUMax < point.CPU || point.CPUPeak == nil || *point.CPUPeak < point.CPUMax {
				t.Fatalf("%s point %+v: max below average or peak missing", rng.Key, point)
			}
		}
		if rng.Key == "24h" && (last.CPU != 20 || last.CPUMax != 30 || *last.CPUPeak != 35) {
			t.Fatalf("24h bucket = avg %v max %v peak %v; want 20, 30, 35", last.CPU, last.CPUMax, *last.CPUPeak)
		}
		if rng.Key == "30d" && len(points) < 300 {
			t.Fatalf("30d answered only %d points", len(points))
		}
	}
}

func TestMetricsStepIsAWholeNumberOfSourceRows(t *testing.T) {
	want := map[string]int64{"1h": 30, "6h": 60, "24h": 240, "7d": 1800, "30d": 7200}
	for _, rng := range metricsRanges {
		if got := metricsStep(rng); got != want[rng.Key] {
			t.Fatalf("%s step = %d, want %d", rng.Key, got, want[rng.Key])
		}
	}
	for query, key := range map[string]string{
		"range=7d": "7d", "range=30d": "30d", "range=nonsense": "24h", "": "24h",
		"hours=1": "1h", "hours=6": "6h", "hours=48": "7d", "hours=2000": "30d",
	} {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/metrics/history?"+query, nil)
		if got := metricsRangeFor(r).Key; got != key {
			t.Fatalf("%q → %s, want %s", query, got, key)
		}
	}
}

func TestMetricsHistoryHandlerNamesRangeVCPUsAndLatest(t *testing.T) {
	p, db := metricsTestDB(t)
	insertTestSamples(t, db, time.Now().Add(-10*time.Minute), 20, metricsInterval, func(int) float64 { return 12 }, true)
	w := httptest.NewRecorder()
	p.handleMetricsHistory(w, httptest.NewRequest(http.MethodGet, "/api/v1/metrics/history?range=1h", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
	var body struct {
		Range   string         `json:"range"`
		Ranges  []string       `json:"ranges"`
		VCPUs   int            `json:"vcpus"`
		Samples []metricsPoint `json:"samples"`
		Latest  *metricsPoint  `json:"latest"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Range != "1h" || len(body.Ranges) != 5 || len(body.Samples) != 20 || body.Latest == nil || body.Latest.CPU != 12 {
		t.Fatalf("unexpected answer: %+v", body)
	}
	if body.Latest.CPUPeak == nil || *body.Latest.CPUPeak != 17 {
		t.Fatalf("latest short peak = %v", body.Latest.CPUPeak)
	}
}
