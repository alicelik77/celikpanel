package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// heldLockPanelForTest is a panel whose database answers busy after 50 ms
// instead of 5 s (one pooled connection, its busy timeout lowered), and a
// second connection that holds the write lock the way the alpha.123 lab did
// (BEGIN IMMEDIATE from another process). release ends the hold.
func heldLockPanelForTest(t *testing.T) (p *Panel, path string, hold func(), release func()) {
	t.Helper()
	path = filepath.Join(t.TempDir(), "panel.sqlite")
	database, err := openTemplatePanelDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	pool := database.GetDB()
	pool.SetMaxOpenConns(1)
	if _, err := pool.Exec(`PRAGMA busy_timeout = 50`); err != nil {
		t.Fatal(err)
	}
	other, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(50)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { other.Close() })
	var conn *sql.Conn
	hold = func() {
		t.Helper()
		c, err := other.Conn(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := c.ExecContext(context.Background(), `BEGIN IMMEDIATE`); err != nil {
			t.Fatal(err)
		}
		conn = c
	}
	release = func() {
		if conn == nil {
			return
		}
		_, _ = conn.ExecContext(context.Background(), `ROLLBACK`)
		conn.Close()
		conn = nil
	}
	t.Cleanup(release)
	return &Panel{db: database}, path, hold, release
}

func shortEvidenceRetriesForTest(t *testing.T) {
	t.Helper()
	saved := []any{panelBusyWriteBound, panelBusyWriteFirstDelay, panelBusyWriteMaxDelay, auditRetryFirstDelay, auditRetryMaxDelay, auditDirectTimeout}
	panelBusyWriteBound, panelBusyWriteFirstDelay, panelBusyWriteMaxDelay = 5*time.Second, 10*time.Millisecond, 50*time.Millisecond
	auditRetryFirstDelay, auditRetryMaxDelay, auditDirectTimeout = 10*time.Millisecond, 50*time.Millisecond, 2*time.Second
	t.Cleanup(func() {
		panelBusyWriteBound = saved[0].(time.Duration)
		panelBusyWriteFirstDelay = saved[1].(time.Duration)
		panelBusyWriteMaxDelay = saved[2].(time.Duration)
		auditRetryFirstDelay = saved[3].(time.Duration)
		auditRetryMaxDelay = saved[4].(time.Duration)
		auditDirectTimeout = saved[5].(time.Duration)
	})
}

func auditRowsForTest(t *testing.T, p *Panel, action string) (count int, createdAt string) {
	t.Helper()
	if err := p.db.GetDB().QueryRow(`SELECT COUNT(*), COALESCE(MIN(created_at), '') FROM audit_logs WHERE action = ?`, action).Scan(&count, &createdAt); err != nil {
		t.Fatal(err)
	}
	return count, createdAt
}

func waitDrainedForTest(t *testing.T, drained <-chan struct{}) {
	t.Helper()
	select {
	case <-drained:
	case <-time.After(10 * time.Second):
		t.Fatal("the pending audit rows were never written")
	}
}

// alpha.123 lab (Debian 13): a write lock held past the busy timeout lost
// dns.engine.switch.setup.reconcile_required ("audit write failed ...:
// context deadline exceeded"). The row is now kept, said on the audit page,
// and written with its own time once the lock is gone.
func TestAuditRowIsKeptWhileThePanelDatabaseIsHeldAndWrittenAfter(t *testing.T) {
	shortEvidenceRetriesForTest(t)
	p, _, hold, release := heldLockPanelForTest(t)
	drained := make(chan struct{}, 1)
	p.auditPending.drained = func() { drained <- struct{}{} }
	persisted := persistedDNSEngineSwitch{RequestID: strings.Repeat("8", 32), SwitchID: strings.Repeat("a", 32), TargetEngine: "pdns", Action: "install", Mode: "switch"}
	action := dnsEngineAuditAction("setup.reconcile_required", persisted)

	hold()
	before := time.Now().UTC().Add(-time.Second).Format(auditCreatedAtLayout)
	p.auditDNSEngineBounded(dnsEngineAuditActor{}, "setup.reconcile_required", persisted)
	if count, _ := auditRowsForTest(t, p, action); count != 0 {
		t.Fatalf("written under a held lock: %d", count)
	}
	summary := p.auditPendingSummary(time.Now())
	if summary == nil || summary.Count != 1 || summary.Code != auditPendingWaitingCode || !strings.Contains(summary.Message, "Nobody needs to act") {
		t.Fatalf("the waiting row is not said: %+v", summary)
	}
	// Past the stall bound the page names the owner's action.
	if stalled := p.auditPendingSummary(time.Now().Add(auditPendingStallTime + time.Minute)); stalled == nil || stalled.Code != auditPendingStalledCode || !strings.Contains(stalled.Message, "systemctl restart celikpanel-panel") {
		t.Fatalf("the stalled row is not said: %+v", stalled)
	}
	// A later row joins the queue behind it instead of jumping ahead.
	p.recordAudit(auditRecord{Action: "later.row"})
	time.Sleep(150 * time.Millisecond)
	release()
	waitDrainedForTest(t, drained)

	count, createdAt := auditRowsForTest(t, p, action)
	if count != 1 || createdAt < before {
		t.Fatalf("the kept row: %d rows, created_at %q (want >= %q)", count, createdAt, before)
	}
	var first, second int
	if err := p.db.GetDB().QueryRow(`SELECT (SELECT id FROM audit_logs WHERE action = ?), (SELECT id FROM audit_logs WHERE action = 'later.row')`, action).Scan(&first, &second); err != nil || first >= second {
		t.Fatalf("order lost: %d %d %v", first, second, err)
	}
	if p.auditPendingSummary(time.Now()) != nil {
		t.Fatal("rows are still said to be waiting")
	}
	// The once-per-outcome rule of the DNS engine rows still holds.
	p.auditDNSEngineBounded(dnsEngineAuditActor{}, "setup.reconcile_required", persisted)
	if count, _ := auditRowsForTest(t, p, action); count != 1 {
		t.Fatalf("written twice: %d", count)
	}
}

// A row kept pending survives a panel stop: it is in the pending file (0600),
// a new start writes it, and a replay of a row already written (a stop
// between the write and the file update) stores nothing twice.
func TestPendingAuditRowsSurviveARestartAndAreNotWrittenTwice(t *testing.T) {
	shortEvidenceRetriesForTest(t)
	p, _, hold, release := heldLockPanelForTest(t)
	pending := filepath.Join(t.TempDir(), auditPendingFileName)
	p.auditPending.path = pending
	// This panel "stops" before its drainer runs: none is started.
	p.auditPending.draining = true
	hold()
	p.recordAudit(auditRecord{Action: "kept.row", ResourceType: "server_setup"})
	info, err := os.Stat(pending)
	if err != nil {
		t.Fatalf("the pending file was not written: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("pending file mode %v", info.Mode().Perm())
	}
	raw, err := os.ReadFile(pending)
	if err != nil {
		t.Fatal(err)
	}
	var file auditPendingFile
	if err := json.Unmarshal(raw, &file); err != nil || len(file.Entries) != 1 {
		t.Fatalf("pending file %s (%v)", raw, err)
	}
	// The first panel stops here (its drainer would only find the lock).
	release()

	// The row was written once already before the "crash" (the file was
	// not updated yet): the replay finds it and writes nothing.
	if err := p.insertAuditRecord(context.Background(), file.Entries[0].Record, false); err != nil {
		t.Fatal(err)
	}
	// A second pending row was never written.
	other := file.Entries[0]
	other.Record.EventID = strings.Repeat("b", 32)
	other.Record.Action = "replayed.row"
	file.Entries = append(file.Entries, other)
	encoded, _ := json.Marshal(file)
	if err := os.WriteFile(pending, encoded, 0o600); err != nil {
		t.Fatal(err)
	}

	restarted := &Panel{db: p.db}
	restarted.auditPending.path = pending
	drained := make(chan struct{}, 1)
	restarted.auditPending.drained = func() { drained <- struct{}{} }
	restarted.resumePendingAudit()
	waitDrainedForTest(t, drained)
	if count, _ := auditRowsForTest(t, restarted, "kept.row"); count != 1 {
		t.Fatalf("kept row written %d times", count)
	}
	if count, _ := auditRowsForTest(t, restarted, "replayed.row"); count != 1 {
		t.Fatalf("replayed row written %d times", count)
	}
	if _, err := os.Stat(pending); !os.IsNotExist(err) {
		t.Fatalf("the pending file is left after every row was written: %v", err)
	}
}

// A component operation's own state under a held lock (alpha.123 lab): the
// read says it waits for the panel's database, the result is saved once the
// lock is gone, and a succeeded operation is never recorded as failed only
// because its result could not be saved at once.
func TestServiceOperationResultWaitsForAHeldPanelDatabase(t *testing.T) {
	shortEvidenceRetriesForTest(t)
	p, _, hold, release := heldLockPanelForTest(t)
	id := strings.Repeat("c", 32)
	if _, err := p.db.GetDB().Exec(`INSERT INTO service_operations (id, kind, service_id, status, phase, started_at, created_at, updated_at) VALUES (?, 'service_install', 'nginx', 'running', 'installing', 'x', 'x', 'x')`, id); err != nil {
		t.Fatal(err)
	}
	hold()
	done := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		// The caller's own deadline is shorter than the hold: the write
		// still outlives it.
		done <- p.finishServiceOperationSucceeded(ctx, id, serviceOperationResult{"success": true})
	}()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, ok := panelBusyNoted(panelBusyKeyOperation(id)); ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the unsaved wait was never noted")
		}
		time.Sleep(10 * time.Millisecond)
	}
	op, err := p.serviceOperationByID(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	shown := p.markServerSetupChild(context.Background(), op)
	if shown.Status != serviceOperationRunning || shown.Phase != serviceOperationPanelBusyPhase {
		t.Fatalf("the read during the hold: %s %s", shown.Status, shown.Phase)
	}
	time.Sleep(200 * time.Millisecond)
	release()
	if err := <-done; err != nil {
		t.Fatalf("the result was not saved after the hold: %v", err)
	}
	op, err = p.serviceOperationByID(context.Background(), id)
	if err != nil || op.Status != serviceOperationSucceeded {
		t.Fatalf("after the hold: %+v %v", op, err)
	}
	if _, ok := panelBusyNoted(panelBusyKeyOperation(id)); ok {
		t.Fatal("the wait is still noted after the save")
	}
}

// Past its bound the write is a verified failure that says the database
// stayed busy, and the read names the owner's action; nothing is guessed.
func TestServiceOperationStateWriteStopsAtItsBound(t *testing.T) {
	shortEvidenceRetriesForTest(t)
	panelBusyWriteBound = 200 * time.Millisecond
	p, _, hold, _ := heldLockPanelForTest(t)
	id := strings.Repeat("d", 32)
	if _, err := p.db.GetDB().Exec(`INSERT INTO service_operations (id, kind, service_id, status, phase, started_at, created_at, updated_at) VALUES (?, 'service_install', 'nginx', 'running', 'installing', 'x', 'x', 'x')`, id); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { clearPanelBusy(panelBusyKeyOperation(id)) })
	hold()
	err := p.updateServiceOperationPhase(context.Background(), id, "verifying")
	if err == nil || !strings.Contains(err.Error(), errPanelStayedBusy.Error()) {
		t.Fatalf("bounded failure: %v", err)
	}
	shown := overlayServiceOperationPanelBusy(serviceOperation{ID: id, Status: serviceOperationRunning, Phase: "installing"})
	if shown.Phase != serviceOperationPanelStayedBusyPhase {
		t.Fatalf("phase after the bound: %s", shown.Phase)
	}
}

// Setup's own state under a held lock: the save fails, the read shows the
// wait (server_setup_panel_busy) instead of "running", and the next save
// clears it.
func TestServerSetupWaitIsShownWhenItCannotBeSaved(t *testing.T) {
	p, _, hold, release := heldLockPanelForTest(t)
	id := strings.Repeat("e", 32)
	execution := serverSetupExecution{ID: id, Status: "running", Phase: "dns"}
	encoded, _ := json.Marshal(execution)
	// Only the run's own row matters here, not its plan (the pool has one
	// connection, so the pragma covers the insert).
	pool := p.db.GetDB()
	if _, err := pool.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(`INSERT INTO server_setup_executions (id, request_id, plan_id, status, execution_json, created_at, updated_at) VALUES (?, ?, ?, 'running', ?, 'x', 'x')`, id, strings.Repeat("f", 32), "plan", string(encoded)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { clearPanelBusy(panelBusyKeySetup(id)) })
	hold()
	waiting := execution
	waiting.Status = "waiting"
	waiting.Error = &serviceOperationError{Code: serverSetupPanelBusyCode, Message: serverSetupPanelBusyMessage}
	if err := p.persistServerSetupExecution(context.Background(), waiting); err == nil || !panelDatabaseBusy(err) {
		t.Fatalf("saved under a held lock: %v", err)
	}
	shown := execution
	overlayServerSetupPanelBusy(&shown)
	if shown.Status != "waiting" || shown.Error == nil || shown.Error.Code != serverSetupPanelBusyCode {
		t.Fatalf("the read during the hold: %s %+v", shown.Status, shown.Error)
	}
	release()
	if err := p.persistServerSetupExecution(context.Background(), execution); err != nil {
		t.Fatal(err)
	}
	if _, ok := panelBusyNoted(panelBusyKeySetup(id)); ok {
		t.Fatal("the wait is still noted after a save")
	}
}
