package membership

// The S2 third look (REVIEW-S2-ROUND2.md): each test fails without its fix.

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/payment"
)

// F1: the one-trial-per-server rule holds only while the licence is still in its trial, per
// mode; a bought year works on that server.
func TestABoughtTrialLicenceWorksOnATrialServer(t *testing.T) {
	site := buildStoreSite(t, "test", "test", false)
	site.page(http.MethodPost, "action=issue&lang=en", nil)
	trialID, _, _ := must3(site.s.Trial(site.member, ChargingTest))
	key := must(site.s.Reveal(site.member, trialID, "buyer-password-1234")).Key
	server := strings.Repeat("d", 64)
	other := must(site.s.Issue(site.member))
	if _, err := site.s.DB().Exec("INSERT INTO store_trial_servers(server_id,mode,license_id,at) VALUES(?,'live',?,1)", server, other.ID); err != nil {
		t.Fatal(err)
	}
	// A live-mode trial on that server does not count for a test-mode trial.
	if _, err := site.s.Activate(key, server, "d.example.com", "8.8.8.8"); err != nil {
		t.Fatalf("a live trial server blocked a test trial: %v", err)
	}
	server2 := strings.Repeat("f", 64)
	if _, err := site.s.DB().Exec("INSERT INTO store_trial_servers(server_id,mode,license_id,at) VALUES(?,'test',?,1)", server2, other.ID); err != nil {
		t.Fatal(err)
	}
	if err := site.s.Release(site.member, trialID, "buyer-password-1234"); err != nil {
		t.Fatal(err)
	}
	if _, err := site.s.Activate(key, server2, "f.example.com", "8.8.4.4"); !errors.Is(err, ErrInvalidLicense) {
		t.Fatalf("a trial on a server that ran a test trial: %v", err)
	}
	// Buy the year: the licence activates on that server like any paid licence.
	site.licence = Issued{ID: trialID}
	session := site.buy()
	site.stripe.pay(session)
	order := must(site.s.OrderByProviderRef("stripe", session))
	site.page(http.MethodGet, "action=order&id="+order.ID+"&lang=en", nil)
	if _, err := site.s.Activate(key, server2, "f.example.com", "8.8.4.4"); err != nil {
		t.Fatalf("a bought year on the trial server: %v", err)
	}
}

// F3: reinstate gives no time back; the history is append-only; a lost dispute is recorded
// as such.
func TestReinstateGivesNoTimeBack(t *testing.T) {
	f := newStore(t)
	s := f.s
	f.h.now = f.expires(f.licence.ID) + 86400
	o := must(s.CreateOrder(f.member, f.licence.ID, ChargingTest))
	o = must(s.ApplyFact(o.ID, "stripe", "provider:stripe", paid(o, "pi_1")))
	lost := paid(o, "pi_1")
	lost.Outcome, lost.Disputed, lost.DisputeStatus = payment.OutcomeReversed, true, "lost"
	f.h.now += 3600
	must(s.ApplyFact(o.ID, "stripe", "provider:stripe", lost))
	if _, why, _ := s.LicenceRevocation(f.licence.ID); why != "dispute_lost" {
		t.Fatalf("a lost dispute recorded as %q", why)
	}
	if _, err := s.DB().Exec("UPDATE store_revocations SET expires_before=0"); err == nil {
		t.Fatal("the revocation history was changed")
	}
	if err := s.Reinstate("operator:test", f.licence.ID, "applied to the wrong licence"); err != nil {
		t.Fatal(err)
	}
	if end := f.expires(f.licence.ID); end != f.h.now {
		t.Fatalf("reinstate gave time back: end %d, now %d", end, f.h.now)
	}
}

// F4: a webhook whose payment cannot be confirmed opens an attention entry.
func TestAWebhookThatCannotConfirmIsListed(t *testing.T) {
	site := newStoreSite(t, "test", "test")
	order := must(site.s.CreateOrder(site.member, site.licence.ID, ChargingTest))
	if err := site.s.AttachCheckout(order.ID, "stripe", "cs_missing"); err != nil {
		t.Fatal(err)
	}
	if status, _ := site.event(completed("evt_x", "cs_missing", order.ID, site.licence.ID), site.webhook); status != 200 {
		t.Fatalf("status %d", status)
	}
	list := must(site.s.AttentionOrders())
	if len(list) != 1 || list[0].Kind != AttentionUnconfirmed || list[0].Order.ID != order.ID {
		t.Fatalf("not listed: %+v", list)
	}
}

// F5: removing a licence ends its open payment page first; a page that is being paid refuses
// the removal.
func TestRemovingALicenceEndsItsPaymentPage(t *testing.T) {
	site := newStoreSite(t, "test", "test")
	session := site.buy()
	// Being paid: refused.
	site.stripe.mu.Lock()
	site.stripe.sessions[session].status = "complete"
	site.stripe.mu.Unlock()
	if _, _, body := site.page(http.MethodPost, "action=remove&lang=en", url.Values{"id": {site.licence.ID}, "password": {"buyer-password-1234"}}); !strings.Contains(body, "A payment for this license is being completed.") {
		t.Fatalf("removed while being paid: %s", body)
	}
	// Open: the page is ended at the provider, then the licence goes.
	site.stripe.mu.Lock()
	site.stripe.sessions[session].status = "open"
	site.stripe.mu.Unlock()
	site.page(http.MethodPost, "action=remove&lang=en", url.Values{"id": {site.licence.ID}, "password": {"buyer-password-1234"}})
	if site.stripe.session(session).status != "expired" {
		t.Fatal("the payment page was left open")
	}
	if len(must(site.s.Licenses(site.member))) != 0 {
		t.Fatal("the licence was not removed")
	}
}

// F6: a locked renewal confirmed after its checkout's end when the licence's end moved earlier
// starts at the checkout's end, never earlier; the operator's late grant starts now.
func TestLockedStartsNeverBackDate(t *testing.T) {
	f := newStore(t)
	s := f.s
	first := must(s.CreateOrder(f.member, f.licence.ID, ChargingTest))
	must(s.ApplyFact(first.ID, "stripe", "provider:stripe", paid(first, "pi_1")))
	end := f.expires(f.licence.ID)
	f.h.now = end - 7200
	renewal := must(s.CreateOrder(f.member, f.licence.ID, ChargingTest))
	// A lost dispute elsewhere moved the end earlier meanwhile (simulated).
	moved := f.h.now - 86400
	if _, err := s.DB().Exec("UPDATE licenses SET expires=? WHERE id=?", moved, f.licence.ID); err != nil {
		t.Fatal(err)
	}
	f.h.now = renewal.Expires + 600
	renewal = must(s.ApplyFact(renewal.ID, "stripe", "provider:stripe", paid(renewal, "pi_2")))
	if renewal.GrantedFrom != renewal.Expires {
		t.Fatalf("back-dated: from %d, the checkout ended %d (moved end %d)", renewal.GrantedFrom, renewal.Expires, moved)
	}
}

// F7: what a member reads is true: a reversed payment that left time says until when; the buy
// page offers the trial only to who can have it; a locked order with too little time says so.
func TestMemberTextsAreTrue(t *testing.T) {
	site := newStoreSite(t, "test", "test")
	session := site.buy()
	site.stripe.pay(session)
	order := must(site.s.OrderByProviderRef("stripe", session))
	site.page(http.MethodGet, "action=order&id="+order.ID+"&lang=en", nil)
	freeEnd := must(site.s.GetOrder(order.ID)).PreviousExpires
	site.stripe.mu.Lock()
	site.stripe.sessions[session].refunded = site.stripe.sessions[session].amount
	site.stripe.mu.Unlock()
	refund := `{"id":"evt_r","type":"charge.refunded","data":{"object":{"id":"ch_` + session + `","payment_intent":"pi_` + session + `","amount":9900,"amount_refunded":9900,"refunded":true}}}`
	site.event(refund, site.webhook)
	_, _, page := site.page(http.MethodGet, "action=order&id="+order.ID+"&lang=en", nil)
	if !strings.Contains(page, "your license now runs until "+day(freeEnd)) || strings.Contains(page, "the license has ended") {
		t.Fatalf("a reversed payment that left time: %s", page)
	}
	// A beta account: the buy page does not offer a trial it cannot have.
	_, _, buy := site.page(http.MethodGet, "action=buy&id="+site.licence.ID+"&lang=en", nil)
	if strings.Contains(buy, "30-day trial") || !strings.Contains(buy, "Payments are not refunded.") {
		t.Fatalf("the buy page of a beta account: %s", buy)
	}
	// A new account sees the trial sentence on the buy page of its licence awaiting payment.
	fresh := buildStoreSite(t, "test", "test", false)
	fresh.s.clock = func() int64 { return time.Now().Unix() - 60 }
	if _, trial, err := fresh.s.IssueCharged(fresh.member, ChargingTest); err != nil || !trial {
		t.Fatalf("the trial: %v %v", trial, err)
	}
	trialID, _, _ := must3(fresh.s.Trial(fresh.member, ChargingTest))
	if _, err := fresh.s.DB().Exec("DELETE FROM store_trials"); err != nil { // as if it had none yet
		t.Fatal(err)
	}
	if _, err := fresh.s.DB().Exec("UPDATE licenses SET expires=created WHERE id=?", trialID); err != nil {
		t.Fatal(err)
	}
	_, _, buy = fresh.page(http.MethodGet, "action=buy&id="+trialID+"&lang=en", nil)
	if !strings.Contains(buy, "A free 30-day trial is available the first time.") {
		t.Fatalf("the trial sentence for an account that can have it: %s", buy)
	}
}

// F8: the cap leaves the rest for the next run and rotates; a failing order backs off.
func TestReconciliationCapAndRotation(t *testing.T) {
	site := newStoreSite(t, "test", "test")
	config := must(LoadConfig(site.dir))
	clock := time.Now().Unix()
	site.s.clock = func() int64 { return clock }
	var ids []string
	for i := 0; i < 3; i++ {
		l := must(site.s.Issue(site.member))
		o := must(site.s.CreateOrder(site.member, l.ID, ChargingTest))
		ref := "cs_none_" + string(rune('a'+i))
		if err := site.s.AttachCheckout(o.ID, "stripe", ref); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, o.ID)
		if _, err := site.s.DB().Exec("DELETE FROM rate_limits"); err != nil {
			t.Fatal(err)
		}
	}
	old := ReconcileCap
	ReconcileCap = 2
	defer func() { ReconcileCap = old }()
	clock += 700
	r := must(site.s.Reconcile(t.Context(), site.handler.Payments, config, "reconcile:test"))
	if r.Left != 1 || len(r.Failed) != 2 {
		t.Fatalf("the cap: %+v", r)
	}
	// The next run asks the one left first; the failing two wait their day.
	r = must(site.s.Reconcile(t.Context(), site.handler.Payments, config, "reconcile:test"))
	if r.Left != 0 || len(r.Failed) != 1 || !strings.HasPrefix(r.Failed[0], ids[2]) {
		t.Fatalf("the rotation: %+v", r)
	}
	var lines int
	if err := site.s.DB().QueryRow("SELECT COUNT(*) FROM store_audit WHERE action='attention_opened'").Scan(&lines); err != nil || lines != 3 {
		t.Fatalf("attention lines: %d", lines)
	}
}

// F9: a partial refund's entry closes once the payment is refunded in full; a beta licence that
// was never activated counts as the account's free period.
func TestPartialRefundClosesAndUnactivatedBetaCounts(t *testing.T) {
	f := newStore(t)
	s := f.s
	f.h.now = f.expires(f.licence.ID) + 86400
	o := must(s.CreateOrder(f.member, f.licence.ID, ChargingTest))
	o = must(s.ApplyFact(o.ID, "stripe", "provider:stripe", paid(o, "pi_1")))
	part := paid(o, "pi_1")
	part.Outcome, part.RefundedMinor = payment.OutcomePartlyRefunded, 100
	must(s.ApplyFact(o.ID, "stripe", "provider:stripe", part))
	if len(must(s.AttentionOrders())) != 1 {
		t.Fatal("the partial refund is not listed")
	}
	full := paid(o, "pi_1")
	full.Outcome = payment.OutcomeRefunded
	must(s.ApplyFact(o.ID, "stripe", "provider:stripe", full))
	if list := must(s.AttentionOrders()); len(list) != 0 {
		t.Fatalf("the partial refund stays listed after the full refund: %+v", list)
	}
	// A beta licence never activated (no end yet) counts as the free period.
	h := newHarness(t)
	if err := h.s.MigrateStore("operator:test"); err != nil {
		t.Fatal(err)
	}
	if err := h.s.Register("beta@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := h.s.ConsumeToken(h.lastMail().Token, "verify", "beta-password-12345"); err != nil {
		t.Fatal(err)
	}
	m := must(h.s.Login("beta@example.com", "beta-password-12345"))
	l := must(h.s.Issue(m.ID))
	if _, err := h.s.DB().Exec("UPDATE licenses SET expires=NULL WHERE id=?", l.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, used := must3(h.s.Trial(m.ID, ChargingTest)); !used {
		t.Fatal("an unactivated beta licence did not count as the free period")
	}
}
