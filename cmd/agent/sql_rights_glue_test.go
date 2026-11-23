package main

import (
	"github.com/alicelik/celikpanel/internal/backupspec"
	"github.com/alicelik/celikpanel/internal/transport"
)

// s1WithCredentials gives a restore the database's own account (S1). The
// engine tests of the lane call it so that the same tests also run against
// the code before the lane, where a restore had no account.
func s1WithCredentials(req *backupspec.RestoreRequest, id int, user, password string) {
	req.Credentials = append(req.Credentials, backupspec.DatabaseCredential{ID: id, User: user, Password: password})
}

// s1CopyRequest is a database copy with the new database's own account.
func s1CopyRequest(source transport.DBToolsTarget, target backupspec.DatabaseIdentity, user, password string) transport.DBToolsCopyRequest {
	return transport.DBToolsCopyRequest{Source: source, Target: target, TargetUser: user, TargetPassword: password}
}
