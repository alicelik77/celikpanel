package main

import (
	"strings"
	"testing"
)

// Frankfurt 2026-10-16: an installed, verified update whose last step (turning
// Certbot renewal back on) is repeated by automatic recovery is said as
// "installed; finishing", never as a failure, and its pause names the cause.
func TestFinishingSchedulerStepIsSaidAsInstalledAndFinishing(t *testing.T) {
	for _, phase := range []string{"failed", "recovering"} {
		s := knownStatus(phase)
		s.PreviousFailure, s.FailureCode = "update_failed", "update_finishing_scheduler"
		en := ownerStatusText(t, s, "en")
		tr := ownerStatusText(t, s, "tr")
		if !strings.HasPrefix(en[0], "The new version is installed and running. One final step, turning automatic certificate renewal (Certbot) back on, did not confirm on the first try") ||
			!strings.HasPrefix(tr[0], "Yeni sürüm kuruldu ve çalışıyor. Son adım olan otomatik sertifika yenilemeyi (Certbot) yeniden açma ilk denemede doğrulanmadı") {
			t.Fatalf("%s finishing guidance:\n%s\n%s", phase, en[0], tr[0])
		}
		for _, line := range []string{en[0], tr[0]} {
			if strings.Contains(strings.ToLower(line), "failed") || internalToken.MatchString(line) {
				t.Fatalf("%s finishing guidance reads as a failure or leaks a token: %q", phase, line)
			}
		}
		if !strings.Contains(en[len(en)-1], "failure_code=update_finishing_scheduler") {
			t.Fatalf("support line lost the typed cause: %q", en[len(en)-1])
		}
	}
	// Between automatic attempts the first cause keeps the same calm guidance.
	retry := knownStatus("recovering")
	retry.PreviousFailure, retry.FirstFailureCode = "recovery_failed", "update_finishing_scheduler"
	if line := ownerStatusText(t, retry, "en")[0]; !strings.HasPrefix(line, "The new version is installed and running.") {
		t.Fatalf("retrying guidance lost the finishing cause: %q", line)
	}
	// At the pause the owner acts; the cause comes first.
	paused := knownStatus("recovery_required")
	paused.AutomaticRecovery, paused.PreviousFailure, paused.FirstFailureCode = "paused_retry_limit", "recovery_failed", "update_finishing_scheduler"
	en := ownerStatusText(t, paused, "en")[0]
	tr := ownerStatusText(t, paused, "tr")[0]
	if !strings.HasPrefix(en, "The new version is installed and running, but turning automatic certificate renewal (Certbot) back on did not confirm after the automatic attempts.") ||
		!strings.Contains(en, "Automatic recovery used all three attempts without finishing") ||
		!strings.HasPrefix(tr, "Yeni sürüm kuruldu ve çalışıyor, ancak otomatik sertifika yenilemeyi (Certbot) yeniden açma") {
		t.Fatalf("paused finishing guidance:\n%s\n%s", en, tr)
	}
	// Negative control: a verified update keeps its own success wording.
	done := knownStatus("succeeded")
	done.PreviousFailure, done.FailureCode = "update_failed", "update_finishing_scheduler"
	if line := ownerStatusText(t, done, "en")[0]; !strings.HasPrefix(line, "The update completed and the new version was verified") {
		t.Fatalf("verified update replaced by finishing guidance: %q", line)
	}
}
