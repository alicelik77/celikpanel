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
