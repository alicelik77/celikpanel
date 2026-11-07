package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/dbremote"
	"github.com/alicelik/celikpanel/internal/transport"
)

// PostgreSQL side of remote access. The rules are a managed block of
// hostssl lines in the cluster's pg_hba.conf, written through the validated
// writer (db_config.go); the listen address, TLS and auth_delay are set with
// ALTER SYSTEM, PostgreSQL's own postgresql.auto.conf, and only undone when
// CelikPanel's record says CelikPanel set them.
//
// PostgreSQL tarafı: kurallar pg_hba.conf'ta yönetilen hostssl bloğu; dinleme
// adresi, TLS ve auth_delay ALTER SYSTEM ile.

// databaseRemotePGQuery runs one statement as postgres on the cluster on
// port (psql unaligned, tuples only). Set per platform; swapped by tests.
var databaseRemotePGQuery = func(port int, statement string) (string, error) {
	return "", errors.New("PostgreSQL is not reachable on this platform")
}

const (
	pgRemoteFieldSep = "\x1f"
	pgRemotePairSep  = "\x1e"
	// pgAuthDelayMilliseconds: the wait after each failed sign-in.
	pgAuthDelayMilliseconds = "1000"
)

type pgRemoteInstance struct{}

// pgRemoteSettings is what one cluster says about itself.
type pgRemoteSettings struct {
	listen          string // running listen_addresses
	ssl             bool
	hbaFile         string
	preload         string // running shared_preload_libraries
	pkgLibDir       string
	pendingRestart  []string
	listenFile      string // file of the last listen_addresses line ("" = default)
	listenFileValue string
	preloadFile     string // last configured shared_preload_libraries; "<none>" when no file sets it
	alterSystem     bool
}

const pgRemoteInspectSQL = `SELECT concat_ws(chr(31),
 current_setting('listen_addresses'),
 current_setting('ssl'),
 current_setting('hba_file'),
 current_setting('shared_preload_libraries'),
 coalesce((SELECT setting FROM pg_config WHERE name = 'PKGLIBDIR'), ''),
 coalesce((SELECT string_agg(name, ',' ORDER BY name) FROM pg_settings WHERE pending_restart), ''),
 coalesce((SELECT sourcefile || chr(30) || setting FROM pg_file_settings WHERE name = 'listen_addresses' ORDER BY seqno DESC LIMIT 1), ''),
 coalesce((SELECT setting FROM pg_file_settings WHERE name = 'shared_preload_libraries' ORDER BY seqno DESC LIMIT 1), '<none>'),
 coalesce(current_setting('allow_alter_system', true), 'on'));`

func parsePGRemoteSettings(out string) (pgRemoteSettings, error) {
	line := strings.TrimSpace(out)
	fields := strings.Split(line, pgRemoteFieldSep)
	if len(fields) != 9 {
		return pgRemoteSettings{}, fmt.Errorf("PostgreSQL answered %d values, CelikPanel asked for 9", len(fields))
	}
	s := pgRemoteSettings{
		listen:      fields[0],
		ssl:         fields[1] == "on",
		hbaFile:     fields[2],
		preload:     fields[3],
		pkgLibDir:   fields[4],
		preloadFile: fields[7],
		alterSystem: fields[8] != "off",
	}
	if fields[5] != "" {
		s.pendingRestart = strings.Split(fields[5], ",")
	}
	if fields[6] != "" {
		file, value, _ := strings.Cut(fields[6], pgRemotePairSep)
		s.listenFile, s.listenFileValue = file, value
	}
	return s, nil
}

// pgListenIsLocal: every address in a listen_addresses value is this server
// itself. An empty value (no TCP at all) is the owner's choice, not local.
func pgListenIsLocal(value string) bool {
	value = strings.TrimSpace(strings.Trim(strings.TrimSpace(value), "'\""))
	if value == "" {
		return false
	}
	for _, part := range strings.Split(value, ",") {
		switch strings.TrimSpace(part) {
		case "localhost", "127.0.0.1", "::1":
		default:
			return false
		}
	}
	return true
}

func (pgRemoteInstance) read(port int) (pgRemoteSettings, error) {
	out, err := databaseRemotePGQuery(port, pgRemoteInspectSQL)
	if err != nil {
		return pgRemoteSettings{}, err
	}
	return parsePGRemoteSettings(out)
}

func (p pgRemoteInstance) inspect(target dbremote.Target, ts dbremote.TargetState) dbRemoteFacts {
	s, err := p.read(target.Port)
	if err != nil {
		return dbRemoteFacts{readError: "PostgreSQL on port " + fmt.Sprint(target.Port) + " did not answer: " + boundedText(err.Error(), 200)}
	}
	facts := dbRemoteFacts{running: true, listenRemote: !pgListenIsLocal(s.listen)}
	if !ts.Listen {
		configured := "localhost"
		if s.listenFile != "" {
			configured = s.listenFileValue
		}
		if !pgListenIsLocal(configured) || (facts.listenRemote && !ts.ListenPending) {
			facts.ownerManaged = true
			where := "PostgreSQL's default"
			if s.listenFile != "" {
				where = s.listenFile
			}
			facts.ownerDetail = fmt.Sprintf("listen_addresses = '%s' (%s) was set outside CelikPanel", boundedText(configured, 120), where)
		}
	}
	return facts
}

// pgRemoteRoles checks every rule's user and database on the cluster and
// answers the rules that may be written and a problem for each that may not.
func pgRemoteRoles(port int, rules []dbremote.Rule) ([]dbremote.Rule, []transport.DatabaseRemoteProblem, error) {
	if len(rules) == 0 {
		return nil, nil, nil
	}
	users, databases := map[string]bool{}, map[string]bool{}
	for _, r := range rules {
		users[r.User], databases[r.Database] = true, true
	}
	quoteList := func(set map[string]bool) string {
		list := make([]string, 0, len(set))
		for name := range set {
			list = append(list, "'"+name+"'") // names are checked identifiers
		}
		sort.Strings(list)
		return strings.Join(list, ",")
	}
	out, err := databaseRemotePGQuery(port, `SELECT concat_ws(chr(31), rolname, (rolsuper OR rolreplication OR rolbypassrls)::text, coalesce(rolpassword LIKE 'SCRAM-SHA-256$%', false)::text, rolcanlogin::text) FROM pg_authid WHERE rolname IN (`+quoteList(users)+`) UNION ALL SELECT 'db' || chr(31) || datname FROM pg_database WHERE datname IN (`+quoteList(databases)+`);`)
	if err != nil {
		return nil, nil, err
	}
	type role struct{ privileged, scram, login bool }
	roles, dbs := map[string]role{}, map[string]bool{}
	for _, row := range pgRows(out) {
		f := strings.Split(row, pgRemoteFieldSep)
		if len(f) == 2 && f[0] == "db" {
			dbs[f[1]] = true
		} else if len(f) == 4 {
			roles[f[0]] = role{privileged: f[1] == "true", scram: f[2] == "true", login: f[3] == "true"}
		}
	}
	var ok []dbremote.Rule
	var problems []transport.DatabaseRemoteProblem
	for _, r := range rules {
		role, found := roles[r.User]
		switch {
		case !found || !role.login:
			problems = append(problems, ruleProblem(transport.DatabaseRemoteUserMissing, r, "the user "+r.User+" does not exist on this PostgreSQL or cannot sign in"))
		case role.privileged:
			problems = append(problems, ruleProblem(transport.DatabaseRemotePrivilegedUser, r, "the user "+r.User+" has server-wide rights and is never reachable from outside"))
		case !role.scram:
			problems = append(problems, ruleProblem(transport.DatabaseRemotePasswordMD5, r, "the password of "+r.User+" is stored in the old md5 form; set a new password for this user once"))
		case !dbs[r.Database]:
			problems = append(problems, ruleProblem(transport.DatabaseRemoteNoAccess, r, "the database "+r.Database+" does not exist on this PostgreSQL"))
		default:
			ok = append(ok, r)
		}
	}
	return ok, problems, nil
}

// pgRemoteWriteHBA puts the lines into the cluster's pg_hba.conf through the
// validated writer.
func pgRemoteWriteHBA(path string, lines []string) error {
	if target, ok := dbConfigTargetFor(path); !ok || target.kind != dbConfigHBA {
		return fmt.Errorf("pg_hba.conf is at %s, a place CelikPanel does not edit", path)
	}
	target, _ := dbConfigTargetFor(path)
	current, metadata, err := readDNSFileForSnapshot(path)
	if err != nil {
		return err
	}
	next, err := dbremote.SpliceHBA(string(current), lines)
	if err != nil {
		return err
	}
	if next == string(current) {
		return nil
	}
	reload := databaseRemoteHBAReload
	if reload == nil {
		return errors.New("no PostgreSQL reload is available")
	}
	// The same validated path as the configuration editor's write of this
	// file (updateManagedConfig): line check, lockout check, backup, atomic
	// replace, PostgreSQL asked, reload, put back on refusal. The caller
	// holds the editor's lock, so the file read is the file replaced.
	pre := dnsFileSnapshot{
		Path: path, Exists: true, Mode: uint32(metadata.Mode.Perm()),
		OwnerKnown: metadata.OwnerKnown, UID: metadata.UID, GID: metadata.GID,
		SHA256: digestDNSBytes(current), Data: current,
	}
	_, err = applyDatabaseConfigUpdate(target, pre, []byte(next), reload)
	return err
}

func pgAlterSystem(port int, statements ...string) error {
	_, err := databaseRemotePGQuery(port, strings.Join(statements, "\n")+"\nSELECT pg_reload_conf();")
	return err
}

// databaseRemoteWriteHBA puts the managed block into pg_hba.conf (swapped
// by tests).
var databaseRemoteWriteHBA = pgRemoteWriteHBA

// databaseRemoteReadHBA reads the cluster's pg_hba.conf (swapped by tests).
var databaseRemoteReadHBA = secureReadConfig

func (p pgRemoteInstance) apply(target dbremote.Target, facts dbRemoteFacts, ts *dbremote.TargetState, hostName string) transport.DatabaseRemoteTargetResult {
	result := transport.DatabaseRemoteTargetResult{Engine: target.Engine, Port: target.Port, Unit: dbRemoteUnit(target), TLS: "off", SlowDown: "none"}
	problem := func(code, detail string) {
		result.Problems = append(result.Problems, transport.DatabaseRemoteProblem{Code: code, Detail: detail})
	}
	fail := func(code, detail string) transport.DatabaseRemoteTargetResult {
		result.State = transport.DatabaseRemoteStateFailed
		problem(code, detail)
		return result
	}
	s, err := p.read(target.Port)
	if err != nil {
		return fail(transport.DatabaseRemoteEngineDown, err.Error())
	}
	candidates, problems, err := pgRemoteRoles(target.Port, target.Rules)
	if err != nil {
		return fail(transport.DatabaseRemoteFailed, "the users could not be checked: "+boundedText(err.Error(), 200))
	}
	result.Problems = append(result.Problems, problems...)

	// 1. The settings, TLS first (alpha.129 review L4): a hostssl line is
	// written only once the cluster runs TLS, so the validated writer never
	// sees a block PostgreSQL would refuse.
	var statements []string
	if len(candidates) > 0 {
		if !s.alterSystem {
			problem(transport.DatabaseRemoteAlterSystemOff, "allow_alter_system is off on this cluster, so CelikPanel cannot set its listen address or TLS")
		} else {
			if !s.ssl && !ts.TLS {
				cert, key, err := ensureDatabaseRemoteCertificate("postgresql", "postgres", hostName)
				if err != nil {
					problem(transport.DatabaseRemoteTLSMissing, err.Error())
				} else {
					statements = append(statements,
						"ALTER SYSTEM SET ssl_cert_file = '"+cert+"';",
						"ALTER SYSTEM SET ssl_key_file = '"+key+"';",
						"ALTER SYSTEM SET ssl = 'on';")
					ts.TLS = true
				}
			}
			if !ts.Listen {
				statements = append(statements, "ALTER SYSTEM SET listen_addresses = '*';")
				ts.Listen, ts.ListenPending = true, false
			}
			if !ts.AuthDelay && strings.TrimSpace(s.preload) == "" && (s.preloadFile == "<none>" || strings.Trim(s.preloadFile, "' ") == "") &&
				s.pkgLibDir != "" && fileIsRegular(filepath.Join(s.pkgLibDir, "auth_delay.so")) {
				statements = append(statements,
					"ALTER SYSTEM SET shared_preload_libraries = 'auth_delay';",
					"ALTER SYSTEM SET auth_delay.milliseconds = '"+pgAuthDelayMilliseconds+"';")
				ts.AuthDelay = true
			}
		}
	} else if ts.Listen {
		statements = append(statements, "ALTER SYSTEM RESET listen_addresses;")
		ts.Listen = false
		ts.ListenPending = true
	}
	if len(statements) > 0 {
		if err := pgAlterSystem(target.Port, statements...); err != nil {
			// Nothing of these statements is assumed: read what took.
			problem(transport.DatabaseRemoteFailed, "PostgreSQL did not accept a setting: "+boundedText(err.Error(), 200))
		}
		time.Sleep(databaseRemotePGReloadWait) // pg_reload_conf only signals the server
	}
	after, err := p.read(target.Port)
	if err != nil {
		return fail(transport.DatabaseRemoteEngineDown, err.Error())
	}
	if ts.TLS && !after.ssl && !containsString(after.pendingRestart, "ssl") {
		// TLS did not come on: put the settings back, say so.
		_ = pgAlterSystem(target.Port, "ALTER SYSTEM RESET ssl;", "ALTER SYSTEM RESET ssl_cert_file;", "ALTER SYSTEM RESET ssl_key_file;")
		ts.TLS = false
		problem(transport.DatabaseRemoteTLSMissing, "PostgreSQL did not turn TLS on with the certificate CelikPanel made; its log says why")
	}
	if ts.Listen && after.listenFile != "" && !strings.HasSuffix(after.listenFile, "postgresql.auto.conf") {
		problem(transport.DatabaseRemoteOwnerListen, "listen_addresses is set again in "+after.listenFile+", after CelikPanel's setting")
	}

	// 2. The rules in force: only with TLS on.
	active := candidates
	if !after.ssl {
		active = nil
	}
	lines, err := dbremote.HBALines(dbremote.Target{Engine: target.Engine, Port: target.Port, Rules: active})
	if err != nil {
		return fail(transport.DatabaseRemoteFailed, err.Error())
	}
	if err := databaseRemoteWriteHBA(s.hbaFile, lines); err != nil {
		code := transport.DatabaseRemoteFailed
		if errors.Is(err, dbremote.ErrHBABlockEdited) {
			code = transport.DatabaseRemoteOwnerFile
		}
		return fail(code, "pg_hba.conf: "+boundedText(err.Error(), 300))
	}
	result.Active = toTransportRules(active)

	listenRemote := !pgListenIsLocal(after.listen)
	if !listenRemote && ts.ListenPending {
		ts.ListenPending = false
	}
	result.ListenRemote = listenRemote
	switch {
	case after.ssl:
		result.TLS = "on"
	case ts.TLS:
		result.TLS = "pending_restart"
	}
	switch {
	case pgPreloadHasAuthDelay(after.preload):
		result.SlowDown = "auth_delay"
	case ts.AuthDelay:
		result.SlowDown = "auth_delay_pending_restart"
	}
	if len(candidates) > 0 && result.SlowDown == "none" {
		detail := "auth_delay is not available in this PostgreSQL"
		if strings.TrimSpace(s.preload) != "" || (s.preloadFile != "<none>" && strings.Trim(s.preloadFile, "' ") != "") {
			detail = "shared_preload_libraries is set by the server's owner; add auth_delay to it to slow down wrong passwords"
		}
		problem(transport.DatabaseRemoteNoSlowDown, detail)
	}
	switch {
	case len(candidates) == 0 && ts.ListenPending:
		result.State = transport.DatabaseRemoteStateClosing
		problem(transport.DatabaseRemoteRestartNeeded, "PostgreSQL still listens beyond this server until it restarts; the port stays closed by the address filter meanwhile")
	case len(candidates) == 0:
		result.State = transport.DatabaseRemoteStateOff
	case ts.Listen && (!listenRemote || len(after.pendingRestart) > 0):
		result.State = transport.DatabaseRemoteStateWaitRestart
		problem(transport.DatabaseRemoteRestartNeeded, "PostgreSQL reads its listen address"+pendingRestartDetail(after.pendingRestart)+" only when it starts")
	case len(active) > 0 && listenRemote:
		result.State = transport.DatabaseRemoteStateOpen
	default:
		result.State = transport.DatabaseRemoteStateWaitRestart
	}
	return result
}

// databaseRemotePGReloadWait: pg_reload_conf only signals the server.
var databaseRemotePGReloadWait = 300 * time.Millisecond

// pgPreloadHasAuthDelay: auth_delay is among the loaded libraries.
func pgPreloadHasAuthDelay(preload string) bool {
	for _, part := range strings.Split(preload, ",") {
		if strings.Trim(strings.TrimSpace(part), "'\"") == "auth_delay" {
			return true
		}
	}
	return false
}

func pendingRestartDetail(names []string) string {
	others := []string{}
	for _, n := range names {
		if n != "listen_addresses" {
			others = append(others, n)
		}
	}
	if len(others) == 0 {
		return ""
	}
	return " and " + strings.Join(others, ", ")
}

func fileIsRegular(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// pgClusterForPort names a Debian cluster ("17-main") by its port.
func pgClusterForPort(port int) (string, bool) {
	host := pgClusterHostActive()
	if host.family == nil || host.family() != "apt" || host.lsclusters == nil {
		return "", false
	}
	ctx, cancel := context.WithTimeout(context.Background(), pgReadBound)
	defer cancel()
	out, err := host.lsclusters(ctx)
	if err != nil {
		return "", false
	}
	for _, c := range parsePGLsClusters(out) {
		if c.Port == port {
			return c.Version + "-" + c.Name, true
		}
	}
	return "", false
}
