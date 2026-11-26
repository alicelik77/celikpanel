// Package sitecron turns a site's crontab text into systemd timers that run
// in the site's own slice (S4 R2b, docs/SITE-SANDBOX.md section 9). It only
// parses and renders text; the Agent writes the files.
//
// What it rests on (measured 21 Nov 2026 on throwaway Debian 13 / systemd
// 257 / cron 3.0pl1-197 and Ubuntu 24.04 / systemd 255 / cron
// 3.0pl1-184ubuntu2 machines, raw outputs in the pull request):
//
//   - cron runs a line when minute, hour and month match and, for the two day
//     fields, both match when either field starts with "*", otherwise either
//     one (crontab(5)). A timer ANDs a weekday with a day of the month, and
//     several OnCalendar= lines OR, so cron's OR is two lines; both systems
//     accepted two lines and listed both next times.
//   - A timer's default AccuracySec= is one minute: AccuracySec=1s keeps
//     cron's minute. Persistent= stays off, as cron catches nothing up.
//   - "%" in a command ends it; the rest, every further "%" a newline, is the
//     job's standard input, with a final newline (od -c gave the same bytes
//     for cron and the timer). "\%" is a literal "%".
//   - A timer never starts its service while the service still runs; a time
//     that passed meanwhile starts it once right after it ends (both
//     Type=oneshot and Type=simple, both systems). cron starts a second copy.
//
// Paket sitecron, bir sitenin crontab metnini sitenin kendi diliminde çalışan
// systemd zamanlayıcılarına çevirir; yalnız metin okur ve yazar.
package sitecron

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Kind is what a crontab line is.
type Kind int

const (
	KindBlank Kind = iota
	KindComment
	// KindEnv: NAME=value (crontab(5) "environment setting").
	KindEnv
	// KindJob: a schedule and a command.
	KindJob
	// KindDisabled: a task CelikPanel paused ("# DISABLED: <task>"); no timer.
	KindDisabled
	// KindInvalid: a line cron would refuse or that this package does not
	// read (Entry.Reason says why).
	KindInvalid
)

// Reasons a line cannot become a timer. They travel to the screens as codes.
const (
	ReasonSchedule   = "schedule"    // a schedule form cron or this translation does not know
	ReasonMailto     = "mailto"      // MAILTO with an address: a timer mails nothing
	ReasonHome       = "home"        // HOME= changes cron's working folder
	ReasonShell      = "shell"       // SHELL= that is not a plain absolute path
	ReasonEnvValue   = "env_value"   // a value a systemd environment file cannot hold exactly
	ReasonCarriage   = "carriage"    // a carriage return or another control character
	ReasonEmpty      = "empty"       // a schedule without a command
	ReasonInvalidEnv = "invalid_env" // a NAME=value line with a name cron does not take
	ReasonTooLong    = "too_long"    // a command (or setting) longer than cron reads (MAX_COMMAND, MAX_ENVSTR)
	// ReasonCronSetting: CRON_TZ=, RANDOM_DELAY= or START_HOURS_RANGE=, which
	// cronie (and systemd-cron) read as part of the schedule and Debian's cron
	// 3.0pl1 keeps as a plain setting (alpha.150 fix round, F5). A timer would
	// keep only the setting, so the schedule could change silently; which cron
	// a host runs cannot be told apart reliably, so the line is refused on
	// every host.
	ReasonCronSetting = "cron_setting"
)

// scheduleSettings are the setting names some crons read as part of the
// schedule (ReasonCronSetting).
var scheduleSettings = map[string]bool{"CRON_TZ": true, "RANDOM_DELAY": true, "START_HOURS_RANGE": true}

// MaxCommand is cron 3.0pl1's MAX_COMMAND: load_entry reads the command
// with get_string(cmd, MAX_COMMAND, ...) and refuses the line (e_cmd_len)
// when the command fills the buffer, so a command of MaxCommand-1 bytes or
// more is never run by cron. MaxEnvLine is MAX_ENVSTR: load_env keeps only
// the first MaxEnvLine-1 bytes of a setting's line (the rest is dropped
// silently), so a longer setting line is refused here instead.
const (
	MaxCommand = 1000
	MaxEnvLine = 1000
)

// CommandFits reports whether cron runs a command of this length (the text
// after the schedule's fields and the blanks that follow them).
func CommandFits(command string) bool { return len(command) < MaxCommand-1 }

// isBlank is cron's field separator: an ASCII space or tab only
// (Skip_Blanks/Skip_Nonblanks in cron.h). Any other byte, a no-break space
// or another Unicode blank included, is part of a field.
func isBlank(b byte) bool { return b == ' ' || b == '\t' }

// SplitFields splits on ASCII spaces and tabs only, as cron does.
func SplitFields(text string) []string {
	var out []string
	start := -1
	for i := 0; i < len(text); i++ {
		if isBlank(text[i]) {
			if start >= 0 {
				out = append(out, text[start:i])
				start = -1
			}
			continue
		}
		if start < 0 {
			start = i
		}
	}
	if start >= 0 {
		out = append(out, text[start:])
	}
	return out
}

// Entry is one line of a crontab.
type Entry struct {
	Line int    // 1-based
	Raw  string // exact bytes
	Kind Kind
	// Env lines.
	Name, Value string
	// Job and disabled lines: the schedule as written, the command cron hands
	// to the shell and the standard input it writes (HasInput).
	Schedule string
	Command  string
	Input    string
	HasInput bool
	Spec     Spec
	// Text is the task text as the Scheduled tasks screen identifies it
	// (schedule and command as written, without "# DISABLED: ").
	Text string
	// Reason (KindInvalid) is a Reason* code; Detail says more in English.
	Reason, Detail string
}

// Spec is when a job runs, as timer settings.
type Spec struct {
	// Reboot: @reboot, OnBootSec=1min (the timer is enabled, not started:
	// it runs once after each boot, as cron runs @reboot lines when it starts).
	Reboot bool
	// Calendars are the OnCalendar= values (one, or two for cron's OR).
	Calendars []string
}

// Crontab is a parsed crontab.
type Crontab struct {
	Entries []Entry
	// Env are the environment settings in order (the last one of a name wins,
	// as in cron), MAILTO and SHELL included as written.
	Env []Entry
	// Shell is cron's shell for the jobs: SHELL= or /bin/sh.
	Shell string
}

var envNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// shellPathPattern: SHELL= must be a plain absolute path to be used as a
// timer's interpreter.
var shellPathPattern = regexp.MustCompile(`^/[A-Za-z0-9/_.+-]{1,200}$`)

// Parse reads a crontab's text line by line, the way cron 3.0pl1 reads it.
func Parse(text string) Crontab {
	c := Crontab{Shell: "/bin/sh"}
	lines := strings.Split(text, "\n")
	if strings.HasSuffix(text, "\n") {
		lines = lines[:len(lines)-1]
	}
	for i, raw := range lines {
		e := parseLine(i+1, raw)
		if e.Kind == KindEnv {
			switch e.Name {
			case "SHELL":
				if !shellPathPattern.MatchString(e.Value) {
					e.Kind, e.Reason, e.Detail = KindInvalid, ReasonShell, "SHELL must be a plain absolute path to be used for a timer"
				} else {
					c.Shell = e.Value
				}
			case "HOME":
				e.Kind, e.Reason, e.Detail = KindInvalid, ReasonHome, "HOME= changes the folder cron starts the jobs in; a timer starts them in the account's home"
			case "MAILTO":
				if e.Value != "" {
					e.Kind, e.Reason, e.Detail = KindInvalid, ReasonMailto, "MAILTO with an address: cron mails the output, a timer keeps it in the journal"
				}
			default:
				if scheduleSettings[e.Name] {
					e.Kind, e.Reason, e.Detail = KindInvalid, ReasonCronSetting, e.Name+" is part of the schedule for some crons (cronie); a timer would keep only the setting"
				}
			}
			if e.Kind == KindEnv {
				c.Env = append(c.Env, e)
			}
		}
		c.Entries = append(c.Entries, e)
	}
	return c
}

func parseLine(n int, raw string) Entry {
	e := Entry{Line: n, Raw: raw}
	if strings.ContainsAny(raw, "\r\x00\v\f") {
		e.Kind, e.Reason, e.Detail = KindInvalid, ReasonCarriage, "the line holds a carriage return or another control character"
		return e
	}
	// cron's skip_comments: leading spaces and tabs, then a newline (blank)
	// or "#" (a comment); nothing else counts as blank.
	trimmed := strings.TrimLeft(raw, " \t")
	if trimmed == "" {
		e.Kind = KindBlank
		return e
	}
	if strings.HasPrefix(trimmed, "#") {
		rest, disabled := strings.CutPrefix(trimmed, "# DISABLED:")
		if !disabled {
			e.Kind = KindComment
			return e
		}
		job := parseJob(n, raw, strings.Trim(rest, " \t"))
		if job.Kind == KindJob {
			job.Kind = KindDisabled
			return job
		}
		// A paused line that is not a job is only a comment for cron.
		e.Kind = KindComment
		return e
	}
	name, value, kind := envLine(trimmed)
	switch kind {
	case envUnsure:
		e.Kind, e.Reason, e.Detail = KindInvalid, ReasonInvalidEnv, "cron cannot read this setting reliably (it ends right after an opening quote)"
		return e
	case envSetting:
		if len(trimmed) >= MaxEnvLine {
			e.Kind, e.Reason, e.Detail = KindInvalid, ReasonTooLong, fmt.Sprintf("cron reads at most %d bytes of a setting's line", MaxEnvLine-1)
			return e
		}
		if !envNamePattern.MatchString(name) {
			e.Kind, e.Reason, e.Detail = KindInvalid, ReasonInvalidEnv, "not a name a systemd environment file takes"
			return e
		}
		e.Kind, e.Name, e.Value = KindEnv, name, value
		if !envValueRepresentable(value) {
			e.Kind, e.Reason, e.Detail = KindInvalid, ReasonEnvValue, "the value is not valid UTF-8 text, or holds a single quote, a backslash, a control character, U+FEFF or a noncharacter"
		}
		return e
	}
	// cron keeps the command's trailing blanks (get_string reads up to the
	// newline): nothing is trimmed on the right.
	return parseJob(n, raw, trimmed)
}

// What envLine found.
const (
	envNot     = iota // not a setting: cron reads the line as a job
	envSetting        // NAME=value
	envUnsure         // the line ends right after an opening quote: cron's
	// parser then reads past the line's end, so what it makes of it is not
	// known; the line is refused
)

// envLine is cron 3.0pl1's load_env (Debian's env.c), state by state:
// NAMEI (a quote may open the name), NAME (an unquoted name ends at a blank
// or "="; "=" inside a quoted name is an error), EQ1 (blanks, then "="),
// EQ2 (blanks after "="), VALUEI (a quote may open the value), VALUE, FINI
// (only blanks after a closing quote). The line is a setting when the parse
// ends in FINI, in an unquoted VALUE, or in EQ2 (an empty value: "MAILTO=").
// An unquoted value loses its trailing blanks; then every value loses its
// trailing blanks (strdtb) and one pair of matching outer quotes left in it.
// So `A="x` and `A="x"y` are not settings (cron reads them as jobs, which
// fail), `A=` is the empty value, and `A=" x "` is " x".
func envLine(line string) (string, string, int) {
	const (
		stNameI = iota
		stName
		stEq1
		stEq2
		stValueI
		stValue
		stFini
		stError
	)
	state := stNameI
	var quote byte
	var name, value []byte
	inValue := false
	c := 0
	for state != stError && c < len(line) {
		switch state {
		case stNameI, stValueI:
			if line[c] == '\'' || line[c] == '"' {
				quote = line[c]
				c++
			}
			state++
			if c >= len(line) {
				// C's loop goes on with the terminating NUL here.
				return "", "", envUnsure
			}
			fallthrough
		case stName, stValue:
			ch := line[c]
			if quote != 0 {
				if ch == quote {
					state++
					c++
					continue
				}
				if state == stName && ch == '=' {
					state = stError
					continue
				}
			} else if state == stName {
				if isBlank(ch) {
					c++
					state++
					continue
				}
				if ch == '=' {
					state++
					continue
				}
			}
			if inValue {
				value = append(value, ch)
			} else {
				name = append(name, ch)
			}
			c++
		case stEq1:
			if line[c] == '=' {
				state++
				inValue = true
				quote = 0
			} else if !isBlank(line[c]) {
				state = stError
			}
			c++
		case stEq2, stFini:
			if isBlank(line[c]) {
				c++
			} else {
				state++
			}
		}
	}
	if state != stFini && !(state == stValue && quote == 0) && !(state == stEq2 && quote == 0) {
		return "", "", envNot
	}
	v := string(value)
	if state == stValue {
		v = strings.TrimRight(v, " \t")
	}
	v = strings.TrimRight(v, " \t") // strdtb
	if len(v) >= 2 && (v[0] == '\'' || v[0] == '"') && v[len(v)-1] == v[0] {
		v = v[1 : len(v)-1]
	}
	// cron keeps only MaxEnvLine-1 bytes of the line (the rest is dropped
	// silently); parseLine refuses a longer setting line (too_long) instead.
	return string(name), v, envSetting
}

// envValueRepresentable: the value can be written to a systemd environment
// file in single quotes byte for byte, and systemd reads it: valid UTF-8
// (systemd.exec(5): the file "must be UTF-8 encoded"; one invalid value
// would fail the whole file and with it every task of the site), no byte
// order mark or noncharacter, no single quote, no backslash, no control
// character.
func envValueRepresentable(value string) bool {
	if !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		switch {
		case r < 0x20 || r == 0x7f || r == '\'' || r == '\\':
			return false
		case r == 0xfeff, r >= 0xfdd0 && r <= 0xfdef, r&0xfffe == 0xfffe:
			return false
		}
	}
	return true
}

var specialSchedules = map[string]string{
	"@yearly":   "0 0 1 1 *",
	"@annually": "0 0 1 1 *",
	"@monthly":  "0 0 1 * *",
	"@weekly":   "0 0 * * 0",
	"@daily":    "0 0 * * *",
	"@midnight": "0 0 * * *",
	"@hourly":   "0 * * * *",
}

func parseJob(n int, raw, text string) Entry {
	e := Entry{Line: n, Raw: raw}
	fields := SplitFields(text)
	if len(fields) == 0 {
		e.Kind = KindBlank
		return e
	}
	var schedule, rest string
	if strings.HasPrefix(fields[0], "@") {
		schedule = fields[0]
		rest = strings.TrimLeft(strings.TrimPrefix(strings.TrimLeft(text, " \t"), fields[0]), " \t")
	} else {
		if len(fields) < 6 {
			e.Kind, e.Reason, e.Detail = KindInvalid, ReasonEmpty, "a schedule needs five fields and a command"
			if len(fields) < 5 {
				e.Reason, e.Detail = ReasonSchedule, "not a crontab line"
			}
			return e
		}
		schedule = strings.Join(fields[:5], " ")
		rest = text
		for i := 0; i < 5; i++ {
			rest = strings.TrimLeft(rest, " \t")
			rest = rest[len(fields[i]):]
		}
		rest = strings.TrimLeft(rest, " \t")
	}
	if rest == "" {
		e.Kind, e.Reason, e.Detail = KindInvalid, ReasonEmpty, "a schedule without a command"
		return e
	}
	spec, err := Translate(schedule)
	if err != nil {
		e.Kind, e.Reason, e.Detail = KindInvalid, ReasonSchedule, err.Error()
		return e
	}
	if !CommandFits(rest) {
		e.Kind, e.Reason, e.Detail = KindInvalid, ReasonTooLong, fmt.Sprintf("cron runs a command of at most %d bytes; this one has %d", MaxCommand-2, len(rest))
		return e
	}
	e.Kind, e.Schedule, e.Spec = KindJob, schedule, spec
	e.Command, e.Input, e.HasInput = SplitCommand(rest)
	// The task's identity (the screen's ID) is its text without trailing
	// blanks, as the screen reads the line; the command keeps them.
	e.Text = strings.TrimRight(text, " \t")
	return e
}

// SplitCommand is cron's reading of the command part: the first "%" that is
// not "\%" ends the command; the rest is standard input with every further
// unescaped "%" a newline and a newline at the end; "\%" stands for "%" in
// both parts; every other backslash stays (cron 3.0pl1 do_command.c).
//
// The two loops follow cron's own (do_command.c, child_process): in the
// command a backslash escapes the next byte for this scan only and is
// dropped before "%"; in the input "%" is a newline, "\%" a "%", any other
// backslash is kept, and a newline is added unless the input ends with one.
// An empty input writes nothing (cron then gives the job an empty stdin, a
// timer /dev/null: the same end of input).
func SplitCommand(rest string) (command, input string, hasInput bool) {
	var cmd strings.Builder
	escaped := false
	tail := ""
	cut := false
	for i := 0; i < len(rest); i++ {
		ch := rest[i]
		if escaped {
			if ch == '%' {
				// "\%": the backslash already written goes, the "%" stays.
				s := cmd.String()
				cmd.Reset()
				cmd.WriteString(s[:len(s)-1])
			}
			cmd.WriteByte(ch)
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			cmd.WriteByte(ch)
			continue
		}
		if ch == '%' {
			tail, cut = rest[i+1:], true
			break
		}
		cmd.WriteByte(ch)
	}
	if !cut || tail == "" {
		return cmd.String(), "", false
	}
	var in strings.Builder
	escaped = false
	needNewline := false
	for i := 0; i < len(tail); i++ {
		ch := tail[i]
		if escaped {
			if ch != '%' {
				in.WriteByte('\\')
			}
		} else if ch == '%' {
			ch = '\n'
		}
		escaped = ch == '\\'
		if !escaped {
			in.WriteByte(ch)
		}
		needNewline = ch != '\n'
	}
	if escaped {
		in.WriteByte('\\')
	}
	if needNewline {
		in.WriteByte('\n')
	}
	return cmd.String(), in.String(), true
}

// field is one schedule field's range.
type field struct {
	name    string
	lo, hi  int
	names   []string // index = value, for month and weekday names
	nameOff int
}

var (
	fieldMinute = field{name: "minute", lo: 0, hi: 59}
	fieldHour   = field{name: "hour", lo: 0, hi: 23}
	fieldDOM    = field{name: "day of month", lo: 1, hi: 31}
	fieldMonth  = field{name: "month", lo: 1, hi: 12, names: []string{"jan", "feb", "mar", "apr", "may", "jun", "jul", "aug", "sep", "oct", "nov", "dec"}, nameOff: 1}
	fieldDOW    = field{name: "day of week", lo: 0, hi: 7, names: []string{"sun", "mon", "tue", "wed", "thu", "fri", "sat"}, nameOff: 0}
)

// value reads a number or (month, weekday) a three-letter name.
func (f field) value(s string) (int, error) {
	if s == "" {
		return 0, fmt.Errorf("%s: an empty value", f.name)
	}
	if s[0] >= '0' && s[0] <= '9' {
		n, err := strconv.Atoi(s)
		if err != nil || len(s) > 2 {
			return 0, fmt.Errorf("%s: %q is not a number cron takes", f.name, s)
		}
		if n < f.lo || n > f.hi {
			return 0, fmt.Errorf("%s: %d is outside %d-%d", f.name, n, f.lo, f.hi)
		}
		return n, nil
	}
	for i, name := range f.names {
		if strings.EqualFold(s, name) {
			return i + f.nameOff, nil
		}
	}
	return 0, fmt.Errorf("%s: %q is not a value cron takes", f.name, s)
}

// parse returns the field's values and whether it starts with "*" (cron's
// DOM_STAR/DOW_STAR flag, which decides the day fields' AND or OR).
func (f field) parse(text string) (map[int]bool, bool, error) {
	set := map[int]bool{}
	star := strings.HasPrefix(text, "*")
	for _, element := range strings.Split(text, ",") {
		rangeText, stepText, hasStep := strings.Cut(element, "/")
		step := 1
		if hasStep {
			n, err := strconv.Atoi(stepText)
			if err != nil || n < 1 || len(stepText) > 2 {
				return nil, false, fmt.Errorf("%s: %q is not a step cron takes", f.name, stepText)
			}
			step = n
		}
		var from, to int
		switch {
		case rangeText == "*":
			from, to = f.lo, f.hi
			if f.name == fieldDOW.name {
				to = 6 // "*" is Sunday to Saturday once
			}
		case strings.Contains(rangeText, "-"):
			a, b, _ := strings.Cut(rangeText, "-")
			var err error
			if from, err = f.value(a); err != nil {
				return nil, false, err
			}
			if to, err = f.value(b); err != nil {
				return nil, false, err
			}
			if from > to {
				return nil, false, fmt.Errorf("%s: the range %s runs backwards", f.name, rangeText)
			}
		default:
			if hasStep {
				return nil, false, fmt.Errorf("%s: a step needs a range or *", f.name)
			}
			v, err := f.value(rangeText)
			if err != nil {
				return nil, false, err
			}
			from, to = v, v
		}
		for v := from; v <= to; v += step {
			set[v] = true
		}
	}
	return set, star, nil
}

// Translate turns a cron schedule into timer settings.
func Translate(schedule string) (Spec, error) {
	if strings.HasPrefix(schedule, "@") {
		if schedule == "@reboot" {
			return Spec{Reboot: true}, nil
		}
		expanded, ok := specialSchedules[schedule]
		if !ok {
			return Spec{}, fmt.Errorf("%q is not a special schedule cron knows", schedule)
		}
		schedule = expanded
	}
	parts := SplitFields(schedule)
	if len(parts) != 5 {
		return Spec{}, errors.New("a schedule has five fields")
	}
	minutes, _, err := fieldMinute.parse(parts[0])
	if err != nil {
		return Spec{}, err
	}
	hours, _, err := fieldHour.parse(parts[1])
	if err != nil {
		return Spec{}, err
	}
	days, domStar, err := fieldDOM.parse(parts[2])
	if err != nil {
		return Spec{}, err
	}
	months, _, err := fieldMonth.parse(parts[3])
	if err != nil {
		return Spec{}, err
	}
	weekdays, dowStar, err := fieldDOW.parse(parts[4])
	if err != nil {
		return Spec{}, err
	}
	if weekdays[7] { // 7 is Sunday too
		weekdays[0] = true
		delete(weekdays, 7)
	}
	tail := "-" + numberList(months, 1, 12) + "-"
	clock := " " + numberList(hours, 0, 23) + ":" + numberList(minutes, 0, 59) + ":00"
	if domStar || dowStar {
		day := numberList(days, 1, 31)
		prefix := weekdayList(weekdays)
		if prefix != "" {
			prefix += " "
		}
		return Spec{Calendars: []string{prefix + "*" + tail + day + clock}}, nil
	}
	// Neither day field starts with "*": cron runs when EITHER matches.
	byDay := "*" + tail + numberList(days, 1, 31) + clock
	byWeekday := "*" + tail + "*" + clock
	if prefix := weekdayList(weekdays); prefix != "" {
		byWeekday = prefix + " " + byWeekday
	}
	if byDay == byWeekday { // both fields name every day
		return Spec{Calendars: []string{byDay}}, nil
	}
	return Spec{Calendars: []string{byDay, byWeekday}}, nil
}

// numberList renders a set as systemd's list: "*" when it is the whole
// range, otherwise values and "a..b" runs, two digits each.
func numberList(set map[int]bool, lo, hi int) string {
	full := true
	for v := lo; v <= hi; v++ {
		if !set[v] {
			full = false
			break
		}
	}
	if full {
		return "*"
	}
	var values []int
	for v := range set {
		values = append(values, v)
	}
	sort.Ints(values)
	var parts []string
	for i := 0; i < len(values); {
		j := i
		for j+1 < len(values) && values[j+1] == values[j]+1 {
			j++
		}
		if j-i >= 2 {
			parts = append(parts, fmt.Sprintf("%02d..%02d", values[i], values[j]))
		} else {
			for k := i; k <= j; k++ {
				parts = append(parts, fmt.Sprintf("%02d", values[k]))
			}
		}
		i = j + 1
	}
	return strings.Join(parts, ",")
}

var weekdayNames = []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}

// weekdayList renders a weekday set ("" when every day), Monday first as
// systemd orders the week.
func weekdayList(set map[int]bool) string {
	order := []int{1, 2, 3, 4, 5, 6, 0}
	var names []string
	for _, d := range order {
		if set[d] {
			names = append(names, weekdayNames[d])
		}
	}
	if len(names) == 7 {
		return ""
	}
	return strings.Join(names, ",")
}

// Jobs are the lines that become timers (enabled jobs), in order.
func (c Crontab) Jobs() []Entry {
	var out []Entry
	for _, e := range c.Entries {
		if e.Kind == KindJob {
			out = append(out, e)
		}
	}
	return out
}

// Untranslatable are the lines that keep a crontab in cron: every invalid
// line (an env line too, since it changes every job).
func (c Crontab) Untranslatable() []Entry {
	var out []Entry
	for _, e := range c.Entries {
		if e.Kind == KindInvalid {
			out = append(out, e)
		}
	}
	return out
}

// EnvFileBody is the systemd environment file of the crontab's settings, in
// order, each value in single quotes (Parse refused values that cannot be
// written so). cron hands every setting of the crontab to its jobs (SHELL
// and an empty MAILTO too), and they outrank the PAM environment
// (/etc/environment), which outranks cron's own PATH=/usr/bin:/bin and
// SHELL=/bin/sh; the unit keeps that order (UnitText).
func (c Crontab) EnvFileBody() string {
	var b strings.Builder
	for _, e := range c.Env {
		b.WriteString(e.Name + "='" + e.Value + "'\n")
	}
	return b.String()
}
