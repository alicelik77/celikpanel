//go:build linux && realsystem

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Real-system evidence for alpha.109 (Frankfurt's webmail update), run only
// inside a disposable Debian 13 virtual machine booted from the base image
// (build tag realsystem, plus CELIKPANEL_REAL_SYSTEM_A109=1). Never run it on
// an installed server.
//
// Frankfurt's webmail configuration was written by an older release, without
// the location of CelikPanel's PHP check, so the proof after the swap read
// HTTP 404 and Roundcube 1.6.20 was rolled back. Under the Agent unit's
// UMask=0027 this run installs Roundcube 1.6.15 with the distribution's PHP,
// puts the older configuration back, reproduces the 404, then:
//  1. an update whose first proof is made to fail: the check location is put
//     in place first, the previous tree comes back and answers, its
//     stylesheet arrives as text/css and is readable by the web server user;
//  2. the real update to the newest release on the line, proved through the
//     check, the login page and its stylesheet.
//
// alpha.109 gerçek sistem kanıtı; yalnız atılabilir Debian 13 sanal makinesinde.

func requireRealSystemA109(t *testing.T) {
	t.Helper()
	if os.Getenv("CELIKPANEL_REAL_SYSTEM_A109") != "1" {
		t.Skip("real-system evidence runs only in the disposable alpha.109 VM")
	}
}

func a109WebmailGet(t *testing.T, path string) (int, string, []byte) {
	t.Helper()
	socket, err := webmailSocketPathForNginx()
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Timeout: 20 * time.Second, Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		var dialer net.Dialer
		return dialer.DialContext(ctx, "unix", socket)
	}}}
	response, err := client.Get("http://webmail.internal" + path)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	return response.StatusCode, response.Header.Get("Content-Type"), body
}

var a109InlineScript = regexp.MustCompile(`<script(\s[^>]*)?>`)

func TestRealSystemA109WebmailUpdateFromAnOlderConfiguration(t *testing.T) {
	requireRealSystemA109(t)
	a := &Agent{}
	if err := os.Remove("/etc/apt/sources.list.d/php.list"); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	a105Run(t, "apt-get", "update", "-q")
	if resp := a107Install(t, a, "php-fpm", ""); resp.Error != "" {
		t.Fatalf("php-fpm: %s", resp.Error)
	}
	previousMask := syscall.Umask(0o027)
	t.Cleanup(func() { syscall.Umask(previousMask) })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	const older = "1.6.15"
	archive := a107Fetch(t, roundcubeReleaseURL(older, ""))
	signature := a107Fetch(t, roundcubeReleaseURL(older, ".asc"))
	if err := verifyRoundcubeSignature(strings.NewReader(string(archive)), signature, roundcubeSigningKey, roundcubeSigningPrimary); err != nil {
		t.Fatalf("Roundcube %s's signature: %v", older, err)
	}
	sum := sha256.Sum256(archive)
	previousVersion, previousSHA := roundcubeVersion, roundcubeSHA256
	roundcubeVersion, roundcubeSHA256 = older, hex.EncodeToString(sum[:])
	t.Cleanup(func() { roundcubeVersion, roundcubeSHA256 = previousVersion, previousSHA })
	var installed InstallRoundcubeResponse
	a.installRoundcubeWith(ctx, &installed)
	logJSON(t, "install "+older, installed)
	if installed.Error != "" {
		t.Fatalf("install: %s", installed.Error)
	}
	var configured ConfigureWebmailResponse
	a.configureWebmailWith(ctx, &configured)
	if configured.Error != "" {
		t.Fatalf("configure: %s", configured.Error)
	}

	// Frankfurt's state: the configuration an older release wrote.
	current, err := os.ReadFile(webmailConfPath)
	if err != nil {
		t.Fatal(err)
	}
	socket, _ := webmailSocketPathForNginx()
	fpm := parseWebmailFPMSocket(string(current))
	legacy := legacyRenderWebmailNginxConfig(socket, roundcubeRootDir(), fpm)
	if err := os.WriteFile(webmailConfPath, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	a105Run(t, "nginx", "-t")
	a105Run(t, "systemctl", "reload", "nginx")
	time.Sleep(2 * time.Second)
	version := phpToolStateFor(phpToolWebmail).version()
	reproduced := probePHPToolHTTP(ctx, phpToolWebmail, version)
	t.Logf("Frankfurt's state: %v", reproduced)
	if reproduced == nil || !strings.Contains(reproduced.Error(), "HTTP 404") {
		t.Fatalf("the older configuration did not reproduce Frankfurt's 404: %v", reproduced)
	}

	// What the browser receives through webmail's socket: the login page
	// starts Roundcube with inline scripts (the panel's old script-src
	// 'self' refused them), and the stylesheet's type.
	status, contentType, page := a109WebmailGet(t, "/webmail/")
	inline := 0
	for _, match := range a109InlineScript.FindAllStringSubmatch(string(page), -1) {
		if !strings.Contains(match[1], "src=") {
			inline++
		}
	}
	t.Logf("login page: HTTP %d %s, %d inline scripts, rcmail started inline: %v", status, contentType, inline, strings.Contains(string(page), "new rcube_webmail()"))
	sheet, err := webmailStylesheetURL("http://webmail.internal", page)
	if err != nil {
		t.Fatal(err)
	}
	sheetStatus, sheetType, _ := a109WebmailGet(t, strings.TrimPrefix(sheet, "http://webmail.internal"))
	t.Logf("stylesheet %s: HTTP %d %q", sheet, sheetStatus, sheetType)
	if inline == 0 || sheetStatus != http.StatusOK || !strings.HasPrefix(sheetType, "text/css") {
		t.Fatalf("login page facts: inline=%d stylesheet %d %q", inline, sheetStatus, sheetType)
	}

	check := roundcubeUpdateCheck(ctx, true)
	logJSON(t, "update check", check)
	if !check.Installed || check.Version != older || !roundcubeVersionLess(older, check.LatestOnLine) {
		t.Fatalf("check = %+v", check)
	}
	target := check.LatestOnLine
	host := hostRoundcubeUpdateOps()

	// 1. The first proof made to fail: the check location goes in first,
	// the previous tree comes back and answers.
	forced := host
	calls := 0
	forced.prove = func(ctx context.Context) error {
		calls++
		if calls == 1 {
			return fmt.Errorf("injected for the evidence: Roundcube %s did not answer", target)
		}
		return host.prove(ctx)
	}
	back := updateRoundcubeTo(ctx, target, forced)
	logJSON(t, "update with a forced failed proof", back)
	if back.Updated || !back.RolledBack || !back.PreviousAnswers || back.ErrorCode != "PROOF_FAILED" {
		t.Fatalf("rollback = %+v", back)
	}
	conf, _ := os.ReadFile(webmailConfPath)
	if string(conf) != renderWebmailNginxConfig(socket, roundcubeRootDir(), fpm) {
		t.Fatalf("the configuration is not today's form with the same sockets:\n%s", conf)
	}
	if got := roundcubeTreeVersion(webmailBaseDir); got != older {
		t.Fatalf("after the rollback the tree is %q", got)
	}
	if err := host.prove(ctx); err != nil {
		t.Fatalf("webmail does not answer after the rollback: %v", err)
	}
	group := webServerGroup()
	user := "www-data"
	if group != "www-data" {
		user = group
	}
	styles := filepath.Join(webmailBaseDir, "skins", "elastic", "styles", "styles.min.css")
	if out, err := exec.Command("runuser", "-u", user, "--", "head", "-c", "16", styles).CombinedOutput(); err != nil {
		t.Fatalf("the web server user cannot read the stylesheet after the rollback: %v %s", err, out)
	}
	a107Out(t, "stat", "-c", "%A %U:%G %n", webmailBaseDir, filepath.Join(webmailBaseDir, "skins"), styles)

	// 2. The real update.
	updated := updateRoundcubeTo(ctx, target, host)
	logJSON(t, "update", updated)
	if !updated.Updated || !updated.Proved || updated.Error != "" {
		t.Fatalf("update = %+v", updated)
	}
	if got := roundcubeTreeVersion(webmailBaseDir); got != target {
		t.Fatalf("the tree is %q", got)
	}
	if out, err := exec.Command("runuser", "-u", user, "--", "head", "-c", "16", styles).CombinedOutput(); err != nil {
		t.Fatalf("the web server user cannot read the new stylesheet: %v %s", err, out)
	}
	a107Out(t, "stat", "-c", "%A %U:%G %n", webmailBaseDir, styles)
}
