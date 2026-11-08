package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Mail server settings one at a time (alpha.130; docs/MAIL-SERVER-SETTINGS-
// DESIGN.md; transport/mail_server_settings_contracts.go).
//
// Subaddressing: main.cf `recipient_delimiter = +`, so user+tag@domain is
// delivered to user@domain's mailbox (virtual(8) and transport/access lookups
// try the address without "+tag" next). Dovecot's LMTP must strip the same
// character for the mailboxes it delivers (its default is "+"): the Agent reads
// it and refuses when it differs, it never writes Dovecot. A marker file
// /etc/postfix/celikpanel-subaddressing says the line is CelikPanel's; a
// recipient_delimiter without it is the owner's and is never changed. Off
// removes the line and the marker.
//
// Sender list: /etc/postfix/celikpanel_sender_access (marker and digest, like
// the domain options map) named by `check_sender_access <type>:<map>` at the
// front of main.cf smtpd_sender_restrictions. A block line refuses mail from
// that address or domain with 550 5.7.1; an allow line says DUNNO, which only
// stops the lookup (a domain block then spares that address). The map never
// says OK, so it can never open a relay or skip a spam check. An empty list
// removes the element (and the setting when nothing else is in it), the map
// and its index.
//
// Every change is one transaction over main.cf and the files: Postfix's
// verified reload, and on a refusal the earlier files are put back.
//
// Alt adresleme ve gönderen listesi: Postfix'in kendi ayarları; sahibin
// değeri korunur; liste asla OK demez, aktarma açamaz.

var (
	mailSubaddressingMarker = "/etc/postfix/celikpanel-subaddressing"
	mailSenderMapPath       = "/etc/postfix/celikpanel_sender_access"
)

const (
	mailSubaddressingMarkerText = "# CelikPanel turned on Postfix subaddressing (recipient_delimiter = +). Turn it off in CelikPanel to remove the line and this file.\n"
	mailSenderMarker            = "# Managed by CelikPanel: the server's sender allow and block list."
	mailSenderDigestPrefix      = "# celikpanel-body-sha256: "
	mailSenderBlockAction       = "550 5.7.1 Mail from this sender is not accepted by this server"
	mailSenderRestriction       = "check_sender_access"
	mailSenderParam             = "smtpd_sender_restrictions"
)

// Swapped by tests.
var (
	mailSettingsRun = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(ctx, mailRelayCommandWait)
		defer cancel()
		return executeMailTLSCommand(ctx, name, args...)
	}
	mailSettingsLookPath = exec.LookPath
	mailSettingsRead     = func(path string) ([]byte, bool, error) { return mailRelayRead(path) }
	mailSettingsWrite    = func(path string, content []byte) error { return secureWriteConfig(path, content, 0o644) }
	mailSettingsRemove   = func(path string) error { return mailRelayRemove(path) }
	mailSettingsSnapshot = snapshotMailFile
	mailSettingsRestore  = restoreMailFile
	mailSettingsMapType  = postfixMapTypeContext
	mailSettingsReload   = func() error {
		_, err := applyPostfixVerified(runMailTLSCommand, mailServiceReload)
		return err
	}
)

var mailSettingsMu sync.Mutex

func mailSettingsValue(ctx context.Context, name string) (string, error) {
	out, err := mailSettingsRun(ctx, "postconf", "-n", "-h", name)
	if err != nil {
		return "", fmt.Errorf("postconf -n -h %s: %s", name, boundedNativeDetail(firstNonEmpty(string(out), err.Error())))
	}
	if mailRelayOnlyPostconfMessages(out) {
		return "", nil
	}
	value, known := postconfOneValue(out)
	if !known {
		return "", fmt.Errorf("postconf -n -h %s did not answer one value", name)
	}
	return value, nil
}

// ---- Subaddressing ----------------------------------------------------------

type subaddressingFacts struct {
	value  string
	marker bool
}

func (f subaddressingFacts) state() string {
	switch {
	case f.value == "" && !f.marker:
		return transport.MailSettingOff
	case f.value == "+" && f.marker:
		return transport.MailSettingOn
	case f.value != "" && !f.marker:
		return transport.MailSettingOwner
	case f.value == "" && f.marker:
		return transport.MailSettingPartial
	default:
		return transport.MailSettingEdited
	}
}

func readSubaddressingFacts(ctx context.Context) (subaddressingFacts, error) {
	value, err := mailSettingsValue(ctx, "recipient_delimiter")
	if err != nil {
		return subaddressingFacts{}, err
	}
	_, marker, err := mailSettingsRead(mailSubaddressingMarker)
	if err != nil {
		return subaddressingFacts{}, fmt.Errorf("read %s: %s", mailSubaddressingMarker, boundedNativeDetail(err.Error()))
	}
	return subaddressingFacts{value: value, marker: marker}, nil
}

// dovecotDelimiter is Dovecot's recipient_delimiter ("" with ok=false when it
// cannot be read).
func dovecotDelimiter(ctx context.Context) (string, bool) {
	if _, err := mailSettingsLookPath("doveconf"); err != nil {
		return "", false
	}
	out, err := mailSettingsRun(ctx, "doveconf", "-h", "recipient_delimiter")
	if err != nil {
		return "", false
	}
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(lines) != 1 {
		return "", false
	}
	return strings.TrimSpace(lines[0]), true
}

func subaddressingDetail(facts subaddressingFacts) string {
	switch facts.state() {
	case transport.MailSettingOwner:
		if strings.Contains(facts.value, "+") {
			return "main.cf already has recipient_delimiter = " + facts.value + ", set outside CelikPanel (Debian and Ubuntu set it by default), so user+tag@ already reaches user@. CelikPanel keeps the line; to turn it off, the server administrator removes it (`sudo postconf -X recipient_delimiter`, then `sudo postfix reload`)"
		}
		return "main.cf already has the server owner's recipient_delimiter = " + facts.value + ". CelikPanel keeps it and changes nothing"
	case transport.MailSettingEdited:
		return "recipient_delimiter was changed on the server to " + facts.value + ". CelikPanel keeps it and changes nothing; the server owner sets it back to + or removes " + mailSubaddressingMarker + " to keep their own value"
	}
	return ""
}

// MailSubaddressingV1 turns subaddressing on or off.
func (a *Agent) MailSubaddressingV1(req *transport.MailSubaddressingRequest, resp *transport.MailSettingApplyResponse) error {
	if req == nil || resp == nil {
		return errors.New("subaddressing request and response are required")
	}
	*resp = transport.MailSettingApplyResponse{}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "changing subaddressing"); err != nil {
		resp.Code, resp.Error = transport.MailSettingInvalid, err.Error()
		return nil
	}
	if !lockMailSettings(resp) {
		return nil
	}
	defer unlockMailSettings()
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	applySubaddressing(ctx, req.Enabled, resp)
	return nil
}

func lockMailSettings(resp *transport.MailSettingApplyResponse) bool {
	if !mailSettingsMu.TryLock() {
		resp.Code, resp.Error = transport.MailSettingBusy, "another change of the mail server settings is running; nothing was changed"
		return false
	}
	if !mailMutex.TryLock() {
		mailSettingsMu.Unlock()
		resp.Code, resp.Error = transport.MailSettingBusy, "another mail change is running; nothing was changed"
		return false
	}
	return true
}

func unlockMailSettings() {
	mailMutex.Unlock()
	mailSettingsMu.Unlock()
}

func applySubaddressing(ctx context.Context, enabled bool, resp *transport.MailSettingApplyResponse) {
	if _, err := mailSettingsLookPath("postconf"); err != nil {
		resp.Code, resp.Error = transport.MailSettingNoPostfix, "Postfix is not installed on this server; nothing was changed"
		return
	}
	facts, err := readSubaddressingFacts(ctx)
	if err != nil {
		resp.Code, resp.Error = transport.MailSettingUnreadable, "the Postfix settings could not be read ("+err.Error()+"); nothing was changed"
		return
	}
	state := facts.state()
	resp.State = state
	switch {
	case state == transport.MailSettingEdited:
		resp.Code, resp.Error = transport.MailSettingEditedConfig, subaddressingDetail(facts)
		return
	case state == transport.MailSettingOwner:
		// The value set outside CelikPanel stays whatever was asked. Debian's
		// and Ubuntu's own main.cf already carry `recipient_delimiter = +`
		// (measured on Debian 13): turning on then has nothing to do. A wish
		// the owner's value contradicts (on with another character, off with
		// their "+") changes nothing and says how the owner changes it.
		if enabled == strings.Contains(facts.value, "+") {
			resp.Applied = true
			return
		}
		resp.Code, resp.Error = transport.MailSettingOwnerConfig, subaddressingDetail(facts)
		return
	case enabled && state == transport.MailSettingOn, !enabled && state == transport.MailSettingOff:
		resp.Applied = true
		return
	}
	if enabled {
		delimiter, ok := dovecotDelimiter(ctx)
		if !ok {
			resp.Code, resp.Error = transport.MailSettingDovecot, "Dovecot's recipient_delimiter could not be read (`doveconf -h recipient_delimiter`), so CelikPanel did not turn subaddressing on: mail Dovecot delivers to user+tag@ could bounce. Nothing was changed"
			return
		}
		if !strings.Contains(delimiter, "+") {
			resp.Code, resp.Error = transport.MailSettingDovecot, "Dovecot's recipient_delimiter is \""+delimiter+"\", not +, so mail Dovecot delivers to user+tag@ would bounce. CelikPanel never changes Dovecot's own setting; the server owner sets recipient_delimiter = + in Dovecot (or removes their line) and turns subaddressing on again. Nothing was changed"
			return
		}
	}
	err = mailSettingsTransaction(ctx, []string{mailSubaddressingMarker}, resp, func(run func(...string) error) error {
		if enabled {
			if err := mailSettingsWrite(mailSubaddressingMarker, []byte(mailSubaddressingMarkerText)); err != nil {
				return fmt.Errorf("the marker file could not be written (%s)", boundedNativeDetail(err.Error()))
			}
			if err := run("postconf", "-e", "recipient_delimiter=+"); err != nil {
				return err
			}
			resp.Steps = append(resp.Steps, "main.cf delivers user+tag@ to user@ (recipient_delimiter = +)")
			return nil
		}
		if facts.value == "+" {
			if err := run("postconf", "-X", "recipient_delimiter"); err != nil {
				return err
			}
		}
		if err := mailSettingsRemove(mailSubaddressingMarker); err != nil {
			return fmt.Errorf("the marker file could not be removed (%s)", boundedNativeDetail(err.Error()))
		}
		resp.Steps = append(resp.Steps, "CelikPanel's recipient_delimiter line and its marker file were removed")
		return nil
	})
	if err != nil {
		return
	}
	resp.Applied, resp.Changed = true, true
	resp.State = transport.MailSettingOff
	if enabled {
		resp.State = transport.MailSettingOn
	}
}

// mailSettingsTransaction snapshots main.cf and the named files, runs the
// change and Postfix's verified reload, and puts every file back when a step
// or the reload fails. The refusal is written into resp.
func mailSettingsTransaction(ctx context.Context, files []string, resp *transport.MailSettingApplyResponse, change func(run func(...string) error) error) error {
	all := append([]string{"/etc/postfix/main.cf"}, files...)
	snapshots := make([]mailFileSnapshot, 0, len(all))
	for _, path := range all {
		snapshot, err := mailSettingsSnapshot(path)
		if err != nil {
			resp.Code, resp.Error = transport.MailSettingUnreadable, "the Postfix files could not be read before the change ("+boundedNativeDetail(err.Error())+"); nothing was changed"
			return err
		}
		snapshots = append(snapshots, snapshot)
	}
	putBack := func(cause string) error {
		var errs []string
		for _, snapshot := range snapshots {
			if err := mailSettingsRestore(snapshot); err != nil {
				errs = append(errs, boundedNativeDetail(err.Error()))
			}
		}
		switch {
		case len(errs) > 0:
			resp.Code = transport.MailSettingRollbackFailed
			resp.Error = cause + "; the earlier Postfix files could not all be put back (" + strings.Join(errs, "; ") + "). The server administrator runs `sudo postfix check` and compares /etc/postfix/main.cf"
		default:
			if err := mailSettingsReload(); err != nil {
				resp.Code = transport.MailSettingRollbackFailed
				resp.Error = cause + "; the earlier files were put back but Postfix did not reload them (" + boundedNativeDetail(err.Error()) + "). The server administrator runs `sudo postfix check` and `sudo systemctl reload postfix`"
			} else {
				resp.Code = transport.MailSettingPostfixFailed
				resp.Error = cause + "; the earlier settings were put back and mail works as before"
			}
		}
		return errors.New(resp.Error)
	}
	run := func(args ...string) error {
		out, err := mailSettingsRun(ctx, args[0], args[1:]...)
		if err != nil {
			return fmt.Errorf("%s: %s", strings.Join(args[:2], " "), boundedNativeDetail(firstNonEmpty(string(out), err.Error())))
		}
		return nil
	}
	if err := change(run); err != nil {
		return putBack(err.Error())
	}
	if err := mailSettingsReload(); err != nil {
		return putBack("Postfix did not take the change (" + boundedNativeDetail(err.Error()) + ")")
	}
	resp.Steps = append(resp.Steps, "Postfix took the change")
	return nil
}

// ---- Sender list --------------------------------------------------------------

func renderMailSenderMap(rules []transport.MailSenderRule) []byte {
	sorted := append([]transport.MailSenderRule(nil), rules...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Value < sorted[j].Value })
	var body strings.Builder
	for _, rule := range sorted {
		action := "DUNNO"
		if rule.Action == transport.MailSenderBlock {
			action = mailSenderBlockAction
		}
		body.WriteString(rule.Value + " " + action + "\n")
	}
	sum := sha256.Sum256([]byte(body.String()))
	return []byte(mailSenderMarker + "\n" + mailSenderDigestPrefix + hex.EncodeToString(sum[:]) + "\n" + body.String())
}

func mailSenderMapIsPanels(raw []byte, exists bool) bool {
	if !exists {
		return true
	}
	rest, ok := strings.CutPrefix(string(raw), mailSenderMarker+"\n"+mailSenderDigestPrefix)
	if !ok {
		return false
	}
	digest, body, found := strings.Cut(rest, "\n")
	if !found {
		return false
	}
	sum := sha256.Sum256([]byte(body))
	return hex.EncodeToString(sum[:]) == digest
}

func mailSenderOperand(text string) bool {
	index := strings.IndexByte(text, ':')
	return index > 0 && text[index+1:] == mailSenderMapPath
}

// planMailSenderRestriction returns the new smtpd_sender_restrictions value
// ("" with unset=true removes the setting) and whether it changes.
func planMailSenderRestriction(current string, want bool, mapType string) (value string, unset, change bool, err error) {
	if strings.Contains(current, "$") {
		return "", false, false, errors.New("smtpd_sender_restrictions names a variable ($), so CelikPanel cannot tell what it holds")
	}
	elements, ok := splitRecipientRestrictions(current)
	if !ok {
		return "", false, false, errors.New("smtpd_sender_restrictions has unbalanced braces")
	}
	var kept []string
	present := false
	for index := 0; index < len(elements); index++ {
		if elements[index].text == mailSenderRestriction && index+1 < len(elements) && mailSenderOperand(elements[index+1].text) {
			present = true
			index++
			continue
		}
		kept = append(kept, elements[index].text)
	}
	element := mailSenderRestriction + " " + mapType + ":" + mailSenderMapPath
	switch {
	case want && present:
		return "", false, false, nil
	case want:
		return strings.Join(append([]string{element}, kept...), ", "), false, true, nil
	case !present:
		return "", false, false, nil
	case len(kept) == 0:
		return "", true, true, nil
	default:
		return strings.Join(kept, ", "), false, true, nil
	}
}

func validMailSenderRules(rules []transport.MailSenderRule) error {
	if len(rules) > transport.MailSenderMaxRules {
		return fmt.Errorf("the sender list holds at most %d entries", transport.MailSenderMaxRules)
	}
	seen := map[string]bool{}
	for _, rule := range rules {
		value, err := transport.CanonicalMailSenderValue(rule.Value)
		if err != nil || value != rule.Value {
			return errors.New("a sender list entry is not a canonical address or domain")
		}
		if rule.Action != transport.MailSenderAllow && rule.Action != transport.MailSenderBlock {
			return errors.New("a sender list entry is neither allow nor block")
		}
		if seen[value] {
			return errors.New("a sender list entry appears twice")
		}
		seen[value] = true
	}
	return nil
}

func mailSenderIndexPaths() []string {
	return []string{mailSenderMapPath + ".db", mailSenderMapPath + ".lmdb", mailSenderMapPath + ".cdb"}
}

// MailSenderListV1 makes the server hold the wanted sender list.
func (a *Agent) MailSenderListV1(req *transport.MailSenderListRequest, resp *transport.MailSettingApplyResponse) error {
	if req == nil || resp == nil {
		return errors.New("sender list request and response are required")
	}
	*resp = transport.MailSettingApplyResponse{}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "changing the sender list"); err != nil {
		resp.Code, resp.Error = transport.MailSettingInvalid, err.Error()
		return nil
	}
	if !lockMailSettings(resp) {
		return nil
	}
	defer unlockMailSettings()
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	applyMailSenderList(ctx, req.Rules, resp)
	return nil
}

func applyMailSenderList(ctx context.Context, rules []transport.MailSenderRule, resp *transport.MailSettingApplyResponse) {
	if _, err := mailSettingsLookPath("postconf"); err != nil {
		resp.Code, resp.Error = transport.MailSettingNoPostfix, "Postfix is not installed on this server; nothing was changed"
		return
	}
	if err := validMailSenderRules(rules); err != nil {
		resp.Code, resp.Error = transport.MailSettingInvalid, err.Error()+"; nothing was changed"
		return
	}
	raw, exists, err := mailSettingsRead(mailSenderMapPath)
	if err != nil {
		resp.Code, resp.Error = transport.MailSettingUnreadable, "the sender list map could not be read ("+boundedNativeDetail(err.Error())+"); nothing was changed"
		return
	}
	if !mailSenderMapIsPanels(raw, exists) {
		resp.State = transport.MailSettingEdited
		resp.Code, resp.Error = transport.MailSettingEditedConfig, mailSenderMapPath+" was changed on the server, so CelikPanel keeps it and changed nothing. The server owner moves their own lines elsewhere and deletes the file, then saves the list again"
		return
	}
	current, err := mailSettingsValue(ctx, mailSenderParam)
	if err != nil {
		resp.Code, resp.Error = transport.MailSettingUnreadable, "the Postfix settings could not be read ("+err.Error()+"); nothing was changed"
		return
	}
	mapType := mailSettingsMapType(ctx)
	want := len(rules) > 0
	value, unset, change, err := planMailSenderRestriction(current, want, mapType)
	if err != nil {
		resp.State = transport.MailSettingOwner
		resp.Code, resp.Error = transport.MailSettingOwnerConfig, err.Error()+"; CelikPanel keeps it and changed nothing"
		return
	}
	content := renderMailSenderMap(rules)
	mapChanged := want && (!exists || string(raw) != string(content))
	if !change && !mapChanged && (want || !exists) {
		resp.Applied = true
		resp.State = transport.MailSettingOff
		if want {
			resp.State = transport.MailSettingOn
		}
		return
	}
	files := append([]string{mailSenderMapPath}, mailSenderIndexPaths()...)
	err = mailSettingsTransaction(ctx, files, resp, func(run func(...string) error) error {
		if want {
			if err := mailSettingsWrite(mailSenderMapPath, content); err != nil {
				return fmt.Errorf("the sender list map could not be written (%s)", boundedNativeDetail(err.Error()))
			}
			if mapType != "texthash" {
				if err := run("postmap", mapType+":"+mailSenderMapPath); err != nil {
					return err
				}
			}
			resp.Steps = append(resp.Steps, fmt.Sprintf("the sender list (%d entries) is in %s", len(rules), mailSenderMapPath))
		}
		if change {
			if unset {
				if err := run("postconf", "-X", mailSenderParam); err != nil {
					return err
				}
			} else if err := run("postconf", "-e", mailSenderParam+"="+value); err != nil {
				return err
			}
		}
		if !want {
			for _, path := range files {
				if err := mailSettingsRemove(path); err != nil {
					return fmt.Errorf("%s could not be removed (%s)", path, boundedNativeDetail(err.Error()))
				}
			}
			resp.Steps = append(resp.Steps, "the sender list, its map and its main.cf element were removed")
		}
		return nil
	})
	if err != nil {
		return
	}
	resp.Applied, resp.Changed = true, true
	resp.State = transport.MailSettingOff
	if want {
		resp.State = transport.MailSettingOn
	}
}

// ---- Status -------------------------------------------------------------------

// MailServerSettingsStatusV1 reads both settings. Read only.
func (a *Agent) MailServerSettingsStatusV1(_ *transport.Empty, resp *transport.MailServerSettingsStatus) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	*resp = readMailServerSettingsStatus(ctx)
	return nil
}

func readMailServerSettingsStatus(ctx context.Context) transport.MailServerSettingsStatus {
	if _, err := mailSettingsLookPath("postconf"); err != nil {
		return transport.MailServerSettingsStatus{}
	}
	status := transport.MailServerSettingsStatus{Postfix: true}
	facts, err := readSubaddressingFacts(ctx)
	if err != nil {
		status.Error = err.Error()
		return status
	}
	status.Subaddressing = facts.state()
	status.RecipientDelimiter = facts.value
	status.SubaddressingDetail = subaddressingDetail(facts)
	if delimiter, ok := dovecotDelimiter(ctx); ok {
		status.DovecotDelimiter = delimiter
	}
	raw, exists, err := mailSettingsRead(mailSenderMapPath)
	if err != nil {
		status.Error = err.Error()
		return status
	}
	current, err := mailSettingsValue(ctx, mailSenderParam)
	if err != nil {
		status.Error = err.Error()
		return status
	}
	_, _, _, planErr := planMailSenderRestriction(current, false, "x")
	elements, _ := splitRecipientRestrictions(current)
	wired := false
	for index := 0; index+1 < len(elements); index++ {
		wired = wired || (elements[index].text == mailSenderRestriction && mailSenderOperand(elements[index+1].text))
	}
	switch {
	case !mailSenderMapIsPanels(raw, exists):
		status.SenderList = transport.MailSettingEdited
		status.SenderListDetail = mailSenderMapPath + " was changed on the server; CelikPanel keeps it"
	case planErr != nil:
		status.SenderList = transport.MailSettingOwner
		status.SenderListDetail = planErr.Error()
	case exists && wired:
		status.SenderList = transport.MailSettingOn
	case !exists && !wired:
		status.SenderList = transport.MailSettingOff
	default:
		status.SenderList = transport.MailSettingPartial
	}
	return status
}
