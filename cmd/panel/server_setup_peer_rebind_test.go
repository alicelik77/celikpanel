package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

// Beta-1 VM cell b (P3, then P4): Option A, the second server is the main
// DNS server with PowerDNS; its DNS step rolled back, the owner chose BIND
// there and its setup sent the changed answers. The first server refused
// them (server_setup_peer_names_bound), reopening its plan re-bound the old
// answers at once and a new code was refused (join_bound): no recovery.
// Now the first server's run shows server_setup_peer_join_changed (the owner
// creates a new code here and enters it on the second server), a new code is
// allowed, and the run binds the new answers and continues. After a run
// succeeded the answers are final, said plainly.
func TestServerSetupPeerChangedAnswersRecoverWithANewCode(t *testing.T) {
	previous := probeServerSetupPrimaryCatalogSOA
	t.Cleanup(func() { probeServerSetupPrimaryCatalogSOA = previous })
	probeServerSetupPrimaryCatalogSOA = func(context.Context, string, string) (uint32, error) {
		return 0, errors.New("the main DNS server is not ready")
	}
	first := joinFirstDraft()
	first.MainDNSServer = "peer"
	pair := newPeerPair(t, first)
	// Both panels run in this process: each acts with its own address.
	firstAdvance := func() (bool, error) {
		t.Setenv("CELIKPANEL_SERVER_IP", "203.0.113.10")
		return pair.first.panel.advanceServerSetupExecution(pair.plan, pair.execution)
	}
	secondReview := func(change func(*serverSetupDraft)) (serverSetupState, serverSetupPlan) {
		t.Setenv("CELIKPANEL_SERVER_IP", "198.51.100.20")
		return pair.secondReview(t, change)
	}
	if answer := pair.connect(t, issueJoinCode(t, pair.first).Code); answer.State != "ok" {
		t.Fatalf("join: %+v", answer)
	}
	_, plan := secondReview(func(d *serverSetupDraft) { d.DNSEngine = "pdns" })
	step := serverSetupExecutionStep{serverSetupPlanStep: plan.Steps[0], RequestID: strings.Repeat("5", 32), OwnerID: strings.Repeat("6", 32)}
	if done, err := pair.second.panel.runServerSetupPeerNames(context.Background(), plan, step); !done || err != nil {
		t.Fatalf("names: %v %v", done, err)
	}
	joinIndex := pair.advanceToJoin(t)
	bound, err := pair.first.panel.readServerSetupPeerBinding(context.Background(), pair.plan, pair.execution.ID)
	if err != nil || bound == nil || bound.PeerDNSEngine != "pdns" || pair.execution.Steps[joinIndex].Status != "succeeded" {
		t.Fatalf("first binding: %+v %v %+v", bound, err, pair.execution.Error)
	}
	// The first server waits for the main DNS server before installing.
	for pass := 0; pass < 3 && pair.execution.Phase != "primary_dns"; pass++ {
		if _, err := firstAdvance(); err != nil {
			t.Fatal(err)
		}
	}
	if pair.execution.Status != "waiting" || pair.execution.Phase != "primary_dns" {
		t.Fatalf("first server before the change: %s %s %+v", pair.execution.Status, pair.execution.Phase, pair.execution.Error)
	}
	// The main's DNS step rolled back; the owner chose BIND there.
	_, changed := secondReview(func(d *serverSetupDraft) { d.DNSEngine = "bind" })
	step.serverSetupPlanStep = changed.Steps[0]
	_, err = pair.second.panel.runServerSetupPeerNames(context.Background(), changed, step)
	var failure *serverSetupChildFailure
	if !errors.As(err, &failure) || failure.Code != "server_setup_peer_names_changed" || !strings.Contains(failure.Message, "new join code on the first server") {
		t.Fatalf("changed answers on the second server: %v", err)
	}
	// The first server says what changed on its setup link (the DNS step
	// that is waiting keeps its own phase and reason, D-024); asking again
	// changes nothing and the run does not go on with the old answers.
	for pass := 0; pass < 2; pass++ {
		if _, err := firstAdvance(); err != nil {
			t.Fatal(err)
		}
		if pair.execution.Status != "waiting" || pair.execution.Phase != "primary_dns" || pair.execution.Error == nil || pair.execution.Error.Code != "server_setup_primary_dns_required" {
			t.Fatalf("pass %d: the waiting step's state was replaced: %s %s %+v", pass, pair.execution.Status, pair.execution.Phase, pair.execution.Error)
		}
		view, err := pair.first.panel.serverSetupPeerState(context.Background())
		if err != nil || !view.Changed {
			t.Fatalf("pass %d: the link view does not say the change: %+v %v", pass, view, err)
		}
		if again, _ := pair.first.panel.readServerSetupPeerBinding(context.Background(), pair.plan, pair.execution.ID); again == nil || again.PeerDNSEngine != "pdns" {
			t.Fatalf("pass %d: polling changed the binding: %+v", pass, again)
		}
	}
	// A new code is allowed in this state; the second server's failed run
	// lets it join again.
	code := issueJoinCode(t, pair.first)
	if _, err := pair.second.database.GetDB().Exec(`UPDATE server_setup_executions SET status='failed'`); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CELIKPANEL_SERVER_IP", "198.51.100.20")
	if answer := pair.connect(t, code.Code); answer.State != "ok" {
		t.Fatalf("join again: %+v", answer)
	}
	// A crash after the run marked its join step running and before the
	// stale binding was withdrawn: the next pass finishes the same change.
	pair.execution.Steps[joinIndex].Status = "running"
	crashed, _ := json.Marshal(pair.execution)
	if _, err := pair.first.database.GetDB().Exec(`UPDATE server_setup_executions SET execution_json=? WHERE id=?`, string(crashed), pair.execution.ID); err != nil {
		t.Fatal(err)
	}
	if still, _ := pair.first.panel.readServerSetupPeerBinding(context.Background(), pair.plan, pair.execution.ID); still == nil {
		t.Fatal("the crash state needs the stale binding")
	}
	// Before the new answers arrive the run waits for them; nothing is bound.
	if _, err := firstAdvance(); err != nil {
		t.Fatal(err)
	}
	if pair.execution.Error == nil || pair.execution.Error.Code != "server_setup_peer_names_waiting" {
		t.Fatalf("after the new join: %+v", pair.execution.Error)
	}
	if again, _ := pair.first.panel.readServerSetupPeerBinding(context.Background(), pair.plan, pair.execution.ID); again != nil {
		t.Fatalf("the stale binding stayed: %+v", again)
	}
	_, changed = secondReview(func(d *serverSetupDraft) { d.DNSEngine = "bind" })
	step.serverSetupPlanStep, step.RequestID = changed.Steps[0], strings.Repeat("7", 32)
	if done, err := pair.second.panel.runServerSetupPeerNames(context.Background(), changed, step); !done || err != nil {
		t.Fatalf("new answers: %v %v", done, err)
	}
	if _, err := firstAdvance(); err != nil {
		t.Fatal(err)
	}
	rebound, err := pair.first.panel.readServerSetupPeerBinding(context.Background(), pair.plan, pair.execution.ID)
	if err != nil || rebound == nil || rebound.PeerDNSEngine != "bind" || rebound.JoinRequestID == bound.JoinRequestID || pair.execution.Steps[joinIndex].Status != "succeeded" {
		t.Fatalf("setup continues with the new answers: %+v %v %+v", rebound, err, pair.execution.Error)
	}
	for pass := 0; pass < 3 && pair.execution.Phase != "primary_dns"; pass++ {
		if _, err := firstAdvance(); err != nil {
			t.Fatal(err)
		}
	}
	if pair.execution.Phase != "primary_dns" {
		t.Fatalf("the run went on to its DNS step: %s %+v", pair.execution.Phase, pair.execution.Error)
	}
	// Unchanged answers again are fine and change nothing.
	if done, err := pair.second.panel.runServerSetupPeerNames(context.Background(), changed, step); !done || err != nil {
		t.Fatalf("same answers again: %v %v", done, err)
	}
	// After the run succeeded the answers are final; no new code either.
	pair.execution.Status = "succeeded"
	finished, _ := json.Marshal(pair.execution)
	if _, err := pair.first.database.GetDB().Exec(`UPDATE server_setup_executions SET status='succeeded',execution_json=? WHERE id=?`, string(finished), pair.execution.ID); err != nil {
		t.Fatal(err)
	}
	_, other := secondReview(func(d *serverSetupDraft) { d.DNSEngine = "pdns" })
	step.serverSetupPlanStep = other.Steps[0]
	if _, err := pair.second.panel.runServerSetupPeerNames(context.Background(), other, step); !errors.As(err, &failure) || failure.Code != "server_setup_peer_names_final" || !strings.Contains(failure.Message, "not available in this release") {
		t.Fatalf("answers after a finished setup: %v", err)
	}
	w := httptest.NewRecorder()
	pair.first.panel.handleServerSetupJoinCodes(w, serviceOperationAdminRequest(t, http.MethodPost, serverSetupJoinCodesPath, "{}", pair.first.userID))
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "server_setup_join_finished") {
		t.Fatalf("code after a finished setup: %d %s", w.Code, w.Body.String())
	}
}

// The same change after the run already used the earlier answers (a step
// after the join ran): the second server gets the earlier refusal, the join
// is left as it is, the first server's run keeps its own state and a new
// code is still refused (reopening the plan is the path).
func TestServerSetupPeerChangedAnswersAfterUseKeepTheEarlierRefusal(t *testing.T) {
	pair := newPeerPair(t, joinFirstDraft())
	if answer := pair.connect(t, issueJoinCode(t, pair.first).Code); answer.State != "ok" {
		t.Fatalf("join: %+v", answer)
	}
	_, plan := pair.secondReview(t, nil)
	step := serverSetupExecutionStep{serverSetupPlanStep: plan.Steps[0], RequestID: strings.Repeat("5", 32), OwnerID: strings.Repeat("6", 32)}
	if done, err := pair.second.panel.runServerSetupPeerNames(context.Background(), plan, step); !done || err != nil {
		t.Fatalf("names: %v %v", done, err)
	}
	joinIndex := pair.advanceToJoin(t)
	// A step after the join already ran with the binding.
	pair.execution.Steps[joinIndex+1].Status = "succeeded"
	used, _ := json.Marshal(pair.execution)
	if _, err := pair.first.database.GetDB().Exec(`UPDATE server_setup_executions SET execution_json=? WHERE id=?`, string(used), pair.execution.ID); err != nil {
		t.Fatal(err)
	}
	before := *pair.execution
	_, changed := pair.secondReview(t, func(d *serverSetupDraft) { d.PanelDomain = "gamma.example.test" })
	step.serverSetupPlanStep = changed.Steps[0]
	_, err := pair.second.panel.runServerSetupPeerNames(context.Background(), changed, step)
	var failure *serverSetupChildFailure
	if !errors.As(err, &failure) || failure.Code != "server_setup_peer_names_bound" {
		t.Fatalf("changed answers after use: %v", err)
	}
	var join serverSetupJoinRecord
	if found, _ := readServerSetupJSONSetting(context.Background(), pair.first.database.GetDB(), serverSetupJoinKey, &join); !found || join.NamesChanged {
		t.Fatalf("the join was changed: %+v", join)
	}
	if _, err := pair.first.panel.advanceServerSetupExecution(pair.plan, pair.execution); err != nil {
		t.Fatal(err)
	}
	if pair.execution.Error != nil && (pair.execution.Error.Code == "server_setup_peer_join_changed" || pair.execution.Error.Code == "server_setup_peer_join_reopen_required") {
		t.Fatalf("the run's state was replaced: %+v (before %+v)", pair.execution.Error, before.Error)
	}
	w := httptest.NewRecorder()
	pair.first.panel.handleServerSetupJoinCodes(w, serviceOperationAdminRequest(t, http.MethodPost, serverSetupJoinCodesPath, "{}", pair.first.userID))
	if w.Code != http.StatusConflict || !(strings.Contains(w.Body.String(), "server_setup_join_bound") || strings.Contains(w.Body.String(), "server_setup_join_not_waiting")) {
		t.Fatalf("code with a used binding: %d %s", w.Code, w.Body.String())
	}
}

// Beta-1 VM cell a: extending the first server after its pair succeeded
// added a new join step. The extension's plan reuses the succeeded binding:
// no peer_join, the bound second server's address in its answers, no
// blocker for a missing peer.
func TestServerSetupExtensionAfterASucceededPairReusesItsBinding(t *testing.T) {
	f, plan, execution := joinFirstFixture(t, joinFirstDraft())
	binding := serverSetupPeerBinding{ExecutionID: execution.ID, PlanID: plan.ID, JoinRequestID: strings.Repeat("3", 32), PeerIPv4: "198.51.100.20",
		PeerPanelDomain: "beta.example.test", PeerAddress: "https://198.51.100.20:2083", PeerPin: strings.Repeat("ab", 32), PeerDNSEngine: "bind",
		JoinDigest: serverSetupJoinDocumentDigest(serverSetupJoinDocumentFor(plan.Draft))}
	if err := writeServerSetupJSONSetting(context.Background(), f.database.GetDB(), serverSetupPeerBindingKey(execution.ID), binding); err != nil {
		t.Fatal(err)
	}
	if _, err := f.database.GetDB().Exec(`UPDATE server_setup_executions SET status='succeeded' WHERE id=?`, execution.ID); err != nil {
		t.Fatal(err)
	}
	state, err := f.panel.loadServerSetup(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	extended, err := f.panel.buildServerSetupPlan(context.Background(), state, serviceOperationActor{UserID: f.userID})
	if err != nil {
		t.Fatal(err)
	}
	if serverSetupPlanHasStep(extended, "peer_join") || extended.Draft.PeerIP != "198.51.100.20" || extended.Draft.PeerNS != "ns2.example.test" {
		t.Fatalf("extension plan: %+v %+v", extended.Draft, extended.Steps)
	}
	for _, blocker := range extended.Blockers {
		if strings.Contains(blocker, "dns_identity") || blocker == "server_setup_extension_needs_second_server" {
			t.Fatalf("the reused binding completes the DNS identity: %v", extended.Blockers)
		}
	}
	// An extension that would change what the second server received (its
	// shared answers, names or mail) is a verified prerequisite at review.
	for name, change := range map[string]func(*serverSetupDraft){
		"panel name": func(d *serverSetupDraft) { d.PanelDomain = "admin.example.test" },
		"mail":       func(d *serverSetupDraft) { d.Purpose, d.MailHostname = "web_mail", "mail.example.test" },
	} {
		edited := state
		change(&edited.Draft)
		blocked, err := f.panel.buildServerSetupPlan(context.Background(), edited, serviceOperationActor{UserID: f.userID})
		if err != nil || blocked.CanStart || !slices.Contains(blocked.Blockers, "server_setup_extension_needs_second_server") {
			t.Fatalf("%s: %v %v", name, err, blocked.Blockers)
		}
	}
	// Without a succeeded binding the join step stays.
	if _, err := f.database.GetDB().Exec(`UPDATE server_setup_executions SET status='running' WHERE id=?`, execution.ID); err != nil {
		t.Fatal(err)
	}
	again, err := f.panel.buildServerSetupPlan(context.Background(), state, serviceOperationActor{UserID: f.userID})
	if err != nil || !serverSetupPlanHasStep(again, "peer_join") {
		t.Fatalf("join step without a succeeded pair: %v %+v", err, again.Steps)
	}
}
