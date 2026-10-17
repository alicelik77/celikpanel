package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The Components row offers the Roundcube leftovers cleanup only when the
// Panel's own record shows an install or removal that did not finish; the
// newest Roundcube entry decides, and a later clean install or removal ends it.
func TestWebmailCleanupRecordFollowsTheNewestRoundcubeEntry(t *testing.T) {
	for action, want := range map[string]string{
		"service.install:roundcube":                                   "",
		"service.install.failed:roundcube — roundcube_install_failed": webmailCleanupInstallFailed,
		"service.install.recovered.failed:roundcube":                  webmailCleanupInstallFailed,
		"service.uninstall:roundcube":                                 "",
		"service.uninstall.partial:roundcube — webmail cleanup":       webmailCleanupRemovalIncomplete,
		"service.uninstall.failed:roundcube — still installed":        webmailCleanupRemovalIncomplete,
		"service.uninstall.refresh.failed:roundcube — scan":           "",
		"service.install.failed:roundcubex":                           "",
		"service.install.failed:nginx — x":                            "",
	} {
		if got := webmailCleanupFromAudit(action); got != want {
			t.Fatalf("%q = %q, want %q", action, got, want)
		}
	}

	panel := newManagedServiceCachePanel(t)
	read := func() string {
		recorder := httptest.NewRecorder()
		panel.handleManagedServices(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/managed-services", nil))
		var payload map[string]any
		if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		value, _ := payload["webmail_cleanup"].(string)
		return value
	}
	record := func(action string) {
		if _, err := panel.db.GetDB().Exec(`INSERT INTO audit_logs (action, resource_type) VALUES (?, 'service')`, action); err != nil {
			t.Fatal(err)
		}
	}
	if got := read(); got != "" {
		t.Fatalf("no record = %q", got)
	}
	record("service.install:roundcube")
	record("service.uninstall.partial:roundcube — webmail configuration")
	record("service.uninstall.refresh.failed:roundcube — scan")
	record("service.install:nginx")
	if got := read(); got != webmailCleanupRemovalIncomplete {
		t.Fatalf("after a partial removal = %q", got)
	}
	if got := panel.webmailCleanupRecord(context.Background()); got != webmailCleanupRemovalIncomplete {
		t.Fatalf("record = %q", got)
	}
	record("service.uninstall:roundcube")
	if got := read(); got != "" {
		t.Fatalf("after a clean removal = %q", got)
	}
	record("service.install.failed:roundcube — roundcube_install_failed")
	if got := read(); got != webmailCleanupInstallFailed {
		t.Fatalf("after a failed install = %q", got)
	}
}

// The record read is bounded to the newest webmailCleanupAuditWindow audit
// entries (a primary-key range), so a long audit log is never scanned whole;
// an unfinished operation older than the window is no longer offered on the row.
func TestWebmailCleanupRecordReadIsBoundedToTheNewestEntries(t *testing.T) {
	panel := newManagedServiceCachePanel(t)
	db := panel.db.GetDB()
	if _, err := db.Exec(`INSERT INTO audit_logs (action, resource_type) VALUES ('service.install.failed:roundcube — x', 'service')`); err != nil {
		t.Fatal(err)
	}
	if got := panel.webmailCleanupRecord(context.Background()); got != webmailCleanupInstallFailed {
		t.Fatalf("inside the window = %q", got)
	}
	var last int64
	if err := db.QueryRow(`SELECT MAX(id) FROM audit_logs`).Scan(&last); err != nil {
		t.Fatal(err)
	}
	// Move the id sequence past the window with one row instead of 20000 inserts.
	if _, err := db.Exec(`INSERT INTO audit_logs (id, action, resource_type) VALUES (?, 'auth.login', 'user')`, last+webmailCleanupAuditWindow); err != nil {
		t.Fatal(err)
	}
	if got := panel.webmailCleanupRecord(context.Background()); got != "" {
		t.Fatalf("older than the window = %q", got)
	}
}

// alpha.96: the record is the newest of the Roundcube audit entry and the
// webmail profile's attempt, so a later clean removal ends the offer that a
// failed webmail attempt raised, and a later failed attempt raises it again.
func TestWebmailCleanupRecordIsTheNewestOfAuditAndWebmailAttempt(t *testing.T) {
	panel := newManagedServiceCachePanel(t)
	db := panel.db.GetDB()
	audit := func(action, at string) {
		t.Helper()
		if _, err := db.Exec(`INSERT INTO audit_logs (action, resource_type, created_at) VALUES (?, 'service', ?)`, action, at); err != nil {
			t.Fatal(err)
		}
	}
	attempt := func(id, status, at string) {
		t.Helper()
		if _, err := db.Exec(`INSERT INTO service_operations (id, kind, service_id, status, phase, started_at, created_at, updated_at, finished_at)
			VALUES (?, ?, 'webmail', ?, 'done', ?, ?, ?, ?)`, id, serviceOperationKindMailProfileInstall, status, at, at, at, at); err != nil {
			t.Fatal(err)
		}
	}
	ctx := context.Background()
	attempt("w1", serviceOperationFailed, "2026-10-17T10:00:00.5Z")
	if got := panel.webmailCleanupRecord(ctx); got != webmailCleanupInstallFailed {
		t.Fatalf("failed webmail attempt alone = %q", got)
	}
	audit("service.uninstall:roundcube", "2026-10-17 10:05:00")
	if got := panel.webmailCleanupRecord(ctx); got != "" {
		t.Fatalf("a later clean removal kept the offer: %q", got)
	}
	attempt("w2", serviceOperationFailed, "2026-10-17T11:00:00Z")
	if got := panel.webmailCleanupRecord(ctx); got != webmailCleanupInstallFailed {
		t.Fatalf("a later failed attempt = %q", got)
	}
	attempt("w3", serviceOperationSucceeded, "2026-10-17T12:00:00Z")
	if got := panel.webmailCleanupRecord(ctx); got != "" {
		t.Fatalf("a later attempt that did not fail = %q", got)
	}
	audit("service.uninstall.partial:roundcube — x", "2026-10-17 12:00:00")
	if got := panel.webmailCleanupRecord(ctx); got != webmailCleanupRemovalIncomplete {
		t.Fatalf("at an equal second the Roundcube entry decides: %q", got)
	}
}
