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

// Target is the database part of `GRANT … ON <target>.*` in the exact form,
// in backticks.
func Target(name string) (string, error) {
	if !ValidName(name) {
		return "", ErrName
	}
	return "`" + Exact(name) + "`", nil
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
	if user == "" {
		return "", errors.New("an empty account name")
	}
	return "'" + user + "'@'" + host + "'", nil
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
