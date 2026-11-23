//go:build linux

package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// S4 R1, fourth fix round (23 Nov 2026): the narrow review of round 3. Each
// test fails on 336c1da28, or does not build there.

// Finding 2: the review's case. apache2.conf includes a folder outside
// /etc/apache2 through a variable; a site there hands PHP to the stock
// socket. The include itself keeps the pool on, named by its line, and a
// later include outside the root is still followed. On 336c1da28 the
// fallback read only /etc/apache2: no user, the pool was turned off.
func TestAnApacheIncludeThatCannotBeFollowedKeepsThePoolOn(t *testing.T) {
	root := apacheTree(t)
	sites := filepath.Join(t.TempDir(), "apache-sites")
	writeRound3File(t, filepath.Join(sites, "shop.conf"), "<VirtualHost *:8080>\n  SetHandler \"proxy:unix:/run/php/php8.2-fpm.sock|fcgi://localhost\"\n</VirtualHost>\n")
	other := filepath.Join(t.TempDir(), "more")
	writeRound3File(t, filepath.Join(other, "blog.conf"), "SetHandler \"proxy:unix:/run/php/php8.3-fpm.sock|fcgi://localhost\"\n")
	writeRound3File(t, filepath.Join(root, "apache2.conf"),
		"IncludeOptional ${SITES}/*.conf\nIncludeOptional conf-enabled/*.conf\nInclude "+filepath.Join(other, "*.conf")+"\n")
	files := apacheLoadedFiles(root, true)
	include := filepath.Join(root, "apache2.conf") + ": IncludeOptional ${SITES}/*.conf"
	if got := stockPoolUsers("", "/run/php/php8.2-fpm.sock", "", files, nil); !reflect.DeepEqual(got, []string{include}) {
		t.Fatalf("8.2: %v", got)
	}
	got := stockPoolUsers("", "/run/php/php8.3-fpm.sock", "", files, nil)
	if len(got) != 2 || got[0] != include || got[1] != filepath.Join(other, "blog.conf") {
		t.Fatalf("8.3: the later include outside the root was not followed: %v", got)
	}
}

// Finding 3: other web servers and Apache's .htaccess overrides keep the
// pools on; a default Apache (AllowOverride None) is read as before; units
// that cannot be listed keep them on.
func TestOtherWebServersKeepTheStockPoolOn(t *testing.T) {
	root := apacheTree(t)
	second := filepath.Join(t.TempDir(), "apache2-second")
	writeRound3File(t, filepath.Join(second, "apache2.conf"), "<Directory /srv/blog>\n  AllowOverride FileInfo\n</Directory>\n")
	writeRound3File(t, filepath.Join(root, "apache2.conf"), "<Directory />\n  AllowOverride None\n</Directory>\n# AllowOverride All\n")
	load := func(r string, installed bool) []stockPoolConfigText {
		switch r {
		case "/etc/apache2":
			return apacheLoadedFiles(root, installed)
		case "/etc/apache2-second":
			return apacheLoadedFiles(second, installed)
		}
		return nil
	}
	names := func(files []stockPoolConfigText) []string {
		var out []string
		for _, f := range files {
			if f.Err != nil {
				out = append(out, f.Name)
			}
		}
		return out
	}
	// A default Apache, enabled: its files are read, nothing else counts.
	if got := names(webServerUsers(map[string]webServerUnit{"apache2.service": {Enabled: true}}, nil, true, load)); len(got) != 0 {
		t.Fatalf("a default Apache: %v", got)
	}
	// The packaged snippet alone is still not a user (finding 1 of round 3).
	files := webServerUsers(map[string]webServerUnit{"apache2.service": {Enabled: true, Active: true}}, nil, true, load)
	if got := stockPoolUsers("", "/run/php/php8.4-fpm.sock", "", files, nil); len(got) != 0 {
		t.Fatalf("the packaged snippet: %v", got)
	}
	// An enabled second instance whose configuration allows overrides.
	got := names(webServerUsers(map[string]webServerUnit{"apache2@second.service": {Active: true}}, nil, false, load))
	if len(got) != 1 || !strings.HasSuffix(got[0], "apache2.conf: AllowOverride FileInfo") {
		t.Fatalf("overrides: %v", got)
	}
	// Another front end, enabled or active; httpd instances too.
	units := map[string]webServerUnit{"lighttpd.service": {Enabled: true}, "caddy.service": {Active: true}, "httpd@x.service": {Active: true}, "lshttpd.service": {}}
	got = names(webServerUsers(units, nil, false, load))
	if !reflect.DeepEqual(got, []string{"lighttpd.service (enabled)", "caddy.service (active)", "httpd@x.service (active)"}) {
		t.Fatalf("other web servers: %v", got)
	}
	// The units could not be listed.
	got = names(webServerUsers(nil, errors.New("System has not been booted with systemd"), false, load))
	if len(got) != 1 || got[0] != "systemctl" {
		t.Fatalf("no units: %v", got)
	}
	// Every such entry keeps the pool on and is named.
	if used := stockPoolUsers("", "/run/php/php8.4-fpm.sock", "", webServerUsers(units, nil, false, load), nil); len(used) != 3 {
		t.Fatalf("users: %v", used)
	}
}

func TestSystemctlShowIsReadPerUnit(t *testing.T) {
	text := "Id=apache2.service\nActiveState=inactive\nUnitFileState=disabled\n\nUnitFileState=enabled\nId=lighttpd.service\nActiveState=inactive\n\nId=caddy.service\nActiveState=active\nUnitFileState=\n\nId=httpd.service\nActiveState=inactive\nUnitFileState=\n"
	got := parseWebServerUnits(text)
	want := map[string]webServerUnit{"apache2.service": {}, "lighttpd.service": {Enabled: true}, "caddy.service": {Active: true}, "httpd.service": {}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%+v", got)
	}
	if _, err := os.Stat("/run/systemd/system"); err == nil {
		// On a systemd host the real call answers for every unit asked.
		units, err := hostWebServerUnits(t.Context())
		if err != nil || len(units) < 1+len(otherWebServerUnits) {
			t.Fatalf("systemctl: %v %v", units, err)
		}
	}
}
