package main

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Notifications by recipient (alpha.128, G18): the email choice in the usage
// and alert mail paths, the one list, the person's preferences and the
// administrator's settings, on the suspension fixture (see
// account_notices_test.go).

func TestUsageNoticeMailFollowsThePersonsChoice(t *testing.T) {
	f := newUsageFixture(t)
	box := stubAlertMail(t, true)
	ctx := context.Background()
	f.exec(t, `INSERT INTO notification_preferences (user_id, mode) VALUES (9703, 'none'), (9701, 'none')`)
	f.exec(t, `INSERT INTO notification_preferences (user_id, mode, extra_email) VALUES (9702, 'all', 'desk@example.com')`)
	f.exec(t, `UPDATE subscriptions SET bandwidth_quota_mb = 1 WHERE id = 9802`)
	f.addTraffic(1, 9902, "2026-11-10", 2*mib)
	f.panel.runUsageRound(ctx)
	rows, err := f.panel.db.GetDB().Query(`SELECT user_id, mail_state, COALESCE(mail_detail, '') FROM usage_notices WHERE kind = ?`, noticeTraffic100)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	states := map[int]string{}
	for rows.Next() {
		var user int
		var state, detail string
		_ = rows.Scan(&user, &state, &detail)
		states[user] = state + ":" + detail
	}
	if states[9703] != "no_address:not_wanted" || states[9701] != "no_address:not_wanted" || states[9702] != "sent:" {
		t.Fatalf("usage mail states %v", states)
	}
	sort.Strings(box.to)
	if strings.Join(box.to, ",") != "desk@example.com,matrix-reseller@example.test" {
		t.Fatalf("sent to %v", box.to)
	}
}

func TestAlertMailFollowsTheAdministratorsChoice(t *testing.T) {
	p, db := metricsTestDB(t)
	box := stubAlertMail(t, true)
	seedAlertAdmin(t, db, "admin1", true)
	var id int
	if err := db.QueryRow(`SELECT id FROM users WHERE username = 'admin1'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO notification_preferences (user_id, mode) VALUES (?, 'none')`, id); err != nil {
		t.Fatal(err)
	}
	openTestAlert(t, db, "disk:/")
	p.sendPendingAlertMail(context.Background(), time.Now())
	var detail string
	_ = db.QueryRow(`SELECT mail_state || ':' || COALESCE(mail_detail, '') FROM monitoring_alerts WHERE key = 'disk:/'`).Scan(&detail)
	if detail != "suppressed:not_wanted" || box.sends != 0 {
		t.Fatalf("an administrator who chose nothing was emailed: %q, %d", detail, box.sends)
	}
	// The group address still gets alerts; "only what needs my action"
	// still gets them (an alert always does), with the extra address.
	if err := p.setSetting(context.Background(), "notify.extra.monitoring", "noc@example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE notification_preferences SET mode = 'action', extra_email = 'pager@example.com' WHERE user_id = ?`, id); err != nil {
		t.Fatal(err)
	}
	openTestAlert(t, db, "cpu")
	p.sendPendingAlertMail(context.Background(), time.Now())
	sort.Strings(box.to)
	if strings.Join(box.to, ",") != "admin1@example.com,noc@example.com,pager@example.com" {
		t.Fatalf("sent to %v", box.to)
	}
}

func notificationsFixture(t *testing.T) *usageFixture {
	t.Helper()
	f := newUsageFixture(t)
	mux := http.NewServeMux()
	mux.HandleFunc(notificationsPath, f.panel.handleNotifications)
	mux.HandleFunc(notificationsPath+"/", f.panel.handleNotifications)
	mux.HandleFunc(notificationPreferencesPath, f.panel.handleNotificationPreferences)
	mux.HandleFunc(notificationSettingsPath, f.panel.handleNotificationSettings)
	f.handler = f.panel.requireAuth(mux)
	return f
}

func TestNotificationsListIsTheCallersOwnWithWhoActsAndTheFix(t *testing.T) {
	f := notificationsFixture(t)
	ctx := context.Background()
	setEndsAt(t, f, 9802, f.now.Add(3*24*time.Hour))
	_ = f.panel.raiseExpiryNotices(ctx, f.now)
	f.exec(t, `UPDATE subscriptions SET bandwidth_quota_mb = 1 WHERE id = 9802`)
	f.addTraffic(1, 9902, "2026-11-10", 2*mib)
	f.panel.runUsageRound(ctx)
	read := func(user int) notificationsReport {
		rec := f.as(t, user, http.MethodGet, notificationsPath, "")
		wantStatus(t, rec, http.StatusOK, "")
		if rec.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("cache header %q", rec.Header().Get("Cache-Control"))
		}
		var report notificationsReport
		if err := json.Unmarshal(rec.Body.Bytes(), &report); err != nil {
			t.Fatal(err)
		}
		return report
	}
	customer := read(9703)
	if customer.Monitoring != nil {
		t.Fatal("a customer was shown the server's alerts")
	}
	var account, usage *notificationItem
	for i := range customer.Items {
		item := &customer.Items[i]
		switch item.Source {
		case "account":
			account = item
		case "usage":
			usage = item
		}
	}
	if account == nil || account.NeedsAction || account.FixPath != "/account" || account.Words == nil || !strings.Contains(account.Words.TextEN, "your provider (matrix-reseller)") {
		t.Fatalf("customer's account notice %+v", account)
	}
	if usage == nil || usage.Kind != noticeTraffic100 || usage.NeedsAction || usage.Usage == nil {
		t.Fatalf("customer's usage notice %+v", usage)
	}
	reseller := read(9702)
	acting := 0
	for _, item := range reseller.Items {
		if item.NeedsAction {
			acting++
			if item.FixPath != "/users?rights=matrix-customer" {
				t.Fatalf("reseller fix path %q", item.FixPath)
			}
		}
	}
	if acting < 2 {
		t.Fatalf("the reseller's actions are not marked: %+v", reseller.Items)
	}
	if admin := read(9701); admin.Monitoring == nil {
		t.Fatal("the administrator lacks the alerts line")
	}
	if outsider := read(9704); len(outsider.Items) != 0 {
		t.Fatalf("another account's notices leaked: %+v", outsider.Items)
	}
	// Seen: only by its recipient.
	seenPath := notificationsPath + "/" + strconv.Itoa(account.ID) + "/seen"
	wantStatus(t, f.as(t, 9704, http.MethodPost, seenPath, ""), http.StatusNotFound, "")
	wantStatus(t, f.as(t, 9703, http.MethodGet, seenPath, ""), http.StatusMethodNotAllowed, "")
	wantStatus(t, f.as(t, 9703, http.MethodPost, seenPath, ""), http.StatusOK, "")
	for _, item := range read(9703).Items {
		if item.Source == "account" {
			t.Fatalf("a seen notice is still listed: %+v", item)
		}
	}
}

func TestNotificationPreferencesAreValidatedAndThePersonsOwn(t *testing.T) {
	f := notificationsFixture(t)
	path := notificationPreferencesPath
	rec := f.as(t, 9703, http.MethodGet, path, "")
	wantStatus(t, rec, http.StatusOK, "")
	var view notificationPreferencesView
	_ = json.Unmarshal(rec.Body.Bytes(), &view)
	if view.Mode != notifyModeAll || view.ExtraEmail != "" || !view.OwnEmailSet {
		t.Fatalf("default view %+v", view)
	}
	for _, body := range []string{
		`{"mode":"sometimes","extra_email":""}`,
		`{"mode":"all","extra_email":"not an address"}`,
		`{"mode":"all","extra_email":"","other":1}`,
		`{"mode":"all"}`,
		`{"mode":"all","extra_email":""} {}`,
	} {
		wantStatus(t, f.as(t, 9703, http.MethodPut, path, body), http.StatusBadRequest, errCodeNotificationPrefsInvalid)
	}
	rec = f.as(t, 9703, http.MethodPut, path, `{"mode":"action","extra_email":" Desk@Example.COM "}`)
	wantStatus(t, rec, http.StatusOK, "")
	pref := notificationPreferenceFor(context.Background(), f.panel.db.GetDB(), 9703)
	if pref.Mode != notifyModeAction || pref.ExtraEmail != "Desk@example.com" {
		t.Fatalf("stored %+v", pref)
	}
	wantStatus(t, f.as(t, 9703, http.MethodPut, path, `{"mode":"none","extra_email":""}`), http.StatusOK, "")
	if pref := notificationPreferenceFor(context.Background(), f.panel.db.GetDB(), 9703); pref.Mode != notifyModeNone || pref.ExtraEmail != "" {
		t.Fatalf("cleared %+v", pref)
	}
	// "Log in as" reads but never changes where the person's mail goes.
	token, err := f.panel.sessions.CreateImpersonation(context.Background(), 9703, 9701)
	if err != nil {
		t.Fatal(err)
	}
	wantStatus(t, f.send(t, http.MethodGet, path, token, ""), http.StatusOK, "")
	wantStatus(t, f.send(t, http.MethodPut, path, token, `{"mode":"all","extra_email":"x@example.com"}`), http.StatusConflict, errCodeNotificationPrefsImpersonal)
	if pref := notificationPreferenceFor(context.Background(), f.panel.db.GetDB(), 9703); pref.Mode != notifyModeNone {
		t.Fatalf("an impersonated change was stored: %+v", pref)
	}
}

func TestNotificationSettingsAreTheAdministrators(t *testing.T) {
	f := notificationsFixture(t)
	path := notificationSettingsPath
	for _, user := range []int{9702, 9703} {
		if rec := f.as(t, user, http.MethodGet, path, ""); rec.Code == http.StatusOK {
			t.Fatalf("user %d read the server-wide settings", user)
		}
	}
	rec := f.as(t, 9701, http.MethodGet, path, "")
	wantStatus(t, rec, http.StatusOK, "")
	var view notificationSettingsView
	_ = json.Unmarshal(rec.Body.Bytes(), &view)
	if view.ExpiryDaysBefore != expiryDaysDefault || len(view.ExpiryChoices) != len(expiryDaysChoices) || len(view.Extra) != 3 {
		t.Fatalf("default settings %+v", view)
	}
	for _, body := range []string{
		`{"expiry_days_before":11,"extra":{"monitoring":"","account":"","security":""}}`,
		`{"expiry_days_before":7,"extra":{"monitoring":"","account":""}}`,
		`{"expiry_days_before":7,"extra":{"monitoring":"","account":"","security":"","billing":""}}`,
		`{"expiry_days_before":7,"extra":{"monitoring":"bad","account":"","security":""}}`,
	} {
		wantStatus(t, f.as(t, 9701, http.MethodPut, path, body), http.StatusBadRequest, errCodeNotificationSettingsInvalid)
	}
	rec = f.as(t, 9701, http.MethodPut, path, `{"expiry_days_before":14,"extra":{"monitoring":"noc@example.com","account":"","security":"sec@example.com"}}`)
	wantStatus(t, rec, http.StatusOK, "")
	ctx := context.Background()
	if f.panel.expiryDaysBefore(ctx) != 14 || f.panel.adminGroupExtraEmail(ctx, notifyGroupMonitoring) != "noc@example.com" ||
		f.panel.adminGroupExtraEmail(ctx, notifyGroupAccount) != "" || f.panel.adminGroupExtraEmail(ctx, notifyGroupSecurity) != "sec@example.com" {
		t.Fatalf("stored settings: %s", rec.Body.String())
	}
}
