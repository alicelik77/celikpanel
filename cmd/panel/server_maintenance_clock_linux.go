//go:build linux

package main

import "golang.org/x/sys/unix"

// The kernel's clock discipline, read with adjtimex and no mode bits set: a
// read only, allowed to an unprivileged process. STA_UNSYNC (0x0040) or the
// TIME_ERROR state (5) means no time service is keeping the clock right; the
// kernel also sets them when the estimated error passes 16 s.
// Saat eşitlemesi: adjtimex ile salt okuma.
const (
	clockStatusUnsync = 0x0040
	clockStateError   = 5
)

func readClockSync() clockSync {
	var tx unix.Timex
	state, err := unix.Adjtimex(&tx)
	if err != nil {
		return clockSync{State: "unknown"}
	}
	maxError := int64(tx.Maxerror) / 1000
	if state == clockStateError || tx.Status&clockStatusUnsync != 0 {
		return clockSync{State: "not_synchronised", MaxErrorMS: &maxError}
	}
	return clockSync{State: "synchronised", MaxErrorMS: &maxError}
}
