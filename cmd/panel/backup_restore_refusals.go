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
			"This backup failed its checks (it is damaged or belongs to another site), so nothing was restored and the site is unchanged. Choose another backup.", "", detail)
	case backupspec.RestoreCodeFinishFailed:
		writeCodedErrorDetails(w, http.StatusInternalServerError, errCodeRestoreFinishFailed,
			"The backup was restored, but its last step (cleanup and saving to disk) did not finish. The safety copy taken before is kept. Check the site; restore again if something is missing.", "", detail)
	default:
		return false
	}
	return true
}
