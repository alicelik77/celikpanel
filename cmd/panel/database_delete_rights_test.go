package main

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.133 lab finding: a kept account must not keep any right on the
// deleted database. The deletion asks the Agent to take away every right that
// names it (DatabaseDropped); when one cannot be removed the answer is the
// partial outcome with that fact (reason "rights"), never "access removed",
// and pressing Delete again finishes it.

type rightsLeftNative struct {
	fakeDatabaseAccessNative
	leftOnce bool
	asked    []transport.DatabaseAccessUserRequest
}

func (n *rightsLeftNative) remove(_ context.Context, req transport.DatabaseAccessUserRequest) (transport.DatabaseAccessUserResponse, error) {
	n.asked = append(n.asked, req)
	if n.leftOnce {
		n.leftOnce = false
		return transport.DatabaseAccessUserResponse{RightsLeft: true, Error: "1 right(s) on the database are still recorded for the kept user"}, nil
	}
	return transport.DatabaseAccessUserResponse{Applied: true, Kept: true, KeptReason: transport.DatabaseAccessKeptOtherRights}, nil
}

func TestDatabaseDeleteSaysARightLeftOnAKeptAccountAndFinishesOnTheNextPress(t *testing.T) {
	f := newVisibleDatabasesFixture(t)
	recordDatabaseDrops(t)
	native := &rightsLeftNative{leftOnce: true}
	f.panel.databaseAccessNative = native

	got := f.as(t, "admin", http.MethodDelete, "/api/v1/databases/9961", "")
	var body map[string]any
	_ = json.Unmarshal(got.Body.Bytes(), &body)
	if got.Code != http.StatusConflict || body["code"] != errCodeDatabaseDeleteUsersLeft || body["reason"] != "rights" || body["mutation_applied"] != true {
		t.Fatalf("%d %s", got.Code, got.Body.String())
	}
	if len(native.asked) == 0 || !native.asked[0].DatabaseDropped {
		t.Fatalf("the Agent was not told the database is dropped: %+v", native.asked)
	}
	if f.count(t, `SELECT COUNT(*) FROM databases_v2 WHERE id = 9961`) != 1 {
		t.Fatal("the record went while a right on the database is left")
	}

	got = f.as(t, "admin", http.MethodDelete, "/api/v1/databases/9961", "")
	if got.Code != http.StatusOK {
		t.Fatalf("again: %d %s", got.Code, got.Body.String())
	}
	var answer struct {
		KeptOnEngine []string `json:"users_kept_on_engine"`
	}
	_ = json.Unmarshal(got.Body.Bytes(), &answer)
	if len(answer.KeptOnEngine) != 1 || answer.KeptOnEngine[0] != "cust_user" {
		t.Fatalf("answer %s", got.Body.String())
	}
}

// The lab round's review, item 1: a shared user stays but loses CelikPanel's
// grant on the deleted database (rights only, no DROP USER). A failure is a
// truthful partial result ("rights"); Delete again finishes it.
type sharedRightsNative struct {
	fakeDatabaseAccessNative
	failShared bool
	exists     bool
}

func (n *sharedRightsNative) remove(ctx context.Context, req transport.DatabaseAccessUserRequest) (transport.DatabaseAccessUserResponse, error) {
	if n.exists {
		n.removed = append(n.removed, req)
		return transport.DatabaseAccessUserResponse{DatabaseExists: true, Error: "the database still exists; nothing was changed"}, nil
	}
	if req.RightsOnly && n.failShared {
		n.removed = append(n.removed, req)
		return transport.DatabaseAccessUserResponse{RightsLeft: true, Error: "1 right(s) still loaded"}, nil
	}
	return n.fakeDatabaseAccessNative.remove(ctx, req)
}

func TestDatabaseDeleteTakesASharedUsersRightsOnTheDatabaseAndSaysWhenItCannot(t *testing.T) {
	f := newVisibleDatabasesFixture(t)
	recordDatabaseDrops(t)
	native := &sharedRightsNative{failShared: true}
	f.panel.databaseAccessNative = native
	got := f.as(t, "admin", http.MethodDelete, "/api/v1/databases/9961", "")
	var body map[string]any
	_ = json.Unmarshal(got.Body.Bytes(), &body)
	vars, _ := body["vars"].(map[string]any)
	if got.Code != http.StatusConflict || body["reason"] != "rights" || vars["rights"] != "wp_user" || vars["pending"] != "" {
		t.Fatalf("%d %s", got.Code, got.Body.String())
	}
	// The shared user is asked for its rights only; the grant on the blog
	// database is not the Agent's to touch (it removes by this name only).
	var shared transport.DatabaseAccessUserRequest
	for _, req := range native.removed {
		if req.Username == "wp_user" {
			shared = req
		}
	}
	if !shared.RightsOnly || !shared.DatabaseDropped || shared.Database != "customer_matrix_test_shop" {
		t.Fatalf("shared user request %+v", shared)
	}
	if f.count(t, `SELECT COUNT(*) FROM database_user_grants WHERE user_id = 9942 AND database_id = 9962`) != 1 {
		t.Fatal("the shared user's other database lost its grant record")
	}
	native.failShared = false
	if got := f.as(t, "admin", http.MethodDelete, "/api/v1/databases/9961", ""); got.Code != http.StatusOK {
		t.Fatalf("again: %d %s", got.Code, got.Body.String())
	}
}

// Review item 2: the Agent finds a database of that name on the engine and
// removes nothing; the answer says so (reason "exists"), the records stay.
func TestDatabaseDeleteSaysTheDatabaseIsStillThere(t *testing.T) {
	f := newVisibleDatabasesFixture(t)
	recordDatabaseDrops(t)
	f.panel.databaseAccessNative = &sharedRightsNative{exists: true}
	got := f.as(t, "admin", http.MethodDelete, "/api/v1/databases/9961", "")
	var body map[string]any
	_ = json.Unmarshal(got.Body.Bytes(), &body)
	if got.Code != http.StatusConflict || body["reason"] != "exists" {
		t.Fatalf("%d %s", got.Code, got.Body.String())
	}
	if f.count(t, `SELECT COUNT(*) FROM databases_v2 WHERE id = 9961`) != 1 {
		t.Fatal("records went while the database is still there")
	}
}
