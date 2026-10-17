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
