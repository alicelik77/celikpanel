package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"github.com/alicelik/celikpanel/internal/hostingpath"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Scheduled jobs a site's own code can make (S4 R1, 21 Nov 2026), the interim
// until site jobs become timers in the site's own slice:
//
//  1. `at` is closed to site accounts. Where the at package is installed and
//     no /etc/at.allow exists, the step writes one that lists exactly the
//     accounts that may use at today (every account of /etc/passwd that
//     /etc/at.deny does not name) minus CelikPanel's website accounts (every
//     account whose home is under the hosting root, and the database tools'
//     and webmail's own accounts). root may always use at (at's
//     check_permission returns at once for uid 0). With at.allow present at
//     reads at.deny no more, so every account keeps exactly what it had.
//     The file is given /etc/at.deny's owner and mode (Debian and Ubuntu:
//     root:daemon 0640; at reads both files with the daemon group). An
//     existing at.allow is the owner's and is left as it is; the site
//     accounts it names are listed. CelikPanel's own file is brought up to
//     date at each start while it is as CelikPanel wrote it, so an account
//     made later may use at as before; once changed it is the owner's.
//     Every account that shares a website account's id is left out too: an
//     extra FTP login is made with the site's uid (useradd -o), and at and
//     crontab decide by the name getpwuid() returns for the caller's uid,
//     which is the FIRST /etc/passwd entry with that uid (at 3.2 perm.c
//     check_permission; cron 3.0pl1 crontab.c main), so such a login could
//     stand for the site.
//     Undo: delete the file. The owner's undo stands (S4 R1 fix round): a
//     file CelikPanel wrote and the owner deleted is never written again by
//     itself; Problems now says at is open, and the administrator's explicit
//     "close it again" writes it anew.
//     CelikPanel's record of its file (at-allow.sha256 in the Agent's private
//     state) is written BEFORE the file (S4 R1, second fix round): first
//     "pending <new digest> <earlier digest or ->", then the file, then the
//     new digest alone. A crash in between reads at the next start as
//     CelikPanel's unfinished step: a file with either digest is CelikPanel's
//     (finished or brought up to date), a first write that never happened is
//     written, and only a file that is gone after it was there reads as the
//     owner's undo. Accounts that come from a directory
//     service (LDAP, SSSD, NIS) cannot be listed safely, so nothing is
//     written then. Pending at jobs of site accounts are listed (number,
//     account, time; never their commands, which are the site's own text) and
//     never removed.
//  2. Site crontabs with lines not made in CelikPanel are counted
//     (site_crontab_record.go).
//
// Bir sitenin kendi kodunun kurabileceği zamanlanmış işler: at, site
// hesaplarına kapatılır (at.allow, bugün at kullanabilen herkes eksi site
// hesapları); CelikPanel'de yapılmamış crontab satırları sayılır. Hiçbir
// şey kendiliğinden silinmez.

// Paths are variables so tests can point them at a temporary tree.
var (
	atAllowPath       = "/etc/at.allow"
	atDenyPath        = "/etc/at.deny"
	atCommandPaths    = []string{"/usr/bin/at", "/bin/at"}
	sideDoorsPasswd   = "/etc/passwd"
	sideDoorsNSSwitch = "/etc/nsswitch.conf"
	atAllowRecordPath = filepath.Join(hostingpath.ServiceMutationStateRoot(), "at-allow.sha256")
)

// atAllowMark is the first line of CelikPanel's at.allow.
const atAllowMark = "# Written by CelikPanel to keep its website accounts from using at."

const atAllowHeader = atAllowMark + `
# Only the accounts listed below may use at and batch; root always may.
# They are every account that could use at before (not named in
# /etc/at.deny), except CelikPanel's website accounts, so a website cannot
# plant jobs that run outside its limits.
# While this file is as CelikPanel wrote it, CelikPanel adds accounts made
# later at its next start. Once you change it, it is yours and CelikPanel
# leaves it as it is.
# To undo: delete this file. /etc/at.deny then applies again, as before, and
# CelikPanel does not write it again by itself: its Problems now list says at
# is open to website accounts, and closing it again there writes it anew.
`

// passwdEntry is one line of /etc/passwd.
type passwdEntry struct {
	Name string
	UID  int
	Home string
}

// readPasswd reads the local account list; directory reports a line that
// pulls accounts from NIS (+name, -name, +).
func readPasswd(file string) (entries []passwdEntry, directory bool, err error) {
	handle, err := os.Open(file)
	if err != nil {
		return nil, false, err
	}
	defer handle.Close()
	scanner := bufio.NewScanner(handle)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "+") || strings.HasPrefix(line, "-") {
			directory = true
			continue
		}
		fields := strings.Split(line, ":")
		if len(fields) < 7 || fields[0] == "" {
			continue
		}
		uid, convErr := strconv.Atoi(fields[2])
		if convErr != nil {
			continue
		}
		entries = append(entries, passwdEntry{Name: fields[0], UID: uid, Home: path.Clean(fields[5])})
	}
	return entries, directory, scanner.Err()
}

// nssDirectoryAccounts: the passwd line of nsswitch.conf names a source
// other than the local files (or systemd's own dynamic users).
func nssDirectoryAccounts(content string) bool {
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(raw)
		if hash := strings.Index(line, "#"); hash >= 0 {
			line = strings.TrimSpace(line[:hash])
		}
		name, sources, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(name) != "passwd" {
			continue
		}
		for _, source := range strings.Fields(sources) {
			if strings.HasPrefix(source, "[") {
				continue
			}
			switch source {
			case "files", "compat", "systemd":
			default:
				return true
			}
		}
	}
	return false
}

// atWebsiteAccount: an account CelikPanel runs website code as.
func atWebsiteAccount(entry passwdEntry) bool {
	if hostingpath.IsSiteHome(entry.Home) {
		return true
	}
	for _, tool := range []string{"webmail", "phpmyadmin", "phppgadmin"} {
		if entry.Name == phpToolAccount(tool) {
			return true
		}
	}
	return false
}

// siteAccountTenant: a site account's subscription and domain from its home.
var siteHomeIDs = regexp.MustCompile(`^/([0-9]{1,9})/sites/([0-9]{1,9})$`)

func siteAccountTenant(home string) (int, int, bool) {
	if !hostingpath.IsSiteHome(home) {
		return 0, 0, false
	}
	match := siteHomeIDs.FindStringSubmatch(strings.TrimPrefix(home, hostingpath.SubscriptionsRoot()))
	if match == nil {
		return 0, 0, false
	}
	sub, _ := strconv.Atoi(match[1])
	dom, _ := strconv.Atoi(match[2])
	return sub, dom, sub > 0 && dom > 0
}

func nameLines(content string) []string {
	var names []string
	for _, line := range strings.Split(content, "\n") {
		if line != "" && !strings.HasPrefix(line, "#") {
			names = append(names, line)
		}
	}
	return names
}

// atWebsiteUIDs are the ids of the website accounts (never root's): an
// account that shares one (an extra FTP login, useradd -o) is treated as
// the website account.
func atWebsiteUIDs(entries []passwdEntry) map[int]bool {
	uids := map[int]bool{}
	for _, entry := range entries {
		if atWebsiteAccount(entry) && entry.UID != 0 {
			uids[entry.UID] = true
		}
	}
	return uids
}

// renderAtAllow is the at.allow that keeps today's rule minus the website
// accounts: root, then every account at.deny does not name, in passwd order;
// an account sharing a website account's id is left out with it.
func renderAtAllow(entries []passwdEntry, deny string) string {
	denied := map[string]bool{}
	for _, name := range nameLines(deny) {
		denied[name] = true
	}
	websiteUIDs := atWebsiteUIDs(entries)
	seen := map[string]bool{"root": true}
	var b strings.Builder
	b.WriteString(atAllowHeader)
	b.WriteString("root\n")
	for _, entry := range entries {
		if seen[entry.Name] || denied[entry.Name] || atWebsiteAccount(entry) || websiteUIDs[entry.UID] || len(entry.Name) > 255 {
			continue
		}
		seen[entry.Name] = true
		b.WriteString(entry.Name + "\n")
	}
	return b.String()
}

type siteAtOps struct {
	installed func() bool
	readFile  func(string) ([]byte, error)
	// writeLike writes path atomically with the owner and mode of like.
	writeLike func(path, like string, data []byte) error
	atq       func(ctx context.Context) (string, error)
}

func atDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// atRecord is CelikPanel's record of its /etc/at.allow: the digest of the
// file it wrote (done), or a write it began (pendingNew, with pendingOld the
// digest it replaced, or "-" for a first write).
type atRecord struct {
	exists                       bool
	done, pendingNew, pendingOld string
	err                          error
}

func readAtRecord(ops siteAtOps) atRecord {
	data, err := ops.readFile(atAllowRecordPath)
	if errors.Is(err, fs.ErrNotExist) {
		return atRecord{}
	}
	if err != nil {
		return atRecord{err: err}
	}
	fields := strings.Fields(string(data))
	switch {
	case len(fields) == 1:
		return atRecord{exists: true, done: fields[0]}
	case len(fields) == 3 && fields[0] == "pending":
		return atRecord{exists: true, pendingNew: fields[1], pendingOld: fields[2]}
	}
	return atRecord{err: fmt.Errorf("%s is not CelikPanel's record", atAllowRecordPath)}
}

// ours: the file with this digest is the one CelikPanel wrote, or the one an
// unfinished write of its own was replacing.
func (r atRecord) ours(digest string) bool {
	return r.exists && (r.done == digest || r.pendingNew == digest || (r.pendingOld != "-" && r.pendingOld == digest))
}

// gone: CelikPanel's file was there and is not: the owner deleted it. A first
// write that never finished ("pending <new> -") is not.
func (r atRecord) gone() bool {
	return r.exists && (r.done != "" || (r.pendingOld != "" && r.pendingOld != "-"))
}

// lastDigest is the digest the record holds of a file that was there.
func (r atRecord) lastDigest() string {
	if r.done != "" {
		return r.done
	}
	if r.pendingOld != "-" {
		return r.pendingOld
	}
	return ""
}

// atPass says what one pass over at may do: reclose is the administrator's
// explicit "close it again" (a file the owner deleted is written anew; a cut
// or refused attempt leaves the owner's undo recorded); readOnly reads again
// without writing (the hourly round, B7), and a state only the step decides
// comes back as "".
type atPass struct {
	reclose  bool
	readOnly bool
}

// closeSiteAtWith decides and, where allowed, writes /etc/at.allow, and lists
// the site accounts' pending jobs.
func closeSiteAtWith(ctx context.Context, ops siteAtOps) *transport.SideDoorsAt {
	return closeSiteAtFor(ctx, ops, atPass{})
}

func closeSiteAtFor(ctx context.Context, ops siteAtOps, pass atPass) *transport.SideDoorsAt {
	if !ops.installed() {
		return &transport.SideDoorsAt{State: transport.SideDoorsAtNotInstalled}
	}
	at := &transport.SideDoorsAt{Path: atAllowPath}
	entries, nisLines, passwdErr := readPasswd(sideDoorsPasswd)
	sites := map[string]bool{}
	website := map[string]bool{}
	websiteUIDs := atWebsiteUIDs(entries)
	siteUIDs := map[int]bool{}
	for _, entry := range entries {
		if hostingpath.IsSiteHome(entry.Home) && entry.UID != 0 {
			siteUIDs[entry.UID] = true
		}
	}
	for _, entry := range entries {
		// An account sharing a website account's id stands for it (at and
		// crontab resolve the caller's uid to the first passwd name).
		if atWebsiteAccount(entry) || websiteUIDs[entry.UID] {
			website[entry.Name] = true
		}
		if hostingpath.IsSiteHome(entry.Home) || siteUIDs[entry.UID] {
			sites[entry.Name] = true
		}
	}
	at.Jobs = pendingSiteAtJobs(ctx, ops, sites)
	if passwdErr != nil {
		at.State, at.Detail = transport.SideDoorsAtFailed, "the account list could not be read: "+passwdErr.Error()
		return at
	}
	record := readAtRecord(ops)
	if record.err != nil {
		// Whether CelikPanel wrote the file, and whether the owner deleted
		// it, cannot be told: nothing is written.
		at.State, at.Detail = transport.SideDoorsAtFailed, "CelikPanel's record of /etc/at.allow could not be read, so nothing was written: "+record.err.Error()
		return at
	}
	current, err := ops.readFile(atAllowPath)
	switch {
	case err == nil:
		digest := atDigest(current)
		desired := ""
		ours := strings.HasPrefix(string(current), atAllowMark+"\n") && record.ours(digest)
		if ours {
			deny, denyErr := ops.readFile(atDenyPath)
			if denyErr == nil && !nisLines {
				desired = renderAtAllow(entries, string(deny))
			}
		}
		if desired == "" {
			at.State = transport.SideDoorsAtOwnerFile
			for _, name := range nameLines(string(current)) {
				if website[name] {
					at.SiteAccounts = append(at.SiteAccounts, name)
				}
			}
			sort.Strings(at.SiteAccounts)
			return at
		}
		if pass.readOnly {
			at.State = transport.SideDoorsAtCurrent
			return at
		}
		if desired == string(current) {
			at.State = transport.SideDoorsAtCurrent
			if record.done != digest {
				// CelikPanel's own write finished but its record did not.
				if err := writePrivateFileAtomic(atAllowRecordPath, []byte(digest+"\n"), ".at-allow-*"); err != nil {
					at.Detail = "CelikPanel's record of the file could not be written: " + err.Error()
				}
			}
			return at
		}
		return writeAtAllow(ops, at, desired, digest)
	case !errors.Is(err, fs.ErrNotExist):
		at.State, at.Detail = transport.SideDoorsAtFailed, err.Error()
		return at
	}
	// The owner's undo stands (D-022): CelikPanel's file was there and is
	// gone, so the owner deleted it. It is not written again by itself; the
	// administrator's explicit "close it again" writes it anew.
	if record.gone() && !pass.reclose {
		at.State = transport.SideDoorsAtOwnerRemoved
		return at
	}
	if pass.readOnly {
		at.State = ""
		return at
	}
	deny, err := ops.readFile(atDenyPath)
	if errors.Is(err, fs.ErrNotExist) {
		// Neither file: only root may use at (at.allow(5)).
		at.State = transport.SideDoorsAtOnlyRoot
		return at
	}
	if err != nil {
		at.State, at.Detail = transport.SideDoorsAtFailed, err.Error()
		return at
	}
	nss, nssErr := ops.readFile(sideDoorsNSSwitch)
	if nisLines || (nssErr == nil && nssDirectoryAccounts(string(nss))) {
		at.State = transport.SideDoorsAtDirectory
		at.Detail = "accounts come from a directory service (" + sideDoorsNSSwitch + " or + lines in " + sideDoorsPasswd + "), so who may use at today cannot be listed; nothing was written"
		return at
	}
	// A first write is recorded with "-"; the administrator's "close it
	// again" keeps the digest of the file the owner deleted, so a cut attempt
	// still reads as the owner's undo.
	earlier := "-"
	if last := record.lastDigest(); last != "" {
		earlier = last
	}
	return writeAtAllow(ops, at, renderAtAllow(entries, string(deny)), earlier)
}

// writeAtAllow writes CelikPanel's record of the write first, then the file,
// then the record of the finished write (B5/B10: a crash in between is
// CelikPanel's unfinished step, never the owner's file or the owner's undo).
// earlier is the digest of the file this write replaces, "-" for none.
func writeAtAllow(ops siteAtOps, at *transport.SideDoorsAt, content, earlier string) *transport.SideDoorsAt {
	digest := atDigest([]byte(content))
	previous, previousErr := ops.readFile(atAllowRecordPath)
	if err := writePrivateFileAtomic(atAllowRecordPath, []byte("pending "+digest+" "+earlier+"\n"), ".at-allow-*"); err != nil {
		at.State, at.Detail = transport.SideDoorsAtFailed, "CelikPanel's record of the file could not be written, so nothing was written: "+err.Error()
		return at
	}
	if err := ops.writeLike(atAllowPath, atDenyPath, []byte(content)); err != nil {
		// The record goes back to what it was; nothing else changed.
		if previousErr == nil {
			_ = writePrivateFileAtomic(atAllowRecordPath, previous, ".at-allow-*")
		} else {
			_ = os.Remove(atAllowRecordPath)
		}
		at.State, at.Detail = transport.SideDoorsAtFailed, err.Error()
		return at
	}
	at.State = transport.SideDoorsAtWritten
	if err := writePrivateFileAtomic(atAllowRecordPath, []byte(digest+"\n"), ".at-allow-*"); err != nil {
		at.Detail = "CelikPanel's record of the finished write could not be written; the next start finishes it: " + err.Error()
	}
	return at
}

// atqLine: "<id>\t<date> <queue> <user>" (atq(1), at 3.2.5).
var atqLine = regexp.MustCompile(`^([0-9]+)\s+(.+?)\s+(\S)\s+(\S+)$`)

// sideDoorsAtJobsShown bounds the jobs kept (number, account, time).
const sideDoorsAtJobsShown = 200

// pendingSiteAtJobs lists the pending at jobs of site accounts.
func pendingSiteAtJobs(ctx context.Context, ops siteAtOps, sites map[string]bool) []transport.SideDoorsAtJob {
	out, err := ops.atq(ctx)
	if err != nil {
		return nil
	}
	var jobs []transport.SideDoorsAtJob
	for _, line := range strings.Split(out, "\n") {
		match := atqLine.FindStringSubmatch(strings.TrimSpace(line))
		if match == nil || !sites[match[4]] {
			continue
		}
		// Number, account and time only: the job's commands are the site's
		// own text (it may hold secrets, and its first line is the site's
		// choice); the administrator reads them with "at -c <number>".
		if len(jobs) < sideDoorsAtJobsShown {
			jobs = append(jobs, transport.SideDoorsAtJob{ID: match[1], Account: match[4], When: match[2]})
		}
	}
	return jobs
}

func hostSiteAtOps() siteAtOps {
	return siteAtOps{
		installed: func() bool {
			for _, candidate := range atCommandPaths {
				if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
					return true
				}
			}
			return false
		},
		readFile:  os.ReadFile,
		writeLike: writeFileLike,
		atq: func(ctx context.Context) (string, error) {
			cmd := serviceMutationCommand(ctx, "atq")
			cmd.Env = append(os.Environ(), "LC_ALL=C", "LANGUAGE=C")
			out, err := cmd.Output()
			return string(out), err
		},
	}
}

// writeFileLike writes path atomically with like's owner and permission bits.
func writeFileLike(target, like string, data []byte) error {
	info, err := os.Stat(like)
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("the owner of %s could not be read", like)
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), ".celikpanel-"+filepath.Base(target)+"-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	_, err = tmp.Write(data)
	if err == nil {
		err = tmp.Chown(int(stat.Uid), int(stat.Gid))
	}
	if err == nil {
		err = tmp.Chmod(info.Mode().Perm())
	}
	if err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(name, target)
	}
	return err
}

// --- site crontabs ---

// siteCrontabAccounts lists the site accounts with their tenant identities.
// Replaced by tests.
var siteCrontabAccounts = func() ([]passwdEntry, error) {
	entries, _, err := readPasswd(sideDoorsPasswd)
	if err != nil {
		return nil, err
	}
	var sites []passwdEntry
	for _, entry := range entries {
		if _, _, ok := siteAccountTenant(entry.Home); ok && entry.UID >= 1000 && entry.Name != "root" && cronUsernameRe.MatchString(entry.Name) {
			sites = append(sites, entry)
		}
	}
	sort.Slice(sites, func(i, j int) bool { return sites[i].Name < sites[j].Name })
	return sites, nil
}

// checkSiteCrontabs counts each site crontab's lines not made in CelikPanel.
// The first run of this code takes the existing crontabs as the baseline
// (site_crontab_record.go): every readable crontab gets a record, an empty
// one too, so a line planted later is listed. A crontab that cannot be read
// does not hold the baseline open for the others: it is named, with why, for
// Problems now, and gets no record (once readable its lines are listed until
// the administrator keeps or someone deletes them).
func checkSiteCrontabs() []transport.SideDoorsSiteCrontab {
	return checkSiteCrontabsFor(false)
}

// checkSiteCrontabsFor: readOnly counts without taking the baseline (the
// hourly round and after a Manage-window move write nothing).
func checkSiteCrontabsFor(readOnly bool) []transport.SideDoorsSiteCrontab {
	if requireCronInstalled() != nil {
		return nil
	}
	accounts, err := siteCrontabAccounts()
	if err != nil {
		return nil
	}
	cronMu.Lock()
	defer cronMu.Unlock()
	baseline := !readOnly && !siteCrontabBaselineTaken()
	recorded := true
	var rows []transport.SideDoorsSiteCrontab
	for _, account := range accounts {
		sub, dom, _ := siteAccountTenant(account.Home)
		content, err := readCrontab(account.Name)
		if err != nil {
			rows = append(rows, transport.SideDoorsSiteCrontab{Account: account.Name, SubscriptionID: sub, DomainID: dom, Unreadable: boundedNativeDetail(err.Error())})
			continue
		}
		if baseline {
			if _, found := readSiteCrontabRecord(account.Name); !found {
				record := siteCrontabRecordAfter(account.Name, content, content, nil)
				record.Baseline = true
				if writeSiteCrontabRecord(account.Name, record) != nil {
					recorded = false
				}
			}
		}
		if outside := cronOutsideLines(account.Name, content); len(outside) > 0 {
			rows = append(rows, transport.SideDoorsSiteCrontab{Account: account.Name, SubscriptionID: sub, DomainID: dom, Outside: len(outside)})
		}
	}
	if baseline && recorded {
		if err := os.MkdirAll(siteCrontabRecordDir, 0o700); err == nil {
			_ = writePrivateFileAtomic(filepath.Join(siteCrontabRecordDir, siteCrontabBaselineName), []byte("taken\n"), ".baseline-*")
		}
	}
	return rows
}
