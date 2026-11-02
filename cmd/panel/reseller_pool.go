package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
)

// Reseller pools (alpha.114; real-use rule since alpha.117, owner decision
// K2 in D-042): the quantity part of a reseller's rights set. Each limit is a
// total over the reseller's whole tree — the reseller's own subscriptions and
// every subscription of its customers — plus the number of customers. The
// pool limits REAL use, not what is handed out: a reseller may give its
// customers more than the pool in total, but the tree's real use cannot pass
// it (rights_pool.go refuses the creation). Each limit of 0 is unlimited; a
// reseller without a pool row has no limits at all.
//
// Bayi havuzu, bayinin hak kümesinin sayı kısmıdır. Havuz GERÇEK kullanımı
// sınırlar (K2): dağıtım havuzu aşabilir, gerçek toplam aşamaz.

const (
	errCodeResellerPoolExceeded   = "RESELLER_POOL_EXCEEDED"
	errCodeResellerCustomersLimit = "RESELLER_CUSTOMER_LIMIT"
	// errCodePlanExceedsPool is no longer produced (K2); kept for screens
	// that still name it.
	errCodePlanExceedsPool = "PLAN_EXCEEDS_POOL"
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

func (pool resellerPool) valid() bool {
	return pool.MaxCustomers >= 0 && pool.Domains >= 0 && pool.Databases >= 0 && pool.Mailboxes >= 0 && pool.DiskMB >= 0
}

// poolAllocation is what a reseller's tree has been handed: per kind the sum
// of the limits, and whether any subscription in the tree is unlimited there.
// Shown for information; the pool limits real use (poolUsage).
// poolAllocation, bir bayi ağacına dağıtılmış olandır (bilgi içindir).
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
// leaving out excludeSubID (0 for none).
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

// handleResellerPool serves /api/v1/users/{id}/pool for the administrator:
// GET reads the pool with the tree's real use, PUT sets it. Changing the pool
// of a reseller bound to a reseller plan makes it Custom (P3); giving a pool
// to an Unlimited reseller makes it Custom with everything offered. A pool
// below today's real use is stored with the list of what is over: nothing is
// taken away, and new creations of those kinds are refused.
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
		var shown *resellerPool
		if ok {
			shown = &pool
		}
		p.writePoolAnswer(w, r, targetID, shown)
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
		if body.Pool != nil && !body.Pool.valid() {
			writeClientError(w, http.StatusBadRequest, "pool limits must be 0 (unlimited) or more")
			return
		}
		tx, err := p.db.GetDB().BeginTx(ctx, nil)
		if err != nil {
			writeServerError(w, err)
			return
		}
		defer tx.Rollback()
		converted, err := resellerToCustom(ctx, tx, targetID, body.Pool != nil)
		if err != nil {
			writeServerError(w, err)
			return
		}
		if body.Pool == nil {
			// No pool: the reseller has no quantity limits again.
			if _, err := tx.ExecContext(ctx, `DELETE FROM reseller_pools WHERE reseller_id = ?`, targetID); err != nil {
				writeServerError(w, err)
				return
			}
		} else if err := upsertResellerPool(ctx, tx, targetID, *body.Pool); err != nil {
			writeServerError(w, err)
			return
		}
		if err := tx.Commit(); err != nil {
			writeServerError(w, err)
			return
		}
		if converted != "" {
			p.auditDetailed(r, "reseller.rights_custom:"+targetName, "user", targetID, map[string]any{"from": converted})
		}
		if body.Pool == nil {
			p.auditDetailed(r, "reseller.pool:"+targetName, "user", targetID, map[string]any{"pool": "none"})
		} else {
			pool := *body.Pool
			p.auditDetailed(r, "reseller.pool:"+targetName, "user", targetID, map[string]any{
				"max_customers": pool.MaxCustomers, "max_domains": pool.Domains, "max_databases": pool.Databases,
				"max_email_accounts": pool.Mailboxes, "disk_quota_mb": pool.DiskMB,
			})
		}
		p.writePoolAnswer(w, r, targetID, body.Pool)
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

// writePoolAnswer: the pool (null = no limits), the tree's real use, what is
// handed out (information) and the kinds whose real use is over the pool.
func (p *Panel) writePoolAnswer(w http.ResponseWriter, r *http.Request, resellerID int, pool *resellerPool) {
	db := p.db.GetDB()
	allocation, err := resellerAllocation(r.Context(), db, resellerID, 0)
	if err != nil {
		writeServerError(w, err)
		return
	}
	usage, err := resellerTreeUsage(r.Context(), db, resellerID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	over := []string{}
	if pool != nil {
		over = poolOverUsage(*pool, usage)
	}
	if err := json.NewEncoder(w).Encode(map[string]any{
		"success":    true,
		"pool":       pool,
		"used":       usage,
		"allocated":  allocation,
		"over_limit": over,
	}); err != nil {
		log.Printf("encode reseller pool: %v", err)
	}
}
