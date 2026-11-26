//go:build !linux

package main

// monotonicClockUSec: no systemd here; a put-back program's time without a
// break is never known, so a status read never settles one.
func monotonicClockUSec() (uint64, bool) { return 0, false }
