//go:build linux

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The 90% warning (alpha.118, D-044) on files only: the script is
// CelikPanel's own text, a changed file is kept, and both dialects name the
// script from Dovecot's configuration.
func TestQuotaWarningFilesAreCelikPanelsAndKeptWhenChanged(t *testing.T) {
	for _, is24 := range []bool{false, true} {
		script := renderQuotaWarningScript(is24)
		if !strings.HasPrefix(script, "#!/bin/sh\n"+quotaWarningHeader) || !quotaWarningIntact([]byte(script)) {
			t.Fatalf("2.4=%v: script header", is24)
		}
		if quotaWarningIntact([]byte(script + "echo mine\n")) {
			t.Fatal("an edited script reads as CelikPanel's")
		}
		body := dovecotQuotaBody(is24)
		if !strings.Contains(body, "executable = script "+quotaWarningScriptPath) || !strings.Contains(body, "warn 90") {
			t.Fatalf("2.4=%v: the drop-in does not run the warning:\n%s", is24, body)
		}
	}
	if !strings.Contains(renderQuotaWarningTimer(), "OnCalendar=hourly") || !quotaWarningIntact([]byte(renderQuotaWarningService())) {
		t.Fatal("timer or service text")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "unit")
	if written, kept, err := reconcileQuotaWarningFile(path, renderQuotaWarningTimer(), 0o644); err != nil || !written || kept {
		t.Fatalf("first write: %v %v %v", written, kept, err)
	}
	if written, _, _ := reconcileQuotaWarningFile(path, renderQuotaWarningTimer(), 0o644); written {
		t.Fatal("an unchanged file was written again")
	}
	if err := os.WriteFile(path, []byte("[Timer]\nOnCalendar=daily\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if written, kept, _ := reconcileQuotaWarningFile(path, renderQuotaWarningTimer(), 0o644); written || !kept {
		t.Fatal("the owner's file was not kept")
	}
}

// With Dovecot 2.4 installed and CELIKPANEL_DOVECOT_LAB=1 (the Debian 13
// development guest, as root): a mailbox filled straight into its maildir
// (as Postfix's virtual(8) does) gets one warning from the hourly scan, and
// no second one while it stays over 90%.
func TestQuotaWarningScanWarnsOncePerCrossing(t *testing.T) {
	if os.Getenv("CELIKPANEL_DOVECOT_LAB") != "1" || os.Geteuid() != 0 {
		t.Skip("set CELIKPANEL_DOVECOT_LAB=1 and run as root in a guest with Dovecot 2.4")
	}
	version, err := exec.Command("dovecot", "--version").Output()
	if err != nil || !strings.HasPrefix(strings.TrimSpace(string(version)), "2.4") {
		t.Skip("Dovecot 2.4 is not installed")
	}
	dir, err := os.MkdirTemp("/tmp", "cpqw")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(dir, "vhosts")
	state := filepath.Join(dir, "state")
	conf := filepath.Join(dir, "dovecot.conf")
	users := filepath.Join(dir, "users")
	script := filepath.Join(dir, "warn.sh")
	previous := quotaWarningScriptPath
	quotaWarningScriptPath = script
	t.Cleanup(func() { quotaWarningScriptPath = previous })

	text := "dovecot_config_version = 2.4.1\ndovecot_storage_version = 2.4.1\n" +
		"base_dir = " + dir + "/run\nstate_dir = " + dir + "/statedir\nprotocols =\nlog_path = " + dir + "/log\n" +
		"default_internal_user = dovecot\ndefault_login_user = dovenull\n" +
		dovecotVirtualBody(true, root) + dovecotQuotaBody(true)
	text = strings.ReplaceAll(text, dovecotUsersPath, users)
	text = strings.ReplaceAll(text, "user = "+vmailUser, "user = root")
	text = strings.ReplaceAll(text, "/var/spool/postfix/"+mailQuotaPolicySocket, dir+"/run/quota-status")
	for _, sub := range []string{"run", "statedir", state, root + "/ex.test/box/new", root + "/ex.test/box/cur", root + "/ex.test/box/tmp"} {
		if !filepath.IsAbs(sub) {
			sub = filepath.Join(dir, sub)
		}
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.WriteFile(conf, []byte(text), 0o644))
	must(os.WriteFile(users, []byte("box@ex.test:{PLAIN}x::::::"+dovecotQuotaFields(2)+"\n"), 0o644))
	must(os.WriteFile(script, []byte(sealQuotaWarning("#!/bin/sh\n", quotaWarningScriptBody(true, quotaWarningLDA(), "-c "+conf, "-c "+conf, state))), 0o755))
	must(exec.Command("chown", "-R", vmailUID+":"+vmailGID, root).Run())
	// Run, not CombinedOutput: the daemon keeps a captured pipe open forever.
	if err := exec.Command("dovecot", "-c", conf).Run(); err != nil {
		t.Fatalf("start dovecot: %v", err)
	}
	t.Cleanup(func() { _ = exec.Command("doveadm", "-c", conf, "stop").Run(); time.Sleep(500 * time.Millisecond) })
	time.Sleep(time.Second)

	// 1.9 MB straight into the maildir: 2 MB quota, over 90%.
	payload := strings.Repeat("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcde\n", 30000)
	must(os.WriteFile(filepath.Join(root, "ex.test/box/new/1700000000.M1P1.host"), []byte("Subject: big\n\n"+payload), 0o644))
	must(exec.Command("chown", "-R", vmailUID+":"+vmailGID, root).Run())

	scan := func() {
		t.Helper()
		if out, err := exec.Command("/bin/sh", script, "scan").CombinedOutput(); err != nil {
			t.Fatalf("scan: %v %s", err, out)
		}
	}
	countWarnings := func() int {
		n := 0
		for _, folder := range []string{"new", "cur"} {
			entries, _ := os.ReadDir(filepath.Join(root, "ex.test/box", folder))
			for _, entry := range entries {
				content, _ := os.ReadFile(filepath.Join(root, "ex.test/box", folder, entry.Name()))
				if strings.Contains(string(content), "Subject: Your mailbox is") {
					n++
				}
			}
		}
		return n
	}
	scan()
	if got := countWarnings(); got != 1 {
		log, _ := os.ReadFile(filepath.Join(dir, "log"))
		t.Fatalf("warnings after the first scan = %d, want 1\n%s", got, log)
	}
	if _, err := os.Stat(filepath.Join(state, "box@ex.test")); err != nil {
		t.Fatalf("no marker: %v", err)
	}
	scan()
	if got := countWarnings(); got != 1 {
		t.Fatalf("warnings after a second scan = %d, want still 1", got)
	}
}
