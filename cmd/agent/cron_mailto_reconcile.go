package main

import (
	"bufio"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/hostingpath"
)

// Existing site crontabs at the moment passwordless mail is closed (D-046,
// alpha.125). New and changed site crontabs get MAILTO="" when the Panel
// writes them (cron_mailto.go); a crontab written before this release would
// otherwise start failing mail at every run with output. Once, right after
// authorized_submit_users is CelikPanel's value, every site account's
// crontab with at least one job and no MAILTO of its own gets the marked
// MAILTO="" line at the top. A crontab that sets MAILTO is the owner's: kept
// and named in the note. root and system accounts are never looked at. The
// before/after versions are recorded beside the Postfix change, and the step
// is not run again once recorded.
//
// Var olan site crontab'ları: parolasız posta kapandığı an, kendi MAILTO'su
// olmayan ve görev taşıyan her site crontab'ı bir kez MAILTO="" alır; sahibin
// MAILTO'su korunur ve bildirilir; root'a dokunulmaz; önce/sonra kaydedilir.

// passwordlessCronChange is one crontab the step changed.
type passwordlessCronChange struct {
	User   string `json:"user"`
	Before string `json:"before_version"`
	After  string `json:"after_version"`
}

// cronSiteAccounts lists the site accounts (uid >= 1000, home under the
// hosting root), sorted. Replaced by tests.
var cronSiteAccounts = func() ([]string, error) {
	file, err := os.Open("/etc/passwd")
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var names []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), ":")
		if len(fields) < 7 {
			continue
		}
		uid, err := strconv.Atoi(fields[2])
		if err != nil || uid < 1000 || fields[0] == "root" || !cronUsernameRe.MatchString(fields[0]) {
			continue
		}
		if hostingpath.IsSiteHome(path.Clean(fields[5])) {
			names = append(names, fields[0])
		}
	}
	sort.Strings(names)
	return names, scanner.Err()
}

// reconcileSiteCronMailto runs the step and returns what it changed, the
// owners' crontabs it kept, and the users whose crontab could not be read or
// written (left as they were).
func reconcileSiteCronMailto() (changed []passwordlessCronChange, kept, failed []string) {
	if err := requireCronInstalled(); err != nil {
		return nil, nil, nil
	}
	users, err := cronSiteAccounts()
	if err != nil {
		return nil, nil, []string{"the account list (" + boundedNativeDetail(err.Error()) + ")"}
	}
	cronMu.Lock()
	defer cronMu.Unlock()
	for _, username := range users {
		content, err := readCrontab(username)
		if err != nil {
			failed = append(failed, username)
			continue
		}
		// No job, or CelikPanel's own line already there (a retry).
		if len(parseCrontab(content)) == 0 || strings.Contains(content, cronMailtoComment) {
			continue
		}
		if cronSetsMailto(content) {
			kept = append(kept, username)
			continue
		}
		next := withSiteCronMailto(username, content)
		if err := setCrontab(username, next); err != nil {
			failed = append(failed, username)
			continue
		}
		changed = append(changed, passwordlessCronChange{User: username, Before: cronVersion(content), After: cronVersion(next)})
	}
	return changed, kept, failed
}

// passwordlessCronStep runs the step once, after the sendmail command is
// closed, and records it in the passwordless record.
func passwordlessCronStep(record *passwordlessRecord, now time.Time) string {
	if record == nil || record.SubmitAfter == "" || record.CronMailtoAt != "" {
		return ""
	}
	changed, kept, failed := reconcileSiteCronMailto()
	if len(failed) > 0 {
		// Nothing is recorded as done: the next start tries the rest again;
		// the crontabs already changed now carry MAILTO and are skipped.
		record.CronMailtoChanged = append(record.CronMailtoChanged, changed...)
		_ = writePasswordlessRecord(record)
		return "site crontabs: MAILTO=\"\" could not be added for " + strings.Join(failed, ", ") +
			" (their crontab could not be read or written; left as it was, tried again at the next panel start); " +
			"until then cron output of those sites fails to be mailed"
	}
	record.CronMailtoAt = now.UTC().Format(time.RFC3339)
	record.CronMailtoChanged = append(record.CronMailtoChanged, changed...)
	record.CronMailtoKept = kept
	if err := writePasswordlessRecord(record); err != nil {
		return "site crontabs were given MAILTO=\"\", but CelikPanel's record of it could not be written (" + boundedNativeDetail(err.Error()) + ")"
	}
	var notes []string
	if len(changed) > 0 {
		notes = append(notes, "site cron output is no longer mailed: MAILTO=\"\" added to "+strconv.Itoa(len(changed))+" site crontab(s)")
	}
	if len(kept) > 0 {
		notes = append(notes, "site crontabs that set their own MAILTO were kept as they are ("+strings.Join(kept, ", ")+
			"); their cron output can no longer be mailed, because the sendmail command refuses site accounts; the server owner sets MAILTO=\"\" there or keeps it on purpose")
	}
	return strings.Join(notes, "; ")
}
