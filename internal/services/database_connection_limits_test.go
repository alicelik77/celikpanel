package services

import "testing"

func TestConnectionLimitStatements(t *testing.T) {
	got, err := mariaDBConnectionLimitSQL("sub12_app", 25)
	if err != nil || got != "ALTER USER 'sub12_app'@'localhost' WITH MAX_USER_CONNECTIONS 25;" {
		t.Fatalf("MariaDB: %q %v", got, err)
	}
	if got, err := mariaDBConnectionLimitSQL("sub12_app", 0); err != nil || got != "ALTER USER 'sub12_app'@'localhost' WITH MAX_USER_CONNECTIONS 0;" {
		t.Fatalf("MariaDB no limit: %q %v", got, err)
	}
	got, err = postgreSQLConnectionLimitSQL("sub12_app", 25)
	if err != nil || got != `ALTER ROLE "sub12_app" CONNECTION LIMIT 25;` {
		t.Fatalf("PostgreSQL: %q %v", got, err)
	}
	if got, err := postgreSQLConnectionLimitSQL("sub12_app", 0); err != nil || got != `ALTER ROLE "sub12_app" CONNECTION LIMIT -1;` {
		t.Fatalf("PostgreSQL no limit: %q %v", got, err)
	}
	for _, bad := range []struct {
		user  string
		limit int
	}{{"x'; DROP USER root; --", 5}, {"ok_user", -1}, {"ok_user", MaxUserConnections + 1}} {
		if _, err := mariaDBConnectionLimitSQL(bad.user, bad.limit); err == nil {
			t.Errorf("MariaDB accepted %+v", bad)
		}
		if _, err := postgreSQLConnectionLimitSQL(bad.user, bad.limit); err == nil && bad.limit != 5 {
			t.Errorf("PostgreSQL accepted %+v", bad)
		}
	}
}
