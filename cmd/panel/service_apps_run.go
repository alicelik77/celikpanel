package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/serviceapps"
	"github.com/alicelik/celikpanel/internal/transport"
)

// The service-app operation (D-025). Steps, each repeatable:
//
//	install/update: prepare -> installing -> activate -> verify -> publish -> finalize -> done
//	uninstall:      unpublish -> remove -> done
//
// prepare carries the customer's secrets, which live only in memory: if the
// Panel stops during prepare the operation ends as an unknown result and the
// customer starts it again (re-entering the keys). Every later step reads the
// server and continues; a crash continues the same operation at start, and
// the same operation id never starts the install unit twice. Status reads
// (the screens' polling) never start a step.
//
// Hizmet uygulaması işlemi: her adım tekrarlanabilir; çökme sonrası aynı işlem
// kaldığı adımdan sürer. Ekranın durum sorgusu hiçbir adımı başlatmaz.

var (
	serviceAppActiveOps sync.Map // operation id -> struct{}
	// serviceAppInstallTimeout bounds npm / pip.
	serviceAppInstallTimeout = 2 * time.Hour
	// serviceAppStartTimeout bounds the first start (Open WebUI downloads
	// its embedding model on first start).
	serviceAppStartTimeout = 20 * time.Minute
)

// serviceAppSecretInput is what only the prepare step needs.
type serviceAppSecretInput struct {
	OwnerEmail  string
	OwnerSecret string
	Secrets     map[string]string
}

// startServiceAppOperation runs an operation in the background once.
func (p *Panel) startServiceAppOperation(opID string, input *serviceAppSecretInput) {
	if _, busy := serviceAppActiveOps.LoadOrStore(opID, struct{}{}); busy {
		return
	}
	go func() {
		defer serviceAppActiveOps.Delete(opID)
		ctx, cancel := context.WithTimeout(context.Background(), serviceAppInstallTimeout+serviceAppStartTimeout+30*time.Minute)
		defer cancel()
		if err := p.runServiceAppOperation(ctx, opID, input); err != nil {
			log.Printf("[service apps] operation %s: %v", opID, err)
		}
	}()
}

// resumeServiceAppOperations continues what a restart left running.
func (p *Panel) resumeServiceAppOperations() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	rows, err := p.db.GetDB().QueryContext(ctx, `SELECT id FROM service_app_operations WHERE state = 'running'`)
	if err != nil {
		log.Printf("[service apps] resume: %v", err)
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
		p.startServiceAppOperation(id, nil)
	}
}

func (p *Panel) runServiceAppOperation(ctx context.Context, opID string, input *serviceAppSecretInput) error {
	op, err := p.serviceAppOperationByID(ctx, opID)
	if err != nil {
		return err
	}
	if op.State != "running" {
		return nil
	}
	in, err := p.serviceAppInstallByID(ctx, op.InstallID)
	if err != nil {
		p.failServiceAppOperation(ctx, op, transport.ServiceAppUnknown, "install_missing", "the install record is gone")
		return err
	}
	native := p.serviceAppNativeOps()
	if op.Kind == "uninstall" {
		return p.runServiceAppUninstall(ctx, native, op, in)
	}
	app, ok := serviceapps.Lookup(in.AppID)
	if !ok || !app.Installable() {
		p.failServiceAppOperation(ctx, op, transport.ServiceAppVerifiedFailure, "not_installable", "this release cannot install "+in.AppID)
		return nil
	}
	offerings, err := p.loadServiceAppOfferings(ctx)
	if err != nil {
		return err
	}
	limits := serviceAppLimitsFor(app, offerings)
	version := in.TargetVersion

	if op.Step == "prepare" {
		if input == nil {
			// The Panel stopped while the keys were in memory.
			p.failServiceAppOperation(ctx, op, transport.ServiceAppUnknown, "interrupted",
				"the Panel restarted while the install was being prepared; start it again")
			return nil
		}
		resp, err := native.prepare(ctx, transport.ServiceAppPrepareRequest{
			OperationID: op.ID, Target: in.target(), Version: version, Port: in.Port, AuxPort: in.AuxPort,
			Limits: limits, OwnerEmail: input.OwnerEmail, OwnerSecret: input.OwnerSecret, Secrets: input.Secrets,
		})
		if err != nil {
			p.failServiceAppOperation(ctx, op, transport.ServiceAppUnknown, "agent_unreachable", "the server did not answer: "+err.Error())
			return nil
		}
		if !resp.Done {
			p.failServiceAppOperation(ctx, op, resp.Class, resp.Reason, resp.Error)
			return nil
		}
		op.Step = "installing"
		_ = p.setServiceAppStep(ctx, op.ID, op.Step)
	}

	// Proof: a running unit is proved against its sandbox profile (systemd's
	// view, the SMTP block set, loopback only) before the site is published.
	statusReq := transport.ServiceAppStatusRequest{Target: in.target(), Version: version, Port: in.Port, OperationID: op.ID,
		Proof: true, AuxPort: in.AuxPort, Limits: limits}
	if op.Step == "installing" {
		deadline := time.Now().Add(serviceAppInstallTimeout)
		startedAt := time.Now()
		for {
			status, err := native.status(ctx, statusReq)
			if err == nil && status.Error == "" {
				if status.Installed && status.Install != "running" {
					break
				}
				if status.Install == "failed" {
					p.failServiceAppOperation(ctx, op, transport.ServiceAppVerifiedFailure, "install_failed",
						"the install step ended with an error ("+status.Result+"); its log shows why")
					return nil
				}
				if status.Install == "none" && time.Since(startedAt) > 3*time.Minute {
					p.failServiceAppOperation(ctx, op, transport.ServiceAppUnknown, "install_lost",
						"the install step is no longer on the server and the application is not in place")
					return nil
				}
			}
			if time.Now().After(deadline) {
				p.failServiceAppOperation(ctx, op, transport.ServiceAppUnknown, "install_timeout", "the install step did not finish in time")
				return nil
			}
			if !sleepContext(ctx, serviceAppPollInterval) {
				return ctx.Err()
			}
		}
		op.Step = "activate"
		_ = p.setServiceAppStep(ctx, op.ID, op.Step)
	}

	activateReq := transport.ServiceAppActivateRequest{OperationID: op.ID, Target: in.target(), Version: version,
		Port: in.Port, AuxPort: in.AuxPort, Limits: limits}
	if op.Step == "activate" {
		resp, err := native.activate(ctx, activateReq)
		if err != nil {
			p.failServiceAppOperation(ctx, op, transport.ServiceAppUnknown, "agent_unreachable", "the server did not answer: "+err.Error())
			return nil
		}
		if !resp.Done {
			p.failServiceAppOperation(ctx, op, resp.Class, resp.Reason, resp.Error)
			return nil
		}
		op.Step = "verify"
		_ = p.setServiceAppStep(ctx, op.ID, op.Step)
	}

	if op.Step == "verify" {
		deadline := time.Now().Add(serviceAppStartTimeout)
		for {
			status, err := native.status(ctx, statusReq)
			if err == nil && status.Error == "" {
				if len(status.Exposed) > 0 || status.Squatted {
					_, _ = native.control(ctx, transport.ServiceAppControlRequest{Target: in.target(), Action: "stop"})
					reason, why := "exposed", "the application listened on an address other than 127.0.0.1 ("+fmt.Sprint(status.Exposed)+"), so it was stopped"
					if status.Squatted {
						reason, why = "port_taken", fmt.Sprintf("another program already listens on 127.0.0.1:%d, so the application was stopped", in.Port)
					}
					p.failServiceAppOperation(ctx, op, transport.ServiceAppVerifiedFailure, reason, why)
					return nil
				}
				if status.Serving {
					if status.SandboxProof != nil && !status.SandboxProof.OK {
						_, _ = native.control(ctx, transport.ServiceAppControlRequest{Target: in.target(), Action: "stop"})
						p.failServiceAppOperation(ctx, op, transport.ServiceAppVerifiedFailure, "sandbox_proof", serviceAppSandboxProofFailure(status.SandboxProof))
						return nil
					}
					if status.SandboxProof != nil {
						break
					}
					// No proof yet (it could not be read): keep reading until the
					// deadline; the site is not published unproved.
				}
				if status.ActiveState == "failed" {
					p.failServiceAppOperation(ctx, op, transport.ServiceAppVerifiedFailure, "start_failed",
						"the application stopped while starting ("+status.Result+"); its log shows why")
					return nil
				}
			}
			if time.Now().After(deadline) {
				p.failServiceAppOperation(ctx, op, transport.ServiceAppUnknown, "start_timeout",
					"the application did not answer on its port in time; it may still be starting")
				return nil
			}
			if !sleepContext(ctx, serviceAppPollInterval) {
				return ctx.Err()
			}
		}
		op.Step = "publish"
		_ = p.setServiceAppStep(ctx, op.ID, op.Step)
	}

	if op.Step == "publish" {
		if err := p.publishServiceAppSite(ctx, in); err != nil {
			p.failServiceAppOperation(ctx, op, transport.ServiceAppUnknown, "publish",
				"the application runs, but the domain could not be switched to it: "+err.Error())
			return nil
		}
		if err := native.renderSite(ctx, in.DomainID); err != nil {
			p.failServiceAppOperation(ctx, op, transport.ServiceAppUnknown, "publish",
				"the application runs, but the domain's nginx file could not be written: "+err.Error())
			return nil
		}
		op.Step = "finalize"
		_ = p.setServiceAppStep(ctx, op.ID, op.Step)
	}

	message := ""
	if resp, err := native.finalize(ctx, activateReq); err != nil || !resp.Done {
		// The app runs and is published; only the cleanup of earlier files
		// is left. It is said, not hidden.
		message = "the application runs; removing the earlier version's files did not finish and is retried with the next update"
	}
	if _, err := p.db.GetDB().ExecContext(ctx, `
		UPDATE service_app_installs SET state = 'running', version = target_version, updated_at = datetime('now')
		WHERE id = ?`, in.ID); err != nil {
		return err
	}
	return p.succeedServiceAppOperation(ctx, op.ID, message)
}

// publishServiceAppSite turns the domain's site into the app's address:
// HTTPS only, proxied to 127.0.0.1:<port>. The site's earlier type is kept
// in the install row for the uninstall.
func (p *Panel) publishServiceAppSite(ctx context.Context, in serviceAppInstall) error {
	_, err := p.db.GetDB().ExecContext(ctx, `
		UPDATE sites SET project_type = 'service_app', app_port = ?, force_https = 1, updated_at = datetime('now')
		WHERE id = ? AND domain_id = ?`, in.Port, in.SiteID, in.DomainID)
	return err
}

func (p *Panel) runServiceAppUninstall(ctx context.Context, native serviceAppNative, op serviceAppOperation, in serviceAppInstall) error {
	if op.Step == "unpublish" {
		res, err := p.db.GetDB().ExecContext(ctx, `
			UPDATE sites SET project_type = ?, app_port = ?, updated_at = datetime('now')
			WHERE id = ? AND domain_id = ? AND project_type = 'service_app'`,
			in.PreviousProjectType, in.PreviousAppPort, in.SiteID, in.DomainID)
		if err != nil {
			p.failServiceAppOperation(ctx, op, transport.ServiceAppUnknown, "unpublish", err.Error())
			return nil
		}
		if changed, _ := res.RowsAffected(); changed > 0 {
			if err := native.renderSite(ctx, in.DomainID); err != nil && !errors.Is(err, sql.ErrNoRows) {
				p.failServiceAppOperation(ctx, op, transport.ServiceAppUnknown, "unpublish",
					"the domain's nginx file could not be written back: "+err.Error())
				return nil
			}
		}
		op.Step = "remove"
		_ = p.setServiceAppStep(ctx, op.ID, op.Step)
	}
	resp, err := native.remove(ctx, transport.ServiceAppRemoveRequest{OperationID: op.ID, Target: in.target(), RemoveData: op.RemoveData})
	if err != nil {
		p.failServiceAppOperation(ctx, op, transport.ServiceAppUnknown, "agent_unreachable", "the server did not answer: "+err.Error())
		return nil
	}
	if !resp.Done {
		p.failServiceAppOperation(ctx, op, resp.Class, resp.Reason, resp.Error)
		return nil
	}
	message := ""
	if resp.KeptCopy != "" {
		message = "the unit had been changed on the server; a copy is kept as " + resp.KeptCopy
	}
	if resp.KeptEnvCopy != "" {
		if message != "" {
			message += "; "
		}
		message += "the environment file had been changed on the server; a copy is kept as " + resp.KeptEnvCopy
	}
	if _, err := p.db.GetDB().ExecContext(ctx, `DELETE FROM service_app_installs WHERE id = ?`, in.ID); err != nil {
		return err
	}
	return p.succeedServiceAppOperation(ctx, op.ID, message)
}

func sleepContext(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
