//go:build !linux

package main

func hostingFileSystemFacts(path string) (int, bool) { return -1, false }
