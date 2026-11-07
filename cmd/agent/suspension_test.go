package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

func TestSuspensionDecideTable(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		current, target, from string
		want                  suspensionStep
	}{
		{"suspend: current is the before-image", "", "1", "", suspensionChange},
		{"suspend: already suspended", "1", "1", "", suspensionAlready},
		{"suspend: changed by someone else", "20000", "1", "", suspensionConflict},
		{"restore: current is suspended", "1", "", "1", suspensionChange},
		{"restore: already restored", "", "", "1", suspensionAlready},
		{"restore: changed by someone else", "20000", "", "1", suspensionConflict},
		{"before-image equals the suspended value", "1", "1", "1", suspensionAlready},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := suspensionDecide(tc.current, tc.target, tc.from); got != tc.want {
				t.Fatalf("suspensionDecide(%q, %q, %q) = %d, want %d", tc.current, tc.target, tc.from, got, tc.want)
			}
		})
	}

	open := transport.DatabaseUserState{Present: true}
	locked := transport.DatabaseUserState{Present: true, Locked: true}
	for _, tc := range []struct {
		name                  string
		current, target, from transport.DatabaseUserState
		want                  suspensionStep
	}{
		{"db suspend from open", open, locked, open, suspensionChange},
		{"db suspend already locked", locked, locked, open, suspensionAlready},
		{"db restore to open", locked, open, locked, suspensionChange},
		{"db restore already open", open, open, locked, suspensionAlready},
		{"db restore owner-locked account stays locked", locked, locked, locked, suspensionAlready},
		{"db restore owner-locked but now open", open, locked, locked, suspensionConflict},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := suspensionDecide(tc.current, tc.target, tc.from); got != tc.want {
				t.Fatalf("got %d, want %d", got, tc.want)
			}
		})
	}
}

const suspensionShadowHash = "$6$secretsalt$SECRETHASHVALUE"

func suspensionShadowFixture(expiry string) string {
	return "root:" + suspensionShadowHash + ":19000:0:99999:7:::\n" +
		"site_example_com:" + suspensionShadowHash + ":19500:0:99999:7::" + expiry + ":\n" +
		"other:!:19500::::::\n"
}

func TestShadowExpiryFor(t *testing.T) {
	for _, expiry := range []string{"", "1", "20000"} {
		got, found, err := shadowExpiryFor([]byte(suspensionShadowFixture(expiry)), "site_example_com")
		if err != nil || !found || got != expiry {
			t.Fatalf("expiry %q: got %q found=%v err=%v", expiry, got, found, err)
		}
	}
	if _, found, err := shadowExpiryFor([]byte(suspensionShadowFixture("")), "absent_user"); err != nil || found {
		t.Fatalf("absent user: found=%v err=%v", found, err)
	}
	// A prefix of another name is not that name.
	if _, found, _ := shadowExpiryFor([]byte(suspensionShadowFixture("")), "site_example"); found {
		t.Fatal("a name prefix matched")
	}
	for name, content := range map[string]string{
		"duplicate":    suspensionShadowFixture("") + "site_example_com:" + suspensionShadowHash + ":1:0:99999:7:::\n",
		"short":        "site_example_com:" + suspensionShadowHash + ":19500:0\n",
		"not a number": "site_example_com:" + suspensionShadowHash + ":19500:0:99999:7::never:\n",
	} {
		_, _, err := shadowExpiryFor([]byte(content), "site_example_com")
		if err == nil {
			t.Fatalf("%s: unexpectedly accepted", name)
		}
		if strings.Contains(err.Error(), suspensionShadowHash) || strings.Contains(err.Error(), "secretsalt") {
			t.Fatalf("%s: error discloses the password hash: %v", name, err)
		}
	}
}

func TestChageExpiryArgs(t *testing.T) {
	for _, tc := range []struct {
		expiry string
		want   []string
	}{
		{"1", []string{"-E", "1", "site_example_com"}},
		{"", []string{"-E", "-1", "site_example_com"}},
		{"20000", []string{"-E", "20000", "site_example_com"}},
	} {
		if got := chageExpiryArgs("site_example_com", tc.expiry); !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("chageExpiryArgs(%q) = %v, want %v", tc.expiry, got, tc.want)
		}
	}
}

// siteSuspensionHarness points the shadow path at a fixture and replaces the
// account proof and chage with fakes that edit the fixture.
type siteSuspensionHarness struct {
	shadowPath string
	calls      [][]string
	chageErr   error
}

func newSiteSuspensionHarness(t *testing.T, expiry string, present bool) *siteSuspensionHarness {
	t.Helper()
	harness := &siteSuspensionHarness{shadowPath: filepath.Join(t.TempDir(), "shadow")}
	if err := os.WriteFile(harness.shadowPath, []byte(suspensionShadowFixture(expiry)), 0o600); err != nil {
		t.Fatal(err)
	}
	oldPath, oldUser, oldChage := suspensionShadowPath, suspensionSiteUser, suspensionRunChage
	suspensionShadowPath = harness.shadowPath
	suspensionSiteUser = func(account transport.SuspensionSiteAccount) (string, error) {
		if !present {
			return "", fmt.Errorf("look up cron user %q: %w", "site_example_com", user.UnknownUserError("site_example_com"))
		}
		return "site_example_com", nil
	}
	suspensionRunChage = func(_ context.Context, args ...string) ([]byte, error) {
		harness.calls = append(harness.calls, append([]string(nil), args...))
		if harness.chageErr != nil {
			return []byte("chage: cannot lock /etc/shadow; try again later."), harness.chageErr
		}
		next := args[1]
		if next == "-1" {
			next = ""
		}
		if err := os.WriteFile(harness.shadowPath, []byte(suspensionShadowFixture(next)), 0o600); err != nil {
			t.Fatal(err)
		}
		return nil, nil
	}
	t.Cleanup(func() {
		suspensionShadowPath, suspensionSiteUser, suspensionRunChage = oldPath, oldUser, oldChage
	})
	return harness
}

func setSiteSuspension(t *testing.T, suspend bool, before transport.SiteAccountState) transport.SetSiteAccountSuspensionResponse {
	t.Helper()
	var response transport.SetSiteAccountSuspensionResponse
	request := transport.SetSiteAccountSuspensionRequest{
		Account: transport.SuspensionSiteAccount{SubscriptionID: 3, DomainID: 7, Domain: "example.com"},
		Suspend: suspend,
		Before:  before,
	}
	if err := (&Agent{}).SetSiteAccountSuspensionV1(&request, &response); err != nil {
		t.Fatalf("SetSiteAccountSuspensionV1: %v", err)
	}
	if strings.Contains(response.Error, "SECRETHASH") {
		t.Fatalf("response discloses the password hash: %q", response.Error)
	}
	return response
}

func TestSiteAccountSuspensionRoundTripIsIdempotent(t *testing.T) {
	for _, original := range []string{"", "20000"} {
		t.Run("before="+original, func(t *testing.T) {
			harness := newSiteSuspensionHarness(t, original, true)
			var inspect transport.SuspensionInspectResponse
			_ = (&Agent{}).SuspensionInspectV1(&transport.SuspensionInspectRequest{
				SiteAccounts: []transport.SuspensionSiteAccount{{SubscriptionID: 3, DomainID: 7, Domain: "example.com"}},
			}, &inspect)
			before := inspect.SiteAccounts[0].State
			if inspect.SiteAccounts[0].Error != "" || before != (transport.SiteAccountState{Present: true, Expiry: original}) {
				t.Fatalf("inspect = %+v", inspect.SiteAccounts[0])
			}

			first := setSiteSuspension(t, true, before)
			if !first.Applied || !first.Changed || first.Conflict || first.Error != "" || first.State.Expiry != "1" {
				t.Fatalf("suspend = %+v", first)
			}
			second := setSiteSuspension(t, true, before)
			if !second.Applied || second.Changed || second.State.Expiry != "1" {
				t.Fatalf("second suspend = %+v", second)
			}
			restored := setSiteSuspension(t, false, before)
			if !restored.Applied || !restored.Changed || restored.State.Expiry != original {
				t.Fatalf("restore = %+v", restored)
			}
			again := setSiteSuspension(t, false, before)
			if !again.Applied || again.Changed {
				t.Fatalf("second restore = %+v", again)
			}
			wantCalls := [][]string{chageExpiryArgs("site_example_com", "1"), chageExpiryArgs("site_example_com", original)}
			if !reflect.DeepEqual(harness.calls, wantCalls) {
				t.Fatalf("chage calls = %v, want %v", harness.calls, wantCalls)
			}
			if got, _ := os.ReadFile(harness.shadowPath); string(got) != suspensionShadowFixture(original) {
				t.Fatalf("shadow file not restored exactly: %q", got)
			}
		})
	}
}

func TestSiteAccountSuspensionConflictChangesNothing(t *testing.T) {
	// Suspend: the owner set 20000 after the before-image ("") was taken.
	harness := newSiteSuspensionHarness(t, "20000", true)
	response := setSiteSuspension(t, true, transport.SiteAccountState{Present: true, Expiry: ""})
	if !response.Conflict || response.Applied || response.Changed || len(harness.calls) != 0 {
		t.Fatalf("suspend conflict = %+v, calls %v", response, harness.calls)
	}
	// Restore: the account is neither suspended nor at its before-image.
	response = setSiteSuspension(t, false, transport.SiteAccountState{Present: true, Expiry: ""})
	if !response.Conflict || response.Applied || len(harness.calls) != 0 {
		t.Fatalf("restore conflict = %+v, calls %v", response, harness.calls)
	}
	if got, _ := os.ReadFile(harness.shadowPath); string(got) != suspensionShadowFixture("20000") {
		t.Fatalf("shadow file changed on conflict: %q", got)
	}
}

func TestSiteAccountSuspensionAbsentAndInvalid(t *testing.T) {
	harness := newSiteSuspensionHarness(t, "", false)
	response := setSiteSuspension(t, true, transport.SiteAccountState{Present: true})
	if response.Applied || response.Conflict || !strings.HasPrefix(response.Error, "site account is absent") {
		t.Fatalf("absent = %+v", response)
	}
	var inspect transport.SuspensionInspectResponse
	_ = (&Agent{}).SuspensionInspectV1(&transport.SuspensionInspectRequest{
		SiteAccounts: []transport.SuspensionSiteAccount{{SubscriptionID: 3, DomainID: 7, Domain: "example.com"}},
	}, &inspect)
	if inspect.SiteAccounts[0].Error != "" || inspect.SiteAccounts[0].State.Present {
		t.Fatalf("absent inspect = %+v", inspect.SiteAccounts[0])
	}
	response = setSiteSuspension(t, true, transport.SiteAccountState{Present: true, Expiry: "-1"})
	if response.Applied || response.Error == "" {
		t.Fatalf("invalid before-image accepted: %+v", response)
	}
	if len(harness.calls) != 0 {
		t.Fatalf("chage ran: %v", harness.calls)
	}
}

func TestSiteAccountSuspensionChageFailureIsReported(t *testing.T) {
	harness := newSiteSuspensionHarness(t, "", true)
	harness.chageErr = errors.New("exit status 1")
	response := setSiteSuspension(t, true, transport.SiteAccountState{Present: true})
	if response.Applied || response.Changed || !strings.Contains(response.Error, "cannot lock /etc/shadow") {
		t.Fatalf("chage failure = %+v", response)
	}
	if response.State != (transport.SiteAccountState{Present: true, Expiry: ""}) {
		t.Fatalf("state after failure = %+v", response.State)
	}
}

func TestSuspensionNologinTokenRoundTrip(t *testing.T) {
	hash := "{SHA512-CRYPT}$6$salt$HASH"
	for _, tc := range []struct {
		name, line, suspended string
	}{
		{"empty extra", "a@example.com:" + hash + "::::::", "a@example.com:" + hash + "::::::nologin=y"},
		{"quota extra", "a@example.com:" + hash + "::::::userdb_quota_rule=*:storage=1024M",
			"a@example.com:" + hash + "::::::nologin=y userdb_quota_rule=*:storage=1024M"},
		{"several extra fields", "a@example.com:" + hash + "::::::userdb_quota_rule=*:storage=1M userdb_mail=maildir:~/x",
			"a@example.com:" + hash + "::::::nologin=y userdb_quota_rule=*:storage=1M userdb_mail=maildir:~/x"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if dovecotUsersLineRefusesLogin(tc.line) {
				t.Fatal("original line reads refused")
			}
			got, err := addSuspensionNologin(tc.line)
			if err != nil || got != tc.suspended {
				t.Fatalf("add = %q, %v; want %q", got, err, tc.suspended)
			}
			if !dovecotUsersLineRefusesLogin(got) {
				t.Fatal("suspended line does not read refused")
			}
			back, removed, err := removeSuspensionNologin(got)
			if err != nil || !removed || back != tc.line {
				t.Fatalf("remove = %q, %v, %v; want %q", back, removed, err, tc.line)
			}
		})
	}

	// A quota change after suspension keeps the token.
	suspended := "a@example.com:" + hash + "::::::nologin=y userdb_quota_rule=*:storage=1024M"
	quotaIndex := strings.Index(suspended, "userdb_quota_rule=")
	requota := suspended[:quotaIndex] + "userdb_quota_rule=*:storage=2048M"
	if !dovecotUsersLineRefusesLogin(requota) {
		t.Fatal("the token did not survive a quota rewrite")
	}

	// A nologin field the owner put elsewhere reads refused but is not removed.
	owner := "a@example.com:" + hash + "::::::userdb_quota_rule=*:storage=1M nologin=y"
	if !dovecotUsersLineRefusesLogin(owner) {
		t.Fatal("owner nologin not read")
	}
	if _, removed, _ := removeSuspensionNologin(owner); removed {
		t.Fatal("removed a nologin suspension did not place")
	}
	if !dovecotUsersLineRefusesLogin("a@example.com:" + hash + "::::::nologin") {
		t.Fatal("bare nologin not read")
	}

	// Fewer than 7 colons: refused, never guessed.
	for _, short := range []string{"a@example.com:" + hash, "a@example.com:" + hash + ":::::"} {
		if _, err := addSuspensionNologin(short); err == nil {
			t.Fatalf("short line accepted: %q", short)
		}
		if _, _, err := removeSuspensionNologin(short); err == nil {
			t.Fatalf("short line accepted for removal: %q", short)
		}
		if dovecotUsersLineRefusesLogin(short) {
			t.Fatalf("short line reads refused: %q", short)
		}
	}
}

func TestMailDomainSuspensionStateAndPlan(t *testing.T) {
	hash := "{SHA512-CRYPT}$6$salt$HASH"
	domains := "other.com OK\nexample.com OK\nexample.net OK\n"
	deferral := "other.com " + suspensionMailDeferReply + "\n"
	users := "a@example.com:" + hash + "::::::userdb_quota_rule=*:storage=1M\n" +
		"b@example.com:" + hash + "::::::userdb_quota_rule=*:storage=1M nologin=y\n" +
		"c@other.com:" + hash + "::::::\n"
	mailboxes := []string{"a@example.com", "b@example.com", "gone@example.com"}

	before, err := mailDomainSuspensionState([]byte(domains), []byte(deferral), []byte(users), "example.com", mailboxes)
	if err != nil {
		t.Fatal(err)
	}
	wantBefore := transport.MailDomainState{
		DomainListed: true, DomainLine: "example.com OK",
		LoginRefused: map[string]bool{"a@example.com": false, "b@example.com": true},
	}
	if !reflect.DeepEqual(before, wantBefore) {
		t.Fatalf("state = %+v, want %+v", before, wantBefore)
	}

	plan, err := planMailDomainSuspension([]byte(deferral), []byte(users), "example.com", mailboxes, true, before)
	if err != nil || len(plan.conflicts) != 0 || !plan.deferralChanged || !plan.usersChanged {
		t.Fatalf("suspend plan = %+v, %v", plan, err)
	}
	// The domain stays in the domain map; its mail is answered with a 450.
	if string(plan.deferral) != deferral+"example.com 450 4.7.1 Mailbox temporarily unavailable\n" {
		t.Fatalf("suspended deferral map = %q", plan.deferral)
	}
	wantUsers := "a@example.com:" + hash + "::::::nologin=y userdb_quota_rule=*:storage=1M\n" +
		"b@example.com:" + hash + "::::::userdb_quota_rule=*:storage=1M nologin=y\n" +
		"c@other.com:" + hash + "::::::\n"
	if string(plan.users) != wantUsers {
		t.Fatalf("suspended users = %q", plan.users)
	}
	suspended, _ := mailDomainSuspensionState([]byte(domains), plan.deferral, plan.users, "example.com", mailboxes)
	if !mailDomainStateMeetsTarget(suspended, true, before) || !suspended.DomainListed {
		t.Fatalf("suspended state does not meet the target: %+v", suspended)
	}

	// Already suspended: nothing to write.
	again, _ := planMailDomainSuspension(plan.deferral, plan.users, "example.com", mailboxes, true, before)
	if again.deferralChanged || again.usersChanged || len(again.conflicts) != 0 {
		t.Fatalf("second suspend plan = %+v", again)
	}

	// Restore removes exactly our line and only our token.
	restore, err := planMailDomainSuspension(plan.deferral, plan.users, "example.com", mailboxes, false, before)
	if err != nil || len(restore.conflicts) != 0 {
		t.Fatalf("restore plan = %+v, %v", restore, err)
	}
	if string(restore.deferral) != deferral {
		t.Fatalf("restored deferral map = %q", restore.deferral)
	}
	if string(restore.users) != users {
		t.Fatalf("restored users = %q, want %q", restore.users, users)
	}
	restored, _ := mailDomainSuspensionState([]byte(domains), restore.deferral, restore.users, "example.com", mailboxes)
	if !mailDomainStateMeetsTarget(restored, false, before) {
		t.Fatalf("restored state does not meet the target: %+v", restored)
	}
	if again, _ := planMailDomainSuspension(restore.deferral, restore.users, "example.com", mailboxes, false, before); again.deferralChanged || again.usersChanged || len(again.conflicts) != 0 {
		t.Fatalf("second restore plan = %+v", again)
	}
}

func TestMailDomainSuspensionPlanConflicts(t *testing.T) {
	hash := "{SHA512-CRYPT}$6$salt$HASH"
	users := "a@example.com:" + hash + "::::::\n"
	ours := "example.com " + suspensionMailDeferReply + "\n"
	before := transport.MailDomainState{DomainListed: true, DomainLine: "example.com OK", LoginRefused: map[string]bool{"a@example.com": false}}
	for _, tc := range []struct {
		name            string
		deferral, users string
		suspend         bool
		before          transport.MailDomainState
	}{
		{"suspend: someone else's line for the domain", "example.com REJECT\n", users, true, before},
		{"suspend: domain listed twice", ours + ours, users, true, before},
		{"suspend: short users line", "", "a@example.com:" + hash + "\n", true, before},
		{"suspend: recorded nologin removed", "", users, true,
			transport.MailDomainState{DomainListed: true, DomainLine: "example.com OK", LoginRefused: map[string]bool{"a@example.com": true}}},
		{"restore: line changed while suspended", "example.com 554 gone\n", "a@example.com:" + hash + "::::::nologin=y\n", false, before},
		{"restore: nologin placed by someone else", "", "a@example.com:" + hash + "::::::foo=1 nologin=y\n", false, before},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := planMailDomainSuspension([]byte(tc.deferral), []byte(tc.users), "example.com", []string{"a@example.com"}, tc.suspend, tc.before)
			if err != nil {
				t.Fatal(err)
			}
			if len(plan.conflicts) == 0 {
				t.Fatalf("no conflict: %+v", plan)
			}
			for _, conflict := range plan.conflicts {
				if strings.Contains(conflict, "HASH") {
					t.Fatalf("conflict discloses a hash: %q", conflict)
				}
			}
		})
	}
}

func TestMailDomainSuspensionUnrecordedMailbox(t *testing.T) {
	hash := "{SHA512-CRYPT}$6$salt$HASH"
	// n@example.com was created after inspection: suspend refuses it too...
	before := transport.MailDomainState{DomainListed: true, DomainLine: "example.com OK", LoginRefused: map[string]bool{}}
	users := "n@example.com:" + hash + "::::::\n"
	plan, _ := planMailDomainSuspension(nil, []byte(users), "example.com", []string{"n@example.com"}, true, before)
	if len(plan.conflicts) != 0 || string(plan.users) != "n@example.com:"+hash+"::::::nologin=y\n" {
		t.Fatalf("suspend plan = %+v", plan)
	}
	// ...and restore opens it again rather than leaving it refused.
	restore, _ := planMailDomainSuspension(plan.deferral, plan.users, "example.com", []string{"n@example.com"}, false, before)
	if len(restore.conflicts) != 0 || string(restore.users) != users || len(restore.deferral) != 0 {
		t.Fatalf("restore plan = %+v", restore)
	}
}

// The managed check_recipient_access element goes in front of the owner's
// restrictions while a domain is suspended and comes off exactly.
func TestSuspensionRestrictionPlanKeepsTheOwnersValue(t *testing.T) {
	old := postfixSuspendedRecipientsPath
	postfixSuspendedRecipientsPath = "/etc/postfix/celikpanel_suspended_recipients"
	t.Cleanup(func() { postfixSuspendedRecipientsPath = old })
	owner := "permit_mynetworks, permit_sasl_authenticated, reject_unauth_destination, {check_policy_service inet:127.0.0.1:10023}"
	prefix := "check_recipient_access hash:/etc/postfix/celikpanel_suspended_recipients"

	add, err := planSuspensionRestriction(owner, true, "hash")
	if err != nil || !add.change || add.unset || add.value != prefix+", "+owner {
		t.Fatalf("add = %+v, %v", add, err)
	}
	if same, _ := planSuspensionRestriction(add.value, true, "hash"); same.change {
		t.Fatalf("present element changed again: %+v", same)
	}
	remove, err := planSuspensionRestriction(add.value, false, "hash")
	if err != nil || !remove.change || remove.unset || remove.value != owner {
		t.Fatalf("remove = %+v, %v", remove, err)
	}
	// An empty (default) value: the element alone, then unset again.
	add, _ = planSuspensionRestriction("", true, "lmdb")
	if add.value != "check_recipient_access lmdb:/etc/postfix/celikpanel_suspended_recipients" {
		t.Fatalf("empty add = %+v", add)
	}
	if remove, _ := planSuspensionRestriction(add.value, false, "lmdb"); !remove.change || !remove.unset {
		t.Fatalf("empty remove = %+v", remove)
	}
	// Moved by the owner: left alone. Nothing to do without suspension.
	moved := "permit_mynetworks, " + prefix
	if plan, _ := planSuspensionRestriction(moved, false, "hash"); plan.change {
		t.Fatalf("moved element was rewritten: %+v", plan)
	}
	if plan, _ := planSuspensionRestriction(owner, false, "hash"); plan.change {
		t.Fatalf("an unsuspended server was changed: %+v", plan)
	}
	if _, err := planSuspensionRestriction("permit_mynetworks, {broken", true, "hash"); err == nil {
		t.Fatal("unbalanced braces accepted")
	}
}

func TestCanonicalSuspensionMailInputs(t *testing.T) {
	domain, mailboxes, err := canonicalSuspensionMailDomain(transport.SuspensionMailDomain{
		Domain: "Example.COM", Mailboxes: []string{"B@example.com", "a@EXAMPLE.com", "b@example.com"},
	})
	if err != nil || domain != "example.com" || !reflect.DeepEqual(mailboxes, []string{"a@example.com", "b@example.com"}) {
		t.Fatalf("got %q %v %v", domain, mailboxes, err)
	}
	if _, _, err := canonicalSuspensionMailDomain(transport.SuspensionMailDomain{Domain: "example.com", Mailboxes: []string{"a@other.com"}}); err == nil {
		t.Fatal("foreign mailbox accepted")
	}
	for _, before := range []transport.MailDomainState{
		{DomainListed: true, DomainLine: "other.com OK"},
		{DomainListed: true, DomainLine: "example.com OK\nevil.com OK"},
		{DomainLine: "example.com OK"},
		{LoginRefused: map[string]bool{"a@other.com": false}},
	} {
		if _, err := canonicalSuspensionMailBefore("example.com", before); err == nil {
			t.Fatalf("unsafe before-image accepted: %+v", before)
		}
	}
}

func TestSuspensionDatabaseSQL(t *testing.T) {
	mysql, err := suspensionDatabaseSQL("mysql", "shop_user")
	if err != nil {
		t.Fatal(err)
	}
	wantMySQL := suspensionDatabaseStatements{
		inspect:    "SELECT CONCAT('state=', CASE WHEN COUNT(*) = 0 THEN 'absent' WHEN MAX(JSON_VALUE(Priv, '$.account_locked')) IN ('true', '1') THEN 'locked' ELSE 'open' END) FROM mysql.global_priv WHERE User = 'shop_user' AND Host = 'localhost';",
		lock:       "ALTER USER 'shop_user'@'localhost' ACCOUNT LOCK;",
		disconnect: "KILL CONNECTION USER 'shop_user';",
		unlock:     "ALTER USER 'shop_user'@'localhost' ACCOUNT UNLOCK;",
	}
	if mysql != wantMySQL {
		t.Fatalf("mysql statements = %+v", mysql)
	}
	pg, err := suspensionDatabaseSQL("postgresql", "shop_user")
	if err != nil {
		t.Fatal(err)
	}
	wantPG := suspensionDatabaseStatements{
		inspect:    "SELECT 'state=' || COALESCE((SELECT CASE WHEN rolcanlogin THEN 'open' ELSE 'locked' END FROM pg_catalog.pg_roles WHERE rolname = 'shop_user'), 'absent') AS celikpanel_state;",
		lock:       `ALTER ROLE "shop_user" NOLOGIN;`,
		disconnect: "SELECT count(pg_catalog.pg_terminate_backend(pid)) FROM pg_catalog.pg_stat_activity WHERE usename = 'shop_user';",
		unlock:     `ALTER ROLE "shop_user" LOGIN;`,
	}
	if pg != wantPG {
		t.Fatalf("postgresql statements = %+v", pg)
	}

	for _, hostile := range []string{"x'; DROP USER root; --", `a"b`, "a b", "", "root", "postgres", "Root", "pg_monitor", databaseAdminAccountName, strings.Repeat("a", 64)} {
		for _, engine := range []string{"mysql", "postgresql"} {
			if _, err := suspensionDatabaseSQL(engine, hostile); err == nil {
				t.Fatalf("%s accepted %q", engine, hostile)
			}
		}
	}
	if _, err := suspensionDatabaseSQL("sqlite", "shop_user"); err == nil {
		t.Fatal("unknown engine accepted")
	}
}

func TestParseSuspensionDatabaseState(t *testing.T) {
	for output, want := range map[string]transport.DatabaseUserState{
		"state=open\n":   {Present: true},
		"state=locked\n": {Present: true, Locked: true},
		"state=absent\n": {},
		" celikpanel_state \n------------------\n state=locked\n(1 row)\n\n": {Present: true, Locked: true},
	} {
		got, err := parseSuspensionDatabaseState([]byte(output))
		if err != nil || got != want {
			t.Fatalf("parse %q = %+v, %v", output, got, err)
		}
	}
	for _, bad := range []string{"", "state=maybe\n", "state=open\nstate=open\n", "ERROR 1045"} {
		if _, err := parseSuspensionDatabaseState([]byte(bad)); err == nil {
			t.Fatalf("parse %q accepted", bad)
		}
	}
}

// fakeDatabaseEngine answers the suspension statements from an in-memory
// account and records every statement.
type fakeDatabaseEngine struct {
	present, locked bool
	statements      []string
	failOn          string
	copyHosts       string // the answer to the remote copies' host list
}

func (engine *fakeDatabaseEngine) run(statements suspensionDatabaseStatements) func(string) ([]byte, error) {
	return func(statement string) ([]byte, error) {
		engine.statements = append(engine.statements, statement)
		if engine.failOn != "" && statement == engine.failOn {
			return []byte("ERROR 1227 (42000): Access denied"), errors.New("exit status 1")
		}
		switch statement {
		case statements.inspect:
			switch {
			case !engine.present:
				return []byte("state=absent\n"), nil
			case engine.locked:
				return []byte("state=locked\n"), nil
			default:
				return []byte("state=open\n"), nil
			}
		case statements.lock:
			engine.locked = true
		case statements.unlock:
			engine.locked = false
		case statements.disconnect:
		case statements.copies:
			return []byte(engine.copyHosts), nil
		default:
			if strings.HasPrefix(statement, "ALTER USER 'shop_user'@'") && strings.Contains(statement, " ACCOUNT ") {
				return nil, nil
			}
			return nil, fmt.Errorf("unexpected statement %q", statement)
		}
		return nil, nil
	}
}

func installFakeDatabaseEngine(t *testing.T, engineName string, engine *fakeDatabaseEngine) suspensionDatabaseStatements {
	t.Helper()
	statements, err := suspensionDatabaseSQL(engineName, "shop_user")
	if err != nil {
		t.Fatal(err)
	}
	oldMySQL, oldPG := mysqlExecStatement, postgreSQLExecStatement
	mysqlExecStatement = engine.run(statements)
	postgreSQLExecStatement = engine.run(statements)
	t.Cleanup(func() { mysqlExecStatement, postgreSQLExecStatement = oldMySQL, oldPG })
	return statements
}

func setDatabaseSuspension(t *testing.T, engineName string, suspend bool, before transport.DatabaseUserState) transport.SetDatabaseUserSuspensionResponse {
	t.Helper()
	var response transport.SetDatabaseUserSuspensionResponse
	request := transport.SetDatabaseUserSuspensionRequest{
		User:    transport.SuspensionDatabaseUser{Engine: engineName, Username: "shop_user"},
		Suspend: suspend,
		Before:  before,
	}
	if err := (&Agent{}).SetDatabaseUserSuspensionV1(&request, &response); err != nil {
		t.Fatal(err)
	}
	return response
}

func TestDatabaseUserSuspensionRoundTrip(t *testing.T) {
	for _, engineName := range []string{"mysql", "postgresql"} {
		t.Run(engineName, func(t *testing.T) {
			engine := &fakeDatabaseEngine{present: true}
			statements := installFakeDatabaseEngine(t, engineName, engine)
			before := transport.DatabaseUserState{Present: true}

			first := setDatabaseSuspension(t, engineName, true, before)
			if !first.Applied || !first.Changed || first.State != (transport.DatabaseUserState{Present: true, Locked: true}) {
				t.Fatalf("suspend = %+v", first)
			}
			second := setDatabaseSuspension(t, engineName, true, before)
			if !second.Applied || second.Changed {
				t.Fatalf("second suspend = %+v", second)
			}
			restored := setDatabaseSuspension(t, engineName, false, before)
			if !restored.Applied || !restored.Changed || restored.State != before {
				t.Fatalf("restore = %+v", restored)
			}
			again := setDatabaseSuspension(t, engineName, false, before)
			if !again.Applied || again.Changed {
				t.Fatalf("second restore = %+v", again)
			}
			want := []string{
				statements.inspect, statements.lock, statements.disconnect, statements.inspect,
				statements.inspect, statements.disconnect, statements.inspect,
				statements.inspect, statements.unlock, statements.inspect,
				statements.inspect, statements.inspect,
			}
			if engineName == "mysql" {
				// alpha.129: the remote copies' hosts are read on every
				// change, before the sessions are ended.
				want = []string{
					statements.inspect, statements.lock, statements.copies, statements.disconnect, statements.inspect,
					statements.inspect, statements.copies, statements.disconnect, statements.inspect,
					statements.inspect, statements.unlock, statements.copies, statements.inspect,
					statements.inspect, statements.copies, statements.inspect,
				}
			}
			if !reflect.DeepEqual(engine.statements, want) {
				t.Fatalf("statements =\n%s\nwant\n%s", strings.Join(engine.statements, "\n"), strings.Join(want, "\n"))
			}
		})
	}
}

func TestDatabaseUserSuspensionOwnerLockAndConflicts(t *testing.T) {
	// The owner had locked the account before suspension: restore keeps it locked.
	engine := &fakeDatabaseEngine{present: true, locked: true}
	installFakeDatabaseEngine(t, "mysql", engine)
	ownerLocked := transport.DatabaseUserState{Present: true, Locked: true}
	if response := setDatabaseSuspension(t, "mysql", false, ownerLocked); !response.Applied || response.Changed || !engine.locked {
		t.Fatalf("owner-locked restore = %+v", response)
	}
	// Recorded as locked, now open while suspended: neither state.
	engine.locked = false
	if response := setDatabaseSuspension(t, "mysql", false, ownerLocked); !response.Conflict || response.Applied || engine.locked {
		t.Fatalf("conflict = %+v", response)
	}
	// Absent account.
	engine.present = false
	if response := setDatabaseSuspension(t, "mysql", true, transport.DatabaseUserState{Present: true}); response.Applied || !strings.HasPrefix(response.Error, "database user is absent") {
		t.Fatalf("absent = %+v", response)
	}
}

func TestDatabaseUserSuspensionDisconnectFailureIsNotApplied(t *testing.T) {
	engine := &fakeDatabaseEngine{present: true}
	statements := installFakeDatabaseEngine(t, "postgresql", engine)
	engine.failOn = statements.disconnect
	response := setDatabaseSuspension(t, "postgresql", true, transport.DatabaseUserState{Present: true})
	if response.Applied || !response.Changed || response.Error == "" || !response.State.Locked {
		t.Fatalf("disconnect failure = %+v", response)
	}
	// The retry ends the sessions even though the lock is already there.
	engine.failOn = ""
	response = setDatabaseSuspension(t, "postgresql", true, transport.DatabaseUserState{Present: true})
	if !response.Applied || response.Changed {
		t.Fatalf("retry = %+v", response)
	}
	if engine.statements[len(engine.statements)-2] != statements.disconnect {
		t.Fatalf("retry did not end sessions: %v", engine.statements)
	}
}

// alpha.129 (review M1): suspension locks every remote copy of a MariaDB
// user with the local account, and restore unlocks them; hosts in any other
// form are never touched, and a copy that cannot be locked fails the step.
func TestDatabaseUserSuspensionLocksTheRemoteCopies(t *testing.T) {
	engine := &fakeDatabaseEngine{present: true, copyHosts: "203.0.113.7\n198.51.100.0/255.255.255.0\n%\nexample.com\n"}
	installFakeDatabaseEngine(t, "mysql", engine)
	before := transport.DatabaseUserState{Present: true}
	if response := setDatabaseSuspension(t, "mysql", true, before); !response.Applied {
		t.Fatalf("suspend = %+v", response)
	}
	lock := "ALTER USER 'shop_user'@'203.0.113.7' ACCOUNT LOCK;\nALTER USER 'shop_user'@'198.51.100.0/255.255.255.0' ACCOUNT LOCK;"
	if !containsString(engine.statements, lock) {
		t.Fatalf("copies not locked: %q", engine.statements)
	}
	if response := setDatabaseSuspension(t, "mysql", false, before); !response.Applied {
		t.Fatalf("restore = %+v", response)
	}
	if !containsString(engine.statements, strings.ReplaceAll(lock, "ACCOUNT LOCK", "ACCOUNT UNLOCK")) {
		t.Fatalf("copies not unlocked: %q", engine.statements)
	}
	for _, s := range engine.statements {
		if strings.Contains(s, "'%'") || strings.Contains(s, "example.com") {
			t.Fatalf("a host CelikPanel does not write was touched: %q", s)
		}
	}
	engine.failOn = lock
	if response := setDatabaseSuspension(t, "mysql", true, before); response.Applied || response.Error == "" {
		t.Fatalf("a copy that could not be locked was reported applied: %+v", response)
	}
}
