package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Re-review: a shared user stays by design; when no answer comes back about
// its rights on the deleted database, the result says exactly that
// ("unknown_rights"), never "not known whether they were removed".
type sharedNoAnswerNative struct {
	fakeDatabaseAccessNative
}

func (n *sharedNoAnswerNative) remove(ctx context.Context, req transport.DatabaseAccessUserRequest) (transport.DatabaseAccessUserResponse, error) {
	if req.RightsOnly {
		return transport.DatabaseAccessUserResponse{}, errors.New("read: connection reset mid-call")
	}
	return n.fakeDatabaseAccessNative.remove(ctx, req)
}

func TestDatabaseDeleteWithNoAnswerAboutASharedUserSaysItsRightsAreUnknown(t *testing.T) {
	f := newVisibleDatabasesFixture(t)
	recordDatabaseDrops(t)
	f.panel.databaseAccessNative = &sharedNoAnswerNative{}
	got := f.as(t, "admin", http.MethodDelete, "/api/v1/databases/9961", "")
	var body map[string]any
	_ = json.Unmarshal(got.Body.Bytes(), &body)
	vars, _ := body["vars"].(map[string]any)
	if got.Code != http.StatusConflict || body["reason"] != "unknown_rights" || vars["rights"] != "wp_user" || vars["pending"] != "" {
		t.Fatalf("%d %s", got.Code, got.Body.String())
	}
	// cust_user went; wp_user's record and its other database stay.
	if f.count(t, `SELECT COUNT(*) FROM database_user_grants WHERE user_id = 9942 AND database_id = 9962`) != 1 {
		t.Fatal("the shared user's other database lost its grant record")
	}
}
