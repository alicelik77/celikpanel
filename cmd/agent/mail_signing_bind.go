package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"
)

// Mail is signed only for the sender's own domain (20 Nov 2026, tenant
// isolation, lane S3 part 3).
//
// Before: OpenDKIM signed every message whose From: named a hosted domain, if
// the client was authenticated or connected from loopback, and it sat on
// every smtpd, port 25 included. So a mailbox could send with another
// customer's address in From: over submission, and a site's own code could
// hand a message to port 25 over loopback, and either left this server with
// that customer's DKIM signature.
//
// Now, with native settings only:
//
//  1. OpenDKIM chooses the key by the SASL login (SenderMacro {auth_authen},
//     dkim_signing_rpc.go). Submission requires a login, so its mail is
//     signed for the login's own domain or not at all. Mail made on the
//     server without SMTP (sendmail of root and vmail only, Postfix's
//     authorized_submit_users) has no login and is signed by its From: as
//     before: the panel's notices, out-of-office replies.
//  2. The smtpd that receives mail on port 25 (smtp/inet, or smtpd/pass
//     behind postscreen) gets its own `-o smtpd_milters=` without OpenDKIM:
//     the spam filter stays there, and nothing that arrives on port 25,
//     from outside or from loopback, is signed by this server. main.cf keeps
//     OpenDKIM, so submission and smtps (and any smtpd the owner added) are
//     signed as before.
//
// An opendkim.conf or a port-25 milter setting the owner changed is kept as he
// wrote it (D-022): CelikPanel's opendkim.conf is put beside his file as
// <file>.celikpanel-pending and the state is reported where the administrator
// sees problems (Monitoring → Problems now). A new configuration is checked
// before it is used (`opendkim -n`, `postfix check`), and a restart or reload
// that does not take is undone, so mail keeps flowing as before (D-024).
//
// Posta yalnız göndericinin kendi alan adıyla imzalanır: anahtar girişe göre
// seçilir, 25. kapıdaki smtpd artık imzalamaz. Sahibin değiştirdiği dosya
// korunur; yeni yapılandırma kullanılmadan önce denetlenir, tutmazsa geri alınır.

// / Signing states (transport.MailSigningState.State).
const (
	mailSigningBound       = "bound"       // both parts are in place
	mailSigningNoDKIM      = "no_dkim"     // OpenDKIM is not configured here
	mailSigningConfKept    = "conf_kept"   // the owner's opendkim.conf was kept
	mailSigningPort25Kept  = "port25_kept" // the owner's port-25 milter setting was kept
	mailSigningFailed      = "failed"      // a change was refused or undone
	mailSigningUnsupported = "unsupported" // this OpenDKIM build has no SenderMacro
)

// opendkimPendingSuffix names CelikPanel's text held beside an owner's file.
const opendkimPendingSuffix = ".celikpanel-pending"

// opendkimCandidateSuffix is the checked-before-use copy of a new text.
const opendkimCandidateSuffix = ".celikpanel-new"

// Seams for the file operations (production: the secure helpers).
var (
	opendkimConfRead   = secureReadConfig
	opendkimConfWrite  = secureWriteConfig
	opendkimConfRemove = os.Remove
	opendkimConfRename = os.Rename
	// opendkimProbeDir holds the support probe's throw-away file.
	opendkimProbeDir = os.TempDir
)

// openDKIMSupportsSenderMacro asks the installed OpenDKIM whether it knows
// SenderMacro. It is an experimental option compiled in only with
// _FFR_SENDER_MACRO (opendkim/opendkim-config.h); a build without it refuses
// the line as an "unrecognized parameter" (opendkim/config.c) and does not
// start. Measured 20 Nov 2026 in the packages: Debian 11, 12 and 13 and
// Ubuntu's 2.11.0 builds (24.04: 2.11.0~beta2-9build4) have it; Arch's
// opendkim 2.11.0beta2-2 has neither it nor Lua. `opendkim -n` parses a
// configuration and exits (opendkim(8)); the probe's file has no key table,
// so nothing but the option itself is judged. ok=false with known=false: the
// probe could not be run, which is not taken for a refusal.
func openDKIMSupportsSenderMacro(run postconfRunner) (ok bool, known bool) {
	probe, err := os.CreateTemp(opendkimProbeDir(), "celikpanel-opendkim-probe-*.conf")
	if err != nil {
		return false, false
	}
	name := probe.Name()
	defer os.Remove(name)
	_, writeErr := probe.WriteString("Mode\t\t\tv\nSyslog\t\t\tno\nSocket\t\t\t" + opendkimSocket + "\nSenderMacro\t\t" + openDKIMSenderMacro + "\n")
	if errors.Join(writeErr, probe.Close()) != nil {
		return false, false
	}
	out, err := run("opendkim", "-n", "-x", name)
	if err == nil {
		return true, true
	}
	if strings.Contains(string(out), "unrecognized parameter") {
		return false, true
	}
	return false, false
}

// desiredOpenDKIMConf is CelikPanel's text for this host: with SenderMacro
// when the installed OpenDKIM knows it, the earlier text otherwise.
func desiredOpenDKIMConf(layout opendkimLayout, senderMacro bool) []byte {
	return []byte(renderOpenDKIMConfText(layout, opendkimSocket, senderMacro))
}

// openDKIMConfKind classifies the file on disk: "absent", "current" (the text
// wanted), "earlier" (another text a CelikPanel release writes: the earlier
// releases' with either socket name, or this release's other form) or
// "owner" (anything else).
func openDKIMConfKind(content []byte, exists bool, layout opendkimLayout, want []byte) string {
	if !exists {
		return "absent"
	}
	if bytes.Equal(content, want) {
		return "current"
	}
	for _, text := range []string{
		renderOpenDKIMConf(layout),
		renderOpenDKIMConfWithSocket(layout, opendkimSocket),
		renderOpenDKIMConfWithSocket(layout, legacyOpenDKIMSocket),
	} {
		if bytes.Equal(content, []byte(text)) {
			return "earlier"
		}
	}
	return "owner"
}

// readOpenDKIMConf reads the file; exists=false when it is not there.
func readOpenDKIMConf(path string) ([]byte, bool, error) {
	content, err := opendkimConfRead(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return content, true, nil
}

// unsupportedNote says what an OpenDKIM without SenderMacro means.
func unsupportedNote() string {
	return "The installed OpenDKIM does not know the SenderMacro option (it is compiled in only by some distributions; Arch Linux's package lacks it), " +
		"so mail sent with a mailbox's login is still signed for any hosted domain its From: names. Mail that arrives on port 25 is not signed any more. " +
		"Nothing else is needed for mail to flow"
}

// reconcileOpenDKIMConf brings an earlier release's opendkim.conf to this
// release's text and keeps an owner's file. It runs at every Panel start
// (Agent.WireMailFilters) and returns the state and a note (empty when
// nothing needed saying).
//
// Order, so that no state loses mail: the new text is written beside the file
// and checked with `opendkim -n` first (a refusal changes nothing); only then
// is it renamed over the file and OpenDKIM restarted, if it was running; when
// OpenDKIM is not active afterwards the earlier file is put back and OpenDKIM
// restarted with it. While OpenDKIM restarts, Postfix passes mail on unsigned
// (milter_default_action = accept), never bounces or holds it.
func reconcileOpenDKIMConf(run postconfRunner, layout opendkimLayout) (string, string) {
	path := layout.confPath
	content, exists, err := readOpenDKIMConf(path)
	if err != nil {
		return mailSigningFailed, fmt.Sprintf("%s could not be read (%v); nothing was changed", path, err)
	}
	if !exists {
		return mailSigningNoDKIM, ""
	}
	supported, known := openDKIMSupportsSenderMacro(run)
	if !known {
		return mailSigningFailed, fmt.Sprintf("whether the installed OpenDKIM knows SenderMacro could not be checked (opendkim -n did not run); %s was not changed", path)
	}
	want := desiredOpenDKIMConf(layout, supported)
	state := mailSigningBound
	note := ""
	if !supported {
		state, note = mailSigningUnsupported, unsupportedNote()
	}
	switch openDKIMConfKind(content, exists, layout, want) {
	case "current":
		return state, note
	case "owner":
		return mailSigningConfKept, holdOpenDKIMPending(path, layout)
	}
	if refusal := checkOpenDKIMCandidate(run, path, want); refusal != "" {
		return mailSigningFailed, refusal
	}
	active := strings.TrimSpace(string(firstOutput(run("systemctl", "is-active", "opendkim")))) == "active"
	if err := opendkimConfRename(path+opendkimCandidateSuffix, path); err != nil {
		_ = opendkimConfRemove(path + opendkimCandidateSuffix)
		return mailSigningFailed, fmt.Sprintf("CelikPanel's new %s could not be put in place (%v); the earlier one stays in use", path, err)
	}
	if !active || openDKIMRestarted(run) {
		// A stopped OpenDKIM reads the file when it is started.
		return state, note
	}
	// Undo: the earlier text back, OpenDKIM restarted with it.
	restoreErr := opendkimConfWrite(path, content, 0o644)
	if restoreErr == nil && openDKIMRestarted(run) {
		return mailSigningFailed, fmt.Sprintf("OpenDKIM did not start with CelikPanel's new %s, so the earlier file was put back and OpenDKIM runs with it; mail is signed as before. "+
			"The server owner can read why with: journalctl -u opendkim", path)
	}
	return mailSigningFailed, fmt.Sprintf("OpenDKIM did not start with CelikPanel's new %s, and starting it again with the earlier file did not succeed either (%v): mail is delivered unsigned until OpenDKIM runs. "+
		"The server owner reads why with: journalctl -u opendkim", path, restoreErr)
}

// checkOpenDKIMCandidate writes next beside path and asks OpenDKIM whether it
// would start with it; "" when it would. The candidate stays for the rename on
// success and is removed on a refusal.
func checkOpenDKIMCandidate(run postconfRunner, path string, next []byte) string {
	candidate := path + opendkimCandidateSuffix
	if err := opendkimConfWrite(candidate, next, 0o644); err != nil {
		return fmt.Sprintf("CelikPanel's new %s could not be written beside it (%v); the earlier one stays in use", path, err)
	}
	if out, err := run("opendkim", "-n", "-x", candidate); err != nil {
		_ = opendkimConfRemove(candidate)
		detail := firstLine(strings.TrimSpace(string(out)))
		if detail == "" {
			detail = err.Error()
		}
		return fmt.Sprintf("OpenDKIM refused CelikPanel's new configuration (%s); %s was not changed and stays in use", detail, path)
	}
	return ""
}

// openDKIMRestarted restarts OpenDKIM and says whether it is active after.
func openDKIMRestarted(run postconfRunner) bool {
	if _, err := run("systemctl", "restart", "opendkim"); err != nil {
		return false
	}
	return strings.TrimSpace(string(firstOutput(run("systemctl", "is-active", "opendkim")))) == "active"
}

// holdOpenDKIMPending puts CelikPanel's text beside an owner's file and says
// so. The owner's file is not read by anything of CelikPanel's after this.
func holdOpenDKIMPending(path string, layout opendkimLayout) string {
	pending := path + opendkimPendingSuffix
	next := []byte(renderOpenDKIMConf(layout))
	if current, exists, err := readOpenDKIMConf(pending); err != nil || !exists || !bytes.Equal(current, next) {
		if err := opendkimConfWrite(pending, next, 0o644); err != nil {
			return fmt.Sprintf("%s was changed by the server owner and kept as it is; CelikPanel's text could not be put beside it (%v)", path, err)
		}
	}
	return fmt.Sprintf("%s was changed by the server owner and kept as it is, so OpenDKIM may still sign mail for a domain other than the sender's own. "+
		"CelikPanel's text is beside it in %s: its SenderMacro %s line makes OpenDKIM sign submitted mail only for the sender's own domain. "+
		"The server owner adds that line to his file, or replaces his file with CelikPanel's, and restarts OpenDKIM", path, pending, openDKIMSenderMacro)
}

// firstOutput keeps a command's output and drops its error (the caller judges
// by the output).
func firstOutput(out []byte, _ error) []byte { return out }

// port25SMTPDEntries are the master.cf entries that receive mail on port 25:
// smtp/inet when it runs smtpd itself, smtpd/pass when postscreen is in front.
func port25SMTPDEntries(run postconfRunner) []string {
	var entries []string
	for _, entry := range []string{"smtp/inet", "smtpd/pass"} {
		if fields, ok := masterServiceFields(run, entry); ok {
			if command := strings.Fields(fields["command"]); len(command) > 0 && command[0] == "smtpd" {
				entries = append(entries, entry)
			}
		}
	}
	return entries
}

// masterServiceParam reads one -o parameter of a master.cf entry; set=false
// when the entry does not set it (it then follows main.cf).
func masterServiceParam(run postconfRunner, entry, name string) (string, bool, error) {
	key := entry + "/" + name
	out, err := run("postconf", "-P", key)
	if err != nil {
		return "", false, fmt.Errorf("postconf -P %s: %s", key, firstLine(strings.TrimSpace(string(out))))
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if rest, ok := strings.CutPrefix(line, key); ok {
			rest = strings.TrimSpace(rest)
			if value, ok := strings.CutPrefix(rest, "="); ok {
				return strings.TrimSpace(value), true, nil
			}
		}
	}
	return "", false, nil
}

// port25MilterChange is one planned port-25 entry change and how to undo it.
type port25MilterChange struct {
	entry     string
	want      string
	before    string
	beforeSet bool
}

// planPort25Milters decides the port-25 entries' own milter list: the spam
// filter only (or none). An entry that already sets a list the owner wrote
// (milterListPanelComposed false) is kept and named.
func planPort25Milters(run postconfRunner, want, spam string) ([]port25MilterChange, []string, error) {
	var changes []port25MilterChange
	var kept []string
	for _, entry := range port25SMTPDEntries(run) {
		value, set, err := masterServiceParam(run, entry, "smtpd_milters")
		if err != nil {
			return nil, nil, err
		}
		if set && value == want {
			continue
		}
		if set && !milterListPanelComposed(value, spam) {
			kept = append(kept, fmt.Sprintf("%s smtpd_milters = %s", entry, value))
			continue
		}
		changes = append(changes, port25MilterChange{entry: entry, want: want, before: value, beforeSet: set})
	}
	return changes, kept, nil
}

// applyPort25Milters writes the planned values (postconf -P, master.cf).
func applyPort25Milters(run postconfRunner, changes []port25MilterChange) error {
	for _, change := range changes {
		key := change.entry + "/smtpd_milters"
		if out, err := run("postconf", "-P", key+"="+change.want); err != nil {
			return fmt.Errorf("postconf -P %s: %s", key, firstLine(strings.TrimSpace(string(out))))
		}
	}
	return nil
}

// undoPort25Milters puts every planned entry back as it was.
func undoPort25Milters(run postconfRunner, changes []port25MilterChange) error {
	var errs []error
	for _, change := range changes {
		key := change.entry + "/smtpd_milters"
		args := []string{"-PX", key}
		if change.beforeSet {
			args = []string{"-P", key + "=" + change.before}
		}
		if out, err := run("postconf", args...); err != nil {
			errs = append(errs, fmt.Errorf("postconf %s: %s", strings.Join(args, " "), firstLine(strings.TrimSpace(string(out)))))
		}
	}
	return errors.Join(errs...)
}

// port25KeptNote is the D-024 note for an owner's port-25 milter setting.
func port25KeptNote(kept []string) string {
	if len(kept) == 0 {
		return ""
	}
	return "The milter setting of the smtpd that receives mail on port 25 was changed by the server owner and kept as it is (" +
		strings.Join(kept, "; ") + "). If it names OpenDKIM (" + opendkimMilter + "), mail that arrives on port 25, also from this server's own sites over loopback, " +
		"can still leave signed for a hosted domain. The server owner removes OpenDKIM from that list (postconf -P), or removes the setting so CelikPanel writes it (postconf -PX), and restarts the panel"
}
