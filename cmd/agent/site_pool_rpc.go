package main

import (
	"log"

	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// A hosted site's PHP-FPM pool file under D-031 (second step, 2026-10-11):
// read with its state and difference, and written only by the render rule
// (services/site_pool_file.go). Both methods are bound to the Panel's build,
// as the vhost's are.
//
// Barındırılan sitenin PHP-FPM havuz dosyası D-031 kuralıyla okunur ve yazılır.

func (a *Agent) sitePoolManager() *services.PHPPoolManager {
	if a != nil && a.phpManager != nil && a.phpManager.PoolManager != nil {
		return a.phpManager.PoolManager
	}
	return services.NewPHPPoolManager()
}

// InspectSitePool classifies a site's pool and returns the difference from
// CelikPanel's text. It writes nothing.
// InspectSitePool sitenin havuzunu sınıflandırır ve farkı döndürür; yazmaz.
func (a *Agent) InspectSitePool(req *transport.SitePoolRequest, resp *transport.InspectSiteFileResponse) error {
	if req == nil {
		resp.Error = "pool request is required"
		return nil
	}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "inspecting a PHP-FPM pool"); err != nil {
		resp.Error = err.Error()
		return nil
	}
	file, diff, truncated, err := a.sitePoolManager().InspectSitePool(*req)
	if err != nil {
		resp.Error = err.Error()
		return nil
	}
	resp.File, resp.Diff, resp.DiffTruncated = file, diff, truncated
	return nil
}

// ApplySitePool is a save, a PHP switch, or the site configuration page's
// take, recreate or apply on a site's pool. A file the rule keeps is an answer
// (File), never an error.
// ApplySitePool havuzu kaydeder, PHP sürümünü değiştirir ya da sayfanın
// seçimini uygular; korunan dosya bir yanıttır, hata değil.
func (a *Agent) ApplySitePool(req *transport.SitePoolRequest, resp *transport.ApplySiteFileResponse) error {
	if req == nil {
		resp.Error = "pool request is required"
		return nil
	}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "changing a PHP-FPM pool"); err != nil {
		resp.Error = err.Error()
		return nil
	}
	file, err := a.sitePoolManager().ApplySitePool(*req)
	if file.Path != "" {
		resp.File = &file
		log.Printf("ApplySitePool site %d (%s): %s %s %s", req.SiteID, req.Operation, file.Outcome, file.State, file.Path)
	}
	if err != nil {
		resp.Error = err.Error()
	}
	return nil
}
