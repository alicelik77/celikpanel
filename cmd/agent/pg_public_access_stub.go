//go:build !linux

package main

import "os/exec"

// pgBoundedProcess: the deadline ends the client (the Agent runs on Linux).
func pgBoundedProcess(cmd *exec.Cmd) {}

// pgPublicProcessesRunning: no process table to read off Linux.
func pgPublicProcessesRunning(application string) (bool, error) { return false, nil }
