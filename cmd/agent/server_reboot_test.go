package main

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.128 ops, G29: the Agent's reboot.

type fakeServerReboot struct {
	mu       sync.Mutex
	outcome  string
	acquired int
	released int
	reboots  int
	err      error
	done     chan struct{}
}

func (f *fakeServerReboot) acquireIdle() (func(), string, string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.outcome != transport.ServerRebootScheduled {
		return nil, f.outcome, "busy"
	}
	f.acquired++
	return func() {
		f.mu.Lock()
		f.released++
		f.mu.Unlock()
		close(f.done)
	}, transport.ServerRebootScheduled, ""
}

func (f *fakeServerReboot) reboot() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reboots++
	return f.err
}

func installFakeServerReboot(t *testing.T, fake *fakeServerReboot) {
	t.Helper()
	previous, previousSleep := serverRebootPlatform, serverRebootSleep
	serverRebootPlatform = fake
	serverRebootSleep = func(time.Duration) {}
	serverRebootMu.Lock()
	serverRebootPending = false
	serverRebootMu.Unlock()
	t.Cleanup(func() {
		serverRebootPlatform, serverRebootSleep = previous, previousSleep
		serverRebootMu.Lock()
		serverRebootPending = false
		serverRebootMu.Unlock()
	})
}

func TestServerRebootSchedulesOnceAndRefusesWhenBusy(t *testing.T) {
	agent := &Agent{}
	busy := &fakeServerReboot{outcome: transport.ServerRebootBusyPackageManager, done: make(chan struct{})}
	installFakeServerReboot(t, busy)
	var response transport.ServerRebootResponse
	if err := agent.ServerRebootV1(&transport.ServerRebootRequest{RequestID: "abc"}, &response); err != nil {
		t.Fatal(err)
	}
	if response.Outcome != transport.ServerRebootBusyPackageManager || busy.reboots != 0 {
		t.Fatalf("busy: %+v reboots=%d", response, busy.reboots)
	}

	// A failed systemctl lets the lock go and allows a later request.
	failing := &fakeServerReboot{outcome: transport.ServerRebootScheduled, err: errors.New("no"), done: make(chan struct{})}
	installFakeServerReboot(t, failing)
	if err := agent.ServerRebootV1(&transport.ServerRebootRequest{RequestID: "abc"}, &response); err != nil {
		t.Fatal(err)
	}
	if response.Outcome != transport.ServerRebootScheduled || response.DelaySeconds != 3 {
		t.Fatalf("scheduled: %+v", response)
	}
	<-failing.done
	waitRebootIdle(t)

	// Success: the second request schedules nothing.
	hold := make(chan struct{})
	ok := &fakeServerReboot{outcome: transport.ServerRebootScheduled, done: make(chan struct{})}
	installFakeServerReboot(t, ok)
	calls := 0
	serverRebootSleep = func(time.Duration) {
		calls++
		if calls == 2 {
			<-hold // the machine is going down; the lock stays held
		}
	}
	if err := agent.ServerRebootV1(&transport.ServerRebootRequest{RequestID: "first"}, &response); err != nil || response.Outcome != transport.ServerRebootScheduled {
		t.Fatalf("first: %+v %v", response, err)
	}
	if err := agent.ServerRebootV1(&transport.ServerRebootRequest{RequestID: "second"}, &response); err != nil || response.Outcome != transport.ServerRebootAlreadyScheduled {
		t.Fatalf("second: %+v %v", response, err)
	}
	close(hold)
	<-ok.done
	if ok.acquired != 1 || ok.released != 1 {
		t.Fatalf("lock: acquired=%d released=%d", ok.acquired, ok.released)
	}
}

func waitRebootIdle(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		serverRebootMu.Lock()
		pending := serverRebootPending
		serverRebootMu.Unlock()
		if !pending {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("reboot still pending")
}

func TestServerRebootRefusesAnotherPanelBuild(t *testing.T) {
	previous := buildCommit
	buildCommit = "agentbuild"
	t.Cleanup(func() { buildCommit = previous })
	fake := &fakeServerReboot{outcome: transport.ServerRebootScheduled, done: make(chan struct{})}
	installFakeServerReboot(t, fake)
	var response transport.ServerRebootResponse
	_ = (&Agent{}).ServerRebootV1(&transport.ServerRebootRequest{ExpectedBuildCommit: "panelbuild"}, &response)
	if response.Outcome != transport.ServerRebootUnavailable || response.Error == "" || fake.acquired != 0 {
		t.Fatalf("build gate: %+v", response)
	}
	if sanitizedRebootRequestID("bad id; rm") != "unnamed" || sanitizedRebootRequestID("ab-12") != "ab-12" {
		t.Fatal("request id")
	}
}
