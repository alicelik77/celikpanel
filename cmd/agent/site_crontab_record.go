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
//     state, a digest of the exact bytes of every recorded line of it that
//     CelikPanel accounts for: the lines this write made, and the lines it
//     kept that were already accounted for. A recorded line is one cron acts
//     on (not blank, not a comment; crontab(5)) or a task CelikPanel paused
//     ("# DISABLED: <task>"), which a later Enable would make one. A line
//     that was there but not accounted for stays not accounted for:
//     rewriting a crontab never makes an unknown line CelikPanel's.
//   - A recorded line of the live crontab that the record does not account
//     for is "not made in CelikPanel": the site's Scheduled tasks list marks
//     it (delete only), and the side doors step counts it for the
//     administrator's Problems now. Two identical lines are counted one by
//     one: the first ones the record accounts for stay CelikPanel's.
//   - The administrator may keep such a line ("Keep", KeepCronLineV1): it is
//     added to the record and is no longer listed. A customer may only
//     delete it (a site's own code may have planted it).
//   - Existing installs: CelikPanel kept no record of a site's tasks before
//     this (the Scheduled tasks screen always read the crontab itself). The
//     first side doors step that runs this code takes each existing crontab
//     as it is and accounts for the lines CelikPanel's own writer can make:
//     MAILTO="" and a task (or a paused task) with a five-field schedule of
//     the form the panel accepts, byte for byte as the panel writes them. Any
//     other line (an @reboot or @daily line, an environment setting such as
//     SHELL= or PATH=, an owner's MAILTO, a schedule written with names or
//     steps the panel does not write, a line ending in a carriage return) is
//     listed. Every readable crontab gets a record then, an empty one too, so
//     a line planted later in it is listed. A crontab that cannot be read
//     does not hold the others: it gets no record, is named in Problems now,
//     and once readable its lines are listed until kept or deleted.
//
// Bir sitenin crontab'ı CelikPanel dışında değiştirilirse görülür: Agent her
// yazışında hesapladığı satırların tam baytlarının özetini saklar; kaydın
// açıklamadığı satır "CelikPanel'de yapılmadı" diye işaretlenir. Yönetici
// böyle bir satırı "Kalsın" ile tutabilir; müşteri yalnız silebilir. Hiçbir
// şey kendiliğinden engellenmez ya da silinmez.

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

// cronRecordedLine: a line cron acts on, or a task CelikPanel paused. The
// test looks past leading blanks (cron does); the line's bytes are kept as
// they are.
func cronRecordedLine(raw string) bool {
	trimmed := strings.TrimLeft(raw, " \t")
	if strings.TrimSpace(trimmed) == "" {
		return false
	}
	if strings.HasPrefix(trimmed, "#") {
		_, disabled, ok := cronJobText(raw)
		return ok && disabled
	}
	return true
}

// cronEffectiveLines are the recorded lines of a crontab, exact bytes.
func cronEffectiveLines(content string) []string {
	var lines []string
	for _, raw := range strings.Split(content, "\n") {
		if cronRecordedLine(raw) {
			lines = append(lines, raw)
		}
	}
	return lines
}

// cronLineDigest is the digest of a line's exact bytes: a trailing carriage
// return or blank is a different line.
func cronLineDigest(line string) string {
	sum := sha256.Sum256([]byte(line))
	return hex.EncodeToString(sum[:16])
}

// cronPanelShaped: a line CelikPanel's own writer makes, byte for byte.
func cronPanelShaped(line string) bool {
	if line == `MAILTO=""` {
		return true
	}
	if strings.ContainsAny(line, "\r\t\v\f") || line != strings.TrimSpace(line) {
		return false
	}
	text := strings.TrimPrefix(line, "# DISABLED: ")
	if strings.HasPrefix(text, "#") {
		return false
	}
	parts := parseCronLine(text)
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

// cronLineClass is one recorded line, in order, and whether CelikPanel
// accounts for it.
type cronLineClass struct {
	Line    string
	Outside bool
}

// cronClassifyLines walks the recorded lines in order: the record accounts
// for the first occurrences of each line it knows, the rest are outside.
func cronClassifyLines(username, content string) []cronLineClass {
	lines := cronEffectiveLines(content)
	if len(lines) == 0 {
		return nil
	}
	out := make([]cronLineClass, 0, len(lines))
	record, found := readSiteCrontabRecord(username)
	if !found {
		baseline := siteCrontabBaselineTaken()
		for _, line := range lines {
			// After the baseline, a crontab CelikPanel never wrote is listed
			// whole; before it, only what the panel's writer cannot make.
			out = append(out, cronLineClass{Line: line, Outside: baseline || !cronPanelShaped(line)})
		}
		return out
	}
	left := map[string]int{}
	for digest, count := range record.Known {
		left[digest] = count
	}
	for _, line := range lines {
		digest := cronLineDigest(line)
		if left[digest] > 0 {
			left[digest]--
			out = append(out, cronLineClass{Line: line})
			continue
		}
		out = append(out, cronLineClass{Line: line, Outside: true})
	}
	return out
}

// cronOutsideLines returns the lines of content that CelikPanel does not
// account for, in order.
func cronOutsideLines(username, content string) []string {
	var outside []string
	for _, class := range cronClassifyLines(username, content) {
		if class.Outside {
			outside = append(outside, class.Line)
		}
	}
	return outside
}

// siteCrontabRecordAfter is the record of a write: the lines of after that
// were accounted for in before, or that this write made (authored, exact
// lines as written).
func siteCrontabRecordAfter(username, before, after string, authored []string) siteCrontabRecord {
	known := map[string]int{}
	for _, class := range cronClassifyLines(username, before) {
		if !class.Outside {
			known[cronLineDigest(class.Line)]++
		}
	}
	for _, line := range authored {
		if cronRecordedLine(line) {
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

// setSiteCrontab writes a site's crontab, then its record.
//
// The order fails in the safe direction. A power loss (or a failed record
// write) between the two leaves the new crontab with the earlier record: a
// line this write made is then listed as not made in CelikPanel, and the
// administrator's Keep, or adding it again, resolves it. The other order
// would leave a record that accounts for a line the crontab does not have,
// and site code that then added exactly that text would be taken as
// CelikPanel's.
// Önce crontab, sonra kaydı yazılır; arada kesinti olursa yeni satır
// "CelikPanel'de yapılmadı" görünür, tersi asla olmaz.
func setSiteCrontab(username, before, content string, authored []string) error {
	if !validSiteCrontabRecordName(username) {
		return setCrontab(username, content)
	}
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	record := siteCrontabRecordAfter(username, before, content, authored)
	if err := setCrontab(username, content); err != nil {
		return err
	}
	if err := writeSiteCrontabRecord(username, record); err != nil {
		log.Printf("cron: the record of %s's crontab could not be written; its new lines will be listed as not made in CelikPanel: %v", username, err)
	}
	return nil
}

// cronSplitLines are the lines of a crontab text as written (for authored).
func cronSplitLines(text string) []string {
	return strings.Split(strings.TrimRight(text, "\n"), "\n")
}
