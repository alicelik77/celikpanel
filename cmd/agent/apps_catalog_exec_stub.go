//go:build !linux

package main

import (
	"context"
	"errors"
)

func runAsSiteUser(_ context.Context, _, _ string, _ []string, _ []byte) (string, error) {
	return "", errors.New("application setup is supported only on Linux")
}

func freeDiskMB(_ string) int64 { return 0 }
