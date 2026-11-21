package services

import (
	"strings"
	"testing"
)

// S4 R2a: the time and temporary-file limits of a database user.

func TestMariaDBStatementTimeLimitSQL(t *testing.T) {
	got, err := mariaDBRoleLimitSQL("'shop_user'@'localhost'", RoleLimits{StatementSeconds: 300, IdleInTransactionSeconds: 600, TempFileMB: 1024})
	if err != nil || got != "ALTER USER 'shop_user'@'localhost' WITH MAX_STATEMENT_TIME 300;" {
		t.Fatalf("%q %v", got, err)
	}
	// No limit removes an earlier one.
	if got, _ := mariaDBRoleLimitSQL("'shop_user'@'localhost'", RoleLimits{}); got != "ALTER USER 'shop_user'@'localhost' WITH MAX_STATEMENT_TIME 0;" {
		t.Fatalf("off: %q", got)
	}
	for _, bad := range []RoleLimits{{StatementSeconds: -1}, {StatementSeconds: MaxRoleLimitSeconds + 1}, {TempFileMB: MaxRoleTempFileMB + 1}} {
		if _, err := mariaDBRoleLimitSQL("'u'@'localhost'", bad); err == nil {
			t.Errorf("%+v accepted", bad)
		}
	}
}

func TestPostgreSQLRoleLimitsSQL(t *testing.T) {
	got, err := postgreSQLRoleLimitSQL("shop_user", RoleLimits{StatementSeconds: 300, IdleInTransactionSeconds: 600, TempFileMB: 2048})
	if err != nil || len(got) != 4 {
		t.Fatalf("%v %v", got, err)
	}
	want := []string{
		`ALTER ROLE "shop_user" SET statement_timeout = '300s'`,
		`ALTER ROLE "shop_user" SET idle_in_transaction_session_timeout = '600s'`,
		`ALTER ROLE "shop_user" SET temp_file_limit = '2048MB'`,
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("statement %d = %q, want %q", i, got[i], w)
		}
	}
	// The load's overrides are removed with every write, by the role's own
	// name and with every name schema-qualified.
	if !strings.Contains(got[3], "r.rolname OPERATOR(pg_catalog.=) 'shop_user'") || !strings.Contains(got[3], "IN DATABASE %I RESET temp_file_limit") ||
		strings.Contains(got[3], " LIKE ") {
		t.Fatalf("override reset: %s", got[3])
	}
	off, _ := postgreSQLRoleLimitSQL("shop_user", RoleLimits{})
	for i, w := range []string{`ALTER ROLE "shop_user" RESET statement_timeout`, `ALTER ROLE "shop_user" RESET idle_in_transaction_session_timeout`, `ALTER ROLE "shop_user" RESET temp_file_limit`} {
		if off[i] != w {
			t.Errorf("off %d = %q", i, off[i])
		}
	}
	if _, err := postgreSQLRoleLimitSQL(`x"; DROP ROLE postgres; --`, RoleLimits{}); err == nil {
		t.Fatal("an unsafe role name was written")
	}
}
