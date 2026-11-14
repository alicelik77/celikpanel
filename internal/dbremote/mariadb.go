package dbremote

import (
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/alicelik/celikpanel/internal/mariadbgrant"
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

// MariaDBCredential is what a remote copy takes from the local account:
// the plugin and stored value, its connection limit (review L1; 0 = none)
// and whether it is locked (suspension, review M1).
type MariaDBCredential struct {
	Plugin             string
	Auth               string
	MaxUserConnections int
	Locked             bool
}

// Validate refuses a credential that cannot be copied safely.
func (c MariaDBCredential) Validate() error {
	if !mariadbPlugins[c.Plugin] {
		return fmt.Errorf("the local account signs in with %q, which has no password a remote program could send", c.Plugin)
	}
	if !mariadbAuthText.MatchString(c.Auth) {
		return errors.New("the local account has no password stored in a form CelikPanel can copy; set a new password for this user once")
	}
	if c.MaxUserConnections < 0 || c.MaxUserConnections > 1000000 {
		return errors.New("the local account's connection limit is out of range")
	}
	return nil
}

func mysqlString(value string) string {
	return "'" + strings.ReplaceAll(strings.ReplaceAll(value, `\`, `\\`), `'`, `\'`) + "'"
}

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
	lock := "ACCOUNT UNLOCK"
	if credential.Locked {
		lock = "ACCOUNT LOCK"
	}
	return fmt.Sprintf("%s %s IDENTIFIED VIA %s USING %s REQUIRE SSL WITH MAX_USER_CONNECTIONS %d %s;",
		verb, name, credential.Plugin, mysqlString(credential.Auth), credential.MaxUserConnections, lock), nil
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

// GrantMariaDB renders one database grant for a remote account, by the
// database's exact name (alpha.134: `shop\_a`.*; written `shop_a`.* the
// grant is a pattern that also covers other databases).
func GrantMariaDB(privileges, database, user, host string) (string, error) {
	if _, err := normalizeMariaDBPrivileges(privileges); err != nil {
		return "", err
	}
	if err := validName(database); err != nil {
		return "", err
	}
	target, err := copyTarget(database)
	if err != nil {
		return "", err
	}
	name, err := MariaDBAccountName(user, host)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("GRANT %s ON %s.* TO %s;", privileges, target, name), nil
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
	// Databases maps a database to its normalized privilege list: the grant
	// in its exact form when the account has one, otherwise the one in the
	// pattern form (alpha.134).
	Databases map[string]string
	// Exact and Patterns (alpha.134) split the same grants by the form they
	// are held in: the exact form (`shop\_a`.*, written since alpha.134)
	// and the pattern form (`shop_a`.*, written before; `_` covers any one
	// character). A grant whose name is neither form of one database (a
	// wildcard on purpose, `shop\_%`) is in none of these maps: it is never
	// copied to a remote account.
	Exact    map[string]string
	Patterns map[string]string
}

var grantLine = regexp.MustCompile("^GRANT (.+?) ON (\\*|`(?:[^`]|``)+`)\\.\\* TO `((?:[^`]|``)+)`@`((?:[^`]|``)+)`(.*)$")

// ParseMariaDBGrants reads the lines of SHOW GRANTS FOR user@host. Only
// whole-database grants are copied; a table or column grant, a role or a
// proxy grant is not, so a remote account never holds more than the local
// one. The lines may carry the password hash (IDENTIFIED BY PASSWORD): this
// function never returns or quotes them.
func ParseMariaDBGrants(lines []string, user, host string) (LocalGrants, error) {
	out := LocalGrants{Databases: map[string]string{}, Exact: map[string]string{}, Patterns: map[string]string{}}
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
		stored := strings.ReplaceAll(strings.Trim(scope, "`"), "``", "`")
		database, pattern, ok := mariadbgrant.Decode(stored)
		if !ok {
			continue // a wildcard on purpose, or a name CelikPanel does not write
		}
		if pattern {
			out.Patterns[database] = normalized
		} else {
			out.Exact[database] = normalized
		}
	}
	for database, privileges := range out.Patterns {
		out.Databases[database] = privileges
	}
	for database, privileges := range out.Exact {
		out.Databases[database] = privileges
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
func MariaDBOptionFile(cert, key string) string { return MariaDBOptionFileWith(cert, key, false) }

// mariadbSkipNameResolveLines turn MariaDB's host name lookup off (alpha.130,
// written only after the administrator agreed; docs/DATABASE-REMOTE-ACCESS.md
// "Host name lookup"): MariaDB then names the client's address, not its host
// name, in a refused sign-in, so the Fail2ban jail can ban it.
const mariadbSkipNameResolveLines = "# Host name lookup off (the administrator agreed in CelikPanel): refused\n" +
	"# sign-ins are logged with the client's address, which Fail2ban bans.\n" +
	"skip-name-resolve\n"

// MariaDBOptionFileWith renders the option file, with host name lookup off
// when skipNameResolve.
func MariaDBOptionFileWith(cert, key string, skipNameResolve bool) string {
	text := mariadbOptionHeader
	if cert != "" && key != "" {
		text += "ssl_cert = " + cert + "\nssl_key = " + key + "\n"
	}
	if skipNameResolve {
		text += mariadbSkipNameResolveLines
	}
	return text
}

// IsOwnMariaDBOptionFile: the file is one CelikPanel wrote, unchanged.
func IsOwnMariaDBOptionFile(content, cert, key string) bool {
	for _, skip := range []bool{false, true} {
		if content == MariaDBOptionFileWith("", "", skip) || (cert != "" && content == MariaDBOptionFileWith(cert, key, skip)) {
			return true
		}
	}
	return false
}

// MariaDBOptionFileSkipsNameResolve: CelikPanel's own option file turns host
// name lookup off.
func MariaDBOptionFileSkipsNameResolve(content string) bool {
	return strings.Contains(content, mariadbSkipNameResolveLines)
}

// Fail2ban (review M3). CelikPanel ships its own filter and jail, named
// celikpanel-mariadb-auth, so an owner's [mysqld-auth] is never changed, and
// the filter does not depend on the syslog prefix of the distribution's
// mysqld-auth filter (the journal names the program mariadbd, not mysqld).
// Remote access waits until `fail2ban-client status celikpanel-mariadb-auth`
// answers, so wrong passwords are slowed down before anybody can try one
// from outside.
//
// The jail reads MariaDB's journal only (re-review LOW 1). The user name in
// "Access denied for user '<name>'@'<host>'" is whatever the client sent, so
// a name holding a newline and a forged "'@'<another address>' (using
// password: YES)" would, in a log FILE, end a line that blames someone else.
// In the journal the whole message is one entry; the failregex is anchored
// at its start (the optional journal prefix, MariaDB's own timestamp and
// "[Warning]") and at its very end (\Z, not $), and the name may hold no
// quote and no newline, so only the address MariaDB itself appended can be
// the one banned. A MariaDB that logs to a file (log_error) waits, with the
// step to remove that setting.
const (
	MariaDBJailName   = "celikpanel-mariadb-auth"
	MariaDBJailPath   = "/etc/fail2ban/jail.d/celikpanel-database-remote.conf"
	MariaDBFilterPath = "/etc/fail2ban/filter.d/" + MariaDBJailName + ".conf"
	jailMaxRetry      = 5
	jailFindTime      = "10m"
	jailBanTime       = "1h"
)

// MariaDBFailRegex is the filter's failregex (Python syntax, as Fail2ban
// reads it): one refused sign-in (log_warnings >= 2) of MariaDB 10.x/11.x as
// a journal entry. <ADDR> is Fail2ban's placeholder for an IP address only.
//
// Measured with Fail2ban 1.1 on Debian 13 (alpha.129 lab): Fail2ban cuts the
// leading timestamp out of a line and leaves the space after it, and hands
// an entry over with its final newline. The first version (anchored with
// "^(?:" and " *\Z") matched no real line, so seven wrong passwords banned
// nobody. " *" at the start and one optional "\n" before \Z accept exactly
// those two; the name still may hold no quote and no newline.
//
// alpha.130: <ADDR>, not <HOST>. While MariaDB looks client host names up
// (skip_name_resolve off, its default) the line names the client's host
// name ('_gateway' in the alpha.129 lab, so nobody was banned). Fail2ban
// could only resolve such a name again by DNS when it bans, and whoever
// controls that name's DNS can point it elsewhere by then. A line with a
// name therefore bans nobody, and MariaDB rules wait until MariaDB logs
// addresses (host name lookup off; database_remote_mariadb.go).
const MariaDBFailRegex = `^ *(?:\S+ +\S+\[\d+\]: +)?(?:(?:\d{4}-\d\d-\d\d|\d{6}) +\d{1,2}:\d\d:\d\d +)?(?:\d+ +)?\[Warning\] Access denied for user '[^'\n]*'@'<ADDR>' \(using password: (?:YES|NO)\) *\n?\Z`

// mariaDBEarlierFailRegexes are failregexes CelikPanel wrote before
// (newest first). A filter file holding exactly one of them is CelikPanel's
// own earlier version, not the owner's: the Agent replaces it with the
// current one (alpha.129 lab: the first version stayed on disk after an
// update, kept as "the owner's", and still banned nobody).
var mariaDBEarlierFailRegexes = []string{
	// alpha.129 after its lab round (<HOST>).
	`^ *(?:\S+ +\S+\[\d+\]: +)?(?:(?:\d{4}-\d\d-\d\d|\d{6}) +\d{1,2}:\d\d:\d\d +)?(?:\d+ +)?\[Warning\] Access denied for user '[^'\n]*'@'<HOST>' \(using password: (?:YES|NO)\) *\n?\Z`,
	`^(?:\S+ +\S+\[\d+\]: +)?(?:(?:\d{4}-\d\d-\d\d|\d{6}) +\d{1,2}:\d\d:\d\d +)?(?:\d+ +)?\[Warning\] Access denied for user '[^'\n]*'@'<HOST>' \(using password: (?:YES|NO)\) *\Z`,
}

// MariaDBFilter renders the filter file.
func MariaDBFilter() string { return mariaDBFilterWith(MariaDBFailRegex) }

func mariaDBFilterWith(failRegex string) string {
	return "# CelikPanel: remote database access (docs/DATABASE-REMOTE-ACCESS.md).\n" +
		"# A refused MariaDB sign-in, one journal entry. Removed with the last\n" +
		"# remote access rule.\n" +
		"[Definition]\n" +
		"failregex = " + failRegex + "\n" +
		"ignoreregex =\n"
}

// MariaDBFilterEarlier reports whether content is exactly a filter file an
// earlier CelikPanel wrote (never the current one, never an owner's edit).
func MariaDBFilterEarlier(content string) bool {
	for _, re := range mariaDBEarlierFailRegexes {
		if content == mariaDBFilterWith(re) {
			return true
		}
	}
	return false
}

var jailUnitPattern = regexp.MustCompile(`^[a-z0-9@._-]{1,64}\.service$`)

// MariaDBJail renders the jail for MariaDB's port: the unit's journal.
func MariaDBJail(port int, unit string) (string, error) {
	if port < 1 || port > 65535 || !jailUnitPattern.MatchString(unit) {
		return "", errors.New("invalid MariaDB port or unit")
	}
	return "# CelikPanel: remote database access (docs/DATABASE-REMOTE-ACCESS.md).\n" +
		"# Bans an address after repeated wrong MariaDB passwords. Removed with the\n" +
		"# last remote access rule. If you change this file, CelikPanel leaves it.\n" +
		"[" + MariaDBJailName + "]\n" +
		"enabled = true\n" +
		"filter = " + MariaDBJailName + "\n" +
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

// mariadbPrivilegeSet reads a normalized privilege list as a set.
func mariadbPrivilegeSet(text string) (map[string]bool, error) {
	normalized, err := normalizeMariaDBPrivileges(text)
	if err != nil {
		return nil, err
	}
	set := map[string]bool{}
	for _, p := range strings.Split(normalized, ", ") {
		set[p] = true
	}
	return set, nil
}

// MariaDBPrivilegeChange answers the statements that bring a remote copy's
// grant on one database from current to want ("" = none), adding before
// taking away, so an unchanged grant gets no statement and a copy never
// loses a privilege it keeps (re-review). The one exception is narrowing
// ALL PRIVILEGES to a list, which MariaDB can only express as REVOKE ALL
// then GRANT.
//
// current is the copy's grant in the exact form. alpha.134: currentPattern
// says the copy also holds a grant in the pattern form (written before
// alpha.134); the wanted grant is then written in the exact form first and
// the pattern form is taken away after it, so the copy never loses access
// it keeps and no longer reaches other databases.
func MariaDBPrivilegeChange(current string, currentPattern bool, want, database, user, host string) ([]string, error) {
	name, err := MariaDBAccountName(user, host)
	if err != nil {
		return nil, err
	}
	if err := validName(database); err != nil {
		return nil, err
	}
	exact, err := copyTarget(database)
	if err != nil {
		return nil, err
	}
	out, err := mariadbExactPrivilegeChange(current, want, name, " ON "+exact+".* ")
	if err != nil || !currentPattern || !mariadbgrant.ExactFits(database) {
		return out, err
	}
	pattern, err := mariadbgrant.PatternTarget(database)
	if err != nil {
		return nil, err
	}
	return append(out, "REVOKE ALL PRIVILEGES ON "+pattern+".* FROM "+name+";"), nil
}

// copyTarget is the database part of a remote copy's grant: the exact form,
// or - for a name whose exact form does not fit a grant row (measured: it
// would be stored cut short, a right on another name) - the pattern form the
// local account keeps too (alpha.134 review). A copy never holds more than
// the local account.
func copyTarget(database string) (string, error) {
	target, err := mariadbgrant.Target(database)
	if errors.Is(err, mariadbgrant.ErrExactTooLong) {
		return mariadbgrant.PatternTarget(database)
	}
	return target, err
}

// mariadbExactPrivilegeChange is MariaDBPrivilegeChange for one form.
func mariadbExactPrivilegeChange(current, want, name, on string) ([]string, error) {
	switch {
	case current == "" && want == "":
		return nil, nil
	case want == "":
		return []string{"REVOKE ALL PRIVILEGES" + on + "FROM " + name + ";"}, nil
	}
	wantSet, err := mariadbPrivilegeSet(want)
	if err != nil {
		return nil, err
	}
	grant := func(privileges string) string { return "GRANT " + privileges + on + "TO " + name + ";" }
	if current == "" {
		normalized, _ := normalizeMariaDBPrivileges(want)
		return []string{grant(normalized)}, nil
	}
	currentSet, err := mariadbPrivilegeSet(current)
	if err != nil {
		return nil, err
	}
	if len(currentSet) == len(wantSet) {
		same := true
		for p := range wantSet {
			same = same && currentSet[p]
		}
		if same {
			return nil, nil
		}
	}
	switch {
	case wantSet["ALL PRIVILEGES"]:
		return []string{grant("ALL PRIVILEGES")}, nil
	case currentSet["ALL PRIVILEGES"]:
		normalized, _ := normalizeMariaDBPrivileges(want)
		return []string{"REVOKE ALL PRIVILEGES" + on + "FROM " + name + ";", grant(normalized)}, nil
	}
	var add, extra []string
	for p := range wantSet {
		if !currentSet[p] {
			add = append(add, p)
		}
	}
	for p := range currentSet {
		if !wantSet[p] {
			extra = append(extra, p)
		}
	}
	sort.Strings(add)
	sort.Strings(extra)
	var out []string
	if len(add) > 0 {
		out = append(out, grant(strings.Join(add, ", ")))
	}
	if len(extra) > 0 {
		out = append(out, "REVOKE "+strings.Join(extra, ", ")+on+"FROM "+name+";")
	}
	return out, nil
}
