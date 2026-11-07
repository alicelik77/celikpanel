//go:build !linux

package main

import (
	"context"
	"errors"
	"io"
)

var runDatabaseClientAsSiteUser = func(ctx context.Context, username string, argv []string, env []string, stdin io.Reader) (string, error) {
	return "", errors.New("loading a database file is supported on Linux only")
}
