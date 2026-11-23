//go:build linux

package main

import (
	"context"
	"os"
	"strings"
	"syscall"
	"testing"
)

// S4 R1 with alpha.145: webmail's move to its own account writes the local
// server in alpha.145's closed form with PHP handed to webmail's own pool
// (renderPHPToolConfig), the form is read back as CelikPanel's own and
// current, the start-time correction of an earlier form keeps the own
// socket, and the Agent's proof and the Panel's readiness check still answer.
func TestWebmailLocalServerOnItsOwnPoolKeepsAlpha145Rules(t *testing.T) {
	own := phpToolPoolSocket(phpToolWebmail, "8.4")
	conf := renderWebmailNginxConfig(a145Socket, a145Root, own)
	if !managedWebmailConfig(conf) || !webmailLocalServerCurrent(conf) || parseWebmailFPMSocket(conf) != own {
		t.Fatalf("not read back as CelikPanel's current form with the own socket:\n%s", conf)
	}
	if strings.Contains(conf, "/run/php/php8.4-fpm.sock;") {
		t.Fatal("the stock pool is named")
	}
	for uri, want := range map[string]string{
		"/webmail/":                                    "index",
		phpToolCheckPrefix + phpToolWebmail:            "php",
		"/webmail/skins/elastic/styles/styles.min.css": "file",
		"/webmail/plugins/password/config.inc.php.bak": "refused",
		"/webmail/plugins/password/password.php":       "refused",
	} {
		if got := a145LocalAnswer(t, conf, uri); got != want {
			t.Errorf("%s: %s, want %s", uri, got, want)
		}
	}

	// The move writes exactly that form for the own socket.
	path := a145PointLocalServer(t)
	_, data, err := renderPHPToolConfig(phpToolWebmail, own)
	if err != nil {
		t.Fatal(err)
	}
	if !managedWebmailConfig(string(data)) || !webmailLocalServerCurrent(string(data)) || parseWebmailFPMSocket(string(data)) != own {
		t.Fatalf("the move's text:\n%s", data)
	}

	// An earlier form already on the own socket (a move made before the
	// update) is brought to the current form at start, keeping the socket.
	mustWriteTestFile(t, path, []byte(alpha144RenderWebmailNginxConfig(a145Socket, a145Root, own)), 0o600)
	run := func(context.Context, string, ...string) ([]byte, error) { return nil, nil }
	if said := reconcileWebmailLocalServerAtStartup(context.Background(), run); len(said) != 1 || !strings.Contains(said[0], "rewritten in the current form") {
		t.Fatalf("said %q", said)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != renderWebmailNginxConfig(a145Socket, a145Root, own) {
		t.Fatalf("not the current form on the own socket:\n%s", got)
	}
}

// alpha.145's start-time rewrite of webmail's configuration keeps the owner
// and mode the own-account move gave it (root:<webmail's account> 0640), so
// webmail's own pool still reads it, and its exact-text check still matches.
func TestWebmailConfigRewriteKeepsTheOwnAccountsGroup(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("changing a file's group needs root")
	}
	path := useWebmailBaseDir(t)
	key := strings.Repeat("5e", 18)
	mustWriteTestFile(t, path, []byte(legacyRenderRoundcubeConfig(key)), 0o640)
	const ownAccountGID = 4243
	if err := os.Chown(path, 0, ownAccountGID); err != nil {
		t.Fatal(err)
	}
	if said := reconcileRoundcubeConfigAtStartup(); len(said) != 1 || !strings.Contains(said[0], "rewritten in the current form") {
		t.Fatalf("said %q", said)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	stat := info.Sys().(*syscall.Stat_t)
	if stat.Uid != 0 || stat.Gid != ownAccountGID || info.Mode().Perm() != 0o640 {
		t.Fatalf("owner %d:%d mode %v", stat.Uid, stat.Gid, info.Mode())
	}
	if form, managed := parseRoundcubeConfigForm(func() string { b, _ := os.ReadFile(path); return string(b) }()); !managed || !form.current {
		t.Fatal("the exact-text check no longer matches")
	}
}
