//go:build linux

package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"syscall"
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
//   - its traffic folder, owned by root at a mode CelikPanel set (0755, or
//     0711 from a build before the independent review), becomes 0710 with
//     the web server's group: its workers pass, nobody else does (review
//     of PR #355, finding 1: at 0711 every local account could open
//     another site's log by name);
//   - a traffic log readable by others (nginx made the first one at its
//     own 0644) loses the others' bits (0644 -> 0640);
//   - the certificate tool's log folder is not touched (0700) and its mode
//     is said in the journal.
//
// Any other mode or owner is the server owner's: kept and named. A receipt
// in the Agent's private state directory records that it ran, so a mode the
// owner sets later is never changed back (D-022).
//
// CelikPanel günlük klasörünün bir kerelik düzeltmesi: web sunucusunun
// işçileri trafik günlüklerine geçebilir; sahibin kipi korunur.

// v2: the review's correction (a server that ran v1 in a lab still gets
// the traffic folder closed).
const logRootModesReceiptName = "log-root-modes-v2.done"

// logRootPanelModes are the log folder's modes CelikPanel produced, and what
// each becomes.
var logRootPanelModes = map[os.FileMode]os.FileMode{0o700: 0o711, 0o750: 0o751}

// passLogRootOnce is the production entry point.
func passLogRootOnce() {
	root := services.CelikPanelLogRoot()
	receipt := filepath.Join(serviceMutationStateDirectory(), logRootModesReceiptName)
	_, webGID, ok := services.WebServerIDs()
	if !ok {
		log.Print("log folder modes: the web server's group could not be read; nothing changed, tried again at the next start")
		return
	}
	lines, _ := passLogRoot(receipt, root, services.TrafficLogDir(), filepath.Join(root, "certbot"), webGID, time.Now)
	for _, line := range lines {
		log.Print("log folder modes: " + line)
	}
}

// passLogRoot runs the correction unless the receipt exists; ran=false: it
// had run before (or its receipt could not be read), nothing was looked at.
func passLogRoot(receipt, root, traffic, certbot string, webGID int, now func() time.Time) ([]string, bool) {
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
	trafficLine, trafficDid, trafficOurs := correctTrafficFolder(traffic, webGID)
	count(trafficLine, trafficDid)
	if trafficOurs {
		count(closeTrafficLogs(traffic))
	}
	if info, err := os.Lstat(certbot); err == nil {
		lines = append(lines, fmt.Sprintf("left %s at %04o (the certificate tool's logs stay closed)", certbot, uint32(info.Mode().Perm())))
	}
	if held := logRootHolds(root); held != "" {
		lines = append(lines, held)
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

// correctTrafficFolder makes the traffic folder 0710 root:<web server's
// group> from a mode CelikPanel set (0755, 0711, or 0710 with another
// group). ours: the folder is CelikPanel's real folder (now or already at
// the closed form), so its logs may be closed too. A link, another owner or
// another mode is the owner's: kept and named.
func correctTrafficFolder(dir string, webGID int) (string, bool, bool) {
	info, err := os.Lstat(dir)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, false
	}
	if err != nil {
		return fmt.Sprintf("kept %s: %v", dir, err), false, false
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Sprintf("kept %s: not a directory (a link the owner made is the owner's)", dir), false, false
	}
	if !rootOwnedInfo(info) {
		return fmt.Sprintf("kept %s: not owned by root; the owner's choice", dir), false, false
	}
	perm := info.Mode().Perm()
	gid := -1
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		gid = int(stat.Gid)
	}
	if perm == services.TrafficLogDirMode && gid == webGID {
		return "", false, true
	}
	if perm != 0o755 && perm != 0o711 && perm != services.TrafficLogDirMode {
		return fmt.Sprintf("kept %s: mode %04o is the owner's choice", dir, uint32(perm)), false, false
	}
	if err := os.Lchown(dir, 0, webGID); err != nil {
		return fmt.Sprintf("kept %s: %v", dir, err), false, false
	}
	if err := os.Chmod(dir, services.TrafficLogDirMode); err != nil {
		return fmt.Sprintf("kept %s: %v", dir, err), false, false
	}
	return fmt.Sprintf("%04o -> %04o (group of the web server's workers) %s", uint32(perm), uint32(services.TrafficLogDirMode), dir), true, true
}

// closeTrafficLogs takes the others' bits off every traffic log (current and
// rotated) that has them; links are never followed.
func closeTrafficLogs(dir string) (string, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Sprintf("kept the logs in %s: %v", dir, err), false
	}
	closed := 0
	for _, entry := range entries {
		info, err := os.Lstat(filepath.Join(dir, entry.Name()))
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o007 == 0 {
			continue
		}
		if err := os.Chmod(filepath.Join(dir, entry.Name()), info.Mode().Perm()&^0o007); err == nil {
			closed++
		}
	}
	if closed == 0 {
		return "", false
	}
	return fmt.Sprintf("%d traffic logs in %s closed to other accounts (others' bits removed, e.g. 0644 -> 0640)", closed, dir), true
}

// logRootHolds says what the log folder holds, with each entry's mode.
func logRootHolds(root string) string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return ""
	}
	var held []string
	for _, entry := range entries {
		if info, err := os.Lstat(filepath.Join(root, entry.Name())); err == nil {
			held = append(held, fmt.Sprintf("%s (%04o)", entry.Name(), uint32(info.Mode().Perm())))
		}
	}
	return fmt.Sprintf("%s holds: %s", root, strings.Join(held, ", "))
}
