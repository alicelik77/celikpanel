package payment

// CelikPanel's additions to the copied Stripe adapter (D-047, amended 2026-11-23), each against
// the fake server: a one-time payment for every purchase, a subscription only by the member's
// choice with no bonus months, refunds of one-time payments, the refund, dispute and
// subscription-change events, FetchOrder as the truth behind every event, payment states that
// only move forward, and no test ever reaching api.stripe.com.

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
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

func intentJSON(status string, amount, refunded int64, fully, disputed bool) string {
	return `{"id":"pi_1","status":"` + status + `","amount":` + itoa64(amount) + `,"currency":"usd","customer":"cus_1",` +
		`"metadata":{"order":"CPC` + licenseID + `abcdefabcdef","license_id":"` + licenseID + `"},` +
		`"latest_charge":{"id":"ch_1","payment_intent":"pi_1","amount":` + itoa64(amount) + `,"amount_refunded":` + itoa64(refunded) +
		`,"refunded":` + boolText(fully) + `,"disputed":` + boolText(disputed) + `,"currency":"usd"}}`
}

func boolText(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func TestStripeFetchOrderIsTheTruth(t *testing.T) {
	fake := newFakeServer(t)
	provider := &Stripe{BaseURL: fake.server.URL, Now: func() time.Time { return testNow }}
	order := "CPC" + licenseID + "abcdefabcdef"
	session := func(status, paymentStatus, intent string) func(recordedRequest) (int, string) {
		return func(r recordedRequest) (int, string) {
			if query, _ := url.ParseQuery(r.Query); query.Get("expand[]") != "payment_intent.latest_charge" {
				return 400, `{"error":{}}`
			}
			return 200, `{"id":"cs_1","mode":"payment","status":"` + status + `","payment_status":"` + paymentStatus +
				`","client_reference_id":"` + order + `","metadata":{"license_id":"` + licenseID + `"},"payment_intent":` + intent + `}`
		}
	}
	for _, c := range []struct {
		name             string
		handler          func(recordedRequest) (int, string)
		status           Status
		amount, refunded string
		disputed         bool
	}{
		{"open", session("open", "unpaid", "null"), StatusPending, "", "", false},
		{"delayed method", session("complete", "unpaid", `"pi_1"`), StatusPending, "", "", false},
		{"expired", session("expired", "unpaid", "null"), StatusExpired, "", "", false},
		{"paid", session("complete", "paid", intentJSON("succeeded", 9900, 0, false, false)), StatusSucceeded, "99.00", "", false},
		{"refunded", session("complete", "paid", intentJSON("succeeded", 9900, 9900, true, false)), StatusRefunded, "99.00", "99.00", false},
		{"partly refunded", session("complete", "paid", intentJSON("succeeded", 9900, 1000, false, false)), StatusSucceeded, "99.00", "10.00", false},
		{"disputed", session("complete", "paid", intentJSON("succeeded", 9900, 0, false, true)), StatusSucceeded, "99.00", "", true},
	} {
		fake.on(http.MethodGet, "/v1/checkout/sessions/cs_1", c.handler)
		state, err := provider.FetchOrder(context.Background(), stripeCredentials(t), "cs_1")
		if err != nil || state.Status != c.status || state.Amount != c.amount || state.RefundedAmount != c.refunded ||
			state.Disputed != c.disputed || state.LicenseID != licenseID || state.Order != order {
			t.Errorf("%s: %+v %v", c.name, state, err)
		}
	}
	fake.on(http.MethodGet, "/v1/payment_intents/pi_1", func(recordedRequest) (int, string) {
		return 200, intentJSON("canceled", 9900, 0, false, false)
	})
	if state, err := provider.FetchOrder(context.Background(), stripeCredentials(t), "pi_1"); err != nil || state.Status != StatusFailed || state.LicenseID != licenseID {
		t.Fatalf("a cancelled PaymentIntent: %+v %v", state, err)
	}
	fake.on(http.MethodGet, "/v1/checkout/sessions/cs_2", func(recordedRequest) (int, string) {
		return 200, `{"id":"cs_2","mode":"subscription","status":"complete","payment_status":"no_payment_required",` +
			`"client_reference_id":"CPS` + licenseID + `abcdefabcdef","subscription":"sub_1","customer":"cus_1"}`
	})
	if state, err := provider.FetchOrder(context.Background(), stripeCredentials(t), "cs_2"); err != nil || state.Status != StatusSucceeded ||
		state.Subscription != "sub_1" || state.Amount != "" || state.LicenseID != licenseID {
		t.Fatalf("a subscription set up without a charge: %+v %v", state, err)
	}
	fake.on(http.MethodGet, "/v1/invoices/in_1", func(recordedRequest) (int, string) { return 200, `{"id":"in_1","status":"paid"}` })
	fake.on(http.MethodGet, "/v1/invoice_payments", func(recordedRequest) (int, string) {
		return 200, `{"data":[{"payment":{"type":"payment_intent","payment_intent":"pi_1"}}]}`
	})
	fake.on(http.MethodGet, "/v1/payment_intents/pi_1", func(recordedRequest) (int, string) {
		return 200, intentJSON("succeeded", 9900, 9900, true, false)
	})
	if state, err := provider.FetchOrder(context.Background(), stripeCredentials(t), "in_1"); err != nil || state.Status != StatusRefunded || state.Reference != "in_1" {
		t.Fatalf("a refunded renewal invoice: %+v %v", state, err)
	}
	if _, err := provider.FetchOrder(context.Background(), stripeCredentials(t), "ch_1"); !errors.Is(err, ErrNotSupported) {
		t.Fatalf("an unknown object: %v", err)
	}

	// A signed event that says "paid" is only a hint: the fetched state decides. Here Stripe
	// says the session is still open, so the order stays pending.
	fake.on(http.MethodGet, "/v1/checkout/sessions/cs_1", session("open", "unpaid", "null"))
	body := `{"id":"evt_9","type":"checkout.session.completed","data":{"object":{"id":"cs_1","mode":"payment","status":"complete",` +
		`"payment_status":"paid","client_reference_id":"` + order + `","amount_total":9900,"currency":"usd"}}}`
	event, err := stripeNotification(t, provider, body, true)
	if err != nil || event.Status != StatusSucceeded || event.Reference != "cs_1" {
		t.Fatalf("event: %+v %v", event, err)
	}
	state, err := provider.FetchOrder(context.Background(), stripeCredentials(t), event.Reference)
	if err != nil || state.Status != StatusPending {
		t.Fatalf("the fetched state must win over the event body: %+v %v", state, err)
	}
}

func TestStripeRefundOfAOneTimePayment(t *testing.T) {
	fake := newFakeServer(t)
	fake.on(http.MethodPost, "/v1/refunds", func(r recordedRequest) (int, string) {
		form, _ := url.ParseQuery(string(r.Body))
		if form.Get("payment_intent") != "pi_1" || form.Get("amount") != "" {
			return 400, `{"error":{}}`
		}
		return 200, `{"id":"re_1","status":"pending"}`
	})
	order := "CPC" + licenseID + "abcdefabcdef"
	fake.on(http.MethodGet, "/v1/checkout/sessions/cs_1", func(recordedRequest) (int, string) {
		return 200, `{"id":"cs_1","mode":"payment","status":"complete","payment_status":"paid","client_reference_id":"` + order +
			`","payment_intent":` + intentJSON("succeeded", 9900, 0, false, false) + `}`
	})
	fake.on(http.MethodGet, "/v1/checkout/sessions/cs_open", func(recordedRequest) (int, string) {
		return 200, `{"id":"cs_open","mode":"payment","status":"open","payment_status":"unpaid","client_reference_id":"` + order + `"}`
	})
	provider := &Stripe{BaseURL: fake.server.URL}
	if err := provider.Refund(context.Background(), stripeCredentials(t), Event{PaymentIntent: "pi_1", LicenseID: licenseID, Order: order}); err != nil {
		t.Fatalf("refund by PaymentIntent: %v", err)
	}
	sent := fake.last()
	form, _ := url.ParseQuery(string(sent.Body))
	if sent.Header.Get("Idempotency-Key") != "refund-pi_1" || form.Get("metadata[license_id]") != licenseID || form.Get("metadata[order]") != order {
		t.Fatalf("refund request: %v %v", sent.Header, form)
	}
	if err := provider.Refund(context.Background(), stripeCredentials(t), Event{Reference: "cs_1"}); err != nil {
		t.Fatalf("refund by checkout session: %v", err)
	}
	if got := fake.last(); got.Path != "/v1/refunds" || got.Header.Get("Idempotency-Key") != "refund-pi_1" {
		t.Fatalf("a refund through the session must refund its PaymentIntent once: %+v", got)
	}
	before := fake.count()
	if err := provider.Refund(context.Background(), stripeCredentials(t), Event{Reference: "cs_open"}); !errors.Is(err, ErrProviderRefused) {
		t.Fatalf("refunding an unpaid checkout: %v", err)
	}
	if fake.last().Path == "/v1/refunds" && fake.count() > before+1 {
		t.Fatal("an unpaid checkout reached the refund call")
	}
	if err := provider.Refund(context.Background(), stripeCredentials(t), Event{Reference: "ch_1"}); !errors.Is(err, ErrNotSupported) {
		t.Fatalf("an unknown object: %v", err)
	}
}

func TestStripeRefundDisputeAndSubscriptionEvents(t *testing.T) {
	provider := &Stripe{Now: func() time.Time { return testNow }}
	order := "CPC" + licenseID + "abcdefabcdef"
	for _, c := range []struct {
		name, body string
		check      func(Event) bool
	}{
		{"one-time payment completed", `{"id":"evt_1","type":"checkout.session.completed","data":{"object":{"id":"cs_1","mode":"payment",` +
			`"payment_status":"paid","client_reference_id":"` + order + `","payment_intent":"pi_1","amount_total":9900,"currency":"usd"}}}`,
			func(e Event) bool {
				return e.Kind == EventPayment && e.Status == StatusSucceeded && e.Reference == "cs_1" && e.Order == order &&
					e.LicenseID == licenseID && e.PaymentIntent == "pi_1" && e.Amount == "99.00" && e.EventID == "evt_1"
			}},
		{"delayed payment pending", `{"id":"evt_2","type":"checkout.session.completed","data":{"object":{"id":"cs_1","mode":"payment",` +
			`"payment_status":"unpaid","client_reference_id":"` + order + `"}}}`,
			func(e Event) bool { return e.Kind == EventPayment && e.Status == StatusPending }},
		{"delayed payment succeeded", `{"id":"evt_3","type":"checkout.session.async_payment_succeeded","data":{"object":{"id":"cs_1","mode":"payment",` +
			`"payment_status":"paid","client_reference_id":"` + order + `"}}}`,
			func(e Event) bool { return e.Status == StatusSucceeded }},
		{"delayed payment failed", `{"id":"evt_4","type":"checkout.session.async_payment_failed","data":{"object":{"id":"cs_1","mode":"payment",` +
			`"payment_status":"unpaid","client_reference_id":"` + order + `"}}}`,
			func(e Event) bool { return e.Status == StatusFailed }},
		{"checkout expired", `{"id":"evt_5","type":"checkout.session.expired","data":{"object":{"id":"cs_1","mode":"payment",` +
			`"status":"expired","client_reference_id":"` + order + `"}}}`,
			func(e Event) bool { return e.Status == StatusExpired && e.Reference == "cs_1" }},
		{"another shop's checkout", `{"id":"evt_6","type":"checkout.session.completed","data":{"object":{"id":"cs_9","mode":"payment",` +
			`"payment_status":"paid","client_reference_id":"order-77"}}}`,
			func(e Event) bool { return e.Kind == EventIgnored }},
		{"fully refunded charge (Dashboard refund)", `{"id":"evt_7","type":"charge.refunded","data":{"object":{"id":"ch_1",` +
			`"payment_intent":"pi_1","amount":9900,"amount_refunded":9900,"refunded":true,"currency":"usd"}}}`,
			func(e Event) bool {
				return e.Kind == EventRefund && e.Status == StatusRefunded && e.Reference == "pi_1" && e.Amount == "99.00"
			}},
		{"partly refunded charge", `{"id":"evt_8","type":"charge.refunded","data":{"object":{"id":"ch_1",` +
			`"payment_intent":"pi_1","amount":9900,"amount_refunded":1000,"refunded":false,"currency":"usd"}}}`,
			func(e Event) bool { return e.Kind == EventRefund && e.Status == StatusPending && e.Amount == "10.00" }},
		{"refund succeeded", `{"id":"evt_9","type":"refund.updated","data":{"object":{"id":"re_1","payment_intent":"pi_1",` +
			`"status":"succeeded","amount":9900,"currency":"usd"}}}`,
			func(e Event) bool { return e.Kind == EventRefund && e.Status == StatusRefunded && e.Refund == "re_1" }},
		{"refund failed", `{"id":"evt_10","type":"refund.failed","data":{"object":{"id":"re_1","payment_intent":"pi_1",` +
			`"status":"failed","amount":9900,"currency":"usd"}}}`,
			func(e Event) bool { return e.Kind == EventRefund && e.Status == StatusFailed }},
		{"dispute opened", `{"id":"evt_11","type":"charge.dispute.created","data":{"object":{"id":"dp_1","payment_intent":"pi_1",` +
			`"status":"needs_response","amount":9900,"currency":"usd"}}}`,
			func(e Event) bool {
				return e.Kind == EventDispute && e.Dispute == "dp_1" && e.Reference == "pi_1" && e.Note == "needs_response" && e.Status == ""
			}},
		{"automatic renewal turned off at the period end", `{"id":"evt_12","type":"customer.subscription.updated","data":{"object":{"id":"sub_1",` +
			`"customer":"cus_1","status":"active","cancel_at_period_end":true,"metadata":{"license_id":"` + licenseID + `"}}}}`,
			func(e Event) bool {
				return e.Kind == EventRenewalMethod && e.Renewal != nil && !e.Renewal.Active && e.LicenseID == licenseID && e.Reference == "sub_1"
			}},
		{"automatic renewal on", `{"id":"evt_13","type":"customer.subscription.updated","data":{"object":{"id":"sub_1",` +
			`"customer":"cus_1","status":"trialing","cancel_at_period_end":false,"metadata":{"license_id":"` + licenseID + `"}}}}`,
			func(e Event) bool { return e.Kind == EventRenewalMethod && e.Renewal.Active }},
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

func TestNoTestReachesTheRealStripe(t *testing.T) {
	provider := &Stripe{Now: func() time.Time { return testNow }} // no BaseURL: api.stripe.com
	if _, err := provider.StartCheckout(context.Background(), stripeCredentials(t), oneTimeCheckout(t)); !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf("checkout: %v", err)
	}
	if _, err := provider.FetchOrder(context.Background(), stripeCredentials(t), "cs_1"); !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf("fetch: %v", err)
	}
	if err := provider.Refund(context.Background(), stripeCredentials(t), Event{PaymentIntent: "pi_1"}); !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf("refund: %v", err)
	}
}
