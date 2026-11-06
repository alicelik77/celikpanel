package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func alertState(t *testing.T, db *sql.DB, key string) (state, mail string) {
	t.Helper()
	err := db.QueryRow(`SELECT state, mail_state FROM monitoring_alerts WHERE key = ? ORDER BY id DESC LIMIT 1`, key).Scan(&state, &mail)
	if err == sql.ErrNoRows {
		return "", ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return state, mail
}

func problemsWith(problems ...monitoringProblem) monitoringProblemsReport {
	return monitoringProblemsReport{Problems: problems, Unknown: []monitoringUnknown{}}
}

var diskProblem = monitoringProblem{Key: "disk:/", Kind: "disk_full", Severity: "warning", Value: 88}

func TestAlertOpensOnlyAfterTheDelayAndABlipNeverAlerts(t *testing.T) {
	_, db := metricsTestDB(t)
	ctx := context.Background()
	settings := alertSettings{DelayMinutes: 10, Email: true}
	noDisk := func() (float64, bool) { return 50, true }
	start := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)

	// A blip: seen for five minutes, then gone. It never opens and leaves no row.
	for minute := 0; minute < 5; minute++ {
		if err := applyAlertProblems(ctx, db, problemsWith(diskProblem), settings, start.Add(time.Duration(minute)*time.Minute), noDisk); err != nil {
			t.Fatal(err)
		}
	}
	if state, _ := alertState(t, db, "disk:/"); state != "pending" {
		t.Fatalf("after five minutes: %q", state)
	}
	if err := applyAlertProblems(ctx, db, problemsWith(), settings, start.Add(5*time.Minute), noDisk); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, db, `SELECT COUNT(*) FROM monitoring_alerts`); n != 0 {
		t.Fatalf("a five-minute blip left %d rows", n)
	}

	// Lasting: opens at ten minutes, once, with its email pending.
	start = start.Add(time.Hour)
	for minute := 0; minute <= 12; minute++ {
		if err := applyAlertProblems(ctx, db, problemsWith(diskProblem), settings, start.Add(time.Duration(minute)*time.Minute), noDisk); err != nil {
			t.Fatal(err)
		}
		state, _ := alertState(t, db, "disk:/")
		if want := map[bool]string{true: "open", false: "pending"}[minute >= 10]; state != want {
			t.Fatalf("minute %d: %q, want %q", minute, state, want)
		}
	}
	if n := countRows(t, db, `SELECT COUNT(*) FROM monitoring_alerts`); n != 1 {
		t.Fatalf("one lasting problem made %d rows", n)
	}
	if _, mail := alertState(t, db, "disk:/"); mail != "pending" {
		t.Fatalf("mail = %q", mail)
	}
}

func TestAlertCountsFromTheProblemsOwnSinceAndHonoursTheDelaySetting(t *testing.T) {
	_, db := metricsTestDB(t)
	now := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	cpu := monitoringProblem{Key: "cpu", Kind: "cpu_sustained", Severity: "warning", Value: 97, Since: metricsTime(now.Add(-12 * time.Minute))}
	if err := applyAlertProblems(context.Background(), db, problemsWith(cpu), alertSettings{DelayMinutes: 10}, now, nil); err != nil {
		t.Fatal(err)
	}
	if state, mail := alertState(t, db, "cpu"); state != "open" || mail != "none" {
		t.Fatalf("a load sustained 12 minutes: %q %q (email off: none)", state, mail)
	}
	site := monitoringProblem{Key: "site:a.example", Kind: "site_down", Severity: "critical", Since: metricsTime(now.Add(-12 * time.Minute))}
	if err := applyAlertProblems(context.Background(), db, problemsWith(cpu, site), alertSettings{DelayMinutes: 30}, now, nil); err != nil {
		t.Fatal(err)
	}
	if state, _ := alertState(t, db, "site:a.example"); state != "pending" {
		t.Fatalf("a 30-minute delay opened a 12-minute outage: %q", state)
	}
}

func TestAlertClearsOnlyAfterTheClearDelayAndKeepsWhatCannotBeChecked(t *testing.T) {
	_, db := metricsTestDB(t)
	ctx := context.Background()
	settings := alertSettings{DelayMinutes: 5}
	now := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	service := monitoringProblem{Key: "service:nginx", Kind: "service_stopped", Severity: "critical", Since: metricsTime(now.Add(-time.Hour))}
	step := func(at time.Time, report monitoringProblemsReport, disk float64) {
		t.Helper()
		if err := applyAlertProblems(ctx, db, report, settings, at, func() (float64, bool) { return disk, true }); err != nil {
			t.Fatal(err)
		}
	}
	step(now, problemsWith(service, diskProblem), 88)
	step(now.Add(6*time.Minute), problemsWith(service, diskProblem), 88)
	if state, _ := alertState(t, db, "service:nginx"); state != "open" {
		t.Fatalf("service: %q", state)
	}
	// Services cannot be read: the open alert is kept, not resolved.
	unreadable := problemsWith(diskProblem)
	unreadable.Unknown = []monitoringUnknown{{Source: "services", Reason: "unreadable"}}
	for minute := 7; minute < 40; minute++ {
		step(now.Add(time.Duration(minute)*time.Minute), unreadable, 88)
	}
	if state, _ := alertState(t, db, "service:nginx"); state != "open" {
		t.Fatalf("an unknown reading closed the alert: %q", state)
	}
	// Gone for five minutes, back: the same alert (hysteresis, no second row).
	at := now.Add(40 * time.Minute)
	step(at, problemsWith(diskProblem), 88)
	step(at.Add(5*time.Minute), problemsWith(diskProblem), 88)
	step(at.Add(6*time.Minute), problemsWith(service, diskProblem), 88)
	if n := countRows(t, db, `SELECT COUNT(*) FROM monitoring_alerts WHERE key = 'service:nginx'`); n != 1 {
		t.Fatalf("a flapping service made %d alerts", n)
	}
	// Gone for ten minutes: resolved.
	at = at.Add(7 * time.Minute)
	step(at, problemsWith(diskProblem), 88)
	step(at.Add(9*time.Minute), problemsWith(diskProblem), 88)
	if state, _ := alertState(t, db, "service:nginx"); state != "open" {
		t.Fatalf("nine minutes cleared it: %q", state)
	}
	step(at.Add(10*time.Minute), problemsWith(diskProblem), 88)
	if state, _ := alertState(t, db, "service:nginx"); state != "resolved" {
		t.Fatalf("ten minutes did not clear it: %q", state)
	}
	// The disk at 82 % is still the same problem; at 70 % it clears.
	for minute := 0; minute <= 20; minute++ {
		step(at.Add(time.Duration(20+minute)*time.Minute), problemsWith(), 82)
	}
	if state, _ := alertState(t, db, "disk:/"); state != "open" {
		t.Fatalf("82 %% cleared the disk alert: %q", state)
	}
	for minute := 0; minute <= 11; minute++ {
		step(at.Add(time.Duration(50+minute)*time.Minute), problemsWith(), 70)
	}
	if state, _ := alertState(t, db, "disk:/"); state != "resolved" {
		t.Fatalf("70 %% did not clear the disk alert: %q", state)
	}
	// It comes back within six hours: a new alert, shown, but not emailed again.
	if _, err := db.Exec(`UPDATE monitoring_alerts SET mail_state = 'sent' WHERE key = 'service:nginx'`); err != nil {
		t.Fatal(err)
	}
	settings.Email = true
	later := at.Add(2 * time.Hour)
	step(later, problemsWith(service), 50)
	if state, mail := alertState(t, db, "service:nginx"); state != "open" || mail != "suppressed" {
		t.Fatalf("a repeat within six hours: %q %q", state, mail)
	}
}

// --- Email ------------------------------------------------------------------

type alertMailbox struct {
	mu      sync.Mutex
	to      []string
	bodies  []string
	sendErr error
	sends   int
}

func stubAlertMail(t *testing.T, mailSystem bool) *alertMailbox {
	t.Helper()
	box := &alertMailbox{}
	oldHost, oldProbe, oldSend := accountLinkPanelHost, accountMailProbe, accountMailSend
	t.Cleanup(func() {
		accountLinkPanelHost, accountMailProbe, accountMailSend = oldHost, oldProbe, oldSend
		resetAccountMailCache()
		alertMailLog.mu.Lock()
		alertMailLog.sent = nil
		alertMailLog.mu.Unlock()
	})
	accountLinkPanelHost = func() string { return "panel.example.test" }
	accountMailProbe = func(context.Context) error {
		if !mailSystem {
			return errors.New("connection refused")
		}
		return nil
	}
	accountMailSend = func(_ context.Context, _, to string, message []byte) error {
		box.mu.Lock()
		defer box.mu.Unlock()
		box.sends++
		if box.sendErr != nil {
			return box.sendErr
		}
		box.to = append(box.to, to)
		box.bodies = append(box.bodies, decodeRecoveryMail(t, message))
		return nil
	}
	resetAccountMailCache()
	alertMailLog.mu.Lock()
	alertMailLog.sent = nil
	alertMailLog.mu.Unlock()
	return box
}

func seedAlertAdmin(t *testing.T, db *sql.DB, name string, verified bool) {
	t.Helper()
	var at any
	if verified {
		at = "2026-11-01T10:00:00Z"
	}
	if _, err := db.Exec(`INSERT INTO users (username, password_hash, email, role, email_verified_at) VALUES (?, 'x', ?, 'admin', ?)`,
		name, name+"@example.com", at); err != nil {
		t.Fatal(err)
	}
}

func openTestAlert(t *testing.T, db *sql.DB, key string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO monitoring_alerts (key, kind, severity, subject, value, state, first_seen, opened_at, last_seen, mail_state)
		VALUES (?, 'disk_full', 'warning', '/', 91, 'open', 'a', 'b', 'b', 'pending')`, key); err != nil {
		t.Fatal(err)
	}
}

func TestAlertEmailWaitsForAConfirmedAddress(t *testing.T) {
	p, db := metricsTestDB(t)
	box := stubAlertMail(t, true)
	seedAlertAdmin(t, db, "admin1", false)
	openTestAlert(t, db, "disk:/")
	p.sendPendingAlertMail(context.Background(), time.Now())
	if _, mail := alertState(t, db, "disk:/"); mail != "not_confirmed" || box.sends != 0 {
		t.Fatalf("an unconfirmed address was emailed: %q, %d sends", mail, box.sends)
	}
	// Confirmed once: the next alert is emailed, to that address only.
	if _, err := db.Exec(`UPDATE users SET email_verified_at = '2026-11-06T09:00:00Z' WHERE username = 'admin1'`); err != nil {
		t.Fatal(err)
	}
	seedAlertAdmin(t, db, "admin2", false)
	openTestAlert(t, db, "cpu")
	p.sendPendingAlertMail(context.Background(), time.Now())
	if _, mail := alertState(t, db, "cpu"); mail != "sent" {
		t.Fatalf("mail = %q", mail)
	}
	if len(box.to) != 1 || box.to[0] != "admin1@example.com" {
		t.Fatalf("sent to %v", box.to)
	}
	body := box.bodies[0]
	for _, want := range []string{"/monitoring", "The disk is 91% full", "outside uptime checker", "Disk %91 dolu"} {
		if !strings.Contains(body, want) {
			t.Fatalf("the email lacks %q:\n%s", want, body)
		}
	}
}

func TestAlertEmailFailureIsRecordedAndNeverRetriedInALoop(t *testing.T) {
	p, db := metricsTestDB(t)
	box := stubAlertMail(t, true)
	seedAlertAdmin(t, db, "admin1", true)
	box.sendErr = errors.New("sendmail: queue file write error")
	openTestAlert(t, db, "disk:/")
	for i := 0; i < 5; i++ {
		p.sendPendingAlertMail(context.Background(), time.Now())
	}
	if box.sends != 1 {
		t.Fatalf("a refused hand-over was tried %d times", box.sends)
	}
	var detail string
	if err := db.QueryRow(`SELECT mail_state || ':' || COALESCE(mail_detail, '') FROM monitoring_alerts WHERE key = 'disk:/'`).Scan(&detail); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(detail, "failed:") || !strings.Contains(detail, "queue file") {
		t.Fatalf("mail state = %q", detail)
	}
}

func TestAlertEmailSaysWhenNoMailSystemAnswersAndCapsAnHour(t *testing.T) {
	p, db := metricsTestDB(t)
	stubAlertMail(t, false)
	seedAlertAdmin(t, db, "admin1", true)
	openTestAlert(t, db, "disk:/")
	p.sendPendingAlertMail(context.Background(), time.Now())
	if _, mail := alertState(t, db, "disk:/"); mail != "not_available" {
		t.Fatalf("no mail system: %q", mail)
	}

	box := stubAlertMail(t, true)
	now := time.Now()
	for i := 0; i < alertMailsPerHour+2; i++ {
		openTestAlert(t, db, "k"+string(rune('a'+i)))
		p.sendPendingAlertMail(context.Background(), now.Add(time.Duration(i)*time.Minute))
	}
	if box.sends != alertMailsPerHour {
		t.Fatalf("%d emails in an hour, want %d", box.sends, alertMailsPerHour)
	}
	if _, mail := alertState(t, db, "kf"); mail != "suppressed" {
		t.Fatalf("over the cap: %q", mail)
	}
}

func TestAlertSettingsHandlerValidatesAndStores(t *testing.T) {
	p, db := metricsTestDB(t)
	stubAlertMail(t, true)
	if got := p.alertSettings(context.Background()); got != (alertSettings{DelayMinutes: 10, Email: true, SiteChecks: true}) {
		t.Fatalf("defaults = %+v", got)
	}
	for _, body := range []string{`{}`, `{"delay_minutes":7,"email":true,"site_checks":true}`, `{"delay_minutes":10,"email":true,"site_checks":true,"x":1}`, `nonsense`} {
		w := httptest.NewRecorder()
		p.handleMonitoringAlertSettings(w, httptest.NewRequest(http.MethodPut, "/api/v1/metrics/alerts/settings", strings.NewReader(body)))
		if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), errCodeAlertSettingsInvalid) {
			t.Fatalf("%s: %d %s", body, w.Code, w.Body)
		}
	}
	if _, err := db.Exec(`INSERT INTO site_uptime (domain, checked_at, state, scheme) VALUES ('a.example', 'x', 'up', 'http')`); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	p.handleMonitoringAlertSettings(w, httptest.NewRequest(http.MethodPut, "/api/v1/metrics/alerts/settings",
		strings.NewReader(`{"delay_minutes":30,"email":false,"site_checks":false}`)))
	if w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if got := p.alertSettings(context.Background()); got != (alertSettings{DelayMinutes: 30, Email: false, SiteChecks: false}) {
		t.Fatalf("stored = %+v", got)
	}
	if n := countRows(t, db, `SELECT COUNT(*) FROM site_uptime`); n != 0 {
		t.Fatal("turning site checks off kept the old results")
	}

	// The list: open first, unseen counted, then marked seen.
	openTestAlert(t, db, "disk:/")
	w = httptest.NewRecorder()
	p.handleMonitoringAlerts(w, httptest.NewRequest(http.MethodGet, "/api/v1/metrics/alerts", nil))
	var report alertsReport
	if err := json.Unmarshal(w.Body.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Unseen != 1 || len(report.Alerts) != 1 || report.Settings.DelayMinutes != 30 || !report.Email.Mail.Available {
		t.Fatalf("report = %+v", report)
	}
	w = httptest.NewRecorder()
	p.handleMonitoringAlertsSeen(w, httptest.NewRequest(http.MethodPost, "/api/v1/metrics/alerts/seen", nil))
	if w.Code != http.StatusOK || countRows(t, db, `SELECT COUNT(*) FROM monitoring_alerts WHERE seen_at IS NULL`) != 0 {
		t.Fatalf("seen: %d", w.Code)
	}
}
