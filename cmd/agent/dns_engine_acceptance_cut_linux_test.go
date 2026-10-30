//go:build linux && acceptance_license && !dns_kill_matrix

package main

import (
	"errors"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/dnsengineartifact"
)

func TestParseAcceptanceCutSpec(t *testing.T) {
	spec, ok, err := parseAcceptanceCutSpec("pdns-switch after_write target-enable-intent reset\n")
	if err != nil || !ok || spec != (acceptanceCutSpec{Driver: "pdns-switch", Point: "after_write", Phase: "target-enable-intent", Action: "reset"}) {
		t.Fatalf("spec=%+v ok=%v err=%v", spec, ok, err)
	}
	for _, bad := range []string{"", "pdns-switch after_write target-enable-intent", "nope after_write intent reset",
		"pdns-switch during target-enable-intent reset", "pdns-switch after_write nonsense reset", "pdns-switch after_write intent reboot now"} {
		if _, ok, err := parseAcceptanceCutSpec(bad); ok || err == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
}

// The hook ignores other boundaries, then at the named one records, disarms,
// flushes and resets, in that order, and never returns to the journal writer.
func TestAcceptanceCutHookFiresOnlyAtItsBoundary(t *testing.T) {
	var events []string
	armed := true
	stopped := make(chan struct{})
	ops := acceptanceCutOps{
		readArm: func() (acceptanceCutSpec, bool, error) {
			if !armed {
				return acceptanceCutSpec{}, false, nil
			}
			return acceptanceCutSpec{Driver: "pdns-switch", Point: "after_write", Phase: dnsengineartifact.SwitchPhaseTargetEnableIntent, Action: "reset"}, true, nil
		},
		record: func(r acceptanceCutRecord) error {
			events = append(events, "record:"+r.Phase+":"+r.MutationRequestID)
			return nil
		},
		disarm: func() error { armed = false; events = append(events, "disarm"); return nil },
		sync:   func() { events = append(events, "sync") },
		reset:  func() error { events = append(events, "reset"); return errors.New("not in a test") },
		kill: func() error {
			events = append(events, "kill")
			close(stopped)
			select {}
		},
		now: time.Now,
	}
	hook := acceptanceCutHook(ops)
	journal := dnsEngineSwitchJournal{Phase: dnsengineartifact.SwitchPhaseTargetStaged, MutationRequestID: "r"}
	if err := hook("pdns-switch", "after_write", journal); err != nil || len(events) != 0 {
		t.Fatalf("fired at another phase: %v %v", err, events)
	}
	journal.Phase = dnsengineartifact.SwitchPhaseTargetEnableIntent
	if err := hook("pdns-switch", "before_write", journal); err != nil || len(events) != 0 {
		t.Fatalf("fired at another point: %v %v", err, events)
	}
	go func() { _ = hook("pdns-switch", "after_write", journal) }()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("cut did not stop at its boundary")
	}
	want := []string{"record:target-enable-intent:r", "disarm", "sync", "reset", "kill"}
	if len(events) != len(want) {
		t.Fatalf("events=%v", events)
	}
	for i := range want {
		if events[i] != want[i] {
			t.Fatalf("events=%v", events)
		}
	}
}

func TestAcceptanceCutHookRefusesWhenItCannotRecord(t *testing.T) {
	ops := acceptanceCutOps{
		readArm: func() (acceptanceCutSpec, bool, error) {
			return acceptanceCutSpec{Driver: "bind", Point: "after_write", Phase: dnsengineartifact.SwitchPhaseIntent, Action: "kill"}, true, nil
		},
		record: func(acceptanceCutRecord) error { return errors.New("read-only") },
		now:    time.Now,
	}
	if err := acceptanceCutHook(ops)("bind", "after_write", dnsEngineSwitchJournal{Phase: dnsengineartifact.SwitchPhaseIntent}); err == nil {
		t.Fatal("an unrecorded cut continued past its boundary")
	}
}
