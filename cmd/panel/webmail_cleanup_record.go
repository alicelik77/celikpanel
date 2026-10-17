package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"strings"
	"time"
)

// The Panel's own record of a Roundcube install or removal that did not
// finish (alpha.95, independent review of 2026-10-17). The Components row
// offers "Clean up leftovers" only when this record says so: a component scan
// cannot tell whether Roundcube files are left on the server (the Agent's
// install check is the presence of three files, and a half tree reads as "not
// installed"), so without a record the cleanup stays on the component's own
// page and the row says nothing.
//
// The record is the newest of two things the Panel wrote: its newest Roundcube
// install or removal entry in the audit log, and the webmail profile's newest
// attempt. An install that failed, a removal that was partial or failed, or a
// failed webmail attempt leaves the cleanup offered until a later install, a
// clean removal or a webmail attempt that did not fail is recorded (alpha.96:
// before, a failed webmail attempt kept the offer even after a later clean
// removal). Nothing here changes the host.
//
// Panel'in, bitmeyen bir Roundcube kurulumu ya da kaldırması hakkındaki kendi
// kaydı. Bileşenler satırı "Kalıntıları temizle"yi yalnız bu kayıt varken
// sunar; kayıt yoksa temizlik bileşenin kendi sayfasında kalır.
const (
	webmailCleanupAuditWindow       = 20000
	webmailCleanupInstallFailed     = "install_failed"
	webmailCleanupRemovalIncomplete = "removal_incomplete"
)

// webmailCleanupAuditActions are the Roundcube install and removal actions
// the Panel writes (service_operations.go, service_install_handler.go). Each
// is matched exactly or followed by " — reason".
var webmailCleanupAuditActions = []struct {
	action string
	record string
}{
	{"service.install:roundcube", ""},
	{"service.install.recovered:roundcube", ""},
	{"service.install.failed:roundcube", webmailCleanupInstallFailed},
	{"service.install.recovered.failed:roundcube", webmailCleanupInstallFailed},
	{"service.uninstall:roundcube", ""},
	{"service.uninstall.partial:roundcube", webmailCleanupRemovalIncomplete},
	{"service.uninstall.failed:roundcube", webmailCleanupRemovalIncomplete},
}

// webmailCleanupFromAudit says what the newest Roundcube entry records: an
// unfinished install, an unfinished removal, or nothing to clean up.
func webmailCleanupFromAudit(action string) string {
	for _, known := range webmailCleanupAuditActions {
		if action == known.action || strings.HasPrefix(action, known.action+" ") {
			return known.record
		}
	}
	return ""
}

// webmailCleanupRecord reads the newest Roundcube install or removal entry.
// A read that fails is logged and offers nothing on the row; the cleanup
// stays reachable from the component's page.
func (p *Panel) webmailCleanupRecord(ctx context.Context) string {
	if p == nil || p.db == nil || p.db.GetDB() == nil {
		return ""
	}
	clauses := make([]string, 0, len(webmailCleanupAuditActions)*2)
	args := make([]any, 0, len(webmailCleanupAuditActions)*2)
	for _, known := range webmailCleanupAuditActions {
		clauses = append(clauses, "action = ?", "action LIKE ?")
		args = append(args, known.action, known.action+" %")
	}
	// The read is bounded to the newest webmailCleanupAuditWindow entries, a
	// range on the primary key: audit_logs has no index on action and is never
	// pruned, and this read runs on every managed-services answer. An older
	// unfinished operation is no longer offered on the row; the cleanup stays
	// on the component's own page.
	// Okuma en yeni webmailCleanupAuditWindow girdiyle sınırlıdır (birincil
	// anahtar aralığı); daha eski bir kayıt satırda sunulmaz, temizlik
	// bileşenin kendi sayfasında kalır.
	args = append([]any{webmailCleanupAuditWindow}, args...)
	var action, recordedAt string
	err := p.db.GetDB().QueryRowContext(ctx,
		`SELECT action, COALESCE(created_at, '') FROM audit_logs
		  WHERE id > (SELECT COALESCE(MAX(id), 0) FROM audit_logs) - ?
		    AND (`+strings.Join(clauses, " OR ")+`)
		  ORDER BY id DESC LIMIT 1`,
		args...,
	).Scan(&action, &recordedAt)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		log.Printf("read the Roundcube cleanup record: %v", err)
		return ""
	}
	audited := err == nil
	// The webmail profile's newest attempt, read as the mail profiles read it.
	var status, attemptedAt string
	attemptErr := p.db.GetDB().QueryRowContext(ctx, `
		SELECT status, COALESCE(NULLIF(finished_at, ''), NULLIF(updated_at, ''), started_at, '')
		  FROM service_operations
		 WHERE kind = ? AND service_id = 'webmail'
		 ORDER BY started_at DESC, updated_at DESC, created_at DESC, id DESC
		 LIMIT 1`,
		serviceOperationKindMailProfileInstall,
	).Scan(&status, &attemptedAt)
	if attemptErr != nil && !errors.Is(attemptErr, sql.ErrNoRows) {
		log.Printf("read the webmail attempt for the Roundcube cleanup record: %v", attemptErr)
	}
	attempted := attemptErr == nil
	switch {
	case attempted && audited:
		// The newer of the two decides; at an equal second the Roundcube
		// entry, the more specific one, decides.
		if webmailAttemptAfter(attemptedAt, recordedAt) {
			return webmailCleanupFromAttempt(status)
		}
		return webmailCleanupFromAudit(action)
	case attempted:
		return webmailCleanupFromAttempt(status)
	case audited:
		return webmailCleanupFromAudit(action)
	}
	return ""
}

// webmailCleanupFromAttempt: a failed webmail attempt may have left Roundcube
// files; any other state records nothing to clean up.
func webmailCleanupFromAttempt(status string) string {
	if status == serviceOperationFailed {
		return webmailCleanupInstallFailed
	}
	return ""
}

// webmailAttemptAfter compares the attempt's moment (RFC 3339, the Panel's
// clock) with the audit entry's (SQLite datetime('now'), UTC, whole seconds).
// A moment that cannot be read is not newer.
func webmailAttemptAfter(attemptedAt, recordedAt string) bool {
	attempt, err := time.Parse(time.RFC3339Nano, attemptedAt)
	if err != nil {
		return false
	}
	recorded, err := time.Parse("2006-01-02 15:04:05", recordedAt)
	if err != nil {
		if recorded, err = time.Parse(time.RFC3339Nano, recordedAt); err != nil {
			return true
		}
	}
	return attempt.UTC().Truncate(time.Second).After(recorded.UTC())
}
