// Source: bioexamine, backend/internal/payment/payment.go, read at commit 181fccf6
// (last changed there in 602dbeeb, 2026-11-04); copied into CelikPanel on 2026-11-23 under
// the shared catalogue's entry A (same language: copy with a source note). CelikPanel's
// changes after the copy are marked "CelikPanel:" and are in this repository's history.

// Package payment is the seam between a licence and whoever takes the money.
//
// bioexamine, founder, 4 Kasım 2026: payments are modular -- PayTR, iyzico, Stripe and others plug
// in; none is hard-wired. A Provider turns "pay for this" into a checkout, and turns what the
// provider later sends back into something the store records.
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
//   - a webhook body is only a hint, and the API says so: ParseNotification answers an Event
//     that holds identifiers and the kind of hint, no status and no amount. The one way to
//     learn what happened is Confirm, which reads the provider's current object, checks it
//     against what the store expects (order, licence, amount, currency) and answers a Fact:
//     paid, not paid, refunded, set up, or a mismatch;
//   - a paid amount is reported only when money arrived (a subscription set-up is never paid);
//   - payment states only move forward (Advance): a late "failed" never replaces "succeeded";
//   - idempotency keys name the operation, so a retry is the same request to the provider.
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
	// ErrNotSupported: the provider has no such operation.
	ErrNotSupported = errors.New("payment: the provider does not support this")
	// ErrNotConfigured: the provider's credentials are missing or incomplete.
	ErrNotConfigured = errors.New("payment: the provider's credentials are not entered")
	// ErrCheckoutIncomplete: something the provider needs (the return addresses, the order
	// reference, the first charge date of a subscription, an operation key) is missing.
	ErrCheckoutIncomplete = errors.New("payment: the request lacks what the provider needs")
	// ErrProviderRefused: the provider answered and said no (a declined card, a refused refund).
	ErrProviderRefused = errors.New("payment: the provider refused")
	// ErrProviderUnavailable: the provider could not be reached or answered unreadably; try again.
	ErrProviderUnavailable = errors.New("payment: the provider could not be reached")
	// CelikPanel: ErrUnexpectedState: the provider's object is in a shape this package does not
	// apply (for example an invoice marked paid with nothing paid); the operator looks at it.
	ErrUnexpectedState = errors.New("payment: the provider's object is in an unexpected state")
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
	// it and confirms the order with it.
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

// EventKind says what a notification hints at. The zero value is a payment.
type EventKind string

const (
	// EventPayment: a payment may have changed (a one-time checkout or a renewal invoice).
	EventPayment EventKind = ""
	// EventRenewalMethod: the way a licence renews may have changed (a subscription was set up,
	// turned off at the period end, or ended).
	EventRenewalMethod EventKind = "renewal_method"
	// EventRefund: a refund was made or changed on PaymentIntent.
	EventRefund EventKind = "refund"
	// EventDispute: the payer's bank disputes the payment of PaymentIntent. A dispute only flags
	// the order for the operator; it never revokes by itself.
	EventDispute EventKind = "dispute"
	// EventIgnored: a verified notification that concerns nothing we record. The door answers
	// that it was received, so the provider stops sending it.
	EventIgnored EventKind = "ignored"
)

// Event is what a verified notification hints at: identifiers and a kind, nothing to act on.
// CelikPanel (review of #361, 3): it carries no status, amount or renewal state, so a store
// cannot apply a body. The store finds its order by Order, Reference or PaymentIntent, then
// calls Confirm (payments) or ConfirmRenewal (renewal methods) with what it expects.
type Event struct {
	Kind     EventKind
	Provider string
	// EventID is the provider's id for this notification, for the store's
	// UNIQUE(provider, event id): a replay or duplicate is recorded once.
	EventID string
	// Reference is the provider object to confirm: a Checkout Session, an invoice or a
	// PaymentIntent (refund and dispute hints), or a subscription (renewal hints).
	Reference string
	// Order and LicenseID as the body names them: only for finding the store's own order.
	Order     string
	LicenseID string
	// PaymentIntent, Refund, Dispute and Subscription name the provider's objects (ids only).
	PaymentIntent string
	Refund        string
	Dispute       string
	Subscription  string
}

// OrderKind is what an order's provider object is (CelikPanel).
type OrderKind string

const (
	// OrderOneTime: a one-time purchase of a year (Checkout in payment mode, its PaymentIntent).
	OrderOneTime OrderKind = "one_time"
	// OrderSubscriptionSetup: automatic renewal set up (Checkout in subscription mode). No money
	// moves at set-up; it is never "paid".
	OrderSubscriptionSetup OrderKind = "subscription_setup"
	// OrderRenewalInvoice: one year charged by the subscription (an invoice).
	OrderRenewalInvoice OrderKind = "renewal_invoice"
)

// Expected is what the store's order says should have happened: the order the provider must
// echo, the licence, and the amount and currency that must have been paid.
type Expected struct {
	Kind        OrderKind
	Order       string
	LicenseID   string
	AmountMinor int64
	Currency    string
}

// Outcome is what Confirm found.
type Outcome string

const (
	// OutcomePaid: the expected amount arrived for this order and licence.
	OutcomePaid Outcome = "paid"
	// OutcomeNotPaid: nothing arrived (yet): open, processing, failed, cancelled or expired.
	OutcomeNotPaid Outcome = "not_paid"
	// OutcomeRefunded: the payment arrived and was given back in full.
	OutcomeRefunded Outcome = "refunded"
	// OutcomeSetUp: a subscription is in place; nothing was charged (subscription set-up only).
	OutcomeSetUp Outcome = "set_up"
	// OutcomeMismatch: the provider's object is not what the order expects (Fact.Mismatch says
	// which part); nothing may be applied.
	OutcomeMismatch Outcome = "mismatch"
)

// Fact is the only thing a store may apply: the provider's current object, checked against the
// order. PaidMinor is set only when money arrived.
type Fact struct {
	Outcome  Outcome
	Mismatch string // "kind", "order", "licence", "amount" or "currency" when Outcome is mismatch
	// Status is the provider's own state behind the outcome, for Advance.
	Status        Status
	Kind          OrderKind
	Reference     string
	Order         string
	LicenseID     string
	PaidMinor     int64
	Currency      string
	RefundedMinor int64
	Disputed      bool
	PaymentIntent string
	Subscription  string
	Customer      string
}

// Confirmer reads the provider's current object and checks it against the order.
type Confirmer interface {
	Confirm(ctx context.Context, credentials Credentials, reference string, expected Expected) (Fact, error)
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

// RenewalFact is a subscription's current state, read from the provider and checked against the
// licence (CelikPanel). Active: renewal goes on (not cancelled, not set to stop at the period end).
type RenewalFact struct {
	Subscription      string
	LicenseID         string
	Order             string
	Customer          string
	Active            bool
	CancelAtPeriodEnd bool
	Mismatch          bool // the subscription names another licence
}

// RenewalChange turns automatic renewal on or off. Change is the store's id for this change
// (one id per member action): with the licence and the target state it makes the idempotency key,
// so a retried call is the same change and a later change is a new one.
type RenewalChange struct {
	Subscription string
	LicenseID    string
	On           bool
	Change       string
}

// RenewalSwitch is a provider whose subscription must be told when the member turns automatic
// renewal off or on again (Stripe's cancel_at_period_end).
type RenewalSwitch interface {
	SetRenewal(ctx context.Context, credentials Credentials, change RenewalChange) error
}

// RefundRequest gives a confirmed payment back, whole (CelikPanel). Reference is the order's
// provider object (Checkout Session or invoice) or the PaymentIntent; AmountMinor and Currency are
// the order's. Attempt numbers the store's tries: keep it while the outcome is unknown (a timeout
// or a 5xx is asked again with the same key, and the provider answers once), raise it only after a
// definite refusal the store wants to try again (a cached refusal would otherwise come back).
type RefundRequest struct {
	Reference     string
	PaymentIntent string
	Order         string
	LicenseID     string
	AmountMinor   int64
	Currency      string
	Attempt       int
}

// Provider is one way of taking money. Implementations must be safe for concurrent use.
type Provider interface {
	// Code is the provider's code ("manual", "stripe", ...).
	Code() string
	// StartCheckout begins paying for one licence year.
	StartCheckout(ctx context.Context, credentials Credentials, checkout Checkout) (CheckoutStart, error)
	// ParseNotification verifies what the provider sent and answers what it hints at.
	ParseNotification(ctx context.Context, credentials Credentials, notification Notification) (Event, error)
	// Refund asks the provider to give a confirmed payment back, whole.
	Refund(ctx context.Context, credentials Credentials, request RefundRequest) error
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

// ParseEntry reads and checks the operator's entry. The operator is the source here (there is
// no provider to ask), so the answer is a Fact. CelikPanel (review of #361, 9): the status must
// be stated -- an entry without one is refused, never read as paid -- and the licence id must
// be one.
func (Manual) ParseEntry(notification Notification) (Fact, error) {
	var entry ManualEntry
	decoder := json.NewDecoder(strings.NewReader(string(notification.Body)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&entry); err != nil {
		return Fact{}, ErrInvalidNotification
	}
	period := strings.TrimSpace(entry.Period)
	if period == "" {
		period = PeriodYearly
	}
	if period != PeriodYearly {
		return Fact{}, ErrPeriodNotYearly
	}
	status := Status(strings.TrimSpace(entry.Status))
	reference := strings.TrimSpace(entry.Reference)
	license := strings.TrimSpace(entry.LicenseID)
	currency := strings.TrimSpace(entry.Currency)
	if !status.Valid() || reference == "" || len(reference) > 200 ||
		!amountPattern.MatchString(strings.TrimSpace(entry.Amount)) || !currencyPattern.MatchString(currency) ||
		!licensePattern.MatchString(license) || len(entry.Note) > 500 {
		return Fact{}, ErrInvalidNotification
	}
	minor, err := ToMinor(strings.TrimSpace(entry.Amount), currency)
	if err != nil {
		return Fact{}, ErrInvalidNotification
	}
	fact := Fact{Status: status, Kind: OrderOneTime, Reference: reference, LicenseID: license, Currency: currency}
	switch status {
	case StatusSucceeded:
		fact.Outcome, fact.PaidMinor = OutcomePaid, minor
	case StatusRefunded:
		fact.Outcome, fact.PaidMinor, fact.RefundedMinor = OutcomeRefunded, minor, minor
	default:
		fact.Outcome = OutcomeNotPaid
	}
	return fact, nil
}

// ParseNotification implements Provider: the entry's identifiers; ParseEntry gives its facts.
func (m Manual) ParseNotification(_ context.Context, _ Credentials, notification Notification) (Event, error) {
	fact, err := m.ParseEntry(notification)
	if err != nil {
		return Event{}, err
	}
	return Event{Provider: ManualCode, Reference: fact.Reference, LicenseID: fact.LicenseID}, nil
}

// Refund implements Provider: a manual refund is the operator's entry with status refunded.
func (Manual) Refund(context.Context, Credentials, RefundRequest) error { return ErrNotSupported }
