package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Moving a database to another PostgreSQL cluster (alpha.108). A move is a
// copy with proof, never a conversion: pg_dump (custom format) from the
// source cluster's port into a file only root can read, the roles the
// database needs created on the target from the source's own definitions
// (pg_dumpall --roles-only: their attributes and password hashes, carried
// on standard input and never printed), pg_restore --create into the target,
// then every ordinary table's rows counted on both sides. The source is not
// changed. A copy that fails or whose counts differ is removed from the
// target again, with the roles it created. Switching the panel's record to
// the copy and dropping the source are the administrator's own later steps
// (the Panel; DropPostgreSQLDatabaseV1 here).
//
// Bir veritabanını başka bir PostgreSQL kümesine taşımak bir kopyadır: döküm,
// roller, geri yükleme ve her tablonun satır sayısı iki tarafta; kaynak
// değişmez.

const (
	pgCopyBound      = 18 * time.Minute
	pgCopyTablesCap  = 200
	pgRolesDumpLimit = 16 << 20
	pgSpaceMargin    = 64 << 20
)

var (
	pgDumpDir    = "/var/lib/celikpanel-agent-private/pg-moves"
	pgRoleNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_$.@-]{0,62}$`)
)

func pgDatabaseTarget(name string) string { return "pgdb-" + strings.ToLower(name) }

func pgCopyQualifier(source, target int) string {
	return strconv.Itoa(source) + "-" + strconv.Itoa(target)
}

const pgTableCountsStatement = `SELECT n.nspname || chr(31) || c.relname || chr(31) || (xpath('/row/c/text()', query_to_xml(format('SELECT count(*) AS c FROM %I.%I', n.nspname, c.relname), false, true, '')))[1]::text FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace WHERE c.relkind = 'r' AND n.nspname NOT IN ('pg_catalog', 'information_schema') AND n.nspname NOT LIKE 'pg\_toast%' AND n.nspname NOT LIKE 'pg\_temp%' ORDER BY n.nspname, c.relname;`

// pgTableCounts is every ordinary table of a database and its rows.
func pgTableCounts(ctx context.Context, host pgClusterHost, owner string, port int, database string) (map[string]int64, []string, error) {
	out, err := host.query(ctx, owner, port, database, pgTableCountsStatement)
	if err != nil {
		return nil, nil, err
	}
	counts := map[string]int64{}
	var order []string
	for _, row := range pgRows(out) {
		fields := strings.Split(row, "\x1f")
		if len(fields) != 3 {
			return nil, nil, fmt.Errorf("a table's row count could not be read")
		}
		count, err := strconv.ParseInt(strings.TrimSpace(fields[2]), 10, 64)
		if err != nil {
			return nil, nil, fmt.Errorf("a table's row count could not be read")
		}
		key := fields[0] + "\x1f" + fields[1]
		counts[key] = count
		order = append(order, key)
	}
	return counts, order, nil
}

// pgWritesStatement reads one database's write counters (alpha.108 review).
func pgWritesStatement(database string) string {
	literal, _ := services.QuotePGStringLiteral(database)
	return "SELECT tup_inserted || chr(31) || tup_updated || chr(31) || tup_deleted || chr(31) || coalesce(stats_reset::text, '') FROM pg_stat_database WHERE datname = " + literal + ";"
}

// pgReadWrites reads the write counters of database on the cluster.
func pgReadWrites(ctx context.Context, host pgClusterHost, owner string, port int, database string) (transport.PostgreSQLWriteMark, error) {
	out, err := host.query(ctx, owner, port, "postgres", pgWritesStatement(database))
	if err != nil {
		return transport.PostgreSQLWriteMark{}, err
	}
	rows := pgRows(out)
	if len(rows) != 1 {
		return transport.PostgreSQLWriteMark{}, fmt.Errorf("the database's write counters could not be read")
	}
	fields := strings.Split(rows[0], "\x1f")
	if len(fields) != 4 {
		return transport.PostgreSQLWriteMark{}, fmt.Errorf("the database's write counters could not be read")
	}
	var mark transport.PostgreSQLWriteMark
	values := []*int64{&mark.Inserted, &mark.Updated, &mark.Deleted}
	for i, target := range values {
		value, err := strconv.ParseInt(strings.TrimSpace(fields[i]), 10, 64)
		if err != nil {
			return transport.PostgreSQLWriteMark{}, fmt.Errorf("the database's write counters could not be read")
		}
		*target = value
	}
	mark.StatsReset = strings.TrimSpace(fields[3])
	return mark, nil
}

// pgSourceUnchanged says whether a move's source is still as it was when the
// copy was proved: the same write counters and, for every table the proof
// kept, the same rows (alpha.108 review). The copy proves rows at copy time
// only; anything written to the source after it would be lost with a switch
// or a drop. Unknown is never "unchanged".
func pgSourceUnchanged(ctx context.Context, host pgClusterHost, cluster transport.PostgreSQLCluster, database string, expect transport.PostgreSQLSourceExpectation) (code, message string) {
	ref := pgClusterRef(cluster.Version, cluster.Name)
	writes, err := pgReadWrites(ctx, host, cluster.Owner, cluster.Port, database)
	if err != nil {
		return "SOURCE_UNKNOWN", fmt.Sprintf("whether %s on cluster %s was written after the copy could not be read (%s); nothing was changed. Try again", database, ref, boundedText(err.Error(), 160))
	}
	if writes.StatsReset != expect.Writes.StatsReset {
		return "SOURCE_UNKNOWN", fmt.Sprintf("the statistics of cluster %s were reset since the copy, so whether %s was written after it cannot be proved; copy it again; nothing was changed", ref, database)
	}
	changed := writes != expect.Writes
	if !changed {
		counts, _, err := pgTableCounts(ctx, host, cluster.Owner, cluster.Port, database)
		if err != nil {
			return "SOURCE_UNKNOWN", fmt.Sprintf("the rows of %s on cluster %s could not be counted again (%s); nothing was changed. Try again", database, ref, boundedText(err.Error(), 160))
		}
		if expect.TablesTotal != len(counts) {
			changed = true
		}
		for _, table := range expect.Tables {
			if count, ok := counts[table.Schema+"\x1f"+table.Table]; !ok || count != table.Source {
				changed = true
			}
		}
	}
	if changed {
		return "SOURCE_CHANGED", fmt.Sprintf("%s on cluster %s was written after the copy, so the copy no longer has everything; nothing was changed. Copy it again, or point the application at the copy first and then copy again", database, ref)
	}
	return "", ""
}

// PostgreSQLSourceCheckV1 answers, read only, whether a move's source is
// still as it was at the copy (the Panel asks before a switch).
func (a *Agent) PostgreSQLSourceCheckV1(req *transport.PostgreSQLSourceCheckRequest, resp *transport.PostgreSQLSourceCheckResponse) error {
	*resp = transport.PostgreSQLSourceCheckResponse{}
	if req == nil || !pgDatabaseNameRe.MatchString(req.Database) || req.Port < 1 || req.Port > 65535 {
		resp.Code, resp.Error = "INVALID_REQUEST", "a database and its cluster's port are required"
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	sourceCheck(ctx, pgClusterHostActive(), req, resp)
	return nil
}

func sourceCheck(ctx context.Context, host pgClusterHost, req *transport.PostgreSQLSourceCheckRequest, resp *transport.PostgreSQLSourceCheckResponse) {
	out, err := host.lsclusters(ctx)
	if err != nil {
		resp.Code, resp.Error = "SOURCE_UNKNOWN", "pg_lsclusters could not be read; nothing was changed"
		return
	}
	cluster, ok := findPGClusterByPort(parsePGLsClusters(out), req.Port)
	if !ok || !cluster.Online {
		resp.Code, resp.Error = "SOURCE_UNKNOWN", fmt.Sprintf("no running PostgreSQL cluster on port %d, so the source cannot be checked; nothing was changed", req.Port)
		return
	}
	resp.Code, resp.Error = pgSourceUnchanged(ctx, host, cluster, req.Database, req.Expect)
	resp.Unchanged = resp.Code == ""
}

// pgCompareCounts is the proof: the same tables on both sides, every count
// equal.
func pgCompareCounts(source map[string]int64, target map[string]int64) transport.PostgreSQLCopyProof {
	proof := transport.PostgreSQLCopyProof{Tables: []transport.PostgreSQLTableCount{}, RolesCreated: []string{}, RolesKept: []string{}}
	keys := map[string]bool{}
	for key := range source {
		keys[key] = true
	}
	for key := range target {
		keys[key] = true
	}
	ordered := make([]string, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)
	proof.TablesTotal = len(ordered)
	var mismatched, matched []transport.PostgreSQLTableCount
	for _, key := range ordered {
		schema, table, _ := strings.Cut(key, "\x1f")
		src, inSource := source[key]
		dst, inTarget := target[key]
		entry := transport.PostgreSQLTableCount{Schema: schema, Table: table, Source: src, Target: dst}
		if !inSource {
			entry.Source = -1
		}
		if !inTarget {
			entry.Target = -1
		}
		if !inSource || !inTarget || src != dst {
			proof.Mismatches++
			mismatched = append(mismatched, entry)
		} else {
			matched = append(matched, entry)
		}
	}
	// Mismatches are shown first, so a capped list never hides one.
	for _, entry := range append(mismatched, matched...) {
		if len(proof.Tables) == pgCopyTablesCap {
			break
		}
		proof.Tables = append(proof.Tables, entry)
	}
	proof.Equal = proof.Mismatches == 0
	return proof
}

// pgIdent reads one SQL identifier at the start of s (bare lower-case, or
// double-quoted with "" for a quote) and returns it and the rest.
func pgIdent(s string) (string, string, bool) {
	if strings.HasPrefix(s, `"`) {
		var b strings.Builder
		for i := 1; i < len(s); i++ {
			if s[i] == '"' {
				if i+1 < len(s) && s[i+1] == '"' {
					b.WriteByte('"')
					i++
					continue
				}
				return b.String(), s[i+1:], true
			}
			b.WriteByte(s[i])
		}
		return "", "", false
	}
	end := 0
	for end < len(s) {
		c := s[end]
		if c == '_' || c == '$' || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			end++
			continue
		}
		break
	}
	if end == 0 || (s[0] >= '0' && s[0] <= '9') {
		return "", "", false
	}
	return s[:end], s[end:], true
}

// pgRoleScript picks, from pg_dumpall --roles-only, the statements that
// create the missing roles as the source defines them (CREATE ROLE, ALTER
// ROLE … WITH, ALTER ROLE … SET) and their memberships, in one transaction.
// Only a line that reads completely as one of those shapes for one of those
// roles is taken.
func pgRoleScript(dump string, missing map[string]bool) string {
	var b strings.Builder
	b.WriteString("BEGIN;\n")
	for _, raw := range strings.Split(dump, "\n") {
		line := strings.TrimRight(raw, "\r")
		if !strings.HasSuffix(line, ";") {
			continue
		}
		switch {
		case strings.HasPrefix(line, "CREATE ROLE "):
			name, rest, ok := pgIdent(strings.TrimPrefix(line, "CREATE ROLE "))
			if ok && rest == ";" && missing[name] {
				b.WriteString(line + "\n")
			}
		case strings.HasPrefix(line, "ALTER ROLE "):
			name, rest, ok := pgIdent(strings.TrimPrefix(line, "ALTER ROLE "))
			if ok && missing[name] && (strings.HasPrefix(rest, " WITH ") || strings.HasPrefix(rest, " SET ")) {
				b.WriteString(line + "\n")
			}
		case strings.HasPrefix(line, "GRANT "):
			granted, rest, ok := pgIdent(strings.TrimPrefix(line, "GRANT "))
			if !ok || !strings.HasPrefix(rest, " TO ") {
				continue
			}
			member, rest, ok := pgIdent(strings.TrimPrefix(rest, " TO "))
			if !ok || !missing[member] || granted == "" {
				continue
			}
			if i := strings.Index(rest, " GRANTED BY "); i >= 0 {
				rest = rest[:i] + ";"
			}
			b.WriteString("GRANT " + quotePGIdent(granted) + " TO " + quotePGIdent(member) + rest + "\n")
		}
	}
	b.WriteString("COMMIT;\n")
	return b.String()
}

func quotePGIdent(name string) string { return `"` + strings.ReplaceAll(name, `"`, `""`) + `"` }

const pgNeededRolesStatement = `SELECT r.rolname || chr(31) || CASE WHEN r.rolsuper THEN 's' ELSE '-' END FROM pg_roles r WHERE r.rolname NOT LIKE 'pg\_%' AND r.rolname <> 'postgres' AND r.oid IN (SELECT datdba FROM pg_database WHERE datname = current_database() UNION SELECT relowner FROM pg_class UNION SELECT nspowner FROM pg_namespace UNION SELECT proowner FROM pg_proc UNION SELECT typowner FROM pg_type UNION SELECT (aclexplode(relacl)).grantee FROM pg_class WHERE relacl IS NOT NULL UNION SELECT (aclexplode(nspacl)).grantee FROM pg_namespace WHERE nspacl IS NOT NULL UNION SELECT (aclexplode(datacl)).grantee FROM pg_database WHERE datname = current_database() AND datacl IS NOT NULL) ORDER BY 1;`

func findPGClusterByPort(clusters []transport.PostgreSQLCluster, port int) (transport.PostgreSQLCluster, bool) {
	for _, cluster := range clusters {
		if cluster.Port == port {
			return cluster, true
		}
	}
	return transport.PostgreSQLCluster{}, false
}

func pgDatabaseExists(ctx context.Context, host pgClusterHost, cluster transport.PostgreSQLCluster, name string) (bool, error) {
	literal, err := services.QuotePGStringLiteral(name)
	if err != nil {
		return false, err
	}
	out, err := host.query(ctx, cluster.Owner, cluster.Port, "postgres", "SELECT count(*) FROM pg_database WHERE datname = "+literal+";")
	if err != nil {
		return false, err
	}
	rows := pgRows(out)
	return len(rows) == 1 && strings.TrimSpace(rows[0]) == "1", nil
}

// CopyPostgreSQLDatabaseV1 copies one database to another cluster with
// proof, in its own pg_database_copy job.
func (a *Agent) CopyPostgreSQLDatabaseV1(req *transport.PostgreSQLCopyRequest, resp *transport.PostgreSQLCopyResponse) error {
	*resp = transport.PostgreSQLCopyResponse{Proof: emptyPGProof()}
	if req == nil || !pgDatabaseNameRe.MatchString(req.Database) || pgSystemDatabases[req.Database] ||
		req.SourcePort < 1 || req.SourcePort > 65535 || req.TargetPort < 1 || req.TargetPort > 65535 || req.SourcePort == req.TargetPort {
		resp.ErrorCode, resp.Error = "INVALID_REQUEST", "a database, its cluster's port and another cluster's port are required"
		return nil
	}
	ctx, finishStep, err := a.requiredServiceMutationStep(
		req.ServiceMutationBinding,
		newServiceMutationStepClaim(serviceMutationStepCopyPGDatabase, pgDatabaseTarget(req.Database), pgCopyQualifier(req.SourcePort, req.TargetPort), "copy"),
	)
	if err != nil {
		resp.ErrorCode, resp.Error = "MUTATION_REFUSED", err.Error()
		return nil
	}
	defer finishStep()
	copyPGDatabase(ctx, pgClusterHostActive(), req, resp)
	return nil
}

func emptyPGProof() transport.PostgreSQLCopyProof {
	return transport.PostgreSQLCopyProof{Tables: []transport.PostgreSQLTableCount{}, RolesCreated: []string{}, RolesKept: []string{}}
}

func copyPGDatabase(parent context.Context, host pgClusterHost, req *transport.PostgreSQLCopyRequest, resp *transport.PostgreSQLCopyResponse) {
	started := host.now()
	ctx, cancel := context.WithTimeout(parent, pgCopyBound)
	defer cancel()
	fail := func(code, format string, args ...any) {
		resp.ErrorCode, resp.Error = code, fmt.Sprintf(format, args...)
	}
	if host.family() != "apt" {
		fail("UNSUPPORTED_SYSTEM", "moving a database between PostgreSQL clusters needs Debian or Ubuntu's postgresql-common; nothing was changed")
		return
	}
	out, err := host.lsclusters(ctx)
	if err != nil {
		fail("CLUSTERS_UNREADABLE", "pg_lsclusters could not be read (%s); nothing was changed", boundedText(err.Error(), 160))
		return
	}
	clusters := parsePGLsClusters(out)
	source, okSource := findPGClusterByPort(clusters, req.SourcePort)
	target, okTarget := findPGClusterByPort(clusters, req.TargetPort)
	switch {
	case !okSource:
		fail("CLUSTER_NOT_FOUND", "no PostgreSQL cluster runs on port %d; nothing was changed", req.SourcePort)
		return
	case !okTarget:
		fail("CLUSTER_NOT_FOUND", "no PostgreSQL cluster runs on port %d; nothing was changed", req.TargetPort)
		return
	case !source.Online || !target.Online:
		fail("CLUSTER_DOWN", "both clusters must be running (%s is %s, %s is %s); start them and try again; nothing was changed",
			pgClusterRef(source.Version, source.Name), source.Status, pgClusterRef(target.Version, target.Name), target.Status)
		return
	case !pgOwnerRe.MatchString(source.Owner) || !pgOwnerRe.MatchString(target.Owner):
		fail("CLUSTER_OWNER", "a cluster's owner is not an account CelikPanel runs as; nothing was changed")
		return
	}
	resp.Proof.SourceVersion, resp.Proof.TargetVersion = source.Version, target.Version
	exists, err := pgDatabaseExists(ctx, host, source, req.Database)
	if err != nil || !exists {
		fail("SOURCE_MISSING", "database %s is not on cluster %s (port %d); nothing was changed", req.Database, pgClusterRef(source.Version, source.Name), source.Port)
		return
	}
	if exists, err := pgDatabaseExists(ctx, host, target, req.Database); err != nil {
		fail("TARGET_UNREADABLE", "cluster %s did not answer (%s); nothing was changed", pgClusterRef(target.Version, target.Name), boundedText(err.Error(), 160))
		return
	} else if exists {
		fail("TARGET_EXISTS", "cluster %s already has a database named %s; CelikPanel never copies over a database. Remove or rename that one first, or choose another cluster; nothing was changed", pgClusterRef(target.Version, target.Name), req.Database)
		return
	}
	literal, _ := services.QuotePGStringLiteral(req.Database)
	if out, err := host.query(ctx, source.Owner, source.Port, "postgres", "SELECT pg_get_userbyid(datdba) || chr(31) || pg_database_size(datname) FROM pg_database WHERE datname = "+literal+";"); err == nil {
		if rows := pgRows(out); len(rows) == 1 {
			ownerName, size, _ := strings.Cut(rows[0], "\x1f")
			resp.Proof.Owner = ownerName
			resp.Proof.SizeBytes, _ = strconv.ParseInt(strings.TrimSpace(size), 10, 64)
		}
	}
	if err := os.MkdirAll(pgDumpDir, 0o700); err != nil {
		fail("DUMP_DIRECTORY", "the copy's working directory %s could not be made (%s); nothing was changed", pgDumpDir, boundedText(err.Error(), 120))
		return
	}
	if free, err := host.freeBytes(pgDumpDir); err == nil && free < resp.Proof.SizeBytes+pgSpaceMargin {
		fail("NO_SPACE", "the copy needs about %d MiB on the disk of %s and %d MiB are free; free space first; nothing was changed", (resp.Proof.SizeBytes+pgSpaceMargin)>>20, pgDumpDir, free>>20)
		return
	}
	sourceBefore, _, err := pgTableCounts(ctx, host, source.Owner, source.Port, req.Database)
	if err != nil {
		fail("COUNT_FAILED", "the rows of %s could not be counted on the source (%s); nothing was changed", req.Database, boundedText(err.Error(), 160))
		return
	}
	_ = sourceBefore
	// The write counters before the dump; read again after the proof, they
	// must be the same (alpha.108 review).
	writesBefore, err := pgReadWrites(ctx, host, source.Owner, source.Port, req.Database)
	if err != nil {
		fail("COUNT_FAILED", "the write counters of %s could not be read on the source (%s); nothing was changed", req.Database, boundedText(err.Error(), 160))
		return
	}

	// Roles: those the database needs, created on the target only when missing.
	out, err = host.query(ctx, source.Owner, source.Port, req.Database, pgNeededRolesStatement)
	if err != nil {
		fail("ROLES_UNREADABLE", "the roles %s needs could not be read (%s); nothing was changed", req.Database, boundedText(err.Error(), 160))
		return
	}
	needed := map[string]bool{}
	for _, row := range pgRows(out) {
		name, flag, _ := strings.Cut(row, "\x1f")
		if !pgRoleNameRe.MatchString(name) {
			fail("ROLE_NAME", "role %q has a name CelikPanel does not carry; create it on cluster %s yourself and try again; nothing was changed", boundedText(name, 64), pgClusterRef(target.Version, target.Name))
			return
		}
		needed[name] = flag == "s"
	}
	out, err = host.query(ctx, target.Owner, target.Port, "postgres", "SELECT rolname FROM pg_roles ORDER BY 1;")
	if err != nil {
		fail("TARGET_UNREADABLE", "cluster %s did not answer (%s); nothing was changed", pgClusterRef(target.Version, target.Name), boundedText(err.Error(), 160))
		return
	}
	existing := map[string]bool{}
	for _, name := range pgRows(out) {
		existing[name] = true
	}
	missing := map[string]bool{}
	for name, super := range needed {
		if existing[name] {
			resp.Proof.RolesKept = append(resp.Proof.RolesKept, name)
			continue
		}
		if super {
			fail("ROLE_SUPERUSER", "role %s, which %s needs, is a superuser on the source; CelikPanel does not create superusers. Create it on cluster %s yourself and try again; nothing was changed", name, req.Database, pgClusterRef(target.Version, target.Name))
			return
		}
		missing[name] = true
	}
	sort.Strings(resp.Proof.RolesKept)
	var created []string
	cleanup := func(databaseCreated bool) {
		cleanCtx, cancel := context.WithTimeout(context.WithoutCancel(parent), 2*time.Minute)
		defer cancel()
		ok := true
		if databaseCreated {
			ident, _ := services.QuotePGIdentifier(req.Database)
			if _, err := host.query(cleanCtx, target.Owner, target.Port, "postgres", "DROP DATABASE IF EXISTS "+ident+";"); err != nil {
				ok = false
			}
		}
		for _, name := range created {
			if _, err := host.query(cleanCtx, target.Owner, target.Port, "postgres", "DROP ROLE IF EXISTS "+quotePGIdent(name)+";"); err != nil {
				ok = false
			}
		}
		resp.Cleaned = ok
	}
	if len(missing) > 0 {
		dump := &pgBuffer{limit: pgRolesDumpLimit}
		if err := host.stream(ctx, source.Owner, "pg_dumpall", nil, dump, "--port", strconv.Itoa(source.Port), "--roles-only", "--no-password"); err != nil {
			fail("ROLES_UNREADABLE", "the role definitions could not be read from the source; nothing was changed")
			return
		}
		// The script carries password hashes: PostgreSQL's own answer is not
		// returned, only that it refused.
		if _, err := host.query(ctx, target.Owner, target.Port, "postgres", pgRoleScript(dump.String(), missing)); err != nil {
			fail("ROLES_FAILED", "cluster %s refused to create the roles %s needs; nothing was copied", pgClusterRef(target.Version, target.Name), req.Database)
			return
		}
		for name := range missing {
			created = append(created, name)
		}
		sort.Strings(created)
	}

	file, err := os.CreateTemp(pgDumpDir, "copy-*.dump")
	if err != nil {
		cleanup(false)
		fail("DUMP_FAILED", "the copy's file could not be made (%s); nothing was copied", boundedText(err.Error(), 120))
		return
	}
	defer func() {
		file.Close()
		os.Remove(file.Name())
	}()
	if err := host.stream(ctx, source.Owner, "pg_dump", nil, file, "--port", strconv.Itoa(source.Port), "--format", "custom", "--no-password", "--dbname", req.Database); err != nil {
		cleanup(false)
		fail("DUMP_FAILED", "pg_dump of %s on cluster %s failed (%s); nothing was copied, the source is unchanged", req.Database, pgClusterRef(source.Version, source.Name), boundedText(err.Error(), 240))
		return
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		cleanup(false)
		fail("DUMP_FAILED", "the copy's file could not be read back; nothing was copied")
		return
	}
	if err := host.stream(ctx, target.Owner, "pg_restore", file, io.Discard, "--port", strconv.Itoa(target.Port), "--create", "--exit-on-error", "--no-password", "--dbname", "postgres"); err != nil {
		cleanup(true)
		fail("RESTORE_FAILED", "pg_restore into cluster %s stopped (%s); the partial copy was removed from the target and the source is unchanged", pgClusterRef(target.Version, target.Name), boundedText(err.Error(), 240))
		return
	}
	targetCounts, _, err := pgTableCounts(ctx, host, target.Owner, target.Port, req.Database)
	if err != nil {
		cleanup(true)
		fail("COUNT_FAILED", "the rows of the copy could not be counted (%s); the copy was removed from the target", boundedText(err.Error(), 160))
		return
	}
	sourceAfter, _, err := pgTableCounts(ctx, host, source.Owner, source.Port, req.Database)
	if err != nil {
		cleanup(true)
		fail("COUNT_FAILED", "the rows of the source could not be counted again (%s); the copy was removed from the target", boundedText(err.Error(), 160))
		return
	}
	proof := pgCompareCounts(sourceAfter, targetCounts)
	proof.SourceVersion, proof.TargetVersion = resp.Proof.SourceVersion, resp.Proof.TargetVersion
	proof.Owner, proof.SizeBytes = resp.Proof.Owner, resp.Proof.SizeBytes
	proof.RolesKept = resp.Proof.RolesKept
	proof.RolesCreated = append([]string{}, created...)
	proof.Seconds = int(host.now().Sub(started).Seconds())
	resp.Proof = proof
	if proof.Equal {
		writesAfter, err := pgReadWrites(ctx, host, source.Owner, source.Port, req.Database)
		if err != nil {
			cleanup(true)
			fail("COUNT_FAILED", "the write counters of the source could not be read again (%s); the copy was removed from the target", boundedText(err.Error(), 160))
			return
		}
		if writesAfter != writesBefore {
			cleanup(true)
			fail("PROOF_FAILED", "%s was written while it was copied, so the copy may miss rows; it was removed from the target and the source is unchanged. Stop what writes to it, then copy again", req.Database)
			return
		}
		resp.Proof.SourceWrites = &writesAfter
	}
	if !proof.Equal {
		cleanup(true)
		changed := ""
		if len(sourceBefore) != len(sourceAfter) || !pgCompareCounts(sourceBefore, sourceAfter).Equal {
			changed = " The source itself changed while it was copied: stop what writes to it, then copy again."
		}
		fail("PROOF_FAILED", "%d of %d tables do not have the same rows on both sides, so the copy was removed from the target; the source is unchanged.%s", proof.Mismatches, proof.TablesTotal, changed)
		return
	}
	resp.Copied = true
}

// DropPostgreSQLDatabaseV1 drops one database on one cluster, only while a
// copy of it exists on another (the administrator's explicit last step of a
// move), in its own pg_database_drop job. Roles stay; open sessions are not
// broken (the drop is refused while anything is connected).
func (a *Agent) DropPostgreSQLDatabaseV1(req *transport.PostgreSQLDropRequest, resp *transport.PostgreSQLDropResponse) error {
	*resp = transport.PostgreSQLDropResponse{}
	if req == nil || !pgDatabaseNameRe.MatchString(req.Database) || pgSystemDatabases[req.Database] ||
		req.Port < 1 || req.Port > 65535 || req.KeepPort < 1 || req.KeepPort > 65535 || req.Port == req.KeepPort {
		resp.ErrorCode, resp.Error = "INVALID_REQUEST", "a database, its cluster's port and the port of the cluster with its copy are required"
		return nil
	}
	ctx, finishStep, err := a.requiredServiceMutationStep(
		req.ServiceMutationBinding,
		newServiceMutationStepClaim(serviceMutationStepDropPGDatabase, pgDatabaseTarget(req.Database), strconv.Itoa(req.Port), "drop"),
	)
	if err != nil {
		resp.ErrorCode, resp.Error = "MUTATION_REFUSED", err.Error()
		return nil
	}
	defer finishStep()
	dropPGDatabase(ctx, pgClusterHostActive(), req, resp)
	return nil
}

func dropPGDatabase(parent context.Context, host pgClusterHost, req *transport.PostgreSQLDropRequest, resp *transport.PostgreSQLDropResponse) {
	ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
	defer cancel()
	out, err := host.lsclusters(ctx)
	if err != nil {
		resp.ErrorCode, resp.Error = "CLUSTERS_UNREADABLE", "pg_lsclusters could not be read; nothing was changed"
		return
	}
	clusters := parsePGLsClusters(out)
	cluster, ok := findPGClusterByPort(clusters, req.Port)
	keep, okKeep := findPGClusterByPort(clusters, req.KeepPort)
	if !ok || !okKeep || !cluster.Online || !keep.Online {
		resp.ErrorCode, resp.Error = "CLUSTER_DOWN", "both clusters must be running so the copy can be seen before the drop; nothing was changed"
		return
	}
	if copyThere, err := pgDatabaseExists(ctx, host, keep, req.Database); err != nil || !copyThere {
		resp.ErrorCode, resp.Error = "COPY_MISSING", fmt.Sprintf("cluster %s has no database %s, so the one on cluster %s is not dropped; nothing was changed", pgClusterRef(keep.Version, keep.Name), req.Database, pgClusterRef(cluster.Version, cluster.Name))
		return
	}
	if there, err := pgDatabaseExists(ctx, host, cluster, req.Database); err == nil && !there {
		resp.Dropped = true
		return
	}
	// The source of a move is dropped only while it is as it was at the copy
	// (alpha.108 review): writes after the copy would be lost with it.
	if req.ExpectSource != nil {
		if code, message := pgSourceUnchanged(ctx, host, cluster, req.Database, *req.ExpectSource); code != "" {
			resp.ErrorCode, resp.Error = code, message
			return
		}
	}
	ident, _ := services.QuotePGIdentifier(req.Database)
	if _, err := host.query(ctx, cluster.Owner, cluster.Port, "postgres", "DROP DATABASE "+ident+";"); err != nil {
		if strings.Contains(err.Error(), "being accessed by other users") {
			resp.ErrorCode, resp.Error = "DATABASE_IN_USE", fmt.Sprintf("something is still connected to %s on cluster %s; stop the application that uses it, or point it at the copy, and try again; nothing was changed", req.Database, pgClusterRef(cluster.Version, cluster.Name))
			return
		}
		resp.ErrorCode, resp.Error = "DROP_FAILED", fmt.Sprintf("cluster %s refused to drop %s (%s)", pgClusterRef(cluster.Version, cluster.Name), req.Database, boundedText(err.Error(), 200))
		return
	}
	if there, err := pgDatabaseExists(ctx, host, cluster, req.Database); err != nil || there {
		resp.ErrorCode, resp.Error = "DROP_UNPROVED", fmt.Sprintf("whether %s is gone from cluster %s could not be proved; look at the cluster list again", req.Database, pgClusterRef(cluster.Version, cluster.Name))
		return
	}
	resp.Dropped = true
}
