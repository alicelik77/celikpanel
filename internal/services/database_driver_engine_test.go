package services

import (
	"errors"
	"os"
	"testing"
)

// The panel-side drivers against a private MariaDB and PostgreSQL that the
// test environment started (alpha.129; see cmd/agent's
// database_access_users_engine_test.go): creation is exclusive and a name
// the engine has already is ErrDatabaseNameTaken, never taken over.
// CELIKPANEL_TEST_MARIADB_PORT / CELIKPANEL_TEST_PG_PORT name the TCP ports
// (127.0.0.1); the PostgreSQL superuser's password is
// CELIKPANEL_TEST_PG_PASSWORD; the mariadb client must be on PATH as mysql.

func TestMariaDBDriverCreatesExclusivelyOnARealEngine(t *testing.T) {
	port := os.Getenv("CELIKPANEL_TEST_MARIADB_PORT")
	if port == "" {
		t.Skip("CELIKPANEL_TEST_MARIADB_PORT is not set: no private MariaDB")
	}
	driver := NewMariaDBDriver(DriverConfig{Host: "127.0.0.1", Port: atoiForTest(t, port), Username: "root", Password: os.Getenv("CELIKPANEL_TEST_MARIADB_PASSWORD")})
	_ = driver.DeleteDatabase("a129names")
	_ = driver.DeleteUser("a129names_user")
	t.Cleanup(func() { _ = driver.DeleteDatabase("a129names"); _ = driver.DeleteUser("a129names_user") })
	if err := driver.CreateDatabase("a129names"); err != nil {
		t.Fatal(err)
	}
	if err := driver.CreateDatabase("a129names"); !errors.Is(err, ErrDatabaseNameTaken) {
		t.Fatalf("second create = %v, want ErrDatabaseNameTaken", err)
	}
	if err := driver.CreateUser("a129names_user", "First-Passw0rd!x"); err != nil {
		t.Fatal(err)
	}
	if err := driver.CreateUser("a129names_user", "Other-Passw0rd!x"); !errors.Is(err, ErrDatabaseNameTaken) {
		t.Fatalf("second user create = %v, want ErrDatabaseNameTaken", err)
	}
}

func TestPostgreSQLDriverCreatesExclusivelyOnARealEngine(t *testing.T) {
	port := os.Getenv("CELIKPANEL_TEST_PG_PORT")
	if port == "" || os.Getenv("CELIKPANEL_TEST_PG_PASSWORD") == "" {
		t.Skip("CELIKPANEL_TEST_PG_PORT/PASSWORD are not set: no private PostgreSQL")
	}
	driver := NewPostgreSQLDriver(DriverConfig{Host: "127.0.0.1", Port: atoiForTest(t, port), Username: "postgres", Password: os.Getenv("CELIKPANEL_TEST_PG_PASSWORD")})
	_ = driver.DeleteDatabase("a129names")
	_ = driver.DeleteUser("a129names_user")
	t.Cleanup(func() { _ = driver.DeleteDatabase("a129names"); _ = driver.DeleteUser("a129names_user") })
	if err := driver.CreateDatabase("a129names"); err != nil {
		t.Fatal(err)
	}
	if err := driver.CreateDatabase("a129names"); !errors.Is(err, ErrDatabaseNameTaken) {
		t.Fatalf("second create = %v, want ErrDatabaseNameTaken", err)
	}
	if err := driver.CreateUser("a129names_user", "First-Passw0rd!x"); err != nil {
		t.Fatal(err)
	}
	if err := driver.CreateUser("a129names_user", "Other-Passw0rd!x"); !errors.Is(err, ErrDatabaseNameTaken) {
		t.Fatalf("second user create = %v, want ErrDatabaseNameTaken", err)
	}
}

func atoiForTest(t *testing.T, text string) int {
	t.Helper()
	value := 0
	for _, c := range text {
		if c < '0' || c > '9' {
			t.Fatalf("not a port: %q", text)
		}
		value = value*10 + int(c-'0')
	}
	return value
}
