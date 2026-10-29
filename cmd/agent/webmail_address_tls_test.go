//go:build linux

package main

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

func withWebmailAddressHTTP2Form(t *testing.T, form string) {
	t.Helper()
	saved := webmailAddressHTTP2Form
	webmailAddressHTTP2Form = func() string { return form }
	t.Cleanup(func() { webmailAddressHTTP2Form = saved })
}

// 2026-10-30: the address file carries the sites' TLS policy (Mozilla
// intermediate) and the installed nginx's HTTP/2 form. A file alpha.110 to
// alpha.112 wrote (the earlier policy, no HTTP/2) is still CelikPanel's own
// form: it is not "changed outside CelikPanel", it can be removed, and a PHP
// version move writes it in this release's form.
func TestWebmailAddressAcceptsTheEarlierTLSPolicy(t *testing.T) {
	h := newWebmailAddressTestHost(t)
	withWebmailAddressHTTP2Form(t, services.NginxHTTP2Listen)
	host := "mail.celikhost.com"
	legacy := renderWebmailAddressConfigWith(host, "/var/lib/celikpanel-acme/panel", "/srv/rc/public_html", "/run/php/php8.4-fpm.sock", true, services.NginxHTTP2Off, webmailAddressLegacyTLS)
	if !strings.Contains(legacy, "server {\n    listen 443 ssl;\n    listen [::]:443 ssl;\n    server_name mail.celikhost.com;\n") ||
		!strings.Contains(legacy, "    ssl_session_cache shared:celikpanel_webmail:10m;\n    ssl_session_timeout 10m;\n    ssl_protocols TLSv1.2 TLSv1.3;\n    ssl_ciphers HIGH:!aNULL:!MD5;\n    ssl_prefer_server_ciphers on;\n\n    # Roundcube") {
		t.Fatalf("the earlier form is not alpha.112's text:\n%s", legacy)
	}
	current := renderWebmailAddressConfig(host, "/var/lib/celikpanel-acme/panel", "/srv/rc/public_html", "/run/php/php8.4-fpm.sock", true)
	if strings.Contains(current, "HIGH:") ||
		!strings.Contains(current, "server {\n    listen 443 ssl http2;\n    listen [::]:443 ssl http2;\n    server_name mail.celikhost.com;\n") ||
		!strings.Contains(current, "    ssl_session_cache shared:celikpanel_webmail:10m;\n    ssl_protocols TLSv1.2 TLSv1.3;\n    ssl_ciphers ECDHE-ECDSA-AES128-GCM-SHA256:") ||
		!strings.Contains(current, "    ssl_session_tickets off;\n\n    # Roundcube") {
		t.Fatalf("the current form lacks the intermediate policy or the 1.24 HTTP/2 form:\n%s", current)
	}
	for _, text := range []string{legacy, current} {
		form, managed := parseWebmailAddressConfig(host, text)
		if !managed || !form.withHTTP || form.fpm != "/run/php/php8.4-fpm.sock" || form.stale() != (text == legacy) {
			t.Fatalf("not read as CelikPanel's form: %+v %v", form, managed)
		}
	}
	if _, managed := parseWebmailAddressConfig(host, strings.Replace(legacy, "HIGH:!aNULL:!MD5", "HIGH:!aNULL", 1)); managed {
		t.Fatal("an owner's TLS change counts as CelikPanel's form")
	}
	if err := os.WriteFile(webmailAddressConfPath(host), []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	if warning := refreshWebmailAddresses(context.Background(), h.ops.run, "/run/php/php8.4-fpm.sock"); warning != "" {
		t.Fatal(warning)
	}
	content, _ := os.ReadFile(webmailAddressConfPath(host))
	if string(content) != current {
		t.Fatalf("the earlier form was not written in this release's form:\n%s", content)
	}
	// Written for nginx 1.24, read after an upgrade to 1.26: still CelikPanel's.
	withWebmailAddressHTTP2Form(t, services.NginxHTTP2Directive)
	form, managed := parseWebmailAddressConfig(host, current)
	if !managed || !form.otherHTTP2 {
		t.Fatalf("the 1.24 form after an upgrade: %+v %v", form, managed)
	}
	next := renderWebmailAddressConfig(host, "/var/lib/celikpanel-acme/panel", "/srv/rc/public_html", "/run/php/php8.4-fpm.sock", true)
	if !strings.Contains(next, "    listen 443 ssl;\n    listen [::]:443 ssl;\n    http2 on;\n    server_name") {
		t.Fatalf("1.26 form:\n%s", next)
	}
}

// The default HTTPS server follows a vhost write only when nginx reloaded
// one; a call that wrote nothing does not read nginx.
func TestSiteFilesReloaded(t *testing.T) {
	if siteFilesReloaded(nil) || siteFilesReloaded([]transport.SiteFileResult{{Outcome: transport.SiteFileOutcomeUnchanged}, {Outcome: transport.SiteFileOutcomeKept}}) {
		t.Fatal("nothing was reloaded")
	}
	if !siteFilesReloaded([]transport.SiteFileResult{{Outcome: transport.SiteFileOutcomeUnchanged}, {Outcome: transport.SiteFileOutcomeWritten, Reloaded: true}}) {
		t.Fatal("a reloaded write was not seen")
	}
}
