package db

import (
	"path/filepath"
	"strings"
	"testing"
)

// Migration 79 (alpha.134, rights by exact name, docs/DATABASE-USERS.md):
// database_grant_forms holds the before-image of every pattern-form grant
// row found on a MariaDB, and database_grant_form_engines where each engine
// stands. Additive only; a release before it refuses a ledger that carries
// it, and going back is the pre-update snapshot restore (D-025). The
// integrator renumbers it when the release's migrations are joined.
const databaseGrantFormsMigrationVersion = 79

func TestOlderReleaseRefusesALedgerWithTheDatabaseGrantFormsEntry(t *testing.T) {
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
		if migration.version < databaseGrantFormsMigrationVersion {
			older = append(older, migration)
		}
	}
	requireMigrationsFrom(t, embedded, older, databaseGrantFormsMigrationVersion)
	err = database.verifyMigrationLedgerCoverage(t.Context(), older)
	database.Close()
	if err == nil || !strings.Contains(err.Error(), "no matching embedded migration") {
		t.Fatalf("an older release accepted the newer ledger: %v", err)
	}
}

// The tables keep only what they are for: a known kind and state, a valid
// JSON list of names, one row per account, host and stored name.
func TestDatabaseGrantFormsTablesRefuseWhatTheyAreNotFor(t *testing.T) {
	database, err := NewSQLiteDB(filepath.Join(t.TempDir(), "panel.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	db := database.GetDB()
	insert := `INSERT INTO database_grant_forms (engine_key, username, host, stored_name, kind, state, reached) VALUES (?, ?, ?, ?, ?, ?, ?)`
	if _, err := db.Exec(insert, "mariadb@this-machine:3306", "s5__x_user", "localhost", "s5__x", "ours", "captured", `["s51_x"]`); err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]any{
		{"mariadb@this-machine:3306", "s5__x_user", "localhost", "s5__x", "ours", "captured", `[]`}, // the same row again
		{"mariadb@this-machine:3306", "u", "localhost", "a_b", "mine", "captured", `[]`},            // unknown kind
		{"mariadb@this-machine:3306", "u", "localhost", "a_b", "ours", "converting", `[]`},          // unknown state
		{"mariadb@this-machine:3306", "u", "localhost", "a_b", "ours", "captured", `not json`},      // not a list
		{"", "u", "localhost", "a_b", "ours", "captured", `[]`},                                     // no engine
	} {
		if _, err := db.Exec(insert, bad...); err == nil {
			t.Fatalf("accepted %v", bad)
		}
	}
	if _, err := db.Exec(`INSERT INTO database_grant_form_engines (engine_key, state) VALUES ('mariadb@this-machine:3306', 'paused')`); err == nil {
		t.Fatal("an unknown engine state was accepted")
	}
	if _, err := db.Exec(`INSERT INTO database_grant_form_engines (engine_key, state, reason) VALUES ('mariadb@this-machine:3306', 'waiting', 'update_verifying')`); err != nil {
		t.Fatal(err)
	}
}
