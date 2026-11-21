//go:build linux

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Tenant isolation, 20 Nov 2026 (lane S3). These tests use only what existed
// before the change, so they run unchanged on the earlier code and fail there.

// Adding an additional login says only that the name cannot be used, the same
// words whatever holds it: another subscription's login, a system account or
// a system group. The words used to say which, so a customer could learn what
// exists on the server and in other subscriptions.
func TestTenantIsolationFTPLoginNameTakenSaysNothingOfWhoHasIt(t *testing.T) {
	refusal := func(t *testing.T, prepare func(*fileAccessFakeHost), login string) string {
		t.Helper()
		host := installFileAccessFakeHost(t)
		installFTPLoginSeams(t, host)
		prepare(host)
		response := ftpLoginApply(t, transport.ApplyFTPLoginRequest{Site: exampleSite, Action: transport.FTPLoginAdd,
			Login: login, Folder: "/", Password: "a long secret"})
		if response.Refusal != transport.FTPLoginRefusedNameTaken {
			t.Fatalf("%s: response %+v", login, response)
		}
		return strings.ReplaceAll(response.Detail, login, "NAME")
	}
	account := refusal(t, func(host *fileAccessFakeHost) {
		host.accounts["systemd-netd"] = fileAccessAccount{Name: "systemd-netd", UID: 998, GID: 998, Group: "systemd-netd", Home: "/", Shell: "/usr/sbin/nologin"}
	}, "systemd-netd")
	group := refusal(t, func(*fileAccessFakeHost) {
		if err := os.WriteFile(fileAccessGroupPath, []byte("example_com:x:1001:\ncelikpanel-site:x:990:example_com\nweb-team:x:1500:\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}, "web-team")
	login := refusal(t, func(host *fileAccessFakeHost) {
		first := ftpLoginApply(t, transport.ApplyFTPLoginRequest{Site: exampleSite, Action: transport.FTPLoginAdd,
			Login: "shop-upload", Folder: "/", Password: "a long secret"})
		if !first.Applied {
			t.Fatalf("first login: %+v", first)
		}
	}, "shop-upload")
	if account != group || group != login {
		t.Fatalf("the answers tell what holds the name:\n account: %s\n group:   %s\n login:   %s", account, group, login)
	}
	for _, word := range []string{"account", "group", "site", "exists"} {
		if strings.Contains(strings.ToLower(account), word) {
			t.Fatalf("the answer names what holds the name (%q): %s", word, account)
		}
	}
}

// A site account cannot list the hosting base or the subscriptions root (it
// would learn which subscriptions exist); it can still pass through them.
func TestTenantIsolationHostingBaseIsNotListable(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("the layout sets owners; run as root")
	}
	base := filepath.Join(t.TempDir(), "celikpanel")
	documentRoot := filepath.Join(base, "subscriptions", "4", "sites", "9", "public_html")
	// As site creation does: MkdirAll under the Agent's umask, then the layout.
	if err := os.MkdirAll(documentRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	_ = applyHostingLayout(documentRoot, "root")
	for _, dir := range []string{base, filepath.Join(base, "subscriptions")} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm&0o004 != 0 || perm&0o001 == 0 {
			t.Errorf("%s is %04o: want passable (o+x) and not listable (no o+r)", dir, perm)
		}
	}
}
