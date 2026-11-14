package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/alicelik/celikpanel/internal/backupspec"
	"github.com/alicelik/celikpanel/internal/transport"
)

// One restore per domain at a time (D-029). A restore writes a safety backup,
// replaces the document root and imports the databases; two of them on the
// same domain extracted into the same document root and imported the same
// database side by side. The second one is refused before it reads or writes
// anything, it does not wait: a restore that arrives while one is running was
// not asked for after seeing the first one's result.
//
// Alan adı başına aynı anda tek geri yükleme (D-029). İkincisi hiçbir şey
// okumadan ve yazmadan reddedilir; beklemez.
type backupRestoreKey struct {
	subscriptionID int
	domainID       int
}

var backupRestoresRunning = struct {
	sync.Mutex
	domains map[backupRestoreKey]struct{}
}{domains: make(map[backupRestoreKey]struct{})}

var errBackupRestoreInProgress = errors.New(backupspec.RestoreInProgress)

// tryLockBackupRestore claims the domain for one restore. ok is false while
// another restore of the same domain holds it.
func tryLockBackupRestore(scope backupScope) (release func(), ok bool) {
	key := backupRestoreKey{subscriptionID: scope.SubscriptionID, domainID: scope.DomainID}
	backupRestoresRunning.Lock()
	defer backupRestoresRunning.Unlock()
	if _, running := backupRestoresRunning.domains[key]; running {
		return nil, false
	}
	backupRestoresRunning.domains[key] = struct{}{}
	return func() {
		backupRestoresRunning.Lock()
		delete(backupRestoresRunning.domains, key)
		backupRestoresRunning.Unlock()
	}, true
}

func (a *Agent) restoreBackup(req *backupspec.RestoreRequest, resp *backupspec.RestoreResponse) error {
	scope := restoreScope(req)
	if err := validateV2Scope(scope); err != nil {
		return err
	}
	release, ok := tryLockBackupRestore(scope)
	if !ok {
		return errBackupRestoreInProgress
	}
	defer release()
	backupPath, legacy, err := resolveBackup(scope, req.BackupName)
	if err != nil {
		return backupFindRefusal(err)
	}
	docroot, err := backupDocumentRoot(scope.SubscriptionID, scope.DomainID)
	if err != nil {
		return err
	}
	if legacy {
		info, err := inspectLegacyBackup(backupPath, req.BackupName)
		if err != nil {
			return err
		}
		if !info.Restorable || (info.Type != backupspec.TypeFiles && info.Type != backupspec.TypeFull) {
			return errors.New("legacy database backups cannot be restored safely")
		}
		// Pre-v2 full archives did not contain a verifiable database manifest.
		// Restore them explicitly as file-only archives.
		safety, err := a.createSafetyBackup(scope, backupspec.TypeFiles, backupspec.DatabaseIdentity{}, nil)
		if err != nil {
			return &restoreFailure{code: backupspec.RestoreCodeSafetyFailed, reason: err, err: fmt.Errorf("create pre-restore safety backup: %w", err)}
		}
		resp.SafetyBackup = &safety
		if err := restoreFilesArchive(backupPath, docroot); err != nil {
			return err
		}
		resp.Success = true
		return nil
	}

	manifest, err := readBackupManifest(backupPath)
	if err != nil {
		return &restoreFailure{code: backupspec.RestoreCodeInvalid, reason: err, err: err}
	}
	if err := validateManifest(manifest, scope); err != nil {
		if errors.Is(err, errManifestScopeMismatch) {
			return backupPackageRefusal(err)
		}
		return &restoreFailure{code: backupspec.RestoreCodeInvalid, reason: err, err: err}
	}
	var requestedDatabases []backupspec.DatabaseIdentity
	var singleDatabase backupspec.DatabaseIdentity
	switch manifest.Type {
	case backupspec.TypeFiles:
		// No database identity participates in a files-only restore.
	case backupspec.TypeDatabase:
		if len(manifest.Databases) != 1 {
			return errors.New("database backup manifest is invalid")
		}
		singleDatabase, err = validateRestoreDatabase(manifest.Databases[0].Identity, req.Database)
		if err != nil {
			return err
		}
		// alpha.127: the database's running cluster of the backup's major,
		// decided before the safety backup and before anything is written.
		if singleDatabase, err = restoreDatabaseTarget(manifest.Databases[0].Identity, singleDatabase); err != nil {
			return err
		}
		requestedDatabases = []backupspec.DatabaseIdentity{singleDatabase}
	case backupspec.TypeFull:
		requestedDatabases, err = validateFullRestoreSet(manifest.databaseIdentities(), req.Databases)
		if err != nil {
			return err
		}
		stored := make(map[int]backupspec.DatabaseIdentity, len(manifest.Databases))
		for _, database := range manifest.Databases {
			stored[database.Identity.ID] = database.Identity
		}
		for i := range requestedDatabases {
			if requestedDatabases[i], err = restoreDatabaseTarget(stored[requestedDatabases[i].ID], requestedDatabases[i]); err != nil {
				return err
			}
		}
	default:
		return errors.New("unsupported backup type")
	}

	safety, err := a.createSafetyBackup(scope, manifest.Type, singleDatabase, requestedDatabases)
	if err != nil {
		if refusal := backupClusterRefusalOf(err); refusal != nil {
			return err
		}
		return &restoreFailure{code: backupspec.RestoreCodeSafetyFailed, reason: err, err: fmt.Errorf("create pre-restore safety backup: %w", err)}
	}
	resp.SafetyBackup = &safety

	workDir, err := newRestoreWorkDir(scope)
	if err != nil {
		return backupStagingRefusal(err)
	}
	defer os.RemoveAll(workDir)
	targetDir := filepath.Join(workDir, "target")
	unpacked, err := unpackBackupPackage(backupPath, targetDir, scope)
	if err != nil {
		// The payloads' checksums are proved here: a damaged archive stops
		// here, before anything is replaced.
		return backupPackageRefusal(err)
	}
	if unpacked.Type != manifest.Type {
		return errors.New("backup changed during restore preparation")
	}

	// Site-home areas (version 3): the ones this release puts back are
	// staged with the site's files; the others are named and left as they
	// are on the server.
	areas, err := a.restoreAreaPlan(scope, targetDir, unpacked, resp)
	if err != nil {
		return err
	}
	switch unpacked.Type {
	case backupspec.TypeFiles:
		if len(areas.names) == 0 {
			if err := restoreFilesArchive(filepath.Join(targetDir, filepath.FromSlash(unpacked.Files.Name)), docroot); err != nil {
				return err
			}
			break
		}
		if err := restoreFilesAndAreas(filepath.Join(targetDir, filepath.FromSlash(unpacked.Files.Name)), docroot, areas); err != nil {
			return err
		}
	case backupspec.TypeDatabase:
		if _, err := validateRestoreDatabase(unpacked.Databases[0].Identity, singleDatabase); err != nil {
			return err
		}
		if err := a.restoreDatabasePackage(scope, targetDir, safety.Path, singleDatabase); err != nil {
			return err
		}
	case backupspec.TypeFull:
		if _, err := validateFullRestoreSet(unpacked.databaseIdentities(), requestedDatabases); err != nil {
			return err
		}
		if err := a.restoreFullPackage(scope, targetDir, safety.Path, docroot, unpacked, requestedDatabases, areas); err != nil {
			return err
		}
	}
	resp.Restored = areas.names
	resp.Success = true
	return nil
}

// areaRestorePlan is what a restore does with a package's site-home areas:
// the areas it puts back (staged from the home payload), the site home and
// the site's account the restored areas are given to.
type areaRestorePlan struct {
	home    string
	payload string
	names   []string
	owner   areaOwner
}

// areaOwner is the owner and group a restored area gets: the site's own
// account and its own group, exactly as the Programs tab's deploy makes the
// program's folders (runtimeAppAsUser runs mkdir as that account). Never the
// site home's group: that is the web server's (applyHostingLayout), and the
// web server has no business in the program's folder.
//
// Geri yüklenen alan sitenin kendi hesabına ve kendi grubuna verilir; site ev
// dizininin grubu web sunucusununkidir, programın klasörüne verilmez.
type areaOwner struct{ uid, gid int }

// backupAreaOwner resolves and proves the site's account with the deploy
// path's own helper (its home must be exactly the site home).
func backupAreaOwner(scope backupScope) (areaOwner, error) {
	account, err := serviceAppLookupAccount(transport.ServiceAppTarget{
		SubscriptionID: scope.SubscriptionID, DomainID: scope.DomainID, Domain: scope.DomainName,
	})
	if err != nil {
		return areaOwner{}, err
	}
	return areaOwner{uid: account.UID, gid: account.GID}, nil
}

// restoreAreaPlan names the areas this restore puts back and, in the
// answer, the ones it leaves as they are (service apps; not restored by this
// release, docs/SITE-BACKUP-CONTENTS-DESIGN.md).
func (a *Agent) restoreAreaPlan(scope backupScope, targetDir string, manifest backupManifest, resp *backupspec.RestoreResponse) (areaRestorePlan, error) {
	plan := areaRestorePlan{}
	if manifest.Home == nil {
		return plan, nil
	}
	home, err := backupSiteHome(scope.SubscriptionID, scope.DomainID)
	if err != nil {
		return plan, err
	}
	plan.home = home
	plan.payload = filepath.Join(targetDir, filepath.FromSlash(manifest.Home.Name))
	for _, area := range manifest.areaNames() {
		if restorableArea(area) {
			plan.names = append(plan.names, area)
		} else {
			resp.NotRestored = append(resp.NotRestored, area)
		}
	}
	if len(plan.names) > 0 {
		// Before anything is staged: without the account nothing is
		// replaced (the safety copy just made stays in the list).
		owner, err := backupAreaOwner(scope)
		if err != nil {
			return plan, &restoreFailure{code: backupspec.RestoreCodeSiteAccount, err: fmt.Errorf("the site's account: %w", err)}
		}
		plan.owner = owner
	}
	return plan, nil
}

// stage stages every area of the plan. On an error nothing stays staged and
// an area folder this restore made is removed again.
func (plan areaRestorePlan) stage() (stagedSet, func(), error) {
	set := make(stagedSet, 0, len(plan.names))
	made := make([]string, 0)
	forget := func() {
		for _, area := range made {
			removeMadeArea(plan.home, area)
		}
	}
	for _, area := range plan.names {
		staged, wasMade, err := prepareAreaRestore(plan.payload, plan.home, area, plan.owner)
		if wasMade {
			made = append(made, area)
		}
		if err != nil {
			set.cleanup()
			forget()
			return nil, func() {}, fmt.Errorf("restore %s: %w", area, err)
		}
		set = append(set, staged)
	}
	return set, forget, nil
}

// stagedSet is several staged folder replacements published together: in
// order, and when one fails the ones already published are put back.
type stagedSet []*stagedFilesRestore

func (set stagedSet) publish() error {
	for i, staged := range set {
		if err := staged.Publish(); err != nil {
			if i == 0 {
				return err
			}
			var rollbackErrors []error
			for j := i - 1; j >= 0; j-- {
				if rollbackErr := set[j].Rollback(); rollbackErr != nil {
					rollbackErrors = append(rollbackErrors, rollbackErr)
				}
			}
			return restoreRollbackError(err, errors.Join(rollbackErrors...))
		}
	}
	return nil
}

func (set stagedSet) commit() error {
	var commitErrors []error
	for _, staged := range set {
		if err := staged.Commit(); err != nil {
			commitErrors = append(commitErrors, err)
		}
	}
	return errors.Join(commitErrors...)
}

func (set stagedSet) cleanup() {
	for _, staged := range set {
		staged.Cleanup()
	}
}

// restoreFilesAndAreas restores public_html and the plan's areas together.
func restoreFilesAndAreas(archivePath, docroot string, plan areaRestorePlan) error {
	files, err := prepareFilesRestore(archivePath, docroot)
	if err != nil {
		return backupStagingRefusal(err)
	}
	set := stagedSet{files}
	// A closure: the areas are appended after this line.
	defer func() { set.cleanup() }()
	areas, forget, err := plan.stage()
	if err != nil {
		return backupStagingRefusal(err)
	}
	set = append(set, areas...)
	if err := set.publish(); err != nil {
		forget()
		return err
	}
	if err := set.commit(); err != nil {
		return &restoreFailure{code: backupspec.RestoreCodeFinishFailed, reason: err, err: fmt.Errorf(
			"restored files were published, but final cleanup or durability proof failed; pre-restore safety backup was retained: %w",
			err,
		)}
	}
	return nil
}

func (a *Agent) createSafetyBackup(scope backupScope, backupType string, database backupspec.DatabaseIdentity, databases []backupspec.DatabaseIdentity) (backupspec.Info, error) {
	request := &backupspec.CreateRequest{
		ProtocolVersion: backupspec.ProtocolVersion,
		SubscriptionID:  scope.SubscriptionID,
		DomainID:        scope.DomainID,
		DomainName:      scope.DomainName,
		Type:            backupType,
		Origin:          backupspec.OriginPreRestore,
		Database:        database,
		Databases:       databases,
	}
	return a.createBackup(request)
}

func newRestoreWorkDir(scope backupScope) (string, error) {
	dir, err := ensureBackupDir(scope)
	if err != nil {
		return "", err
	}
	workDir, err := os.MkdirTemp(dir, ".restore-work-")
	if err != nil {
		return "", err
	}
	if err := os.Chmod(workDir, 0o700); err != nil {
		_ = os.RemoveAll(workDir)
		return "", err
	}
	return workDir, nil
}

func restoreFilesArchive(archivePath, docroot string) error {
	staged, err := prepareFilesRestore(archivePath, docroot)
	if err != nil {
		return backupStagingRefusal(err)
	}
	defer staged.Cleanup()
	if err := staged.Publish(); err != nil {
		return err
	}
	if err := staged.Commit(); err != nil {
		return &restoreFailure{code: backupspec.RestoreCodeFinishFailed, reason: err, err: fmt.Errorf(
			"restored files were published, but final cleanup or durability proof failed; pre-restore safety backup was retained: %w",
			err,
		)}
	}
	return nil
}

func (a *Agent) restoreDatabasePackage(scope backupScope, targetDir, safetyPath string, database backupspec.DatabaseIdentity) error {
	safetyDir := filepath.Join(filepath.Dir(targetDir), "safety")
	safetyManifest, err := unpackBackupPackage(safetyPath, safetyDir, scope)
	if err != nil {
		return err
	}
	if safetyManifest.Type != backupspec.TypeDatabase || len(safetyManifest.Databases) != 1 {
		return errors.New("safety database backup is invalid")
	}
	if _, err := validateRestoreDatabase(safetyManifest.Databases[0].Identity, database); err != nil {
		return err
	}
	targetPayload := filepath.Join(targetDir, filepath.FromSlash(databasePayloadName(database.ID)))
	safetyPayload := filepath.Join(safetyDir, filepath.FromSlash(databasePayloadName(database.ID)))
	if err := restoreDatabaseFromFile(database, targetPayload); err != nil {
		rollbackErr := restoreDatabaseFromFile(database, safetyPayload)
		return restoreRollbackError(err, rollbackErr)
	}
	return nil
}

func (a *Agent) restoreFullPackage(scope backupScope, targetDir, safetyPath, docroot string, manifest backupManifest, databases []backupspec.DatabaseIdentity, plan areaRestorePlan) error {
	safetyDir := filepath.Join(filepath.Dir(targetDir), "safety")
	safetyManifest, err := unpackBackupPackage(safetyPath, safetyDir, scope)
	if err != nil {
		return err
	}
	if safetyManifest.Type != backupspec.TypeFull {
		return errors.New("safety full backup is invalid")
	}
	if _, err := validateFullRestoreSet(safetyManifest.databaseIdentities(), databases); err != nil {
		return err
	}
	filesPayload := filepath.Join(targetDir, filepath.FromSlash(manifest.Files.Name))
	staged, err := prepareFilesRestore(filesPayload, docroot)
	if err != nil {
		return backupStagingRefusal(err)
	}
	set := stagedSet{staged}
	// A closure: the areas are appended after this line.
	defer func() { set.cleanup() }()
	areas, forget, err := plan.stage()
	if err != nil {
		return backupStagingRefusal(err)
	}
	set = append(set, areas...)

	attempted := make([]backupspec.DatabaseIdentity, 0, len(databases))
	for _, database := range databases {
		attempted = append(attempted, database)
		targetPayload := filepath.Join(targetDir, filepath.FromSlash(databasePayloadName(database.ID)))
		if err := restoreDatabaseFromFile(database, targetPayload); err != nil {
			rollbackErr := rollbackDatabases(attempted, safetyDir)
			forget()
			return restoreRollbackError(err, rollbackErr)
		}
	}
	if err := set.publish(); err != nil {
		forget()
		var failure *restoreFailure
		if errors.As(err, &failure) && failure.code == backupspec.RestoreCodeRollbackFailed {
			// The folders could not all be put back: the databases are
			// put back all the same, and the answer stays "rollback failed".
			_ = rollbackDatabases(attempted, safetyDir)
			return err
		}
		rollbackErr := rollbackDatabases(attempted, safetyDir)
		return restoreRollbackError(err, rollbackErr)
	}
	if err := set.commit(); err != nil {
		return &restoreFailure{code: backupspec.RestoreCodeFinishFailed, reason: err, err: fmt.Errorf(
			"full restore was published, but final cleanup or durability proof failed; pre-restore safety backup was retained: %w",
			err,
		)}
	}
	return nil
}

func rollbackDatabases(databases []backupspec.DatabaseIdentity, safetyDir string) error {
	errorsFound := make([]error, 0)
	for i := len(databases) - 1; i >= 0; i-- {
		database := databases[i]
		payload := filepath.Join(safetyDir, filepath.FromSlash(databasePayloadName(database.ID)))
		if err := restoreDatabaseFromFile(database, payload); err != nil {
			errorsFound = append(errorsFound, fmt.Errorf("database %d rollback: %w", database.ID, err))
		}
	}
	return errors.Join(errorsFound...)
}

func restoreRollbackError(restoreErr, rollbackErr error) error {
	if rollbackErr == nil {
		return &restoreFailure{code: backupspec.RestoreCodeLoadReverted, reason: restoreErr,
			err: fmt.Errorf("restore failed and safety backup was reapplied: %w", restoreErr)}
	}
	return &restoreFailure{code: backupspec.RestoreCodeRollbackFailed, reason: restoreErr,
		err: fmt.Errorf("restore failed: %v; safety rollback also failed: %w", restoreErr, rollbackErr)}
}

// restoreFailure is a restore that stopped, typed (alpha.128 lab finding: a
// refused PostgreSQL restore answered a generic error). reason is the
// engine's own refusal, which the screen shows bounded.
type restoreFailure struct {
	code   string
	reason error
	err    error
}

func (e *restoreFailure) Error() string { return e.err.Error() }
func (e *restoreFailure) Unwrap() error { return e.err }

// restoreEngineReason is the part of a failure the engine said: from its
// first "ERROR" on, one line, bounded; control characters removed.
func restoreEngineReason(err error) string {
	if err == nil {
		return ""
	}
	text := err.Error()
	if i := strings.Index(text, "ERROR"); i >= 0 {
		text = text[i:]
	}
	text = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, text)
	return boundedText(strings.Join(strings.Fields(text), " "), 300)
}

// Refusals the screen names (alpha.134 lab: another site's backup answered
// "internal error"). Each is typed only where the cause is certain; any other
// error stays untyped.
//
// Ekranın adlandırdığı retler: yalnız nedeni kesin olanlar türlenir.

// backupFindRefusal types an error of finding a backup by name: a name that is
// not in this site's backup folder (or no folder at all), or is no backup
// name at all, is not found.
func backupFindRefusal(err error) error {
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, errInvalidBackupName) {
		return &restoreFailure{code: backupspec.RestoreCodeNotFound, err: err}
	}
	return err
}

// backupPackageRefusal types an error of reading, checking or unpacking a
// package: another site's, no room on the disk, or failed its checks. An
// error of the server itself (a read or write that failed) stays untyped.
func backupPackageRefusal(err error) error {
	var pathErr *os.PathError
	var errno syscall.Errno
	switch {
	case err == nil:
		return nil
	case errors.Is(err, errManifestScopeMismatch):
		return &restoreFailure{code: backupspec.RestoreCodeOtherSite, err: err}
	case noSpace(err):
		return &restoreFailure{code: backupspec.RestoreCodeNoSpace, err: err}
	case errors.As(err, &pathErr), errors.As(err, &errno):
		return err
	}
	return &restoreFailure{code: backupspec.RestoreCodeInvalid, reason: err, err: err}
}

// backupStagingRefusal types a failed staging (nothing is replaced yet): no
// room on the disk; anything else stays as it is.
func backupStagingRefusal(err error) error {
	if noSpace(err) {
		return &restoreFailure{code: backupspec.RestoreCodeNoSpace, err: err}
	}
	return err
}

func noSpace(err error) bool {
	return errors.Is(err, syscall.ENOSPC) || errors.Is(err, syscall.EDQUOT)
}

// restoreFailureOf types an error of restoreBackup for the response.
func restoreFailureOf(err error) (string, string) {
	var failure *restoreFailure
	if errors.As(err, &failure) {
		return failure.code, restoreEngineReason(failure.reason)
	}
	return "", ""
}
