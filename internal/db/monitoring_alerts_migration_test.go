package db

import (
	"path/filepath"
	"strings"
	"testing"
)

// Migration 65 (alpha.127): monitoring stage 2 and 3. Additive: four NULL
// columns on metrics_samples and three new tables. A release before it
// refuses a ledger that carries it; going back is the pre-update snapshot
// restore (D-025).
const monitoringAlertsMigrationVersion = 65

func TestOlderReleaseRefusesALedgerWithTheMonitoringAlertsEntry(t *testing.T) {
	database, err := NewSQLiteDB(filepath.Join(t.TempDir(), "panel.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	embedded, err := loadEmbeddedMigrations()
	if err != nil {
		t.Fatal(err)
	}
	older := make([]embeddedMigration, 0, len(embedded))
	for _, migration := range embedded {
		if migration.version < monitoringAlertsMigrationVersion {
			older = append(older, migration)
		}
	}
	requireMigrationsFrom(t, embedded, older, monitoringAlertsMigrationVersion)
	err = database.verifyMigrationLedgerCoverage(t.Context(), older)
	database.Close()
	if err == nil || !strings.Contains(err.Error(), "no matching embedded migration") {
		t.Fatalf("an older release accepted the newer ledger: %v", err)
	}
}

func TestMonitoringAlertsMigrationKeepsOldSamplesAndChecksItsRows(t *testing.T) {
	database, err := NewSQLiteDB(filepath.Join(t.TempDir(), "panel.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	db := database.GetDB()
	// An older release's insert still works and leaves the new columns NULL.
	if _, err := db.Exec(`INSERT INTO metrics_samples (ts, cpu, cpu_peak, mem_used, mem_total, disk_used, disk_total, load1)
		VALUES ('2026-11-05T10:00:00Z', 51.2, 60, 1, 2, 3, 4, 0.1)`); err != nil {
		t.Fatal(err)
	}
	var allNull bool
	if err := db.QueryRow(`SELECT net_rx IS NULL AND net_tx IS NULL AND disk_read IS NULL AND disk_write IS NULL FROM metrics_samples`).Scan(&allNull); err != nil || !allNull {
		t.Fatalf("an old-style row did not keep the new columns NULL: %v %v", allNull, err)
	}
	good := []string{
		`INSERT INTO metrics_process_snapshots (ts, reason, value, processes) VALUES ('2026-11-05T10:00:30Z', 'cpu', 95, '[{"pid":1,"name":"php-fpm"}]')`,
		`INSERT INTO monitoring_alerts (key, kind, severity, state, first_seen, last_seen) VALUES ('disk:/', 'disk_full', 'warning', 'pending', 'a', 'a')`,
		`INSERT INTO monitoring_alerts (key, kind, severity, state, first_seen, opened_at, last_seen, resolved_at) VALUES ('disk:/', 'disk_full', 'warning', 'resolved', 'a', 'b', 'b', 'c')`,
		`INSERT INTO site_uptime (domain, checked_at, state, scheme, down_since) VALUES ('example.com', 'a', 'down', 'https', 'a')`,
		`INSERT INTO site_uptime (domain, checked_at, state, scheme, http_status) VALUES ('example.org', 'a', 'up', 'http', 200)`,
	}
	for _, statement := range good {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
	bad := []string{
		// A second active row for the same key (de-duplication).
		`INSERT INTO monitoring_alerts (key, kind, severity, state, first_seen, opened_at, last_seen) VALUES ('disk:/', 'disk_full', 'warning', 'open', 'a', 'b', 'b')`,
		// Open without opened_at; pending with it.
		`INSERT INTO monitoring_alerts (key, kind, severity, state, first_seen, last_seen) VALUES ('x', 'disk_full', 'warning', 'open', 'a', 'a')`,
		`INSERT INTO monitoring_alerts (key, kind, severity, state, first_seen, opened_at, last_seen) VALUES ('y', 'disk_full', 'warning', 'pending', 'a', 'a', 'a')`,
		// A severity or mail state the panel does not use.
		`INSERT INTO monitoring_alerts (key, kind, severity, state, first_seen, last_seen) VALUES ('z', 'disk_full', 'info', 'pending', 'a', 'a')`,
		`INSERT INTO monitoring_alerts (key, kind, severity, state, first_seen, last_seen, mail_state) VALUES ('w', 'disk_full', 'warning', 'pending', 'a', 'a', 'queued')`,
		// A snapshot that is not a list, or of another reason.
		`INSERT INTO metrics_process_snapshots (ts, reason, value, processes) VALUES ('2026-11-05T10:00:30Z', 'cpu', 95, '{}')`,
		`INSERT INTO metrics_process_snapshots (ts, reason, value, processes) VALUES ('2026-11-05T10:00:30Z', 'disk', 95, '[]')`,
		`INSERT INTO metrics_process_snapshots (ts, reason, value, processes) VALUES ('2026-11-05 10:00:30', 'cpu', 95, '[]')`,
		// Down without down_since; up with it.
		`INSERT INTO site_uptime (domain, checked_at, state, scheme) VALUES ('a.example', 'a', 'down', 'http')`,
		`INSERT INTO site_uptime (domain, checked_at, state, scheme, down_since) VALUES ('b.example', 'a', 'up', 'http', 'a')`,
	}
	for _, statement := range bad {
		if _, err := db.Exec(statement); err == nil {
			t.Fatalf("accepted: %s", statement)
		}
	}
}
