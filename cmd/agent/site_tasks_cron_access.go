package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/hostingpath"
)

// Who may still put a crontab in place for a moved site's account, and the
// account's crontab read straight from cron's spool (S4 R2b fix round, 22
// Nov 2026; docs/SITE-SANDBOX.md sections 9.3-9.5).
//
// Measured (Stage 1 F1, Debian 13 and Ubuntu 24.04) and read in Debian's
// cron 3.0pl1 source: an account cron.allow leaves out keeps the crontab it
// already has, and `crontab -u <account>` is refused even for root; root
// still reads and removes the spool file. crontab checks the caller once,
// at its start (allowed() in main, before it reads the new crontab from its
// input), so a crontab process started before cron.allow closed can still
// install a file afterwards: the move ends every such process of the
// account's uid once cron.allow leaves the account out, and only then reads
// and removes the spool file.
//
// Taşınmış bir sitenin hesabı için crontab'ı kimin geri koyabileceği ve
// crontab'ın cron'un kendi dosyasından okunması.

// siteCronSpoolDirs are where cron keeps an account's crontab: Debian's and
// Ubuntu's cron (/var/spool/cron/crontabs/<account>) and cronie on Arch
// (/var/spool/cron/<account>). A variable for tests.
var siteCronSpoolDirs = []string{"/var/spool/cron/crontabs", "/var/spool/cron"}

// spoolFile is one existing spool file of an account.
type spoolFile struct {
	Path    string
	Raw     string
	ModTime time.Time
}

// accountSpoolFiles reads the account's spool files (regular files of at
// most 1 MiB; a link or anything else is reported as an error, never
// followed).
func accountSpoolFiles(account string) ([]spoolFile, error) {
	if !validSiteCrontabRecordName(account) {
		return nil, fmt.Errorf("%q is not an account name", account)
	}
	var out []spoolFile
	for _, dir := range siteCronSpoolDirs {
		path := filepath.Join(dir, account)
		info, err := os.Lstat(path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return out, err
		}
		if info.IsDir() {
			continue // /var/spool/cron/crontabs itself on Debian when account == "crontabs"
		}
		if !info.Mode().IsRegular() || info.Size() > 1<<20 {
			return out, fmt.Errorf("%s is not a regular file of at most 1 MiB", path)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return out, err
		}
		out = append(out, spoolFile{Path: path, Raw: string(raw), ModTime: info.ModTime()})
	}
	return out, nil
}

// crontabListView is what `crontab -l` prints of a spool file on Debian's
// cron 3.0pl1 (crontab.c list_cmd): up to NHEADER_LINES (3) leading lines
// that start with "#" are skipped (crontab writes its three header lines
// there), the rest is printed as it is.
func crontabListView(raw string) string {
	rest := raw
	for i := 0; i < 3 && strings.HasPrefix(rest, "#"); i++ {
		newline := strings.IndexByte(rest, '\n')
		if newline < 0 {
			return ""
		}
		rest = rest[newline+1:]
	}
	return rest
}

// spoolHolds: the spool file holds this crontab (as `crontab -l` printed
// it on Debian and Ubuntu, or byte for byte as cronie keeps it).
func spoolHolds(f spoolFile, crontab string) bool {
	return f.Raw == crontab || crontabListView(f.Raw) == crontab
}

// spoolCrontabLines reads an account's crontab straight from cron's spool
// (root may read it; `crontab -u` refuses an account that /etc/cron.allow
// leaves out) and returns the lines cron acts on, from every supported
// spool path.
func spoolCrontabLines(account string) []string {
	files, _ := accountSpoolFiles(account)
	var lines []string
	for _, f := range files {
		for _, line := range cronEffectiveLines(crontabListView(f.Raw)) {
			if !strings.HasPrefix(strings.TrimLeft(line, " \t"), "#") {
				lines = append(lines, line)
			}
		}
	}
	return lines
}

// --- Who may use crontab ------------------------------------------------------

// namesOfUID are the account and every passwd name sharing its uid (crontab
// decides by the first passwd name of the caller's uid, so a login that
// shares the site's uid speaks for the site).
func namesOfUID(entries []passwdEntry, account string) (map[string]bool, int) {
	names := map[string]bool{account: true}
	uid := -1
	for _, e := range entries {
		if e.Name == account {
			uid = e.UID
		}
	}
	if uid > 0 {
		for _, e := range entries {
			if e.UID == uid {
				names[e.Name] = true
			}
		}
	}
	return names, uid
}

// cronMoveRefusal says, before anything changes, why moving the account's
// tasks would leave it able to install its own crontab ("" when cron's
// access files will keep it out once the move is done). A moved site whose
// account may still install jobs is not a state CelikPanel creates:
//
//   - the server owner's own /etc/cron.allow names the account (or a login
//     sharing its uid): the owner removes it, or deletes the file so
//     CelikPanel writes its own;
//   - CelikPanel's file was deleted by the owner (it is not written again by
//     itself): the administrator closes it again in Problems now;
//   - accounts come from a directory service, so the file cannot be written.
//
// /etc/cron.deny naming the account already keeps it out.
func cronMoveRefusal(account string) string {
	ops := hostSiteCronOps()
	entries, nisLines, err := readPasswd(sideDoorsPasswd)
	if err != nil {
		return "the account list could not be read (" + err.Error() + "), so it is not known whether " + account + " could install its own cron jobs after the move; nothing was changed. Try again; the server owner checks " + sideDoorsPasswd + " if it repeats"
	}
	names, _ := namesOfUID(entries, account)
	current, err := ops.readFile(cronAllowPath)
	switch {
	case err == nil:
		if cronAllowOurs(ops, current) && !nisLines {
			return ""
		}
		for _, name := range nameLines(string(current)) {
			if names[name] {
				return "the server owner's " + cronAllowPath + " lets " + name + " use cron, so the site's own code could install jobs that run outside its limits after the move; nothing was changed. The server owner removes " + name + " from that file (or deletes the file, so CelikPanel writes its own), then moves the site again"
			}
		}
		return ""
	case !errors.Is(err, fs.ErrNotExist):
		return cronAllowPath + " could not be read (" + err.Error() + "); nothing was changed. The server owner checks the file, then moves the site again"
	}
	if deny, err := ops.readFile(cronDenyPath); err == nil {
		for _, name := range nameLines(string(deny)) {
			if name == account {
				return ""
			}
		}
	}
	if _, recordErr := ops.readFile(cronAllowRecordPath); recordErr == nil {
		return "CelikPanel's " + cronAllowPath + " was deleted on this server and is not written again by itself, so the site's own code could install jobs that run outside its limits after the move; nothing was changed. The administrator presses \"Close it again\" in Problems now, then moves the site again"
	}
	nss, nssErr := ops.readFile(sideDoorsNSSwitch)
	if nisLines || (nssErr == nil && nssDirectoryAccounts(string(nss))) {
		return "accounts come from a directory service, so CelikPanel cannot write " + cronAllowPath + " and the site's own code could install jobs that run outside its limits after the move; nothing was changed. The server owner keeps the account out with their own " + cronAllowPath + " (or " + cronDenyPath + "), then moves the site again"
	}
	return ""
}

// cronRefusesAccount: cron's access files keep the account (and every login
// sharing its uid) out now.
func cronRefusesAccount(account string) bool {
	ops := hostSiteCronOps()
	entries, _, err := readPasswd(sideDoorsPasswd)
	if err != nil {
		return false
	}
	names, _ := namesOfUID(entries, account)
	if current, err := ops.readFile(cronAllowPath); err == nil {
		for _, name := range nameLines(string(current)) {
			if names[name] {
				return false
			}
		}
		return true
	} else if !errors.Is(err, fs.ErrNotExist) {
		return false
	}
	deny, err := ops.readFile(cronDenyPath)
	if err != nil {
		return false
	}
	for _, name := range nameLines(string(deny)) {
		if name == account {
			return true
		}
	}
	return false
}

// --- Since when an account has been kept out ----------------------------------

// siteCronClosedPath records, per moved account, since when CelikPanel's own
// /etc/cron.allow has kept it out with no crontab left (the move checked
// that). A crontab found later was then made by root (the server owner): the
// account could not have installed it. Kept only while CelikPanel's file is
// as it wrote it and leaves the account out; otherwise the record goes, and
// a crontab found is one the site's own code can have made.
var siteCronClosedPath = filepath.Join(hostingpath.ServiceMutationStateRoot(), "sandbox", "site-cron-closed.json")

var siteCronClosedMu sync.Mutex

func readSiteCronClosed() map[string]string {
	out := map[string]string{}
	if raw, err := os.ReadFile(siteCronClosedPath); err == nil {
		_ = json.Unmarshal(raw, &out)
	}
	return out
}

func changeSiteCronClosed(change func(map[string]string) bool) {
	siteCronClosedMu.Lock()
	defer siteCronClosedMu.Unlock()
	all := readSiteCronClosed()
	if !change(all) {
		return
	}
	raw, err := json.Marshal(all)
	if err == nil {
		err = writePrivateFile(siteCronClosedPath, raw)
	}
	if err != nil {
		log.Printf("site tasks: record since when cron keeps moved accounts out: %v", err)
	}
}

// noteCronClosed records that the account is kept out from now on.
func noteCronClosed(account string, at time.Time) {
	changeSiteCronClosed(func(all map[string]string) bool {
		all[account] = at.UTC().Format(time.RFC3339Nano)
		return true
	})
}

// forgetCronClosed drops the record of these accounts (nil: of every account).
func forgetCronClosed(accounts map[string]bool) {
	changeSiteCronClosed(func(all map[string]string) bool {
		changed := false
		for account := range all {
			if accounts == nil || accounts[account] {
				delete(all, account)
				changed = true
			}
		}
		return changed
	})
}

// cronKeptOutByUs: CelikPanel's own, unchanged cron.allow leaves the account
// out.
func cronKeptOutByUs(account string) bool {
	ops := hostSiteCronOps()
	current, err := ops.readFile(cronAllowPath)
	return err == nil && cronAllowOurs(ops, current) && cronRefusesAccount(account)
}

// reappearedBySite: a crontab found for a moved account can have been made
// by the site's own code (true), or only by root (false): the latter only
// while CelikPanel's own file has kept the account out since a time the move
// recorded, and the crontab was written after it.
func reappearedBySite(account string, files []spoolFile) bool {
	since, ok := readSiteCronClosed()[account]
	if !ok || !cronKeptOutByUs(account) {
		return true
	}
	at, err := time.Parse(time.RFC3339Nano, since)
	if err != nil {
		return true
	}
	for _, f := range files {
		if !f.ModTime.After(at) {
			return true
		}
	}
	return false
}

// pruneCronClosed drops the records of accounts CelikPanel's file no longer
// keeps out (the owner changed or deleted the file, or let one in).
func pruneCronClosed() {
	for account := range readSiteCronClosed() {
		if !cronKeptOutByUs(account) {
			forgetCronClosed(map[string]bool{account: true})
		}
	}
}

// --- crontab processes started before cron.allow closed ------------------------

// siteTaskEndCrontabs ends every crontab process running as the uid (a test
// seam). It runs after cron.allow leaves the account out: such a process
// passed crontab's access check before, and could still install a crontab.
var siteTaskEndCrontabs = endCrontabProcesses

func endCrontabProcesses(uid int) (int, error) {
	if uid < 1 {
		return 0, nil
	}
	program, err := cronLookPath("crontab")
	if err != nil {
		return 0, nil
	}
	if resolved, err := filepath.EvalSymlinks(program); err == nil {
		program = resolved
	}
	entries, err := os.ReadDir(procRoot)
	if err != nil {
		return 0, err
	}
	ended := 0
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid < 2 {
			continue
		}
		status, err := os.ReadFile(filepath.Join(procRoot, e.Name(), "status"))
		if err != nil || procRealUID(string(status)) != uid {
			continue
		}
		exe, err := os.Readlink(filepath.Join(procRoot, e.Name(), "exe"))
		if err != nil || strings.TrimSuffix(exe, " (deleted)") != program {
			continue
		}
		if p, err := os.FindProcess(pid); err == nil && p.Kill() == nil {
			ended++
		}
	}
	return ended, nil
}

// procRealUID reads the real uid from /proc/<pid>/status ("Uid:\t<real>\t...").
func procRealUID(status string) int {
	for _, line := range strings.Split(status, "\n") {
		if rest, ok := strings.CutPrefix(line, "Uid:"); ok {
			fields := strings.Fields(rest)
			if len(fields) > 0 {
				if uid, err := strconv.Atoi(fields[0]); err == nil {
					return uid
				}
			}
		}
	}
	return -1
}
