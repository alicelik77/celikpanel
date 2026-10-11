package services

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/alicelik/celikpanel/internal/transport"
)

// The second step of D-031 (2026-10-11): the site files the Panel writes
// outside nginx, the PHP-FPM pool (kind php_pool) and the application unit
// (kind app_unit), follow the vhost's rule. Each starts with the render header
// in its format's comment form, is classified before every write, is replaced
// only when it is CelikPanel's unchanged text (identical bytes are not
// rewritten and nothing is reloaded for them), and is otherwise kept with the
// Panel's text held beside it in <file>.celikpanel-pending. "Take
// CelikPanel's" keeps a dated copy of the owner's file first. Writes go
// through a temporary file and one rename, keep the file's mode, owner and
// group, and never follow a symlink.
//
// D-031'in ikinci adımı: Panel'in nginx dışında yazdığı site dosyaları (PHP-FPM
// havuzu ve uygulama birimi) vhost ile aynı kurala uyar: biçimin yorum
// biçiminde başlık, her yazmadan önce sınıflandırma, yalnız Panel'in
// değişmemiş metninin değiştirilmesi, aksi hâlde korunması ve Panel'in metninin
// yanına bekleyen dosya olarak bırakılması.

// ManagedHeaderPrefixINI is the render header of an INI file (a PHP-FPM pool):
// a ";" comment line. nginx files and systemd units use "# ".
const ManagedHeaderPrefixINI = "; celikpanel-render v2 sha256="

// ManagedHeaderPrefixHash is the render header in "#" comment form (nginx,
// systemd units).
const ManagedHeaderPrefixHash = managedRenderHeaderPrefix

// ManagedSiteFile is one site file outside nginx and the evidence that
// decides its render. The caller holds the lock that serializes writes of
// the file (the pool's managedConfigMutationMu, the unit's per-site lock).
type ManagedSiteFile struct {
	Kind         string
	Path         string
	HeaderPrefix string
	// Body is the Panel's text without the header.
	Body    string
	Trigger string
	// RecordedSHA256 is the body digest the Panel's ledger last recorded.
	RecordedSHA256       string
	ExpectedFileSHA256   string
	ExpectedRenderSHA256 string
	// Legacy renders the frozen earlier releases' texts for a headerless
	// file; nil when no earlier text applies.
	Legacy func() ([]LegacyVhostRender, error)
	// IncludeDir names the owner's supported extension point for the file
	// (a systemd drop-in directory), "" when the format has none.
	IncludeDir string
}

func (f ManagedSiteFile) sealed() []byte {
	return []byte(SealManagedTextWith(f.HeaderPrefix, f.Body))
}

func (f ManagedSiteFile) inspect() siteFileInspection {
	return inspectSiteFileWith(f.Path, f.HeaderPrefix, f.Legacy, f.RecordedSHA256)
}

func (f ManagedSiteFile) result(inspected siteFileInspection) transport.SiteFileResult {
	return transport.SiteFileResult{
		Kind:         f.Kind,
		Path:         f.Path,
		State:        inspected.state,
		Reason:       inspected.reason,
		Detail:       inspected.detail,
		AdoptedFrom:  inspected.adoptedFrom,
		RenderSHA256: sha256Hex([]byte(f.Body)),
		FileSHA256:   inspected.fileSHA,
		IncludeDir:   f.IncludeDir,
	}
}

// SiteFileHeldError is an operation on a pool the render rule did not let
// write. Its text starts with SiteFileHeldErrorPrefix and names the kind,
// state and outcome as tokens, so a Panel that only receives the RPC error's
// text (the older pool RPCs) still answers a typed refusal.
type SiteFileHeldError struct {
	File transport.SiteFileResult
}

// SiteFileHeldErrorPrefix starts SiteFileHeldError's text.
const SiteFileHeldErrorPrefix = "site_file_held"

func (e *SiteFileHeldError) Error() string {
	return fmt.Sprintf("%s kind=%s state=%s outcome=%s reason=%s: %s was not changed",
		SiteFileHeldErrorPrefix, e.File.Kind, e.File.State, e.File.Outcome, e.File.Reason, e.File.Path)
}

// ParseSiteFileHeldError reads SiteFileHeldError's tokens back from its text.
func ParseSiteFileHeldError(text string) (transport.SiteFileResult, bool) {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, SiteFileHeldErrorPrefix+" ") {
		return transport.SiteFileResult{}, false
	}
	head, _, _ := strings.Cut(text[len(SiteFileHeldErrorPrefix)+1:], ":")
	var file transport.SiteFileResult
	for _, field := range strings.Fields(head) {
		key, value, ok := strings.Cut(field, "=")
		if !ok {
			continue
		}
		switch key {
		case "kind":
			file.Kind = value
		case "state":
			file.State = value
		case "outcome":
			file.Outcome = value
		case "reason":
			file.Reason = value
		}
	}
	if file.Kind == "" || file.Outcome == "" {
		return transport.SiteFileResult{}, false
	}
	return file, true
}

// SiteFileApplied says whether an outcome left the Panel's text in place.
func SiteFileApplied(outcome string) bool {
	switch outcome {
	case transport.SiteFileOutcomeWritten, transport.SiteFileOutcomeUnchanged,
		transport.SiteFileOutcomeRecreated, transport.SiteFileOutcomeTaken:
		return true
	}
	return false
}

// ApplyManagedSiteFile classifies the file and writes only what the render
// rule allows:
//   - CelikPanel's unchanged text: replaced; identical bytes are left and
//     nothing is validated or activated;
//   - absent: written only on create and recreate, otherwise "missing";
//   - owner-edited, foreign, unknown origin: kept, with the Panel's text in
//     <file>.celikpanel-pending, except "take" bound to the digests the owner
//     was shown, which keeps a dated copy first;
//   - unreadable (a symlink, not a regular file, too large, not read): refused.
//
// After a write, validate (the service's own check) and activate (its reload)
// run; when either fails, the previous bytes (or no file) are put back with
// their mode and owner and activate runs again. A refused replacement (an
// immutable file) keeps the Panel's text in the pending file too.
//
// ApplyManagedSiteFile dosyayı sınıflandırır ve yalnız kuralın izin verdiğini
// yazar; doğrulama ya da etkinleştirme başarısız olursa önceki baytları geri
// koyar.
func ApplyManagedSiteFile(f ManagedSiteFile, validate, activate func() error) (transport.SiteFileResult, error) {
	sealed := f.sealed()
	inspected := f.inspect()
	result := f.result(inspected)
	trigger := normalizedSiteFileTrigger(f.Trigger)
	outcome := transport.SiteFileOutcomeWritten
	backup := false
	switch inspected.state {
	case transport.SiteFileManagedUnchanged:
		if bytes.Equal(inspected.content, sealed) {
			result.Outcome = transport.SiteFileOutcomeUnchanged
			result.WrittenSHA256 = result.RenderSHA256
			removeStalePending(f.Path)
			return result, nil
		}
		if trigger == transport.SiteFileTriggerTake {
			outcome = transport.SiteFileOutcomeTaken
		}
	case transport.SiteFileAbsent:
		switch trigger {
		case transport.SiteFileTriggerCreate:
		case transport.SiteFileTriggerRecreate:
			outcome = transport.SiteFileOutcomeRecreated
		default:
			result.Outcome = transport.SiteFileOutcomeMissing
			return result, nil
		}
	case transport.SiteFileOwnerEdited, transport.SiteFileForeign, transport.SiteFileUnknownOrigin:
		if trigger != transport.SiteFileTriggerTake {
			result.Outcome = transport.SiteFileOutcomeKept
			holdPendingSiteFile(&result, f.Path, sealed)
			return result, nil
		}
		if f.ExpectedFileSHA256 == "" || f.ExpectedFileSHA256 != inspected.fileSHA ||
			(f.ExpectedRenderSHA256 != "" && f.ExpectedRenderSHA256 != result.RenderSHA256) {
			result.Outcome = transport.SiteFileOutcomeRefused
			result.Reason = transport.SiteFileReasonChanged
			return result, nil
		}
		outcome = transport.SiteFileOutcomeTaken
		backup = true
	default: // unreadable
		result.Outcome = transport.SiteFileOutcomeRefused
		return result, nil
	}

	if backup {
		name, err := writeSiteFileBackup(f.Path, inspected)
		if err != nil {
			result.Outcome = transport.SiteFileOutcomeRefused
			result.Reason = transport.SiteFileReasonWriteRefused
			result.Detail = boundedSiteFileDetail("keep a copy of the file: " + err.Error())
			return result, nil
		}
		result.BackupPath = name
	}
	if err := writeManagedSiteFile(f.Path, sealed, inspected); err != nil {
		result.Outcome = transport.SiteFileOutcomeRefused
		if errors.Is(err, errSiteFileChanged) {
			result.Reason = transport.SiteFileReasonChanged
			return result, nil
		}
		result.Reason = transport.SiteFileReasonWriteRefused
		result.Detail = boundedSiteFileDetail(err.Error())
		result.SystemError = siteFileErrnoName(err)
		var measuredError string
		result.Protection, measuredError = siteFileProtectionOf(f.Path)
		if result.SystemError == "" {
			result.SystemError = measuredError
		}
		if _, _, pendingErr := writePendingSiteFile(f.Path, sealed); pendingErr == nil {
			result.PendingPath, result.PendingSHA256 = existingPending(f.Path)
		}
		return result, nil
	}
	fail := func(reason string, cause error) (transport.SiteFileResult, error) {
		restoreErr := restoreManagedSiteFile(f.Path, inspected)
		var activateErr error
		if activate != nil {
			activateErr = activate()
		}
		result.Outcome = transport.SiteFileOutcomeFailed
		result.Reason = reason
		result.Detail = boundedSiteFileDetail(cause.Error())
		if restoreErr != nil || activateErr != nil {
			result.Restore = transport.SiteFileRestoreUnconfirmed
			return result, fmt.Errorf("%w; rollback incomplete: %v", cause, errors.Join(restoreErr, activateErr))
		}
		result.Restore = transport.SiteFileRestoreConfirmed
		return result, nil
	}
	if validate != nil {
		if err := validate(); err != nil {
			return fail(transport.SiteFileReasonConfigRefused, err)
		}
	}
	if activate != nil {
		if err := activate(); err != nil {
			return fail(transport.SiteFileReasonReloadFailed, err)
		}
	}
	result.Outcome = outcome
	result.WrittenSHA256 = result.RenderSHA256
	result.FileSHA256 = sha256Hex(sealed)
	result.Reloaded = activate != nil
	removeStalePending(f.Path)
	return result, nil
}

// holdPendingSiteFile writes the Panel's text beside a kept file.
func holdPendingSiteFile(result *transport.SiteFileResult, path string, sealed []byte) {
	pendingPath, pendingSHA, err := writePendingSiteFile(path, sealed)
	if err != nil {
		result.Reason = transport.SiteFileReasonPendingNotSet
		result.Detail = boundedSiteFileDetail(err.Error())
		return
	}
	result.PendingPath, result.PendingSHA256 = pendingPath, pendingSHA
}

// restoreManagedSiteFile puts back exactly what was there: the previous bytes
// with their mode and owner, or no file.
func restoreManagedSiteFile(path string, inspected siteFileInspection) error {
	if inspected.exists {
		return atomicWriteSiteFile(path, inspected.content, inspected.mode, inspected.ownership)
	}
	if info, err := os.Lstat(path); err == nil && info.Mode().IsRegular() {
		return os.Remove(path)
	}
	return nil
}

// InspectManagedSiteFile classifies the file and computes the difference
// between it and the Panel's text. It writes nothing.
func InspectManagedSiteFile(f ManagedSiteFile) (transport.SiteFileResult, string, bool) {
	inspected := f.inspect()
	result := f.result(inspected)
	result.Outcome = transport.SiteFileOutcomeInspected
	if inspected.state == transport.SiteFileManagedUnchanged && inspected.adoptedFrom == "" {
		if declared, _, ok := splitManagedHeaderWith(f.HeaderPrefix, inspected.content); ok {
			result.WrittenSHA256 = declared
		}
	}
	if inspected.exists && inspected.state != transport.SiteFileUnreadable {
		result.Protection, result.SystemError = siteFileProtectionOf(f.Path)
	}
	result.PendingPath, result.PendingSHA256 = existingPending(f.Path)
	sealed := f.sealed()
	if !inspected.exists || inspected.state == transport.SiteFileUnreadable || bytes.Equal(inspected.content, sealed) {
		return result, "", false
	}
	diff, truncated := UnifiedSiteFileDiff(
		f.Path+" (on this server)", "CelikPanel's text",
		stripManagedHeaderWith(f.HeaderPrefix, inspected.content), stripManagedHeaderWith(f.HeaderPrefix, sealed),
	)
	return result, diff, truncated
}

// BackupUnlessManaged keeps a dated copy of a file that is about to be removed
// (the site is deleted, the application type changed) when it is not
// CelikPanel's unchanged text. It returns the copy's path ("" when none was
// needed). An unreadable file is not copied; its removal decides on its own.
func BackupUnlessManaged(path, headerPrefix string, legacy func() ([]LegacyVhostRender, error)) (string, error) {
	inspected := inspectSiteFileWith(path, headerPrefix, legacy, "")
	if !inspected.exists || inspected.state == transport.SiteFileUnreadable ||
		inspected.state == transport.SiteFileManagedUnchanged {
		return "", nil
	}
	return writeSiteFileBackup(path, inspected)
}

// RemoveSiteFilePending removes the Panel's pending file of a removed file.
func RemoveSiteFilePending(path string) {
	removeStalePending(path)
}

// HeldPendingBody returns the body of the Panel's pending text beside path
// when its header verifies (it is the Panel's own held text), "" otherwise.
func HeldPendingBody(path, headerPrefix string) (string, bool) {
	pending := path + SiteFilePendingSuffix
	info, err := os.Lstat(pending)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxManagedSiteFileBytes {
		return "", false
	}
	content, err := readManagedConfig(pending)
	if err != nil {
		return "", false
	}
	declared, body, ok := splitManagedHeaderWith(headerPrefix, content)
	if !ok || sha256Hex(body) != declared {
		return "", false
	}
	return string(body), true
}
