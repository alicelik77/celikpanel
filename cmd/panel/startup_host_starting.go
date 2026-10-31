package main

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Panel start while the server is still booting (final alpha.113 acceptance
// run 4, D8, 31 Oct 2026).
//
// After every reboot (Debian 13, Ubuntu 24.04, Arch) the Panel reached its
// startup reconciliation while systemd still reported the system as
// "starting". Every host-changing Agent call is refused before dispatch until
// the Agent can read the host identity, and the Agent reads it only once
// systemd is running (or degraded), so the startup steps were refused and
// nothing ran them again in that process:
//
//   - certificate startup reconcile: staged lineages (Agent.ReconcileSiteCertLineages)
//   - certificate startup reconcile: restore hosted vhost batch (Agent.ApplyVhosts)
//   - milter wiring at startup (Agent.WireMailFilters)
//
// and, when there is work for them, the interrupted-operation recovery and
// the VPN provisioning recovery, which mark their subsystem degraded. Sites
// kept serving with their files on disk, but the reconciliation silently did
// not run until the next Panel start.
//
// These steps run before normal management access opens, on purpose: they
// rewrite every hosted site file and remove certificate lineages the ledger
// does not name, which must not overlap an owner's site or certificate
// operation. So rather than repeat them in the background after access has
// opened, Panel start waits here, still gated (the startup page says
// management is still starting), until the host identity can be read, with a
// short bound. The wait changes nothing and starts nothing; it repeats one
// read. Once the identity is read it is kept for the life of the process, so
// every later startup step and operation passes the same check.
//
// Sunucu hâlâ açılırken Panel başlangıcı: başlangıç uzlaştırması, sunucu
// açılışı bitene kadar (sınırlı süre) bekler; bekleme hiçbir şeyi değiştirmez.

const (
	startupHostStartingWaitText = "startup: this server is still starting (its system manager, systemd, " +
		"does not report it ready yet), so the Panel's startup reconciliation (site configuration files, " +
		"certificate lineages, interrupted operations, VPN and mail filters) waits before it changes anything. " +
		"Nothing was changed. The Panel checks again by itself for up to %s and then continues; " +
		"nobody needs to act: %v"
	startupHostStartedText = "startup: this server finished starting after %s; startup reconciliation continues"
	// A Panel stopped while it waits (systemd stops or restarts it) did not
	// wait out the bound: it says so, never the timeout text (run 5, D12).
	startupHostStoppedText = "startup: the Panel was asked to stop after waiting %s for this server to finish starting; " +
		"nothing was changed and the startup reconciliation did not run in this process. " +
		"Nobody needs to act: the next Panel start waits again and then runs it"
	// The owner's way out names the commands that show what keeps the boot
	// from finishing and the restart that runs the reconciliation again.
	startupHostNotStartedText = "startup: this server was still starting after %s (systemd has not reported it " +
		"running), so the Panel continues without waiting longer: its startup reconciliation steps are refused " +
		"before they change anything and run at the next Panel start. Sites keep serving with their current " +
		"configuration. The server administrator: run `systemctl is-system-running` and `systemctl list-jobs` " +
		"to see what keeps the boot from finishing; once it reports running or degraded, run " +
		"`sudo systemctl restart celikpanel-panel`. Last answer: %v"
)

// startupHostStartingBound bounds how long Panel start waits for the boot.
var (
	startupHostStartingBound    = 2 * time.Minute
	startupHostStartingMinPause = time.Second
	startupHostStartingMaxPause = 10 * time.Second
	startupHostStartingAttempt  = 15 * time.Second
)

// startupHostStartingWait is one bounded wait. probe reads the host
// identity (nil when it is known); only a refusal that names systemd still
// starting is waited for, any other answer returns at once and the startup
// steps behave as before.
type startupHostStartingWait struct {
	bound    time.Duration
	minPause time.Duration
	maxPause time.Duration
	attempt  time.Duration
	probe    func(context.Context) error
	sleep    func(context.Context, time.Duration) bool
	now      func() time.Time
	logf     func(string, ...any)
}

// startupHostStillStarting: the identity read was refused because systemd
// still reports the host as starting (or initializing).
func startupHostStillStarting(err error) bool {
	return serverSetupHostStarting(err) && serverSetupHostStartingBySystemd(err)
}

// run reports whether the host identity is known (or the refusal was not a
// boot state); false means the bound passed, or ctx ended, while the host
// was still starting. It writes at most three lines: the wait, then the
// outcome.
func (w startupHostStartingWait) run(ctx context.Context) bool {
	started := w.now()
	pause := w.minPause
	announced := false
	for {
		attemptCtx, cancel := context.WithTimeout(ctx, w.attempt)
		err := w.probe(attemptCtx)
		cancel()
		if ctx.Err() != nil {
			// Stopped (or restarted) while waiting: a stop line, never the
			// "finished starting" or timeout text (run 5, D12).
			if announced {
				w.logf(startupHostStoppedText, w.now().Sub(started).Round(100*time.Millisecond))
			}
			return false
		}
		if !startupHostStillStarting(err) {
			if announced {
				w.logf(startupHostStartedText, w.now().Sub(started).Round(time.Second))
			}
			return true
		}
		if !announced {
			w.logf(startupHostStartingWaitText, w.bound, err)
			announced = true
		}
		if w.now().Sub(started) >= w.bound {
			w.logf(startupHostNotStartedText, w.bound, err)
			return false
		}
		if !w.sleep(ctx, pause) {
			if ctx.Err() != nil {
				w.logf(startupHostStoppedText, w.now().Sub(started).Round(100*time.Millisecond))
			} else {
				w.logf(startupHostNotStartedText, w.bound, err)
			}
			return false
		}
		pause *= 2
		if pause > w.maxPause {
			pause = w.maxPause
		}
	}
}

// startupHostIdentityProbe is the same pre-dispatch identity read every
// host-changing Agent call makes, wrapped the way callAgentContext reports
// it, so the one classifier (server_setup_host_starting.go) decides.
func (p *Panel) startupHostIdentityProbe(ctx context.Context) error {
	if _, err := p.agentRPCHostIdentity(ctx); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return fmt.Errorf("%w: startup: %w", errAgentRPCPlatformIdentityUnavailable, err)
	}
	return nil
}

// waitForHostStartedAtStartup is Panel start's bounded wait (D8).
func (p *Panel) waitForHostStartedAtStartup(ctx context.Context, logf func(string, ...any)) bool {
	return startupHostStartingWait{
		bound:    startupHostStartingBound,
		minPause: startupHostStartingMinPause,
		maxPause: startupHostStartingMaxPause,
		attempt:  startupHostStartingAttempt,
		probe:    p.startupHostIdentityProbe,
		sleep:    waitStartupDeferredRetry,
		now:      time.Now,
		logf:     logf,
	}.run(ctx)
}
