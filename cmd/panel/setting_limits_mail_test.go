package main

import (
	"context"
	"testing"
)

// Integration (alpha.128): the mail keys of the limits model fold into every
// mail value the server enforces or a screen shows; the tighter one wins.
func TestMailKeysOfTheLimitsModelTightenTheMailValues(t *testing.T) {
	f := newOutgoingFixture(t)
	f.enable(t)
	ctx := context.Background()
	for _, key := range []string{settingMailboxSizeMB, settingMailOutgoingMailbox, settingMailOutgoingDomain, settingMailOutgoingSubscription} {
		if _, ok := settingKeyDefs[key]; !ok {
			t.Fatalf("%s is not a registered setting key", key)
		}
	}
	for _, stmt := range []string{
		// The mail row of plan 9651 allows 800 per mailbox and 2048 MB.
		`INSERT INTO mail_plan_limits (owner_kind, owner_id, mailbox_size_mb, outgoing_mailbox) VALUES ('plan', 9651, 2048, 800)`,
		// The plan form's ceilings are tighter for the mailbox values.
		`INSERT INTO setting_ceilings (owner_kind, owner_id, key, value) VALUES ('plan', 9651, 'mail.outgoing.mailbox', 300)`,
		`INSERT INTO setting_ceilings (owner_kind, owner_id, key, value) VALUES ('plan', 9651, 'mail.mailbox_size_mb', 1024)`,
		// The server's ceiling is tighter than the Mail server page's 750 per domain.
		`UPDATE setting_policy SET server_ceiling = 600 WHERE key = 'mail.outgoing.domain'`,
		// A looser ceiling than the Mail server page's 1000 changes nothing.
		`UPDATE setting_policy SET server_ceiling = 5000 WHERE key = 'mail.outgoing.subscription'`,
	} {
		if _, err := f.db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	if ceiling, err := f.p.mailboxSizeCeiling(ctx, 9801); err != nil || ceiling != 1024 {
		t.Fatalf("mailbox size ceiling = %d (%v)", ceiling, err)
	}
	view, err := f.p.outgoingDomainView(ctx, 9801, &Caller{ID: 9602, Role: roleCustomer})
	if err != nil {
		t.Fatal(err)
	}
	if view.Ceiling != 300 || view.Default != 300 || view.DomainLimit != 600 || view.SubscriptionLimit != 1000 {
		t.Fatalf("view = ceiling %d default %d domain %d subscription %d", view.Ceiling, view.Default, view.DomainLimit, view.SubscriptionLimit)
	}
	if _, _, err := f.p.applyOutgoingMail(ctx, true); err != nil || len(f.applied) == 0 {
		t.Fatalf("apply: %v (%d)", err, len(f.applied))
	}
	spec := f.applied[len(f.applied)-1]
	if spec.MailboxLimits["a@example.com"] != 300 || spec.DomainLimits["example.com"] != 600 {
		t.Fatalf("postfwd values = %+v", spec)
	}
	if len(spec.Subscriptions) != 1 || spec.Subscriptions[0].Limit != 1000 {
		t.Fatalf("hosting account value = %+v", spec.Subscriptions)
	}
	// Without chain ceilings the mail row applies as before.
	if row := foldMailChain(mailPlanLimitRow{}, outgoingSettings{MailboxPerHour: 500}, nil); row.OutgoingMailbox != nil || row.MailboxSizeMB != nil {
		t.Fatalf("an empty chain changed the row: %+v", row)
	}
}
