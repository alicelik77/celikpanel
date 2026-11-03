package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"io"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/backupspec"
	"github.com/alicelik/celikpanel/internal/remotebackup"
)

// The remote backup worker. One goroutine moves every operation, one at a
// time, oldest first: a screen that polls only reads remote_backup_operations
// and can never start a second upload. An operation interrupted by a restart
// is continued by the same worker under the same id; the local backup a
// schedule makes for it has the job key "remote:<operation id>", so the
// Agent hands back the same backup instead of making another.
//
// Uzak yedek işçisi: tek goroutine, sırayla. Ekran yalnız okur; ikinci yükleme
// başlatamaz. Yeniden başlatmada aynı işlem aynı kimlikle sürer.

const (
	remoteOpQueued    = "queued"
	remoteOpRunning   = "running"
	remoteOpSucceeded = "succeeded"
	remoteOpFailed    = "failed"

	remoteFailureVerified     = "verified_failure"
	remoteFailurePrerequisite = "prerequisite"
	remoteFailureUnknown      = "unknown"

	remoteAutoAttempts      = 3
	remoteRetryDelay        = 10 * time.Minute
	remoteUploadTimeout     = 6 * time.Hour
	remoteJobKeyPrefix      = "remote:"
	remoteProgressEvery     = 2 * time.Second
	remotePolicyCheckEvery  = 30 * time.Minute
	remoteWorkerTick        = time.Minute
	remoteRecentOperations  = 20
	remoteImportChunk       = backupspec.MaxChunkBytes
	remoteEncryptedOverhead = 64 << 10
)

// remoteBackupWake nudges the worker after a new operation is queued.
var remoteBackupWake = make(chan struct{}, 1)

func wakeRemoteBackupWorker() {
	select {
	case remoteBackupWake <- struct{}{}:
	default:
	}
}

type remoteOperation struct {
	ID             string
	Kind           string
	DestinationID  int64
	DomainID       int
	SubscriptionID int
	PolicyID       int64
	BackupName     string
	BackupType     string
	ObjectKey      string
	Transient      bool
	State          string
	Phase          string
	BytesDone      int64
	BytesTotal     int64
	Attempts       int
	NextAttemptAt  string
	FailureClass   string
	FailureCode    string
	FailureDetail  string
	Verified       string
	RequestedBy    int64
	CreatedAt      string
	FinishedAt     string
}

const remoteOperationColumns = `id, kind, COALESCE(destination_id, 0), domain_id, subscription_id, COALESCE(policy_id, 0),
	backup_name, backup_type, object_key, transient, state, phase, bytes_done, bytes_total, attempts,
	COALESCE(next_attempt_at, ''), failure_class, failure_code, failure_detail, verified,
	COALESCE(requested_by, 0), created_at, COALESCE(finished_at, '')`

func scanRemoteOperation(row interface{ Scan(...any) error }) (remoteOperation, error) {
	var op remoteOperation
	var transient int
	err := row.Scan(&op.ID, &op.Kind, &op.DestinationID, &op.DomainID, &op.SubscriptionID, &op.PolicyID,
		&op.BackupName, &op.BackupType, &op.ObjectKey, &transient, &op.State, &op.Phase, &op.BytesDone,
		&op.BytesTotal, &op.Attempts, &op.NextAttemptAt, &op.FailureClass, &op.FailureCode, &op.FailureDetail,
		&op.Verified, &op.RequestedBy, &op.CreatedAt, &op.FinishedAt)
	op.Transient = transient == 1
	return op, err
}

func (p *Panel) loadRemoteOperation(ctx context.Context, id string) (remoteOperation, error) {
	return scanRemoteOperation(p.db.GetDB().QueryRowContext(ctx,
		`SELECT `+remoteOperationColumns+` FROM remote_backup_operations WHERE id = ?`, id))
}

// remoteOperationRequest is one operation to queue.
type remoteOperationRequest struct {
	Kind            string
	DestinationID   int64
	Site            remoteDomain
	PolicyID        int64
	BackupName      string
	BackupType      string
	ObjectKey       string
	Transient       bool
	RequestedBy     int64
	RequestIdentity string
}

// queueRemoteOperation records an operation, or returns the one already
// recorded for the same request identity or the same work still waiting or
// running (created == false then).
func (p *Panel) queueRemoteOperation(ctx context.Context, req remoteOperationRequest) (string, bool, error) {
	db := p.db.GetDB()
	if req.RequestIdentity != "" {
		var id string
		err := db.QueryRowContext(ctx, `SELECT id FROM remote_backup_operations WHERE request_identity = ?`, req.RequestIdentity).Scan(&id)
		if err == nil {
			return id, false, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return "", false, err
		}
	}
	var existing string
	err := db.QueryRowContext(ctx, `
		SELECT id FROM remote_backup_operations
		WHERE kind = ? AND COALESCE(destination_id, 0) = ? AND domain_id = ? AND COALESCE(policy_id, 0) = ?
		  AND backup_name = ? AND object_key = ? AND state IN ('queued', 'running')
		ORDER BY created_at LIMIT 1`,
		req.Kind, req.DestinationID, req.Site.ID, req.PolicyID, req.BackupName, req.ObjectKey).Scan(&existing)
	if err == nil {
		return existing, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", false, err
	}
	id, err := newRemoteBackupOperationID()
	if err != nil {
		return "", false, err
	}
	var identity, policy, requestedBy any
	if req.RequestIdentity != "" {
		identity = req.RequestIdentity
	}
	if req.PolicyID > 0 {
		policy = req.PolicyID
	}
	if req.RequestedBy > 0 {
		requestedBy = req.RequestedBy
	}
	transient := 0
	if req.Transient {
		transient = 1
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO remote_backup_operations
		    (id, kind, destination_id, domain_id, subscription_id, policy_id, backup_name, backup_type,
		     object_key, transient, state, phase, requested_by, request_identity)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'queued', 'waiting', ?, ?)`,
		id, req.Kind, req.DestinationID, req.Site.ID, req.Site.SubscriptionID, policy, req.BackupName,
		req.BackupType, req.ObjectKey, transient, requestedBy, identity)
	if err != nil {
		return "", false, err
	}
	wakeRemoteBackupWorker()
	return id, true, nil
}

// ----- the worker -----

var remoteBackupWorkerOnce sync.Once

func (p *Panel) startRemoteBackupWorker() {
	remoteBackupWorkerOnce.Do(func() {
		go func() {
			ctx := context.Background()
			p.recoverInterruptedRemoteOperations(ctx)
			ticker := time.NewTicker(remoteWorkerTick)
			defer ticker.Stop()
			lastPolicyCheck := time.Time{}
			for {
				if time.Since(lastPolicyCheck) >= remotePolicyCheckEvery {
					lastPolicyCheck = time.Now()
					p.queueDueRemotePolicies(ctx, time.Now())
				}
				p.runRemoteBackupQueue(ctx)
				select {
				case <-ticker.C:
				case <-remoteBackupWake:
				}
			}
		}()
	})
}

// recoverInterruptedRemoteOperations: an operation that was running when the
// Panel stopped has an unknown result. It goes back to the queue under the
// same id, with that fact recorded, and is reconciled by running again
// (uploads write the same key; a restore's import is verified again).
func (p *Panel) recoverInterruptedRemoteOperations(ctx context.Context) {
	_, err := p.db.GetDB().ExecContext(ctx, `
		UPDATE remote_backup_operations
		SET state = 'queued', phase = 'waiting', failure_class = 'unknown', failure_code = 'PANEL_RESTARTED',
		    failure_detail = 'The panel stopped while this operation ran; it is checked and continued.',
		    next_attempt_at = NULL
		WHERE state = 'running'`)
	if err != nil {
		log.Printf("remote backup recovery: %v", err)
	}
}

func (p *Panel) runRemoteBackupQueue(ctx context.Context) {
	for {
		op, ok, err := p.claimNextRemoteOperation(ctx)
		if err != nil {
			log.Printf("remote backup queue: %v", err)
			return
		}
		if !ok {
			return
		}
		p.runRemoteOperation(ctx, op)
	}
}

func (p *Panel) claimNextRemoteOperation(ctx context.Context) (remoteOperation, bool, error) {
	db := p.db.GetDB()
	now := time.Now().UTC().Format(time.RFC3339)
	for i := 0; i < 5; i++ {
		var id string
		err := db.QueryRowContext(ctx, `
			SELECT id FROM remote_backup_operations
			WHERE state = 'queued' AND (next_attempt_at IS NULL OR next_attempt_at <= ?)
			ORDER BY created_at, id LIMIT 1`, now).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return remoteOperation{}, false, nil
		}
		if err != nil {
			return remoteOperation{}, false, err
		}
		res, err := db.ExecContext(ctx, `
			UPDATE remote_backup_operations
			SET state = 'running', attempts = attempts + 1, started_at = COALESCE(started_at, datetime('now')),
			    heartbeat_at = datetime('now')
			WHERE id = ? AND state = 'queued'`, id)
		if err != nil {
			return remoteOperation{}, false, err
		}
		if n, _ := res.RowsAffected(); n == 1 {
			op, err := p.loadRemoteOperation(ctx, id)
			return op, err == nil, err
		}
	}
	return remoteOperation{}, false, nil
}

// remoteFailure is a classified failure of one attempt.
type remoteFailure struct {
	class, code, detail string
	retry               bool
}

func (f *remoteFailure) Error() string { return f.code + ": " + f.detail }

func remotePrerequisite(code, detail string) *remoteFailure {
	return &remoteFailure{class: remoteFailurePrerequisite, code: code, detail: detail}
}

func remoteVerifiedFailure(code, detail string) *remoteFailure {
	return &remoteFailure{class: remoteFailureVerified, code: code, detail: detail}
}

// classifyRemoteError maps a storage or crypto error to the guidance.
func classifyRemoteError(err error) *remoteFailure {
	var f *remoteFailure
	if errors.As(err, &f) {
		return f
	}
	var safe string
	var re *remotebackup.Error
	if errors.As(err, &re) {
		safe = re.Error()
	} else {
		safe = "the storage could not be reached"
	}
	switch remotebackup.KindOf(err) {
	case remotebackup.KindCredentials:
		return remotePrerequisite("REMOTE_CREDENTIALS", safe)
	case remotebackup.KindPermission:
		return remotePrerequisite("REMOTE_PERMISSION", safe)
	case remotebackup.KindNotFound:
		return remotePrerequisite("REMOTE_NOT_FOUND", safe)
	case remotebackup.KindHostKey:
		return remotePrerequisite("REMOTE_HOST_KEY", safe)
	case remotebackup.KindQuota:
		return remotePrerequisite("REMOTE_FULL", safe)
	case remotebackup.KindClock:
		return remotePrerequisite("SERVER_CLOCK", safe)
	case remotebackup.KindConfig:
		return remotePrerequisite("REMOTE_SETTINGS", safe)
	case remotebackup.KindIntegrity:
		return &remoteFailure{class: remoteFailureVerified, code: "REMOTE_INTEGRITY", detail: safe, retry: true}
	case remotebackup.KindDecrypt:
		return remoteVerifiedFailure("DECRYPT_FAILED", safe)
	}
	return &remoteFailure{class: remoteFailureUnknown, code: "REMOTE_UNREACHABLE", detail: safe, retry: true}
}

func (p *Panel) runRemoteOperation(parent context.Context, op remoteOperation) {
	ctx, cancel := context.WithTimeout(parent, remoteUploadTimeout)
	defer cancel()
	var verified string
	var err error
	switch op.Kind {
	case "upload":
		verified, err = p.runRemoteUpload(ctx, op)
	case "restore":
		verified, err = p.runRemoteRestore(ctx, op)
	default:
		err = remoteVerifiedFailure("UNKNOWN_OPERATION", "unknown operation kind")
	}
	db := p.db.GetDB()
	writeCtx, cancelWrite := context.WithTimeout(context.WithoutCancel(parent), 10*time.Second)
	defer cancelWrite()
	if err == nil {
		if _, werr := db.ExecContext(writeCtx, `
			UPDATE remote_backup_operations
			SET state = 'succeeded', phase = 'done', verified = ?, failure_class = '', failure_code = '',
			    failure_detail = '', next_attempt_at = NULL, finished_at = datetime('now'), heartbeat_at = datetime('now')
			WHERE id = ?`, verified, op.ID); werr != nil {
			log.Printf("remote backup %s finish: %v", op.ID, werr)
		}
		p.auditBackupSystem(writeCtx, "backup.remote.done:"+op.Kind+":"+op.BackupType, op.DomainID)
		return
	}
	f := classifyRemoteError(err)
	if errors.Is(err, context.DeadlineExceeded) {
		f = &remoteFailure{class: remoteFailureUnknown, code: "REMOTE_TIMED_OUT", detail: "the operation did not finish in its time limit", retry: true}
	}
	if f.retry && op.Attempts < remoteAutoAttempts {
		next := time.Now().UTC().Add(time.Duration(op.Attempts) * remoteRetryDelay).Format(time.RFC3339)
		if _, werr := db.ExecContext(writeCtx, `
			UPDATE remote_backup_operations
			SET state = 'queued', phase = 'waiting', failure_class = ?, failure_code = ?, failure_detail = ?,
			    next_attempt_at = ?, heartbeat_at = datetime('now')
			WHERE id = ?`, f.class, f.code, auditReason(f.detail), next, op.ID); werr != nil {
			log.Printf("remote backup %s retry: %v", op.ID, werr)
		}
		return
	}
	if op.Kind == "upload" && op.Transient {
		// The local copy made only for this upload is not kept after the
		// last attempt; the site's own local backups are untouched.
		if current, lerr := p.loadRemoteOperation(writeCtx, op.ID); lerr == nil && current.BackupName != "" {
			if site, serr := p.loadRemoteDomain(writeCtx, op.DomainID); serr == nil {
				p.removeLocalBackup(writeCtx, site.backupDomain, current.BackupName)
			}
		}
	}
	if _, werr := db.ExecContext(writeCtx, `
		UPDATE remote_backup_operations
		SET state = 'failed', failure_class = ?, failure_code = ?, failure_detail = ?, next_attempt_at = NULL,
		    finished_at = datetime('now'), heartbeat_at = datetime('now')
		WHERE id = ?`, f.class, f.code, auditReason(f.detail), op.ID); werr != nil {
		log.Printf("remote backup %s failure: %v", op.ID, werr)
	}
	p.auditBackupSystem(writeCtx, "backup.remote.failed:"+op.Kind+" — "+f.code, op.DomainID)
}

// progress records the phase and bytes at most every remoteProgressEvery.
type remoteProgress struct {
	p        *Panel
	id       string
	phase    string
	total    int64
	done     int64
	lastSave time.Time
}

func (p *Panel) newRemoteProgress(ctx context.Context, id, phase string, total int64) *remoteProgress {
	pr := &remoteProgress{p: p, id: id, phase: phase, total: total}
	pr.save(ctx, true)
	return pr
}

func (r *remoteProgress) add(ctx context.Context, n int64) {
	r.done += n
	r.save(ctx, false)
}

func (r *remoteProgress) save(ctx context.Context, force bool) {
	if !force && time.Since(r.lastSave) < remoteProgressEvery {
		return
	}
	r.lastSave = time.Now()
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	_, _ = r.p.db.GetDB().ExecContext(writeCtx, `
		UPDATE remote_backup_operations SET phase = ?, bytes_done = ?, bytes_total = ?, heartbeat_at = datetime('now')
		WHERE id = ?`, r.phase, r.done, r.total, r.id)
}

func (p *Panel) userIsAdministrator(ctx context.Context, userID int64) bool {
	if userID <= 0 {
		return false
	}
	var role string
	err := p.db.GetDB().QueryRowContext(ctx, `SELECT role FROM users WHERE id = ?`, userID).Scan(&role)
	return err == nil && role == roleAdmin
}

// operationSite loads the destination and the site of an operation and
// checks, again at run time, that the destination may hold the site's data.
func (p *Panel) operationSite(ctx context.Context, op remoteOperation) (remoteDestination, remoteDomain, error) {
	d, err := p.loadRemoteDestination(ctx, op.DestinationID)
	if errors.Is(err, sql.ErrNoRows) {
		return d, remoteDomain{}, remotePrerequisite("DESTINATION_REMOVED", "the destination was removed")
	}
	if err != nil {
		return d, remoteDomain{}, err
	}
	site, err := p.loadRemoteDomain(ctx, op.DomainID)
	if errors.Is(err, sql.ErrNoRows) {
		return d, site, remotePrerequisite("SITE_REMOVED", "the site was removed")
	}
	if err != nil {
		return d, site, err
	}
	if !destinationUsableForSite(d, site, p.userIsAdministrator(ctx, op.RequestedBy)) {
		return d, site, remotePrerequisite("DESTINATION_NOT_AVAILABLE", "this destination may not hold this site's backups (it is not offered, or the site changed owner)")
	}
	return d, site, nil
}

func (p *Panel) setOperationField(ctx context.Context, id, column, value string) {
	switch column {
	case "backup_name", "object_key", "backup_type":
	default:
		return
	}
	_, _ = p.db.GetDB().ExecContext(ctx, `UPDATE remote_backup_operations SET `+column+` = ? WHERE id = ?`, value, id)
}

// ----- upload -----

func (p *Panel) runRemoteUpload(ctx context.Context, op remoteOperation) (string, error) {
	d, site, err := p.operationSite(ctx, op)
	if err != nil {
		return "", err
	}
	if op.BackupName == "" {
		// A schedule's own local backup, made for this operation only.
		p.newRemoteProgress(ctx, op.ID, "backup", 0)
		name, err := p.createRemoteTransientBackup(ctx, site, op)
		if err != nil {
			return "", err
		}
		op.BackupName = name
		p.setOperationField(ctx, op.ID, "backup_name", name)
	}
	inspection, err := p.inspectLocalBackup(ctx, site.backupDomain, op.BackupName)
	if err != nil {
		return "", err
	}
	if !remotebackup.ValidBackupName(op.BackupName) || inspection.Backup.Legacy {
		return "", remoteVerifiedFailure("LEGACY_BACKUP", "backups made before the current format cannot be sent; make a new backup and send that")
	}
	if op.BackupType == "" {
		p.setOperationField(ctx, op.ID, "backup_type", inspection.Backup.Type)
		op.BackupType = inspection.Backup.Type
	}
	tag, err := p.remoteServerTag(ctx)
	if err != nil {
		return "", err
	}
	group := remotebackup.GroupManual
	if op.PolicyID > 0 {
		group = remotebackup.GroupForPolicy(op.PolicyID)
	}
	key, err := remotebackup.ObjectKey(tag, site.Name, site.ID, group, op.BackupName)
	if err != nil {
		return "", remoteVerifiedFailure("REMOTE_KEY", err.Error())
	}
	p.setOperationField(ctx, op.ID, "object_key", key)
	store, passphrase, err := p.openDestination(ctx, d)
	if err != nil {
		return "", err
	}
	defer store.Close()

	progress := p.newRemoteProgress(ctx, op.ID, "upload", inspection.Backup.Size)
	plainHash := sha256.New()
	reader, writer := io.Pipe()
	produced := make(chan error, 1)
	go func() {
		produced <- p.streamEncryptedBackup(ctx, writer, site.backupDomain, op.BackupName, inspection.Backup.Size, passphrase, plainHash, progress)
	}()
	result, putErr := store.Put(ctx, key, &sizeHintReader{r: reader, size: inspection.Backup.Size + remoteEncryptedOverhead})
	// Unblock the producer if the upload stopped before reading everything.
	_ = reader.CloseWithError(errRemoteUploadEnded)
	produceErr := <-produced
	localFault := produceErr != nil && !errors.Is(produceErr, errRemoteUploadEnded) && !errors.Is(produceErr, io.ErrClosedPipe)
	if localFault {
		return "", &remoteFailure{class: remoteFailureUnknown, code: "LOCAL_READ_FAILED",
			detail: "the local backup could not be read while sending: " + auditReason(produceErr.Error()), retry: true}
	}
	if putErr != nil {
		return "", putErr
	}
	progress.phase = "verify"
	progress.save(ctx, true)
	sidecar := remotebackup.Sidecar{
		Server: tag, Domain: site.Name, DomainID: site.ID, SubscriptionID: site.SubscriptionID,
		BackupName: op.BackupName, Type: inspection.Backup.Type, Origin: inspection.Backup.Origin,
		CreatedAt: inspection.Backup.CreatedAt.UTC(), UploadedAt: time.Now().UTC(),
		PlainSize: inspection.Backup.Size, PlainSHA256: hex.EncodeToString(plainHash.Sum(nil)),
		EncryptedSize: result.Size, EncryptedSHA256: result.SHA256, PassphraseCheck: d.PassphraseCheck,
		PanelVersion: buildVersion,
	}
	sideBytes, err := remotebackup.MarshalSidecar(sidecar)
	if err != nil {
		return "", err
	}
	if _, err := store.Put(ctx, key+remotebackup.SidecarSuffix, bytes.NewReader(sideBytes)); err != nil {
		return "", err
	}
	obj, err := store.Stat(ctx, key)
	if err != nil {
		return "", err
	}
	if obj.Size != result.Size {
		return "", &remoteFailure{class: remoteFailureVerified, code: "REMOTE_INTEGRITY", detail: "the stored size does not match what was sent", retry: true}
	}
	if op.Transient {
		p.removeLocalBackup(ctx, site.backupDomain, op.BackupName)
	}
	if op.PolicyID > 0 {
		progress.phase = "retention"
		progress.save(ctx, true)
		if err := p.applyRemoteRetention(ctx, store, tag, site, op.PolicyID); err != nil {
			log.Printf("remote backup retention for %s: %v", site.Name, err)
		}
	}
	return result.Verified, nil
}

var errRemoteUploadEnded = errors.New("upload ended")

// sizeHintReader tells the S3 store how large the object will be.
type sizeHintReader struct {
	r    io.Reader
	size int64
}

func (s *sizeHintReader) Read(p []byte) (int, error) { return s.r.Read(p) }
func (s *sizeHintReader) SizeHint() int64            { return s.size }

// streamEncryptedBackup reads the local backup from the Agent in bounded
// chunks and writes it encrypted into w.
func (p *Panel) streamEncryptedBackup(ctx context.Context, w *io.PipeWriter, d backupDomain, name string, size int64,
	passphrase string, plainHash io.Writer, progress *remoteProgress) (returnErr error) {
	defer func() { _ = w.CloseWithError(returnErr) }()
	enc, err := remotebackup.Encrypt(w, passphrase, name)
	if err != nil {
		return err
	}
	var offset int64
	for {
		chunk, err := p.readBackupChunk(ctx, d, name, offset)
		if err != nil {
			return err
		}
		if err := validateBackupChunk(chunk, offset, size); err != nil {
			return err
		}
		if len(chunk.Data) > 0 {
			if _, err := enc.Write(chunk.Data); err != nil {
				return err
			}
			plainHash.Write(chunk.Data)
			progress.add(ctx, int64(len(chunk.Data)))
		}
		offset = chunk.Offset
		if chunk.EOF {
			break
		}
		if len(chunk.Data) == 0 {
			return errors.New("empty chunk before the end")
		}
	}
	return enc.Close()
}

func (p *Panel) createRemoteTransientBackup(ctx context.Context, site remoteDomain, op remoteOperation) (string, error) {
	req := backupspec.CreateRequest{
		ProtocolVersion: backupspec.ProtocolVersion, SubscriptionID: site.SubscriptionID,
		DomainID: site.ID, DomainName: site.Name, Type: op.BackupType,
		Origin: backupspec.OriginManual, JobKey: remoteJobKeyPrefix + op.ID, SourceDir: site.DocumentRoot,
	}
	switch op.BackupType {
	case backupspec.TypeFiles:
	case backupspec.TypeFull:
		databases, err := p.domainDatabaseIdentities(ctx, site.backupDomain)
		if err != nil {
			return "", err
		}
		req.Databases = databases
	default:
		return "", remoteVerifiedFailure("LOCAL_BACKUP_FAILED", "unsupported backup type")
	}
	var resp backupspec.CreateResponse
	if err := p.callAgentContext(ctx, "Agent.CreateBackup", &req, &resp); err != nil {
		return "", &remoteFailure{class: remoteFailureUnknown, code: "LOCAL_BACKUP_FAILED", detail: auditReason(err.Error()), retry: true}
	}
	if !resp.Success || resp.Error != "" {
		return "", remoteVerifiedFailure("LOCAL_BACKUP_FAILED", auditReason(resp.Error))
	}
	return resp.Backup.Name, nil
}

func (p *Panel) inspectLocalBackup(ctx context.Context, d backupDomain, name string) (backupspec.InspectResponse, error) {
	req := backupspec.InspectRequest{
		ProtocolVersion: backupspec.ProtocolVersion, SubscriptionID: d.SubscriptionID,
		DomainID: d.ID, DomainName: d.Name, BackupName: name,
	}
	var resp backupspec.InspectResponse
	if err := p.callAgentContext(ctx, "Agent.InspectBackup", &req, &resp); err != nil {
		return resp, &remoteFailure{class: remoteFailureUnknown, code: "LOCAL_BACKUP_UNREADABLE", detail: auditReason(err.Error()), retry: true}
	}
	if !resp.Success || resp.Error != "" {
		return resp, remotePrerequisite("LOCAL_BACKUP_MISSING", "the local backup is no longer there or cannot be read: "+auditReason(resp.Error))
	}
	return resp, nil
}

func (p *Panel) removeLocalBackup(ctx context.Context, d backupDomain, name string) {
	req := backupspec.DeleteRequest{
		ProtocolVersion: backupspec.ProtocolVersion, SubscriptionID: d.SubscriptionID,
		DomainID: d.ID, DomainName: d.Name, BackupName: name,
	}
	var ok bool
	if err := p.callAgentContext(ctx, "Agent.DeleteBackup", &req, &ok); err != nil || !ok {
		log.Printf("remote backup: the local copy %s of %s was kept: %v", name, d.Name, err)
	}
}

// applyRemoteRetention keeps the schedule's daily and weekly copies of one
// site and removes the rest (object and side file). Copies sent by hand and
// other schedules' copies are never looked at.
func (p *Panel) applyRemoteRetention(ctx context.Context, store remotebackup.Store, tag string, site remoteDomain, policyID int64) error {
	var keepDaily, keepWeekly int
	if err := p.db.GetDB().QueryRowContext(ctx, `SELECT keep_daily, keep_weekly FROM remote_backup_policies WHERE id = ?`, policyID).
		Scan(&keepDaily, &keepWeekly); err != nil {
		return err
	}
	folder, err := remotebackup.SiteFolder(tag, site.Name, site.ID)
	if err != nil {
		return err
	}
	prefix := folder + "/" + remotebackup.GroupForPolicy(policyID) + "/"
	objects, err := store.List(ctx, prefix)
	if err != nil {
		return err
	}
	var items []remotebackup.RetentionItem
	for _, obj := range objects {
		parsed, err := remotebackup.ParseObjectKey(obj.Key)
		if err != nil || parsed.PolicyID != policyID || parsed.DomainID != site.ID {
			continue
		}
		created, _ := remotebackup.BackupCreatedAt(parsed.BackupName)
		items = append(items, remotebackup.RetentionItem{Key: obj.Key, Created: created})
	}
	_, remove := remotebackup.SelectRetention(items, keepDaily, keepWeekly)
	for _, key := range remove {
		if err := store.Delete(ctx, key); err != nil {
			return err
		}
		_ = store.Delete(ctx, key+remotebackup.SidecarSuffix)
		p.auditBackupSystem(ctx, "backup.remote.done:retention-delete", site.ID)
	}
	return nil
}

// ----- restore -----

func (p *Panel) runRemoteRestore(ctx context.Context, op remoteOperation) (string, error) {
	d, site, err := p.operationSite(ctx, op)
	if err != nil {
		return "", err
	}
	if err := p.checkSubscriptionQuota(ctx, site.SubscriptionID, quotaDisk); err != nil {
		var q *quotaExceededError
		switch {
		case errors.As(err, &q):
			return "", remotePrerequisite("DISK_QUOTA", q.Error())
		case errors.Is(err, errSubscriptionSuspendedCreate):
			return "", remotePrerequisite("SUBSCRIPTION_SUSPENDED", "the subscription is suspended")
		}
		return "", err
	}
	parsed, err := remotebackup.ParseObjectKey(op.ObjectKey)
	if err != nil || parsed.DomainID != site.ID {
		return "", remoteVerifiedFailure("WRONG_SITE", "this remote backup belongs to another site")
	}
	store, passphrase, err := p.openDestination(ctx, d)
	if err != nil {
		return "", err
	}
	defer store.Close()

	var sidecar *remotebackup.Sidecar
	if rc, err := store.Get(ctx, op.ObjectKey+remotebackup.SidecarSuffix); err == nil {
		raw, readErr := io.ReadAll(io.LimitReader(rc, 64<<10))
		rc.Close()
		if readErr == nil {
			if s, err := remotebackup.ParseSidecar(raw); err == nil {
				sidecar = &s
			}
		}
	} else if remotebackup.KindOf(err) != remotebackup.KindNotFound {
		return "", err
	}
	obj, err := store.Stat(ctx, op.ObjectKey)
	if err != nil {
		return "", err
	}
	progress := p.newRemoteProgress(ctx, op.ID, "download", obj.Size)
	rc, err := store.Get(ctx, op.ObjectKey)
	if err != nil {
		return "", err
	}
	defer rc.Close()
	encHash := sha256.New()
	counted := &countingReader{r: io.TeeReader(rc, encHash), onRead: func(n int) { progress.add(ctx, int64(n)) }}
	plain, err := remotebackup.Decrypt(counted, passphrase)
	if err != nil {
		return "", err
	}
	importID := "import-" + strings.TrimPrefix(op.ID, "rb-")
	abort := func() {
		abortCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		var ok bool
		_ = p.callAgentContext(abortCtx, "Agent.AbortBackupImport", &backupspec.ImportAbortRequest{
			ProtocolVersion: backupspec.ProtocolVersion, SubscriptionID: site.SubscriptionID,
			DomainID: site.ID, DomainName: site.Name, ImportID: importID}, &ok)
	}
	// A previous attempt's staging file is dropped first: the import starts at 0.
	abort()
	plainHash := sha256.New()
	buf := make([]byte, remoteImportChunk)
	var offset int64
	for {
		n, readErr := io.ReadFull(plain, buf)
		if n > 0 {
			plainHash.Write(buf[:n])
			if err := p.sendImportChunk(ctx, site, op.BackupName, parsed.BackupName, importID, offset, buf[:n], false, 0, ""); err != nil {
				abort()
				return "", err
			}
			offset += int64(n)
		}
		if errors.Is(readErr, io.EOF) || errors.Is(readErr, io.ErrUnexpectedEOF) {
			break
		}
		if readErr != nil {
			abort()
			return "", readErr
		}
	}
	// The whole object was read and its integrity check (MDC) passed.
	if sidecar != nil {
		if sidecar.EncryptedSHA256 != "" && sidecar.EncryptedSHA256 != hex.EncodeToString(encHash.Sum(nil)) {
			abort()
			return "", remoteVerifiedFailure("REMOTE_INTEGRITY", "the downloaded backup does not match its recorded checksum")
		}
		if sidecar.PlainSHA256 != "" && (sidecar.PlainSHA256 != hex.EncodeToString(plainHash.Sum(nil)) || sidecar.PlainSize != offset) {
			abort()
			return "", remoteVerifiedFailure("REMOTE_INTEGRITY", "the decrypted backup does not match its recorded checksum")
		}
	}
	progress.phase = "import"
	progress.save(ctx, true)
	if err := p.sendImportChunk(ctx, site, op.BackupName, parsed.BackupName, importID, offset, nil, true, offset,
		hex.EncodeToString(plainHash.Sum(nil))); err != nil {
		abort()
		return "", err
	}
	progress.phase = "restore"
	progress.save(ctx, true)
	if err := p.restoreLocalBackup(ctx, site.backupDomain, parsed.BackupName); err != nil {
		return "", err
	}
	verified := "sha256+mdc"
	if sidecar == nil {
		verified = "mdc"
	}
	return verified, nil
}

type countingReader struct {
	r      io.Reader
	onRead func(int)
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	if n > 0 {
		c.onRead(n)
	}
	return n, err
}

func (p *Panel) sendImportChunk(ctx context.Context, site remoteDomain, _ string, name, importID string, offset int64,
	data []byte, final bool, size int64, sum string) error {
	req := backupspec.ImportChunkRequest{
		ProtocolVersion: backupspec.ProtocolVersion, SubscriptionID: site.SubscriptionID, DomainID: site.ID,
		DomainName: site.Name, BackupName: name, ImportID: importID, Offset: offset, Data: data,
		Final: final, Size: size, SHA256: sum,
	}
	var resp backupspec.ImportChunkResponse
	err := p.callAgentContext(ctx, "Agent.ImportBackupChunk", &req, &resp)
	if err != nil {
		// The answer may have been lost: the Agent accepts the same chunk again.
		resp = backupspec.ImportChunkResponse{}
		err = p.callAgentContext(ctx, "Agent.ImportBackupChunk", &req, &resp)
	}
	if err != nil {
		return &remoteFailure{class: remoteFailureUnknown, code: "LOCAL_IMPORT_FAILED", detail: auditReason(err.Error()), retry: true}
	}
	if !resp.Success {
		return remoteVerifiedFailure("LOCAL_IMPORT_REFUSED", "this server refused the downloaded backup: "+auditReason(resp.Error))
	}
	return nil
}

// restoreLocalBackup runs the existing restore path for a local backup:
// inspect, authorize every database again, restore.
func (p *Panel) restoreLocalBackup(ctx context.Context, d backupDomain, name string) error {
	inspection, err := p.inspectLocalBackup(ctx, d, name)
	if err != nil {
		return err
	}
	if !inspection.Backup.Restorable {
		return remoteVerifiedFailure("NOT_RESTORABLE", "this backup cannot be restored safely")
	}
	req := backupspec.RestoreRequest{
		ProtocolVersion: backupspec.ProtocolVersion, SubscriptionID: d.SubscriptionID,
		DomainID: d.ID, DomainName: d.Name, BackupName: name, TargetDir: d.DocumentRoot,
	}
	if err := p.authorizeRestoreDatabases(ctx, d, inspection, &req); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return remotePrerequisite("DATABASE_GONE", "a database in this backup no longer belongs to this site; create it again, then restore")
		}
		return remoteVerifiedFailure("NOT_RESTORABLE", "the backup's contents are incomplete or unsafe")
	}
	var resp backupspec.RestoreResponse
	if err := p.callAgentContext(ctx, "Agent.RestoreBackup", &req, &resp); err != nil {
		return &remoteFailure{class: remoteFailureUnknown, code: "RESTORE_RESULT_UNKNOWN", detail: auditReason(err.Error())}
	}
	if resp.Error == backupspec.RestoreInProgress {
		return remotePrerequisite("RESTORE_IN_PROGRESS", "another restore of this site is still running")
	}
	if !resp.Success || resp.Error != "" {
		return remoteVerifiedFailure("RESTORE_FAILED", auditReason(resp.Error))
	}
	return nil
}

// ----- schedules -----

type remotePolicy struct {
	ID            int64
	DestinationID int64
	DomainID      int // 0 = every site
	Frequency     string
	BackupType    string
	KeepDaily     int
	KeepWeekly    int
	Enabled       bool
	CreatedBy     int64
}

const remotePolicyColumns = `id, destination_id, COALESCE(domain_id, 0), frequency, backup_type, keep_daily, keep_weekly, enabled, COALESCE(created_by, 0)`

func scanRemotePolicy(row interface{ Scan(...any) error }) (remotePolicy, error) {
	var r remotePolicy
	var enabled int
	err := row.Scan(&r.ID, &r.DestinationID, &r.DomainID, &r.Frequency, &r.BackupType, &r.KeepDaily, &r.KeepWeekly, &enabled, &r.CreatedBy)
	r.Enabled = enabled == 1
	return r, err
}

// queueDueRemotePolicies queues one upload per site whose schedule is due.
// A site with an operation of that schedule still waiting or running is
// skipped, so a slow upload is never doubled.
func (p *Panel) queueDueRemotePolicies(ctx context.Context, now time.Time) {
	db := p.db.GetDB()
	rows, err := db.QueryContext(ctx, `SELECT `+remotePolicyColumns+` FROM remote_backup_policies WHERE enabled = 1 ORDER BY id`)
	if err != nil {
		log.Printf("remote backup schedules: %v", err)
		return
	}
	var policies []remotePolicy
	for rows.Next() {
		r, err := scanRemotePolicy(rows)
		if err != nil {
			break
		}
		policies = append(policies, r)
	}
	rows.Close()
	for _, policy := range policies {
		domains := []int{policy.DomainID}
		if policy.DomainID == 0 {
			domains = nil
			drows, err := db.QueryContext(ctx, `SELECT id FROM domains ORDER BY id`)
			if err != nil {
				continue
			}
			for drows.Next() {
				var id int
				if drows.Scan(&id) == nil {
					domains = append(domains, id)
				}
			}
			drows.Close()
		}
		interval := 24 * time.Hour
		if policy.Frequency == "weekly" {
			interval = 7 * 24 * time.Hour
		}
		for _, domainID := range domains {
			var created, state string
			err := db.QueryRowContext(ctx, `
				SELECT created_at, state FROM remote_backup_operations
				WHERE policy_id = ? AND domain_id = ? ORDER BY created_at DESC, id DESC LIMIT 1`,
				policy.ID, domainID).Scan(&created, &state)
			if err == nil {
				if state == remoteOpQueued || state == remoteOpRunning {
					continue
				}
				if last, perr := time.ParseInLocation("2006-01-02 15:04:05", created, time.UTC); perr == nil && now.Sub(last) < interval-time.Hour {
					continue
				}
			} else if !errors.Is(err, sql.ErrNoRows) {
				continue
			}
			site, err := p.loadRemoteDomain(ctx, domainID)
			if err != nil {
				continue
			}
			if _, _, err := p.queueRemoteOperation(ctx, remoteOperationRequest{
				Kind: "upload", DestinationID: policy.DestinationID, Site: site, PolicyID: policy.ID,
				BackupType: policy.BackupType, Transient: true, RequestedBy: policy.CreatedBy,
			}); err != nil {
				log.Printf("remote backup schedule %d for %s: %v", policy.ID, site.Name, err)
			}
		}
	}
}
