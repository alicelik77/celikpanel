package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Regression tests for the alpha.114 lab run 1 findings on a server joined
// by code (E1, E2, E6, E8). Component tests only; the lab rerun proves the
// owner's path.

// --- E1: the main DNS server's answer to the publishing connection ---

func postRemoteDNSAccept(t *testing.T, p *Panel, code string, request remoteDNSAcceptRequest) (int, string) {
	t.Helper()
	raw, _ := json.Marshal(request)
	r := httptest.NewRequest(http.MethodPost, "https://dns.remote.test:2083/api/v1/dns/remote/accept", bytes.NewReader(raw))
	r.Header.Set("Authorization", "Bearer "+code)
	w := httptest.NewRecorder()
	p.handleRemoteDNSMachine(w, r)
	return w.Code, remoteDNSRefusalCode(w.Body)
}

func TestRemoteDNSEnrollmentRefusalsAreTypedAndBusyIsAWait(t *testing.T) {
	f := newRemoteDNSHTTPFixture(t)
	request := remoteDNSAcceptRequest{ClientID: strings.Repeat("b", 32), Credential: strings.Repeat("c", 64), Label: "another client"}
	if status, code := postRemoteDNSAccept(t, f.receiver, strings.Repeat("9", 64), request); status != http.StatusUnauthorized || code != errCodeRemoteDNSEnrollmentUnknown {
		t.Fatalf("unknown code: %d %q", status, code)
	}
	f.connect(t)
	if status, code := postRemoteDNSAccept(t, f.receiver, f.code, request); status != http.StatusUnauthorized || code != errCodeRemoteDNSEnrollmentUsed {
		t.Fatalf("used by another connection: %d %q", status, code)
	}
	fresh := strings.Repeat("d", 64)
	if _, err := f.receiver.db.GetDB().Exec(`INSERT INTO remote_dns_enrollments(code_hash,expires_at) VALUES(?,0)`, remoteDNSHash(fresh)); err != nil {
		t.Fatal(err)
	}
	if status, code := postRemoteDNSAccept(t, f.receiver, fresh, request); status != http.StatusUnauthorized || code != errCodeRemoteDNSEnrollmentExpired {
		t.Fatalf("expired: %d %q", status, code)
	}
	// A database this server cannot read now is "ask again" (503), never a
	// refusal (the lab's first server was in its own setup).
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if status, code := f.receiver.remoteDNSEnrollmentVerdict(ctx, remoteDNSHash(fresh), request); status != http.StatusServiceUnavailable || code != errCodeRemoteDNSBusy {
		t.Fatalf("unreadable enrollment: %d %q", status, code)
	}
}

func TestServerSetupPublisherRefusalOnlyAVerifiedAnswerAsksForANewCode(t *testing.T) {
	f, _ := setupOperationFixture(t)
	p := f.panel
	ctx := context.Background()
	execution := strings.Repeat("e", 32)
	endpoint := "https://frankfurt.example.test:2083"
	waiting := &serverSetupPeerWait{Code: "server_setup_dns_publisher_waiting"}
	classify := func(err error) (string, bool) {
		got := p.serverSetupPublisherRefusal(ctx, execution, endpoint, err, waiting)
		var wait *serverSetupPeerWait
		var failure *serverSetupChildFailure
		switch {
		case errors.As(got, &wait):
			if !strings.Contains(wait.Message, "frankfurt.example.test") && wait != waiting {
				t.Fatalf("wait does not name the main DNS server: %q", wait.Message)
			}
			return wait.Code, false
		case errors.As(got, &failure):
			return failure.Code, true
		}
		t.Fatalf("unclassified %v", got)
		return "", false
	}
	for _, c := range []struct {
		err    error
		code   string
		failed bool
	}{
		{&remoteDNSHTTPError{StatusCode: 401, Code: errCodeRemoteDNSEnrollmentExpired}, "server_setup_dns_publisher_refused", true},
		{&remoteDNSHTTPError{StatusCode: 401, Code: errCodeRemoteDNSEnrollmentUnknown}, "server_setup_dns_publisher_refused", true},
		{&remoteDNSHTTPError{StatusCode: 409, Code: errCodeRemoteDNSEnrollmentUsed}, "server_setup_dns_publisher_refused", true},
		{&remoteDNSHTTPError{StatusCode: 503, Code: "PANEL_STARTING"}, "server_setup_dns_publisher_peer_busy", false},
		{&remoteDNSHTTPError{StatusCode: 503, Code: errCodeRemoteDNSBusy}, "server_setup_dns_publisher_peer_busy", false},
		{&remoteDNSHTTPError{StatusCode: 500}, "server_setup_dns_publisher_peer_busy", false},
		{&remoteDNSHTTPError{StatusCode: 429}, "server_setup_dns_publisher_peer_busy", false},
		{&remoteDNSHTTPError{StatusCode: 403, Code: "license_required"}, "server_setup_dns_publisher_peer_license", false},
		{&remoteDNSHTTPError{StatusCode: 409, Code: "REMOTE_DNS_AUTHORITY_NOT_READY"}, "server_setup_dns_publisher_waiting", false},
		{errors.New("remote DNS HTTPS exchange failed"), "server_setup_dns_publisher_waiting", false},
	} {
		if code, failed := classify(c.err); code != c.code || failed != c.failed {
			t.Fatalf("%v: %s failed=%v, want %s failed=%v", c.err, code, failed, c.code, c.failed)
		}
	}
	// The lab case: an alpha.113 main in its own setup answers 409 (or 401)
	// without a reason. That is a wait, bounded; past the bound it fails
	// with what to check, not with "the code expired".
	now := time.Date(2026, 10, 31, 19, 23, 23, 0, time.UTC)
	previous := serverSetupPublisherNow
	serverSetupPublisherNow = func() time.Time { return now }
	t.Cleanup(func() { serverSetupPublisherNow = previous })
	unexplained := &remoteDNSHTTPError{StatusCode: 409}
	if code, failed := classify(unexplained); code != "server_setup_dns_publisher_peer_unexplained" || failed {
		t.Fatalf("unexplained refusal: %s %v", code, failed)
	}
	now = now.Add(29 * time.Minute)
	if code, failed := classify(&remoteDNSHTTPError{StatusCode: 401, Code: "REMOTE_DNS_UNAUTHORIZED"}); code != "server_setup_dns_publisher_peer_unexplained" || failed {
		t.Fatalf("within the bound: %s %v", code, failed)
	}
	now = now.Add(2 * time.Minute)
	if code, failed := classify(unexplained); code != "server_setup_dns_publisher_unconfirmed" || !failed {
		t.Fatalf("past the bound: %s %v", code, failed)
	}
	p.clearServerSetupPublisherRefusedSince(ctx, execution)
	if code, failed := classify(unexplained); code != "server_setup_dns_publisher_peer_unexplained" || failed {
		t.Fatalf("an accepted connection restarts the bound: %s %v", code, failed)
	}
}

// --- E2: a joined pair refuses a port change on both sides after setup ---

func TestPanelPortRefusedOnBothSidesOfAFinishedJoin(t *testing.T) {
	panel, _, _, _ := setupPanelPortTest(t, ":2083")
	ctx := context.Background()
	if refusal := panel.panelPortPeerRefusal(ctx); refusal != nil {
		t.Fatalf("an unpaired server was refused: %+v", refusal)
	}
	// The first server after setup: its join record keeps the second
	// server's names (its address), the link credential has ended.
	join := serverSetupJoinRecord{LinkID: strings.Repeat("1", 32), Names: &serverSetupPeerNames{}, EndedCredentialHash: "x"}
	if err := writeServerSetupJSONSetting(ctx, panel.db.GetDB(), serverSetupJoinKey, join); err != nil {
		t.Fatal(err)
	}
	if refusal := panel.panelPortPeerRefusal(ctx); refusal == nil || panelPortRefusalCode(refusal) != "PANEL_PORT_PAIRED" {
		t.Fatalf("first server after setup: %+v", refusal)
	}
	join.PeerRetiredAt = "2026-10-31T00:00:00Z"
	if err := writeServerSetupJSONSetting(ctx, panel.db.GetDB(), serverSetupJoinKey, join); err != nil {
		t.Fatal(err)
	}
	if refusal := panel.panelPortPeerRefusal(ctx); refusal != nil {
		t.Fatalf("a retired peer still blocks: %+v", refusal)
	}
	// The joined server after setup: its link record stays, its credential
	// has ended (the lab's boston answered 200 here).
	link := serverSetupLinkRecord{Address: "https://192.0.2.1:1923", LinkID: strings.Repeat("2", 32), EndedCredentialHash: "y"}
	if err := panel.writeServerSetupLink(ctx, panel.db.GetDB(), link); err != nil {
		t.Fatal(err)
	}
	if refusal := panel.panelPortPeerRefusal(ctx); refusal == nil || panelPortRefusalCode(refusal) != "PANEL_PORT_PAIRED" {
		t.Fatalf("joined server after setup: %+v", refusal)
	}
	if _, err := panel.startPanelPortChange(ctx, 5001); panelPortRefusalCode(err) != "PANEL_PORT_PAIRED" {
		t.Fatalf("start on the joined server: %v", err)
	}
}

// --- E8: the final DNS check of a later run on a joined backup ---

func TestServerSetupJoinedBackupMailExtensionPassesTheDNSCheck(t *testing.T) {
	f := newSecondaryHostingFixture(t)
	f.wait(t)
	if response := f.bind(t, f.id); response.Code != http.StatusAccepted {
		t.Fatalf("bind %d %s", response.Code, response.Body.String())
	}
	execution, err := f.f.panel.latestServerSetupExecution(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.f.panel.advanceServerSetupExecution(f.plan, execution); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if ready, reason, err := f.f.panel.serverSetupSecondaryHostingReadinessReason(ctx, f.plan.Draft); err != nil || !ready {
		t.Fatalf("first setup: %v %s %v", ready, reason, err)
	}
	// A later run without a publishing step (the mail added from
	// Components, or an extension of a server set up on alpha.113).
	later := f.plan
	later.Steps = nil
	for _, step := range f.plan.Steps {
		if step.Kind != "dns_publisher" && step.Kind != "dns" {
			later.Steps = append(later.Steps, step)
		}
	}
	later.ID = ""
	later.ID = serverSetupPlanIdentity(later)
	raw, _ := json.Marshal(later)
	if _, err := f.f.database.GetDB().Exec(`INSERT INTO server_setup_plans(id,revision,plan_json,requested_by,created_at) VALUES(?,?,?,?,?)`, later.ID, later.Revision, string(raw), f.f.userID, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	// The first setup finished; the later run is the one active run.
	if _, err := f.f.database.GetDB().Exec(`UPDATE server_setup_executions SET status='succeeded' WHERE id=?`, f.execution.ID); err != nil {
		t.Fatal(err)
	}
	run := serverSetupExecution{ID: strings.Repeat("f", 32), RequestID: strings.Repeat("f", 32), PlanID: later.ID, Status: "waiting", Phase: "verification"}
	for _, step := range later.Steps {
		run.Steps = append(run.Steps, serverSetupExecutionStep{serverSetupPlanStep: step, Status: "succeeded", RequestID: serverSetupID(run.ID, step.ID, "request"), OwnerID: serverSetupID(run.ID, step.ID, "owner")})
	}
	encoded, _ := json.Marshal(run)
	if _, err := f.f.database.GetDB().Exec(`INSERT INTO server_setup_executions(id,request_id,plan_id,status,execution_json,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`, run.ID, run.RequestID, later.ID, run.Status, string(encoded), "9999-01-01T00:00:00Z", "9999-01-01T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	if latest, err := f.f.panel.latestServerSetupExecution(ctx); err != nil || latest.ID != run.ID {
		t.Fatalf("fixture: the later run is not the latest: %v", err)
	}
	ready, reason, err := f.f.panel.serverSetupSecondaryHostingReadinessReason(ctx, later.Draft)
	if err != nil || !ready {
		t.Fatalf("the later run waits for ever on dns: ready=%v reason=%q err=%v", ready, reason, err)
	}
	// The committed connection is still proven live: an unready main DNS
	// server is said, typed, as not confirmed now.
	f.proof.Ready = false
	if ready, reason, err := f.f.panel.serverSetupSecondaryHostingReadinessReason(ctx, later.Draft); ready || err != nil || reason != setupDNSReasonPublisherUnconfirmed {
		t.Fatalf("unready main: %v %q %v", ready, reason, err)
	}
	f.proof.Ready = true
	if _, err := f.f.database.GetDB().Exec(`DELETE FROM panel_settings WHERE key=?`, settingRemoteDNSConnection); err != nil {
		t.Fatal(err)
	}
	if ready, reason, err := f.f.panel.serverSetupSecondaryHostingReadinessReason(ctx, later.Draft); ready || err != nil || reason != setupDNSReasonPublisherMissing {
		t.Fatalf("no committed connection: %v %q %v", ready, reason, err)
	}
}

func TestSetupDNSCheckCarriesItsReasonAndNames(t *testing.T) {
	draft := secondaryHostingDraft()
	check := setupDNSCheck(draft, false, setupDNSReasonBackupNotServing, nil)
	if check.State != "action_required" || check.Reason != setupDNSReasonBackupNotServing || check.Vars["ns1"] != draft.NS1 || check.Vars["ns2"] != draft.NS2 || check.Vars["primary"] != "frankfurt.example.test" {
		t.Fatalf("action check: %+v", check)
	}
	check = setupDNSCheck(draft, false, setupDNSReasonPublisherUnconfirmed, nil)
	if check.State != "unknown" || check.Reason != setupDNSReasonPublisherUnconfirmed {
		t.Fatalf("an unproven connection is not an owner action: %+v", check)
	}
	if check := setupDNSCheck(draft, true, setupDNSReasonBackupNotServing, nil); check.State != "ready" || check.Reason != "" {
		t.Fatalf("ready: %+v", check)
	}
	if check := setupDNSCheck(draft, false, setupDNSReasonBackupNotServing, errors.New("read")); check.State != "unknown" || check.Reason != "" {
		t.Fatalf("could not check: %+v", check)
	}
}

func TestServerSetupVerificationWaitBecomesOverdueWithItsOpenChecks(t *testing.T) {
	start := time.Date(2026, 10, 31, 20, 0, 0, 0, time.UTC)
	execution := &serverSetupExecution{Checks: []serverSetupCheck{
		{ID: "dns", State: "action_required", Code: "dns_required", Reason: setupDNSReasonPublisherMissing},
		{ID: "mail_identity", State: "action_required", Advisory: true},
	}}
	serverSetupVerificationWait(execution, start)
	if execution.VerificationSince == "" || execution.Error != nil {
		t.Fatalf("first wait: %+v", execution)
	}
	serverSetupVerificationWait(execution, start.Add(29*time.Minute))
	if execution.Error != nil {
		t.Fatal("overdue before its bound")
	}
	serverSetupVerificationWait(execution, start.Add(31*time.Minute))
	if execution.Error == nil || execution.Error.Code != serverSetupVerificationOverdueCode ||
		!strings.Contains(execution.Error.Message, "dns ("+setupDNSReasonPublisherMissing+")") || strings.Contains(execution.Error.Message, "mail_identity") {
		t.Fatalf("overdue: %+v", execution.Error)
	}
}

// --- E6: a domain on the joined server gets its mail records on the main DNS server ---

func remoteRecordSet(t *testing.T, p *Panel, domain string) map[string]string {
	t.Helper()
	records, err := p.remoteDomainDNSRecords(context.Background(), domain)
	if err != nil {
		t.Fatal(err)
	}
	set := map[string]string{}
	for _, record := range records {
		value := record.Content
		if record.Type == "TXT" {
			value = remoteDNSTXTValue(record)
		}
		key := record.Name + " " + record.Type
		if record.Type == "TXT" {
			key += " " + remoteDNSMailTXTGroup(record)
		}
		set[key] = value
	}
	return set
}

func TestRemoteDomainGetsItsMailRecordsOnTheMainDNSServer(t *testing.T) {
	f := newRemoteDNSHTTPFixture(t)
	f.connect(t)
	t.Setenv("CELIKPANEL_SERVER_IP", "192.0.2.44")
	t.Setenv("CELIKPANEL_SERVER_IPV6", "")
	ctx := context.Background()
	id := f.domain(t, "custb.test", nil)
	if err := f.origin.ensureRemoteDomainDNS(ctx, "custb.test"); err != nil {
		t.Fatal(err)
	}
	// No mail on this server and no mailbox: nothing is added.
	f.origin.publishRemoteDomainMailOnCreate(ctx, id, "custb.test")
	if _, ok := remoteRecordSet(t, f.origin, "custb.test")["custb.test MX"]; ok {
		t.Fatal("mail records published for a domain without mail")
	}
	if _, err := f.origin.db.GetDB().Exec(`INSERT INTO email_accounts (domain_id, address, password_hash, quota_mb) VALUES (?, 'carol@custb.test', 'managed-by-agent', 100)`, id); err != nil {
		t.Fatal(err)
	}
	f.origin.publishRemoteDomainMailOnCreate(ctx, id, "custb.test")
	set := remoteRecordSet(t, f.origin, "custb.test")
	for key, want := range map[string]string{
		"custb.test MX":                  "mail.custb.test",
		"mail.custb.test A":              "192.0.2.44",
		"custb.test TXT v=spf1":          spfRecommended(),
		"_dmarc.custb.test TXT v=dmarc1": dmarcRecommended("custb.test", ""),
	} {
		if got := strings.TrimSuffix(set[key], "."); got != want {
			t.Fatalf("%s = %q, want %q (all: %v)", key, got, want, set)
		}
	}
	// The automatic DKIM work adds the DKIM record there too.
	item := dkimAutomaticItem{record: dkimAutomaticRecord{DomainID: id, Domain: "custb.test", Trigger: dkimTriggerFirstMailbox, State: dkimAutoPending}, pub: "PUBKEY"}
	f.origin.automaticDKIMPublish(ctx, item)
	set = remoteRecordSet(t, f.origin, "custb.test")
	if got := set["celik._domainkey.custb.test TXT v=dkim1"]; got != dkimRecommended("PUBKEY") {
		t.Fatalf("DKIM on the main DNS server = %q (all: %v)", got, set)
	}
	// Served by the main DNS server (the receiver's zone).
	var served int
	if err := f.receiver.db.GetDB().QueryRow(`SELECT COUNT(*) FROM pdns_records r JOIN pdns_domains d ON d.id=r.domain_id WHERE d.name='custb.test' AND r.type IN ('MX','TXT')`).Scan(&served); err != nil || served < 4 {
		t.Fatalf("the main DNS server serves %d mail records: %v", served, err)
	}
	// Removing the domain removes its records with the zone.
	if err := f.origin.syncRemoteDomainDNS(ctx, "custb.test", true); err != nil {
		t.Fatal(err)
	}
	if err := f.receiver.db.GetDB().QueryRow(`SELECT COUNT(*) FROM pdns_records r JOIN pdns_domains d ON d.id=r.domain_id WHERE d.name='custb.test'`).Scan(&served); err != nil || served != 0 {
		t.Fatalf("records left after the domain was removed: %d %v", served, err)
	}
}

func TestRemoteDomainMailRecordsKeepTheOwnersOwn(t *testing.T) {
	existing := []DNSRecord{
		{Name: "own.test", Type: "MX", Content: "mx.provider.test", Prio: 5},
		{Name: "own.test", Type: "TXT", Content: `"v=spf1 include:provider.test -all"`},
		{Name: "own.test", Type: "TXT", Content: `"google-site-verification=abc"`},
	}
	desired := append(remoteDomainMailRecords("own.test", "192.0.2.44", ""), DNSRecord{Name: "celik._domainkey.own.test", Type: "TXT", Content: dkimRecommended("K")})
	missing, kept := remoteDNSMissingMailRecords(existing, desired)
	for _, record := range missing {
		if record.Type == "MX" || (record.Type == "TXT" && remoteDNSMailTXTGroup(record) == "v=spf1") {
			t.Fatalf("an owner record would be replaced: %+v", record)
		}
	}
	if len(missing) != 3 { // mail A, DMARC, DKIM
		t.Fatalf("missing = %+v", missing)
	}
	if strings.Join(kept, ",") != "own.test MX,own.test TXT" {
		t.Fatalf("kept = %v", kept)
	}
}
