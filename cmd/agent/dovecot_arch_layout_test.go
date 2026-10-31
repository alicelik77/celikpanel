package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
)

// archPackageDovecotConf is the main file of extra/dovecot 2.4.5-1, as
// extracted from the official package on 2026-10-30 (comments trimmed).
const archPackageDovecotConf = `## Dovecot configuration file
dovecot_config_version = 2.4.5
dovecot_storage_version = 2.4.5
!include_try conf.d/*.conf
protocols {
  imap = yes
  lmtp = yes
}
mail_home = /srv/mail/%{user}
mail_driver = sdbox
mail_path = ~/mail
mail_uid = vmail
mail_gid = vmail
namespace inbox {
  inbox = yes
  separator = /
}
passdb pam {
}
ssl_server {
  cert_file = /etc/dovecot/ssl-cert.pem
  key_file = /etc/dovecot/ssl-key.pem
}
`

func digestOf(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func TestArchDovecotMainClassification(t *testing.T) {
	pkgDigest := digestOf(archPackageDovecotConf)
	managed := renderArchDovecotMain("2.4.5", "2.4.5")
	cases := []struct {
		name    string
		content string
		exists  bool
		digest  string
		want    dovecotArchMainState
	}{
		{"absent", "", false, pkgDigest, dovecotArchMainAbsent},
		{"package file", archPackageDovecotConf, true, pkgDigest, dovecotArchMainPackage},
		{"package file, digest unknown", archPackageDovecotConf, true, "", dovecotArchMainOwnerEdited},
		{"managed", string(managed), true, pkgDigest, dovecotArchMainManaged},
		{"managed then edited", strings.Replace(string(managed), "pop3 = yes", "pop3 = no", 1), true, pkgDigest, dovecotArchMainOwnerIncludesAll},
		{"managed body edited, include removed", strings.Replace(string(managed), "!include_try conf.d/*.conf\n!include_try local.conf\n", "", 1), true, pkgDigest, dovecotArchMainOwnerEdited},
		{"owner edited package file", strings.Replace(archPackageDovecotConf, "sdbox", "mdbox", 1), true, pkgDigest, dovecotArchMainOwnerEdited},
		{"owner file ending with conf.d", "mail_driver = maildir\n!include conf.d/*.conf\n# end\n", true, pkgDigest, dovecotArchMainOwnerIncludesAll},
		{"owner file ending with absolute conf.d", "!include_try /etc/dovecot/conf.d/*.conf\n", true, pkgDigest, dovecotArchMainOwnerIncludesAll},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyArchDovecotMain([]byte(tc.content), tc.exists, tc.digest); got != tc.want {
				t.Fatalf("state=%s want %s", got, tc.want)
			}
		})
	}
}

func TestRenderArchDovecotMainPutsConfDLastAndKeepsVersions(t *testing.T) {
	configVersion, storageVersion, ok := archDovecotVersions([]byte(archPackageDovecotConf))
	if !ok || configVersion != "2.4.5" || storageVersion != "2.4.5" {
		t.Fatalf("versions=%q %q %v", configVersion, storageVersion, ok)
	}
	rendered := renderArchDovecotMain(configVersion, storageVersion)
	if !archDovecotManagedIntact(rendered) {
		t.Fatal("rendered file does not verify its own digest header")
	}
	if !dovecotIncludesConfDLast(rendered) {
		t.Fatal("rendered file does not include conf.d last")
	}
	text := string(rendered)
	for _, forbidden := range []string{"passdb pam", "mail_driver", "mail_home", "sdbox", "separator"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("rendered main file carries the package's %q", forbidden)
		}
	}
	for _, want := range []string{"dovecot_config_version = 2.4.5", "imap = yes", "pop3 = yes", "lmtp = yes", "ssl_server_cert_file = /etc/dovecot/ssl-cert.pem"} {
		if !strings.Contains(text, want) {
			t.Fatalf("rendered main file lacks %q", want)
		}
	}
	// Defaults precede the include so the drop-ins win.
	if strings.Index(text, "ssl_server_cert_file") > strings.Index(text, "!include_try conf.d/*.conf") {
		t.Fatal("TLS default comes after the conf.d include")
	}
}

func TestPacmanLocalPackageDirAndMtreeDigest(t *testing.T) {
	dir, err := pacmanLocalPackageDir("dovecot", []byte("dovecot 2.4.5-1\n"))
	if err != nil || dir != "/var/lib/pacman/local/dovecot-2.4.5-1" {
		t.Fatalf("dir=%q err=%v", dir, err)
	}
	for _, bad := range []string{"", "postfix 3.11-1\n", "dovecot ../x\n", "dovecot 2.4 extra\n", "dovecot 2.4/5\n"} {
		if _, err := pacmanLocalPackageDir("dovecot", []byte(bad)); err == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
	want := digestOf(archPackageDovecotConf)
	mtree := "#mtree\n/set type=file uid=0 gid=0 mode=644\n" +
		"./etc/dovecot time=1.0 type=dir\n" +
		"./etc/dovecot/dovecot.conf time=1.0 size=10 md5digest=00 sha256digest=" + want + "\n" +
		"./etc/pam.d/dovecot time=1.0 size=1 sha256digest=" + strings.Repeat("a", 64) + "\n"
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, _ = zw.Write([]byte(mtree))
	_ = zw.Close()
	got, err := pacmanMtreeSHA256(buf.Bytes(), "/etc/dovecot/dovecot.conf")
	if err != nil || got != want {
		t.Fatalf("digest=%q err=%v", got, err)
	}
	if got, err := pacmanMtreeSHA256(buf.Bytes(), "/etc/dovecot/absent.conf"); err != nil || got != "" {
		t.Fatalf("absent entry digest=%q err=%v", got, err)
	}
	if _, err := pacmanMtreeSHA256([]byte("not gzip"), "/etc/dovecot/dovecot.conf"); err == nil {
		t.Fatal("accepted a non-gzip mtree")
	}
}

func TestOwnerEditedArchDovecotReasonNamesActorAndResume(t *testing.T) {
	for _, part := range []string{"/etc/dovecot/dovecot.conf", "unchanged", "server administrator", "!include_try conf.d/*.conf", "again"} {
		if !strings.Contains(dovecotArchOwnerEditedReason, part) {
			t.Fatalf("reason lacks %q", part)
		}
	}
}

func TestOpenDKIMLayoutFollowsTheDistributionUnit(t *testing.T) {
	apt, ok := opendkimLayoutFor("apt")
	if !ok || apt.confPath != "/etc/opendkim.conf" || !strings.Contains(renderOpenDKIMConf(apt), "TrustAnchorFile\t\t/usr/share/dns/root.key") {
		t.Fatalf("apt layout=%+v", apt)
	}
	arch, ok := opendkimLayoutFor("pacman")
	if !ok || arch.confPath != "/etc/opendkim/opendkim.conf" || len(arch.packages) != 1 || arch.packages[0] != "opendkim" {
		t.Fatalf("pacman layout=%+v", arch)
	}
	if strings.Contains(renderOpenDKIMConf(arch), "TrustAnchorFile") {
		t.Fatal("pacman layout names a root anchor file Arch does not install")
	}
	if _, ok := opendkimLayoutFor("dnf"); ok {
		t.Fatal("dnf layout is not certified")
	}
}

func TestPacmanPHPToolExtensionsDropIn(t *testing.T) {
	loaded := map[string]bool{"mbstring": true}
	written := map[string][]byte{}
	reloads := 0
	ops := pacmanPHPToolOps{
		loaded: func(_ context.Context, ext string) (bool, error) { return loaded[ext], nil },
		read: func(path string) ([]byte, bool, error) {
			content, ok := written[path]
			return content, ok, nil
		},
		write: func(path string, content []byte) error {
			written[path] = content
			for _, line := range strings.Split(string(content), "\n") {
				if ext, ok := strings.CutPrefix(line, "extension="); ok {
					loaded[ext] = true
				}
			}
			return nil
		},
		reload: func(context.Context) error { reloads++; return nil },
	}
	if err := ensurePacmanPHPToolExtensionsWithOps(context.Background(), phpToolPHPMyAdmin, ops); err != nil {
		t.Fatal(err)
	}
	path := pacmanPHPToolDropInPath(phpToolPHPMyAdmin)
	if got := string(written[path]); !strings.Contains(got, "extension=mysqli\n") || !strings.Contains(got, "extension=iconv\n") || reloads != 1 {
		t.Fatalf("drop-in=%q reloads=%d", got, reloads)
	}
	// Second run: everything loaded, nothing written or reloaded.
	if err := ensurePacmanPHPToolExtensionsWithOps(context.Background(), phpToolPHPMyAdmin, ops); err != nil || reloads != 1 {
		t.Fatalf("second run err=%v reloads=%d", err, reloads)
	}
	// An extension the owner already enables is not loaded twice.
	loaded = map[string]bool{"mysqli": true}
	written = map[string][]byte{}
	if err := ensurePacmanPHPToolExtensionsWithOps(context.Background(), phpToolPHPMyAdmin, ops); err != nil {
		t.Fatal(err)
	}
	if got := string(written[path]); strings.Contains(got, "mysqli") || !strings.Contains(got, "iconv") {
		t.Fatalf("drop-in=%q", got)
	}
	// Our file exists but PHP still lacks a module: refused, not rewritten.
	loaded = map[string]bool{}
	written = map[string][]byte{path: []byte("owner text")}
	ops.write = func(string, []byte) error { return errors.New("must not write") }
	err := ensurePacmanPHPToolExtensionsWithOps(context.Background(), phpToolPHPMyAdmin, ops)
	if err == nil || !strings.Contains(err.Error(), "server administrator") {
		t.Fatalf("err=%v", err)
	}
	// Tools without a pacman module list do nothing.
	if err := ensurePacmanPHPToolExtensionsWithOps(context.Background(), phpToolWebmail, ops); err != nil {
		t.Fatal(err)
	}
}
