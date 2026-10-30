package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// 2026-10-30 (Arch run ap-e): while setup ran, another task briefly held the
// host lock; the mail renewal hand-off was refused as busy and setup waited for
// the owner's "Retry reviewed handoff". A busy refusal is a verified "nothing
// started": setup now waits and hands off the same request again by itself,
// bounded by serverSetupHostBusyBound, and asks the owner only past the bound.

type setupBusyEnrollmentHarness struct {
	t        *testing.T
	req      transport.MailEnrollmentStartRequest
	state    string
	busy     int // hand-offs still refused as busy
	reply    func(*transport.MailEnrollmentStartRequest, *transport.MailEnrollmentStartResponse) error
	starts   int
	admits   int
	saved    serverSetupExecutionStep
	persists int
}

func (h *setupBusyEnrollmentHarness) advance(step *serverSetupExecutionStep) (bool, error) {
	return advanceSetupMailEnrollment(context.Background(), h.req, step,
		func() error { h.admits++; return nil },
		func() error { h.persists++; h.saved = *step; return nil },
		func(r *transport.MailEnrollmentRequest, out *transport.MailEnrollmentStatusResponse) error {
			*out = transport.MailEnrollmentStatusResponse{MailEnrollmentRequest: *r, State: h.state, ObservedAt: time.Now().UTC()}
			return nil
		},
		func(r *transport.MailEnrollmentStartRequest, out *transport.MailEnrollmentStartResponse) error {
			h.starts++
			if *r != h.req {
				h.t.Fatalf("hand-off %d changed the reviewed request: %+v", h.starts, *r)
			}
			// The fence is durable before every call, and a re-hand-off is never
			// left armed while its outcome is not known.
			if !h.saved.EnrollmentDispatchAttempted || h.saved.BusyRetry {
				h.t.Fatalf("hand-off %d before its fence: %+v", h.starts, h.saved)
			}
			if h.reply != nil {
				return h.reply(r, out)
			}
			if h.busy > 0 {
				h.busy--
				*out = transport.MailEnrollmentStartResponse{RequestID: r.RequestID, Handoff: transport.MailEnrollmentHandoffBusy, Reason: transport.HostMutationReasonHostLock}
				return nil
			}
			*out = transport.MailEnrollmentStartResponse{RequestID: r.RequestID, Handoff: "accepted"}
			h.state = "forward"
			return nil
		})
}

func newSetupBusyEnrollmentHarness(t *testing.T) (*setupBusyEnrollmentHarness, *serverSetupExecutionStep) {
	t.Helper()
	plan, execution := setupEnrollmentTestPlan()
	req, err := setupMailEnrollmentRequest(plan, &execution, 0)
	if err != nil {
		t.Fatal(err)
	}
	return &setupBusyEnrollmentHarness{t: t, req: req, state: "not_recorded"}, &execution.Steps[0]
}

func TestSetupMailEnrollmentBusyRefusalIsHandedOffAgainWithTheSameIdentity(t *testing.T) {
	h, step := newSetupBusyEnrollmentHarness(t)
	h.busy = 2
	for attempt := 1; attempt <= 2; attempt++ {
		done, err := h.advance(step)
		var wait *serverSetupPeerWait
		if done || !errors.As(err, &wait) || wait.Code != serverSetupHostBusyCode || !strings.Contains(wait.Message, "Nobody needs to act") {
			t.Fatalf("busy hand-off %d: %v %v", attempt, done, err)
		}
		if !step.BusyRetry || step.BusyAttempts != attempt || step.BusySince == "" || step.BusyReason != "" || h.starts != attempt {
			t.Fatalf("busy hand-off %d: %+v starts=%d", attempt, *step, h.starts)
		}
	}
	// The host is free: the same request is handed off and accepted.
	done, err := h.advance(step)
	var enrollment *serverSetupMailEnrollmentWait
	if done || !errors.As(err, &enrollment) || enrollment.Code != "server_setup_mail_enrollment_handoff" || h.starts != 3 || h.admits != 3 || step.BusyRetry {
		t.Fatalf("hand-off after the busy state: %v %v starts=%d admits=%d %+v", done, err, h.starts, h.admits, *step)
	}
	// From here only reads: the recorded request is observed, never re-sent.
	if done, err = h.advance(step); done || !errors.As(err, &enrollment) || enrollment.Code != "server_setup_mail_enrollment_running" || h.starts != 3 {
		t.Fatalf("recorded request: %v %v starts=%d", done, err, h.starts)
	}
	h.state = "published"
	if done, err = h.advance(step); !done || err != nil || h.starts != 3 {
		t.Fatalf("completion: %v %v starts=%d", done, err, h.starts)
	}
}

func TestSetupMailEnrollmentStayingBusyAsksTheOwnerAfterTheBound(t *testing.T) {
	h, step := newSetupBusyEnrollmentHarness(t)
	h.busy = 1 << 20
	if _, err := h.advance(step); err == nil || !step.BusyRetry {
		t.Fatalf("first refusal: %v %+v", err, *step)
	}
	step.BusySince = time.Now().Add(-serverSetupHostBusyBound - time.Minute).UTC().Format(time.RFC3339Nano)
	done, err := h.advance(step)
	var wait *serverSetupMailEnrollmentWait
	if done || !errors.As(err, &wait) || wait.Code != serverSetupMailEnrollmentHostBusyCode {
		t.Fatalf("past the bound: %v %v", done, err)
	}
	for _, want := range []string{"more than 30 minutes", "an unfinished change was holding it", "nothing was changed", "You, the owner", "Retry reviewed handoff", "celikpanel-mail-enrollment-" + h.req.RequestID + ".service"} {
		if !strings.Contains(wait.Message, want) {
			t.Fatalf("owner guidance lacks %q: %s", want, wait.Message)
		}
	}
	if step.BusyRetry || step.BusyReason != transport.HostMutationReasonHostLock || step.BusyAttempts != 2 {
		t.Fatalf("past the bound: %+v", *step)
	}
	// It stays the owner's decision: setup keeps observing and sends nothing.
	starts, admits, persists := h.starts, h.admits, h.persists
	for pass := 0; pass < 3; pass++ {
		if _, err = h.advance(step); !errors.As(err, &wait) || wait.Code != serverSetupMailEnrollmentHostBusyCode {
			t.Fatalf("observation %d: %v", pass, err)
		}
	}
	if h.starts != starts || h.admits != admits || h.persists != persists {
		t.Fatalf("an observation sent, admitted or wrote: %d/%d/%d", h.starts-starts, h.admits-admits, h.persists-persists)
	}
	// The owner's retry got the worker going: the recorded request is observed.
	h.state = "forward"
	if _, err = h.advance(step); !errors.As(err, &wait) || wait.Code != "server_setup_mail_enrollment_running" || h.starts != starts {
		t.Fatalf("after the owner's retry: %v", err)
	}
}

// An unknown outcome is never taken for a busy refusal: a lost reply, an
// untyped error that only reads like "busy", a busy answer for another request
// and a terminal-uncertain mutation all stay observational and are not re-sent.
func TestSetupMailEnrollmentUnknownOutcomeIsNotRetriedAsBusy(t *testing.T) {
	for name, reply := range map[string]func(*transport.MailEnrollmentStartRequest, *transport.MailEnrollmentStartResponse) error{
		"lost reply": func(*transport.MailEnrollmentStartRequest, *transport.MailEnrollmentStartResponse) error {
			return errors.New("connection reset")
		},
		"untyped busy text": func(*transport.MailEnrollmentStartRequest, *transport.MailEnrollmentStartResponse) error {
			return errors.New(errHostMutationBusy.Error())
		},
		"busy for another request": func(r *transport.MailEnrollmentStartRequest, out *transport.MailEnrollmentStartResponse) error {
			*out = transport.MailEnrollmentStartResponse{RequestID: strings.Repeat("9", 32), Handoff: transport.MailEnrollmentHandoffBusy}
			return nil
		},
		"handoff unknown": func(r *transport.MailEnrollmentStartRequest, out *transport.MailEnrollmentStartResponse) error {
			*out = transport.MailEnrollmentStartResponse{RequestID: r.RequestID, Handoff: "unknown", Reason: "mail_enrollment_handoff_unknown"}
			return nil
		},
		"terminal uncertain": func(*transport.MailEnrollmentStartRequest, *transport.MailEnrollmentStartResponse) error {
			return &agentMutationTerminalUncertainError{kind: "mail_enrollment", err: &hostMutationBusyError{reason: transport.HostMutationReasonHostLock}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			h, step := newSetupBusyEnrollmentHarness(t)
			h.reply = reply
			_, err := h.advance(step)
			var wait *serverSetupMailEnrollmentWait
			if !errors.As(err, &wait) || wait.Code != "server_setup_mail_enrollment_unknown" || step.BusyRetry || step.BusyAttempts != 0 || h.starts != 1 {
				t.Fatalf("first answer: %v %+v starts=%d", err, *step, h.starts)
			}
			for pass := 0; pass < 3; pass++ {
				if _, err = h.advance(step); !errors.As(err, &wait) || wait.Code != "server_setup_mail_enrollment_not_recorded" {
					t.Fatalf("observation %d: %v", pass, err)
				}
			}
			if h.starts != 1 {
				t.Fatalf("an unknown outcome was handed off again: %d", h.starts)
			}
		})
	}
}

// The runner end to end: a refused hand-off makes the run wait with the
// "nobody needs to act" reason (persisted), the next pass hands off the same
// request, and the step completes without any owner action and with one audit
// row for the reviewed request.
type setupBusyEnrollmentAgent struct {
	setupEnrollmentContinueAgent
	mu       sync.Mutex
	busyLeft int
	requests []transport.MailEnrollmentStartRequest
}

func (a *setupBusyEnrollmentAgent) MailEnrollmentStatusV1(req *transport.MailEnrollmentRequest, out *transport.MailEnrollmentStatusResponse) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	*out = transport.MailEnrollmentStatusResponse{MailEnrollmentRequest: *req, State: a.state, ObservedAt: time.Now().UTC()}
	return nil
}
func (a *setupBusyEnrollmentAgent) StartMailEnrollmentV1(req *transport.MailEnrollmentStartRequest, out *transport.MailEnrollmentStartResponse) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.requests = append(a.requests, *req)
	if a.busyLeft > 0 {
		a.busyLeft--
		*out = transport.MailEnrollmentStartResponse{RequestID: req.RequestID, Handoff: transport.MailEnrollmentHandoffBusy, Reason: transport.HostMutationReasonHostLock}
		return nil
	}
	*out = transport.MailEnrollmentStartResponse{RequestID: req.RequestID, Handoff: "accepted"}
	a.state = "published"
	return nil
}

func TestServerSetupMailEnrollmentWaitsThroughABusyHostWithoutTheOwner(t *testing.T) {
	f, _ := setupOperationFixture(t)
	plan, execution := setupEnrollmentTestPlan()
	old := buildCommit
	buildCommit = plan.BuildCommit
	t.Cleanup(func() { buildCommit = old })
	plan.Actor.UserID = f.userID
	plan.ID = serverSetupPlanIdentity(plan)
	execution.PlanID = plan.ID
	agent := &setupBusyEnrollmentAgent{busyLeft: 1}
	agent.state, agent.commit = "not_recorded", buildCommit
	f.panel.agentClient = newPolicyDispatchTestPanel(t, agent).agentClient
	raw, _ := json.Marshal(plan)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := f.database.GetDB().Exec(`INSERT INTO server_setup_plans(id,revision,plan_json,requested_by,created_at) VALUES(?,?,?,?,?)`, plan.ID, 1, string(raw), f.userID, now); err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(execution)
	if _, err := f.database.GetDB().Exec(`INSERT INTO server_setup_executions(id,request_id,plan_id,status,execution_json,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`, execution.ID, execution.RequestID, plan.ID, "running", string(raw), now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := f.panel.advanceServerSetupExecution(plan, &execution); err != nil {
		t.Fatal(err)
	}
	saved, err := f.panel.latestServerSetupExecution(context.Background())
	if err != nil || saved == nil || saved.Status != "waiting" || saved.Error == nil || saved.Error.Code != serverSetupHostBusyCode ||
		!saved.Steps[0].BusyRetry || !saved.Steps[0].EnrollmentDispatchAttempted || saved.Steps[0].Status != "running" {
		t.Fatalf("after the busy refusal: %+v %v", saved, err)
	}
	// The owner's retry is not offered while setup retries by itself.
	if _, err := setupMailEnrollmentContinuation(plan, saved, saved.ID, saved.Steps[0].ID); err == nil {
		t.Fatal("owner retry offered during the automatic wait")
	}
	// The next pass (the runner's, after its waiting interval) from the stored state.
	_, _ = f.panel.advanceServerSetupExecution(plan, saved)
	_, _ = f.panel.advanceServerSetupExecution(plan, saved)
	if saved.Steps[0].Status != "succeeded" {
		t.Fatalf("the step did not complete by itself: %+v %+v", saved.Steps[0], saved.Error)
	}
	agent.mu.Lock()
	requests := append([]transport.MailEnrollmentStartRequest(nil), agent.requests...)
	agent.mu.Unlock()
	expected, err := setupMailEnrollmentRequest(plan, &execution, 0)
	if err != nil || len(requests) != 2 || requests[0] != expected || requests[1] != expected {
		t.Fatalf("hand-offs: %+v (want twice %+v) %v", requests, expected, err)
	}
	var audits int
	f.database.GetDB().QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action='server_setup_mail_enrollment_requested'`).Scan(&audits)
	if audits != 1 {
		t.Fatalf("audit rows for the one reviewed request: %d", audits)
	}
}

// The owner's own retry, refused because the server is busy, says so instead
// of "could not be confirmed"; nothing was dispatched.
func TestSetupMailEnrollmentOwnerRetryReportsABusyRefusal(t *testing.T) {
	plan, execution := setupEnrollmentTestPlan()
	req, err := setupMailEnrollmentRequest(plan, &execution, 0)
	if err != nil {
		t.Fatal(err)
	}
	out, err := handoffSetupMailEnrollment(context.Background(), req, func(r *transport.MailEnrollmentRequest, o *transport.MailEnrollmentStatusResponse) error {
		*o = transport.MailEnrollmentStatusResponse{MailEnrollmentRequest: *r, State: "not_recorded", ObservedAt: time.Now().UTC()}
		return nil
	}, func(r *transport.MailEnrollmentStartRequest, o *transport.MailEnrollmentStartResponse) error {
		*o = transport.MailEnrollmentStartResponse{RequestID: r.RequestID, Handoff: transport.MailEnrollmentHandoffBusy, Reason: transport.HostMutationReasonHostLock}
		return nil
	}, true)
	if err != nil || out.Handoff != transport.MailEnrollmentHandoffBusy || out.Reason != "mail_enrollment_host_busy" || out.RequestID != req.RequestID {
		t.Fatalf("busy owner retry: %+v %v", out, err)
	}
}

// The firewall and mail certificate steps: a pure busy refusal of the step's
// own request with no ledger job waits (same identity, bounded); anything
// else keeps the existing failure or reconciliation path.
func TestServerSetupAgentStepBusyRefusalWaitsOnlyWithProof(t *testing.T) {
	f, _ := setupOperationFixture(t)
	busy := &hostMutationBusyError{reason: transport.HostMutationReasonPackageManager}
	step := func(kind string) *serverSetupExecutionStep {
		return &serverSetupExecutionStep{serverSetupPlanStep: serverSetupPlanStep{ID: "04-" + kind, Kind: kind}, Status: "running", RequestID: strings.Repeat("4", 32), OwnerID: strings.Repeat("5", 32)}
	}
	for _, kind := range []string{"firewall", "mail_certificate"} {
		s := step(kind)
		err := f.panel.serverSetupAgentRequestRefusedBusy(context.Background(), s, busy)
		var wait *serverSetupPeerWait
		if !errors.As(err, &wait) || wait.Code != serverSetupHostBusyCode || s.BusyAttempts != 1 || s.BusySince == "" {
			t.Fatalf("%s busy refusal: %v %+v", kind, err, *s)
		}
		s.BusySince = time.Now().Add(-serverSetupHostBusyBound - time.Minute).UTC().Format(time.RFC3339Nano)
		err = f.panel.serverSetupAgentRequestRefusedBusy(context.Background(), s, busy)
		var failure *serverSetupChildFailure
		if !errors.As(err, &failure) || failure.Code != serverSetupHostStayedBusyCode || failure.Reason != transport.HostMutationReasonPackageManager ||
			!strings.Contains(failure.Message, "package manager") || !strings.Contains(failure.Message, "nothing was changed") {
			t.Fatalf("%s past the bound: %v", kind, err)
		}
	}
	unchanged := map[string]error{
		"joined":             errors.Join(busy, errors.New("compensation failed")),
		"terminal uncertain": &agentMutationTerminalUncertainError{kind: "firewall_apply", err: busy},
		"not busy":           errors.New("firewall application was not verified"),
	}
	for name, cause := range unchanged {
		s := step("firewall")
		if got := f.panel.serverSetupAgentRequestRefusedBusy(context.Background(), s, cause); got != cause || s.BusyAttempts != 0 {
			t.Fatalf("%s: %v %+v", name, got, *s)
		}
	}
	for _, kind := range []string{"dns", "infrastructure_dns", "panel_certificate", "service"} {
		s := step(kind)
		if got := f.panel.serverSetupAgentRequestRefusedBusy(context.Background(), s, busy); got != busy {
			t.Fatalf("%s changed: %v", kind, got)
		}
	}
	// The Agent's ledger knows this request: not proof that nothing ran.
	s := step("mail_certificate")
	f.agent.mu.Lock()
	f.agent.mutationJobs[s.RequestID] = &ServiceOperationMutationJob{RequestID: s.RequestID, OwnerID: s.OwnerID, Kind: "mail_host_certificate", Status: agentMutationFailed}
	f.agent.mu.Unlock()
	if got := f.panel.serverSetupAgentRequestRefusedBusy(context.Background(), s, busy); got != busy || s.BusyAttempts != 0 {
		t.Fatalf("a request the ledger knows: %v %+v", got, *s)
	}
}
