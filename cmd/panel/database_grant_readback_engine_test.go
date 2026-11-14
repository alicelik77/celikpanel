package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/services"
)

// alpha.134 follow-up, on a real MariaDB (CELIKPANEL_TEST_MARIADB_PORT and
// CELIKPANEL_TEST_MARIADB_PASSWORD, the private engine of
// internal/services/database_driver_engine_test.go; skipped without one).
//
// A registered MariaDB whose account may create databases and users and give
// rights but may not read mysql.db: nothing that worked on alpha.133 is
// refused because of the read-back. The right is written by the exact name
// (the length rule is checked first), the engine's state records once that
// rights there cannot be verified, and a later verified read-back clears it.

const readBackAccount = "a134reg"
const readBackPassword = "Reg-Account-Passw0rd!7"

func readBackEngine(t *testing.T) (port int, root *services.MariaDBDriver) {
	t.Helper()
	raw := os.Getenv("CELIKPANEL_TEST_MARIADB_PORT")
	if raw == "" || os.Getenv("CELIKPANEL_TEST_MARIADB_PASSWORD") == "" {
		t.Skip("no private MariaDB (CELIKPANEL_TEST_MARIADB_PORT)")
	}
	port, err := strconv.Atoi(raw)
	if err != nil {
		t.Fatal(err)
	}
	return port, services.NewMariaDBDriver(services.DriverConfig{Host: "127.0.0.1", Port: port, Username: readBackSuperuser(), Password: os.Getenv("CELIKPANEL_TEST_MARIADB_PASSWORD")})
}

// readBackSuperuser is the private engine's superuser (CELIKPANEL_TEST_MARIADB_USER, root when unset).
func readBackSuperuser() string {
	if user := os.Getenv("CELIKPANEL_TEST_MARIADB_USER"); user != "" {
		return user
	}
	return "root"
}

// rootSQL runs a statement as the engine's superuser and answers its output.
func rootSQL(t *testing.T, root *services.MariaDBDriver, statement string) string {
	t.Helper()
	out, err := root.GrantExec()(statement)
	if err != nil {
		t.Fatalf("%s: %v", statement, err)
	}
	return string(out)
}

func readBackMarker(t *testing.T, root *services.MariaDBDriver, statement string) string {
	t.Helper()
	for _, line := range strings.Split(rootSQL(t, root, statement), "\n") {
		if value, ok := strings.CutPrefix(strings.TrimSpace(line), "n="); ok {
			return value
		}
	}
	t.Fatalf("no answer to %s", statement)
	return ""
}

func TestRegisteredMariaDBWithoutReadRightGrantsAndRecordsUnverifiable(t *testing.T) {
	port, root := readBackEngine(t)
	long := "s9201_" + strings.Repeat("l_", 28)
	cleanup := "DROP DATABASE IF EXISTS s9201_regdb; DROP DATABASE IF EXISTS s9201_regdb2; DROP DATABASE IF EXISTS s9201_rootdb; DROP DATABASE IF EXISTS `" + long + "`;" +
		" DROP USER IF EXISTS '" + readBackAccount + "'@'%', 's9201_reg_u'@'localhost', 's9201_reg_v'@'localhost', 's9201_root_u'@'localhost', 's9201_long_u'@'localhost';"
	rootSQL(t, root, cleanup)
	t.Cleanup(func() { _, _ = root.GrantExec()(cleanup) })
	// A restricted account as an owner registers it: rights on its own
	// databases with GRANT OPTION, CREATE USER and RELOAD, and only
	// mysql.user readable - mysql.db is not.
	rootSQL(t, root, "CREATE USER '"+readBackAccount+"'@'%' IDENTIFIED BY '"+readBackPassword+"';"+
		" GRANT ALL PRIVILEGES ON `s9201\\_%`.* TO '"+readBackAccount+"'@'%' WITH GRANT OPTION;"+
		" GRANT CREATE USER, RELOAD ON *.* TO '"+readBackAccount+"'@'%';"+
		" GRANT SELECT ON mysql.user TO '"+readBackAccount+"'@'%';")
	restricted := services.NewMariaDBDriver(services.DriverConfig{Host: "127.0.0.1", Port: port, Username: readBackAccount, Password: readBackPassword})
	if _, err := restricted.GrantExec()("SELECT CONCAT('n=', COUNT(*)) FROM mysql.db;"); err == nil {
		t.Fatal("fixture: the restricted account reads mysql.db")
	}

	fixture := newDatabaseV2SecurityFixture(t)
	sealed, err := fixture.box.Encrypt(readBackPassword)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.sql.Exec(`UPDATE database_servers SET host = '127.0.0.1', port = ?, admin_username = ?, root_password_encrypted = ? WHERE id = ?`,
		port, readBackAccount, sealed, dbSecurityServer); err != nil {
		t.Fatal(err)
	}
	unverifiable := func() sql.NullString {
		t.Helper()
		var since sql.NullString
		err := fixture.sql.QueryRow(`SELECT grants_unverifiable_since FROM database_grant_form_engines WHERE server_id = ?`, dbSecurityServer).Scan(&since)
		if err != nil && err != sql.ErrNoRows {
			t.Fatal(err)
		}
		return since
	}
	create := func(name, user, password string) *httptest.ResponseRecorder {
		t.Helper()
		recorder := httptest.NewRecorder()
		fixture.panel.handleCreateDatabaseV2(recorder, fixture.request(fmt.Sprintf(
			`{"database_name":%q,"domain_id":%d,"new_username":%q,"new_password":%q}`, name, dbSecurityDomain, user, password)))
		return recorder
	}

	// 1. Creating a database with a new user works as on alpha.133.
	if answer := create("regdb", "reg_u", "Reg-User-Passw0rd!1"); answer.Code != http.StatusOK {
		t.Fatalf("create on a restricted account: %d %s", answer.Code, answer.Body.String())
	}
	// The right is stored by the exact name, whole.
	if n := readBackMarker(t, root, "SELECT CONCAT('n=', COUNT(*)) FROM mysql.db WHERE User = 's9201_reg_u' AND Db = CONCAT('s9201', CHAR(92), '_regdb');"); n != "1" {
		t.Fatalf("exact right stored: %s", n)
	}
	// 2. Recorded once; a second grant keeps the first time.
	first := unverifiable()
	if !first.Valid {
		t.Fatal("the unverifiable read-back was not recorded")
	}
	if _, err := fixture.sql.Exec(`UPDATE database_grant_form_engines SET grants_unverifiable_since = '2026-01-01 00:00:00' WHERE server_id = ?`, dbSecurityServer); err != nil {
		t.Fatal(err)
	}
	if answer := create("regdb2", "reg_v", "Reg-User-Passw0rd!2"); answer.Code != http.StatusOK {
		t.Fatalf("second create: %d %s", answer.Code, answer.Body.String())
	}
	if again := unverifiable(); again.String != "2026-01-01 00:00:00" {
		t.Fatalf("recorded again per operation: %+v", again)
	}
	// The MariaDB page says it.
	view, err := fixture.panel.databaseGrantFormsView(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	// Only the registered engine's own state carries it.
	for _, engine := range view.Engines {
		want := ""
		if engine.Name == "owner-logical-server" {
			want = "2026-01-01 00:00:00"
		}
		if engine.GrantsUnverifiableSince != want {
			t.Fatalf("view: %+v", view.Engines)
		}
	}

	// 3. With an account that reads mysql.db the read-back is verified and the
	// note goes.
	rootSecret, _ := fixture.box.Encrypt(os.Getenv("CELIKPANEL_TEST_MARIADB_PASSWORD"))
	if _, err := fixture.sql.Exec(`UPDATE database_servers SET admin_username = ?, root_password_encrypted = ? WHERE id = ?`, readBackSuperuser(), rootSecret, dbSecurityServer); err != nil {
		t.Fatal(err)
	}
	if answer := create("rootdb", "root_u", "Root-User-Passw0rd!5"); answer.Code != http.StatusOK {
		t.Fatalf("create as root: %d %s", answer.Code, answer.Body.String())
	}
	if since := unverifiable(); since.Valid {
		t.Fatalf("a verified read-back left the note: %+v", since)
	}
	// 4. An existing database whose exact form does not fit keeps working: a
	// user given to it gets the pattern form, stored whole, never cut. (With
	// the restricted account the engine refuses a pattern-form GRANT, ERROR
	// 1044 - measured; alpha.133 wrote every right in that form, so such an
	// account could give none there. The exact form it can give.)
	rootSQL(t, root, "CREATE DATABASE `"+long+"`;")
	var databaseID int64
	result, err := fixture.sql.Exec(`INSERT INTO databases_v2 (server_id, subscription_id, domain_id, name) VALUES (?, ?, ?, ?)`, dbSecurityServer, dbSecuritySubscription, dbSecurityDomain, long)
	if err == nil {
		databaseID, err = result.LastInsertId()
	}
	if err != nil {
		t.Fatal(err)
	}
	rootSQL(t, root, "CREATE USER 's9201_long_u'@'localhost' IDENTIFIED BY 'Long-User-Passw0rd!3';")
	userSecret, _ := fixture.box.Encrypt("Long-User-Passw0rd!3")
	result, err = fixture.sql.Exec(`INSERT INTO database_users (server_id, subscription_id, username, password) VALUES (?, ?, 's9201_long_u', ?)`, dbSecurityServer, dbSecuritySubscription, userSecret)
	var userID int64
	if err == nil {
		userID, err = result.LastInsertId()
	}
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	fixture.panel.handleGrantDatabaseAccess(recorder, fixture.requestAsOwner(http.MethodPost, fmt.Sprintf(`/api/v1/databases/%d/grants`, databaseID), fmt.Sprintf(`{"user_id":%d}`, userID)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("grant on an existing long name: %d %s", recorder.Code, recorder.Body.String())
	}
	if n := readBackMarker(t, root, "SELECT CONCAT('n=', COUNT(*)) FROM mysql.db WHERE User = 's9201_long_u' AND Db = '"+long+"';"); n != "1" {
		t.Fatalf("pattern right stored whole: %s", n)
	}
	if n := readBackMarker(t, root, "SELECT CONCAT('n=', COUNT(*)) FROM mysql.db WHERE User = 's9201_long_u' AND Db <> '"+long+"';"); n != "0" {
		t.Fatalf("a right on another (cut) name: %s", n)
	}
	// Second review: that pattern-form right is listed on the MariaDB page at
	// once, with its reason.
	var listed int
	if err := fixture.sql.QueryRow(`SELECT COUNT(*) FROM database_grant_forms WHERE username = 's9201_long_u' AND stored_name = ? AND kind = 'ours' AND state = 'left' AND reason = ?`,
		long, grantFormsReasonNameTooLong).Scan(&listed); err != nil || listed != 1 {
		t.Fatalf("the pattern-form right is not listed: %d %v", listed, err)
	}
	// A NEW database of such a name is refused before anything is made, with
	// what to do and by how much.
	answer := create(strings.Repeat("n_", 28), "long_w", "Long-User-Passw0rd!4")
	if answer.Code != http.StatusConflict || !strings.Contains(answer.Body.String(), errCodeDatabaseNameTooLong) {
		t.Fatalf("a too long new name: %d %s", answer.Code, answer.Body.String())
	}
	var refusal apiErrorBody
	_ = json.Unmarshal(answer.Body.Bytes(), &refusal)
	if refusal.Vars["over"] == "" || !strings.Contains(refusal.Error, "shorter") {
		t.Fatalf("refusal: %+v", refusal)
	}

}
