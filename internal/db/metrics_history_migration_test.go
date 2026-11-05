package db

import (
	"path/filepath"
	"strings"
	"testing"
)

// Migration 64 (alpha.126): monitoring history. Additive: one NULL column on
// metrics_samples (cpu_peak) and the metrics_rollups table. A release before
// it refuses a ledger that carries it; going back is the pre-update snapshot
// restore (D-025).
const metricsHistoryMigrationVersion = 64

func TestOlderReleaseRefusesALedgerWithTheMetricsHistoryEntry(t *testing.T) {
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
		if migration.version < metricsHistoryMigrationVersion {
			older = append(older, migration)
		}
	}
	requireMigrationsFrom(t, embedded, older, metricsHistoryMigrationVersion)
	err = database.verifyMigrationLedgerCoverage(t.Context(), older)
	database.Close()
	if err == nil || !strings.Contains(err.Error(), "no matching embedded migration") {
		t.Fatalf("an older release accepted the newer ledger: %v", err)
	}
}

func TestMetricsHistoryMigrationKeepsOldSamplesAndChecksRollups(t *testing.T) {
	database, err := NewSQLiteDB(filepath.Join(t.TempDir(), "panel.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	db := database.GetDB()
	// An older release's insert (no cpu_peak) still works and leaves it NULL.
	if _, err := db.Exec(`INSERT INTO metrics_samples (ts, cpu, mem_used, mem_total, disk_used, disk_total, load1)
		VALUES ('2026-11-05T10:00:00Z', 51.2, 1, 2, 3, 4, 0.1)`); err != nil {
		t.Fatal(err)
	}
	var peakIsNull bool
	if err := db.QueryRow(`SELECT cpu_peak IS NULL FROM metrics_samples`).Scan(&peakIsNull); err != nil || !peakIsNull {
		t.Fatalf("an old-style row did not keep cpu_peak NULL: %v %v", peakIsNull, err)
	}
	good := []string{
		`INSERT INTO metrics_rollups VALUES (300, '2026-11-05T10:05:00Z', 10, 2.5, 9, 40, 1, 2, 4, 1, 9, 0.1, 0.2)`,
		`INSERT INTO metrics_rollups VALUES (3600, '2026-11-05T10:00:00Z', 120, 2.5, 9, NULL, 1, 2, 4, 1, 9, 0.1, 0.2)`,
	}
	for _, statement := range good {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
	bad := []string{
		// Same bucket twice.
		`INSERT INTO metrics_rollups VALUES (300, '2026-11-05T10:05:00Z', 10, 2.5, 9, 40, 1, 2, 4, 1, 9, 0.1, 0.2)`,
		// A resolution the panel does not keep.
		`INSERT INTO metrics_rollups VALUES (60, '2026-11-05T10:06:00Z', 10, 2.5, 9, 40, 1, 2, 4, 1, 9, 0.1, 0.2)`,
		// Another text form of the time (would break text-ordered ranges).
		`INSERT INTO metrics_rollups VALUES (300, '2026-11-05 10:10:00', 10, 2.5, 9, 40, 1, 2, 4, 1, 9, 0.1, 0.2)`,
		`INSERT INTO metrics_rollups VALUES (300, '2026-11-05T10:10:30Z', 10, 2.5, 9, 40, 1, 2, 4, 1, 9, 0.1, 0.2)`,
		// An empty bucket.
		`INSERT INTO metrics_rollups VALUES (300, '2026-11-05T10:15:00Z', 0, 2.5, 9, 40, 1, 2, 4, 1, 9, 0.1, 0.2)`,
	}
	for _, statement := range bad {
		if _, err := db.Exec(statement); err == nil {
			t.Fatalf("accepted: %s", statement)
		}
	}
}
