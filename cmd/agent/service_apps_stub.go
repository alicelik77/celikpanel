//go:build !linux

package main

import (
	"errors"
	"io"
)

func runServiceAppAsUser(serviceAppAccount, io.Reader, string, ...string) ([]byte, error) {
	return nil, errors.New("service apps run only on Linux")
}

func unitCgroupPIDs(string) (map[int]bool, error) {
	return nil, errors.New("service apps run only on Linux")
}
