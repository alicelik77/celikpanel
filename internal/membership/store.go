package membership

// The store (second work package of the member accounts): prices, orders, payments,
// provider events, refunds, test accounts and an append-only audit log, in the same
// members.sqlite. Its tables and the two licence columns it needs ("revoked",
// "revoked_reason") are created only by MigrateStore -- the explicit `membership
// migrate-store` command -- never at request time, and only additively: the PHP service
// keeps working on the same file (it names its columns and never reads these tables).
//
// Rules the store keeps (D-047, the design doc, the reviews' list in PR #361):
//   - an order moves only forward (Go checks the predecessor in every UPDATE, and a trigger
//     refuses any other change of state);
//   - a fact is applied only from Confirm, and a year is granted in the same transaction
//     that records the payment, once per (provider, payment id);
//   - a revocation raises the licence's generation in the same statement and never deletes
//     a row ("Revocations raise the generation");
//   - the owner's answers of 23 November 2026 (D-047 amendment): there is no grace period and
//     no refund. A year bought before the licence's end continues from the old end at the
//     locked price (the price paid last, never higher); after the end it starts at the payment
//     at the current list price. A refund or a lost dispute can still happen AT the provider
//     (the operator's exceptional act, or a chargeback): it arrives as a confirmed fact and is
//     applied -- a first year is revoked, a renewal's end goes back -- and is in the log.

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"

	"github.com/alicelik/celikpanel/internal/payment"
)

// StoreVersion is the store schema this code reads.
const StoreVersion = 1

// The store's refusals that a member or the operator can see.
const (
	ErrChargingOff          Problem = "charging_off"
	ErrChargingLiveDisabled Problem = "charging_live_not_enabled"
	ErrNotATestAccount      Problem = "not_a_test_account"
	ErrOrderNotFound        Problem = "order_not_found"
	ErrRefundNotPossible    Problem = "refund_not_possible"
	// ErrTestTimeRemains: the account still has test-paid time running; the test-account flag
	// stays until it ends (the operator's refusal).
	ErrTestTimeRemains Problem = "test_time_remains"
	// ErrBuyAfterEnd: too little time before the licence's end for the provider's checkout; the
	// year is bought as a new one after the end.
	ErrBuyAfterEnd Problem = "buy_after_end"
	// ErrRenewalIsBought: an account that buys its years has no free renewal.
	ErrRenewalIsBought Problem = "renewal_is_bought"
)

// ErrStoreNotReady: the store's tables are missing or of another version. The service then
// behaves as with charging off; only `membership migrate-store` creates them.
var ErrStoreNotReady = errors.New("membership: the store is not set up (run membership migrate-store)")

// ChargingMode is the private configuration's "charging": off (the default), test or live.
type ChargingMode string

const (
	ChargingOff  ChargingMode = "off"
	ChargingTest ChargingMode = "test"
	ChargingLive ChargingMode = "live"
)

// Order kinds: the first paid year of a licence, and every later one.
const (
	KindFirstYear = "first_year"
	KindRenewal   = "renewal"
)

// Order states.
const (
	OrderCreated  = "created"
	OrderPending  = "pending"
	OrderPaid     = "paid"
	OrderMismatch = "mismatch"
	OrderExpired  = "expired"
	OrderFailed   = "failed"
	OrderRefunded = "refunded"
)

// orderNext is the forward-only state table: for each state, the states it may become.
// mismatch and refunded are final; an expired or failed order still becomes paid when the
// provider shows the money arrived (the payer's money is never ignored).
var orderNext = map[string][]string{
	OrderCreated:  {OrderPending, OrderPaid, OrderMismatch, OrderExpired, OrderFailed, OrderRefunded},
	OrderPending:  {OrderPaid, OrderMismatch, OrderExpired, OrderFailed, OrderRefunded},
	OrderExpired:  {OrderPaid, OrderMismatch, OrderRefunded},
	OrderFailed:   {OrderPaid, OrderMismatch, OrderRefunded},
	OrderPaid:     {OrderRefunded},
	OrderMismatch: {},
	OrderRefunded: {},
}

// OrderLifetime: an order nobody paid ends after 23 hours, and its checkout session with it
// (S2 review, 7; Stripe allows 30 minutes to 24 hours). A renewal at the locked price ends
// earlier: at the licence's end date.
const OrderLifetime = 23 * 3600

// MinimumOrderLifetime: the provider's shortest checkout session. A locked renewal with less
// time than this before the licence's end is not offered; the year is bought after the end.
const MinimumOrderLifetime = 30 * 60

// RecheckWindow: paid orders whose payment is younger than this are confirmed again by
// reconciliation, at most once a day each, so a lost webhook cannot hide a refund or a lost
// dispute. 120 days is the longest time card networks give a payer to dispute a payment (to
// verify against the provider's documentation, V16).
const RecheckWindow = 120 * 86400

func storeSchema() []string {
	var pairs []string
	for from, to := range orderNext {
		for _, t := range to {
			pairs = append(pairs, "(OLD.state='"+from+"' AND NEW.state='"+t+"')")
		}
	}
	return []string{
		"CREATE TABLE IF NOT EXISTS store_meta (\n key TEXT PRIMARY KEY, value TEXT NOT NULL\n)",
		"CREATE TABLE IF NOT EXISTS store_prices (\n id INTEGER PRIMARY KEY, currency TEXT NOT NULL CHECK(length(currency)=3),\n amount_minor INTEGER NOT NULL CHECK(amount_minor>0), valid_from INTEGER NOT NULL,\n changed_by TEXT NOT NULL, reason TEXT NOT NULL, created INTEGER NOT NULL\n)",
		"CREATE TRIGGER IF NOT EXISTS store_prices_no_update BEFORE UPDATE ON store_prices\n BEGIN SELECT RAISE(ABORT, 'store_prices is append-only'); END",
		"CREATE TRIGGER IF NOT EXISTS store_prices_no_delete BEFORE DELETE ON store_prices\n BEGIN SELECT RAISE(ABORT, 'store_prices is append-only'); END",
		"CREATE TABLE IF NOT EXISTS store_trials (\n member_id INTEGER PRIMARY KEY REFERENCES members(id), license_id TEXT NOT NULL REFERENCES licenses(id),\n started INTEGER NOT NULL, ends INTEGER NOT NULL\n)",
		"CREATE TABLE IF NOT EXISTS store_test_accounts (\n member_id INTEGER PRIMARY KEY REFERENCES members(id), flagged_by TEXT NOT NULL,\n reason TEXT NOT NULL, created INTEGER NOT NULL\n)",
		"CREATE TABLE IF NOT EXISTS store_orders (\n id TEXT PRIMARY KEY, member_id INTEGER NOT NULL REFERENCES members(id),\n license_id TEXT NOT NULL REFERENCES licenses(id), kind TEXT NOT NULL CHECK(kind IN ('first_year','renewal')),\n amount_minor INTEGER NOT NULL, currency TEXT NOT NULL, live INTEGER NOT NULL CHECK(live IN (0,1)),\n state TEXT NOT NULL CHECK(state IN ('created','pending','paid','mismatch','expired','failed','refunded')),\n provider TEXT, provider_ref TEXT, session_url TEXT, locked INTEGER NOT NULL DEFAULT 0,\n granted_from INTEGER, granted_to INTEGER, previous_expires INTEGER, receipt_sent INTEGER, attention TEXT, checked INTEGER,\n created INTEGER NOT NULL, expires INTEGER NOT NULL, updated INTEGER NOT NULL\n)",
		"CREATE INDEX IF NOT EXISTS store_orders_license ON store_orders(license_id, state)",
		"CREATE INDEX IF NOT EXISTS store_orders_ref ON store_orders(provider, provider_ref)",
		"CREATE TRIGGER IF NOT EXISTS store_orders_forward BEFORE UPDATE OF state ON store_orders\n WHEN OLD.state<>NEW.state AND NOT (" + strings.Join(sortedStrings(pairs), " OR ") + ")\n BEGIN SELECT RAISE(ABORT, 'store_orders: a state only moves forward'); END",
		"CREATE TABLE IF NOT EXISTS store_events (\n provider TEXT NOT NULL, event_id TEXT NOT NULL, kind TEXT NOT NULL, reference TEXT NOT NULL,\n received INTEGER NOT NULL, PRIMARY KEY(provider, event_id)\n)",
		"CREATE TABLE IF NOT EXISTS store_payments (\n provider TEXT NOT NULL, payment_id TEXT NOT NULL, order_id TEXT NOT NULL REFERENCES store_orders(id),\n outcome TEXT NOT NULL, paid_minor INTEGER NOT NULL, refunded_minor INTEGER NOT NULL, currency TEXT NOT NULL,\n live INTEGER NOT NULL, disputed INTEGER NOT NULL, confirmed INTEGER NOT NULL, updated INTEGER NOT NULL,\n PRIMARY KEY(provider, payment_id)\n)",
		"CREATE TABLE IF NOT EXISTS store_refunds (\n id INTEGER PRIMARY KEY, order_id TEXT NOT NULL REFERENCES store_orders(id), attempt INTEGER NOT NULL,\n state TEXT NOT NULL CHECK(state IN ('requested','sent','done','refused','final')), outcome TEXT NOT NULL DEFAULT '',\n requested_by TEXT NOT NULL, reason TEXT NOT NULL, created INTEGER NOT NULL, updated INTEGER NOT NULL\n)",
		"CREATE UNIQUE INDEX IF NOT EXISTS store_refunds_open ON store_refunds(order_id) WHERE state IN ('requested','sent')",
		"CREATE TABLE IF NOT EXISTS store_audit (\n id INTEGER PRIMARY KEY, at INTEGER NOT NULL, actor TEXT NOT NULL, action TEXT NOT NULL,\n target TEXT NOT NULL, before TEXT NOT NULL, after TEXT NOT NULL, reason TEXT NOT NULL\n)",
		"CREATE TRIGGER IF NOT EXISTS store_audit_no_update BEFORE UPDATE ON store_audit\n BEGIN SELECT RAISE(ABORT, 'store_audit is append-only'); END",
		"CREATE TRIGGER IF NOT EXISTS store_audit_no_delete BEFORE DELETE ON store_audit\n BEGIN SELECT RAISE(ABORT, 'store_audit is append-only'); END",
	}
}

func sortedStrings(in []string) []string {
	out := append([]string(nil), in...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// MigrateStore creates the store's tables and the licence columns it needs, and the
// D-047 list price (99 USD a year) when no price exists. Additive and repeatable; it never
// runs at request time. actor names who ran it (for the audit log).
func (s *Service) MigrateStore(actor string) error {
	ctx := context.Background()
	err := s.atomic(func(tx *sql.Tx) error {
		for _, statement := range storeSchema() {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return err
			}
		}
		columns, err := tableColumns(ctx, tx, "licenses")
		if err != nil {
			return err
		}
		for _, add := range []struct{ name, sql string }{
			{"revoked", "ALTER TABLE licenses ADD COLUMN revoked INTEGER"},
			{"revoked_reason", "ALTER TABLE licenses ADD COLUMN revoked_reason TEXT"},
		} {
			if !columns[add.name] {
				if _, err := tx.ExecContext(ctx, add.sql); err != nil {
					return err
				}
			}
		}
		var version sql.NullString
		if err := tx.QueryRowContext(ctx, "SELECT value FROM store_meta WHERE key='version'").Scan(&version); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if version.Valid && version.String != strconv.Itoa(StoreVersion) {
			return errors.New("membership: the store has version " + version.String + "; this program knows " + strconv.Itoa(StoreVersion))
		}
		if !version.Valid {
			if _, err := tx.ExecContext(ctx, "INSERT INTO store_meta(key,value) VALUES('version',?)", strconv.Itoa(StoreVersion)); err != nil {
				return err
			}
			if err := s.audit(ctx, tx, actor, "store_created", "store", "", "version "+strconv.Itoa(StoreVersion), "membership migrate-store"); err != nil {
				return err
			}
		}
		var prices int
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM store_prices").Scan(&prices); err != nil {
			return err
		}
		if prices == 0 {
			if _, err := tx.ExecContext(ctx, "INSERT INTO store_prices(currency,amount_minor,valid_from,changed_by,reason,created) VALUES('USD',9900,0,?,?,?)",
				actor, "D-047: list price 99 USD per server per year", s.Now()); err != nil {
				return err
			}
			if err := s.audit(ctx, tx, actor, "price_set", "USD", "", "9900", "D-047: list price 99 USD per server per year"); err != nil {
				return err
			}
		}
		return nil
	})
	if err == nil {
		s.hasRevoked = true
	}
	return err
}

// StoreReady reports whether the store's tables exist in this program's version. Request
// handling never creates them: without them, charging is off.
func (s *Service) StoreReady() bool {
	var version string
	err := s.db.QueryRowContext(context.Background(), "SELECT value FROM store_meta WHERE key='version'").Scan(&version)
	return err == nil && version == strconv.Itoa(StoreVersion)
}

// audit appends one line to the append-only log.
func (s *Service) audit(ctx context.Context, q querier, actor, action, target, before, after, reason string) error {
	_, err := q.ExecContext(ctx, "INSERT INTO store_audit(at,actor,action,target,before,after,reason) VALUES(?,?,?,?,?,?,?)",
		s.Now(), actor, action, target, before, after, reason)
	return err
}

// AuditLine is one line of the log.
type AuditLine struct {
	At                                        int64
	Actor, Action, Target, Before, After, Why string
}

// Audit lists the log for a target ("" for all), oldest first.
func (s *Service) Audit(target string) ([]AuditLine, error) {
	query, args := "SELECT at,actor,action,target,before,after,reason FROM store_audit ORDER BY id", []any{}
	if target != "" {
		query, args = "SELECT at,actor,action,target,before,after,reason FROM store_audit WHERE target=? ORDER BY id", []any{target}
	}
	rows, err := s.db.QueryContext(context.Background(), query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AuditLine
	for rows.Next() {
		var l AuditLine
		if err := rows.Scan(&l.At, &l.Actor, &l.Action, &l.Target, &l.Before, &l.After, &l.Why); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Prices and test accounts.

// Price is one row of the price history.
type Price struct {
	Currency    string
	AmountMinor int64
	ValidFrom   int64
	ChangedBy   string
	Reason      string
}

// CurrentPrice is the price in force now for a currency: the latest valid_from not in the
// future.
func (s *Service) CurrentPrice(currency string) (Price, error) {
	return s.currentPrice(context.Background(), s.db, currency)
}

func (s *Service) currentPrice(ctx context.Context, q querier, currency string) (Price, error) {
	p := Price{Currency: currency}
	err := q.QueryRowContext(ctx, "SELECT amount_minor,valid_from,changed_by,reason FROM store_prices WHERE currency=? AND valid_from<=? ORDER BY valid_from DESC,id DESC LIMIT 1",
		currency, s.Now()).Scan(&p.AmountMinor, &p.ValidFrom, &p.ChangedBy, &p.Reason)
	if errors.Is(err, sql.ErrNoRows) {
		return Price{}, ErrStoreNotReady
	}
	return p, err
}

// SetPrice adds a price row (history is kept, append-only); an operator act with a reason. A
// price never starts in the past (S2 review, 11): the history says when each was in force.
func (s *Service) SetPrice(actor, reason, currency string, amountMinor, validFrom int64) error {
	if strings.TrimSpace(reason) == "" || len(currency) != 3 || strings.ToUpper(currency) != currency || amountMinor <= 0 || validFrom < s.Now() {
		return ErrInvalidRequest
	}
	ctx := context.Background()
	return s.atomic(func(tx *sql.Tx) error {
		before := ""
		if p, err := s.currentPrice(ctx, tx, currency); err == nil {
			before = strconv.FormatInt(p.AmountMinor, 10)
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO store_prices(currency,amount_minor,valid_from,changed_by,reason,created) VALUES(?,?,?,?,?,?)",
			currency, amountMinor, validFrom, actor, reason, s.Now()); err != nil {
			return err
		}
		return s.audit(ctx, tx, actor, "price_set", currency, before, strconv.FormatInt(amountMinor, 10)+" from "+strconv.FormatInt(validFrom, 10), reason)
	})
}

// SetTestAccount flags (or unflags) a member as a test account: only such accounts can
// buy in test mode, so test money never extends a real customer's licence.
func (s *Service) SetTestAccount(actor, reason string, member int64, on bool) error {
	if strings.TrimSpace(reason) == "" {
		return ErrInvalidRequest
	}
	ctx := context.Background()
	return s.atomic(func(tx *sql.Tx) error {
		var exists int
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM members WHERE id=?", member).Scan(&exists); err != nil {
			return err
		}
		if exists == 0 {
			return ErrAuthenticationRequired
		}
		was, err := isTestAccount(ctx, tx, member)
		if err != nil {
			return err
		}
		if was == on {
			return nil
		}
		if !on {
			// Test-paid time that has not ended stays on the licences; the flag is not taken away
			// while it lasts (S2 review, 3), so it is never mistaken for paid time.
			var running int
			if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM store_orders o JOIN licenses l ON l.id=o.license_id WHERE o.member_id=? AND o.live=0 AND o.state='paid' AND o.granted_to>? AND l.expires>?",
				member, s.Now(), s.Now()).Scan(&running); err != nil {
				return err
			}
			if running > 0 {
				return ErrTestTimeRemains
			}
		}
		if on {
			_, err = tx.ExecContext(ctx, "INSERT INTO store_test_accounts(member_id,flagged_by,reason,created) VALUES(?,?,?,?)", member, actor, reason, s.Now())
		} else {
			_, err = tx.ExecContext(ctx, "DELETE FROM store_test_accounts WHERE member_id=?", member)
		}
		if err != nil {
			return err
		}
		return s.audit(ctx, tx, actor, "test_account", "member:"+strconv.FormatInt(member, 10), strconv.FormatBool(was), strconv.FormatBool(on), reason)
	})
}

func isTestAccount(ctx context.Context, q querier, member int64) (bool, error) {
	var n int
	err := q.QueryRowContext(ctx, "SELECT COUNT(*) FROM store_test_accounts WHERE member_id=?", member).Scan(&n)
	return n == 1, err
}

// IsTestAccount reports the flag.
func (s *Service) IsTestAccount(member int64) (bool, error) {
	return isTestAccount(context.Background(), s.db, member)
}

// ---------------------------------------------------------------------------
// Orders.

// Order is one purchase of one year of one licence.
type Order struct {
	ID              string
	Member          int64
	LicenseID       string
	Kind            string
	AmountMinor     int64
	Currency        string
	Live            bool
	State           string
	Provider        string
	ProviderRef     string
	GrantedFrom     int64 // 0 until paid
	GrantedTo       int64
	PreviousExpires int64
	Created         int64
	Expires         int64
	Updated         int64
	SessionURL      string
	Locked          bool   // a renewal at the locked price (valid only before the licence's end)
	Attention       string // why the operator must look at it (empty: nothing)
	// ReceiptMine is set (not stored) on the order ApplyFact returns when this very call made it
	// paid and claimed its receipt: only that caller mails it.
	ReceiptMine bool
}

const orderColumns = "id,member_id,license_id,kind,amount_minor,currency,live,state,COALESCE(provider,''),COALESCE(provider_ref,''),COALESCE(granted_from,0),COALESCE(granted_to,0),COALESCE(previous_expires,0),created,expires,updated,COALESCE(session_url,''),locked,COALESCE(attention,'')"

func scanOrder(scan func(...any) error) (Order, error) {
	var o Order
	var live, lockedInt int
	err := scan(&o.ID, &o.Member, &o.LicenseID, &o.Kind, &o.AmountMinor, &o.Currency, &live, &o.State, &o.Provider, &o.ProviderRef,
		&o.GrantedFrom, &o.GrantedTo, &o.PreviousExpires, &o.Created, &o.Expires, &o.Updated, &o.SessionURL, &lockedInt, &o.Attention)
	o.Locked = lockedInt == 1
	o.Live = live == 1
	return o, err
}

func (s *Service) orderTx(ctx context.Context, q querier, id string) (Order, error) {
	o, err := scanOrder(q.QueryRowContext(ctx, "SELECT "+orderColumns+" FROM store_orders WHERE id=?", id).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return Order{}, ErrOrderNotFound
	}
	return o, err
}

// GetOrder reads one order.
func (s *Service) GetOrder(id string) (Order, error) {
	if !s.StoreReady() {
		return Order{}, ErrStoreNotReady
	}
	return s.orderTx(context.Background(), s.db, id)
}

// Orders lists a member's orders (member > 0) or every order, newest first.
func (s *Service) Orders(member int64) ([]Order, error) {
	if !s.StoreReady() {
		return nil, ErrStoreNotReady
	}
	query, args := "SELECT "+orderColumns+" FROM store_orders ORDER BY created DESC,id", []any{}
	if member > 0 {
		query, args = "SELECT "+orderColumns+" FROM store_orders WHERE member_id=? ORDER BY created DESC,id", []any{member}
	}
	rows, err := s.db.QueryContext(context.Background(), query, args...)
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

// OrderByProviderRef finds an order by the provider's checkout reference.
func (s *Service) OrderByProviderRef(provider, ref string) (Order, error) {
	if !s.StoreReady() {
		return Order{}, ErrStoreNotReady
	}
	o, err := scanOrder(s.db.QueryRowContext(context.Background(), "SELECT "+orderColumns+" FROM store_orders WHERE provider=? AND provider_ref=?", provider, ref).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return Order{}, ErrOrderNotFound
	}
	return o, err
}

// OrderByPayment finds the order a recorded payment belongs to.
func (s *Service) OrderByPayment(provider, paymentID string) (Order, error) {
	if !s.StoreReady() {
		return Order{}, ErrStoreNotReady
	}
	var id string
	err := s.db.QueryRowContext(context.Background(), "SELECT order_id FROM store_payments WHERE provider=? AND payment_id=?", provider, paymentID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return Order{}, ErrOrderNotFound
	}
	if err != nil {
		return Order{}, err
	}
	return s.GetOrder(id)
}

// PaidAt is when the store first confirmed a payment for this order (0: none).
func (s *Service) PaidAt(orderID string) (int64, error) {
	var at sql.NullInt64
	err := s.db.QueryRowContext(context.Background(), "SELECT MIN(confirmed) FROM store_payments WHERE order_id=?", orderID).Scan(&at)
	return at.Int64, err
}

// Expected is what the provider must confirm for this order (Confirm's input).
func (o Order) Expected() payment.Expected {
	return payment.Expected{Kind: payment.OrderOneTime, Order: o.ID, LicenseID: o.LicenseID, AmountMinor: o.AmountMinor, Currency: o.Currency, Live: o.Live}
}

// CreateOrder starts the purchase of a year of a licence: its first paid year, or a renewal
// when it already had one. A second click while an order for the licence is still open
// returns that order. Charging off refuses; live is not enabled in this release; test mode
// only for accounts the operator flagged as test accounts.
//
// The price (the owner, 23 November 2026, and D-047 point 2): a renewal ordered before the
// licence's end keeps the price its last paid year cost (never higher than the list price
// in force); after the end it is a new purchase at the list price. No grace period exists.
func (s *Service) CreateOrder(member int64, licenseID string, mode ChargingMode) (Order, error) {
	switch mode {
	case ChargingTest:
	case ChargingLive:
		return Order{}, ErrChargingLiveDisabled
	default:
		return Order{}, ErrChargingOff
	}
	if !s.StoreReady() {
		return Order{}, ErrStoreNotReady
	}
	if !hex32.MatchString(licenseID) {
		return Order{}, ErrLicenseNotFound
	}
	ctx := context.Background()
	var out Order
	err := s.atomic(func(tx *sql.Tx) error {
		test, err := isTestAccount(ctx, tx, member)
		if err != nil {
			return err
		}
		if !test {
			return ErrNotATestAccount
		}
		l, err := scanRow(tx.QueryRowContext(ctx, "SELECT "+rowColumns+" FROM licenses WHERE id=? AND member_id=? AND removed IS NULL", licenseID, member))
		if err != nil {
			return err
		}
		if l == nil {
			return ErrLicenseNotFound
		}
		if err := s.loadRevoked(ctx, tx, l); err != nil {
			return err
		}
		if l.Revoked {
			return ErrLicenseRevoked
		}
		now := s.Now()
		open, err := scanOrder(tx.QueryRowContext(ctx, "SELECT "+orderColumns+" FROM store_orders WHERE license_id=? AND state IN ('created','pending') AND expires>? ORDER BY created DESC LIMIT 1",
			licenseID, now).Scan)
		if err == nil {
			out = open
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		q, err := s.quote(ctx, tx, l, now, false)
		if err != nil {
			return err
		}
		if q.WaitUntilEnd {
			return ErrBuyAfterEnd
		}
		kind, amount := q.Kind, q.AmountMinor
		id, err := payment.NewOrderReference(licenseID, false)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO store_orders(id,member_id,license_id,kind,amount_minor,currency,live,state,locked,created,expires,updated) VALUES(?,?,?,?,?,?,0,'created',?,?,?,?)",
			id, member, licenseID, kind, amount, "USD", boolInt(q.Locked), now, q.Expires, now); err != nil {
			return err
		}
		if err := s.audit(ctx, tx, memberActor(member), "order_created", id, "", kind+" "+strconv.FormatInt(amount, 10)+" USD test", ""); err != nil {
			return err
		}
		out, err = s.orderTx(ctx, tx, id)
		return err
	})
	return out, err
}

// Quote is what buying a year of a licence would be now: its kind, its price, where the year
// would start (the later of now and the current end, as granted on payment) and until when the
// order (and its checkout) can be paid.
type Quote struct {
	Kind        string
	AmountMinor int64
	Currency    string
	Locked      bool // a renewal before the end keeps the price its last paid year cost
	From, To    int64
	Expires     int64 // the order's end: a day, or the licence's end for a locked renewal
	// WaitUntilEnd: a locked renewal has less time before the licence's end than the
	// provider's shortest checkout; the year is bought as a new one after the end.
	WaitUntilEnd bool
}

// quote prices a year in one mode (live or test): "has paid before" and the locked price look
// only at that mode's payments (S2 review, 3). The price is locked only while the licence's
// current end was set by its last paid year -- never by a free renewal (S2 review, 2d).
func (s *Service) quote(ctx context.Context, q querier, l *row, now int64, live bool) (Quote, error) {
	price, err := s.currentPrice(ctx, q, "USD")
	if err != nil {
		return Quote{}, err
	}
	out := Quote{Kind: KindFirstYear, AmountMinor: price.AmountMinor, Currency: "USD", From: now, Expires: now + OrderLifetime}
	if l.Expires.Valid && l.Expires.Int64 > now {
		out.From = l.Expires.Int64
	}
	out.To = anniversary(out.From)
	var everPaid int
	if err := q.QueryRowContext(ctx, "SELECT COUNT(*) FROM store_orders WHERE license_id=? AND live=? AND state IN ('paid','refunded')", l.ID, boolInt(live)).Scan(&everPaid); err != nil {
		return Quote{}, err
	}
	if everPaid == 0 {
		return out, nil
	}
	out.Kind = KindRenewal
	var amount, grantedTo, paidAt sql.NullInt64
	err = q.QueryRowContext(ctx, "SELECT o.amount_minor,o.granted_to,(SELECT MIN(confirmed) FROM store_payments p WHERE p.order_id=o.id) FROM store_orders o WHERE o.license_id=? AND o.live=? AND o.state='paid' AND o.currency='USD' ORDER BY o.granted_to DESC,o.updated DESC LIMIT 1",
		l.ID, boolInt(live)).Scan(&amount, &grantedTo, &paidAt)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Quote{}, err
	}
	if !amount.Valid || !l.Expires.Valid || l.Expires.Int64 <= now || grantedTo.Int64 > l.Expires.Int64 || amount.Int64 > out.AmountMinor {
		return out, nil
	}
	var freeAfter int
	if err := q.QueryRowContext(ctx, "SELECT COUNT(*) FROM events WHERE license_id=? AND kind='renewed_free' AND created>=?", l.ID, paidAt.Int64).Scan(&freeAfter); err != nil {
		return Quote{}, err
	}
	if freeAfter > 0 {
		return out, nil // the end was moved by a free renewal: interrupted
	}
	// Uninterrupted: the price does not rise, and the order is valid only until the end.
	if l.Expires.Int64-now < MinimumOrderLifetime {
		out.WaitUntilEnd = true
		return out, nil
	}
	out.AmountMinor, out.Locked = amount.Int64, true
	if l.Expires.Int64 < out.Expires {
		out.Expires = l.Expires.Int64
	}
	return out, nil
}

// QuoteFor is the quote for a member's own licence (the page before payment).
func (s *Service) QuoteFor(member int64, licenseID string) (Quote, error) {
	if !s.StoreReady() {
		return Quote{}, ErrStoreNotReady
	}
	ctx := context.Background()
	l, err := scanRow(s.db.QueryRowContext(ctx, "SELECT "+rowColumns+" FROM licenses WHERE id=? AND member_id=? AND removed IS NULL", licenseID, member))
	if err != nil {
		return Quote{}, err
	}
	if l == nil {
		return Quote{}, ErrLicenseNotFound
	}
	if err := s.loadRevoked(ctx, s.db, l); err != nil {
		return Quote{}, err
	}
	if l.Revoked {
		return Quote{}, ErrLicenseRevoked
	}
	return s.quote(ctx, s.db, l, s.Now(), false)
}

func memberActor(member int64) string { return "member:" + strconv.FormatInt(member, 10) }

// moveOrder changes an order's state only from an allowed predecessor.
func (s *Service) moveOrder(ctx context.Context, tx *sql.Tx, o *Order, to, actor, reason string) error {
	if o.State == to {
		return nil
	}
	allowed := false
	for _, next := range orderNext[o.State] {
		allowed = allowed || next == to
	}
	if !allowed {
		return errors.New("membership: order " + o.ID + " cannot go from " + o.State + " to " + to)
	}
	result, err := tx.ExecContext(ctx, "UPDATE store_orders SET state=?,updated=? WHERE id=? AND state=?", to, s.Now(), o.ID, o.State)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return errors.New("membership: order " + o.ID + " changed meanwhile")
	}
	if err := s.audit(ctx, tx, actor, "order_state", o.ID, o.State, to, reason); err != nil {
		return err
	}
	o.State = to
	return nil
}

// AttachCheckout records the provider's checkout reference (created -> pending). The same
// reference again is a no-op.
func (s *Service) AttachCheckout(orderID, provider, ref, address string) error {
	if !s.StoreReady() {
		return ErrStoreNotReady
	}
	ctx := context.Background()
	return s.atomic(func(tx *sql.Tx) error {
		o, err := s.orderTx(ctx, tx, orderID)
		if err != nil {
			return err
		}
		if o.ProviderRef != "" {
			if o.Provider == provider && o.ProviderRef == ref {
				return nil
			}
			return errors.New("membership: order " + orderID + " already has another checkout")
		}
		if _, err := tx.ExecContext(ctx, "UPDATE store_orders SET provider=?,provider_ref=?,session_url=?,updated=? WHERE id=?", provider, ref, address, s.Now(), orderID); err != nil {
			return err
		}
		if o.State == OrderCreated {
			return s.moveOrder(ctx, tx, &o, OrderPending, memberActor(o.Member), "checkout "+provider)
		}
		return nil
	})
}

// RecordEvent stores a provider's notification id once. fresh is false for a replay or a
// duplicate (the store still confirms the order: the provider's current state decides).
func (s *Service) RecordEvent(provider, eventID, kind, reference string) (bool, error) {
	if !s.StoreReady() {
		return false, ErrStoreNotReady
	}
	result, err := s.db.ExecContext(context.Background(), "INSERT INTO store_events(provider,event_id,kind,reference,received) VALUES(?,?,?,?,?) ON CONFLICT DO NOTHING",
		provider, eventID, kind, reference, s.Now())
	if err != nil {
		return false, err
	}
	n, _ := result.RowsAffected()
	return n == 1, nil
}

// ExpireOrder ends an unpaid order past its lifetime that the provider shows as not paid
// (the caller confirmed each one first: reconciliation).
// force ends it before its time (its checkout session ended at the provider).
func (s *Service) ExpireOrder(orderID, actor string, force bool) error {
	ctx := context.Background()
	return s.atomic(func(tx *sql.Tx) error {
		o, err := s.orderTx(ctx, tx, orderID)
		if err != nil {
			return err
		}
		if (o.State != OrderCreated && o.State != OrderPending) || (o.Expires > s.Now() && !force) {
			return nil
		}
		return s.moveOrder(ctx, tx, &o, OrderExpired, actor, "unpaid; its checkout ended")
	})
}

// ---------------------------------------------------------------------------
// Applying a confirmed fact: the only way an order is paid, a year granted or a refund
// completed.

// ApplyFact records a fact that Confirm returned for this order's Expected. A paid fact
// grants the year in the same transaction that records the payment, once per (provider,
// payment id); a refunded fact ends a first year (revoked, the generation raised); a
// mismatch marks the order for the operator and grants nothing.
func (s *Service) ApplyFact(orderID, provider, actor string, fact payment.Fact) (Order, error) {
	if !s.StoreReady() {
		return Order{}, ErrStoreNotReady
	}
	ctx := context.Background()
	var out Order
	err := s.atomic(func(tx *sql.Tx) error {
		o, err := s.orderTx(ctx, tx, orderID)
		if err != nil {
			return err
		}
		defer func() { out = o }()
		now := s.Now()
		switch fact.Outcome {
		case payment.OutcomeMismatch:
			if o.State == OrderPaid || o.State == OrderRefunded || o.State == OrderMismatch {
				return s.audit(ctx, tx, actor, "mismatch_seen", o.ID, o.State, fact.Mismatch, "a later fact did not match; nothing changed")
			}
			return s.moveOrder(ctx, tx, &o, OrderMismatch, actor, "the provider's object does not match: "+fact.Mismatch)
		case payment.OutcomeNotPaid:
			if fact.Status == payment.StatusFailed && (o.State == OrderCreated || o.State == OrderPending) {
				return s.moveOrder(ctx, tx, &o, OrderFailed, actor, "the provider shows the payment failed")
			}
			return nil
		case payment.OutcomePaid, payment.OutcomeRefunded, payment.OutcomePartlyRefunded, payment.OutcomeReversed:
		default:
			return s.audit(ctx, tx, actor, "fact_ignored", o.ID, o.State, string(fact.Outcome), "not a payment of a first year")
		}
		// Defence in depth (S2 review, 11): the fact is this order's, in its mode, for its amount
		// and currency -- whatever the caller confirmed.
		if fact.PaymentID == "" || fact.LicenseID != o.LicenseID || fact.Live != o.Live || fact.Order != o.ID ||
			fact.PaidMinor != o.AmountMinor || !strings.EqualFold(fact.Currency, o.Currency) {
			return errors.New("membership: a fact that was not confirmed against this order")
		}
		// The payment, once per (provider, payment id).
		var owner string
		err = tx.QueryRowContext(ctx, "SELECT order_id FROM store_payments WHERE provider=? AND payment_id=?", provider, fact.PaymentID).Scan(&owner)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			if _, err := tx.ExecContext(ctx, "INSERT INTO store_payments(provider,payment_id,order_id,outcome,paid_minor,refunded_minor,currency,live,disputed,confirmed,updated) VALUES(?,?,?,?,?,?,?,?,?,?,?)",
				provider, fact.PaymentID, o.ID, string(fact.Outcome), fact.PaidMinor, fact.RefundedMinor, strings.ToUpper(fact.Currency), boolInt(fact.Live), boolInt(fact.Disputed), now, now); err != nil {
				return err
			}
			if err := s.audit(ctx, tx, actor, "payment_recorded", o.ID, "", provider+" "+fact.PaymentID+" "+string(fact.Outcome), ""); err != nil {
				return err
			}
		case err != nil:
			return err
		case owner != o.ID:
			return errors.New("membership: payment " + fact.PaymentID + " belongs to another order")
		default:
			if _, err := tx.ExecContext(ctx, "UPDATE store_payments SET outcome=?,refunded_minor=?,disputed=?,updated=? WHERE provider=? AND payment_id=?",
				string(fact.Outcome), fact.RefundedMinor, boolInt(fact.Disputed), now, provider, fact.PaymentID); err != nil {
				return err
			}
		}
		if fact.Disputed && fact.Outcome != payment.OutcomeReversed {
			// An open or won dispute changes nothing; the operator sees it (orders -attention).
			if err := s.flag(ctx, tx, &o, actor, "dispute "+fact.DisputeStatus); err != nil {
				return err
			}
		}
		switch fact.Outcome {
		case payment.OutcomePaid:
			if o.State == OrderPaid || o.State == OrderRefunded || o.State == OrderMismatch {
				return nil // granted once; a replay changes nothing
			}
			return s.grantYear(ctx, tx, &o, actor, now)
		case payment.OutcomePartlyRefunded:
			return s.flag(ctx, tx, &o, actor, "partly refunded at the provider: "+strconv.FormatInt(fact.RefundedMinor, 10)+" of "+strconv.FormatInt(fact.PaidMinor, 10))
		case payment.OutcomeReversed:
			if err := s.flag(ctx, tx, &o, actor, "dispute lost: the payment was taken back"); err != nil {
				return err
			}
			return s.completeRefund(ctx, tx, &o, actor, now)
		default: // refunded in full
			return s.completeRefund(ctx, tx, &o, actor, now)
		}
	})
	return out, err
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// grantYear moves the licence's end by one year: the new end is the anniversary of the later
// of the payment's confirmation and the current end. For a first year that is the reading the
// coordinator confirmed (a free term still running is not cut short); for a renewal it is the
// owner's rule of 23 November 2026 (paid before the end: from the old end; after the end:
// from the payment; no grace).
func (s *Service) grantYear(ctx context.Context, tx *sql.Tx, o *Order, actor string, now int64) error {
	l, err := scanRow(tx.QueryRowContext(ctx, "SELECT "+rowColumns+" FROM licenses WHERE id=?", o.LicenseID))
	if err != nil {
		return err
	}
	if l == nil {
		return errors.New("membership: order " + o.ID + " names a licence that does not exist")
	}
	if err := s.loadRevoked(ctx, tx, l); err != nil {
		return err
	}
	// Nothing is granted into nothing (S2 review, 3, 4, 7): the payment is recorded, the order
	// goes to the operator.
	refuse := func(why string) error {
		if err := s.flag(ctx, tx, o, actor, why); err != nil {
			return err
		}
		return s.moveOrder(ctx, tx, o, OrderMismatch, actor, why)
	}
	switch {
	case l.Removed.Valid:
		return refuse("paid for a licence the member removed; nothing granted")
	case l.Revoked:
		return refuse("paid for a licence that was revoked; nothing granted")
	case o.Locked && (!l.Expires.Valid || l.Expires.Int64 <= now):
		return refuse("the locked price was paid after the licence's end; nothing granted")
	}
	if !o.Live {
		test, err := isTestAccount(ctx, tx, o.Member)
		if err != nil {
			return err
		}
		if !test {
			return refuse("a test payment for an account that is no longer a test account; nothing granted")
		}
	}
	from := now
	if l.Expires.Valid && l.Expires.Int64 > from {
		from = l.Expires.Int64
	}
	to := anniversary(from)
	if _, err := tx.ExecContext(ctx, "UPDATE licenses SET expires=? WHERE id=?", to, l.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE store_orders SET granted_from=?,granted_to=?,previous_expires=?,updated=? WHERE id=?",
		from, to, l.Expires.Int64, now, o.ID); err != nil {
		return err
	}
	o.GrantedFrom, o.GrantedTo, o.PreviousExpires = from, to, l.Expires.Int64
	if err := event(ctx, tx, now, l.Member, &l.ID, "paid_year"); err != nil {
		return err
	}
	if err := s.audit(ctx, tx, actor, "year_granted", l.ID, strconv.FormatInt(l.Expires.Int64, 10), strconv.FormatInt(to, 10), "order "+o.ID); err != nil {
		return err
	}
	if err := s.moveOrder(ctx, tx, o, OrderPaid, actor, "the provider confirmed the payment"); err != nil {
		return err
	}
	// The receipt is claimed in the same transaction (S2 review, 5): only this call mails it.
	result, err := tx.ExecContext(ctx, "UPDATE store_orders SET receipt_sent=? WHERE id=? AND receipt_sent IS NULL", now, o.ID)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	o.ReceiptMine = n == 1
	return nil
}

// flag records why the operator must look at an order (orders -attention) and logs it.
func (s *Service) flag(ctx context.Context, tx *sql.Tx, o *Order, actor, why string) error {
	if _, err := tx.ExecContext(ctx, "UPDATE store_orders SET attention=?,updated=? WHERE id=?", why, s.Now(), o.ID); err != nil {
		return err
	}
	o.Attention = why
	return s.audit(ctx, tx, actor, "attention", o.ID, "", why, "")
}

// completeRefund applies a refund (or a lost dispute) that happened at the provider -- there
// are no refunds by policy (the owner, 23 November 2026), but the operator's exceptional act
// or a chargeback still arrives as a confirmed fact. A first year: the licence is revoked in
// one statement that clears the binding and raises the generation (the rule), its end set to
// the refund time at the latest (so the PHP service, which does not read "revoked", also
// refuses to activate it). A renewal: the end goes back by the year it gave. An order that
// was never fulfilled only becomes refunded.
func (s *Service) completeRefund(ctx context.Context, tx *sql.Tx, o *Order, actor string, now int64) error {
	if o.State == OrderRefunded {
		return nil
	}
	if o.State == OrderMismatch {
		// A payment that was never granted (a mismatch) and was given back at the provider:
		// settled; nothing on the licence to undo (S2 review, 8).
		return s.flag(ctx, tx, o, actor, "settled: the payment was given back at the provider")
	}
	fulfilled := o.State == OrderPaid
	if fulfilled {
		if o.Kind == KindRenewal {
			var expires int64
			if err := tx.QueryRowContext(ctx, "SELECT COALESCE(expires,0) FROM licenses WHERE id=?", o.LicenseID).Scan(&expires); err != nil {
				return err
			}
			back := expires - (o.GrantedTo - o.GrantedFrom)
			if expires == o.GrantedTo {
				back = o.PreviousExpires // nothing changed since: exactly what it was
			}
			if _, err := tx.ExecContext(ctx, "UPDATE licenses SET expires=? WHERE id=?", back, o.LicenseID); err != nil {
				return err
			}
			if err := s.audit(ctx, tx, actor, "renewal_taken_back", o.LicenseID, strconv.FormatInt(expires, 10), strconv.FormatInt(back, 10), "order "+o.ID+": refunded at the provider"); err != nil {
				return err
			}
		} else {
			if _, err := tx.ExecContext(ctx, "UPDATE licenses SET revoked=?,revoked_reason='refunded',server_id=NULL,server_ip=NULL,hostname=NULL,expires=MIN(COALESCE(expires,?),?),generation=generation+1 WHERE id=? AND revoked IS NULL",
				now, now, now, o.LicenseID); err != nil {
				return err
			}
			var member int64
			if err := tx.QueryRowContext(ctx, "SELECT member_id FROM licenses WHERE id=?", o.LicenseID).Scan(&member); err != nil {
				return err
			}
			if err := event(ctx, tx, now, member, &o.LicenseID, "revoked_refunded"); err != nil {
				return err
			}
			if err := s.audit(ctx, tx, actor, "licence_revoked", o.LicenseID, "", "revoked (refunded)", "order "+o.ID); err != nil {
				return err
			}
		}
	}
	if _, err := tx.ExecContext(ctx, "UPDATE store_refunds SET state='done',outcome='refunded',updated=? WHERE order_id=? AND state IN ('requested','sent')", now, o.ID); err != nil {
		return err
	}
	return s.moveOrder(ctx, tx, o, OrderRefunded, actor, "the provider confirmed the refund")
}

// ---------------------------------------------------------------------------
// Refunds: there are none by policy (the owner, 23 November 2026: "hiçbir zaman iade yok").
// The operator's exceptional act remains, with a reason, executed through the provider's
// Refund with confirm-first and completed only by a confirmed fact. No page offers it.

// Refund is one refund request and its attempt number.
type Refund struct {
	ID      int64
	OrderID string
	Attempt int
	State   string // requested, sent, done, refused, final
	Outcome string
}

// RequestRefund opens (or returns the open) refund of a paid order: an operator's exceptional
// act with a reason. The attempt is persisted before the provider is asked; after a definite
// refusal the next request raises it.
func (s *Service) RequestRefund(actor, orderID, reason string) (Refund, error) {
	if strings.TrimSpace(reason) == "" {
		return Refund{}, ErrInvalidRequest
	}
	if !s.StoreReady() {
		return Refund{}, ErrStoreNotReady
	}
	ctx := context.Background()
	var out Refund
	err := s.atomic(func(tx *sql.Tx) error {
		o, err := s.orderTx(ctx, tx, orderID)
		if err != nil {
			return err
		}
		if o.State != OrderPaid {
			return ErrRefundNotPossible
		}
		open, err := scanRefund(tx.QueryRowContext(ctx, "SELECT id,order_id,attempt,state,outcome FROM store_refunds WHERE order_id=? AND state IN ('requested','sent')", o.ID).Scan)
		if err == nil {
			out = open
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		var final int
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM store_refunds WHERE order_id=? AND state IN ('done','final')", o.ID).Scan(&final); err != nil {
			return err
		}
		if final > 0 {
			return ErrRefundNotPossible
		}
		var attempt int
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM store_refunds WHERE order_id=?", o.ID).Scan(&attempt); err != nil {
			return err
		}
		now := s.Now()
		result, err := tx.ExecContext(ctx, "INSERT INTO store_refunds(order_id,attempt,state,requested_by,reason,created,updated) VALUES(?,?,'requested',?,?,?,?)",
			o.ID, attempt, actor, reason, now, now)
		if err != nil {
			return err
		}
		id, _ := result.LastInsertId()
		if err := s.audit(ctx, tx, actor, "refund_requested", o.ID, "", "attempt "+strconv.Itoa(attempt), reason); err != nil {
			return err
		}
		out = Refund{ID: id, OrderID: o.ID, Attempt: attempt, State: "requested"}
		return nil
	})
	return out, err
}

func scanRefund(scan func(...any) error) (Refund, error) {
	var r Refund
	err := scan(&r.ID, &r.OrderID, &r.Attempt, &r.State, &r.Outcome)
	return r, err
}

// Refunds lists an order's refund requests, oldest first.
func (s *Service) Refunds(orderID string) ([]Refund, error) {
	if !s.StoreReady() {
		return nil, ErrStoreNotReady
	}
	rows, err := s.db.QueryContext(context.Background(), "SELECT id,order_id,attempt,state,outcome FROM store_refunds WHERE order_id=? ORDER BY id", orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Refund
	for rows.Next() {
		r, err := scanRefund(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// RefundAnswered records what the provider's Refund answered for this attempt:
//   - nil: accepted (the money is on its way); the refund completes when a confirmed fact
//     shows it refunded;
//   - ErrProviderUnavailable / ErrProviderBusy: unknown; the same attempt is asked again;
//   - ErrProviderRefused: refused; the next request raises the attempt;
//   - ErrAlreadyRefunded: the provider already gave it back; a confirmed fact completes it;
//   - any other final answer (disputed, mismatch, partly refunded, nothing paid): final, for
//     the operator.
func (s *Service) RefundAnswered(refundID int64, actor string, answer error) error {
	if !s.StoreReady() {
		return ErrStoreNotReady
	}
	state, outcome := "sent", "being processed"
	switch {
	case answer == nil:
	case errors.Is(answer, payment.ErrProviderUnavailable), errors.Is(answer, payment.ErrProviderBusy):
		outcome = "unknown; asked again"
	case errors.Is(answer, payment.ErrProviderRefused):
		state, outcome = "refused", "refused by the provider"
	case errors.Is(answer, payment.ErrAlreadyRefunded):
		outcome = "already refunded at the provider"
	case errors.Is(answer, payment.ErrDisputed):
		state, outcome = "final", "disputed"
	case errors.Is(answer, payment.ErrPartlyRefunded):
		state, outcome = "final", "partly refunded"
	case errors.Is(answer, payment.ErrMismatch):
		state, outcome = "final", "mismatch"
	case errors.Is(answer, payment.ErrNothingPaid):
		state, outcome = "final", "nothing paid"
	default:
		outcome = "unknown; asked again"
	}
	ctx := context.Background()
	return s.atomic(func(tx *sql.Tx) error {
		r, err := scanRefund(tx.QueryRowContext(ctx, "SELECT id,order_id,attempt,state,outcome FROM store_refunds WHERE id=?", refundID).Scan)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrOrderNotFound
		}
		if err != nil {
			return err
		}
		if r.State != "requested" && r.State != "sent" {
			return nil
		}
		if _, err := tx.ExecContext(ctx, "UPDATE store_refunds SET state=?,outcome=?,updated=? WHERE id=?", state, outcome, s.Now(), refundID); err != nil {
			return err
		}
		return s.audit(ctx, tx, actor, "refund_answered", r.OrderID, r.State, state+": "+outcome, "attempt "+strconv.Itoa(r.Attempt))
	})
}

// ---------------------------------------------------------------------------
// Operator: a gift extension (a year or days by hand, with a reason).

// ExtendByHand moves a licence's end by days, from the later of now and the current end;
// an operator act with a reason (a gift). It never touches the binding or the generation.
func (s *Service) ExtendByHand(actor, reason, licenseID string, days int) error {
	if strings.TrimSpace(reason) == "" || days <= 0 || days > 3660 || !hex32.MatchString(licenseID) {
		return ErrInvalidRequest
	}
	ctx := context.Background()
	return s.atomic(func(tx *sql.Tx) error {
		l, err := scanRow(tx.QueryRowContext(ctx, "SELECT "+rowColumns+" FROM licenses WHERE id=? AND removed IS NULL", licenseID))
		if err != nil {
			return err
		}
		if l == nil {
			return ErrLicenseNotFound
		}
		if err := s.loadRevoked(ctx, tx, l); err != nil {
			return err
		}
		if l.Revoked {
			return ErrLicenseRevoked
		}
		from := s.Now()
		if l.Expires.Valid && l.Expires.Int64 > from {
			from = l.Expires.Int64
		}
		to := from + int64(days)*86400
		if _, err := tx.ExecContext(ctx, "UPDATE licenses SET expires=? WHERE id=?", to, l.ID); err != nil {
			return err
		}
		if err := event(ctx, tx, s.Now(), l.Member, &l.ID, "extended_by_operator"); err != nil {
			return err
		}
		return s.audit(ctx, tx, actor, "extended_by_hand", l.ID, strconv.FormatInt(l.Expires.Int64, 10), strconv.FormatInt(to, 10), reason)
	})
}

// LicenceRevocation says whether and why the store revoked a licence (false when the file
// has no store).
func (s *Service) LicenceRevocation(licenseID string) (bool, string, error) {
	if !s.hasRevoked {
		return false, "", nil
	}
	var at sql.NullInt64
	var why sql.NullString
	err := s.db.QueryRowContext(context.Background(), "SELECT revoked,revoked_reason FROM licenses WHERE id=?", licenseID).Scan(&at, &why)
	if errors.Is(err, sql.ErrNoRows) {
		return false, "", nil
	}
	return at.Valid, why.String, err
}

// MemberIDByEmail finds a member for the operator's commands (0 when there is none).
func (s *Service) MemberIDByEmail(email string) (int64, error) {
	normal, err := normalEmail(email)
	if err != nil {
		return 0, err
	}
	var id int64
	err = s.db.QueryRowContext(context.Background(), "SELECT id FROM members WHERE email=?", normal).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return id, err
}

// AttentionOrders lists what the operator must look at (S2 review, 1): mismatches not yet
// settled, partial refunds, open disputes, payments on a removed or revoked licence, and
// refunds that failed or have been "sent" for more than a day.
func (s *Service) AttentionOrders() ([]Order, error) {
	if !s.StoreReady() {
		return nil, ErrStoreNotReady
	}
	rows, err := s.db.QueryContext(context.Background(), "SELECT "+orderColumns+" FROM store_orders o WHERE "+
		"(COALESCE(o.attention,'') NOT LIKE 'settled%' AND COALESCE(o.attention,'') NOT IN ('','dispute won')) "+
		"OR (o.state='mismatch' AND COALESCE(o.attention,'') NOT LIKE 'settled%') "+
		"OR EXISTS (SELECT 1 FROM store_refunds r WHERE r.order_id=o.id AND (r.state='final' OR (r.state IN ('requested','sent') AND r.updated<?))) "+
		"ORDER BY o.updated", s.Now()-86400)
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

// MarkChecked records that reconciliation confirmed an order now (at most once a day each).
func (s *Service) MarkChecked(orderID string) error {
	_, err := s.db.ExecContext(context.Background(), "UPDATE store_orders SET checked=? WHERE id=?", s.Now(), orderID)
	return err
}

// CheckedAt is when reconciliation last confirmed an order (0: never).
func (s *Service) CheckedAt(orderID string) (int64, error) {
	var at sql.NullInt64
	err := s.db.QueryRowContext(context.Background(), "SELECT checked FROM store_orders WHERE id=?", orderID).Scan(&at)
	return at.Int64, err
}

// closeOpenOrders ends a removed licence's unpaid orders (S2 review, 4); a payment that still
// arrives for one is recorded, grants nothing and goes to the operator.
func (s *Service) closeOpenOrders(ctx context.Context, tx *sql.Tx, member int64, licenseID string) error {
	rows, err := tx.QueryContext(ctx, "SELECT "+orderColumns+" FROM store_orders WHERE license_id=? AND state IN ('created','pending')", licenseID)
	if err != nil {
		return err
	}
	var open []Order
	for rows.Next() {
		o, err := scanOrder(rows.Scan)
		if err != nil {
			rows.Close()
			return err
		}
		open = append(open, o)
	}
	rows.Close()
	for i := range open {
		if err := s.moveOrder(ctx, tx, &open[i], OrderExpired, memberActor(member), "the member removed the licence"); err != nil {
			return err
		}
	}
	return nil
}

// HasOpenOrder says whether a licence has an unpaid order (the removal page says it closes).
func (s *Service) HasOpenOrder(licenseID string) bool {
	if !s.StoreReady() {
		return false
	}
	var n int
	if s.db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM store_orders WHERE license_id=? AND state IN ('created','pending')", licenseID).Scan(&n) != nil {
		return false
	}
	return n > 0
}

// TrialDays is the first licence's free trial (D-047 point 6; the owner, 23 November 2026:
// "ilk seferde 30 gün trial var").
const TrialDays = 30

// IssueCharged creates a licence for an account that buys its years (S2 review, 2): the
// account's first licence ever is the 30-day trial; any later one is created awaiting payment
// -- its end is its creation time, so neither this service nor the PHP service activates it,
// and the panels' contract is unchanged -- together with its first-year order. No payment
// details are asked for the trial.
func (s *Service) IssueCharged(member int64, mode ChargingMode) (Issued, bool, error) {
	if err := s.Limit("issue:"+strconv.FormatInt(member, 10), 10, 86400); err != nil {
		return Issued{}, false, err
	}
	if !s.StoreReady() {
		return Issued{}, false, ErrStoreNotReady
	}
	ctx := context.Background()
	var out Issued
	trial := false
	err := s.atomic(func(tx *sql.Tx) error {
		var id int64
		err := tx.QueryRowContext(ctx, "SELECT id FROM members WHERE id=? AND verified=1", member).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrAuthenticationRequired
		}
		if err != nil {
			return err
		}
		var used int
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM store_trials WHERE member_id=?", member).Scan(&used); err != nil {
			return err
		}
		now := s.Now()
		if used == 0 {
			trial = true
			if out, err = s.insertLicence(ctx, tx, member, now+TrialDays*86400, "trial_started"); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, "INSERT INTO store_trials(member_id,license_id,started,ends) VALUES(?,?,?,?)", member, out.ID, now, now+TrialDays*86400); err != nil {
				return err
			}
			return s.audit(ctx, tx, memberActor(member), "trial_started", out.ID, "", strconv.FormatInt(now+TrialDays*86400, 10), "")
		}
		if out, err = s.insertLicence(ctx, tx, member, now, "issued_awaiting_payment"); err != nil {
			return err
		}
		return s.audit(ctx, tx, memberActor(member), "licence_awaiting_payment", out.ID, "", "awaiting its first year", "")
	})
	if err != nil || trial {
		return out, trial, err
	}
	_, err = s.CreateOrder(member, out.ID, mode)
	return out, false, err
}

// Trial is a member's trial licence and its end (empty when the member has none).
func (s *Service) Trial(member int64) (string, int64, error) {
	var id string
	var ends int64
	err := s.db.QueryRowContext(context.Background(), "SELECT license_id,ends FROM store_trials WHERE member_id=?", member).Scan(&id, &ends)
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, nil
	}
	return id, ends, err
}

// AwaitingPayment says whether a licence was created to be paid and has no paid year yet.
func (s *Service) AwaitingPayment(licenseID string) (bool, error) {
	var created, expires int64
	if err := s.db.QueryRowContext(context.Background(), "SELECT created,COALESCE(expires,0) FROM licenses WHERE id=?", licenseID).Scan(&created, &expires); err != nil {
		return false, err
	}
	if expires > created {
		return false, nil
	}
	var paid int
	err := s.db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM store_orders WHERE license_id=? AND state='paid'", licenseID).Scan(&paid)
	return paid == 0, err
}
