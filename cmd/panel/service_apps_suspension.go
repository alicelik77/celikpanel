package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Suspension (alpha.117) of a subscription stops its service apps the same
// way as its Node.js apps: systemd stops and disables each unit, so it stays
// stopped after a reboot and without the Panel; a resume restores only what
// ran before, compare-and-set (D-022). The effects use the "app" kind with
// the install's identity in their reference, so the suspension tables of
// migration 053 need no change.
//
// Askıya alma, hizmet uygulamalarını Node.js uygulamaları gibi durdurur;
// geri açılışta yalnız önceden çalışanlar başlatılır.

func serviceAppSuspensionTarget(domain string) string { return "service-app:" + domain }

// planServiceAppSuspension lists the subscription's installs as effects.
func planServiceAppSuspension(ctx context.Context, q sqlQueryer, subscriptionID int, preview *suspensionPreview) ([]suspensionPlanItem, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT i.id, i.site_id, i.domain_id, d.name, i.app_id
		FROM service_app_installs i JOIN domains d ON d.id = i.domain_id
		WHERE i.subscription_id = ? AND i.version <> '' ORDER BY d.name`, subscriptionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []suspensionPlanItem
	for rows.Next() {
		var in serviceAppInstall
		if err := rows.Scan(&in.ID, &in.SiteID, &in.DomainID, &in.Domain, &in.AppID); err != nil {
			return nil, err
		}
		ref := suspensionEffectRef{SubscriptionID: subscriptionID, DomainID: in.DomainID, SiteID: in.SiteID, Domain: in.Domain, InstallID: in.ID}
		items = append(items, suspensionPlanItem{suspensionEffectApp, serviceAppSuspensionTarget(in.Domain), ref})
		preview.Apps = append(preview.Apps, serviceAppLabel(in))
	}
	return items, rows.Err()
}

func (p *Panel) runServiceAppSuspensionEffect(ctx context.Context, op suspensionOperationRow, e suspensionEffectRow) (string, string, error) {
	suspend := op.Kind == suspensionKindSuspend
	native := p.serviceAppNativeOps()
	var before transport.AppUnitState
	var capture func() (any, error)
	if suspend {
		capture = func() (any, error) {
			resp, err := native.unitState(ctx, e.Ref.InstallID)
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
		return effectDone, "the app has no unit on the server; nothing to change", nil
	}
	if !suspend {
		// A security hold keeps the app stopped; the owner starts it after
		// the hold is lifted.
		var held int
		if err := p.db.GetDB().QueryRowContext(ctx, `
			SELECT COALESCE((SELECT o.held FROM service_app_offerings o JOIN service_app_installs i ON i.app_id = o.app_id WHERE i.id = ?), 0)`,
			e.Ref.InstallID).Scan(&held); err != nil {
			return "", "", unknownResult(err)
		}
		if held == 1 {
			return effectDone, "kept stopped: the administrator holds this app for security; it is started again after the hold is lifted", nil
		}
	}
	resp, err := native.setSuspension(ctx, transport.SetServiceAppSuspensionRequest{InstallID: e.Ref.InstallID, Suspend: suspend, Before: before})
	if err != nil {
		return "", "", unknownResult(err)
	}
	if suspend {
		return applyResult(resp.SuspensionApplyResult, "app stopped and kept from starting at boot", "already stopped")
	}
	if !before.Active {
		return applyResult(resp.SuspensionApplyResult, "boot setting put back; the app was not running before, so it was not started", "already as it was")
	}
	return applyResult(resp.SuspensionApplyResult, fmt.Sprintf("app on %s started again", e.Ref.Domain), "already running")
}
