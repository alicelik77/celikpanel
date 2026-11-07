package fileaccess

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/alicelik/celikpanel/internal/hostingpath"
)

// Additional FTP/SFTP logins of a site (alpha.128, gap G16,
// docs/FTP-LOGINS.md).
//
// A login is its own entry in /etc/passwd with the site account's uid and
// primary group (useradd -o), a no-login shell and no key: files it uploads
// belong to the site exactly like the site account's own uploads, so PHP,
// Node, the disk count and the web server's group read them as today, and
// removing a login never leaves a file without its owner. The login cannot
// run anything: SFTP is internal-sftp inside sshd (ForceCommand), FTP is
// vsftpd, forwarding and tunnels are off.
//
// It is confined to one folder of its site. sshd's ChrootDirectory and
// vsftpd's chroot need root-owned folders on the whole way, and a folder of
// the site belongs to the site account, which could swap it for a link. So
// the login's root is a root-owned jail outside the hosting tree,
// /run/celikpanel-ftp/<login> (root:<site group> 0750), and the chosen folder
// is bound onto its "files" folder by the sandbox helper: opened one folder
// at a time without following any link (openat2 RESOLVE_NO_SYMLINKS, no
// mount point crossed), its owners checked, and bound by descriptor, never
// by name. A boot-persistent unit per login, celikpanel-ftp-<login>.service,
// does it at every start; /run is empty after a reboot, so a jail whose
// check fails does not exist and sshd and vsftpd refuse the login (fail
// closed). Everything is native configuration and keeps working without
// CelikPanel (D-022).
//
// Sitenin ek FTP/SFTP girişleri: sitenin uid'iyle ayrı bir hesap, yalnız
// seçilen klasöre kapalı. Klasör, root'a ait bir hapis klasörüne bağlantı
// izlenmeden açılıp tanımlayıcıyla bağlanır.

const (
	// LoginJailRoot holds one jail per login (root:root 0711): the logins
	// pass through it and nobody can list it.
	LoginJailRoot = "/run/celikpanel-ftp"
	// LoginUnitPrefix names a login's jail unit: celikpanel-ftp-<login>.service.
	LoginUnitPrefix = "celikpanel-ftp-"
	// LoginUnitDir is where the jail units live.
	LoginUnitDir = "/etc/systemd/system"
	// MaxLoginsPerServer bounds the record (and the sshd file) on one server.
	MaxLoginsPerServer = 500
	// LoginFolderMaxDepth bounds a chosen folder below the site home.
	LoginFolderMaxDepth = 12

	loginMarker = "# celikpanel-login "
)

// Login is one additional FTP/SFTP login of a site.
type Login struct {
	// Login is the login's own account name.
	Login string
	// Site is the site account the login belongs to (same uid and group).
	Site           string
	SubscriptionID int
	DomainID       int
	// Folder is the chosen folder below the site home, "/" for the home
	// itself, "/public_html/uploads" for a folder below it.
	Folder string
	// ReadOnly follows the subscription (alpha.118, D-044): its disk is full.
	ReadOnly bool
}

var (
	loginNamePattern  = regexp.MustCompile(`^[a-z][a-z0-9_-]{1,30}[a-z0-9]$`)
	folderPartPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)
)

// ValidateLoginName checks a login name. It must hold a hyphen: a site
// account's name never does (dots and hyphens of the domain become "_"), so a
// login can never take the name a future site needs.
func ValidateLoginName(name string) error {
	switch {
	case !loginNamePattern.MatchString(name):
		return errors.New("a login name is 3 to 32 characters: lowercase letters, digits, - and _, starting with a letter and ending with a letter or digit")
	case !strings.Contains(name, "-"):
		return errors.New("a login name must contain a hyphen (for example anna-uploads), so it can never be the name of a site's own account")
	case strings.Contains(name, "--"):
		return errors.New("a login name cannot contain two hyphens in a row")
	case strings.HasPrefix(name, "celikpanel"):
		return errors.New("names starting with celikpanel are kept for the panel itself")
	}
	return nil
}

// CleanLoginFolder brings a chosen folder to its stored form: "/" for the
// site home, otherwise "/a/b" with plain folder names (letters, digits,
// . _ -), no "." or "..", at most LoginFolderMaxDepth levels. It never
// resolves anything on disk.
func CleanLoginFolder(folder string) (string, error) {
	folder = strings.TrimSpace(folder)
	if folder == "" || folder == "/" {
		return "/", nil
	}
	if len(folder) > 512 {
		return "", errors.New("the folder path is too long")
	}
	var parts []string
	for _, part := range strings.Split(strings.Trim(folder, "/"), "/") {
		switch {
		case part == "":
			return "", errors.New("the folder path has an empty part (//)")
		case part == "." || part == "..":
			return "", errors.New("the folder path cannot contain . or ..")
		case !folderPartPattern.MatchString(part):
			return "", fmt.Errorf("the folder name %q may hold only letters, digits, dot, hyphen and underscore", part)
		}
		parts = append(parts, part)
	}
	if len(parts) > LoginFolderMaxDepth {
		return "", fmt.Errorf("the folder may be at most %d levels below the site", LoginFolderMaxDepth)
	}
	return "/" + strings.Join(parts, "/"), nil
}

// FolderParts are the folder names below the site home (none for "/").
func FolderParts(folder string) []string {
	trimmed := strings.Trim(folder, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}

// LoginFolderPath is the chosen folder on the host.
func LoginFolderPath(subscriptionID, domainID int, folder string) string {
	return path.Join(append([]string{SiteHome(subscriptionID, domainID)}, FolderParts(folder)...)...)
}

// SplitLoginFolderPath reads a host folder path back into the site home's
// identities and the folder below it. Only a path the hosting tree gives a
// site is accepted.
func SplitLoginFolderPath(hostPath string) (subscriptionID, domainID int, folder string, err error) {
	root := hostingpath.SubscriptionsRoot()
	if path.Clean(hostPath) != hostPath || !strings.HasPrefix(hostPath, root+"/") {
		return 0, 0, "", fmt.Errorf("%s is not a folder in the hosting tree", hostPath)
	}
	parts := strings.Split(strings.TrimPrefix(hostPath, root+"/"), "/")
	if len(parts) < 3 || parts[1] != "sites" {
		return 0, 0, "", fmt.Errorf("%s is not a folder of a site", hostPath)
	}
	subscriptionID, err1 := strconv.Atoi(parts[0])
	domainID, err2 := strconv.Atoi(parts[2])
	if err1 != nil || err2 != nil || subscriptionID <= 0 || domainID <= 0 ||
		strconv.Itoa(subscriptionID) != parts[0] || strconv.Itoa(domainID) != parts[2] {
		return 0, 0, "", fmt.Errorf("%s is not a folder of a site", hostPath)
	}
	folder, err = CleanLoginFolder("/" + strings.Join(parts[3:], "/"))
	if err != nil {
		return 0, 0, "", err
	}
	if LoginFolderPath(subscriptionID, domainID, folder) != hostPath {
		return 0, 0, "", fmt.Errorf("%s is not a plain folder path", hostPath)
	}
	return subscriptionID, domainID, folder, nil
}

// Validate checks a login's identity fields.
func (l Login) Validate() error {
	if err := ValidateLoginName(l.Login); err != nil {
		return fmt.Errorf("invalid login name %q: %v", l.Login, err)
	}
	if !ValidUsername(l.Site) {
		return fmt.Errorf("invalid site account name %q", l.Site)
	}
	if l.SubscriptionID <= 0 || l.DomainID <= 0 {
		return errors.New("subscription and domain identities must be positive")
	}
	clean, err := CleanLoginFolder(l.Folder)
	if err != nil {
		return err
	}
	if clean != l.Folder {
		return fmt.Errorf("the folder %q is not in its clean form", l.Folder)
	}
	return nil
}

// LoginJail is the login's root (root:<site group> 0750).
func LoginJail(login string) string { return LoginJailRoot + "/" + login }

// LoginFilesDir is the mount point inside the jail the folder is bound onto.
func LoginFilesDir(login string) string { return LoginJail(login) + "/files" }

// LoginHome is the login's home in /etc/passwd. vsftpd (passwd_chroot_enable)
// reads "/./" as the chroot point and starts in /files; sshd chroots to
// ChrootDirectory and internal-sftp starts in /files.
func LoginHome(login string) string { return LoginJail(login) + "/./files" }

// LoginStartDir is the folder a login starts in, inside its jail.
const LoginStartDir = "/files"

// LoginUnit is the login's jail unit.
func LoginUnit(login string) string { return LoginUnitPrefix + login + ".service" }

// LoginUnitPath is the jail unit's file.
func LoginUnitPath(login string) string { return LoginUnitDir + "/" + LoginUnit(login) }

// loginSFTPCommand is a login's ForceCommand.
func loginSFTPCommand(l Login) string {
	if l.ReadOnly {
		return "internal-sftp -R -d " + LoginStartDir + " -u 0027"
	}
	return "internal-sftp -d " + LoginStartDir + " -u 0027"
}

// RenderLoginUnit is a login's jail unit. helperPath is the sandbox helper
// (sitesandbox.HelperPath). The unit has no sandbox on purpose: the helper
// runs as root in the host's mount namespace.
func RenderLoginUnit(l Login, helperPath string) (string, error) {
	if err := l.Validate(); err != nil {
		return "", err
	}
	if !strings.HasPrefix(helperPath, "/") || strings.ContainsAny(helperPath, " \t\n\r") {
		return "", fmt.Errorf("invalid helper path %q", helperPath)
	}
	folder := LoginFolderPath(l.SubscriptionID, l.DomainID, l.Folder)
	var b strings.Builder
	fmt.Fprintf(&b, "# FTP/SFTP login %s of the site account %s: binds its folder %s\n", l.Login, l.Site, l.Folder)
	b.WriteString("# onto the root-owned jail " + LoginJail(l.Login) + " at every start (docs/FTP-LOGINS.md).\n")
	b.WriteString("# The folder is opened without following any link and checked before it is bound.\n")
	b.WriteString("# If the check fails there is no jail, and sshd and vsftpd refuse the login.\n")
	b.WriteString("# It works without CelikPanel:\n")
	fmt.Fprintf(&b, "#   systemctl status %s ; journalctl -u %s\n", LoginUnit(l.Login), LoginUnit(l.Login))
	b.WriteString("# CelikPanel rewrites this file only while it is exactly as CelikPanel wrote it.\n")
	b.WriteString("[Unit]\n")
	fmt.Fprintf(&b, "Description=CelikPanel FTP/SFTP login %s: folder jail\n", l.Login)
	b.WriteString("After=local-fs.target\n")
	fmt.Fprintf(&b, "RequiresMountsFor=%s\n", hostingpath.SubscriptionsRoot())
	b.WriteString("Before=ssh.service sshd.service vsftpd.service\n\n")
	b.WriteString("[Service]\nType=oneshot\nRemainAfterExit=yes\n")
	fmt.Fprintf(&b, "ExecStart=%s ftp-bind --login=%s --site=%s --folder=%s\n", helperPath, l.Login, l.Site, folder)
	fmt.Fprintf(&b, "ExecStop=%s ftp-unbind --login=%s\n\n", helperPath, l.Login)
	b.WriteString("[Install]\nWantedBy=multi-user.target\n")
	return Seal("FTP/SFTP login jail of "+l.Login, b.String()), nil
}

// RenderVsftpdLoginConf is a login's vsftpd setting. It sets no local_root:
// the jail comes from the home's "/./" (passwd_chroot_enable).
func RenderVsftpdLoginConf(l Login) (string, error) {
	if err := l.Validate(); err != nil {
		return "", err
	}
	body := "# The jail is the home's \"/./\" part: " + LoginHome(l.Login) + "\n"
	if l.ReadOnly {
		body += "write_enable=NO\n"
	}
	return Seal("FTP login of "+l.Login, body), nil
}

// ExpectedLoginSSHDEffective is what `sshd -T -C user=<login>,...` must print.
func ExpectedLoginSSHDEffective(l Login) map[string]string {
	return map[string]string{
		"chrootdirectory":      LoginJail(l.Login),
		"forcecommand":         loginSFTPCommand(l),
		"pubkeyauthentication": "no",
		"disableforwarding":    "yes",
		"permittunnel":         "no",
		"permittty":            "no",
	}
}

func renderLoginBlock(b *strings.Builder, l Login) {
	readOnly := 0
	if l.ReadOnly {
		readOnly = 1
	}
	fmt.Fprintf(b, "\n%slogin=%s site=%s subscription=%d domain=%d folder=%s readonly=%d\n",
		loginMarker, l.Login, l.Site, l.SubscriptionID, l.DomainID, l.Folder, readOnly)
	fmt.Fprintf(b, "Match User %s\n", l.Login)
	fmt.Fprintf(b, "\tChrootDirectory %s\n", LoginJail(l.Login))
	b.WriteString("\tPubkeyAuthentication no\n")
	fmt.Fprintf(b, "\tForceCommand %s\n", loginSFTPCommand(l))
	b.WriteString("\tPermitTTY no\n")
	b.WriteString("\tDisableForwarding yes\n")
	b.WriteString("\tPermitTunnel no\n")
	b.WriteString("\tPermitUserRC no\n")
}

func parseLoginLine(line string) (Login, error) {
	fields := map[string]string{}
	for _, field := range strings.Fields(strings.TrimPrefix(line, loginMarker)) {
		key, value, ok := strings.Cut(field, "=")
		if !ok {
			return Login{}, fmt.Errorf("malformed login entry %q", line)
		}
		fields[key] = value
	}
	subscription, err1 := strconv.Atoi(fields["subscription"])
	domain, err2 := strconv.Atoi(fields["domain"])
	if err1 != nil || err2 != nil || (fields["readonly"] != "0" && fields["readonly"] != "1") {
		return Login{}, fmt.Errorf("malformed login entry %q", line)
	}
	l := Login{Login: fields["login"], Site: fields["site"], SubscriptionID: subscription, DomainID: domain,
		Folder: fields["folder"], ReadOnly: fields["readonly"] == "1"}
	if err := l.Validate(); err != nil {
		return Login{}, err
	}
	return l, nil
}

// SortedLogins validates logins, refuses duplicates and sorts them by name.
func SortedLogins(logins []Login) ([]Login, error) {
	out := append([]Login(nil), logins...)
	seen := map[string]bool{}
	for _, l := range out {
		if err := l.Validate(); err != nil {
			return nil, err
		}
		if seen[l.Login] {
			return nil, fmt.Errorf("login %s is listed twice", l.Login)
		}
		seen[l.Login] = true
	}
	if len(out) > MaxLoginsPerServer {
		return nil, fmt.Errorf("at most %d logins per server", MaxLoginsPerServer)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Login < out[j].Login })
	return out, nil
}

// FindLogin returns a login's record.
func FindLogin(logins []Login, name string) (Login, bool) {
	for _, l := range logins {
		if l.Login == name {
			return l, true
		}
	}
	return Login{}, false
}

// MergeLogin returns logins with one replaced, added or removed.
func MergeLogin(logins []Login, change Login, keep bool) []Login {
	out := make([]Login, 0, len(logins)+1)
	for _, l := range logins {
		if l.Login != change.Login {
			out = append(out, l)
		}
	}
	if keep {
		out = append(out, change)
	}
	return out
}

// LoginsOfSite lists the logins of one site account.
func LoginsOfSite(logins []Login, site string) []Login {
	var out []Login
	for _, l := range logins {
		if l.Site == site {
			out = append(out, l)
		}
	}
	return out
}
