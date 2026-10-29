package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// boundPeerPair: the first server is the main DNS server, the second joined
// it, sent its names and is bound; both runs are at their final check.
func boundPeerPair(t *testing.T) (*peerPair, serverSetupPlan, *serverSetupExecution) {
	t.Helper()
	pair := newPeerPair(t, joinFirstDraft())
	if answer := pair.connect(t, issueJoinCode(t, pair.first).Code); answer.State != "ok" {
		t.Fatalf("connect: %+v", answer)
	}
	_, plan := pair.secondReview(t, nil)
	second := pair.startSecond(t, plan)
	if done, err := pair.second.panel.runServerSetupPeerNames(context.Background(), plan, second.Steps[0]); !done || err != nil {
		t.Fatalf("names: %v %v", done, err)
	}
	if joinIndex := pair.advanceToJoin(t); pair.execution.Steps[joinIndex].Status != "succeeded" {
		t.Fatalf("bind: %+v", pair.execution.Error)
	}
	return pair, plan, second
}

func peerViewOf(t *testing.T, f serviceOperationTestFixture) serverSetupPeerView {
	t.Helper()
	w := httptest.NewRecorder()
	f.panel.handleServerSetupPeer(w, serviceOperationAdminRequest(t, http.MethodGet, serverSetupPeerPath, "", f.userID))
	var view serverSetupPeerView
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &view) != nil {
		t.Fatalf("peer view: %d %s", w.Code, w.Body.String())
	}
	return view
}

func firstJoinRecord(t *testing.T, pair *peerPair) serverSetupJoinRecord {
	t.Helper()
	var join serverSetupJoinRecord
	if found, err := readServerSetupJSONSetting(context.Background(), pair.first.database.GetDB(), serverSetupJoinKey, &join); err != nil || !found {
		t.Fatalf("join record: %v %v", found, err)
	}
	return join
}

// Beta-1 re-run P7: the main DNS server finished first and ended the link
// at its own completion; the backup then failed (its panel certificate) and
// its retry got server_setup_peer_link_revoked forever. The main now keeps
// the link until the backup reports ready over it: the backup's retry
// works, the main's peer view says the second server has not finished, and
// it ends the link once the backup has reported. A report for another join
// is refused; the same report again changes nothing.
func TestServerSetupPeerLinkOutlivesTheMainUntilTheBackupReportsReady(t *testing.T) {
	pair, plan, second := boundPeerPair(t)
	completeWithChecks(t, pair.first, pair.plan, pair.execution)
	if pair.execution.Status != "succeeded" {
		t.Fatalf("the main completes: %s %+v", pair.execution.Status, pair.execution.Error)
	}
	join := firstJoinRecord(t, pair)
	if join.LinkCredentialHash == "" || join.PeerCredential == "" {
		t.Fatal("the main ended the link before the backup finished")
	}
	if found, _ := readServerSetupJSONSetting(context.Background(), pair.first.database.GetDB(), serverSetupJoinCodeKey, &serverSetupJoinCodeRecord{}); found {
		t.Fatal("the join code outlived the main's setup")
	}
	if view := peerViewOf(t, pair.first); view.PeerSetup != "pending" {
		t.Fatalf("the main's peer view: %+v", view)
	}
	// The backup failed after the main completed; its retry goes on over
	// the same link.
	if done, err := pair.second.panel.runServerSetupPeerNames(context.Background(), plan, second.Steps[0]); !done || err != nil {
		t.Fatalf("the backup's retry after the main completed: %v %v", done, err)
	}
	// A report that names another join is not recorded.
	link, _ := pair.second.panel.readServerSetupLink(context.Background())
	other := joinMachineRequest(t, pair.first, serverSetupPeerReadyPath, link.bearer(), serverSetupPeerReadyRequest{Protocol: serverSetupLinkProtocol, JoinRequestID: strings.Repeat("e", 32)})
	if other.Code != http.StatusConflict || !strings.Contains(other.Body.String(), "server_setup_peer_ready_other_join") {
		t.Fatalf("another join's report: %d %s", other.Code, other.Body.String())
	}
	if join := firstJoinRecord(t, pair); join.PeerReadyAt != "" || join.LinkCredentialHash == "" {
		t.Fatal("another join's report was recorded")
	}
	// The backup completes: it reports first, then both are ready.
	completeWithChecks(t, pair.second, plan, second)
	if second.Status != "succeeded" {
		t.Fatalf("the backup completes: %s %+v", second.Status, second.Error)
	}
	join = firstJoinRecord(t, pair)
	if join.PeerReadyAt == "" || join.LinkCredentialHash != "" || join.PeerCredential != "" {
		t.Fatalf("the main did not end the link after the report: %+v", join)
	}
	if view := peerViewOf(t, pair.first); view.PeerSetup != "ready" {
		t.Fatalf("the main's peer view after the report: %+v", view)
	}
	if after, _ := pair.second.panel.readServerSetupLink(context.Background()); after == nil || after.LinkCredential != "" {
		t.Fatal("the backup kept its side of the link")
	}
	// The same report again (a replay with the ended credential, from the
	// joined address) is told the link ended, and records nothing new.
	replay := joinMachineRequest(t, pair.first, serverSetupPeerReadyPath, link.bearer(), serverSetupPeerReadyRequest{Protocol: serverSetupLinkProtocol, JoinRequestID: link.RequestID})
	if replay.Code != http.StatusConflict || !strings.Contains(replay.Body.String(), "server_setup_peer_link_ended") {
		t.Fatalf("a replay after the link ended: %d %s", replay.Code, replay.Body.String())
	}
	if again := firstJoinRecord(t, pair); again.PeerReadyAt != join.PeerReadyAt {
		t.Fatal("a replay changed the record")
	}
}

// The owner may stop waiting for the second server on the main: the link
// ends there at once, and the backup's later completion is not held by it.
func TestServerSetupPeerOwnerStopsWaitingForTheSecondServer(t *testing.T) {
	pair, plan, second := boundPeerPair(t)
	retire := func(f serviceOperationTestFixture, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		f.panel.handleServerSetupPeerRetire(w, serviceOperationAdminRequest(t, http.MethodPost, serverSetupPeerRetirePath, body, f.userID))
		return w
	}
	// Not before the main's own setup is ready.
	if w := retire(pair.first, `{"confirmed":true}`); w.Code != http.StatusConflict {
		t.Fatalf("retire before completion: %d %s", w.Code, w.Body.String())
	}
	completeWithChecks(t, pair.first, pair.plan, pair.execution)
	if w := retire(pair.first, `{}`); w.Code != http.StatusBadRequest {
		t.Fatalf("an unconfirmed retire: %d %s", w.Code, w.Body.String())
	}
	w := retire(pair.first, `{"confirmed":true}`)
	var view serverSetupPeerView
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &view) != nil || view.PeerSetup != "retired" {
		t.Fatalf("retire: %d %s", w.Code, w.Body.String())
	}
	if join := firstJoinRecord(t, pair); join.LinkCredentialHash != "" || join.PeerRetiredAt == "" {
		t.Fatalf("the link did not end: %+v", join)
	}
	completeWithChecks(t, pair.second, plan, second)
	if second.Status != "succeeded" {
		t.Fatalf("the backup's completion was held by a retired link: %s %+v", second.Status, second.Error)
	}
}

// The main cannot be reached when the backup finishes: the backup waits
// with the reason and is not marked ready yet.
func TestServerSetupPeerBackupWaitsWhenTheMainCannotBeTold(t *testing.T) {
	pair, plan, second := boundPeerPair(t)
	completeWithChecks(t, pair.first, pair.plan, pair.execution)
	joinTestDial(pair.second.panel, func() string { return "127.0.0.1:1" })
	completeWithChecks(t, pair.second, plan, second)
	if second.Status != "waiting" || second.Error == nil || second.Error.Code != "server_setup_peer_ready_unreachable" {
		t.Fatalf("the backup did not wait for the main: %s %+v", second.Status, second.Error)
	}
	if state, _ := pair.second.panel.loadServerSetup(context.Background()); state.Status == "ready" {
		t.Fatal("the backup was marked ready before the main was told")
	}
	if view := peerViewOf(t, pair.first); view.PeerSetup != "pending" {
		t.Fatalf("the main's peer view: %+v", view)
	}
}
