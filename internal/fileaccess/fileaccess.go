// Package fileaccess renders the native configuration that gives a hosting
// account FTP over TLS, SFTP and (with the ssh right) a jailed command line.
// It does no I/O: the Agent writes what is rendered here, and the Panel uses
// the same validation before it asks.
//
// Every file is ordinary OpenSSH, vsftpd, PAM or Fail2ban configuration, so
// access keeps working with CelikPanel stopped or removed (D-022). A managed
// file starts with a header that carries the SHA-256 of the rest; a file that
// does not match was edited by the owner and is kept, never overwritten.
//
// Dosya erişimi (FTP/TLS, SFTP, hapsedilmiş SSH) için yerel yapılandırma.
// Burada G/Ç yoktur; Agent yazar. Sahibin düzenlediği dosya korunur.
package fileaccess

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/alicelik/celikpanel/internal/hostingpath"
)

// Host paths and names. They are fixed: nothing here comes from tenant text.
const (
	// SiteGroup holds every hosting account CelikPanel created or
	// reconciled. sshd refuses the group unless the account has file access.
	SiteGroup = "celikpanel-site"

	SSHDropInDir  = "/etc/ssh/sshd_config.d"
	SSHDropInPath = SSHDropInDir + "/60-celikpanel-file-access.conf"
	// KeysDir holds one root-owned authorized_keys file per account, so a
	// key can only come through CelikPanel (or the owner as root), never
	// from a file uploaded into the site.
	KeysDir = "/etc/ssh/celikpanel-keys"

	VsftpdConfPath    = "/etc/vsftpd.conf"
	VsftpdDir         = "/etc/vsftpd"
	VsftpdUserList    = VsftpdDir + "/celikpanel-users.list"
	VsftpdUserConfDir = VsftpdDir + "/celikpanel-users"
	VsftpdPAMService  = "celikpanel-vsftpd"
	VsftpdPAMPath     = "/etc/pam.d/" + VsftpdPAMService
	VsftpdLogPath     = "/var/log/vsftpd.log"

	Fail2banJailPath = "/etc/fail2ban/jail.d/celikpanel-file-access.conf"

	// SelfSignedDir holds the fallback FTP certificate when the server has
	// no trusted mail host certificate.
	SelfSignedDir  = "/etc/ssl/celikpanel/_ftp"
	SelfSignedCert = SelfSignedDir + "/selfsigned.crt"
	SelfSignedKey  = SelfSignedDir + "/selfsigned.key"
	// MailHostCert/Key are the server's mail host certificate, renewed by
	// certbot. vsftpd reads the files for every new connection.
	MailHostCert = "/etc/ssl/celikpanel/_mail/host/current/fullchain.pem"
	MailHostKey  = "/etc/ssl/celikpanel/_mail/host/current/privkey.pem"

	// Passive data ports. TLS hides the PASV reply from connection
	// tracking, so the range is opened in the firewall explicitly.
	PassiveMin = 40000
	PassiveMax = 40049

	// NoLoginShell is the shell of every account without the ssh right.
	NoLoginShell = "/usr/sbin/nologin"
	// JailShell is the shell of an account with the ssh right. sshd starts
	// it inside the chroot, where only the jail's copy exists.
	JailShell = "/bin/bash"

	managedPrefix = "# Managed by CelikPanel: "
)

// PassivePorts lists the passive range, for the firewall catalogue.
func PassivePorts() []int {
	ports := make([]int, 0, PassiveMax-PassiveMin+1)
	for port := PassiveMin; port <= PassiveMax; port++ {
		ports = append(ports, port)
	}
	return ports
}

// ---------------------------------------------------------------------------
// Managed header

// ManagedState is what a file on disk is to CelikPanel.
type ManagedState string

const (
	FileAbsent  ManagedState = "absent"
	FileManaged ManagedState = "managed"
	// FileOwner: the file is not CelikPanel's unchanged text. It is kept.
	FileOwner ManagedState = "owner"
)

// Seal puts the managed header on body.
func Seal(what, body string) string {
	sum := sha256.Sum256([]byte(body))
	return managedPrefix + what + ". sha256=" + hex.EncodeToString(sum[:]) + "\n" + body
}

// Classify reads a file's text. exists=false is FileAbsent.
func Classify(content []byte, exists bool) (ManagedState, string) {
	if !exists {
		return FileAbsent, ""
	}
	text := string(content)
	first, body, ok := strings.Cut(text, "\n")
	if !ok || !strings.HasPrefix(first, managedPrefix) {
		return FileOwner, ""
	}
	marker := strings.LastIndex(first, ". sha256=")
	if marker < 0 {
		return FileOwner, ""
	}
	sum := sha256.Sum256([]byte(body))
	if first[marker+len(". sha256="):] != hex.EncodeToString(sum[:]) {
		return FileOwner, ""
	}
	return FileManaged, body
}

// ---------------------------------------------------------------------------
// Accounts

// Entry is one hosting account with file access.
type Entry struct {
	Username       string
	SubscriptionID int
	DomainID       int
	SSH            bool
	// ReadOnly (alpha.118, D-044): the subscription's disk is full, so new
	// uploads are closed: SFTP is read-only (internal-sftp -R) and FTP has
	// write_enable=NO. A command-line (SSH) account cannot be made read-only.
	ReadOnly bool
}

// sftpCommand is the ForceCommand of an SFTP-only account.
func sftpCommand(e Entry) string {
	if e.ReadOnly {
		return "internal-sftp -R -d " + StartDir(e.DomainID) + " -u 0027"
	}
	return "internal-sftp -d " + StartDir(e.DomainID) + " -u 0027"
}

var usernamePattern = regexp.MustCompile(`^[a-z0-9_][a-z0-9_.-]{0,31}$`)

// ValidUsername accepts the names SiteUsername produces. The pattern keeps
// every sshd and vsftpd pattern character (* ? ! , space) out.
func ValidUsername(name string) bool {
	return usernamePattern.MatchString(name) && !strings.Contains(name, "..")
}

// Validate checks an entry's identity fields.
func (e Entry) Validate() error {
	if !ValidUsername(e.Username) {
		return fmt.Errorf("invalid account name %q", e.Username)
	}
	if e.SubscriptionID <= 0 || e.DomainID <= 0 {
		return errors.New("subscription and domain identities must be positive")
	}
	return nil
}

// ChrootDir is the root an account sees: its subscription's sites folder.
// It is root-owned (0751), so sshd and vsftpd accept it, and an account
// can enter only its own site folder below it.
func ChrootDir(subscriptionID int) string {
	return path.Join(hostingpath.SubscriptionsRoot(), strconv.Itoa(subscriptionID), "sites")
}

// SiteHome is the account's home on the host.
func SiteHome(subscriptionID, domainID int) string {
	return path.Join(ChrootDir(subscriptionID), strconv.Itoa(domainID))
}

// StartDir is the account's own folder as seen inside the chroot.
func StartDir(domainID int) string {
	return "/" + strconv.Itoa(domainID)
}

// ShellFor is the login shell an account must have.
func ShellFor(enabled, ssh bool) string {
	if enabled && ssh {
		return JailShell
	}
	return NoLoginShell
}

// UsableShell reports whether a login shell gives a command line.
func UsableShell(shell string) bool {
	switch strings.TrimSpace(shell) {
	case "", "/usr/sbin/nologin", "/sbin/nologin", "/usr/bin/nologin", "/bin/nologin",
		"/bin/false", "/usr/bin/false":
		return false
	}
	return true
}

// PasswordState reads the password field of /etc/shadow without exposing it.
type PasswordState string

const (
	PasswordNone   PasswordState = "none"   // no password, keys still work: "*"
	PasswordSet    PasswordState = "set"    // a hash
	PasswordLocked PasswordState = "locked" // a hash behind "!"
	PasswordEmpty  PasswordState = "empty"  // empty field: a login without a password
	// PasswordDisabled: "!" without a hash (useradd's default). With
	// UsePAM no, sshd treats it as a locked account and refuses keys too.
	PasswordDisabled PasswordState = "disabled"
)

// ClassifyShadowPassword reads one shadow password field.
func ClassifyShadowPassword(field string) PasswordState {
	switch {
	case field == "":
		return PasswordEmpty
	case field == "!" || field == "!!" || field == "!*" || field == "*LK*":
		return PasswordDisabled
	case field == "*":
		return PasswordNone
	case strings.HasPrefix(field, "!"):
		return PasswordLocked
	case strings.HasPrefix(field, "$"):
		return PasswordSet
	default:
		return PasswordNone
	}
}

// ValidatePassword applies the panel's account password rule to an FTP/SSH
// password: 8 to 128 bytes of printable text on one line.
func ValidatePassword(password string) error {
	if len(password) < 8 {
		return errors.New("password must be at least 8 characters")
	}
	if len(password) > 128 {
		return errors.New("password must be at most 128 bytes")
	}
	if !utf8.ValidString(password) {
		return errors.New("password must be valid text")
	}
	for _, r := range password {
		if unicode.IsControl(r) {
			return errors.New("password must be printable text on one line")
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// OpenSSH drop-in

const entryMarker = "# celikpanel-entry "

// RenderSSHDropIn renders the sshd drop-in for every account with access
// and every additional login (alpha.128). The entry lines are the drop-in's
// own record of who has access; the Agent reads them back only from a file
// whose header verifies. Every caller passes the logins it read: a drop-in
// rendered without them would drop their blocks (they would then fall into
// the deny block below: closed, not open). Without logins the text is the
// alpha.117 text, byte for byte.
func RenderSSHDropIn(entries []Entry, logins []Login) (string, error) {
	sorted, err := sortedEntries(entries)
	if err != nil {
		return "", err
	}
	sortedLogins, err := SortedLogins(logins)
	if err != nil {
		return "", err
	}
	for _, l := range sortedLogins {
		if _, clash := FindEntry(sorted, l.Login); clash {
			return "", fmt.Errorf("login %s has the name of a site account", l.Login)
		}
	}
	var b strings.Builder
	b.WriteString("# File access for hosting accounts (SFTP, and SSH with the ssh right).\n")
	b.WriteString("# Every account below is confined to its subscription's folder; any other\n")
	b.WriteString("# hosting account in group " + SiteGroup + " cannot sign in over SSH.\n")
	b.WriteString("# CelikPanel rewrites this file only while it is exactly as CelikPanel wrote\n")
	b.WriteString("# it. Edit it by hand and CelikPanel keeps your version and says so.\n")
	var names []string
	for _, e := range sorted {
		names = append(names, e.Username)
		ssh := 0
		if e.SSH {
			ssh = 1
		}
		readOnly := ""
		if e.ReadOnly {
			readOnly = " readonly=1"
		}
		fmt.Fprintf(&b, "\n%suser=%s subscription=%d domain=%d ssh=%d%s\n", entryMarker, e.Username, e.SubscriptionID, e.DomainID, ssh, readOnly)
		fmt.Fprintf(&b, "Match User %s\n", e.Username)
		fmt.Fprintf(&b, "\tChrootDirectory %s\n", ChrootDir(e.SubscriptionID))
		fmt.Fprintf(&b, "\tAuthorizedKeysFile %s/%%u\n", KeysDir)
		if !e.SSH {
			// internal-sftp runs inside sshd: no shell and no program from
			// the jail. -u 0027 keeps new files readable by the web server
			// group only.
			fmt.Fprintf(&b, "\tForceCommand %s\n", sftpCommand(e))
			b.WriteString("\tPermitTTY no\n")
		}
		b.WriteString("\tDisableForwarding yes\n")
		b.WriteString("\tPermitTunnel no\n")
		b.WriteString("\tPermitUserRC no\n")
	}
	if len(sortedLogins) > 0 {
		b.WriteString("\n# Additional FTP/SFTP logins: SFTP only, each closed into the jail of the\n")
		b.WriteString("# folder its site gave it (" + LoginJailRoot + "/<login>, docs/FTP-LOGINS.md).\n")
	}
	for _, l := range sortedLogins {
		names = append(names, l.Login)
		renderLoginBlock(&b, l)
	}
	sort.Strings(names)
	b.WriteString("\n# Every other hosting account: no SSH sign-in, not even a tunnel.\n")
	if len(names) == 0 {
		fmt.Fprintf(&b, "Match Group %s\n", SiteGroup)
	} else {
		var users []string
		for _, name := range names {
			users = append(users, "!"+name)
		}
		users = append(users, "*")
		fmt.Fprintf(&b, "Match Group %s User %s\n", SiteGroup, strings.Join(users, ","))
	}
	fmt.Fprintf(&b, "\tDenyGroups %s\n", SiteGroup)
	b.WriteString("\tDisableForwarding yes\n")
	b.WriteString("\n# The rest of the server's sshd configuration is not affected.\nMatch all\n")
	return Seal("file access for hosting accounts", b.String()), nil
}

// ParseSSHDropIn reads the account and login records of a verified drop-in
// body.
func ParseSSHDropIn(body string) ([]Entry, []Login, error) {
	entries, err := parseSSHDropInEntries(body)
	if err != nil {
		return nil, nil, err
	}
	var logins []Login
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, loginMarker) {
			continue
		}
		l, err := parseLoginLine(line)
		if err != nil {
			return nil, nil, err
		}
		logins = append(logins, l)
	}
	if logins, err = SortedLogins(logins); err != nil {
		return nil, nil, err
	}
	return entries, logins, nil
}

func parseSSHDropInEntries(body string) ([]Entry, error) {
	var entries []Entry
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, entryMarker) {
			continue
		}
		fields := map[string]string{}
		for _, field := range strings.Fields(strings.TrimPrefix(line, entryMarker)) {
			key, value, ok := strings.Cut(field, "=")
			if !ok {
				return nil, fmt.Errorf("malformed entry %q", line)
			}
			fields[key] = value
		}
		subscription, err1 := strconv.Atoi(fields["subscription"])
		domain, err2 := strconv.Atoi(fields["domain"])
		if err1 != nil || err2 != nil || (fields["ssh"] != "0" && fields["ssh"] != "1") ||
			(fields["readonly"] != "" && fields["readonly"] != "1") {
			return nil, fmt.Errorf("malformed entry %q", line)
		}
		entry := Entry{Username: fields["user"], SubscriptionID: subscription, DomainID: domain, SSH: fields["ssh"] == "1", ReadOnly: fields["readonly"] == "1"}
		if err := entry.Validate(); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return sortedEntries(entries)
}

func sortedEntries(entries []Entry) ([]Entry, error) {
	out := append([]Entry(nil), entries...)
	seen := map[string]bool{}
	for _, e := range out {
		if err := e.Validate(); err != nil {
			return nil, err
		}
		if seen[e.Username] {
			return nil, fmt.Errorf("account %s is listed twice", e.Username)
		}
		seen[e.Username] = true
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Username < out[j].Username })
	return out, nil
}

// MergeEntry returns entries with one account replaced, added or removed.
func MergeEntry(entries []Entry, change Entry, enabled bool) []Entry {
	out := make([]Entry, 0, len(entries)+1)
	for _, e := range entries {
		if e.Username != change.Username {
			out = append(out, e)
		}
	}
	if enabled {
		out = append(out, change)
	}
	return out
}

// FindEntry returns an account's entry.
func FindEntry(entries []Entry, username string) (Entry, bool) {
	for _, e := range entries {
		if e.Username == username {
			return e, true
		}
	}
	return Entry{}, false
}

// ExpectedSSHDEffective lists what `sshd -T -C user=<name>,...` must print
// for an account (lowercase keywords, as sshd prints them).
func ExpectedSSHDEffective(e Entry, enabled bool) map[string]string {
	if !enabled {
		return map[string]string{"denygroups": SiteGroup}
	}
	want := map[string]string{
		"chrootdirectory":    ChrootDir(e.SubscriptionID),
		"authorizedkeysfile": KeysDir + "/%u",
		"disableforwarding":  "yes",
		"permittunnel":       "no",
	}
	if !e.SSH {
		want["forcecommand"] = sftpCommand(e)
	}
	return want
}

// CheckSSHDEffective compares `sshd -T` output with the expectation and
// returns the first keyword that differs.
func CheckSSHDEffective(output string, want map[string]string) error {
	got := map[string]string{}
	for _, line := range strings.Split(output, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), " ")
		if !ok {
			continue
		}
		key = strings.ToLower(key)
		if existing, seen := got[key]; seen {
			got[key] = existing + " " + strings.TrimSpace(value)
		} else {
			got[key] = strings.TrimSpace(value)
		}
	}
	keys := make([]string, 0, len(want))
	for key := range want {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := got[key]
		if key == "denygroups" {
			if !containsField(value, want[key]) {
				return fmt.Errorf("sshd does not apply %s %s (it says %q)", key, want[key], value)
			}
			continue
		}
		if value != want[key] {
			return fmt.Errorf("sshd does not apply %s %s (it says %q)", key, want[key], value)
		}
	}
	return nil
}

func containsField(value, want string) bool {
	for _, field := range strings.Fields(value) {
		if field == want {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Chroot safety

// DirInfo is one directory on the way to a chroot.
type DirInfo struct {
	Path  string
	IsDir bool
	UID   uint32
	Mode  uint32 // permission bits
}

// UnsafeChrootComponent applies sshd's own rule (each component root-owned,
// not writable by group or others) and returns the first directory that
// breaks it, with a short reason. Empty means safe.
func UnsafeChrootComponent(chain []DirInfo) (string, string) {
	for _, dir := range chain {
		switch {
		case !dir.IsDir:
			return dir.Path, "not a directory"
		case dir.UID != 0:
			return dir.Path, "not owned by root"
		case dir.Mode&0o022 != 0:
			return dir.Path, "writable by group or others"
		}
	}
	return "", ""
}

// ChrootChain lists every directory sshd checks for a chroot path.
func ChrootChain(chroot string) []string {
	clean := path.Clean(chroot)
	chain := []string{"/"}
	current := ""
	for _, part := range strings.Split(strings.TrimPrefix(clean, "/"), "/") {
		if part == "" {
			continue
		}
		current += "/" + part
		chain = append(chain, current)
	}
	return chain
}
