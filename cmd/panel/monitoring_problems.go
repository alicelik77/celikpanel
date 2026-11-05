package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

// "Problems now" (alpha.126, Monitoring stage 1; D-024). One list, shown at
// the top of Monitoring and on the Dashboard, computed only from what the
// panel already reads: the stored component scan, the monitoring samples,
// the root file system, the certificate rows, the cached reading of waiting
// operating-system updates and, where Postfix is installed, the mail queue
// summary (read at most every five minutes, in the background). No new
// daemon, no change to any service: reading this list never starts a
// mutation.
//
// Each problem carries a stable Key, its Severity, who acts, the one next
// action and, where the data says it, Since. The screen says the reason and
// how work resumes from Kind. Stage 3 (alerts by email, site uptime) is to
// reuse this model: Key for de-duplication, Since for "lasting more than N
// minutes".
//
// What could not be checked is reported in Unknown, never as "no problem".
//
// "Şu anki sorunlar": panelin zaten okuduğu verilerden hesaplanan tek liste;
// yeni hizmet yok, okumak hiçbir değişiklik başlatmaz. Her sorun kalıcı bir
// anahtar, önem, kimin işlem yapacağı ve tek sonraki eylemi taşır. Kontrol
// edilemeyen şey "sorun yok" değil, Unknown'da bildirilir.

const (
	problemDiskWarning     = 85.0
	problemDiskCritical    = 95.0
	problemSustainedLevel  = 90.0
	problemSustainedWindow = 10 * time.Minute
	problemMailDeferred    = 50
	problemCertificateDays = 14
	problemCertificateCrit = 3
	problemMailQueueFresh  = 5 * time.Minute
	problemMailQueueRetry  = 2 * time.Minute
)

type monitoringProblemAction struct {
	// Kind: start_service (POST /api/v1/service/action with Unit) or open
	// (a screen of the panel, To).
	Kind string `json:"kind"`
	To   string `json:"to,omitempty"`
}

type monitoringProblem struct {
	Key  string `json:"key"`
	Kind string `json:"kind"`
	// Severity: warning or critical.
	Severity string `json:"severity"`
	// Actor: administrator (acts in the panel) or server_owner (acts on the
	// server; the panel cannot do it for them).
	Actor      string                  `json:"actor"`
	Subject    string                  `json:"subject,omitempty"`
	Name       string                  `json:"name,omitempty"`
	Unit       string                  `json:"unit,omitempty"`
	Value      float64                 `json:"value"`
	Since      string                  `json:"since,omitempty"`
	ObservedAt string                  `json:"observed_at,omitempty"`
	Action     monitoringProblemAction `json:"action"`
}

type monitoringUnknown struct {
	// Source: services, metrics, disk, certificates, mail_queue, security_updates.
	Source string `json:"source"`
	// Reason: not_scanned, unreadable, checking.
	Reason string `json:"reason"`
}

type monitoringProblemsReport struct {
	CheckedAt string              `json:"checked_at"`
	Problems  []monitoringProblem `json:"problems"`
	Unknown   []monitoringUnknown `json:"unknown"`
}

var problemKindOrder = map[string]int{
	"service_stopped":            0,
	"disk_full":                  1,
	"certificate_expired":        2,
	"certificate_renewal_failed": 3,
	"certificate_expiring":       4,
	"memory_sustained":           5,
	"cpu_sustained":              6,
	"mail_queue_deferred":        7,
	"security_updates":           8,
}

// collectMonitoringProblems builds the list. Each source is independent: one
// that cannot be read adds an Unknown entry and the others still answer.
func (p *Panel) collectMonitoringProblems(ctx context.Context, now time.Time) monitoringProblemsReport {
	report := monitoringProblemsReport{CheckedAt: metricsTime(now), Problems: []monitoringProblem{}, Unknown: []monitoringUnknown{}}
	add := func(problems []monitoringProblem, unknown *monitoringUnknown) {
		report.Problems = append(report.Problems, problems...)
		if unknown != nil {
			report.Unknown = append(report.Unknown, *unknown)
		}
	}
	db := p.db.GetDB()
	observations, scannedAt, unknown := p.problemScanObservations(ctx)
	add(serviceProblems(observations, p.managedServiceHostProfile(), scannedAt), unknown)
	diskUsed, diskTotal := monitoringReadDisk("/")
	add(diskProblems(diskUsed, diskTotal, now))
	add(sustainedProblems(ctx, db, now))
	add(certificateProblems(ctx, db, now))
	add(p.mailQueueProblems(observations, now))
	add(securityUpdateProblems(monitoringSecurityUpdates(p)))
	sort.SliceStable(report.Problems, func(i, j int) bool {
		a, b := report.Problems[i], report.Problems[j]
		if (a.Severity == "critical") != (b.Severity == "critical") {
			return a.Severity == "critical"
		}
		return problemKindOrder[a.Kind] < problemKindOrder[b.Kind]
	})
	return report
}

// handleMonitoringProblems: GET /api/v1/metrics/problems (administrator,
// through the /api/v1/metrics/ prefix). A read only.
func (p *Panel) handleMonitoringProblems(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := json.NewEncoder(w).Encode(p.collectMonitoringProblems(r.Context(), time.Now())); err != nil {
		log.Printf("encode monitoring problems: %v", err)
	}
}

// --- Components -------------------------------------------------------------

func (p *Panel) problemScanObservations(ctx context.Context) ([]serviceObservation, time.Time, *monitoringUnknown) {
	var data, scannedAt string
	err := p.db.GetDB().QueryRowContext(ctx,
		`SELECT data, scanned_at FROM service_scan_cache WHERE id = 1`).Scan(&data, &scannedAt)
	if err == sql.ErrNoRows {
		return nil, time.Time{}, &monitoringUnknown{Source: "services", Reason: "not_scanned"}
	}
	if err != nil {
		return nil, time.Time{}, &monitoringUnknown{Source: "services", Reason: "unreadable"}
	}
	snapshot, err := decodeScanCacheSnapshot(data)
	if err != nil {
		return nil, time.Time{}, &monitoringUnknown{Source: "services", Reason: "unreadable"}
	}
	at, _ := time.Parse(time.RFC3339, scannedAt)
	return snapshot.Observations, at, nil
}

// serviceProblems: an installed component of kind "service" whose status
// does not say running (the Dashboard's rule, lib/serviceStatus.ts). Tools
// and unit-less runtimes have no daemon to start and are never listed.
func serviceProblems(observations []serviceObservation, host core.ManagedServiceHostProfile, scannedAt time.Time) []monitoringProblem {
	if len(observations) == 0 {
		return nil
	}
	observed := ""
	if !scannedAt.IsZero() {
		observed = metricsTime(scannedAt)
	}
	var problems []monitoringProblem
	for _, service := range catalogViewForHost(observations, host) {
		if service.Kind != core.KindService || service.IsInstalled == nil || !*service.IsInstalled {
			continue
		}
		if serviceStatusRunning(service.Status) {
			continue
		}
		unit := service.Unit
		if unit == "" {
			unit = service.ID
		}
		problems = append(problems, monitoringProblem{
			Key: "service:" + service.ID, Kind: "service_stopped", Severity: "critical", Actor: "administrator",
			Subject: service.ID, Name: service.Name, Unit: unit, ObservedAt: observed,
			Action: monitoringProblemAction{Kind: "start_service", To: "/components/" + url.PathEscape(service.ID)},
		})
	}
	return problems
}

func serviceStatusRunning(status string) bool {
	normalized := strings.ToLower(strings.TrimSpace(status))
	return normalized == "running" || strings.Contains(normalized, "(running)")
}

// --- Disk --------------------------------------------------------------------

var monitoringReadDisk = func(path string) (used, total uint64) { return readDisk(path) }

func diskProblems(used, total uint64, now time.Time) ([]monitoringProblem, *monitoringUnknown) {
	if total == 0 {
		return nil, &monitoringUnknown{Source: "disk", Reason: "unreadable"}
	}
	percent := round1(float64(used) / float64(total) * 100)
	if percent < problemDiskWarning {
		return nil, nil
	}
	severity := "warning"
	if percent >= problemDiskCritical {
		severity = "critical"
	}
	return []monitoringProblem{{
		Key: "disk:/", Kind: "disk_full", Severity: severity, Actor: "server_owner", Subject: "/",
		Value: percent, ObservedAt: metricsTime(now),
		Action: monitoringProblemAction{Kind: "open", To: "/domains"},
	}}, nil
}

// --- CPU and memory sustained ------------------------------------------------

type sustainRow struct {
	at          time.Time
	cpu         float64
	memPercent  float64
	hasMemTotal bool
}

// sustainedAbove reports whether every row of the last window is at or above
// level, and the rows cover the window: the oldest is no later than a minute
// into it and no gap between rows is longer than two intervals. Rows are in
// time order. A short burst never qualifies; a missing stretch is not
// guessed.
func sustainedAbove(rows []sustainRow, now time.Time, window time.Duration, level float64, value func(sustainRow) (float64, bool)) bool {
	start := now.Add(-window)
	var inWindow []sustainRow
	for _, row := range rows {
		if !row.at.Before(start) && !row.at.After(now) {
			inWindow = append(inWindow, row)
		}
	}
	if len(inWindow) == 0 || inWindow[0].at.After(start.Add(time.Minute)) || now.Sub(inWindow[len(inWindow)-1].at) > 2*metricsInterval {
		return false
	}
	for i, row := range inWindow {
		v, ok := value(row)
		if !ok || v < level {
			return false
		}
		if i > 0 && row.at.Sub(inWindow[i-1].at) > 2*metricsInterval {
			return false
		}
	}
	return true
}

// sinceAbove walks back from the newest row while the value stays at or
// above level and returns the first row of that stretch.
func sinceAbove(rows []sustainRow, level float64, value func(sustainRow) (float64, bool)) time.Time {
	var since time.Time
	for i := len(rows) - 1; i >= 0; i-- {
		v, ok := value(rows[i])
		if !ok || v < level {
			break
		}
		if i < len(rows)-1 && rows[i+1].at.Sub(rows[i].at) > 2*metricsInterval {
			break
		}
		since = rows[i].at
	}
	return since
}

var (
	sustainCPU    = func(r sustainRow) (float64, bool) { return r.cpu, true }
	sustainMemory = func(r sustainRow) (float64, bool) { return r.memPercent, r.hasMemTotal }
)

// sustainedProblems reads the interval averages of the last hours. Only rows
// written as averages (cpu_peak present, alpha.126 and later) count: an
// older row is a 200 ms snapshot and proves nothing about ten minutes.
func sustainedProblems(ctx context.Context, db *sql.DB, now time.Time) ([]monitoringProblem, *monitoringUnknown) {
	rows, err := db.QueryContext(ctx, `
		SELECT ts, cpu, mem_used, mem_total FROM metrics_samples
		WHERE ts >= ? AND cpu_peak IS NOT NULL
		ORDER BY ts`, metricsTime(now.Add(-6*time.Hour)))
	if err != nil {
		return nil, &monitoringUnknown{Source: "metrics", Reason: "unreadable"}
	}
	defer rows.Close()
	var samples []sustainRow
	for rows.Next() {
		var ts string
		var cpu float64
		var memUsed, memTotal int64
		if err := rows.Scan(&ts, &cpu, &memUsed, &memTotal); err != nil {
			return nil, &monitoringUnknown{Source: "metrics", Reason: "unreadable"}
		}
		at, err := time.Parse(time.RFC3339, ts)
		if err != nil {
			continue
		}
		row := sustainRow{at: at, cpu: cpu}
		if memTotal > 0 {
			row.memPercent, row.hasMemTotal = float64(memUsed)/float64(memTotal)*100, true
		}
		samples = append(samples, row)
	}
	if rows.Err() != nil {
		return nil, &monitoringUnknown{Source: "metrics", Reason: "unreadable"}
	}
	var problems []monitoringProblem
	for _, check := range []struct {
		kind  string
		value func(sustainRow) (float64, bool)
	}{{"cpu_sustained", sustainCPU}, {"memory_sustained", sustainMemory}} {
		if !sustainedAbove(samples, now, problemSustainedWindow, problemSustainedLevel, check.value) {
			continue
		}
		latest, _ := check.value(samples[len(samples)-1])
		problem := monitoringProblem{
			Key: strings.TrimSuffix(check.kind, "_sustained"), Kind: check.kind, Severity: "warning", Actor: "server_owner",
			Value: round1(latest), ObservedAt: metricsTime(samples[len(samples)-1].at),
			Action: monitoringProblemAction{Kind: "open", To: "/monitoring"},
		}
		if since := sinceAbove(samples, problemSustainedLevel, check.value); !since.IsZero() {
			problem.Since = metricsTime(since)
		}
		problems = append(problems, problem)
	}
	return problems, nil
}

// --- Certificates ------------------------------------------------------------

// certificateProblems: an active certificate whose renewal failed (the
// panel's own renewal, or the operating system's certbot state that needs
// the administrator, D-040), or that expires in under 14 days. A
// certificate waiting for the owner's kept configuration file (D-031) has
// its own notice on the Dashboard and the domain page and is not repeated.
func certificateProblems(ctx context.Context, db *sql.DB, now time.Time) ([]monitoringProblem, *monitoringUnknown) {
	rows, err := db.QueryContext(ctx, `
		SELECT d.name, c.expires_at, COALESCE(c.renewal_status, '')
		FROM ssl_certificates c
		JOIN domains d ON d.id = c.domain_id
		WHERE c.status = 'active'
		ORDER BY d.name`)
	if err != nil {
		return nil, &monitoringUnknown{Source: "certificates", Reason: "unreadable"}
	}
	defer rows.Close()
	var problems []monitoringProblem
	var unknown *monitoringUnknown
	for rows.Next() {
		var name, expires, renewal string
		if err := rows.Scan(&name, &expires, &renewal); err != nil {
			return problems, &monitoringUnknown{Source: "certificates", Reason: "unreadable"}
		}
		if renewal == sslWaitingForOwner {
			continue
		}
		expiry, ok := parseCertTime(expires)
		if !ok {
			unknown = &monitoringUnknown{Source: "certificates", Reason: "unreadable"}
			continue
		}
		days := int(expiry.Sub(now).Hours() / 24)
		failed := renewal == "failed"
		if state, native := strings.CutPrefix(renewal, nativeRenewalStatusPrefix); native && transport.SiteRenewalStateIsProblem(state) {
			failed = true
		}
		kind := ""
		switch {
		case !expiry.After(now):
			kind = "certificate_expired"
		case failed:
			kind = "certificate_renewal_failed"
		case days < problemCertificateDays:
			kind = "certificate_expiring"
		default:
			continue
		}
		severity := "warning"
		if kind == "certificate_expired" || days <= problemCertificateCrit {
			severity = "critical"
		}
		problems = append(problems, monitoringProblem{
			Key: "certificate:" + name, Kind: kind, Severity: severity, Actor: "administrator",
			Subject: name, Value: float64(days),
			Action: monitoringProblemAction{Kind: "open", To: "/domains/" + url.PathEscape(name) + "?tab=ssl"},
		})
	}
	if rows.Err() != nil {
		return problems, &monitoringUnknown{Source: "certificates", Reason: "unreadable"}
	}
	return problems, unknown
}

// --- Mail queue --------------------------------------------------------------

// mailQueueReading is the last summary of the Postfix queue. It is read in
// the background at most every five minutes, and only while this list is
// being looked at; the Postfix page reads the full queue itself.
type mailQueueReading struct {
	mu         sync.Mutex
	summary    core.PostfixSummary
	installed  bool
	at         time.Time
	failedAt   time.Time
	refreshing bool
}

var mailQueueProblemReading mailQueueReading

var startMailQueueRead = func(run func()) { go run() }

var readMailQueueSummary = func(p *Panel, ctx context.Context) (core.PostfixQueueResult, error) {
	var result core.PostfixQueueResult
	err := p.callAgentContext(ctx, "Agent.PostfixQueue", &transport.Empty{}, &result)
	return result, err
}

func postfixInstalled(observations []serviceObservation) bool {
	for _, observation := range observations {
		if observation.ID == "postfix" && observation.IsInstalled {
			return true
		}
	}
	return false
}

func (p *Panel) mailQueueProblems(observations []serviceObservation, now time.Time) ([]monitoringProblem, *monitoringUnknown) {
	if !postfixInstalled(observations) {
		return nil, nil
	}
	c := &mailQueueProblemReading
	c.mu.Lock()
	defer c.mu.Unlock()
	fresh := !c.at.IsZero() && now.Sub(c.at) < problemMailQueueFresh
	waiting := !c.failedAt.IsZero() && now.Sub(c.failedAt) < problemMailQueueRetry
	if !fresh && !waiting && !c.refreshing {
		c.refreshing = true
		startMailQueueRead(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			result, err := readMailQueueSummary(p, ctx)
			c.mu.Lock()
			defer c.mu.Unlock()
			c.refreshing = false
			if err != nil {
				log.Printf("monitoring: mail queue summary could not be read: %v", err)
				c.failedAt = time.Now()
				return
			}
			c.summary, c.installed, c.at, c.failedAt = result.Summary, result.Installed, time.Now(), time.Time{}
		})
	}
	if c.at.IsZero() {
		if waiting && !c.refreshing {
			return nil, &monitoringUnknown{Source: "mail_queue", Reason: "unreadable"}
		}
		return nil, &monitoringUnknown{Source: "mail_queue", Reason: "checking"}
	}
	if !c.installed || c.summary.Deferred <= problemMailDeferred {
		return nil, nil
	}
	return []monitoringProblem{{
		Key: "mail_queue", Kind: "mail_queue_deferred", Severity: "warning", Actor: "administrator",
		Value: float64(c.summary.Deferred), ObservedAt: metricsTime(c.at),
		Action: monitoringProblemAction{Kind: "open", To: "/components/postfix"},
	}}, nil
}

// --- Security updates --------------------------------------------------------

var monitoringSecurityUpdates = func(p *Panel) osUpdatesSummaryView { return osUpdatesSummary.view(p) }

func securityUpdateProblems(view osUpdatesSummaryView) ([]monitoringProblem, *monitoringUnknown) {
	switch view.State {
	case "checking":
		return nil, &monitoringUnknown{Source: "security_updates", Reason: "checking"}
	case "ready":
	default:
		return nil, &monitoringUnknown{Source: "security_updates", Reason: "unreadable"}
	}
	if view.Pending != nil && view.Pending.State == "busy" {
		// Another package operation holds the lock; the count follows.
		return nil, &monitoringUnknown{Source: "security_updates", Reason: "checking"}
	}
	if view.Pending == nil || view.Pending.State != "known" {
		return nil, &monitoringUnknown{Source: "security_updates", Reason: "unreadable"}
	}
	if view.Pending.SecurityState != "" && view.Pending.SecurityState != "known" {
		// pacman without arch-audit: which updates fix security problems is
		// not known, so neither a problem nor "none" is claimed.
		return nil, &monitoringUnknown{Source: "security_updates", Reason: "unreadable"}
	}
	if view.Pending.Security <= 0 {
		return nil, nil
	}
	return []monitoringProblem{{
		Key: "security_updates", Kind: "security_updates", Severity: "warning", Actor: "administrator",
		Value: float64(view.Pending.Security), ObservedAt: view.CheckedAt,
		Action: monitoringProblemAction{Kind: "open", To: "/security"},
	}}, nil
}
