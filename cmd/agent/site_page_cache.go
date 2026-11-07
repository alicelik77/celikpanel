package main

import (
	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// SitePageCacheClearV1 empties one site's page cache folder (alpha.128,
// PLESK-GAP G28; services.ClearPageCache). nginx keeps serving and fills the
// cache again; clearing twice is harmless.
//
// Sitenin sayfa önbelleği klasörünü boşaltır.
func (a *Agent) SitePageCacheClearV1(req *transport.SitePageCacheClearRequest, resp *transport.SitePageCacheClearResponse) error {
	if req == nil {
		resp.Error = "a request is required"
		return nil
	}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "clearing a site's page cache"); err != nil {
		resp.Error = err.Error()
		return nil
	}
	removed, err := services.ClearPageCache(req.SiteID)
	resp.Removed = removed
	if err != nil {
		resp.Error = err.Error()
	}
	return nil
}
