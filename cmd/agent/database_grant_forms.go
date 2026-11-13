package main

import (
	"errors"
	"log"
	"sort"

	"github.com/alicelik/celikpanel/internal/mariadbgrant"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Rights by exact name on this machine's MariaDB (alpha.134,
// docs/DATABASE-USERS.md). The panel proves from its records which
// pattern-form grants are CelikPanel's and asks for them one at a time; the
// Agent only reads (DatabaseGrantFormsScanV1) and rewrites the one row it is
// given (DatabaseGrantFormConvertV1: the exact form first, the pattern form
// taken away after it). It never decides on its own which rows are whose.
//
// Bu makinedeki MariaDB'de hakların tam ada çevrilmesi: Ajan okur ve yalnız
// kendisine verilen satırı çevirir.

// databaseGrantFormFault is a test seam (mariadbgrant.Fault).
var databaseGrantFormFault mariadbgrant.Fault

// DatabaseGrantFormsScanV1 reads the engine's pattern-form grant rows. Read
// only.
func (a *Agent) DatabaseGrantFormsScanV1(req *transport.DatabaseGrantFormsScanRequest, resp *transport.DatabaseGrantFormsScanResponse) error {
	if req == nil || resp == nil {
		return errors.New("grant forms scan request and response are required")
	}
	*resp = transport.DatabaseGrantFormsScanResponse{}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "reading the database rights"); err != nil {
		resp.Error, resp.BuildMismatch = err.Error(), true
		return nil
	}
	scan, err := mariadbgrant.ScanEngine(mysqlGrantExec("the database server"))
	if err != nil {
		resp.Error = err.Error()
		return nil
	}
	resp.Scan = scan
	resp.RemoteHosts = map[string][]string{}
	for _, row := range scan.Patterns {
		if _, seen := resp.RemoteHosts[row.User]; seen {
			continue
		}
		hosts, err := mysqlAccessUserOwnHosts(row.User)
		if err != nil {
			// Unreadable: only localhost is proven; the copies wait.
			log.Printf("database rights: the remote access record could not be read: %v", err)
		}
		list := []string{}
		for host := range hosts {
			if host != "localhost" {
				list = append(list, host)
			}
		}
		sort.Strings(list)
		resp.RemoteHosts[row.User] = list
	}
	return nil
}

// DatabaseGrantFormConvertV1 rewrites the one captured row it is given.
func (a *Agent) DatabaseGrantFormConvertV1(req *transport.DatabaseGrantFormConvertRequest, resp *transport.DatabaseGrantFormConvertResponse) error {
	if req == nil || resp == nil {
		return errors.New("grant form request and response are required")
	}
	*resp = transport.DatabaseGrantFormConvertResponse{}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "rewriting a database right"); err != nil {
		resp.Error, resp.BuildMismatch = err.Error(), true
		return nil
	}
	if !mariadbgrant.ValidName(req.Database) {
		resp.Error = "invalid database name; nothing was changed"
		return nil
	}
	suspensionDatabaseMutex.Lock()
	defer suspensionDatabaseMutex.Unlock()
	outcome, err := mariadbgrant.Convert(mysqlGrantExec("the database server"), req.ConvertRequest, databaseGrantFormFault)
	if err != nil {
		resp.Error = err.Error()
		return nil
	}
	resp.Outcome = outcome
	return nil
}
