package sitecron

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/alicelik/celikpanel/internal/sitesandbox"
)

// Files of a moved site's tasks (docs/SITE-SANDBOX.md section 9):
//
//	/etc/celikpanel-cron/<site id>.crontab        the source: crontab syntax, root's (0640 root:celikpanel)
//	/etc/celikpanel-cron/<site id>.d/env          its NAME=value settings (an environment file)
//	/etc/celikpanel-cron/<site id>.d/<n>.sh       task n's command, as cron hands it to the shell
//	/etc/celikpanel-cron/<site id>.d/<n>.in       task n's standard input ("%" in cron), if any
//	/etc/systemd/system/celikcron-<site id>-<n>.service and .timer
//
// The command and the input live in files the site's account can read (root
// with the site's group, 0640, in a 0750 folder of that group) and never in
// a unit's own settings: systemd tells every local account a unit's
// settings (systemctl show), while a crontab is readable by its account
// only. The environment file is read by systemd as root.
// SourceDir is a variable so tests can point it at a temporary folder.
var SourceDir = "/etc/celikpanel-cron"

const (
	// UnitPrefix starts every task unit's name.
	UnitPrefix = "celikcron-"
	// TaskMarker is the comment line in a task's service that names the task
	// (its ID on the Scheduled tasks screen), so a task keeps its number.
	TaskMarker = "# celikpanel-task: "
	// MaxTasks bounds the timers of one site.
	MaxTasks = 500
)

// SourcePath is a site's task source.
func SourcePath(siteID int) string { return SourceDir + "/" + strconv.Itoa(siteID) + ".crontab" }

// TaskDir holds a site's task files.
func TaskDir(siteID int) string { return SourceDir + "/" + strconv.Itoa(siteID) + ".d" }

// EnvPath is a site's environment file.
func EnvPath(siteID int) string { return TaskDir(siteID) + "/env" }

// ScriptPath and InputPath are task n's command and input files.
func ScriptPath(siteID, n int) string { return TaskDir(siteID) + "/" + strconv.Itoa(n) + ".sh" }
func InputPath(siteID, n int) string  { return TaskDir(siteID) + "/" + strconv.Itoa(n) + ".in" }

// UnitBase is task n's unit name without the suffix: celikcron-<site>-<n>.
// A dash is fine here: only slice names nest at a dash.
func UnitBase(siteID, n int) string {
	return UnitPrefix + strconv.Itoa(siteID) + "-" + strconv.Itoa(n)
}

// UnitPattern matches a task's service or timer file name.
var UnitPattern = regexp.MustCompile(`^celikcron-([1-9][0-9]{0,9})-([1-9][0-9]{0,3})\.(service|timer)$`)

// ParseUnit reads a task unit's name back.
func ParseUnit(name string) (siteID, n int, suffix string, ok bool) {
	m := UnitPattern.FindStringSubmatch(name)
	if m == nil {
		return 0, 0, "", false
	}
	siteID, _ = strconv.Atoi(m[1])
	n, _ = strconv.Atoi(m[2])
	return siteID, n, m[3], true
}

// Unit is everything one task's unit pair is made of.
type Unit struct {
	SiteID int
	N      int
	// TaskID is the task's ID on the Scheduled tasks screen.
	TaskID string
	Domain string
	// Account and Group run the task; Home is its working folder (the
	// account's home, as cron's).
	Account, Group, Home string
	// Shell runs the script (cron's SHELL).
	Shell string
	// HasEnv: the crontab has settings (EnvPath is read); HasInput: the task
	// has standard input (InputPath).
	HasEnv, HasInput bool
	Spec             Spec
	// Line is the source line the task comes from.
	Line int
	// Egress: the host holds the SMTP block (the unit adds itself to it);
	// BPF: systemd can enforce IPAddressDeny=.
	Egress, BPF bool
}

var (
	accountPattern = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)
	domainPattern  = regexp.MustCompile(`^[a-z0-9.-]{1,253}$`)
	taskIDPattern  = regexp.MustCompile(`^[0-9a-f]{16}(#[0-9]{1,3})?$`)
	homePattern    = regexp.MustCompile(`^/[A-Za-z0-9/_.-]{1,400}$`)
	calendarChars  = regexp.MustCompile(`^[A-Za-z0-9*:.,\- ]{1,600}$`)
)

func (u Unit) validate() error {
	switch {
	case u.SiteID < 1 || u.N < 1 || u.N > 9999:
		return errors.New("a task needs its site and a number")
	case !taskIDPattern.MatchString(u.TaskID):
		return errors.New("not a task ID")
	case !domainPattern.MatchString(u.Domain):
		return errors.New("not a domain name")
	case !accountPattern.MatchString(u.Account) || !accountPattern.MatchString(u.Group) || u.Account == "root":
		return errors.New("not a site account")
	case !homePattern.MatchString(u.Home) || path.Clean(u.Home) != u.Home || strings.Contains(u.Home, ".."):
		return errors.New("not a home folder")
	case !shellPathPattern.MatchString(u.Shell):
		return errors.New("not a shell path")
	case !u.Spec.Reboot && (len(u.Spec.Calendars) == 0 || len(u.Spec.Calendars) > 2):
		return errors.New("a task needs when it runs")
	}
	for _, c := range u.Spec.Calendars {
		if !calendarChars.MatchString(c) {
			return fmt.Errorf("not a calendar: %q", c)
		}
	}
	return nil
}

// ServiceText is the task's service (without CelikPanel's render header).
//
// The task runs as cron runs the same line (Stage 1 F2): the account and its
// groups, the home as the working folder, umask 0002 (cron's pam_umask),
// PATH=/usr/bin:/bin and SHELL=/bin/sh unless /etc/environment or the
// crontab says otherwise (EnvironmentFile= order: later files win), and the
// crontab's shell running the command. It runs in the site's slice under the
// network rules of the site's other programs (the SMTP block, no link-local
// or metadata addresses, the same address families). It is not given the
// programs' file system sandbox: a cron job never had one, and it would stop
// tasks that write the site's files.
func ServiceText(u Unit) (string, error) {
	if err := u.validate(); err != nil {
		return "", err
	}
	unit := UnitBase(u.SiteID, u.N) + ".service"
	var b strings.Builder
	b.WriteString("# CelikPanel scheduled task " + strconv.Itoa(u.N) + " of " + u.Domain + " (docs/SITE-SANDBOX.md section 9).\n")
	b.WriteString(TaskMarker + u.TaskID + "\n")
	b.WriteString("# Made from line " + strconv.Itoa(u.Line) + " of " + SourcePath(u.SiteID) + ": change the task there (or on the\n")
	b.WriteString("# site's Scheduled tasks screen), not here. It runs without CelikPanel:\n")
	b.WriteString("#   systemctl status " + unit + " ; journalctl -u " + unit + "\n")
	b.WriteString("#\n# CelikPanel zamanlanmis gorevi: sitenin kendi sinirlari icinde calisir.\n")
	b.WriteString("[Unit]\n")
	b.WriteString("Description=CelikPanel task " + strconv.Itoa(u.N) + " of " + u.Domain + "\n")
	b.WriteString("\n[Service]\n")
	b.WriteString("Type=oneshot\n")
	b.WriteString("User=" + u.Account + "\n")
	b.WriteString("Group=" + u.Group + "\n")
	b.WriteString("Slice=" + sitesandbox.SliceName(u.SiteID) + "\n")
	b.WriteString("WorkingDirectory=" + u.Home + "\n")
	b.WriteString("UMask=0002\n")
	b.WriteString("Environment=PATH=/usr/bin:/bin SHELL=/bin/sh\n")
	b.WriteString("EnvironmentFile=-/etc/environment\n")
	b.WriteString("EnvironmentFile=-/etc/default/locale\n")
	if u.HasEnv {
		b.WriteString("EnvironmentFile=" + EnvPath(u.SiteID) + "\n")
	}
	if u.HasInput {
		b.WriteString("StandardInput=file:" + InputPath(u.SiteID, u.N) + "\n")
	}
	if u.Egress {
		b.WriteString("ExecStartPre=+" + sitesandbox.HelperCommand(sitesandbox.EgressAddArgs(unit)) + "\n")
	}
	b.WriteString("ExecStart=" + u.Shell + " " + ScriptPath(u.SiteID, u.N) + "\n")
	if u.Egress {
		b.WriteString("ExecStopPost=-+" + sitesandbox.HelperCommand(sitesandbox.EgressRemoveArgs(unit)) + "\n")
	}
	b.WriteString("RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX AF_NETLINK\n")
	if u.BPF {
		b.WriteString("IPAddressDeny=" + strings.Join(sitesandbox.DeniedAddresses, " ") + "\n")
	}
	return b.String(), nil
}

// TimerText is the task's timer (without CelikPanel's render header).
func TimerText(u Unit) (string, error) {
	if err := u.validate(); err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("# CelikPanel scheduled task " + strconv.Itoa(u.N) + " of " + u.Domain + ": when it runs (docs/SITE-SANDBOX.md section 9).\n")
	b.WriteString("# A run that is still going when its next time comes is not started a second time;\n")
	b.WriteString("# it starts once more right after it ends. Change the time in " + SourcePath(u.SiteID) + ".\n")
	b.WriteString("[Unit]\n")
	b.WriteString("Description=CelikPanel task " + strconv.Itoa(u.N) + " of " + u.Domain + ": timer\n")
	b.WriteString("\n[Timer]\n")
	if u.Spec.Reboot {
		b.WriteString("OnBootSec=1min\n")
	}
	for _, c := range u.Spec.Calendars {
		b.WriteString("OnCalendar=" + c + "\n")
	}
	b.WriteString("AccuracySec=1s\n")
	b.WriteString("Persistent=false\n")
	b.WriteString("\n[Install]\n")
	b.WriteString("WantedBy=timers.target\n")
	return b.String(), nil
}

// ScriptText is task n's command file: a comment, then the command exactly
// as cron hands it to the shell (the shell reads the file; its $0 is the
// file's path, the one difference from "sh -c").
func ScriptText(n int, line int, command string) string {
	return "# CelikPanel task " + strconv.Itoa(n) + ", from line " + strconv.Itoa(line) + " of the site's task file.\n" + command + "\n"
}
