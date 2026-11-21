package main

import (
	"context"
	"net/http"
	"testing"
)

// Fix round after the alpha.142 lab (21 November 2026).
//
// 1. A subdomain of one's own domain is an ordinary subdomain also when a
//    server name lies above that domain (the lab: every customer's domain
//    under the server's zone dr.test). The panel (insideOwnDomainUnder) and
//    the database (migration 081) give the same answer for the same case.
// 2. An administrator refused placing a name into another account is told
//    the other account's nearest name, not a name far above it.

// ownSubdomainFixture: alice (2) holds kahvedukkani.dr.test and example.com
// with the alias al.dr.test, bob (3) holds pastane.dr.test and infra.test,
// all given before the server's names below existed (as on an installation
// made before migration 080): the zone dr.test hosted here without a domain
// and recorded as an infrastructure zone, the infrastructure zone
// infra.test, the mail host name mail.example.com and the nameserver
// ns1.zone.example.com. Subscription 5 is the administrator's, holding
// blog3.kahvedukkani.dr.test on purpose. The domains keep their DNS
// elsewhere, so only the name rule is involved.
func ownSubdomainFixture(t *testing.T) *Panel {
	t.Helper()
	p, _ := nameRuleFixture(t)
	savedName, savedCert := claimLocalServerName, claimPanelCertificateName
	claimLocalServerName = func() (string, error) { return "", nil }
	claimPanelCertificateName = func() string { return "" }
	t.Cleanup(func() { claimLocalServerName, claimPanelCertificateName = savedName, savedCert })
	for _, stmt := range []string{
		`INSERT INTO domains(id,subscription_id,name,dns_management) VALUES (10,2,'kahvedukkani.dr.test','external')`,
		`INSERT INTO domains(id,subscription_id,name,dns_management) VALUES (11,3,'pastane.dr.test','external')`,
		`INSERT INTO domains(id,subscription_id,name,dns_management) VALUES (12,2,'example.com','external')`,
		`INSERT INTO domains(id,subscription_id,name,dns_management) VALUES (13,3,'infra.test','external')`,
		`INSERT INTO domain_aliases(id,domain_id,alias) VALUES (30,12,'al.dr.test')`,
		`INSERT INTO subscriptions(id,owner_id,name) VALUES (5,1,'admin-two')`,
		`INSERT INTO domains(id,subscription_id,name,dns_management) VALUES (14,5,'blog3.kahvedukkani.dr.test','external')`,
		`INSERT INTO pdns_domains(name,type) VALUES ('dr.test','NATIVE')`,
		`INSERT INTO panel_settings(key,value) VALUES ('server_setup_infrastructure_zone:dr.test','{}')`,
		`INSERT INTO panel_settings(key,value) VALUES ('server_setup_infrastructure_zone:infra.test','{}')`,
		`INSERT INTO panel_settings(key,value) VALUES ('mail_hostname','mail.example.com')`,
		`INSERT INTO panel_settings(key,value) VALUES ('nameserver1','ns1.zone.example.com')`,
	} {
		if _, err := p.db.GetDB().Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	return p
}

// The same table of cases through the panel's rule and through the
// database's triggers: the same answer each time.
func TestOwnSubdomainUnderAServerNameRuleAndTriggersAgree(t *testing.T) {
	p := ownSubdomainFixture(t)
	ctx := context.Background()
	db := p.db.GetDB()
	type tc struct {
		what    string
		sub     int // the subscription that receives a domain
		domain  int // or: the domain that receives an alias
		name    string
		allowed bool
	}
	cases := []tc{
		// Inside a domain the account holds under the server's zone.
		{what: "the lab's case", sub: 2, name: "blog.kahvedukkani.dr.test", allowed: true},
		{what: "deeper inside the own domain", sub: 2, name: "x.y.kahvedukkani.dr.test", allowed: true},
		{what: "inside the own alias", sub: 2, name: "shop.al.dr.test", allowed: true},
		{what: "an alias inside the own domain", domain: 10, name: "www2.kahvedukkani.dr.test", allowed: true},
		{what: "bob inside his own domain", sub: 3, name: "cake.pastane.dr.test", allowed: true},
		{what: "an unrelated name", sub: 2, name: "fine.example.org", allowed: true},
		// Still refused.
		{what: "the server name itself", sub: 2, name: "dr.test"},
		{what: "under the server name without such a domain", sub: 2, name: "other.dr.test"},
		{what: "a server name under the requested name", sub: 2, name: "zone.example.com"},
		{what: "the server's mail host name itself", sub: 2, name: "mail.example.com"},
		{what: "the server name between the domain and the name", sub: 2, name: "x.mail.example.com"},
		{what: "another account's domain under the zone", sub: 2, name: "blog.pastane.dr.test"},
		{what: "another account's domain, as an alias", domain: 10, name: "x.pastane.dr.test"},
		{what: "bob inside alice's domain", sub: 3, name: "blog.kahvedukkani.dr.test"},
		{what: "an account holding the server name itself (implicit names are not counted)", sub: 3, name: "x.www.infra.test"},
		{what: "an account holding the server name itself", sub: 3, name: "blog.infra.test"},
	}
	for _, c := range cases {
		sub := c.sub
		if c.domain != 0 {
			if err := db.QueryRow(`SELECT subscription_id FROM domains WHERE id = ?`, c.domain).Scan(&sub); err != nil {
				t.Fatal(err)
			}
		}
		refusal, err := p.claimForSubscription(ctx, sub, c.name, false)
		if err != nil {
			t.Fatalf("%s: %v", c.what, err)
		}
		if goAllowed := refusal == nil; goAllowed != c.allowed {
			t.Errorf("%s (%s): the panel's rule allowed=%v, want %v (%+v)", c.what, c.name, goAllowed, c.allowed, refusal)
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		if c.domain != 0 {
			_, err = tx.Exec(`INSERT INTO domain_aliases(domain_id,alias) VALUES (?,?)`, c.domain, c.name)
		} else {
			_, err = tx.Exec(`INSERT INTO domains(subscription_id,name) VALUES (?,?)`, c.sub, c.name)
		}
		if dbAllowed := err == nil; dbAllowed != c.allowed {
			t.Errorf("%s (%s): the database allowed=%v, want %v (%v)", c.what, c.name, dbAllowed, c.allowed, err)
		}
		if err := tx.Rollback(); err != nil {
			t.Fatal(err)
		}
	}
	// A subscription that changes owner: the administrator's
	// blog3.kahvedukkani.dr.test may go to alice, never to bob.
	for _, receiver := range []struct {
		id      int
		allowed bool
	}{{2, true}, {3, false}} {
		lines, err := p.transferNameOverlaps(ctx, db, 5, receiver.id)
		if err != nil {
			t.Fatal(err)
		}
		if goAllowed := len(lines) == 0; goAllowed != receiver.allowed {
			t.Errorf("transfer to %d: the panel's rule allowed=%v, want %v (%v)", receiver.id, goAllowed, receiver.allowed, lines)
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		_, err = tx.Exec(`UPDATE subscriptions SET owner_id = ? WHERE id = 5`, receiver.id)
		if dbAllowed := err == nil; dbAllowed != receiver.allowed {
			t.Errorf("transfer to %d: the database allowed=%v, want %v (%v)", receiver.id, dbAllowed, receiver.allowed, err)
		}
		if err := tx.Rollback(); err != nil {
			t.Fatal(err)
		}
	}
}

// The lab's case through the page's own request: alice adds
// blog.kahvedukkani.dr.test; bob's same request still gets the one neutral
// sentence.
func TestOwnSubdomainUnderTheServersZoneIsCreated(t *testing.T) {
	p := ownSubdomainFixture(t)
	// The zone's records are not this test's subject (a name inside a zone
	// hosted here needs the DNS engine): dr.test stays a server name as the
	// infrastructure zone setup recorded.
	if _, err := p.db.GetDB().Exec(`DELETE FROM pdns_domains WHERE name = 'dr.test'`); err != nil {
		t.Fatal(err)
	}
	if w := nameRuleCreate(t, p, nameRuleAlice, 2, "blog.kahvedukkani.dr.test"); w.Code != http.StatusOK {
		t.Fatalf("alice's subdomain: %d %s", w.Code, w.Body.String())
	}
	w := nameRuleCreate(t, p, nameRuleBob, 3, "shop.kahvedukkani.dr.test")
	nameRuleRefused(t, w, "bob's shop.kahvedukkani.dr.test")
	if answer := decodeClaimAnswer(t, w.Body.String()); answer.Reason != "" || len(answer.Vars) != 1 {
		t.Fatalf("bob's answer says more than the name: %+v", answer)
	}
	nameRuleRefused(t, nameRuleCreate(t, p, nameRuleAlice, 2, "other.dr.test"), "alice's other.dr.test")
}

// Correction 2: the administrator's own domain dr.test lies far above
// alice's kahvedukkani.dr.test. Placing a name under alice's domain into
// bob's subscription is refused naming alice's domain and alice, the
// nearest name the request meets (before: "meets dr.test of the account
// admin", the list ran shortest first).
func TestNameRuleAdministratorIsToldTheNearestHolder(t *testing.T) {
	p, _ := nameRuleFixture(t)
	for _, stmt := range []string{
		`INSERT INTO domains(subscription_id,name) VALUES (2,'kahvedukkani.dr.test')`,
		`INSERT INTO domains(subscription_id,name) VALUES (2,'deep.adm5.kahvedukkani.dr.test')`,
		`INSERT INTO domains(subscription_id,name) VALUES (1,'dr.test')`,
	} {
		if _, err := p.db.GetDB().Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	w := nameRuleCreate(t, p, nameRuleAdmin, 3, "adm5.kahvedukkani.dr.test")
	answer := decodeClaimAnswer(t, w.Body.String())
	if w.Code != http.StatusConflict || answer.Reason != hostnameClaimOtherAccount || answer.Vars["other"] != "kahvedukkani.dr.test" || answer.Vars["holder"] != "alice" {
		t.Fatalf("administrator into bob's subscription: %d %+v", w.Code, answer)
	}
	// A name of the administrator's own zone only: that is the nearest.
	w = nameRuleCreate(t, p, nameRuleAdmin, 3, "ns9.dr.test")
	if answer := decodeClaimAnswer(t, w.Body.String()); answer.Vars["other"] != "dr.test" || answer.Vars["holder"] != "admin" {
		t.Fatalf("a name under the administrator's zone: %d %+v", w.Code, answer)
	}
	// Nearest order: the name itself, then above from the closest, then
	// under from the closest (the implicit www and mail names of
	// deep.adm5.kahvedukkani.dr.test come after it).
	held, err := reservationsOverlapping(context.Background(), p.db.GetDB(), "adm5.kahvedukkani.dr.test")
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	for _, r := range held {
		order = append(order, r.hostname)
	}
	want := []string{"kahvedukkani.dr.test", "dr.test", "deep.adm5.kahvedukkani.dr.test"}
	if len(order) != 5 {
		t.Fatalf("order = %v", order)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order = %v, want %v", order, want)
		}
	}
}
