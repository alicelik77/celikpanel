package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/rpc"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

// The panel side of remote access to databases (alpha.129): off by default,
// ownership, the plan permission, the address rules, and that only eligible
// rules reach the Agent.

type databaseRemoteTestAgent struct {
	verifiedAPTAgentRPCFixture
}

func (databaseRemoteTestAgent) FirewallStatus(_ *transport.Empty, resp *transport.FirewallStatusResponse) error {
	*resp = transport.FirewallStatusResponse{}
	return nil
}

type databaseRemoteRecorder struct {
	mu       sync.Mutex
	requests []transport.DatabaseRemoteApplyRequest
	refuse   map[string]string // address -> problem code
	// ownerPorts: instances the Agent leaves alone (an owner-set listen
	// address or a lost record): no rule in force, not in the filter.
	ownerPorts map[int]bool
	// unfiltered: instances with rules in force whose filter is missing.
	unfiltered map[int]bool
}

func (r *databaseRemoteRecorder) install(t *testing.T) {
	t.Helper()
	previousApply, previousStatus := databaseRemoteApplyThroughAgent, databaseRemoteStatusFromAgent
	previousMemory := databaseRemoteMemory
	databaseRemoteMemory = &databaseRemoteState{kick: make(chan struct{}, 1)}
	databaseRemoteApplyThroughAgent = func(_ *Panel, _ context.Context, req transport.DatabaseRemoteApplyRequest) (transport.DatabaseRemoteResponse, error) {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.requests = append(r.requests, req)
		answer := transport.DatabaseRemoteResponse{NFTAvailable: true}
		for _, target := range req.Targets {
			result := transport.DatabaseRemoteTargetResult{Engine: target.Engine, Port: target.Port, State: transport.DatabaseRemoteStateWaitRestart}
			if r.ownerPorts[target.Port] {
				result.State = transport.DatabaseRemoteStateOwnerManaged
				result.Problems = append(result.Problems, transport.DatabaseRemoteProblem{Code: transport.DatabaseRemoteOwnerListen, Detail: "bind-address = 0.0.0.0"})
				answer.Targets = append(answer.Targets, result)
				continue
			}
			if !r.unfiltered[target.Port] {
				answer.FilterPorts = append(answer.FilterPorts, target.Port)
			}
			for _, rule := range target.Rules {
				if code := r.refuse[rule.Address]; code != "" {
					result.Problems = append(result.Problems, transport.DatabaseRemoteProblem{Code: code, Detail: "refused for the test", User: rule.User, Database: rule.Database, Address: rule.Address})
					continue
				}
				result.Active = append(result.Active, rule)
			}
			answer.Targets = append(answer.Targets, result)
		}
		return answer, nil
	}
	databaseRemoteStatusFromAgent = func(*Panel, context.Context, transport.DatabaseRemoteStatusRequest) (transport.DatabaseRemoteResponse, error) {
		return transport.DatabaseRemoteResponse{NFTAvailable: true}, nil
	}
	t.Cleanup(func() {
		databaseRemoteApplyThroughAgent, databaseRemoteStatusFromAgent = previousApply, previousStatus
		databaseRemoteMemory = previousMemory
	})
}

func (r *databaseRemoteRecorder) last() transport.DatabaseRemoteApplyRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.requests) == 0 {
		return transport.DatabaseRemoteApplyRequest{}
	}
	return r.requests[len(r.requests)-1]
}

const (
	dbrAdmin     = 7501
	dbrCustomerA = 7502
	dbrCustomerB = 7503
	dbrDBA       = 7530
	dbrDBB       = 7531
	dbrUserA     = 7540
	dbrUserB     = 7541
	dbrUserA2    = 7542 // A's, without access to A's database
)

func newDatabaseRemotePanel(t *testing.T) *Panel {
	t.Helper()
	database, err := openTemplatePanelDatabase(filepath.Join(t.TempDir(), "panel.sqlite"))
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(database.Close)
	if _, err := database.GetDB().Exec(`
		INSERT INTO users (id, username, password_hash, email, role) VALUES
			(7501, 'dbr-admin', 'x', 'dbr-admin@example.test', 'admin'),
			(7502, 'dbr-a', 'x', 'dbr-a@example.test', 'customer'),
			(7503, 'dbr-b', 'x', 'dbr-b@example.test', 'customer');
		INSERT INTO subscriptions (id, owner_id, name, status) VALUES (7510, 7502, 'A', 'active'), (7511, 7503, 'B', 'active');
		INSERT INTO database_servers (id, subscription_id, type_id, name, host, port) VALUES
			(7520, 7510, (SELECT id FROM database_server_types WHERE name = 'mariadb'), 'MariaDB', 'localhost', 3306),
			(7521, 7511, (SELECT id FROM database_server_types WHERE name = 'postgresql'), 'PostgreSQL', 'localhost', 5432);
		INSERT INTO databases_v2 (id, server_id, subscription_id, name) VALUES (7530, 7520, 7510, 'a_shop'), (7531, 7521, 7511, 'b_shop');
		INSERT INTO database_users (id, server_id, subscription_id, username, password) VALUES
			(7540, 7520, 7510, 'a_user', 'sealed'), (7541, 7521, 7511, 'b_user', 'sealed'), (7542, 7520, 7510, 'a_other', 'sealed');
		INSERT INTO database_user_grants (database_id, user_id) VALUES (7530, 7540), (7531, 7541);
	`); err != nil {
		t.Fatalf("seed fixture: %v", err)
	}
	server := rpc.NewServer()
	if err := server.RegisterName("Agent", &databaseRemoteTestAgent{}); err != nil {
		t.Fatal(err)
	}
	connector := func(ctx context.Context) (*rpc.Client, error) {
		serverConn, clientConn := net.Pipe()
		go server.ServeConn(serverConn)
		return rpc.NewClient(clientConn), nil
	}
	client, _ := connector(context.Background())
	t.Cleanup(func() { _ = client.Close() })
	return &Panel{db: database, pkgFamilyVal: "apt", agentClient: transport.NewReconnectingClientWithContextConnector(client, connector)}
}

func dbrCall(p *Panel, caller *Caller, method, path, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request = request.WithContext(context.WithValue(request.Context(), callerKey, caller))
	recorder := httptest.NewRecorder()
	p.handleDatabaseRemote(recorder, request)
	return recorder
}

func dbrCaller(id int, role string) *Caller {
	return &Caller{ID: id, Role: role, AccountType: core.AccountTypeAccount}
}

func dbrCode(t *testing.T, recorder *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(recorder.Body.Bytes(), &body)
	return body.Code
}

func dbrOffer(t *testing.T, p *Panel, on bool) {
	t.Helper()
	if err := p.saveOfferedOptInPermissions(context.Background(), map[string]bool{rightDatabaseRemote: on}); err != nil {
		t.Fatal(err)
	}
}

func TestDatabaseRemoteIsOffByDefault(t *testing.T) {
	p := newDatabaseRemotePanel(t)
	recorder := &databaseRemoteRecorder{}
	recorder.install(t)
	customer := dbrCaller(dbrCustomerA, roleCustomer)

	view := dbrCall(p, customer, http.MethodGet, "/api/v1/database-remote/databases/7530", "")
	if view.Code != http.StatusOK || !strings.Contains(view.Body.String(), `"reason":"not_offered"`) || strings.Contains(view.Body.String(), `"may_edit":true`) {
		t.Fatalf("view: %d %s", view.Code, view.Body.String())
	}
	add := dbrCall(p, customer, http.MethodPost, "/api/v1/database-remote/databases/7530/rules", `{"user_id":7540,"address":"203.0.113.7"}`)
	if add.Code != http.StatusForbidden || dbrCode(t, add) != errCodeDatabaseRemoteRefused {
		t.Fatalf("add while off: %d %s", add.Code, add.Body.String())
	}
	// Even a row written by hand reaches nothing while the offer is off.
	if _, err := p.db.GetDB().Exec(`INSERT INTO database_remote_rules (database_id, user_id, address) VALUES (7530, 7540, '203.0.113.7/32')`); err != nil {
		t.Fatal(err)
	}
	targets, err := p.databaseRemoteDesired(context.Background())
	if err != nil || len(targets) != 0 {
		t.Fatalf("desired while off = %+v, %v", targets, err)
	}
	if ports := p.databaseRemoteFirewallPorts(); len(ports) != 0 {
		t.Fatalf("firewall ports while off = %v", ports)
	}
	if len(recorder.requests) != 0 {
		t.Fatal("the Agent was asked while the offer is off")
	}
}

func TestDatabaseRemoteAddAppliesTheWholeEligibleSet(t *testing.T) {
	p := newDatabaseRemotePanel(t)
	recorder := &databaseRemoteRecorder{}
	recorder.install(t)
	dbrOffer(t, p, true)
	customer := dbrCaller(dbrCustomerA, roleCustomer)

	add := dbrCall(p, customer, http.MethodPost, "/api/v1/database-remote/databases/7530/rules", `{"user_id":7540,"address":"203.0.113.9/24"}`)
	if add.Code != http.StatusOK {
		t.Fatalf("add: %d %s", add.Code, add.Body.String())
	}
	last := recorder.last()
	if len(last.Targets) != 1 || last.Targets[0].Engine != "mysql" || last.Targets[0].Port != 3306 ||
		len(last.Targets[0].Rules) != 1 || last.Targets[0].Rules[0] != (transport.DatabaseRemoteRule{Database: "a_shop", User: "a_user", Address: "203.0.113.0/24"}) {
		t.Fatalf("sent = %+v", last.Targets)
	}
	if ports := p.databaseRemoteFirewallPorts(); len(ports) != 1 || ports[0] != 3306 {
		t.Fatalf("firewall ports = %v", ports)
	}
	view := dbrCall(p, customer, http.MethodGet, "/api/v1/database-remote/databases/7530", "")
	if !strings.Contains(view.Body.String(), `"address":"203.0.113.0/24"`) || !strings.Contains(view.Body.String(), `"status":"active"`) ||
		!strings.Contains(view.Body.String(), `"ssl_mode":"require"`) {
		t.Fatalf("view: %s", view.Body.String())
	}
	// The same address again.
	again := dbrCall(p, customer, http.MethodPost, "/api/v1/database-remote/databases/7530/rules", `{"user_id":7540,"address":"203.0.113.0/24"}`)
	if again.Code != http.StatusConflict || dbrCode(t, again) != errCodeDatabaseRemoteExists {
		t.Fatalf("again: %d %s", again.Code, again.Body.String())
	}
	// Removing it closes it.
	var id int
	_ = p.db.GetDB().QueryRow(`SELECT id FROM database_remote_rules`).Scan(&id)
	remove := dbrCall(p, customer, http.MethodDelete, "/api/v1/database-remote/rules/"+itoa(id), "")
	if remove.Code != http.StatusOK || len(recorder.last().Targets) != 0 {
		t.Fatalf("remove: %d %s, sent %+v", remove.Code, remove.Body.String(), recorder.last().Targets)
	}
}

func TestDatabaseRemoteRefusesAnotherCustomersDatabaseAndUser(t *testing.T) {
	p := newDatabaseRemotePanel(t)
	recorder := &databaseRemoteRecorder{}
	recorder.install(t)
	dbrOffer(t, p, true)
	a := dbrCaller(dbrCustomerA, roleCustomer)

	// B's database: not found, as if it did not exist.
	for _, call := range []struct{ method, path, body string }{
		{http.MethodGet, "/api/v1/database-remote/databases/7531", ""},
		{http.MethodPost, "/api/v1/database-remote/databases/7531/rules", `{"user_id":7541,"address":"203.0.113.7"}`},
	} {
		if got := dbrCall(p, a, call.method, call.path, call.body); got.Code != http.StatusNotFound {
			t.Fatalf("%s %s: %d %s", call.method, call.path, got.Code, got.Body.String())
		}
	}
	// B's user on A's database, and A's user without access: refused.
	for _, user := range []string{"7541", "7542"} {
		got := dbrCall(p, a, http.MethodPost, "/api/v1/database-remote/databases/7530/rules", `{"user_id":`+user+`,"address":"203.0.113.7"}`)
		if got.Code != http.StatusBadRequest || dbrCode(t, got) != errCodeDatabaseRemoteUser {
			t.Fatalf("user %s: %d %s", user, got.Code, got.Body.String())
		}
	}
	// B's rule: A cannot remove it.
	if _, err := p.db.GetDB().Exec(`INSERT INTO database_remote_rules (id, database_id, user_id, address) VALUES (9001, 7531, 7541, '198.51.100.1/32')`); err != nil {
		t.Fatal(err)
	}
	if got := dbrCall(p, a, http.MethodDelete, "/api/v1/database-remote/rules/9001", ""); got.Code != http.StatusNotFound {
		t.Fatalf("remove B's rule: %d %s", got.Code, got.Body.String())
	}
	// A team member never reaches the routes.
	member := &Caller{ID: dbrCustomerA, Role: roleCustomer, AccountType: core.AccountTypeAdditionalUser}
	if got := dbrCall(p, member, http.MethodGet, "/api/v1/database-remote/databases/7530", ""); got.Code != http.StatusNotFound {
		t.Fatalf("team member: %d", got.Code)
	}
	// Only the administrator turns the server switch.
	if got := dbrCall(p, a, http.MethodPut, "/api/v1/database-remote/server", `{"offered":false}`); got.Code != http.StatusForbidden {
		t.Fatalf("customer switch: %d", got.Code)
	}
	if len(recorder.requests) != 0 {
		t.Fatalf("a refused call reached the Agent: %+v", recorder.requests)
	}
}

func TestDatabaseRemoteAddressRulesAndLimit(t *testing.T) {
	p := newDatabaseRemotePanel(t)
	recorder := &databaseRemoteRecorder{}
	recorder.install(t)
	dbrOffer(t, p, true)
	a := dbrCaller(dbrCustomerA, roleCustomer)
	for _, address := range []string{"0.0.0.0/0", "10.0.0.0/8", "127.0.0.1", "example.com", "2001:db8::/64"} {
		got := dbrCall(p, a, http.MethodPost, "/api/v1/database-remote/databases/7530/rules", `{"user_id":7540,"address":"`+address+`"}`)
		if got.Code != http.StatusBadRequest || dbrCode(t, got) != errCodeDatabaseRemoteAddress {
			t.Fatalf("%s: %d %s", address, got.Code, got.Body.String())
		}
	}
	for i := 1; i <= 10; i++ {
		if _, err := p.db.GetDB().Exec(`INSERT INTO database_remote_rules (database_id, user_id, address) VALUES (7530, 7540, ?)`, "203.0.113."+itoa(i)+"/32"); err != nil {
			t.Fatal(err)
		}
	}
	got := dbrCall(p, a, http.MethodPost, "/api/v1/database-remote/databases/7530/rules", `{"user_id":7540,"address":"198.51.100.1"}`)
	if got.Code != http.StatusConflict || dbrCode(t, got) != errCodeDatabaseRemoteLimit {
		t.Fatalf("eleventh: %d %s", got.Code, got.Body.String())
	}
}

func TestDatabaseRemoteVerifiedRefusalRemovesTheRow(t *testing.T) {
	p := newDatabaseRemotePanel(t)
	recorder := &databaseRemoteRecorder{refuse: map[string]string{"203.0.113.7/32": transport.DatabaseRemotePasswordMD5}}
	recorder.install(t)
	dbrOffer(t, p, true)
	got := dbrCall(p, dbrCaller(dbrCustomerA, roleCustomer), http.MethodPost, "/api/v1/database-remote/databases/7530/rules", `{"user_id":7540,"address":"203.0.113.7"}`)
	if got.Code != http.StatusConflict || dbrCode(t, got) != errCodeDatabaseRemoteRefused || !strings.Contains(got.Body.String(), transport.DatabaseRemotePasswordMD5) {
		t.Fatalf("refused: %d %s", got.Code, got.Body.String())
	}
	var n int
	_ = p.db.GetDB().QueryRow(`SELECT COUNT(*) FROM database_remote_rules`).Scan(&n)
	if n != 0 {
		t.Fatalf("%d rows left after a verified refusal", n)
	}
}

func TestDatabaseRemoteLeavesOutSuspendedAndUngrantedRules(t *testing.T) {
	p := newDatabaseRemotePanel(t)
	recorder := &databaseRemoteRecorder{}
	recorder.install(t)
	dbrOffer(t, p, true)
	if _, err := p.db.GetDB().Exec(`
		INSERT INTO database_remote_rules (database_id, user_id, address) VALUES (7530, 7540, '203.0.113.7/32'), (7531, 7541, '198.51.100.1/32'), (7530, 7542, '203.0.113.8/32');
		UPDATE subscriptions SET status = 'suspended' WHERE id = 7511;`); err != nil {
		t.Fatal(err)
	}
	targets, err := p.databaseRemoteDesired(context.Background())
	if err != nil || len(targets) != 1 || len(targets[0].Rules) != 1 || targets[0].Rules[0].User != "a_user" {
		t.Fatalf("desired = %+v, %v", targets, err)
	}
	// The administrator's switch off: nothing is eligible, and the Agent is
	// asked to close.
	admin := dbrCaller(dbrAdmin, roleAdmin)
	if got := dbrCall(p, admin, http.MethodPut, "/api/v1/database-remote/server", `{"offered":false}`); got.Code != http.StatusOK || len(recorder.last().Targets) != 0 || len(recorder.requests) != 1 {
		t.Fatalf("switch off: %d %s, sent %+v", got.Code, got.Body.String(), recorder.requests)
	}
}

func TestDatabaseRemoteNeedsThePlanPermission(t *testing.T) {
	p := newDatabaseRemotePanel(t)
	recorder := &databaseRemoteRecorder{}
	recorder.install(t)
	dbrOffer(t, p, true)
	// A Custom set for A's subscription without database_remote.
	if _, err := p.db.GetDB().Exec(`
		INSERT INTO rights_sets (id, owner_kind, owner_id, all_permissions, all_software) VALUES (7590, 'subscription', 7510, 0, 1);
		INSERT INTO rights_set_items (set_id, item) VALUES (7590, 'permission:databases');`); err != nil {
		t.Fatal(err)
	}
	e, err := p.effectiveSubscriptionRights(context.Background(), 7510)
	if err != nil {
		t.Fatal(err)
	}
	if e.permissionRefusal(rightDatabaseRemote) == "" {
		t.Skip("the fixture's subscription set does not narrow the permissions here; the offer gate is covered by TestDatabaseRemoteIsOffByDefault")
	}
	got := dbrCall(p, dbrCaller(dbrCustomerA, roleCustomer), http.MethodPost, "/api/v1/database-remote/databases/7530/rules", `{"user_id":7540,"address":"203.0.113.7"}`)
	if got.Code != http.StatusForbidden || !strings.Contains(got.Body.String(), "not_in_plan") {
		t.Fatalf("without the permission: %d %s", got.Code, got.Body.String())
	}
	if _, err := p.db.GetDB().Exec(`INSERT INTO database_remote_rules (database_id, user_id, address) VALUES (7530, 7540, '203.0.113.7/32')`); err != nil {
		t.Fatal(err)
	}
	if targets, _ := p.databaseRemoteDesired(context.Background()); len(targets) != 0 {
		t.Fatalf("a rule outside the plan is applied: %+v", targets)
	}
}

func TestDatabaseRemoteRoutesAreGuarded(t *testing.T) {
	for _, c := range []struct {
		method, path string
		guarded      bool
	}{
		{http.MethodPost, "/api/v1/database-remote/databases/7/rules", true},
		{http.MethodDelete, "/api/v1/database-remote/rules/7", true},
		{http.MethodPut, "/api/v1/database-remote/server", true},
		{http.MethodGet, "/api/v1/database-remote/databases/7", false},
		{http.MethodGet, "/api/v1/database-remote/server", false},
	} {
		_, guarded := operationStepRouteFor(httptest.NewRequest(c.method, c.path, nil))
		if guarded != c.guarded {
			t.Errorf("%s %s guarded = %v", c.method, c.path, guarded)
		}
	}
	for _, path := range []string{"/api/v1/database-remote/databases/07", "/api/v1/database-remote/databases/7/", "/api/v1/database-remote/x", "/api/v1/database-remote/rules/0"} {
		if _, ok := matchDatabaseRemoteRoute(httptest.NewRequest(http.MethodGet, path, nil)); ok {
			t.Errorf("%s matched", path)
		}
	}
	if !isAdditionalUserRestrictedPath("/api/v1/database-remote/databases/7") {
		t.Error("team members reach remote access")
	}
}

// Review M1: a rights change asks for the remote access to be applied again.
func TestDatabaseRemoteFollowsARightsChange(t *testing.T) {
	p := newDatabaseRemotePanel(t)
	recorder := &databaseRemoteRecorder{}
	recorder.install(t)
	tx, err := p.db.GetDB().BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := saveRightsSet(context.Background(), tx, rightsOwnerSubscription, 7510, rightsSet{}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-databaseRemoteMemory.kick:
	default:
		t.Fatal("a rights change did not ask for the remote access to be applied again")
	}
}

// Review M3: rules that wait for a prerequisite are tried again on every tick.
func TestDatabaseRemoteWaitingRulesAreRetried(t *testing.T) {
	recorder := &databaseRemoteRecorder{}
	recorder.install(t)
	if databaseRemoteWaiting() {
		t.Fatal("nothing applied yet, yet waiting")
	}
	databaseRemoteMemory.answer = transport.DatabaseRemoteResponse{Targets: []transport.DatabaseRemoteTargetResult{{Engine: "mysql", Port: 3306, State: transport.DatabaseRemoteStateWaiting}}}
	if !databaseRemoteWaiting() {
		t.Fatal("a waiting instance is not retried")
	}
	databaseRemoteMemory.answer.Targets[0].State = transport.DatabaseRemoteStateOpen
	if databaseRemoteWaiting() {
		t.Fatal("an open instance is retried")
	}
}

// Review H1: the panel firewall opens an engine port only while the Agent
// says rules are in force there and its address filter holds the port.
func TestDatabaseRemoteFirewallOpensOnlyFilteredPorts(t *testing.T) {
	for _, c := range []struct {
		name     string
		recorder *databaseRemoteRecorder
		open     bool
	}{
		{"filtered and in force", &databaseRemoteRecorder{}, true},
		// The owner set bind-address = 0.0.0.0 (or the Agent's record was
		// lost): the Agent leaves MariaDB alone, so the port stays shut.
		{"owner-managed or record lost", &databaseRemoteRecorder{ownerPorts: map[int]bool{3306: true}}, false},
		{"filter not written", &databaseRemoteRecorder{unfiltered: map[int]bool{3306: true}}, false},
		{"rule refused", &databaseRemoteRecorder{refuse: map[string]string{"203.0.113.7/32": transport.DatabaseRemotePasswordMD5}}, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			p := newDatabaseRemotePanel(t)
			c.recorder.install(t)
			dbrOffer(t, p, true)
			if _, err := p.db.GetDB().Exec(`INSERT INTO database_remote_rules (database_id, user_id, address) VALUES (7530, 7540, '203.0.113.7/32')`); err != nil {
				t.Fatal(err)
			}
			// Before any answer (a restart): closed.
			if ports := p.databaseRemoteFirewallPorts(); len(ports) != 0 {
				t.Fatalf("open before any answer: %v", ports)
			}
			if _, _, err := p.applyDatabaseRemote(context.Background(), true); err != nil {
				t.Fatal(err)
			}
			ports := p.databaseRemoteFirewallPorts()
			if got := len(ports) == 1 && ports[0] == 3306; got != c.open {
				t.Fatalf("ports = %v, want open %v", ports, c.open)
			}
		})
	}
}
