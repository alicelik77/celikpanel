package main

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/alicelik/celikpanel/internal/mariadbgrant"
	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Closing PostgreSQL databases made before alpha.141 to other accounts
// (security lane S1b, 20 November 2026; transport/postgresql_public_access.go).
//
// Every statement here is CelikPanel's own, run by the engine's
// administrator with the fixed search path (pg_admin_session.go); nothing is
// taken from a file or a database. A close and an undo each run in one
// transaction inside the database: they change nothing unless the database
// is still as it was read, and they commit only after reading back the
// result. Interrupted anywhere, the database is either untouched or closed
// with its grants, never closed without them.
//
// alpha.143 fix round (independent review, 21 November 2026): a failure
// after the transaction began, or a missing read-back, is answered as an
// unknown result, never as "nothing changed" (D-024); role names and rights
// are passed to the guards as transaction settings, never inside a
// dollar-quoted body; the roles counted as inside a database also cover
// column rights, per-database role settings, policy roles, the remaining
// owners, foreign servers and user mappings, publications and
// subscriptions; a scan reads every database of a cluster in one session.
//
// alpha.141'den önce açılan PostgreSQL veritabanlarını başka hesaplara kapatan
// okuma, kapatma ve geri alma; her biri veritabanının içinde tek işlemde.
// Düzeltme turu: BEGIN'den sonraki hata "bilinmiyor" diye yanıtlanır; adlar
// koruma bloklarına ayar olarak geçer; tarama bir kümede tek oturumdur.

// pgInsideRolesStatement lists, inside the current database, every role
// that owns something there or holds a right there (administrators and
// PostgreSQL's own roles left out; PUBLIC is not a role).
const pgInsideRolesStatement = `SELECT 'role=' OPERATOR(pg_catalog.||) r.rolname FROM pg_catalog.pg_roles r WHERE NOT r.rolsuper AND r.rolname NOT LIKE 'pg\_%' AND r.oid IN (` +
	pgInsideRoleOids + `) ORDER BY 1;`

// pgInsideRoleOids is every role with a footprint in the current database:
// the database's owner and rights, the owners and rights of its objects
// (relations and their columns, schemas, functions, types, large objects,
// extensions, languages, operators and their classes and families,
// collations, conversions, text search objects, extended statistics,
// foreign data wrappers and servers, publications, subscriptions), default
// rights, user mappings, row security policy roles and per-database role
// settings. The cluster-wide catalogs (pg_database, pg_db_role_setting,
// pg_subscription) are narrowed to the current database.
const pgInsideRoleOids = `SELECT d.datdba FROM pg_catalog.pg_database d WHERE d.datname OPERATOR(pg_catalog.=) pg_catalog.current_database() ` +
	`UNION SELECT (pg_catalog.aclexplode(d.datacl)).grantee FROM pg_catalog.pg_database d WHERE d.datname OPERATOR(pg_catalog.=) pg_catalog.current_database() AND d.datacl IS NOT NULL ` +
	`UNION SELECT s.setrole FROM pg_catalog.pg_db_role_setting s JOIN pg_catalog.pg_database d ON d.oid OPERATOR(pg_catalog.=) s.setdatabase WHERE d.datname OPERATOR(pg_catalog.=) pg_catalog.current_database() ` +
	`UNION SELECT c.relowner FROM pg_catalog.pg_class c ` +
	`UNION SELECT (pg_catalog.aclexplode(c.relacl)).grantee FROM pg_catalog.pg_class c WHERE c.relacl IS NOT NULL ` +
	`UNION SELECT (pg_catalog.aclexplode(a.attacl)).grantee FROM pg_catalog.pg_attribute a WHERE a.attacl IS NOT NULL ` +
	`UNION SELECT n.nspowner FROM pg_catalog.pg_namespace n ` +
	`UNION SELECT (pg_catalog.aclexplode(n.nspacl)).grantee FROM pg_catalog.pg_namespace n WHERE n.nspacl IS NOT NULL ` +
	`UNION SELECT p.proowner FROM pg_catalog.pg_proc p ` +
	`UNION SELECT (pg_catalog.aclexplode(p.proacl)).grantee FROM pg_catalog.pg_proc p WHERE p.proacl IS NOT NULL ` +
	`UNION SELECT t.typowner FROM pg_catalog.pg_type t ` +
	`UNION SELECT (pg_catalog.aclexplode(t.typacl)).grantee FROM pg_catalog.pg_type t WHERE t.typacl IS NOT NULL ` +
	`UNION SELECT l.lomowner FROM pg_catalog.pg_largeobject_metadata l ` +
	`UNION SELECT (pg_catalog.aclexplode(l.lomacl)).grantee FROM pg_catalog.pg_largeobject_metadata l WHERE l.lomacl IS NOT NULL ` +
	`UNION SELECT e.extowner FROM pg_catalog.pg_extension e ` +
	`UNION SELECT g.lanowner FROM pg_catalog.pg_language g ` +
	`UNION SELECT (pg_catalog.aclexplode(g.lanacl)).grantee FROM pg_catalog.pg_language g WHERE g.lanacl IS NOT NULL ` +
	`UNION SELECT o.oprowner FROM pg_catalog.pg_operator o ` +
	`UNION SELECT oc.opcowner FROM pg_catalog.pg_opclass oc ` +
	`UNION SELECT f.opfowner FROM pg_catalog.pg_opfamily f ` +
	`UNION SELECT co.collowner FROM pg_catalog.pg_collation co ` +
	`UNION SELECT cv.conowner FROM pg_catalog.pg_conversion cv ` +
	`UNION SELECT tc.cfgowner FROM pg_catalog.pg_ts_config tc ` +
	`UNION SELECT td.dictowner FROM pg_catalog.pg_ts_dict td ` +
	`UNION SELECT x.stxowner FROM pg_catalog.pg_statistic_ext x ` +
	`UNION SELECT w.fdwowner FROM pg_catalog.pg_foreign_data_wrapper w ` +
	`UNION SELECT (pg_catalog.aclexplode(w.fdwacl)).grantee FROM pg_catalog.pg_foreign_data_wrapper w WHERE w.fdwacl IS NOT NULL ` +
	`UNION SELECT fs.srvowner FROM pg_catalog.pg_foreign_server fs ` +
	`UNION SELECT (pg_catalog.aclexplode(fs.srvacl)).grantee FROM pg_catalog.pg_foreign_server fs WHERE fs.srvacl IS NOT NULL ` +
	`UNION SELECT um.umuser FROM pg_catalog.pg_user_mapping um ` +
	`UNION SELECT pb.pubowner FROM pg_catalog.pg_publication pb ` +
	`UNION SELECT sb.subowner FROM pg_catalog.pg_subscription sb JOIN pg_catalog.pg_database d ON d.oid OPERATOR(pg_catalog.=) sb.subdbid WHERE d.datname OPERATOR(pg_catalog.=) pg_catalog.current_database() ` +
	`UNION SELECT pg_catalog.unnest(po.polroles) FROM pg_catalog.pg_policy po ` +
	`UNION SELECT a.defaclrole FROM pg_catalog.pg_default_acl a ` +
	`UNION SELECT (pg_catalog.aclexplode(a.defaclacl)).grantee FROM pg_catalog.pg_default_acl a`

// pgInsideRoleNames is pgInsideRoleOids as role names (for the checks).
const pgInsideRoleNames = `SELECT r.rolname::pg_catalog.text FROM pg_catalog.pg_roles r WHERE NOT r.rolsuper AND r.rolname NOT LIKE 'pg\_%' AND r.oid IN (` + pgInsideRoleOids + `)`

// pgSessionsNames: the roles (not administrators) connected to the current
// database, this session left out.
const pgSessionsNames = `SELECT r.rolname::pg_catalog.text FROM pg_catalog.pg_stat_activity s JOIN pg_catalog.pg_roles r ON r.oid OPERATOR(pg_catalog.=) s.usesysid ` +
	`WHERE s.datname OPERATOR(pg_catalog.=) pg_catalog.current_database() AND s.pid OPERATOR(pg_catalog.<>) pg_catalog.pg_backend_pid() AND NOT r.rolsuper`

// pgDatabaseFactsStatement answers one marker per row (psql -At joins the
// columns of one row with "|").
const pgDatabaseFactsStatement = `SELECT x FROM pg_catalog.pg_database d CROSS JOIN LATERAL (VALUES ` +
	`(1, 'owner=' OPERATOR(pg_catalog.||) pg_catalog.pg_get_userbyid(d.datdba)::pg_catalog.text), ` +
	`(2, 'acl=' OPERATOR(pg_catalog.||) coalesce(d.datacl::pg_catalog.text, '')), ` +
	`(3, 'public=' OPERATOR(pg_catalog.||) pg_catalog.has_database_privilege('public', d.oid, 'CONNECT')::pg_catalog.text OPERATOR(pg_catalog.||) ',' OPERATOR(pg_catalog.||) pg_catalog.has_database_privilege('public', d.oid, 'TEMPORARY')::pg_catalog.text)` +
	`) v(n, x) WHERE d.datname OPERATOR(pg_catalog.=) pg_catalog.current_database() ORDER BY v.n;`

const pgMonitoringStatement = `SELECT DISTINCT 'monitor=' OPERATOR(pg_catalog.||) m.rolname FROM pg_catalog.pg_auth_members am ` +
	`JOIN pg_catalog.pg_roles g ON g.oid OPERATOR(pg_catalog.=) am.roleid JOIN pg_catalog.pg_roles m ON m.oid OPERATOR(pg_catalog.=) am.member ` +
	`WHERE g.rolname OPERATOR(pg_catalog.=) ANY (ARRAY['pg_monitor','pg_read_all_stats','pg_read_all_data','pg_read_all_settings','pg_stat_scan_tables']) ` +
	`AND NOT m.rolsuper AND m.rolcanlogin ORDER BY 1;`

// pgReplicationStatement: roles that may open a replication connection
// (CelikPanel makes none; an administrator has the right anyway).
const pgReplicationStatement = `SELECT 'replication=' OPERATOR(pg_catalog.||) r.rolname FROM pg_catalog.pg_roles r WHERE r.rolreplication AND NOT r.rolsuper ORDER BY 1;`

// pgScanDatabaseMark separates the databases of one scan session's output.
const pgScanDatabaseMark = "celikpanel-database="

// pgPublicAccessRunner is the statement runner of one cluster (a seam).
var pgPublicAccessRunner = postgreSQLStatementFor

func pgPublicAccessFailure(err error) (string, *transport.DatabaseClusterRefusal) {
	return boundedText(err.Error(), 300), pgClusterRefusalOf(err)
}

// PostgreSQLPublicAccessScanV1 reads, without changing anything, the rights
// of the recorded databases of one cluster: one session for the cluster
// (\connect to each database in turn); a database that cannot be entered
// ends that session, and the next session starts after it.
func (a *Agent) PostgreSQLPublicAccessScanV1(req *transport.PostgreSQLPublicAccessScanRequest, resp *transport.PostgreSQLPublicAccessScanResponse) error {
	if req == nil || resp == nil {
		return errors.New("request and response are required")
	}
	*resp = transport.PostgreSQLPublicAccessScanResponse{Monitoring: []string{}, Replication: []string{}, Databases: []transport.PostgreSQLPublicAccessFacts{}}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "reading the PostgreSQL databases' rights"); err != nil {
		resp.Error, resp.BuildMismatch = err.Error(), true
		return nil
	}
	run, err := pgPublicAccessRunner(req.Port)
	if err != nil {
		resp.Error, resp.Refusal = pgPublicAccessFailure(err)
		return nil
	}
	// The cluster's facts first, then every database in the same session.
	header := []string{pgMonitoringStatement, pgReplicationStatement}
	recorded := []string{}
	facts := make([]transport.PostgreSQLPublicAccessFacts, len(req.Databases))
	var todo []int
	for i, database := range req.Databases {
		facts[i] = transport.PostgreSQLPublicAccessFacts{Name: database.Name, Roles: []string{}, Sessions: []string{}}
		if services.ValidateSQLIdentifier(database.Name) != nil {
			facts[i].Error = "not a database name CelikPanel manages"
			continue
		}
		todo = append(todo, i)
		for _, role := range database.Recorded {
			if literal, err := services.QuotePGStringLiteral(role); err == nil {
				recorded = append(recorded, literal)
			}
		}
	}
	if len(recorded) > 0 {
		header = append(header, "SELECT 'known=' OPERATOR(pg_catalog.||) r.rolname FROM pg_catalog.pg_roles r WHERE r.rolname OPERATOR(pg_catalog.=) ANY (ARRAY["+strings.Join(recorded, ",")+"]::pg_catalog.text[]) ORDER BY 1;")
	}
	header = append(header, `\echo celikpanel-header-done`)
	out, ok := pgPublicScanDatabases(run, header, todo, facts)
	if !ok {
		resp.Error = "the cluster could not be asked about its roles and databases: " + boundedText(string(out), 200)
		return nil
	}
	resp.Monitoring = append(resp.Monitoring, mariadbgrant.Markers(out, "monitor")...)
	resp.Replication = append(resp.Replication, mariadbgrant.Markers(out, "replication")...)
	known := map[string]bool{}
	for _, name := range mariadbgrant.Markers(out, "known") {
		known[name] = true
	}
	for i, database := range req.Databases {
		if !facts[i].Exists {
			continue
		}
		for _, role := range database.Recorded {
			if !known[role] {
				facts[i].Missing = append(facts[i].Missing, role)
			}
		}
	}
	resp.Databases = append(resp.Databases, facts...)
	return nil
}

// pgPublicScanDatabases runs header and reads each database in todo, in as
// few sessions as it can: one, unless a database cannot be entered (it is
// answered with the reason and the next session starts after it). It
// returns the header's output, and false when the header was not answered.
func pgPublicScanDatabases(run func(string) ([]byte, error), header []string, todo []int, facts []transport.PostgreSQLPublicAccessFacts) ([]byte, bool) {
	var headerOut []byte
	first := true
	for first || len(todo) > 0 {
		var script []string
		if first {
			script = append(script, header...)
		}
		for _, i := range todo {
			ident, _ := services.QuotePGIdentifier(facts[i].Name)
			literal, _ := services.QuotePGStringLiteral(facts[i].Name)
			// psql's own \if: a database that is gone is answered, not entered.
			script = append(script, `\echo `+pgScanDatabaseMark+strconv.Itoa(i),
				"SELECT EXISTS (SELECT 1 FROM pg_catalog.pg_database d WHERE d.datname OPERATOR(pg_catalog.=) "+literal+") AS celikpanel_exists \\gset",
				`\if :celikpanel_exists`, `\connect `+ident, pgDatabaseFactsStatement, pgInsideRolesStatement,
				"SELECT DISTINCT 'session=' OPERATOR(pg_catalog.||) x.rolname FROM ("+pgSessionsNames+") x(rolname) ORDER BY 1;",
				`\else`, `\echo absent=1`, `\endif`)
		}
		script = append(script, `\echo `+pgScanDatabaseMark+"end")
		out, err := run(strings.Join(script, "\n"))
		if first {
			first = false
			if !strings.Contains(string(out), "celikpanel-header-done") {
				return out, false
			}
			headerOut = out
		}
		segments, last := pgScanSegments(out)
		next := []int{}
		for at, i := range todo {
			segment, seen := segments[i]
			switch {
			case !seen:
				// Not reached: the session ended before it.
				if err == nil {
					facts[i].Error = "the database's answer could not be read"
					continue
				}
				next = todo[at:]
			case i == last && (err != nil || !strings.Contains(string(out), pgScanDatabaseMark+"end")):
				facts[i].Exists = true
				facts[i].Error = "the database could not be read: " + boundedText(string(segment), 200)
				next = todo[at+1:]
			default:
				if _, absent := markerValue(segment, "absent"); !absent {
					facts[i].Exists = true
					pgPublicFactsFrom(segment, &facts[i])
				}
				continue
			}
			break
		}
		if len(next) == len(todo) {
			// Nothing was read in this session: no further session.
			for _, i := range next {
				facts[i].Exists = true
				facts[i].Error = "the cluster did not answer: " + boundedText(string(out), 200)
			}
			return headerOut, true
		}
		todo = next
	}
	return headerOut, true
}

// pgScanSegments splits one scan session's output at its database marks:
// each database's lines, and the index of the last database it reached.
func pgScanSegments(out []byte) (map[int][]byte, int) {
	segments := map[int][]byte{}
	current, last := -1, -1
	for _, line := range strings.SplitAfter(string(out), "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), pgScanDatabaseMark); ok {
			if i, err := strconv.Atoi(rest); err == nil {
				current, last = i, i
				segments[i] = []byte{}
				continue
			}
			current = -1
			continue
		}
		if current >= 0 {
			segments[current] = append(segments[current], line...)
		}
	}
	return segments, last
}

func pgPublicFactsFrom(out []byte, facts *transport.PostgreSQLPublicAccessFacts) {
	owner, okOwner := markerValue(out, "owner")
	acl, okACL := markerValue(out, "acl")
	public, okPublic := markerValue(out, "public")
	if !okOwner || !okACL || !okPublic {
		facts.Error = "the database's answer could not be read"
		return
	}
	facts.Owner, facts.ACL = owner, acl
	connect, temporary, _ := strings.Cut(public, ",")
	facts.PublicConnect, facts.PublicTemporary = connect == "true", temporary == "true"
	facts.Roles = append(facts.Roles, mariadbgrant.Markers(out, "role")...)
	facts.Sessions = append(facts.Sessions, mariadbgrant.Markers(out, "session")...)
}

// pgNameArray is ARRAY['a','b']::text[] of validated role names. It is only
// ever placed outside a dollar-quoted body (pgPublicSetting).
func pgNameArray(names []string) (string, error) {
	quoted := make([]string, 0, len(names))
	for _, name := range names {
		if !pgRoleNameRe.MatchString(name) {
			return "", fmt.Errorf("role %q has a name CelikPanel does not carry", boundedText(name, 64))
		}
		literal, err := services.QuotePGStringLiteral(name)
		if err != nil {
			return "", err
		}
		quoted = append(quoted, literal)
	}
	return "ARRAY[" + strings.Join(quoted, ",") + "]::pg_catalog.text[]", nil
}

// pgPublicSetting keeps a value for the rest of the transaction as the
// setting celikpanel.<name>. Names and rights reach the guards this way,
// never inside a dollar-quoted body, so no name can end one.
func pgPublicSetting(name, expression string) string {
	return "SELECT 'set=' OPERATOR(pg_catalog.||) pg_catalog.length(pg_catalog.set_config('celikpanel." + name + "', (" + expression + ")::pg_catalog.text, true));"
}

// pgPublicSettingArray reads a setting kept from pgNameArray.
func pgPublicSettingArray(name string) string {
	return "pg_catalog.current_setting('celikpanel." + name + "')::pg_catalog.text[]"
}

// pgACLLiteral is the rights as a literal ("" is the untouched default;
// QuotePGStringLiteral refuses an empty one).
func pgACLLiteral(acl string) (string, error) {
	if acl == "" {
		return "''", nil
	}
	return services.QuotePGStringLiteral(acl)
}

// pgACLGuard is a DO block that stops the transaction unless the database's
// rights are still the setting celikpanel.expect_acl.
const pgACLGuard = "DO $celikpanel$ BEGIN IF (SELECT coalesce(d.datacl::pg_catalog.text, '') FROM pg_catalog.pg_database d WHERE d.datname OPERATOR(pg_catalog.=) pg_catalog.current_database()) " +
	"OPERATOR(pg_catalog.<>) pg_catalog.current_setting('celikpanel.expect_acl') THEN RAISE EXCEPTION 'celikpanel-changed'; END IF; END $celikpanel$;"

// pgPublicOutcome sorts a failed close or undo script's output: the guard
// refused (verified: nothing changed), the read-back refused (verified: the
// transaction was rolled back), the database could not be entered (verified:
// psql stops at its first line, before BEGIN), or anything else (unknown:
// it may have stopped after BEGIN, even after COMMIT).
func pgPublicOutcome(out []byte) (changed, verified bool) {
	text := string(out)
	switch {
	case strings.Contains(text, "celikpanel-changed"):
		return true, true
	case strings.Contains(text, "celikpanel-readback"):
		return false, true
	case strings.Contains(text, `\connect:`) && !strings.Contains(text, "began=1"):
		return false, true
	}
	return false, false
}

// PostgreSQLPublicAccessCloseV1 closes one database to other accounts (see
// the request's comment).
func (a *Agent) PostgreSQLPublicAccessCloseV1(req *transport.PostgreSQLPublicAccessCloseRequest, resp *transport.PostgreSQLPublicAccessCloseResponse) error {
	if req == nil || resp == nil {
		return errors.New("request and response are required")
	}
	*resp = transport.PostgreSQLPublicAccessCloseResponse{}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "closing a PostgreSQL database to other accounts"); err != nil {
		resp.Error, resp.BuildMismatch = err.Error(), true
		return nil
	}
	script, err := pgPublicCloseScript(req)
	if err != nil {
		resp.Error = err.Error() + "; nothing was changed"
		return nil
	}
	run, err := pgPublicAccessRunner(req.Port)
	if err != nil {
		resp.Error, resp.Refusal = pgPublicAccessFailure(err)
		return nil
	}
	out, err := run(script)
	if err != nil {
		changed, verified := pgPublicOutcome(out)
		switch {
		case changed:
			resp.Changed = true
			resp.Error = "the database is no longer as it was read; nothing was changed"
		case verified:
			resp.Error = "the database server did not close it; nothing was changed: " + boundedText(pgPublicErrorLines(out), 240)
		default:
			// Past BEGIN: rolled back, or committed with its answer lost.
			resp.Unknown = true
			resp.Error = "the database server's answer ended inside the close, so whether it was closed is not known: " + boundedText(pgPublicErrorLines(out), 240)
		}
		return nil
	}
	before, okBefore := markerValue(out, "before")
	after, okAfter := markerValue(out, "after")
	if !okBefore || !okAfter {
		resp.Unknown = true
		resp.Error = "the close's result could not be read back, so whether it was closed is not known"
		return nil
	}
	had := map[string]bool{}
	for _, entry := range mariadbgrant.Markers(out, "had") {
		had[entry] = true
	}
	change := &transport.PostgreSQLPublicAccessChange{BeforeACL: before, AfterACL: after, Added: []string{}}
	for _, grant := range req.Grants {
		privileges := []string{"CONNECT"}
		if grant.Temporary {
			privileges = append(privileges, "TEMPORARY")
		}
		for _, privilege := range privileges {
			if entry := grant.Role + ":" + privilege; !had[entry] {
				change.Added = append(change.Added, entry)
			}
		}
	}
	sort.Strings(change.Added)
	resp.Closed, resp.Change = true, change
	return nil
}

// pgPublicErrorLines is the server's and the client's error lines of an
// output (the markers left out).
func pgPublicErrorLines(out []byte) string {
	var lines []string
	for _, line := range strings.Split(string(out), "\n") {
		if line = strings.TrimSpace(line); strings.Contains(line, "ERROR") || strings.Contains(line, "FATAL") || strings.Contains(line, "error:") {
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return strings.TrimSpace(string(out))
	}
	return strings.Join(lines, " ")
}

func pgPublicCloseScript(req *transport.PostgreSQLPublicAccessCloseRequest) (string, error) {
	ident, err := services.QuotePGIdentifier(req.Name)
	if err != nil {
		return "", fmt.Errorf("not a database name CelikPanel manages")
	}
	acl, err := pgACLLiteral(req.ExpectACL)
	if err != nil {
		return "", err
	}
	expected, err := pgNameArray(req.ExpectRoles)
	if err != nil {
		return "", err
	}
	recorded, err := pgNameArray(req.Recorded)
	if err != nil {
		return "", err
	}
	var connect, temporary []string
	grants := []string{}
	for _, grant := range req.Grants {
		// The strict identifier check: only these names are written into a
		// statement, and only outside a dollar-quoted body.
		role, err := services.QuotePGIdentifier(grant.Role)
		if err != nil || !pgRoleNameRe.MatchString(grant.Role) {
			return "", fmt.Errorf("role %q has a name CelikPanel does not carry", boundedText(grant.Role, 64))
		}
		connect = append(connect, grant.Role)
		privileges := "CONNECT"
		if grant.Temporary {
			privileges = "CONNECT, TEMPORARY"
			temporary = append(temporary, grant.Role)
		}
		grants = append(grants, "GRANT "+privileges+" ON DATABASE "+ident+" TO "+role+";")
	}
	connectArray, _ := pgNameArray(connect)
	temporaryArray, _ := pgNameArray(temporary)
	lines := []string{`\connect ` + ident, "BEGIN;", "SELECT 'began=1';",
		pgPublicSetting("expect_acl", acl), pgPublicSetting("expect_roles", expected), pgPublicSetting("recorded", recorded),
		pgPublicSetting("connect", connectArray), pgPublicSetting("temporary", temporaryArray),
		"SELECT 'before=' OPERATOR(pg_catalog.||) coalesce(d.datacl::pg_catalog.text, '') FROM pg_catalog.pg_database d WHERE d.datname OPERATOR(pg_catalog.=) pg_catalog.current_database();",
		"SELECT 'had=' OPERATOR(pg_catalog.||) r.rolname OPERATOR(pg_catalog.||) ':' OPERATOR(pg_catalog.||) a.privilege_type FROM pg_catalog.pg_database d " +
			"CROSS JOIN LATERAL pg_catalog.aclexplode(d.datacl) a JOIN pg_catalog.pg_roles r ON r.oid OPERATOR(pg_catalog.=) a.grantee " +
			"WHERE d.datname OPERATOR(pg_catalog.=) pg_catalog.current_database() AND d.datacl IS NOT NULL;",
		pgACLGuard,
		"DO $celikpanel$ BEGIN IF EXISTS (SELECT x FROM (" + pgInsideRoleNames + ") i(x) WHERE x OPERATOR(pg_catalog.<>) ALL (" + pgPublicSettingArray("expect_roles") + ")) " +
			"OR EXISTS (SELECT 1 FROM pg_catalog.unnest(" + pgPublicSettingArray("expect_roles") + ") e(x) WHERE x OPERATOR(pg_catalog.<>) ALL (ARRAY(" + pgInsideRoleNames + "))) " +
			"THEN RAISE EXCEPTION 'celikpanel-changed'; END IF; END $celikpanel$;",
	}
	if req.NoOtherSessions {
		lines = append(lines, "DO $celikpanel$ BEGIN IF EXISTS (SELECT x FROM ("+pgSessionsNames+") s(x) WHERE x OPERATOR(pg_catalog.<>) ALL ("+pgPublicSettingArray("recorded")+")) "+
			"THEN RAISE EXCEPTION 'celikpanel-changed'; END IF; END $celikpanel$;")
	}
	lines = append(lines, grants...)
	// Only what lets another account in: an undo gives back exactly these.
	lines = append(lines, "REVOKE CONNECT, TEMPORARY ON DATABASE "+ident+" FROM PUBLIC;")
	checks := []string{
		"EXISTS (SELECT 1 FROM pg_catalog.unnest(" + pgPublicSettingArray("connect") + ") g(x) WHERE NOT pg_catalog.has_database_privilege(x, pg_catalog.current_database(), 'CONNECT'))",
		"EXISTS (SELECT 1 FROM pg_catalog.unnest(" + pgPublicSettingArray("temporary") + ") g(x) WHERE NOT pg_catalog.has_database_privilege(x, pg_catalog.current_database(), 'TEMPORARY'))",
		"pg_catalog.has_database_privilege('public', pg_catalog.current_database(), 'CONNECT')",
		"pg_catalog.has_database_privilege('public', pg_catalog.current_database(), 'TEMPORARY')",
		"NOT pg_catalog.has_database_privilege((SELECT d.datdba FROM pg_catalog.pg_database d WHERE d.datname OPERATOR(pg_catalog.=) pg_catalog.current_database()), pg_catalog.current_database(), 'CONNECT')",
	}
	lines = append(lines, "DO $celikpanel$ BEGIN IF "+strings.Join(checks, " OR ")+" THEN RAISE EXCEPTION 'celikpanel-readback'; END IF; END $celikpanel$;",
		"COMMIT;",
		"SELECT 'after=' OPERATOR(pg_catalog.||) coalesce(d.datacl::pg_catalog.text, '') FROM pg_catalog.pg_database d WHERE d.datname OPERATOR(pg_catalog.=) pg_catalog.current_database();")
	return strings.Join(lines, "\n"), nil
}

// PostgreSQLPublicAccessUndoV1 undoes one close (see the request's comment).
func (a *Agent) PostgreSQLPublicAccessUndoV1(req *transport.PostgreSQLPublicAccessUndoRequest, resp *transport.PostgreSQLPublicAccessUndoResponse) error {
	if req == nil || resp == nil {
		return errors.New("request and response are required")
	}
	*resp = transport.PostgreSQLPublicAccessUndoResponse{}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "opening a PostgreSQL database again"); err != nil {
		resp.Error, resp.BuildMismatch = err.Error(), true
		return nil
	}
	script, err := pgPublicUndoScript(req)
	if err != nil {
		resp.Error = err.Error() + "; nothing was changed"
		return nil
	}
	run, err := pgPublicAccessRunner(req.Port)
	if err != nil {
		resp.Error, resp.Refusal = pgPublicAccessFailure(err)
		return nil
	}
	out, err := run(script)
	if err != nil {
		changed, verified := pgPublicOutcome(out)
		switch {
		case changed:
			resp.Changed = true
			resp.Error = "the database's rights were changed since it was closed; nothing was changed"
		case verified:
			resp.Error = "the database server did not open it again; nothing was changed: " + boundedText(pgPublicErrorLines(out), 240)
		default:
			resp.Unknown = true
			resp.Error = "the database server's answer ended inside the undo, so whether it was opened again is not known: " + boundedText(pgPublicErrorLines(out), 240)
		}
		return nil
	}
	after, ok := markerValue(out, "after")
	if !ok {
		resp.Unknown = true
		resp.Error = "the undo's result could not be read back, so whether it was opened again is not known"
		return nil
	}
	resp.Undone, resp.AfterACL = true, after
	return nil
}

func pgPublicUndoScript(req *transport.PostgreSQLPublicAccessUndoRequest) (string, error) {
	ident, err := services.QuotePGIdentifier(req.Name)
	if err != nil {
		return "", fmt.Errorf("not a database name CelikPanel manages")
	}
	acl, err := pgACLLiteral(req.ExpectACL)
	if err != nil {
		return "", err
	}
	lines := []string{`\connect ` + ident, "BEGIN;", "SELECT 'began=1';", pgPublicSetting("expect_acl", acl), pgACLGuard}
	var public []string
	if req.PublicConnect {
		public = append(public, "CONNECT")
	}
	if req.PublicTemporary {
		public = append(public, "TEMPORARY")
	}
	if len(public) > 0 {
		lines = append(lines, "GRANT "+strings.Join(public, ", ")+" ON DATABASE "+ident+" TO PUBLIC;")
	}
	for _, entry := range req.Added {
		name, privilege, ok := strings.Cut(entry, ":")
		if !ok || (privilege != "CONNECT" && privilege != "TEMPORARY") || !pgRoleNameRe.MatchString(name) {
			return "", fmt.Errorf("the close's record cannot be read")
		}
		role, err := services.QuotePGIdentifier(name)
		if err != nil {
			return "", fmt.Errorf("role %q has a name CelikPanel does not carry", boundedText(name, 64))
		}
		lines = append(lines, "REVOKE "+privilege+" ON DATABASE "+ident+" FROM "+role+";")
	}
	expect := func(privilege string, want bool) string {
		negate := ""
		if want {
			negate = "NOT "
		}
		return negate + "pg_catalog.has_database_privilege('public', pg_catalog.current_database(), '" + privilege + "')"
	}
	lines = append(lines, "DO $celikpanel$ BEGIN IF "+expect("CONNECT", req.PublicConnect)+" OR "+expect("TEMPORARY", req.PublicTemporary)+
		" THEN RAISE EXCEPTION 'celikpanel-readback'; END IF; END $celikpanel$;",
		"COMMIT;",
		"SELECT 'after=' OPERATOR(pg_catalog.||) coalesce(d.datacl::pg_catalog.text, '') FROM pg_catalog.pg_database d WHERE d.datname OPERATOR(pg_catalog.=) pg_catalog.current_database();")
	return strings.Join(lines, "\n"), nil
}
