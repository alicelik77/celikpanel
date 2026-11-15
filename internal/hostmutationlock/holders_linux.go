//go:build linux

package hostmutationlock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// Holder is a process the kernel lists as holding a flock on a lock file.
// Command is its short process name and Unit its systemd unit, when readable.
// Neither carries arguments or environment, so no secret is exposed.
// Kilidi tutan süreç: kısa adı ve systemd birimi; argüman veya ortam yok.
type Holder struct {
	PID     int
	Command string
	Unit    string
}

func (h Holder) String() string {
	var parts []string
	for _, part := range []string{h.Command, h.Unit} {
		if part != "" {
			parts = append(parts, part)
		}
	}
	if len(parts) == 0 {
		return "process " + strconv.Itoa(h.PID)
	}
	return "process " + strconv.Itoa(h.PID) + " (" + strings.Join(parts, ", ") + ")"
}

// DescribeHolders is the one-line form used in journals and busy answers; ""
// when nobody holds the lock.
func DescribeHolders(holders []Holder) string {
	names := make([]string, 0, len(holders))
	for _, holder := range holders {
		names = append(names, holder.String())
	}
	return strings.Join(names, "; ")
}

// Holders names who holds the lock right now, read from the kernel's lock
// table and, when that names nobody, from every process's descriptors, without
// taking the lock. It is diagnostic only. An empty answer without an error
// means nobody held the lock during the read: for a lock a writer just found
// busy, the holder released it in between. When the holder cannot be
// established (the descriptor walk was incomplete), the error wraps ErrUnknown.
// A missing file in a trusted parent has no holder.
func Holders(path string, owner Owner) ([]Holder, error) {
	holders, observed, err := observe(path, owner)
	if err == nil && !observed {
		err = errors.New("the kernel lock table is unavailable")
	}
	return holders, err
}

// probeObservedHook runs after the lock table was read and before the file
// identity is checked again (tests only).
var probeObservedHook func()

// procRoot is the procfs mount read for the lock table (tests only change it).
var procRoot = "/proc"

// observe reports the flock holders of the trusted lock file. observed is false
// when the kernel's lock table could not be read. Nothing is locked either way.
//
// A holder the lock table names is authoritative. The table is not a snapshot,
// though: /proc/locks is produced one read() chunk at a time and resumes by
// line number, so a line can be skipped when other locks are released between
// two chunks (alpha.135: ProbeIdle answered idle in 1094 of 5000 probes while
// a child held the lock and other processes took and dropped flocks). When the
// table names nobody, the descriptor walk (descriptorFlockHolders) confirms
// it; an incomplete walk that finds nobody is ErrUnknown, never idle (ProbeIdle
// then decides with the momentary flock).
//
// Kilit tablosunun adlandırdığı tutucu kesindir; tablo kimseyi
// göstermezse her sürecin tanımlayıcıları okunur. Tamamlanamayan okuma
// "belirlenemedi" demektir, asla "boşta" değil.
func observe(path string, owner Owner) (holders []Holder, observed bool, err error) {
	loc, err := openLocation(path, owner)
	if err != nil {
		return nil, false, err
	}
	defer unix.Close(loc.parent)
	fd, err := loc.openFile()
	if err != nil {
		if errors.Is(err, unix.ENOENT) {
			return nil, true, loc.verifyDirectory()
		}
		return nil, false, fmt.Errorf("open existing host mutation lock: %w", err)
	}
	defer unix.Close(fd)
	before, err := loc.verifyFile(fd, nil)
	if err != nil {
		return nil, false, err
	}
	pids, major, minor, readErr := flockHolderPIDs(fd, before)
	var walkErr error
	if readErr == nil && len(pids) == 0 {
		pids, walkErr = descriptorFlockHolders(before, major, minor)
	}
	if probeObservedHook != nil {
		probeObservedHook()
	}
	if _, err = loc.verifyFile(fd, &before); err != nil {
		return nil, false, err
	}
	if readErr != nil {
		return nil, false, nil
	}
	if len(pids) == 0 && walkErr != nil {
		return nil, true, walkErr
	}
	for _, pid := range pids {
		holders = append(holders, describeHolder(pid))
	}
	return holders, true, nil
}

// flockHolderPIDs lists the processes holding a flock (shared or exclusive;
// either refuses an exclusive writer) on the open file, and the device the
// kernel names it by. Waiters ("->") are not holders. The kernel names the file
// by its superblock device, which differs from stat's device on btrfs
// subvolumes, so the device comes from the mount.
func flockHolderPIDs(fd int, st unix.Stat_t) ([]int, uint32, uint32, error) {
	major, minor, err := lockTableDevice(fd, st)
	if err != nil {
		return nil, 0, 0, err
	}
	raw, err := os.ReadFile(filepath.Join(procRoot, "locks"))
	if err != nil {
		return nil, 0, 0, err
	}
	var pids []int
	seen := map[int]bool{}
	for _, line := range strings.Split(string(raw), "\n") {
		// "1: FLOCK  ADVISORY  WRITE 1234 00:1a:5678 0 EOF"
		fields := strings.Fields(line)
		if len(fields) < 8 || fields[1] != "FLOCK" {
			continue
		}
		id := strings.Split(fields[5], ":")
		if len(id) != 3 {
			continue
		}
		maj, errMaj := strconv.ParseUint(id[0], 16, 32)
		min, errMin := strconv.ParseUint(id[1], 16, 32)
		ino, errIno := strconv.ParseUint(id[2], 10, 64)
		pid, errPID := strconv.Atoi(fields[4])
		if errMaj != nil || errMin != nil || errIno != nil || errPID != nil {
			continue
		}
		if uint32(maj) == major && uint32(min) == minor && ino == st.Ino && !seen[pid] {
			seen[pid] = true
			pids = append(pids, pid)
		}
	}
	return pids, major, minor, nil
}

// lockTableDevice is the superblock device of the open file: its mount's
// major:minor (mountinfo), or stat's device on kernels without mnt_id.
func lockTableDevice(fd int, st unix.Stat_t) (uint32, uint32, error) {
	info, err := os.ReadFile(filepath.Join(procRoot, "self", "fdinfo", strconv.Itoa(fd)))
	if err != nil {
		return 0, 0, err
	}
	mountID := ""
	for _, line := range strings.Split(string(info), "\n") {
		if fields := strings.Fields(line); len(fields) == 2 && fields[0] == "mnt_id:" {
			mountID = fields[1]
		}
	}
	if mountID == "" {
		return unix.Major(st.Dev), unix.Minor(st.Dev), nil
	}
	mounts, err := os.ReadFile(filepath.Join(procRoot, "self", "mountinfo"))
	if err != nil {
		return 0, 0, err
	}
	for _, line := range strings.Split(string(mounts), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 || fields[0] != mountID {
			continue
		}
		dev := strings.Split(fields[2], ":")
		if len(dev) != 2 {
			break
		}
		maj, errMaj := strconv.ParseUint(dev[0], 10, 32)
		min, errMin := strconv.ParseUint(dev[1], 10, 32)
		if errMaj != nil || errMin != nil {
			break
		}
		return uint32(maj), uint32(min), nil
	}
	return 0, 0, errors.New("the lock file's mount is not listed")
}

// describeHolder reads only the short process name and the systemd unit.
func describeHolder(pid int) Holder {
	holder := Holder{PID: pid}
	base := filepath.Join(procRoot, strconv.Itoa(pid))
	if raw, err := os.ReadFile(filepath.Join(base, "comm")); err == nil {
		holder.Command = printable(strings.TrimSpace(string(raw)), 32)
	}
	raw, err := os.ReadFile(filepath.Join(base, "cgroup"))
	if err != nil {
		return holder
	}
	for _, line := range strings.Split(string(raw), "\n") {
		parts := strings.SplitN(line, ":", 3)
		if len(parts) != 3 || (parts[0] != "0" && !strings.Contains(parts[1], "name=systemd")) {
			continue
		}
		for _, element := range strings.Split(parts[2], "/") {
			if strings.HasSuffix(element, ".service") || strings.HasSuffix(element, ".scope") {
				holder.Unit = printable(element, 128)
			}
		}
		if holder.Unit != "" {
			break
		}
	}
	return holder
}

func printable(value string, limit int) string {
	value = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, value)
	if len(value) > limit {
		value = value[:limit]
	}
	return value
}
