//go:build linux

package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// alpha.110: webmail on its own address, served by the server's nginx.

func writeWebmailAddressTestCertificate(t *testing.T, host string, notAfter time.Time) (tls.Certificate, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: host}, DNSNames: []string{host},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: notAfter, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	live := webmailAddressLiveDir(webmailAddressLineageName(host))
	if err := os.MkdirAll(live, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(live, "fullchain.pem"), certPEM, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(live, "privkey.pem"), keyPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	return pair, der
}

type webmailAddressTestHost struct {
	ops      webmailAddressOps
	commands []string
	issued   int
	proofs   int
	proveErr error
	dump     string
	settings []bool
}

func newWebmailAddressTestHost(t *testing.T) *webmailAddressTestHost {
	t.Helper()
	root := t.TempDir()
	savedConf, savedLE, savedBase, savedCheck := webmailAddressConfDir, webmailAddressLetsEncryptDir, webmailBaseDir, phpToolCheckScriptPath
	t.Cleanup(func() {
		webmailAddressConfDir, webmailAddressLetsEncryptDir, webmailBaseDir, phpToolCheckScriptPath = savedConf, savedLE, savedBase, savedCheck
	})
	webmailAddressConfDir = filepath.Join(root, "conf.d")
	webmailAddressLetsEncryptDir = filepath.Join(root, "letsencrypt")
	webmailBaseDir = filepath.Join(root, "webmail")
	phpToolCheckScriptPath = filepath.Join(root, "check", "check.php")
	if err := os.MkdirAll(webmailAddressConfDir, 0o755); err != nil {
		t.Fatal(err)
	}
	h := &webmailAddressTestHost{}
	now := time.Now()
	h.ops = webmailAddressOps{
		run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			h.commands = append(h.commands, strings.Join(append([]string{name}, args...), " "))
			return nil, nil
		},
		nginxDump:          func(context.Context) (string, error) { return h.dump, nil },
		nginxActive:        func(context.Context) error { return nil },
		certbotPresent:     func() error { return nil },
		challengeRoot:      func() (string, error) { return "/var/lib/celikpanel-acme/panel", nil },
		awaitChallenge:     func(context.Context, string, string) error { return nil },
		roundcubeInstalled: func() (bool, error) { return true, nil },
		fpmSocket:          func() (string, error) { return "/run/php/php8.4-fpm.sock", nil },
		ensureCheck:        func() error { return nil },
		ensureRenewal:      func(context.Context) error { return nil },
		deleteCertificate:  func(context.Context, string) error { return nil },
		roundcubeSettings: func(enabled bool) (string, error) {
			h.settings = append(h.settings, enabled)
			return "applied", nil
		},
		prove: func(context.Context, string, string) error {
			h.proofs++
			return h.proveErr
		},
		now:    func() time.Time { return now },
		settle: 0,
	}
	h.ops.issue = func(_ context.Context, host, email, lineage, webroot string) (string, error) {
		h.issued++
		if lineage != webmailAddressLineageName(host) || webroot != "/var/lib/celikpanel-acme/panel" || email != "owner@example.test" {
			t.Errorf("issue %s %s %s %s", host, email, lineage, webroot)
		}
		// certbot's check runs while the pending file answers port 80.
		content, err := os.ReadFile(webmailAddressConfPath(host))
		if strings.Contains(h.dump, panelACMEVhostPrefix) {
			err, content = nil, []byte(renderWebmailAddressPending(host, webroot))
		}
		if err != nil || string(content) != renderWebmailAddressPending(host, webroot) {
			t.Errorf("port 80 is not answering the check during issuance: %v\n%s", err, content)
		}
		writeWebmailAddressTestCertificate(t, host, now.Add(90*24*time.Hour))
		return "", nil
	}
	return h
}

func TestWebmailAddressEnablesWithACertificateAndANativeServer(t *testing.T) {
	h := newWebmailAddressTestHost(t)
	resp := enableWebmailAddress(context.Background(), h.ops, "Mail.CelikHost.com.", "owner@example.test", "")
	if !resp.Enabled || !resp.Proved || resp.Host != "mail.celikhost.com" || !resp.CertificateIssued || resp.Error != "" || resp.RoundcubeSettings != "applied" {
		t.Fatalf("enable = %+v", resp)
	}
	path := webmailAddressConfPath("mail.celikhost.com")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := renderWebmailAddressConfig("mail.celikhost.com", "/var/lib/celikpanel-acme/panel", roundcubeRootDir(), "/run/php/php8.4-fpm.sock", true)
	if string(content) != want {
		t.Fatalf("file:\n%s", content)
	}
	for _, line := range []string{
		"listen 443 ssl;", "server_name mail.celikhost.com;", "return 301 https://$host$request_uri;",
		"location ^~ /.well-known/acme-challenge/", "frame-ancestors 'self'", "Strict-Transport-Security", "nosniff",
		"fastcgi_pass unix:/run/php/php8.4-fpm.sock;", "allow 127.0.0.1;", "deny all;",
		"/letsencrypt/live/" + webmailAddressLineageName("mail.celikhost.com") + "/fullchain.pem;",
	} {
		if !strings.Contains(string(content), line) {
			t.Fatalf("missing %q:\n%s", line, content)
		}
	}
	if strings.Contains(string(content), "celikpanel_session") || strings.Contains(string(content), "proxy_pass") {
		t.Fatalf("the panel is involved:\n%s", content)
	}
	if form, managed := parseWebmailAddressConfig("mail.celikhost.com", string(content)); !managed || !form.withHTTP || form.fpm != "/run/php/php8.4-fpm.sock" {
		t.Fatalf("not read back as CelikPanel's form: %+v %v", form, managed)
	}
	if got := strings.Join(h.commands, "|"); strings.Count(got, "nginx -t") != 2 || strings.Count(got, "systemctl reload nginx") != 2 {
		t.Fatalf("commands = %s", got)
	}
	// Enabling again reuses the certificate that still has 90 days.
	again := enableWebmailAddress(context.Background(), h.ops, "mail.celikhost.com", "owner@example.test", "")
	if !again.Enabled || again.CertificateIssued || h.issued != 1 {
		t.Fatalf("again = %+v issued=%d", again, h.issued)
	}
}

func TestWebmailAddressRollsBackTheServerAndKeepsTheCertificate(t *testing.T) {
	h := newWebmailAddressTestHost(t)
	h.proveErr = &phpToolProbeError{Kind: "asset_unreadable", Message: "webmail's stylesheet answered HTTP 404"}
	resp := enableWebmailAddress(context.Background(), h.ops, "mail.celikhost.com", "owner@example.test", "")
	if resp.Enabled || !resp.RolledBack || resp.ErrorCode != errCodeWebmailAddressProof || !strings.Contains(resp.Error, "HTTP 404") {
		t.Fatalf("enable = %+v", resp)
	}
	if _, err := os.Stat(webmailAddressConfPath("mail.celikhost.com")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the failed server stayed: %v", err)
	}
	if _, _, err := readWebmailAddressCertificate(webmailAddressLineageName("mail.celikhost.com")); err != nil {
		t.Fatalf("the certificate was not kept: %v", err)
	}
	if len(h.settings) != 2 || h.settings[0] != true || h.settings[1] != false {
		t.Fatalf("Roundcube's lines were not taken back: %v", h.settings)
	}
	// A certbot failure says why and what to check, and changes nothing.
	h2 := newWebmailAddressTestHost(t)
	h2.ops.issue = func(context.Context, string, string, string, string) (string, error) {
		return "Timeout during connect (likely firewall problem)", errors.New("certbot could not obtain a certificate for mail.celikhost.com: exit status 1")
	}
	// Port 80 does not answer the check on this server: Let's Encrypt is
	// never asked, the pending server goes.
	h3 := newWebmailAddressTestHost(t)
	h3.ops.awaitChallenge = func(context.Context, string, string) error { return errors.New("HTTP 404") }
	if resp := enableWebmailAddress(context.Background(), h3.ops, "mail.celikhost.com", "owner@example.test", ""); resp.ErrorCode != errCodeWebmailAddressChallenge || h3.issued != 0 || !resp.RolledBack {
		t.Fatalf("unanswered check = %+v issued=%d", resp, h3.issued)
	}
	resp = enableWebmailAddress(context.Background(), h2.ops, "mail.celikhost.com", "owner@example.test", "")
	if resp.ErrorCode != errCodeWebmailAddressCertificate || !resp.RolledBack || !strings.Contains(resp.Error, "firewall problem") || !strings.Contains(resp.Error, "port 80") {
		t.Fatalf("certbot failure = %+v", resp)
	}
	if _, err := os.Stat(webmailAddressConfPath("mail.celikhost.com")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the pending server stayed: %v", err)
	}
}

func TestWebmailAddressRefusesNamesInUseAndOwnerEditedFiles(t *testing.T) {
	h := newWebmailAddressTestHost(t)
	site := "/etc/nginx/sites-enabled/celikhost.com.conf"
	h.dump = "# configuration file " + site + ":\nserver {\n    listen 443 ssl;\n    server_name celikhost.com mail.celikhost.com;\n}\n"
	resp := enableWebmailAddress(context.Background(), h.ops, "mail.celikhost.com", "owner@example.test", "")
	if resp.ErrorCode != errCodeWebmailAddressConflict || !strings.Contains(resp.Error, site) || h.issued != 0 || len(h.commands) != 0 {
		t.Fatalf("https in use = %+v %v", resp, h.commands)
	}
	h.dump = "# configuration file /etc/nginx/conf.d/owner.conf:\nserver {\n    listen 80;\n    server_name mail.celikhost.com;\n}\n"
	if resp := enableWebmailAddress(context.Background(), h.ops, "mail.celikhost.com", "owner@example.test", ""); resp.ErrorCode != errCodeWebmailAddressConflict || !strings.Contains(resp.Error, "port 80") {
		t.Fatalf("http in use = %+v", resp)
	}
	// The mail certificate's own challenge server shares port 80: webmail's
	// file then serves 443 only.
	h.dump = "# configuration file /etc/nginx/sites-enabled/" + panelACMEVhostPrefix + "abc.conf:\nserver {\n    listen 80;\n    listen [::]:80;\n    server_name mail.celikhost.com;\n}\n"
	resp = enableWebmailAddress(context.Background(), h.ops, "mail.celikhost.com", "owner@example.test", "")
	content, _ := os.ReadFile(webmailAddressConfPath("mail.celikhost.com"))
	if !resp.Enabled || strings.Contains(string(content), "listen 80;") {
		t.Fatalf("shared port 80 = %+v\n%s", resp, content)
	}
	// An owner-edited file is never overwritten or removed.
	edited := strings.Replace(string(content), "client_max_body_size 25m;", "client_max_body_size 50m;", 1)
	if err := os.WriteFile(webmailAddressConfPath("mail.celikhost.com"), []byte(edited), 0o600); err != nil {
		t.Fatal(err)
	}
	if resp := enableWebmailAddress(context.Background(), h.ops, "mail.celikhost.com", "owner@example.test", ""); resp.ErrorCode != errCodeWebmailAddressOwnerEdited {
		t.Fatalf("owner edited = %+v", resp)
	}
	if removed := removeWebmailAddress(context.Background(), h.ops, "mail.celikhost.com", false); removed.Removed || removed.ErrorCode != errCodeWebmailAddressOwnerEdited {
		t.Fatalf("remove owner edited = %+v", removed)
	}
	if after, _ := os.ReadFile(webmailAddressConfPath("mail.celikhost.com")); string(after) != edited {
		t.Fatal("the owner's file changed")
	}
	for _, bad := range []string{"", "localhost", "203.0.113.10", "mail.example.com/x", "mail..example.com", "-mail.example.com"} {
		if resp := enableWebmailAddress(context.Background(), h.ops, bad, "", ""); resp.ErrorCode != errCodeWebmailAddressInvalid {
			t.Fatalf("%q accepted: %+v", bad, resp)
		}
	}
}

func TestWebmailAddressChangeEnablesTheNewNameThenRemovesTheOld(t *testing.T) {
	h := newWebmailAddressTestHost(t)
	if resp := enableWebmailAddress(context.Background(), h.ops, "mail.celikhost.com", "owner@example.test", ""); !resp.Enabled {
		t.Fatalf("first = %+v", resp)
	}
	resp := enableWebmailAddress(context.Background(), h.ops, "webmail.celikhost.com", "owner@example.test", "mail.celikhost.com")
	if !resp.Enabled || resp.Warning != "" {
		t.Fatalf("change = %+v", resp)
	}
	if hosts := webmailAddressHosts(); len(hosts) != 1 || hosts[0] != "webmail.celikhost.com" {
		t.Fatalf("hosts = %v", hosts)
	}
	if _, _, err := readWebmailAddressCertificate(webmailAddressLineageName("mail.celikhost.com")); err != nil {
		t.Fatalf("the old certificate was removed: %v", err)
	}
	// Removing the last address takes Roundcube's lines back, and the
	// certificate only when asked.
	deleted := ""
	h.ops.deleteCertificate = func(_ context.Context, lineage string) error { deleted = lineage; return nil }
	removed := removeWebmailAddress(context.Background(), h.ops, "webmail.celikhost.com", true)
	if !removed.Removed || !removed.CertificateRemoved || deleted != webmailAddressLineageName("webmail.celikhost.com") || h.settings[len(h.settings)-1] {
		t.Fatalf("remove = %+v deleted=%q settings=%v", removed, deleted, h.settings)
	}
	if again := removeWebmailAddress(context.Background(), h.ops, "webmail.celikhost.com", false); !again.Removed {
		t.Fatalf("removing twice = %+v", again)
	}
}

func TestWebmailAddressFollowsWebmailsPHPVersion(t *testing.T) {
	h := newWebmailAddressTestHost(t)
	if resp := enableWebmailAddress(context.Background(), h.ops, "mail.celikhost.com", "owner@example.test", ""); !resp.Enabled {
		t.Fatalf("enable = %+v", resp)
	}
	if warning := refreshWebmailAddresses(context.Background(), h.ops.run, "/run/php/php8.5-fpm.sock"); warning != "" {
		t.Fatal(warning)
	}
	content, _ := os.ReadFile(webmailAddressConfPath("mail.celikhost.com"))
	if strings.Contains(string(content), "php8.4") || strings.Count(string(content), "fastcgi_pass unix:/run/php/php8.5-fpm.sock;") != 2 {
		t.Fatalf("socket not followed:\n%s", content)
	}
	if err := removeAllWebmailAddresses(context.Background(), h.ops.run); err != nil || len(webmailAddressHosts()) != 0 {
		t.Fatalf("remove all: %v %v", err, webmailAddressHosts())
	}
}

func TestRoundcubeAddressSettingsOnlyInCelikPanelsForm(t *testing.T) {
	saved := webmailBaseDir
	t.Cleanup(func() { webmailBaseDir = saved })
	webmailBaseDir = t.TempDir()
	path := filepath.Join(webmailBaseDir, "config", "config.inc.php")
	base := renderRoundcubeConfig(strings.Repeat("ab", 18))
	mustWriteTestFile(t, path, []byte(base), 0o640)
	for i := 0; i < 2; i++ {
		if state, err := setRoundcubeAddressSettings(true); err != nil || state != "applied" {
			t.Fatalf("enable %d: %q %v", i, state, err)
		}
	}
	content, _ := os.ReadFile(path)
	if string(content) != base+roundcubeAddressSettings || !strings.Contains(string(content), "$config['use_https'] = true;") || !strings.Contains(string(content), "'Strict'") {
		t.Fatalf("settings:\n%s", content)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o640 {
		t.Fatalf("mode = %v", info.Mode())
	}
	if state, err := setRoundcubeAddressSettings(false); err != nil || state != "unchanged" {
		t.Fatalf("disable: %q %v", state, err)
	}
	if content, _ := os.ReadFile(path); string(content) != base {
		t.Fatalf("not taken back:\n%s", content)
	}
	edited := base + "$config['skin'] = 'larry';\n"
	mustWriteTestFile(t, path, []byte(edited), 0o640)
	if state, err := setRoundcubeAddressSettings(true); err != nil || state != "owner_edited" {
		t.Fatalf("edited: %q %v", state, err)
	}
	if content, _ := os.ReadFile(path); string(content) != edited {
		t.Fatal("the owner's configuration changed")
	}
}

func TestWebmailAddressNameUsesReadEveryFile(t *testing.T) {
	dump := strings.Join([]string{
		"# configuration file /etc/nginx/nginx.conf:", "http { include /etc/nginx/conf.d/*.conf; }",
		"# configuration file /etc/nginx/conf.d/celikpanel-webmail-origin-mail.example.test.conf:", "server { listen 443 ssl; server_name mail.example.test; }",
		"# configuration file /etc/nginx/conf.d/other.conf:", "server { listen 127.0.0.1:8080; server_name mail.example.test; }",
		"server { listen [::]:443 ssl; server_name other.example.test; }",
	}, "\n")
	use := webmailAddressNameUses(dump, "mail.example.test", "/etc/nginx/conf.d/celikpanel-webmail-origin-mail.example.test.conf")
	if use.httpsFile != "" || use.httpFile != "" {
		t.Fatalf("own file or another port counted: %+v", use)
	}
	use = webmailAddressNameUses(dump, "other.example.test", "")
	if use.httpsFile != "/etc/nginx/conf.d/other.conf" {
		t.Fatalf("IPv6 https not seen: %+v", use)
	}
	// A block bound to an explicit public address takes that address's
	// visitors before any name is looked at: refused, whatever its name.
	bound := dump + "\n# configuration file /etc/nginx/sites-enabled/kept.conf:\nserver { listen 203.0.113.10:443 ssl; server_name kept.example.test; }\n"
	if use := webmailAddressNameUses(bound, "mail.example.test", ""); use.boundFile != "/etc/nginx/sites-enabled/kept.conf" || use.boundListen != "203.0.113.10:443" {
		t.Fatalf("bound listen not seen: %+v", use)
	}
	loopback := dump + "\n# configuration file /etc/nginx/conf.d/local.conf:\nserver { listen 127.0.0.1:80; listen [::]:443 ssl; server_name local.example.test; }\n"
	if use := webmailAddressNameUses(loopback, "mail.example.test", ""); use.boundFile != "" {
		t.Fatalf("loopback or wildcard counted as bound: %+v", use)
	}
	if !strings.Contains(renderWebmailAddressConfig("mail.example.test", "/c", "/r", "/s", true), "shared:celikpanel_webmail:10m") {
		t.Fatal("webmail's TLS session cache must not share a zone name another server may size differently")
	}
}

func TestProveWebmailAddressOverHTTPSOnTheName(t *testing.T) {
	saved := webmailAddressLetsEncryptDir
	t.Cleanup(func() { webmailAddressLetsEncryptDir = saved })
	webmailAddressLetsEncryptDir = t.TempDir()
	pair, leaf := writeWebmailAddressTestCertificate(t, "mail.example.test", time.Now().Add(24*time.Hour))
	policy := true
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "mail.example.test" || r.TLS == nil || r.TLS.ServerName != "mail.example.test" {
			http.Error(w, "wrong name", http.StatusMisdirectedRequest)
			return
		}
		switch r.URL.Path {
		case "/__celikpanel_php_check/webmail":
			_, _ = w.Write([]byte("celikpanel-php-check:ok:8.4.11"))
		case "/":
			if policy {
				w.Header().Set("Content-Security-Policy", "frame-ancestors 'self'; object-src 'none'")
				w.Header().Set("Strict-Transport-Security", "max-age=31536000")
				w.Header().Set("X-Content-Type-Options", "nosniff")
			}
			_, _ = w.Write([]byte(`<html><link rel="stylesheet" href="skins/elastic/styles/styles.min.css?s=1"><input name="_user" id="rcmloginuser"><script>var rcmail = new rcube_webmail();</script></html>`))
		case "/skins/elastic/styles/styles.min.css":
			w.Header().Set("Content-Type", "text/css")
			_, _ = w.Write([]byte("body{}"))
		default:
			http.NotFound(w, r)
		}
	}))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{pair}}
	server.StartTLS()
	defer server.Close()
	address := strings.TrimPrefix(server.URL, "https://")
	if err := proveWebmailAddressAt(context.Background(), address, "mail.example.test", "8.4", leaf); err != nil {
		t.Fatalf("proof: %v", err)
	}
	if err := proveWebmailAddressAt(context.Background(), address, "mail.example.test", "8.5", leaf); err == nil || !strings.Contains(err.Error(), "not on PHP 8.5") {
		t.Fatalf("wrong PHP accepted: %v", err)
	}
	policy = false
	if err := proveWebmailAddressAt(context.Background(), address, "mail.example.test", "8.4", leaf); err == nil || !strings.Contains(err.Error(), "frame-ancestors") {
		t.Fatalf("a page without its policy passed: %v", err)
	}
	policy = true
	_, other := writeWebmailAddressTestCertificate(t, "other.example.test", time.Now().Add(24*time.Hour))
	if err := proveWebmailAddressAt(context.Background(), address, "mail.example.test", "8.4", other); err == nil || !strings.Contains(err.Error(), "another certificate") {
		t.Fatalf("another certificate passed: %v", err)
	}
}
