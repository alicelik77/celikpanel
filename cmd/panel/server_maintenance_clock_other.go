//go:build !linux

package main

func readClockSync() clockSync {
	return clockSync{State: "unknown"}
}
