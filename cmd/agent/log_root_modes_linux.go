//go:build linux

package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/services"
)

// The one-time correction of CelikPanel's log folder (alpha.144; the owner's
// server read on 22 November 2026: /var/log/celikpanel 0750 root:celikpanel,
// its traffic folder 0755, nginx's workers as www-data outside that group).
// The workers reopen the traffic logs by name after each nightly rotation and
// could not pass through the folder, so each logged "[emerg] open() …
// failed (13: Permission denied)" and kept writing into the rotated file
// until the next reload. Same style as the tenant file modes
// (tenant_file_modes_linux.go):
//
//   - /var/log/celikpanel, owned by root at a mode CelikPanel itself produced
//     (0700, 0750), gets pass-through for others: 0711, 0751 (the group's
//     bits are kept);
//   - its traffic folder, owned by root at the 0755 CelikPanel set, becomes
//     0711, so the pass-through does not let anybody list the traffic logs;
//   - the certificate tool's log folder is not touched (0700) and its mode
//     is said in the journal.
//
// Any other mode or owner is the server owner's: kept and named. A receipt
// in the Agent's private state directory records that it ran, so a mode the
// owner sets later is never changed back (D-022).
//
// CelikPanel günlük klasörünün bir kerelik düzeltmesi: web sunucusunun
// işçileri trafik günlüklerine geçebilir; sahibin kipi korunur.

const logRootModesReceiptName = "log-root-modes-v1.done"

// logRootPanelModes are the log folder's modes CelikPanel produced, and what
// each becomes.
var logRootPanelModes = map[os.FileMode]os.FileMode{0o700: 0o711, 0o750: 0o751}

// passLogRootOnce is the production entry point.
func passLogRootOnce() {
	root := services.CelikPanelLogRoot()
	receipt := filepath.Join(serviceMutationStateDirectory(), logRootModesReceiptName)
	lines, _ := passLogRoot(receipt, root, services.TrafficLogDir(), filepath.Join(root, "certbot"), time.Now)
	for _, line := range lines {
		log.Print("log folder modes: " + line)
	}
}

// passLogRoot runs the correction unless the receipt exists; ran=false: it
// had run before (or its receipt could not be read), nothing was looked at.
func passLogRoot(receipt, root, traffic, certbot string, now func() time.Time) ([]string, bool) {
	if _, err := os.Lstat(receipt); err == nil {
		return nil, false
	} else if !errors.Is(err, os.ErrNotExist) {
		return []string{fmt.Sprintf("receipt %s unreadable (%v); nothing changed", receipt, err)}, false
	}
	var lines []string
	changed, kept := 0, 0
	count := func(line string, did bool) {
		if line == "" {
			return
		}
		if did {
			changed++
		} else {
			kept++
		}
		lines = append(lines, line)
	}
	count(correctPanelDirMode(root, logRootPanelModes))
	count(correctPanelDirMode(traffic, map[os.FileMode]os.FileMode{0o755: services.TrafficLogDirMode}))
	if info, err := os.Lstat(certbot); err == nil {
		lines = append(lines, fmt.Sprintf("left %s at %04o (the certificate tool's logs stay closed)", certbot, uint32(info.Mode().Perm())))
	}
	text := fmt.Sprintf("ran %s: %d changed, %d kept\n", now().UTC().Format(time.RFC3339), changed, kept)
	if err := writeTenantModesReceipt(receipt, text); err != nil {
		lines = append(lines, fmt.Sprintf("receipt %s not written (%v); the correction runs again at the next start", receipt, err))
	}
	return append(lines, strings.TrimSpace(text)), true
}

// correctPanelDirMode changes dir from a mode CelikPanel produced to its
// corrected mode. "" when there is nothing to say (absent, or already at a
// corrected mode). Any other mode, a link or another owner is the owner's.
func correctPanelDirMode(dir string, modes map[os.FileMode]os.FileMode) (string, bool) {
	info, err := os.Lstat(dir)
	if errors.Is(err, os.ErrNotExist) {
		return "", false
	}
	if err != nil {
		return fmt.Sprintf("kept %s: %v", dir, err), false
	}
	if !info.IsDir() {
		return fmt.Sprintf("kept %s: not a directory (a link the owner made is the owner's)", dir), false
	}
	perm := info.Mode().Perm()
	for _, corrected := range modes {
		if perm == corrected {
			return "", false
		}
	}
	next, ours := modes[perm]
	if !ours {
		return fmt.Sprintf("kept %s: mode %04o is the owner's choice", dir, uint32(perm)), false
	}
	if !rootOwnedInfo(info) {
		return fmt.Sprintf("kept %s: not owned by root; the owner's choice", dir), false
	}
	if err := os.Chmod(dir, next); err != nil {
		return fmt.Sprintf("kept %s: %v", dir, err), false
	}
	return fmt.Sprintf("%04o -> %04o %s", uint32(perm), uint32(next), dir), true
}
