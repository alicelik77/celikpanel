package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// startSecond saves the second server's reviewed plan and starts its
// execution (running, as the runner leaves it between passes).
func (pair *peerPair) startSecond(t *testing.T, plan serverSetupPlan) *serverSetupExecution {
	t.Helper()
	if !plan.CanStart {
		t.Fatalf("second plan blocked: %v", plan.Blockers)
	}
	db := pair.second.database.GetDB()
	raw, _ := json.Marshal(plan)
	if _, err := db.Exec(`INSERT INTO server_setup_plans(id,revision,plan_json,requested_by,created_at) VALUES(?,?,?,?,?)`, plan.ID, plan.Revision, string(raw), pair.second.userID, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	execution := &serverSetupExecution{ID: strings.Repeat("8", 32), RequestID: strings.Repeat("8", 32), PlanID: plan.ID, Status: "running"}
	for _, step := range plan.Steps {
		execution.Steps = append(execution.Steps, serverSetupExecutionStep{serverSetupPlanStep: step, Status: "pending", RequestID: serverSetupID(execution.ID, step.ID, "request"), OwnerID: serverSetupID(execution.ID, step.ID, "owner")})
	}
	raw, _ = json.Marshal(execution)
	if _, err := db.Exec(`INSERT INTO server_setup_executions(id,request_id,plan_id,status,execution_json,created_at,updated_at) VALUES(?,?,?,?,?,'now','now')`, execution.ID, execution.RequestID, plan.ID, execution.Status, string(raw)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE server_setup_state SET status='running' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	return execution
}

// serveLinkOf serves a panel's link endpoints over TLS with the given key
// and returns the listener's address.
func serveLinkOf(t *testing.T, f serviceOperationTestFixture, served tls.Certificate) string {
	t.Helper()
	server := httptest.NewUnstartedServer(f.panel.requireServerSetupLinkMachine(http.NotFoundHandler()))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{served}}
	server.StartTLS()
	t.Cleanup(server.Close)
	return server.Listener.Addr().String()
}

// completeWithChecks marks every installing step done and runs the final
// verification: the probe stands in for the host checks, the other
// server's DKIM record is checked for real.
func completeWithChecks(t *testing.T, f serviceOperationTestFixture, plan serverSetupPlan, execution *serverSetupExecution) {
	t.Helper()
	f.panel.serverSetupProbe = func(ctx context.Context, d serverSetupDraft) ([]serverSetupCheck, error) {
		checks := []serverSetupCheck{setupCheck("dns", true, nil)}
		if d.PeerMail != nil && d.InfrastructureDNS != nil && !serverSetupHasMail(d) {
			present, err := f.panel.serverSetupPeerDKIMPresent(ctx, d)
			checks = append(checks, setupCheck("peer_mail_dkim", present, err))
		}
		return checks, nil
	}
	for index := range execution.Steps {
		if execution.Steps[index].Kind != "verify" {
			execution.Steps[index].Status = "succeeded"
		}
	}
	if _, err := f.panel.advanceServerSetupExecution(plan, execution); err != nil {
		t.Fatal(err)
	}
}

// MUST-2 (alpha.114 review): Option A with the main domain's mail on the
// first server. The second server is the main DNS server and has no binding
// step; the first server's DKIM record is accepted over the link, written
// into the main's zone, and both setups complete.
func TestServerSetupPeerOptionAWithMailOnTheFirstServer(t *testing.T) {
	first := joinFirstDraft()
	first.MainDNSServer, first.Purpose, first.MailHostname = "peer", "web_mail", "mail.example.test"
	pair := newPeerPair(t, first)
	if !serverSetupHasMail(pair.plan.Draft) || pair.plan.Draft.DNSRole != "secondary" || !serverSetupPlanHasStep(pair.plan, "mail_dns") {
		t.Fatalf("the first server is the backup with the main domain's mail: %+v %+v", pair.plan.Draft, pair.plan.Steps)
	}
	answer := pair.connect(t, issueJoinCode(t, pair.first).Code)
	if answer.State != "ok" || answer.Setup == nil || answer.Setup.Draft.PeerMail == nil || answer.Setup.Draft.PeerMail.Hostname != "mail.example.test" {
		t.Fatalf("the main takes the first server's mail choice: %+v", answer)
	}
	_, plan := pair.secondReview(t, nil)
	second := pair.startSecond(t, plan)
	if done, err := pair.second.panel.runServerSetupPeerNames(context.Background(), plan, second.Steps[0]); !done || err != nil {
		t.Fatalf("names: %v %v", done, err)
	}
	joinIndex := pair.advanceToJoin(t)
	if pair.execution.Steps[joinIndex].Status != "succeeded" {
		t.Fatalf("the first server binds the main: %+v", pair.execution.Error)
	}
	effective, err := pair.first.panel.effectiveServerSetupPlan(context.Background(), pair.plan, pair.execution.ID)
	if err != nil || !effective.peerBound {
		t.Fatalf("bound: %v", err)
	}
	// The first server sends its DKIM record over the link to the main.
	listener := serveLinkOf(t, pair.second, pair.served)
	joinTestDial(pair.first.panel, func() string { return listener })
	var mailStep serverSetupExecutionStep
	for _, step := range pair.execution.Steps {
		if step.Kind == "mail_dns" {
			mailStep = step
		}
	}
	const value = "v=DKIM1; k=rsa; p=QUJD"
	send := func() error {
		return pair.first.panel.sendServerSetupPeerDKIM(context.Background(), effective, pair.execution.ID, mailStep, value)
	}
	waitCode := func(err error) string {
		var wait *serverSetupPeerWait
		if errors.As(err, &wait) {
			return wait.Code
		}
		return ""
	}
	if err := send(); waitCode(err) != "server_setup_peer_dkim_zone_waiting" {
		t.Fatalf("before the main's zone exists: %v", err)
	}
	db := pair.second.database.GetDB()
	if _, err := db.Exec(`INSERT INTO pdns_domains(name,type) VALUES('example.test','MASTER')`); err != nil {
		t.Fatal(err)
	}
	if err := send(); waitCode(err) != "server_setup_peer_dkim_publishing" {
		t.Fatalf("accepted, publishing: %v", err)
	}
	// The zone's own operation published it (the DNS engine is not part of
	// this test).
	if _, err := db.Exec(`INSERT INTO dns_zone_sync_state(zone_name,source_domain_id,desired_generation,applied_generation,desired_action,desired_zone_type,status) SELECT name,id,1,1,'sync','MASTER','applied' FROM pdns_domains WHERE name='example.test'
		ON CONFLICT(zone_name) DO UPDATE SET status='applied',applied_generation=desired_generation`); err != nil {
		t.Fatal(err)
	}
	if err := send(); err != nil {
		t.Fatalf("the same record again, published: %v", err)
	}
	var rows int
	db.QueryRow(`SELECT COUNT(*) FROM pdns_records WHERE name=? AND type='TXT'`, dkimSelector+"._domainkey.example.test").Scan(&rows)
	if rows != 1 {
		t.Fatalf("one DKIM record in the main's zone: %d", rows)
	}
	// The main's completion waits for the record; it is there.
	completeWithChecks(t, pair.second, plan, second)
	if second.Status != "succeeded" || second.Phase != "complete" {
		t.Fatalf("the main completes once the first server's DKIM record is in its zone: %s %s %+v", second.Status, second.Phase, second.Checks)
	}
	completeWithChecks(t, pair.first, pair.plan, pair.execution)
	if pair.execution.Status != "succeeded" || pair.execution.Phase != "complete" {
		t.Fatalf("the first server completes: %s %s %+v", pair.execution.Status, pair.execution.Phase, pair.execution.Error)
	}
	// The link ended with the main's setup: nothing more is accepted.
	var failure *serverSetupChildFailure
	if err := send(); !errors.As(err, &failure) {
		t.Fatalf("after completion: %v", err)
	}
}

// peerOutcomeCode is the code of a wait or a verified failure.
func peerOutcomeCode(err error) (string, string) {
	var wait *serverSetupPeerWait
	var failure *serverSetupChildFailure
	switch {
	case errors.As(err, &wait):
		return "wait", wait.Code
	case errors.As(err, &failure):
		return "failed", failure.Code
	case err == nil:
		return "done", ""
	}
	return "error", err.Error()
}

// SHOULD-1 (alpha.114 review): a DKIM record sent before the main DNS
// server bound this server, while its setup does not run, or before its
// zone exists is a wait, sent again by itself; only the owner's own record
// in the way is a verified failure.
func TestServerSetupPeerDKIMBeforeTheMainIsReadyWaits(t *testing.T) {
	pair := newPeerPair(t, joinFirstDraft())
	if answer := pair.connect(t, issueJoinCode(t, pair.first).Code); answer.State != "ok" {
		t.Fatalf("connect: %+v", answer)
	}
	plan := serverSetupPlan{Draft: serverSetupDraft{MainDomain: "example.test"}}
	step := serverSetupExecutionStep{RequestID: strings.Repeat("5", 32)}
	send := func() (string, string) {
		return peerOutcomeCode(pair.second.panel.sendServerSetupPeerDKIM(context.Background(), plan, "", step, "v=DKIM1; k=rsa; p=QUJD"))
	}
	if kind, code := send(); kind != "wait" || code != "server_setup_peer_dkim_not_bound" {
		t.Fatalf("before the binding: %s %s", kind, code)
	}
	link, _ := pair.second.panel.readServerSetupLink(context.Background())
	names := serverSetupPeerNames{Protocol: serverSetupLinkProtocol, RequestID: strings.Repeat("4", 32), ShortName: "beta", PanelDomain: "beta.example.test", MailHostname: "mail.example.test", DNSEngine: "bind",
		Mail: &serverSetupPeerMail{Hostname: "mail.example.test", MX: "now"}, Address: "https://198.51.100.20:" + itoaTest(panelPort()), Pin: strings.Repeat("cd", 32)}
	if w := joinMachineRequest(t, pair.first, serverSetupPeerNamesPath, link.bearer(), names); w.Code != http.StatusOK {
		t.Fatalf("names: %d %s", w.Code, w.Body.String())
	}
	pair.advanceToJoin(t)
	if kind, code := send(); kind != "wait" || code != "server_setup_peer_dkim_zone_waiting" {
		t.Fatalf("before the zone: %s %s", kind, code)
	}
	db := pair.first.database.GetDB()
	if _, err := db.Exec(`INSERT INTO pdns_domains(name,type) VALUES('example.test','MASTER')`); err != nil {
		t.Fatal(err)
	}
	if kind, code := send(); kind != "wait" || code != "server_setup_peer_dkim_publishing" {
		t.Fatalf("saved: %s %s", kind, code)
	}
	if _, err := db.Exec(`UPDATE pdns_records SET content='"v=DKIM1; p=OWNER"' WHERE name=? AND type='TXT'`, dkimSelector+"._domainkey.example.test"); err != nil {
		t.Fatal(err)
	}
	if kind, code := send(); kind != "failed" || code != "server_setup_peer_dkim_conflict" {
		t.Fatalf("the owner's record: %s %s", kind, code)
	}
	if _, err := db.Exec(`UPDATE server_setup_executions SET status='failed',execution_json=replace(replace(execution_json,'"status":"running"','"status":"failed"'),'"status":"waiting"','"status":"failed"') WHERE id=?`, pair.execution.ID); err != nil {
		t.Fatal(err)
	}
	if kind, code := send(); kind != "wait" || code != "server_setup_peer_dkim_not_running" {
		t.Fatalf("the main's setup stopped: %s %s", kind, code)
	}
}
