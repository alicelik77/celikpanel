//go:build !linux

package main

import (
	"errors"

	"github.com/alicelik/celikpanel/internal/fileaccess"
)

func statFileAccessChain([]string) ([]fileaccess.DirInfo, error) {
	return nil, errors.New("file access is only configured on Linux servers")
}

func missingJailCommands() []string { return nil }

func ensureFileAccessJail(string, []fileaccess.JailUser) ([]string, bool, error) {
	return nil, false, errors.New("file access is only configured on Linux servers")
}
