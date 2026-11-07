package db

import (
	"path/filepath"
	"strings"
	"testing"
)

// Migration 71 (alpha.128, G15): remote API tokens. Additive: one table and
// one index. A release before it refuses a ledger that carries it; going back
// is the pre-update snapshot restore (D-025).
const apiTokensMigrationVersion = 71

func TestOlderReleaseRefusesALedgerWithTheAPITokensEntry(t *testing.T) {
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
		if migration.version < apiTokensMigrationVersion {
			older = append(older, migration)
		}
	}
	requireMigrationsFrom(t, embedded, older, apiTokensMigrationVersion)
	err = database.verifyMigrationLedgerCoverage(t.Context(), older)
	database.Close()
	if err == nil || !strings.Contains(err.Error(), "no matching embedded migration") {
		t.Fatalf("an older release accepted the newer ledger: %v", err)
	}
}

func TestAPITokensMigrationChecksItsRows(t *testing.T) {
	database, err := NewSQLiteDB(filepath.Join(t.TempDir(), "panel.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	db := database.GetDB()
	if _, err := db.Exec(`INSERT INTO users (id, username, password_hash, email, role, status) VALUES (41, 'token-owner', 'x', 'o@example.test', 'customer', 'active')`); err != nil {
		t.Fatal(err)
	}
	hash := strings.Repeat("a", 64)
	good := `INSERT INTO api_tokens (user_id, name, token_id, secret_sha256, scope, auth_epoch, created_at, expires_at)
		VALUES (41, 'deploy', 'abcdefghijkl', '` + hash + `', 'read', 0, 100, 200)`
	if _, err := db.Exec(good); err != nil {
		t.Fatalf("a valid token row was refused: %v", err)
	}
	for name, statement := range map[string]string{
		"duplicate id":   strings.Replace(good, "'deploy'", "'other'", 1),
		"short id":       strings.Replace(good, "'abcdefghijkl'", "'abc'", 1),
		"short hash":     strings.Replace(good, "'"+hash+"'", "'abc'", 1),
		"never expiring": strings.Replace(strings.Replace(good, "'abcdefghijkl'", "'mnopqrstuvwx'", 1), "100, 200", "100, 100", 1),
		"no owner":       strings.Replace(strings.Replace(good, "'abcdefghijkl'", "'mnopqrstuvwx'", 1), "(41,", "(4242,", 1),
	} {
		if _, err := db.Exec(statement); err == nil {
			t.Errorf("%s: the row was accepted", name)
		}
	}
	// Removing the account removes its tokens.
	if _, err := db.Exec(`DELETE FROM users WHERE id = 41`); err != nil {
		t.Fatal(err)
	}
	var left int
	if err := db.QueryRow(`SELECT COUNT(*) FROM api_tokens`).Scan(&left); err != nil || left != 0 {
		t.Fatalf("tokens of a removed account were kept: %d %v", left, err)
	}
}
