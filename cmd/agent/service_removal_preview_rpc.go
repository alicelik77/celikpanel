package main

import (
	"context"
	"errors"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

// ServiceRemovalPreviewV1 answers, read-only, what removing a component (or
// one version of it) would take on this server: the package manager's own
// simulation of the exact removal Agent.UninstallService would run, what it
// would remove that the removal did not name (which refuses the removal),
// and the PHP tools served from the PHP being removed (alpha.104). It starts
// no mutation: it takes no service-mutation step, never waits for the
// package lock (another package operation → "busy") and is bounded.
//
// ServiceRemovalPreviewV1, bir bileşeni (ya da tek sürümünü) kaldırmanın bu
// sunucuda neyi alacağını salt-okur yanıtlar: paket yöneticisinin benzetimi,
// adı söylenmeden alınacaklar ve kaldırılan PHP'de koşan araçlar. Değişiklik
// başlatmaz, paket kilidini beklemez ve sınırlıdır.

const serviceRemovalPreviewBound = 25 * time.Second

func (a *Agent) ServiceRemovalPreviewV1(req *transport.ServiceRemovalPreviewRequest, resp *transport.ServiceRemovalPreviewResponse) error {
	*resp = emptyServiceRemovalPreview()
	if req == nil {
		resp.Error = "missing request"
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), serviceRemovalPreviewBound)
	defer cancel()
	ops, err := defaultServiceUninstallOps(ctx)
	if err != nil {
		resp.Error = "detect host platform: " + err.Error()
		return nil
	}
	// Read-only: the preview never stops a unit or removes a package.
	ops.disableUnit = nil
	ops.removePackages = nil
	serviceRemovalPreviewWithOps(ctx, req, resp, ops, hostPackageRemovalExecutor)
	return nil
}

func emptyServiceRemovalPreview() transport.ServiceRemovalPreviewResponse {
	return transport.ServiceRemovalPreviewResponse{
		State:      "failed",
		Requested:  []string{},
		Remove:     []string{},
		Collateral: []transport.PackageRemovalCollateral{},
		BoundTools: []string{},
	}
}

func serviceRemovalPreviewWithOps(
	ctx context.Context,
	req *transport.ServiceRemovalPreviewRequest,
	resp *transport.ServiceRemovalPreviewResponse,
	ops serviceUninstallOps,
	executorFor func(string) (packageRemovalExecutor, error),
) {
	svc := core.GetManagedServiceByID(req.ID)
	if svc == nil {
		resp.Error = "unknown service"
		return
	}
	if refusal := genericDNSEngineMutationRefusal(svc); refusal != "" {
		resp.State, resp.Error = "unsupported", refusal
		return
	}
	if refusal := nativeCronRemovalRefusal(svc); refusal != "" {
		resp.State, resp.Error = "unsupported", refusal
		return
	}
	if svc.ID == "roundcube" {
		// Webmail is a tree, not a package: nothing to simulate.
		resp.State = "unsupported"
		return
	}
	work, refusal := planServiceRemovalWork(svc, req.Package, ops)
	resp.PackageFamily = work.family
	if refusal != "" {
		resp.State, resp.Error = "unsupported", refusal
		return
	}
	if work.notInstalled != "" {
		resp.State = "not_installed"
		return
	}
	resp.Requested = append([]string{}, work.request.Packages...)
	if work.request.KeepData {
		resp.KeepsData = true
		resp.DataPaths = removalDataPaths(svc, work.family, req.Package)
	}
	if work.boundTools != nil {
		resp.BoundTools = append([]string{}, work.boundTools...)
	}
	executor, err := executorFor(work.family)
	if err != nil {
		resp.Error = err.Error()
		if errors.Is(err, errDNFMutationCertificationPending) {
			resp.State = "unsupported"
		}
		return
	}
	if !packageOperationMu.TryLock() {
		resp.State = "busy"
		return
	}
	plan, err := executor.plan(ctx, work.request)
	packageOperationMu.Unlock()
	if err != nil {
		var refusal *packageRemovalRefusal
		if errors.As(err, &refusal) && refusal.Code == errCodeRemovalPlanChanged {
			resp.State = "busy"
			return
		}
		resp.Error = err.Error()
		return
	}
	resp.State = "checked"
	resp.Remove = append([]string{}, plan.Remove...)
	collateral := plan.Collateral
	if len(collateral) > packageRemovalCollateralCap {
		collateral = collateral[:packageRemovalCollateralCap]
	}
	resp.Collateral = append([]transport.PackageRemovalCollateral{}, collateral...)
}
