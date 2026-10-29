package main

import (
	"context"
	"strconv"
)

// Another change on this server (Beta-1 third re-run, side effect,
// 2026-10-29). A start while a Components install (redis) ran created a run
// whose first component the Agent refused at admission (HOST_MUTATION_BUSY,
// package manager active): the run failed and needed Edit plan.
//
// Now the start is refused before a run exists (server_setup_host_busy), and
// a run that meets the busy state waits instead of failing: when another
// operation is active nothing is created, and when the Agent refuses a child
// at admission (nothing ran) the step asks again under a new child identity,
// up to serverSetupHostBusyAttempts times, then fails as before.
//
// Sunucuda başka bir değişiklik sürerken kurulum başlamaz; süren kurulum
// adımı bekler.

const (
	serverSetupHostBusyCode         = "server_setup_host_busy"
	serverSetupHostBusyStartMessage = "Another change is running on this server (for example a component installing from Components), so setup did not start and nothing was changed. Nobody needs to fix anything: wait until it finishes (its progress shows on its own page), then start setup."
	serverSetupHostBusyWaitMessage  = "Another change is running on this server (for example a component installing from Components, or the package manager is busy), so this step waits and started nothing. Nobody needs to act: setup continues this step by itself once the other change has finished."
)

// serverSetupHostBusyAttempts bounds the refused child attempts of one step
// (the runner asks again about every 20 s).
const serverSetupHostBusyAttempts = 30

// serverSetupChildRequestID is the request identity of a step's current
// child operation: the step's own one, then one per refused attempt. The
// panel certificate saga keeps its own identity.
func serverSetupChildRequestID(step serverSetupExecutionStep) string {
	if step.ChildAttempt == 0 || step.Kind == "panel_certificate" {
		return step.RequestID
	}
	return serverSetupID(step.RequestID, "child-attempt", strconv.Itoa(step.ChildAttempt))
}

// serverSetupHostBusy: another service operation or Agent mutation is
// active on this server.
func (p *Panel) serverSetupHostBusy(ctx context.Context) (bool, error) {
	active, err := p.activeServiceOperation(ctx)
	if err != nil || active != nil {
		return active != nil, err
	}
	job, err := p.statusAgentMutation(ctx, "")
	if err != nil {
		return false, err
	}
	return job != nil && agentMutationActive(job.Status), nil
}
