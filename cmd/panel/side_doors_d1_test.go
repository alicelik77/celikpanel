package main

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// sideDoorsLostAgent answers the side doors step only when released; its
// lease renewals go unanswered on demand (the Agent contact is lost while
// the step runs).
type sideDoorsLostAgent struct {
	*standaloneRenewalAgent
	release chan struct{}
	steps   atomic.Int32
}

func (a *sideDoorsLostAgent) CloseSideDoorsV1(_ *transport.SideDoorsRequest, resp *transport.SideDoorsResponse) error {
	a.steps.Add(1)
	<-a.release
	*resp = transport.SideDoorsResponse{}
	return nil
}

// Final merge with PR #366: the side doors step and the administrator's
// "close it again" run as a standalone mutation. When the Agent stops
// answering the lease renewals while the step runs, the step's call ends
// within the renewal window and the mutation is finished as uncertain (the
// D1 rule), never left running; the close-again then records "could not
// run" (handleSideDoorsReClose's error path). No Panel lock is held across
// that wait: the Panel's close-again lock is taken only to read or write its
// record, and sideDoorsMu is the Agent's own.
func TestASideDoorsStepThatLosesTheAgentEndsAndIsNotLeftRunning(t *testing.T) {
	shortenD1Bounds(t)
	agent := &sideDoorsLostAgent{
		standaloneRenewalAgent: &standaloneRenewalAgent{standaloneFailureFinishAgent: &standaloneFailureFinishAgent{}},
		release:                make(chan struct{}),
	}
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(agent.release) }) })
	agent.unanswered.Store(true)
	panel := newPolicyDispatchTestPanel(t, agent)
	began := time.Now()
	_, err := closeSideDoorsThroughAgent(panel, context.Background(), transport.SideDoorsReCloseAt, "")
	if err == nil || time.Since(began) > 5*time.Second {
		t.Fatalf("err=%v after %v: the step's call was not ended", err, time.Since(began))
	}
	if agent.steps.Load() != 1 {
		t.Fatalf("the step was asked %d times", agent.steps.Load())
	}
	finishes := agent.finishRequests()
	if len(finishes) != 1 || finishes[0].Success || finishes[0].FailureCode != errCodeServiceOperationHeartbeatUncertain {
		t.Fatalf("finishes: %+v", finishes)
	}
}
