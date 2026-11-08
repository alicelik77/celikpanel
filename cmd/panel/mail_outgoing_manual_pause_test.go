package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/core"
)

// alpha.130: a pause an administrator or reseller set by hand (paused_reason
// 'manual') is never ended by a password change, whatever path changes the
// password; a security pause (spike, refusals) still is (owner decision of
// 7 Nov 2026, alpha.128/129).

func TestOutgoingManualPauseIsNotEndedByAPasswordChange(t *testing.T) {
	f := newOutgoingFixture(t)
	f.enable(t)
	ctx := context.Background()
	if _, err := f.db.Exec(`
		INSERT INTO mailbox_outgoing (email_account_id, paused_at, paused_reason, paused_detail) VALUES (9901, datetime('now'), 'manual', 'stopped by the administrator');
		INSERT INTO mailbox_outgoing (email_account_id, paused_at, paused_reason, paused_detail) VALUES (9903, datetime('now'), 'refusals', 'limit refusals')`); err != nil {
		t.Fatal(err)
	}
	resumed, err := f.p.resumeOutgoingAfterPassword(ctx, 9901)
	if err != nil || resumed {
		t.Fatalf("a manual pause was ended by a password change: %v %v", resumed, err)
	}
	var reason, pausedAt string
	if err := f.db.QueryRow(`SELECT COALESCE(paused_at, ''), COALESCE(paused_reason, '') FROM mailbox_outgoing WHERE email_account_id = 9901`).Scan(&pausedAt, &reason); err != nil || pausedAt == "" || reason != "manual" {
		t.Fatalf("manual pause after the change: %q %q %v", pausedAt, reason, err)
	}
	if len(f.applied) != 0 {
		t.Fatalf("postfwd was given a new state for a pause that stays: %+v", f.applied)
	}
	// A security pause still ends with a new password.
	if resumed, err := f.p.resumeOutgoingAfterPassword(ctx, 9903); err != nil || !resumed {
		t.Fatalf("a security pause did not end: %v %v", resumed, err)
	}
	if last := f.applied[len(f.applied)-1]; len(last.Paused) != 1 || last.Paused[0] != "a@example.com" {
		t.Fatalf("postfwd must keep only the manual pause: %+v", last)
	}
}

// Every panel path that changes a mailbox password (an administrator, a
// reseller, the customer, a team member or a mail API token: all reach
// handleUpdateEmailPassword through the "mail" gate) keeps a manual pause and
// says so; the recovery reset is TestMailboxRecoveryKeepsAManualPause.
func TestPanelPasswordChangeKeepsAManualPauseForEveryRole(t *testing.T) {
	for _, role := range []string{roleAdmin, roleReseller, roleCustomer} {
		t.Run(role, func(t *testing.T) {
			agent := &mailMutationPanelAgent{passwordApplied: true}
			panel, domainID := newMailMutationPanel(t, agent)
			if _, err := panel.db.GetDB().Exec(`
				INSERT INTO email_accounts (id, domain_id, address, password_hash, quota_mb) VALUES (6501, ?, 'user@example.com', 'managed-by-agent', 100);
				INSERT INTO mailbox_outgoing (email_account_id, paused_at, paused_reason) VALUES (6501, datetime('now'), 'manual')`, domainID); err != nil {
				t.Fatal(err)
			}
			previous := buildCommit
			buildCommit = "manual-pause-test"
			t.Cleanup(func() { buildCommit = previous })
			request := httptest.NewRequest(http.MethodPut, "/mail/accounts/password", strings.NewReader(`{"id":6501,"new_password":"Replacement-password-1!"}`))
			request = request.WithContext(context.WithValue(request.Context(), callerKey, &Caller{ID: 6101, Role: role, AccountType: core.AccountTypeAccount}))
			response := httptest.NewRecorder()
			panel.handleUpdateEmailPassword(response, request, domainID)
			if response.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			var answer map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
				t.Fatal(err)
			}
			if answer["sending_paused"] != true || answer["sending_resumed"] != nil {
				t.Fatalf("answer = %v", answer)
			}
			var reason string
			if err := panel.db.GetDB().QueryRow(`SELECT COALESCE(paused_reason, '') FROM mailbox_outgoing WHERE email_account_id = 6501 AND paused_at IS NOT NULL`).Scan(&reason); err != nil || reason != "manual" {
				t.Fatalf("pause after the change: %q %v", reason, err)
			}
		})
	}
}
