package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
)

// Subscription suspension (alpha.117, PLANS-AND-RIGHTS 9A, K6, K7).
//
// A subscription is suspended while it has at least one cause: a manual
// suspension by the administrator or its reseller, a suspension of its
// reseller, or its end date having passed. subscriptions.status is the one
// summary every other part reads ('suspended' or 'active'); it changes the
// moment a cause is accepted, and an operation (suspension_run.go) then
// converges the server's native configuration to it, with checkpoints.
//
// Abonelik, en az bir nedeni olduğu sürece askıdadır: elle askı, bayisinin
// askısı ya da bitiş tarihinin geçmesi. subscriptions.status herkesin okuduğu
// özettir; neden kabul edildiği an değişir, ardından bir işlem sunucunun yerel
// yapılandırmasını kontrol noktalarıyla ona getirir.

const (
	suspensionKindSuspend = "suspend"
	suspensionKindResume  = "resume"

	suspensionTriggerManual   = "manual"
	suspensionTriggerReseller = "reseller"
	suspensionTriggerExpired  = "expired"

	suspensionReasonMaxLen = 500
)

var (
	errSuspensionBusy          = errors.New("another suspension operation is running on this subscription")
	errSuspensionCauseReseller = errors.New("suspended because its reseller is suspended")
	errSuspensionNotFound      = errors.New("subscription not found")
)

// suspensionCauses is why a subscription is suspended.
type suspensionCauses struct {
	Manual     bool   `json:"manual"`
	ResellerID int    `json:"reseller_id,omitempty"`
	Reseller   string `json:"reseller,omitempty"`
	Expired    bool   `json:"expired"`
}

func (c suspensionCauses) any() bool { return c.Manual || c.ResellerID > 0 || c.Expired }

// suspensionCauseChange is one accepted change of one cause.
type suspensionCauseChange struct {
	// Set or clear the manual cause.
	Manual *bool
	// Set (id > 0) or clear (0) the reseller cause; nil leaves it.
	ResellerID *int
	// Set or clear the expiry cause; EndsAt is the end date it is about.
	Expired *bool
	EndsAt  string
	// WaiveExpiry records that an operator resumed after EndsAt passed.
	WaiveExpiry bool
	Reason      string
	ActorID     int
	Trigger     string
}

// suspensionRow is the stored cause row of one subscription.
type suspensionRow struct {
	Causes        suspensionCauses
	ExpiredEndsAt string
	WaivedEndsAt  string
	Reason        string
	SuspendedAt   string
}

type sqlQueryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func loadSuspensionRow(ctx context.Context, q sqlQueryer, subscriptionID int) (suspensionRow, error) {
	var row suspensionRow
	var reseller sql.NullInt64
	var expiredEnds, waived, reason, since sql.NullString
	err := q.QueryRowContext(ctx, `
		SELECT cause_manual, cause_reseller_id, cause_expired, expired_ends_at,
		       expiry_waived_ends_at, reason, suspended_at
		FROM subscription_suspensions WHERE subscription_id = ?`, subscriptionID).
		Scan(&row.Causes.Manual, &reseller, &row.Causes.Expired, &expiredEnds, &waived, &reason, &since)
	if errors.Is(err, sql.ErrNoRows) {
		return row, nil
	}
	if err != nil {
		return row, err
	}
	row.Causes.ResellerID = int(reseller.Int64)
	row.ExpiredEndsAt, row.WaivedEndsAt = expiredEnds.String, waived.String
	row.Reason, row.SuspendedAt = reason.String, since.String
	return row, nil
}

// subscriptionSuspended reports whether a subscription is suspended. Other
// features (file access) call it; it reads subscriptions.status only.
// subscriptionSuspended: abonelik askıda mı (yalnız subscriptions.status).
func (p *Panel) subscriptionSuspended(ctx context.Context, subscriptionID int) (bool, error) {
	var status sql.NullString
	err := p.db.GetDB().QueryRowContext(ctx,
		`SELECT status FROM subscriptions WHERE id = ?`, subscriptionID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return false, errSuspensionNotFound
	}
	if err != nil {
		return false, err
	}
	return subscriptionStatusSuspended(status.String), nil
}

// subscriptionStatusSuspended: 'expired' (the 001 value no release wrote)
// is treated as suspended too; anything but active serves nothing.
func subscriptionStatusSuspended(status string) bool {
	return status == "suspended" || status == "expired"
}

// subscriptionsHaveEndsAt reports whether subscriptions carries ends_at (it
// comes with the plans-and-rights migration; missing means "never ends").
func subscriptionsHaveEndsAt(ctx context.Context, q sqlQueryer) (bool, error) {
	rows, err := q.QueryContext(ctx, `PRAGMA table_info(subscriptions)`)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			cid        int
			name, kind string
			notNull    int
			def        sql.NullString
			pk         int
		)
		if err := rows.Scan(&cid, &name, &kind, &notNull, &def, &pk); err != nil {
			return false, err
		}
		if name == "ends_at" {
			return true, nil
		}
	}
	return false, rows.Err()
}

// subscriptionEndsAt is the subscription's end date, "" when it has none or
// the column does not exist yet.
func subscriptionEndsAt(ctx context.Context, q sqlQueryer, subscriptionID int) (string, error) {
	has, err := subscriptionsHaveEndsAt(ctx, q)
	if err != nil || !has {
		return "", err
	}
	var endsAt sql.NullString
	err = q.QueryRowContext(ctx, `SELECT ends_at FROM subscriptions WHERE id = ?`, subscriptionID).Scan(&endsAt)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return strings.TrimSpace(endsAt.String), err
}

// suspensionAccept is the result of accepting a cause change.
type suspensionAccept struct {
	OperationID string
	// Started: a new operation was created by this change.
	Started   bool
	Suspended bool
}

// acceptSuspensionChange stores one cause change of one subscription, sets
// subscriptions.status to match, and creates the converging operation when
// the wanted state differs from what the last operation converged to. An
// operation of the other kind still running makes a manual change refuse
// (errSuspensionBusy); a cascade or expiry change is stored anyway and the
// worker starts the following operation when the running one ends.
func (p *Panel) acceptSuspensionChange(ctx context.Context, subscriptionID int, change suspensionCauseChange, refuseWhenBusy bool) (suspensionAccept, error) {
	var result suspensionAccept
	tx, err := p.db.GetDB().BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	// Take the write lock first so the read below cannot go stale.
	if _, err := tx.ExecContext(ctx, `UPDATE subscriptions SET updated_at = updated_at WHERE id = ?`, subscriptionID); err != nil {
		return result, err
	}
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM subscriptions WHERE id = ?`, subscriptionID).Scan(&exists); err != nil {
		return result, err
	}
	if exists != 1 {
		return result, errSuspensionNotFound
	}
	row, err := loadSuspensionRow(ctx, tx, subscriptionID)
	if err != nil {
		return result, err
	}
	causes := row.Causes
	if change.Manual != nil {
		causes.Manual = *change.Manual
	}
	if change.ResellerID != nil {
		causes.ResellerID = *change.ResellerID
	}
	expiredEnds, waived := row.ExpiredEndsAt, row.WaivedEndsAt
	if change.Expired != nil {
		causes.Expired = *change.Expired
		if *change.Expired {
			expiredEnds = change.EndsAt
		} else {
			expiredEnds = ""
		}
	}
	if change.WaiveExpiry && change.EndsAt != "" {
		waived = change.EndsAt
	}
	want := causes.any()

	running, runningKind, err := runningSuspensionOperation(ctx, tx, subscriptionID)
	if err != nil {
		return result, err
	}
	wantKind := suspensionKindResume
	if want {
		wantKind = suspensionKindSuspend
	}
	if running != "" && runningKind != wantKind && refuseWhenBusy {
		return result, errSuspensionBusy
	}

	reason := row.Reason
	since := row.SuspendedAt
	if want && !row.Causes.any() {
		reason = strings.TrimSpace(change.Reason)
		since = ""
	} else if want && strings.TrimSpace(change.Reason) != "" {
		reason = strings.TrimSpace(change.Reason)
	}
	if !want {
		reason, since = "", ""
	}
	if len(reason) > suspensionReasonMaxLen {
		reason = reason[:suspensionReasonMaxLen]
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO subscription_suspensions (subscription_id, cause_manual, cause_reseller_id,
			cause_expired, expired_ends_at, expiry_waived_ends_at, reason, suspended_at, suspended_by, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, CASE WHEN ? THEN COALESCE(NULLIF(?, ''), datetime('now')) END, ?, datetime('now'))
		ON CONFLICT(subscription_id) DO UPDATE SET
			cause_manual = excluded.cause_manual, cause_reseller_id = excluded.cause_reseller_id,
			cause_expired = excluded.cause_expired, expired_ends_at = excluded.expired_ends_at,
			expiry_waived_ends_at = excluded.expiry_waived_ends_at, reason = excluded.reason,
			suspended_at = excluded.suspended_at, suspended_by = excluded.suspended_by,
			updated_at = excluded.updated_at`,
		subscriptionID, causes.Manual, nullablePositiveInt(causes.ResellerID), causes.Expired,
		nullIfEmpty(expiredEnds), nullIfEmpty(waived), nullIfEmpty(reason), want, since,
		nullablePositiveInt(change.ActorID)); err != nil {
		return result, err
	}
	status := "active"
	if want {
		status = "suspended"
	}
	if _, err := tx.ExecContext(ctx, `UPDATE subscriptions SET status = ?, updated_at = datetime('now') WHERE id = ?`, status, subscriptionID); err != nil {
		return result, err
	}
	result.Suspended = want
	if running != "" {
		result.OperationID = running
	} else {
		lastKind, err := lastSuspensionOperationKind(ctx, tx, subscriptionID)
		if err != nil {
			return result, err
		}
		// Nothing was ever suspended: an active subscription needs no resume.
		if lastKind != wantKind && !(lastKind == "" && wantKind == suspensionKindResume) {
			trigger := change.Trigger
			if trigger == "" {
				trigger = suspensionTriggerManual
			}
			id, err := p.createSuspensionOperation(ctx, tx, subscriptionID, wantKind, trigger, change.ActorID)
			if err != nil {
				return result, err
			}
			result.OperationID, result.Started = id, true
		}
	}
	if err := tx.Commit(); err != nil {
		return result, err
	}
	p.kickSuspensionWorker()
	return result, nil
}

func nullIfEmpty(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func runningSuspensionOperation(ctx context.Context, q sqlQueryer, subscriptionID int) (id, kind string, err error) {
	err = q.QueryRowContext(ctx, `
		SELECT id, kind FROM suspension_operations
		WHERE subscription_id = ? AND state = 'running'`, subscriptionID).Scan(&id, &kind)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", nil
	}
	return id, kind, err
}

func lastSuspensionOperationKind(ctx context.Context, q sqlQueryer, subscriptionID int) (string, error) {
	var kind string
	err := q.QueryRowContext(ctx, `
		SELECT kind FROM suspension_operations WHERE subscription_id = ?
		ORDER BY created_at DESC, rowid DESC LIMIT 1`, subscriptionID).Scan(&kind)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return kind, err
}

// reconcileSuspensionOperations starts the operation a subscription needs
// when its wanted state differs from its last operation and nothing runs
// (after a cascade or expiry change met a running operation). A failed
// operation of the wanted kind is not restarted: an operator continues it.
func (p *Panel) reconcileSuspensionOperations(ctx context.Context) error {
	rows, err := p.db.GetDB().QueryContext(ctx, `
		SELECT s.id, s.status,
		       (SELECT o.kind FROM suspension_operations o WHERE o.subscription_id = s.id
		        ORDER BY o.created_at DESC, o.rowid DESC LIMIT 1)
		FROM subscriptions s
		WHERE NOT EXISTS (SELECT 1 FROM suspension_operations r WHERE r.subscription_id = s.id AND r.state = 'running')
		  AND (s.status IN ('suspended', 'expired') OR EXISTS (SELECT 1 FROM suspension_operations o2 WHERE o2.subscription_id = s.id))`)
	if err != nil {
		return err
	}
	type need struct {
		id   int
		kind string
	}
	var needs []need
	for rows.Next() {
		var id int
		var status string
		var last sql.NullString
		if err := rows.Scan(&id, &status, &last); err != nil {
			rows.Close()
			return err
		}
		want := suspensionKindResume
		if subscriptionStatusSuspended(status) {
			want = suspensionKindSuspend
		}
		if last.String != want && !(last.String == "" && want == suspensionKindResume) {
			needs = append(needs, need{id, want})
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, n := range needs {
		tx, err := p.db.GetDB().BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		running, _, err := runningSuspensionOperation(ctx, tx, n.id)
		if err == nil && running == "" {
			trigger := suspensionTriggerManual
			if row, rowErr := loadSuspensionRow(ctx, tx, n.id); rowErr == nil {
				switch {
				case row.Causes.Expired && !row.Causes.Manual && row.Causes.ResellerID == 0:
					trigger = suspensionTriggerExpired
				case row.Causes.ResellerID > 0 && !row.Causes.Manual:
					trigger = suspensionTriggerReseller
				}
			}
			_, err = p.createSuspensionOperation(ctx, tx, n.id, n.kind, trigger, 0)
		}
		if err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// checkSubscriptionExpiry suspends every subscription whose end date has
// passed (once per end date: an operator's resume waives that date) and
// clears the expiry cause of one whose end date moved into the future or was
// removed. Without an ends_at column nothing ever expires (K7: infinite).
func (p *Panel) checkSubscriptionExpiry(ctx context.Context) error {
	db := p.db.GetDB()
	has, err := subscriptionsHaveEndsAt(ctx, db)
	if err != nil || !has {
		return err
	}
	type item struct {
		id      int
		endsAt  string
		expired bool
		passed  bool
		waived  string
	}
	rows, err := db.QueryContext(ctx, `
		SELECT s.id, COALESCE(s.ends_at, ''),
		       COALESCE(ss.cause_expired, 0),
		       CASE WHEN s.ends_at IS NOT NULL AND julianday(s.ends_at) IS NOT NULL
		                 AND julianday(s.ends_at) <= julianday('now') THEN 1 ELSE 0 END,
		       COALESCE(ss.expiry_waived_ends_at, '')
		FROM subscriptions s
		LEFT JOIN subscription_suspensions ss ON ss.subscription_id = s.id`)
	if err != nil {
		return err
	}
	var items []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.id, &it.endsAt, &it.expired, &it.passed, &it.waived); err != nil {
			rows.Close()
			return err
		}
		items = append(items, it)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	var errs []error
	for _, it := range items {
		switch {
		case it.passed && !it.expired && it.waived != it.endsAt:
			yes := true
			_, err := p.acceptSuspensionChange(ctx, it.id, suspensionCauseChange{
				Expired: &yes, EndsAt: it.endsAt, Trigger: suspensionTriggerExpired,
				Reason: "The subscription's end date (" + it.endsAt + ") has passed.",
			}, false)
			if err != nil {
				errs = append(errs, fmt.Errorf("subscription %d: %w", it.id, err))
			}
		case it.expired && !it.passed:
			no := false
			_, err := p.acceptSuspensionChange(ctx, it.id, suspensionCauseChange{
				Expired: &no, Trigger: suspensionTriggerExpired,
			}, false)
			if err != nil {
				errs = append(errs, fmt.Errorf("subscription %d: %w", it.id, err))
			}
		}
	}
	return errors.Join(errs...)
}

// suspensionWorkerState is the in-process worker: one goroutine runs the
// operations one at a time, so two operations never touch one resource
// together.
type suspensionWorkerState struct {
	once sync.Once
	kick chan struct{}
	mu   sync.Mutex
}
