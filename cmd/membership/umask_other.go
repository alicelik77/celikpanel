//go:build !unix

package main

// restrictUmask has nothing to do where there is no umask (development builds).
func restrictUmask() {}
