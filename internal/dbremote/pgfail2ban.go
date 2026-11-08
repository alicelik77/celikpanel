package dbremote

import (
	"errors"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Fail2ban for PostgreSQL (alpha.130, owner decision 8 Nov 2026): an EXTRA
// layer on top of auth_delay, which stays the required gate. A missing or
// stopped Fail2ban never holds a PostgreSQL rule back.
//
// PostgreSQL's refused sign-in ("FATAL:  password authentication failed for
// user ...") carries no client address unless log_line_prefix names it. The
// Agent therefore adds a marker of its own, "celikpanel-client=%h ", at the
// end of the prefix, with ALTER SYSTEM (postgresql.auto.conf, reverted with
// ALTER SYSTEM RESET exactly like the listen address), and only when the
// cluster's prefix is one of the two package defaults below. Any other
// prefix is the owner's logging: it is left as it is and the extra layer is
// not used (the screen says how to enable it). log_hostname must be off, so
// %h is an address, never a name.
//
// The failregex is anchored on the whole known line, so a line written into
// a log FILE by a crafted user or database name cannot pass for a refused
// sign-in of another address: names in these lines are panel names
// ([A-Za-z0-9_], at most 63 characters, the pg_hba lines name them exactly),
// and PostgreSQL cuts a startup name at 63 bytes, which is too short to hold
// a whole forged line with its date, process number, marker and message.
//
// Fail2ban için PostgreSQL: auth_delay'in üstünde EK katman; Fail2ban yoksa
// kurallar beklemez. İstemci adresi log_line_prefix'e eklenen işaretten okunur.
const (
	PGJailNamePrefix = "celikpanel-postgresql-auth"
	PGFilterName     = "celikpanel-postgresql-auth"
	PGFilterPath     = "/etc/fail2ban/filter.d/" + PGFilterName + ".conf"
	pgJailDir        = "/etc/fail2ban/jail.d"
	// PGClientMarker is what CelikPanel adds to log_line_prefix.
	PGClientMarker = "celikpanel-client=%h "
)

// The package defaults of log_line_prefix CelikPanel may extend: PostgreSQL's
// own (10 and later) and the one Debian's and Ubuntu's pg_createcluster
// writes into postgresql.conf.
const (
	pgDefaultPrefix       = "%m [%p] "
	pgDebianDefaultPrefix = "%m [%p] %q%u@%d "
)

// PGPrefixWithMarker answers the prefix CelikPanel sets for current, or ""
// when current is not a package default (the owner's own logging).
func PGPrefixWithMarker(current string) string {
	switch current {
	case pgDefaultPrefix:
		return pgDefaultPrefix + "%q" + PGClientMarker
	case pgDebianDefaultPrefix:
		return pgDebianDefaultPrefix + PGClientMarker
	}
	return ""
}

// PGPrefixCarriesMarker: the prefix is one CelikPanel sets (its own or an
// owner who copied it); the filter reads such lines.
func PGPrefixCarriesMarker(prefix string) bool {
	return prefix == PGPrefixWithMarker(pgDefaultPrefix) || prefix == PGPrefixWithMarker(pgDebianDefaultPrefix)
}

// PGFailRegex is the filter's failregex (Python syntax, as Fail2ban reads
// it). Fail2ban cuts the date (the %m of the prefix, with its time zone when
// its date pattern takes it) and hands the rest over; a journal entry keeps
// its "host program[pid]: " head. <ADDR> accepts only an IP address.
const PGFailRegex = `^ *(?:\S+ +\S+\[\d+\]: +)?(?:\d{4}-\d\d-\d\d +\d\d:\d\d:\d\d(?:\.\d+)? *)?(?:[A-Za-z+-][A-Za-z0-9+:-]{0,9} +)?\[\d+\] +(?:[A-Za-z0-9_]{1,63}@[A-Za-z0-9_]{1,63} +)?celikpanel-client=<ADDR> +FATAL: +password authentication failed for user "[A-Za-z0-9_]{1,63}" *\n?\Z`

// PGFilter renders the filter file. The date pattern is PostgreSQL's %m
// ("2026-11-08 10:00:00.123 UTC") at the start of a line.
func PGFilter() string {
	return "# CelikPanel: remote database access (docs/DATABASE-REMOTE-ACCESS.md).\n" +
		"# A refused PostgreSQL sign-in, read from the client address CelikPanel\n" +
		"# adds to log_line_prefix. Removed with the last remote access rule.\n" +
		"[Definition]\n" +
		"failregex = " + PGFailRegex + "\n" +
		"ignoreregex =\n" +
		"datepattern = {^LN-BEG}%%Y-%%m-%%d %%H:%%M:%%S(?:\\.%%f)?(?: *%%z)?\n"
}

var pgJailFilePattern = regexp.MustCompile(`^celikpanel-database-remote-postgresql-(\d{1,5})\.conf$`)

// PGJailPath is the jail file of the cluster on port.
func PGJailPath(port int) string {
	return filepath.Join(pgJailDir, "celikpanel-database-remote-postgresql-"+strconv.Itoa(port)+".conf")
}

// PGJailPort reads the port of a CelikPanel PostgreSQL jail file name.
func PGJailPort(name string) (int, bool) {
	m := pgJailFilePattern.FindStringSubmatch(name)
	if m == nil {
		return 0, false
	}
	port, err := strconv.Atoi(m[1])
	return port, err == nil && port > 0 && port < 65536
}

// PGJailName is the jail of the cluster on port.
func PGJailName(port int) string { return PGJailNamePrefix + "-" + strconv.Itoa(port) }

// PGLogSource is where one cluster's log lines are read: a file (Debian's
// and Ubuntu's pg_ctlcluster log) or a systemd unit's journal.
type PGLogSource struct {
	File string
	Unit string
}

var pgLogFilePattern = regexp.MustCompile(`^/var/log/postgresql/[A-Za-z0-9._-]{1,120}\.log$`)

// PGJail renders the jail of the cluster on port.
func PGJail(port int, source PGLogSource) (string, error) {
	if port < 1 || port > 65535 {
		return "", errors.New("invalid PostgreSQL port")
	}
	var reading string
	switch {
	case source.File != "" && pgLogFilePattern.MatchString(source.File) && !strings.Contains(source.File, ".."):
		reading = "backend = auto\nlogpath = " + source.File + "\n"
	case source.Unit != "" && jailUnitPattern.MatchString(source.Unit):
		reading = "backend = systemd\njournalmatch = _SYSTEMD_UNIT=" + source.Unit + "\n"
	default:
		return "", errors.New("no PostgreSQL log CelikPanel can read")
	}
	return "# CelikPanel: remote database access (docs/DATABASE-REMOTE-ACCESS.md).\n" +
		"# Bans an address after repeated wrong PostgreSQL passwords (an extra layer\n" +
		"# on top of auth_delay). Removed with the last remote access rule of this\n" +
		"# cluster. If you change this file, CelikPanel leaves it.\n" +
		"[" + PGJailName(port) + "]\n" +
		"enabled = true\n" +
		"filter = " + PGFilterName + "\n" +
		"port = " + strconv.Itoa(port) + "\n" +
		reading +
		"maxretry = " + strconv.Itoa(jailMaxRetry) + "\n" +
		"findtime = " + jailFindTime + "\n" +
		"bantime = " + jailBanTime + "\n", nil
}

// PGLocaleEnglish: lc_messages writes PostgreSQL's messages in English, which
// the failregex reads.
func PGLocaleEnglish(lcMessages string) bool {
	v := strings.TrimSpace(lcMessages)
	switch {
	case v == "", v == "C", v == "POSIX", strings.HasPrefix(v, "C."), strings.HasPrefix(v, "en_"), strings.HasPrefix(v, "en."), v == "en":
		return true
	}
	return false
}
