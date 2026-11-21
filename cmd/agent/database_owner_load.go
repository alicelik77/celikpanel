package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/backupspec"
	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// SQL from a file runs with the rights of the database's own account and
// nothing more (security lane S1, 20 November 2026).
//
// "SQL from a file" is every payload the Agent loads into a database: a
// backup's dump (made here or brought back from a remote destination the
// customer controls), the safety copy put back after a failed restore, the
// dump of a database or site copy, a cPanel archive's dump and a file a
// customer uploads. Its text is the customer's: even a dump CelikPanel made
// itself holds names and bodies the customer chose. Before this lane a
// restore and a copy fed it to `mysql --user=root` (no --sandbox: a `\!` line
// ran a shell command as root) or to `sudo -u postgres psql` (no
// --no-psqlrc; a `\!` line ran as the postgres account, and `RESET ROLE`
// undid the prelude's SET ROLE, giving the file the engine's superuser).
//
// The one mechanism, used by every such load:
//   - MariaDB: the client with --no-defaults --sandbox (it refuses \!,
//     system, source, tee and pager, so the file cannot reach a shell or a
//     local file), signed in as the database's own user, run under the
//     site's own system account (never root). A client without sandbox mode
//     is refused, never replaced by an unsandboxed one. DEFINER clauses of the
//     dump's views, triggers and routines are written as CURRENT_USER (the
//     database's own user may not create objects for another account).
//   - PostgreSQL: no psql at all. The Agent signs in as the database's own
//     role over the protocol and sends the file's statements itself
//     (database_import_pg.go), in one transaction; a meta-command is refused,
//     an administrator role is refused.
//
// The engine's administrator only runs fixed statements CelikPanel writes
// itself, never a statement taken from a file.
//
// Dosyadan gelen SQL yalnız veritabanının kendi hesabının haklarıyla çalışır:
// MariaDB'de sandbox kipindeki istemci, sitenin kendi hesabıyla; PostgreSQL'de
// psql değil, veritabanının kendi rolüyle protokol üzerinden. Yönetici yalnız
// CelikPanel'in kendi yazdığı sabit ifadeleri çalıştırır.

// databaseLoadAccount is the database's own account.
type databaseLoadAccount struct {
	User     string
	Password string
}

// databaseLoadTarget is one database a file is loaded into: the account
// whose rights the file gets, and (MariaDB) the system account the client
// runs under.
type databaseLoadTarget struct {
	backupspec.DatabaseIdentity
	Account  databaseLoadAccount
	SiteUser string
}

// databaseLoadRefusal is a load refused before anything was written. Code is
// a backupspec.RestoreCode*.
type databaseLoadRefusal struct {
	Code   string
	Reason string
}

func (e *databaseLoadRefusal) Error() string { return e.Reason }

func loadRefusal(code, format string, args ...any) error {
	return &databaseLoadRefusal{Code: code, Reason: fmt.Sprintf(format, args...)}
}

var errLoadAdministratorRole = errors.New("administrator role")

// databaseLoadFallbackUser runs the MariaDB client when the domain has no
// hosting account of its own (a database of a domain without hosting).
const databaseLoadFallbackUser = "nobody"

// databaseLoadSiteUser is the system account a MariaDB client of this
// scope runs under: the site's own account, else an account with no rights
// at all. Never root.
var databaseLoadSiteUser = func(subscriptionID, domainID int, domain string) string {
	name, err := cronTenantUser(transport.CronTenant{SubscriptionID: subscriptionID, DomainID: domainID, Domain: domain})
	if err == nil {
		return name
	}
	return databaseLoadFallbackUser
}

// databaseLoadAccounts matches a restore's credentials to its databases.
func databaseLoadAccounts(credentials []backupspec.DatabaseCredential) map[int]databaseLoadAccount {
	accounts := make(map[int]databaseLoadAccount, len(credentials))
	for _, credential := range credentials {
		if credential.ID > 0 {
			accounts[credential.ID] = databaseLoadAccount{User: credential.User, Password: credential.Password}
		}
	}
	return accounts
}

// prepareDatabaseLoadFor is prepareDatabaseLoad (a seam for tests).
var prepareDatabaseLoadFor = prepareDatabaseLoad

// prepareDatabaseLoad proves, without changing anything, that a file can be
// loaded into target with its account's rights: the account is there and
// signs in, the MariaDB client has its sandbox, and the PostgreSQL role is
// the database's owner and no administrator. A refusal is a
// *databaseLoadRefusal; any other error is a failure to ask.
func prepareDatabaseLoad(target databaseLoadTarget) error {
	database, err := validateDatabaseIdentity(target.DatabaseIdentity)
	if err != nil {
		return err
	}
	if services.ValidateSQLIdentifier(target.Account.User) != nil || target.Account.Password == "" {
		return loadRefusal(backupspec.RestoreCodeDatabaseAccount,
			"database %s has no user with a stored password to load the backup as", database.Name)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if database.Type == "postgresql" {
		conn, err := pgOpenLoadSession(ctx, database, target.Account)
		if err != nil {
			return pgLoadSessionRefusal(database, target.Account, err)
		}
		defer conn.Close(context.Background())
		return pgCheckLoadOwner(ctx, conn, database, target.Account)
	}
	return mariadbSignIn(ctx, target.SiteUser, database, target.Account)
}

// loadDatabaseFile loads a gzip'd dump into target with its account's
// rights (restoreDatabaseFromFile). PostgreSQL: the account's objects in the
// database are dropped first and the dump recreates them, in one transaction
// (what a restore needs on a database that still holds its tables).
func loadDatabaseFile(target databaseLoadTarget, source string) error {
	file, _, err := secureOpenRegular(source)
	if err != nil {
		return err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gz.Close()
	ctx, cancel := context.WithTimeout(context.Background(), backupDatabaseTimeout)
	defer cancel()
	return loadDatabaseSQL(ctx, target, gz, true)
}

// loadDatabaseSQL is the one place SQL from a file reaches a database.
func loadDatabaseSQL(ctx context.Context, target databaseLoadTarget, r io.Reader, pgEmptyFirst bool) error {
	database, err := validateDatabaseIdentity(target.DatabaseIdentity)
	if err != nil {
		return err
	}
	if services.ValidateSQLIdentifier(target.Account.User) != nil || target.Account.Password == "" {
		return loadRefusal(backupspec.RestoreCodeDatabaseAccount,
			"database %s has no user with a stored password to load the file as", database.Name)
	}
	if database.Type == "postgresql" {
		conn, err := pgOpenLoadSession(ctx, database, target.Account)
		if err != nil {
			return pgLoadSessionRefusal(database, target.Account, err)
		}
		defer conn.Close(context.Background())
		// S4 R2a: the user's time defaults do not cut the load; its
		// temporary-file limit stays (database_load_limits.go).
		if err := pgLoadLiftSessionLimits(ctx, conn); err != nil {
			return pgLoadSessionDefaultsError(err)
		}
		limit := pgLoadTempFileLimit(ctx, conn)
		if err := pgLoadTransaction(ctx, conn, r, pgEmptyFirst); err != nil {
			if pgTempFileLimitReached(err) {
				return fmt.Errorf("ERROR: %s", pgLoadTempFileLimitText(target.Account.User, limit))
			}
			return pgLoadErrorText(err)
		}
		return nil
	}
	output, err := mariadbLoadAsAccount(ctx, target.SiteUser, database, target.Account, r)
	if err == nil {
		return nil
	}
	var refusal *databaseLoadRefusal
	if errors.As(err, &refusal) {
		return err
	}
	if text := strings.TrimSpace(output); text != "" {
		return fmt.Errorf("mariadb: %s", boundedText(text, 2000))
	}
	return err
}

// ---- PostgreSQL ----

// connectPGForLoad signs in as the database's own role (a seam for tests).
var connectPGForLoad = connectPGAsDatabaseUser

func pgLoadPort(database backupspec.DatabaseIdentity) int {
	port := database.ClusterPort
	if port == 0 {
		if resolved, err := resolvePGStatementPort(0); err == nil {
			port = resolved
		}
	}
	return port
}

// pgOpenLoadSession signs in as account and refuses an administrator role:
// a file loaded as one would have every database of the server. The answer
// comes from the server's own report at sign-in (is_superuser), not from a
// query the database's search_path could steer.
func pgOpenLoadSession(ctx context.Context, database backupspec.DatabaseIdentity, account databaseLoadAccount) (pgLoadConn, error) {
	conn, err := connectPGForLoad(ctx, pgLoadPort(database), database.Name, account.User, account.Password)
	if err != nil {
		return nil, err
	}
	if conn.ParameterStatus("is_superuser") != "off" {
		conn.Close(context.Background())
		return nil, errLoadAdministratorRole
	}
	return conn, nil
}

// pgLoadSessionRefusal types a failed sign-in.
func pgLoadSessionRefusal(database backupspec.DatabaseIdentity, account databaseLoadAccount, err error) error {
	if errors.Is(err, errLoadAdministratorRole) {
		return loadRefusal(backupspec.RestoreCodeNeedsAdministrator,
			"the user %s of database %s is an administrator role, so CelikPanel does not load a file as it", account.User, database.Name)
	}
	if text, ok := pgAuthRefusal(err); ok {
		return loadRefusal(backupspec.RestoreCodeDatabaseAccount, "database %s: %s", database.Name, text)
	}
	return fmt.Errorf("could not connect to PostgreSQL as the database's user: %s", boundedText(pgErrorText(err), 300))
}

// pgCheckLoadOwner: a restore drops the account's objects and the dump
// recreates them, so the account must be the database's owner (a restore as
// another role would stop at the owner's tables).
func pgCheckLoadOwner(ctx context.Context, conn pgLoadConn, database backupspec.DatabaseIdentity, account databaseLoadAccount) error {
	rows, err := conn.QueryValues(ctx,
		"SELECT pg_catalog.pg_get_userbyid(d.datdba)::pg_catalog.text, pg_catalog.current_user()::pg_catalog.text "+
			"FROM pg_catalog.pg_database d WHERE d.datname OPERATOR(pg_catalog.=) pg_catalog.current_database()")
	if err != nil {
		return fmt.Errorf("could not read database %s's owner: %s", database.Name, boundedText(pgErrorText(err), 300))
	}
	if len(rows) != 1 || len(rows[0]) != 2 {
		return fmt.Errorf("could not read database %s's owner", database.Name)
	}
	if owner, current := rows[0][0], rows[0][1]; owner != current {
		return loadRefusal(backupspec.RestoreCodeDatabaseAccount,
			"the user %s CelikPanel has stored for database %s is not its owner (%s)", account.User, database.Name, owner)
	}
	return nil
}

// pgLoadTransaction loads r in one transaction: BEGIN, (the account's
// objects dropped), the file's statements, COMMIT; anything that stops
// rolls the whole load back.
func pgLoadTransaction(ctx context.Context, exec pgLoadExecutor, r io.Reader, emptyFirst bool) error {
	if err := exec.Exec(ctx, "BEGIN"); err != nil {
		return err
	}
	var loadErr error
	if emptyFirst {
		loadErr = exec.Exec(ctx, "DROP OWNED BY CURRENT_USER")
	}
	if loadErr == nil {
		loadErr = loadPGStatements(ctx, exec, r)
	}
	if loadErr == nil {
		if loadErr = exec.Exec(ctx, "COMMIT"); loadErr == nil {
			return nil
		}
	}
	_ = exec.Exec(context.Background(), "ROLLBACK")
	return loadErr
}

// pgLoadErrorText keeps a refusal typed and says where a statement stopped.
func pgLoadErrorText(err error) error {
	var refusal *pgLoadRefusal
	if errors.As(err, &refusal) {
		return fmt.Errorf("ERROR: %s; nothing was changed", refusal.Error())
	}
	var statement *pgLoadStatementError
	if errors.As(err, &statement) {
		return fmt.Errorf("ERROR: %s: %s; nothing was changed", strings.TrimPrefix(statement.where(), "near "), pgErrorText(statement.Err))
	}
	return fmt.Errorf("ERROR: %s; nothing was changed", pgErrorText(err))
}

// ---- MariaDB ----

// mariadbLoadArgv is the client's command line: no option file (a file on
// the system could undo the sandbox), sandbox mode, the database's own user.
func mariadbLoadArgv(client, database, user string) []string {
	// S4 R2a: the user's statement time does not cut the load
	// (database_load_limits.go).
	return mariadbLoadArgvWithLimits([]string{client, "--no-defaults", "--sandbox", "--batch", "--protocol=socket",
		"--user=" + user, "--database=" + database, "--default-character-set=utf8mb4",
		"--max-allowed-packet=1073741824"})
}

// mariadbSafeClient is the client to load files with, or the refusal.
func mariadbSafeClient() (string, error) {
	client, sandbox := mariadbClient()
	if client == "" || !sandbox {
		return "", loadRefusal(backupspec.RestoreCodeClientUnsafe, "%s", mariadbImportBlocker())
	}
	return client, nil
}

func mariadbLoadEnv(account databaseLoadAccount) []string {
	return []string{"MYSQL_PWD=" + account.Password}
}

// mariadbSignIn tries the account first, so a stale password changes
// nothing.
func mariadbSignIn(ctx context.Context, siteUser string, database backupspec.DatabaseIdentity, account databaseLoadAccount) error {
	client, err := mariadbSafeClient()
	if err != nil {
		return err
	}
	output, err := runDatabaseClientAsSiteUser(ctx, siteUser, mariadbLoadArgv(client, database.Name, account.User), mariadbLoadEnv(account), strings.NewReader("SELECT 1;\n"))
	if err == nil {
		return nil
	}
	if mariadbAccessDeniedRe.MatchString(output) {
		return loadRefusal(backupspec.RestoreCodeDatabaseAccount,
			"MariaDB refused the password CelikPanel has stored for user %s of database %s; it was probably changed outside the panel", account.User, database.Name)
	}
	return fmt.Errorf("could not connect to MariaDB as the database's user: %s", boundedText(strings.TrimSpace(output+" "+err.Error()), 300))
}

// mariadbLoadAsAccount runs the file through the sandboxed client as the
// account. The output is the client's (the error comes first).
func mariadbLoadAsAccount(ctx context.Context, siteUser string, database backupspec.DatabaseIdentity, account databaseLoadAccount, r io.Reader) (string, error) {
	client, err := mariadbSafeClient()
	if err != nil {
		return "", err
	}
	if siteUser == "" {
		siteUser = databaseLoadFallbackUser
	}
	return runDatabaseClientAsSiteUser(ctx, siteUser, mariadbLoadArgv(client, database.Name, account.User), mariadbLoadEnv(account), newMariaDBDefinerFilter(r))
}

// mariadbDefinerRe finds the DEFINER clause dump programs write at the start
// of a view, trigger, routine or event (mariadb-dump/mysqldump's versioned
// comments, phpMyAdmin's plain form). A definer other than the signed-in
// user needs SET USER (an administrator's right); the clause is written as
// CURRENT_USER, so the object belongs to the database's own user. Rights
// cannot grow this way: the definer can only become the account loading.
var mariadbDefinerRe = regexp.MustCompile(
	"(?i)^(\\s*(?:/\\*!\\d+\\s+)?CREATE\\s+(?:OR\\s+REPLACE\\s+)?(?:ALGORITHM\\s*=\\s*\\w+\\s+)?" +
		"|\\s*/\\*!\\d+\\s+" +
		"|\\s*/\\*!\\d+\\s+CREATE\\s*\\*/\\s*/\\*!\\d+\\s+)" +
		"DEFINER\\s*=\\s*(?:CURRENT_USER(?:\\s*\\(\\s*\\))?" +
		"|(?:`(?:[^`]|``)*`|'(?:[^'\\\\]|''|\\\\.)*'|\"(?:[^\"\\\\]|\\\\.)*\"|[A-Za-z0-9_.$-]+)" +
		"(?:\\s*@\\s*(?:`(?:[^`]|``)*`|'(?:[^'\\\\]|''|\\\\.)*'|\"(?:[^\"\\\\]|\\\\.)*\"|[A-Za-z0-9_.%$:-]+))?)")

// mariadbDefinerFilter rewrites the DEFINER clause at the start of a line;
// every other byte passes unchanged (a data line never starts a line with a
// CREATE: dump programs write a row's newlines as \n).
type mariadbDefinerFilter struct {
	r       *bufio.Reader
	pending []byte
	atStart bool
	err     error
}

func newMariaDBDefinerFilter(r io.Reader) io.Reader {
	return &mariadbDefinerFilter{r: bufio.NewReaderSize(r, 64<<10), atStart: true}
}

func (f *mariadbDefinerFilter) Read(p []byte) (int, error) {
	for len(f.pending) == 0 {
		if f.err != nil {
			return 0, f.err
		}
		chunk, err := f.r.ReadSlice('\n')
		if len(chunk) > 0 {
			line := append([]byte(nil), chunk...)
			if f.atStart && bytes.Contains(bytes.ToUpper(line[:min(len(line), 4096)]), []byte("DEFINER")) {
				line = mariadbDefinerRe.ReplaceAll(line, []byte("${1}DEFINER=CURRENT_USER"))
			}
			f.atStart = line[len(line)-1] == '\n'
			f.pending = line
		}
		if err != nil && !errors.Is(err, bufio.ErrBufferFull) {
			f.err = err
		}
	}
	n := copy(p, f.pending)
	f.pending = f.pending[n:]
	return n, nil
}

// mariadbRoutineRe: a line that creates a stored function or a trigger.
var mariadbRoutineRe = regexp.MustCompile(
	"(?i)^\\s*(?:/\\*!\\d+\\s+)?CREATE(?:\\s*\\*/)?\\s+(?:/\\*!\\d+\\s+)?(?:OR\\s+REPLACE\\s+)?" +
		"(?:DEFINER\\s*=\\s*\\S+?\\s*(?:\\*/\\s*/\\*!\\d+\\s+)?)?(?:AGGREGATE\\s+)?(?:FUNCTION|TRIGGER)\\b")

// mariadbBinlogGate is whether this MariaDB lets only an administrator
// create stored functions and triggers (binary logging on and
// log_bin_trust_function_creators off). Asked as the administrator, with a
// fixed statement.
var mariadbBinlogGate = func() (bool, error) {
	out, err := mysqlExecStatement("SELECT CONCAT('gate=', @@global.log_bin, ':', @@global.log_bin_trust_function_creators);")
	if err != nil {
		return false, fmt.Errorf("the MariaDB server could not be asked about its binary log: %s", boundedText(string(out), 200))
	}
	value, ok := markerValue(out, "gate")
	if !ok {
		return false, errors.New("the MariaDB server's answer about its binary log could not be read")
	}
	logBin, trust, _ := strings.Cut(value, ":")
	return logBin == "1" && trust != "1", nil
}

// sqlFileCreatesRoutines reads a gzip'd dump for a stored function or
// trigger.
func sqlFileCreatesRoutines(path string) (bool, error) {
	file, _, err := secureOpenRegular(path)
	if err != nil {
		return false, err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return false, err
	}
	defer gz.Close()
	return sqlReaderCreatesRoutines(gz)
}

// sqlReaderCreatesRoutines reads plain SQL for a line that creates a stored
// function or trigger.
func sqlReaderCreatesRoutines(r io.Reader) (bool, error) {
	reader := bufio.NewReaderSize(r, 64<<10)
	atStart := true
	for {
		chunk, err := reader.ReadSlice('\n')
		if len(chunk) > 0 {
			if atStart && mariadbRoutineRe.Match(chunk[:min(len(chunk), 4096)]) {
				return true, nil
			}
			atStart = chunk[len(chunk)-1] == '\n'
		}
		if errors.Is(err, io.EOF) {
			return false, nil
		}
		if err != nil && !errors.Is(err, bufio.ErrBufferFull) {
			return false, err
		}
	}
}

// checkLoadableByAccount refuses, before anything is written, a MariaDB
// load that would stop half-way because the server lets only its
// administrator create the dump's stored functions and triggers. paths are
// every dump the operation may load (the backup's and the safety copy's).
func checkLoadableByAccount(database backupspec.DatabaseIdentity, paths ...string) error {
	if database.Type == "postgresql" {
		return nil
	}
	gated, err := mariadbBinlogGate()
	if err != nil || !gated {
		return err
	}
	for _, path := range paths {
		found, err := sqlFileCreatesRoutines(path)
		if err != nil {
			return err
		}
		if found {
			return loadRefusal(backupspec.RestoreCodeNeedsAdministrator,
				"database %s holds stored functions or triggers, and this MariaDB server writes a binary log and lets only its administrator create them (log_bin_trust_function_creators is off)", database.Name)
		}
	}
	return nil
}

// ---- restore failures ----

// databaseLoadFailure types a refusal for a restore's answer.
func databaseLoadFailure(err error) error {
	var refusal *databaseLoadRefusal
	if errors.As(err, &refusal) {
		return &restoreFailure{code: refusal.Code, reason: errors.New(refusal.Reason), err: err}
	}
	return err
}
