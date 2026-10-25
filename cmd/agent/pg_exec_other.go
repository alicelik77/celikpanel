//go:build !linux

package main

import (
	"context"
	"errors"
	"io"
	"time"
)

var errPGClustersLinuxOnly = errors.New("PostgreSQL clusters are managed on Linux only")

// defaultPGClusterHost on other systems answers that clusters are not
// offered; nothing runs.
func defaultPGClusterHost() pgClusterHost {
	fail := func(context.Context) (string, error) { return "", errPGClustersLinuxOnly }
	return pgClusterHost{
		family: func() string { return "" },
		// No postgresql-common here: no cluster any check can see.
		lsclusters: func(context.Context) (string, error) { return "", errPGLsclustersMissing },
		query: func(context.Context, string, int, string, string) (string, error) {
			return "", errPGClustersLinuxOnly
		},
		restart:   func(context.Context, string, string) error { return errPGClustersLinuxOnly },
		readFile:  func(string) ([]byte, error) { return nil, errPGClustersLinuxOnly },
		writeFile: func(string, []byte) error { return errPGClustersLinuxOnly },
		listening: func() map[int]bool { return map[int]bool{} },
		distroVersions: func(map[string]bool) (map[string]map[string]bool, error) {
			return nil, errPGClustersLinuxOnly
		},
		vendorIndex: func() (map[string]map[string]bool, bool) { return nil, false },
		installed:   func() map[string][]string { return nil },
		now:         time.Now,
		stream: func(context.Context, string, string, io.Reader, io.Writer, ...string) error {
			return errPGClustersLinuxOnly
		},
		freeBytes: func(string) (int64, error) { return 0, errPGClustersLinuxOnly },
	}
}
