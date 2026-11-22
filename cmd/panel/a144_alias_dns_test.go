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
	if pending, err := p.withdrawAliasDNS(ctx, 7121, "kampanya.randevu.test", ""); err != nil || pending {
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
	// The owner already points the name here: the record is the owner's
	// (kept), nothing is added, and the alias's removal leaves it.
	if _, err := p.db.GetDB().Exec(`INSERT INTO pdns_records (domain_id, name, type, content, ttl) VALUES (?, 'own.randevu.test', 'A', '192.0.3.210', 600)`, zoneID); err != nil {
		t.Fatal(err)
	}
	if got := p.publishAliasDNS(ctx, 7121, "own.randevu.test"); got != aliasDNSKept {
		t.Fatalf("publish over the owner's equal record: %s", got)
	}
	if _, err := p.withdrawAliasDNS(ctx, 7121, "own.randevu.test", ""); err != nil {
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

// Review of PR #355, finding 4: an owner's A record at the name (no AAAA)
// keeps the panel from adding an AAAA beside it.
func TestA144AliasAddsNothingBesideAnOwnersAddressRecord(t *testing.T) {
	p, _ := newA144AliasDNSPanel(t)
	t.Setenv("CELIKPANEL_SERVER_IPV6", "2001:db8::210")
	zoneID := strictDNSZoneID(t, p, "randevu.test")
	if _, err := p.db.GetDB().Exec(`INSERT INTO pdns_records (domain_id, name, type, content, ttl) VALUES (?, 'shop.randevu.test', 'A', '192.0.3.210', 600)`, zoneID); err != nil {
		t.Fatal(err)
	}
	if got := p.publishAliasDNS(context.Background(), 7121, "shop.randevu.test"); got != aliasDNSKept {
		t.Fatalf("beside the owner's A: %s", got)
	}
	assertSubdomainRecordCount(t, p, "randevu.test", "shop.randevu.test", "AAAA", "2001:db8::210", 0)
	if raw := p.setting(context.Background(), aliasDNSKey("shop.randevu.test")); raw != "" {
		t.Fatalf("a note was kept: %s", raw)
	}
}

// Review of PR #355, findings 1 and 6: a removal whose publication does not
// finish is no error of the domain's deletion; the zone is marked as
// waiting, the alias rows say so, and the hourly round publishes it.
func TestA144AliasPublicationThatWaitsIsRetriedAndNeverStopsADeletion(t *testing.T) {
	p, agent := newA144AliasDNSPanel(t)
	ctx := context.Background()
	if got := p.publishAliasDNS(ctx, 7121, "kampanya.kahve.test"); got != aliasDNSPublished {
		t.Fatalf("publish: %s", got)
	}
	if _, err := p.db.GetDB().Exec(`INSERT INTO domain_aliases (domain_id, alias) VALUES (7121, 'kampanya.kahve.test')`); err != nil {
		t.Fatal(err)
	}
	// The backup waits (the publication of kahve.test does not finish).
	agent.mu.Lock()
	agent.failZone = "kahve.test"
	agent.mu.Unlock()
	if err := p.withdrawDomainAliasesDNS(ctx, "randevu.test", "randevu.test"); err != nil {
		t.Fatalf("the deletion's alias step stopped: %v", err)
	}
	assertSubdomainRecordCount(t, p, "kahve.test", "kampanya.kahve.test", "A", "192.0.3.210", 0)
	if zones, _ := p.aliasZonesUnpublished(ctx); !zones["kahve.test"] {
		t.Fatalf("the wait was not recorded: %v", zones)
	}
	// An alias whose record waits is named as waiting on its domain's page.
	if got := p.publishAliasDNS(ctx, 7122, "neu.kahve.test"); got != aliasDNSPending {
		t.Fatalf("publish while waiting: %s", got)
	}
	if waiting := p.aliasesWaitingForPublication(ctx, []string{"neu.kahve.test", "other.example"}); len(waiting) != 1 || waiting[0] != "neu.kahve.test" {
		t.Fatalf("waiting aliases: %v", waiting)
	}
	// The hourly round publishes it once the backup answers, and the wait goes.
	agent.mu.Lock()
	agent.failZone = ""
	agent.mu.Unlock()
	before := len(agent.syncCalls)
	p.republishAliasZones(ctx)
	if len(agent.syncCalls) == before || !a144Published(a144SyncedRecords(agent, "kahve.test"), "neu.kahve.test", "A", "192.0.3.210") ||
		a144Published(a144SyncedRecords(agent, "kahve.test"), "kampanya.kahve.test", "A", "192.0.3.210") {
		t.Fatalf("the round did not publish the waiting zone: %+v", a144SyncedRecords(agent, "kahve.test"))
	}
	if zones, _ := p.aliasZonesUnpublished(ctx); len(zones) != 0 {
		t.Fatalf("the wait stayed after publication: %v", zones)
	}
}

// Review of PR #355, finding 7: a domain whose own DNS is managed elsewhere
// still takes its alias records out of a zone of the account kept here.
func TestA144DeletingADomainWithExternalDNSRemovesItsAliasRecordsHere(t *testing.T) {
	p, _ := newA144AliasDNSPanel(t)
	ctx := context.Background()
	if _, err := p.db.GetDB().Exec(`
		INSERT INTO domains (id, subscription_id, name, status, dns_management) VALUES (7124, 7111, 'elsewhere.example', 'active', 'external');
		INSERT INTO domain_aliases (domain_id, alias) VALUES (7124, 'promo.kahve.test');`); err != nil {
		t.Fatal(err)
	}
	if got := p.publishAliasDNS(ctx, 7124, "promo.kahve.test"); got != aliasDNSPublished {
		t.Fatalf("publish: %s", got)
	}
	if err := p.removeDomainDNSForDeletion(ctx, "elsewhere.example", ""); err != nil {
		t.Fatal(err)
	}
	assertSubdomainRecordCount(t, p, "kahve.test", "promo.kahve.test", "A", "192.0.3.210", 0)
}

// Review of PR #355, finding 5: the handlers' own name checks answer coded.
func TestA144AliasHandlersRefuseAnInvalidNameCoded(t *testing.T) {
	p := newDNSPanelForTest(t)
	for _, c := range []struct {
		method, path, body string
	}{
		{http.MethodPost, "/api/v1/domains/7/aliases", `{"alias":"a..b"}`},
		{http.MethodDelete, "/api/v1/domains/7/aliases/a..b", ""},
	} {
		recorder := httptest.NewRecorder()
		p.handleDomainAliases(recorder, httptest.NewRequest(c.method, c.path, strings.NewReader(c.body)))
		var body apiErrorBody
		if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil || recorder.Code != http.StatusBadRequest || body.Code != errCodeAliasInvalid {
			t.Fatalf("%s: %d %s", c.method, recorder.Code, recorder.Body.String())
		}
	}
}

// Review of PR #355 fixes, finding 5: records are withdrawn only while the
// zone still belongs to the account that held it; a zone made again by
// another account keeps its records and the note is cleared.
func TestA144AliasRecordsStayInAZoneNowHeldByAnotherAccount(t *testing.T) {
	p, _ := newA144AliasDNSPanel(t)
	ctx := context.Background()
	if got := p.publishAliasDNS(ctx, 7121, "kampanya.kahve.test"); got != aliasDNSPublished {
		t.Fatalf("publish: %s", got)
	}
	// kahve.test now belongs to bob (deleted by alice, made again by him).
	if _, err := p.db.GetDB().Exec(`UPDATE domains SET subscription_id = 7113 WHERE name = 'kahve.test'`); err != nil {
		t.Fatal(err)
	}
	if pending, err := p.withdrawAliasDNS(ctx, 7121, "kampanya.kahve.test", ""); err != nil || pending {
		t.Fatalf("withdraw: %v %v", pending, err)
	}
	assertSubdomainRecordCount(t, p, "kahve.test", "kampanya.kahve.test", "A", "192.0.3.210", 1)
	if raw := p.setting(ctx, aliasDNSKey("kampanya.kahve.test")); raw != "" {
		t.Fatalf("the note stayed: %s", raw)
	}
}

// Independent review of the last commit, C1: after an ordinary
// subscription transfer the alias and its zone moved together, so the
// records CelikPanel added go with the alias; a note written without the
// owner behaves the same. The owner kept in the note does not decide.
func TestA144AliasRecordsGoWithTheAliasAfterASubscriptionTransfer(t *testing.T) {
	p, _ := newA144AliasDNSPanel(t)
	ctx := context.Background()
	if got := p.publishAliasDNS(ctx, 7121, "kampanya.randevu.test"); got != aliasDNSPublished {
		t.Fatalf("publish: %s", got)
	}
	if got := p.publishAliasDNS(ctx, 7121, "yaz.randevu.test"); got != aliasDNSPublished {
		t.Fatalf("publish: %s", got)
	}
	// The subscription of randevu.test goes from alice to bob.
	if _, err := p.db.GetDB().Exec(`UPDATE subscriptions SET owner_id = 7102 WHERE id = 7111`); err != nil {
		t.Fatal(err)
	}
	if _, err := p.withdrawAliasDNS(ctx, 7121, "kampanya.randevu.test", ""); err != nil {
		t.Fatal(err)
	}
	assertSubdomainRecordCount(t, p, "randevu.test", "kampanya.randevu.test", "A", "192.0.3.210", 0)
	// A note without the owner (a lab build's) is decided the same way.
	var added aliasDNSAdded
	if err := json.Unmarshal([]byte(p.setting(ctx, aliasDNSKey("yaz.randevu.test"))), &added); err != nil {
		t.Fatal(err)
	}
	added.Owner = 0
	raw, _ := json.Marshal(added)
	if err := p.setSetting(ctx, aliasDNSKey("yaz.randevu.test"), string(raw)); err != nil {
		t.Fatal(err)
	}
	if _, err := p.withdrawAliasDNS(ctx, 7121, "yaz.randevu.test", ""); err != nil {
		t.Fatal(err)
	}
	assertSubdomainRecordCount(t, p, "randevu.test", "yaz.randevu.test", "A", "192.0.3.210", 0)
	for _, alias := range []string{"kampanya.randevu.test", "yaz.randevu.test"} {
		if raw := p.setting(ctx, aliasDNSKey(alias)); raw != "" {
			t.Fatalf("%s: the note stayed: %s", alias, raw)
		}
	}
}

// Independent review of the last commit, C3: a zone whose DNS mode cannot be
// read keeps its wait (it may still be kept here); it is neither published
// nor cleared until the mode reads again.
func TestA144AWaitingZoneWhoseModeCannotBeReadKeepsItsWait(t *testing.T) {
	p, agent := newA144AliasDNSPanel(t)
	ctx := context.Background()
	p.markAliasZoneUnpublished(ctx, "kahve.test")
	// The domains table cannot be read for a moment: the mode read fails.
	if _, err := p.db.GetDB().Exec(`ALTER TABLE domains RENAME TO a144_domains_away`); err != nil {
		t.Fatal(err)
	}
	before := len(agent.syncCalls)
	p.republishAliasZones(ctx)
	zones, _ := p.aliasZonesUnpublished(ctx)
	if _, err := p.db.GetDB().Exec(`ALTER TABLE a144_domains_away RENAME TO domains`); err != nil {
		t.Fatal(err)
	}
	if !zones["kahve.test"] || len(agent.syncCalls) != before {
		t.Fatalf("after a read error: marker %v, publications %d -> %d", zones, before, len(agent.syncCalls))
	}
	p.republishAliasZones(ctx)
	if zones, _ := p.aliasZonesUnpublished(ctx); len(zones) != 0 || len(agent.syncCalls) == before {
		t.Fatalf("once readable: marker %v, publications %d -> %d", zones, before, len(agent.syncCalls))
	}
}

// Review of PR #355 fixes, finding 4: a waiting zone that is no longer kept
// here is cleared without publishing; a marker matches its zone without
// case (and is published and cleared).
func TestA144AWaitingZoneNoLongerKeptHereIsClearedWithoutPublishing(t *testing.T) {
	p, agent := newA144AliasDNSPanel(t)
	ctx := context.Background()
	p.markAliasZoneUnpublished(ctx, "gone.test")
	before := len(agent.syncCalls)
	p.republishAliasZones(ctx)
	if len(agent.syncCalls) != before {
		t.Fatal("a zone not kept here was published")
	}
	if zones, _ := p.aliasZonesUnpublished(ctx); len(zones) != 0 {
		t.Fatalf("the marker stayed: %v", zones)
	}
	if err := p.setSetting(ctx, aliasDNSUnpublishedPrefix+"KAHVE.test", "1"); err != nil {
		t.Fatal(err)
	}
	p.republishAliasZones(ctx)
	if len(agent.syncCalls) == before || agent.syncCalls[len(agent.syncCalls)-1] != "kahve.test" {
		t.Fatalf("the zone named in another case was not published: %v", agent.syncCalls)
	}
	if zones, _ := p.aliasZonesUnpublished(ctx); len(zones) != 0 {
		t.Fatalf("the marker stayed: %v", zones)
	}
}
