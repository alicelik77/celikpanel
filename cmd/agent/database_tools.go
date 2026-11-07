package main

import (
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/backupspec"
	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Database tools per database (alpha.128, G7). The Panel resolved and
// authorized every identity; the Agent validates them again and derives each
// path from the subscription and domain IDs (the domain's private backup
// folder, root-only, outside the site).
//
// - Size, tables, connections: read as the engine's administrator (reads).
// - Export: the backup's own dump (mariadb-dump/pg_dump of the database's
//   running cluster, alpha.127), gzip'd into a private file that the Panel
//   streams in chunks; the file is removed after its last byte, and any
//   export or upload older than six hours is swept.
// - Import: the file is uploaded in chunks into a private file. A safety
//   copy (an ordinary pre-restore backup, listed under Backups) is taken
//   first. The file is then loaded as the database's OWN user, never as the
//   administrator, because its contents are the customer's: MariaDB through
//   the client in --sandbox mode (no shell, no local files) running under the
//   site's own account; PostgreSQL over the protocol in one transaction
//   (database_import_pg.go). A MariaDB load that stops is put back from the
//   safety copy; a PostgreSQL one is rolled back by the server.
// - Copy: dump and restore into a database the Panel has just made on the
//   same engine and cluster (the WordPress-copy path).
// - Check/repair: mariadb-check; PostgreSQL has no repair, its check is
//   VACUUM ANALYZE.
//
// Veritabanı araçları: boyut, dışa aktarma, güvenlik kopyalı içe aktarma,
// kopya, denetim/onarım. İçe aktarılan dosya müşterinindir; yönetici olarak
// asla çalıştırılmaz, veritabanının kendi kullanıcısıyla yüklenir.

const (
	dbToolsMaxUploadBytes = int64(4) << 30
	dbToolsMaxChunkBytes  = 4 << 20
	dbToolsStaleAfter     = 6 * time.Hour
	dbToolsLoadTimeout    = 2 * time.Hour
)

var dbToolsMu sync.Mutex

var dbToolsIDRe = regexp.MustCompile(`^[0-9a-f]{32}$`)

func dbToolsScope(t transport.DBToolsTarget) backupScope {
	return backupScope{backupspec.ProtocolVersion, t.SubscriptionID, t.DomainID, t.Domain}
}

// dbToolsTarget validates a target and resolves a PostgreSQL database's
// running cluster (refused with its facts when it does not run).
func dbToolsTarget(t transport.DBToolsTarget) (backupScope, backupspec.DatabaseIdentity, error) {
	scope := dbToolsScope(t)
	if err := validateV2Scope(scope); err != nil {
		return scope, backupspec.DatabaseIdentity{}, err
	}
	database, err := validateDatabaseIdentity(t.Database)
	if err != nil {
		return scope, database, err
	}
	database, err = resolveBackupCluster(database)
	return scope, database, err
}

func dbToolsFile(scope backupScope, kind, id string) (string, error) {
	if !dbToolsIDRe.MatchString(id) {
		return "", errors.New("invalid file identity")
	}
	dir, err := ensureBackupDir(scope)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(kind + "\x00" + id))
	return filepath.Join(dir, ".db"+kind+"-"+hex.EncodeToString(sum[:12])), nil
}

// sweepDBToolsFiles removes exports and uploads nobody finished.
func sweepDBToolsFiles(scope backupScope) {
	dir, err := scopeBackupDir(scope)
	if err != nil {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, ".dbexport-") && !strings.HasPrefix(name, ".dbimport-") && !strings.HasPrefix(name, ".dbcopy-") {
			continue
		}
		info, err := entry.Info()
		if err != nil || time.Since(info.ModTime()) < dbToolsStaleAfter {
			continue
		}
		_ = os.RemoveAll(filepath.Join(dir, name))
	}
}

func dbToolsFailure(err error) (string, *transport.DatabaseClusterRefusal) {
	return boundedText(err.Error(), 400), pgClusterRefusalOf(err)
}

// ---- size, tables, connections ----

// dbToolsQueryTimeout bounds one size, tables or connections read.
const dbToolsQueryTimeout = 60 * time.Second

var dbToolsQuery = func(database backupspec.DatabaseIdentity, onDatabase bool, statement string) (string, error) {
	if database.Type == "postgresql" {
		args := []string{"-u", "postgres", "psql", "--no-psqlrc", "--set", "ON_ERROR_STOP=on", "--tuples-only", "--no-align"}
		if database.ClusterPort > 0 {
			args = append(args, "--port", strconv.Itoa(database.ClusterPort))
		}
		if onDatabase {
			args = append(args, "--dbname", database.Name)
		}
		// Integration (alpha.128): a deadline, as every other database read.
		ctx, cancel := context.WithTimeout(context.Background(), dbToolsQueryTimeout)
		defer cancel()
		cmd := exec.CommandContext(ctx, "sudo", args...)
		cmd.Stdin = strings.NewReader(statement + "\n")
		out, err := cmd.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("psql: %s", boundedText(string(out), 300))
		}
		return strings.TrimSpace(string(out)), nil
	}
	out, err := mysqlExecStatement(statement)
	if err != nil {
		return "", fmt.Errorf("mariadb: %s", boundedText(string(out), 300))
	}
	return strings.TrimSpace(string(out)), nil
}

func dbToolsInts(text string, n int) ([]int64, error) {
	fields := strings.FieldsFunc(text, func(r rune) bool { return r == '\t' || r == '|' || r == '\n' || r == ' ' })
	if len(fields) < n {
		return nil, fmt.Errorf("unexpected answer %q", boundedText(text, 80))
	}
	values := make([]int64, n)
	for i := 0; i < n; i++ {
		v, err := strconv.ParseInt(strings.TrimSpace(fields[i]), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("unexpected answer %q", boundedText(text, 80))
		}
		values[i] = v
	}
	return values, nil
}

// DatabaseToolsInfoV1 reads size, tables and connections.
func (a *Agent) DatabaseToolsInfoV1(req transport.DBToolsTarget, resp *transport.DBToolsInfoResponse) error {
	*resp = transport.DBToolsInfoResponse{}
	_, database, err := dbToolsTarget(req)
	if err != nil {
		resp.Error, resp.Refusal = dbToolsFailure(err)
		return nil
	}
	resp.ClusterVersion, resp.ClusterPort = database.ClusterVersion, database.ClusterPort
	if database.Type == "postgresql" {
		out, err := dbToolsQuery(database, false, fmt.Sprintf(
			"SELECT pg_database_size('%s'), (SELECT count(*) FROM pg_stat_activity WHERE datname = '%s');",
			database.Name, database.Name))
		if err == nil {
			var v []int64
			if v, err = dbToolsInts(out, 2); err == nil {
				resp.SizeBytes, resp.Connections = v[0], int(v[1])
				out, err = dbToolsQuery(database, true,
					"SELECT count(*) FROM pg_catalog.pg_tables WHERE schemaname NOT IN ('pg_catalog','information_schema');")
				if err == nil {
					if v, err = dbToolsInts(out, 1); err == nil {
						resp.Tables = int(v[0])
					}
				}
			}
		}
		if err != nil {
			resp.Error = boundedText(err.Error(), 300)
		}
		return nil
	}
	out, err := dbToolsQuery(database, false, fmt.Sprintf(
		"SELECT COALESCE(SUM(data_length + index_length), 0), COUNT(*) FROM information_schema.tables WHERE table_schema = '%s';"+
			" SELECT COUNT(*) FROM information_schema.processlist WHERE db = '%s';",
		database.Name, database.Name))
	if err == nil {
		var v []int64
		if v, err = dbToolsInts(out, 3); err == nil {
			resp.SizeBytes, resp.Tables, resp.Connections = v[0], int(v[1]), int(v[2])
		}
	}
	if err != nil {
		resp.Error = boundedText(err.Error(), 300)
	}
	if blocker := mariadbImportBlocker(); blocker != "" {
		resp.ImportBlocker = blocker
	}
	return nil
}

// ---- export ----

// DatabaseExportV1 makes the export file.
func (a *Agent) DatabaseExportV1(req transport.DBToolsExportRequest, resp *transport.DBToolsExportResponse) error {
	*resp = transport.DBToolsExportResponse{}
	scope, database, err := dbToolsTarget(req.Target)
	if err != nil {
		resp.Error, resp.Refusal = dbToolsFailure(err)
		return nil
	}
	path, err := dbToolsFile(scope, "export", req.ExportID)
	if err != nil {
		resp.Error = err.Error()
		return nil
	}
	sweepDBToolsFiles(scope)
	dbToolsMu.Lock()
	defer dbToolsMu.Unlock()
	if file, info, err := secureOpenRegular(path); err == nil {
		// The same export again (a replayed request): the file is there.
		_ = file.Close()
		resp.Size = info.Size()
		return nil
	}
	if err := dumpDatabaseToFile(database, path); err != nil {
		resp.Error, resp.Refusal = dbToolsFailure(err)
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		resp.Error = err.Error()
		return nil
	}
	resp.Size = info.Size()
	return nil
}

// DatabaseExportReadV1 reads one chunk; after the last one the file goes.
func (a *Agent) DatabaseExportReadV1(req transport.DBToolsReadExportRequest, resp *transport.DBToolsReadExportResponse) error {
	*resp = transport.DBToolsReadExportResponse{}
	scope := dbToolsScope(req.Target)
	if err := validateV2Scope(scope); err != nil {
		resp.Error = err.Error()
		return nil
	}
	path, err := dbToolsFile(scope, "export", req.ExportID)
	if err != nil {
		resp.Error = err.Error()
		return nil
	}
	maxBytes := req.MaxBytes
	if maxBytes <= 0 || maxBytes > backupspec.MaxChunkBytes {
		maxBytes = backupspec.MaxChunkBytes
	}
	file, info, err := secureOpenRegular(path)
	if err != nil {
		resp.Error = "the export file is no longer there; make a new export"
		return nil
	}
	defer file.Close()
	if req.Offset < 0 || req.Offset > info.Size() {
		resp.Error = "invalid offset"
		return nil
	}
	data := make([]byte, maxBytes)
	n, readErr := file.ReadAt(data, req.Offset)
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		resp.Error = readErr.Error()
		return nil
	}
	resp.Data, resp.Offset, resp.Size = data[:n], req.Offset+int64(n), info.Size()
	resp.EOF = resp.Offset >= info.Size()
	if resp.EOF {
		_ = os.Remove(path)
	}
	return nil
}

// ---- upload ----

// DatabaseImportUploadV1 appends one chunk of an import file. A chunk sent
// again after a lost answer is recognised and not written twice.
func (a *Agent) DatabaseImportUploadV1(req transport.DBToolsUploadChunkRequest, resp *transport.DBToolsUploadChunkResponse) error {
	*resp = transport.DBToolsUploadChunkResponse{}
	scope := dbToolsScope(req.Target)
	if err := validateV2Scope(scope); err != nil {
		resp.Error = err.Error()
		return nil
	}
	if len(req.Data) > dbToolsMaxChunkBytes || req.Offset < 0 {
		resp.Error = "invalid upload chunk"
		return nil
	}
	path, err := dbToolsFile(scope, "import", req.ImportID)
	if err != nil {
		resp.Error = err.Error()
		return nil
	}
	if req.Offset == 0 {
		sweepDBToolsFiles(scope)
	}
	dbToolsMu.Lock()
	defer dbToolsMu.Unlock()
	var current int64
	if st, err := os.Lstat(path); err == nil {
		if !st.Mode().IsRegular() {
			resp.Error = "upload file is not a regular file"
			return nil
		}
		current = st.Size()
	} else if !errors.Is(err, os.ErrNotExist) {
		resp.Error = err.Error()
		return nil
	}
	end := req.Offset + int64(len(req.Data))
	if end > dbToolsMaxUploadBytes {
		resp.Size, resp.Error = current, "the file is larger than 4 GiB"
		return nil
	}
	switch {
	case req.Offset == current && len(req.Data) > 0:
		if err := appendImportChunk(path, current, req.Data); err != nil {
			resp.Size, resp.Error = current, err.Error()
			return nil
		}
		current = end
	case end == current:
	default:
		resp.Size, resp.Error = current, "upload chunk out of order"
		return nil
	}
	resp.Size = current
	return nil
}

// ---- safety copy ----

// DatabaseImportSafetyV1 takes the safety copy before an import: an ordinary
// pre-restore database backup of this domain.
func (a *Agent) DatabaseImportSafetyV1(req transport.DBToolsSafetyRequest, resp *transport.DBToolsSafetyResponse) error {
	*resp = transport.DBToolsSafetyResponse{}
	scope, database, err := dbToolsTarget(req.Target)
	if err != nil {
		resp.Error, resp.Refusal = dbToolsFailure(err)
		return nil
	}
	release, ok := tryLockBackupRestore(scope)
	if !ok {
		resp.Error = backupspec.RestoreInProgress
		return nil
	}
	defer release()
	info, err := a.createSafetyBackup(scope, backupspec.TypeDatabase, database, []backupspec.DatabaseIdentity{database})
	if err != nil {
		resp.Error, resp.Refusal = dbToolsFailure(err)
		return nil
	}
	resp.SafetyBackup = info.Name
	return nil
}

// ---- import ----

// dbImportReader opens the uploaded file, gunzipping a .gz upload.
func dbImportReader(path string) (io.Reader, func(), error) {
	file, _, err := secureOpenRegular(path)
	if err != nil {
		return nil, nil, err
	}
	buffered := bufio.NewReaderSize(file, 256<<10)
	head, _ := buffered.Peek(2)
	if len(head) == 2 && head[0] == 0x1f && head[1] == 0x8b {
		gz, err := gzip.NewReader(buffered)
		if err != nil {
			_ = file.Close()
			return nil, nil, fmt.Errorf("the file looks compressed but is not a readable .gz file")
		}
		return gz, func() { _ = gz.Close(); _ = file.Close() }, nil
	}
	return buffered, func() { _ = file.Close() }, nil
}

// DatabaseImportV1 loads the uploaded file as the database's own user.
func (a *Agent) DatabaseImportV1(req transport.DBToolsImportRequest, resp *transport.DBToolsImportResponse) error {
	*resp = transport.DBToolsImportResponse{}
	scope, database, err := dbToolsTarget(req.Target)
	if err != nil {
		resp.Error, resp.Refusal = dbToolsFailure(err)
		return nil
	}
	if req.SafetyBackup == "" || !generatedBackupName.MatchString(req.SafetyBackup) {
		resp.Code, resp.Error = transport.DBImportCodeSafetyRequired, "no safety copy was named; nothing was loaded"
		return nil
	}
	if err := services.ValidateSQLIdentifier(req.User); err != nil || req.Password == "" {
		resp.Code, resp.Error = transport.DBImportCodeAuth, "this database has no user with a stored password to load the file as"
		return nil
	}
	path, err := dbToolsFile(scope, "import", req.ImportID)
	if err != nil {
		resp.Error = err.Error()
		return nil
	}
	st, err := os.Lstat(path)
	if err != nil || !st.Mode().IsRegular() {
		resp.Code, resp.Error = transport.DBImportCodeNotFound, "the uploaded file is no longer there; upload it again"
		return nil
	}
	if req.Size > 0 && st.Size() != req.Size {
		resp.Code, resp.Error = transport.DBImportCodeUploadIncorrect, "the uploaded file is incomplete; upload it again"
		return nil
	}
	release, ok := tryLockBackupRestore(scope)
	if !ok {
		resp.Error = backupspec.RestoreInProgress
		return nil
	}
	defer release()
	safetyPath, _, err := resolveBackup(scope, req.SafetyBackup)
	if err != nil {
		resp.Code, resp.Error = transport.DBImportCodeSafetyRequired, "the safety copy is not there; nothing was loaded"
		return nil
	}
	defer os.Remove(path)

	ctx, cancel := context.WithTimeout(context.Background(), dbToolsLoadTimeout)
	defer cancel()
	if database.Type == "postgresql" {
		importPostgreSQL(ctx, database, req, path, resp)
		return nil
	}
	importMariaDB(ctx, scope, database, req, path, safetyPath, resp)
	return nil
}

func importPostgreSQL(ctx context.Context, database backupspec.DatabaseIdentity, req transport.DBToolsImportRequest, path string, resp *transport.DBToolsImportResponse) {
	port := database.ClusterPort
	if port == 0 {
		if resolved, err := resolvePGStatementPort(0); err == nil {
			port = resolved
		}
	}
	conn, err := connectPGAsDatabaseUser(ctx, port, database.Name, req.User, req.Password)
	if err != nil {
		if text, ok := pgAuthRefusal(err); ok {
			resp.Code, resp.Error = transport.DBImportCodeAuth, text
			return
		}
		resp.Error = "could not connect to PostgreSQL as the database's user: " + boundedText(pgErrorText(err), 300)
		return
	}
	defer conn.Close(context.Background())
	exec := pgconnLoadExecutor{conn: conn}
	if err := exec.Exec(ctx, "DO $celikpanel$ BEGIN IF (SELECT rolsuper FROM pg_roles WHERE rolname = current_user) THEN RAISE EXCEPTION 'administrator role'; END IF; END $celikpanel$"); err != nil {
		resp.Error = "the database's user is an administrator role, so CelikPanel does not load a file as it; nothing was changed"
		return
	}
	reader, closeReader, err := dbImportReader(path)
	if err != nil {
		resp.Code, resp.Error = transport.DBImportCodeLoadFailed, err.Error()
		return
	}
	defer closeReader()
	if err := exec.Exec(ctx, "BEGIN"); err != nil {
		resp.Error = boundedText(pgErrorText(err), 300)
		return
	}
	loadErr := error(nil)
	if req.EmptyFirst {
		loadErr = exec.Exec(ctx, "DROP OWNED BY CURRENT_USER")
	}
	if loadErr == nil {
		loadErr = loadPGStatements(ctx, exec, reader)
	}
	if loadErr == nil {
		loadErr = exec.Exec(ctx, "COMMIT")
		if loadErr == nil {
			resp.Success = true
			return
		}
	}
	_ = exec.Exec(context.Background(), "ROLLBACK")
	resp.RolledBack = true
	var refusal *pgLoadRefusal
	if errors.As(loadErr, &refusal) {
		resp.Code, resp.Error = refusal.Code, boundedText(refusal.Error(), 400)
		return
	}
	var statement *pgLoadStatementError
	if errors.As(loadErr, &statement) {
		resp.Code = transport.DBImportCodeLoadFailed
		resp.Error = boundedText(fmt.Sprintf("%s: %s", strings.TrimPrefix(statement.where(), "near "), pgErrorText(statement.Err)), 400)
		return
	}
	resp.Code, resp.Error = transport.DBImportCodeLoadFailed, boundedText(pgErrorText(loadErr), 400)
}

// mariadbClient is the client program and whether it has --sandbox (MariaDB
// 10.5.25, 10.6.18, 10.11.8, 11.0.6 and later): in sandbox mode the client
// refuses every command that reaches the system (\! and system, source,
// tee, pager), so a file cannot run anything outside SQL.
var mariadbClient = func() (string, bool) {
	for _, name := range []string{"mariadb", "mysql"} {
		path, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err = exec.CommandContext(ctx, path, "--no-defaults", "--sandbox", "--version").Run()
		cancel()
		return path, err == nil
	}
	return "", false
}

func mariadbImportBlocker() string {
	path, sandbox := mariadbClient()
	switch {
	case path == "":
		return "the MariaDB client program is not installed on this server"
	case !sandbox:
		return "this server's MariaDB client is older than its sandbox mode (MariaDB 10.5.25, 10.6.18, 10.11.8 or later), so CelikPanel does not load uploaded files with it; update the server's MariaDB packages"
	}
	return ""
}

// mariadbEmptyDatabase drops the views, tables and routines of one database
// (as the administrator; names come from the server, quoted by the server).
var mariadbEmptyDatabase = func(name string) error {
	if err := services.ValidateSQLIdentifier(name); err != nil {
		return err
	}
	drop := func(kind, filter string) string {
		return fmt.Sprintf("SELECT GROUP_CONCAT(CONCAT('`', REPLACE(table_name, '`', '``'), '`') SEPARATOR ', ') INTO @celik_names "+
			"FROM information_schema.tables WHERE table_schema = '%s' AND %s; "+
			"SET @celik_sql = IF(@celik_names IS NULL, 'DO 0', CONCAT('DROP %s IF EXISTS ', @celik_names)); "+
			"PREPARE celik_stmt FROM @celik_sql; EXECUTE celik_stmt; DEALLOCATE PREPARE celik_stmt; ", name, filter, kind)
	}
	statement := fmt.Sprintf("USE `%s`; SET SESSION group_concat_max_len = 4294967295; SET FOREIGN_KEY_CHECKS = 0; ", name) +
		drop("VIEW", "table_type = 'VIEW'") + drop("TABLE", "table_type <> 'VIEW'")
	if out, err := mysqlExecStatement(statement); err != nil {
		return fmt.Errorf("empty the database: %s", boundedText(string(out), 300))
	}
	out, err := mysqlExecStatement(fmt.Sprintf(
		"SELECT routine_type, routine_name FROM information_schema.routines WHERE routine_schema = '%s';", name))
	if err != nil {
		return fmt.Errorf("list routines: %s", boundedText(string(out), 300))
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) != 2 || (fields[0] != "PROCEDURE" && fields[0] != "FUNCTION") || services.ValidateSQLIdentifier(fields[1]) != nil {
			continue
		}
		if out, err := mysqlExecStatement(fmt.Sprintf("DROP %s IF EXISTS `%s`.`%s`;", fields[0], name, fields[1])); err != nil {
			return fmt.Errorf("drop routine: %s", boundedText(string(out), 300))
		}
	}
	return nil
}

// mariadbAccessDenied: the stored password no longer signs in.
var mariadbAccessDeniedRe = regexp.MustCompile(`ERROR 1045 \(28000\)|ERROR 1698 \(28000\)`)

func importMariaDB(ctx context.Context, scope backupScope, database backupspec.DatabaseIdentity, req transport.DBToolsImportRequest, path, safetyPath string, resp *transport.DBToolsImportResponse) {
	client, sandbox := mariadbClient()
	if client == "" || !sandbox {
		resp.Code, resp.Error = transport.DBImportCodeClientUnsafe, mariadbImportBlocker()
		return
	}
	username, err := cronTenantUser(transport.CronTenant{SubscriptionID: scope.SubscriptionID, DomainID: scope.DomainID, Domain: scope.DomainName})
	if err != nil {
		resp.Code, resp.Error = transport.DBImportCodeNoSiteAccount, "this domain has no hosting account to load the file under; nothing was changed"
		return
	}
	argv := []string{client, "--no-defaults", "--sandbox", "--batch", "--protocol=socket",
		"--user=" + req.User, "--database=" + database.Name, "--default-character-set=utf8mb4"}
	env := []string{"MYSQL_PWD=" + req.Password}
	// The sign-in is tried first, so a stale password changes nothing.
	if output, err := runDatabaseClientAsSiteUser(ctx, username, argv, env, strings.NewReader("SELECT 1;\n")); err != nil {
		if mariadbAccessDeniedRe.MatchString(output) {
			resp.Code, resp.Error = transport.DBImportCodeAuth, "MariaDB refused the password CelikPanel has stored for this database's user; it was probably changed outside the panel"
			return
		}
		resp.Error = "could not connect to MariaDB as the database's user: " + boundedText(strings.TrimSpace(output+" "+err.Error()), 300)
		return
	}
	if req.EmptyFirst {
		if err := mariadbEmptyDatabase(database.Name); err != nil {
			rollbackMariaDBImport(scope, database, safetyPath, err, resp)
			return
		}
	}
	reader, closeReader, err := dbImportReader(path)
	if err != nil {
		rollbackMariaDBImport(scope, database, safetyPath, err, resp)
		return
	}
	defer closeReader()
	output, err := runDatabaseClientAsSiteUser(ctx, username, argv, env, reader)
	if err == nil {
		resp.Success = true
		return
	}
	loadErr := errors.New(strings.TrimSpace(output))
	if strings.TrimSpace(output) == "" {
		loadErr = err
	}
	rollbackMariaDBImport(scope, database, safetyPath, loadErr, resp)
}

// rollbackMariaDBImport puts the database back as the safety copy was:
// everything is dropped and the copy is restored.
func rollbackMariaDBImport(scope backupScope, database backupspec.DatabaseIdentity, safetyPath string, loadErr error, resp *transport.DBToolsImportResponse) {
	resp.Code = transport.DBImportCodeLoadFailed
	resp.Error = boundedText(loadErr.Error(), 400)
	work, err := newRestoreWorkDir(scope)
	if err == nil {
		defer os.RemoveAll(work)
		var manifest backupManifest
		manifest, err = unpackBackupPackage(safetyPath, filepath.Join(work, "safety"), scope)
		if err == nil && (manifest.Type != backupspec.TypeDatabase || len(manifest.Databases) != 1 || !manifest.Databases[0].Identity.SameDatabase(database)) {
			err = errors.New("the safety copy is not this database's")
		}
		if err == nil {
			err = mariadbEmptyDatabase(database.Name)
		}
		if err == nil {
			err = restoreDatabaseFromFile(database, filepath.Join(work, "safety", filepath.FromSlash(databasePayloadName(database.ID))))
		}
	}
	if err != nil {
		resp.Code = transport.DBImportCodeRollbackFailed
		resp.Error = boundedText(resp.Error, 300) + "; putting the database back from the safety copy did not finish: " + boundedText(err.Error(), 200)
		return
	}
	resp.RolledBack = true
}

// ---- copy ----

// DatabaseCopyV1 copies a database into one the Panel has just created.
func (a *Agent) DatabaseCopyV1(req transport.DBToolsCopyRequest, resp *transport.DBToolsCopyResponse) error {
	*resp = transport.DBToolsCopyResponse{}
	scope, source, err := dbToolsTarget(req.Source)
	if err != nil {
		resp.Error, resp.Refusal = dbToolsFailure(err)
		return nil
	}
	target, err := validateDatabaseIdentity(req.Target)
	if err == nil && target.Type != source.Type {
		err = errors.New("a database is copied only to the same engine")
	}
	if err == nil {
		target, err = resolveBackupCluster(target)
	}
	if err == nil && source.Type == "postgresql" && target.ClusterVersion != source.ClusterVersion {
		err = errors.New("the copy is on another PostgreSQL major than the source")
	}
	if err != nil {
		resp.Error, resp.Refusal = dbToolsFailure(err)
		return nil
	}
	dir, err := ensureBackupDir(scope)
	if err != nil {
		resp.Error = err.Error()
		return nil
	}
	work, err := os.MkdirTemp(dir, ".dbcopy-")
	if err != nil {
		resp.Error = err.Error()
		return nil
	}
	defer os.RemoveAll(work)
	dump := filepath.Join(work, "source.sql.gz")
	if err := dumpDatabaseToFile(source, dump); err != nil {
		resp.Error, resp.Refusal = dbToolsFailure(fmt.Errorf("copy the source database: %w", err))
		return nil
	}
	if err := restoreDatabaseFromFile(target, dump); err != nil {
		resp.Error = "load the copy: " + boundedText(err.Error(), 300)
		return nil
	}
	resp.Success = true
	return nil
}

// ---- check and repair ----

var mariadbCheckRun = func(name string, repair bool) (string, error) {
	program := "mariadb-check"
	if _, err := exec.LookPath(program); err != nil {
		program = "mysqlcheck"
	}
	mode := "--check"
	if repair {
		mode = "--repair"
	}
	ctx, cancel := context.WithTimeout(context.Background(), dbToolsLoadTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, program, "--protocol=socket", "--user=root", mode, name).CombinedOutput()
	return string(out), err
}

// parseMariaDBCheck reads mariadb-check's report: "db.table   OK", or the
// table's line followed by "error :", "warning :", "note :" and "status :".
func parseMariaDBCheck(database, output string) transport.DBToolsCheckResponse {
	result := transport.DBToolsCheckResponse{Problems: []transport.DBToolsTableProblem{}}
	prefix := database + "."
	current := ""
	selfRepair := false
	finish := func() {
		if current != "" && selfRepair {
			result.SelfRepairing++
		}
	}
	for _, raw := range strings.Split(output, "\n") {
		line := strings.TrimRight(raw, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, prefix) {
			finish()
			fields := strings.Fields(trimmed)
			current = strings.TrimPrefix(fields[0], prefix)
			selfRepair = false
			result.Checked++
			continue
		}
		kind, message, found := strings.Cut(trimmed, ":")
		if !found || current == "" {
			continue
		}
		kind, message = strings.TrimSpace(kind), strings.TrimSpace(message)
		switch kind {
		case "error", "Error", "warning", "Warning":
			result.Problems = append(result.Problems, transport.DBToolsTableProblem{Table: current, Message: boundedText(message, 200)})
		case "note", "Note":
			if strings.Contains(message, "doesn't support repair") || strings.Contains(message, "does not support repair") {
				selfRepair = true
			}
		case "status":
			if !strings.EqualFold(message, "OK") && !strings.EqualFold(message, "Table is already up to date") {
				result.Problems = append(result.Problems, transport.DBToolsTableProblem{Table: current, Message: boundedText(message, 200)})
			}
		}
	}
	finish()
	return result
}

// DatabaseCheckV1 checks or repairs.
func (a *Agent) DatabaseCheckV1(req transport.DBToolsCheckRequest, resp *transport.DBToolsCheckResponse) error {
	*resp = transport.DBToolsCheckResponse{Problems: []transport.DBToolsTableProblem{}}
	_, database, err := dbToolsTarget(req.Target)
	if err != nil {
		resp.Error, resp.Refusal = dbToolsFailure(err)
		return nil
	}
	if database.Type == "postgresql" {
		if req.Repair {
			resp.Error = "PostgreSQL keeps its tables consistent itself; it has no repair step"
			return nil
		}
		if _, err := dbToolsQuery(database, true, "VACUUM (ANALYZE);"); err != nil {
			resp.Error = boundedText(err.Error(), 300)
			return nil
		}
		out, err := dbToolsQuery(database, true,
			"SELECT count(*) FROM pg_catalog.pg_tables WHERE schemaname NOT IN ('pg_catalog','information_schema');")
		if err == nil {
			if v, err := dbToolsInts(out, 1); err == nil {
				resp.Checked = int(v[0])
			}
		}
		return nil
	}
	output, err := mariadbCheckRun(database.Name, req.Repair)
	parsed := parseMariaDBCheck(database.Name, output)
	if err != nil && parsed.Checked == 0 {
		resp.Error = boundedText(strings.TrimSpace(output), 300)
		if resp.Error == "" {
			resp.Error = err.Error()
		}
		return nil
	}
	*resp = parsed
	return nil
}
