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
// Version 2 (the S2 review's second fix round): receipts owed and sent, payments' dispute
// status, an attention list with its own entries, trials per mode and per server, and the
// revocations' history. MigrateStore brings a version 1 file (the earlier S2 commits) to it.
const StoreVersion = 2

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
	OrderMismatch: {OrderPaid}, // only the operator's grant (GrantOrder) for a payment that arrived
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
		"CREATE TABLE IF NOT EXISTS store_trials (\n member_id INTEGER NOT NULL REFERENCES members(id), mode TEXT NOT NULL CHECK(mode IN ('test','live')),\n license_id TEXT NOT NULL REFERENCES licenses(id), started INTEGER NOT NULL, ends INTEGER NOT NULL, refused INTEGER,\n PRIMARY KEY(member_id, mode)\n)",
		"CREATE TABLE IF NOT EXISTS store_trial_servers (\n server_id TEXT PRIMARY KEY, license_id TEXT NOT NULL REFERENCES licenses(id), at INTEGER NOT NULL\n)",
		"CREATE TABLE IF NOT EXISTS store_revocations (\n id INTEGER PRIMARY KEY, license_id TEXT NOT NULL REFERENCES licenses(id), at INTEGER NOT NULL,\n expires_before INTEGER NOT NULL, reason TEXT NOT NULL, reinstated INTEGER\n)",
		"CREATE TABLE IF NOT EXISTS store_attention (\n id INTEGER PRIMARY KEY, order_id TEXT NOT NULL REFERENCES store_orders(id), kind TEXT NOT NULL,\n detail TEXT NOT NULL, state TEXT NOT NULL CHECK(state IN ('open','closed')), opened INTEGER NOT NULL,\n closed INTEGER, closed_by TEXT, close_reason TEXT\n)",
		"CREATE UNIQUE INDEX IF NOT EXISTS store_attention_open ON store_attention(order_id, kind) WHERE state='open'",
		"CREATE TABLE IF NOT EXISTS store_test_accounts (\n member_id INTEGER PRIMARY KEY REFERENCES members(id), flagged_by TEXT NOT NULL,\n reason TEXT NOT NULL, created INTEGER NOT NULL\n)",
		"CREATE TABLE IF NOT EXISTS store_orders (\n id TEXT PRIMARY KEY, member_id INTEGER NOT NULL REFERENCES members(id),\n license_id TEXT NOT NULL REFERENCES licenses(id), kind TEXT NOT NULL CHECK(kind IN ('first_year','renewal')),\n amount_minor INTEGER NOT NULL, currency TEXT NOT NULL, live INTEGER NOT NULL CHECK(live IN (0,1)),\n state TEXT NOT NULL CHECK(state IN ('created','pending','paid','mismatch','expired','failed','refunded')),\n provider TEXT, provider_ref TEXT, locked INTEGER NOT NULL DEFAULT 0,\n granted_from INTEGER, granted_to INTEGER, previous_expires INTEGER, receipt_owed INTEGER, receipt_sent INTEGER, checked INTEGER,\n created INTEGER NOT NULL, expires INTEGER NOT NULL, updated INTEGER NOT NULL\n)",
		"CREATE INDEX IF NOT EXISTS store_orders_license ON store_orders(license_id, state)",
		"CREATE INDEX IF NOT EXISTS store_orders_ref ON store_orders(provider, provider_ref)",
		"CREATE TRIGGER IF NOT EXISTS store_orders_forward BEFORE UPDATE OF state ON store_orders\n WHEN OLD.state<>NEW.state AND NOT (" + strings.Join(sortedStrings(pairs), " OR ") + ")\n BEGIN SELECT RAISE(ABORT, 'store_orders: a state only moves forward'); END",
		"CREATE TABLE IF NOT EXISTS store_events (\n provider TEXT NOT NULL, event_id TEXT NOT NULL, kind TEXT NOT NULL, reference TEXT NOT NULL,\n received INTEGER NOT NULL, PRIMARY KEY(provider, event_id)\n)",
		"CREATE TABLE IF NOT EXISTS store_payments (\n provider TEXT NOT NULL, payment_id TEXT NOT NULL, order_id TEXT NOT NULL REFERENCES store_orders(id),\n outcome TEXT NOT NULL, paid_minor INTEGER NOT NULL, refunded_minor INTEGER NOT NULL, currency TEXT NOT NULL,\n live INTEGER NOT NULL, disputed INTEGER NOT NULL, dispute_status TEXT NOT NULL DEFAULT '', confirmed INTEGER NOT NULL, updated INTEGER NOT NULL,\n PRIMARY KEY(provider, payment_id)\n)",
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
		upgraded, err := s.migrateStoreFromVersion1(ctx, tx)
		if err != nil {
			return err
		}
		for _, statement := range storeSchema() {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return err
			}
		}
		if upgraded {
			if _, err := tx.ExecContext(ctx, "UPDATE store_meta SET value=? WHERE key='version'", strconv.Itoa(StoreVersion)); err != nil {
				return err
			}
			if err := s.audit(ctx, tx, actor, "store_upgraded", "store", "version 1", "version "+strconv.Itoa(StoreVersion), "membership migrate-store"); err != nil {
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

// migrateStoreFromVersion1 brings a file made by the earlier S2 commits (version 1) to
// version 2: the order and payment columns added, the trials re-keyed by mode (a version 1
// trial was a test-mode trial), the forward-only trigger replaced (a mismatch may become paid
// by the operator's grant). Nothing is removed from the orders; unused version 1 columns stay.
func (s *Service) migrateStoreFromVersion1(ctx context.Context, tx *sql.Tx) (bool, error) {
	var version string
	if err := tx.QueryRowContext(ctx, "SELECT value FROM store_meta WHERE key='version'").Scan(&version); err != nil {
		return false, nil // no store yet, or no meta table: nothing to bring forward
	}
	if version != "1" {
		return false, nil
	}
	orders, err := tableColumns(ctx, tx, "store_orders")
	if err != nil {
		return false, err
	}
	for _, add := range []struct{ name, sql string }{
		{"locked", "ALTER TABLE store_orders ADD COLUMN locked INTEGER NOT NULL DEFAULT 0"},
		{"receipt_owed", "ALTER TABLE store_orders ADD COLUMN receipt_owed INTEGER"},
		{"receipt_sent", "ALTER TABLE store_orders ADD COLUMN receipt_sent INTEGER"},
		{"checked", "ALTER TABLE store_orders ADD COLUMN checked INTEGER"},
	} {
		if !orders[add.name] {
			if _, err := tx.ExecContext(ctx, add.sql); err != nil {
				return false, err
			}
		}
	}
	if orders["receipt_sent"] { // version 1 claimed receipts in receipt_sent: they were owed
		if _, err := tx.ExecContext(ctx, "UPDATE store_orders SET receipt_owed=receipt_sent WHERE receipt_owed IS NULL AND receipt_sent IS NOT NULL"); err != nil {
			return false, err
		}
	}
	payments, err := tableColumns(ctx, tx, "store_payments")
	if err != nil {
		return false, err
	}
	if !payments["dispute_status"] {
		if _, err := tx.ExecContext(ctx, "ALTER TABLE store_payments ADD COLUMN dispute_status TEXT NOT NULL DEFAULT ''"); err != nil {
			return false, err
		}
	}
	trials, err := tableColumns(ctx, tx, "store_trials")
	if err != nil {
		return false, err
	}
	if len(trials) > 0 && !trials["mode"] {
		for _, statement := range []string{
			"ALTER TABLE store_trials RENAME TO store_trials_v1",
			"CREATE TABLE store_trials (\n member_id INTEGER NOT NULL REFERENCES members(id), mode TEXT NOT NULL CHECK(mode IN ('test','live')),\n license_id TEXT NOT NULL REFERENCES licenses(id), started INTEGER NOT NULL, ends INTEGER NOT NULL, refused INTEGER,\n PRIMARY KEY(member_id, mode)\n)",
			"INSERT INTO store_trials(member_id,mode,license_id,started,ends) SELECT member_id,'test',license_id,started,ends FROM store_trials_v1",
			"DROP TABLE store_trials_v1",
		} {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return false, err
			}
		}
	}
	if _, err := tx.ExecContext(ctx, "DROP TRIGGER IF EXISTS store_orders_forward"); err != nil {
		return false, err
	}
	return true, nil // the version is raised after the schema statements (MigrateStore)
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
// price starts now or later, never in the past (the history says when each was in force): a
// day already begun (the operator's "-from today") or 0 means now (S2 review 2, N7).
func (s *Service) SetPrice(actor, reason, currency string, amountMinor, validFrom int64) error {
	if strings.TrimSpace(reason) == "" || len(currency) != 3 || strings.ToUpper(currency) != currency || amountMinor <= 0 ||
		(validFrom != 0 && validFrom <= s.Now()-86400) {
		return ErrInvalidRequest
	}
	if validFrom < s.Now() {
		validFrom = s.Now()
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
	Locked          bool   // a renewal at the locked price (valid only before the licence's end)
	Attention       string // the open attention entries' details (filled by AttentionOrders only)
	// ReceiptMine is set (not stored) on the order ApplyFact returns when this very call made it
	// paid and claimed its receipt: only that caller mails it.
	ReceiptMine bool
}

const orderColumns = "id,member_id,license_id,kind,amount_minor,currency,live,state,COALESCE(provider,''),COALESCE(provider_ref,''),COALESCE(granted_from,0),COALESCE(granted_to,0),COALESCE(previous_expires,0),created,expires,updated,locked"

func scanOrder(scan func(...any) error) (Order, error) {
	var o Order
	var live, lockedInt int
	err := scan(&o.ID, &o.Member, &o.LicenseID, &o.Kind, &o.AmountMinor, &o.Currency, &live, &o.State, &o.Provider, &o.ProviderRef,
		&o.GrantedFrom, &o.GrantedTo, &o.PreviousExpires, &o.Created, &o.Expires, &o.Updated, &lockedInt)
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
	// The lock holds only while the licence is in paid time without a gap: now is still inside
	// its last paid year (an operator's gift after that year ended does not revive it).
	if !amount.Valid || !l.Expires.Valid || l.Expires.Int64 <= now || grantedTo.Int64 <= now || grantedTo.Int64 > l.Expires.Int64 || amount.Int64 > out.AmountMinor {
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
			if err := s.openAttention(ctx, tx, o.ID, actor, AttentionMismatch, "the provider's payment does not match the order: "+fact.Mismatch); err != nil {
				return err
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
		if _, err := tx.ExecContext(ctx, "UPDATE store_payments SET dispute_status=? WHERE provider=? AND payment_id=?", fact.DisputeStatus, provider, fact.PaymentID); err != nil {
			return err
		}
		// A dispute changes nothing for the customer while it is open; the operator sees it, and
		// reconciliation asks again until it closes (S2 review 2, N5).
		switch {
		case fact.Disputed && fact.Outcome != payment.OutcomeReversed && DisputeClosed(fact.DisputeStatus):
			if err := s.closeAttention(ctx, tx, o.ID, actor, AttentionDispute, "dispute "+fact.DisputeStatus+": nothing changes"); err != nil {
				return err
			}
		case fact.Disputed && fact.Outcome != payment.OutcomeReversed:
			if err := s.openAttention(ctx, tx, o.ID, actor, AttentionDispute, fact.DisputeStatus); err != nil {
				return err
			}
		}
		switch fact.Outcome {
		case payment.OutcomePaid:
			if o.State == OrderPaid || o.State == OrderRefunded || o.State == OrderMismatch {
				return nil // granted once; a replay changes nothing
			}
			return s.grantYear(ctx, tx, &o, actor, now, false)
		case payment.OutcomePartlyRefunded:
			return s.openAttention(ctx, tx, o.ID, actor, AttentionPartRefund, strconv.FormatInt(fact.RefundedMinor, 10)+" of "+strconv.FormatInt(fact.PaidMinor, 10)+" given back at the provider")
		case payment.OutcomeReversed:
			if err := s.closeAttention(ctx, tx, o.ID, actor, AttentionDispute, "dispute lost: the time this payment gave was taken back"); err != nil {
				return err
			}
			return s.completeRefund(ctx, tx, &o, actor, now, "dispute lost")
		default: // refunded in full
			return s.completeRefund(ctx, tx, &o, actor, now, "refunded at the provider")
		}
	})
	return out, err
}

// DisputeClosed says whether a dispute's status is final and leaves the payment with us (won,
// an inquiry closed, a prevented one). Lost is final too, but takes the payment back.
func DisputeClosed(status string) bool {
	switch status {
	case "won", "warning_closed", "prevented":
		return true
	}
	return false
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// grantYear moves the licence's end by one year. A renewal at the locked price continues from
// the old end, whenever its confirmation arrives: its checkout could only be paid before the
// licence's end (its session ended then), so a payment made in time is always honoured (S2
// review 2, N1). Otherwise the year runs from the later of the confirmation and the current
// end (the first year: the coordinator's confirmed reading; a renewal after the end: from the
// payment). operator is the operator's grant of a payment that could not be granted.
func (s *Service) grantYear(ctx context.Context, tx *sql.Tx, o *Order, actor string, now int64, operator bool) error {
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
	// Nothing is granted into nothing: the payment is recorded, the order goes to the
	// operator, who can grant it later (GrantOrder).
	refuse := func(why string) error {
		if operator {
			return errors.New("membership: cannot grant: " + why)
		}
		if err := s.openAttention(ctx, tx, o.ID, actor, AttentionNotGranted, why); err != nil {
			return err
		}
		return s.moveOrder(ctx, tx, o, OrderMismatch, actor, "a payment arrived; no year could be added: "+why)
	}
	switch {
	case l.Removed.Valid:
		return refuse(NotGrantedRemoved)
	case l.Revoked:
		return refuse(NotGrantedRevoked)
	}
	if !o.Live && !operator {
		test, err := isTestAccount(ctx, tx, o.Member)
		if err != nil {
			return err
		}
		if !test {
			return refuse(NotGrantedUnflagged)
		}
	}
	from := now
	if l.Expires.Valid && (l.Expires.Int64 > from || o.Locked) {
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
	if err := s.moveOrder(ctx, tx, o, OrderPaid, actor, "the payment was confirmed"); err != nil {
		return err
	}
	// The receipt is claimed in the same transaction (one mail): owed until it went.
	result, err := tx.ExecContext(ctx, "UPDATE store_orders SET receipt_owed=? WHERE id=? AND receipt_owed IS NULL", now, o.ID)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	o.ReceiptMine = n == 1
	return nil
}

// Why a payment could not be granted (the attention entry's detail; the member's page says it).
const (
	NotGrantedRemoved   = "the licence was removed"
	NotGrantedRevoked   = "the licence was revoked"
	NotGrantedUnflagged = "a test payment for an account that is no longer a test account"
)

// Attention kinds: what the operator must look at for an order. Each is its own entry with
// its own state; an entry closes by itself when the matter is applied or settled, or by the
// operator's `attend` (S2 review 2, N6).
const (
	AttentionMismatch    = "mismatch"    // the provider's object did not match the order
	AttentionNotGranted  = "not_granted" // a payment arrived; no year could be added (detail: why)
	AttentionPartRefund  = "partial_refund"
	AttentionDispute     = "dispute"     // an open dispute (detail: its status)
	AttentionRefund      = "refund"      // the operator's refund got a final answer
	AttentionUnconfirmed = "unconfirmed" // reconciliation could not confirm it (not "unreachable")
)

// openAttention opens (or refreshes the detail of) an order's entry of this kind.
func (s *Service) openAttention(ctx context.Context, tx *sql.Tx, orderID, actor, kind, detail string) error {
	result, err := tx.ExecContext(ctx, "UPDATE store_attention SET detail=? WHERE order_id=? AND kind=? AND state='open'", detail, orderID, kind)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		if _, err := tx.ExecContext(ctx, "INSERT INTO store_attention(order_id,kind,detail,state,opened) VALUES(?,?,?,'open',?)", orderID, kind, detail, s.Now()); err != nil {
			return err
		}
	}
	return s.audit(ctx, tx, actor, "attention_opened", orderID, kind, detail, "")
}

// closeAttention closes an order's open entry of this kind (nothing when there is none).
func (s *Service) closeAttention(ctx context.Context, tx *sql.Tx, orderID, actor, kind, reason string) error {
	result, err := tx.ExecContext(ctx, "UPDATE store_attention SET state='closed',closed=?,closed_by=?,close_reason=? WHERE order_id=? AND kind=? AND state='open'",
		s.Now(), actor, reason, orderID, kind)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return nil
	}
	return s.audit(ctx, tx, actor, "attention_closed", orderID, kind, "closed", reason)
}

// completeRefund applies a refund or a lost dispute that happened at the provider (there are
// no refunds by policy; the operator's exceptional act or a chargeback still arrives as a
// confirmed fact). It takes back exactly the time THIS payment gave (S2 review 2): later paid
// years stay. If what remains has already ended, the licence is ended: revoked in one statement
// that clears the binding and raises the generation, its end set to now at the latest (so the
// PHP service, which does not read "revoked", also refuses it), with the end it had kept for
// `reinstate`. A payment that was never granted (a mismatch) is settled; nothing to undo.
func (s *Service) completeRefund(ctx context.Context, tx *sql.Tx, o *Order, actor string, now int64, why string) error {
	if o.State == OrderRefunded {
		return nil
	}
	if o.State == OrderMismatch {
		for _, kind := range []string{AttentionMismatch, AttentionNotGranted} {
			if err := s.closeAttention(ctx, tx, o.ID, actor, kind, Settled+why); err != nil {
				return err
			}
		}
		return s.audit(ctx, tx, actor, "settled", o.ID, o.State, why, "the payment went back at the provider; nothing was granted")
	}
	if o.State == OrderPaid {
		var expires int64
		if err := tx.QueryRowContext(ctx, "SELECT COALESCE(expires,0) FROM licenses WHERE id=?", o.LicenseID).Scan(&expires); err != nil {
			return err
		}
		back := expires - (o.GrantedTo - o.GrantedFrom)
		if expires == o.GrantedTo {
			back = o.PreviousExpires // nothing changed since: exactly what it was
		}
		if back > now {
			if _, err := tx.ExecContext(ctx, "UPDATE licenses SET expires=? WHERE id=?", back, o.LicenseID); err != nil {
				return err
			}
			if err := s.audit(ctx, tx, actor, "year_taken_back", o.LicenseID, strconv.FormatInt(expires, 10), strconv.FormatInt(back, 10), "order "+o.ID+": "+why); err != nil {
				return err
			}
		} else if err := s.revoke(ctx, tx, o.LicenseID, actor, expires, now, why, "order "+o.ID); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, "UPDATE store_refunds SET state='done',outcome='refunded',updated=? WHERE order_id=? AND state IN ('requested','sent')", now, o.ID); err != nil {
		return err
	}
	return s.moveOrder(ctx, tx, o, OrderRefunded, actor, why)
}

// Settled starts an attention entry's close reason when the payment went back at the provider.
const Settled = "settled: "

// revoke ends a licence whose remaining time is gone: the binding cleared and the generation
// raised in one statement (the rule), the end it had kept for `reinstate`.
func (s *Service) revoke(ctx context.Context, tx *sql.Tx, licenseID, actor string, expiresBefore, now int64, why, what string) error {
	result, err := tx.ExecContext(ctx, "UPDATE licenses SET revoked=?,revoked_reason='refunded',server_id=NULL,server_ip=NULL,hostname=NULL,expires=MIN(COALESCE(expires,?),?),generation=generation+1 WHERE id=? AND revoked IS NULL",
		now, now, now, licenseID)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return nil
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO store_revocations(license_id,at,expires_before,reason) VALUES(?,?,?,?)", licenseID, now, expiresBefore, why); err != nil {
		return err
	}
	var member int64
	if err := tx.QueryRowContext(ctx, "SELECT member_id FROM licenses WHERE id=?", licenseID).Scan(&member); err != nil {
		return err
	}
	if err := event(ctx, tx, now, member, &licenseID, "revoked_refunded"); err != nil {
		return err
	}
	return s.audit(ctx, tx, actor, "licence_revoked", licenseID, strconv.FormatInt(expiresBefore, 10), "revoked", what+": "+why)
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
		if state == "final" {
			if err := s.openAttention(ctx, tx, r.OrderID, actor, AttentionRefund, "the operator's refund got a final answer: "+outcome); err != nil {
				return err
			}
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

// AttentionEntry is one open matter for the operator.
type AttentionEntry struct {
	Order  Order
	Kind   string
	Detail string
	Opened int64
}

// AttentionOrders lists the open entries (oldest first) and refunds that have been waiting at
// the provider for more than a day. Entries close by themselves when applied or settled, or
// by the operator's Attend (S2 review 2, N6).
func (s *Service) AttentionOrders() ([]AttentionEntry, error) {
	if !s.StoreReady() {
		return nil, ErrStoreNotReady
	}
	ctx := context.Background()
	rows, err := s.db.QueryContext(ctx, "SELECT a.order_id,a.kind,a.detail,a.opened FROM store_attention a WHERE a.state='open' "+
		"UNION ALL SELECT r.order_id,'refund_waiting','a refund has waited at the provider for more than a day',r.updated FROM store_refunds r WHERE r.state IN ('requested','sent') AND r.updated<? "+
		"ORDER BY 4", s.Now()-86400)
	if err != nil {
		return nil, err
	}
	type entry struct {
		order, kind, detail string
		opened              int64
	}
	var found []entry
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.order, &e.kind, &e.detail, &e.opened); err != nil {
			rows.Close()
			return nil, err
		}
		found = append(found, e)
	}
	rows.Close()
	var out []AttentionEntry
	for _, e := range found {
		o, err := s.orderTx(ctx, s.db, e.order)
		if err != nil {
			return nil, err
		}
		o.Attention = e.detail
		out = append(out, AttentionEntry{Order: o, Kind: e.kind, Detail: e.detail, Opened: e.opened})
	}
	return out, nil
}

// Attend closes the operator's open entry of an order (all kinds, or one), with a reason.
func (s *Service) Attend(actor, orderID, kind, reason string) (int, error) {
	if strings.TrimSpace(reason) == "" {
		return 0, ErrInvalidRequest
	}
	ctx := context.Background()
	closed := 0
	err := s.atomic(func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, "SELECT kind FROM store_attention WHERE order_id=? AND state='open' AND (?='' OR kind=?)", orderID, kind, kind)
		if err != nil {
			return err
		}
		var kinds []string
		for rows.Next() {
			var k string
			if err := rows.Scan(&k); err != nil {
				rows.Close()
				return err
			}
			kinds = append(kinds, k)
		}
		rows.Close()
		for _, k := range kinds {
			if err := s.closeAttention(ctx, tx, orderID, actor, k, reason); err != nil {
				return err
			}
			closed++
		}
		return nil
	})
	return closed, err
}

// Cause is what the member's order page says about an order that did not become paid
// normally: the latest mismatch or "not granted" entry and whether it was settled (the payment
// went back at the provider).
func (s *Service) Cause(orderID string) (kind, detail string, settled bool, err error) {
	var reason sql.NullString
	err = s.db.QueryRowContext(context.Background(), "SELECT kind,detail,close_reason FROM store_attention WHERE order_id=? AND kind IN (?,?) ORDER BY id DESC LIMIT 1",
		orderID, AttentionMismatch, AttentionNotGranted).Scan(&kind, &detail, &reason)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", false, nil
	}
	return kind, detail, strings.HasPrefix(reason.String, Settled), err
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

// TrialDays is the first licence's free trial (D-047 point 6; the owner, 23 November 2026:
// "ilk seferde 30 gün trial var").
const TrialDays = 30

// MaxAwaitingPayment is how many licences an account may have waiting for their first year at
// a time.
const MaxAwaitingPayment = 3

// ErrTooManyAwaiting: the account already has MaxAwaitingPayment licences waiting for payment.
const ErrTooManyAwaiting Problem = "too_many_awaiting"

// IssueCharged creates a licence for an account that buys its years (S2 reviews). The first
// licence of an account in this mode is the 30-day trial -- unless the account held a free
// beta licence before, which was its free period. Any other licence waits for its first year:
// its end is its creation time, so neither this service nor the PHP service activates it, and
// the panels' contract is unchanged; its order is made only when the member asks to pay. At most
// MaxAwaitingPayment wait at a time. A second click within ten seconds creates nothing more.
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
		now := s.Now()
		var last sql.NullInt64
		if err := tx.QueryRowContext(ctx, "SELECT MAX(created) FROM licenses WHERE member_id=?", member).Scan(&last); err != nil {
			return err
		}
		if last.Valid && now-last.Int64 < 10 {
			return errDoubleClick // the same click twice: one licence
		}
		used, err := s.trialUsed(ctx, tx, member, mode)
		if err != nil {
			return err
		}
		if !used {
			trial = true
			if out, err = s.insertLicence(ctx, tx, member, now+TrialDays*86400, "trial_started"); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, "INSERT INTO store_trials(member_id,mode,license_id,started,ends) VALUES(?,?,?,?,?)", member, string(mode), out.ID, now, now+TrialDays*86400); err != nil {
				return err
			}
			return s.audit(ctx, tx, memberActor(member), "trial_started", out.ID, "", strconv.FormatInt(now+TrialDays*86400, 10), string(mode))
		}
		var waiting int
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM licenses l WHERE l.member_id=? AND l.removed IS NULL AND COALESCE(l.expires,0)<=l.created "+
			"AND NOT EXISTS (SELECT 1 FROM store_orders o WHERE o.license_id=l.id AND o.state='paid')", member).Scan(&waiting); err != nil {
			return err
		}
		if waiting >= MaxAwaitingPayment {
			return ErrTooManyAwaiting
		}
		if out, err = s.insertLicence(ctx, tx, member, now, "issued_awaiting_payment"); err != nil {
			return err
		}
		return s.audit(ctx, tx, memberActor(member), "licence_awaiting_payment", out.ID, "", "awaiting its first year", "")
	})
	if errors.Is(err, errDoubleClick) {
		return Issued{}, false, nil
	}
	return out, trial, err
}

var errDoubleClick = errors.New("membership: the same click twice")

// trialUsed: the account had its trial in this mode, or held a free beta licence (one that was
// neither a trial nor waiting for payment) -- that was its free period (S2 review 2).
func (s *Service) trialUsed(ctx context.Context, q querier, member int64, mode ChargingMode) (bool, error) {
	var n int
	if err := q.QueryRowContext(ctx, "SELECT COUNT(*) FROM store_trials WHERE member_id=? AND mode=?", member, string(mode)).Scan(&n); err != nil {
		return false, err
	}
	if n > 0 {
		return true, nil
	}
	err := q.QueryRowContext(ctx, "SELECT COUNT(*) FROM licenses l WHERE l.member_id=? AND COALESCE(l.expires,0)>l.created "+
		"AND NOT EXISTS (SELECT 1 FROM store_trials t WHERE t.license_id=l.id) "+
		"AND NOT EXISTS (SELECT 1 FROM store_orders o WHERE o.license_id=l.id AND o.state IN ('paid','refunded'))", member).Scan(&n)
	return n > 0, err
}

// Trial is a member's trial licence in this mode and its end ("" when none); used says whether
// the trial is used (also by a free beta licence).
func (s *Service) Trial(member int64, mode ChargingMode) (string, int64, bool, error) {
	ctx := context.Background()
	used, err := s.trialUsed(ctx, s.db, member, mode)
	if err != nil {
		return "", 0, false, err
	}
	var id string
	var ends int64
	err = s.db.QueryRowContext(ctx, "SELECT license_id,ends FROM store_trials WHERE member_id=? AND mode=?", member, string(mode)).Scan(&id, &ends)
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, used, nil
	}
	return id, ends, used, err
}

// TrialRefused says whether a trial licence was refused on a server that already had a trial.
func (s *Service) TrialRefused(licenseID string) bool {
	var refused sql.NullInt64
	if s.db.QueryRowContext(context.Background(), "SELECT refused FROM store_trials WHERE license_id=?", licenseID).Scan(&refused) != nil {
		return false
	}
	return refused.Valid
}

// trialServer is the one-trial-per-server rule (the coordinator's anti-abuse decision, to be
// confirmed by the owner): a trial licence does not activate on a server that already ran a
// trial with another licence. It answers whether the activation may go on.
func (s *Service) trialServer(ctx context.Context, tx *sql.Tx, licenseID, server string) (bool, error) {
	if !s.hasRevoked {
		return true, nil
	}
	var isTrial int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM store_trials WHERE license_id=?", licenseID).Scan(&isTrial); err != nil {
		return false, err
	}
	if isTrial == 0 {
		return true, nil
	}
	var other string
	err := tx.QueryRowContext(ctx, "SELECT license_id FROM store_trial_servers WHERE server_id=?", server).Scan(&other)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		_, err := tx.ExecContext(ctx, "INSERT INTO store_trial_servers(server_id,license_id,at) VALUES(?,?,?)", server, licenseID, s.Now())
		return err == nil, err
	case err != nil:
		return false, err
	case other == licenseID:
		return true, nil
	}
	return false, nil
}

// errTrialServerUsed: Activate's own refusal for a trial on a server that had a trial; it
// answers the ordinary invalid_license and records the refusal for the member's page.
var errTrialServerUsed = errors.New("membership: this server already ran a trial")

func (s *Service) noteTrialRefused(licenseID string) {
	_, _ = s.db.ExecContext(context.Background(), "UPDATE store_trials SET refused=? WHERE license_id=?", s.Now(), licenseID)
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

// GrantOrder is the operator's grant of a payment that arrived but could not be granted (the
// licence was removed meanwhile, the test flag was removed, a mismatch the operator looked
// into): since there are no refunds, the payment is honoured this way. It needs a recorded
// payment for the order; it refuses a removed or revoked licence and says so; it closes the
// order's attention entries. Audited; the receipt is owed as for any payment.
func (s *Service) GrantOrder(actor, orderID, reason string) (Order, error) {
	if strings.TrimSpace(reason) == "" {
		return Order{}, ErrInvalidRequest
	}
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
		if o.State != OrderMismatch {
			return errors.New("membership: only an order that could not be granted (state mismatch) can be granted; this one is " + o.State)
		}
		var paid int
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM store_payments WHERE order_id=? AND outcome='paid'", o.ID).Scan(&paid); err != nil {
			return err
		}
		if paid == 0 {
			return errors.New("membership: no confirmed payment is recorded for this order")
		}
		if err := s.grantYear(ctx, tx, &o, actor, s.Now(), true); err != nil {
			return err
		}
		for _, kind := range []string{AttentionMismatch, AttentionNotGranted} {
			if err := s.closeAttention(ctx, tx, o.ID, actor, kind, "granted by the operator: "+reason); err != nil {
				return err
			}
		}
		out = o
		return s.audit(ctx, tx, actor, "granted_by_operator", o.ID, OrderMismatch, OrderPaid, reason)
	})
	return out, err
}

// Reinstate undoes a revocation that should not have happened (the operator, with a reason):
// the revocation is cleared, the end it had is restored, and the generation is raised again;
// the server activates its key again to get the licence back. Audited.
func (s *Service) Reinstate(actor, licenseID, reason string) error {
	if strings.TrimSpace(reason) == "" || !hex32.MatchString(licenseID) {
		return ErrInvalidRequest
	}
	if !s.StoreReady() {
		return ErrStoreNotReady
	}
	ctx := context.Background()
	return s.atomic(func(tx *sql.Tx) error {
		var id, before int64
		err := tx.QueryRowContext(ctx, "SELECT id,expires_before FROM store_revocations WHERE license_id=? AND reinstated IS NULL ORDER BY id DESC LIMIT 1", licenseID).Scan(&id, &before)
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("membership: this licence has no revocation to undo")
		}
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE licenses SET revoked=NULL,revoked_reason=NULL,expires=?,generation=generation+1 WHERE id=? AND revoked IS NOT NULL", before, licenseID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE store_revocations SET reinstated=? WHERE id=?", s.Now(), id); err != nil {
			return err
		}
		var member int64
		if err := tx.QueryRowContext(ctx, "SELECT member_id FROM licenses WHERE id=?", licenseID).Scan(&member); err != nil {
			return err
		}
		if err := event(ctx, tx, s.Now(), member, &licenseID, "reinstated"); err != nil {
			return err
		}
		return s.audit(ctx, tx, actor, "licence_reinstated", licenseID, "revoked", strconv.FormatInt(before, 10), reason)
	})
}
