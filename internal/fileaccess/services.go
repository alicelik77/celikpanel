package fileaccess

import (
	"crypto/rsa"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/crypto/ssh"
)

// ---------------------------------------------------------------------------
// vsftpd

// VsftpdParams are the host facts vsftpd.conf depends on.
type VsftpdParams struct {
	CertFile string
	KeyFile  string
	// SecureChrootDir is vsftpd's empty, root-owned directory: Debian and
	// Ubuntu use /var/run/vsftpd/empty, Arch /usr/share/empty.
	SecureChrootDir string
}

// SecureChrootDirFor returns the distro's empty directory for vsftpd.
func SecureChrootDirFor(family string) string {
	if family == "pacman" {
		return "/usr/share/empty"
	}
	return "/var/run/vsftpd/empty"
}

// RenderVsftpdConf renders vsftpd.conf. Plain FTP is never accepted: the
// login and every data connection must use TLS. Only accounts in the user
// list may sign in, each chrooted to its subscription's sites folder.
func RenderVsftpdConf(p VsftpdParams) (string, error) {
	for _, value := range []string{p.CertFile, p.KeyFile, p.SecureChrootDir} {
		if !strings.HasPrefix(value, "/") || strings.ContainsAny(value, " \t\n\r") {
			return "", fmt.Errorf("invalid vsftpd path %q", value)
		}
	}
	var b strings.Builder
	b.WriteString("# FTP over TLS for hosting accounts with file access. Plain FTP is refused.\n")
	b.WriteString("# CelikPanel rewrites this file only while it is exactly as CelikPanel wrote\n")
	b.WriteString("# it. Edit it by hand and CelikPanel keeps your version and says so.\n")
	lines := []string{
		"listen=YES",
		"listen_ipv6=NO",
		"anonymous_enable=NO",
		"local_enable=YES",
		"write_enable=YES",
		"local_umask=027",
		"file_open_mode=0666",
		"dirmessage_enable=NO",
		"use_localtime=YES",
		"xferlog_enable=YES",
		"xferlog_std_format=NO",
		"vsftpd_log_file=" + VsftpdLogPath,
		"connect_from_port_20=NO",
		"port_enable=NO",
		"pasv_enable=YES",
		fmt.Sprintf("pasv_min_port=%d", PassiveMin),
		fmt.Sprintf("pasv_max_port=%d", PassiveMax),
		"pasv_promiscuous=NO",
		"chroot_local_user=YES",
		"allow_writeable_chroot=NO",
		"user_config_dir=" + VsftpdUserConfDir,
		"secure_chroot_dir=" + p.SecureChrootDir,
		"pam_service_name=" + VsftpdPAMService,
		"userlist_enable=YES",
		"userlist_deny=NO",
		"userlist_file=" + VsftpdUserList,
		"hide_ids=NO",
		"ssl_enable=YES",
		"allow_anon_ssl=NO",
		"force_local_logins_ssl=YES",
		"force_local_data_ssl=YES",
		"require_ssl_reuse=YES",
		"ssl_sslv2=NO",
		"ssl_sslv3=NO",
		"ssl_tlsv1=YES",
		"ssl_ciphers=HIGH",
		"rsa_cert_file=" + p.CertFile,
		"rsa_private_key_file=" + p.KeyFile,
		"max_clients=200",
		"max_per_ip=10",
		"max_login_fails=3",
		"delay_failed_login=3",
		"idle_session_timeout=600",
		"data_connection_timeout=120",
	}
	b.WriteString(strings.Join(lines, "\n"))
	b.WriteString("\n")
	return Seal("FTP over TLS (vsftpd)", b.String()), nil
}

// RenderVsftpdUserList lists the accounts vsftpd lets sign in.
func RenderVsftpdUserList(entries []Entry) (string, error) {
	sorted, err := sortedEntries(entries)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("# Hosting accounts with file access; vsftpd refuses everyone else.\n")
	for _, e := range sorted {
		b.WriteString(e.Username + "\n")
	}
	return Seal("FTP accounts", b.String()), nil
}

// RenderVsftpdUserConf is one account's vsftpd setting: the folder it is
// chrooted to (root-owned, so vsftpd never needs a writable root).
func RenderVsftpdUserConf(e Entry) (string, error) {
	if err := e.Validate(); err != nil {
		return "", err
	}
	return Seal("FTP folder of "+e.Username, "local_root="+ChrootDir(e.SubscriptionID)+"\n"), nil
}

// RenderVsftpdPAM is vsftpd's own PAM service. It does not use pam_shells,
// so an account without a command line can still use FTP, and it accepts
// only the accounts in the user list.
func RenderVsftpdPAM(family string) string {
	var b strings.Builder
	b.WriteString("#%PAM-1.0\n")
	b.WriteString("# FTP sign-in for hosting accounts with file access.\n")
	b.WriteString("auth\trequired\tpam_listfile.so item=user sense=deny file=/etc/ftpusers onerr=succeed\n")
	b.WriteString("auth\trequired\tpam_listfile.so item=user sense=allow file=" + VsftpdUserList + " onerr=fail\n")
	if family == "pacman" {
		b.WriteString("auth\tinclude\tsystem-auth\naccount\tinclude\tsystem-auth\nsession\tinclude\tsystem-auth\n")
	} else {
		b.WriteString("@include common-auth\n@include common-account\n@include common-session\n")
	}
	// PAM ignores comment lines, so the header can be the first line.
	return Seal("FTP sign-in (PAM)", b.String())
}

// ---------------------------------------------------------------------------
// Fail2ban

// RenderFail2banJail is the vsftpd jail. The sshd jail is the distro's own
// (Debian and Ubuntu enable it; CelikPanel enables it on Arch when Fail2ban
// is installed), so it is reported, not written here.
func RenderFail2banJail() string {
	body := fmt.Sprintf(`# Bans addresses that fail FTP sign-in again and again.
[vsftpd]
enabled = true
port = ftp,ftp-data,ftps,ftps-data,%d:%d
filter = vsftpd
logpath = %s
backend = auto
maxretry = 5
findtime = 10m
bantime = 1h
`, PassiveMin, PassiveMax, VsftpdLogPath)
	return Seal("Fail2ban jail for FTP", body)
}

// ---------------------------------------------------------------------------
// SSH keys

// Key is one accepted public key.
type Key struct {
	Type        string `json:"type"`
	Fingerprint string `json:"fingerprint"`
	Comment     string `json:"comment"`
	Line        string `json:"-"`
}

var commentPattern = regexp.MustCompile(`[^A-Za-z0-9@._+-]`)

// MaxKeys bounds one account's key list.
const MaxKeys = 20

// ParsePublicKey accepts one OpenSSH public key line. Options before the
// key are refused (CelikPanel sets them), DSA and RSA under 2048 bits are
// refused, and the comment is cut to a short safe text.
func ParsePublicKey(text string) (Key, error) {
	text = strings.TrimSpace(text)
	if text == "" || len(text) > 16*1024 || strings.ContainsAny(text, "\r\n") {
		return Key{}, errors.New("paste one public key on one line")
	}
	key, comment, options, rest, err := ssh.ParseAuthorizedKey([]byte(text))
	if err != nil {
		return Key{}, errors.New("this is not an OpenSSH public key (it starts with ssh-ed25519, ecdsa-sha2-… or ssh-rsa)")
	}
	if len(options) > 0 || len(rest) > 0 {
		return Key{}, errors.New("paste the key without options in front of it")
	}
	switch key.Type() {
	case ssh.KeyAlgoED25519, ssh.KeyAlgoECDSA256, ssh.KeyAlgoECDSA384, ssh.KeyAlgoECDSA521,
		ssh.KeyAlgoSKED25519, ssh.KeyAlgoSKECDSA256:
	case ssh.KeyAlgoRSA:
		crypto, ok := key.(ssh.CryptoPublicKey)
		if !ok {
			return Key{}, errors.New("the RSA key could not be read")
		}
		rsaKey, ok := crypto.CryptoPublicKey().(*rsa.PublicKey)
		if !ok || rsaKey.N.BitLen() < 2048 {
			return Key{}, errors.New("RSA keys must have at least 2048 bits; an Ed25519 key is recommended")
		}
	default:
		return Key{}, fmt.Errorf("key type %s is not accepted; use Ed25519", key.Type())
	}
	comment = commentPattern.ReplaceAllString(comment, "")
	if len(comment) > 64 {
		comment = comment[:64]
	}
	line := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key)))
	if comment != "" {
		line += " " + comment
	}
	return Key{Type: key.Type(), Fingerprint: ssh.FingerprintSHA256(key), Comment: comment, Line: line}, nil
}

// RenderKeysFile renders an account's authorized_keys. The options keep a
// file-only account to SFTP even if sshd's own settings were loosened.
func RenderKeysFile(keys []Key, sshRight bool) string {
	options := "restrict"
	if sshRight {
		options = "restrict,pty"
	}
	sorted := append([]Key(nil), keys...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Fingerprint < sorted[j].Fingerprint })
	var b strings.Builder
	b.WriteString("# Keys for this hosting account; add or remove them in CelikPanel's Access tab.\n")
	for _, key := range sorted {
		b.WriteString(options + " " + key.Line + "\n")
	}
	return Seal("SSH keys", b.String())
}

// ParseKeysFile reads the keys of a verified keys file body.
func ParseKeysFile(body string) ([]Key, error) {
	var keys []Key
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		_, rest, ok := strings.Cut(line, " ")
		if !ok {
			return nil, errors.New("malformed keys file line")
		}
		key, err := ParsePublicKey(rest)
		if err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, nil
}

// ---------------------------------------------------------------------------
// Jail for the command line

// JailCommands are the programs an account with the ssh right finds in its
// jail. Basic file work only: no compiler, no package manager, no sudo, su
// or other setuid program, no ps (and no /proc). A program the host does
// not have is left out and reported.
var JailCommands = []string{
	"bash", "sh", "ls", "cp", "mv", "rm", "mkdir", "rmdir", "cat", "chmod", "touch", "ln",
	"head", "tail", "less", "grep", "sed", "awk", "find", "xargs", "sort", "uniq", "cut", "tr",
	"wc", "diff", "du", "tar", "gzip", "gunzip", "zcat", "unzip", "zip", "nano", "vi",
	"id", "whoami", "env", "basename", "dirname", "date", "stat", "realpath", "readlink",
	"md5sum", "sha256sum", "tee", "true", "false", "sleep", "clear", "scp", "rsync",
}

// JailTerminfo are the terminal descriptions less, nano and clear need.
var JailTerminfo = []string{
	"x/xterm", "x/xterm-256color", "l/linux", "v/vt100", "v/vt220",
	"s/screen", "s/screen-256color", "t/tmux", "t/tmux-256color",
}

// JailDirs are the jail's own top-level names in the sites folder. Site
// folders are numbers, so the names never meet a site.
var JailDirs = []string{"bin", "dev", "etc", "lib", "lib32", "lib64", "sbin", "tmp", "usr"}

// JailUser is one account the jail's passwd names.
type JailUser struct {
	Name     string
	UID, GID int
	Group    string
	DomainID int
}

// RenderJailPasswd renders the jail's /etc/passwd: root and the jail's
// accounts, each with its own site folder as home.
func RenderJailPasswd(users []JailUser) string {
	sorted := append([]JailUser(nil), users...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	var b strings.Builder
	b.WriteString("root:x:0:0:root:/:/usr/sbin/nologin\n")
	for _, u := range sorted {
		fmt.Fprintf(&b, "%s:x:%d:%d::%s:%s\n", u.Name, u.UID, u.GID, StartDir(u.DomainID), JailShell)
	}
	return b.String()
}

// RenderJailGroup renders the jail's /etc/group.
func RenderJailGroup(users []JailUser, webGroup string, webGID int) string {
	groups := map[string]int{"root": 0}
	for _, u := range users {
		if u.Group != "" {
			groups[u.Group] = u.GID
		}
	}
	if webGroup != "" && webGID > 0 {
		groups[webGroup] = webGID
	}
	names := make([]string, 0, len(groups))
	for name := range groups {
		names = append(names, name)
	}
	sort.Strings(names)
	var b strings.Builder
	for _, name := range names {
		fmt.Fprintf(&b, "%s:x:%d:\n", name, groups[name])
	}
	return b.String()
}

// RenderJailProfile renders the jail's /etc/profile: the account's own folder
// as home and start folder, and a umask that keeps files out of reach of
// other accounts.
func RenderJailProfile(users []JailUser) string {
	sorted := append([]JailUser(nil), users...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	var b strings.Builder
	b.WriteString("# Command line of a hosting account, closed to its subscription's folder.\n")
	b.WriteString("umask 027\nPATH=/usr/bin:/bin\nexport PATH\n")
	b.WriteString("case \"$USER\" in\n")
	for _, u := range sorted {
		fmt.Fprintf(&b, "\t%s) HOME=%s ;;\n", u.Name, StartDir(u.DomainID))
	}
	b.WriteString("esac\nexport HOME\ncd \"$HOME\" 2>/dev/null\n")
	b.WriteString("PS1='\\u:\\w\\$ '\n")
	return b.String()
}
