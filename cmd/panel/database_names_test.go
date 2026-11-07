package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/services"
)

// alpha.129: names are unique per engine, not per account
// (database_names.go).

func TestCreateDatabaseV2RefusesANameAnotherAccountHasOnTheSameEngine(t *testing.T) {
	fixture := newDatabaseV2SecurityFixture(t)
	driver := useRecordingDatabaseDriver(t)
	// The other subscription's record of the same engine (127.0.0.1:3306,
	// MariaDB) already holds the name this request would make: a domain
	// database of a domain named like "s9201.shop" would.
	if _, err := fixture.sql.Exec(`INSERT INTO databases_v2 (id, server_id, subscription_id, name) VALUES (9611, 9302, 9202, 's9201_shop')`); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	fixture.panel.handleCreateDatabaseV2(recorder, fixture.request(`{"database_name":"shop","new_username":"shop","new_password":"New-clear-secret1"}`))
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), errCodeDatabaseNameTaken) {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if driver.factoryCalls != 0 || len(driver.events) != 0 {
		t.Fatalf("the engine was reached: %v", driver.events)
	}

	// The same for a user name another account has.
	if _, err := fixture.sql.Exec(`INSERT INTO database_users (id, server_id, subscription_id, username, password) VALUES (9612, 9302, 9202, 's9201_taken', 'x')`); err != nil {
		t.Fatal(err)
	}
	recorder = httptest.NewRecorder()
	fixture.panel.handleCreateDatabaseV2(recorder, fixture.request(`{"database_name":"fresh","new_username":"taken","new_password":"New-clear-secret1"}`))
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), "database user named s9201_taken") {
		t.Fatalf("user: status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if len(driver.events) != 0 {
		t.Fatalf("the engine was reached: %v", driver.events)
	}
}

func TestCreateDatabaseV2NeverTakesOverWhatTheEngineAlreadyHas(t *testing.T) {
	fixture := newDatabaseV2SecurityFixture(t)
	driver := useRecordingDatabaseDriver(t)
	// A database made outside CelikPanel: the engine says the name is taken;
	// nothing is granted and nothing is recorded.
	driver.createDatabaseErr = fmt.Errorf("database s9201_outside: %w", services.ErrDatabaseNameTaken)
	before := countDatabaseSecurityRows(t, fixture.sql)
	recorder := httptest.NewRecorder()
	fixture.panel.handleCreateDatabaseV2(recorder, fixture.request(`{"database_name":"outside","new_username":"outside","new_password":"New-clear-secret1"}`))
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), errCodeDatabaseNameTaken) {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	requireDatabaseDriverEvents(t, driver, "create-database:s9201_outside")
	if after := countDatabaseSecurityRows(t, fixture.sql); after != before {
		t.Fatalf("rows %v -> %v", before, after)
	}

	// A user made outside CelikPanel: the database this request made is
	// dropped again, the existing user is never granted anything.
	driver.createDatabaseErr = nil
	driver.events = nil
	driver.createUserErr = fmt.Errorf("database user s9201_outside: %w", services.ErrDatabaseNameTaken)
	recorder = httptest.NewRecorder()
	fixture.panel.handleCreateDatabaseV2(recorder, fixture.request(`{"database_name":"outside","new_username":"outside","new_password":"New-clear-secret1"}`))
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), "database user named s9201_outside") {
		t.Fatalf("user: status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	requireDatabaseDriverEvents(t, driver, "create-database:s9201_outside", "create-user:s9201_outside", "delete-database:s9201_outside")
	if after := countDatabaseSecurityRows(t, fixture.sql); after != before {
		t.Fatalf("rows %v -> %v", before, after)
	}
}

func TestDatabaseNamesMayRepeatAcrossEngines(t *testing.T) {
	fixture := newDatabaseV2SecurityFixture(t)
	// A PostgreSQL record of the same account (type 1) holds the name; the
	// MariaDB create of that name is not refused (owner, 7 Nov 2026).
	if _, err := fixture.sql.Exec(`INSERT INTO database_servers (id, subscription_id, type_id, name, version, host, port, status) VALUES (9303, 9201, 1, 'pg', '17', '127.0.0.1', 5432, 'active');
		INSERT INTO databases_v2 (id, server_id, subscription_id, name) VALUES (9613, 9303, 9201, 's9201_same')`); err != nil {
		t.Fatal(err)
	}
	what, err := fixture.panel.databaseNameTakenOnEngine(t.Context(), dbSecurityServer, "s9201_same", "", false)
	if err != nil || what != "" {
		t.Fatalf("MariaDB name refused because PostgreSQL has it: %q %v", what, err)
	}
	what, err = fixture.panel.databaseNameTakenOnEngine(t.Context(), 9303, "s9201_same", "", false)
	if err != nil || what != "database" {
		t.Fatalf("PostgreSQL record: %q %v", what, err)
	}
	// localhost and 127.0.0.1 are one engine.
	if _, err := fixture.sql.Exec(`UPDATE database_servers SET host = 'localhost' WHERE id = 9302;
		INSERT INTO database_users (id, server_id, subscription_id, username, password) VALUES (9614, 9302, 9202, 'shared_name', 'x')`); err != nil {
		t.Fatal(err)
	}
	if what, err := fixture.panel.databaseNameTakenOnEngine(t.Context(), dbSecurityServer, "", "shared_name", false); err != nil || what != "user" {
		t.Fatalf("localhost/127.0.0.1: %q %v", what, err)
	}
	// The domain create reuses its own record's user, never another's.
	if what, err := fixture.panel.databaseNameTakenOnEngine(t.Context(), 9302, "", "shared_name", true); err != nil || what != "" {
		t.Fatalf("own user: %q %v", what, err)
	}
	if what, err := fixture.panel.databaseNameTakenOnEngine(t.Context(), dbSecurityServer, "", "shared_name", true); err != nil || what != "user" {
		t.Fatalf("another record's user: %q %v", what, err)
	}
}

func TestDatabaseExportFileNameCarriesTheEngine(t *testing.T) {
	at := time.Date(2026, 11, 7, 14, 5, 0, 0, time.UTC)
	maria := databaseExportFilename("bioindex_net_kapsora", "mariadb", at)
	pg := databaseExportFilename("bioindex_net_kapsora", "postgresql", at)
	if maria != "bioindex_net_kapsora-mariadb-20261107-1405.sql.gz" || pg != "bioindex_net_kapsora-postgresql-20261107-1405.sql.gz" {
		t.Fatalf("%s / %s", maria, pg)
	}
}
