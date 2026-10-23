package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
