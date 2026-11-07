package db

import (
	"path/filepath"
	"strings"
	"testing"
)

// Migration 72 (alpha.128, workstream "ops"; 69 on its branch): sessions gain three NULL
// columns; notification_preferences, account_notices and
// certificate_requests are new; audit_logs gains two indexes. A release
// before it refuses a ledger that carries it; going back is the pre-update
// snapshot restore (D-025). The integrator may renumber it.
const panelOperationsMigrationVersion = 72

func TestOlderReleaseRefusesALedgerWithThePanelOperationsEntry(t *testing.T) {
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
		if migration.version < panelOperationsMigrationVersion {
			older = append(older, migration)
		}
	}
	err = database.verifyMigrationLedgerCoverage(t.Context(), older)
	database.Close()
	if err == nil || !strings.Contains(err.Error(), "no matching embedded migration") {
		t.Fatalf("an older release accepted the newer ledger: %v", err)
	}
}

func TestPanelOperationsMigrationKeepsOldSessionsAndChecksItsRows(t *testing.T) {
	database, err := NewSQLiteDB(filepath.Join(t.TempDir(), "panel.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	db := database.GetDB()
	if _, err := db.Exec(`INSERT INTO users (id, username, email, password_hash, role) VALUES (7, 'u7', 'u7@example.com', 'x', 'customer')`); err != nil {
		t.Fatal(err)
	}
	// An older release's session insert still works and leaves the new columns NULL.
	if _, err := db.Exec(`INSERT INTO sessions (token_hash, user_id, expires_at) VALUES ('h1', 7, '2030-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	var allNull bool
	if err := db.QueryRow(`SELECT last_seen_at IS NULL AND client_ip IS NULL AND user_agent IS NULL FROM sessions`).Scan(&allNull); err != nil || !allNull {
		t.Fatalf("an old-style session did not keep the new columns NULL: %v %v", allNull, err)
	}
	good := []string{
		`INSERT INTO notification_preferences (user_id) VALUES (7)`,
		`INSERT INTO account_notices (user_id, audience, kind, needs_action, subject_key, period, fix_path) VALUES (7, 'customer', 'expiry_soon', 1, 'subscription:1', '2027-01-31', '/account')`,
		`INSERT INTO certificate_requests (common_name, key_type, csr_pem, public_key_sha256, key_file) VALUES ('example.com', 'ecdsa-p256', '` + strings.Repeat("A", 120) + `', '` + strings.Repeat("a", 64) + `', '/etc/celikpanel/csr/1.key')`,
	}
	for _, statement := range good {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
	bad := []string{
		`INSERT INTO sessions (token_hash, user_id, expires_at, user_agent) VALUES ('h2', 7, 'x', '` + strings.Repeat("a", 201) + `')`,
		`UPDATE notification_preferences SET mode = 'some' WHERE user_id = 7`,
		// The same stage twice.
		`INSERT INTO account_notices (user_id, audience, kind, subject_key, period) VALUES (7, 'customer', 'expiry_soon', 'subscription:1', '2027-01-31')`,
		// A fix link that leaves the panel.
		`INSERT INTO account_notices (user_id, audience, kind, period, fix_path) VALUES (7, 'customer', 'x', 'p', 'https://example.com/')`,
		`INSERT INTO account_notices (user_id, audience, kind, period, fix_path) VALUES (7, 'customer', 'x', 'p2', '//example.com/')`,
		`INSERT INTO account_notices (user_id, audience, kind, period, mail_state) VALUES (7, 'customer', 'x', 'p3', 'queued')`,
		`INSERT INTO certificate_requests (common_name, key_type, csr_pem, public_key_sha256, key_file) VALUES ('example.com', 'dsa', '` + strings.Repeat("A", 120) + `', '` + strings.Repeat("a", 64) + `', '/k')`,
		`INSERT INTO certificate_requests (common_name, key_type, csr_pem, public_key_sha256, key_file, state) VALUES ('example.com', 'rsa-2048', '` + strings.Repeat("A", 120) + `', '` + strings.Repeat("a", 64) + `', '/k', 'installed')`,
	}
	for _, statement := range bad {
		if _, err := db.Exec(statement); err == nil {
			t.Fatalf("accepted: %s", statement)
		}
	}
	// Deleting the person removes their preference and notices.
	if _, err := db.Exec(`DELETE FROM users WHERE id = 7`); err != nil {
		t.Fatal(err)
	}
	var left int
	if err := db.QueryRow(`SELECT (SELECT COUNT(*) FROM notification_preferences) + (SELECT COUNT(*) FROM account_notices)`).Scan(&left); err != nil || left != 0 {
		t.Fatalf("rows left after the user was deleted: %d %v", left, err)
	}
}
