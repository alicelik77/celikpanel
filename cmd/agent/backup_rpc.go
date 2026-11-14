package main

import (
	"errors"
	"os"
	"time"

	"github.com/alicelik/celikpanel/internal/backupspec"
	"github.com/alicelik/celikpanel/internal/hostingpath"
)

var backupBaseDir = func() string {
	if dir := os.Getenv("CELIKPANEL_BACKUP_DIR"); dir != "" {
		return dir
	}
	return "/var/backups/celikpanel"
}()

var backupDocumentRoot = hostingpath.DocumentRoot

type backupScope struct {
	ProtocolVersion int
	SubscriptionID  int
	DomainID        int
	DomainName      string
}

func createScope(r *backupspec.CreateRequest) backupScope {
	return backupScope{r.ProtocolVersion, r.SubscriptionID, r.DomainID, r.DomainName}
}

func listScope(r *backupspec.ListRequest) backupScope {
	return backupScope{r.ProtocolVersion, r.SubscriptionID, r.DomainID, r.DomainName}
}

func inspectScope(r *backupspec.InspectRequest) backupScope {
	return backupScope{r.ProtocolVersion, r.SubscriptionID, r.DomainID, r.DomainName}
}

func restoreScope(r *backupspec.RestoreRequest) backupScope {
	return backupScope{r.ProtocolVersion, r.SubscriptionID, r.DomainID, r.DomainName}
}

func deleteScope(r *backupspec.DeleteRequest) backupScope {
	return backupScope{r.ProtocolVersion, r.SubscriptionID, r.DomainID, r.DomainName}
}

func readScope(r *backupspec.ReadChunkRequest) backupScope {
	return backupScope{r.ProtocolVersion, r.SubscriptionID, r.DomainID, r.DomainName}
}

func validateV2Scope(s backupScope) error {
	return validateScopeIDs(s.ProtocolVersion, s.SubscriptionID, s.DomainID)
}

func (a *Agent) CreateBackup(req *backupspec.CreateRequest, resp *backupspec.CreateResponse) error {
	info, err := a.createBackup(req)
	if err != nil {
		resp.Error = err.Error()
		resp.Refusal = backupClusterRefusalOf(err)
		return nil
	}
	resp.Success = true
	resp.Backup = info
	return nil
}

func (a *Agent) InspectBackup(req *backupspec.InspectRequest, resp *backupspec.InspectResponse) error {
	info, databases, err := inspectBackup(inspectScope(req), req.BackupName)
	if err != nil {
		resp.Error = err.Error()
		resp.Code, _ = restoreFailureOf(err)
		return nil
	}
	resp.Success = true
	resp.Backup = info
	resp.Databases = databases
	return nil
}

// backupContentsTimeLimit bounds the measurement below the Panel's 60-second
// read timeout; past it the sizes are "at least".
const backupContentsTimeLimit = 45 * time.Second

// BackupContentsV1 says what a files backup of the site would hold now
// (docs/SITE-BACKUP-CONTENTS-DESIGN.md), measured with the archive's own walk
// and rules. Reads only.
// BackupContentsV1, sitenin dosya yedeğinin şu an neleri içereceğini söyler.
func (a *Agent) BackupContentsV1(req *backupspec.ListRequest, resp *backupspec.ContentsResponse) error {
	scope := listScope(req)
	if err := validateV2Scope(scope); err != nil {
		return err
	}
	docroot, err := backupDocumentRoot(scope.SubscriptionID, scope.DomainID)
	if err != nil {
		return err
	}
	home, err := backupSiteHome(scope.SubscriptionID, scope.DomainID)
	if err != nil {
		return err
	}
	*resp = measureSiteBackup(docroot, home, backupContentsTimeLimit)
	return nil
}

func (a *Agent) RestoreBackup(req *backupspec.RestoreRequest, resp *backupspec.RestoreResponse) error {
	if err := a.restoreBackup(req, resp); err != nil {
		resp.Error = err.Error()
		resp.Success = false
		resp.Refusal = backupClusterRefusalOf(err)
		resp.Code, resp.Detail = restoreFailureOf(err)
	}
	return nil
}

func (a *Agent) DeleteBackup(req *backupspec.DeleteRequest, resp *bool) error {
	filePath, _, err := resolveBackup(deleteScope(req), req.BackupName)
	if err != nil {
		return err
	}
	if err := secureRemoveRegular(filePath); err != nil {
		return err
	}
	*resp = true
	return nil
}

var errInvalidBackupName = errors.New("invalid backup name")

func invalidBackupName() error {
	return errInvalidBackupName
}
