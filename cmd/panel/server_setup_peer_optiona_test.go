package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/secrets"
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
	// Beta-1 third re-run N3/N5: mail on both servers is refused on the
	// main's review; the join's shared answer says the first receives it.
	before, _ := pair.second.panel.loadServerSetup(context.Background())
	if _, both := pair.secondReview(t, func(d *serverSetupDraft) { d.Purpose, d.MailHostname = "web_mail", "mail2.example.test" }); both.CanStart || !strings.Contains(strings.Join(both.Blockers, ","), "server_setup_peer_mail_both") {
		t.Fatalf("mail on both servers: %v", both.Blockers)
	}
	_, plan := pair.secondReview(t, func(d *serverSetupDraft) { d.MailHostname = before.Draft.MailHostname })
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
	// The main finished; its link is live until the backup reports: the
	// backup's DKIM is still taken (N3).
	if err := send(); err != nil {
		t.Fatalf("the backup's DKIM after the main finished: %v", err)
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
	if _, err := db.Exec(`UPDATE pdns_records SET content='"v=DKIM1; k=rsa; p=QUJD"' WHERE name=? AND type='TXT'`, dkimSelector+"._domainkey.example.test"); err != nil {
		t.Fatal(err)
	}
	// Beta-1 third re-run N3: the main finished while its link is live (it
	// waits for the backup's report): the backup's DKIM is still taken.
	if _, err := db.Exec(`UPDATE server_setup_executions SET status='succeeded' WHERE id=?`, pair.execution.ID); err != nil {
		t.Fatal(err)
	}
	if kind, code := send(); kind == "failed" || code == "server_setup_peer_dkim_not_running" {
		t.Fatalf("the finished main refused the backup's DKIM over a live link: %s %s", kind, code)
	}
	if err := pair.first.panel.revokeServerSetupLinkSide(context.Background(), serverSetupLinkRoleFirst); err != nil {
		t.Fatal(err)
	}
	if kind, code := send(); kind != "failed" || code != "server_setup_peer_link_revoked" {
		t.Fatalf("an ended link: %s %s", kind, code)
	}
}

// SHOULD-4 (alpha.114 review): a server that is the second of one pair and
// the first of another keeps the two credential namespaces apart: the names
// endpoint takes only credentials it issued with a join, the DKIM endpoint
// only those of its own setup's role, and an ended join never falls
// through to the link record.
func TestServerSetupPeerCredentialsDoNotCrossRoles(t *testing.T) {
	pair := newPeerPair(t, joinFirstDraft())
	if answer := pair.connect(t, issueJoinCode(t, pair.first).Code); answer.State != "ok" {
		t.Fatalf("connect: %+v", answer)
	}
	// B (the second server here) is also the first server of another pair:
	// a third server C joined it.
	b := pair.second
	link, _ := b.panel.readServerSetupLink(context.Background())
	reverse := link.LinkID + "." + serverSetupReverseCredential(link.LinkCredential)
	issued := strings.Repeat("c", 64)
	asFirst := serverSetupJoinRecord{RequestID: strings.Repeat("1", 32), PlanID: strings.Repeat("2", 32), LinkID: strings.Repeat("3", 32), LinkCredentialHash: b.panel.serverSetupMAC("link", issued), Peer: serverSetupJoinPeer{IPv4: "198.51.100.30"}}
	if err := writeServerSetupJSONSetting(context.Background(), b.database.GetDB(), serverSetupJoinKey, asFirst); err != nil {
		t.Fatal(err)
	}
	fromC := asFirst.LinkID + "." + issued
	ctx := context.Background()
	id, credential, _ := strings.Cut(reverse, ".")
	cID, cCredential, _ := strings.Cut(fromC, ".")
	if !b.panel.serverSetupLinkAuthorized(ctx, serverSetupLinkRoleSecond, id, credential) || b.panel.serverSetupLinkAuthorized(ctx, serverSetupLinkRoleFirst, id, credential) ||
		!b.panel.serverSetupLinkAuthorized(ctx, serverSetupLinkRoleFirst, cID, cCredential) || b.panel.serverSetupLinkAuthorized(ctx, serverSetupLinkRoleSecond, cID, cCredential) {
		t.Fatal("each credential belongs to its own role only")
	}
	// The names endpoint is a first server's: A's credential is refused.
	if w := joinMachineRequest(t, b, serverSetupPeerNamesPath, reverse, serverSetupPeerNames{Protocol: serverSetupLinkProtocol}); w.Code != http.StatusUnauthorized {
		t.Fatalf("names with the link record's credential: %d %s", w.Code, w.Body.String())
	}
	// B's own setup runs as the second server: its DKIM endpoint takes A's
	// credential only, never one B issued to C.
	_, plan := pair.secondReview(t, nil)
	pair.startSecond(t, plan)
	dkim := serverSetupPeerDKIMRequest{Protocol: serverSetupLinkProtocol, RequestID: strings.Repeat("3", 32), Domain: "example.test", Selector: dkimSelector, Value: "v=DKIM1; k=rsa; p=QUJD"}
	if w := joinMachineRequest(t, b, serverSetupPeerDKIMPath, fromC, dkim); w.Code != http.StatusUnauthorized {
		t.Fatalf("DKIM with a join-issued credential on a second server: %d %s", w.Code, w.Body.String())
	}
	if w := joinMachineRequest(t, b, serverSetupPeerDKIMPath, reverse, dkim); w.Code == http.StatusUnauthorized {
		t.Fatalf("DKIM with the first server's credential: %d %s", w.Code, w.Body.String())
	}
	// B's join as first server ended: nothing falls through to its link.
	asFirst.LinkCredentialHash = ""
	if err := writeServerSetupJSONSetting(context.Background(), b.database.GetDB(), serverSetupJoinKey, asFirst); err != nil {
		t.Fatal(err)
	}
	if w := joinMachineRequest(t, b, serverSetupPeerNamesPath, reverse, serverSetupPeerNames{Protocol: serverSetupLinkProtocol}); w.Code != http.StatusUnauthorized {
		t.Fatalf("names after the join ended: %d %s", w.Code, w.Body.String())
	}
}

// Claim-8 (alpha.114 review): a first server's completion is checked on its
// bound plan only. Saved answers that no longer match the running plan, or
// a plan or binding that cannot be read, are not complete: a wait with its
// code, never the unbound draft.
func TestServerSetupPeerCompletionNeverFallsBackToTheUnboundDraft(t *testing.T) {
	pair := newPeerPair(t, joinFirstDraft())
	if answer := pair.connect(t, issueJoinCode(t, pair.first).Code); answer.State != "ok" {
		t.Fatalf("connect: %+v", answer)
	}
	_, plan := pair.secondReview(t, nil)
	if done, err := pair.second.panel.runServerSetupPeerNames(context.Background(), plan, serverSetupExecutionStep{serverSetupPlanStep: plan.Steps[0], RequestID: strings.Repeat("5", 32)}); !done || err != nil {
		t.Fatalf("names: %v %v", done, err)
	}
	pair.advanceToJoin(t)
	db := pair.first.database.GetDB()
	var original string
	db.QueryRow(`SELECT draft_json FROM server_setup_state WHERE id=1`).Scan(&original)
	changed := pair.plan.Draft
	changed.ShortName = "changed"
	raw, _ := json.Marshal(changed)
	if _, err := db.Exec(`UPDATE server_setup_state SET draft_json=? WHERE id=1`, string(raw)); err != nil {
		t.Fatal(err)
	}
	completeWithChecks(t, pair.first, pair.plan, pair.execution)
	state, _ := pair.first.panel.loadServerSetup(context.Background())
	if pair.execution.Status != "waiting" || pair.execution.Error == nil || pair.execution.Error.Code != "server_setup_completion_plan_changed" || state.Status == "ready" {
		t.Fatalf("changed answers are not complete: %s %+v %s", pair.execution.Status, pair.execution.Error, state.Status)
	}
	if _, err := db.Exec(`UPDATE server_setup_state SET draft_json=? WHERE id=1`, original); err != nil {
		t.Fatal(err)
	}
	state, _ = pair.first.panel.loadServerSetup(context.Background())
	var wait *serverSetupPeerWait
	if _, err := pair.first.panel.serverSetupCompletionDraft(context.Background(), state, strings.Repeat("9", 32)); !errors.As(err, &wait) || wait.Code != "server_setup_completion_unknown" {
		t.Fatalf("another execution: %v", err)
	}
	var binding string
	key := serverSetupPeerBindingKey(pair.execution.ID)
	db.QueryRow(`SELECT value FROM panel_settings WHERE key=?`, key).Scan(&binding)
	if _, err := db.Exec(`UPDATE panel_settings SET value='{"execution_id":"x"}' WHERE key=?`, key); err != nil {
		t.Fatal(err)
	}
	if _, err := pair.first.panel.serverSetupCompletionDraft(context.Background(), state, pair.execution.ID); !errors.As(err, &wait) || wait.Code != "server_setup_completion_unknown" {
		t.Fatalf("an unreadable binding: %v", err)
	}
	if _, err := db.Exec(`UPDATE panel_settings SET value=? WHERE key=?`, binding, key); err != nil {
		t.Fatal(err)
	}
	completeWithChecks(t, pair.first, pair.plan, pair.execution)
	if pair.execution.Status != "succeeded" || pair.execution.Error != nil {
		t.Fatalf("the bound plan completes: %s %+v", pair.execution.Status, pair.execution.Error)
	}
}

// Claim-9 (alpha.114 review): neither database alone yields a bearer, a
// code or an enrollment. The second server keeps its link credential and
// both enrollment codes sealed, the first server the enrollment code the
// main sent with its names (Option A), each under its own key file and
// labelled with the key generation; with another key nothing opens.
func TestServerSetupJoinSecretsAreSealedAtRest(t *testing.T) {
	for _, main := range []string{"first", "second"} {
		t.Run("main is the "+main, func(t *testing.T) {
			draft := joinFirstDraft()
			if main == "second" {
				draft.MainDNSServer = "peer"
			}
			pair := newPeerPair(t, draft)
			boxes := []*secrets.Box{}
			for range 3 {
				box, err := secrets.LoadOrCreate(filepath.Join(t.TempDir(), "secret.key"))
				if err != nil {
					t.Fatal(err)
				}
				boxes = append(boxes, box)
			}
			pair.first.panel.secrets, pair.second.panel.secrets = boxes[0], boxes[1]
			if answer := pair.connect(t, issueJoinCode(t, pair.first).Code); answer.State != "ok" {
				t.Fatalf("connect: %+v", answer)
			}
			_, plan := pair.secondReview(t, nil)
			if done, err := pair.second.panel.runServerSetupPeerNames(context.Background(), plan, serverSetupExecutionStep{serverSetupPlanStep: plan.Steps[0], RequestID: strings.Repeat("5", 32)}); !done || err != nil {
				t.Fatalf("names: %v %v", done, err)
			}
			joinIndex := pair.advanceToJoin(t)
			if pair.execution.Steps[joinIndex].Status != "succeeded" {
				t.Fatalf("the sealed names bind: %+v", pair.execution.Error)
			}
			link, err := pair.second.panel.readServerSetupLink(context.Background())
			if err != nil || link == nil || !remoteDNSValidSecret(link.LinkCredential) {
				t.Fatalf("the link opens with its own key: %v", err)
			}
			plain := []string{link.LinkCredential, serverSetupReverseCredential(link.LinkCredential)}
			if main == "first" {
				if !remoteDNSValidSecret(link.EnrollmentCode) {
					t.Fatal("the enrollment code the first server handed over")
				}
				plain = append(plain, link.EnrollmentCode)
			} else {
				var join serverSetupJoinRecord
				readServerSetupJSONSetting(context.Background(), pair.first.database.GetDB(), serverSetupJoinKey, &join)
				names, err := pair.first.panel.openServerSetupPeerNames(*join.Names)
				if err != nil || !remoteDNSValidSecret(link.IssuedEnrollment) || names.EnrollmentCode != link.IssuedEnrollment || !strings.HasPrefix(join.Names.EnrollmentCode, "k1:enc:v1:") {
					t.Fatalf("the main's enrollment code, sealed on the first server: %v %q", err, join.Names.EnrollmentCode)
				}
				plain = append(plain, link.IssuedEnrollment)
				if _, err := (&Panel{db: pair.first.database, secrets: boxes[2]}).openServerSetupPeerNames(*join.Names); err == nil {
					t.Fatal("the first server's database alone opens no enrollment code")
				}
			}
			for _, f := range []serviceOperationTestFixture{pair.first, pair.second} {
				rows, err := f.database.GetDB().Query(`SELECT key,value FROM panel_settings`)
				if err != nil {
					t.Fatal(err)
				}
				for rows.Next() {
					var key, value string
					rows.Scan(&key, &value)
					for _, secret := range plain {
						if strings.Contains(value, secret) {
							t.Errorf("%s keeps a secret in the clear", key)
						}
					}
				}
				rows.Close()
			}
			var raw string
			pair.second.database.GetDB().QueryRow(`SELECT value FROM panel_settings WHERE key=?`, serverSetupLinkKey).Scan(&raw)
			if !strings.Contains(raw, `"link_credential":"k1:enc:v1:`) {
				t.Fatalf("sealed with its key generation: %s", raw)
			}
			// The second server's database with another key: nothing opens.
			copied := &Panel{db: pair.second.database, secrets: boxes[2]}
			if _, err := copied.readServerSetupLink(context.Background()); err == nil {
				t.Fatal("the second server's database alone opens no link")
			}
		})
	}
}

// N5: the second server's DNS overview names the receiver the join's shared
// answer names, even on a pair installed before the refusal where the second
// also runs mail.
func TestDNSOverviewMailReceiverFollowsTheJoinsSharedAnswer(t *testing.T) {
	in := dnsOverviewInput{Draft: serverSetupDraft{MainDomain: "example.test", Purpose: "web_mail", MailHostname: "mail2.example.test", ShortName: "beta"},
		Peer: dnsOverviewPeer{FromLink: true, Bound: true, ShortName: "alpha", MailHost: "mail.example.test", MainMail: true}}
	receiver, _, ok := dnsOverviewMailInputs(in)
	if !ok || receiver.Server != "peer" || receiver.Host != "mail.example.test" {
		t.Fatalf("receiver: %+v %v", receiver, ok)
	}
	in.Peer.MainMail = false
	if receiver, _, _ := dnsOverviewMailInputs(in); receiver.Server != "this" {
		t.Fatalf("without the shared answer: %+v", receiver)
	}
}

// Review fix (item 3): the refusal of a second receiver applies to a first
// setup only. The link record outlives a finished pair, so an extension of
// that pair is judged by the extension rules, not refused by it.
func TestServerSetupPeerMailBothDoesNotRefuseAnExtension(t *testing.T) {
	first := joinFirstDraft()
	first.MainDNSServer, first.Purpose, first.MailHostname = "peer", "web_mail", "mail.example.test"
	pair := newPeerPair(t, first)
	if answer := pair.connect(t, issueJoinCode(t, pair.first).Code); answer.State != "ok" {
		t.Fatalf("join: %+v", answer)
	}
	mail := func(d *serverSetupDraft) { d.Purpose, d.MailHostname = "web_mail", "mail2.example.test" }
	if _, plan := pair.secondReview(t, mail); !strings.Contains(strings.Join(plan.Blockers, ","), "server_setup_peer_mail_both") {
		t.Fatalf("a first setup: %v", plan.Blockers)
	}
	// The pair finished earlier (an installed pair); this is its extension.
	if _, err := pair.second.database.GetDB().Exec(`UPDATE server_setup_state SET completed_at='2026-10-29T07:00:00Z', status='draft' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	if _, plan := pair.secondReview(t, mail); strings.Contains(strings.Join(plan.Blockers, ","), "server_setup_peer_mail_both") {
		t.Fatalf("an extension of a finished pair was refused: %v", plan.Blockers)
	}
}
