package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// S4 R1 (21 Nov 2026): a site's own code can run `crontab` as its account and
// add lines to its crontab; the server owner can edit it by hand. Neither is
// blocked, but the Scheduled tasks list must not show such a line as if
// CelikPanel had made it, and a line that is not a task (an @reboot line, an
// environment setting) must not be invisible. This test uses only what the
// list already answered before the change, so it builds on the base and
// fails there.
func TestCrontabLinesNotMadeInThePanelAreMarkedAndRemovable(t *testing.T) {
	fake := installFakeCrontab(t, noCrontabYet())
	if err := addCronJobFor(cronTestUser, &transport.AddCronJobRequest{
		Schedule: "*/5 * * * *", Command: "/usr/bin/php /var/www/example.com/cron.php", Comment: "site cron", Version: cronVersion(""),
	}); err != nil {
		t.Fatalf("add: %v", err)
	}
	// Written outside CelikPanel, after it: a task, a line that runs at every
	// start of the server, and a setting for every task.
	fake.content += "* * * * * /tmp/x.sh\n@reboot /tmp/y.sh\nSHELL=/bin/bash\n"

	list := func() map[string]any {
		t.Helper()
		var resp ListCronJobsResponse
		if err := listCronJobsFor(cronTestUser, &resp); err != nil {
			t.Fatalf("list: %v", err)
		}
		raw, _ := json.Marshal(resp)
		var out map[string]any
		_ = json.Unmarshal(raw, &out)
		return out
	}
	got := list()
	marked := map[string]bool{}
	for _, job := range got["jobs"].([]any) {
		row := job.(map[string]any)
		marked[row["command"].(string)] = row["outside_panel"] == true
	}
	if marked["/usr/bin/php /var/www/example.com/cron.php"] || !marked["/tmp/x.sh"] {
		t.Fatalf("marks %v: want only the task added outside CelikPanel marked", marked)
	}
	others, _ := got["other_lines"].([]any)
	var otherText []string
	rebootID := ""
	for _, other := range others {
		row := other.(map[string]any)
		otherText = append(otherText, row["line"].(string))
		if row["line"] == "@reboot /tmp/y.sh" {
			rebootID, _ = row["id"].(string)
		}
	}
	if strings.Join(otherText, "|") != "@reboot /tmp/y.sh|SHELL=/bin/bash" || got["outside_lines"] != float64(3) {
		t.Fatalf("other lines %v, outside %v: want the @reboot and SHELL lines listed, 3 lines in all", otherText, got["outside_lines"])
	}

	// The @reboot line is removed by its ID, like a task.
	version, _ := got["version"].(string)
	if err := deleteCronJobFor(cronTestUser, &transport.DeleteCronJobRequest{ID: rebootID, Version: version}); err != nil {
		t.Fatalf("delete the @reboot line: %v", err)
	}
	if strings.Contains(fake.content, "@reboot") {
		t.Fatalf("the @reboot line is still there:\n%s", fake.content)
	}

	// A later write by CelikPanel keeps the lines it did not make, and never
	// makes them its own.
	got = list()
	if err := addCronJobFor(cronTestUser, &transport.AddCronJobRequest{
		Schedule: "0 3 * * *", Command: "/usr/bin/php /var/www/example.com/nightly.php", Version: got["version"].(string),
	}); err != nil {
		t.Fatalf("second add: %v", err)
	}
	got = list()
	if got["outside_lines"] != float64(2) {
		t.Fatalf("after CelikPanel wrote again: %v lines not made in it, want 2 (the task and SHELL)", got["outside_lines"])
	}
	// The panel's own lines can never be removed as a line "not made in
	// CelikPanel": the MAILTO line it wrote has no such ID.
	for _, other := range got["other_lines"].([]any) {
		if strings.HasPrefix(other.(map[string]any)["line"].(string), "MAILTO") {
			t.Fatalf("CelikPanel's own MAILTO line is listed as not made in it")
		}
	}
}
