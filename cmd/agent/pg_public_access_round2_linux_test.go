//go:build linux

package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// Second review of alpha.143, B1: a client of a close is found by its
// application_name in the process table, also one an earlier Agent process
// started.
func TestPGPublicAClientIsFoundByItsNameAmongTheProcesses(t *testing.T) {
	application := pgPublicApplication(5432, "s1r_process")
	if running, err := pgPublicProcessesRunning(application); err != nil || running {
		t.Fatalf("before: %v %v", running, err)
	}
	sleeper := exec.Command("sleep", "30")
	sleeper.Env = append(os.Environ(), "PGAPPNAME="+application)
	if err := sleeper.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sleeper.Process.Kill(); _ = sleeper.Wait() }()
	waitUntilProcessIs(t, sleeper.Process.Pid, "sleep")
	if running, err := pgPublicProcessesRunning(application); err != nil || !running {
		t.Fatalf("a process with the close's name was not found: %v %v", running, err)
	}
	if running, _ := pgPublicProcessesRunning(pgPublicApplication(5432, "another")); running {
		t.Fatal("another database's name matched")
	}
}

// waitUntilProcessIs waits until a child Start returned for is really the
// program it starts, running and asleep in it. Start returns while the
// kernel is still inside the exec: the child's own environment is put in
// place only at the end of it, and until then /proc/<pid>/environ reads the
// parent's environment or nothing (this test failed a CI run on 23 November
// 2026 that way). The command name changes early in the exec, so it alone
// is not enough; a process asleep ("S") under the new name has finished it.
func waitUntilProcessIs(t *testing.T, pid int, program string) {
	t.Helper()
	stat := filepath.Join("/proc", strconv.Itoa(pid), "stat")
	deadline := time.Now().Add(10 * time.Second)
	for {
		line, err := os.ReadFile(stat)
		// "<pid> (<comm>) <state> ...": the name is in parentheses.
		if err == nil && bytes.HasPrefix(line, []byte(strconv.Itoa(pid)+" ("+program+") S ")) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("process %d was not %s asleep within 10 seconds: %q %v", pid, program, line, err)
		}
		time.Sleep(time.Millisecond)
	}
}
