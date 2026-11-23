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

// OrderLifetime: an order nobody paid ends after a day (a provider's checkout page expires
// in the same time).
const OrderLifetime = 86400

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
		"CREATE TABLE IF NOT EXISTS store_test_accounts (\n member_id INTEGER PRIMARY KEY REFERENCES members(id), flagged_by TEXT NOT NULL,\n reason TEXT NOT NULL, created INTEGER NOT NULL\n)",
		"CREATE TABLE IF NOT EXISTS store_orders (\n id TEXT PRIMARY KEY, member_id INTEGER NOT NULL REFERENCES members(id),\n license_id TEXT NOT NULL REFERENCES licenses(id), kind TEXT NOT NULL CHECK(kind IN ('first_year','renewal')),\n amount_minor INTEGER NOT NULL, currency TEXT NOT NULL, live INTEGER NOT NULL CHECK(live IN (0,1)),\n state TEXT NOT NULL CHECK(state IN ('created','pending','paid','mismatch','expired','failed','refunded')),\n provider TEXT, provider_ref TEXT, granted_from INTEGER, granted_to INTEGER, previous_expires INTEGER,\n created INTEGER NOT NULL, expires INTEGER NOT NULL, updated INTEGER NOT NULL\n)",
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

// SetPrice adds a price row (history is kept); an operator act with a reason.
func (s *Service) SetPrice(actor, reason, currency string, amountMinor, validFrom int64) error {
	if strings.TrimSpace(reason) == "" || len(currency) != 3 || strings.ToUpper(currency) != currency || amountMinor <= 0 {
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
}

const orderColumns = "id,member_id,license_id,kind,amount_minor,currency,live,state,COALESCE(provider,''),COALESCE(provider_ref,''),COALESCE(granted_from,0),COALESCE(granted_to,0),COALESCE(previous_expires,0),created,expires,updated"

func scanOrder(scan func(...any) error) (Order, error) {
	var o Order
	var live int
	err := scan(&o.ID, &o.Member, &o.LicenseID, &o.Kind, &o.AmountMinor, &o.Currency, &live, &o.State, &o.Provider, &o.ProviderRef,
		&o.GrantedFrom, &o.GrantedTo, &o.PreviousExpires, &o.Created, &o.Expires, &o.Updated)
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
		q, err := s.quote(ctx, tx, l, now)
		if err != nil {
			return err
		}
		kind, amount := q.Kind, q.AmountMinor
		id, err := payment.NewOrderReference(licenseID, false)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO store_orders(id,member_id,license_id,kind,amount_minor,currency,live,state,created,expires,updated) VALUES(?,?,?,?,?,?,0,'created',?,?,?)",
			id, member, licenseID, kind, amount, "USD", now, now+OrderLifetime, now); err != nil {
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

// Quote is what buying a year of a licence would be now: its kind, its price and where the
// year would start (the later of now and the current end, as granted on payment).
type Quote struct {
	Kind        string
	AmountMinor int64
	Currency    string
	Locked      bool // a renewal before the end keeps the price its last paid year cost
	From, To    int64
}

func (s *Service) quote(ctx context.Context, q querier, l *row, now int64) (Quote, error) {
	price, err := s.currentPrice(ctx, q, "USD")
	if err != nil {
		return Quote{}, err
	}
	out := Quote{Kind: KindFirstYear, AmountMinor: price.AmountMinor, Currency: "USD", From: now}
	if l.Expires.Valid && l.Expires.Int64 > now {
		out.From = l.Expires.Int64
	}
	out.To = anniversary(out.From)
	var everPaid int
	if err := q.QueryRowContext(ctx, "SELECT COUNT(*) FROM store_orders WHERE license_id=? AND state IN ('paid','refunded')", l.ID).Scan(&everPaid); err != nil {
		return Quote{}, err
	}
	if everPaid == 0 {
		return out, nil
	}
	out.Kind = KindRenewal
	var locked sql.NullInt64
	err = q.QueryRowContext(ctx, "SELECT amount_minor FROM store_orders WHERE license_id=? AND state='paid' AND currency='USD' ORDER BY granted_to DESC,updated DESC LIMIT 1", l.ID).Scan(&locked)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Quote{}, err
	}
	if locked.Valid && l.Expires.Valid && l.Expires.Int64 > now && locked.Int64 <= out.AmountMinor {
		out.AmountMinor, out.Locked = locked.Int64, true // uninterrupted: the price does not rise
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
	return s.quote(ctx, s.db, l, s.Now())
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
func (s *Service) AttachCheckout(orderID, provider, ref string) error {
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
		if _, err := tx.ExecContext(ctx, "UPDATE store_orders SET provider=?,provider_ref=?,updated=? WHERE id=?", provider, ref, s.Now(), orderID); err != nil {
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
func (s *Service) ExpireOrder(orderID, actor string) error {
	ctx := context.Background()
	return s.atomic(func(tx *sql.Tx) error {
		o, err := s.orderTx(ctx, tx, orderID)
		if err != nil {
			return err
		}
		if (o.State != OrderCreated && o.State != OrderPending) || o.Expires > s.Now() {
			return nil
		}
		return s.moveOrder(ctx, tx, &o, OrderExpired, actor, "unpaid after a day")
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
		case payment.OutcomePaid, payment.OutcomeRefunded, payment.OutcomePartlyRefunded:
		default:
			return s.audit(ctx, tx, actor, "fact_ignored", o.ID, o.State, string(fact.Outcome), "not a payment of a first year")
		}
		if fact.PaymentID == "" || fact.LicenseID != o.LicenseID || fact.Live != o.Live {
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
		if fact.Disputed {
			if err := s.audit(ctx, tx, actor, "disputed", o.ID, "", provider+" "+fact.PaymentID, "the payer's bank disputes the payment; the operator decides"); err != nil {
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
			return s.audit(ctx, tx, actor, "partly_refunded", o.ID, "", strconv.FormatInt(fact.RefundedMinor, 10)+" of "+strconv.FormatInt(fact.PaidMinor, 10),
				"a partial refund was made at the provider; the operator decides")
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
	return s.moveOrder(ctx, tx, o, OrderPaid, actor, "the provider confirmed the payment")
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
