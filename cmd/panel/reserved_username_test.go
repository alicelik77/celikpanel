package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/core"
)

// The reserved audit actor name is refused with its own typed code on every
// path that creates or renames a user, and the unrelated format refusals keep
// their explicit messages.
// Ayrılmış denetim aktörü adı, kullanıcı oluşturan ya da yeniden adlandıran her
// yolda kendi kodlu cevabıyla reddedilir; ilgisiz biçim retleri açık iletilerini
// korur.
func TestReservedUsernameIsRefusedWithItsOwnCode(t *testing.T) {
	decode := func(t *testing.T, rec *httptest.ResponseRecorder) apiErrorBody {
		t.Helper()
		var body apiErrorBody
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("body %q: %v", rec.Body.String(), err)
		}
		return body
	}

	t.Run("account creation", func(t *testing.T) {
		for _, name := range []string{"system", "SYSTEM"} {
			payload := `{"username":"` + name + `","email":"a@example.com","password":"correct-horse","role":"customer"}`
			req := httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(payload))
			rec := httptest.NewRecorder()
			(&Panel{}).handleCreateUser(rec, req, &Caller{ID: 1, Role: roleAdmin})
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("%s: status %d, body %s", name, rec.Code, rec.Body.String())
			}
			if body := decode(t, rec); body.Code != errCodeUsernameReserved || body.Error == "" {
				t.Fatalf("%s: body %+v", name, body)
			}
		}
	})

	t.Run("team member validation", func(t *testing.T) {
		err := validateTeamMemberIdentity("System", "a@example.com", "correct-horse", "active", true)
		if err == nil {
			t.Fatal("identity: reserved name accepted")
		}
		rec := httptest.NewRecorder()
		writeUsernameError(rec, err)
		if rec.Code != http.StatusBadRequest || decode(t, rec).Code != errCodeUsernameReserved {
			t.Fatalf("identity: %d %s", rec.Code, rec.Body.String())
		}
		name := " system "
		if err := validateTeamMemberUpdateRequest(updateTeamMemberRequest{Username: &name}); err == nil {
			t.Fatal("update: reserved name accepted")
		}
		formatErr := validateTeamMemberIdentity("bad name", "a@example.com", "correct-horse", "active", true)
		if formatErr == nil || formatErr.Error() != "invalid username" {
			t.Fatalf("a format error keeps its message: %v", formatErr)
		}
		rec = httptest.NewRecorder()
		writeUsernameError(rec, formatErr)
		if body := decode(t, rec); body.Code != "" || body.Error != "invalid username" {
			t.Fatalf("a format error is not the reserved code: %+v", body)
		}
	})
}

// A member that already carries the reserved name (nothing in the Panel
// creates one) keeps working: only a new or a changed name is refused.
// Ayrılmış ada zaten sahip bir üye (Panel'de hiçbir şey oluşturmaz) çalışmaya
// devam eder: yalnız yeni ya da değişen ad reddedilir.
func TestReservedUsernameDoesNotBreakAnExistingMember(t *testing.T) {
	p := newDNSPanelForTest(t)
	ownerID := createTeamMemberHandlerUser(t, p, roleCustomer, core.AccountTypeAccount, nil, "reserved-owner")
	subscriptionID, domainID := createTeamMemberHandlerScope(t, p, ownerID, "reserved-owner")
	caller := &Caller{ID: ownerID, Role: roleCustomer, AccountType: core.AccountTypeAccount, CustomerID: ownerID}

	// A new member named system is refused with the typed code.
	refused := httptest.NewRecorder()
	p.handleTeamMembers(refused, teamMemberHandlerRequest(http.MethodPost, "/api/v1/team-members",
		teamMemberCreateBody(nil, "System", subscriptionID, domainID), caller))
	var refusedBody apiErrorBody
	if refused.Code != http.StatusBadRequest || json.Unmarshal(refused.Body.Bytes(), &refusedBody) != nil || refusedBody.Code != errCodeUsernameReserved {
		t.Fatalf("create: %d %s", refused.Code, refused.Body.String())
	}

	// An ordinary member cannot be renamed to it.
	created := httptest.NewRecorder()
	p.handleTeamMembers(created, teamMemberHandlerRequest(http.MethodPost, "/api/v1/team-members",
		teamMemberCreateBody(nil, "ordinary-member", subscriptionID, domainID), caller))
	if created.Code != http.StatusCreated {
		t.Fatalf("create ordinary: %d %s", created.Code, created.Body.String())
	}
	ordinary := decodeTeamMemberHandlerResponse(t, created)
	rename := httptest.NewRecorder()
	p.handleTeamMemberByID(rename, teamMemberHandlerRequest(http.MethodPut,
		fmt.Sprintf("/api/v1/team-members/%d", ordinary.ID), `{"username":"system"}`, caller))
	if rename.Code != http.StatusBadRequest || !strings.Contains(rename.Body.String(), errCodeUsernameReserved) {
		t.Fatalf("rename: %d %s", rename.Code, rename.Body.String())
	}

	// A row that already has the name (inserted below the API) is still edited
	// while the name is unchanged.
	if _, err := p.db.GetDB().Exec(`UPDATE users SET username = 'system' WHERE id = ?`, ordinary.ID); err != nil {
		t.Fatal(err)
	}
	keep := httptest.NewRecorder()
	p.handleTeamMemberByID(keep, teamMemberHandlerRequest(http.MethodPut,
		fmt.Sprintf("/api/v1/team-members/%d", ordinary.ID), `{"username":"system","status":"suspended"}`, caller))
	if keep.Code != http.StatusOK {
		t.Fatalf("unchanged reserved name: %d %s", keep.Code, keep.Body.String())
	}
}
