//go:build !linux

package main

import (
	"errors"
	"io"
	"os"
)

func runServiceAppAsUser(serviceAppAccount, io.Reader, string, ...string) ([]byte, error) {
	return nil, errors.New("service apps run only on Linux")
}

func unitCgroupPIDs(string) (map[int]bool, error) {
	return nil, errors.New("service apps run only on Linux")
}

func fileOwnedBy(os.FileInfo, uint32) bool { return false }
