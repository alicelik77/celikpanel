package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Passwordless mail, phase 2 (alpha.125; owner decision of 4 Nov 2026:
// "parolasız posta kesinlikle yasak. spam sorunu olabilir.").
//
// Two native Postfix settings close the two ways out, for every site, every
// PHP version and every pool at once, existing and new, with no CelikPanel
// process involved in sending:
//
//   - authorized_submit_users = root, vmail. The sendmail command and its
//     postdrop helper check the real UID of the caller against this list
//     (Postfix 2.2+). PHP mail() runs sendmail as the site's own account, so
//     every site is refused, whatever its PHP version, pool or sandbox, and
//     so is any script or cron job of a site user. Postfix refuses with
//     "fatal: User <name>(<uid>) is not allowed to submit mail", written to
//     the mail log and to the caller's error output, and mail() returns
//     false: a clear refusal, never a silent drop. root keeps it (cron,
//     unattended-upgrades, fail2ban, apt and every other system notice run
//     as root; CelikPanel's own account mail is handed over by the Agent as
//     root, Agent.SendAccountMailV1), and vmail keeps it because Dovecot's
//     Sieve vacation replies (mail_autoresponder.go) run sendmail as the
//     mailbox owner. A PHP-level sendmail_path override was considered and
//     not used: it would need a CelikPanel helper on every PHP version's path
//     that stays behind if the panel is removed, and it closes nothing the
//     Postfix list does not already close.
//   - smtpd_relay_restrictions without permit_mynetworks. SMTP from this
//     server to port 25 is then treated like any other client: it may still
//     deliver into this server's own mailboxes (what every server on the
//     Internet may do), but relaying anywhere else needs SASL, which on this
//     server means submission (587/465) with a mailbox's password. Postfix
//     answers "Relay access denied". mynetworks and smtpd_recipient_restrictions
//     are not changed: Postfix needs both restriction lists to allow a relay,
//     and the relay list now refuses it.
//
// Owner independence (D-022): these are ordinary main.cf settings that keep
// working with the panel stopped or removed. CelikPanel writes a setting only
// while it holds Postfix's default or the form CelikPanel itself wrote (its
// record in passwordlessRecordPath); a value the server owner set is kept and
// reported with CelikPanel's form beside it. A Postfix that does not take the
// change back (postfix check or a verified reload fails) gets the earlier
// values back and the failure recorded (D-024, D-025).
//
// Parolasız posta, 2. aşama: iki yerel Postfix ayarı (authorized_submit_users
// ve permit_mynetworks'süz smtpd_relay_restrictions) her siteyi, her PHP
// sürümünü ve havuzu birden kapatır. root ve vmail gönderebilir. Sahibin
// değeri korunur ve bildirilir; Postfix değişikliği almazsa önceki değerler
// geri yazılır ve hata kaydedilir.

const (
	passwordlessSubmitSetting = "authorized_submit_users"
	passwordlessRelaySetting  = "smtpd_relay_restrictions"
	// passwordlessSubmitUsers is CelikPanel's form of authorized_submit_users.
	passwordlessSubmitUsers = "root, " + vmailUser
	// passwordlessRelayFallback is written when the relay list is empty
	// (compatibility_level 0): Postfix's own default from level 1 on, without
	// permit_mynetworks.
	passwordlessRelayFallback = "permit_sasl_authenticated, defer_unauth_destination"

	passwordlessStateClosed    = "closed"
	passwordlessStateOpen      = "open"
	passwordlessStateOwner     = "owner"
	passwordlessStateFailed    = "failed"
	passwordlessStateNoPostfix = "no_postfix"
)

var passwordlessRecordPath = "/var/lib/celikpanel-agent-private/mail-passwordless.json"

// passwordlessSetting is one main.cf setting: Set is false when main.cf does
// not name it (Postfix's default applies; Value is then the default).
type passwordlessSetting struct {
	Value string `json:"value"`
	Set   bool   `json:"set"`
}

// passwordlessRecord is CelikPanel's before/after of the change.
type passwordlessRecord struct {
	ChangedAt    string               `json:"changed_at,omitempty"`
	SubmitBefore *passwordlessSetting `json:"submit_before,omitempty"`
	SubmitAfter  string               `json:"submit_after,omitempty"`
	RelayBefore  *passwordlessSetting `json:"relay_before,omitempty"`
	RelayAfter   string               `json:"relay_after,omitempty"`
	FailedAt     string               `json:"failed_at,omitempty"`
	Failure      string               `json:"failure,omitempty"`
	// Existing site crontabs given MAILTO="" once (cron_mailto_reconcile.go).
	CronMailtoAt      string                   `json:"cron_mailto_at,omitempty"`
	CronMailtoChanged []passwordlessCronChange `json:"cron_mailto_changed,omitempty"`
	CronMailtoKept    []string                 `json:"cron_mailto_kept,omitempty"`
}

// passwordlessPlan is what a reconcile writes (an empty write leaves the
// setting) and the state it reports.
type passwordlessPlan struct {
	writeSubmit, writeRelay string
	expectedRelay           string
	owner                   []string
}

func restrictionElements(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\r' || r == '\n'
	})
}

// relayWithoutMynetworks is the owner's relay list with permit_mynetworks
// taken out and everything else kept in order. ok is false for a list whose
// meaning CelikPanel cannot be certain of ($variables, {} groups,
// check_*_access tables that could permit by client).
func relayWithoutMynetworks(value string) (next string, changed, ok bool) {
	if strings.ContainsAny(value, "${}") {
		return "", false, false
	}
	elements := restrictionElements(value)
	if len(elements) == 0 {
		return passwordlessRelayFallback, true, true
	}
	var kept []string
	for _, element := range elements {
		if strings.EqualFold(element, "permit_mynetworks") {
			changed = true
			continue
		}
		kept = append(kept, element)
	}
	if !changed {
		return value, false, true
	}
	if len(kept) == 0 {
		return passwordlessRelayFallback, true, true
	}
	return strings.Join(kept, ", "), true, true
}

func sameRestrictions(a, b string) bool {
	x, y := restrictionElements(strings.ToLower(a)), restrictionElements(strings.ToLower(b))
	if len(x) != len(y) {
		return false
	}
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}

// planPasswordlessClose decides, without touching anything, what closing
// passwordless mail writes now. submit and relay are main.cf's current
// values (relay with Postfix's default filled in when unset).
func planPasswordlessClose(submit, relay passwordlessSetting, record *passwordlessRecord) passwordlessPlan {
	plan := passwordlessPlan{}
	switch {
	case record != nil && record.SubmitAfter != "":
		if !sameRestrictions(submit.Value, record.SubmitAfter) || !submit.Set {
			plan.owner = append(plan.owner, passwordlessSubmitSetting)
		}
	case !submit.Set:
		plan.writeSubmit = passwordlessSubmitUsers
	case !sameRestrictions(submit.Value, passwordlessSubmitUsers):
		plan.owner = append(plan.owner, passwordlessSubmitSetting)
	}

	switch {
	case record != nil && record.RelayAfter != "":
		plan.expectedRelay = record.RelayAfter
		if !sameRestrictions(relay.Value, record.RelayAfter) {
			plan.owner = append(plan.owner, passwordlessRelaySetting)
		}
	default:
		next, changed, ok := relayWithoutMynetworks(relay.Value)
		plan.expectedRelay = next
		switch {
		case !ok:
			plan.expectedRelay = ""
			plan.owner = append(plan.owner, passwordlessRelaySetting)
		case changed:
			plan.writeRelay = next
		}
	}
	return plan
}

func readPasswordlessRecord() (*passwordlessRecord, error) {
	raw, err := os.ReadFile(passwordlessRecordPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var record passwordlessRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		return nil, fmt.Errorf("CelikPanel's record %s is not readable: %w", passwordlessRecordPath, err)
	}
	return &record, nil
}

func writePasswordlessRecord(record *passwordlessRecord) error {
	raw, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(passwordlessRecordPath), 0o700); err != nil {
		return err
	}
	tmp := passwordlessRecordPath + ".tmp"
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, passwordlessRecordPath)
}

// readPasswordlessSetting reads one setting: `postconf -n` names it only when
// main.cf sets it; otherwise `postconf -x -h` gives the default in effect.
func readPasswordlessSetting(run postconfRunner, name string) (passwordlessSetting, error) {
	out, err := run("postconf", "-n", "-h", name)
	if err != nil {
		return passwordlessSetting{}, &postconfUnreadError{setting: name, command: "postconf -n -h " + name}
	}
	if value, known := postconfOneValue(out); known {
		return passwordlessSetting{Value: value, Set: true}, nil
	}
	if strings.TrimSpace(string(out)) != "" {
		return passwordlessSetting{}, &postconfUnreadError{setting: name, command: "postconf -n -h " + name}
	}
	out, err = run("postconf", "-x", "-h", name)
	value, known := postconfOneValue(out)
	if err != nil || !known {
		return passwordlessSetting{}, &postconfUnreadError{setting: name, command: "postconf -x -h " + name}
	}
	return passwordlessSetting{Value: value}, nil
}

func restorePasswordlessSetting(run postconfRunner, name string, before *passwordlessSetting) error {
	if before == nil {
		return nil
	}
	var out []byte
	var err error
	if before.Set {
		out, err = run("postconf", "-e", name+"="+before.Value)
	} else {
		out, err = run("postconf", "-X", name)
	}
	if err != nil {
		return fmt.Errorf("restore %s: %s", name, boundedNativeDetail(firstNonEmpty(string(out), err.Error())))
	}
	return nil
}

// passwordlessOwnerNote is the D-024 note for a setting the owner keeps.
func passwordlessOwnerNote(plan passwordlessPlan, submit, relay passwordlessSetting) string {
	if len(plan.owner) == 0 {
		return ""
	}
	var parts []string
	for _, name := range plan.owner {
		switch name {
		case passwordlessSubmitSetting:
			parts = append(parts, fmt.Sprintf("%s = %s (CelikPanel's form: %s)", name, submit.Value, passwordlessSubmitUsers))
		case passwordlessRelaySetting:
			want := plan.expectedRelay
			if want == "" {
				want = "the same list without permit_mynetworks"
			}
			parts = append(parts, fmt.Sprintf("%s = %s (CelikPanel's form: %s)", name, relay.Value, want))
		}
	}
	return "mail without a password: Postfix settings changed by the server owner were kept as they are (" +
		strings.Join(parts, "; ") + "). Sites may still be able to send mail without a password through what " +
		"they allow; the server owner sets CelikPanel's form in /etc/postfix/main.cf (postconf -e) and reloads Postfix, " +
		"or keeps the own setting deliberately"
}

// reconcilePasswordlessMail is the native-configuration step run by
// Agent.WireMailFilters on every Panel start (so on the first start after the
// update that brings it). It returns a note for the wiring's detail and an
// error only when a written change had to be rolled back.
func reconcilePasswordlessMail(run postconfRunner, apply func() error, now time.Time) (string, error) {
	record, err := readPasswordlessRecord()
	if err != nil {
		return "mail without a password was not closed: " + err.Error() +
			"; the server owner removes or repairs that file and restarts the panel", nil
	}
	submit, err := readPasswordlessSetting(run, passwordlessSubmitSetting)
	if err != nil {
		return "mail without a password was not closed: " + err.Error(), nil
	}
	relay, err := readPasswordlessSetting(run, passwordlessRelaySetting)
	if err != nil {
		return "mail without a password was not closed: " + err.Error(), nil
	}
	plan := planPasswordlessClose(submit, relay, record)
	ownerNote := passwordlessOwnerNote(plan, submit, relay)
	if plan.writeSubmit == "" && plan.writeRelay == "" {
		return joinNotes(ownerNote, passwordlessCronStep(record, now)), nil
	}

	next := passwordlessRecord{}
	if record != nil {
		next = *record
	}
	next.FailedAt, next.Failure = "", ""
	var written []func() error
	if plan.writeSubmit != "" {
		before := submit
		next.SubmitBefore, next.SubmitAfter = &before, plan.writeSubmit
		written = append(written, func() error { return restorePasswordlessSetting(run, passwordlessSubmitSetting, &before) })
	}
	if plan.writeRelay != "" {
		before := relay
		next.RelayBefore, next.RelayAfter = &before, plan.writeRelay
		written = append(written, func() error { return restorePasswordlessSetting(run, passwordlessRelaySetting, &before) })
	}
	rollback := func(cause error) (string, error) {
		var errs []error
		for i := len(written) - 1; i >= 0; i-- {
			if err := written[i](); err != nil {
				errs = append(errs, err)
			}
		}
		if len(errs) == 0 {
			if err := apply(); err != nil {
				errs = append(errs, fmt.Errorf("reload after restoring: %w", err))
			}
		}
		failed := passwordlessRecord{}
		if record != nil {
			failed = *record
		}
		failed.FailedAt, failed.Failure = now.UTC().Format(time.RFC3339), boundedNativeDetail(cause.Error())
		_ = writePasswordlessRecord(&failed)
		if len(errs) > 0 {
			return "", fmt.Errorf("closing mail without a password failed (%v) and the earlier Postfix settings could not all be restored (%v); "+
				"the server owner checks %s and %s in /etc/postfix/main.cf (postconf -n) and runs sudo postfix check",
				cause, errors.Join(errs...), passwordlessSubmitSetting, passwordlessRelaySetting)
		}
		return "", fmt.Errorf("closing mail without a password failed (%v); the earlier Postfix settings were restored, so mail "+
			"keeps working as before and nothing is retried until the panel starts again", cause)
	}
	for _, kv := range [][2]string{{passwordlessSubmitSetting, plan.writeSubmit}, {passwordlessRelaySetting, plan.writeRelay}} {
		if kv[1] == "" {
			continue
		}
		if out, err := run("postconf", "-e", kv[0]+"="+kv[1]); err != nil {
			return rollback(fmt.Errorf("postconf %s: %s", kv[0], boundedNativeDetail(firstNonEmpty(string(out), err.Error()))))
		}
	}
	if err := apply(); err != nil {
		return rollback(err)
	}
	next.ChangedAt = now.UTC().Format(time.RFC3339)
	if err := writePasswordlessRecord(&next); err != nil {
		// The change is in place and works; only CelikPanel's record of it
		// is missing, so a later owner change would be taken for the default.
		return joinNotes("mail without a password is closed, but CelikPanel's record of it could not be written ("+
			boundedNativeDetail(err.Error())+")", ownerNote), nil
	}
	return joinNotes(fmt.Sprintf("mail without a password is closed (%s)", passwordlessChangeSummary(plan)), ownerNote,
		passwordlessCronStep(&next, now)), nil
}

func passwordlessChangeSummary(plan passwordlessPlan) string {
	var parts []string
	if plan.writeSubmit != "" {
		parts = append(parts, passwordlessSubmitSetting+" = "+plan.writeSubmit)
	}
	if plan.writeRelay != "" {
		parts = append(parts, passwordlessRelaySetting+" = "+plan.writeRelay)
	}
	return strings.Join(parts, "; ")
}

// passwordlessEnforcement reads, without changing anything, what the server
// allows now: the report's view of phase 2.
func passwordlessEnforcement(run postconfRunner) *transport.PasswordlessMailEnforcement {
	out := &transport.PasswordlessMailEnforcement{ExpectedSubmitUsers: passwordlessSubmitUsers}
	if _, err := run("postconf", "-h", "mail_version"); err != nil {
		out.State = passwordlessStateNoPostfix
		return out
	}
	record, err := readPasswordlessRecord()
	if err != nil {
		out.State, out.Detail = passwordlessStateFailed, err.Error()
		return out
	}
	submit, err := readPasswordlessSetting(run, passwordlessSubmitSetting)
	if err != nil {
		out.State, out.Detail = passwordlessStateFailed, err.Error()
		return out
	}
	relay, err := readPasswordlessSetting(run, passwordlessRelaySetting)
	if err != nil {
		out.State, out.Detail = passwordlessStateFailed, err.Error()
		return out
	}
	out.SubmitUsers, out.RelayRestrictions = submit.Value, relay.Value
	plan := planPasswordlessClose(submit, relay, record)
	out.ExpectedRelayRestrictions = plan.expectedRelay
	if record != nil {
		out.ChangedAt = record.ChangedAt
	}
	switch {
	case len(plan.owner) > 0:
		out.State = passwordlessStateOwner
		out.Detail = passwordlessOwnerNote(plan, submit, relay)
	case plan.writeSubmit == "" && plan.writeRelay == "":
		out.State = passwordlessStateClosed
	case record != nil && record.Failure != "":
		out.State, out.Detail = passwordlessStateFailed, record.Failure
	default:
		out.State = passwordlessStateOpen
	}
	return out
}
