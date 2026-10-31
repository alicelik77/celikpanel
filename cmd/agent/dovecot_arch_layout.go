package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

// Arch Linux's dovecot package (2.4) ships ONE /etc/dovecot/dovecot.conf
// whose own mail storage (sdbox under /srv/mail), PAM login and TLS settings
// come AFTER its `!include_try conf.d/*.conf` line, and no conf.d directory.
// In Dovecot 2.4 a later setting wins, so CelikPanel's conf.d drop-ins
// (99-celikpanel.conf, 98-celikpanel-tls.conf, 97-celikpanel-submission.conf)
// would be overridden by the package's own lines. Debian's layout is the
// reverse: a main file that only includes conf.d.
//
// The Arch layout therefore gives the main file the Debian shape, and only
// when that is the owner's untouched choice (D-022, D-031):
//
//   - absent, or byte-identical to the file the installed package recorded
//     (pacman's own mtree digest): CelikPanel writes its main file, which
//     keeps the package's config/storage version lines, enables imap, pop3
//     and lmtp, keeps the default TLS pair and includes conf.d LAST, so the
//     drop-ins are what Dovecot uses;
//   - CelikPanel's own main file whose digest header still matches its body:
//     kept (rewritten only when the rendering changed);
//   - an owner file whose last effective line includes conf.d: kept as it is,
//     because the drop-ins already win there;
//   - anything else: kept, never overwritten, and the operation stops with
//     the owner's two choices named.
//
// The file stays a native Dovecot configuration: Dovecot runs from it with or
// without CelikPanel, and pacman keeps it as a modified backup file on every
// upgrade (a newer package text arrives as dovecot.conf.pacnew).
//
// Arch'ın dovecot paketi tek bir dovecot.conf getirir; kendi posta, PAM ve
// TLS ayarları conf.d include'undan SONRA gelir ve 2.4'te sonraki ayar
// kazanır. Arch yerleşimi ana dosyaya Debian biçimini verir: yalnız dosya
// yoksa ya da paketin kaydettiği haliyle bayt bayt aynıysa yazılır; sahibin
// değiştirdiği dosya korunur ve adıyla söylenir.

const (
	dovecotMainConfPath    = "/etc/dovecot/dovecot.conf"
	dovecotConfDirPath     = "/etc/dovecot/conf.d"
	dovecotArchHeaderStart = "# Managed by CelikPanel: Dovecot main file, Arch Linux layout. sha256="
	pacmanLocalDBDir       = "/var/lib/pacman/local"
)

type dovecotArchMainState string

const (
	dovecotArchMainAbsent           dovecotArchMainState = "absent"
	dovecotArchMainPackage          dovecotArchMainState = "package_file"
	dovecotArchMainManaged          dovecotArchMainState = "managed"
	dovecotArchMainOwnerIncludesAll dovecotArchMainState = "owner_includes_conf_d_last"
	dovecotArchMainOwnerEdited      dovecotArchMainState = "owner_edited"
)

// dovecotArchOwnerEditedReason is the D-024 text when the main file is the
// owner's: what CelikPanel found, that it changed nothing, who acts and how
// the same operation resumes.
const dovecotArchOwnerEditedReason = "CelikPanel left /etc/dovecot/dovecot.conf unchanged: it differs from the file the Arch Linux dovecot package installed and from CelikPanel's own main file, so it holds the server administrator's settings. Mail is not configured yet. The server administrator chooses one of two ways, then runs the same mail setup again: either end the file with the line `!include_try conf.d/*.conf` so CelikPanel's settings in /etc/dovecot/conf.d apply last, or move the file aside (for example `sudo mv /etc/dovecot/dovecot.conf /etc/dovecot/dovecot.conf.mine`) so CelikPanel writes its main file."

var dovecotVersionLine = regexp.MustCompile(`^(dovecot_config_version|dovecot_storage_version)\s*=\s*([0-9][0-9A-Za-z.+~-]*)\s*$`)

// renderArchDovecotMain renders CelikPanel's main file. The digest header
// covers every byte after the first line.
func renderArchDovecotMain(configVersion, storageVersion string) []byte {
	body := fmt.Sprintf(`# This file replaces the Arch Linux package's single-file layout with the
# Debian shape: defaults first, then every file in conf.d, so CelikPanel's
# drop-ins (and yours) apply last. Dovecot runs from it without CelikPanel.
# CelikPanel does not rewrite this file once you edit it; it keeps your
# version and tells you so. A newer package text arrives as dovecot.conf.pacnew.
# Put your own settings in /etc/dovecot/local.conf or a conf.d file.
dovecot_config_version = %s
dovecot_storage_version = %s

protocols {
  imap = yes
  pop3 = yes
  lmtp = yes
}

namespace inbox {
  inbox = yes
}

ssl = yes
ssl_server_cert_file = /etc/dovecot/ssl-cert.pem
ssl_server_key_file = /etc/dovecot/ssl-key.pem
ssl_min_protocol = TLSv1.2

!include_try conf.d/*.conf
!include_try local.conf
`, configVersion, storageVersion)
	sum := sha256.Sum256([]byte(body))
	return []byte(dovecotArchHeaderStart + hex.EncodeToString(sum[:]) + "\n" + body)
}

// archDovecotVersions reads the two version lines a 2.4 main file must carry.
func archDovecotVersions(content []byte) (string, string, bool) {
	var configVersion, storageVersion string
	for _, line := range strings.Split(string(content), "\n") {
		match := dovecotVersionLine.FindStringSubmatch(strings.TrimSpace(line))
		if match == nil {
			continue
		}
		switch match[1] {
		case "dovecot_config_version":
			if configVersion == "" {
				configVersion = match[2]
			}
		case "dovecot_storage_version":
			if storageVersion == "" {
				storageVersion = match[2]
			}
		}
	}
	return configVersion, storageVersion, configVersion != "" && storageVersion != ""
}

// archDovecotManagedIntact reports whether content is CelikPanel's main file
// with an unchanged body.
func archDovecotManagedIntact(content []byte) bool {
	text := string(content)
	newline := strings.IndexByte(text, '\n')
	if newline < 0 || !strings.HasPrefix(text, dovecotArchHeaderStart) {
		return false
	}
	want := strings.TrimPrefix(text[:newline], dovecotArchHeaderStart)
	sum := sha256.Sum256([]byte(text[newline+1:]))
	return want == hex.EncodeToString(sum[:])
}

// dovecotIncludesConfDLast reports whether a main file includes conf.d after
// every setting (only further include lines, such as local.conf, may follow),
// so CelikPanel's drop-ins apply last.
func dovecotIncludesConfDLast(content []byte) bool {
	seenConfD := false
	for _, line := range strings.Split(string(content), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		fields := strings.Fields(trimmed)
		isInclude := len(fields) == 2 && (fields[0] == "!include_try" || fields[0] == "!include")
		if isInclude && (fields[1] == "conf.d/*.conf" || fields[1] == dovecotConfDirPath+"/*.conf") {
			seenConfD = true
			continue
		}
		if seenConfD && !isInclude {
			seenConfD = false
		}
	}
	return seenConfD
}

// classifyArchDovecotMain decides what the main file is. packageDigest is the
// SHA-256 the installed package recorded for the file ("" when unknown).
func classifyArchDovecotMain(content []byte, exists bool, packageDigest string) dovecotArchMainState {
	if !exists {
		return dovecotArchMainAbsent
	}
	if archDovecotManagedIntact(content) {
		return dovecotArchMainManaged
	}
	if packageDigest != "" {
		sum := sha256.Sum256(content)
		if hex.EncodeToString(sum[:]) == packageDigest {
			return dovecotArchMainPackage
		}
	}
	if dovecotIncludesConfDLast(content) {
		return dovecotArchMainOwnerIncludesAll
	}
	return dovecotArchMainOwnerEdited
}

var pacmanVersionPattern = regexp.MustCompile(`^[0-9A-Za-z._+:~-]+$`)

// pacmanLocalPackageDir names the local database entry of an installed
// package from `pacman -Q <name>` output.
func pacmanLocalPackageDir(name string, queryOutput []byte) (string, error) {
	fields := strings.Fields(strings.TrimSpace(string(queryOutput)))
	if len(fields) != 2 || fields[0] != name || !pacmanVersionPattern.MatchString(fields[1]) ||
		strings.Contains(fields[1], "..") {
		return "", fmt.Errorf("pacman returned a non-canonical status for package %s", name)
	}
	return pacmanLocalDBDir + "/" + name + "-" + fields[1], nil
}

// pacmanMtreeSHA256 returns the sha256digest a package's gzip-compressed
// mtree recorded for one installed path ("/etc/..."), or "" when the entry
// carries none.
func pacmanMtreeSHA256(mtreeGzip []byte, path string) (string, error) {
	reader, err := gzip.NewReader(bytes.NewReader(mtreeGzip))
	if err != nil {
		return "", fmt.Errorf("read pacman mtree: %w", err)
	}
	defer reader.Close()
	plain, err := io.ReadAll(io.LimitReader(reader, 16<<20))
	if err != nil {
		return "", fmt.Errorf("read pacman mtree: %w", err)
	}
	want := "." + path
	scanner := bufio.NewScanner(bytes.NewReader(plain))
	scanner.Buffer(make([]byte, 64<<10), 1<<20)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 0 || fields[0] != want {
			continue
		}
		for _, field := range fields[1:] {
			if digest, ok := strings.CutPrefix(field, "sha256digest="); ok && len(digest) == 64 {
				return digest, nil
			}
		}
		return "", nil
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read pacman mtree: %w", err)
	}
	return "", nil
}

// archDovecotPackageDigest asks the verified pacman for the installed dovecot
// and reads the digest its local database recorded for dovecot.conf. An
// unreadable record is "" (unknown), never "pristine".
func archDovecotPackageDigest() string {
	profile, err := verifiedHostProfile("pacman")
	if err != nil {
		return ""
	}
	pacman, err := executableForProfile(profile, "pacman", "pacman")
	if err != nil {
		return ""
	}
	out, err := runMailTLSCommand(pacman, "-Q", "--", "dovecot")
	if err != nil {
		return ""
	}
	dir, err := pacmanLocalPackageDir("dovecot", out)
	if err != nil {
		return ""
	}
	mtree, err := secureReadConfig(dir + "/mtree")
	if err != nil {
		return ""
	}
	digest, err := pacmanMtreeSHA256(mtree, dovecotMainConfPath)
	if err != nil {
		return ""
	}
	return digest
}

// dovecotNeedsArchLayout is true on a verified pacman host.
func dovecotNeedsArchLayout() bool {
	return detectPkgFamily() == "pacman"
}

// ensureArchDovecotLayout makes the conf.d drop-ins effective on Arch. It
// returns the main file's snapshot so the caller's rollback can restore it.
func ensureArchDovecotLayout() (mailFileSnapshot, error) {
	snapshot, err := snapshotMailFile(dovecotMainConfPath)
	if err != nil {
		return mailFileSnapshot{}, fmt.Errorf("snapshot dovecot main configuration: %w", err)
	}
	if err := ensureDovecotConfDir(); err != nil {
		return snapshot, err
	}
	state := classifyArchDovecotMain(snapshot.content, snapshot.exists, archDovecotPackageDigest())
	switch state {
	case dovecotArchMainOwnerIncludesAll:
		return snapshot, nil
	case dovecotArchMainOwnerEdited:
		return snapshot, errors.New(dovecotArchOwnerEditedReason)
	}
	configVersion, storageVersion, ok := archDovecotVersions(snapshot.content)
	if !ok {
		version, err := installedDovecotVersion()
		if err != nil {
			return snapshot, err
		}
		configVersion, storageVersion = version, version
	}
	rendered := renderArchDovecotMain(configVersion, storageVersion)
	if snapshot.exists && bytes.Equal(snapshot.content, rendered) {
		return snapshot, nil
	}
	if err := secureWriteConfig(dovecotMainConfPath, rendered, 0o644); err != nil {
		return snapshot, fmt.Errorf("write dovecot main configuration: %w", err)
	}
	if err := secureSetMailFileMetadata(dovecotMainConfPath, 0o644, 0, 0); err != nil {
		if restoreErr := restoreMailFile(snapshot); restoreErr != nil {
			return snapshot, fmt.Errorf("%w; restore dovecot main configuration: %v", err, restoreErr)
		}
		return snapshot, err
	}
	return snapshot, nil
}

// installedDovecotVersion reads `dovecot --version` ("2.4.5 (abc123)") for a
// main file that carries no version lines (an absent file).
func installedDovecotVersion() (string, error) {
	out, err := runMailTLSCommand("dovecot", "--version")
	if err != nil {
		return "", fmt.Errorf("read the installed Dovecot version: %w", err)
	}
	fields := strings.Fields(string(out))
	if len(fields) == 0 || !dovecotVersionLine.MatchString("dovecot_config_version = "+fields[0]) {
		return "", errors.New("read the installed Dovecot version: unrecognised output")
	}
	return fields[0], nil
}

// ensureDovecotConfDir creates /etc/dovecot/conf.d (root, 0755) when absent.
// A symbolic link or a non-directory is refused.
func ensureDovecotConfDir() error {
	info, err := os.Lstat(dovecotConfDirPath)
	if err == nil {
		if !info.IsDir() {
			return fmt.Errorf("%s exists but is not a directory; CelikPanel left it unchanged", dovecotConfDirPath)
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect %s: %w", dovecotConfDirPath, err)
	}
	parent, err := os.Lstat("/etc/dovecot")
	if err != nil || !parent.IsDir() {
		return errors.New("/etc/dovecot is missing or is not a directory; reinstall Dovecot with the operating system's package manager")
	}
	if err := os.Mkdir(dovecotConfDirPath, 0o755); err != nil && !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("create %s: %w", dovecotConfDirPath, err)
	}
	return os.Chmod(dovecotConfDirPath, 0o755)
}
