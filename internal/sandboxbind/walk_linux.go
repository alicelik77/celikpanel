//go:build linux

package sandboxbind

import (
	"errors"
	"fmt"
	"path"

	"golang.org/x/sys/unix"
)

// resolveBeneath is how every name below the start is resolved: beneath the
// folder it is opened from, no link of any kind (RESOLVE_NO_SYMLINKS refuses
// a link even as the last part, so O_NOFOLLOW cannot hand back the link
// itself), no /proc magic link.
const resolveBeneath = unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS

// openStep opens one folder beneath dirFD and checks it.
func openStep(dirFD int, step Step, at string) (int, unix.Stat_t, error) {
	var st unix.Stat_t
	if step.Name == "" || step.Name == "." || step.Name == ".." || path.Base(step.Name) != step.Name {
		return -1, st, refused(CodeNotReal, at, "is not a plain folder name")
	}
	resolve := uint64(resolveBeneath)
	if step.SameMount {
		resolve |= unix.RESOLVE_NO_XDEV
	}
	fd, err := unix.Openat2(dirFD, step.Name, &unix.OpenHow{
		Flags: unix.O_PATH | unix.O_DIRECTORY | unix.O_NOFOLLOW | unix.O_CLOEXEC, Resolve: resolve})
	switch {
	case errors.Is(err, unix.ELOOP):
		return -1, st, refused(CodeNotReal, at, "is a link, not a real folder")
	case errors.Is(err, unix.ENOTDIR):
		// O_DIRECTORY answers before the link check; say which it is.
		var lst unix.Stat_t
		if unix.Fstatat(dirFD, step.Name, &lst, unix.AT_SYMLINK_NOFOLLOW) == nil && lst.Mode&unix.S_IFMT == unix.S_IFLNK {
			return -1, st, refused(CodeNotReal, at, "is a link, not a real folder")
		}
		return -1, st, refused(CodeNotReal, at, "is not a folder")
	case errors.Is(err, unix.ENOENT):
		return -1, st, refused(CodeNotReal, at, "is missing")
	case errors.Is(err, unix.EXDEV):
		return -1, st, refused(CodeNotReal, at, "is a mount point of another file system")
	case errors.Is(err, unix.ENOSYS):
		return -1, st, refused(CodeUnreadable, at, "could not be checked: this kernel has no openat2 (Linux 5.6 or later is needed)")
	case err != nil:
		return -1, st, refused(CodeUnreadable, at, "could not be opened: %v", err)
	}
	if err := checkFolder(fd, step.UID, step.Private, at, &st); err != nil {
		unix.Close(fd)
		return -1, st, err
	}
	return fd, st, nil
}

func checkFolder(fd int, uid uint32, private bool, at string, st *unix.Stat_t) error {
	if err := unix.Fstat(fd, st); err != nil {
		return refused(CodeUnreadable, at, "could not be read: %v", err)
	}
	if st.Mode&unix.S_IFMT != unix.S_IFDIR {
		return refused(CodeNotReal, at, "is not a folder")
	}
	if st.Uid != uid {
		return refused(CodeNotOwned, at, "belongs to uid %d, not uid %d", st.Uid, uid)
	}
	if private && st.Mode&0o022 != 0 {
		return refused(CodeNotOwned, at, "can be changed by other accounts (mode %04o)", st.Mode&0o7777)
	}
	return nil
}

// Open walks the plan and returns the app folder, opened O_PATH, and what
// fstat read of it. Nothing below Start is resolved by name more than one
// folder at a time, and never through a link.
func Open(plan Plan) (int, unix.Stat_t, error) {
	var st unix.Stat_t
	fd, err := unix.Open(plan.Start, unix.O_PATH|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, st, refused(CodeUnreadable, plan.Start, "could not be opened: %v", err)
	}
	if err := checkFolder(fd, plan.StartUID, true, plan.Start, &st); err != nil {
		unix.Close(fd)
		return -1, st, err
	}
	at := plan.Start
	for _, step := range plan.Steps {
		at = path.Join(at, step.Name)
		next, nextSt, err := openStep(fd, step, at)
		unix.Close(fd)
		if err != nil {
			return -1, st, err
		}
		fd, st = next, nextSt
	}
	return fd, st, nil
}

// Check walks the plan and closes the folder again: the Agent's own check
// before it writes or starts a unit (the helper checks again at every start).
func Check(plan Plan) error {
	fd, _, err := Open(plan)
	if err != nil {
		return err
	}
	unix.Close(fd)
	return nil
}

// readEnv reads rel beneath the opened app folder: a regular file of the
// site's account, read through no link, no FIFO, at most MaxEnvBytes.
func readEnv(appFD int, rel string, uid uint32, at string) ([]byte, error) {
	fd, err := unix.Openat2(appFD, rel, &unix.OpenHow{
		Flags:   unix.O_RDONLY | unix.O_NOFOLLOW | unix.O_NONBLOCK | unix.O_NOCTTY | unix.O_CLOEXEC,
		Resolve: resolveBeneath | unix.RESOLVE_NO_XDEV})
	switch {
	case errors.Is(err, unix.ELOOP):
		return nil, refused(CodeNotReal, at, "is a link, not the app's own file")
	case errors.Is(err, unix.ENOENT) || errors.Is(err, unix.ENOTDIR):
		return nil, refused(CodeNotReal, at, "is missing")
	case errors.Is(err, unix.EXDEV):
		return nil, refused(CodeNotReal, at, "lies on another file system")
	case err != nil:
		return nil, refused(CodeUnreadable, at, "could not be opened: %v", err)
	}
	defer unix.Close(fd)
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return nil, refused(CodeUnreadable, at, "could not be read: %v", err)
	}
	switch {
	case st.Mode&unix.S_IFMT != unix.S_IFREG:
		return nil, refused(CodeNotReal, at, "is not a regular file")
	case st.Uid != uid:
		return nil, refused(CodeNotOwned, at, "belongs to uid %d, not uid %d", st.Uid, uid)
	case st.Size > MaxEnvBytes:
		return nil, refused(CodeNotReal, at, "is larger than %d bytes", MaxEnvBytes)
	}
	buf := make([]byte, 0, st.Size+1)
	chunk := make([]byte, 32<<10)
	for {
		n, err := unix.Read(fd, chunk)
		if n > 0 {
			buf = append(buf, chunk[:n]...)
			if len(buf) > MaxEnvBytes {
				return nil, refused(CodeNotReal, at, "is larger than %d bytes", MaxEnvBytes)
			}
		}
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			return nil, refused(CodeUnreadable, at, "could not be read: %v", err)
		}
		if n == 0 {
			return buf, nil
		}
	}
}

// fdPath is what /proc says an opened folder is (for messages and tests).
func fdPath(fd int) string {
	target, err := readlink(fmt.Sprintf("/proc/self/fd/%d", fd))
	if err != nil {
		return "?"
	}
	return target
}

func readlink(p string) (string, error) {
	buf := make([]byte, 4096)
	n, err := unix.Readlink(p, buf)
	if err != nil {
		return "", err
	}
	return string(buf[:n]), nil
}
