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
	"testing"
	"time"
)

// STRIPE.
//
// CelikPanel (D-047, amended 2026-11-23): every purchase is a Checkout Session in payment mode
// (one charge, nothing stored for later, cards only so the result is known at once); a
// subscription-mode session exists only when the member turned automatic renewal on, its first
// charge on the licence end. Webhook events are hints: each names the object FetchOrder reads,
// and only that fetched state is applied by the store. bioexamine's checkout was always a yearly
// subscription and it recorded payments from the signed event body; both are changed here.
//
// API calls carry Stripe-Version 2025-03-31.basil (the release that moved an invoice's
// subscription under parent.subscription_details and its payments to /v1/invoice_payments); the
// webhook parser reads both that shape and the older top-level one, because an event's shape
// follows the webhook endpoint's own API version, not ours.
//
// Official documentation followed:
//   - Checkout Session create: https://docs.stripe.com/api/checkout/sessions/create
//   - Checkout Session retrieve: https://docs.stripe.com/api/checkout/sessions/retrieve
//   - PaymentIntent retrieve: https://docs.stripe.com/api/payment_intents/retrieve
//   - Webhook signatures (manual verification): https://docs.stripe.com/webhooks?verify=verify-manually
//   - Invoice object: https://docs.stripe.com/api/invoices/object
//   - Invoice payments list: https://docs.stripe.com/api/invoice-payment/list
//   - Subscription update (cancel_at_period_end): https://docs.stripe.com/api/subscriptions/update
//   - Refund create: https://docs.stripe.com/api/refunds/create
//   - Disputes: https://docs.stripe.com/api/disputes/object
//   - Idempotent requests: https://docs.stripe.com/api/idempotent_requests
//   - Test and live keys: https://docs.stripe.com/keys
//
// Facts still to be confirmed in Stripe's current documentation before live use are named in
// the plan's VERIFY list (V3 card-only payment mode, V4 the subscription's first charge on a
// future date, V8 refunds, V9 disputes, V12 the pinned version).

// StripeCode is Stripe's provider code.
const StripeCode = "stripe"

const (
	stripeAPIBase = "https://api.stripe.com"
	stripeVersion = "2025-03-31.basil"
	// stripeTolerance is the official libraries' default: "a default tolerance of 5 minutes
	// between the timestamp and the current time" (webhooks, "Preventing replay attacks").
	stripeTolerance = 5 * time.Minute
	// CelikPanel: Stripe requires a subscription's trial end at least two days ahead (VERIFY V4).
	stripeMinimumFirstCharge = 48 * time.Hour
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
	// CelikPanel: a test never reaches the real Stripe, whatever it forgot to set (the same
	// guard internal/licensing keeps for the live licence service).
	if testing.Testing() && s.base() == stripeAPIBase {
		return ErrProviderUnavailable
	}
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

// StartCheckout implements Provider. CelikPanel: a Checkout Session in payment mode for every
// purchase (one charge of the list price, card only, nothing saved for a later charge); in
// subscription mode only when checkout.AutomaticRenewal is set, a yearly price whose first
// charge falls on checkout.FirstChargeAt (the licence end) and never earlier.
// https://docs.stripe.com/api/checkout/sessions/create
func (s *Stripe) StartCheckout(ctx context.Context, credentials Credentials, checkout Checkout) (CheckoutStart, error) {
	if checkout.Period != PeriodYearly {
		return CheckoutStart{}, ErrPeriodNotYearly
	}
	config, err := s.config(credentials)
	if err != nil {
		return CheckoutStart{}, err
	}
	if !licensePattern.MatchString(checkout.LicenseID) || LicenseFromReference(checkout.Order) != checkout.LicenseID ||
		!strings.HasPrefix(checkout.SuccessURL, "https://") || !strings.HasPrefix(checkout.CancelURL, "https://") {
		return CheckoutStart{}, ErrCheckoutIncomplete
	}
	kind := byte('C') // the order says which kind it is: C one-time, S subscription
	if checkout.AutomaticRenewal {
		kind = 'S'
	}
	if checkout.Order[2] != kind {
		return CheckoutStart{}, ErrCheckoutIncomplete
	}
	minor, err := ToMinor(checkout.Amount, checkout.Currency)
	if err != nil {
		return CheckoutStart{}, err
	}
	if minor <= 0 {
		return CheckoutStart{}, ErrCheckoutIncomplete
	}
	form := url.Values{}
	form.Set("client_reference_id", checkout.Order)
	form.Set("success_url", checkout.SuccessURL)
	form.Set("cancel_url", checkout.CancelURL)
	form.Set("locale", languageOr(checkout.Language))
	if checkout.PayerEmail != "" {
		form.Set("customer_email", checkout.PayerEmail)
	}
	form.Set("line_items[0][quantity]", "1")
	form.Set("line_items[0][price_data][currency]", strings.ToLower(checkout.Currency))
	form.Set("line_items[0][price_data][unit_amount]", strconv.FormatInt(minor, 10))
	form.Set("line_items[0][price_data][product_data][name]", productName(checkout.Language))
	form.Set("metadata[order]", checkout.Order)
	form.Set("metadata[license_id]", checkout.LicenseID)
	if !checkout.AutomaticRenewal {
		form.Set("mode", "payment")
		// Cards only: the payment is confirmed or refused while the member waits (VERIFY V3).
		form.Set("payment_method_types[0]", "card")
		form.Set("payment_intent_data[metadata][order]", checkout.Order)
		form.Set("payment_intent_data[metadata][license_id]", checkout.LicenseID)
	} else {
		first := checkout.FirstChargeAt
		if first.IsZero() || first.Before(s.now().Add(stripeMinimumFirstCharge)) {
			return CheckoutStart{}, ErrCheckoutIncomplete
		}
		form.Set("mode", "subscription")
		form.Set("line_items[0][price_data][recurring][interval]", "year")
		form.Set("line_items[0][price_data][recurring][interval_count]", "1")
		// The year already paid is not charged again: the first charge is the licence end.
		form.Set("subscription_data[trial_end]", strconv.FormatInt(first.Unix(), 10))
		form.Set("subscription_data[metadata][order]", checkout.Order)
		form.Set("subscription_data[metadata][license_id]", checkout.LicenseID)
		form.Set("payment_method_collection", "always")
	}
	var session struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if err := s.call(ctx, config, http.MethodPost, "/v1/checkout/sessions", form, checkout.Order, &session); err != nil {
		return CheckoutStart{}, err
	}
	if !strings.HasPrefix(session.ID, "cs_") || !strings.HasPrefix(session.URL, "https://") {
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

// subscription answers the invoice's subscription and the licence its metadata names.
func (i stripeInvoice) subscription() (string, string) {
	if i.Parent != nil && i.Parent.SubscriptionDetails != nil {
		return i.Parent.SubscriptionDetails.Subscription, i.Parent.SubscriptionDetails.Metadata["license_id"]
	}
	license := i.Metadata["license_id"]
	if i.SubscriptionDetails != nil && i.SubscriptionDetails.Metadata["license_id"] != "" {
		license = i.SubscriptionDetails.Metadata["license_id"]
	}
	return i.Subscription, license
}

// stripeSession is the part of a Checkout Session we read.
type stripeSession struct {
	ID                string            `json:"id"`
	Mode              string            `json:"mode"`
	Status            string            `json:"status"`
	PaymentStatus     string            `json:"payment_status"`
	ClientReferenceID string            `json:"client_reference_id"`
	Subscription      json.RawMessage   `json:"subscription"`
	Customer          json.RawMessage   `json:"customer"`
	PaymentIntent     json.RawMessage   `json:"payment_intent"`
	AmountTotal       int64             `json:"amount_total"`
	Currency          string            `json:"currency"`
	Metadata          map[string]string `json:"metadata"`
}

// expandableID reads a field Stripe sends either as an id or, expanded, as an object with one.
func expandableID(raw json.RawMessage) string {
	var id string
	if json.Unmarshal(raw, &id) == nil {
		return id
	}
	var object struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(raw, &object) == nil {
		return object.ID
	}
	return ""
}

func (s stripeSession) license() string {
	if license := s.Metadata["license_id"]; license != "" {
		return license
	}
	return LicenseFromReference(s.ClientReferenceID)
}

// stripeCharge is the part of a Charge we read (refunds and disputes are told on it).
type stripeCharge struct {
	ID             string `json:"id"`
	PaymentIntent  string `json:"payment_intent"`
	Amount         int64  `json:"amount"`
	AmountRefunded int64  `json:"amount_refunded"`
	Refunded       bool   `json:"refunded"`
	Disputed       bool   `json:"disputed"`
	Currency       string `json:"currency"`
}

// ParseNotification implements Provider: verify the Stripe-Signature header, then read the event.
// CelikPanel: the answer is a hint naming what to fetch (Event.Reference); the store applies
// FetchOrder's state, never this body.
//
//	checkout.session.completed            payment mode: the payment (paid, or pending for a delayed method)
//	                                      subscription mode: the licence's renewal method
//	checkout.session.async_payment_*      the delayed payment succeeded or failed
//	checkout.session.expired              the member never paid
//	invoice.paid / invoice.payment_failed a renewal year paid or failed (reference = invoice id)
//	charge.refunded, refund.*             a refund (Dashboard refunds included)
//	charge.dispute.*                      a dispute: flag for the operator, never a revocation
//	customer.subscription.updated/deleted automatic renewal on, off at the period end, or ended
//	anything else                         received, nothing recorded
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
	if err := json.Unmarshal(notification.Body, &event); err != nil || event.Type == "" || event.ID == "" {
		return Event{}, ErrInvalidNotification
	}
	ignored := Event{Kind: EventIgnored, Provider: StripeCode, EventID: event.ID, Reference: event.ID}
	switch event.Type {
	case "checkout.session.completed", "checkout.session.async_payment_succeeded",
		"checkout.session.async_payment_failed", "checkout.session.expired":
		var session stripeSession
		if json.Unmarshal(event.Data.Object, &session) != nil || !strings.HasPrefix(session.ID, "cs_") {
			return Event{}, ErrInvalidNotification
		}
		license := session.license()
		if license == "" {
			return ignored, nil // not one of our checkouts
		}
		out := Event{Provider: StripeCode, EventID: event.ID, Reference: session.ID, Order: session.ClientReferenceID,
			LicenseID: license, PaymentIntent: expandableID(session.PaymentIntent), Period: PeriodYearly}
		if session.Mode == "subscription" {
			subscription := expandableID(session.Subscription)
			if event.Type != "checkout.session.completed" || subscription == "" {
				return ignored, nil
			}
			out.Kind = EventRenewalMethod
			out.Renewal = &Renewal{Kind: RenewalProviderSubscription, Reference: subscription,
				Customer: expandableID(session.Customer), Active: true}
			return out, nil
		}
		if session.Mode != "payment" {
			return ignored, nil
		}
		switch {
		case event.Type == "checkout.session.expired":
			out.Status = StatusExpired
		case event.Type == "checkout.session.async_payment_failed":
			out.Status = StatusFailed
		case event.Type == "checkout.session.async_payment_succeeded" || session.PaymentStatus == "paid":
			out.Status = StatusSucceeded
		default:
			out.Status = StatusPending
		}
		if session.AmountTotal > 0 {
			currency := strings.ToUpper(session.Currency)
			amount, err := FromMinor(session.AmountTotal, currency)
			if err != nil {
				return Event{}, ErrInvalidNotification
			}
			out.Amount, out.Currency = amount, currency
		}
		return out, nil

	case "invoice.paid", "invoice.payment_failed":
		var invoice stripeInvoice
		if json.Unmarshal(event.Data.Object, &invoice) != nil || invoice.ID == "" {
			return Event{}, ErrInvalidNotification
		}
		subscription, license := invoice.subscription()
		if license == "" {
			return ignored, nil // not one of our licences' invoices
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
		out := Event{Provider: StripeCode, EventID: event.ID, Reference: invoice.ID, LicenseID: license, Status: status,
			Amount: amount, Currency: currency, Period: PeriodYearly}
		if subscription != "" && status == StatusSucceeded {
			out.Renewal = &Renewal{Kind: RenewalProviderSubscription, Reference: subscription,
				Customer: invoice.Customer, Active: true}
		}
		return out, nil

	case "charge.refunded":
		var charge stripeCharge
		if json.Unmarshal(event.Data.Object, &charge) != nil || charge.ID == "" {
			return Event{}, ErrInvalidNotification
		}
		if !strings.HasPrefix(charge.PaymentIntent, "pi_") {
			return ignored, nil
		}
		currency := strings.ToUpper(charge.Currency)
		amount, err := FromMinor(charge.AmountRefunded, currency)
		if err != nil {
			return Event{}, ErrInvalidNotification
		}
		status := StatusPending // part of the payment was given back; the operator decides
		if charge.Refunded && charge.AmountRefunded >= charge.Amount {
			status = StatusRefunded
		}
		return Event{Kind: EventRefund, Provider: StripeCode, EventID: event.ID, Reference: charge.PaymentIntent,
			PaymentIntent: charge.PaymentIntent, Status: status, Amount: amount, Currency: currency}, nil

	case "refund.created", "refund.updated", "refund.failed":
		var refund struct {
			ID            string `json:"id"`
			PaymentIntent string `json:"payment_intent"`
			Status        string `json:"status"`
			Amount        int64  `json:"amount"`
			Currency      string `json:"currency"`
		}
		if json.Unmarshal(event.Data.Object, &refund) != nil || !strings.HasPrefix(refund.ID, "re_") {
			return Event{}, ErrInvalidNotification
		}
		if !strings.HasPrefix(refund.PaymentIntent, "pi_") {
			return ignored, nil
		}
		currency := strings.ToUpper(refund.Currency)
		amount, err := FromMinor(refund.Amount, currency)
		if err != nil {
			return Event{}, ErrInvalidNotification
		}
		var status Status
		switch refund.Status {
		case "succeeded":
			status = StatusRefunded
		case "failed", "canceled":
			status = StatusFailed
		default: // pending, requires_action
			status = StatusPending
		}
		return Event{Kind: EventRefund, Provider: StripeCode, EventID: event.ID, Reference: refund.PaymentIntent,
			PaymentIntent: refund.PaymentIntent, Refund: refund.ID, Status: status, Amount: amount, Currency: currency}, nil

	case "charge.dispute.created", "charge.dispute.updated", "charge.dispute.closed",
		"charge.dispute.funds_withdrawn", "charge.dispute.funds_reinstated":
		var dispute struct {
			ID            string `json:"id"`
			PaymentIntent string `json:"payment_intent"`
			Status        string `json:"status"`
			Amount        int64  `json:"amount"`
			Currency      string `json:"currency"`
		}
		if json.Unmarshal(event.Data.Object, &dispute) != nil || !strings.HasPrefix(dispute.ID, "dp_") {
			return Event{}, ErrInvalidNotification
		}
		if !strings.HasPrefix(dispute.PaymentIntent, "pi_") {
			return ignored, nil
		}
		return Event{Kind: EventDispute, Provider: StripeCode, EventID: event.ID, Reference: dispute.PaymentIntent,
			PaymentIntent: dispute.PaymentIntent, Dispute: dispute.ID, Note: dispute.Status}, nil

	case "customer.subscription.updated", "customer.subscription.deleted":
		var subscription struct {
			ID                string            `json:"id"`
			Customer          string            `json:"customer"`
			Status            string            `json:"status"`
			CancelAtPeriodEnd bool              `json:"cancel_at_period_end"`
			Metadata          map[string]string `json:"metadata"`
		}
		if json.Unmarshal(event.Data.Object, &subscription) != nil || subscription.ID == "" {
			return Event{}, ErrInvalidNotification
		}
		if subscription.Metadata["license_id"] == "" {
			return ignored, nil
		}
		// Renewal goes on only while the subscription lives and was not told to stop at the
		// period end; the paid year itself is never shortened by this.
		active := event.Type == "customer.subscription.updated" && !subscription.CancelAtPeriodEnd &&
			(subscription.Status == "active" || subscription.Status == "trialing" || subscription.Status == "past_due")
		return Event{Kind: EventRenewalMethod, Provider: StripeCode, EventID: event.ID, Reference: subscription.ID,
			LicenseID: subscription.Metadata["license_id"],
			Renewal: &Renewal{Kind: RenewalProviderSubscription, Reference: subscription.ID,
				Customer: subscription.Customer, Active: active}}, nil
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

// Refund implements Provider: the whole payment back. CelikPanel: a one-time payment is refunded
// through its PaymentIntent (the member's 30-day refund or the operator's), a renewal through its
// invoice; the idempotency key is the paid object, so a repeated request is one refund.
// https://docs.stripe.com/api/refunds/create
func (s *Stripe) Refund(ctx context.Context, credentials Credentials, event Event) error {
	config, err := s.config(credentials)
	if err != nil {
		return err
	}
	var target url.Values
	var paid string
	switch {
	case strings.HasPrefix(event.PaymentIntent, "pi_"):
		paid = event.PaymentIntent
	case strings.HasPrefix(event.Reference, "pi_"):
		paid = event.Reference
	case strings.HasPrefix(event.Reference, "cs_"):
		state, err := s.FetchOrder(ctx, credentials, event.Reference)
		if err != nil {
			return err
		}
		if !strings.HasPrefix(state.PaymentIntent, "pi_") || state.Status != StatusSucceeded {
			return ErrProviderRefused
		}
		paid = state.PaymentIntent
	case strings.HasPrefix(event.Reference, "in_"):
		if target, err = s.stripePaymentOf(ctx, config, event.Reference); err != nil {
			return err
		}
		paid = event.Reference
	default:
		return ErrNotSupported
	}
	if target == nil {
		target = url.Values{}
		target.Set("payment_intent", paid)
	}
	if event.LicenseID != "" {
		target.Set("metadata[license_id]", event.LicenseID)
	}
	if event.Order != "" {
		target.Set("metadata[order]", event.Order)
	}
	var refund struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := s.call(ctx, config, http.MethodPost, "/v1/refunds", target, "refund-"+paid, &refund); err != nil {
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

// stripePaymentIntent is the part of a PaymentIntent we read, with its latest charge expanded.
type stripePaymentIntent struct {
	ID           string            `json:"id"`
	Status       string            `json:"status"`
	Amount       int64             `json:"amount"`
	Currency     string            `json:"currency"`
	Customer     json.RawMessage   `json:"customer"`
	Metadata     map[string]string `json:"metadata"`
	LatestCharge json.RawMessage   `json:"latest_charge"`
}

// FetchOrder implements Fetcher (CelikPanel): the current state of a Checkout Session (cs_), a
// PaymentIntent (pi_) or a renewal invoice (in_), read from Stripe. This, never an event body,
// is what the store applies; a refund shows as refunded only when all of it was given back.
func (s *Stripe) FetchOrder(ctx context.Context, credentials Credentials, reference string) (OrderState, error) {
	config, err := s.config(credentials)
	if err != nil {
		return OrderState{}, err
	}
	switch {
	case strings.HasPrefix(reference, "cs_"):
		query := url.Values{}
		query.Add("expand[]", "payment_intent.latest_charge")
		var session stripeSession
		if err := s.call(ctx, config, http.MethodGet, "/v1/checkout/sessions/"+url.PathEscape(reference), query, "", &session); err != nil {
			return OrderState{}, err
		}
		if session.ID != reference {
			return OrderState{}, ErrProviderUnavailable
		}
		state := OrderState{Reference: reference, Order: session.ClientReferenceID, LicenseID: session.license(),
			Customer: expandableID(session.Customer), PaymentIntent: expandableID(session.PaymentIntent)}
		if session.Mode == "subscription" {
			state.Subscription = expandableID(session.Subscription)
			switch {
			case session.Status == "expired":
				state.Status = StatusExpired
			case session.Status == "complete" && state.Subscription != "":
				state.Status = StatusSucceeded
			default:
				state.Status = StatusPending
			}
			return state, nil
		}
		switch {
		case session.Status == "expired":
			state.Status = StatusExpired
			return state, nil
		case session.Status != "complete" || session.PaymentStatus != "paid":
			state.Status = StatusPending
			return state, nil
		}
		var intent stripePaymentIntent
		if json.Unmarshal(session.PaymentIntent, &intent) != nil || intent.ID == "" {
			// Not expanded (an older endpoint): read the PaymentIntent itself.
			fetched, err := s.FetchOrder(ctx, credentials, state.PaymentIntent)
			if err != nil {
				return OrderState{}, err
			}
			fetched.Reference, fetched.Order, fetched.LicenseID = reference, state.Order, state.LicenseID
			return fetched, nil
		}
		return s.intentState(intent, state)
	case strings.HasPrefix(reference, "pi_"):
		query := url.Values{}
		query.Add("expand[]", "latest_charge")
		var intent stripePaymentIntent
		if err := s.call(ctx, config, http.MethodGet, "/v1/payment_intents/"+url.PathEscape(reference), query, "", &intent); err != nil {
			return OrderState{}, err
		}
		if intent.ID != reference {
			return OrderState{}, ErrProviderUnavailable
		}
		return s.intentState(intent, OrderState{Reference: reference, Order: intent.Metadata["order"], LicenseID: intent.Metadata["license_id"]})
	case strings.HasPrefix(reference, "in_"):
		status, err := s.Status(ctx, credentials, reference)
		if err != nil {
			return OrderState{}, err
		}
		state := OrderState{Reference: reference, Status: status}
		if status != StatusSucceeded {
			return state, nil
		}
		target, err := s.stripePaymentOf(ctx, config, reference)
		if err != nil || target.Get("payment_intent") == "" {
			return state, err
		}
		paid, err := s.FetchOrder(ctx, credentials, target.Get("payment_intent"))
		if err != nil {
			return OrderState{}, err
		}
		paid.Reference = reference
		return paid, nil
	}
	return OrderState{}, ErrNotSupported
}

func (s *Stripe) intentState(intent stripePaymentIntent, state OrderState) (OrderState, error) {
	state.PaymentIntent = intent.ID
	if state.Customer == "" {
		state.Customer = expandableID(intent.Customer)
	}
	currency := strings.ToUpper(intent.Currency)
	amount, err := FromMinor(intent.Amount, currency)
	if err != nil {
		return OrderState{}, ErrProviderUnavailable
	}
	state.Amount, state.Currency = amount, currency
	switch intent.Status {
	case "succeeded":
		state.Status = StatusSucceeded
	case "canceled":
		state.Status = StatusFailed
		return state, nil
	default: // processing, requires_payment_method, requires_confirmation, requires_action, requires_capture
		state.Status = StatusPending
		return state, nil
	}
	var charge stripeCharge
	if json.Unmarshal(intent.LatestCharge, &charge) != nil || charge.ID == "" {
		return state, nil // not expanded: no refund or dispute is known from this answer
	}
	state.Disputed = charge.Disputed
	if charge.AmountRefunded > 0 {
		refunded, err := FromMinor(charge.AmountRefunded, currency)
		if err != nil {
			return OrderState{}, ErrProviderUnavailable
		}
		state.RefundedAmount = refunded
		if charge.Refunded && charge.AmountRefunded >= charge.Amount {
			state.Status = StatusRefunded
		}
	}
	return state, nil
}
