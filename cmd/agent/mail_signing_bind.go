package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Mail is signed only for the sender's own domain (20 Nov 2026, tenant
// isolation, lane S3 part 3; corrected 21 Nov 2026 after the independent
// review of alpha.143).
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
//     server without SMTP has no login and is signed by its From:. Only root
//     and vmail may hand mail over that way (Postfix's
//     authorized_submit_users, mail_passwordless_enforce.go); what they send
//     carries a From: of the account it belongs to: the panel's notices
//     (no-reply@ of the owner's main domain) and out-of-office replies (the
//     mailbox's own address, or an additional address that is forwarded to
//     the mailbox from its own account's domains,
//     transport.ForeignAutoresponderAddresses).
//     Where authorized_submit_users is not limited to root and vmail, sites'
//     programs can hand over mail signed for any hosted domain, and the state
//     says so (submit_open) instead of "in place".
//  2. The smtpd that receives mail on port 25 (smtp/inet, or smtpd/pass
//     behind postscreen) gets its own `-o smtpd_milters=`: main.cf's list as
//     it is at every start, the server owner's filters included, with only
//     OpenDKIM left out. Nothing that arrives on port 25, from outside or
//     from loopback, is signed by this server. The setting is removed again
//     when main.cf's list has no OpenDKIM (OpenDKIM removed). main.cf keeps
//     OpenDKIM, so submission and smtps are signed as before.
//
// An opendkim.conf or a port-25 milter setting the owner changed is kept as he
// wrote it (D-022) and reported where the administrator sees problems
// (Monitoring → Problems now), with the concrete step. A new configuration is
// checked before it is used (`opendkim -n`, `postfix check`), a restart or
// reload that does not take is undone, and a restart that is owed (the file
// is in place, the running OpenDKIM may not have read it) is recorded before
// the file is put in place and finished at the next start (D-024, D-025).
//
// Posta yalnız göndericinin kendi alan adıyla imzalanır: anahtar girişe göre
// seçilir, 25. kapıdaki smtpd artık imzalamaz. Sahibin değiştirdiği dosya
// korunur; yeni yapılandırma kullanılmadan önce denetlenir, tutmazsa geri alınır;
// borçlu kalan yeniden başlatma kaydedilir ve sonraki açılışta tamamlanır.

// / Signing states (transport.MailSigningState.State).
const (
	mailSigningBound       = "bound"       // both parts are in place
	mailSigningNoDKIM      = "no_dkim"     // OpenDKIM is not configured here
	mailSigningConfKept    = "conf_kept"   // the owner's opendkim.conf was kept
	mailSigningPort25Kept  = "port25_kept" // the owner's port-25 milter setting was kept
	mailSigningFailed      = "failed"      // a change was refused or undone, or OpenDKIM did not run again
	mailSigningUnsupported = "unsupported" // this OpenDKIM build has no SenderMacro
	// mailSigningSubmitOpen: authorized_submit_users lets other accounts than
	// root and vmail hand mail over without a login (21 Nov 2026).
	mailSigningSubmitOpen = "submit_open"
	// mailSigningUnknown: the signing step did not run or its result is not
	// known; never shown as "in place" (21 Nov 2026).
	mailSigningUnknown = "unknown"
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
	// opendkimRestartOwedPath records that an opendkim.conf was put in place
	// and the running OpenDKIM has not yet been verified restarted with it
	// (21 Nov 2026, review finding 4). Written before the file is put in
	// place, removed only after a verified restart (or when OpenDKIM is not
	// running, which reads the file when it starts).
	opendkimRestartOwedPath = "/var/lib/celikpanel-agent-private/opendkim-restart-owed"
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

// unsupportedNote says what an OpenDKIM without SenderMacro means (review
// finding 6: such a system is never told to set the option).
func unsupportedNote() string {
	return "The installed OpenDKIM cannot choose the signing key by the mailbox's login (its build has no SenderMacro option; Arch Linux's package lacks it), " +
		"so on this server mail sent with a mailbox's password is still signed by its From: header, and a mailbox can send mail signed for another hosted domain. " +
		"Mail that arrives on port 25 is not signed any more. Mail is delivered; nothing set on this server changes this until the distribution's OpenDKIM has the option"
}

// openDKIMRestartIsOwed reports whether a restart was recorded as owed.
func openDKIMRestartIsOwed() bool {
	_, err := os.Stat(opendkimRestartOwedPath)
	return err == nil
}

// markOpenDKIMRestartOwed records the owed restart before a file is put in
// place; the record names the file.
func markOpenDKIMRestartOwed(path string) error {
	if err := os.MkdirAll(filepath.Dir(opendkimRestartOwedPath), 0o700); err != nil {
		return err
	}
	return os.WriteFile(opendkimRestartOwedPath, []byte(path+"\n"), 0o600)
}

// clearOpenDKIMRestartOwed removes the record (absent is fine).
func clearOpenDKIMRestartOwed() {
	if err := os.Remove(opendkimRestartOwedPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		// The next start restarts OpenDKIM once more; nothing else follows.
		return
	}
}

// openDKIMRunning reports whether a restart is needed for OpenDKIM to read a
// new file: anything but "inactive" or "failed" (active, reloading,
// activating, or an answer that could not be read) may be a process running
// with the earlier file (review finding 4: "reloading" used to skip the
// restart).
func openDKIMRunning(run postconfRunner) bool {
	state := strings.TrimSpace(string(firstOutput(run("systemctl", "is-active", "opendkim"))))
	return state != "inactive" && state != "failed"
}

// milterDefaultActionNote says what happens to mail that passes OpenDKIM while
// it restarts (review finding 7). Where the milter lists are CelikPanel's,
// milter_default_action = accept is ensured before (the milter chain is
// written first): mail is passed on unsigned. Where the server owner keeps
// his own list, his milter_default_action applies and is not changed.
func milterDefaultActionNote(run postconfRunner) string {
	out, err := run("postconf", "-h", "milter_default_action")
	value, known := postconfOneValue(out)
	if err != nil || !known {
		return ""
	}
	what := ""
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "accept":
		return ""
	case "tempfail":
		what = "answered with a temporary error, so senders send it again"
	case "reject":
		what = "refused, so senders get it back as undeliverable"
	case "quarantine":
		what = "kept in Postfix's hold queue until the server owner releases it (postsuper -H ALL)"
	default:
		what = "handled as milter_default_action = " + value + " says"
	}
	return "While OpenDKIM restarted (a few seconds), mail sent with a mailbox's password and mail made on this server was " + what +
		": the server owner's milter_default_action (" + value + ") applies because his own milter list is kept"
}

// reconcileOpenDKIMConf brings an earlier release's opendkim.conf to this
// release's text and keeps an owner's file. It runs at every Panel start
// (Agent.WireMailFilters) and returns the state and a note (empty when
// nothing needed saying).
//
// Order, so that no state loses mail: the new text is written beside the file
// and checked with `opendkim -n` first (a refusal changes nothing); the owed
// restart is recorded; only then is the text renamed over the file and
// OpenDKIM restarted and verified; when OpenDKIM is not active afterwards the
// earlier file is put back and OpenDKIM restarted with it. A restart recorded
// as owed by an earlier start that did not finish (killed Agent, power loss,
// a status other than "active") is made now, whatever the file says.
func reconcileOpenDKIMConf(run postconfRunner, layout opendkimLayout) (string, string) {
	path := layout.confPath
	content, exists, err := readOpenDKIMConf(path)
	if err != nil {
		return mailSigningUnknown, fmt.Sprintf("%s could not be read (%v); nothing was changed", path, err)
	}
	if !exists {
		clearOpenDKIMRestartOwed()
		return mailSigningNoDKIM, ""
	}
	supported, known := openDKIMSupportsSenderMacro(run)
	if !known {
		return mailSigningUnknown, fmt.Sprintf("whether the installed OpenDKIM knows SenderMacro could not be checked (opendkim -n did not run); %s was not changed", path)
	}
	want := desiredOpenDKIMConf(layout, supported)
	state := mailSigningBound
	note := ""
	if !supported {
		state, note = mailSigningUnsupported, unsupportedNote()
	}
	switch openDKIMConfKind(content, exists, layout, want) {
	case "current":
		if openDKIMRestartIsOwed() {
			return finishOwedOpenDKIMRestart(run, path, state, note)
		}
		return state, note
	case "owner":
		clearOpenDKIMRestartOwed()
		return holdOpenDKIMPending(path, layout, supported)
	}
	if refusal := checkOpenDKIMCandidate(run, path, want); refusal != "" {
		return mailSigningFailed, refusal
	}
	if err := markOpenDKIMRestartOwed(path); err != nil {
		_ = opendkimConfRemove(path + opendkimCandidateSuffix)
		return mailSigningFailed, fmt.Sprintf("CelikPanel could not record the OpenDKIM restart it owes (%v), so %s was not changed and stays in use", err, path)
	}
	running := openDKIMRunning(run)
	if err := opendkimConfRename(path+opendkimCandidateSuffix, path); err != nil {
		_ = opendkimConfRemove(path + opendkimCandidateSuffix)
		clearOpenDKIMRestartOwed()
		return mailSigningFailed, fmt.Sprintf("CelikPanel's new %s could not be put in place (%v); the earlier one stays in use", path, err)
	}
	if !running {
		// A stopped OpenDKIM reads the file when it is started.
		clearOpenDKIMRestartOwed()
		return state, note
	}
	window := milterDefaultActionNote(run)
	if openDKIMRestarted(run) {
		clearOpenDKIMRestartOwed()
		return state, joinNotes(note, window)
	}
	// Undo: the earlier text back, OpenDKIM restarted with it.
	restoreErr := opendkimConfWrite(path, content, 0o644)
	if restoreErr == nil && openDKIMRestarted(run) {
		clearOpenDKIMRestartOwed()
		return mailSigningFailed, fmt.Sprintf("OpenDKIM did not start with CelikPanel's new %s, so the earlier file was put back and OpenDKIM runs with it; mail is signed as before", path)
	}
	// The record stays: the next start tries again.
	return mailSigningFailed, fmt.Sprintf("OpenDKIM did not start with CelikPanel's new %s, and starting it again with the earlier file did not succeed either (%v): mail is delivered unsigned until OpenDKIM runs", path, restoreErr)
}

// finishOwedOpenDKIMRestart makes the restart an earlier start owed: the file
// is CelikPanel's current text, the running OpenDKIM may still have the
// earlier one.
func finishOwedOpenDKIMRestart(run postconfRunner, path, state, note string) (string, string) {
	if !openDKIMRunning(run) {
		clearOpenDKIMRestartOwed()
		return state, note
	}
	window := milterDefaultActionNote(run)
	if openDKIMRestarted(run) {
		clearOpenDKIMRestartOwed()
		return state, joinNotes("OpenDKIM was restarted to load "+path+", which an earlier start had put in place without a confirmed restart", note, window)
	}
	return mailSigningFailed, fmt.Sprintf("OpenDKIM did not run again after CelikPanel restarted it to load %s: mail is delivered unsigned until OpenDKIM runs, and the next panel start tries again", path)
}

// observeOpenDKIMConf reads, without changing anything, what the OpenDKIM
// configuration means for signing: the state after a step that wrote it
// (ConfigureDKIMSigning) or did not touch it (ConfigureMailStack).
func observeOpenDKIMConf(run postconfRunner, layout opendkimLayout) (string, string) {
	content, exists, err := readOpenDKIMConf(layout.confPath)
	if err != nil {
		return mailSigningUnknown, fmt.Sprintf("%s could not be read (%v)", layout.confPath, err)
	}
	if !exists {
		return mailSigningNoDKIM, ""
	}
	supported, known := openDKIMSupportsSenderMacro(run)
	if !known {
		return mailSigningUnknown, "whether the installed OpenDKIM knows SenderMacro could not be checked (opendkim -n did not run)"
	}
	switch openDKIMConfKind(content, exists, layout, desiredOpenDKIMConf(layout, supported)) {
	case "owner":
		if !supported {
			return mailSigningUnsupported, unsupportedNote()
		}
		return mailSigningConfKept, ownerOpenDKIMNote(layout.confPath)
	case "earlier":
		return mailSigningUnknown, layout.confPath + " is an earlier release's text; the next panel start puts this release's in place"
	}
	if openDKIMRestartIsOwed() {
		return mailSigningFailed, "OpenDKIM has not been confirmed restarted with " + layout.confPath + "; the next panel start restarts it"
	}
	if !supported {
		return mailSigningUnsupported, unsupportedNote()
	}
	return mailSigningBound, ""
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

// ownerOpenDKIMNote is the note for an owner's opendkim.conf on an OpenDKIM
// that knows SenderMacro.
func ownerOpenDKIMNote(path string) string {
	return fmt.Sprintf("%s was changed by the server owner and kept as it is, so OpenDKIM may still sign mail for a domain other than the sender's own. "+
		"The server owner adds the line \"SenderMacro %s\" to that file (CelikPanel's whole text is beside it in %s%s) and restarts OpenDKIM",
		path, openDKIMSenderMacro, path, opendkimPendingSuffix)
}

// holdOpenDKIMPending keeps an owner's file. Where the installed OpenDKIM
// knows SenderMacro, CelikPanel's text is put beside it and the state is
// conf_kept. Where it does not (review finding 6), nothing is put beside it
// (a text the owner's OpenDKIM would refuse is never offered), a pending
// text an earlier candidate left there is removed, and the state is
// unsupported. The owner's file is not read by anything of CelikPanel's.
func holdOpenDKIMPending(path string, layout opendkimLayout, supported bool) (string, string) {
	pending := path + opendkimPendingSuffix
	current, exists, err := readOpenDKIMConf(pending)
	if !supported {
		if err == nil && exists && openDKIMConfKind(current, true, layout, nil) != "owner" {
			_ = opendkimConfRemove(pending)
		}
		return mailSigningUnsupported, unsupportedNote() + "; " + path + " was changed by the server owner and is kept as it is"
	}
	next := []byte(renderOpenDKIMConf(layout))
	if err != nil || !exists || !bytes.Equal(current, next) {
		if err := opendkimConfWrite(pending, next, 0o644); err != nil {
			return mailSigningConfKept, fmt.Sprintf("%s was changed by the server owner and kept as it is; CelikPanel's text could not be put beside it (%v). "+
				"The server owner adds the line \"SenderMacro %s\" to that file and restarts OpenDKIM", path, err, openDKIMSenderMacro)
		}
	}
	return mailSigningConfKept, ownerOpenDKIMNote(path)
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

// Port-25 record (21 Nov 2026, review finding 3): what CelikPanel last wrote
// into each port-25 entry, so that its own value is recognised (and rewritten
// or removed) after the owner's main.cf list changed, while a value the owner
// wrote is kept.
var (
	mailPort25RecordPath = "/var/lib/celikpanel-agent-private/mail-port25-milters.json"
	readPort25Record     = func() (map[string]string, error) {
		raw, err := os.ReadFile(mailPort25RecordPath)
		if errors.Is(err, os.ErrNotExist) {
			return map[string]string{}, nil
		}
		if err != nil {
			return nil, err
		}
		record := map[string]string{}
		if err := json.Unmarshal(raw, &record); err != nil {
			return nil, fmt.Errorf("CelikPanel's record %s is not readable: %w", mailPort25RecordPath, err)
		}
		return record, nil
	}
	writePort25Record = func(record map[string]string) error {
		if len(record) == 0 {
			if err := os.Remove(mailPort25RecordPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			return nil
		}
		raw, err := json.MarshalIndent(record, "", "  ")
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(mailPort25RecordPath), 0o700); err != nil {
			return err
		}
		tmp := mailPort25RecordPath + ".tmp"
		if err := os.WriteFile(tmp, append(raw, '\n'), 0o600); err != nil {
			return err
		}
		return os.Rename(tmp, mailPort25RecordPath)
	}
)

// port25MilterChange is one planned port-25 entry change and how to undo it.
// remove: the entry's own setting is removed (postconf -PX), so it follows
// main.cf again.
type port25MilterChange struct {
	entry     string
	want      string
	remove    bool
	before    string
	beforeSet bool
}

// milterEntries splits a Postfix milter list.
func milterEntries(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	})
}

// isOpenDKIMMilter reports one of the names CelikPanel gives OpenDKIM.
func isOpenDKIMMilter(entry string) bool {
	return entry == opendkimMilter || entry == legacyOpenDKIMMilter
}

// milterListWithoutOpenDKIM is main.cf's list with only OpenDKIM left out, in
// the owner's order (review finding 3: the owner's filters stay on port 25),
// joined by commas without spaces: a master.cf -o value may not contain
// whitespace (postconf -P refuses it, measured with the release's postconf).
// named is true when the list names OpenDKIM; ok is false for a list whose
// entries CelikPanel cannot read one by one ($variables, {} groups).
func milterListWithoutOpenDKIM(value string) (rest string, named bool, ok bool) {
	if strings.ContainsAny(value, "${}") {
		return "", false, false
	}
	var kept []string
	for _, entry := range milterEntries(value) {
		if isOpenDKIMMilter(entry) {
			named = true
			continue
		}
		kept = append(kept, entry)
	}
	return strings.Join(kept, ","), named, true
}

// port25Plan is what the port-25 step changes and what it found.
type port25Plan struct {
	changes []port25MilterChange
	// owner: entries whose own setting the owner wrote and which name
	// OpenDKIM (or cannot be read), kept as they are and reported.
	owner []string
	// variable: main.cf's list cannot be read entry by entry, so port 25 was
	// not changed.
	variable bool
	record   map[string]string
}

// planPort25Milters decides each port-25 entry's own milter list from
// main.cf's list as it will be after this step (mainList): that list without
// OpenDKIM while OpenDKIM is configured and main.cf names it; otherwise no
// own setting at all. CelikPanel's own value (the record, or a list only of
// entries a CelikPanel release writes) is rewritten or removed; a value the
// owner wrote is kept, and reported when it names OpenDKIM.
func planPort25Milters(run postconfRunner, mainList string, hasDKIM bool, spam string, record map[string]string) (port25Plan, error) {
	plan := port25Plan{record: map[string]string{}}
	for entry, value := range record {
		plan.record[entry] = value
	}
	want, named, readable := milterListWithoutOpenDKIM(mainList)
	need := hasDKIM && named
	for _, entry := range port25SMTPDEntries(run) {
		value, set, err := masterServiceParam(run, entry, "smtpd_milters")
		if err != nil {
			return port25Plan{}, err
		}
		recorded, hasRecord := record[entry]
		ours := set && ((hasRecord && value == recorded) || milterListPanelComposed(value, spam))
		if set && !ours {
			// The owner's own setting: kept as he wrote it.
			delete(plan.record, entry)
			if _, ownerNamed, ownerReadable := milterListWithoutOpenDKIM(value); ownerNamed || !ownerReadable {
				plan.owner = append(plan.owner, fmt.Sprintf("%s smtpd_milters = %s", entry, value))
			}
			continue
		}
		if hasDKIM && !readable {
			// main.cf's list hides its entries behind a variable: port 25 is
			// not changed and the state says so.
			plan.variable = true
			continue
		}
		switch {
		case need && (!set || value != want):
			plan.changes = append(plan.changes, port25MilterChange{entry: entry, want: want, before: value, beforeSet: set})
			plan.record[entry] = want
		case need:
			plan.record[entry] = want
		case set:
			plan.changes = append(plan.changes, port25MilterChange{entry: entry, remove: true, before: value, beforeSet: true})
			delete(plan.record, entry)
		default:
			delete(plan.record, entry)
		}
	}
	return plan, nil
}

// applyPort25Milters writes the planned values (postconf -P / -PX, master.cf).
func applyPort25Milters(run postconfRunner, changes []port25MilterChange) error {
	for _, change := range changes {
		key := change.entry + "/smtpd_milters"
		args := []string{"-P", key + "=" + change.want}
		if change.remove {
			args = []string{"-PX", key}
		}
		if out, err := run("postconf", args...); err != nil {
			return fmt.Errorf("postconf %s: %s", strings.Join(args, " "), firstLine(strings.TrimSpace(string(out))))
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

// port25KeptNote is the D-024 note for an owner's port-25 milter setting that
// names OpenDKIM, or for a main.cf list port 25 could not be derived from.
func port25KeptNote(plan port25Plan) string {
	var notes []string
	if len(plan.owner) > 0 {
		notes = append(notes, "The milter setting of the smtpd that receives mail on port 25 was changed by the server owner in master.cf and kept as it is ("+
			strings.Join(plan.owner, "; ")+"). It names OpenDKIM ("+opendkimMilter+"), so mail that arrives on port 25, also from this server's own sites over loopback, "+
			"can still leave signed for a hosted domain. The server owner removes OpenDKIM from that setting, or removes the setting so CelikPanel writes it, and restarts the panel")
	}
	if plan.variable {
		notes = append(notes, "main.cf's smtpd_milters names its filters through a variable, so CelikPanel could not leave OpenDKIM out for port 25 and did not change it; "+
			"mail that arrives on port 25 may still be signed. The server owner writes the filters into smtpd_milters by name and restarts the panel")
	}
	return joinNotes(notes...)
}

// localSubmitOpen reports whether mail handed over on this server without a
// login (the sendmail command) can come from other accounts than root and
// vmail while OpenDKIM signs such mail by its From: (review finding 2).
// Postfix's authorized_submit_users decides who may run sendmail; CelikPanel
// sets it to "root, vmail" (mail_passwordless_enforce.go). A list of only
// those names (the owner's narrower one included) is closed; Postfix's
// default (static:anyone), another name or a table is open. A
// non_smtpd_milters without OpenDKIM signs no local mail at all.
func localSubmitOpen(run postconfRunner) (bool, string, error) {
	out, err := run("postconf", "-h", "non_smtpd_milters")
	nonSMTPD, known := postconfOneValue(out)
	if err != nil || !known {
		return false, "", &postconfUnreadError{setting: "non_smtpd_milters", command: "postconf -h non_smtpd_milters"}
	}
	signsLocal := false
	for _, entry := range milterEntries(nonSMTPD) {
		if isOpenDKIMMilter(entry) || strings.Contains(entry, "$") {
			signsLocal = true
		}
	}
	if !signsLocal {
		return false, "", nil
	}
	submit, err := readPasswordlessSetting(run, passwordlessSubmitSetting)
	if err != nil {
		return false, "", err
	}
	for _, element := range restrictionElements(submit.Value) {
		name := strings.ToLower(element)
		if name != "root" && name != vmailUser {
			return true, fmt.Sprintf("Postfix's authorized_submit_users is %q, so programs of sites can hand mail to the sendmail command, "+
				"and OpenDKIM signs it for whatever hosted domain its From: names. The server owner sets \"authorized_submit_users = %s\" in /etc/postfix/main.cf and reloads Postfix",
				submit.Value, passwordlessSubmitUsers), nil
		}
	}
	return false, "", nil
}

// mailSigningInputs are the parts one Signing state is decided from.
type mailSigningInputs struct {
	confState, confNote string
	confPath            string
	port25              port25Plan
	milterErr           error
	submitOpen          bool
	submitNote          string
	submitErr           error
}

// decideMailSigning is the one Signing state of a step. Only "bound" says
// signing is in place; everything else is shown in Problems now with its
// step. The order puts what the administrator can act on first.
func decideMailSigning(in mailSigningInputs) *transport.MailSigningState {
	switch {
	case in.confState == mailSigningNoDKIM:
		return &transport.MailSigningState{State: mailSigningNoDKIM}
	case in.milterErr != nil:
		return &transport.MailSigningState{State: mailSigningFailed, Detail: in.milterErr.Error()}
	case in.confState == mailSigningFailed || in.confState == mailSigningUnknown:
		return &transport.MailSigningState{State: in.confState, Detail: in.confNote}
	case in.confState == mailSigningConfKept:
		return &transport.MailSigningState{State: mailSigningConfKept, Detail: in.confNote, Subject: in.confPath}
	case len(in.port25.owner) > 0 || in.port25.variable:
		subject := ""
		if len(in.port25.owner) > 0 {
			subject = strings.SplitN(in.port25.owner[0], " ", 2)[0]
		}
		return &transport.MailSigningState{State: mailSigningPort25Kept, Detail: port25KeptNote(in.port25), Subject: subject}
	case in.submitErr != nil:
		return &transport.MailSigningState{State: mailSigningUnknown, Detail: "who may hand mail over on this server could not be read: " + in.submitErr.Error()}
	case in.submitOpen:
		return &transport.MailSigningState{State: mailSigningSubmitOpen, Detail: in.submitNote}
	case in.confState == mailSigningUnsupported:
		return &transport.MailSigningState{State: mailSigningUnsupported, Detail: in.confNote}
	}
	return &transport.MailSigningState{State: mailSigningBound, Detail: in.confNote}
}

// mailSigningNotRun is the state of a step that stopped before the signing
// part ran (review finding 5): unknown, never an earlier "in place".
func mailSigningNotRun(err error) *transport.MailSigningState {
	return &transport.MailSigningState{State: mailSigningUnknown, Detail: "the mail signing step did not run: " + err.Error()}
}
