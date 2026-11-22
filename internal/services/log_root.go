package services

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
)

// CelikPanel's own log folder (alpha.144, the owner's server read on
// 22 November 2026). nginx's workers run as the web server's account and
// reopen the traffic logs by their path after each nightly rotation; the
// folder above the traffic logs was made by the root Agent under its
// umask 0027 (0700 when the certificate tool's logs made it first, 0750
// otherwise), so the workers could not pass through it: one "[emerg]
// open() … failed (13: Permission denied)" per worker and log every night,
// and the workers kept writing into the rotated file until the next reload
// (traffic under-counted).
//
//   - /var/log/celikpanel is passable for everybody and listable by nobody
//     else (0711). It holds only the certificate tool's logs (0700, closed)
//     and the traffic folder.
//   - The traffic folder is passable for the web server's workers and for
//     nobody else: 0710 root:<the web server's group> (independent review
//     of PR #355, finding 1: with 0711 every local account, a site's PHP
//     too, could open another site's traffic log by name).
//   - A site's first traffic log is made by CelikPanel, owned by the web
//     server's account at 0640, before nginx opens it, so it is never at
//     nginx's own 0644 (logrotate's create keeps that mode afterwards).
//
// A folder that is a link the owner made is the owner's: nothing is made or
// changed there, and the work goes on through the link as before.
//
// CelikPanel'in kendi günlük klasörü: trafik klasörüne yalnız web
// sunucusunun işçileri geçebilir; hiçbir sitenin hesabı başka sitenin
// günlüğünü açamaz.

// celikPanelLogRoot is the folder (a seam for tests).
var celikPanelLogRoot = "/var/log/celikpanel"

// LogRootMode is the mode a new log folder is made with.
const LogRootMode os.FileMode = 0o711

// TrafficLogDirMode is the traffic folder's mode, with the web server's
// group: its workers open their logs by name; nobody else passes.
const TrafficLogDirMode os.FileMode = 0o710

// TrafficLogFileMode is a traffic log's mode: the web server's account
// writes it; nobody else reads it.
const TrafficLogFileMode os.FileMode = 0o640

// CelikPanelLogRoot is CelikPanel's log folder.
func CelikPanelLogRoot() string { return celikPanelLogRoot }

// SetCelikPanelLogRootForTests points the log folder elsewhere and returns
// the function that puts it back (tests of other packages).
func SetCelikPanelLogRootForTests(dir string) func() {
	saved := celikPanelLogRoot
	celikPanelLogRoot = dir
	return func() { celikPanelLogRoot = saved }
}

// webServerIDs is the web server's account and group: the account the PHP
// pools' listen.owner names (phpWebServerAccount). A seam for tests.
var webServerIDs = func() (uid, gid int, ok bool) {
	name := phpWebServerAccount()
	account, err := user.Lookup(name)
	if err != nil {
		return 0, 0, false
	}
	group, err := user.LookupGroup(name)
	if err != nil {
		return 0, 0, false
	}
	uid, errU := strconv.Atoi(account.Uid)
	gid, errG := strconv.Atoi(group.Gid)
	if errU != nil || errG != nil {
		return 0, 0, false
	}
	return uid, gid, true
}

// WebServerIDs is the web server's account and group ids.
func WebServerIDs() (uid, gid int, ok bool) { return webServerIDs() }

// WebServerAccountName is the name of the web server's account, which is
// also its group's name (phpWebServerAccount), when both can be read; ""
// otherwise.
func WebServerAccountName() string {
	if _, _, ok := webServerIDs(); !ok {
		return ""
	}
	return phpWebServerAccount()
}

// EnsureCelikPanelLogRoot makes the log folder at LogRootMode when it is
// absent. An existing folder, or a link the owner made, is left as it is (an
// earlier release's mode is corrected once by the Agent at start; any other
// mode is the owner's).
func EnsureCelikPanelLogRoot() error {
	_, err := ensureLogFolder(celikPanelLogRoot, LogRootMode, -1)
	return err
}

// ensureLogFolder makes dir with exactly mode (and group gid when gid >= 0
// and the Agent runs as root) when it is absent, the folders above it with
// MkdirAll. made=false when it existed: a real folder or a link to one is
// left as it is (a link is the owner's; work goes on through it); a link
// that leads nowhere, or another kind of file, is an error.
func ensureLogFolder(dir string, mode os.FileMode, gid int) (bool, error) {
	if info, err := os.Lstat(dir); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			if target, err := os.Stat(dir); err != nil || !target.IsDir() {
				return false, fmt.Errorf("%s is a link that does not lead to a folder; it was left alone", dir)
			}
			return false, nil
		}
		if !info.IsDir() {
			return false, fmt.Errorf("%s exists and is not a directory; it was left alone", dir)
		}
		return false, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return false, err
	}
	if err := os.Mkdir(dir, mode); err != nil && !errors.Is(err, os.ErrExist) {
		return false, err
	}
	if gid >= 0 && os.Geteuid() == 0 && !nginxDevMode() {
		if err := os.Lchown(dir, 0, gid); err != nil {
			return true, err
		}
	}
	// The umask took bits away; the mode is set as such.
	return true, os.Chmod(dir, mode)
}

// errWebServerAccountUnknown: the web server's account or group could not be
// read where a folder or a first log had to be made for it.
var errWebServerAccountUnknown = errors.New("the web server's account could not be read")

// ensureTrafficLogFile makes a site's traffic log before nginx opens it:
// owned by the web server's account and group at TrafficLogFileMode, never
// through a link. An existing file is left as it is (an earlier one readable
// by others is corrected once by the Agent at start), with no lookup of the
// web server's account. O_CREATE with O_EXCL never follows a link: a link
// in its place is "exists".
func ensureTrafficLogFile(path string) error {
	if _, err := os.Lstat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	chown := os.Geteuid() == 0 && !nginxDevMode()
	uid, gid := -1, -1
	if chown {
		var ok bool
		if uid, gid, ok = webServerIDs(); !ok {
			return fmt.Errorf("%w (to make %s)", errWebServerAccountUnknown, path)
		}
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, TrafficLogFileMode)
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer file.Close()
	if chown {
		if err := file.Chown(uid, gid); err != nil {
			return err
		}
	}
	return file.Chmod(TrafficLogFileMode)
}
