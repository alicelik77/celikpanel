package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.130 remote database access on the panel side: timed access, closing
// at the filter when the full apply cannot run (D5), the switch written from
// either screen (rights lane coordination) and the administrator's view.

func dbrClock(t *testing.T, at time.Time) {
	t.Helper()
	previous := databaseRemoteNow
	databaseRemoteNow = func() time.Time { return at }
	t.Cleanup(func() { databaseRemoteNow = previous })
}

func dbrRuleEnd(t *testing.T, p *Panel, id int) string {
	t.Helper()
	var end string
	if err := p.db.GetDB().QueryRow(`SELECT COALESCE(expires_at, '') FROM database_remote_rules WHERE id = ?`, id).Scan(&end); err != nil {
		t.Fatal(err)
	}
	return end
}

func dbrAddRule(t *testing.T, p *Panel, caller *Caller, body string) int {
	t.Helper()
	added := dbrCall(p, caller, http.MethodPost, "/api/v1/database-remote/databases/7530/rules", body)
	if added.Code != http.StatusOK {
		t.Fatalf("add: %d %s", added.Code, added.Body.String())
	}
	var answer struct {
		ID int `json:"id"`
	}
	_ = json.Unmarshal(added.Body.Bytes(), &answer)
	return answer.ID
}

func TestDatabaseRemoteDurationsAndTheDefault(t *testing.T) {
	now := time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC)
	for duration, want := range map[string]string{
		"":          "2026-11-09 10:00:00",
		"1d":        "2026-11-09 10:00:00",
		"7d":        "2026-11-15 10:00:00",
		"permanent": "",
	} {
		got, ok := databaseRemoteExpiresAt(duration, now)
		if !ok || got != want {
			t.Errorf("%q -> %q %v, want %q", duration, got, ok, want)
		}
	}
	if _, ok := databaseRemoteExpiresAt("30d", now); ok {
		t.Fatal("a duration CelikPanel does not offer was accepted")
	}
	if !databaseRemoteExpired("2026-11-08 10:00:00", now) || databaseRemoteExpired("2026-11-08 10:00:01", now) || databaseRemoteExpired("", now) {
		t.Fatal("end comparison")
	}
	if !databaseRemoteExpired("garbage", now) {
		t.Fatal("an unreadable end kept a rule open")
	}
}

// An address is allowed for one day by default; past its end it is never
// applied and the sweep removes it like a manual removal, with an audit
// entry. A rule given a new end meanwhile keeps it.
func TestDatabaseRemoteTimedRuleEndsAndIsRemoved(t *testing.T) {
	p := newDatabaseRemotePanel(t)
	recorder := &databaseRemoteRecorder{}
	recorder.install(t)
	dbrOffer(t, p, true)
	start := time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC)
	dbrClock(t, start)
	admin := dbrCaller(dbrAdmin, roleAdmin)
	id := dbrAddRule(t, p, admin, `{"user_id": 7540, "address": "203.0.113.7"}`)
	if end := dbrRuleEnd(t, p, id); end != "2026-11-09 10:00:00" {
		t.Fatalf("default end = %q", end)
	}
	if rules := recorder.last().Targets; len(rules) != 1 || len(rules[0].Rules) != 1 {
		t.Fatalf("not applied before its end: %+v", rules)
	}
	view := dbrCall(p, admin, http.MethodGet, "/api/v1/database-remote/databases/7530", "")
	if !strings.Contains(view.Body.String(), `"expires_at":"2026-11-09T10:00:00Z"`) {
		t.Fatalf("view: %s", view.Body.String())
	}

	// Past the end: left out of the set at once, then removed.
	dbrClock(t, start.Add(24*time.Hour+time.Second))
	targets, err := p.databaseRemoteDesired(context.Background())
	if err != nil || len(targets) != 0 {
		t.Fatalf("an ended rule is still wanted: %+v %v", targets, err)
	}
	if removed := p.sweepExpiredDatabaseRemoteRules(context.Background()); removed != 1 {
		t.Fatalf("removed %d", removed)
	}
	var left, audited int
	_ = p.db.GetDB().QueryRow(`SELECT COUNT(*) FROM database_remote_rules`).Scan(&left)
	_ = p.db.GetDB().QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action LIKE 'database.remote.expire:%'`).Scan(&audited)
	if left != 0 || audited != 1 {
		t.Fatalf("rules left: %d, audit entries: %d", left, audited)
	}

	// A permanent rule is never swept.
	id = dbrAddRule(t, p, admin, `{"user_id": 7540, "address": "203.0.113.8", "duration": "permanent"}`)
	dbrClock(t, start.Add(400*24*time.Hour))
	if removed := p.sweepExpiredDatabaseRemoteRules(context.Background()); removed != 0 || dbrRuleEnd(t, p, id) != "" {
		t.Fatalf("a rule with no end was removed: %d", removed)
	}
}

func TestDatabaseRemoteRefusesAnUnofferedDuration(t *testing.T) {
	p := newDatabaseRemotePanel(t)
	recorder := &databaseRemoteRecorder{}
	recorder.install(t)
	dbrOffer(t, p, true)
	added := dbrCall(p, dbrCaller(dbrAdmin, roleAdmin), http.MethodPost, "/api/v1/database-remote/databases/7530/rules", `{"user_id": 7540, "address": "203.0.113.7", "duration": "30d"}`)
	if added.Code != http.StatusBadRequest || dbrCode(t, added) != errCodeDatabaseRemoteDuration {
		t.Fatalf("add: %d %s", added.Code, added.Body.String())
	}
	var rows int
	_ = p.db.GetDB().QueryRow(`SELECT COUNT(*) FROM database_remote_rules`).Scan(&rows)
	if rows != 0 {
		t.Fatal("a refused rule was stored")
	}
}

// A new end without adding the address again; another customer cannot.
func TestDatabaseRemoteRuleGetsANewEnd(t *testing.T) {
	p := newDatabaseRemotePanel(t)
	recorder := &databaseRemoteRecorder{}
	recorder.install(t)
	dbrOffer(t, p, true)
	start := time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC)
	dbrClock(t, start)
	admin := dbrCaller(dbrAdmin, roleAdmin)
	id := dbrAddRule(t, p, admin, `{"user_id": 7540, "address": "203.0.113.7"}`)
	path := "/api/v1/database-remote/rules/" + strconv.Itoa(id)

	dbrClock(t, start.Add(20*time.Hour))
	extended := dbrCall(p, admin, http.MethodPut, path, `{"duration": "7d"}`)
	if extended.Code != http.StatusOK || dbrRuleEnd(t, p, id) != "2026-11-16 06:00:00" {
		t.Fatalf("extend: %d %s, end %q", extended.Code, extended.Body.String(), dbrRuleEnd(t, p, id))
	}
	if response := dbrCall(p, admin, http.MethodPut, path, `{"duration": "permanent"}`); response.Code != http.StatusOK || dbrRuleEnd(t, p, id) != "" {
		t.Fatalf("permanent: %d, end %q", response.Code, dbrRuleEnd(t, p, id))
	}
	if response := dbrCall(p, admin, http.MethodPut, path, `{"duration": ""}`); response.Code != http.StatusBadRequest {
		t.Fatalf("an empty duration was taken: %d", response.Code)
	}
	if response := dbrCall(p, dbrCaller(dbrCustomerB, roleCustomer), http.MethodPut, path, `{"duration": "1d"}`); response.Code != http.StatusNotFound || dbrRuleEnd(t, p, id) != "" {
		t.Fatalf("another customer changed the end: %d", response.Code)
	}
}

// D5: the full apply is refused (the server is still starting); what is no
// longer wanted is still closed at the address filter, the panel firewall
// opens nothing for it, and the screen says the rest is pending.
func TestDatabaseRemoteClosesAtTheFilterWhenTheApplyIsRefused(t *testing.T) {
	p := newDatabaseRemotePanel(t)
	recorder := &databaseRemoteRecorder{}
	recorder.install(t)
	dbrOffer(t, p, true)
	admin := dbrCaller(dbrAdmin, roleAdmin)
	id := dbrAddRule(t, p, admin, `{"user_id": 7540, "address": "203.0.113.7", "duration": "permanent"}`)
	if ports := databaseRemoteFilteredOpenPorts(); len(ports) != 1 {
		t.Fatalf("test setup: open ports %v", ports)
	}
	recorder.mu.Lock()
	recorder.failApply = true
	recorder.narrowed = []transport.DatabaseRemoteTargetResult{{Engine: "mysql", Port: 3306, State: transport.DatabaseRemoteStateClosing}}
	recorder.mu.Unlock()
	removed := dbrCall(p, admin, http.MethodDelete, "/api/v1/database-remote/rules/"+strconv.Itoa(id), "")
	if removed.Code != http.StatusBadGateway {
		t.Fatalf("remove while starting: %d %s", removed.Code, removed.Body.String())
	}
	recorder.mu.Lock()
	narrows := append([]transport.DatabaseRemoteNarrowRequest(nil), recorder.narrows...)
	recorder.mu.Unlock()
	if len(narrows) != 1 || len(narrows[0].Targets) != 0 {
		t.Fatalf("closing at the filter: %+v", narrows)
	}
	if ports := databaseRemoteFilteredOpenPorts(); len(ports) != 0 {
		t.Fatalf("the panel firewall still opens %v for a removed address", ports)
	}
	if pending, _ := databaseRemoteClosePending(); !pending {
		t.Fatal("the screen does not say the rest is pending")
	}
	if !databaseRemoteFailing() {
		t.Fatal("a refused apply is not tried again every minute")
	}
	// The server finished starting: the next apply clears the note.
	recorder.mu.Lock()
	recorder.failApply = false
	recorder.mu.Unlock()
	if _, _, err := p.applyDatabaseRemote(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if pending, _ := databaseRemoteClosePending(); pending || databaseRemoteFailing() {
		t.Fatal("the note stayed after the server took the change")
	}
}

// Rights lane coordination: the Offered tab and the Databases page write the
// same stored offer; a change from either asks for an apply, a save that
// changes nothing does not.
func TestDatabaseRemoteOfferChangeFromAnyScreenAsksForAnApply(t *testing.T) {
	p := newDatabaseRemotePanel(t)
	recorder := &databaseRemoteRecorder{}
	recorder.install(t)
	drain := func() bool {
		select {
		case <-databaseRemoteMemory.kick:
			return true
		default:
			return false
		}
	}
	drain()
	if err := p.saveOfferedOptInPermissions(context.Background(), map[string]bool{rightDatabaseRemote: true}); err != nil {
		t.Fatal(err)
	}
	if !drain() {
		t.Fatal("turning the offer on asked for no apply")
	}
	if err := p.saveOfferedOptInPermissions(context.Background(), map[string]bool{rightDatabaseRemote: true, "nginx_directives": true}); err != nil {
		t.Fatal(err)
	}
	if drain() {
		t.Fatal("another permission's offer asked for an apply")
	}
	if err := p.saveOfferedOptInPermissions(context.Background(), map[string]bool{}); err != nil {
		t.Fatal(err)
	}
	if !drain() {
		t.Fatal("withdrawing the offer asked for no apply")
	}
	offered, err := p.databaseRemoteOffered(context.Background())
	if err != nil || offered {
		t.Fatalf("the switch reads %v, %v", offered, err)
	}
}

// The administrator's view lists the server's engines with their readiness
// and carries the agreement on MariaDB's host name lookup to the Agent; a
// click that changes nothing says so.
func TestDatabaseRemoteServerViewAndTheNameLookupAgreement(t *testing.T) {
	p := newDatabaseRemotePanel(t)
	recorder := &databaseRemoteRecorder{}
	recorder.install(t)
	databaseRemoteStatusFromAgent = func(_ *Panel, _ context.Context, req transport.DatabaseRemoteStatusRequest) (transport.DatabaseRemoteResponse, error) {
		answer := transport.DatabaseRemoteResponse{NFTAvailable: true}
		for _, target := range req.Targets {
			result := transport.DatabaseRemoteTargetResult{Engine: target.Engine, Port: target.Port, State: transport.DatabaseRemoteStateOff}
			if target.Engine == "mysql" {
				result.Readiness = []transport.DatabaseRemoteProblem{{Code: transport.DatabaseRemoteNameResolve, Detail: "names"}}
				result.NameResolve = "on"
			}
			answer.Targets = append(answer.Targets, result)
		}
		return answer, nil
	}
	admin := dbrCaller(dbrAdmin, roleAdmin)
	view := dbrCall(p, admin, http.MethodGet, "/api/v1/database-remote/server", "")
	body := view.Body.String()
	if view.Code != http.StatusOK || !strings.Contains(body, `"engine":"mysql"`) || !strings.Contains(body, `"engine":"postgresql"`) ||
		!strings.Contains(body, `"Code":"NAME_RESOLVE"`) || !strings.Contains(body, `"mariadb_skip_name_resolve":false`) {
		t.Fatalf("view: %d %s", view.Code, body)
	}
	agreed := dbrCall(p, admin, http.MethodPut, "/api/v1/database-remote/server", `{"mariadb_skip_name_resolve": true}`)
	if agreed.Code != http.StatusOK || !strings.Contains(agreed.Body.String(), `"changed":true`) || !recorder.last().MariaDBSkipNameResolve {
		t.Fatalf("agreement: %d %s, request %+v", agreed.Code, agreed.Body.String(), recorder.last())
	}
	again := dbrCall(p, admin, http.MethodPut, "/api/v1/database-remote/server", `{"mariadb_skip_name_resolve": true}`)
	if again.Code != http.StatusOK || !strings.Contains(again.Body.String(), `"changed":false`) {
		t.Fatalf("an unchanged click: %d %s", again.Code, again.Body.String())
	}
	if customer := dbrCall(p, dbrCaller(dbrCustomerA, roleCustomer), http.MethodPut, "/api/v1/database-remote/server", `{"mariadb_skip_name_resolve": false}`); customer.Code != http.StatusForbidden {
		t.Fatalf("a customer changed the agreement: %d", customer.Code)
	}
}
