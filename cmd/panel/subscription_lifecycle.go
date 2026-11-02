package main

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// Subscription lifecycle (K7, migration 052): ready for a future billing
// system. Every subscription has a start, an end that may be empty (never
// ends; ended only by hand) and a status (active, suspended, expired as in
// 001) with a suspension reason and time. This file only reads them; what a
// suspension does to services is a separate part (PLANS-AND-RIGHTS.md 9A).
//
// Abonelik yaşam döngüsü: başlangıç, boş olabilen bitiş, durum, askıya alma
// nedeni ve zamanı. Bu dosya yalnız okur; askının etkisi ayrı bir iştir.

const (
	suspendReasonManual   = "manual"
	suspendReasonExpired  = "expired"
	suspendReasonPayment  = "payment"
	suspendReasonSecurity = "security"

	lifecycleActive    = "active"
	lifecycleSuspended = "suspended"
	lifecycleEnded     = "ended"
)

var suspendReasons = map[string]bool{
	suspendReasonManual: true, suspendReasonExpired: true, suspendReasonPayment: true, suspendReasonSecurity: true,
}

func validSuspendReason(reason string) bool { return suspendReasons[reason] }

// subscriptionLifecycle is the stored lifecycle of one subscription.
type subscriptionLifecycle struct {
	Status        string     `json:"status"`
	StartsAt      *time.Time `json:"starts_at"`
	EndsAt        *time.Time `json:"ends_at"`
	SuspendReason string     `json:"suspend_reason,omitempty"`
	SuspendedAt   *time.Time `json:"suspended_at,omitempty"`
}

// parseLifecycleTime reads the timestamp layouts SQLite and Go write. An
// unreadable value is nil (unknown), never "now".
func parseLifecycleTime(raw sql.NullString) *time.Time {
	if !raw.Valid || strings.TrimSpace(raw.String) == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05", "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, strings.TrimSpace(raw.String)); err == nil {
			utc := t.UTC()
			return &utc
		}
	}
	return nil
}

// lifecycleFromRow is the pure reading of the stored columns. starts_at
// falls back to created_at (rows made before 052 or by a path that does not
// write it); a suspended row without a reason reads as manual.
func lifecycleFromRow(status, startsAt, createdAt, endsAt, suspendReason, suspendedAt sql.NullString) subscriptionLifecycle {
	l := subscriptionLifecycle{Status: lifecycleActive}
	switch status.String {
	case "suspended", "expired":
		l.Status = status.String
	}
	l.StartsAt = parseLifecycleTime(startsAt)
	if l.StartsAt == nil {
		l.StartsAt = parseLifecycleTime(createdAt)
	}
	l.EndsAt = parseLifecycleTime(endsAt)
	if l.Status != lifecycleActive {
		l.SuspendReason = suspendReason.String
		if !validSuspendReason(l.SuspendReason) {
			l.SuspendReason = suspendReasonManual
			if l.Status == "expired" {
				l.SuspendReason = suspendReasonExpired
			}
		}
		l.SuspendedAt = parseLifecycleTime(suspendedAt)
	}
	return l
}

// state says where the subscription stands at now: "active", "suspended" or
// "ended" (its end date has passed or it is stored as expired). An end date
// that has passed does not suspend anything by itself here.
func (l subscriptionLifecycle) state(now time.Time) string {
	switch {
	case l.Status == "expired":
		return lifecycleEnded
	case l.Status == lifecycleSuspended:
		return lifecycleSuspended
	case l.EndsAt != nil && !now.Before(*l.EndsAt):
		return lifecycleEnded
	}
	return lifecycleActive
}

var errLifecycleSubscriptionMissing = errors.New("subscription not found")

// readSubscriptionLifecycle reads one subscription's lifecycle.
func readSubscriptionLifecycle(ctx context.Context, q rowQueryer, subID int) (subscriptionLifecycle, error) {
	var status, startsAt, createdAt, endsAt, reason, suspendedAt sql.NullString
	err := q.QueryRowContext(ctx, `
		SELECT status, starts_at, created_at, ends_at, suspend_reason, suspended_at
		FROM subscriptions WHERE id = ?`, subID).Scan(&status, &startsAt, &createdAt, &endsAt, &reason, &suspendedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return subscriptionLifecycle{}, errLifecycleSubscriptionMissing
	}
	if err != nil {
		return subscriptionLifecycle{}, err
	}
	return lifecycleFromRow(status, startsAt, createdAt, endsAt, reason, suspendedAt), nil
}
