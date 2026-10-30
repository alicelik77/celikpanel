//go:build linux

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type dovecotRepairHost struct {
	dropIn string
	log    string
	root   string
}

// newDovecotRepairHost stands in Dovecot's commands (dovecot --version,
// doveconf -n, systemctl show/reload) on PATH and points the drop-in and the
// mail root at scratch paths.
func newDovecotRepairHost(t *testing.T, version string, doveconfOK, running bool) *dovecotRepairHost {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(dir, "commands.log")
	doveconfExit := "0"
	if !doveconfOK {
		doveconfExit = "1"
	}
	unit := `ActiveState=inactive\nSubState=dead\nMainPID=0\n`
	if running {
		unit = `ActiveState=active\nSubState=running\nMainPID=7100\n`
	}
	scripts := map[string]string{
		"dovecot":   "printf '%s (0000000000)\\n' '" + version + "'\n",
		"doveconf":  "if [ " + doveconfExit + " -ne 0 ]; then printf 'doveconf: Fatal: Error in configuration file /etc/dovecot/conf.d/99-celikpanel.conf line 2: Unknown setting: mail_inbox_path\\n'; fi\nexit " + doveconfExit + "\n",
		"systemctl": "if [ \"$1\" = show ]; then printf '" + unit + "'; fi\nexit 0\n",
	}
	for name, body := range scripts {
		script := "#!/bin/sh\nprintf '%s %s\\n' '" + name + "' \"$*\" >>'" + logPath + "'\n" + body
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)

	previousPath, previousInstalled, previousRoot, previousSleep := dovecotVirtualDropInPath, dovecotInstalledForRepair, mailRootDir, mailServiceSleep
	host := &dovecotRepairHost{
		dropIn: filepath.Join(dir, "conf.d", "99-celikpanel.conf"),
		log:    logPath,
		root:   filepath.Join(dir, "mail", "vhosts"),
	}
	if err := os.MkdirAll(filepath.Dir(host.dropIn), 0o755); err != nil {
		t.Fatal(err)
	}
	dovecotVirtualDropInPath = host.dropIn
	dovecotInstalledForRepair = func() bool { return true }
	mailRootDir = host.root
	mailServiceSleep = func(time.Duration) {}
	t.Cleanup(func() {
		dovecotVirtualDropInPath, dovecotInstalledForRepair, mailRootDir, mailServiceSleep = previousPath, previousInstalled, previousRoot, previousSleep
	})
	return host
}

func (h *dovecotRepairHost) write(t *testing.T, content string) {
	t.Helper()
	if err := os.WriteFile(h.dropIn, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (h *dovecotRepairHost) read(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(h.dropIn)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func (h *dovecotRepairHost) commands(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(h.log)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return string(data)
}

// An existing Debian 13 server (alpha.112 text) gets the INBOX fix at the
// Panel's next start, validated and reloaded once.
func TestDovecotStartRepairUpdatesAnEarlierReleaseText(t *testing.T) {
	host := newDovecotRepairHost(t, "2.4.1-4", true, true)
	host.write(t, earlierDovecotVirtualTexts(host.root)[1])

	note := reconcileDovecotVirtualDropIn(context.Background())
	if note != dovecotVirtualRepairUpdated {
		t.Fatalf("note = %q", note)
	}
	got := host.read(t)
	if got != buildDovecotVirtualConfAt(true, host.root) {
		t.Fatalf("drop-in is not this release's text:\n%s", got)
	}
	if !strings.Contains(got, "\nmail_inbox_path = "+host.root+"/%{user | domain}/%{user | username}\n") {
		t.Fatalf("drop-in lacks the INBOX setting:\n%s", got)
	}
	log := host.commands(t)
	if !strings.Contains(log, "doveconf -n") || strings.Count(log, "systemctl reload dovecot") != 1 {
		t.Fatalf("commands = %q, want validation and exactly one reload", log)
	}

	// The next start finds this release's text: nothing is written or reloaded.
	if err := os.Remove(host.log); err != nil {
		t.Fatal(err)
	}
	if note := reconcileDovecotVirtualDropIn(context.Background()); note != "" {
		t.Fatalf("second start note = %q", note)
	}
	if log := host.commands(t); strings.Contains(log, "doveconf") || strings.Contains(log, "systemctl") {
		t.Fatalf("second start ran %q", log)
	}
}

// Ubuntu 24.04 (2.3): the earlier text is CelikPanel's and is brought to the
// current form, whose mail settings are unchanged.
func TestDovecotStartRepairAdoptsTheEarlier23Text(t *testing.T) {
	host := newDovecotRepairHost(t, "2.3.21", true, true)
	host.write(t, earlierDovecotVirtualTexts(host.root)[0])
	if note := reconcileDovecotVirtualDropIn(context.Background()); note != dovecotVirtualRepairUpdated {
		t.Fatalf("note = %q", note)
	}
	if got := host.read(t); got != buildDovecotVirtualConfAt(false, host.root) {
		t.Fatalf("drop-in:\n%s", got)
	}
}

// The administrator's file is kept byte for byte and nothing is run against it.
func TestDovecotStartRepairKeepsTheAdministratorsFile(t *testing.T) {
	host := newDovecotRepairHost(t, "2.4.1-4", true, true)
	owner := earlierDovecotVirtualTexts(host.root)[1] + "mail_inbox_path = /srv/inbox/%{user}\n"
	host.write(t, owner)
	note := reconcileDovecotVirtualDropIn(context.Background())
	if !strings.Contains(note, dovecotVirtualOwnerEditedReason) {
		t.Fatalf("note = %q", note)
	}
	if got := host.read(t); got != owner {
		t.Fatalf("administrator's file changed:\n%s", got)
	}
	if log := host.commands(t); strings.Contains(log, "doveconf") || strings.Contains(log, "systemctl") {
		t.Fatalf("commands = %q, want none against the administrator's file", log)
	}
}

// No drop-in: mail was never set up (or the administrator removed it). The
// start repair does not create it.
func TestDovecotStartRepairNeverCreatesTheFile(t *testing.T) {
	host := newDovecotRepairHost(t, "2.4.1-4", true, true)
	if note := reconcileDovecotVirtualDropIn(context.Background()); note != "" {
		t.Fatalf("note = %q", note)
	}
	if _, err := os.Stat(host.dropIn); !os.IsNotExist(err) {
		t.Fatalf("drop-in created: %v", err)
	}
}

// Dovecot refuses the new text: the previous file is back and Dovecot is not
// reloaded.
func TestDovecotStartRepairRestoresWhenDovecotRefuses(t *testing.T) {
	host := newDovecotRepairHost(t, "2.4.1-4", false, true)
	earlier := earlierDovecotVirtualTexts(host.root)[1]
	host.write(t, earlier)
	note := reconcileDovecotVirtualDropIn(context.Background())
	if !strings.Contains(note, "were not updated") || !strings.Contains(note, "Unknown setting") {
		t.Fatalf("note = %q", note)
	}
	if got := host.read(t); got != earlier {
		t.Fatalf("previous file not restored:\n%s", got)
	}
	if log := host.commands(t); strings.Contains(log, "systemctl reload") {
		t.Fatalf("refused text was reloaded: %q", log)
	}
}

// A stopped Dovecot stays stopped (the owner's choice); it reads the new text
// when it starts.
func TestDovecotStartRepairLeavesAStoppedDovecotStopped(t *testing.T) {
	host := newDovecotRepairHost(t, "2.4.1-4", true, false)
	host.write(t, earlierDovecotVirtualTexts(host.root)[1])
	if note := reconcileDovecotVirtualDropIn(context.Background()); note != dovecotVirtualRepairWritten {
		t.Fatalf("note = %q", note)
	}
	if log := host.commands(t); strings.Contains(log, "systemctl reload") || strings.Contains(log, "systemctl start") ||
		strings.Contains(log, "systemctl restart") {
		t.Fatalf("stopped Dovecot was started or reloaded: %q", log)
	}
}

// The mail setup itself stops on the administrator's file before writing
// anything, with the reason naming who acts and how it resumes.
func TestConfigureDovecotVirtualRefusesTheAdministratorsFile(t *testing.T) {
	host := newDovecotRepairHost(t, "2.4.1-4", true, true)
	owner := "# mine\nmail_driver = maildir\nmail_path = /srv/mail/%{user}\n"
	host.write(t, owner)
	err := configureDovecotVirtual()
	if err == nil || err.Error() != dovecotVirtualOwnerEditedReason {
		t.Fatalf("err = %v", err)
	}
	if got := host.read(t); got != owner {
		t.Fatalf("administrator's file changed:\n%s", got)
	}
	if log := host.commands(t); strings.Contains(log, "doveconf") || strings.Contains(log, "systemctl") {
		t.Fatalf("commands = %q, want none", log)
	}
}
