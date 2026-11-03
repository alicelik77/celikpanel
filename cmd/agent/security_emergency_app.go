package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Security emergency, a Node.js application's start lock (alpha.117, D-043).
// Stopping and disabling the app is not enough: anything that starts the
// unit again (a site file reapply, a reboot with the owner's own enable, a
// hand-typed systemctl start) would run the withdrawn version. The lock is a
// systemd drop-in that refuses to start the unit while its PATH names the
// withdrawn Node.js version, so a move to another version starts normally
// and a put-back on the withdrawn one never runs, not even briefly.
//
// The drop-in is the one file CelikPanel writes in the unit's drop-in
// directory. It is removed only while it is exactly what CelikPanel wrote;
// a file changed on the server is the owner's and is left as it is (D-022).
//
// Güvenlik acil durumu: Node.js uygulamasının başlatma kilidi. Birim, geri
// çekilen sürümle başlatılamaz; başka sürüme taşınan uygulama normal başlar.

const emergencyAppLockFile = "celikpanel-security-emergency.conf"

// emergencyAppLockPath is the lock's path for one site.
func emergencyAppLockPath(siteID int) (string, error) {
	dir, err := unitDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, appUnitName(siteID)+".service.d", emergencyAppLockFile), nil
}

// emergencyAppLockText is the drop-in for one withdrawn Node.js version.
// "$$" is systemd's escape for a literal "$"; Restart=no keeps the refused
// start from being retried every few seconds.
func emergencyAppLockText(version string) string {
	nodeDir := filepath.Dir(nodeBinPath(version))
	return "# CelikPanel security emergency (D-043): Node.js " + version + " is withdrawn for security.\n" +
		"# This file keeps the application from starting on Node.js " + version + ".\n" +
		"# CelikPanel removes it when the administrator re-enables Node.js " + version + "\n" +
		"# or the site moves to another version. To start the application anyway,\n" +
		"# delete this file and run: systemctl daemon-reload\n" +
		"[Service]\n" +
		"Restart=no\n" +
		"ExecStartPre=/bin/sh -c 'case \":$${PATH}:\" in *\":" + nodeDir + ":\"*) " +
		"echo \"CelikPanel security emergency: Node.js " + version + " is withdrawn for security, so this application does not start on it. " +
		"Move the site to another Node.js version in CelikPanel.\" >&2; exit 1;; esac'\n"
}

// readEmergencyAppLock reads the lock file; present false when there is none.
func readEmergencyAppLock(path string) (string, bool, error) {
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	file, _, err := secureOpenRegular(path)
	if err != nil {
		return "", true, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 64*1024))
	if err != nil {
		return "", true, err
	}
	return string(data), true, nil
}

// SetEmergencyAppLockV1 writes or removes one application's start lock.
func (a *Agent) SetEmergencyAppLockV1(req *transport.SetEmergencyAppLockRequest, resp *transport.SetEmergencyAppLockResponse) error {
	if req == nil || resp == nil {
		return fmt.Errorf("emergency app lock request and response are required")
	}
	*resp = transport.SetEmergencyAppLockResponse{}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "locking or unlocking a Node.js application in a security emergency"); err != nil {
		resp.Error = err.Error()
		return nil
	}
	if req.SiteID <= 0 || !nodeVersionRe.MatchString(req.NodeVersion) {
		resp.Error = "a positive site identity and a Node.js version such as 22.11.0 are required; nothing was changed"
		return nil
	}
	unlock := lockAppUnitMutation(req.SiteID)
	defer unlock()
	path, err := emergencyAppLockPath(req.SiteID)
	if err != nil {
		resp.Error = err.Error() + "; nothing was changed"
		return nil
	}
	resp.Path = path
	want := emergencyAppLockText(req.NodeVersion)
	current, present, err := readEmergencyAppLock(path)
	if err != nil {
		resp.Conflict = true
		resp.Error = "the lock file " + path + " could not be read as a plain file, so it was left as it is: " + err.Error()
		return nil
	}
	if present && current != want {
		resp.Conflict = true
		resp.Error = "the lock file " + path + " was changed on the server, so it was left as it is"
		return nil
	}
	switch {
	case req.Lock && present, !req.Lock && !present:
		resp.Applied = true
		return nil
	case req.Lock:
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			resp.Error = "the drop-in directory could not be made: " + err.Error()
			return nil
		}
		if err := writeAtomicFile(path, []byte(want), 0o644); err != nil {
			resp.Error = "the lock file could not be written: " + err.Error()
			return nil
		}
	default:
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			resp.Error = "the lock file could not be removed: " + err.Error()
			return nil
		}
		// The directory goes too when the lock was the only file in it.
		_ = os.Remove(filepath.Dir(path))
	}
	resp.Changed = true
	if out, err := appUnitSystemctl(systemctlArgs("daemon-reload")...); err != nil {
		resp.Error = fmt.Sprintf("the lock file was changed but systemctl daemon-reload failed, so systemd may still use the old state: %v: %s", err, strings.TrimSpace(string(out)))
		return nil
	}
	resp.Applied = true
	return nil
}
