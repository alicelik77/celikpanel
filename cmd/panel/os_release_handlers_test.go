package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.107 (D-038 §17): the first read records the release; after an OS
// release upgrade the same read says what changed with nothing changed by
// the Panel, until the administrator marks it reviewed.

func TestAnOSReleaseChangeIsSaidUntilItIsReviewed(t *testing.T) {
	panel, _, database, userID := newServiceStateAtomicPanel(t)
	restore := readOSReleaseFactsFromAgent
	t.Cleanup(func() { readOSReleaseFactsFromAgent = restore })
	facts := transport.OSReleaseFactsResponse{ID: "debian", VersionID: "13", Codename: "trixie", PrettyName: "Debian GNU/Linux 13 (trixie)",
		RepoSuites: map[string]string{"sury": "trixie", "pgdg": "trixie-pgdg"}, PostgreSQLClusters: []string{"17/main"}, PHPVersions: []string{"8.4"}, MariaDBVersion: "11.8"}
	readOSReleaseFactsFromAgent = func(*Panel, context.Context) (transport.OSReleaseFactsResponse, error) { return facts, nil }
	read := func(method, path string) string {
		recorder := httptest.NewRecorder()
		panel.handleOSRelease(recorder, serviceStateAtomicRequest(method, path, "", userID))
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s %s: %d %s", method, path, recorder.Code, recorder.Body.String())
		}
		return recorder.Body.String()
	}
	body := read(http.MethodGet, osReleaseAPIPath)
	if !strings.Contains(body, `"level":"tested"`) || !strings.Contains(body, `"changed":false`) || !strings.Contains(body, `"changes":[]`) || !strings.Contains(body, `"patched_until":"2030-06-30"`) {
		t.Fatalf("first read = %s", body)
	}
	facts = transport.OSReleaseFactsResponse{ID: "debian", VersionID: "14", Codename: "forky", PrettyName: "Debian GNU/Linux 14 (forky)",
		RepoSuites: map[string]string{"sury": "trixie", "pgdg": "forky-pgdg"}, PostgreSQLClusters: []string{"17/main", "18/main"}, PHPVersions: []string{"8.4", "8.5"}, MariaDBVersion: "12.1"}
	body = read(http.MethodGet, osReleaseAPIPath)
	for _, want := range []string{`"level":"not_listed"`, `"changed":true`, `"kind":"repo_suite","subject":"sury","from":"trixie","to":"forky"`,
		`"kind":"pg_cluster_new","subject":"18/main"`, `"kind":"php_added","subject":"8.5"`, `"kind":"mariadb_version","from":"11.8","to":"12.1"`, `"codename":"trixie"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("after the upgrade, %s is missing: %s", want, body)
		}
	}
	if strings.Contains(body, `"subject":"pgdg"`) {
		t.Fatalf("a repository already on the new release is not drift: %s", body)
	}
	body = read(http.MethodPost, osReleaseReviewedAPIPath)
	if !strings.Contains(body, `"changed":false`) || !strings.Contains(body, `"kind":"repo_suite","subject":"sury"`) {
		t.Fatalf("reviewed = %s", body)
	}
	var action string
	database.GetDB().QueryRow(`SELECT action FROM audit_logs ORDER BY id DESC LIMIT 1`).Scan(&action)
	if action != "server.os_release_reviewed:debian 13 (trixie)->debian 14 (forky)" {
		t.Fatalf("audit = %q", action)
	}
}
