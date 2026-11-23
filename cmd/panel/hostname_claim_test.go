package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// The one name rule (hostname_claim.go): what each account is told, the
// administrator's deliberate placement, transfers and the list of what an
// existing installation already holds.

type claimAnswer struct {
	Error  string            `json:"error"`
	Code   string            `json:"code"`
	Reason string            `json:"reason"`
	Vars   map[string]string `json:"vars"`
}

func decodeClaimAnswer(t *testing.T, body string) claimAnswer {
	t.Helper()
	var answer claimAnswer
	if err := json.Unmarshal([]byte(body), &answer); err != nil {
		t.Fatalf("answer %q: %v", body, err)
	}
	return answer
}

func TestNameRuleCustomerIsToldTheSameWhoeverHoldsTheName(t *testing.T) {
	p, _ := nameRuleFixture(t)
	ctx := context.Background()
	saved := claimSystemAccountExists
	claimSystemAccountExists = func(name string) bool { return name == "taken_test" }
	t.Cleanup(func() { claimSystemAccountExists = saved })
	if err := p.setSetting(ctx, settingNS1, "ns1.celikhost.com"); err != nil {
		t.Fatal(err)
	}
	if w := nameRuleCreate(t, p, nameRuleAlice, 2, "victim.test"); w.Code != 200 {
		t.Fatalf("alice: %d %s", w.Code, w.Body.String())
	}
	answers := map[string]claimAnswer{}
	for _, name := range []string{"shop.victim.test", "x.ns1.celikhost.com", "taken.test"} {
		w := nameRuleCreate(t, p, nameRuleBob, 3, name)
		nameRuleRefused(t, w, name)
		answer := decodeClaimAnswer(t, w.Body.String())
		if answer.Reason != "" || len(answer.Vars) != 1 || answer.Vars["name"] != name {
			t.Fatalf("%s: the customer's answer says more than the name: %+v", name, answer)
		}
		if strings.Contains(w.Body.String(), "alice") || strings.Contains(w.Body.String(), "victim.test") && name != "shop.victim.test" {
			t.Fatalf("%s: the answer names another account: %s", name, w.Body.String())
		}
		answers[name] = answer
	}
	generic := func(a claimAnswer, name string) string { return strings.ReplaceAll(a.Error, name, "NAME") }
	if generic(answers["shop.victim.test"], "shop.victim.test") != generic(answers["x.ns1.celikhost.com"], "x.ns1.celikhost.com") ||
		generic(answers["shop.victim.test"], "shop.victim.test") != generic(answers["taken.test"], "taken.test") {
		t.Fatalf("the answers differ by cause: %+v", answers)
	}
	// Her own name again: she may be told it is hers.
	w := nameRuleCreate(t, p, nameRuleAlice, 2, "victim.test")
	if answer := decodeClaimAnswer(t, w.Body.String()); answer.Reason != hostnameClaimOwnName {
		t.Fatalf("own name: %d %+v", w.Code, answer)
	}
}

func TestNameRuleAdministratorIsToldWhyAndMayPlaceInItsOwnSubscription(t *testing.T) {
	p, _ := nameRuleFixture(t)
	ctx := context.Background()
	if w := nameRuleCreate(t, p, nameRuleAlice, 2, "victim.test"); w.Code != 200 {
		t.Fatalf("alice: %d %s", w.Code, w.Body.String())
	}
	// Into bob's subscription: refused, with the reason and the holder.
	w := nameRuleCreate(t, p, nameRuleAdmin, 3, "shop.victim.test")
	answer := decodeClaimAnswer(t, w.Body.String())
	if w.Code != 409 || answer.Reason != hostnameClaimOtherAccount || answer.Vars["other"] != "victim.test" || answer.Vars["holder"] != "alice" {
		t.Fatalf("administrator into a customer's subscription: %d %+v", w.Code, answer)
	}
	// Into its own subscription: its deliberate placement.
	if w := nameRuleCreate(t, p, nameRuleAdmin, 1, "shop.victim.test"); w.Code != 200 {
		t.Fatalf("administrator's own subscription: %d %s", w.Code, w.Body.String())
	}
	// A server name: only an administrator's own subscription.
	if err := p.setSetting(ctx, settingMailHostname, "mail.celikhost.com"); err != nil {
		t.Fatal(err)
	}
	refusal, err := p.claimForSubscription(ctx, 3, "celikhost.com", false)
	if err != nil || refusal == nil || refusal.Reason != hostnameClaimServerName || refusal.Other != "mail.celikhost.com" {
		t.Fatalf("server name for a customer: %+v %v", refusal, err)
	}
	if refusal, err := p.claimForSubscription(ctx, 1, "celikhost.com", false); err != nil || refusal != nil {
		t.Fatalf("server name for the administrator's subscription: %+v %v", refusal, err)
	}
	// The exact name of another account is one name whoever asks.
	if refusal, err := p.claimForSubscription(ctx, 1, "victim.test", false); err != nil || refusal == nil || refusal.Reason != hostnameClaimOtherAccount {
		t.Fatalf("exact name for the administrator: %+v %v", refusal, err)
	}
}

func TestNameRuleResellerActsWithinItsCustomersNames(t *testing.T) {
	p, _ := nameRuleFixture(t)
	db := p.db.GetDB()
	for _, stmt := range []string{
		`INSERT INTO users(id,username,password_hash,email,role) VALUES (4,'reseller','hash','reseller@example.test','reseller')`,
		`UPDATE users SET parent_id = 4 WHERE id IN (2, 3)`,
		`INSERT INTO subscriptions(id,owner_id,name) VALUES (4,4,'reseller-own')`,
		`INSERT INTO domains(subscription_id,name) VALUES (4,'brand.test')`,
		`INSERT INTO domains(subscription_id,name) VALUES (2,'alice.test')`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	ctx := context.Background()
	// The reseller's own domain is another account for its customers, and
	// one customer's domain is another account for the other.
	for _, tc := range []struct {
		sub  int
		name string
	}{{3, "shop.brand.test"}, {3, "x.alice.test"}, {4, "y.alice.test"}} {
		if refusal, err := p.claimForSubscription(ctx, tc.sub, tc.name, false); err != nil || refusal == nil || refusal.Reason != hostnameClaimOtherAccount {
			t.Fatalf("%d %s: %+v %v", tc.sub, tc.name, refusal, err)
		}
	}
	// A customer's own subdomain stays a subdomain.
	if refusal, err := p.claimForSubscription(ctx, 2, "shop.alice.test", false); err != nil || refusal != nil {
		t.Fatalf("own subdomain: %+v %v", refusal, err)
	}
}

func TestNameRuleTransferRefusesNamesTheReceiverMayNotHold(t *testing.T) {
	p, _ := nameRuleFixture(t)
	db := p.db.GetDB()
	for _, stmt := range []string{
		`INSERT INTO subscriptions(id,owner_id,name) VALUES (5,2,'alice-two')`,
		`INSERT INTO domains(subscription_id,name) VALUES (2,'victim.test')`,
		`INSERT INTO domains(subscription_id,name) VALUES (5,'shop.victim.test')`,
		`INSERT INTO domains(subscription_id,name) VALUES (3,'bob.test')`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	ctx := context.Background()
	lines, err := p.transferNameOverlaps(ctx, db, 5, 3)
	if err != nil || len(lines) != 1 || lines[0] != "shop.victim.test meets victim.test" {
		t.Fatalf("lines = %v, %v", lines, err)
	}
	if lines, err := p.transferNameOverlaps(ctx, db, 3, 2); err != nil || len(lines) != 0 {
		t.Fatalf("an unrelated subscription: %v %v", lines, err)
	}
	refusal, err := p.transferNameRefusal(ctx, db, &Caller{ID: 9, Role: roleReseller}, 5, 3)
	if err != nil || refusal == nil || refusal.Code != errCodeTransferNameOverlap || len(refusal.Details) != 0 {
		t.Fatalf("reseller's refusal = %+v, %v", refusal, err)
	}
	refusal, err = p.transferNameRefusal(ctx, db, nameRuleAdmin, 5, 3)
	if err != nil || refusal == nil || len(refusal.Details) != 1 {
		t.Fatalf("administrator's refusal = %+v, %v", refusal, err)
	}
	if _, err := db.Exec(`UPDATE subscriptions SET owner_id = 3 WHERE id = 5`); err == nil {
		t.Fatal("the database moved the subscription")
	}
}

// An installation made before the rule may hold overlaps and a Node.js site
// on a port of a server service: listed for the administrator, untouched.
func TestNameRuleExistingInstallationListsWhatItHolds(t *testing.T) {
	p, _ := nameRuleFixture(t)
	db := p.db.GetDB()
	for _, trigger := range []string{"trg_domains_namespace_tenant_insert", "trg_domain_aliases_namespace_tenant_insert"} {
		if _, err := db.Exec(`DROP TRIGGER ` + trigger); err != nil {
			t.Fatal(err)
		}
	}
	ctx := context.Background()
	if err := p.setSetting(ctx, settingNS1, "ns1.celikhost.com"); err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`INSERT INTO domains(id,subscription_id,name) VALUES (20,2,'victim.test')`,
		`INSERT INTO domains(id,subscription_id,name) VALUES (21,3,'shop.victim.test')`,
		`INSERT INTO domains(id,subscription_id,name) VALUES (22,3,'celikhost.com')`,
		`INSERT INTO domains(id,subscription_id,name) VALUES (23,3,'fine.test')`,
		`INSERT INTO domains(id,subscription_id,name) VALUES (24,1,'admin-owned.victim.test')`,
		`INSERT INTO sites(domain_id,document_root,project_type,app_port) VALUES (23,'/srv/fine','node',8306)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	problems, unknown := p.namespaceProblems(ctx)
	if unknown != nil {
		t.Fatalf("unknown: %+v", unknown)
	}
	got := map[string]monitoringProblem{}
	for _, problem := range problems {
		got[problem.Key] = problem
	}
	// The name given later is listed (bob's), not the one it met (alice's);
	// the administrator's deliberate placement is not listed.
	for key, want := range map[string]string{
		"name_overlap:21":         "shop.victim.test",
		"name_server_reserved:22": "celikhost.com",
		"node_port_reserved:23":   "fine.test",
	} {
		if got[key].Subject != want || got[key].Action.To != "/domains/"+want {
			t.Fatalf("%s = %+v (all %+v)", key, got[key], problems)
		}
	}
	if got["name_overlap:21"].Name != "victim.test" || len(got) != 3 {
		t.Fatalf("listed: %+v", problems)
	}
	if got["node_port_reserved:23"].Value != 8306 {
		t.Fatalf("port = %v", got["node_port_reserved:23"].Value)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM domains`).Scan(&count); err != nil || count != 5 {
		t.Fatalf("domains after the check = %d, %v", count, err)
	}
}

func TestNodeSitePortRule(t *testing.T) {
	for port, allowed := range map[int]bool{3001: true, 3500: true, 3999: true, 3306: false, 3000: false, 4000: false, 8080: false, 8306: false, 30010: false} {
		if got := nodeSitePortChoiceRefusal(port) == ""; got != allowed {
			t.Fatalf("port %d allowed = %v", port, got)
		}
	}
}
