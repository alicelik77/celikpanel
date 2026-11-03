package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"
)

// A setup DNS install that the Agent never admitted (alpha.118 lab, D1).
//
// The setup DNS step records its switch (phase "activating", attached to the
// DNS engine state) before it asks the Agent for the host lease. When the
// Agent refused that lease (for example the package manager or the host lock
// was busy on a freshly booted server), nothing was written to the Agent's
// mutation ledger, so no rollback evidence could ever exist and setup showed
// server_setup_dns_result_unknown forever, with the switch left "activating".
//
// Now the Panel proves "never admitted" from the Agent's own ledger and the
// native state: no job for the exact request, no Agent hold, no retained DNS
// switch journal, no DNS engine running, observed twice around the runtime
// read, while this exact first-install switch is still the attached one. With
// that proof the step waits and the next pass admits the same request under
// the same identities (the Agent admits one lease per request and one lease
// per host, so this cannot start a second mutation). Past
// serverSetupHostBusyBound the reserved switch is released as a verified
// "nothing changed" and the step fails with the owner's next action. Without
// the proof the earlier unknown/reconcile path is kept unchanged.
//
// Agent'ın hiç kabul etmediği kurulum DNS işlemi: kanıtlanırsa adım bekler ve
// aynı istek yeniden denenir; süre aşılınca ayrılan işlem bırakılır ve adım,
// hiçbir şeyin değişmediğini söyleyen doğrulanmış bir hatayla durur.

const (
	serverSetupDNSNotAdmittedCode       = "server_setup_dns_not_admitted"
	serverSetupDNSNotAdmittedFailedCode = "server_setup_dns_not_admitted_failed"
	serverSetupDNSNotAdmittedMessage    = "The CelikPanel Agent did not accept the DNS installation yet, so nothing was started or changed on this server. Setup asks the Agent again by itself with the same operation; nobody needs to act now. If this message stays for several minutes, the server administrator can check the Agent on this server with `sudo systemctl status celikpanel-agent` and `sudo journalctl -u celikpanel-agent -n 50`."
	serverSetupDNSNotAdmittedFailedText = "For more than 30 minutes the CelikPanel Agent did not accept the DNS installation, so this step stopped. Nothing was installed or changed, and CelikPanel released the DNS operation it had reserved. You, the server administrator: check the Agent on this server with `sudo systemctl status celikpanel-agent` and `sudo journalctl -u celikpanel-agent -n 50` and fix what it reports; then choose Review a revised plan."
)

// serverSetupDNSNotAdmittedError: the Agent refused the exact setup DNS
// request before its lease began, and the Panel proved that nothing began.
type serverSetupDNSNotAdmittedError struct {
	RequestID string
	cause     error
}

func (e *serverSetupDNSNotAdmittedError) Error() string {
	return "the Agent did not admit DNS setup request " + e.RequestID + ": " + e.cause.Error()
}

func (e *serverSetupDNSNotAdmittedError) Unwrap() error { return e.cause }

// busy: the refusal was the Agent's typed host-busy answer.
func (e *serverSetupDNSNotAdmittedError) busy() bool { return hostMutationRefusedAsBusy(e.cause) }

// setupDNSSwitchNeverAdmitted proves, read only, that the attached
// first-install switch of this exact setup request never began on the host.
// A false result without error means "not proven"; callers keep the existing
// reconciliation. It never starts, finishes or cancels anything.
func (p *Panel) setupDNSSwitchNeverAdmitted(ctx context.Context, persisted persistedDNSEngineSwitch) (bool, error) {
	db := p.db.GetDB()
	state, err := readDNSEngineDBState(ctx, db)
	if err != nil {
		return false, err
	}
	current, err := readDNSEngineSwitchByRequest(ctx, db, persisted.RequestID)
	if err != nil {
		return false, err
	}
	if err := attachDNSEngineOperationAction(ctx, db, &current); err != nil {
		return false, err
	}
	if current.SwitchID != persisted.SwitchID || current.OwnerID != persisted.OwnerID ||
		current.Qualifier != persisted.Qualifier || current.Phase != "activating" ||
		state.CurrentSwitchID != current.SwitchID ||
		!exactSourceEmptyDNSEngineSwitchAttachedState(state, current) ||
		validateInitialDNSEngineInstallReconcileScope(current) != nil {
		return false, nil
	}
	marker, err := readDNSEngineOperationMarker(ctx, db)
	if err != nil {
		return false, err
	}
	if marker == nil || marker.Phase != dnsEngineOperationAccepted ||
		marker.SwitchID != current.SwitchID || marker.RequestID != current.RequestID {
		return false, nil
	}
	absent := func() (bool, error) {
		job, hold, err := p.statusAgentMutationWithHold(ctx, current.RequestID)
		if err != nil {
			return false, err
		}
		return job == nil && hold == "", nil
	}
	if ok, err := absent(); err != nil || !ok {
		return false, err
	}
	readiness, err := p.readDNSBackendReadinessResponse(ctx)
	if err != nil {
		return false, err
	}
	if retainedDNSSwitchJournal(readiness) != nil {
		return false, nil
	}
	runtimes, _, hold, err := validateDNSBackendReadiness(readiness)
	if err != nil {
		return false, err
	}
	if hold != "" {
		return false, nil
	}
	for _, runtime := range runtimes {
		if runtime.Running {
			return false, nil
		}
	}
	return absent()
}

// serverSetupDNSNotAdmitted classifies a failed switch call. It returns a
// *serverSetupDNSNotAdmittedError only when the Agent provably never began
// the exact request; otherwise nil, and the caller keeps its existing path.
// The caller holds serviceMutationMu, dnsTopologyMu and dnsPublicationMu.
func (p *Panel) serverSetupDNSNotAdmitted(ctx context.Context, persisted persistedDNSEngineSwitch, cause error) error {
	if cause == nil {
		return nil
	}
	var appliedFollowup *dnsEngineMutationAppliedFollowupError
	if errors.As(cause, &appliedFollowup) {
		return nil
	}
	proven, err := p.setupDNSSwitchNeverAdmitted(ctx, persisted)
	if err != nil || !proven {
		return nil
	}
	log.Printf("server setup DNS request %s was not admitted by the Agent; nothing began and the same request is asked again: %v", persisted.RequestID, cause)
	return &serverSetupDNSNotAdmittedError{RequestID: persisted.RequestID, cause: cause}
}

// releaseNeverAdmittedSetupDNS detaches the exact never-admitted setup switch
// as a verified unchanged outcome, so a revised plan can install DNS. It
// proves "never admitted" again under the DNS locks first.
func (p *Panel) releaseNeverAdmittedSetupDNS(ctx context.Context, requestID string) error {
	p.serviceMutationMu.Lock()
	defer p.serviceMutationMu.Unlock()
	p.dnsTopologyMu.Lock()
	defer p.dnsTopologyMu.Unlock()
	dnsPublicationMu.Lock()
	defer dnsPublicationMu.Unlock()

	db := p.db.GetDB()
	persisted, err := readDNSEngineSwitchByRequest(ctx, db, requestID)
	if err != nil {
		return err
	}
	if err := attachDNSEngineOperationAction(ctx, db, &persisted); err != nil {
		return err
	}
	released, err := p.releaseNeverAdmittedDNSEngineSwitchLocked(ctx, persisted)
	if err != nil {
		return err
	}
	if !released {
		return errors.New("the DNS setup operation is not proven unstarted")
	}
	return nil
}

// releaseNeverAdmittedDNSEngineSwitchLocked records a proven never-admitted
// first-install switch as ended without change (phase rolled_back, detached,
// marker cleared, audit change_not_committed). It reports false, with no
// change, when the proof does not hold. Used by setup past its bound and by
// the agent-evidence reconcile (the owner's DNS reconcile and setup's own).
// The caller holds serviceMutationMu, dnsTopologyMu and dnsPublicationMu.
func (p *Panel) releaseNeverAdmittedDNSEngineSwitchLocked(ctx context.Context, persisted persistedDNSEngineSwitch) (bool, error) {
	proven, err := p.setupDNSSwitchNeverAdmitted(ctx, persisted)
	if err != nil || !proven {
		return false, err
	}
	current, err := readDNSEngineSwitchByRequest(ctx, p.db.GetDB(), persisted.RequestID)
	if err != nil {
		return false, err
	}
	if err := attachDNSEngineOperationAction(ctx, p.db.GetDB(), &current); err != nil {
		return false, err
	}
	manifest, err := p.reconstructPersistedDNSEngineManifest(ctx, current)
	if err != nil {
		return false, fmt.Errorf("verify persisted DNS engine manifest: %w", err)
	}
	if err := p.rollbackVerifiedSourceEmptyDNSEngineSwitch(ctx, current, manifest); err != nil {
		return false, err
	}
	auditCtx, cancel := context.WithTimeout(context.Background(), dnsEnginePostCommitAuditTimeout)
	defer cancel()
	// The existing outcome says exactly this: ended without changing anything.
	p.auditDNSEngineSystem(auditCtx, "change_not_committed", current)
	log.Printf("DNS engine switch %s (request %s) was never admitted by the Agent; released unchanged", current.SwitchID, current.RequestID)
	return true, nil
}

// serverSetupDNSNotAdmittedAttempt records one proven refusal on the DNS
// step: a bounded wait, or past the bound the released reservation and a
// verified failure naming the owner's next action.
func (p *Panel) serverSetupDNSNotAdmittedAttempt(ctx context.Context, step *serverSetupExecutionStep, notAdmitted *serverSetupDNSNotAdmittedError, now time.Time) error {
	if recordServerSetupHostBusy(step, now) {
		if notAdmitted.busy() {
			return &serverSetupPeerWait{Code: serverSetupHostBusyCode, Message: serverSetupHostBusyWaitMessage}
		}
		return &serverSetupPeerWait{Code: serverSetupDNSNotAdmittedCode, Message: serverSetupDNSNotAdmittedMessage}
	}
	if err := p.releaseNeverAdmittedSetupDNS(ctx, step.RequestID); err != nil {
		// Not provable now: keep checking the same operation, never fail it
		// while its reservation is still attached.
		return fmt.Errorf("%w: %v", errServerSetupDNSReconciliationRequired, errors.Join(notAdmitted, err))
	}
	if notAdmitted.busy() {
		var failure *serverSetupChildFailure
		if !errors.As(serverSetupHostStayedBusy(step, hostMutationBusyReason(notAdmitted.cause),
			"the DNS installation was not started and nothing was changed (CelikPanel released the DNS operation it had reserved)"), &failure) {
			return errors.New("setup host busy failure is untyped")
		}
		return &serverSetupDNSNotAdmittedReleased{failure: failure}
	}
	return &serverSetupDNSNotAdmittedReleased{failure: &serverSetupChildFailure{Code: serverSetupDNSNotAdmittedFailedCode, Message: serverSetupDNSNotAdmittedFailedText}}
}

// serverSetupDNSNotAdmittedReleased: the never-admitted reservation was
// released, so the step's failure is verified (nothing changed). The runner
// records it at once; an unrelated active operation must not turn it back
// into "reconciling" (the switch it would wait for no longer exists).
type serverSetupDNSNotAdmittedReleased struct{ failure *serverSetupChildFailure }

func (e *serverSetupDNSNotAdmittedReleased) Error() string { return e.failure.Error() }

func (e *serverSetupDNSNotAdmittedReleased) Unwrap() error { return e.failure }
