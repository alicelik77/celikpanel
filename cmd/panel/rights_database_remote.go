package main

// Remote access to databases (alpha.129, docs/DATABASE-REMOTE-ACCESS.md). It
// joins the closed permission set as an opt-in permission, like the
// additional nginx directives: closed for everyone, Unlimited plans included,
// until the administrator offers it on the server (admin_offered_permissions).
// Unlike them, withdrawing the offer closes what is open: the server switch
// of remote access is this offer (database_remote.go).
//
// Veritabanlarına dışarıdan erişim izni: yönetici sunmadıkça herkese kapalı;
// sunumu geri çekmek açık olanı kapatır.

const rightDatabaseRemote = "database_remote"

func init() {
	if knownPermission(rightDatabaseRemote) {
		return
	}
	rightPermissionIDs = append(rightPermissionIDs, rightDatabaseRemote)
	rightPermissionNames[rightDatabaseRemote] = "Remote access to databases"
	optInPermissions[rightDatabaseRemote] = true
}
