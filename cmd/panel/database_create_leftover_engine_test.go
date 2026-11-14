package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicelik/celikpanel/internal/services"
)

// alpha.134 fix round, defect 3, on a real MariaDB (the private engine of
// database_grant_readback_engine_test.go; skipped without one). A registered
// server's restricted account: a create that fails after the engine made the
// database and the user removes exactly what it made, and when its clean-up
// cannot finish the answer names what is left. The alpha.133 lab (P.6) saw
// the account an account without RELOAD had made stay behind: CREATE USER
// went through, the FLUSH PRIVILEGES after it was refused, and the clean-up
// took the user for never made.

const leftoverAccountPassword = "Left-Account-Passw0rd!7"

// leftoverFixture points the fixture's server record at the private engine
// with a restricted account made from grants.
func leftoverFixture(t *testing.T, account, grants string) (databaseV2SecurityFixture, *services.MariaDBDriver, func(name, user string) *httptest.ResponseRecorder) {
	t.Helper()
	port, root := readBackEngine(t)
	cleanup := "DROP DATABASE IF EXISTS s9201_leftdb; DROP DATABASE IF EXISTS s9201_keepdb;" +
		" DROP USER IF EXISTS '" + account + "'@'%', 's9201_left_u'@'localhost', 's9201_keep_u'@'localhost';"
	rootSQL(t, root, cleanup)
	t.Cleanup(func() { _, _ = root.GrantExec()(cleanup) })
	rootSQL(t, root, "CREATE USER '"+account+"'@'%' IDENTIFIED BY '"+leftoverAccountPassword+"'; "+grants)
	fixture := newDatabaseV2SecurityFixture(t)
	sealed, err := fixture.box.Encrypt(leftoverAccountPassword)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.sql.Exec(`UPDATE database_servers SET host = '127.0.0.1', port = ?, admin_username = ?, root_password_encrypted = ? WHERE id = ?`,
		port, account, sealed, dbSecurityServer); err != nil {
		t.Fatal(err)
	}
	create := func(name, user string) *httptest.ResponseRecorder {
		t.Helper()
		recorder := httptest.NewRecorder()
		fixture.panel.handleCreateDatabaseV2(recorder, fixture.request(fmt.Sprintf(
			`{"database_name":%q,"new_username":%q,"new_password":"Left-User-Passw0rd!1"}`, name, user)))
		return recorder
	}
	return fixture, root, create
}

func leftoverCount(t *testing.T, root *services.MariaDBDriver, statement string) string {
	t.Helper()
	return readBackMarker(t, root, statement)
}

// The P.6 account: rights on its own databases without GRANT OPTION, CREATE
// USER, mysql.user readable, no RELOAD. The GRANT is refused (ERROR 1044)
// after the database and the user were made; both go again, nothing of
// anyone else's is touched, and nothing is reported left.
func TestFailedCreateOnRestrictedMariaDBRemovesItsOwnUserOnRealEngine(t *testing.T) {
	_, root, create := leftoverFixture(t, "a134left",
		"GRANT ALL PRIVILEGES ON `s9201\\_%`.* TO 'a134left'@'%'; GRANT CREATE USER ON *.* TO 'a134left'@'%'; GRANT SELECT ON mysql.user TO 'a134left'@'%';")
	// Someone else's account on the same engine stays whatever happens.
	rootSQL(t, root, "CREATE USER IF NOT EXISTS 's9201_keep_u'@'localhost' IDENTIFIED BY 'Other-Passw0rd!2';")

	answer := create("leftdb", "left_u")
	if answer.Code == http.StatusOK {
		t.Fatalf("the create went through: %s", answer.Body.String())
	}
	var refusal apiErrorBody
	_ = json.Unmarshal(answer.Body.Bytes(), &refusal)
	if refusal.Code == errCodeDatabaseCreateLeftOver {
		t.Fatalf("reported something left although the clean-up finished: %s", answer.Body.String())
	}
	if n := leftoverCount(t, root, "SELECT CONCAT('n=', COUNT(*)) FROM mysql.user WHERE User = 's9201_left_u';"); n != "0" {
		t.Fatalf("the user this create made stayed on the engine (%s)", n)
	}
	if n := leftoverCount(t, root, "SELECT CONCAT('n=', COUNT(*)) FROM information_schema.SCHEMATA WHERE SCHEMA_NAME = 's9201_leftdb';"); n != "0" {
		t.Fatalf("the database this create made stayed on the engine (%s)", n)
	}
	if n := leftoverCount(t, root, "SELECT CONCAT('n=', COUNT(*)) FROM mysql.user WHERE User = 's9201_keep_u';"); n != "1" {
		t.Fatalf("an account this create did not make was touched (%s)", n)
	}
	// Pressing again changes nothing else and is answered the same way: the
	// names are free again, so it is not refused as taken.
	again := create("leftdb", "left_u")
	refusal = apiErrorBody{}
	_ = json.Unmarshal(again.Body.Bytes(), &refusal)
	if again.Code == http.StatusOK || refusal.Code == errCodeDatabaseNameTaken || refusal.Code == errCodeDatabaseCreateLeftOver {
		t.Fatalf("second press: %d %s", again.Code, again.Body.String())
	}
	if n := leftoverCount(t, root, "SELECT CONCAT('n=', COUNT(*)) FROM mysql.user WHERE User = 's9201_left_u';"); n != "0" {
		t.Fatalf("second press left the user (%s)", n)
	}
}

// An account that may create a database but not drop one: the clean-up
// takes the user back and cannot drop the database. The answer names the
// database - and only it - says who acts, and a second press with the same
// name is refused as taken, as the answer said.
func TestFailedCreateWhoseCleanUpCannotFinishNamesWhatIsLeftOnRealEngine(t *testing.T) {
	fixture, root, create := leftoverFixture(t, "a134keep",
		"GRANT SELECT, INSERT, UPDATE, DELETE, CREATE, ALTER, INDEX ON `s9201\\_%`.* TO 'a134keep'@'%'; GRANT CREATE USER ON *.* TO 'a134keep'@'%'; GRANT SELECT ON mysql.user TO 'a134keep'@'%';")
	answer := create("keepdb", "keep_u")
	if answer.Code != http.StatusConflict {
		t.Fatalf("answer %d %s", answer.Code, answer.Body.String())
	}
	var refusal apiErrorBody
	if err := json.Unmarshal(answer.Body.Bytes(), &refusal); err != nil {
		t.Fatal(err)
	}
	if refusal.Code != errCodeDatabaseCreateLeftOver || refusal.Vars["left"] != "s9201_keepdb" || refusal.Vars["name"] != "s9201_keepdb" {
		t.Fatalf("refusal %+v", refusal)
	}
	// A customer is told the administrator acts; no administrator reason.
	if refusal.Reason != "" {
		t.Fatalf("customer answer carries reason %q", refusal.Reason)
	}
	if n := leftoverCount(t, root, "SELECT CONCAT('n=', COUNT(*)) FROM mysql.user WHERE User = 's9201_keep_u';"); n != "0" {
		t.Fatalf("the user this create made stayed (%s)", n)
	}
	if n := leftoverCount(t, root, "SELECT CONCAT('n=', COUNT(*)) FROM information_schema.SCHEMATA WHERE SCHEMA_NAME = 's9201_keepdb';"); n != "1" {
		t.Fatalf("fixture: the database was expected to stay (%s)", n)
	}
	again := create("keepdb", "keep_u")
	refusal = apiErrorBody{}
	_ = json.Unmarshal(again.Body.Bytes(), &refusal)
	if again.Code != http.StatusConflict || refusal.Code != errCodeDatabaseNameTaken {
		t.Fatalf("second press: %d %s", again.Code, again.Body.String())
	}
	// Nothing was recorded by either press.
	if counts := countDatabaseSecurityRows(t, fixture.sql); counts != [3]int{0, 2, 0} {
		t.Fatalf("records %v", counts)
	}
}
