package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.143 lab item 1 (alpha.144): an alias inside the customer's own
// domain got no DNS record (kampanya.randevu.dr.test NXDOMAIN on both
// servers). No release before alpha.144 published any record for an alias
// (alpha.142's lab: alias142.test REFUSED). An alias now answers like its
// domain in the zone of its account it lies inside, on the pair through the
// zone's publication, and goes again with the alias; only what CelikPanel
// added is removed.

func newA144AliasDNSPanel(t *testing.T) (*Panel, *strictDNSRPCAgent) {
	t.Helper()
	t.Setenv("CELIKPANEL_SERVER_IP", "192.0.3.210")
	t.Setenv("CELIKPANEL_SERVER_IPV6", "")
	p := newDNSPanelForTest(t)
	setDNSIdentityForTest(t, p, "paired")
	if _, err := p.db.GetDB().Exec(`
		INSERT INTO users (id, username, password_hash, email, role) VALUES
			(7101, 'alice', 'x', 'alice@example.test', 'customer'),
			(7102, 'bob', 'x', 'bob@example.test', 'customer');
		INSERT INTO subscriptions (id, owner_id, name, status) VALUES
			(7111, 7101, 'alice one', 'active'), (7112, 7101, 'alice two', 'active'), (7113, 7102, 'bob', 'active');
		INSERT INTO domains (id, subscription_id, name, status, dns_management) VALUES
			(7121, 7111, 'randevu.test', 'active', 'local'),
			(7122, 7112, 'kahve.test', 'active', 'local'),
			(7123, 7113, 'pastane.test', 'active', 'local');`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	for _, zone := range []string{"randevu.test", "kahve.test", "pastane.test"} {
		seedStrictDNSZone(t, p, zone)
	}
	agent := &strictDNSRPCAgent{}
	attachStrictDNSRPCAgent(t, p, agent)
	return p, agent
}

func a144SyncedRecords(agent *strictDNSRPCAgent, zone string) []transport.ZoneRecord {
	agent.mu.Lock()
	defer agent.mu.Unlock()
	var last []transport.ZoneRecord
	for _, request := range agent.syncRequests {
		if request.Domain == zone {
			last = request.Records
		}
	}
	return last
}

func a144Published(records []transport.ZoneRecord, name, typ, content string) bool {
	for _, record := range records {
		if strings.TrimSuffix(record.Name, ".") == name && record.Type == typ && record.Content == content {
			return true
		}
	}
	return false
}

func TestA144AliasInsideItsAccountsZoneAnswersAndGoesAgain(t *testing.T) {
	p, agent := newA144AliasDNSPanel(t)
	ctx := context.Background()
	// The lab's shape: an alias of randevu.test inside randevu.test.
	if got := p.publishAliasDNS(ctx, 7121, "kampanya.randevu.test"); got != aliasDNSPublished {
		t.Fatalf("publish: %s", got)
	}
	assertSubdomainRecordCount(t, p, "randevu.test", "kampanya.randevu.test", "A", "192.0.3.210", 1)
	if !a144Published(a144SyncedRecords(agent, "randevu.test"), "kampanya.randevu.test", "A", "192.0.3.210") {
		t.Fatalf("the zone's publication does not carry the alias: %+v", a144SyncedRecords(agent, "randevu.test"))
	}
	// An alias of randevu.test inside another zone of the same account.
	if got := p.publishAliasDNS(ctx, 7121, "kampanya.kahve.test"); got != aliasDNSPublished {
		t.Fatalf("publish in the account's other zone: %s", got)
	}
	assertSubdomainRecordCount(t, p, "kahve.test", "kampanya.kahve.test", "A", "192.0.3.210", 1)
	// Never inside another account's zone, and nothing for a name outside
	// every zone kept here.
	for _, alias := range []string{"kampanya.pastane.test", "elsewhere.example"} {
		if got := p.publishAliasDNS(ctx, 7121, alias); got != aliasDNSElsewhere {
			t.Fatalf("%s: %s", alias, got)
		}
	}
	assertSubdomainRecordCount(t, p, "pastane.test", "kampanya.pastane.test", "A", "192.0.3.210", 0)

	// Removed with the alias, and the removal is published.
	before := len(agent.syncCalls)
	if err := p.withdrawAliasDNS(ctx, "kampanya.randevu.test", ""); err != nil {
		t.Fatal(err)
	}
	assertSubdomainRecordCount(t, p, "randevu.test", "kampanya.randevu.test", "A", "192.0.3.210", 0)
	if len(agent.syncCalls) == before || a144Published(a144SyncedRecords(agent, "randevu.test"), "kampanya.randevu.test", "A", "192.0.3.210") {
		t.Fatal("the removal was not published")
	}
	// Deleting randevu.test removes its alias's records in kahve.test, which stays.
	if err := p.withdrawDomainAliasesDNS(ctx, "randevu.test", "randevu.test"); err != nil {
		t.Fatal(err)
	}
	assertSubdomainRecordCount(t, p, "kahve.test", "kampanya.kahve.test", "A", "192.0.3.210", 1) // no alias row: nothing named
	if _, err := p.db.GetDB().Exec(`INSERT INTO domain_aliases (domain_id, alias) VALUES (7121, 'kampanya.kahve.test')`); err != nil {
		t.Fatal(err)
	}
	if err := p.withdrawDomainAliasesDNS(ctx, "randevu.test", "randevu.test"); err != nil {
		t.Fatal(err)
	}
	assertSubdomainRecordCount(t, p, "kahve.test", "kampanya.kahve.test", "A", "192.0.3.210", 0)
}

func TestA144AliasNeverTakesTheOwnersRecords(t *testing.T) {
	p, _ := newA144AliasDNSPanel(t)
	ctx := context.Background()
	zoneID := strictDNSZoneID(t, p, "randevu.test")
	// The owner already points the name here: nothing is added, and the
	// alias's removal leaves the owner's record.
	if _, err := p.db.GetDB().Exec(`INSERT INTO pdns_records (domain_id, name, type, content, ttl) VALUES (?, 'own.randevu.test', 'A', '192.0.3.210', 600)`, zoneID); err != nil {
		t.Fatal(err)
	}
	if got := p.publishAliasDNS(ctx, 7121, "own.randevu.test"); got != aliasDNSPublished {
		t.Fatalf("publish over the owner's equal record: %s", got)
	}
	if err := p.withdrawAliasDNS(ctx, "own.randevu.test", ""); err != nil {
		t.Fatal(err)
	}
	assertSubdomainRecordCount(t, p, "randevu.test", "own.randevu.test", "A", "192.0.3.210", 1)
	// The owner points the name elsewhere: kept, nothing written.
	if _, err := p.db.GetDB().Exec(`INSERT INTO pdns_records (domain_id, name, type, content, ttl) VALUES (?, 'away.randevu.test', 'A', '198.51.100.7', 600)`, zoneID); err != nil {
		t.Fatal(err)
	}
	if got := p.publishAliasDNS(ctx, 7121, "away.randevu.test"); got != aliasDNSKept {
		t.Fatalf("publish over the owner's other address: %s", got)
	}
	assertSubdomainRecordCount(t, p, "randevu.test", "away.randevu.test", "A", "192.0.3.210", 0)
	if raw := p.setting(ctx, aliasDNSKey("away.randevu.test")); raw != "" {
		t.Fatalf("a note was kept for a record CelikPanel did not add: %s", raw)
	}
}

func TestA144AliasRecordsSurviveAFailedPublicationAndArePublishedLater(t *testing.T) {
	p, agent := newA144AliasDNSPanel(t)
	agent.mu.Lock()
	agent.failZone = "randevu.test"
	agent.mu.Unlock()
	if got := p.publishAliasDNS(context.Background(), 7121, "kampanya.randevu.test"); got != aliasDNSPending {
		t.Fatalf("publish while the zone cannot be published: %s", got)
	}
	assertSubdomainRecordCount(t, p, "randevu.test", "kampanya.randevu.test", "A", "192.0.3.210", 1)
	agent.mu.Lock()
	agent.failZone = ""
	agent.mu.Unlock()
	if err := p.syncZoneToDNS(context.Background(), "randevu.test", false); err != nil {
		t.Fatal(err)
	}
	if !a144Published(a144SyncedRecords(agent, "randevu.test"), "kampanya.randevu.test", "A", "192.0.3.210") {
		t.Fatal("the zone's next publication does not carry the alias")
	}
}

// Item 4: the alias refusals reach the page coded (its own words), never as
// the server's English sentence alone.
func TestA144AliasRefusalsAreCoded(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeAliasReissueFailed(recorder, "kampanya.kahve.test", errAliasReissueUntrusted)
	var body apiErrorBody
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil || recorder.Code != http.StatusConflict ||
		body.Code != errCodeAliasCertificateReissueFailed || body.Reason != "untrusted_source" || body.Vars["name"] != "kampanya.kahve.test" {
		t.Fatalf("reissue refusal: %d %s", recorder.Code, recorder.Body.String())
	}
	recorder = httptest.NewRecorder()
	writeAliasReissueFailed(recorder, "x.kahve.test", errAliasReissueMailName)
	_ = json.Unmarshal(recorder.Body.Bytes(), &body)
	if body.Reason != "mail_name" {
		t.Fatalf("mail-name refusal: %s", recorder.Body.String())
	}
	recorder = httptest.NewRecorder()
	if !writeHSTSRemovalRefusal(recorder, errHSTSEnabled, "the alias") {
		t.Fatal("HSTS refusal not recognized")
	}
	_ = json.Unmarshal(recorder.Body.Bytes(), &body)
	if body.Code != errCodeHSTSRemovalBlocked || body.Reason != "enabled" {
		t.Fatalf("HSTS refusal: %s", recorder.Body.String())
	}
	if writeHSTSRemovalRefusal(httptest.NewRecorder(), context.Canceled, "the alias") {
		t.Fatal("another error taken for an HSTS refusal")
	}
}
