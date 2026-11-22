//go:build !linux

package main

// tightenTenantVisibleModesOnce corrects modes on Linux only, where the
// Agent runs in production.
func tightenTenantVisibleModesOnce() {}

// passLogRootOnce corrects the log folder on Linux only.
func passLogRootOnce() {}
