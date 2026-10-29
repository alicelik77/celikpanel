package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
)

func readyReportForTest(t *testing.T, f serviceOperationTestFixture, bearer, joinRequestID string) *httptest.ResponseRecorder {
	t.Helper()
	return joinMachineRequest(t, f, serverSetupPeerReadyPath, bearer, serverSetupPeerReadyRequest{Protocol: serverSetupLinkProtocol, JoinRequestID: joinRequestID})
}

// Review fix (item 1): the backup took every refusal for "the main already
// ended the link" and marked itself ready while the main waited for ever.
// Now only an acknowledgement or the main's authenticated "link ended" (for
// exactly the credential that link accepted, from the joined address) is
// done; a refusal from a live link is a wait for the owner.
func TestServerSetupPeerReadyTellsAnEndedLinkFromARefusal(t *testing.T) {
	pair, plan, second := boundPeerPair(t)
	completeWithChecks(t, pair.first, pair.plan, pair.execution)
	link, _ := pair.second.panel.readServerSetupLink(context.Background())
	// A credential the live main does not accept: 401, and the backup waits.
	good := link.LinkCredential
	link.LinkCredential = strings.Repeat("e", len(good))
	if err := pair.second.panel.writeServerSetupLink(context.Background(), pair.second.database.GetDB(), *link); err != nil {
		t.Fatal(err)
	}
	completeWithChecks(t, pair.second, plan, second)
	if second.Status != "waiting" || second.Error == nil || second.Error.Code != "server_setup_peer_ready_refused" ||
		!strings.Contains(second.Error.Message, "Stop waiting for the second server") {
		t.Fatalf("a refusal from a live link was taken for done: %s %+v", second.Status, second.Error)
	}
	if join := firstJoinRecord(t, pair); join.PeerReadyAt != "" || join.LinkCredentialHash == "" {
		t.Fatalf("the main's link changed: %+v", join)
	}
	// With its real credential the backup reports and both are ready.
	link.LinkCredential = good
	if err := pair.second.panel.writeServerSetupLink(context.Background(), pair.second.database.GetDB(), *link); err != nil {
		t.Fatal(err)
	}
	completeWithChecks(t, pair.second, plan, second)
	if second.Status != "succeeded" || firstJoinRecord(t, pair).LinkCredentialHash != "" {
		t.Fatalf("after the report: %s %+v", second.Status, second.Error)
	}
	bearer := link.LinkID + "." + good
	// The ended credential from the joined address: told it ended.
	if w := readyReportForTest(t, pair.first, bearer, link.RequestID); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "server_setup_peer_link_ended") {
		t.Fatalf("ended link: %d %s", w.Code, w.Body.String())
	}
	// Another credential, or the same from another address: a bare refusal.
	if w := readyReportForTest(t, pair.first, link.LinkID+"."+strings.Repeat("f", len(good)), link.RequestID); w.Code != http.StatusUnauthorized {
		t.Fatalf("another credential: %d %s", w.Code, w.Body.String())
	}
	pair.first.panel.setupLink.sourceIP = func(*http.Request) (netip.Addr, bool) { return netip.MustParseAddr("198.51.100.99"), true }
	if w := readyReportForTest(t, pair.first, bearer, link.RequestID); w.Code != http.StatusUnauthorized {
		t.Fatalf("the ended credential from another address: %d %s", w.Code, w.Body.String())
	}
}

// Review fix (items 3 and 10): a server that was the second of pair A and
// is now the first of pair B keeps both records. A report acts only on the
// record of the namespace its credential authenticates in; a credential is
// never accepted in the other namespace; a report from another address than
// the second server's joined one is not recorded.
func TestServerSetupPeerReadyActsOnlyOnItsOwnRolesRecord(t *testing.T) {
	pair, _, _ := boundPeerPair(t)
	ctx := context.Background()
	db := pair.first.database.GetDB()
	// Pair A: this first server had joined another first server earlier.
	older := serverSetupLinkRecord{RequestID: strings.Repeat("a", 32), Address: "https://192.0.2.50:2083", Pin: strings.Repeat("0", 64), LinkID: strings.Repeat("b", 32), LinkCredential: strings.Repeat("c", 64), JoinedAt: "2026-10-01T00:00:00Z"}
	older.Document.First.IPv4 = "192.0.2.50"
	if err := pair.first.panel.writeServerSetupLink(ctx, db, older); err != nil {
		t.Fatal(err)
	}
	link, _ := pair.second.panel.readServerSetupLink(ctx)
	joinB := firstJoinRecord(t, pair)
	aBearer := older.LinkID + "." + serverSetupReverseCredential(older.LinkCredential)
	// Pair A's credential with pair B's join, and pair B's with pair A's: other joins.
	if w := readyReportForTest(t, pair.first, aBearer, joinB.RequestID); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "server_setup_peer_ready_other_join") {
		t.Fatalf("A's credential for B's join: %d %s", w.Code, w.Body.String())
	}
	if w := readyReportForTest(t, pair.first, link.bearer(), older.RequestID); w.Code != http.StatusConflict {
		t.Fatalf("B's credential for A's join: %d %s", w.Code, w.Body.String())
	}
	// A credential presented with the other namespace's link id: refused.
	if w := readyReportForTest(t, pair.first, joinB.LinkID+"."+serverSetupReverseCredential(older.LinkCredential), joinB.RequestID); w.Code != http.StatusUnauthorized {
		t.Fatalf("a credential from the other namespace: %d %s", w.Code, w.Body.String())
	}
	// Pair B's report from another address than the joined one.
	pair.first.panel.setupLink.sourceIP = func(*http.Request) (netip.Addr, bool) { return netip.MustParseAddr("198.51.100.99"), true }
	if w := readyReportForTest(t, pair.first, link.bearer(), joinB.RequestID); w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "server_setup_peer_ready_source") {
		t.Fatalf("a report from another address: %d %s", w.Code, w.Body.String())
	}
	pair.first.panel.setupLink.sourceIP = func(*http.Request) (netip.Addr, bool) { return netip.MustParseAddr("198.51.100.20"), true }
	// Pair A's report acts on pair A's record only.
	if w := readyReportForTest(t, pair.first, aBearer, older.RequestID); w.Code != http.StatusOK {
		t.Fatalf("A's report: %d %s", w.Code, w.Body.String())
	}
	if again, _ := pair.first.panel.readServerSetupLink(ctx); again == nil || again.PeerReadyAt == "" {
		t.Fatalf("A's record: %+v", again)
	}
	if join := firstJoinRecord(t, pair); join.PeerReadyAt != "" {
		t.Fatalf("A's report changed B's record: %+v", join)
	}
	// This server's own setup is pair B's first: its view reads B's record.
	if role, err := pair.first.panel.serverSetupOwnLinkRole(ctx); err != nil || role != serverSetupLinkRoleFirst {
		t.Fatalf("own role: %q %v", role, err)
	}
}

// optionAPair: the second server is the main DNS server (Option A); the
// first, the backup, reaches the main's link listener.
func optionAPair(t *testing.T) (*peerPair, serverSetupPlan, *serverSetupExecution) {
	t.Helper()
	first := joinFirstDraft()
	first.MainDNSServer = "peer"
	pair := newPeerPair(t, first)
	if answer := pair.connect(t, issueJoinCode(t, pair.first).Code); answer.State != "ok" {
		t.Fatalf("join: %+v", answer)
	}
	_, plan := pair.secondReview(t, nil)
	second := pair.startSecond(t, plan)
	if done, err := pair.second.panel.runServerSetupPeerNames(context.Background(), plan, second.Steps[0]); !done || err != nil {
		t.Fatalf("names: %v %v", done, err)
	}
	if joinIndex := pair.advanceToJoin(t); pair.execution.Steps[joinIndex].Status != "succeeded" {
		t.Fatalf("bind: %+v", pair.execution.Error)
	}
	listener := serveLinkOf(t, pair.second, pair.served)
	joinTestDial(pair.first.panel, func() string { return listener })
	pair.second.panel.setupLink.sourceIP = func(*http.Request) (netip.Addr, bool) { return netip.MustParseAddr("203.0.113.10"), true }
	return pair, plan, second
}

// Review fix (item 10): Option A, the main is the second server. Its link
// stays open until the first server (the backup) reports; the owner can stop
// waiting there too, with its audit row; the backup's later report is then
// told the link ended and the backup finishes.
func TestServerSetupPeerOptionAMainKeepsAndRetiresItsLink(t *testing.T) {
	t.Run("report", func(t *testing.T) {
		pair, plan, second := optionAPair(t)
		completeWithChecks(t, pair.second, plan, second)
		if second.Status != "succeeded" {
			t.Fatalf("the main completes: %s %+v", second.Status, second.Error)
		}
		if link, _ := pair.second.panel.readServerSetupLink(context.Background()); link == nil || link.LinkCredential == "" {
			t.Fatal("the main ended its link before the backup reported")
		}
		if view := peerViewOf(t, pair.second); view.PeerSetup != "pending" {
			t.Fatalf("the main's view: %+v", view)
		}
		completeWithChecks(t, pair.first, pair.plan, pair.execution)
		if pair.execution.Status != "succeeded" {
			t.Fatalf("the backup completes: %s %+v", pair.execution.Status, pair.execution.Error)
		}
		link, _ := pair.second.panel.readServerSetupLink(context.Background())
		if link == nil || link.PeerReadyAt == "" || link.LinkCredential != "" || link.EndedCredentialHash == "" {
			t.Fatalf("the main's link after the report: %+v", link)
		}
	})
	t.Run("retire", func(t *testing.T) {
		pair, plan, second := optionAPair(t)
		completeWithChecks(t, pair.second, plan, second)
		w := httptest.NewRecorder()
		pair.second.panel.handleServerSetupPeerRetire(w, serviceOperationAdminRequest(t, http.MethodPost, serverSetupPeerRetirePath, `{"confirmed":true}`, pair.second.userID))
		var view serverSetupPeerView
		if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &view) != nil || view.PeerSetup != "retired" {
			t.Fatalf("retire: %d %s", w.Code, w.Body.String())
		}
		var rows int
		pair.second.database.GetDB().QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action='server.setup.peer_retire'`).Scan(&rows)
		if rows != 1 {
			t.Fatalf("retire audit rows: %d", rows)
		}
		// Retiring again ends nothing more and writes no second row.
		w = httptest.NewRecorder()
		pair.second.panel.handleServerSetupPeerRetire(w, serviceOperationAdminRequest(t, http.MethodPost, serverSetupPeerRetirePath, `{"confirmed":true}`, pair.second.userID))
		pair.second.database.GetDB().QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action='server.setup.peer_retire'`).Scan(&rows)
		if w.Code != http.StatusOK || rows != 1 {
			t.Fatalf("retire again: %d %s, rows %d", w.Code, w.Body.String(), rows)
		}
		completeWithChecks(t, pair.first, pair.plan, pair.execution)
		if pair.execution.Status != "succeeded" {
			t.Fatalf("the backup after the main stopped waiting: %s %+v", pair.execution.Status, pair.execution.Error)
		}
	})
}

// Review fix (item 4): a ready report whose main cannot read its own setup
// state was answered "recorded" with the link left open for ever. It is an
// error now, so the backup reports again; the next report ends the link.
// Retiring with no record for this server's role is refused, never written.
func TestServerSetupPeerReadyNeverLeavesTheLinkOpenBehindRecorded(t *testing.T) {
	pair, _, _ := boundPeerPair(t)
	completeWithChecks(t, pair.first, pair.plan, pair.execution)
	link, _ := pair.second.panel.readServerSetupLink(context.Background())
	db := pair.first.database.GetDB()
	if _, err := db.Exec(`ALTER TABLE server_setup_state RENAME TO server_setup_state_away`); err != nil {
		t.Fatal(err)
	}
	if w := readyReportForTest(t, pair.first, link.bearer(), link.RequestID); w.Code < 500 {
		t.Fatalf("an unreadable state was answered: %d %s", w.Code, w.Body.String())
	}
	if _, err := db.Exec(`ALTER TABLE server_setup_state_away RENAME TO server_setup_state`); err != nil {
		t.Fatal(err)
	}
	if w := readyReportForTest(t, pair.first, link.bearer(), link.RequestID); w.Code != http.StatusOK {
		t.Fatalf("the report again: %d %s", w.Code, w.Body.String())
	}
	if join := firstJoinRecord(t, pair); join.PeerReadyAt == "" || join.LinkCredentialHash != "" {
		t.Fatalf("the link after the report again: %+v", join)
	}
	if _, err := db.Exec(`DELETE FROM panel_settings WHERE key=?`, serverSetupJoinKey); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	pair.first.panel.handleServerSetupPeerRetire(w, serviceOperationAdminRequest(t, http.MethodPost, serverSetupPeerRetirePath, `{"confirmed":true}`, pair.first.userID))
	if w.Code != http.StatusConflict {
		t.Fatalf("retire with no record: %d %s", w.Code, w.Body.String())
	}
	if found, _ := readServerSetupJSONSetting(context.Background(), db, serverSetupJoinKey, &serverSetupJoinRecord{}); found {
		t.Fatal("retire wrote a record that did not exist")
	}
}

// Review fix (item 9): the main answering with another TLS key is a verified
// state with the owner's action (a new join code), not "unreachable".
func TestServerSetupPeerReadyToAMainWithAnotherKeySaysSo(t *testing.T) {
	pair, plan, second := boundPeerPair(t)
	completeWithChecks(t, pair.first, pair.plan, pair.execution)
	other, _ := joinTestCertificate(t)
	listener := serveLinkOf(t, pair.first, other)
	joinTestDial(pair.second.panel, func() string { return listener })
	completeWithChecks(t, pair.second, plan, second)
	if second.Status != "waiting" || second.Error == nil || second.Error.Code != "server_setup_peer_ready_identity_changed" || !strings.Contains(second.Error.Message, "new join code") {
		t.Fatalf("another key: %s %+v", second.Status, second.Error)
	}
	if join := firstJoinRecord(t, pair); join.PeerReadyAt != "" {
		t.Fatal("something was sent to another key")
	}
}

// Beta-1 third re-run N6: in Option A the backup DNS server is the server
// that created the code; the ended-link text told it to make a new code "on
// the first server". Each side is now told by role where the code is made.
func TestServerSetupPeerLinkRevokedNamesTheServerByRole(t *testing.T) {
	var onFirst, onSecond *serverSetupChildFailure
	if !errors.As(serverSetupPeerLinkRevoked(false), &onFirst) || !errors.As(serverSetupPeerLinkRevoked(true), &onSecond) {
		t.Fatal("not a coded failure")
	}
	for _, message := range []string{onFirst.Message, onSecond.Message} {
		if strings.Contains(message, "first server") || strings.Contains(message, "second server") {
			t.Fatalf("named by position: %q", message)
		}
	}
	if !strings.Contains(onFirst.Message, "Create a new join code here") || !strings.Contains(onSecond.Message, "the server that created the earlier code") {
		t.Fatalf("by role: %q / %q", onFirst.Message, onSecond.Message)
	}
}
