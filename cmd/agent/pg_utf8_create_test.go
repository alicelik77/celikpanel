package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// 20 Nov 2026, from the owner's production server: the cluster 18/main had
// SQL_ASCII templates (it was initialised under locale C), and every database
// the panel made on it inherited SQL_ASCII from template1. A database the
// Agent creates names template0, UTF8 and its locale, whatever the cluster's
// templates are.
//
// Sahibin sunucusunda küme şablonları SQL_ASCII idi; panelin yaptığı her
// veritabanı da öyle oldu. Agent artık template0, UTF8 ve yerel ayarı açıkça
// yazar.

// pgCreateAnswers answers the create's inspection with a server version (or
// none, as a server older than this change's question would), records every
// statement and lets one CREATE DATABASE fail with a given answer.
func pgCreateAnswers(t *testing.T, version string, failCreate func(statement string) []byte) *[]string {
	t.Helper()
	usePGStatementClusters(t, bostonLsclusters)
	var got []string
	previous := postgreSQLExecStatementOnPort
	postgreSQLExecStatementOnPort = func(_ int, statement string) ([]byte, error) {
		got = append(got, statement)
		switch {
		case strings.HasPrefix(statement, "SELECT 'roles='"):
			answer := "roles=0\nmark=\nowner=\n"
			if version != "" {
				answer += "ver=" + version + "\n"
			}
			return []byte(answer), nil
		case strings.HasPrefix(statement, "CREATE DATABASE") && failCreate != nil:
			if out := failCreate(statement); out != nil {
				return out, errors.New("exit status 3")
			}
		}
		return nil, nil
	}
	t.Cleanup(func() { postgreSQLExecStatementOnPort = previous })
	return &got
}

func createdDatabaseStatements(got []string) []string {
	var creates []string
	for _, statement := range got {
		if strings.HasPrefix(statement, "CREATE DATABASE") {
			creates = append(creates, statement)
		}
	}
	return creates
}

func TestPostgreSQLDatabaseIsCreatedUTF8FromTemplate0OnEveryMajor(t *testing.T) {
	for _, c := range []struct{ version, want string }{
		{"180001", `CREATE DATABASE "shop" OWNER "shop_user" TEMPLATE template0 ENCODING 'UTF8' LOCALE_PROVIDER builtin BUILTIN_LOCALE 'C.UTF-8' LC_COLLATE 'C' LC_CTYPE 'C';`},
		{"170006", `CREATE DATABASE "shop" OWNER "shop_user" TEMPLATE template0 ENCODING 'UTF8' LOCALE_PROVIDER builtin BUILTIN_LOCALE 'C.UTF-8' LC_COLLATE 'C' LC_CTYPE 'C';`},
		{"160010", `CREATE DATABASE "shop" OWNER "shop_user" TEMPLATE template0 ENCODING 'UTF8' LOCALE_PROVIDER libc LC_COLLATE 'C.UTF-8' LC_CTYPE 'C.UTF-8';`},
		{"150014", `CREATE DATABASE "shop" OWNER "shop_user" TEMPLATE template0 ENCODING 'UTF8' LOCALE_PROVIDER libc LC_COLLATE 'C.UTF-8' LC_CTYPE 'C.UTF-8';`},
		{"140019", `CREATE DATABASE "shop" OWNER "shop_user" TEMPLATE template0 ENCODING 'UTF8' LC_COLLATE 'C.UTF-8' LC_CTYPE 'C.UTF-8';`},
		// A server that does not answer the version question: the form every
		// major since 8.4 takes.
		{"", `CREATE DATABASE "shop" OWNER "shop_user" TEMPLATE template0 ENCODING 'UTF8' LC_COLLATE 'C.UTF-8' LC_CTYPE 'C.UTF-8';`},
	} {
		got := pgCreateAnswers(t, c.version, nil)
		var resp CreateDatabaseResponse
		if err := (&Agent{}).CreateDatabase(CreateDatabaseRequest{Type: "postgresql", Name: "shop", User: "shop_user", Password: "pw", Port: 5433}, &resp); err != nil || !resp.Success {
			t.Fatalf("%s: resp=%+v err=%v", c.version, resp, err)
		}
		if creates := createdDatabaseStatements(*got); len(creates) != 1 || creates[0] != c.want {
			t.Fatalf("version %q: CREATE DATABASE sent = %q\nwant %q", c.version, creates, c.want)
		}
	}
}

// Before 17, a server without the C.UTF-8 libc locale refuses the name;
// nothing was created, so the statement is sent once more with libc C (still
// UTF8). Any other refusal is not retried.
func TestPostgreSQLDatabaseCreateFallsBackToLibcCOnlyForAMissingLocale(t *testing.T) {
	got := pgCreateAnswers(t, "160010", func(statement string) []byte {
		if strings.Contains(statement, "'C.UTF-8'") {
			return []byte(`ERROR:  invalid LC_COLLATE locale name: "C.UTF-8"`)
		}
		return nil
	})
	var resp CreateDatabaseResponse
	if err := (&Agent{}).CreateDatabase(CreateDatabaseRequest{Type: "postgresql", Name: "shop", User: "shop_user", Password: "pw", Port: 5433}, &resp); err != nil || !resp.Success {
		t.Fatalf("resp=%+v err=%v", resp, err)
	}
	creates := createdDatabaseStatements(*got)
	if len(creates) != 2 || creates[1] != `CREATE DATABASE "shop" OWNER "shop_user" TEMPLATE template0 ENCODING 'UTF8' LOCALE_PROVIDER libc LC_COLLATE 'C' LC_CTYPE 'C';` {
		t.Fatalf("statements = %q", creates)
	}

	got = pgCreateAnswers(t, "180001", func(string) []byte { return []byte(`ERROR:  permission denied to create database`) })
	resp = CreateDatabaseResponse{}
	if err := (&Agent{}).CreateDatabase(CreateDatabaseRequest{Type: "postgresql", Name: "shop", User: "shop_user", Password: "pw", Port: 5433}, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Success || len(createdDatabaseStatements(*got)) != 1 || !strings.Contains(strings.Join(*got, "\n"), `DROP ROLE IF EXISTS "shop_user"`) {
		t.Fatalf("resp=%+v statements=%q", resp, *got)
	}
}

// A cluster the Agent creates itself (a PGDG major on the distribution's
// postgresql-common creates none in its own install) is UTF8 with C.UTF-8,
// not whatever the Agent's command environment says: that environment carries
// LC_ALL=C for parseable output, and pg_createcluster inherits its locale
// from the environment, so 18/main came out SQL_ASCII.
func TestPGCreateMainClusterNamesItsLocaleAndEncoding(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("runs a recording pg_createcluster script")
	}
	for _, c := range []struct {
		version string
		want    string
	}{
		{"18", "18 main -p 5433 --locale C.UTF-8 -e UTF8 -- --locale-provider=builtin --builtin-locale=C.UTF-8"},
		{"17", "17 main -p 5433 --locale C.UTF-8 -e UTF8 -- --locale-provider=builtin --builtin-locale=C.UTF-8"},
		{"16", "16 main -p 5433 --locale C.UTF-8 -e UTF8"},
	} {
		args := recordPGCreateCluster(t)
		if err := pgCreateMainCluster(context.Background(), c.version, 5433); err != nil {
			t.Fatal(err)
		}
		if got := readRecorded(t, args); got != c.want {
			t.Fatalf("pg_createcluster %s:\n got %q\nwant %q", c.version, got, c.want)
		}
	}
}

// The owner's createcluster.conf that passes initdb a locale, encoding or
// provider of its own keeps it: pg_createcluster puts initdb_options before
// its own command-line options, so naming ours would override the owner's.
func TestPGCreateMainClusterKeepsTheOwnersInitdbLocale(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("runs a recording pg_createcluster script")
	}
	for conf, ours := range map[string]bool{
		"# initdb_options = ''\n":                                      true,
		"initdb_options = '--data-checksums'\n":                        true,
		"initdb_options = '--locale=tr_TR.UTF-8'\n":                    false,
		"initdb_options = '-E LATIN1 --locale=de_DE'\n":                false,
		"initdb_options = '--locale-provider=icu --icu-locale=tr'\n":   false,
		"create_main_cluster = true\ninitdb_options = '--no-locale'\n": false,
	} {
		args := recordPGCreateCluster(t)
		if err := os.WriteFile(pgCommonConfPath, []byte(conf), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := pgCreateMainCluster(context.Background(), "18", 5433); err != nil {
			t.Fatal(err)
		}
		got := readRecorded(t, args)
		if named := strings.Contains(got, "--locale C.UTF-8"); named != ours {
			t.Fatalf("conf %q: pg_createcluster %q", conf, got)
		}
	}
}

// A PostgreSQL server package's own install script creates NN/main with
// pg_createcluster, which takes the locale from the install's environment:
// that install runs with LC_ALL=C.UTF-8 whatever the Agent inherited. Every
// other install keeps the environment it had.
func TestAPTInstallOfAPostgreSQLServerRunsUnderCUTF8(t *testing.T) {
	base := []string{"PATH=/usr/bin", "LANG=", "LC_ALL=C"}
	env := aptInstallEnv(base, []string{"postgresql-17", "postgresql-contrib"})
	if last := env[len(env)-1]; last != "LC_ALL=C.UTF-8" || !slicesContain(env, "DEBIAN_FRONTEND=noninteractive") {
		t.Fatalf("postgresql-17 env = %q", env)
	}
	env = aptInstallEnv(base, []string{"nginx"})
	if slicesContain(env, "LC_ALL=C.UTF-8") || !slicesContain(env, "DEBIAN_FRONTEND=noninteractive") {
		t.Fatalf("nginx env = %q", env)
	}
	if !slicesContain(aptInstallEnv(base, []string{"postgresql-18=18.1-1.pgdg13+2"}), "LC_ALL=C.UTF-8") {
		t.Fatal("a pinned postgresql-18 install kept the inherited locale")
	}
	// The client alone creates no cluster.
	if slicesContain(aptInstallEnv(base, []string{"postgresql-client-17"}), "LC_ALL=C.UTF-8") {
		t.Fatal("a client install changed its locale")
	}
}

func slicesContain(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

// recordPGCreateCluster points pgCreateClusterPath at a script that writes
// its arguments, one line, to the returned file.
func recordPGCreateCluster(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	out := filepath.Join(dir, "args")
	script := filepath.Join(dir, "pg_createcluster")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho \"$*\" > '"+out+"'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	previous, previousConf := pgCreateClusterPath, pgCommonConfPath
	pgCreateClusterPath = script
	// Never the guest's own createcluster.conf; a test writes one here.
	pgCommonConfPath = filepath.Join(dir, "createcluster.conf")
	t.Cleanup(func() { pgCreateClusterPath, pgCommonConfPath = previous, previousConf })
	return out
}

func readRecorded(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(data))
}
