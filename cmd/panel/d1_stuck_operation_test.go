package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/rpc"
	"os"
	"reflect"
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

// heldStepAgent is the test Agent with what D1 needs, modelled on the real
// one: renewals that go unanswered; install steps that are counted, so that
// Finish is refused while one runs, a lease that expires or is cancelled
// meanwhile stays "cancelling" until the step has ended, and a second step
// that starts while one runs is caught; and, for the webmail update (whose
// step is a seam, not an RPC here), a step held from outside.
type heldStepAgent struct {
	*serviceOperationTestAgent

	unanswered    atomic.Bool  // every renewal goes unanswered while set
	unansweredOne atomic.Int32 // this many single renewals go unanswered
	statusFails   atomic.Bool  // every status read fails while set
	held          atomic.Bool  // a step outside the RPCs still runs
	beginBusy     atomic.Int32 // this many leases are refused as host busy
	finishRefused chan struct{}
	refusedOnce   sync.Once
	refusals      atomic.Int32

	steps     atomic.Int32
	overlap   atomic.Bool
	calls     atomic.Int32
	started   chan int32
	gateMu    sync.Mutex
	gates     map[int32]chan struct{}
	pending   map[string]string // request id -> code once its step ends
	foreignFW atomic.Bool       // one foreign job after the first firewall apply
	foreignOK atomic.Bool
	vanish    atomic.Bool // a job read as ended is gone from the ledger after that read
}

func newHeldStepAgent(base *serviceOperationTestAgent) *heldStepAgent {
	return &heldStepAgent{
		serviceOperationTestAgent: base, finishRefused: make(chan struct{}),
		started: make(chan int32, 16), gates: map[int32]chan struct{}{}, pending: map[string]string{},
	}
}

// gate is the channel install call n waits on (created closed = open when
// the test opens it before the call).
func (a *heldStepAgent) gate(n int32) chan struct{} {
	a.gateMu.Lock()
	defer a.gateMu.Unlock()
	g, ok := a.gates[n]
	if !ok {
		g = make(chan struct{})
		a.gates[n] = g
	}
	return g
}

func (a *heldStepAgent) open(n int32) {
	g := a.gate(n)
	select {
	case <-g:
	default:
		close(g)
	}
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

func (a *heldStepAgent) ServiceMutationStatus(req *ServiceOperationMutationStatusRequest, resp *ServiceOperationMutationResponse) error {
	if a.statusFails.Load() {
		return errors.New("simulated agent that does not answer")
	}
	if req.RequestID == "" && a.foreignFW.Load() && !a.foreignOK.Load() {
		a.mu.Lock()
		applied := a.firewallCalls > 0
		a.mu.Unlock()
		if applied {
			a.foreignOK.Store(true)
			resp.Job = &ServiceOperationMutationJob{
				RequestID: strings.Repeat("f", 32), OwnerID: strings.Repeat("e", 32),
				Kind: "mail_renewal", Target: "mail-tls", Status: agentMutationRunning,
			}
			return nil
		}
	}
	err := a.serviceOperationTestAgent.ServiceMutationStatus(req, resp)
	if req.RequestID != "" && a.vanish.Load() && resp.Job != nil && !agentMutationActive(resp.Job.Status) {
		// The Agent's ledger is reset between this read and the next.
		a.mu.Lock()
		delete(a.mutationJobs, req.RequestID)
		a.mu.Unlock()
	}
	return err
}

func (a *heldStepAgent) BeginServiceMutation(req *ServiceOperationMutationBeginRequest, resp *ServiceOperationMutationResponse) error {
	if a.beginBusy.Load() > 0 {
		a.beginBusy.Add(-1)
		resp.ErrorCode, resp.Reason = transport.HostMutationBusy, "package_manager"
		return nil
	}
	return a.serviceOperationTestAgent.BeginServiceMutation(req, resp)
}

func (a *heldStepAgent) FinishServiceMutation(req *ServiceOperationMutationFinishRequest, resp *ServiceOperationMutationResponse) error {
	if a.held.Load() || a.steps.Load() > 0 {
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

// CancelServiceMutation (startup recovery) cancels a running job; while a
// step runs, the job stays "cancelling" until the step has ended.
func (a *heldStepAgent) CancelServiceMutation(req *ServiceOperationMutationCancelRequest, resp *ServiceOperationMutationResponse) error {
	if a.steps.Load() > 0 {
		a.mu.Lock()
		defer a.mu.Unlock()
		job := a.mutationJobs[req.RequestID]
		if job == nil || job.OwnerID != req.ExpectedOwner {
			resp.Error = "service mutation owner mismatch"
			return nil
		}
		job.Status, job.Phase = agentMutationCancelling, "cancelling"
		a.pending[req.RequestID] = req.FailureCode
		a.mutationEvents = append(a.mutationEvents, "cancel:"+job.Kind)
		resp.Job = cloneServiceOperationMutationJob(job)
		return nil
	}
	return a.serviceOperationTestAgent.CancelServiceMutation(req, resp)
}

// InstallService is one counted step: it waits on its gate, and a second
// step that starts while one runs is recorded as an overlap.
func (a *heldStepAgent) InstallService(req *ServiceOperationInstallRequest, resp *ServiceOperationInstallResponse) error {
	n := a.calls.Add(1)
	if a.steps.Add(1) > 1 {
		a.overlap.Store(true)
	}
	a.started <- n
	<-a.gate(n)
	err := a.serviceOperationTestAgent.InstallService(req, resp)
	if a.steps.Add(-1) == 0 {
		a.mu.Lock()
		for requestID, code := range a.pending {
			if job := a.mutationJobs[requestID]; job != nil && job.Status == agentMutationCancelling {
				job.Status, job.Phase, job.ErrorCode = agentMutationFailed, "failed", code
				if a.mutationActive == requestID {
					a.mutationActive = ""
				}
			}
			delete(a.pending, requestID)
		}
		a.mu.Unlock()
	}
	return err
}

// expireJob is the Agent's watchdog once nobody renews the lease: the job
// fails as lease-expired, or stays cancelling until its step has ended.
func (a *heldStepAgent) expireJob(requestID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	job := a.mutationJobs[requestID]
	if job == nil || !agentMutationActive(job.Status) {
		return
	}
	job.ErrorMessage = "The panel stopped heartbeating before the service mutation completed."
	if a.steps.Load() > 0 {
		job.Status, job.Phase = agentMutationCancelling, "cancelling_expired_lease"
		a.pending[requestID] = agentMutationLeaseExpiredErrorCode
		return
	}
	job.Status, job.Phase, job.ErrorCode = agentMutationFailed, "failed", agentMutationLeaseExpiredErrorCode
	if a.mutationActive == requestID {
		a.mutationActive = ""
	}
	a.mutationEvents = append(a.mutationEvents, "expired:"+job.Kind)
}

// endStep ends a held (seam) step; the lease nobody renewed expires.
func (a *heldStepAgent) endStep(requestID string) {
	a.held.Store(false)
	a.expireJob(requestID)
}

func (a *heldStepAgent) jobStatus(requestID string) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if job := a.mutationJobs[requestID]; job != nil {
		return job.Status
	}
	return ""
}

// attachHeldStepAgent connects p to agent; the returned function makes every
// connection of p fail from then on (its process has gone).
func attachHeldStepAgent(t *testing.T, p *Panel, agent *heldStepAgent) func() {
	t.Helper()
	server := rpc.NewServer()
	if err := server.RegisterName("Agent", agent); err != nil {
		t.Fatalf("register held-step agent: %v", err)
	}
	var mu sync.Mutex
	var clients []*rpc.Client
	dead := false
	connector := func(ctx context.Context) (*rpc.Client, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		mu.Lock()
		defer mu.Unlock()
		if dead {
			return nil, errors.New("this panel process has gone")
		}
		serverConn, clientConn := net.Pipe()
		go server.ServeConn(serverConn)
		client := rpc.NewClient(clientConn)
		clients = append(clients, client)
		return client, nil
	}
	client, err := connector(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	p.agentClient = transport.NewReconnectingClientWithContextConnector(client, connector)
	kill := func() {
		mu.Lock()
		defer mu.Unlock()
		dead = true
		for _, c := range clients {
			_ = c.Close()
		}
	}
	t.Cleanup(kill)
	return kill
}

func newHeldStepFixture(t *testing.T) (serviceOperationTestFixture, *heldStepAgent, func()) {
	t.Helper()
	f := newServiceOperationTestFixture(t)
	agent := newHeldStepAgent(f.agent)
	kill := attachHeldStepAgent(t, f.panel, agent)
	t.Cleanup(func() {
		agent.gateMu.Lock()
		for _, g := range agent.gates {
			select {
			case <-g:
			default:
				close(g)
			}
		}
		agent.gateMu.Unlock()
	})
	return f, agent, kill
}

// shortenD1Bounds makes the heartbeat and the settling reads fast.
func shortenD1Bounds(t *testing.T) {
	t.Helper()
	interval, window := panelOperationHeartbeatInterval.get(), panelHeartbeatUncertainWindow.get()
	poll, retry, margin := serviceOperationSettlePoll.get(), serviceOperationSettleRetry.get(), serviceOperationSettleMargin.get()
	silence, busy, pause := serviceOperationAgentSilence.get(), serviceOperationResumeBusyBound.get(), panelHeartbeatRetryPause.get()
	panelOperationHeartbeatInterval.set(20 * time.Millisecond)
	panelHeartbeatUncertainWindow.set(100 * time.Millisecond)
	panelHeartbeatRetryPause.set(10 * time.Millisecond)
	serviceOperationSettlePoll.set(10 * time.Millisecond)
	serviceOperationSettleRetry.set(20 * time.Millisecond)
	serviceOperationAgentSilence.set(2 * time.Second)
	serviceOperationResumeBusyBound.set(2 * time.Second)
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
		serviceOperationAgentSilence.set(silence)
		serviceOperationResumeBusyBound.set(busy)
		panelHeartbeatRetryPause.set(pause)
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

func waitForInstallCall(t *testing.T, agent *heldStepAgent, want int32) {
	t.Helper()
	select {
	case n := <-agent.started:
		if n != want {
			t.Fatalf("install call %d started, want %d", n, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("install call %d never started", want)
	}
}

func waitFor(t *testing.T, what string, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !done() {
		if time.Now().After(deadline) {
			t.Fatalf("%s did not happen", what)
		}
		time.Sleep(2 * time.Millisecond)
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

// seedInterruptedOperation is a running row whose Agent job has ended with
// status/code: what a running panel meets after its Finish failed.
func seedInterruptedOperation(t *testing.T, f serviceOperationTestFixture, agent *serviceOperationTestAgent, kind, target, pkg, status, code string) serviceOperation {
	t.Helper()
	op, err := f.panel.createServiceOperationRequest(context.Background(), kind, target, pkg, mustServiceOperationRequestID(t), serviceOperationActor{UserID: f.userID})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.panel.markServiceOperationRunning(context.Background(), op.ID, "installing"); err != nil {
		t.Fatal(err)
	}
	op.Status = serviceOperationRunning
	if status != "" {
		agent.mu.Lock()
		agent.mutationJobs[op.RequestID] = &ServiceOperationMutationJob{
			RequestID: op.RequestID, OwnerID: strings.Repeat("a", 32), Kind: kind, Target: target, PackageName: pkg,
			Status: status, ErrorCode: code, DeadlineAt: time.Now().Add(time.Hour),
		}
		agent.mu.Unlock()
	}
	return op
}

// settleDirectly runs the live settle for op as the launch goroutine does,
// holding the process lock; it reports whether the lock went to a
// continuation, and waits for the lock to come back.
func settleDirectly(t *testing.T, p *Panel, op serviceOperation, deadline time.Time) bool {
	t.Helper()
	p.serviceMutationMu.Lock()
	var once sync.Once
	release := func() { once.Do(p.serviceMutationMu.Unlock) }
	transferred := p.settleAbandonedServiceOperation(op, deadline, release)
	if !transferred {
		release()
	}
	return transferred
}

// The read-only family (webmail update; a webmail address below): the
// diagnosis' test. A held step, renewals unanswered until the worker is
// stopped, Finish refused, the step released: the row settles by itself as
// "updated" and a second operation is admitted, without startup recovery.
// The update is never started a second time.
func TestD1WebmailUpdateSettlesItselfAfterTheAgentStepEnds(t *testing.T) {
	f, agent, _ := newHeldStepFixture(t)
	shortenD1Bounds(t)
	clearD1Settles(t)
	agent.open(1)
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

// The run-again family (component install; install-together,
// remove-together, a mail profile and a runtime are run again the same
// way): the lease expires while the first install step still runs, so the
// Agent keeps the job "cancelling"; no second step starts until the first
// has ended, then the install is run once more from its start.
func TestD1ComponentInstallIsRunOnceMoreOnlyAfterItsStepEnded(t *testing.T) {
	f, agent, _ := newHeldStepFixture(t)
	shortenD1Bounds(t)
	clearD1Settles(t)
	audited := observeServiceOperationAudits(f.panel)
	agent.unanswered.Store(true)

	recorder, queued := postServiceInstall(t, f, "certbot")
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("install: %d %s", recorder.Code, recorder.Body.String())
	}
	waitForInstallCall(t, agent, 1)
	waitForClosed(t, agent.finishRefused, "the refused Finish")
	waitForServiceOperationPhase(t, f.panel, f.userID, queued.ID, serviceOperationSettleContactPhase)

	// The Agent's watchdog ends the lease while the step still runs: the job
	// is "cancelling", and the panel must not start the install again yet.
	agent.unanswered.Store(false)
	agent.expireJob(queued.RequestID)
	time.Sleep(150 * time.Millisecond)
	if calls := agent.calls.Load(); calls != 1 || agent.jobStatus(queued.RequestID) != agentMutationCancelling {
		t.Fatalf("while the first step runs: calls=%d status=%s", calls, agent.jobStatus(queued.RequestID))
	}
	agent.open(2)
	agent.open(1)
	waitForInstallCall(t, agent, 2)

	done, _ := waitForServiceOperation(t, f.panel, f.userID, queued.ID, serviceOperationSucceeded)
	waitForServiceOperationAudit(t, audited, "service.install.recovered:certbot")
	var result map[string]any
	if err := json.Unmarshal(done.Result, &result); err != nil || result["installed"] != true {
		t.Fatalf("row: %v %s", err, done.Result)
	}
	if calls := agent.calls.Load(); calls != 2 || agent.overlap.Load() {
		t.Fatalf("install calls = %d, overlap = %v", calls, agent.overlap.Load())
	}
	// A second operation is admitted, without startup recovery.
	agent.open(3)
	recorder, next := postServiceInstall(t, f, "certbot")
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("second operation: %d %s", recorder.Code, recorder.Body.String())
	}
	waitForServiceOperation(t, f.panel, f.userID, next.ID, serviceOperationSucceeded)
}

// A running panel runs a row again at most once: when that run loses its
// Agent too, the row is left to the restart, and nothing runs a third time.
func TestD1APanelRunsARowAgainAtMostOnce(t *testing.T) {
	f, agent, _ := newHeldStepFixture(t)
	shortenD1Bounds(t)
	clearD1Settles(t)
	agent.unanswered.Store(true)

	recorder, queued := postServiceInstall(t, f, "certbot")
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("install: %d %s", recorder.Code, recorder.Body.String())
	}
	waitForInstallCall(t, agent, 1)
	waitForClosed(t, agent.finishRefused, "the refused Finish")
	agent.expireJob(queued.RequestID)
	agent.open(1)
	// The second run starts and loses its Agent the same way.
	waitForInstallCall(t, agent, 2)
	waitFor(t, "the second run's refused Finish", func() bool { return agent.refusals.Load() >= 2 })
	agent.expireJob(queued.RequestID)
	agent.open(2)
	shown := waitForServiceOperationPhase(t, f.panel, f.userID, queued.ID, serviceOperationUnsettledPhase)
	if shown.Status != serviceOperationRunning {
		t.Fatalf("row: %+v", shown)
	}
	select {
	case n := <-agent.started:
		t.Fatalf("install call %d started by itself", n)
	case <-time.After(300 * time.Millisecond):
	}
	if calls := agent.calls.Load(); calls != 2 || agent.overlap.Load() {
		t.Fatalf("install calls = %d, overlap = %v", calls, agent.overlap.Load())
	}
}

// A panel restart while the running panel's second run still has its step
// running: startup recovery cancels it, waits until the step has ended, and
// runs it exactly once more; no two steps overlap.
func TestD1ARestartDuringTheSecondRunRunsItOnceMoreWithoutOverlap(t *testing.T) {
	f, agent, kill := newHeldStepFixture(t)
	shortenD1Bounds(t)
	clearD1Settles(t)
	serviceOperationAgentSilence.set(200 * time.Millisecond)
	agent.unanswered.Store(true)

	recorder, queued := postServiceInstall(t, f, "certbot")
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("install: %d %s", recorder.Code, recorder.Body.String())
	}
	waitForInstallCall(t, agent, 1)
	waitForClosed(t, agent.finishRefused, "the refused Finish")
	agent.unanswered.Store(false)
	agent.expireJob(queued.RequestID)
	agent.open(1)
	waitForInstallCall(t, agent, 2) // the second run's step now runs

	// The panel process goes; a new one starts on the same database.
	kill()
	restarted := &Panel{db: f.database}
	attachHeldStepAgent(t, restarted, agent)
	recovered := make(chan error, 1)
	go func() {
		_, err := restarted.recoverInterruptedServiceOperations(context.Background())
		recovered <- err
	}()
	waitFor(t, "the restart's cancel", func() bool { return agent.jobStatus(queued.RequestID) == agentMutationCancelling })
	time.Sleep(150 * time.Millisecond)
	if calls := agent.calls.Load(); calls != 2 {
		t.Fatalf("a run started while the second step still ran: calls=%d", calls)
	}
	agent.open(3)
	agent.open(2)
	if err := <-recovered; err != nil {
		t.Fatalf("startup recovery: %v", err)
	}
	waitForInstallCall(t, agent, 3)
	waitForServiceOperation(t, restarted, f.userID, queued.ID, serviceOperationSucceeded)
	if calls := agent.calls.Load(); calls != 3 || agent.overlap.Load() {
		t.Fatalf("install calls = %d, overlap = %v", calls, agent.overlap.Load())
	}
}

// When the row cannot be settled (here: the Agent's step never ends before
// the job's deadline), the row says so, every refusal names the Agent check
// and the restart, Problems now carries one line, and startup recovery (the
// restart that line asks for) settles it.
func TestD1AnUnsettledRowNamesTheAgentCheckAndTheRestartSettlesIt(t *testing.T) {
	f, agent, _ := newHeldStepFixture(t)
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
	reason, message := busyReason(t, second)
	agentCheck, restart := strings.Index(message, "systemctl status celikpanel-agent"), strings.Index(message, "systemctl restart celikpanel-panel")
	if reason != serviceOperationBusyReasonUnsettled || agentCheck < 0 || restart < agentCheck {
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

// Finding 4: an Agent that answers no read for the silence bound leaves the
// row unsettled and the process lock released at once (not at the job's
// 45-minute deadline); the job's own lease keeps everything else off the
// host meanwhile.
func TestD1AnAgentThatStopsAnsweringReleasesTheLock(t *testing.T) {
	f, agent, _ := newHeldStepFixture(t)
	shortenD1Bounds(t)
	clearD1Settles(t)
	serviceOperationAgentSilence.set(150 * time.Millisecond)
	restoreUpdate := updateRoundcubeThroughAgent
	t.Cleanup(func() { updateRoundcubeThroughAgent = restoreUpdate })
	updateRoundcubeThroughAgent = func(_ *Panel, ctx context.Context, _ string) (transport.RoundcubeUpdateResponse, error) {
		<-ctx.Done()
		agent.statusFails.Store(true) // the Agent stops answering
		return transport.RoundcubeUpdateResponse{}, ctx.Err()
	}
	agent.held.Store(true)
	agent.unanswered.Store(true)
	t.Cleanup(func() { agent.statusFails.Store(false) })

	recorder := postWebmailUpdate(t, f.panel, f.userID, `{"version":"1.6.21","request_id":"`+mustServiceOperationRequestID(t)+`"}`)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("accepted: %d %s", recorder.Code, recorder.Body.String())
	}
	accepted := decodeServiceOperationEnvelope(t, recorder)
	shown := waitForServiceOperationPhase(t, f.panel, f.userID, accepted.ID, serviceOperationUnsettledPhase)
	if shown.Status != serviceOperationRunning {
		t.Fatalf("row: %+v", shown)
	}
	waitFor(t, "the process lock's release", func() bool {
		if f.panel.serviceMutationMu.TryLock() {
			f.panel.serviceMutationMu.Unlock()
			return true
		}
		return false
	})
}

// Finding 7: an Agent that has no record of the job has not "ended" it: the
// row stays running as unsettled, never failed with "the panel restarted".
func TestD1AnAgentWithoutARecordIsUnknownNotEnded(t *testing.T) {
	f, _, _ := newHeldStepFixture(t)
	shortenD1Bounds(t)
	clearD1Settles(t)
	op := seedInterruptedOperation(t, f, f.agent, serviceOperationKindWebmailUpdate, "roundcube", "1.6.21", "", "")
	if settleDirectly(t, f.panel, op, time.Now().Add(time.Hour)) {
		t.Fatal("a job without a record was run again")
	}
	shown, body := getServiceOperation(t, f.panel, f.userID, op.ID)
	if shown.Status != serviceOperationRunning || shown.Phase != serviceOperationUnsettledPhase || strings.Contains(body, "restarted") {
		t.Fatalf("row: %s", body)
	}
}

// Second look, finding 4: the reconciliation's own read also finds no
// record (the Agent's ledger was reset after the wait read the job as
// ended): in a running panel the row is left unsettled, never failed with
// "the panel restarted".
func TestD1InTheRunningPanelTheReconcilesNoRecordLeavesTheRowUnsettled(t *testing.T) {
	f, agent, _ := newHeldStepFixture(t)
	shortenD1Bounds(t)
	clearD1Settles(t)
	agent.vanish.Store(true)
	op := seedInterruptedOperation(t, f, f.agent, serviceOperationKindInstall, "certbot", "", agentMutationFailed, agentMutationLeaseExpiredErrorCode)
	if settleDirectly(t, f.panel, op, time.Now().Add(time.Hour)) {
		t.Fatal("a job without a record was run again")
	}
	shown, body := getServiceOperation(t, f.panel, f.userID, op.ID)
	if shown.Status != serviceOperationRunning || shown.Phase != serviceOperationUnsettledPhase || strings.Contains(body, "restarted") || agent.calls.Load() != 0 {
		t.Fatalf("row: %s", body)
	}
}

// Second look, finding 5: a server setup step waits for a busy host as long
// as setup itself does, not the two minutes of an ordinary run again.
func TestD1ASetupStepWaitsForABusyHostAsLongAsSetupDoes(t *testing.T) {
	f, agent, _ := newHeldStepFixture(t)
	shortenD1Bounds(t)
	clearD1Settles(t)
	serviceOperationResumeBusyBound.set(60 * time.Millisecond)
	restoreSetup := serverSetupHostBusyBound
	serverSetupHostBusyBound = 5 * time.Second
	t.Cleanup(func() { serverSetupHostBusyBound = restoreSetup })
	agent.open(1)
	// Refused for longer than the ordinary bound (pauses 20+40+80+160 ms).
	agent.beginBusy.Store(4)
	op := seedInterruptedOperation(t, f, f.agent, serviceOperationKindInstall, "certbot", "", agentMutationFailed, agentMutationLeaseExpiredErrorCode)
	// The row is a step of a server setup run.
	conn, err := f.database.GetDB().Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{`PRAGMA foreign_keys = OFF`,
		`INSERT INTO server_setup_executions (id, request_id, plan_id, status, execution_json, created_at, updated_at) VALUES ('` + strings.Repeat("d", 32) + `', '` + strings.Repeat("d", 32) + `', 'plan', 'running', '{"steps":[{"request_id":"` + op.RequestID + `"}]}', 'x', 'x')`,
		`PRAGMA foreign_keys = ON`} {
		if _, err := conn.ExecContext(context.Background(), statement); err != nil {
			t.Fatal(err)
		}
	}
	_ = conn.Close()
	if !f.panel.markServerSetupChild(context.Background(), op).SetupChild {
		t.Fatal("fixture: the row is not a setup step")
	}
	if !settleDirectly(t, f.panel, op, time.Now().Add(time.Hour)) {
		t.Fatal("the row was not handed to its run again")
	}
	waitForServiceOperation(t, f.panel, f.userID, op.ID, serviceOperationSucceeded)
	if agent.beginBusy.Load() != 0 || agent.calls.Load() != 1 {
		t.Fatalf("busy refusals left %d, install calls %d", agent.beginBusy.Load(), agent.calls.Load())
	}
}

// Second look, finding 7: point updates read back from a row are used only
// when bounded and each is an exact "package=version"; otherwise the list
// is dropped.
func TestD1AcceptedPointUpdatesReadBackAreValidated(t *testing.T) {
	for name, items := range map[string][]string{
		"not package=version": {"libc6=2.41-12", "rm -rf /"},
		"no version":          {"openssl="},
		"too many":            make([]string, maxAcceptedPointUpdates+1),
	} {
		t.Run(name, func(t *testing.T) {
			for i := range items {
				if items[i] == "" && name == "too many" {
					items[i] = "libc6=2.41-12"
				}
			}
			raw, _ := json.Marshal(serviceInstallOperationData{AcceptedPointUpdates: items})
			var resumed serviceInstallRequest
			restoreServiceInstallChoices(&resumed, string(raw))
			if len(resumed.AcceptedPointUpdates) != 0 {
				t.Fatalf("restored %d entries", len(resumed.AcceptedPointUpdates))
			}
		})
	}
	good := []string{"libc6=2.41-12", "openssl=3.5.1-1+deb13u1", "tzdata=2025b-4+deb13u1", "libssl3t64=3.0.13-0ubuntu3.5", "bind9=1:9.20.11-4"}
	var resumed serviceInstallRequest
	restoreServiceInstallChoices(&resumed, encodeServiceInstallOperationData(serviceInstallRequest{AcceptedPointUpdates: good}))
	if !reflect.DeepEqual(resumed.AcceptedPointUpdates, good) {
		t.Fatalf("valid list restored as %v", resumed.AcceptedPointUpdates)
	}
}

// Finding 5: once a privileged sync has run, a reconciliation that fails is
// never asked again by the running panel: the firewall is applied once and
// the row is left to the restart, not recorded as failed.
func TestD1AfterAPrivilegedSyncTheReconciliationIsNotRunAgain(t *testing.T) {
	f, agent, _ := newHeldStepFixture(t)
	shortenD1Bounds(t)
	clearD1Settles(t)
	f.agent.mu.Lock()
	f.agent.firewallEnabled = true
	f.agent.mu.Unlock()
	agent.foreignFW.Store(true)
	op := seedInterruptedOperation(t, f, f.agent, serviceOperationKindInstall, "pdns", "", agentMutationSucceeded, "")
	if settleDirectly(t, f.panel, op, time.Now().Add(time.Hour)) {
		t.Fatal("a succeeded job was run again")
	}
	f.agent.mu.Lock()
	applied := f.agent.firewallCalls
	f.agent.mu.Unlock()
	shown, body := getServiceOperation(t, f.panel, f.userID, op.ID)
	if applied != 1 || !agent.foreignOK.Load() || shown.Status != serviceOperationRunning || shown.Phase != serviceOperationUnsettledPhase {
		t.Fatalf("firewall applied %d times (foreign job seen: %v); row %s", applied, agent.foreignOK.Load(), body)
	}
}

// Finding 6: a run again that the Agent refuses because the host is busy is
// asked again for a bounded time; it starts once the host is free, and a
// host that stays busy leaves the row unsettled, never "not started".
func TestD1ARunAgainTheBusyHostRefusesIsAskedAgainThenLeftUnsettled(t *testing.T) {
	t.Run("the host becomes free", func(t *testing.T) {
		f, agent, _ := newHeldStepFixture(t)
		shortenD1Bounds(t)
		clearD1Settles(t)
		agent.open(1)
		agent.beginBusy.Store(2)
		op := seedInterruptedOperation(t, f, f.agent, serviceOperationKindInstall, "certbot", "", agentMutationFailed, agentMutationLeaseExpiredErrorCode)
		if !settleDirectly(t, f.panel, op, time.Now().Add(time.Hour)) {
			t.Fatal("the row was not run again")
		}
		waitForServiceOperation(t, f.panel, f.userID, op.ID, serviceOperationSucceeded)
		if calls := agent.calls.Load(); calls != 1 || agent.beginBusy.Load() != 0 {
			t.Fatalf("calls=%d busy refusals left=%d", calls, agent.beginBusy.Load())
		}
	})
	t.Run("the host stays busy", func(t *testing.T) {
		f, agent, _ := newHeldStepFixture(t)
		shortenD1Bounds(t)
		clearD1Settles(t)
		serviceOperationResumeBusyBound.set(150 * time.Millisecond)
		agent.beginBusy.Store(1 << 20)
		op := seedInterruptedOperation(t, f, f.agent, serviceOperationKindInstall, "certbot", "", agentMutationFailed, agentMutationLeaseExpiredErrorCode)
		if !settleDirectly(t, f.panel, op, time.Now().Add(time.Hour)) {
			t.Fatal("the row was not handed to its run again")
		}
		shown := waitForServiceOperationPhase(t, f.panel, f.userID, op.ID, serviceOperationUnsettledPhase)
		if shown.Status != serviceOperationRunning || agent.calls.Load() != 0 {
			t.Fatalf("row %+v, calls %d", shown, agent.calls.Load())
		}
		waitFor(t, "the process lock's release", func() bool {
			if f.panel.serviceMutationMu.TryLock() {
				f.panel.serviceMutationMu.Unlock()
				return true
			}
			return false
		})
	})
}

// The same at startup, where there is no further step: after the bounded
// retries the row ends with what is true (not "could not be started"), and
// other changes are admitted again; a webmail update says its result could
// not be read.
func TestD1AtStartupARunAgainTheHostKeepsRefusingEndsTheRow(t *testing.T) {
	for _, tc := range []struct {
		kind, target, pkg, code string
	}{
		{serviceOperationKindInstall, "certbot", "", errCodeServiceOperationRerunHostBusy},
		{serviceOperationKindWebmailUpdate, "roundcube", "1.6.21", errCodeServiceOperationRereadHostBusy},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			f, agent, _ := newHeldStepFixture(t)
			shortenD1Bounds(t)
			clearD1Settles(t)
			serviceOperationResumeBusyBound.set(150 * time.Millisecond)
			agent.beginBusy.Store(1 << 20)
			op := seedInterruptedOperation(t, f, f.agent, tc.kind, tc.target, tc.pkg, agentMutationFailed, agentMutationLeaseExpiredErrorCode)
			if _, err := f.panel.recoverInterruptedServiceOperations(context.Background()); err != nil {
				t.Fatalf("startup recovery: %v", err)
			}
			ended, body := waitForServiceOperation(t, f.panel, f.userID, op.ID, serviceOperationFailed)
			if ended.Error == nil || ended.Error.Code != tc.code || strings.Contains(body, "could not be started") ||
				!strings.Contains(ended.Error.Message, "not known") || agent.calls.Load() != 0 {
				t.Fatalf("ended row: %s (install calls %d)", body, agent.calls.Load())
			}
			if reason := serviceOperationSettleBusyReason(); reason != "" {
				t.Fatalf("an ended row still marks refusals: %q", reason)
			}
			// Other changes are admitted again (the ended run releases the
			// process lock right after it records the row).
			agent.beginBusy.Store(0)
			agent.open(1)
			waitFor(t, "the process lock's release", func() bool {
				if f.panel.serviceMutationMu.TryLock() {
					f.panel.serviceMutationMu.Unlock()
					return true
				}
				return false
			})
			recorder, next := postServiceInstall(t, f, "certbot")
			if recorder.Code != http.StatusAccepted {
				t.Fatalf("next change: %d %s", recorder.Code, recorder.Body.String())
			}
			waitForServiceOperation(t, f.panel, f.userID, next.ID, serviceOperationSucceeded)
		})
	}
}

// Finding 6: a setup step's accepted point updates travel with its row, so
// an install run again is asked as the review accepted it.
func TestD1ASetupInstallKeepsItsAcceptedPointUpdatesWhenRunAgain(t *testing.T) {
	accepted := []string{"libc6=2.41-12", "openssl=3.5.1-1"}
	data := encodeServiceInstallOperationData(serviceInstallRequest{AcceptedPointUpdates: accepted})
	var resumed serviceInstallRequest
	restoreServiceInstallChoices(&resumed, data)
	if !reflect.DeepEqual(resumed.AcceptedPointUpdates, accepted) {
		t.Fatalf("data %s restored %v", data, resumed.AcceptedPointUpdates)
	}
	// A request body never carries them (json "-").
	var fromBody serviceInstallRequest
	if err := json.Unmarshal([]byte(`{"service_id":"certbot","AcceptedPointUpdates":["x=1"],"setup_accepted_point_updates":["x=1"]}`), &fromBody); err != nil || len(fromBody.AcceptedPointUpdates) != 0 {
		t.Fatalf("a body set accepted point updates: %v %v", fromBody.AcceptedPointUpdates, err)
	}
}

// The read-only family, webmail address: settled by reading the server's
// state; the address is never set up a second time.
func TestD1AWebmailAddressIsOnlyReadWhenSettled(t *testing.T) {
	f, _, _ := newHeldStepFixture(t)
	shortenD1Bounds(t)
	clearD1Settles(t)
	restoreRead, restoreEnable := readWebmailAddressStatusFromAgent, enableWebmailAddressThroughAgent
	t.Cleanup(func() {
		readWebmailAddressStatusFromAgent, enableWebmailAddressThroughAgent = restoreRead, restoreEnable
	})
	var enables atomic.Int32
	enableWebmailAddressThroughAgent = func(*Panel, context.Context, transport.WebmailAddressEnableRequest) (transport.WebmailAddressResponse, error) {
		enables.Add(1)
		return transport.WebmailAddressResponse{}, errors.New("must not be called")
	}
	readWebmailAddressStatusFromAgent = func(*Panel, context.Context, string, bool) (transport.WebmailAddressStatusResponse, error) {
		return transport.WebmailAddressStatusResponse{Present: true, Managed: true, Proved: true}, nil
	}
	op := seedInterruptedOperation(t, f, f.agent, serviceOperationKindWebmailAddress, "roundcube", "mail.example.test", agentMutationFailed, agentMutationLeaseExpiredErrorCode)
	if !settleDirectly(t, f.panel, op, time.Now().Add(time.Hour)) {
		t.Fatal("the row was not reconciled")
	}
	done, _ := waitForServiceOperation(t, f.panel, f.userID, op.ID, serviceOperationSucceeded)
	if enables.Load() != 0 || !strings.Contains(string(done.Result), `"recovered":true`) {
		t.Fatalf("enables=%d row %s", enables.Load(), done.Result)
	}
}

// The run-again family, mail profile: run once more from its first service.
func TestD1AMailProfileIsRunOnceMoreWhenSettled(t *testing.T) {
	fixture, agent := newMailProfileTestFixture(t)
	shortenD1Bounds(t)
	clearD1Settles(t)
	seedInstalledServices(agent.serviceOperationTestAgent, "postfix")
	agent.serviceOperationTestAgent.mu.Lock()
	agent.active["postfix"] = true
	agent.serviceOperationTestAgent.mu.Unlock()
	op := seedProfileOperation(t, fixture, "core-mail", mailProfilePhase("core-mail", "dovecot", "installing"))
	agent.serviceOperationTestAgent.mu.Lock()
	agent.mutationJobs[op.RequestID] = &ServiceOperationMutationJob{
		RequestID: op.RequestID, OwnerID: strings.Repeat("c", 32),
		Kind: serviceOperationKindMailProfileInstall, Target: "core-mail",
		Status: agentMutationFailed, ErrorCode: agentMutationLeaseExpiredErrorCode,
	}
	agent.serviceOperationTestAgent.mu.Unlock()
	if !settleDirectly(t, fixture.panel, op, time.Now().Add(time.Hour)) {
		t.Fatal("the profile was not run again")
	}
	waitForServiceOperation(t, fixture.panel, fixture.userID, op.ID, serviceOperationSucceeded)
	var installed []string
	for _, call := range agent.callsSnapshot() {
		if call.Name == "install" {
			installed = append(installed, call.ServiceID)
		}
	}
	if !reflect.DeepEqual(installed, []string{"postfix", "dovecot"}) {
		t.Fatalf("installs=%v", installed)
	}
}

// Item 3: one renewal that is not answered in time does not stop a step
// that is running; the operation ends as it would have.
func TestD1OneUnansweredRenewalDoesNotStopTheStep(t *testing.T) {
	f, agent, _ := newHeldStepFixture(t)
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

// blockingRenewalAgent answers its first renewals, then lets every renewal
// hang (an Agent that stalls); slow, when set, is one renewal (by number)
// that hangs for slowFor and then answers.
type blockingRenewalAgent struct {
	answered  int32
	slow      int32
	slowFor   time.Duration
	calls     atomic.Int32
	block     chan struct{}
	requestID string
	ownerID   string
}

func (a *blockingRenewalAgent) PkgFamily(_ *transport.Empty, out *string) error {
	*out = "apt"
	return nil
}

func (a *blockingRenewalAgent) HeartbeatServiceMutation(_ *ServiceOperationMutationHeartbeatRequest, resp *ServiceOperationMutationResponse) error {
	n := a.calls.Add(1)
	if n == a.slow {
		time.Sleep(a.slowFor)
	}
	if n <= a.answered || a.answered < 0 {
		resp.Job = &ServiceOperationMutationJob{RequestID: a.requestID, OwnerID: a.ownerID, Status: agentMutationRunning}
		return nil
	}
	<-a.block
	return errors.New("released")
}

// heartbeatAtProductionRatio sets interval : window = 1 : 3 (production:
// 5 s : 15 s), scaled to 100 ms : 300 ms, with a prompt retry of 30 ms.
func heartbeatAtProductionRatio(t *testing.T) (interval, window time.Duration) {
	t.Helper()
	restoreWindow, restorePause := panelHeartbeatUncertainWindow.get(), panelHeartbeatRetryPause.get()
	interval, window = 100*time.Millisecond, 300*time.Millisecond
	panelHeartbeatUncertainWindow.set(window)
	panelHeartbeatRetryPause.set(30 * time.Millisecond)
	t.Cleanup(func() {
		panelHeartbeatUncertainWindow.set(restoreWindow)
		panelHeartbeatRetryPause.set(restorePause)
	})
	return interval, window
}

// Item 3, the rule, measured on the Panel's side with generous bounds: the
// worker is never stopped while renewals are answered, and when they stop
// being answered it is stopped before the Agent's lease could end (scaled:
// window 300 ms, lease 400 ms, from the last renewal the Agent answered).
// The Agent's definite answer stops it at once.
func TestD1HeartbeatStopsTheWorkerBeforeTheLeaseEnds(t *testing.T) {
	requestID, ownerID := strings.Repeat("6", 32), strings.Repeat("7", 32)
	binding := agentMutationBinding{MutationRequestID: requestID, MutationOwnerID: ownerID}
	for _, answered := range []int32{0, 3} {
		t.Run(map[int32]string{0: "no renewal answered", 3: "three renewals answered"}[answered], func(t *testing.T) {
			interval, window := heartbeatAtProductionRatio(t)
			lease := window * 4 / 3
			agent := &blockingRenewalAgent{answered: answered, block: make(chan struct{}), requestID: requestID, ownerID: ownerID}
			t.Cleanup(func() { close(agent.block) })
			panel := newPolicyDispatchTestPanel(t, agent)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			began := time.Now()
			stop := panel.startObservedAgentMutationHeartbeatSince(ctx, cancel, binding, nil, interval, began)
			// The last renewal sent before the stall is sent by the Panel no
			// later than answered intervals after the start.
			select {
			case <-ctx.Done():
			case <-time.After(5 * time.Second):
				t.Fatal("the worker was never stopped")
			}
			stopped := time.Since(began)
			if agent.calls.Load() < answered {
				t.Fatalf("stopped after %v with only %d renewals", stopped, agent.calls.Load())
			}
			// Not stopped while answered: never before the window after the
			// last answered renewal was sent (at the earliest, the start
			// plus answered-1 intervals).
			lastAnsweredSent := time.Duration(0)
			if answered > 0 {
				lastAnsweredSent = time.Duration(answered) * interval
			}
			if stopped < lastAnsweredSent+window-interval {
				t.Fatalf("stopped after %v, too early", stopped)
			}
			// Before the lease could end: the latest possible answered send
			// plus the lease, with the scheduling slack of a loaded runner.
			if stopped >= lastAnsweredSent+interval+lease+time.Second {
				t.Fatalf("stopped after %v, past the lease", stopped)
			}
			if _, err := stop(); !errors.Is(err, errAgentMutationRenewalsUnanswered) {
				t.Fatalf("reason: %v", err)
			}
		})
	}
	t.Run("lease lost", func(t *testing.T) {
		agent := &heartbeatFailureClassificationAgent{job: &ServiceOperationMutationJob{
			RequestID: requestID, OwnerID: ownerID, Status: agentMutationFailed, ErrorCode: agentMutationLeaseExpiredErrorCode,
		}, responseError: "service mutation lease is not running"}
		panel := newPolicyDispatchTestPanel(t, agent)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		began := time.Now()
		stop := panel.startObservedAgentMutationHeartbeatSince(ctx, cancel, binding, nil, 20*time.Millisecond, began)
		select {
		case <-ctx.Done():
		case <-time.After(5 * time.Second):
			t.Fatal("the worker was never stopped")
		}
		if elapsed := time.Since(began); elapsed > 2*time.Second {
			t.Fatalf("a definite answer stopped the worker only after %v", elapsed)
		}
		if _, err := stop(); !errors.Is(err, errAgentMutationLeaseLost) {
			t.Fatalf("reason: %v", err)
		}
	})
}

// lockedLog collects the panel's log lines while a test runs.
type lockedLog struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (l *lockedLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

func (l *lockedLog) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

// Second look, finding 6: a worker stopped by the renewal rule leaves one
// journal line with the time since the last answered renewal and the last
// error, so the lab can see why and when.
func TestD1AHeartbeatStopIsLoggedWithItsCause(t *testing.T) {
	requestID, ownerID := strings.Repeat("6", 32), strings.Repeat("7", 32)
	binding := agentMutationBinding{MutationRequestID: requestID, MutationOwnerID: ownerID}
	interval, _ := heartbeatAtProductionRatio(t)
	lines := &lockedLog{}
	log.SetOutput(lines)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	agent := &heartbeatFailureClassificationAgent{transportErr: errors.New("simulated renewal timeout")}
	panel := newPolicyDispatchTestPanel(t, agent)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stop := panel.startObservedAgentMutationHeartbeatSince(ctx, cancel, binding, nil, interval, time.Now())
	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("the worker was never stopped")
	}
	_, _ = stop()
	text := lines.String()
	// The last renewal error is the Agent's or, for a renewal cut at the
	// window's end, the renewal's own deadline.
	if !strings.Contains(text, "agent mutation "+requestID+": worker stopped: no lease renewal answered for") ||
		!(strings.Contains(text, "last renewal error: simulated renewal timeout") || strings.Contains(text, "last renewal error: context deadline exceeded")) {
		t.Fatalf("log: %q", text)
	}
}

// Second look, finding 1: at the production ratio, one renewal that hangs
// for half the window and then answers does not stop the step.
func TestD1OneSlowRenewalThatAnswersInTheWindowKeepsTheStep(t *testing.T) {
	requestID, ownerID := strings.Repeat("6", 32), strings.Repeat("7", 32)
	binding := agentMutationBinding{MutationRequestID: requestID, MutationOwnerID: ownerID}
	interval, window := heartbeatAtProductionRatio(t)
	agent := &blockingRenewalAgent{answered: -1, slow: 2, slowFor: window / 2, block: make(chan struct{}), requestID: requestID, ownerID: ownerID}
	t.Cleanup(func() { close(agent.block) })
	panel := newPolicyDispatchTestPanel(t, agent)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stop := panel.startObservedAgentMutationHeartbeatSince(ctx, cancel, binding, nil, interval, time.Now())
	select {
	case <-ctx.Done():
		t.Fatalf("the step was stopped by one slow renewal (renewals %d)", agent.calls.Load())
	case <-time.After(4 * window):
	}
	if agent.calls.Load() < 3 {
		t.Fatalf("renewals after the slow one: %d", agent.calls.Load())
	}
	if _, err := stop(); err != nil {
		t.Fatalf("reason: %v", err)
	}
}

// Second look, finding 2: a lease granted slowly (here the grant took 280 ms
// of the 300 ms window) still gets its first renewal at once, and the step
// goes on while it is answered.
func TestD1ASlowLeaseGrantStillGetsItsFirstRenewal(t *testing.T) {
	requestID, ownerID := strings.Repeat("6", 32), strings.Repeat("7", 32)
	binding := agentMutationBinding{MutationRequestID: requestID, MutationOwnerID: ownerID}
	interval, window := heartbeatAtProductionRatio(t)
	agent := &blockingRenewalAgent{answered: -1, block: make(chan struct{}), requestID: requestID, ownerID: ownerID}
	t.Cleanup(func() { close(agent.block) })
	panel := newPolicyDispatchTestPanel(t, agent)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stop := panel.startObservedAgentMutationHeartbeatSince(ctx, cancel, binding, nil, interval, time.Now().Add(-(window - 20*time.Millisecond)))
	select {
	case <-ctx.Done():
		t.Fatalf("the step was stopped before its first renewal (renewals %d)", agent.calls.Load())
	case <-time.After(2 * window):
	}
	if agent.calls.Load() < 1 {
		t.Fatal("no renewal was sent")
	}
	if _, err := stop(); err != nil {
		t.Fatalf("reason: %v", err)
	}
}

// standaloneRenewalAgent is the standalone fixture whose renewals go
// unanswered on demand.
type standaloneRenewalAgent struct {
	*standaloneFailureFinishAgent
	unansweredOne atomic.Int32
	unanswered    atomic.Bool
}

func (a *standaloneRenewalAgent) HeartbeatServiceMutation(req *ServiceOperationMutationHeartbeatRequest, resp *ServiceOperationMutationResponse) error {
	if a.unanswered.Load() {
		return errors.New("simulated renewal without an answer in time")
	}
	if a.unansweredOne.Load() > 0 {
		a.unansweredOne.Add(-1)
		return errors.New("simulated renewal without an answer in time")
	}
	return a.standaloneFailureFinishAgent.HeartbeatServiceMutation(req, resp)
}

// Item 3 for standalone mutations (firewall, DNS, mail TLS, certificates,
// VPN, webmail): one unanswered renewal does not stop the call; renewals
// unanswered for the window do, and the mutation is finished as uncertain.
func TestD1AStandaloneMutationFollowsTheSameRenewalRule(t *testing.T) {
	shortenD1Bounds(t)
	t.Run("one unanswered renewal", func(t *testing.T) {
		agent := &standaloneRenewalAgent{standaloneFailureFinishAgent: &standaloneFailureFinishAgent{}}
		agent.unansweredOne.Store(1)
		panel := newPolicyDispatchTestPanel(t, agent)
		err := panel.withStandaloneAgentMutation(context.Background(), "service_action", "nginx", "", func(ctx context.Context, _ agentMutationBinding) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(200 * time.Millisecond):
				return nil
			}
		})
		if err != nil || agent.unansweredOne.Load() != 0 {
			t.Fatalf("err=%v unanswered left=%d", err, agent.unansweredOne.Load())
		}
		finishes := agent.finishRequests()
		if len(finishes) != 1 || !finishes[0].Success {
			t.Fatalf("finishes: %+v", finishes)
		}
	})
	t.Run("renewals unanswered for the window", func(t *testing.T) {
		agent := &standaloneRenewalAgent{standaloneFailureFinishAgent: &standaloneFailureFinishAgent{}}
		agent.unanswered.Store(true)
		panel := newPolicyDispatchTestPanel(t, agent)
		began := time.Now()
		err := panel.withStandaloneAgentMutation(context.Background(), "service_action", "nginx", "", func(ctx context.Context, _ agentMutationBinding) error {
			<-ctx.Done()
			return ctx.Err()
		})
		if err == nil || time.Since(began) > time.Second {
			t.Fatalf("err=%v after %v", err, time.Since(began))
		}
		finishes := agent.finishRequests()
		if len(finishes) != 1 || finishes[0].Success || finishes[0].FailureCode != errCodeServiceOperationHeartbeatUncertain {
			t.Fatalf("finishes: %+v", finishes)
		}
	})
}
