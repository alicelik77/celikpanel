package main

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Limits of scheduled tasks (alpha.128, gap G5, docs/SITE-LIMITS.md): how
// many tasks a site may have (cron.max_jobs) and the fewest minutes between
// two runs of a task (cron.min_interval_minutes, a floor), both from the
// rights chain. They are checked when a task is added or its schedule is
// changed; the crontab lines stay plain, and tasks that already run keep
// running (P5). The administrator is not held by them.
//
// Zamanlanmış görev sınırları: sayı ve iki çalışma arası en az dakika.
// Görev eklenirken ya da zamanı değişirken denetlenir; çalışan görev sürer.

// cronField expands one crontab field (minute or hour) into its values.
// ok is false for a form this check does not read (the Agent still checks).
func cronField(field string, max int) ([]int, bool) {
	seen := map[int]bool{}
	for _, part := range strings.Split(field, ",") {
		step := 1
		if base, s, found := strings.Cut(part, "/"); found {
			n, err := strconv.Atoi(s)
			if err != nil || n <= 0 {
				return nil, false
			}
			step, part = n, base
		}
		lo, hi := 0, max
		switch {
		case part == "*":
		case strings.Contains(part, "-"):
			a, b, _ := strings.Cut(part, "-")
			x, err1 := strconv.Atoi(a)
			y, err2 := strconv.Atoi(b)
			if err1 != nil || err2 != nil || x < 0 || y > max || x > y {
				return nil, false
			}
			lo, hi = x, y
		default:
			n, err := strconv.Atoi(part)
			if err != nil || n < 0 || n > max {
				return nil, false
			}
			lo, hi = n, n
			if step > 1 {
				hi = max
			}
		}
		for v := lo; v <= hi; v += step {
			seen[v] = true
		}
	}
	out := make([]int, 0, len(seen))
	for v := range seen {
		out = append(out, v)
	}
	sort.Ints(out)
	return out, len(out) > 0
}

// cronMinIntervalMinutes is the smallest gap, in minutes, between two runs
// of a schedule within a day (the day, month and weekday fields only make a
// task run less often). ok is false when the schedule is not read here.
func cronMinIntervalMinutes(schedule string) (int, bool) {
	schedule = strings.TrimSpace(schedule)
	switch schedule {
	case "@reboot":
		return 24 * 60, true
	case "@hourly":
		return 60, true
	case "@daily", "@midnight", "@weekly", "@monthly", "@yearly", "@annually":
		return 24 * 60, true
	}
	fields := strings.Fields(schedule)
	if len(fields) != 5 {
		return 0, false
	}
	minutes, ok := cronField(fields[0], 59)
	if !ok {
		return 0, false
	}
	hours, ok := cronField(fields[1], 23)
	if !ok {
		return 0, false
	}
	times := make([]int, 0, len(minutes)*len(hours))
	for _, h := range hours {
		for _, m := range minutes {
			times = append(times, h*60+m)
		}
	}
	if len(times) == 1 {
		return 24 * 60, true
	}
	smallest := 24*60 - times[len(times)-1] + times[0]
	for i := 1; i < len(times); i++ {
		if gap := times[i] - times[i-1]; gap < smallest {
			smallest = gap
		}
	}
	return smallest, true
}

// refuseCronLimits writes the refusal for a task the site's limits do not
// allow and reports whether it did. existing is the site's task count
// (adding: one more is asked).
func (p *Panel) refuseCronLimits(ctx context.Context, w http.ResponseWriter, r *http.Request, tenant transport.CronTenant, schedule string, adding bool, existing int) bool {
	if callerIsAdministrator(r) {
		return false
	}
	limits, err := subscriptionSettingLimits(ctx, p.db.GetDB(), tenant.SubscriptionID)
	if err != nil {
		writeServerError(w, err)
		return true
	}
	refusals := []settingRefusal{}
	if limit := limits[settingCronMinIntervalMin]; limit.Ceiling != nil {
		if gap, ok := cronMinIntervalMinutes(schedule); ok && gap < *limit.Ceiling {
			refusals = append(refusals, settingRefusal{Key: settingCronMinIntervalMin, Value: gap, Limit: *limit.Ceiling, SetBy: limit.SetBy, SetByName: limit.SetByName})
		}
	}
	if limit := limits[settingCronJobs]; adding && limit.Ceiling != nil && existing+1 > *limit.Ceiling {
		refusals = append(refusals, settingRefusal{Key: settingCronJobs, Value: existing + 1, Limit: *limit.Ceiling, SetBy: limit.SetBy, SetByName: limit.SetByName})
	}
	if len(refusals) == 0 {
		return false
	}
	writeSettingAboveLimit(w, refusals)
	return true
}
