//go:build !linux

package main

import "os"

// Only Linux runs the Agent; elsewhere the owner is not checked.
func certificateRequestOwnedBySelf(os.FileInfo) bool { return true }
