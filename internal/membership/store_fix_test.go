package membership

// The S2 fix round (REVIEW-S2.md, S2-FIX-ROUND.md): each test fails without its fix.

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/alicelik/celikpanel/internal/payment"
)

// 1. A lost dispute is applied like a refund; an open one only reaches the operator.
// Reconciliation re-confirms paid orders, so a lost webhook cannot hide it.
func TestALostDisputeIsApplied(t *testing.T) {
	site := newStoreSite(t, "test", "test")
	config := must(LoadConfig(site.dir))
	session := site.buy()
	site.stripe.pay(session)
	order := must(site.s.OrderByProviderRef("stripe", session))
	clock := order.Created + 700
	site.s.clock = func() int64 { return clock }
	must(site.s.Reconcile(t.Context(), site.handler.Payments, config, "reconcile:test"))
	if must(site.s.GetOrder(order.ID)).State != OrderPaid {
		t.Fatal("not paid")
	}
	// An open dispute: nothing changes, the operator sees it.
	site.stripe.mu.Lock()
	site.stripe.sessions[session].disputed, site.stripe.sessions[session].dispute = true, "needs_response"
	site.stripe.mu.Unlock()
	clock += 86400
	report := must(site.s.Reconcile(t.Context(), site.handler.Payments, config, "reconcile:test"))
	if revoked, _, _ := site.s.LicenceRevocation(site.licence.ID); revoked || len(report.Attention) != 1 || !strings.Contains(report.Attention[0].Attention, "dispute needs_response") {
		t.Fatalf("an open dispute: %+v", report.Attention)
	}
	// Lost, and no webhook came: the next day's reconciliation applies it.
	site.stripe.mu.Lock()
	site.stripe.sessions[session].dispute = "lost"
	site.stripe.mu.Unlock()
	clock += 3600
	if report = must(site.s.Reconcile(t.Context(), site.handler.Payments, config, "reconcile:test")); report.Confirmed != 0 {
		t.Fatal("a paid order was asked about twice in a day")
	}
	clock += 86400
	report = must(site.s.Reconcile(t.Context(), site.handler.Payments, config, "reconcile:test"))
	if revoked, _, _ := site.s.LicenceRevocation(site.licence.ID); !revoked || report.Refunded != 1 || must(site.s.GetOrder(order.ID)).State != OrderRefunded {
		t.Fatalf("a lost dispute: %+v", report)
	}
}

// 2. An account that buys its years: the first licence is the 30-day trial, a later one waits
// for its first year, and there is no free renewal. The beta (charging off) is unchanged.
func TestTrialAndLicencesAwaitingPayment(t *testing.T) {
	site := newStoreSite(t, "test", "test")
	// The site's member already has a free licence from before; the trial is still unused.
	_, _, home := site.page(http.MethodGet, "lang=en", nil)
	if !strings.Contains(home, "Your first license is a free 30-day trial; no payment details are asked.") || !strings.Contains(home, "Start the 30-day trial") ||
		strings.Contains(home, "Create a free license") || strings.Contains(home, "Renew for free") {
		t.Fatalf("home of a charged account: %s", home)
	}
	site.page(http.MethodPost, "action=issue&lang=en", nil)
	_, _, home = site.page(http.MethodGet, "lang=en", nil)
	trialID, ends := must2(site.s.Trial(site.member))
	if trialID == "" || !strings.Contains(home, "Free trial until "+day(ends)+".") || !strings.Contains(home, "A new license can be used once its first year is paid.") {
		t.Fatalf("after the trial started: %s", home)
	}
	// A second licence: awaiting payment, with its order, and it cannot be activated.
	site.page(http.MethodPost, "action=issue&lang=en", nil)
	_, _, home = site.page(http.MethodGet, "lang=en", nil)
	if strings.Count(home, "Awaiting payment: this license can be used once its first year is paid.") != 1 {
		t.Fatalf("a licence awaiting payment: %s", home)
	}
	var waiting Issued
	for _, l := range must(site.s.Licenses(site.member)) {
		if yes, _ := site.s.AwaitingPayment(l.ID); yes {
			waiting.ID = l.ID
		}
	}
	key := must(site.s.Reveal(site.member, waiting.ID, "buyer-password-1234")).Key
	if _, err := site.s.Activate(key, strings.Repeat("c", 64), "c.example.com", "8.8.8.8"); !errors.Is(err, ErrLicenseExpired) {
		t.Fatalf("activating a licence awaiting payment: %v", err)
	}
	if !site.s.HasOpenOrder(waiting.ID) {
		t.Fatal("the licence awaiting payment has no order")
	}
	// No free renewal for a charged account.
	if _, _, body := site.page(http.MethodPost, "action=renew&lang=en", url.Values{"id": {trialID}, "password": {"buyer-password-1234"}}); !strings.Contains(body, "This license is renewed by buying a year") {
		t.Fatalf("a free renewal: %s", body)
	}
}

// 2 (beta), 3. Everyone who is not a flagged test account sees the beta pages.
func TestTheBetaPagesStayForEveryoneElse(t *testing.T) {
	site := newStoreSite(t, "test", "test")
	if err := site.s.SetTestAccount("operator:test", "unflag", site.member, false); err != nil {
		t.Fatal(err)
	}
	_, _, home := site.page(http.MethodGet, "lang=en", nil)
	for _, word := range []string{"Buy a year · ", "Test mode", "Start the 30-day trial"} {
		if strings.Contains(home, word) {
			t.Fatalf("%q shown to an account that does not buy: %s", word, home)
		}
	}
	if !strings.Contains(home, "Create a free license") || !strings.Contains(home, "CelikPanel is free during the beta.") {
		t.Fatalf("the beta home: %s", home)
	}
}

// 3. Test money stays apart: an unflagged account's test payment grants nothing; the flag stays
// while test-paid time runs; live history never sees test orders.
func TestTestMoneyIsFencedOff(t *testing.T) {
	f := newStore(t)
	s := f.s
	o := must(s.CreateOrder(f.member, f.licence.ID, ChargingTest))
	if err := s.SetTestAccount("operator:test", "unflag before paying", f.member, false); err != nil {
		t.Fatal(err)
	}
	before := f.expires(f.licence.ID)
	o = must(s.ApplyFact(o.ID, "stripe", "provider:stripe", paid(o, "pi_1")))
	if o.State != OrderMismatch || f.expires(f.licence.ID) != before || !strings.Contains(o.Attention, "no longer a test account") {
		t.Fatalf("a test payment for an unflagged account: %+v", o)
	}
	if err := s.SetTestAccount("operator:test", "flag", f.member, true); err != nil {
		t.Fatal(err)
	}
	l2 := must(s.Issue(f.member))
	o2 := must(s.CreateOrder(f.member, l2.ID, ChargingTest))
	must(s.ApplyFact(o2.ID, "stripe", "provider:stripe", paid(o2, "pi_2")))
	if err := s.SetTestAccount("operator:test", "unflag", f.member, false); !errors.Is(err, ErrTestTimeRemains) {
		t.Fatalf("unflagging while test time runs: %v", err)
	}
	l, _ := scanRow(s.DB().QueryRow("SELECT "+rowColumns+" FROM licenses WHERE id=?", l2.ID))
	live := must(s.quote(t.Context(), s.DB(), l, f.h.now, true))
	if live.Kind != KindFirstYear || live.Locked {
		t.Fatalf("a live quote sees test history: %+v", live)
	}
}

// 4. Removing a licence closes its open order; a payment that still arrives grants nothing.
func TestAPaymentNeverFallsIntoNothing(t *testing.T) {
	f := newStore(t)
	s := f.s
	o := must(s.CreateOrder(f.member, f.licence.ID, ChargingTest))
	if err := s.Remove(f.member, f.licence.ID, f.password); err != nil {
		t.Fatal(err)
	}
	if must(s.GetOrder(o.ID)).State != OrderExpired {
		t.Fatal("the removed licence's order is still open")
	}
	o = must(s.ApplyFact(o.ID, "stripe", "provider:stripe", paid(o, "pi_1")))
	if o.State != OrderMismatch || !strings.Contains(o.Attention, "removed") {
		t.Fatalf("a payment for a removed licence: %+v", o)
	}
}

// 5. The return page and the webhook at once: one receipt.
func TestTheReceiptGoesOnce(t *testing.T) {
	site := newStoreSite(t, "test", "test")
	session := site.buy()
	order := must(site.s.OrderByProviderRef("stripe", session))
	site.stripe.pay(session)
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if response, err := site.client.Get(site.server.URL + "/account/?action=order&id=" + order.ID + "&lang=en"); err == nil {
				response.Body.Close()
			}
		}()
		go func(i int) {
			defer wg.Done()
			site.event(completed("evt_"+string(rune('a'+i)), session, order.ID, site.licence.ID), site.webhook)
		}(i)
	}
	wg.Wait()
	site.mu.Lock()
	defer site.mu.Unlock()
	if len(site.mails) != 1 {
		t.Fatalf("%d receipt mails", len(site.mails))
	}
}

// 6. A second "Continue to payment" from the other language goes to the same session.
func TestOneCheckoutSessionPerOrder(t *testing.T) {
	site := newStoreSite(t, "test", "test")
	first := site.buy()
	_, header, _ := site.page(http.MethodPost, "action=buy&lang=tr", url.Values{"id": {site.licence.ID}})
	if header.Get("Location") != "https://checkout.example/"+first || site.stripe.posts != 1 {
		t.Fatalf("a second click in Turkish: %q, %d sessions created", header.Get("Location"), site.stripe.posts)
	}
	// The session ended at the provider: the order closes and a new one gets a new session.
	site.stripe.mu.Lock()
	site.stripe.sessions[first].status = "expired"
	site.stripe.mu.Unlock()
	if again := site.buy(); again == first || site.stripe.posts != 2 {
		t.Fatalf("after the session ended: %s, %d", again, site.stripe.posts)
	}
	if must(site.s.OrderByProviderRef("stripe", first)).State != OrderExpired {
		t.Fatal("the first order did not close")
	}
	if site.stripe.session(first).expiresAt == "" {
		t.Fatal("the session was created without the order's end")
	}
}

// 7. A renewal at the locked price is valid only until the licence's end; too close to the end
// it waits; a locked price paid after the end grants nothing.
func TestALockedRenewalEndsWithTheLicence(t *testing.T) {
	f := newStore(t)
	s := f.s
	first := must(s.CreateOrder(f.member, f.licence.ID, ChargingTest))
	must(s.ApplyFact(first.ID, "stripe", "provider:stripe", paid(first, "pi_1")))
	end := f.expires(f.licence.ID)
	f.h.now = end - 3600 // an hour before the end
	renewal := must(s.CreateOrder(f.member, f.licence.ID, ChargingTest))
	if !renewal.Locked || renewal.Expires != end {
		t.Fatalf("a locked renewal an hour before the end: %+v", renewal)
	}
	f.h.now = end + 60 // paid after the end at the locked price
	renewal = must(s.ApplyFact(renewal.ID, "stripe", "provider:stripe", paid(renewal, "pi_2")))
	if renewal.State != OrderMismatch || f.expires(f.licence.ID) != end {
		t.Fatalf("a locked price paid after the end: %+v", renewal)
	}
	// Ten minutes before an end: the year is bought after the end.
	l2 := must(s.Issue(f.member))
	o := must(s.CreateOrder(f.member, l2.ID, ChargingTest))
	must(s.ApplyFact(o.ID, "stripe", "provider:stripe", paid(o, "pi_3")))
	f.h.now = f.expires(l2.ID) - 600
	if _, err := s.CreateOrder(f.member, l2.ID, ChargingTest); !errors.Is(err, ErrBuyAfterEnd) {
		t.Fatalf("ten minutes before the end: %v", err)
	}
}

// 11. Smaller points: ApplyFact checks the amount itself; prices never start in the past and
// are append-only.
func TestStoreSmallerPoints(t *testing.T) {
	f := newStore(t)
	s := f.s
	o := must(s.CreateOrder(f.member, f.licence.ID, ChargingTest))
	wrong := paid(o, "pi_1")
	wrong.PaidMinor = 100
	if _, err := s.ApplyFact(o.ID, "stripe", "provider:stripe", wrong); err == nil {
		t.Fatal("a fact for another amount was applied")
	}
	if err := s.SetPrice("operator:test", "past", "USD", 12900, f.h.now-86400); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("a price from yesterday: %v", err)
	}
	if _, err := s.DB().Exec("UPDATE store_prices SET amount_minor=1"); err == nil {
		t.Fatal("a price was changed in place")
	}
	if _, err := s.DB().Exec("DELETE FROM store_prices"); err == nil {
		t.Fatal("the price history was deleted")
	}
	// The operator's refund leaves no row when nothing can be sent.
	o = must(s.ApplyFact(o.ID, "stripe", "provider:stripe", paid(o, "pi_1")))
	if _, _, err := s.OperatorRefund(t.Context(), payment.DefaultRegistry(), Config{}, "operator:test", o.ID, "nothing to send"); !errors.Is(err, ErrPaymentNotReady) {
		t.Fatalf("a refund without the provider: %v", err)
	}
	if refunds := must(s.Refunds(o.ID)); len(refunds) != 0 {
		t.Fatalf("a refund row without a request: %+v", refunds)
	}
}

func must2[A, B any](a A, b B, err error) (A, B) {
	if err != nil {
		panic(err)
	}
	return a, b
}
