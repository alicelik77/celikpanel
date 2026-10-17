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
