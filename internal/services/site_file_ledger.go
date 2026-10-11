package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// The managed site files ledger (migration 044, D-031). The file's header is
// the authority for "is this still CelikPanel's text"; a row records what the
// Panel last wrote, what it last observed and the owner's last decision. A row
// is re-created from the next render when a restore lost it.
//
// Yönetilen site dosyaları defteri. Dosyanın başlığı belirleyicidir; satır
// Panel'in en son ne yazdığını, ne gözlemlediğini ve sahibin kararını tutar.

// SiteFileDecisionKeepMine and the others are the owner's decisions.
const (
	SiteFileDecisionKeepMine = "keep_mine"
	SiteFileDecisionTake     = "take_celikpanel"
	SiteFileDecisionRecreate = "recreate"
)

// The ledger's certificate reasons for a kept file (D-031 step 1b). They are
// kept in state_reason while the file stays kept: the operations that render
// the file again (a start, a setting) do not erase them.
const (
	// SiteFileReasonCertificate: a new certificate was obtained and installed
	// in the Panel's certificate store, and the kept file does not name it;
	// the site keeps serving the certificate the file names.
	SiteFileReasonCertificate = "certificate"
	// SiteFileReasonCertificateValidation: a certificate could not be
	// requested or renewed, because the kept file does not let the
	// validation be published (transport.SiteFileValidation*).
	SiteFileReasonCertificateValidation = "certificate_validation"
)

// SiteFileReasonProtectionRemoved is stored with managed_unchanged when the
// last render's replacement was refused (write_refused) and a later read of
// the domain screen ("check again") measured no protection on the same bytes.
// CelikPanel's held text is then written at the next start, the next change
// saved for the site, or the owner's "apply"; that write clears the reason.
const SiteFileReasonProtectionRemoved = "protection_removed"

// siteFileStatePrecedence is the one order in which the facts about a site's
// file are shown when two of them hold for the same bytes (the live
// classification and the ledger's refused replacement): the domain screen,
// the domains list row and its badge show the higher one. unreadable >
// unwritable > missing/absent > owner_edited/foreign/unknown_origin >
// unknown > managed_unchanged. A file the Agent did not read ("unknown",
// reason not_read or agent_does_not_report) is not known to be CelikPanel's
// unchanged text, so it outranks it (2026-10-11 correction: it used to rank
// lowest and a site whose pool was not read showed "unchanged"); every
// observed problem still outranks it.
var siteFileStatePrecedence = map[string]int{
	transport.SiteFileUnreadable:       6,
	transport.SiteFileUnwritable:       5,
	transport.SiteFileMissingState:     4,
	transport.SiteFileAbsent:           4,
	transport.SiteFileOwnerEdited:      3,
	transport.SiteFileForeign:          3,
	transport.SiteFileUnknownOrigin:    3,
	transport.SiteFileStateUnknown:     2,
	transport.SiteFileManagedUnchanged: 1,
}

// SiteFileShownState answers the state shown for two facts about the same
// file: the higher one in siteFileStatePrecedence (the first on a tie).
func SiteFileShownState(first, second string) string {
	if siteFileStatePrecedence[second] > siteFileStatePrecedence[first] {
		return second
	}
	return first
}

// SiteFileWorstState is the state shown for a site whose files have the given
// (presented) states: the highest in siteFileStatePrecedence, the first on a
// tie (D-031 second step: the vhost, then the pool, then the unit). It
// returns the index of the file that decided it, -1 for none.
func SiteFileWorstState(states ...string) (string, int) {
	shown, index := "", -1
	for position, state := range states {
		if index < 0 || SiteFileShownState(shown, state) != shown {
			shown, index = state, position
		}
	}
	return shown, index
}

// SiteFileCertificateReason says whether a stored reason is one of them.
func SiteFileCertificateReason(reason string) bool {
	return reason == SiteFileReasonCertificate || reason == SiteFileReasonCertificateValidation
}

func siteFileStoredKept(state string) bool {
	switch state {
	case transport.SiteFileOwnerEdited, transport.SiteFileForeign, transport.SiteFileUnknownOrigin:
		return true
	}
	return false
}

// SiteFileRecord is one ledger row.
type SiteFileRecord struct {
	SiteID             int
	DomainID           int
	Kind               string
	Path               string
	FileFormat         string
	BodySHA256         string
	WrittenRelease     string
	WrittenAt          int64
	AdoptedFrom        string
	State              string
	StateReason        string
	FileSHA256         string
	ObservedAt         int64
	PendingPath        string
	PendingSHA256      string
	BackupPath         string
	Decision           string
	DecisionFileSHA256 string
	DecidedBy          int64
	DecidedAt          int64
}

type siteFileQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

const siteFileRecordColumns = `site_id, domain_id, kind, path, file_format, body_sha256,
	written_release, written_at, adopted_from, state, state_reason, file_sha256,
	observed_at, pending_path, pending_sha256, backup_path, decision,
	decision_file_sha256, COALESCE(decided_by, 0), decided_at`

func scanSiteFileRecord(row *sql.Row) (SiteFileRecord, error) {
	var record SiteFileRecord
	err := row.Scan(&record.SiteID, &record.DomainID, &record.Kind, &record.Path,
		&record.FileFormat, &record.BodySHA256, &record.WrittenRelease, &record.WrittenAt,
		&record.AdoptedFrom, &record.State, &record.StateReason, &record.FileSHA256,
		&record.ObservedAt, &record.PendingPath, &record.PendingSHA256, &record.BackupPath,
		&record.Decision, &record.DecisionFileSHA256, &record.DecidedBy, &record.DecidedAt)
	return record, err
}

// LoadSiteFileRecord reads the newest row of one kind for a site.
func LoadSiteFileRecord(ctx context.Context, db siteFileQuerier, siteID int, kind string) (SiteFileRecord, bool, error) {
	record, err := scanSiteFileRecord(db.QueryRowContext(ctx, `SELECT `+siteFileRecordColumns+`
		FROM managed_site_files WHERE site_id = ? AND kind = ? ORDER BY id DESC LIMIT 1`, siteID, kind))
	if errors.Is(err, sql.ErrNoRows) {
		return SiteFileRecord{}, false, nil
	}
	if err != nil {
		return SiteFileRecord{}, false, err
	}
	return record, true, nil
}

// SiteFileStoredState is what the ledger stores after an operation: the state
// of the file as it is now.
func SiteFileStoredState(result transport.SiteFileResult) string {
	switch result.Outcome {
	case transport.SiteFileOutcomeWritten, transport.SiteFileOutcomeUnchanged,
		transport.SiteFileOutcomeRecreated, transport.SiteFileOutcomeTaken:
		return transport.SiteFileManagedUnchanged
	case transport.SiteFileOutcomeMissing:
		return transport.SiteFileMissingState
	}
	if result.Reason == transport.SiteFileReasonWriteRefused {
		// The file could not be replaced (an immutable file): it was kept.
		// Migration 044's state set has no "unwritable"; the reason
		// write_refused tells it apart and SiteFilePresentedState answers it.
		return transport.SiteFileUnreadable
	}
	switch result.State {
	case transport.SiteFileAbsent, transport.SiteFileManagedUnchanged, transport.SiteFileOwnerEdited,
		transport.SiteFileForeign, transport.SiteFileUnreadable, transport.SiteFileUnknownOrigin:
		return result.State
	}
	return transport.SiteFileStateUnknown
}

// SiteFilePresentedState is the state the domain screen and the domains list
// answer for a stored state and reason: a file that was read but whose
// replacement was refused is "unwritable", never "unreadable" (set10
// scenario g). Every other stored state is answered as stored.
func SiteFilePresentedState(state, reason string) string {
	if state == transport.SiteFileUnreadable && reason == transport.SiteFileReasonWriteRefused {
		return transport.SiteFileUnwritable
	}
	return state
}

// RecordSiteFileResult stores an Agent's typed answer for one file. now is a
// Unix time; 0 takes the clock.
func RecordSiteFileResult(
	ctx context.Context,
	db siteFileQuerier,
	siteID, domainID int,
	result transport.SiteFileResult,
	release string,
	now int64,
) error {
	if result.Path == "" {
		return fmt.Errorf("site file result has no path")
	}
	if now == 0 {
		now = time.Now().Unix()
	}
	kind := result.Kind
	if kind == "" {
		kind = transport.SiteFileKindNginxVhost
	}
	previous, found, err := LoadSiteFileRecord(ctx, db, siteID, kind)
	if err != nil {
		return err
	}
	record := previous
	if !found || previous.Path != result.Path {
		record = SiteFileRecord{SiteID: siteID, DomainID: domainID, Kind: kind, Path: result.Path}
	}
	record.State = SiteFileStoredState(result)
	record.StateReason = result.Reason
	// A certificate reason stays while the file stays kept and no other
	// reason replaces it. The certificate one ends when the kept file names
	// the certificate (the owner updated its lines).
	if found && previous.Path == result.Path && result.Reason == "" && siteFileStoredKept(record.State) &&
		SiteFileCertificateReason(previous.StateReason) {
		record.StateReason = previous.StateReason
		if previous.StateReason == SiteFileReasonCertificate && result.CertificateReferenced {
			record.StateReason = ""
		}
	}
	record.FileSHA256 = result.FileSHA256
	record.ObservedAt = now
	record.PendingPath = result.PendingPath
	record.PendingSHA256 = result.PendingSHA256
	if result.BackupPath != "" {
		record.BackupPath = result.BackupPath
	}
	switch result.Outcome {
	case transport.SiteFileOutcomeWritten, transport.SiteFileOutcomeRecreated, transport.SiteFileOutcomeTaken:
		record.FileFormat = ManagedRenderFormat
		record.BodySHA256 = result.WrittenSHA256
		record.WrittenRelease = release
		record.WrittenAt = now
		if result.AdoptedFrom != "" {
			record.AdoptedFrom = result.AdoptedFrom
		}
	case transport.SiteFileOutcomeUnchanged:
		record.FileFormat = ManagedRenderFormat
		if result.WrittenSHA256 != "" {
			record.BodySHA256 = result.WrittenSHA256
		}
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO managed_site_files (site_id, domain_id, kind, path, file_format, body_sha256,
			written_release, written_at, adopted_from, state, state_reason, file_sha256,
			observed_at, pending_path, pending_sha256, backup_path, decision,
			decision_file_sha256, decided_by, decided_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULLIF(?, 0), ?)
		ON CONFLICT(site_id, kind, path) DO UPDATE SET
			domain_id = excluded.domain_id, file_format = excluded.file_format,
			body_sha256 = excluded.body_sha256, written_release = excluded.written_release,
			written_at = excluded.written_at, adopted_from = excluded.adopted_from,
			state = excluded.state, state_reason = excluded.state_reason,
			file_sha256 = excluded.file_sha256, observed_at = excluded.observed_at,
			pending_path = excluded.pending_path, pending_sha256 = excluded.pending_sha256,
			backup_path = excluded.backup_path`,
		record.SiteID, domainID, record.Kind, record.Path, record.FileFormat, record.BodySHA256,
		record.WrittenRelease, record.WrittenAt, record.AdoptedFrom, record.State, record.StateReason,
		record.FileSHA256, record.ObservedAt, record.PendingPath, record.PendingSHA256, record.BackupPath,
		record.Decision, record.DecisionFileSHA256, record.DecidedBy, record.DecidedAt)
	return err
}

// RecordSiteFileUnknown says that the Agent did not report the file's state:
// an older Agent, an Agent that could not be asked, or a render input the
// Panel could not prepare. Nothing else in the row changes.
func RecordSiteFileUnknown(ctx context.Context, db siteFileQuerier, siteID, domainID int, path, reason string, now int64) error {
	return RecordSiteFileUnknownKind(ctx, db, siteID, domainID, transport.SiteFileKindNginxVhost, path, reason, now)
}

// RecordSiteFileUnknownKind is RecordSiteFileUnknown for a file of any kind
// (D-031 second step: the pool and the application unit).
func RecordSiteFileUnknownKind(ctx context.Context, db siteFileQuerier, siteID, domainID int, kind, path, reason string, now int64) error {
	if now == 0 {
		now = time.Now().Unix()
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO managed_site_files (site_id, domain_id, kind, path, state, state_reason, observed_at)
		VALUES (?, ?, ?, ?, 'unknown', ?, ?)
		ON CONFLICT(site_id, kind, path) DO UPDATE SET
			state = 'unknown', state_reason = excluded.state_reason, observed_at = excluded.observed_at`,
		siteID, domainID, kind, path, reason, now)
	return err
}

// SiteFileWriteRefused says whether a row's last observation is a refused
// replacement (presented "unwritable").
func SiteFileWriteRefused(record SiteFileRecord) bool {
	return SiteFilePresentedState(record.State, record.StateReason) == transport.SiteFileUnwritable
}

// RecordSiteFileRecheck stores what the domain screen's "check again" read
// on a row whose last observation is a refused replacement: the state and
// reason to store (migration 044's state set), the file's digest and the
// time. Nothing else in the row changes; a row that is not (or no longer) a
// refused replacement is left alone. It returns whether the row changed.
func RecordSiteFileRecheck(ctx context.Context, db siteFileQuerier, siteID int, kind, path, state, reason, fileSHA256 string, now int64) (bool, error) {
	switch state {
	case transport.SiteFileManagedUnchanged, transport.SiteFileOwnerEdited, transport.SiteFileForeign,
		transport.SiteFileUnknownOrigin, transport.SiteFileUnreadable, transport.SiteFileMissingState:
	default:
		return false, fmt.Errorf("not a state a recheck stores: %q", state)
	}
	if now == 0 {
		now = time.Now().Unix()
	}
	result, err := db.ExecContext(ctx, `
		UPDATE managed_site_files SET state = ?, state_reason = ?, file_sha256 = ?, observed_at = ?
		WHERE site_id = ? AND kind = ? AND path = ? AND state = ? AND state_reason = ?`,
		state, reason, fileSHA256, now, siteID, kind, path,
		transport.SiteFileUnreadable, transport.SiteFileReasonWriteRefused)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected > 0, err
}

// RecordSiteFileDecision stores the owner's decision and the digest of the
// file the decision was made on.
func RecordSiteFileDecision(ctx context.Context, db siteFileQuerier, siteID int, kind, path, decision, fileSHA256 string, actor int, now int64) error {
	if now == 0 {
		now = time.Now().Unix()
	}
	result, err := db.ExecContext(ctx, `
		UPDATE managed_site_files SET decision = ?, decision_file_sha256 = ?, decided_by = NULLIF(?, 0), decided_at = ?
		WHERE site_id = ? AND kind = ? AND path = ?`,
		decision, fileSHA256, actor, now, siteID, kind, path)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return fmt.Errorf("no ledger row for site %d %s", siteID, kind)
	}
	return nil
}

// SetSiteFileCertificateReason records (or, with "", clears) a certificate
// reason on the site's vhost row. Clearing touches only a certificate reason.
// It returns whether a row changed.
func SetSiteFileCertificateReason(ctx context.Context, db siteFileQuerier, siteID int, reason string) (bool, error) {
	var (
		result sql.Result
		err    error
	)
	if reason == "" {
		result, err = db.ExecContext(ctx, `
			UPDATE managed_site_files SET state_reason = ''
			WHERE site_id = ? AND kind = ? AND state_reason IN (?, ?)`,
			siteID, transport.SiteFileKindNginxVhost, SiteFileReasonCertificate, SiteFileReasonCertificateValidation)
	} else {
		if !SiteFileCertificateReason(reason) {
			return false, fmt.Errorf("not a certificate reason: %q", reason)
		}
		result, err = db.ExecContext(ctx, `
			UPDATE managed_site_files SET state_reason = ?
			WHERE site_id = ? AND kind = ?`,
			reason, siteID, transport.SiteFileKindNginxVhost)
	}
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected > 0, err
}
