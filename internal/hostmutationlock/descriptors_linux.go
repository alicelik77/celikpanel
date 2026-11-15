//go:build linux

package hostmutationlock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// descriptorWalkBudget bounds the descriptor walk. A walk that has not finished
// within it found nobody only among the processes it reached, so it answers
// ErrUnknown. Measured on a 16-thread WSL guest (alpha.136): 4,031 processes
// with 96,908 descriptors took 0.41 s (median of 21), about 4 µs a descriptor.
var descriptorWalkBudget = 5 * time.Second

// capSysPtrace is CAP_SYS_PTRACE: without it, another user's descriptor table
// (and, on a /proc mounted with hidepid, the process itself) is not readable.
const capSysPtrace = 19

// descriptorFlockHolders confirms who holds a flock on the open lock file
// (file is its fstat; the kernel's lock lines name it major:minor and its
// inode), from every process's open descriptors. It reads only procfs and
// takes no lock. A process holds the flock when one of its
// descriptors refers to the locked open file description; its fdinfo then lists
// the same "lock:" line as /proc/locks, for every process sharing that
// description (a descriptor inherited across fork or exec, or passed on). Shared
// and exclusive flocks both count: either refuses an exclusive writer.
//
// Unlike /proc/locks, the walk cannot skip a process that holds the lock for the
// whole probe: /proc lists processes, and /proc/<pid>/fd descriptors, in
// ascending number from where the previous chunk ended, so processes or
// descriptors appearing or disappearing elsewhere do not move a stable one.
//
// A holder found is returned even when the walk was incomplete. When nobody is
// found and some process could not be read (permission, a security module, no
// CAP_SYS_PTRACE, or the budget ran out), the error wraps ErrUnknown. A process
// or descriptor that disappears during the walk is not holding the lock.
//
// Her sürecin tanımlayıcıları okunur; bulunan tutucu kesindir, okunamayan süreç
// varsa ve kimse bulunmadıysa cevap "belirlenemedi"dir.
func descriptorFlockHolders(file unix.Stat_t, major, minor uint32) ([]int, error) {
	deadline := time.Now().Add(descriptorWalkBudget)
	var incomplete error
	if !effectiveCapability(capSysPtrace) {
		incomplete = errors.New("this process cannot read every process's descriptors (it lacks CAP_SYS_PTRACE)")
	}
	dir, err := os.Open(procRoot)
	if err != nil {
		return nil, fmt.Errorf("%w: list processes: %v", ErrUnknown, err)
	}
	names, err := dir.Readdirnames(-1)
	dir.Close()
	if err != nil {
		return nil, fmt.Errorf("%w: list processes: %v", ErrUnknown, err)
	}
	var pids []int
	for _, name := range names {
		pid, err := strconv.Atoi(name)
		if err != nil || pid <= 0 {
			continue
		}
		if time.Now().After(deadline) {
			if incomplete == nil {
				incomplete = fmt.Errorf("the descriptor walk did not finish within %s", descriptorWalkBudget)
			}
			break
		}
		held, err := processHoldsFlock(name, file, major, minor)
		if err != nil {
			if incomplete == nil {
				incomplete = err
			}
			continue
		}
		if held {
			pids = append(pids, pid)
		}
	}
	if len(pids) == 0 && incomplete != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnknown, incomplete)
	}
	return pids, nil
}

// processHoldsFlock reports whether one of the process's descriptors holds the
// flock. Each descriptor is identified by one statx of its /proc link with
// AT_STATX_DONT_SYNC, so a network or FUSE filesystem answers from its cache
// and a stalled server cannot stall the probe; nothing is opened through a
// descriptor. Only a descriptor of the lock file itself has its fdinfo read.
func processHoldsFlock(pid string, file unix.Stat_t, major, minor uint32) (bool, error) {
	fdDir := filepath.Join(procRoot, pid, "fd")
	dir, err := os.Open(fdDir)
	if err != nil {
		if vanished(err) {
			return false, nil
		}
		return false, fmt.Errorf("read the descriptors of process %s: %w", pid, err)
	}
	defer dir.Close()
	fds, err := dir.Readdirnames(-1)
	if err != nil {
		if vanished(err) {
			return false, nil
		}
		return false, fmt.Errorf("read the descriptors of process %s: %w", pid, err)
	}
	for _, fd := range fds {
		var target unix.Statx_t
		if err := unix.Statx(int(dir.Fd()), fd, unix.AT_STATX_DONT_SYNC, unix.STATX_INO, &target); err != nil {
			if vanished(err) {
				continue
			}
			return false, fmt.Errorf("read descriptor %s of process %s: %w", fd, pid, err)
		}
		if target.Ino != file.Ino || unix.Mkdev(target.Dev_major, target.Dev_minor) != file.Dev {
			continue
		}
		info, err := os.ReadFile(filepath.Join(procRoot, pid, "fdinfo", fd))
		if err != nil {
			if vanished(err) {
				continue
			}
			return false, fmt.Errorf("read descriptor %s of process %s: %w", fd, pid, err)
		}
		if fdinfoHoldsFlock(info, major, minor, file.Ino) {
			return true, nil
		}
	}
	return false, nil
}

// fdinfoHoldsFlock finds a granted flock on major:minor:ino among the fdinfo
// "lock:" lines ("lock:\t1: FLOCK  ADVISORY  WRITE 1234 00:1a:5678 0 EOF").
func fdinfoHoldsFlock(raw []byte, major, minor uint32, ino uint64) bool {
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 9 || fields[0] != "lock:" || fields[2] != "FLOCK" {
			continue
		}
		id := strings.Split(fields[6], ":")
		if len(id) != 3 {
			continue
		}
		maj, errMaj := strconv.ParseUint(id[0], 16, 32)
		min, errMin := strconv.ParseUint(id[1], 16, 32)
		num, errIno := strconv.ParseUint(id[2], 10, 64)
		if errMaj == nil && errMin == nil && errIno == nil && uint32(maj) == major && uint32(min) == minor && num == ino {
			return true
		}
	}
	return false
}

// vanished is a process or descriptor that ended while it was being read.
func vanished(err error) bool {
	return errors.Is(err, unix.ENOENT) || errors.Is(err, unix.ESRCH)
}

// effectiveCapability reads this process's effective capability set.
func effectiveCapability(capability uint) bool {
	raw, err := os.ReadFile(filepath.Join(procRoot, "self", "status"))
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if value, ok := strings.CutPrefix(line, "CapEff:"); ok {
			set, err := strconv.ParseUint(strings.TrimSpace(value), 16, 64)
			return err == nil && set&(1<<capability) != 0
		}
	}
	return false
}
