// Source: bioexamine, backend/internal/payment/stripe.go, read at commit 181fccf6
// (last changed there in 898b3288, 2026-11-04); copied into CelikPanel on 2026-11-23 under
// the shared catalogue's entry A (same language: copy with a source note). CelikPanel's
// changes after the copy are marked "CelikPanel:" and are in this repository's history.

package payment

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// STRIPE -- the INTL region's provider (EUR).
//
// Checkout is a Stripe Checkout Session in subscription mode with an inline yearly price
// (price_data.recurring.interval=year): Stripe charges each year itself and reports every paid
// year with invoice.paid. We record a payment on invoice.paid (reference: the invoice id), never on
// checkout.session.completed, so the first year is recorded once whichever event arrives first;
// checkout.session.completed only links the subscription to the membership. The payer turning
// auto-renewal off sets cancel_at_period_end, so the paid year runs out and nothing more is charged.
//
// API calls carry Stripe-Version 2025-03-31.basil (the release that moved an invoice's
// subscription under parent.subscription_details and its payments to /v1/invoice_payments); the
// webhook parser reads both that shape and the older top-level one, because an event's shape
// follows the webhook endpoint's own API version, not ours.
//
// Official documentation followed:
//   - Checkout Session create: https://docs.stripe.com/api/checkout/sessions/create
//   - Webhook signatures (manual verification): https://docs.stripe.com/webhooks?verify=verify-manually
//   - Invoice object: https://docs.stripe.com/api/invoices/object
//   - Invoice payments list: https://docs.stripe.com/api/invoice-payment/list
//   - Subscription update (cancel_at_period_end): https://docs.stripe.com/api/subscriptions/update
//   - Refund create: https://docs.stripe.com/api/refunds/create
//   - Idempotent requests: https://docs.stripe.com/api/idempotent_requests
//   - Test and live keys: https://docs.stripe.com/keys

// StripeCode is Stripe's provider code.
const StripeCode = "stripe"

const (
	stripeAPIBase = "https://api.stripe.com"
	stripeVersion = "2025-03-31.basil"
	// stripeTolerance is the official libraries' default: "a default tolerance of 5 minutes
	// between the timestamp and the current time" (webhooks, "Preventing replay attacks").
	stripeTolerance = 5 * time.Minute
)

// Stripe is the Stripe provider. The zero value talks to api.stripe.com; tests set BaseURL to a
// fake server and Now to a fixed clock.
type Stripe struct {
	BaseURL string
	Client  *http.Client
	Now     func() time.Time
}

// Code implements Provider.
func (*Stripe) Code() string { return StripeCode }

// ConfigSchema implements Configurable.
func (*Stripe) ConfigSchema() ConfigSchema {
	return ConfigSchema{
		Provider: StripeCode,
		Fields: []ConfigField{
			modeField(),
			{Name: "secret_key", Label: "Gizli anahtar (Secret key)", Kind: FieldSecret, Required: true,
				Hint: "Stripe panelinde Developers → API keys; test kipinde sk_test_, canlıda sk_live_ ile başlar."},
			{Name: "webhook_secret", Label: "Webhook imza anahtarı (Signing secret)", Kind: FieldSecret, Required: true,
				Hint: "Webhooks → uç noktanız → Reveal secret; whsec_ ile başlar. Test ve canlı uç noktalarının anahtarı ayrıdır."},
		},
		Last4Field:       "secret_key",
		DocsURL:          "https://docs.stripe.com/payments/checkout",
		Webhook:          true,
		Renewal:          string(RenewalProviderSubscription),
		Refund:           true,
		CancelAtProvider: true,
		Currencies:       []string{"EUR", "TRY", "USD", "GBP"},
	}
}

// CheckStripeConfig refuses a secret key whose prefix does not match the mode (a live key in test
// mode would charge real cards) and a webhook secret that is not one.
func CheckStripeConfig(c Config) error {
	key := c["secret_key"]
	switch {
	case c.Mode() == ModeLive && !(strings.HasPrefix(key, "sk_live_") || strings.HasPrefix(key, "rk_live_")):
		return &CredentialError{Field: "secret_key", Code: CredentialLiveKeyRequired, Reason: "live mode needs a live key (sk_live_)"}
	case c.Mode() == ModeTest && !(strings.HasPrefix(key, "sk_test_") || strings.HasPrefix(key, "rk_test_")):
		return &CredentialError{Field: "secret_key", Code: CredentialTestKeyRequired, Reason: "test mode needs a test key (sk_test_)"}
	case !strings.HasPrefix(c["webhook_secret"], "whsec_"):
		return &CredentialError{Field: "webhook_secret", Code: CredentialSigningSecretRequired, Reason: "a signing secret starts with whsec_"}
	}
	return nil
}

func (s *Stripe) config(credentials Credentials) (Config, error) {
	config, err := requireConfig(credentials, s.ConfigSchema())
	if err != nil {
		return nil, err
	}
	if CheckStripeConfig(config) != nil {
		return nil, ErrNotConfigured
	}
	return config, nil
}

func (s *Stripe) base() string {
	if s.BaseURL != "" {
		return strings.TrimRight(s.BaseURL, "/")
	}
	return stripeAPIBase
}

func (s *Stripe) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// call sends one API request: form-encoded, Bearer key, pinned version, an idempotency key on
// every POST (https://docs.stripe.com/api/idempotent_requests). A 4xx is ErrProviderRefused; a 5xx
// or transport failure ErrProviderUnavailable. The answer is decoded into out.
func (s *Stripe) call(ctx context.Context, config Config, method, path string, form url.Values, idempotencyKey string, out any) error {
	var body *strings.Reader
	target := s.base() + path
	if method == http.MethodGet {
		if len(form) > 0 {
			target += "?" + form.Encode()
		}
		body = strings.NewReader("")
	} else {
		body = strings.NewReader(form.Encode())
	}
	request, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return ErrProviderUnavailable
	}
	request.Header.Set("Authorization", "Bearer "+config["secret_key"])
	request.Header.Set("Stripe-Version", stripeVersion)
	if method != http.MethodGet {
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if idempotencyKey != "" {
			request.Header.Set("Idempotency-Key", idempotencyKey)
		}
	}
	status, answer, err := send(s.Client, request)
	if err != nil {
		return err
	}
	if status >= 500 || status == http.StatusTooManyRequests {
		return ErrProviderUnavailable
	}
	if status >= 400 {
		return ErrProviderRefused
	}
	if out != nil && json.Unmarshal(answer, out) != nil {
		return ErrProviderUnavailable
	}
	return nil
}

// StartCheckout implements Provider: a Checkout Session in subscription mode with a yearly price.
// https://docs.stripe.com/api/checkout/sessions/create -- mode=subscription,
// line_items[].price_data.recurring.interval=year, client_reference_id,
// subscription_data.metadata (copied onto every invoice the subscription makes).
func (s *Stripe) StartCheckout(ctx context.Context, credentials Credentials, checkout Checkout) (CheckoutStart, error) {
	if checkout.Period != PeriodYearly {
		return CheckoutStart{}, ErrPeriodNotYearly
	}
	config, err := s.config(credentials)
	if err != nil {
		return CheckoutStart{}, err
	}
	if checkout.Origin == "" || checkout.MembershipID == "" {
		return CheckoutStart{}, ErrCheckoutIncomplete
	}
	minor, err := ToMinor(checkout.Amount, checkout.Currency)
	if err != nil {
		return CheckoutStart{}, err
	}
	reference, err := NewCheckoutReference(checkout.MembershipID)
	if err != nil {
		return CheckoutStart{}, err
	}
	form := url.Values{}
	form.Set("mode", "subscription")
	form.Set("client_reference_id", checkout.MembershipID)
	form.Set("success_url", ReturnURL(checkout.Origin, "tamam"))
	form.Set("cancel_url", ReturnURL(checkout.Origin, "vazgecildi"))
	form.Set("locale", languageOr(checkout.Language))
	if checkout.PayerEmail != "" {
		form.Set("customer_email", checkout.PayerEmail)
	}
	form.Set("line_items[0][quantity]", "1")
	form.Set("line_items[0][price_data][currency]", strings.ToLower(checkout.Currency))
	form.Set("line_items[0][price_data][unit_amount]", strconv.FormatInt(minor, 10))
	form.Set("line_items[0][price_data][recurring][interval]", "year")
	form.Set("line_items[0][price_data][recurring][interval_count]", "1")
	form.Set("line_items[0][price_data][product_data][name]", productName(checkout.PlanKind, checkout.Language))
	form.Set("metadata[membership_id]", checkout.MembershipID)
	form.Set("metadata[reference]", reference)
	form.Set("subscription_data[metadata][membership_id]", checkout.MembershipID)
	var session struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if err := s.call(ctx, config, http.MethodPost, "/v1/checkout/sessions", form, reference, &session); err != nil {
		return CheckoutStart{}, err
	}
	if session.ID == "" || !strings.HasPrefix(session.URL, "https://") {
		return CheckoutStart{}, ErrProviderUnavailable
	}
	return CheckoutStart{Provider: StripeCode, Reference: session.ID, RedirectURL: session.URL}, nil
}

// VerifyStripeSignature checks a Stripe-Signature header against the raw body, as the manual
// verification steps say (https://docs.stripe.com/webhooks?verify=verify-manually):
//
//  1. split the header on "," and each element on "="; t is the timestamp, every v1 a signature
//     (more than one while a rolled secret is still active); every other scheme is ignored
//     ("To prevent downgrade attacks, ignore all schemes that aren't v1");
//  2. signed_payload = timestamp + "." + body;
//  3. expected = HMAC-SHA256(key = the endpoint's signing secret, message = signed_payload), hex;
//  4. compare in constant time with each v1, then refuse a timestamp outside the tolerance.
func VerifyStripeSignature(header string, body []byte, secret string, now time.Time, tolerance time.Duration) bool {
	if secret == "" || header == "" || tolerance <= 0 {
		return false
	}
	var timestamp string
	var signatures []string
	for _, element := range strings.Split(header, ",") {
		prefix, value, ok := strings.Cut(strings.TrimSpace(element), "=")
		if !ok {
			continue
		}
		switch prefix {
		case "t":
			timestamp = value
		case "v1":
			signatures = append(signatures, value)
		}
	}
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || len(signatures) == 0 {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(body)
	expected := mac.Sum(nil)
	matched := false
	for _, signature := range signatures {
		given, err := hex.DecodeString(signature)
		if err == nil && hmac.Equal(given, expected) {
			matched = true
		}
	}
	if !matched {
		return false
	}
	age := now.Sub(time.Unix(seconds, 0))
	return age <= tolerance && age >= -tolerance
}

// stripeEvent is the part of an Event object we read (https://docs.stripe.com/api/events/object).
type stripeEvent struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Data struct {
		Object json.RawMessage `json:"object"`
	} `json:"data"`
}

type stripeInvoice struct {
	ID         string            `json:"id"`
	AmountPaid int64             `json:"amount_paid"`
	AmountDue  int64             `json:"amount_due"`
	Currency   string            `json:"currency"`
	Customer   string            `json:"customer"`
	Metadata   map[string]string `json:"metadata"`
	// 2025-03-31.basil and later.
	Parent *struct {
		SubscriptionDetails *struct {
			Subscription string            `json:"subscription"`
			Metadata     map[string]string `json:"metadata"`
		} `json:"subscription_details"`
	} `json:"parent"`
	// Before basil.
	Subscription        string `json:"subscription"`
	SubscriptionDetails *struct {
		Metadata map[string]string `json:"metadata"`
	} `json:"subscription_details"`
}

func (i stripeInvoice) subscription() (string, string) {
	if i.Parent != nil && i.Parent.SubscriptionDetails != nil {
		return i.Parent.SubscriptionDetails.Subscription, i.Parent.SubscriptionDetails.Metadata["membership_id"]
	}
	membership := i.Metadata["membership_id"]
	if i.SubscriptionDetails != nil && i.SubscriptionDetails.Metadata["membership_id"] != "" {
		membership = i.SubscriptionDetails.Metadata["membership_id"]
	}
	return i.Subscription, membership
}

// ParseNotification implements Provider: verify the Stripe-Signature header, then read the event.
//
//	checkout.session.completed     -> the subscription is this membership's renewal method
//	invoice.paid                   -> a year paid (the first and every renewal), reference = invoice id
//	invoice.payment_failed         -> that invoice failed (a later invoice.paid on it succeeds it)
//	customer.subscription.deleted  -> the subscription ended; the membership no longer renews
//	anything else                  -> received, nothing recorded
func (s *Stripe) ParseNotification(_ context.Context, credentials Credentials, notification Notification) (Event, error) {
	config, err := s.config(credentials)
	if err != nil {
		return Event{}, ErrInvalidNotification
	}
	if !VerifyStripeSignature(notification.header("Stripe-Signature"), notification.Body, config["webhook_secret"],
		s.now(), stripeTolerance) {
		return Event{}, ErrInvalidNotification
	}
	var event stripeEvent
	if err := json.Unmarshal(notification.Body, &event); err != nil || event.Type == "" {
		return Event{}, ErrInvalidNotification
	}
	ignored := Event{Kind: EventIgnored, Provider: StripeCode, Reference: event.ID}
	switch event.Type {
	case "checkout.session.completed":
		var session struct {
			Mode              string            `json:"mode"`
			ClientReferenceID string            `json:"client_reference_id"`
			Subscription      string            `json:"subscription"`
			Customer          string            `json:"customer"`
			Metadata          map[string]string `json:"metadata"`
		}
		if json.Unmarshal(event.Data.Object, &session) != nil {
			return Event{}, ErrInvalidNotification
		}
		membership := session.ClientReferenceID
		if membership == "" {
			membership = session.Metadata["membership_id"]
		}
		if session.Mode != "subscription" || session.Subscription == "" || membership == "" {
			return ignored, nil
		}
		return Event{Kind: EventRenewalMethod, Provider: StripeCode, Reference: event.ID, MembershipID: membership,
			Renewal: &Renewal{Kind: RenewalProviderSubscription, Reference: session.Subscription,
				Customer: session.Customer, Active: true}}, nil

	case "invoice.paid", "invoice.payment_failed":
		var invoice stripeInvoice
		if json.Unmarshal(event.Data.Object, &invoice) != nil || invoice.ID == "" {
			return Event{}, ErrInvalidNotification
		}
		subscription, membership := invoice.subscription()
		if membership == "" {
			return ignored, nil // not one of our memberships' invoices
		}
		status, minor := StatusSucceeded, invoice.AmountPaid
		if event.Type == "invoice.payment_failed" {
			status, minor = StatusFailed, invoice.AmountDue
		}
		if minor <= 0 {
			return ignored, nil // a zero invoice moves no money and buys no year
		}
		currency := strings.ToUpper(invoice.Currency)
		amount, err := FromMinor(minor, currency)
		if err != nil {
			return Event{}, ErrInvalidNotification
		}
		out := Event{Provider: StripeCode, Reference: invoice.ID, MembershipID: membership, Status: status,
			Amount: amount, Currency: currency, Period: PeriodYearly}
		if subscription != "" && status == StatusSucceeded {
			out.Renewal = &Renewal{Kind: RenewalProviderSubscription, Reference: subscription,
				Customer: invoice.Customer, Active: true}
		}
		return out, nil

	case "customer.subscription.deleted":
		var subscription struct {
			ID       string            `json:"id"`
			Customer string            `json:"customer"`
			Metadata map[string]string `json:"metadata"`
		}
		if json.Unmarshal(event.Data.Object, &subscription) != nil || subscription.ID == "" {
			return Event{}, ErrInvalidNotification
		}
		if subscription.Metadata["membership_id"] == "" {
			return ignored, nil
		}
		return Event{Kind: EventRenewalMethod, Provider: StripeCode, Reference: event.ID,
			MembershipID: subscription.Metadata["membership_id"],
			Renewal: &Renewal{Kind: RenewalProviderSubscription, Reference: subscription.ID,
				Customer: subscription.Customer, Active: false}}, nil
	}
	return ignored, nil
}

// SetRenewal implements RenewalSwitch: off sets cancel_at_period_end=true (the paid year runs out,
// no further invoice), on sets it back to false.
// https://docs.stripe.com/api/subscriptions/update#update_subscription-cancel_at_period_end
func (s *Stripe) SetRenewal(ctx context.Context, credentials Credentials, renewal Renewal, on bool) error {
	config, err := s.config(credentials)
	if err != nil {
		return err
	}
	if renewal.Kind != RenewalProviderSubscription || !strings.HasPrefix(renewal.Reference, "sub_") {
		return ErrNotSupported
	}
	form := url.Values{}
	form.Set("cancel_at_period_end", strconv.FormatBool(!on))
	var answer struct {
		ID                string `json:"id"`
		CancelAtPeriodEnd bool   `json:"cancel_at_period_end"`
	}
	key := "renewal-" + renewal.Reference + "-" + strconv.FormatBool(on) + "-" + strconv.FormatInt(s.now().Unix(), 10)
	if err := s.call(ctx, config, http.MethodPost, "/v1/subscriptions/"+url.PathEscape(renewal.Reference), form, key, &answer); err != nil {
		return err
	}
	if answer.ID != renewal.Reference || answer.CancelAtPeriodEnd == on {
		return ErrProviderUnavailable
	}
	return nil
}

// stripePaymentOf finds what paid an invoice (https://docs.stripe.com/api/invoice-payment/list):
// a payment_intent, or a charge.
func (s *Stripe) stripePaymentOf(ctx context.Context, config Config, invoiceID string) (url.Values, error) {
	query := url.Values{}
	query.Set("invoice", invoiceID)
	query.Set("status", "paid")
	var list struct {
		Data []struct {
			Payment struct {
				Type          string `json:"type"`
				PaymentIntent string `json:"payment_intent"`
				Charge        string `json:"charge"`
			} `json:"payment"`
		} `json:"data"`
	}
	if err := s.call(ctx, config, http.MethodGet, "/v1/invoice_payments", query, "", &list); err != nil {
		return nil, err
	}
	for _, item := range list.Data {
		target := url.Values{}
		switch {
		case item.Payment.PaymentIntent != "":
			target.Set("payment_intent", item.Payment.PaymentIntent)
			return target, nil
		case item.Payment.Charge != "":
			target.Set("charge", item.Payment.Charge)
			return target, nil
		}
	}
	return nil, ErrProviderRefused
}

// Refund implements Provider: the whole invoice back (the operator's exceptional act; a person
// is never refunded). https://docs.stripe.com/api/refunds/create
func (s *Stripe) Refund(ctx context.Context, credentials Credentials, event Event) error {
	config, err := s.config(credentials)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(event.Reference, "in_") {
		return ErrNotSupported
	}
	target, err := s.stripePaymentOf(ctx, config, event.Reference)
	if err != nil {
		return err
	}
	target.Set("metadata[membership_id]", event.MembershipID)
	var refund struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := s.call(ctx, config, http.MethodPost, "/v1/refunds", target, "refund-"+event.Reference, &refund); err != nil {
		return err
	}
	switch refund.Status {
	case "succeeded", "pending", "requires_action":
		return nil
	}
	return ErrProviderRefused
}

// Status implements Provider: what Stripe says of an invoice
// (https://docs.stripe.com/api/invoices/object#invoice_object-status).
func (s *Stripe) Status(ctx context.Context, credentials Credentials, reference string) (Status, error) {
	config, err := s.config(credentials)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(reference, "in_") {
		return "", ErrNotSupported
	}
	var invoice struct {
		Status string `json:"status"`
	}
	if err := s.call(ctx, config, http.MethodGet, "/v1/invoices/"+url.PathEscape(reference), nil, "", &invoice); err != nil {
		return "", err
	}
	switch invoice.Status {
	case "paid":
		return StatusSucceeded, nil
	case "draft", "open":
		return StatusPending, nil
	case "void", "uncollectible":
		return StatusFailed, nil
	}
	return "", ErrProviderUnavailable
}
