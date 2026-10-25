package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.108: PostgreSQL majors side by side. Mounted tests: the clusters'
// configuration lives in a temporary /etc/postgresql, pg_lsclusters and the
// clusters' answers are recorded, nothing runs on the test host.

const fakeLsclusters = `17  main    5432 online postgres /var/lib/postgresql/17/main /var/log/postgresql/postgresql-17-main.log
18  main    5433 online postgres /var/lib/postgresql/18/main /var/log/postgresql/postgresql-18-main.log
16  old     5434 down   postgres /var/lib/postgresql/16/old  /var/log/postgresql/postgresql-16-old.log
bad line
19  ma;n    5435 online postgres /x /y
`

type fakePGHost struct {
	t         *testing.T
	root      string
	ls        string
	answers   func(owner string, port int, database, statement string) (string, error)
	statement []string
	restarts  []string
	writes    map[string]string
	listening map[int]bool
	streams   []string
	distro    map[string]map[string]bool
	vendor    map[string]map[string]bool
	vendorOn  bool
	installed map[string][]string
	restart   func(version, name string) error
	streamFn  func(client string, stdin io.Reader, stdout io.Writer, args []string) error
}

func newFakePGHost(t *testing.T) *fakePGHost {
	t.Helper()
	root := t.TempDir()
	oldConf, oldCommon, oldTool := pgConfRoot, pgCommonConfPath, pgPhpPgAdminConf
	pgConfRoot = filepath.Join(root, "etc", "postgresql")
	pgCommonConfPath = filepath.Join(root, "etc", "postgresql-common", "createcluster.conf")
	pgPhpPgAdminConf = filepath.Join(root, "etc", "phppgadmin", "config.inc.php")
	t.Cleanup(func() { pgConfRoot, pgCommonConfPath, pgPhpPgAdminConf = oldConf, oldCommon, oldTool })
	return &fakePGHost{t: t, root: root, ls: fakeLsclusters, writes: map[string]string{}, listening: map[int]bool{22: true}, installed: map[string][]string{"postgresql-17": {"17.6-0+deb13u1"}, "postgresql-18": {"18.1-1.pgdg13+1"}}}
}

func (f *fakePGHost) writeConf(version, name, content string) string {
	path := filepath.Join(pgConfRoot, version, name, "postgresql.conf")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		f.t.Fatal(err)
	}
	return path
}

func (f *fakePGHost) host() pgClusterHost {
	return pgClusterHost{
		family:     func() string { return "apt" },
		lsclusters: func(context.Context) (string, error) { return f.ls, nil },
		query: func(_ context.Context, owner string, port int, database, statement string) (string, error) {
			f.statement = append(f.statement, statement)
			if f.answers == nil {
				return "", errors.New("no answer")
			}
			return f.answers(owner, port, database, statement)
		},
		restart: func(_ context.Context, version, name string) error {
			f.restarts = append(f.restarts, version+"/"+name)
			if f.restart != nil {
				return f.restart(version, name)
			}
			return nil
		},
		readFile: os.ReadFile,
		writeFile: func(path string, content []byte) error {
			f.writes[path] = string(content)
			return os.WriteFile(path, content, 0o644)
		},
		listening:      func() map[int]bool { return f.listening },
		distroVersions: func(map[string]bool) (map[string]map[string]bool, error) { return f.distro, nil },
		vendorIndex:    func() (map[string]map[string]bool, bool) { return f.vendor, f.vendorOn },
		installed:      func() map[string][]string { return f.installed },
		now:            func() time.Time { return time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC) },
		stream: func(_ context.Context, owner, client string, stdin io.Reader, stdout io.Writer, args ...string) error {
			f.streams = append(f.streams, client+" "+strings.Join(args, " "))
			if f.streamFn != nil {
				return f.streamFn(client, stdin, stdout, args)
			}
			return nil
		},
		freeBytes: func(string) (int64, error) { return 1 << 40, nil },
	}
}

func TestPGLsclustersAreReadAndOddLinesLeftOut(t *testing.T) {
	clusters := parsePGLsClusters(fakeLsclusters)
	if len(clusters) != 3 {
		t.Fatalf("clusters = %+v", clusters)
	}
	if clusters[0].Version != "16" || clusters[0].Online || clusters[1].Version != "17" || !clusters[1].Online || clusters[2].Port != 5433 {
		t.Fatalf("order and state: %+v", clusters)
	}
	if clusters[1].DataDirectory != "/var/lib/postgresql/17/main" || !strings.HasSuffix(clusters[1].ConfigPath, filepath.Join("17", "main", "postgresql.conf")) {
		t.Fatalf("paths: %+v", clusters[1])
	}
}

func TestPGPortLineIsChangedAloneAndKeepsItsComment(t *testing.T) {
	conf := "data_directory = '/var/lib/postgresql/17/main'\n#port = 1\nport = 5432\t\t\t\t# (change requires restart)\nmax_connections = 100\n"
	port, index := pgConfPort(conf)
	if port != 5432 || index != 2 {
		t.Fatalf("port %d at %d", port, index)
	}
	updated, before, after := pgConfWithPort(conf, 5440)
	if before != "port = 5432\t\t\t\t# (change requires restart)" || after != "port = 5440\t\t\t\t# (change requires restart)" {
		t.Fatalf("lines %q -> %q", before, after)
	}
	if strings.Replace(updated, "5440", "5432", 1) != conf {
		t.Fatalf("other lines changed:\n%s", updated)
	}
	if quoted, _, after := pgConfWithPort("port = '5432'\n", 5441); quoted != "port = '5441'\n" || after != "port = '5441'" {
		t.Fatalf("quoted: %q", quoted)
	}
	if appended, before, _ := pgConfWithPort("max_connections = 100", 5442); appended != "max_connections = 100\nport = 5442\n" || before != "" {
		t.Fatalf("appended: %q", appended)
	}
	// The last active line wins, as in PostgreSQL.
	if port, _ := pgConfPort("port = 5432\nport = 5499\n"); port != 5499 {
		t.Fatalf("last line: %d", port)
	}
}

func TestPGNextFreePortIsPostgresqlCommonsRule(t *testing.T) {
	listening := map[int]bool{}
	parseProcNetListening(`  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 00000000:1538 00000000:0000 0A 00000000:00000000 00:00000000 00000000   113        0 1 1 0000000000000000 100 0 0 10 0
   1: 0100007F:1539 0100007F:9999 01 00000000:00000000 00:00000000 00000000   113        0 1 1 0000000000000000 100 0 0 10 0
   2: 0100007F:153A 00000000:0000 0A 00000000:00000000 00:00000000 00000000   113        0 1 1 0000000000000000 100 0 0 10 0
`, listening)
	if !reflect.DeepEqual(listening, map[int]bool{5432: true, 5434: true}) {
		t.Fatalf("listening = %v", listening)
	}
	if port := nextFreePGPort(map[int]bool{5432: true, 5433: true}, listening); port != 5435 {
		t.Fatalf("next = %d", port)
	}
	if port := nextFreePGPort(map[int]bool{}, map[int]bool{}); port != 5432 {
		t.Fatalf("empty = %d", port)
	}
}

func TestPGClustersReadNamesDatabasesOffersAndThePortAnInstallWouldGet(t *testing.T) {
	f := newFakePGHost(t)
	f.writeConf("17", "main", "port = 5432\n")
	f.writeConf("18", "main", "port = 5433\n")
	f.writeConf("16", "old", "port = 5434\n")
	// A cluster of 15 kept from an earlier removal, on its own port.
	f.writeConf("15", "main", "port = 5436\n")
	f.listening[5435] = true
	f.distro = map[string]map[string]bool{"postgresql-17": {"17.6-0+deb13u1": true}}
	f.vendor = map[string]map[string]bool{"postgresql-16": {"16.10-1.pgdg13+1": true}, "postgresql-15": {"15.14-1.pgdg13+1": true}}
	f.vendorOn = true
	f.answers = func(owner string, port int, database, statement string) (string, error) {
		if owner != "postgres" || database != "postgres" || statement != pgUserDatabasesStatement {
			t.Fatalf("asked %s %d %s %q", owner, port, database, statement)
		}
		if port == 5432 {
			return "shop\nblog\n", nil
		}
		return "", nil
	}
	resp := pgClustersRead(context.Background(), f.host())
	if !resp.Supported || len(resp.Clusters) != 3 || resp.NextPort != 5437 || !resp.CreatesCluster || !resp.RepositoryOn {
		t.Fatalf("resp = %+v", resp)
	}
	byRef := map[string]transport.PostgreSQLCluster{}
	for _, cluster := range resp.Clusters {
		byRef[cluster.Version+"/"+cluster.Name] = cluster
	}
	if got := byRef["17/main"]; !got.DatabasesKnown || !reflect.DeepEqual(got.Databases, []string{"shop", "blog"}) || got.ConfiguredPort != 5432 {
		t.Fatalf("17/main = %+v", got)
	}
	if got := byRef["16/old"]; got.DatabasesKnown || got.DatabasesError == "" {
		t.Fatalf("a cluster that is down is not known, never empty: %+v", got)
	}
	offers := map[string]transport.PostgreSQLMajorOffer{}
	for _, offer := range resp.Offers {
		offers[offer.Major] = offer
	}
	if _, ok := offers["17"]; ok {
		t.Fatal("an installed major is offered")
	}
	if got := offers["16"]; got.Origin != "pgdg" || got.Package != "postgresql-16" || got.Port != 5437 {
		t.Fatalf("16 = %+v", got)
	}
	if got := offers["15"]; !got.KeptCluster || got.Port != 5436 {
		t.Fatalf("15 with its kept cluster = %+v", got)
	}
	// 14 is still supported upstream (until 2026-11-12) but nothing here
	// offers it: listed as not offered. 13 ended: not listed.
	if got := offers["14"]; got.Major != "14" || got.Origin != "" || got.NeedsRepository {
		t.Fatalf("14 = %+v", got)
	}
	if _, ok := offers["13"]; ok {
		t.Fatal("13, out of support and offered by nothing, is listed")
	}
	// Without PGDG, a major only PGDG ships needs the repository.
	f.vendorOn, f.vendor = false, nil
	if err := os.MkdirAll(filepath.Dir(pgCommonConfPath), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(pgCommonConfPath, []byte("# owner's\ncreate_main_cluster = false\n"), 0o644)
	resp = pgClustersRead(context.Background(), f.host())
	for _, offer := range resp.Offers {
		if offer.Major == "16" && (!offer.NeedsRepository || offer.Origin != "") {
			t.Fatalf("16 without PGDG = %+v", offer)
		}
	}
	if resp.CreatesCluster {
		t.Fatal("the owner's create_main_cluster = false was not read")
	}
}

func TestPGClustersAreNotOfferedOutsidePostgresqlCommon(t *testing.T) {
	f := newFakePGHost(t)
	host := f.host()
	host.family = func() string { return "pacman" }
	if resp := pgClustersRead(context.Background(), host); resp.Supported || !strings.Contains(resp.Reason, "postgresql-common") {
		t.Fatalf("pacman = %+v", resp)
	}
	host.family = func() string { return "apt" }
	host.lsclusters = func(context.Context) (string, error) { return "", errors.New("not found") }
	if resp := pgClustersRead(context.Background(), host); resp.Supported || !strings.Contains(resp.Reason, "install PostgreSQL first") {
		t.Fatalf("no postgresql-common = %+v", resp)
	}
}

// proofAnswers answers the clusters' proof as a cluster on its port would.
func proofAnswers(t *testing.T, live map[int]string) func(string, int, string, string) (string, error) {
	return func(owner string, port int, database, statement string) (string, error) {
		switch {
		case strings.Contains(statement, "pg_stat_activity"):
			return "shop|shop_user|local|psql|2\n", nil
		case strings.Contains(statement, "current_setting('port')"):
			version, ok := live[port]
			if !ok {
				return "", errors.New("connection refused")
			}
			return strings.Replace(version, "PORT", strconv.Itoa(port), 1), nil
		}
		t.Fatalf("unexpected statement %q", statement)
		return "", nil
	}
}

// pgProofBoundForTests shortens the proof's wait for one test.
func pgProofBoundForTests(t *testing.T, bound time.Duration) time.Duration {
	previous := pgProofBound
	pgProofBound = bound
	t.Cleanup(func() { pgProofBound = previous })
	return previous
}

func TestPGPortChangeIsShownFirstWrittenOverTheSeenFileAndProved(t *testing.T) {
	f := newFakePGHost(t)
	conf := "data_directory = '/var/lib/postgresql/17/main'\nport = 5432\t\t# (change requires restart)\n"
	path := f.writeConf("17", "main", conf)
	f.writeConf("18", "main", "port = 5433\n")
	os.MkdirAll(filepath.Dir(pgPhpPgAdminConf), 0o755)
	os.WriteFile(pgPhpPgAdminConf, []byte("$conf['servers'][0]['port'] = 5432;\n"), 0o644)
	// The cluster answers on 5432 until it is restarted, then on 5440.
	live := map[int]string{5432: "PORT|170006|/var/lib/postgresql/17/main"}
	f.answers = proofAnswers(t, live)
	f.restart = func(string, string) error {
		delete(live, 5432)
		data, _ := os.ReadFile(path)
		port, _ := pgConfPort(string(data))
		live[port] = "PORT|170006|/var/lib/postgresql/17/main"
		return nil
	}
	plan, _, sha := pgPortPlan(context.Background(), f.host(), "17", "main", 5440)
	if plan.RefusalCode != "" || plan.From != 5432 || plan.To != 5440 || sha == "" || plan.ToolPort != 5432 || !plan.SessionsOK || len(plan.Sessions) != 1 || plan.Sessions[0].Count != 2 {
		t.Fatalf("plan = %+v", plan)
	}
	if plan.LineAfter != "port = 5440\t\t# (change requires restart)" {
		t.Fatalf("line after = %q", plan.LineAfter)
	}
	// A file that changed after it was shown is not written.
	var resp transport.PostgreSQLSetPortResponse
	setPGClusterPort(context.Background(), f.host(), &transport.PostgreSQLSetPortRequest{PostgreSQLClusterRef: transport.PostgreSQLClusterRef{Version: "17", Name: "main"}, Port: 5440, ExpectedSHA256: strings.Repeat("0", 64)}, &resp)
	if resp.ErrorCode != "CONFIG_CHANGED" || len(f.writes) != 0 || len(f.restarts) != 0 {
		t.Fatalf("changed file = %+v %v", resp, f.writes)
	}
	resp = transport.PostgreSQLSetPortResponse{}
	setPGClusterPort(context.Background(), f.host(), &transport.PostgreSQLSetPortRequest{PostgreSQLClusterRef: transport.PostgreSQLClusterRef{Version: "17", Name: "main"}, Port: 5440, ExpectedSHA256: sha}, &resp)
	if !resp.Changed || !resp.Proved || resp.From != 5432 || resp.To != 5440 || resp.Error != "" {
		t.Fatalf("change = %+v", resp)
	}
	if got := f.writes[path]; got != strings.Replace(conf, "5432", "5440", 1) || !reflect.DeepEqual(f.restarts, []string{"17/main"}) {
		t.Fatalf("written %q, restarts %v", got, f.restarts)
	}
}

func TestPGPortChangeThatDoesNotAnswerIsPutBackAndProvedOnTheOldPort(t *testing.T) {
	if testing.Short() {
		t.Skip("waits for the proof's bound")
	}
	f := newFakePGHost(t)
	conf := "port = 5432\n"
	path := f.writeConf("17", "main", conf)
	live := map[int]string{5432: "PORT|170006|/var/lib/postgresql/17/main"}
	f.answers = proofAnswers(t, live)
	// The new port never answers (something else came up on it).
	f.restart = func(string, string) error {
		data, _ := os.ReadFile(path)
		if port, _ := pgConfPort(string(data)); port == 5432 {
			live[5432] = "PORT|170006|/var/lib/postgresql/17/main"
		} else {
			delete(live, 5432)
		}
		return nil
	}
	previous := pgProofBoundForTests(t, 50*time.Millisecond)
	_ = previous
	_, _, sha := pgPortPlan(context.Background(), f.host(), "17", "main", 5441)
	var resp transport.PostgreSQLSetPortResponse
	setPGClusterPort(context.Background(), f.host(), &transport.PostgreSQLSetPortRequest{PostgreSQLClusterRef: transport.PostgreSQLClusterRef{Version: "17", Name: "main"}, Port: 5441, ExpectedSHA256: sha}, &resp)
	if resp.Changed || resp.Proved || !resp.RolledBack || !resp.Restored || resp.ErrorCode != "PROOF_FAILED" || !strings.Contains(resp.Error, "answers on port 5432 again") {
		t.Fatalf("rollback = %+v", resp)
	}
	if data, _ := os.ReadFile(path); string(data) != conf || len(f.restarts) != 2 {
		t.Fatalf("file %q restarts %v", data, f.restarts)
	}
}

func TestPGPortChangeRefusesWhatItMustNotDo(t *testing.T) {
	f := newFakePGHost(t)
	f.writeConf("17", "main", "port = 5432\n")
	f.writeConf("18", "main", "port = 5433\n")
	f.listening[5450] = true
	f.answers = proofAnswers(t, map[int]string{})
	cases := map[int]string{80: "PORT_INVALID", 5432: "PORT_UNCHANGED", 5433: "PORT_IN_USE", 5434: "PORT_IN_USE", 5450: "PORT_IN_USE"}
	for port, code := range cases {
		plan, _, _ := pgPortPlan(context.Background(), f.host(), "17", "main", port)
		if plan.RefusalCode != code {
			t.Errorf("port %d: %s %q", port, plan.RefusalCode, plan.Refusal)
		}
	}
	// The port set in conf.d (the owner's file) wins: CelikPanel does not edit it.
	os.MkdirAll(filepath.Join(pgConfRoot, "17", "main", "conf.d"), 0o755)
	os.WriteFile(filepath.Join(pgConfRoot, "17", "main", "conf.d", "owner.conf"), []byte("port = 5432\n"), 0o644)
	if plan, _, _ := pgPortPlan(context.Background(), f.host(), "17", "main", 5460); plan.RefusalCode != "PORT_SET_ELSEWHERE" || !strings.Contains(plan.Refusal, "owner.conf") {
		t.Fatalf("conf.d = %+v", plan)
	}
	if plan, _, _ := pgPortPlan(context.Background(), f.host(), "17", "../x", 5460); plan.RefusalCode != "INVALID_REQUEST" {
		t.Fatalf("bad name = %+v", plan)
	}
	if plan, _, _ := pgPortPlan(context.Background(), f.host(), "16", "old", 5460); plan.RefusalCode != "CONFIG_UNREADABLE" && plan.RefusalCode != "CLUSTER_DOWN" {
		t.Fatalf("down cluster = %+v", plan)
	}
	if len(f.writes) != 0 || len(f.restarts) != 0 {
		t.Fatal("a refused plan changed something")
	}
}

func TestPGRemovalIsRefusedWhileAClusterHoldsDatabases(t *testing.T) {
	f := newFakePGHost(t)
	f.answers = func(_ string, port int, _, _ string) (string, error) {
		if port == 5432 {
			return "shop\n", nil
		}
		return "", nil
	}
	held, unknown := pgRemovalHolds(context.Background(), f.host(), "17")
	if !reflect.DeepEqual(held, []string{"17/main: shop"}) || len(unknown) != 0 {
		t.Fatalf("17: %v %v", held, unknown)
	}
	if held, unknown := pgRemovalHolds(context.Background(), f.host(), "18"); len(held) != 0 || len(unknown) != 0 {
		t.Fatalf("18 holds nothing: %v %v", held, unknown)
	}
	if held, unknown := pgRemovalHolds(context.Background(), f.host(), ""); len(held) != 1 || !reflect.DeepEqual(unknown, []string{"16/old"}) {
		t.Fatalf("every major: %v %v", held, unknown)
	}
	code, message := pgRemovalRefusal([]string{"17/main: shop"}, nil)
	if code != errCodeRemovalHoldsDatabases || !strings.Contains(message, "17/main: shop") || !strings.Contains(message, "nothing was changed") {
		t.Fatalf("refusal = %s %s", code, message)
	}

	// The uninstall refuses before it stops or removes anything.
	removed := false
	ops := serviceUninstallOps{
		detectPackageFamily: func() string { return "apt" },
		packageInstalled:    func(pkg string) bool { return pkg == "postgresql-17" },
		unitExists:          func(string) bool { return true },
		unitsMatching:       func(string) []string { return nil },
		disableUnit: func(string) error {
			t.Fatal("a unit was stopped")
			return nil
		},
		removePackages: func(string, packageRemovalRequest, removalHooks) (packageRemovalPlan, string, error) {
			removed = true
			return packageRemovalPlan{}, "", nil
		},
		postgresqlHolds: func(major string) ([]string, []string) {
			if major != "17" {
				t.Fatalf("major = %q", major)
			}
			return []string{"17/main: shop"}, nil
		},
	}
	var response UninstallServiceResponse
	if err := (&Agent{}).uninstallServiceWithOps(&InstallServiceRequest{ID: "postgresql", Package: "postgresql-17"}, &response, ops); err != nil {
		t.Fatal(err)
	}
	if response.ErrorCode != errCodeRemovalHoldsDatabases || removed || response.Removed {
		t.Fatalf("response = %+v", response)
	}
	if major, ok := pgRemovalMajor(core.GetManagedServiceByID("mariadb"), ""); ok || major != "" {
		t.Fatal("MariaDB is not a PostgreSQL removal")
	}
}

func TestPGInstallProofNamesThePortOrSaysNoClusterWasMade(t *testing.T) {
	f := newFakePGHost(t)
	f.answers = proofAnswers(t, map[int]string{5433: "PORT|180001|/var/lib/postgresql/18/main"})
	if got, err := pgInstallProofWith(context.Background(), f.host(), "postgresql-18"); err != nil || got != "cluster 18/main answers on port 5433" {
		t.Fatalf("18 = %q %v", got, err)
	}
	f.ls = "17  main    5432 online postgres /var/lib/postgresql/17/main log\n"
	if _, err := pgInstallProofWith(context.Background(), f.host(), "postgresql-18"); err == nil {
		t.Fatal("a missing cluster was not said")
	}
	os.MkdirAll(filepath.Dir(pgCommonConfPath), 0o755)
	os.WriteFile(pgCommonConfPath, []byte("create_main_cluster = false\n"), 0o644)
	if got, err := pgInstallProofWith(context.Background(), f.host(), "postgresql-18"); err != nil || !strings.Contains(got, "create_main_cluster = false") {
		t.Fatalf("owner's choice = %q %v", got, err)
	}
}

func TestPGStatementsGoOnStdinWithThePort(t *testing.T) {
	command := newPostgreSQLStatementCommandOn(5433, "CREATE USER x WITH PASSWORD 'secret';")
	args := strings.Join(command.Args, " ")
	if strings.Contains(args, "secret") || !strings.Contains(args, "--port 5433") || !strings.Contains(args, "ON_ERROR_STOP=on") {
		t.Fatalf("args = %v", command.Args)
	}
	if _, err := postgreSQLStatementFor(70000); err == nil {
		t.Fatal("an invalid port was accepted")
	}
	var got []string
	previous := postgreSQLExecStatementOnPort
	postgreSQLExecStatementOnPort = func(port int, statement string) ([]byte, error) {
		got = append(got, strconv.Itoa(port)+" "+statement)
		return nil, nil
	}
	t.Cleanup(func() { postgreSQLExecStatementOnPort = previous })
	var resp CreateDatabaseResponse
	if err := (&Agent{}).CreateDatabase(CreateDatabaseRequest{Type: "postgresql", Name: "shop", User: "shop_user", Password: "pw", Port: 5433}, &resp); err != nil || !resp.Success {
		t.Fatalf("create = %+v %v", resp, err)
	}
	if len(got) != 2 || !strings.HasPrefix(got[0], "5433 CREATE USER") || !strings.HasPrefix(got[1], "5433 CREATE DATABASE") {
		t.Fatalf("statements = %v", got)
	}
}
