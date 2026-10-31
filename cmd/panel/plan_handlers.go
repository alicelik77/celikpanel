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
	"strconv"
	"strings"
)

// Service plans: reusable quota templates (Plesk "Service Plans"). The
// administrator's plans are global (owner_id NULL) and every reseller may use
// them; a reseller's own plans (owner_id = the reseller, alpha.114) are seen
// and used only by that reseller and the administrator, and never promise
// more than the reseller's pool. Every limit of 0 is unlimited.
//
// Bandwidth: bandwidth_quota_mb is stored and accepted for compatibility, but
// traffic is not measured yet, so nothing enforces it and the screens say so
// instead of showing a limit (ROADMAP v0.35 "enforce or delete").
//
// Servis planları: yeniden kullanılabilir kota şablonları. Yöneticinin
// planları geneldir; bayinin kendi planlarını (alpha.114) yalnız o bayi ve
// yönetici görür ve kullanır. Her 0 sınırı sınırsızdır. Trafik henüz
// ölçülmediği için bant genişliği sınırı uygulanmaz; ekranlar bunu söyler.

type planPayload struct {
	ID               int    `json:"id,omitempty"`
	Name             string `json:"name"`
	MaxDomains       int    `json:"max_domains"`
	MaxDatabases     int    `json:"max_databases"`
	MaxEmailAccounts int    `json:"max_email_accounts"`
	DiskQuotaMB      int    `json:"disk_quota_mb"`
	BandwidthQuotaMB int    `json:"bandwidth_quota_mb"`
	Subscribers      int    `json:"subscribers,omitempty"`
	// Read-only facts of a listed plan; a client may send them back and they
	// are ignored (ownership never comes from a request body).
	// Listelenen planın salt-okur bilgileri; istemci geri gönderebilir,
	// yok sayılır (sahiplik asla istek gövdesinden gelmez).
	OwnerID *int   `json:"owner_id,omitempty"`
	Owner   string `json:"owner,omitempty"`
	Mine    bool   `json:"mine,omitempty"`
}

func (pl planPayload) limits() planLimits {
	return planLimits{Domains: pl.MaxDomains, Databases: pl.MaxDatabases, Mailboxes: pl.MaxEmailAccounts, DiskMB: pl.DiskQuotaMB}
}

const (
	maxPlanRequestBytes = 32 << 10
	maxPlanNameLength   = 100
)

// planManager admits the administrator and resellers acting as themselves
// or through "Log in as" (a reseller impersonated by the administrator acts
// in the reseller's scope).
func (p *Panel) planManager(w http.ResponseWriter, r *http.Request) *Caller {
	c := currentCaller(r)
	if c == nil || (!c.hasAccountRole(roleAdmin) && !c.hasAccountRole(roleReseller)) {
		writeClientError(w, http.StatusForbidden, "administrator or reseller access required")
		return nil
	}
	return c
}

func (p *Panel) handlePlans(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	c := p.planManager(w, r)
	if c == nil {
		return
	}

	switch r.Method {
	case http.MethodGet:
		query := `
			SELECT sp.id, sp.name, sp.max_domains, sp.max_databases, sp.max_email_accounts,
			       sp.disk_quota_mb, sp.bandwidth_quota_mb, sp.owner_id, COALESCE(u.username, ''),
			       (SELECT COUNT(*) FROM subscriptions s WHERE s.plan_id = sp.id)
			FROM service_plans sp LEFT JOIN users u ON u.id = sp.owner_id`
		args := []any{}
		if !c.hasAccountRole(roleAdmin) {
			query += ` WHERE sp.owner_id IS NULL OR sp.owner_id = ?`
			args = append(args, c.ID)
		}
		query += ` ORDER BY sp.owner_id IS NOT NULL, sp.name`
		rows, err := p.db.GetDB().QueryContext(r.Context(), query, args...)
		if err != nil {
			writeServerError(w, err)
			return
		}
		defer rows.Close()

		plans := make([]planPayload, 0)
		for rows.Next() {
			var pl planPayload
			var owner sql.NullInt64
			if err := rows.Scan(&pl.ID, &pl.Name, &pl.MaxDomains, &pl.MaxDatabases,
				&pl.MaxEmailAccounts, &pl.DiskQuotaMB, &pl.BandwidthQuotaMB, &owner, &pl.Owner, &pl.Subscribers); err != nil {
				writeServerError(w, fmt.Errorf("scan service plan: %w", err))
				return
			}
			if owner.Valid {
				id := int(owner.Int64)
				pl.OwnerID = &id
				pl.Mine = id == c.ID
			} else {
				pl.Mine = c.hasAccountRole(roleAdmin)
			}
			plans = append(plans, pl)
		}
		if err := rows.Err(); err != nil {
			writeServerError(w, fmt.Errorf("read service plans: %w", err))
			return
		}
		if err := json.NewEncoder(w).Encode(map[string]any{"plans": plans}); err != nil {
			log.Printf("encode service plans: %v", err)
		}

	case http.MethodPost:
		pl, ok := decodePlan(w, r)
		if !ok {
			return
		}
		var owner any
		if c.hasAccountRole(roleReseller) {
			// A reseller's own plan never promises more than its pool.
			// Bayinin kendi planı asla havuzundan fazlasını vaat etmez.
			pool, hasPool, err := loadResellerPool(r.Context(), p.db.GetDB(), c.ID)
			if err != nil {
				writeServerError(w, err)
				return
			}
			if hasPool && !limitsWithinPool(pool, pl.limits()) {
				writePlanExceedsPool(w, pool)
				return
			}
			owner = c.ID
		}
		res, err := p.db.GetDB().ExecContext(r.Context(), `
			INSERT INTO service_plans (owner_id, name, max_domains, max_databases, max_email_accounts, disk_quota_mb, bandwidth_quota_mb)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			owner, pl.Name, pl.MaxDomains, pl.MaxDatabases, pl.MaxEmailAccounts, pl.DiskQuotaMB, pl.BandwidthQuotaMB)
		if err != nil {
			writeServerError(w, err)
			return
		}
		id, err := res.LastInsertId()
		if err != nil {
			writeServerError(w, fmt.Errorf("read inserted service plan identity: %w", err))
			return
		}
		p.auditDetailed(r, "plan.create:"+pl.Name, "service_plan", int(id), planAuditDetails(pl))
		if err := json.NewEncoder(w).Encode(map[string]any{"success": true, "id": id}); err != nil {
			log.Printf("encode service plan creation: %v", err)
		}

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func planAuditDetails(pl planPayload) map[string]any {
	return map[string]any{
		"max_domains": pl.MaxDomains, "max_databases": pl.MaxDatabases,
		"max_email_accounts": pl.MaxEmailAccounts, "disk_quota_mb": pl.DiskQuotaMB,
	}
}

func writePlanExceedsPool(w http.ResponseWriter, pool resellerPool) {
	writeCodedErrorDetails(w, http.StatusConflict, errCodePlanExceedsPool,
		"a plan may not promise more than your pool; lower the limits (0 = unlimited is not possible where your pool is limited), or ask the administrator for a larger pool. Nothing was changed.",
		"", []string{fmt.Sprintf("pool: domains %d, databases %d, mailboxes %d, disk %d MB (0 = unlimited)",
			pool.Domains, pool.Databases, pool.Mailboxes, pool.DiskMB)})
}

// loadPlanOwner reads a plan's owner (0 = global); errNotFound when missing.
func loadPlanOwner(ctx context.Context, q rowQueryer, planID int) (int, error) {
	var owner sql.NullInt64
	err := q.QueryRowContext(ctx, `SELECT owner_id FROM service_plans WHERE id = ?`, planID).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, errNotFound
	}
	if err != nil {
		return 0, err
	}
	return int(owner.Int64), nil
}

func (p *Panel) handlePlanByID(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	c := p.planManager(w, r)
	if c == nil {
		return
	}

	idStr := strings.TrimPrefix(r.URL.Path, "/api/v1/plans/")
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		writeClientError(w, http.StatusBadRequest, "invalid plan id")
		return
	}
	// The administrator edits every plan; a reseller only its own. A plan the
	// caller may not edit is "not found" (no probing by id).
	// Yönetici her planı, bayi yalnız kendininkini düzenler.
	ownerID, err := loadPlanOwner(r.Context(), p.db.GetDB(), id)
	if errors.Is(err, errNotFound) || (err == nil && !c.hasAccountRole(roleAdmin) && ownerID != c.ID) {
		writeClientError(w, http.StatusNotFound, "service plan not found")
		return
	}
	if err != nil {
		writeServerError(w, err)
		return
	}

	switch r.Method {
	case http.MethodPut:
		pl, ok := decodePlan(w, r)
		if !ok {
			return
		}
		if ownerID != 0 {
			pool, hasPool, err := loadResellerPool(r.Context(), p.db.GetDB(), ownerID)
			if err != nil {
				writeServerError(w, err)
				return
			}
			if hasPool && !limitsWithinPool(pool, pl.limits()) {
				writePlanExceedsPool(w, pool)
				return
			}
		}
		// Plan edits also refresh subscriptions born from the plan — that is
		// the point of a template (Plesk: "apply changes to subscribers").
		// Plan düzenlemeleri, plandan doğan abonelikleri de tazeler.
		tx, err := p.db.GetDB().BeginTx(r.Context(), nil)
		if err != nil {
			writeServerError(w, fmt.Errorf("begin service plan update: %w", err))
			return
		}
		defer tx.Rollback()
		// The resellers whose trees hold this plan, with what they had
		// handed out before the change.
		// Bu planı ağacında tutan bayiler ve değişiklikten önceki dağıtımları.
		affected, err := resellersUsingPlan(r.Context(), tx, id)
		if err != nil {
			writeServerError(w, err)
			return
		}
		before := map[int]poolAllocation{}
		for _, resellerID := range affected {
			allocation, err := resellerAllocation(r.Context(), tx, resellerID, 0)
			if err != nil {
				writeServerError(w, err)
				return
			}
			before[resellerID] = allocation
		}
		result, err := tx.ExecContext(r.Context(), `
			UPDATE service_plans SET name = ?, max_domains = ?, max_databases = ?,
			       max_email_accounts = ?, disk_quota_mb = ?, bandwidth_quota_mb = ?,
			       updated_at = datetime('now')
			WHERE id = ?`,
			pl.Name, pl.MaxDomains, pl.MaxDatabases, pl.MaxEmailAccounts,
			pl.DiskQuotaMB, pl.BandwidthQuotaMB, id)
		if err != nil {
			writeServerError(w, fmt.Errorf("update service plan: %w", err))
			return
		}
		if n, err := result.RowsAffected(); err != nil || n != 1 {
			writeServerError(w, fmt.Errorf("update service plan: affected %d, %v", n, err))
			return
		}
		if _, err := tx.ExecContext(r.Context(), `
			UPDATE subscriptions SET max_domains = ?, max_databases = ?,
			       max_email_accounts = ?, disk_quota_mb = ?, bandwidth_quota_mb = ?,
			       updated_at = datetime('now')
			WHERE plan_id = ?`,
			pl.MaxDomains, pl.MaxDatabases, pl.MaxEmailAccounts,
			pl.DiskQuotaMB, pl.BandwidthQuotaMB, id); err != nil {
			writeServerError(w, fmt.Errorf("apply service plan to subscriptions: %w", err))
			return
		}
		violations := []string{}
		for _, resellerID := range affected {
			pool, hasPool, err := loadResellerPool(r.Context(), tx, resellerID)
			if err != nil {
				writeServerError(w, err)
				return
			}
			if !hasPool {
				continue
			}
			after, err := resellerAllocation(r.Context(), tx, resellerID, 0)
			if err != nil {
				writeServerError(w, err)
				return
			}
			for _, v := range worsenedPoolViolations(pool, before[resellerID], after, false) {
				violations = append(violations, fmt.Sprintf("reseller #%d %s", resellerID, v))
			}
		}
		if len(violations) > 0 {
			writePoolRefusal(w, violations)
			return
		}
		if err := tx.Commit(); err != nil {
			writeServerError(w, fmt.Errorf("commit service plan update: %w", err))
			return
		}
		p.auditDetailed(r, "plan.update:"+pl.Name, "service_plan", id, planAuditDetails(pl))
		if err := json.NewEncoder(w).Encode(map[string]bool{"success": true}); err != nil {
			log.Printf("encode service plan update: %v", err)
		}

	case http.MethodDelete:
		result, err := p.db.GetDB().ExecContext(r.Context(), `
			DELETE FROM service_plans
			WHERE id = ?
			  AND NOT EXISTS (SELECT 1 FROM subscriptions WHERE plan_id = service_plans.id)`, id)
		if err != nil {
			writeServerError(w, fmt.Errorf("delete service plan: %w", err))
			return
		}
		affected, err := result.RowsAffected()
		if err != nil {
			writeServerError(w, fmt.Errorf("verify service plan deletion: %w", err))
			return
		}
		if affected == 0 {
			writeClientError(w, http.StatusConflict, "plan has subscriptions; move them to another plan first")
			return
		}
		p.audit(r, "plan.delete:"+strconv.Itoa(id), "service_plan", id)
		if err := json.NewEncoder(w).Encode(map[string]bool{"success": true}); err != nil {
			log.Printf("encode service plan deletion: %v", err)
		}

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// resellersUsingPlan lists the resellers with a subscription on the plan in
// their tree (their own or a customer's).
func resellersUsingPlan(ctx context.Context, q rowQueryer, planID int) ([]int, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT DISTINCT CASE WHEN o.role = 'reseller' THEN o.id ELSE p.id END
		FROM subscriptions s
		JOIN users o ON o.id = s.owner_id
		LEFT JOIN users p ON p.id = o.parent_id
		WHERE s.plan_id = ?
		  AND (o.role = 'reseller' OR p.role = 'reseller')`, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []int{}
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func decodePlan(w http.ResponseWriter, r *http.Request) (planPayload, bool) {
	var pl planPayload
	r.Body = http.MaxBytesReader(w, r.Body, maxPlanRequestBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&pl); err != nil {
		writeClientError(w, http.StatusBadRequest, "invalid service plan")
		return pl, false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeClientError(w, http.StatusBadRequest, "invalid service plan")
		return pl, false
	}
	pl.Name = strings.TrimSpace(pl.Name)
	if pl.Name == "" {
		writeClientError(w, http.StatusBadRequest, "plan name is required")
		return pl, false
	}
	if len(pl.Name) > maxPlanNameLength {
		writeClientError(w, http.StatusBadRequest, "plan name is too long (100 characters at most)")
		return pl, false
	}
	// 0 = unlimited for every limit (quota.go); negative numbers mean nothing.
	// Her sınır için 0 = sınırsız; negatif sayının anlamı yok.
	if pl.MaxDomains < 0 || pl.MaxDatabases < 0 || pl.MaxEmailAccounts < 0 ||
		pl.DiskQuotaMB < 0 || pl.BandwidthQuotaMB < 0 {
		writeClientError(w, http.StatusBadRequest, "limits must be 0 (unlimited) or a positive number")
		return pl, false
	}
	return pl, true
}
