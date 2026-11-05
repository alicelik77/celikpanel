//go:build linux

package main

import (
	"context"
	"path/filepath"
	"reflect"
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
