package main

// The remote API permission (alpha.128, G15; D-042, docs/REMOTE-API.md). It
// joins the closed permission set like the site tools of alpha.118
// (rights_site_tools.go). Like the additional nginx directives it is usable
// only once the administrator turned the remote API on for the server
// (panel setting remote_api_enabled, off by default): an "all permissions"
// set (the Unlimited plans) holds the permission, but no token works while
// the server switch is off, and a reseller or customer can then still be
// refused by the reseller's rights or the plan.
//
// Uzak API izni: yönetici sunucuda açmadıkça hiçbir jeton çalışmaz.

const rightRemoteAPI = "remote_api"

func init() {
	if knownPermission(rightRemoteAPI) {
		return
	}
	rightPermissionIDs = append(rightPermissionIDs, rightRemoteAPI)
	rightPermissionNames[rightRemoteAPI] = "Remote API"
}
