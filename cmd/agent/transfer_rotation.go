package main

import (
	"errors"
	"fmt"
	"log"
	"os/exec"
	"regexp"
	"strings"
	"unicode"

	"github.com/alicelik/celikpanel/internal/hostcmd"
	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Credential rotation after a subscription transfer (alpha.117, D-042
// section 8). Two calls: a database user's new password (engine, open
// sessions, and the wp-config.php CelikPanel's installer wrote), and the end
// of a mailbox's open sessions after its password changed. The password is
// passed on stdin or written to a root-checked file; it is never logged or
// returned, and an engine's answer to the statement that carries it is not
// forwarded.
//
// Devirden sonra veritabanı kullanıcısının yeni parolası ve posta kutusunun
// açık oturumlarının kapatılması. Parola günlüğe yazılmaz, geri gönderilmez.

// rotationPasswordValid: printable text on one line, within the bounds.
func rotationPasswordValid(password string) error {
	if len(password) < transport.MinDatabaseUserPasswordBytes || len(password) > transport.MaxDatabaseUserPasswordBytes {
		return fmt.Errorf("the password must be %d to %d bytes", transport.MinDatabaseUserPasswordBytes, transport.MaxDatabaseUserPasswordBytes)
	}
	for _, r := range password {
		if r == unicode.ReplacementChar || unicode.IsControl(r) {
			return errors.New("the password must be printable text on one line")
		}
	}
	return nil
}

// rotationPasswordStatement is the statement that sets the password. It is
// passed on stdin; its failure text is never forwarded.
//
// alpha.129: MariaDB sets it on every host the account has (hosts, read by
// rotationMySQLHostsStatement; CelikPanel creates 'name'@'localhost' only,
// but an owner who added 'name'@'127.0.0.1' or another host keeps one
// password for all of them). PostgreSQL stores it as a SCRAM-SHA-256 verifier
// whatever the server's password_encryption says: the SET is local to this
// psql session, and an md5 pg_hba line still accepts a SCRAM verifier.
//
// alpha.129: MariaDB'de hesabın her makine adına; PostgreSQL'de SCRAM.
func rotationPasswordStatement(engine, username, password string, hosts []string) (string, error) {
	switch engine {
	case "mysql":
		user, err := services.QuoteMySQLStringLiteral(username)
		if err != nil {
			return "", err
		}
		secret, err := services.QuoteMySQLStringLiteral(password)
		if err != nil {
			return "", errors.New("the password cannot be written as a MariaDB string")
		}
		if len(hosts) == 0 {
			hosts = []string{"localhost"}
		}
		accounts := make([]string, 0, len(hosts))
		for _, host := range hosts {
			literal, err := services.QuoteMySQLStringLiteral(host)
			if err != nil {
				return "", fmt.Errorf("the account's host %q cannot be written as a MariaDB string", host)
			}
			accounts = append(accounts, user+"@"+literal+" IDENTIFIED BY "+secret)
		}
		return "ALTER USER " + strings.Join(accounts, ", ") + ";", nil
	case "postgresql":
		ident, err := services.QuotePGIdentifier(username)
		if err != nil {
			return "", err
		}
		secret, err := services.QuotePGStringLiteral(password)
		if err != nil {
			return "", errors.New("the password cannot be written as a PostgreSQL string")
		}
		return "SET password_encryption = 'scram-sha-256'; ALTER ROLE " + ident + " WITH PASSWORD " + secret + ";", nil
	}
	return "", fmt.Errorf("unknown database engine %q; expected mysql or postgresql", engine)
}

// rotationMySQLHostsStatement lists every host of a MariaDB account name, one
// "host=" line each.
func rotationMySQLHostsStatement(username string) (string, error) {
	literal, err := services.QuoteMySQLStringLiteral(username)
	if err != nil {
		return "", err
	}
	return "SELECT CONCAT('host=', Host) FROM mysql.user WHERE User = " + literal + " ORDER BY Host;", nil
}

// parseRotationMySQLHosts reads the "host=" lines; anything else (a client
// warning) is ignored.
func parseRotationMySQLHosts(output []byte) []string {
	var hosts []string
	for _, line := range strings.Fields(string(output)) {
		if host, ok := strings.CutPrefix(strings.TrimSpace(line), "host="); ok && host != "" {
			hosts = append(hosts, host)
		}
	}
	return hosts
}

func validDatabaseUserApp(app *transport.DatabaseUserApp, username string) error {
	if app.Kind != "wordpress" {
		if _, ok := appConfigFormatFor(app.Kind); !ok {
			return fmt.Errorf("unknown application %q", app.Kind)
		}
	}
	if app.SubscriptionID <= 0 || app.DomainID <= 0 {
		return errors.New("the application's site identity is required")
	}
	if strings.TrimSpace(app.SiteUsername) == "" || strings.ContainsAny(app.SiteUsername, "/\x00") {
		return errors.New("the application's site user is required")
	}
	if err := services.ValidateSQLIdentifier(app.DBName); err != nil {
		return fmt.Errorf("invalid application database name: %w", err)
	}
	if username == "" {
		return errors.New("the database user is required")
	}
	return nil
}

// RotateDatabaseUserPasswordV1 gives one local database user a new password,
// ends its open sessions and, with App, writes the password into the
// application's configuration file. A required App whose file does not name
// this database and user any more changes nothing.
func (a *Agent) RotateDatabaseUserPasswordV1(
	req *transport.RotateDatabaseUserPasswordRequest,
	resp *transport.RotateDatabaseUserPasswordResponse,
) error {
	if req == nil || resp == nil {
		return errors.New("database password request and response are required")
	}
	*resp = transport.RotateDatabaseUserPasswordResponse{}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "changing a database user's password"); err != nil {
		resp.Error = err.Error()
		return nil
	}
	if err := rotationPasswordValid(req.NewPassword); err != nil {
		resp.Error = err.Error() + "; nothing was changed"
		return nil
	}
	run, statements, err := prepareSuspensionDatabaseUser(req.User)
	if err != nil {
		resp.Error = err.Error() + "; nothing was changed"
		return nil
	}
	// The statement is composed after the account was read (MariaDB: its hosts).
	if _, err := rotationPasswordStatement(req.User.Engine, req.User.Username, req.NewPassword, nil); err != nil {
		resp.Error = err.Error() + "; nothing was changed"
		return nil
	}

	suspensionDatabaseMutex.Lock()
	defer suspensionDatabaseMutex.Unlock()

	// The application's file first (read only): a required file that does
	// not match stops before the engine changes.
	var config *wordpressConfigFile
	if req.App != nil {
		if err := validDatabaseUserApp(req.App, req.User.Username); err != nil {
			resp.Error = err.Error() + "; nothing was changed"
			return nil
		}
		found, reason := openWordPressConfigForRotation(*req.App, req.User.Username)
		switch {
		case found != nil:
			config = found
			defer config.close()
		case req.App.Required:
			resp.AppConflict = true
			resp.AppDetail = reason
			resp.Error = reason + "; CelikPanel changed nothing, so the site keeps working"
			return nil
		default:
			resp.AppDetail = reason
		}
	}

	current, err := readSuspensionDatabaseUserState(run, statements)
	if err != nil {
		resp.Error = err.Error() + "; nothing was changed"
		return nil
	}
	if !current.Present {
		resp.Absent = true
		resp.Error = fmt.Sprintf("database user is absent: %s does not exist on this server's %s engine, so nothing was changed", req.User.Username, req.User.Engine)
		return nil
	}
	var hosts []string
	if req.User.Engine == "mysql" {
		listHosts, err := rotationMySQLHostsStatement(req.User.Username)
		if err != nil {
			resp.Error = err.Error() + "; nothing was changed"
			return nil
		}
		output, runErr := run(listHosts)
		if runErr != nil {
			resp.Error = hostcmd.Fail("the database account's hosts could not be read; nothing was changed", output, runErr, databaseClientMeaning).Error()
			return nil
		}
		hosts = parseRotationMySQLHosts(output)
	}
	setPassword, err := rotationPasswordStatement(req.User.Engine, req.User.Username, req.NewPassword, hosts)
	if err != nil {
		resp.Error = err.Error() + "; nothing was changed"
		return nil
	}
	if _, runErr := run(setPassword); runErr != nil {
		// The engine's words may quote the statement: never forwarded.
		resp.Error = "the database engine did not accept the new password for " + req.User.Username + "; the old password may still be in effect"
		return nil
	}
	resp.PasswordChanged = true
	// alpha.129: CelikPanel's remote copies of a MariaDB account
	// (docs/DATABASE-REMOTE-ACCESS.md) follow the new password, or go.
	if req.User.Engine == "mysql" {
		if err := databaseRemoteFollowMariaDBUser(req.User.Username, false); err != nil {
			log.Printf("database password change: %v", err)
		}
	}
	if output, runErr := run(statements.disconnect); runErr != nil {
		resp.Error = hostcmd.Fail("the password changed but the account's open sessions could not be ended", output, runErr, databaseClientMeaning).Error()
		return nil
	}
	resp.SessionsEnded = true
	if config != nil {
		if err := config.writePassword(req.NewPassword); err != nil {
			resp.Error = "the password changed but the application's configuration file could not be updated (" + err.Error() + "); the site cannot reach its database until it is"
			return nil
		}
		resp.AppUpdated = true
	}
	resp.Applied = true
	return nil
}

// EndMailSessionsV1 ends a mailbox's open IMAP and POP3 sessions (doveadm
// kick). Nobody signed in is a success.
func (a *Agent) EndMailSessionsV1(req *transport.EndMailSessionsRequest, resp *transport.EndMailSessionsResponse) error {
	if req == nil || resp == nil {
		return errors.New("mail session request and response are required")
	}
	*resp = transport.EndMailSessionsResponse{}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "ending a mailbox's sessions"); err != nil {
		resp.Error = err.Error()
		return nil
	}
	email, _, _, err := canonicalAgentMailbox(req.Email)
	if err != nil {
		resp.Error = "invalid mailbox address"
		return nil
	}
	output, err := runMailTLSCommand("doveadm", "kick", email)
	if err == nil {
		resp.Applied, resp.Ended = true, true
		return nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && doveadmNobodyKicked(exit.ExitCode()) {
		resp.Applied = true
		return nil
	}
	resp.Error = hostcmd.Fail("Dovecot could not end the mailbox's open sessions", output, err, nil).Error()
	return nil
}

// doveadmNobodyKicked: doveadm's "not found" exit codes (EX_NOUSER,
// EX_NOHOST): nobody of that name was signed in.
func doveadmNobodyKicked(code int) bool { return code == 67 || code == 68 }

// ---- wp-config.php ----

// wordpressDefinePattern matches one define( 'NAME', 'value' ); with a
// single-quoted PHP value, as the installer writes it.
var wordpressDefinePattern = regexp.MustCompile(`define\(\s*['"](DB_NAME|DB_USER|DB_PASSWORD)['"]\s*,\s*'((?:[^'\\]|\\.)*)'\s*\)\s*;`)

// wordpressDBDefines reads DB_NAME, DB_USER and the span of DB_PASSWORD's
// value. Each must appear exactly once.
func wordpressDBDefines(content []byte) (name, user string, passStart, passEnd int, err error) {
	matches := wordpressDefinePattern.FindAllSubmatchIndex(content, -1)
	seen := map[string]int{}
	for _, m := range matches {
		key := string(content[m[2]:m[3]])
		seen[key]++
		value := phpUnquote(string(content[m[4]:m[5]]))
		switch key {
		case "DB_NAME":
			name = value
		case "DB_USER":
			user = value
		case "DB_PASSWORD":
			passStart, passEnd = m[4]-1, m[5]+1
		}
	}
	for _, key := range []string{"DB_NAME", "DB_USER", "DB_PASSWORD"} {
		if seen[key] != 1 {
			return "", "", 0, 0, fmt.Errorf("wp-config.php does not have exactly one %s line in the form CelikPanel wrote", key)
		}
	}
	return name, user, passStart, passEnd, nil
}

func phpUnquote(value string) string {
	var b strings.Builder
	for i := 0; i < len(value); i++ {
		if value[i] == '\\' && i+1 < len(value) && (value[i+1] == '\\' || value[i+1] == '\'') {
			i++
		}
		b.WriteByte(value[i])
	}
	return b.String()
}

// wordpressConfigMatches says why a file is not the one to update ("" when
// it is): it must name this database and this user.
func wordpressConfigMatches(content []byte, dbName, dbUser string) string {
	name, user, _, _, err := wordpressDBDefines(content)
	switch {
	case err != nil:
		return err.Error()
	case name != dbName || user != dbUser:
		return "wp-config.php no longer uses database " + dbName + " with user " + dbUser
	}
	return ""
}

// replaceWordPressDBPassword writes password as DB_PASSWORD's value and
// changes nothing else.
func replaceWordPressDBPassword(content []byte, password string) ([]byte, error) {
	_, _, start, end, err := wordpressDBDefines(content)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, len(content)+len(password))
	out = append(out, content[:start]...)
	out = append(out, phpQuote(password)...)
	out = append(out, content[end:]...)
	return out, nil
}
