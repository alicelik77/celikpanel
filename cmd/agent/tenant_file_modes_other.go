//go:build !linux

package main

// tightenTenantVisibleModesOnce corrects modes on Linux only, where the
// Agent runs in production.
func tightenTenantVisibleModesOnce() {}
