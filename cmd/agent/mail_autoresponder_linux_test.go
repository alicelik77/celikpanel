//go:build linux

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
	"golang.org/x/sys/unix"
)

type autoresponderFixture struct {
	dir, root     string
	plugin        bool
	installed     []string
	confWrites    int
	reloaded      bool
	transportMaps string
	postfixReload int
}

func newAutoresponderFixture(t *testing.T) *autoresponderFixture {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("the mailbox Sieve store sets the mail user's ownership; run as root")
	}
	dir := configureMailMutationTest(t)
	f := &autoresponderFixture{dir: dir, root: filepath.Join(dir, "vhosts")}
	for _, path := range []string{filepath.Join(f.root, "example.test", "me")} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	writeMailTestFile(t, postfixVBoxPath, "me@example.test example.test/me/\n", 0o644)

	saved := struct {
		conf, transportPath string
		exists              func(string) bool
		family              func() string
		install             func(context.Context, string, []string) error
		is24                func() (bool, error)
		apply               func(string, string) error
		reloadDovecot       func(context.Context) (string, error)
		reloadPostfix       func(context.Context) (string, error)
		get                 func(context.Context, string) (string, error)
		set                 func(context.Context, string, string) error
		mapType             func(context.Context) string
		socket              func() bool
		installedRepair     func() bool
		openRoot            func() (int, error)
	}{autoresponderDovecotConfPath, autoresponderTransportPath, autoresponderFileExists, autoresponderPkgFamily,
		autoresponderInstallPackages, autoresponderDovecotIs24, autoresponderApplyDovecotCfg, autoresponderReloadDovecot,
		autoresponderReloadPostfix, autoresponderPostconfGet, autoresponderPostconfSet, autoresponderMapType,
		autoresponderSocketReady, dovecotInstalledForRepair, autoresponderOpenMailRoot}
	t.Cleanup(func() {
		autoresponderDovecotConfPath, autoresponderTransportPath = saved.conf, saved.transportPath
		autoresponderFileExists, autoresponderPkgFamily = saved.exists, saved.family
		autoresponderInstallPackages, autoresponderDovecotIs24 = saved.install, saved.is24
		autoresponderApplyDovecotCfg, autoresponderReloadDovecot = saved.apply, saved.reloadDovecot
		autoresponderReloadPostfix, autoresponderPostconfGet = saved.reloadPostfix, saved.get
		autoresponderPostconfSet, autoresponderMapType = saved.set, saved.mapType
		autoresponderSocketReady, dovecotInstalledForRepair = saved.socket, saved.installedRepair
		autoresponderOpenMailRoot = saved.openRoot
	})
	autoresponderDovecotConfPath = filepath.Join(dir, "96-celikpanel-sieve.conf")
	autoresponderTransportPath = filepath.Join(dir, "celikpanel_transport")
	autoresponderFileExists = func(path string) bool {
		if path == "/etc/postfix" {
			return true
		}
		return f.plugin
	}
	autoresponderPkgFamily = func() string { return "apt" }
	autoresponderInstallPackages = func(_ context.Context, family string, packages []string) error {
		f.installed = append(f.installed, family+":"+strings.Join(packages, ","))
		f.plugin = true
		return nil
	}
	autoresponderDovecotIs24 = func() (bool, error) { return true, nil }
	autoresponderApplyDovecotCfg = func(path, content string) error {
		f.confWrites++
		return os.WriteFile(path, []byte(content), 0o644)
	}
	autoresponderReloadDovecot = func(context.Context) (string, error) {
		f.reloaded = true
		return mailServiceReloaded, nil
	}
	autoresponderSocketReady = func() bool { return f.reloaded }
	autoresponderReloadPostfix = func(context.Context) (string, error) {
		f.postfixReload++
		return mailServiceReloaded, nil
	}
	autoresponderPostconfGet = func(context.Context, string) (string, error) { return f.transportMaps, nil }
	autoresponderPostconfSet = func(_ context.Context, key, value string) error {
		if key != "transport_maps" {
			t.Fatalf("postconf %s", key)
		}
		f.transportMaps = value
		return nil
	}
	autoresponderMapType = func(context.Context) string { return "hash" }
	dovecotInstalledForRepair = func() bool { return true }
	autoresponderOpenMailRoot = func() (int, error) {
		return unix.Open(f.root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	}
	return f
}

func (f *autoresponderFixture) set(t *testing.T, setting transport.MailAutoresponder) transport.MailAutoresponderResponse {
	t.Helper()
	normalized, err := transport.NormalizeMailAutoresponder(setting, "me@example.test")
	if err != nil {
		t.Fatal(err)
	}
	var resp transport.MailAutoresponderResponse
	if err := fillAutoresponderRefusal(&resp, applyMailAutoresponder(context.Background(),
		"me@example.test", "me", "example.test", normalized, &resp)); err != nil {
		t.Fatal(err)
	}
	return resp
}

func (f *autoresponderFixture) mailbox(name string) string {
	return filepath.Join(f.root, "example.test", "me", name)
}

func TestAutoresponderEnableInstallsSieveRoutesOneMailboxAndDisableRemovesIt(t *testing.T) {
	f := newAutoresponderFixture(t)
	resp := f.set(t, transport.MailAutoresponder{Enabled: true, Subject: "Away", Message: "Back soon", IntervalDays: 2})
	if !resp.Applied || resp.Script != transport.AutoresponderScriptManaged || resp.Refusal != "" {
		t.Fatalf("enable: %+v", resp)
	}
	if strings.Join(f.installed, " ") != "apt:dovecot-sieve,dovecot-lmtpd" {
		t.Fatalf("installed: %v", f.installed)
	}
	if conf, _ := os.ReadFile(autoresponderDovecotConfPath); string(conf) != renderAutoresponderDovecotConf(true) {
		t.Fatalf("dovecot drop-in:\n%s", conf)
	}
	if target, err := os.Readlink(f.mailbox(".dovecot.sieve")); err != nil || target != autoresponderLinkTarget {
		t.Fatalf("active link: %q %v", target, err)
	}
	script, err := os.ReadFile(f.mailbox("sieve/" + autoresponderScriptName))
	if err != nil || !strings.Contains(string(script), `"Back soon"`) {
		t.Fatalf("script: %s %v", script, err)
	}
	var st unix.Stat_t
	if err := unix.Stat(f.mailbox("sieve/"+autoresponderScriptName), &st); err != nil || st.Uid != 5000 || st.Mode&0o777 != 0o600 {
		t.Fatalf("script owner/mode: %+v %v", st, err)
	}
	routes, _ := os.ReadFile(autoresponderTransportPath)
	if string(routes) != "me@example.test lmtp:unix:private/dovecot-lmtp\n" {
		t.Fatalf("transport: %q", routes)
	}
	if f.transportMaps != "hash:"+autoresponderTransportPath || f.postfixReload != 1 {
		t.Fatalf("transport_maps %q reloads %d", f.transportMaps, f.postfixReload)
	}

	// Saving again with a new text rewrites only the script.
	resp = f.set(t, transport.MailAutoresponder{Enabled: true, Subject: "Away", Message: "Back Monday", IntervalDays: 2})
	if !resp.Applied || f.confWrites != 1 || f.postfixReload != 1 || len(f.installed) != 1 {
		t.Fatalf("second save: %+v writes=%d reloads=%d", resp, f.confWrites, f.postfixReload)
	}
	if script, _ := os.ReadFile(f.mailbox("sieve/" + autoresponderScriptName)); !strings.Contains(string(script), "Back Monday") {
		t.Fatalf("script not updated: %s", script)
	}

	resp = f.set(t, transport.MailAutoresponder{Enabled: false, Subject: "Away", Message: "Back Monday", IntervalDays: 2})
	if !resp.Applied || resp.Script != transport.AutoresponderScriptNone {
		t.Fatalf("disable: %+v", resp)
	}
	if routes, _ := os.ReadFile(autoresponderTransportPath); len(routes) != 0 {
		t.Fatalf("route left: %q", routes)
	}
	if _, err := os.Lstat(f.mailbox(".dovecot.sieve")); !os.IsNotExist(err) {
		t.Fatalf("active link left: %v", err)
	}
	if _, err := os.Stat(f.mailbox("sieve/" + autoresponderScriptName)); !os.IsNotExist(err) {
		t.Fatalf("script left: %v", err)
	}
}

func TestAutoresponderKeepsTheMailboxOwnersScript(t *testing.T) {
	f := newAutoresponderFixture(t)
	if err := os.MkdirAll(f.mailbox("sieve"), 0o700); err != nil {
		t.Fatal(err)
	}
	owner := "require \"fileinto\";\nfileinto \"Junk\";\n"
	if err := os.WriteFile(f.mailbox("sieve/roundcube.sieve"), []byte(owner), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("sieve/roundcube.sieve", f.mailbox(".dovecot.sieve")); err != nil {
		t.Fatal(err)
	}
	for _, enabled := range []bool{true, false} {
		resp := f.set(t, transport.MailAutoresponder{Enabled: enabled, Subject: "Away", Message: "x"})
		if resp.Applied || resp.Refusal != transport.AutoresponderRefusedOwnerScript || resp.Script != transport.AutoresponderScriptOwner {
			t.Fatalf("enabled=%v: %+v", enabled, resp)
		}
	}
	if target, _ := os.Readlink(f.mailbox(".dovecot.sieve")); target != "sieve/roundcube.sieve" {
		t.Fatalf("owner's active script replaced: %q", target)
	}
	if len(f.installed) != 0 || f.confWrites != 0 {
		t.Fatal("something was installed or written for a refused reply")
	}
	if _, err := os.Stat(autoresponderTransportPath); !os.IsNotExist(err) {
		t.Fatalf("a route was written: %v", err)
	}

	// CelikPanel's own script edited by hand is the owner's too.
	if err := os.Remove(f.mailbox(".dovecot.sieve")); err != nil {
		t.Fatal(err)
	}
	if resp := f.set(t, transport.MailAutoresponder{Enabled: true, Subject: "Away", Message: "x"}); !resp.Applied {
		t.Fatalf("enable: %+v", resp)
	}
	path := f.mailbox("sieve/" + autoresponderScriptName)
	edited, _ := os.ReadFile(path)
	edited = append(edited, []byte("# mine\n")...)
	if err := os.WriteFile(path, edited, 0o600); err != nil {
		t.Fatal(err)
	}
	if resp := f.set(t, transport.MailAutoresponder{Enabled: true, Subject: "Other", Message: "y"}); resp.Refusal != transport.AutoresponderRefusedOwnerScript {
		t.Fatalf("edited script was not kept: %+v", resp)
	}
	if after, _ := os.ReadFile(path); string(after) != string(edited) {
		t.Fatal("edited script overwritten")
	}
}

func TestAutoresponderRefusesOwnerDovecotAndTransportSettings(t *testing.T) {
	f := newAutoresponderFixture(t)
	f.plugin = true
	edited := strings.Replace(renderAutoresponderDovecotConf(true), "0600", "0660", 1)
	if err := os.WriteFile(autoresponderDovecotConfPath, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	resp := f.set(t, transport.MailAutoresponder{Enabled: true, Subject: "Away", Message: "x"})
	if resp.Refusal != transport.AutoresponderRefusedOwnerDovecotConf || f.confWrites != 0 {
		t.Fatalf("owner drop-in: %+v writes=%d", resp, f.confWrites)
	}
	if after, _ := os.ReadFile(autoresponderDovecotConfPath); string(after) != edited {
		t.Fatal("owner drop-in overwritten")
	}
	if _, err := os.Lstat(f.mailbox(".dovecot.sieve")); !os.IsNotExist(err) {
		t.Fatal("a script was activated although Dovecot is not ready")
	}

	if err := os.Remove(autoresponderDovecotConfPath); err != nil {
		t.Fatal(err)
	}
	f.transportMaps = "hash:/etc/postfix/transport"
	resp = f.set(t, transport.MailAutoresponder{Enabled: true, Subject: "Away", Message: "x"})
	if resp.Refusal != transport.AutoresponderRefusedOwnerTransport || !strings.Contains(resp.Detail, "hash:"+autoresponderTransportPath) {
		t.Fatalf("owner transport_maps: %+v", resp)
	}
	if f.transportMaps != "hash:/etc/postfix/transport" {
		t.Fatal("owner transport_maps changed")
	}
	if _, err := os.Lstat(f.mailbox(".dovecot.sieve")); !os.IsNotExist(err) {
		t.Fatal("a script was left active without its route")
	}
	if routes, _ := os.ReadFile(autoresponderTransportPath); len(routes) != 0 {
		t.Fatalf("route written: %q", routes)
	}
}

func TestAutoresponderStoreRefusesASymlinkedMailbox(t *testing.T) {
	f := newAutoresponderFixture(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(f.root, "example.test", "evil")); err != nil {
		t.Fatal(err)
	}
	if err := (nativeMailboxSieveStore{}).Write("example.test", "evil", []byte("x")); err == nil {
		t.Fatal("wrote through a symbolic link")
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Fatalf("files appeared outside the mail root: %v", entries)
	}
}
