package main

// The offer of database_remote is the server switch of remote access to
// databases (alpha.129). alpha.130: it can be written from two screens (the
// Offered tab of plans and rights, and the administrator's Databases page);
// both write admin_offered_permissions through saveOfferedOptInPermissions,
// which calls this after every save, so a change from either place asks the
// sync loop for an apply (opening or closing what the switch decides) and
// both screens read the same stored value.
//
// Uzak veritabanı erişiminin sunucu anahtarı iki ekrandan yazılabilir; hangisi
// yazarsa yazsın değişiklik sunucuya ulaşır.

// databaseRemoteOfferMaybeChanged kicks the sync when the switch moved. An
// unreadable "before" counts as a change (an apply that changes nothing is
// harmless; a missed one leaves access open).
func databaseRemoteOfferMaybeChanged(before, after map[string]bool) {
	if before == nil || before[rightDatabaseRemote] != after[rightDatabaseRemote] {
		kickDatabaseRemoteSync()
	}
}
