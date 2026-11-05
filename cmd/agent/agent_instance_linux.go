//go:build linux

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"
)

// errAgentAlreadyRunning: another process holds the instance lock.
var errAgentAlreadyRunning = errors.New("another agent holds the instance lock")

// acquireAgentInstanceLock takes the exclusive, non-blocking lock in dir for
// the life of the process (the returned file is kept open; the kernel drops
// the lock when the process ends, however it ends).
func acquireAgentInstanceLock(dir string) (*os.File, string, error) {
	path := filepath.Join(dir, agentInstanceLockName)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, path, fmt.Errorf("create the agent's run directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, path, fmt.Errorf("open the agent instance lock: %w", err)
	}
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		file.Close()
		if errors.Is(err, unix.EWOULDBLOCK) {
			return nil, path, errAgentAlreadyRunning
		}
		return nil, path, fmt.Errorf("lock the agent instance: %w", err)
	}
	return file, path, nil
}

// alignAgentSocketGroup gives the socket its directory's group, so the panel
// can open it whoever started the Agent (systemd's Group=celikpanel or a
// person's root shell).
func alignAgentSocketGroup(socketPath string) error {
	dir, err := os.Stat(filepath.Dir(socketPath))
	if err != nil {
		return err
	}
	dirStat, ok := dir.Sys().(*syscall.Stat_t)
	if !ok {
		return errors.New("socket directory ownership is unreadable")
	}
	socket, err := os.Lstat(socketPath)
	if err != nil {
		return err
	}
	socketStat, ok := socket.Sys().(*syscall.Stat_t)
	if !ok {
		return errors.New("socket ownership is unreadable")
	}
	if socketStat.Gid == dirStat.Gid {
		return nil
	}
	return os.Lchown(socketPath, -1, int(dirStat.Gid))
}
