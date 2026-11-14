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
