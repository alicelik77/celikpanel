package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.144 (security): webmail's own address served every file of
// Roundcube's linked folders (plugins, skins, program) as it is and ran
// every .php in them. The address file is read here the way nginx selects a
// location (an exact match first; otherwise the regular expressions in
// their order, a nested one inside the matching one; otherwise the longest
// prefix) and each request is answered by what the selected location does.
// The configuration's syntax itself is checked by the distribution matrix's
// real nginx (TestRenderDistroConfigMatrix renders this file).

type ngxLocation struct {
	modifier, pattern string
	// body is the location's own directives, its nested locations left out.
	body   string
	nested []ngxLocation
}

var ngxLocationHead = regexp.MustCompile(`(?m)^\s*location\s+(=|~\*|~|\^~)?\s*(\S+)\s*\{`)

// ngxLocations reads the location blocks directly inside text (one level)
// and what of text lies outside them.
func ngxLocations(text string) ([]ngxLocation, string) {
	var out []ngxLocation
	var outside strings.Builder
	for len(text) > 0 {
		m := ngxLocationHead.FindStringSubmatchIndex(text)
		if m == nil {
			break
		}
		outside.WriteString(text[:m[0]])
		start := m[1]
		depth, end := 1, start
		for end < len(text) && depth > 0 {
			switch text[end] {
			case '{':
				depth++
			case '}':
				depth--
			}
			end++
		}
		nested, own := ngxLocations(text[start : end-1])
		loc := ngxLocation{body: own, pattern: text[m[4]:m[5]], nested: nested}
		if m[2] >= 0 {
			loc.modifier = text[m[2]:m[3]]
		}
		out = append(out, loc)
		text = text[end:]
	}
	outside.WriteString(text)
	return out, outside.String()
}

func (l ngxLocation) matches(uri string) bool {
	switch l.modifier {
	case "=":
		return uri == l.pattern
	case "~":
		return regexp.MustCompile(l.pattern).MatchString(uri)
	case "~*":
		return regexp.MustCompile("(?i)" + l.pattern).MatchString(uri)
	}
	return strings.HasPrefix(uri, l.pattern)
}

// ngxSelect is nginx's choice among locations for uri.
func ngxSelect(locations []ngxLocation, uri string) *ngxLocation {
	var prefix *ngxLocation
	for i := range locations {
		l := &locations[i]
		if l.modifier == "=" && l.matches(uri) {
			return l
		}
		if (l.modifier == "" || l.modifier == "^~") && l.matches(uri) && (prefix == nil || len(l.pattern) > len(prefix.pattern)) {
			prefix = l
		}
	}
	if prefix != nil && prefix.modifier == "^~" {
		return prefix
	}
	for i := range locations {
		l := &locations[i]
		if (l.modifier == "~" || l.modifier == "~*") && l.matches(uri) {
			if inner := ngxSelect(l.nested, uri); inner != nil {
				return inner
			}
			return l
		}
	}
	return prefix
}

// ngxAnswer says what the location does with a file that exists.
func ngxAnswer(l *ngxLocation) string {
	switch {
	case l == nil:
		return "none"
	case strings.Contains(l.body, "deny all"):
		return "refused"
	case strings.Contains(l.body, "fastcgi_pass"):
		return "php"
	case regexp.MustCompile(`(?m)^\s*return 404;`).MatchString(l.body):
		return "refused"
	case strings.Contains(l.body, "try_files $uri =404"):
		return "file"
	case strings.Contains(l.body, "try_files $uri $uri/"):
		return "file"
	}
	return "unknown"
}

func webmailAddressServerBlock(t *testing.T, current bool) []ngxLocation {
	t.Helper()
	text := renderWebmailAddressConfigForm("webmail.example.test", "", "/opt/celikpanel/webmail/public_html", "/run/php/php8.4-fpm.sock",
		false, services.NginxHTTP2Off, webmailAddressLegacyTLS, current)
	i := strings.Index(text, "listen 443")
	if i < 0 {
		t.Fatal("no HTTPS server")
	}
	locations, _ := ngxLocations(text[i:])
	return locations
}

var webmailRefusedExamples = []string{
	"/plugins/password/config.inc.php.bak",
	"/plugins/password/config.inc.php.dist",
	"/plugins/password/config.inc.php",
	"/plugins/password/config.inc.php.20261101",
	"/plugins/password/config.inc.php~",
	"/plugins/password/password.php",
	"/plugins/password/drivers/sql.php",
	"/plugins/managesieve/dump.sql",
	"/plugins/example/debug.log",
	"/plugins/example/settings.ini",
	"/plugins/example/settings.yml",
	"/plugins/example/composer.json",
	"/plugins/example/.env",
	"/plugins/example/.git/config",
	"/skins/elastic/templates/mail.php",
	"/skins/elastic/meta.json",
	"/program/include/rcmail.php",
	"/program/localization/en_US/labels.inc",
	"/static.php",
	"/.htaccess",
	"/index.php.bak",
}

var webmailServedExamples = map[string]string{
	"/plugins/zipdownload/zipdownload.js":           "file",
	"/plugins/archive/skins/elastic/archive.css":    "file",
	"/plugins/archive/skins/elastic/foldericon.png": "file",
	"/skins/elastic/styles/styles.min.css":          "file",
	"/skins/elastic/fonts/roboto-regular.woff2":     "file",
	"/skins/elastic/images/logo.svg":                "file",
	"/skins/elastic/watermark.html":                 "file",
	"/program/js/app.min.js":                        "file",
	"/program/resources/blank.tiff":                 "file",
	"/plugins/newmail_notifier/sound.mp3":           "file",
	"/index.php":                                    "php",
	"/":                                             "file",
}

func TestWebmailAddressServesOnlyRoundcubesStaticFilesAndEntryPoint(t *testing.T) {
	locations := webmailAddressServerBlock(t, true)
	for _, uri := range webmailRefusedExamples {
		if got := ngxAnswer(ngxSelect(locations, uri)); got != "refused" {
			t.Errorf("%s: %s, want refused", uri, got)
		}
	}
	for uri, want := range webmailServedExamples {
		if got := ngxAnswer(ngxSelect(locations, uri)); got != want {
			t.Errorf("%s: %s, want %s", uri, got, want)
		}
	}
	// CelikPanel's PHP check stays answered (local only).
	if got := ngxAnswer(ngxSelect(locations, phpToolCheckPrefix+phpToolWebmail)); got != "refused" && got != "php" {
		t.Errorf("the PHP check: %s", got)
	}
}

// The form of alpha.143 and earlier gave the plugin's copied configuration
// out as a file and ran a plugin's PHP: what this release closes.
func TestWebmailAddressEarlierFormServedPluginFiles(t *testing.T) {
	locations := webmailAddressServerBlock(t, false)
	for uri, want := range map[string]string{
		"/plugins/password/config.inc.php.bak": "file",
		"/plugins/managesieve/dump.sql":        "file",
		"/plugins/password/password.php":       "php",
	} {
		if got := ngxAnswer(ngxSelect(locations, uri)); got != want {
			t.Errorf("earlier form, %s: %s, want %s", uri, got, want)
		}
	}
}

// Both forms are CelikPanel's own: the earlier one is read back as stale and
// rewritten once at start; an owner's file is kept and the current text goes
// beside it.
func TestWebmailAddressEarlierFormIsRewrittenAtStartAndAnOwnersFileKept(t *testing.T) {
	root := t.TempDir()
	savedDir, savedHTTP2 := webmailAddressConfDir, webmailAddressHTTP2Form
	webmailAddressConfDir = filepath.Join(root, "conf.d")
	webmailAddressHTTP2Form = func() string { return services.NginxHTTP2Off }
	t.Cleanup(func() { webmailAddressConfDir, webmailAddressHTTP2Form = savedDir, savedHTTP2 })
	if err := os.MkdirAll(webmailAddressConfDir, 0o755); err != nil {
		t.Fatal(err)
	}
	tls := webmailAddressTLSLines()
	earlier := renderWebmailAddressConfigForm("old.example.test", "", "/opt/celikpanel/webmail/public_html", "/run/php/php8.4-fpm.sock", false, services.NginxHTTP2Off, tls, false)
	form, managed := parseWebmailAddressConfig("old.example.test", earlier)
	if !managed || !form.stale() || !form.openLayout {
		t.Fatalf("the earlier form read back: managed=%v %+v", managed, form)
	}
	owners := strings.Replace(earlier, "client_max_body_size 25m;", "client_max_body_size 50m;", 1)
	if err := os.WriteFile(webmailAddressConfPath("old.example.test"), []byte(earlier), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(webmailAddressConfPath("own.example.test"), []byte(strings.ReplaceAll(owners, "old.example.test", "own.example.test")), 0o600); err != nil {
		t.Fatal(err)
	}
	var commands []string
	run := func(_ context.Context, name string, args ...string) ([]byte, error) {
		commands = append(commands, name+" "+strings.Join(args, " "))
		return nil, nil
	}
	said := reconcileWebmailAddressesAtStartup(context.Background(), run)
	written, _ := os.ReadFile(webmailAddressConfPath("old.example.test"))
	if form, managed := parseWebmailAddressConfig("old.example.test", string(written)); !managed || form.stale() {
		t.Fatalf("not rewritten in the current form: %v %+v\n%s", said, form, written)
	}
	if len(commands) != 2 || !strings.HasPrefix(commands[0], "nginx -t") || !strings.HasPrefix(commands[1], "systemctl reload nginx") {
		t.Fatalf("checked and reloaded: %v", commands)
	}
	kept, _ := os.ReadFile(webmailAddressConfPath("own.example.test"))
	if !strings.Contains(string(kept), "client_max_body_size 50m;") {
		t.Fatal("the owner's file was changed")
	}
	pending, err := os.ReadFile(webmailAddressConfPath("own.example.test") + webmailAddressPendingSuffix)
	if err != nil || !strings.Contains(string(pending), "location ~ ^/(plugins|skins|program)/") {
		t.Fatalf("CelikPanel's text beside the owner's file: %v", err)
	}
	// A second start changes nothing.
	commands = nil
	if again := reconcileWebmailAddressesAtStartup(context.Background(), run); len(again) != 0 || len(commands) != 0 {
		t.Fatalf("second start: %v %v", again, commands)
	}
}

// Review of PR #355, finding 2: a file the start could not bring to the
// current form is listed for Problems now with why, and goes once it is.
func TestWebmailAddressFormsNameWhatStillServesPluginFiles(t *testing.T) {
	root := t.TempDir()
	savedDir, savedHTTP2 := webmailAddressConfDir, webmailAddressHTTP2Form
	webmailAddressConfDir = filepath.Join(root, "conf.d")
	webmailAddressHTTP2Form = func() string { return services.NginxHTTP2Off }
	t.Cleanup(func() {
		webmailAddressConfDir, webmailAddressHTTP2Form = savedDir, savedHTTP2
		webmailAddressRewriteRefusalsMu.Lock()
		webmailAddressRewriteRefusals = map[string]string{}
		webmailAddressRewriteRefusalsMu.Unlock()
	})
	if err := os.MkdirAll(webmailAddressConfDir, 0o755); err != nil {
		t.Fatal(err)
	}
	tls := webmailAddressTLSLines()
	render := func(host string, current bool) string {
		return renderWebmailAddressConfigForm(host, "", "/opt/celikpanel/webmail/public_html", "/run/php/php8.4-fpm.sock", false, services.NginxHTTP2Off, tls, current)
	}
	_ = os.WriteFile(webmailAddressConfPath("old.example.test"), []byte(render("old.example.test", false)), 0o600)
	_ = os.WriteFile(webmailAddressConfPath("own.example.test"), []byte(strings.Replace(render("own.example.test", false), "25m", "50m", 1)), 0o600)
	_ = os.WriteFile(webmailAddressConfPath("new.example.test"), []byte(render("new.example.test", true)), 0o600)
	// The start's rewrite of old.example.test was put back: nginx -t named another file.
	run := func(_ context.Context, name string, args ...string) ([]byte, error) {
		if name == "nginx" {
			return []byte(`nginx: [emerg] unknown directive "oops" in /etc/nginx/conf.d/broken.conf:3`), errors.New("exit status 1")
		}
		return nil, nil
	}
	reconcileWebmailAddressesAtStartup(context.Background(), run)
	var resp transport.WebmailAddressFormsResponse
	if err := (&Agent{}).WebmailAddressFormsV1(&transport.WebmailAddressFormsRequest{}, &resp); err != nil {
		t.Fatal(err)
	}
	got := map[string]transport.WebmailAddressForm{}
	for _, open := range resp.Open {
		got[open.Host] = open
	}
	if len(got) != 2 || got["old.example.test"].State != "test_failed" || got["old.example.test"].File != "/etc/nginx/conf.d/broken.conf" ||
		got["own.example.test"].State != "owner_edited" || !strings.HasSuffix(got["own.example.test"].File, webmailAddressPendingSuffix) {
		t.Fatalf("listed: %+v", resp.Open)
	}
	// The owner takes CelikPanel's text: the line goes by itself.
	pending, _ := os.ReadFile(webmailAddressConfPath("own.example.test") + webmailAddressPendingSuffix)
	_ = os.WriteFile(webmailAddressConfPath("own.example.test"), pending, 0o600)
	resp = transport.WebmailAddressFormsResponse{}
	_ = (&Agent{}).WebmailAddressFormsV1(&transport.WebmailAddressFormsRequest{}, &resp)
	for _, open := range resp.Open {
		if open.Host == "own.example.test" {
			t.Fatalf("still listed after the owner took the text: %+v", open)
		}
	}
}
