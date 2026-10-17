package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"strings"
)

// The Panel's own record of a Roundcube install or removal that did not
// finish (alpha.95, independent review of 2026-10-17). The Components row
// offers "Clean up leftovers" only when this record says so: a component scan
// cannot tell whether Roundcube files are left on the server (the Agent's
// install check is the presence of three files, and a half tree reads as "not
// installed"), so without a record the cleanup stays on the component's own
// page and the row says nothing.
//
// The record is the newest Roundcube install or removal entry the Panel wrote
// to its audit log: an install that failed, or a removal that was partial or
// failed, leaves the cleanup offered until a later install or a clean removal
// is recorded. Nothing here changes the host.
//
// Panel'in, bitmeyen bir Roundcube kurulumu ya da kaldırması hakkındaki kendi
// kaydı. Bileşenler satırı "Kalıntıları temizle"yi yalnız bu kayıt varken
// sunar; kayıt yoksa temizlik bileşenin kendi sayfasında kalır.
const (
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
	var action string
	err := p.db.GetDB().QueryRowContext(ctx,
		`SELECT action FROM audit_logs WHERE `+strings.Join(clauses, " OR ")+` ORDER BY id DESC LIMIT 1`,
		args...,
	).Scan(&action)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			log.Printf("read the Roundcube cleanup record: %v", err)
		}
		return ""
	}
	return webmailCleanupFromAudit(action)
}
