//go:build linux

package recoveryobs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// alpha.134: optional work that changes a service (the database grant
// migration) waits while an update to this build is not yet verified. The
// phase of the latest attempt to the commit is read; nothing recorded is
// "", an unreadable directory is unknown.
func TestLatestPhaseForTargetFollowsTheLatestAttempt(t *testing.T) {
	root, anchor, uid, gid := observationFixture(t)
	target := strings.Repeat("c", 40)
	phase := func() (string, bool) { return latestPhaseAt(root, target, uid, gid, anchor) }
	if _, known := phase(); known {
		t.Fatal("a directory that cannot be opened was read as known")
	}
	seed := testRecord()
	seed.RequestID, seed.TargetCommit = strings.Repeat("0", 32), strings.Repeat("9", 40)
	if err := publishAt(root, seed, uid, gid, anchor); err != nil {
		t.Fatal(err)
	}
	if got, known := phase(); !known || got != "" {
		t.Fatalf("no attempt to this commit: %q %v", got, known)
	}
	write := func(id, phase, reason, proof, at string) {
		t.Helper()
		r := Record{RequestID: id, TargetCommit: target, Phase: phase, Reason: reason, TerminalProof: proof, ObservedAt: at, PreviousFailure: "none"}
		raw, err := r.Encode()
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(root, id+".status")
		if err := os.WriteFile(path, raw, 0o640); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, 0o640); err != nil {
			t.Fatal(err)
		}
	}
	write(strings.Repeat("1", 32), "running", "update_running", "none", "2026-11-13T10:00:00Z")
	if got, known := phase(); !known || got != "running" {
		t.Fatalf("running: %q %v", got, known)
	}
	write(strings.Repeat("1", 32), "succeeded", "update_verified", "update_verified", "2026-11-13T10:02:00Z")
	if got, known := phase(); !known || got != "succeeded" {
		t.Fatalf("succeeded: %q %v", got, known)
	}
	if _, known := latestPhaseAt(root, "not-a-commit", uid, gid, anchor); known {
		t.Fatal("an invalid commit was read")
	}
}
