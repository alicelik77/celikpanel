//go:build !(linux || darwin || freebsd || netbsd || openbsd || dragonfly)

package membership

// lockSession is a no-op where there is no flock (development builds only; the
// service runs on Linux).
func lockSession(string, string) func() { return func() {} }
