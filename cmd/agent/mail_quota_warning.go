package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
)

// The mailbox 90% warning (alpha.118, D-044; owner's choice of 3 Nov 2026).
//
// Dovecot's own quota warning runs a script when a save through Dovecot
// (IMAP, LMTP, dovecot-lda) takes a mailbox over 90%. Mail that Postfix's
// virtual(8) writes straight into the maildir does not go through Dovecot,
// so it never fires the warning (measured with Dovecot 2.4.1 on 3 Nov 2026).
// The same script therefore also runs hourly from a systemd timer, reads the
// usage with `doveadm quota get -A`, and warns a mailbox that is at 90% or
// more. It writes one plain message in English and Turkish into the mailbox
// with dovecot-lda (quota not enforced for this one message), once per
// crossing: a marker file per mailbox, removed when the mailbox is under 85%.
// Everything is native and keeps working without CelikPanel (D-022):
//
//   - /usr/local/libexec/celikpanel-quota-warning (root 0755)
//   - /etc/systemd/system/celikpanel-quota-warning.{service,timer}
//   - /var/lib/celikpanel-quota-warning (vmail 0700): the markers
//
// Each file carries a digest line like CelikPanel's Dovecot drop-ins; a file
// the owner changed is kept and named.
//
// Posta kutusu %90 uyarısı: Dovecot'un kendi uyarısı ve saatlik bir systemd
// zamanlayıcısı aynı betiği çalıştırır; kutuya iki dilde bir ileti bırakır,
// her aşımda bir kez.

var (
	quotaWarningScriptPath  = "/usr/local/libexec/celikpanel-quota-warning"
	quotaWarningServicePath = "/etc/systemd/system/celikpanel-quota-warning.service"
	quotaWarningTimerPath   = "/etc/systemd/system/celikpanel-quota-warning.timer"
	quotaWarningStateDir    = "/var/lib/celikpanel-quota-warning"
	quotaWarningLDAPaths    = []string{"/usr/lib/dovecot/dovecot-lda", "/usr/libexec/dovecot/dovecot-lda"}
	// quotaWarningSystemctl runs systemctl (a test seam).
	quotaWarningSystemctl = func(ctx context.Context, args ...string) ([]byte, error) {
		return serviceMutationCommand(ctx, "systemctl", args...).CombinedOutput()
	}
)

const (
	quotaWarningPercent    = 90
	quotaWarningResetBelow = 85
	quotaWarningHeader     = "# Managed by CelikPanel: mailbox 90% warning. sha256="
)

func quotaWarningLDA() string {
	for _, path := range quotaWarningLDAPaths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return quotaWarningLDAPaths[0]
}

// quotaWarningNoEnforce is dovecot-lda's option that lets the warning in
// even when the mailbox is nearly full, in the dialect of the installed
// Dovecot.
func quotaWarningNoEnforce(is24 bool) string {
	if is24 {
		return "-o quota_enforce=no"
	}
	return "-o 'plugin/quota=count:User quota:noenforcing'"
}

// quotaWarningScriptBody is the script after its first two lines (the
// shebang and the digest line). doveadmArgs and ldaArgs are extra arguments
// (empty in production; a test points them at a scratch configuration).
func quotaWarningScriptBody(is24 bool, lda, doveadmArgs, ldaArgs, stateDir string) string {
	return `# Tells a mailbox that it is 90% full, once per crossing. Run by Dovecot
# ("warn <percent> <user>") and hourly by celikpanel-quota-warning.timer
# ("scan"). CelikPanel rewrites this file only while it is exactly as
# CelikPanel wrote it. It works with or without CelikPanel.
# Posta kutusu %90 dolunca kutuya bir kez iki dilde ileti bırakır.
set -u
STATE=` + stateDir + `
LDA=` + lda + `

deliver() {
    user=$1
    percent=$2
    domain=${user#*@}
    {
        printf 'From: postmaster@%s\n' "$domain"
        printf 'To: %s\n' "$user"
        printf 'Subject: Your mailbox is %s%% full / Posta kutunuz %%%s dolu\n' "$percent" "$percent"
        printf 'Auto-Submitted: auto-generated\nMIME-Version: 1.0\nContent-Type: text/plain; charset=utf-8\n\n'
        printf 'Your mailbox %s is %s%% full.\n' "$user" "$percent"
        printf 'When it is full, new mail to it is refused and the sender gets it back.\n'
        printf 'Delete mail you no longer need and empty the Trash folder, or ask your\n'
        printf 'hosting provider for a larger mailbox.\n\n'
        printf '%s posta kutunuz %%%s dolu.\n' "$user" "$percent"
        printf 'Dolduğunda kutuya gelen yeni posta reddedilir ve gönderene geri döner.\n'
        printf 'Gerekmeyen iletileri silip Çöp klasörünü boşaltın ya da barındırma\n'
        printf 'sağlayıcınızdan daha büyük bir posta kutusu isteyin.\n'
    } | "$LDA" ` + quotaWarningNoEnforce(is24) + ` ` + ldaArgs + ` -d "$user"
}

warn() {
    percent=$1
    user=$2
    case "$user" in ''|*[!A-Za-z0-9@._+-]*|*/*) return 0 ;; esac
    [ -e "$STATE/$user" ] && return 0
    deliver "$user" "$percent" && : > "$STATE/$user"
}

scan() {
    doveadm ` + doveadmArgs + ` quota get -A 2>/dev/null | awk '$0 ~ / STORAGE / { print $1, $NF }' |
    while read -r user percent; do
        case "$percent" in ''|*[!0-9]*) continue ;; esac
        case "$user" in ''|*[!A-Za-z0-9@._+-]*|*/*) continue ;; esac
        if [ "$percent" -ge ` + strconv.Itoa(quotaWarningPercent) + ` ]; then
            warn "$percent" "$user"
        elif [ "$percent" -lt ` + strconv.Itoa(quotaWarningResetBelow) + ` ]; then
            rm -f "$STATE/$user"
        fi
    done
}

case "${1:-}" in
    warn) warn "${2:-90}" "${3:-}" ;;
    scan) scan ;;
    *) echo "usage: $0 warn <percent> <user> | scan" >&2; exit 2 ;;
esac
exit 0
`
}

// sealQuotaWarning puts the digest line after an optional first line.
func sealQuotaWarning(first, body string) string {
	sum := sha256.Sum256([]byte(body))
	return first + quotaWarningHeader + hex.EncodeToString(sum[:]) + "\n" + body
}

func renderQuotaWarningScript(is24 bool) string {
	return sealQuotaWarning("#!/bin/sh\n", quotaWarningScriptBody(is24, quotaWarningLDA(), "", "", quotaWarningStateDir))
}

func renderQuotaWarningService() string {
	return sealQuotaWarning("", `[Unit]
Description=Tell mailboxes that are 90% full (CelikPanel, Dovecot quota)
After=dovecot.service

[Service]
Type=oneshot
ExecStart=`+quotaWarningScriptPath+` scan
Nice=19
IOSchedulingClass=idle
`)
}

func renderQuotaWarningTimer() string {
	return sealQuotaWarning("", `[Unit]
Description=Hourly check of mailboxes that are 90% full (CelikPanel)

[Timer]
OnCalendar=hourly
RandomizedDelaySec=300
Persistent=true

[Install]
WantedBy=timers.target
`)
}

// quotaWarningIntact: the file is CelikPanel's text, unchanged.
func quotaWarningIntact(content []byte) bool {
	text := string(content)
	if strings.HasPrefix(text, "#!") {
		_, rest, ok := strings.Cut(text, "\n")
		if !ok {
			return false
		}
		text = rest
	}
	first, body, ok := strings.Cut(text, "\n")
	if !ok || !strings.HasPrefix(first, quotaWarningHeader) {
		return false
	}
	sum := sha256.Sum256([]byte(body))
	return strings.TrimPrefix(first, quotaWarningHeader) == hex.EncodeToString(sum[:])
}

// reconcileQuotaWarningFile writes one file when absent or still
// CelikPanel's (other) text. It answers whether it wrote and whether the
// file is the owner's (kept).
func reconcileQuotaWarningFile(path, want string, mode os.FileMode) (written, kept bool, err error) {
	current, err := os.ReadFile(path)
	switch {
	case err == nil && bytes.Equal(current, []byte(want)):
		return false, false, nil
	case err == nil && !quotaWarningIntact(current):
		return false, true, nil
	case err != nil && !errors.Is(err, os.ErrNotExist):
		return false, false, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, false, err
	}
	if err := secureWriteConfig(path, []byte(want), mode); err != nil {
		return false, false, err
	}
	return true, false, nil
}

// ensureQuotaWarningStateDir makes the marker directory, owned by vmail.
func ensureQuotaWarningStateDir() error {
	if err := os.MkdirAll(quotaWarningStateDir, 0o700); err != nil {
		return err
	}
	account, err := user.Lookup(vmailUser)
	if err != nil {
		return fmt.Errorf("the %s account is missing: %w", vmailUser, err)
	}
	uid, _ := strconv.Atoi(account.Uid)
	gid, _ := strconv.Atoi(account.Gid)
	if err := os.Chown(quotaWarningStateDir, uid, gid); err != nil {
		return err
	}
	return os.Chmod(quotaWarningStateDir, 0o700)
}

// reconcileQuotaWarning keeps the script, the timer and the marker
// directory, and enables the timer. It returns a sentence for the detail
// when something is the owner's or could not be done ("" when all is in
// place).
func reconcileQuotaWarning(ctx context.Context, is24 bool) (changed bool, note string) {
	var notes []string
	if err := ensureQuotaWarningStateDir(); err != nil {
		notes = append(notes, "the warning's marker folder could not be prepared: "+err.Error())
	}
	unitsChanged := false
	for _, file := range []struct {
		path  string
		want  string
		mode  os.FileMode
		units bool
	}{
		{quotaWarningScriptPath, renderQuotaWarningScript(is24), 0o755, false},
		{quotaWarningServicePath, renderQuotaWarningService(), 0o644, true},
		{quotaWarningTimerPath, renderQuotaWarningTimer(), 0o644, true},
	} {
		written, kept, err := reconcileQuotaWarningFile(file.path, file.want, file.mode)
		switch {
		case err != nil:
			notes = append(notes, file.path+" could not be written: "+err.Error())
		case kept:
			notes = append(notes, file.path+" was changed outside CelikPanel and was left as it is")
		case written:
			changed = true
			unitsChanged = unitsChanged || file.units
		}
	}
	if unitsChanged {
		if out, err := quotaWarningSystemctl(ctx, "daemon-reload"); err != nil {
			notes = append(notes, "systemctl daemon-reload: "+strings.TrimSpace(string(out)))
		}
	}
	if out, err := quotaWarningSystemctl(ctx, "is-enabled", "celikpanel-quota-warning.timer"); err != nil || strings.TrimSpace(string(out)) != "enabled" {
		if out, err := quotaWarningSystemctl(ctx, "enable", "--now", "celikpanel-quota-warning.timer"); err != nil {
			notes = append(notes, "the hourly 90% check could not be turned on: "+strings.TrimSpace(string(out)))
		} else {
			changed = true
		}
	}
	return changed, strings.Join(notes, "; ")
}
