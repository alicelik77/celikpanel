package main

import (
	"context"
	"net/http"
	"testing"
)

// Fix round after the alpha.142 lab (21 November 2026).
//
// 1. A subdomain of one's own domain is an ordinary subdomain also when a
//    server name or an administrator's name lies above that domain (the lab:
//    every customer's domain under the server's zone dr.test, which the
//    administrator's subscription holds as the main domain). The panel
//    (insideOwnDomainUnder) and the database (migration 081) give the same
//    answer for the same case.
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
	stubClaimMachineNames(t)
	execAll(t, p,
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
	)
	return p
}

// stubClaimMachineNames: the machine's and the panel certificate's names
// play no part in these tests.
func stubClaimMachineNames(t *testing.T) {
	t.Helper()
	savedName, savedCert := claimLocalServerName, claimPanelCertificateName
	claimLocalServerName = func() (string, error) { return "", nil }
	claimPanelCertificateName = func() string { return "" }
	t.Cleanup(func() { claimLocalServerName, claimPanelCertificateName = savedName, savedCert })
}

func execAll(t *testing.T, p *Panel, stmts ...string) {
	t.Helper()
	for _, stmt := range stmts {
		if _, err := p.db.GetDB().Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
}

// ownSubdomainCase is one requested name: a domain for subscription sub, or
// an alias of domain domain.
type ownSubdomainCase struct {
	what    string
	sub     int
	domain  int
	name    string
	allowed bool
}

// checkOwnSubdomainCases runs each case through the panel's rule and through
// the database's triggers (a rolled-back insert) and wants the same answer
// from both; each answer pair is logged as a table row.
func checkOwnSubdomainCases(t *testing.T, p *Panel, cases []ownSubdomainCase) {
	t.Helper()
	ctx := context.Background()
	db := p.db.GetDB()
	verdict := func(ok bool) string {
		if ok {
			return "allowed"
		}
		return "refused"
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
		goAllowed := refusal == nil
		if goAllowed != c.allowed {
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
		dbAllowed := err == nil
		if dbAllowed != c.allowed {
			t.Errorf("%s (%s): the database allowed=%v, want %v (%v)", c.what, c.name, dbAllowed, c.allowed, err)
		}
		if err := tx.Rollback(); err != nil {
			t.Fatal(err)
		}
		t.Logf("CASE | %s | %s | rule %s | triggers %s", c.what, c.name, verdict(goAllowed), verdict(dbAllowed))
	}
}

// checkOwnSubdomainTransfers moves subscription sub to each receiver through
// the panel's rule and through the triggers (rolled back): the same answer.
func checkOwnSubdomainTransfers(t *testing.T, p *Panel, sub int, receivers map[int]bool) {
	t.Helper()
	ctx := context.Background()
	db := p.db.GetDB()
	for receiver, allowed := range receivers {
		lines, err := p.transferNameOverlaps(ctx, db, sub, receiver)
		if err != nil {
			t.Fatal(err)
		}
		goAllowed := len(lines) == 0
		if goAllowed != allowed {
			t.Errorf("transfer of %d to %d: the panel's rule allowed=%v, want %v (%v)", sub, receiver, goAllowed, allowed, lines)
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		_, err = tx.Exec(`UPDATE subscriptions SET owner_id = ? WHERE id = ?`, receiver, sub)
		dbAllowed := err == nil
		if dbAllowed != allowed {
			t.Errorf("transfer of %d to %d: the database allowed=%v, want %v (%v)", sub, receiver, dbAllowed, allowed, err)
		}
		if err := tx.Rollback(); err != nil {
			t.Fatal(err)
		}
		t.Logf("TRANSFER | subscription %d to account %d | rule allowed=%v | triggers allowed=%v", sub, receiver, goAllowed, dbAllowed)
	}
}

// The same table of cases through the panel's rule and through the
// database's triggers: the same answer each time. Here dr.test is a zone
// hosted here that no domain holds.
func TestOwnSubdomainUnderAServerNameRuleAndTriggersAgree(t *testing.T) {
	p := ownSubdomainFixture(t)
	checkOwnSubdomainCases(t, p, []ownSubdomainCase{
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
	})
	// A subscription that changes owner: the administrator's
	// blog3.kahvedukkani.dr.test may go to alice, never to bob.
	checkOwnSubdomainTransfers(t, p, 5, map[int]bool{2: true, 3: false})
}

// labShapeFixture: the alpha.142 lab's real shape. The administrator's own
// subscription holds the server's zone dr.test as a domain (the main
// domain's website on the infrastructure zone), and adm.kahvedukkani.dr.test
// and deep.inner.kahvedukkani.dr.test inside alice's domain on purpose.
// alice (2) holds kahvedukkani.dr.test, shop.www.dr.test (under the
// administrator's implicit www.dr.test) and a.bobzone.test (under bob's
// bobzone.test: an overlap of two ordinary accounts made before the rule);
// bob (3) holds pastane.dr.test and bobzone.test; carol (4) holds nothing.
// The customers' names were given before the administrator's and the
// server's names, as on an installation made before migration 080.
// Subscription 5 is the administrator's, holding blog3.kahvedukkani.dr.test.
func labShapeFixture(t *testing.T) *Panel {
	t.Helper()
	p, _ := nameRuleFixture(t)
	stubClaimMachineNames(t)
	execAll(t, p,
		`INSERT INTO users(id,username,password_hash,email,role) VALUES (4,'carol','hash','carol@example.test','customer')`,
		`INSERT INTO subscriptions(id,owner_id,name) VALUES (4,4,'carol')`,
		`INSERT INTO subscriptions(id,owner_id,name) VALUES (5,1,'admin-two')`,
		`INSERT INTO domains(id,subscription_id,name,dns_management) VALUES (10,2,'kahvedukkani.dr.test','external')`,
		`INSERT INTO domains(id,subscription_id,name,dns_management) VALUES (11,3,'pastane.dr.test','external')`,
		`INSERT INTO domains(id,subscription_id,name,dns_management) VALUES (15,2,'shop.www.dr.test','external')`,
		`INSERT INTO domains(id,subscription_id,name,dns_management) VALUES (16,2,'a.bobzone.test','external')`,
	)
	// bob's bobzone.test above alice's a.bobzone.test is refused today, so
	// it is put in the way an earlier release did, without the rule's
	// trigger, which is created again right after.
	var insertTrigger string
	if err := p.db.GetDB().QueryRow(`SELECT sql FROM sqlite_schema WHERE type = 'trigger' AND name = 'trg_domains_namespace_tenant_insert'`).Scan(&insertTrigger); err != nil {
		t.Fatal(err)
	}
	execAll(t, p,
		`DROP TRIGGER trg_domains_namespace_tenant_insert`,
		`INSERT INTO domains(id,subscription_id,name,dns_management) VALUES (17,3,'bobzone.test','external')`,
		insertTrigger,
		`INSERT INTO domains(id,subscription_id,name,dns_management) VALUES (12,1,'dr.test','local')`,
		`INSERT INTO domains(id,subscription_id,name,dns_management) VALUES (13,1,'adm.kahvedukkani.dr.test','external')`,
		`INSERT INTO domains(id,subscription_id,name,dns_management) VALUES (18,1,'deep.inner.kahvedukkani.dr.test','external')`,
		`INSERT INTO domains(id,subscription_id,name,dns_management) VALUES (14,5,'blog3.kahvedukkani.dr.test','external')`,
		`INSERT INTO pdns_domains(name,type) VALUES ('dr.test','NATIVE')`,
		`INSERT INTO panel_settings(key,value) VALUES ('server_setup_infrastructure_zone:dr.test','{}')`,
	)
	return p
}

// The lab's real shape through the panel's rule and the triggers: an
// administrator's name above a domain the account already holds is treated
// like a server name; every other overlap stays refused, and an overlap
// between two ordinary accounts is never extended.
func TestOwnSubdomainUnderAnAdministratorsNameRuleAndTriggersAgree(t *testing.T) {
	p := labShapeFixture(t)
	checkOwnSubdomainCases(t, p, []ownSubdomainCase{
		{what: "lab: a subdomain inside the own domain under the administrator's dr.test", sub: 2, name: "blog2.kahvedukkani.dr.test", allowed: true},
		{what: "lab: an alias inside the own domain", domain: 10, name: "www2.kahvedukkani.dr.test", allowed: true},
		{what: "inside the own domain under the administrator's implicit www.dr.test", sub: 2, name: "x.shop.www.dr.test", allowed: true},
		{what: "bob inside his own domain", sub: 3, name: "cake.pastane.dr.test", allowed: true},
		{what: "equal to an administrator's name", sub: 2, name: "adm.kahvedukkani.dr.test"},
		{what: "an administrator's name between the domain and the name", sub: 2, name: "x.adm.kahvedukkani.dr.test"},
		{what: "an administrator's name under the requested name", sub: 2, name: "inner.kahvedukkani.dr.test"},
		{what: "the administrator's zone itself", sub: 2, name: "dr.test"},
		{what: "the administrator's implicit www.dr.test itself", sub: 2, name: "www.dr.test"},
		{what: "under the administrator's zone without such a domain", sub: 2, name: "other.dr.test"},
		{what: "an account without any domain", sub: 4, name: "carol.dr.test"},
		{what: "another ordinary account's name above the own domain", sub: 2, name: "x.a.bobzone.test"},
		{what: "another ordinary account's name above the own domain, as an alias", domain: 16, name: "y.a.bobzone.test"},
		{what: "bob inside alice's domain", sub: 3, name: "blog.kahvedukkani.dr.test"},
	})
	// The administrator's blog3.kahvedukkani.dr.test may go to alice, never
	// to bob or carol.
	checkOwnSubdomainTransfers(t, p, 5, map[int]bool{2: true, 3: false, 4: false})
}

// The lab's case through the page's own request: alice adds
// blog.kahvedukkani.dr.test; bob's same request still gets the one neutral
// sentence.
func TestOwnSubdomainUnderTheServersZoneIsCreated(t *testing.T) {
	for _, shape := range []string{"server zone", "lab shape"} {
		var p *Panel
		if shape == "server zone" {
			p = ownSubdomainFixture(t)
		} else {
			p = labShapeFixture(t)
		}
		// The zone's records are not this test's subject (a name inside a
		// zone hosted here needs the DNS engine): dr.test stays a server
		// name as the infrastructure zone setup recorded.
		execAll(t, p, `DELETE FROM pdns_domains WHERE name = 'dr.test'`)
		if w := nameRuleCreate(t, p, nameRuleAlice, 2, "blog.kahvedukkani.dr.test"); w.Code != http.StatusOK {
			t.Fatalf("%s: alice's subdomain: %d %s", shape, w.Code, w.Body.String())
		}
		w := nameRuleCreate(t, p, nameRuleBob, 3, "shop.kahvedukkani.dr.test")
		nameRuleRefused(t, w, shape+": bob's shop.kahvedukkani.dr.test")
		if answer := decodeClaimAnswer(t, w.Body.String()); answer.Reason != "" || len(answer.Vars) != 1 {
			t.Fatalf("%s: bob's answer says more than the name: %+v", shape, answer)
		}
		nameRuleRefused(t, nameRuleCreate(t, p, nameRuleAlice, 2, "other.dr.test"), shape+": alice's other.dr.test")
	}
}

// Correction 2: the administrator's own domain dr.test lies far above
// alice's kahvedukkani.dr.test. Placing a name under alice's domain into
// bob's subscription is refused naming alice's domain and alice, the
// nearest name the request meets (before: "meets dr.test of the account
// admin", the list ran shortest first).
func TestNameRuleAdministratorIsToldTheNearestHolder(t *testing.T) {
	p, _ := nameRuleFixture(t)
	execAll(t, p,
		`INSERT INTO domains(subscription_id,name) VALUES (2,'kahvedukkani.dr.test')`,
		`INSERT INTO domains(subscription_id,name) VALUES (2,'deep.adm5.kahvedukkani.dr.test')`,
		`INSERT INTO domains(subscription_id,name) VALUES (1,'dr.test')`,
	)
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
