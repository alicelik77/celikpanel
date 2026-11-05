package main

import (
	"strings"
	"testing"
)

func TestSiteCronMailtoAddedOnlyWhereNobodySetIt(t *testing.T) {
	plain := "*/5 * * * * /usr/bin/php /var/www/site/cron.php\n"
	got := withSiteCronMailto("shop_example", plain)
	if got != cronMailtoComment+"\nMAILTO=\"\"\n"+plain {
		t.Fatalf("%q", got)
	}
	// Idempotent: the second write keeps the one line.
	if again := withSiteCronMailto("shop_example", got); again != got {
		t.Fatalf("%q", again)
	}
	// The owner's own MAILTO (any spelling cron accepts) is kept untouched.
	for _, owner := range []string{"MAILTO=owner@example.com\n" + plain, "  MAILTO = \"\"\n" + plain, plain + "MAILTO=me@example.org\n"} {
		if out := withSiteCronMailto("shop_example", owner); out != owner {
			t.Fatalf("owner crontab rewritten: %q", out)
		}
	}
	// root still sends mail; its crontab is never changed.
	if out := withSiteCronMailto("root", plain); out != plain {
		t.Fatalf("%q", out)
	}
	// The comment and the assignment are not jobs and never attach to one.
	jobs := parseCrontab(got)
	if len(jobs) != 1 || jobs[0].Comment != "" || !strings.Contains(jobs[0].Command, "cron.php") {
		t.Fatalf("%+v", jobs)
	}
}

func TestUpdateCronJobAddsSiteMailto(t *testing.T) {
	content := "0 3 * * * /usr/bin/true\n"
	fake := installFakeCrontab(t, &fakeCrontab{content: content})
	err := updateCronJobFor(cronTestUser, &UpdateCronJobRequest{
		ID: generateCronID("0 3 * * * /usr/bin/true"), Schedule: "0 4 * * *", Command: "/usr/bin/true", Enabled: true,
		Version: cronVersion(content),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(fake.installs) != 1 || !strings.HasPrefix(fake.installs[0], cronMailtoComment+"\nMAILTO=\"\"\n0 4 * * * /usr/bin/true") {
		t.Fatalf("%q", fake.installs)
	}
}
