package services

import (
	"testing"
)

// S4 R2a: the time and temporary-file limits of a database user, one key at
// a time (review finding 2: the panel never resets what it did not set).

func TestMariaDBStatementTimeLimitSQL(t *testing.T) {
	got, err := mariaDBStatementTimeSQL("'shop_user'@'localhost'", 300)
	if err != nil || got != "ALTER USER 'shop_user'@'localhost' WITH MAX_STATEMENT_TIME 300;" {
		t.Fatalf("%q %v", got, err)
	}
	if got, _ := mariaDBStatementTimeSQL("'shop_user'@'localhost'", 0); got != "ALTER USER 'shop_user'@'localhost' WITH MAX_STATEMENT_TIME 0;" {
		t.Fatalf("off: %q", got)
	}
	for _, bad := range []int{-1, MaxRoleLimitSeconds + 1} {
		if _, err := mariaDBStatementTimeSQL("'u'@'localhost'", bad); err == nil {
			t.Errorf("%d accepted", bad)
		}
	}
	for output, want := range map[string]string{"max_statement_time\n0.000000\n": "", "max_statement_time\n30.000000\n": "30s", "max_statement_time\n1.500000\n": "1.5s"} {
		if got, err := mariaDBStatementTimeValue(output); err != nil || got != want {
			t.Errorf("%q = %q %v, want %q", output, got, err, want)
		}
	}
	if _, err := mariaDBStatementTimeValue("max_statement_time\n"); err == nil {
		t.Error("a missing account read as no limit")
	}
}

func TestPostgreSQLRoleSettingSQLIsOneKey(t *testing.T) {
	for _, c := range []struct {
		key   string
		value int
		want  string
	}{
		{RoleKeyStatementSeconds, 300, `ALTER ROLE "shop_user" SET statement_timeout = '300s'`},
		{RoleKeyIdleInTransactionSeconds, 600, `ALTER ROLE "shop_user" SET idle_in_transaction_session_timeout = '600s'`},
		{RoleKeyTempFileMB, 2048, `ALTER ROLE "shop_user" SET temp_file_limit = '2048MB'`},
		{RoleKeyTempFileMB, 0, `ALTER ROLE "shop_user" RESET temp_file_limit`},
	} {
		if got, err := postgreSQLRoleSettingSQL("shop_user", c.key, c.value); err != nil || got != c.want {
			t.Errorf("%s=%d: %q %v", c.key, c.value, got, err)
		}
	}
	if _, err := postgreSQLRoleSettingSQL(`x"; DROP ROLE postgres; --`, RoleKeyTempFileMB, 1); err == nil {
		t.Fatal("an unsafe role name was written")
	}
	got := postgreSQLRoleSettingsFrom([]string{"statement_timeout=30s", "search_path=public", "temp_file_limit=2GB"})
	if got[RoleKeyStatementSeconds] != "30s" || got[RoleKeyTempFileMB] != "2GB" || got[RoleKeyIdleInTransactionSeconds] != "" {
		t.Fatalf("settings = %v", got)
	}
}
