// Package mariadbgrant writes and reads MariaDB (and MySQL) database-level
// grants by exact name (alpha.134).
//
// In a database-level grant (`GRANT … ON db.*`) the engine reads the
// database name as a pattern: `_` stands for any one character and `%` for
// any run of characters, unless they are written `\_` and `\%`. MySQL's
// manual says so (Reference Manual 8.0, GRANT, "Object Quoting Guidelines":
// "to use a _ character as part of a database name, specify it using the \
// escape character as \_ in the GRANT statement, to prevent the user from
// being able to access additional databases matching the wildcard pattern");
// MariaDB's documentation only says that database names in GRANT "can
// contain % and _ wildcards", and the alpha.133 lab measured it on MariaDB
// 11.8.6 and 10.11.14: a grant on `labd_1__wp`.* let its user create a
// table in labd_12_wp, and `labd\_x`.* is stored with its backslash. Table,
// column and routine grants name the database literally (an escaped name
// there is refused, ERROR 1146), so the exact form is for `.*` grants only.
//
// Every name CelikPanel makes contains `_`, so every grant it wrote before
// alpha.134 is such a pattern ("pattern form"); from alpha.134 it writes the
// escaped "exact form". Both forms name the database and both are handled
// wherever a right is taken away or counted, for as long as pattern rows can
// exist.
//
// Values are compared through hexadecimal literals (X'…') and read back with
// HEX(): the answer does not depend on the server's sql_mode
// (NO_BACKSLASH_ESCAPES) or on whether the client escapes backslashes in its
// output. Inside backticks a backslash is always literal, so the exact form
// of a GRANT target does not depend on sql_mode either.
//
// Veritabanı düzeyindeki hakta ad bir kalıptır (`_` herhangi bir karakter);
// alpha.134'ten itibaren hak tam adla (`\_`) yazılır, iki biçim de her
// kaldırma ve sayımda ele alınır.
package mariadbgrant

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// maxNameLen is the longest database name the engines take (MariaDB and
// MySQL: 64).
const maxNameLen = 64

// ErrName: the name is not one CelikPanel writes.
var ErrName = errors.New("not a database name CelikPanel writes")

// ValidName accepts the names CelikPanel makes: letters, digits and `_`,
// not starting with a digit, at most 64 characters. Such a name never holds
// `%`, `\` or a backtick.
func ValidName(name string) bool {
	if name == "" || len(name) > maxNameLen {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		letter := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
		digit := c >= '0' && c <= '9'
		if !letter && c != '_' && !(digit && i > 0) {
			return false
		}
	}
	return true
}

// Exact is the exact form of a database name as a grant stores it: every
// `_` and `%` escaped with a backslash.
func Exact(name string) string {
	return strings.NewReplacer(`_`, `\_`, `%`, `\%`).Replace(name)
}

// Forms are the two ways a database-level grant can name this database:
// as written (a pattern when the name holds `_`) and exact. A name without
// `_` has one form.
func Forms(name string) []string {
	if exact := Exact(name); exact != name {
		return []string{name, exact}
	}
	return []string{name}
}

// maxStoredLen is what mysql.db.Db holds (char(64)).
const maxStoredLen = 64

// ErrExactTooLong: the name's exact form (each `_` written `\_`) is longer
// than the 64 characters a grant row stores. Measured on MariaDB 11.8.6
// (alpha.134 review): the GRANT is accepted, SHOW GRANTS shows the whole
// exact name, but mysql.db.Db holds it cut to 64 characters - after the next
// reload that is a grant on ANOTHER (shorter) name. Such a name is never
// written in the exact form: a new database of such a name is refused, an
// existing one keeps its pattern-form grants and is listed.
var ErrExactTooLong = errors.New("the database name is too long for a grant by its exact name")

// ExactFits reports whether the exact form of the name fits a grant row.
func ExactFits(name string) bool { return len(Exact(name)) <= maxStoredLen }

// Target is the database part of `GRANT … ON <target>.*` in the exact form,
// in backticks.
func Target(name string) (string, error) {
	if !ValidName(name) {
		return "", ErrName
	}
	if !ExactFits(name) {
		return "", ErrExactTooLong
	}
	return "`" + Exact(name) + "`", nil
}

// NoAutoCreateUser is put before every GRANT on MariaDB: without
// NO_AUTO_CREATE_USER in the session a GRANT to an account that was dropped
// meanwhile creates it again with an empty password (measured on MariaDB
// 11.8.6: `GRANT SELECT ON nosuchdb.* TO ghost@localhost` under
// sql_mode=STRICT_TRANS_TABLES created ghost@localhost with an empty
// authentication string; with the mode it is refused, ERROR 1133). MariaDB
// 10.11 and 11.8 have the mode in their default sql_mode; MySQL 8 removed it
// (and never creates an account by GRANT), so it is for MariaDB only.
const NoAutoCreateUser = "SET SESSION sql_mode = CONCAT(@@SESSION.sql_mode, ',NO_AUTO_CREATE_USER');"

// GrantPrefix is what goes before a GRANT on this server: NoAutoCreateUser
// on MariaDB, nothing on another engine (MySQL 8 refuses the mode and never
// creates an account by GRANT).
func GrantPrefix(run Exec) (string, error) {
	out, err := run("SELECT CONCAT('v=', VERSION());")
	if err != nil {
		return "", fmt.Errorf("the database server's version could not be read: %w", err)
	}
	version, ok := Marker(out, "v")
	if !ok {
		return "", errors.New("the database server's version could not be read")
	}
	if strings.Contains(strings.ToLower(version), "mariadb") {
		return NoAutoCreateUser + "\n", nil
	}
	return "", nil
}

// StoredStatement answers "n=<count>" of grant rows of the account that
// hold exactly the name's exact form: every GRANT is read back with it.
func StoredStatement(user, host, name string) string {
	return "SELECT CONCAT('n=', COUNT(*)) FROM mysql.db WHERE User = " + Hex(user) + " AND Host = " + Hex(host) + " AND Db = " + Hex(Exact(name)) + ";"
}

// ErrNotStored: a GRANT was answered, but no grant row holds exactly the
// intended exact form.
var ErrNotStored = errors.New("the database server did not store the right by the database's exact name")

// VerifyStored reads a GRANT back: one row of the account must hold exactly
// the name's exact form.
func VerifyStored(run Exec, user, host, name string) error {
	n, err := countMarker(run, StoredStatement(user, host, name), "n", "whether the right was stored by its exact name")
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrNotStored
	}
	return nil
}

// PatternTarget is the same part in the pattern form (as CelikPanel wrote it
// before alpha.134): only for taking such a grant away.
func PatternTarget(name string) (string, error) {
	if !ValidName(name) {
		return "", ErrName
	}
	return "`" + name + "`", nil
}

// Hex is value as a hexadecimal string literal; the empty value is ”.
func Hex(value string) string {
	if value == "" {
		return "''"
	}
	return "X'" + strings.ToUpper(hex.EncodeToString([]byte(value))) + "'"
}

// HexList joins Hex of each value for IN (…).
func HexList(values []string) string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, Hex(value))
	}
	return strings.Join(out, ", ")
}

// FromHex decodes what HEX() answered.
func FromHex(value string) (string, error) {
	raw, err := hex.DecodeString(strings.TrimSpace(value))
	if err != nil {
		return "", fmt.Errorf("an answer of the database server could not be read")
	}
	return string(raw), nil
}

// Decode reads the database name a grant row stores. ok is false when the
// row is not one database name in either form (a wildcard on purpose such
// as `shop\_%`, or a character CelikPanel never writes); pattern tells which
// form it is (a name without `_` is exact either way).
func Decode(stored string) (name string, pattern bool, ok bool) {
	if ValidName(stored) {
		return stored, strings.Contains(stored, "_"), true
	}
	var b strings.Builder
	for i := 0; i < len(stored); i++ {
		c := stored[i]
		if c == '\\' && i+1 < len(stored) && stored[i+1] == '_' {
			b.WriteByte('_')
			i++
			continue
		}
		b.WriteByte(c)
	}
	name = b.String()
	if !ValidName(name) || Exact(name) != stored {
		return "", false, false
	}
	return name, false, true
}

// HasWildcard reports whether a stored database-level grant name holds an
// unescaped `_` or `%`, i.e. may cover more than one database.
func HasWildcard(stored string) bool {
	for i := 0; i < len(stored); i++ {
		switch stored[i] {
		case '\\':
			i++
		case '_', '%':
			return true
		}
	}
	return false
}

// Matches reports whether the stored grant name covers the database name,
// the way the engine compares them: `_` any one character, `%` any run,
// `\` takes the next character literally. foldCase is for a server with
// lower_case_table_names other than 0.
func Matches(stored, name string, foldCase bool) bool {
	if foldCase {
		stored, name = strings.ToLower(stored), strings.ToLower(name)
	}
	return wildMatch(stored, name)
}

func wildMatch(pattern, name string) bool {
	for len(pattern) > 0 {
		switch pattern[0] {
		case '%':
			for len(pattern) > 0 && pattern[0] == '%' {
				pattern = pattern[1:]
			}
			if pattern == "" {
				return true
			}
			for i := 0; i <= len(name); i++ {
				if wildMatch(pattern, name[i:]) {
					return true
				}
			}
			return false
		case '_':
			if name == "" {
				return false
			}
			pattern, name = pattern[1:], name[1:]
		case '\\':
			if len(pattern) > 1 {
				pattern = pattern[1:]
			}
			fallthrough
		default:
			if name == "" || name[0] != pattern[0] {
				return false
			}
			pattern, name = pattern[1:], name[1:]
		}
	}
	return name == ""
}

// AccountLiteral is 'user'@'host' for a user and host that need no escaping
// (no quote and no backslash): what CelikPanel writes and the hosts it
// converts. Anything else is refused rather than escaped, so the literal
// means the same under every sql_mode.
func AccountLiteral(user, host string) (string, error) {
	for _, part := range []string{user, host} {
		if strings.ContainsAny(part, "'\\\x00\n\r") {
			return "", errors.New("an account name or host CelikPanel does not write")
		}
	}
	if user == "" && host == "" {
		return "", errors.New("an empty account name")
	}
	return "'" + user + "'@'" + host + "'", nil
}

// GranteeLiteral is how a statement names the holder of a grant row: a role
// (an empty host) by its name alone, the anonymous account (an empty user)
// as ”@'host', any other account as 'user'@'host'. Only for statements
// shown to an administrator; CelikPanel converts accounts only.
func GranteeLiteral(user, host string) (string, error) {
	if host == "" && user != "" {
		if strings.ContainsAny(user, "'\\\x00\n\r") {
			return "", errors.New("an account name or host CelikPanel does not write")
		}
		return "'" + user + "'", nil
	}
	return AccountLiteral(user, host)
}

// Markers reads every "name=value" line an answer carries for name.
func Markers(output []byte, name string) []string {
	var out []string
	for _, line := range strings.Split(string(output), "\n") {
		if value, ok := strings.CutPrefix(strings.TrimSpace(line), name+"="); ok {
			out = append(out, value)
		}
	}
	return out
}

// Marker reads the one "name=value" line of an answer.
func Marker(output []byte, name string) (string, bool) {
	values := Markers(output, name)
	if len(values) != 1 {
		return "", false
	}
	return values[0], true
}
