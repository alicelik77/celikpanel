package main

import (
	"log"

	"github.com/alicelik/celikpanel/internal/transport"
)

// A site's own PHP values and its nginx settings file (alpha.128,
// docs/SITE-LIMITS.md). The values are written by Agent.ApplySitePool
// (operation "settings"); these two methods read them for the site screen
// and the plan preview, and write the nginx file. Both are bound to the
// Panel's build.
//
// Sitenin kendi PHP değerleri ve nginx ayar dosyası; okuma ve yazma.

// maxSitePHPSettingsSites bounds one read (the plan preview reads the sites
// of every subscription on a plan in one call).
const maxSitePHPSettingsSites = 2000

// SitePHPSettingsV1 reads, for each site, the pool's own values and state,
// the version's php.ini and which file sets the nginx upload size and read
// timeout. It writes nothing.
// SitePHPSettingsV1 her site için havuzdaki değerleri okur; yazmaz.
func (a *Agent) SitePHPSettingsV1(req *transport.SitePHPSettingsRequest, resp *transport.SitePHPSettingsResponse) error {
	if req == nil {
		resp.Error = "settings request is required"
		return nil
	}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "reading site PHP settings"); err != nil {
		resp.Error = err.Error()
		return nil
	}
	if len(req.Sites) > maxSitePHPSettingsSites {
		resp.Error = "too many sites in one read"
		return nil
	}
	manager := a.sitePoolManager()
	resp.Sites = make([]transport.SitePHPSettingsState, 0, len(req.Sites))
	for _, site := range req.Sites {
		resp.Sites = append(resp.Sites, manager.ReadSitePHPSettings(site))
	}
	return nil
}

// ApplySiteSettingsIncludeV1 writes, keeps or removes the site's
// site-settings.conf (client_max_body_size, fastcgi_read_timeout) with an
// nginx check and put-back. A kept or refused file is an answer, not an
// error.
// ApplySiteSettingsIncludeV1 sitenin nginx ayar dosyasını denetimle yazar.
func (a *Agent) ApplySiteSettingsIncludeV1(req *transport.SiteSettingsIncludeRequest, resp *transport.SiteSettingsIncludeResponse) error {
	if req == nil {
		resp.Outcome, resp.Error = transport.SiteSettingsIncludeRefused, "settings request is required"
		return nil
	}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "writing a site settings file"); err != nil {
		resp.Outcome, resp.Error = transport.SiteSettingsIncludeRefused, err.Error()
		return nil
	}
	if a == nil || a.nginxGen == nil {
		resp.Outcome, resp.Error = transport.SiteSettingsIncludeRefused, "nginx is not available on this agent"
		return nil
	}
	*resp = a.nginxGen.ApplySiteSettingsInclude(*req)
	log.Printf("ApplySiteSettingsIncludeV1 %s: %s %s", req.Domain, resp.Outcome, resp.Detail)
	return nil
}
