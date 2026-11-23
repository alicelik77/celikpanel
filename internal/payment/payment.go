// Source: bioexamine, backend/internal/payment/payment.go, read at commit 181fccf6
// (last changed there in 602dbeeb, 2026-11-04); copied into CelikPanel on 2026-11-23 under
// the shared catalogue's entry A (same language: copy with a source note). CelikPanel's
// changes after the copy are marked "CelikPanel:" and are in this repository's history.

// Package payment is the seam between a licence and whoever takes the money.
//
// bioexamine, founder, 4 Kasım 2026: payments are modular -- PayTR, iyzico, Stripe and others plug
// in; none is hard-wired. A Provider turns "pay for this" into a checkout, and turns what the
// provider later sends back (a webhook, or -- for the manual provider -- the operator's own entry)
// into one normalized Event.
//
// CelikPanel (D-047, amended 2026-11-23): Stripe is the first provider; the seam stays
// provider-neutral. The member service stays the only issuer and signer of licences: a provider
// only reports what was paid, refunded or disputed, and never knows a licence key. Rules this
// package keeps for CelikPanel, each with a test:
//
//   - every purchase is a one-time payment (Checkout.AutomaticRenewal false, the zero value): no
//     card is stored for a later charge;
//   - a provider subscription exists only when the member turned automatic renewal on, its first
//     charge on the licence end (Checkout.FirstChargeAt), one year at a time, no bonus months;
//   - a webhook body is a hint, never the truth: every Event names a provider object that
//     Fetcher.FetchOrder reads back, and only the fetched state is applied;
//   - payment states only move forward (Advance): a late "failed" never replaces "succeeded";
//   - a refund of a one-time payment (a PaymentIntent) as well as of an invoice.
//
// Credentials: in CelikPanel they come from the private configuration on the portal host, never
// from a database, a page or a message (bioexamine stores them in its database). This package
// only parses and checks them (ParseConfig, CheckStripeConfig).
//
// Why no provider SDK (bioexamine): each provider is a handful of HTTPS calls and one HMAC, all in
// the standard library. An SDK would bring its own HTTP client, retries and logging (the place a
// key leaks).
package payment

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"strings"
	"time"
)

// PeriodYearly is the only period a licence is paid for. There is no monthly plan.
const PeriodYearly = "yearly"

// Status is a payment's state as the provider reports it.
type Status string

const (
	StatusPending   Status = "pending"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
	StatusRefunded  Status = "refunded"
	// CelikPanel: a checkout the payer never completed (Stripe's session expired).
	StatusExpired Status = "expired"
)

// Valid reports whether s is one of the known states.
func (s Status) Valid() bool {
	switch s {
	case StatusPending, StatusSucceeded, StatusFailed, StatusRefunded, StatusExpired:
		return true
	}
	return false
}

// CelikPanel: states only move forward. A payment is first unknown, then pending, then ended
// without money (failed, expired) or paid (succeeded), and a paid one can be refunded. The
// provider's fetched state is the truth, so money that did arrive after a recorded failure is
// recorded (failed -> succeeded), but nothing ever moves back: a late or replayed "failed",
// "pending" or "expired" never replaces "succeeded", and nothing replaces "refunded". This closes
// the gap the shared catalogue names in bioexamine's record_payment (any status overwrote any).
var statusRank = map[Status]int{"": 0, StatusPending: 1, StatusFailed: 2, StatusExpired: 2, StatusSucceeded: 3, StatusRefunded: 4}

// Advance answers the state to keep when the provider reports next for a payment now in
// current, and whether it changed.
func Advance(current, next Status) (Status, bool) {
	if !next.Valid() || (current != "" && !current.Valid()) {
		return current, false
	}
	if statusRank[next] > statusRank[current] {
		return next, true
	}
	return current, false
}

// Sentinel errors. The store turns each into a coded refusal.
var (
	// ErrPeriodNotYearly: a licence is paid for a year; anything else is refused by name.
	ErrPeriodNotYearly = errors.New("payment: a licence is paid for a year")
	// ErrInvalidNotification: the notification did not verify or did not parse.
	ErrInvalidNotification = errors.New("payment: the notification is not valid")
	// ErrNotSupported: the provider has no such operation (the manual provider has no status
	// query; a refund by hand is an entry with status refunded).
	ErrNotSupported = errors.New("payment: the provider does not support this")
	// ErrNotConfigured: the provider's credentials are missing or incomplete.
	ErrNotConfigured = errors.New("payment: the provider's credentials are not entered")
	// ErrCheckoutIncomplete: something the provider needs (the return addresses, the order
	// reference, the first charge date of a subscription) is missing.
	ErrCheckoutIncomplete = errors.New("payment: the checkout lacks what the provider needs")
	// ErrProviderRefused: the provider answered and said no (a declined card, a refused refund).
	ErrProviderRefused = errors.New("payment: the provider refused")
	// ErrProviderUnavailable: the provider could not be reached or answered unreadably; try again.
	ErrProviderUnavailable = errors.New("payment: the provider could not be reached")
)

// Credentials are a provider's secret as the owner placed it ("" when none). Never logged.
type Credentials string

// Checkout is what a member asks to pay for: one licence's year at the list price.
// CelikPanel: a licence and our order reference instead of bioexamine's membership and region;
// explicit return addresses instead of a fixed return page; a subscription only on request.
type Checkout struct {
	// LicenseID is the licence the year is for (32 hex).
	LicenseID string
	// Order is our order reference (NewOrderReference), made by the store before the call: the
	// provider echoes it, and it is the idempotency key, so a retried call is the same checkout.
	Order    string
	Amount   string
	Currency string
	Period   string

	PayerEmail string
	Language   string // "tr" or "en"
	// SuccessURL and CancelURL are where the payer's browser returns (https).
	SuccessURL string
	CancelURL  string

	// AutomaticRenewal is true only when the member turned automatic renewal on. The zero value
	// is a one-time payment: nothing is stored for a later charge.
	AutomaticRenewal bool
	// FirstChargeAt is the licence end, where a subscription's first charge falls; required with
	// AutomaticRenewal (the year already paid is not charged again).
	FirstChargeAt time.Time
}

// CheckoutStart is the provider's answer: where to send the payer, or -- for the manual provider
// -- that the operator records the payment (Instructions is a code the screen says in words).
type CheckoutStart struct {
	Provider string `json:"provider"`
	// Reference is the provider's checkout object (a Stripe Checkout Session id): the store keeps
	// it and FetchOrder reads its state.
	Reference    string `json:"reference,omitempty"`
	RedirectURL  string `json:"redirect_url,omitempty"`
	Instructions string `json:"instructions,omitempty"`
}

// Notification is what arrived from a provider: a webhook's headers and body, or the operator's
// entry for the manual provider.
type Notification struct {
	Headers map[string][]string
	Body    []byte
}

// EventKind says what a notification was about. The zero value is a payment.
type EventKind string

const (
	// EventPayment: a payment's state (a one-time checkout or a renewal invoice).
	EventPayment EventKind = ""
	// EventRenewalMethod: only the way a licence renews changed (a subscription was created,
	// turned off at the period end, or ended); no money moved.
	EventRenewalMethod EventKind = "renewal_method"
	// CelikPanel: EventRefund: a refund was made or changed; Status is the refund's own
	// (refunded = done, pending, failed), Amount the amount refunded so far.
	EventRefund EventKind = "refund"
	// CelikPanel: EventDispute: the payer's bank disputes a payment; Note is the dispute's
	// state. A dispute only flags the order for the operator; it never revokes by itself.
	EventDispute EventKind = "dispute"
	// EventIgnored: a verified notification that concerns nothing we record. The door answers
	// that it was received, so the provider stops sending it.
	EventIgnored EventKind = "ignored"
)

// Event is one payment fact, normalized. CelikPanel: it is a hint -- the store fetches the state
// of Reference (FetchOrder) and applies that, never the event body.
type Event struct {
	Kind     EventKind
	Provider string
	// EventID is the provider's id for this notification, for the store's
	// UNIQUE(provider, event id): a replay or duplicate is recorded once.
	EventID string
	// Reference is the provider object to fetch: a Checkout Session, a PaymentIntent or an
	// invoice id.
	Reference string
	// Order is our order reference when the provider echoed it; LicenseID the licence.
	Order     string
	LicenseID string
	Status    Status
	Amount    string
	Currency  string
	Period    string
	Note      string
	// PaymentIntent, Refund and Dispute name the provider's objects (ids only).
	PaymentIntent string
	Refund        string
	Dispute       string
	// Renewal, when set, is how this licence renews from now on.
	Renewal *Renewal
}

// RenewalKind is how a licence's next year is paid without the member acting.
type RenewalKind string

const (
	// RenewalProviderSubscription: the provider charges each year itself (Stripe) and reports it.
	RenewalProviderSubscription RenewalKind = "provider_subscription"
	// RenewalStoredCard: the provider stored a card; we charge it when the year ends (bioexamine's
	// iyzico and PayTR; kept in the seam for a later provider).
	RenewalStoredCard RenewalKind = "stored_card"
)

// Renewal is a licence's renewal method at one provider. The references are provider tokens:
// usable only with the merchant's own credentials, still never shown and never logged.
type Renewal struct {
	Kind      RenewalKind
	Reference string // Stripe subscription id
	Customer  string // Stripe customer id
	Card      string // a stored-card provider's card token
	Active    bool
}

// RenewalCharge is one year to charge to a stored card.
type RenewalCharge struct {
	LicenseID  string
	Reference  string // our deterministic reference for this year end; the provider's order id
	Amount     string
	Currency   string
	PayerEmail string
	Language   string
	Renewal    Renewal
}

// Renewer is a provider that can charge a stored card for the next year. The answer is an Event
// to record: succeeded, failed, or pending when the provider reports later.
type Renewer interface {
	ChargeRenewal(ctx context.Context, credentials Credentials, charge RenewalCharge) (Event, error)
}

// RenewalSwitch is a provider whose subscription must be told when the member turns automatic
// renewal off or on again (Stripe's cancel_at_period_end).
type RenewalSwitch interface {
	SetRenewal(ctx context.Context, credentials Credentials, renewal Renewal, on bool) error
}

// OrderState is what the provider says now about one of its objects (CelikPanel). It is the
// truth the store applies; amounts are decimal strings with two places.
type OrderState struct {
	// Reference is the object that was read.
	Reference string
	// Subscription: a subscription checkout (no money moved at checkout); Status is then the
	// set-up's: pending, succeeded (in place) or expired.
	Subscription  string
	Status        Status
	Amount        string
	Currency      string
	Order         string
	LicenseID     string
	PaymentIntent string
	Customer      string
	// RefundedAmount is what was given back so far ("" when nothing); Status is refunded only
	// when everything was.
	RefundedAmount string
	// Disputed: the payer's bank disputes the payment (the operator decides; never automatic).
	Disputed bool
}

// Fetcher reads the provider's current state of an object an Event or a checkout named.
type Fetcher interface {
	FetchOrder(ctx context.Context, credentials Credentials, reference string) (OrderState, error)
}

// Provider is one way of taking money. Implementations must be safe for concurrent use.
type Provider interface {
	// Code is the provider's code ("manual", "stripe", ...).
	Code() string
	// StartCheckout begins paying for one licence year.
	StartCheckout(ctx context.Context, credentials Credentials, checkout Checkout) (CheckoutStart, error)
	// ParseNotification verifies what the provider sent and turns it into an Event (a hint).
	ParseNotification(ctx context.Context, credentials Credentials, notification Notification) (Event, error)
	// Refund asks the provider to give a recorded payment back, whole.
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

// DefaultRegistry is what the product runs with: the manual provider and Stripe. A provider is
// offered to members only when the owner has placed its credentials; until then it is inert.
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
// Manual: the operator enters a payment by hand (CelikPanel: the operator's "gift a year").

// ManualCode is the manual provider's code.
const ManualCode = "manual"

// InstructionOperatorRecords is the manual checkout's answer: the operator records the payment
// and the licence is extended then.
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
	LicenseID string `json:"license_id"`
	Reference string `json:"reference"`
	Status    string `json:"status"`
	Amount    string `json:"amount"`
	Currency  string `json:"currency"`
	Period    string `json:"period"`
	Note      string `json:"note"`
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
		strings.TrimSpace(entry.LicenseID) == "" || len(entry.Note) > 500 {
		return Event{}, ErrInvalidNotification
	}
	return Event{
		Provider:  ManualCode,
		Reference: reference,
		LicenseID: strings.TrimSpace(entry.LicenseID),
		Status:    status,
		Amount:    strings.TrimSpace(entry.Amount),
		Currency:  strings.TrimSpace(entry.Currency),
		Period:    period,
		Note:      strings.TrimSpace(entry.Note),
	}, nil
}

// Refund implements Provider: a manual refund is the operator's entry with status refunded.
func (Manual) Refund(context.Context, Credentials, Event) error { return ErrNotSupported }

// Status implements Provider: the manual provider has nobody to ask.
func (Manual) Status(context.Context, Credentials, string) (Status, error) {
	return "", ErrNotSupported
}
