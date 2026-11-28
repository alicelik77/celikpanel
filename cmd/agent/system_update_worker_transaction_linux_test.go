//go:build linux

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The page's "the transaction began" words (web/src/lib/systemUpdateOutcome.ts
// UPDATER_TRANSACTION_BEGAN, its summary alternative) and the Panel's bounded
// summary (cmd/panel panelUpdateFailureLine).
var workerTransactionBeganRE = regexp.MustCompile(`(?:^|: )!! CELIKPANEL_UPDATE_FAILURE code=update_failed state=recovery_required reason=[^/]{1,120} detail=`)

// A killed updater (no last "!! " line) leaves the worker to say whether its
// release transaction had begun: marker present or unreadable → the summary
// the page treats as begun; marker absent → the updater's own last line.
func TestKilledUpdaterRecordSaysWhetherItsTransactionBegan(t *testing.T) {
	oldResolver, oldRunner, oldPresent := systemUpdateExecutableResolver, systemUpdateCommandRunner, systemUpdateReleaseTransactionPresent
	t.Cleanup(func() {
		systemUpdateExecutableResolver, systemUpdateCommandRunner, systemUpdateReleaseTransactionPresent = oldResolver, oldRunner, oldPresent
	})
	systemUpdateExecutableResolver = func(candidates ...string) (string, error) { return candidates[0], nil }

	transactionRoot := filepath.Join(t.TempDir(), "celikpanel-release-transaction")
	if err := os.Mkdir(transactionRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(transactionRoot, "quiesce.pending")
	systemUpdateReleaseTransactionPresent = func() (bool, error) {
		return persistentReleaseTransactionPresent(transactionRoot)
	}
	run := func(output string, runErr error) string {
		t.Helper()
		systemUpdateCommandRunner = func(context.Context, string, []string) ([]byte, error) {
			return []byte(output), runErr
		}
		err := runSystemUpdateInstaller(context.Background(), linuxSystemUpdateTestState(strings.Repeat("7", 32)), "41")
		if err == nil {
			t.Fatal("a failed updater was reported as success")
		}
		// What failStateLocked records.
		return sanitizedSystemUpdateError(err)
	}
	killed := errors.New("signal: killed")
	progress := "celikpanel archive: OK\nStopping celikpanel services\n"

	// 1. Marker absent: as before, the updater's own last line.
	if got := run(progress, killed); got != "reviewed updater failed: signal: killed: Stopping celikpanel services" {
		t.Fatalf("no marker: %q", got)
	}

	// 2. Marker present (any of the durable markers, a dangling link too).
	for _, name := range agentReleaseTransactionMarkers {
		path := filepath.Join(transactionRoot, name)
		if err := os.Symlink(filepath.Join(transactionRoot, "missing"), path); err != nil {
			t.Fatal(err)
		}
		got := run(progress, killed)
		if !workerTransactionBeganRE.MatchString(got) ||
			!strings.Contains(got, "reason=the updater stopped without its own summary while its release transaction was open detail=Stopping celikpanel services") {
			t.Fatalf("marker %s: %q", name, got)
		}
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
	}
	// No output at all (killed before its first line) with a marker.
	if err := os.WriteFile(marker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := run("", killed); !workerTransactionBeganRE.MatchString(got) {
		t.Fatalf("marker, empty output: %q", got)
	}
	// A long last line cannot push the began words out of the bounded record.
	if got := run(strings.Repeat("p", 3000)+"\n", killed); !workerTransactionBeganRE.MatchString(got) || len(got) > systemUpdateMaxErrorSize {
		t.Fatalf("marker, long last line: %d %q", len(got), got[:120])
	}
	// The updater's own last "!! " line always stands, marker or not: its exit
	// handler said what happened (the summary or a pending handler's line).
	for _, own := range []string{
		"!! CELIKPANEL_UPDATE_FAILURE code=update_failed state=unchanged reason=cannot read the candidate manifest detail=",
		"!! release recovery foundation monotonic preflight failed",
		"!! Pending update finalization failed; marker preserved and both coordinators stopped.",
	} {
		if got := run(progress+own+"\n", errors.New("exit status 1")); got != "reviewed updater failed: exit status 1: "+own {
			t.Fatalf("own line replaced: %q", got)
		}
	}
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}

	// 3. The markers cannot be read: doubt falls to "began".
	if err := os.Remove(transactionRoot); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(transactionRoot, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	got := run(progress, killed)
	if !workerTransactionBeganRE.MatchString(got) ||
		!strings.Contains(got, "reason=the updater stopped without its own summary and its release transaction could not be checked detail=Stopping celikpanel services") {
		t.Fatalf("unreadable marker: %q", got)
	}
	systemUpdateReleaseTransactionPresent = func() (bool, error) { return false, errors.New("permission denied") }
	if got := run(progress, killed); !workerTransactionBeganRE.MatchString(got) {
		t.Fatalf("marker read error: %q", got)
	}

	// The production reader is the Agent's own durable-marker gate.
	systemUpdateReleaseTransactionPresent = oldPresent
	present, err := systemUpdateReleaseTransactionPresent()
	wantPresent, wantErr := persistentReleaseTransactionPresent(agentReleaseTransactionRoot)
	if present != wantPresent || (err == nil) != (wantErr == nil) {
		t.Fatalf("production reader differs from the release transaction gate: %v %v / %v %v", present, err, wantPresent, wantErr)
	}
}
