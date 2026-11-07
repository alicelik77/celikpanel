package main

import (
	"os"
	"path/filepath"

	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// SiteCopyV1 copies one site's web folder, and optionally one MariaDB
// database, into another site (alpha.128, PLESK-GAP G8). It uses the backup
// engine's own safe parts: the source folder is archived without following a
// link, the copy is unpacked into a stage beside the target owned by the
// target's account, and published with one rename (the previous folder kept
// until the rename succeeded). The source is only read. A target with content
// is refused unless the Panel says it made the target's backup first
// (Overwrite). The target is checked again right before publishing.
//
// Bir sitenin web klasörünü ve isteğe bağlı bir MariaDB veritabanını başka bir
// siteye kopyalar; kaynak yalnız okunur.
func (a *Agent) SiteCopyV1(req *transport.SiteCopyRequest, resp *transport.SiteCopyResponse) error {
	if req == nil {
		resp.Error = "a request is required"
		return nil
	}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "copying a website"); err != nil {
		resp.Error = err.Error()
		return nil
	}
	resp.CompensationSafe = true
	if !validWordPressOperationID(req.OperationID) {
		resp.Error = "an operation identity is required"
		return nil
	}
	sourceRoot, err := validWordPressSite(req.Source)
	if err != nil {
		resp.Error = "source: " + err.Error()
		return nil
	}
	targetRoot, err := validWordPressSite(req.Target)
	if err != nil {
		resp.Error = "target: " + err.Error()
		return nil
	}
	if req.Source.DomainID == req.Target.DomainID || req.Source.SiteID == req.Target.SiteID {
		resp.Error = "a site is copied to another site"
		return nil
	}
	withDatabase := req.SourceDatabase != "" || req.TargetDatabase != ""
	if withDatabase {
		for _, name := range []string{req.SourceDatabase, req.TargetDatabase} {
			if err := services.ValidateSQLIdentifier(name); err != nil {
				resp.Error = "invalid database identity"
				return nil
			}
		}
		if req.SourceDatabase == req.TargetDatabase {
			resp.Error = "the copy's database must be a new one"
			return nil
		}
	}
	first, second := siteMutationMutex(req.Source.SiteID), siteMutationMutex(req.Target.SiteID)
	first.Lock()
	defer first.Unlock()
	if second != first {
		second.Lock()
		defer second.Unlock()
	}
	empty := func() bool {
		_, err := inspectInstallableDocumentRoot(targetRoot, req.Target.Domain)
		return err == nil
	}
	if !req.Overwrite && !empty() {
		resp.Code = transport.SiteCopyTargetNotEmpty
		resp.Error = "the target site's web folder already has content; nothing was changed"
		return nil
	}

	if err := secureMkdirAll(backupBaseDir, 0o700); err != nil {
		resp.Error = "prepare the copy folder: " + err.Error()
		return nil
	}
	work, err := os.MkdirTemp(backupBaseDir, ".site-copy-")
	if err != nil {
		resp.Error = "prepare the copy folder: " + err.Error()
		return nil
	}
	defer os.RemoveAll(work)
	if err := os.Chmod(work, 0o700); err != nil {
		resp.Error = err.Error()
		return nil
	}
	archive := filepath.Join(work, "files.tar.gz")
	if err := safeCreateFilesArchive(sourceRoot, archive); err != nil {
		resp.Error = "read the source files: " + boundedText(err.Error(), 300)
		return nil
	}
	staged, err := prepareFilesRestoreEdited(archive, targetRoot, nil)
	if err != nil {
		resp.Error = "prepare the copied files: " + boundedText(err.Error(), 300)
		return nil
	}
	defer staged.Cleanup()
	if withDatabase {
		dump := filepath.Join(work, "database.sql.gz")
		if err := dumpDatabaseToFile(copyDatabaseIdentity(req.SourceDatabase), dump); err != nil {
			resp.Error = "read the source database: " + boundedText(err.Error(), 300)
			return nil
		}
		if err := restoreDatabaseFromFile(copyDatabaseIdentity(req.TargetDatabase), dump); err != nil {
			resp.Error = "load the database copy: " + boundedText(err.Error(), 300)
			return nil
		}
	}
	if !req.Overwrite && !empty() {
		resp.Code = transport.SiteCopyTargetNotEmpty
		resp.Error = "the target site's web folder got content while the copy was prepared; nothing was published"
		return nil
	}
	if err := staged.Publish(); err != nil {
		resp.Error = "publish the copied files: " + boundedText(err.Error(), 300)
		return nil
	}
	resp.CompensationSafe = false
	if err := staged.Commit(); err != nil {
		resp.Detail = "the copy is published; cleaning up the previous folder did not finish: " + boundedText(err.Error(), 200)
	}
	resp.Copied = true
	return nil
}
