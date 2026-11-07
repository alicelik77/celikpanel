package main

import (
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/dbremote"
	"github.com/alicelik/celikpanel/internal/transport"
)

// The independent review of alpha.129's remote database access: each item
// has its test here.

// fakePGCluster answers the statements the PostgreSQL side sends and keeps
// the order of what changed.
type fakePGCluster struct {
	ssl, sslTakes bool
	listen        string
	preload       string
	pending       []string
	events        []string
	hbaLines      []string
	hbaText       string
}

func (f *fakePGCluster) query(_ int, sql string) (string, error) {
	switch {
	case strings.Contains(sql, "current_setting('hba_file')"):
		ssl := "off"
		if f.ssl {
			ssl = "on"
		}
		return strings.Join([]string{f.listen, ssl, "/etc/postgresql/17/main/pg_hba.conf", f.preload, "/nonexistent",
			strings.Join(f.pending, ","), "", "<none>", "on"}, "\x1f"), nil
	case strings.Contains(sql, "FROM pg_authid"):
		return "shop_u\x1ffalse\x1ftrue\x1ftrue\ndb\x1fshop\n", nil
	case strings.Contains(sql, "ALTER SYSTEM"):
		f.events = append(f.events, "alter")
		if strings.Contains(sql, "ssl = 'on'") && f.sslTakes {
			f.ssl = true
		}
		if strings.Contains(sql, "listen_addresses = '*'") {
			f.pending = append(f.pending, "listen_addresses")
		}
	}
	return "", nil
}

func withFakePG(t *testing.T, f *fakePGCluster) {
	t.Helper()
	restoreQuery, restoreWrite, restoreWait, restoreRoot, restoreRead := databaseRemotePGQuery, databaseRemoteWriteHBA, databaseRemotePGReloadWait, databaseRemoteCertRoot, databaseRemoteReadHBA
	databaseRemotePGQuery = f.query
	databaseRemoteWriteHBA = func(_ string, lines []string) error {
		f.events = append(f.events, "hba")
		f.hbaLines = append([]string(nil), lines...)
		return nil
	}
	databaseRemoteReadHBA = func(string) ([]byte, error) { return []byte(f.hbaText), nil }
	databaseRemotePGReloadWait = 0
	root := t.TempDir()
	databaseRemoteCertRoot = root
	if err := os.MkdirAll(filepath.Join(root, "postgresql"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"server.crt", "server.key"} {
		if err := os.WriteFile(filepath.Join(root, "postgresql", name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		databaseRemotePGQuery, databaseRemoteWriteHBA, databaseRemotePGReloadWait, databaseRemoteCertRoot, databaseRemoteReadHBA = restoreQuery, restoreWrite, restoreWait, restoreRoot, restoreRead
	})
}

func shopTarget() dbremote.Target {
	return dbremote.Target{Engine: dbremote.EnginePostgreSQL, Port: 5432, Rules: []dbremote.Rule{{Database: "shop", User: "shop_u", Address: "203.0.113.7/32"}}}
}

func problemCodes(result transport.DatabaseRemoteTargetResult) map[string]bool {
	codes := map[string]bool{}
	for _, p := range result.Problems {
		codes[p.Code] = true
	}
	return codes
}

// L4: a cluster with ssl = off gets TLS before any hostssl line is written.
func TestPGRemoteTurnsTLSOnBeforeWritingTheBlock(t *testing.T) {
	f := &fakePGCluster{listen: "localhost", sslTakes: true, hbaText: "local all postgres peer\n"}
	withFakePG(t, f)
	var ts dbremote.TargetState
	(pgRemoteInstance{}).apply(shopTarget(), dbRemoteFacts{running: true}, &ts, "db.example.com")
	if strings.Join(f.events, ",") != "alter,hba" || !f.ssl {
		t.Fatalf("events = %v, ssl = %v", f.events, f.ssl)
	}
}

func TestPGRemoteWritesNoLineWhileTLSIsOff(t *testing.T) {
	f := &fakePGCluster{listen: "localhost", sslTakes: false, hbaText: "local all postgres peer\n"}
	withFakePG(t, f)
	var ts dbremote.TargetState
	result := (pgRemoteInstance{}).apply(shopTarget(), dbRemoteFacts{running: true}, &ts, "db.example.com")
	if len(f.hbaLines) != 0 || len(result.Active) != 0 || !problemCodes(result)[transport.DatabaseRemoteTLSMissing] || ts.TLS {
		t.Fatalf("lines %v, active %v, problems %+v, tls %v", f.hbaLines, result.Active, result.Problems, ts.TLS)
	}
}

// M2: an owner's network line in pg_hba.conf means nothing is opened, and
// what CelikPanel opened before is closed.
func TestPGRemoteOpensNothingBehindAnOwnersNetworkLine(t *testing.T) {
	f := &fakePGCluster{listen: "localhost", ssl: true, sslTakes: true, hbaText: "local all postgres peer\nhost all all 10.0.0.0/8 md5\n"}
	withFakePG(t, f)
	var ts dbremote.TargetState
	result := (pgRemoteInstance{}).apply(shopTarget(), dbRemoteFacts{running: true}, &ts, "db.example.com")
	if len(f.hbaLines) != 0 || len(result.Active) != 0 || ts.Listen || !problemCodes(result)[transport.DatabaseRemoteOwnerHBA] {
		t.Fatalf("lines %v, active %v, listen %v, problems %+v", f.hbaLines, result.Active, ts.Listen, result.Problems)
	}
	for _, e := range f.events {
		if e == "alter" {
			t.Fatal("a setting was changed behind an owner's network line")
		}
	}
	// Opened before the owner added the line: the listen address goes back.
	f.events, f.listen = nil, "*"
	ts = dbremote.TargetState{Engine: "postgresql", Port: 5432, Listen: true}
	(pgRemoteInstance{}).apply(shopTarget(), dbRemoteFacts{running: true, listenRemote: true}, &ts, "db.example.com")
	if ts.Listen || !ts.ListenPending || len(f.hbaLines) != 0 {
		t.Fatalf("not closed: %+v, lines %v", ts, f.hbaLines)
	}
}

// M3: PostgreSQL rules wait until auth_delay is loaded; an owner's
// shared_preload_libraries is never rewritten, the guidance names it.
func TestPGRemoteRulesWaitForAuthDelay(t *testing.T) {
	f := &fakePGCluster{listen: "*", ssl: true, preload: "pg_stat_statements", hbaText: "local all postgres peer\n"}
	withFakePG(t, f)
	ts := dbremote.TargetState{Engine: "postgresql", Port: 5432, Listen: true}
	result := (pgRemoteInstance{}).apply(shopTarget(), dbRemoteFacts{running: true, listenRemote: true}, &ts, "db.example.com")
	if len(f.hbaLines) != 0 || len(result.Active) != 0 || ts.AuthDelay {
		t.Fatalf("lines %v, active %v, auth_delay %v", f.hbaLines, result.Active, ts.AuthDelay)
	}
	found := false
	for _, p := range result.Problems {
		found = found || (p.Code == transport.DatabaseRemoteNoSlowDown && strings.Contains(p.Detail, "'pg_stat_statements,auth_delay'"))
	}
	if !found || result.State != transport.DatabaseRemoteStateWaiting {
		t.Fatalf("state %s, problems %+v", result.State, result.Problems)
	}
	// Once auth_delay is loaded (the owner added it and restarted), the
	// rules are written.
	f.preload = "pg_stat_statements,auth_delay"
	result = (pgRemoteInstance{}).apply(shopTarget(), dbRemoteFacts{running: true, listenRemote: true}, &ts, "db.example.com")
	if len(f.hbaLines) != 1 || len(result.Active) != 1 || result.State != transport.DatabaseRemoteStateOpen || result.SlowDown != "auth_delay" {
		t.Fatalf("lines %v, state %s, slow %s, problems %+v", f.hbaLines, result.State, result.SlowDown, result.Problems)
	}
}

// M3: MariaDB copies wait until CelikPanel's Fail2ban jail runs.
func TestMariaDBRemoteCopiesWaitForTheJail(t *testing.T) {
	f := &fakeMariaDB{accounts: map[string]bool{"shop_u@localhost": true, "shop_u@203.0.113.7": true}, noJail: true}
	files := withFakeMariaDB(t, f)
	target := dbremote.Target{Engine: "mysql", Port: 3306, Rules: []dbremote.Rule{{Database: "shop", User: "shop_u", Address: "203.0.113.7/32"}}}
	ts := dbremote.TargetState{Engine: "mysql", Port: 3306, Accounts: []string{"shop_u@203.0.113.7"}}
	result := mariadbRemoteInstance{}.apply(target, dbRemoteFacts{running: true, listenRemote: true}, &ts, "")
	if len(result.Active) != 0 || f.accounts["shop_u@203.0.113.7"] || len(ts.Accounts) != 0 || !problemCodes(result)[transport.DatabaseRemoteNoSlowDown] {
		t.Fatalf("a copy exists without the jail: active %v, accounts %v, problems %+v", result.Active, f.accounts, result.Problems)
	}
	if string(files[dbremote.MariaDBFilterPath]) != dbremote.MariaDBFilter() {
		t.Fatal("CelikPanel's filter was not written")
	}
	f.noJail = false
	result = mariadbRemoteInstance{}.apply(target, dbRemoteFacts{running: true, listenRemote: true}, &ts, "")
	if len(result.Active) != 1 || !f.accounts["shop_u@203.0.113.7"] || result.SlowDown != "fail2ban" || result.State != transport.DatabaseRemoteStateOpen {
		t.Fatalf("with the jail: active %v, slow %s, state %s, problems %+v", result.Active, result.SlowDown, result.State, result.Problems)
	}
	// Without TLS on the running server the copies wait too.
	f.haveSSL = "NO"
	result = mariadbRemoteInstance{}.apply(target, dbRemoteFacts{running: true, listenRemote: true}, &ts, "")
	if len(result.Active) != 0 || f.accounts["shop_u@203.0.113.7"] {
		t.Fatalf("a copy exists without TLS: %v", f.accounts)
	}
}

// fakeFilterHost stands in for nft and systemctl and keeps the filter's
// files in a temporary directory.
func fakeFilterHost(t *testing.T) *fakeNFT {
	t.Helper()
	host := &fakeNFT{}
	dir := t.TempDir()
	restoreRun, restoreLook, restoreFiles, restoreState := databaseRemoteRun, databaseRemoteLookPath, databaseRemoteFilterFiles, databaseRemoteStatePath
	databaseRemoteLookPath = func(string) (string, error) { return "/usr/sbin/nft", nil }
	databaseRemoteRun = host.run
	databaseRemoteFilterFiles.nftPath = filepath.Join(dir, "database-remote.nft")
	databaseRemoteFilterFiles.unitPath = filepath.Join(dir, "unit.service")
	databaseRemoteFilterFiles.guardPath = filepath.Join(dir, "guard.sh")
	databaseRemoteFilterFiles.timerPath = filepath.Join(dir, "unit.timer")
	databaseRemoteStatePath = filepath.Join(dir, "database-remote.json")
	t.Cleanup(func() {
		databaseRemoteRun, databaseRemoteLookPath, databaseRemoteFilterFiles, databaseRemoteStatePath = restoreRun, restoreLook, restoreFiles, restoreState
	})
	return host
}

// fakeNFT is nft and systemctl: it keeps whether the table is loaded.
type fakeNFT struct {
	loaded bool
	calls  []string
}

func (h *fakeNFT) run(name string, args []string, _ string) ([]byte, error) {
	call := name + " " + strings.Join(args, " ")
	h.calls = append(h.calls, call)
	switch {
	case strings.HasSuffix(name, "nft") && len(args) == 2 && args[0] == "-f":
		h.loaded = true
	case strings.HasSuffix(name, "nft") && strings.HasPrefix(call, name+" delete table"):
		h.loaded = false
	case strings.HasSuffix(name, "nft") && strings.HasPrefix(call, name+" list tables") && h.loaded:
		return []byte("table inet celikpanel_dbremote\n"), nil
	case strings.HasSuffix(name, "nft") && strings.HasPrefix(call, name+" list chain"):
		if !h.loaded {
			return []byte("Error: No such file or directory"), errors.New("exit status 1")
		}
		return []byte("tcp dport 5432 drop\n"), nil
	}
	return nil, nil
}

// L2: the filter is loaded, verified, guarded by a timer; a flushed
// ruleset is noticed (FilterLoaded false) and the next write loads it again.
func TestDatabaseRemoteFilterIsGuardedAndVerified(t *testing.T) {
	host := fakeFilterHost(t)
	ports := []dbremote.NFTPort{{Port: 5432, V4: []netip.Prefix{netip.MustParsePrefix("203.0.113.7/32")}}}
	if err := writeDatabaseRemoteFilter("/usr/sbin/nft", ports); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(host.calls, "\n")
	for _, want := range []string{"systemctl enable celikpanel-database-remote-firewall.service", "systemctl enable --now celikpanel-database-remote-firewall.timer"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in\n%s", want, joined)
		}
	}
	guard, err := os.ReadFile(databaseRemoteFilterFiles.guardPath)
	if err != nil || !strings.Contains(string(guard), "list chain inet celikpanel_dbremote input") {
		t.Fatalf("guard = %s, %v", guard, err)
	}
	if !databaseRemoteFilterLoaded("/usr/sbin/nft") {
		t.Fatal("not loaded after the write")
	}
	host.loaded = false // nft flush ruleset
	if databaseRemoteFilterLoaded("/usr/sbin/nft") {
		t.Fatal("a flushed filter reads as loaded")
	}
	if err := writeDatabaseRemoteFilter("/usr/sbin/nft", ports); err != nil || !host.loaded {
		t.Fatalf("not loaded again: %v", err)
	}
	if err := writeDatabaseRemoteFilter("/usr/sbin/nft", nil); err != nil || host.loaded {
		t.Fatalf("not removed: %v", err)
	}
	for _, path := range []string{databaseRemoteFilterFiles.guardPath, databaseRemoteFilterFiles.timerPath, databaseRemoteFilterFiles.unitPath, databaseRemoteFilterFiles.nftPath} {
		if _, err := os.Stat(path); err == nil {
			t.Errorf("%s left behind", path)
		}
	}
}

// H1: an engine whose listen address the owner set, or whose record was
// lost after CelikPanel set it, is left alone and its port is never named
// in FilterPorts, so the panel firewall never opens it.
func TestApplyDatabaseRemoteNamesNoPortForAnInstanceItLeavesAlone(t *testing.T) {
	for _, c := range []struct{ name, file, value string }{
		{"owner's postgresql.conf", "/etc/postgresql/17/main/postgresql.conf", "*"},
		{"record lost", "/var/lib/postgresql/17/main/postgresql.auto.conf", "*"},
	} {
		t.Run(c.name, func(t *testing.T) {
			fakeFilterHost(t)
			f := &fakePGCluster{listen: "*", ssl: true, preload: "auth_delay", hbaText: "local all postgres peer\n"}
			withFakePG(t, f)
			databaseRemotePGQuery = func(port int, sql string) (string, error) {
				if strings.Contains(sql, "current_setting('hba_file')") {
					return strings.Join([]string{"*", "on", "/etc/postgresql/17/main/pg_hba.conf", "auth_delay", "/nonexistent", "", c.file + "\x1e" + c.value, "<none>", "on"}, "\x1f"), nil
				}
				return f.query(port, sql)
			}
			var resp transport.DatabaseRemoteResponse
			applyDatabaseRemote([]dbremote.Target{shopTarget()}, "db.example.com", nil, &resp)
			if len(resp.FilterPorts) != 0 || len(resp.Targets) != 1 || resp.Targets[0].State != transport.DatabaseRemoteStateOwnerManaged ||
				len(resp.Targets[0].Active) != 0 || len(f.hbaLines) != 0 {
				t.Fatalf("resp = %+v, lines %v", resp, f.hbaLines)
			}
		})
	}
}
