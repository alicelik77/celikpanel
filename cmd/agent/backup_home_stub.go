//go:build !linux

package main

import (
	"time"

	"github.com/alicelik/celikpanel/internal/backupspec"
)

func safeCreateHomeArchive(string, string) (homeWalk, bool, error) {
	return homeWalk{}, false, errBackupPlatformUnsupported
}

func measureSiteBackup(string, string, time.Duration) backupspec.ContentsResponse {
	return backupspec.ContentsResponse{Areas: []backupspec.Area{}, LeftOut: []backupspec.LeftOut{},
		BackupFreeBytes: -1, Error: errBackupPlatformUnsupported.Error()}
}

func prepareAreaRestore(string, string, string) (*stagedFilesRestore, bool, error) {
	return nil, false, errBackupPlatformUnsupported
}

func removeMadeArea(string, string) {}
