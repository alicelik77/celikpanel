package main

import "github.com/alicelik/celikpanel/internal/backupspec"

// s1WithCredentials gives a restore the database's own account (S1). The
// engine tests of the lane call it so that the same test runs against the
// code before the lane, where a restore had no account (see the lane's
// report for that run).
func s1WithCredentials(req *backupspec.RestoreRequest, id int, user, password string) {
	req.Credentials = append(req.Credentials, backupspec.DatabaseCredential{ID: id, User: user, Password: password})
}
