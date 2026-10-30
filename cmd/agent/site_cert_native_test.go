package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

const testNativeLineage = "celikpanel-site-42-00112233445566778899aabb"

// nativeSiteTestStore points every native path at a temporary directory.
func nativeSiteTestStore(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	saved := []any{nativeSiteConfigDir, nativeSiteHookPath, nativeSiteStatusDir, nativeSiteLogDir, nativeSiteCronEntry}
	nativeSiteConfigDir = filepath.Join(root, "letsencrypt")
	nativeSiteHookPath = filepath.Join(root, "letsencrypt", "renewal-hooks", "deploy", "celikpanel-site-certificates")
	nativeSiteStatusDir = filepath.Join(root, "status")
	nativeSiteLogDir = filepath.Join(root, "log")
	nativeSiteCronEntry = filepath.Join(root, "cron.d", "certbot")
	savedPublish, savedRun, savedEnable, savedQuery, savedNow := nativeSiteHookPublish, nativeSiteCertbotRun, nativeSiteEnableTimer, nativeSiteRunQuery, nativeSiteNow
	nativeSiteHookPublish = func() error {
		if err := os.MkdirAll(filepath.Dir(nativeSiteHookPath), 0o755); err != nil {
			return err
		}
		return os.WriteFile(nativeSiteHookPath, []byte(nativeSiteDeployHook), 0o755)
	}
	t.Cleanup(func() {
		nativeSiteConfigDir = saved[0].(string)
		nativeSiteHookPath = saved[1].(string)
		nativeSiteStatusDir = saved[2].(string)
		nativeSiteLogDir = saved[3].(string)
		nativeSiteCronEntry = saved[4].(string)
		nativeSiteHookPublish, nativeSiteCertbotRun, nativeSiteEnableTimer, nativeSiteRunQuery, nativeSiteNow = savedPublish, savedRun, savedEnable, savedQuery, savedNow
	})
	return root
}

// writeNativeLineage lays out a lineage as certbot does: archive/<l>/*N.pem
// and live/<l>/*.pem symbolic links into it, plus a renewal configuration.
func writeNativeLineage(t *testing.T, lineage string, generation int, leaf, key, chain, conf string) {
	t.Helper()
	archive := filepath.Join(nativeSiteConfigDir, "archive", lineage)
	live := filepath.Join(nativeSiteConfigDir, "live", lineage)
	for _, dir := range []string{archive, live, filepath.Join(nativeSiteConfigDir, "renewal")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{"cert": leaf, "privkey": key, "chain": chain, "fullchain": leaf + chain}
	for name, content := range files {
		archived := filepath.Join(archive, name+strconv.Itoa(generation)+".pem")
		if err := os.WriteFile(archived, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(live, name+".pem")
		_ = os.Remove(link)
		if err := os.Symlink(filepath.Join("..", "..", "archive", lineage, name+strconv.Itoa(generation)+".pem"), link); err != nil {
			t.Fatal(err)
		}
	}
	if conf != "" {
		if err := os.WriteFile(nativeSiteRenewalConfPath(lineage), []byte(conf), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// A renewal configuration as certbot 4.0 (Debian 13) writes it.
const testRenewalConf = `# renew_before_expiry = 30 days
version = 4.0.0
archive_dir = /etc/letsencrypt/archive/celikpanel-site-42-00112233445566778899aabb
cert = /etc/letsencrypt/live/celikpanel-site-42-00112233445566778899aabb/cert.pem
privkey = /etc/letsencrypt/live/celikpanel-site-42-00112233445566778899aabb/privkey.pem
chain = /etc/letsencrypt/live/celikpanel-site-42-00112233445566778899aabb/chain.pem
fullchain = /etc/letsencrypt/live/celikpanel-site-42-00112233445566778899aabb/fullchain.pem

# Options used in the renewal process
[renewalparams]
account = 0123456789abcdef0123456789abcdef
authenticator = webroot
webroot_path = /var/lib/celikpanel-agent/acme-http-01/subscriptions/7/domains/42,
server = https://acme-v02.api.letsencrypt.org/directory
key_type = ecdsa
pref_challs = http-01,
[[webroot_map]]
example.test = /var/lib/celikpanel-agent/acme-http-01/subscriptions/7/domains/42
www.example.test = /var/lib/celikpanel-agent/acme-http-01/subscriptions/7/domains/42
`

func TestNativeSiteLineageNamesCarryTheDomainIdentity(t *testing.T) {
	saved := nativeSiteLineageRead
	t.Cleanup(func() { nativeSiteLineageRead = saved })
	nativeSiteLineageRead = func(b []byte) (int, error) {
		for i := range b {
			b[i] = byte(i)
		}
		return len(b), nil
	}
	got, err := newNativeSiteLineage(42)
	if err != nil {
		t.Fatal(err)
	}
	if got != "celikpanel-site-42-000102030405060708090a0b" {
		t.Fatalf("lineage = %q", got)
	}
	if !transport.NativeSiteLineageBelongsTo(got, 42) || transport.NativeSiteLineageBelongsTo(got, 4) {
		t.Fatal("lineage must belong to exactly its domain identity")
	}
	if _, err := newNativeSiteLineage(0); err == nil {
		t.Fatal("a lineage without a domain identity was made")
	}
	nativeSiteLineageRead = func([]byte) (int, error) { return 0, errors.New("no entropy") }
	if _, err := newNativeSiteLineage(42); err == nil {
		t.Fatal("a lineage was made without randomness")
	}
	for _, bad := range []string{"example.test", "cp-site-42-00112233445566778899aabb", "celikpanel-site-0-00112233445566778899aabb",
		"celikpanel-site-42-00112233445566778899AABB", "celikpanel-site-42-../../x", "celikpanel-webmail-0011"} {
		if transport.IsNativeSiteLineage(bad) {
			t.Errorf("%q accepted as a native lineage", bad)
		}
	}
	full, key, chain := transport.NativeSiteLineagePaths(got)
	for _, p := range []string{full, key, chain} {
		if lineage, ok := transport.NativeSiteLineageFromPath(p); !ok || lineage != got {
			t.Errorf("path %q does not name lineage %q", p, got)
		}
	}
	if _, ok := transport.NativeSiteLineageFromPath("/etc/letsencrypt/live/" + got + "/../x/fullchain.pem"); ok {
		t.Fatal("an unclean path was accepted")
	}
}

func TestNativeSiteIssueArgsKeepEABOutOfTheCommandLine(t *testing.T) {
	req := IssueLetsEncryptRequest{
		Domain: "example.test", Aliases: []string{"www.example.test", "EXAMPLE.test", "mail.example.test"},
		ACMEServer: "https://acme.zerossl.com/v2/DV90", EABKeyID: "kid-123", EABHMACKey: "c2VjcmV0LWhtYWMta2V5",
	}
	args := buildNativeSiteIssueArgs(req, "/var/lib/celikpanel-agent/acme-http-01/subscriptions/7/domains/42", testNativeLineage, "/tmp/x/eab.ini")
	joined := strings.Join(args, " ")
	for _, secret := range []string{req.EABKeyID, req.EABHMACKey, "--eab"} {
		if strings.Contains(joined, secret) {
			t.Fatalf("certbot arguments carry EAB material %q: %s", secret, joined)
		}
	}
	for _, flag := range []string{"--config-dir", "--work-dir", "--logs-dir", "--force-renewal"} {
		if hasArg(args, flag) {
			t.Fatalf("native issuance must use certbot's standard store and a fresh lineage, got %s", flag)
		}
	}
	for _, pair := range [][2]string{{"--config", "/tmp/x/eab.ini"}, {"--cert-name", testNativeLineage},
		{"-w", "/var/lib/celikpanel-agent/acme-http-01/subscriptions/7/domains/42"}, {"--server", req.ACMEServer}, {"--preferred-challenges", "http"}} {
		if !hasArgPair(args, pair[0], pair[1]) {
			t.Fatalf("missing %s %s in %v", pair[0], pair[1], args)
		}
	}
	if !hasArg(args, "--webroot") || !hasArg(args, "--register-unsafely-without-email") {
		t.Fatalf("args = %v", args)
	}
	if got, want := domainArgs(args), []string{"example.test", "www.example.test", "mail.example.test"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("domains = %v, want %v", got, want)
	}
	req.Email = "admin@example.test"
	args = buildNativeSiteIssueArgs(req, "/w", testNativeLineage, "")
	if hasArg(args, "--config") || hasArg(args, "--register-unsafely-without-email") || !hasArgPair(args, "--email", req.Email) {
		t.Fatalf("args = %v", args)
	}
}

func TestNativeSiteEABConfigIsPrivateAndRemoved(t *testing.T) {
	file, cleanup, err := writeNativeSiteEABConfig(IssueLetsEncryptRequest{})
	if err != nil || file != "" {
		t.Fatalf("no credentials must write no file: %q %v", file, err)
	}
	cleanup()
	if _, _, err := writeNativeSiteEABConfig(IssueLetsEncryptRequest{EABKeyID: "kid\nserver = evil", EABHMACKey: "x"}); err == nil {
		t.Fatal("a credential with a line break reached certbot's configuration")
	}
	file, cleanup, err = writeNativeSiteEABConfig(IssueLetsEncryptRequest{EABKeyID: "kid-1", EABHMACKey: "aG1hYw_-"})
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "eab-kid = kid-1\neab-hmac-key = aG1hYw_-\n" {
		t.Fatalf("content = %q", content)
	}
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(file)
		dir, _ := os.Stat(filepath.Dir(file))
		if info.Mode().Perm() != 0o600 || dir.Mode().Perm() != 0o700 {
			t.Fatalf("modes file %v dir %v", info.Mode().Perm(), dir.Mode().Perm())
		}
	}
	cleanup()
	if _, err := os.Stat(filepath.Dir(file)); !os.IsNotExist(err) {
		t.Fatalf("EAB configuration was left behind: %v", err)
	}
}

func TestParseNativeSiteRenewalConfReadsCertbotsOwnFile(t *testing.T) {
	conf, err := parseNativeSiteRenewalConf([]byte(testRenewalConf))
	if err != nil {
		t.Fatal(err)
	}
	if conf.Authenticator != "webroot" || conf.Server != letsEncryptDefaultDirectory || !conf.AutoRenew ||
		conf.RenewBefore != 30*24*time.Hour {
		t.Fatalf("conf = %+v", conf)
	}
	if !nativeSiteWebrootMatches(conf, "/var/lib/celikpanel-agent/acme-http-01/subscriptions/7/domains/42") ||
		nativeSiteWebrootMatches(conf, "/var/www/other") {
		t.Fatalf("webroot match wrong: %+v", conf)
	}
	changed := strings.Replace(testRenewalConf, "# renew_before_expiry = 30 days", "renew_before_expiry = 14 days", 1)
	changed = strings.Replace(changed, "key_type = ecdsa", "key_type = ecdsa\nautorenew = False", 1)
	conf, err = parseNativeSiteRenewalConf([]byte(changed))
	if err != nil || conf.AutoRenew || conf.RenewBefore != 14*24*time.Hour {
		t.Fatalf("conf = %+v %v", conf, err)
	}
	if _, err := parseNativeSiteRenewalConf([]byte("version = 4.0.0\n")); err == nil {
		t.Fatal("a file without [renewalparams] was read as certbot's")
	}
}

func TestWithNativeSiteAutoRenewChangesOnlyItsLine(t *testing.T) {
	off, changed, err := withNativeSiteAutoRenew([]byte(testRenewalConf), false)
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	if want := strings.Replace(testRenewalConf, "[renewalparams]\n", "[renewalparams]\nautorenew = False\n", 1); string(off) != want {
		t.Fatalf("got:\n%s", off)
	}
	if _, changed, _ := withNativeSiteAutoRenew(off, false); changed {
		t.Fatal("an unchanged setting was rewritten")
	}
	on, changed, err := withNativeSiteAutoRenew(off, true)
	if err != nil || !changed || !strings.Contains(string(on), "autorenew = True\n") {
		t.Fatalf("on:\n%s %v", on, err)
	}
	if conf, _ := parseNativeSiteRenewalConf(on); !conf.AutoRenew {
		t.Fatal("certbot would not renew after switching on")
	}
	if _, changed, _ := withNativeSiteAutoRenew([]byte(testRenewalConf), true); changed {
		t.Fatal("no line means certbot renews already; nothing must be written")
	}
}

func TestReadNativeSiteLineageRequiresItsOwnArchive(t *testing.T) {
	nativeSiteTestStore(t)
	leaf, key := makeCertificatePair(t, "ecdsa", []string{"example.test", "www.example.test"})
	chain, _ := makeCertificatePair(t, "ecdsa", []string{"issuer.test"})
	writeNativeLineage(t, testNativeLineage, 1, leaf, key, chain, testRenewalConf)
	paths, cert, raw, err := readNativeSiteLineage(testNativeLineage)
	if err != nil {
		t.Fatal(err)
	}
	if paths.Fullchain != filepath.Join(nativeSiteConfigDir, "live", testNativeLineage, "fullchain.pem") || cert == nil || len(raw) == 0 {
		t.Fatalf("paths=%+v", paths)
	}
	if err := verifyNativeSiteCertificatePaths(paths.Fullchain, paths.Key, paths.Chain); err != nil {
		t.Fatal(err)
	}
	other := "celikpanel-site-43-00112233445566778899aabb"
	otherLeaf, otherKey := makeCertificatePair(t, "ecdsa", []string{"other.test"})
	writeNativeLineage(t, other, 1, otherLeaf, otherKey, chain, "")
	if err := verifyNativeSiteCertificatePaths(paths.Fullchain, filepath.Join(nativeSiteConfigDir, "live", other, "privkey.pem"), ""); err == nil {
		t.Fatal("a key of another lineage was accepted")
	}
	// A live link that leaves its own archive is refused.
	link := filepath.Join(nativeSiteConfigDir, "live", testNativeLineage, "privkey.pem")
	_ = os.Remove(link)
	if err := os.Symlink(filepath.Join(nativeSiteConfigDir, "archive", other, "privkey1.pem"), link); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := readNativeSiteLineage(testNativeLineage); err == nil {
		t.Fatal("a live link into another lineage's archive was accepted")
	}
	// A key that does not match.
	writeNativeLineage(t, testNativeLineage, 2, leaf, otherKey, chain, "")
	if _, _, _, err := readNativeSiteLineage(testNativeLineage); err == nil {
		t.Fatal("a mismatched key was accepted")
	}
}

func TestInspectInstalledCertificateAcceptsANativeLineage(t *testing.T) {
	nativeSiteTestStore(t)
	leaf, key := makeCertificatePair(t, "rsa", []string{"example.test"})
	chain, _ := makeCertificatePair(t, "rsa", []string{"issuer.test"})
	writeNativeLineage(t, testNativeLineage, 1, leaf, key, chain, testRenewalConf)
	paths := nativeSiteLivePaths(testNativeLineage)
	var resp ValidateCertResponse
	if err := (&Agent{}).InspectInstalledCertificate(InspectCertificateRequest{
		Domain: "example.test", CertPath: paths.Fullchain, KeyPath: paths.Key, ChainPath: paths.Chain,
	}, &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Valid || !reflect.DeepEqual(resp.DNSNames, []string{"example.test"}) {
		t.Fatalf("resp = %+v", resp)
	}
	resp = ValidateCertResponse{}
	_ = (&Agent{}).InspectInstalledCertificate(InspectCertificateRequest{
		Domain: "example.test", CertPath: paths.Cert, KeyPath: paths.Key,
	}, &resp)
	if resp.Valid {
		t.Fatal("cert.pem was accepted where the full chain is required")
	}
}

func TestVhostAcceptsOnlyTheDomainsOwnNativeLineage(t *testing.T) {
	saved := nativeSiteConfigDir
	t.Cleanup(func() { nativeSiteConfigDir = saved })
	nativeSiteConfigDir = transport.NativeSiteCertbotConfigDir
	full, key, _ := transport.NativeSiteLineagePaths(testNativeLineage)
	if err := validateVhostCertificatePathsFor("example.test", 42, full, key); err != nil {
		t.Fatal(err)
	}
	if err := validateVhostCertificatePathsFor("example.test", 43, full, key); err == nil {
		t.Fatal("another domain's lineage was accepted")
	}
	otherFull, otherKey, _ := transport.NativeSiteLineagePaths("celikpanel-site-42-ffffffffffffffffffffffff")
	if err := validateVhostCertificatePathsFor("example.test", 42, full, otherKey); err == nil {
		t.Fatal("a key from another lineage was accepted")
	}
	_ = otherFull
	if err := validateVhostCertificatePathsFor("example.test", 42, "/etc/letsencrypt/live/example.test/fullchain.pem",
		"/etc/letsencrypt/live/example.test/privkey.pem"); err != nil {
		t.Fatalf("the earlier releases' paths must keep working: %v", err)
	}
}

func TestNativeSiteDeployHookIsPublishedOnceAndNeverReplaced(t *testing.T) {
	nativeSiteTestStore(t)
	if state := nativeSiteHookState(); state != "missing" {
		t.Fatalf("state = %s", state)
	}
	state, err := ensureNativeSiteDeployHook()
	if err != nil || state != "installed" {
		t.Fatalf("state=%s err=%v", state, err)
	}
	owner := []byte("#!/bin/sh\n# the owner's own\nsystemctl reload nginx\n")
	if err := os.WriteFile(nativeSiteHookPath, owner, 0o755); err != nil {
		t.Fatal(err)
	}
	published := false
	nativeSiteHookPublish = func() error { published = true; return nil }
	state, err = ensureNativeSiteDeployHook()
	if err != nil || state != "changed" || published {
		t.Fatalf("state=%s err=%v published=%v", state, err, published)
	}
	if got, _ := os.ReadFile(nativeSiteHookPath); !bytes.Equal(got, owner) {
		t.Fatal("the owner's hook was overwritten")
	}
}

// The hook is run by sh exactly as certbot runs it, with stand-ins for nginx
// and systemctl on PATH. Only its record directory is moved into the test.
func TestNativeSiteDeployHookReloadsNginxAndRecordsTheOutcome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX sh")
	}
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh")
	}
	root := t.TempDir()
	statusDir := filepath.Join(root, "status")
	hook := filepath.Join(root, "hook")
	body := strings.Replace(nativeSiteDeployHook, "dir=/var/lib/celikpanel-site-certificates", "dir="+statusDir, 1)
	if body == nativeSiteDeployHook || !strings.Contains(nativeSiteDeployHook, "dir="+transport.NativeSiteDeployStatusDir+"\n") {
		t.Fatal("the hook's record directory is not the contract's")
	}
	if err := os.WriteFile(hook, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("sh", "-n", hook).CombinedOutput(); err != nil {
		t.Fatalf("sh -n: %v %s", err, out)
	}
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	calls := filepath.Join(root, "calls")
	write := func(name, script string) {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\necho \""+name+" $*\" >>"+calls+"\n"+script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	run := func(lineage string, nginxTest, active string) (string, error) {
		_ = os.Remove(calls)
		write("nginx", "exit "+nginxTest+"\n")
		write("systemctl", `case "$1" in is-active) exit `+active+` ;; esac
exit 0
`)
		cmd := exec.Command("sh", hook)
		cmd.Env = []string{"PATH=" + bin + ":/usr/bin:/bin", "RENEWED_LINEAGE=" + lineage}
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	live := "/etc/letsencrypt/live/" + testNativeLineage
	if out, err := run(live, "0", "0"); err != nil {
		t.Fatalf("hook failed: %v %s", err, out)
	}
	record, err := os.ReadFile(filepath.Join(statusDir, testNativeLineage))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(record)), "\n")
	if len(lines) != 4 || lines[0] != "format="+transport.NativeSiteDeployFormat || lines[1] != "lineage="+testNativeLineage || lines[3] != "nginx=reloaded" {
		t.Fatalf("record = %q", record)
	}
	if got, _ := os.ReadFile(calls); !strings.Contains(string(got), "systemctl reload nginx") || !strings.Contains(string(got), "nginx -t") {
		t.Fatalf("calls = %s", got)
	}
	// The record is what the Agent reads.
	savedDir := nativeSiteStatusDir
	nativeSiteStatusDir = statusDir
	deploy, err := readNativeSiteDeployStatus(testNativeLineage)
	nativeSiteStatusDir = savedDir
	if err != nil || deploy == nil || deploy.Nginx != "reloaded" || time.Since(deploy.CompletedAt) > time.Hour {
		t.Fatalf("deploy = %+v %v", deploy, err)
	}

	if out, err := run(live, "1", "0"); err == nil {
		t.Fatalf("a failed nginx test must fail certbot's hook run: %s", out)
	}
	record, _ = os.ReadFile(filepath.Join(statusDir, testNativeLineage))
	if !strings.Contains(string(record), "nginx=config_test_failed") {
		t.Fatalf("record = %q", record)
	}
	if got, _ := os.ReadFile(calls); strings.Contains(string(got), "systemctl reload") {
		t.Fatal("nginx was reloaded after its configuration test failed")
	}

	if _, err := run(live, "0", "3"); err != nil {
		t.Fatal("a stopped nginx is not a failed deployment")
	}
	record, _ = os.ReadFile(filepath.Join(statusDir, testNativeLineage))
	if !strings.Contains(string(record), "nginx=not_running") {
		t.Fatalf("record = %q", record)
	}

	// Another lineage (the owner's, the panel's, webmail's) is left alone.
	_ = os.RemoveAll(statusDir)
	for _, other := range []string{"/etc/letsencrypt/live/example.test", "/etc/letsencrypt/live/celikpanel-webmail-0a", "", "/etc/letsencrypt/live/celikpanel-site-1-$(id)"} {
		if out, err := run(other, "0", "0"); err != nil {
			t.Fatalf("hook failed for %q: %v %s", other, err, out)
		}
		if got, _ := os.ReadFile(calls); strings.Contains(string(got), "reload") {
			t.Fatalf("hook acted on %q", other)
		}
	}
	if _, err := os.Stat(statusDir); !os.IsNotExist(err) {
		t.Fatal("a record was written for another lineage")
	}
}

func TestReadNativeSiteRenewalFailureFindsCertbotsNewestLineForTheLineage(t *testing.T) {
	nativeSiteTestStore(t)
	if err := os.MkdirAll(nativeSiteLogDir, 0o700); err != nil {
		t.Fatal(err)
	}
	older := "2026-10-01 01:00:00,100:ERROR:certbot._internal.renewal:Failed to renew certificate " + testNativeLineage + " with error: old reason\n"
	newer := "2026-10-02 01:00:00,100:DEBUG:certbot._internal.display.obj:Notifying user\n" +
		"2026-10-02 01:00:01,100:ERROR:certbot._internal.renewal:Failed to renew certificate celikpanel-site-43-ffffffffffffffffffffffff with error: not ours\n" +
		"2026-10-02 01:00:02,100:ERROR:certbot._internal.renewal:Failed to renew certificate " + testNativeLineage + " with error: Some challenges have failed.\x1b[0m\n"
	if err := os.WriteFile(filepath.Join(nativeSiteLogDir, "letsencrypt.log.1"), []byte(older), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nativeSiteLogDir, "letsencrypt.log"), []byte(newer), 0o600); err != nil {
		t.Fatal(err)
	}
	failure, err := readNativeSiteRenewalFailure(testNativeLineage)
	if err != nil || failure == nil {
		t.Fatalf("failure=%v err=%v", failure, err)
	}
	want := time.Date(2026, 10, 2, 1, 0, 2, 0, time.Local).UTC()
	if !failure.At.Equal(want) || failure.Detail != "Some challenges have failed. [0m" {
		t.Fatalf("failure = %+v", failure)
	}
	if none, _ := readNativeSiteRenewalFailure("celikpanel-site-44-ffffffffffffffffffffffff"); none != nil {
		t.Fatalf("a failure of another lineage was attributed: %+v", none)
	}
}

func TestReadNativeSiteDeployStatusRefusesAnotherLineagesRecord(t *testing.T) {
	nativeSiteTestStore(t)
	if got, err := readNativeSiteDeployStatus(testNativeLineage); got != nil || err != nil {
		t.Fatalf("absent record = %v %v", got, err)
	}
	_ = os.MkdirAll(nativeSiteStatusDir, 0o755)
	write := func(content string) {
		_ = os.WriteFile(filepath.Join(nativeSiteStatusDir, testNativeLineage), []byte(content), 0o644)
	}
	write("format=celikpanel-site-deploy-v1\nlineage=celikpanel-site-9-ffffffffffffffffffffffff\ncompleted_at=2026-10-02T01:00:00Z\nnginx=reloaded\n")
	if _, err := readNativeSiteDeployStatus(testNativeLineage); err == nil {
		t.Fatal("another lineage's record was read")
	}
	write("format=celikpanel-site-deploy-v9\nlineage=" + testNativeLineage + "\ncompleted_at=2026-10-02T01:00:00Z\nnginx=reloaded\n")
	if _, err := readNativeSiteDeployStatus(testNativeLineage); err == nil {
		t.Fatal("an unknown format was read")
	}
}

func TestClassifyNativeSiteRenewal(t *testing.T) {
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	conf, _ := parseNativeSiteRenewalConf([]byte(testRenewalConf))
	leaf := func(issuedDaysAgo, lifetimeDays int) nativeSiteFacts {
		leaf, _ := makeCertificatePair(t, "ecdsa", []string{"example.test"})
		certs, _ := parseCertificateBundle(leaf)
		c := *certs[0]
		c.NotBefore = now.Add(-time.Duration(issuedDaysAgo) * 24 * time.Hour)
		c.NotAfter = c.NotBefore.Add(time.Duration(lifetimeDays) * 24 * time.Hour)
		return nativeSiteFacts{Conf: conf, Leaf: &c, Hook: "installed",
			Scheduler:     transport.SiteRenewalScheduler{Name: "certbot.timer", Enabled: true, Active: true, Known: true},
			ExpectWebroot: "/var/lib/celikpanel-agent/acme-http-01/subscriptions/7/domains/42", ExpectServer: letsEncryptDefaultDirectory}
	}
	cases := []struct {
		name  string
		facts func() nativeSiteFacts
		want  string
	}{
		{"healthy", func() nativeSiteFacts { return leaf(10, 90) }, transport.SiteRenewalScheduled},
		{"lineage gone", func() nativeSiteFacts { f := leaf(10, 90); f.ConfErr = os.ErrNotExist; return f }, transport.SiteRenewalLineageMissing},
		{"live gone", func() nativeSiteFacts { f := leaf(10, 90); f.CertErr = os.ErrNotExist; f.Leaf = nil; return f }, transport.SiteRenewalLineageMissing},
		{"live unreadable", func() nativeSiteFacts {
			f := leaf(10, 90)
			f.CertErr = errors.New("permission denied")
			f.Leaf = nil
			return f
		}, transport.SiteRenewalUnknown},
		{"expired", func() nativeSiteFacts { return leaf(91, 90) }, transport.SiteRenewalExpired},
		{"disabled", func() nativeSiteFacts { f := leaf(10, 90); f.Conf.AutoRenew = false; return f }, transport.SiteRenewalDisabled},
		{"no scheduler", func() nativeSiteFacts {
			f := leaf(10, 90)
			f.Scheduler = transport.SiteRenewalScheduler{Known: true}
			return f
		}, transport.SiteRenewalSchedulerMissing},
		{"scheduler unknown is not missing", func() nativeSiteFacts {
			f := leaf(10, 90)
			f.Scheduler = transport.SiteRenewalScheduler{}
			return f
		}, transport.SiteRenewalScheduled},
		{"failed after issuance", func() nativeSiteFacts {
			f := leaf(62, 90)
			f.Failure = &transport.SiteRenewalFailure{At: now.Add(-time.Hour), Detail: "connection refused"}
			return f
		}, transport.SiteRenewalFailed},
		{"failure before the current certificate is history", func() nativeSiteFacts {
			f := leaf(5, 90)
			f.Failure = &transport.SiteRenewalFailure{At: now.Add(-6 * 24 * time.Hour), Detail: "old"}
			return f
		}, transport.SiteRenewalScheduled},
		{"overdue without a failure", func() nativeSiteFacts { return leaf(65, 90) }, transport.SiteRenewalOverdue},
		{"inside the window, not overdue yet", func() nativeSiteFacts { return leaf(61, 90) }, transport.SiteRenewalScheduled},
		{"renewed, nginx test failed", func() nativeSiteFacts {
			f := leaf(1, 90)
			f.Deploy = &transport.SiteRenewalDeploy{CompletedAt: f.Leaf.NotBefore.Add(time.Minute), Nginx: "config_test_failed"}
			return f
		}, transport.SiteRenewalDeployIncomplete},
		{"an older deployment's outcome is history", func() nativeSiteFacts {
			f := leaf(1, 90)
			f.Deploy = &transport.SiteRenewalDeploy{CompletedAt: f.Leaf.NotBefore.Add(-60 * 24 * time.Hour), Nginx: "reload_failed"}
			return f
		}, transport.SiteRenewalScheduled},
		{"hook missing", func() nativeSiteFacts { f := leaf(10, 90); f.Hook = "missing"; return f }, transport.SiteRenewalHookMissing},
		{"hook changed", func() nativeSiteFacts { f := leaf(10, 90); f.Hook = "changed"; return f }, transport.SiteRenewalHookChanged},
		{"owner switched the authenticator", func() nativeSiteFacts { f := leaf(10, 90); f.Conf.Authenticator = "dns-cloudflare"; return f }, transport.SiteRenewalOwnerConfigured},
		{"another validation directory", func() nativeSiteFacts { f := leaf(10, 90); f.ExpectWebroot = "/elsewhere"; return f }, transport.SiteRenewalOwnerConfigured},
		{"another ACME server", func() nativeSiteFacts {
			f := leaf(10, 90)
			f.ExpectServer = "https://acme.zerossl.com/v2/DV90"
			return f
		}, transport.SiteRenewalOwnerConfigured},
	}
	for _, tc := range cases {
		got, detail := classifyNativeSiteRenewal(tc.facts(), now)
		if got != tc.want {
			t.Errorf("%s: state = %s (%s), want %s", tc.name, got, detail, tc.want)
		}
		if transport.SiteRenewalStateIsProblem(got) && detail == "" {
			t.Errorf("%s: a problem without a reason", tc.name)
		}
	}
}

func TestSiteCertificateRenewalStateReadsTheStore(t *testing.T) {
	nativeSiteTestStore(t)
	nativeSiteRunQuery = func(_ context.Context, name string, args ...string) ([]byte, error) {
		if name == "systemctl" && len(args) == 2 && args[1] == "certbot.timer" {
			if args[0] == "is-enabled" {
				return []byte("enabled\n"), nil
			}
			return []byte("active\n"), nil
		}
		return []byte("not-found\n"), errors.New("exit status 4")
	}
	leaf, key := makeCertificatePair(t, "ecdsa", []string{"example.test"})
	chain, _ := makeCertificatePair(t, "ecdsa", []string{"issuer.test"})
	writeNativeLineage(t, testNativeLineage, 1, leaf, key, chain, testRenewalConf)
	if _, err := ensureNativeSiteDeployHook(); err != nil {
		t.Fatal(err)
	}
	var resp transport.SiteCertificateRenewalStateResponse
	if err := (&Agent{}).SiteCertificateRenewalStateV1(&transport.SiteCertificateRenewalStateRequest{
		Domain: "example.test", DomainID: 42, SubscriptionID: 7, LineageName: testNativeLineage,
	}, &resp); err != nil {
		t.Fatal(err)
	}
	if _, err := exec.LookPath("systemctl"); err != nil {
		t.Skipf("no systemctl on this host; state=%s", resp.State)
	}
	if resp.State != transport.SiteRenewalScheduled || resp.Scheduler.Name != "certbot.timer" || !resp.AutoRenew ||
		resp.Hook != "installed" || resp.LeafSHA256 == "" || resp.Authenticator != "webroot" {
		t.Fatalf("resp = %+v", resp)
	}
	resp = transport.SiteCertificateRenewalStateResponse{}
	_ = (&Agent{}).SiteCertificateRenewalStateV1(&transport.SiteCertificateRenewalStateRequest{
		DomainID: 43, LineageName: testNativeLineage,
	}, &resp)
	if resp.State != transport.SiteRenewalUnknown || resp.Error == "" {
		t.Fatalf("another domain's lineage was read: %+v", resp)
	}
}

func TestIssueNativeSiteCertificateUsesAFreshLineageAndReportsRenewal(t *testing.T) {
	nativeSiteTestStore(t)
	leaf, key := makeCertificatePair(t, "ecdsa", []string{"example.test", "www.example.test"})
	chain, _ := makeCertificatePair(t, "ecdsa", []string{"issuer.test"})
	var gotArgs []string
	var eabSeen string
	nativeSiteCertbotRun = func(_ context.Context, args ...string) ([]byte, error) {
		gotArgs = append([]string(nil), args...)
		for i := range args {
			if args[i] == "--config" {
				content, _ := os.ReadFile(args[i+1])
				eabSeen = string(content)
			}
		}
		lineage := ""
		for i := range args {
			if args[i] == "--cert-name" {
				lineage = args[i+1]
			}
		}
		writeNativeLineage(t, lineage, 1, leaf, key, chain, strings.ReplaceAll(testRenewalConf, testNativeLineage, lineage))
		return []byte("Successfully received certificate."), nil
	}
	nativeSiteEnableTimer = func(context.Context) error { return errors.New("Unit certbot.timer not found") }
	var resp IssueLetsEncryptResponse
	issueNativeSiteCertificate(IssueLetsEncryptRequest{
		Domain: "example.test", Aliases: []string{"www.example.test"}, DomainID: 42, AutoRenew: true,
		ACMEServer: "https://acme.zerossl.com/v2/DV90", EABKeyID: "kid", EABHMACKey: "hmacvalue",
	}, "/w", &resp)
	if !resp.Success {
		t.Fatalf("resp = %+v", resp)
	}
	if !transport.NativeSiteLineageBelongsTo(resp.LineageName, 42) {
		t.Fatalf("lineage = %q", resp.LineageName)
	}
	full, keyPath, chainPath := filepath.Join(nativeSiteConfigDir, "live", resp.LineageName, "fullchain.pem"),
		filepath.Join(nativeSiteConfigDir, "live", resp.LineageName, "privkey.pem"),
		filepath.Join(nativeSiteConfigDir, "live", resp.LineageName, "chain.pem")
	if resp.CertPath != full || resp.KeyPath != keyPath || resp.ChainPath != chainPath {
		t.Fatalf("paths = %s %s %s", resp.CertPath, resp.KeyPath, resp.ChainPath)
	}
	if !strings.Contains(eabSeen, "eab-hmac-key = hmacvalue") || strings.Contains(strings.Join(gotArgs, " "), "hmacvalue") {
		t.Fatalf("EAB handling: file=%q args=%v", eabSeen, gotArgs)
	}
	if !strings.Contains(resp.RenewalWarning, "certbot timer could not be enabled") {
		t.Fatalf("warning = %q", resp.RenewalWarning)
	}
	if nativeSiteHookState() != "installed" {
		t.Fatal("the deploy hook was not published with the first certificate")
	}

	// Not renewing automatically is written into certbot's own file.
	var off IssueLetsEncryptResponse
	nativeSiteEnableTimer = func(context.Context) error { return nil }
	issueNativeSiteCertificate(IssueLetsEncryptRequest{Domain: "example.test", DomainID: 42, AutoRenew: false}, "/w", &off)
	if !off.Success || off.LineageName == resp.LineageName {
		t.Fatalf("second issuance = %+v", off)
	}
	content, _ := os.ReadFile(nativeSiteRenewalConfPath(off.LineageName))
	if conf, _ := parseNativeSiteRenewalConf(content); conf.AutoRenew {
		t.Fatal("auto renew off was not written into certbot's configuration")
	}

	// A failed run still names its lineage, for the Panel's cleanup.
	nativeSiteCertbotRun = func(context.Context, ...string) ([]byte, error) {
		return []byte("Certbot failed to authenticate some domains"), errors.New("exit status 1")
	}
	var failed IssueLetsEncryptResponse
	issueNativeSiteCertificate(IssueLetsEncryptRequest{Domain: "example.test", DomainID: 42, AutoRenew: true}, "/w", &failed)
	if failed.Success || !transport.NativeSiteLineageBelongsTo(failed.LineageName, 42) || failed.Failure != transport.CertificateFailureValidation {
		t.Fatalf("failed = %+v", failed)
	}
}

func TestSetSiteCertificateAutoRenewWritesCertbotsSetting(t *testing.T) {
	nativeSiteTestStore(t)
	leaf, key := makeCertificatePair(t, "ecdsa", []string{"example.test"})
	writeNativeLineage(t, testNativeLineage, 1, leaf, key, leaf, testRenewalConf)
	var resp transport.SetSiteCertificateAutoRenewResponse
	if err := (&Agent{}).SetSiteCertificateAutoRenewV1(&transport.SetSiteCertificateAutoRenewRequest{
		DomainID: 42, LineageName: testNativeLineage, AutoRenew: false,
	}, &resp); err != nil || resp.Error != "" || !resp.Changed {
		t.Fatalf("resp=%+v err=%v", resp, err)
	}
	content, _ := os.ReadFile(nativeSiteRenewalConfPath(testNativeLineage))
	if conf, _ := parseNativeSiteRenewalConf(content); conf.AutoRenew {
		t.Fatal("certbot would still renew")
	}
	resp = transport.SetSiteCertificateAutoRenewResponse{}
	_ = (&Agent{}).SetSiteCertificateAutoRenewV1(&transport.SetSiteCertificateAutoRenewRequest{
		DomainID: 43, LineageName: testNativeLineage, AutoRenew: true,
	}, &resp)
	if resp.Error == "" {
		t.Fatal("another domain's lineage was changed")
	}
	// A lineage that is gone renews nothing: "off" is satisfied.
	gone := "celikpanel-site-42-ffffffffffffffffffffffff"
	resp = transport.SetSiteCertificateAutoRenewResponse{}
	_ = (&Agent{}).SetSiteCertificateAutoRenewV1(&transport.SetSiteCertificateAutoRenewRequest{
		DomainID: 42, LineageName: gone, AutoRenew: false,
	}, &resp)
	if resp.Error != "" || resp.Changed {
		t.Fatalf("resp = %+v", resp)
	}
	resp = transport.SetSiteCertificateAutoRenewResponse{}
	_ = (&Agent{}).SetSiteCertificateAutoRenewV1(&transport.SetSiteCertificateAutoRenewRequest{
		DomainID: 42, LineageName: gone, AutoRenew: true,
	}, &resp)
	if resp.Error == "" {
		t.Fatal("switching renewal on for a missing lineage must say so")
	}
}

func TestRenewLetsEncryptCertificateNeverRenewsANativeLineage(t *testing.T) {
	var resp RenewCertResponse
	if err := (&Agent{}).RenewLetsEncryptCertificate(RenewCertRequest{
		Domain: "example.test", DomainID: 42, SubscriptionID: 7, LineageName: testNativeLineage,
	}, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Success || resp.Error != transport.ErrNativeSiteRenewal {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestDeleteCertLineageRemovesANativeLineageFromTheStandardStore(t *testing.T) {
	harness := newCertificateCleanupTestHarness(t)
	writeCertificateCleanupRenewal(t, harness.legacy, testNativeLineage)
	var resp DeleteCertLineageResponse
	if err := (&Agent{}).DeleteCertLineage(&DeleteCertLineageRequest{
		Domain: "example.test", LineageNames: []string{testNativeLineage},
	}, &resp); err != nil {
		t.Fatal(err)
	}
	legacy := harness.isolated
	legacy.ConfigDir = harness.legacy
	if resp.Error != "" || !reflect.DeepEqual(harness.calls, [][]string{expectedCertificateDeleteArgs(legacy, testNativeLineage)}) {
		t.Fatalf("resp=%+v calls=%v", resp, harness.calls)
	}
}

func TestReconcileSiteCertLineagesKeepsReferencedNativeLineagesAndDeletesOrphans(t *testing.T) {
	harness := newCertificateCleanupTestHarness(t)
	orphan := "celikpanel-site-43-ffffffffffffffffffffffff"
	for _, lineage := range []string{testNativeLineage, orphan, "example.test", "celikpanel-webmail-0123"} {
		writeCertificateCleanupRenewal(t, harness.legacy, lineage)
	}
	var resp ReconcileSiteCertLineagesResponse
	if err := (&Agent{}).ReconcileSiteCertLineages(&ReconcileSiteCertLineagesRequest{
		ReferencedLineages: []string{testNativeLineage},
	}, &resp); err != nil {
		t.Fatal(err)
	}
	legacy := harness.isolated
	legacy.ConfigDir = harness.legacy
	if resp.Error != "" || resp.Deleted != 1 || !reflect.DeepEqual(harness.calls, [][]string{expectedCertificateDeleteArgs(legacy, orphan)}) {
		t.Fatalf("resp=%+v calls=%v", resp, harness.calls)
	}
}
