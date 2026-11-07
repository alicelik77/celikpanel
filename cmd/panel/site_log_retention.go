package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Site log retention per plan (alpha.128, PLESK-GAP G27; transport/site_logs.go).
//
//	GET  /api/v1/domains/{id}/logs/retention   how long this site's logs are kept, and why
//	POST /api/v1/domains/{id}/logs/retention   (administrator) set it for the subscription, its plan or the server
//
// The distribution's logrotate rule stays the only rotation rule of the site
// logs; CelikPanel adds no second rule (logrotate would refuse it and stop
// rotating every log). A limit makes the Agent remove that site's rotated
// files older than the limit, at once after a change and every six hours.
// The subscription's value wins over its plan's, the plan's over the
// server's; none means the system's own rule alone.
//
// Site günlüklerinin saklanması: dağıtımın kuralı tek döndürme kuralıdır;
// sınır, sitenin döndürülmüş eski dosyalarının silinmesidir.

const (
	settingSiteLogsKeepDays = "site_logs_keep_days"
	siteLogRetentionTick    = 6 * time.Hour
	maxLogRetentionDays     = 3650

	errCodeLogRetentionInvalid = "LOG_RETENTION_INVALID"
)

var requestIdentityRouteLogRetention = requestIdentityRoute{
	pattern: "/api/v1/domains/{id}/logs/retention", optional: true,
	timeout: agentRPCMutationTimeout + time.Minute,
}

// Seams (tests replace them).
var (
	siteLogRetentionCall = func(p *Panel, ctx context.Context, items []transport.SiteLogRetentionItem) (transport.SiteLogRetentionResponse, error) {
		var resp transport.SiteLogRetentionResponse
		err := p.callAgentContext(ctx, "Agent.SiteLogRetentionV1", &transport.SiteLogRetentionRequest{
			ExpectedBuildCommit: strings.TrimSpace(buildCommit), Items: items,
		}, &resp)
		return resp, err
	}
	siteLogStatusCall = func(p *Panel, ctx context.Context, domain string) (transport.SiteLogStatusResponse, error) {
		var resp transport.SiteLogStatusResponse
		err := p.callAgentContext(ctx, "Agent.SiteLogStatusV1", &transport.SiteLogStatusRequest{Domain: domain}, &resp)
		return resp, err
	}
)

// logRetention is the effective limit of one subscription and where it comes from.
type logRetention struct {
	KeepDays         int    `json:"keep_days"`
	Source           string `json:"source"` // subscription, plan, server, system
	SubscriptionDays int    `json:"subscription_days,omitempty"`
	PlanID           int    `json:"plan_id,omitempty"`
	PlanName         string `json:"plan_name,omitempty"`
	PlanDays         int    `json:"plan_days,omitempty"`
	ServerDays       int    `json:"server_days,omitempty"`
}

func (p *Panel) effectiveLogRetention(ctx context.Context, subscriptionID int) (logRetention, error) {
	var r logRetention
	db := p.db.GetDB()
	var planID sql.NullInt64
	var planName sql.NullString
	if err := db.QueryRowContext(ctx, `
		SELECT s.plan_id, sp.name FROM subscriptions s LEFT JOIN service_plans sp ON sp.id = s.plan_id WHERE s.id = ?`,
		subscriptionID).Scan(&planID, &planName); err != nil {
		return r, err
	}
	_ = db.QueryRowContext(ctx, `SELECT keep_days FROM log_retention_subscriptions WHERE subscription_id = ?`, subscriptionID).Scan(&r.SubscriptionDays)
	if planID.Valid {
		r.PlanID, r.PlanName = int(planID.Int64), planName.String
		_ = db.QueryRowContext(ctx, `SELECT keep_days FROM log_retention_plans WHERE plan_id = ?`, r.PlanID).Scan(&r.PlanDays)
	}
	if days, err := strconv.Atoi(p.setting(ctx, settingSiteLogsKeepDays)); err == nil && days > 0 && days <= maxLogRetentionDays {
		r.ServerDays = days
	}
	switch {
	case r.SubscriptionDays > 0:
		r.KeepDays, r.Source = r.SubscriptionDays, "subscription"
	case r.PlanDays > 0:
		r.KeepDays, r.Source = r.PlanDays, "plan"
	case r.ServerDays > 0:
		r.KeepDays, r.Source = r.ServerDays, "server"
	default:
		r.Source = "system"
	}
	return r, nil
}

type logRetentionView struct {
	logRetention
	Domain    string                      `json:"domain"`
	MayChange bool                        `json:"may_change"`
	Files     []transport.SiteLogFile     `json:"files"`
	System    transport.SiteLogSystemRule `json:"system"`
	// FilesUnknown: the server could not be read just now.
	FilesUnknown bool `json:"files_unknown,omitempty"`
}

func (p *Panel) handleDomainLogRetention(w http.ResponseWriter, r *http.Request, domainID int) {
	ctx := r.Context()
	var domain string
	var subscriptionID int
	if err := p.db.GetDB().QueryRowContext(ctx, `SELECT name, subscription_id FROM domains WHERE id = ?`, domainID).Scan(&domain, &subscriptionID); err != nil {
		writeClientError(w, http.StatusNotFound, "domain not found")
		return
	}
	if r.Method == http.MethodPost {
		if !callerIsAdministrator(r) {
			writeClientError(w, http.StatusForbidden, "only the administrator changes how long logs are kept")
			return
		}
		var req struct {
			Scope    string `json:"scope"`
			KeepDays int    `json:"keep_days"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&req); err != nil {
			writeClientError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		reportOperationPlan(ctx, "check", "save", "sweep")
		reportOperationStep(ctx, "check")
		if req.KeepDays < 0 || req.KeepDays > maxLogRetentionDays {
			writeCodedError(w, http.StatusBadRequest, errCodeLogRetentionInvalid, "Nothing was changed: logs are kept 1 to 3650 days, or as the system keeps them.", "")
			return
		}
		current, err := p.effectiveLogRetention(ctx, subscriptionID)
		if err != nil {
			writeServerError(w, err)
			return
		}
		reportOperationStep(ctx, "save")
		if err := p.saveLogRetention(ctx, req.Scope, subscriptionID, current.PlanID, req.KeepDays); err != nil {
			var bad *errLogRetentionScope
			if errors.As(err, &bad) {
				writeCodedError(w, http.StatusBadRequest, errCodeLogRetentionInvalid, "Nothing was changed: "+bad.Error()+".", "")
				return
			}
			writeServerError(w, err)
			return
		}
		p.auditDetailed(r, "logs.retention", "domain", domainID, map[string]any{"scope": req.Scope, "keep_days": req.KeepDays})
		reportOperationStep(ctx, "sweep")
		// The change is applied at once to every site it covers (also the
		// next six-hourly round would); a failed sweep is said, not hidden.
		if err := p.runSiteLogRetention(context.WithoutCancel(ctx)); err != nil {
			writeCodedError(w, http.StatusBadGateway, "LOG_RETENTION_SWEEP",
				"Saved. Removing the older rotated logs did not finish ("+firstLineOf(err.Error())+"); it is tried again within six hours.", "")
			return
		}
	}
	retention, err := p.effectiveLogRetention(ctx, subscriptionID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	view := logRetentionView{logRetention: retention, Domain: domain, MayChange: callerIsAdministrator(r), Files: []transport.SiteLogFile{}}
	readCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if status, err := siteLogStatusCall(p, readCtx, domain); err == nil && status.Error == "" {
		view.Files, view.System = status.Files, status.System
	} else {
		view.FilesUnknown = true
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(view)
}

type errLogRetentionScope struct{ msg string }

func (e *errLogRetentionScope) Error() string { return e.msg }

// saveLogRetention stores one value (0 removes it).
func (p *Panel) saveLogRetention(ctx context.Context, scope string, subscriptionID, planID, days int) error {
	db := p.db.GetDB()
	switch scope {
	case "subscription":
		if days == 0 {
			_, err := db.ExecContext(ctx, `DELETE FROM log_retention_subscriptions WHERE subscription_id = ?`, subscriptionID)
			return err
		}
		_, err := db.ExecContext(ctx, `INSERT INTO log_retention_subscriptions (subscription_id, keep_days) VALUES (?, ?)
			ON CONFLICT(subscription_id) DO UPDATE SET keep_days = excluded.keep_days, updated_at = datetime('now')`, subscriptionID, days)
		return err
	case "plan":
		if planID == 0 {
			return &errLogRetentionScope{"this subscription follows no plan; set it for the subscription instead"}
		}
		if days == 0 {
			_, err := db.ExecContext(ctx, `DELETE FROM log_retention_plans WHERE plan_id = ?`, planID)
			return err
		}
		_, err := db.ExecContext(ctx, `INSERT INTO log_retention_plans (plan_id, keep_days) VALUES (?, ?)
			ON CONFLICT(plan_id) DO UPDATE SET keep_days = excluded.keep_days, updated_at = datetime('now')`, planID, days)
		return err
	case "server":
		value := ""
		if days > 0 {
			value = strconv.Itoa(days)
		}
		return p.setSetting(ctx, settingSiteLogsKeepDays, value)
	}
	return &errLogRetentionScope{"choose the subscription, its plan or the server"}
}

// runSiteLogRetention applies every site's effective limit once.
func (p *Panel) runSiteLogRetention(ctx context.Context) error {
	rows, err := p.db.GetDB().QueryContext(ctx, `
		SELECT d.name, d.subscription_id FROM domains d JOIN sites s ON s.domain_id = d.id ORDER BY d.id`)
	if err != nil {
		return err
	}
	type site struct {
		domain string
		sub    int
	}
	var sites []site
	for rows.Next() {
		var s site
		if rows.Scan(&s.domain, &s.sub) == nil {
			sites = append(sites, s)
		}
	}
	rows.Close()
	var items []transport.SiteLogRetentionItem
	cache := map[int]int{}
	for _, s := range sites {
		days, seen := cache[s.sub]
		if !seen {
			r, err := p.effectiveLogRetention(ctx, s.sub)
			if err != nil {
				continue
			}
			days = r.KeepDays
			cache[s.sub] = days
		}
		if days > 0 {
			items = append(items, transport.SiteLogRetentionItem{Domain: s.domain, KeepDays: days})
		}
	}
	if len(items) == 0 {
		return nil
	}
	callCtx, cancel := context.WithTimeout(ctx, agentRPCMutationTimeout)
	defer cancel()
	resp, err := siteLogRetentionCall(p, callCtx, items)
	if err != nil {
		return err
	}
	if resp.Error != "" {
		return errors.New(resp.Error)
	}
	return nil
}

// startSiteLogRetention applies the limits every six hours.
func (p *Panel) startSiteLogRetention() {
	go func() {
		ticker := time.NewTicker(siteLogRetentionTick)
		defer ticker.Stop()
		for range ticker.C {
			ctx, cancel := context.WithTimeout(context.Background(), agentRPCMutationTimeout+time.Minute)
			if err := p.runSiteLogRetention(ctx); err != nil {
				log.Printf("[logs] retention: %v", err)
			}
			cancel()
		}
	}()
}
