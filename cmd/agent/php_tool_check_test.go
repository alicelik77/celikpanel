package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/hostplatform"
)

// The PHP check behind a tool's own socket (alpha.104): only a PHP-FPM that
// runs the expected version with the tool's extensions loaded passes; the
// check's location is outside the paths the Panel forwards.

func TestPHPToolCheckResponse(t *testing.T) {
	for _, tc := range []struct {
		version string
		status  int
		body    string
		ok      bool
	}{
		{"8.5", 200, "celikpanel-php-check:ok:8.5.0", true},
		{"8.5", 200, "celikpanel-php-check:ok:8.4.27", false},
		{"8.10", 200, "celikpanel-php-check:ok:8.1.30", false},
		{"8.5", 200, "celikpanel-php-check:missing:pgsql:8.5.0", false},
		{"8.5", 502, "<html>502 Bad Gateway</html>", false},
		{"8.5", 200, "<?php\nheader('Content-Type: text/plain');", false},
		{"", 200, "celikpanel-php-check:ok:8.4.27", true},
	} {
		err := checkPHPToolCheckResponse(phpToolPHPPgAdmin, tc.version, tc.status, []byte(tc.body))
		if (err == nil) != tc.ok {
			t.Fatalf("%s %d %q: err = %v, want ok=%v", tc.version, tc.status, tc.body, err, tc.ok)
		}
	}
	err := checkPHPToolCheckResponse(phpToolPHPPgAdmin, "8.5", 200, []byte("celikpanel-php-check:missing:pgsql,xml:8.5.0"))
	if err == nil || !strings.Contains(err.Error(), "lacks pgsql,xml for phpPgAdmin") {
		t.Fatalf("err = %v", err)
	}
}

func TestPHPToolCheckLocationIsOutsideTheForwardedPaths(t *testing.T) {
	conf := renderDBToolsConfig(
		[]string{phpToolPHPPgAdmin},
		map[string]string{phpToolPHPPgAdmin: "/usr/share/phppgadmin"},
		map[string]string{phpToolPHPPgAdmin: "/run/php/php8.5-fpm.sock"},
	)
	if !strings.Contains(conf, "location = /__celikpanel_php_check/phppgadmin {") ||
		!strings.Contains(conf, `fastcgi_param CELIKPANEL_PHP_EXTENSIONS "pgsql,mbstring,xml";`) {
		t.Fatalf("conf = %s", conf)
	}
	if strings.HasPrefix(phpToolCheckPrefix, "/dbtool/") || strings.HasPrefix(phpToolCheckPrefix, "/webmail/") {
		t.Fatal("the check path is one the Panel forwards")
	}
	// The check's own fastcgi_pass is not read as the tool's binding.
	if got := parseDBToolsFPMSockets(conf)[phpToolPHPPgAdmin]; got != "/run/php/php8.5-fpm.sock" {
		t.Fatalf("binding = %q", got)
	}
	webmail := renderWebmailNginxConfig("/run/celikpanel-webmail.sock", "/var/lib/celikpanel-webmail/public_html", "/run/php/php8.4-fpm.sock")
	if !strings.Contains(webmail, "location = /__celikpanel_php_check/webmail {") || parseWebmailFPMSocket(webmail) != "/run/php/php8.4-fpm.sock" {
		t.Fatalf("webmail = %s", webmail)
	}
}

func TestPHPToolCheckScriptIsWrittenOnceAndReadable(t *testing.T) {
	saved := phpToolCheckScriptPath
	phpToolCheckScriptPath = filepath.Join(t.TempDir(), "check", "check.php")
	t.Cleanup(func() { phpToolCheckScriptPath = saved })
	for i := 0; i < 2; i++ {
		if err := ensurePHPToolCheckScript(); err != nil {
			t.Fatal(err)
		}
	}
	info, err := os.Stat(phpToolCheckScriptPath)
	if err != nil || info.Mode().Perm() != 0o644 {
		t.Fatalf("script = %v %v", info, err)
	}
	data, _ := os.ReadFile(phpToolCheckScriptPath)
	if !strings.Contains(string(data), "extension_loaded") || !strings.Contains(string(data), "PHP_VERSION") {
		t.Fatalf("script = %s", data)
	}
}

func TestPHPToolProbeWaitsForTheReloadedConfiguration(t *testing.T) {
	calls := 0
	probe := func(context.Context, string, string) error {
		calls++
		if calls < 3 {
			return errors.New("phpPgAdmin answered on PHP 8.4.27, not on PHP 8.5")
		}
		return nil
	}
	if err := retryPHPToolProbe(context.Background(), phpToolPHPPgAdmin, "8.5", probe, time.Second, time.Millisecond); err != nil || calls != 3 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
	calls = 0
	failing := func(context.Context, string, string) error { calls++; return errors.New("PHP-FPM 8.5 lacks pgsql") }
	started := time.Now()
	err := retryPHPToolProbe(context.Background(), phpToolPHPPgAdmin, "8.5", failing, 50*time.Millisecond, 10*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "lacks pgsql") || calls < 2 || time.Since(started) > time.Second {
		t.Fatalf("err=%v calls=%d took=%v", err, calls, time.Since(started))
	}
}

func TestOwnerEditedToolConfigurationsAreNotOverwritten(t *testing.T) {
	host := newPHPToolHost(t)
	host.installVersion(t, "8.4", true)
	// alpha.103's form and alpha.104's form are CelikPanel's own, also with
	// an owner's fastcgi_pass edit; anything else is the owner's file.
	roots := map[string]string{phpToolPHPMyAdmin: "/usr/share/phpmyadmin", phpToolPHPPgAdmin: "/usr/share/phppgadmin"}
	sockets := map[string]string{phpToolPHPMyAdmin: "/run/php/php8.4-fpm.sock", phpToolPHPPgAdmin: "/run/php/php8.5-fpm.sock"}
	order := []string{phpToolPHPPgAdmin, phpToolPHPMyAdmin}
	for _, content := range []string{renderDBToolsConfig(order, roots, sockets), legacyRenderDBToolsConfig(order, roots, sockets)} {
		if !managedDBToolsConfig(content) {
			t.Fatalf("CelikPanel's own form was taken for an owner edit:\n%s", content)
		}
		if err := os.WriteFile(host.dbToolsConf, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := refuseOwnerEditedConfig(host.dbToolsConf); err != nil {
			t.Fatal(err)
		}
	}
	edited := legacyRenderDBToolsConfig(order, roots, sockets) + "# my own rate limit\n"
	if err := os.WriteFile(host.dbToolsConf, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	var owner *errPHPToolConfigOwnerEdited
	if err := refuseOwnerEditedConfig(host.dbToolsConf); !errors.As(err, &owner) || !strings.Contains(err.Error(), "changed outside CelikPanel") {
		t.Fatalf("err = %v", err)
	}
	webmail := legacyRenderWebmailNginxConfig("/run/celikpanel-webmail.sock", "/var/lib/celikpanel-webmail/public_html", "/run/php/php8.4-fpm.sock")
	if !managedWebmailConfig(webmail) || !managedWebmailConfig(renderWebmailNginxConfig("/run/celikpanel-webmail.sock", "/var/lib/celikpanel-webmail/public_html", "/run/php/php8.4-fpm.sock")) {
		t.Fatal("webmail forms not recognised")
	}
	if managedWebmailConfig(strings.Replace(webmail, "client_max_body_size 25m;", "client_max_body_size 200m;", 1)) {
		t.Fatal("an owner edit of the webmail server was taken for CelikPanel's form")
	}
}

func TestAStaleRecordDoesNotBlockAReinstall(t *testing.T) {
	host := newPHPToolHost(t)
	host.installVersion(t, "8.4", true)
	host.installVersion(t, "8.6", true)
	// Roundcube was on 8.4, then removed (its line went), then 8.4 was
	// removed, then Roundcube is installed again: the remembered 8.4 is no
	// binding; the default (here the newest, 8.6) is used.
	for _, tool := range []string{phpToolWebmail, phpToolPHPMyAdmin} {
		if err := writePHPToolBinding(tool, "8.4"); err != nil {
			t.Fatal(err)
		}
	}
	host.removeVersion(t, "8.4")
	for _, tool := range []string{phpToolWebmail, phpToolPHPMyAdmin} {
		state := phpToolStateFor(tool)
		if state.Socket != "" {
			t.Fatalf("%s state = %+v", tool, state)
		}
		socket, err := phpToolSocket(tool, state.Socket)
		if err != nil || socket != filepath.Join(host.run, "php8.6-fpm.sock") {
			t.Fatalf("%s socket = %q, %v", tool, socket, err)
		}
	}
}

func TestBindingsSayWhetherTheExtensionPackagesArePresent(t *testing.T) {
	host := newPHPToolHost(t)
	host.installVersion(t, "8.4", true)
	host.writeFrankfurtConfs(t)
	saved := detectHostPlatform
	t.Cleanup(func() { detectHostPlatform = saved })
	detectHostPlatform = func() (hostplatform.Profile, error) {
		return hostplatform.Profile{PackageManager: hostplatform.PackageManagerAPT}, nil
	}
	phpToolPackageInstalled = func(pkg string) bool { return pkg != "php8.4-mysql" }
	got := map[string]bool{}
	for _, binding := range phpToolBindingsView().Tools {
		got[binding.Tool] = binding.ExtensionsPresent
	}
	if got[phpToolPHPMyAdmin] || !got[phpToolWebmail] {
		t.Fatalf("extensions present = %v", got)
	}
}
