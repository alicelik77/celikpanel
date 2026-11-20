package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/services"
)

// alpha.141 carried item 12: a database deletion remembers that the engine
// already dropped the database, so a database of the same name made in the
// meantime is not deleted by a second press.
//
// The engine below is the one the deletion meets: names, each with the
// identity of the database that holds it now (a new one each time a database
// of that name is made), drops whose answer can be lost after they ran, and
// user removals that refuse while a database of the name exists, as the
// Agent's do. installFakeDatabaseEngine (database_delete_once_wiring_test.go)
// puts it behind the deletion's Agent seams.

type fakeDatabaseEngine struct {
	databases map[string]string
	made      int
	// dropped: the identities of the databases the engine dropped, in order.
	dropped []string
	// loseAnswer: the next drop runs (when dropRuns) and its answer is lost.
	loseAnswer, dropRuns bool
	// noIdentity: the engine tells no identity (a filesystem without birth
	// times, a MariaDB reached with a record's own credential).
	noIdentity bool
	// usersFail: the next user removals are refused (and stay refused).
	usersFail bool
	removed   []string
}

func newFakeDatabaseEngine(names ...string) *fakeDatabaseEngine {
	e := &fakeDatabaseEngine{databases: map[string]string{}}
	for _, name := range names {
		e.create(name)
	}
	return e
}

// create makes a database of that name: what the customer's own user, the
// owner by hand, a restore or an installer does on the engine.
func (e *fakeDatabaseEngine) create(name string) string {
	e.made++
	identity := fmt.Sprintf("generation-%d-of-%s", e.made, name)
	e.databases[name] = identity
	return identity
}

func (e *fakeDatabaseEngine) identity(name string) (bool, string) {
	identity, ok := e.databases[name]
	if e.noIdentity {
		identity = ""
	}
	return ok, identity
}

// dropByName is DROP DATABASE IF EXISTS <name>.
func (e *fakeDatabaseEngine) dropByName(name string) {
	if identity, ok := e.databases[name]; ok {
		e.dropped = append(e.dropped, identity)
		delete(e.databases, name)
	}
}

func deleteOnceAnswer(t *testing.T, body []byte) apiErrorBody {
	t.Helper()
	var answer apiErrorBody
	if err := json.Unmarshal(body, &answer); err != nil {
		t.Fatalf("answer %s: %v", body, err)
	}
	return answer
}

// requireSurvives is the invariant this item is about.
func requireSurvives(t *testing.T, e *fakeDatabaseEngine, name, identity string) {
	t.Helper()
	if e.databases[name] != identity {
		t.Fatalf("the database %s made after the first press (%s) was deleted by a later press: engine now %q, dropped %v",
			name, identity, e.databases[name], e.dropped)
	}
}

func TestDatabaseDeleteAgainNeverDropsASameNamedDatabaseMadeMeanwhile(t *testing.T) {
	const shop = "customer_matrix_test_shop"
	cases := []struct {
		name, path, database string
		id                   int
		first                func(e *fakeDatabaseEngine)
		firstCode            int
	}{
		{name: "the drop's answer was lost (MariaDB)", path: "/api/v1/databases/9961", database: shop, id: 9961,
			first: func(e *fakeDatabaseEngine) { e.loseAnswer, e.dropRuns = true, true }, firstCode: http.StatusBadGateway},
		{name: "the drop's answer was lost (PostgreSQL)", path: "/api/v1/databases/9964", database: "admin_tools", id: 9964,
			first: func(e *fakeDatabaseEngine) { e.loseAnswer, e.dropRuns = true, true }, firstCode: http.StatusBadGateway},
		{name: "the users were not all removed", path: "/api/v1/databases/9961", database: shop, id: 9961,
			first: func(e *fakeDatabaseEngine) { e.usersFail = true }, firstCode: http.StatusConflict},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newVisibleDatabasesFixture(t)
			e := newFakeDatabaseEngine(shop, "admin_tools")
			installFakeDatabaseEngine(t, f.panel, e)
			original := e.databases[c.database]

			c.first(e)
			if got := f.as(t, "admin", http.MethodDelete, c.path, ""); got.Code != c.firstCode {
				t.Fatalf("first press: %d %s", got.Code, got.Body.String())
			}
			if _, there := e.databases[c.database]; there {
				t.Fatalf("the first press did not reach the drop: %v", e.databases)
			}
			if f.count(t, fmt.Sprintf(`SELECT COUNT(*) FROM databases_v2 WHERE id = %d`, c.id)) != 1 {
				t.Fatal("the record went although the deletion did not finish")
			}
			// Meanwhile a database of the same name is made on the engine.
			e.usersFail = false
			made := e.create(c.database)

			for press := 2; press <= 3; press++ {
				got := f.as(t, "admin", http.MethodDelete, c.path, "")
				requireSurvives(t, e, c.database, made)
				answer := deleteOnceAnswer(t, got.Body.Bytes())
				if got.Code != http.StatusConflict || answer.Code != errCodeDatabaseDeleteUsersLeft || answer.Reason != "exists" ||
					strings.Contains(answer.Error, "deletes the database that is there now") {
					t.Fatalf("press %d: %d %s", press, got.Code, got.Body.String())
				}
			}
			if strings.Join(e.dropped, ",") != original {
				t.Fatalf("dropped %v, want only %s", e.dropped, original)
			}
			// Once that database is gone (the administrator decides on the
			// server), Delete finishes without dropping anything.
			delete(e.databases, c.database)
			if got := f.as(t, "admin", http.MethodDelete, c.path, ""); got.Code != http.StatusOK {
				t.Fatalf("after the other database went: %d %s", got.Code, got.Body.String())
			}
			if f.count(t, fmt.Sprintf(`SELECT COUNT(*) FROM databases_v2 WHERE id = %d`, c.id)) != 0 ||
				f.count(t, `SELECT COUNT(*) FROM panel_settings WHERE key LIKE 'database_delete_v1:%'`) != 0 {
				t.Fatal("the record or its deletion marker stayed after the deletion finished")
			}
		})
	}
}

// The drop's answer was lost, but the drop never ran: the database there is
// still the one asked for (same identity), and pressing again deletes it.
func TestDatabaseDeleteAgainAfterADropThatNeverRanDeletesTheSameDatabase(t *testing.T) {
	f := newVisibleDatabasesFixture(t)
	e := newFakeDatabaseEngine("customer_matrix_test_shop")
	installFakeDatabaseEngine(t, f.panel, e)
	original := e.databases["customer_matrix_test_shop"]
	e.loseAnswer = true
	if got := f.as(t, "admin", http.MethodDelete, "/api/v1/databases/9961", ""); got.Code != http.StatusBadGateway {
		t.Fatalf("first press: %d %s", got.Code, got.Body.String())
	}
	if got := f.as(t, "admin", http.MethodDelete, "/api/v1/databases/9961", ""); got.Code != http.StatusOK {
		t.Fatalf("second press: %d %s", got.Code, got.Body.String())
	}
	if strings.Join(e.dropped, ",") != original || len(e.databases) != 0 {
		t.Fatalf("dropped %v, engine %v", e.dropped, e.databases)
	}
}

// Without an identity a database of the name found after a lost answer is
// never dropped: it may be the one asked for or another. The answer says so
// and who acts; nothing changes; once it is gone, Delete finishes.
func TestDatabaseDeleteWithoutAnIdentityNeverDropsWhatItCannotTellApart(t *testing.T) {
	for _, who := range []string{"admin", "customer"} {
		t.Run(who, func(t *testing.T) {
			f := newVisibleDatabasesFixture(t)
			e := newFakeDatabaseEngine("customer_matrix_test_shop")
			e.noIdentity = true
			installFakeDatabaseEngine(t, f.panel, e)
			e.loseAnswer, e.dropRuns = true, true
			if got := f.as(t, who, http.MethodDelete, "/api/v1/databases/9961", ""); got.Code != http.StatusBadGateway {
				t.Fatalf("first press: %d %s", got.Code, got.Body.String())
			}
			made := e.create("customer_matrix_test_shop")
			got := f.as(t, who, http.MethodDelete, "/api/v1/databases/9961", "")
			requireSurvives(t, e, "customer_matrix_test_shop", made)
			answer := deleteOnceAnswer(t, got.Body.Bytes())
			if got.Code != http.StatusConflict || answer.Code != errCodeDatabaseDeleteUnverified || answer.Vars["name"] != "customer_matrix_test_shop" ||
				(answer.Reason == "administrator") != (who == "admin") {
				t.Fatalf("second press: %d %s", got.Code, got.Body.String())
			}
			if f.count(t, `SELECT COUNT(*) FROM databases_v2 WHERE id = 9961`) != 1 || len(e.removed) != 0 {
				t.Fatal("an unverified press changed a record or a user")
			}
			delete(e.databases, "customer_matrix_test_shop")
			if got := f.as(t, who, http.MethodDelete, "/api/v1/databases/9961", ""); got.Code != http.StatusOK {
				t.Fatalf("after it went: %d %s", got.Code, got.Body.String())
			}
			if len(e.dropped) != 1 {
				t.Fatalf("dropped %v", e.dropped)
			}
		})
	}
}

// The domain deletion that was asked to delete the domain's databases goes
// through the same deletion: its retry never drops a database of the same
// name made meanwhile either.
func TestDomainDeletionRetryNeverDropsASameNamedDatabaseMadeMeanwhile(t *testing.T) {
	p, f, _ := newDatabaseDeletionPanel(t, "made-again.example")
	e := newFakeDatabaseEngine("shop_db", "blog_db", "elsewhere_db")
	installFakeDatabaseEngine(t, p, e)
	e.loseAnswer, e.dropRuns = true, true

	first := deleteDomainWithQuery(t, p, f.domainID, "?databases=delete")
	requireDeletionPendingStage(t, first, domainDeletionStageDatabases)
	if _, there := e.databases["blog_db"]; there {
		t.Fatalf("the first press did not reach blog_db's drop: %v", e.databases)
	}
	made := e.create("blog_db")
	second := deleteDomainWithQuery(t, p, f.domainID, "")
	requireSurvives(t, e, "blog_db", made)
	if second.Code == http.StatusOK {
		t.Fatalf("the retry finished while a database of that name is there: %s", second.Body.String())
	}
	if _, there := e.databases["shop_db"]; !there {
		t.Fatal("the retry went past blog_db")
	}
}

// remoteOnceDriver is a database server reached with its record's own
// credential, holding the same fake engine; it tells identities the way the
// driver of its engine does (PostgreSQL its oid, MariaDB none).
type remoteOnceDriver struct {
	authzGrantIsolationDriver
	e *fakeDatabaseEngine
}

func (d *remoteOnceDriver) DatabaseIdentity(name string) (bool, string, error) {
	exists, identity := d.e.identity(name)
	return exists, identity, nil
}

func (d *remoteOnceDriver) DeleteDatabase(name string) error {
	if d.e.loseAnswer {
		d.e.loseAnswer = false
		d.e.dropByName(name)
		return errors.New("read tcp 10.0.0.1:41000->10.0.0.9:5432: i/o timeout")
	}
	d.e.dropByName(name)
	return nil
}

// The same on a database server reached with its record's own credential.
func TestDatabaseDeleteAgainOnAnotherMachineNeverDropsASameNamedDatabase(t *testing.T) {
	for _, c := range []struct {
		engine     string
		noIdentity bool
		secondCode int
	}{{"postgresql", false, http.StatusOK}, {"mariadb", true, http.StatusConflict}} {
		t.Run(c.engine, func(t *testing.T) {
			f := newVisibleDatabasesFixture(t)
			e := newFakeDatabaseEngine("customer_matrix_test_shop")
			e.noIdentity = c.noIdentity
			installFakeDatabaseEngine(t, f.panel, e)
			if c.engine == "postgresql" {
				f.exec(t, `UPDATE database_servers SET host = 'far-db.example.net', port = 5432,
					type_id = (SELECT id FROM database_server_types WHERE name = 'postgresql') WHERE id = 9921`)
			} else {
				f.exec(t, `UPDATE database_servers SET host = 'far-db.example.net' WHERE id = 9921`)
			}
			driver := &remoteOnceDriver{e: e}
			saved := newDatabaseDriver
			newDatabaseDriver = func(services.DriverConfig) (services.DatabaseDriver, error) { return driver, nil }
			t.Cleanup(func() { newDatabaseDriver = saved })

			e.loseAnswer = true
			if got := f.as(t, "admin", http.MethodDelete, "/api/v1/databases/9961", ""); got.Code < 500 {
				t.Fatalf("first press: %d %s", got.Code, got.Body.String())
			}
			made := e.create("customer_matrix_test_shop")
			got := f.as(t, "admin", http.MethodDelete, "/api/v1/databases/9961", "")
			requireSurvives(t, e, "customer_matrix_test_shop", made)
			if got.Code != c.secondCode {
				t.Fatalf("second press: %d %s", got.Code, got.Body.String())
			}
		})
	}
}
