package main

import (
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/alicelik/celikpanel/internal/dbremote"
	"github.com/alicelik/celikpanel/internal/transport"
)

// MariaDB side of remote access. Each (user, address) gets an account of its
// own with the local account's credential, REQUIRE SSL and only the grants
// the local account holds on the databases its rules name. CelikPanel's
// option file sets bind-address (and TLS when MariaDB has none) and takes
// effect at the next start; a Fail2ban jail slows wrong passwords down.
//
// MariaDB tarafı: her (kullanıcı, adres) için yerel hesabın kimliğiyle,
// REQUIRE SSL ve yalnız gereken yetkilerle bir hesap.

// databaseRemoteMySQLQuery runs statements as the engine's root over its
// socket (mysql --batch --raw --skip-column-names). Set per platform;
// swapped by tests.
var databaseRemoteMySQLQuery = func(statement string) (string, error) {
	return "", errors.New("MariaDB is not reachable on this platform")
}

// databaseRemoteFiles is the file side (option file, jail), swapped by tests.
var databaseRemoteFiles = struct {
	read   func(path string) ([]byte, error)
	write  func(path string, content []byte) error
	remove func(path string) error
	isDir  func(path string) bool
}{
	read:   os.ReadFile,
	write:  func(path string, content []byte) error { return secureWriteConfig(path, content, 0o644) },
	remove: os.Remove,
	isDir: func(path string) bool {
		info, err := os.Stat(path)
		return err == nil && info.IsDir()
	},
}

// databaseRemoteValidateMariaDBOption lets mariadbd read the option file
// before it is written (db_config.go); swapped by tests.
var databaseRemoteValidateMariaDBOption = func(path string, content []byte) error {
	return dbConfigValidateWithDaemon(dbConfigTarget{kind: dbConfigMariaDB, unit: "mariadb"},
		dnsFileSnapshot{Path: path, Exists: false, Mode: 0o644, OwnerKnown: true}, content)
}

// databaseRemoteFail2ban reloads Fail2ban and says whether it, and one of
// its jails, runs.
var databaseRemoteFail2ban = struct {
	// installed: fail2ban-client is on this server (alpha.130: after a
	// removal its folders stay behind with CelikPanel's files in them).
	installed  func() bool
	active     func() bool
	jailActive func(name string) bool
	reload     func() error
}{
	installed:  func() bool { return false },
	active:     func() bool { return false },
	jailActive: func(string) bool { return false },
	reload:     func() error { return errors.New("Fail2ban is not reachable on this platform") },
}

// databaseRemoteMariaDBSkipNameResolve is the administrator's agreement
// carried by the apply in progress (transport.DatabaseRemoteApplyRequest);
// set and read under databaseRemoteMu.
var databaseRemoteMariaDBSkipNameResolve bool

// fail2banInstalled: Fail2ban is on this server with its folders.
func fail2banInstalled() bool {
	return databaseRemoteFiles.isDir("/etc/fail2ban/jail.d") && databaseRemoteFiles.isDir("/etc/fail2ban/filter.d") &&
		databaseRemoteFail2ban.installed()
}

const (
	mariadbRemoteUnit = "mariadb.service"
	mariadbCertDir    = "/etc/ssl/celikpanel-database/mariadb"
)

type mariadbRemoteInstance struct{}

type mariadbRemoteSettings struct {
	bind           string
	skipNetworking bool
	haveSSL        bool
	port           int
	logError       string
	logWarnings    int
	dataDir        string
	// skipNameResolve: the running MariaDB does not look client host names
	// up, so a refused sign-in names the client's address (alpha.130).
	skipNameResolve bool
}

func (mariadbRemoteInstance) read() (mariadbRemoteSettings, error) {
	out, err := databaseRemoteMySQLQuery("SELECT @@bind_address, @@skip_networking, @@have_ssl, @@port, @@log_error, @@log_warnings, @@datadir, @@skip_name_resolve;")
	if err != nil {
		return mariadbRemoteSettings{}, err
	}
	fields := strings.Split(strings.TrimSpace(out), "\t")
	if len(fields) != 8 {
		return mariadbRemoteSettings{}, fmt.Errorf("MariaDB answered %d values, CelikPanel asked for 8", len(fields))
	}
	port, _ := strconv.Atoi(fields[3])
	bind := fields[0]
	if bind == "NULL" {
		bind = ""
	}
	warnings, _ := strconv.Atoi(fields[5])
	return mariadbRemoteSettings{bind: bind, skipNetworking: fields[1] == "1", haveSSL: fields[2] == "YES", port: port,
		logError: fields[4], logWarnings: warnings, dataDir: fields[6], skipNameResolve: fields[7] == "1"}, nil
}

// mariadbBindIsLocal: bind-address names this server only. MariaDB without
// a bind-address listens on every interface.
func mariadbBindIsLocal(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	for _, part := range strings.Split(value, ",") {
		switch strings.TrimSpace(part) {
		case "127.0.0.1", "localhost", "::1":
		default:
			return false
		}
	}
	return true
}

// mariadbOptionPath is CelikPanel's option file on this layout.
func mariadbOptionPath() (string, error) {
	switch {
	case databaseRemoteFiles.isDir("/etc/mysql/mariadb.conf.d"):
		return dbremote.MariaDBOptionFileDebian, nil
	case databaseRemoteFiles.isDir("/etc/my.cnf.d"):
		return dbremote.MariaDBOptionFileArch, nil
	}
	return "", errors.New("neither /etc/mysql/mariadb.conf.d nor /etc/my.cnf.d exists, so CelikPanel has no place for its MariaDB option file")
}

func (m mariadbRemoteInstance) inspect(target dbremote.Target, ts dbremote.TargetState) dbRemoteFacts {
	s, err := m.read()
	if err != nil {
		return dbRemoteFacts{readError: "MariaDB did not answer: " + boundedText(err.Error(), 200)}
	}
	facts := dbRemoteFacts{running: true, listenRemote: !s.skipNetworking && !mariadbBindIsLocal(s.bind)}
	if !ts.Listen {
		switch {
		case s.skipNetworking:
			facts.ownerManaged = true
			facts.ownerDetail = "skip-networking is on: MariaDB takes no network connections, set outside CelikPanel"
		case facts.listenRemote && !ts.ListenPending:
			facts.ownerManaged = true
			facts.ownerDetail = fmt.Sprintf("bind-address = %q was set outside CelikPanel", s.bind)
		}
	}
	return facts
}

// mariadbAccountState is one account as MariaDB holds it.
type mariadbAccountState struct {
	credential dbremote.MariaDBCredential
	grants     dbremote.LocalGrants
	requireSSL bool
}

// mariadbReadAccount reads one account: credential, connection limit, lock,
// TLS requirement and grants. host is "localhost" or a remote copy's host.
func mariadbReadAccount(user, host string) (mariadbAccountState, error) {
	if _, err := dbremote.MariaDBAccountName(user, "203.0.113.1"); err != nil { // validates the user only
		return mariadbAccountState{}, err
	}
	if host != "localhost" && !dbremote.IsRemoteAccountHost(host) {
		return mariadbAccountState{}, errors.New("not a host CelikPanel reads")
	}
	quoted, hostQuoted := "'"+user+"'", "'"+host+"'" // checked identifier and host
	out, err := databaseRemoteMySQLQuery("SELECT plugin, authentication_string, max_user_connections, " +
		"IF(JSON_VALUE(Priv, '$.account_locked') IN ('true', '1'), 1, 0), ssl_type FROM mysql.global_priv " +
		"JOIN mysql.user USING (User, Host) WHERE User = " + quoted + " AND Host = " + hostQuoted + ";")
	if err != nil {
		return mariadbAccountState{}, err
	}
	rows := pgRows(out)
	if len(rows) != 1 {
		return mariadbAccountState{}, errMariaDBUserMissing
	}
	fields := strings.Split(rows[0], "\t")
	if len(fields) != 5 {
		return mariadbAccountState{}, errors.New("MariaDB's answer about the account could not be read")
	}
	limit, err := strconv.Atoi(fields[2])
	if err != nil {
		return mariadbAccountState{}, errors.New("MariaDB's answer about the account's connection limit could not be read")
	}
	state := mariadbAccountState{
		credential: dbremote.MariaDBCredential{Plugin: fields[0], Auth: fields[1], MaxUserConnections: limit, Locked: fields[3] == "1"},
		requireSSL: fields[4] == "ANY",
	}
	grantsOut, err := databaseRemoteMySQLQuery("SHOW GRANTS FOR " + quoted + "@" + hostQuoted + ";")
	if err != nil {
		return mariadbAccountState{}, err
	}
	state.grants, err = dbremote.ParseMariaDBGrants(pgRows(grantsOut), user, host)
	return state, err
}

// mariadbLocalAccount reads the local account's credential and grants.
func mariadbLocalAccount(user string) (dbremote.MariaDBCredential, dbremote.LocalGrants, error) {
	state, err := mariadbReadAccount(user, "localhost")
	return state.credential, state.grants, err
}

// mariadbCopyChanges answers the statements that bring an existing remote
// copy to the wanted credential and grants, and nothing when it already is
// (re-review). A copy that cannot be read, or that holds a server-wide
// privilege it must never have, is reset as a whole.
func mariadbCopyChanges(account dbremote.MariaDBAccount, credential dbremote.MariaDBCredential, wanted map[string]string) ([]string, error) {
	alter, err := dbremote.AlterMariaDBAccount(account.User, account.Host, credential)
	if err != nil {
		return nil, err
	}
	current, readErr := mariadbReadAccount(account.User, account.Host)
	if readErr != nil || current.grants.Global {
		revoke, _ := dbremote.RevokeAllMariaDB(account.User, account.Host)
		batch := []string{alter, revoke}
		for db, privileges := range wanted {
			grant, err := dbremote.GrantMariaDB(privileges, db, account.User, account.Host)
			if err != nil {
				return nil, err
			}
			batch = append(batch, grant)
		}
		return batch, nil
	}
	var batch []string
	if current.credential != credential || !current.requireSSL {
		batch = append(batch, alter)
	}
	databases := map[string]bool{}
	for db := range wanted {
		databases[db] = true
	}
	for db := range current.grants.Databases {
		databases[db] = true
	}
	names := make([]string, 0, len(databases))
	for db := range databases {
		names = append(names, db)
	}
	sort.Strings(names)
	for _, db := range names {
		change, err := dbremote.MariaDBPrivilegeChange(current.grants.Databases[db], wanted[db], db, account.User, account.Host)
		if err != nil {
			return nil, err
		}
		batch = append(batch, change...)
	}
	return batch, nil
}

var errMariaDBUserMissing = errors.New("the user does not exist on MariaDB")

// mariadbAccountHosts lists the hosts of every account named user.
func mariadbAccountHosts(user string) (map[string]bool, error) {
	out, err := databaseRemoteMySQLQuery("SELECT Host FROM mysql.user WHERE User = '" + user + "';")
	if err != nil {
		return nil, err
	}
	hosts := map[string]bool{}
	for _, row := range pgRows(out) {
		hosts[row] = true
	}
	return hosts, nil
}

func splitAccountID(id string) (user, host string) {
	user, host, _ = strings.Cut(id, "@")
	return user, host
}

func (m mariadbRemoteInstance) apply(target dbremote.Target, facts dbRemoteFacts, ts *dbremote.TargetState, hostName string) transport.DatabaseRemoteTargetResult {
	result := transport.DatabaseRemoteTargetResult{Engine: target.Engine, Port: target.Port, Unit: "mariadb", TLS: "off", SlowDown: "none"}
	problem := func(code, detail string) {
		result.Problems = append(result.Problems, transport.DatabaseRemoteProblem{Code: code, Detail: detail})
	}
	s, err := m.read()
	if err != nil {
		result.State = transport.DatabaseRemoteStateFailed
		problem(transport.DatabaseRemoteEngineDown, err.Error())
		return result
	}
	if s.port != 0 && s.port != target.Port {
		result.State = transport.DatabaseRemoteStateFailed
		problem(transport.DatabaseRemoteFailed, fmt.Sprintf("MariaDB runs on port %d, the panel's record says %d; nothing was changed", s.port, target.Port))
		return result
	}
	wantOpen := len(target.Rules) > 0

	// 1. The option file (listen address, TLS, host name lookup).
	var names mariadbNameResolvePlan
	path, pathErr := mariadbOptionPath()
	if pathErr != nil && (wantOpen || ts.Listen) {
		problem(transport.DatabaseRemoteFailed, pathErr.Error())
	}
	certPath, keyPath := mariadbCertDir+"/server.crt", mariadbCertDir+"/server.key"
	if pathErr == nil {
		current, readErr := databaseRemoteFiles.read(path)
		exists := readErr == nil
		own := exists && dbremote.IsOwnMariaDBOptionFile(string(current), certPath, keyPath)
		switch {
		case exists && !own:
			problem(transport.DatabaseRemoteOwnerFile, path+" was changed outside CelikPanel; CelikPanel leaves it as it is")
			// An owner's file may still turn the lookup off: the running
			// value decides below.
			names = mariadbPlanNameResolve(wantOpen, s, false, path)
		case wantOpen:
			names = mariadbPlanNameResolve(wantOpen, s, own && dbremote.MariaDBOptionFileSkipsNameResolve(string(current)), path)
			var content string
			if s.haveSSL && !(own && strings.Contains(string(current), "ssl_cert")) {
				content = dbremote.MariaDBOptionFileWith("", "", names.write)
			} else {
				cert, key, err := ensureDatabaseRemoteCertificate("mariadb", "mysql", hostName)
				if err != nil {
					problem(transport.DatabaseRemoteTLSMissing, err.Error())
					content = dbremote.MariaDBOptionFileWith("", "", names.write)
				} else {
					content = dbremote.MariaDBOptionFileWith(cert, key, names.write)
					ts.TLS = true
				}
			}
			if !exists || string(current) != content {
				if err := databaseRemoteValidateMariaDBOption(path, []byte(content)); err != nil {
					problem(transport.DatabaseRemoteFailed, "MariaDB does not accept CelikPanel's option file: "+boundedText(err.Error(), 200))
				} else if err := databaseRemoteFiles.write(path, []byte(content)); err != nil {
					problem(transport.DatabaseRemoteFailed, "the option file could not be written: "+err.Error())
				} else {
					ts.Listen, ts.ListenPending = true, false
					ts.SkipNameResolve = names.write
				}
			} else {
				ts.Listen = true
				ts.SkipNameResolve = names.write
			}
		case exists && own:
			if err := databaseRemoteFiles.remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				problem(transport.DatabaseRemoteFailed, "the option file could not be removed: "+err.Error())
			} else {
				ts.Listen, ts.TLS, ts.SkipNameResolve = false, false, false
				ts.ListenPending = facts.listenRemote
			}
		default:
			if ts.Listen {
				ts.Listen = false
				ts.ListenPending = facts.listenRemote
			}
			ts.SkipNameResolve = false
		}
	}
	switch {
	case s.skipNameResolve:
		result.NameResolve = "off"
	case ts.SkipNameResolve:
		result.NameResolve = "pending_restart"
	default:
		result.NameResolve = "on"
	}
	if wantOpen && !s.skipNameResolve && names.code != "" {
		problem(names.code, names.detail)
	}
	if ts.ListenPending && !facts.listenRemote {
		ts.ListenPending = false
	}

	// 2. Slowing wrong passwords down (review M3): the jail must be running
	// before any copy exists, and TLS must run before a REQUIRE SSL copy is
	// of any use. alpha.130: and MariaDB must name addresses in refused
	// sign-ins (host name lookup off), or the jail bans nobody. Until all
	// three hold, the rules wait and no copy is made.
	m.applyJail(target, wantOpen, s, ts, &result)
	ready := s.haveSSL && result.SlowDown == "fail2ban" && s.skipNameResolve
	accountTarget := dbremote.Target{Engine: target.Engine, Port: target.Port}
	if ready {
		accountTarget.Rules = target.Rules
	}

	// 3. Accounts: the desired ones from the rules, each checked against
	// the local account.
	desired, err := dbremote.MariaDBAccounts(accountTarget)
	if err != nil {
		result.State = transport.DatabaseRemoteStateFailed
		problem(transport.DatabaseRemoteFailed, err.Error())
		return result
	}
	made := map[string]bool{}
	for _, id := range ts.Accounts {
		made[id] = true
	}
	keep := map[string]bool{}
	var active []dbremote.Rule
	for _, account := range desired {
		rulesOf := func() []dbremote.Rule {
			var rs []dbremote.Rule
			for _, r := range target.Rules {
				if r.User == account.User {
					if prefix, err := parsePrefix(r.Address); err == nil {
						if host, err := dbremote.MariaDBHost(prefix); err == nil && host == account.Host {
							rs = append(rs, r)
						}
					}
				}
			}
			return rs
		}
		refuseAll := func(code, detail string) {
			for _, r := range rulesOf() {
				result.Problems = append(result.Problems, ruleProblem(code, r, detail))
			}
		}
		credential, grants, err := mariadbLocalAccount(account.User)
		switch {
		case errors.Is(err, errMariaDBUserMissing):
			refuseAll(transport.DatabaseRemoteUserMissing, "the user "+account.User+" does not exist on MariaDB")
			continue
		case err != nil:
			refuseAll(transport.DatabaseRemoteFailed, "the user "+account.User+" could not be read: "+boundedText(err.Error(), 200))
			continue
		case grants.Global:
			refuseAll(transport.DatabaseRemotePrivilegedUser, "the user "+account.User+" has server-wide rights and is never reachable from outside")
			continue
		}
		if err := credential.Validate(); err != nil {
			refuseAll(transport.DatabaseRemotePasswordMD5, err.Error())
			continue
		}
		var statements []string
		var granted []string
		wanted := map[string]string{}
		for _, db := range account.Databases {
			privileges, ok := grants.Databases[db]
			if !ok {
				for _, r := range rulesOf() {
					if r.Database == db {
						result.Problems = append(result.Problems, ruleProblem(transport.DatabaseRemoteNoAccess, r, "the user "+account.User+" has no access to "+db+" on this server"))
					}
				}
				continue
			}
			grant, err := dbremote.GrantMariaDB(privileges, db, account.User, account.Host)
			if err != nil {
				continue
			}
			statements = append(statements, grant)
			granted = append(granted, db)
			wanted[db] = privileges
		}
		if len(granted) == 0 {
			continue
		}
		hosts, err := mariadbAccountHosts(account.User)
		if err != nil {
			refuseAll(transport.DatabaseRemoteFailed, "the accounts of "+account.User+" could not be read")
			continue
		}
		var batch []string
		if hosts[account.Host] {
			if !made[account.ID()] {
				refuseAll(transport.DatabaseRemoteAccountConflict, "the account "+account.ID()+" already exists on MariaDB and was not made by CelikPanel; CelikPanel leaves it as it is")
				continue
			}
			// Only what differs (re-review): an unchanged copy gets no
			// statement, so its sessions never see a REVOKE→GRANT window.
			batch, err = mariadbCopyChanges(account, credential, wanted)
		} else {
			var head string
			head, err = dbremote.CreateMariaDBAccount(account.User, account.Host, credential)
			batch = append([]string{head}, statements...)
		}
		if err != nil {
			refuseAll(transport.DatabaseRemoteFailed, err.Error())
			continue
		}
		made[account.ID()] = true // from here on it may exist: it is CelikPanel's
		if len(batch) == 0 {
			// already right
		} else if _, err := databaseRemoteMySQLQuery(strings.Join(batch, "\n")); err != nil {
			// The engine's words may quote the statement (and so the hash):
			// never forwarded.
			refuseAll(transport.DatabaseRemoteFailed, "MariaDB did not accept the account "+account.ID())
			continue
		}
		keep[account.ID()] = true
		for _, r := range rulesOf() {
			if containsString(granted, r.Database) {
				active = append(active, r)
			}
		}
	}
	// Accounts CelikPanel made that no rule needs any more (or that wait).
	for id := range made {
		if keep[id] {
			continue
		}
		user, host := splitAccountID(id)
		drop, err := dbremote.DropMariaDBAccount(user, host)
		if err == nil {
			_, err = databaseRemoteMySQLQuery(drop)
		}
		if err != nil {
			keep[id] = true // still there: still CelikPanel's to remove
			problem(transport.DatabaseRemoteFailed, "the account "+id+" could not be removed; it is removed at the next change")
		}
	}
	ts.Accounts = ts.Accounts[:0]
	for id := range keep {
		ts.Accounts = append(ts.Accounts, id)
	}
	sort.Strings(ts.Accounts)
	result.Active = toTransportRules(active)

	// The answer.
	result.ListenRemote = facts.listenRemote
	switch {
	case s.haveSSL:
		result.TLS = "on"
	case ts.TLS:
		result.TLS = "pending_restart"
	}
	switch {
	case !wantOpen && ts.ListenPending:
		result.State = transport.DatabaseRemoteStateClosing
		problem(transport.DatabaseRemoteRestartNeeded, "MariaDB still listens beyond this server until it restarts; the port stays closed by the address filter meanwhile")
	case !wantOpen:
		result.State = transport.DatabaseRemoteStateOff
	case ts.Listen && (!facts.listenRemote || !s.haveSSL || (ts.SkipNameResolve && !s.skipNameResolve)):
		result.State = transport.DatabaseRemoteStateWaitRestart
		problem(transport.DatabaseRemoteRestartNeeded, "MariaDB reads its listen address, TLS and host name lookup settings only when it starts")
	case len(active) > 0 && ts.Listen:
		result.State = transport.DatabaseRemoteStateOpen
	default:
		result.State = transport.DatabaseRemoteStateWaiting
	}
	return result
}

// applyJail writes or removes CelikPanel's Fail2ban filter and jail and says
// whether the jail runs (result.SlowDown "fail2ban"), else why not.
func (mariadbRemoteInstance) applyJail(target dbremote.Target, want bool, s mariadbRemoteSettings, ts *dbremote.TargetState, result *transport.DatabaseRemoteTargetResult) {
	noSlowDown := func(detail string) {
		if want {
			result.Problems = append(result.Problems, transport.DatabaseRemoteProblem{Code: transport.DatabaseRemoteNoSlowDown, Detail: detail})
		}
	}
	result.Fail2ban = "unavailable"
	jail, err := dbremote.MariaDBJail(target.Port, mariadbRemoteUnit)
	if err != nil {
		noSlowDown(err.Error())
		return
	}
	filter := dbremote.MariaDBFilter()
	currentJail, jailErr := databaseRemoteFiles.read(dbremote.MariaDBJailPath)
	currentFilter, filterErr := databaseRemoteFiles.read(dbremote.MariaDBFilterPath)
	jailExists, filterExists := jailErr == nil, filterErr == nil
	jailOwn := jailExists && string(currentJail) == jail
	// CelikPanel's own earlier filter is replaced, not kept as the owner's.
	filterEarlier := filterExists && dbremote.MariaDBFilterEarlier(string(currentFilter))
	filterOwn := filterExists && (string(currentFilter) == filter || filterEarlier)
	installed := fail2banInstalled()
	// No rule, or Fail2ban removed (alpha.130, no leftovers): CelikPanel's
	// own files go; an owner's version stays.
	if !want || !installed {
		changed := false
		if jailOwn {
			changed = databaseRemoteFiles.remove(dbremote.MariaDBJailPath) == nil
		}
		if filterOwn {
			changed = databaseRemoteFiles.remove(dbremote.MariaDBFilterPath) == nil || changed
		}
		ts.Jail = false
		if changed && installed {
			_ = databaseRemoteFail2ban.reload()
		}
		if !want {
			result.Fail2ban = "off"
			return
		}
	}
	if logError := strings.TrimSpace(s.logError); logError != "" && logError != "stderr" {
		// Journal only (re-review LOW 1): in a log file a crafted user name
		// can end a line that blames another address.
		noSlowDown("MariaDB writes its log to " + logError + " (log_error); the protection against password guessing reads only the journal, where a crafted user name cannot fake a line. Remove log_error from MariaDB's settings, restart MariaDB, then apply again")
		return
	}
	switch {
	case !installed:
		noSlowDown("Fail2ban is not installed; install it in Components, then apply again")
		return
	case !databaseRemoteFail2ban.active():
		noSlowDown("Fail2ban is installed but does not run; start it in Components, then apply again")
		return
	case s.logWarnings < 2:
		noSlowDown(fmt.Sprintf("MariaDB does not log refused sign-ins (log_warnings = %d); set log_warnings = 2, MariaDB's default, and restart MariaDB", s.logWarnings))
		return
	case (jailExists && !jailOwn) || (filterExists && !filterOwn):
		// The owner's version: used as it is, if it runs.
	default:
		wrote := false
		if !filterExists || filterEarlier {
			if err := databaseRemoteFiles.write(dbremote.MariaDBFilterPath, []byte(filter)); err != nil {
				noSlowDown("the Fail2ban filter could not be written: " + err.Error())
				return
			}
			wrote = true
		}
		if !jailExists {
			if err := databaseRemoteFiles.write(dbremote.MariaDBJailPath, []byte(jail)); err != nil {
				noSlowDown("the Fail2ban jail could not be written: " + err.Error())
				return
			}
			wrote = true
		}
		ts.Jail = true
		if wrote {
			if err := databaseRemoteFail2ban.reload(); err != nil {
				_ = databaseRemoteFiles.remove(dbremote.MariaDBJailPath)
				_ = databaseRemoteFiles.remove(dbremote.MariaDBFilterPath)
				_ = databaseRemoteFail2ban.reload()
				ts.Jail = false
				noSlowDown("Fail2ban did not accept the MariaDB jail, so it was taken back; its log says why")
				return
			}
		}
	}
	if !databaseRemoteFail2ban.jailActive(dbremote.MariaDBJailName) {
		noSlowDown("Fail2ban does not run the " + dbremote.MariaDBJailName + " jail; its log says why (on a journal-only system Fail2ban needs its systemd backend)")
		return
	}
	result.SlowDown = "fail2ban"
	result.Fail2ban = "on"
}

// mariadbNameResolvePlan is what CelikPanel does about MariaDB's host name
// lookup (alpha.130): write turns it off in CelikPanel's option file; code
// and detail say what keeps it on.
type mariadbNameResolvePlan struct {
	write  bool
	code   string
	detail string
}

// mariadbPlanNameResolve decides the lookup for an engine that should carry
// rules. CelikPanel turns the lookup off only with the administrator's
// agreement (it changes how every account matches: a program on this server
// that connects through 127.0.0.1, or an account written with a host name,
// stops matching), never against an owner's own setting, and never while an
// account with a host name exists. ownSkip: CelikPanel's file already turns
// it off.
func mariadbPlanNameResolve(wantOpen bool, s mariadbRemoteSettings, ownSkip bool, ownPath string) mariadbNameResolvePlan {
	if !wantOpen {
		return mariadbNameResolvePlan{}
	}
	ownerValue, ownerFile := mariadbOwnerNameResolve(ownPath)
	switch {
	case ownerValue == "lookup_on":
		return mariadbNameResolvePlan{code: transport.DatabaseRemoteOwnerNameResolve,
			detail: ownerFile + " turns MariaDB's host name lookup on (skip-name-resolve off); CelikPanel does not change your setting"}
	case s.skipNameResolve && !ownSkip:
		// Off already, by the owner (or MariaDB's own build): nothing to do.
		return mariadbNameResolvePlan{}
	case !databaseRemoteMariaDBSkipNameResolve:
		if ownSkip && s.skipNameResolve {
			// The agreement was withdrawn: the line goes, MariaDB keeps its
			// running value until the next start.
			return mariadbNameResolvePlan{}
		}
		return mariadbNameResolvePlan{code: transport.DatabaseRemoteNameResolve,
			detail: "MariaDB looks up the host name of every client and then names the host, not its address, in a refused sign-in, so Fail2ban cannot ban it"}
	case ownSkip:
		return mariadbNameResolvePlan{write: true}
	}
	accounts, err := mariadbHostNameAccounts()
	if err != nil {
		return mariadbNameResolvePlan{code: transport.DatabaseRemoteNameResolve,
			detail: "the accounts of MariaDB could not be read (" + boundedText(err.Error(), 120) + "), so host name lookup stays on"}
	}
	if len(accounts) > 0 {
		shown := accounts
		if len(shown) > 5 {
			shown = append(append([]string(nil), shown[:5]...), fmt.Sprintf("and %d more", len(accounts)-5))
		}
		return mariadbNameResolvePlan{code: transport.DatabaseRemoteNameAccounts, detail: strings.Join(shown, ", ")}
	}
	return mariadbNameResolvePlan{write: true}
}

// databaseRemoteMariaDBConfigFiles lists MariaDB's option files on this
// server (Debian/Ubuntu and Arch layouts); swapped by tests.
var databaseRemoteMariaDBConfigFiles = func() []string {
	files := []string{"/etc/mysql/my.cnf", "/etc/mysql/mariadb.cnf", "/etc/my.cnf"}
	for _, pattern := range []string{"/etc/mysql/conf.d/*.cnf", "/etc/mysql/mariadb.conf.d/*.cnf", "/etc/my.cnf.d/*.cnf"} {
		matches, _ := filepath.Glob(pattern)
		sort.Strings(matches)
		files = append(files, matches...)
	}
	return files
}

// mariadbOwnerNameResolve reads the owner's own setting of the lookup from
// MariaDB's option files (CelikPanel's own file left out): "lookup_on"
// (skip-name-resolve set off), "lookup_off" (set on) or "" (not set). The
// last setting read wins, as in MariaDB.
func mariadbOwnerNameResolve(ownPath string) (value, file string) {
	for _, path := range databaseRemoteMariaDBConfigFiles() {
		if path == ownPath || path == dbremote.MariaDBOptionFileDebian || path == dbremote.MariaDBOptionFileArch {
			continue
		}
		data, err := databaseRemoteFiles.read(path)
		if err != nil {
			continue
		}
		if v := mariadbNameResolveSetting(string(data)); v != "" {
			value, file = v, path
		}
	}
	return value, file
}

// mariadbNameResolveSetting reads skip-name-resolve from the server sections
// of one option file's text.
func mariadbNameResolveSetting(text string) string {
	value := ""
	server := false
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || line[0] == '#' || line[0] == ';' {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			name := strings.ToLower(strings.TrimSpace(line[1 : len(line)-1]))
			server = strings.HasPrefix(name, "mysqld") || strings.HasPrefix(name, "mariadbd") || name == "server" ||
				name == "galera" || name == "mariadb" || (strings.HasPrefix(name, "mariadb-") && len(name) > 8 && name[8] >= '0' && name[8] <= '9')
			continue
		}
		if !server {
			continue
		}
		key, val, hasValue := strings.Cut(line, "=")
		key = strings.ReplaceAll(strings.ToLower(strings.TrimSpace(key)), "-", "_")
		val = strings.ToLower(strings.Trim(strings.TrimSpace(val), `'"`))
		switch key {
		case "skip_name_resolve":
			if !hasValue || val == "1" || val == "on" || val == "true" {
				value = "lookup_off"
			} else if val == "0" || val == "off" || val == "false" {
				value = "lookup_on"
			}
		case "disable_skip_name_resolve", "skip_skip_name_resolve":
			value = "lookup_on"
		}
	}
	return value
}

// mariadbHostNameAccounts lists the accounts (user@host) whose host is a
// name: with the lookup off MariaDB matches clients by address only, so
// these would stop working. localhost keeps working (the socket).
func mariadbHostNameAccounts() ([]string, error) {
	out, err := databaseRemoteMySQLQuery("SELECT User, Host FROM mysql.user;")
	if err != nil {
		return nil, err
	}
	var named []string
	for _, row := range pgRows(out) {
		user, host, ok := strings.Cut(row, "\t")
		if !ok || mariadbHostIsAddressForm(host) {
			continue
		}
		named = append(named, "'"+user+"'@'"+host+"'")
	}
	sort.Strings(named)
	return named, nil
}

// mariadbHostIsAddressForm: host matches clients by address (or is the local
// socket's localhost), not by a host name.
func mariadbHostIsAddressForm(host string) bool {
	if host == "localhost" || host == "%" || host == "" {
		return true
	}
	if _, err := netip.ParseAddr(host); err == nil {
		return true
	}
	if address, mask, ok := strings.Cut(host, "/"); ok {
		_, errA := netip.ParseAddr(address)
		_, errM := netip.ParseAddr(mask)
		return errA == nil && errM == nil
	}
	ipv4Pattern, ipv6Pattern := true, strings.Contains(host, ":")
	for _, r := range host {
		digit := r >= '0' && r <= '9'
		hex := digit || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
		if !digit && r != '.' && r != '%' && r != '_' {
			ipv4Pattern = false
		}
		if !hex && r != ':' && r != '%' && r != '_' {
			ipv6Pattern = false
		}
	}
	return ipv4Pattern || ipv6Pattern
}

// databaseRemoteFollowMariaDBUser keeps CelikPanel's remote copies of one
// MariaDB user in step with the local account: after its password changed
// each copy gets the new credential; after it was dropped, or when a copy
// cannot follow, the copy is removed (closed rather than left with an old
// password). Called by the password change (transfer_rotation.go) and the
// database deletion (database_rpc.go) of this Agent; the panel's own engine
// driver does the same for every host of the user (mariadb_driver.go).
//
// CelikPanel'in bir MariaDB kullanıcısının uzak kopyalarını yerel hesapla
// uyumlu tutar; uyamayan kopya kaldırılır.
func databaseRemoteFollowMariaDBUser(user string, dropped bool) error {
	databaseRemoteMu.Lock()
	defer databaseRemoteMu.Unlock()
	state, err := loadDatabaseRemoteState()
	if err != nil {
		return err
	}
	changed := false
	var failed []string
	var credential *dbremote.MariaDBCredential
	for key, ts := range state.Targets {
		if ts.Engine != dbremote.EngineMySQL {
			continue
		}
		var keep []string
		for _, id := range ts.Accounts {
			u, host := splitAccountID(id)
			if u != user {
				keep = append(keep, id)
				continue
			}
			if !dropped {
				if credential == nil {
					if c, _, err := mariadbLocalAccount(user); err == nil {
						credential = &c
					}
				}
				if credential != nil {
					if statement, err := dbremote.AlterMariaDBAccount(u, host, *credential); err == nil {
						if _, err := databaseRemoteMySQLQuery(statement); err == nil {
							keep = append(keep, id)
							continue
						}
					}
				}
			}
			changed = true
			drop, err := dbremote.DropMariaDBAccount(u, host)
			if err == nil {
				_, err = databaseRemoteMySQLQuery(drop)
			}
			if err != nil {
				failed = append(failed, id)
				keep = append(keep, id)
			}
		}
		ts.Accounts = keep
		state.Targets[key] = ts
	}
	if changed {
		if err := saveDatabaseRemoteState(state); err != nil {
			return err
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("the remote copies %s could not be updated or removed", strings.Join(failed, ", "))
	}
	return nil
}
