//go:build linux

package main

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// Re-review LOW 2: the Agent's lock is the one the guard's `flock 9` takes.
func TestDatabaseRemoteLockIsSharedWithTheGuard(t *testing.T) {
	flock, err := exec.LookPath("flock")
	if err != nil {
		t.Skip("flock (util-linux) is not installed here")
	}
	restore := databaseRemoteFilterLockPath
	databaseRemoteFilterLockPath = filepath.Join(t.TempDir(), "filter.lock")
	t.Cleanup(func() { databaseRemoteFilterLockPath = restore })
	unlock, err := hostDatabaseRemoteLockFilter()
	if err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("/bin/sh", "-c", "exec 9>"+databaseRemoteFilterLockPath+"; "+flock+" -n 9").Run(); err == nil {
		t.Fatal("the guard's flock got the lock the Agent holds")
	}
	unlock()
	if err := exec.Command("/bin/sh", "-c", "exec 9>"+databaseRemoteFilterLockPath+"; "+flock+" -n 9").Run(); err != nil {
		t.Fatalf("the guard's flock could not take the released lock: %v", err)
	}
}
