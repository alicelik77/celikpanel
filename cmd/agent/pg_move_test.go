package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.108: a database moves to another PostgreSQL cluster as a copy with
// proof. The roles it needs are carried from the source's own definitions,
// only for roles the target lacks, and nothing that does not read as one of
// the expected statements is executed.

const fakeRolesDump = `--
-- Roles
--

CREATE ROLE postgres;
ALTER ROLE postgres WITH SUPERUSER INHERIT CREATEROLE CREATEDB LOGIN REPLICATION BYPASSRLS;
CREATE ROLE shop_user;
ALTER ROLE shop_user WITH NOSUPERUSER INHERIT NOCREATEROLE NOCREATEDB LOGIN NOREPLICATION NOBYPASSRLS PASSWORD 'SCRAM-SHA-256$4096:c2FsdA==$a2V5:c2Vy';
CREATE ROLE "Shop Readers";
ALTER ROLE "Shop Readers" WITH NOSUPERUSER INHERIT NOCREATEROLE NOCREATEDB NOLOGIN NOREPLICATION NOBYPASSRLS;
ALTER ROLE shop_user SET search_path TO 'shop';
ALTER ROLE shop_user IN DATABASE other SET work_mem TO '64MB';
CREATE ROLE "x"";
DROP DATABASE other;--";
CREATE ROLE other_user;
GRANT "Shop Readers" TO shop_user WITH INHERIT TRUE GRANTED BY postgres;
GRANT other_user TO postgres GRANTED BY postgres;
`

func TestPGRoleScriptCarriesOnlyTheMissingRolesWholeStatements(t *testing.T) {
	script := pgRoleScript(fakeRolesDump, map[string]bool{"shop_user": true, "Shop Readers": true})
	want := `BEGIN;
CREATE ROLE shop_user;
ALTER ROLE shop_user WITH NOSUPERUSER INHERIT NOCREATEROLE NOCREATEDB LOGIN NOREPLICATION NOBYPASSRLS PASSWORD 'SCRAM-SHA-256$4096:c2FsdA==$a2V5:c2Vy';
CREATE ROLE "Shop Readers";
ALTER ROLE "Shop Readers" WITH NOSUPERUSER INHERIT NOCREATEROLE NOCREATEDB NOLOGIN NOREPLICATION NOBYPASSRLS;
ALTER ROLE shop_user SET search_path TO 'shop';
GRANT "Shop Readers" TO "shop_user" WITH INHERIT TRUE;
COMMIT;
`
	if script != want {
		t.Fatalf("script:\n%s", script)
	}
	if strings.Contains(script, "DROP") || strings.Contains(script, "other") || strings.Contains(script, "postgres") {
		t.Fatal("a line that was not one of the missing roles' was taken")
	}
	if name, rest, ok := pgIdent(`"a""b" TO x`); !ok || name != `a"b` || rest != " TO x" {
		t.Fatalf("quoted identifier: %q %q %v", name, rest, ok)
	}
	if _, _, ok := pgIdent(`"open`); ok {
		t.Fatal("an unterminated identifier was read")
	}
}

func TestPGProofListsMismatchesFirstAndNeedsTheSameTables(t *testing.T) {
	source := map[string]int64{"public\x1forders": 10, "public\x1fusers": 3, "public\x1fz": 1}
	target := map[string]int64{"public\x1forders": 10, "public\x1fusers": 2}
	proof := pgCompareCounts(source, target)
	if proof.Equal || proof.Mismatches != 2 || proof.TablesTotal != 3 {
		t.Fatalf("proof = %+v", proof)
	}
	if proof.Tables[0].Table != "users" || proof.Tables[1].Table != "z" || proof.Tables[1].Target != -1 || proof.Tables[2].Table != "orders" {
		t.Fatalf("order = %+v", proof.Tables)
	}
	if !pgCompareCounts(source, source).Equal {
		t.Fatal("equal counts did not prove")
	}
}

// copyHost answers like a 17 cluster on 5432 with database shop and an 18
// cluster on 5433; restoreRows is what the copy holds after pg_restore.
// sourceWrites is the source's write counters in the copy tests; a test
// changes it to simulate writes.
var sourceWrites = "100\x1f20\x1f3\x1f2026-10-01 00:00:00+00\n"

func copyHost(t *testing.T, restoreRows string) (*fakePGHost, *bool) {
	f := newFakePGHost(t)
	dumpDir := t.TempDir()
	previous := pgDumpDir
	pgDumpDir = filepath.Join(dumpDir, "pg-moves")
	t.Cleanup(func() { pgDumpDir = previous })
	restored := false
	f.answers = func(owner string, port int, database, statement string) (string, error) {
		switch {
		case strings.HasPrefix(statement, "SELECT count(*) FROM pg_database WHERE datname = 'shop'"):
			if port == 5432 || (port == 5433 && restored) {
				return "1\n", nil
			}
			return "0\n", nil
		case strings.Contains(statement, "pg_database_size"):
			return "shop_user\x1f8192\n", nil
		case statement == pgWritesStatement("shop"):
			if port != 5432 {
				t.Fatalf("write counters read on %d", port)
			}
			return sourceWrites, nil
		case statement == pgTableCountsStatement:
			if port == 5432 {
				return "public\x1forders\x1f10\npublic\x1fusers\x1f3\n", nil
			}
			return restoreRows, nil
		case statement == pgNeededRolesStatement:
			return "shop_user\x1f-\n", nil
		case strings.HasPrefix(statement, "SELECT rolname FROM pg_roles"):
			return "postgres\n", nil
		case strings.HasPrefix(statement, "BEGIN;"):
			if port != 5433 || !strings.Contains(statement, "CREATE ROLE shop_user;") {
				t.Fatalf("role script on %d: %q", port, statement)
			}
			return "", nil
		case strings.HasPrefix(statement, "DROP DATABASE IF EXISTS"), strings.HasPrefix(statement, "DROP ROLE IF EXISTS"):
			if port != 5433 {
				t.Fatalf("a cleanup reached the source: %q", statement)
			}
			return "", nil
		}
		t.Fatalf("unexpected statement on %d: %q", port, statement)
		return "", nil
	}
	f.streamFn = func(client string, stdin io.Reader, stdout io.Writer, args []string) error {
		switch client {
		case "pg_dumpall":
			_, err := io.WriteString(stdout, fakeRolesDump)
			return err
		case "pg_dump":
			_, err := io.WriteString(stdout, "PGDMP-custom")
			return err
		case "pg_restore":
			data, _ := io.ReadAll(stdin)
			if string(data) != "PGDMP-custom" {
				t.Fatalf("restore read %q", data)
			}
			restored = true
			return nil
		}
		return errors.New("unexpected client")
	}
	return f, &restored
}

func TestPGCopyIsProvedByEveryTablesRows(t *testing.T) {
	f, _ := copyHost(t, "public\x1forders\x1f10\npublic\x1fusers\x1f3\n")
	var resp transport.PostgreSQLCopyResponse
	copyPGDatabase(context.Background(), f.host(), &transport.PostgreSQLCopyRequest{Database: "shop", SourcePort: 5432, TargetPort: 5433}, &resp)
	if !resp.Copied || !resp.Proof.Equal || resp.Proof.TablesTotal != 2 || resp.Error != "" {
		t.Fatalf("copy = %+v", resp)
	}
	if resp.Proof.SourceWrites == nil || *resp.Proof.SourceWrites != (transport.PostgreSQLWriteMark{Inserted: 100, Updated: 20, Deleted: 3, StatsReset: "2026-10-01 00:00:00+00"}) {
		t.Fatalf("the proof does not keep the source's writes: %+v", resp.Proof.SourceWrites)
	}
	if resp.Proof.SourceVersion != "17" || resp.Proof.TargetVersion != "18" || resp.Proof.Owner != "shop_user" || len(resp.Proof.RolesCreated) != 1 {
		t.Fatalf("proof = %+v", resp.Proof)
	}
	if !strings.Contains(strings.Join(f.streams, "\n"), "pg_dump --port 5432 --format custom --no-password --dbname shop") ||
		!strings.Contains(strings.Join(f.streams, "\n"), "pg_restore --port 5433 --create --exit-on-error --no-password --dbname postgres") {
		t.Fatalf("streams = %v", f.streams)
	}
	// The dump file is gone and nothing that holds password hashes is returned.
	entries, _ := os.ReadDir(pgDumpDir)
	if len(entries) != 0 || strings.Contains(resp.Error, "SCRAM") {
		t.Fatalf("left behind: %v", entries)
	}
}

func TestPGCopyThatDoesNotProveIsRemovedFromTheTarget(t *testing.T) {
	f, _ := copyHost(t, "public\x1forders\x1f10\npublic\x1fusers\x1f2\n")
	var resp transport.PostgreSQLCopyResponse
	copyPGDatabase(context.Background(), f.host(), &transport.PostgreSQLCopyRequest{Database: "shop", SourcePort: 5432, TargetPort: 5433}, &resp)
	if resp.Copied || resp.ErrorCode != "PROOF_FAILED" || !resp.Cleaned || resp.Proof.Mismatches != 1 {
		t.Fatalf("copy = %+v", resp)
	}
	joined := strings.Join(f.statement, "\n")
	if !strings.Contains(joined, `DROP DATABASE IF EXISTS "shop";`) || !strings.Contains(joined, `DROP ROLE IF EXISTS "shop_user";`) {
		t.Fatalf("cleanup = %s", joined)
	}
}

func TestPGCopyNeverWritesOverADatabaseOnTheTarget(t *testing.T) {
	f, restored := copyHost(t, "")
	*restored = true // the target already has a shop
	var resp transport.PostgreSQLCopyResponse
	copyPGDatabase(context.Background(), f.host(), &transport.PostgreSQLCopyRequest{Database: "shop", SourcePort: 5432, TargetPort: 5433}, &resp)
	if resp.Copied || resp.ErrorCode != "TARGET_EXISTS" || len(f.streams) != 0 {
		t.Fatalf("copy = %+v streams %v", resp, f.streams)
	}
	resp = transport.PostgreSQLCopyResponse{}
	copyPGDatabase(context.Background(), f.host(), &transport.PostgreSQLCopyRequest{Database: "shop", SourcePort: 5432, TargetPort: 5434}, &resp)
	if resp.ErrorCode != "CLUSTER_DOWN" {
		t.Fatalf("a cluster that is down = %+v", resp)
	}
}

func TestPGDropNeedsTheCopyAndRefusesWhileConnected(t *testing.T) {
	f := newFakePGHost(t)
	copyThere := true
	inUse := true
	f.answers = func(owner string, port int, database, statement string) (string, error) {
		switch {
		case strings.HasPrefix(statement, "SELECT count(*) FROM pg_database"):
			if port == 5433 {
				if copyThere {
					return "1\n", nil
				}
				return "0\n", nil
			}
			if inUse {
				return "1\n", nil
			}
			return "0\n", nil
		case statement == `DROP DATABASE "shop";`:
			if port != 5432 {
				t.Fatalf("dropped on %d", port)
			}
			if inUse {
				return "", errors.New(`exit status 1: ERROR:  database "shop" is being accessed by other users`)
			}
			return "", nil
		}
		t.Fatalf("unexpected %q", statement)
		return "", nil
	}
	request := &transport.PostgreSQLDropRequest{Database: "shop", Port: 5432, KeepPort: 5433}
	var resp transport.PostgreSQLDropResponse
	dropPGDatabase(context.Background(), f.host(), request, &resp)
	if resp.Dropped || resp.ErrorCode != "DATABASE_IN_USE" {
		t.Fatalf("in use = %+v", resp)
	}
	inUse = false
	copyThere = false
	resp = transport.PostgreSQLDropResponse{}
	dropPGDatabase(context.Background(), f.host(), request, &resp)
	if resp.ErrorCode != "COPY_MISSING" {
		t.Fatalf("no copy = %+v", resp)
	}
	copyThere = true
	resp = transport.PostgreSQLDropResponse{}
	dropPGDatabase(context.Background(), f.host(), request, &resp)
	if !resp.Dropped || resp.Error != "" {
		t.Fatalf("drop = %+v", resp)
	}
}

// alpha.108 review: the copy proves rows at copy time only. A source written
// while it is copied is refused; a source written after the copy blocks the
// switch and the drop; unknown is never "unchanged".
func TestPGSourceWrittenDuringOrAfterTheCopyIsCaught(t *testing.T) {
	f, _ := copyHost(t, "public\x1forders\x1f10\npublic\x1fusers\x1f3\n")
	defer func() { sourceWrites = "100\x1f20\x1f3\x1f2026-10-01 00:00:00+00\n" }()
	base := f.answers
	reads := 0
	f.answers = func(owner string, port int, database, statement string) (string, error) {
		if statement == pgWritesStatement("shop") {
			reads++
			if reads == 2 { // written while the dump ran
				return "101\x1f20\x1f3\x1f2026-10-01 00:00:00+00\n", nil
			}
		}
		return base(owner, port, database, statement)
	}
	var resp transport.PostgreSQLCopyResponse
	copyPGDatabase(context.Background(), f.host(), &transport.PostgreSQLCopyRequest{Database: "shop", SourcePort: 5432, TargetPort: 5433}, &resp)
	if resp.Copied || resp.ErrorCode != "PROOF_FAILED" || !resp.Cleaned || !strings.Contains(resp.Error, "written while it was copied") {
		t.Fatalf("written during the copy = %+v", resp)
	}

	f.answers = base
	cluster := transport.PostgreSQLCluster{Version: "17", Name: "main", Port: 5432, Owner: "postgres", Online: true}
	expect := transport.PostgreSQLSourceExpectation{
		Writes:      transport.PostgreSQLWriteMark{Inserted: 100, Updated: 20, Deleted: 3, StatsReset: "2026-10-01 00:00:00+00"},
		Tables:      []transport.PostgreSQLTableCount{{Schema: "public", Table: "orders", Source: 10, Target: 10}, {Schema: "public", Table: "users", Source: 3, Target: 3}},
		TablesTotal: 2,
	}
	if code, msg := pgSourceUnchanged(context.Background(), f.host(), cluster, "shop", expect); code != "" {
		t.Fatalf("unchanged source = %s %s", code, msg)
	}
	sourceWrites = "100\x1f21\x1f3\x1f2026-10-01 00:00:00+00\n" // an UPDATE after the copy
	if code, msg := pgSourceUnchanged(context.Background(), f.host(), cluster, "shop", expect); code != "SOURCE_CHANGED" || !strings.Contains(msg, "point the application at the copy first") {
		t.Fatalf("written after the copy = %s %s", code, msg)
	}
	sourceWrites = "0\x1f0\x1f0\x1f2026-10-25 00:00:00+00\n" // statistics reset
	if code, _ := pgSourceUnchanged(context.Background(), f.host(), cluster, "shop", expect); code != "SOURCE_UNKNOWN" {
		t.Fatalf("reset statistics = %s", code)
	}
	sourceWrites = "100\x1f20\x1f3\x1f2026-10-01 00:00:00+00\n"
	expect.TablesTotal = 3 // a table the copy did not have
	if code, _ := pgSourceUnchanged(context.Background(), f.host(), cluster, "shop", expect); code != "SOURCE_CHANGED" {
		t.Fatalf("a new table = %s", code)
	}

	// The drop of the source refuses before any DROP when it changed.
	expect.TablesTotal = 2
	sourceWrites = "105\x1f20\x1f3\x1f2026-10-01 00:00:00+00\n"
	f.answers = func(owner string, port int, database, statement string) (string, error) {
		switch {
		case strings.HasPrefix(statement, "SELECT count(*) FROM pg_database"):
			return "1\n", nil
		case strings.HasPrefix(statement, "DROP DATABASE"):
			t.Fatalf("a changed source was dropped")
		}
		return base(owner, port, database, statement)
	}
	var drop transport.PostgreSQLDropResponse
	dropPGDatabase(context.Background(), f.host(), &transport.PostgreSQLDropRequest{Database: "shop", Port: 5432, KeepPort: 5433, ExpectSource: &expect}, &drop)
	if drop.Dropped || drop.ErrorCode != "SOURCE_CHANGED" {
		t.Fatalf("drop of a changed source = %+v", drop)
	}
}
