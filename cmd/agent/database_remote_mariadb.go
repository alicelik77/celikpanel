package main

import (
	"errors"
	"fmt"
	"os"
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

// databaseRemoteFail2ban reloads Fail2ban and says whether it runs.
var databaseRemoteFail2ban = struct {
	active func() bool
	reload func() error
}{
	active: func() bool { return false },
	reload: func() error { return errors.New("Fail2ban is not reachable on this platform") },
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
}

func (mariadbRemoteInstance) read() (mariadbRemoteSettings, error) {
	out, err := databaseRemoteMySQLQuery("SELECT @@bind_address, @@skip_networking, @@have_ssl, @@port;")
	if err != nil {
		return mariadbRemoteSettings{}, err
	}
	fields := strings.Split(strings.TrimSpace(out), "\t")
	if len(fields) != 4 {
		return mariadbRemoteSettings{}, fmt.Errorf("MariaDB answered %d values, CelikPanel asked for 4", len(fields))
	}
	port, _ := strconv.Atoi(fields[3])
	bind := fields[0]
	if bind == "NULL" {
		bind = ""
	}
	return mariadbRemoteSettings{bind: bind, skipNetworking: fields[1] == "1", haveSSL: fields[2] == "YES", port: port}, nil
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

// mariadbLocalAccount reads the local account's credential and grants.
func mariadbLocalAccount(user string) (dbremote.MariaDBCredential, dbremote.LocalGrants, error) {
	name, err := dbremote.MariaDBAccountName(user, "203.0.113.1") // validates the user only
	_ = name
	if err != nil {
		return dbremote.MariaDBCredential{}, dbremote.LocalGrants{}, err
	}
	quoted := "'" + user + "'" // a checked identifier
	out, err := databaseRemoteMySQLQuery("SELECT plugin, authentication_string, max_user_connections, " +
		"IF(JSON_VALUE(Priv, '$.account_locked') IN ('true', '1'), 1, 0) FROM mysql.global_priv " +
		"JOIN mysql.user USING (User, Host) WHERE User = " + quoted + " AND Host = 'localhost';")
	if err != nil {
		return dbremote.MariaDBCredential{}, dbremote.LocalGrants{}, err
	}
	rows := pgRows(out)
	if len(rows) != 1 {
		return dbremote.MariaDBCredential{}, dbremote.LocalGrants{}, errMariaDBUserMissing
	}
	fields := strings.Split(rows[0], "\t")
	if len(fields) != 4 {
		return dbremote.MariaDBCredential{}, dbremote.LocalGrants{}, errors.New("MariaDB's answer about the account could not be read")
	}
	limit, err := strconv.Atoi(fields[2])
	if err != nil {
		return dbremote.MariaDBCredential{}, dbremote.LocalGrants{}, errors.New("MariaDB's answer about the account's connection limit could not be read")
	}
	credential := dbremote.MariaDBCredential{Plugin: fields[0], Auth: fields[1], MaxUserConnections: limit, Locked: fields[3] == "1"}
	grantsOut, err := databaseRemoteMySQLQuery("SHOW GRANTS FOR " + quoted + "@'localhost';")
	if err != nil {
		return dbremote.MariaDBCredential{}, dbremote.LocalGrants{}, err
	}
	grants, err := dbremote.ParseMariaDBGrants(pgRows(grantsOut), user, "localhost")
	return credential, grants, err
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

	// 1. Accounts: the desired ones from the rules, each checked against
	// the local account.
	desired, err := dbremote.MariaDBAccounts(target)
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
		}
		if len(granted) == 0 {
			continue
		}
		hosts, err := mariadbAccountHosts(account.User)
		if err != nil {
			refuseAll(transport.DatabaseRemoteFailed, "the accounts of "+account.User+" could not be read")
			continue
		}
		var head string
		if hosts[account.Host] {
			if !made[account.ID()] {
				refuseAll(transport.DatabaseRemoteAccountConflict, "the account "+account.ID()+" already exists on MariaDB and was not made by CelikPanel; CelikPanel leaves it as it is")
				continue
			}
			head, err = dbremote.AlterMariaDBAccount(account.User, account.Host, credential)
		} else {
			head, err = dbremote.CreateMariaDBAccount(account.User, account.Host, credential)
		}
		if err != nil {
			refuseAll(transport.DatabaseRemoteFailed, err.Error())
			continue
		}
		revoke, _ := dbremote.RevokeAllMariaDB(account.User, account.Host)
		batch := append([]string{head, revoke}, statements...)
		made[account.ID()] = true // from here on it may exist: it is CelikPanel's
		if _, err := databaseRemoteMySQLQuery(strings.Join(batch, "\n")); err != nil {
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
	// Accounts CelikPanel made that no rule needs any more.
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

	// 2. The option file (listen address, TLS) and 3. the jail.
	path, pathErr := mariadbOptionPath()
	if pathErr != nil && (len(active) > 0 || ts.Listen) {
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
		case len(active) > 0:
			var content string
			if s.haveSSL && !(own && strings.Contains(string(current), "ssl_cert")) {
				content = dbremote.MariaDBOptionFile("", "")
			} else {
				cert, key, err := ensureDatabaseRemoteCertificate("mariadb", "mysql", hostName)
				if err != nil {
					problem(transport.DatabaseRemoteTLSMissing, err.Error())
					content = dbremote.MariaDBOptionFile("", "")
				} else {
					content = dbremote.MariaDBOptionFile(cert, key)
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
				}
			} else {
				ts.Listen = true
			}
		case exists && own:
			if err := databaseRemoteFiles.remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				problem(transport.DatabaseRemoteFailed, "the option file could not be removed: "+err.Error())
			} else {
				ts.Listen, ts.TLS = false, false
				ts.ListenPending = facts.listenRemote
			}
		default:
			if ts.Listen {
				ts.Listen = false
				ts.ListenPending = facts.listenRemote
			}
		}
	}
	if ts.ListenPending && !facts.listenRemote {
		ts.ListenPending = false
	}
	m.applyJail(target, len(active) > 0, ts, &result)

	// The answer.
	result.ListenRemote = facts.listenRemote
	switch {
	case s.haveSSL:
		result.TLS = "on"
	case ts.TLS:
		result.TLS = "pending_restart"
	}
	switch {
	case len(active) == 0 && ts.ListenPending:
		result.State = transport.DatabaseRemoteStateClosing
		problem(transport.DatabaseRemoteRestartNeeded, "MariaDB still listens beyond this server until it restarts; the port stays closed by the address filter meanwhile")
	case len(active) == 0:
		result.State = transport.DatabaseRemoteStateOff
	case ts.Listen && facts.listenRemote && s.haveSSL:
		result.State = transport.DatabaseRemoteStateOpen
	case ts.Listen:
		result.State = transport.DatabaseRemoteStateWaitRestart
		problem(transport.DatabaseRemoteRestartNeeded, "MariaDB reads its listen address and TLS settings only when it starts")
	default:
		result.State = transport.DatabaseRemoteStateFailed
	}
	return result
}

// applyJail writes or removes the Fail2ban jail.
func (mariadbRemoteInstance) applyJail(target dbremote.Target, want bool, ts *dbremote.TargetState, result *transport.DatabaseRemoteTargetResult) {
	jail, err := dbremote.MariaDBJail(target.Port, mariadbRemoteUnit)
	if err != nil {
		return
	}
	current, readErr := databaseRemoteFiles.read(dbremote.MariaDBJailPath)
	exists := readErr == nil
	own := exists && string(current) == jail
	installed := databaseRemoteFiles.isDir("/etc/fail2ban/jail.d")
	switch {
	case want && exists && !own:
		result.SlowDown = "fail2ban" // the owner's own version of the jail
	case want && installed && databaseRemoteFail2ban.active():
		if !exists {
			if err := databaseRemoteFiles.write(dbremote.MariaDBJailPath, []byte(jail)); err != nil {
				break
			}
			if err := databaseRemoteFail2ban.reload(); err != nil {
				_ = databaseRemoteFiles.remove(dbremote.MariaDBJailPath)
				_ = databaseRemoteFail2ban.reload()
				result.Problems = append(result.Problems, transport.DatabaseRemoteProblem{Code: transport.DatabaseRemoteNoSlowDown,
					Detail: "Fail2ban did not accept the MariaDB jail, so it was taken back"})
				return
			}
		}
		ts.Jail = true
		result.SlowDown = "fail2ban"
	case !want && own:
		if err := databaseRemoteFiles.remove(dbremote.MariaDBJailPath); err == nil {
			ts.Jail = false
			_ = databaseRemoteFail2ban.reload()
		}
	}
	if want && result.SlowDown == "none" {
		detail := "Fail2ban is not installed, so wrong MariaDB passwords are not slowed down"
		if installed {
			detail = "Fail2ban is installed but does not run, so wrong MariaDB passwords are not slowed down"
		}
		result.Problems = append(result.Problems, transport.DatabaseRemoteProblem{Code: transport.DatabaseRemoteNoSlowDown, Detail: detail})
	}
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
