package sitecron

import (
	"reflect"
	"strings"
	"testing"
)

// TestTranslateTable is the translation table (S4 R2b): every schedule form
// the product writes (five fields of "*", numbers, ranges, lists and "*/n",
// cron_rpc.go isValidCronSchedule), the forms an owner's crontab may hold
// that cron 3.0pl1 accepts (names, steps on ranges, 7 for Sunday, the
// special strings), and the forms that are refused. The OR of the two day
// fields is two calendar lines; a field that starts with "*" (also "*/n")
// keeps cron's AND.
func TestTranslateTable(t *testing.T) {
	cases := []struct {
		schedule string
		want     []string
		reboot   bool
		refused  string // part of the refusal
	}{
		// Forms the product's own writer makes.
		{schedule: "* * * * *", want: []string{"*-*-* *:*:00"}},
		{schedule: "*/15 * * * *", want: []string{"*-*-* *:00,15,30,45:00"}},
		{schedule: "0 3 * * *", want: []string{"*-*-* 03:00:00"}},
		{schedule: "17 3 * * *", want: []string{"*-*-* 03:17:00"}},
		{schedule: "*/30 * * * *", want: []string{"*-*-* *:00,30:00"}},
		{schedule: "5-10 * * * *", want: []string{"*-*-* *:05..10:00"}},
		{schedule: "0,30 8-17 * * *", want: []string{"*-*-* 08..17:00,30:00"}},
		{schedule: "0 */2 * * *", want: []string{"*-*-* 00,02,04,06,08,10,12,14,16,18,20,22:00:00"}},
		{schedule: "30 9 * * 1-5", want: []string{"Mon,Tue,Wed,Thu,Fri *-*-* 09:30:00"}},
		{schedule: "0 0 1,15 * *", want: []string{"*-*-01,15 00:00:00"}},
		{schedule: "0 0 * * 0", want: []string{"Sun *-*-* 00:00:00"}},
		{schedule: "0 0 1 1 *", want: []string{"*-01-01 00:00:00"}},
		{schedule: "0 6 * 1-3 *", want: []string{"*-01..03-* 06:00:00"}},
		// cron's OR of the two day fields: two lines.
		{schedule: "0 12 1 * 1", want: []string{"*-*-01 12:00:00", "Mon *-*-* 12:00:00"}},
		{schedule: "0 0 1-7 * 1", want: []string{"*-*-01..07 00:00:00", "Mon *-*-* 00:00:00"}},
		{schedule: "0 0 13 * 5", want: []string{"*-*-13 00:00:00", "Fri *-*-* 00:00:00"}},
		// A day field written "*/n" starts with "*": cron ANDs.
		{schedule: "0 0 */2 * 1", want: []string{"Mon *-*-01,03,05,07,09,11,13,15,17,19,21,23,25,27,29,31 00:00:00"}},
		{schedule: "0 0 1 * */2", want: []string{"Tue,Thu,Sat,Sun *-*-01 00:00:00"}},
		// Forms of an owner's crontab that cron accepts.
		{schedule: "0 0 * * 7", want: []string{"Sun *-*-* 00:00:00"}},
		{schedule: "0 0 * * sat,sun", want: []string{"Sat,Sun *-*-* 00:00:00"}},
		{schedule: "0 0 * * Mon-Fri", want: []string{"Mon,Tue,Wed,Thu,Fri *-*-* 00:00:00"}},
		{schedule: "0 0 1 jan,jul *", want: []string{"*-01,07-01 00:00:00"}},
		{schedule: "1-30/5 * * * *", want: []string{"*-*-* *:01,06,11,16,21,26:00"}},
		{schedule: "0 2-22/4 * * *", want: []string{"*-*-* 02,06,10,14,18,22:00:00"}},
		{schedule: "0 0 29 2 *", want: []string{"*-02-29 00:00:00"}},
		{schedule: "0-59 0-23 1-31 1-12 0-6", want: []string{"*-*-* *:*:00"}},
		{schedule: "@yearly", want: []string{"*-01-01 00:00:00"}},
		{schedule: "@annually", want: []string{"*-01-01 00:00:00"}},
		{schedule: "@monthly", want: []string{"*-*-01 00:00:00"}},
		{schedule: "@weekly", want: []string{"Sun *-*-* 00:00:00"}},
		{schedule: "@daily", want: []string{"*-*-* 00:00:00"}},
		{schedule: "@midnight", want: []string{"*-*-* 00:00:00"}},
		{schedule: "@hourly", want: []string{"*-*-* *:00:00"}},
		{schedule: "@reboot", reboot: true},
		// Refused.
		{schedule: "60 * * * *", refused: "outside 0-59"},
		{schedule: "* 24 * * *", refused: "outside 0-23"},
		{schedule: "* * 0 * *", refused: "outside 1-31"},
		{schedule: "* * * 13 *", refused: "outside 1-12"},
		{schedule: "* * * * 8", refused: "outside 0-7"},
		{schedule: "10-5 * * * *", refused: "backwards"},
		{schedule: "*/0 * * * *", refused: "step"},
		{schedule: "5/10 * * * *", refused: "needs a range"},
		{schedule: "L * * * *", refused: "not a value"},
		{schedule: "* * * * MON#2", refused: "not a value"},
		{schedule: "? * * * *", refused: "not a value"},
		{schedule: "* * * * * *", refused: "five fields"},
		{schedule: "@every", refused: "special schedule"},
		{schedule: "1,,2 * * * *", refused: "empty value"},
	}
	for _, c := range cases {
		spec, err := Translate(c.schedule)
		if c.refused != "" {
			if err == nil || !strings.Contains(err.Error(), c.refused) {
				t.Errorf("%q: want a refusal with %q, got %v %v", c.schedule, c.refused, spec, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: %v", c.schedule, err)
			continue
		}
		if spec.Reboot != c.reboot || !reflect.DeepEqual(spec.Calendars, c.want) {
			t.Errorf("%q: got %+v, want %v reboot=%v", c.schedule, spec, c.want, c.reboot)
		}
	}
}

// TestSplitCommandLikeCron: "%" ends the command, the rest is standard input
// with a final newline; "\%" is "%" in both parts (the bytes measured with
// od -c on both systems for "od -c%line one%line two\%literal").
func TestSplitCommandLikeCron(t *testing.T) {
	cases := []struct {
		rest, command, input string
		has                  bool
	}{
		{`od -c > $HOME/x%line one%line two\%literal`, `od -c > $HOME/x`, "line one\nline two%literal\n", true},
		{`date +\%T >> log`, `date +%T >> log`, "", false},
		{`echo a\\%b`, `echo a\\`, "b\n", true},
		{`cat%`, `cat`, "", false},
		{`cat%x%`, `cat`, "x\n", true},
		{`cat%a\b`, `cat`, "a\\b\n", true},
		{`printf '\%s|\%s\n' "a b" 'c"d'`, `printf '%s|%s\n' "a b" 'c"d'`, "", false},
	}
	for _, c := range cases {
		command, input, has := SplitCommand(c.rest)
		if command != c.command || input != c.input || has != c.has {
			t.Errorf("%q: got %q %q %v, want %q %q %v", c.rest, command, input, has, c.command, c.input, c.has)
		}
	}
}

// TestParseLines: what keeps a crontab in cron (an untranslatable line) and
// what is carried (settings, paused tasks, comments).
func TestParseLines(t *testing.T) {
	text := "# CelikPanel: cron output of this site is not mailed\n" +
		"MAILTO=\"\"\n" +
		"PATH = /usr/local/bin:/usr/bin:/bin\n" +
		"SHELL=/bin/bash\n" +
		"# nightly\n" +
		"17 3 * * * php artisan schedule:run\n" +
		"# DISABLED: */5 * * * * curl -s https://example.test/\n" +
		"@reboot /usr/bin/true\n" +
		"\n"
	c := Parse(text)
	if bad := c.Untranslatable(); len(bad) != 0 {
		t.Fatalf("untranslatable: %+v", bad)
	}
	if c.Shell != "/bin/bash" {
		t.Fatalf("shell %q", c.Shell)
	}
	jobs := c.Jobs()
	if len(jobs) != 2 || jobs[0].Command != "php artisan schedule:run" || jobs[0].Line != 6 || !jobs[1].Spec.Reboot {
		t.Fatalf("jobs %+v", jobs)
	}
	if got := c.EnvFileBody(); got != "MAILTO=''\nPATH='/usr/local/bin:/usr/bin:/bin'\nSHELL='/bin/bash'\n" {
		t.Fatalf("env file %q", got)
	}
	if c.Entries[6].Kind != KindDisabled {
		t.Fatalf("paused task: %+v", c.Entries[6])
	}

	refused := map[string]string{
		"MAILTO=owner@example.test\n": ReasonMailto,
		"HOME=/tmp\n":                 ReasonHome,
		"SHELL=bash\n":                ReasonShell,
		"X='it''s'\n":                 ReasonSchedule, // cron: not a setting (text after the closing quote), so a job, which fails
		"X=a\\b\n":                    ReasonEnvValue,
		"0 3 * * * php x.php\r\n":     ReasonCarriage,
		"0 3 * * *\n":                 ReasonEmpty,
		"0 3 * *\n":                   ReasonSchedule,
		"@every 5m x\n":               ReasonSchedule,
		"0 3 * * mon#1 x\n":           ReasonSchedule,
		"0 3 L * * x\n":               ReasonSchedule,
		"MY-VAR=1\n":                  ReasonInvalidEnv,
		// alpha.150 fix round (F5): part of the schedule for cronie.
		"CRON_TZ=Europe/Berlin\n":  ReasonCronSetting,
		"RANDOM_DELAY=30\n":        ReasonCronSetting,
		"START_HOURS_RANGE=3-22\n": ReasonCronSetting,
	}
	for line, reason := range refused {
		bad := Parse(line).Untranslatable()
		if len(bad) != 1 || bad[0].Reason != reason {
			t.Errorf("%q: want %s, got %+v", line, reason, bad)
		}
	}
}

// TestParseLinesLikeCron3 (S4 R2b fix round, review finding 7): a line is
// read exactly as Debian's cron 3.0pl1 reads it (entry.c, env.c, cron.h),
// or refused with a reason - never silently read differently:
//   - only ASCII spaces and tabs separate fields (Skip_Blanks); a no-break
//     space or U+3000 is part of a field, which is then not a value;
//   - the command keeps its trailing blanks (get_string reads to the newline);
//   - a command of MAX_COMMAND-1 (999) bytes or more is refused by cron
//     (e_cmd_len), so it is refused here;
//   - settings follow load_env's states: "A=" is the empty value, `A="x` and
//     `A="x"y` are not settings (cron then fails them as jobs), quotes and
//     blanks come off as strdtb and the dequoting do;
//   - a value systemd cannot read (not UTF-8, U+FEFF, a noncharacter) is a
//     line that cannot be translated (env_value).
func TestParseLinesLikeCron3(t *testing.T) {
	long := strings.Repeat("x", MaxCommand-2) // 998 bytes: cron runs it
	type want struct {
		kind           Kind
		reason         string
		command, input string
		name, value    string
		schedule, text string
	}
	cases := []struct {
		line string
		want want
	}{
		// Field separators.
		{"* * * * *\u00a0echo hi", want{kind: KindInvalid, reason: ReasonSchedule}},
		{"*\u00a0* * * * echo hi", want{kind: KindInvalid, reason: ReasonSchedule}},
		{"* * * * *\u3000echo hi", want{kind: KindInvalid, reason: ReasonSchedule}},
		{"* * * *\u2003* echo hi", want{kind: KindInvalid, reason: ReasonSchedule}},
		{"*\t*\t*\t*\t*\techo\u00a0hi", want{kind: KindJob, schedule: "* * * * *", command: "echo\u00a0hi", text: "*\t*\t*\t*\t*\techo\u00a0hi"}},
		{"\u00a0", want{kind: KindInvalid, reason: ReasonSchedule}},
		// Trailing blanks stay in the command and its input.
		{"* * * * * echo foo\\ ", want{kind: KindJob, schedule: "* * * * *", command: "echo foo\\ ", text: "* * * * * echo foo\\"}},
		{"* * * * * cat%abc   ", want{kind: KindJob, schedule: "* * * * *", command: "cat", input: "abc   \n", text: "* * * * * cat%abc"}},
		{"@daily  run.sh\t", want{kind: KindJob, schedule: "@daily", command: "run.sh\t", text: "@daily  run.sh"}},
		// The command limit.
		{"* * * * * " + long, want{kind: KindJob, schedule: "* * * * *", command: long, text: "* * * * * " + long}},
		{"* * * * * " + long + "y", want{kind: KindInvalid, reason: ReasonTooLong}},
		{"@hourly " + long + "yz", want{kind: KindInvalid, reason: ReasonTooLong}},
		// Settings as load_env reads them.
		{"MAILTO=", want{kind: KindEnv, name: "MAILTO", value: ""}},
		{"MAILTO=\"\"", want{kind: KindEnv, name: "MAILTO", value: ""}},
		{"A = x  ", want{kind: KindEnv, name: "A", value: "x"}},
		{"A=\" x \"", want{kind: KindEnv, name: "A", value: " x"}},
		{"A=' x'  ", want{kind: KindEnv, name: "A", value: " x"}},
		{"\"A\"=x", want{kind: KindEnv, name: "A", value: "x"}},
		{"A=x y", want{kind: KindEnv, name: "A", value: "x y"}},
		{"A=\"'x'\"", want{kind: KindEnv, name: "A", value: "x"}},
		{"A=\"x", want{kind: KindInvalid, reason: ReasonSchedule}},
		{"A=\"x\"y", want{kind: KindInvalid, reason: ReasonSchedule}},
		{"A=\"", want{kind: KindInvalid, reason: ReasonInvalidEnv}},
		{"A B=c", want{kind: KindInvalid, reason: ReasonSchedule}},
		{"A" + strings.Repeat("v", MaxEnvLine), want{kind: KindInvalid, reason: ReasonSchedule}},
		{"A=" + strings.Repeat("v", MaxEnvLine), want{kind: KindInvalid, reason: ReasonTooLong}},
		// Values systemd cannot read.
		{"GREETING=caf\xe9", want{kind: KindInvalid, reason: ReasonEnvValue}},
		{"X=\ufeffa", want{kind: KindInvalid, reason: ReasonEnvValue}},
		{"X=a\ufffe", want{kind: KindInvalid, reason: ReasonEnvValue}},
		{"X=a\ufdd0", want{kind: KindInvalid, reason: ReasonEnvValue}},
		{"X=caf\u00e9", want{kind: KindEnv, name: "X", value: "caf\u00e9"}},
	}
	for _, c := range cases {
		got := Parse(c.line + "\n").Entries[0]
		w := c.want
		if got.Kind != w.kind || got.Reason != w.reason {
			t.Errorf("%q: kind %d reason %q (%s), want kind %d reason %q", c.line, got.Kind, got.Reason, got.Detail, w.kind, w.reason)
			continue
		}
		switch w.kind {
		case KindJob:
			if got.Schedule != w.schedule || got.Command != w.command || got.Input != w.input || got.Text != w.text {
				t.Errorf("%q: schedule %q command %q input %q text %q", c.line, got.Schedule, got.Command, got.Input, got.Text)
			}
		case KindEnv:
			if got.Name != w.name || got.Value != w.value {
				t.Errorf("%q: name %q value %q", c.line, got.Name, got.Value)
			}
		}
	}
	// A setting that cannot be read never reaches the environment file.
	if body := Parse("GREETING=caf\xe9\nX=1\n").EnvFileBody(); body != "X='1'\n" {
		t.Errorf("environment file %q", body)
	}
	if !CommandFits(long) || CommandFits(long+"y") {
		t.Error("CommandFits")
	}
	if got := SplitFields(" a\u00a0b\tc  d "); strings.Join(got, "|") != "a\u00a0b|c|d" {
		t.Errorf("SplitFields %q", got)
	}
}

// TestUnitTexts: the rendered pair names the slice, the account and home,
// cron's umask and environment order, the script (never the command), the
// SMTP block and the address rule; the timer keeps cron's minute and does
// not catch up.
func TestUnitTexts(t *testing.T) {
	u := Unit{SiteID: 7, N: 3, TaskID: "0123456789abcdef", Domain: "shop.example.test", Account: "shop_example_test", Group: "shop_example_test",
		Home: "/var/www/celikpanel/subscriptions/2/sites/9", Shell: "/bin/sh", HasEnv: true, HasInput: true, Line: 4,
		Spec: Spec{Calendars: []string{"*-*-01 12:00:00", "Mon *-*-* 12:00:00"}}, Egress: true, BPF: true,
		SliceFile: "/etc/systemd/system/celiksite_7.slice"}
	service, err := ServiceText(u)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"# celikpanel-task: 0123456789abcdef\n", "Type=oneshot\n", "User=shop_example_test\n", "Group=shop_example_test\n",
		"Slice=celiksite_7.slice\n", "WorkingDirectory=/var/www/celikpanel/subscriptions/2/sites/9\n", "UMask=0002\n",
		"Environment=PATH=/usr/bin:/bin SHELL=/bin/sh\nEnvironmentFile=-/etc/environment\nEnvironmentFile=-/etc/default/locale\nEnvironmentFile=/etc/celikpanel-cron/7.d/env\n",
		"StandardInput=file:/etc/celikpanel-cron/7.d/3.in\n",
		// Second fix round: the task's own copy of the helper, which no
		// earlier release manages (going back keeps the tasks starting).
		"ExecStartPre=+/usr/lib/celikpanel-sandbox/sandbox-task-helper egress-add --unit=celikcron-7-3.service\n",
		"ExecStart=/bin/sh /etc/celikpanel-cron/7.d/3.sh\n",
		"ExecStopPost=-+/usr/lib/celikpanel-sandbox/sandbox-task-helper egress-remove --unit=celikcron-7-3.service\n",
		"RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX AF_NETLINK\n", "IPAddressDeny=169.254.0.0/16",
		// Fix round: what a task leaves running keeps running in its own
		// cgroup (inside the slice, under the SMTP block), as under cron;
		// the programs' log rate limit and no core files.
		"Type=oneshot\nKillMode=process\n", "LogRateLimitIntervalSec=30s\n", "LogRateLimitBurst=1000\n", "LimitCORE=0\n",
		// Second fix round: NoNewPrivileges is set, not assumed (measured:
		// RestrictAddressFamilies= with User= does not imply it); and the run
		// is skipped while the site's limits file is missing, also after a
		// reboot and without the panel.
		"NoNewPrivileges=yes\n",
		"[Unit]\nDescription=CelikPanel task 3 of shop.example.test\nConditionPathExists=/etc/systemd/system/celiksite_7.slice\n",
	} {
		if !strings.Contains(service, want) {
			t.Errorf("service lacks %q:\n%s", want, service)
		}
	}
	if strings.Contains(service, "/sandbox-helper ") {
		t.Errorf("a task names the helper an earlier release overwrites:\n%s", service)
	}
	timer, err := TimerText(u)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"OnCalendar=*-*-01 12:00:00\nOnCalendar=Mon *-*-* 12:00:00\n", "AccuracySec=1s\n", "Persistent=false\n", "WantedBy=timers.target\n"} {
		if !strings.Contains(timer, want) {
			t.Errorf("timer lacks %q:\n%s", want, timer)
		}
	}
	u.Spec = Spec{Reboot: true}
	if timer, _ = TimerText(u); !strings.Contains(timer, "OnBootSec=1min\n") || strings.Contains(timer, "OnCalendar") {
		t.Errorf("reboot timer:\n%s", timer)
	}
	for _, bad := range []func(*Unit){
		func(u *Unit) { u.Account = "root" },
		func(u *Unit) { u.Home = "/var/www/../etc" },
		func(u *Unit) { u.Shell = "bash" },
		func(u *Unit) { u.Domain = "a b" },
		func(u *Unit) { u.Spec = Spec{Calendars: []string{"*-*-* 00:00:00\nExecStart=/bin/x"}} },
		func(u *Unit) { u.TaskID = "x" },
		func(u *Unit) { u.SliceFile = "" },
		func(u *Unit) { u.SliceFile = "/etc/systemd/system/celiksite_8.slice" },
		func(u *Unit) { u.SliceFile = "/etc/systemd/system/x\nExecStart=/bin/x/celiksite_7.slice" },
	} {
		v := u
		bad(&v)
		if _, err := ServiceText(v); err == nil {
			t.Errorf("not refused: %+v", v)
		}
	}
}
