//go:build !linux

package main

// databaseDirectoryIdentity: no birth time is read off Linux; the identity
// is not known.
func databaseDirectoryIdentity(string) string { return "" }
