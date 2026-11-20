package services

import (
	"errors"
	"strings"
	"testing"
)

// Every PostgreSQL database CelikPanel creates is UTF8 with an explicit,
// language-neutral locale, whatever the cluster's template1 is (20 Nov 2026:
// an Ubuntu 24.04 server's cluster had SQL_ASCII templates, and every database
// the panel made there inherited SQL_ASCII). One statement per major, as the
// official CREATE DATABASE pages give the options:
//   - 17 and later: the builtin provider (BUILTIN_LOCALE, new in 17);
//   - 15 and 16: libc named explicitly (LOCALE_PROVIDER, new in 15);
//   - 14 and earlier (and an unknown version): LC_COLLATE / LC_CTYPE only.
func TestPostgreSQLCreateDatabaseStatementPerMajor(t *testing.T) {
	cases := []struct {
		name    string
		version int
		want    string
	}{
		{"18", 180001, `CREATE DATABASE "shop" OWNER "shop_user" TEMPLATE template0 ENCODING 'UTF8' LOCALE_PROVIDER builtin BUILTIN_LOCALE 'C.UTF-8' LC_COLLATE 'C' LC_CTYPE 'C';`},
		{"17", 170006, `CREATE DATABASE "shop" OWNER "shop_user" TEMPLATE template0 ENCODING 'UTF8' LOCALE_PROVIDER builtin BUILTIN_LOCALE 'C.UTF-8' LC_COLLATE 'C' LC_CTYPE 'C';`},
		{"16", 160010, `CREATE DATABASE "shop" OWNER "shop_user" TEMPLATE template0 ENCODING 'UTF8' LOCALE_PROVIDER libc LC_COLLATE 'C.UTF-8' LC_CTYPE 'C.UTF-8';`},
		{"15", 150014, `CREATE DATABASE "shop" OWNER "shop_user" TEMPLATE template0 ENCODING 'UTF8' LOCALE_PROVIDER libc LC_COLLATE 'C.UTF-8' LC_CTYPE 'C.UTF-8';`},
		{"14", 140019, `CREATE DATABASE "shop" OWNER "shop_user" TEMPLATE template0 ENCODING 'UTF8' LC_COLLATE 'C.UTF-8' LC_CTYPE 'C.UTF-8';`},
		{"13", 130022, `CREATE DATABASE "shop" OWNER "shop_user" TEMPLATE template0 ENCODING 'UTF8' LC_COLLATE 'C.UTF-8' LC_CTYPE 'C.UTF-8';`},
		{"unknown", 0, `CREATE DATABASE "shop" OWNER "shop_user" TEMPLATE template0 ENCODING 'UTF8' LC_COLLATE 'C.UTF-8' LC_CTYPE 'C.UTF-8';`},
	}
	for _, c := range cases {
		if got := PostgreSQLCreateDatabaseStatement(`"shop"`, `"shop_user"`, c.version, false); got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got, c.want)
		}
	}
}

// A system without a C.UTF-8 libc locale (before 17): libc "C" with UTF8,
// which every system has; the text is still checked as UTF-8. From 17 the
// builtin locale needs nothing from the system, so there is no fallback.
func TestPostgreSQLCreateDatabaseStatementFallbackIsLibcCWithUTF8(t *testing.T) {
	if got, want := PostgreSQLCreateDatabaseStatement(`"shop"`, `"shop_user"`, 160010, true),
		`CREATE DATABASE "shop" OWNER "shop_user" TEMPLATE template0 ENCODING 'UTF8' LOCALE_PROVIDER libc LC_COLLATE 'C' LC_CTYPE 'C';`; got != want {
		t.Fatalf("16 fallback:\n got %s\nwant %s", got, want)
	}
	if got, want := PostgreSQLCreateDatabaseStatement(`"shop"`, `"shop_user"`, 140019, true),
		`CREATE DATABASE "shop" OWNER "shop_user" TEMPLATE template0 ENCODING 'UTF8' LC_COLLATE 'C' LC_CTYPE 'C';`; got != want {
		t.Fatalf("14 fallback:\n got %s\nwant %s", got, want)
	}
	if got := PostgreSQLCreateDatabaseStatement(`"shop"`, `"shop_user"`, 170006, true); !strings.Contains(got, "BUILTIN_LOCALE 'C.UTF-8'") {
		t.Fatalf("17 has no fallback: %s", got)
	}
	if PostgreSQLCreateDatabaseNeedsFallback(170006, errors.New(`ERROR:  invalid LC_COLLATE locale name: "C.UTF-8"`)) {
		t.Fatal("17 never falls back")
	}
}

// The driver's statement names no owner (the registered server's account
// owns it, as before).
func TestPostgreSQLCreateDatabaseStatementWithoutOwner(t *testing.T) {
	if got, want := PostgreSQLCreateDatabaseStatement(`"shop"`, "", 170006, false),
		`CREATE DATABASE "shop" TEMPLATE template0 ENCODING 'UTF8' LOCALE_PROVIDER builtin BUILTIN_LOCALE 'C.UTF-8' LC_COLLATE 'C' LC_CTYPE 'C';`; got != want {
		t.Fatalf("got %s", got)
	}
}

func TestPostgreSQLCreateDatabaseNeedsFallbackOnlyForAMissingLocale(t *testing.T) {
	for text, want := range map[string]bool{
		`ERROR:  invalid LC_COLLATE locale name: "C.UTF-8"`:                                                  true,
		`ERROR:  invalid LC_CTYPE locale name: "C.UTF-8"` + "\nHINT:  If the locale name is specific to ICU": true,
		`ERROR:  invalid locale name: "C.UTF-8"`:                                                             true,
		`ERROR:  database "shop" already exists`:                                                             false,
		`ERROR:  permission denied to create database`:                                                       false,
		`ERROR:  invalid LC_COLLATE locale name: "en_US.UTF-8"`:                                              false,
	} {
		if got := PostgreSQLCreateDatabaseNeedsFallback(160010, errors.New(text)); got != want {
			t.Errorf("%q: got %v", text, got)
		}
	}
	if PostgreSQLCreateDatabaseNeedsFallback(160010, nil) {
		t.Fatal("no error, no fallback")
	}
}

// The registered-server driver sends the same statement for its server's
// major. Its old "WITH ENCODING='UTF8'" copied template1 and was refused on a
// cluster whose template1 is SQL_ASCII.
func TestPostgreSQLDriverCreatesUTF8FromTemplate0(t *testing.T) {
	state := &postgreSQLScript{versionNum: "180001"}
	if err := newScriptedPostgreSQLDriver(state).CreateDatabase("shop"); err != nil {
		t.Fatal(err)
	}
	_, execs, _, _, _ := state.snapshot()
	if len(execs) != 2 || execs[0] != `CREATE DATABASE "shop" TEMPLATE template0 ENCODING 'UTF8' LOCALE_PROVIDER builtin BUILTIN_LOCALE 'C.UTF-8' LC_COLLATE 'C' LC_CTYPE 'C';` {
		t.Fatalf("execs = %q", execs)
	}

	state = &postgreSQLScript{versionNum: "160010", failExecMatch: "'C.UTF-8'", execErr: errors.New(`ERROR: invalid LC_COLLATE locale name: "C.UTF-8" (SQLSTATE 42809)`)}
	if err := newScriptedPostgreSQLDriver(state).CreateDatabase("shop"); err != nil {
		t.Fatal(err)
	}
	_, execs, _, _, _ = state.snapshot()
	if len(execs) != 3 || execs[1] != `CREATE DATABASE "shop" TEMPLATE template0 ENCODING 'UTF8' LOCALE_PROVIDER libc LC_COLLATE 'C' LC_CTYPE 'C';` {
		t.Fatalf("fallback execs = %q", execs)
	}
}

// The driver closes the new database to PUBLIC and reads it back; a database
// still open is removed again and the create fails.
func TestPostgreSQLDriverClosesTheNewDatabaseToPublic(t *testing.T) {
	state := &postgreSQLScript{versionNum: "180001"}
	if err := newScriptedPostgreSQLDriver(state).CreateDatabase("shop"); err != nil {
		t.Fatal(err)
	}
	queries, execs, _, _, _ := state.snapshot()
	if len(execs) != 2 || execs[1] != `REVOKE ALL ON DATABASE "shop" FROM PUBLIC;` || queries[len(queries)-1] != PostgreSQLPublicReachQuery {
		t.Fatalf("execs = %q queries = %q", execs, queries)
	}

	state = &postgreSQLScript{versionNum: "180001", terminationOK: true} // the read-back answers "open"
	err := newScriptedPostgreSQLDriver(state).CreateDatabase("shop")
	_, execs, _, _, _ = state.snapshot()
	if err == nil || !strings.Contains(err.Error(), "removed again") || execs[len(execs)-1] != `DROP DATABASE IF EXISTS "shop";` {
		t.Fatalf("err = %v execs = %q", err, execs)
	}
}

func TestParsePostgreSQLVersionNum(t *testing.T) {
	for text, want := range map[string]int{"180001": 180001, " 170006\n": 170006, "90624": 90624, "": 0, "x": 0, "-1": 0} {
		if got := ParsePostgreSQLVersionNum(text); got != want {
			t.Errorf("%q: got %d", text, got)
		}
	}
}

// What a database's encoding means to a person: only UTF8 holds text checked
// as Unicode.
func TestPostgreSQLEncodingIsUTF8(t *testing.T) {
	for name, want := range map[string]bool{"UTF8": true, "utf8": true, "SQL_ASCII": false, "LATIN1": false, "": false} {
		if got := PostgreSQLEncodingIsUTF8(name); got != want {
			t.Errorf("%q: got %v", name, got)
		}
	}
}
