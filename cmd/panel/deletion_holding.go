package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/rpc"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/backupspec"
	"github.com/alicelik/celikpanel/internal/transport"
)

// What deletions left, kept 30 days and then removed (alpha.135; owner, 14
// Nov 2026: "30 gün sakla, sonra sil" for the stored mail of a deleted
// domain or mailbox, and "Posta ile aynı kural" for a deleted domain's
// backups on this server; remote backup targets are never touched).
//
// One holding list, one versioned panel_settings record per item
// (deletion_hold_v1:<id>; no schema change):
//
//   - written when a domain deletion moves the domain's mail aside or finds
//     its backups, and when a mailbox deletion moves the mailbox's mail
//     aside; a deletion pressed again keeps the first record, so its 30 days
//     do not start again;
//   - adopted, with its 30 days starting then, for an item the Agent finds in
//     the two holding places that no record names and whose domain or
//     mailbox no longer exists: the folders earlier releases left behind,
//     and one whose record a crash did not let us write. Adoption changes
//     nothing on disk;
//   - removed by the periodic step once its date has passed and its domain or
//     mailbox is gone (a domain still being deleted keeps it), or by the
//     administrator's "Delete now". The Agent derives the folder from the
//     record's kind and identifier and removes exactly that one
//     (cmd/agent/deletion_holding.go); every removal writes an audit row.
//   - A removal that failed is kept with the record as what it is (the Agent
//     answered: failed; no answer: not known), shown with its reason, and
//     tried again after an hour, then less often (at most daily); a status
//     read never removes anything.
//
// Silmelerden kalanlar: 30 gün saklanır, sonra silinir; yönetici Bakım
// sayfasında görür, isterse önce siler.

const (
	holdingKeyPrefix = "deletion_hold_v1:"
	holdingVersion   = 1
	holdingKeep      = 30 * 24 * time.Hour
	holdingTick      = 10 * time.Minute
	holdingRetryMin  = time.Hour
	holdingRetryMax  = 24 * time.Hour

	holdingPath = "/api/v1/maintenance/deletion-holding"

	holdingFailed  = "failed"
	holdingUnknown = "unknown"
)

var holdingIDPattern = regexp.MustCompile(`^(mail\.(domain|mailbox)-[1-9][0-9]{0,17}-[0-9a-f]{16}|backups\.s[1-9][0-9]{0,17}-d[1-9][0-9]{0,17})$`)

// holdingMu keeps the periodic step and "Delete now" from removing at once.
var holdingMu sync.Mutex

// holdingNow is the clock (tests replace it).
var holdingNow = time.Now

type holdingFailure struct {
	// Kind: failed (the Agent answered that it could not remove it) or
	// unknown (no answer came back).
	Kind string `json:"kind"`
	Line string `json:"line"`
	At   string `json:"at"`
}

type holdingRecord struct {
	Version        int    `json:"version"`
	ID             string `json:"id"`
	Kind           string `json:"kind"`
	Name           string `json:"name,omitempty"`
	SubscriptionID int    `json:"subscription_id,omitempty"`
	DomainID       int    `json:"domain_id,omitempty"`
	Domain         string `json:"domain,omitempty"`
	AccountID      int    `json:"account_id,omitempty"`
	Mailbox        string `json:"mailbox,omitempty"`
	// Bytes is -1 when the size could not be read.
	Bytes int64 `json:"bytes"`
	Files int   `json:"files,omitempty"`
	// Path is where the Agent said the item is, for the administrator's
	// eyes only; a removal never uses it.
	Path        string          `json:"path,omitempty"`
	DeletedAt   string          `json:"deleted_at"`
	RemoveAfter string          `json:"remove_after"`
	Adopted     bool            `json:"adopted,omitempty"`
	Attempts    int             `json:"attempts,omitempty"`
	NextTryAt   string          `json:"next_try_at,omitempty"`
	Failure     *holdingFailure `json:"failure,omitempty"`
}

func holdingMailID(name string) string { return "mail." + name }

func holdingBackupsID(subscriptionID, domainID int) string {
	return fmt.Sprintf("backups.s%d-d%d", subscriptionID, domainID)
}

func validHoldingRecord(rec holdingRecord) bool {
	if rec.Version != holdingVersion || !holdingIDPattern.MatchString(rec.ID) {
		return false
	}
	switch rec.Kind {
	case transport.HeldItemMail:
		return rec.ID == holdingMailID(rec.Name)
	case transport.HeldItemBackups:
		return rec.Name == "" && rec.ID == holdingBackupsID(rec.SubscriptionID, rec.DomainID)
	}
	return false
}

func holdingTime(raw string) (time.Time, bool) {
	at, err := time.Parse(time.RFC3339, raw)
	return at, err == nil
}

// holdItem saves a new item, or answers the record already there (a
// deletion pressed again keeps its first date).
func (p *Panel) holdItem(ctx context.Context, rec holdingRecord) (holdingRecord, error) {
	now := holdingNow().UTC()
	rec.Version = holdingVersion
	if rec.DeletedAt == "" {
		rec.DeletedAt = now.Format(time.RFC3339)
	}
	if rec.RemoveAfter == "" {
		rec.RemoveAfter = now.Add(holdingKeep).Format(time.RFC3339)
	}
	if !validHoldingRecord(rec) {
		return rec, fmt.Errorf("hold %s: not a valid holding record", rec.ID)
	}
	raw, err := json.Marshal(rec)
	if err != nil {
		return rec, err
	}
	if _, err := p.db.GetDB().ExecContext(ctx, `
		INSERT INTO panel_settings (key, value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO NOTHING`, holdingKeyPrefix+rec.ID, string(raw)); err != nil {
		return rec, fmt.Errorf("hold %s: %w", rec.ID, err)
	}
	if saved, ok := p.readHoldingRecord(ctx, rec.ID); ok {
		return saved, nil
	}
	return rec, fmt.Errorf("hold %s: the saved record could not be read back", rec.ID)
}

func (p *Panel) readHoldingRecord(ctx context.Context, id string) (holdingRecord, bool) {
	var raw string
	if err := p.db.GetDB().QueryRowContext(ctx, `SELECT value FROM panel_settings WHERE key = ?`, holdingKeyPrefix+id).Scan(&raw); err != nil {
		return holdingRecord{}, false
	}
	return decodeHoldingRecord(id, raw)
}

func decodeHoldingRecord(id, raw string) (holdingRecord, bool) {
	var rec holdingRecord
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&rec) != nil || rec.ID != id || !validHoldingRecord(rec) {
		return holdingRecord{}, false
	}
	return rec, true
}

// readHoldingRecords lists every valid record, oldest removal first. A
// record this release cannot read is logged and left alone.
func (p *Panel) readHoldingRecords(ctx context.Context) ([]holdingRecord, error) {
	rows, err := p.db.GetDB().QueryContext(ctx, `SELECT key, value FROM panel_settings WHERE key LIKE ?`, holdingKeyPrefix+"%")
	if err != nil {
		return nil, fmt.Errorf("read the holding list: %w", err)
	}
	defer rows.Close()
	out := []holdingRecord{}
	for rows.Next() {
		var key, raw string
		if err := rows.Scan(&key, &raw); err != nil {
			return nil, err
		}
		rec, ok := decodeHoldingRecord(strings.TrimPrefix(key, holdingKeyPrefix), raw)
		if !ok {
			log.Printf("[holding] %s: a record this release cannot read is left as it is", key)
			continue
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].RemoveAfter != out[j].RemoveAfter {
			return out[i].RemoveAfter < out[j].RemoveAfter
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (p *Panel) writeHoldingRecord(ctx context.Context, rec holdingRecord) error {
	raw, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	_, err = p.db.GetDB().ExecContext(ctx, `UPDATE panel_settings SET value = ?, updated_at = CURRENT_TIMESTAMP WHERE key = ?`,
		string(raw), holdingKeyPrefix+rec.ID)
	return err
}

// holdingOwnerGone: the item's domain (or mailbox) no longer exists, so
// nothing live can use it. Read only.
func (p *Panel) holdingOwnerGone(ctx context.Context, rec holdingRecord) (bool, error) {
	var n int
	var err error
	if rec.AccountID > 0 {
		err = p.db.GetDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM email_accounts WHERE id = ?`, rec.AccountID).Scan(&n)
	} else {
		err = p.db.GetDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM domains WHERE id = ?`, rec.DomainID).Scan(&n)
	}
	return n == 0, err
}

// holdingDeletedDomainName is the name an audit row gave a deleted domain,
// or "" (an adopted item from an earlier release).
func (p *Panel) holdingDeletedDomainName(ctx context.Context, domainID int) string {
	var action string
	if err := p.db.GetDB().QueryRowContext(ctx, `
		SELECT action FROM audit_logs
		WHERE resource_type = 'domain' AND resource_id = ?
		  AND (action LIKE 'domain.delete:%' OR action LIKE 'domain.delete.pending:%')
		ORDER BY id DESC LIMIT 1`, domainID).Scan(&action); err != nil {
		return ""
	}
	name := strings.TrimPrefix(strings.TrimPrefix(action, "domain.delete.pending:"), "domain.delete:")
	if index := strings.IndexByte(name, ':'); index >= 0 {
		name = name[:index]
	}
	return name
}

// holdingAgentInventory and holdingAgentRemove are the two Agent calls
// (tests replace them).
var holdingAgentInventory = func(p *Panel, ctx context.Context) (transport.DeletionHoldingInventoryResponse, error) {
	var resp transport.DeletionHoldingInventoryResponse
	err := p.callAgentContext(ctx, "Agent.DeletionHoldingInventoryV1", &transport.DeletionHoldingInventoryRequest{}, &resp)
	return resp, err
}

var holdingAgentRemove = func(p *Panel, ctx context.Context, req transport.RemoveHeldItemRequest) (transport.RemoveHeldItemResponse, error) {
	var resp transport.RemoveHeldItemResponse
	err := p.callAgentContext(ctx, "Agent.RemoveHeldItemV1", &req, &resp)
	return resp, err
}

// adoptHeldItems takes into the list what the Agent finds that no record
// names and whose domain or mailbox is gone; it drops the record of an item
// that is no longer there (removed on the server by its owner), when that
// place could be read. Nothing on disk changes.
func (p *Panel) adoptHeldItems(ctx context.Context) error {
	inventory, err := holdingAgentInventory(p, ctx)
	if err != nil {
		return fmt.Errorf("read what is held: %w", err)
	}
	records, err := p.readHoldingRecords(ctx)
	if err != nil {
		return err
	}
	known := map[string]holdingRecord{}
	for _, rec := range records {
		known[rec.ID] = rec
	}
	found := map[string]bool{}
	for _, item := range inventory.Items {
		rec := holdingRecord{Kind: item.Kind, Bytes: item.Bytes, Files: item.Files, Path: item.Path, Adopted: true}
		if item.Unknown {
			rec.Bytes = -1
		}
		switch item.Kind {
		case transport.HeldItemMail:
			rec.ID, rec.Name, rec.DomainID, rec.AccountID = holdingMailID(item.Name), item.Name, item.DomainID, item.AccountID
		case transport.HeldItemBackups:
			rec.ID, rec.SubscriptionID, rec.DomainID = holdingBackupsID(item.SubscriptionID, item.DomainID), item.SubscriptionID, item.DomainID
		default:
			continue
		}
		found[rec.ID] = true
		if old, ok := known[rec.ID]; ok {
			// The size as it is now.
			if old.Bytes != rec.Bytes || old.Files != rec.Files {
				old.Bytes, old.Files = rec.Bytes, rec.Files
				if err := p.writeHoldingRecord(ctx, old); err != nil {
					log.Printf("[holding] %s: size not updated: %v", old.ID, err)
				}
			}
			continue
		}
		gone, err := p.holdingOwnerGone(ctx, rec)
		if err != nil {
			return err
		}
		if !gone {
			// A live domain's backups, or a mailbox that still exists: not a
			// leftover of a deletion.
			continue
		}
		if rec.DomainID > 0 {
			rec.Domain = p.holdingDeletedDomainName(ctx, rec.DomainID)
		}
		if _, err := p.holdItem(ctx, rec); err != nil {
			return err
		}
		p.recordAudit(auditRecord{Action: "deletion_hold.adopt:" + rec.ID, ResourceType: "domain", ResourceID: rec.DomainID})
	}
	for _, rec := range records {
		if found[rec.ID] {
			continue
		}
		if (rec.Kind == transport.HeldItemMail && inventory.MailError != "") ||
			(rec.Kind == transport.HeldItemBackups && inventory.BackupsError != "") {
			continue
		}
		if _, err := p.db.GetDB().ExecContext(ctx, `DELETE FROM panel_settings WHERE key = ?`, holdingKeyPrefix+rec.ID); err != nil {
			return err
		}
		p.recordAudit(auditRecord{Action: "deletion_hold.gone:" + rec.ID, ResourceType: "domain", ResourceID: rec.DomainID})
	}
	return nil
}

// holdingRemoval is how one removal ended.
type holdingRemoval struct {
	Removed bool
	// Failure: why it did not; the record keeps it.
	Failure *holdingFailure
}

// removeHeldItem removes one item, now. r is the administrator's request
// ("Delete now"), nil for the periodic step. The caller holds holdingMu.
func (p *Panel) removeHeldItem(ctx context.Context, r *http.Request, rec holdingRecord) (holdingRemoval, error) {
	gone, err := p.holdingOwnerGone(ctx, rec)
	if err != nil {
		return holdingRemoval{}, err
	}
	if !gone {
		return holdingRemoval{Failure: &holdingFailure{Kind: holdingFailed, Line: "its domain or mailbox still exists (a deletion that has not finished); it is kept until that deletion finishes"}}, nil
	}
	req := transport.RemoveHeldItemRequest{
		ExpectedBuildCommit: strings.TrimSpace(buildCommit), Kind: rec.Kind, Name: rec.Name,
		SubscriptionID: rec.SubscriptionID, DomainID: rec.DomainID,
	}
	if rec.Kind == transport.HeldItemMail {
		req.SubscriptionID, req.DomainID = 0, 0
	}
	resp, callErr := holdingAgentRemove(p, ctx, req)
	now := holdingNow().UTC()
	details := map[string]any{"kind": rec.Kind, "domain": rec.Domain, "mailbox": rec.Mailbox, "bytes": rec.Bytes, "by": "schedule"}
	if r != nil {
		details["by"] = "administrator"
	}
	var failure *holdingFailure
	var serverErr rpc.ServerError
	switch {
	case callErr != nil && errors.As(callErr, &serverErr):
		failure = &holdingFailure{Kind: holdingFailed, Line: boundedDeletionErrorLine(callErr.Error())}
	case callErr != nil:
		failure = &holdingFailure{Kind: holdingUnknown, Line: boundedDeletionErrorLine(callErr.Error())}
	case !resp.Removed:
		line := strings.TrimSpace(resp.Error)
		if line == "" {
			line = "the server did not confirm the removal"
		}
		failure = &holdingFailure{Kind: holdingFailed, Line: boundedDeletionErrorLine(line)}
	}
	if failure != nil {
		failure.At = now.Format(time.RFC3339)
		rec.Failure = failure
		rec.Attempts++
		wait := holdingRetryMin << min(rec.Attempts-1, 5)
		if wait > holdingRetryMax {
			wait = holdingRetryMax
		}
		rec.NextTryAt = now.Add(wait).Format(time.RFC3339)
		if err := p.writeHoldingRecord(ctx, rec); err != nil {
			log.Printf("[holding] %s: the failure could not be kept: %v", rec.ID, err)
		}
		details["outcome"], details["error"] = failure.Kind, failure.Line
		action := "deletion_hold.remove.failed:"
		if failure.Kind == holdingUnknown {
			action = "deletion_hold.remove.unknown:"
		}
		p.holdingAudit(r, action+rec.ID, rec, details)
		return holdingRemoval{Failure: failure}, nil
	}
	if _, err := p.db.GetDB().ExecContext(ctx, `DELETE FROM panel_settings WHERE key = ?`, holdingKeyPrefix+rec.ID); err != nil {
		return holdingRemoval{Removed: true}, fmt.Errorf("the item was removed, its record not: %w", err)
	}
	details["outcome"], details["absent"] = "removed", resp.Absent
	p.holdingAudit(r, "deletion_hold.remove:"+rec.ID, rec, details)
	return holdingRemoval{Removed: true}, nil
}

func (p *Panel) holdingAudit(r *http.Request, action string, rec holdingRecord, details map[string]any) {
	if r != nil {
		p.auditDetailed(r, action, "domain", rec.DomainID, details)
		return
	}
	raw, _ := json.Marshal(details)
	p.recordAudit(auditRecord{Action: action, ResourceType: "domain", ResourceID: rec.DomainID, Details: string(raw)})
}

// runHoldingStep adopts, then removes every due item whose owner is gone
// and whose next try has come.
func (p *Panel) runHoldingStep(ctx context.Context) {
	holdingMu.Lock()
	defer holdingMu.Unlock()
	if err := p.adoptHeldItems(ctx); err != nil {
		log.Printf("[holding] adoption: %v", err)
	}
	records, err := p.readHoldingRecords(ctx)
	if err != nil {
		log.Printf("[holding] %v", err)
		return
	}
	now := holdingNow().UTC()
	for _, rec := range records {
		due, ok := holdingTime(rec.RemoveAfter)
		if !ok || now.Before(due) {
			continue
		}
		if next, ok := holdingTime(rec.NextTryAt); ok && now.Before(next) {
			continue
		}
		if gone, err := p.holdingOwnerGone(ctx, rec); err != nil || !gone {
			continue
		}
		if _, err := p.removeHeldItem(ctx, nil, rec); err != nil {
			log.Printf("[holding] %s: %v", rec.ID, err)
		}
	}
}

// startDeletionHolding runs the step at start (the first adoption) and every
// ten minutes.
func (p *Panel) startDeletionHolding() {
	go func() {
		for {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			p.runHoldingStep(ctx)
			cancel()
			time.Sleep(holdingTick)
		}
	}()
}

// holdingSummary is the list's total: how many, how large, the first date.
type holdingSummary struct {
	Count int `json:"count"`
	// Bytes counts the items whose size is known.
	Bytes          int64  `json:"bytes"`
	EarliestRemove string `json:"earliest_remove_after,omitempty"`
}

func summarizeHolding(records []holdingRecord) holdingSummary {
	out := holdingSummary{Count: len(records)}
	for _, rec := range records {
		if rec.Bytes > 0 {
			out.Bytes += rec.Bytes
		}
		if out.EarliestRemove == "" || rec.RemoveAfter < out.EarliestRemove {
			out.EarliestRemove = rec.RemoveAfter
		}
	}
	return out
}

// holdingItemView is one row of the list.
type holdingItemView struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Domain      string `json:"domain,omitempty"`
	DomainID    int    `json:"domain_id,omitempty"`
	Mailbox     string `json:"mailbox,omitempty"`
	Bytes       int64  `json:"bytes"`
	Files       int    `json:"files,omitempty"`
	DeletedAt   string `json:"deleted_at"`
	RemoveAfter string `json:"remove_after"`
	Adopted     bool   `json:"adopted,omitempty"`
	Path        string `json:"path,omitempty"`
	// State: kept (until its date), waiting (its domain's deletion has not
	// finished), failed or unknown (the last removal; tried again at
	// next_try_at).
	State     string          `json:"state"`
	Failure   *holdingFailure `json:"failure,omitempty"`
	NextTryAt string          `json:"next_try_at,omitempty"`
}

// handleDeletionHolding: GET the list (administrator only). Reads only.
func (p *Panel) handleDeletionHolding(w http.ResponseWriter, r *http.Request) {
	if !requireMaintenanceAdmin(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		rejectRouteMethod(w, []string{http.MethodGet})
		return
	}
	records, err := p.readHoldingRecords(r.Context())
	if err != nil {
		writeServerError(w, err)
		return
	}
	items := make([]holdingItemView, 0, len(records))
	for _, rec := range records {
		view := holdingItemView{ID: rec.ID, Kind: rec.Kind, Domain: rec.Domain, DomainID: rec.DomainID, Mailbox: rec.Mailbox,
			Bytes: rec.Bytes, Files: rec.Files, DeletedAt: rec.DeletedAt, RemoveAfter: rec.RemoveAfter, Adopted: rec.Adopted,
			Path: rec.Path, State: "kept", Failure: rec.Failure, NextTryAt: rec.NextTryAt}
		if rec.Failure != nil {
			view.State = rec.Failure.Kind
		} else if gone, err := p.holdingOwnerGone(r.Context(), rec); err == nil && !gone {
			view.State = "waiting"
		}
		items = append(items, view)
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"items": items, "summary": summarizeHolding(records),
		"read_at": holdingNow().UTC().Format(time.RFC3339)})
}

// handleDeletionHoldingItem: DELETE removes one item now; GET .../backups
// lists a backups item's files; GET .../download?name= downloads one.
func (p *Panel) handleDeletionHoldingItem(w http.ResponseWriter, r *http.Request) {
	if !requireMaintenanceAdmin(w, r) {
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, holdingPath+"/")
	id, sub, _ := strings.Cut(rest, "/")
	if !holdingIDPattern.MatchString(id) {
		writeClientError(w, http.StatusNotFound, "invalid request")
		return
	}
	rec, ok := p.readHoldingRecord(r.Context(), id)
	switch {
	case sub == "" && r.Method == http.MethodDelete:
		if !ok {
			// Already gone: the list is read again.
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "removed", "absent": true})
			return
		}
		holdingMu.Lock()
		defer holdingMu.Unlock()
		// Read again under the lock: the periodic step may have finished it.
		if rec, ok = p.readHoldingRecord(r.Context(), id); !ok {
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "removed", "absent": true})
			return
		}
		outcome, err := p.removeHeldItem(r.Context(), r, rec)
		if err != nil {
			writeServerError(w, err)
			return
		}
		if outcome.Failure != nil {
			code := "HOLDING_REMOVE_FAILED"
			status := http.StatusConflict
			if outcome.Failure.Kind == holdingUnknown {
				code, status = "HOLDING_REMOVE_UNKNOWN", http.StatusBadGateway
			}
			writeCodedErrorVars(w, status, code,
				"It was not removed; the reason is kept with the item in the list (Details). It stays in the list and is tried again by itself; press Delete now again after the cause is fixed.",
				"", nil, map[string]string{"line": outcome.Failure.Line})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "removed"})
	case sub == "backups" && r.Method == http.MethodGet:
		if !ok || rec.Kind != transport.HeldItemBackups {
			writeClientError(w, http.StatusNotFound, "invalid request")
			return
		}
		var resp backupspec.ListResponse
		if err := p.callAgentContext(r.Context(), "Agent.ListBackups", &backupspec.ListRequest{
			ProtocolVersion: backupspec.ProtocolVersion, SubscriptionID: rec.SubscriptionID, DomainID: rec.DomainID,
		}, &resp); err != nil {
			writeAgentError(w, err, "")
			return
		}
		files := []backupspec.Info{}
		for _, item := range resp.Backups {
			if !item.Legacy {
				files = append(files, item)
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"backups": files})
	case sub == "download" && r.Method == http.MethodGet:
		if !ok || rec.Kind != transport.HeldItemBackups {
			writeClientError(w, http.StatusNotFound, "invalid request")
			return
		}
		// No domain name: only the backup store's own files of this scope,
		// never a folder named after a domain that may exist again.
		p.streamBackupDownload(w, r, backupDomain{ID: rec.DomainID, SubscriptionID: rec.SubscriptionID})
	default:
		rejectRouteMethod(w, []string{http.MethodGet, http.MethodDelete})
	}
}

// holdingForDashboard is the administrator home page's one line, or nil.
func (p *Panel) holdingForDashboard(ctx context.Context) *holdingSummary {
	records, err := p.readHoldingRecords(ctx)
	if err != nil || len(records) == 0 {
		return nil
	}
	summary := summarizeHolding(records)
	return &summary
}

// holdingKeptView is what a deletion's answer says about one held item.
type holdingKeptView struct {
	Bytes       int64  `json:"bytes"`
	Files       int    `json:"files,omitempty"`
	RemoveAfter string `json:"remove_after"`
	// Path: for the administrator only.
	Path string `json:"path,omitempty"`
}

func holdingKept(rec holdingRecord, admin bool) *holdingKeptView {
	view := &holdingKeptView{Bytes: rec.Bytes, Files: rec.Files, RemoveAfter: rec.RemoveAfter}
	if admin {
		view.Path = rec.Path
	}
	return view
}

func callerIsAdmin(r *http.Request) bool {
	caller := currentCaller(r)
	return caller != nil && caller.hasAccountRole(roleAdmin)
}

// holdDomainDeletionItem saves what a deletion put aside and answers what
// its result says about it. A record that could not be saved is logged; the
// next periodic step adopts the item (its 30 days then start there).
func (p *Panel) holdDomainDeletionItem(ctx context.Context, r *http.Request, rec holdingRecord) *holdingKeptView {
	saved, err := p.holdItem(ctx, rec)
	if err != nil {
		log.Printf("[holding] %s: %v", rec.ID, err)
		saved = rec
		saved.RemoveAfter = holdingNow().UTC().Add(holdingKeep).Format(time.RFC3339)
	}
	return holdingKept(saved, callerIsAdmin(r))
}
