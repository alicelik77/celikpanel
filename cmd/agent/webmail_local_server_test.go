//go:build linux

package main

import (
	"context"
	"errors"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.145 (security): webmail's local server (/run/celikpanel-webmail.sock,
// 0666 under /run) gave every account of the server every file of the
// Roundcube tree and ran every .php in it. It now answers only what
// CelikPanel asks. Requests are read with the location selection of
// webmail_address_hardening_test.go (nginx normalises the path first).

const (
	a145Socket = "/run/celikpanel-webmail.sock"
	a145Root   = "/var/lib/celikpanel-webmail/public_html"
	a145FPM    = "/run/php/php8.4-fpm.sock"
)

// a145LocalAnswer says what the local server does with a request for uri
// (a file that exists): php, index (Roundcube's entry point through its
// index), file, or refused.
func a145LocalAnswer(t *testing.T, conf, uri string) string {
	t.Helper()
	i := strings.Index(conf, "server {")
	if i < 0 {
		t.Fatal("no server block")
	}
	locations, _ := ngxLocations(conf[i:])
	clean := path.Clean(uri)
	if strings.HasSuffix(uri, "/") && clean != "/" {
		clean += "/"
	}
	l := a145Select(locations, clean)
	switch {
	case l == nil:
		return "none"
	case regexp.MustCompile(`(?m)^\s*return 404;`).MatchString(l.body):
		return "refused"
	case strings.Contains(l.body, "fastcgi_pass"):
		return "php"
	case strings.Contains(l.body, "index index.php") && !strings.Contains(l.body, "try_files"):
		return "index"
	case strings.Contains(l.body, "try_files $uri"):
		return "file"
	case strings.Contains(l.body, "alias "):
		return "file"
	}
	return "unknown"
}

// a145Select is nginx's choice as ngxSelect makes it, with one more rule the
// local server's earlier form needs: the regular expressions nested in the
// matched prefix location are tried first, then the server's own.
func a145Select(locations []ngxLocation, uri string) *ngxLocation {
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
	var regexes []*ngxLocation
	if prefix != nil {
		for i := range prefix.nested {
			regexes = append(regexes, &prefix.nested[i])
		}
	}
	for i := range locations {
		regexes = append(regexes, &locations[i])
	}
	for _, l := range regexes {
		if (l.modifier == "~" || l.modifier == "~*") && l.matches(uri) {
			return l
		}
	}
	return prefix
}

func TestA145LocalServerAnswersOnlyCelikPanelsOwnChecks(t *testing.T) {
	conf := renderWebmailNginxConfig(a145Socket, a145Root, a145FPM)
	for uri, want := range map[string]string{
		// The Panel's readiness check and the Agent's proof.
		"/webmail/":                                     "index",
		"/webmail/index.php":                            "php",
		phpToolCheckPrefix + phpToolWebmail:             "php",
		"/webmail/skins/elastic/styles/styles.min.css":  "file",
		"/webmail/skins/elastic/deps/bootstrap.min.css": "file",
		// Everything else is not found.
		"/webmail/plugins/password/config.inc.php.bak": "refused",
		"/webmail/plugins/password/config.inc.php":     "refused",
		"/webmail/plugins/archive/archive.php":         "refused",
		"/webmail/.htaccess":                           "refused",
		"/webmail/composer.json":                       "refused",
		"/webmail/program/js/app.min.js":               "refused",
		"/webmail/index.php/extra":                     "refused",
		"/webmail/skins/x/../../plugins/y.php.css":     "refused",
		"/webmail/skins/elastic/.hidden.css":           "refused",
		"/webmail/skins/elastic/templates/login.html":  "refused",
		"/webmail/skins/elastic/watermark.html":        "refused",
		"/webmail/skins/elastic/styles/x.css.php":      "refused",
		"/webmail/plugins/x/skins/elastic/x.css":       "refused",
		"/webmail":                                     "refused",
		"/":                                            "refused",
		"/index.php":                                   "refused",
		"/__celikpanel_php_check/phpmyadmin":           "refused",
	} {
		if got := a145LocalAnswer(t, conf, uri); got != want {
			t.Errorf("%s: %s, want %s", uri, got, want)
		}
	}
	// Roundcube's entry point is the only PHP; its index reaches it.
	if !strings.Contains(conf, "location = /webmail/index.php {\n        alias "+a145Root+"/index.php;") {
		t.Fatalf("the entry point does not run the tree's index.php:\n%s", conf)
	}
	// The baseline: the earlier form gave these out and ran them.
	earlier := alpha144RenderWebmailNginxConfig(a145Socket, a145Root, a145FPM)
	for uri, want := range map[string]string{
		"/webmail/plugins/password/config.inc.php.bak": "file",
		"/webmail/.htaccess":                           "file",
		"/webmail/plugins/archive/archive.php":         "php",
	} {
		if got := a145LocalAnswer(t, earlier, uri); got != want {
			t.Errorf("earlier form %s: %s, want %s", uri, got, want)
		}
	}
	// Still read back as CelikPanel's own, with its listen, root and socket.
	if !managedWebmailConfig(conf) || !managedWebmailConfig(earlier) || parseWebmailFPMSocket(conf) != a145FPM {
		t.Fatal("the forms are not CelikPanel's own")
	}
	if next, err := currentFormPHPToolConfig(phpToolWebmail, earlier); err != nil || next != conf {
		t.Fatalf("the earlier form's current text: %v\n%s", err, next)
	}
}

// The three requests of the Agent's proof and the Panel's readiness check
// still answer against the new block, and none of them needs anything else.
func TestA145ProofAndReadinessStillAnswerOnTheLocalServer(t *testing.T) {
	conf := renderWebmailNginxConfig(a145Socket, a145Root, a145FPM)
	panel, err := os.ReadFile("../panel/webmail_proxy.go")
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`webmailPublicPath\s+= "/webmail/"`).Match(panel) || !strings.Contains(string(panel), `"http://"+webmailInternalHost+webmailPublicPath`) {
		t.Fatal("the Panel's readiness check no longer asks /webmail/")
	}
	if got := a145LocalAnswer(t, conf, "/webmail/"); got != "index" {
		t.Fatalf("readiness: %s", got)
	}
	if got := a145LocalAnswer(t, conf, phpToolCheckPrefix+phpToolWebmail); got != "php" {
		t.Fatalf("PHP check: %s", got)
	}
	// Roundcube 1.6's elastic login page names its two style sheets so; the
	// proof asks for the "styles" one, without its query.
	page := []byte(`<link rel="stylesheet" href="skins/elastic/deps/bootstrap.min.css?s=1760000000">` +
		`<link rel="stylesheet" href="skins/elastic/styles/styles.min.css?s=1760000000">`)
	address, err := webmailStylesheetURL("http://webmail.internal", page)
	if err != nil {
		t.Fatal(err)
	}
	uri := strings.TrimPrefix(strings.SplitN(address, "?", 2)[0], "http://webmail.internal")
	if uri != "/webmail/skins/elastic/styles/styles.min.css" || a145LocalAnswer(t, conf, uri) != "file" {
		t.Fatalf("stylesheet %s: %s", uri, a145LocalAnswer(t, conf, uri))
	}
}

func a145PointLocalServer(t *testing.T) string {
	t.Helper()
	conf, _ := pointPHPToolConfs(t)
	savedScript := phpToolCheckScriptPath
	t.Cleanup(func() {
		phpToolCheckScriptPath = savedScript
		webmailAddressRewriteRefusalsMu.Lock()
		delete(webmailAddressRewriteRefusals, webmailLocalServerHost)
		webmailAddressRewriteRefusalsMu.Unlock()
	})
	phpToolCheckScriptPath = filepath.Join(t.TempDir(), "check", "check.php")
	return conf
}

func a145LocalForms(t *testing.T) []transport.WebmailAddressForm {
	t.Helper()
	form, open := webmailLocalServerForm()
	if !open {
		return nil
	}
	return []transport.WebmailAddressForm{form}
}

func TestA145EarlierLocalServerIsRewrittenAtStartWithOneReload(t *testing.T) {
	conf := a145PointLocalServer(t)
	earlier := alpha144RenderWebmailNginxConfig(a145Socket, a145Root, a145FPM)
	mustWriteTestFile(t, conf, []byte(earlier), 0o600)
	var commands []string
	refuse := true
	run := func(_ context.Context, name string, args ...string) ([]byte, error) {
		commands = append(commands, name+" "+strings.Join(args, " "))
		if refuse && name == "nginx" {
			return []byte(`nginx: [emerg] unknown directive "oops" in /etc/nginx/conf.d/broken.conf:3`), errors.New("exit status 1")
		}
		return nil, nil
	}
	// The configuration test fails (another file): the earlier file comes
	// back as it was, nginx is not reloaded, the start goes on, and Problems
	// now names the file the test named.
	said := reconcileWebmailLocalServerAtStartup(context.Background(), run)
	if len(said) != 1 || !strings.Contains(said[0], "kept in its earlier form") {
		t.Fatalf("said %q", said)
	}
	if got, _ := os.ReadFile(conf); string(got) != earlier {
		t.Fatal("the earlier file was not put back")
	}
	for _, command := range commands {
		if strings.HasPrefix(command, "systemctl") {
			t.Fatalf("reloaded after a failed test: %q", commands)
		}
	}
	if forms := a145LocalForms(t); len(forms) != 1 || forms[0].Host != "" || forms[0].State != "test_failed" || forms[0].File != "/etc/nginx/conf.d/broken.conf" {
		t.Fatalf("forms: %+v", forms)
	}
	// The next start succeeds: one test, one reload; the line goes.
	refuse, commands = false, nil
	said = reconcileWebmailLocalServerAtStartup(context.Background(), run)
	if len(said) != 1 || !strings.Contains(said[0], "rewritten in the current form") {
		t.Fatalf("said %q", said)
	}
	if got, _ := os.ReadFile(conf); string(got) != renderWebmailNginxConfig(a145Socket, a145Root, a145FPM) {
		t.Fatalf("not the current form:\n%s", got)
	}
	if strings.Join(commands, "; ") != "nginx -t; systemctl reload nginx" {
		t.Fatalf("commands: %q", commands)
	}
	if forms := a145LocalForms(t); len(forms) != 0 {
		t.Fatalf("still listed: %+v", forms)
	}
	commands = nil
	if said := reconcileWebmailLocalServerAtStartup(context.Background(), run); len(said) != 0 || len(commands) != 0 {
		t.Fatalf("a second start: %q %q", said, commands)
	}
	// alpha.1 to alpha.12's loopback form also becomes the current one.
	mustWriteTestFile(t, conf, []byte(loopbackRenderWebmailNginxConfig("127.0.0.1:8090", a145Root, a145FPM)), 0o600)
	if said := reconcileWebmailLocalServerAtStartup(context.Background(), run); len(said) != 1 || !strings.Contains(said[0], "rewritten") {
		t.Fatalf("loopback form: %q", said)
	}
	if got, _ := os.ReadFile(conf); !strings.Contains(string(got), webmailLocalServerMarker) || strings.Contains(string(got), "127.0.0.1:8090") {
		t.Fatalf("loopback form not moved:\n%s", got)
	}
}

func TestA145OwnersLocalServerFileIsKeptWithTheTextBesideIt(t *testing.T) {
	conf := a145PointLocalServer(t)
	owners := strings.Replace(alpha144RenderWebmailNginxConfig(a145Socket, a145Root, a145FPM), "client_max_body_size 25m;", "client_max_body_size 50m;", 1)
	mustWriteTestFile(t, conf, []byte(owners), 0o600)
	var commands []string
	run := func(_ context.Context, name string, args ...string) ([]byte, error) {
		commands = append(commands, name)
		return nil, nil
	}
	said := reconcileWebmailLocalServerAtStartup(context.Background(), run)
	pending := conf + webmailAddressPendingSuffix
	if len(said) != 1 || !strings.Contains(said[0], "kept as it is") || len(commands) != 0 {
		t.Fatalf("said %q commands %q", said, commands)
	}
	if got, _ := os.ReadFile(conf); string(got) != owners {
		t.Fatal("the owner's file changed")
	}
	text, err := os.ReadFile(pending)
	if err != nil || string(text) != renderWebmailNginxConfig(a145Socket, a145Root, a145FPM) {
		t.Fatalf("CelikPanel's text beside it: %v\n%s", err, text)
	}
	if info, _ := os.Stat(pending); info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", info.Mode())
	}
	if forms := a145LocalForms(t); len(forms) != 1 || forms[0].State != "owner_edited" || forms[0].File != pending {
		t.Fatalf("forms: %+v", forms)
	}
	// The Forms RPC lists it among the address forms, with no address.
	var resp transport.WebmailAddressFormsResponse
	if err := (&Agent{}).WebmailAddressFormsV1(&transport.WebmailAddressFormsRequest{}, &resp); err != nil {
		t.Fatal(err)
	}
	listed := false
	for _, open := range resp.Open {
		listed = listed || (open.Host == "" && open.State == "owner_edited" && open.File == pending)
	}
	if !listed {
		t.Fatalf("not listed: %+v", resp.Open)
	}
	// The owner takes the text: the line goes, the text beside it goes at the next start.
	mustWriteTestFile(t, conf, text, 0o600)
	if forms := a145LocalForms(t); len(forms) != 0 {
		t.Fatalf("still listed: %+v", forms)
	}
	if said := reconcileWebmailLocalServerAtStartup(context.Background(), run); len(said) != 0 {
		t.Fatalf("said %q", said)
	}
	if _, err := os.Stat(pending); !os.IsNotExist(err) {
		t.Fatalf("the text beside it stayed: %v", err)
	}
}

// Second review of PR #357, findings 1 and 6: only another file the web
// server's test named is named; CelikPanel's own file, a failed reload or a
// check script that could not be written are said without a file (the
// journal and a restart of the Agent).
func TestA145LocalServerRewriteFailuresThatAreNotAnotherFileAreUnnamed(t *testing.T) {
	conf := a145PointLocalServer(t)
	earlier := alpha144RenderWebmailNginxConfig(a145Socket, a145Root, a145FPM)
	for name, run := range map[string]webmailCommandRunner{
		"own file named": func(_ context.Context, name string, _ ...string) ([]byte, error) {
			if name == "nginx" {
				return []byte(`nginx: [emerg] unknown directive "x" in ` + conf + `:12`), errors.New("exit status 1")
			}
			return nil, nil
		},
		"reload failed": func(_ context.Context, name string, args ...string) ([]byte, error) {
			if name == "systemctl" && len(args) > 0 && args[0] == "reload" {
				return []byte("nginx.service is not active, cannot reload."), errors.New("exit status 1")
			}
			return nil, nil
		},
	} {
		mustWriteTestFile(t, conf, []byte(earlier), 0o600)
		if said := reconcileWebmailLocalServerAtStartup(context.Background(), run); len(said) != 1 || !strings.Contains(said[0], "kept in its earlier form") {
			t.Fatalf("%s: said %q", name, said)
		}
		if got, _ := os.ReadFile(conf); string(got) != earlier {
			t.Fatalf("%s: the earlier file was not put back", name)
		}
		if forms := a145LocalForms(t); len(forms) != 1 || forms[0].State != "test_failed" || forms[0].File != "" {
			t.Fatalf("%s: forms %+v", name, forms)
		}
	}
	// The PHP check script cannot be written (its folder is a file).
	blocker := filepath.Join(t.TempDir(), "file")
	mustWriteTestFile(t, blocker, []byte("x"), 0o600)
	phpToolCheckScriptPath = filepath.Join(blocker, "check.php")
	// (The earlier text names the check script's path: rendered again.)
	earlier = alpha144RenderWebmailNginxConfig(a145Socket, a145Root, a145FPM)
	mustWriteTestFile(t, conf, []byte(earlier), 0o600)
	ok := func(context.Context, string, ...string) ([]byte, error) { return nil, nil }
	if said := reconcileWebmailLocalServerAtStartup(context.Background(), ok); len(said) != 1 || !strings.Contains(said[0], "kept in its earlier form") {
		t.Fatalf("check script: said %q", said)
	}
	if forms := a145LocalForms(t); len(forms) != 1 || forms[0].File != "" {
		t.Fatalf("check script: forms %+v", forms)
	}
}

// Second review of PR #357, finding 4: CelikPanel's text beside the local
// server's file goes, by name, when the file it was for is gone.
func TestA145LocalServerPendingTextGoesWithTheFile(t *testing.T) {
	conf := a145PointLocalServer(t)
	pending := conf + webmailAddressPendingSuffix
	mustWriteTestFile(t, pending, []byte("CelikPanel's text"), 0o600)
	if said := reconcileWebmailLocalServerAtStartup(context.Background(), func(context.Context, string, ...string) ([]byte, error) { return nil, nil }); len(said) != 0 {
		t.Fatalf("said %q", said)
	}
	if _, err := os.Stat(pending); !os.IsNotExist(err) {
		t.Fatalf("the text beside a missing file stayed: %v", err)
	}
	// Webmail removed: the same by-name removal (a link is left alone).
	mustWriteTestFile(t, pending, []byte("CelikPanel's text"), 0o600)
	removeWebmailLocalServerPending()
	if _, err := os.Stat(pending); !os.IsNotExist(err) {
		t.Fatalf("not removed with webmail: %v", err)
	}
	target := filepath.Join(t.TempDir(), "owner")
	mustWriteTestFile(t, target, []byte("owner"), 0o600)
	if err := os.Symlink(target, pending); err != nil {
		t.Fatal(err)
	}
	removeWebmailLocalServerPending()
	if _, err := os.Lstat(pending); err != nil {
		t.Fatalf("a link of that name was removed: %v", err)
	}
}
