package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// The VM's own error after a reboot (third Beta-1 re-run N1).
func bootingHostErrorForTest() error {
	agentErr := errors.New(`read HostPlatform: detect host platform: systemd is not ready: systemd state "starting" exited with status 1`)
	return fmt.Errorf("authorize DNS engine host mutation before snapshot persistence: %w",
		fmt.Errorf("%w: %s: %w", errAgentRPCPlatformIdentityUnavailable, "Agent.SwitchDNSEngineV1", agentErr))
}

func TestServerSetupHostStartingClassifierNamesOnlyABootingHost(t *testing.T) {
	if !serverSetupHostStarting(bootingHostErrorForTest()) {
		t.Fatal("the VM's booting-host error was not classified")
	}
	for _, err := range []error{
		nil,
		errors.New(`systemd is not ready: systemd state "starting"`), // not a pre-dispatch identity refusal
		fmt.Errorf("%w: %s: %w", errAgentRPCPlatformIdentityUnavailable, "Agent.X", errors.New(`systemd is not ready: systemd state "degraded"`)),
		fmt.Errorf("%w: package_family=dnf", errAgentRPCPlatformCapabilityDenied),
	} {
		if serverSetupHostStarting(err) {
			t.Fatalf("classified as booting: %v", err)
		}
	}
	if !serverSetupHostStarting(fmt.Errorf("%w: setup child: %w", errAgentRPCPlatformIdentityUnavailable, errAgentRPCClientUnavailable)) {
		t.Fatal("an Agent that does not answer yet was not classified")
	}
}

// N1: a step resumed while the host is still starting waits with its own
// reason, is asked again without a new mutation, and goes on once the host
// answers; a host that stays starting past the bound fails naming systemd.
func TestServerSetupStepWaitsWhileTheHostIsStarting(t *testing.T) {
	f, state := setupOperationFixture(t)
	plan := saveSetupPlanForTest(t, f, state)
	serverSetupRunners.Store(f.panel, true)
	t.Cleanup(func() { serverSetupRunners.Delete(f.panel) })
	response := postSetupStartForTest(t, f, plan.ID, strings.Repeat("9", 32))
	var execution serverSetupExecution
	if response.Code != 202 || json.Unmarshal(response.Body.Bytes(), &execution) != nil {
		t.Fatalf("start %d %s", response.Code, response.Body.String())
	}
	booting, calls := true, 0
	f.panel.serverSetupStepOverride = func(_ context.Context, step serverSetupExecutionStep) (bool, error, bool) {
		if step.ID != execution.Steps[0].ID {
			return false, nil, false
		}
		calls++
		if booting {
			return true, bootingHostErrorForTest(), true
		}
		return true, nil, true
	}
	for pass := 0; pass < 2; pass++ {
		if _, err := f.panel.advanceServerSetupExecution(plan, &execution); err != nil {
			t.Fatal(err)
		}
		if execution.Status != "waiting" || execution.Steps[0].Status != "running" || execution.Error == nil || execution.Error.Code != serverSetupHostStartingCode {
			t.Fatalf("pass %d: a booting host failed the step: %s %+v", pass, execution.Status, execution.Error)
		}
	}
	booting = false
	if _, err := f.panel.advanceServerSetupExecution(plan, &execution); err != nil {
		t.Fatal(err)
	}
	if execution.Steps[0].Status != "succeeded" || execution.HostStartingSince != "" || calls != 3 {
		t.Fatalf("the step did not go on: %+v calls=%d", execution, calls)
	}
	// Past the bound: a verified failure that names systemd.
	booting = true
	f.panel.serverSetupStepOverride = func(_ context.Context, step serverSetupExecutionStep) (bool, error, bool) {
		return true, bootingHostErrorForTest(), true
	}
	execution.HostStartingSince = time.Now().Add(-serverSetupHostStartingBound - time.Minute).UTC().Format(time.RFC3339Nano)
	if _, err := f.panel.advanceServerSetupExecution(plan, &execution); err != nil {
		t.Fatal(err)
	}
	if execution.Status != "failed" || execution.Error == nil || execution.Error.Code != serverSetupHostNotStartedCode || !strings.Contains(execution.Error.Message, "systemctl is-system-running") {
		t.Fatalf("past the bound: %s %+v", execution.Status, execution.Error)
	}
}
