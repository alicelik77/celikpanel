package main

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/services"
)

// Second review of PR #370 (alpha.149), the second fix round.

// readBackLostLimiter accepts writes and then loses the read-back.
type readBackLostLimiter struct {
	fakeRoleLimiter
	reads int
}

func (l *readBackLostLimiter) ReadUserRoleSettings(user string) (services.RoleSettings, error) {
	l.reads++
	if l.reads > 1 {
		return nil, errors.New("connection reset by peer")
	}
	return l.fakeRoleLimiter.ReadUserRoleSettings(user)
}

// connOf reads the in-memory role's connection limit under the fake's lock.
func connOf(agent *pgAgentRequests, role string) int {
	agent.mu.Lock()
	defer agent.mu.Unlock()
	return agent.conn[role]
}

// Finding 4 (decision 7 of the second round): a connection limit alpha.128
// to 148 wrote without a record - through celikpanel_admin on PostgreSQL 15
// or older, or on 16 and later on a role celikpanel_admin made itself (the
// panel's logic is the same for both: the Agent reads rolconnlimit, the
// record has no entry) - follows the next change of the limit. The reviewer's
// probe: Ubuntu 24.04, a user made on the Database Servers screen under a
// plan limit of 10; after the update the plan goes to 20; before the fix it
// stayed 10, labelled the owner's.
func TestALimitEarlierReleasesWroteFollowsTheNextChange(t *testing.T) {
	f, _, agent := dbLimitsFixture(t)
	ctx := context.Background()
	f.exec(t, `INSERT INTO database_users (server_id, subscription_id, username, password) VALUES (9961, 9802, 'pg15_u', 'x'), (9961, 9802, 'pg16_admin_made', 'x'), (9961, 9802, 'own_u', 'x')`)
	setPlanDBLimit(t, f, settingDBConnections, 10)
	agent.conn["pg15_u"], agent.conn["pg16_admin_made"] = 10, 10 // written by alpha.148, no record
	agent.conn["own_u"] = 25                                     // the owner's, by hand
	// Through the plan's Save (the previous value is read before it).
	wantStatus(t, f.call(t, authzMatrixAdminID, http.MethodPut, "/api/v1/plans/9951",
		`{"name":"DB","max_domains":0,"max_databases":0,"max_email_accounts":0,"disk_quota_mb":0,"bandwidth_quota_mb":0,"ceilings":{"db.max_user_connections":20}}`), http.StatusOK, "")
	if connOf(agent, "pg15_u") != 20 || connOf(agent, "pg16_admin_made") != 20 || connOf(agent, "own_u") != 25 {
		t.Fatalf("after the plan went 10 -> 20: %v", agent.conn)
	}
	for _, s := range f.panel.dbOwnerSettingsOf(ctx, 9802) {
		if s.User != "own_u" {
			t.Fatalf("a limit the panel wrote is labelled the owner's: %+v", s)
		}
	}
	// To no limit and back: the panel's value goes, then comes back; the
	// owner's stays.
	prev := f.panel.dbConnectionLimitsNow(ctx, []int{9802})
	setPlanDBLimit(t, f, settingDBConnections, 0)
	if _, err := f.panel.reapplyDBConnectionLimitsNowFrom(ctx, []int{9802}, nil, prev); err != nil {
		t.Fatal(err)
	}
	if connOf(agent, "pg15_u") != -1 || connOf(agent, "own_u") != 25 {
		t.Fatalf("after 20 -> no limit: %v", agent.conn)
	}
	prev = f.panel.dbConnectionLimitsNow(ctx, []int{9802})
	setPlanDBLimit(t, f, settingDBConnections, 12)
	if _, err := f.panel.reapplyDBConnectionLimitsNowFrom(ctx, []int{9802}, nil, prev); err != nil {
		t.Fatal(err)
	}
	if connOf(agent, "pg15_u") != 12 || connOf(agent, "own_u") != 25 {
		t.Fatalf("after no limit -> 12: %v", agent.conn)
	}
}

// Without the previous value (Apply again, the hourly round) a value equal to
// neither the record nor the plan stays the owner's (decision 3 of the first
// round): the limit stated in the PR.
func TestWithoutThePreviousValueAnUnrecordedLimitStaysTheOwners(t *testing.T) {
	f, _, agent := dbLimitsFixture(t)
	ctx := context.Background()
	f.exec(t, `INSERT INTO database_users (server_id, subscription_id, username, password) VALUES (9961, 9802, 'old_u', 'x')`)
	agent.conn["old_u"] = 10
	setPlanDBLimit(t, f, settingDBConnections, 20)
	if _, err := f.panel.reapplyDBConnectionLimitsNow(ctx, []int{9802}, nil); err != nil {
		t.Fatal(err)
	}
	if connOf(agent, "old_u") != 10 {
		t.Fatalf("an unrecorded value was taken without the previous plan value: %v", agent.conn)
	}
}

// A user whose subscription moves to another plan: the previous plan's value
// is read before the move.
func TestALimitEarlierReleasesWroteFollowsAMoveToAnotherPlan(t *testing.T) {
	f, _, agent := dbLimitsFixture(t)
	f.exec(t, `INSERT INTO database_users (server_id, subscription_id, username, password) VALUES (9961, 9802, 'mv_u', 'x')`)
	setPlanDBLimit(t, f, settingDBConnections, 10)
	agent.mu.Lock()
	agent.conn["mv_u"] = 10
	agent.mu.Unlock()
	f.exec(t, `INSERT INTO service_plans (id, name, max_domains, max_databases, max_email_accounts, disk_quota_mb, bandwidth_quota_mb) VALUES (9952, 'DB2', 0, 0, 0, 0, 0)`)
	f.exec(t, `INSERT INTO setting_ceilings (owner_kind, owner_id, key, value) VALUES ('plan', 9952, 'db.max_user_connections', 30)`)
	wantStatus(t, f.call(t, authzMatrixAdminID, http.MethodPut, "/api/v1/subscriptions/9802/rights", `{"state":"plan","plan_id":9952,"confirm":true}`), http.StatusOK, "")
	deadline := time.Now().Add(15 * time.Second)
	for connOf(agent, "mv_u") != 30 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if got := connOf(agent, "mv_u"); got != 30 {
		t.Fatalf("after the move to a plan of 30: %d", got)
	}
}

// Finding 8: a lost read-back carries the "unknown" reason code, so the
// Problems now line says "not known yet", not that the server refused.
func TestALostReadBackIsNotKnownYet(t *testing.T) {
	f := newRightsFixture(t)
	engine := &readBackLostLimiter{fakeRoleLimiter: fakeRoleLimiter{keys: services.RoleKeys, settings: map[string]string{}}}
	err := f.panel.applyDBRoleLimits(context.Background(), engine, 3, "shop_user", services.RoleLimits{StatementSeconds: 60}, false)
	state, code := dbLimitClassOf(err)
	if state != dbLimitUnknown || code != dbReasonUnknown {
		t.Fatalf("a lost read-back: %q %q (%v)", state, code, err)
	}
}
