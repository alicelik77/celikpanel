package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/rpc"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.145 lab, D1 (23 November 2026): a longer change must never stay "in
// progress" for ever. The Panel stopped an operation's worker while the
// Agent's step ran on; the Agent refused Finish while the step was active,
// and the Panel returned without settling its own row. The row stayed
// running, and every other server change was refused until a restart.

// heldStepAgent is the test Agent with the two things D1 needs: renewals
// that go unanswered, and a privileged step that is still running, so
// Finish is refused as the real Agent refuses it.
type heldStepAgent struct {
	*serviceOperationTestAgent

	unanswered    atomic.Bool  // every renewal goes unanswered while set
	unansweredOne atomic.Int32 // this many single renewals go unanswered
	held          atomic.Bool  // the Agent's step still runs
	finishRefused chan struct{}
	refusedOnce   sync.Once
	refusals      atomic.Int32

	installActive  atomic.Int32
	installOverlap atomic.Bool
}

func newHeldStepAgent(base *serviceOperationTestAgent) *heldStepAgent {
	return &heldStepAgent{serviceOperationTestAgent: base, finishRefused: make(chan struct{})}
}

func (a *heldStepAgent) HeartbeatServiceMutation(req *ServiceOperationMutationHeartbeatRequest, resp *ServiceOperationMutationResponse) error {
	if a.unanswered.Load() {
		return errors.New("simulated renewal without an answer in time")
	}
	if a.unansweredOne.Load() > 0 {
		a.unansweredOne.Add(-1)
		return errors.New("simulated renewal without an answer in time")
	}
	return a.serviceOperationTestAgent.HeartbeatServiceMutation(req, resp)
}

func (a *heldStepAgent) FinishServiceMutation(req *ServiceOperationMutationFinishRequest, resp *ServiceOperationMutationResponse) error {
	if a.held.Load() {
		a.mu.Lock()
		resp.Job = cloneServiceOperationMutationJob(a.mutationJobs[req.RequestID])
		a.mutationEvents = append(a.mutationEvents, "finish:refused")
		a.mu.Unlock()
		resp.Error = "service mutation still has an active privileged step"
		a.refusals.Add(1)
		a.refusedOnce.Do(func() { close(a.finishRefused) })
		return nil
	}
	return a.serviceOperationTestAgent.FinishServiceMutation(req, resp)
}

// InstallService records whether a second install step ever ran while the
// first one was still running on the Agent.
func (a *heldStepAgent) InstallService(req *ServiceOperationInstallRequest, resp *ServiceOperationInstallResponse) error {
	if a.installActive.Add(1) > 1 {
		a.installOverlap.Store(true)
	}
	defer a.installActive.Add(-1)
	return a.serviceOperationTestAgent.InstallService(req, resp)
}

// endStep ends the held step. The Panel no longer renews the lease, so the
// Agent's watchdog records the job as the real one does.
func (a *heldStepAgent) endStep(requestID string) {
	a.expireJob(requestID)
	a.held.Store(false)
}

// expireJob records the job's lease as expired, as the Agent's watchdog does
// once a step has ended and nobody renews the lease.
func (a *heldStepAgent) expireJob(requestID string) {
	a.mu.Lock()
	if job := a.mutationJobs[requestID]; job != nil && agentMutationActive(job.Status) {
		job.Status = agentMutationFailed
		job.Phase = "failed"
		job.ErrorCode = agentMutationLeaseExpiredErrorCode
		job.ErrorMessage = "The panel stopped heartbeating before the service mutation completed."
		if a.mutationActive == requestID {
			a.mutationActive = ""
		}
		a.mutationEvents = append(a.mutationEvents, "expired:"+job.Kind)
	}
	a.mu.Unlock()
}

func attachHeldStepAgent(t *testing.T, p *Panel, agent *heldStepAgent) {
	t.Helper()
	server := rpc.NewServer()
	if err := server.RegisterName("Agent", agent); err != nil {
		t.Fatalf("register held-step agent: %v", err)
	}
	connector := func(ctx context.Context) (*rpc.Client, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		serverConn, clientConn := net.Pipe()
		go server.ServeConn(serverConn)
		return rpc.NewClient(clientConn), nil
	}
	client, err := connector(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	p.agentClient = transport.NewReconnectingClientWithContextConnector(client, connector)
	t.Cleanup(func() { _ = client.Close() })
}

func newHeldStepFixture(t *testing.T) (serviceOperationTestFixture, *heldStepAgent) {
	t.Helper()
	f := newServiceOperationTestFixture(t)
	agent := newHeldStepAgent(f.agent)
	attachHeldStepAgent(t, f.panel, agent)
	return f, agent
}

// shortenD1Bounds makes the heartbeat and the settling reads fast.
func shortenD1Bounds(t *testing.T) {
	t.Helper()
	interval, window := panelOperationHeartbeatInterval.get(), panelHeartbeatUncertainWindow.get()
	poll, retry, margin := serviceOperationSettlePoll.get(), serviceOperationSettleRetry.get(), serviceOperationSettleMargin.get()
	panelOperationHeartbeatInterval.set(20 * time.Millisecond)
	panelHeartbeatUncertainWindow.set(100 * time.Millisecond)
	serviceOperationSettlePoll.set(10 * time.Millisecond)
	serviceOperationSettleRetry.set(20 * time.Millisecond)
	// The 15-second terminal read after a refused Finish, made short: in the
	// lab it ran out the same way (D1).
	restoreWait := waitExpectedAgentMutationTerminalFn
	waitExpectedAgentMutationTerminalFn = func(panel *Panel, ctx context.Context, identity agentMutationIdentity) (*agentMutationJob, error) {
		short, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		defer cancel()
		return panel.waitExpectedAgentMutationTerminal(short, identity)
	}
	t.Cleanup(func() {
		waitExpectedAgentMutationTerminalFn = restoreWait
		panelOperationHeartbeatInterval.set(interval)
		panelHeartbeatUncertainWindow.set(window)
		serviceOperationSettlePoll.set(poll)
		serviceOperationSettleRetry.set(retry)
		serviceOperationSettleMargin.set(margin)
	})
}

func clearD1Settles(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		for _, marks := range []*sync.Map{&serviceOperationSettles, &serviceOperationSettleResumed} {
			marks.Range(func(key, _ any) bool {
				marks.Delete(key)
				return true
			})
		}
	})
}

func waitForClosed(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatalf("%s did not happen", what)
	}
}

func waitForServiceOperationPhase(t *testing.T, p *Panel, userID int, id, phase string) *serviceOperation {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		op, body := getServiceOperation(t, p, userID, id)
		if op != nil && op.Phase == phase {
			return op
		}
		if time.Now().After(deadline) {
			t.Fatalf("operation never showed phase %q: %s", phase, body)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func busyReason(t *testing.T, recorder *httptest.ResponseRecorder) (string, string) {
	t.Helper()
	if recorder.Code != http.StatusConflict {
		t.Fatalf("want 409, got %d %s", recorder.Code, recorder.Body.String())
	}
	var body struct {
		Error  string `json:"error"`
		Code   string `json:"code"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil || body.Code != errCodeServiceOperationBusy {
		t.Fatalf("busy body %s: %v", recorder.Body.String(), err)
	}
	return body.Reason, body.Error
}

// The read-only family (webmail update; a webmail address is reconciled the
// same way): the diagnosis' test. A held step, renewals unanswered until the
// worker is stopped, Finish refused, the step released: the row settles by
// itself as "updated" and a second operation is admitted, without startup
// recovery. The update is never started a second time.
func TestD1WebmailUpdateSettlesItselfAfterTheAgentStepEnds(t *testing.T) {
	f, agent := newHeldStepFixture(t)
	shortenD1Bounds(t)
	clearD1Settles(t)
	audited := observeServiceOperationAudits(f.panel)
	restoreRead, restoreProof, restoreUpdate := readRoundcubeUpdateFromAgent, readWebmailProofFromAgent, updateRoundcubeThroughAgent
	t.Cleanup(func() {
		readRoundcubeUpdateFromAgent, readWebmailProofFromAgent, updateRoundcubeThroughAgent = restoreRead, restoreProof, restoreUpdate
	})
	var updates atomic.Int32
	started := make(chan struct{})
	updateRoundcubeThroughAgent = func(_ *Panel, ctx context.Context, _ string) (transport.RoundcubeUpdateResponse, error) {
		if updates.Add(1) == 1 {
			close(started)
		}
		// The Panel stops waiting; the Agent's step goes on (net/rpc cannot
		// cancel it on the Agent's side).
		<-ctx.Done()
		return transport.RoundcubeUpdateResponse{}, ctx.Err()
	}
	readRoundcubeUpdateFromAgent = func(*Panel, context.Context) (transport.RoundcubeUpdateCheckResponse, error) {
		return transport.RoundcubeUpdateCheckResponse{Installed: true, Version: "1.6.21"}, nil
	}
	readWebmailProofFromAgent = func(*Panel, context.Context) (transport.WebmailProofResponse, error) {
		return transport.WebmailProofResponse{Proved: true, Version: "1.6.21"}, nil
	}
	agent.held.Store(true)
	agent.unanswered.Store(true)

	recorder := postWebmailUpdate(t, f.panel, f.userID, `{"version":"1.6.21","request_id":"`+mustServiceOperationRequestID(t)+`"}`)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("accepted: %d %s", recorder.Code, recorder.Body.String())
	}
	accepted := decodeServiceOperationEnvelope(t, recorder)
	waitForClosed(t, started, "the update step")
	waitForClosed(t, agent.finishRefused, "the refused Finish")

	// While the Agent's step still runs, the row says the panel is reading
	// what was done, and another change is told that this ends by itself.
	shown := waitForServiceOperationPhase(t, f.panel, f.userID, accepted.ID, serviceOperationSettleContactPhase)
	if shown.Status != serviceOperationRunning {
		t.Fatalf("settling row: %+v", shown)
	}
	second, _ := postServiceInstall(t, f, "certbot")
	if reason, message := busyReason(t, second); reason != serviceOperationBusyReasonSettling || !strings.Contains(message, "ends by itself") {
		t.Fatalf("refusal while settling: %q %q", reason, message)
	}

	// The step ends; the lease the Panel stopped renewing expires.
	agent.unanswered.Store(false)
	agent.endStep(accepted.RequestID)

	done, _ := waitForServiceOperation(t, f.panel, f.userID, accepted.ID, serviceOperationSucceeded)
	if !strings.Contains(string(done.Result), `"outcome":"updated"`) || !strings.Contains(string(done.Result), `"recovered":true`) {
		t.Fatalf("settled row: %+v %s", done.Error, done.Result)
	}
	waitForServiceOperationAudit(t, audited, "webmail.update.recovered:1.6.21")
	if n := updates.Load(); n != 1 {
		t.Fatalf("the update ran %d times", n)
	}
	if reason := serviceOperationSettleBusyReason(); reason != "" {
		t.Fatalf("a settled row still marks refusals: %q", reason)
	}
	// A second operation is admitted, without startup recovery.
	recorder, queued := postServiceInstall(t, f, "certbot")
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("second operation: %d %s", recorder.Code, recorder.Body.String())
	}
	waitForServiceOperation(t, f.panel, f.userID, queued.ID, serviceOperationSucceeded)
}

// The resume family (component install; install-together, remove-together,
// a mail profile and a runtime resume the same way): the row follows what
// startup does for it, it resumes once, after the first step has ended, and
// never runs two install steps at the same time.
func TestD1ComponentInstallResumesOnceAfterTheAgentStepEnds(t *testing.T) {
	f, agent := newHeldStepFixture(t)
	shortenD1Bounds(t)
	clearD1Settles(t)
	audited := observeServiceOperationAudits(f.panel)
	started := make(chan struct{})
	release := make(chan struct{})
	f.agent.installStarted = started
	f.agent.releaseInstall = release
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	agent.held.Store(true)
	agent.unanswered.Store(true)

	recorder, queued := postServiceInstall(t, f, "certbot")
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("install: %d %s", recorder.Code, recorder.Body.String())
	}
	waitForClosed(t, started, "the install step")
	waitForClosed(t, agent.finishRefused, "the refused Finish")
	waitForServiceOperationPhase(t, f.panel, f.userID, queued.ID, serviceOperationSettleContactPhase)
	if calls := f.agent.installCalls.Load(); calls != 1 {
		t.Fatalf("install calls while the first step runs: %d", calls)
	}

	// The step ends on the Agent; its lease expires.
	agent.unanswered.Store(false)
	close(release)
	deadline := time.Now().Add(5 * time.Second)
	for agent.installActive.Load() != 0 || f.agent.installCalls.Load() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the first install step did not end")
		}
		time.Sleep(time.Millisecond)
	}
	agent.endStep(queued.RequestID)

	done, _ := waitForServiceOperation(t, f.panel, f.userID, queued.ID, serviceOperationSucceeded)
	waitForServiceOperationAudit(t, audited, "service.install.recovered:certbot")
	var result map[string]any
	if err := json.Unmarshal(done.Result, &result); err != nil || result["installed"] != true {
		t.Fatalf("resumed row: %v %s", err, done.Result)
	}
	if calls := f.agent.installCalls.Load(); calls != 2 {
		t.Fatalf("install calls = %d, want the first and one continuation", calls)
	}
	if agent.installOverlap.Load() {
		t.Fatal("a second install step ran while the first was still running")
	}
	// A second operation is admitted, without startup recovery.
	recorder, next := postServiceInstall(t, f, "certbot")
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("second operation: %d %s", recorder.Code, recorder.Body.String())
	}
	waitForServiceOperation(t, f.panel, f.userID, next.ID, serviceOperationSucceeded)
}

// A running panel continues a row at most once: when the continuation loses
// its Agent too, the row is left to the restart, and nothing runs a third
// time by itself.
func TestD1APanelContinuesARowAtMostOnce(t *testing.T) {
	f, agent := newHeldStepFixture(t)
	shortenD1Bounds(t)
	clearD1Settles(t)
	t.Cleanup(func() { agent.held.Store(false) })
	started := make(chan struct{})
	release := make(chan struct{})
	f.agent.installStarted = started
	f.agent.releaseInstall = release
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	agent.held.Store(true)
	agent.unanswered.Store(true)

	recorder, queued := postServiceInstall(t, f, "certbot")
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("install: %d %s", recorder.Code, recorder.Body.String())
	}
	waitForClosed(t, started, "the install step")
	waitForClosed(t, agent.finishRefused, "the refused Finish")
	close(release)
	// The first step ends; the continuation starts, and its own Finish is
	// refused again while its step is still recorded as running.
	agent.expireJob(queued.RequestID)
	deadline := time.Now().Add(5 * time.Second)
	for agent.refusals.Load() < 2 {
		if time.Now().After(deadline) {
			t.Fatalf("the continuation never reached its Finish (install calls %d)", f.agent.installCalls.Load())
		}
		time.Sleep(time.Millisecond)
	}
	agent.expireJob(queued.RequestID)
	shown := waitForServiceOperationPhase(t, f.panel, f.userID, queued.ID, serviceOperationUnsettledPhase)
	if shown.Status != serviceOperationRunning {
		t.Fatalf("row: %+v", shown)
	}
	time.Sleep(100 * time.Millisecond)
	if calls := f.agent.installCalls.Load(); calls != 2 {
		t.Fatalf("install calls = %d, want the first and one continuation only", calls)
	}
}

// When the row cannot be settled (here: the Agent's step never ends before
// the job's deadline), the row says so, every refusal names the server
// owner's step, Problems now carries one line, and startup recovery (the
// restart that line asks for) settles it.
func TestD1AnUnsettledRowNamesTheRestartAndTheRestartSettlesIt(t *testing.T) {
	f, agent := newHeldStepFixture(t)
	shortenD1Bounds(t)
	clearD1Settles(t)
	serviceOperationSettleMargin.set(50 * time.Millisecond)
	f.agent.mutationDeadlineAfter = 400 * time.Millisecond
	restoreRead, restoreProof, restoreUpdate := readRoundcubeUpdateFromAgent, readWebmailProofFromAgent, updateRoundcubeThroughAgent
	t.Cleanup(func() {
		readRoundcubeUpdateFromAgent, readWebmailProofFromAgent, updateRoundcubeThroughAgent = restoreRead, restoreProof, restoreUpdate
	})
	var updates atomic.Int32
	updateRoundcubeThroughAgent = func(_ *Panel, ctx context.Context, _ string) (transport.RoundcubeUpdateResponse, error) {
		updates.Add(1)
		<-ctx.Done()
		return transport.RoundcubeUpdateResponse{}, ctx.Err()
	}
	readRoundcubeUpdateFromAgent = func(*Panel, context.Context) (transport.RoundcubeUpdateCheckResponse, error) {
		return transport.RoundcubeUpdateCheckResponse{Installed: true, Version: "1.6.21"}, nil
	}
	readWebmailProofFromAgent = func(*Panel, context.Context) (transport.WebmailProofResponse, error) {
		return transport.WebmailProofResponse{Proved: true, Version: "1.6.21"}, nil
	}
	agent.held.Store(true)
	agent.unanswered.Store(true)

	recorder := postWebmailUpdate(t, f.panel, f.userID, `{"version":"1.6.21","request_id":"`+mustServiceOperationRequestID(t)+`"}`)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("accepted: %d %s", recorder.Code, recorder.Body.String())
	}
	accepted := decodeServiceOperationEnvelope(t, recorder)
	shown := waitForServiceOperationPhase(t, f.panel, f.userID, accepted.ID, serviceOperationUnsettledPhase)
	if shown.Status != serviceOperationRunning {
		t.Fatalf("unsettled row: %+v", shown)
	}
	second, _ := postServiceInstall(t, f, "certbot")
	if reason, message := busyReason(t, second); reason != serviceOperationBusyReasonUnsettled || !strings.Contains(message, "sudo systemctl restart celikpanel-panel") {
		t.Fatalf("refusal of an unsettled row: %q %q", reason, message)
	}
	problems := f.panel.serviceOperationUnsettledProblems(context.Background())
	if len(problems) != 1 || problems[0].Kind != "service_operation_unsettled" || problems[0].Subject != "roundcube" ||
		problems[0].Actor != "server_owner" || problems[0].Since == "" {
		t.Fatalf("problems now: %+v", problems)
	}

	// The restart: startup recovery reads the Agent's record and settles the
	// row read only. Nothing is updated again.
	agent.unanswered.Store(false)
	agent.endStep(accepted.RequestID)
	if _, err := f.panel.recoverInterruptedServiceOperations(context.Background()); err != nil {
		t.Fatalf("startup recovery: %v", err)
	}
	done, _ := waitForServiceOperation(t, f.panel, f.userID, accepted.ID, serviceOperationSucceeded)
	if !strings.Contains(string(done.Result), `"outcome":"updated"`) {
		t.Fatalf("restart-settled row: %s", done.Result)
	}
	if n := updates.Load(); n != 1 {
		t.Fatalf("the update ran %d times", n)
	}
	if problems := f.panel.serviceOperationUnsettledProblems(context.Background()); len(problems) != 0 {
		t.Fatalf("a settled row is still a problem: %+v", problems)
	}
}

// Item 3: one renewal that is not answered in time does not stop a step
// that is running; the operation ends as it would have.
func TestD1OneUnansweredRenewalDoesNotStopTheStep(t *testing.T) {
	f, agent := newHeldStepFixture(t)
	shortenD1Bounds(t)
	clearD1Settles(t)
	restoreUpdate := updateRoundcubeThroughAgent
	t.Cleanup(func() { updateRoundcubeThroughAgent = restoreUpdate })
	var stopped atomic.Bool
	updateRoundcubeThroughAgent = func(_ *Panel, ctx context.Context, version string) (transport.RoundcubeUpdateResponse, error) {
		// The step outlasts several renewals; the first is not answered.
		select {
		case <-ctx.Done():
			stopped.Store(true)
			return transport.RoundcubeUpdateResponse{}, ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
		return transport.RoundcubeUpdateResponse{Updated: true, Proved: true, From: "1.6.20", To: version}, nil
	}
	agent.unansweredOne.Store(1)

	recorder := postWebmailUpdate(t, f.panel, f.userID, `{"version":"1.6.21","request_id":"`+mustServiceOperationRequestID(t)+`"}`)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("accepted: %d %s", recorder.Code, recorder.Body.String())
	}
	done, _ := waitForServiceOperation(t, f.panel, f.userID, decodeServiceOperationEnvelope(t, recorder).ID, serviceOperationSucceeded)
	if stopped.Load() || !strings.Contains(string(done.Result), `"outcome":"updated"`) || strings.Contains(string(done.Result), `"recovered"`) {
		t.Fatalf("stopped=%v row: %+v %s", stopped.Load(), done.Error, done.Result)
	}
	if agent.unansweredOne.Load() != 0 {
		t.Fatal("no renewal went unanswered")
	}
}

// Item 3, the rule itself: unanswered renewals stop the worker only after
// the window; the Agent's definite answer stops it at once.
func TestD1HeartbeatStopsTheWorkerOnlyOnADefiniteAnswerOrAfterItsWindow(t *testing.T) {
	requestID, ownerID := strings.Repeat("6", 32), strings.Repeat("7", 32)
	binding := agentMutationBinding{MutationRequestID: requestID, MutationOwnerID: ownerID}
	cases := []struct {
		name     string
		agent    *heartbeatFailureClassificationAgent
		window   time.Duration
		earliest time.Duration
	}{
		{"unanswered", &heartbeatFailureClassificationAgent{transportErr: errors.New("simulated timeout")}, 300 * time.Millisecond, 300 * time.Millisecond},
		{"lease lost", &heartbeatFailureClassificationAgent{job: &ServiceOperationMutationJob{
			RequestID: requestID, OwnerID: ownerID, Status: agentMutationFailed, ErrorCode: agentMutationLeaseExpiredErrorCode,
		}, responseError: "service mutation lease is not running"}, time.Hour, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			window := panelHeartbeatUncertainWindow.get()
			panelHeartbeatUncertainWindow.set(tc.window)
			t.Cleanup(func() { panelHeartbeatUncertainWindow.set(window) })
			panel := newPolicyDispatchTestPanel(t, tc.agent)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			began := time.Now()
			stop := panel.startObservedAgentMutationHeartbeat(ctx, cancel, binding, nil, 20*time.Millisecond)
			select {
			case <-ctx.Done():
			case <-time.After(5 * time.Second):
				t.Fatal("the worker was never stopped")
			}
			if elapsed := time.Since(began); elapsed < tc.earliest {
				t.Fatalf("stopped after %v, before the %v window", elapsed, tc.earliest)
			}
			if _, err := stop(); err == nil {
				t.Fatal("a stopped worker carries no reason")
			}
		})
	}
}
