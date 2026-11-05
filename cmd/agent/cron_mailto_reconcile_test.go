package main

import (
	"errors"
	"strings"
	"testing"
	"time"
)

type fakeSiteCrontabs struct {
	content  map[string]string
	broken   map[string]bool
	installs map[string]string
}

func installFakeSiteCrontabs(t *testing.T, fake *fakeSiteCrontabs, users ...string) {
	t.Helper()
	oldList, oldInstall, oldLook, oldAccess, oldAccounts := cronListCrontab, cronInstallCrontab, cronLookPath, cronAccessFile, cronSiteAccounts
	t.Cleanup(func() {
		cronListCrontab, cronInstallCrontab, cronLookPath, cronAccessFile, cronSiteAccounts = oldList, oldInstall, oldLook, oldAccess, oldAccounts
	})
	fake.installs = map[string]string{}
	cronAccessFile = func(string) ([]byte, error) { return nil, errors.New("no such file") }
	cronLookPath = func(string) (string, error) { return "/usr/bin/crontab", nil }
	cronSiteAccounts = func() ([]string, error) { return users, nil }
	cronListCrontab = func(username string) ([]byte, string, int, error) {
		if fake.broken[username] {
			return nil, "crontab: permission denied", 2, errors.New("exit status 2")
		}
		content, ok := fake.content[username]
		if !ok {
			return nil, "no crontab for " + username + "\n", 1, errors.New("exit status 1")
		}
		return []byte(content), "", 0, nil
	}
	cronInstallCrontab = func(username, content string) error {
		fake.installs[username] = content
		fake.content[username] = content
		return nil
	}
}

func TestExistingSiteCrontabsGetMailtoOnceOwnersKept(t *testing.T) {
	withPasswordlessRecord(t)
	job := "*/5 * * * * /usr/bin/php /var/www/site/cron.php\n"
	fake := &fakeSiteCrontabs{
		content: map[string]string{
			"shop_example":  job,
			"blog_example":  "MAILTO=owner@blog.example\n" + job,
			"empty_example": "# nothing scheduled\n",
			"stuck_example": job,
		},
		broken: map[string]bool{"stuck_example": true},
	}
	installFakeSiteCrontabs(t, fake, "blog_example", "empty_example", "none_example", "shop_example", "stuck_example")
	record := &passwordlessRecord{SubmitAfter: passwordlessSubmitUsers}
	now := time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC)

	note := passwordlessCronStep(record, now)
	if !strings.Contains(note, "stuck_example") || record.CronMailtoAt != "" {
		t.Fatalf("an unreadable crontab must keep the step open: %q %+v", note, record)
	}
	if fake.installs["shop_example"] != cronMailtoComment+"\nMAILTO=\"\"\n"+job || len(fake.installs) != 1 {
		t.Fatalf("installs %q", fake.installs)
	}

	// Next start: the stuck crontab is readable again; shop is not touched twice.
	fake.broken = nil
	note = passwordlessCronStep(record, now.Add(time.Hour))
	if record.CronMailtoAt == "" || len(record.CronMailtoChanged) != 2 || len(fake.installs) != 2 ||
		len(record.CronMailtoKept) != 1 || record.CronMailtoKept[0] != "blog_example" ||
		!strings.Contains(note, "blog_example") || !strings.Contains(note, "1 site crontab") {
		t.Fatalf("note %q record %+v installs %q", note, record, fake.installs)
	}
	if fake.content["blog_example"] != "MAILTO=owner@blog.example\n"+job || fake.content["empty_example"] != "# nothing scheduled\n" {
		t.Fatalf("owner or empty crontab changed: %q", fake.content)
	}
	if record.CronMailtoChanged[0].Before == record.CronMailtoChanged[0].After {
		t.Fatalf("before/after not recorded: %+v", record.CronMailtoChanged)
	}
	stored, err := readPasswordlessRecord()
	if err != nil || stored == nil || stored.CronMailtoAt == "" || len(stored.CronMailtoChanged) != 2 {
		t.Fatalf("stored %+v %v", stored, err)
	}

	// Once: later starts do nothing, even for a crontab that appears later.
	fake.content["late_example"] = job
	installs := len(fake.installs)
	if note := passwordlessCronStep(record, now.Add(2*time.Hour)); note != "" || len(fake.installs) != installs {
		t.Fatalf("ran again: %q", note)
	}
}

func TestSiteCronStepWaitsForTheSendmailClose(t *testing.T) {
	withPasswordlessRecord(t)
	fake := &fakeSiteCrontabs{content: map[string]string{"shop_example": "0 3 * * * /usr/bin/true\n"}}
	installFakeSiteCrontabs(t, fake, "shop_example")
	for _, record := range []*passwordlessRecord{nil, {RelayAfter: "permit_sasl_authenticated"}} {
		if note := passwordlessCronStep(record, time.Now()); note != "" || len(fake.installs) != 0 {
			t.Fatalf("ran before authorized_submit_users was closed: %q", note)
		}
	}
}
