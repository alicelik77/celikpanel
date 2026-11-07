package dbremote

import (
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// A MariaDB account is user@host, so a remote rule needs an account of its
// own: the same name, the local account's plugin and password hash (never the
// password), REQUIRE SSL, and only the grants the local account holds on the
// databases its rules name.
//
// MariaDB hesabı kullanıcı@makinedir; uzak kural kendi hesabını ister: aynı ad,
// yerel hesabın eklentisi ve parola özeti, REQUIRE SSL ve yalnız kuralların
// adlandırdığı veritabanlarındaki yetkiler.

// MariaDBHost is the host part MariaDB matches for a listed address: the
// address itself, or an IPv4 network in MariaDB's netmask form. IPv6 ranges
// never reach here (ParseAddress refuses them for MariaDB).
func MariaDBHost(prefix netip.Prefix) (string, error) {
	addr := prefix.Addr()
	if prefix.IsSingleIP() {
		return addr.String(), nil
	}
	if !addr.Is4() {
		return "", errors.New("MariaDB takes single IPv6 addresses only")
	}
	bits := prefix.Bits()
	mask := uint32(0xffffffff) << (32 - bits)
	return fmt.Sprintf("%s/%d.%d.%d.%d", addr, byte(mask>>24), byte(mask>>16), byte(mask>>8), byte(mask)), nil
}

// MariaDBAccount is one remote account and the databases its rules name.
type MariaDBAccount struct {
	User      string   `json:"user"`
	Host      string   `json:"host"`
	Databases []string `json:"databases"`
}

// ID names the account as MariaDB prints it, for state and messages.
func (a MariaDBAccount) ID() string { return a.User + "@" + a.Host }

// MariaDBAccounts groups a MariaDB target's rules by account.
func MariaDBAccounts(t Target) ([]MariaDBAccount, error) {
	norm, err := t.Normalize()
	if err != nil {
		return nil, err
	}
	if norm.Engine != EngineMySQL {
		return nil, fmt.Errorf("MariaDB accounts are for MariaDB, not %s", norm.Engine)
	}
	byID := map[string]*MariaDBAccount{}
	for _, rule := range norm.Rules {
		prefix, err := netip.ParsePrefix(rule.Address)
		if err != nil {
			return nil, err
		}
		host, err := MariaDBHost(prefix)
		if err != nil {
			return nil, err
		}
		id := rule.User + "@" + host
		account := byID[id]
		if account == nil {
			account = &MariaDBAccount{User: rule.User, Host: host}
			byID[id] = account
		}
		account.Databases = append(account.Databases, rule.Database)
	}
	out := make([]MariaDBAccount, 0, len(byID))
	for _, account := range byID {
		sort.Strings(account.Databases)
		out = append(out, *account)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID() < out[j].ID() })
	return out, nil
}

// mariadbPlugins are the password plugins whose stored value is copied to a
// remote account. Anything else (unix_socket, pam, gssapi) has no password a
// remote client could present, and is refused.
var mariadbPlugins = map[string]bool{"mysql_native_password": true, "ed25519": true, "parsec": true}

var mariadbAuthText = regexp.MustCompile(`^[A-Za-z0-9*+/=.$_-]{1,512}$`)

// MariaDBCredential is the local account's plugin and stored value.
type MariaDBCredential struct {
	Plugin string
	Auth   string
}

// Validate refuses a credential that cannot be copied safely.
func (c MariaDBCredential) Validate() error {
	if !mariadbPlugins[c.Plugin] {
		return fmt.Errorf("the local account signs in with %q, which has no password a remote program could send", c.Plugin)
	}
	if !mariadbAuthText.MatchString(c.Auth) {
		return errors.New("the local account has no password stored in a form CelikPanel can copy; set a new password for this user once")
	}
	return nil
}

func mysqlString(value string) string {
	return "'" + strings.ReplaceAll(strings.ReplaceAll(value, `\`, `\\`), `'`, `\'`) + "'"
}

func mysqlIdent(value string) string { return "`" + strings.ReplaceAll(value, "`", "``") + "`" }

// MariaDBAccountName renders 'user'@'host' after checking both parts.
func MariaDBAccountName(user, host string) (string, error) {
	if err := validName(user); err != nil {
		return "", err
	}
	if !validMariaDBHost(host) {
		return "", fmt.Errorf("%q is not a host CelikPanel writes", host)
	}
	return mysqlString(user) + "@" + mysqlString(host), nil
}

// validMariaDBHost: an address, or an IPv4 network in netmask form, never a
// wildcard or a name.
func validMariaDBHost(host string) bool {
	if addrText, mask, found := strings.Cut(host, "/"); found {
		addr, err := netip.ParseAddr(addrText)
		maskAddr, maskErr := netip.ParseAddr(mask)
		if err != nil || maskErr != nil || !addr.Is4() || !maskAddr.Is4() {
			return false
		}
		m := maskAddr.As4()
		value := uint32(m[0])<<24 | uint32(m[1])<<16 | uint32(m[2])<<8 | uint32(m[3])
		ones := 0
		for value&0x80000000 != 0 {
			ones++
			value <<= 1
		}
		return value == 0 && ones >= MinIPv4Prefix && ones < 32
	}
	addr, err := netip.ParseAddr(host)
	return err == nil && addr.Zone() == "" && !addr.IsLoopback() && !addr.IsUnspecified()
}

// CreateMariaDBAccount: the remote account, with the copied credential and TLS
// required.
func CreateMariaDBAccount(user, host string, credential MariaDBCredential) (string, error) {
	return mariadbIdentified("CREATE USER", user, host, credential)
}

// AlterMariaDBAccount brings an existing remote account's credential and TLS
// requirement back to the local account's.
func AlterMariaDBAccount(user, host string, credential MariaDBCredential) (string, error) {
	return mariadbIdentified("ALTER USER", user, host, credential)
}

func mariadbIdentified(verb, user, host string, credential MariaDBCredential) (string, error) {
	name, err := MariaDBAccountName(user, host)
	if err != nil {
		return "", err
	}
	if err := credential.Validate(); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s %s IDENTIFIED VIA %s USING %s REQUIRE SSL;", verb, name, credential.Plugin, mysqlString(credential.Auth)), nil
}

// DropMariaDBAccount removes a remote account.
func DropMariaDBAccount(user, host string) (string, error) {
	name, err := MariaDBAccountName(user, host)
	if err != nil {
		return "", err
	}
	return "DROP USER IF EXISTS " + name + ";", nil
}

// RevokeAllMariaDB takes every privilege from a remote account before its
// grants are written again.
func RevokeAllMariaDB(user, host string) (string, error) {
	name, err := MariaDBAccountName(user, host)
	if err != nil {
		return "", err
	}
	return "REVOKE ALL PRIVILEGES, GRANT OPTION FROM " + name + ";", nil
}

// GrantMariaDB renders one database grant for a remote account.
func GrantMariaDB(privileges, database, user, host string) (string, error) {
	if _, err := normalizeMariaDBPrivileges(privileges); err != nil {
		return "", err
	}
	if err := validName(database); err != nil {
		return "", err
	}
	name, err := MariaDBAccountName(user, host)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("GRANT %s ON %s.* TO %s;", privileges, mysqlIdent(database), name), nil
}

// mariadbPrivilegeNames: database-level privileges that are copied.
var mariadbPrivilegeNames = map[string]bool{
	"SELECT": true, "INSERT": true, "UPDATE": true, "DELETE": true, "CREATE": true, "DROP": true,
	"REFERENCES": true, "INDEX": true, "ALTER": true, "CREATE TEMPORARY TABLES": true, "LOCK TABLES": true,
	"EXECUTE": true, "CREATE VIEW": true, "SHOW VIEW": true, "CREATE ROUTINE": true, "ALTER ROUTINE": true,
	"EVENT": true, "TRIGGER": true, "DELETE HISTORY": true, "SHOW CREATE ROUTINE": true,
}

func normalizeMariaDBPrivileges(text string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "ALL PRIVILEGES" || text == "ALL" {
		return "ALL PRIVILEGES", nil
	}
	parts := strings.Split(text, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		word := strings.Join(strings.Fields(strings.ToUpper(part)), " ")
		if !mariadbPrivilegeNames[word] {
			return "", fmt.Errorf("privilege %q is not copied to a remote account", word)
		}
		out = append(out, word)
	}
	if len(out) == 0 {
		return "", errors.New("no privilege")
	}
	return strings.Join(out, ", "), nil
}

// LocalGrants is what SHOW GRANTS says about a local account.
type LocalGrants struct {
	// Global is true when the account holds any server-wide privilege other
	// than USAGE: such an account is never reachable from outside.
	Global bool
	// Databases maps a database to its normalized privilege list.
	Databases map[string]string
}

var grantLine = regexp.MustCompile("^GRANT (.+?) ON (\\*|`(?:[^`]|``)+`)\\.\\* TO `((?:[^`]|``)+)`@`((?:[^`]|``)+)`(.*)$")

// ParseMariaDBGrants reads the lines of SHOW GRANTS FOR user@host. Only
// whole-database grants are copied; a table or column grant, a role or a
// proxy grant is not, so a remote account never holds more than the local
// one. The lines may carry the password hash (IDENTIFIED BY PASSWORD): this
// function never returns or quotes them.
func ParseMariaDBGrants(lines []string, user, host string) (LocalGrants, error) {
	out := LocalGrants{Databases: map[string]string{}}
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		match := grantLine.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		if strings.ReplaceAll(match[3], "``", "`") != user || strings.ReplaceAll(match[4], "``", "`") != host {
			continue
		}
		privileges, scope, tail := match[1], match[2], strings.TrimSpace(match[5])
		if scope == "*" {
			if strings.TrimSpace(privileges) != "USAGE" {
				out.Global = true
			}
			continue
		}
		if strings.Contains(privileges, "(") {
			continue // column grants are not copied
		}
		normalized, err := normalizeMariaDBPrivileges(privileges)
		if err != nil {
			return LocalGrants{}, err
		}
		if strings.Contains(tail, "WITH GRANT OPTION") {
			// Passing privileges on is the owner's business, not a remote client's.
			out.Global = true
		}
		database := strings.ReplaceAll(strings.Trim(scope, "`"), "``", "`")
		out.Databases[database] = normalized
	}
	return out, nil
}

// MariaDB option file. Debian and Ubuntu read /etc/mysql/mariadb.conf.d in
// name order (50-server.cnf binds 127.0.0.1), so "90-" comes later; Arch reads
// /etc/my.cnf.d, where CelikPanel's install-time file is
// celikpanel-bind-address.cnf, which sorts before this one.
const (
	MariaDBOptionFileDebian = "/etc/mysql/mariadb.conf.d/90-celikpanel-remote-access.cnf"
	MariaDBOptionFileArch   = "/etc/my.cnf.d/celikpanel-remote-access.cnf"
)

const mariadbOptionHeader = `# CelikPanel: remote database access (docs/DATABASE-REMOTE-ACCESS.md).
# Written while at least one remote access rule exists on this server and
# removed with the last one; MariaDB reads it when it starts. Only addresses
# listed in CelikPanel reach the port (nftables table celikpanel_dbremote) and
# their accounts require TLS. If you change this file, CelikPanel leaves it
# as it is.
[mysqld]
bind-address = *
`

// MariaDBOptionFile renders the option file; cert and key are empty when
// MariaDB already runs TLS of its own.
func MariaDBOptionFile(cert, key string) string {
	text := mariadbOptionHeader
	if cert != "" && key != "" {
		text += "ssl_cert = " + cert + "\nssl_key = " + key + "\n"
	}
	return text
}

// IsOwnMariaDBOptionFile: the file is one CelikPanel wrote, unchanged.
func IsOwnMariaDBOptionFile(content, cert, key string) bool {
	return content == MariaDBOptionFile("", "") || (cert != "" && content == MariaDBOptionFile(cert, key))
}

// Fail2ban. The jail file uses the mysqld-auth filter that Fail2ban ships and
// reads MariaDB's journal. jail.local and *.local files are read after it, so
// an owner's own [mysqld-auth] section wins.
const (
	MariaDBJailPath = "/etc/fail2ban/jail.d/celikpanel-database-remote.conf"
	jailMaxRetry    = 5
	jailFindTime    = "10m"
	jailBanTime     = "1h"
)

// MariaDBJail renders the jail for MariaDB's port and unit.
func MariaDBJail(port int, unit string) (string, error) {
	if port < 1 || port > 65535 || !regexp.MustCompile(`^[a-z0-9@._-]{1,64}\.service$`).MatchString(unit) {
		return "", errors.New("invalid MariaDB port or unit")
	}
	return "# CelikPanel: remote database access (docs/DATABASE-REMOTE-ACCESS.md).\n" +
		"# Bans an address after repeated wrong MariaDB passwords. Removed with the\n" +
		"# last remote access rule. If you change this file, CelikPanel leaves it.\n" +
		"[mysqld-auth]\n" +
		"enabled = true\n" +
		"port = " + strconv.Itoa(port) + "\n" +
		"backend = systemd\n" +
		"journalmatch = _SYSTEMD_UNIT=" + unit + "\n" +
		"maxretry = " + strconv.Itoa(jailMaxRetry) + "\n" +
		"findtime = " + jailFindTime + "\n" +
		"bantime = " + jailBanTime + "\n", nil
}

// IsRemoteAccountHost: host is in a form CelikPanel writes for a remote copy
// (an address, or an IPv4 network in netmask form): never a wildcard, a name
// or this server. The panel's engine driver acts on such copies when it
// deletes a user, revokes its access or changes its password.
func IsRemoteAccountHost(host string) bool { return validMariaDBHost(host) }
