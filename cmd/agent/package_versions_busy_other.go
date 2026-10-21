//go:build !linux

package main

// The installed package version read is supported only on Linux; elsewhere
// nothing holds a package lock. Yalnız Linux'ta desteklenir.
func realPackageDatabaseReadBusy() (bool, error) {
	return false, nil
}
