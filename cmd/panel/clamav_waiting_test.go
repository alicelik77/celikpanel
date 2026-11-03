package main

import (
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.118 lab, D6: ClamAV installed before freshclam's first download is
// "waiting", never a failure, and the waiting ends with the running daemon.
func TestClamAVWaitingOnlyOnAnInstalledStoppedRow(t *testing.T) {
	const first = transport.InstallWaitingFirstSignatures
	for _, c := range []struct {
		installed bool
		status    string
		waiting   string
		want      string
	}{
		{true, "inactive (dead)", first, first},
		{true, "active (running)", first, ""},
		{false, "inactive", first, ""},
		{true, "inactive (dead)", "something_else", ""},
		{true, "inactive (dead)", "", ""},
	} {
		if got := waitingForResponse(c.installed, c.status, c.waiting); got != c.want {
			t.Errorf("waitingForResponse(%v, %q, %q) = %q, want %q", c.installed, c.status, c.waiting, got, c.want)
		}
	}
}
