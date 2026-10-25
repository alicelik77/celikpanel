//go:build linux && realsystem

package main

import (
	"context"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Real-system evidence for alpha.108's PostgreSQL majors side by side, run
// only inside a disposable Debian 13 virtual machine booted from the base
// image (nginx, the vendor PHP repository's own source, no PostgreSQL;
// network available; build tag realsystem, plus
// CELIKPANEL_REAL_SYSTEM_A108=1). Through the Agent's own code: the
// distribution's PostgreSQL 17 is installed by its exact name without PGDG; a
// database with two tables, rows and its own role is made; PGDG is enabled
// and confined; PostgreSQL 18 is installed as a second cluster on the port
// shown before; that cluster's port changes from 5433 to 5440 with proof; the
// database is copied 17 → 18 with every table's rows counted on both sides
// and its role carried with its password; removing postgresql-17 is refused
// while 17/main still holds the database; the source is dropped only as the
// explicit step, after which the removal is no longer refused and runs,
// keeping 17's data. Never run it on an installed server.
//
// alpha.108 gerçek sistem kanıtı; yalnız atılabilir bir Debian 13 sanal
// makinesinde koşar. Kurulu bir sunucuda asla koşturulmaz.

func requireRealSystemA108(t *testing.T) {
	t.Helper()
	if os.Getenv("CELIKPANEL_REAL_SYSTEM_A108") != "1" {
		t.Skip("real-system evidence runs only in the disposable alpha.108 VM")
	}
}

// a108Psql runs one statement as postgres on a port (and database), for the
// evidence's own setup and checks; the statement goes on stdin.
func a108Psql(t *testing.T, port, database, statement string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "runuser", "-u", "postgres", "--", "psql", "--no-psqlrc", "-X", "-At", "--set", "ON_ERROR_STOP=on", "-p", port, "-d", database)
	cmd.Stdin = strings.NewReader(statement + "\n")
	out, err := cmd.CombinedOutput()
	t.Logf("psql -p %s -d %s <<< %q (err=%v)\n%s", port, database, statement, err, lastLines(string(out), 20))
	if err != nil {
		t.Fatalf("psql on %s: %v", port, err)
	}
	return strings.TrimSpace(string(out))
}

// a108Job runs one Agent mutation in its own service-mutation job, as the
// Panel does.
func a108Job(t *testing.T, kind, target, qualifier string, call func(ServiceMutationBinding) bool) {
	t.Helper()
	manager, _ := newMutationTestManager(t)
	installGlobalMutationTestManager(t, manager)
	beginMutationTestJobWithIdentity(t, manager, kind, target, qualifier)
	ok := call(ServiceMutationBinding{MutationRequestID: testMutationRequestID, MutationOwnerID: testMutationOwnerID})
	if _, err := manager.finish(&ServiceMutationFinishRequest{RequestID: testMutationRequestID, OwnerID: testMutationOwnerID, Success: ok}); err != nil {
		t.Fatal(err)
	}
}

func a108Clusters(t *testing.T) transport.PostgreSQLClustersResponse {
	t.Helper()
	var resp transport.PostgreSQLClustersResponse
	if err := (&Agent{}).PostgreSQLClustersV1(&transport.Empty{}, &resp); err != nil {
		t.Fatal(err)
	}
	logJSON(t, "clusters", resp)
	a107Out(t, "pg_lsclusters")
	return resp
}

func a108Cluster(resp transport.PostgreSQLClustersResponse, version string) (transport.PostgreSQLCluster, bool) {
	for _, cluster := range resp.Clusters {
		if cluster.Version == version && cluster.Name == "main" {
			return cluster, true
		}
	}
	return transport.PostgreSQLCluster{}, false
}

func TestRealSystemA108PostgreSQLSideBySide(t *testing.T) {
	requireRealSystemA108(t)
	a := &Agent{}

	// 1. The distribution's PostgreSQL 17, by its exact name, without PGDG.
	if _, ok := enabledRepoLocations()["pgdg"]; ok {
		t.Fatal("the base image already has PGDG enabled")
	}
	if resp := a107Install(t, a, "postgresql", "postgresql-17"); resp.Error != "" {
		t.Fatalf("postgresql-17 from the distribution: %s", resp.Error)
	} else if !strings.Contains(resp.Detail, "cluster 17/main answers on port 5432") {
		t.Fatalf("the install did not prove its cluster: %q", resp.Detail)
	}
	if a107Version("postgresql") != "" {
		t.Fatal("the moving postgresql package was installed")
	}
	before := a108Clusters(t)
	seventeen, ok := a108Cluster(before, "17")
	if !before.Supported || !ok || seventeen.Port != 5432 || !seventeen.Online || !seventeen.DatabasesKnown || len(seventeen.Databases) != 0 {
		t.Fatalf("17/main = %+v", before)
	}

	// 2. A database with its own role, two tables and rows.
	a108Psql(t, "5432", "postgres", "CREATE ROLE a108_user LOGIN PASSWORD 'a108-evidence-only';")
	a108Psql(t, "5432", "postgres", "CREATE DATABASE a108shop OWNER a108_user;")
	a108Psql(t, "5432", "a108shop", "SET ROLE a108_user; CREATE TABLE orders (id integer PRIMARY KEY, note text); INSERT INTO orders SELECT g, 'order ' || g FROM generate_series(1, 1000) g; CREATE SCHEMA shop; CREATE TABLE shop.users (id integer, name text); INSERT INTO shop.users SELECT g, 'user ' || g FROM generate_series(1, 37) g;")

	// 3. PGDG, enabled through its vendor lane (repository, confinement and
	// automatic updates in one confirmed operation), offers 18 on the next
	// free port.
	a108AutomaticUpdates(t, a)
	if _, written := a108Lane(t, a, "pgdg"); !written.Enabled {
		t.Fatalf("pgdg lane = %+v", written)
	}
	if state := repoConfinementOf(repoFromCatalogue("pgdg"), enabledRepoLocations()["pgdg"]); state.State != repoConfinementConfined {
		t.Fatalf("pgdg after its lane: %+v", state)
	}
	offered := a108Clusters(t)
	var eighteen transport.PostgreSQLMajorOffer
	for _, offer := range offered.Offers {
		if offer.Major == "18" {
			eighteen = offer
		}
	}
	if eighteen.Origin != "pgdg" || eighteen.Port != 5433 || offered.NextPort != 5433 {
		t.Fatalf("18 offered as %+v (next port %d)", eighteen, offered.NextPort)
	}
	var preview transport.ServiceInstallPreviewResponse
	if err := a.ServiceInstallPreviewV1(&transport.ServiceInstallPreviewRequest{Steps: []transport.ServiceInstallPreviewStep{{ID: "postgresql", Package: "postgresql-18"}}}, &preview); err != nil || preview.State != "known" {
		t.Fatalf("preview of postgresql-18: %v %+v", err, preview)
	}
	logJSON(t, "install preview of postgresql-18", preview)

	// 4. PostgreSQL 18 beside 17, on the port shown.
	if resp := a107Install(t, a, "postgresql", "postgresql-18"); resp.Error != "" {
		t.Fatalf("postgresql-18: %s", resp.Error)
	} else if !strings.Contains(resp.Detail, "cluster 18/main answers on port 5433") {
		t.Fatalf("the install did not prove the shown port: %q", resp.Detail)
	}
	side := a108Clusters(t)
	if c17, _ := a108Cluster(side, "17"); c17.Port != 5432 || !slices.Contains(c17.Databases, "a108shop") {
		t.Fatalf("17/main after the second install: %+v", c17)
	}
	if c18, ok := a108Cluster(side, "18"); !ok || c18.Port != eighteen.Port || !c18.Online {
		t.Fatalf("18/main = %+v, shown port %d", c18, eighteen.Port)
	}

	// 5. 18/main's port: 5432 is refused (in use), 5440 is shown, written and proved.
	var refused transport.PostgreSQLPortPreviewResponse
	a.PostgreSQLClusterPortPreviewV1(&transport.PostgreSQLPortPreviewRequest{PostgreSQLClusterRef: transport.PostgreSQLClusterRef{Version: "18", Name: "main"}, Port: 5432}, &refused)
	if refused.RefusalCode != "PORT_IN_USE" {
		t.Fatalf("port 5432 for 18/main: %+v", refused)
	}
	var plan transport.PostgreSQLPortPreviewResponse
	a.PostgreSQLClusterPortPreviewV1(&transport.PostgreSQLPortPreviewRequest{PostgreSQLClusterRef: transport.PostgreSQLClusterRef{Version: "18", Name: "main"}, Port: 5440}, &plan)
	logJSON(t, "port plan", plan)
	if plan.RefusalCode != "" || plan.ConfigSHA == "" || plan.From != 5433 || !strings.Contains(plan.LineAfter, "5440") {
		t.Fatalf("plan = %+v", plan)
	}
	var moved transport.PostgreSQLSetPortResponse
	a108Job(t, "pg_cluster_port", "pg-18-main", "5440", func(binding ServiceMutationBinding) bool {
		if err := a.SetPostgreSQLClusterPortV1(&transport.PostgreSQLSetPortRequest{ServiceMutationBinding: binding, PostgreSQLClusterRef: transport.PostgreSQLClusterRef{Version: "18", Name: "main"}, Port: 5440, ExpectedSHA256: plan.ConfigSHA}, &moved); err != nil {
			t.Fatal(err)
		}
		return moved.Proved
	})
	logJSON(t, "port change", moved)
	if !moved.Changed || !moved.Proved || moved.From != 5433 || moved.To != 5440 {
		t.Fatalf("port change = %+v", moved)
	}
	if got := a108Psql(t, "5440", "postgres", "SELECT current_setting('port') || ' ' || current_setting('server_version_num');"); !strings.HasPrefix(got, "5440 18") {
		t.Fatalf("18/main on 5440 says %q", got)
	}
	a107Out(t, "grep", "-n", "^port", "/etc/postgresql/18/main/postgresql.conf")

	// 6. a108shop copied 17 → 18 with proof; its role carried with its password.
	var copied transport.PostgreSQLCopyResponse
	a108Job(t, "pg_database_copy", "pgdb-a108shop", "5432-5440", func(binding ServiceMutationBinding) bool {
		if err := a.CopyPostgreSQLDatabaseV1(&transport.PostgreSQLCopyRequest{ServiceMutationBinding: binding, Database: "a108shop", SourcePort: 5432, TargetPort: 5440}, &copied); err != nil {
			t.Fatal(err)
		}
		return copied.Copied
	})
	logJSON(t, "copy", copied)
	if !copied.Copied || !copied.Proof.Equal || copied.Proof.TablesTotal != 2 || !slices.Contains(copied.Proof.RolesCreated, "a108_user") || copied.Proof.SourceVersion != "17" || copied.Proof.TargetVersion != "18" {
		t.Fatalf("copy = %+v", copied)
	}
	if got := a108Psql(t, "5440", "a108shop", "SELECT (SELECT count(*) FROM orders) || ' ' || (SELECT count(*) FROM shop.users) || ' ' || pg_get_userbyid((SELECT datdba FROM pg_database WHERE datname = current_database()));"); got != "1000 37 a108_user" {
		t.Fatalf("the copy holds %q", got)
	}
	signIn := exec.Command("psql", "--no-psqlrc", "-X", "-At", "-h", "127.0.0.1", "-p", "5440", "-U", "a108_user", "-d", "a108shop")
	signIn.Stdin = strings.NewReader("SELECT count(*) FROM orders;\n")
	signIn.Env = append(os.Environ(), "PGPASSWORD=a108-evidence-only")
	if out, err := signIn.CombinedOutput(); err != nil || strings.TrimSpace(string(out)) != "1000" {
		t.Fatalf("a108_user signs in to the copy with its own password: %v %s", err, out)
	}
	if entries, _ := os.ReadDir(pgDumpDir); len(entries) != 0 {
		t.Fatalf("the dump was left behind: %v", entries)
	}

	// 7. Removing postgresql-17 is refused while 17/main holds a108shop.
	var removal transport.ServiceRemovalPreviewResponse
	if err := a.ServiceRemovalPreviewV1(&transport.ServiceRemovalPreviewRequest{ID: "postgresql", Package: "postgresql-17"}, &removal); err != nil {
		t.Fatal(err)
	}
	logJSON(t, "removal preview of postgresql-17", removal)
	if !slices.Contains(removal.HoldsDatabases, "17/main: a108shop") {
		t.Fatalf("the preview does not name the database: %+v", removal)
	}
	ops, err := defaultServiceUninstallOps(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var refusedRemoval UninstallServiceResponse
	if err := a.uninstallServiceWithOps(&InstallServiceRequest{ID: "postgresql", Package: "postgresql-17"}, &refusedRemoval, ops); err != nil {
		t.Fatal(err)
	}
	logJSON(t, "removal while it holds a database", refusedRemoval)
	if refusedRemoval.ErrorCode != errCodeRemovalHoldsDatabases || refusedRemoval.Removed || a107Version("postgresql-17") == "" {
		t.Fatalf("removal = %+v", refusedRemoval)
	}

	// 8. The source dropped only as the explicit step; then the removal runs and keeps 17's data.
	var dropped transport.PostgreSQLDropResponse
	a108Job(t, "pg_database_drop", "pgdb-a108shop", "5432", func(binding ServiceMutationBinding) bool {
		if err := a.DropPostgreSQLDatabaseV1(&transport.PostgreSQLDropRequest{ServiceMutationBinding: binding, Database: "a108shop", Port: 5432, KeepPort: 5440}, &dropped); err != nil {
			t.Fatal(err)
		}
		return dropped.Dropped
	})
	logJSON(t, "drop source", dropped)
	if !dropped.Dropped {
		t.Fatalf("drop = %+v", dropped)
	}
	if got := a108Psql(t, "5432", "postgres", "SELECT count(*) FROM pg_database WHERE datname = 'a108shop';"); got != "0" {
		t.Fatalf("the source is still there: %q", got)
	}
	if got := a108Psql(t, "5440", "a108shop", "SELECT count(*) FROM orders;"); got != "1000" {
		t.Fatalf("the copy changed: %q", got)
	}
	var removed UninstallServiceResponse
	if err := a.uninstallServiceWithOps(&InstallServiceRequest{ID: "postgresql", Package: "postgresql-17"}, &removed, ops); err != nil {
		t.Fatal(err)
	}
	logJSON(t, "removal of postgresql-17 after the move", removed)
	if removed.Error != "" || !removed.Removed || a107Version("postgresql-17") != "" {
		t.Fatalf("removal = %+v", removed)
	}
	if _, err := os.Stat("/var/lib/postgresql/17/main/PG_VERSION"); err != nil {
		t.Fatalf("17's data did not stay: %v", err)
	}
	after := a108Clusters(t)
	if c18, ok := a108Cluster(after, "18"); !ok || c18.Port != 5440 || !c18.Online || !slices.Contains(c18.Databases, "a108shop") {
		t.Fatalf("18/main after removing 17: %+v", c18)
	}
}
