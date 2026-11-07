package main

// The permission of the additional nginx directives (alpha.128, owner
// decision 4 of 2026-11-07; SITE-SETTINGS-DESIGN §2.2). It joins the closed
// permission set like the site tools of alpha.118 (rights_site_tools.go).
// Unlike the other permissions it is usable only when the administrator
// offered it first (panel setting nginx_directives_offered, off by default):
// an "all permissions" set (the Unlimited plans) holds it, but the gate in
// site_web_settings.go also asks for the offer.
//
// The "limits" workstream of alpha.128 may register the same id; whichever
// file runs first registers it, the other leaves it (no duplicate entry).
//
// Ek nginx yönergeleri izni; yalnız yönetici sunduktan sonra kullanılır.

const rightNginxDirectivesWeb = "nginx_directives"

func init() {
	if knownPermission(rightNginxDirectivesWeb) {
		return
	}
	rightPermissionIDs = append(rightPermissionIDs, rightNginxDirectivesWeb)
	rightPermissionNames[rightNginxDirectivesWeb] = "Additional nginx directives"
}
