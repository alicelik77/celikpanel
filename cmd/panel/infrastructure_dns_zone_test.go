package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Gap 5 of the second server as a full peer (2026-10-16): the primary's
// infrastructure zone is listed beside the domains and opens the existing DNS
// record editor; setup's address records are managed by setup, the owner's
// records stay the owner's.

const infrastructureTestPlanID = "0123456789abcdef0123456789abcdef"

// seedSetupInfrastructureZone writes the zone the way setup's step leaves it:
// the zone, its ownership record, setup's address records and one owner TXT,
// and a setup draft that names the zone.
func seedSetupInfrastructureZone(t *testing.T, p *Panel) serverSetupDraft {
	t.Helper()
	d := infrastructureDNSTestDraft()
	d.Purpose = "web_mail"
	d.MailHostname = "mail.frankfurt.example.test"
	d.InfrastructureDNS.PeerMailHostname = "mail.boston.example.test"
	seedInfrastructureDNSZone(t, p, d.InfrastructureDNS.Zone)
	var zoneID int64
	if err := p.db.GetDB().QueryRow(`SELECT id FROM pdns_domains WHERE name=?`, d.InfrastructureDNS.Zone).Scan(&zoneID); err != nil {
		t.Fatal(err)
	}
	owner, _ := json.Marshal(serverSetupInfrastructureDNSOwner{Zone: d.InfrastructureDNS.Zone, ZoneID: zoneID, PlanID: infrastructureTestPlanID})
	if _, err := p.db.GetDB().Exec(`INSERT INTO panel_settings(key,value) VALUES(?,?)`, serverSetupInfrastructureDNSOwnerKey(d.InfrastructureDNS.Zone), string(owner)); err != nil {
		t.Fatal(err)
	}
	for _, r := range []struct{ name, typ, content string }{
		{d.PanelDomain, "A", d.LocalIP},
		{d.MailHostname, "A", d.LocalIP},
		{"boston.example.test", "A", d.PeerIP},
		{"ns1.example.test", "A", d.LocalIP},
		{"ns2.example.test", "A", d.PeerIP},
		{d.InfrastructureDNS.Zone, "TXT", "\"owner-verification\""},
	} {
		if _, err := p.db.GetDB().Exec(`INSERT INTO pdns_records(domain_id,name,type,content,ttl) VALUES(?,?,?,?,300)`, zoneID, r.name, r.typ, r.content); err != nil {
			t.Fatal(err)
		}
	}
	raw, _ := json.Marshal(d)
	if _, err := p.db.GetDB().Exec(`UPDATE server_setup_state SET draft_json=?, status='ready', completed_at='2026-10-16T00:00:00Z' WHERE id=1`, string(raw)); err != nil {
		t.Fatal(err)
	}
	return d
}

func infrastructureRecordID(t *testing.T, p *Panel, name, typ string) int {
	t.Helper()
	var id int
	if err := p.db.GetDB().QueryRow(`SELECT id FROM pdns_records WHERE name=? AND type=?`, name, typ).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func infraAdminRequest(method, target, body string) *http.Request {
	r := httptest.NewRequest(method, target, strings.NewReader(body))
	return r.WithContext(context.WithValue(r.Context(), callerKey, &Caller{ID: 1, Role: roleAdmin}))
}

func TestInfrastructureDNSZoneIsListedFromSetupsOwnershipRecord(t *testing.T) {
	p := newDNSPanelForTest(t)
	// Control: a zone without setup's ownership record is not listed.
	seedInfrastructureDNSZone(t, p, "unrelated.test")
	w := httptest.NewRecorder()
	p.handleInfrastructureDNSZones(w, infraAdminRequest(http.MethodGet, infrastructureDNSZonesPath, ""))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"zones":[]`) {
		t.Fatalf("unowned zone listed: %d %s", w.Code, w.Body.String())
	}
	d := seedSetupInfrastructureZone(t, p)
	w = httptest.NewRecorder()
	p.handleInfrastructureDNSZones(w, infraAdminRequest(http.MethodGet, infrastructureDNSZonesPath, ""))
	var answer struct {
		Zones []infrastructureDNSZone `json:"zones"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &answer); err != nil || len(answer.Zones) != 1 {
		t.Fatalf("answer=%s err=%v", w.Body.String(), err)
	}
	got := answer.Zones[0]
	if got.Zone != d.InfrastructureDNS.Zone || got.Kind != "infrastructure" || got.DomainID != 0 || got.Records != 9 {
		t.Fatalf("entry=%+v", got)
	}
	// An ownership record whose zone id no longer matches is not trusted.
	if _, err := p.db.GetDB().Exec(`UPDATE panel_settings SET value=replace(value,'"zone_id":','"zone_id":9') WHERE key=?`, serverSetupInfrastructureDNSOwnerKey(d.InfrastructureDNS.Zone)); err != nil {
		t.Fatal(err)
	}
	if ok, err := p.isInfrastructureDNSZone(context.Background(), d.InfrastructureDNS.Zone); err != nil || ok {
		t.Fatalf("mismatched ownership trusted: %v %v", ok, err)
	}
}

func TestInfrastructureDNSRecordsSayWhoManagesThem(t *testing.T) {
	p := newDNSPanelForTest(t)
	d := seedSetupInfrastructureZone(t, p)
	w := httptest.NewRecorder()
	p.handleInfrastructureDNSZones(w, infraAdminRequest(http.MethodGet, infrastructureDNSZonesPath+"/"+d.InfrastructureDNS.Zone+"/records", ""))
	if w.Code != http.StatusOK {
		t.Fatalf("list=%d %s", w.Code, w.Body.String())
	}
	var answer struct {
		Records []DNSRecord `json:"records"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}
	managed := map[string]string{}
	for _, r := range answer.Records {
		managed[r.Name+" "+r.Type] = r.ManagedBy
	}
	for _, name := range []string{d.PanelDomain, d.MailHostname, "boston.example.test", "ns1.example.test", "ns2.example.test"} {
		if managed[name+" A"] != "setup" {
			t.Fatalf("%s managed=%q", name, managed[name+" A"])
		}
	}
	if managed[d.InfrastructureDNS.Zone+" TXT"] != "" {
		t.Fatal("the owner's TXT record was called managed")
	}
	// An address the owner changed is no longer setup's record: it is the
	// owner's (setup's next review reports the difference).
	if _, err := p.db.GetDB().Exec(`UPDATE pdns_records SET content='198.51.100.9' WHERE name=? AND type='A'`, d.PanelDomain); err != nil {
		t.Fatal(err)
	}
	managedNow, err := p.infrastructureDNSManagedRecords(context.Background(), d.InfrastructureDNS.Zone)
	if err != nil || managedNow[infrastructureDNSRecordKey(d.PanelDomain, "A", "198.51.100.9")] != "" {
		t.Fatalf("changed address managed=%v err=%v", managedNow, err)
	}
	// Another zone's records carry no marker.
	seedInfrastructureDNSZone(t, p, "customer.test")
	w = httptest.NewRecorder()
	p.handleListDNSRecords(w, infraAdminRequest(http.MethodGet, "/", ""), "customer.test")
	if strings.Contains(w.Body.String(), "managed_by") {
		t.Fatalf("customer zone marked: %s", w.Body.String())
	}
}

func TestInfrastructureDNSRefusesToChangeSetupsRecordsAndKeepsOwnerEdits(t *testing.T) {
	p := newDNSPanelForTest(t)
	d := seedSetupInfrastructureZone(t, p)
	zone := d.InfrastructureDNS.Zone
	panelID := infrastructureRecordID(t, p, d.PanelDomain, "A")
	w := httptest.NewRecorder()
	if p.handleDeleteDNSRecord(w, infraAdminRequest(http.MethodDelete, "/?id="+itoaForTest(panelID), ""), zone) {
		t.Fatal("setup's record was deleted")
	}
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), errCodeInfrastructureDNSRecordBySetup) {
		t.Fatalf("delete=%d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	body := `{"id":` + itoaForTest(panelID) + `,"content":"198.51.100.7","ttl":300,"prio":0,"disabled":false}`
	if p.handleUpdateDNSRecord(w, infraAdminRequest(http.MethodPut, "/", body), zone) || w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), errCodeInfrastructureDNSRecordBySetup) {
		t.Fatalf("update=%d %s", w.Code, w.Body.String())
	}
	var content string
	p.db.GetDB().QueryRow(`SELECT content FROM pdns_records WHERE id=?`, panelID).Scan(&content)
	if content != d.LocalIP {
		t.Fatalf("setup's record changed to %s", content)
	}
	// Control: the owner's TXT record passes the guard (the ledger delete is
	// committed; publication then needs the DNS engine this fixture lacks).
	txtID := infrastructureRecordID(t, p, zone, "TXT")
	w = httptest.NewRecorder()
	p.handleDeleteDNSRecord(w, infraAdminRequest(http.MethodDelete, "/?id="+itoaForTest(txtID), ""), zone)
	if strings.Contains(w.Body.String(), errCodeInfrastructureDNSRecordBySetup) {
		t.Fatalf("owner record refused: %s", w.Body.String())
	}
	var left int
	p.db.GetDB().QueryRow(`SELECT COUNT(*) FROM pdns_records WHERE id=?`, txtID).Scan(&left)
	if left != 0 {
		t.Fatal("the owner's record was not deleted")
	}
	// The same guard on an ordinary customer zone: nothing is refused.
	seedInfrastructureDNSZone(t, p, "customer.test")
	if _, err := p.db.GetDB().Exec(`INSERT INTO pdns_records(domain_id,name,type,content,ttl) SELECT id,'www.customer.test','A',?,300 FROM pdns_domains WHERE name='customer.test'`, d.LocalIP); err != nil {
		t.Fatal(err)
	}
	if bySetup, err := p.infrastructureDNSRecordManagedBySetup(context.Background(), "customer.test", "www.customer.test", "A", d.LocalIP); err != nil || bySetup {
		t.Fatalf("customer record managed=%v err=%v", bySetup, err)
	}
}

func TestInfrastructureDNSRouteRefusesAnUnknownZoneAndOtherMethods(t *testing.T) {
	p := newDNSPanelForTest(t)
	seedInfrastructureDNSZone(t, p, "customer.test")
	for _, c := range []struct {
		method, path string
		status       int
	}{
		{http.MethodGet, infrastructureDNSZonesPath + "/customer.test/records", http.StatusNotFound},
		{http.MethodGet, infrastructureDNSZonesPath + "/Customer.Test/records", http.StatusNotFound},
		{http.MethodGet, infrastructureDNSZonesPath + "/customer.test/other", http.StatusNotFound},
		{http.MethodPost, infrastructureDNSZonesPath, http.StatusMethodNotAllowed},
	} {
		w := httptest.NewRecorder()
		p.handleInfrastructureDNSZones(w, infraAdminRequest(c.method, c.path, ""))
		if w.Code != c.status {
			t.Fatalf("%s %s = %d %s", c.method, c.path, w.Code, w.Body.String())
		}
	}
	if !isAdminOnlyPath(infrastructureDNSZonesPath) || !isAdminOnlyPath(infrastructureDNSZonesPath+"/example.test/records") {
		t.Fatal("the infrastructure zone routes are not administrator only")
	}
}

func TestRemovingASiteForTheInfrastructureZoneKeepsTheZone(t *testing.T) {
	p := newDNSPanelForTest(t)
	d := seedSetupInfrastructureZone(t, p)
	sub := seedSetupDNSOwner(t, p)
	seedSetupDNSDomain(t, p, sub, d.InfrastructureDNS.Zone, setupDNSModeLocal)
	if err := p.removeDomainDNSForDeletion(context.Background(), d.InfrastructureDNS.Zone, ""); err != nil {
		t.Fatal(err)
	}
	var zones int
	p.db.GetDB().QueryRow(`SELECT COUNT(*) FROM pdns_domains WHERE name=?`, d.InfrastructureDNS.Zone).Scan(&zones)
	if zones != 1 {
		t.Fatal("removing the site removed the infrastructure zone")
	}
	// The listing names the website that now uses the name.
	zonesListed, err := p.infrastructureDNSZones(context.Background())
	if err != nil || len(zonesListed) != 1 || zonesListed[0].DomainID == 0 {
		t.Fatalf("listing=%+v err=%v", zonesListed, err)
	}
}
