package db

import (
	"path/filepath"
	"strings"
	"testing"
)

// Migration 62 (alpha.119): the first-login tour's state, one row per person.
// Additive: one table and a trigger that removes the row with its user;
// nothing existing changes. A release before it refuses a ledger that carries
// it; going back is the pre-update snapshot restore (D-025).
const userGuideStateMigrationVersion = 62

func TestOlderReleaseRefusesALedgerWithTheUserGuideStateEntry(t *testing.T) {
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
		if migration.version < userGuideStateMigrationVersion {
			older = append(older, migration)
		}
	}
	requireMigrationsFrom(t, embedded, older, userGuideStateMigrationVersion)
	err = database.verifyMigrationLedgerCoverage(t.Context(), older)
	database.Close()
	if err == nil || !strings.Contains(err.Error(), "no matching embedded migration") {
		t.Fatalf("an older release accepted the newer ledger: %v", err)
	}
}

func TestUserGuideStateMigrationCreatesItsTableAndChecks(t *testing.T) {
	database, err := NewSQLiteDB(filepath.Join(t.TempDir(), "panel.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	db := database.GetDB()
	for _, object := range []struct{ kind, name string }{{"table", "user_guide_state"}, {"trigger", "user_guide_state_user_deleted"}} {
		var name string
		if err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type = ? AND name = ?`, object.kind, object.name).Scan(&name); err != nil {
			t.Fatalf("%s %s is missing: %v", object.kind, object.name, err)
		}
	}
	if _, err := db.Exec(`INSERT INTO user_guide_state (user_id) VALUES (1)`); err != nil {
		t.Fatal(err)
	}
	var version int
	var state string
	if err := db.QueryRow(`SELECT tour_version, tour_state FROM user_guide_state WHERE user_id = 1`).Scan(&version, &state); err != nil || version != 0 || state != "pending" {
		t.Fatalf("defaults %d %q (%v)", version, state, err)
	}
	for _, bad := range []string{
		`INSERT INTO user_guide_state (user_id, tour_state) VALUES (2, 'done')`,
		`INSERT INTO user_guide_state (user_id, tour_version) VALUES (3, -1)`,
		`INSERT INTO user_guide_state (user_id, tour_version) VALUES (4, 1001)`,
		`INSERT INTO user_guide_state (user_id) VALUES (1)`,
	} {
		if _, err := db.Exec(bad); err == nil {
			t.Fatalf("accepted: %s", bad)
		}
	}
}
