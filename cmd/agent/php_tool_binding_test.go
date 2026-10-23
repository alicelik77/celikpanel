package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type phpToolHost struct {
	run, etc, bin, webmailConf, dbToolsConf string
}

func newPHPToolHost(t *testing.T) phpToolHost {
	t.Helper()
	root := t.TempDir()
	host := phpToolHost{
		run:         filepath.Join(root, "run", "php"),
		etc:         filepath.Join(root, "etc", "php"),
		bin:         filepath.Join(root, "usr", "bin"),
		webmailConf: filepath.Join(root, "nginx", "celikpanel-webmail.conf"),
		dbToolsConf: filepath.Join(root, "nginx", "celikpanel-dbtools.conf"),
	}
	for _, dir := range []string{host.run, host.etc, host.bin, filepath.Dir(host.webmailConf)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	saved := []string{phpFPMRunDir, phpEtcDir, phpCLIAlternative, phpToolWebmailConfPath, phpToolDBToolsConfPath, phpToolBindingRecordPath}
	savedFallback := phpFallbackSocketGlobs
	savedMetas, savedInstalled, savedPackage := phpDefaultVersionFromMetas, phpToolInstalledCheck, phpToolPackageInstalled
	phpFPMRunDir, phpEtcDir = host.run, host.etc
	phpCLIAlternative = filepath.Join(host.bin, "php")
	phpToolWebmailConfPath, phpToolDBToolsConfPath = host.webmailConf, host.dbToolsConf
	phpToolBindingRecordPath = filepath.Join(root, "php-tool-bindings.json")
	phpFallbackSocketGlobs = nil
	phpDefaultVersionFromMetas = func() string { return "" }
	phpToolInstalledCheck = func(string) bool { return true }
	phpToolPackageInstalled = func(string) bool { return false }
	t.Cleanup(func() {
		phpFPMRunDir, phpEtcDir, phpCLIAlternative, phpToolWebmailConfPath, phpToolDBToolsConfPath, phpToolBindingRecordPath = saved[0], saved[1], saved[2], saved[3], saved[4], saved[5]
		phpFallbackSocketGlobs = savedFallback
		phpDefaultVersionFromMetas, phpToolInstalledCheck, phpToolPackageInstalled = savedMetas, savedInstalled, savedPackage
	})
	return host
}

func (h phpToolHost) installVersion(t *testing.T, version string, running bool) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(h.etc, version, "fpm"), 0o755); err != nil {
		t.Fatal(err)
	}
	if running {
		if err := os.WriteFile(filepath.Join(h.run, "php"+version+"-fpm.sock"), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func (h phpToolHost) removeVersion(t *testing.T, version string) {
	t.Helper()
	_ = os.RemoveAll(filepath.Join(h.etc, version))
	_ = os.Remove(filepath.Join(h.run, "php"+version+"-fpm.sock"))
}

// The generated configurations as Frankfurt had them on 2026-10-23.
func (h phpToolHost) writeFrankfurtConfs(t *testing.T) {
	t.Helper()
	sock := filepath.Join(h.run, "php8.4-fpm.sock")
	if err := os.WriteFile(h.webmailConf, []byte(renderWebmailNginxConfig("/run/celikpanel-webmail.sock", "/var/lib/celikpanel-webmail/public_html", sock)), 0o644); err != nil {
		t.Fatal(err)
	}
	conf := "# Managed by CelikPanel — database web tools, loopback only.\nserver {\n    listen 127.0.0.1:8306;\n" +
		"\n    location /dbtool/phpmyadmin/ {\n        alias /usr/share/phpmyadmin/;\n        location ~ ^/dbtool/phpmyadmin/(.+\\.php)$ {\n            fastcgi_pass unix:" + sock + ";\n        }\n    }\n" +
		"\n    location /dbtool/phppgadmin/ {\n        alias /usr/share/phppgadmin/;\n        location ~ ^/dbtool/phppgadmin/(.+\\.php)$ {\n            fastcgi_pass unix:" + filepath.Join(h.run, "php8.5-fpm.sock") + ";\n        }\n    }\n}\n"
	if err := os.WriteFile(h.dbToolsConf, []byte(conf), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPHPToolsKeepTheirBoundVersionWhenANewerOneIsInstalled(t *testing.T) {
	host := newPHPToolHost(t)
	for _, version := range []string{"8.4", "8.5", "8.6"} {
		host.installVersion(t, version, true)
	}
	host.writeFrankfurtConfs(t)
	bound := currentPHPToolSockets()
	want := map[string]string{
		phpToolWebmail:    filepath.Join(host.run, "php8.4-fpm.sock"),
		phpToolPHPMyAdmin: filepath.Join(host.run, "php8.4-fpm.sock"),
		phpToolPHPPgAdmin: filepath.Join(host.run, "php8.5-fpm.sock"),
	}
	if !reflect.DeepEqual(bound, want) {
		t.Fatalf("bound = %v, want %v", bound, want)
	}
	for tool, socket := range want {
		got, err := phpToolSocket(tool, bound[tool])
		if err != nil || got != socket {
			t.Fatalf("%s socket = %q, %v; want %q (no move to 8.6)", tool, got, err, socket)
		}
	}
	if got := phpToolsBoundToVersion("8.4"); !reflect.DeepEqual(got, []string{"phpmyadmin", "webmail"}) {
		t.Fatalf("bound to 8.4 = %v", got)
	}
	if got := phpToolsBoundToVersion("8.6"); len(got) != 0 {
		t.Fatalf("bound to 8.6 = %v", got)
	}
	if got := phpToolsBoundToVersion(""); len(got) != 3 {
		t.Fatalf("bound to any = %v", got)
	}
}

func TestPHPToolOnAStoppedButInstalledVersionStaysThere(t *testing.T) {
	host := newPHPToolHost(t)
	host.installVersion(t, "8.4", false)
	host.installVersion(t, "8.6", true)
	sock := filepath.Join(host.run, "php8.4-fpm.sock")
	if got, err := phpToolSocket(phpToolWebmail, sock); err != nil || got != sock {
		t.Fatalf("socket = %q, %v", got, err)
	}
}

func TestPHPToolWhoseVersionIsGoneIsReportedNotMoved(t *testing.T) {
	host := newPHPToolHost(t)
	host.installVersion(t, "8.4", true)
	host.installVersion(t, "8.6", true)
	host.writeFrankfurtConfs(t)
	host.removeVersion(t, "8.4")
	got, err := phpToolSocket(phpToolWebmail, currentPHPToolSockets()[phpToolWebmail])
	var gone *errPHPToolVersionGone
	if !errors.As(err, &gone) || gone.Version != "8.4" {
		t.Fatalf("err = %v", err)
	}
	if got != filepath.Join(host.run, "php8.4-fpm.sock") {
		t.Fatalf("socket = %q, want the bound one unchanged", got)
	}
	if !strings.Contains(err.Error(), "install PHP-FPM 8.4 again") || !strings.Contains(err.Error(), "Manage window") {
		t.Fatalf("message = %q", err.Error())
	}
}

func TestNothingBoundPrefersTheDistributionDefaultPHP(t *testing.T) {
	host := newPHPToolHost(t)
	for _, version := range []string{"8.4", "8.5", "8.6"} {
		host.installVersion(t, version, true)
	}
	// The php-fpm.sock alternative points at 8.4 (Debian 13's default).
	if err := os.Symlink(filepath.Join(host.run, "php8.4-fpm.sock"), filepath.Join(host.run, "php-fpm.sock")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if got, err := phpToolSocket(phpToolPHPMyAdmin, ""); err != nil || got != filepath.Join(host.run, "php8.4-fpm.sock") {
		t.Fatalf("socket = %q, %v; want the versioned default, not the alias", got, err)
	}
	// Without the socket alternative, the php command's alternative.
	_ = os.Remove(filepath.Join(host.run, "php-fpm.sock"))
	if err := os.WriteFile(filepath.Join(host.bin, "php8.5"), nil, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(host.bin, "php8.5"), phpCLIAlternative); err != nil {
		t.Fatal(err)
	}
	if got, _ := phpToolSocket(phpToolPHPMyAdmin, ""); got != filepath.Join(host.run, "php8.5-fpm.sock") {
		t.Fatalf("socket = %q, want 8.5 from the php alternative", got)
	}
}

func TestNewestSocketIsNumericNotLexical(t *testing.T) {
	host := newPHPToolHost(t)
	host.installVersion(t, "8.9", true)
	host.installVersion(t, "8.10", true)
	if got := newestVersionedFPMSocket(); got != filepath.Join(host.run, "php8.10-fpm.sock") {
		t.Fatalf("newest = %q", got)
	}
	if got, _ := phpToolSocket(phpToolWebmail, ""); got != filepath.Join(host.run, "php8.10-fpm.sock") {
		t.Fatalf("default without alternatives = %q", got)
	}
}

func TestUnversionedArchSocketIsStillFound(t *testing.T) {
	host := newPHPToolHost(t)
	arch := filepath.Join(filepath.Dir(host.run), "php-fpm", "php-fpm.sock")
	if err := os.MkdirAll(filepath.Dir(arch), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(arch, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	phpFallbackSocketGlobs = []string{arch}
	if got, err := phpToolSocket(phpToolWebmail, ""); err != nil || got != arch {
		t.Fatalf("socket = %q, %v", got, err)
	}
	if _, err := phpToolSocket(phpToolWebmail, filepath.Join(host.run, "missing.sock")); err == nil {
		t.Fatal("a missing unversioned bound socket was accepted")
	}
}

func TestBindPHPToolSocketSeamValidates(t *testing.T) {
	host := newPHPToolHost(t)
	host.installVersion(t, "8.5", true)
	if got, err := bindPHPToolSocket(phpToolWebmail, "8.5"); err != nil || got != filepath.Join(host.run, "php8.5-fpm.sock") {
		t.Fatalf("bind = %q, %v", got, err)
	}
	for _, bad := range []struct{ tool, version string }{{phpToolWebmail, "8.4"}, {phpToolWebmail, "../x"}, {"wordpress", "8.5"}} {
		if _, err := bindPHPToolSocket(bad.tool, bad.version); err == nil {
			t.Fatalf("bind(%q, %q) accepted", bad.tool, bad.version)
		}
	}
}

func TestPHPCLIForVersion(t *testing.T) {
	host := newPHPToolHost(t)
	if err := os.WriteFile(filepath.Join(host.bin, "php8.4"), nil, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := phpCLIForVersion("8.4"); got != filepath.Join(host.bin, "php8.4") {
		t.Fatalf("cli = %q", got)
	}
	if got := phpCLIForVersion("8.6"); got != "php" {
		t.Fatalf("cli for a missing version = %q", got)
	}
}
