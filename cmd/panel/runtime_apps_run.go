package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/siteruntime"
	"github.com/alicelik/celikpanel/internal/transport"
)

// The runtime-app operation (D-025). Steps, each repeatable:
//
//	deploy: validate -> [build] -> render -> restart -> proof -> health -> publish -> done
//	remove: unpublish -> remove -> done
//
// build is the Python kind's: the Agent builds the next environment beside
// the running one (a failed build changes nothing that runs), and its record
// names the generation built and the one that ran before. When a Python app
// that was deployed before does not come up after the restart, the unit is
// rendered again with the environment it ran with and restarted (one step
// back), and the operation still fails with what went wrong.
//
// Nothing secret is in memory only: the environment file is already on the
// server, so a Panel restart continues the same operation from its step.
// Status reads (the screens' polling) never start a step; one operation runs
// per app (a unique index), and a repeated operation id is answered with the
// first answer.
//
// Çalışma ortamı uygulaması işlemi: her adım tekrarlanabilir; Panel yeniden
// başlarsa aynı işlem kaldığı adımdan sürer. Ekranın durum sorgusu hiçbir
// adımı başlatmaz.

var (
	runtimeAppActiveOps sync.Map // operation id -> struct{}
	// runtimeAppStartTimeout bounds the wait for the program to listen.
	runtimeAppStartTimeout = 2 * time.Minute
	// runtimeAppHealthTimeout bounds the wait for a healthy answer.
	runtimeAppHealthTimeout = 90 * time.Second
	// runtimeAppBuildTimeout bounds a Python app's build (the unit's own
	// limit is siteruntime.BuildTimeoutSec).
	runtimeAppBuildTimeout = 35 * time.Minute
)

// startRuntimeAppOperation runs an operation in the background once.
func (p *Panel) startRuntimeAppOperation(opID string) {
	if _, busy := runtimeAppActiveOps.LoadOrStore(opID, struct{}{}); busy {
		return
	}
	go func() {
		defer runtimeAppActiveOps.Delete(opID)
		ctx, cancel := context.WithTimeout(context.Background(), runtimeAppBuildTimeout+runtimeAppStartTimeout+runtimeAppHealthTimeout+15*time.Minute)
		defer cancel()
		if err := p.runRuntimeAppOperation(ctx, opID); err != nil {
			log.Printf("[runtime apps] operation %s: %v", opID, err)
		}
	}()
}

// resumeRuntimeAppOperations continues what a restart left running.
func (p *Panel) resumeRuntimeAppOperations() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	rows, err := p.db.GetDB().QueryContext(ctx, `SELECT id FROM runtime_app_operations WHERE state = 'running'`)
	if err != nil {
		log.Printf("[runtime apps] resume: %v", err)
		return
	}
	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	for _, id := range ids {
		p.startRuntimeAppOperation(id)
	}
}

// runtimeStatusFailure reads a failed start out of a status: restart loop,
// out of memory, a listener on another address or a taken port.
func runtimeStatusFailure(s transport.RuntimeAppStatusResponse, port int) (reason, message string) {
	switch {
	case len(s.Exposed) > 0:
		return "listens_elsewhere", "the program listened on " + strings.Join(s.Exposed, ", ") + ", not only on 127.0.0.1; it was stopped. Read HOST and PORT from the environment and listen on 127.0.0.1"
	case s.Squatted:
		return "port_taken", fmt.Sprintf("another program already listens on 127.0.0.1:%d, so this one was stopped", port)
	case s.OOMKills > 0 || s.Result == "oom-kill":
		return "oom", "the program used more memory than its limit and was stopped by the kernel"
	case s.StartLimitHit:
		return "restart_loop", "the program stopped again and again right after starting, so systemd stopped restarting it; its log shows why"
	}
	return "", ""
}

func (p *Panel) runRuntimeAppOperation(ctx context.Context, opID string) error {
	op, err := p.runtimeAppOperationByID(ctx, opID)
	if err != nil {
		return err
	}
	if op.State != "running" {
		return nil
	}
	a, err := p.runtimeAppByID(ctx, op.AppID)
	if err != nil {
		p.failRuntimeAppOperation(ctx, op, transport.RuntimeAppUnknown, "app_missing", "the app record is gone")
		return err
	}
	native := p.runtimeAppNativeOps()
	if op.Kind == "remove" {
		return p.runRuntimeAppRemove(ctx, native, op, a)
	}
	offerings, err := p.loadRuntimeOfferings(ctx)
	if err != nil {
		return err
	}
	o, has := offerings[a.OfferingID]
	spec := a.spec(o, has)
	// The gates of new use, read again at validate, before the program is
	// started and before the site is switched to it: a hold, a withdrawn
	// offer or a suspension that came while the deploy ran stops it there.
	stop := func() {
		_, _ = native.control(ctx, transport.RuntimeAppControlRequest{Target: a.target(), Action: "stop"})
	}
	gate := func(started bool) bool {
		reason, message := p.runtimeDeployGate(ctx, a)
		if reason == "" {
			return true
		}
		if started {
			stop()
		}
		p.failRuntimeAppOperation(ctx, op, transport.RuntimeAppPrerequisite, reason, message)
		return false
	}
	if op.Step == "validate" {
		if !gate(false) {
			return nil
		}
		resp, err := native.check(ctx, transport.RuntimeAppCheckRequest{Target: a.target(), Spec: spec})
		if err != nil {
			p.failRuntimeAppOperation(ctx, op, transport.RuntimeAppUnknown, "agent_unreachable", "the server did not answer: "+err.Error())
			return nil
		}
		if !resp.Done {
			p.failRuntimeAppOperation(ctx, op, resp.Class, resp.Reason, resp.Error)
			return nil
		}
		op.Step = "render"
		if a.Kind == siteruntime.KindPython {
			op.Step = "build"
		}
		_ = p.setRuntimeAppStep(ctx, op.ID, op.Step)
	}
	if op.Step == "build" {
		if !gate(false) {
			return nil
		}
		if !p.runRuntimePythonBuild(ctx, native, op, a, spec) {
			return nil
		}
		op.Step = "render"
		_ = p.setRuntimeAppStep(ctx, op.ID, op.Step)
	}
	if a.Kind == siteruntime.KindPython && spec.Python != nil {
		// The generation this deploy built, from the Agent's record of the
		// operation (a restarted Panel reads it again).
		built, err := native.buildAdvance(ctx, transport.RuntimeAppBuildAdvanceRequest{OperationID: op.ID, Target: a.target()})
		if err != nil {
			p.failRuntimeAppOperation(ctx, op, transport.RuntimeAppUnknown, "agent_unreachable", "the server did not answer: "+err.Error())
			return nil
		}
		if !built.Done || built.Phase != siteruntime.BuildPhaseSucceeded {
			p.failRuntimeAppOperation(ctx, op, transport.RuntimeAppUnknown, siteruntime.BuildFailed, "the server has no finished build of this deploy; deploy again")
			return nil
		}
		py := *spec.Python
		py.Venv = built.Venv
		spec.Python = &py
	}
	// failStarted ends a deploy whose new version did not come up: a
	// Python app that ran before goes back to the environment it ran with.
	failStarted := func(stopFirst bool, reason, message string) {
		if stopFirst {
			stop()
		}
		if a.Kind == siteruntime.KindPython && a.Deployed {
			message += p.runtimePythonRollback(ctx, native, op.ID, a, spec)
		}
		p.failRuntimeAppOperation(ctx, op, transport.RuntimeAppVerifiedFailure, reason, message)
	}
	if op.Step == "render" {
		resp, err := native.render(ctx, transport.RuntimeAppRenderRequest{OperationID: op.ID, Target: a.target(), Spec: spec})
		if err != nil {
			p.failRuntimeAppOperation(ctx, op, transport.RuntimeAppUnknown, "agent_unreachable", "the server did not answer: "+err.Error())
			return nil
		}
		if !resp.Done {
			p.failRuntimeAppOperation(ctx, op, resp.Class, resp.Reason, resp.Error)
			return nil
		}
		op.Step = "restart"
		_ = p.setRuntimeAppStep(ctx, op.ID, op.Step)
	}
	if op.Step == "restart" {
		if !gate(false) {
			return nil
		}
		resp, err := native.control(ctx, transport.RuntimeAppControlRequest{Target: a.target(), Action: "restart"})
		if err != nil {
			p.failRuntimeAppOperation(ctx, op, transport.RuntimeAppUnknown, "agent_unreachable", "the server did not answer: "+err.Error())
			return nil
		}
		if !resp.Done {
			p.failRuntimeAppOperation(ctx, op, resp.Class, resp.Reason, resp.Error)
			return nil
		}
		op.Step = "proof"
		_ = p.setRuntimeAppStep(ctx, op.ID, op.Step)
	}
	if op.Step == "proof" {
		deadline := time.Now().Add(runtimeAppStartTimeout)
		for {
			status, err := native.status(ctx, transport.RuntimeAppStatusRequest{Target: a.target(), Spec: spec, Proof: true})
			if err == nil && status.Error == "" {
				if reason, message := runtimeStatusFailure(status, a.Port); reason != "" {
					failStarted(true, reason, message)
					return nil
				}
				if status.Serving && status.Proof != nil {
					if !status.Proof.OK {
						// The loaded sandbox differs from the profile (an
						// owner's drop-in, a missing mail block): the program is
						// stopped and never published.
						stop()
						p.failRuntimeAppOperation(ctx, op, transport.RuntimeAppVerifiedFailure, "sandbox_proof",
							"the running program's sandbox differs from its profile ("+status.Proof.Code+": "+status.Proof.Detail+"); it was stopped and not published")
						return nil
					}
					break
				}
				if status.ActiveState == "failed" {
					failStarted(false, "start_failed", "the program stopped while starting ("+status.Result+"); its log shows why")
					return nil
				}
			}
			if time.Now().After(deadline) {
				if err != nil {
					p.failRuntimeAppOperation(ctx, op, transport.RuntimeAppUnknown, "agent_unreachable", "the server did not answer while the program started")
					return nil
				}
				failStarted(false, "not_listening",
					fmt.Sprintf("the program runs but did not listen on 127.0.0.1:%d in time; it must read PORT and HOST from its environment (or use {port} in its arguments)", a.Port))
				return nil
			}
			if !sleepContext(ctx, runtimeAppPollInterval) {
				return ctx.Err()
			}
		}
		op.Step = "health"
		_ = p.setRuntimeAppStep(ctx, op.ID, op.Step)
	}
	if op.Step == "health" {
		deadline := time.Now().Add(runtimeAppHealthTimeout)
		for {
			status, err := native.status(ctx, transport.RuntimeAppStatusRequest{Target: a.target(), Spec: spec, HealthPath: a.HealthPath})
			if err == nil && status.Error == "" {
				if reason, message := runtimeStatusFailure(status, a.Port); reason != "" {
					failStarted(true, reason, message)
					return nil
				}
				if status.Health.OK {
					break
				}
			}
			if time.Now().After(deadline) {
				detail := "no answer"
				if err == nil && status.Health.Status > 0 {
					detail = fmt.Sprintf("HTTP %d", status.Health.Status)
				}
				// The program keeps running so its log and its answers can be
				// read; the site is not switched to it.
				failStarted(false, "health_failing",
					"the program did not answer "+a.HealthPath+" with a success or client status ("+detail+"), so the site was not switched to it")
				return nil
			}
			if !sleepContext(ctx, runtimeAppPollInterval) {
				return ctx.Err()
			}
		}
		op.Step = "publish"
		_ = p.setRuntimeAppStep(ctx, op.ID, op.Step)
	}
	if op.Step == "publish" {
		if !gate(true) {
			return nil
		}
		if _, err := p.db.GetDB().ExecContext(ctx, `
			UPDATE sites SET project_type = 'runtime_app', app_port = ?, force_https = 1, updated_at = datetime('now')
			WHERE id = ? AND domain_id = ?`, a.Port, a.SiteID, a.DomainID); err != nil {
			p.failRuntimeAppOperation(ctx, op, transport.RuntimeAppUnknown, "publish", "the program runs, but the domain could not be switched to it: "+err.Error())
			return nil
		}
		if err := native.renderSite(ctx, a.DomainID); err != nil {
			p.failRuntimeAppOperation(ctx, op, transport.RuntimeAppUnknown, "publish", "the program runs, but the domain's nginx file could not be written: "+err.Error())
			return nil
		}
	}
	if _, err := p.db.GetDB().ExecContext(ctx, `
		UPDATE runtime_apps SET state = 'running', deployed = 1, updated_at = datetime('now') WHERE id = ?`, a.ID); err != nil {
		return err
	}
	return p.succeedRuntimeAppOperation(ctx, op.ID, "")
}

// runRuntimePythonBuild starts the build of the operation (once: the Agent
// answers a repeat with its record) and waits for it. It returns false when
// the operation ended. A build never touches the running app.
func (p *Panel) runRuntimePythonBuild(ctx context.Context, native runtimeAppNative, op runtimeAppOperation, a runtimeApp, spec transport.RuntimeAppSpec) bool {
	started, err := native.build(ctx, transport.RuntimeAppBuildRequest{OperationID: op.ID, Target: a.target(), Spec: spec})
	if err != nil {
		p.failRuntimeAppOperation(ctx, op, transport.RuntimeAppUnknown, "agent_unreachable", "the server did not answer: "+err.Error())
		return false
	}
	if !started.Done {
		p.failRuntimeAppOperation(ctx, op, started.Class, started.Reason, started.Error)
		return false
	}
	deadline := time.Now().Add(runtimeAppBuildTimeout)
	for {
		state, err := native.buildAdvance(ctx, transport.RuntimeAppBuildAdvanceRequest{OperationID: op.ID, Target: a.target()})
		if err == nil {
			if !state.Done {
				p.failRuntimeAppOperation(ctx, op, state.Class, state.Reason, state.Error)
				return false
			}
			if state.Phase == siteruntime.BuildPhaseSucceeded {
				return true
			}
		}
		if time.Now().After(deadline) {
			p.failRuntimeAppOperation(ctx, op, transport.RuntimeAppUnknown, siteruntime.BuildFailed,
				"the build did not finish in time (the server did not say it ended); the running app was not changed. Deploy again")
			return false
		}
		if !sleepContext(ctx, runtimeAppPollInterval) {
			return false
		}
	}
}

// runtimePythonRollback renders the unit again with the environment the app
// ran with before this deploy's build and restarts it; it returns the
// sentence the failure adds ("" when there is nothing to go back to).
func (p *Panel) runtimePythonRollback(ctx context.Context, native runtimeAppNative, opID string, a runtimeApp, spec transport.RuntimeAppSpec) string {
	built, err := native.buildAdvance(ctx, transport.RuntimeAppBuildAdvanceRequest{OperationID: opID, Target: a.target()})
	if err != nil || built.Phase != siteruntime.BuildPhaseSucceeded || built.Previous <= 0 || built.Previous == built.Venv || spec.Python == nil {
		return ""
	}
	back := spec
	py := *spec.Python
	py.Venv = built.Previous
	back.Python = &py
	rendered, err := native.render(ctx, transport.RuntimeAppRenderRequest{OperationID: opID, Target: a.target(), Spec: back})
	if err != nil || !rendered.Done {
		detail := "no answer"
		if err == nil {
			detail = rendered.Error
		}
		return ". Going back to the previous environment failed too: " + detail
	}
	restarted, err := native.control(ctx, transport.RuntimeAppControlRequest{Target: a.target(), Action: "restart"})
	if err != nil || !restarted.Done {
		return fmt.Sprintf(". The unit names the previous environment (venv-%d) again, but it did not restart; start it on this page", built.Previous)
	}
	return fmt.Sprintf(". The app was put back on its previous environment (venv-%d) and restarted; the code in app/src is what you uploaded", built.Previous)
}

// runtimeDeployGate reads the hold, the administrator's offer and the
// suspension now ("" = the deploy may go on).
func (p *Panel) runtimeDeployGate(ctx context.Context, a runtimeApp) (string, string) {
	offerings, err := p.loadRuntimeOfferings(ctx)
	if err != nil {
		return "agent_unreachable", "the offering could not be read: " + err.Error()
	}
	o, has := offerings[a.OfferingID]
	switch {
	case has && o.Held:
		return "held", "the administrator put " + runtimeKindName(a.Kind) + " on a security hold"
	case !has || !runtimeOfferOpen(o):
		return "not_offered", "the administrator no longer offers " + runtimeKindName(a.Kind)
	}
	if suspended, err := p.subscriptionSuspended(ctx, a.SubscriptionID); err == nil && suspended {
		return "suspended", "the subscription is suspended"
	}
	return "", ""
}

func (p *Panel) runRuntimeAppRemove(ctx context.Context, native runtimeAppNative, op runtimeAppOperation, a runtimeApp) error {
	if op.Step == "unpublish" {
		res, err := p.db.GetDB().ExecContext(ctx, `
			UPDATE sites SET project_type = ?, app_port = ?, updated_at = datetime('now')
			WHERE id = ? AND domain_id = ? AND project_type = 'runtime_app'`,
			a.PreviousProjectType, a.PreviousAppPort, a.SiteID, a.DomainID)
		if err != nil {
			p.failRuntimeAppOperation(ctx, op, transport.RuntimeAppUnknown, "unpublish", err.Error())
			return nil
		}
		if changed, _ := res.RowsAffected(); changed > 0 {
			if err := native.renderSite(ctx, a.DomainID); err != nil && !errors.Is(err, sql.ErrNoRows) {
				p.failRuntimeAppOperation(ctx, op, transport.RuntimeAppUnknown, "unpublish", "the domain's nginx file could not be written back: "+err.Error())
				return nil
			}
		}
		op.Step = "remove"
		_ = p.setRuntimeAppStep(ctx, op.ID, op.Step)
	}
	resp, err := native.remove(ctx, transport.RuntimeAppRemoveRequest{OperationID: op.ID, Target: a.target(), RemoveFiles: op.RemoveFiles})
	if err != nil {
		p.failRuntimeAppOperation(ctx, op, transport.RuntimeAppUnknown, "agent_unreachable", "the server did not answer: "+err.Error())
		return nil
	}
	if !resp.Done {
		p.failRuntimeAppOperation(ctx, op, resp.Class, resp.Reason, resp.Error)
		return nil
	}
	message := ""
	if resp.KeptCopy != "" {
		message = "the unit had been changed on the server; a copy is kept as " + resp.KeptCopy
	}
	if _, err := p.db.GetDB().ExecContext(ctx, `DELETE FROM runtime_apps WHERE id = ?`, a.ID); err != nil {
		return err
	}
	return p.succeedRuntimeAppOperation(ctx, op.ID, message)
}
