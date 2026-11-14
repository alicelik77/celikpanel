package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/mariadbgrant"
)

// Findings of the alpha.134 review, on the production path of the loop.

// An anonymous account's row on `test\_%` and a hand-made pattern that
// reaches hundreds of databases are recorded and listed; they never stop the
// scan (the CHECKs refused both before).
func TestGrantFormsRecordAnonymousAndWideRows(t *testing.T) {
	f, engine := grantFormsFixture(t)
	wide := make([]string, 300)
	for i := range wide {
		wide[i] = fmt.Sprintf("shop%03d", i)
	}
	engine.scanResult.Patterns = append(engine.scanResult.Patterns,
		mariadbgrant.Row{User: "", Host: "%", Stored: `test\_%`, Flags: allFlags, Privileges: "ALL PRIVILEGES", Reached: []string{"test_a"}},
		mariadbgrant.Row{User: "hand", Host: "%", Stored: "shop%", Flags: allFlags, Privileges: "ALL PRIVILEGES", Reached: wide},
	)
	f.panel.runDatabaseGrantFormsRound(context.Background(), longAgo())
	if state, reason := grantFormEngineState(t, f); state != grantFormsDone {
		t.Fatalf("engine %s/%s", state, reason)
	}
	if kind, state, _ := grantFormRowState(t, f, "", "%", `test\_%`); kind != "not_ours" || state != "left" {
		t.Fatalf("anonymous row %s/%s", kind, state)
	}
	var reached string
	var more int
	if err := f.panel.db.GetDB().QueryRow(`SELECT reached, reached_more FROM database_grant_forms WHERE username = 'hand' AND stored_name = 'shop%'`).Scan(&reached, &more); err != nil {
		t.Fatal(err)
	}
	var names []string
	_ = json.Unmarshal([]byte(reached), &names)
	if len(names) != grantFormsReachedCap || more != 300-grantFormsReachedCap {
		t.Fatalf("reached %d + %d", len(names), more)
	}
	recorder := grantFormsRequest(t, f, "admin")
	var view grantFormsView
	_ = json.Unmarshal(recorder.Body.Bytes(), &view)
	found := false
	for _, row := range view.Left {
		if row.AccountKind == "anonymous" {
			found = true
			// A wildcard on purpose: the statement removes it, and says so.
			if row.StatementKind != mariadbgrant.StatementRemove || !strings.Contains(row.Statement, "FROM ''@'%';") {
				t.Fatalf("anonymous row %+v", row)
			}
		}
		if row.Account == "hand@%" && row.Pattern == "shop%" && row.ReachedMore != 250 {
			t.Fatalf("wide row %+v", row)
		}
	}
	if !found {
		t.Fatalf("left %+v", view.Left)
	}
}

// What was found but could not be recorded puts the engine into a stored
// blocked state with its reason; it is never silent.
func TestGrantFormsRecordingFailureBlocksTheEngine(t *testing.T) {
	f, _ := grantFormsFixture(t)
	f.exec(t, `CREATE TRIGGER refuse_grant_forms BEFORE INSERT ON database_grant_forms BEGIN SELECT RAISE(ABORT, 'disk full'); END`)
	f.panel.runDatabaseGrantFormsRound(context.Background(), longAgo())
	if state, reason := grantFormEngineState(t, f); state != grantFormsBlocked || reason != grantFormsReasonRecordFailed {
		t.Fatalf("engine %s/%s", state, reason)
	}
}

// The engine's gate is taken for one row at a time: a deletion waiting on
// the same engine gets in between two rows, not after the whole round.
func TestGrantFormsTakeTheEngineGatePerRow(t *testing.T) {
	f, engine := grantFormsFixture(t)
	engines, err := f.panel.databaseGrantFormsEngines(context.Background())
	if err != nil || len(engines) == 0 {
		t.Fatal(err)
	}
	server := engines[0].servers[0]
	var deletionRan atomic.Bool
	var waited atomic.Int64
	calls := 0
	engine.onConvert = func(mariadbgrant.ConvertRequest) {
		calls++
		switch calls {
		case 1:
			go func() {
				started := time.Now()
				release, err := acquireDatabaseEngineGate(context.Background(), server, databaseEngineDeleteWait)
				if err != nil {
					return
				}
				waited.Store(int64(time.Since(started)))
				deletionRan.Store(true)
				time.Sleep(20 * time.Millisecond)
				release()
			}()
			time.Sleep(100 * time.Millisecond) // the deletion now waits on the gate
		case 2:
			if !deletionRan.Load() {
				t.Error("the deletion did not get in between two rows")
			}
		}
	}
	f.panel.runDatabaseGrantFormsRound(context.Background(), longAgo())
	if calls < 2 {
		t.Fatalf("converted %d rows", calls)
	}
	if time.Duration(waited.Load()) > 2*time.Second {
		t.Fatalf("the deletion waited %s", time.Duration(waited.Load()))
	}
}

// An Agent that is another build, or does not answer, while a row is
// converted makes the engine wait; it is not counted as the row's failure.
func TestGrantFormsAgentTroubleIsAnEngineWaitNotARowFailure(t *testing.T) {
	for _, c := range []struct {
		err    error
		reason string
	}{
		{errGrantFormsBuildMismatch, grantFormsReasonAgentVersion},
		{&grantFormsAgentError{errors.New("connection refused")}, grantFormsReasonAgentUnreached},
	} {
		f, engine := grantFormsFixture(t)
		engine.convertErr = c.err
		for i := 0; i < grantFormsRowFailuresToBlock+1; i++ {
			f.panel.runDatabaseGrantFormsRound(context.Background(), longAgo())
		}
		if state, reason := grantFormEngineState(t, f); state != grantFormsWaiting || reason != c.reason {
			t.Fatalf("%v: engine %s/%s", c.err, state, reason)
		}
		var failures int
		_ = f.panel.db.GetDB().QueryRow(`SELECT MAX(failures) FROM database_grant_forms WHERE kind = 'ours'`).Scan(&failures)
		if failures != 0 {
			t.Fatalf("%v: rows counted %d failures", c.err, failures)
		}
	}
}

// A recorded database whose exact form does not fit a grant row stays in the
// pattern form and is listed with that reason; a blocked row is shown with
// its statement; the notification says why rows are left.
func TestGrantFormsLongNamesBlockedRowsAndNoticeWords(t *testing.T) {
	f, engine := grantFormsFixture(t)
	long := "abc_efg_ijk_mno_qrs_uvw_yzabcdefghijklmnopqrstuvwxyzabcdefgh"
	f.exec(t, `INSERT INTO databases_v2 (id, server_id, subscription_id, domain_id, name) VALUES (9970, 9921, 9802, 9902, ?)`, long)
	f.exec(t, `INSERT INTO database_user_grants (database_id, user_id, privileges) VALUES (9970, 9941, 'ALL')`)
	engine.scanResult.Patterns = append(engine.scanResult.Patterns,
		mariadbgrant.Row{User: "cust_user", Host: "localhost", Stored: long, Flags: allFlags, Privileges: "ALL PRIVILEGES"})
	f.panel.runDatabaseGrantFormsRound(context.Background(), longAgo())
	if kind, state, reason := grantFormRowState(t, f, "cust_user", "localhost", long); kind != "ours" || state != "left" || reason != grantFormsReasonNameTooLong {
		t.Fatalf("long name %s/%s/%s", kind, state, reason)
	}
	for _, req := range engine.converted {
		if req.Database == long {
			t.Fatal("a too long name was converted")
		}
	}
	f.exec(t, `UPDATE database_grant_forms SET state = 'blocked', kind = 'ours', reason = 'convert_failed' WHERE username = 'wp_user'`)
	var view grantFormsView
	_ = json.Unmarshal(grantFormsRequest(t, f, "admin").Body.Bytes(), &view)
	if len(view.Blocked) != 1 || view.Blocked[0].StatementKind != mariadbgrant.StatementNarrow {
		t.Fatalf("blocked %+v", view.Blocked)
	}
	var body string
	_ = f.panel.db.GetDB().QueryRow(`SELECT body FROM account_notices WHERE kind = ? LIMIT 1`, noticeDatabasePatternRights).Scan(&body)
	var parsed map[string]any
	_ = json.Unmarshal([]byte(body), &parsed)
	words := databasePatternRightsNoticeWords(audienceAdministrator, parsed)
	for _, want := range []string{"that CelikPanel did not give", "carry rights CelikPanel never gives", "too long for its exact form"} {
		if !strings.Contains(words.TextEN, want) {
			t.Fatalf("notice lacks %q: %s", want, words.TextEN)
		}
	}
	if strings.Contains(words.TextEN, "CelikPanel did not make them") {
		t.Fatal("the old sentence is back")
	}
}

// Second review (S2): a database deleted after the scan, before its row's
// turn, is not given an exact right: inside the gate the pair is read again
// from the records, and a pair no longer there leaves the round.
func TestGrantFormsSkipARowWhoseDatabaseWasDeletedSinceTheScan(t *testing.T) {
	f, engine := grantFormsFixture(t)
	calls := 0
	engine.onConvert = func(req mariadbgrant.ConvertRequest) {
		calls++
		if calls == 1 {
			// A deletion of customer_matrix_test_blog finishes between two
			// rows: its records go.
			if _, err := f.panel.db.GetDB().Exec(`DELETE FROM database_user_grants WHERE database_id IN (SELECT id FROM databases_v2 WHERE name = 'customer_matrix_test_blog')`); err != nil {
				t.Error(err)
			}
		}
	}
	f.panel.runDatabaseGrantFormsRound(context.Background(), longAgo())
	for _, req := range engine.converted {
		if req.Database == "customer_matrix_test_blog" {
			t.Fatalf("the deleted database was given an exact right: %+v", engine.converted)
		}
	}
	if _, state, reason := grantFormRowState(t, f, "wp_user", "203.0.113.7", "customer_matrix_test_blog"); state != "done" || reason != mariadbgrant.DatabaseGone {
		t.Fatalf("row %s/%s", state, reason)
	}
}

// Second review (W): what an engine waits for is said as it is.
func TestGrantFormsSayWhatIsTrueWhileWaiting(t *testing.T) {
	// The Agent does not answer the build check: "waiting for the Agent",
	// never "another build".
	f, engine := grantFormsFixture(t)
	saved := buildCommit
	buildCommit = strings.Repeat("b", 40)
	f.panel.runDatabaseGrantFormsRound(context.Background(), longAgo())
	buildCommit = saved
	if state, reason := grantFormEngineState(t, f); state != grantFormsWaiting || reason != grantFormsReasonAgentUnreached {
		t.Fatalf("no Agent: %s/%s", state, reason)
	}

	// CelikPanel's own records cannot be read: an unknown result, recorded.
	if _, err := f.panel.db.GetDB().Exec(`ALTER TABLE database_user_grants RENAME TO database_user_grants_away`); err != nil {
		t.Fatal(err)
	}
	f.panel.runDatabaseGrantFormsRound(context.Background(), longAgo())
	if state, reason := grantFormEngineState(t, f); state != grantFormsWaiting || reason != grantFormsReasonRecordsUnread || len(engine.converted) != 0 {
		t.Fatalf("records unread: %s/%s, %d converted", state, reason, len(engine.converted))
	}
	if _, err := f.panel.db.GetDB().Exec(`ALTER TABLE database_user_grants_away RENAME TO database_user_grants`); err != nil {
		t.Fatal(err)
	}

	// A row holding a privilege CelikPanel cannot name is never offered a
	// "narrow" statement that would drop it.
	engine.scanResult.Patterns = append(engine.scanResult.Patterns, mariadbgrant.Row{User: "hand2", Host: "%", Stored: "customer_matrix_test_shop",
		Flags: allFlags, Privileges: "SELECT", Unnamed: true})
	databaseGrantFormsUpdatePhase = func(string) (string, bool) { return "running", true }
	f.panel.runDatabaseGrantFormsRound(context.Background(), longAgo())
	view, err := f.panel.databaseGrantFormsView(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range view.Left {
		if row.Account == "hand2@%" {
			found = row.StatementKind == mariadbgrant.StatementRemove
		}
	}
	if !found {
		t.Fatalf("unnamed privilege row: %+v", view.Left)
	}

	// Waiting for a confirmed update longer than a day: one notification
	// to the administrators per wait, never one a round; cleared when the
	// wait ends; told again for a later, separate wait.
	if state, reason := grantFormEngineState(t, f); state != grantFormsWaiting || reason != grantFormsReasonUpdate {
		t.Fatalf("update: %s/%s", state, reason)
	}
	count := func(where string) int {
		var n int
		if err := f.panel.db.GetDB().QueryRow(`SELECT COUNT(DISTINCT period) FROM account_notices WHERE kind = ? AND `+where, noticeDatabaseRightsWaiting).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	ageWait := func(days string) {
		if _, err := f.panel.db.GetDB().Exec(`UPDATE database_grant_form_engines SET waiting_since = datetime('now', ?) WHERE engine_key = 'mariadb@this-machine:3306'`, "-"+days+" days"); err != nil {
			t.Fatal(err)
		}
	}
	if count("1") != 0 {
		t.Fatal("told before a day passed")
	}
	ageWait("3")
	for range 2 {
		f.panel.runDatabaseGrantFormsRound(context.Background(), longAgo())
	}
	if count("1") != 1 || count("seen_at IS NULL") != 1 {
		t.Fatalf("update wait: %d notices", count("1"))
	}
	// The update is confirmed: the wait ends and its notification is cleared.
	databaseGrantFormsUpdatePhase = func(string) (string, bool) { return "succeeded", true }
	f.panel.runDatabaseGrantFormsRound(context.Background(), longAgo())
	if count("seen_at IS NULL") != 0 {
		t.Fatal("the ended wait's notification was not cleared")
	}
	// A later, separate wait (the Agent stops answering) is told again once.
	saved = buildCommit
	buildCommit = strings.Repeat("c", 40)
	t.Cleanup(func() { buildCommit = saved })
	f.panel.runDatabaseGrantFormsRound(context.Background(), longAgo())
	ageWait("2")
	for range 2 {
		f.panel.runDatabaseGrantFormsRound(context.Background(), longAgo())
	}
	if count("1") != 2 || count("seen_at IS NULL") != 1 {
		t.Fatalf("agent wait: %d notices, %d open", count("1"), count("seen_at IS NULL"))
	}
	agentWords := databaseRightsWaitingNoticeWords("", map[string]any{"reason": grantFormsReasonAgentUnreached, "rows": float64(1)})
	if !strings.Contains(agentWords.TextEN, "systemctl restart celikpanel-agent") {
		t.Fatalf("agent words: %+v", agentWords)
	}
	words := databaseRightsWaitingNoticeWords("", map[string]any{"reason": grantFormsReasonUpdate, "rows": float64(2)})
	if !strings.Contains(words.TextEN, "Settings → Updates") || !strings.Contains(words.TextTR, "Ayarlar → Güncellemeler") {
		t.Fatalf("words: %+v", words)
	}
}

// Verification of the second review: the Agent finds the database gone
// while the pair is still on record (dropped by hand after the scan): the row
// is listed with what to do and counted as left, never marked done.
func TestGrantFormsListARowWhoseDatabaseIsGoneButStillRecorded(t *testing.T) {
	f, engine := grantFormsFixture(t)
	engine.outcomeFor = func(req mariadbgrant.ConvertRequest) string {
		if req.Database == "customer_matrix_test_shop" {
			return mariadbgrant.DatabaseGone
		}
		return mariadbgrant.Converted
	}
	f.panel.runDatabaseGrantFormsRound(context.Background(), longAgo())
	if kind, state, reason := grantFormRowState(t, f, "cust_user", "localhost", "customer_matrix_test_shop"); kind != "ours" || state != "left" || reason != grantFormsReasonDatabaseMissing {
		t.Fatalf("row %s/%s/%s", kind, state, reason)
	}
	view, err := f.panel.databaseGrantFormsView(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range view.Left {
		if row.Account == "cust_user@localhost" && row.Pattern == "customer_matrix_test_shop" {
			if row.Reason != grantFormsReasonDatabaseMissing || row.StatementKind != mariadbgrant.StatementRemove {
				t.Fatalf("listed %+v", row)
			}
			return
		}
	}
	t.Fatalf("not listed: %+v", view.Left)
}
