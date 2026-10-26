package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Webmail on its own address (alpha.110; owner 2026-10-26: "webmail on its
// own address, served by the server's own web server"). Roundcube no longer
// runs on the panel's origin, where a script running in it (its past XSS
// CVEs 2023-5631, 2024-37383, 2024-42009) could call the panel's API with the
// administrator's session. The server's own nginx serves it on a name the
// owner chooses, with a certificate for that name from certbot (HTTP-01 on
// port 80 through nginx, renewed by the operating system's certbot timer),
// so webmail keeps answering when the panel is stopped or removed (D-022).
//
// One native nginx file per name, in CelikPanel's own form:
// /etc/nginx/conf.d/celikpanel-webmail-origin-<name>.conf. Port 80 answers
// certbot's check and redirects everything else to HTTPS; port 443 serves
// Roundcube's public tree through the PHP-FPM socket webmail is bound to
// (alpha.104), with Roundcube's own security policy for its own origin
// (frame-ancestors 'self', HSTS, nosniff; its inline start-up script is
// allowed there because nothing of the panel lives on that origin). A file
// changed by the owner is never overwritten or removed (D-022).
//
// Web postası kendi adında: sunucunun nginx'i sunar; sertifika certbot'tan,
// yenileme işletim sisteminin certbot zamanlayıcısıyla. Sahibin değiştirdiği
// dosyaya dokunulmaz.

const (
	webmailAddressConfPrefix = "celikpanel-webmail-origin-"
	webmailAddressLineage    = "celikpanel-webmail-"
	// A certificate that still has this long is used as it is.
	webmailAddressCertificateReuse = 30 * 24 * time.Hour
	webmailAddressProofSettle      = 20 * time.Second
	webmailAddressProofBodyLimit   = 1 << 20

	errCodeWebmailAddressInvalid        = "ADDRESS_INVALID"
	errCodeWebmailAddressNotInstalled   = "ROUNDCUBE_NOT_INSTALLED"
	errCodeWebmailAddressNginx          = "NGINX_UNAVAILABLE"
	errCodeWebmailAddressCertbot        = "CERTBOT_MISSING"
	errCodeWebmailAddressConflict       = "NAME_IN_USE"
	errCodeWebmailAddressOwnerEdited    = "CONFIG_OWNER_EDITED"
	errCodeWebmailAddressRejected       = "NGINX_REJECTED"
	errCodeWebmailAddressCertificate    = "CERTIFICATE_FAILED"
	errCodeWebmailAddressProof          = "PROOF_FAILED"
	errCodeWebmailAddressRollbackFailed = "ROLLBACK_FAILED"
	errCodeWebmailAddressPHP            = "PHP_UNAVAILABLE"
)

var (
	webmailAddressConfDir        = "/etc/nginx/conf.d"
	webmailAddressLetsEncryptDir = "/etc/letsencrypt"
)

// webmailAddressOps are the host operations, replaced in tests.
type webmailAddressOps struct {
	run                webmailCommandRunner
	nginxDump          func(context.Context) (string, error)
	nginxActive        func(context.Context) error
	certbotPresent     func() error
	challengeRoot      func() (string, error)
	issue              func(ctx context.Context, host, email, lineage, webroot string) (string, error)
	ensureRenewal      func(context.Context) error
	deleteCertificate  func(ctx context.Context, lineage string) error
	roundcubeInstalled func() (bool, error)
	fpmSocket          func() (string, error)
	ensureCheck        func() error
	prove              func(ctx context.Context, host, version string) error
	roundcubeSettings  func(enabled bool) (string, error)
	now                func() time.Time
	settle             time.Duration
}

func hostWebmailAddressOps() webmailAddressOps {
	return webmailAddressOps{
		run: runServiceMutationCombinedOutput,
		nginxDump: func(ctx context.Context) (string, error) {
			out, err := runServiceMutationCombinedOutput(ctx, "nginx", "-T")
			if err != nil {
				return "", fmt.Errorf("nginx -T: %s", commandFailureDetail("nginx -T", out, err))
			}
			return string(out), nil
		},
		nginxActive: func(ctx context.Context) error {
			if _, err := exec.LookPath("nginx"); err != nil {
				return errors.New("nginx is not installed")
			}
			if out, err := runServiceMutationCombinedOutput(ctx, "systemctl", "is-active", "--quiet", "nginx"); err != nil {
				return fmt.Errorf("nginx is installed but not running (%s); start it in Components and try again", commandFailureDetail("systemctl is-active nginx", out, err))
			}
			return nil
		},
		certbotPresent: func() error {
			if _, err := exec.LookPath("certbot"); err != nil {
				return errors.New("certbot is not installed; install it in Components (Certbot) and try again")
			}
			return nil
		},
		challengeRoot: preparePanelACMEChallengeRoot,
		issue:         issueWebmailAddressCertificate,
		ensureRenewal: ensurePanelCertRenewalScheduler,
		deleteCertificate: func(ctx context.Context, lineage string) error {
			out, err := runServiceMutationCombinedOutput(ctx, "certbot", "delete", "--non-interactive", "--cert-name", lineage)
			if err != nil {
				return fmt.Errorf("certbot delete: %s", commandFailureDetail("certbot delete", out, err))
			}
			return nil
		},
		roundcubeInstalled: roundcubeInstallState,
		fpmSocket: func() (string, error) {
			return phpToolSocket(phpToolWebmail, phpToolStateFor(phpToolWebmail).Socket)
		},
		ensureCheck:       ensurePHPToolCheckScript,
		prove:             proveWebmailAddress,
		roundcubeSettings: setRoundcubeAddressSettings,
		now:               time.Now,
		settle:            webmailAddressProofSettle,
	}
}

// canonicalWebmailAddress is a lowercase host name with at least two labels.
func canonicalWebmailAddress(raw string) (string, error) {
	host, err := canonicalCertificateDomain(raw)
	if err != nil || !strings.Contains(host, ".") || net.ParseIP(host) != nil || !validPanelCertDomain.MatchString(host) {
		return "", fmt.Errorf("%q is not a host name such as mail.example.com", strings.TrimSpace(raw))
	}
	return host, nil
}

func webmailAddressConfPath(host string) string {
	return filepath.Join(webmailAddressConfDir, webmailAddressConfPrefix+host+".conf")
}

func webmailAddressLineageName(host string) string {
	sum := sha256.Sum256([]byte(host))
	return fmt.Sprintf("%s%x", webmailAddressLineage, sum[:12])
}

func webmailAddressLiveDir(lineage string) string {
	return filepath.Join(webmailAddressLetsEncryptDir, "live", lineage)
}

// renderWebmailAddressHTTP is port 80: certbot's HTTP-01 check (the panel's
// own challenge root, root-owned) and a redirect to HTTPS for everything else.
func renderWebmailAddressHTTP(host, challengeRoot string) string {
	return fmt.Sprintf(`server {
    listen 80;
    listen [::]:80;
    server_name %s;

    location ^~ /.well-known/acme-challenge/ {
        root %s;
        default_type text/plain;
        try_files $uri =404;
    }

    location / {
        return 301 https://$host$request_uri;
    }
}
`, host, challengeRoot)
}

func webmailAddressHeader(host, lineage string) string {
	return fmt.Sprintf(`# Managed by CelikPanel — webmail (Roundcube) on its own address %s.
# Served by this server's nginx and PHP-FPM; the panel is not involved, so
# webmail answers while the panel is stopped. The certificate is certbot's
# lineage %s, renewed by the operating system's certbot timer.
# Changing this file by hand makes CelikPanel leave it as it is.
`, host, lineage)
}

// renderWebmailAddressPending is the file before the certificate exists:
// port 80 only, so certbot's check reaches the challenge root.
func renderWebmailAddressPending(host, challengeRoot string) string {
	return webmailAddressHeader(host, webmailAddressLineageName(host)) + renderWebmailAddressHTTP(host, challengeRoot)
}

// renderWebmailAddressConfig is the complete file. withHTTP is false when
// another CelikPanel server block already answers port 80 for the name (the
// mail host certificate's challenge server, with the same challenge root).
func renderWebmailAddressConfig(host, challengeRoot, roundcubeRoot, fpmSocket string, withHTTP bool) string {
	lineage := webmailAddressLineageName(host)
	live := webmailAddressLiveDir(lineage)
	var b strings.Builder
	b.WriteString(webmailAddressHeader(host, lineage))
	if withHTTP {
		b.WriteString(renderWebmailAddressHTTP(host, challengeRoot))
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, `server {
    listen 443 ssl;
    listen [::]:443 ssl;
    server_name %s;

    ssl_certificate %s/fullchain.pem;
    ssl_certificate_key %s/privkey.pem;
    ssl_session_cache shared:SSL:10m;
    ssl_session_timeout 10m;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;
    ssl_prefer_server_ciphers on;

    # Roundcube on its own origin: nothing of the panel lives here, so its
    # inline start-up script may run; it may only be framed by itself.
    add_header Strict-Transport-Security "max-age=31536000" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header Referrer-Policy "same-origin" always;
    add_header Content-Security-Policy "frame-ancestors 'self'; object-src 'none'; base-uri 'self'; form-action 'self'" always;

    root %s;
    index index.php;
    client_max_body_size 25m;

    location ~ /\. {
        deny all;
    }

    location / {
        try_files $uri $uri/ /index.php$is_args$args;
    }

    location ~ \.php$ {
        try_files $uri =404;
        include fastcgi_params;
        fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;
        fastcgi_param HTTPS on;
        fastcgi_pass unix:%s;
    }

    # CelikPanel's PHP check for webmail, asked only from this server.
    location = %s%s {
        allow 127.0.0.1;
        allow ::1;
        deny all;
        include fastcgi_params;
        fastcgi_param SCRIPT_FILENAME %s;
        fastcgi_param CELIKPANEL_PHP_EXTENSIONS "%s";
        fastcgi_pass unix:%s;
    }
}
`, host, live, live, roundcubeRoot, fpmSocket, phpToolCheckPrefix, phpToolWebmail,
		phpToolCheckScriptPath, strings.Join(phpToolExtensions[phpToolWebmail], ","), fpmSocket)
	return b.String()
}

var (
	webmailAddressRootLine      = regexp.MustCompile(`(?m)^    root (/[^;\s]+);$`)
	webmailAddressChallengeLine = regexp.MustCompile(`(?m)^        root (/[^;\s]+);$`)
	webmailAddressFPMLine       = regexp.MustCompile(`(?m)^        fastcgi_pass unix:(/[^;\s]+);$`)
)

// webmailAddressForm reads a file back into its parameters when it is
// exactly one of CelikPanel's forms for host.
type webmailAddressForm struct {
	pending   bool
	withHTTP  bool
	challenge string
	root      string
	fpm       string
}

func parseWebmailAddressConfig(host, content string) (webmailAddressForm, bool) {
	var form webmailAddressForm
	if m := webmailAddressChallengeLine.FindStringSubmatch(content); m != nil {
		form.challenge = m[1]
	}
	if form.challenge != "" && content == renderWebmailAddressPending(host, form.challenge) {
		form.pending = true
		return form, true
	}
	if m := webmailAddressRootLine.FindStringSubmatch(content); m != nil {
		form.root = m[1]
	}
	if m := webmailAddressFPMLine.FindStringSubmatch(content); m != nil {
		form.fpm = m[1]
	}
	if form.root == "" || form.fpm == "" {
		return form, false
	}
	if form.challenge != "" && content == renderWebmailAddressConfig(host, form.challenge, form.root, form.fpm, true) {
		form.withHTTP = true
		return form, true
	}
	if content == renderWebmailAddressConfig(host, "", form.root, form.fpm, false) {
		return form, true
	}
	return form, false
}

// webmailAddressHosts lists the names that have an address file.
func webmailAddressHosts() []string {
	matches, _ := filepath.Glob(filepath.Join(webmailAddressConfDir, webmailAddressConfPrefix+"*.conf"))
	var hosts []string
	for _, match := range matches {
		name := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(match), webmailAddressConfPrefix), ".conf")
		if host, err := canonicalWebmailAddress(name); err == nil && host == name {
			hosts = append(hosts, host)
		}
	}
	sort.Strings(hosts)
	return hosts
}

// webmailAddressConflict reads `nginx -T` for another server block that
// answers for host. https: some block outside CelikPanel's file listens on
// 443 (or with ssl) for the name; http: one listens on port 80 for it;
// panelACME: that port-80 block is CelikPanel's own challenge server (the
// same challenge root), which this address may share.
type webmailAddressNameUse struct {
	httpsFile string
	httpFile  string
	panelACME bool
}

var nginxConfigFileHeader = regexp.MustCompile(`(?m)^# configuration file (\S+):$`)

func webmailAddressNameUses(dump, host, ownPath string) webmailAddressNameUse {
	var use webmailAddressNameUse
	otherHTTP := ""
	indexes := nginxConfigFileHeader.FindAllStringSubmatchIndex(dump, -1)
	for i, index := range indexes {
		file := dump[index[2]:index[3]]
		end := len(dump)
		if i+1 < len(indexes) {
			end = indexes[i+1][0]
		}
		if filepath.Clean(file) == filepath.Clean(ownPath) {
			continue
		}
		for _, block := range services.NginxServerBlocks(dump[index[1]:end]) {
			named := false
			for _, name := range block.Names {
				if strings.EqualFold(name, host) {
					named = true
				}
			}
			if !named {
				continue
			}
			https, http := webmailAddressListenKinds(block.Listens)
			if https && use.httpsFile == "" {
				use.httpsFile = file
			}
			if !http {
				continue
			}
			if strings.HasPrefix(filepath.Base(file), panelACMEVhostPrefix) {
				use.panelACME = true
				if use.httpFile == "" {
					use.httpFile = file
				}
			} else if otherHTTP == "" {
				otherHTTP = file
			}
		}
	}
	if otherHTTP != "" {
		use.httpFile, use.panelACME = otherHTTP, false
	}
	return use
}

// webmailAddressListenKinds says whether a server block's listen directives
// answer HTTPS (port 443 or ssl) and plain HTTP on port 80 (a block without
// listen answers port 80).
func webmailAddressListenKinds(listens [][]string) (https, http bool) {
	if len(listens) == 0 {
		return false, true
	}
	for _, listen := range listens {
		if len(listen) == 0 || strings.HasPrefix(listen[0], "unix:") {
			continue
		}
		value, port := listen[0], "80"
		switch {
		case strings.HasPrefix(value, "["):
			if end := strings.IndexByte(value, ']'); end >= 0 && strings.HasPrefix(value[end+1:], ":") {
				port = value[end+2:]
			}
		case isDigits(value):
			port = value
		case strings.Contains(value, ":"):
			port = value[strings.LastIndexByte(value, ':')+1:]
		}
		ssl := false
		for _, argument := range listen[1:] {
			if argument == "ssl" {
				ssl = true
			}
		}
		switch {
		case ssl || port == "443":
			https = true
		case port == "80":
			http = true
		}
	}
	return https, http
}

// readWebmailAddressCertificate reads the lineage's live certificate.
func readWebmailAddressCertificate(lineage string) (*x509.Certificate, []byte, error) {
	raw, err := os.ReadFile(filepath.Join(webmailAddressLiveDir(lineage), "fullchain.pem"))
	if err != nil {
		return nil, nil, err
	}
	block, _ := pem.Decode(raw)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, nil, errors.New("the certificate file holds no certificate")
	}
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, nil, err
	}
	if _, err := os.Stat(filepath.Join(webmailAddressLiveDir(lineage), "privkey.pem")); err != nil {
		return nil, nil, fmt.Errorf("the certificate's key is missing: %w", err)
	}
	return leaf, block.Bytes, nil
}

// issueWebmailAddressCertificate asks certbot for the name's certificate
// through the challenge root nginx serves on port 80. The deploy hook stored
// in certbot's own renewal configuration reloads nginx after every renewal,
// without CelikPanel.
func issueWebmailAddressCertificate(ctx context.Context, host, email, lineage, webroot string) (string, error) {
	args := []string{"certonly", "--webroot", "--webroot-path", webroot, "--preferred-challenges", "http",
		"--cert-name", lineage, "-d", host, "--agree-tos", "--non-interactive", "--keep-until-expiring",
		"--deploy-hook", "systemctl reload nginx"}
	if email != "" {
		args = append(args, "--email", email)
	} else {
		args = append(args, "--register-unsafely-without-email")
	}
	out, err := panelCertRunMutationCommand(ctx, panelCertIssueTimeout, "certbot", args...)
	if err != nil {
		detail := certbotFailureLine(string(out))
		if detail == "" {
			detail = certbotFirstError(string(out))
		}
		return detail, fmt.Errorf("certbot could not obtain a certificate for %s: %w", host, err)
	}
	return "", nil
}

func webmailAddressFail(resp *transport.WebmailAddressResponse, code, format string, args ...any) {
	resp.ErrorCode = code
	resp.Error = fmt.Sprintf(format, args...)
}

// enableWebmailAddress is the whole enable: checks, certificate, the nginx
// file, Roundcube's lines, proof; the file comes back as it was on any
// failure after it changed (the certificate stays).
func enableWebmailAddress(ctx context.Context, ops webmailAddressOps, rawHost, email, rawReplace string) (resp transport.WebmailAddressResponse) {
	host, err := canonicalWebmailAddress(rawHost)
	if err != nil {
		webmailAddressFail(&resp, errCodeWebmailAddressInvalid, "%v", err)
		return
	}
	resp.Host = host
	replace := ""
	if strings.TrimSpace(rawReplace) != "" {
		if replace, err = canonicalWebmailAddress(rawReplace); err != nil {
			webmailAddressFail(&resp, errCodeWebmailAddressInvalid, "the previous address: %v", err)
			return
		}
	}
	lineage := webmailAddressLineageName(host)
	resp.Lineage = lineage
	installed, err := ops.roundcubeInstalled()
	if err != nil || !installed {
		webmailAddressFail(&resp, errCodeWebmailAddressNotInstalled, "Roundcube is not installed where CelikPanel installs it; install webmail in Components first")
		return
	}
	if err := ops.nginxActive(ctx); err != nil {
		webmailAddressFail(&resp, errCodeWebmailAddressNginx, "%v", err)
		return
	}
	path := webmailAddressConfPath(host)
	snapshot, err := snapshotMailFile(path)
	if err != nil {
		webmailAddressFail(&resp, errCodeWebmailAddressRejected, "the current file %s could not be read: %v", path, err)
		return
	}
	if snapshot.exists {
		if _, managed := parseWebmailAddressConfig(host, string(snapshot.content)); !managed {
			webmailAddressFail(&resp, errCodeWebmailAddressOwnerEdited, "%s was changed outside CelikPanel, so it is left as it is; the server owner removes or restores it, then tries again", path)
			return
		}
	}
	dump, err := ops.nginxDump(ctx)
	if err != nil {
		webmailAddressFail(&resp, errCodeWebmailAddressRejected, "the web server's configuration could not be read (%v); nothing was changed", err)
		return
	}
	use := webmailAddressNameUses(dump, host, path)
	if use.httpsFile != "" {
		webmailAddressFail(&resp, errCodeWebmailAddressConflict, "%s already answers HTTPS for %s, so webmail cannot have that name; choose another name or remove that server block. Nothing was changed", use.httpsFile, host)
		return
	}
	if use.httpFile != "" && !use.panelACME {
		webmailAddressFail(&resp, errCodeWebmailAddressConflict, "%s already answers port 80 for %s, so certbot's check would not reach webmail; choose another name or remove that server block. Nothing was changed", use.httpFile, host)
		return
	}
	withHTTP := !use.panelACME
	fpm, err := ops.fpmSocket()
	if err != nil {
		webmailAddressFail(&resp, errCodeWebmailAddressPHP, "webmail's PHP-FPM could not be found (%v); install or start PHP-FPM in Components and try again", err)
		return
	}
	version := resolvedSocketVersion(fpm)
	if err := ops.ensureCheck(); err != nil {
		webmailAddressFail(&resp, errCodeWebmailAddressPHP, "%v", err)
		return
	}
	challenge, err := ops.challengeRoot()
	if err != nil {
		webmailAddressFail(&resp, errCodeWebmailAddressRejected, "the certificate check's folder could not be prepared: %v; nothing was changed", err)
		return
	}

	// restore puts the file back as it was and says whether that worked.
	restore := func(cause string) {
		var restoreErr error
		if snapshot.exists {
			restoreErr = applyWebmailNginxMutation(ctx, path, snapshot.content, true, ops.run)
		} else {
			restoreErr = applyWebmailNginxMutation(ctx, path, nil, false, ops.run)
		}
		if restoreErr != nil {
			resp.ErrorCode = errCodeWebmailAddressRollbackFailed
			resp.Error = fmt.Sprintf("%s; the previous web server file could not be put back (%v); the server owner checks %s and runs nginx -t", cause, restoreErr, path)
			return
		}
		resp.RolledBack = true
	}

	// 1. The certificate: the lineage's own when it still has 30 days,
	// otherwise certbot's (port 80 must answer the check first).
	leaf, _, certErr := readWebmailAddressCertificate(lineage)
	usable := certErr == nil && leaf.VerifyHostname(host) == nil && leaf.NotAfter.After(ops.now().Add(webmailAddressCertificateReuse))
	if !usable {
		if err := ops.certbotPresent(); err != nil {
			webmailAddressFail(&resp, errCodeWebmailAddressCertbot, "%v", err)
			return
		}
		if withHTTP && !(snapshot.exists && strings.Contains(string(snapshot.content), "listen 80;")) {
			if err := applyWebmailNginxMutation(ctx, path, []byte(renderWebmailAddressPending(host, challenge)), true, ops.run); err != nil {
				webmailAddressFail(&resp, errCodeWebmailAddressRejected, "nginx did not accept the certificate check's server for %s (%v); the previous file was put back", host, err)
				resp.RolledBack = true
				return
			}
		}
		detail, err := ops.issue(ctx, host, email, lineage, challenge)
		if err != nil {
			reason := err.Error()
			if detail != "" {
				reason += ": " + detail
			}
			webmailAddressFail(&resp, errCodeWebmailAddressCertificate, "%s. Check that %s points to this server and that port 80 is reachable from the internet", reason, host)
			restore(resp.Error)
			return
		}
		resp.CertificateIssued = true
		leaf, _, certErr = readWebmailAddressCertificate(lineage)
		if certErr != nil || leaf.VerifyHostname(host) != nil {
			cause := certErr
			if cause == nil {
				cause = fmt.Errorf("it is not issued for %s", host)
			}
			webmailAddressFail(&resp, errCodeWebmailAddressCertificate, "certbot answered, but the certificate could not be used (%v)", cause)
			restore(resp.Error)
			return
		}
		if err := ops.ensureRenewal(ctx); err != nil {
			resp.Warning = fmt.Sprintf("the operating system's certbot timer could not be enabled (%v); the server owner enables certbot.timer so the certificate renews", err)
		}
	}
	resp.CertificateExpires = leaf.NotAfter.UTC().Format(time.RFC3339)

	// 2. The complete file, validated before nginx reloads it.
	content := renderWebmailAddressConfig(host, challenge, roundcubeRootDir(), fpm, withHTTP)
	if err := applyWebmailNginxMutation(ctx, path, []byte(content), true, ops.run); err != nil {
		webmailAddressFail(&resp, errCodeWebmailAddressRejected, "nginx did not accept webmail's server for %s (%v)", host, err)
		restore(resp.Error)
		return
	}
	// 3. Roundcube's own lines for its own origin (only in CelikPanel's
	// form of its configuration; an owner-edited one is left as it is).
	settings, settingsErr := ops.roundcubeSettings(true)
	resp.RoundcubeSettings = settings
	if settingsErr != nil {
		resp.RoundcubeSettings = "unchanged"
		resp.Warning = joinWarning(resp.Warning, fmt.Sprintf("Roundcube's configuration was not changed (%v)", settingsErr))
	}
	// 4. Proof over HTTPS on the name, asked on this server.
	if err := retryWebmailAddressProof(ctx, ops, host, version); err != nil {
		if settings == "applied" && len(webmailAddressHostsExcept(host)) == 0 && !snapshot.exists {
			_, _ = ops.roundcubeSettings(false)
		}
		webmailAddressFail(&resp, errCodeWebmailAddressProof, "webmail did not answer as itself on https://%s/ (%v)", host, err)
		restore(resp.Error)
		return
	}
	resp.Enabled, resp.Proved = true, true
	// 5. Changing the address: the previous one goes only now.
	if replace != "" && replace != host {
		removed := removeWebmailAddress(ctx, ops, replace, false)
		if removed.Error != "" {
			resp.Warning = joinWarning(resp.Warning, fmt.Sprintf("the previous address %s still answers (%s); remove it from the webmail address card", replace, removed.Error))
		}
	}
	return
}

func joinWarning(current, next string) string {
	if current == "" {
		return next
	}
	return current + "; " + next
}

func webmailAddressHostsExcept(host string) []string {
	var others []string
	for _, h := range webmailAddressHosts() {
		if h != host {
			others = append(others, h)
		}
	}
	return others
}

// retryWebmailAddressProof repeats the proof while nginx's reloaded
// workers settle, bounded.
func retryWebmailAddressProof(ctx context.Context, ops webmailAddressOps, host, version string) error {
	deadline := ops.now().Add(ops.settle)
	for {
		err := ops.prove(ctx, host, version)
		if err == nil || !ops.now().Before(deadline) || ctx.Err() != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return err
		case <-time.After(time.Second):
		}
	}
}

// removeWebmailAddress removes the name's nginx file (validated, reloaded,
// put back on refusal); the certificate stays unless asked.
func removeWebmailAddress(ctx context.Context, ops webmailAddressOps, rawHost string, removeCertificate bool) (resp transport.WebmailAddressRemoveResponse) {
	host, err := canonicalWebmailAddress(rawHost)
	if err != nil {
		resp.ErrorCode, resp.Error = errCodeWebmailAddressInvalid, err.Error()
		return
	}
	path := webmailAddressConfPath(host)
	snapshot, err := snapshotMailFile(path)
	if err != nil {
		resp.ErrorCode, resp.Error = errCodeWebmailAddressRejected, fmt.Sprintf("%s could not be read: %v", path, err)
		return
	}
	if snapshot.exists {
		if _, managed := parseWebmailAddressConfig(host, string(snapshot.content)); !managed {
			resp.ErrorCode = errCodeWebmailAddressOwnerEdited
			resp.Error = fmt.Sprintf("%s was changed outside CelikPanel, so it is left as it is; the server owner removes it by hand", path)
			return
		}
		if err := applyWebmailNginxMutation(ctx, path, nil, false, ops.run); err != nil {
			resp.ErrorCode, resp.Error = errCodeWebmailAddressRejected, fmt.Sprintf("nginx did not accept the change (%v); the file was put back", err)
			return
		}
	}
	resp.Removed = true
	if len(webmailAddressHosts()) == 0 && ops.roundcubeSettings != nil {
		if _, err := ops.roundcubeSettings(false); err != nil {
			resp.Warning = fmt.Sprintf("CelikPanel's lines for the address stay in Roundcube's configuration (%v)", err)
		}
	}
	if removeCertificate {
		if err := ops.deleteCertificate(ctx, webmailAddressLineageName(host)); err != nil {
			resp.Warning = joinWarning(resp.Warning, fmt.Sprintf("the certificate for %s was kept (%v)", host, err))
		} else {
			resp.CertificateRemoved = true
		}
	}
	return
}

// webmailAddressStatus reads one address, read only.
func webmailAddressStatus(ctx context.Context, ops webmailAddressOps, rawHost string, prove bool) (resp transport.WebmailAddressStatusResponse) {
	resp.Hosts = webmailAddressHosts()
	if strings.TrimSpace(rawHost) == "" {
		return
	}
	host, err := canonicalWebmailAddress(rawHost)
	if err != nil {
		resp.Error = err.Error()
		return
	}
	path := webmailAddressConfPath(host)
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		resp.Error = err.Error()
		return
	}
	resp.Present = true
	form, managed := parseWebmailAddressConfig(host, string(content))
	resp.Managed = managed
	if leaf, _, err := readWebmailAddressCertificate(webmailAddressLineageName(host)); err == nil {
		resp.CertificateExpires = leaf.NotAfter.UTC().Format(time.RFC3339)
	}
	if prove && !form.pending {
		if err := ops.prove(ctx, host, resolvedSocketVersion(form.fpm)); err != nil {
			resp.ProofError = err.Error()
		} else {
			resp.Proved = true
		}
	}
	return
}

// refreshWebmailAddresses follows webmail's PHP-FPM socket in every address
// file in CelikPanel's form after webmail moved to another PHP version
// (alpha.104 binding); a file the owner changed is left as it is. It answers
// a sentence for the screen when an address could not follow.
func refreshWebmailAddresses(ctx context.Context, run webmailCommandRunner, socket string) string {
	var problems []string
	for _, host := range webmailAddressHosts() {
		path := webmailAddressConfPath(host)
		content, err := os.ReadFile(path)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", host, err))
			continue
		}
		form, managed := parseWebmailAddressConfig(host, string(content))
		if !managed || form.pending || form.fpm == socket {
			continue
		}
		next := renderWebmailAddressConfig(host, form.challenge, form.root, socket, form.withHTTP)
		if err := applyWebmailNginxMutation(ctx, path, []byte(next), true, run); err != nil {
			problems = append(problems, fmt.Sprintf("%s still uses %s (%v)", host, form.fpm, err))
		}
	}
	if len(problems) == 0 {
		return ""
	}
	return "webmail's own address did not follow the new PHP version: " + strings.Join(problems, "; ")
}

// removeAllWebmailAddresses removes the address files in CelikPanel's form
// when Roundcube itself is removed (the certificates stay).
func removeAllWebmailAddresses(ctx context.Context, run webmailCommandRunner) error {
	var errs []error
	for _, host := range webmailAddressHosts() {
		path := webmailAddressConfPath(host)
		content, err := os.ReadFile(path)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if _, managed := parseWebmailAddressConfig(host, string(content)); !managed {
			continue
		}
		if err := applyWebmailNginxMutation(ctx, path, nil, false, run); err != nil {
			errs = append(errs, fmt.Errorf("remove webmail's address %s: %w", host, err))
		}
	}
	return errors.Join(errs...)
}

// proveWebmailAddress asks webmail over HTTPS on its name at this server's
// own 127.0.0.1:443, with the name as SNI and Host: the certificate nginx
// serves must be the lineage's and valid for the name; then CelikPanel's PHP
// check (webmail's PHP version and extensions), the login page with its
// security policy, and its stylesheet as text/css.
func proveWebmailAddress(ctx context.Context, host, version string) error {
	_, want, err := readWebmailAddressCertificate(webmailAddressLineageName(host))
	if err != nil {
		return fmt.Errorf("the certificate for %s could not be read: %w", host, err)
	}
	return proveWebmailAddressAt(ctx, "127.0.0.1:443", host, version, want)
}

func proveWebmailAddressAt(ctx context.Context, address, host, version string, wantLeaf []byte) error {
	probeCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	client := &http.Client{
		Timeout: 20 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: &http.Transport{
			Proxy: nil,
			DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
				var dialer net.Dialer
				return dialer.DialContext(ctx, "tcp", address)
			},
			TLSClientConfig: &tls.Config{
				ServerName: host,
				MinVersion: tls.VersionTLS12,
				// The chain is the CA's business; the proof is that nginx
				// serves exactly the lineage's certificate for this name.
				InsecureSkipVerify: true,
				VerifyConnection: func(state tls.ConnectionState) error {
					if len(state.PeerCertificates) == 0 {
						return errors.New("no certificate was served")
					}
					leaf := state.PeerCertificates[0]
					if wantLeaf != nil && !bytes.Equal(leaf.Raw, wantLeaf) {
						return fmt.Errorf("nginx serves another certificate for %s (%s), not the one issued for webmail", host, leafFingerprint(leaf.Raw))
					}
					return leaf.VerifyHostname(host)
				},
			},
		},
	}
	defer client.CloseIdleConnections()
	base := "https://" + host
	get := func(path string) (*http.Response, []byte, error) {
		request, err := http.NewRequestWithContext(probeCtx, http.MethodGet, base+path, nil)
		if err != nil {
			return nil, nil, err
		}
		request.Header.Set("Accept", "text/html,text/css,*/*")
		response, err := client.Do(request)
		if err != nil {
			return nil, nil, &phpToolProbeError{Kind: "no_answer", Message: fmt.Sprintf("webmail did not answer on https://%s/: %v", host, err)}
		}
		defer response.Body.Close()
		body, err := io.ReadAll(io.LimitReader(response.Body, webmailAddressProofBodyLimit))
		return response, body, err
	}
	// 1. PHP behind webmail's socket: its version and extensions.
	response, body, err := get(phpToolCheckPrefix + phpToolWebmail)
	if err != nil {
		return err
	}
	if err := checkPHPToolCheckResponse(phpToolWebmail, version, response.StatusCode, body); err != nil {
		return err
	}
	// 2. The login page, with Roundcube's policy for its own origin.
	response, body, err = get("/")
	if err != nil {
		return err
	}
	if err := checkPHPToolResponse(phpToolWebmail, response.StatusCode, body); err != nil {
		return err
	}
	if missing := webmailAddressPolicyMissing(response.Header); missing != "" {
		return &phpToolProbeError{Kind: "policy_missing", Message: fmt.Sprintf("webmail on https://%s/ answered without %s", host, missing)}
	}
	// 3. Its stylesheet as text/css.
	sheet, err := webmailStylesheetURLAt(base, "/", body)
	if err != nil {
		return err
	}
	response, _, err = get(strings.TrimPrefix(sheet, base))
	if err != nil {
		return err
	}
	if contentType := response.Header.Get("Content-Type"); response.StatusCode != http.StatusOK || !strings.HasPrefix(strings.ToLower(contentType), "text/css") {
		return &phpToolProbeError{Kind: "asset_unreadable", Status: response.StatusCode,
			Message: fmt.Sprintf("webmail's stylesheet %s answered HTTP %d as %q, not as text/css", strings.TrimPrefix(sheet, base), response.StatusCode, contentType)}
	}
	return nil
}

func leafFingerprint(raw []byte) string {
	sum := sha256.Sum256(raw)
	return "SHA-256 " + hex.EncodeToString(sum[:8])
}

// webmailAddressPolicyMissing names the first security header the login
// page lacks on its own origin.
func webmailAddressPolicyMissing(header http.Header) string {
	if !strings.Contains(header.Get("Content-Security-Policy"), "frame-ancestors 'self'") {
		return "its frame-ancestors 'self' policy"
	}
	if !strings.HasPrefix(header.Get("Strict-Transport-Security"), "max-age=") {
		return "Strict-Transport-Security"
	}
	if !strings.EqualFold(header.Get("X-Content-Type-Options"), "nosniff") {
		return "X-Content-Type-Options: nosniff"
	}
	return ""
}

// EnableWebmailAddressV1 enables webmail on its own address in the
// administrator's confirmed webmail_address job.
func (a *Agent) EnableWebmailAddressV1(req *transport.WebmailAddressEnableRequest, resp *transport.WebmailAddressResponse) error {
	*resp = transport.WebmailAddressResponse{}
	if req == nil {
		resp.Error = "missing request"
		return nil
	}
	host, err := canonicalWebmailAddress(req.Host)
	if err != nil {
		webmailAddressFail(resp, errCodeWebmailAddressInvalid, "%v", err)
		return nil
	}
	ctx, finishStep, err := a.requiredServiceMutationStep(
		req.ServiceMutationBinding,
		newServiceMutationStepClaim(serviceMutationStepEnableWebmailAddress, "roundcube", host, "enable"),
	)
	if err != nil {
		resp.Error = err.Error()
		return nil
	}
	defer finishStep()
	if !acquireSiteCertbot() {
		webmailAddressFail(resp, errCodeWebmailAddressCertificate, "another certificate operation is running on this server; try again when it has finished")
		return nil
	}
	defer releaseSiteCertbot()
	*resp = enableWebmailAddress(ctx, hostWebmailAddressOps(), host, strings.TrimSpace(req.Email), req.Replace)
	return nil
}

// RemoveWebmailAddressV1 removes an address in its own
// webmail_address_remove job.
func (a *Agent) RemoveWebmailAddressV1(req *transport.WebmailAddressRemoveRequest, resp *transport.WebmailAddressRemoveResponse) error {
	*resp = transport.WebmailAddressRemoveResponse{}
	if req == nil {
		resp.Error = "missing request"
		return nil
	}
	host, err := canonicalWebmailAddress(req.Host)
	if err != nil {
		resp.ErrorCode, resp.Error = errCodeWebmailAddressInvalid, err.Error()
		return nil
	}
	ctx, finishStep, err := a.requiredServiceMutationStep(
		req.ServiceMutationBinding,
		newServiceMutationStepClaim(serviceMutationStepRemoveWebmailAddress, "roundcube", host, "remove"),
	)
	if err != nil {
		resp.Error = err.Error()
		return nil
	}
	defer finishStep()
	*resp = removeWebmailAddress(ctx, hostWebmailAddressOps(), host, req.RemoveCertificate)
	return nil
}

// WebmailAddressStatusV1 reads an address (read only).
func (a *Agent) WebmailAddressStatusV1(req *transport.WebmailAddressStatusRequest, resp *transport.WebmailAddressStatusResponse) error {
	*resp = transport.WebmailAddressStatusResponse{}
	if req == nil {
		req = &transport.WebmailAddressStatusRequest{}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	*resp = webmailAddressStatus(ctx, hostWebmailAddressOps(), req.Host, req.Prove)
	return nil
}

func isDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
