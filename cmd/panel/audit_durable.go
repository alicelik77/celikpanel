package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Durable audit rows (alpha.124; RESILIENCE-CONTRACT: persisted evidence is
// never silently lost).
//
// Until alpha.123 every audit row was one INSERT under the caller's context
// (often a request's, or a 5 s deadline) and a failure was only logged. In
// the alpha.123 lab a 60 s write lock on the panel database dropped
// dns.engine.switch.setup.reconcile_required that way ("audit write failed
// ...: context deadline exceeded").
//
// Every audit row now goes through recordAudit. It is tried once at once, on
// its own bounded context (a closed browser tab no longer cancels it). If the
// database does not take it, the row is kept: in memory and in a small
// pending file in the panel's data folder (written atomically, 0600), and a
// single drainer writes the pending rows in their order, with their own time,
// with a growing pause (1 s doubling to 30 s). No schema change: a row that
// comes back from the file after a start (it may have been written just
// before a stop, before the file was updated) is written only when no row
// with exactly its fields and its second is there yet. Rows still in memory
// are known: their write either returned success or did not. After ten
// minutes the wait is reported as stalled (log and the audit log page, with
// the owner's next action); the rows are still kept and tried again. Nothing
// is dropped.
//
// Rows written inside the transaction of the change they record
// (entitlements, the store catalogue) stay there: they commit or fail with
// that change, so they are never lost on their own.
//
// Denetim satırları artık kaybolmaz: veritabanı almazsa satır bellekte ve veri
// klasöründeki küçük bekleyen dosyasında tutulur, sırayla ve kendi zamanıyla
// yazılır; olay kimliği çift yazmayı önler.

// auditRecord is one audit row as it is stored.
type auditRecord struct {
	// EventID identifies the row inside the pending file (not stored in
	// the table).
	EventID      string `json:"event_id"`
	UserID       int    `json:"user_id,omitempty"`
	ActingAs     int    `json:"acting_as,omitempty"`
	Action       string `json:"action"`
	ResourceType string `json:"resource_type,omitempty"`
	ResourceID   int    `json:"resource_id,omitempty"`
	IP           string `json:"ip,omitempty"`
	UserAgent    string `json:"user_agent,omitempty"`
	// Details is the stored JSON object text ("" stores NULL).
	Details string `json:"details,omitempty"`
	// OncePerAction: write nothing when a row of this action and resource
	// type already exists (the DNS engine's outcome rows).
	OncePerAction bool `json:"once_per_action,omitempty"`
	// CreatedAt is when the action happened, in the column's own format
	// (UTC "2006-01-02 15:04:05"), so a row written late keeps its time.
	CreatedAt string `json:"created_at"`
}

const auditCreatedAtLayout = "2006-01-02 15:04:05"

func newAuditEventID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		// crypto/rand does not fail on a supported system; a time-based id
		// still keeps the row (the id only tells pending rows apart).
		return fmt.Sprintf("t%031x", time.Now().UnixNano())
	}
	return hex.EncodeToString(raw[:])
}

func nullablePositive(value int) any {
	if value > 0 {
		return value
	}
	return nil
}

// insertAuditRecord writes rec unless, for a once-per-action row, its action
// is there already, or, for a replay (a row read back from the pending file
// after a start), the same row is there already.
func (p *Panel) insertAuditRecord(ctx context.Context, rec auditRecord, replay bool) error {
	once, replayed := 0, 0
	if rec.OncePerAction {
		once = 1
	}
	if replay {
		replayed = 1
	}
	var details any
	if rec.Details != "" {
		details = rec.Details
	}
	_, err := p.db.GetDB().ExecContext(ctx, `
		INSERT INTO audit_logs (
		  user_id, action, resource_type, resource_id, ip_address, user_agent,
		  details, acting_as, created_at
		)
		SELECT ?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9
		WHERE NOT (?10 = 1 AND EXISTS (
		    SELECT 1 FROM audit_logs WHERE action = ?2 AND resource_type IS ?3
		  ))
		  AND NOT (?11 = 1 AND EXISTS (
		    SELECT 1 FROM audit_logs
		    WHERE created_at = ?9 AND action = ?2 AND resource_type IS ?3
		      AND resource_id IS ?4 AND user_id IS ?1 AND acting_as IS ?8
		      AND details IS ?7
		  ))`,
		nullablePositive(rec.UserID), rec.Action, nullableNonEmpty(rec.ResourceType),
		nullablePositive(rec.ResourceID), nullableNonEmpty(rec.IP), nullableNonEmpty(rec.UserAgent),
		details, nullablePositive(rec.ActingAs), rec.CreatedAt,
		once, replayed,
	)
	return err
}

// Bounds of the pending rows. Variables only so tests can make them short.
var (
	auditDirectTimeout    = newTunableDuration(panelBusyAttemptTimeout)
	auditRetryFirstDelay  = newTunableDuration(time.Second)
	auditRetryMaxDelay    = newTunableDuration(30 * time.Second)
	auditPendingStallTime = 10 * time.Minute
)

// auditPendingFileName is the pending rows' file in the data folder.
const auditPendingFileName = "audit-pending.json"

type auditPendingEntry struct {
	Record auditRecord `json:"record"`
	// FirstFailure: when the database first did not take the row.
	FirstFailure string `json:"first_failure"`
	Attempts     int    `json:"attempts"`
	// Replay: read back from the pending file after a start; it may have
	// been written just before the stop (not saved in the file).
	Replay    bool   `json:"-"`
	LastError string `json:"last_error,omitempty"`
}

type auditPendingFile struct {
	Version int                 `json:"version"`
	Entries []auditPendingEntry `json:"entries"`
}

// auditPendingQueue is the Panel's pending audit rows. Its zero value is
// ready; path "" keeps them in memory only (handler tests).
type auditPendingQueue struct {
	mu            sync.Mutex
	path          string
	loaded        bool
	entries       []auditPendingEntry
	draining      bool
	stallReported bool
	// spoolErr is the last failure to save the pending file ("" when saved).
	spoolErr string
	// drained is told when the drainer stops (tests).
	drained func()
}

// recordAudit stores one audit row; it never fails the action it records
// and never drops the row.
func (p *Panel) recordAudit(rec auditRecord) {
	if rec.EventID == "" {
		rec.EventID = newAuditEventID()
	}
	if rec.CreatedAt == "" {
		rec.CreatedAt = time.Now().UTC().Format(auditCreatedAtLayout)
	}
	// The column refuses details over its bound or not JSON; such a row
	// would wait forever, so it keeps a bounded marker instead.
	if rec.Details != "" && (len(rec.Details) > auditDetailsCap || !json.Valid([]byte(rec.Details))) {
		rec.Details = `{"truncated":true}`
	}
	q := &p.auditPending
	q.mu.Lock()
	q.loadLocked()
	waiting := len(q.entries) > 0
	q.mu.Unlock()
	cause := errors.New("earlier audit rows are still waiting to be saved")
	if !waiting {
		// Rows already waiting keep their order: a new row joins them.
		ctx, cancel := context.WithTimeout(context.Background(), auditDirectTimeout.get())
		err := p.insertAuditRecord(ctx, rec, false)
		cancel()
		if err == nil {
			return
		}
		cause = err
		log.Printf("audit write deferred (%s): %v; the row is kept and saved once the panel database takes it", rec.Action, err)
	}
	p.deferAuditRecord(rec, cause)
}

func (p *Panel) deferAuditRecord(rec auditRecord, cause error) {
	q := &p.auditPending
	q.mu.Lock()
	defer q.mu.Unlock()
	q.loadLocked()
	q.entries = append(q.entries, auditPendingEntry{
		Record:       rec,
		FirstFailure: time.Now().UTC().Format(time.RFC3339Nano),
		Attempts:     1,
		LastError:    boundedAuditError(cause),
	})
	q.saveLocked()
	p.startAuditDrainLocked()
}

func boundedAuditError(err error) string {
	if err == nil {
		return ""
	}
	text := err.Error()
	if len(text) > 300 {
		text = text[:300]
	}
	return text
}

// resumePendingAudit is called once at start: rows a stopped panel could not
// save are written now.
func (p *Panel) resumePendingAudit() {
	q := &p.auditPending
	q.mu.Lock()
	defer q.mu.Unlock()
	q.loadLocked()
	if len(q.entries) > 0 {
		log.Printf("audit: %d row(s) from before the start are waiting to be saved; they are written now", len(q.entries))
		p.startAuditDrainLocked()
	}
}

func (q *auditPendingQueue) loadLocked() {
	if q.loaded {
		return
	}
	q.loaded = true
	if q.path == "" {
		return
	}
	raw, err := os.ReadFile(q.path)
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		q.spoolErr = boundedAuditError(err)
		log.Printf("audit: the pending rows file could not be read: %v", err)
		return
	}
	var file auditPendingFile
	if err := json.Unmarshal(raw, &file); err != nil || file.Version != 1 {
		// Kept aside, never overwritten: it is evidence.
		aside := q.path + ".unreadable-" + time.Now().UTC().Format("20060102T150405Z")
		if renameErr := os.Rename(q.path, aside); renameErr != nil {
			q.spoolErr = boundedAuditError(renameErr)
		}
		log.Printf("audit: the pending rows file is not readable (%v); it is kept as %s", err, aside)
		return
	}
	for index := range file.Entries {
		file.Entries[index].Replay = true
	}
	q.entries = append(file.Entries, q.entries...)
}

// saveLocked writes the pending rows atomically (temporary file, fsync,
// rename, directory fsync), or removes the file when none are left.
func (q *auditPendingQueue) saveLocked() {
	if q.path == "" {
		return
	}
	if len(q.entries) == 0 {
		if err := os.Remove(q.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			q.spoolErr = boundedAuditError(err)
			return
		}
		q.spoolErr = ""
		return
	}
	data, err := json.Marshal(auditPendingFile{Version: 1, Entries: q.entries})
	if err == nil {
		err = writeFileDurably(q.path, data, 0o600)
	}
	if err != nil {
		// The rows are still in memory and still written; only a crash
		// before then would lose them, and the page says so.
		q.spoolErr = boundedAuditError(err)
		log.Printf("audit: the pending rows could not be saved to %s: %v", q.path, err)
		return
	}
	q.spoolErr = ""
}

func writeFileDurably(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	cleanup := func() { _ = os.Remove(name) }
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		cleanup()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(name, path); err != nil {
		cleanup()
		return err
	}
	if handle, err := os.Open(dir); err == nil {
		_ = handle.Sync()
		handle.Close()
	}
	return nil
}

func (p *Panel) startAuditDrainLocked() {
	q := &p.auditPending
	if q.draining || len(q.entries) == 0 {
		return
	}
	q.draining = true
	go p.drainPendingAudit()
}

// drainPendingAudit writes the pending rows in order until none is left.
func (p *Panel) drainPendingAudit() {
	q := &p.auditPending
	delay := auditRetryFirstDelay.get()
	for {
		time.Sleep(delay)
		written, blocked := p.auditDrainPass()
		q.mu.Lock()
		if len(q.entries) == 0 {
			q.draining = false
			q.stallReported = false
			done := q.drained
			q.mu.Unlock()
			log.Printf("audit: every waiting row is saved")
			if done != nil {
				done()
			}
			return
		}
		if stalled, count := q.stalledLocked(time.Now()); stalled && !q.stallReported {
			q.stallReported = true
			log.Printf("audit: %d row(s) have waited more than %s to be saved (last reason: %s); they are kept and tried again. Owner: sudo systemctl restart celikpanel-panel restarts only the panel; sudo journalctl -u celikpanel-panel -n 100 shows what keeps the database busy", count, auditPendingStallTime, q.entries[0].LastError)
		}
		q.mu.Unlock()
		if written > 0 && !blocked {
			delay = auditRetryFirstDelay.get()
			continue
		}
		delay *= 2
		if maxDelay := auditRetryMaxDelay.get(); delay > maxDelay {
			delay = maxDelay
		}
	}
}

// auditDrainPass tries every pending row once, in order. A busy database
// ends the pass (the rows behind keep their order); a row refused for
// another reason stays pending and the rows behind it still go on.
func (p *Panel) auditDrainPass() (written int, blocked bool) {
	q := &p.auditPending
	q.mu.Lock()
	snapshot := append([]auditPendingEntry(nil), q.entries...)
	q.mu.Unlock()
	done := map[string]bool{}
	failed := map[string]string{}
	for _, entry := range snapshot {
		ctx, cancel := context.WithTimeout(context.Background(), auditDirectTimeout.get())
		err := p.insertAuditRecord(ctx, entry.Record, entry.Replay)
		busy := err != nil && retryableEvidenceWrite(context.Background(), ctx, err)
		if err != nil && !busy && panelDatabaseConstraint(err) && (entry.Record.UserID > 0 || entry.Record.ActingAs > 0) {
			// The person was removed while the row waited (users.id is a
			// foreign key): the row is kept and names them in its details.
			err = p.insertAuditRecord(ctx, withoutAuditPerson(entry.Record), entry.Replay)
		}
		cancel()
		if err == nil {
			done[entry.Record.EventID] = true
			written++
			continue
		}
		failed[entry.Record.EventID] = boundedAuditError(err)
		if busy {
			blocked = true
			break
		}
	}
	q.mu.Lock()
	kept := q.entries[:0]
	for _, entry := range q.entries {
		if done[entry.Record.EventID] {
			continue
		}
		if reason, ok := failed[entry.Record.EventID]; ok {
			entry.Attempts++
			entry.LastError = reason
		}
		kept = append(kept, entry)
	}
	q.entries = kept
	q.saveLocked()
	q.mu.Unlock()
	return written, blocked
}

func (q *auditPendingQueue) stalledLocked(now time.Time) (bool, int) {
	if len(q.entries) == 0 {
		return false, 0
	}
	first, err := time.Parse(time.RFC3339Nano, q.entries[0].FirstFailure)
	return err == nil && now.Sub(first) >= auditPendingStallTime, len(q.entries)
}

// auditPendingSummary is what the audit log page says about rows not saved
// yet (D-024: the reason, who acts, how it goes on).
type auditPendingSummary struct {
	Count   int    `json:"count"`
	Since   string `json:"since"`
	Code    string `json:"code"`
	Message string `json:"message"`
	// FileError: the pending rows could not be saved to their file either.
	FileError bool `json:"file_error,omitempty"`
}

const (
	auditPendingWaitingCode = "audit_rows_waiting"
	auditPendingStalledCode = "audit_rows_stalled"
)

func (p *Panel) auditPendingSummary(now time.Time) *auditPendingSummary {
	q := &p.auditPending
	q.mu.Lock()
	defer q.mu.Unlock()
	q.loadLocked()
	if len(q.entries) == 0 {
		return nil
	}
	summary := &auditPendingSummary{
		Count:     len(q.entries),
		Since:     q.entries[0].FirstFailure,
		Code:      auditPendingWaitingCode,
		FileError: q.spoolErr != "",
		Message: fmt.Sprintf("%d audit row(s) are waiting to be saved: CelikPanel's own database was busy when they were recorded. "+
			"They are kept and saved by themselves, in their order and with their own time, as soon as the database takes them. Nobody needs to act.", len(q.entries)),
	}
	if stalled, _ := q.stalledLocked(now); stalled {
		summary.Code = auditPendingStalledCode
		summary.Message = fmt.Sprintf("%d audit row(s) have not been saved for more than ten minutes: CelikPanel's own database keeps refusing them. "+
			"They are kept, not lost, and tried again by themselves. You, the owner: run sudo systemctl restart celikpanel-panel (it restarts only the panel; websites, mail and DNS keep running); "+
			"the rows are saved after the start. If this stays, sudo journalctl -u celikpanel-panel -n 100 shows the reason.", len(q.entries))
	}
	return summary
}

// panelDatabaseConstraint reports a constraint refusal of the panel's own
// database (SQLITE_CONSTRAINT, any extended code).
func panelDatabaseConstraint(err error) bool {
	var coded interface{ Code() int }
	return errors.As(err, &coded) && coded.Code()&0xff == 19
}

// withoutAuditPerson is rec without its user references, which move into its
// details (removed_user_id, removed_acting_as).
func withoutAuditPerson(rec auditRecord) auditRecord {
	details := map[string]any{}
	if rec.Details != "" {
		_ = json.Unmarshal([]byte(rec.Details), &details)
	}
	if rec.UserID > 0 {
		details["removed_user_id"] = rec.UserID
	}
	if rec.ActingAs > 0 {
		details["removed_acting_as"] = rec.ActingAs
	}
	rec.UserID, rec.ActingAs = 0, 0
	rec.Details = auditDetailsText(details)
	return rec
}
