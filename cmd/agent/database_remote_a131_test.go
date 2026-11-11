package main

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/dbremote"
	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.131: CelikPanel's record of a PostgreSQL cluster follows the cluster
// when its port changes, and customers' addresses coming and going never ask
// for an engine restart while the administrator's server switch is on.

const bostonDataDir = "/var/lib/postgresql/18/main"

// bostonPG is the owner's Boston server after alpha.130's port change:
// cluster 18/main moved from 5433 to 5432 and carries CelikPanel's own
// listen_addresses = '*' and auth_delay in postgresql.auto.conf (loaded).
type bostonPG struct {
	port       int // where the cluster runs now
	listen     string
	listenFile string
	preload    string
	statements []string
	hbaLines   []string
	hbaWrites  int
}

func (b *bostonPG) query(port int, sql string) (string, error) {
	if port != b.port {
		return "", errors.New("connection refused")
	}
	switch {
	case strings.Contains(sql, "current_setting('data_directory')"):
		return bostonDataDir + "\n", nil
	case strings.Contains(sql, "current_setting('hba_file')"):
		pair := ""
		if b.listenFile != "" {
			pair = b.listenFile + "\x1e" + b.listen
		}
		return strings.Join([]string{b.listen, "on", "/etc/postgresql/18/main/pg_hba.conf", b.preload, "/nonexistent", "",
			pair, b.preload, "on"}, "\x1f") + "\n", nil
	case strings.Contains(sql, "FROM pg_authid"):
		return "shop_u\x1ffalse\x1ftrue\x1ftrue\ndb\x1fshop\n", nil
	case strings.Contains(sql, "ALTER SYSTEM"):
		b.statements = append(b.statements, sql)
	}
	return "", nil
}

func newBostonPG(t *testing.T) *bostonPG {
	t.Helper()
	b := &bostonPG{port: 5432, listen: "*", listenFile: bostonDataDir + "/postgresql.auto.conf", preload: "auth_delay"}
	withFakePG(t, &fakePGCluster{})
	databaseRemotePGQuery = b.query
	databaseRemoteWriteHBA = func(_ string, lines []string) error {
		b.hbaWrites++
		b.hbaLines = append([]string(nil), lines...)
		return nil
	}
	restorePorts, restoreOffered := databaseRemotePGClusterPorts, databaseRemoteServerOffered
	databaseRemotePGClusterPorts = func() (map[int]bool, bool) { return map[int]bool{b.port: true}, true }
	t.Cleanup(func() { databaseRemotePGClusterPorts, databaseRemoteServerOffered = restorePorts, restoreOffered })
	return b
}

// The record Boston carries (alpha.130, version 1) and its filter file.
func writeBostonRecord(t *testing.T) {
	t.Helper()
	record := `{"version":1,"targets":{"postgresql:5433":{"engine":"postgresql","port":5433,"listen":true,"auth_delay":true}},"applied":"f97b0000"}` + "\n"
	if err := os.WriteFile(databaseRemoteStatePath, []byte(record), 0o600); err != nil {
		t.Fatal(err)
	}
	text, err := dbremote.NFTFile([]dbremote.NFTPort{{Port: 5433}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(databaseRemoteFilterFiles.nftPath, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func bostonRule() dbremote.Target {
	return dbremote.Target{Engine: dbremote.EnginePostgreSQL, Port: 5432, Rules: []dbremote.Rule{{Database: "shop", User: "shop_u", Address: "176.88.93.168/32"}}}
}

func applyBoston(t *testing.T, offered bool, targets ...dbremote.Target) transport.DatabaseRemoteResponse {
	t.Helper()
	databaseRemoteServerOffered = offered
	var resp transport.DatabaseRemoteResponse
	applyDatabaseRemote(targets, "boston.example.test", func(string) error { return nil }, &resp)
	return resp
}

func filterText(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(databaseRemoteFilterFiles.nftPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	return string(data)
}

// Boston's exact state heals by itself after the update: the record moves to
// 5432, the filter lets the listed address reach 5432 and has no 5433 rule,
// the rule is in force, and no setting changes (no restart needed).
func TestDatabaseRemoteBostonPortChangeHealsByItself(t *testing.T) {
	host := fakeFilterHost(t)
	host.loaded = true
	b := newBostonPG(t)
	writeBostonRecord(t)

	// The status (read only) already sees CelikPanel's own settings.
	var status transport.DatabaseRemoteResponse
	if err := (&Agent{}).DatabaseRemoteStatusV1(&transport.DatabaseRemoteStatusRequest{Targets: []transport.DatabaseRemoteTarget{{Engine: "postgresql", Port: 5432}}}, &status); err != nil {
		t.Fatal(err)
	}
	if len(status.Targets) != 1 || status.Targets[0].State == transport.DatabaseRemoteStateOwnerManaged || len(status.Targets[0].Active) != 0 || status.AppliedDigest != "" {
		t.Fatalf("status before the apply: %+v", status)
	}
	if raw, _ := os.ReadFile(databaseRemoteStatePath); !strings.Contains(string(raw), `"version":1`) {
		t.Fatal("a status read rewrote the record")
	}

	resp := applyBoston(t, true, bostonRule())
	if resp.Error != "" || len(resp.Targets) != 1 {
		t.Fatalf("apply: %s %+v", resp.Error, resp.Targets)
	}
	result := resp.Targets[0]
	if result.Port != 5432 || result.State != transport.DatabaseRemoteStateOpen || len(result.Active) != 1 || problemCodes(result)[transport.DatabaseRemoteOwnerListen] ||
		problemCodes(result)[transport.DatabaseRemoteNoSlowDown] || problemCodes(result)[transport.DatabaseRemoteRestartNeeded] {
		t.Fatalf("result: %+v", result)
	}
	if len(b.statements) != 0 {
		t.Fatalf("settings were changed (a restart would be needed): %v", b.statements)
	}
	if len(b.hbaLines) != 1 || !strings.Contains(b.hbaLines[0], "176.88.93.168/32") {
		t.Fatalf("pg_hba block: %v", b.hbaLines)
	}
	filter := filterText(t)
	if !strings.Contains(filter, "tcp dport 5432 ip saddr { 176.88.93.168 } accept") || !strings.Contains(filter, "tcp dport 5432 drop") || strings.Contains(filter, "5433") {
		t.Fatalf("filter:\n%s", filter)
	}
	if len(resp.FilterPorts) != 1 || resp.FilterPorts[0] != 5432 || !resp.FilterLoaded {
		t.Fatalf("filter ports %v loaded %v", resp.FilterPorts, resp.FilterLoaded)
	}
	raw, _ := os.ReadFile(databaseRemoteStatePath)
	state, err := dbremote.DecodeState(raw)
	if err != nil || !strings.Contains(string(raw), `"version": 2`) {
		t.Fatalf("record: %v\n%s", err, raw)
	}
	moved, ok := state.Targets["postgresql:5432"]
	if _, stale := state.Targets["postgresql:5433"]; stale || !ok || !moved.Listen || !moved.AuthDelay || moved.DataDir != bostonDataDir || len(moved.Active) != 1 {
		t.Fatalf("record targets: %+v", state.Targets)
	}
	if state.Applied == "" || state.Applied != dbremote.RequestDigestOffered([]dbremote.Target{bostonRule()}, false, true) {
		t.Fatalf("applied digest %q", state.Applied)
	}
}

// Owner decision (8 Nov): the last address leaving keeps the listen address
// while the server switch is on (no restart for the next customer); the
// filter still drops the port. Only the switch turned off takes it back.
func TestDatabaseRemoteLastRuleKeepsTheEngineReadyWhileOffered(t *testing.T) {
	host := fakeFilterHost(t)
	host.loaded = true
	b := newBostonPG(t)
	writeBostonRecord(t)
	if resp := applyBoston(t, true, bostonRule()); resp.Error != "" {
		t.Fatal(resp.Error)
	}

	// The customer removes the address (or it ends).
	resp := applyBoston(t, true)
	if resp.Error != "" || len(resp.Targets) != 1 || resp.Targets[0].State != transport.DatabaseRemoteStateOff || problemCodes(resp.Targets[0])[transport.DatabaseRemoteRestartNeeded] {
		t.Fatalf("removal: %s %+v", resp.Error, resp.Targets)
	}
	for _, s := range b.statements {
		if strings.Contains(s, "RESET listen_addresses") {
			t.Fatalf("the listen address was taken back while the switch is on: %v", b.statements)
		}
	}
	if len(b.hbaLines) != 0 {
		t.Fatalf("pg_hba block kept a line: %v", b.hbaLines)
	}
	if filter := filterText(t); !strings.Contains(filter, "tcp dport 5432 drop") || strings.Contains(filter, "176.88.93.168") {
		t.Fatalf("filter after the last rule:\n%s", filter)
	}
	state, _ := loadDatabaseRemoteState()
	if ts := state.Targets["postgresql:5432"]; !ts.Listen || ts.ListenPending || len(ts.Active) != 0 {
		t.Fatalf("record after the last rule: %+v", ts)
	}

	// The next customer's address works at once, without a setting change.
	b.statements = nil
	resp = applyBoston(t, true, bostonRule())
	if resp.Error != "" || resp.Targets[0].State != transport.DatabaseRemoteStateOpen || len(resp.Targets[0].Active) != 1 || len(b.statements) != 0 {
		t.Fatalf("re-add: %s %+v statements %v", resp.Error, resp.Targets, b.statements)
	}

	// The administrator turns the switch off: the listen address goes back
	// and the screen says a restart stops the listening.
	resp = applyBoston(t, false)
	if resp.Error != "" || !strings.Contains(strings.Join(b.statements, "\n"), "ALTER SYSTEM RESET listen_addresses") ||
		resp.Targets[0].State != transport.DatabaseRemoteStateClosing || !problemCodes(resp.Targets[0])[transport.DatabaseRemoteRestartNeeded] {
		t.Fatalf("switch off: %s %+v statements %v", resp.Error, resp.Targets, b.statements)
	}
	if filter := filterText(t); !strings.Contains(filter, "tcp dport 5432 drop") || strings.Contains(filter, "176.88.93.168") {
		t.Fatalf("filter after the switch went off:\n%s", filter)
	}
}

// The port change itself moves the record and the cluster's filter port
// (the panel's apply follows); the old port keeps nothing.
func TestDatabaseRemoteFollowsAProvedPortChange(t *testing.T) {
	host := fakeFilterHost(t)
	host.loaded = true
	newBostonPG(t)
	files := map[string][]byte{}
	restoreFiles := databaseRemoteFiles
	t.Cleanup(func() { databaseRemoteFiles = restoreFiles })
	databaseRemoteFiles.read = func(p string) ([]byte, error) {
		if data, ok := files[p]; ok {
			return data, nil
		}
		return nil, os.ErrNotExist
	}
	databaseRemoteFiles.remove = func(p string) error { delete(files, p); return nil }
	jail, err := dbremote.PGJail(5433, dbremote.PGLogSource{File: "/var/log/postgresql/postgresql-18-main.log"})
	if err != nil {
		t.Fatal(err)
	}
	files[dbremote.PGJailPath(5433)] = []byte(jail)
	if err := saveDatabaseRemoteState(dbremote.State{Targets: map[string]dbremote.TargetState{
		"postgresql:5433": {Engine: "postgresql", Port: 5433, Listen: true, AuthDelay: true, PGJail: true, DataDir: bostonDataDir,
			Addresses: []string{"176.88.93.168/32"}, Active: bostonRule().Rules},
		"mysql:3306": {Engine: "mysql", Port: 3306, Listen: true},
	}, Applied: "abc"}); err != nil {
		t.Fatal(err)
	}
	text, _ := dbremote.NFTFile([]dbremote.NFTPort{{Port: 5433, V4: listedPrefixes([]string{"176.88.93.168/32"})}, {Port: 3306}})
	if err := os.WriteFile(databaseRemoteFilterFiles.nftPath, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}

	databaseRemoteFollowPGPort(5433, 5432)

	filter := filterText(t)
	if !strings.Contains(filter, "tcp dport 5432 ip saddr { 176.88.93.168 } accept") || strings.Contains(filter, "5433") || !strings.Contains(filter, "tcp dport 3306 drop") {
		t.Fatalf("filter:\n%s", filter)
	}
	state, err := loadDatabaseRemoteState()
	if err != nil {
		t.Fatal(err)
	}
	ts, ok := state.Targets["postgresql:5432"]
	if _, stale := state.Targets["postgresql:5433"]; stale || !ok || !ts.Listen || ts.PGJail || state.Applied != "" || len(ts.Addresses) != 1 {
		t.Fatalf("record: %+v applied %q", state.Targets, state.Applied)
	}
	if _, left := files[dbremote.PGJailPath(5433)]; left {
		t.Fatal("the old port's Fail2ban jail was left behind")
	}
	// Nothing recorded on the old port: a second call changes nothing.
	databaseRemoteFollowPGPort(5433, 5432)
	if again, _ := loadDatabaseRemoteState(); len(again.Targets) != 2 {
		t.Fatalf("second call: %+v", again.Targets)
	}
}

// A record moves only to a cluster CelikPanel can tell is the same; anything
// ambiguous stays as it was (closed).
func TestDatabaseRemoteHealMovesOnlyAProvenCluster(t *testing.T) {
	restoreIdentity, restorePorts, restoreQuery := databaseRemotePGIdentity, databaseRemotePGClusterPorts, databaseRemotePGQuery
	t.Cleanup(func() {
		databaseRemotePGIdentity, databaseRemotePGClusterPorts, databaseRemotePGQuery = restoreIdentity, restorePorts, restoreQuery
	})
	dirs := map[int]string{}
	databaseRemotePGIdentity = func(port int) (string, bool) {
		dir, ok := dirs[port]
		return dir, ok
	}
	listed := map[int]bool{}
	databaseRemotePGClusterPorts = func() (map[int]bool, bool) { return listed, true }
	own := map[int]bool{} // ports whose postgresql.auto.conf carries CelikPanel's settings
	databaseRemotePGQuery = func(port int, sql string) (string, error) {
		pair, preload := "/etc/postgresql/18/main/postgresql.conf\x1e*", "shared_x"
		if own[port] {
			pair, preload = "/var/lib/postgresql/18/main/postgresql.auto.conf\x1e*", "auth_delay"
		}
		return strings.Join([]string{"*", "on", "/x/pg_hba.conf", preload, "", "", pair, preload, "on"}, "\x1f"), nil
	}
	legacy := func() dbremote.State {
		return dbremote.State{Targets: map[string]dbremote.TargetState{
			"postgresql:5433": {Engine: "postgresql", Port: 5433, Listen: true, AuthDelay: true},
		}}
	}
	named := []dbremote.Target{{Engine: "postgresql", Port: 5432}, {Engine: "postgresql", Port: 5434}}
	reset := func() {
		dirs, listed, own = map[int]string{}, map[int]bool{}, map[int]bool{}
	}

	// Two clusters carry CelikPanel's settings: which one moved is unknown.
	reset()
	dirs[5432], dirs[5434] = "/a", "/b"
	own[5432], own[5434] = true, true
	state := legacy()
	if moves := healDatabaseRemotePGPorts(&state, named, true); len(moves) != 0 || state.Targets["postgresql:5433"].Port != 5433 {
		t.Fatalf("ambiguous: %v %+v", moves, state.Targets)
	}
	// Only the owner's own settings elsewhere: nothing moves.
	reset()
	dirs[5432] = "/a"
	state = legacy()
	if moves := healDatabaseRemotePGPorts(&state, named, true); len(moves) != 0 {
		t.Fatalf("owner's settings taken for CelikPanel's: %v", moves)
	}
	// The old port's cluster is only stopped (pg_lsclusters still lists it).
	reset()
	dirs[5432] = "/a"
	own[5432] = true
	listed[5433] = true
	state = legacy()
	if moves := healDatabaseRemotePGPorts(&state, named, true); len(moves) != 0 {
		t.Fatalf("a stopped cluster's record moved: %v", moves)
	}
	// The port taken by another record: never merged.
	reset()
	dirs[5432] = "/a"
	own[5432] = true
	state = legacy()
	state.Targets["postgresql:5432"] = dbremote.TargetState{Engine: "postgresql", Port: 5432, TLS: true}
	if moves := healDatabaseRemotePGPorts(&state, named, true); len(moves) != 0 {
		t.Fatalf("merged into another record: %v", moves)
	}
	// One proven candidate: moved, with its data directory.
	reset()
	dirs[5432] = "/a"
	own[5432] = true
	state = legacy()
	if moves := healDatabaseRemotePGPorts(&state, named, true); len(moves) != 1 || state.Targets["postgresql:5432"].DataDir != "/a" {
		t.Fatalf("not moved: %v %+v", moves, state.Targets)
	}
	// A record with a data directory follows it, even when another cluster
	// now answers on its old port.
	reset()
	dirs[5433], dirs[5434] = "/new-cluster", "/a"
	state = dbremote.State{Targets: map[string]dbremote.TargetState{
		"postgresql:5433": {Engine: "postgresql", Port: 5433, Listen: true, DataDir: "/a"},
	}}
	if moves := healDatabaseRemotePGPorts(&state, named, true); len(moves) != 1 || moves[0].to != 5434 {
		t.Fatalf("data directory: %v %+v", moves, state.Targets)
	}
	// A cluster that answers on its port only gets its data directory noted.
	reset()
	dirs[5433] = "/c"
	state = legacy()
	if moves := healDatabaseRemotePGPorts(&state, named, true); len(moves) != 0 || state.Targets["postgresql:5433"].DataDir != "/c" {
		t.Fatalf("in place: %v %+v", moves, state.Targets)
	}
}

// MariaDB: the option file stays while the server switch is on and goes
// when it is turned off.
func TestMariaDBOptionFileStaysWhileOffered(t *testing.T) {
	fakeFilterHost(t)
	f := &fakeMariaDB{accounts: map[string]bool{"shop_u@localhost": true}}
	files := withFakeMariaDB(t, f)
	databaseRemoteFail2ban.jailActive = func(name string) bool { return name == dbremote.MariaDBJailName }
	restoreOffered := databaseRemoteServerOffered
	t.Cleanup(func() { databaseRemoteServerOffered = restoreOffered })
	path, err := mariadbOptionPath()
	if err != nil {
		t.Fatal(err)
	}
	target := dbremote.Target{Engine: "mysql", Port: 3306, Rules: []dbremote.Rule{{Database: "shop", User: "shop_u", Address: "203.0.113.7/32"}}}
	databaseRemoteServerOffered = true
	var resp transport.DatabaseRemoteResponse
	applyDatabaseRemote([]dbremote.Target{target}, "", nil, &resp)
	if _, ok := files[path]; !ok || resp.Error != "" {
		t.Fatalf("first rule: no option file (%s)", resp.Error)
	}
	resp = transport.DatabaseRemoteResponse{}
	applyDatabaseRemote(nil, "", nil, &resp)
	state, _ := loadDatabaseRemoteState()
	if _, ok := files[path]; !ok || !state.Targets["mysql:3306"].Listen || len(state.Targets["mysql:3306"].Accounts) != 0 {
		t.Fatalf("last rule while offered: file kept %v, record %+v", files[path] != nil, state.Targets)
	}
	databaseRemoteServerOffered = false
	resp = transport.DatabaseRemoteResponse{}
	applyDatabaseRemote(nil, "", nil, &resp)
	if _, ok := files[path]; ok {
		t.Fatal("the switch turned off left CelikPanel's option file")
	}
}

// Boston after the owner-authorised repair (8 Nov 18:49Z): the record is
// already keyed by the cluster's port. The update changes nothing on the
// engine (no setting, no restart); it only notes the data directory.
func TestDatabaseRemoteCorrectRecordIsLeftAsItIs(t *testing.T) {
	host := fakeFilterHost(t)
	host.loaded = true
	b := newBostonPG(t)
	record := `{"version":1,"targets":{"postgresql:5432":{"engine":"postgresql","port":5432,"addresses":["176.88.93.168/32"],"listen":true,"auth_delay":true,` +
		`"active":[{"Database":"shop","User":"shop_u","Address":"176.88.93.168/32"}]}},"applied":"f97b0000"}` + "\n"
	if err := os.WriteFile(databaseRemoteStatePath, []byte(record), 0o600); err != nil {
		t.Fatal(err)
	}
	text, _ := dbremote.NFTFile([]dbremote.NFTPort{{Port: 5432, V4: listedPrefixes([]string{"176.88.93.168/32"})}})
	if err := os.WriteFile(databaseRemoteFilterFiles.nftPath, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	resp := applyBoston(t, true, bostonRule())
	if resp.Error != "" || resp.Targets[0].State != transport.DatabaseRemoteStateOpen || len(b.statements) != 0 {
		t.Fatalf("apply: %s %+v statements %v", resp.Error, resp.Targets, b.statements)
	}
	if filterText(t) != text {
		t.Fatalf("filter changed:\n%s", filterText(t))
	}
	state, _ := loadDatabaseRemoteState()
	if ts := state.Targets["postgresql:5432"]; len(state.Targets) != 1 || !ts.Listen || !ts.AuthDelay || ts.DataDir != bostonDataDir || len(ts.Active) != 1 {
		t.Fatalf("record: %+v", state.Targets)
	}
}
