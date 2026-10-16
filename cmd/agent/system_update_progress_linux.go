//go:build linux

package main

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// Where the reviewed installer (download-portal/get.sh, installed as
// /usr/libexec/celikpanel/get.sh and run by the worker) keeps the archive it
// downloads: a fresh `mktemp -d "$releases_root/.download.XXXXXXXX"` directory
// under this root, which the installer itself creates root:root 0700.
// Kurucunun indirdigi arsivi tuttugu kok; her calismada yeni .download.* dizini.
const systemUpdateDownloadRoot = "/var/backups/celikpanel/releases"

// The phases of a real update, as the updater runs them, and what of each can
// be read from outside without asking the updater anything (owner design
// 2026-10-16):
//
//  1. get.sh fetches and verifies the signed manifest, then downloads the
//     archive with curl into its .download.* directory. curl writes the file
//     as the bytes arrive, so the file's size is the download's progress. The
//     installer reports nothing else about it; when the directory cannot be
//     matched exactly (none, or more than one candidate), the size is not
//     reported and the download step is indeterminate.
//  2. get.sh checks the archive's size and SHA-256 against the signed
//     manifest, lists and extracts it and checks every extracted file against
//     its SHA256SUMS and provenance, and only then publishes the release floor
//     (persist_release_sequence_floor: "Consume the authenticated sequence only
//     after the entire archive and its provenance are verified, but before the
//     updater can mutate host services"). A floor naming this request's
//     sequence and version, written at or after the request was created, is
//     the evidence that verification finished, and its modification time is
//     when. A retry of the same release does not rewrite the floor; then this
//     boundary is not reported.
//  3. update.sh (from the verified archive) checks the server, freezes and
//     stops the Panel and the Agent, installs and starts the new release. The
//     Agent answering this read is the old one until it is stopped; it has
//     nothing more to read here. The Panel reports the rest.
//
// Guncellemenin gercek asamalari ve disaridan okunabilenleri: indirme (dosya
// boyutu), dogrulama (bu istegin surum tabani yayimlandi), sonrasi Panel'de.
func (backend *linuxSystemUpdateBackend) Progress(state *systemUpdateState) systemUpdateProgress {
	var progress systemUpdateProgress
	if state == nil {
		return progress
	}
	created, err := time.Parse(time.RFC3339Nano, state.CreatedAt)
	if err != nil {
		return progress
	}
	// File times have the file system's precision; the record's creation time
	// is compared at whole seconds so a floor written in the same second counts.
	since := created.UTC().Truncate(time.Second)
	if verified, ok := systemUpdateFloorVerifiedAt(backend.floorPath, state, since); ok {
		progress.VerifiedAt = verified
		return progress
	}
	if !state.active() {
		return progress
	}
	root := backend.downloadRoot
	if root == "" {
		return progress
	}
	size, err := parseCanonicalPositiveDecimal(state.TargetArchiveSize, systemUpdateMaxArchiveSize)
	if err != nil {
		return progress
	}
	written, modified, ok := systemUpdateDownloadedArchive(root, systemUpdateArchiveName(state), since)
	if !ok {
		return progress
	}
	if written > size {
		// More than the signed size is not this download; say nothing.
		return progress
	}
	progress.DownloadedBytes, progress.HaveBytes = written, true
	if written == size {
		progress.DownloadedAt = modified.UTC()
	}
	return progress
}

// The archive name get.sh downloads for a signed update:
// celikpanel-$version-$os-$arch.tar.gz.
func systemUpdateArchiveName(state *systemUpdateState) string {
	return "celikpanel-" + state.TargetVersion + "-" + state.TargetOS + "-" + state.TargetArch + ".tar.gz"
}

func systemUpdateFloorVerifiedAt(path string, state *systemUpdateState, since time.Time) (time.Time, bool) {
	if path == "" {
		return time.Time{}, false
	}
	raw, err := readRootOwnedSystemUpdateFile(path, 1024, false)
	if err != nil {
		return time.Time{}, false
	}
	floor, err := parseCanonicalSystemUpdateFloor(raw)
	if err != nil || floor.Sequence != state.TargetSequence || floor.Version != state.TargetVersion {
		return time.Time{}, false
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return time.Time{}, false
	}
	modified := info.ModTime().UTC()
	if modified.Before(since) {
		return time.Time{}, false
	}
	return modified, true
}

// systemUpdateDownloadedArchive finds the one archive of this request in the
// installer's download directories and returns its size and modification
// time. Every entry is read with Lstat (no link is followed) and must have the
// owner and mode the installer gives it; an archive older than the request is
// a leftover of another run and is not counted. Anything ambiguous is
// reported as not found.
func systemUpdateDownloadedArchive(root, name string, since time.Time) (uint64, time.Time, bool) {
	uid, gid := systemUpdateExpectedOwner(root)
	if !systemUpdateProgressDirectory(root, uid, gid) {
		return 0, time.Time{}, false
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return 0, time.Time{}, false
	}
	var (
		found    bool
		size     uint64
		modified time.Time
	)
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), ".download.") {
			continue
		}
		directory := filepath.Join(root, entry.Name())
		if !systemUpdateProgressDirectory(directory, uid, gid) {
			continue
		}
		info, err := os.Lstat(filepath.Join(directory, name))
		if err != nil {
			continue
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || !info.Mode().IsRegular() || stat.Uid != uid || stat.Gid != gid || stat.Nlink != 1 ||
			info.Size() < 0 || info.ModTime().Before(since) {
			continue
		}
		if found {
			return 0, time.Time{}, false
		}
		found, size, modified = true, uint64(info.Size()), info.ModTime()
	}
	return size, modified, found
}

func systemUpdateProgressDirectory(path string, uid, gid uint32) bool {
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uid && stat.Gid == gid && info.Mode().Perm() == 0o700
}
