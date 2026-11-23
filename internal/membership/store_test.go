package membership

// The store (S2, step 1): the explicit migration, prices, test accounts, orders that only
// move forward, a fact applied once per payment, the first paid year granted in the same
// transaction, refunds that revoke with the generation raised, and the append-only log.

import (
	"errors"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/payment"
)

type storeFixture struct {
	h        *serviceHarness
	s        *Service
	member   int64
	password string
	licence  Issued
}

// newStore: a verified member flagged as a test account, one licence bound to server a,
// and the store migrated.
func newStore(t *testing.T) *storeFixture {
	t.Helper()
	h := newHarness(t)
	s := h.s
	f := &storeFixture{h: h, s: s, password: "store-password-12345"}
	if err := s.Register("buyer@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := s.ConsumeToken(h.lastMail().Token, "verify", f.password); err != nil {
		t.Fatal(err)
	}
	f.member = must(s.Login("buyer@example.com", f.password)).ID
	f.licence = must(s.Issue(f.member))
	must(s.Activate(f.licence.Key, strings.Repeat("a", 64), "a.example.com", "8.8.8.8"))
	if err := s.MigrateStore("operator:test"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetTestAccount("operator:test", "store tests", f.member, true); err != nil {
		t.Fatal(err)
	}
	return f
}

// paid is the fact Confirm returns for a paid order (what the fake provider confirms).
func paid(o Order, paymentID string) payment.Fact {
	return payment.Fact{Outcome: payment.OutcomePaid, PaymentID: paymentID, Live: o.Live, Status: payment.StatusSucceeded, Kind: payment.OrderOneTime,
		Order: o.ID, LicenseID: o.LicenseID, PaidMinor: o.AmountMinor, Currency: o.Currency, PaymentIntent: paymentID}
}

func (f *storeFixture) expires(id string) int64 {
	f.h.t.Helper()
	var at int64
	if err := f.s.DB().QueryRow("SELECT expires FROM licenses WHERE id=?", id).Scan(&at); err != nil {
		f.h.t.Fatal(err)
	}
	return at
}

func TestStoreIsCreatedOnlyByItsMigration(t *testing.T) {
	h := newHarness(t)
	s := h.s
	if s.StoreReady() {
		t.Fatal("the store exists before its migration")
	}
	var tables int
	if err := s.DB().QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE name LIKE 'store_%'").Scan(&tables); err != nil || tables != 0 {
		t.Fatalf("opening the service made store tables: %d %v", tables, err)
	}
	if _, err := s.CreateOrder(1, strings.Repeat("a", 32), ChargingTest); !errors.Is(err, ErrStoreNotReady) {
		t.Fatalf("an order without the store: %v", err)
	}
	for i := 0; i < 2; i++ { // repeatable
		if err := s.MigrateStore("operator:test"); err != nil {
			t.Fatal(err)
		}
	}
	if !s.StoreReady() {
		t.Fatal("not ready after the migration")
	}
	price, err := s.CurrentPrice("USD")
	if err != nil || price.AmountMinor != 9900 || !strings.Contains(price.Reason, "D-047") {
		t.Fatalf("the D-047 list price: %+v %v", price, err)
	}
	var prices int
	if err := s.DB().QueryRow("SELECT COUNT(*) FROM store_prices").Scan(&prices); err != nil || prices != 1 {
		t.Fatalf("the price was seeded twice: %d", prices)
	}
	columns, err := tableColumns(t.Context(), s.DB(), "licenses")
	if err != nil || !columns["revoked"] || !columns["revoked_reason"] {
		t.Fatalf("licence columns: %v %v", columns, err)
	}
	// A new connection (a later request) sees it without creating anything.
	again := h.open()
	if !again.StoreReady() || !again.hasRevoked {
		t.Fatal("a later request does not see the store")
	}
}

func TestStoreAuditIsAppendOnlyAndOrdersOnlyMoveForward(t *testing.T) {
	f := newStore(t)
	if _, err := f.s.DB().Exec("UPDATE store_audit SET actor='x'"); err == nil || !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("an audit line was changed: %v", err)
	}
	if _, err := f.s.DB().Exec("DELETE FROM store_audit"); err == nil {
		t.Fatal("the audit log was deleted")
	}
	o := must(f.s.CreateOrder(f.member, f.licence.ID, ChargingTest))
	must(f.s.ApplyFact(o.ID, "stripe", "provider:stripe", paid(o, "pi_1")))
	for _, back := range []string{OrderCreated, OrderPending, OrderExpired, OrderMismatch} {
		if _, err := f.s.DB().Exec("UPDATE store_orders SET state=? WHERE id=?", back, o.ID); err == nil {
			t.Fatalf("paid went back to %s", back)
		}
	}
	if _, err := f.s.DB().Exec("UPDATE store_orders SET kind='gift' WHERE id=?", o.ID); err == nil {
		t.Fatal("an unknown kind was accepted by the schema")
	}
}

func TestOrderRulesForChargingAndAccounts(t *testing.T) {
	f := newStore(t)
	s := f.s
	if _, err := s.CreateOrder(f.member, f.licence.ID, ChargingOff); !errors.Is(err, ErrChargingOff) {
		t.Fatalf("charging off: %v", err)
	}
	if _, err := s.CreateOrder(f.member, f.licence.ID, ChargingLive); !errors.Is(err, ErrChargingLiveDisabled) {
		t.Fatalf("live: %v", err)
	}
	if _, err := s.CreateOrder(f.member, f.licence.ID, ""); !errors.Is(err, ErrChargingOff) {
		t.Fatalf("an unset mode is off: %v", err)
	}
	// Not a test account: refused.
	if err := s.SetTestAccount("operator:test", "store tests", f.member, false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateOrder(f.member, f.licence.ID, ChargingTest); !errors.Is(err, ErrNotATestAccount) {
		t.Fatalf("a test order on a real account: %v", err)
	}
	if err := s.SetTestAccount("operator:test", "store tests", f.member, true); err != nil {
		t.Fatal(err)
	}
	// Another member's licence, a removed one: not found.
	if _, err := s.CreateOrder(f.member+1, f.licence.ID, ChargingTest); err == nil {
		t.Fatal("another member's licence")
	}
	other := must(s.Issue(f.member))
	if err := s.Remove(f.member, other.ID, f.password); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateOrder(f.member, other.ID, ChargingTest); !errors.Is(err, ErrLicenseNotFound) {
		t.Fatalf("a removed licence: %v", err)
	}
	// A double click is the same order; the order is a test order at the list price.
	first := must(s.CreateOrder(f.member, f.licence.ID, ChargingTest))
	second := must(s.CreateOrder(f.member, f.licence.ID, ChargingTest))
	if first.ID != second.ID || first.Live || first.AmountMinor != 9900 || first.Currency != "USD" || first.State != OrderCreated || first.Kind != KindFirstYear {
		t.Fatalf("orders: %+v %+v", first, second)
	}
	if payment.LicenseFromReference(first.ID) != f.licence.ID {
		t.Fatalf("the order reference does not name its licence: %s", first.ID)
	}
	// After the order's lifetime a new click makes a new order.
	f.h.now += OrderLifetime + 1
	if third := must(s.CreateOrder(f.member, f.licence.ID, ChargingTest)); third.ID == first.ID {
		t.Fatal("an expired order was reused")
	}
}

func TestAPaidFactGrantsTheFirstYearOnce(t *testing.T) {
	f := newStore(t)
	s := f.s
	before := f.expires(f.licence.ID) // the free year still runs
	o := must(s.CreateOrder(f.member, f.licence.ID, ChargingTest))
	if err := s.AttachCheckout(o.ID, "stripe", "cs_1"); err != nil {
		t.Fatal(err)
	}
	if err := s.AttachCheckout(o.ID, "stripe", "cs_1"); err != nil {
		t.Fatal("the same checkout again must be a no-op")
	}
	if err := s.AttachCheckout(o.ID, "stripe", "cs_2"); err == nil {
		t.Fatal("a second checkout for one order")
	}
	o = must(s.GetOrder(o.ID))
	if o.State != OrderPending {
		t.Fatalf("state %s", o.State)
	}
	fact := paid(o, "pi_1")
	o = must(s.ApplyFact(o.ID, "stripe", "provider:stripe", fact))
	want := anniversary(before)
	if o.State != OrderPaid || f.expires(f.licence.ID) != want || o.GrantedFrom != before || o.GrantedTo != want || o.PreviousExpires != before {
		t.Fatalf("the year after the running free term: %+v, expires %d, want %d", o, f.expires(f.licence.ID), want)
	}
	// A replay, a duplicate event and the same fact through another reference: nothing more.
	for i := 0; i < 3; i++ {
		must(s.ApplyFact(o.ID, "stripe", "provider:stripe", fact))
	}
	if f.expires(f.licence.ID) != want {
		t.Fatal("a replay granted another year")
	}
	var payments int
	if err := s.DB().QueryRow("SELECT COUNT(*) FROM store_payments").Scan(&payments); err != nil || payments != 1 {
		t.Fatalf("payments recorded: %d", payments)
	}
	// The same payment id for another order is refused.
	l2 := must(s.Issue(f.member))
	o2 := must(s.CreateOrder(f.member, l2.ID, ChargingTest))
	if _, err := s.ApplyFact(o2.ID, "stripe", "provider:stripe", paid(o2, "pi_1")); err == nil {
		t.Fatal("one payment fulfilled two orders")
	}
	// A licence with a paid year: "Renew" is not built (the owner's questions 5 and 6).
	if next := must(s.CreateOrder(f.member, f.licence.ID, ChargingTest)); next.Kind != KindRenewal || next.ID == o.ID {
		t.Fatalf("a second year is a renewal: %+v", next)
	}
	// Events are stored once.
	if fresh, err := s.RecordEvent("stripe", "evt_1", "payment", "cs_1"); !fresh || err != nil {
		t.Fatalf("first event: %v %v", fresh, err)
	}
	if fresh, err := s.RecordEvent("stripe", "evt_1", "payment", "cs_1"); fresh || err != nil {
		t.Fatalf("replayed event: %v %v", fresh, err)
	}
}

func TestAPaidFactOnAnEndedLicenceStartsAtThePayment(t *testing.T) {
	f := newStore(t)
	f.h.now = f.expires(f.licence.ID) + 40*86400 // the free year ended 40 days ago
	o := must(f.s.CreateOrder(f.member, f.licence.ID, ChargingTest))
	o = must(f.s.ApplyFact(o.ID, "stripe", "provider:stripe", paid(o, "pi_9")))
	if o.GrantedFrom != f.h.now || f.expires(f.licence.ID) != anniversary(f.h.now) {
		t.Fatalf("an ended licence: %+v %d", o, f.expires(f.licence.ID))
	}
}

func TestFactsThatGrantNothing(t *testing.T) {
	f := newStore(t)
	s := f.s
	before := f.expires(f.licence.ID)
	o := must(s.CreateOrder(f.member, f.licence.ID, ChargingTest))
	// Not paid yet, then failed.
	o = must(s.ApplyFact(o.ID, "stripe", "provider:stripe", payment.Fact{Outcome: payment.OutcomeNotPaid, Status: payment.StatusPending}))
	if o.State != OrderCreated {
		t.Fatalf("pending fact: %s", o.State)
	}
	o = must(s.ApplyFact(o.ID, "stripe", "provider:stripe", payment.Fact{Outcome: payment.OutcomeNotPaid, Status: payment.StatusFailed}))
	if o.State != OrderFailed {
		t.Fatalf("failed fact: %s", o.State)
	}
	// A late payment of a failed order is still recorded and granted (the money arrived).
	o = must(s.ApplyFact(o.ID, "stripe", "provider:stripe", paid(o, "pi_late")))
	if o.State != OrderPaid || f.expires(f.licence.ID) == before {
		t.Fatalf("a late payment: %+v", o)
	}
	// A mismatch on another licence's order: the order is for the operator; nothing granted.
	l2 := must(s.Issue(f.member))
	o2 := must(s.CreateOrder(f.member, l2.ID, ChargingTest))
	e2 := f.expires(l2.ID)
	o2 = must(s.ApplyFact(o2.ID, "stripe", "provider:stripe", payment.Fact{Outcome: payment.OutcomeMismatch, Mismatch: "amount"}))
	if o2.State != OrderMismatch || f.expires(l2.ID) != e2 {
		t.Fatalf("mismatch: %+v", o2)
	}
	// Even a paid fact afterwards grants nothing: mismatch is final, for the operator.
	o2 = must(s.ApplyFact(o2.ID, "stripe", "provider:stripe", paid(o2, "pi_2")))
	if o2.State != OrderMismatch || f.expires(l2.ID) != e2 {
		t.Fatalf("paid after mismatch: %+v", o2)
	}
	// A fact for another licence or mode is refused outright.
	l3 := must(s.Issue(f.member))
	o3 := must(s.CreateOrder(f.member, l3.ID, ChargingTest))
	wrong := paid(o3, "pi_3")
	wrong.LicenseID = f.licence.ID
	if _, err := s.ApplyFact(o3.ID, "stripe", "provider:stripe", wrong); err == nil {
		t.Fatal("a fact for another licence")
	}
	wrong = paid(o3, "pi_3")
	wrong.Live = true
	if _, err := s.ApplyFact(o3.ID, "stripe", "provider:stripe", wrong); err == nil {
		t.Fatal("a live fact for a test order")
	}
	// A partial refund and a dispute are recorded for the operator only.
	part := paid(o3, "pi_3")
	part.Outcome, part.RefundedMinor, part.Disputed = payment.OutcomePartlyRefunded, 100, true
	o3 = must(s.ApplyFact(o3.ID, "stripe", "provider:stripe", part))
	listed := false
	for _, a := range must(s.AttentionOrders()) {
		listed = listed || a.Order.ID == o3.ID
	}
	if o3.State != OrderCreated || !listed {
		t.Fatalf("partial refund and dispute: %+v listed %v", o3, listed)
	}
}

func TestARefundedFirstYearIsRevoked(t *testing.T) {
	f := newStore(t)
	s := f.s
	a := strings.Repeat("a", 64)
	f.h.now = f.expires(f.licence.ID) + 86400 // the free year ended: taking the paid year back ends the licence
	o := must(s.CreateOrder(f.member, f.licence.ID, ChargingTest))
	o = must(s.ApplyFact(o.ID, "stripe", "provider:stripe", paid(o, "pi_1")))
	envelope := must(s.Activate(f.licence.Key, a, "a.example.com", "8.8.8.8"))
	var generation int64
	if err := s.DB().QueryRow("SELECT generation FROM licenses WHERE id=?", f.licence.ID).Scan(&generation); err != nil {
		t.Fatal(err)
	}
	// No refunds by policy; the operator's exceptional act needs a reason and keeps the attempt.
	if _, err := s.RequestRefund("operator:test", o.ID, " "); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("a refund without a reason: %v", err)
	}
	r := must(s.RequestRefund("operator:test", o.ID, "an exceptional act for a test"))
	if again := must(s.RequestRefund("operator:test", o.ID, "an exceptional act for a test")); again.ID != r.ID || r.Attempt != 0 {
		t.Fatalf("refund requests: %+v %+v", r, again)
	}
	// A definite refusal: the next request raises the attempt.
	if err := s.RefundAnswered(r.ID, "provider:stripe", payment.ErrProviderRefused); err != nil {
		t.Fatal(err)
	}
	r = must(s.RequestRefund("operator:test", o.ID, "again"))
	if r.Attempt != 1 {
		t.Fatalf("attempt after a refusal: %d", r.Attempt)
	}
	// Accepted: "being processed" until the provider confirms the refund.
	if err := s.RefundAnswered(r.ID, "provider:stripe", nil); err != nil {
		t.Fatal(err)
	}
	if refunds := must(s.Refunds(o.ID)); refunds[1].State != "sent" || refunds[1].Outcome != "being processed" {
		t.Fatalf("refunds: %+v", refunds)
	}
	if revoked, _, _ := s.LicenceRevocation(f.licence.ID); revoked {
		t.Fatal("revoked before the provider confirmed the refund")
	}
	refunded := paid(o, "pi_1")
	refunded.Outcome, refunded.RefundedMinor = payment.OutcomeRefunded, o.AmountMinor
	o = must(s.ApplyFact(o.ID, "stripe", "provider:stripe", refunded))
	revoked, why, err := s.LicenceRevocation(f.licence.ID)
	if o.State != OrderRefunded || !revoked || why != "refunded" || err != nil {
		t.Fatalf("after the refund: %+v %v %s %v", o, revoked, why, err)
	}
	var after struct {
		generation int64
		server     *string
		expires    int64
	}
	if err := s.DB().QueryRow("SELECT generation,server_id,expires FROM licenses WHERE id=?", f.licence.ID).Scan(&after.generation, &after.server, &after.expires); err != nil {
		t.Fatal(err)
	}
	if after.generation != generation+1 || after.server != nil || after.expires > f.h.now {
		t.Fatalf("the revocation: %+v (generation before %d)", after, generation)
	}
	if refunds := must(s.Refunds(o.ID)); refunds[1].State != "done" {
		t.Fatalf("refund state: %+v", refunds)
	}
	// The panel's refresh is rejected (never "database behind"); the key does not bind again;
	// the member's actions answer license_revoked, except removing it.
	if _, err := s.Refresh(f.licence.ID, a, envelope.ActivationToken, "8.8.8.8"); !errors.Is(err, ErrInvalidActivation) {
		t.Fatalf("refresh after the refund: %v", err)
	}
	if _, err := s.Refresh(f.licence.ID, a, s.activationToken(f.licence.ID, a, generation+1), "8.8.8.8"); !errors.Is(err, ErrInvalidActivation) {
		t.Fatalf("a token for the new generation: %v", err)
	}
	if _, err := s.Activate(f.licence.Key, a, "a.example.com", "8.8.8.8"); !errors.Is(err, ErrInvalidLicense) {
		t.Fatalf("activate after the refund: %v", err)
	}
	if err := s.Renew(f.member, f.licence.ID, f.password); !errors.Is(err, ErrLicenseRevoked) {
		t.Fatalf("renew: %v", err)
	}
	if _, err := s.Reveal(f.member, f.licence.ID, f.password); !errors.Is(err, ErrLicenseRevoked) {
		t.Fatalf("reveal: %v", err)
	}
	if _, err := s.CreateOrder(f.member, f.licence.ID, ChargingTest); !errors.Is(err, ErrLicenseRevoked) {
		t.Fatalf("buying a revoked licence: %v", err)
	}
	if err := s.ExtendByHand("operator:test", "gift", f.licence.ID, 30); !errors.Is(err, ErrLicenseRevoked) {
		t.Fatalf("extending a revoked licence: %v", err)
	}
	if _, err := s.RequestRefund("operator:test", o.ID, "again"); !errors.Is(err, ErrRefundNotPossible) {
		t.Fatalf("a second refund: %v", err)
	}
	if err := s.Remove(f.member, f.licence.ID, f.password); err != nil {
		t.Fatalf("removing a revoked licence: %v", err)
	}
}

func TestRefundRules(t *testing.T) {
	f := newStore(t)
	s := f.s
	o := must(s.CreateOrder(f.member, f.licence.ID, ChargingTest))
	if _, err := s.RequestRefund("operator:test", o.ID, "test"); !errors.Is(err, ErrRefundNotPossible) {
		t.Fatalf("an unpaid order: %v", err)
	}
	o = must(s.ApplyFact(o.ID, "stripe", "provider:stripe", paid(o, "pi_1")))
	f.h.now += 400 * 86400 // no window: the operator's act is not bound to 30 days (there are no member refunds)
	r := must(s.RequestRefund("operator:test", o.ID, "test"))
	// Unknown outcomes keep the attempt; a final answer ends the request for the operator.
	for _, answer := range []error{payment.ErrProviderUnavailable, payment.ErrProviderBusy} {
		if err := s.RefundAnswered(r.ID, "provider:stripe", answer); err != nil {
			t.Fatal(err)
		}
		if again := must(s.RequestRefund("operator:test", o.ID, "test")); again.ID != r.ID || again.Attempt != r.Attempt {
			t.Fatalf("after %v the same attempt must be asked again: %+v", answer, again)
		}
	}
	if err := s.RefundAnswered(r.ID, "provider:stripe", payment.ErrDisputed); err != nil {
		t.Fatal(err)
	}
	if refunds := must(s.Refunds(o.ID)); refunds[0].State != "final" || refunds[0].Outcome != "disputed" {
		t.Fatalf("disputed: %+v", refunds)
	}
	if _, err := s.RequestRefund("operator:test", o.ID, "test"); !errors.Is(err, ErrRefundNotPossible) {
		t.Fatalf("after a final answer: %v", err)
	}
	if revoked, _, _ := s.LicenceRevocation(f.licence.ID); revoked {
		t.Fatal("a disputed refund revoked the licence")
	}
}

func TestOperatorStoreActsAreAudited(t *testing.T) {
	f := newStore(t)
	s := f.s
	if err := s.SetPrice("operator:test", "", "USD", 12900, f.h.now); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("a price without a reason: %v", err)
	}
	if err := s.SetPrice("operator:test", "test price", "USD", 12900, f.h.now+86400); err != nil {
		t.Fatal(err)
	}
	if p := must(s.CurrentPrice("USD")); p.AmountMinor != 9900 {
		t.Fatalf("a future price is not in force yet: %+v", p)
	}
	f.h.now += 86400
	if p := must(s.CurrentPrice("USD")); p.AmountMinor != 12900 {
		t.Fatalf("the new price: %+v", p)
	}
	before := f.expires(f.licence.ID)
	if err := s.ExtendByHand("operator:test", "a gift for a test", f.licence.ID, 30); err != nil {
		t.Fatal(err)
	}
	if f.expires(f.licence.ID) != before+30*86400 {
		t.Fatal("the gift")
	}
	actions := map[string]int{}
	for _, l := range must(s.Audit("")) {
		actions[l.Action]++
		if l.Actor == "" {
			t.Fatalf("a line without an actor: %+v", l)
		}
	}
	for _, want := range []string{"store_created", "price_set", "test_account", "extended_by_hand"} {
		if actions[want] == 0 {
			t.Fatalf("%s not in the log: %v", want, actions)
		}
	}
}

// The store's licence changes keep the generation rule (third look at #361, 1): granting a
// year and a gift only move the end; a refund revokes with a raise, in one statement.
func TestStoreChangesKeepTheGenerationRule(t *testing.T) {
	f := newStore(t)
	s := f.s
	f.h.now = f.expires(f.licence.ID) + 86400 // the free year ended: a refund leaves nothing, the licence ends
	steps := []struct {
		name string
		run  func() error
	}{}
	var o Order
	steps = append(steps,
		struct {
			name string
			run  func() error
		}{"grant a first year", func() error {
			o = must(s.CreateOrder(f.member, f.licence.ID, ChargingTest))
			_, err := s.ApplyFact(o.ID, "stripe", "provider:stripe", paid(o, "pi_1"))
			return err
		}},
		struct {
			name string
			run  func() error
		}{"refund and revoke", func() error {
			fact := paid(o, "pi_1")
			fact.Outcome = payment.OutcomeRefunded
			_, err := s.ApplyFact(o.ID, "stripe", "provider:stripe", fact)
			return err
		}},
		struct {
			name string
			run  func() error
		}{"reinstate", func() error { return s.Reinstate("operator:test", f.licence.ID, "by mistake") }},
		struct {
			name string
			run  func() error
		}{"extend by hand", func() error { return s.ExtendByHand("operator:test", "gift", f.licence.ID, 10) }},
	)
	for _, step := range steps {
		before := licenceStates(t, s.DB())
		if err := step.run(); err != nil {
			t.Fatalf("%s: %v", step.name, err)
		}
		after := licenceStates(t, s.DB())
		checkGenerationInvariant(t, step.name, before, after)
		if (step.name == "refund and revoke" || step.name == "reinstate") && after[f.licence.ID].generation != before[f.licence.ID].generation+1 {
			t.Fatalf("%s did not raise the generation", step.name)
		}
	}
}

// The owner's rule of 23 November 2026: no grace. A renewal before the end continues from the
// old end at the locked price; after the end it is a new purchase at the list price from the
// payment. A renewal refunded at the provider takes back exactly the year it gave.
func TestRenewalByTheOwnersRule(t *testing.T) {
	f := newStore(t)
	s := f.s
	first := must(s.CreateOrder(f.member, f.licence.ID, ChargingTest))
	first = must(s.ApplyFact(first.ID, "stripe", "provider:stripe", paid(first, "pi_1")))
	end := f.expires(f.licence.ID)
	// The list price rises; a renewal before the end keeps the price paid.
	if err := s.SetPrice("operator:test", "test rise", "USD", 12900, f.h.now); err != nil {
		t.Fatal(err)
	}
	q := must(s.QuoteFor(f.member, f.licence.ID))
	if q.Kind != KindRenewal || q.AmountMinor != 9900 || !q.Locked || q.From != end || q.To != anniversary(end) {
		t.Fatalf("a renewal before the end: %+v", q)
	}
	renewal := must(s.CreateOrder(f.member, f.licence.ID, ChargingTest))
	if renewal.Kind != KindRenewal || renewal.AmountMinor != 9900 {
		t.Fatalf("the renewal order: %+v", renewal)
	}
	renewal = must(s.ApplyFact(renewal.ID, "stripe", "provider:stripe", paid(renewal, "pi_2")))
	if renewal.GrantedFrom != end || f.expires(f.licence.ID) != anniversary(end) {
		t.Fatalf("from the old end: %+v", renewal)
	}
	// A renewal refunded at the provider (the operator's exceptional act): the year goes back.
	back := paid(renewal, "pi_2")
	back.Outcome = payment.OutcomeRefunded
	renewal = must(s.ApplyFact(renewal.ID, "stripe", "provider:stripe", back))
	if renewal.State != OrderRefunded || f.expires(f.licence.ID) != end {
		t.Fatalf("a refunded renewal: %+v, expires %d, want %d", renewal, f.expires(f.licence.ID), end)
	}
	if revoked, _, _ := s.LicenceRevocation(f.licence.ID); revoked {
		t.Fatal("a refunded renewal revoked the licence")
	}
	// After the end: the list price, and the year starts at the payment.
	f.h.now = end + 10*86400
	q = must(s.QuoteFor(f.member, f.licence.ID))
	if q.Kind != KindRenewal || q.AmountMinor != 12900 || q.Locked || q.From != f.h.now {
		t.Fatalf("after the end: %+v", q)
	}
	late := must(s.CreateOrder(f.member, f.licence.ID, ChargingTest))
	late = must(s.ApplyFact(late.ID, "stripe", "provider:stripe", paid(late, "pi_3")))
	if late.AmountMinor != 12900 || late.GrantedFrom != f.h.now || f.expires(f.licence.ID) != anniversary(f.h.now) {
		t.Fatalf("a purchase after the end: %+v", late)
	}
}
