package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
)

// Reseller pools (alpha.114, ROADMAP v0.3 "the reseller pool"): what the
// administrator lets one reseller hand out, as totals over the reseller's
// whole tree — the reseller's own subscriptions and every subscription of its
// customers — plus the number of customers. The pool is an allocation limit:
// what counts is the sum of the subscription limits handed out (a plan's
// limits are what the customer may use), not today's usage, so the pool can
// never be oversold. Each limit of 0 is unlimited, the same meaning a plan's
// 0 has; a reseller without a pool row has no limits at all, which keeps
// every reseller created before this release working exactly as before.
//
// Bayi havuzları (alpha.114): yöneticinin bir bayiye dağıtma izni verdiği
// toplamlar. Sayılan, dağıtılmış abonelik sınırlarının toplamıdır (bugünkü
// kullanım değil); böylece havuz asla aşırı satılamaz. Her 0 sınırsızdır;
// havuz satırı olmayan bayinin sınırı yoktur.

const (
	errCodeResellerPoolExceeded   = "RESELLER_POOL_EXCEEDED"
	errCodeResellerCustomersLimit = "RESELLER_CUSTOMER_LIMIT"
	errCodePlanExceedsPool        = "PLAN_EXCEEDS_POOL"
)

// planLimits is the four limits a plan and a subscription carry (0 = unlimited).
// planLimits, bir planın ve aboneliğin taşıdığı dört sınırdır (0 = sınırsız).
type planLimits struct {
	Domains   int `json:"max_domains"`
	Databases int `json:"max_databases"`
	Mailboxes int `json:"max_email_accounts"`
	DiskMB    int `json:"disk_quota_mb"`
}

type resellerPool struct {
	MaxCustomers int `json:"max_customers"`
	planLimits
}

// poolAllocation is what a reseller's tree has been handed: per kind the sum
// of the limits, and whether any subscription in the tree is unlimited there.
// poolAllocation, bir bayi ağacına dağıtılmış olandır.
type poolAllocation struct {
	Customers          int   `json:"customers"`
	Domains            int64 `json:"domains"`
	Databases          int64 `json:"databases"`
	Mailboxes          int64 `json:"mailboxes"`
	DiskMB             int64 `json:"disk_mb"`
	UnlimitedDomains   bool  `json:"unlimited_domains,omitempty"`
	UnlimitedDatabases bool  `json:"unlimited_databases,omitempty"`
	UnlimitedMailboxes bool  `json:"unlimited_mailboxes,omitempty"`
	UnlimitedDisk      bool  `json:"unlimited_disk,omitempty"`
}

func (a *poolAllocation) add(l planLimits) {
	if l.Domains <= 0 {
		a.UnlimitedDomains = true
	} else {
		a.Domains += int64(l.Domains)
	}
	if l.Databases <= 0 {
		a.UnlimitedDatabases = true
	} else {
		a.Databases += int64(l.Databases)
	}
	if l.Mailboxes <= 0 {
		a.UnlimitedMailboxes = true
	} else {
		a.Mailboxes += int64(l.Mailboxes)
	}
	if l.DiskMB <= 0 {
		a.UnlimitedDisk = true
	} else {
		a.DiskMB += int64(l.DiskMB)
	}
}

// poolViolations names every kind where the allocation does not fit the
// pool. An unlimited allocation never fits a limited pool.
// poolViolations, dağıtımın havuza sığmadığı her türü adlandırır.
func poolViolations(pool resellerPool, a poolAllocation) []string {
	out := []string{}
	check := func(name string, limit int, allocated int64, unlimited bool, unit string) {
		if limit <= 0 {
			return
		}
		if unlimited {
			out = append(out, fmt.Sprintf("%s: an unlimited allocation does not fit the pool of %d%s", name, limit, unit))
			return
		}
		if allocated > int64(limit) {
			out = append(out, fmt.Sprintf("%s: %d%s allocated, pool %d%s", name, allocated, unit, limit, unit))
		}
	}
	check("domains", pool.Domains, a.Domains, a.UnlimitedDomains, "")
	check("databases", pool.Databases, a.Databases, a.UnlimitedDatabases, "")
	check("mailboxes", pool.Mailboxes, a.Mailboxes, a.UnlimitedMailboxes, "")
	check("disk", pool.DiskMB, a.DiskMB, a.UnlimitedDisk, " MB")
	if pool.MaxCustomers > 0 && a.Customers > pool.MaxCustomers {
		out = append(out, fmt.Sprintf("customers: %d, pool %d", a.Customers, pool.MaxCustomers))
	}
	return out
}

type rowQueryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// treeResellerID returns the reseller whose pool covers userID's
// subscriptions: the user itself when it is a reseller, its parent when that
// parent is a reseller, else 0 (the administrator's own tree has no pool).
// treeResellerID, userID'nin aboneliklerini kapsayan havuzun bayisini döndürür.
func treeResellerID(ctx context.Context, q rowQueryer, userID int) (int, error) {
	var role string
	var parentID sql.NullInt64
	var parentRole sql.NullString
	err := q.QueryRowContext(ctx, `
		SELECT u.role, u.parent_id, p.role
		FROM users u LEFT JOIN users p ON p.id = u.parent_id
		WHERE u.id = ? AND COALESCE(NULLIF(u.account_type, ''), 'account') = 'account'`, userID).
		Scan(&role, &parentID, &parentRole)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if role == roleReseller {
		return userID, nil
	}
	if parentID.Valid && parentRole.Valid && parentRole.String == roleReseller {
		return int(parentID.Int64), nil
	}
	return 0, nil
}

// loadResellerPool reads a reseller's pool; ok is false when it has none.
func loadResellerPool(ctx context.Context, q rowQueryer, resellerID int) (resellerPool, bool, error) {
	var pool resellerPool
	err := q.QueryRowContext(ctx, `
		SELECT max_customers, max_domains, max_databases, max_email_accounts, disk_quota_mb
		FROM reseller_pools WHERE reseller_id = ?`, resellerID).
		Scan(&pool.MaxCustomers, &pool.Domains, &pool.Databases, &pool.Mailboxes, &pool.DiskMB)
	if errors.Is(err, sql.ErrNoRows) {
		return resellerPool{}, false, nil
	}
	if err != nil {
		return resellerPool{}, false, err
	}
	return pool, true, nil
}

// resellerAllocation sums the limits handed out in a reseller's tree,
// leaving out excludeSubID (the subscription being changed; 0 for none).
// resellerAllocation, bir bayi ağacında dağıtılmış sınırları toplar.
func resellerAllocation(ctx context.Context, q rowQueryer, resellerID, excludeSubID int) (poolAllocation, error) {
	var a poolAllocation
	if err := q.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM users
		WHERE parent_id = ? AND role = 'customer'
		  AND COALESCE(NULLIF(account_type, ''), 'account') = 'account'`, resellerID).Scan(&a.Customers); err != nil {
		return a, err
	}
	rows, err := q.QueryContext(ctx, `
		SELECT s.max_domains, s.max_databases, s.max_email_accounts, s.disk_quota_mb
		FROM subscriptions s
		WHERE s.id <> ?
		  AND (s.owner_id = ? OR s.owner_id IN (
		        SELECT id FROM users
		        WHERE parent_id = ? AND role = 'customer'
		          AND COALESCE(NULLIF(account_type, ''), 'account') = 'account'))`,
		excludeSubID, resellerID, resellerID)
	if err != nil {
		return a, err
	}
	defer rows.Close()
	for rows.Next() {
		var l planLimits
		var domains, databases, mailboxes, disk sql.NullInt64
		if err := rows.Scan(&domains, &databases, &mailboxes, &disk); err != nil {
			return a, err
		}
		l.Domains, l.Databases, l.Mailboxes, l.DiskMB = int(domains.Int64), int(databases.Int64), int(mailboxes.Int64), int(disk.Int64)
		a.add(l)
	}
	return a, rows.Err()
}

// checkResellerPoolFor answers whether giving ownerID one more subscription
// with limits (or changing excludeSubID to them) still fits the pool of the
// reseller whose tree ownerID is in. newCustomer counts one more customer.
// It returns the violations (empty when it fits or there is no pool).
// checkResellerPoolFor, ownerID'ye bu sınırlarla bir abonelik vermenin
// havuza hâlâ sığıp sığmadığını yanıtlar.
func checkResellerPoolFor(ctx context.Context, q rowQueryer, ownerID, excludeSubID int, limits *planLimits, newCustomer bool) (int, []string, error) {
	resellerID, err := treeResellerID(ctx, q, ownerID)
	if err != nil || resellerID == 0 {
		return 0, nil, err
	}
	violations, err := checkResellerPool(ctx, q, resellerID, excludeSubID, limits, newCustomer)
	return resellerID, violations, err
}

// checkResellerPool is checkResellerPoolFor for a known reseller (used before
// the new customer's own row exists).
func checkResellerPool(ctx context.Context, q rowQueryer, resellerID, excludeSubID int, limits *planLimits, newCustomer bool) ([]string, error) {
	pool, ok, err := loadResellerPool(ctx, q, resellerID)
	if err != nil || !ok {
		return nil, err
	}
	before, err := resellerAllocation(ctx, q, resellerID, 0)
	if err != nil {
		return nil, err
	}
	after, err := resellerAllocation(ctx, q, resellerID, excludeSubID)
	if err != nil {
		return nil, err
	}
	if newCustomer {
		after.Customers++
	}
	if limits != nil {
		after.add(*limits)
	}
	return worsenedPoolViolations(pool, before, after, newCustomer), nil
}

// worsenedPoolViolations names the kinds that end over the pool AND that
// this request made larger. A pool the administrator lowered below what is
// already handed out is not this request's fault, so a change that keeps or
// shrinks a kind is never refused for it.
// worsenedPoolViolations, havuzun üstünde biten VE bu isteğin büyüttüğü
// türleri adlandırır. Küçülten ya da aynı bırakan değişiklik reddedilmez.
func worsenedPoolViolations(pool resellerPool, before, after poolAllocation, newCustomer bool) []string {
	out := []string{}
	check := func(name, unit string, limit int, old, now int64, oldUnlimited, nowUnlimited bool) {
		if limit <= 0 {
			return
		}
		over := nowUnlimited || now > int64(limit)
		worse := (nowUnlimited && !oldUnlimited) || (!nowUnlimited && !oldUnlimited && now > old)
		if !over || !worse {
			return
		}
		if nowUnlimited {
			out = append(out, fmt.Sprintf("%s: an unlimited allocation does not fit the pool of %d%s", name, limit, unit))
			return
		}
		out = append(out, fmt.Sprintf("%s: %d%s would be allocated, pool %d%s", name, now, unit, limit, unit))
	}
	check("domains", "", pool.Domains, before.Domains, after.Domains, before.UnlimitedDomains, after.UnlimitedDomains)
	check("databases", "", pool.Databases, before.Databases, after.Databases, before.UnlimitedDatabases, after.UnlimitedDatabases)
	check("mailboxes", "", pool.Mailboxes, before.Mailboxes, after.Mailboxes, before.UnlimitedMailboxes, after.UnlimitedMailboxes)
	check("disk", " MB", pool.DiskMB, before.DiskMB, after.DiskMB, before.UnlimitedDisk, after.UnlimitedDisk)
	if newCustomer && pool.MaxCustomers > 0 && after.Customers > pool.MaxCustomers {
		out = append(out, fmt.Sprintf("customers: %d would exist, pool %d", after.Customers, pool.MaxCustomers))
	}
	return out
}

// limitsWithinPool reports whether one plan's limits fit inside a pool on
// their own (a reseller's own plan may never promise more than its pool).
// limitsWithinPool, tek bir planın sınırlarının havuza tek başına sığıp
// sığmadığını bildirir.
func limitsWithinPool(pool resellerPool, l planLimits) bool {
	fits := func(poolLimit, planLimit int) bool {
		return poolLimit <= 0 || (planLimit > 0 && planLimit <= poolLimit)
	}
	return fits(pool.Domains, l.Domains) && fits(pool.Databases, l.Databases) &&
		fits(pool.Mailboxes, l.Mailboxes) && fits(pool.DiskMB, l.DiskMB)
}

// writePoolRefusal is the coded 409 for a request that would exceed a
// reseller's pool. Who acts: the reseller lowers another customer's plan, or
// asks the administrator for a larger pool.
// writePoolRefusal, bir bayinin havuzunu aşacak istek için kodlu 409'dur.
func writePoolRefusal(w http.ResponseWriter, violations []string) {
	writeCodedErrorDetails(w, http.StatusConflict, errCodeResellerPoolExceeded,
		"this would exceed the reseller's pool; move a customer to a smaller plan, or ask the administrator for a larger pool. Nothing was changed.",
		"", violations)
}

// handleResellerPool serves /api/v1/users/{id}/pool for the administrator:
// GET reads the pool with what is handed out, PUT sets it. A pool set below
// what is already handed out is stored with warnings: nothing is taken
// away, and new allocations in that tree are refused until it fits again.
// handleResellerPool, yönetici için /api/v1/users/{id}/pool'a hizmet eder.
func (p *Panel) handleResellerPool(w http.ResponseWriter, r *http.Request, c *Caller, targetID int, targetRole, targetName string) {
	if !c.hasAccountRole(roleAdmin) || c.ImpersonatorID > 0 {
		writeCodedError(w, http.StatusForbidden, errCodeAdminOnly, "only the administrator sets a reseller's pool", "")
		return
	}
	if targetRole != roleReseller {
		writeClientError(w, http.StatusBadRequest, "only a reseller has a pool")
		return
	}
	ctx := r.Context()
	switch r.Method {
	case http.MethodGet:
		pool, ok, err := loadResellerPool(ctx, p.db.GetDB(), targetID)
		if err != nil {
			writeServerError(w, err)
			return
		}
		allocation, err := resellerAllocation(ctx, p.db.GetDB(), targetID, 0)
		if err != nil {
			writeServerError(w, err)
			return
		}
		var shown *resellerPool
		if ok {
			shown = &pool
		}
		writePoolAnswer(w, shown, allocation)
	case http.MethodPut:
		var body struct {
			Pool *resellerPool `json:"pool"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil {
			writeClientError(w, http.StatusBadRequest, "invalid pool")
			return
		}
		if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
			writeClientError(w, http.StatusBadRequest, "invalid pool")
			return
		}
		if body.Pool == nil {
			// No pool: the reseller has no limits again.
			if _, err := p.db.GetDB().ExecContext(ctx, `DELETE FROM reseller_pools WHERE reseller_id = ?`, targetID); err != nil {
				writeServerError(w, err)
				return
			}
			p.auditDetailed(r, "reseller.pool:"+targetName, "user", targetID, map[string]any{"pool": "none"})
			allocation, err := resellerAllocation(ctx, p.db.GetDB(), targetID, 0)
			if err != nil {
				writeServerError(w, err)
				return
			}
			writePoolAnswer(w, nil, allocation)
			return
		}
		pool := *body.Pool
		if pool.MaxCustomers < 0 || pool.Domains < 0 || pool.Databases < 0 || pool.Mailboxes < 0 || pool.DiskMB < 0 {
			writeClientError(w, http.StatusBadRequest, "pool limits must be 0 (unlimited) or more")
			return
		}
		if err := upsertResellerPool(ctx, p.db.GetDB(), targetID, pool); err != nil {
			writeServerError(w, err)
			return
		}
		allocation, err := resellerAllocation(ctx, p.db.GetDB(), targetID, 0)
		if err != nil {
			writeServerError(w, err)
			return
		}
		p.auditDetailed(r, "reseller.pool:"+targetName, "user", targetID, map[string]any{
			"max_customers": pool.MaxCustomers, "max_domains": pool.Domains, "max_databases": pool.Databases,
			"max_email_accounts": pool.Mailboxes, "disk_quota_mb": pool.DiskMB,
		})
		writePoolAnswer(w, &pool, allocation)
	default:
		rejectRouteMethod(w, []string{http.MethodGet, http.MethodPut})
	}
}

type sqlExecer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func upsertResellerPool(ctx context.Context, db sqlExecer, resellerID int, pool resellerPool) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO reseller_pools (reseller_id, max_customers, max_domains, max_databases, max_email_accounts, disk_quota_mb)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(reseller_id) DO UPDATE SET
		  max_customers = excluded.max_customers, max_domains = excluded.max_domains,
		  max_databases = excluded.max_databases, max_email_accounts = excluded.max_email_accounts,
		  disk_quota_mb = excluded.disk_quota_mb, updated_at = datetime('now')`,
		resellerID, pool.MaxCustomers, pool.Domains, pool.Databases, pool.Mailboxes, pool.DiskMB)
	return err
}

// writePoolAnswer: the pool (null = no limits), what is handed out, and the
// kinds already over the pool (stored anyway; new allocations are refused).
func writePoolAnswer(w http.ResponseWriter, pool *resellerPool, allocation poolAllocation) {
	over := []string{}
	if pool != nil {
		over = poolViolations(*pool, allocation)
	}
	if err := json.NewEncoder(w).Encode(map[string]any{
		"success":    true,
		"pool":       pool,
		"allocated":  allocation,
		"over_limit": over,
	}); err != nil {
		log.Printf("encode reseller pool: %v", err)
	}
}
