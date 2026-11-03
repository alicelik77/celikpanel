package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/alicelik/celikpanel/internal/backupspec"
)

// Importing a backup package that came back from a remote destination
// (alpha.118, remote backups). The Panel downloads, verifies and decrypts the
// object, then hands the plain package to the Agent in chunks. The Agent
// appends them to a private staging file in the domain's own backup folder
// (a dot-name, so listings never show it) and, on the final chunk, checks the
// whole file's size and SHA-256, the manifest's scope (this subscription and
// this domain) and every payload's size and SHA-256 before it publishes the
// package under its original name. Only then can the existing restore path
// use it. Nothing here restores; a refused import leaves nothing behind.
//
// Uzaktan dönen yedek paketi parça parça özel bir hazırlık dosyasına yazılır;
// son parçada boyut, özet, kapsam ve her yükün özeti doğrulanmadan yedek
// yayımlanmaz. Geri yükleme mevcut yoldan yapılır.

var backupImportMu sync.Mutex

func importStagingPath(dir, importID string) string {
	sum := sha256.Sum256([]byte(importID))
	return filepath.Join(dir, ".import-"+hex.EncodeToString(sum[:12])+".cpbak")
}

func importScope(r *backupspec.ImportChunkRequest) backupScope {
	return backupScope{r.ProtocolVersion, r.SubscriptionID, r.DomainID, r.DomainName}
}

// ImportBackupChunk is the RPC.
func (a *Agent) ImportBackupChunk(req *backupspec.ImportChunkRequest, resp *backupspec.ImportChunkResponse) error {
	info, offset, err := importBackupChunk(req)
	resp.Offset = offset
	if err != nil {
		resp.Error = err.Error()
		return nil
	}
	resp.Success = true
	resp.Backup = info
	return nil
}

// AbortBackupImport is the RPC that drops an unfinished import.
func (a *Agent) AbortBackupImport(req *backupspec.ImportAbortRequest, resp *bool) error {
	scope := backupScope{req.ProtocolVersion, req.SubscriptionID, req.DomainID, req.DomainName}
	if err := validateV2Scope(scope); err != nil {
		return err
	}
	if !backupspec.ValidJobKey(req.ImportID) {
		return errors.New("invalid import ID")
	}
	dir, err := scopeBackupDir(scope)
	if err != nil {
		return err
	}
	backupImportMu.Lock()
	defer backupImportMu.Unlock()
	staging := importStagingPath(dir, req.ImportID)
	if err := os.Remove(staging); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	*resp = true
	return nil
}

func importBackupChunk(req *backupspec.ImportChunkRequest) (backupspec.Info, int64, error) {
	if req == nil {
		return backupspec.Info{}, 0, errors.New("import request is required")
	}
	scope := importScope(req)
	if err := validateV2Scope(scope); err != nil {
		return backupspec.Info{}, 0, err
	}
	if !backupspec.ValidJobKey(req.ImportID) {
		return backupspec.Info{}, 0, errors.New("invalid import ID")
	}
	if !generatedBackupName.MatchString(req.BackupName) {
		return backupspec.Info{}, 0, invalidBackupName()
	}
	if len(req.Data) > backupspec.MaxChunkBytes || req.Offset < 0 {
		return backupspec.Info{}, 0, errors.New("invalid import chunk")
	}
	dir, err := ensureBackupDir(scope)
	if err != nil {
		return backupspec.Info{}, 0, err
	}
	backupImportMu.Lock()
	defer backupImportMu.Unlock()
	staging := importStagingPath(dir, req.ImportID)

	var current int64
	if st, err := os.Lstat(staging); err == nil {
		if !st.Mode().IsRegular() {
			return backupspec.Info{}, 0, errors.New("import staging file is not a regular file")
		}
		current = st.Size()
	} else if !errors.Is(err, os.ErrNotExist) {
		return backupspec.Info{}, 0, err
	}
	end := req.Offset + int64(len(req.Data))
	if end > maxPackagePayloadBytes {
		return backupspec.Info{}, current, errors.New("package size limit exceeded")
	}
	switch {
	case req.Offset == current && len(req.Data) > 0:
		if err := appendImportChunk(staging, current, req.Data); err != nil {
			return backupspec.Info{}, current, err
		}
		current = end
	case end == current:
		// The same chunk again after a lost answer: already written.
	case req.Offset != current:
		return backupspec.Info{}, current, errors.New("import chunk out of order")
	}
	if !req.Final {
		return backupspec.Info{}, current, nil
	}
	info, err := finishBackupImport(scope, dir, staging, req)
	if err != nil {
		_ = os.Remove(staging)
		return backupspec.Info{}, 0, err
	}
	return info, current, nil
}

func appendImportChunk(staging string, offset int64, data []byte) error {
	var file *os.File
	var err error
	if offset == 0 {
		file, err = openPrivateExclusive(staging)
	} else {
		if err := rejectSymlinkPath(filepath.Dir(staging)); err != nil {
			return err
		}
		file, err = os.OpenFile(staging, os.O_WRONLY|os.O_APPEND, 0o600)
	}
	if err != nil {
		return err
	}
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}

func finishBackupImport(scope backupScope, dir, staging string, req *backupspec.ImportChunkRequest) (backupspec.Info, error) {
	file, stat, err := secureOpenRegular(staging)
	if err != nil {
		return backupspec.Info{}, err
	}
	hash := sha256.New()
	size, copyErr := io.Copy(hash, file)
	syncErr := file.Sync()
	_ = file.Close()
	if copyErr != nil {
		return backupspec.Info{}, copyErr
	}
	if syncErr != nil {
		return backupspec.Info{}, syncErr
	}
	if size != stat.Size() || size != req.Size || hex.EncodeToString(hash.Sum(nil)) != req.SHA256 {
		return backupspec.Info{}, errors.New("imported package does not match its size or checksum")
	}
	manifest, err := verifyBackupPackage(staging, scope)
	if err != nil {
		return backupspec.Info{}, err
	}
	if manifest.Type == backupspec.TypeDatabase {
		if want, err := newBackupNameDatabasePrefix(manifestDatabaseID(manifest)); err != nil || len(req.BackupName) < len(want) || req.BackupName[:len(want)] != want {
			return backupspec.Info{}, errors.New("imported package name does not match its contents")
		}
	} else if len(req.BackupName) < len(manifest.Type)+1 || req.BackupName[:len(manifest.Type)+1] != manifest.Type+"-" {
		return backupspec.Info{}, errors.New("imported package name does not match its contents")
	}
	finalPath := filepath.Join(dir, req.BackupName)
	if existing, _, err := secureOpenRegular(finalPath); err == nil {
		// The package is already here (still local, or imported before):
		// the same bytes are the same backup.
		existingHash := sha256.New()
		_, copyErr := io.Copy(existingHash, existing)
		_ = existing.Close()
		if copyErr != nil || hex.EncodeToString(existingHash.Sum(nil)) != req.SHA256 {
			return backupspec.Info{}, errors.New("a different backup with this name already exists")
		}
		_ = os.Remove(staging)
	} else if errors.Is(err, os.ErrNotExist) {
		if err := os.Chmod(staging, 0o600); err != nil {
			return backupspec.Info{}, err
		}
		if err := atomicPublishFile(staging, finalPath); err != nil {
			return backupspec.Info{}, err
		}
		if err := syncDirectory(dir); err != nil {
			return backupspec.Info{}, err
		}
	} else {
		return backupspec.Info{}, err
	}
	info := manifest.info(req.BackupName, size, false)
	info.Path = finalPath
	return info, nil
}

func newBackupNameDatabasePrefix(databaseID int) (string, error) {
	if databaseID < 1 {
		return "", errors.New("database ID required")
	}
	return "database-" + strconv.Itoa(databaseID) + "-", nil
}

// verifyBackupPackage reads a package through without writing anything:
// the manifest must be first and valid for this scope, and every payload
// must be present once with the size and SHA-256 the manifest names.
func verifyBackupPackage(backupPath string, scope backupScope) (backupManifest, error) {
	manifest, err := readBackupManifest(backupPath)
	if err != nil {
		return backupManifest{}, err
	}
	if err := validateManifest(manifest, scope); err != nil {
		return backupManifest{}, err
	}
	expected := make(map[string]manifestPayload)
	for _, payload := range sortedPayloads(manifest) {
		expected[payload.Name] = payload
	}
	file, _, err := secureOpenRegular(backupPath)
	if err != nil {
		return backupManifest{}, err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return backupManifest{}, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	seen := make(map[string]bool)
	entries := 0
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return backupManifest{}, err
		}
		entries++
		if entries > maxPackageEntries || header.Typeflag != tar.TypeReg || !validPackageEntryName(header.Name) {
			return backupManifest{}, errors.New("unsafe package entry")
		}
		if header.Name == backupManifestName {
			if entries != 1 {
				return backupManifest{}, errors.New("duplicate or misplaced manifest")
			}
			continue
		}
		payload, ok := expected[header.Name]
		if !ok || seen[header.Name] || header.Size != payload.Size {
			return backupManifest{}, errors.New("unexpected package payload")
		}
		seen[header.Name] = true
		hash := sha256.New()
		written, err := io.Copy(hash, tr)
		if err != nil {
			return backupManifest{}, err
		}
		if written != payload.Size || hex.EncodeToString(hash.Sum(nil)) != payload.SHA256 {
			return backupManifest{}, errors.New("payload hash mismatch")
		}
	}
	if len(seen) != len(expected) {
		return backupManifest{}, errors.New("incomplete package")
	}
	return manifest, nil
}
