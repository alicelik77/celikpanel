package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// D8 (final alpha.113 acceptance run 4): after a reboot the startup
// reconciliation was refused because systemd still reported "starting", and
// nothing ran it again. Panel start now waits, bounded, until the host
// identity can be read; the wait only repeats the read.

func startupHostStartingRefusal() error {
	return fmt.Errorf("%w: startup: %w", errAgentRPCPlatformIdentityUnavailable,
		errors.New(`read HostPlatform: detect host platform: systemd is not ready: systemd state "starting" exited with status 1`))
}

type startupHostStartingFixture struct {
	answers []error
	probes  int
	pauses  []time.Duration
	clock   time.Time
	lines   []string
	cancel  bool
}

func (f *startupHostStartingFixture) wait() startupHostStartingWait {
	f.clock = time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)
	return startupHostStartingWait{
		bound:    2 * time.Minute,
		minPause: time.Second,
		maxPause: 10 * time.Second,
		attempt:  time.Second,
		probe: func(context.Context) error {
			f.probes++
			if len(f.answers) == 0 {
				return startupHostStartingRefusal()
			}
			answer := f.answers[0]
			f.answers = f.answers[1:]
			return answer
		},
		sleep: func(_ context.Context, pause time.Duration) bool {
			if f.cancel {
				return false
			}
			f.pauses = append(f.pauses, pause)
			f.clock = f.clock.Add(pause)
			return true
		},
		now:  func() time.Time { return f.clock },
		logf: func(format string, args ...any) { f.lines = append(f.lines, fmt.Sprintf(format, args...)) },
	}
}

func TestStartupWaitsWhileTheHostIsStillStartingThenContinues(t *testing.T) {
	f := &startupHostStartingFixture{answers: []error{startupHostStartingRefusal(), startupHostStartingRefusal(), nil}}
	if !f.wait().run(context.Background()) {
		t.Fatal("the wait did not report the host started")
	}
	if f.probes != 3 {
		t.Fatalf("probes=%d, want 3", f.probes)
	}
	if len(f.pauses) != 2 || f.pauses[0] != time.Second || f.pauses[1] != 2*time.Second {
		t.Fatalf("pauses=%v, want backoff 1s, 2s", f.pauses)
	}
	if len(f.lines) != 2 {
		t.Fatalf("lines=%q, want the wait and the outcome", f.lines)
	}
	for _, want := range []string{"still starting", "Nothing was changed", "nobody needs to act", "up to 2m0s", `systemd state "starting"`} {
		if !strings.Contains(f.lines[0], want) {
			t.Fatalf("wait line lacks %q: %s", want, f.lines[0])
		}
	}
	if !strings.Contains(f.lines[1], "finished starting after 3s") || !strings.Contains(f.lines[1], "continues") {
		t.Fatalf("outcome line: %s", f.lines[1])
	}
}

func TestStartupWaitForAStartedHostIsSilent(t *testing.T) {
	for _, answer := range []error{
		nil,
		// Another refusal is not a boot state: the startup steps report it
		// themselves, as before; startup does not wait for it.
		fmt.Errorf("%w: startup: %w", errAgentRPCPlatformIdentityUnavailable,
			errors.New(`systemd is not ready: systemd state "maintenance" exited with status 1`)),
		fmt.Errorf("%w: startup: %w", errAgentRPCPlatformIdentityUnavailable, errAgentRPCClientUnavailable),
		errors.New("verify panel-agent build pair: mismatch"),
	} {
		f := &startupHostStartingFixture{answers: []error{answer}}
		if !f.wait().run(context.Background()) {
			t.Fatalf("%v: the wait reported the host still starting", answer)
		}
		if f.probes != 1 || len(f.pauses) != 0 || len(f.lines) != 0 {
			t.Fatalf("%v: probes=%d pauses=%v lines=%q", answer, f.probes, f.pauses, f.lines)
		}
	}
}

func TestStartupWaitEndsAtItsBoundWithTheOwnersWayOut(t *testing.T) {
	f := &startupHostStartingFixture{}
	if f.wait().run(context.Background()) {
		t.Fatal("a host that never finished starting was reported started")
	}
	var waited time.Duration
	for _, pause := range f.pauses {
		if pause > 10*time.Second {
			t.Fatalf("pause %s exceeds the cap", pause)
		}
		waited += pause
	}
	if waited < 2*time.Minute || waited > 2*time.Minute+10*time.Second {
		t.Fatalf("waited %s, want the 2 minute bound", waited)
	}
	if len(f.lines) != 2 {
		t.Fatalf("lines=%q, want the wait and the final reason", f.lines)
	}
	final := f.lines[1]
	for _, want := range []string{
		"still starting after 2m0s", "refused before they change anything", "next Panel start",
		"Sites keep serving", "server administrator", "systemctl is-system-running", "systemctl list-jobs",
		"sudo systemctl restart celikpanel-panel", `systemd state "starting"`,
	} {
		if !strings.Contains(final, want) {
			t.Fatalf("final line lacks %q: %s", want, final)
		}
	}
}

func TestStartupWaitStopsWhenThePanelIsStopped(t *testing.T) {
	f := &startupHostStartingFixture{cancel: true}
	if f.wait().run(context.Background()) {
		t.Fatal("a stopped wait reported the host started")
	}
	if f.probes != 1 || len(f.lines) != 2 {
		t.Fatalf("probes=%d lines=%q", f.probes, f.lines)
	}
}

// The probe is the identity read every host-changing call makes, wrapped
// so the one classifier decides; a Panel without an Agent is not "starting".
func TestStartupHostIdentityProbeWrapsTheRefusal(t *testing.T) {
	p := &Panel{}
	err := p.startupHostIdentityProbe(context.Background())
	if err == nil || !errors.Is(err, errAgentRPCPlatformIdentityUnavailable) {
		t.Fatalf("probe without an Agent: %v", err)
	}
	if startupHostStillStarting(err) {
		t.Fatalf("a missing Agent was classified as a booting host: %v", err)
	}
	if !startupHostStillStarting(startupHostStartingRefusal()) {
		t.Fatal("the run 4 refusal was not classified as a booting host")
	}
	var lines []string
	if !p.waitForHostStartedAtStartup(context.Background(), func(format string, args ...any) {
		lines = append(lines, fmt.Sprintf(format, args...))
	}) || len(lines) != 0 {
		t.Fatalf("a Panel without an Agent waited: %q", lines)
	}
}
