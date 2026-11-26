//go:build linux

package main

import "golang.org/x/sys/unix"

// monotonicClockUSec is CLOCK_MONOTONIC in microseconds: the clock systemd's
// *TimestampMonotonic properties are read on (alpha.149 second fix round,
// R1: how long a put-back program has run without a break).
func monotonicClockUSec() (uint64, bool) {
	var ts unix.Timespec
	if err := unix.ClockGettime(unix.CLOCK_MONOTONIC, &ts); err != nil {
		return 0, false
	}
	return uint64(ts.Sec)*1e6 + uint64(ts.Nsec)/1e3, true
}
