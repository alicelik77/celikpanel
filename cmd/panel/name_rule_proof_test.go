package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/rpc"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Security release (20 November 2026), the owner's rule: a reseller or a
// customer never reaches anything of another customer or of the server
// itself. These tests perform each sequence the release closes; on the code
// before the release they show the harm.

// nameRuleAgent is the Agent of these tests: it records every site creation
// and deletion it is asked for. refuseCreate answers a creation the way the
// Agent refuses an existing system account.
type nameRuleAgent struct {
	setupDNSHostingAgent
	refuseCreate bool
	deleted      []transport.DeleteSiteRequest
}

func (a *nameRuleAgent) CreateSite(req *transport.CreateSiteRequest, out *transport.CreateSiteResponse) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.created = append(a.created, *req)
	if a.refuseCreate {
		out.ErrorMessage = "site provisioning refused because its system user already exists"
		out.ErrorCode = "site_identity_exists"
		return nil
	}
	out.Success = true
	return nil
}

func (a *nameRuleAgent) DeleteSite(req *transport.DeleteSiteRequest, out *transport.DeleteSiteResponse) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.deleted = append(a.deleted, *req)
	out.Success = true
	return nil
}

func (a *nameRuleAgent) createdNames() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	var names []string
	for _, c := range a.created {
		names = append(names, c.Domain)
	}
	return names
}

// nameRuleFixture: the administrator (1), customer alice (2) and customer
// bob (3), each with a subscription of the same number; new domains keep
// their DNS elsewhere, so only the name rule and the site are involved.
func nameRuleFixture(t *testing.T) (*Panel, *nameRuleAgent) {
	t.Helper()
	p := newDNSPanelForTest(t)
	ctx := context.Background()
	for _, stmt := range []string{
		`INSERT INTO users(id,username,password_hash,email,role) VALUES (1,'admin','hash','admin@example.test','admin')`,
		`INSERT INTO users(id,username,password_hash,email,role) VALUES (2,'alice','hash','alice@example.test','customer')`,
		`INSERT INTO users(id,username,password_hash,email,role) VALUES (3,'bob','hash','bob@example.test','customer')`,
		`INSERT INTO subscriptions(id,owner_id,name) VALUES (1,1,'admin')`,
		`INSERT INTO subscriptions(id,owner_id,name) VALUES (2,2,'alice')`,
		`INSERT INTO subscriptions(id,owner_id,name) VALUES (3,3,'bob')`,
	} {
		if _, err := p.db.GetDB().Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	if err := p.saveSetupDNSManagementMode(ctx, setupDNSModeExternal); err != nil {
		t.Fatal(err)
	}
	agent := &nameRuleAgent{setupDNSHostingAgent: setupDNSHostingAgent{hostingCapabilitiesTestAgent: hostingCapabilitiesTestAgent{installed: []string{"nginx"}}}}
	server := rpc.NewServer()
	if err := server.RegisterName("Agent", agent); err != nil {
		t.Fatal(err)
	}
	connector := func(ctx context.Context) (*rpc.Client, error) {
		a, b := net.Pipe()
		go server.ServeConn(a)
		return rpc.NewClient(b), nil
	}
	client, err := connector(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	p.agentClient = transport.NewReconnectingClientWithContextConnector(client, connector)
	p.orchestrator = services.NewSiteOrchestrator(p.db.GetDB(), p.agentClient)
	return p, agent
}

func nameRuleCreate(t *testing.T, p *Panel, caller *Caller, subscription int, name string) *httptest.ResponseRecorder {
	t.Helper()
	body := fmt.Sprintf(`{"domain":%q,"project_type":"static","subscription_id":%d}`, name, subscription)
	r := httptest.NewRequest(http.MethodPost, "/api/v1/domains", strings.NewReader(body))
	r = r.WithContext(context.WithValue(r.Context(), callerKey, caller))
	w := httptest.NewRecorder()
	p.handleCreateDomain(w, r)
	return w
}

var (
	nameRuleAdmin = &Caller{ID: 1, Role: roleAdmin}
	nameRuleAlice = &Caller{ID: 2, Role: roleCustomer}
	nameRuleBob   = &Caller{ID: 3, Role: roleCustomer}
)

func nameRuleRefused(t *testing.T, w *httptest.ResponseRecorder, what string) {
	t.Helper()
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "HOSTNAME_NOT_AVAILABLE") {
		t.Fatalf("%s was not refused: %d %s", what, w.Code, w.Body.String())
	}
}

// A: customer bob takes a name under, equal to or above customer alice's
// domain (and an alias of it). Before the release bob got shop.victim.test:
// its own zone, site, certificate and mail for alice's names.
func TestNameRuleProofAnotherAccountsNameIsRefused(t *testing.T) {
	p, agent := nameRuleFixture(t)
	if w := nameRuleCreate(t, p, nameRuleAlice, 2, "victim.test"); w.Code != http.StatusOK {
		t.Fatalf("alice's own domain: %d %s", w.Code, w.Body.String())
	}
	for _, name := range []string{"shop.victim.test", "victim.test", "deep.vpn.victim.test"} {
		nameRuleRefused(t, nameRuleCreate(t, p, nameRuleBob, 3, name), "bob's "+name)
	}
	if w := nameRuleCreate(t, p, nameRuleAlice, 2, "blog.victim.test"); w.Code != http.StatusOK {
		t.Fatalf("alice's own subdomain: %d %s", w.Code, w.Body.String())
	}
	if got := strings.Join(agent.createdNames(), ","); got != "victim.test,blog.victim.test" {
		t.Fatalf("sites created on the server: %s", got)
	}
	// Above: alice holds a name under what bob asks for.
	if w := nameRuleCreate(t, p, nameRuleAlice, 2, "shop.parent.test"); w.Code != http.StatusOK {
		t.Fatalf("alice's shop.parent.test: %d %s", w.Code, w.Body.String())
	}
	nameRuleRefused(t, nameRuleCreate(t, p, nameRuleBob, 3, "parent.test"), "bob's parent.test")
	// An alias of bob's own domain under alice's name.
	if w := nameRuleCreate(t, p, nameRuleBob, 3, "bob.test"); w.Code != http.StatusOK {
		t.Fatalf("bob's own domain: %d %s", w.Code, w.Body.String())
	}
	var bobDomain int
	if err := p.db.GetDB().QueryRow(`SELECT id FROM domains WHERE name='bob.test'`).Scan(&bobDomain); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"alias":"www2.victim.test"}`))
	r = r.WithContext(context.WithValue(r.Context(), callerKey, nameRuleBob))
	w := httptest.NewRecorder()
	p.handleAddAlias(w, r, bobDomain)
	if w.Code < 400 || aliasCount(t, p, bobDomain, "www2.victim.test") != 0 {
		t.Fatalf("bob's alias under alice's domain: %d %s", w.Code, w.Body.String())
	}
}

// B: a customer takes the server's own names: a nameserver host name, the
// zone above it, the infrastructure zone and a name in it.
func TestNameRuleProofTheServersNamesAreRefused(t *testing.T) {
	p, _ := nameRuleFixture(t)
	ctx := context.Background()
	for key, value := range map[string]string{settingNS1: "ns1.celikhost.com", settingNS2: "ns2.celikhost.com"} {
		if err := p.setSetting(ctx, key, value); err != nil {
			t.Fatal(err)
		}
	}
	result, err := p.db.GetDB().Exec(`INSERT INTO pdns_domains(name,type) VALUES ('infra.test','NATIVE')`)
	if err != nil {
		t.Fatal(err)
	}
	zoneID, _ := result.LastInsertId()
	owner, _ := json.Marshal(map[string]any{"zone": "infra.test", "zone_id": zoneID, "plan_id": "plan"})
	if _, err := p.db.GetDB().Exec(`INSERT INTO panel_settings(key,value) VALUES ('server_setup_infrastructure_zone:infra.test',?)`, string(owner)); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"ns1.celikhost.com", "celikhost.com", "infra.test", "boston.infra.test"} {
		nameRuleRefused(t, nameRuleCreate(t, p, nameRuleBob, 3, name), "bob's "+name)
	}
}

// B: an existing zone of the server is never returned as a customer's
// domain's zone. Before the release, a customer's domain named like the
// infrastructure zone was handed that zone and its records.
func TestNameRuleProofTheInfrastructureZoneIsNotAdopted(t *testing.T) {
	p, _ := nameRuleFixture(t)
	if _, err := p.db.GetDB().Exec(`INSERT INTO domains(subscription_id,name,dns_management) VALUES (3,'infra.test','local')`); err != nil {
		t.Fatal(err)
	}
	result, err := p.db.GetDB().Exec(`INSERT INTO pdns_domains(name,type) VALUES ('infra.test','NATIVE')`)
	if err != nil {
		t.Fatal(err)
	}
	zoneID, _ := result.LastInsertId()
	owner, _ := json.Marshal(map[string]any{"zone": "infra.test", "zone_id": zoneID, "plan_id": "plan"})
	if _, err := p.db.GetDB().Exec(`INSERT INTO panel_settings(key,value) VALUES ('server_setup_infrastructure_zone:infra.test',?)`, string(owner)); err != nil {
		t.Fatal(err)
	}
	if id, created, err := p.createZoneWithTemplate(context.Background(), "infra.test"); err == nil {
		t.Fatalf("the infrastructure zone (%d, created=%v) was handed to a customer's domain", id, created)
	}
}

// C: bob's ab-cd.test derives the system account of alice's ab.cd.test.
// Before the release bob's creation reached the Agent.
func TestNameRuleProofASharedSystemAccountIsRefusedBeforeCreation(t *testing.T) {
	p, agent := nameRuleFixture(t)
	if w := nameRuleCreate(t, p, nameRuleAlice, 2, "ab.cd.test"); w.Code != http.StatusOK {
		t.Fatalf("alice's ab.cd.test: %d %s", w.Code, w.Body.String())
	}
	nameRuleRefused(t, nameRuleCreate(t, p, nameRuleBob, 3, "ab-cd.test"), "bob's ab-cd.test")
	if got := strings.Join(agent.createdNames(), ","); got != "ab.cd.test" {
		t.Fatalf("site creations asked of the Agent: %s", got)
	}
}

// C: the Agent refused a site because its system account exists (another
// site's). Before the release the failed creation was compensated with a
// deletion keyed by that account's name: its crontab and FTP logins went.
func TestNameRuleProofARefusedCreationIsNotCompensatedByAccountName(t *testing.T) {
	p, agent := nameRuleFixture(t)
	agent.refuseCreate = true
	_, err := p.orchestrator.CreateSite(context.Background(), &services.CreateSiteRequest{
		SubscriptionID: 3, Domain: "ab-cd.test", ProjectType: "static", SSLType: "none", AccessMethod: "sftp",
		DNSManagement: setupDNSModeExternal,
	})
	if err == nil {
		t.Fatal("a refused creation reported success")
	}
	agent.mu.Lock()
	defer agent.mu.Unlock()
	if len(agent.deleted) != 0 {
		t.Fatalf("the refused creation asked for a deletion of account %q", agent.deleted[0].Username)
	}
	var rows int
	if err := p.db.GetDB().QueryRow(`SELECT COUNT(*) FROM domains WHERE name='ab-cd.test'`).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("the refused creation's records stayed: %d %v", rows, err)
	}
}

// D: a Node.js site's port aimed at a service of this server (the database
// web tools listen on 127.0.0.1:8306). Before the release the vhost passing
// the public to that port was written.
func TestNameRuleProofANodePortOfAServerServiceIsRefused(t *testing.T) {
	for _, port := range []int{8306, 11334, 1923, 30010} {
		panel, domainID := newDomainAliasFixture(t)
		resetHostingTestState(t, panel, domainID, "static")
		agent := &hostingTestAgent{}
		attachHostingTestAgent(t, panel, agent)
		request := nodeHostingRequest()
		request.AppPort = port
		if _, err := panel.transitionHosting(context.Background(), domainID, request); err == nil {
			t.Fatalf("port %d was accepted; agent calls %#v", port, agent.snapshotCalls())
		}
		for _, call := range agent.snapshotCalls() {
			if call.ProjectType == "node" {
				t.Fatalf("a Node.js vhost for port %d was written", port)
			}
		}
	}
}
