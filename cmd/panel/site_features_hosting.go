package main

import (
	"context"
	"database/sql"
	"errors"
)

// refuseProjectTypeWithSiteFeatures is the answer to a change of a site's
// project type that would leave a protected folder or a redirect without a
// place to live (D-024: an unmet prerequisite with the person's next step). It
// returns "" when the change may go on.
func (p *Panel) refuseProjectTypeWithSiteFeatures(ctx context.Context, domainID int, projectType string) (string, error) {
	site, err := p.siteFeatureSite(ctx, domainID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	state, err := p.loadSiteFeatureState(ctx, site.SiteID)
	if err != nil {
		return "", err
	}
	if len(state.Dirs) > 0 && projectType != "php" && projectType != "static" {
		return "This site has password-protected folders, and only PHP and static sites can have them. " +
			"Nothing was changed. Remove the folders' protection under Protected folders, then change the site type.", nil
	}
	if len(state.Redirects) > 0 && projectType == "forwarding" {
		return "This site has redirects of its own, and a forwarding site sends every address to its target. " +
			"Nothing was changed. Remove the redirects under Redirects, then change the site type.", nil
	}
	return "", nil
}
