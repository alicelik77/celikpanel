//go:build linux && realsystem

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
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Real-system evidence for alpha.110 (webmail on its own address), run only
// inside a disposable Debian 13 virtual machine booted from the base image
// (build tag realsystem, plus CELIKPANEL_REAL_SYSTEM_A110=1). Never run it on
// an installed server. No real CA is contacted: certbot's issuance is
// replaced by a temporary test CA that this run adds to the VM's trust
// store; everything else is the production path (nginx, PHP-FPM, certbot's
// timer, Roundcube's tree and configuration). The name resolves through
// /etc/hosts inside the VM.
//
// alpha.110 gerçek sistem kanıtı; yalnız atılabilir Debian 13 sanal makinesi.

const a110Host = "mail.a110.test"

func requireRealSystemA110(t *testing.T) {
	t.Helper()
	if os.Getenv("CELIKPANEL_REAL_SYSTEM_A110") != "1" {
		t.Skip("real-system evidence runs only in the disposable alpha.110 VM")
	}
}

// a110Issue is certbot's issuance replaced by the test CA: it first proves
// that nginx answers certbot's HTTP-01 check for the name on port 80 (a token
// file in the challenge root, fetched over http://<name>/), then writes the
// lineage's live files as certbot would.
func a110Issue(t *testing.T) func(context.Context, string, string, string, string) (string, error) {
	return func(ctx context.Context, host, _, lineage, webroot string) (string, error) {
		dir := filepath.Join(webroot, ".well-known", "acme-challenge")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(filepath.Join(webroot, ".well-known"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		token := filepath.Join(dir, "a110-token")
		if err := os.WriteFile(token, []byte("a110-key-authorization"), 0o644); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(token)
		response, err := http.Get("http://" + host + "/.well-known/acme-challenge/a110-token")
		if err != nil {
			t.Fatalf("certbot's check over port 80: %v", err)
		}
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		t.Logf("HTTP-01 check through nginx: HTTP %d %q", response.StatusCode, body)
		if response.StatusCode != http.StatusOK || string(body) != "a110-key-authorization" {
			t.Fatalf("port 80 does not answer certbot's check: %d %q", response.StatusCode, body)
		}
		a110WriteCertificate(t, host, lineage)
		return "", nil
	}
}

func a110WriteCertificate(t *testing.T, host, lineage string) {
	t.Helper()
	caKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	now := time.Now()
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "CelikPanel alpha.110 disposable test CA"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(30 * 24 * time.Hour), IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	caDER, err := x509.CreateCertificate(rand.Reader, ca, ca, caKey.Public(), caKey)
	if err != nil {
		t.Fatal(err)
	}
	caCert, _ := x509.ParseCertificate(caDER)
	leafKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	leaf := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: host}, DNSNames: []string{host},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(90 * 24 * time.Hour), KeyUsage: x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	leafDER, err := x509.CreateCertificate(rand.Reader, leaf, caCert, leafKey.Public(), caKey)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, _ := x509.MarshalECPrivateKey(leafKey)
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	live := webmailAddressLiveDir(lineage)
	if err := os.MkdirAll(live, 0o700); err != nil {
		t.Fatal(err)
	}
	full := append(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER}), caPEM...)
	if err := os.WriteFile(filepath.Join(live, "fullchain.pem"), full, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(live, "privkey.pem"), pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatal(err)
	}
	// The browser's view: the test CA is trusted inside this VM only.
	if err := os.WriteFile("/usr/local/share/ca-certificates/celikpanel-a110-test.crt", caPEM, 0o644); err != nil {
		t.Fatal(err)
	}
	a105Run(t, "update-ca-certificates")
}

func a110Prepare(t *testing.T, a *Agent) {
	t.Helper()
	hosts, _ := os.ReadFile("/etc/hosts")
	if !strings.Contains(string(hosts), a110Host) {
		if err := os.WriteFile("/etc/hosts", append(hosts, []byte("127.0.0.1 "+a110Host+"\n")...), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Remove("/etc/apt/sources.list.d/php.list"); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	a105Run(t, "apt-get", "update", "-q")
	for _, id := range []string{"php-fpm", "certbot"} {
		if resp := a107Install(t, a, id, ""); resp.Error != "" {
			t.Fatalf("%s: %s", id, resp.Error)
		}
	}
}

func TestRealSystemA110WebmailAddressEnable(t *testing.T) {
	requireRealSystemA110(t)
	a := &Agent{}
	a110Prepare(t, a)
	// The installed Agent's unit runs with UMask=0027.
	previousMask := syscall.Umask(0o027)
	t.Cleanup(func() { syscall.Umask(previousMask) })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	var installed InstallRoundcubeResponse
	a.installRoundcubeWith(ctx, &installed)
	logJSON(t, "install Roundcube", installed)
	if installed.Error != "" {
		t.Fatalf("install: %s", installed.Error)
	}
	var configured ConfigureWebmailResponse
	a.configureWebmailWith(ctx, &configured)
	if configured.Error != "" {
		t.Fatalf("configure: %s", configured.Error)
	}

	ops := hostWebmailAddressOps()
	ops.issue = a110Issue(t)
	resp := enableWebmailAddress(ctx, ops, a110Host, "owner@a110.test", "")
	logJSON(t, "enable", resp)
	if !resp.Enabled || !resp.Proved || !resp.CertificateIssued || resp.RoundcubeSettings != "applied" {
		t.Fatalf("enable = %+v", resp)
	}
	conf := a107Out(t, "cat", webmailAddressConfPath(a110Host))
	if !strings.Contains(conf, "listen 443 ssl;") || !strings.Contains(conf, "frame-ancestors 'self'") {
		t.Fatalf("file:\n%s", conf)
	}
	a107Out(t, "stat", "-c", "%A %U:%G %n", webmailAddressConfPath(a110Host))
	a107Out(t, "systemctl", "is-enabled", "certbot.timer")
	a107Out(t, "systemctl", "is-active", "certbot.timer")
	rc := a107Out(t, "tail", "-n", "4", filepath.Join(webmailBaseDir, "config", "config.inc.php"))
	if !strings.Contains(rc, "$config['use_https'] = true;") {
		t.Fatalf("Roundcube's lines are missing:\n%s", rc)
	}

	// What a browser gets, verified against the VM's trust store.
	client := &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	redirect, err := client.Get("http://" + a110Host + "/?_task=mail")
	if err != nil {
		t.Fatal(err)
	}
	redirect.Body.Close()
	t.Logf("http://%s/?_task=mail: HTTP %d Location %q", a110Host, redirect.StatusCode, redirect.Header.Get("Location"))
	if redirect.StatusCode != http.StatusMovedPermanently || redirect.Header.Get("Location") != "https://"+a110Host+"/?_task=mail" {
		t.Fatal("port 80 does not redirect to HTTPS")
	}
	page, err := client.Get("https://" + a110Host + "/")
	if err != nil {
		t.Fatalf("https: %v", err)
	}
	body, _ := io.ReadAll(page.Body)
	page.Body.Close()
	for _, header := range []string{"Content-Security-Policy", "Strict-Transport-Security", "X-Content-Type-Options", "X-Frame-Options", "Set-Cookie"} {
		t.Logf("%s: %q", header, page.Header.Values(header))
	}
	inline := strings.Contains(string(body), "new rcube_webmail()")
	t.Logf("login page: HTTP %d, rcmloginuser=%v, inline start-up script=%v", page.StatusCode, strings.Contains(string(body), "rcmloginuser"), inline)
	cookies := strings.ToLower(strings.Join(page.Header.Values("Set-Cookie"), "; "))
	if page.StatusCode != http.StatusOK || !inline || !strings.Contains(cookies, "secure") || !strings.Contains(cookies, "samesite=strict") {
		t.Fatal("the login page on its own address is not Roundcube with secure, strict cookies")
	}
	sheet, err := webmailStylesheetURLAt("https://"+a110Host, "/", body)
	if err != nil {
		t.Fatal(err)
	}
	css, err := client.Get(sheet)
	if err != nil {
		t.Fatal(err)
	}
	css.Body.Close()
	t.Logf("stylesheet %s: HTTP %d %q", sheet, css.StatusCode, css.Header.Get("Content-Type"))
	if css.StatusCode != http.StatusOK || !strings.HasPrefix(css.Header.Get("Content-Type"), "text/css") {
		t.Fatal("the stylesheet is not text/css")
	}
	// CelikPanel's PHP check answers this server only: from another address
	// of the VM it is refused.
	if addrs, _ := net.InterfaceAddrs(); len(addrs) > 0 {
		for _, raw := range addrs {
			ip, _, _ := net.ParseCIDR(raw.String())
			if ip == nil || ip.IsLoopback() || ip.To4() == nil {
				continue
			}
			remote := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{
				TLSClientConfig: &tls.Config{ServerName: a110Host},
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					var d net.Dialer
					return d.DialContext(ctx, "tcp", net.JoinHostPort(ip.String(), "443"))
				}}}
			check, err := remote.Get("https://" + a110Host + phpToolCheckPrefix + phpToolWebmail)
			if err != nil {
				t.Fatal(err)
			}
			check.Body.Close()
			t.Logf("PHP check asked from %s: HTTP %d", ip, check.StatusCode)
			if check.StatusCode != http.StatusForbidden {
				t.Fatal("CelikPanel's PHP check answers another address")
			}
			break
		}
	}
	status := webmailAddressStatus(ctx, ops, a110Host, true)
	logJSON(t, "status", status)
	if !status.Present || !status.Managed || !status.Proved {
		t.Fatalf("status = %+v", status)
	}
}

func TestRealSystemA110WebmailAddressRemove(t *testing.T) {
	requireRealSystemA110(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	ops := hostWebmailAddressOps()
	resp := removeWebmailAddress(ctx, ops, a110Host, false)
	logJSON(t, "remove", resp)
	if !resp.Removed || resp.CertificateRemoved {
		t.Fatalf("remove = %+v", resp)
	}
	if _, err := os.Stat(webmailAddressConfPath(a110Host)); !os.IsNotExist(err) {
		t.Fatalf("the file stayed: %v", err)
	}
	if _, _, err := readWebmailAddressCertificate(webmailAddressLineageName(a110Host)); err != nil {
		t.Fatalf("the certificate was not kept: %v", err)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	page, err := client.Get("https://" + a110Host + "/")
	answered := err == nil
	if answered {
		body, _ := io.ReadAll(page.Body)
		page.Body.Close()
		answered = strings.Contains(string(body), "rcmloginuser")
	}
	t.Logf("https://%s/ after removal: err=%v, Roundcube answered=%v", a110Host, err, answered)
	if answered {
		t.Fatal("webmail still answers on the removed name")
	}
	rc := a107Out(t, "tail", "-n", "3", filepath.Join(webmailBaseDir, "config", "config.inc.php"))
	if strings.Contains(rc, "use_https") {
		t.Fatal("Roundcube's lines for the address stayed")
	}
	a107Out(t, "nginx", "-t")
}
