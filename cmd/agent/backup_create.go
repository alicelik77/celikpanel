package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/alicelik/celikpanel/internal/backupspec"
)

func (a *Agent) createBackup(req *backupspec.CreateRequest) (backupspec.Info, error) {
	identity, err := normalizeBackupCreateRequest(req)
	if err != nil {
		return backupspec.Info{}, err
	}
	if identity.jobKey == "" {
		return a.createBackupPhysical(identity)
	}
	lock := backupJobMutex(identity)
	lock.Lock()
	defer lock.Unlock()
	if info, found, err := findPublishedBackupForJob(identity); err != nil {
		return backupspec.Info{}, err
	} else if found {
		return info, nil
	}
	return a.createBackupPhysical(identity)
}

func (a *Agent) createBackupPhysical(identity backupCreateIdentity) (backupspec.Info, error) {
	backupDir, err := ensureBackupDir(identity.scope)
	if err != nil {
		return backupspec.Info{}, err
	}
	workDir, err := os.MkdirTemp(backupDir, ".build-")
	if err != nil {
		return backupspec.Info{}, err
	}
	if err := os.Chmod(workDir, 0o700); err != nil {
		_ = os.RemoveAll(workDir)
		return backupspec.Info{}, err
	}
	defer os.RemoveAll(workDir)

	manifest := backupManifest{
		Version:        backupspec.ManifestVersionSite,
		Type:           identity.typeName,
		Origin:         identity.origin,
		JobKey:         identity.jobKey,
		SubscriptionID: identity.scope.SubscriptionID,
		DomainID:       identity.scope.DomainID,
		CreatedAt:      backupNow().UTC(),
	}
	switch identity.typeName {
	case backupspec.TypeFiles:
		if err := buildFilesPayload(identity.scope, workDir, &manifest); err != nil {
			return backupspec.Info{}, err
		}
		if err := buildHomePayload(identity.scope, workDir, &manifest); err != nil {
			return backupspec.Info{}, err
		}
	case backupspec.TypeDatabase:
		if err := buildDatabasePayload(workDir, identity.databases[0], &manifest); err != nil {
			return backupspec.Info{}, err
		}
	case backupspec.TypeFull:
		if err := buildFilesPayload(identity.scope, workDir, &manifest); err != nil {
			return backupspec.Info{}, err
		}
		if err := buildHomePayload(identity.scope, workDir, &manifest); err != nil {
			return backupspec.Info{}, err
		}
		for _, database := range identity.databases {
			if err := buildDatabasePayload(workDir, database, &manifest); err != nil {
				return backupspec.Info{}, err
			}
		}
	}
	if err := validateManifest(manifest, identity.scope); err != nil {
		return backupspec.Info{}, err
	}
	name, err := newBackupName(identity.typeName, manifestDatabaseID(manifest))
	if err != nil {
		return backupspec.Info{}, err
	}
	finalPath := filepath.Join(backupDir, name)
	if err := publishBackupPackage(finalPath, workDir, manifest); err != nil {
		return backupspec.Info{}, err
	}
	info, err := os.Stat(finalPath)
	if err != nil {
		return backupspec.Info{}, err
	}
	result := manifest.info(name, info.Size(), false)
	result.Path = finalPath
	return result, nil
}

func buildFilesPayload(scope backupScope, workDir string, manifest *backupManifest) error {
	docroot, err := backupDocumentRoot(scope.SubscriptionID, scope.DomainID)
	if err != nil {
		return err
	}
	payloadPath := filepath.Join(workDir, filepath.FromSlash(filesPayloadName))
	if err := mkdirPrivate(filepath.Dir(payloadPath)); err != nil {
		return err
	}
	if err := safeCreateFilesArchive(docroot, payloadPath); err != nil {
		return err
	}
	manifest.Files, err = describePayload(payloadPath, filesPayloadName)
	return err
}

// buildHomePayload adds the site-home areas (docs/SITE-BACKUP-CONTENTS-DESIGN.md)
// when the site has any: the manifest becomes version 3. A site without an
// area keeps the version-2 package unchanged.
// Sitenin ev dizini alanları varsa eklenir; paket sürüm 3 olur.
func buildHomePayload(scope backupScope, workDir string, manifest *backupManifest) error {
	home, err := backupSiteHome(scope.SubscriptionID, scope.DomainID)
	if err != nil {
		return err
	}
	payloadPath := filepath.Join(workDir, filepath.FromSlash(homePayloadName))
	if err := mkdirPrivate(filepath.Dir(payloadPath)); err != nil {
		return err
	}
	walk, wrote, err := safeCreateHomeArchive(home, payloadPath)
	if err != nil {
		return err
	}
	if !wrote {
		return nil
	}
	payload, err := describePayload(payloadPath, homePayloadName)
	if err != nil {
		return err
	}
	manifest.Version = backupspec.ManifestVersionHome
	manifest.Home = &payload
	manifest.Areas = walk.areas
	manifest.LeftOut = walk.left.items
	manifest.LeftOutMore = walk.left.more
	return nil
}

func buildDatabasePayload(workDir string, database backupspec.DatabaseIdentity, manifest *backupManifest) error {
	database, err := validateDatabaseIdentity(database)
	if err != nil {
		return err
	}
	// alpha.127: the PostgreSQL cluster is resolved now and recorded.
	if database, err = resolveBackupCluster(database); err != nil {
		return err
	}
	payloadName := databasePayloadName(database.ID)
	payloadPath := filepath.Join(workDir, filepath.FromSlash(payloadName))
	if err := mkdirPrivate(filepath.Dir(payloadPath)); err != nil {
		return err
	}
	if err := dumpDatabaseToFile(database, payloadPath); err != nil {
		return fmt.Errorf("database %d dump: %w", database.ID, err)
	}
	payload, err := describePayload(payloadPath, payloadName)
	if err != nil {
		return err
	}
	manifest.Databases = append(manifest.Databases, manifestDatabase{Identity: database, Payload: payload})
	return nil
}

func databaseFromCreateRequest(req *backupspec.CreateRequest) (backupspec.DatabaseIdentity, error) {
	database := req.Database
	if database.Name == "" && req.ProtocolVersion != backupspec.ProtocolVersion {
		database.Name = req.DatabaseName
		database.Type = req.DatabaseType
	}
	return validateDatabaseIdentity(database)
}
