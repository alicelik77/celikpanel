package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Outgoing mail limits (alpha.128, gap G1; owner decisions of 7 Nov 2026).
//
// The panel keeps the wanted state (server defaults, plan values, mailbox
// values and pauses) and hands the whole of it to the Agent, which makes
// postfwd hold it natively (cmd/agent/mail_outgoing_limits.go). Five minutes
// apart the worker reads what each mailbox sent from Postfix's log, raises
// "limit reached" notices (at most one per mailbox per hour), pauses a
// mailbox whose hour is far above its own normal, and re-applies when the
// state changed (a new mailbox's first day ended, a domain moved). A paused
// mailbox sends again only after its password changes.
//
//	GET  /api/v1/mail/outgoing                 administrator: settings, server state, paused
//	POST /api/v1/mail/outgoing                 administrator: save and apply (steps)
//	GET  /api/v1/mail/outgoing/report?days=N   administrator: who sent, who was refused
//	GET  /api/v1/domains/{id}/mail/outgoing    the domain's mailboxes, limits and use
//	PUT  /api/v1/domains/{id}/mail/outgoing    one mailbox's value (within the ceiling)
//	GET  /api/v1/mail-outgoing-notices         the caller's unseen notices
//	POST /api/v1/mail-outgoing-notices/{id}/seen
//
// Polling only reads: no GET starts an apply or a pause.
//
// Giden posta sınırları: panel istenen durumu tutar ve tamamını Agent'a
// verir; beş dakikada bir gönderimleri okur, bildirimleri yazar, olağandışı
// artışta posta kutusunu duraklatır. Duraklatılan kutu yalnız parolası
// değişince yeniden gönderir.

const (
	mailOutgoingPath        = "/api/v1/mail/outgoing"
	mailOutgoingReportPath  = "/api/v1/mail/outgoing/report"
	mailOutgoingNoticesPath = "/api/v1/mail-outgoing-notices"

	outgoingWorkerTick      = 5 * time.Minute
	outgoingRetryAfterFail  = 30 * time.Minute
	outgoingHourlyRetention = 35 * 24 * time.Hour

	outgoingNoticeLimit  = "limit_reached"
	outgoingNoticePaused = "paused"

	errCodeOutgoingAboveCeiling = "OUTGOING_ABOVE_CEILING"
)

// outgoingSettings is the server's row.
type outgoingSettings struct {
	Enabled             bool `json:"enabled"`
	MailboxPerHour      int  `json:"mailbox_per_hour"`
	DomainPerHour       int  `json:"domain_per_hour"`
	SubscriptionPerHour int  `json:"subscription_per_hour"`
	NewMailboxPerHour   int  `json:"new_mailbox_per_hour"`
	NewMailboxHours     int  `json:"new_mailbox_hours"`
	SpikeFactor         int  `json:"spike_factor"`
	SpikeMinimum        int  `json:"spike_minimum"`
	RefusalPause        int  `json:"refusal_pause"`
}

// outgoingApplyRecord is the last attempt to make the server hold the state.
type outgoingApplyRecord struct {
	State       string `json:"state,omitempty"`
	Code        string `json:"code,omitempty"`
	Detail      string `json:"detail,omitempty"`
	AppliedAt   string `json:"applied_at,omitempty"`
	AttemptedAt string `json:"attempted_at,omitempty"`
	VerifiedAt  string `json:"verified_at,omitempty"`
	hash        string
	attempted   string
}

func (s outgoingSettings) validate() error {
	inRange := func(v, lo, hi int) bool { return v >= lo && v <= hi }
	switch {
	case !inRange(s.MailboxPerHour, 0, transport.OutgoingMailMaxPerHour),
		!inRange(s.DomainPerHour, 0, transport.OutgoingMailMaxPerHour),
		!inRange(s.SubscriptionPerHour, 0, transport.OutgoingMailMaxPerHour),
		!inRange(s.NewMailboxPerHour, 0, transport.OutgoingMailMaxPerHour):
		return fmt.Errorf("an hourly limit must be between 0 (no limit) and %d", transport.OutgoingMailMaxPerHour)
	case !inRange(s.NewMailboxHours, 0, 720):
		return errors.New("the first-day period must be between 0 and 720 hours")
	case !inRange(s.SpikeFactor, 2, 100):
		return errors.New("the unusual-sending factor must be between 2 and 100")
	case !inRange(s.SpikeMinimum, 10, transport.OutgoingMailMaxPerHour):
		return errors.New("the unusual-sending minimum must be at least 10 recipients")
	case !inRange(s.RefusalPause, 0, transport.OutgoingMailMaxPerHour):
		return errors.New("the refusal count that pauses a mailbox must be 0 (never) or more")
	}
	return nil
}

func (p *Panel) loadOutgoingSettings(ctx context.Context) (outgoingSettings, outgoingApplyRecord, error) {
	var s outgoingSettings
	var rec outgoingApplyRecord
	var enabled int
	var hash, state, code, detail, applied, attempted, verified, attemptedHash sql.NullString
	err := p.db.GetDB().QueryRowContext(ctx, `
		SELECT enabled, mailbox_per_hour, domain_per_hour, subscription_per_hour, new_mailbox_per_hour,
		       new_mailbox_hours, spike_factor, spike_minimum, refusal_pause,
		       applied_hash, apply_state, apply_code, apply_detail, applied_at, attempted_at, verified_at, attempted_hash
		FROM mail_outgoing_settings WHERE id = 1`).Scan(&enabled, &s.MailboxPerHour, &s.DomainPerHour, &s.SubscriptionPerHour,
		&s.NewMailboxPerHour, &s.NewMailboxHours, &s.SpikeFactor, &s.SpikeMinimum, &s.RefusalPause,
		&hash, &state, &code, &detail, &applied, &attempted, &verified, &attemptedHash)
	if err != nil {
		return s, rec, err
	}
	s.Enabled = enabled == 1
	rec = outgoingApplyRecord{State: state.String, Code: code.String, Detail: detail.String, AppliedAt: applied.String,
		AttemptedAt: attempted.String, VerifiedAt: verified.String, hash: hash.String, attempted: attemptedHash.String}
	return s, rec, nil
}

// ---- The wanted state -------------------------------------------------------

// mailPlanLimitRow is one plan's or Custom subscription's mail settings
// (G12; NULL = the server's value, 0 = no limit).
type mailPlanLimitRow struct {
	MailboxSizeMB        *int `json:"mailbox_size_mb"`
	OutgoingMailbox      *int `json:"outgoing_mailbox"`
	OutgoingDomain       *int `json:"outgoing_domain"`
	OutgoingSubscription *int `json:"outgoing_subscription"`
}

type mailPlanLimitKey struct {
	kind string
	id   int
}

func loadMailPlanLimits(ctx context.Context, q sqlQueryer) (map[mailPlanLimitKey]mailPlanLimitRow, error) {
	rows, err := q.QueryContext(ctx, `SELECT owner_kind, owner_id, mailbox_size_mb, outgoing_mailbox, outgoing_domain, outgoing_subscription FROM mail_plan_limits`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[mailPlanLimitKey]mailPlanLimitRow{}
	for rows.Next() {
		var key mailPlanLimitKey
		var size, mailbox, domain, sub sql.NullInt64
		if err := rows.Scan(&key.kind, &key.id, &size, &mailbox, &domain, &sub); err != nil {
			return nil, err
		}
		out[key] = mailPlanLimitRow{MailboxSizeMB: nullIntPtr(size), OutgoingMailbox: nullIntPtr(mailbox),
			OutgoingDomain: nullIntPtr(domain), OutgoingSubscription: nullIntPtr(sub)}
	}
	return out, rows.Err()
}

// nullIntPtr is shared with the limits model (setting_limits.go).

// planLimitsFor: a plan-bound subscription reads its plan's row, a Custom
// subscription its own; Unlimited has none.
func planLimitsFor(limits map[mailPlanLimitKey]mailPlanLimitRow, subscriptionID int, planID sql.NullInt64) mailPlanLimitRow {
	if planID.Valid && planID.Int64 > 0 {
		return limits[mailPlanLimitKey{"plan", int(planID.Int64)}]
	}
	return limits[mailPlanLimitKey{"subscription", subscriptionID}]
}

type outgoingMailbox struct {
	ID             int
	Address        string
	DomainID       int
	Domain         string
	SubscriptionID int
	PlanID         sql.NullInt64
	Created        time.Time
	PerHour        *int
	Paused         bool
	PausedAt       string
	PausedReason   string
	PausedDetail   string
	ResumedAt      string
}

func loadOutgoingMailboxes(ctx context.Context, q sqlQueryer, domainID int) ([]outgoingMailbox, error) {
	query := `
		SELECT a.id, a.address, COALESCE(a.created_at, ''), d.id, d.name, d.subscription_id, s.plan_id,
		       mo.per_hour, COALESCE(mo.paused_at, ''), COALESCE(mo.paused_reason, ''), COALESCE(mo.paused_detail, ''),
		       COALESCE(mo.resumed_at, '')
		FROM email_accounts a
		JOIN domains d ON d.id = a.domain_id
		JOIN subscriptions s ON s.id = d.subscription_id
		LEFT JOIN mailbox_outgoing mo ON mo.email_account_id = a.id`
	var args []any
	if domainID > 0 {
		query += ` WHERE d.id = ?`
		args = append(args, domainID)
	}
	query += ` ORDER BY a.address`
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []outgoingMailbox
	for rows.Next() {
		var m outgoingMailbox
		var created string
		var perHour sql.NullInt64
		if err := rows.Scan(&m.ID, &m.Address, &created, &m.DomainID, &m.Domain, &m.SubscriptionID, &m.PlanID,
			&perHour, &m.PausedAt, &m.PausedReason, &m.PausedDetail, &m.ResumedAt); err != nil {
			return nil, err
		}
		m.Address, m.Domain = strings.ToLower(m.Address), strings.ToLower(m.Domain)
		m.Created = parseSQLiteTime(created)
		m.PerHour = nullIntPtr(perHour)
		m.Paused = m.PausedAt != ""
		out = append(out, m)
	}
	return out, rows.Err()
}

// parseSQLiteTime reads SQLite's datetime('now') (UTC) or RFC 3339; zero when
// neither.
func parseSQLiteTime(value string) time.Time {
	value = strings.TrimSpace(value)
	for _, layout := range []string{"2006-01-02 15:04:05", time.RFC3339, time.RFC3339Nano} {
		if t, err := time.ParseInLocation(layout, value, time.UTC); err == nil {
			return t
		}
	}
	return time.Time{}
}

// outgoingEffective is one mailbox's limit as the server enforces it.
// Limit 0 means no mailbox limit (domain and account totals still apply).
type outgoingEffective struct {
	Limit int `json:"limit"`
	// Mode: default, custom or unlimited (the mailbox's own choice).
	Mode    string `json:"mode"`
	Custom  int    `json:"custom,omitempty"`
	Default int    `json:"default"`
	// Ceiling: the most a reseller or customer may choose (0 = no ceiling).
	Ceiling int `json:"ceiling"`
	// NewUntil: the first-day limit applies until then.
	NewUntil string `json:"new_until,omitempty"`
}

// lowerLimit is the smaller of two hourly limits where 0 means none.
func lowerLimit(a, b int) int {
	switch {
	case a == 0:
		return b
	case b == 0:
		return a
	case a < b:
		return a
	}
	return b
}

// outgoingCeilingAndDefault: the plan's mailbox value is the ceiling (0 = no
// ceiling) and the default is the server default within it; without a plan
// value the server default is both.
func outgoingCeilingAndDefault(settings outgoingSettings, plan mailPlanLimitRow) (ceiling, def int) {
	if plan.OutgoingMailbox == nil {
		return settings.MailboxPerHour, settings.MailboxPerHour
	}
	ceiling = *plan.OutgoingMailbox
	return ceiling, lowerLimit(settings.MailboxPerHour, ceiling)
}

func effectiveOutgoing(settings outgoingSettings, plan mailPlanLimitRow, m outgoingMailbox, now time.Time) outgoingEffective {
	ceiling, def := outgoingCeilingAndDefault(settings, plan)
	e := outgoingEffective{Mode: "default", Default: def, Ceiling: ceiling, Limit: def}
	if m.PerHour != nil {
		if *m.PerHour == 0 {
			e.Mode, e.Limit = "unlimited", 0
		} else {
			e.Mode, e.Custom, e.Limit = "custom", *m.PerHour, *m.PerHour
		}
	}
	if settings.NewMailboxPerHour > 0 && settings.NewMailboxHours > 0 && !m.Created.IsZero() {
		until := m.Created.Add(time.Duration(settings.NewMailboxHours) * time.Hour)
		if now.Before(until) {
			e.NewUntil = until.UTC().Format(time.RFC3339)
			e.Limit = lowerLimit(e.Limit, settings.NewMailboxPerHour)
		}
	}
	return e
}

func outgoingDomainLimit(settings outgoingSettings, plan mailPlanLimitRow) int {
	if plan.OutgoingDomain != nil {
		return *plan.OutgoingDomain
	}
	return settings.DomainPerHour
}

func outgoingSubscriptionLimit(settings outgoingSettings, plan mailPlanLimitRow) int {
	if plan.OutgoingSubscription != nil {
		return *plan.OutgoingSubscription
	}
	return settings.SubscriptionPerHour
}

// buildOutgoingSpec turns the database into the Agent's whole wanted state.
// chains folds the rights chain's mail ceilings in (nil: the mail rows only).
func buildOutgoingSpec(settings outgoingSettings, mailboxes []outgoingMailbox, limits map[mailPlanLimitKey]mailPlanLimitRow, chains *mailLimitChains, now time.Time) transport.OutgoingMailLimitsSpec {
	spec := transport.OutgoingMailLimitsSpec{Enabled: settings.Enabled, MailboxLimits: map[string]int{}, DomainLimits: map[string]int{}}
	if !settings.Enabled {
		return spec
	}
	subDomains := map[int]map[string]bool{}
	subLimit := map[int]int{}
	for _, m := range mailboxes {
		if _, err := transport.CanonicalMailAddress(m.Address); err != nil {
			continue
		}
		plan := chains.mailRowFor(limits, settings, m.SubscriptionID, m.PlanID)
		if m.Paused {
			spec.Paused = append(spec.Paused, m.Address)
		}
		if limit := effectiveOutgoing(settings, plan, m, now).Limit; limit > 0 {
			spec.MailboxLimits[m.Address] = limit
		}
		if limit := outgoingDomainLimit(settings, plan); limit > 0 {
			spec.DomainLimits[m.Domain] = limit
		}
		if limit := outgoingSubscriptionLimit(settings, plan); limit > 0 {
			if subDomains[m.SubscriptionID] == nil {
				subDomains[m.SubscriptionID] = map[string]bool{}
			}
			subDomains[m.SubscriptionID][m.Domain] = true
			subLimit[m.SubscriptionID] = limit
		}
	}
	sort.Strings(spec.Paused)
	ids := make([]int, 0, len(subDomains))
	for id := range subDomains {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	for _, id := range ids {
		domains := make([]string, 0, len(subDomains[id]))
		for domain := range subDomains[id] {
			domains = append(domains, domain)
		}
		sort.Strings(domains)
		spec.Subscriptions = append(spec.Subscriptions, transport.OutgoingMailSubscription{
			Key: fmt.Sprintf("s%d", id), Domains: domains, Limit: subLimit[id],
		})
	}
	return spec
}

func outgoingSpecHash(spec transport.OutgoingMailLimitsSpec) string {
	raw, _ := json.Marshal(spec)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// ---- Applying ---------------------------------------------------------------

type outgoingMailWorker struct {
	once  sync.Once
	kick  chan struct{}
	apply sync.Mutex
}

var outgoingWorkers sync.Map // *Panel -> *outgoingMailWorker

func (p *Panel) outgoingWorker() *outgoingMailWorker {
	worker, _ := outgoingWorkers.LoadOrStore(p, &outgoingMailWorker{kick: make(chan struct{}, 1)})
	return worker.(*outgoingMailWorker)
}

// requestOutgoingApply asks the worker to apply soon (a new or deleted
// mailbox, a changed plan); it never blocks the caller.
func (p *Panel) requestOutgoingApply() {
	select {
	case p.outgoingWorker().kick <- struct{}{}:
	default:
	}
}

// outgoingAgentApply is replaced by tests.
var outgoingAgentApply = func(p *Panel, ctx context.Context, spec *transport.OutgoingMailLimitsSpec) (transport.OutgoingMailLimitsApplyResponse, error) {
	var resp transport.OutgoingMailLimitsApplyResponse
	err := p.callAgentContext(ctx, "Agent.ApplyOutgoingMailLimitsV1", spec, &resp)
	return resp, err
}

// applyOutgoingMail makes the server hold the current wanted state. force
// asks even when the server already holds the same state (an explicit save).
// It records the outcome; the answer is the Agent's.
func (p *Panel) applyOutgoingMail(ctx context.Context, force bool) (transport.OutgoingMailLimitsApplyResponse, bool, error) {
	worker := p.outgoingWorker()
	worker.apply.Lock()
	defer worker.apply.Unlock()
	db := p.db.GetDB()
	settings, record, err := p.loadOutgoingSettings(ctx)
	if err != nil {
		return transport.OutgoingMailLimitsApplyResponse{}, false, err
	}
	mailboxes, err := loadOutgoingMailboxes(ctx, db, 0)
	if err != nil {
		return transport.OutgoingMailLimitsApplyResponse{}, false, err
	}
	limits, err := loadMailPlanLimits(ctx, db)
	if err != nil {
		return transport.OutgoingMailLimitsApplyResponse{}, false, err
	}
	spec := buildOutgoingSpec(settings, mailboxes, limits, newMailLimitChains(ctx, db), time.Now())
	hash := outgoingSpecHash(spec)
	if !force {
		switch {
		case !settings.Enabled && record.hash == "" && record.State == "":
			// Never turned on: nothing on the server to change.
			return transport.OutgoingMailLimitsApplyResponse{}, false, nil
		case hash == record.hash && record.State == "applied":
			// The server already holds exactly this state (a saved value
			// that does not change what postfwd counts, e.g. a mailbox
			// limit above its new-mailbox cap): it is in force, not
			// deferred (alpha.128 lab: bulk saves answered applied false).
			return transport.OutgoingMailLimitsApplyResponse{Applied: true}, false, nil
		case hash == record.attempted && record.State != "applied":
			// A refusal that ends without anyone touching the panel (the
			// administrator installed postfwd from Components, another mail
			// change finished) is asked again at the next round; a failure
			// waits 30 minutes.
			waitsForOthers := record.Code == transport.OutgoingMailNotInstalled || record.Code == transport.OutgoingMailBusy
			if at := parseSQLiteTime(record.AttemptedAt); !waitsForOthers && !at.IsZero() && time.Since(at) < outgoingRetryAfterFail {
				return transport.OutgoingMailLimitsApplyResponse{}, false, nil
			}
		}
	}
	reportOperationStep(ctx, "apply")
	resp, callErr := outgoingAgentApply(p, ctx, &spec)
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	state, code, detail := "applied", "", resp.Detail
	switch {
	case callErr != nil:
		state, code, detail = "failed", "AGENT_CALL_FAILED", boundedAgentDiagnostic(callErr.Error())
	case !resp.Applied:
		state, code, detail = "refused", resp.Code, boundedAgentDiagnostic(resp.Error)
		if resp.Code == transport.OutgoingMailCheckFailed || resp.Code == transport.OutgoingMailServiceFailed ||
			resp.Code == transport.OutgoingMailPostfixFailed || resp.Code == transport.OutgoingMailSelfTestFailed {
			state = "failed"
		}
	}
	appliedHash := record.hash
	if state == "applied" {
		appliedHash = hash
	}
	if _, err := db.ExecContext(ctx, `
		UPDATE mail_outgoing_settings SET applied_hash = ?, attempted_hash = ?, apply_state = ?, apply_code = ?, apply_detail = ?, attempted_at = ?,
		       applied_at = CASE WHEN ? = 'applied' THEN ? ELSE applied_at END,
		       verified_at = CASE WHEN ? THEN ? ELSE verified_at END
		WHERE id = 1`,
		nullIfEmpty(appliedHash), hash, state, nullIfEmpty(code), nullIfEmpty(detail), now, state, now,
		state == "applied" && resp.Verified, now); err != nil {
		log.Printf("[mail-outgoing] record apply: %v", err)
	}
	if callErr != nil {
		return resp, true, callErr
	}
	return resp, true, nil
}

// ---- The worker -------------------------------------------------------------

func (p *Panel) startOutgoingMailWorker() {
	worker := p.outgoingWorker()
	worker.once.Do(func() {
		go func() {
			ticker := time.NewTicker(outgoingWorkerTick)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
				case <-worker.kick:
				}
				ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
				p.runOutgoingMailRound(ctx)
				cancel()
			}
		}()
	})
}

// outgoingAgentActivity is replaced by tests.
var outgoingAgentActivity = func(p *Panel, ctx context.Context, since time.Time) (transport.OutgoingMailActivityResponse, error) {
	var resp transport.OutgoingMailActivityResponse
	err := p.callAgentContext(ctx, "Agent.OutgoingMailActivityV1",
		&transport.OutgoingMailActivityRequest{Since: since.UTC().Format(time.RFC3339)}, &resp)
	return resp, err
}

func (p *Panel) runOutgoingMailRound(ctx context.Context) {
	settings, _, err := p.loadOutgoingSettings(ctx)
	if err != nil {
		log.Printf("[mail-outgoing] settings: %v", err)
		return
	}
	now := time.Now().UTC()
	if settings.Enabled {
		if err := p.ingestOutgoingActivity(ctx, now); err != nil {
			log.Printf("[mail-outgoing] activity: %v", err)
		} else {
			p.raiseOutgoingLimitNotices(ctx, now)
			p.pauseUnusualSenders(ctx, settings, now)
		}
	}
	if _, _, err := p.applyOutgoingMail(ctx, false); err != nil {
		log.Printf("[mail-outgoing] apply: %v", err)
	}
	if err := p.sendPendingOutgoingNotices(ctx); err != nil {
		log.Printf("[mail-outgoing] notices: %v", err)
	}
	// alpha.128 (G25): the per-domain options map follows the database (a
	// lost earlier apply, a mailbox added elsewhere); unused, nothing is sent.
	if _, _, err := p.applyDomainMailOptions(ctx, false); err != nil {
		log.Printf("[mail-options] apply: %v", err)
	}
}

// ingestOutgoingActivity stores the current and the previous hour as Postfix's
// log tells them (replacing what an earlier round stored for them).
func (p *Panel) ingestOutgoingActivity(ctx context.Context, now time.Time) error {
	since := now.Truncate(time.Hour).Add(-time.Hour)
	resp, err := outgoingAgentActivity(p, ctx, since)
	if err != nil {
		return err
	}
	if !resp.Read {
		return errors.New(firstNonEmptyPanel(resp.Error, "Postfix's log could not be read"))
	}
	return storeOutgoingBuckets(ctx, p.db.GetDB(), resp.Buckets, since, now)
}

func firstNonEmptyPanel(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func storeOutgoingBuckets(ctx context.Context, db *sql.DB, buckets []transport.OutgoingMailBucket, since, now time.Time) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, b := range buckets {
		hour, err := time.Parse(time.RFC3339, b.Hour)
		if err != nil || hour.Before(since) || len(b.Address) < 3 || len(b.Address) > 320 {
			continue
		}
		var kind any
		switch b.LimitKind {
		case "mailbox", "domain", "subscription":
			kind = b.LimitKind
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO mail_outgoing_hourly (address, hour, messages, recipients, limit_refusals, limit_kind, paused_refusals, mismatch_refusals)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(address, hour) DO UPDATE SET messages = excluded.messages, recipients = excluded.recipients,
			    limit_refusals = excluded.limit_refusals, limit_kind = excluded.limit_kind,
			    paused_refusals = excluded.paused_refusals, mismatch_refusals = excluded.mismatch_refusals`,
			strings.ToLower(b.Address), hour.UTC().Format(time.RFC3339), max(b.Messages, 0), max(b.Recipients, 0),
			max(b.LimitRefusals, 0), kind, max(b.PausedRefusals, 0), max(b.MismatchRefusals, 0)); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM mail_outgoing_hourly WHERE hour < ?`,
		now.Add(-outgoingHourlyRetention).UTC().Format(time.RFC3339)); err != nil {
		return err
	}
	return tx.Commit()
}

// outgoingNoticeBody is the facts of one notice (no secret, no recipient).
type outgoingNoticeBody struct {
	Address      string `json:"address"`
	Subscription string `json:"subscription,omitempty"`
	Owner        string `json:"owner,omitempty"`
	Reseller     string `json:"reseller,omitempty"`
	DomainID     int    `json:"domain_id,omitempty"`
	Domain       string `json:"domain,omitempty"`
	Hour         string `json:"hour,omitempty"`
	LimitKind    string `json:"limit_kind,omitempty"`
	Limit        int    `json:"limit,omitempty"`
	Refused      int    `json:"refused,omitempty"`
	Reason       string `json:"reason,omitempty"`
	Recipients   int    `json:"recipients,omitempty"`
	Normal       int    `json:"normal,omitempty"`
}

type outgoingAccountRef struct {
	id, domainID, subscriptionID int
}

func outgoingAccountByAddress(ctx context.Context, q sqlQueryer, address string) (outgoingAccountRef, bool) {
	var ref outgoingAccountRef
	err := q.QueryRowContext(ctx, `
		SELECT a.id, d.id, d.subscription_id FROM email_accounts a JOIN domains d ON d.id = a.domain_id
		WHERE lower(a.address) = lower(?)`, address).Scan(&ref.id, &ref.domainID, &ref.subscriptionID)
	return ref, err == nil
}

func (p *Panel) storeOutgoingNotice(ctx context.Context, kind, address, period string, ref outgoingAccountRef, body outgoingNoticeBody, adminsOnlyFirst bool) {
	db := p.db.GetDB()
	recipients, err := usageRecipientsFor(ctx, db, ref.subscriptionID)
	if err != nil {
		log.Printf("[mail-outgoing] notice recipients: %v", err)
		return
	}
	body.Address, body.DomainID = address, ref.domainID
	_ = db.QueryRowContext(ctx, `SELECT name FROM domains WHERE id = ?`, ref.domainID).Scan(&body.Domain)
	body.Subscription, body.Owner, body.Reseller = subscriptionNoticeNames(ctx, db, ref.subscriptionID)
	raw, err := json.Marshal(body)
	if err != nil {
		return
	}
	for _, r := range recipients {
		if r.UserID <= 0 {
			continue
		}
		if _, err := db.ExecContext(ctx, `
			INSERT INTO mail_outgoing_notices (user_id, audience, kind, address, subscription_id, period, body)
			VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT DO NOTHING`,
			r.UserID, r.Audience, kind, address, ref.subscriptionID, period, string(raw)); err != nil {
			log.Printf("[mail-outgoing] store notice: %v", err)
		}
	}
	_ = adminsOnlyFirst
}

// raiseOutgoingLimitNotices: a mailbox refused by an hourly limit in the
// current or previous hour gets one notice per person for that hour.
func (p *Panel) raiseOutgoingLimitNotices(ctx context.Context, now time.Time) {
	db := p.db.GetDB()
	since := now.Truncate(time.Hour).Add(-time.Hour).Format(time.RFC3339)
	rows, err := db.QueryContext(ctx, `
		SELECT address, hour, limit_refusals, COALESCE(limit_kind, '') FROM mail_outgoing_hourly
		WHERE hour >= ? AND limit_refusals > 0 ORDER BY hour`, since)
	if err != nil {
		log.Printf("[mail-outgoing] limit notices: %v", err)
		return
	}
	type hit struct {
		address, hour, kind string
		refused             int
	}
	var hits []hit
	for rows.Next() {
		var h hit
		if rows.Scan(&h.address, &h.hour, &h.refused, &h.kind) == nil {
			hits = append(hits, h)
		}
	}
	rows.Close()
	if len(hits) == 0 {
		return
	}
	settings, _, err := p.loadOutgoingSettings(ctx)
	if err != nil {
		return
	}
	limits, _ := loadMailPlanLimits(ctx, db)
	chains := newMailLimitChains(ctx, db)
	for _, h := range hits {
		ref, ok := outgoingAccountByAddress(ctx, db, h.address)
		if !ok {
			continue
		}
		body := outgoingNoticeBody{Hour: h.hour, LimitKind: h.kind, Refused: h.refused}
		if mailboxes, err := loadOutgoingMailboxes(ctx, db, ref.domainID); err == nil {
			for _, m := range mailboxes {
				if m.ID != ref.id {
					continue
				}
				plan := chains.mailRowFor(limits, settings, m.SubscriptionID, m.PlanID)
				switch h.kind {
				case "domain":
					body.Limit = outgoingDomainLimit(settings, plan)
				case "subscription":
					body.Limit = outgoingSubscriptionLimit(settings, plan)
				default:
					body.Limit = effectiveOutgoing(settings, plan, m, now).Limit
				}
			}
		}
		p.storeOutgoingNotice(ctx, outgoingNoticeLimit, h.address, h.hour, ref, body, false)
	}
}

// outgoingMedian is the median of the positive values (0 when none).
func outgoingMedian(values []int) int {
	var kept []int
	for _, v := range values {
		if v > 0 {
			kept = append(kept, v)
		}
	}
	if len(kept) == 0 {
		return 0
	}
	sort.Ints(kept)
	return kept[len(kept)/2]
}

// outgoingSpikeDecision says whether one hour is unusual for a mailbox:
// spike_factor times its normal (median active hour of the last 7 days) and
// at least spike_minimum recipients, or refusal_pause limit refusals. A
// mailbox with no active hour is judged by volume only once the panel has
// watched for a day (observed): when the limits were just turned on, every
// mailbox lacks a history, and a busy but ordinary one must not be paused.
func outgoingSpikeDecision(settings outgoingSettings, recipients, refusals int, history []int, observed bool) (pause bool, reason string, normal int) {
	normal = outgoingMedian(history)
	threshold := settings.SpikeFactor * normal
	if threshold < settings.SpikeMinimum {
		threshold = settings.SpikeMinimum
	}
	if (normal > 0 || observed) && recipients >= threshold {
		return true, "spike", normal
	}
	if settings.RefusalPause > 0 && refusals >= settings.RefusalPause {
		return true, "refusals", normal
	}
	return false, "", normal
}

// pauseUnusualSenders pauses a mailbox whose current or previous hour is
// unusual (owner decision 2: sending from it is refused permanently until its
// password changes, the administrator is told at once).
func (p *Panel) pauseUnusualSenders(ctx context.Context, settings outgoingSettings, now time.Time) {
	db := p.db.GetDB()
	recent := now.Truncate(time.Hour).Add(-time.Hour)
	rows, err := db.QueryContext(ctx, `
		SELECT h.address, h.hour, h.recipients, h.limit_refusals, a.id, COALESCE(mo.paused_at, ''), COALESCE(mo.resumed_at, '')
		FROM mail_outgoing_hourly h
		JOIN email_accounts a ON lower(a.address) = h.address
		LEFT JOIN mailbox_outgoing mo ON mo.email_account_id = a.id
		WHERE h.hour >= ? ORDER BY h.hour`, recent.Format(time.RFC3339))
	if err != nil {
		log.Printf("[mail-outgoing] spike read: %v", err)
		return
	}
	type candidate struct {
		address, hour, pausedAt, resumedAt string
		recipients, refusals, id           int
	}
	var candidates []candidate
	for rows.Next() {
		var c candidate
		if rows.Scan(&c.address, &c.hour, &c.recipients, &c.refusals, &c.id, &c.pausedAt, &c.resumedAt) == nil {
			candidates = append(candidates, c)
		}
	}
	rows.Close()
	var oldest string
	_ = db.QueryRowContext(ctx, `SELECT COALESCE(MIN(hour), '') FROM mail_outgoing_hourly`).Scan(&oldest)
	first, _ := time.Parse(time.RFC3339, oldest)
	observed := !first.IsZero() && now.Sub(first) >= 24*time.Hour
	paused := map[int]bool{}
	for _, c := range candidates {
		if c.pausedAt != "" || paused[c.id] {
			continue
		}
		hour, err := time.Parse(time.RFC3339, c.hour)
		if err != nil {
			continue
		}
		// Hours before the last password change were judged already.
		if resumed := parseSQLiteTime(c.resumedAt); !resumed.IsZero() && hour.Before(resumed.Truncate(time.Hour).Add(time.Hour)) {
			continue
		}
		history, err := outgoingHistory(ctx, db, c.address, hour.Add(-7*24*time.Hour), hour.Add(-time.Hour))
		if err != nil {
			continue
		}
		pause, reason, normal := outgoingSpikeDecision(settings, c.recipients, c.refusals, history, observed)
		if !pause {
			continue
		}
		detail := fmt.Sprintf("%d recipients in the hour from %s (normal hour: %d)", c.recipients, hour.Format("2006-01-02 15:04 UTC"), normal)
		if reason == "refusals" {
			detail = fmt.Sprintf("%d messages refused by the hourly limit in the hour from %s", c.refusals, hour.Format("2006-01-02 15:04 UTC"))
		}
		stamp := now.Format("2006-01-02 15:04:05")
		if _, err := db.ExecContext(ctx, `
			INSERT INTO mailbox_outgoing (email_account_id, paused_at, paused_reason, paused_detail, updated_at)
			VALUES (?, ?, ?, ?, datetime('now'))
			ON CONFLICT(email_account_id) DO UPDATE SET paused_at = excluded.paused_at, paused_reason = excluded.paused_reason,
			    paused_detail = excluded.paused_detail, updated_at = excluded.updated_at
			WHERE mailbox_outgoing.paused_at IS NULL`, c.id, stamp, reason, detail); err != nil {
			log.Printf("[mail-outgoing] pause %s: %v", c.address, err)
			continue
		}
		paused[c.id] = true
		log.Printf("[mail-outgoing] paused %s: %s", c.address, detail)
		if ref, ok := outgoingAccountByAddress(ctx, db, c.address); ok {
			p.storeOutgoingNotice(ctx, outgoingNoticePaused, c.address, stamp, ref,
				outgoingNoticeBody{Hour: c.hour, Reason: reason, Recipients: c.recipients, Refused: c.refusals, Normal: normal}, true)
		}
	}
	if len(paused) > 0 {
		// The pause is held by postfwd now, not at the next round.
		if _, _, err := p.applyOutgoingMail(ctx, false); err != nil {
			log.Printf("[mail-outgoing] apply after pause: %v", err)
		}
		if err := p.sendPendingOutgoingNotices(ctx); err != nil {
			log.Printf("[mail-outgoing] pause notices: %v", err)
		}
	}
}

func outgoingHistory(ctx context.Context, q sqlQueryer, address string, from, to time.Time) ([]int, error) {
	rows, err := q.QueryContext(ctx, `SELECT recipients FROM mail_outgoing_hourly WHERE address = ? AND hour >= ? AND hour < ?`,
		address, from.UTC().Format(time.RFC3339), to.UTC().Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// resumeOutgoingAfterPassword ends a pause once the mailbox has a new
// password (the only way out, owner decision 2) and has postfwd let it send.
func (p *Panel) resumeOutgoingAfterPassword(ctx context.Context, accountID int) (bool, error) {
	resumed, pending, err := p.resumeOutgoingAfterPasswordConfirmed(ctx, accountID)
	if err == nil && pending {
		err = errOutgoingResumeUnconfirmed
	}
	return resumed, err
}

// errOutgoingResumeUnconfirmed: the pause is lifted, but the running postfwd
// still answered the mailbox as paused when its check ended; it takes the
// new rules within minutes by itself.
var errOutgoingResumeUnconfirmed = errors.New("postfwd has not confirmed the resumed mailbox yet")

// resumeOutgoingAfterPasswordConfirmed also says whether postfwd still
// answered the mailbox as paused after the apply (pending).
func (p *Panel) resumeOutgoingAfterPasswordConfirmed(ctx context.Context, accountID int) (bool, bool, error) {
	result, err := p.db.GetDB().ExecContext(ctx, `
		UPDATE mailbox_outgoing SET paused_at = NULL, paused_reason = NULL, paused_detail = NULL,
		       resumed_at = datetime('now'), updated_at = datetime('now')
		WHERE email_account_id = ? AND paused_at IS NOT NULL`, accountID)
	if err != nil {
		return false, false, err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return false, false, nil
	}
	resp, _, err := p.applyOutgoingMail(ctx, false)
	if err != nil {
		return true, false, err
	}
	return true, len(resp.ResumeUnconfirmed) > 0, nil
}

// ---- Notices ----------------------------------------------------------------

func outgoingNoticeMail(kind, audience string, body outgoingNoticeBody, baseURL string) (string, string) {
	subject, _, _, en, tr := outgoingNoticeTexts(kind, body)
	if subject == "" {
		return "", ""
	}
	_ = audience
	return subject, strings.Join([]string{en, "", tr, "", baseURL}, "\n")
}

// outgoingNoticeTexts is one notice's words: the mail subject (both
// languages), the title in each language and the text in each language. The
// mail and the panel's one notifications list (notifications_handlers.go)
// read the same words.
func outgoingNoticeTexts(kind string, body outgoingNoticeBody) (subject, titleEN, titleTR, en, tr string) {
	who := body.Address
	limitWhatEN, limitWhatTR := "this mailbox", "bu posta kutusu"
	switch body.LimitKind {
	case "domain":
		limitWhatEN, limitWhatTR = "all mailboxes of this domain together", "bu alan adının bütün posta kutuları birlikte"
	case "subscription":
		limitWhatEN, limitWhatTR = "all mailboxes of this hosting account together", "bu barındırma hesabının bütün posta kutuları birlikte"
	}
	switch kind {
	case outgoingNoticeLimit:
		subject = "Hourly sending limit reached / Saatlik gönderim sınırı doldu: " + who
		titleEN, titleTR = "Hourly sending limit reached: "+who, "Saatlik gönderim sınırı doldu: "+who
		en = fmt.Sprintf("%s reached the hourly sending limit (%d recipients per hour for %s). Further messages were refused temporarily (%d so far); nothing is lost, the mail program sends them again when the hour is over. If this mailbox did not send that much, change its password now: someone else may be using it. A larger limit is set by the provider or administrator.", who, body.Limit, limitWhatEN, body.Refused)
		tr = fmt.Sprintf("%s saatlik gönderim sınırına ulaştı (%s için saatte %d alıcı). Sonraki iletiler geçici olarak reddedildi (şimdiye dek %d); hiçbiri kaybolmaz, saat dolunca posta programı yeniden gönderir. Bu kutu bu kadar göndermediyse parolasını hemen değiştirin: başkası kullanıyor olabilir. Daha yüksek sınırı sağlayıcı ya da yönetici verir.", who, limitWhatTR, body.Limit, body.Refused)
	case outgoingNoticePaused:
		subject = "Mailbox sending stopped for security / Posta kutusunun gönderimi güvenlik için durduruldu: " + who
		titleEN, titleTR = "Sending stopped for security: "+who, "Gönderim güvenlik için durduruldu: "+who
		why := fmt.Sprintf("it sent to %d recipients in one hour, far more than its normal hour (%d)", body.Recipients, body.Normal)
		whyTR := fmt.Sprintf("bir saatte %d alıcıya gönderdi; normal saatinin (%d) çok üstünde", body.Recipients, body.Normal)
		if body.Reason == "refusals" {
			why = fmt.Sprintf("its hourly limit refused %d messages in one hour", body.Refused)
			whyTR = fmt.Sprintf("saatlik sınırı bir saatte %d iletiyi reddetti", body.Refused)
		}
		en = fmt.Sprintf("Sending from %s was stopped for security because %s. This usually means someone else knows its password. Receiving mail keeps working. Every message it tries to send is refused until its password is changed; the mail program keeps the messages. Next step: change the mailbox password in the panel (Mail > the mailbox > Change password), then update it in the mail programs that use it. Sending reopens within a minute after the change.", who, why)
		tr = fmt.Sprintf("%s adresinden gönderim güvenlik için durduruldu, çünkü %s. Bu çoğunlukla parolasını başkasının bildiği anlamına gelir. Posta almak çalışmaya devam eder. Parola değişene dek göndermeye çalıştığı her ileti reddedilir; iletiler posta programında kalır. Sonraki adım: paneldeki posta kutusunun parolasını değiştirin (Posta > posta kutusu > Parolayı değiştir), sonra onu kullanan posta programlarında da güncelleyin. Değişiklikten sonra bir dakika içinde gönderim yeniden açılır.", who, whyTR)
	default:
		return "", "", "", "", ""
	}
	return subject, titleEN, titleTR, en, tr
}

func (p *Panel) sendPendingOutgoingNotices(ctx context.Context) error {
	db := p.db.GetDB()
	rows, err := db.QueryContext(ctx, `
		SELECT n.id, n.kind, n.audience, n.body, COALESCE(u.email, '')
		FROM mail_outgoing_notices n LEFT JOIN users u ON u.id = n.user_id
		WHERE n.mail_state = 'pending' ORDER BY n.id LIMIT 200`)
	if err != nil {
		return err
	}
	type pending struct {
		id                          int
		kind, audience, body, email string
	}
	var items []pending
	for rows.Next() {
		var item pending
		if err := rows.Scan(&item.id, &item.kind, &item.audience, &item.body, &item.email); err != nil {
			rows.Close()
			return err
		}
		items = append(items, item)
	}
	rows.Close()
	if err := rows.Err(); err != nil || len(items) == 0 {
		return err
	}
	setup := p.accountMailSetup(ctx)
	mark := func(id int, state, detail string) {
		if _, err := db.ExecContext(ctx, `UPDATE mail_outgoing_notices SET mail_state = ?, mail_detail = ? WHERE id = ?`, state, nullIfEmpty(detail), id); err != nil {
			log.Printf("[mail-outgoing] notice %d mail state: %v", id, err)
		}
	}
	for _, item := range items {
		if !setup.Available {
			mark(item.id, "not_available", setup.Reason)
			continue
		}
		if strings.TrimSpace(item.email) == "" {
			mark(item.id, "no_address", "")
			continue
		}
		var body outgoingNoticeBody
		if err := json.Unmarshal([]byte(item.body), &body); err != nil {
			mark(item.id, "failed", "the notice could not be read")
			continue
		}
		subject, text := outgoingNoticeMail(item.kind, item.audience, body, setup.baseURL)
		if subject == "" {
			mark(item.id, "failed", "unknown notice kind")
			continue
		}
		if err := p.sendAccountMail(ctx, setup, strings.TrimSpace(item.email), accountMailContent{kind: accountMailUsageNotice, subject: subject, body: text}); err != nil {
			mark(item.id, "failed", boundedAgentDiagnostic(err.Error()))
			continue
		}
		mark(item.id, "sent", "")
	}
	return nil
}

type outgoingNoticeView struct {
	ID        int                `json:"id"`
	Kind      string             `json:"kind"`
	Audience  string             `json:"audience"`
	CreatedAt string             `json:"created_at"`
	Body      outgoingNoticeBody `json:"body"`
}

func (p *Panel) handleOutgoingNotices(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	caller := currentCaller(r)
	if caller == nil {
		writeClientError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	db := p.db.GetDB()
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, mailOutgoingNoticesPath), "/")
	if rest == "" && r.Method == http.MethodGet {
		rows, err := db.QueryContext(r.Context(), `
			SELECT id, kind, audience, body, created_at FROM mail_outgoing_notices
			WHERE user_id = ? AND seen_at IS NULL ORDER BY created_at DESC, id DESC LIMIT 50`, caller.ID)
		if err != nil {
			writeServerError(w, err)
			return
		}
		defer rows.Close()
		views := []outgoingNoticeView{}
		for rows.Next() {
			var v outgoingNoticeView
			var raw string
			if err := rows.Scan(&v.ID, &v.Kind, &v.Audience, &raw, &v.CreatedAt); err != nil {
				writeServerError(w, err)
				return
			}
			if json.Unmarshal([]byte(raw), &v.Body) != nil {
				continue
			}
			views = append(views, v)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"notices": views})
		return
	}
	idText, action, _ := strings.Cut(rest, "/")
	var id int
	if _, err := fmt.Sscanf(idText, "%d", &id); err != nil || id <= 0 || fmt.Sprint(id) != idText || action != "seen" || r.Method != http.MethodPost {
		writeClientError(w, http.StatusNotFound, "not found")
		return
	}
	result, err := db.ExecContext(r.Context(), `UPDATE mail_outgoing_notices SET seen_at = COALESCE(seen_at, datetime('now')) WHERE id = ? AND user_id = ?`, id, caller.ID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if n, _ := result.RowsAffected(); n != 1 {
		writeClientError(w, http.StatusNotFound, "not found")
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]bool{"seen": true})
}

// ---- Administrator ----------------------------------------------------------

// outgoingAgentStatus is replaced by tests.
var outgoingAgentStatus = func(p *Panel, ctx context.Context) (transport.OutgoingMailLimitsStatus, error) {
	var status transport.OutgoingMailLimitsStatus
	err := p.callAgentContext(ctx, "Agent.OutgoingMailLimitsStatusV1", &transport.Empty{}, &status)
	return status, err
}

type outgoingPausedView struct {
	ID       int    `json:"id"`
	Address  string `json:"address"`
	DomainID int    `json:"domain_id"`
	Domain   string `json:"domain,omitempty"`
	PausedAt string `json:"paused_at"`
	Reason   string `json:"reason"`
	Detail   string `json:"detail,omitempty"`
}

type outgoingPlanView struct {
	ID     int              `json:"id"`
	Name   string           `json:"name"`
	Limits mailPlanLimitRow `json:"limits"`
}

type outgoingAdminView struct {
	Settings outgoingSettings                    `json:"settings"`
	Apply    outgoingApplyRecord                 `json:"apply"`
	Server   *transport.OutgoingMailLimitsStatus `json:"server,omitempty"`
	// ServerUnknown: the Agent could not be asked; the server state is
	// unknown, never "off".
	ServerUnknown string               `json:"server_unknown,omitempty"`
	Paused        []outgoingPausedView `json:"paused"`
	Plans         []outgoingPlanView   `json:"plans"`
}

func (p *Panel) outgoingAdminView(ctx context.Context) (outgoingAdminView, error) {
	settings, record, err := p.loadOutgoingSettings(ctx)
	if err != nil {
		return outgoingAdminView{}, err
	}
	view := outgoingAdminView{Settings: settings, Apply: record, Paused: []outgoingPausedView{}, Plans: []outgoingPlanView{}}
	if status, err := outgoingAgentStatus(p, ctx); err != nil {
		view.ServerUnknown = boundedAgentDiagnostic(err.Error())
	} else {
		view.Server = &status
	}
	db := p.db.GetDB()
	rows, err := db.QueryContext(ctx, `
		SELECT a.id, a.address, a.domain_id, d.name, mo.paused_at, COALESCE(mo.paused_reason, ''), COALESCE(mo.paused_detail, '')
		FROM mailbox_outgoing mo JOIN email_accounts a ON a.id = mo.email_account_id JOIN domains d ON d.id = a.domain_id
		WHERE mo.paused_at IS NOT NULL ORDER BY mo.paused_at DESC LIMIT 200`)
	if err != nil {
		return view, err
	}
	for rows.Next() {
		var v outgoingPausedView
		if err := rows.Scan(&v.ID, &v.Address, &v.DomainID, &v.Domain, &v.PausedAt, &v.Reason, &v.Detail); err != nil {
			rows.Close()
			return view, err
		}
		view.Paused = append(view.Paused, v)
	}
	rows.Close()
	limits, err := loadMailPlanLimits(ctx, db)
	if err != nil {
		return view, err
	}
	planRows, err := db.QueryContext(ctx, `SELECT id, name FROM service_plans ORDER BY name, id LIMIT 500`)
	if err != nil {
		return view, err
	}
	defer planRows.Close()
	for planRows.Next() {
		var v outgoingPlanView
		if err := planRows.Scan(&v.ID, &v.Name); err != nil {
			return view, err
		}
		v.Limits = limits[mailPlanLimitKey{"plan", v.ID}]
		view.Plans = append(view.Plans, v)
	}
	return view, planRows.Err()
}

// handleMailOutgoing: GET the administrator's view; POST save and apply.
func (p *Panel) handleMailOutgoing(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if r.URL.Path == mailOutgoingReportPath {
		p.handleMailOutgoingReport(w, r)
		return
	}
	if r.URL.Path != mailOutgoingPath {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		view, err := p.outgoingAdminView(r.Context())
		if err != nil {
			writeServerError(w, err)
			return
		}
		_ = json.NewEncoder(w).Encode(view)
	case http.MethodPost:
		var request outgoingSettings
		if err := decodeStrictJSON(w, r, &request); err != nil {
			writeClientError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if err := request.validate(); err != nil {
			writeClientError(w, http.StatusBadRequest, err.Error())
			return
		}
		ctx := r.Context()
		reportOperationPlan(ctx, "save", "apply")
		reportOperationStep(ctx, "save")
		enabled := 0
		if request.Enabled {
			enabled = 1
		}
		if _, err := p.db.GetDB().ExecContext(ctx, `
			UPDATE mail_outgoing_settings SET enabled = ?, mailbox_per_hour = ?, domain_per_hour = ?, subscription_per_hour = ?,
			       new_mailbox_per_hour = ?, new_mailbox_hours = ?, spike_factor = ?, spike_minimum = ?, refusal_pause = ?,
			       updated_at = datetime('now')
			WHERE id = 1`, enabled, request.MailboxPerHour, request.DomainPerHour, request.SubscriptionPerHour,
			request.NewMailboxPerHour, request.NewMailboxHours, request.SpikeFactor, request.SpikeMinimum, request.RefusalPause); err != nil {
			writeServerError(w, err)
			return
		}
		p.audit(r, "mail.outgoing.settings", "mail_outgoing", 1)
		resp, called, err := p.applyOutgoingMail(ctx, true)
		answer := map[string]any{"saved": true, "called": called, "result": resp}
		if err != nil {
			answer["unknown"] = "the server could not be asked to take the settings; they are saved and tried again within five minutes"
		}
		_ = json.NewEncoder(w).Encode(answer)
	default:
		w.Header().Set("Allow", "GET, POST")
		writeClientError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

type outgoingReportRow struct {
	Address    string `json:"address"`
	DomainID   int    `json:"domain_id,omitempty"`
	Messages   int    `json:"messages"`
	Recipients int    `json:"recipients"`
	Limit      int    `json:"limit_refusals"`
	Paused     int    `json:"paused_refusals"`
	Mismatch   int    `json:"mismatch_refusals"`
}

func (p *Panel) handleMailOutgoingReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		writeClientError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	days := 1
	switch r.URL.Query().Get("days") {
	case "", "1":
	case "7":
		days = 7
	case "30":
		days = 30
	default:
		writeClientError(w, http.StatusBadRequest, "days must be 1, 7 or 30")
		return
	}
	since := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour).Truncate(time.Hour).Format(time.RFC3339)
	rows, err := p.db.GetDB().QueryContext(r.Context(), `
		SELECT h.address, COALESCE(MAX(a.domain_id), 0), SUM(h.messages), SUM(h.recipients), SUM(h.limit_refusals),
		       SUM(h.paused_refusals), SUM(h.mismatch_refusals)
		FROM mail_outgoing_hourly h LEFT JOIN email_accounts a ON lower(a.address) = h.address
		WHERE h.hour >= ? GROUP BY h.address
		ORDER BY SUM(h.recipients) DESC, SUM(h.limit_refusals + h.paused_refusals + h.mismatch_refusals) DESC LIMIT 100`, since)
	if err != nil {
		writeServerError(w, err)
		return
	}
	defer rows.Close()
	out := []outgoingReportRow{}
	var total outgoingReportRow
	for rows.Next() {
		var row outgoingReportRow
		if err := rows.Scan(&row.Address, &row.DomainID, &row.Messages, &row.Recipients, &row.Limit, &row.Paused, &row.Mismatch); err != nil {
			writeServerError(w, err)
			return
		}
		total.Messages += row.Messages
		total.Recipients += row.Recipients
		total.Limit += row.Limit
		total.Paused += row.Paused
		total.Mismatch += row.Mismatch
		out = append(out, row)
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"days": days, "since": since, "senders": out, "total": total})
}

// ---- One domain's mailboxes -------------------------------------------------

type outgoingMailboxView struct {
	ID        int                 `json:"id"`
	Address   string              `json:"address"`
	Effective outgoingEffective   `json:"effective"`
	UsedHour  int                 `json:"used_hour"`
	Sent7d    int                 `json:"sent_7d"`
	Refused7d int                 `json:"refused_7d"`
	Paused    *outgoingPausedView `json:"paused,omitempty"`
}

type outgoingDomainView struct {
	// Enforced: the limits are on and the server proved it counts.
	Enforced bool `json:"enforced"`
	// State: off (the administrator has not turned them on), pending, not_enforced or on.
	State             string                `json:"state"`
	Reason            string                `json:"reason,omitempty"`
	DomainLimit       int                   `json:"domain_limit"`
	DomainUsedHour    int                   `json:"domain_used_hour"`
	SubscriptionLimit int                   `json:"subscription_limit"`
	Ceiling           int                   `json:"ceiling"`
	Default           int                   `json:"default"`
	MayExceedCeiling  bool                  `json:"may_exceed_ceiling"`
	Mailboxes         []outgoingMailboxView `json:"mailboxes"`
}

func (p *Panel) handleDomainMailOutgoing(w http.ResponseWriter, r *http.Request, domainID int) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	switch r.Method {
	case http.MethodGet:
		view, err := p.outgoingDomainView(r.Context(), domainID, currentCaller(r))
		if err != nil {
			writeServerError(w, err)
			return
		}
		_ = json.NewEncoder(w).Encode(view)
	case http.MethodPut:
		p.handleDomainMailOutgoingSet(w, r, domainID)
	default:
		w.Header().Set("Allow", "GET, PUT")
		writeClientError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (p *Panel) outgoingDomainView(ctx context.Context, domainID int, caller *Caller) (outgoingDomainView, error) {
	db := p.db.GetDB()
	settings, record, err := p.loadOutgoingSettings(ctx)
	if err != nil {
		return outgoingDomainView{}, err
	}
	mailboxes, err := loadOutgoingMailboxes(ctx, db, domainID)
	if err != nil {
		return outgoingDomainView{}, err
	}
	limits, err := loadMailPlanLimits(ctx, db)
	if err != nil {
		return outgoingDomainView{}, err
	}
	view := outgoingDomainView{Mailboxes: []outgoingMailboxView{}, MayExceedCeiling: caller != nil && caller.Role == roleAdmin}
	var subID int
	var planID sql.NullInt64
	if err := db.QueryRowContext(ctx, `SELECT d.subscription_id, s.plan_id FROM domains d JOIN subscriptions s ON s.id = d.subscription_id WHERE d.id = ?`,
		domainID).Scan(&subID, &planID); err != nil {
		return view, err
	}
	plan := newMailLimitChains(ctx, db).mailRowFor(limits, settings, subID, planID)
	view.Ceiling, view.Default = outgoingCeilingAndDefault(settings, plan)
	view.DomainLimit = outgoingDomainLimit(settings, plan)
	view.SubscriptionLimit = outgoingSubscriptionLimit(settings, plan)
	switch {
	case !settings.Enabled:
		view.State = "off"
	case record.State == "applied" && record.VerifiedAt != "":
		view.State, view.Enforced = "on", true
	case record.State == "":
		view.State = "pending"
	default:
		view.State = "not_enforced"
		view.Reason = record.Code
	}
	now := time.Now().UTC()
	hour := now.Truncate(time.Hour).Format(time.RFC3339)
	weekAgo := now.Add(-7 * 24 * time.Hour).Format(time.RFC3339)
	for _, m := range mailboxes {
		item := outgoingMailboxView{ID: m.ID, Address: m.Address, Effective: effectiveOutgoing(settings, plan, m, now)}
		_ = db.QueryRowContext(ctx, `SELECT COALESCE(SUM(recipients), 0) FROM mail_outgoing_hourly WHERE address = ? AND hour = ?`, m.Address, hour).Scan(&item.UsedHour)
		_ = db.QueryRowContext(ctx, `SELECT COALESCE(SUM(recipients), 0), COALESCE(SUM(limit_refusals + paused_refusals), 0) FROM mail_outgoing_hourly WHERE address = ? AND hour >= ?`,
			m.Address, weekAgo).Scan(&item.Sent7d, &item.Refused7d)
		view.DomainUsedHour += item.UsedHour
		if m.Paused {
			item.Paused = &outgoingPausedView{ID: m.ID, Address: m.Address, DomainID: m.DomainID, Domain: m.Domain, PausedAt: m.PausedAt, Reason: m.PausedReason, Detail: m.PausedDetail}
		}
		view.Mailboxes = append(view.Mailboxes, item)
	}
	return view, nil
}

// handleDomainMailOutgoingSet sets one mailbox's value: default, a number of
// recipients per hour, or no limit. A reseller or customer stays within the
// ceiling (no limit only when the ceiling is none); the administrator may go
// above it.
func (p *Panel) handleDomainMailOutgoingSet(w http.ResponseWriter, r *http.Request, domainID int) {
	var request struct {
		ID      int    `json:"id"`
		Mode    string `json:"mode"`
		PerHour int    `json:"per_hour"`
	}
	if err := decodeStrictJSON(w, r, &request); err != nil || request.ID <= 0 {
		writeClientError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	ctx := r.Context()
	view, err := p.outgoingDomainView(ctx, domainID, currentCaller(r))
	if err != nil {
		writeServerError(w, err)
		return
	}
	found := false
	for _, m := range view.Mailboxes {
		if m.ID == request.ID {
			found = true
		}
	}
	if !found {
		writeClientError(w, http.StatusNotFound, "mailbox not found")
		return
	}
	var value any
	switch request.Mode {
	case "default":
		value = nil
	case "unlimited":
		if !view.MayExceedCeiling && view.Ceiling != 0 {
			writeCodedError(w, http.StatusForbidden, errCodeOutgoingAboveCeiling,
				fmt.Sprintf("No limit is not allowed by this hosting plan; the most is %d recipients per hour. The provider or administrator can raise the plan's limit.", view.Ceiling), "")
			return
		}
		value = 0
	case "custom":
		if request.PerHour < 1 || request.PerHour > transport.OutgoingMailMaxPerHour {
			writeClientError(w, http.StatusBadRequest, fmt.Sprintf("the limit must be between 1 and %d recipients per hour", transport.OutgoingMailMaxPerHour))
			return
		}
		if !view.MayExceedCeiling && view.Ceiling != 0 && request.PerHour > view.Ceiling {
			writeCodedError(w, http.StatusForbidden, errCodeOutgoingAboveCeiling,
				fmt.Sprintf("This hosting plan allows at most %d recipients per hour for one mailbox. Choose %d or less, or ask the provider or administrator to raise the plan's limit.", view.Ceiling, view.Ceiling), "")
			return
		}
		value = request.PerHour
	default:
		writeClientError(w, http.StatusBadRequest, "mode must be default, custom or unlimited")
		return
	}
	if _, err := p.db.GetDB().ExecContext(ctx, `
		INSERT INTO mailbox_outgoing (email_account_id, per_hour, updated_at) VALUES (?, ?, datetime('now'))
		ON CONFLICT(email_account_id) DO UPDATE SET per_hour = excluded.per_hour, updated_at = excluded.updated_at`,
		request.ID, value); err != nil {
		writeServerError(w, err)
		return
	}
	p.audit(r, "mail.outgoing.mailbox", "email_account", request.ID)
	resp, called, err := p.applyOutgoingMail(ctx, false)
	answer := map[string]any{"saved": true, "called": called, "applied": resp.Applied}
	if err != nil {
		answer["unknown"] = true
	} else if called && !resp.Applied {
		answer["code"] = resp.Code
	} else if !called && !resp.Applied {
		// Not sent now: the limits are off, or a failed state waits for
		// its retry; the screen says which instead of a silent "saved".
		if settings, _, err := p.loadOutgoingSettings(ctx); err == nil && !settings.Enabled {
			answer["off"] = true
		} else {
			answer["pending"] = true
		}
	}
	_ = json.NewEncoder(w).Encode(answer)
}
