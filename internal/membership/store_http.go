package membership

// The store's doors (S2, step 2): "Buy a year" on the member's licence (an order and the
// provider's checkout), the order page the member returns to (settled by Confirm, never by
// the return address's parameters), the webhook route, and reconciliation. Nothing here
// trusts a notification's body: every state comes from Confirm against the order's stored
// expectation (the list "what the next package must respect" in PR #361).

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/payment"
)

// maxWebhookBody is the webhook route's body limit (1 MiB).
const maxWebhookBody = 1 << 20

// Payment readiness, as check-db and the pages say it (never a value).
const (
	PaymentNotSet    = "not set"
	PaymentNotUsable = "not usable"
	PaymentReady     = "set"
)

// The member-facing refusals of the payment step.
const (
	ErrPaymentNotReady    Problem = "payment_not_ready"
	ErrPaymentUnavailable Problem = "payment_unavailable"
)

func (h *Handler) payments() *payment.Registry {
	if h.Payments != nil {
		return h.Payments
	}
	return payment.DefaultRegistry()
}

// PaymentState says whether the configured provider's credentials can be used for the
// configured charging mode: test charging accepts only the provider's test keys.
func PaymentState(registry *payment.Registry, config Config) string {
	if config.PaymentProvider == "" || strings.TrimSpace(string(config.PaymentCredentials)) == "" {
		return PaymentNotSet
	}
	provider, ok := registry.WebhookProvider(config.PaymentProvider)
	if !ok {
		return PaymentNotUsable
	}
	parsed, err := payment.ParseConfig(config.PaymentCredentials)
	if err != nil {
		return PaymentNotUsable
	}
	if config.Charging != ChargingLive && parsed.Mode() != payment.ModeTest {
		return PaymentNotUsable // test charging takes test keys only
	}
	if provider.Code() == payment.StripeCode && payment.CheckStripeConfig(parsed) != nil {
		return PaymentNotUsable
	}
	return PaymentReady
}

// chargingProvider is the provider for new orders, or the member-facing refusal.
func (x *exchange) chargingProvider(service *Service, config Config) (payment.Provider, error) {
	switch config.Charging {
	case ChargingTest:
	case ChargingLive:
		return nil, ErrChargingLiveDisabled
	default:
		return nil, ErrChargingOff
	}
	if !service.StoreReady() {
		return nil, ErrChargingOff
	}
	if PaymentState(x.h.payments(), config) != PaymentReady {
		x.h.log("charging is on but the payment provider's keys are not set or not usable")
		return nil, ErrPaymentNotReady
	}
	provider, _ := x.h.payments().WebhookProvider(config.PaymentProvider)
	return provider, nil
}

// buy is the "Buy a year" form: an order (a second click returns the same one), the
// provider's checkout for it (idempotent on the order id), then the member goes to the
// provider's page.
func (x *exchange) buy(service *Service, config Config, member *Member, licenseID, lang string) (string, error) {
	provider, err := x.chargingProvider(service, config)
	if err != nil {
		return "", err
	}
	// Its own bucket (the reviews' list, 10): a member starts at most 20 checkouts an hour.
	if err := service.Limit("buy:"+strconv.FormatInt(member.ID, 10), 20, 3600); err != nil {
		return "", err
	}
	order, err := service.CreateOrder(member.ID, licenseID, config.Charging)
	if err != nil {
		return "", err
	}
	amount, err := payment.FromMinor(order.AmountMinor, order.Currency)
	if err != nil {
		return "", err
	}
	back := strings.TrimRight(config.Origin, "/") + "/account/?action=order&id=" + order.ID + "&lang=" + lang
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	start, err := provider.StartCheckout(ctx, config.PaymentCredentials, payment.Checkout{
		LicenseID: order.LicenseID, Order: order.ID, Amount: amount, Currency: order.Currency, Period: payment.PeriodYearly,
		PayerEmail: member.Email, Language: lang, SuccessURL: back, CancelURL: back,
	})
	if err != nil {
		return "", paymentProblem(err)
	}
	if err := service.AttachCheckout(order.ID, provider.Code(), start.Reference); err != nil {
		return "", err
	}
	if !strings.HasPrefix(start.RedirectURL, "https://") {
		return "", ErrPaymentUnavailable
	}
	return start.RedirectURL, nil
}

// paymentProblem turns a provider's answer into what the member is told.
func paymentProblem(err error) error {
	switch {
	case errors.Is(err, payment.ErrNotConfigured):
		return ErrPaymentNotReady
	default:
		return ErrPaymentUnavailable
	}
}

// settle confirms an open order of this member with the provider (the return page): the
// return address's parameters are never read. A provider that cannot be asked leaves the
// order as it is; the page says so.
func (x *exchange) settle(service *Service, config Config, member *Member, orderID string) (Order, error) {
	order, err := service.GetOrder(orderID)
	if err != nil || order.Member != member.ID {
		return Order{}, ErrOrderNotFound
	}
	if (order.State != OrderCreated && order.State != OrderPending) || order.ProviderRef == "" {
		return order, nil
	}
	if err := service.Limit("settle:"+orderID, 30, 60); err != nil {
		return order, nil // asked often enough; the page shows the stored state
	}
	provider, ok := x.h.payments().WebhookProvider(order.Provider)
	confirmer, isConfirmer := provider.(payment.Confirmer)
	if !ok || !isConfirmer || config.PaymentProvider != order.Provider {
		return order, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	fact, err := confirmer.Confirm(ctx, config.PaymentCredentials, order.ProviderRef, order.Expected())
	if err != nil {
		return order, nil
	}
	applied, err := service.ApplyFact(order.ID, order.Provider, "member:return", fact)
	if err != nil {
		return order, err
	}
	x.receipt(service, config, order, applied)
	return applied, nil
}

// receipt mails the receipt when this request made the order paid (once: only the request
// that moved it).
func (x *exchange) receipt(service *Service, config Config, before, after Order) {
	if before.State == OrderPaid || after.State != OrderPaid {
		return
	}
	if err := service.sendReceipt(after, x.h.mail(config)); err != nil {
		x.h.log("the receipt mail could not be sent; the receipt stays on the member's page")
	}
}

// webhook is the provider's notification route: POST /account/?action=webhook&provider=CODE.
// Only providers with a webhook (never "manual"); the signature is verified on the raw
// body; an event id is recorded once, after its order was confirmed; 2xx once recorded,
// ignored or a duplicate; 5xx only when the provider could not be asked (it sends again).
func (x *exchange) webhook(service *Service, config Config, ip string, get phpVars) error {
	answer := func(status int, body string) error {
		x.w.Header().Set("Content-Type", "application/json")
		x.w.WriteHeader(status)
		io.WriteString(x.w, body)
		x.written = true
		return nil
	}
	if x.r.Method != http.MethodPost {
		x.w.Header().Set("Allow", "POST")
		return answer(http.StatusMethodNotAllowed, `{"error":"method_not_allowed"}`)
	}
	if err := service.Limit("webhook:"+ip, 600, 60); err != nil {
		return answer(http.StatusTooManyRequests, `{"error":"rate_limited"}`)
	}
	code := get.str("provider")
	provider, ok := x.h.payments().WebhookProvider(code)
	if !ok {
		return answer(http.StatusNotFound, `{"error":"unknown_provider"}`)
	}
	if !service.StoreReady() || config.PaymentProvider != code || PaymentState(x.h.payments(), config) != PaymentReady {
		x.h.log("webhook: the store or the provider's keys are not set up")
		return answer(http.StatusServiceUnavailable, `{"error":"service_unavailable"}`)
	}
	raw, err := io.ReadAll(io.LimitReader(x.r.Body, maxWebhookBody+1))
	if err != nil {
		return answer(http.StatusBadRequest, `{"error":"invalid_notification"}`)
	}
	if len(raw) > maxWebhookBody {
		return answer(http.StatusRequestEntityTooLarge, `{"error":"too_large"}`)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	event, err := provider.ParseNotification(ctx, config.PaymentCredentials, payment.Notification{Headers: x.r.Header, Body: raw})
	if err != nil {
		x.h.log("webhook: a notification that does not verify")
		return answer(http.StatusBadRequest, `{"error":"invalid_notification"}`)
	}
	if seen, err := service.EventSeen(code, event.EventID); err != nil {
		return err
	} else if seen {
		return answer(http.StatusOK, `{"received":true}`)
	}
	record := func() error {
		if _, err := service.RecordEvent(code, event.EventID, string(event.Kind), event.Reference); err != nil {
			return err
		}
		return answer(http.StatusOK, `{"received":true}`)
	}
	switch event.Kind {
	case payment.EventPayment, payment.EventRefund, payment.EventDispute:
	default: // ignored, and renewal methods (not built: they wait for the owner)
		return record()
	}
	order, err := service.findOrder(code, event)
	if errors.Is(err, ErrOrderNotFound) {
		x.h.log("webhook: a verified notification for no order of ours")
		return record()
	}
	if err != nil {
		return err
	}
	confirmer, ok := provider.(payment.Confirmer)
	if !ok {
		return record()
	}
	reference := event.Reference
	if event.Kind == payment.EventPayment && order.ProviderRef != "" && strings.HasPrefix(reference, "cs_") && reference != order.ProviderRef {
		reference = order.ProviderRef
	}
	fact, err := confirmer.Confirm(ctx, config.PaymentCredentials, reference, order.Expected())
	if errors.Is(err, payment.ErrProviderUnavailable) || errors.Is(err, payment.ErrProviderBusy) {
		return answer(http.StatusServiceUnavailable, `{"error":"provider_unavailable"}`)
	}
	if err != nil {
		x.h.log("webhook: the provider's object is in an unexpected state; the operator looks at it")
		return record()
	}
	applied, err := service.ApplyFact(order.ID, code, "provider:"+code, fact)
	if err != nil {
		return err
	}
	x.receipt(service, config, order, applied)
	return record()
}

// EventSeen reports whether a provider's notification id was recorded.
func (s *Service) EventSeen(provider, eventID string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM store_events WHERE provider=? AND event_id=?", provider, eventID).Scan(&n)
	return n == 1, err
}

// findOrder finds the order a notification hints at: by our order reference, by the
// checkout reference, or by a recorded payment.
func (s *Service) findOrder(provider string, event payment.Event) (Order, error) {
	if event.Order != "" {
		if o, err := s.GetOrder(event.Order); err == nil {
			return o, nil
		}
	}
	if event.Reference != "" {
		if o, err := s.OrderByProviderRef(provider, event.Reference); err == nil {
			return o, nil
		}
	}
	for _, id := range []string{event.PaymentIntent, event.Charge, event.Reference} {
		if id == "" {
			continue
		}
		if o, err := s.OrderByPayment(provider, id); err == nil {
			return o, nil
		}
	}
	return Order{}, ErrOrderNotFound
}

// ReconcileReport is what one reconciliation run did.
type ReconcileReport struct {
	Confirmed, Paid, Expired, Refunded, Unreachable int
	Mismatches                                      []Order
	// PaidOrders became paid in this run: their receipts are to be mailed.
	PaidOrders []Order
}

// Reconcile asks the provider about every open order older than ten minutes and every
// refund still being processed, applies what it confirms, ends orders past their day that
// are still unpaid, and lists mismatches for the operator (never fulfilled). It is the
// command for a timer: `membership reconcile`.
func (s *Service) Reconcile(ctx context.Context, registry *payment.Registry, config Config, actor string) (ReconcileReport, error) {
	var report ReconcileReport
	if !s.StoreReady() {
		return report, ErrStoreNotReady
	}
	orders, err := s.Orders(0)
	if err != nil {
		return report, err
	}
	provider, _ := registry.WebhookProvider(config.PaymentProvider)
	confirmer, canConfirm := provider.(payment.Confirmer)
	usable := canConfirm && PaymentState(registry, config) == PaymentReady
	now := s.Now()
	for _, o := range orders {
		if o.State == OrderMismatch {
			report.Mismatches = append(report.Mismatches, o)
			continue
		}
		open := (o.State == OrderCreated || o.State == OrderPending) && o.Created <= now-600
		refunding := false
		if o.State == OrderPaid {
			refunds, err := s.Refunds(o.ID)
			if err != nil {
				return report, err
			}
			for _, r := range refunds {
				refunding = refunding || r.State == "sent" || r.State == "requested"
			}
		}
		if !open && !refunding {
			continue
		}
		if o.ProviderRef == "" || !usable || o.Provider != config.PaymentProvider {
			if open && o.Expires <= now && o.ProviderRef == "" {
				if err := s.ExpireOrder(o.ID, actor); err != nil {
					return report, err
				}
				report.Expired++
			}
			continue
		}
		fact, err := confirmer.Confirm(ctx, config.PaymentCredentials, o.ProviderRef, o.Expected())
		if err != nil {
			report.Unreachable++
			continue
		}
		report.Confirmed++
		applied, err := s.ApplyFact(o.ID, o.Provider, actor, fact)
		if err != nil {
			return report, err
		}
		switch {
		case applied.State == OrderPaid && o.State != OrderPaid:
			report.Paid++
			report.PaidOrders = append(report.PaidOrders, applied)
		case applied.State == OrderRefunded:
			report.Refunded++
		case applied.State == OrderMismatch:
			report.Mismatches = append(report.Mismatches, applied)
		case (applied.State == OrderCreated || applied.State == OrderPending) && applied.Expires <= now && fact.Outcome == payment.OutcomeNotPaid:
			if err := s.ExpireOrder(o.ID, actor); err != nil {
				return report, err
			}
			report.Expired++
		}
	}
	return report, nil
}

// storePage gathers the store's facts for the signed-in member's pages.
func (x *exchange) storePage(service *Service, config Config, member *Member, licenses []License) (*storePage, error) {
	store := &storePage{charging: config.Charging, revoked: map[string]bool{}, quotes: map[string]Quote{}, paidAt: map[string]int64{}}
	store.ready = config.Charging == ChargingTest && PaymentState(x.h.payments(), config) == PaymentReady
	orders, err := service.Orders(member.ID)
	if err != nil {
		return nil, err
	}
	store.orders = orders
	for _, o := range orders {
		if o.State == OrderPaid || o.State == OrderRefunded {
			at, err := service.PaidAt(o.ID)
			if err != nil {
				return nil, err
			}
			store.paidAt[o.ID] = at
		}
	}
	for _, l := range licenses {
		revoked, _, err := service.LicenceRevocation(l.ID)
		if err != nil {
			return nil, err
		}
		store.revoked[l.ID] = revoked
		if store.ready && !revoked {
			if q, err := service.QuoteFor(member.ID, l.ID); err == nil {
				store.quotes[l.ID] = q
			}
		}
	}
	return store, nil
}

// buyPage prepares the page before payment, or answers home with the refusal in words.
func (x *exchange) buyPage(service *Service, config Config, member *Member, licenses []License, id string, store *storePage) (string, string) {
	if _, err := x.chargingProvider(service, config); err != nil {
		if problem, ok := err.(Problem); ok {
			return "home", string(problem)
		}
		return "home", string(ErrPaymentUnavailable)
	}
	for i := range licenses {
		if licenses[i].ID != id {
			continue
		}
		q, err := service.QuoteFor(member.ID, id)
		if err != nil {
			if problem, ok := err.(Problem); ok {
				return "home", string(problem)
			}
			return "home", string(ErrPaymentUnavailable)
		}
		store.buy, store.quote = &licenses[i], q
		return "buy", ""
	}
	return "home", string(ErrLicenseNotFound)
}

// OperatorRefund is the operator's exceptional act (there are no refunds by policy): with a
// reason, the attempt persisted first, the provider's Refund with confirm-first, its answer
// recorded, and -- when the provider accepted -- the order confirmed again so a completed
// refund is applied at once (otherwise reconciliation completes it).
func (s *Service) OperatorRefund(ctx context.Context, registry *payment.Registry, config Config, actor, orderID, reason string) (Refund, Order, error) {
	r, err := s.RequestRefund(actor, orderID, reason)
	if err != nil {
		return Refund{}, Order{}, err
	}
	order, err := s.GetOrder(orderID)
	if err != nil {
		return r, order, err
	}
	provider, ok := registry.WebhookProvider(order.Provider)
	if !ok || config.PaymentProvider != order.Provider || PaymentState(registry, config) != PaymentReady || order.ProviderRef == "" {
		return r, order, ErrPaymentNotReady
	}
	answer := provider.Refund(ctx, config.PaymentCredentials, payment.RefundRequest{Reference: order.ProviderRef, Expected: order.Expected(), Attempt: r.Attempt})
	if err := s.RefundAnswered(r.ID, actor, answer); err != nil {
		return r, order, err
	}
	if answer == nil || errors.Is(answer, payment.ErrAlreadyRefunded) {
		if confirmer, ok := provider.(payment.Confirmer); ok {
			if fact, err := confirmer.Confirm(ctx, config.PaymentCredentials, order.ProviderRef, order.Expected()); err == nil {
				if order, err = s.ApplyFact(order.ID, order.Provider, actor, fact); err != nil {
					return r, order, err
				}
			}
		}
	}
	refunds, err := s.Refunds(orderID)
	if err == nil {
		for _, x := range refunds {
			if x.ID == r.ID {
				r = x
			}
		}
	}
	return r, order, answer
}
