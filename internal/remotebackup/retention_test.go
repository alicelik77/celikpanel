package remotebackup

import (
	"sort"
	"testing"
	"time"
)

func TestRetentionKeepsDailyAndWeekly(t *testing.T) {
	base := time.Date(2026, 11, 3, 2, 0, 0, 0, time.UTC) // a Tuesday
	var items []RetentionItem
	// One backup a day for 30 days, plus a second one on the newest day.
	for d := 0; d < 30; d++ {
		ts := base.AddDate(0, 0, -d)
		items = append(items, RetentionItem{Key: ts.Format("2006-01-02"), Created: ts})
	}
	items = append(items, RetentionItem{Key: "newest-second", Created: base.Add(time.Hour)})
	keep, remove := SelectRetention(items, 3, 2)
	sort.Strings(keep)
	// Days: 3 newest days -> newest-second (3 Nov), 2 Nov, 1 Nov.
	// Weeks: ISO week of 3 Nov (newest-second) and the week before (newest
	// of it: Sunday 1 Nov), both already kept.
	want := []string{"2026-11-01", "2026-11-02", "newest-second"}
	if len(keep) != len(want) {
		t.Fatalf("keep %v, want %v", keep, want)
	}
	for i := range want {
		if keep[i] != want[i] {
			t.Fatalf("keep %v, want %v", keep, want)
		}
	}
	if len(remove) != len(items)-len(want) {
		t.Fatalf("remove %d", len(remove))
	}

	keep, _ = SelectRetention(items, 1, 4)
	sort.Strings(keep)
	// newest of each of the 4 newest weeks: 3 Nov (wk45), 1 Nov (wk44), 25 Oct (wk43), 18 Oct (wk42)
	want = []string{"2026-10-18", "2026-10-25", "2026-11-01", "newest-second"}
	for i := range want {
		if i >= len(keep) || keep[i] != want[i] {
			t.Fatalf("weekly keep %v, want %v", keep, want)
		}
	}
}

func TestRetentionSafetyRules(t *testing.T) {
	items := []RetentionItem{
		{Key: "unknown-time"},
		{Key: "old", Created: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)},
		{Key: "new", Created: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
	}
	keep, remove := SelectRetention(items, 0, 0)
	if len(remove) != 1 || remove[0] != "old" {
		t.Fatalf("remove %v keep %v", remove, keep)
	}
	keep, remove = SelectRetention(items[:2], 0, 0)
	if len(remove) != 0 || len(keep) != 2 {
		t.Fatalf("the newest known backup must stay: keep %v remove %v", keep, remove)
	}
}

func TestLayoutKeysRoundTrip(t *testing.T) {
	name := "full-20261103T020000.123456789Z-0123456789abcdef.cpbak"
	key, err := ObjectKey(ServerTag("Panel.Example.COM"), "Shop.Example", 7, GroupForPolicy(3), name)
	if err != nil {
		t.Fatal(err)
	}
	if key != "panel.example.com/sites/shop.example-d7/schedule-3/"+name+".gpg" {
		t.Fatalf("key %s", key)
	}
	parsed, err := ParseObjectKey(key)
	if err != nil || parsed.DomainID != 7 || parsed.PolicyID != 3 || parsed.BackupName != name || parsed.Domain != "shop.example" {
		t.Fatalf("parsed %+v %v", parsed, err)
	}
	if ts, ok := BackupCreatedAt(name); !ok || !ts.Equal(time.Date(2026, 11, 3, 2, 0, 0, 123456789, time.UTC)) {
		t.Fatalf("created %v %v", ts, ok)
	}
	for _, bad := range []string{
		"x/sites/a-d1/manual/../" + name + ".gpg",
		"x/sites/a-d0/manual/" + name + ".gpg",
		"x/sites/a-d1/schedule-0/" + name + ".gpg",
		"x/sites/a-d1/manual/evil.gpg",
		"x/other/a-d1/manual/" + name + ".gpg",
	} {
		if _, err := ParseObjectKey(bad); err == nil {
			t.Errorf("accepted %s", bad)
		}
	}
	if _, err := ObjectKey("srv", "a.example", 1, "manual", "../../etc/passwd"); err == nil {
		t.Fatal("bad backup name accepted")
	}
}
