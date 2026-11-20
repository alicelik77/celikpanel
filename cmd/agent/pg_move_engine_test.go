//go:build linux

package main

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// S1b (20 November 2026), on real engines: moving a database to another
// PostgreSQL cluster never runs the customer's code with the administrator's
// rights. Two private clusters of the test environment (ports
// CELIKPANEL_TEST_PG_PORT and CELIKPANEL_TEST_PG_PORT2); skipped without
// them. The customer's function records the role it runs as in a large
// object (the copy's row proof counts tables only).

// s1bEngineClient runs one of the private engine's PostgreSQL clients the
// way the Agent's host would: as the cluster's owner over the socket, or
// signed in with a password where the arguments name a host.
func s1bEngineClient(ctx context.Context, e testEngines, client, password string, stdin io.Reader, stdout io.Writer, args ...string) error {
	binary := filepath.Join(e.dir, "root/usr/lib/postgresql/17/bin", client)
	all := args
	if !strings.Contains(strings.Join(args, " "), "--host") {
		all = append([]string{"-h", filepath.Join(e.dir, "run"), "-U", "postgres"}, args...)
	}
	cmd := exec.CommandContext(ctx, binary, all...)
	cmd.Env = append(e.env(), pgAdminSessionEnv)
	if password != "" {
		cmd.Env = append(cmd.Env, "PGPASSWORD="+password)
	}
	var stderr strings.Builder
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, stdout, &stderr
	if err := cmd.Run(); err != nil {
		return pgCommandError(err, stderr.String())
	}
	return nil
}

func TestS1bMovingADatabaseNeverRunsItsCodeWithAdministratorRights(t *testing.T) {
	e := requireTestEngines(t)
	second := os.Getenv("CELIKPANEL_TEST_PG_PORT2")
	if e.pgPort == "" || second == "" {
		t.Skip("two private PostgreSQL clusters are needed (CELIKPANEL_TEST_PG_PORT, CELIKPANEL_TEST_PG_PORT2)")
	}
	ports := map[string]string{"source": e.pgPort, "target": second}
	pg := func(port, database, statement string) engineResult {
		var out strings.Builder
		args := []string{"--no-psqlrc", "-At", "--set", "ON_ERROR_STOP=on", "-p", port}
		if database != "" {
			args = append(args, "-d", database)
		}
		err := s1bEngineClient(context.Background(), e, "psql", "", strings.NewReader(statement+"\n"), &out, args...)
		return engineResult{[]byte(out.String()), err}
	}
	for _, port := range ports {
		mustRun(t, "clean", pg(port, "", "DROP DATABASE IF EXISTS s1m; DROP ROLE IF EXISTS s1m_user;"))
	}
	t.Cleanup(func() {
		for _, port := range ports {
			_ = pg(port, "", "DROP DATABASE IF EXISTS s1m; DROP ROLE IF EXISTS s1m_user;")
		}
	})
	src := ports["source"]
	mustRun(t, "role", pg(src, "", "CREATE ROLE s1m_user LOGIN PASSWORD '"+engineOwnerPassword+"'; CREATE DATABASE s1m OWNER s1m_user;"))
	mustRun(t, "objects", pg(src, "s1m", strings.Join([]string{
		"SET ROLE s1m_user;",
		"CREATE FUNCTION public.rec(int) RETURNS boolean LANGUAGE plpgsql AS $f$ BEGIN PERFORM pg_catalog.lo_from_bytea(0, pg_catalog.convert_to(current_user::text, 'UTF8')); RETURN true; END $f$;",
		"CREATE DOMAIN public.checked AS int CHECK (public.rec(VALUE));",
		"CREATE TABLE public.t (id int CHECK (public.rec(id)), v public.checked);",
		"INSERT INTO public.t VALUES (1, 2);",
		"SELECT pg_catalog.lo_unlink(oid) FROM pg_catalog.pg_largeobject_metadata;",
		"COMMENT ON TABLE public.t IS 'kept';",
	}, "\n")))
	sourcePort, targetPort := 0, 0
	for _, c := range src {
		sourcePort = sourcePort*10 + int(c-'0')
	}
	for _, c := range second {
		targetPort = targetPort*10 + int(c-'0')
	}
	host := s1bMoveHost(t, e, func(ctx context.Context, owner, client, password string, stdin io.Reader, stdout io.Writer, args ...string) error {
		return s1bEngineClient(ctx, e, client, password, stdin, stdout, args...)
	})
	host.lsclusters = func(context.Context) (string, error) {
		return "17 one " + src + " online postgres /var/lib/postgresql/17/one /var/log/one.log\n17 two " + second + " online postgres /var/lib/postgresql/17/two /var/log/two.log\n", nil
	}
	host.query = func(ctx context.Context, owner string, port int, database, statement string) (string, error) {
		var out strings.Builder
		err := s1bEngineClient(ctx, e, "psql", "", strings.NewReader(statement+"\n"), &out, "--no-psqlrc", "-At", "--set", "ON_ERROR_STOP=on", "-p", itoaPort(port), "-d", database)
		return out.String(), err
	}
	dir := t.TempDir()
	previous := pgDumpDir
	pgDumpDir = filepath.Join(dir, "pg-moves")
	t.Cleanup(func() { pgDumpDir = previous })

	var resp transport.PostgreSQLCopyResponse
	copyPGDatabase(context.Background(), host, &transport.PostgreSQLCopyRequest{Database: "s1m", SourcePort: sourcePort, TargetPort: targetPort}, &resp)
	recorded := ""
	if resp.Copied {
		recorded = mustRun(t, "recorded", pg(second, "s1m", "SELECT coalesce(string_agg(DISTINCT pg_catalog.convert_from(pg_catalog.lo_get(oid), 'UTF8'), ','), '') FROM pg_catalog.pg_largeobject_metadata;"))
	}
	t.Logf("move = copied %v code %q; the customer's code ran as %q", resp.Copied, resp.ErrorCode, recorded)
	if strings.Contains(recorded, "postgres") {
		t.Fatalf("the customer's code ran with the administrator's rights during the move (%s)", recorded)
	}
	if !resp.Copied || recorded != "s1m_user" {
		t.Fatalf("move = %+v, recorded %q; want a copy loaded as the table's owner", resp, recorded)
	}
	kept := mustRun(t, "kept", pg(second, "s1m", "SELECT pg_catalog.pg_get_userbyid(c.relowner) || ',' || pg_catalog.obj_description(c.oid, 'pg_class') || ',' || (SELECT count(*) FROM public.t) FROM pg_catalog.pg_class c WHERE c.oid = 'public.t'::pg_catalog.regclass;"))
	if kept != "s1m_user,kept,1" {
		t.Fatalf("what the move kept: %s", kept)
	}
	if left := mustRun(t, "move roles", pg(second, "", `SELECT count(*) FROM pg_catalog.pg_roles WHERE rolname LIKE 'celikpanel\_move\_%';`)); left != "0" {
		t.Fatalf("the move role was left behind (%s)", left)
	}
}

func itoaPort(port int) string {
	digits := ""
	for port > 0 {
		digits = string(rune('0'+port%10)) + digits
		port /= 10
	}
	return digits
}
