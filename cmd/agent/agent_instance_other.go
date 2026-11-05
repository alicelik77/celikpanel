//go:build !linux

package main

import (
	"errors"
	"os"
	"path/filepath"
)

var errAgentAlreadyRunning = errors.New("another agent holds the instance lock")

// The Agent runs only on Linux; other systems build it for tests.
func acquireAgentInstanceLock(dir string) (*os.File, string, error) {
	return nil, filepath.Join(dir, agentInstanceLockName), nil
}

func alignAgentSocketGroup(string) error { return nil }
