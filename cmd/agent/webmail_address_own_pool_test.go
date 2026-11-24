package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// S4 R1 with alpha.144: webmail's move to its own account hands every
// address file in CelikPanel's form to the account's own PHP-FPM socket
// (refreshWebmailAddresses); alpha.144's start-time rewrite keeps the socket
// a file names. Whichever runs first, the address ends in alpha.144's form
// (only Roundcube's entry point runs PHP, plugin copies refused) with PHP
// handed to webmail's own pool, and it is not listed as an earlier form.
func TestWebmailAddressOnItsOwnPoolKeepsAlpha144Form(t *testing.T) {
	root := t.TempDir()
	savedDir, savedHTTP2 := webmailAddressConfDir, webmailAddressHTTP2Form
	webmailAddressConfDir = filepath.Join(root, "conf.d")
	webmailAddressHTTP2Form = func() string { return services.NginxHTTP2Off }
	t.Cleanup(func() { webmailAddressConfDir, webmailAddressHTTP2Form = savedDir, savedHTTP2 })
	if err := os.MkdirAll(webmailAddressConfDir, 0o755); err != nil {
		t.Fatal(err)
	}
	const tree = "/opt/celikpanel/webmail/public_html"
	stock := "/run/php/php8.4-fpm.sock"
	own := phpToolPoolSocket(phpToolWebmail, "8.4")
	if _, isOwn := phpToolOwnSocket(own); !isOwn {
		t.Fatalf("%s is not read as webmail's own socket", own)
	}
	tls := webmailAddressTLSLines()
	run := func(context.Context, string, ...string) ([]byte, error) { return nil, nil }
	check := func(host, want string) {
		t.Helper()
		written, err := os.ReadFile(webmailAddressConfPath(host))
		if err != nil {
			t.Fatal(err)
		}
		form, managed := parseWebmailAddressConfig(host, string(written))
		if !managed || form.stale() || form.openLayout || form.fpm != want {
			t.Fatalf("%s: managed=%v %+v\n%s", host, managed, form, written)
		}
		text := string(written)
		i := strings.Index(text, "listen 443")
		if i < 0 {
			t.Fatalf("%s: no HTTPS server", host)
		}
		locations, _ := ngxLocations(text[i:])
		if got := ngxAnswer(ngxSelect(locations, "/plugins/password/config.inc.php.bak")); got != "refused" {
			t.Errorf("%s: a plugin's copied configuration is %s", host, got)
		}
		entry := ngxSelect(locations, "/index.php")
		if ngxAnswer(entry) != "php" || !strings.Contains(entry.body, "fastcgi_pass unix:"+want+";") {
			t.Errorf("%s: the entry point is not handed to %s: %+v", host, want, entry)
		}
		if strings.Contains(text, "unix:"+stock+";") && want != stock {
			t.Errorf("%s: the stock pool is still named", host)
		}
	}

	// 1. The move first: an alpha.143 form on the stock pool follows the own
	//    socket and gets the current form in the same write.
	earlier := renderWebmailAddressConfigForm("move.example.test", "", tree, stock, false, services.NginxHTTP2Off, tls, false)
	if err := os.WriteFile(webmailAddressConfPath("move.example.test"), []byte(earlier), 0o600); err != nil {
		t.Fatal(err)
	}
	if warning := refreshWebmailAddresses(context.Background(), run, own); warning != "" {
		t.Fatal(warning)
	}
	check("move.example.test", own)

	// 2. The start-time rewrite of an earlier form already on the own
	//    socket keeps the own socket.
	ownEarlier := renderWebmailAddressConfigForm("start.example.test", "", tree, own, false, services.NginxHTTP2Off, tls, false)
	if err := os.WriteFile(webmailAddressConfPath("start.example.test"), []byte(ownEarlier), 0o600); err != nil {
		t.Fatal(err)
	}
	reconcileWebmailAddressesAtStartup(context.Background(), run)
	check("start.example.test", own)

	// 3. Neither is named in Problems now as an earlier form, and a second
	//    refresh to the same socket changes nothing.
	var forms transport.WebmailAddressFormsResponse
	if err := (&Agent{}).WebmailAddressFormsV1(&transport.WebmailAddressFormsRequest{}, &forms); err != nil || len(forms.Open) != 0 {
		t.Fatalf("listed as an earlier form: %v %+v", err, forms.Open)
	}
	writes := 0
	counting := func(_ context.Context, name string, _ ...string) ([]byte, error) {
		if name == "nginx" {
			writes++
		}
		return nil, nil
	}
	if warning := refreshWebmailAddresses(context.Background(), counting, own); warning != "" || writes != 0 {
		t.Fatalf("second refresh: %q, %d checks", warning, writes)
	}
}
