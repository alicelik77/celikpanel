package main

import (
	"context"
	"errors"
	"sync"
	"time"
)

// The Panel's own database held busy for longer than its busy timeout
// (alpha.123 lab, Debian 13, 2026-11-05).
//
// Another SQLite writer held the panel database's write lock for 60 s. The
// pool's busy_timeout (5 s) ran out on every write in that time, so:
//   - setup's own "waiting" state could not be saved, and its screen kept
//     saying "running" for the whole minute (the wait existed only in the
//     runner's memory);
//   - one audit row (dns.engine.switch.setup.reconcile_required) was written
//     once, under a 5 s deadline, and dropped with a log line.
//
// Two things change. A wait that cannot be saved is still shown: the runner
// notes it here, in memory, and the status reads lay it over the saved
// "running" row (readers are not blocked by a writer in WAL mode). And the
// writes that are persisted evidence are retried with a bounded backoff
// instead of being tried once: an operation's own state (this file) and the
// audit trail (audit_durable.go). Past the bound the write is a verified,
// visible failure with the owner's next action; it is never silently dropped
// (RESILIENCE-CONTRACT invariant: persisted evidence is never lost and never
// replaced by a guess).
//
// Panelin kendi veritabanı meşgul zaman aşımından uzun süre kilitli kalırsa:
// kaydedilemeyen bekleme bellekte tutulup ekrana yansıtılır; kalıcı kanıt
// yazımları sınırlı artan aralıklarla yeniden denenir, asla sessizce düşmez.

// panelBusyWait is one wait the runner could not save: since when, and
// whether it went past its bound.
type panelBusyWait struct {
	Since  time.Time
	Stayed bool
}

// panelBusyWaits holds the waits that are only in memory, by key
// ("setup:<execution id>", "op:<service operation id>").
var panelBusyWaits sync.Map

func panelBusyKeySetup(executionID string) string     { return "setup:" + executionID }
func panelBusyKeyOperation(operationID string) string { return "op:" + operationID }

// notePanelBusy records that the write of key met the panel database busy.
// The first note of an episode keeps its time.
func notePanelBusy(key string, stayed bool) {
	if key == "" || key == "setup:" || key == "op:" {
		return
	}
	now := time.Now()
	for {
		previous, loaded := panelBusyWaits.LoadOrStore(key, panelBusyWait{Since: now, Stayed: stayed})
		if !loaded {
			return
		}
		wait := previous.(panelBusyWait)
		next := panelBusyWait{Since: wait.Since, Stayed: wait.Stayed || stayed}
		if next == wait || panelBusyWaits.CompareAndSwap(key, previous, next) {
			return
		}
	}
}

// clearPanelBusy ends the episode of key: its state was saved.
func clearPanelBusy(key string) { panelBusyWaits.Delete(key) }

// panelBusyNoted reports the unsaved wait of key, if there is one.
func panelBusyNoted(key string) (panelBusyWait, bool) {
	value, ok := panelBusyWaits.Load(key)
	if !ok {
		return panelBusyWait{}, false
	}
	return value.(panelBusyWait), true
}

// Bounds of the retried evidence writes. Variables only so tests can make
// them short; production never changes them.
var (
	// panelBusyWriteBound: how long one state write is retried while the
	// database answers busy (setup's own bound is about ten minutes too).
	panelBusyWriteBound = 10 * time.Minute
	// panelBusyWriteFirstDelay / panelBusyWriteMaxDelay: the backoff.
	panelBusyWriteFirstDelay = 250 * time.Millisecond
	panelBusyWriteMaxDelay   = 5 * time.Second
	// panelBusyAttemptTimeout bounds one attempt. It is longer than the
	// pool's busy_timeout (5 s), so a held lock answers SQLITE_BUSY instead
	// of an expired context that would hide the reason.
	panelBusyAttemptTimeout = 8 * time.Second
)

// errPanelStayedBusy marks a write that still met the database busy when its
// bound ran out.
var errPanelStayedBusy = errors.New("the panel database stayed busy")

// retryableEvidenceWrite reports whether a failed write is worth asking
// again: the database answered busy or locked, or the attempt's own deadline
// expired while the caller's context is still alive (a lock held past the
// attempt's bound).
func retryableEvidenceWrite(parent, attempt context.Context, err error) bool {
	if panelDatabaseBusy(err) {
		return true
	}
	return !errors.Is(parent.Err(), context.Canceled) && attempt.Err() != nil &&
		(errors.Is(err, context.DeadlineExceeded) || errors.Is(attempt.Err(), context.DeadlineExceeded))
}

// retryPanelBusyWrite runs write until it succeeds, fails for another reason,
// the caller's context ends, or panelBusyWriteBound passes. While it waits,
// key carries the unsaved wait for the status reads. Each attempt has its own
// deadline that the caller's shorter one does not cut: a terminal write must
// outlive a lock held longer than the caller's 15 s, or the operation's
// result is lost (the alpha.123 lab). The caller's deadline does not end
// the retries either (the bound does); cancelling the caller's context (a
// lost Agent lease, a stop) does.
func retryPanelBusyWrite(ctx context.Context, key string, write func(context.Context) error) error {
	deadline := time.Now().Add(panelBusyWriteBound)
	delay := panelBusyWriteFirstDelay
	base := context.WithoutCancel(ctx)
	for {
		attempt, cancel := context.WithTimeout(base, panelBusyAttemptTimeout)
		err := write(attempt)
		retry := err != nil && retryableEvidenceWrite(ctx, attempt, err)
		cancel()
		if err == nil {
			if key != "" {
				clearPanelBusy(key)
			}
			return nil
		}
		if !retry {
			return err
		}
		if !time.Now().Add(delay).Before(deadline) {
			if key != "" {
				notePanelBusy(key, true)
			}
			return errors.Join(errPanelStayedBusy, err)
		}
		if key != "" {
			notePanelBusy(key, false)
		}
		if errors.Is(ctx.Err(), context.Canceled) {
			return errors.Join(ctx.Err(), err)
		}
		timer := time.NewTimer(delay)
		var done <-chan struct{}
		if ctx.Err() == nil {
			done = ctx.Done()
		}
		select {
		case <-done:
			if errors.Is(ctx.Err(), context.Canceled) {
				timer.Stop()
				return errors.Join(ctx.Err(), err)
			}
			<-timer.C
		case <-timer.C:
		}
		delay *= 2
		if delay > panelBusyWriteMaxDelay {
			delay = panelBusyWriteMaxDelay
		}
	}
}

// serviceOperationPanelBusyPhase / ...StayedPhase are the phases a running
// operation is shown in while its own state cannot be saved. They exist only
// in the read answer; the saved row keeps its real phase.
const (
	serviceOperationPanelBusyPhase       = "panel_busy"
	serviceOperationPanelStayedBusyPhase = "panel_stayed_busy"
)

// overlayServiceOperationPanelBusy lays an unsaved wait over a running
// operation read from the database.
func overlayServiceOperationPanelBusy(op serviceOperation) serviceOperation {
	if op.Status != serviceOperationRunning && op.Status != serviceOperationQueued {
		return op
	}
	wait, ok := panelBusyNoted(panelBusyKeyOperation(op.ID))
	if !ok {
		return op
	}
	if wait.Stayed {
		op.Phase = serviceOperationPanelStayedBusyPhase
	} else {
		op.Phase = serviceOperationPanelBusyPhase
	}
	return op
}

// overlayServerSetupPanelBusy lays an unsaved wait over the run read from the
// database: the screen says why it waits, who acts (nobody, or the owner past
// the bound) and how it goes on, instead of "running".
func overlayServerSetupPanelBusy(execution *serverSetupExecution) {
	if execution == nil || (execution.Status != "running" && execution.Status != "waiting") {
		return
	}
	wait, ok := panelBusyNoted(panelBusyKeySetup(execution.ID))
	if !ok {
		return
	}
	execution.Status = "waiting"
	if wait.Stayed {
		execution.Error = &serviceOperationError{Code: serverSetupPanelStayedBusyCode, Message: serverSetupPanelStayedBusyText}
		return
	}
	execution.Error = &serviceOperationError{Code: serverSetupPanelBusyCode, Message: serverSetupPanelBusyMessage}
}
