//go:build linux

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

// alpha.124: both real servers listed celikpanel-self-update-<id>.service
// under systemctl --failed for attempts that a later update resolved. At
// start the Agent clears the failed mark of exactly those units; an attempt
// with no later success keeps its unit, and every attempt's record is kept.
func TestSystemUpdateClearsOnlyResolvedAttemptUnits(t *testing.T) {
	root := linuxSystemUpdateTestRoot(t)
	at := func(id, status, created string) *systemUpdateState {
		state := linuxSystemUpdateTestState(strings.Repeat(id, 32))
		state.Status, state.CreatedAt, state.UpdatedAt = status, created, created
		if status == systemUpdateFailed {
			state.Error = "package manager busy"
		}
		if err := writeSystemUpdateState(root, state, nil); err != nil {
			t.Fatal(err)
		}
		return state
	}
	busyEarlier := at("1", systemUpdateFailed, "2026-11-01T10:00:00Z")
	succeeded := at("2", systemUpdateSucceeded, "2026-11-01T11:00:00Z")
	unresolved := at("3", systemUpdateFailed, "2026-11-02T10:00:00Z")

	backend := newLinuxSystemUpdateBackend()
	backend.stateRoot = root
	backend.floorPath = filepath.Join(root, "missing-floor")
	backend.now = func() time.Time { return time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC) }
	backend.installedBuild = func(context.Context) (string, string, error) { return "v1.2.2", strings.Repeat("c", 40), nil }
	backend.unitState = func(context.Context, string) (systemUpdateUnitState, error) { return systemUpdateUnitInactive, nil }
	backend.observe = nil
	var cleared []string
	backend.resetFailedUnit = func(_ context.Context, id string) error { cleared = append(cleared, id); return nil }
	if err := backend.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := []string{busyEarlier.RequestID, succeeded.RequestID}
	if !reflect.DeepEqual(cleared, want) {
		t.Fatalf("cleared %v, want %v (never the unresolved %s)", cleared, want, unresolved.RequestID)
	}
	for _, state := range []*systemUpdateState{busyEarlier, succeeded, unresolved} {
		if _, err := readSystemUpdateState(root, state.RequestID); err != nil {
			t.Fatalf("the attempt's record %s is gone: %v", state.RequestID, err)
		}
	}

	// An active attempt: nothing is cleared at all.
	active := at("4", systemUpdateRunning, "2026-11-05T11:59:00Z")
	backend.unitState = func(context.Context, string) (systemUpdateUnitState, error) { return systemUpdateUnitActive, nil }
	cleared = nil
	if err := backend.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(cleared) != 0 {
		t.Fatalf("cleared units while %s is active: %v", active.RequestID, cleared)
	}

	// No success at all: every failed attempt is unresolved.
	if got := systemUpdateResolvedAttempts([]*systemUpdateState{busyEarlier, unresolved}); len(got) != 0 {
		t.Fatalf("resolved without a success: %v", got)
	}
}

// alpha.133 lab (13 Nov 2026): a press refused on a busy package manager left
// its celikpanel-self-update-<id>.service under systemctl --failed, because
// only a later successful update cleared it. An attempt whose own record says
// the reviewed updater stopped before changing anything is final; its unit's
// failed mark is cleared at Agent start and when it is read, and its record is
// kept. Attempts that may still need recovery keep their unit.
func TestSystemUpdateClearsTheUnitOfAnAttemptThatStoppedUnchanged(t *testing.T) {
	root := linuxSystemUpdateTestRoot(t)
	at := func(id, failure string) *systemUpdateState {
		state := linuxSystemUpdateTestState(strings.Repeat(id, 32))
		state.Status, state.Error = systemUpdateFailed, failure
		state.CreatedAt, state.UpdatedAt = "2026-11-12T22:23:35Z", "2026-11-12T22:23:35Z"
		if err := writeSystemUpdateState(root, state, nil); err != nil {
			t.Fatal(err)
		}
		return state
	}
	refused := at("1", "reviewed updater failed: exit status 1: !! CELIKPANEL_UPDATE_FAILURE code=package_manager_busy state=unchanged reason=the host package manager is active detail=")
	preflight := at("2", "reviewed updater failed: exit status 1: !! CELIKPANEL_UPDATE_FAILURE code=update_preflight_refused state=unchanged reason=update preflight step=agent_idle class=check_failed detail=")
	needsRecovery := at("3", "reviewed updater failed: exit status 1: !! CELIKPANEL_UPDATE_FAILURE code=update_failed state=recovery_required reason=installer stopped detail=")
	untyped := at("4", "installer stopped")

	backend := newLinuxSystemUpdateBackend()
	backend.stateRoot = root
	backend.floorPath = filepath.Join(root, "missing-floor")
	backend.now = func() time.Time { return time.Date(2026, 11, 13, 1, 0, 0, 0, time.UTC) }
	backend.installedBuild = func(context.Context) (string, string, error) { return "v1.2.2", strings.Repeat("c", 40), nil }
	backend.unitState = func(context.Context, string) (systemUpdateUnitState, error) { return systemUpdateUnitInactive, nil }
	backend.observe = nil
	var cleared []string
	backend.resetFailedUnit = func(_ context.Context, id string) error { cleared = append(cleared, id); return nil }

	// Agent start: only the two that stopped unchanged.
	if err := backend.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if want := []string{refused.RequestID, preflight.RequestID}; !reflect.DeepEqual(cleared, want) {
		t.Fatalf("cleared %v, want %v", cleared, want)
	}

	// A read of the refused attempt clears its own unit; a read of the others does not.
	cleared = nil
	if _, err := backend.Status(context.Background(), refused.RequestID); err != nil {
		t.Fatal(err)
	}
	for _, other := range []*systemUpdateState{needsRecovery, untyped} {
		if _, err := backend.Status(context.Background(), other.RequestID); err != nil {
			t.Fatal(err)
		}
	}
	if want := []string{refused.RequestID}; !reflect.DeepEqual(cleared, want) {
		t.Fatalf("status reads cleared %v, want %v", cleared, want)
	}
	for _, state := range []*systemUpdateState{refused, preflight, needsRecovery, untyped} {
		loaded, err := readSystemUpdateState(root, state.RequestID)
		if err != nil || loaded.Status != systemUpdateFailed || loaded.Error != state.Error {
			t.Fatalf("the attempt's record %s changed: %v %v", state.RequestID, loaded, err)
		}
	}

	// An abandon receipt never had a worker; it is not an unchanged stop.
	receipt := linuxSystemUpdateTestState(strings.Repeat("5", 32))
	receipt.Status, receipt.Error = systemUpdateFailed, systemUpdateAbandonedError
	if systemUpdateStoppedUnchanged(receipt) {
		t.Fatal("an abandon receipt was taken for an unchanged stop")
	}
}

// The follow-up after a start clears a refused attempt's unit only once its
// worker has ended, and stops for every other outcome.
func TestSystemUpdateSettleStoppedAttemptWaitsForItsWorker(t *testing.T) {
	root := linuxSystemUpdateTestRoot(t)
	state := linuxSystemUpdateTestState(strings.Repeat("6", 32))
	state.Status = systemUpdateRunning
	state.CreatedAt, state.UpdatedAt = "2026-11-12T22:23:35Z", "2026-11-12T22:23:35Z"
	write := func() {
		if err := writeSystemUpdateState(root, state, nil); err != nil {
			t.Fatal(err)
		}
	}
	write()
	backend := newLinuxSystemUpdateBackend()
	backend.stateRoot = root
	unit := systemUpdateUnitActive
	backend.unitState = func(context.Context, string) (systemUpdateUnitState, error) { return unit, nil }
	var cleared []string
	backend.resetFailedUnit = func(_ context.Context, id string) error { cleared = append(cleared, id); return nil }
	settle := func(id string) bool {
		t.Helper()
		done, err := backend.SettleStoppedAttempt(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		return done
	}

	if settle(state.RequestID) || len(cleared) != 0 {
		t.Fatalf("a running attempt was settled: %v", cleared)
	}
	// The worker wrote the refusal and is still exiting.
	state.Status = systemUpdateFailed
	state.Error = "reviewed updater failed: exit status 1: !! CELIKPANEL_UPDATE_FAILURE code=package_manager_busy state=unchanged reason=the host package manager is active detail="
	state.UpdatedAt = "2026-11-12T22:24:15Z"
	write()
	if settle(state.RequestID) || len(cleared) != 0 {
		t.Fatalf("settled while the worker unit was still active: %v", cleared)
	}
	unit = systemUpdateUnitInactive
	if !settle(state.RequestID) || !reflect.DeepEqual(cleared, []string{state.RequestID}) {
		t.Fatalf("the ended refusal was not settled: %v", cleared)
	}

	// Every other outcome ends the follow-up without touching the unit.
	for _, outcome := range []struct{ status, failure string }{
		{systemUpdateFailed, "reviewed updater failed: exit status 1: !! CELIKPANEL_UPDATE_FAILURE code=update_failed state=recovery_required reason=installer stopped detail="},
		{systemUpdateFailed, "installer stopped"},
		{systemUpdateSucceeded, ""},
	} {
		cleared = nil
		state.Status, state.Error = outcome.status, outcome.failure
		write()
		if !settle(state.RequestID) || len(cleared) != 0 {
			t.Fatalf("%s %q: cleared %v", outcome.status, outcome.failure, cleared)
		}
	}
	if !settle(strings.Repeat("7", 32)) {
		t.Fatal("an unknown attempt keeps the follow-up running")
	}
}

type fakeSystemUpdateSettler struct {
	answers []bool
	calls   int
}

func (settler *fakeSystemUpdateSettler) SettleStoppedAttempt(context.Context, string) (bool, error) {
	settler.calls++
	if settler.calls > len(settler.answers) {
		return false, errors.New("unexpected read")
	}
	return settler.answers[settler.calls-1], nil
}

func TestFollowSystemUpdateAttemptStopsWhenSettledAndIsBounded(t *testing.T) {
	start := time.Date(2026, 11, 13, 1, 0, 0, 0, time.UTC)
	clock := start
	settler := &fakeSystemUpdateSettler{answers: []bool{false, false, true}}
	followSystemUpdateAttempt(settler, strings.Repeat("8", 32), start.Add(systemUpdateSettleWindow),
		func() time.Time { return clock }, func(d time.Duration) { clock = clock.Add(d) })
	if settler.calls != 3 {
		t.Fatalf("reads = %d, want 3 (stops once settled)", settler.calls)
	}
	never := &fakeSystemUpdateSettler{answers: make([]bool, 1000)}
	clock = start
	followSystemUpdateAttempt(never, strings.Repeat("8", 32), start.Add(30*time.Second),
		func() time.Time { return clock }, func(d time.Duration) { clock = clock.Add(d) })
	if never.calls != 10 {
		t.Fatalf("bounded follow-up made %d reads, want 10 within 30 seconds", never.calls)
	}
}

// The reset names exactly the attempt's own unit, and only when systemd says
// that unit failed.
func TestSystemUpdateResetFailedWorkerUnitTouchesOnlyItsOwnFailedUnit(t *testing.T) {
	oldResolver, oldRunner := systemUpdateExecutableResolver, systemUpdateCommandRunner
	t.Cleanup(func() { systemUpdateExecutableResolver, systemUpdateCommandRunner = oldResolver, oldRunner })
	systemUpdateExecutableResolver = func(candidates ...string) (string, error) { return candidates[0], nil }
	id := strings.Repeat("a", 32)
	unit := "celikpanel-self-update-" + id + ".service"
	for _, tc := range []struct {
		state string
		want  [][]string
	}{
		{"ActiveState=failed\n", [][]string{{"show", unit, "--property=ActiveState", "--no-pager"}, {"reset-failed", unit}}},
		{"ActiveState=inactive\n", [][]string{{"show", unit, "--property=ActiveState", "--no-pager"}}},
		{"ActiveState=active\n", [][]string{{"show", unit, "--property=ActiveState", "--no-pager"}}},
	} {
		var calls [][]string
		systemUpdateCommandRunner = func(_ context.Context, name string, args []string) ([]byte, error) {
			if name != "/usr/bin/systemctl" {
				t.Fatalf("ran %s", name)
			}
			calls = append(calls, append([]string(nil), args...))
			if args[0] == "show" {
				return []byte(tc.state), nil
			}
			return nil, nil
		}
		backend := newLinuxSystemUpdateBackend()
		if err := backend.resetFailedWorkerUnit(context.Background(), id); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(calls, tc.want) {
			t.Fatalf("%q: %v", tc.state, calls)
		}
	}
	backend := newLinuxSystemUpdateBackend()
	if err := backend.resetFailedWorkerUnit(context.Background(), "../other"); err == nil {
		t.Fatal("a name that is not an attempt id was accepted")
	}
}

// alpha.124 lab (123 -> 124 on Debian 13): the new Agent started while the
// attempt that restarted it was still active, so the start-time Reconcile
// cleared nothing and the two earlier failed units stayed under systemctl
// --failed after the update succeeded. The status read after the attempt
// ends clears them, once.
func TestSystemUpdateStatusClearsResolvedUnitsOnceAfterTheSuccess(t *testing.T) {
	root := linuxSystemUpdateTestRoot(t)
	at := func(id, status, created string) *systemUpdateState {
		state := linuxSystemUpdateTestState(strings.Repeat(id, 32))
		state.Status, state.CreatedAt, state.UpdatedAt = status, created, created
		if status == systemUpdateFailed {
			state.Error = "installer stopped"
		}
		if err := writeSystemUpdateState(root, state, nil); err != nil {
			t.Fatal(err)
		}
		return state
	}
	refused := at("1", systemUpdateFailed, "2026-11-05T12:22:15Z")
	raced := at("2", systemUpdateFailed, "2026-11-05T12:23:59Z")
	update := at("3", systemUpdateRunning, "2026-11-05T12:29:49Z")

	backend := newLinuxSystemUpdateBackend()
	backend.stateRoot = root
	backend.floorPath = filepath.Join(root, "missing-floor")
	backend.now = func() time.Time { return time.Date(2026, 11, 5, 12, 31, 11, 0, time.UTC) }
	backend.installedBuild = func(context.Context) (string, string, error) { return "v1.2.2", strings.Repeat("c", 40), nil }
	backend.unitState = func(context.Context, string) (systemUpdateUnitState, error) { return systemUpdateUnitActive, nil }
	backend.observe = nil
	var cleared []string
	backend.resetFailedUnit = func(_ context.Context, id string) error { cleared = append(cleared, id); return nil }

	// The new Agent starts while the worker still runs: nothing is cleared.
	if err := backend.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := backend.Status(context.Background(), update.RequestID); err != nil {
		t.Fatal(err)
	}
	if len(cleared) != 0 {
		t.Fatalf("cleared %v while the update was active", cleared)
	}

	// The worker records the success; the update screen reads the status.
	done := *update
	done.Status = systemUpdateSucceeded
	if err := writeSystemUpdateState(root, &done, nil); err != nil {
		t.Fatal(err)
	}
	backend.unitState = func(context.Context, string) (systemUpdateUnitState, error) { return systemUpdateUnitInactive, nil }
	state, err := backend.Status(context.Background(), update.RequestID)
	if err != nil || state.Status != systemUpdateSucceeded {
		t.Fatalf("status %v %v", state, err)
	}
	want := []string{refused.RequestID, raced.RequestID, update.RequestID}
	if !reflect.DeepEqual(cleared, want) {
		t.Fatalf("cleared %v, want %v", cleared, want)
	}
	// A later poll of the same success clears nothing again.
	cleared = nil
	if _, err := backend.Status(context.Background(), update.RequestID); err != nil {
		t.Fatal(err)
	}
	if len(cleared) != 0 {
		t.Fatalf("a second status read cleared again: %v", cleared)
	}
	for _, s := range []*systemUpdateState{refused, raced, update} {
		if _, err := readSystemUpdateState(root, s.RequestID); err != nil {
			t.Fatalf("the attempt's record %s is gone: %v", s.RequestID, err)
		}
	}
}

// alpha.124 Boston (123 -> 124): the update's worker failed (a transient
// database read after the new panel started), OnFailure release recovery
// started the new Agent at 16:48:08 and finalized the update forward at
// 16:48:28. The start-time Reconcile ran in between: it cleared the older
// resolved unit but kept the attempt failed while recovery's final proof was
// missing, and nothing read it again, so its unit stayed under systemctl
// --failed. The Agent now re-reads such an attempt until recovery finishes.
func TestSystemUpdateAttemptFinalizedByRecoveryAfterAgentStartIsResolved(t *testing.T) {
	root := linuxSystemUpdateTestRoot(t)
	at := func(id, status, created string) *systemUpdateState {
		state := linuxSystemUpdateTestState(strings.Repeat(id, 32))
		state.Status, state.CreatedAt, state.UpdatedAt = status, created, created
		if id == "1" || id == "2" {
			// The earlier attempts targeted the previous release.
			state.TargetVersion, state.TargetCommit, state.TargetSequence = "v1.2.2", strings.Repeat("c", 40), "41"
			state.ExpectedCurrentVersion, state.ExpectedCurrentCommit = "v1.2.1", strings.Repeat("d", 40)
		}
		if status == systemUpdateFailed {
			state.Error = "database publication is unverified"
		}
		if err := writeSystemUpdateState(root, state, nil); err != nil {
			t.Fatal(err)
		}
		return state
	}
	busyEarlier := at("1", systemUpdateFailed, "2026-11-04T09:00:00Z")
	previous := at("2", systemUpdateSucceeded, "2026-11-04T09:10:00Z")
	update := at("3", systemUpdateFailed, "2026-11-05T16:46:40Z")

	backend := newLinuxSystemUpdateBackend()
	backend.stateRoot = root
	backend.floorPath = filepath.Join(filepath.Dir(root), "sequence.floor")
	if err := os.WriteFile(backend.floorPath, canonicalSystemUpdateFloor(systemUpdateFloor{Sequence: update.TargetSequence, Version: update.TargetVersion}), 0o600); err != nil {
		t.Fatal(err)
	}
	backend.now = func() time.Time { return time.Date(2026, 11, 5, 16, 48, 8, 0, time.UTC) }
	backend.installedBuild = func(context.Context) (string, string, error) {
		return update.TargetVersion, update.TargetCommit, nil
	}
	backend.unitState = func(context.Context, string) (systemUpdateUnitState, error) { return systemUpdateUnitInactive, nil }
	backend.observe = nil
	recoveryFinished := false
	backend.finalProof = func(context.Context, *systemUpdateState) error {
		if !recoveryFinished {
			return errors.New("release completion marker is still present")
		}
		return nil
	}
	var cleared []string
	backend.resetFailedUnit = func(_ context.Context, id string) error { cleared = append(cleared, id); return nil }

	// Agent start, recovery still running: the attempt waits, only the
	// earlier resolved units are cleared.
	if err := backend.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if want := []string{busyEarlier.RequestID, previous.RequestID}; !reflect.DeepEqual(cleared, want) {
		t.Fatalf("cleared %v, want %v", cleared, want)
	}
	if !backend.RecoveryPromotionPending() {
		t.Fatal("the attempt waiting for release recovery is not followed")
	}

	// Recovery finishes between two follow-up reads; the follow-up promotes
	// the attempt, clears its unit and stops.
	cleared = nil
	reads := 0
	start := time.Date(2026, 11, 5, 16, 48, 8, 0, time.UTC)
	clock := start
	followSystemUpdateRecovery(backend, backend, start.Add(systemUpdateRecoveryGrace),
		func() time.Time { return clock },
		func(d time.Duration) {
			reads++
			clock = clock.Add(d)
			recoveryFinished = reads == 2
		})
	if reads != 2 {
		t.Fatalf("follow-up reads = %d, want 2 (stops once reconciled)", reads)
	}
	state, err := readSystemUpdateState(root, update.RequestID)
	if err != nil || state.Status != systemUpdateSucceeded {
		t.Fatalf("the attempt finalized by recovery is %v (%v)", state, err)
	}
	if !slices.Contains(cleared, update.RequestID) {
		t.Fatalf("the attempt's own unit mark was not cleared: %v", cleared)
	}
	if backend.RecoveryPromotionPending() {
		t.Fatal("still pending after promotion")
	}

	// A rollback (installed build is not the target) is never followed.
	other := newLinuxSystemUpdateBackend()
	other.stateRoot = root
	other.floorPath = backend.floorPath
	other.now = backend.now
	other.installedBuild = func(context.Context) (string, string, error) { return "v1.2.2", strings.Repeat("c", 40), nil }
	other.unitState = backend.unitState
	other.observe = nil
	other.resetFailedUnit = func(context.Context, string) error { return nil }
	at("4", systemUpdateFailed, "2026-11-05T17:00:00Z")
	if err := other.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if other.RecoveryPromotionPending() {
		t.Fatal("a failed attempt whose target is not installed was followed")
	}

	// The follow-up is bounded by the recovery window.
	never := 0
	backend.finalProof = func(context.Context, *systemUpdateState) error { return errors.New("marker remains") }
	at("5", systemUpdateFailed, "2026-11-05T17:10:00Z")
	backend.installedBuild = func(context.Context) (string, string, error) {
		return update.TargetVersion, update.TargetCommit, nil
	}
	clock = start
	followSystemUpdateRecovery(backend, backend, start.Add(time.Minute),
		func() time.Time { return clock },
		func(d time.Duration) { never++; clock = clock.Add(d) })
	if never != 4 {
		t.Fatalf("bounded follow-up made %d reads, want 4 within one minute", never)
	}
}
