package main

import (
	"fmt"
	"net/http"

	"github.com/alicelik/celikpanel/internal/backupspec"
)

// ---- typed restore failures (alpha.128, lab finding of alpha.127) ----

const (
	errCodeRestoreSafetyFailed   = "BACKUP_RESTORE_SAFETY_FAILED"
	errCodeRestoreLoadReverted   = "BACKUP_RESTORE_LOAD_REVERTED"
	errCodeRestoreRollbackFailed = "BACKUP_RESTORE_ROLLBACK_FAILED"
	errCodeRestoreInvalid        = "BACKUP_RESTORE_INVALID"
	errCodeRestoreFinishFailed   = "BACKUP_RESTORE_FINISH_FAILED"
	// alpha.134 lab findings: these answered "internal error".
	errCodeRestoreNotFound    = "BACKUP_RESTORE_NOT_FOUND"
	errCodeRestoreOtherSite   = "BACKUP_RESTORE_OTHER_SITE"
	errCodeRestoreNoSpace     = "BACKUP_RESTORE_NO_SPACE"
	errCodeRestoreSiteAccount = "BACKUP_RESTORE_SITE_ACCOUNT"
)

// writeRestoreFailure answers a restore the Agent typed: what happened,
// what was and was not changed, who acts and the next step (D-024), with the
// engine's own reason as the detail. False when the failure is untyped.
func writeRestoreFailure(w http.ResponseWriter, resp backupspec.RestoreResponse) bool {
	detail := []string{}
	if resp.Detail != "" {
		detail = append(detail, boundedAgentDiagnostic(resp.Detail))
	}
	safety := ""
	if resp.SafetyBackup != nil {
		safety = resp.SafetyBackup.Name
	}
	switch resp.Code {
	case backupspec.RestoreCodeSafetyFailed:
		writeCodedErrorDetails(w, http.StatusBadGateway, errCodeRestoreSafetyFailed,
			"The safety copy that is taken before every restore could not be made, so nothing was restored and the site is unchanged. The reason is below; fix it (for example free disk space or start the database server), then restore again.", "", detail)
	case backupspec.RestoreCodeLoadReverted:
		writeCodedErrorDetails(w, http.StatusUnprocessableEntity, errCodeRestoreLoadReverted,
			"The database server refused this backup's data, so the restore stopped and the site was put back as it was before. The server's reason is below. Restore another backup, or ask your administrator if the reason is not clear.", "", detail)
	case backupspec.RestoreCodeRollbackFailed:
		writeCodedErrorDetails(w, http.StatusInternalServerError, errCodeRestoreRollbackFailed,
			fmt.Sprintf("The restore stopped, and putting the site back did not finish either. The safety copy %s is kept: restore it from this list to return to the state before. The reason is below.", safety), "", detail)
	case backupspec.RestoreCodeInvalid:
		writeCodedErrorDetails(w, http.StatusConflict, errCodeRestoreInvalid,
			"This backup failed its checks (for example, it is damaged), so nothing was restored and the site is unchanged. Choose another backup.", "", detail)
	case backupspec.RestoreCodeFinishFailed:
		writeCodedErrorDetails(w, http.StatusInternalServerError, errCodeRestoreFinishFailed,
			"The backup was restored, but its last step (cleanup and saving to disk) did not finish. The safety copy taken before is kept. Check the site; restore again if something is missing.", "", detail)
	case backupspec.RestoreCodeNotFound:
		writeCodedError(w, http.StatusNotFound, errCodeRestoreNotFound,
			"This backup is not among this site's backups on the server (it was deleted, or it is another site's), so nothing was restored and the site is unchanged. Choose a backup from this site's list.", "")
	case backupspec.RestoreCodeOtherSite:
		writeCodedError(w, http.StatusConflict, errCodeRestoreOtherSite,
			"This backup was made of another site, so it is not restored here; nothing was restored and the site is unchanged. Choose a backup of this site, or restore this one on the site it was made of.", "")
	case backupspec.RestoreCodeNoSpace:
		writeCodedError(w, http.StatusConflict, errCodeRestoreNoSpace,
			"The server's disk has no room to unpack this backup, so nothing was restored and the site is unchanged. The server administrator frees disk space on the server; then restore again.", "")
	case backupspec.RestoreCodeSiteAccount:
		writeCodedError(w, http.StatusConflict, errCodeRestoreSiteAccount,
			"The site's own system account, which the program's files belong to, could not be found on the server, so nothing was restored and the site is unchanged. The server administrator checks the site's hosting account; then restore again.", "")
	default:
		return false
	}
	return true
}
