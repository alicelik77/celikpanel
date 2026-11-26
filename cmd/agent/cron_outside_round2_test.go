package main

import (
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// S4 R1, second fix round (22 Nov 2026): crontab lines not made in
// CelikPanel, the second review's B8 and B9.

// B8: a change that takes away a line CelikPanel accounted for writes the
// record without it FIRST. At the moment the crontab is written (a power
// loss could come next) the record must account for no line the new crontab
// does not have. Fails on 193b87001 (the record was written after).
func TestADeleteWritesTheRecordWithoutTheLineBeforeTheCrontab(t *testing.T) {
	fake := installFakeCrontab(t, noCrontabYet())
	for _, command := range []string{"/usr/bin/true", "/usr/bin/backup"} {
		if err := addCronJobFor(cronTestUser, &transport.AddCronJobRequest{Schedule: "0 2 * * *", Command: command, Version: cronVersion(fake.content)}); err != nil {
			t.Fatal(err)
		}
	}
	removed := "0 2 * * * /usr/bin/backup"
	install := cronInstallCrontab
	stale := false
	cronInstallCrontab = func(user, content string) error {
		if record, found := readSiteCrontabRecord(cronTestUser); found && record.Known[cronLineDigest(removed)] > 0 {
			stale = true
		}
		return install(user, content)
	}
	t.Cleanup(func() { cronInstallCrontab = install })
	resp := listOutside(t)
	var id string
	for _, job := range resp.Jobs {
		if job.Command == "/usr/bin/backup" {
			id = job.ID
		}
	}
	if err := deleteCronJobFor(cronTestUser, &transport.DeleteCronJobRequest{ID: id, Version: resp.Version}); err != nil {
		t.Fatal(err)
	}
	if stale {
		t.Fatal("the record still accounted for the deleted line when the crontab was written")
	}
	if strings.Contains(fake.content, removed) {
		t.Fatal("the line was not deleted")
	}
	if listOutside(t).OutsideLines != 0 {
		t.Fatal("the remaining task is listed as not made in CelikPanel")
	}
}

// B9: the Agent, not only the screen, refuses Edit and Enable of a line not
// made in CelikPanel until the administrator keeps it; Delete stays. Fails on
// 193b87001 (the edit was accepted and made the line CelikPanel's).
func TestTheAgentRefusesToEditOrEnableALineNotMadeInCelikPanel(t *testing.T) {
	fake := installFakeCrontab(t, noCrontabYet())
	if err := addCronJobFor(cronTestUser, &transport.AddCronJobRequest{Schedule: "0 1 * * *", Command: "/usr/bin/true", Version: cronVersion("")}); err != nil {
		t.Fatal(err)
	}
	fake.content += "# DISABLED: 0 3 * * * /usr/local/bin/planted\n*/5 * * * * /usr/local/bin/also-planted\n"
	resp := listOutside(t)
	for _, job := range resp.Jobs {
		if !job.OutsidePanel {
			continue
		}
		before := fake.content
		err := updateCronJobFor(cronTestUser, &transport.UpdateCronJobRequest{ID: job.ID, Schedule: job.Schedule, Command: job.Command, Enabled: true, Version: resp.Version})
		if err == nil || err.Error() != transport.CronJobOutsidePanel {
			t.Fatalf("%s: err %v", job.Command, err)
		}
		if fake.content != before {
			t.Fatalf("%s: the crontab changed", job.Command)
		}
	}
	// Kept by the administrator, the line may be edited.
	var planted transport.CronJob
	for _, job := range resp.Jobs {
		if job.Command == "/usr/local/bin/also-planted" {
			planted = job
		}
	}
	if err := keepCronLineFor(cronTestUser, planted.ID, resp.Version); err != nil {
		t.Fatal(err)
	}
	resp = listOutside(t)
	if err := updateCronJobFor(cronTestUser, &transport.UpdateCronJobRequest{ID: planted.ID, Schedule: "*/10 * * * *", Command: planted.Command, Enabled: true, Version: resp.Version}); err != nil {
		t.Fatalf("a kept line could not be edited: %v", err)
	}
	// Delete stays for everyone.
	resp = listOutside(t)
	for _, job := range resp.Jobs {
		if job.OutsidePanel {
			if err := deleteCronJobFor(cronTestUser, &transport.DeleteCronJobRequest{ID: job.ID, Version: resp.Version}); err != nil {
				t.Fatalf("delete: %v", err)
			}
			break
		}
	}
}
