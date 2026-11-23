//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly

package membership

import (
	"os"
	"path/filepath"
	"syscall"
)

// lockSession holds an exclusive lock for one session id until the request ends, as
// PHP's files handler holds its session file: a second request with the same cookie
// (a sign-out, for example) waits, so a request still in flight can never write back
// a session that was ended meanwhile. flock locks belong to an open file description,
// so two requests in one process exclude each other too.
func lockSession(dir, id string) func() {
	file, err := os.OpenFile(filepath.Join(dir, "go-lock_"+id), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return func() {}
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		file.Close()
		return func() {}
	}
	return func() {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		file.Close()
	}
}
