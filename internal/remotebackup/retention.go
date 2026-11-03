package remotebackup

import (
	"sort"
	"time"
)

// RetentionItem is one remote backup a schedule made.
type RetentionItem struct {
	Key     string
	Created time.Time // the backup's own creation time (UTC); zero = unknown
}

// MaxKeepDaily and MaxKeepWeekly bound a schedule's retention.
const (
	MaxKeepDaily  = 90
	MaxKeepWeekly = 104
)

// SelectRetention is a simple grandfather-father-son choice: keep the newest
// backup of each of the newest keepDaily calendar days (UTC) that have a
// backup, and the newest backup of each of the newest keepWeekly ISO weeks.
// Everything else is returned in remove.
//
// Safety rules: the newest backup is always kept; a backup whose time is
// unknown is always kept (an uncertain item is never deleted); keepDaily and
// keepWeekly both 0 behave as keepDaily 1.
//
// Basit GFS: en yeni keepDaily günün ve keepWeekly haftanın her birinin en
// yeni yedeği kalır; en yeni yedek ve zamanı bilinmeyen yedek hiç silinmez.
func SelectRetention(items []RetentionItem, keepDaily, keepWeekly int) (keep, remove []string) {
	if keepDaily < 0 {
		keepDaily = 0
	}
	if keepWeekly < 0 {
		keepWeekly = 0
	}
	if keepDaily == 0 && keepWeekly == 0 {
		keepDaily = 1
	}
	sorted := append([]RetentionItem(nil), items...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Created.Equal(sorted[j].Created) {
			return sorted[i].Key > sorted[j].Key
		}
		return sorted[i].Created.After(sorted[j].Created)
	})
	kept := map[string]bool{}
	days := map[string]bool{}
	weeks := map[string]bool{}
	newestKnown := ""
	for _, item := range sorted {
		if item.Created.IsZero() {
			kept[item.Key] = true
			continue
		}
		if newestKnown == "" {
			newestKnown = item.Key
			kept[item.Key] = true
		}
		t := item.Created.UTC()
		day := t.Format("2006-01-02")
		if !days[day] && len(days) < keepDaily {
			days[day] = true
			kept[item.Key] = true
		}
		year, week := t.ISOWeek()
		weekKey := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC).Format("2006") + "-" + twoDigits(week)
		if !weeks[weekKey] && len(weeks) < keepWeekly {
			weeks[weekKey] = true
			kept[item.Key] = true
		}
	}
	for _, item := range sorted {
		if kept[item.Key] {
			keep = append(keep, item.Key)
		} else {
			remove = append(remove, item.Key)
		}
	}
	return keep, remove
}

func twoDigits(n int) string {
	if n < 10 {
		return "0" + string(rune('0'+n))
	}
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}
