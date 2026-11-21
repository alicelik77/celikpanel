//go:build linux

package main

import (
	"context"
	"io"
	"os"
	"testing"
	"time"
)

// s1bMoveHost is a cluster host for the move's engine test. The S1b move
// signs in as the move role through streamLogin; the same test runs against
// the code before S1b, whose host has no such field.
func s1bMoveHost(t *testing.T, e testEngines, client func(ctx context.Context, owner, client, password string, stdin io.Reader, stdout io.Writer, args ...string) error) pgClusterHost {
	return pgClusterHost{
		family: func() string { return "apt" },
		now:    time.Now,
		stream: func(ctx context.Context, owner, name string, stdin io.Reader, stdout io.Writer, args ...string) error {
			return client(ctx, owner, name, "", stdin, stdout, args...)
		},
		streamLogin: client,
		privateDir:  func(string) (string, error) { return os.MkdirTemp(t.TempDir(), "list-") },
		freeBytes:   func(string) (int64, error) { return 1 << 40, nil },
	}
}
