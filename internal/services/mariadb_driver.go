package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/dbremote"
	"github.com/alicelik/celikpanel/internal/mariadbgrant"
)

// MariaDBDriver implements DatabaseDriver for MariaDB
type MariaDBDriver struct {
	host     string
	port     int
	username string
	password string
	// grantReadBack: what the last GrantPrivileges could read back;
	// grantPattern: it wrote the pattern form (a name whose exact form does
	// not fit a grant row).
	grantReadBack string
	grantPattern  bool
}

// mariaDBSuperuser is the account a MariaDB arrives with, and the one every
// credential stored before R-057 belongs to.
// mariaDBSuperuser, bir MariaDB'nin birlikte geldigi hesaptir.
const mariaDBSuperuser = `root`

const mariaDBCommandTimeout = 30 * time.Second

func quoteMySQLOptionValue(value string) (string, error) {
	if strings.IndexByte(value, 0) >= 0 {
		return ``, fmt.Errorf(`MySQL option value contains NUL`)
	}
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, string(rune(13)), `\r`)
	value = strings.ReplaceAll(value, string(rune(10)), `\n`)
	value = strings.ReplaceAll(value, string(rune(9)), `\t`)
	value = strings.ReplaceAll(value, string(rune(34)), string([]rune{92, 34}))
	return string(rune(34)) + value + string(rune(34)), nil
}

func (d *MariaDBDriver) writeMySQLClientFile() (string, func(), error) {
	file, err := os.CreateTemp(``, `celikpanel-mysql-*.cnf`)
	if err != nil {
		return ``, nil, fmt.Errorf(`create protected MySQL client file: %w`, err)
	}
	path := file.Name()
	cleanup := func() {
		_ = os.Remove(path)
	}
	fail := func(cause error) (string, func(), error) {
		_ = file.Close()
		cleanup()
		return ``, nil, cause
	}
	if err := file.Chmod(0o600); err != nil {
		return fail(fmt.Errorf(`protect MySQL client file: %w`, err))
	}
	if _, err := fmt.Fprintln(file, `[client]`); err != nil {
		return fail(fmt.Errorf(`write MySQL client file: %w`, err))
	}
	writeOption := func(key, value string) error {
		quoted, err := quoteMySQLOptionValue(value)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(file, key+`=`+quoted)
		return err
	}
	if err := writeOption(`user`, driverUsername(d.username, mariaDBSuperuser)); err != nil {
		return fail(fmt.Errorf(`write MySQL client user: %w`, err))
	}
	if d.password != `` {
		if err := writeOption(`password`, d.password); err != nil {
			return fail(fmt.Errorf(`write MySQL client password: %w`, err))
		}
	}
	if d.host != `` {
		if err := writeOption(`host`, d.host); err != nil {
			return fail(fmt.Errorf(`write MySQL client host: %w`, err))
		}
	}
	if d.port > 0 {
		if _, err := fmt.Fprintln(file, `port=`+fmt.Sprint(d.port)); err != nil {
			return fail(fmt.Errorf(`write MySQL client port: %w`, err))
		}
	}
	if d.host != `` || d.port > 0 {
		if _, err := fmt.Fprintln(file, `protocol=tcp`); err != nil {
			return fail(fmt.Errorf(`write MySQL client protocol: %w`, err))
		}
	}
	if err := file.Close(); err != nil {
		cleanup()
		return ``, nil, fmt.Errorf(`close MySQL client file: %w`, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		cleanup()
		return ``, nil, fmt.Errorf(`inspect MySQL client file: %w`, err)
	}
	if runtime.GOOS != `windows` && info.Mode().Perm() != 0o600 {
		cleanup()
		return ``, nil, fmt.Errorf(`MySQL client file mode is %o, want 600`, info.Mode().Perm())
	}
	return path, cleanup, nil
}

// NewMariaDBDriver creates a new MariaDB driver
func NewMariaDBDriver(config DriverConfig) *MariaDBDriver {
	return &MariaDBDriver{
		host:     config.Host,
		port:     config.Port,
		username: config.Username,
		password: config.Password,
	}
}

func (d *MariaDBDriver) mysqlCommand(ctx context.Context, sql string) (*exec.Cmd, func(), error) {
	path, cleanup, err := d.writeMySQLClientFile()
	if err != nil {
		return nil, nil, err
	}
	cmd := exec.CommandContext(
		ctx,
		`mysql`,
		`--defaults-extra-file=`+path,
		`--batch`,
		`--raw`,
	)
	cmd.Stdin = strings.NewReader(sql)
	return cmd, cleanup, nil
}

func (d *MariaDBDriver) runSQL(sql string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), mariaDBCommandTimeout)
	defer cancel()
	cmd, cleanup, err := d.mysqlCommand(ctx, sql)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf(`MariaDB command timed out: %w`, ctx.Err())
		}
		// R-053. The client wrote why it failed and this call used to throw it
		// away, so every layer above saw "exit status 1" and could tell an
		// engine that refused a password from an engine that was not running
		// only by guessing. The output is read here, where it exists, and what
		// it means is kept - not the text, which can echo a statement.
		// R-053. Istemci neden basarisiz oldugunu yazdi ve bu cagri onu atardi.
		// Cikti burada okunur ve ne anlama geldigi saklanir; metin degil.
		wrapped := WrapDatabaseEngineFailure(
			fmt.Errorf(`MariaDB command failed: %w`, err), output,
		)
		// alpha.134 follow-up: "may not read this table" (ERROR 1142) is a
		// verified answer the grant read-back tells apart from an unknown one.
		if mariadbgrant.ReadDenied(string(output)) {
			return nil, fmt.Errorf("%w (%w)", wrapped, mariadbgrant.ErrReadDenied)
		}
		return nil, wrapped
	}
	return output, nil
}

// TestConnection tests MariaDB connection
func (d *MariaDBDriver) TestConnection() error {
	_, err := d.runSQL(`SELECT 1;`)
	if err != nil {
		return fmt.Errorf(`MariaDB connection failed: %w`, err)
	}
	return nil
}

// ServerVersion asks the MariaDB server what it is.
//
// The answer is marked by the statement itself (`version=...`) and looked for
// by that mark, because what else the client prints differs by platform
// (11 Oct 2026). It used to be "the first non-empty line after the first
// line", which assumed the first line is the column name. On Arch the client
// invoked as `mysql` first prints that the name is deprecated, so the column
// name `VERSION()` became the second line and was recorded as the version.
//
// ServerVersion, MariaDB sunucusuna ne oldugunu sorar. Yanit, ifadenin kendi
// koydugu isaretle (`version=...`) aranir; istemcinin baska ne yazdigi
// platforma gore degisir.
func (d *MariaDBDriver) ServerVersion() (string, error) {
	output, err := d.runSQL(`SELECT CONCAT('version=', VERSION());`)
	if err != nil {
		return ``, err
	}
	if version := mariaDBMarkedVersion(string(output)); version != `` {
		return version, nil
	}
	return ``, fmt.Errorf(`MariaDB reported no version`)
}

// mariaDBMarkedVersion finds the one line that is the server's answer to
// `SELECT CONCAT('version=', VERSION())`. The column name, which repeats the
// expression, does not start with the mark; neither does a client warning.
func mariaDBMarkedVersion(output string) string {
	for _, line := range strings.Split(output, "\n") {
		if version, marked := strings.CutPrefix(strings.TrimSpace(line), `version=`); marked {
			if version = strings.TrimSpace(version); version != `` {
				return version
			}
		}
	}
	return ``
}

// CreateDatabase creates a MariaDB database; a name that exists already is
// ErrDatabaseNameTaken.
func (d *MariaDBDriver) CreateDatabase(name string) error {
	ident, err := QuoteMySQLIdentifier(name)
	if err != nil {
		return fmt.Errorf("invalid database name: %w", err)
	}
	literal, err := QuoteMySQLStringLiteral(name)
	if err != nil {
		return fmt.Errorf("invalid database name: %w", err)
	}
	// alpha.134 review: a name whose exact form does not fit a grant row would
	// be granted cut short - a right on another name - so it is not made.
	if !mariadbgrant.ExactFits(name) {
		return fmt.Errorf("database %s: %w", name, ErrDatabaseNameTooLong)
	}
	// alpha.129: exclusive. "IF NOT EXISTS" took over a database of the same
	// name that was already there (another account's, or one made outside
	// CelikPanel) and the caller then granted its new user on it.
	// alpha.134: compared exactly (BINARY); information_schema compares names
	// case-insensitively.
	if taken, err := d.countIsNonZero("SELECT CONCAT('n=', COUNT(*)) FROM information_schema.SCHEMATA WHERE BINARY SCHEMA_NAME = " + literal + ";"); err != nil {
		return err
	} else if taken {
		return fmt.Errorf("database %s: %w", name, ErrDatabaseNameTaken)
	}
	sql := fmt.Sprintf("CREATE DATABASE %s CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;", ident)
	return d.executeSQL(sql)
}

// countIsNonZero runs a statement that answers one "n=<count>" line.
func (d *MariaDBDriver) countIsNonZero(statement string) (bool, error) {
	output, err := d.runSQL(statement)
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(string(output), "\n") {
		if count, ok := strings.CutPrefix(strings.TrimSpace(line), "n="); ok {
			return count != "0", nil
		}
	}
	return false, fmt.Errorf("the MariaDB answer about the name could not be read")
}

// DeleteDatabase drops a MariaDB database
func (d *MariaDBDriver) DeleteDatabase(name string) error {
	ident, err := QuoteMySQLIdentifier(name)
	if err != nil {
		return fmt.Errorf("invalid database name: %w", err)
	}
	sql := fmt.Sprintf("DROP DATABASE IF EXISTS %s;", ident)
	return d.executeSQL(sql)
}

// ListDatabases lists all MariaDB databases
func (d *MariaDBDriver) ListDatabases() ([]string, error) {
	output, err := d.runSQL(`SHOW DATABASES;`)
	if err != nil {
		return nil, fmt.Errorf("failed to list databases: %v", err)
	}

	var databases []string
	lines := strings.Split(string(output), "\n")
	for i, line := range lines {
		if i == 0 {
			continue // Skip header
		}
		line = strings.TrimSpace(line)
		if line != "" && line != "information_schema" && line != "performance_schema" && line != "mysql" {
			databases = append(databases, line)
		}
	}
	return databases, nil
}

// CreateUser creates a MariaDB user (localhost only); a name that exists
// already is ErrDatabaseNameTaken.
func (d *MariaDBDriver) CreateUser(username, password string) error {
	// The user name is a MySQL account name literal here, not a backtick
	// identifier, so it is validated as an identifier and then quoted as a
	// string literal.
	// Kullanıcı adı burada ters tırnaklı tanımlayıcı değil, MySQL hesap adı
	// literalidir; bu yüzden tanımlayıcı olarak doğrulanıp string literali
	// olarak tırnaklanır.
	if err := ValidateSQLIdentifier(username); err != nil {
		return fmt.Errorf("invalid username: %w", err)
	}
	userLiteral, err := QuoteMySQLStringLiteral(username)
	if err != nil {
		return fmt.Errorf("invalid username: %w", err)
	}
	pwLiteral, err := QuoteMySQLStringLiteral(password)
	if err != nil {
		return fmt.Errorf("invalid password: %w", err)
	}
	// alpha.129: exclusive, for the same reason as CreateDatabase: an
	// existing account of this name (on any host) is never taken over.
	if taken, err := d.countIsNonZero("SELECT CONCAT('n=', COUNT(*)) FROM mysql.user WHERE User = " + userLiteral + ";"); err != nil {
		return err
	} else if taken {
		return fmt.Errorf("database user %s: %w", username, ErrDatabaseNameTaken)
	}
	// alpha.134 fix round: CREATE USER is the last statement, so an error
	// here means this call made no account, and the caller's clean-up is
	// right to leave the name alone. It used to be followed by FLUSH
	// PRIVILEGES, which an account statement does not need (the server
	// reloads its own tables; FLUSH is for edits made to the grant tables
	// directly) and which a registered account without RELOAD is refused
	// (ERROR 1227): the account then stood, the create was reported failed
	// and its clean-up left the account behind (alpha.133 lab, P.6).
	sql := fmt.Sprintf("CREATE USER %s@'localhost' IDENTIFIED BY %s;", userLiteral, pwLiteral)
	return d.executeSQL(sql)
}

// DeleteUser drops a MariaDB user
func (d *MariaDBDriver) DeleteUser(username string) error {
	if err := ValidateSQLIdentifier(username); err != nil {
		return fmt.Errorf("invalid username: %w", err)
	}
	userLiteral, err := QuoteMySQLStringLiteral(username)
	if err != nil {
		return fmt.Errorf("invalid username: %w", err)
	}
	sql := fmt.Sprintf("DROP USER IF EXISTS %s@'localhost';", userLiteral)
	if err := d.executeSQL(sql); err != nil {
		return err
	}
	// alpha.129: the user's remote copies go with it.
	if err := d.forEachRemoteCopy(userLiteral, func(account string) string {
		return "DROP USER IF EXISTS " + account + ";"
	}); err != nil {
		return err
	}
	return d.executeSQL("FLUSH PRIVILEGES;")
}

// ChangePassword changes a MariaDB user's password
func (d *MariaDBDriver) ChangePassword(username, newPassword string) error {
	if err := ValidateSQLIdentifier(username); err != nil {
		return fmt.Errorf("invalid username: %w", err)
	}
	userLiteral, err := QuoteMySQLStringLiteral(username)
	if err != nil {
		return fmt.Errorf("invalid username: %w", err)
	}
	pwLiteral, err := QuoteMySQLStringLiteral(newPassword)
	if err != nil {
		return fmt.Errorf("invalid password: %w", err)
	}
	sql := fmt.Sprintf("ALTER USER %s@'localhost' IDENTIFIED BY %s;", userLiteral, pwLiteral)
	if err := d.executeSQL(sql); err != nil {
		return err
	}
	// alpha.129: the remote copies take the same password (their REQUIRE
	// SSL stays), so an old password never keeps working from outside.
	if err := d.forEachRemoteCopy(userLiteral, func(account string) string {
		return "ALTER USER " + account + " IDENTIFIED BY " + pwLiteral + ";"
	}); err != nil {
		return err
	}
	return d.executeSQL("FLUSH PRIVILEGES;")
}

// ListUsers lists all MariaDB users
func (d *MariaDBDriver) ListUsers() ([]string, error) {
	output, err := d.runSQL(`SELECT User FROM mysql.user WHERE Host='localhost';`)
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %v", err)
	}

	var users []string
	lines := strings.Split(string(output), "\n")
	for i, line := range lines {
		if i == 0 {
			continue // Skip header
		}
		line = strings.TrimSpace(line)
		if line != "" && line != "root" && line != "mysql.sys" {
			users = append(users, line)
		}
	}
	return users, nil
}

// GrantPrivileges grants privileges to a user on a database. alpha.134: by
// the database's exact name (`shop\_a`.*): written as `shop_a`.* the grant
// is a pattern in which `_` stands for any one character
// (internal/mariadbgrant).
func (d *MariaDBDriver) GrantPrivileges(database, user, privileges string) error {
	if _, err := QuoteMySQLIdentifier(database); err != nil {
		return fmt.Errorf("invalid database name: %w", err)
	}
	// The exact form where it fits a grant row; for an existing database
	// whose exact form does not fit, the pattern form it always had (a new
	// one of such a name was refused by CreateDatabase).
	dbIdent, stored, err := mariadbgrant.GrantTarget(database)
	if err != nil {
		return fmt.Errorf("invalid database name: %w", err)
	}
	if err := ValidateSQLIdentifier(user); err != nil {
		return fmt.Errorf("invalid username: %w", err)
	}
	userLiteral, err := QuoteMySQLStringLiteral(user)
	if err != nil {
		return fmt.Errorf("invalid username: %w", err)
	}

	privClause := "ALL PRIVILEGES"
	if privileges != "ALL" {
		privClause, err = ValidatePrivileges(privileges)
		if err != nil {
			return fmt.Errorf("invalid privileges: %w", err)
		}
	}
	// alpha.134 review: NO_AUTO_CREATE_USER in the session on MariaDB (a
	// GRANT never re-creates an account dropped meanwhile), and the stored row
	// is read back: it must hold exactly the exact form.
	prefix, err := mariadbgrant.GrantPrefix(d.runSQL)
	if err != nil {
		return err
	}
	sql := fmt.Sprintf("%sGRANT %s ON %s.* TO %s@'localhost';", prefix, privClause, dbIdent, userLiteral)
	if err := d.executeSQL(sql); err != nil {
		return err
	}
	if err := d.executeSQL("FLUSH PRIVILEGES;"); err != nil {
		return err
	}
	// Follow-up: an account that may not read mysql.db (a registered server's
	// restricted account) cannot read the right back. That is a verified
	// answer, not a failure: the write stands on the length rule above, and
	// the caller records once that rights here cannot be verified
	// (GrantReadBack). A read that failed otherwise stays unknown.
	d.grantPattern = stored == database && mariadbgrant.Exact(database) != database
	switch err := mariadbgrant.VerifyStoredForm(d.runSQL, user, "localhost", stored); {
	case errors.Is(err, mariadbgrant.ErrUnverifiable):
		d.grantReadBack = GrantReadBackUnverifiable
		return nil
	case err != nil:
		return err
	}
	d.grantReadBack = GrantReadBackVerified
	return nil
}

// What the last GrantPrivileges could read back (alpha.134 follow-up).
const (
	GrantReadBackVerified     = "verified"
	GrantReadBackUnverifiable = "unverifiable"
)

// GrantReadBacker is a driver that says what its last grant's read-back
// showed: "" (none made), GrantReadBackVerified or GrantReadBackUnverifiable.
type GrantReadBacker interface {
	GrantReadBack() string
	// GrantWasPattern: the last grant was written in the pattern form (an
	// existing database whose exact form does not fit a grant row).
	GrantWasPattern() bool
}

// GrantReadBack implements GrantReadBacker.
func (d *MariaDBDriver) GrantReadBack() string { return d.grantReadBack }

// GrantWasPattern implements GrantReadBacker.
func (d *MariaDBDriver) GrantWasPattern() bool { return d.grantPattern }

// RevokePrivileges revokes privileges from a user on a database. alpha.134:
// the grant may be held in the exact form (written since alpha.134) or the
// pattern form (written before); each form the account holds is taken away.
func (d *MariaDBDriver) RevokePrivileges(database, user string) error {
	if _, err := QuoteMySQLIdentifier(database); err != nil {
		return fmt.Errorf("invalid database name: %w", err)
	}
	if err := ValidateSQLIdentifier(user); err != nil {
		return fmt.Errorf("invalid username: %w", err)
	}
	userLiteral, err := QuoteMySQLStringLiteral(user)
	if err != nil {
		return fmt.Errorf("invalid username: %w", err)
	}
	if err := d.revokeDatabaseGrantForms(database, user, userLiteral); err != nil {
		return err
	}
	// alpha.129: a remote copy loses every privilege (closed, never more
	// than the local account); the next remote access change gives back
	// what the local account still holds.
	if err := d.forEachRemoteCopy(userLiteral, func(account string) string {
		return "REVOKE ALL PRIVILEGES, GRANT OPTION FROM " + account + ";"
	}); err != nil {
		return err
	}
	return d.executeSQL("FLUSH PRIVILEGES;")
}

// revokeDatabaseGrantForms takes user@localhost's grant on the database away
// in each form the account holds. When the grant table cannot be read with
// the record's account, both forms are revoked one by one and it is enough
// that one of them was there.
func (d *MariaDBDriver) revokeDatabaseGrantForms(database, user, userLiteral string) error {
	pattern, _ := mariadbgrant.PatternTarget(database)
	targets := map[string]string{database: pattern}
	candidates := []string{pattern}
	// A name whose exact form does not fit a grant row only ever has the
	// pattern form (mariadbgrant.ErrExactTooLong).
	if exact, err := mariadbgrant.Target(database); err == nil && exact != pattern {
		targets[mariadbgrant.Exact(database)] = exact
		candidates = []string{exact, pattern}
	}
	output, readErr := d.runSQL("SELECT CONCAT('f=', HEX(Db)) FROM mysql.db WHERE User = " + mariadbgrant.Hex(user) +
		" AND Host = 'localhost' AND Db IN (" + mariadbgrant.HexList(mariadbgrant.Forms(database)) + ");")
	if readErr == nil {
		for _, value := range mariadbgrant.Markers(output, "f") {
			stored, err := mariadbgrant.FromHex(value)
			if err != nil {
				return err
			}
			target, ok := targets[stored]
			if !ok {
				continue
			}
			if err := d.executeSQL(fmt.Sprintf("REVOKE ALL PRIVILEGES ON %s.* FROM %s@'localhost';", target, userLiteral)); err != nil {
				return err
			}
		}
		return nil
	}
	var firstErr error
	revoked := false
	for _, target := range candidates {
		if err := d.executeSQL(fmt.Sprintf("REVOKE ALL PRIVILEGES ON %s.* FROM %s@'localhost';", target, userLiteral)); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		revoked = true
	}
	if !revoked {
		return firstErr
	}
	return nil
}

// forEachRemoteCopy runs one statement for every remote copy of a user
// (docs/DATABASE-REMOTE-ACCESS.md): an account of the same name whose host
// is an address or an IPv4 network, the forms CelikPanel writes. Hosts in any
// other form (a wildcard, a name) are never touched.
// forEachRemoteCopy, kullanıcının her uzak kopyası için bir ifade çalıştırır.
func (d *MariaDBDriver) forEachRemoteCopy(userLiteral string, statement func(account string) string) error {
	output, err := d.runSQL("SELECT Host FROM mysql.user WHERE User = " + userLiteral + " AND Host <> 'localhost';")
	if err != nil {
		return fmt.Errorf("list the remote copies: %w", err)
	}
	var batch []string
	for i, line := range strings.Split(string(output), "\n") {
		host := strings.TrimSpace(line)
		if i == 0 || host == "" || !dbremote.IsRemoteAccountHost(host) {
			continue // the header, or a host CelikPanel does not write
		}
		hostLiteral, err := QuoteMySQLStringLiteral(host)
		if err != nil {
			continue
		}
		batch = append(batch, statement(userLiteral+"@"+hostLiteral))
	}
	if len(batch) == 0 {
		return nil
	}
	return d.executeSQL(strings.Join(batch, "\n"))
}

// executeSQL executes SQL command via mysql CLI
func (d *MariaDBDriver) executeSQL(sql string) error {
	_, err := d.runSQL(sql)
	return err
}

// GrantExec (alpha.134) runs statements for internal/mariadbgrant with the
// record's own account: the rewrite of pattern-form grants on a registered
// MariaDB (cmd/panel database_grant_forms.go).
func (d *MariaDBDriver) GrantExec() mariadbgrant.Exec { return d.runSQL }

// RemoveRightsOnDroppedDatabase (alpha.134) takes every right that names a
// dropped database from every grantee on this server, with the record's own
// account (mariadbgrant.RemoveDroppedRights), and answers whose rights went.
// An account that may not read or change the grant tables gets an error;
// the caller then says rights may remain there.
// Silinen veritabanını adlandıran haklar, kaydın kendi hesabıyla kaldırılır.
func (d *MariaDBDriver) RemoveRightsOnDroppedDatabase(name string, owners ...string) (mariadbgrant.DroppedResult, error) {
	return mariadbgrant.RemoveDroppedRights(d.runSQL, name, owners...)
}
