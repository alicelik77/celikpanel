package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// The review of alpha.133 round 5 (W2, W3, W4, W7, N1, N4), on the production
// routes with the Agent faked.

// keptNative answers removals like the Agent, keeping the named accounts
// for rights their owner gave by hand.
type keptNative struct {
	fakeDatabaseAccessNative
	keep map[string]bool
}

func (k *keptNative) remove(ctx context.Context, req transport.DatabaseAccessUserRequest) (transport.DatabaseAccessUserResponse, error) {
	if k.keep[req.Username] {
		k.removed = append(k.removed, req)
		return transport.DatabaseAccessUserResponse{Applied: true, Kept: true, KeptReason: transport.DatabaseAccessKeptOtherRights}, nil
	}
	return k.fakeDatabaseAccessNative.remove(ctx, req)
}

func auditDetails(t *testing.T, f visibleDatabasesFixture, action string) map[string]any {
	t.Helper()
	var details string
	if err := f.panel.db.GetDB().QueryRow(`SELECT COALESCE(details, '') FROM audit_logs WHERE action = ? ORDER BY id DESC LIMIT 1`, action).Scan(&details); err != nil {
		t.Fatalf("%s: %v", action, err)
	}
	var parsed map[string]any
	_ = json.Unmarshal([]byte(details), &parsed)
	return parsed
}

// W2: "only this database uses it" is decided by the engine account (the
// engine and the name), across every record of that engine; a grant linking
// a user record of another server record keeps that user too.
func TestDatabaseDeleteKeepsAnAccountAnotherRecordOfTheSameEngineNames(t *testing.T) {
	f := newVisibleDatabasesFixture(t)
	recordDatabaseDrops(t)
	native := &fakeDatabaseAccessNative{}
	f.panel.databaseAccessNative = native
	// Server 9923 (another subscription) is the same local MariaDB; a record
	// there names cust_user too, on its own database.
	f.exec(t, `INSERT INTO database_users (id, server_id, subscription_id, username, password) VALUES (9951, 9923, 9804, 'cust_user', 'x')`)
	f.exec(t, `INSERT INTO database_user_grants (database_id, user_id, privileges) VALUES (9963, 9951, 'ALL')`)
	f.exec(t, `INSERT INTO database_user_grants (database_id, user_id, privileges) VALUES (9961, 9944, 'ALL')`)
	// The list says it before the deletion, decided the same way.
	fates := f.panel.databaseUserFates(context.Background(), []int{9961})
	if fates[9961][9941] != databaseUserShared || fates[9961][9944] != databaseUserShared || fates[9961][9942] != databaseUserShared {
		t.Fatalf("fates %v", fates)
	}
	got := f.as(t, "admin", http.MethodDelete, "/api/v1/databases/9961", "")
	if got.Code != http.StatusOK {
		t.Fatalf("%d %s", got.Code, got.Body.String())
	}
	// Every user stays; each loses only its rights on this database.
	for _, req := range native.removed {
		if !req.RightsOnly || !req.DatabaseDropped {
			t.Fatalf("a staying user was asked for more than its rights: %+v", req)
		}
	}
	if len(native.removed) != 3 {
		t.Fatalf("removed %+v", native.removed)
	}
	var answer struct {
		Kept []string `json:"users_kept"`
	}
	_ = json.Unmarshal(got.Body.Bytes(), &answer)
	if strings.Join(answer.Kept, ",") != "cust_user,wp_user,other_user" {
		t.Fatalf("answer %s", got.Body.String())
	}
	if f.count(t, `SELECT COUNT(*) FROM database_users WHERE id IN (9941, 9944, 9951)`) != 3 {
		t.Fatal("a kept user's record went")
	}
}

// W3: an account the Agent keeps for rights the owner gave by hand is said
// as kept on the engine; its record goes (it is no longer CelikPanel's).
func TestDatabaseDeleteReportsAnAccountKeptOnTheEngine(t *testing.T) {
	f := newVisibleDatabasesFixture(t)
	recordDatabaseDrops(t)
	f.panel.databaseAccessNative = &keptNative{keep: map[string]bool{"cust_user": true}}
	got := f.as(t, "admin", http.MethodDelete, "/api/v1/databases/9961", "")
	if got.Code != http.StatusOK {
		t.Fatalf("%d %s", got.Code, got.Body.String())
	}
	var answer struct {
		Removed      []string `json:"users_removed"`
		KeptOnEngine []string `json:"users_kept_on_engine"`
	}
	_ = json.Unmarshal(got.Body.Bytes(), &answer)
	if len(answer.Removed) != 0 || strings.Join(answer.KeptOnEngine, ",") != "cust_user" {
		t.Fatalf("answer %s", got.Body.String())
	}
	if f.count(t, `SELECT COUNT(*) FROM database_users WHERE id = 9941`)+f.count(t, `SELECT COUNT(*) FROM databases_v2 WHERE id = 9961`) != 0 {
		t.Fatal("records stayed")
	}
	if details := auditDetails(t, f, "database.delete:customer_matrix_test_shop"); details["outcome"] != "deleted" {
		t.Fatalf("audit %v", details)
	}
}

// N1: an account the Agent refuses to touch (pg_*, debian-sys-maint, a name
// CelikPanel does not manage) is kept, never a permanent "users left".
func TestDatabaseDeleteKeepsAccountsTheAgentWouldRefuse(t *testing.T) {
	f := newVisibleDatabasesFixture(t)
	recordDatabaseDrops(t)
	native := &fakeDatabaseAccessNative{}
	f.panel.databaseAccessNative = native
	f.exec(t, `INSERT INTO database_users (id, server_id, subscription_id, username, password) VALUES (9952, 9921, 9802, 'pg_monitor', 'x'), (9953, 9921, 9802, 'debian-sys-maint', 'x')`)
	f.exec(t, `INSERT INTO database_user_grants (database_id, user_id, privileges) VALUES (9961, 9952, 'ALL'), (9961, 9953, 'ALL')`)
	got := f.as(t, "admin", http.MethodDelete, "/api/v1/databases/9961", "")
	if got.Code != http.StatusOK || removedUsernames(native) != "cust_user,wp_user" {
		t.Fatalf("%d %s removed %q", got.Code, got.Body.String(), removedUsernames(native))
	}
}

// W4 + N4: the domain route records its deletion; a deletion whose records
// could not be removed after the drop says so (coded) and is recorded too.
func TestDatabaseDeleteIsAuditedOnBothRoutesAndARecordsFailureIsSaid(t *testing.T) {
	f := newVisibleDatabasesFixture(t)
	recordDatabaseDrops(t)
	f.panel.databaseAccessNative = &fakeDatabaseAccessNative{}
	if got := f.call(t, authzMatrixCustomerID, http.MethodDelete, "/api/v1/domains/9902/databases/9962", ""); got.Code != http.StatusOK {
		t.Fatalf("%d %s", got.Code, got.Body.String())
	}
	// 9962's only user wp_user also reads 9961: kept.
	details := auditDetails(t, f, "database.delete:customer_matrix_test_blog")
	if kept, _ := details["users_kept"].([]any); details["outcome"] != "deleted" || details["owner"] == nil || len(kept) != 1 {
		t.Fatalf("domain route audit %v", details)
	}

	f.exec(t, `CREATE TRIGGER refuse_record_removal BEFORE DELETE ON databases_v2 WHEN OLD.id = 9961 BEGIN SELECT RAISE(ABORT, 'records are read-only'); END`)
	got := f.as(t, "admin", http.MethodDelete, "/api/v1/databases/9961", "")
	var body map[string]any
	_ = json.Unmarshal(got.Body.Bytes(), &body)
	if got.Code != http.StatusInternalServerError || body["code"] != errCodeDatabaseDeleteRecordsLeft || body["mutation_applied"] != true {
		t.Fatalf("%d %s", got.Code, got.Body.String())
	}
	if details := auditDetails(t, f, "database.delete.partial:customer_matrix_test_shop"); details["outcome"] != "records_left" {
		t.Fatalf("partial audit %v", details)
	}
	f.exec(t, `DROP TRIGGER refuse_record_removal`)
	if got := f.as(t, "admin", http.MethodDelete, "/api/v1/databases/9961", ""); got.Code != http.StatusOK {
		t.Fatalf("again: %d %s", got.Code, got.Body.String())
	}
}

// W4: a partial result (users left) is recorded with its outcome.
func TestDatabaseDeleteRecordsAPartialResult(t *testing.T) {
	f := newVisibleDatabasesFixture(t)
	recordDatabaseDrops(t)
	f.panel.databaseAccessNative = &fakeDatabaseAccessNative{failRemove: true}
	if got := f.as(t, "admin", http.MethodDelete, "/api/v1/databases/9961", ""); got.Code != http.StatusConflict {
		t.Fatalf("%d %s", got.Code, got.Body.String())
	}
	if details := auditDetails(t, f, "database.delete.partial:customer_matrix_test_shop"); details["outcome"] != "users_left" {
		t.Fatalf("audit %v", details)
	}
}

// W7: a grant of the same account to another database waits for a deletion
// of the same engine, and then finds the account gone; it never re-grants an
// account the deletion is dropping.
func TestAGrantDuringADeletionOfTheSameEngineWaits(t *testing.T) {
	f := newVisibleDatabasesFixture(t)
	recordDatabaseDrops(t)
	f.panel.databaseAccessNative = &fakeDatabaseAccessNative{}
	granted := make(chan int, 1)
	waited := false
	databaseDeleteAfterClassify = func() {
		go func() {
			granted <- f.as(t, "admin", http.MethodPost, "/api/v1/databases/9962/grants", `{"user_id":9941}`).Code
		}()
		select {
		case <-granted:
		case <-time.After(400 * time.Millisecond):
			waited = true
		}
	}
	t.Cleanup(func() { databaseDeleteAfterClassify = nil })
	if got := f.as(t, "admin", http.MethodDelete, "/api/v1/databases/9961", ""); got.Code != http.StatusOK {
		t.Fatalf("%d %s", got.Code, got.Body.String())
	}
	if !waited {
		t.Fatal("the grant ran between the deletion's choice of users and their removal")
	}
	if code := <-granted; code == http.StatusOK {
		t.Fatal("an account the deletion dropped was granted afterwards")
	}
	if f.count(t, `SELECT COUNT(*) FROM database_user_grants WHERE user_id = 9941`) != 0 {
		t.Fatal("a grant of the dropped account was recorded")
	}
}

// W-C (review): a grant that meets a deletion on the same engine waits a
// short, bounded time and is then refused, typed, instead of waiting silently.
func TestAGrantThatMeetsALongDeletionIsToldTheServerIsBusy(t *testing.T) {
	f := newVisibleDatabasesFixture(t)
	recordDatabaseDrops(t)
	f.panel.databaseAccessNative = &fakeDatabaseAccessNative{}
	savedWait := databaseEngineLinkWait
	databaseEngineLinkWait = 50 * time.Millisecond
	t.Cleanup(func() { databaseEngineLinkWait = savedWait })
	answer := make(chan string, 1)
	databaseDeleteAfterClassify = func() {
		answer <- f.as(t, "admin", http.MethodPost, "/api/v1/databases/9962/grants", `{"user_id":9941}`).Body.String()
	}
	t.Cleanup(func() { databaseDeleteAfterClassify = nil })
	if got := f.as(t, "admin", http.MethodDelete, "/api/v1/databases/9961", ""); got.Code != http.StatusOK {
		t.Fatalf("%d %s", got.Code, got.Body.String())
	}
	if body := <-answer; !strings.Contains(body, errCodeDatabaseEngineBusy) {
		t.Fatalf("grant answer %s", body)
	}
}

// N-F (review): the lists load when what deleting would leave cannot be
// read; each user then says "unknown" and the screens offer no deletion.
func TestDatabaseListsLoadWhenFatesCannotBeRead(t *testing.T) {
	f := newVisibleDatabasesFixture(t)
	saved := databaseUserFatesRead
	databaseUserFatesRead = func(context.Context, databaseQueryer, []int) (map[int][]databaseDeleteUser, error) {
		return nil, errors.New("read failed")
	}
	t.Cleanup(func() { databaseUserFatesRead = saved })
	got := f.as(t, "admin", http.MethodGet, databasesVisiblePath, "")
	if got.Code != http.StatusOK || !strings.Contains(got.Body.String(), `"stays":"unknown"`) {
		t.Fatalf("%d %s", got.Code, got.Body.String())
	}
	domain := f.call(t, authzMatrixCustomerID, http.MethodGet, "/api/v1/domains/9902/databases", "")
	if domain.Code != http.StatusOK || !strings.Contains(domain.Body.String(), `"stays":"unknown"`) {
		t.Fatalf("%d %s", domain.Code, domain.Body.String())
	}
}
