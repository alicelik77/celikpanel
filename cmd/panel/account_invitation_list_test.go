package main

import (
	"encoding/json"
	"net/http"
	"testing"
)

// The users and team member lists say which accounts were invited and have
// not chosen a password yet, and the state follows resend, cancel and accept.
func TestListsMarkInvitedAccounts(t *testing.T) {
	f := newRecoveryFixture(t)
	f.setMail(false, nil)
	admin := f.tokens[authzMatrixAdminID]

	res := f.do(t, http.MethodPost, "/api/v1/users", admin,
		`{"username":"invited-list","email":"invited-list@example.test","role":"customer","invite":true}`)
	var created struct {
		ID         int               `json:"id"`
		Invitation invitationOutcome `json:"invitation"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &created); err != nil || res.Code != http.StatusOK {
		t.Fatalf("create: %d %s", res.Code, res.Body)
	}

	listed := func() map[int]*accountInvitationState {
		t.Helper()
		res := f.do(t, http.MethodGet, "/api/v1/users", admin, "")
		var body struct {
			Users []userResponse `json:"users"`
		}
		if res.Code != http.StatusOK || json.Unmarshal(res.Body.Bytes(), &body) != nil {
			t.Fatalf("users list: %d %s", res.Code, res.Body)
		}
		out := map[int]*accountInvitationState{}
		for _, u := range body.Users {
			out[u.ID] = u.Invitation
		}
		return out
	}

	got := listed()
	if s := got[created.ID]; s == nil || s.State != "pending" || s.ID != created.Invitation.ID || s.ExpiresAt == "" {
		t.Fatalf("invited account: %+v", s)
	}
	if s := got[authzMatrixCustomerID]; s != nil {
		t.Fatalf("an ordinary account is marked invited: %+v", s)
	}

	// Cancel: still not signed in, and no link to act on.
	if code := f.do(t, http.MethodDelete, invitationsPath+"/"+recoveryItoa(int(created.Invitation.ID)), admin, "").Code; code != http.StatusNoContent {
		t.Fatalf("revoke: %d", code)
	}
	if s := listed()[created.ID]; s == nil || s.State != "cancelled" || s.ID != 0 {
		t.Fatalf("after cancel: %+v", s)
	}

	// A team member invited by its owner shows the same mark in the team list.
	customer := f.tokens[authzMatrixCustomerID]
	res = f.do(t, http.MethodPost, "/api/v1/team-members", customer,
		`{"username":"invited-teammate","email":"invited-teammate@example.test","invite":true,"access":{"subscription_permissions":[],"domain_permissions":[]}}`)
	var member struct {
		TeamMember struct {
			ID int `json:"id"`
		} `json:"team_member"`
		Invitation invitationOutcome `json:"invitation"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &member); err != nil || res.Code != http.StatusCreated {
		t.Fatalf("team member: %d %s", res.Code, res.Body)
	}
	teamState := func() *accountInvitationState {
		t.Helper()
		res := f.do(t, http.MethodGet, "/api/v1/team-members", customer, "")
		var body struct {
			TeamMembers []struct {
				ID         int                     `json:"id"`
				Username   string                  `json:"username"`
				Invitation *accountInvitationState `json:"invitation"`
			} `json:"team_members"`
		}
		if res.Code != http.StatusOK || json.Unmarshal(res.Body.Bytes(), &body) != nil {
			t.Fatalf("team list: %d %s", res.Code, res.Body)
		}
		for _, m := range body.TeamMembers {
			if m.ID == member.TeamMember.ID {
				if m.Username != "invited-teammate" {
					t.Fatalf("member fields lost: %+v", m)
				}
				return m.Invitation
			}
		}
		t.Fatal("member not listed")
		return nil
	}
	if s := teamState(); s == nil || s.State != "pending" || s.ID != member.Invitation.ID {
		t.Fatalf("invited member: %+v", s)
	}

	// Accepting the invitation removes the mark.
	token := member.Invitation.LinkPath[len("/account-link/invitation#t="):]
	if code := f.do(t, http.MethodPost, recoveryAcceptInvitePath, "", recoveryBody(map[string]string{"token": token, "new_password": "my own password"})).Code; code != http.StatusOK {
		t.Fatalf("accept: %d", code)
	}
	if s := teamState(); s != nil {
		t.Fatalf("accepted member still marked: %+v", s)
	}
}
