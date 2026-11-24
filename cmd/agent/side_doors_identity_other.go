//go:build !linux

package main

import "os"

// fileIdentity is not known off Linux: every unix: path then counts as one
// that cannot be followed.
func fileIdentity(os.FileInfo) (string, bool) { return "", false }
