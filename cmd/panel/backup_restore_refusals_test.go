package main

import (
	"net/http"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/backupspec"
)

// alpha.134 lab finding: a backup that is not this site's (another site's
// name, or its package put in this site's folder) was refused by the Agent,
// and the screen said "internal error". It is now said as such, before the
// program is touched, and nothing is restored.
func TestInspectionRefusalsAreSaidAsSuch(t *testing.T) {
	cases := []struct {
		code, agentError string
		status           int
		want             string
	}{
		{backupspec.RestoreCodeNotFound, "lstat /var/backups/celikpanel/subscriptions/3/domains/6: no such file or directory", http.StatusNotFound, errCodeRestoreNotFound},
		{backupspec.RestoreCodeOtherSite, "manifest scope mismatch", http.StatusConflict, errCodeRestoreOtherSite},
		{backupspec.RestoreCodeInvalid, "gzip: invalid header", http.StatusConflict, errCodeRestoreInvalid},
	}
	for _, c := range cases {
		t.Run(c.code, func(t *testing.T) {
			f := newProgramRestoreFixture(t, true, "app")
			f.agent.inspectResp = backupspec.InspectResponse{Error: c.agentError, Code: c.code}
			rec := f.restore(t)
			body := decodeRestoreAnswer(t, rec)
			if rec.Code != c.status || body["code"] != c.want {
				t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), "/var/backups") || strings.Contains(rec.Body.String(), "manifest") {
				t.Fatalf("the Agent's own words reached the screen: %s", rec.Body.String())
			}
			if len(f.agent.restoreReqs) != 0 || len(f.controls()) != 0 {
				t.Fatalf("something ran: restores %d, controls %v", len(f.agent.restoreReqs), f.controls())
			}
		})
	}
}

// An inspection error the Agent did not type stays the server's own error.
func TestUntypedInspectionErrorStaysAServerError(t *testing.T) {
	f := newProgramRestoreFixture(t, true, "app")
	f.agent.inspectResp = backupspec.InspectResponse{Error: "read /var/backups/x: input/output error"}
	rec := f.restore(t)
	if rec.Code != http.StatusInternalServerError || len(f.agent.restoreReqs) != 0 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
}

// The Agent's restore refusals of this release are said with their own code,
// and the program the restore stopped is started again on its files.
func TestRestoreRefusalsOfTheAgentAreSaidAsSuch(t *testing.T) {
	cases := []struct {
		code   string
		status int
		want   string
	}{
		{backupspec.RestoreCodeNotFound, http.StatusNotFound, errCodeRestoreNotFound},
		{backupspec.RestoreCodeOtherSite, http.StatusConflict, errCodeRestoreOtherSite},
		{backupspec.RestoreCodeNoSpace, http.StatusConflict, errCodeRestoreNoSpace},
		{backupspec.RestoreCodeSiteAccount, http.StatusConflict, errCodeRestoreSiteAccount},
	}
	for _, c := range cases {
		t.Run(c.code, func(t *testing.T) {
			f := newProgramRestoreFixture(t, true, "app")
			f.agent.restoreResp = backupspec.RestoreResponse{Code: c.code, Error: "refused"}
			rec := f.restore(t)
			body := decodeRestoreAnswer(t, rec)
			if rec.Code != c.status || body["code"] != c.want || errorVars(body)["program"] != backupProgramStarted {
				t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
			}
			if got := f.controls(); strings.Join(got, ",") != "control:stop,control:start" {
				t.Fatalf("controls = %v", got)
			}
		})
	}
}

// Restored, but the program did not start: a verified failure that says the
// files came back and why the program does not run, never "unknown".
func TestRestoredProgramThatDoesNotStartIsAVerifiedFailure(t *testing.T) {
	f := newProgramRestoreFixture(t, true, "app")
	f.agent.restoreResp = backupspec.RestoreResponse{Success: true, Restored: []string{"app"}}
	f.rt.mu.Lock()
	f.rt.exposed = true
	f.rt.mu.Unlock()
	rec := f.restore(t)
	body := decodeRestoreAnswer(t, rec)
	program, _ := body["program"].(map[string]any)
	restored, _ := body["restored"].([]any)
	if rec.Code != http.StatusOK || program["state"] != backupProgramNotStarted || program["reason"] != "listens_elsewhere" ||
		len(restored) != 1 || restored[0] != "app" {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
}
