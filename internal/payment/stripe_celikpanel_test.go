package payment

// CelikPanel's additions to the copied Stripe adapter (D-047, amended 2026-11-23), each against
// the fake server: a one-time payment for every purchase, a subscription only by the member's
// choice with no bonus months, events that carry identifiers only, Confirm as the one way to
// learn what happened (checked against the order), a paid amount only when money arrived,
// stable idempotency keys, payment states that only move forward, and no test ever reaching
// Stripe whatever the address's spelling.

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

var testNow = time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)

func oneTimeCheckout(t *testing.T) Checkout {
	return Checkout{LicenseID: licenseID, Order: mustReference(t, licenseID, false), Amount: "99.00", Currency: "USD",
		Period: PeriodYearly, PayerEmail: "payer@example.com", Language: "tr",
		SuccessURL: "https://account.example/done", CancelURL: "https://account.example/back"}
}

func sessionServer(t *testing.T) *fakeServer {
	fake := newFakeServer(t)
	fake.on(http.MethodPost, "/v1/checkout/sessions", func(recordedRequest) (int, string) {
		return 200, `{"id":"cs_test_1","url":"https://checkout.stripe.com/c/pay/cs_test_1"}`
	})
	return fake
}

func TestStripeEveryPurchaseIsAOneTimePayment(t *testing.T) {
	fake := sessionServer(t)
	provider := &Stripe{BaseURL: fake.server.URL, Now: func() time.Time { return testNow }}
	checkout := oneTimeCheckout(t) // the zero value of AutomaticRenewal: no subscription by default
	if _, err := provider.StartCheckout(context.Background(), stripeCredentials(t), checkout); err != nil {
		t.Fatal(err)
	}
	sent := fake.last()
	form, _ := url.ParseQuery(string(sent.Body))
	for field, value := range map[string]string{
		"mode": "payment", "payment_method_types[0]": "card", "client_reference_id": checkout.Order,
		"line_items[0][price_data][unit_amount]": "9900", "line_items[0][price_data][currency]": "usd",
		"line_items[0][quantity]": "1", "metadata[license_id]": licenseID, "metadata[order]": checkout.Order,
		"payment_intent_data[metadata][license_id]": licenseID, "payment_intent_data[metadata][order]": checkout.Order,
		"line_items[0][price_data][product_data][name]": "CelikPanel lisansı, bir sunucu, bir yıl",
	} {
		if form.Get(field) != value {
			t.Errorf("%s = %q, want %q", field, form.Get(field), value)
		}
	}
	// Nothing is kept for a later charge and nothing recurs.
	for field := range form {
		if strings.Contains(field, "recurring") || strings.HasPrefix(field, "subscription_data") ||
			strings.Contains(field, "setup_future_usage") || strings.Contains(field, "trial") ||
			field == "payment_method_collection" || field == "customer_creation" {
			t.Errorf("a one-time payment sends %s", field)
		}
	}
	if sent.Header.Get("Idempotency-Key") != checkout.Order || !strings.HasPrefix(checkout.Order, "CPC") {
		t.Fatalf("a retry must be the same checkout: %q", sent.Header.Get("Idempotency-Key"))
	}
	refused := map[string]Checkout{}
	c := checkout
	c.SuccessURL = "http://account.example/done"
	refused["plain http return"] = c
	c = checkout
	c.Order = mustReference(t, licenseID, true) // an S order is a subscription
	refused["a subscription order as a one-time payment"] = c
	c = checkout
	c.Order, _ = RenewalReference(licenseID, testNow)
	refused["a renewal reference"] = c
	c = checkout
	c.Order = mustReference(t, "ffffffffffffffffffffffffffffffff", false)
	refused["an order for another licence"] = c
	c = checkout
	c.Amount = "0"
	refused["nothing to pay"] = c
	before := fake.count()
	for name, c := range refused {
		if _, err := provider.StartCheckout(context.Background(), stripeCredentials(t), c); !errors.Is(err, ErrCheckoutIncomplete) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if fake.count() != before {
		t.Fatal("a refused checkout reached the provider")
	}
}

func TestStripeSubscriptionOnlyByChoiceWithoutBonusMonths(t *testing.T) {
	fake := sessionServer(t)
	provider := &Stripe{BaseURL: fake.server.URL, Now: func() time.Time { return testNow }}
	checkout := oneTimeCheckout(t)
	checkout.AutomaticRenewal = true
	checkout.Order = mustReference(t, licenseID, true)
	// No first-charge date, or one too close: refused, so the paid year is never charged twice.
	for _, first := range []time.Time{{}, testNow.Add(time.Hour)} {
		checkout.FirstChargeAt = first
		if _, err := provider.StartCheckout(context.Background(), stripeCredentials(t), checkout); !errors.Is(err, ErrCheckoutIncomplete) {
			t.Fatalf("first charge %v: %v", first, err)
		}
	}
	licenceEnd := testNow.AddDate(0, 0, 200)
	checkout.FirstChargeAt = licenceEnd
	if _, err := provider.StartCheckout(context.Background(), stripeCredentials(t), checkout); err != nil {
		t.Fatal(err)
	}
	form, _ := url.ParseQuery(string(fake.last().Body))
	if form.Get("mode") != "subscription" || form.Get("payment_method_collection") != "always" {
		t.Fatalf("subscription: %v", form)
	}
	// One year at a time, the list price, the first charge exactly the licence end: no bonus.
	if form.Get("line_items[0][price_data][recurring][interval]") != "year" || form.Get("line_items[0][price_data][recurring][interval_count]") != "1" ||
		form.Get("line_items[0][price_data][unit_amount]") != "9900" || form.Get("subscription_data[trial_end]") != itoa64(licenceEnd.Unix()) {
		t.Fatalf("a renewal must be one year at the list price from the licence end: %v", form)
	}
	for field := range form {
		if strings.Contains(field, "trial_period_days") || strings.Contains(field, "coupon") || strings.Contains(field, "discount") ||
			strings.HasPrefix(field, "line_items[1]") || strings.Contains(field, "billing_cycle_anchor") {
			t.Errorf("a subscription sends %s", field)
		}
	}
}

func itoa64(n int64) string { return strconv.FormatInt(n, 10) }

const oneTimeOrder = "CPC" + licenseID + "abcdefabcdef"
const renewalOrder = "CPS" + licenseID + "abcdefabcdef"

// intentJSON is a PaymentIntent with its latest charge expanded.
func intentJSON(status string, amount, refunded int64, fully, disputed bool) string {
	paid, received := "true", amount
	if status != "succeeded" {
		paid, received = "false", 0
	}
	return `{"id":"pi_1","status":"` + status + `","amount":` + itoa64(amount) + `,"amount_received":` + itoa64(received) +
		`,"currency":"usd","customer":"cus_1","metadata":{"order":"` + oneTimeOrder + `","license_id":"` + licenseID + `"},` +
		`"latest_charge":{"id":"ch_1","payment_intent":"pi_1","paid":` + paid + `,"amount":` + itoa64(amount) +
		`,"amount_captured":` + itoa64(received) + `,"amount_refunded":` + itoa64(refunded) +
		`,"refunded":` + boolText(fully) + `,"disputed":` + boolText(disputed) + `,"currency":"usd"}}`
}

// invoiceJSON is a renewal invoice; withMetadata puts the subscription's metadata on it (basil).
func invoiceJSON(id, status string, paid int64, withMetadata bool) string {
	metadata := `{}`
	if withMetadata {
		metadata = `{"license_id":"` + licenseID + `","order":"` + renewalOrder + `"}`
	}
	return `{"id":"` + id + `","object":"invoice","status":"` + status + `","amount_paid":` + itoa64(paid) + `,"currency":"usd",` +
		`"customer":"cus_1","parent":{"type":"subscription_details","subscription_details":{"subscription":"sub_1","metadata":` + metadata + `}}}`
}

func boolText(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func sessionAnswer(mode, status, paymentStatus, order, intent string) func(recordedRequest) (int, string) {
	return func(r recordedRequest) (int, string) {
		if query, _ := url.ParseQuery(r.Query); query.Get("expand[]") != "payment_intent.latest_charge" {
			return 400, `{"error":{}}`
		}
		return 200, `{"id":"cs_1","mode":"` + mode + `","status":"` + status + `","payment_status":"` + paymentStatus +
			`","client_reference_id":"` + order + `","metadata":{"license_id":"` + licenseID + `"},"subscription":"sub_1","payment_intent":` + intent + `}`
	}
}

var expectOneTime = Expected{Kind: OrderOneTime, Order: oneTimeOrder, LicenseID: licenseID, AmountMinor: 9900, Currency: "USD"}

// Review of #361, 2: every shape says what really happened; a paid amount only when money arrived.
func TestStripeFetchOrderIsTheTruth(t *testing.T) {
	fake := newFakeServer(t)
	fake.on(http.MethodGet, "/v1/disputes", disputeAnswer("needs_response"))
	provider := &Stripe{BaseURL: fake.server.URL, Now: func() time.Time { return testNow }}
	confirm := func(reference string, expected Expected) (Fact, error) {
		return provider.Confirm(context.Background(), stripeCredentials(t), reference, expected)
	}
	for _, c := range []struct {
		name     string
		handler  func(recordedRequest) (int, string)
		outcome  Outcome
		paid     int64
		refunded int64
		disputed bool
	}{
		{"open", sessionAnswer("payment", "open", "unpaid", oneTimeOrder, "null"), OutcomeNotPaid, 0, 0, false},
		{"delayed method", sessionAnswer("payment", "complete", "unpaid", oneTimeOrder, `"pi_1"`), OutcomeNotPaid, 0, 0, false},
		{"expired", sessionAnswer("payment", "expired", "unpaid", oneTimeOrder, "null"), OutcomeNotPaid, 0, 0, false},
		{"paid", sessionAnswer("payment", "complete", "paid", oneTimeOrder, intentJSON("succeeded", 9900, 0, false, false)), OutcomePaid, 9900, 0, false},
		{"refunded", sessionAnswer("payment", "complete", "paid", oneTimeOrder, intentJSON("succeeded", 9900, 9900, true, false)), OutcomeRefunded, 9900, 9900, false},
		{"partly refunded", sessionAnswer("payment", "complete", "paid", oneTimeOrder, intentJSON("succeeded", 9900, 1000, false, false)), OutcomePartlyRefunded, 9900, 1000, false},
		{"disputed", sessionAnswer("payment", "complete", "paid", oneTimeOrder, intentJSON("succeeded", 9900, 0, false, true)), OutcomePaid, 9900, 0, true},
	} {
		fake.on(http.MethodGet, "/v1/checkout/sessions/cs_1", c.handler)
		fact, err := confirm("cs_1", expectOneTime)
		if err != nil || fact.Outcome != c.outcome || fact.PaidMinor != c.paid || fact.RefundedMinor != c.refunded ||
			fact.Disputed != c.disputed || fact.Kind != OrderOneTime || fact.LicenseID != licenseID {
			t.Errorf("%s: %+v %v", c.name, fact, err)
		}
		if c.outcome == OutcomeNotPaid && fact.PaidMinor != 0 {
			t.Errorf("%s: an amount without money", c.name)
		}
	}
	fake.on(http.MethodGet, "/v1/payment_intents/pi_1", func(recordedRequest) (int, string) {
		return 200, intentJSON("canceled", 9900, 0, false, false)
	})
	if fact, err := confirm("pi_1", expectOneTime); err != nil || fact.Outcome != OutcomeNotPaid || fact.Status != StatusFailed || fact.PaidMinor != 0 {
		t.Fatalf("a cancelled PaymentIntent: %+v %v", fact, err)
	}

	// A subscription set-up moves no money: never paid, whatever Stripe calls the session.
	fake.on(http.MethodGet, "/v1/checkout/sessions/cs_2", func(recordedRequest) (int, string) {
		return 200, `{"id":"cs_2","mode":"subscription","status":"complete","payment_status":"no_payment_required",` +
			`"client_reference_id":"` + renewalOrder + `","subscription":"sub_1","customer":"cus_1"}`
	})
	setup := Expected{Kind: OrderSubscriptionSetup, Order: renewalOrder, LicenseID: licenseID}
	if fact, err := confirm("cs_2", setup); err != nil || fact.Outcome != OutcomeSetUp || fact.PaidMinor != 0 || fact.Status == StatusSucceeded ||
		fact.Subscription != "sub_1" {
		t.Fatalf("a subscription set up without a charge: %+v %v", fact, err)
	}
	setup.AmountMinor, setup.Currency = 9900, "USD"
	if fact, _ := confirm("cs_2", setup); fact.Outcome == OutcomePaid {
		t.Fatal("a set-up was read as a paid year")
	}

	// Renewal invoices name their licence and order from Stripe's own objects.
	renewal := Expected{Kind: OrderRenewalInvoice, Order: renewalOrder, LicenseID: licenseID, AmountMinor: 9900, Currency: "USD"}
	fake.on(http.MethodGet, "/v1/invoice_payments", func(r recordedRequest) (int, string) {
		query, _ := url.ParseQuery(r.Query)
		switch query.Get("invoice") {
		case "in_charge":
			return 200, `{"data":[{"payment":{"type":"charge","charge":"ch_9"}}]}`
		case "in_none":
			return 200, `{"data":[]}`
		}
		return 200, `{"data":[{"payment":{"type":"payment_intent","payment_intent":"pi_1"}}]}`
	})
	fake.on(http.MethodGet, "/v1/payment_intents/pi_1", func(recordedRequest) (int, string) {
		return 200, intentJSON("succeeded", 9900, 0, false, false)
	})
	fake.on(http.MethodGet, "/v1/charges/ch_9", func(recordedRequest) (int, string) {
		return 200, `{"id":"ch_9","paid":true,"amount":9900,"amount_captured":9900,"amount_refunded":9900,"refunded":true,"currency":"usd"}`
	})
	fake.on(http.MethodGet, "/v1/subscriptions/sub_1", func(recordedRequest) (int, string) {
		return 200, `{"id":"sub_1","status":"active","customer":"cus_1","metadata":{"license_id":"` + licenseID + `","order":"` + renewalOrder + `"}}`
	})
	for _, c := range []struct {
		name    string
		invoice string
		outcome Outcome
		err     error
	}{
		{"metadata on the invoice", invoiceJSON("in_1", "paid", 9900, true), OutcomePaid, nil},
		{"metadata only on the subscription", invoiceJSON("in_1", "paid", 9900, false), OutcomePaid, nil},
		{"paid with a charge only, refunded", invoiceJSON("in_charge", "paid", 9900, true), OutcomeRefunded, nil},
		{"marked paid with nothing paid", invoiceJSON("in_1", "paid", 0, true), "", ErrUnexpectedState},
		{"no payment listed", invoiceJSON("in_none", "paid", 9900, true), "", ErrUnexpectedState},
		{"still open", invoiceJSON("in_1", "open", 0, true), OutcomeNotPaid, nil},
	} {
		id := strings.SplitN(strings.TrimPrefix(c.invoice, `{"id":"`), `"`, 2)[0]
		body := c.invoice
		fake.on(http.MethodGet, "/v1/invoices/"+id, func(recordedRequest) (int, string) { return 200, body })
		fact, err := confirm(id, renewal)
		if c.err != nil {
			if !errors.Is(err, c.err) || fact.Outcome == OutcomePaid {
				t.Errorf("%s: %+v %v", c.name, fact, err)
			}
			continue
		}
		if err != nil || fact.Outcome != c.outcome || fact.LicenseID != licenseID || fact.Order != renewalOrder || fact.Kind != OrderRenewalInvoice {
			t.Errorf("%s: %+v %v", c.name, fact, err)
		}
		if c.outcome == OutcomePaid && fact.PaidMinor != 9900 {
			t.Errorf("%s: paid %d", c.name, fact.PaidMinor)
		}
	}
	if _, err := confirm("xx_1", expectOneTime); !errors.Is(err, ErrNotSupported) {
		t.Fatalf("an unknown object: %v", err)
	}
}

// Review of #361, 3: a body is only a hint; Confirm checks the provider's object against the order.
func TestStripeConfirmChecksTheOrder(t *testing.T) {
	fake := newFakeServer(t)
	provider := &Stripe{BaseURL: fake.server.URL, Now: func() time.Time { return testNow }}
	fake.on(http.MethodGet, "/v1/checkout/sessions/cs_1", sessionAnswer("payment", "complete", "paid", oneTimeOrder,
		intentJSON("succeeded", 9900, 0, false, false)))
	for name, c := range map[string]struct {
		expected Expected
		mismatch string
	}{
		"another kind":     {Expected{Kind: OrderRenewalInvoice, Order: oneTimeOrder, LicenseID: licenseID, AmountMinor: 9900, Currency: "USD"}, "kind"},
		"another order":    {Expected{Kind: OrderOneTime, Order: "CPC" + licenseID + "000000000000", LicenseID: licenseID, AmountMinor: 9900, Currency: "USD"}, "order"},
		"another licence":  {Expected{Kind: OrderOneTime, Order: oneTimeOrder, LicenseID: strings.Repeat("f", 32), AmountMinor: 9900, Currency: "USD"}, "licence"},
		"another amount":   {Expected{Kind: OrderOneTime, Order: oneTimeOrder, LicenseID: licenseID, AmountMinor: 19900, Currency: "USD"}, "amount"},
		"another currency": {Expected{Kind: OrderOneTime, Order: oneTimeOrder, LicenseID: licenseID, AmountMinor: 9900, Currency: "EUR"}, "currency"},
	} {
		fact, err := provider.Confirm(context.Background(), stripeCredentials(t), "cs_1", c.expected)
		if err != nil || fact.Outcome != OutcomeMismatch || fact.Mismatch != c.mismatch {
			t.Errorf("%s: %+v %v", name, fact, err)
		}
	}
	if _, err := provider.Confirm(context.Background(), stripeCredentials(t), "cs_1", Expected{Kind: OrderOneTime}); !errors.Is(err, ErrCheckoutIncomplete) {
		t.Fatalf("an empty expectation must not match an empty answer: %v", err)
	}

	// A signed body that says "paid" for this session, while Stripe says it is still open: the
	// event offers no status to act on, and Confirm answers not paid.
	fake.on(http.MethodGet, "/v1/checkout/sessions/cs_1", sessionAnswer("payment", "open", "unpaid", oneTimeOrder, "null"))
	body := `{"id":"evt_9","type":"checkout.session.completed","data":{"object":{"id":"cs_1","mode":"payment","status":"complete",` +
		`"payment_status":"paid","client_reference_id":"` + oneTimeOrder + `","amount_total":9900,"currency":"usd"}}}`
	event, err := stripeNotification(t, provider, body, true)
	if err != nil || event.Reference != "cs_1" || event.Kind != EventPayment {
		t.Fatalf("event: %+v %v", event, err)
	}
	if fact, err := provider.Confirm(context.Background(), stripeCredentials(t), event.Reference, expectOneTime); err != nil || fact.Outcome != OutcomeNotPaid {
		t.Fatalf("the fetched state must win over the body: %+v %v", fact, err)
	}
	// A replay is the same hint with the same event id (the store's duplicate key), and
	// confirming it again gives the same fact.
	again, err := stripeNotification(t, provider, body, true)
	if err != nil || again != event {
		t.Fatalf("a replay: %+v %v", again, err)
	}
	// A forged body (another secret) is refused before anything is read.
	forged := map[string][]string{"Stripe-Signature": {signStripe(body, "whsec_forged", testNow)}}
	if _, err := provider.ParseNotification(context.Background(), stripeCredentials(t), Notification{Headers: forged, Body: []byte(body)}); !errors.Is(err, ErrInvalidNotification) {
		t.Fatalf("a forged body: %v", err)
	}
	// A body naming one licence while Stripe's session belongs to another: mismatch.
	fake.on(http.MethodGet, "/v1/checkout/sessions/cs_1", func(recordedRequest) (int, string) {
		return 200, `{"id":"cs_1","mode":"payment","status":"complete","payment_status":"paid","client_reference_id":"` + oneTimeOrder +
			`","metadata":{"license_id":"` + strings.Repeat("e", 32) + `"},"payment_intent":` + intentJSON("succeeded", 9900, 0, false, false) + `}`
	})
	if fact, _ := provider.Confirm(context.Background(), stripeCredentials(t), "cs_1", expectOneTime); fact.Outcome != OutcomeMismatch || fact.Mismatch != "licence" {
		t.Fatalf("a session for another licence: %+v", fact)
	}
}

func TestStripeConfirmRenewal(t *testing.T) {
	fake := newFakeServer(t)
	provider := &Stripe{BaseURL: fake.server.URL}
	fake.on(http.MethodGet, "/v1/subscriptions/sub_1", func(recordedRequest) (int, string) {
		return 200, `{"id":"sub_1","status":"active","cancel_at_period_end":true,"customer":"cus_1","metadata":{"license_id":"` + licenseID + `"}}`
	})
	fact, err := provider.ConfirmRenewal(context.Background(), stripeCredentials(t), "sub_1", licenseID, false)
	if err != nil || fact.Active || !fact.CancelAtPeriodEnd || fact.Mismatch {
		t.Fatalf("renewal turned off at the period end: %+v %v", fact, err)
	}
	if fact, _ := provider.ConfirmRenewal(context.Background(), stripeCredentials(t), "sub_1", strings.Repeat("f", 32), false); !fact.Mismatch || fact.Active {
		t.Fatalf("another licence's subscription: %+v", fact)
	}
}

// stripeCache is a fake that answers like Stripe's idempotency layer: the first answer for a key
// is kept and returned again for that key without running the handler.
type stripeCache struct {
	mu      sync.Mutex
	answers map[string][2]string
	runs    int
}

func (c *stripeCache) wrap(handler func(recordedRequest) (int, string)) func(recordedRequest) (int, string) {
	return func(r recordedRequest) (int, string) {
		c.mu.Lock()
		defer c.mu.Unlock()
		key := r.Header.Get("Idempotency-Key")
		if answer, ok := c.answers[key]; ok {
			status, _ := strconv.Atoi(answer[0])
			return status, answer[1]
		}
		c.runs++
		status, body := handler(r)
		c.answers[key] = [2]string{strconv.Itoa(status), body}
		return status, body
	}
}

// Review of #361, 8, and the second review, 3: keys name the payment and the operation; a
// definite refusal is asked again with the next attempt; nothing goes out without the confirm step.
func TestStripeRefundOfAOneTimePayment(t *testing.T) {
	fake := newFakeServer(t)
	cache := &stripeCache{answers: map[string][2]string{}}
	balance := false
	fake.on(http.MethodPost, "/v1/refunds", cache.wrap(func(r recordedRequest) (int, string) {
		form, _ := url.ParseQuery(string(r.Body))
		if form.Get("payment_intent") != "pi_1" || form.Get("amount") != "9900" {
			return 400, `{"error":{"code":"parameter_invalid"}}`
		}
		if !balance {
			return 400, `{"error":{"code":"balance_insufficient"}}`
		}
		return 200, `{"id":"re_1","status":"pending"}`
	}))
	fake.on(http.MethodGet, "/v1/checkout/sessions/cs_1", sessionAnswer("payment", "complete", "paid", oneTimeOrder, intentJSON("succeeded", 9900, 0, false, false)))
	fake.on(http.MethodGet, "/v1/checkout/sessions/cs_open", func(recordedRequest) (int, string) {
		return 200, `{"id":"cs_open","mode":"payment","status":"open","payment_status":"unpaid","client_reference_id":"` + oneTimeOrder + `"}`
	})
	provider := &Stripe{BaseURL: fake.server.URL}
	request := RefundRequest{Reference: "cs_1", Expected: expectOneTime}
	if err := provider.Refund(context.Background(), stripeCredentials(t), request); !errors.Is(err, ErrProviderRefused) {
		t.Fatalf("the first try is refused: %v", err)
	}
	if key := fake.last().Header.Get("Idempotency-Key"); key != "refund-pi_1-9900usd-a0" {
		t.Fatalf("key %q", key)
	}
	balance = true
	// The same attempt (an unknown outcome retried) is the same key: Stripe answers once.
	if err := provider.Refund(context.Background(), stripeCredentials(t), request); !errors.Is(err, ErrProviderRefused) || cache.runs != 1 {
		t.Fatalf("a retry of the same attempt must be the same request: %v, %d runs", err, cache.runs)
	}
	// After a definite refusal the store asks again with the next attempt, and Stripe runs it.
	request.Attempt = 1
	if err := provider.Refund(context.Background(), stripeCredentials(t), request); err != nil || cache.runs != 2 {
		t.Fatalf("the next attempt: %v, %d runs", err, cache.runs)
	}
	sent := fake.last()
	form, _ := url.ParseQuery(string(sent.Body))
	if sent.Header.Get("Idempotency-Key") != "refund-pi_1-9900usd-a1" || form.Get("metadata[license_id]") != licenseID ||
		form.Get("metadata[order]") != oneTimeOrder || form.Get("payment_intent") != "pi_1" {
		t.Fatalf("refund request: %v %v", sent.Header, form)
	}
	refunds := cache.runs
	request.Reference, request.Attempt = "cs_open", 2
	if err := provider.Refund(context.Background(), stripeCredentials(t), request); !errors.Is(err, ErrNothingPaid) || cache.runs != refunds {
		t.Fatalf("refunding an unpaid checkout: %v", err)
	}
	for name, bad := range map[string]RefundRequest{
		"no order":      {Reference: "cs_1", Expected: Expected{Kind: OrderOneTime, LicenseID: licenseID, AmountMinor: 9900, Currency: "USD"}},
		"no amount":     {Reference: "cs_1", Expected: Expected{Kind: OrderOneTime, Order: oneTimeOrder, LicenseID: licenseID, Currency: "USD"}},
		"unknown thing": {Reference: "xx_1", Expected: expectOneTime},
	} {
		if err := provider.Refund(context.Background(), stripeCredentials(t), bad); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if cache.runs != refunds {
		t.Fatal("a refused request reached the refund call")
	}
}

// Second review of #361, 3: a refund acts only on the order's own, fully paid payment, and its
// final answers are distinct so a store never loops; a 409 is not a refusal.
func TestStripeRefundOnlyAfterConfirm(t *testing.T) {
	fake := newFakeServer(t)
	fake.on(http.MethodGet, "/v1/disputes", disputeAnswer("needs_response"))
	refunds := 0
	conflict := false
	fake.on(http.MethodPost, "/v1/refunds", func(recordedRequest) (int, string) {
		refunds++
		if conflict {
			return 409, `{"error":{"type":"idempotency_error"}}`
		}
		return 200, `{"id":"re_1","status":"succeeded"}`
	})
	provider := &Stripe{BaseURL: fake.server.URL}
	for name, c := range map[string]struct {
		intent string
		want   error
	}{
		"already refunded":                {intentJSON("succeeded", 9900, 9900, true, false), ErrAlreadyRefunded},
		"disputed (third look, 6)":        {intentJSON("succeeded", 9900, 0, false, true), ErrDisputed},
		"partly refunded (operator)":      {intentJSON("succeeded", 9900, 1000, false, false), ErrPartlyRefunded},
		"another licence's payment":       {strings.Replace(intentJSON("succeeded", 9900, 0, false, false), `"license_id":"`+licenseID, `"license_id":"`+strings.Repeat("f", 32), 1), ErrMismatch},
		"another amount than the order's": {intentJSON("succeeded", 4900, 0, false, false), ErrMismatch},
	} {
		body := c.intent
		fake.on(http.MethodGet, "/v1/payment_intents/pi_1", func(recordedRequest) (int, string) { return 200, body })
		before := refunds
		if err := provider.Refund(context.Background(), stripeCredentials(t), RefundRequest{Reference: "pi_1", Expected: expectOneTime}); !errors.Is(err, c.want) || refunds != before {
			t.Errorf("%s: %v (%d refund calls)", name, err, refunds-before)
		}
	}
	fake.on(http.MethodGet, "/v1/payment_intents/pi_1", func(recordedRequest) (int, string) { return 200, intentJSON("succeeded", 9900, 0, false, false) })
	conflict = true
	if err := provider.Refund(context.Background(), stripeCredentials(t), RefundRequest{Reference: "pi_1", Expected: expectOneTime}); !errors.Is(err, ErrProviderBusy) || errors.Is(err, ErrProviderRefused) {
		t.Fatalf("a 409 is not a refusal: %v", err)
	}
}

// subscriptionServer answers a subscription's state and keeps the change a POST makes; the first
// POST can fail with a 500 (an unknown outcome).
func subscriptionServer(t *testing.T, failFirst bool) (*fakeServer, *int) {
	fake := newFakeServer(t)
	cancel, posts := "false", 0
	fake.on(http.MethodGet, "/v1/subscriptions/sub_1", func(recordedRequest) (int, string) {
		return 200, `{"id":"sub_1","status":"active","livemode":false,"cancel_at_period_end":` + cancel +
			`,"metadata":{"license_id":"` + licenseID + `","order":"` + renewalOrder + `"}}`
	})
	fake.on(http.MethodPost, "/v1/subscriptions/sub_1", func(r recordedRequest) (int, string) {
		posts++
		if failFirst && posts == 1 {
			return 500, `{"error":{}}`
		}
		form, _ := url.ParseQuery(string(r.Body))
		cancel = form.Get("cancel_at_period_end")
		return 200, `{"id":"sub_1","cancel_at_period_end":` + cancel + `}`
	})
	return fake, &posts
}

func TestStripeRenewalKeysAreStable(t *testing.T) {
	fake, posts := subscriptionServer(t, true)
	clock := testNow
	provider := &Stripe{BaseURL: fake.server.URL, Now: func() time.Time { return clock }}
	off := RenewalChange{Subscription: "sub_1", LicenseID: licenseID, On: false, Change: "ch1"}
	if err := provider.SetRenewal(context.Background(), stripeCredentials(t), off); !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf("the first try's outcome is unknown: %v", err)
	}
	first := fake.last().Header.Get("Idempotency-Key")
	clock = clock.Add(time.Minute) // the retry a minute later is the same request
	if err := provider.SetRenewal(context.Background(), stripeCredentials(t), off); err != nil {
		t.Fatal(err)
	}
	if again := fake.last().Header.Get("Idempotency-Key"); again != first || first != "renewal-"+licenseID+"-off-ch1" {
		t.Fatalf("keys %q %q", first, again)
	}
	on := RenewalChange{Subscription: "sub_1", LicenseID: licenseID, On: true, Change: "ch2"}
	if err := provider.SetRenewal(context.Background(), stripeCredentials(t), on); err != nil || fake.last().Header.Get("Idempotency-Key") == first {
		t.Fatalf("turning on: %v", err)
	}
	if err := provider.SetRenewal(context.Background(), stripeCredentials(t), RenewalChange{Subscription: "sub_1", LicenseID: licenseID}); !errors.Is(err, ErrCheckoutIncomplete) {
		t.Fatalf("a change without an id: %v", err)
	}
	_ = posts
}

// Second review of #361, 3 and 4: the switch acts only on the licence's own subscription, in the
// order's mode, and answers "already in that state" without a change.
func TestStripeSetRenewalOnlyOnTheLicencesSubscription(t *testing.T) {
	fake, posts := subscriptionServer(t, false)
	provider := &Stripe{BaseURL: fake.server.URL}
	for name, c := range map[string]struct {
		change RenewalChange
		want   error
	}{
		"already on":         {RenewalChange{Subscription: "sub_1", LicenseID: licenseID, On: true, Change: "c1"}, ErrAlreadyInThatState},
		"another licence":    {RenewalChange{Subscription: "sub_1", LicenseID: strings.Repeat("f", 32), On: false, Change: "c2"}, ErrMismatch},
		"the other mode":     {RenewalChange{Subscription: "sub_1", LicenseID: licenseID, On: false, Change: "c3", Live: true}, ErrMismatch},
		"not a subscription": {RenewalChange{Subscription: "cus_1", LicenseID: licenseID, On: false, Change: "c4"}, ErrCheckoutIncomplete},
	} {
		if err := provider.SetRenewal(context.Background(), stripeCredentials(t), c.change); !errors.Is(err, c.want) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if *posts != 0 {
		t.Fatalf("%d changes were sent", *posts)
	}
}

// Third look at #361, 6: switching an ended subscription is a final answer of its own, never
// "refused, retry", and nothing is sent.
func TestStripeSetRenewalOnAnEndedSubscriptionIsFinal(t *testing.T) {
	fake := newFakeServer(t)
	posts := 0
	fake.on(http.MethodGet, "/v1/subscriptions/sub_1", func(recordedRequest) (int, string) {
		return 200, `{"id":"sub_1","status":"canceled","livemode":false,"cancel_at_period_end":false,"metadata":{"license_id":"` + licenseID + `"}}`
	})
	fake.on(http.MethodPost, "/v1/subscriptions/sub_1", func(recordedRequest) (int, string) { posts++; return 200, `{}` })
	provider := &Stripe{BaseURL: fake.server.URL}
	for _, on := range []bool{true, false} {
		err := provider.SetRenewal(context.Background(), stripeCredentials(t), RenewalChange{Subscription: "sub_1", LicenseID: licenseID, On: on, Change: "c1"})
		if !errors.Is(err, ErrSubscriptionEnded) || errors.Is(err, ErrProviderRefused) {
			t.Fatalf("on=%v: %v", on, err)
		}
	}
	if posts != 0 {
		t.Fatalf("%d changes were sent", posts)
	}
}

// Second review of #361, 3: one payment has one identity whatever it was reached through; a
// renewal's PaymentIntent and charge are a renewal; a charge-only payment is found.
func TestStripePaymentIdentity(t *testing.T) {
	fake := newFakeServer(t)
	provider := &Stripe{BaseURL: fake.server.URL}
	fake.on(http.MethodGet, "/v1/checkout/sessions/cs_1", sessionAnswer("payment", "complete", "paid", oneTimeOrder, intentJSON("succeeded", 9900, 0, false, false)))
	fake.on(http.MethodGet, "/v1/payment_intents/pi_1", func(recordedRequest) (int, string) { return 200, intentJSON("succeeded", 9900, 0, false, false) })
	viaSession, err1 := provider.Confirm(context.Background(), stripeCredentials(t), "cs_1", expectOneTime)
	viaIntent, err2 := provider.Confirm(context.Background(), stripeCredentials(t), "pi_1", expectOneTime)
	if err1 != nil || err2 != nil || viaSession.PaymentID != "pi_1" || viaIntent.PaymentID != viaSession.PaymentID || viaIntent.Outcome != OutcomePaid {
		t.Fatalf("one payment, two ways: %+v %v / %+v %v", viaSession, err1, viaIntent, err2)
	}

	// A renewal invoice paid by a PaymentIntent without our metadata (Stripe made it), and
	// another paid by a charge alone.
	renewal := Expected{Kind: OrderRenewalInvoice, Order: renewalOrder, LicenseID: licenseID, AmountMinor: 9900, Currency: "USD"}
	invoiceIntent := `{"id":"pi_9","status":"succeeded","amount":9900,"amount_received":9900,"currency":"usd","metadata":{},` +
		`"latest_charge":{"id":"ch_8","payment_intent":"pi_9","paid":true,"amount":9900,"amount_captured":9900,"amount_refunded":0,"currency":"usd"}}`
	fake.on(http.MethodGet, "/v1/payment_intents/pi_9", func(recordedRequest) (int, string) { return 200, invoiceIntent })
	fake.on(http.MethodGet, "/v1/charges/ch_8", func(recordedRequest) (int, string) {
		return 200, `{"id":"ch_8","payment_intent":"pi_9","paid":true,"amount":9900,"amount_captured":9900,"currency":"usd"}`
	})
	fake.on(http.MethodGet, "/v1/charges/ch_9", func(recordedRequest) (int, string) {
		return 200, `{"id":"ch_9","paid":true,"amount":9900,"amount_captured":9900,"amount_refunded":0,"currency":"usd"}`
	})
	fake.on(http.MethodGet, "/v1/invoices/in_2", func(recordedRequest) (int, string) { return 200, invoiceJSON("in_2", "paid", 9900, true) })
	fake.on(http.MethodGet, "/v1/invoices/in_3", func(recordedRequest) (int, string) { return 200, invoiceJSON("in_3", "paid", 9900, true) })
	fake.on(http.MethodGet, "/v1/invoice_payments", func(r recordedRequest) (int, string) {
		query, _ := url.ParseQuery(r.Query)
		switch {
		case query.Get("invoice") == "in_2", query.Get("payment[payment_intent]") == "pi_9":
			return 200, `{"data":[{"invoice":"in_2","payment":{"type":"payment_intent","payment_intent":"pi_9"}}]}`
		case query.Get("invoice") == "in_3", query.Get("payment[charge]") == "ch_9":
			return 200, `{"data":[{"invoice":"in_3","payment":{"type":"charge","charge":"ch_9"}}]}`
		}
		return 200, `{"data":[]}`
	})
	for reference, want := range map[string]string{"in_2": "pi_9", "pi_9": "pi_9", "ch_8": "pi_9", "in_3": "ch_9", "ch_9": "ch_9"} {
		fact, err := provider.Confirm(context.Background(), stripeCredentials(t), reference, renewal)
		if err != nil || fact.Outcome != OutcomePaid || fact.Kind != OrderRenewalInvoice || fact.PaymentID != want || fact.LicenseID != licenseID {
			t.Errorf("%s: %+v %v", reference, fact, err)
		}
	}
	// Two invoices of one subscription have different payment ids, so their refund keys differ.
	fake.on(http.MethodPost, "/v1/refunds", func(recordedRequest) (int, string) { return 200, `{"id":"re_1","status":"succeeded"}` })
	keys := map[string]bool{}
	for _, reference := range []string{"in_2", "in_3"} {
		if err := provider.Refund(context.Background(), stripeCredentials(t), RefundRequest{Reference: reference, Expected: renewal}); err != nil {
			t.Fatalf("refund %s: %v", reference, err)
		}
		keys[fake.last().Header.Get("Idempotency-Key")] = true
		if form, _ := url.ParseQuery(string(fake.last().Body)); reference == "in_3" && form.Get("charge") != "ch_9" {
			t.Fatalf("a charge-only payment is refunded through its charge: %v", form)
		}
	}
	if len(keys) != 2 {
		t.Fatalf("two renewal refunds share a key: %v", keys)
	}
	// A PaymentIntent that is neither ours nor an invoice's is an error, not a guessed kind.
	fake.on(http.MethodGet, "/v1/payment_intents/pi_7", func(recordedRequest) (int, string) {
		return 200, strings.Replace(invoiceIntent, `"pi_9"`, `"pi_7"`, -1)
	})
	if _, err := provider.Confirm(context.Background(), stripeCredentials(t), "pi_7", renewal); !errors.Is(err, ErrUnexpectedState) {
		t.Fatalf("a stranger's PaymentIntent: %v", err)
	}
	// A refund hint for a charge-only payment names the charge (no PaymentIntent).
	event, err := stripeNotification(t, &Stripe{Now: func() time.Time { return testNow }},
		`{"id":"evt_1","type":"charge.refunded","data":{"object":{"id":"ch_9","amount":9900,"amount_refunded":9900,"refunded":true,"currency":"usd"}}}`, true)
	if err != nil || event.Kind != EventRefund || event.Reference != "ch_9" || event.Charge != "ch_9" {
		t.Fatalf("a charge-only refund hint: %+v %v", event, err)
	}
}

// Second review of #361, 4: a test-mode payment never confirms a live order, nor the reverse.
func TestStripeTestModeNeverConfirmsALiveOrder(t *testing.T) {
	fake := newFakeServer(t)
	provider := &Stripe{BaseURL: fake.server.URL}
	fake.on(http.MethodGet, "/v1/payment_intents/pi_1", func(recordedRequest) (int, string) { return 200, intentJSON("succeeded", 9900, 0, false, false) })
	live := expectOneTime
	live.Live = true
	if fact, err := provider.Confirm(context.Background(), stripeCredentials(t), "pi_1", live); err != nil || fact.Outcome != OutcomeMismatch || fact.Mismatch != "mode" {
		t.Fatalf("a test payment for a live order: %+v %v", fact, err)
	}
	fake.on(http.MethodGet, "/v1/payment_intents/pi_1", func(recordedRequest) (int, string) {
		return 200, strings.Replace(intentJSON("succeeded", 9900, 0, false, false), `"id":"pi_1",`, `"id":"pi_1","livemode":true,`, 1)
	})
	if fact, err := provider.Confirm(context.Background(), stripeCredentials(t), "pi_1", expectOneTime); err != nil || fact.Outcome != OutcomeMismatch || fact.Mismatch != "mode" {
		t.Fatalf("a live payment for a test order: %+v %v", fact, err)
	}
	if fact, err := provider.Confirm(context.Background(), stripeCredentials(t), "pi_1", live); err != nil || fact.Outcome != OutcomePaid || !fact.Live {
		t.Fatalf("a live payment for a live order: %+v %v", fact, err)
	}
}

// Second review of #361, 5: the $0 invoice of a set-up is its own kind (provider behaviour to
// verify), and a partial refund is its own outcome with the amounts.
func TestStripeSetupInvoiceAndPartialRefund(t *testing.T) {
	fake := newFakeServer(t)
	provider := &Stripe{BaseURL: fake.server.URL}
	fake.on(http.MethodGet, "/v1/invoices/in_0", func(recordedRequest) (int, string) {
		return 200, strings.Replace(invoiceJSON("in_0", "paid", 0, true), `"amount_paid":0`, `"amount_paid":0,"amount_due":0,"total":0,"billing_reason":"subscription_create"`, 1)
	})
	renewal := Expected{Kind: OrderRenewalInvoice, Order: renewalOrder, LicenseID: licenseID, AmountMinor: 9900, Currency: "USD"}
	fact, err := provider.Confirm(context.Background(), stripeCredentials(t), "in_0", renewal)
	if err != nil || fact.Outcome != OutcomeNothingCharged || fact.Kind != OrderSetupInvoice || fact.PaidMinor != 0 {
		t.Fatalf("the $0 set-up invoice: %+v %v", fact, err)
	}
	// A later (renewal-cycle) invoice with nothing paid is not a set-up: still an anomaly.
	fake.on(http.MethodGet, "/v1/invoices/in_z", func(recordedRequest) (int, string) {
		return 200, strings.Replace(invoiceJSON("in_z", "paid", 0, true), `"amount_paid":0`, `"amount_paid":0,"amount_due":0,"total":0,"billing_reason":"subscription_cycle"`, 1)
	})
	if fact, err := provider.Confirm(context.Background(), stripeCredentials(t), "in_z", renewal); !errors.Is(err, ErrUnexpectedState) || fact.Outcome == OutcomeNothingCharged {
		t.Fatalf("a cycle invoice with nothing paid: %+v %v", fact, err)
	}
	fake.on(http.MethodGet, "/v1/payment_intents/pi_1", func(recordedRequest) (int, string) { return 200, intentJSON("succeeded", 9900, 2500, false, false) })
	fact, err = provider.Confirm(context.Background(), stripeCredentials(t), "pi_1", expectOneTime)
	if err != nil || fact.Outcome != OutcomePartlyRefunded || fact.PaidMinor != 9900 || fact.RefundedMinor != 2500 {
		t.Fatalf("a partial refund: %+v %v", fact, err)
	}
}

// Second review of #361, 7: the manual provider has no webhook; a route dispatching by code
// cannot reach it.
func TestManualHasNoWebhook(t *testing.T) {
	if _, err := (Manual{}).ParseNotification(context.Background(), "", Notification{Body: []byte(`{}`)}); !errors.Is(err, ErrNoWebhook) {
		t.Fatalf("manual parse: %v", err)
	}
	registry := DefaultRegistry()
	if _, ok := registry.WebhookProvider(ManualCode); ok {
		t.Fatal("the manual provider is reachable from a webhook route")
	}
	if p, ok := registry.WebhookProvider(StripeCode); !ok || p.Code() != StripeCode {
		t.Fatal("Stripe must be reachable from its webhook route")
	}
	if _, ok := registry.WebhookProvider("nobody"); ok {
		t.Fatal("an unknown code")
	}
}

func TestStripeRefundDisputeAndSubscriptionEvents(t *testing.T) {
	provider := &Stripe{Now: func() time.Time { return testNow }}
	for _, c := range []struct {
		name, body string
		check      func(Event) bool
	}{
		{"one-time payment completed", `{"id":"evt_1","type":"checkout.session.completed","data":{"object":{"id":"cs_1","mode":"payment",` +
			`"payment_status":"paid","client_reference_id":"` + oneTimeOrder + `","payment_intent":"pi_1","amount_total":9900,"currency":"usd"}}}`,
			func(e Event) bool {
				return e.Kind == EventPayment && e.Reference == "cs_1" && e.Order == oneTimeOrder &&
					e.LicenseID == licenseID && e.PaymentIntent == "pi_1" && e.EventID == "evt_1"
			}},
		{"delayed payment succeeded", `{"id":"evt_3","type":"checkout.session.async_payment_succeeded","data":{"object":{"id":"cs_1","mode":"payment",` +
			`"payment_status":"paid","client_reference_id":"` + oneTimeOrder + `"}}}`,
			func(e Event) bool { return e.Kind == EventPayment && e.Reference == "cs_1" }},
		{"checkout expired", `{"id":"evt_5","type":"checkout.session.expired","data":{"object":{"id":"cs_1","mode":"payment",` +
			`"status":"expired","client_reference_id":"` + oneTimeOrder + `"}}}`,
			func(e Event) bool { return e.Kind == EventPayment && e.Reference == "cs_1" }},
		{"another shop's checkout", `{"id":"evt_6","type":"checkout.session.completed","data":{"object":{"id":"cs_9","mode":"payment",` +
			`"payment_status":"paid","client_reference_id":"order-77"}}}`,
			func(e Event) bool { return e.Kind == EventIgnored }},
		{"refunded charge (Dashboard refund)", `{"id":"evt_7","type":"charge.refunded","data":{"object":{"id":"ch_1",` +
			`"payment_intent":"pi_1","amount":9900,"amount_refunded":9900,"refunded":true,"currency":"usd"}}}`,
			func(e Event) bool { return e.Kind == EventRefund && e.Reference == "pi_1" && e.PaymentIntent == "pi_1" }},
		{"refund updated", `{"id":"evt_9","type":"refund.updated","data":{"object":{"id":"re_1","payment_intent":"pi_1",` +
			`"status":"succeeded","amount":9900,"currency":"usd"}}}`,
			func(e Event) bool { return e.Kind == EventRefund && e.Refund == "re_1" && e.Reference == "pi_1" }},
		{"dispute opened", `{"id":"evt_11","type":"charge.dispute.created","data":{"object":{"id":"dp_1","payment_intent":"pi_1",` +
			`"status":"needs_response","amount":9900,"currency":"usd"}}}`,
			func(e Event) bool { return e.Kind == EventDispute && e.Dispute == "dp_1" && e.Reference == "pi_1" }},
		{"renewal changed", `{"id":"evt_12","type":"customer.subscription.updated","data":{"object":{"id":"sub_1",` +
			`"customer":"cus_1","status":"active","cancel_at_period_end":true,"metadata":{"license_id":"` + licenseID + `"}}}}`,
			func(e Event) bool {
				return e.Kind == EventRenewalMethod && e.Reference == "sub_1" && e.Subscription == "sub_1" && e.LicenseID == licenseID
			}},
	} {
		event, err := stripeNotification(t, provider, c.body, true)
		if err != nil || !c.check(event) {
			t.Errorf("%s: %+v %v", c.name, event, err)
		}
		if event.Kind != EventIgnored && event.EventID == "" {
			t.Errorf("%s: no event id for the store's duplicate check", c.name)
		}
	}
}

func TestPaymentStatesOnlyMoveForward(t *testing.T) {
	for _, c := range []struct {
		current, next Status
		want          Status
		changed       bool
	}{
		{"", StatusPending, StatusPending, true},
		{StatusPending, StatusSucceeded, StatusSucceeded, true},
		{StatusSucceeded, StatusFailed, StatusSucceeded, false},  // a late failure never undoes a payment
		{StatusSucceeded, StatusPending, StatusSucceeded, false}, // nor a replayed pending
		{StatusSucceeded, StatusExpired, StatusSucceeded, false},
		{StatusSucceeded, StatusRefunded, StatusRefunded, true},
		{StatusRefunded, StatusSucceeded, StatusRefunded, false}, // a refund is final
		{StatusRefunded, StatusFailed, StatusRefunded, false},
		{StatusFailed, StatusSucceeded, StatusSucceeded, true}, // money that did arrive is recorded
		{StatusExpired, StatusFailed, StatusExpired, false},
		{StatusPending, StatusPending, StatusPending, false},
		{StatusPending, "maybe", StatusPending, false},
	} {
		got, changed := Advance(c.current, c.next)
		if got != c.want || changed != c.changed {
			t.Errorf("Advance(%q, %q) = %q %v, want %q %v", c.current, c.next, got, changed, c.want, c.changed)
		}
	}
	// Replayed, duplicated and reordered reports end in the same state as the right order.
	reports := []Status{StatusPending, StatusSucceeded, StatusFailed, StatusPending, StatusSucceeded, StatusRefunded, StatusFailed, StatusSucceeded}
	for shift := range reports {
		state := Status("")
		for i := range reports {
			state, _ = Advance(state, reports[(i+shift)%len(reports)])
		}
		if state != StatusRefunded {
			t.Fatalf("order %d ended in %q", shift, state)
		}
	}
}

// refuseTransport fails the test on any request: nothing may leave the machine.
type refuseTransport struct{ t *testing.T }

func (r refuseTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	r.t.Errorf("a request left for %s", request.URL.Host)
	return nil, errors.New("refused by the test")
}

// Review of #361, 7: the guard reads the parsed host, whatever the spelling.
func TestNoTestReachesTheRealStripe(t *testing.T) {
	for _, base := range []string{"", "https://api.stripe.com", "https://api.stripe.com:443", "https://API.stripe.com",
		"https://api.stripe.com/v1", "https://api.stripe.com.", "HTTPS://api.stripe.com", "https://files.stripe.com", "::bad"} {
		provider := &Stripe{BaseURL: base, Client: &http.Client{Transport: refuseTransport{t}}, Now: func() time.Time { return testNow }}
		if _, err := provider.StartCheckout(context.Background(), stripeCredentials(t), oneTimeCheckout(t)); !errors.Is(err, ErrProviderUnavailable) {
			t.Errorf("%q checkout: %v", base, err)
		}
		if _, err := provider.Confirm(context.Background(), stripeCredentials(t), "cs_1", expectOneTime); !errors.Is(err, ErrProviderUnavailable) {
			t.Errorf("%q confirm: %v", base, err)
		}
	}
}

// disputeAnswer is Stripe's list of a charge's disputes with one dispute in this status.
func disputeAnswer(status string) func(recordedRequest) (int, string) {
	return func(r recordedRequest) (int, string) {
		if query, _ := url.ParseQuery(r.Query); query.Get("charge") == "" {
			return 400, `{"error":{}}`
		}
		return 200, `{"data":[{"id":"dp_1","status":"` + status + `"}]}`
	}
}

// S2 review, 1: a dispute's status decides. Lost is money taken back (reversed, applied like a
// refund); open or won changes nothing but is reported.
func TestStripeADisputeIsReadByItsStatus(t *testing.T) {
	fake := newFakeServer(t)
	provider := &Stripe{BaseURL: fake.server.URL}
	fake.on(http.MethodGet, "/v1/payment_intents/pi_1", func(recordedRequest) (int, string) {
		return 200, intentJSON("succeeded", 9900, 0, false, true)
	})
	for status, want := range map[string]Outcome{"needs_response": OutcomePaid, "under_review": OutcomePaid, "won": OutcomePaid, "lost": OutcomeReversed} {
		fake.on(http.MethodGet, "/v1/disputes", disputeAnswer(status))
		fact, err := provider.Confirm(context.Background(), stripeCredentials(t), "pi_1", expectOneTime)
		if err != nil || fact.Outcome != want || !fact.Disputed || fact.DisputeStatus != status {
			t.Errorf("%s: %+v %v", status, fact, err)
		}
	}
	// A charge marked disputed whose dispute cannot be found is not applied either way.
	fake.on(http.MethodGet, "/v1/disputes", func(recordedRequest) (int, string) { return 200, `{"data":[]}` })
	if _, err := provider.Confirm(context.Background(), stripeCredentials(t), "pi_1", expectOneTime); !errors.Is(err, ErrUnexpectedState) {
		t.Fatalf("no dispute found: %v", err)
	}
}

// S2 review, 6, 7 and 8: the checkout ends with the order, takes no converted currency, and can
// be read again and ended.
func TestStripeCheckoutSessionLifetime(t *testing.T) {
	fake := newFakeServer(t)
	clock := testNow
	provider := &Stripe{BaseURL: fake.server.URL, Now: func() time.Time { return clock }}
	fake.on(http.MethodPost, "/v1/checkout/sessions", func(recordedRequest) (int, string) {
		return 200, `{"id":"cs_9","url":"https://checkout.stripe.com/c/pay/cs_9"}`
	})
	checkout := oneTimeCheckout(t)
	checkout.ExpiresAt = clock.Add(23 * time.Hour)
	if _, err := provider.StartCheckout(context.Background(), stripeCredentials(t), checkout); err != nil {
		t.Fatal(err)
	}
	form, _ := url.ParseQuery(string(fake.last().Body))
	if form.Get("expires_at") != strconv.FormatInt(checkout.ExpiresAt.Unix(), 10) || form.Get("adaptive_pricing[enabled]") != "false" {
		t.Fatalf("checkout form: %v", form)
	}
	for _, bad := range []time.Duration{10 * time.Minute, 25 * time.Hour} {
		checkout.ExpiresAt = clock.Add(bad)
		if _, err := provider.StartCheckout(context.Background(), stripeCredentials(t), checkout); !errors.Is(err, ErrCheckoutIncomplete) {
			t.Fatalf("an expiry %v ahead: %v", bad, err)
		}
	}
	status := "open"
	fake.on(http.MethodGet, "/v1/checkout/sessions/cs_9", func(recordedRequest) (int, string) {
		return 200, `{"id":"cs_9","status":"` + status + `","url":"https://checkout.stripe.com/c/pay/cs_9"}`
	})
	expired := 0
	fake.on(http.MethodPost, "/v1/checkout/sessions/cs_9/expire", func(recordedRequest) (int, string) {
		expired++
		status = "expired"
		return 200, `{"id":"cs_9","status":"expired"}`
	})
	if s, err := provider.Session(context.Background(), stripeCredentials(t), "cs_9"); err != nil || s.Status != "open" || s.URL == "" {
		t.Fatalf("session: %+v %v", s, err)
	}
	for i := 0; i < 2; i++ {
		if err := provider.ExpireSession(context.Background(), stripeCredentials(t), "cs_9"); err != nil {
			t.Fatal(err)
		}
	}
	if expired != 1 {
		t.Fatalf("an expired session was ended again: %d", expired)
	}
}
