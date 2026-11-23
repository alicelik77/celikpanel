//go:build unix

package main

import "syscall"

// restrictUmask is http.php's umask(0077): the database, its WAL files and the
// session files are readable by the service's own account only.
func restrictUmask() { syscall.Umask(0o077) }
