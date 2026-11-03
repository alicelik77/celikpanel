package main

import (
	"context"
	"errors"
	"fmt"
	"log"
)

// The reseller pool limits REAL use (owner decision K2, D-042): a reseller
// may hand its customers more than its pool in total, but the real total of
// its whole tree (its own subscriptions and every customer's) cannot pass
// the pool. When it is reached, no subscription of that tree can create more
// of that kind; at 80% and at 100% a notice is stored for the reseller and
// the administrator (rights_notices). Traffic (alpha.118, D-044) is pooled by
// the month's real total; the usage worker applies it (traffic_collect.go).
//
// Bayi havuzu GERÇEK kullanımı sınırlar: dağıtım havuzu aşabilir, ağacın
// gerçek toplamı aşamaz. %80 ve %100'de bildirim saklanır.

const (
	noticeUnlimitedAfterMigration = "unlimited_after_migration"
	noticeResellerPool80          = "reseller_pool_80"
	noticeResellerPool100         = "reseller_pool_100"
)

// poolUsage is what a reseller's tree really uses now.
type poolUsage struct {
	Customers     int64 `json:"customers"`
	Domains       int64 `json:"domains"`
	Databases     int64 `json:"databases"`
	Mailboxes     int64 `json:"mailboxes"`
	DiskUsedBytes int64 `json:"disk_used_bytes"`
	// TrafficMonthBytes: the whole tree's website traffic this month.
	TrafficMonthBytes int64 `json:"traffic_month_bytes"`
}

const treeSubscriptionsSQL = `
	SELECT s.id FROM subscriptions s
	WHERE s.owner_id = ? OR s.owner_id IN (
	    SELECT id FROM users
	    WHERE parent_id = ? AND role = 'customer'
	      AND COALESCE(NULLIF(account_type, ''), 'account') = 'account')`

// resellerTreeUsage counts the real use of a reseller's tree.
func resellerTreeUsage(ctx context.Context, q rowQueryer, resellerID int) (poolUsage, error) {
	var u poolUsage
	err := q.QueryRowContext(ctx, `
		SELECT
		  (SELECT COUNT(*) FROM users WHERE parent_id = ? AND role = 'customer'
		     AND COALESCE(NULLIF(account_type, ''), 'account') = 'account'),
		  (SELECT COUNT(*) FROM domains d WHERE d.subscription_id IN (`+treeSubscriptionsSQL+`)),
		  (SELECT COUNT(*) FROM databases_v2 db WHERE db.subscription_id IN (`+treeSubscriptionsSQL+`)),
		  (SELECT COUNT(*) FROM email_accounts ea JOIN domains d ON ea.domain_id = d.id
		     WHERE d.subscription_id IN (`+treeSubscriptionsSQL+`)),
		  COALESCE((SELECT SUM(`+subscriptionDiskBytesSQL+`) FROM subscriptions s
		     WHERE s.id IN (`+treeSubscriptionsSQL+`)), 0)`,
		resellerID,
		resellerID, resellerID,
		resellerID, resellerID,
		resellerID, resellerID,
		resellerID, resellerID,
	).Scan(&u.Customers, &u.Domains, &u.Databases, &u.Mailboxes, &u.DiskUsedBytes)
	if err != nil {
		return poolUsage{}, fmt.Errorf("read reseller tree usage: %w", err)
	}
	traffic, err := treeTrafficMonth(ctx, q, resellerID, usageNow())
	if err != nil {
		return poolUsage{}, fmt.Errorf("read reseller tree traffic: %w", err)
	}
	u.TrafficMonthBytes = traffic
	return u, nil
}

// resellerPoolExceededError: the tree's real use already fills the pool.
type resellerPoolExceededError struct {
	Kind  quotaKind
	Used  int64
	Limit int64
}

func (e *resellerPoolExceededError) Error() string {
	unit := ""
	if e.Kind == quotaDisk {
		unit = " MB"
	}
	return fmt.Sprintf("Nothing was created: your provider's total allowance for %s is used up (%d%s of %d%s in use across all of its accounts). "+
		"Remove one you no longer need, or ask your provider; a reseller asks the administrator for a larger pool.",
		e.Kind, e.Used, unit, e.Limit, unit)
}

// poolLimitFor: the pool limit and real use of one kind (MB for disk).
func poolLimitFor(pool resellerPool, usage poolUsage, kind quotaKind) (limit, used int64) {
	switch kind {
	case quotaDomains:
		return int64(pool.Domains), usage.Domains
	case quotaDBs:
		return int64(pool.Databases), usage.Databases
	case quotaMail:
		return int64(pool.Mailboxes), usage.Mailboxes
	case quotaDisk:
		return int64(pool.DiskMB), usage.DiskUsedBytes / (1024 * 1024)
	}
	return 0, 0
}

// checkResellerPoolUsage refuses one more resource of kind in a subscription
// whose reseller tree already uses its whole pool, and stores the 80%/100%
// notices. nil when the subscription has no reseller or no pool.
func (p *Panel) checkResellerPoolUsage(ctx context.Context, subID int, kind quotaKind) error {
	db := p.db.GetDB()
	_, resellerID, err := subscriptionOwnerChain(ctx, db, subID)
	if errors.Is(err, errRightsSubscriptionMissing) {
		return errQuotaSubscriptionMissing
	}
	if err != nil || resellerID == 0 {
		return err
	}
	pool, ok, err := loadResellerPool(ctx, db, resellerID)
	if err != nil || !ok {
		return err
	}
	usage, err := resellerTreeUsage(ctx, db, resellerID)
	if err != nil {
		return err
	}
	limit, used := poolLimitFor(pool, usage, kind)
	if limit <= 0 {
		return nil
	}
	if kind == quotaDisk {
		if usage.DiskUsedBytes >= limit*1024*1024 {
			p.storePoolNotice(ctx, resellerID, kind, used, limit, true)
			return &resellerPoolExceededError{Kind: kind, Used: used, Limit: limit}
		}
		p.notePoolLevel(ctx, resellerID, kind, used, limit)
		return nil
	}
	if used >= limit {
		p.storePoolNotice(ctx, resellerID, kind, used, limit, true)
		return &resellerPoolExceededError{Kind: kind, Used: used, Limit: limit}
	}
	// This creation makes it used+1.
	p.notePoolLevel(ctx, resellerID, kind, used+1, limit)
	return nil
}

// checkResellerCustomerCount: one more customer must fit max_customers.
func (p *Panel) checkResellerCustomerCount(ctx context.Context, resellerID int) error {
	db := p.db.GetDB()
	pool, ok, err := loadResellerPool(ctx, db, resellerID)
	if err != nil || !ok || pool.MaxCustomers <= 0 {
		return err
	}
	usage, err := resellerTreeUsage(ctx, db, resellerID)
	if err != nil {
		return err
	}
	if usage.Customers >= int64(pool.MaxCustomers) {
		p.storePoolNotice(ctx, resellerID, "customers", usage.Customers, int64(pool.MaxCustomers), true)
		return &resellerPoolExceededError{Kind: "customers", Used: usage.Customers, Limit: int64(pool.MaxCustomers)}
	}
	p.notePoolLevel(ctx, resellerID, "customers", usage.Customers+1, int64(pool.MaxCustomers))
	return nil
}

func (p *Panel) notePoolLevel(ctx context.Context, resellerID int, kind quotaKind, used, limit int64) {
	if limit <= 0 {
		return
	}
	switch {
	case used >= limit:
		p.storePoolNotice(ctx, resellerID, kind, used, limit, true)
	case used*10 >= limit*8:
		p.storePoolNotice(ctx, resellerID, kind, used, limit, false)
	}
}

// storePoolNotice keeps one open notice per reseller, level and kind. A
// failure to store it never blocks the creation (it is logged).
func (p *Panel) storePoolNotice(ctx context.Context, resellerID int, kind quotaKind, used, limit int64, full bool) {
	notice := noticeResellerPool80
	if full {
		notice = noticeResellerPool100
	}
	detail := fmt.Sprintf("%d of %d", used, limit)
	if kind == quotaDisk {
		detail += " MB"
	}
	if _, err := p.db.GetDB().ExecContext(ctx, `
		INSERT INTO rights_notices (kind, user_id, subject, detail) VALUES (?, ?, ?, ?)
		ON CONFLICT DO NOTHING`, notice, resellerID, string(kind), detail); err != nil {
		log.Printf("store reseller pool notice: %v", err)
	}
}

// poolOverUsage lists the kinds whose real use is above the pool (what a
// lowered pool leaves frozen; nothing is taken away).
func poolOverUsage(pool resellerPool, usage poolUsage) []string {
	out := []string{}
	check := func(name string, limit int, used int64, unit string) {
		if limit > 0 && used > int64(limit) {
			out = append(out, fmt.Sprintf("%s: %d%s in use, pool %d%s", name, used, unit, limit, unit))
		}
	}
	check("domains", pool.Domains, usage.Domains, "")
	check("databases", pool.Databases, usage.Databases, "")
	check("mailboxes", pool.Mailboxes, usage.Mailboxes, "")
	check("disk", pool.DiskMB, usage.DiskUsedBytes/(1024*1024), " MB")
	check("traffic", pool.TrafficMB, usage.TrafficMonthBytes/mib, " MB")
	check("customers", pool.MaxCustomers, usage.Customers, "")
	return out
}
