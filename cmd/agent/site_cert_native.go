package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/hostingpath"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Website certificates renewed by the operating system (D-022, D-039).
//
// Issuance runs certbot against the standard store (/etc/letsencrypt) with a
// fresh lineage celikpanel-site-<domain id>-<24 hex>, the webroot
// authenticator and the site's own challenge root. certbot writes the
// renewal configuration itself (authenticator, webroot map, ACME server,
// account); certbot's timer (or cron) renews it; the deploy hook below
// reloads nginx. nginx names certbot's live files, so a renewed certificate
// is served without any CelikPanel program. The Agent only reads what the
// operating system did (SiteCertificateRenewalStateV1) and never renews a
// native lineage itself, so two renewers never race on one lineage.
//
// İşletim sisteminin yenilediği web sitesi sertifikaları: certbot standart
// depoda, kendi zamanlayıcısıyla yeniler; dağıtım kancası nginx'i yeniden
// yükler. Agent yalnız okur; yerel bir lineage'ı kendisi asla yenilemez.

var (
	nativeSiteConfigDir    = transport.NativeSiteCertbotConfigDir
	nativeSiteHookPath     = transport.NativeSiteDeployHookPath
	nativeSiteStatusDir    = transport.NativeSiteDeployStatusDir
	nativeSiteLogDir       = "/var/log/letsencrypt"
	nativeSiteCronEntry    = "/etc/cron.d/certbot"
	nativeSiteLineageRead  = rand.Read
	nativeSiteHookPublish  = publishNativeSiteDeployHook
	nativeSiteRunQuery     = runNativeSiteQuery
	nativeSiteEnableTimer  = ensurePanelCertRenewalScheduler
	nativeSiteCertbotRun   = runNativeSiteCertbot
	nativeSiteNow          = time.Now
	nativeSiteSchedulerIDs = []string{"certbot.timer", "certbot-renew.timer", "snap.certbot.renew.timer"}
)

const (
	nativeSiteHookVersionLine = "# celikpanel-site-deploy-hook v1"
	// certbot's own default when a lineage has no renew_before_expiry.
	nativeSiteDefaultRenewBefore = 30 * 24 * time.Hour
	// A twice-daily timer with up to 12 h random delay has had several
	// chances after this long inside the renewal window.
	nativeSiteOverdueGrace = 3 * 24 * time.Hour
	nativeSiteMaxConfBytes = 256 << 10
	nativeSiteMaxLogBytes  = 512 << 10
	nativeSiteMaxLogFiles  = 30
	nativeSiteMaxDetail    = 300
)

// nativeSiteDeployHook is the deploy hook. It is plain POSIX sh with no
// CelikPanel program, so it keeps working after the Panel and the Agent are
// removed. certbot runs every file in renewal-hooks/deploy after each
// renewed lineage with RENEWED_LINEAGE set; this one acts only on
// celikpanel-site-* lineages. It tests nginx's configuration before
// reloading, records the outcome in one small file the Panel reads, and
// exits non-zero when nginx could not load the renewed certificate, so
// certbot's own run (and its systemd unit) is marked failed.
const nativeSiteDeployHook = nativeSiteHookVersionLine + `
# Managed by CelikPanel. certbot runs this after it renewed a certificate
# (certbot.timer or cron). For website certificates named celikpanel-site-*
# it checks nginx's configuration and reloads nginx so the renewed
# certificate is served. It needs no CelikPanel program and keeps working if
# CelikPanel is removed. If you change or remove this file, CelikPanel leaves
# it as it is and says so on the certificate's page.
# CelikPanel tarafından yönetilir: yenilenen web sitesi sertifikası için
# nginx yapılandırmasını sınar ve nginx'i yeniden yükler. Panel gerekmez.
set -u
lineage=${RENEWED_LINEAGE:-}
case "$lineage" in
  */live/celikpanel-site-*) ;;
  *) exit 0 ;;
esac
name=${lineage##*/}
case "$name" in
  ""|*[!a-z0-9-]*) exit 0 ;;
esac
result=not_installed
if command -v nginx >/dev/null 2>&1; then
  if ! nginx -t >/dev/null 2>&1; then
    result=config_test_failed
  elif systemctl is-active --quiet nginx >/dev/null 2>&1; then
    if systemctl reload nginx >/dev/null 2>&1; then
      result=reloaded
    else
      result=reload_failed
    fi
  else
    result=not_running
  fi
fi
dir=/var/lib/celikpanel-site-certificates
umask 022
if mkdir -p "$dir" 2>/dev/null; then
  tmp="$dir/.$name.$$"
  if printf 'format=celikpanel-site-deploy-v1\nlineage=%s\ncompleted_at=%s\nnginx=%s\n' \
      "$name" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$result" >"$tmp" 2>/dev/null; then
    mv -f "$tmp" "$dir/$name" 2>/dev/null || rm -f "$tmp"
  fi
fi
case "$result" in
  reloaded|not_running|not_installed) exit 0 ;;
esac
echo "celikpanel-site-certificates: $name was renewed but nginx did not load it ($result)" >&2
exit 1
`

// newNativeSiteLineage is a fresh lineage name for one domain identity.
func newNativeSiteLineage(domainID int) (string, error) {
	random := make([]byte, 12)
	n, err := nativeSiteLineageRead(random)
	if err != nil {
		return "", fmt.Errorf("generate certificate lineage identity: %w", err)
	}
	if n != len(random) {
		return "", fmt.Errorf("generate certificate lineage identity: short random read")
	}
	return transport.NativeSiteLineageName(domainID, random)
}

func nativeSiteStorage() certbotStorage {
	return certbotStorage{ConfigDir: nativeSiteConfigDir}
}

func nativeSiteLiveDir(lineage string) string {
	return filepath.Join(nativeSiteConfigDir, "live", lineage)
}

func nativeSiteRenewalConfPath(lineage string) string {
	return filepath.Join(nativeSiteConfigDir, "renewal", lineage+".conf")
}

// nativeSiteLivePaths are the stable files nginx names.
func nativeSiteLivePaths(lineage string) certificateVersionPaths {
	dir := nativeSiteLiveDir(lineage)
	return certificateVersionPaths{
		Cert:      filepath.Join(dir, "cert.pem"),
		Key:       filepath.Join(dir, "privkey.pem"),
		Chain:     filepath.Join(dir, "chain.pem"),
		Fullchain: filepath.Join(dir, "fullchain.pem"),
	}
}

// nativeSiteLineageOfPath is the lineage whose live file candidate names
// exactly, under this Agent's store.
func nativeSiteLineageOfPath(candidate string) (string, bool) {
	if candidate == "" || filepath.Clean(candidate) != candidate {
		return "", false
	}
	switch filepath.Base(candidate) {
	case "fullchain.pem", "privkey.pem", "chain.pem", "cert.pem":
	default:
		return "", false
	}
	dir := filepath.Dir(candidate)
	if filepath.Dir(dir) != filepath.Join(nativeSiteConfigDir, "live") {
		return "", false
	}
	lineage := filepath.Base(dir)
	return lineage, transport.IsNativeSiteLineage(lineage)
}

// buildNativeSiteIssueArgs is certbot's command line. EAB credentials never
// appear in it (certbot logs its arguments): they go in the private
// configuration file configFile names, which certbot reads with --config
// and does not copy into the lineage's renewal configuration.
func buildNativeSiteIssueArgs(req IssueLetsEncryptRequest, webroot, lineage, configFile string) []string {
	args := []string{"certonly"}
	if configFile != "" {
		args = append(args, "--config", configFile)
	}
	args = append(args,
		"--webroot", "-w", webroot,
		"--preferred-challenges", "http",
		"--cert-name", lineage,
		"--agree-tos", "--non-interactive",
	)
	if email := strings.TrimSpace(req.Email); email != "" {
		args = append(args, "--email", email)
	} else {
		args = append(args, "--register-unsafely-without-email")
	}
	if req.ACMEServer != "" {
		args = append(args, "--server", req.ACMEServer)
	}
	seen := make(map[string]struct{}, len(req.Aliases)+1)
	for _, domain := range append([]string{req.Domain}, req.Aliases...) {
		domain = strings.TrimSpace(domain)
		key := strings.ToLower(domain)
		if domain == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		args = append(args, "-d", domain)
	}
	return args
}

var eabValueRE = regexp.MustCompile(`^[A-Za-z0-9_\-=+/.]{1,512}$`)

// writeNativeSiteEABConfig puts the EAB credentials into a private certbot
// configuration file (0600 in a 0700 directory) and returns its path and the
// function that removes it. No file is written without credentials.
func writeNativeSiteEABConfig(req IssueLetsEncryptRequest) (string, func(), error) {
	kid, hmac := strings.TrimSpace(req.EABKeyID), strings.TrimSpace(req.EABHMACKey)
	if kid == "" || hmac == "" {
		return "", func() {}, nil
	}
	if !eabValueRE.MatchString(kid) || !eabValueRE.MatchString(hmac) {
		return "", func() {}, errors.New("EAB credentials contain characters a certificate authority never issues")
	}
	dir, err := os.MkdirTemp("", "celikpanel-eab-")
	if err != nil {
		return "", func() {}, fmt.Errorf("prepare EAB configuration: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	if err := os.Chmod(dir, 0o700); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("prepare EAB configuration: %w", err)
	}
	file := filepath.Join(dir, "eab.ini")
	content := "eab-kid = " + kid + "\neab-hmac-key = " + hmac + "\n"
	if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("prepare EAB configuration: %w", err)
	}
	return file, cleanup, nil
}

func runNativeSiteCertbot(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "certbot", args...)
	cmd.WaitDelay = 2 * time.Second
	return cmd.CombinedOutput()
}

func runNativeSiteQuery(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = time.Second
	return cmd.CombinedOutput()
}

// readNativeSiteLineage reads and checks the certificate certbot's live
// files name now: every live file must resolve inside the lineage's own
// archive directory, the key must match the certificate.
func readNativeSiteLineage(lineage string) (certificateVersionPaths, *x509.Certificate, []byte, error) {
	if !transport.IsNativeSiteLineage(lineage) {
		return certificateVersionPaths{}, nil, nil, errors.New("invalid certificate lineage identity")
	}
	paths := nativeSiteLivePaths(lineage)
	archive := filepath.Join(nativeSiteConfigDir, "archive", lineage)
	read := func(file string) ([]byte, error) {
		resolved, err := filepath.EvalSymlinks(file)
		if err != nil {
			return nil, fmt.Errorf("read certbot %s: %w", filepath.Base(file), err)
		}
		realArchive, err := filepath.EvalSymlinks(archive)
		if err != nil {
			return nil, fmt.Errorf("read certbot archive: %w", err)
		}
		if filepath.Dir(resolved) != realArchive {
			return nil, fmt.Errorf("certbot %s does not name a file of its own lineage", filepath.Base(file))
		}
		return os.ReadFile(resolved)
	}
	certPEM, err := read(paths.Cert)
	if err != nil {
		return paths, nil, nil, err
	}
	keyPEM, err := read(paths.Key)
	if err != nil {
		return paths, nil, nil, err
	}
	if _, err := read(paths.Chain); err != nil {
		return paths, nil, nil, err
	}
	if _, err := read(paths.Fullchain); err != nil {
		return paths, nil, nil, err
	}
	certificates, err := parseCertificateBundle(string(certPEM))
	if err != nil {
		return paths, nil, nil, err
	}
	key, err := parsePrivateKey(string(keyPEM))
	if err != nil {
		return paths, nil, nil, fmt.Errorf("invalid certbot private key: %w", err)
	}
	matches, err := certificateMatchesPrivateKey(certificates[0], key)
	if err != nil {
		return paths, nil, nil, err
	}
	if !matches {
		return paths, nil, nil, errors.New("certbot private key does not match its certificate")
	}
	return paths, certificates[0], certificates[0].Raw, nil
}

// verifyNativeSiteCertificatePaths accepts the exact live files of one
// native lineage as a site's runtime pair.
func verifyNativeSiteCertificatePaths(certPath, keyPath, chainPath string) error {
	certLineage, ok := nativeSiteLineageOfPath(certPath)
	if !ok || filepath.Base(certPath) != "fullchain.pem" {
		return errors.New("certificate path is not a certbot lineage's full chain")
	}
	keyLineage, ok := nativeSiteLineageOfPath(keyPath)
	if !ok || keyLineage != certLineage || filepath.Base(keyPath) != "privkey.pem" {
		return errors.New("certificate and private key are not from the same certbot lineage")
	}
	if strings.TrimSpace(chainPath) != "" {
		chainLineage, ok := nativeSiteLineageOfPath(chainPath)
		if !ok || chainLineage != certLineage || filepath.Base(chainPath) != "chain.pem" {
			return errors.New("certificate chain is not from the same certbot lineage")
		}
	}
	_, _, _, err := readNativeSiteLineage(certLineage)
	return err
}

// ensureNativeSiteDeployHook publishes the deploy hook when it is absent and
// reads what is there: installed (CelikPanel's bytes), changed (kept as the
// owner left it), missing or unreadable.
func ensureNativeSiteDeployHook() (string, error) {
	state := nativeSiteHookState()
	if state != "missing" {
		return state, nil
	}
	if err := nativeSiteHookPublish(); err != nil {
		return nativeSiteHookState(), err
	}
	return nativeSiteHookState(), nil
}

func nativeSiteHookState() string {
	info, err := os.Lstat(nativeSiteHookPath)
	if errors.Is(err, os.ErrNotExist) {
		return "missing"
	}
	if err != nil {
		return "unreadable"
	}
	if !info.Mode().IsRegular() {
		return "changed"
	}
	content, err := readBounded(nativeSiteHookPath, int64(len(nativeSiteDeployHook))+4096)
	if err != nil {
		return "unreadable"
	}
	if !bytes.Equal(content, []byte(nativeSiteDeployHook)) || info.Mode().Perm()&0o111 == 0 {
		return "changed"
	}
	return "installed"
}

func readBounded(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(content)) > limit {
		return nil, errors.New("file is larger than expected")
	}
	return content, nil
}

// nativeSiteRenewalConf is what the Panel reads from certbot's renewal
// configuration of one lineage.
type nativeSiteRenewalConf struct {
	Authenticator string
	Server        string
	AutoRenew     bool
	RenewBefore   time.Duration
	WebrootPaths  []string
	WebrootMap    map[string]string
}

// parseNativeSiteRenewalConf reads certbot's renewal configuration (a
// configobj file: top-level keys, [renewalparams], [[webroot_map]]).
func parseNativeSiteRenewalConf(content []byte) (nativeSiteRenewalConf, error) {
	conf := nativeSiteRenewalConf{AutoRenew: true, RenewBefore: nativeSiteDefaultRenewBefore, WebrootMap: map[string]string{}}
	section := ""
	sawParams := false
	scanner := bufio.NewScanner(bytes.NewReader(content))
	scanner.Buffer(make([]byte, 0, 4096), 64<<10)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			section = strings.ToLower(strings.Trim(line, "[] "))
			if section == "renewalparams" {
				sawParams = true
			}
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		switch section {
		case "":
			if key == "renew_before_expiry" {
				if duration, ok := parseCertbotInterval(value); ok {
					conf.RenewBefore = duration
				}
			}
		case "renewalparams":
			switch key {
			case "authenticator":
				conf.Authenticator = value
			case "server":
				conf.Server = value
			case "autorenew":
				conf.AutoRenew = !strings.EqualFold(value, "false")
			case "webroot_path":
				for _, part := range strings.Split(value, ",") {
					if part = strings.TrimSpace(part); part != "" {
						conf.WebrootPaths = append(conf.WebrootPaths, part)
					}
				}
			}
		case "webroot_map":
			conf.WebrootMap[strings.ToLower(key)] = value
		}
	}
	if err := scanner.Err(); err != nil {
		return conf, err
	}
	if !sawParams {
		return conf, errors.New("the renewal configuration has no [renewalparams] section")
	}
	return conf, nil
}

var certbotIntervalRE = regexp.MustCompile(`^\s*(\d+)\s*(day|days|d|week|weeks|w|hour|hours|h)?\s*$`)

// parseCertbotInterval reads certbot's renew_before_expiry ("30 days").
func parseCertbotInterval(value string) (time.Duration, bool) {
	match := certbotIntervalRE.FindStringSubmatch(strings.ToLower(value))
	if match == nil {
		return 0, false
	}
	n, err := strconv.Atoi(match[1])
	if err != nil || n <= 0 || n > 3650 {
		return 0, false
	}
	switch match[2] {
	case "week", "weeks", "w":
		return time.Duration(n) * 7 * 24 * time.Hour, true
	case "hour", "hours", "h":
		return time.Duration(n) * time.Hour, true
	default:
		return time.Duration(n) * 24 * time.Hour, true
	}
}

// readNativeSiteDeployStatus reads the hook's record for the lineage.
func readNativeSiteDeployStatus(lineage string) (*transport.SiteRenewalDeploy, error) {
	content, err := readBounded(filepath.Join(nativeSiteStatusDir, lineage), 4096)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	values := map[string]string{}
	for i, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if i == 0 && line != "format="+transport.NativeSiteDeployFormat {
			return nil, errors.New("deploy status has an unknown format")
		}
		if key, value, ok := strings.Cut(line, "="); ok {
			values[key] = value
		}
	}
	if values["lineage"] != lineage {
		return nil, errors.New("deploy status names another lineage")
	}
	at, err := time.Parse(time.RFC3339, values["completed_at"])
	if err != nil {
		return nil, errors.New("deploy status has no completion time")
	}
	switch values["nginx"] {
	case "reloaded", "not_running", "not_installed", "config_test_failed", "reload_failed":
	default:
		return nil, errors.New("deploy status has an unknown web server outcome")
	}
	return &transport.SiteRenewalDeploy{CompletedAt: at.UTC(), Nginx: values["nginx"]}, nil
}

// certbot's log line for a failed renewal (certbot._internal.renewal):
// "2026-10-30 12:00:01,234:ERROR:certbot._internal.renewal:Failed to renew
// certificate <name> with error: <reason>". Its time is the host's local time.
var certbotRenewFailureRE = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}),\d+:[A-Z]+:[^:]*:Failed to renew certificate (\S+) with error: (.*)$`)

// readNativeSiteRenewalFailure is certbot's newest logged failure for the
// lineage in its own log files (certbot rotates them at every run).
func readNativeSiteRenewalFailure(lineage string) (*transport.SiteRenewalFailure, error) {
	entries, err := os.ReadDir(nativeSiteLogDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	type logFile struct {
		path string
		mod  time.Time
	}
	var files []logFile
	for _, entry := range entries {
		name := entry.Name()
		if name != "letsencrypt.log" && !strings.HasPrefix(name, "letsencrypt.log.") {
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		files = append(files, logFile{filepath.Join(nativeSiteLogDir, name), info.ModTime()})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].mod.After(files[j].mod) })
	if len(files) > nativeSiteMaxLogFiles {
		files = files[:nativeSiteMaxLogFiles]
	}
	var newest *transport.SiteRenewalFailure
	for _, file := range files {
		content, err := readTail(file.path, nativeSiteMaxLogBytes)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(content), "\n") {
			match := certbotRenewFailureRE.FindStringSubmatch(strings.TrimRight(line, "\r"))
			if match == nil || match[2] != lineage {
				continue
			}
			at, err := time.ParseInLocation("2006-01-02 15:04:05", match[1], time.Local)
			if err != nil {
				continue
			}
			if newest == nil || at.After(newest.At) {
				newest = &transport.SiteRenewalFailure{At: at.UTC(), Detail: boundedNativeDetail(match[3])}
			}
		}
	}
	return newest, nil
}

func readTail(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > limit {
		if _, err := file.Seek(info.Size()-limit, io.SeekStart); err != nil {
			return nil, err
		}
	}
	return io.ReadAll(io.LimitReader(file, limit))
}

func boundedNativeDetail(value string) string {
	value = strings.Join(strings.Fields(strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, value)), " ")
	if len(value) > nativeSiteMaxDetail {
		value = value[:nativeSiteMaxDetail]
	}
	return value
}

// readNativeSiteScheduler finds what runs `certbot renew` on this host.
func readNativeSiteScheduler(ctx context.Context) transport.SiteRenewalScheduler {
	if _, err := exec.LookPath("systemctl"); err == nil {
		known := false
		for _, unit := range nativeSiteSchedulerIDs {
			out, err := nativeSiteRunQuery(ctx, "systemctl", "is-enabled", unit)
			state := strings.TrimSpace(string(out))
			if err == nil || state != "" {
				known = true
			}
			if state == "enabled" || state == "static" || state == "enabled-runtime" {
				activeOut, _ := nativeSiteRunQuery(ctx, "systemctl", "is-active", unit)
				return transport.SiteRenewalScheduler{
					Name: unit, Enabled: true, Known: true,
					Active: strings.TrimSpace(string(activeOut)) == "active",
				}
			}
		}
		if info, err := os.Stat(nativeSiteCronEntry); err == nil && info.Mode().IsRegular() {
			// Debian's cron entry runs only when systemd is not the init.
			if _, err := os.Stat("/run/systemd/system"); err != nil {
				return transport.SiteRenewalScheduler{Name: "cron", Enabled: true, Active: true, Known: true}
			}
		}
		return transport.SiteRenewalScheduler{Known: known}
	}
	if info, err := os.Stat(nativeSiteCronEntry); err == nil && info.Mode().IsRegular() {
		return transport.SiteRenewalScheduler{Name: "cron", Enabled: true, Active: true, Known: true}
	}
	return transport.SiteRenewalScheduler{Known: true}
}

// nativeSiteFacts is everything the state is decided from.
type nativeSiteFacts struct {
	ConfErr       error
	Conf          nativeSiteRenewalConf
	CertErr       error
	Leaf          *x509.Certificate
	Hook          string
	Scheduler     transport.SiteRenewalScheduler
	Deploy        *transport.SiteRenewalDeploy
	Failure       *transport.SiteRenewalFailure
	ExpectWebroot string
	ExpectServer  string
}

const letsEncryptDefaultDirectory = "https://acme-v02.api.letsencrypt.org/directory"

// classifyNativeSiteRenewal decides one state from the facts, the most
// urgent first. Pure, so every case is tested.
func classifyNativeSiteRenewal(facts nativeSiteFacts, now time.Time) (string, string) {
	if errors.Is(facts.ConfErr, os.ErrNotExist) || errors.Is(facts.CertErr, os.ErrNotExist) {
		return transport.SiteRenewalLineageMissing, "certbot's renewal configuration or live files of this lineage are gone"
	}
	if facts.CertErr != nil {
		return transport.SiteRenewalUnknown, "the certificate certbot's live files name could not be read: " + boundedNativeDetail(facts.CertErr.Error())
	}
	leaf := facts.Leaf
	if !now.Before(leaf.NotAfter) {
		if facts.Failure != nil && facts.Failure.At.After(leaf.NotBefore) {
			return transport.SiteRenewalExpired, "the certificate expired; certbot's last renewal failed: " + facts.Failure.Detail
		}
		return transport.SiteRenewalExpired, "the certificate expired and certbot has not renewed it"
	}
	if facts.ConfErr != nil {
		return transport.SiteRenewalUnknown, "certbot's renewal configuration could not be read: " + boundedNativeDetail(facts.ConfErr.Error())
	}
	conf := facts.Conf
	if !conf.AutoRenew {
		return transport.SiteRenewalDisabled, "certbot's renewal configuration sets autorenew = False"
	}
	if facts.Scheduler.Known && facts.Scheduler.Name == "" {
		return transport.SiteRenewalSchedulerMissing, "no certbot timer or cron entry was found on this server"
	}
	if facts.Failure != nil && facts.Failure.At.After(leaf.NotBefore) &&
		(facts.Deploy == nil || facts.Failure.At.After(facts.Deploy.CompletedAt)) {
		return transport.SiteRenewalFailed, facts.Failure.Detail
	}
	if facts.Deploy != nil && !facts.Deploy.CompletedAt.Before(leaf.NotBefore.Add(-time.Hour)) {
		switch facts.Deploy.Nginx {
		case "config_test_failed":
			return transport.SiteRenewalDeployIncomplete, "certbot renewed it, but nginx's configuration test failed, so nginx was not reloaded"
		case "reload_failed":
			return transport.SiteRenewalDeployIncomplete, "certbot renewed it, but nginx could not be reloaded"
		}
	}
	renewAfter, grace := nativeSiteRenewalWindow(leaf, conf.RenewBefore)
	if now.After(renewAfter.Add(grace)) {
		return transport.SiteRenewalOverdue, "renewal was due on " + renewAfter.UTC().Format("2006-01-02") + " and certbot has not renewed it"
	}
	if facts.Hook == "missing" || facts.Hook == "unreadable" {
		return transport.SiteRenewalHookMissing, "the deploy hook that reloads nginx after a renewal is not on this server"
	}
	if facts.Hook == "changed" {
		return transport.SiteRenewalHookChanged, "the deploy hook that reloads nginx after a renewal was changed outside CelikPanel"
	}
	if !strings.EqualFold(conf.Authenticator, "webroot") || !nativeSiteWebrootMatches(conf, facts.ExpectWebroot) ||
		(facts.ExpectServer != "" && !strings.EqualFold(strings.TrimRight(conf.Server, "/"), strings.TrimRight(facts.ExpectServer, "/"))) {
		return transport.SiteRenewalOwnerConfigured, "certbot's renewal configuration was changed outside CelikPanel; it is kept as it is"
	}
	return transport.SiteRenewalScheduled, ""
}

// nativeSiteRenewalWindow is when certbot starts renewing the certificate
// and how long after that a missing renewal counts as overdue. A window as
// long as the certificate's whole life (a short-lived certificate under the
// 30-day default) is read as the last third of its life, as certbot renews
// short-lived certificates.
func nativeSiteRenewalWindow(leaf *x509.Certificate, renewBefore time.Duration) (time.Time, time.Duration) {
	lifetime := leaf.NotAfter.Sub(leaf.NotBefore)
	if lifetime > 0 && renewBefore >= lifetime {
		renewBefore = lifetime / 3
	}
	grace := nativeSiteOverdueGrace
	if lifetime > 0 && grace > lifetime/10 {
		grace = lifetime / 10
	}
	return leaf.NotAfter.Add(-renewBefore), grace
}

func nativeSiteWebrootMatches(conf nativeSiteRenewalConf, expected string) bool {
	if expected == "" {
		return true
	}
	if len(conf.WebrootMap) == 0 && len(conf.WebrootPaths) == 0 {
		return false
	}
	for _, value := range conf.WebrootMap {
		if filepath.Clean(value) != expected {
			return false
		}
	}
	for _, value := range conf.WebrootPaths {
		if filepath.Clean(value) != expected {
			return false
		}
	}
	return true
}

// SiteCertificateRenewalStateV1 reads the operating system's renewal state
// of one native website lineage. Read only: it runs no certbot and changes
// nothing.
func (a *Agent) SiteCertificateRenewalStateV1(req *transport.SiteCertificateRenewalStateRequest, resp *transport.SiteCertificateRenewalStateResponse) error {
	if req == nil {
		resp.State = transport.SiteRenewalUnknown
		resp.Error = "renewal state request is required"
		return nil
	}
	lineage := strings.TrimSpace(req.LineageName)
	resp.LineageName = lineage
	if !transport.NativeSiteLineageBelongsTo(lineage, req.DomainID) {
		resp.State = transport.SiteRenewalUnknown
		resp.Error = "the certificate lineage does not belong to this domain"
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	*resp = nativeSiteRenewalState(ctx, *req)
	return nil
}

func nativeSiteRenewalState(ctx context.Context, req transport.SiteCertificateRenewalStateRequest) transport.SiteCertificateRenewalStateResponse {
	lineage := strings.TrimSpace(req.LineageName)
	resp := transport.SiteCertificateRenewalStateResponse{LineageName: lineage}
	facts := nativeSiteFacts{ExpectServer: req.ACMEServer}
	if facts.ExpectServer == "" {
		facts.ExpectServer = letsEncryptDefaultDirectory
	}
	if root, err := hostingpath.ACMEChallengeRoot(req.SubscriptionID, req.DomainID); err == nil {
		facts.ExpectWebroot = root
	}
	content, err := readBounded(nativeSiteRenewalConfPath(lineage), nativeSiteMaxConfBytes)
	if err != nil {
		facts.ConfErr = err
	} else {
		facts.Conf, facts.ConfErr = parseNativeSiteRenewalConf(content)
	}
	_, leaf, raw, err := readNativeSiteLineage(lineage)
	facts.CertErr = err
	facts.Leaf = leaf
	facts.Hook = nativeSiteHookState()
	facts.Scheduler = readNativeSiteScheduler(ctx)
	if deploy, err := readNativeSiteDeployStatus(lineage); err == nil {
		facts.Deploy = deploy
	}
	if failure, err := readNativeSiteRenewalFailure(lineage); err == nil {
		facts.Failure = failure
	}
	resp.State, resp.Detail = classifyNativeSiteRenewal(facts, nativeSiteNow())
	resp.Hook = facts.Hook
	resp.Scheduler = facts.Scheduler
	resp.LastDeploy = facts.Deploy
	resp.LastFailed = facts.Failure
	if facts.ConfErr == nil {
		resp.AutoRenew = facts.Conf.AutoRenew
		resp.Authenticator = facts.Conf.Authenticator
		resp.Server = facts.Conf.Server
	}
	if leaf != nil {
		resp.NotBefore = leaf.NotBefore.UTC()
		resp.NotAfter = leaf.NotAfter.UTC()
		sum := sha256.Sum256(raw)
		resp.LeafSHA256 = fmt.Sprintf("%x", sum)
		renewBefore := nativeSiteDefaultRenewBefore
		if facts.ConfErr == nil {
			renewBefore = facts.Conf.RenewBefore
		}
		renewAfter, _ := nativeSiteRenewalWindow(leaf, renewBefore)
		resp.RenewAfter = renewAfter.UTC()
	}
	return resp
}

// setNativeSiteAutoRenew writes certbot's own `autorenew` setting into the
// lineage's [renewalparams]. Only that one line changes; the rest of the
// file (the owner's or certbot's) is kept byte for byte.
func setNativeSiteAutoRenew(lineage string, enabled bool) (bool, error) {
	path := nativeSiteRenewalConfPath(lineage)
	info, err := os.Lstat(path)
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, errors.New("the renewal configuration is not a regular file")
	}
	content, err := readBounded(path, nativeSiteMaxConfBytes)
	if err != nil {
		return false, err
	}
	updated, changed, err := withNativeSiteAutoRenew(content, enabled)
	if err != nil || !changed {
		return false, err
	}
	if err := writeCertificateFile(path, updated, info.Mode().Perm()); err != nil {
		return false, err
	}
	return true, nil
}

func withNativeSiteAutoRenew(content []byte, enabled bool) ([]byte, bool, error) {
	value := "True"
	if !enabled {
		value = "False"
	}
	lines := strings.SplitAfter(string(content), "\n")
	section := ""
	paramsAt := -1
	for i, raw := range lines {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "[") {
			section = strings.ToLower(strings.Trim(line, "[] "))
			if section == "renewalparams" {
				paramsAt = i
			}
			continue
		}
		if section != "renewalparams" {
			continue
		}
		key, current, ok := strings.Cut(line, "=")
		if !ok || strings.ToLower(strings.TrimSpace(key)) != "autorenew" {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(current), value) {
			return content, false, nil
		}
		lines[i] = "autorenew = " + value + "\n"
		return []byte(strings.Join(lines, "")), true, nil
	}
	if paramsAt < 0 {
		return nil, false, errors.New("the renewal configuration has no [renewalparams] section")
	}
	if enabled {
		// No line means certbot renews it already.
		return content, false, nil
	}
	insert := "autorenew = False\n"
	if !strings.HasSuffix(lines[paramsAt], "\n") {
		lines[paramsAt] += "\n"
	}
	out := append(append(append([]string{}, lines[:paramsAt+1]...), insert), lines[paramsAt+1:]...)
	return []byte(strings.Join(out, "")), true, nil
}

// SetSiteCertificateAutoRenewV1 switches certbot's `autorenew` of one native
// lineage (the page's "renew automatically").
func (a *Agent) SetSiteCertificateAutoRenewV1(req *transport.SetSiteCertificateAutoRenewRequest, resp *transport.SetSiteCertificateAutoRenewResponse) error {
	if req == nil {
		resp.Error = "renewal setting request is required"
		return nil
	}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "changing a certificate's renewal setting"); err != nil {
		resp.Error = err.Error()
		return nil
	}
	if !transport.NativeSiteLineageBelongsTo(req.LineageName, req.DomainID) {
		resp.Error = "the certificate lineage does not belong to this domain"
		return nil
	}
	if !acquireSiteCertbot() {
		resp.Error = transport.SiteCertificateBusy
		return nil
	}
	defer releaseSiteCertbot()
	changed, err := setNativeSiteAutoRenew(req.LineageName, req.AutoRenew)
	if err != nil {
		resp.Error = "certbot's renewal configuration could not be changed: " + boundedNativeDetail(err.Error())
		return nil
	}
	resp.Changed = changed
	return nil
}

// SnapshotSiteCertificateV1 copies the certificate a native lineage's live
// files name now into the immutable snapshot store (content-addressed, so
// the same certificate always gives the same paths). The mail SNI map names
// only immutable snapshots.
func (a *Agent) SnapshotSiteCertificateV1(req *transport.SnapshotSiteCertificateRequest, resp *transport.SnapshotSiteCertificateResponse) error {
	if req == nil {
		resp.Error = "certificate snapshot request is required"
		return nil
	}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "copying a site certificate for mail"); err != nil {
		resp.Error = err.Error()
		return nil
	}
	domain, err := canonicalCertificateDomain(req.Domain)
	if err != nil {
		resp.Error = err.Error()
		return nil
	}
	if !transport.NativeSiteLineageBelongsTo(req.LineageName, req.DomainID) {
		resp.Error = "the certificate lineage does not belong to this domain"
		return nil
	}
	if _, _, _, err := readNativeSiteLineage(req.LineageName); err != nil {
		resp.Error = err.Error()
		return nil
	}
	paths, _, err := snapshotCertbotCertificateFromLineage(req.LineageName, domain, nativeSiteStorage())
	if err != nil {
		resp.Error = err.Error()
		return nil
	}
	resp.CertPath, resp.KeyPath, resp.ChainPath = paths.Fullchain, paths.Key, paths.Chain
	return nil
}

// issueNativeSiteCertificate is IssueLetsEncryptCertificate's certbot run
// and verification, after the request was checked and the lock taken.
func issueNativeSiteCertificate(req IssueLetsEncryptRequest, webroot string, resp *IssueLetsEncryptResponse) {
	lineage, err := newNativeSiteLineage(req.DomainID)
	if err != nil {
		resp.Success = false
		resp.Error = err.Error()
		return
	}
	// The exact lineage identity is returned even when certbot fails, so the
	// Panel can remove any partial lineage without guessing a name.
	resp.LineageName = lineage

	// The hook first: the very first renewal must already reload nginx.
	hook, hookErr := ensureNativeSiteDeployHook()

	configFile, cleanup, err := writeNativeSiteEABConfig(req)
	if err != nil {
		resp.Success = false
		resp.Error = err.Error()
		return
	}
	defer cleanup()

	ctx, cancel := certbotRenewContext(context.Background())
	defer cancel()
	output, err := nativeSiteCertbotRun(ctx, buildNativeSiteIssueArgs(req, webroot, lineage, configFile)...)
	if err != nil {
		resp.Success = false
		resp.FailureDetail = certbotFailureLine(string(output))
		if ctx.Err() == context.DeadlineExceeded {
			resp.Failure = transport.CertificateFailureTimeout
			resp.Error = fmt.Sprintf("certbot issue timed out after %s\nOutput: %s", certbotRenewTimeout, string(output))
			return
		}
		resp.Failure = certbotFailureKind(string(output))
		resp.Error = fmt.Sprintf("certbot failed: %v\nOutput: %s", err, string(output))
		return
	}

	paths, cert, _, err := readNativeSiteLineage(lineage)
	if err != nil {
		resp.Success = false
		resp.Error = fmt.Sprintf("certbot reported success, but its lineage could not be used: %v", err)
		return
	}
	if !req.AutoRenew {
		if _, err := setNativeSiteAutoRenew(lineage, false); err != nil {
			resp.RenewalWarning = joinWarning(resp.RenewalWarning,
				"automatic renewal could not be switched off in certbot's configuration ("+boundedNativeDetail(err.Error())+")")
		}
	}
	switch {
	case hookErr != nil:
		resp.RenewalWarning = joinWarning(resp.RenewalWarning,
			"the deploy hook that reloads nginx after a renewal could not be written ("+boundedNativeDetail(hookErr.Error())+"); until it is, run `systemctl reload nginx` after a renewal")
	case hook == "changed":
		resp.RenewalWarning = joinWarning(resp.RenewalWarning,
			"the deploy hook "+nativeSiteHookPath+" was changed outside CelikPanel and is kept as it is")
	}
	if req.AutoRenew {
		renewCtx, renewCancel := context.WithTimeout(context.Background(), 30*time.Second)
		if err := nativeSiteEnableTimer(renewCtx); err != nil {
			resp.RenewalWarning = joinWarning(resp.RenewalWarning,
				"the operating system's certbot timer could not be enabled ("+boundedNativeDetail(err.Error())+"); the server administrator runs `systemctl enable --now certbot.timer`")
		}
		renewCancel()
	}

	resp.CertPath = paths.Fullchain
	resp.KeyPath = paths.Key
	resp.ChainPath = paths.Chain
	resp.ExpiresAt = cert.NotAfter
	resp.DNSNames = append([]string(nil), cert.DNSNames...)
	resp.Success = true
}
