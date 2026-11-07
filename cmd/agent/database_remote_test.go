package main

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/dbremote"
	"github.com/alicelik/celikpanel/internal/transport"
)

func TestPGRemoteSettingsParseAndListenRules(t *testing.T) {
	out := strings.Join([]string{"localhost", "on", "/etc/postgresql/17/main/pg_hba.conf", "", "/usr/lib/postgresql/17/lib",
		"listen_addresses", "/var/lib/postgresql/17/main/postgresql.auto.conf\x1e*", "<none>", "on"}, "\x1f") + "\n"
	s, err := parsePGRemoteSettings(out)
	if err != nil {
		t.Fatal(err)
	}
	if s.listen != "localhost" || !s.ssl || s.hbaFile != "/etc/postgresql/17/main/pg_hba.conf" || s.listenFileValue != "*" ||
		!strings.HasSuffix(s.listenFile, "postgresql.auto.conf") || len(s.pendingRestart) != 1 || s.preloadFile != "<none>" || !s.alterSystem {
		t.Fatalf("settings = %+v", s)
	}
	if _, err := parsePGRemoteSettings("a\x1fb"); err == nil {
		t.Fatal("a short answer was read")
	}
	for value, local := range map[string]bool{"localhost": true, "'localhost'": true, "127.0.0.1,::1": true, "*": false, "0.0.0.0": false, "": false, "localhost,10.0.0.5": false} {
		if pgListenIsLocal(value) != local {
			t.Errorf("pgListenIsLocal(%q) = %v", value, !local)
		}
	}
	for value, local := range map[string]bool{"127.0.0.1": true, "localhost": true, "*": false, "0.0.0.0": false, "": false, "::": false} {
		if mariadbBindIsLocal(value) != local {
			t.Errorf("mariadbBindIsLocal(%q) = %v", value, !local)
		}
	}
}

func TestPGRemoteInspectLeavesOwnerListenAlone(t *testing.T) {
	restore := databaseRemotePGQuery
	defer func() { databaseRemotePGQuery = restore }()
	answer := func(running, file, value string) {
		databaseRemotePGQuery = func(int, string) (string, error) {
			pair := ""
			if file != "" {
				pair = file + "\x1e" + value
			}
			return strings.Join([]string{running, "on", "/etc/postgresql/17/main/pg_hba.conf", "", "", "", pair, "<none>", "on"}, "\x1f"), nil
		}
	}
	target := dbremote.Target{Engine: dbremote.EnginePostgreSQL, Port: 5432}
	answer("localhost", "", "")
	if f := (pgRemoteInstance{}).inspect(target, dbremote.TargetState{}); !f.running || f.ownerManaged || f.listenRemote {
		t.Fatalf("default cluster = %+v", f)
	}
	answer("*", "/etc/postgresql/17/main/postgresql.conf", "*")
	if f := (pgRemoteInstance{}).inspect(target, dbremote.TargetState{}); !f.ownerManaged {
		t.Fatal("an owner's listen_addresses was not recognised")
	}
	// CelikPanel's own setting is not the owner's.
	answer("*", "/var/lib/postgresql/17/main/postgresql.auto.conf", "*")
	if f := (pgRemoteInstance{}).inspect(target, dbremote.TargetState{Listen: true}); f.ownerManaged {
		t.Fatal("CelikPanel's own listen address taken for the owner's")
	}
	// Going back to local, before the restart.
	answer("*", "", "")
	if f := (pgRemoteInstance{}).inspect(target, dbremote.TargetState{ListenPending: true}); f.ownerManaged || !f.listenRemote {
		t.Fatalf("closing cluster = %+v", f)
	}
	databaseRemotePGQuery = func(int, string) (string, error) { return "", errors.New("connection refused") }
	if f := (pgRemoteInstance{}).inspect(target, dbremote.TargetState{}); f.running || f.readError == "" {
		t.Fatalf("down cluster = %+v", f)
	}
}

func TestDBRemoteFilterPorts(t *testing.T) {
	pg := dbremote.Target{Engine: dbremote.EnginePostgreSQL, Port: 5432, Rules: []dbremote.Rule{{Database: "shop", User: "shop_u", Address: "203.0.113.7/32"}, {Database: "shop", User: "shop_u", Address: "2001:db8::7/128"}}}
	owner := dbremote.Target{Engine: dbremote.EnginePostgreSQL, Port: 5433, Rules: []dbremote.Rule{{Database: "a", User: "b", Address: "198.51.100.1/32"}}}
	closing := dbremote.Target{Engine: dbremote.EngineMySQL, Port: 3306}
	idle := dbremote.Target{Engine: dbremote.EnginePostgreSQL, Port: 5434}
	targets := []dbremote.Target{pg, owner, closing, idle}
	facts := []dbRemoteFacts{{running: true}, {running: true, ownerManaged: true, listenRemote: true}, {running: true, listenRemote: true}, {running: true}}
	state := dbremote.State{Targets: map[string]dbremote.TargetState{
		"mysql:3306":      {Engine: "mysql", Port: 3306, ListenPending: true},
		"postgresql:5432": {Engine: "postgresql", Port: 5432, Addresses: []string{"203.0.113.7/32"}},
	}}
	before := dbRemoteFilterPorts(targets, facts, state, false)
	if len(before) != 2 || before[0].Port != 5432 || len(before[0].V4) != 1 || len(before[0].V6) != 1 || before[1].Port != 3306 || len(before[1].V4)+len(before[1].V6) != 0 {
		t.Fatalf("before = %+v", before)
	}
	after := dbRemoteFilterPorts(targets, facts, state, true)
	if len(after) != 2 || len(after[0].V4) != 1 || len(after[0].V6) != 0 {
		t.Fatalf("after: a refused rule's address stayed in the filter: %+v", after)
	}
}

// fakeMariaDB answers the statements the MariaDB side sends.
type fakeMariaDB struct {
	accounts map[string]bool // user@host
	grants   map[string][]string
	executed []string
	limit    string // the local account's max_user_connections
	locked   string // "1" when the local account is locked
	haveSSL  string // @@have_ssl, "YES" by default
	noJail   bool   // Fail2ban does not run the jail
	logError string // @@log_error, "" (the journal) by default
	// copies as the fake engine holds them: limit and lock, and grants
	copyAuth   map[string]string
	copyGrants map[string][]string
}

// fakeCopyHost answers the host of "... Host = '<host>'" or "'user'@'<host>'".
func fakeCopyHost(statement string) string {
	if i := strings.Index(statement, "Host = '"); i >= 0 {
		rest := statement[i+len("Host = '"):]
		return rest[:strings.Index(rest, "'")]
	}
	if i := strings.Index(statement, "'@'"); i >= 0 {
		rest := statement[i+3:]
		return rest[:strings.Index(rest, "'")]
	}
	return ""
}

// record keeps what the statements of one batch did to the copies.
func (f *fakeMariaDB) record(batch string) {
	for _, line := range strings.Split(batch, "\n") {
		host := fakeCopyHost(line)
		id := "shop_u@" + host
		switch {
		case strings.HasPrefix(line, "CREATE USER 'shop_u'"), strings.HasPrefix(line, "ALTER USER 'shop_u'"):
			if f.copyAuth == nil {
				f.copyAuth = map[string]string{}
			}
			f.copyAuth[id] = orDefault(f.limit, "0") + "\t" + orDefault(f.locked, "0")
		case strings.HasPrefix(line, "REVOKE ALL PRIVILEGES, GRANT OPTION FROM 'shop_u'"):
			delete(f.copyGrants, id)
		case strings.HasPrefix(line, "GRANT ") && strings.Contains(line, " TO 'shop_u'@'"):
			if f.copyGrants == nil {
				f.copyGrants = map[string][]string{}
			}
			db := line[strings.Index(line, " ON ")+4 : strings.Index(line, " TO ")]
			privileges := strings.TrimPrefix(line[:strings.Index(line, " ON ")], "GRANT ")
			f.copyGrants[id] = append(f.copyGrants[id], "GRANT "+privileges+" ON "+db+" TO `shop_u`@`"+host+"`")
		}
	}
}

func (f *fakeMariaDB) query(statement string) (string, error) {
	f.executed = append(f.executed, statement)
	f.record(statement)
	switch {
	case strings.HasPrefix(statement, "SELECT @@bind_address"):
		return "127.0.0.1\t0\t" + orDefault(f.haveSSL, "YES") + "\t3306\t" + f.logError + "\t2\t/var/lib/mysql/\n", nil
	case strings.HasPrefix(statement, "SELECT plugin") && strings.Contains(statement, "User = 'shop_u'") && fakeCopyHost(statement) == "localhost":
		return "mysql_native_password\t*0123456789ABCDEF0123456789ABCDEF01234567\t" + orDefault(f.limit, "0") + "\t" + orDefault(f.locked, "0") + "\t\n", nil
	case strings.HasPrefix(statement, "SELECT plugin") && strings.Contains(statement, "User = 'shop_u'"):
		state, ok := f.copyAuth["shop_u@"+fakeCopyHost(statement)]
		if !ok {
			return "", nil
		}
		return "mysql_native_password\t*0123456789ABCDEF0123456789ABCDEF01234567\t" + state + "\tANY\n", nil
	case strings.HasPrefix(statement, "SELECT plugin") && strings.Contains(statement, "User = 'boss'"):
		return "mysql_native_password\t*0123456789ABCDEF0123456789ABCDEF01234567\t0\t0\t\n", nil
	case strings.HasPrefix(statement, "SELECT plugin"):
		return "", nil
	case strings.HasPrefix(statement, "SHOW GRANTS FOR 'shop_u'@'localhost'"):
		return "GRANT USAGE ON *.* TO `shop_u`@`localhost` IDENTIFIED BY PASSWORD '*0123456789ABCDEF0123456789ABCDEF01234567'\nGRANT ALL PRIVILEGES ON `shop`.* TO `shop_u`@`localhost`\n", nil
	case strings.HasPrefix(statement, "SHOW GRANTS FOR 'shop_u'@"):
		host := fakeCopyHost(statement)
		return "GRANT USAGE ON *.* TO `shop_u`@`" + host + "`\n" + strings.Join(f.copyGrants["shop_u@"+host], "\n") + "\n", nil
	case strings.HasPrefix(statement, "SHOW GRANTS FOR 'boss'"):
		return "GRANT ALL PRIVILEGES ON *.* TO `boss`@`localhost`\n", nil
	case strings.HasPrefix(statement, "SELECT Host FROM mysql.user WHERE User = '"):
		user := strings.TrimSuffix(strings.TrimPrefix(statement, "SELECT Host FROM mysql.user WHERE User = '"), "';")
		var hosts []string
		for id := range f.accounts {
			if u, h := splitAccountID(id); u == user {
				hosts = append(hosts, h)
			}
		}
		return strings.Join(hosts, "\n"), nil
	case strings.HasPrefix(statement, "CREATE USER"):
		name := strings.Fields(statement)[2]
		f.accounts[strings.ReplaceAll(name, "'", "")] = true
		return "", nil
	case strings.HasPrefix(statement, "DROP USER IF EXISTS "):
		name := strings.TrimSuffix(strings.TrimPrefix(statement, "DROP USER IF EXISTS "), ";")
		delete(f.accounts, strings.ReplaceAll(name, "'", ""))
		return "", nil
	}
	return "", nil
}

func withFakeMariaDB(t *testing.T, f *fakeMariaDB) map[string][]byte {
	t.Helper()
	files := map[string][]byte{}
	restoreQuery, restoreFiles, restoreValidate, restoreState := databaseRemoteMySQLQuery, databaseRemoteFiles, databaseRemoteValidateMariaDBOption, databaseRemoteStatePath
	databaseRemoteMySQLQuery = f.query
	databaseRemoteFiles.read = func(p string) ([]byte, error) {
		if data, ok := files[p]; ok {
			return data, nil
		}
		return nil, os.ErrNotExist
	}
	databaseRemoteFiles.write = func(p string, c []byte) error { files[p] = append([]byte(nil), c...); return nil }
	databaseRemoteFiles.remove = func(p string) error { delete(files, p); return nil }
	databaseRemoteFiles.isDir = func(p string) bool {
		return p == "/etc/mysql/mariadb.conf.d" || p == "/etc/fail2ban/jail.d" || p == "/etc/fail2ban/filter.d"
	}
	databaseRemoteValidateMariaDBOption = func(string, []byte) error { return nil }
	databaseRemoteStatePath = t.TempDir() + "/database-remote.json"
	restoreFail2ban := databaseRemoteFail2ban
	databaseRemoteFail2ban.active = func() bool { return true }
	databaseRemoteFail2ban.jailActive = func(name string) bool {
		_, written := files[dbremote.MariaDBJailPath]
		return !f.noJail && written && name == dbremote.MariaDBJailName
	}
	databaseRemoteFail2ban.reload = func() error { return nil }
	t.Cleanup(func() {
		databaseRemoteMySQLQuery, databaseRemoteFiles, databaseRemoteValidateMariaDBOption, databaseRemoteStatePath = restoreQuery, restoreFiles, restoreValidate, restoreState
		databaseRemoteFail2ban = restoreFail2ban
	})
	return files
}

func TestMariaDBRemoteApplyCreatesTLSAccountsAndRefusesOthers(t *testing.T) {
	f := &fakeMariaDB{accounts: map[string]bool{"shop_u@localhost": true, "shop_u@198.51.100.9": true}, limit: "15"}
	files := withFakeMariaDB(t, f)
	target, err := dbremote.Target{Engine: "mysql", Port: 3306, Rules: []dbremote.Rule{
		{Database: "shop", User: "shop_u", Address: "203.0.113.7"},
		{Database: "blog", User: "shop_u", Address: "203.0.113.7"},  // no local grant
		{Database: "shop", User: "shop_u", Address: "198.51.100.9"}, // owner's account
		{Database: "shop", User: "boss", Address: "203.0.113.7"},    // server-wide rights
		{Database: "shop", User: "ghost", Address: "203.0.113.7"},   // no such user
	}}.Normalize()
	if err != nil {
		t.Fatal(err)
	}
	var ts dbremote.TargetState
	result := mariadbRemoteInstance{}.apply(target, dbRemoteFacts{running: true}, &ts, "db.example.com")
	codes := map[string]bool{}
	for _, p := range result.Problems {
		codes[p.Code] = true
	}
	for _, want := range []string{transport.DatabaseRemoteNoAccess, transport.DatabaseRemoteAccountConflict, transport.DatabaseRemotePrivilegedUser, transport.DatabaseRemoteUserMissing, transport.DatabaseRemoteRestartNeeded} {
		if !codes[want] {
			t.Errorf("problem %s missing: %+v", want, result.Problems)
		}
	}
	if len(result.Active) != 1 || result.Active[0].Address != "203.0.113.7/32" || result.Active[0].Database != "shop" {
		t.Fatalf("active = %+v", result.Active)
	}
	if len(ts.Accounts) != 1 || ts.Accounts[0] != "shop_u@203.0.113.7" {
		t.Fatalf("recorded accounts = %v", ts.Accounts)
	}
	joined := strings.Join(f.executed, "\n")
	if !strings.Contains(joined, "CREATE USER 'shop_u'@'203.0.113.7' IDENTIFIED VIA mysql_native_password USING '*0123456789ABCDEF0123456789ABCDEF01234567' REQUIRE SSL WITH MAX_USER_CONNECTIONS 15 ACCOUNT UNLOCK;") ||
		!strings.Contains(joined, "GRANT ALL PRIVILEGES ON `shop`.* TO 'shop_u'@'203.0.113.7';") {
		t.Fatalf("statements:\n%s", joined)
	}
	if strings.Contains(joined, "'%'") || strings.Contains(joined, "`blog`") || strings.Contains(joined, "'boss'@'203") || strings.Contains(joined, "198.51.100.9' IDENTIFIED") {
		t.Fatalf("a refused rule reached MariaDB:\n%s", joined)
	}
	if string(files[dbremote.MariaDBOptionFileDebian]) != dbremote.MariaDBOptionFile("", "") || !ts.Listen {
		t.Fatalf("option file = %q, listen %v", files[dbremote.MariaDBOptionFileDebian], ts.Listen)
	}
	if result.State != transport.DatabaseRemoteStateWaitRestart {
		t.Fatalf("state = %s", result.State)
	}

	// The last rule goes: CelikPanel's account and option file go, the
	// owner's account stays.
	empty := dbremote.Target{Engine: "mysql", Port: 3306}
	result = mariadbRemoteInstance{}.apply(empty, dbRemoteFacts{running: true, listenRemote: true}, &ts, "db.example.com")
	if f.accounts["shop_u@203.0.113.7"] || !f.accounts["shop_u@198.51.100.9"] || len(ts.Accounts) != 0 {
		t.Fatalf("accounts after closing = %v, recorded %v", f.accounts, ts.Accounts)
	}
	if _, ok := files[dbremote.MariaDBOptionFileDebian]; ok || ts.Listen || !ts.ListenPending {
		t.Fatalf("option file left or record wrong: %+v", ts)
	}
	if result.State != transport.DatabaseRemoteStateClosing {
		t.Fatalf("state = %s", result.State)
	}
}

func TestMariaDBRemoteLeavesAnEditedOptionFile(t *testing.T) {
	f := &fakeMariaDB{accounts: map[string]bool{"shop_u@localhost": true}}
	files := withFakeMariaDB(t, f)
	edited := dbremote.MariaDBOptionFile("", "") + "max_connections = 50\n"
	files[dbremote.MariaDBOptionFileDebian] = []byte(edited)
	target := dbremote.Target{Engine: "mysql", Port: 3306}
	var ts dbremote.TargetState
	result := mariadbRemoteInstance{}.apply(target, dbRemoteFacts{running: true}, &ts, "")
	if string(files[dbremote.MariaDBOptionFileDebian]) != edited {
		t.Fatal("an edited option file was removed")
	}
	found := false
	for _, p := range result.Problems {
		found = found || p.Code == transport.DatabaseRemoteOwnerFile
	}
	if !found {
		t.Fatalf("problems = %+v", result.Problems)
	}
}

func TestMariaDBRemoteInspectLeavesOwnerNetworkingAlone(t *testing.T) {
	restore := databaseRemoteMySQLQuery
	defer func() { databaseRemoteMySQLQuery = restore }()
	databaseRemoteMySQLQuery = func(string) (string, error) { return "0.0.0.0\t0\tNO\t3306\t\t2\t/var/lib/mysql/\n", nil }
	if f := (mariadbRemoteInstance{}).inspect(dbremote.Target{Engine: "mysql", Port: 3306}, dbremote.TargetState{}); !f.ownerManaged {
		t.Fatal("an owner's bind-address was not recognised")
	}
	databaseRemoteMySQLQuery = func(string) (string, error) { return "127.0.0.1\t1\tNO\t3306\t\t2\t/var/lib/mysql/\n", nil }
	if f := (mariadbRemoteInstance{}).inspect(dbremote.Target{Engine: "mysql", Port: 3306}, dbremote.TargetState{}); !f.ownerManaged {
		t.Fatal("skip-networking was not recognised")
	}
}

func TestFollowMariaDBUserDropsCopiesOfADroppedUser(t *testing.T) {
	f := &fakeMariaDB{accounts: map[string]bool{"shop_u@203.0.113.7": true, "other@203.0.113.7": true}}
	withFakeMariaDB(t, f)
	if err := saveDatabaseRemoteState(dbremote.State{Targets: map[string]dbremote.TargetState{
		"mysql:3306": {Engine: "mysql", Port: 3306, Listen: true, Accounts: []string{"shop_u@203.0.113.7", "other@203.0.113.7"}},
	}}); err != nil {
		t.Skipf("the record cannot be written here: %v", err)
	}
	if err := databaseRemoteFollowMariaDBUser("shop_u", true); err != nil {
		t.Fatal(err)
	}
	state, err := loadDatabaseRemoteState()
	if err != nil {
		t.Fatal(err)
	}
	if f.accounts["shop_u@203.0.113.7"] || !f.accounts["other@203.0.113.7"] || len(state.Targets["mysql:3306"].Accounts) != 1 {
		t.Fatalf("accounts = %v, record = %+v", f.accounts, state.Targets)
	}
	// A password change gives the copy the local account's new credential.
	f.executed = nil
	if err := databaseRemoteFollowMariaDBUser("other", false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(f.executed, "\n"), "ALTER USER 'other'@'203.0.113.7'") && f.accounts["other@203.0.113.7"] {
		t.Fatalf("the copy neither followed nor went: %v", f.executed)
	}
}

func TestDatabaseRemoteCertificate(t *testing.T) {
	now := time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC)
	certPEM, keyPEM, err := newDatabaseRemoteCertificate("db.example.com", now)
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(certPEM)
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if cert.Subject.CommonName != "db.example.com" || cert.NotAfter.Before(now.AddDate(9, 11, 0)) || cert.IsCA {
		t.Fatalf("certificate: %v %v", cert.Subject, cert.NotAfter)
	}
	keyBlock, _ := pem.Decode(keyPEM)
	key, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
	if _, ok := key.(*ecdsa.PrivateKey); err != nil || !ok {
		t.Fatalf("key: %T %v", key, err)
	}
}

func TestNormalizeDBRemoteRequestRefusesRepeatsAndBadRules(t *testing.T) {
	if _, err := normalizeDBRemoteRequest([]transport.DatabaseRemoteTarget{{Engine: "mysql", Port: 3306}, {Engine: "mysql", Port: 3306}}); err == nil {
		t.Fatal("a repeated target was accepted")
	}
	if _, err := normalizeDBRemoteRequest([]transport.DatabaseRemoteTarget{{Engine: "mysql", Port: 3306, Rules: []transport.DatabaseRemoteRule{{Database: "shop", User: "shop_u", Address: "0.0.0.0/0"}}}}); err == nil {
		t.Fatal("0.0.0.0/0 was accepted")
	}
}

func orDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
