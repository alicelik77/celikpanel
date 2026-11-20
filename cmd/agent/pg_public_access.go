package main

import (
	"errors"
	"fmt"
	"sort"
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
// alpha.141'den önce açılan PostgreSQL veritabanlarını başka hesaplara kapatan
// okuma, kapatma ve geri alma; her biri veritabanının içinde tek işlemde.

// pgInsideRolesStatement lists, inside the current database, every role
// that owns something there or holds a right there (administrators and
// PostgreSQL's own roles left out; PUBLIC is not a role).
const pgInsideRolesStatement = `SELECT 'role=' OPERATOR(pg_catalog.||) r.rolname FROM pg_catalog.pg_roles r WHERE NOT r.rolsuper AND r.rolname NOT LIKE 'pg\_%' AND r.oid IN (` +
	pgInsideRoleOids + `) ORDER BY 1;`

const pgInsideRoleOids = `SELECT d.datdba FROM pg_catalog.pg_database d WHERE d.datname OPERATOR(pg_catalog.=) pg_catalog.current_database() ` +
	`UNION SELECT c.relowner FROM pg_catalog.pg_class c ` +
	`UNION SELECT n.nspowner FROM pg_catalog.pg_namespace n ` +
	`UNION SELECT p.proowner FROM pg_catalog.pg_proc p ` +
	`UNION SELECT t.typowner FROM pg_catalog.pg_type t ` +
	`UNION SELECT l.lomowner FROM pg_catalog.pg_largeobject_metadata l ` +
	`UNION SELECT e.extowner FROM pg_catalog.pg_extension e ` +
	`UNION SELECT a.defaclrole FROM pg_catalog.pg_default_acl a ` +
	`UNION SELECT (pg_catalog.aclexplode(c.relacl)).grantee FROM pg_catalog.pg_class c WHERE c.relacl IS NOT NULL ` +
	`UNION SELECT (pg_catalog.aclexplode(n.nspacl)).grantee FROM pg_catalog.pg_namespace n WHERE n.nspacl IS NOT NULL ` +
	`UNION SELECT (pg_catalog.aclexplode(p.proacl)).grantee FROM pg_catalog.pg_proc p WHERE p.proacl IS NOT NULL ` +
	`UNION SELECT (pg_catalog.aclexplode(t.typacl)).grantee FROM pg_catalog.pg_type t WHERE t.typacl IS NOT NULL ` +
	`UNION SELECT (pg_catalog.aclexplode(l.lomacl)).grantee FROM pg_catalog.pg_largeobject_metadata l WHERE l.lomacl IS NOT NULL ` +
	`UNION SELECT (pg_catalog.aclexplode(a.defaclacl)).grantee FROM pg_catalog.pg_default_acl a ` +
	`UNION SELECT (pg_catalog.aclexplode(d.datacl)).grantee FROM pg_catalog.pg_database d WHERE d.datname OPERATOR(pg_catalog.=) pg_catalog.current_database() AND d.datacl IS NOT NULL`

// pgInsideRoleNames is pgInsideRoleOids as role names (for the checks).
const pgInsideRoleNames = `SELECT r.rolname::pg_catalog.text FROM pg_catalog.pg_roles r WHERE NOT r.rolsuper AND r.rolname NOT LIKE 'pg\_%' AND r.oid IN (` + pgInsideRoleOids + `)`

// pgSessionsNames: the roles (not administrators) connected to the current
// database, this session left out.
const pgSessionsNames = `SELECT r.rolname::pg_catalog.text FROM pg_catalog.pg_stat_activity s JOIN pg_catalog.pg_roles r ON r.oid OPERATOR(pg_catalog.=) s.usesysid ` +
	`WHERE s.datname OPERATOR(pg_catalog.=) pg_catalog.current_database() AND s.pid OPERATOR(pg_catalog.<>) pg_catalog.pg_backend_pid() AND NOT r.rolsuper`

const pgDatabaseFactsStatement = `SELECT 'owner=' OPERATOR(pg_catalog.||) pg_catalog.pg_get_userbyid(d.datdba)::pg_catalog.text, ` +
	`'acl=' OPERATOR(pg_catalog.||) coalesce(d.datacl::pg_catalog.text, ''), ` +
	`'public=' OPERATOR(pg_catalog.||) pg_catalog.has_database_privilege('public', d.oid, 'CONNECT')::pg_catalog.text OPERATOR(pg_catalog.||) ',' OPERATOR(pg_catalog.||) pg_catalog.has_database_privilege('public', d.oid, 'TEMPORARY')::pg_catalog.text ` +
	`FROM pg_catalog.pg_database d WHERE d.datname OPERATOR(pg_catalog.=) pg_catalog.current_database();`

const pgMonitoringStatement = `SELECT DISTINCT 'monitor=' OPERATOR(pg_catalog.||) m.rolname FROM pg_catalog.pg_auth_members am ` +
	`JOIN pg_catalog.pg_roles g ON g.oid OPERATOR(pg_catalog.=) am.roleid JOIN pg_catalog.pg_roles m ON m.oid OPERATOR(pg_catalog.=) am.member ` +
	`WHERE g.rolname OPERATOR(pg_catalog.=) ANY (ARRAY['pg_monitor','pg_read_all_stats','pg_read_all_data','pg_read_all_settings','pg_stat_scan_tables']) ` +
	`AND NOT m.rolsuper AND m.rolcanlogin ORDER BY 1;`

// pgPublicAccessRunner is the statement runner of one cluster (a seam).
var pgPublicAccessRunner = postgreSQLStatementFor

func pgPublicAccessFailure(err error) (string, *transport.DatabaseClusterRefusal) {
	return boundedText(err.Error(), 300), pgClusterRefusalOf(err)
}

// PostgreSQLPublicAccessScanV1 reads, without changing anything, the rights
// of the recorded databases of one cluster.
func (a *Agent) PostgreSQLPublicAccessScanV1(req *transport.PostgreSQLPublicAccessScanRequest, resp *transport.PostgreSQLPublicAccessScanResponse) error {
	if req == nil || resp == nil {
		return errors.New("request and response are required")
	}
	*resp = transport.PostgreSQLPublicAccessScanResponse{Monitoring: []string{}, Databases: []transport.PostgreSQLPublicAccessFacts{}}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "reading the PostgreSQL databases' rights"); err != nil {
		resp.Error, resp.BuildMismatch = err.Error(), true
		return nil
	}
	run, err := pgPublicAccessRunner(req.Port)
	if err != nil {
		resp.Error, resp.Refusal = pgPublicAccessFailure(err)
		return nil
	}
	out, err := run(pgMonitoringStatement)
	if err != nil {
		resp.Error = "the cluster could not be asked about its monitoring roles: " + boundedText(string(out), 200)
		return nil
	}
	resp.Monitoring = append(resp.Monitoring, mariadbgrant.Markers(out, "monitor")...)
	for _, database := range req.Databases {
		resp.Databases = append(resp.Databases, pgPublicAccessFactsOf(run, database.Name))
	}
	return nil
}

func pgPublicAccessFactsOf(run func(string) ([]byte, error), name string) transport.PostgreSQLPublicAccessFacts {
	facts := transport.PostgreSQLPublicAccessFacts{Name: name, Roles: []string{}, Sessions: []string{}}
	if services.ValidateSQLIdentifier(name) != nil {
		facts.Error = "not a database name CelikPanel manages"
		return facts
	}
	literal, _ := services.QuotePGStringLiteral(name)
	out, err := run("SELECT 'exists=' OPERATOR(pg_catalog.||) count(*) FROM pg_catalog.pg_database WHERE datname OPERATOR(pg_catalog.=) " + literal + ";")
	if err != nil {
		facts.Error = "the cluster did not answer: " + boundedText(string(out), 200)
		return facts
	}
	if exists, _ := markerValue(out, "exists"); exists != "1" {
		return facts
	}
	facts.Exists = true
	ident, _ := services.QuotePGIdentifier(name)
	out, err = run(`\connect ` + ident + "\n" + pgDatabaseFactsStatement + "\n" + pgInsideRolesStatement + "\n" +
		"SELECT DISTINCT 'session=' OPERATOR(pg_catalog.||) x.rolname FROM (" + pgSessionsNames + ") x(rolname) ORDER BY 1;")
	if err != nil {
		facts.Error = "the database could not be read: " + boundedText(string(out), 200)
		return facts
	}
	owner, okOwner := markerValue(out, "owner")
	acl, okACL := markerValue(out, "acl")
	public, okPublic := markerValue(out, "public")
	if !okOwner || !okACL || !okPublic {
		facts.Error = "the database's answer could not be read"
		return facts
	}
	facts.Owner, facts.ACL = owner, acl
	connect, temporary, _ := strings.Cut(public, ",")
	facts.PublicConnect, facts.PublicTemporary = connect == "true", temporary == "true"
	facts.Roles = append(facts.Roles, mariadbgrant.Markers(out, "role")...)
	facts.Sessions = append(facts.Sessions, mariadbgrant.Markers(out, "session")...)
	return facts
}

// pgNameArray is ARRAY['a','b']::text[] of validated role names.
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

// pgACLGuard is a DO block that stops the transaction unless the database's
// rights are still expect.
func pgACLGuard(expect string) (string, error) {
	// The untouched default is "" (QuotePGStringLiteral refuses an empty one).
	literal := "''"
	if expect != "" {
		quoted, err := services.QuotePGStringLiteral(expect)
		if err != nil {
			return "", err
		}
		literal = quoted
	}
	return "DO $celikpanel$ BEGIN IF (SELECT coalesce(d.datacl::pg_catalog.text, '') FROM pg_catalog.pg_database d WHERE d.datname OPERATOR(pg_catalog.=) pg_catalog.current_database()) " +
		"OPERATOR(pg_catalog.<>) " + literal + " THEN RAISE EXCEPTION 'celikpanel-changed'; END IF; END $celikpanel$;", nil
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
		if strings.Contains(string(out), "celikpanel-changed") {
			resp.Changed = true
			resp.Error = "the database is no longer as it was read; nothing was changed"
			return nil
		}
		// psql stopped inside the transaction: its connection ended, so
		// PostgreSQL rolled it back; or the COMMIT itself was not answered.
		resp.Error = "the database server did not close it: " + boundedText(string(out), 240)
		return nil
	}
	before, okBefore := markerValue(out, "before")
	after, okAfter := markerValue(out, "after")
	if !okBefore || !okAfter {
		resp.Error = "the database was changed, but its rights could not be read back"
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

func pgPublicCloseScript(req *transport.PostgreSQLPublicAccessCloseRequest) (string, error) {
	ident, err := services.QuotePGIdentifier(req.Name)
	if err != nil {
		return "", fmt.Errorf("not a database name CelikPanel manages")
	}
	guard, err := pgACLGuard(req.ExpectACL)
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
	lines := []string{`\connect ` + ident, "BEGIN;",
		"SELECT 'before=' OPERATOR(pg_catalog.||) coalesce(d.datacl::pg_catalog.text, '') FROM pg_catalog.pg_database d WHERE d.datname OPERATOR(pg_catalog.=) pg_catalog.current_database();",
		"SELECT 'had=' OPERATOR(pg_catalog.||) r.rolname OPERATOR(pg_catalog.||) ':' OPERATOR(pg_catalog.||) a.privilege_type FROM pg_catalog.pg_database d " +
			"CROSS JOIN LATERAL pg_catalog.aclexplode(d.datacl) a JOIN pg_catalog.pg_roles r ON r.oid OPERATOR(pg_catalog.=) a.grantee " +
			"WHERE d.datname OPERATOR(pg_catalog.=) pg_catalog.current_database() AND d.datacl IS NOT NULL;",
		guard,
		"DO $celikpanel$ BEGIN IF EXISTS (SELECT x FROM (" + pgInsideRoleNames + ") i(x) WHERE x OPERATOR(pg_catalog.<>) ALL (" + expected + ")) " +
			"OR EXISTS (SELECT 1 FROM pg_catalog.unnest(" + expected + ") e(x) WHERE x OPERATOR(pg_catalog.<>) ALL (ARRAY(" + pgInsideRoleNames + "))) " +
			"THEN RAISE EXCEPTION 'celikpanel-changed'; END IF; END $celikpanel$;",
	}
	if req.NoOtherSessions {
		lines = append(lines, "DO $celikpanel$ BEGIN IF EXISTS (SELECT x FROM ("+pgSessionsNames+") s(x) WHERE x OPERATOR(pg_catalog.<>) ALL ("+recorded+")) "+
			"THEN RAISE EXCEPTION 'celikpanel-changed'; END IF; END $celikpanel$;")
	}
	checks := []string{}
	for _, grant := range req.Grants {
		role, err := services.QuotePGIdentifier(grant.Role)
		if err != nil || !pgRoleNameRe.MatchString(grant.Role) {
			return "", fmt.Errorf("role %q has a name CelikPanel does not carry", boundedText(grant.Role, 64))
		}
		literal, _ := services.QuotePGStringLiteral(grant.Role)
		privileges := "CONNECT"
		if grant.Temporary {
			privileges = "CONNECT, TEMPORARY"
			checks = append(checks, "NOT pg_catalog.has_database_privilege("+literal+", pg_catalog.current_database(), 'TEMPORARY')")
		}
		checks = append(checks, "NOT pg_catalog.has_database_privilege("+literal+", pg_catalog.current_database(), 'CONNECT')")
		lines = append(lines, "GRANT "+privileges+" ON DATABASE "+ident+" TO "+role+";")
	}
	// Only what lets another account in: an undo gives back exactly these.
	lines = append(lines, "REVOKE CONNECT, TEMPORARY ON DATABASE "+ident+" FROM PUBLIC;")
	checks = append(checks,
		"pg_catalog.has_database_privilege('public', pg_catalog.current_database(), 'CONNECT')",
		"pg_catalog.has_database_privilege('public', pg_catalog.current_database(), 'TEMPORARY')",
		"NOT pg_catalog.has_database_privilege((SELECT d.datdba FROM pg_catalog.pg_database d WHERE d.datname OPERATOR(pg_catalog.=) pg_catalog.current_database()), pg_catalog.current_database(), 'CONNECT')")
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
		if strings.Contains(string(out), "celikpanel-changed") {
			resp.Changed = true
			resp.Error = "the database's rights were changed since it was closed; nothing was changed"
			return nil
		}
		resp.Error = "the database server did not open it again: " + boundedText(string(out), 240)
		return nil
	}
	after, ok := markerValue(out, "after")
	if !ok {
		resp.Error = "the database was opened again, but its rights could not be read back"
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
	guard, err := pgACLGuard(req.ExpectACL)
	if err != nil {
		return "", err
	}
	lines := []string{`\connect ` + ident, "BEGIN;", guard}
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
