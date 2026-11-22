//go:build linux

package main

import (
	"os"
	"os/exec"
	"testing"
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
	if running, err := pgPublicProcessesRunning(application); err != nil || !running {
		t.Fatalf("a process with the close's name was not found: %v %v", running, err)
	}
	if running, _ := pgPublicProcessesRunning(pgPublicApplication(5432, "another")); running {
		t.Fatal("another database's name matched")
	}
}
