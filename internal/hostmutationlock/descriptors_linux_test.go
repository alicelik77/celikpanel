//go:build linux

package hostmutationlock

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// requireNoHolders accepts "nobody holds it" from the diagnostic read only
// where this process can read every process's descriptors; elsewhere (an
// unprivileged test run) the same free lock must answer ErrUnknown: a read
// that could not confirm that nobody holds the lock never says so.
func requireNoHolders(t *testing.T, path string, owner Owner, what string) {
	t.Helper()
	holders, err := Holders(path, owner)
	if len(holders) != 0 {
		t.Fatalf("%s: holders %+v", what, holders)
	}
	if effectiveCapability(capSysPtrace) {
		if err != nil {
			t.Fatalf("%s: want no holder, got %v", what, err)
		}
		return
	}
	if !errors.Is(err, ErrUnknown) {
		t.Fatalf("%s without CAP_SYS_PTRACE: want ErrUnknown, got %v", what, err)
	}
}

// startChildHolder starts a separate process that holds the lock until the test
// ends (it has proved the exclusion before it says READY) and returns its pid.
func startChildHolder(t *testing.T, path string) int {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestNativeProcessHoldsUntilReleased$")
	cmd.Env = append(os.Environ(), "CP_TEST_HOLD_LOCK="+path)
	release, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { release.Close(); cmd.Wait() })
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
	return cmd.Process.Pid
}

// TestNativeProcessHoldsUntilReleased is startChildHolder's process: it holds
// the lock until its standard input closes, so no test depends on how long a
// fixed hold lasts.
func TestNativeProcessHoldsUntilReleased(t *testing.T) {
	path := os.Getenv("CP_TEST_HOLD_LOCK")
	if path == "" {
		t.Skip("subprocess only")
	}
	owner := Owner{uint32(os.Geteuid()), uint32(os.Getegid())}
	// The kernel lists locks per CPU, in CPU order, newest first. Taking this
	// lock on the highest allowed CPU puts every lock other threads take and
	// drop ahead of it in /proc/locks, where their release can shift its line.
	runtime.LockOSThread()
	var allowed unix.CPUSet
	if unix.SchedGetaffinity(0, &allowed) == nil {
		last := -1
		for cpu := 0; cpu < len(allowed)*64; cpu++ {
			if allowed.IsSet(cpu) {
				last = cpu
			}
		}
		if last >= 0 {
			var pinned unix.CPUSet
			pinned.Set(last)
			unix.SchedSetaffinity(0, &pinned)
		}
	}
	file, err := AcquireExisting(path, owner)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err = VerifyInherited(path, int(file.Fd()), owner); err != nil {
		t.Fatal(err)
	}
	os.Stdout.WriteString("READY\n")
	io.Copy(io.Discard, os.Stdin)
}

// fakeProc is a procfs view for one test: the real lock table or an empty one,
// this process's own entries, and only the process entries the test names.
// Entries are links into the real /proc, or broken entries a walk cannot read.
func fakeProc(t *testing.T, realLockTable bool, pids []int, unreadable ...string) {
	t.Helper()
	root := t.TempDir()
	if realLockTable {
		if err := os.Symlink("/proc/locks", filepath.Join(root, "locks")); err != nil {
			t.Fatal(err)
		}
	} else if err := os.WriteFile(filepath.Join(root, "locks"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/proc/self", filepath.Join(root, "self")); err != nil {
		t.Fatal(err)
	}
	for _, pid := range pids {
		if err := os.Symlink(fmt.Sprintf("/proc/%d", pid), filepath.Join(root, strconv.Itoa(pid))); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range unreadable {
		// "fd" is a regular file: listing it fails with ENOTDIR, which is not a
		// process that ended, so the walk cannot be complete.
		if err := os.Mkdir(filepath.Join(root, name), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name, "fd"), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	procRoot = root
	t.Cleanup(func() { procRoot = "/proc" })
}

// The lock table skipped the holder's line (here: an empty table, which is
// exactly what a skipped line looks like to the reader). The descriptor walk
// still finds the child, so the probe is busy and the holder is named.
func TestDescriptorWalkFindsAHolderTheLockTableMissed(t *testing.T) {
	path, owner := fixture(t)
	writeLock(t, path)
	child := startChildHolder(t, path)
	fakeProc(t, false, []int{os.Getpid(), child})
	if err := ProbeIdle(path, owner); !errors.Is(err, ErrBusy) {
		t.Fatalf("holder missing from the lock table: want busy, got %v", err)
	}
	holders, err := Holders(path, owner)
	if err != nil || len(holders) != 1 || holders[0].PID != child {
		t.Fatalf("holder missing from the lock table: %+v %v (child %d)", holders, err, child)
	}
	// A holder found is busy even when some other process could not be read.
	fakeProc(t, false, []int{child}, "1")
	if err := ProbeIdle(path, owner); !errors.Is(err, ErrBusy) {
		t.Fatalf("found holder with an incomplete walk: want busy, got %v", err)
	}
}

// When nobody is found but some process could not be read, the holder is not
// established: the diagnostic read says ErrUnknown and never that nobody holds
// it. The probe then decides with the momentary flock, which answers exactly:
// idle for a free lock, busy for a held one the walk could not see.
func TestIncompleteDescriptorWalkDecidesWithTheMomentaryFlock(t *testing.T) {
	path, owner := fixture(t)
	writeLock(t, path)
	fakeProc(t, false, []int{os.Getpid()}, "1")
	if err := ProbeIdle(path, owner); err != nil {
		t.Fatalf("free lock, unreadable process: want idle, got %v", err)
	}
	if holders, err := Holders(path, owner); len(holders) != 0 || !errors.Is(err, ErrUnknown) {
		t.Fatalf("free lock, unreadable process, diagnostic read: %+v %v", holders, err)
	}
	child := startChildHolder(t, path)
	// The lock table is empty and the walk sees neither the child nor process 1.
	fakeProc(t, false, []int{os.Getpid()}, "1")
	if err := ProbeIdle(path, owner); !errors.Is(err, ErrBusy) {
		t.Fatalf("held lock, incomplete walk: want busy, got %v", err)
	}
	if holders, err := Holders(path, owner); len(holders) != 0 || !errors.Is(err, ErrUnknown) {
		t.Fatalf("held lock, incomplete walk, diagnostic read: %+v %v", holders, err)
	}
	// A walk that runs out of its budget before it reaches the child decides
	// the same way.
	fakeProc(t, false, []int{child})
	previous := descriptorWalkBudget
	descriptorWalkBudget = -time.Second
	t.Cleanup(func() { descriptorWalkBudget = previous })
	if err := ProbeIdle(path, owner); !errors.Is(err, ErrBusy) {
		t.Fatalf("held lock, walk past its budget: want busy, got %v", err)
	}
}

// noPtraceProc is a procfs view for a process without CAP_SYS_PTRACE: another
// user's descriptors, and on a hidepid /proc the process itself, are invisible,
// so the walk can never be complete. The lock table is empty, as when it
// skipped the holder's line.
func noPtraceProc(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "locks"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	self := filepath.Join(root, "self")
	if err := os.Mkdir(self, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"fdinfo", "mountinfo"} {
		if err := os.Symlink("/proc/self/"+name, filepath.Join(self, name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(self, "status"), []byte("Name:\tprobe\nCapEff:\t0000000000000000\n"), 0600); err != nil {
		t.Fatal(err)
	}
	procRoot = root
	t.Cleanup(func() { procRoot = "/proc" })
}

// A host where the walk can never be complete (a platform or hardening profile
// that drops CAP_SYS_PTRACE) still gets exact answers, so its updates are not
// stopped by an answer that can never be established.
func TestProbeWithoutPtraceCapabilityStillAnswersExactly(t *testing.T) {
	path, owner := fixture(t)
	writeLock(t, path)
	noPtraceProc(t)
	if err := ProbeIdle(path, owner); err != nil {
		t.Fatalf("free lock without CAP_SYS_PTRACE: want idle, got %v", err)
	}
	startChildHolder(t, path)
	if err := ProbeIdle(path, owner); !errors.Is(err, ErrBusy) {
		t.Fatalf("held lock without CAP_SYS_PTRACE: want busy, got %v", err)
	}
	if holders, err := Holders(path, owner); len(holders) != 0 || !errors.Is(err, ErrUnknown) {
		t.Fatalf("held lock without CAP_SYS_PTRACE, diagnostic read: %+v %v", holders, err)
	}
}

// The exception to "a probe never makes a writer busy", and its exact extent:
// where the walk cannot be completed, the momentary flock holds the lock for an
// instant, and a writer's non-blocking acquisition inside that instant is
// refused as busy. Only inside it: the probe still answers idle and the writer
// acquires the lock right after. Where the walk completes (CAP_SYS_PTRACE, as
// the Agent runs), no flock is taken at all.
func TestMomentaryFallbackRefusesAWriterOnlyInsideItsInstant(t *testing.T) {
	path, owner := fixture(t)
	writeLock(t, path)
	noPtraceProc(t)
	inside := errors.New("the momentary flock was not taken")
	momentaryProbeHeldHook = func() {
		file, err := AcquireExisting(path, owner)
		if file != nil {
			file.Close()
		}
		inside = err
	}
	t.Cleanup(func() { momentaryProbeHeldHook = nil })
	if err := ProbeIdle(path, owner); err != nil {
		t.Fatalf("free lock: want idle, got %v", err)
	}
	if !errors.Is(inside, ErrBusy) {
		t.Fatalf("writer inside the momentary flock: want busy, got %v", inside)
	}
	momentaryProbeHeldHook = nil
	file, err := AcquireExisting(path, owner)
	if err != nil {
		t.Fatalf("writer after the probe: %v", err)
	}
	file.Close()
	procRoot = "/proc"
	if !effectiveCapability(capSysPtrace) {
		return
	}
	momentaryProbeHeldHook = func() { t.Error("a complete walk took the flock") }
	if err := ProbeIdle(path, owner); err != nil {
		t.Fatalf("free lock, complete walk: want idle, got %v", err)
	}
}

// alpha.135 CI (TestHoldersNameAnotherProcess, "child's lock not busy: <nil>"):
// /proc/locks is produced one read() chunk at a time and resumes by line
// number, so while other locks are taken and released a holder's line can be
// skipped and the probe answered idle although the child held the lock. This
// creates that churn on purpose, while every probe must name the child: twenty
// held locks ahead of the child's line, so the table is read in a few small
// chunks (a table of several pages is read a page at a time and misses far
// less often), and threads taking and dropping others. On the code before
// alpha.136 every run of this test failed in the guest (see the PR).
func TestProbeNeverMissesAHolderWhileOtherLocksComeAndGo(t *testing.T) {
	path, owner := fixture(t)
	writeLock(t, path)
	child := startChildHolder(t, path)
	directory := t.TempDir()
	open := func(name string) *os.File {
		file, err := os.OpenFile(filepath.Join(directory, name), os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { file.Close() })
		return file
	}
	for i := 0; i < 20; i++ {
		if err := unix.Flock(int(open(fmt.Sprintf("held-%d", i)).Fd()), unix.LOCK_EX); err != nil {
			t.Fatal(err)
		}
	}
	stop := make(chan struct{})
	var churners sync.WaitGroup
	// Four threads keep the table moving without starving the probe on a small
	// runner; each takes and drops its own five locks as fast as it can.
	for w := 0; w < 4; w++ {
		files := make([]*os.File, 5)
		for i := range files {
			files[i] = open(fmt.Sprintf("churn-%d-%d", w, i))
		}
		churners.Add(1)
		go func() {
			defer churners.Done()
			runtime.LockOSThread()
			for {
				select {
				case <-stop:
					return
				default:
				}
				for _, file := range files {
					unix.Flock(int(file.Fd()), unix.LOCK_EX)
				}
				for _, file := range files {
					unix.Flock(int(file.Fd()), unix.LOCK_UN)
				}
			}
		}()
	}
	const probes = 600
	missedProbe, missedHolder := 0, 0
	var first error
	started := time.Now()
	for i := 0; i < probes; i++ {
		if err := ProbeIdle(path, owner); !errors.Is(err, ErrBusy) {
			missedProbe++
			if first == nil {
				first = fmt.Errorf("probe %d: %v", i, err)
			}
		}
		holders, err := Holders(path, owner)
		named := false
		for _, holder := range holders {
			named = named || holder.PID == child
		}
		if err != nil || !named {
			missedHolder++
			if first == nil {
				first = fmt.Errorf("holders %d: %+v %v", i, holders, err)
			}
		}
	}
	close(stop)
	churners.Wait()
	t.Logf("%d probes and diagnostic reads in %s", probes, time.Since(started).Round(time.Millisecond))
	if missedProbe != 0 || missedHolder != 0 {
		t.Fatalf("while other locks came and went, %d of %d probes did not answer busy and %d did not name the child %d (first: %v)", missedProbe, probes, missedHolder, child, first)
	}
}
