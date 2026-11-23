package main

import (
	"encoding/json"
	"errors"
	"log"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/alicelik/celikpanel/internal/hostingpath"
	"github.com/alicelik/celikpanel/internal/sitesandbox"
)

// A changed limit reaches a running slice at once (S4 R2a,
// docs/SITE-SANDBOX.md section 8).
//
// Measured 21 Nov 2026 with plain systemctl: on systemd 255 (Ubuntu 24.04)
// daemon-reload applies a changed slice file to the running slice's cgroup
// at once. On systemd 257 (Debian 13) it does not (memory.max stayed 512M
// six seconds after a reload while systemctl show said 300M; daemon-reexec
// did not apply it either): the new values reach the cgroup only when a unit
// in the slice starts, or when `systemctl set-property --runtime` realizes
// the slice. So after writing slice files and reloading, the Agent compares
// what the kernel enforces (memory.max, memory.swap.max, pids.max, cpu.max)
// with the EFFECTIVE value systemd reports for the slice (systemctl show:
// the file and every drop-in, so an owner's drop-in keeps its number) and,
// only for a key that differs, runs `systemctl set-property --runtime` with
// that effective value. systemd writes it as a control drop-in
// (/run/systemd/system.control/<slice>.d/50-<Key>.conf), which outranks
// /etc; the Agent removes exactly the files that call made and reloads, so
// the slice file and the owner's drop-ins stay the only settings. Nothing is
// restarted, and no process is started in a customer's slice.
//
// A key whose control drop-in already exists (the owner's own runtime
// setting) is never touched. On systemd 255 the kernel already holds the
// new values after the reload, so nothing is pushed.
//
// The files the Agent is about to make are recorded first
// (siteSliceRuntimeJournalPath), and removed in the same step on every way
// out of it (a deferred cleanup after the record). Only an Agent stop
// between set-property and the removal leaves them (review B6): they carry
// the effective values the slice had at that moment, so nothing changes for
// the owner then, but until they are removed they outrank the owner's own
// drop-ins in /etc/systemd/system/<slice>.d for those keys: a drop-in the
// owner adds or changes for such a key meanwhile does not apply until the
// Agent's next start removes the file (cleanupSiteSliceRuntime, the first
// thing the Agent does after taking its instance lock, before anything
// reads limits) or the next reboot (/run is emptied). An owner's own
// `systemctl set-property --runtime` of the same key made in that window is
// removed with them.
//
// Değişen sınır çalışan dilime hemen ulaşır: systemd 257 dosyayı yeniden
// yüklemede uygulamadığı için yalnız farklı olan anahtar, systemd'nin
// bildirdiği etkin değerle anlık olarak uygulanır ve geçici dosya hemen
// silinir.

var (
	siteSliceControlDir         = "/run/systemd/system.control"
	siteSliceRuntimeJournalPath = filepath.Join(hostingpath.ServiceMutationStateRoot(), "sandbox", "site-slice-runtime.json")
)

type siteSliceRuntimeJournal struct {
	Files []string `json:"files"`
}

// siteSliceKernelKey is one limit: systemd's property, the kernel's file, and
// how both are compared.
type siteSliceKernelKey struct {
	property string // systemctl show property
	assign   string // set-property key (its control drop-in is 50-<assign>.conf)
	file     string // cgroup file
}

var siteSliceKernelKeys = []siteSliceKernelKey{
	{"MemoryMax", "MemoryMax", "memory.max"},
	{"MemorySwapMax", "MemorySwapMax", "memory.swap.max"},
	{"TasksMax", "TasksMax", "pids.max"},
	{"CPUQuotaPerSecUSec", "CPUQuota", "cpu.max"},
}

// kernelBytes reads memory.max-style text ("max": -1).
func kernelBytes(text string) int64 {
	text = strings.TrimSpace(text)
	if text == "max" {
		return -1
	}
	n, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return -2
	}
	return n
}

// systemdBytes reads a byte or count value systemctl shows ("infinity": -1).
func systemdBytes(text string) int64 {
	text = strings.TrimSpace(text)
	if text == "infinity" {
		return -1
	}
	n, err := strconv.ParseUint(text, 10, 64)
	if err != nil {
		return -2
	}
	if n >= math.MaxInt64 {
		return -1
	}
	return int64(n)
}

// kernelCPUPercent reads cpu.max ("max 100000": -1; "50000 100000": 50).
func kernelCPUPercent(text string) int64 {
	fields := strings.Fields(text)
	if len(fields) != 2 {
		return -2
	}
	if fields[0] == "max" {
		return -1
	}
	quota, err1 := strconv.ParseFloat(fields[0], 64)
	period, err2 := strconv.ParseFloat(fields[1], 64)
	if err1 != nil || err2 != nil || period <= 0 {
		return -2
	}
	return int64(math.Round(quota / period * 100))
}

// siteSliceAssignment is the set-property assignment for one key when the
// kernel differs from systemd's effective value ("" when they agree or
// either cannot be read).
func siteSliceAssignment(k siteSliceKernelKey, effective, kernel string) string {
	switch k.assign {
	case "CPUQuota":
		usec, limited := systemdTimespanUSec(effective)
		want := int64(-1)
		if limited {
			want = int64(math.Round(usec / 1e4))
		} else if strings.TrimSpace(effective) != "infinity" {
			return ""
		}
		have := kernelCPUPercent(kernel)
		if have == -2 || have == want {
			return ""
		}
		if want < 0 {
			return "CPUQuota="
		}
		return "CPUQuota=" + strconv.FormatInt(want, 10) + "%"
	default:
		want, have := systemdBytes(effective), kernelBytes(kernel)
		if want == -2 || have == -2 || want == have {
			return ""
		}
		if strings.HasPrefix(k.file, "memory.") && siteSliceSameBytes(want, have) {
			return "" // the kernel's page rounding of the same value
		}
		if want < 0 {
			return k.assign + "=infinity"
		}
		return k.assign + "=" + strconv.FormatInt(want, 10)
	}
}

// siteSliceSameBytes: a memory value systemd applies and the one the kernel
// shows are the same (-1: no limit). The kernel keeps memory.max in whole
// pages, rounding a value down by less than one page (64 KiB at most).
func siteSliceSameBytes(want, have int64) bool {
	if want < 0 || have < 0 {
		return want == have
	}
	return have <= want && want-have < 64<<10
}

// logSiteSlicePushFailures logs a push's failures where no record follows
// (a program's write, the move: each program started in the slice makes
// systemd apply the file, and the card reads the kernel's values).
func logSiteSlicePushFailures(failed map[int]string) {
	for id, why := range failed {
		log.Printf("site slice: %s keeps its earlier running limits until a program of the site starts: %s", sitesandbox.SliceName(id), why)
	}
}

func saveSiteSliceRuntimeJournal(files []string) error {
	raw, err := json.Marshal(siteSliceRuntimeJournal{Files: files})
	if err != nil {
		return err
	}
	return writePrivateFile(siteSliceRuntimeJournalPath, raw)
}

// pushSiteSlicesToKernel makes the running slices of these sites enforce
// their effective limits now (the rules above). The caller has written the
// slice files and reloaded systemd. It answers, per site, why its running
// slice does not hold them (S4 R2a third fix round, review N3): systemd
// could not be read, the record could not be written, set-property failed,
// or the kernel's files still differ after it. ApplySiteSlicesV1 turns these
// into failed outcomes, which the Panel records for its hourly round and
// its Problems now line.
func pushSiteSlicesToKernel(siteIDs []int) map[int]string {
	failed := map[int]string{}
	if systemdUserMode || len(siteIDs) == 0 {
		return failed
	}
	fail := func(name, why string) {
		if id, ok := sitesandbox.ParseSliceName(name); ok && failed[id] == "" {
			failed[id] = why
		}
	}
	var names []string
	for _, id := range siteIDs {
		name := sitesandbox.SliceName(id)
		if info, err := os.Stat(filepath.Join(siteSliceCgroupRoot, name)); err == nil && info.IsDir() {
			names = append(names, name) // a slice not running gets its limits when it starts
		}
	}
	if len(names) == 0 {
		return failed
	}
	args := []string{"show", "--property=Id,MemoryMax,MemorySwapMax,TasksMax,CPUQuotaPerSecUSec"}
	raw, err := appUnitSystemctl(systemctlArgs(append(args, names...)...)...)
	if err != nil {
		why := "systemd's running limits could not be read: " + strings.TrimSpace(err.Error()+" "+string(raw))
		for _, name := range names {
			fail(name, why)
		}
		return failed
	}
	effective := map[string]map[string]string{}
	for _, block := range strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n\n") {
		props := parseShow(block)
		if id := props["Id"]; id != "" {
			effective[id] = props
		}
	}
	plan := map[string][]string{}
	var files []string
	for _, name := range names {
		props := effective[name]
		if props == nil {
			fail(name, "systemd did not report the slice's running limits")
			continue
		}
		for _, k := range siteSliceKernelKeys {
			control := filepath.Join(siteSliceControlDir, name+".d", "50-"+k.assign+".conf")
			if _, err := os.Lstat(control); err == nil {
				continue // the owner's own runtime setting: never touched
			}
			assignment := siteSliceAssignment(k, props[k.property], readSmallFile(filepath.Join(siteSliceCgroupRoot, name, k.file)))
			if assignment == "" {
				continue
			}
			plan[name] = append(plan[name], assignment)
			files = append(files, control)
		}
	}
	if len(files) == 0 {
		return failed // systemd already applied them (systemd 255), or nothing changed
	}
	if err := saveSiteSliceRuntimeJournal(files); err != nil {
		for name := range plan {
			fail(name, "the running limits were not pushed: their record could not be written ("+err.Error()+")")
		}
		return failed
	}
	// The control drop-ins go again in this same step, on every way out.
	defer cleanupSiteSliceRuntime()
	for _, name := range names {
		if len(plan[name]) == 0 {
			continue
		}
		out, err := appUnitSystemctl(systemctlArgs(append([]string{"set-property", "--runtime", name}, plan[name]...)...)...)
		if err != nil {
			fail(name, "systemctl set-property: "+strings.TrimSpace(err.Error()+" "+string(out)))
		}
	}
	// What the kernel holds now is the answer, not what was asked.
	for name := range plan {
		for _, k := range siteSliceKernelKeys {
			if siteSliceAssignment(k, effective[name][k.property], readSmallFile(filepath.Join(siteSliceCgroupRoot, name, k.file))) != "" {
				fail(name, "the kernel still holds the earlier "+k.file)
			}
		}
	}
	return failed
}

// cleanupSiteSliceRuntime removes the control drop-ins the Agent recorded
// before set-property (and only those), reloads systemd when one was there,
// and forgets the record. At the Agent's start it removes what a stop left.
func cleanupSiteSliceRuntime() {
	raw, err := os.ReadFile(siteSliceRuntimeJournalPath)
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	var journal siteSliceRuntimeJournal
	if err == nil {
		err = json.Unmarshal(raw, &journal)
	}
	if err != nil {
		log.Printf("site slice: the record of pushed limits could not be read: %v", err)
		_ = os.Remove(siteSliceRuntimeJournalPath)
		return
	}
	removed := false
	prefix := filepath.Clean(siteSliceControlDir) + string(filepath.Separator)
	for _, file := range journal.Files {
		clean := filepath.Clean(file)
		// Only a site slice's control drop-in, never anything else.
		dir := filepath.Base(filepath.Dir(clean))
		if !strings.HasPrefix(clean, prefix) || !strings.HasSuffix(dir, ".slice.d") {
			continue
		}
		if _, ok := sitesandbox.ParseSliceName(strings.TrimSuffix(dir, ".d")); !ok {
			continue
		}
		if err := os.Remove(clean); err == nil {
			removed = true
		} else if !errors.Is(err, os.ErrNotExist) {
			log.Printf("site slice: %s could not be removed: %v (it holds the limits in force when it was made; until it is removed or the next reboot it outranks the owner's own drop-ins for that key)", clean, err)
		}
		_ = os.Remove(filepath.Dir(clean)) // only when empty
	}
	if removed {
		if err := siteSliceDaemonReload(); err != nil {
			log.Printf("site slice: %v", err)
		}
	}
	_ = os.Remove(siteSliceRuntimeJournalPath)
}
