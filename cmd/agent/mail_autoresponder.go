package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Out-of-office reply per mailbox (alpha.114, Plesk parity; D-022, D-024).
//
// The reply is Dovecot Pigeonhole's Sieve `vacation`, executed by Dovecot's
// own LMTP delivery. Nothing of CelikPanel runs when mail arrives, so the
// reply keeps working with the Panel stopped or removed. Three native pieces
// make it work, each one an ordinary file the owner can read and change:
//
//  1. /etc/dovecot/conf.d/96-celikpanel-sieve.conf: Dovecot's LMTP listener
//     inside Postfix's spool (private/dovecot-lmtp), the Sieve plugin for
//     LMTP, LMTP's user lookup by the full address, and the personal script
//     location (~/sieve, active ~/dovecot.sieve). First line: the SHA-256 of
//     the rest, so an administrator's edit is detected and kept (never
//     overwritten).
//
//     The user lookup: Debian 13's stock conf.d/20-lmtp.conf sets
//     auth_username_format = %{user | username | lower} for LMTP, which
//     strips the domain; CelikPanel's passwd-file keys are full addresses,
//     so Dovecot answered "550 5.1.1 User doesn't exist" and every message
//     to a mailbox with a reply bounced (alpha.114 lab, E4). The drop-in
//     sets the full address in the lmtp protocol scope only, and the Agent
//     asks Dovecot's LMTP for the address (RCPT, no message) before any
//     mail is routed there.
//
//     The active name has no leading dot: the mailbox's home is its maildir
//     root, where Maildir++ treats every dot name as a folder, so the stock
//     ~/.dovecot.sieve made Dovecot log "stat(.../.dovecot.sieve/tmp)
//     failed: Not a directory" on each delivery (Ubuntu 24.04, lab E4).
//  2. /etc/postfix/celikpanel_transport (transport_maps): only the mailboxes
//     with a reply are delivered through Dovecot LMTP; every other mailbox
//     keeps Postfix's virtual(8) delivery exactly as before. A transport_maps
//     the administrator set to another table is kept; the reply is refused
//     with guidance instead.
//  3. The mailbox's own Sieve script <maildir>/sieve/celikpanel-autoresponder.sieve
//     and the active link <maildir>/.dovecot.sieve. The start and end dates
//     are Sieve `currentdate` tests, so the reply starts and stops by itself.
//     An active script that is not CelikPanel's (for example one a mail
//     client wrote through ManageSieve), or CelikPanel's script edited by
//     hand, is the mailbox owner's: it is reported, never replaced.
//
// Posta kutusu başına "ofis dışında" yanıtı: Dovecot'un kendi LMTP
// teslimindeki Sieve `vacation`. Panel durduğunda da çalışır. Üç yerel
// parça: Dovecot Sieve eki (özetli; yöneticinin düzenlemesi korunur),
// yalnız yanıtı olan kutuları LMTP'ye veren Postfix transport tablosu ve
// kutunun kendi Sieve betiği. Sahibin betiği asla değiştirilmez.

const (
	autoresponderScriptName   = "celikpanel-autoresponder.sieve"
	autoresponderCompiledName = "celikpanel-autoresponder.svbin"
	autoresponderLinkName     = "dovecot.sieve"
	// autoresponderLegacyLinkName is the active name the first alpha.114
	// candidates used. A link there naming CelikPanel's script is taken as
	// CelikPanel's and removed; anything else there is left alone.
	autoresponderLegacyLinkName = ".dovecot.sieve"
	autoresponderLinkTarget     = "sieve/" + autoresponderScriptName
	autoresponderScriptHeader   = "# Managed by CelikPanel: out-of-office reply. sha256="
	autoresponderDovecotHead    = "# Managed by CelikPanel: Dovecot Sieve for out-of-office replies. sha256="
	autoresponderLMTPSocket     = "/var/spool/postfix/private/dovecot-lmtp"
	autoresponderLMTPNexthop    = "lmtp:unix:private/dovecot-lmtp"
)

// Seams: production values, replaced by component tests.
var (
	autoresponderDovecotConfPath = dovecotConfDirPath + "/96-celikpanel-sieve.conf"
	autoresponderTransportPath   = "/etc/postfix/celikpanel_transport"
	autoresponderSievePlugin     = []string{
		"/usr/lib/dovecot/modules/lib90_sieve_plugin.so",
		"/usr/lib64/dovecot/modules/lib90_sieve_plugin.so",
	}
	autoresponderLMTPBinary = []string{"/usr/lib/dovecot/lmtp", "/usr/libexec/dovecot/lmtp"}
	autoresponderFileExists = func(path string) bool {
		_, err := os.Stat(path)
		return err == nil
	}
	autoresponderPkgFamily       = detectPkgFamily
	autoresponderInstallPackages = func(ctx context.Context, family string, packages []string) error {
		_, err := installPackagesContext(ctx, family, packages)
		return err
	}
	autoresponderDovecotIs24     = dovecotIs24
	autoresponderApplyDovecotCfg = applyDovecotConf
	autoresponderReloadDovecot   = func(ctx context.Context) (string, error) {
		return applyDovecotVerified(mailServiceLeaseRunner(ctx), mailServiceReload)
	}
	autoresponderRestartDovecot = func(ctx context.Context) (string, error) {
		return applyDovecotVerified(mailServiceLeaseRunner(ctx), mailServiceRestart)
	}
	// autoresponderDovecotRunning asks systemd whether dovecot.service runs
	// (ActiveState, SubState and a main process), read only. A listener file
	// left in Postfix's spool by a stopped Dovecot says nothing (E-new-2/3).
	autoresponderDovecotRunning = func(ctx context.Context) (bool, error) {
		state, failure := dovecotUnit(mailServiceLeaseRunner(ctx))
		if failure != nil {
			return false, failure
		}
		return state.active, nil
	}
	autoresponderRestoreDovecotCfg = restoreMailFile
	autoresponderReloadPostfix     = func(ctx context.Context) (string, error) {
		return applyPostfixVerified(mailServiceLeaseRunner(ctx), mailServiceReload)
	}
	autoresponderPostconfGet = func(ctx context.Context, key string) (string, error) {
		out, err := runMailTLSMutationCommand(ctx, "postconf", "-h", key)
		return strings.TrimSpace(string(out)), err
	}
	autoresponderPostconfSet = func(ctx context.Context, key, value string) error {
		out, err := runMailTLSMutationCommand(ctx, "postconf", "-e", key+"="+value)
		if err != nil {
			return fmt.Errorf("postconf %s: %s", key, firstLine(string(out)))
		}
		return nil
	}
	autoresponderMapType    = postfixMapTypeContext
	autoresponderSocketWait = 10 * time.Second
	// autoresponderSocketReady connects to the LMTP listener: a socket file
	// alone may be left over from a Dovecot that is no longer running.
	autoresponderSocketReady = func() bool {
		conn, err := net.DialTimeout("unix", autoresponderLMTPSocket, 2*time.Second)
		if err != nil {
			return false
		}
		_ = conn.Close()
		return true
	}
	autoresponderSleep = time.Sleep
	autoresponderStore = mailboxSieveStore(nativeMailboxSieveStore{})
	// autoresponderLMTPProbe asks Dovecot's LMTP whether it accepts the
	// address, exactly as Postfix will (RCPT TO, no message is sent).
	autoresponderLMTPProbe = func(ctx context.Context, email string) error {
		return probeLMTPRecipient(ctx, autoresponderLMTPSocket, email)
	}
	autoresponderProbeAttempts = 3
	// autoresponderRouted reports whether CelikPanel's transport table
	// routes the mailbox to Dovecot LMTP (read only).
	autoresponderRouted = func(email string) (bool, error) {
		content, err := readMailFile(autoresponderTransportPath)
		if err != nil {
			return false, err
		}
		return mailFileHasKey(content, email), nil
	}
)

// mailboxSieveState is what one mailbox's directory holds.
type mailboxSieveState struct {
	// LinkKind: "none", "link" (a symbolic link) or "file" (a regular
	// dovecot.sieve, which is always the owner's).
	LinkKind      string
	LinkTarget    string
	ScriptExists  bool
	ScriptContent []byte
	// LegacyLink: .dovecot.sieve is a symbolic link to CelikPanel's script
	// (written by the first alpha.114 candidates); Write and Remove drop it.
	LegacyLink bool
}

// mailboxSieveStore reads and writes the mailbox's Sieve files as the mail
// user, refusing every symbolic link on the way (nativeMailboxSieveStore).
type mailboxSieveStore interface {
	Read(domain, local string) (mailboxSieveState, error)
	Write(domain, local string, script []byte) error
	Remove(domain, local string) error
}

// renderAutoresponderSieve renders the mailbox's script. The first line
// carries the SHA-256 of everything after it (classifyMailboxSieve).
func renderAutoresponderSieve(setting transport.MailAutoresponder) string {
	var body strings.Builder
	body.WriteString("# CelikPanel writes this script only while it is exactly as CelikPanel wrote\n")
	body.WriteString("# it. If you change it, CelikPanel keeps your version and says so.\n")
	body.WriteString("# Dovecot runs it when mail arrives, with or without CelikPanel.\n")
	requires := []string{`"vacation"`}
	var tests []string
	if setting.StartDate != "" {
		tests = append(tests, `currentdate :value "ge" "date" `+sieveQuote(setting.StartDate))
	}
	if setting.EndDate != "" {
		tests = append(tests, `currentdate :value "le" "date" `+sieveQuote(setting.EndDate))
	}
	if len(tests) > 0 {
		requires = append(requires, `"date"`, `"relational"`)
	}
	body.WriteString("require [" + strings.Join(requires, ", ") + "];\n")
	action := fmt.Sprintf("vacation :days %d :subject %s", setting.IntervalDays, sieveQuote(setting.Subject))
	if len(setting.Addresses) > 0 {
		quoted := make([]string, 0, len(setting.Addresses))
		for _, address := range setting.Addresses {
			quoted = append(quoted, sieveQuote(address))
		}
		action += " :addresses [" + strings.Join(quoted, ", ") + "]"
	}
	action += " " + sieveQuote(setting.Message) + ";\n"
	switch len(tests) {
	case 0:
		body.WriteString(action)
	case 1:
		body.WriteString("if " + tests[0] + " {\n  " + action + "}\n")
	default:
		body.WriteString("if allof (" + strings.Join(tests, ", ") + ") {\n  " + action + "}\n")
	}
	text := body.String()
	sum := sha256.Sum256([]byte(text))
	return autoresponderScriptHeader + hex.EncodeToString(sum[:]) + "\n" + text
}

// sieveQuote is an RFC 5228 quoted string: backslash and double quote are
// escaped; line breaks are allowed inside it.
func sieveQuote(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return `"` + value + `"`
}

// digestHeaderIntact reports whether content starts with header + the
// SHA-256 of everything after the first line.
func digestHeaderIntact(content []byte, header string) bool {
	text := string(content)
	newline := strings.IndexByte(text, '\n')
	if newline < 0 || !strings.HasPrefix(text, header) {
		return false
	}
	sum := sha256.Sum256([]byte(text[newline+1:]))
	return strings.TrimPrefix(text[:newline], header) == hex.EncodeToString(sum[:])
}

// classifyMailboxSieve decides whose the mailbox's Sieve state is.
func classifyMailboxSieve(state mailboxSieveState) string {
	switch state.LinkKind {
	case "file":
		return transport.AutoresponderScriptOwner
	case "link":
		if state.LinkTarget != autoresponderLinkTarget {
			return transport.AutoresponderScriptOwner
		}
		if !state.ScriptExists {
			// A dangling link to CelikPanel's own name: nothing answers.
			return transport.AutoresponderScriptNone
		}
		if digestHeaderIntact(state.ScriptContent, autoresponderScriptHeader) {
			return transport.AutoresponderScriptManaged
		}
		return transport.AutoresponderScriptOwner
	default:
		if state.ScriptExists && !digestHeaderIntact(state.ScriptContent, autoresponderScriptHeader) {
			// CelikPanel's file name, changed by hand, not active: still
			// the owner's text, never overwritten.
			return transport.AutoresponderScriptOwner
		}
		return transport.AutoresponderScriptNone
	}
}

// renderAutoresponderDovecotConf renders the Sieve drop-in for the dialect.
//
// 2.3 (Debian 12, Ubuntu 22.04/24.04): mail_plugins is a string, the
// %-variables are single letters (%Lu = the full user name, lower case) and
// the personal script is the plugin { sieve = file:...;active=... } location.
// 2.4 (Debian 13, Arch): mail_plugins is a boolean list, variables are
// %{user | lower}, and the personal script is a named sieve_script block
// (named "personal", as in the stock 90-sieve.conf example).
// auth_username_format sits inside protocol lmtp only: IMAP, POP3 and SMTP
// logins keep whatever the server administrator set for them.
func renderAutoresponderDovecotConf(is24 bool) string {
	listener := `service lmtp {
  unix_listener ` + autoresponderLMTPSocket + ` {
    mode = 0600
    user = postfix
    group = postfix
  }
}
`
	preamble := `# CelikPanel writes this file only while it is exactly as CelikPanel wrote it.
# Once you edit it, CelikPanel keeps your version and tells you so.
# It lets Postfix deliver the mailboxes that have an out-of-office reply
# through Dovecot LMTP, where the Sieve plugin sends the reply. Dovecot runs
# from this file with or without CelikPanel.
# LMTP looks mailboxes up by their full address (name@domain), as they are
# listed in /etc/dovecot/users; the active script is ~/dovecot.sieve because
# a dot name in the maildir would be taken as a mail folder.
`
	var body string
	if is24 {
		body = preamble + `protocol lmtp {
  mail_plugins {
    sieve = yes
  }
  auth_username_format = %{user | lower}
}
` + listener + `sieve_script personal {
  driver = file
  path = ~/sieve
  active_path = ~/` + autoresponderLinkName + `
}
`
	} else {
		body = preamble + `protocol lmtp {
  mail_plugins = $mail_plugins sieve
  auth_username_format = %Lu
}
` + listener + `plugin {
  sieve = file:~/sieve;active=~/` + autoresponderLinkName + `
}
`
	}
	sum := sha256.Sum256([]byte(body))
	return autoresponderDovecotHead + hex.EncodeToString(sum[:]) + "\n" + body
}

// autoresponderSievePackages are the official packages that provide the
// Sieve plugin and the LMTP server on each package family.
func autoresponderSievePackages(family string) ([]string, bool) {
	switch family {
	case "apt":
		return []string{"dovecot-sieve", "dovecot-lmtpd"}, true
	case "pacman":
		// Arch's dovecot package already contains LMTP.
		return []string{"pigeonhole"}, true
	default:
		return nil, false
	}
}

func autoresponderAnyExists(paths []string) bool {
	for _, path := range paths {
		if autoresponderFileExists(path) {
			return true
		}
	}
	return false
}

func autoresponderSieveAvailable() bool {
	return autoresponderAnyExists(autoresponderSievePlugin) && autoresponderAnyExists(autoresponderLMTPBinary)
}

// autoresponderTransportValue is transport_maps naming CelikPanel's table.
func autoresponderTransportValue(ctx context.Context) string {
	return autoresponderMapType(ctx) + ":" + autoresponderTransportPath
}

// transportMapsIncludesOurs reports whether a transport_maps value already
// names CelikPanel's table (alone or in the administrator's own list).
func transportMapsIncludesOurs(value string) bool {
	for _, field := range strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' }) {
		if index := strings.IndexByte(field, ':'); index > 0 && field[index+1:] == autoresponderTransportPath {
			return true
		}
	}
	return false
}

type autoresponderRefusal struct {
	code, detail string
}

func (r *autoresponderRefusal) Error() string { return r.detail }

func refuseAutoresponder(code, detail string) error {
	return &autoresponderRefusal{code: code, detail: detail}
}

// Owner-facing D-024 texts: what was found, that nothing changed, who acts.
const (
	autoresponderOwnerScriptDetail    = "This mailbox already has its own mail filter (Sieve) script, made outside CelikPanel (for example in a mail program). CelikPanel did not change it. The mailbox owner adds the out-of-office reply to that script, or removes the script so CelikPanel can write its own."
	autoresponderOwnerDovecotDetail   = "/etc/dovecot/conf.d/96-celikpanel-sieve.conf was changed outside CelikPanel, so CelikPanel left it as it is and changed nothing. The server administrator either keeps it (it must enable Sieve for LMTP and the private/dovecot-lmtp listener) or moves it aside, for example `sudo mv /etc/dovecot/conf.d/96-celikpanel-sieve.conf /root/`, and saves the reply again."
	autoresponderOwnerTransportDetail = "Postfix's transport_maps is set to a table that is not CelikPanel's, so CelikPanel changed nothing. The server administrator adds `%s` to transport_maps (`sudo postconf -e 'transport_maps = <yours>, %s'`) and saves the reply again."
	autoresponderDovecotStoppedDetail = "Dovecot is stopped, so CelikPanel could not check that Dovecot accepts this mailbox and changed nothing; mail keeps arriving as before. The server administrator starts Dovecot (`sudo systemctl start dovecot`), then the reply is saved again."
	autoresponderLMTPRejectedDetail   = "Dovecot's mail delivery (LMTP) did not accept %s (it answered: %s). CelikPanel did not move the mailbox to Dovecot delivery, so mail keeps arriving as before and no reply was set. The server administrator checks Dovecot's LMTP settings (`sudo doveconf -n -P | grep -A3 'protocol lmtp'`; auth_username_format for LMTP must keep the full address) and `journalctl -u dovecot`, then the reply is saved again."
)

// ensureAutoresponderInfrastructure installs the Sieve package when it is
// missing, writes the Dovecot drop-in (validated by doveconf, rolled back on
// refusal), reloads Dovecot and waits for the LMTP socket. A stopped Dovecot
// is a refusal: LMTP cannot be asked whether it accepts the mailbox, so no
// mail is moved to it (the mailbox keeps Postfix's own delivery).
//
// Whether Dovecot runs is asked of systemd first, before anything is
// installed or written (lab run 2, E-new-3): a stopped Dovecot leaves its
// listener file in Postfix's spool, so the file said "ready" and the save
// ended as an LMTP refusal instead of "Dovecot is stopped".
//
// The reload is verified from Dovecot's own log (dovecotReloadOutcome), not
// from systemctl's exit status (E-new-2). A refused reload leaves the running
// Dovecot on its previous settings; the drop-in written now is put back, so
// files and the running Dovecot agree and mail keeps its delivery. One refusal
// is answered by a restart instead: "Unknown section name" or "Unknown
// setting" for files doveconf -n has just accepted means the running Dovecot
// started before a plugin was installed (measured on Arch, Dovecot 2.4.5:
// pigeonhole installed while Dovecot ran; every reload refused sieve_script
// until Dovecot was restarted). A restart that fails puts the previous file
// back and starts Dovecot again.
func ensureAutoresponderInfrastructure(ctx context.Context) (string, error) {
	running, err := autoresponderDovecotRunning(ctx)
	if err != nil {
		return "", refuseAutoresponder(transport.AutoresponderRefusedDovecotRejected,
			"CelikPanel could not read whether Dovecot is running ("+boundedNativeDetail(err.Error())+"), so it changed nothing and mail keeps arriving as before. Save the reply again; if this repeats, the server administrator checks `systemctl status dovecot`.")
	}
	if !running {
		return "", refuseAutoresponder(transport.AutoresponderRefusedDovecotStopped, autoresponderDovecotStoppedDetail)
	}
	if !autoresponderSieveAvailable() {
		family := autoresponderPkgFamily()
		packages, ok := autoresponderSievePackages(family)
		if !ok {
			return "", refuseAutoresponder(transport.AutoresponderRefusedSievePackage,
				"CelikPanel cannot install Dovecot's Sieve plugin on this operating system. Nothing was changed.")
		}
		if err := autoresponderInstallPackages(ctx, family, packages); err != nil {
			return "", refuseAutoresponder(transport.AutoresponderRefusedSievePackage,
				fmt.Sprintf("The Dovecot Sieve package (%s) could not be installed: %s. Nothing else was changed; save the reply again when the package manager works (another update may be running).",
					strings.Join(packages, ", "), boundedNativeDetail(err.Error())))
		}
		if !autoresponderSieveAvailable() {
			return "", refuseAutoresponder(transport.AutoresponderRefusedSievePackage,
				"The Dovecot Sieve package was installed, but its plugin or Dovecot's LMTP server is still missing. Nothing else was changed.")
		}
	}
	is24, err := autoresponderDovecotIs24()
	if err != nil {
		return "", refuseAutoresponder(transport.AutoresponderRefusedDovecotRejected,
			"The installed Dovecot version could not be read, so CelikPanel changed nothing: "+boundedNativeDetail(err.Error()))
	}
	want := renderAutoresponderDovecotConf(is24)
	snapshot, err := snapshotMailFile(autoresponderDovecotConfPath)
	if err != nil {
		return "", err
	}
	changed := false
	switch {
	case snapshot.exists && string(snapshot.content) == want:
	case snapshot.exists && !digestHeaderIntact(snapshot.content, autoresponderDovecotHead):
		return "", refuseAutoresponder(transport.AutoresponderRefusedOwnerDovecotConf, autoresponderOwnerDovecotDetail)
	default:
		if err := autoresponderApplyDovecotCfg(autoresponderDovecotConfPath, want); err != nil {
			return "", refuseAutoresponder(transport.AutoresponderRefusedDovecotRejected,
				"Dovecot refused the Sieve settings, so the previous configuration was kept and no reply was set: "+boundedNativeDetail(err.Error()))
		}
		changed = true
	}
	warning := ""
	if changed || !autoresponderSocketReady() {
		var err error
		if warning, err = loadAutoresponderDovecotConf(ctx, snapshot, changed); err != nil {
			return "", err
		}
	}
	deadline := time.Now().Add(autoresponderSocketWait)
	for !autoresponderSocketReady() {
		if time.Now().After(deadline) {
			return "", refuseAutoresponder(transport.AutoresponderRefusedDovecotRejected,
				"Dovecot runs with the reply settings loaded, but its LMTP listener "+autoresponderLMTPSocket+" does not answer, so no mailbox was moved to Dovecot delivery and mail keeps arriving as before. The server administrator checks that `protocols` includes lmtp (`doveconf protocols`) and reads `sudo doveadm log errors`.")
		}
		autoresponderSleep(200 * time.Millisecond)
	}
	return warning, nil
}

// autoresponderDovecotRestartedWarning is the note for the one restart.
const autoresponderDovecotRestartedWarning = "Dovecot was restarted once: it had been running since before its Sieve plugin was installed, and a running Dovecot cannot take a new plugin's settings by reloading. Mail programs reconnect by themselves."

// dovecotRefusedUnknownSetting reports a verified reload refusal naming a
// setting or section the running Dovecot does not know.
func dovecotRefusedUnknownSetting(err error) bool {
	var failure *mailServiceError
	if !errors.As(err, &failure) || failure.unknown || failure.stage != mailServiceStageReload {
		return false
	}
	return strings.Contains(failure.detail, "Unknown section name") || strings.Contains(failure.detail, "Unknown setting")
}

// loadAutoresponderDovecotConf makes the running Dovecot take the drop-in
// (ensureAutoresponderInfrastructure says why it may restart once). On any
// failure the drop-in CelikPanel wrote now is put back.
func loadAutoresponderDovecotConf(ctx context.Context, snapshot mailFileSnapshot, changed bool) (string, error) {
	putBack := func() string {
		if !changed {
			return ""
		}
		if err := autoresponderRestoreDovecotCfg(snapshot); err != nil {
			return " Putting the previous " + autoresponderDovecotConfPath + " back failed too (" + boundedNativeDetail(err.Error()) + "); the server administrator moves that file aside."
		}
		return " CelikPanel put the previous " + autoresponderDovecotConfPath + " back."
	}
	state, err := autoresponderReloadDovecot(ctx)
	if err == nil && state == mailServiceNotRunning {
		// Stopped between the first look and the reload.
		restored := putBack()
		return "", refuseAutoresponder(transport.AutoresponderRefusedDovecotStopped, autoresponderDovecotStoppedDetail+restored)
	}
	if err == nil {
		return "", nil
	}
	if dovecotRefusedUnknownSetting(err) {
		_, restartErr := autoresponderRestartDovecot(ctx)
		if restartErr == nil {
			return autoresponderDovecotRestartedWarning, nil
		}
		restored := putBack()
		again := ""
		if _, startErr := autoresponderRestartDovecot(ctx); startErr != nil {
			again = " Dovecot did not start again either (" + boundedNativeDetail(startErr.Error()) + "): the server administrator starts it (`sudo systemctl start dovecot`) and reads `sudo journalctl -u dovecot`."
		}
		return "", refuseAutoresponder(transport.AutoresponderRefusedDovecotRejected,
			"Dovecot refused the Sieve settings while running ("+boundedNativeDetail(err.Error())+") and did not restart with them ("+boundedNativeDetail(restartErr.Error())+")."+restored+again+" No mailbox was moved to Dovecot delivery.")
	}
	restored := putBack()
	var failure *mailServiceError
	if errors.As(err, &failure) && failure.unknown {
		return "", refuseAutoresponder(transport.AutoresponderRefusedDovecotRejected,
			"CelikPanel could not confirm that Dovecot loaded the Sieve settings ("+boundedNativeDetail(err.Error())+")."+restored+" No mailbox was moved to Dovecot delivery and mail keeps arriving as before. Save the reply again; if this repeats, the server administrator reads `sudo doveadm log errors`.")
	}
	return "", refuseAutoresponder(transport.AutoresponderRefusedDovecotRejected,
		"Dovecot did not load the Sieve settings and keeps running with its previous ones ("+boundedNativeDetail(err.Error())+")."+restored+" No mailbox was moved to Dovecot delivery and mail keeps arriving as before. The server administrator fixes what Dovecot names (`sudo doveadm log errors`), then the reply is saved again.")
}

// setAutoresponderTransport adds or removes one mailbox's LMTP route. It
// sets transport_maps only when it is empty or already CelikPanel's.
func setAutoresponderTransport(ctx context.Context, email string, enabled bool) (string, error) {
	current, err := readMailFile(autoresponderTransportPath)
	if err != nil {
		return "", err
	}
	var next []byte
	if enabled {
		next = upsertMailLine(current, email, email+" "+autoresponderLMTPNexthop)
	} else {
		var removed bool
		next, removed = removeMailLine(current, email)
		if !removed {
			return "", nil
		}
	}
	value, err := autoresponderPostconfGet(ctx, "transport_maps")
	if err != nil {
		return "", fmt.Errorf("read Postfix transport_maps: %w", err)
	}
	setMaps, inUse := false, false
	switch {
	case transportMapsIncludesOurs(value):
		inUse = true
	case strings.TrimSpace(value) == "":
		setMaps = enabled
	default:
		if enabled {
			ours := autoresponderTransportValue(ctx)
			return "", refuseAutoresponder(transport.AutoresponderRefusedOwnerTransport,
				fmt.Sprintf(autoresponderOwnerTransportDetail, ours, ours))
		}
	}
	if bytes.Equal(current, next) && !setMaps {
		return "", nil
	}
	if err := applyMailFileMutation(ctx, []mailFileWrite{
		{path: autoresponderTransportPath, content: next, mode: 0o644},
	}, []string{autoresponderTransportPath}, nil); err != nil {
		return "", err
	}
	if setMaps {
		if err := autoresponderPostconfSet(ctx, "transport_maps", autoresponderTransportValue(ctx)); err != nil {
			// The table is written but unused: mail keeps its old delivery.
			return "", err
		}
	} else if !inUse {
		return "", nil
	}
	// Postfix's address resolver keeps the table it opened and notices a
	// change only after it has answered one more lookup, so the next mail
	// would still take the old route (lab run 5: a mailbox whose route was
	// just removed bounced once). Every change to a table in use is
	// followed by a reload.
	if _, err := autoresponderReloadPostfix(ctx); err != nil {
		if !enabled {
			return "Postfix did not confirm the reload (" + boundedNativeDetail(err.Error()) + "); the next message may still go to Dovecot until the server administrator reloads Postfix (`sudo systemctl reload postfix`).", nil
		}
		return "Postfix did not confirm the reload (" + boundedNativeDetail(err.Error()) + "); the reply starts with Postfix's next reload (`sudo systemctl reload postfix`).", nil
	}
	return "", nil
}

// mailboxExistsInVBox reports whether the mailbox is one Postfix delivers.
func mailboxExistsInVBox(email string) (bool, error) {
	content, err := readMailFile(postfixVBoxPath)
	if err != nil {
		return false, err
	}
	return mailFileHasKey(content, email), nil
}

func fillAutoresponderRefusal(resp *transport.MailAutoresponderResponse, err error) error {
	var refusal *autoresponderRefusal
	if errors.As(err, &refusal) {
		resp.Applied = false
		resp.Refusal = refusal.code
		resp.Detail = refusal.detail
		return nil
	}
	return err
}

// SetMailAutoresponder brings one mailbox's out-of-office reply to the
// requested setting. A refusal is an answer (Refusal + Detail), not an RPC
// error, so the page can say who acts.
func (a *Agent) SetMailAutoresponder(req *transport.SetMailAutoresponderRequest, resp *transport.MailAutoresponderResponse) error {
	if req == nil || resp == nil {
		return fmt.Errorf("autoresponder request and response are required")
	}
	*resp = transport.MailAutoresponderResponse{}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "setting an out-of-office reply"); err != nil {
		return err
	}
	email, local, domain, err := canonicalAgentMailbox(req.Email)
	if err != nil {
		return err
	}
	setting, err := transport.NormalizeMailAutoresponder(req.Autoresponder, email)
	if err != nil {
		return err
	}
	mailMutex.Lock()
	defer mailMutex.Unlock()
	ctx, cancel := newMailMutationContext()
	defer cancel()
	return fillAutoresponderRefusal(resp, applyMailAutoresponder(ctx, email, local, domain, setting, resp))
}

func applyMailAutoresponder(ctx context.Context, email, local, domain string, setting transport.MailAutoresponder, resp *transport.MailAutoresponderResponse) error {
	if !dovecotInstalledForRepair() || !autoresponderFileExists("/etc/postfix") {
		return refuseAutoresponder(transport.AutoresponderRefusedMailNotInstalled,
			"Mail is not set up on this server (Postfix and Dovecot), so there is nothing to answer mail. Install the mail service first.")
	}
	exists, err := mailboxExistsInVBox(email)
	if err != nil {
		return err
	}
	if !exists && setting.Enabled {
		return refuseAutoresponder(transport.AutoresponderRefusedNoMailbox,
			"This mailbox is not in the server's mail configuration. Save the mailbox again, then the reply.")
	}
	state, err := autoresponderStore.Read(domain, local)
	if err != nil {
		return fmt.Errorf("read the mailbox's Sieve script: %w", err)
	}
	resp.Script = classifyMailboxSieve(state)
	if resp.Script == transport.AutoresponderScriptOwner {
		return refuseAutoresponder(transport.AutoresponderRefusedOwnerScript, autoresponderOwnerScriptDetail)
	}
	if !setting.Enabled {
		// Delivery first: once the route is gone no script runs.
		warning, err := setAutoresponderTransport(ctx, email, false)
		if err != nil {
			return err
		}
		if resp.Script == transport.AutoresponderScriptManaged || state.ScriptExists || state.LinkKind == "link" {
			if err := autoresponderStore.Remove(domain, local); err != nil {
				return fmt.Errorf("remove CelikPanel's Sieve script: %w", err)
			}
		}
		resp.Script = transport.AutoresponderScriptNone
		resp.SieveAvailable = autoresponderSieveAvailable()
		resp.Warning = warning
		resp.Applied = true
		return nil
	}
	warning, err := ensureAutoresponderInfrastructure(ctx)
	if err != nil {
		return err
	}
	resp.SieveAvailable = true
	// Ask Dovecot's LMTP for the address before any mail goes there. A
	// mailbox that is already routed (saved before) loses its route and
	// CelikPanel's script when LMTP refuses it now: mail returns to
	// Postfix's own delivery instead of bouncing.
	if err := verifyAutoresponderLMTP(ctx, email); err != nil {
		reloadWarning, rollbackErr := rollbackAutoresponderRoute(ctx, email, domain, local, resp.Script)
		if rollbackErr != nil {
			return errors.Join(err, rollbackErr)
		}
		resp.Script = transport.AutoresponderScriptNone
		var refusal *autoresponderRefusal
		if reloadWarning != "" && errors.As(err, &refusal) {
			refusal.detail += " " + reloadWarning
		}
		return err
	}
	if err := autoresponderStore.Write(domain, local, []byte(renderAutoresponderSieve(setting))); err != nil {
		return fmt.Errorf("write the mailbox's Sieve script: %w", err)
	}
	transportWarning, err := setAutoresponderTransport(ctx, email, true)
	if err != nil {
		// Without the route the script is never run; leave no half state.
		if removeErr := autoresponderStore.Remove(domain, local); removeErr != nil {
			err = errors.Join(err, fmt.Errorf("remove the new Sieve script: %w", removeErr))
		}
		return err
	}
	resp.Script = transport.AutoresponderScriptManaged
	resp.Warning = joinWarning(warning, transportWarning)
	resp.Applied = true
	return nil
}

// verifyAutoresponderLMTP asks Dovecot's LMTP whether it accepts the
// address. Right after a reload an old LMTP process may still answer, so a
// refusal is asked again a few times before it counts.
func verifyAutoresponderLMTP(ctx context.Context, email string) error {
	var last error
	for attempt := 0; attempt < autoresponderProbeAttempts; attempt++ {
		if attempt > 0 {
			autoresponderSleep(time.Second)
		}
		last = autoresponderLMTPProbe(ctx, email)
		if last == nil {
			return nil
		}
		if ctx.Err() != nil {
			break
		}
	}
	return refuseAutoresponder(transport.AutoresponderRefusedLMTPRejected,
		fmt.Sprintf(autoresponderLMTPRejectedDetail, email, boundedNativeDetail(last.Error())))
}

// rollbackAutoresponderRoute returns the mailbox to Postfix's own delivery:
// its LMTP route goes first, then CelikPanel's script (never the owner's).
func rollbackAutoresponderRoute(ctx context.Context, email, domain, local, script string) (string, error) {
	warning, err := setAutoresponderTransport(ctx, email, false)
	if err != nil {
		return "", fmt.Errorf("remove the mailbox's LMTP route: %w", err)
	}
	if script == transport.AutoresponderScriptManaged {
		if err := autoresponderStore.Remove(domain, local); err != nil {
			return "", fmt.Errorf("remove CelikPanel's Sieve script: %w", err)
		}
	}
	return warning, nil
}

// MailAutoresponderState reads one mailbox's Sieve state (read only).
// Applied is true when the server state is whole: CelikPanel's script is
// active and the mailbox is routed to LMTP, or neither is. A script
// without its route (or a route without the script) is false.
func (a *Agent) MailAutoresponderState(req *transport.MailAutoresponderStateRequest, resp *transport.MailAutoresponderResponse) error {
	if req == nil || resp == nil {
		return fmt.Errorf("autoresponder state request and response are required")
	}
	*resp = transport.MailAutoresponderResponse{}
	email, local, domain, err := canonicalAgentMailbox(req.Email)
	if err != nil {
		return err
	}
	resp.SieveAvailable = autoresponderSieveAvailable()
	state, err := autoresponderStore.Read(domain, local)
	if err != nil {
		return fmt.Errorf("read the mailbox's Sieve script: %w", err)
	}
	resp.Script = classifyMailboxSieve(state)
	routed, err := autoresponderRouted(email)
	if err != nil {
		return fmt.Errorf("read the mailbox's mail route: %w", err)
	}
	switch resp.Script {
	case transport.AutoresponderScriptManaged:
		resp.Applied = routed
	case transport.AutoresponderScriptNone:
		resp.Applied = !routed
	}
	if snapshot, err := snapshotMailFile(autoresponderDovecotConfPath); err == nil && snapshot.exists &&
		!digestHeaderIntact(snapshot.content, autoresponderDovecotHead) {
		resp.Refusal = transport.AutoresponderRefusedOwnerDovecotConf
		resp.Detail = autoresponderOwnerDovecotDetail
	}
	if resp.Script == transport.AutoresponderScriptOwner {
		resp.Refusal = transport.AutoresponderRefusedOwnerScript
		resp.Detail = autoresponderOwnerScriptDetail
	}
	return nil
}
