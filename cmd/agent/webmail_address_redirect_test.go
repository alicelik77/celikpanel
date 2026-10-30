//go:build linux

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// alpha.113: webmail on mail.<domain> shares port 80 with the mail
// certificate's check server, which answered 404 for everything but the
// check. While webmail serves the name over HTTPS that server redirects to
// https; the check stays reachable over http; removing the address puts the
// 404 back; a check server the owner changed is left as it is.

func sharedPanelACMETestHost(t *testing.T, host string) (*webmailAddressTestHost, string) {
	t.Helper()
	h := newWebmailAddressTestHost(t)
	saved := webmailPanelACMEDir
	t.Cleanup(func() { webmailPanelACMEDir = saved })
	webmailPanelACMEDir = filepath.Join(t.TempDir(), "sites-available")
	if err := os.MkdirAll(webmailPanelACMEDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(webmailPanelACMEDir, panelACMEVhostName(host)+".conf")
	if err := os.WriteFile(path, []byte(renderPanelACMEChallengeVhostForm(host, "/var/lib/celikpanel-acme/panel", false)), 0o644); err != nil {
		t.Fatal(err)
	}
	h.dump = "# configuration file /etc/nginx/sites-enabled/" + panelACMEVhostName(host) + ".conf:\nserver {\n    listen 80;\n    listen [::]:80;\n    server_name " + host + ";\n}\n"
	return h, path
}

func TestWebmailAddressRedirectsTheSharedPortEightyServer(t *testing.T) {
	host := "mail.celikhost.com"
	h, path := sharedPanelACMETestHost(t, host)

	resp := enableWebmailAddress(context.Background(), h.ops, host, "owner@example.test", "")
	if !resp.Enabled || resp.Warning != "" {
		t.Fatalf("enable = %+v", resp)
	}
	content, _ := os.ReadFile(path)
	if string(content) != renderPanelACMEChallengeVhostForm(host, "/var/lib/celikpanel-acme/panel", true) ||
		!strings.Contains(string(content), "location ^~ /.well-known/acme-challenge/") ||
		!strings.Contains(string(content), "return 301 https://$host$request_uri;") || strings.Contains(string(content), "return 404;\n    }\n}") {
		t.Fatalf("shared port-80 server after enable:\n%s", content)
	}
	// The panel's own certificate machinery renders the same form now.
	if renderPanelACMEChallengeVhost(host, "/var/lib/celikpanel-acme/panel") != string(content) {
		t.Fatal("a reissue of the mail certificate would put the 404 back")
	}

	removed := removeWebmailAddress(context.Background(), h.ops, host, false)
	if !removed.Removed || removed.Warning != "" {
		t.Fatalf("remove = %+v", removed)
	}
	content, _ = os.ReadFile(path)
	if string(content) != renderPanelACMEChallengeVhostForm(host, "/var/lib/celikpanel-acme/panel", false) {
		t.Fatalf("shared port-80 server after remove:\n%s", content)
	}
	if renderPanelACMEChallengeVhost(host, "/var/lib/celikpanel-acme/panel") != string(content) {
		t.Fatal("the certificate machinery's form does not follow the removal")
	}
}

func TestWebmailAddressKeepsAnOwnerEditedPortEightyServer(t *testing.T) {
	host := "mail.celikhost.com"
	h, path := sharedPanelACMETestHost(t, host)
	owner := strings.Replace(renderPanelACMEChallengeVhostForm(host, "/var/lib/celikpanel-acme/panel", false), "return 404;", "return 410;", 1)
	if err := os.WriteFile(path, []byte(owner), 0o644); err != nil {
		t.Fatal(err)
	}
	resp := enableWebmailAddress(context.Background(), h.ops, host, "owner@example.test", "")
	if !resp.Enabled || !strings.Contains(resp.Warning, "changed outside CelikPanel") || !strings.Contains(resp.Warning, "does not redirect") {
		t.Fatalf("enable = %+v", resp)
	}
	if content, _ := os.ReadFile(path); string(content) != owner {
		t.Fatalf("the owner's port-80 server changed:\n%s", content)
	}
}
