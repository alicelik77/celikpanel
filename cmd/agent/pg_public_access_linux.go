//go:build linux

package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

// pgBoundedProcess makes a bounded client end cleanly at its deadline
// (second review of alpha.143, B1): it runs in its own process group, and
// the deadline sends SIGINT to the group (sudo passes it on; psql cancels
// the statement it waits on, stops the script and exits, its output
// complete). A client still there 10 seconds later is killed.
func pgBoundedProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGINT)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	cmd.WaitDelay = 10 * time.Second
}

// pgPublicProcessesRunning reports whether any process carries
// PGAPPNAME=<application> in its environment: a client of a close or undo,
// also one an earlier Agent process started and no longer waits for.
func pgPublicProcessesRunning(application string) (bool, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return false, err
	}
	want := []byte("PGAPPNAME=" + application)
	for _, entry := range entries {
		if _, err := strconv.Atoi(entry.Name()); err != nil {
			continue
		}
		environ, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "environ"))
		if err != nil {
			continue // gone, or a kernel thread
		}
		for _, variable := range bytes.Split(environ, []byte{0}) {
			if bytes.Equal(variable, want) {
				return true, nil
			}
		}
	}
	return false, nil
}
