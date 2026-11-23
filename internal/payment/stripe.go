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
	"regexp"
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
// charge on the licence end. A webhook body is only a hint: ParseNotification answers
// identifiers, and Confirm / ConfirmRenewal read Stripe's current object and check it against
// the store's order. bioexamine's checkout was always a yearly subscription and it recorded
// payments from the signed event body; both are changed here.
//
// API calls carry Stripe-Version 2025-03-31.basil (the release that moved an invoice's
// subscription under parent.subscription_details and its payments to /v1/invoice_payments); the
// parsers read both that shape and the older top-level one, because an event's shape follows the
// webhook endpoint's own API version, not ours.
//
// Official documentation followed:
//   - Checkout Session create: https://docs.stripe.com/api/checkout/sessions/create
//   - Checkout Session retrieve: https://docs.stripe.com/api/checkout/sessions/retrieve
//   - PaymentIntent retrieve: https://docs.stripe.com/api/payment_intents/retrieve
//   - Charge retrieve: https://docs.stripe.com/api/charges/retrieve
//   - Subscription retrieve: https://docs.stripe.com/api/subscriptions/retrieve
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

// ConfigSchema implements Configurable. CelikPanel: Currencies is metadata only (what Stripe
// takes); D-047 keeps currencies other than USD open.
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

// stripeHost reports whether base names one of Stripe's own hosts: the parsed host,
// lower-cased, without a trailing dot, whatever the port or path. An address that does
// not parse counts as Stripe's, so the guard fails closed.
func stripeHost(base string) bool {
	parsed, err := url.Parse(strings.TrimSpace(base))
	if err != nil || parsed.Host == "" {
		return true
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	return host == "stripe.com" || strings.HasSuffix(host, ".stripe.com")
}

// call sends one API request: form-encoded, Bearer key, pinned version, an idempotency key on
// every POST (https://docs.stripe.com/api/idempotent_requests). A 4xx is ErrProviderRefused; a 5xx
// or transport failure ErrProviderUnavailable. The answer is decoded into out.
func (s *Stripe) call(ctx context.Context, config Config, method, path string, form url.Values, idempotencyKey string, out any) error {
	// CelikPanel: a test never reaches the real Stripe, whatever it forgot to set or however
	// it spelled the address (the same guard internal/licensing keeps for the live licence
	// service).
	if testing.Testing() && stripeHost(s.base()) {
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
	if status == http.StatusConflict {
		return ErrProviderBusy // another request with this key is in flight: not a refusal
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
// charge falls on checkout.FirstChargeAt (the licence end) and never earlier. The deferral uses
// Stripe's trial_end: what the member is shown for it is open (VERIFY V4, the PR names it).
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
	ID         string `json:"id"`
	Status     string `json:"status"`
	AmountPaid int64  `json:"amount_paid"`
	// Pointers: an absent field is not a stated zero.
	AmountDue     *int64            `json:"amount_due"`
	Total         *int64            `json:"total"`
	BillingReason string            `json:"billing_reason"`
	Livemode      bool              `json:"livemode"`
	Currency      string            `json:"currency"`
	Customer      string            `json:"customer"`
	Metadata      map[string]string `json:"metadata"`
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

// subscription answers the invoice's subscription and the metadata it carries (licence, order).
func (i stripeInvoice) subscription() (string, map[string]string) {
	if i.Parent != nil && i.Parent.SubscriptionDetails != nil {
		return i.Parent.SubscriptionDetails.Subscription, i.Parent.SubscriptionDetails.Metadata
	}
	metadata := i.Metadata
	if i.SubscriptionDetails != nil && i.SubscriptionDetails.Metadata["license_id"] != "" {
		metadata = i.SubscriptionDetails.Metadata
	}
	return i.Subscription, metadata
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
	Metadata          map[string]string `json:"metadata"`
	Livemode          bool              `json:"livemode"`
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
	AmountCaptured int64  `json:"amount_captured"`
	AmountRefunded int64  `json:"amount_refunded"`
	Paid           bool   `json:"paid"`
	Refunded       bool   `json:"refunded"`
	Disputed       bool   `json:"disputed"`
	Currency       string `json:"currency"`
	Livemode       bool   `json:"livemode"`
}

// ParseNotification implements Provider: verify the Stripe-Signature header, then say what the
// event hints at. CelikPanel (review of #361, 3): the answer holds identifiers only; the store
// finds its order and asks Confirm or ConfirmRenewal.
//
//	checkout.session.*                    payment mode: a payment hint on the session
//	                                      subscription mode (completed): a renewal-method hint
//	invoice.paid / invoice.payment_failed a renewal invoice hint
//	charge.refunded, refund.*             a refund hint on the PaymentIntent (Dashboard refunds included)
//	charge.dispute.*                      a dispute hint on the PaymentIntent
//	customer.subscription.updated/deleted a renewal-method hint on the subscription
//	anything else                         received, nothing to do
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
			LicenseID: license, PaymentIntent: expandableID(session.PaymentIntent), Subscription: expandableID(session.Subscription)}
		switch session.Mode {
		case "payment":
			return out, nil
		case "subscription":
			out.Kind = EventRenewalMethod
			return out, nil
		}
		return ignored, nil

	case "invoice.paid", "invoice.payment_failed":
		var invoice stripeInvoice
		if json.Unmarshal(event.Data.Object, &invoice) != nil || !strings.HasPrefix(invoice.ID, "in_") {
			return Event{}, ErrInvalidNotification
		}
		subscription, metadata := invoice.subscription()
		if metadata["license_id"] == "" && subscription == "" {
			return ignored, nil // not one of our licences' invoices
		}
		return Event{Provider: StripeCode, EventID: event.ID, Reference: invoice.ID, LicenseID: metadata["license_id"],
			Order: metadata["order"], Subscription: subscription}, nil

	case "charge.refunded", "charge.dispute.created", "charge.dispute.updated", "charge.dispute.closed",
		"charge.dispute.funds_withdrawn", "charge.dispute.funds_reinstated", "refund.created", "refund.updated", "refund.failed":
		var object struct {
			ID            string `json:"id"`
			PaymentIntent string `json:"payment_intent"`
			Charge        string `json:"charge"`
		}
		if json.Unmarshal(event.Data.Object, &object) != nil || object.ID == "" {
			return Event{}, ErrInvalidNotification
		}
		charge := object.Charge
		if event.Type == "charge.refunded" {
			charge = object.ID // the object is the charge itself
		}
		// Second review of #361, 3: a payment made by a charge alone (some invoices) has no
		// PaymentIntent; the hint names the charge so it is still found.
		reference := object.PaymentIntent
		if !strings.HasPrefix(reference, "pi_") {
			reference = charge
		}
		if !strings.HasPrefix(reference, "pi_") && !strings.HasPrefix(reference, "ch_") && !strings.HasPrefix(reference, "py_") {
			return ignored, nil
		}
		out := Event{Kind: EventRefund, Provider: StripeCode, EventID: event.ID, Reference: reference,
			PaymentIntent: object.PaymentIntent, Charge: charge}
		switch {
		case strings.HasPrefix(event.Type, "charge.dispute."):
			if !strings.HasPrefix(object.ID, "dp_") {
				return Event{}, ErrInvalidNotification
			}
			out.Kind, out.Dispute = EventDispute, object.ID
		case strings.HasPrefix(event.Type, "refund."):
			if !strings.HasPrefix(object.ID, "re_") {
				return Event{}, ErrInvalidNotification
			}
			out.Refund = object.ID
		}
		return out, nil

	case "customer.subscription.updated", "customer.subscription.deleted":
		var subscription struct {
			ID       string            `json:"id"`
			Metadata map[string]string `json:"metadata"`
		}
		if json.Unmarshal(event.Data.Object, &subscription) != nil || !strings.HasPrefix(subscription.ID, "sub_") {
			return Event{}, ErrInvalidNotification
		}
		if subscription.Metadata["license_id"] == "" {
			return ignored, nil
		}
		return Event{Kind: EventRenewalMethod, Provider: StripeCode, EventID: event.ID, Reference: subscription.ID,
			Subscription: subscription.ID, LicenseID: subscription.Metadata["license_id"], Order: subscription.Metadata["order"]}, nil
	}
	return ignored, nil
}

// operationKey is the part of an idempotency key the store supplies: letters, digits, "-" and "_".
var operationKey = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// SetRenewal implements RenewalSwitch: off sets cancel_at_period_end=true (the paid year runs out,
// no further invoice), on sets it back to false. CelikPanel: the idempotency key is the licence,
// the target state and the store's change id (review of #361, 8), and the subscription is read
// first (second review, 3): another licence's or the other mode's is ErrMismatch, one already in
// the asked state is ErrAlreadyInThatState -- both final, nothing changed.
// https://docs.stripe.com/api/subscriptions/update#update_subscription-cancel_at_period_end
func (s *Stripe) SetRenewal(ctx context.Context, credentials Credentials, change RenewalChange) error {
	config, err := s.config(credentials)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(change.Subscription, "sub_") || !licensePattern.MatchString(change.LicenseID) || !operationKey.MatchString(change.Change) {
		return ErrCheckoutIncomplete
	}
	current, err := s.readSubscription(ctx, config, change.Subscription)
	if err != nil {
		return err
	}
	if current.LicenseID != change.LicenseID || current.Live != change.Live {
		return ErrMismatch
	}
	if current.Status != "active" && current.Status != "trialing" && current.Status != "past_due" {
		return ErrSubscriptionEnded // final: an ended subscription cannot be switched again
	}
	if current.CancelAtPeriodEnd == !change.On {
		return ErrAlreadyInThatState
	}
	form := url.Values{}
	form.Set("cancel_at_period_end", strconv.FormatBool(!change.On))
	var answer struct {
		ID                string `json:"id"`
		CancelAtPeriodEnd bool   `json:"cancel_at_period_end"`
	}
	state := "off"
	if change.On {
		state = "on"
	}
	key := "renewal-" + change.LicenseID + "-" + state + "-" + change.Change
	if err := s.call(ctx, config, http.MethodPost, "/v1/subscriptions/"+url.PathEscape(change.Subscription), form, key, &answer); err != nil {
		return err
	}
	if answer.ID != change.Subscription || answer.CancelAtPeriodEnd == change.On {
		return ErrProviderUnavailable
	}
	return nil
}

// invoicePayments lists what paid invoices, filtered by the invoice or by the payment
// (https://docs.stripe.com/api/invoice-payment/list).
type invoicePayment struct {
	Invoice string `json:"invoice"`
	Payment struct {
		Type          string `json:"type"`
		PaymentIntent string `json:"payment_intent"`
		Charge        string `json:"charge"`
	} `json:"payment"`
}

func (s *Stripe) invoicePayments(ctx context.Context, config Config, query url.Values) ([]invoicePayment, error) {
	query.Set("status", "paid")
	var list struct {
		Data []invoicePayment `json:"data"`
	}
	if err := s.call(ctx, config, http.MethodGet, "/v1/invoice_payments", query, "", &list); err != nil {
		return nil, err
	}
	return list.Data, nil
}

// Refund implements Provider: the order's amount back, whole, and only for the order's own,
// fully paid payment. CelikPanel (second review of #361, 3): Refund runs Confirm first and acts
// on its fact; a mismatch, an already (or partly) refunded or an unpaid order is a final answer
// and nothing is sent. The idempotency key is the payment's own id, the amount and the store's
// attempt (see RefundRequest), so two renewal invoices of one subscription never share a key.
// https://docs.stripe.com/api/refunds/create
func (s *Stripe) Refund(ctx context.Context, credentials Credentials, request RefundRequest) error {
	config, err := s.config(credentials)
	if err != nil {
		return err
	}
	if request.Attempt < 0 || request.Expected.AmountMinor <= 0 || !currencyPattern.MatchString(strings.ToUpper(request.Expected.Currency)) {
		return ErrCheckoutIncomplete
	}
	fact, err := s.Confirm(ctx, credentials, request.Reference, request.Expected)
	if err != nil {
		return err
	}
	switch fact.Outcome {
	case OutcomePaid:
		if fact.Disputed {
			return ErrDisputed // final: the provider refuses a refund of a disputed charge
		}
	case OutcomeMismatch:
		return ErrMismatch
	case OutcomeRefunded:
		return ErrAlreadyRefunded
	case OutcomePartlyRefunded:
		return ErrPartlyRefunded
	default:
		return ErrNothingPaid
	}
	target := url.Values{}
	switch {
	case strings.HasPrefix(fact.PaymentIntent, "pi_"):
		target.Set("payment_intent", fact.PaymentIntent)
	case strings.HasPrefix(fact.PaymentID, "ch_") || strings.HasPrefix(fact.PaymentID, "py_"):
		target.Set("charge", fact.PaymentID)
	default:
		return ErrUnexpectedState
	}
	target.Set("amount", strconv.FormatInt(request.Expected.AmountMinor, 10))
	target.Set("metadata[license_id]", request.Expected.LicenseID)
	target.Set("metadata[order]", request.Expected.Order)
	key := "refund-" + fact.PaymentID + "-" + strconv.FormatInt(request.Expected.AmountMinor, 10) +
		strings.ToLower(request.Expected.Currency) + "-a" + strconv.Itoa(request.Attempt)
	var refund struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := s.call(ctx, config, http.MethodPost, "/v1/refunds", target, key, &refund); err != nil {
		return err
	}
	switch refund.Status {
	case "succeeded", "pending", "requires_action":
		return nil
	}
	return ErrProviderRefused
}

// stripePaymentIntent is the part of a PaymentIntent we read, with its latest charge expanded.
type stripePaymentIntent struct {
	ID             string            `json:"id"`
	Status         string            `json:"status"`
	Amount         int64             `json:"amount"`
	AmountReceived int64             `json:"amount_received"`
	Currency       string            `json:"currency"`
	Customer       json.RawMessage   `json:"customer"`
	Metadata       map[string]string `json:"metadata"`
	LatestCharge   json.RawMessage   `json:"latest_charge"`
	Livemode       bool              `json:"livemode"`
}

// orderState is Stripe's current object, before it is checked against an order.
type orderState struct {
	kind          OrderKind
	status        Status
	setUp         bool
	partly        bool
	live          bool
	paidMinor     int64
	currency      string
	refundedMinor int64
	disputed      bool
	order         string
	license       string
	paymentID     string
	paymentIntent string
	subscription  string
	customer      string
}

// fetchOrder reads the current state of a Checkout Session (cs_), a PaymentIntent (pi_), a charge
// (ch_, py_) or a renewal invoice (in_). A paid amount is set only when money arrived; the payment
// id is the same whichever of these the payment was reached through. CelikPanel (review of #361,
// 2; second review, 3): a subscription set-up is never paid; a renewal invoice -- also when reached
// through its PaymentIntent or charge -- is a renewal and names its licence and order from the
// subscription's own metadata; a PaymentIntent that is neither ours nor an invoice's is an error.
func (s *Stripe) fetchOrder(ctx context.Context, config Config, reference string) (orderState, error) {
	switch {
	case strings.HasPrefix(reference, "cs_"):
		query := url.Values{}
		query.Add("expand[]", "payment_intent.latest_charge")
		var session stripeSession
		if err := s.call(ctx, config, http.MethodGet, "/v1/checkout/sessions/"+url.PathEscape(reference), query, "", &session); err != nil {
			return orderState{}, err
		}
		if session.ID != reference {
			return orderState{}, ErrProviderUnavailable
		}
		state := orderState{order: session.ClientReferenceID, license: session.license(), customer: expandableID(session.Customer),
			paymentIntent: expandableID(session.PaymentIntent), subscription: expandableID(session.Subscription), live: session.Livemode}
		switch session.Mode {
		case "subscription":
			state.kind = OrderSubscriptionSetup
			switch {
			case session.Status == "expired":
				state.status = StatusExpired
			case session.Status == "complete" && state.subscription != "":
				state.setUp = true
			default:
				state.status = StatusPending
			}
			return state, nil
		case "payment":
			state.kind = OrderOneTime
		default:
			return orderState{}, ErrUnexpectedState
		}
		switch {
		case session.Status == "expired":
			state.status = StatusExpired
			return state, nil
		case session.Status != "complete" || session.PaymentStatus != "paid":
			state.status = StatusPending
			return state, nil
		}
		var intent stripePaymentIntent
		if json.Unmarshal(session.PaymentIntent, &intent) != nil || intent.ID == "" {
			fetched, err := s.fetchIntent(ctx, config, state.paymentIntent)
			if err != nil {
				return orderState{}, err
			}
			intent = fetched
		}
		return s.intentState(ctx, config, intent, state)
	case strings.HasPrefix(reference, "pi_"):
		intent, err := s.fetchIntent(ctx, config, reference)
		if err != nil {
			return orderState{}, err
		}
		if intent.Metadata["order"] != "" {
			return s.intentState(ctx, config, intent, orderState{kind: OrderOneTime, order: intent.Metadata["order"],
				license: intent.Metadata["license_id"], live: intent.Livemode})
		}
		return s.invoiceOfPayment(ctx, config, url.Values{"payment[type]": {"payment_intent"}, "payment[payment_intent]": {reference}})
	case strings.HasPrefix(reference, "ch_"), strings.HasPrefix(reference, "py_"):
		charge, err := s.fetchCharge(ctx, config, reference)
		if err != nil {
			return orderState{}, err
		}
		if strings.HasPrefix(charge.PaymentIntent, "pi_") {
			return s.fetchOrder(ctx, config, charge.PaymentIntent)
		}
		return s.invoiceOfPayment(ctx, config, url.Values{"payment[type]": {"charge"}, "payment[charge]": {reference}})
	case strings.HasPrefix(reference, "in_"):
		return s.invoiceState(ctx, config, reference)
	}
	return orderState{}, ErrNotSupported
}

// invoiceOfPayment finds the invoice a payment paid; a payment that paid none is not one of
// ours (our one-time payments carry their order) and is an error, never a guessed kind.
func (s *Stripe) invoiceOfPayment(ctx context.Context, config Config, query url.Values) (orderState, error) {
	payments, err := s.invoicePayments(ctx, config, query)
	if err != nil {
		return orderState{}, err
	}
	for _, payment := range payments {
		if strings.HasPrefix(payment.Invoice, "in_") {
			return s.invoiceState(ctx, config, payment.Invoice)
		}
	}
	return orderState{}, ErrUnexpectedState
}

func (s *Stripe) fetchIntent(ctx context.Context, config Config, id string) (stripePaymentIntent, error) {
	if !strings.HasPrefix(id, "pi_") {
		return stripePaymentIntent{}, ErrUnexpectedState
	}
	query := url.Values{}
	query.Add("expand[]", "latest_charge")
	var intent stripePaymentIntent
	if err := s.call(ctx, config, http.MethodGet, "/v1/payment_intents/"+url.PathEscape(id), query, "", &intent); err != nil {
		return stripePaymentIntent{}, err
	}
	if intent.ID != id {
		return stripePaymentIntent{}, ErrProviderUnavailable
	}
	return intent, nil
}

func (s *Stripe) fetchCharge(ctx context.Context, config Config, id string) (stripeCharge, error) {
	if !strings.HasPrefix(id, "ch_") && !strings.HasPrefix(id, "py_") {
		return stripeCharge{}, ErrUnexpectedState
	}
	var charge stripeCharge
	if err := s.call(ctx, config, http.MethodGet, "/v1/charges/"+url.PathEscape(id), nil, "", &charge); err != nil {
		return stripeCharge{}, err
	}
	if charge.ID != id {
		return stripeCharge{}, ErrProviderUnavailable
	}
	return charge, nil
}

// intentState reads a PaymentIntent: money arrived only when it succeeded; its charge tells
// refunds and disputes (fetched when Stripe did not expand it).
func (s *Stripe) intentState(ctx context.Context, config Config, intent stripePaymentIntent, state orderState) (orderState, error) {
	state.paymentIntent, state.paymentID = intent.ID, intent.ID
	if state.customer == "" {
		state.customer = expandableID(intent.Customer)
	}
	switch intent.Status {
	case "succeeded":
	case "canceled":
		state.status = StatusFailed
		return state, nil
	default: // processing, requires_payment_method, requires_confirmation, requires_action, requires_capture
		state.status = StatusPending
		return state, nil
	}
	var charge stripeCharge
	if json.Unmarshal(intent.LatestCharge, &charge) != nil || charge.ID == "" {
		fetched, err := s.fetchCharge(ctx, config, expandableID(intent.LatestCharge))
		if err != nil {
			return orderState{}, err
		}
		charge = fetched
	}
	return chargeState(charge, state)
}

// chargeState: the money that arrived, what was given back, a dispute.
func chargeState(charge stripeCharge, state orderState) (orderState, error) {
	received := charge.AmountCaptured
	if received == 0 && charge.Paid {
		received = charge.Amount
	}
	if !charge.Paid || received <= 0 {
		return orderState{}, ErrUnexpectedState // "succeeded" with no money is never applied
	}
	if state.paymentID == "" {
		state.paymentID = charge.ID // a payment without a PaymentIntent is known by its charge
	}
	state.status = StatusSucceeded
	state.paidMinor, state.currency = received, strings.ToUpper(charge.Currency)
	state.disputed = charge.Disputed
	state.refundedMinor = charge.AmountRefunded
	switch {
	case charge.AmountRefunded >= received:
		state.status = StatusRefunded
	case charge.AmountRefunded > 0:
		state.partly = true
	}
	return state, nil
}

// invoiceState reads a renewal invoice; its licence and order come from the subscription's
// metadata (on the invoice, or read from the subscription itself), never from an event body.
func (s *Stripe) invoiceState(ctx context.Context, config Config, reference string) (orderState, error) {
	var invoice stripeInvoice
	if err := s.call(ctx, config, http.MethodGet, "/v1/invoices/"+url.PathEscape(reference), nil, "", &invoice); err != nil {
		return orderState{}, err
	}
	if invoice.ID != reference {
		return orderState{}, ErrProviderUnavailable
	}
	subscription, metadata := invoice.subscription()
	state := orderState{kind: OrderRenewalInvoice, subscription: subscription, customer: invoice.Customer,
		license: metadata["license_id"], order: metadata["order"], live: invoice.Livemode}
	if (state.license == "" || state.order == "") && strings.HasPrefix(subscription, "sub_") {
		fact, err := s.readSubscription(ctx, config, subscription)
		if err != nil {
			return orderState{}, err
		}
		state.license, state.order = fact.LicenseID, fact.Order
	}
	switch invoice.Status {
	case "draft", "open":
		state.status = StatusPending
		return state, nil
	case "void", "uncollectible":
		state.status = StatusFailed
		return state, nil
	case "paid":
	default:
		return orderState{}, ErrProviderUnavailable
	}
	if invoice.AmountPaid <= 0 {
		if invoice.BillingReason == "subscription_create" && invoice.AmountDue != nil && *invoice.AmountDue == 0 &&
			invoice.Total != nil && *invoice.Total == 0 {
			// The $0 invoice of a subscription set up with a deferred first charge (V4,
			// provider behaviour to verify): its creation invoice states a zero total and
			// nothing due; nothing was charged and nothing is to be applied. Any other paid
			// invoice with nothing paid stays an anomaly.
			state.kind = OrderSetupInvoice
			return state, nil
		}
		return orderState{}, ErrUnexpectedState // marked paid with nothing paid: never a bare "succeeded"
	}
	payments, err := s.invoicePayments(ctx, config, url.Values{"invoice": {reference}})
	if err != nil {
		return orderState{}, err
	}
	var charge stripeCharge
	found := false
	for _, payment := range payments {
		switch {
		case payment.Payment.PaymentIntent != "":
			intent, err := s.fetchIntent(ctx, config, payment.Payment.PaymentIntent)
			if err != nil {
				return orderState{}, err
			}
			if intent.Status != "succeeded" {
				return orderState{}, ErrUnexpectedState
			}
			state.paymentIntent, state.paymentID = intent.ID, intent.ID
			if json.Unmarshal(intent.LatestCharge, &charge) != nil || charge.ID == "" {
				if charge, err = s.fetchCharge(ctx, config, expandableID(intent.LatestCharge)); err != nil {
					return orderState{}, err
				}
			}
			found = true
		case payment.Payment.Charge != "":
			if charge, err = s.fetchCharge(ctx, config, payment.Payment.Charge); err != nil {
				return orderState{}, err
			}
			found = true
		}
		if found {
			break
		}
	}
	if !found {
		return orderState{}, ErrUnexpectedState
	}
	state, err = chargeState(charge, state)
	if err != nil {
		return orderState{}, err
	}
	if state.paidMinor != invoice.AmountPaid || state.currency != strings.ToUpper(invoice.Currency) {
		return orderState{}, ErrUnexpectedState
	}
	return state, nil
}

// subscriptionState is a subscription as read from Stripe.
type subscriptionState struct {
	RenewalFact
	Status string
}

// readSubscription is a subscription's current state.
func (s *Stripe) readSubscription(ctx context.Context, config Config, id string) (subscriptionState, error) {
	var subscription struct {
		ID                string            `json:"id"`
		Status            string            `json:"status"`
		CancelAtPeriodEnd bool              `json:"cancel_at_period_end"`
		Customer          string            `json:"customer"`
		Metadata          map[string]string `json:"metadata"`
		Livemode          bool              `json:"livemode"`
	}
	if err := s.call(ctx, config, http.MethodGet, "/v1/subscriptions/"+url.PathEscape(id), nil, "", &subscription); err != nil {
		return subscriptionState{}, err
	}
	if subscription.ID != id {
		return subscriptionState{}, ErrProviderUnavailable
	}
	live := subscription.Status == "active" || subscription.Status == "trialing" || subscription.Status == "past_due"
	return subscriptionState{Status: subscription.Status, RenewalFact: RenewalFact{Subscription: id,
		LicenseID: subscription.Metadata["license_id"], Order: subscription.Metadata["order"], Customer: subscription.Customer,
		CancelAtPeriodEnd: subscription.CancelAtPeriodEnd, Live: subscription.Livemode,
		Active: live && !subscription.CancelAtPeriodEnd}}, nil
}

// Confirm implements Confirmer (CelikPanel, review of #361, 3): the one way to learn what
// happened to an order. It reads Stripe's current object and checks it against the order: the
// mode (a test payment never confirms a live order), the kind, our order reference, the licence,
// and -- when money arrived -- the amount and currency. Anything that differs is a mismatch and
// nothing may be applied. A $0 set-up invoice is "nothing charged"; a partial refund is its own
// outcome (the operator decides).
func (s *Stripe) Confirm(ctx context.Context, credentials Credentials, reference string, expected Expected) (Fact, error) {
	config, err := s.config(credentials)
	if err != nil {
		return Fact{}, err
	}
	if expected.Order == "" || !licensePattern.MatchString(expected.LicenseID) || expected.Kind == "" {
		return Fact{}, ErrCheckoutIncomplete
	}
	state, err := s.fetchOrder(ctx, config, reference)
	if err != nil {
		return Fact{}, err
	}
	fact := Fact{Status: state.status, Kind: state.kind, Reference: reference, Order: state.order, LicenseID: state.license,
		PaidMinor: state.paidMinor, Currency: state.currency, RefundedMinor: state.refundedMinor, Disputed: state.disputed,
		PaymentID: state.paymentID, PaymentIntent: state.paymentIntent, Subscription: state.subscription, Customer: state.customer,
		Live: state.live}
	mismatch := func(what string) (Fact, error) {
		fact.Outcome, fact.Mismatch = OutcomeMismatch, what
		return fact, nil
	}
	if state.live != expected.Live {
		return mismatch("mode")
	}
	if state.kind == OrderSetupInvoice {
		if state.license != expected.LicenseID {
			return mismatch("licence")
		}
		fact.Outcome = OutcomeNothingCharged
		return fact, nil
	}
	switch {
	case state.kind != expected.Kind:
		return mismatch("kind")
	case state.order != expected.Order:
		return mismatch("order")
	case state.license != expected.LicenseID:
		return mismatch("licence")
	}
	if state.kind == OrderSubscriptionSetup {
		fact.Outcome = OutcomeNotPaid
		if state.setUp {
			fact.Outcome = OutcomeSetUp
		}
		return fact, nil
	}
	switch state.status {
	case StatusSucceeded, StatusRefunded:
		if state.paidMinor != expected.AmountMinor {
			return mismatch("amount")
		}
		if !strings.EqualFold(state.currency, expected.Currency) {
			return mismatch("currency")
		}
		switch {
		case state.status == StatusRefunded:
			fact.Outcome = OutcomeRefunded
		case state.partly:
			fact.Outcome = OutcomePartlyRefunded
		default:
			fact.Outcome = OutcomePaid
		}
	default:
		fact.Outcome = OutcomeNotPaid
	}
	return fact, nil
}

// ConfirmRenewal reads a subscription's current state for a licence in the order's mode
// (renewal-method hints).
func (s *Stripe) ConfirmRenewal(ctx context.Context, credentials Credentials, subscription, licenseID string, live bool) (RenewalFact, error) {
	config, err := s.config(credentials)
	if err != nil {
		return RenewalFact{}, err
	}
	if !strings.HasPrefix(subscription, "sub_") || !licensePattern.MatchString(licenseID) {
		return RenewalFact{}, ErrCheckoutIncomplete
	}
	current, err := s.readSubscription(ctx, config, subscription)
	if err != nil {
		return RenewalFact{}, err
	}
	if current.LicenseID != licenseID || current.Live != live {
		return RenewalFact{Subscription: subscription, LicenseID: current.LicenseID, Live: current.Live, Mismatch: true}, nil
	}
	return current.RenewalFact, nil
}
