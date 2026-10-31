package main

import (
	"errors"
	"strings"
	"testing"
)

// Arch PowerDNS follow-up drill (2026-10-30): the main reached verification
// while its backup was still reinstalling after a cut; the completion check
// (read with the backup's binding) was not ready, the runner ended on a plain
// error and the step stayed "running" without guidance. The completion of a
// running setup now returns a wait the runner handles (status waiting, its
// reason, rechecked). Component test only.
func TestServerSetupCompletionWaitingNamesThePendingCheck(t *testing.T) {
	checks := []serverSetupCheck{
		{ID: "ptr", State: "missing", Advisory: true},
		{ID: "panel_https", State: "ready"},
		{ID: "dns_redundancy", State: "pending"},
		{ID: "firewall", State: "missing"},
	}
	wait := serverSetupCompletionWaiting(checks)
	var asWait *serverSetupPeerWait
	if !errors.As(error(wait), &asWait) || wait.Code != serverSetupCompletionWaitingCode ||
		!strings.Contains(wait.Message, "dns_redundancy (pending)") ||
		strings.Contains(wait.Message, "ptr") || !strings.Contains(wait.Message, "setup checks again by itself") {
		t.Fatalf("wait=%+v", wait)
	}
	if got := serverSetupCompletionWaiting(nil); !strings.Contains(got.Message, "a final check") {
		t.Fatalf("empty checks: %+v", got)
	}
}
