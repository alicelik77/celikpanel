package main

import (
	"context"
	"sort"
	"strings"
	"testing"
	"time"
)

// Account notices, subscription expiry and notifications by recipient
// (alpha.128, G11 + G18), on the suspension fixture: admin 9701, reseller
// 9702 (subscription 9801), its customer 9703 (subscription 9802) and the
// administrator's own customer 9704 (subscription 9803).

type noticeRow struct {
	user        int
	audience    string
	kind        string
	needsAction bool
	fixPath     string
	period      string
	seen        bool
	mail        string
}

func accountNoticeRows(t *testing.T, f *usageFixture, subjectKey string) []noticeRow {
	t.Helper()
	rows, err := f.panel.db.GetDB().Query(`SELECT user_id, audience, kind, needs_action, fix_path, period, seen_at IS NOT NULL, mail_state
		FROM account_notices WHERE subject_key = ? ORDER BY id`, subjectKey)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []noticeRow
	for rows.Next() {
		var r noticeRow
		if err := rows.Scan(&r.user, &r.audience, &r.kind, &r.needsAction, &r.fixPath, &r.period, &r.seen, &r.mail); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	return out
}

func setEndsAt(t *testing.T, f *usageFixture, sub int, at time.Time) string {
	t.Helper()
	value := at.UTC().Format(endsAtLayout)
	f.exec(t, `UPDATE subscriptions SET ends_at = ? WHERE id = ?`, value, sub)
	return value
}

func TestExpiryStageWindows(t *testing.T) {
	now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		left time.Duration
		days int
		want string
	}{
		{-time.Minute, 10, ""},
		{0, 10, ""},
		{time.Hour, 10, noticeExpiryToday},
		{24 * time.Hour, 10, noticeExpiryToday},
		{25 * time.Hour, 10, noticeExpirySoon},
		{10 * 24 * time.Hour, 10, noticeExpirySoon},
		{10*24*time.Hour + time.Minute, 10, ""},
		{5 * 24 * time.Hour, 3, ""},
	} {
		if got := expiryStage(now.Add(c.left), now, c.days); got != c.want {
			t.Fatalf("left %v days %d: %q, want %q", c.left, c.days, got, c.want)
		}
	}
}

func TestExpiryNoticesReachOwnerResellerAndAdministratorOncePerStage(t *testing.T) {
	f := newUsageFixture(t)
	ctx := context.Background()
	period := setEndsAt(t, f, 9802, f.now.Add(5*24*time.Hour))
	for i := 0; i < 3; i++ {
		if err := f.panel.raiseExpiryNotices(ctx, f.now); err != nil {
			t.Fatal(err)
		}
	}
	got := accountNoticeRows(t, f, "subscription:9802")
	want := []noticeRow{
		{9703, audienceCustomer, noticeExpirySoon, false, "/account", period, false, "pending"},
		{9702, audienceReseller, noticeExpirySoon, true, "/users?rights=matrix-customer", period, false, "pending"},
		{9701, audienceAdministrator, noticeExpirySoon, false, "/users?rights=matrix-customer", period, false, "pending"},
	}
	if len(got) != len(want) {
		t.Fatalf("rows %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("row %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	// The last day: one more stage each, never the first one again.
	f.now = f.now.Add(4*24*time.Hour + 12*time.Hour)
	_ = f.panel.raiseExpiryNotices(ctx, f.now)
	_ = f.panel.raiseExpiryNotices(ctx, f.now)
	got = accountNoticeRows(t, f, "subscription:9802")
	if len(got) != 6 || got[3].kind != noticeExpiryToday || got[5].kind != noticeExpiryToday {
		t.Fatalf("last day rows %+v", got)
	}
	// The end date moves later: the old notices no longer hold (seen); a
	// date inside the window again gives fresh stages for the new date.
	later := setEndsAt(t, f, 9802, f.now.Add(20*24*time.Hour))
	_ = f.panel.raiseExpiryNotices(ctx, f.now)
	got = accountNoticeRows(t, f, "subscription:9802")
	for _, r := range got {
		if !r.seen {
			t.Fatalf("a notice of the old end date is still shown: %+v", r)
		}
	}
	if len(got) != 6 {
		t.Fatalf("a date outside the window raised a notice: %+v", got)
	}
	f.now = f.now.Add(12 * 24 * time.Hour)
	_ = f.panel.raiseExpiryNotices(ctx, f.now)
	got = accountNoticeRows(t, f, "subscription:9802")
	if len(got) != 9 || got[6].period != later || got[6].kind != noticeExpirySoon || got[6].seen {
		t.Fatalf("new end date rows %+v", got)
	}

	// The administrator's own customer: the administrator acts.
	setEndsAt(t, f, 9803, f.now.Add(2*24*time.Hour))
	_ = f.panel.raiseExpiryNotices(ctx, f.now)
	direct := accountNoticeRows(t, f, "subscription:9803")
	if len(direct) != 2 || direct[0].user != 9704 || direct[0].needsAction || direct[1].user != 9701 || !direct[1].needsAction {
		t.Fatalf("direct customer rows %+v", direct)
	}
}

func TestExpiryWordsSayWhatHappensWhoActsAndHowItResumes(t *testing.T) {
	body := map[string]any{"subscription": "shop", "owner": "ayse", "reseller": "acme", "ends_at": "2027-01-31 00:00:00"}
	customer := accountNoticeWordsFor(noticeExpirySoon, audienceCustomer, body)
	for _, want := range []string{"2027-01-31 00:00 UTC", "Nothing is deleted", "your provider (acme)", "moved later"} {
		if !strings.Contains(customer.TextEN, want) {
			t.Fatalf("customer text lacks %q: %s", want, customer.TextEN)
		}
	}
	for _, want := range []string{"Hiçbir şey silinmez", "sağlayıcınız (acme)"} {
		if !strings.Contains(customer.TextTR, want) {
			t.Fatalf("customer Turkish lacks %q: %s", want, customer.TextTR)
		}
	}
	reseller := accountNoticeWordsFor(noticeExpiryToday, audienceReseller, body)
	if !strings.Contains(reseller.TextEN, "Who acts: you") || !strings.Contains(reseller.TitleEN, "within a day") || !strings.Contains(reseller.TitleEN, "(ayse)") {
		t.Fatalf("reseller words %+v", reseller)
	}
	admin := accountNoticeWordsFor(noticeExpirySoon, audienceAdministrator, body)
	if !strings.HasPrefix(admin.TextEN, "For your information") {
		t.Fatalf("administrator words %+v", admin)
	}
	ownReseller := accountNoticeWordsFor(noticeExpirySoon, audienceReseller, map[string]any{"subscription": "pool", "owner": "acme", "ends_at": "2027-01-31 00:00:00", "owner_is_reseller": true})
	if !strings.Contains(ownReseller.TextEN, "the administrator of this server") || !strings.Contains(ownReseller.TextEN, "customers are suspended too") {
		t.Fatalf("reseller's own subscription words %+v", ownReseller)
	}
	unknown := accountNoticeWordsFor("some_future_kind", audienceCustomer, nil)
	if unknown.TitleEN == "" || unknown.TitleTR == "" || unknown.TextEN == "" {
		t.Fatalf("no safe default: %+v", unknown)
	}
}

func TestRaiseAccountNoticeKeepsBoundsAndLocalFixPaths(t *testing.T) {
	f := newUsageFixture(t)
	ctx := context.Background()
	db := f.panel.db.GetDB()
	recipients := []usageRecipient{{9703, audienceCustomer}, {0, audienceCustomer}, {9702, "weird"}}
	for _, path := range []string{"https://evil.example/x", "//evil.example", "/\\evil", "/domains"} {
		f.panel.raiseAccountNotice(ctx, db, recipients, "mail_limit_near", true, "mailbox:"+path, "2026-11", map[string]any{"n": 1}, path)
	}
	rows, err := db.Query(`SELECT user_id, audience, fix_path FROM account_notices WHERE kind = 'mail_limit_near' ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var paths []string
	for rows.Next() {
		var user int
		var audience, path string
		_ = rows.Scan(&user, &audience, &path)
		if user == 9702 && audience != audienceCustomer {
			t.Fatalf("an unknown audience was stored: %q", audience)
		}
		paths = append(paths, path)
	}
	want := []string{"", "", "", "", "", "", "/domains", "/domains"}
	sort.Strings(paths)
	if strings.Join(paths, ",") != strings.Join(want, ",") {
		t.Fatalf("fix paths %q", paths)
	}
	// Too long a period is refused, not truncated into another period.
	f.panel.raiseAccountNotice(ctx, db, recipients, "mail_limit_near", true, "x", strings.Repeat("9", 65), nil, "")
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM account_notices WHERE subject_key = 'x'`).Scan(&n)
	if n != 0 {
		t.Fatalf("an out-of-bounds period was stored")
	}
}

func TestAccountNoticeMailFollowsEachPersonsChoiceAndTheGroupAddress(t *testing.T) {
	f := newUsageFixture(t)
	box := stubAlertMail(t, true)
	ctx := context.Background()
	f.exec(t, `INSERT INTO notification_preferences (user_id, mode) VALUES (9703, 'none')`)
	f.exec(t, `INSERT INTO notification_preferences (user_id, mode, extra_email) VALUES (9702, 'action', 'desk@example.com')`)
	f.exec(t, `INSERT INTO notification_preferences (user_id, mode) VALUES (9701, 'action')`)
	if err := f.panel.setSetting(ctx, "notify.extra.account", "ops@example.com"); err != nil {
		t.Fatal(err)
	}
	setEndsAt(t, f, 9802, f.now.Add(3*24*time.Hour))
	if err := f.panel.raiseExpiryNotices(ctx, f.now); err != nil {
		t.Fatal(err)
	}
	if err := f.panel.sendPendingAccountNotices(ctx); err != nil {
		t.Fatal(err)
	}
	got := accountNoticeRows(t, f, "subscription:9802")
	states := map[int]string{}
	for _, r := range got {
		states[r.user] = r.mail
	}
	// The customer chose nothing; the administrator only what needs them
	// (here the reseller acts); the reseller is emailed at both addresses.
	if states[9703] != "not_wanted" || states[9701] != "not_wanted" || states[9702] != "sent" {
		t.Fatalf("mail states %v", states)
	}
	sort.Strings(box.to)
	if strings.Join(box.to, ",") != "desk@example.com,matrix-reseller@example.test,ops@example.com" {
		t.Fatalf("sent to %v", box.to)
	}
	for _, body := range box.bodies {
		if !strings.Contains(body, "Nothing is deleted") || !strings.Contains(body, "Hiçbir şey silinmez") {
			t.Fatalf("mail without both languages:\n%s", body)
		}
	}
	// Another round sends nothing again.
	before := box.sends
	_ = f.panel.raiseExpiryNotices(ctx, f.now)
	_ = f.panel.sendPendingAccountNotices(ctx)
	if box.sends != before {
		t.Fatalf("a notice was mailed twice")
	}
}
