//go:build !linux

package main

import "errors"

func productionSSHPasswordLoginEnv() (sshPasswordLoginEnv, error) {
	return sshPasswordLoginEnv{}, errors.New("SSH password sign-in is managed on Linux only")
}
