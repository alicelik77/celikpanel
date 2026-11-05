//go:build !linux

package main

// Outside Linux no fingerprint can be read, so nothing is cached and every
// inventory read is live. Linux dışında önbellek kullanılmaz.
func hostProfileFingerprint() string             { return "" }
func packageDatabaseFingerprint(_ string) string { return "" }
func systemdUnitFingerprint() string             { return "" }
