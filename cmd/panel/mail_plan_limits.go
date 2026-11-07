package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Plan mail settings (alpha.128, gap G12): the largest mailbox a plan allows
// and the plan's outgoing values (mailbox ceiling, domain and hosting account
// totals). They live in mail_plan_limits (migration 067) and plug into the
// generic ceilings model (setting_ceilings, migration 066) as the keys
// mail.mailbox_size_mb and mail.outgoing.{mailbox,domain,subscription} when
// the integrator joins them.
//
//	GET /api/v1/mail/plan-limits                             administrator: every plan
//	GET /api/v1/mail/plan-limits?owner_kind=plan&owner_id=N   administrator
//	PUT /api/v1/mail/plan-limits                             administrator
//
// A reseller or customer stays within the mailbox size of the plan; the
// administrator may go above it (the server owner's direct action).
//
// Planların posta ayarları: planın izin verdiği en büyük posta kutusu ve
// giden posta değerleri. Bayi ve müşteri plan sınırında kalır; yönetici
// aşabilir.

const (
	mailPlanLimitsPath = "/api/v1/mail/plan-limits"

	errCodeMailboxSizeAbovePlan = "MAILBOX_SIZE_ABOVE_PLAN"
)

type mailPlanLimitsRequest struct {
	OwnerKind string `json:"owner_kind"`
	OwnerID   int    `json:"owner_id"`
	mailPlanLimitRow
}

func validPlanLimitValue(v *int, lo, hi int) bool {
	return v == nil || (*v >= lo && *v <= hi)
}

func (p *Panel) handleMailPlanLimits(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	ctx := r.Context()
	db := p.db.GetDB()
	switch r.Method {
	case http.MethodGet:
		if r.URL.RawQuery == "" {
			// Every customer plan with its mail settings.
			view, err := p.outgoingAdminPlans(ctx)
			if err != nil {
				writeServerError(w, err)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"plans": view})
			return
		}
		kind := r.URL.Query().Get("owner_kind")
		id, err := strconv.Atoi(r.URL.Query().Get("owner_id"))
		if (kind != "plan" && kind != "subscription") || err != nil || id <= 0 {
			writeClientError(w, http.StatusBadRequest, "owner_kind must be plan or subscription and owner_id a positive number")
			return
		}
		limits, err := loadMailPlanLimits(ctx, db)
		if err != nil {
			writeServerError(w, err)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"owner_kind": kind, "owner_id": id, "limits": limits[mailPlanLimitKey{kind, id}]})
	case http.MethodPut:
		var request mailPlanLimitsRequest
		if err := decodeStrictJSON(w, r, &request); err != nil {
			writeClientError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if (request.OwnerKind != "plan" && request.OwnerKind != "subscription") || request.OwnerID <= 0 {
			writeClientError(w, http.StatusBadRequest, "owner_kind must be plan or subscription and owner_id a positive number")
			return
		}
		if !validPlanLimitValue(request.MailboxSizeMB, 1, transport.MaxMailboxQuotaMB) {
			writeClientError(w, http.StatusBadRequest, fmt.Sprintf("the largest mailbox must be between 1 and %d MB, or empty", transport.MaxMailboxQuotaMB))
			return
		}
		for _, v := range []*int{request.OutgoingMailbox, request.OutgoingDomain, request.OutgoingSubscription} {
			if !validPlanLimitValue(v, 0, transport.OutgoingMailMaxPerHour) {
				writeClientError(w, http.StatusBadRequest, fmt.Sprintf("an hourly value must be between 0 (no limit) and %d, or empty", transport.OutgoingMailMaxPerHour))
				return
			}
		}
		table := "service_plans"
		if request.OwnerKind == "subscription" {
			table = "subscriptions"
		}
		var exists int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table+` WHERE id = ?`, request.OwnerID).Scan(&exists); err != nil {
			writeServerError(w, err)
			return
		}
		if exists == 0 {
			writeClientError(w, http.StatusNotFound, "no such plan or subscription")
			return
		}
		nullable := func(v *int) any {
			if v == nil {
				return nil
			}
			return *v
		}
		var err error
		if request.MailboxSizeMB == nil && request.OutgoingMailbox == nil && request.OutgoingDomain == nil && request.OutgoingSubscription == nil {
			_, err = db.ExecContext(ctx, `DELETE FROM mail_plan_limits WHERE owner_kind = ? AND owner_id = ?`, request.OwnerKind, request.OwnerID)
		} else {
			_, err = db.ExecContext(ctx, `
				INSERT INTO mail_plan_limits (owner_kind, owner_id, mailbox_size_mb, outgoing_mailbox, outgoing_domain, outgoing_subscription, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, datetime('now'))
				ON CONFLICT(owner_kind, owner_id) DO UPDATE SET mailbox_size_mb = excluded.mailbox_size_mb,
				    outgoing_mailbox = excluded.outgoing_mailbox, outgoing_domain = excluded.outgoing_domain,
				    outgoing_subscription = excluded.outgoing_subscription, updated_at = excluded.updated_at`,
				request.OwnerKind, request.OwnerID, nullable(request.MailboxSizeMB), nullable(request.OutgoingMailbox),
				nullable(request.OutgoingDomain), nullable(request.OutgoingSubscription))
		}
		if err != nil {
			writeServerError(w, err)
			return
		}
		p.audit(r, "mail.plan.limits", request.OwnerKind, request.OwnerID)
		// The outgoing values reach postfwd with the next apply (within a minute).
		p.requestOutgoingApply()
		_ = json.NewEncoder(w).Encode(map[string]any{"saved": true})
	default:
		w.Header().Set("Allow", "GET, PUT")
		writeClientError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// mailboxSizeCeiling is the largest mailbox the domain's plan allows (0 =
// none; then the server's 1 TB applies).
func (p *Panel) mailboxSizeCeiling(ctx context.Context, domainID int) (int, error) {
	db := p.db.GetDB()
	var subID int
	var planID sql.NullInt64
	if err := db.QueryRowContext(ctx, `SELECT d.subscription_id, s.plan_id FROM domains d JOIN subscriptions s ON s.id = d.subscription_id WHERE d.id = ?`,
		domainID).Scan(&subID, &planID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, err
	}
	limits, err := loadMailPlanLimits(ctx, db)
	if err != nil {
		return 0, err
	}
	if size := planLimitsFor(limits, subID, planID).MailboxSizeMB; size != nil {
		return *size, nil
	}
	return 0, nil
}

// refuseMailboxAbovePlan answers a size above the plan's largest mailbox for
// a reseller or customer, and says who can raise it. It reports whether it
// refused.
func (p *Panel) refuseMailboxAbovePlan(w http.ResponseWriter, r *http.Request, domainID, quotaMB int) bool {
	caller := currentCaller(r)
	if caller != nil && caller.Role == roleAdmin {
		return false
	}
	ceiling, err := p.mailboxSizeCeiling(r.Context(), domainID)
	if err != nil {
		writeServerError(w, err)
		return true
	}
	if ceiling > 0 && quotaMB > ceiling {
		writeCodedError(w, http.StatusForbidden, errCodeMailboxSizeAbovePlan,
			fmt.Sprintf("This hosting plan allows mailboxes of at most %d MB. Choose %d MB or less, or ask the provider or administrator for a larger plan.", ceiling, ceiling), "")
		return true
	}
	return false
}

// outgoingAdminPlans lists the customer plans with their mail settings.
func (p *Panel) outgoingAdminPlans(ctx context.Context) ([]outgoingPlanView, error) {
	db := p.db.GetDB()
	limits, err := loadMailPlanLimits(ctx, db)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT id, name FROM service_plans ORDER BY name, id LIMIT 500`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []outgoingPlanView{}
	for rows.Next() {
		var v outgoingPlanView
		if err := rows.Scan(&v.ID, &v.Name); err != nil {
			return nil, err
		}
		v.Limits = limits[mailPlanLimitKey{"plan", v.ID}]
		out = append(out, v)
	}
	return out, rows.Err()
}
