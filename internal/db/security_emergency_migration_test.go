package db

import (
	"path/filepath"
	"testing"
)

// Migration 55 (alpha.117, D-043): the security emergency record, its
// operations with checkpoints and before-images, the affected sites and the
// notices. Additive: six new tables and their indexes; nothing existing
// changes. These tests prove the guards the code relies on: one open
// emergency per target, one running operation per emergency, one notice per
// emergency, person and kind, and the closed value sets.
const securityEmergencyMigrationVersion = 55

func TestSecurityEmergencySchemaGuards(t *testing.T) {
	database, err := NewSQLiteDB(filepath.Join(t.TempDir(), "panel.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	raw := database.GetDB()
	mustExec := func(query string, args ...any) {
		t.Helper()
		if _, err := raw.Exec(query, args...); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
	}
	mustFail := func(why, query string, args ...any) {
		t.Helper()
		if _, err := raw.Exec(query, args...); err == nil {
			t.Fatalf("%s was accepted: %s", why, query)
		}
	}
	var version int
	if err := raw.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != newestMigrationVersion {
		t.Fatalf("ledger at %d, want the newest %d", version, newestMigrationVersion)
	}

	mustExec(`INSERT INTO security_emergencies (id, component, version, reason) VALUES ('emergency-000001', 'php', '8.5', 'CVE-2026-0001')`)
	mustFail("a second open emergency for the same target",
		`INSERT INTO security_emergencies (id, component, version, reason) VALUES ('emergency-000002', 'php', '8.5', 'again')`)
	mustFail("an unknown component", `INSERT INTO security_emergencies (id, component, version, reason) VALUES ('emergency-000003', 'nginx', '', 'x')`)
	mustFail("an empty reason", `INSERT INTO security_emergencies (id, component, version, reason) VALUES ('emergency-000004', 'node', '22.1.0', '')`)
	mustFail("an unknown stage", `UPDATE security_emergencies SET stage = 'paused' WHERE id = 'emergency-000001'`)
	// Once resolved, a new emergency for the same target may open.
	mustExec(`UPDATE security_emergencies SET stage = 'resolved' WHERE id = 'emergency-000001'`)
	mustExec(`INSERT INTO security_emergencies (id, component, version, reason) VALUES ('emergency-000002', 'php', '8.5', 'again')`)

	mustExec(`INSERT INTO security_emergency_operations (id, emergency_id, kind, reason) VALUES ('operation-0000001', 'emergency-000002', 'stop', 'CVE')`)
	mustFail("a second running operation",
		`INSERT INTO security_emergency_operations (id, emergency_id, kind, reason) VALUES ('operation-0000002', 'emergency-000002', 'remove', 'CVE')`)
	mustFail("an unknown operation kind",
		`INSERT INTO security_emergency_operations (id, emergency_id, kind, reason, state) VALUES ('operation-0000003', 'emergency-000002', 'delete', 'x', 'completed')`)
	mustExec(`UPDATE security_emergency_operations SET state = 'partial' WHERE id = 'operation-0000001'`)

	mustExec(`INSERT INTO security_emergency_effects (operation_id, seq, kind, target, ref) VALUES ('operation-0000001', 0, 'units', 'php-fpm:8.5', '{"service":"php-fpm"}')`)
	mustFail("an effect reference that is not JSON",
		`INSERT INTO security_emergency_effects (operation_id, seq, kind, target, ref) VALUES ('operation-0000001', 1, 'site', 'a.test', 'x')`)
	mustFail("an unknown effect kind",
		`INSERT INTO security_emergency_effects (operation_id, seq, kind, target) VALUES ('operation-0000001', 2, 'database', 'a.test')`)

	mustExec(`INSERT INTO security_emergency_sites (emergency_id, domain_id, site_id, subscription_id, domain) VALUES ('emergency-000002', 1, 1, 1, 'a.test')`)
	mustFail("a maintenance flag that is not 0 or 1",
		`UPDATE security_emergency_sites SET maintenance = 2 WHERE domain_id = 1`)
	mustExec(`INSERT INTO security_emergency_before_images (emergency_id, kind, target, before) VALUES ('emergency-000002', 'units', 'php-fpm:8.5', '{}')`)
	mustFail("a before-image that is not JSON",
		`INSERT INTO security_emergency_before_images (emergency_id, kind, target, before) VALUES ('emergency-000002', 'app', 'a.test', 'x')`)

	mustExec(`INSERT INTO security_notices (emergency_id, user_id, kind, audience) VALUES ('emergency-000002', 7, 'emergency', 'customer')`)
	mustFail("a second notice of the same kind to the same person",
		`INSERT INTO security_notices (emergency_id, user_id, kind, audience) VALUES ('emergency-000002', 7, 'emergency', 'reseller')`)
	mustExec(`INSERT INTO security_notices (emergency_id, user_id, kind, audience) VALUES ('emergency-000002', 7, 'resolved', 'customer')`)
	// "Your site was moved" (alpha.117 release decision d).
	mustExec(`INSERT INTO security_notices (emergency_id, user_id, kind, audience) VALUES ('emergency-000002', 7, 'moved', 'customer')`)
	mustFail("an unknown notice kind",
		`INSERT INTO security_notices (emergency_id, user_id, kind, audience) VALUES ('emergency-000002', 8, 'deleted', 'customer')`)
	mustFail("an unknown mail state", `UPDATE security_notices SET mail_state = 'bounced'`)
}
