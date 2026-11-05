package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// The security emergency operation (D-025, D-043). Its checkpoints are
// planned when the step is accepted; each one is a row in
// security_emergency_effects: pending -> captured (before-image stored) ->
// done, or kept (the owner changed it; left, D-022), failed or manual. A crash
// leaves the operation 'running' and the worker continues it at start from
// the first unfinished checkpoint; every Agent call is compare-and-set, so a
// repeated step changes nothing. Polling only reads.
//
// Kontrol noktalı işlem: çökme sonrası aynı işlem kaldığı yerden sürer.

const emergencyAutoAttempts = 3

// emergencyNative is what an operation does on the server. Production uses
// the Agent (panelEmergencyNative); tests replace it.
type emergencyNative interface {
	renderSite(ctx context.Context, domainID int) error
	inspectUnits(ctx context.Context, service, version string) (transport.EmergencyUnitsInspectResponse, error)
	setUnitLock(ctx context.Context, req transport.SetEmergencyUnitLockRequest) (transport.SetEmergencyUnitLockResponse, error)
	inspectApp(ctx context.Context, siteID int) (transport.AppUnitState, error)
	setAppUnit(ctx context.Context, req transport.SetAppUnitSuspensionRequest) (transport.SetAppUnitSuspensionResponse, error)
	// movePHP moves one site's PHP; sourceGone: the source version was
	// removed, so the site's pool is written anew on the target.
	movePHP(ctx context.Context, domainID int, from, to string, sourceGone bool) error
	// moveNode moves one site's Node.js application and proves it answers
	// (a note for the detail); on failure the application stays stopped on
	// the maintenance page and never runs on the old version.
	moveNode(ctx context.Context, domainID int, from, to string) (string, error)
	removePackage(ctx context.Context, service, version string) (string, error)
	// setAppLock writes or removes a Node.js application's start lock.
	setAppLock(ctx context.Context, req transport.SetEmergencyAppLockRequest) (transport.SetEmergencyAppLockResponse, error)
}

type panelEmergencyNative struct{ p *Panel }

func (p *Panel) emergencyNativeOps() emergencyNative {
	if p.emergencyNative != nil {
		return p.emergencyNative
	}
	return panelEmergencyNative{p: p}
}

func (n panelEmergencyNative) renderSite(ctx context.Context, domainID int) error {
	return n.p.applyVhostForDomain(ctx, domainID)
}

func (n panelEmergencyNative) inspectUnits(ctx context.Context, service, version string) (transport.EmergencyUnitsInspectResponse, error) {
	var resp transport.EmergencyUnitsInspectResponse
	err := n.p.callAgentContext(ctx, "Agent.EmergencyUnitsInspectV1", &transport.EmergencyUnitsInspectRequest{Service: service, Version: version}, &resp)
	return resp, err
}

func (n panelEmergencyNative) setUnitLock(ctx context.Context, req transport.SetEmergencyUnitLockRequest) (transport.SetEmergencyUnitLockResponse, error) {
	req.ExpectedBuildCommit = strings.TrimSpace(buildCommit)
	var resp transport.SetEmergencyUnitLockResponse
	err := n.p.callAgentContext(ctx, "Agent.SetEmergencyUnitLockV1", &req, &resp)
	return resp, err
}

func (n panelEmergencyNative) inspectApp(ctx context.Context, siteID int) (transport.AppUnitState, error) {
	var resp transport.SuspensionInspectResponse
	if err := n.p.callAgentContext(ctx, "Agent.SuspensionInspectV1", &transport.SuspensionInspectRequest{AppSites: []int{siteID}}, &resp); err != nil {
		return transport.AppUnitState{}, err
	}
	if resp.Error != "" || len(resp.AppUnits) != 1 {
		return transport.AppUnitState{}, fmt.Errorf("inspection: %s", resp.Error)
	}
	if resp.AppUnits[0].Error != "" {
		return transport.AppUnitState{}, errors.New(resp.AppUnits[0].Error)
	}
	return resp.AppUnits[0].State, nil
}

func (n panelEmergencyNative) setAppUnit(ctx context.Context, req transport.SetAppUnitSuspensionRequest) (transport.SetAppUnitSuspensionResponse, error) {
	return panelSuspensionNative{p: n.p}.setAppUnit(ctx, req)
}

func (n panelEmergencyNative) movePHP(ctx context.Context, domainID int, from, to string, sourceGone bool) error {
	return n.p.moveSitePHPVersion(ctx, domainID, from, to, sourceGone)
}

func (n panelEmergencyNative) setAppLock(ctx context.Context, req transport.SetEmergencyAppLockRequest) (transport.SetEmergencyAppLockResponse, error) {
	req.ExpectedBuildCommit = strings.TrimSpace(buildCommit)
	var resp transport.SetEmergencyAppLockResponse
	err := n.p.callAgentContext(ctx, "Agent.SetEmergencyAppLockV1", &req, &resp)
	return resp, err
}

func (n panelEmergencyNative) moveNode(ctx context.Context, domainID int, from, to string) (string, error) {
	p := n.p
	state, err := p.loadHostingRuntimeState(ctx, domainID)
	if err != nil {
		return "", err
	}
	if state.ProjectType != "node" || !state.AppPort.Valid {
		return "", verifiedFailure("the site is not a Node.js application any more")
	}
	moveCtx, cancel := hostingDurableContext(ctx)
	defer cancel()
	return p.nodeEmergencyMoveSteps(n, state, from, to).run(moveCtx, from, to)
}

// nodeEmergencyMove moves a Node.js site off a withdrawn version (D-043,
// release decision c). The start lock on the withdrawn version stays in
// place during the move, so neither the move nor its undo can start the
// application there. A failed move stops the application and puts the site
// back on its old record without starting it: it stays on the maintenance
// page and is listed as not moved. The lock goes only after a proven move.
type nodeEmergencyMove struct {
	// lock writes (true) or removes (false) the start lock; a lock changed
	// on the server is reported as kept, never overwritten.
	lock func(ctx context.Context, lock bool) (kept string, err error)
	// move runs the application on the new version and proves it answers.
	move func(ctx context.Context) error
	// stop stops the application and keeps it from starting at boot.
	stop func(ctx context.Context) error
	// putBack writes the old record and unit again, not started, and
	// renders the maintenance page.
	putBack func(ctx context.Context) error
}

// run returns a note for the move's detail, or why the site was not moved.
func (m nodeEmergencyMove) run(ctx context.Context, from, to string) (string, error) {
	kept, err := m.lock(ctx, true)
	if err != nil {
		return "", err
	}
	if kept != "" {
		return "", &suspensionStepError{kind: "prerequisite", err: fmt.Errorf(
			"%s; without CelikPanel's lock the application could start on Node.js %s while it moves. The administrator reviews or deletes that file on the server, then moves the site again", kept, from)}
	}
	failure := m.move(ctx)
	if failure == nil {
		kept, err := m.lock(ctx, false)
		switch {
		case err != nil:
			return "the start lock for Node.js " + from + " is still on the server (" + err.Error() + "); it only refuses " + from + ", so the application runs", nil
		case kept != "":
			return kept, nil
		}
		return "", nil
	}
	stopErr := m.stop(ctx)
	backErr := m.putBack(ctx)
	if stopErr != nil || backErr != nil {
		return "", unknownResult(fmt.Errorf("the application did not run on Node.js %s (%v); stopping it and putting the site back on the maintenance page did not complete: %v",
			to, failure, errors.Join(stopErr, backErr)))
	}
	return "", verifiedFailure("the application did not run on Node.js %s (%v); it is stopped and the site stays on the maintenance page on %s, which never started", to, failure, from)
}

// nodeEmergencyMoveSteps are the production steps of one site's move.
func (p *Panel) nodeEmergencyMoveSteps(native emergencyNative, state hostingRuntimeState, from, to string) nodeEmergencyMove {
	return nodeEmergencyMove{
		lock: func(ctx context.Context, lock bool) (string, error) {
			return emergencyAppLockStep(ctx, native, state.SiteID, from, lock)
		},
		move: func(ctx context.Context) error {
			if err := transitionHostingForMove(p, ctx, state.DomainID, hostingSettingsOf(state, to)); err != nil {
				return err
			}
			return proveAppAnswers(ctx, int(state.AppPort.Int64))
		},
		stop: func(ctx context.Context) error {
			resp, err := native.setAppUnit(ctx, transport.SetAppUnitSuspensionRequest{SiteID: state.SiteID, Suspend: true})
			if err != nil {
				return err
			}
			if !resp.Applied && resp.Error != "" && !absentError(resp.Error) {
				return errors.New(resp.Error)
			}
			return nil
		},
		putBack: func(ctx context.Context) error {
			return p.putNodeSiteBackStopped(ctx, state)
		},
	}
}

// emergencyAppLockStep writes or removes one application's start lock. kept
// names a lock file changed on the server (left as it is, D-022).
func emergencyAppLockStep(ctx context.Context, native emergencyNative, siteID int, version string, lock bool) (string, error) {
	resp, err := native.setAppLock(ctx, transport.SetEmergencyAppLockRequest{SiteID: siteID, NodeVersion: version, Lock: lock})
	if err != nil {
		return "", unknownResult(err)
	}
	switch {
	case resp.Conflict:
		return "the start lock " + resp.Path + " was changed on the server and kept as it is", nil
	case resp.Applied:
		return "", nil
	case resp.Error != "":
		return "", verifiedFailure("%s", resp.Error)
	}
	return "", unknownResult(errors.New("the Agent confirmed nothing about the start lock"))
}

// putNodeSiteBackStopped puts a Node.js site back on the record it had
// before a failed move: the record, then the unit, written but neither
// enabled nor started, then the page (the maintenance page while the
// version is under an emergency).
func (p *Panel) putNodeSiteBackStopped(ctx context.Context, original hostingRuntimeState) error {
	unlock, err := lockDomainHostingOperation(ctx, original.DomainID)
	if err != nil {
		return err
	}
	defer unlock()
	current, err := p.loadHostingRuntimeState(ctx, original.DomainID)
	if err != nil {
		return err
	}
	if current.ProjectType != "node" {
		return fmt.Errorf("the site is a %s site now, so it was not put back", current.ProjectType)
	}
	if current.RuntimeVersion != original.RuntimeVersion {
		if err := p.compareAndSwapHostingState(ctx, current, original); err != nil {
			return fmt.Errorf("put the record back: %w", err)
		}
	}
	request := hostingAppRequest(original)
	request.Trigger = transport.SiteFileTriggerChange
	request.RecordedSHA256 = p.recordedSiteFileSHA256(ctx, original.SiteID, transport.SiteFileKindAppUnit)
	request.KeepStopped = true
	site := siteFileSite{SiteID: original.SiteID, DomainID: original.DomainID, Domain: original.DomainName, ProjectType: original.ProjectType}
	if _, err := p.applyAppUnit(ctx, site, request); err != nil {
		return fmt.Errorf("write the unit back, stopped: %w", err)
	}
	if err := p.applyVhostForDomain(ctx, original.DomainID); err != nil {
		return fmt.Errorf("render the maintenance page: %w", err)
	}
	return nil
}

// moveSitePHPVersion is the site PHP change of the PHP settings page
// (domain_php_handlers.go) for one site: the pool moves (D-031 rules), the
// record follows, the vhost is rendered.
func (p *Panel) moveSitePHPVersion(ctx context.Context, domainID int, from, to string, sourceGone bool) error {
	var siteID int
	var socket sql.NullString
	if err := p.db.GetDB().QueryRowContext(ctx, `SELECT id, php_fpm_socket FROM sites WHERE domain_id = ?`, domainID).Scan(&siteID, &socket); err != nil {
		return err
	}
	poolName := fmt.Sprintf("site%d", siteID)
	if socket.Valid && socket.String != "" {
		parts := strings.Split(socket.String, "-")
		if len(parts) >= 2 {
			poolName = strings.TrimSuffix(parts[len(parts)-1], ".sock")
		}
	}
	site, err := p.loadSiteFileSite(ctx, domainID)
	if err != nil {
		return err
	}
	var request transport.SitePoolRequest
	if sourceGone {
		site.PHPVersion = to
		request = p.sitePoolRequest(ctx, site, transport.SitePoolOperationCreate)
	} else {
		site.PHPVersion = from
		request = p.sitePoolRequest(ctx, site, transport.SitePoolOperationSwitch)
		request.NewPHPVersion = to
	}
	if _, err := p.applySitePool(ctx, site, request); err != nil {
		var held *siteFileHeldError
		if errors.As(err, &held) {
			return &suspensionStepError{kind: "prerequisite", err: fmt.Errorf("the site's PHP pool file was changed on the server, so CelikPanel kept it and did not move the site; the administrator reviews it on the domain's Site configuration screen, then moves the site again")}
		}
		return unknownResult(err)
	}
	if _, err := p.db.GetDB().ExecContext(ctx, `UPDATE sites SET php_version = ?, php_fpm_socket = ?, updated_at = datetime('now') WHERE domain_id = ?`,
		to, services.PHPFPMSocketPath(to, poolName), domainID); err != nil {
		return err
	}
	return nil
}

func (n panelEmergencyNative) removePackage(ctx context.Context, service, version string) (string, error) {
	p := n.p
	if !p.serviceMutationMu.TryLock() {
		return "", unknownResult(errors.New("another component operation is running; tried again shortly"))
	}
	defer p.serviceMutationMu.Unlock()
	if service == "node" {
		var resp transport.NodeRemoveResponse
		err := p.withStandaloneAgentMutation(ctx, "runtime_remove", "node:"+version, "", func(callCtx context.Context, binding agentMutationBinding) error {
			if err := p.callAgentContext(callCtx, "Agent.RemoveNodeVersion", &transport.NodeRemoveRequest{
				ServiceMutationBinding: transport.ServiceMutationBinding{MutationRequestID: binding.MutationRequestID, MutationOwnerID: binding.MutationOwnerID},
				Version:                version,
			}, &resp); err != nil {
				return err
			}
			if resp.Error != "" {
				return errors.New(resp.Error)
			}
			return nil
		})
		// alpha.123 (D-045): the Agent's wall holds here too — a service
		// app still running on this version is a prerequisite the
		// administrator resolves (remove or stop the app), then continues.
		if resp.ErrorCode == "RUNTIME_UNITS_BOUND" {
			return "", &suspensionStepError{kind: "prerequisite", err: fmt.Errorf("%s", resp.Error)}
		}
		if resp.Error != "" {
			return "", verifiedFailure("%s", resp.Error)
		}
		if err != nil {
			return "", unknownResult(err)
		}
		if _, err := p.scanManagedServices(ctx); err != nil {
			log.Printf("[emergency] rescan after Node.js removal: %v", err)
		}
		return "Node.js " + version + " removed; the sites' files are kept", nil
	}
	pkg := ""
	if service == "php-fpm" {
		pkg = "php" + version + "-fpm"
	}
	var resp transport.UninstallServiceResponse
	err := p.withStandaloneAgentMutation(ctx, "service_uninstall", service, pkg, func(callCtx context.Context, binding agentMutationBinding) error {
		request := transport.InstallServiceRequest{ServiceMutationBinding: binding, ID: service, Package: pkg}
		if err := p.callAgentContext(callCtx, "Agent.UninstallService", &request, &resp); err != nil {
			return err
		}
		if resp.Error != "" && !resp.MutationApplied {
			return errors.New(resp.Error)
		}
		return nil
	})
	if resp.Error != "" && !resp.MutationApplied {
		if resp.ErrorCode == errCodeRemovalToolsBound {
			return "", &suspensionStepError{kind: "prerequisite", err: fmt.Errorf("%s; move the PHP tools to another version in the PHP-FPM drawer, then continue this removal", resp.Error)}
		}
		return "", verifiedFailure("%s", resp.Error)
	}
	if err != nil && resp.Error == "" {
		return "", unknownResult(err)
	}
	if uninstallReconfiguresDatabaseTools(service) {
		_ = p.withStandaloneAgentMutation(ctx, "dbtools_configure", service, "", func(callCtx context.Context, binding agentMutationBinding) error {
			var dbt transport.ConfigureDBToolsResponse
			return p.callAgentContext(callCtx, "Agent.ConfigureDBTools", &transport.ServiceMutationRequest{ServiceMutationBinding: binding}, &dbt)
		})
	}
	if err := p.syncFirewallLocked(ctx); err != nil {
		log.Printf("[emergency] firewall sync after removal: %v", err)
	}
	if _, err := p.scanManagedServices(ctx); err != nil {
		log.Printf("[emergency] rescan after removal: %v", err)
	}
	detail := "package removed; customer files, databases and mailboxes are kept"
	if resp.Error != "" {
		detail = "package removed, with a follow-up problem: " + resp.Error
	}
	return detail, nil
}

type emergencyEffectRow struct {
	Seq    int
	Kind   string
	Target string
	Ref    emergencyEffectRef
	State  string
	Detail string
}

func loadEmergencyEffects(ctx context.Context, q sqlQueryer, opID string) ([]emergencyEffectRow, error) {
	rows, err := q.QueryContext(ctx, `SELECT seq, kind, target, ref, state, COALESCE(detail, '')
		FROM security_emergency_effects WHERE operation_id = ? ORDER BY seq`, opID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []emergencyEffectRow
	for rows.Next() {
		var e emergencyEffectRow
		var ref string
		if err := rows.Scan(&e.Seq, &e.Kind, &e.Target, &ref, &e.State, &e.Detail); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(ref), &e.Ref); err != nil {
			return nil, fmt.Errorf("effect %d reference: %w", e.Seq, err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (p *Panel) setEmergencyEffect(ctx context.Context, opID string, seq int, state, detail string) error {
	_, err := p.db.GetDB().ExecContext(ctx, `UPDATE security_emergency_effects SET state = ?, detail = ?, updated_at = datetime('now')
		WHERE operation_id = ? AND seq = ?`, state, nullIfEmpty(detail), opID, seq)
	return err
}

// runEmergencyOperation continues one running operation from its first
// unfinished checkpoint.
func (p *Panel) runEmergencyOperation(ctx context.Context, opID string) error {
	db := p.db.GetDB()
	op, err := loadEmergencyOperation(ctx, db, opID)
	if err != nil {
		return err
	}
	if op.State != "running" {
		return nil
	}
	e, err := loadEmergency(ctx, db, op.EmergencyID)
	if err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `UPDATE security_emergency_operations SET attempts = attempts + 1, updated_at = datetime('now')
		WHERE id = ? AND state = 'running'`, opID); err != nil {
		return err
	}
	op.Attempts++
	effects, err := loadEmergencyEffects(ctx, db, opID)
	if err != nil {
		return err
	}
	native := p.emergencyNativeOps()
	failedSites := 0
	for _, effect := range effects {
		if effectFinished(effect.State) {
			continue
		}
		// A move whose site could not be moved stays failed; the others go on.
		if effect.State == effectFailed && op.Kind == emergencyOpMove && strings.HasPrefix(effect.Detail, "not moved: ") {
			failedSites++
			continue
		}
		state, detail, stepErr := p.runEmergencyEffect(ctx, native, e, op, effect)
		if stepErr != nil {
			kind := stepErrorKind(stepErr)
			message := fmt.Sprintf("%s %s: %v", effect.Kind, effect.Target, stepErr)
			if op.Kind == emergencyOpMove && kind != "unknown" {
				failedSites++
				if err := p.setEmergencyEffect(ctx, opID, effect.Seq, effectFailed, "not moved: "+stepErr.Error()); err != nil {
					return err
				}
				continue
			}
			if err := p.setEmergencyEffect(ctx, opID, effect.Seq, effectFailed, stepErr.Error()); err != nil {
				return err
			}
			if kind == "unknown" && op.Attempts < emergencyAutoAttempts {
				_, err := db.ExecContext(ctx, `UPDATE security_emergency_operations SET error_kind = ?, last_error = ?, updated_at = datetime('now') WHERE id = ?`, kind, message, opID)
				return err
			}
			if op.Kind == emergencyOpMove {
				failedSites++
				if err := p.setEmergencyEffect(ctx, opID, effect.Seq, effectFailed, "not moved: "+stepErr.Error()); err != nil {
					return err
				}
				continue
			}
			if _, err := db.ExecContext(ctx, `UPDATE security_emergency_operations SET state = 'failed', error_kind = ?, last_error = ?,
				updated_at = datetime('now'), finished_at = datetime('now') WHERE id = ? AND state = 'running'`, kind, message, opID); err != nil {
				return err
			}
			p.auditEmergencySystem(ctx, op.ActorID, "security.emergency.failed:"+op.Kind+":"+e.Name, map[string]any{
				"emergency": e.ID, "operation": op.ID, "step": op.Kind, "target": e.Name, "error_kind": kind, "error": message})
			return nil
		}
		if err := p.setEmergencyEffect(ctx, opID, effect.Seq, state, detail); err != nil {
			return err
		}
	}
	return p.finishEmergencyOperation(ctx, e, op, failedSites)
}

func (p *Panel) finishEmergencyOperation(ctx context.Context, e securityEmergency, op emergencyOperationRow, failedSites int) error {
	tx, err := p.db.GetDB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	final := "completed"
	if failedSites > 0 {
		final = "partial"
	}
	result, err := tx.ExecContext(ctx, `UPDATE security_emergency_operations SET state = ?, error_kind = NULL, last_error = NULL,
		updated_at = datetime('now'), finished_at = datetime('now') WHERE id = ? AND state = 'running'`, final, op.ID)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return nil
	}
	switch op.Kind {
	case emergencyOpMove:
		// Each customer (and reseller) whose site the administrator moved is
		// told where it went (release decision d). Every move of this
		// emergency counts, so a second move renews the notice with all.
		sites, err := emergencyAdminMovedSites(ctx, tx, e.ID)
		if err != nil {
			return err
		}
		if len(sites) > 0 {
			if err := createEmergencyNotices(ctx, tx, e, sites, "moved"); err != nil {
				return err
			}
		}
	case emergencyOpRemove:
		if _, err := tx.ExecContext(ctx, `UPDATE security_emergencies SET stage = 'removed', updated_at = datetime('now') WHERE id = ?`, e.ID); err != nil {
			return err
		}
	case emergencyOpReenable:
		stage, withdrawn := emergencyStageResolved, 0
		if op.KeepWithdrawn {
			stage, withdrawn = emergencyStageWithdrawn, 1
		}
		if _, err := tx.ExecContext(ctx, `UPDATE security_emergencies SET stage = ?, withdrawn = ?, updated_at = datetime('now'),
			resolved_at = CASE WHEN ? = 'resolved' THEN datetime('now') END, resolved_by = CASE WHEN ? = 'resolved' THEN ? END
			WHERE id = ?`, stage, withdrawn, stage, stage, nullablePositiveInt(op.ActorID), e.ID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM security_emergency_before_images WHERE emergency_id = ?`, e.ID); err != nil {
			return err
		}
		sites, err := emergencyRecordedSites(ctx, tx, e.ID)
		if err != nil {
			return err
		}
		e.Stage = stage
		if err := createEmergencyNotices(ctx, tx, e, sites, "resolved"); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	p.auditEmergencySystem(ctx, op.ActorID, "security.emergency.finished:"+op.Kind+":"+e.Name, map[string]any{
		"emergency": e.ID, "operation": op.ID, "step": op.Kind, "target": e.Name, "state": final, "sites_not_moved": failedSites})
	return nil
}

// emergencyAdminMovedSites are the recorded sites a move operation of this
// emergency moved (not the ones their customers moved themselves), each with
// the version it was moved to.
func emergencyAdminMovedSites(ctx context.Context, q sqlQueryer, emergencyID string) ([]emergencySite, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT json_extract(ef.ref, '$.domain_id'), json_extract(ef.ref, '$.to')
		FROM security_emergency_effects ef
		JOIN security_emergency_operations o ON o.id = ef.operation_id
		WHERE o.emergency_id = ? AND o.kind = 'move' AND ef.kind = 'move' AND ef.state = 'done' AND ef.detail LIKE 'moved to %'
		ORDER BY o.created_at, o.rowid, ef.seq`, emergencyID)
	if err != nil {
		return nil, err
	}
	movedTo := map[int]string{}
	for rows.Next() {
		var domainID int
		var to string
		if err := rows.Scan(&domainID, &to); err != nil {
			rows.Close()
			return nil, err
		}
		movedTo[domainID] = to
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	recorded, err := emergencyRecordedSites(ctx, q, emergencyID)
	if err != nil {
		return nil, err
	}
	out := []emergencySite{}
	for _, site := range recorded {
		if to, ok := movedTo[site.DomainID]; ok {
			site.MovedTo = to
			out = append(out, site)
		}
	}
	return out, nil
}

// emergencyBeforeImage loads or captures one before-image (see beforeImage
// in suspension_run.go: an image is never replaced once stored).
func (p *Panel) emergencyBeforeImage(ctx context.Context, emergencyID, kind, target string, capture func() (any, error), into any) (bool, error) {
	db := p.db.GetDB()
	var raw string
	err := db.QueryRowContext(ctx, `SELECT before FROM security_emergency_before_images WHERE emergency_id = ? AND kind = ? AND target = ?`,
		emergencyID, kind, target).Scan(&raw)
	if err == nil {
		return true, json.Unmarshal([]byte(raw), into)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	if capture == nil {
		return false, nil
	}
	value, err := capture()
	if err != nil {
		return false, err
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return false, err
	}
	if _, err := db.ExecContext(ctx, `INSERT OR IGNORE INTO security_emergency_before_images (emergency_id, kind, target, before) VALUES (?, ?, ?, ?)`,
		emergencyID, kind, target, string(encoded)); err != nil {
		return false, err
	}
	return p.emergencyBeforeImage(ctx, emergencyID, kind, target, nil, into)
}

// siteOnEmergencyVersion: the site still uses the emergency's version.
func siteOnEmergencyVersion(ctx context.Context, q sqlQueryer, e securityEmergency, siteID int) (bool, string, error) {
	var projectType, phpVersion, runtimeVersion string
	err := q.QueryRowContext(ctx, `SELECT COALESCE(project_type, 'php'), COALESCE(php_version, ''), COALESCE(runtime_version, '') FROM sites WHERE id = ?`, siteID).
		Scan(&projectType, &phpVersion, &runtimeVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return false, "", nil
	}
	if err != nil {
		return false, "", err
	}
	switch e.Component {
	case "php":
		return projectType == "php" && phpVersion == e.Version, phpVersion, nil
	case "node":
		return projectType == "node" && runtimeVersion == e.Version, runtimeVersion, nil
	}
	return true, "", nil
}

func (p *Panel) runEmergencyEffect(ctx context.Context, native emergencyNative, e securityEmergency, op emergencyOperationRow, effect emergencyEffectRow) (string, string, error) {
	db := p.db.GetDB()
	switch effect.Kind {
	case emergencyEffectSite:
		restore := op.Kind == emergencyOpReenable
		if restore {
			if _, err := db.ExecContext(ctx, `UPDATE security_emergency_sites SET maintenance = 0, updated_at = datetime('now')
				WHERE emergency_id = ? AND domain_id = ?`, e.ID, effect.Ref.DomainID); err != nil {
				return "", "", err
			}
		}
		if err := native.renderSite(ctx, effect.Ref.DomainID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return effectDone, "the site no longer exists", nil
			}
			var held *siteFileHeldError
			if errors.As(err, &held) {
				return "", "", &suspensionStepError{kind: "prerequisite", err: errors.New(
					"the site's nginx file was changed on the server, so CelikPanel kept it and did not write the new page; the administrator reviews the file on the domain's Site configuration screen, then continues this step")}
			}
			return "", "", unknownResult(err)
		}
		if restore {
			return effectDone, "serves the site again; nginx validated and reloaded", nil
		}
		return effectDone, "shows the maintenance page (503); nginx validated and reloaded", nil

	case emergencyEffectUnits:
		lock := op.Kind == emergencyOpStop
		var before map[string]transport.EmergencyUnitState
		var capture func() (any, error)
		if lock {
			capture = func() (any, error) {
				resp, err := native.inspectUnits(ctx, effect.Ref.Service, effect.Ref.Version)
				if err != nil {
					return nil, unknownResult(err)
				}
				if resp.Error != "" {
					return nil, verifiedFailure("%s", resp.Error)
				}
				image := map[string]transport.EmergencyUnitState{}
				for _, unit := range resp.Units {
					image[unit.Unit] = unit.State
				}
				return image, nil
			}
		}
		found, err := p.emergencyBeforeImage(ctx, e.ID, emergencyEffectUnits, effect.Target, capture, &before)
		if err != nil {
			return "", "", err
		}
		if !found {
			return effectDone, "nothing was locked; nothing to restore", nil
		}
		if lock {
			_ = p.setEmergencyEffect(ctx, op.ID, effect.Seq, effectCaptured, "")
			if len(before) == 0 {
				return effectDone, "no unit of it runs on this server; nothing to stop", nil
			}
		}
		resp, err := native.setUnitLock(ctx, transport.SetEmergencyUnitLockRequest{Service: effect.Ref.Service, Version: effect.Ref.Version, Lock: lock, Before: before})
		if err != nil {
			return "", "", unknownResult(err)
		}
		names := []string{}
		for _, unit := range resp.Units {
			names = append(names, unit.Unit)
		}
		if lock {
			return applyResult(resp.SuspensionApplyResult, "stopped and locked (masked): "+strings.Join(names, ", "), "already stopped and locked: "+strings.Join(names, ", "))
		}
		if resp.Error != "" && !resp.Conflict && !resp.Applied {
			return "", "", verifiedFailure("%s; if the package was removed, install the patched version first, then re-enable again", resp.Error)
		}
		return applyResult(resp.SuspensionApplyResult, "unlocked and started as before: "+strings.Join(names, ", "), "already as before: "+strings.Join(names, ", "))

	case emergencyEffectApp:
		stop := op.Kind == emergencyOpStop
		onVersion, _, err := siteOnEmergencyVersion(ctx, db, e, effect.Ref.SiteID)
		if err != nil {
			return "", "", err
		}
		var before transport.AppUnitState
		var capture func() (any, error)
		if stop {
			if !onVersion {
				return effectDone, "the site no longer uses this version; nothing to stop", nil
			}
			capture = func() (any, error) {
				state, err := native.inspectApp(ctx, effect.Ref.SiteID)
				if err != nil {
					return nil, unknownResult(err)
				}
				return state, nil
			}
		}
		found, err := p.emergencyBeforeImage(ctx, e.ID, emergencyEffectApp, effect.Target, capture, &before)
		if err != nil {
			return "", "", err
		}
		if !found {
			return effectDone, "nothing was stopped; nothing to restore", nil
		}
		if stop {
			_ = p.setEmergencyEffect(ctx, op.ID, effect.Seq, effectCaptured, "")
		}
		if !before.Present {
			return effectDone, "the site has no Node.js app unit; nothing to change", nil
		}
		if !stop {
			// The start lock goes first; a lock changed on the server is kept
			// and the app is not started (it could not start anyway).
			kept, err := emergencyAppLockStep(ctx, native, effect.Ref.SiteID, e.Version, false)
			if err != nil {
				return "", "", err
			}
			if kept != "" {
				return effectKept, kept + "; the application was not started. The administrator reviews or deletes that file, then starts the application from the site's page", nil
			}
			if !onVersion {
				return effectDone, "the site moved to another version; nothing to restore", nil
			}
			// A suspended subscription keeps it stopped: the suspension
			// takes over the earliest before-image and starts it on resume.
			if handed, err := p.handEmergencyAppToSuspension(ctx, e.ID, effect.Ref.DomainID, effect.Target, before); err != nil {
				return "", "", err
			} else if handed {
				return effectDone, "kept stopped: the subscription is suspended; it starts again when the subscription is resumed", nil
			}
		}
		resp, err := native.setAppUnit(ctx, transport.SetAppUnitSuspensionRequest{SiteID: effect.Ref.SiteID, Suspend: stop, Before: before})
		if err != nil {
			return "", "", unknownResult(err)
		}
		if stop {
			state, detail, err := applyResult(resp.SuspensionApplyResult, "Node.js app stopped and kept from starting at boot", "already stopped")
			if err != nil {
				return "", "", err
			}
			// The start lock: systemd itself refuses to start it on this
			// version, whatever asks (release decision a).
			kept, err := emergencyAppLockStep(ctx, native, effect.Ref.SiteID, e.Version, true)
			if err != nil {
				return "", "", err
			}
			if kept != "" {
				return effectKept, detail + "; " + kept, nil
			}
			return state, detail + "; systemd refuses to start it on Node.js " + e.Version, nil
		}
		if !before.Active {
			return applyResult(resp.SuspensionApplyResult, "boot setting put back; the app was not running before, so it was not started", "already as it was")
		}
		return applyResult(resp.SuspensionApplyResult, "Node.js app started again", "already running")

	case emergencyEffectMove:
		onVersion, current, err := siteOnEmergencyVersion(ctx, db, e, effect.Ref.SiteID)
		if err != nil {
			return "", "", err
		}
		note := ""
		if onVersion {
			switch e.Component {
			case "php":
				err = native.movePHP(ctx, effect.Ref.DomainID, effect.Ref.From, effect.Ref.To, e.Stage == emergencyStageRemoved)
			case "node":
				note, err = native.moveNode(ctx, effect.Ref.DomainID, effect.Ref.From, effect.Ref.To)
			default:
				err = verifiedFailure("a whole component has no other version to move to")
			}
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return effectDone, "the site no longer exists", nil
				}
				return "", "", err
			}
			current = effect.Ref.To
		}
		if _, err := db.ExecContext(ctx, `UPDATE security_emergency_sites SET maintenance = 0, moved_to = ?, updated_at = datetime('now')
			WHERE emergency_id = ? AND domain_id = ?`, nullIfEmpty(current), e.ID, effect.Ref.DomainID); err != nil {
			return "", "", err
		}
		// The site's own page again, on the new version.
		if err := native.renderSite(ctx, effect.Ref.DomainID); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return "", "", unknownResult(err)
		}
		if !onVersion {
			return effectDone, "already on " + current + "; its page is shown again", nil
		}
		if note != "" {
			return effectDone, "moved to " + effect.Ref.To + " and verified; its page is shown again; " + note, nil
		}
		return effectDone, "moved to " + effect.Ref.To + " and verified; its page is shown again", nil

	case emergencyEffectPackage:
		detail, err := native.removePackage(ctx, effect.Ref.Service, effect.Ref.Version)
		if err != nil {
			return "", "", err
		}
		return effectDone, detail, nil
	}
	return "", "", verifiedFailure("unknown effect kind %q", effect.Kind)
}

// handEmergencyAppToSuspension: when a re-enable finds the site's
// subscription suspended, the app stays stopped and the suspension's
// before-image becomes the earliest one known, so its resume restores the
// state before both.
func (p *Panel) handEmergencyAppToSuspension(ctx context.Context, emergencyID string, domainID int, domain string, before transport.AppUnitState) (bool, error) {
	db := p.db.GetDB()
	var subscriptionID int
	var status string
	if err := db.QueryRowContext(ctx, `SELECT s.id, s.status FROM domains d JOIN subscriptions s ON s.id = d.subscription_id WHERE d.id = ?`, domainID).
		Scan(&subscriptionID, &status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	if !subscriptionStatusSuspended(status) {
		return false, nil
	}
	encoded, err := json.Marshal(before)
	if err != nil {
		return false, err
	}
	// Ours replaces the suspension's only when ours was captured first.
	if _, err := db.ExecContext(ctx, `
		INSERT INTO suspension_before_images (subscription_id, kind, target, before, captured_at)
		SELECT ?, 'app', ?, ?, captured_at FROM security_emergency_before_images
		WHERE emergency_id = ? AND kind = 'app' AND target = ?
		ON CONFLICT(subscription_id, kind, target) DO UPDATE SET before = excluded.before, captured_at = excluded.captured_at
		WHERE excluded.captured_at < suspension_before_images.captured_at`,
		subscriptionID, domain, string(encoded), emergencyID, domain); err != nil {
		return false, err
	}
	return true, nil
}

// handSuspensionAppToEmergency is the other direction: a suspension resume
// finds the app held by a security emergency; it stays stopped and the
// emergency's before-image becomes the earliest one known.
func (p *Panel) handSuspensionAppToEmergency(ctx context.Context, subscriptionID int, domainID int, domain string) (bool, error) {
	db := p.db.GetDB()
	var emergencyID string
	err := db.QueryRowContext(ctx, `
		SELECT e.id FROM security_emergency_sites es
		JOIN security_emergencies e ON e.id = es.emergency_id
		JOIN sites s ON s.id = es.site_id
		WHERE es.domain_id = ? AND es.maintenance = 1 AND e.component = 'node' AND e.stage IN ('stopped', 'removed')
		  AND s.project_type = 'node' AND s.runtime_version = e.version
		ORDER BY e.created_at LIMIT 1`, domainID).Scan(&emergencyID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO security_emergency_before_images (emergency_id, kind, target, before, captured_at)
		SELECT ?, 'app', ?, before, captured_at FROM suspension_before_images
		WHERE subscription_id = ? AND kind = 'app' AND target = ?
		ON CONFLICT(emergency_id, kind, target) DO UPDATE SET before = excluded.before, captured_at = excluded.captured_at
		WHERE excluded.captured_at < security_emergency_before_images.captured_at`,
		emergencyID, domain, subscriptionID, domain); err != nil {
		return false, err
	}
	return true, nil
}

func (p *Panel) auditEmergencySystem(ctx context.Context, actorID int, action string, details map[string]any) {
	p.recordAudit(auditRecord{UserID: actorID, Action: action, ResourceType: "service", Details: auditDetailsText(details)})
}

// runPendingEmergencyOperations runs every running operation once, oldest
// first, then hands pending notices to the mail system.
func (p *Panel) runPendingEmergencyOperations(ctx context.Context) error {
	p.emergencyWorker.mu.Lock()
	defer p.emergencyWorker.mu.Unlock()
	rows, err := p.db.GetDB().QueryContext(ctx, `SELECT id FROM security_emergency_operations WHERE state = 'running' ORDER BY created_at, rowid`)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	var errs []error
	for _, id := range ids {
		if err := p.runEmergencyOperation(ctx, id); err != nil {
			errs = append(errs, fmt.Errorf("security emergency operation %s: %w", id, err))
		}
	}
	if err := p.sendPendingSecurityNotices(ctx); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (p *Panel) kickEmergencyWorker() {
	if p.emergencyWorker.kick == nil {
		return
	}
	select {
	case p.emergencyWorker.kick <- struct{}{}:
	default:
	}
}

// startEmergencyWorker runs operations on every kick and every minute; at
// start it continues what a crash left running.
func (p *Panel) startEmergencyWorker() {
	p.emergencyWorker.once.Do(func() {
		p.emergencyWorker.kick = make(chan struct{}, 1)
		go func() {
			ticker := time.NewTicker(time.Minute)
			defer ticker.Stop()
			for {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
				if err := p.runPendingEmergencyOperations(ctx); err != nil {
					log.Printf("[emergency] %v", err)
				}
				cancel()
				select {
				case <-ticker.C:
				case <-p.emergencyWorker.kick:
				}
			}
		}()
	})
}
