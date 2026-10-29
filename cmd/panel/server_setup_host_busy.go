package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Another change on this server (Beta-1 third re-run, side effect,
// 2026-10-29). A start while a Components install (redis) ran created a run
// whose first component the Agent refused at admission (HOST_MUTATION_BUSY,
// package manager active): the run failed and needed Edit plan.
//
// Now the start is refused before a run exists (server_setup_host_busy), and
// a run that meets the busy state waits instead of failing: when another
// operation is active nothing is created, and when the Agent refuses a child
// at admission (proven: nothing ran) the same child is admitted again, for
// at most serverSetupHostBusyBound, then the step fails naming the cause.
//
// Sunucuda başka bir değişiklik sürerken kurulum başlamaz; süren kurulum
// adımı bekler.

const (
	serverSetupHostBusyCode         = "server_setup_host_busy"
	serverSetupHostBusyStartMessage = "Another change is running on this server (for example a component installing from Components), so setup did not start and nothing was changed. Nobody needs to fix anything: wait until it finishes (its progress shows on its own page), then start setup."
	serverSetupHostBusyWaitMessage  = "Another change is running on this server (for example a component installing from Components, or the package manager is busy), so this step waits and started nothing. Nobody needs to act: setup continues this step by itself once the other change has finished."
)

// serverSetupHostBusyBound bounds, by time, how long a step's child may be
// refused at admission before the step fails (review fix, item 9).
var serverSetupHostBusyBound = 30 * time.Minute

const serverSetupHostStayedBusyCode = "server_setup_host_stayed_busy"

// serverSetupChildRefusedAtAdmission proves that a failed child never ran:
// it failed at its first (non-resume) admission, at the lease, because the
// host was busy, and the Agent's ledger holds no job for its request. A
// resumed child (its job is in the ledger), a mail profile whose later
// sub-step was refused (another phase and code), or any read that fails is
// not proof.
func (p *Panel) serverSetupChildRefusedAtAdmission(ctx context.Context, op serviceOperation) bool {
	if op.Status != serviceOperationFailed || op.Phase != "lease_failed" || op.Error == nil || op.Error.Code != errCodeHostMutationBusy {
		return false
	}
	job, err := p.statusAgentMutation(ctx, op.RequestID)
	return err == nil && job == nil
}

// serverSetupHostBusyAttempt records one refused admission on the step: a
// wait, or past the bound a verified failure that names what kept the host
// busy when that is known.
func serverSetupHostBusyAttempt(step *serverSetupExecutionStep, op serviceOperation, now time.Time) error {
	since, err := time.Parse(time.RFC3339Nano, step.BusySince)
	if err != nil || step.BusySince == "" {
		since = now
		step.BusySince = now.UTC().Format(time.RFC3339Nano)
	}
	step.BusyAttempts++
	if now.Sub(since) < serverSetupHostBusyBound {
		step.BusyRetry = true
		return &serverSetupPeerWait{Code: serverSetupHostBusyCode, Message: serverSetupHostBusyWaitMessage}
	}
	step.BusyRetry = false
	why := map[string]string{
		transport.HostMutationReasonPackageManager: "its package manager was busy with another package task",
		transport.HostMutationReasonAgentMutation:  "another CelikPanel change was running",
		transport.HostMutationReasonPanelOperation: "another CelikPanel operation was running",
		transport.HostMutationReasonHostLock:       "an unfinished change was holding it",
	}[reasonOf(op)]
	if why == "" {
		why = "another change kept it busy"
	}
	return &serverSetupChildFailure{Code: serverSetupHostStayedBusyCode, Reason: reasonOf(op),
		Message: "This server stayed busy for more than 30 minutes (" + why + "), so this step could not start and nothing was installed (" + strconv.Itoa(step.BusyAttempts) + " tries). You, the owner: finish or stop the other change on this server; then choose Review a revised plan."}
}

func reasonOf(op serviceOperation) string {
	if op.Error == nil {
		return ""
	}
	return op.Error.Reason
}

// requeueRefusedServiceOperation admits the same refused child again: its
// row goes back to queued (it never ran), so no new operation, request
// identity or audit row is made for the attempt.
func (p *Panel) requeueRefusedServiceOperation(ctx context.Context, op serviceOperation) (serviceOperation, error) {
	result, err := p.db.GetDB().ExecContext(ctx, `UPDATE service_operations SET status=?, phase='queued', error_code=NULL, error_message=NULL, finished_at=NULL, updated_at=?
		WHERE id=? AND status=? AND phase='lease_failed' AND error_code=?`,
		serviceOperationQueued, time.Now().UTC().Format(time.RFC3339Nano), op.ID, serviceOperationFailed, errCodeHostMutationBusy)
	if err != nil {
		return serviceOperation{}, err
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		return serviceOperation{}, errors.New("refused setup child changed before it was admitted again")
	}
	return p.serviceOperationByRequestID(ctx, op.RequestID)
}

// serverSetupHostBusy: another service operation or Agent mutation is
// active on this server. An Agent that cannot be asked is
// errServerSetupAgentUnreachable (its own refusal at start).
func (p *Panel) serverSetupHostBusy(ctx context.Context) (bool, error) {
	active, err := p.activeServiceOperation(ctx)
	if err != nil || active != nil {
		return active != nil, err
	}
	job, err := p.statusAgentMutation(ctx, "")
	if err != nil {
		if errors.Is(err, errHostMutationBusy) {
			return false, err
		}
		return false, fmt.Errorf("%w: %v", errServerSetupAgentUnreachable, err)
	}
	return job != nil && agentMutationActive(job.Status), nil
}

var errServerSetupAgentUnreachable = errors.New("the CelikPanel Agent could not be asked")

const (
	serverSetupAgentUnreachableCode    = "server_setup_agent_unreachable"
	serverSetupAgentUnreachableMessage = "CelikPanel could not reach its Agent on this server, so setup did not start and nothing was changed. You, the owner: check that the agent runs (`systemctl status celikpanel-agent`, and `sudo systemctl start celikpanel-agent` if it is stopped); then start setup."
)

// serverSetupStartHostRefusal: the start's coded answer for a host that is
// busy, still starting, or whose Agent does not answer.
func serverSetupStartHostRefusal(err error) (string, string, bool) {
	switch {
	case err == nil:
		return "", "", false
	case errors.Is(err, errHostMutationBusy):
		return serverSetupHostBusyCode, serverSetupHostBusyStartMessage, true
	case serverSetupHostStarting(err) && serverSetupHostStartingBySystemd(err):
		return serverSetupHostStartingCode, "This server is still starting (systemd does not report it ready yet), so setup did not start and nothing was changed. Nobody needs to act: wait a moment until the server has started, then start setup.", true
	case errors.Is(err, errAgentRPCPlatformIdentityUnavailable), errors.Is(err, errServerSetupAgentUnreachable):
		return serverSetupAgentUnreachableCode, serverSetupAgentUnreachableMessage, true
	}
	return "", "", false
}
