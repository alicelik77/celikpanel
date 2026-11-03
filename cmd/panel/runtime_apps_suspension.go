package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Suspension of a subscription stops its runtime apps like its Node.js and
// service apps: systemd stops and disables each unit, so it stays stopped
// after a reboot and without the Panel; a resume restores only what ran
// before, compare-and-set (D-022). A held offering keeps its apps stopped at
// resume. The effects use the "app" kind with the app's identity in the
// reference (RuntimeAppID), so the suspension tables need no change.
//
// Askıya alma çalışma ortamı uygulamalarını da durdurur; geri açılışta yalnız
// önceden çalışanlar başlatılır; güvenlik beklemesindekiler durur.

func runtimeAppSuspensionTarget(domain string) string { return "runtime-app:" + domain }

// planRuntimeAppSuspension lists the subscription's deployed apps as effects.
func planRuntimeAppSuspension(ctx context.Context, q sqlQueryer, subscriptionID int, preview *suspensionPreview) ([]suspensionPlanItem, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT a.id, a.site_id, a.domain_id, d.name
		FROM runtime_apps a JOIN domains d ON d.id = a.domain_id
		WHERE a.subscription_id = ? AND (a.deployed = 1 OR a.state = 'deploying') ORDER BY d.name`, subscriptionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []suspensionPlanItem
	for rows.Next() {
		var a runtimeApp
		if err := rows.Scan(&a.ID, &a.SiteID, &a.DomainID, &a.Domain); err != nil {
			return nil, err
		}
		ref := suspensionEffectRef{SubscriptionID: subscriptionID, DomainID: a.DomainID, SiteID: a.SiteID, Domain: a.Domain, RuntimeAppID: a.ID}
		items = append(items, suspensionPlanItem{suspensionEffectApp, runtimeAppSuspensionTarget(a.Domain), ref})
		preview.Apps = append(preview.Apps, runtimeAppLabel(a))
	}
	return items, rows.Err()
}

func (p *Panel) runRuntimeAppSuspensionEffect(ctx context.Context, op suspensionOperationRow, e suspensionEffectRow) (string, string, error) {
	suspend := op.Kind == suspensionKindSuspend
	native := p.runtimeAppNativeOps()
	var before transport.AppUnitState
	var capture func() (any, error)
	if suspend {
		capture = func() (any, error) {
			resp, err := native.unitState(ctx, e.Ref.RuntimeAppID)
			if err != nil {
				return nil, unknownResult(err)
			}
			if resp.Error != "" {
				return nil, unknownResult(errors.New(resp.Error))
			}
			return resp.State, nil
		}
	}
	found, err := p.beforeImage(ctx, op.SubscriptionID, e.Kind, e.Target, capture, &before)
	if err != nil {
		return "", "", err
	}
	if !found {
		return effectDone, "nothing was changed when suspending; nothing to restore", nil
	}
	if suspend {
		_ = p.setEffectState(ctx, op.ID, e.Seq, effectCaptured, "")
	}
	if !before.Present {
		return effectDone, "the program has no unit on the server; nothing to change", nil
	}
	if !suspend {
		var held int
		if err := p.db.GetDB().QueryRowContext(ctx, `
			SELECT COALESCE((SELECT o.held FROM runtime_offerings o JOIN runtime_apps a ON a.offering_id = o.offering_id WHERE a.id = ?), 0)`,
			e.Ref.RuntimeAppID).Scan(&held); err != nil {
			return "", "", unknownResult(err)
		}
		if held == 1 {
			return effectDone, "kept stopped: the administrator holds uploaded programs for security; it is started again after the hold is lifted", nil
		}
	}
	resp, err := native.setSuspension(ctx, transport.SetRuntimeAppSuspensionRequest{AppID: e.Ref.RuntimeAppID, Suspend: suspend, Before: before})
	if err != nil {
		return "", "", unknownResult(err)
	}
	if suspend {
		return applyResult(resp.SuspensionApplyResult, "program stopped and kept from starting at boot", "already stopped")
	}
	if !before.Active {
		return applyResult(resp.SuspensionApplyResult, "boot setting put back; the program was not running before, so it was not started", "already as it was")
	}
	return applyResult(resp.SuspensionApplyResult, fmt.Sprintf("program on %s started again", e.Ref.Domain), "already running")
}
