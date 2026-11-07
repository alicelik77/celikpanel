package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// The site's web server settings in a vhost render (alpha.128, PLESK-GAP G3,
// G19; transport/site_web_settings.go). The Agent is the trust boundary
// again: the settings are validated here before any template field or file
// is made from them, and a reseller's or customer's directives go through
// the same forbidden list the Panel checked.
//
// Web sunucusu ayarları vhost üretiminde: Agent alanları yeniden doğrular;
// bayi ve müşteri yönergeleri aynı yasak listesinden geçer.

// siteWebSettingsOf validates the request's settings for the project type
// ("" settings: the site never saved one).
func siteWebSettingsOf(req *ApplyVhostRequest, projectType string) (services.SiteWebSettings, bool, error) {
	if req == nil || req.WebSettings == nil {
		return services.SiteWebSettings{}, false, nil
	}
	settings, err := services.ApplicableSiteWebSettings(projectType, req.WebSettings.Settings)
	if err != nil {
		return services.SiteWebSettings{}, false, fmt.Errorf("web server settings: %w", err)
	}
	// G26: an error page whose file is not there is left out of a render
	// (a save refuses it first: missingErrorPages).
	settings.ErrorPages, _ = presentErrorPages(req.DocumentRoot, settings.ErrorPages)
	return settings, true, nil
}

// presentErrorPages splits the pages into those whose file is a regular
// file in the document root (never a link) and the missing ones.
func presentErrorPages(documentRoot string, pages []transport.SiteErrorPage) (present []transport.SiteErrorPage, missing []string) {
	for _, page := range pages {
		info, err := errorPageLstat(services.SiteErrorPageFile(documentRoot, page.Path))
		if err == nil && info.Mode().IsRegular() {
			present = append(present, page)
		} else {
			missing = append(missing, page.Path)
		}
	}
	return present, missing
}

// errorPageLstat is a test seam.
var errorPageLstat = os.Lstat

// errorPageMissingMarker starts the error of a save naming a page whose file
// is not in the document root (an unmet prerequisite; nothing written).
const errorPageMissingMarker = "ERROR_PAGE_MISSING: "

// applySiteWebSettingsToVhost sets the template fields of the settings.
func applySiteWebSettingsToVhost(req *ApplyVhostRequest, data *services.VhostData) error {
	settings, has, err := siteWebSettingsOf(req, data.ProjectType)
	if err != nil || !has {
		return err
	}
	_, anyField, _ := services.SiteWebFieldsApply(data.ProjectType)
	data.SiteInclude = anyField
	if len(settings.IndexFiles) > 0 {
		data.IndexFiles = strings.Join(settings.IndexFiles, " ")
	}
	if settings.NodePublic && data.ProjectType == "node" {
		public := strings.TrimSuffix(data.DocumentRoot, "/") + "/public"
		// Served only while it is a real folder (never a link out of the
		// site): a site without one keeps sending everything to the app.
		if info, statErr := nodePublicLstat(public); statErr == nil && info.IsDir() {
			data.NodePublicRoot = public
		}
	}
	return nil
}

// nodePublicLstat is a test seam.
var nodePublicLstat = os.Lstat

// siteWebPartsFor is the generator's list of the site's files for a render;
// nil when the site never saved a web server setting.
func siteWebPartsFor(req *ApplyVhostRequest, data services.VhostData) []services.SitePart {
	settings, has, err := siteWebSettingsOf(req, data.ProjectType)
	if err != nil || !has {
		return nil
	}
	if _, anyField, _ := services.SiteWebFieldsApply(data.ProjectType); !anyField {
		return nil
	}
	directives := req.WebSettings.Directives
	parts := services.SitePartsForSite(data.Domain, data.SiteID, data.ProjectType, settings, directives)
	if !req.WebSettings.DirectivesTrusted && strings.TrimSpace(directives) != "" {
		if err := services.ValidateSiteDirectives(directives, false, services.SiteHomeOf(data.DocumentRoot)); err != nil {
			// The Panel checked the same rules; a text that fails here is
			// left as it is on disk and never written.
			log.Printf("site %s: additional nginx directives not written: %v", data.Domain, err)
			kept := parts[:0]
			for _, part := range parts {
				if part.Name != services.SiteDirectivesPartName {
					kept = append(kept, part)
				}
			}
			parts = kept
		}
	}
	return parts
}

// nginxFailingBeforeMarker starts the error of a save whose server already
// failed `nginx -t` before anything was written (an unmet prerequisite).
const nginxFailingBeforeMarker = "NGINX_FAILING_BEFORE: "

// precheckSiteWebSettings runs `nginx -t` before a save of the settings
// writes anything ("" when it passes or was not asked for).
func (a *Agent) precheckSiteWebSettings(req *ApplyVhostRequest) string {
	if req == nil || req.WebSettings == nil || !req.WebSettings.PrecheckNginx || a.nginxGen == nil {
		return ""
	}
	if _, missing := presentErrorPages(req.DocumentRoot, req.WebSettings.Settings.ErrorPages); len(missing) > 0 {
		return errorPageMissingMarker + strings.Join(missing, ", ")
	}
	if err := a.nginxGen.ValidateNginx(); err != nil {
		detail := err.Error()
		var refused *services.NginxConfigRefusedError
		if errors.As(err, &refused) {
			detail = refused.FirstLine()
		}
		return nginxFailingBeforeMarker + detail
	}
	return ""
}
