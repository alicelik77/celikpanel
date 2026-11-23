package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"
)

// A durable operation whose end the Panel could not record (alpha.145 lab,
// D1, 23 November 2026).
//
// While an operation's Agent step ran (a webmail update in the lab), the
// Panel's worker was stopped and the Agent finished the step anyway. The
// Panel's Finish was refused ("still has an active privileged step"), it
// polled for 15 seconds and returned without settling its own row. The row
// stayed "running": the page said "in progress" for ever and every other
// server change was refused as "another operation is running" until the
// panel restarted, when startup recovery settled it.
//
// Now the same goroutine settles the row. It reads the Agent's job, without
// changing it, until the job is no longer active (bounded by the job's own
// deadline, not by 15 seconds), then runs for this one row the
// reconciliation startup recovery runs (reconcileInterruptedServiceOperation-
// Locked: the same code). Its outcome is startup's: a succeeded job is
// recorded as succeeded, a failed one with its reason, and a job that was
// only interrupted is resumed exactly as startup resumes it (a webmail update
// or a webmail address is only read again; an install, a removal or a mail
// profile is run once more from its start, which relies on its steps being
// safe to repeat). The process lock is
// held throughout, so nothing else starts meanwhile, and the Agent step is
// never overlapped: the continuation starts only after the Agent recorded
// the first job as ended. A running panel continues one row at most once; a
// continuation interrupted again is left to the restart.
//
// What is true meanwhile is shown, not a bare "in progress": the row's phase
// as read is "settling_contact" while the Panel reads what the Agent did and
// "settling_recording" while it records the result, and a refused change
// says that this ends by itself. A row that still cannot be settled (the
// Agent stays unreachable past the job's deadline, the Panel's database
// refuses the write) is shown as "unsettled", with one line in Problems now:
// the server owner restarts the panel's web process, whose startup recovery
// settles it.
//
// Ayarlanamayan işlem satırı: panel aynı goroutine içinde Agent'ın kaydını
// salt okur, iş bitince başlangıç kurtarmasının uzlaştırmasını bu tek satır
// için çalıştırır. Bu sırada ekran ne olduğunu söyler; yine de kapanmazsa
// Şu anki sorunlar'da sunucu sahibinin adımı yazar.

const (
	// The phases a row is shown in while it is settled (presentation only:
	// the saved row keeps its real phase).
	serviceOperationSettleContactPhase   = "settling_contact"
	serviceOperationSettleRecordingPhase = "settling_recording"
	serviceOperationUnsettledPhase       = "unsettled"

	// The reasons a refused change names (err.service_operation_busy.<reason>).
	serviceOperationBusyReasonSettling  = "settling"
	serviceOperationBusyReasonUnsettled = "unsettled"
)

// Bounds of the settling reads. Variables only so tests can make them short.
var (
	// serviceOperationSettleMargin is added to the Agent job's own deadline:
	// the Agent ends a job whose step stopped within its 20-second lease.
	serviceOperationSettleMargin = newTunableDuration(2 * time.Minute)
	serviceOperationSettlePoll   = newTunableDuration(250 * time.Millisecond)
	serviceOperationSettleRetry  = newTunableDuration(time.Second)
	// serviceOperationAgentSilence: how long the settling reads may go
	// without one answer before the row is left unsettled and the process
	// lock is released (D1 review, finding 4). The Agent itself refuses a new
	// lease while its job is active, so nothing can overlap meanwhile.
	serviceOperationAgentSilence = newTunableDuration(2 * time.Minute)
	// serviceOperationResumeBusyBound: how long a continuation the Agent
	// refuses because the host is busy is asked again (finding 6).
	serviceOperationResumeBusyBound = newTunableDuration(2 * time.Minute)
)

// errAgentMutationSilent: the settling reads went without one answer for
// serviceOperationAgentSilence. errAgentMutationNoRecord: the Agent answered
// twice that it has no record of the job, so its result is not known.
var (
	errAgentMutationSilent   = errors.New("the agent answered no read of the operation's job")
	errAgentMutationNoRecord = errors.New("the agent has no record of this operation, so its result is not known")
)

// recoveryAfterSyncError is a reconciliation that failed after a privileged
// sync had already run: it is never asked again by a running panel (finding
// 5); the restart settles the row.
type recoveryAfterSyncError struct{ err error }

func (e *recoveryAfterSyncError) Error() string { return e.err.Error() }
func (e *recoveryAfterSyncError) Unwrap() error { return e.err }

// errRecoveryHostLeaseHeld marks a reconciliation that found another Agent
// job holding the host. It changed nothing yet; a live Panel asks again.
var errRecoveryHostLeaseHeld = errors.New("another agent mutation holds the host lease")

// recoveryHostLeaseHeldError keeps the reconciliation's own sentence and is
// errRecoveryHostLeaseHeld for errors.Is.
type recoveryHostLeaseHeldError struct{ message string }

func (e *recoveryHostLeaseHeldError) Error() string        { return e.message }
func (e *recoveryHostLeaseHeldError) Is(target error) bool { return target == errRecoveryHostLeaseHeld }

// serviceOperationSettle is what is known of one row being settled.
type serviceOperationSettle struct {
	Phase     string
	Kind      string
	ServiceID string
	Since     time.Time
}

// serviceOperationSettles holds the rows being settled, or that could not
// be, by operation id. Memory only: a restart settles every row.
var serviceOperationSettles sync.Map

// serviceOperationSettleResumed holds the rows this process has continued
// once after settling them (by operation id).
var serviceOperationSettleResumed sync.Map

func noteServiceOperationSettle(op serviceOperation, phase string) {
	since := time.Now()
	if previous, ok := serviceOperationSettles.Load(op.ID); ok {
		since = previous.(serviceOperationSettle).Since
	}
	serviceOperationSettles.Store(op.ID, serviceOperationSettle{Phase: phase, Kind: op.Kind, ServiceID: op.ServiceID, Since: since})
}

func clearServiceOperationSettle(id string) { serviceOperationSettles.Delete(id) }

// noteServiceOperationUnsettled records a row this process could not end.
func noteServiceOperationUnsettled(op serviceOperation) {
	noteServiceOperationSettle(op, serviceOperationUnsettledPhase)
}

// overlayServiceOperationSettle shows a running row as what is being done
// with it.
func overlayServiceOperationSettle(op serviceOperation) serviceOperation {
	if op.Status != serviceOperationRunning && op.Status != serviceOperationQueued {
		return op
	}
	if value, ok := serviceOperationSettles.Load(op.ID); ok {
		op.Phase = value.(serviceOperationSettle).Phase
	}
	return op
}

// serviceOperationSettleBusyReason is the reason a refused change names while
// a row is settled: unsettled before settling, "" when there is none.
func serviceOperationSettleBusyReason() string {
	reason := ""
	serviceOperationSettles.Range(func(_, value any) bool {
		if value.(serviceOperationSettle).Phase == serviceOperationUnsettledPhase {
			reason = serviceOperationBusyReasonUnsettled
			return false
		}
		reason = serviceOperationBusyReasonSettling
		return true
	})
	return reason
}

// serviceOperationUnsettledProblems is one Problems-now line per row this
// process could not settle and that is still active.
func (p *Panel) serviceOperationUnsettledProblems(ctx context.Context) []monitoringProblem {
	var problems []monitoringProblem
	serviceOperationSettles.Range(func(key, value any) bool {
		settle := value.(serviceOperationSettle)
		if settle.Phase != serviceOperationUnsettledPhase {
			return true
		}
		id := key.(string)
		if op, err := p.serviceOperationByID(ctx, id); err == nil &&
			op.Status != serviceOperationRunning && op.Status != serviceOperationQueued {
			return true
		}
		problems = append(problems, monitoringProblem{
			Key: "service_operation:" + id, Kind: "service_operation_unsettled",
			Severity: "critical", Actor: "server_owner", Subject: settle.ServiceID,
			Since:  metricsTime(settle.Since),
			Action: monitoringProblemAction{Kind: "open", To: "/components"},
		})
		return true
	})
	return problems
}

// settleAbandonedServiceOperation settles op after its Finish failed. The
// caller owns the process lock through release; true means the lock went to
// a resumed run, which releases it at its end.
func (p *Panel) settleAbandonedServiceOperation(op serviceOperation, deadline time.Time, release func()) bool {
	noteServiceOperationSettle(op, serviceOperationSettleContactPhase)
	settled := false
	defer func() {
		if !settled {
			noteServiceOperationUnsettled(op)
		}
	}()
	if deadline.IsZero() {
		deadline = time.Now().Add(45 * time.Minute)
	}
	waitCtx, cancel := context.WithDeadline(context.Background(), deadline.Add(serviceOperationSettleMargin.get()))
	defer cancel()
	job, err := p.waitAbandonedAgentMutation(waitCtx, op.RequestID)
	if err != nil {
		log.Printf("service operation %s could not be settled: the agent's record of it was not read to its end: %v; check the agent (systemctl status celikpanel-agent), then restarting celikpanel-panel settles it; the process lock is released", op.ID, err)
		return false
	}
	// A running panel continues one row at most once: a continuation that
	// lost its Agent again is left to the restart (startup recovery), so no
	// change is repeated round after round by itself.
	if _, again := serviceOperationSettleResumed.Load(op.ID); again && agentMutationCanResume(job) {
		log.Printf("service operation %s was already continued once by this panel process and was interrupted again; restarting celikpanel-panel settles it", op.ID)
		return false
	}
	noteServiceOperationSettle(op, serviceOperationSettleRecordingPhase)
	for {
		current, err := p.serviceOperationByID(waitCtx, op.ID)
		if err == nil && current.Status != serviceOperationRunning && current.Status != serviceOperationQueued {
			settled = true
			clearServiceOperationSettle(op.ID)
			return false
		}
		if err == nil {
			recoveryCtx, cancelRecovery := context.WithTimeout(waitCtx, panelMutationRecoveryTimeout)
			var transferred bool
			_, transferred, err = p.reconcileInterruptedServiceOperationLocked(context.Background(), recoveryCtx, &current, release)
			cancelRecovery()
			if err == nil {
				settled = true
				clearServiceOperationSettle(op.ID)
				if transferred {
					serviceOperationSettleResumed.Store(op.ID, true)
				}
				log.Printf("service operation %s settled from the agent's record (continued: %t)", op.ID, transferred)
				return transferred
			}
		}
		// Another Agent job held the host, or the panel database did not
		// answer, before the reconciliation changed anything: the same
		// reading is asked again until the bound. Once a privileged sync has
		// run (finding 5), nothing is asked again: the syncs are never run a
		// second time and the row is left to the restart with its reason.
		var afterSync *recoveryAfterSyncError
		if errors.As(err, &afterSync) || (!errors.Is(err, errRecoveryHostLeaseHeld) && !panelDatabaseBusy(err)) {
			log.Printf("service operation %s could not be settled: %v; restarting celikpanel-panel settles it", op.ID, err)
			return false
		}
		select {
		case <-waitCtx.Done():
			log.Printf("service operation %s could not be settled before its deadline: %v; restarting celikpanel-panel settles it", op.ID, err)
			return false
		case <-time.After(serviceOperationSettleRetry.get()):
		}
	}
}

// beginServiceOperationLease asks the Agent for an operation's lease and
// returns when it was asked for. A continuation (resume) the Agent refuses
// because the host is busy (the package manager, a release) is asked again
// with a growing pause until serviceOperationResumeBusyBound has passed: right
// after an interrupted step the package manager may still be finishing. A
// first start is asked once, as before.
func (p *Panel) beginServiceOperationLease(op serviceOperation, ownerID string, resume bool) (*agentMutationJob, time.Time, error) {
	bound := time.Now().Add(serviceOperationResumeBusyBound.get())
	pause := serviceOperationSettleRetry.get()
	for {
		beginCtx, beginCancel := context.WithTimeout(context.Background(), panelMutationFinishTimeout)
		askedAt := time.Now()
		job, err := p.beginAgentMutation(beginCtx, op, ownerID, resume)
		beginCancel()
		if err == nil || !resume || !hostMutationRefusedAsBusy(err) || !time.Now().Add(pause).Before(bound) {
			return job, askedAt, err
		}
		noteServiceOperationSettle(op, serviceOperationSettleRecordingPhase)
		time.Sleep(pause)
		if pause *= 2; pause > 15*time.Second {
			pause = 15 * time.Second
		}
	}
}

// waitAbandonedAgentMutation reads the Agent's job of requestID, changing
// nothing, until it is no longer active. An unanswered read is asked again
// until ctx ends; an Agent that refuses every mutation (held) ends the wait,
// because its job cannot change.
//
// Two answers end the wait without a job to settle from (D1 review):
// serviceOperationAgentSilence without one answered read (the Agent is not
// answering: its owner checks it), and an Agent that has no record of the
// job on two reads (its result is not known; it has not "ended").
func (p *Panel) waitAbandonedAgentMutation(ctx context.Context, requestID string) (*agentMutationJob, error) {
	lastAnswer := time.Now()
	missing := 0
	for {
		// One read waits at most as long as a Finish does; a hung read is
		// asked again rather than holding the whole wait.
		readCtx, cancelRead := context.WithTimeout(ctx, panelMutationFinishTimeout)
		job, hold, err := p.statusAgentMutationWithHold(readCtx, requestID)
		cancelRead()
		if hold != "" {
			return job, &agentMutationHeldError{Hold: hold}
		}
		if err == nil {
			lastAnswer = time.Now()
			if job == nil {
				if missing++; missing >= 2 {
					return nil, errAgentMutationNoRecord
				}
			} else if !agentMutationActive(job.Status) {
				return job, nil
			} else {
				missing = 0
			}
		} else if time.Since(lastAnswer) >= serviceOperationAgentSilence.get() {
			return job, fmt.Errorf("%w (last read: %v)", errAgentMutationSilent, err)
		}
		select {
		case <-ctx.Done():
			if err != nil {
				return job, fmt.Errorf("%w (last read: %v)", ctx.Err(), err)
			}
			return job, ctx.Err()
		case <-time.After(serviceOperationSettlePoll.get()):
		}
	}
}
