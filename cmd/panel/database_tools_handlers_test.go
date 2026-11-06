package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/backupspec"
	"github.com/alicelik/celikpanel/internal/core"
)

// alpha.128 (G7): the tools of one database are domain sub-routes: one
// ownership chokepoint, the plan's databases permission, a team member only
// through the Databases capability (viewing for the numbers, managing for
// taking data out or changing it), and a guarded identity for the changes.
func TestDatabaseToolsRoutesFollowRightsAndTeamCapability(t *testing.T) {
	rows := []struct {
		method, path, kind string
		mode               core.TeamPermissionMode
		right              string
		guarded            bool
	}{
		{http.MethodGet, "/api/v1/domains/7/databases/3/tools", "database-tools", core.TeamPermissionView, "", false},
		{http.MethodPost, "/api/v1/domains/7/databases/3/export", "database-export", core.TeamPermissionManage, rightDatabases, true},
		{http.MethodGet, "/api/v1/domains/7/databases/3/export", "database-export", core.TeamPermissionManage, rightDatabases, false},
		{http.MethodPost, "/api/v1/domains/7/databases/3/import/upload", "database-import-upload", core.TeamPermissionManage, rightDatabases, false},
		{http.MethodPost, "/api/v1/domains/7/databases/3/import", "database-import", core.TeamPermissionManage, rightDatabases, true},
		{http.MethodPost, "/api/v1/domains/7/databases/3/copy", "database-copy", core.TeamPermissionManage, rightDatabases, true},
		{http.MethodPost, "/api/v1/domains/7/databases/3/check", "database-check", core.TeamPermissionManage, rightDatabases, true},
	}
	for _, row := range rows {
		request := httptest.NewRequest(row.method, row.path, nil)
		match, ok := matchDomainSubroute(request)
		if !ok || match.kind != row.kind || match.domainID != 7 || !routeAllows(row.method, match.methods) {
			t.Errorf("%s %s: matched %+v, %v", row.method, row.path, match, ok)
			continue
		}
		requirement, ok := teamMemberDomainRequirements[teamMemberRouteKey{kind: row.kind, method: row.method}]
		if !ok || requirement.capability != core.TeamCapabilityDatabases || requirement.mode != row.mode {
			t.Errorf("%s %s: team requirement %+v, %v", row.method, row.path, requirement, ok)
		}
		if got := domainSubrouteRightFor(row.kind, row.method); got != row.right {
			t.Errorf("%s %s: right %q, want %q", row.method, row.path, got, row.right)
		}
		_, guarded := requestIdentityRouteFor(request)
		if guarded != row.guarded {
			t.Errorf("%s %s: guarded %v, want %v", row.method, row.path, guarded, row.guarded)
		}
	}
	for _, path := range []string{
		"/api/v1/domains/7/databases/0/tools", "/api/v1/domains/7/databases/03/tools",
		"/api/v1/domains/7/databases/3/drop", "/api/v1/domains/7/databases/3/import/other",
		"/api/v1/domains/7/databases/3/tools/x/y",
	} {
		if match, ok := matchDomainSubroute(httptest.NewRequest(http.MethodGet, path, nil)); ok {
			t.Errorf("%s matched %+v", path, match)
		}
	}
}

func TestDatabaseCopyDigestLeavesThePasswordOut(t *testing.T) {
	a := requestIdentityDatabaseCopyDigest([]byte(`{"name":"test","password":"Secret#1"}`))
	b := requestIdentityDatabaseCopyDigest([]byte(`{"name":"test","password":"Other#22"}`))
	if string(a) != string(b) || strings.Contains(string(a), "Secret") {
		t.Fatalf("digest carries the password: %q %q", a, b)
	}
}

func TestDatabaseExportFilename(t *testing.T) {
	got := databaseExportFilename("example_com_shop", time.Date(2026, 11, 7, 21, 5, 0, 0, time.UTC))
	if got != "example_com_shop-20261107-2105.sql.gz" {
		t.Fatalf("filename = %q", got)
	}
}

// A refused restore says how, with the engine's reason (alpha.127 lab: a
// refused PostgreSQL restore answered a generic INTERNAL).
func TestWriteRestoreFailureIsTyped(t *testing.T) {
	cases := map[string]string{
		backupspec.RestoreCodeSafetyFailed:   errCodeRestoreSafetyFailed,
		backupspec.RestoreCodeLoadReverted:   errCodeRestoreLoadReverted,
		backupspec.RestoreCodeRollbackFailed: errCodeRestoreRollbackFailed,
		backupspec.RestoreCodeInvalid:        errCodeRestoreInvalid,
		backupspec.RestoreCodeFinishFailed:   errCodeRestoreFinishFailed,
	}
	for code, want := range cases {
		recorder := httptest.NewRecorder()
		if !writeRestoreFailure(recorder, backupspec.RestoreResponse{Code: code, Detail: `ERROR: relation "t" already exists`, SafetyBackup: &backupspec.Info{Name: "database-3-x"}}) {
			t.Fatalf("%s: untyped", code)
		}
		var body apiErrorBody
		if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Code != want || len(body.Details) != 1 || !strings.Contains(body.Details[0], "already exists") || recorder.Code < 400 {
			t.Errorf("%s: %d %+v", code, recorder.Code, body)
		}
	}
	if writeRestoreFailure(httptest.NewRecorder(), backupspec.RestoreResponse{Error: "x"}) {
		t.Fatal("an untyped failure was answered as typed")
	}
}
