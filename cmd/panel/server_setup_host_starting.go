package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// The server is still starting (Beta-1 third re-run N1, 2026-10-29).
//
// After a reboot the setup runner resumed the DNS step before the host had
// finished booting. The Panel could not read the Agent's host identity
// ("detect host platform: systemd is not ready: systemd state "starting"")
// and the step failed with the generic server_setup_dns_failed; only Edit
// plan and a new start recovered it.
//
// One classifier now names that state for every step: an Agent call refused
// before dispatch because the host identity is unavailable while systemd is
// still starting (or the Agent itself does not answer yet). Nothing was sent
// to the Agent, so the runner waits (server_setup_host_starting) and asks the
// same step again; no new mutation is made. Only when the host stays in that
// state past serverSetupHostStartingBound does the step fail, with a reason
// that names systemd.
//
// Sunucu hâlâ açılıyor: adım bekler ve aynı adımı yeniden dener.

const (
	serverSetupHostStartingCode      = "server_setup_host_starting"
	serverSetupHostNotStartedCode    = "server_setup_host_not_started"
	serverSetupAgentNotAnsweringCode = "server_setup_agent_not_answering"
	// True for a step that has not begun and for a resumed one (review fix,
	// item 7): nothing new was started either way.
	serverSetupHostStartingMessage   = "This server is still starting (its system manager, systemd, does not report it ready yet, or the CelikPanel Agent is not answering yet), so CelikPanel cannot reach the host. Nothing new was started; once the server has started, setup checks this step's saved operation again and continues it. Nobody needs to act."
	serverSetupHostNotStartedText    = "This server has still not finished starting: its system manager (systemd) has reported it as starting for more than 10 minutes, so CelikPanel cannot verify the host and did not run this step. You, the owner: run `systemctl is-system-running` and `systemctl --failed` on this server and fix what keeps the boot from finishing; then choose Review a revised plan."
	serverSetupAgentNotAnsweringText = "The CelikPanel Agent on this server has not answered for more than 10 minutes, so CelikPanel cannot reach the host and did not run this step. You, the owner: run `systemctl status celikpanel-agent` on this server and, if it is stopped, `sudo systemctl start celikpanel-agent`; then choose Review a revised plan."
)

// serverSetupHostStartingBound bounds the wait for a booting host.
var serverSetupHostStartingBound = 10 * time.Minute

var (
	errServerSetupHostNotStarted    = errors.New("the host did not finish starting")
	errServerSetupAgentNotAnswering = errors.New("the Agent did not answer")
)

// serverSetupHostStartingBySystemd: the refusal names systemd still
// starting (otherwise the Agent itself did not answer).
func serverSetupHostStartingBySystemd(err error) bool {
	return err != nil && strings.Contains(err.Error(), "systemd is not ready")
}

// serverSetupHostStarting: an Agent call was refused before dispatch because
// the host is still starting. Only the platform-identity refusal counts (the
// identity is read before any host mutation is sent), and only with the boot
// states of systemd or an Agent that does not answer yet.
func serverSetupHostStarting(err error) bool {
	if err == nil || !errors.Is(err, errAgentRPCPlatformIdentityUnavailable) {
		return false
	}
	message := err.Error()
	if strings.Contains(message, "systemd is not ready") {
		return strings.Contains(message, `"starting"`) || strings.Contains(message, `"initializing"`) ||
			strings.Contains(message, "state starting") || strings.Contains(message, "state initializing")
	}
	return strings.Contains(message, "connection refused") || strings.Contains(message, "no such file or directory") ||
		strings.Contains(message, "connection reset") || strings.Contains(message, "agent RPC client is unavailable")
}

// serverSetupHostStartingWait records the wait, or turns it into the
// verified failure once the bound has passed. It returns nil when the step
// is to fail with err (wrapped).
func serverSetupHostStartingWait(execution *serverSetupExecution, now time.Time, cause error) (bool, error) {
	// A new episode starts whenever the run's last state was not this wait
	// (review fix, item 2): an earlier boot never shortens a later one.
	if execution.Error == nil || execution.Error.Code != serverSetupHostStartingCode {
		execution.HostStartingSince = ""
	}
	since, err := time.Parse(time.RFC3339Nano, execution.HostStartingSince)
	if err != nil || execution.HostStartingSince == "" {
		since = now
		execution.HostStartingSince = now.UTC().Format(time.RFC3339Nano)
	}
	if now.Sub(since) < serverSetupHostStartingBound {
		execution.Status = "waiting"
		execution.Error = &serviceOperationError{Code: serverSetupHostStartingCode, Message: serverSetupHostStartingMessage}
		return true, nil
	}
	if serverSetupHostStartingBySystemd(cause) {
		return false, fmt.Errorf("%w: %v", errServerSetupHostNotStarted, cause)
	}
	return false, fmt.Errorf("%w: %v", errServerSetupAgentNotAnswering, cause)
}

// requireServerSetupHostIdentity reads the Agent's host identity before a
// setup child operation is created, so a booting host is a wait before the
// operation exists rather than a failed operation.
func (p *Panel) requireServerSetupHostIdentity(ctx context.Context) error {
	if _, err := p.agentRPCHostIdentity(ctx); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return fmt.Errorf("%w: setup child: %w", errAgentRPCPlatformIdentityUnavailable, err)
	}
	return nil
}
