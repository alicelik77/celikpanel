// Source: bioexamine, backend/internal/payment/payment.go, read at commit 181fccf6
// (last changed there in 602dbeeb, 2026-11-04); copied into CelikPanel on 2026-11-23 under
// the shared catalogue's entry A (same language: copy with a source note). CelikPanel's
// changes after the copy are marked "CelikPanel:" and are in this repository's history.

// Package payment is the seam between a membership and whoever takes the money.
//
// Founder, 4 Kasım 2026: payments are modular -- PayTR, iyzico, Stripe and others plug in; none
// is hard-wired. A Provider turns "pay for this membership's next year" into a checkout, and
// turns what the provider later sends back (a webhook, a callback, or -- for the manual provider
// -- the operator's own entry) into one normalized Event. The workspace store records an Event
// with bx_billing.record_payment (000292), which is idempotent on (provider code, provider
// reference): a notification that arrives twice is one payment.
//
// The registry holds Manual (the operator enters a payment by hand) and, since 000298, Stripe,
// iyzico and PayTR (stripe.go, iyzico.go, paytr.go), each written against its provider's official
// documentation and tested against fake servers that answer in the documented formats. A
// provider's credentials live in the database (bx_billing.payment_providers, the ai_providers
// shape) as one JSON object whose fields each provider declares (ConfigSchema): the operator
// enters them on a screen, the store reads them with bx_billing.payment_provider_key and hands
// them to the provider; they are never logged and never returned to a screen -- only the last four
// characters of the provider's main secret are.
//
// Founder decisions this package keeps (4 Kasım 2026, final): yearly plans only, never monthly;
// no refund to a person (Refund exists for the operator's exceptional use); the payer may cancel
// auto-renewal; BioExamine never issues an official invoice.
//
// Why no provider SDK: the repository adds a dependency only with the first module that needs one,
// and none does here -- each provider is a handful of HTTPS calls and one HMAC, all in the standard
// library. An SDK would bring its own HTTP client, retries and logging (the place a key leaks), and
// three SDKs would triple that surface for no behaviour we cannot state in a few lines.
package payment

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"strings"
)

// PeriodYearly is the only period a membership is paid for. There is no monthly plan.
const PeriodYearly = "yearly"

// Status is a payment's state as the provider reports it.
type Status string

const (
	StatusPending   Status = "pending"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
	StatusRefunded  Status = "refunded"
)

// Valid reports whether s is one of the four states bx_billing.payments accepts.
func (s Status) Valid() bool {
	switch s {
	case StatusPending, StatusSucceeded, StatusFailed, StatusRefunded:
		return true
	}
	return false
}

// Sentinel errors. The store turns each into a coded refusal.
var (
	// ErrPeriodNotYearly: a membership is free or yearly; anything else is refused by name.
	ErrPeriodNotYearly = errors.New("payment: a membership is paid for a year")
	// ErrInvalidNotification: the notification did not verify or did not parse.
	ErrInvalidNotification = errors.New("payment: the notification is not valid")
	// ErrNotSupported: the provider has no such operation (the manual provider has no status
	// query; a refund by hand is an entry with status refunded).
	ErrNotSupported = errors.New("payment: the provider does not support this")
	// ErrNotConfigured: the provider's credentials are missing or incomplete.
	ErrNotConfigured = errors.New("payment: the provider's credentials are not entered")
	// ErrCheckoutIncomplete: something the provider needs (the payer's address, the public
	// origin) is missing.
	ErrCheckoutIncomplete = errors.New("payment: the checkout lacks what the provider needs")
	// ErrProviderRefused: the provider answered and said no (a declined card, a refused refund).
	ErrProviderRefused = errors.New("payment: the provider refused")
	// ErrProviderUnavailable: the provider could not be reached or answered unreadably; try again.
	ErrProviderUnavailable = errors.New("payment: the provider could not be reached")
)

// Credentials are a provider's secret as the operator entered it ("" when none). Never logged.
type Credentials string

// Checkout is what a payer asks to pay for: one membership's next year, at the region's price.
// The payer and URL fields are filled by the store and the HTTP layer; a provider that needs one
// that is empty refuses the checkout (ErrCheckoutIncomplete) rather than inventing it.
type Checkout struct {
	MembershipID string
	Region       string
	PlanKind     string
	Amount       string
	Currency     string
	Period       string

	PayerID    string // the payer's principal id
	PayerEmail string
	PayerName  string
	PayerIP    string
	Language   string // "tr" or "en"
	// Origin is the public origin (BIOEXAMINE_PUBLIC_ORIGIN): the payer comes back to
	// Origin+ReturnPath, and a provider that calls back in the browser posts to
	// Origin+"/api/v1/payments/return/<code>".
	Origin string
	// StoredCard is what the provider stored for this membership before (an iyzico cardUserKey,
	// a PayTR utoken), so a second card joins the first; "" when none.
	StoredCard string
}

// CheckoutStart is the provider's answer: where to send the payer, or -- for the manual provider
// -- that the operator records the payment (Instructions is a code the screen says in words).
type CheckoutStart struct {
	Provider     string `json:"provider"`
	Reference    string `json:"reference,omitempty"`
	RedirectURL  string `json:"redirect_url,omitempty"`
	Instructions string `json:"instructions,omitempty"`
	// Form is a form the payer's browser posts straight to the provider (PayTR's Direct API, where
	// the card fields must never reach our server). Nil for a redirect.
	Form *CheckoutForm `json:"form,omitempty"`
}

// CheckoutForm is a form the browser submits to the provider: the hidden fields as given, plus
// the card inputs the screen draws (CardFields), which go to the provider and never to us.
type CheckoutForm struct {
	Action     string            `json:"action"`
	Method     string            `json:"method"`
	Fields     map[string]string `json:"fields"`
	CardFields []string          `json:"card_fields"`
}

// ReturnPath is the membership page a payer comes back to (internal/mail.PathMembership); the
// result is in the query ("?odeme=tamam" or "?odeme=hata").
const ReturnPath = "/ayarlar/uyelik"

// ReturnURL is where a payer lands after a provider's page: the membership page, with outcome
// "tamam" (paid, or being confirmed) or "hata".
func ReturnURL(origin, outcome string) string {
	return strings.TrimRight(origin, "/") + ReturnPath + "?odeme=" + outcome
}

// WebhookPath is the provider's public door (POST, no session).
func WebhookPath(code string) string { return "/api/v1/payments/webhook/" + code }

// BrowserReturnPath is where a provider sends the payer's browser with a result to confirm
// (iyzico's callbackUrl). The store confirms it with the provider before recording anything.
func BrowserReturnPath(code string) string { return "/api/v1/payments/return/" + code }

// Notification is what arrived from a provider: a webhook's headers and body, or the operator's
// entry for the manual provider.
type Notification struct {
	Headers map[string][]string
	Body    []byte
}

// EventKind says what a notification was about. The zero value is a payment.
type EventKind string

const (
	// EventPayment: a payment with a status -- the row bx_billing.record_payment writes.
	EventPayment EventKind = ""
	// EventRenewalMethod: only the way a membership renews changed (a Stripe subscription was
	// created or ended); no money moved. Renewal is set; nothing is written to payments.
	EventRenewalMethod EventKind = "renewal_method"
	// EventIgnored: a verified notification that concerns nothing we record (an event type we do
	// not use). The door answers that it was received, so the provider stops sending it.
	EventIgnored EventKind = "ignored"
)

// Event is one payment fact, normalized: the row bx_billing.record_payment writes.
type Event struct {
	Kind         EventKind
	Provider     string
	Reference    string
	MembershipID string
	Status       Status
	Amount       string
	Currency     string
	Period       string
	Note         string
	// Renewal, when set, is how this membership renews from now on (a Stripe subscription, a card
	// iyzico or PayTR stored); the store writes it with bx_billing.record_payment_subscription.
	Renewal *Renewal
}

// RenewalKind is how a membership's next year is paid without the payer.
type RenewalKind string

const (
	// RenewalProviderSubscription: the provider charges each year itself (Stripe) and reports it.
	RenewalProviderSubscription RenewalKind = "provider_subscription"
	// RenewalStoredCard: the provider stored a card; we charge it when the year ends (iyzico, PayTR).
	RenewalStoredCard RenewalKind = "stored_card"
)

// Renewal is a membership's renewal method at one provider. The references are provider tokens:
// usable only with the merchant's own credentials, still never shown and never logged.
type Renewal struct {
	Kind      RenewalKind
	Reference string // Stripe subscription id, iyzico cardUserKey, PayTR utoken
	Customer  string // Stripe customer id ("" elsewhere)
	Card      string // iyzico cardToken ("" elsewhere; PayTR's ctoken is listed when charging)
	Active    bool
}

// RenewalCharge is one year to charge to a stored card, built by the store from
// bx_billing.claim_due_renewal_charges (000298).
type RenewalCharge struct {
	MembershipID string
	Reference    string // our deterministic reference for this year end; the provider's order id
	Amount       string
	Currency     string
	PayerID      string
	PayerEmail   string
	PayerName    string
	Language     string
	Origin       string
	Renewal      Renewal
}

// Renewer is a provider that can charge a stored card for the next year (iyzico, PayTR). The
// answer is an Event to record: succeeded, failed, or pending when the provider reports later.
type Renewer interface {
	ChargeRenewal(ctx context.Context, credentials Credentials, charge RenewalCharge) (Event, error)
}

// RenewalSwitch is a provider whose subscription must be told when the payer turns auto-renewal
// off or on again (Stripe's cancel_at_period_end). Providers with a stored card need no call: we
// simply do not charge.
type RenewalSwitch interface {
	SetRenewal(ctx context.Context, credentials Credentials, renewal Renewal, on bool) error
}

// WebhookAcknowledger is a provider whose door must answer in a particular way (PayTR expects the
// plain text "OK"). Others get the store's JSON answer.
type WebhookAcknowledger interface {
	WebhookAcknowledgement() (contentType string, body []byte)
}

// Provider is one way of taking money. Implementations must be safe for concurrent use.
type Provider interface {
	// Code is the provider's code in bx_billing.payment_providers ("manual", "paytr", ...).
	Code() string
	// StartCheckout begins paying for one membership year.
	StartCheckout(ctx context.Context, credentials Credentials, checkout Checkout) (CheckoutStart, error)
	// ParseNotification verifies what the provider sent and turns it into an Event.
	ParseNotification(ctx context.Context, credentials Credentials, notification Notification) (Event, error)
	// Refund asks the provider to give a recorded payment back.
	Refund(ctx context.Context, credentials Credentials, event Event) error
	// Status asks the provider what became of a payment it knows by reference.
	Status(ctx context.Context, credentials Credentials, reference string) (Status, error)
}

// Registry is the set of providers this process can use, by code.
type Registry struct {
	providers map[string]Provider
}

// NewRegistry builds a registry; a later provider with the same code replaces an earlier one.
func NewRegistry(providers ...Provider) *Registry {
	r := &Registry{providers: map[string]Provider{}}
	for _, p := range providers {
		r.providers[p.Code()] = p
	}
	return r
}

// DefaultRegistry is what the product runs with: the manual provider and the three integrated
// ones. A provider is offered to payers only when the operator has entered its credentials and
// enabled it in a region; until then it is listed and inert.
func DefaultRegistry() *Registry {
	// CelikPanel: iyzico and PayTR were not copied (the owner chose Stripe first on
	// 2026-11-23; bioexamine's adapters stay the reference if a Turkish provider is
	// added later).
	return NewRegistry(Manual{}, &Stripe{})
}

// Get returns the provider with this code.
func (r *Registry) Get(code string) (Provider, bool) {
	if r == nil {
		return nil, false
	}
	p, ok := r.providers[code]
	return p, ok
}

// Codes lists the implemented provider codes, sorted.
func (r *Registry) Codes() []string {
	codes := make([]string, 0, len(r.providers))
	for code := range r.providers {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	return codes
}

// ---------------------------------------------------------------------------
// Manual: the operator enters a payment by hand.

// ManualCode is the manual provider's code.
const ManualCode = "manual"

// InstructionOperatorRecords is the manual checkout's answer: pay as agreed; the operator
// records it and the membership becomes active then.
const InstructionOperatorRecords = "operator_records_payment"

// Manual is the operator-entered provider. Its "notification" is the operator's entry (JSON).
type Manual struct{}

// Code implements Provider.
func (Manual) Code() string { return ManualCode }

// StartCheckout implements Provider: nothing to redirect to; the operator records the payment.
func (Manual) StartCheckout(_ context.Context, _ Credentials, checkout Checkout) (CheckoutStart, error) {
	if checkout.Period != PeriodYearly {
		return CheckoutStart{}, ErrPeriodNotYearly
	}
	return CheckoutStart{Provider: ManualCode, Instructions: InstructionOperatorRecords}, nil
}

// ManualEntry is the operator's entry, the manual provider's notification body.
type ManualEntry struct {
	MembershipID string `json:"membership_id"`
	Reference    string `json:"reference"`
	Status       string `json:"status"`
	Amount       string `json:"amount"`
	Currency     string `json:"currency"`
	Period       string `json:"period"`
	Note         string `json:"note"`
}

var (
	amountPattern   = regexp.MustCompile(`^[0-9]{1,10}(\.[0-9]{1,2})?$`)
	currencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)
)

// ParseNotification implements Provider: the operator's entry, checked field by field.
func (Manual) ParseNotification(_ context.Context, _ Credentials, notification Notification) (Event, error) {
	var entry ManualEntry
	decoder := json.NewDecoder(strings.NewReader(string(notification.Body)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&entry); err != nil {
		return Event{}, ErrInvalidNotification
	}
	period := strings.TrimSpace(entry.Period)
	if period == "" {
		period = PeriodYearly
	}
	if period != PeriodYearly {
		return Event{}, ErrPeriodNotYearly
	}
	status := Status(strings.TrimSpace(entry.Status))
	if status == "" {
		status = StatusSucceeded
	}
	reference := strings.TrimSpace(entry.Reference)
	if !status.Valid() || reference == "" || len(reference) > 200 ||
		!amountPattern.MatchString(strings.TrimSpace(entry.Amount)) ||
		!currencyPattern.MatchString(strings.TrimSpace(entry.Currency)) ||
		strings.TrimSpace(entry.MembershipID) == "" || len(entry.Note) > 500 {
		return Event{}, ErrInvalidNotification
	}
	return Event{
		Provider:     ManualCode,
		Reference:    reference,
		MembershipID: strings.TrimSpace(entry.MembershipID),
		Status:       status,
		Amount:       strings.TrimSpace(entry.Amount),
		Currency:     strings.TrimSpace(entry.Currency),
		Period:       period,
		Note:         strings.TrimSpace(entry.Note),
	}, nil
}

// Refund implements Provider: a manual refund is the operator's entry with status refunded.
func (Manual) Refund(context.Context, Credentials, Event) error { return ErrNotSupported }

// Status implements Provider: the manual provider has nobody to ask.
func (Manual) Status(context.Context, Credentials, string) (Status, error) {
	return "", ErrNotSupported
}
