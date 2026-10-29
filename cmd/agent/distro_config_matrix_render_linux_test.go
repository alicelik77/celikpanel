//go:build linux

package main

// TestRenderDistroConfigMatrix renders every configuration file and command
// the Agent writes or runs for a native program, through the Agent's own
// generators, into CELIKPANEL_CONFIG_MATRIX_OUT (internal/configmatrix case
// format). deploy/test-distro-config-matrix.sh runs this test binary INSIDE a
// clean container of each supported release, after installing that release's
// packages, so every host probe (Dovecot dialect, Postfix map type, PHP-FPM
// layout, stock nginx.conf / named.conf.options / pdns.conf / 10-auth.conf)
// reads the real distribution. It then runs each program's own validator.
//
// Without CELIKPANEL_CONFIG_MATRIX_OUT the test is skipped: ordinary
// `go test ./...` runs never read the host.

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/bindconfig"
	"github.com/alicelik/celikpanel/internal/binddns"
	"github.com/alicelik/celikpanel/internal/configmatrix"
	"github.com/alicelik/celikpanel/internal/firewallpolicy"
	"github.com/alicelik/celikpanel/internal/firewallruntime"
	"github.com/alicelik/celikpanel/internal/hostingpath"
	"github.com/alicelik/celikpanel/internal/hostplatform"
	"github.com/alicelik/celikpanel/internal/mailrenewalkit"
	"github.com/alicelik/celikpanel/internal/pdnsmanagedconf"
	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

type matrixHost struct {
	out, repo, release, family, distroID, codename, phpVersion string
	cases                                                      int
}

func newMatrixHost(t *testing.T) *matrixHost {
	t.Helper()
	out := configmatrix.OutDir()
	if out == "" {
		t.Skip("set CELIKPANEL_CONFIG_MATRIX_OUT (deploy/test-distro-config-matrix.sh) to render the distro config matrix")
	}
	h := &matrixHost{
		out:        out,
		repo:       os.Getenv("CELIKPANEL_CONFIG_MATRIX_REPO"),
		release:    os.Getenv("CELIKPANEL_CONFIG_MATRIX_RELEASE"),
		family:     os.Getenv("CELIKPANEL_CONFIG_MATRIX_FAMILY"),
		phpVersion: os.Getenv("CELIKPANEL_CONFIG_MATRIX_PHP_VERSION"),
	}
	if h.repo == "" {
		h.repo = "../.."
	}
	if raw, err := os.ReadFile("/etc/os-release"); err == nil {
		for _, line := range strings.Split(string(raw), "\n") {
			k, v, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			v = strings.Trim(v, `"`)
			switch k {
			case "ID":
				h.distroID = v
			case "VERSION_CODENAME":
				h.codename = v
			}
		}
	}
	if h.family == "" {
		t.Fatal("CELIKPANEL_CONFIG_MATRIX_FAMILY (apt|pacman) is required")
	}
	return h
}

func (h *matrixHost) write(t *testing.T, c *configmatrix.Case) {
	t.Helper()
	if err := c.Write(h.out); err != nil {
		t.Fatalf("write case %s: %v", c.ID, err)
	}
	h.cases++
}

// notApplicable records a program/variant the product does not write on this
// release (the generator refuses it), so the report says so instead of
// silently omitting it.
func (h *matrixHost) notApplicable(t *testing.T, id, program, variant, source, reason string) {
	h.write(t, &configmatrix.Case{
		ID: id, Program: program, Variant: variant, Validator: "not-applicable",
		Source: source, Extra: map[string]string{"REASON": reason},
	})
}

// renderFailure records a generator that refused or failed to render.
func (h *matrixHost) renderFailure(t *testing.T, id, program, variant, source string, err error) {
	h.write(t, &configmatrix.Case{
		ID: id, Program: program, Variant: variant, Validator: "render-failed",
		Source: source, Extra: map[string]string{"REASON": err.Error()},
	})
}

func stockFile(path string) []byte {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return data
}

// fixtureCert is a real self-signed pair from the Agent's own generator, so
// nginx/postfix/dovecot can load the files the configuration names.
func fixtureCert(t *testing.T, host string) (cert, key []byte) {
	t.Helper()
	cert, key, err := generateDefaultMailCertPair(host, time.Now())
	if err != nil {
		t.Fatalf("fixture certificate: %v", err)
	}
	return cert, key
}

func TestRenderDistroConfigMatrix(t *testing.T) {
	h := newMatrixHost(t)
	t.Run("nginx", func(t *testing.T) { renderMatrixNginx(t, h) })
	t.Run("roundcube", func(t *testing.T) { renderMatrixRoundcube(t, h) })
	t.Run("postfix", func(t *testing.T) { renderMatrixPostfix(t, h) })
	t.Run("dovecot", func(t *testing.T) { renderMatrixDovecot(t, h) })
	t.Run("bind", func(t *testing.T) { renderMatrixBIND(t, h) })
	t.Run("powerdns", func(t *testing.T) { renderMatrixPowerDNS(t, h) })
	t.Run("nftables", func(t *testing.T) { renderMatrixNftables(t, h) })
	t.Run("systemd", func(t *testing.T) { renderMatrixSystemd(t, h) })
	t.Run("hooks", func(t *testing.T) { renderMatrixHooks(t, h) })
	t.Run("apt", func(t *testing.T) { renderMatrixAPT(t, h) })
	t.Logf("rendered %d cases into %s", h.cases, h.out)
}

// ---------------------------------------------------------------- nginx ---

const matrixManagedCertVersion = "sha256-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func matrixFPMSocket(h *matrixHost) string {
	if socket, err := phpToolSocket("webmail", ""); err == nil && socket != "" {
		return socket
	}
	if h.family == "pacman" {
		return "/run/php-fpm/php-fpm.sock"
	}
	return "/run/php/php" + h.phpVersion + "-fpm.sock"
}

func matrixSitePHPSocket(h *matrixHost, siteID int) string {
	version := h.phpVersion
	if version == "" {
		version = "8.4"
	}
	if h.family == "pacman" {
		return fmt.Sprintf("/run/php-fpm/php%s-fpm-site%d.sock", version, siteID)
	}
	return fmt.Sprintf("/var/run/php/php%s-fpm-site%d.sock", version, siteID)
}

// nginxMainPatch is the Agent's own nginx.conf include repair applied to this
// release's stock nginx.conf (a no-op where the distro already includes
// conf.d and sites-enabled).
func nginxMainPatch(t *testing.T) ([]byte, bool) {
	t.Helper()
	stock := stockFile("/etc/nginx/nginx.conf")
	if stock == nil {
		return nil, false
	}
	tmp := filepath.Join(t.TempDir(), "nginx.conf")
	if err := os.WriteFile(tmp, stock, 0o644); err != nil {
		t.Fatal(err)
	}
	run := func(context.Context, string, ...string) ([]byte, error) { return nil, nil }
	changed, err := ensureNginxMainConfig(context.Background(), tmp, run)
	if err != nil {
		t.Fatalf("ensureNginxMainConfig on stock nginx.conf: %v", err)
	}
	if !changed {
		return nil, false
	}
	patched, err := os.ReadFile(tmp)
	if err != nil {
		t.Fatal(err)
	}
	return patched, true
}

func newNginxCase(t *testing.T, id, variant, source, target string, mainPatch []byte) *configmatrix.Case {
	c := &configmatrix.Case{
		ID: "nginx." + id, Program: "nginx", Variant: variant, Validator: "nginx",
		Source: source, Target: target,
	}
	if mainPatch != nil {
		c.File("/etc/nginx/nginx.conf", mainPatch, 0o644)
		c.Extra = map[string]string{"NOTE": "stock nginx.conf patched by ensureNginxMainConfig (cmd/agent/nginx_ready.go:108)"}
	}
	return c
}

type matrixVhost struct {
	id, variant string
	mutate      func(*ApplyVhostRequest)
}

func matrixVhostShapes(h *matrixHost) []matrixVhost {
	managed := func(domain string) (string, string) {
		dir := path.Join("/etc/ssl/celikpanel", domain, matrixManagedCertVersion)
		return path.Join(dir, "fullchain.pem"), path.Join(dir, "privkey.pem")
	}
	ssl := func(req *ApplyVhostRequest) {
		req.SSLType = "custom"
		req.SSLCert, req.SSLKey = managed(strings.ToLower(strings.TrimSuffix(req.Domain, ".")))
	}
	return []matrixVhost{
		{"site-php-http", "PHP site, HTTP only", nil},
		{"site-static-http", "static site, HTTP only", func(r *ApplyVhostRequest) { r.ProjectType = "static"; r.PHPSocket = "" }},
		{"site-php-ssl", "PHP site, HTTPS (no redirect)", ssl},
		{"site-php-ssl-force-hsts", "PHP site, HTTPS + force-HTTPS + HSTS", func(r *ApplyVhostRequest) {
			ssl(r)
			r.ForceHTTPS, r.HSTSEnabled, r.HSTSMaxAge = true, true, 31536000
		}},
		{"site-php-ssl-force-nohsts", "PHP site, HTTPS + force-HTTPS, HSTS off", func(r *ApplyVhostRequest) {
			ssl(r)
			r.ForceHTTPS = true
		}},
		{"site-www-redirect-ssl", "PHP site, www redirect + HTTPS + force-HTTPS", func(r *ApplyVhostRequest) {
			ssl(r)
			r.ForceHTTPS = true
			r.RedirectWWW = true
			r.ServerNames = append(r.ServerNames, r.Domain)
		}},
		{"site-www-redirect-http", "PHP site, www redirect, HTTP only", func(r *ApplyVhostRequest) {
			r.RedirectWWW = true
			r.ServerNames = append(r.ServerNames, r.Domain)
		}},
		{"site-aliases-acme-mail", "PHP site, aliases + temp domain + mail.<domain> ACME name, HTTPS", func(r *ApplyVhostRequest) {
			ssl(r)
			r.ServerNames = append(r.ServerNames, "alias-one.example.test", "alias-two.example.test")
			r.ACMEChallengeNames = []string{"mail." + strings.ToLower(strings.TrimSuffix(r.Domain, "."))}
		}},
		{"site-node-ssl", "Node.js app proxy, HTTPS + force-HTTPS", func(r *ApplyVhostRequest) {
			ssl(r)
			r.ForceHTTPS = true
			r.ProjectType, r.AppPort, r.PHPSocket = "node", 3001, ""
		}},
		{"site-proxy-http", "reverse proxy, HTTP only", func(r *ApplyVhostRequest) {
			r.ProjectType, r.ForwardTo, r.PHPSocket = "proxy", "https://localhost:8443/base", ""
		}},
		{"site-forwarding-http", "forwarding (301), HTTP only", func(r *ApplyVhostRequest) {
			r.ProjectType, r.ForwardTo, r.PHPSocket = "forwarding", "https://destination.example/path", ""
		}},
		{"site-forwarding-ssl-force", "forwarding (302), HTTPS + force-HTTPS", func(r *ApplyVhostRequest) {
			ssl(r)
			r.ForceHTTPS = true
			r.ProjectType, r.ForwardTo, r.ForwardCode, r.PHPSocket = "forwarding", "https://destination.example/path", 302, ""
		}},
		{"site-letsencrypt-lineage", "PHP site, certbot lineage certificate, HTTPS + force-HTTPS", func(r *ApplyVhostRequest) {
			domain := strings.ToLower(strings.TrimSuffix(r.Domain, "."))
			r.SSLType = "letsencrypt"
			r.SSLCert = "/etc/letsencrypt/live/" + domain + "/fullchain.pem"
			r.SSLKey = "/etc/letsencrypt/live/" + domain + "/privkey.pem"
			r.ForceHTTPS = true
		}},
	}
}

// addMatrixVhost renders one site through the Agent's trust boundary and the
// services generator, exactly as ApplyVhost would publish it (sealed vhost in
// sites-available + sites-enabled link + the managed ACME challenge snippet).
func addMatrixVhost(t *testing.T, h *matrixHost, c *configmatrix.Case, gen *services.NginxGenerator, siteID, subID, domainID int, domain string, mutate func(*ApplyVhostRequest)) error {
	documentRoot, err := hostingpath.DocumentRoot(subID, domainID)
	if err != nil {
		return err
	}
	req := ApplyVhostRequest{
		SiteID: siteID, SubscriptionID: subID, DomainID: domainID,
		Domain: domain, TempDomain: "preview-" + domain,
		ServerNames: []string{"www." + domain}, DocumentRoot: documentRoot,
		PHPSocket: matrixSitePHPSocket(h, siteID), SSLType: "none", ProjectType: "php",
	}
	if mutate != nil {
		mutate(&req)
	}
	data, err := validatedVhostData(&req)
	if err != nil {
		return fmt.Errorf("validatedVhostData: %w", err)
	}
	config, err := gen.Render(data)
	if err != nil {
		return fmt.Errorf("NginxGenerator.Render: %w", err)
	}
	available := services.SiteVhostPath(data.Domain)
	c.File(available, []byte(services.SealManagedText(config)), 0o644)
	c.Symlink(strings.Replace(available, "/sites-available/", "/sites-enabled/", 1), available)
	c.File(services.ACMEChallengeFilePath(data.Domain),
		[]byte(services.SealManagedText(services.RenderACMEChallengeFile(data.Domain, data.ACMEChallengeRoot))), 0o644)
	if data.SSLCert != "" {
		cert, key := fixtureCert(t, data.Domain)
		c.File(data.SSLCert, cert, 0o644)
		c.File(data.SSLKey, key, 0o600)
	}
	if c.Target == "" {
		c.Target = available
	}
	return nil
}

func renderMatrixNginx(t *testing.T, h *matrixHost) {
	gen, err := services.NewNginxGenerator()
	if err != nil {
		t.Fatal(err)
	}
	mainPatch, _ := nginxMainPatch(t)
	const vhostSource = "internal/services/templates/nginx/vhost.conf.tmpl (rendered by internal/services/nginx_generator.go:110 via cmd/agent/vhost_rpc.go:498)"
	for _, shape := range matrixVhostShapes(h) {
		c := newNginxCase(t, shape.id, shape.variant, vhostSource, "", mainPatch)
		if err := addMatrixVhost(t, h, c, gen, 42, 7, 19, "example.test", shape.mutate); err != nil {
			h.renderFailure(t, "nginx."+shape.id, "nginx", shape.variant, vhostSource, err)
			continue
		}
		h.write(t, c)
	}

	// Every shape at once on one server, as a real host with many sites is:
	// shared zones, duplicate listen options and default servers conflict
	// only when several files are loaded together.
	all := newNginxCase(t, "all-sites-together", "all site shapes loaded together", vhostSource, "", mainPatch)
	for i, shape := range matrixVhostShapes(h) {
		domain := fmt.Sprintf("site%d.example.test", i+1)
		if err := addMatrixVhost(t, h, all, gen, 100+i, 30+i, 60+i, domain, shape.mutate); err != nil {
			t.Logf("all-sites-together: %s: %v", shape.id, err)
		}
	}

	fpm := matrixFPMSocket(h)
	challenge := hostingpath.PanelACMEChallengeRoot()
	panelHost := "panel.example.test"
	panelACMEPath := "/etc/nginx/sites-available/" + panelACMEVhostName(panelHost) + ".conf"
	panelACME := renderPanelACMEChallengeVhost(panelHost, challenge)
	{
		c := newNginxCase(t, "panel-acme", "panel host name ACME HTTP-01 server", "cmd/agent/panel_cert_rpc.go:443 renderPanelACMEChallengeVhost", panelACMEPath, mainPatch)
		c.File(panelACMEPath, []byte(panelACME), 0o644)
		c.Symlink(strings.Replace(panelACMEPath, "/sites-available/", "/sites-enabled/", 1), panelACMEPath)
		h.write(t, c)
	}
	webmail := renderWebmailNginxConfig(transport.WebmailSocketPath(), roundcubeRootDir(), fpm)
	{
		c := newNginxCase(t, "webmail-socket", "Roundcube on the panel's unix socket (/webmail/)", "cmd/agent/webmail_rpc.go:952 renderWebmailNginxConfig", webmailConfPath, mainPatch)
		c.File(webmailConfPath, []byte(webmail), 0o600)
		h.write(t, c)
	}
	webmailHost := "webmail.example.test"
	originPath := webmailAddressConfPath(webmailHost)
	live := webmailAddressLiveDir(webmailAddressLineageName(webmailHost))
	cert, key := fixtureCert(t, webmailHost)
	for _, v := range []struct {
		id, variant, body string
		tls               bool
	}{
		{"webmail-origin-pending", "webmail on its own address, before the certificate (port 80 only)", renderWebmailAddressPending(webmailHost, challenge), false},
		{"webmail-origin-with-http", "webmail on its own address, HTTPS + its own port-80 server", renderWebmailAddressConfig(webmailHost, challenge, roundcubeRootDir(), fpm, true), true},
		{"webmail-origin-https-only", "webmail on its own address, HTTPS only (port 80 served by the mail host ACME server)", renderWebmailAddressConfig(webmailHost, challenge, roundcubeRootDir(), fpm, false), true},
	} {
		c := newNginxCase(t, v.id, v.variant, "cmd/agent/webmail_address.go:202/209 renderWebmailAddress*", originPath, mainPatch)
		c.File(originPath, []byte(v.body), 0o644)
		if v.tls {
			c.File(live+"/fullchain.pem", cert, 0o644)
			c.File(live+"/privkey.pem", key, 0o600)
		}
		h.write(t, c)
	}
	roots := map[string]string{}
	for tool, candidates := range dbToolRoots {
		roots[tool] = candidates[0]
		if h.family == "pacman" && len(candidates) > 1 {
			roots[tool] = candidates[len(candidates)-1]
		}
	}
	sockets := map[string]string{"phpmyadmin": fpm, "phppgadmin": fpm}
	dbtools := renderDBToolsConfig([]string{"phpmyadmin", "phppgadmin"}, roots, sockets)
	{
		c := newNginxCase(t, "dbtools", "phpMyAdmin + phpPgAdmin loopback server (/dbtool/)", "cmd/agent/php_tool_binding.go:1053 renderDBToolsConfig", dbToolsConfPath, mainPatch)
		c.File(dbToolsConfPath, []byte(dbtools), 0o644)
		h.write(t, c)
	}

	// Everything the panel can write on one host at the same time.
	all.ID, all.Variant = "nginx.everything-together", "every site shape + panel ACME + webmail socket + webmail origin + dbtools"
	all.File(panelACMEPath, []byte(panelACME), 0o644)
	all.Symlink(strings.Replace(panelACMEPath, "/sites-available/", "/sites-enabled/", 1), panelACMEPath)
	all.File(webmailConfPath, []byte(webmail), 0o600)
	all.File(originPath, []byte(renderWebmailAddressConfig(webmailHost, challenge, roundcubeRootDir(), fpm, true)), 0o644)
	all.File(live+"/fullchain.pem", cert, 0o644)
	all.File(live+"/privkey.pem", key, 0o600)
	all.File(dbToolsConfPath, []byte(dbtools), 0o644)
	h.write(t, all)
}

// ------------------------------------------------------------ roundcube ---

func renderMatrixRoundcube(t *testing.T, h *matrixHost) {
	key := strings.Repeat("ab", 18)
	base := renderRoundcubeConfig(key)
	target := filepath.Join(webmailBaseDir, "config", "config.inc.php")
	c := &configmatrix.Case{ID: "roundcube.config", Program: "roundcube", Variant: "config.inc.php (CelikPanel form)",
		Validator: "php-lint", Source: "cmd/agent/webmail_rpc.go:445 renderRoundcubeConfig", Target: target}
	c.File(target, []byte(base), 0o640)
	h.write(t, c)

	// The address settings through the Agent's own editor, on a scratch copy.
	old := webmailBaseDir
	webmailBaseDir = t.TempDir()
	t.Cleanup(func() { webmailBaseDir = old })
	scratch := filepath.Join(webmailBaseDir, "config", "config.inc.php")
	if err := os.MkdirAll(filepath.Dir(scratch), 0o755); err != nil {
		t.Fatal(err)
	}
	// The form embeds webmailBaseDir (the SQLite path), so the scratch copy
	// is rendered for the scratch directory and mapped back afterwards.
	if err := os.WriteFile(scratch, []byte(renderRoundcubeConfig(key)), 0o640); err != nil {
		t.Fatal(err)
	}
	src := "cmd/agent/webmail_rpc.go:481 setRoundcubeAddressSettings"
	if state, err := setRoundcubeAddressSettings(true); err != nil || state != "applied" {
		h.renderFailure(t, "roundcube.config-address", "roundcube", "config.inc.php + own-address settings", src, fmt.Errorf("state=%q err=%v", state, err))
		return
	}
	with, err := os.ReadFile(scratch)
	if err != nil {
		t.Fatal(err)
	}
	c2 := &configmatrix.Case{ID: "roundcube.config-address", Program: "roundcube", Variant: "config.inc.php + own-address settings",
		Validator: "php-lint", Source: src, Target: target}
	c2.File(target, []byte(strings.ReplaceAll(string(with), webmailBaseDir, old)), 0o640)
	h.write(t, c2)
}

// -------------------------------------------------------------- postfix ---

// postconfRecorder replaces postconf in PATH with a script that records each
// invocation's exact argv (US-separated, RS-terminated) and succeeds, so the
// Agent's own functions can run unchanged and their commands are replayed by
// the harness against the release's real Postfix.
func postconfRecorder(t *testing.T, dir string) (logPath string) {
	t.Helper()
	logPath = filepath.Join(dir, "postconf.argv")
	script := `#!/bin/sh
for a in "$@"; do printf '%s\037' "$a"; done >> "` + logPath + `"
printf '\036' >> "` + logPath + `"
if [ "$1" = "-h" ] && [ "$2" = "queue_directory" ] && [ -n "$MAIL_SUBMISSION_STATE" ]; then
  printf '%s\n' "$MAIL_SUBMISSION_STATE/spool"
fi
exit 0
`
	if err := os.WriteFile(filepath.Join(dir, "postconf"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return logPath
}

func recordedArgv(t *testing.T, logPath string) [][]string {
	t.Helper()
	raw, err := os.ReadFile(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatal(err)
	}
	var calls [][]string
	for _, record := range strings.Split(string(raw), "\036") {
		if record == "" {
			continue
		}
		args := strings.Split(strings.TrimSuffix(record, "\037"), "\037")
		calls = append(calls, append([]string{"postconf"}, args...))
	}
	return calls
}

func addPostconfCalls(c *configmatrix.Case, calls [][]string) {
	for _, argv := range calls {
		if len(argv) > 1 && (argv[1] == "-e" || argv[1] == "-M" || argv[1] == "-P" || argv[1] == "-X" || argv[1] == "-#") {
			c.Command(argv...)
		}
	}
}

func newPostfixCase(id, variant, source string) *configmatrix.Case {
	return &configmatrix.Case{
		ID: "postfix." + id, Program: "postfix", Variant: variant, Validator: "postfix",
		Source: source, Target: "/etc/postfix/main.cf",
		Extra: map[string]string{"RESTORE_DIRS": "/etc/postfix"},
	}
}

var matrixSNI = []MailSNIEntry{
	{Names: []string{"mail.example.test"}, CertPath: "/etc/ssl/celikpanel/example.test/" + matrixManagedCertVersion + "/fullchain.pem", KeyPath: "/etc/ssl/celikpanel/example.test/" + matrixManagedCertVersion + "/privkey.pem"},
	{Names: []string{"mail.second.test", "smtp.second.test"}, CertPath: "/etc/ssl/celikpanel/second.test/" + matrixManagedCertVersion + "/fullchain.pem", KeyPath: "/etc/ssl/celikpanel/second.test/" + matrixManagedCertVersion + "/privkey.pem"},
}

func addMatrixSNIFixtures(t *testing.T, c *configmatrix.Case) {
	for _, entry := range matrixSNI {
		cert, key := fixtureCert(t, entry.Names[0])
		c.File(entry.CertPath, cert, 0o644)
		c.File(entry.KeyPath, key, 0o600)
	}
	cert, key := fixtureCert(t, "mail.example.test")
	c.File(defaultMailCert, cert, 0o644)
	c.File(defaultMailKey, key, 0o600)
}

// mailAccountFiles runs the Agent's own account and forwarding RPCs against
// scratch copies of the map paths, with the real `doveadm pw` hash when
// Dovecot is installed, and returns the bytes per production path.
func mailAccountFiles(t *testing.T, mapType string) (files map[string][]byte, postmaps [][]string) {
	t.Helper()
	production := map[string]string{}
	configureMailMutationTest(t)
	production[postfixVBoxPath] = "/etc/postfix/vmailbox"
	production[postfixVirtualPath] = "/etc/postfix/virtual"
	production[postfixDomainsPath] = "/etc/postfix/vmailbox_domains"
	production[dovecotUsersPath] = "/etc/dovecot/users"
	if _, err := os.Stat("/usr/bin/doveadm"); err == nil {
		mailHashGenerator = generateDovecotHashContext
	}
	seen := map[string]bool{}
	mailMutationPostmap = func(_ context.Context, p string) error {
		if prod, ok := production[p]; ok && !seen[prod] {
			seen[prod] = true
			postmaps = append(postmaps, []string{"postmap", mapType + ":" + prod})
		}
		return nil
	}
	for _, account := range []MailAccount{
		{Email: "user@example.test", Password: matrixMailPassword, QuotaMB: 250},
		{Email: "second@example.test", Password: matrixMailPassword, QuotaMB: 1024},
	} {
		account := account
		var resp transport.MailMutationResponse
		if err := (&Agent{}).AddMailAccount(&account, &resp); err != nil || !resp.Applied {
			t.Fatalf("AddMailAccount %s: err=%v resp=%+v", account.Email, err, resp)
		}
	}
	var resp transport.MailMutationResponse
	if err := (&Agent{}).UpdateMailForwarding(&transport.UpdateMailForwardingRequest{Forwardings: []transport.MailForwarding{
		{Source: "info@example.test", Destination: "user@example.test"},
		{Source: "sales@example.test", Destination: "someone@elsewhere.example"},
	}}, &resp); err != nil || !resp.Applied {
		t.Fatalf("UpdateMailForwarding: err=%v resp=%+v", err, resp)
	}
	files = map[string][]byte{}
	for scratch, prod := range production {
		if data, err := os.ReadFile(scratch); err == nil {
			files[prod] = data
		}
	}
	sort.Slice(postmaps, func(i, j int) bool { return postmaps[i][1] < postmaps[j][1] })
	return files, postmaps
}

const matrixMailPassword = "Matrix-Password-1"

func renderMatrixPostfix(t *testing.T, h *matrixHost) {
	if _, err := os.Stat("/etc/postfix/main.cf"); err != nil {
		h.notApplicable(t, "postfix.all", "postfix", "all", "-", "postfix is not installed on this release")
		return
	}
	mapType := postfixMapTypeContext(context.Background())
	all := newPostfixCase("everything-together", "virtual maps + milters + TLS/SNI + policy + submission/smtps", "cmd/agent (mail_stack_rpc.go, mail_tls_rpc.go, mail_policy_rpc.go, mail_submission_rpc.go)")

	// 1. Virtual delivery maps: configurePostfixVirtual (mail_stack_rpc.go:601).
	t.Run("virtual", func(t *testing.T) {
		shim := t.TempDir()
		log := postconfRecorder(t, shim)
		t.Setenv("PATH", shim+":"+os.Getenv("PATH"))
		if err := configurePostfixVirtual(context.Background()); err != nil {
			t.Fatalf("configurePostfixVirtual: %v", err)
		}
		calls := recordedArgv(t, log)
		files, postmaps := mailAccountFiles(t, mapType)
		c := newPostfixCase("virtual-maps", "virtual mailbox domains/maps/aliases ("+mapType+")", "cmd/agent/mail_stack_rpc.go:601 configurePostfixVirtual + mail_rpc.go AddMailAccount/UpdateMailForwarding")
		for _, cc := range []*configmatrix.Case{c, all} {
			addPostconfCalls(cc, calls)
			for p, data := range files {
				if strings.HasPrefix(p, "/etc/postfix/") {
					cc.File(p, data, 0o644)
				}
			}
			for _, pm := range postmaps {
				cc.Command(pm...)
			}
		}
		h.write(t, c)
	})

	// 2. Milter chains: composeMilterChain (mail_stack_rpc.go:490) with the
	// settings writeMilterChain applies (mail_stack_rpc.go:453).
	for _, v := range []struct {
		id   string
		dkim bool
		spam string
	}{
		{"milters-none", false, ""},
		{"milters-dkim", true, ""},
		{"milters-rspamd", false, rspamdMilter},
		{"milters-dkim-rspamd", true, rspamdMilter},
		{"milters-dkim-spamass", true, "unix:/spamass/spamass.sock"},
	} {
		incoming, outgoing := composeMilterChain(v.dkim, v.spam)
		c := newPostfixCase(v.id, fmt.Sprintf("milter chain dkim=%v spam=%q", v.dkim, v.spam), "cmd/agent/mail_stack_rpc.go:439 writeMilterChain / :490 composeMilterChain")
		for _, kv := range [][2]string{{"smtpd_milters", incoming}, {"non_smtpd_milters", outgoing}, {"milter_default_action", "accept"}, {"milter_protocol", "6"}} {
			c.Command("postconf", "-e", kv[0]+"="+kv[1])
			if v.id == "milters-dkim-rspamd" {
				all.Command("postconf", "-e", kv[0]+"="+kv[1])
			}
		}
		h.write(t, c)
	}

	// 3. TLS + SNI: configurePostfixTLS (mail_tls_rpc.go:1138) with a
	// recording runner; it writes /etc/postfix/celikpanel_sni itself.
	for _, v := range []struct {
		id  string
		sni []MailSNIEntry
	}{{"tls-default", nil}, {"tls-sni", matrixSNI}} {
		sniType, err := probePostfixTLSMapType(runMailTLSCommand)
		if err != nil {
			h.renderFailure(t, "postfix."+v.id, "postfix", "TLS", "cmd/agent/mail_tls_rpc.go:391 probePostfixTLSMapType", err)
			continue
		}
		var calls [][]string
		run := func(name string, args ...string) ([]byte, error) {
			calls = append(calls, append([]string{name}, args...))
			return nil, nil
		}
		_ = os.Remove(postfixSNIPath)
		if err := configurePostfixTLS("mail.example.test", v.sni, sniType, run); err != nil {
			h.renderFailure(t, "postfix."+v.id, "postfix", "TLS", "cmd/agent/mail_tls_rpc.go:1138 configurePostfixTLS", err)
			continue
		}
		c := newPostfixCase(v.id, "TLS defaults + SNI map ("+sniType+")", "cmd/agent/mail_tls_rpc.go:1138 configurePostfixTLS + internal/mailtlsconfig/config.go:18,29")
		targets := []*configmatrix.Case{c}
		if v.sni != nil {
			targets = append(targets, all)
		}
		for _, cc := range targets {
			addMatrixSNIFixtures(t, cc)
			if data, err := os.ReadFile(postfixSNIPath); err == nil {
				cc.File(postfixSNIPath, data, 0o600)
			}
			for _, argv := range calls {
				cc.Command(argv...)
			}
		}
		_ = os.Remove(postfixSNIPath)
		h.write(t, c)
	}

	// 4. Mail policy: SetMailPolicy (mail_policy_rpc.go:171) against the
	// release's real current values, recording only the write.
	t.Run("policy", func(t *testing.T) {
		installFakePostfix(t, nil)
		var writes [][]string
		mailPolicyPostconf = func(args ...string) ([]byte, error) {
			if len(args) > 0 && args[0] == "-e" {
				writes = append(writes, append([]string{"postconf"}, args...))
				return nil, nil
			}
			return serviceMutationCommand(context.Background(), "postconf", args...).CombinedOutput()
		}
		current := readMailPolicyForTest(t)
		current.MessageSizeMB = 50
		current.DNSBLZones = []string{"zen.spamhaus.org", "bl.spamcop.net"}
		current.OutboundRateLimit = 100
		resp := setMailPolicyForTest(t, current)
		if resp.Error != "" {
			h.renderFailure(t, "postfix.policy", "postfix", "size/DNSBL/rate policy", "cmd/agent/mail_policy_rpc.go:171 SetMailPolicy", fmt.Errorf("%s %s", resp.Code, resp.Error))
			return
		}
		c := newPostfixCase("policy", "message size + DNSBL + outbound rate", "cmd/agent/mail_policy_rpc.go:171 SetMailPolicy / mail_policy_restrictions.go:202")
		for _, cc := range []*configmatrix.Case{c, all} {
			for _, argv := range writes {
				cc.Command(argv...)
			}
		}
		h.write(t, c)
	})

	// 5. Submission/smtps in master.cf: ConfigureMailSubmission
	// (mail_submission_rpc.go:36) with its own test environment.
	t.Run("submission", func(t *testing.T) {
		env := newMailSubmissionTestEnvironment(t, true)
		bin := os.Getenv("PATH")
		log := postconfRecorder(t, bin)
		call := runConfigureMailSubmission(env.binding)
		if call.err != nil || call.response.Error != "" {
			h.renderFailure(t, "postfix.submission", "postfix", "submission + smtps", "cmd/agent/mail_submission_rpc.go:36 ConfigureMailSubmission", fmt.Errorf("err=%v resp=%+v", call.err, call.response))
			return
		}
		loginMap, err := os.ReadFile(env.mapPath)
		if err != nil {
			t.Fatal(err)
		}
		c := newPostfixCase("submission", "submission (587) + smtps (465) in master.cf", "cmd/agent/mail_submission_rpc.go:185-224 ConfigureMailSubmission")
		for _, cc := range []*configmatrix.Case{c, all} {
			cc.File("/etc/postfix/celikpanel_login_map", loginMap, 0o644)
			// The test environment points the map at a scratch path; the
			// commands are replayed with the production path.
			calls := recordedArgv(t, log)
			for _, argv := range calls {
				for i := range argv {
					argv[i] = strings.ReplaceAll(argv[i], env.mapPath, "/etc/postfix/celikpanel_login_map")
				}
			}
			addPostconfCalls(cc, calls)
		}
		h.write(t, c)
		if dovecot, err := os.ReadFile(env.confPath); err == nil {
			matrixDovecotSubmission = dovecot
		}
	})
	h.write(t, all)
}

var matrixDovecotSubmission []byte

// -------------------------------------------------------------- dovecot ---

func renderMatrixDovecot(t *testing.T, h *matrixHost) {
	if _, err := os.Stat("/usr/bin/doveconf"); err != nil {
		h.notApplicable(t, "dovecot.all", "dovecot", "all", "-", "dovecot is not installed on this release")
		return
	}
	if _, err := os.Stat("/etc/dovecot/conf.d"); err != nil {
		h.notApplicable(t, "dovecot.all", "dovecot", "all", "cmd/agent/mail_stack_rpc.go:631 dovecotConfDirMissingReason",
			"the release's Dovecot has no /etc/dovecot/conf.d; CelikPanel refuses to configure it (dovecotConfDirMissingReason)")
		return
	}
	is24, err := dovecotIs24()
	if err != nil {
		h.renderFailure(t, "dovecot.all", "dovecot", "dialect", "cmd/agent/dovecot_dialect.go:33 dovecotIs24", err)
		return
	}
	dialect := "2.3"
	if is24 {
		dialect = "2.4"
	}
	newCase := func(id, variant, source string) *configmatrix.Case {
		return &configmatrix.Case{ID: "dovecot." + id, Program: "dovecot", Variant: variant + " (dialect " + dialect + ")",
			Validator: "dovecot", Source: source, Target: "/etc/dovecot/dovecot.conf",
			Extra: map[string]string{"RESTORE_DIRS": "/etc/dovecot", "MAIL_PASSWORD": matrixMailPassword}}
	}
	accountFiles, _ := func() (map[string][]byte, [][]string) {
		var files map[string][]byte
		var postmaps [][]string
		t.Run("accounts", func(t *testing.T) { files, postmaps = mailAccountFiles(t, "hash") })
		return files, postmaps
	}()

	virtual := buildDovecotVirtualConfAt(is24, mailRootDir)
	auth := stockFile("/etc/dovecot/conf.d/10-auth.conf")
	var authEdited []byte
	if auth != nil {
		tmp := filepath.Join(t.TempDir(), "10-auth.conf")
		if err := os.WriteFile(tmp, auth, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := disableDovecotSystemAuth(tmp); err != nil {
			t.Logf("disableDovecotSystemAuth: %v", err)
		} else {
			authEdited, _ = os.ReadFile(tmp)
		}
	}
	addVirtual := func(c *configmatrix.Case) {
		c.File("/etc/dovecot/conf.d/99-celikpanel.conf", []byte(virtual), 0o644)
		if authEdited != nil {
			c.File("/etc/dovecot/conf.d/10-auth.conf", authEdited, 0o644)
		}
		if users, ok := accountFiles["/etc/dovecot/users"]; ok {
			c.File("/etc/dovecot/users", users, 0o640)
			// The mode and owner the Agent requires (dovecot_users_metadata_linux.go).
			c.Extra["OWNERS"] = "/etc/dovecot/users=root:dovecot"
		}
		c.Extra["MAIL_USERS"] = "user@example.test second@example.test"
		c.Extra["MAIL_ROOT"] = mailRootDir
	}
	tlsConf := buildDovecotTLSConf(is24, defaultMailCert, defaultMailKey, matrixSNI)
	addTLS := func(c *configmatrix.Case) {
		c.File(dovecotTLSConf, []byte(tlsConf), 0o644)
		addMatrixSNIFixtures(t, c)
	}

	c := newCase("virtual", "virtual mailboxes + passwd-file users + 10-auth edit", "cmd/agent/dovecot_dialect.go:47 buildDovecotVirtualConfAt + mail_stack_rpc.go:749 disableDovecotSystemAuth + mail_rpc.go users file")
	addVirtual(c)
	h.write(t, c)

	c = newCase("tls-sni", "TLS default + SNI local_name blocks", "internal/mailtlsconfig/config.go:46 Dovecot (cmd/agent/dovecot_dialect.go:101)")
	addTLS(c)
	h.write(t, c)

	if matrixDovecotSubmission != nil {
		c = newCase("submission", "SASL auth socket for Postfix submission", "cmd/agent/mail_submission_rpc.go:145-153 ConfigureMailSubmission (inline literal)")
		c.File("/etc/dovecot/conf.d/97-celikpanel-submission.conf", matrixDovecotSubmission, 0o644)
		h.write(t, c)
	}

	c = newCase("everything-together", "virtual + TLS/SNI + submission", "cmd/agent (dovecot_dialect.go, mail_submission_rpc.go, mail_stack_rpc.go)")
	addVirtual(c)
	addTLS(c)
	if matrixDovecotSubmission != nil {
		c.File("/etc/dovecot/conf.d/97-celikpanel-submission.conf", matrixDovecotSubmission, 0o644)
	}
	h.write(t, c)
}

// ----------------------------------------------------------------- BIND ---

func matrixZoneRecords(domain string) []transport.ZoneRecord {
	return []transport.ZoneRecord{
		{Name: domain, Type: "SOA", Content: "ns1.example.net hostmaster." + domain + " 2026103001 10800 3600 604800 3600", TTL: 3600},
		{Name: domain, Type: "NS", Content: "ns1.example.net", TTL: 3600},
		{Name: domain, Type: "NS", Content: "ns2.example.net", TTL: 3600},
		{Name: domain, Type: "A", Content: "192.0.2.10", TTL: 300},
		{Name: "ipv6." + domain, Type: "AAAA", Content: "2001:db8::1", TTL: 300},
		{Name: "www." + domain, Type: "CNAME", Content: domain + ".", TTL: 300},
		{Name: domain, Type: "MX", Content: "mail." + domain, TTL: 3600, Prio: 10},
		{Name: "mail." + domain, Type: "A", Content: "192.0.2.11", TTL: 300},
		{Name: domain, Type: "TXT", Content: `"v=spf1 mx -all"`, TTL: 300},
		{Name: "celik._domainkey." + domain, Type: "TXT", Content: `"v=DKIM1; k=rsa; p=MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAu1SU1LfVLPHCozMxH2Mo4lgOEePzNm0tRgeLezV6ffAt0gunVTLw7onLRnrq0/IzW7yWR7QkrmBL7jTKEn5u+qKhbwKfBstIs+bMY2Zkp18gnTxKLxoS2tFczGkPLPgizskuemMg" "hRniWaoLcyehkd3qqGElvW/VDL5AaWTg0nLVkjRo9z+40RQzuVaE8AkAFmxZzow3x+VJYKdxvHf6TpFhKFmLc3FjXNj1v8ffqWqlFiBNSrmHN6MdDSupMIzUq2a8wv6UfyKFt5IJIb4KBl4M1BbBCaxyQ3EV8Iw9s1dXmkt8h7nfrsWSDqd9yTuwOz5p+1AjRwLRGpUOAoBcnwIDAQAB"`, TTL: 300},
		{Name: "_sip._tcp." + domain, Type: "SRV", Content: "5 443 sip." + domain, TTL: 300, Prio: 20},
		{Name: domain, Type: "CAA", Content: `0 issue "letsencrypt.org"`, TTL: 300},
	}
}

func matrixZone(domain string, generation int64) binddns.ZoneSnapshot {
	return binddns.ZoneSnapshot{
		DesiredGeneration: generation, Domain: domain,
		Qualifier:         "dns-zone-sync/v3:sha256:" + strings.Repeat("a", 64),
		MutationRequestID: strings.Repeat("1", 32), MutationOwnerID: strings.Repeat("2", 32),
		Records: matrixZoneRecords(domain),
	}
}

func renderMatrixBIND(t *testing.T, h *matrixHost) {
	profile := hostplatform.Profile{ServiceManager: hostplatform.ServiceManagerSystemd}
	if h.family == "pacman" {
		profile.PackageManager, profile.DistroFamily = hostplatform.PackageManagerPacman, hostplatform.DistroFamilyArch
	} else {
		profile.PackageManager, profile.DistroFamily = hostplatform.PackageManagerAPT, hostplatform.DistroFamilyDebian
	}
	layout, err := bindLayout(profile)
	if err != nil {
		h.renderFailure(t, "bind.all", "bind", "layout", "cmd/agent/dns_engine_host.go:200 bindLayout", err)
		return
	}
	if stockFile(layout.OptionsConfig) == nil {
		h.notApplicable(t, "bind.all", "bind", "all", "-", "BIND is not installed on this release ("+layout.OptionsConfig+" missing)")
		return
	}
	reader := func(p string, mode os.FileMode, _ bool) (dnsFileSnapshot, error) {
		data := stockFile(p)
		if data == nil {
			return dnsFileSnapshot{}, fmt.Errorf("stock %s is missing", p)
		}
		return dnsFileSnapshot{Path: p, Exists: true, Mode: uint32(mode.Perm()), Data: data}, nil
	}
	pair := func(role string) *binddns.Pairing {
		if role == binddns.PairRoleSecondary {
			return &binddns.Pairing{Role: role, LocalIP: "192.0.2.20", LocalNS: "ns2.example.net", PeerIP: "192.0.2.10", PeerNS: "ns1.example.net"}
		}
		return &binddns.Pairing{Role: role, LocalIP: "192.0.2.10", LocalNS: "ns1.example.net", PeerIP: "192.0.2.20", PeerNS: "ns2.example.net"}
	}
	for _, v := range []struct {
		id, variant string
		pairing     *binddns.Pairing
		zones       []binddns.ZoneSnapshot
	}{
		{"standalone", "standalone primary, two zones", nil, []binddns.ZoneSnapshot{matrixZone("example.test", 3), matrixZone("second.test", 4)}},
		{"standalone-empty", "standalone, no zones yet", nil, nil},
		{"paired-primary", "paired primary: zones + catalog zone + allow-transfer/also-notify", pair(binddns.PairRolePrimary), []binddns.ZoneSnapshot{matrixZone("example.test", 3), matrixZone("second.test", 4)}},
		{"paired-primary-empty", "paired primary, empty catalog", pair(binddns.PairRolePrimary), nil},
		{"paired-secondary", "paired secondary: catalog-zones consumer in options + secondary catalog zone", pair(binddns.PairRoleSecondary), nil},
	} {
		source := "internal/binddns/manifest.go:85 RenderManifest + cmd/agent/dns_engine_host.go:2472 prepareBINDConfigMutationWithSnapshotReader (dns_engine_bind_config.go:44 options, internal/bindconfig/zone_include.go:29 include)"
		var serial uint32
		if v.pairing != nil && v.pairing.Role == binddns.PairRolePrimary {
			serial = 1
		}
		generation, err := binddns.RenderManifest(layout.GenerationRoot, binddns.Manifest{EngineEpoch: 1, Pairing: v.pairing, PrimaryCatalogSerial: serial, Zones: v.zones})
		if err != nil {
			h.renderFailure(t, "bind."+v.id, "bind", v.variant, source, err)
			continue
		}
		transferPeer := ""
		var pairings []*binddns.Pairing
		if v.pairing != nil {
			// The options carry only a secondary's catalog subscription
			// (dns_engine_host.go:1731 bindSecondaryOptionsPairing).
			if p := bindSecondaryOptionsPairing(v.pairing.Role, v.pairing.LocalIP, v.pairing.LocalNS, v.pairing.PeerIP, v.pairing.PeerNS); p != nil {
				pairings = []*binddns.Pairing{p}
			}
			if v.pairing.Role == binddns.PairRoleSecondary {
				transferPeer = v.pairing.PeerIP
			}
		}
		mutation, err := prepareBINDConfigMutationWithSnapshotReader(layout, transferPeer, bindOptionsTakeover, reader, pairings...)
		if err != nil {
			h.renderFailure(t, "bind."+v.id, "bind", v.variant, source, err)
			continue
		}
		c := &configmatrix.Case{ID: "bind." + v.id, Program: "bind", Variant: v.variant, Validator: "bind",
			Source: source, Target: layout.MainConfig, Extra: map[string]string{}}
		for p, data := range mutation.desired {
			c.File(p, data, os.FileMode(bindVendorConfigMode(layout)))
			if bindLayoutIsPacman(layout) {
				// The vendor owner the Agent keeps on pacman (0640 root:named).
				c.Extra["OWNERS"] = strings.TrimSpace(c.Extra["OWNERS"] + " " + p + "=root:named")
			}
		}
		genDir := path.Join(layout.GenerationRoot, "generations", generation.ID)
		c.File(path.Join(genDir, "zones.conf"), generation.Config, 0o644)
		c.File(path.Join(genDir, "receipt.json"), generation.Receipt, 0o644)
		var zones []string
		for _, zone := range generation.Zones {
			p := path.Join(genDir, "zones", zone.FileName)
			c.File(p, zone.Data, 0o644)
			zones = append(zones, zone.Domain+"="+p)
		}
		if generation.Catalog != nil && generation.ReceiptValue.Pairing != nil {
			p := path.Join(genDir, generation.ReceiptValue.Pairing.CatalogFile)
			c.File(p, generation.Catalog.Data, 0o644)
			zones = append(zones, generation.ReceiptValue.Pairing.LocalCatalog+"="+p)
		}
		c.Symlink(path.Join(layout.GenerationRoot, "current"), path.Join("generations", generation.ID))
		c.Extra["ZONES"] = strings.Join(zones, " ")
		h.write(t, c)
	}
	// The include helper on its own, for the record (also part of every case).
	if _, err := bindconfig.ManagedZoneInclude("", path.Join(layout.GenerationRoot, "current", "zones.conf")); err != nil {
		t.Logf("ManagedZoneInclude: %v", err)
	}
}

// ------------------------------------------------------------- PowerDNS ---

func renderMatrixPowerDNS(t *testing.T, h *matrixHost) {
	if h.family == "pacman" {
		h.notApplicable(t, "powerdns.all", "powerdns", "all", "cmd/agent/dns_engine_pdns_unit.go (APT only)",
			"the PowerDNS engine switch is certified only on APT hosts; pacman hosts are refused")
		return
	}
	stockMain := stockFile(dnsMainConf)
	if stockMain == nil {
		h.notApplicable(t, "powerdns.all", "powerdns", "all", "-", "PowerDNS is not installed on this release")
		return
	}
	managedDir := filepath.Dir(dnsManagedConf)
	hasInclude, err := validateManagedPowerDNSMainConfig(string(stockMain), managedDir)
	if err != nil {
		h.renderFailure(t, "powerdns.all", "powerdns", "main config", "cmd/agent/dns_sync_rpc.go:320 validateManagedPowerDNSMainConfig", err)
		return
	}
	mainDesired := stockMain
	if !hasInclude {
		// The append newPDNSConfigMutationFromSnapshots makes
		// (cmd/agent/dns_engine_pdns_config_owner.go:258).
		mainDesired = append(append([]byte(nil), stockMain...), []byte("\n# Managed by CelikPanel.\ninclude-dir="+managedDir+"\n")...)
	}
	standalone, err := pdnsmanagedconf.Standalone("/var/lib/powerdns/pdns.sqlite3", []string{"192.0.2.10", "2001:db8::10"})
	if err != nil {
		h.renderFailure(t, "powerdns.all", "powerdns", "standalone", "internal/pdnsmanagedconf/render.go:27 Standalone", err)
		return
	}
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "pdns.sqlite3")
	db, err := initializePDNSEngineDB(context.Background(), dbPath)
	if err != nil {
		h.renderFailure(t, "powerdns.all", "powerdns", "database", "cmd/agent/dns_engine_pdns.go:98 initializePDNSEngineDB", err)
		return
	}
	db.Close()
	dbBytes, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_ = sql.ErrNoRows
	for _, v := range []struct {
		id, variant, role string
		legacy            bool
	}{
		{"standalone", "standalone gsqlite3", "", false},
		{"paired-primary", "directional pair, primary", "primary", false},
		{"paired-secondary", "directional pair, secondary", "secondary", false},
		{"legacy-symmetric", "legacy symmetric pair (PairRole empty)", "", true},
	} {
		c := &configmatrix.Case{ID: "powerdns." + v.id, Program: "powerdns", Variant: v.variant, Validator: "powerdns",
			Source: "internal/pdnsmanagedconf/render.go:27 Standalone / :57 DirectionalCluster; cmd/agent/dns_cluster_rpc.go:634 dnsClusterConfig",
			Target: dnsMainConf, Extra: map[string]string{"RESTORE_DIRS": "/etc/powerdns"}}
		c.File(dnsMainConf, mainDesired, 0o640)
		c.File(dnsManagedConf, standalone, 0o644)
		c.File("/var/lib/powerdns/pdns.sqlite3", dbBytes, 0o640)
		switch {
		case v.role != "":
			local, peer := "192.0.2.10", "192.0.2.20"
			if v.role == "secondary" {
				local, peer = peer, local
			}
			cluster, err := pdnsmanagedconf.DirectionalCluster(v.role, local, peer)
			if err != nil {
				h.renderFailure(t, c.ID, "powerdns", v.variant, c.Source, err)
				continue
			}
			c.File(dnsClusterConf, []byte(cluster), 0o644)
		case v.legacy:
			c.File(dnsClusterConf, []byte(dnsClusterConfig(&DNSClusterRequest{PeerIP: "192.0.2.20"})), 0o644)
		}
		h.write(t, c)
	}
}

// ------------------------------------------------------------- nftables ---

func renderMatrixNftables(t *testing.T, h *matrixHost) {
	for _, v := range []struct {
		id, variant string
		replace     bool
		tcp, udp    []int
	}{
		{"ruleset-minimal", "SSH only, first apply", false, []int{22}, nil},
		{"ruleset-full", "web + mail + DNS + panel, first apply", false, []int{22, 25, 53, 80, 110, 143, 443, 465, 587, 993, 995, 8443}, []int{53, 51820}},
		{"ruleset-empty", "no open ports, first apply", false, nil, nil},
		{"ruleset-replace", "replace an existing celikpanel_fw table", true, []int{22, 80, 443}, []int{53}},
	} {
		c := &configmatrix.Case{ID: "nftables." + v.id, Program: "nftables", Variant: v.variant, Validator: "nftables",
			Source: "internal/firewallpolicy/policy.go:118 Ruleset (applied by cmd/agent/firewall_rpc.go:766 via nft -f -)",
			Target: "/run/celikpanel-matrix/ruleset.nft", Extra: map[string]string{}}
		c.File(c.Target, []byte(firewallpolicy.Ruleset(v.replace, v.tcp, v.udp)), 0o600)
		if v.replace {
			c.Extra["PRECREATE"] = firewallpolicy.Ruleset(false, []int{22}, nil)
		}
		h.write(t, c)
	}
}

// -------------------------------------------------------------- systemd ---

func renderMatrixSystemd(t *testing.T, h *matrixHost) {
	rendered := map[string][]byte{}
	unit := func(id, variant, source, name string, data []byte) {
		target := "/etc/systemd/system/" + name
		c := &configmatrix.Case{ID: "systemd." + id, Program: "systemd", Variant: variant, Validator: "systemd-unit",
			Source: source, Target: target}
		c.File(target, data, 0o644)
		// A timer is verified together with the service it activates.
		if service := strings.TrimSuffix(name, ".timer") + ".service"; strings.HasSuffix(name, ".timer") && rendered[service] != nil {
			c.File("/etc/systemd/system/"+service, rendered[service], 0o644)
		}
		rendered[name] = data
		h.write(t, c)
	}
	for _, name := range []string{"celikpanel-agent.service", "celikpanel-panel.service", "celikpanel-firewall-restore.service", "celikpanel-release-recovery.service", "celikpanel-release-recovery.timer"} {
		data, err := os.ReadFile(filepath.Join(h.repo, "deploy", "systemd", name))
		if err != nil {
			t.Fatalf("read deploy/systemd/%s: %v", name, err)
		}
		unit(strings.TrimSuffix(strings.ReplaceAll(name, ".", "-"), "-service"), "static unit "+name, "deploy/systemd/"+name, name, data)
	}
	if _, data, err := firewallruntime.Build([]byte("matrix")); err != nil {
		h.renderFailure(t, "systemd.firewall-runtime", "systemd", "firewall runtime unit", "internal/firewallruntime/contract.go:49 Build", err)
	} else {
		unit("firewall-runtime", "native firewall restore unit (runtime artifact)", "internal/firewallruntime/firewall.service via contract.go:49 Build", "celikpanel-firewall-restore.service", data)
	}
	if _, files, err := mailrenewalkit.Payload([]byte("matrix")); err != nil {
		h.renderFailure(t, "systemd.mail-renewal", "systemd", "mail renewal units", "internal/mailrenewalkit/contract.go:60 Payload", err)
	} else {
		for _, name := range []string{"celikpanel-mail-renewal.service", "celikpanel-mail-renewal.timer"} {
			if data, ok := files[name]; ok {
				unit("mail-renewal-"+strings.TrimPrefix(path.Ext(name), "."), "mail host certificate renewal "+path.Ext(name), "internal/mailrenewalkit/renewal"+path.Ext(name)+" via contract.go:60 Payload", name, data)
			}
		}
		if hook, ok := files["celikpanel-mail-host-cert"]; ok {
			c := &configmatrix.Case{ID: "hooks.mail-renewal-deploy-hook", Program: "certbot-hook", Variant: "mail host certificate deploy hook (runtime artifact)",
				Validator: "shell", Source: "internal/mailrenewalkit/deploy-hook via contract.go:60 Payload",
				Target: "/etc/letsencrypt/renewal-hooks/deploy/celikpanel-mail-host-cert"}
			c.File(c.Target, hook, 0o755)
			h.write(t, c)
		}
	}
	dir := "/etc/systemd/system"
	for _, v := range []struct {
		id, variant string
		req         AppApplyRequest
	}{
		{"app-unit-node", "Node.js app unit with run-as user", AppApplyRequest{SiteID: 42, Description: "CelikPanel app example.test", WorkDir: "/var/www/celikpanel/subscriptions/7/sites/19/app", Command: "node server.js", Port: 3001, NodeVersion: "22.11.0", RunAsUser: "www-data"}},
		{"app-unit-plain", "app unit without Node version or user", AppApplyRequest{SiteID: 43, Description: "CelikPanel app second.test", WorkDir: "/srv/app", Command: "/usr/bin/env python3 -m http.server 3002", Port: 3002}},
	} {
		f := appUnitFile(&v.req, dir)
		data := services.SealManagedTextWith(f.HeaderPrefix, f.Body)
		c := &configmatrix.Case{ID: "systemd." + v.id, Program: "systemd", Variant: v.variant, Validator: "systemd-unit",
			Source: "internal/services/legacy_site_file_texts.go:165 AppUnitBody via cmd/agent/app_rpc.go:130 appUnitFile", Target: f.Path}
		c.File(f.Path, []byte(data), 0o644)
		h.write(t, c)
	}
}

// ---------------------------------------------------------------- hooks ---

func renderMatrixHooks(t *testing.T, h *matrixHost) {
	for _, v := range []struct{ id, variant, source, target, body string }{
		{"panel-cert-deploy-hook", "panel certificate certbot deploy hook", "cmd/agent/panel_cert_files_linux.go:481 renderPanelCertDeployHook", "/etc/letsencrypt/renewal-hooks/deploy/celikpanel-panel-cert", renderPanelCertDeployHook()},
		{"mail-host-cert-legacy-hook", "mail host certificate deploy hook (legacy writer)", "cmd/agent/mail_host_certificate_linux.go:95 -> internal/mailrenewalkit/contract.go:145 LegacyHook", "/etc/letsencrypt/renewal-hooks/deploy/celikpanel-mail-host-cert", renderMailHostCertificateDeployHook()},
	} {
		c := &configmatrix.Case{ID: "hooks." + v.id, Program: "certbot-hook", Variant: v.variant, Validator: "shell", Source: v.source, Target: v.target}
		c.File(v.target, []byte(v.body), 0o755)
		h.write(t, c)
	}
}

// ------------------------------------------------------------------ apt ---

func renderMatrixAPT(t *testing.T, h *matrixHost) {
	if h.family != "apt" {
		h.notApplicable(t, "apt.all", "apt", "all", "-", "not an APT release")
		return
	}
	for _, id := range []string{"sury", "pgdg", "netdata"} {
		repo := repoFromCatalogue(id)
		if repo == nil {
			continue
		}
		source, err := renderRepoSource(repo, h.distroID, h.codename)
		if err != nil {
			h.notApplicable(t, "apt.source-"+id, "apt", "vendor repository "+id, "cmd/agent/repo_rpc.go:758 renderRepoSource", err.Error())
			continue
		}
		keyring := repoKeyringPath(id, false)
		c := &configmatrix.Case{ID: "apt.source-" + id, Program: "apt", Variant: "vendor repository " + id + " (.list + pin + unattended-upgrades origins)",
			Validator: "apt", Source: "cmd/agent/repo_rpc.go:758 renderRepoSource + :162 signedRepoSource; repo_confine.go:374 renderRepoPin; os_updates.go:175 renderVendorOriginsDropIn",
			Target: repoSourcePath(id)}
		c.File(repoSourcePath(id), []byte(signedRepoSource(source, keyring)+"\n"), 0o644)
		c.File(keyring, nil, 0o644)
		host := ""
		if u := strings.Fields(source); len(u) > 1 {
			host = strings.TrimPrefix(strings.TrimPrefix(u[1], "https://"), "http://")
			host = strings.SplitN(host, "/", 2)[0]
		}
		c.File(repoPinPath(id), renderRepoPin(repo, host, []string{"php8.4-fpm", "php8.4-cli"}), 0o644)
		if repo.ReleaseOrigin != "" {
			c.File(vendorOriginsDropInPath, []byte(renderVendorOriginsDropIn([]string{repo.ReleaseOrigin})), 0o644)
		}
		h.write(t, c)
	}
}
