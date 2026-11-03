package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Mailbox quotas enforced by Dovecot (alpha.118, D-044).
//
// Every mailbox line in /etc/dovecot/users already carries its size limit.
// Dovecot's own quota plugin now enforces it, with Dovecot's own "count"
// driver (it also sees mail Postfix's virtual(8) delivered straight into the
// maildir; measured with Dovecot 2.4.1, 3 Nov 2026), and Postfix asks
// Dovecot's quota-status service at RCPT time, so mail to a full mailbox is
// refused with "552 5.2.2 Mailbox is full" before it is accepted (the sender's
// server returns it to the sender; no bounce is made here). Everything is
// native configuration, so it keeps working without CelikPanel (D-022):
//
//   - /etc/dovecot/conf.d/90-celikpanel-quota.conf, in the dialect of the
//     installed Dovecot (2.3: plugin { quota = count:... quota_rule };
//     2.4: quota "User quota" { quota_driver = count } and the global
//     quota_storage_size), with a digest header like 99-celikpanel.conf; a
//     file the owner changed is kept and named;
//   - each users line carries both limit fields, userdb_quota_rule (2.3) and
//     userdb_quota_storage_size (2.4), so a distribution upgrade from one
//     dialect to the other keeps every limit;
//   - one element in Postfix's smtpd_recipient_restrictions,
//     "check_policy_service { unix:private/celikpanel-quota-status,
//     default_action=DUNNO }", right after reject_unauth_destination. When
//     Dovecot's service does not answer, Postfix accepts the mail as before
//     (fail-open): a stopped Dovecot never stops mail from arriving.
//
// Posta kutusu kotası Dovecot'un kendi eklentisiyle uygulanır; Postfix
// RCPT aşamasında Dovecot'un quota-status hizmetine sorar ve dolu kutuya
// gelen postayı "552" ile reddeder. Hepsi yerel yapılandırmadır.

var (
	dovecotQuotaDropInPath = dovecotConfDirPath + "/90-celikpanel-quota.conf"
	// mailQuotaDialect reads the installed Dovecot's dialect (a test seam).
	mailQuotaDialect = dovecotIs24
	// mailQuotaApplyDovecot validates and writes a drop-in (a test seam).
	mailQuotaApplyDovecot = applyDovecotConf
	// mailQuotaReloadDovecot reloads a running Dovecot (a test seam).
	mailQuotaReloadDovecot = func(ctx context.Context) (string, error) {
		return applyDovecotVerified(mailServiceLeaseRunner(ctx), mailServiceReload)
	}
)

const (
	dovecotQuotaHeaderStart = "# Managed by CelikPanel: Dovecot mailbox quotas. sha256="
	mailQuotaPolicySocket   = "private/celikpanel-quota-status"
	mailQuotaFullReply      = "552 5.2.2 Mailbox is full"
)

// mailQuotaPolicyElement is CelikPanel's one element in
// smtpd_recipient_restrictions (Postfix 3.0+ per-service settings).
func mailQuotaPolicyElement() string {
	return "check_policy_service { unix:" + mailQuotaPolicySocket + ", default_action=DUNNO }"
}

// dovecotQuotaFields is the limit tail of a users line, in both dialects.
// UpdateMailQuota rewrites from "userdb_quota_rule=" to the end of the line,
// so both fields always change together.
func dovecotQuotaFields(quotaMB int) string {
	return fmt.Sprintf("userdb_quota_rule=*:storage=%dM userdb_quota_storage_size=%dM", quotaMB, quotaMB)
}

const dovecotQuotaPreamble = `# Mailbox size limits from /etc/dovecot/users, enforced by Dovecot itself.
# Postfix asks the quota-status service below before it accepts mail, so mail
# to a full mailbox is refused (552) instead of stored. CelikPanel rewrites
# this file only while it is exactly as CelikPanel wrote it. Dovecot runs
# from this file with or without CelikPanel.
# Posta kutusu boyut sınırları; dolu kutuya gelen posta reddedilir.
`

func dovecotQuotaBody(is24 bool) string {
	if !is24 {
		return dovecotQuotaPreamble + `mail_plugins = $mail_plugins quota
protocol imap {
  mail_plugins = $mail_plugins imap_quota
}
plugin {
  quota = count:User quota
  quota_vsizes = yes
  quota_rule = *:storage=1024M
  quota_status_success = DUNNO
  quota_status_nouser = DUNNO
  quota_status_overquota = "` + mailQuotaFullReply + `"
  quota_warning = storage=90%% quota-warning warn 90 %u
}
` + quotaWarningService() + `service quota-status {
  executable = quota-status -p postfix
  unix_listener /var/spool/postfix/` + mailQuotaPolicySocket + ` {
    user = postfix
    group = postfix
    mode = 0660
  }
  client_limit = 1
}
`
	}
	// 2.4: a userdb field overrides a global setting, not one written inside
	// the quota filter, so the default size is global (measured 2.4.1).
	return dovecotQuotaPreamble + `mail_plugins {
  quota = yes
}
protocol imap {
  mail_plugins {
    imap_quota = yes
  }
}
quota_storage_size = 1024M
quota "User quota" {
  quota_driver = count
  warning warn-90 {
    quota_storage_percentage = 90
    execute quota-warning {
      args = warn 90 %{user}
    }
  }
}
` + quotaWarningService() + `
quota_status_success = DUNNO
quota_status_nouser = DUNNO
quota_status_overquota = "` + mailQuotaFullReply + `"
service quota-status {
  executable = quota-status -p postfix
  unix_listener /var/spool/postfix/` + mailQuotaPolicySocket + ` {
    user = postfix
    group = postfix
    mode = 0660
  }
  client_limit = 1
}
`
}

// quotaWarningService runs the 90% warning script as the mail user
// (mail_quota_warning.go); the same text in both dialects.
func quotaWarningService() string {
	return `service quota-warning {
  executable = script ` + quotaWarningScriptPath + `
  user = ` + vmailUser + `
  unix_listener quota-warning {
    user = ` + vmailUser + `
  }
}
`
}

// buildDovecotQuotaConf: the body with its digest header.
func buildDovecotQuotaConf(is24 bool) string {
	body := dovecotQuotaBody(is24)
	sum := sha256.Sum256([]byte(body))
	return dovecotQuotaHeaderStart + hex.EncodeToString(sum[:]) + "\n" + body
}

// dovecotQuotaManagedIntact: the file is CelikPanel's text, unchanged since
// it was written (its digest matches).
func dovecotQuotaManagedIntact(content []byte) bool {
	first, body, ok := bytes.Cut(content, []byte("\n"))
	if !ok || !bytes.HasPrefix(first, []byte(dovecotQuotaHeaderStart)) {
		return false
	}
	sum := sha256.Sum256(body)
	return string(bytes.TrimPrefix(first, []byte(dovecotQuotaHeaderStart))) == hex.EncodeToString(sum[:])
}

// normalizeDovecotUsersQuota gives every users line both limit fields; a
// line with only the 2.3 field gets the 2.4 field with the same size.
func normalizeDovecotUsersQuota(content []byte) ([]byte, bool) {
	lines := splitMailFile(content)
	changed := false
	for index, line := range lines {
		ruleAt := strings.Index(line, "userdb_quota_rule=*:storage=")
		if ruleAt < 0 || strings.Contains(line, "userdb_quota_storage_size=") {
			continue
		}
		rest := line[ruleAt+len("userdb_quota_rule=*:storage="):]
		end := strings.IndexAny(rest, " \t")
		value := rest
		if end >= 0 {
			value = rest[:end]
		}
		size, ok := strings.CutSuffix(value, "M")
		if !ok {
			continue
		}
		mb, err := strconv.Atoi(size)
		if err != nil || mb <= 0 {
			continue
		}
		if end >= 0 {
			// Something follows the rule; keep it and add the field after it.
			lines[index] = line[:ruleAt] + "userdb_quota_rule=*:storage=" + value + " userdb_quota_storage_size=" + size + "M" + rest[end:]
		} else {
			lines[index] = line[:ruleAt] + dovecotQuotaFields(mb)
		}
		changed = true
	}
	if !changed {
		return content, false
	}
	return renderMailFile(lines), true
}

// planQuotaPolicyRestriction adds CelikPanel's policy element after
// reject_unauth_destination, or before the first terminal element, or at the
// end. An element already there (anywhere) is left alone.
func planQuotaPolicyRestriction(current string) (string, bool, error) {
	elements, ok := splitRecipientRestrictions(current)
	if !ok {
		return "", false, fmt.Errorf("smtpd_recipient_restrictions has unbalanced braces")
	}
	for _, element := range elements {
		if strings.Contains(element.text, mailQuotaPolicySocket) {
			return "", false, nil
		}
	}
	texts := make([]string, 0, len(elements)+1)
	inserted := false
	for _, element := range elements {
		if !inserted && isTerminalRestriction(element.text) {
			texts = append(texts, mailQuotaPolicyElement())
			inserted = true
		}
		texts = append(texts, element.text)
		if !inserted && strings.EqualFold(element.text, "reject_unauth_destination") {
			texts = append(texts, mailQuotaPolicyElement())
			inserted = true
		}
	}
	if !inserted {
		texts = append(texts, mailQuotaPolicyElement())
	}
	return strings.Join(texts, ", "), true, nil
}

// ReconcileMailQuotaV1 makes Dovecot enforce the mailbox limits (see the
// file comment). It is idempotent: a second call changes nothing.
//
// ReconcileMailQuotaV1 posta kutusu sınırlarını Dovecot'a uygulatır; tekrar
// çağrı hiçbir şey değiştirmez.
func (a *Agent) ReconcileMailQuotaV1(req *transport.MailQuotaReconcileRequest, resp *transport.MailQuotaReconcileResponse) error {
	if resp == nil {
		return nil
	}
	*resp = transport.MailQuotaReconcileResponse{}
	if req == nil {
		resp.State, resp.Error = transport.MailQuotaUnknown, "mail quota request is required"
		return nil
	}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "enforcing mailbox quotas"); err != nil {
		resp.State, resp.Error = transport.MailQuotaUnknown, err.Error()
		return nil
	}
	mailMutex.Lock()
	defer mailMutex.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), fileAccessTimeout)
	defer cancel()
	reconcileMailQuota(ctx, resp)
	return nil
}

func reconcileMailQuota(ctx context.Context, resp *transport.MailQuotaReconcileResponse) {
	if !dovecotInstalledForRepair() {
		resp.State = transport.MailQuotaNoMail
		resp.Detail = "Dovecot is not installed on this server, so there are no mailboxes to limit."
		return
	}
	virtual, err := snapshotMailFile(dovecotVirtualDropInPath)
	if err != nil || !virtual.exists {
		resp.State = transport.MailQuotaNoMail
		resp.Detail = "Mail is not set up by CelikPanel on this server yet; mailbox limits are enforced once it is."
		return
	}
	is24, err := mailQuotaDialect()
	if err != nil {
		resp.State = transport.MailQuotaUnknown
		resp.Detail = "The installed Dovecot version could not be read, so nothing was changed; CelikPanel checks again at its next start."
		return
	}
	resp.Dialect = "2.3"
	if is24 {
		resp.Dialect = "2.4"
	}

	// 1. Both limit fields on every users line (the 2.4 field is new).
	if users, err := snapshotMailFile(dovecotUsersPath); err == nil && users.exists {
		if next, changed := normalizeDovecotUsersQuota(users.content); changed {
			if err := secureWriteConfig(dovecotUsersPath, next, users.mode); err != nil {
				resp.State, resp.Error = transport.MailQuotaUnknown, "the mailbox list could not be updated: "+err.Error()
				return
			}
			_ = secureSetMailFileMetadata(dovecotUsersPath, users.mode, users.uid, users.gid)
			resp.Changed = true
		}
	}

	// 2. The 90% warning: its script, the hourly timer and the markers. They
	// come first: the drop-in below names the script.
	if warned, note := reconcileQuotaWarning(ctx, is24); warned || note != "" {
		resp.Changed = resp.Changed || warned
		if note != "" {
			resp.Detail = "The mailbox 90% warning is not complete: " + note + "."
		}
	}

	// 3. The Dovecot drop-in, validated by doveconf before it stays.
	want := buildDovecotQuotaConf(is24)
	current, err := snapshotMailFile(dovecotQuotaDropInPath)
	if err != nil {
		resp.State, resp.Error = transport.MailQuotaUnknown, fmt.Sprintf("%s could not be read: %v", dovecotQuotaDropInPath, err)
		return
	}
	switch {
	case current.exists && bytes.Equal(current.content, []byte(want)):
	case current.exists && !dovecotQuotaManagedIntact(current.content):
		resp.State = transport.MailQuotaKept
		resp.Detail = dovecotQuotaDropInPath + " was changed outside CelikPanel, so CelikPanel left it as it is; mailbox limits are enforced only as that file says. " +
			"The server administrator either keeps it or moves it aside (for example `sudo mv " + dovecotQuotaDropInPath + " /root/`), then CelikPanel writes its own at its next start."
		return
	default:
		if err := mailQuotaApplyDovecot(dovecotQuotaDropInPath, want); err != nil {
			resp.State = transport.MailQuotaRefused
			resp.Detail = "Dovecot refused CelikPanel's quota settings, so the previous files were put back and mail keeps working without size limits: " + err.Error()
			return
		}
		resp.Changed = true
	}

	// 4. Postfix asks Dovecot before accepting mail.
	values, err := mailPolicyPostconf("-h", suspensionRestrictionsParam)
	if err != nil {
		resp.State = transport.MailQuotaNotApplied
		resp.Detail = "Dovecot enforces the limits, but Postfix's settings could not be read, so mail to a full mailbox may still be accepted and then held by Dovecot; CelikPanel tries again at its next start."
		return
	}
	next, change, err := planQuotaPolicyRestriction(strings.TrimSpace(string(values)))
	if err != nil {
		resp.State = transport.MailQuotaKept
		resp.Detail = "Dovecot enforces the limits, but Postfix's smtpd_recipient_restrictions could not be read safely (" + err.Error() + "), so CelikPanel left it as it is. " +
			"The server administrator adds `" + mailQuotaPolicyElement() + "` after reject_unauth_destination in /etc/postfix/main.cf."
		return
	}
	if change {
		if out, err := mailPolicyPostconf("-e", suspensionRestrictionsParam+"="+next); err != nil {
			resp.State = transport.MailQuotaNotApplied
			resp.Detail = "postconf could not add the quota check: " + strings.TrimSpace(string(out))
			return
		}
		resp.Changed = true
	}
	if resp.Changed {
		if _, err := mailQuotaReloadDovecot(ctx); err != nil {
			resp.State = transport.MailQuotaNotApplied
			resp.Detail = "The settings are written and validated, but Dovecot could not be reloaded (" + err.Error() + "); the server administrator runs `sudo systemctl reload dovecot`."
			return
		}
		if _, err := mailPolicyReload(); err != nil {
			resp.State = transport.MailQuotaNotApplied
			resp.Detail = "The settings are written, but Postfix could not be reloaded (" + err.Error() + "); the server administrator runs `sudo postfix check` and `sudo systemctl reload postfix`."
			return
		}
	}
	resp.State = transport.MailQuotaEnforced
}
