//go:build !linux

package main

import "errors"

func killFTPLoginSessions(string) error {
	return errors.New("FTP logins need Linux")
}

func ftpLoginMountPresent(string) bool { return false }
