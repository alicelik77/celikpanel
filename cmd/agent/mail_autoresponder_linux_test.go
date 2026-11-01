//go:build linux

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	probes        []string
	probeErr      error
	dovecotState  string
	// The verified reload's error, the restarts and their error.
	reloadErr  error
	restarts   int
	restartErr error
	// staleSocket: the listener file answers although Dovecot is stopped
	// (the socket seam); used to show that it is not asked first.
	staleSocket bool
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
		probe               func(context.Context, string) error
		sleep               func(time.Duration)
		restart             func(context.Context) (string, error)
		running             func(context.Context) (bool, error)
	}{autoresponderDovecotConfPath, autoresponderTransportPath, autoresponderFileExists, autoresponderPkgFamily,
		autoresponderInstallPackages, autoresponderDovecotIs24, autoresponderApplyDovecotCfg, autoresponderReloadDovecot,
		autoresponderReloadPostfix, autoresponderPostconfGet, autoresponderPostconfSet, autoresponderMapType,
		autoresponderSocketReady, dovecotInstalledForRepair, autoresponderOpenMailRoot,
		autoresponderLMTPProbe, autoresponderSleep, autoresponderRestartDovecot, autoresponderDovecotRunning}
	t.Cleanup(func() {
		autoresponderDovecotConfPath, autoresponderTransportPath = saved.conf, saved.transportPath
		autoresponderFileExists, autoresponderPkgFamily = saved.exists, saved.family
		autoresponderInstallPackages, autoresponderDovecotIs24 = saved.install, saved.is24
		autoresponderApplyDovecotCfg, autoresponderReloadDovecot = saved.apply, saved.reloadDovecot
		autoresponderReloadPostfix, autoresponderPostconfGet = saved.reloadPostfix, saved.get
		autoresponderPostconfSet, autoresponderMapType = saved.set, saved.mapType
		autoresponderSocketReady, dovecotInstalledForRepair = saved.socket, saved.installedRepair
		autoresponderOpenMailRoot = saved.openRoot
		autoresponderLMTPProbe, autoresponderSleep = saved.probe, saved.sleep
		autoresponderRestartDovecot, autoresponderDovecotRunning = saved.restart, saved.running
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
	f.dovecotState = mailServiceReloaded
	autoresponderReloadDovecot = func(context.Context) (string, error) {
		if f.reloadErr != nil {
			return "", f.reloadErr
		}
		f.reloaded = f.dovecotState != mailServiceNotRunning
		return f.dovecotState, nil
	}
	autoresponderRestartDovecot = func(context.Context) (string, error) {
		f.restarts++
		if f.restartErr != nil {
			return "", f.restartErr
		}
		f.reloaded = true
		return mailServiceRestarted, nil
	}
	autoresponderDovecotRunning = func(context.Context) (bool, error) {
		return f.dovecotState != mailServiceNotRunning, nil
	}
	autoresponderLMTPProbe = func(_ context.Context, email string) error {
		f.probes = append(f.probes, email)
		return f.probeErr
	}
	autoresponderSleep = func(time.Duration) {}
	autoresponderSocketReady = func() bool { return f.reloaded || f.staleSocket }
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
	if target, err := os.Readlink(f.mailbox(autoresponderLinkName)); err != nil || target != autoresponderLinkTarget {
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
	// Postfix's resolver serves one more lookup from the old table unless
	// it is reloaded (lab run 5).
	if f.postfixReload != 2 {
		t.Fatalf("removing the route reloads Postfix: reloads %d", f.postfixReload)
	}
	if _, err := os.Lstat(f.mailbox(autoresponderLinkName)); !os.IsNotExist(err) {
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
	if err := os.Symlink("sieve/roundcube.sieve", f.mailbox(autoresponderLinkName)); err != nil {
		t.Fatal(err)
	}
	for _, enabled := range []bool{true, false} {
		resp := f.set(t, transport.MailAutoresponder{Enabled: enabled, Subject: "Away", Message: "x"})
		if resp.Applied || resp.Refusal != transport.AutoresponderRefusedOwnerScript || resp.Script != transport.AutoresponderScriptOwner {
			t.Fatalf("enabled=%v: %+v", enabled, resp)
		}
	}
	if target, _ := os.Readlink(f.mailbox(autoresponderLinkName)); target != "sieve/roundcube.sieve" {
		t.Fatalf("owner's active script replaced: %q", target)
	}
	if len(f.installed) != 0 || f.confWrites != 0 {
		t.Fatal("something was installed or written for a refused reply")
	}
	if _, err := os.Stat(autoresponderTransportPath); !os.IsNotExist(err) {
		t.Fatalf("a route was written: %v", err)
	}

	// CelikPanel's own script edited by hand is the owner's too.
	if err := os.Remove(f.mailbox(autoresponderLinkName)); err != nil {
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
	if _, err := os.Lstat(f.mailbox(autoresponderLinkName)); !os.IsNotExist(err) {
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
	if _, err := os.Lstat(f.mailbox(autoresponderLinkName)); !os.IsNotExist(err) {
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

func (f *autoresponderFixture) state(t *testing.T) transport.MailAutoresponderResponse {
	t.Helper()
	var resp transport.MailAutoresponderResponse
	if err := (&Agent{}).MailAutoresponderState(&transport.MailAutoresponderStateRequest{Email: "me@example.test"}, &resp); err != nil {
		t.Fatal(err)
	}
	return resp
}

// E4 (alpha.114 lab): Debian 13's LMTP stripped the domain and answered
// 550 for the mailbox, so every message bounced once it was routed there.
// The Agent now asks LMTP first and never routes (or un-routes) a mailbox
// LMTP does not accept.
func TestAutoresponderNeverRoutesAMailboxLMTPRefuses(t *testing.T) {
	f := newAutoresponderFixture(t)
	f.probeErr = errors.New("550 5.1.1 <me@example.test> User doesn't exist: me@example.test")
	resp := f.set(t, transport.MailAutoresponder{Enabled: true, Subject: "Away", Message: "x"})
	if resp.Applied || resp.Refusal != transport.AutoresponderRefusedLMTPRejected ||
		!strings.Contains(resp.Detail, "User doesn't exist") || !strings.Contains(resp.Detail, "mail keeps arriving as before") {
		t.Fatalf("refused LMTP: %+v", resp)
	}
	if len(f.probes) != autoresponderProbeAttempts || f.probes[0] != "me@example.test" {
		t.Fatalf("probes: %v", f.probes)
	}
	if routes, _ := os.ReadFile(autoresponderTransportPath); len(routes) != 0 || f.transportMaps != "" {
		t.Fatalf("mail was routed to LMTP: %q maps=%q", routes, f.transportMaps)
	}
	if _, err := os.Lstat(f.mailbox(autoresponderLinkName)); !os.IsNotExist(err) {
		t.Fatalf("script activated: %v", err)
	}

	// Saved while LMTP accepted it, then LMTP stops accepting it (for
	// example the administrator changed 20-lmtp.conf): saving again takes
	// the route away so mail returns to Postfix's own delivery.
	f.probeErr, f.probes = nil, nil
	if resp := f.set(t, transport.MailAutoresponder{Enabled: true, Subject: "Away", Message: "x"}); !resp.Applied || len(f.probes) != 1 {
		t.Fatalf("enable: %+v probes=%v", resp, f.probes)
	}
	if st := f.state(t); !st.Applied || st.Script != transport.AutoresponderScriptManaged {
		t.Fatalf("state right after enable: %+v", st)
	}
	f.probeErr = errors.New("550 5.1.1 User doesn't exist")
	reloadsBefore := f.postfixReload
	resp = f.set(t, transport.MailAutoresponder{Enabled: true, Subject: "Away", Message: "y"})
	if resp.Refusal != transport.AutoresponderRefusedLMTPRejected || resp.Script != transport.AutoresponderScriptNone {
		t.Fatalf("refused re-save: %+v", resp)
	}
	if routes, _ := os.ReadFile(autoresponderTransportPath); len(routes) != 0 {
		t.Fatalf("route kept although LMTP refuses the mailbox: %q", routes)
	}
	// Lab run 5: without a reload the next message still went to Dovecot
	// and bounced, although the refusal said mail keeps arriving as before.
	if f.postfixReload != reloadsBefore+1 {
		t.Fatalf("route removal after the refusal did not reload Postfix: %d -> %d", reloadsBefore, f.postfixReload)
	}
	if _, err := os.Lstat(f.mailbox(autoresponderLinkName)); !os.IsNotExist(err) {
		t.Fatalf("script left active: %v", err)
	}
	if st := f.state(t); !st.Applied || st.Script != transport.AutoresponderScriptNone {
		t.Fatalf("state after rollback: %+v", st)
	}
}

func TestAutoresponderRefusalSaysWhenPostfixDidNotReloadTheRemovedRoute(t *testing.T) {
	f := newAutoresponderFixture(t)
	if resp := f.set(t, transport.MailAutoresponder{Enabled: true, Subject: "Away", Message: "x"}); !resp.Applied {
		t.Fatalf("enable: %+v", resp)
	}
	autoresponderReloadPostfix = func(context.Context) (string, error) {
		f.postfixReload++
		return "", errors.New("postfix reload timed out")
	}
	f.probeErr = errors.New("550 5.1.1 User doesn't exist")
	resp := f.set(t, transport.MailAutoresponder{Enabled: true, Subject: "Away", Message: "y"})
	if resp.Refusal != transport.AutoresponderRefusedLMTPRejected ||
		!strings.Contains(resp.Detail, "sudo systemctl reload postfix") || !strings.Contains(resp.Detail, "may still go to Dovecot") {
		t.Fatalf("refusal hides the missing reload: %+v", resp)
	}
	if routes, _ := os.ReadFile(autoresponderTransportPath); len(routes) != 0 {
		t.Fatalf("route kept: %q", routes)
	}
}

func TestAutoresponderRefusesWhileDovecotIsStopped(t *testing.T) {
	f := newAutoresponderFixture(t)
	f.dovecotState = mailServiceNotRunning
	// A stopped Dovecot leaves its listener file behind (lab run 2,
	// E-new-3): systemd's answer comes first, before any package, file or
	// reload.
	f.staleSocket = true
	resp := f.set(t, transport.MailAutoresponder{Enabled: true, Subject: "Away", Message: "x"})
	if resp.Applied || resp.Refusal != transport.AutoresponderRefusedDovecotStopped || !strings.Contains(resp.Detail, "systemctl start dovecot") {
		t.Fatalf("stopped Dovecot: %+v", resp)
	}
	if routes, _ := os.ReadFile(autoresponderTransportPath); len(routes) != 0 || len(f.probes) != 0 {
		t.Fatalf("routed without asking LMTP: %q probes=%v", routes, f.probes)
	}
	if len(f.installed) != 0 || f.confWrites != 0 || f.restarts != 0 {
		t.Fatalf("changed something while Dovecot is stopped: installed=%v writes=%d restarts=%d", f.installed, f.confWrites, f.restarts)
	}
	if _, err := os.Stat(autoresponderDovecotConfPath); !os.IsNotExist(err) {
		t.Fatalf("drop-in written while Dovecot is stopped: %v", err)
	}
}

// Arch, Dovecot 2.4.5 (lab run 2, E-new-2): pigeonhole was installed while
// Dovecot ran, and the running Dovecot refused every reload naming
// sieve_script. The refusal is seen (not systemctl's exit 0) and answered by
// one restart.
func TestAutoresponderRestartsADovecotThatPredatesItsSievePlugin(t *testing.T) {
	f := newAutoresponderFixture(t)
	f.reloadErr = &mailServiceError{service: "Dovecot", stage: mailServiceStageReload,
		detail: "Dovecot kept its previous settings: Error in configuration file /etc/dovecot/conf.d/96-celikpanel-sieve.conf line 23: Unknown section name: sieve_script"}
	resp := f.set(t, transport.MailAutoresponder{Enabled: true, Subject: "Away", Message: "x"})
	// Lab run 4: the note ended in a stray "; " when the route step had none.
	if !resp.Applied || f.restarts != 1 || !strings.Contains(resp.Warning, "restarted once") || strings.HasSuffix(strings.TrimSpace(resp.Warning), ";") {
		t.Fatalf("restart answer: %+v restarts=%d", resp, f.restarts)
	}
	if conf, _ := os.ReadFile(autoresponderDovecotConfPath); string(conf) != renderAutoresponderDovecotConf(true) {
		t.Fatalf("drop-in after the restart:\n%s", conf)
	}
	if routes, _ := os.ReadFile(autoresponderTransportPath); !strings.Contains(string(routes), "me@example.test") {
		t.Fatalf("route: %q", routes)
	}
}

// Any other refused reload keeps Dovecot on its previous settings: the drop-in
// written now is put back and no mailbox is routed. An unreadable result is
// not success, and a restart that fails also puts the file back and starts
// Dovecot again.
func TestAutoresponderPutsTheDropInBackWhenDovecotRefusesIt(t *testing.T) {
	f := newAutoresponderFixture(t)
	f.reloadErr = &mailServiceError{service: "Dovecot", stage: mailServiceStageReload,
		detail: "Dovecot kept its previous settings: Error in configuration file /etc/dovecot/conf.d/10-auth.conf line 4: Invalid value"}
	resp := f.set(t, transport.MailAutoresponder{Enabled: true, Subject: "Away", Message: "x"})
	if resp.Applied || resp.Refusal != transport.AutoresponderRefusedDovecotRejected || f.restarts != 0 ||
		!strings.Contains(resp.Detail, "Invalid value") || !strings.Contains(resp.Detail, "put the previous") ||
		strings.Contains(resp.Detail, "protocols") {
		t.Fatalf("refused reload: %+v restarts=%d", resp, f.restarts)
	}
	if _, err := os.Stat(autoresponderDovecotConfPath); !os.IsNotExist(err) {
		t.Fatalf("drop-in kept after the refused reload: %v", err)
	}
	if routes, _ := os.ReadFile(autoresponderTransportPath); len(routes) != 0 || len(f.probes) != 0 {
		t.Fatalf("routed: %q probes=%v", routes, f.probes)
	}

	f.reloadErr = &mailServiceError{service: "Dovecot", stage: mailServiceStageVerify, unknown: true, detail: "doveadm log errors could not be read"}
	resp = f.set(t, transport.MailAutoresponder{Enabled: true, Subject: "Away", Message: "x"})
	if resp.Applied || resp.Refusal != transport.AutoresponderRefusedDovecotRejected || !strings.Contains(resp.Detail, "could not confirm") {
		t.Fatalf("unknown reload: %+v", resp)
	}
	if _, err := os.Stat(autoresponderDovecotConfPath); !os.IsNotExist(err) {
		t.Fatalf("drop-in kept after an unconfirmed reload: %v", err)
	}

	f.reloadErr = &mailServiceError{service: "Dovecot", stage: mailServiceStageReload, detail: "Unknown setting: sieve_script_x"}
	f.restartErr = errors.New("Dovecot is not running with the new configuration")
	resp = f.set(t, transport.MailAutoresponder{Enabled: true, Subject: "Away", Message: "x"})
	if resp.Applied || resp.Refusal != transport.AutoresponderRefusedDovecotRejected || f.restarts != 2 ||
		!strings.Contains(resp.Detail, "did not restart") {
		t.Fatalf("failed restart: %+v restarts=%d", resp, f.restarts)
	}
	if _, err := os.Stat(autoresponderDovecotConfPath); !os.IsNotExist(err) {
		t.Fatalf("drop-in kept after the failed restart: %v", err)
	}
}

// The GET right after a successful PUT showed applied=false (lab E4).
func TestAutoresponderStateReportsAWholeOrHalfMailbox(t *testing.T) {
	f := newAutoresponderFixture(t)
	if st := f.state(t); !st.Applied || st.Script != transport.AutoresponderScriptNone {
		t.Fatalf("nothing set: %+v", st)
	}
	if resp := f.set(t, transport.MailAutoresponder{Enabled: true, Subject: "Away", Message: "x"}); !resp.Applied {
		t.Fatalf("enable: %+v", resp)
	}
	if st := f.state(t); !st.Applied || st.Script != transport.AutoresponderScriptManaged {
		t.Fatalf("enabled: %+v", st)
	}
	// The script without its route is half, not applied.
	if err := os.WriteFile(autoresponderTransportPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if st := f.state(t); st.Applied {
		t.Fatalf("script without route counted as applied: %+v", st)
	}
}

// The first alpha.114 candidates activated the script as .dovecot.sieve in
// the maildir root, which Maildir++ reads as a folder ("stat(.../.dovecot.sieve/tmp)
// failed: Not a directory" on Ubuntu 24.04). Saving again moves it.
func TestAutoresponderMovesTheEarlierDotActiveLink(t *testing.T) {
	f := newAutoresponderFixture(t)
	if err := os.MkdirAll(f.mailbox("sieve"), 0o700); err != nil {
		t.Fatal(err)
	}
	script := renderAutoresponderSieve(transport.MailAutoresponder{Enabled: true, Subject: "Away", Message: "old", IntervalDays: 1})
	if err := os.WriteFile(f.mailbox("sieve/"+autoresponderScriptName), []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(autoresponderLinkTarget, f.mailbox(autoresponderLegacyLinkName)); err != nil {
		t.Fatal(err)
	}
	if st := f.state(t); st.Script != transport.AutoresponderScriptManaged {
		t.Fatalf("earlier link not read as CelikPanel's: %+v", st)
	}
	if resp := f.set(t, transport.MailAutoresponder{Enabled: true, Subject: "Away", Message: "new"}); !resp.Applied {
		t.Fatalf("enable: %+v", resp)
	}
	if _, err := os.Lstat(f.mailbox(autoresponderLegacyLinkName)); !os.IsNotExist(err) {
		t.Fatalf("earlier dot link kept: %v", err)
	}
	if target, err := os.Readlink(f.mailbox(autoresponderLinkName)); err != nil || target != autoresponderLinkTarget {
		t.Fatalf("active link: %q %v", target, err)
	}
	// An owner's .dovecot.sieve that is not CelikPanel's link stays.
	if err := os.WriteFile(f.mailbox(autoresponderLegacyLinkName), []byte("keep;\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if resp := f.set(t, transport.MailAutoresponder{Enabled: false, Subject: "Away", Message: "new"}); !resp.Applied {
		t.Fatalf("disable: %+v", resp)
	}
	if body, err := os.ReadFile(f.mailbox(autoresponderLegacyLinkName)); err != nil || string(body) != "keep;\n" {
		t.Fatalf("owner's file touched: %q %v", body, err)
	}
}
