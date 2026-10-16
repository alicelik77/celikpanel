package recoveryobs

import "testing"

// Frankfurt 2026-10-16: the updater types an installed update whose last step
// (turning Certbot renewal back on) is repeated by recovery. The shell
// publisher writes it and this reader keeps it through the forward recovery
// until the update is verified.
func TestFinishingSchedulerStepIsATypedForwardCause(t *testing.T) {
	shell, read, _, _, _ := shellObservationFixture(t, "finishing-scheduler")
	shell(`release_observation_publish "$3" "$4" running none update_running`)
	shell(`release_observation_publish_failure "$3" "$4" update_finishing_scheduler`)
	shell(`release_observation_publish "$3" "$4" failed none update_failed`)
	if got := read(); got.Phase != "failed" || got.FailureCode != "update_finishing_scheduler" {
		t.Fatalf("finishing step after the update: %#v", got)
	}
	shell(`release_observation_publish "$3" "$4" recovering none recovery_running`)
	if got := read(); got.Phase != "recovering" || got.FailureCode != "update_finishing_scheduler" {
		t.Fatalf("finishing step during recovery: %#v", got)
	}
	shell(`release_observation_publish "$3" "$4" succeeded update_verified update_verified`)
	if got := read(); got.Phase != "succeeded" || got.TerminalProof != "update_verified" || got.FailureCode != "update_finishing_scheduler" {
		t.Fatalf("verified update: %#v", got)
	}
	if !ValidFailureCode("update_finishing_scheduler") || ValidFailureCode("update_finishing") {
		t.Fatal("closed failure codes changed")
	}
}
