package main

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

// Option A (D-021 Q1): the second server is the main DNS server. The first
// installs no DNS until the second joined, hands over the list it prepared,
// then installs as backup with the publishing connection the second server
// issued; ns1 is the main.
func TestServerSetupPeerSecondServerAsMainTakesThePreparedList(t *testing.T) {
	first := joinFirstDraft()
	first.MainDNSServer = "peer"
	first.ImportedRecords = []serverSetupImportedRecord{{Name: "www.example.test", Type: "A", Content: "192.0.2.80", Decision: "import"}}
	pair := newPeerPair(t, first)
	if pair.plan.Draft.DNSRole != "secondary" || serverSetupPlanHasStep(pair.plan, "infrastructure_dns") || !serverSetupPlanHasStep(pair.plan, "peer_join") || !serverSetupPlanHasStep(pair.plan, "dns_publisher") {
		t.Fatalf("the first server waits as backup: %+v %+v", pair.plan.Draft, pair.plan.Steps)
	}
	view := issueJoinCode(t, pair.first)
	answer := pair.connect(t, view.Code)
	if answer.State != "ok" {
		t.Fatalf("connect: %+v", answer)
	}
	d := answer.Setup.Draft
	if d.DNSRole != "primary" || d.MainDNSServer != "this" || d.PeerNS != "ns2.example.test" || d.InfrastructureDNS == nil || d.InfrastructureDNS.PeerPanelDomain != "panel.example.test" ||
		len(d.ImportedRecords) != 1 || d.ImportedRecords[0].Name != "www.example.test" || d.DNSPublisherEndpoint != "" {
		t.Fatalf("the second server keeps the zone with the first's list: %+v", d)
	}
	link, _ := pair.second.panel.readServerSetupLink(context.Background())
	if link.EnrollmentCode != "" || link.Document.PublisherEndpoint != "" {
		t.Fatal("the backup issues no publishing connection")
	}
	_, plan := pair.secondReview(t, nil)
	if !plan.CanStart || plan.InfrastructureDNS == nil {
		t.Fatalf("second plan: %v", plan.Blockers)
	}
	found := false
	for _, record := range plan.InfrastructureDNS.Records {
		found = found || (record.Name == "panel.example.test" && record.Content == "203.0.113.10")
	}
	if !found {
		t.Fatalf("the first server's name is in the main's zone: %+v", plan.InfrastructureDNS.Records)
	}
	step := serverSetupExecutionStep{serverSetupPlanStep: plan.Steps[0], RequestID: strings.Repeat("5", 32), OwnerID: strings.Repeat("6", 32)}
	if done, err := pair.second.panel.runServerSetupPeerNames(context.Background(), plan, step); !done || err != nil {
		t.Fatalf("names: %v %v", done, err)
	}
	link, _ = pair.second.panel.readServerSetupLink(context.Background())
	var enrollments int
	pair.second.database.GetDB().QueryRow(`SELECT COUNT(*) FROM remote_dns_enrollments WHERE code_hash=?`, remoteDNSHash(link.IssuedEnrollment)).Scan(&enrollments)
	if !remoteDNSValidSecret(link.IssuedEnrollment) || enrollments != 1 {
		t.Fatal("the main issues one enrollment for the backup")
	}
	if done, err := pair.second.panel.runServerSetupPeerNames(context.Background(), plan, step); !done || err != nil {
		t.Fatalf("names again: %v %v", done, err)
	}
	pair.second.database.GetDB().QueryRow(`SELECT COUNT(*) FROM remote_dns_enrollments`).Scan(&enrollments)
	if enrollments != 1 {
		t.Fatalf("a retry issues nothing new: %d", enrollments)
	}
	for index, s := range pair.execution.Steps {
		if s.Kind == "peer_join" {
			break
		}
		pair.execution.Steps[index].Status = "succeeded"
	}
	if _, err := pair.first.panel.advanceServerSetupExecution(pair.plan, pair.execution); err != nil {
		t.Fatal(err)
	}
	effective, err := pair.first.panel.effectiveServerSetupPlan(context.Background(), pair.plan, pair.execution.ID)
	if err != nil || !effective.peerBound || effective.Draft.PeerIP != "198.51.100.20" || effective.Draft.PeerNS != "ns1.example.test" || effective.Draft.DNSPublisherEndpoint != "https://beta.example.test:"+itoaTest(panelPort()) || effective.InfrastructureDNS != nil {
		t.Fatalf("the first server binds the main: %+v %v", effective.Draft, err)
	}
	if _, _, err := setupDNSIdentity(effective.Draft); err != nil {
		t.Fatalf("a full backup identity: %v", err)
	}
	// Its publishing connection uses the handed-over code by itself; until
	// the main accepts, it waits with the reason.
	previous := remoteDNSExchange
	t.Cleanup(func() { remoteDNSExchange = previous })
	remoteDNSExchange = func(context.Context, string, string, string, any, any) error { return errServerSetupLinkUnreachable }
	for attempt := 0; attempt < 2; attempt++ {
		_, err := pair.first.panel.autoServerSetupDNSPublisher(context.Background(), effective, pair.execution.ID)
		var wait *serverSetupPeerWait
		if !errors.As(err, &wait) || wait.Code != "server_setup_dns_publisher_waiting" {
			t.Fatalf("publisher wait: %v", err)
		}
	}
	var connections int
	pair.first.database.GetDB().QueryRow(`SELECT COUNT(*) FROM remote_dns_connections WHERE enrollment_hash=?`, remoteDNSHash(link.IssuedEnrollment)).Scan(&connections)
	if connections != 1 {
		t.Fatalf("one prepared connection for the handed-over code: %d", connections)
	}
}

// The backup DNS server that receives the main domain's mail: its mail
// records are written on the main DNS server (MX, SPF, DMARC from its
// answers) and its DKIM record goes over the link.
func TestServerSetupPeerMailRecordsAreWrittenOnTheMain(t *testing.T) {
	pair := newPeerPair(t, joinFirstDraft())
	view := issueJoinCode(t, pair.first)
	if answer := pair.connect(t, view.Code); answer.State != "ok" {
		t.Fatalf("connect: %+v", answer)
	}
	link, _ := pair.second.panel.readServerSetupLink(context.Background())
	if link.Document.First.MainDomainMail {
		t.Fatal("the first server has no mail here")
	}
	var join serverSetupJoinRecord
	readServerSetupJSONSetting(context.Background(), pair.first.database.GetDB(), serverSetupJoinKey, &join)
	names := serverSetupPeerNames{Protocol: serverSetupLinkProtocol, RequestID: strings.Repeat("4", 32), ShortName: "beta", PanelDomain: "beta.example.test", MailHostname: "mail.example.test", DNSEngine: "bind",
		Mail: &serverSetupPeerMail{Hostname: "mail.example.test", MX: "now", DMARC: "add"}, Address: "https://198.51.100.20:" + itoaTest(panelPort()), Pin: strings.Repeat("cd", 32)}
	bearer := join.LinkID + "." + join.LinkCredential
	if w := joinMachineRequest(t, pair.first, serverSetupPeerNamesPath, bearer, names); w.Code != http.StatusOK {
		t.Fatalf("names: %d %s", w.Code, w.Body.String())
	}
	for index, s := range pair.execution.Steps {
		if s.Kind == "peer_join" {
			break
		}
		pair.execution.Steps[index].Status = "succeeded"
	}
	if _, err := pair.first.panel.advanceServerSetupExecution(pair.plan, pair.execution); err != nil {
		t.Fatal(err)
	}
	effective, err := pair.first.panel.effectiveServerSetupPlan(context.Background(), pair.plan, pair.execution.ID)
	if err != nil || effective.Draft.PeerMail == nil || effective.InfrastructureDNS == nil {
		t.Fatalf("bound: %+v %v", effective.Draft.PeerMail, err)
	}
	want := map[string]bool{"example.test MX 10 mail.example.test": false, "example.test TXT v=spf1 mx a:mail.example.test ~all": false, "mail.example.test A 198.51.100.20": false}
	for _, record := range effective.InfrastructureDNS.Records {
		key := record.Name + " " + record.Type + " " + serverSetupPublishedExpectation(record)
		if _, ok := want[key]; ok {
			want[key] = true
		}
	}
	for key, seen := range want {
		if !seen {
			t.Fatalf("missing %s in %+v", key, effective.InfrastructureDNS.Records)
		}
	}
	// DKIM over the link: refused while the zone does not exist, then written.
	dkim := serverSetupPeerDKIMRequest{Protocol: serverSetupLinkProtocol, RequestID: strings.Repeat("3", 32), Domain: "example.test", Selector: dkimSelector, Value: "v=DKIM1; k=rsa; p=QUJD"}
	if w := joinMachineRequest(t, pair.first, serverSetupPeerDKIMPath, bearer, dkim); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "server_setup_peer_dkim_zone_waiting") {
		t.Fatalf("no zone yet: %d %s", w.Code, w.Body.String())
	}
	if _, err := pair.first.database.GetDB().Exec(`INSERT INTO pdns_domains(name,type) VALUES('example.test','MASTER')`); err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		if w := joinMachineRequest(t, pair.first, serverSetupPeerDKIMPath, bearer, dkim); w.Code != http.StatusOK && w.Code != http.StatusAccepted {
			t.Fatalf("dkim %d: %d %s", attempt, w.Code, w.Body.String())
		}
	}
	var rows int
	pair.first.database.GetDB().QueryRow(`SELECT COUNT(*) FROM pdns_records WHERE name=? AND type='TXT'`, dkimSelector+"._domainkey.example.test").Scan(&rows)
	if rows != 1 {
		t.Fatalf("one DKIM record however often it is sent: %d", rows)
	}
	invalid := dkim
	invalid.Value = `v=DKIM1; p="x"`
	if w := joinMachineRequest(t, pair.first, serverSetupPeerDKIMPath, bearer, invalid); w.Code != http.StatusBadRequest {
		t.Fatalf("an invalid record: %d", w.Code)
	}
}

// Joining another first server withdraws the enrollment this server issued
// for the old one.
func TestServerSetupPeerNewLinkWithdrawsTheIssuedEnrollment(t *testing.T) {
	pair := newPeerPair(t, joinFirstDraft())
	issued := strings.Repeat("7", 64)
	db := pair.second.database.GetDB()
	if _, err := db.Exec(`INSERT INTO remote_dns_enrollments(code_hash,expires_at) VALUES(?,?)`, remoteDNSHash(issued), 9999999999); err != nil {
		t.Fatal(err)
	}
	old := serverSetupLinkRecord{RequestID: strings.Repeat("6", 32), Address: "https://203.0.113.99:2083", Pin: strings.Repeat("ab", 10), LinkID: strings.Repeat("5", 32), LinkCredential: strings.Repeat("4", 64), IssuedEnrollment: issued}
	if err := writeServerSetupJSONSetting(context.Background(), db, serverSetupLinkKey, old); err != nil {
		t.Fatal(err)
	}
	view := issueJoinCode(t, pair.first)
	if answer := pair.connect(t, view.Code); answer.State != "ok" {
		t.Fatalf("connect: %+v", answer)
	}
	var left int
	db.QueryRow(`SELECT COUNT(*) FROM remote_dns_enrollments WHERE code_hash=?`, remoteDNSHash(issued)).Scan(&left)
	if left != 0 {
		t.Fatal("the enrollment issued for the old first server is withdrawn")
	}
}
