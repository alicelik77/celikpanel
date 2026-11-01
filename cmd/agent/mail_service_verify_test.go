package main

import (
	"errors"
	"io/fs"
	"reflect"
	"strings"
	"testing"
	"time"
)

// fakeExit stands for a command's own non-zero exit status.
type fakeExit struct{ text string }

func (e fakeExit) Error() string { return e.text }

// fakeMailHost is a server as the mail service helpers see it: what each fixed
// command answers, and whether a reload or restart actually reaches the daemon.
type fakeMailHost struct {
	calls []string

	// Postfix
	checkOutput     string // non-empty: `postfix check` exits 1 printing this
	masterPID       int    // 0: the master is not running
	reloadOutput    string // non-empty: `postfix reload` exits 1 printing this
	reloadKills     bool   // the master exits when it is reloaded
	systemctlStarts bool   // `systemctl start|restart postfix` reaches the daemon
	statusCannotRun bool   // `postfix status` cannot be executed at all
	nextPID         int
	// `systemctl stop postfix` exits 0 and the master keeps running (the
	// wrapper unit's job succeeds whatever the daemon does).
	stopLeavesMaster bool
	// The process ID in master.pid. Postfix leaves the file when the master
	// exits, so it names the last master that ran.
	pidFile int
	// master.pid cannot be read (not "does not exist": an I/O error).
	pidFileUnreadable bool
	// The process ID in master.pid now belongs to another program.
	pidReusedBy string

	// Dovecot
	doveconfOutput  string
	dovecotPID      int
	dovecotExitsAt  int // the restarted dovecot exits after this many readings (0: stays)
	dovecotReadings int
	dovecotSame     bool // `systemctl restart dovecot` leaves the same process
	// `systemctl stop dovecot` exits 0 and the main process keeps running.
	dovecotStopLeaves bool
	// The master's error after a reload, as `doveadm log errors` shows it
	// ("": the reload was taken up). dovecotReloadUnseen: the log never
	// shows the reload; dovecotLogUnreadable: doveadm cannot be run.
	dovecotReloadRefusal string
	dovecotReloadUnseen  bool
	dovecotLogUnreadable bool
	dovecotReloads       int
}

func (h *fakeMailHost) run(name string, args ...string) ([]byte, error) {
	call := name + " " + strings.Join(args, " ")
	h.calls = append(h.calls, call)
	if strings.HasPrefix(call, "doveadm log errors -s ") {
		if h.dovecotLogUnreadable {
			return nil, errors.New("fork/exec /usr/bin/doveadm: no such file or directory")
		}
		log := "Oct 31 23:59:58.000001 Warning: auth: an older line\n"
		if h.dovecotReloads > 0 && !h.dovecotReloadUnseen {
			log += "Nov 01 11:59:43.751803 Warning: master: SIGHUP received - reloading configuration\n"
			if h.dovecotReloadRefusal != "" {
				log += "Nov 01 11:59:43.751803 Error: master: Failed to read configuration: Failed to read config: " +
					h.dovecotReloadRefusal + " - doveconf execution fallback is disabled\n"
			}
		}
		return []byte(log), nil
	}
	switch call {
	case "postfix check":
		if h.checkOutput != "" {
			return []byte(h.checkOutput), fakeExit{"exit status 1"}
		}
		return nil, nil
	case "postfix status":
		if h.statusCannotRun {
			return nil, errors.New("fork/exec /usr/sbin/postfix: resource temporarily unavailable")
		}
		if h.masterPID == 0 {
			return nil, fakeExit{"exit status 1"}
		}
		return nil, nil
	case "postconf -h queue_directory":
		return []byte("/var/spool/postfix\n"), nil
	case "postfix reload":
		if h.reloadOutput != "" {
			return []byte(h.reloadOutput), fakeExit{"exit status 1"}
		}
		if h.reloadKills {
			h.masterPID = 0
		}
		return nil, nil
	case "systemctl start postfix", "systemctl restart postfix":
		// The wrapper unit's job succeeds whatever the daemon does.
		if h.systemctlStarts {
			h.nextPID++
			h.masterPID = 40000 + h.nextPID
		}
		return nil, nil
	case "doveconf -n":
		if h.doveconfOutput != "" {
			return []byte(h.doveconfOutput), fakeExit{"exit status 89"}
		}
		return []byte("# 2.4.1\n"), nil
	case "systemctl show dovecot.service --property=ActiveState --property=SubState --property=MainPID":
		h.dovecotReadings++
		if h.dovecotExitsAt > 0 && h.dovecotReadings > h.dovecotExitsAt {
			return []byte("ActiveState=failed\nSubState=failed\nMainPID=0\n"), nil
		}
		if h.dovecotPID == 0 {
			return []byte("ActiveState=inactive\nSubState=dead\nMainPID=0\n"), nil
		}
		return []byte("ActiveState=active\nSubState=running\nMainPID=" + itoa(h.dovecotPID) + "\n"), nil
	case "systemctl restart dovecot", "systemctl reload-or-restart dovecot":
		if !h.dovecotSame && call == "systemctl restart dovecot" {
			h.dovecotPID += 1000
		} else if h.dovecotPID != 0 {
			h.dovecotReloads++
		}
		h.dovecotReadings = 0
		return nil, nil
	case "systemctl stop postfix":
		if !h.stopLeavesMaster {
			h.masterPID = 0
		}
		return nil, nil
	case "systemctl stop dovecot":
		if !h.dovecotStopLeaves {
			h.dovecotPID = 0
		}
		return nil, nil
	case "systemctl start dovecot":
		if h.dovecotPID == 0 {
			h.dovecotPID = 9000
		}
		h.dovecotReadings = 0
		return nil, nil
	case "systemctl reload dovecot":
		if h.dovecotPID == 0 {
			return []byte("dovecot.service is not active, cannot reload.\n"), fakeExit{"exit status 1"}
		}
		h.dovecotReloads++
		h.dovecotReadings = 0
		return nil, nil
	}
	return nil, errors.New("unexpected command: " + call)
}

func itoa(n int) string {
	digits := ""
	for ; n > 0; n /= 10 {
		digits = string(rune('0'+n%10)) + digits
	}
	if digits == "" {
		return "0"
	}
	return digits
}

func installFakeMailHost(t *testing.T) *fakeMailHost {
	t.Helper()
	host := &fakeMailHost{masterPID: 36110, systemctlStarts: true, dovecotPID: 7000}
	oldSleep, oldRead, oldExited, oldIsDirectory := mailServiceSleep, mailServiceReadFile, mailServiceExited, mailServiceIsDirectory
	t.Cleanup(func() {
		mailServiceSleep, mailServiceReadFile, mailServiceExited, mailServiceIsDirectory = oldSleep, oldRead, oldExited, oldIsDirectory
	})
	mailServiceSleep = func(time.Duration) {}
	// The one directory of this host that holds a master.pid.
	mailServiceIsDirectory = func(path string) bool { return path == "/var/spool/postfix/pid" }
	host.pidFile = host.masterPID
	mailServiceReadFile = func(path string) ([]byte, error) {
		if host.masterPID != 0 {
			host.pidFile = host.masterPID
		}
		switch path {
		case "/var/spool/postfix/pid/master.pid":
			if host.pidFileUnreadable {
				return nil, errors.New("read /var/spool/postfix/pid/master.pid: input/output error")
			}
			if host.pidFile == 0 {
				return nil, fs.ErrNotExist
			}
			return []byte("  " + itoa(host.pidFile) + "\n"), nil
		case "/proc/" + itoa(host.pidFile) + "/comm":
			if host.pidReusedBy != "" {
				return []byte(host.pidReusedBy + "\n"), nil
			}
			if host.masterPID == 0 || host.masterPID != host.pidFile {
				return nil, fs.ErrNotExist
			}
			return []byte("master\n"), nil
		}
		return nil, errors.New("unexpected file: " + path)
	}
	mailServiceExited = func(err error) bool {
		var exit fakeExit
		return errors.As(err, &exit)
	}
	return host
}

func wantMailServiceError(t *testing.T, err error, stage string, unknown bool) *mailServiceError {
	t.Helper()
	var failure *mailServiceError
	if !errors.As(err, &failure) {
		t.Fatalf("err = %v, want a mail service error at stage %q", err, stage)
	}
	if failure.stage != stage || failure.unknown != unknown {
		t.Fatalf("failure = %+v, want stage %q unknown %t", failure, stage, unknown)
	}
	return failure
}

// The measured defect (Ubuntu 24.04, 2026-10-08): the owner left
// `default_process_limit = 200 # raised for the campaign` in main.cf,
// `systemctl reload-or-restart postfix` exited 0 because postfix.service is a
// wrapper there, and the save was answered as a success. Postfix's own check is
// asked first now, and what it says is the answer; no reload is attempted and
// systemctl is not asked at all.
func TestPostfixReloadIsRefusedByItsOwnCheckBeforeAnythingIsReloaded(t *testing.T) {
	host := installFakeMailHost(t)
	host.checkOutput = "postfix: fatal: bad numerical configuration: default_process_limit = 200 # raised for the campaign\n"

	state, err := applyPostfixVerified(host.run, mailServiceReload)
	failure := wantMailServiceError(t, err, mailServiceStageCheck, false)
	if state != "" || failure.detail != "postfix: fatal: bad numerical configuration: default_process_limit = 200 # raised for the campaign" {
		t.Fatalf("state %q detail %q", state, failure.detail)
	}
	if want := []string{"postfix check"}; !reflect.DeepEqual(host.calls, want) {
		t.Fatalf("commands = %v, want %v", host.calls, want)
	}
}

func TestPostfixReloadIsVerifiedByItsOwnCommandsNotBySystemctl(t *testing.T) {
	host := installFakeMailHost(t)
	state, err := applyPostfixVerified(host.run, mailServiceReload)
	if err != nil || state != mailServiceReloaded {
		t.Fatalf("state %q err %v", state, err)
	}
	want := []string{
		"postfix check", "postfix status", "postconf -h queue_directory",
		"postfix reload", "postfix status", "postconf -h queue_directory",
	}
	if !reflect.DeepEqual(host.calls, want) {
		t.Fatalf("commands = %v, want %v", host.calls, want)
	}
}

func TestPostfixReloadThatFailsOrKillsTheMasterIsAVerifiedFailure(t *testing.T) {
	host := installFakeMailHost(t)
	host.reloadOutput = "postfix/postfix-script: fatal: the Postfix mail system is not running\n"
	_, err := applyPostfixVerified(host.run, mailServiceReload)
	if failure := wantMailServiceError(t, err, mailServiceStageReload, false); !strings.Contains(failure.detail, "not running") {
		t.Fatalf("detail = %q", failure.detail)
	}

	// The reload command succeeded and the master then exited on what it read.
	host = installFakeMailHost(t)
	host.reloadKills = true
	_, err = applyPostfixVerified(host.run, mailServiceReload)
	wantMailServiceError(t, err, mailServiceStageVerify, false)
}

// A command that cannot be run says nothing about Postfix: unknown, which is
// neither the verified failure nor a success.
func TestPostfixOutcomeIsUnknownWhenItCannotBeAsked(t *testing.T) {
	host := installFakeMailHost(t)
	host.statusCannotRun = true
	state, err := applyPostfixVerified(host.run, mailServiceReload)
	if state != "" {
		t.Fatalf("state = %q", state)
	}
	wantMailServiceError(t, err, mailServiceStageVerify, true)
	for _, call := range host.calls {
		if call == "postfix reload" {
			t.Fatal("a reload was sent although the state of the master was unknown")
		}
	}
}

// A stopped Postfix is the owner's decision: a reload leaves it stopped and
// says so; only setup starts it, and then the daemon is what is judged.
func TestStoppedPostfixIsLeftStoppedByAReloadAndVerifiedAfterAStart(t *testing.T) {
	host := installFakeMailHost(t)
	host.masterPID = 0
	state, err := applyPostfixVerified(host.run, mailServiceReload)
	if err != nil || state != mailServiceNotRunning {
		t.Fatalf("state %q err %v", state, err)
	}
	for _, call := range host.calls {
		if strings.HasPrefix(call, "systemctl") || call == "postfix reload" {
			t.Fatalf("a stopped Postfix was touched: %v", host.calls)
		}
	}

	host = installFakeMailHost(t)
	host.masterPID = 0
	state, err = applyPostfixVerified(host.run, mailServiceReloadOrStart)
	if err != nil || state != mailServiceStarted {
		t.Fatalf("state %q err %v", state, err)
	}

	// The wrapper unit's start job succeeds and the daemon never comes up.
	host = installFakeMailHost(t)
	host.masterPID, host.systemctlStarts = 0, false
	_, err = applyPostfixVerified(host.run, mailServiceReloadOrStart)
	if failure := wantMailServiceError(t, err, mailServiceStageStart, false); !strings.Contains(failure.detail, "reported success") {
		t.Fatalf("detail = %q", failure.detail)
	}
}

func TestPostfixRestartMustReachTheMaster(t *testing.T) {
	host := installFakeMailHost(t)
	state, err := applyPostfixVerified(host.run, mailServiceRestart)
	if err != nil || state != mailServiceRestarted {
		t.Fatalf("state %q err %v", state, err)
	}

	// systemctl exits 0 and the same master is still there.
	host = installFakeMailHost(t)
	host.systemctlStarts = false
	_, err = applyPostfixVerified(host.run, mailServiceRestart)
	if failure := wantMailServiceError(t, err, mailServiceStageStart, false); !strings.Contains(failure.detail, "36110") {
		t.Fatalf("detail = %q", failure.detail)
	}
}

func TestDovecotRestartIsJudgedByTheRunningProcess(t *testing.T) {
	host := installFakeMailHost(t)
	state, err := applyDovecotVerified(host.run, mailServiceRestart)
	if err != nil || state != mailServiceRestarted {
		t.Fatalf("state %q err %v", state, err)
	}

	// doveconf refuses the configuration: nothing is restarted.
	host = installFakeMailHost(t)
	host.doveconfOutput = "doveconf: Fatal: Error in configuration file /etc/dovecot/conf.d/10-ssl.conf line 12: Unknown setting: ssl_cert\n"
	_, err = applyDovecotVerified(host.run, mailServiceRestart)
	if failure := wantMailServiceError(t, err, mailServiceStageCheck, false); !strings.Contains(failure.detail, "Unknown setting") {
		t.Fatalf("detail = %q", failure.detail)
	}
	if want := []string{"doveconf -n"}; !reflect.DeepEqual(host.calls, want) {
		t.Fatalf("commands = %v", host.calls)
	}

	// Type=simple: the restart job succeeds, the program is seen once and then
	// exits.
	host = installFakeMailHost(t)
	host.dovecotExitsAt = 1
	_, err = applyDovecotVerified(host.run, mailServiceRestart)
	wantMailServiceError(t, err, mailServiceStageVerify, false)

	// The restart left the same main process.
	host = installFakeMailHost(t)
	host.dovecotSame = true
	_, err = applyDovecotVerified(host.run, mailServiceRestart)
	if failure := wantMailServiceError(t, err, mailServiceStageVerify, false); !strings.Contains(failure.detail, "7000") {
		t.Fatalf("detail = %q", failure.detail)
	}
}

// lab run 2, E-new-2 (Arch, Dovecot 2.4.5): `systemctl reload dovecot`
// exited 0 while the master logged "Failed to read configuration" and kept
// its previous settings. The answer comes from Dovecot's own log.
func TestDovecotReloadIsJudgedByDovecotsOwnLog(t *testing.T) {
	host := installFakeMailHost(t)
	state, err := applyDovecotVerified(host.run, mailServiceReload)
	if err != nil || state != mailServiceReloaded {
		t.Fatalf("state %q err %v", state, err)
	}
	if !readDovecotLog(host.calls) {
		t.Fatalf("the reload was not read from Dovecot's log: %v", host.calls)
	}

	host = installFakeMailHost(t)
	host.dovecotReloadRefusal = "Error in configuration file /etc/dovecot/conf.d/96-celikpanel-sieve.conf line 23: Unknown section name: sieve_script"
	_, err = applyDovecotVerified(host.run, mailServiceReload)
	failure := wantMailServiceError(t, err, mailServiceStageReload, false)
	if !strings.Contains(failure.detail, "line 23: Unknown section name: sieve_script") || strings.Contains(failure.detail, "fallback") {
		t.Fatalf("detail = %q", failure.detail)
	}
	if !dovecotRefusedUnknownSetting(err) {
		t.Fatal("an unknown section refused by the running Dovecot must be recognised")
	}

	// The TLS path's reload-or-restart on a running Dovecot is a reload too.
	host = installFakeMailHost(t)
	host.dovecotReloadRefusal = "Error in configuration file /etc/dovecot/conf.d/98-celikpanel-tls.conf line 3: ssl_server_cert_file: open failed"
	_, err = applyDovecotVerified(host.run, mailServiceReloadOrStart)
	wantMailServiceError(t, err, mailServiceStageReload, false)

	// The log never shows the reload, or cannot be read: unknown.
	host = installFakeMailHost(t)
	host.dovecotReloadUnseen = true
	_, err = applyDovecotVerified(host.run, mailServiceReload)
	wantMailServiceError(t, err, mailServiceStageVerify, true)
	host = installFakeMailHost(t)
	host.dovecotLogUnreadable = true
	_, err = applyDovecotVerified(host.run, mailServiceReload)
	wantMailServiceError(t, err, mailServiceStageVerify, true)
	if dovecotRefusedUnknownSetting(err) {
		t.Fatal("an unknown result is not a refusal")
	}

	// A restart is judged by the new main process, not by the log.
	host = installFakeMailHost(t)
	if _, err := applyDovecotVerified(host.run, mailServiceRestart); err != nil || readDovecotLog(host.calls) {
		t.Fatalf("restart: %v %v", err, host.calls)
	}
}

func TestDovecotReloadLogReadsTheMeasuredLines(t *testing.T) {
	// Captured from Arch's Dovecot 2.4.5 (isolated check, 1 Nov 2026).
	out := "Nov 01 11:59:43.751803 Warning: master: SIGHUP received - reloading configuration\n" +
		"Nov 01 11:59:43.751803 Error: master: Failed to read configuration: Failed to read config: Error in configuration file /etc/dovecot/conf.d/96-celikpanel-sieve.conf line 23: Unknown section name: sieve_script - doveconf execution fallback is disabled\n" +
		"Nov 01 11:59:48.873958 Warning: master: SIGHUP received - reloading configuration\n"
	if reloaded, refusal := dovecotReloadLog(out); !reloaded || refusal != "" {
		t.Fatalf("a later successful reload: %v %q", reloaded, refusal)
	}
	reloaded, refusal := dovecotReloadLog(strings.Join(strings.Split(out, "\n")[:2], "\n"))
	if !reloaded || refusal != "Error in configuration file /etc/dovecot/conf.d/96-celikpanel-sieve.conf line 23: Unknown section name: sieve_script" {
		t.Fatalf("refused reload: %v %q", reloaded, refusal)
	}
	if reloaded, refusal := dovecotReloadLog("Nov 01 11:00:00 Error: imap(bob): something else\n"); reloaded || refusal != "" {
		t.Fatalf("no reload: %v %q", reloaded, refusal)
	}
}

func TestMailServiceLineIsOneBoundedLineWithoutPasswords(t *testing.T) {
	line := mailServiceLine([]byte("\npostfix/postfix-script: warning: not owned by root: /var/spool/postfix/etc/resolv.conf\n" +
		"postfix: fatal: bad string length 0 < 1: smtp_sasl_password = hunter2\nmore\n"))
	if !strings.HasPrefix(line, "postfix: fatal: bad string length") || strings.Contains(line, "hunter2") || strings.Contains(line, "\n") {
		t.Fatalf("line = %q", line)
	}
	if got := mailServiceLine([]byte(strings.Repeat("x", 2000))); len(got) > 320 {
		t.Fatalf("an unbounded line: %d bytes", len(got))
	}
	if got := mailServiceLine(nil); got != "" {
		t.Fatalf("line of nothing = %q", got)
	}
}

// readDovecotLog reports whether Dovecot's log of a reload was read.
func readDovecotLog(calls []string) bool {
	for _, call := range calls {
		if strings.HasPrefix(call, "doveadm log errors -s ") {
			return true
		}
	}
	return false
}
