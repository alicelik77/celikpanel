package main

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

type clamAVStubCall struct {
	unit  string
	start bool
}

func stubClamAV(t *testing.T, present func() bool, socket bool) *[]clamAVStubCall {
	t.Helper()
	calls := &[]clamAVStubCall{}
	oldEnable, oldExists, oldFile, oldPresent := clamAVEnableUnit, clamAVUnitExists, clamAVUnitFileFound, clamAVSignaturesPresent
	oldWait, oldPoll := clamAVSignatureWait, clamAVSignaturePoll
	t.Cleanup(func() {
		clamAVEnableUnit, clamAVUnitExists, clamAVUnitFileFound, clamAVSignaturesPresent = oldEnable, oldExists, oldFile, oldPresent
		clamAVSignatureWait, clamAVSignaturePoll = oldWait, oldPoll
	})
	clamAVEnableUnit = func(_ context.Context, _ string, unit string, start bool) error {
		*calls = append(*calls, clamAVStubCall{unit, start})
		return nil
	}
	clamAVUnitExists = func(_, name string) bool { return name == "clamav-freshclam" }
	clamAVUnitFileFound = func(_, name string) bool { return socket && name == clamAVDaemonSocket }
	clamAVSignaturesPresent = present
	clamAVSignatureWait, clamAVSignaturePoll = 50*time.Millisecond, 5*time.Millisecond
	return calls
}

// alpha.118 lab, D6: the daemon's start was checked before freshclam had
// downloaded anything, and the install reported a failure. freshclam now
// starts first; signatures that arrive in the wait let the daemon start as
// before.
func TestClamAVSignaturesArriveDuringTheWait(t *testing.T) {
	checks := 0
	calls := stubClamAV(t, func() bool { checks++; return checks > 3 }, true)
	ready, err := clamAVAwaitFirstSignatures(context.Background(), "systemctl", []string{"clamav-freshclam"})
	if err != nil || !ready {
		t.Fatalf("ready=%v err=%v", ready, err)
	}
	if want := []clamAVStubCall{{"clamav-freshclam", true}}; !reflect.DeepEqual(*calls, want) {
		t.Fatalf("calls %v, want %v", *calls, want)
	}
}

// Signatures already present: nothing is started early, nothing waits.
func TestClamAVSignaturesAlreadyPresent(t *testing.T) {
	calls := stubClamAV(t, func() bool { return true }, true)
	if ready, err := clamAVAwaitFirstSignatures(context.Background(), "systemctl", []string{"clamav-freshclam"}); err != nil || !ready {
		t.Fatalf("ready=%v err=%v", ready, err)
	}
	if len(*calls) != 0 {
		t.Fatalf("unexpected calls %v", *calls)
	}
}

// A download that outlasts the wait is "waiting", not a failure: the daemon
// is enabled without a start (systemd would skip it) and the distribution's
// socket is started so freshclam's notification starts the daemon later.
func TestClamAVWaitingArmsTheSocket(t *testing.T) {
	calls := stubClamAV(t, func() bool { return false }, true)
	ready, err := clamAVAwaitFirstSignatures(context.Background(), "systemctl", []string{"clamav-freshclam"})
	if err != nil || ready {
		t.Fatalf("ready=%v err=%v", ready, err)
	}
	if err := clamAVArmFirstStart(context.Background(), "systemctl", "clamav-daemon"); err != nil {
		t.Fatal(err)
	}
	want := []clamAVStubCall{{"clamav-freshclam", true}, {"clamav-daemon", false}, {clamAVDaemonSocket, true}}
	if !reflect.DeepEqual(*calls, want) {
		t.Fatalf("calls %v, want %v", *calls, want)
	}
}

// Without a socket unit the daemon is only enabled; a failing updater is an
// error the install reports.
func TestClamAVWithoutSocketAndFailingUpdater(t *testing.T) {
	calls := stubClamAV(t, func() bool { return false }, false)
	if err := clamAVArmFirstStart(context.Background(), "systemctl", "clamav-daemon"); err != nil {
		t.Fatal(err)
	}
	if want := []clamAVStubCall{{"clamav-daemon", false}}; !reflect.DeepEqual(*calls, want) {
		t.Fatalf("calls %v, want %v", *calls, want)
	}
	clamAVEnableUnit = func(context.Context, string, string, bool) error { return errors.New("freshclam failed") }
	if _, err := clamAVAwaitFirstSignatures(context.Background(), "systemctl", []string{"clamav-freshclam"}); err == nil {
		t.Fatal("a failing updater must be reported")
	}
}
