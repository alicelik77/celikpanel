//go:build linux && realsystem

package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/hostingpath"
	"github.com/alicelik/celikpanel/internal/mutationpayload"
	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Real-system evidence for D-040 (website certificates renewed by the
// operating system), run only inside a disposable Debian 13 chroot in its
// own network namespace, or a disposable QEMU guest booted from a cloud
// image with no route to the internet (deploy/e2e/cert-mail-boot), build
// tag realsystem plus CELIKPANEL_REAL_SYSTEM_SITECERT=1 and the marker
// file below. Never run it on an installed server.
// The certificate authority is a local Pebble test server, never a real CA.
// The harness (deploy/e2e/d040/run-in-chroot.sh) runs one phase per
// invocation; between phases this binary is deleted, so certbot's own
// renewal runs with no CelikPanel program on the system.
//
// D-040 gerçek sistem kanıtı; yalnız atılabilir chroot, yerel Pebble.

const (
	sitecertDomain       = "site.test"
	sitecertSubscription = 1
	sitecertDomainID     = 42
	sitecertSiteID       = 1
)

func sitecertEnv(t *testing.T, name string) string {
	t.Helper()
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		t.Fatalf("%s is required", name)
	}
	return value
}

func sitecertGuard(t *testing.T) {
	t.Helper()
	if os.Getenv("CELIKPANEL_REAL_SYSTEM_SITECERT") != "1" {
		t.Skip("real-system D-040 run only in the disposable chroot")
	}
	if _, err := os.Stat("/etc/celikpanel-sitecert-chroot"); err != nil {
		t.Fatal("refusing to run outside the disposable chroot (marker /etc/celikpanel-sitecert-chroot missing)")
	}
}

func sitecertWriteVhost(t *testing.T, certPath, keyPath string) {
	t.Helper()
	documentRoot, err := hostingpath.DocumentRoot(sitecertSubscription, sitecertDomainID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(documentRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	req := &ApplyVhostRequest{
		SiteID: sitecertSiteID, SubscriptionID: sitecertSubscription, DomainID: sitecertDomainID,
		Domain: sitecertDomain, ServerNames: []string{sitecertDomain, "www." + sitecertDomain},
		// The certificate also secures mail: the Panel publishes mail.<domain>
		// for validation (buildVhostRequest), so renewals validate it too.
		ACMEChallengeNames: []string{"mail." + sitecertDomain},
		DocumentRoot:       documentRoot, ProjectType: "static", SSLType: "none",
	}
	if certPath != "" {
		req.SSLType, req.SSLCert, req.SSLKey = "letsencrypt", certPath, keyPath
	}
	data, err := validatedVhostData(req)
	if err != nil {
		t.Fatalf("the product's vhost validation refused the request: %v", err)
	}
	generator, err := services.NewNginxGenerator()
	if err != nil {
		t.Fatal(err)
	}
	body, err := generator.Render(data)
	if err != nil {
		t.Fatal(err)
	}
	available := services.SiteVhostPath(sitecertDomain)
	enabled := strings.Replace(available, "/sites-available/", "/sites-enabled/", 1)
	for _, dir := range []string{filepath.Dir(available), filepath.Dir(enabled), services.PanelManagedDir(sitecertDomain), services.OwnerIncludeDir(sitecertDomain)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(available, []byte(services.SealManagedText(body)), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(enabled)
	if err := os.Symlink(available, enabled); err != nil {
		t.Fatal(err)
	}
	root, err := prepareACMEChallengeRoot(sitecertSubscription, sitecertDomainID)
	if err != nil {
		t.Fatal(err)
	}
	challenge := services.SealManagedText(services.RenderACMEChallengeFile(sitecertDomain, root))
	if err := os.WriteFile(services.ACMEChallengeFilePath(sitecertDomain), []byte(challenge), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRealSystemSiteCertPrepare(t *testing.T) {
	sitecertGuard(t)
	// What EnsureNginxReady does before any site is written: the drop-in
	// directories and, where the release's nginx.conf includes neither
	// (Arch), the two include lines (nginx_ready.go).
	for _, dir := range []string{"/etc/nginx/conf.d", "/etc/nginx/sites-available", "/etc/nginx/sites-enabled"} {
		if err := ensureNginxDropInDirectory(dir); err != nil {
			t.Fatal(err)
		}
	}
	changed, err := ensureNginxMainConfig(context.Background(), nginxMainConf,
		func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return exec.CommandContext(ctx, name, args...).CombinedOutput()
		})
	if err != nil {
		t.Fatalf("ensureNginxMainConfig: %v", err)
	}
	t.Logf("nginx.conf includes added: %v", changed)
	sitecertWriteVhost(t, "", "")
}

func TestRealSystemSiteCertIssue(t *testing.T) {
	sitecertGuard(t)
	server := sitecertEnv(t, "SITECERT_ACME_SERVER")
	kid, hmac := os.Getenv("SITECERT_EAB_KID"), os.Getenv("SITECERT_EAB_HMAC")
	var resp IssueLetsEncryptResponse
	if err := (&Agent{}).IssueLetsEncryptCertificate(IssueLetsEncryptRequest{
		Domain: sitecertDomain, Aliases: []string{"www." + sitecertDomain, "mail." + sitecertDomain}, Email: "admin@site.test",
		SubscriptionID: sitecertSubscription, DomainID: sitecertDomainID, AutoRenew: true,
		ACMEServer: server, EABKeyID: kid, EABHMACKey: hmac,
	}, &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Success {
		t.Fatalf("issue failed: %s (%s %s)", resp.Error, resp.Failure, resp.FailureDetail)
	}
	t.Logf("lineage %s, expires %s, renewal warning %q", resp.LineageName, resp.ExpiresAt.UTC().Format(time.RFC3339), resp.RenewalWarning)
	if !transport.NativeSiteLineageBelongsTo(resp.LineageName, sitecertDomainID) {
		t.Fatalf("lineage %q", resp.LineageName)
	}
	full, key, chain := transport.NativeSiteLineagePaths(resp.LineageName)
	if resp.CertPath != full || resp.KeyPath != key || resp.ChainPath != chain {
		t.Fatalf("paths %s %s %s", resp.CertPath, resp.KeyPath, resp.ChainPath)
	}
	conf, err := os.ReadFile(nativeSiteRenewalConfPath(resp.LineageName))
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parseNativeSiteRenewalConf(conf)
	if err != nil {
		t.Fatal(err)
	}
	root, _ := hostingpath.ACMEChallengeRoot(sitecertSubscription, sitecertDomainID)
	if parsed.Authenticator != "webroot" || parsed.Server != server || !parsed.AutoRenew || !nativeSiteWebrootMatches(parsed, root) ||
		parsed.WebrootMap[sitecertDomain] != root || parsed.WebrootMap["www."+sitecertDomain] != root ||
		parsed.WebrootMap["mail."+sitecertDomain] != root {
		t.Fatalf("certbot's renewal configuration is not what renewal without CelikPanel needs:\n%s", conf)
	}
	if hmac != "" && strings.Contains(string(conf), hmac) {
		t.Fatal("the EAB HMAC key is in the renewal configuration")
	}
	if state := nativeSiteHookState(); state != "installed" {
		t.Fatalf("deploy hook state %s", state)
	}
	var inspect ValidateCertResponse
	if err := (&Agent{}).InspectInstalledCertificate(InspectCertificateRequest{
		Domain: sitecertDomain, CertPath: full, KeyPath: key, ChainPath: chain,
	}, &inspect); err != nil || !inspect.Valid {
		t.Fatalf("inspect: %v %+v", err, inspect)
	}
	sitecertWriteVhost(t, full, key)
	for name, value := range map[string]string{"lineage": resp.LineageName, "notafter": resp.ExpiresAt.UTC().Format(time.RFC3339)} {
		if err := os.WriteFile("/root/sitecert."+name, []byte(value+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// TestRealSystemSiteCertMail publishes the certificate to Postfix and
// Dovecot for mail.<domain> with the Agent's own mail TLS writers and
// validators (an SNI entry naming certbot's live files, D-040).
func TestRealSystemSiteCertMail(t *testing.T) {
	sitecertGuard(t)
	lineageRaw, err := os.ReadFile("/root/sitecert.lineage")
	if err != nil {
		t.Fatal(err)
	}
	full, key, _ := transport.NativeSiteLineagePaths(strings.TrimSpace(string(lineageRaw)))
	entries := []MailSNIEntry{{Names: []string{"mail." + sitecertDomain, sitecertDomain}, CertPath: full, KeyPath: key}}
	const myhostname = "mx." + sitecertDomain
	if _, err := mutationpayload.CanonicalMailTLSSync("/etc/ssl/celikpanel", myhostname, entries); err != nil {
		t.Fatalf("the mail TLS plan contract refused the entry: %v", err)
	}
	valid, err := validateMailSNIEntries(entries)
	if err != nil {
		t.Fatalf("validateMailSNIEntries: %v", err)
	}
	preflight, err := preflightMailTLSCommands(true, runMailTLSCommand)
	if err != nil {
		t.Fatalf("preflight: %v", err)
	}
	t.Logf("Postfix SNI map type %s, Dovecot 2.4 dialect %v", preflight.sniMapType, preflight.dovecot24)
	if dovecotNeedsArchLayout() {
		// What the mail stack's Dovecot setup does first on Arch
		// (configureDovecotVirtual): CelikPanel's main file that includes
		// conf.d last, and the default pair that main file names.
		if _, err := ensureArchDovecotLayout(); err != nil {
			t.Fatalf("ensureArchDovecotLayout: %v", err)
		}
		if err := ensureDovecotSSLCert(context.Background()); err != nil {
			t.Fatalf("ensureDovecotSSLCert: %v", err)
		}
	}
	if err := ensureDefaultMailCert(myhostname); err != nil {
		t.Fatal(err)
	}
	if err := configurePostfixTLS(myhostname, valid, preflight.sniMapType, preflight.run); err != nil {
		t.Fatalf("configurePostfixTLS: %v", err)
	}
	if err := configureDovecotTLSForDialect(myhostname, valid, preflight.dovecot24, preflight.run); err != nil {
		t.Fatalf("configureDovecotTLSForDialect: %v", err)
	}
	if err := validatePostfixTLSConfig(preflight.run); err != nil {
		t.Fatal(err)
	}
	if err := validateDovecotTLSConfig(preflight.run); err != nil {
		t.Fatal(err)
	}
}

func TestRealSystemSiteCertObserve(t *testing.T) {
	sitecertGuard(t)
	lineageRaw, err := os.ReadFile("/root/sitecert.lineage")
	if err != nil {
		t.Fatal(err)
	}
	lineage := strings.TrimSpace(string(lineageRaw))
	var resp transport.SiteCertificateRenewalStateResponse
	if err := (&Agent{}).SiteCertificateRenewalStateV1(&transport.SiteCertificateRenewalStateRequest{
		Domain: sitecertDomain, DomainID: sitecertDomainID, SubscriptionID: sitecertSubscription,
		LineageName: lineage, ACMEServer: os.Getenv("SITECERT_ACME_SERVER"),
	}, &resp); err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.MarshalIndent(resp, "", "  ")
	label := sitecertEnv(t, "SITECERT_LABEL")
	if err := os.WriteFile("/root/sitecert-state-"+label+".json", encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Logf("%s", encoded)
	if want := sitecertEnv(t, "SITECERT_EXPECT_STATE"); resp.State != want {
		t.Fatalf("state %s, want %s", resp.State, want)
	}
	if after := os.Getenv("SITECERT_EXPECT_NEWER_THAN"); after != "" {
		previous, err := time.Parse(time.RFC3339, after)
		if err != nil {
			t.Fatal(err)
		}
		if !resp.NotAfter.After(previous) {
			t.Fatalf("not renewed: not_after %s, previous %s", resp.NotAfter, previous)
		}
	}
	if want := os.Getenv("SITECERT_EXPECT_DEPLOY"); want != "" && (resp.LastDeploy == nil || resp.LastDeploy.Nginx != want) {
		t.Fatalf("last deploy %+v, want nginx=%s", resp.LastDeploy, want)
	}
	if want := os.Getenv("SITECERT_EXPECT_MAIL"); want != "" && (resp.LastDeploy == nil || resp.LastDeploy.Mail+"/"+resp.LastDeploy.Dovecot != want) {
		t.Fatalf("last deploy %+v, want mail/dovecot=%s", resp.LastDeploy, want)
	}
	if want := os.Getenv("SITECERT_EXPECT_FAILURE"); want != "" && (resp.LastFailed == nil || !strings.Contains(resp.LastFailed.Detail, want)) {
		t.Fatalf("last failure %+v, want %q", resp.LastFailed, want)
	}
}
