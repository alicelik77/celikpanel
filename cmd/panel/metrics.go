package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"runtime"
	"sort"
	"strconv"
	"sync"
	"time"
)

// Server monitoring history (operator request, 23 Jul). The sampler reuses
// the exact same readers the dashboard's health strip trusts — one source of
// measurement, two time horizons.
//
// alpha.126 ("Monitoring tells the truth", owner 5 Nov 2026):
//   - The CPU figure of a sample is the AVERAGE over the 30 seconds since the
//     previous sample (two /proc/stat readings 30 s apart, no sleep). Until
//     alpha.125 it was one 200 ms snapshot, which aliased onto short periodic
//     jobs (the release watchdog) and drew an idle machine as 50–100 % spikes.
//   - A separate SHORT PEAK is kept: the highest one-second value inside the
//     interval, read from the same /proc/stat line once a second. It is shown
//     labelled "short peak", never as the main line.
//   - Ranges compare against the same RFC 3339 text the rows are written in
//     (the old datetime('now', …) text sorted before it and returned up to a
//     day too much, and the prune kept 48–72 hours instead of 48).
//   - History: raw samples 48 hours, five-minute rollups 7 days, one-hour
//     rollups 30 days (migration 064). Every answer is downsampled on the
//     server to at most 360 points, each with its average and its maximum.
//
// Sunucu izleme geçmişi. alpha.126: örnekteki CPU, önceki örnekten bu yana
// 30 saniyenin ORTALAMASIDIR (uykusuz iki /proc/stat okuması); ayrı bir KISA
// TEPE (aralıktaki en yüksek bir saniyelik değer) "kısa tepe" etiketiyle
// tutulur. Aralıklar satırların yazıldığı RFC 3339 metniyle karşılaştırılır.
// Geçmiş: ham örnek 48 saat, beş dakikalık özet 7 gün, saatlik özet 30 gün;
// her yanıt sunucuda en çok 360 noktaya (ortalama ve en yüksek) indirilir.

const (
	metricsInterval = 30 * time.Second
	metricsTick     = time.Second
	// How often completed buckets are rolled up and old rows pruned.
	metricsMaintainEvery = 5 * time.Minute

	metricsRawRetention        = 48 * time.Hour
	metricsFiveMinuteRetention = 7*24*time.Hour + time.Hour
	metricsHourRetention       = 30*24*time.Hour + 2*time.Hour

	metricsFiveMinute = 300
	metricsHour       = 3600

	metricsMaxPoints = 360
)

// metricsTimeLayout is the one text form of every stored time: RFC 3339 UTC
// without fractions, so text order is time order.
const metricsTimeLayout = "2006-01-02T15:04:05Z"

func metricsTime(t time.Time) string { return t.UTC().Format(metricsTimeLayout) }

// cpuTimes is the aggregate "cpu" line of /proc/stat: idle (idle + iowait)
// and total jiffies.
type cpuTimes struct {
	idle, total uint64
}

// busyPercent is the share of non-idle time between two readings, 0–100 with
// one decimal. ok is false when no time passed or the counters went back
// (a reset), so nothing is invented.
func busyPercent(prev, cur cpuTimes) (float64, bool) {
	if cur.total <= prev.total || cur.idle < prev.idle {
		return 0, false
	}
	totalDelta := float64(cur.total - prev.total)
	idleDelta := float64(cur.idle - prev.idle)
	percent := (1 - idleDelta/totalDelta) * 100
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	return float64(int(percent*10+0.5)) / 10, true
}

// cpuIntervalSampler turns once-a-second /proc/stat readings into the
// interval average and the interval's short peak. It is safe for the
// sampler goroutine and the live-stats handler at once.
type cpuIntervalSampler struct {
	mu        sync.Mutex
	start     cpuTimes
	haveStart bool
	last      cpuTimes
	haveLast  bool
	peak      float64
	havePeak  bool
	latest    float64
	latestAt  time.Time
}

// observe takes one reading. The one-second value against the previous
// reading updates the interval's peak and the latest live value.
func (s *cpuIntervalSampler) observe(cur cpuTimes, at time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.haveStart {
		s.start, s.haveStart = cur, true
	}
	if s.haveLast {
		if value, ok := busyPercent(s.last, cur); ok {
			if !s.havePeak || value > s.peak {
				s.peak, s.havePeak = value, true
			}
			s.latest, s.latestAt = value, at
		}
	}
	s.last, s.haveLast = cur, true
}

// closeInterval ends the interval at cur: the average since the interval
// began and its short peak. The next interval begins at cur. ok is false
// for the very first interval (nothing to compare with) or after a counter
// reset; the caller then writes no sample rather than a made-up one.
func (s *cpuIntervalSampler) closeInterval(cur cpuTimes) (average float64, peak *float64, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.haveStart {
		average, ok = busyPercent(s.start, cur)
	}
	if ok && s.havePeak {
		value := s.peak
		// A short peak is never below the average it belongs to (the
		// one-second readings may miss the interval's first second).
		if value < average {
			value = average
		}
		peak = &value
	}
	s.start, s.haveStart = cur, true
	s.peak, s.havePeak = 0, false
	return average, peak, ok
}

// live returns the latest one-second value when it is at most maxAge old.
func (s *cpuIntervalSampler) live(now time.Time, maxAge time.Duration) (float64, bool) {
	if s == nil {
		return 0, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.latestAt.IsZero() || now.Sub(s.latestAt) > maxAge || now.Before(s.latestAt) {
		return 0, false
	}
	return s.latest, true
}

// liveCPU is the running panel's sampler; the dashboard's live figure reads
// it instead of sleeping 200 ms in every request.
var (
	liveCPUMu sync.Mutex
	liveCPU   *cpuIntervalSampler
)

func currentLiveCPU() *cpuIntervalSampler {
	liveCPUMu.Lock()
	defer liveCPUMu.Unlock()
	return liveCPU
}

// startMetricsSampler runs for the panel's lifetime. Sampling must never
// fight a user request: one /proc/stat line a second, one INSERT every 30 s,
// and the rollup and prune every five minutes.
// startMetricsSampler panel yaşadıkça koşar: saniyede bir /proc/stat satırı,
// 30 saniyede bir INSERT, beş dakikada bir özet ve budama.
func (p *Panel) startMetricsSampler() {
	sampler := &cpuIntervalSampler{}
	liveCPUMu.Lock()
	liveCPU = sampler
	liveCPUMu.Unlock()
	go func() {
		ticker := time.NewTicker(metricsTick)
		defer ticker.Stop()
		if idle, total, ok := readCPUTimes(); ok {
			sampler.observe(cpuTimes{idle, total}, time.Now())
		}
		// The first maintenance runs at start: it rolls up what an older
		// release left before the corrected prune removes it.
		p.maintainMetrics(context.Background(), time.Now())
		nextSample := time.Now().Add(metricsInterval)
		nextMaintain := time.Now().Add(metricsMaintainEvery)
		for now := range ticker.C {
			idle, total, ok := readCPUTimes()
			if !ok {
				continue
			}
			current := cpuTimes{idle, total}
			sampler.observe(current, now)
			if now.Before(nextSample) {
				continue
			}
			nextSample = now.Add(metricsInterval)
			average, peak, ok := sampler.closeInterval(current)
			if ok {
				p.writeMetricsSample(context.Background(), now, average, peak)
			}
			if !now.Before(nextMaintain) {
				nextMaintain = now.Add(metricsMaintainEvery)
				p.maintainMetrics(context.Background(), now)
			}
		}
	}()
}

func (p *Panel) writeMetricsSample(ctx context.Context, at time.Time, cpu float64, cpuPeak *float64) {
	memUsed, memTotal := readMemory()
	diskUsed, diskTotal := readDisk("/")
	load1 := 0.0
	if l := readLoadAvg(); len(l) > 0 {
		load1 = l[0]
	}
	if err := insertMetricsSample(ctx, p.db.GetDB(), metricsRow{
		at: at, cpu: cpu, cpuPeak: cpuPeak,
		memUsed: int64(memUsed), memTotal: int64(memTotal),
		diskUsed: int64(diskUsed), diskTotal: int64(diskTotal), load1: load1,
	}); err != nil {
		log.Printf("metrics sample: %v", err)
	}
}

type metricsRow struct {
	at                  time.Time
	cpu                 float64
	cpuPeak             *float64
	memUsed, memTotal   int64
	diskUsed, diskTotal int64
	load1               float64
}

func insertMetricsSample(ctx context.Context, db *sql.DB, row metricsRow) error {
	var peak any
	if row.cpuPeak != nil {
		peak = *row.cpuPeak
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO metrics_samples (ts, cpu, cpu_peak, mem_used, mem_total, disk_used, disk_total, load1)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		metricsTime(row.at), row.cpu, peak, row.memUsed, row.memTotal, row.diskUsed, row.diskTotal, row.load1)
	return err
}

func (p *Panel) maintainMetrics(ctx context.Context, now time.Time) {
	if err := rollUpMetrics(ctx, p.db.GetDB(), now); err != nil {
		log.Printf("metrics rollup: %v", err)
		// The prune waits for a rollup that worked, so no history is lost
		// to a failed one.
		return
	}
	if err := pruneMetrics(ctx, p.db.GetDB(), now); err != nil {
		log.Printf("metrics prune: %v", err)
	}
}

// rollUpMetrics writes every completed five-minute and one-hour bucket from
// the raw samples, from the newest bucket already written (rewritten, so a
// repeat is harmless) up to the bucket still in progress, which is left for
// the next run. After a stop the next run catches up on what the raw
// samples still hold.
func rollUpMetrics(ctx context.Context, db *sql.DB, now time.Time) error {
	for _, resolution := range []int64{metricsFiveMinute, metricsHour} {
		var newest sql.NullString
		if err := db.QueryRowContext(ctx,
			`SELECT MAX(bucket) FROM metrics_rollups WHERE resolution = ?`, resolution).Scan(&newest); err != nil {
			return fmt.Errorf("read newest %ds bucket: %w", resolution, err)
		}
		from := ""
		if newest.Valid {
			from = newest.String
		}
		until := metricsTime(floorTime(now, resolution))
		bucket := fmt.Sprintf(`strftime('%%Y-%%m-%%dT%%H:%%M:%%SZ', (CAST(strftime('%%s', ts) AS INTEGER) / %d) * %d, 'unixepoch')`, resolution, resolution)
		if _, err := db.ExecContext(ctx, `
			INSERT OR REPLACE INTO metrics_rollups (
				resolution, bucket, samples, cpu_avg, cpu_max, cpu_peak,
				mem_used_avg, mem_used_max, mem_total, disk_used, disk_total, load1_avg, load1_max)
			SELECT ?, `+bucket+`, COUNT(*), ROUND(AVG(cpu), 1), MAX(cpu), MAX(cpu_peak),
				CAST(AVG(mem_used) AS INTEGER), MAX(mem_used), MAX(mem_total), MAX(disk_used), MAX(disk_total),
				ROUND(AVG(load1), 2), MAX(load1)
			FROM metrics_samples
			WHERE ts >= ? AND ts < ?
			GROUP BY 2`, resolution, from, until); err != nil {
			return fmt.Errorf("write %ds buckets: %w", resolution, err)
		}
	}
	return nil
}

// pruneMetrics removes what is older than each kind's retention, comparing
// the same text the rows were written in.
func pruneMetrics(ctx context.Context, db *sql.DB, now time.Time) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM metrics_samples WHERE ts < ?`,
		metricsTime(now.Add(-metricsRawRetention))); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM metrics_rollups WHERE resolution = ? AND bucket < ?`,
		metricsFiveMinute, metricsTime(now.Add(-metricsFiveMinuteRetention))); err != nil {
		return err
	}
	_, err := db.ExecContext(ctx, `DELETE FROM metrics_rollups WHERE resolution = ? AND bucket < ?`,
		metricsHour, metricsTime(now.Add(-metricsHourRetention)))
	return err
}

func floorTime(t time.Time, seconds int64) time.Time {
	unix := t.Unix()
	return time.Unix(unix-unix%seconds, 0).UTC()
}

// metricsRange is one choice on the Monitoring page.
type metricsRange struct {
	Key      string
	Duration time.Duration
	// Source: 0 raw samples, otherwise the rollup resolution in seconds.
	Source int64
}

var metricsRanges = []metricsRange{
	{"1h", time.Hour, 0},
	{"6h", 6 * time.Hour, 0},
	{"24h", 24 * time.Hour, 0},
	{"7d", 7 * 24 * time.Hour, metricsFiveMinute},
	{"30d", 30 * 24 * time.Hour, metricsHour},
}

// metricsRangeFor reads ?range=, or the older ?hours= of a page loaded
// before the update; anything else is 24 hours.
func metricsRangeFor(r *http.Request) metricsRange {
	key := r.URL.Query().Get("range")
	for _, candidate := range metricsRanges {
		if candidate.Key == key {
			return candidate
		}
	}
	if hours, err := strconv.Atoi(r.URL.Query().Get("hours")); err == nil && hours > 0 {
		for _, candidate := range metricsRanges {
			if time.Duration(hours)*time.Hour <= candidate.Duration {
				return candidate
			}
		}
		return metricsRanges[len(metricsRanges)-1]
	}
	return metricsRanges[2]
}

// metricsStep is the bucket width: the range over 360, rounded up to a whole
// number of the source's own resolution, so every bucket holds the same
// number of source rows.
func metricsStep(rng metricsRange) int64 {
	source := int64(metricsInterval / time.Second)
	if rng.Source > 0 {
		source = rng.Source
	}
	step := (int64(rng.Duration/time.Second) + metricsMaxPoints - 1) / metricsMaxPoints
	return (step + source - 1) / source * source
}

// metricsPoint is one point of a chart: the average and the highest value of
// the bucket. CPUPeak is the highest one-second value (the "short peak"),
// absent where only older snapshot rows were recorded.
type metricsPoint struct {
	TS         string   `json:"ts"`
	CPU        float64  `json:"cpu"`
	CPUMax     float64  `json:"cpu_max"`
	CPUPeak    *float64 `json:"cpu_peak,omitempty"`
	MemUsed    int64    `json:"mem_used"`
	MemUsedMax int64    `json:"mem_used_max"`
	MemTotal   int64    `json:"mem_total"`
	DiskUsed   int64    `json:"disk_used"`
	DiskTotal  int64    `json:"disk_total"`
	Load1      float64  `json:"load1"`
	Load1Max   float64  `json:"load1_max"`
}

// metricsInput is one source row: a raw sample (weight 1) or a rollup
// bucket (weight = its sample count).
type metricsInput struct {
	at                       time.Time
	weight                   int64
	cpuAvg, cpuMax           float64
	cpuPeak                  sql.NullFloat64
	memAvg, memMax, memTotal int64
	diskUsed, diskTotal      int64
	loadAvg, loadMax         float64
}

// downsampleMetrics groups rows into buckets of step seconds aligned to the
// epoch: weighted averages, maxima of maxima, the latest totals. Rows must
// be in time order; the result is too.
func downsampleMetrics(rows []metricsInput, step int64) []metricsPoint {
	type acc struct {
		start                   int64
		weight                  int64
		cpuSum, memSum, loadSum float64
		point                   metricsPoint
	}
	var buckets []*acc
	for _, row := range rows {
		start := row.at.Unix() - row.at.Unix()%step
		if len(buckets) == 0 || buckets[len(buckets)-1].start != start {
			buckets = append(buckets, &acc{start: start})
		}
		b := buckets[len(buckets)-1]
		weight := row.weight
		if weight < 1 {
			weight = 1
		}
		b.weight += weight
		b.cpuSum += row.cpuAvg * float64(weight)
		b.memSum += float64(row.memAvg) * float64(weight)
		b.loadSum += row.loadAvg * float64(weight)
		pt := &b.point
		pt.CPUMax = max(pt.CPUMax, row.cpuMax)
		if row.cpuPeak.Valid && (pt.CPUPeak == nil || row.cpuPeak.Float64 > *pt.CPUPeak) {
			value := row.cpuPeak.Float64
			pt.CPUPeak = &value
		}
		pt.MemUsedMax = max(pt.MemUsedMax, row.memMax)
		pt.MemTotal = row.memTotal
		pt.DiskUsed = row.diskUsed
		pt.DiskTotal = row.diskTotal
		pt.Load1Max = max(pt.Load1Max, row.loadMax)
	}
	points := make([]metricsPoint, 0, len(buckets))
	for _, b := range buckets {
		pt := b.point
		pt.TS = metricsTime(time.Unix(b.start, 0))
		w := float64(b.weight)
		pt.CPU = round1(b.cpuSum / w)
		pt.MemUsed = int64(b.memSum/w + 0.5)
		pt.Load1 = float64(int(b.loadSum/w*100+0.5)) / 100
		points = append(points, pt)
	}
	return points
}

func round1(v float64) float64 { return float64(int(v*10+0.5)) / 10 }

// readMetricsHistory answers one range, at most metricsMaxPoints points.
// A rollup range also takes the raw samples after its newest bucket, so the
// chart reaches the present.
func readMetricsHistory(ctx context.Context, db *sql.DB, rng metricsRange, now time.Time) ([]metricsPoint, error) {
	step := metricsStep(rng)
	from := floorTime(now.Add(-rng.Duration), step)
	var rows []metricsInput
	rawFrom := metricsTime(from)
	if rng.Source > 0 {
		result, err := db.QueryContext(ctx, `
			SELECT bucket, samples, cpu_avg, cpu_max, cpu_peak, mem_used_avg, mem_used_max, mem_total,
			       disk_used, disk_total, load1_avg, load1_max
			FROM metrics_rollups
			WHERE resolution = ? AND bucket >= ?
			ORDER BY bucket`, rng.Source, metricsTime(from))
		if err != nil {
			return nil, err
		}
		var newest time.Time
		for result.Next() {
			var ts string
			var row metricsInput
			if err := result.Scan(&ts, &row.weight, &row.cpuAvg, &row.cpuMax, &row.cpuPeak, &row.memAvg, &row.memMax,
				&row.memTotal, &row.diskUsed, &row.diskTotal, &row.loadAvg, &row.loadMax); err != nil {
				result.Close()
				return nil, err
			}
			at, err := time.Parse(time.RFC3339, ts)
			if err != nil {
				continue
			}
			row.at = at
			rows = append(rows, row)
			newest = at
		}
		if err := result.Close(); err != nil {
			return nil, err
		}
		if !newest.IsZero() {
			rawFrom = metricsTime(newest.Add(time.Duration(rng.Source) * time.Second))
		}
	}
	result, err := db.QueryContext(ctx, `
		SELECT ts, cpu, cpu_peak, mem_used, mem_total, disk_used, disk_total, load1
		FROM metrics_samples
		WHERE ts >= ?
		ORDER BY ts`, rawFrom)
	if err != nil {
		return nil, err
	}
	defer result.Close()
	for result.Next() {
		var ts string
		var row metricsInput
		if err := result.Scan(&ts, &row.cpuAvg, &row.cpuPeak, &row.memAvg, &row.memTotal,
			&row.diskUsed, &row.diskTotal, &row.loadAvg); err != nil {
			return nil, err
		}
		at, err := time.Parse(time.RFC3339, ts)
		if err != nil {
			continue
		}
		row.at, row.weight = at, 1
		row.cpuMax, row.memMax, row.loadMax = row.cpuAvg, row.memAvg, row.loadAvg
		rows = append(rows, row)
	}
	if err := result.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].at.Before(rows[j].at) })
	points := downsampleMetrics(rows, step)
	if len(points) > metricsMaxPoints {
		points = points[len(points)-metricsMaxPoints:]
	}
	return points, nil
}

// metricsLatest is the newest raw sample: the cards' current values.
func readMetricsLatest(ctx context.Context, db *sql.DB) (*metricsPoint, error) {
	var point metricsPoint
	var peak sql.NullFloat64
	err := db.QueryRowContext(ctx, `
		SELECT ts, cpu, cpu_peak, mem_used, mem_total, disk_used, disk_total, load1
		FROM metrics_samples ORDER BY ts DESC LIMIT 1`).Scan(
		&point.TS, &point.CPU, &peak, &point.MemUsed, &point.MemTotal, &point.DiskUsed, &point.DiskTotal, &point.Load1)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	point.CPUMax, point.MemUsedMax, point.Load1Max = point.CPU, point.MemUsed, point.Load1
	if peak.Valid {
		value := peak.Float64
		point.CPUPeak = &value
	}
	return &point, nil
}

func vcpuCount() int {
	if n := numCPU(); n > 0 {
		return n
	}
	return runtime.NumCPU()
}

// handleMetricsHistory: GET /api/v1/metrics/history?range=1h|6h|24h|7d|30d
// (admin — enforced by the isAdminOnlyPath prefix, like every other
// server-wide read).
// handleMetricsHistory: GET /api/v1/metrics/history?range=… (yalnız yönetici).
func (p *Panel) handleMetricsHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rng := metricsRangeFor(r)
	now := time.Now()
	points, err := readMetricsHistory(r.Context(), p.db.GetDB(), rng, now)
	if err != nil {
		writeServerError(w, err)
		return
	}
	latest, err := readMetricsLatest(r.Context(), p.db.GetDB())
	if err != nil {
		writeServerError(w, err)
		return
	}
	keys := make([]string, 0, len(metricsRanges))
	for _, candidate := range metricsRanges {
		keys = append(keys, candidate.Key)
	}
	json.NewEncoder(w).Encode(map[string]any{
		"range":          rng.Key,
		"ranges":         keys,
		"step_seconds":   metricsStep(rng),
		"sample_seconds": int(metricsInterval / time.Second),
		"vcpus":          vcpuCount(),
		"samples":        points,
		"latest":         latest,
	})
}
