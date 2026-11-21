package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/hostingpath"
)

// A site's crontab changed outside CelikPanel is seen (S4 R1, 21 Nov 2026).
//
// Until site jobs become timers in the site's own slice (a later release), a
// site's own code can run `crontab` as its account and add lines that run
// outside any limit, and the server owner can edit a crontab by hand as root
// (D-022: the owner's change is kept, never fought). Neither is blocked here;
// both are made visible:
//
//   - Whenever the Agent writes a site's crontab it records, in its private
//     state, a digest of every line of it that makes cron do something (cron
//     ignores blank lines and lines that start with `#`, crontab(5)) and that
//     CelikPanel accounts for: the lines this write made, and the lines it
//     kept that were already accounted for. A line that was there but not
//     accounted for stays not accounted for: rewriting a crontab never makes
//     an unknown line CelikPanel's.
//   - A line of the live crontab that the record does not account for is
//     "not made in CelikPanel": the site's Scheduled tasks list marks it, and
//     the side doors step counts it for the administrator's Problems now.
//   - Existing installs: CelikPanel kept no record of a site's tasks before
//     this (the Scheduled tasks screen always read the crontab itself). The
//     first side doors step that runs this code takes each existing crontab
//     as it is and accounts for the lines CelikPanel's own writer can make:
//     MAILTO="" and a task with a five-field schedule of the form the panel
//     accepts. Any other line (an @reboot or @daily line, an environment
//     setting such as SHELL= or PATH=, an owner's MAILTO, a schedule written
//     with names or steps the panel does not write) is listed. After that
//     first step, a crontab of a site account CelikPanel never wrote is
//     listed whole.
//
// Bir sitenin crontab'ı CelikPanel dışında değiştirilirse görülür: Agent her
// yazışında hesapladığı satırların özetini saklar; kaydın açıklamadığı satır
// "CelikPanel'de yapılmadı" diye işaretlenir. Hiçbir şey kendiliğinden
// engellenmez ya da silinmez.

// Paths are variables so tests can point them at a temporary tree.
var (
	siteCrontabRecordDir = filepath.Join(hostingpath.ServiceMutationStateRoot(), "site-crontabs")
)

const siteCrontabBaselineName = "baseline-v1"

type siteCrontabRecord struct {
	Version int `json:"version"`
	// Digest is cronVersion of the crontab the record was taken from.
	Digest string `json:"digest"`
	// Known: each accounted line's digest and how many times it stands.
	Known map[string]int `json:"known"`
	At    string         `json:"at"`
	// Baseline: taken from an existing crontab by the first step, not written.
	Baseline bool `json:"baseline,omitempty"`
}

// cronEffectiveLines are the lines cron acts on: not blank, not a comment.
func cronEffectiveLines(content string) []string {
	var lines []string
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

func cronLineDigest(line string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(line)))
	return hex.EncodeToString(sum[:16])
}

// cronPanelShaped: a line CelikPanel's own writer can make.
func cronPanelShaped(line string) bool {
	if line == `MAILTO=""` {
		return true
	}
	parts := parseCronLine(line)
	return parts != nil && isValidCronSchedule(parts[0])
}

func validSiteCrontabRecordName(username string) bool {
	return username != "root" && cronUsernameRe.MatchString(username)
}

func siteCrontabRecordPath(username string) string {
	return filepath.Join(siteCrontabRecordDir, username+".json")
}

func readSiteCrontabRecord(username string) (siteCrontabRecord, bool) {
	if !validSiteCrontabRecordName(username) {
		return siteCrontabRecord{}, false
	}
	data, err := os.ReadFile(siteCrontabRecordPath(username))
	if err != nil || len(data) > 1<<20 {
		return siteCrontabRecord{}, false
	}
	var record siteCrontabRecord
	if json.Unmarshal(data, &record) != nil || record.Version != 1 {
		return siteCrontabRecord{}, false
	}
	if record.Known == nil {
		record.Known = map[string]int{}
	}
	return record, true
}

func writeSiteCrontabRecord(username string, record siteCrontabRecord) error {
	if !validSiteCrontabRecordName(username) {
		return errors.New("not a site account name")
	}
	if err := os.MkdirAll(siteCrontabRecordDir, 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return writePrivateFileAtomic(siteCrontabRecordPath(username), data, ".site-crontab-*")
}

// removeSiteCrontabRecord forgets a deleted site account's record.
func removeSiteCrontabRecord(username string) {
	if !validSiteCrontabRecordName(username) {
		return
	}
	if err := os.Remove(siteCrontabRecordPath(username)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		log.Printf("cron: the record of %s's crontab could not be removed: %v", username, err)
	}
}

func siteCrontabBaselineTaken() bool {
	_, err := os.Stat(filepath.Join(siteCrontabRecordDir, siteCrontabBaselineName))
	return err == nil
}

// cronOutsideLines returns the lines of content that CelikPanel does not
// account for, in order (a line standing twice where the record knows it
// once is listed once).
func cronOutsideLines(username, content string) []string {
	lines := cronEffectiveLines(content)
	if len(lines) == 0 {
		return nil
	}
	record, found := readSiteCrontabRecord(username)
	if !found {
		if siteCrontabBaselineTaken() {
			// A crontab of an account CelikPanel never wrote.
			return lines
		}
		var outside []string
		for _, line := range lines {
			if !cronPanelShaped(line) {
				outside = append(outside, line)
			}
		}
		return outside
	}
	left := map[string]int{}
	for digest, count := range record.Known {
		left[digest] = count
	}
	var outside []string
	for _, line := range lines {
		digest := cronLineDigest(line)
		if left[digest] > 0 {
			left[digest]--
			continue
		}
		outside = append(outside, line)
	}
	return outside
}

// siteCrontabRecordAfter is the record of a write: the lines of after that
// were accounted for in before, or that this write made (authored).
func siteCrontabRecordAfter(username, before, after string, authored []string) siteCrontabRecord {
	known := map[string]int{}
	for _, line := range cronEffectiveLines(before) {
		known[cronLineDigest(line)]++
	}
	for _, line := range cronOutsideLines(username, before) {
		digest := cronLineDigest(line)
		if known[digest]--; known[digest] <= 0 {
			delete(known, digest)
		}
	}
	for _, line := range authored {
		if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") {
			known[cronLineDigest(line)]++
		}
	}
	record := siteCrontabRecord{Version: 1, Digest: cronVersion(after), Known: map[string]int{}, At: time.Now().UTC().Format(time.RFC3339)}
	for _, line := range cronEffectiveLines(after) {
		digest := cronLineDigest(line)
		if known[digest] > 0 {
			known[digest]--
			record.Known[digest]++
		}
	}
	return record
}

// setSiteCrontab writes a site's crontab and its record. The record is
// written first, so a crontab CelikPanel wrote is never seen without it; when
// the crontab write fails the earlier record comes back. A record that cannot
// be written does not stop the write: the new lines are then listed as not
// made in CelikPanel, which is said, not hidden.
func setSiteCrontab(username, before, content string, authored []string) error {
	if !validSiteCrontabRecordName(username) {
		return setCrontab(username, content)
	}
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	previous, hadPrevious := readSiteCrontabRecord(username)
	recorded := true
	if err := writeSiteCrontabRecord(username, siteCrontabRecordAfter(username, before, content, authored)); err != nil {
		recorded = false
		log.Printf("cron: the record of %s's crontab could not be written; its new lines will be listed as not made in CelikPanel: %v", username, err)
	}
	if err := setCrontab(username, content); err != nil {
		if recorded {
			if hadPrevious {
				_ = writeSiteCrontabRecord(username, previous)
			} else {
				removeSiteCrontabRecord(username)
			}
		}
		return err
	}
	return nil
}

// cronSplitLines are the lines of a crontab text as written (for authored).
func cronSplitLines(text string) []string {
	return strings.Split(strings.TrimRight(text, "\n"), "\n")
}
