package main

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// New reviewed plans bind independent renewal to a verified installed kit.
// Existing accepted plans are never extended to acquire new enrollment authority.
type serverSetupMailEnrollmentWait struct{ Code, Message string }

func (e *serverSetupMailEnrollmentWait) Error() string { return e.Code }
func setupMailEnrollmentWaiting(reason, requestID string) error {
	message := "The mail renewal result is unknown. The server administrator can inspect celikpanel-mail-enrollment-<request-id>.service using this step's request ID. This setup will keep checking the same request; it will not start it again."
	switch reason {
	case "running":
		message = "The mail renewal request is recorded, but its final result is not verified. Setup will keep checking. The server administrator can inspect celikpanel-mail-enrollment-<request-id>.service and continue the same recorded operation if it stopped."
	case "handoff":
		message = "The independent worker handoff was accepted. Enrollment is not yet confirmed. Setup will check the same request automatically; the server administrator can inspect celikpanel-mail-enrollment-<request-id>.service if it stops."
	case "rollback":
		message = "The recorded mail renewal request requires restoration; its completion is not yet verified. Setup will check that result automatically. The server administrator can inspect the same worker unit if restoration stops."
	case "not_recorded":
		message = "The mail renewal handoff has no verified admission record yet. The server administrator can inspect the same worker unit and use Retry reviewed handoff to submit the original reviewed request again. Setup will keep checking; it will not create another request."
	}
	return &serverSetupMailEnrollmentWait{"server_setup_mail_enrollment_" + reason, strings.ReplaceAll(message, "<request-id>", requestID)}
}

// A handoff refused because the host was busy (2026-10-30, Arch run ap-e).
// The Agent refused before starting a worker, so nothing ran and nothing was
// recorded: the step waits and hands off the same request again by itself, for
// at most serverSetupHostBusyBound. Only past that bound is the owner asked,
// with what kept the server busy and what to do.
//
// Sunucu meşgulken reddedilen devir: hiçbir şey başlamadı; kurulum aynı isteği
// kendiliğinden yeniden iletir; yalnız süre aşılırsa sahipten eylem istenir.
const serverSetupMailEnrollmentHostBusyCode = "server_setup_mail_enrollment_host_busy"

// serverSetupMailEnrollmentBusyUnnamed marks a bound reached by a refusal that
// named no reason.
const serverSetupMailEnrollmentBusyUnnamed = "unknown"

func setupMailEnrollmentRefusedBusy(step *serverSetupExecutionStep, reason, requestID string, now time.Time) error {
	if recordServerSetupHostBusy(step, now) {
		step.BusyRetry = true
		return &serverSetupPeerWait{Code: serverSetupHostBusyCode, Message: serverSetupHostBusyWaitMessage}
	}
	step.BusyRetry = false
	if reason == "" {
		reason = serverSetupMailEnrollmentBusyUnnamed
	}
	step.BusyReason = reason
	return setupMailEnrollmentStayedBusy(*step, requestID)
}

func setupMailEnrollmentStayedBusy(step serverSetupExecutionStep, requestID string) error {
	return &serverSetupMailEnrollmentWait{serverSetupMailEnrollmentHostBusyCode,
		"This server stayed busy for more than 30 minutes (" + serverSetupHostBusyCause(step.BusyReason) + "), so the mail renewal request was not handed off and nothing was changed (" + strconv.Itoa(step.BusyAttempts) + " tries). You, the owner: finish or stop the other change on this server, then choose Retry reviewed handoff; setup continues with the same request. If it still does not start, inspect celikpanel-mail-enrollment-" + requestID + ".service."}
}
func setupMailEnrollmentRequest(plan serverSetupPlan, execution *serverSetupExecution, index int) (transport.MailEnrollmentStartRequest, error) {
	empty := transport.MailEnrollmentStartRequest{}
	if execution == nil || index < 0 || index >= len(execution.Steps) || plan.Version != serverSetupPlanVersion || plan.ID != serverSetupPlanIdentity(plan) || validateServerSetupExecution(plan, *execution) != nil {
		return empty, errors.New("mail enrollment does not match the accepted setup plan")
	}
	step := execution.Steps[index]
	if step.Kind != "mail_enrollment" || step.Target != "mail-renewal" || len(step.Qualifier) != 64 || strings.Trim(step.Qualifier, "0123456789abcdef") != "" {
		return empty, errors.New("reviewed mail renewal generation is invalid")
	}
	for _, earlier := range execution.Steps[:index] {
		if earlier.Status != "succeeded" {
			return empty, errors.New("earlier setup steps have not completed")
		}
	}
	return transport.MailEnrollmentStartRequest{MailEnrollmentRequest: transport.MailEnrollmentRequest{RequestID: step.RequestID, OwnerID: step.OwnerID, Generation: step.Qualifier}, ExpectedBuildCommit: plan.BuildCommit}, nil
}
func (p *Panel) runServerSetupMailEnrollment(ctx context.Context, plan serverSetupPlan, execution *serverSetupExecution, index int) (bool, error) {
	request, err := setupMailEnrollmentRequest(plan, execution, index)
	if err != nil {
		return false, err
	}
	// A hand-off again after a busy refusal is the same reviewed request: it is
	// audited once, at its first hand-off, not once per attempt.
	busyRetry := execution.Steps[index].BusyRetry
	return advanceSetupMailEnrollment(ctx, request, &execution.Steps[index], func() error {
		if err := p.requireServerSetupAdmission(ctx); err != nil {
			return err
		}
		if plan.BuildCommit != buildCommit {
			return errServerSetupBuildChanged
		}
		if err := p.requireMatchingAgentBuild(ctx); err != nil {
			return err
		}
		return p.authorizeAgentRPCContext(ctx, "Agent.StartMailEnrollmentV1")
	}, func() error { return p.persistServerSetupExecution(ctx, *execution) }, func(req *transport.MailEnrollmentRequest, out *transport.MailEnrollmentStatusResponse) error {
		return p.callAgentContext(ctx, "Agent.MailEnrollmentStatusV1", req, out)
	}, func(req *transport.MailEnrollmentStartRequest, out *transport.MailEnrollmentStartResponse) error {
		if !busyRetry {
			p.auditServiceOperation(ctx, plan.Actor, "server_setup_mail_enrollment_requested")
		}
		return p.callAgentContext(ctx, "Agent.StartMailEnrollmentV1", req, out)
	})
}

// The persisted attempt fence precedes IPC. A crash/lost reply leaves the same
// request observational; even proven ledger absence cannot authorize redispatch.
// Only the Agent's typed busy answer to this exact request (no worker started)
// arms one more hand-off of the same request, bounded in time (2026-10-30).
func advanceSetupMailEnrollment(ctx context.Context, req transport.MailEnrollmentStartRequest, step *serverSetupExecutionStep, admit, persist func() error, status func(*transport.MailEnrollmentRequest, *transport.MailEnrollmentStatusResponse) error, start func(*transport.MailEnrollmentStartRequest, *transport.MailEnrollmentStartResponse) error) (bool, error) {
	if ctx == nil || step == nil || step.Status != "running" || req.RequestID != step.RequestID || req.OwnerID != step.OwnerID || req.Generation != step.Qualifier || admit == nil || persist == nil || status == nil || start == nil {
		return false, errors.New("mail enrollment adapter is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	var observed transport.MailEnrollmentStatusResponse
	if err := status(&req.MailEnrollmentRequest, &observed); err != nil || observed.MailEnrollmentRequest != req.MailEnrollmentRequest || observed.ObservedAt.IsZero() {
		return false, setupMailEnrollmentWaiting("unknown", req.RequestID)
	}
	switch observed.State {
	case "published":
		return true, nil // historical completion; final health checks remain separate
	case "restored":
		return false, &serverSetupChildFailure{Code: "mail_enrollment_restored", Message: "Mail renewal enrollment was restored to its previous state. The server administrator must review the failed request before starting a new plan."}
	case "forward":
		return false, setupMailEnrollmentWaiting("running", req.RequestID)
	case "rollback":
		return false, setupMailEnrollmentWaiting("rollback", req.RequestID)
	case "not_recorded":
		// Handed off again only after the Agent's own busy refusal of the
		// previous hand-off (BusyRetry): that refusal proved no worker started.
		// A hand-off whose reply was lost, or that was accepted, is never
		// repeated here.
		if step.EnrollmentDispatchAttempted && !step.BusyRetry {
			if step.BusyReason != "" {
				return false, setupMailEnrollmentStayedBusy(*step, req.RequestID)
			}
			return false, setupMailEnrollmentWaiting("not_recorded", req.RequestID)
		}
	default:
		return false, setupMailEnrollmentWaiting("unknown", req.RequestID)
	}
	if err := admit(); err != nil {
		return false, err
	}
	// The fence is durable before the call: a crash or lost reply from here on
	// leaves the request observational (BusyRetry cleared), never re-sent.
	step.EnrollmentDispatchAttempted = true
	step.BusyRetry, step.BusyReason = false, ""
	if err := persist(); err != nil {
		return false, err
	}
	var handed transport.MailEnrollmentStartResponse
	err := start(&req, &handed)
	if err == nil && handed.RequestID == req.RequestID && handed.Handoff == transport.MailEnrollmentHandoffBusy {
		err = &hostMutationBusyError{reason: handed.Reason}
	}
	if hostMutationRefusedAsBusy(err) {
		return false, setupMailEnrollmentRefusedBusy(step, hostMutationBusyReason(err), req.RequestID, time.Now())
	}
	if err != nil || handed.RequestID != req.RequestID || handed.Handoff != "accepted" {
		return false, setupMailEnrollmentWaiting("unknown", req.RequestID)
	}
	return false, setupMailEnrollmentWaiting("handoff", req.RequestID)
}

// A preview is a point-in-time read. Execution revalidates the installed source,
// before-state and exact reviewed child authority under native exclusion.
func setupMailEnrollmentPlanGeneration(preview transport.MailEnrollmentPreviewResponse, commit string) (string, error) {
	validHex := func(value string, n int) bool {
		return len(value) == n && strings.Trim(value, "0123456789abcdef") == ""
	}
	invalid := errors.New("independent mail renewal source or native configuration could not be verified")
	if preview.State != "verified" || !validHex(commit, 40) || preview.BuildCommit != commit || !validHex(preview.Generation, 64) || preview.ObservedAt.IsZero() {
		return "", invalid
	}
	switch preview.NativeMode {
	case "absent", "legacy":
		if preview.ExistingGeneration != "" || preview.TimerEnablement != "absent" || preview.TimerActivity != "inactive" {
			return "", invalid
		}
		return preview.Generation, nil
	case "independent":
		if !validHex(preview.ExistingGeneration, 64) || (preview.TimerEnablement != "enabled" && preview.TimerEnablement != "disabled") || (preview.TimerActivity != "active" && preview.TimerActivity != "inactive") {
			return "", invalid
		}
		return "", nil // Preserve the existing kit and the owner's timer preference.
	default:
		return "", invalid
	}
}
