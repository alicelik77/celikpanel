package membership

// The store's doors (S2, step 2): "Buy a year" on the member's licence (an order and the
// provider's checkout), the order page the member returns to (settled by Confirm, never by
// the return address's parameters), the webhook route, and reconciliation. Nothing here
// trusts a notification's body: every state comes from Confirm against the order's stored
// expectation (the list "what the next package must respect" in PR #361).

import (
	"context"
	"database/sql"
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
	// A licence with an open payment page is not removed until the page is closed (F5).
	ErrRemovePaymentOpen       Problem = "remove_payment_open"
	ErrRemovePaymentCompleting Problem = "remove_payment_completing"
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

// Charged says whether this member buys their years (S2 review, 2 and 3): in test mode only an
// account the operator flagged. Live mode will make it every account; it is refused in this
// release, so nobody is charged then. While charging is off -- the beta -- nobody is.
func Charged(service *Service, config Config, member int64) bool {
	switch config.Charging {
	case ChargingTest:
		if !service.StoreReady() {
			return false
		}
		test, err := service.IsTestAccount(member)
		return err == nil && test
	case ChargingLive:
		// Every account, once live charging exists; refused in this release, so nobody yet.
		return false
	}
	return false
}

// chargingProvider is the provider for new orders, or the member-facing refusal.
func (x *exchange) chargingProvider(service *Service, config Config, member *Member) (payment.Provider, error) {
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
	if !Charged(service, config, member.ID) {
		return nil, ErrNotATestAccount
	}
	if PaymentState(x.h.payments(), config) != PaymentReady {
		x.h.log("charging is on but the payment provider's keys are not set or not usable")
		return nil, ErrPaymentNotReady
	}
	provider, _ := x.h.payments().WebhookProvider(config.PaymentProvider)
	return provider, nil
}

// buy is the "Continue to payment" form: an order (a second click returns the same one) and
// ONE checkout session for it (S2 review, 6): a later click -- another language, a changed
// address -- goes to the stored session while it is open; a session that ended closes its
// order and a new order is made. The session ends with the order (S2 review, 7).
func (x *exchange) buy(service *Service, config Config, member *Member, licenseID, lang string) (string, error) {
	provider, err := x.chargingProvider(service, config, member)
	if err != nil {
		return "", err
	}
	// Its own bucket (the reviews' list, 10): a member starts at most 20 checkouts an hour.
	if err := service.Limit("buy:"+strconv.FormatInt(member.ID, 10), 20, 3600); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	for attempt := 0; attempt < 2; attempt++ {
		order, err := service.CreateOrder(member.ID, licenseID, config.Charging)
		if err != nil {
			return "", err
		}
		if order.ProviderRef != "" {
			sessions, ok := provider.(payment.Sessions)
			if !ok {
				return "", ErrPaymentUnavailable
			}
			current, err := sessions.Session(ctx, config.PaymentCredentials, order.ProviderRef)
			if err != nil {
				return "", paymentProblem(err)
			}
			switch current.Status {
			case "open":
				if strings.HasPrefix(current.URL, "https://") {
					return current.URL, nil
				}
				return "", ErrPaymentUnavailable
			case "complete":
				return strings.TrimRight(config.Origin, "/") + "/account/?action=order&id=" + order.ID + "&lang=" + lang, nil
			}
			// Expired at the provider: close this order; the next pass makes a new one.
			if err := service.ExpireOrder(order.ID, memberActor(member.ID), true); err != nil {
				return "", err
			}
			continue
		}
		// Too little time left for the provider's shortest checkout (F7): a locked renewal waits for
		// the end; any other order closes and a new one is made.
		if order.Expires-service.Now() < MinimumOrderLifetime {
			if order.Locked {
				return "", ErrBuyAfterEnd
			}
			if err := service.ExpireOrder(order.ID, memberActor(member.ID), true); err != nil {
				return "", err
			}
			continue
		}
		amount, err := payment.FromMinor(order.AmountMinor, order.Currency)
		if err != nil {
			return "", err
		}
		back := strings.TrimRight(config.Origin, "/") + "/account/?action=order&id=" + order.ID + "&lang=" + lang
		start, err := provider.StartCheckout(ctx, config.PaymentCredentials, payment.Checkout{
			LicenseID: order.LicenseID, Order: order.ID, Amount: amount, Currency: order.Currency, Period: payment.PeriodYearly,
			PayerEmail: member.Email, Language: lang, SuccessURL: back, CancelURL: back, ExpiresAt: time.Unix(order.Expires, 0),
		})
		if err != nil {
			return "", paymentProblem(err)
		}
		if !strings.HasPrefix(start.RedirectURL, "https://") {
			return "", ErrPaymentUnavailable
		}
		if err := service.AttachCheckout(order.ID, provider.Code(), start.Reference); err != nil {
			return "", err
		}
		return start.RedirectURL, nil
	}
	return "", ErrPaymentUnavailable
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
		x.h.log("the return page could not apply a confirmed fact; the order stays as it was")
		return order, nil // the page shows the stored state (S2 review, 11)
	}
	x.receipt(service, config, applied)
	return applied, nil
}

// receipt mails the receipt when this request's ApplyFact claimed it (exactly once: the claim
// is made inside the transaction that made the order paid).
func (x *exchange) receipt(service *Service, config Config, after Order) {
	if !after.ReceiptMine {
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
		// The length is set, so the provider has the whole answer at the flush below, before
		// any mail is tried -- under serve and under CGI alike (S2 review 2, N3).
		x.w.Header().Set("Content-Length", strconv.Itoa(len(body)))
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
		// Nothing is dropped silently (S2 third look, F4): the order is listed for the operator.
		if aerr := service.atomic(func(tx *sql.Tx) error {
			return service.openAttention(context.Background(), tx, order.ID, "provider:"+code, AttentionUnconfirmed, "a notification whose payment could not be confirmed ("+err.Error()+")")
		}); aerr != nil {
			return aerr
		}
		return record()
	}
	applied, err := service.ApplyFact(order.ID, code, "provider:"+code, fact)
	if err != nil {
		return err
	}
	// Answer first (the provider does not wait for our mail), then mail (S2 review, 11).
	if err := record(); err != nil {
		return err
	}
	if flusher, ok := x.w.(http.Flusher); ok {
		flusher.Flush()
	}
	x.receipt(service, config, applied)
	return nil
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
	// Left is how many orders that were due are left for the next run (the cap).
	Left int
	// Failed lists orders whose confirmation or application failed for another reason than
	// the provider being unreachable (each also opens an attention entry).
	Failed []string
	// ReceiptsOwed are paid orders whose receipt mail is owed (claimed, not yet sent): the
	// command mails them and marks them sent.
	ReceiptsOwed []Order
	// Attention is what the operator must look at after the run (orders -attention).
	Attention []AttentionEntry
}

// ReconcileCap is how many orders one run asks the provider about; the rest wait for the
// next run (and the output says so).
var ReconcileCap = 200

// Reconcile is the command for a timer (`membership reconcile`). Oldest due first, at most
// ReconcileCap provider questions, one failing order never stopping the run. It asks about:
//   - open orders older than ten minutes (a lost webhook);
//   - orders that closed unpaid, once more an hour after their checkout's own end;
//   - paid orders whose payment is younger than RecheckWindow, at most once a day each;
//   - paid orders with a dispute that is not closed, whatever their age, at most once a day;
//   - paid orders with a refund in progress.
//
// It applies what it confirms, ends an unpaid order past its time (its checkout first), lists
// the receipts still owed, and answers what needs the operator.
func (s *Service) Reconcile(ctx context.Context, registry *payment.Registry, config Config, actor string) (ReconcileReport, error) {
	var report ReconcileReport
	if !s.StoreReady() {
		return report, ErrStoreNotReady
	}
	due, err := s.dueOrders()
	if err != nil {
		return report, err
	}
	provider, _ := registry.WebhookProvider(config.PaymentProvider)
	confirmer, canConfirm := provider.(payment.Confirmer)
	sessions, canExpire := provider.(payment.Sessions)
	usable := canConfirm && PaymentState(registry, config) == PaymentReady
	now := s.Now()
	asked := 0
	fail := func(o Order, what string) {
		report.Failed = append(report.Failed, o.ID+": "+what)
		// Marked checked, so it waits its turn (a day) and cannot take the cap every run; one
		// attention entry, not a new log line each run (S2 third look, F8).
		_ = s.MarkChecked(o.ID)
		_ = s.atomic(func(tx *sql.Tx) error {
			return s.openAttention(ctx, tx, o.ID, actor, AttentionUnconfirmed, "could not be settled by reconciliation")
		})
	}
	for _, o := range due {
		if o.ProviderRef == "" || !usable || o.Provider != config.PaymentProvider {
			if (o.State == OrderCreated || o.State == OrderPending) && o.Expires <= now && o.ProviderRef == "" {
				if err := s.ExpireOrder(o.ID, actor, false); err != nil {
					fail(o, "could not close: "+err.Error())
					continue
				}
				report.Expired++
			}
			continue
		}
		if asked >= ReconcileCap {
			report.Left++
			continue
		}
		asked++
		fact, err := confirmer.Confirm(ctx, config.PaymentCredentials, o.ProviderRef, o.Expected())
		if errors.Is(err, payment.ErrProviderUnavailable) || errors.Is(err, payment.ErrProviderBusy) || ctx.Err() != nil {
			report.Unreachable++
			continue
		}
		if err != nil {
			fail(o, "the provider's answer could not be confirmed ("+err.Error()+")")
			continue
		}
		report.Confirmed++
		if err := s.MarkChecked(o.ID); err != nil {
			fail(o, "could not record the check ("+err.Error()+")")
			continue
		}
		applied, err := s.ApplyFact(o.ID, o.Provider, actor, fact)
		if err != nil {
			fail(o, "could not apply the confirmed answer ("+err.Error()+")")
			continue
		}
		_ = s.atomic(func(tx *sql.Tx) error {
			return s.closeAttention(ctx, tx, o.ID, actor, AttentionUnconfirmed, "confirmed again")
		})
		switch {
		case applied.State == OrderPaid && o.State != OrderPaid:
			report.Paid++
		case applied.State == OrderRefunded && o.State != OrderRefunded:
			report.Refunded++
		case (applied.State == OrderCreated || applied.State == OrderPending) && applied.Expires <= now && fact.Outcome == payment.OutcomeNotPaid:
			// Its checkout is ended first, so nothing can be paid into a closed order.
			if canExpire {
				if err := sessions.ExpireSession(ctx, config.PaymentCredentials, o.ProviderRef); err != nil {
					report.Unreachable++
					continue
				}
			}
			if err := s.ExpireOrder(o.ID, actor, false); err != nil {
				fail(o, "could not close: "+err.Error())
				continue
			}
			report.Expired++
		}
	}
	if report.ReceiptsOwed, err = s.ReceiptsOwed(600); err != nil {
		return report, err
	}
	report.Attention, err = s.AttentionOrders()
	return report, err
}

// dueOrders are the orders reconciliation should ask about now, oldest first.
func (s *Service) dueOrders() ([]Order, error) {
	now := s.Now()
	rows, err := s.db.QueryContext(context.Background(), "SELECT "+orderColumns+" FROM store_orders o WHERE "+
		"(o.state IN ('created','pending') AND o.created<=? AND NOT (COALESCE(o.checked,0)>? AND EXISTS (SELECT 1 FROM store_attention a WHERE a.order_id=o.id AND a.kind='unconfirmed' AND a.state='open'))) "+
		"OR (o.state IN ('expired','failed') AND o.provider_ref IS NOT NULL AND ?>=o.expires+3600 AND COALESCE(o.checked,0)<o.expires+3600) "+
		"OR (o.state='paid' AND COALESCE(o.checked,0)<=? AND ("+
		"EXISTS (SELECT 1 FROM store_payments p WHERE p.order_id=o.id AND (p.confirmed>? OR (p.disputed=1 AND p.dispute_status NOT IN ('won','lost','warning_closed','prevented'))))"+
		" OR EXISTS (SELECT 1 FROM store_refunds r WHERE r.order_id=o.id AND r.state IN ('requested','sent')))) "+
		"OR (o.state='paid' AND EXISTS (SELECT 1 FROM store_refunds r WHERE r.order_id=o.id AND r.state IN ('requested','sent')) AND COALESCE(o.checked,0)<=?) "+
		"OR (o.state='mismatch' AND COALESCE(o.checked,0)<=? AND EXISTS (SELECT 1 FROM store_payments p WHERE p.order_id=o.id AND p.confirmed>?)) "+
		"ORDER BY COALESCE(o.checked,0),o.created", now-600, now-86400, now, now-86400, now-RecheckWindow, now-3600, now-86400, now-RecheckWindow)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Order
	for rows.Next() {
		o, err := scanOrder(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// storePage gathers the store's facts for the signed-in member's pages.
func (x *exchange) storePage(service *Service, config Config, member *Member, licenses []License) (*storePage, error) {
	store := &storePage{charging: config.Charging, revoked: map[string]bool{}, quotes: map[string]Quote{}, paidAt: map[string]int64{}, awaiting: map[string]bool{}}
	store.charged = Charged(service, config, member.ID) // everyone else sees the beta pages (S2 review, 3)
	store.ready = store.charged && config.Charging == ChargingTest && PaymentState(x.h.payments(), config) == PaymentReady
	trialID, trialEnds, trialUsed, err := service.Trial(member.ID, config.Charging)
	if err != nil {
		return nil, err
	}
	store.trialID, store.trialEnds, store.trialUsed = trialID, trialEnds, trialUsed
	store.trialRefused = trialID != "" && service.TrialRefused(trialID)
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
		if store.charged && !revoked {
			if store.awaiting[l.ID], err = service.AwaitingPayment(l.ID); err != nil {
				return nil, err
			}
		}
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
	if _, err := x.chargingProvider(service, config, member); err != nil {
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
	order, err := s.GetOrder(orderID)
	if err != nil {
		return Refund{}, order, err
	}
	// Nothing is recorded as requested when nothing can be sent (S2 review, 11).
	provider, ok := registry.WebhookProvider(order.Provider)
	if strings.TrimSpace(reason) != "" && (!ok || config.PaymentProvider != order.Provider || PaymentState(registry, config) != PaymentReady || order.ProviderRef == "") {
		return Refund{}, order, ErrPaymentNotReady
	}
	r, err := s.RequestRefund(actor, orderID, reason)
	if err != nil {
		return Refund{}, order, err
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

// closeCheckout ends the provider's open session of a licence's unpaid order before the
// licence is removed (S2 third look, F5): a payment can then never arrive for a removed licence
// through a session left open. If the session cannot be ended -- the provider cannot be asked,
// or a payment is already being completed -- the removal is refused with words that say why.
func (x *exchange) closeCheckout(service *Service, config Config, member int64, licenseID string) error {
	if !service.StoreReady() {
		return nil
	}
	orders, err := service.Orders(member)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	for _, o := range orders {
		if o.LicenseID != licenseID || (o.State != OrderCreated && o.State != OrderPending) || o.ProviderRef == "" {
			continue
		}
		provider, ok := x.h.payments().WebhookProvider(o.Provider)
		sessions, canEnd := provider.(payment.Sessions)
		if !ok || !canEnd || config.PaymentProvider != o.Provider || PaymentState(x.h.payments(), config) != PaymentReady {
			return ErrRemovePaymentOpen
		}
		current, err := sessions.Session(ctx, config.PaymentCredentials, o.ProviderRef)
		if err != nil {
			return ErrRemovePaymentOpen
		}
		switch current.Status {
		case "complete":
			return ErrRemovePaymentCompleting
		case "open":
			if err := sessions.ExpireSession(ctx, config.PaymentCredentials, o.ProviderRef); err != nil {
				return ErrRemovePaymentOpen
			}
		}
	}
	return nil
}
