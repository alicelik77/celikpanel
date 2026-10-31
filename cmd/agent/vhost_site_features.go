package main

import (
	"fmt"
	"log"

	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// siteFeaturesFromRequest turns the request's protected folders and redirects
// into the generator's input, refusing any user, hash or count that could not
// be written safely. The generator validates the folders and redirects again
// when it renders (services.PrepareSiteFeatures).
func siteFeaturesFromRequest(req *ApplyVhostRequest) ([]services.SiteProtectedDir, []services.SiteRedirect, error) {
	var dirs []services.SiteProtectedDir
	if len(req.ProtectedDirs) > services.MaxSiteProtectedDirs {
		return nil, nil, fmt.Errorf("too many protected folders")
	}
	for _, dir := range req.ProtectedDirs {
		if len(dir.Users) == 0 {
			return nil, nil, fmt.Errorf("protected folder %q has no user who may open it", dir.Path)
		}
		if len(dir.Users) > services.MaxSiteProtectedUsers {
			return nil, nil, fmt.Errorf("protected folder %q has too many users", dir.Path)
		}
		// The password file is rendered here once, so a bad user name or hash
		// is refused before any file is touched.
		if _, err := services.RenderSiteAuthFile("validation.invalid", services.SiteAuthFile{ID: dir.ID, Path: dir.Path, Users: siteAuthUsers(dir.Users)}); err != nil {
			return nil, nil, fmt.Errorf("protected folder %q: %w", dir.Path, err)
		}
		dirs = append(dirs, services.SiteProtectedDir{ID: dir.ID, Path: dir.Path, Realm: dir.Realm})
	}
	if len(req.Redirects) > services.MaxSiteRedirects {
		return nil, nil, fmt.Errorf("too many redirects")
	}
	var redirects []services.SiteRedirect
	for _, redirect := range req.Redirects {
		redirects = append(redirects, services.SiteRedirect{
			Match: redirect.Match, Source: redirect.Source, Target: redirect.Target,
			Code: redirect.Code, KeepQuery: redirect.KeepQuery,
		})
	}
	return dirs, redirects, nil
}

func siteAuthUsers(users []transport.SiteProtectedUserRecord) []services.SiteAuthUser {
	out := make([]services.SiteAuthUser, 0, len(users))
	for _, user := range users {
		out = append(out, services.SiteAuthUser{Username: user.Username, Hash: user.PasswordHash})
	}
	return out
}

// siteAuthFilesFor is the password files a request asks for (none for a site
// without protected folders, which removes the ones it had).
func siteAuthFilesFor(req *ApplyVhostRequest) []services.SiteAuthFile {
	files := make([]services.SiteAuthFile, 0, len(req.ProtectedDirs))
	for _, dir := range req.ProtectedDirs {
		files = append(files, services.SiteAuthFile{ID: dir.ID, Path: dir.Path, Users: siteAuthUsers(dir.Users)})
	}
	return files
}

// finishSiteAuthChange ends the password-file change of one vhost apply: kept
// when the vhost was applied, put back when it was not.
func finishSiteAuthChange(change *services.SiteAuthChange, applied bool) {
	if change == nil {
		return
	}
	if applied {
		change.Commit()
		return
	}
	if err := change.Rollback(); err != nil {
		log.Printf("put back the password files after a vhost that was not applied: %v", err)
	}
}
