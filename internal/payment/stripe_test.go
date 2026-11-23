// Source: bioexamine, backend/internal/payment/stripe_test.go, read at commit 181fccf6
// (last changed there in 602dbeeb, 2026-11-04); copied into CelikPanel on 2026-11-23 under
// the shared catalogue's entry A (same language: copy with a source note). CelikPanel's
// changes after the copy are marked "CelikPanel:" and are in this repository's history.

package payment

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

// CelikPanel: the test key is visibly not a key (bioexamine's had a real key's shape,
// which secret scanners flag); it keeps the sk_test_ prefix and the last four "1234"
// the tests check. The signing secret is unchanged: the known vector depends on it.
const (
	stripeTestKey    = "sk_test_not-a-real-key-1234"
	stripeTestSecret = "whsec_test_secret"
)

func stripeCredentials(t *testing.T) Credentials {
	return credentialsOf(t, map[string]string{"mode": ModeTest, "secret_key": stripeTestKey, "webhook_secret": stripeTestSecret})
}

// signStripe signs a body the way Stripe does, written independently of VerifyStripeSignature.
func signStripe(body string, secret string, at time.Time) string {
	timestamp := strconv.FormatInt(at.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "." + body))
	return "t=" + timestamp + ",v1=" + hex.EncodeToString(mac.Sum(nil))
}

// A known vector, computed outside Go (.NET HMACSHA256 over "1492774577." + body with the
// signing secret as key; the timestamp is the one in Stripe's own header example).
func TestStripeSignatureKnownVector(t *testing.T) {
	body := []byte(`{"id":"evt_1","type":"invoice.paid"}`)
	header := "t=1492774577,v1=f7a632a9ffdbb1ced2fc48e44c374ec2cb4292469db292862c06ae703e2befbd,v0=6ffbb59b2300aae63f272406069a9788598b792a944a07aba816edb039989a39"
	at := time.Unix(1492774577, 0).Add(10 * time.Second)
	if !VerifyStripeSignature(header, body, stripeTestSecret, at, stripeTolerance) {
		t.Fatal("the known vector must verify")
	}
	tampered := []byte(`{"id":"evt_1","type":"invoice.paid "}`)
	for name, ok := range map[string]bool{
		"tampered body":       VerifyStripeSignature(header, tampered, stripeTestSecret, at, stripeTolerance),
		"other secret":        VerifyStripeSignature(header, body, "whsec_other", at, stripeTolerance),
		"older than 5 min":    VerifyStripeSignature(header, body, stripeTestSecret, at.Add(5*time.Minute), stripeTolerance),
		"v0 only (downgrade)": VerifyStripeSignature("t=1492774577,v0=f7a632a9ffdbb1ced2fc48e44c374ec2cb4292469db292862c06ae703e2befbd", body, stripeTestSecret, at, stripeTolerance),
		"no header":           VerifyStripeSignature("", body, stripeTestSecret, at, stripeTolerance),
		"zero tolerance":      VerifyStripeSignature(header, body, stripeTestSecret, at, 0),
	} {
		if ok {
			t.Errorf("%s verified", name)
		}
	}
	// Two v1 while a rolled secret is active: one matching is enough.
	rolled := "t=1492774577,v1=00" + strings.Repeat("ab", 31) + ",v1=f7a632a9ffdbb1ced2fc48e44c374ec2cb4292469db292862c06ae703e2befbd"
	if !VerifyStripeSignature(rolled, body, stripeTestSecret, at, stripeTolerance) {
		t.Fatal("a second v1 signature must be accepted")
	}
}

// CelikPanel: a yearly subscription session exists only when the member turned automatic
// renewal on; its first charge is the licence end (the one-time purchase is
// TestStripeEveryPurchaseIsAOneTimePayment).
func TestStripeCheckoutIsAYearlySubscriptionSession(t *testing.T) {
	fake := newFakeServer(t)
	fake.on(http.MethodPost, "/v1/checkout/sessions", func(r recordedRequest) (int, string) {
		return 200, `{"id":"cs_test_a1","object":"checkout.session","url":"https://checkout.stripe.com/c/pay/cs_test_a1"}`
	})
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	provider := &Stripe{BaseURL: fake.server.URL, Now: func() time.Time { return now }}
	order := mustReference(t, licenseID, true)
	licenceEnd := now.AddDate(0, 10, 0)
	checkout := Checkout{LicenseID: licenseID, Order: order, Amount: "99.00", Currency: "USD", Period: PeriodYearly,
		PayerEmail: "payer@example.com", SuccessURL: "https://account.example/done", CancelURL: "https://account.example/back",
		Language: "en", AutomaticRenewal: true, FirstChargeAt: licenceEnd}
	start, err := provider.StartCheckout(context.Background(), stripeCredentials(t), checkout)
	if err != nil || start.Reference != "cs_test_a1" || start.RedirectURL != "https://checkout.stripe.com/c/pay/cs_test_a1" {
		t.Fatalf("checkout: %+v %v", start, err)
	}
	sent := fake.last()
	form, _ := url.ParseQuery(string(sent.Body))
	want := map[string]string{
		"mode": "subscription", "client_reference_id": order,
		"line_items[0][price_data][currency]":                  "usd",
		"line_items[0][price_data][unit_amount]":               "9900", // 99.00 USD in cents
		"line_items[0][price_data][recurring][interval]":       "year",
		"line_items[0][price_data][recurring][interval_count]": "1",
		"line_items[0][quantity]":                              "1",
		"subscription_data[metadata][license_id]":              licenseID,
		"subscription_data[trial_end]":                         strconv.FormatInt(licenceEnd.Unix(), 10),
		"success_url":                                          "https://account.example/done",
		"customer_email":                                       "payer@example.com",
	}
	for field, value := range want {
		if form.Get(field) != value {
			t.Errorf("%s = %q, want %q", field, form.Get(field), value)
		}
	}
	if sent.Header.Get("Authorization") != "Bearer "+stripeTestKey || sent.Header.Get("Stripe-Version") != stripeVersion ||
		sent.Header.Get("Idempotency-Key") != order || !strings.HasPrefix(order, "CPS") {
		t.Fatalf("headers: %v", sent.Header)
	}

	checkout.Period = "monthly"
	if _, err := provider.StartCheckout(context.Background(), stripeCredentials(t), checkout); !errors.Is(err, ErrPeriodNotYearly) {
		t.Fatalf("monthly: %v", err)
	}
	checkout.Period = PeriodYearly
	if _, err := provider.StartCheckout(context.Background(), "", checkout); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("no key: %v", err)
	}
	live := credentialsOf(t, map[string]string{"mode": ModeTest, "secret_key": "sk_live_not-a-real-key", "webhook_secret": stripeTestSecret})
	if _, err := provider.StartCheckout(context.Background(), live, checkout); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("a live key in test mode must not be used: %v", err)
	}
	fake.on(http.MethodPost, "/v1/checkout/sessions", func(recordedRequest) (int, string) {
		return 400, `{"error":{"type":"invalid_request_error"}}`
	})
	_, err = provider.StartCheckout(context.Background(), stripeCredentials(t), checkout)
	if !errors.Is(err, ErrProviderRefused) {
		t.Fatalf("a refused session: %v", err)
	}
	noSecretIn(t, err, stripeTestKey)
}

func stripeNotification(t *testing.T, provider *Stripe, body string, sign bool) (Event, error) {
	t.Helper()
	headers := map[string][]string{}
	if sign {
		headers["Stripe-Signature"] = []string{signStripe(body, stripeTestSecret, provider.now())}
	}
	return provider.ParseNotification(context.Background(), stripeCredentials(t), Notification{Headers: headers, Body: []byte(body)})
}

// CelikPanel (review of #361, 3): an event holds identifiers and the kind of hint only.
func TestStripeWebhookEvents(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	provider := &Stripe{Now: func() time.Time { return now }}
	order := mustReference(t, licenseID, true)

	completed := `{"id":"evt_1","type":"checkout.session.completed","data":{"object":{"id":"cs_1","mode":"subscription",` +
		`"client_reference_id":"` + order + `","subscription":"sub_1","customer":"cus_1","payment_status":"paid"}}}`
	event, err := stripeNotification(t, provider, completed, true)
	if err != nil || event.Kind != EventRenewalMethod || event.LicenseID != licenseID || event.Subscription != "sub_1" ||
		event.Reference != "cs_1" || event.Order != order || event.EventID != "evt_1" {
		t.Fatalf("session completed: %+v %v", event, err)
	}

	// invoice.paid, the basil shape: the subscription and its metadata under parent.
	paid := `{"id":"evt_2","type":"invoice.paid","data":{"object":{"id":"in_1","object":"invoice","amount_paid":9900,` +
		`"amount_due":9900,"currency":"usd","customer":"cus_1","billing_reason":"subscription_cycle",` +
		`"parent":{"type":"subscription_details","subscription_details":{"subscription":"sub_1","metadata":{"license_id":"` + licenseID + `"}}}}}}`
	event, err = stripeNotification(t, provider, paid, true)
	if err != nil || event.Kind != EventPayment || event.Reference != "in_1" || event.LicenseID != licenseID || event.Subscription != "sub_1" {
		t.Fatalf("invoice paid: %+v %v", event, err)
	}
	// The pre-basil shape: subscription and subscription_details at the top.
	old := `{"id":"evt_3","type":"invoice.paid","data":{"object":{"id":"in_2","amount_paid":9900,"currency":"usd",` +
		`"subscription":"sub_1","subscription_details":{"metadata":{"license_id":"` + licenseID + `"}}}}}`
	if event, err = stripeNotification(t, provider, old, true); err != nil || event.Reference != "in_2" || event.LicenseID != licenseID {
		t.Fatalf("old shape: %+v %v", event, err)
	}
	failed := strings.Replace(strings.Replace(paid, "invoice.paid", "invoice.payment_failed", 1), `"amount_paid":9900`, `"amount_paid":0`, 1)
	if event, err = stripeNotification(t, provider, failed, true); err != nil || event.Kind != EventPayment || event.Reference != "in_1" {
		t.Fatalf("payment failed: %+v %v", event, err)
	}
	deleted := `{"id":"evt_4","type":"customer.subscription.deleted","data":{"object":{"id":"sub_1","customer":"cus_1",` +
		`"metadata":{"license_id":"` + licenseID + `"}}}}`
	if event, err = stripeNotification(t, provider, deleted, true); err != nil || event.Kind != EventRenewalMethod || event.Reference != "sub_1" {
		t.Fatalf("subscription deleted: %+v %v", event, err)
	}
	other := `{"id":"evt_5","type":"customer.created","data":{"object":{"id":"cus_9"}}}`
	if event, err = stripeNotification(t, provider, other, true); err != nil || event.Kind != EventIgnored {
		t.Fatalf("an unused event type: %+v %v", event, err)
	}

	// What must never be read: unsigned, tampered, stale.
	if _, err := stripeNotification(t, provider, paid, false); !errors.Is(err, ErrInvalidNotification) {
		t.Fatalf("an unsigned invoice.paid was accepted: %v", err)
	}
	header := signStripe(paid, stripeTestSecret, now)
	tampered := strings.Replace(paid, "9900", "990000", 1)
	_, err = provider.ParseNotification(context.Background(), stripeCredentials(t),
		Notification{Headers: map[string][]string{"Stripe-Signature": {header}}, Body: []byte(tampered)})
	if !errors.Is(err, ErrInvalidNotification) {
		t.Fatalf("a tampered amount was accepted: %v", err)
	}
	stale := signStripe(paid, stripeTestSecret, now.Add(-10*time.Minute))
	_, err = provider.ParseNotification(context.Background(), stripeCredentials(t),
		Notification{Headers: map[string][]string{"stripe-signature": {stale}}, Body: []byte(paid)})
	if !errors.Is(err, ErrInvalidNotification) {
		t.Fatalf("a replayed old delivery was accepted: %v", err)
	}
}

// CelikPanel: the renewal switch takes a change id, the refund an order and amount, and an
// invoice's state is read through Confirm.
func TestStripeCancelRefundAndStatus(t *testing.T) {
	fake := newFakeServer(t)
	order := "CPS" + licenseID + "abcdefabcdef"
	fake.on(http.MethodPost, "/v1/subscriptions/sub_1", func(r recordedRequest) (int, string) {
		form, _ := url.ParseQuery(string(r.Body))
		return 200, `{"id":"sub_1","object":"subscription","cancel_at_period_end":` + form.Get("cancel_at_period_end") + `}`
	})
	fake.on(http.MethodGet, "/v1/invoice_payments", func(r recordedRequest) (int, string) {
		query, _ := url.ParseQuery(r.Query)
		if query.Get("invoice") != "in_1" {
			return 200, `{"object":"list","data":[]}`
		}
		return 200, `{"object":"list","data":[{"id":"inpay_1","invoice":"in_1","status":"paid","payment":{"type":"payment_intent","payment_intent":"pi_1"}}]}`
	})
	fake.on(http.MethodPost, "/v1/refunds", func(r recordedRequest) (int, string) {
		form, _ := url.ParseQuery(string(r.Body))
		if form.Get("payment_intent") != "pi_1" {
			return 400, `{"error":{}}`
		}
		return 200, `{"id":"re_1","object":"refund","status":"succeeded"}`
	})
	fake.on(http.MethodGet, "/v1/invoices/in_1", func(recordedRequest) (int, string) {
		return 200, invoiceJSON("in_1", "paid", 9900, true)
	})
	fake.on(http.MethodGet, "/v1/payment_intents/pi_1", func(recordedRequest) (int, string) {
		return 200, intentJSON("succeeded", 9900, 0, false, false)
	})
	provider := &Stripe{BaseURL: fake.server.URL}
	change := RenewalChange{Subscription: "sub_1", LicenseID: licenseID, On: false, Change: "c1"}
	if err := provider.SetRenewal(context.Background(), stripeCredentials(t), change); err != nil {
		t.Fatalf("cancel at period end: %v", err)
	}
	if form, _ := url.ParseQuery(string(fake.last().Body)); form.Get("cancel_at_period_end") != "true" {
		t.Fatalf("turning renewal off must set cancel_at_period_end=true: %s", fake.last().Body)
	}
	change.On, change.Change = true, "c2"
	if err := provider.SetRenewal(context.Background(), stripeCredentials(t), change); err != nil {
		t.Fatalf("resume: %v", err)
	}
	request := RefundRequest{Reference: "in_1", Order: order, LicenseID: licenseID, AmountMinor: 9900, Currency: "USD"}
	if err := provider.Refund(context.Background(), stripeCredentials(t), request); err != nil {
		t.Fatalf("refund: %v", err)
	}
	if got := fake.last(); got.Path != "/v1/refunds" || got.Header.Get("Idempotency-Key") != "refund-"+order+"-9900usd-a0" {
		t.Fatalf("refund request: %+v", got)
	}
	request.Reference = "in_9"
	if err := provider.Refund(context.Background(), stripeCredentials(t), request); !errors.Is(err, ErrProviderRefused) {
		t.Fatalf("refund of an unpaid invoice: %v", err)
	}
	fact, err := provider.Confirm(context.Background(), stripeCredentials(t), "in_1",
		Expected{Kind: OrderRenewalInvoice, Order: order, LicenseID: licenseID, AmountMinor: 9900, Currency: "USD"})
	if err != nil || fact.Outcome != OutcomePaid || fact.Status != StatusSucceeded {
		t.Fatalf("status: %+v %v", fact, err)
	}
}

func TestAmountsInMinorUnits(t *testing.T) {
	for _, c := range []struct {
		amount, currency string
		minor            int64
	}{{"2490.00", "TRY", 249000}, {"99", "EUR", 9900}, {"1.5", "EUR", 150}, {"0.07", "TRY", 7}, {"500", "JPY", 500}} {
		got, err := ToMinor(c.amount, c.currency)
		if err != nil || got != c.minor {
			t.Errorf("ToMinor(%s %s) = %d %v, want %d", c.amount, c.currency, got, err, c.minor)
		}
	}
	if s, _ := FromMinor(249000, "TRY"); s != "2490.00" {
		t.Errorf("FromMinor: %s", s)
	}
	if _, err := ToMinor("1.234", "EUR"); err == nil {
		t.Error("three decimals accepted")
	}
	if _, err := ToMinor("1.000", "KWD"); err == nil {
		t.Error("a three-decimal currency accepted")
	}
	if _, err := ToMinor("1.50", "JPY"); err == nil {
		t.Error("a fraction of a zero-decimal currency accepted")
	}
}

// CelikPanel: references name a licence (32 hex), not a membership UUID.
func TestReferencesNameTheirMembership(t *testing.T) {
	reference, err := NewOrderReference(licenseID, false)
	if err != nil || len(reference) != 47 || LicenseFromReference(reference) != licenseID || !strings.HasPrefix(reference, "CPC") {
		t.Fatalf("checkout reference: %q %v", reference, err)
	}
	if again, _ := NewOrderReference(licenseID, false); again == reference {
		t.Fatal("two checkout attempts got the same reference")
	}
	renewal, _ := RenewalReference(licenseID, time.Date(2027, 12, 5, 9, 30, 0, 0, time.UTC))
	again, _ := RenewalReference(licenseID, time.Date(2027, 12, 5, 9, 30, 0, 0, time.UTC))
	if renewal != again || LicenseFromReference(renewal) != licenseID || !strings.HasPrefix(renewal, "CPR") {
		t.Fatalf("renewal reference must be deterministic: %q %q", renewal, again)
	}
	if LicenseFromReference("CPCnotours") != "" || LicenseFromReference("BXC"+licenseID+"abcdefabcdef") != "" {
		t.Fatal("a foreign reference named a licence")
	}
	if _, err := NewOrderReference("01234567-89ab-cdef-0123-456789abcdef", false); !errors.Is(err, ErrCheckoutIncomplete) {
		t.Fatalf("a non-licence id: %v", err)
	}
}

func TestCredentialsArePreparedWithoutEchoingThem(t *testing.T) {
	schema := (&Stripe{}).ConfigSchema()
	stored, last4, err := PrepareCredentials(schema, map[string]string{"mode": "test", "secret_key": stripeTestKey,
		"webhook_secret": stripeTestSecret}, "", CheckStripeConfig)
	if err != nil || last4 != "1234" {
		t.Fatalf("prepare: %q %v", last4, err)
	}
	// A secret left empty keeps the stored one; the mode can change alone.
	again, last4, err := PrepareCredentials(schema, map[string]string{"mode": "test", "secret_key": ""}, Credentials(stored), CheckStripeConfig)
	if err != nil || last4 != "1234" || again != stored {
		t.Fatalf("keep secret: %v", err)
	}
	_, _, err = PrepareCredentials(schema, map[string]string{"mode": "live", "secret_key": stripeTestKey}, Credentials(stored), CheckStripeConfig)
	var credentialErr *CredentialError
	if !errors.As(err, &credentialErr) || credentialErr.Field != "secret_key" {
		t.Fatalf("a test key in live mode: %v", err)
	}
	noSecretIn(t, err, stripeTestKey)
	if _, _, err := PrepareCredentials(schema, map[string]string{"mode": "test", "password": "x"}, Credentials(stored), nil); err == nil {
		t.Fatal("an unknown field accepted")
	}
	if public := PublicValues(schema, Credentials(stored)); public["mode"] != "test" || public["secret_key"] != "" || public["webhook_secret"] != "" {
		t.Fatalf("public values carry a secret: %v", public)
	}
}
