package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Machine detail (alpha.127, monitoring stage 2), read by the sampler that
// already runs (metrics.go) with no new process:
//
//   - Network throughput from /proc/net/dev (every interface but loopback and
//     the virtual ones a container or bridge adds, which would count the same
//     bytes twice) and disk throughput from /proc/diskstats (whole disks
//     only; partitions, device-mapper and RAID devices repeat their disks'
//     bytes). Both are the average of the 30 seconds a sample covers, in
//     bytes per second.
//   - The five busiest processes, captured once when the 30-second CPU
//     average or memory use reaches the alert level (90 %), at most once per
//     ten minutes for each, and shown beside that moment on the chart. Two
//     readings of /proc/<pid>/stat one second apart give each process's
//     share of the machine. Only the kernel's short process name is kept,
//     never the command line, so no argument (a password on a command line)
//     is ever stored or shown.
//
// Makine ayrıntısı: zaten çalışan örnekleyici okur, yeni süreç yoktur. Ağ ve
// disk hızı 30 saniyenin ortalamasıdır (bayt/saniye). İşlemci ortalaması ya da
// bellek %90'a gelince en yoğun beş işlem bir kez (her biri için en çok on
// dakikada bir) yakalanır; yalnız kısa işlem adı saklanır, komut satırı asla.

const (
	snapshotLevel    = problemSustainedLevel
	snapshotEvery    = 10 * time.Minute
	snapshotTop      = 5
	snapshotRetained = metricsHourRetention
	// clockTicks is USER_HZ, 100 on every Linux the panel supports.
	clockTicks = 100.0
)

// Readers, replaced by tests.
var (
	machineProcRoot = "/proc"
	machineSysBlock = "/sys/block"
)

// ioCounters are the cumulative byte counters of one reading.
type ioCounters struct {
	netRx, netTx       uint64
	diskRead, diskWrit uint64
	haveNet, haveDisk  bool
}

func readIOCounters() ioCounters {
	var c ioCounters
	if data, err := os.ReadFile(filepath.Join(machineProcRoot, "net", "dev")); err == nil {
		c.netRx, c.netTx, c.haveNet = parseNetDev(string(data))
	}
	if data, err := os.ReadFile(filepath.Join(machineProcRoot, "diskstats")); err == nil {
		c.diskRead, c.diskWrit, c.haveDisk = parseDiskStats(string(data), wholeDisk)
	}
	return c
}

// virtualInterface: loopback and what containers, bridges and libvirt add on
// top of a real interface.
func virtualInterface(name string) bool {
	if name == "lo" {
		return true
	}
	for _, prefix := range []string{"veth", "docker", "br-", "virbr", "cni", "flannel", "vnet"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func parseNetDev(text string) (rx, tx uint64, ok bool) {
	for _, line := range strings.Split(text, "\n") {
		name, rest, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		name = strings.TrimSpace(name)
		if name == "" || strings.Contains(name, "|") || virtualInterface(name) {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) < 9 {
			continue
		}
		r, errR := strconv.ParseUint(fields[0], 10, 64)
		t, errT := strconv.ParseUint(fields[8], 10, 64)
		if errR != nil || errT != nil {
			continue
		}
		rx, tx, ok = rx+r, tx+t, true
	}
	return rx, tx, ok
}

// wholeDisk: a block device the kernel lists in /sys/block (so not a
// partition) that is not a loop, RAM, optical, device-mapper or RAID device.
func wholeDisk(name string) bool {
	for _, prefix := range []string{"loop", "ram", "zram", "sr", "fd", "dm-", "md", "nbd"} {
		if strings.HasPrefix(name, prefix) {
			return false
		}
	}
	_, err := os.Stat(filepath.Join(machineSysBlock, name))
	return err == nil
}

func parseDiskStats(text string, include func(string) bool) (read, written uint64, ok bool) {
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 10 || !include(fields[2]) {
			continue
		}
		sectorsRead, errR := strconv.ParseUint(fields[5], 10, 64)
		sectorsWritten, errW := strconv.ParseUint(fields[9], 10, 64)
		if errR != nil || errW != nil {
			continue
		}
		read, written, ok = read+sectorsRead*512, written+sectorsWritten*512, true
	}
	return read, written, ok
}

// ioRates turns two readings dt apart into bytes per second. A counter that
// went back (a reset, an interface that left) gives no value, never a
// negative or a made-up one.
type ioRates struct {
	netRx, netTx, diskRead, diskWrite *float64
}

func rateOf(prev, cur uint64, seconds float64) *float64 {
	if cur < prev || seconds <= 0 {
		return nil
	}
	value := float64(int64(float64(cur-prev)/seconds + 0.5))
	return &value
}

func computeIORates(prev, cur ioCounters, seconds float64) ioRates {
	var r ioRates
	if prev.haveNet && cur.haveNet {
		r.netRx, r.netTx = rateOf(prev.netRx, cur.netRx, seconds), rateOf(prev.netTx, cur.netTx, seconds)
	}
	if prev.haveDisk && cur.haveDisk {
		r.diskRead, r.diskWrite = rateOf(prev.diskRead, cur.diskRead, seconds), rateOf(prev.diskWrit, cur.diskWrit, seconds)
	}
	return r
}

// --- Process snapshots --------------------------------------------------------

type procStat struct {
	pid   int
	name  string
	ticks uint64
	rss   uint64
}

// parseProcStat reads one /proc/<pid>/stat line. The name sits in
// parentheses and may itself hold spaces or parentheses, so the fields are
// read after the last ')'.
func parseProcStat(text string, pageSize uint64) (procStat, bool) {
	open := strings.IndexByte(text, '(')
	closing := strings.LastIndexByte(text, ')')
	if open <= 0 || closing < open {
		return procStat{}, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(text[:open]))
	if err != nil || pid <= 0 {
		return procStat{}, false
	}
	fields := strings.Fields(text[closing+1:])
	// Field 14 (utime) is fields[11], 15 (stime) fields[12], 24 (rss) fields[21].
	if len(fields) < 22 {
		return procStat{}, false
	}
	utime, errU := strconv.ParseUint(fields[11], 10, 64)
	stime, errS := strconv.ParseUint(fields[12], 10, 64)
	rss, errR := strconv.ParseInt(fields[21], 10, 64)
	if errU != nil || errS != nil || errR != nil {
		return procStat{}, false
	}
	if rss < 0 {
		rss = 0
	}
	return procStat{pid: pid, name: sanitizeProcName(text[open+1 : closing]), ticks: utime + stime, rss: uint64(rss) * pageSize}, true
}

// sanitizeProcName keeps the short name printable and bounded.
func sanitizeProcName(name string) string {
	var b strings.Builder
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			continue
		}
		b.WriteRune(r)
		if b.Len() >= 32 {
			break
		}
	}
	return b.String()
}

// readProcesses reads every process the panel can see. ok is false when
// /proc hides other users' processes (hidepid): a list of the panel's own
// processes would name the wrong cause.
func readProcesses() (map[int]procStat, bool) {
	if _, err := os.ReadFile(filepath.Join(machineProcRoot, "1", "stat")); err != nil {
		return nil, false
	}
	entries, err := os.ReadDir(machineProcRoot)
	if err != nil {
		return nil, false
	}
	pageSize := uint64(os.Getpagesize())
	processes := make(map[int]procStat, len(entries))
	for _, entry := range entries {
		if _, err := strconv.Atoi(entry.Name()); err != nil {
			continue
		}
		data, err := os.ReadFile(filepath.Join(machineProcRoot, entry.Name(), "stat"))
		if err != nil {
			continue
		}
		if stat, ok := parseProcStat(string(data), pageSize); ok {
			processes[stat.pid] = stat
		}
	}
	return processes, len(processes) > 0
}

// snapshotProcess is one stored line: CPU is the share of the whole machine
// (all vCPU) over the measured second, so it reads like the CPU card.
type snapshotProcess struct {
	PID  int     `json:"pid"`
	Name string  `json:"name"`
	User string  `json:"user,omitempty"`
	CPU  float64 `json:"cpu"`
	RSS  uint64  `json:"rss"`
}

var processOwner = func(pid int) string {
	data, err := os.ReadFile(filepath.Join(machineProcRoot, strconv.Itoa(pid), "status"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if rest, ok := strings.CutPrefix(line, "Uid:"); ok {
			fields := strings.Fields(rest)
			if len(fields) == 0 {
				return ""
			}
			if u, err := user.LookupId(fields[0]); err == nil {
				return u.Username
			}
			return fields[0]
		}
	}
	return ""
}

// topProcesses ranks by CPU over the two readings (reason cpu) or by memory
// (reason memory) and keeps the first five.
func topProcesses(before, after map[int]procStat, seconds float64, vcpus int, reason string) []snapshotProcess {
	if vcpus < 1 {
		vcpus = 1
	}
	list := make([]snapshotProcess, 0, len(after))
	for pid, cur := range after {
		cpu := 0.0
		if prev, ok := before[pid]; ok && cur.ticks >= prev.ticks && seconds > 0 {
			cpu = round1(float64(cur.ticks-prev.ticks) / clockTicks / seconds / float64(vcpus) * 100)
		}
		list = append(list, snapshotProcess{PID: pid, Name: cur.name, CPU: cpu, RSS: cur.rss})
	}
	sort.Slice(list, func(i, j int) bool {
		a, b := list[i], list[j]
		if reason == "memory" {
			if a.RSS != b.RSS {
				return a.RSS > b.RSS
			}
		} else if a.CPU != b.CPU {
			return a.CPU > b.CPU
		}
		return a.PID < b.PID
	})
	if len(list) > snapshotTop {
		list = list[:snapshotTop]
	}
	for i := range list {
		list[i].User = processOwner(list[i].PID)
	}
	return list
}

// processSnapshotter takes the first reading when a sample crosses the level
// and the second at the next one-second tick.
type processSnapshotter struct {
	mu       sync.Mutex
	last     map[string]time.Time
	pending  string
	value    float64
	at       time.Time
	before   map[int]procStat
	readAt   time.Time
	disabled bool
}

// consider is called after each sample with its CPU average and memory use.
func (s *processSnapshotter) consider(now time.Time, cpu, memPercent float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pending != "" || s.disabled {
		return
	}
	if s.last == nil {
		s.last = map[string]time.Time{}
	}
	reason, value := "", 0.0
	switch {
	case cpu >= snapshotLevel && now.Sub(s.last["cpu"]) >= snapshotEvery:
		reason, value = "cpu", cpu
	case memPercent >= snapshotLevel && now.Sub(s.last["memory"]) >= snapshotEvery:
		reason, value = "memory", memPercent
	default:
		return
	}
	before, ok := readProcesses()
	if !ok {
		// Other users' processes are hidden from the panel; say nothing
		// rather than a wrong cause. Logged once.
		s.disabled = true
		log.Printf("monitoring: the process list is not readable by the panel (/proc hides other users' processes); spike snapshots are off")
		return
	}
	s.pending, s.value, s.at, s.before, s.readAt = reason, value, now, before, time.Now()
	s.last[reason] = now
}

// finish takes the second reading and returns the snapshot to store.
func (s *processSnapshotter) finish(vcpus int) (reason string, value float64, at time.Time, processes []snapshotProcess, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pending == "" {
		return "", 0, time.Time{}, nil, false
	}
	after, readable := readProcesses()
	reason, value, at = s.pending, s.value, s.at
	seconds := time.Since(s.readAt).Seconds()
	s.pending, s.before = "", nil
	if !readable {
		return "", 0, time.Time{}, nil, false
	}
	return reason, value, at, topProcesses(s.beforeOr(after), after, seconds, vcpus, reason), true
}

func (s *processSnapshotter) beforeOr(fallback map[int]procStat) map[int]procStat {
	if s.before != nil {
		return s.before
	}
	return fallback
}

func insertProcessSnapshot(ctx context.Context, db *sql.DB, at time.Time, reason string, value float64, processes []snapshotProcess) error {
	raw, err := json.Marshal(processes)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `INSERT INTO metrics_process_snapshots (ts, reason, value, processes) VALUES (?, ?, ?, ?)`,
		metricsTime(at), reason, round1(value), string(raw))
	return err
}

type processSnapshotView struct {
	TS        string            `json:"ts"`
	Reason    string            `json:"reason"`
	Value     float64           `json:"value"`
	Processes []snapshotProcess `json:"processes"`
}

// readProcessSnapshots returns the snapshots of a range, newest first, at
// most limit.
func readProcessSnapshots(ctx context.Context, db *sql.DB, from time.Time, limit int) ([]processSnapshotView, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT ts, reason, value, processes FROM metrics_process_snapshots
		WHERE ts >= ? ORDER BY ts DESC, id DESC LIMIT ?`, metricsTime(from), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	views := []processSnapshotView{}
	for rows.Next() {
		var view processSnapshotView
		var raw string
		if err := rows.Scan(&view.TS, &view.Reason, &view.Value, &raw); err != nil {
			return nil, err
		}
		if json.Unmarshal([]byte(raw), &view.Processes) != nil {
			continue
		}
		views = append(views, view)
	}
	return views, rows.Err()
}
