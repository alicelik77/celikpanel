package db

import (
	"path/filepath"
	"strings"
	"testing"
)

// Migration 56 (alpha.117, owner decision of 2 November 2026): the change of
// passwords and keys after a subscription transfer. Additive: two tables and
// one index. 055 is the parallel security change; 56 follows it.
const transferRotationMigrationVersion = 56

func TestTransferRotationSchema(t *testing.T) {
	database, err := NewSQLiteDB(filepath.Join(t.TempDir(), "panel.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	raw := database.GetDB()
	op := strings.Repeat("a", 32)
	if _, err := raw.Exec(`INSERT INTO transfer_rotations (id, transfer_id, subscription_id) VALUES (?, 1, 700)`, op); err != nil {
		t.Fatal(err)
	}
	// One rotation per transfer.
	if _, err := raw.Exec(`INSERT INTO transfer_rotations (id, transfer_id, subscription_id) VALUES (?, 1, 700)`, strings.Repeat("b", 32)); err == nil {
		t.Fatal("a second rotation for the same transfer was accepted")
	}
	if _, err := raw.Exec(`INSERT INTO transfer_rotation_effects (operation_id, seq, kind, target) VALUES (?, 0, 'mailbox', 'a@b.test')`, op); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{
		`INSERT INTO transfer_rotation_effects (operation_id, seq, kind, target) VALUES ('` + op + `', 1, 'ftp', 'x')`,
		`INSERT INTO transfer_rotation_effects (operation_id, seq, kind, target, state) VALUES ('` + op + `', 1, 'vpn', 'x', 'lost')`,
		`INSERT INTO transfer_rotation_effects (operation_id, seq, kind, target, ref) VALUES ('` + op + `', 1, 'vpn', 'x', 'not json')`,
		`INSERT INTO transfer_rotations (id, transfer_id, subscription_id, state) VALUES ('` + strings.Repeat("c", 32) + `', 2, 700, 'paused')`,
		`INSERT INTO transfer_rotations (id, transfer_id, subscription_id) VALUES ('short', 3, 700)`,
	} {
		if _, err := raw.Exec(bad); err == nil {
			t.Fatalf("accepted: %s", bad)
		}
	}
}

func TestOlderReleaseRefusesALedgerWithTheTransferRotationEntry(t *testing.T) {
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
		if migration.version < transferRotationMigrationVersion {
			older = append(older, migration)
		}
	}
	requireMigrationsFrom(t, embedded, older, transferRotationMigrationVersion)
	err = database.verifyMigrationLedgerCoverage(t.Context(), older)
	database.Close()
	if err == nil || !strings.Contains(err.Error(), "no matching embedded migration") {
		t.Fatalf("an older release accepted the newer ledger: %v", err)
	}
}
