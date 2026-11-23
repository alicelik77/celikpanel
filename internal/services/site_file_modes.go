package services

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Per-site configuration that other sites cannot read (20 Nov 2026, tenant
// isolation). Until now the vhosts, the Panel's include files of each site
// and the PHP-FPM pools were written 0644, so every site account (its PHP, its
// scheduled tasks) could read every other site's names, aliases, folders,
// application ports and proxy targets. New files are 0640 now
// (managedSiteFileNewMode); a file that is there keeps its mode on every
// rewrite, as before.
//
// Files written by earlier releases are corrected once, by
// TightenManagedSiteFileModes at the Agent's start after the update: only a
// file that is still CelikPanel's unchanged text (its render header's digest
// matches), a regular file owned by root, at exactly 0644, the mode those
// releases wrote, becomes 0640. Any other mode is the owner's choice and is
// kept; an owner-edited file is the owner's and is kept. The Agent records
// that the correction ran and does not run it again, so a mode the owner sets
// afterwards is never changed back.
//
// Site başına yapılandırma artık başka sitelerce okunamaz: yeni dosyalar 0640.
// Eski sürümlerin dosyaları güncellemeden sonra bir kez düzeltilir: yalnız
// CelikPanel'in değişmemiş metni olan, root'a ait ve tam 0644 olan dosya 0640
// olur; başka her kip ve sahibin değiştirdiği dosya korunur.

// legacyManagedFileMode is the mode earlier releases wrote site files with.
const legacyManagedFileMode os.FileMode = 0o644

// ManagedFileModeChange is one file the correction changed or kept.
type ManagedFileModeChange struct {
	Path string
	// Kept: the file was left as it is; Why says why (a log line).
	Kept bool
	Why  string
}

// managedFileModeSets are the places CelikPanel writes per-site configuration
// into, with the header form of their files.
func managedFileModeSets() []managedFileModeSet {
	base := nginxBase()
	return []managedFileModeSet{
		{filepath.Join(base, "sites-available", "*.conf"), managedRenderHeaderPrefix, &nginxMutationMu},
		{filepath.Join(base, "sites-available", "*.conf"+SiteFilePendingSuffix), managedRenderHeaderPrefix, &nginxMutationMu},
		{filepath.Join(base, panelManagedDirName, "*", "*"), managedRenderHeaderPrefix, &nginxMutationMu},
		{filepath.Join(phpEtcDir, "*", "fpm", "pool.d", "*.conf"), ManagedHeaderPrefixINI, &managedConfigMutationMu},
		{filepath.Join(phpEtcDir, "*", "fpm", "pool.d", "*.conf"+SiteFilePendingSuffix), ManagedHeaderPrefixINI, &managedConfigMutationMu},
		{filepath.Join(phpEtcDir, "php-fpm.d", "*.conf"), ManagedHeaderPrefixINI, &managedConfigMutationMu},
		{filepath.Join(phpEtcDir, "php-fpm.d", "*.conf"+SiteFilePendingSuffix), ManagedHeaderPrefixINI, &managedConfigMutationMu},
	}
}

// managedFileModeSet is one place, its files' header form and the lock that
// serializes writes there.
type managedFileModeSet struct {
	pattern, header string
	lock            *sync.Mutex
}

// TightenManagedSiteFileModes is the one-time correction described above. It
// returns what it changed and what it kept on purpose; files that are not
// CelikPanel's are not listed.
func TightenManagedSiteFileModes() []ManagedFileModeChange {
	var changes []ManagedFileModeChange
	seen := map[string]bool{}
	for _, set := range managedFileModeSets() {
		matches, err := filepath.Glob(set.pattern)
		if err != nil {
			continue
		}
		set.lock.Lock()
		for _, path := range matches {
			if seen[path] {
				continue
			}
			seen[path] = true
			if change, ok := tightenManagedFileMode(path, set.header); ok {
				changes = append(changes, change)
			}
		}
		set.lock.Unlock()
	}
	return changes
}

// tightenManagedFileMode corrects one file; ok=false: not CelikPanel's
// unchanged text (not listed).
func tightenManagedFileMode(path, header string) (ManagedFileModeChange, bool) {
	inspected := inspectSiteFileWith(path, header, nil, "")
	if inspected.state != transport.SiteFileManagedUnchanged {
		return ManagedFileModeChange{}, false
	}
	if inspected.mode == managedSiteFileNewMode {
		// Already the mode this release writes: nothing to do or to say.
		return ManagedFileModeChange{}, false
	}
	if inspected.mode != legacyManagedFileMode {
		// Any other mode, also one that already keeps other accounts out
		// (0600 and the like), is the owner's choice: kept and named in the
		// journal, as docs/TENANT-ISOLATION.md says (alpha.142 lab: an
		// owner's 0600 was kept but the summary said "0 kept").
		return ManagedFileModeChange{Path: path, Kept: true,
			Why: fmt.Sprintf("mode %04o is not the %04o earlier releases wrote; the owner's choice is kept", uint32(inspected.mode), uint32(legacyManagedFileMode))}, true
	}
	if err := chmodRegularNoFollow(path, inspected.fileSHA, legacyManagedFileMode&^0o004); err != nil {
		return ManagedFileModeChange{Path: path, Kept: true, Why: err.Error()}, true
	}
	return ManagedFileModeChange{Path: path}, true
}

// chmodRegularNoFollow changes the mode of the regular file at path, never
// through a link, and only while it still holds the bytes that were checked.
func chmodRegularNoFollow(path, wantSHA string, mode os.FileMode) error {
	file, err := openRegularNoFollow(path)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("not a regular file")
	}
	if !rootOwned(info) {
		return errors.New("not owned by root; left as it is")
	}
	content, err := readAllBounded(file, maxManagedSiteFileBytes)
	if err != nil {
		return err
	}
	if sha256Hex(content) != wantSHA {
		return errSiteFileChanged
	}
	return file.Chmod(mode)
}

// readAllBounded reads the file, refusing one larger than limit.
func readAllBounded(file *os.File, limit int64) ([]byte, error) {
	content, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(content)) > limit {
		return nil, errors.New("the file is larger than CelikPanel writes")
	}
	return content, nil
}
