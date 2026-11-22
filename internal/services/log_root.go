package services

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// CelikPanel's own log folder (alpha.144, the owner's server read on
// 22 November 2026). nginx's workers run as the web server's account and
// reopen the traffic logs by their path after each nightly rotation; the
// folder above the traffic logs was made by the root Agent under its
// umask 0027 (0700 when the certificate tool's logs made it first, 0750
// otherwise), so the workers could not pass through it: one "[emerg]
// open() … failed (13: Permission denied)" per worker and log every night,
// and the workers kept writing into the rotated file until the next reload
// (traffic under-counted). It is made passable for everybody and listable
// by nobody else (0711), whatever the umask and whichever path makes it
// first; the traffic folder under it is passable only (0711) too, and the
// certificate tool's log folder stays closed (0700).
//
// CelikPanel'in kendi günlük klasörü: web sunucusunun işçileri trafik
// günlüklerine geçebilir; başka hiçbir şey okunamaz.

// celikPanelLogRoot is the folder (a seam for tests).
var celikPanelLogRoot = "/var/log/celikpanel"

// LogRootMode is the mode a new log folder is made with.
const LogRootMode os.FileMode = 0o711

// TrafficLogDirMode is the traffic folder's mode: the workers open their
// logs by name; nobody else lists it.
const TrafficLogDirMode os.FileMode = 0o711

// CelikPanelLogRoot is CelikPanel's log folder.
func CelikPanelLogRoot() string { return celikPanelLogRoot }

// EnsureCelikPanelLogRoot makes the log folder at LogRootMode when it is
// absent. An existing folder is left as it is (an earlier release's mode is
// corrected once by the Agent at start; any other mode is the owner's).
func EnsureCelikPanelLogRoot() error {
	return ensureLogFolder(celikPanelLogRoot, LogRootMode)
}

// ensureLogFolder makes dir with exactly mode when it is absent (the
// folders above it with MkdirAll), never through a link.
func ensureLogFolder(dir string, mode os.FileMode) error {
	if info, err := os.Lstat(dir); err == nil {
		if !info.IsDir() {
			return fmt.Errorf("%s exists and is not a directory; it was left alone", dir)
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return err
	}
	if err := os.Mkdir(dir, mode); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	// The umask took bits away; the mode is set as such.
	return os.Chmod(dir, mode)
}
