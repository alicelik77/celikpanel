package main

import (
	"errors"
	"io"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/dbremote"
	"github.com/alicelik/celikpanel/internal/firewallpolicy"
	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.130 remote database access: MariaDB host name lookup, the
// PostgreSQL Fail2ban layer, closing at the filter (D5) and the panel
// firewall after a flush (D6).

func mariadbShopRule() dbremote.Target {
	return dbremote.Target{Engine: "mysql", Port: 3306, Rules: []dbremote.Rule{{Database: "shop", User: "shop_u", Address: "203.0.113.7/32"}}}
}

// Item 1: while MariaDB looks host names up, a refused sign-in names a host
// the jail cannot ban, so no copy is made and the administrator is asked.
func TestMariaDBRulesWaitWhileHostNamesAreLookedUp(t *testing.T) {
	f := &fakeMariaDB{accounts: map[string]bool{"shop_u@localhost": true}, nameLookup: "0"}
	files := withFakeMariaDB(t, f)
	ts := dbremote.TargetState{Engine: "mysql", Port: 3306}
	result := mariadbRemoteInstance{}.apply(mariadbShopRule(), dbRemoteFacts{running: true, listenRemote: true}, &ts, "")
	if len(result.Active) != 0 || f.accounts["shop_u@203.0.113.7"] || !problemCodes(result)[transport.DatabaseRemoteNameResolve] || result.NameResolve != "on" {
		t.Fatalf("a copy exists while names are logged: active %v, accounts %v, problems %+v", result.Active, f.accounts, result.Problems)
	}
	if strings.Contains(string(files[dbremote.MariaDBOptionFileDebian]), "skip-name-resolve") || ts.SkipNameResolve {
		t.Fatal("host name lookup was turned off without the administrator's agreement")
	}
}

// With the agreement and no account named by host, CelikPanel's own option
// file turns the lookup off; the rules wait for the restart and open after.
func TestMariaDBHostNameLookupGoesOffWithTheAgreement(t *testing.T) {
	f := &fakeMariaDB{accounts: map[string]bool{"shop_u@localhost": true}, nameLookup: "0"}
	files := withFakeMariaDB(t, f)
	databaseRemoteMariaDBSkipNameResolve = true
	ts := dbremote.TargetState{Engine: "mysql", Port: 3306}
	result := mariadbRemoteInstance{}.apply(mariadbShopRule(), dbRemoteFacts{running: true, listenRemote: true}, &ts, "")
	if !dbremote.MariaDBOptionFileSkipsNameResolve(string(files[dbremote.MariaDBOptionFileDebian])) || !ts.SkipNameResolve {
		t.Fatalf("option file = %q, record %+v", files[dbremote.MariaDBOptionFileDebian], ts)
	}
	if result.State != transport.DatabaseRemoteStateWaitRestart || result.NameResolve != "pending_restart" || len(result.Active) != 0 {
		t.Fatalf("before the restart: state %s, name %s, active %v", result.State, result.NameResolve, result.Active)
	}
	f.nameLookup = "1" // MariaDB restarted
	result = mariadbRemoteInstance{}.apply(mariadbShopRule(), dbRemoteFacts{running: true, listenRemote: true}, &ts, "")
	if len(result.Active) != 1 || result.State != transport.DatabaseRemoteStateOpen || result.NameResolve != "off" {
		t.Fatalf("after the restart: state %s, active %v, problems %+v", result.State, result.Active, result.Problems)
	}
	// The last rule goes: the file (and the setting in it) goes with it.
	result = mariadbRemoteInstance{}.apply(dbremote.Target{Engine: "mysql", Port: 3306}, dbRemoteFacts{running: true, listenRemote: true}, &ts, "")
	if _, ok := files[dbremote.MariaDBOptionFileDebian]; ok || ts.SkipNameResolve {
		t.Fatalf("closing left the setting: %+v", ts)
	}
}

// An account written with a host name would stop working: the lookup stays
// on and the administrator is shown which accounts.
func TestMariaDBHostNameLookupStaysOnForHostNameAccounts(t *testing.T) {
	f := &fakeMariaDB{accounts: map[string]bool{"shop_u@localhost": true}, nameLookup: "0", hostAccounts: []string{"app@app.example.com"}}
	files := withFakeMariaDB(t, f)
	databaseRemoteMariaDBSkipNameResolve = true
	ts := dbremote.TargetState{Engine: "mysql", Port: 3306}
	result := mariadbRemoteInstance{}.apply(mariadbShopRule(), dbRemoteFacts{running: true, listenRemote: true}, &ts, "")
	if dbremote.MariaDBOptionFileSkipsNameResolve(string(files[dbremote.MariaDBOptionFileDebian])) || ts.SkipNameResolve {
		t.Fatal("the lookup was turned off although an account names a host")
	}
	found := false
	for _, p := range result.Problems {
		found = found || (p.Code == transport.DatabaseRemoteNameAccounts && strings.Contains(p.Detail, "'app'@'app.example.com'"))
	}
	if !found || len(result.Active) != 0 {
		t.Fatalf("problems %+v, active %v", result.Problems, result.Active)
	}
}

// The owner's own setting is never overridden.
func TestMariaDBHostNameLookupLeavesTheOwnersSetting(t *testing.T) {
	f := &fakeMariaDB{accounts: map[string]bool{"shop_u@localhost": true}, nameLookup: "0"}
	files := withFakeMariaDB(t, f)
	databaseRemoteMariaDBSkipNameResolve = true
	files["/etc/mysql/mariadb.conf.d/60-owner.cnf"] = []byte("[mysqld]\nskip-name-resolve = 0\n")
	databaseRemoteMariaDBConfigFiles = func() []string { return []string{"/etc/mysql/mariadb.conf.d/60-owner.cnf"} }
	ts := dbremote.TargetState{Engine: "mysql", Port: 3306}
	result := mariadbRemoteInstance{}.apply(mariadbShopRule(), dbRemoteFacts{running: true, listenRemote: true}, &ts, "")
	if dbremote.MariaDBOptionFileSkipsNameResolve(string(files[dbremote.MariaDBOptionFileDebian])) || !problemCodes(result)[transport.DatabaseRemoteOwnerNameResolve] {
		t.Fatalf("owner's setting overridden or not reported: %+v", result.Problems)
	}
}

func TestMariaDBNameResolveSettingAndAddressForms(t *testing.T) {
	for text, want := range map[string]string{
		"[mysqld]\nskip-name-resolve\n":                      "lookup_off",
		"[mysqld]\nskip_name_resolve = ON\n":                 "lookup_off",
		"[mariadbd]\nskip-name-resolve=0\n":                  "lookup_on",
		"[server]\ndisable-skip-name-resolve\n":              "lookup_on",
		"[client]\nskip-name-resolve\n":                      "",
		"[mysqld]\n#skip-name-resolve\n":                     "",
		"[mariadb-10.11]\nskip-name-resolve\n":               "lookup_off",
		"[mariadb-client]\nskip-name-resolve\n":              "",
		"[mysqld]\nskip-name-resolve\nskip-name-resolve=off": "lookup_on",
	} {
		if got := mariadbNameResolveSetting(text); got != want {
			t.Errorf("%q: %q, want %q", text, got, want)
		}
	}
	for host, address := range map[string]bool{
		"localhost": true, "%": true, "203.0.113.7": true, "198.51.100.0/255.255.255.0": true, "192.168.%": true,
		"2001:db8::7": true, "app.example.com": false, "%.example.com": false, "db1": false, "cafe": false,
	} {
		if got := mariadbHostIsAddressForm(host); got != address {
			t.Errorf("%q: address form %v, want %v", host, got, address)
		}
	}
}

// No leftovers: with Fail2ban removed, CelikPanel's MariaDB jail and filter
// go; the rules wait.
func TestMariaDBJailFilesGoWhenFail2banIsRemoved(t *testing.T) {
	f := &fakeMariaDB{accounts: map[string]bool{"shop_u@localhost": true}}
	files := withFakeMariaDB(t, f)
	ts := dbremote.TargetState{Engine: "mysql", Port: 3306}
	mariadbRemoteInstance{}.apply(mariadbShopRule(), dbRemoteFacts{running: true, listenRemote: true}, &ts, "")
	if _, ok := files[dbremote.MariaDBJailPath]; !ok {
		t.Fatal("test setup: no jail written")
	}
	databaseRemoteFail2ban.installed = func() bool { return false }
	result := mariadbRemoteInstance{}.apply(mariadbShopRule(), dbRemoteFacts{running: true, listenRemote: true}, &ts, "")
	_, jail := files[dbremote.MariaDBJailPath]
	_, filter := files[dbremote.MariaDBFilterPath]
	if jail || filter || ts.Jail || len(result.Active) != 0 || !problemCodes(result)[transport.DatabaseRemoteNoSlowDown] {
		t.Fatalf("jail %v filter %v record %v active %v", jail, filter, ts.Jail, result.Active)
	}
}

// Item 2: the PostgreSQL extra layer is set up over a package default
// prefix, and everything it made goes with the last rule.
func TestPGFail2banLayerIsAddedAndRemoved(t *testing.T) {
	f := &fakeMariaDB{}
	files := withFakeMariaDB(t, f)
	var statements []string
	prefix := "%m [%p] %q%u@%d "
	restoreQuery, restoreSource, restoreList := databaseRemotePGQuery, databaseRemotePGLogSource, databaseRemoteListDir
	t.Cleanup(func() {
		databaseRemotePGQuery, databaseRemotePGLogSource, databaseRemoteListDir = restoreQuery, restoreSource, restoreList
	})
	databaseRemotePGQuery = func(_ int, sql string) (string, error) {
		statements = append(statements, sql)
		if strings.Contains(sql, "current_setting('log_line_prefix')") {
			return strings.Join([]string{prefix, "/etc/postgresql/17/main/postgresql.conf", "off", "off", "C.UTF-8"}, "\x1f") + "\n", nil
		}
		if strings.Contains(sql, "ALTER SYSTEM SET log_line_prefix") {
			prefix = dbremote.PGPrefixWithMarker(prefix)
		}
		return "", nil
	}
	databaseRemotePGLogSource = func(dbremote.Target) (dbremote.PGLogSource, error) {
		return dbremote.PGLogSource{File: "/var/log/postgresql/postgresql-17-main.log"}, nil
	}
	databaseRemoteListDir = func(string) ([]string, error) {
		var names []string
		for path := range files {
			if filepath.Dir(path) == "/etc/fail2ban/jail.d" {
				names = append(names, filepath.Base(path))
			}
		}
		return names, nil
	}
	databaseRemoteFail2ban.jailActive = func(name string) bool { return name == dbremote.PGJailName(5432) }
	target := dbremote.Target{Engine: "postgresql", Port: 5432}
	var ts dbremote.TargetState
	var result transport.DatabaseRemoteTargetResult
	applyPGJail(target, true, &ts, &result)
	if result.Fail2ban != "on" || !ts.LogPrefix || !ts.PGJail || prefix != "%m [%p] %q%u@%d celikpanel-client=%h " {
		t.Fatalf("layer: %s %q, record %+v, prefix %q", result.Fail2ban, result.Fail2banDetail, ts, prefix)
	}
	if !strings.Contains(string(files[dbremote.PGJailPath(5432)]), "logpath = /var/log/postgresql/postgresql-17-main.log\n") || string(files[dbremote.PGFilterPath]) != dbremote.PGFilter() {
		t.Fatalf("files: %v", files)
	}
	// The last rule goes.
	result = transport.DatabaseRemoteTargetResult{}
	applyPGJail(target, false, &ts, &result)
	_, jail := files[dbremote.PGJailPath(5432)]
	_, filter := files[dbremote.PGFilterPath]
	if jail || filter || ts.LogPrefix || ts.PGJail || result.Fail2ban != "off" || !strings.Contains(strings.Join(statements, "\n"), "ALTER SYSTEM RESET log_line_prefix") {
		t.Fatalf("leftovers: jail %v filter %v record %+v", jail, filter, ts)
	}
}

// The owner's own prefix is left alone and nothing holds the rules back.
func TestPGFail2banLayerLeavesAnOwnersPrefix(t *testing.T) {
	f := &fakeMariaDB{}
	files := withFakeMariaDB(t, f)
	restoreQuery := databaseRemotePGQuery
	t.Cleanup(func() { databaseRemotePGQuery = restoreQuery })
	altered := false
	databaseRemotePGQuery = func(_ int, sql string) (string, error) {
		if strings.Contains(sql, "current_setting('log_line_prefix')") {
			return strings.Join([]string{"%t [%p]: user=%u,db=%d ", "/etc/postgresql/17/main/postgresql.conf", "off", "off", "en_US.UTF-8"}, "\x1f"), nil
		}
		altered = altered || strings.Contains(sql, "ALTER SYSTEM")
		return "", nil
	}
	var ts dbremote.TargetState
	var result transport.DatabaseRemoteTargetResult
	applyPGJail(dbremote.Target{Engine: "postgresql", Port: 5432}, true, &ts, &result)
	if altered || ts.LogPrefix || result.Fail2ban != "unavailable" || !strings.Contains(result.Fail2banDetail, "log_line_prefix") || len(files) != 0 {
		t.Fatalf("owner's logging changed: altered %v, %+v, files %v", altered, result, files)
	}
}

// Item 4 (D5): narrowing takes an address out of the filter, keeps every
// port's drop rule and never adds anything.
func TestNarrowingOnlyTakesAddressesOut(t *testing.T) {
	host := fakeFilterHost(t)
	ports := []dbremote.NFTPort{
		{Port: 5432, V4: []netip.Prefix{netip.MustParsePrefix("203.0.113.7/32"), netip.MustParsePrefix("198.51.100.9/32")}},
		{Port: 3306, V4: []netip.Prefix{netip.MustParsePrefix("203.0.113.7/32")}},
	}
	text, err := dbremote.NFTFile(ports)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(databaseRemoteFilterFiles.nftPath, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	host.loaded = true
	if err := saveDatabaseRemoteState(dbremote.State{Targets: map[string]dbremote.TargetState{
		"postgresql:5432": {Engine: "postgresql", Port: 5432, Listen: true, Addresses: []string{"198.51.100.9/32", "203.0.113.7/32"}},
		"mysql:3306":      {Engine: "mysql", Port: 3306, Listen: true, Addresses: []string{"203.0.113.7/32"}},
	}}); err != nil {
		t.Fatal(err)
	}
	// Wanted: 5432 keeps 203.0.113.7 and asks for a new address, which must
	// not be added; 3306 has no rule any more.
	wanted := []dbremote.Target{{Engine: "postgresql", Port: 5432, Rules: []dbremote.Rule{
		{Database: "shop", User: "shop_u", Address: "203.0.113.7/32"},
		{Database: "shop", User: "shop_u", Address: "192.0.2.50/32"},
	}}}
	var resp transport.DatabaseRemoteResponse
	narrowDatabaseRemoteFilter(wanted, &resp)
	if resp.Error != "" {
		t.Fatalf("narrow: %s", resp.Error)
	}
	written, _ := os.ReadFile(databaseRemoteFilterFiles.nftPath)
	got := string(written)
	if strings.Contains(got, "198.51.100.9") || strings.Contains(got, "192.0.2.50") || !strings.Contains(got, "tcp dport 5432 ip saddr { 203.0.113.7 } accept") ||
		!strings.Contains(got, "tcp dport 3306 drop") || strings.Contains(got, "tcp dport 3306 ip saddr") || !strings.Contains(got, "tcp dport 5432 drop") {
		t.Fatalf("narrowed filter:\n%s", got)
	}
	for _, call := range host.calls {
		if strings.HasPrefix(call, "systemctl") {
			t.Fatalf("narrowing ran the service manager: %v", host.calls)
		}
	}
	if len(resp.Targets) != 2 || !resp.FilterLoaded {
		t.Fatalf("answer: %+v", resp)
	}
	state, _ := loadDatabaseRemoteState()
	if a := state.Targets["postgresql:5432"].Addresses; len(a) != 1 || a[0] != "203.0.113.7/32" || len(state.Targets["mysql:3306"].Addresses) != 0 {
		t.Fatalf("record: %+v", state.Targets)
	}
	// Nothing left to take out: the file is not rewritten.
	host.calls = nil
	resp = transport.DatabaseRemoteResponse{}
	narrowDatabaseRemoteFilter(wanted, &resp)
	for _, call := range host.calls {
		if strings.Contains(call, " -f ") {
			t.Fatalf("an unchanged filter was loaded again: %v", host.calls)
		}
	}
	if len(resp.Targets) != 0 {
		t.Fatalf("nothing closed, but %+v", resp.Targets)
	}
}

// Item 5 (D6): the panel firewall guard.
type fakeGuardRunner struct {
	tables     string
	unitState  map[string]string
	applied    []string
	restoreErr error
}

func (r *fakeGuardRunner) AcquireFirewallLock() (io.Closer, error) { return io.NopCloser(nil), nil }
func (r *fakeGuardRunner) LookPath(name string) (string, error)    { return "/usr/sbin/" + name, nil }
func (r *fakeGuardRunner) Output(name string, args ...string) ([]byte, error) {
	switch {
	case name == "nft" && len(args) == 2 && args[0] == "list" && args[1] == "tables":
		return []byte(r.tables), nil
	case name == "systemctl" && len(args) == 2 && args[0] == "is-enabled":
		state := r.unitState[args[1]]
		if state == "" {
			state = "enabled"
		}
		if state != "enabled" {
			return []byte(state + "\n"), errors.New("exit status 1")
		}
		return []byte(state + "\n"), nil
	}
	return nil, errors.New("unexpected " + name + " " + strings.Join(args, " "))
}
func (r *fakeGuardRunner) CombinedOutput(name string, args []string, stdin string) ([]byte, error) {
	if name == "nft" && len(args) == 2 && args[0] == "-f" && args[1] == "-" {
		r.applied = append(r.applied, stdin)
		return nil, r.restoreErr
	}
	return nil, nil
}
func (r *fakeGuardRunner) ConfiguredSSHPorts() ([]int, error)       { return []int{22}, nil }
func (r *fakeGuardRunner) ConfiguredSSHSocketPorts() ([]int, error) { return nil, nil }

type memoryFirewallStore struct{ data []byte }

func (s *memoryFirewallStore) Load() ([]byte, bool, error) { return s.data, s.data != nil, nil }
func (s *memoryFirewallStore) Save(b []byte) error         { s.data = b; return nil }
func (s *memoryFirewallStore) Remove() error               { s.data = nil; return nil }

func TestPanelFirewallGuardRestoresWithoutUnprovenDatabasePorts(t *testing.T) {
	host := fakeFilterHost(t) // the database filter: file present, chain missing, nft -f "fails" to prove it
	if err := saveDatabaseRemoteState(dbremote.State{Targets: map[string]dbremote.TargetState{
		"postgresql:5432": {Engine: "postgresql", Port: 5432, Listen: true, Addresses: []string{"203.0.113.7/32"}},
	}}); err != nil {
		t.Fatal(err)
	}
	// No filter file: the database filter has nothing to prove, but the
	// record still names 5432, so this checks the proven path first.
	snapshot := encodeFirewallSnapshot([]int{443, 5432}, nil, []int{22})
	store := &memoryFirewallStore{data: snapshot}
	runner := &fakeGuardRunner{tables: "table inet other\n"}
	if !panelFirewallFlushCheck(runner, store) || len(runner.applied) != 1 || !strings.Contains(runner.applied[0], "5432") {
		t.Fatalf("restore with the filter proven absent-by-design: %v", runner.applied)
	}
	// The filter file exists but its chain cannot be proven: 5432 stays out.
	if err := os.WriteFile(databaseRemoteFilterFiles.nftPath, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	restoreRun := databaseRemoteRun
	databaseRemoteRun = func(name string, args []string, stdin string) ([]byte, error) {
		if strings.HasSuffix(name, "nft") && len(args) == 2 && args[0] == "-f" {
			return []byte("Error: syntax"), errors.New("exit status 1") // cannot be loaded
		}
		return host.run(name, args, stdin)
	}
	t.Cleanup(func() { databaseRemoteRun = restoreRun })
	runner.applied = nil
	if !panelFirewallFlushCheck(runner, store) || len(runner.applied) != 1 {
		t.Fatalf("no restore: %v", runner.applied)
	}
	if strings.Contains(runner.applied[0], "5432") || !strings.Contains(runner.applied[0], "443") || !strings.Contains(runner.applied[0], "22") {
		t.Fatalf("restored ruleset:\n%s", runner.applied[0])
	}
	if n, _ := panelFirewallGuardReport(); n < 2 {
		t.Fatalf("restores counted: %d", n)
	}
}

func TestPanelFirewallGuardLeavesTheOwnersChoice(t *testing.T) {
	fakeFilterHost(t)
	store := &memoryFirewallStore{data: encodeFirewallSnapshot([]int{443}, nil, []int{22})}
	for _, c := range []struct {
		name  string
		units map[string]string
	}{
		{"boot restore disabled", map[string]string{firewallRestoreUnitName: "disabled"}},
		{"nftables masked", map[string]string{"nftables.service": "masked"}},
	} {
		runner := &fakeGuardRunner{tables: "", unitState: c.units}
		if panelFirewallFlushCheck(runner, store) || len(runner.applied) != 0 {
			t.Fatalf("%s: restored against the owner", c.name)
		}
		if _, note := panelFirewallGuardReport(); note == "" {
			t.Fatalf("%s: no note for the screen", c.name)
		}
	}
	// A present table, or no saved policy: nothing to do.
	runner := &fakeGuardRunner{tables: "table inet " + firewallpolicy.Table + "\n"}
	if panelFirewallFlushCheck(runner, store) || len(runner.applied) != 0 {
		t.Fatal("a present table was restored")
	}
	runner = &fakeGuardRunner{}
	if panelFirewallFlushCheck(runner, &memoryFirewallStore{}) || len(runner.applied) != 0 {
		t.Fatal("restored without a saved policy")
	}
}
