//go:build linux

package hostmutationlock

import (
	"bufio"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// 2026-10-30, Arch run ap-e: setup's mail enrollment worker was refused as
// busy while nothing was changing the host. A readiness probe (ProbeIdle) used
// to take the exclusive flock for an instant; a real writer's non-blocking
// acquisition that fell inside that instant was refused. This pins the
// collision deterministically: a writer acquires in the middle of a probe and
// must succeed, and the probe still answers idle.
func TestProbeInProgressNeverRefusesAWriter(t *testing.T) {
	path, owner := fixture(t)
	writeLock(t, path)
	writerRan := false
	probeObservedHook = func() {
		writerRan = true
		file, err := AcquireExisting(path, owner)
		if err != nil {
			t.Errorf("a writer was refused while a read-only probe was in progress: %v", err)
			return
		}
		file.Close()
	}
	t.Cleanup(func() { probeObservedHook = nil })
	if err := ProbeIdle(path, owner); err != nil {
		t.Fatalf("idle lock: %v", err)
	}
	if !writerRan {
		t.Fatal("the probe did not reach its observation")
	}
	// The same holds for the diagnostic read.
	writerRan = false
	if holders, err := Holders(path, owner); err != nil || len(holders) != 0 || !writerRan {
		t.Fatalf("idle holders: %v %v %t", holders, err, writerRan)
	}
}

// A lock held by this process is busy and names this process.
func TestHoldersNameTheHolderWithoutTakingTheLock(t *testing.T) {
	path, owner := fixture(t)
	writeLock(t, path)
	held, err := AcquireExisting(path, owner)
	if err != nil {
		t.Fatal(err)
	}
	if err = ProbeIdle(path, owner); !errors.Is(err, ErrBusy) {
		t.Fatalf("held lock not busy: %v", err)
	}
	holders, err := Holders(path, owner)
	if err != nil || len(holders) != 1 || holders[0].PID != os.Getpid() || holders[0].Command == "" {
		t.Fatalf("holder of an in-process lock: %+v %v", holders, err)
	}
	if !strings.HasPrefix(DescribeHolders(holders), "process ") {
		t.Fatalf("description: %q", DescribeHolders(holders))
	}
	// Reading the holder did not release or steal the lease.
	if err = VerifyInherited(path, int(held.Fd()), owner); err != nil {
		t.Fatal(err)
	}
	held.Close()
	if holders, err = Holders(path, owner); err != nil || len(holders) != 0 {
		t.Fatalf("released lock still has holders: %+v %v", holders, err)
	}
	// Another lock file held elsewhere is not this lock's holder.
	other, owner2 := fixture(t)
	writeLock(t, other)
	otherHeld, err := AcquireExisting(other, owner2)
	if err != nil {
		t.Fatal(err)
	}
	defer otherHeld.Close()
	if err = ProbeIdle(path, owner); err != nil {
		t.Fatalf("a different held file made this lock busy: %v", err)
	}
}

// A lock held by another process names that process.
func TestHoldersNameAnotherProcess(t *testing.T) {
	path, owner := fixture(t)
	writeLock(t, path)
	cmd := exec.Command(os.Args[0], "-test.run=^TestNativeProcessHelper$")
	cmd.Env = append(os.Environ(), "CP_TEST_PROCESS_LOCK="+path)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cmd.Process.Kill(); cmd.Wait() }()
	ready := make(chan string, 1)
	go func() { line, _ := bufio.NewReader(stdout).ReadString('\n'); ready <- line }()
	select {
	case line := <-ready:
		if strings.TrimSpace(line) != "READY" {
			t.Fatalf("child not ready: %q", line)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("child lock timeout")
	}
	holders, err := Holders(path, owner)
	if err != nil || len(holders) != 1 || holders[0].PID != cmd.Process.Pid {
		t.Fatalf("holder of a child's lock: %+v %v (child %d)", holders, err, cmd.Process.Pid)
	}
	if err = ProbeIdle(path, owner); !errors.Is(err, ErrBusy) {
		t.Fatalf("child's lock not busy: %v", err)
	}
}

// Without a readable lock table the probe keeps the earlier momentary flock,
// so it answers correctly; the diagnostic says it cannot name a holder.
func TestProbeWithoutLockTableFallsBack(t *testing.T) {
	path, owner := fixture(t)
	writeLock(t, path)
	procRoot = t.TempDir()
	t.Cleanup(func() { procRoot = "/proc" })
	if err := ProbeIdle(path, owner); err != nil {
		t.Fatalf("idle without lock table: %v", err)
	}
	held, err := AcquireExisting(path, owner)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	if err = ProbeIdle(path, owner); !errors.Is(err, ErrBusy) {
		t.Fatalf("held without lock table: %v", err)
	}
	if _, err = Holders(path, owner); err == nil {
		t.Fatal("holders claimed without a lock table")
	}
}
