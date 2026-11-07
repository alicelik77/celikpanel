package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// The page cache's "Clear cache" (alpha.128, PLESK-GAP G28).
//
//	POST /api/v1/domains/{id}/web-settings/cache-clear
//
// The Agent empties the site's cache folder; nginx keeps serving and fills it
// again from the application. Steps: check → clear. Turning the cache on or
// off, and how long a page is kept, are the administrator's (site_web_settings.go).
//
// Sayfa önbelleğini temizle: Agent sitenin önbellek klasörünü boşaltır.

var requestIdentityRoutePageCacheClear = requestIdentityRoute{
	pattern: "/api/v1/domains/{id}/web-settings/cache-clear", optional: true,
	timeout: agentRPCMutationTimeout + time.Minute,
}

// sitePageCacheClearCall is the Agent call (a test seam).
var sitePageCacheClearCall = func(p *Panel, ctx context.Context, siteID int) (transport.SitePageCacheClearResponse, error) {
	var resp transport.SitePageCacheClearResponse
	err := p.callAgentContext(ctx, "Agent.SitePageCacheClearV1", &transport.SitePageCacheClearRequest{
		ExpectedBuildCommit: strings.TrimSpace(buildCommit), SiteID: siteID,
	}, &resp)
	return resp, err
}

func (p *Panel) handleDomainPageCacheClear(w http.ResponseWriter, r *http.Request, domainID int) {
	ctx := r.Context()
	reportOperationPlan(ctx, "check", "clear")
	reportOperationStep(ctx, "check")
	site, err := p.siteFeatureSite(ctx, domainID)
	if errors.Is(err, sql.ErrNoRows) {
		writeClientError(w, http.StatusNotFound, "domain site not found")
		return
	}
	if err != nil {
		writeServerError(w, err)
		return
	}
	row, err := loadSiteWebSettings(ctx, p.db.GetDB(), site.SiteID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if !row.Settings.PageCache || !services.PageCacheApplies(site.ProjectType) {
		writeCodedError(w, http.StatusConflict, errCodeWebPageCacheOff,
			"This site has no page cache, so there is nothing to clear. The administrator turns it on under Hosting → Web server.", "")
		return
	}
	reportOperationStep(ctx, "clear")
	opCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), agentRPCMutationTimeout)
	defer cancel()
	resp, err := sitePageCacheClearCall(p, opCtx, site.SiteID)
	if err != nil {
		writeCodedError(w, http.StatusBadGateway, errCodeWebPageCacheClear,
			"The server did not answer, so it is not known whether the cache was cleared. Clearing again is safe.", "")
		return
	}
	if resp.Error != "" {
		writeCodedError(w, http.StatusConflict, errCodeWebPageCacheClear, "The cache was not fully cleared: "+resp.Error+". Clearing again is safe.", "")
		return
	}
	p.auditDetailed(r, "domain.web.cache_clear", "domain", domainID, map[string]any{"removed": resp.Removed})
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"cleared": true, "removed": resp.Removed})
}
