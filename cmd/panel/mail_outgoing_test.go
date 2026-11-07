package main

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

type outgoingFixture struct {
	p        *Panel
	db       *sql.DB
	applied  []transport.OutgoingMailLimitsSpec
	activity transport.OutgoingMailActivityResponse
	answer   transport.OutgoingMailLimitsApplyResponse
}

func newOutgoingFixture(t *testing.T) *outgoingFixture {
	t.Helper()
	database, err := openTemplatePanelDatabase(filepath.Join(t.TempDir(), "panel.sqlite"))
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(database.Close)
	f := &outgoingFixture{p: &Panel{db: database}, db: database.GetDB(),
		answer: transport.OutgoingMailLimitsApplyResponse{Applied: true, Verified: true}}
	savedApply, savedActivity := outgoingAgentApply, outgoingAgentActivity
	t.Cleanup(func() { outgoingAgentApply, outgoingAgentActivity = savedApply, savedActivity })
	outgoingAgentApply = func(_ *Panel, _ context.Context, spec *transport.OutgoingMailLimitsSpec) (transport.OutgoingMailLimitsApplyResponse, error) {
		f.applied = append(f.applied, *spec)
		return f.answer, nil
	}
	outgoingAgentActivity = func(_ *Panel, _ context.Context, _ time.Time) (transport.OutgoingMailActivityResponse, error) {
		return f.activity, nil
	}
	for _, stmt := range []string{
		`INSERT INTO users (id, username, password_hash, email, role) VALUES (9601, 'admin1', 'x', 'admin1@example.net', 'admin')`,
		`INSERT INTO users (id, username, password_hash, email, role) VALUES (9602, 'cust1', 'x', 'cust1@example.net', 'customer')`,
		`INSERT INTO service_plans (id, name) VALUES (9651, 'Small')`,
		`INSERT INTO subscriptions (id, owner_id, name, plan_id) VALUES (9701, 9602, 'cust1-sub', 9651)`,
		`INSERT INTO domains (id, subscription_id, name) VALUES (9801, 9701, 'example.com')`,
		`INSERT INTO domains (id, subscription_id, name) VALUES (9802, 9701, 'example.org')`,
		`INSERT INTO email_accounts (id, domain_id, address, password_hash, created_at) VALUES (9901, 9801, 'a@example.com', 'x', datetime('now', '-3 days'))`,
		`INSERT INTO email_accounts (id, domain_id, address, password_hash, created_at) VALUES (9902, 9801, 'new@example.com', 'x', datetime('now', '-1 hours'))`,
		`INSERT INTO email_accounts (id, domain_id, address, password_hash, created_at) VALUES (9903, 9802, 'b@example.org', 'x', datetime('now', '-30 days'))`,
	} {
		if _, err := f.db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	return f
}

func (f *outgoingFixture) enable(t *testing.T) {
	t.Helper()
	if _, err := f.db.Exec(`UPDATE mail_outgoing_settings SET enabled = 1 WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
}

func TestOutgoingSettingsSeedTheOwnersDefaults(t *testing.T) {
	f := newOutgoingFixture(t)
	settings, record, err := f.p.loadOutgoingSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if settings.Enabled || settings.MailboxPerHour != 500 || settings.DomainPerHour != 750 || settings.SubscriptionPerHour != 1000 ||
		settings.NewMailboxPerHour != 50 || settings.NewMailboxHours != 24 || record.State != "" {
		t.Fatalf("seed = %+v %+v", settings, record)
	}
	// Never turned on: nothing is sent to the server.
	if _, called, err := f.p.applyOutgoingMail(context.Background(), false); err != nil || called || len(f.applied) != 0 {
		t.Fatalf("an off state that was never on reached the Agent: %v %v", called, err)
	}
}

func TestOutgoingSpecFollowsPlanMailboxAndFirstDay(t *testing.T) {
	f := newOutgoingFixture(t)
	f.enable(t)
	ctx := context.Background()
	// The plan lets one mailbox choose up to 2000 and gives its domains 900.
	if _, err := f.db.Exec(`INSERT INTO mail_plan_limits (owner_kind, owner_id, outgoing_mailbox, outgoing_domain) VALUES ('plan', 9651, 2000, 900)`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`INSERT INTO mailbox_outgoing (email_account_id, per_hour) VALUES (9903, 0)`); err != nil {
		t.Fatal(err)
	}
	resp, called, err := f.p.applyOutgoingMail(ctx, false)
	if err != nil || !called || !resp.Applied {
		t.Fatalf("apply: %+v %v %v", resp, called, err)
	}
	spec := f.applied[0]
	if spec.MailboxLimits["a@example.com"] != 500 || spec.MailboxLimits["new@example.com"] != 50 {
		t.Fatalf("mailbox limits = %v", spec.MailboxLimits)
	}
	if _, limited := spec.MailboxLimits["b@example.org"]; limited {
		t.Fatalf("an unlimited mailbox got a limit: %v", spec.MailboxLimits)
	}
	if spec.DomainLimits["example.com"] != 900 || spec.DomainLimits["example.org"] != 900 {
		t.Fatalf("domain limits = %v", spec.DomainLimits)
	}
	if len(spec.Subscriptions) != 1 || spec.Subscriptions[0].Key != "s9701" || spec.Subscriptions[0].Limit != 1000 ||
		strings.Join(spec.Subscriptions[0].Domains, ",") != "example.com,example.org" {
		t.Fatalf("subscriptions = %+v", spec.Subscriptions)
	}
	// The same state is not sent twice.
	if _, called, _ := f.p.applyOutgoingMail(ctx, false); called {
		t.Fatal("an unchanged state was sent again")
	}
	_, record, _ := f.p.loadOutgoingSettings(ctx)
	if record.State != "applied" || record.VerifiedAt == "" {
		t.Fatalf("record = %+v", record)
	}
}

func TestOutgoingRefusalIsRecordedAndRetriedLater(t *testing.T) {
	f := newOutgoingFixture(t)
	f.enable(t)
	f.answer = transport.OutgoingMailLimitsApplyResponse{Code: transport.OutgoingMailNotInstalled, Error: "postfwd is not installed"}
	ctx := context.Background()
	if _, called, err := f.p.applyOutgoingMail(ctx, false); !called || err != nil {
		t.Fatalf("first apply: %v %v", called, err)
	}
	_, record, _ := f.p.loadOutgoingSettings(ctx)
	if record.State != "refused" || record.Code != transport.OutgoingMailNotInstalled || record.hash != "" {
		t.Fatalf("record = %+v", record)
	}
	// The worker does not ask again at once (retry after 30 minutes)...
	if _, called, _ := f.p.applyOutgoingMail(ctx, false); called {
		t.Fatal("a refused state was retried at once")
	}
	// ...but a save asks.
	if _, called, _ := f.p.applyOutgoingMail(ctx, true); !called {
		t.Fatal("a forced apply was not sent")
	}
}

func TestOutgoingSpikeDecision(t *testing.T) {
	s := outgoingSettings{SpikeFactor: 5, SpikeMinimum: 100, RefusalPause: 100}
	cases := []struct {
		name       string
		recipients int
		refusals   int
		history    []int
		observed   bool
		want       bool
	}{
		{"normal 5, now 120", 120, 0, []int{5, 4, 6, 0, 0}, false, true},
		{"normal 300, now 400", 400, 0, []int{300, 280, 320}, true, false},
		{"no history yet, busy hour", 300, 0, nil, false, false},
		{"silent for a week, busy hour", 300, 0, nil, true, true},
		{"below the minimum", 90, 0, []int{1}, true, false},
		{"hammering the limit", 10, 150, []int{5}, true, true},
	}
	for _, c := range cases {
		if got, _, _ := outgoingSpikeDecision(s, c.recipients, c.refusals, c.history, c.observed); got != c.want {
			t.Errorf("%s: pause %v, want %v", c.name, got, c.want)
		}
	}
}

func TestOutgoingWorkerPausesNoticesAndAPasswordResumes(t *testing.T) {
	f := newOutgoingFixture(t)
	f.enable(t)
	ctx := context.Background()
	now := time.Now().UTC()
	hour := now.Truncate(time.Hour)
	// A week of a quiet mailbox, so the panel has watched for more than a day.
	for i := 2; i < 40; i += 6 {
		if _, err := f.db.Exec(`INSERT INTO mail_outgoing_hourly (address, hour, messages, recipients) VALUES ('a@example.com', ?, 2, 3)`,
			hour.Add(-time.Duration(i)*time.Hour).Format(time.RFC3339)); err != nil {
			t.Fatal(err)
		}
	}
	f.activity = transport.OutgoingMailActivityResponse{Read: true, Buckets: []transport.OutgoingMailBucket{
		{Hour: hour.Format(time.RFC3339), Address: "a@example.com", Messages: 300, Recipients: 600},
		{Hour: hour.Format(time.RFC3339), Address: "b@example.org", Messages: 1, Recipients: 1, LimitRefusals: 2, LimitKind: "mailbox"},
	}}
	f.p.runOutgoingMailRound(ctx)

	var pausedAt, reason string
	if err := f.db.QueryRow(`SELECT COALESCE(paused_at, ''), COALESCE(paused_reason, '') FROM mailbox_outgoing WHERE email_account_id = 9901`).Scan(&pausedAt, &reason); err != nil || pausedAt == "" || reason != "spike" {
		t.Fatalf("mailbox not paused: %q %q %v", pausedAt, reason, err)
	}
	last := f.applied[len(f.applied)-1]
	if len(last.Paused) != 1 || last.Paused[0] != "a@example.com" {
		t.Fatalf("postfwd was not given the pause: %+v", last)
	}
	counts := map[string]int{}
	rows, _ := f.db.Query(`SELECT kind, audience FROM mail_outgoing_notices`)
	for rows.Next() {
		var kind, audience string
		_ = rows.Scan(&kind, &audience)
		counts[kind+"/"+audience]++
	}
	rows.Close()
	if counts["paused/customer"] != 1 || counts["paused/administrator"] != 1 || counts["limit_reached/customer"] != 1 {
		t.Fatalf("notices = %v", counts)
	}
	// A second round in the same hour adds no second notice and no second pause.
	f.p.runOutgoingMailRound(ctx)
	var total int
	_ = f.db.QueryRow(`SELECT COUNT(*) FROM mail_outgoing_notices`).Scan(&total)
	if total != counts["paused/customer"]+counts["paused/administrator"]+counts["limit_reached/customer"]+counts["limit_reached/administrator"] {
		t.Fatalf("notices were repeated: %d %v", total, counts)
	}

	// The password change ends the pause; the same hour is not judged again.
	resumed, err := f.p.resumeOutgoingAfterPassword(ctx, 9901)
	if err != nil || !resumed {
		t.Fatalf("resume: %v %v", resumed, err)
	}
	if last := f.applied[len(f.applied)-1]; len(last.Paused) != 0 {
		t.Fatalf("postfwd still holds the pause: %+v", last)
	}
	f.p.runOutgoingMailRound(ctx)
	_ = f.db.QueryRow(`SELECT COALESCE(paused_at, '') FROM mailbox_outgoing WHERE email_account_id = 9901`).Scan(&pausedAt)
	if pausedAt != "" {
		t.Fatal("the hour before the password change paused the mailbox again")
	}
}

func TestOutgoingDomainViewAndCeiling(t *testing.T) {
	f := newOutgoingFixture(t)
	f.enable(t)
	ctx := context.Background()
	if _, err := f.db.Exec(`INSERT INTO mail_plan_limits (owner_kind, owner_id, outgoing_mailbox) VALUES ('plan', 9651, 800)`); err != nil {
		t.Fatal(err)
	}
	customer := &Caller{ID: 9602, Role: roleCustomer}
	view, err := f.p.outgoingDomainView(ctx, 9801, customer)
	if err != nil {
		t.Fatal(err)
	}
	if view.State != "pending" || view.Ceiling != 800 || view.Default != 500 || view.MayExceedCeiling || len(view.Mailboxes) != 2 {
		t.Fatalf("view = %+v", view)
	}
	for _, m := range view.Mailboxes {
		if m.Address == "new@example.com" && (m.Effective.NewUntil == "" || m.Effective.Limit != 50) {
			t.Fatalf("first-day limit not shown: %+v", m.Effective)
		}
	}
	admin := &Caller{ID: 9601, Role: roleAdmin}
	if view, _ := f.p.outgoingDomainView(ctx, 9801, admin); !view.MayExceedCeiling {
		t.Fatal("the administrator may not go above the plan")
	}
}

func TestOutgoingNoticeMailNamesTheNextStep(t *testing.T) {
	subject, body := outgoingNoticeMail(outgoingNoticePaused, audienceCustomer, outgoingNoticeBody{Address: "a@example.com", Reason: "spike", Recipients: 600, Normal: 3}, "https://panel.example.net:1923")
	if !strings.Contains(subject, "a@example.com") || !strings.Contains(body, "change the mailbox password") || !strings.Contains(body, "parolasını değiştirin") {
		t.Fatalf("paused mail = %q\n%s", subject, body)
	}
	_, body = outgoingNoticeMail(outgoingNoticeLimit, audienceCustomer, outgoingNoticeBody{Address: "a@example.com", Limit: 500, Refused: 3, LimitKind: "domain"}, "")
	if !strings.Contains(body, "500 recipients per hour for all mailboxes of this domain together") || !strings.Contains(body, "nothing is lost") {
		t.Fatalf("limit mail = %s", body)
	}
}

func outgoingRequest(method, target, body string, caller *Caller) *http.Request {
	r := httptest.NewRequest(method, target, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	return r.WithContext(context.WithValue(r.Context(), callerKey, caller))
}

func TestMailPlanLimitsSaveAndMailboxSizeCeiling(t *testing.T) {
	f := newOutgoingFixture(t)
	admin := &Caller{ID: 9601, Role: roleAdmin}
	w := httptest.NewRecorder()
	f.p.handleMailPlanLimits(w, outgoingRequest(http.MethodPut, mailPlanLimitsPath,
		`{"owner_kind":"plan","owner_id":9651,"mailbox_size_mb":2048,"outgoing_mailbox":null,"outgoing_domain":900,"outgoing_subscription":null}`, admin))
	if w.Code != http.StatusOK {
		t.Fatalf("save: %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	f.p.handleMailPlanLimits(w, outgoingRequest(http.MethodGet, mailPlanLimitsPath, "", admin))
	if !strings.Contains(w.Body.String(), `"name":"Small"`) || !strings.Contains(w.Body.String(), `"mailbox_size_mb":2048`) {
		t.Fatalf("list: %s", w.Body.String())
	}
	if ceiling, err := f.p.mailboxSizeCeiling(context.Background(), 9801); err != nil || ceiling != 2048 {
		t.Fatalf("ceiling = %d %v", ceiling, err)
	}
	customer := &Caller{ID: 9602, Role: roleCustomer}
	w = httptest.NewRecorder()
	if !f.p.refuseMailboxAbovePlan(w, outgoingRequest(http.MethodPost, "/", "", customer), 9801, 4096) || w.Code != http.StatusForbidden ||
		!strings.Contains(w.Body.String(), errCodeMailboxSizeAbovePlan) {
		t.Fatalf("a customer went above the plan: %d %s", w.Code, w.Body.String())
	}
	if f.p.refuseMailboxAbovePlan(httptest.NewRecorder(), outgoingRequest(http.MethodPost, "/", "", customer), 9801, 2048) {
		t.Fatal("a size at the plan's limit was refused")
	}
	if f.p.refuseMailboxAbovePlan(httptest.NewRecorder(), outgoingRequest(http.MethodPost, "/", "", admin), 9801, 4096) {
		t.Fatal("the administrator was held to the plan")
	}
	// Empty values remove the row: the server's values apply again.
	w = httptest.NewRecorder()
	f.p.handleMailPlanLimits(w, outgoingRequest(http.MethodPut, mailPlanLimitsPath, `{"owner_kind":"plan","owner_id":9651}`, admin))
	var n int
	_ = f.db.QueryRow(`SELECT COUNT(*) FROM mail_plan_limits`).Scan(&n)
	if w.Code != http.StatusOK || n != 0 {
		t.Fatalf("clear: %d rows %d", w.Code, n)
	}
}

func TestDomainMailOutgoingSetStaysWithinTheCeiling(t *testing.T) {
	f := newOutgoingFixture(t)
	f.enable(t)
	customer := &Caller{ID: 9602, Role: roleCustomer}
	cases := []struct {
		body string
		code int
	}{
		{`{"id":9901,"mode":"custom","per_hour":600}`, http.StatusForbidden},
		{`{"id":9901,"mode":"unlimited","per_hour":0}`, http.StatusForbidden},
		{`{"id":9901,"mode":"custom","per_hour":200}`, http.StatusOK},
		{`{"id":9903,"mode":"custom","per_hour":200}`, http.StatusNotFound},
	}
	for _, c := range cases {
		w := httptest.NewRecorder()
		f.p.handleDomainMailOutgoing(w, outgoingRequest(http.MethodPut, "/api/v1/domains/9801/mail/outgoing", c.body, customer), 9801)
		if w.Code != c.code {
			t.Errorf("%s: %d %s", c.body, w.Code, w.Body.String())
		}
	}
	var perHour int
	if err := f.db.QueryRow(`SELECT per_hour FROM mailbox_outgoing WHERE email_account_id = 9901`).Scan(&perHour); err != nil || perHour != 200 {
		t.Fatalf("per_hour = %d %v", perHour, err)
	}
	if last := f.applied[len(f.applied)-1]; last.MailboxLimits["a@example.com"] != 200 {
		t.Fatalf("postfwd was not given the mailbox value: %v", last.MailboxLimits)
	}
}
