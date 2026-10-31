package main

import (
	"strings"
	"time"
)

// The final check of a setup run waits while a requirement is not met: a
// public name still spreading, the other server finishing its own setup.
// Until alpha.114 lab E8 that wait had no end and no change of wording: a
// mail extension on a joined server waited 8+ minutes on "dns" with a
// general sentence and nothing to do. The run still checks again by itself
// (a registrar change can legitimately take hours, and a finished run must
// not be failed for it), but past serverSetupVerificationOverdueAfter the
// wait is said as overdue: which checks are still open, that the owner acts
// now, and what to do. Nothing is changed on the host by this.
// Son denetim sinirsiz ve sessiz beklemez: sure asilinca acik denetimler,
// kimin islem yapacagi ve sonraki eylem soylenir; ev sahibinde degisiklik yok.

const serverSetupVerificationOverdueAfter = 30 * time.Minute
const serverSetupVerificationOverdueCode = "server_setup_verification_overdue"

var serverSetupVerificationNow = time.Now

// serverSetupVerificationWait records when this wait began (once) and, past
// the bound, sets the overdue reason on the run. It reads and writes only
// the execution record.
func serverSetupVerificationWait(execution *serverSetupExecution, now time.Time) {
	since, err := time.Parse(time.RFC3339, execution.VerificationSince)
	if execution.VerificationSince == "" || err != nil {
		execution.VerificationSince = now.UTC().Format(time.RFC3339)
		since = now.UTC()
	}
	if now.Sub(since) < serverSetupVerificationOverdueAfter {
		return
	}
	open := []string{}
	for _, check := range execution.Checks {
		if !check.Advisory && check.State != "ready" {
			label := check.ID
			if check.Reason != "" {
				label += " (" + check.Reason + ")"
			}
			open = append(open, label)
		}
	}
	execution.Error = &serviceOperationError{Code: serverSetupVerificationOverdueCode,
		Message: "Every setup step finished more than 30 minutes ago, but the final check still waits for: " + strings.Join(open, ", ") + ". You, the owner: act on the items listed on this page, then use Check requirements again. Setup also keeps checking by itself; installed components stay as they are."}
}
