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

	"github.com/alicelik/celikpanel/internal/hostingpath"
	"github.com/alicelik/celikpanel/internal/services"
)

// The one-time correction of modes that let a site account learn about other
// sites (20 Nov 2026, tenant isolation; services/site_file_modes.go):
//
//   - CelikPanel's per-site nginx and PHP-FPM files still at the 0644 earlier
//     releases wrote become 0640;
//   - the hosting base and the subscriptions root, at the 0755 earlier
//     releases set, become 0711: site accounts still pass through them, but
//     cannot list the subscriptions.
//
// It runs at the Agent's start, before the RPC service, once: a receipt in the
// Agent's private state directory records that it ran, so a mode the owner
// chooses later is never changed back (D-022). Any mode other than the one
// earlier releases wrote is the owner's and is kept, and said in the log.
//
// Diğer siteleri öğrenmeye yarayan kiplerin bir kerelik düzeltmesi: yalnız
// eski sürümlerin yazdığı kipler değişir, sahibin seçtiği kip korunur;
// makbuz yazıldıktan sonra bir daha çalışmaz.

const tenantFileModesReceiptName = "tenant-file-modes-v1.done"

// legacyHostingBaseMode is the mode earlier releases set on the hosting base
// and the subscriptions root at every site creation.
const legacyHostingBaseMode os.FileMode = 0o755

// tightenTenantVisibleModesOnce is the production entry point.
func tightenTenantVisibleModesOnce() {
	receipt := filepath.Join(serviceMutationStateDirectory(), tenantFileModesReceiptName)
	lines, ran := tightenTenantVisibleModes(receipt, services.TightenManagedSiteFileModes,
		[]string{hostingpath.HostingBase(), hostingpath.SubscriptionsRoot()}, time.Now)
	if ran || len(lines) > 0 {
		for _, line := range lines {
			log.Print("tenant file modes: " + line)
		}
	}
}

// rootOwnedInfo: the entry belongs to uid 0.
func rootOwnedInfo(info os.FileInfo) bool {
	uid, ok := mailTLSFileOwner(info)
	return ok && uid == 0
}

// tightenTenantVisibleModes runs the correction unless the receipt exists and
// returns its log lines; ran=false: it had run before (or its receipt could
// not be read), nothing was looked at.
func tightenTenantVisibleModes(
	receipt string,
	siteFiles func() []services.ManagedFileModeChange,
	hostingDirs []string,
	now func() time.Time,
) ([]string, bool) {
	if _, err := os.Lstat(receipt); err == nil {
		return nil, false
	} else if !errors.Is(err, os.ErrNotExist) {
		return []string{fmt.Sprintf("receipt %s unreadable (%v); nothing changed", receipt, err)}, false
	}
	var lines []string
	changed, kept := 0, 0
	for _, change := range siteFiles() {
		if change.Kept {
			kept++
			lines = append(lines, "kept "+change.Path+": "+change.Why)
			continue
		}
		changed++
		lines = append(lines, "0644 -> 0640 "+change.Path)
	}
	for _, dir := range hostingDirs {
		line, did := tightenHostingDirMode(dir)
		if line == "" {
			continue
		}
		if did {
			changed++
		} else {
			kept++
		}
		lines = append(lines, line)
	}
	text := fmt.Sprintf("ran %s: %d changed, %d kept\n", now().UTC().Format(time.RFC3339), changed, kept)
	if err := writeTenantModesReceipt(receipt, text); err != nil {
		lines = append(lines, fmt.Sprintf("receipt %s not written (%v); the correction runs again at the next start", receipt, err))
	}
	return append(lines, strings.TrimSpace(text)), true
}

// tightenHostingDirMode turns the hosting base or the subscriptions root from
// the earlier releases' 0755 into hostingBaseMode. "" when there is nothing to
// say (absent, or already not listable by others).
func tightenHostingDirMode(dir string) (string, bool) {
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
	if perm&0o004 == 0 {
		return "", false
	}
	if perm != legacyHostingBaseMode || !rootOwnedInfo(info) {
		return fmt.Sprintf("kept %s: mode %04o is the owner's choice", dir, uint32(perm)), false
	}
	if err := os.Chmod(dir, hostingBaseMode); err != nil {
		return fmt.Sprintf("kept %s: %v", dir, err), false
	}
	return fmt.Sprintf("0755 -> 0711 %s", dir), true
}

// writeTenantModesReceipt creates the receipt (0600, never through a link) in
// the Agent's private state directory.
func writeTenantModesReceipt(path, text string) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return err
	}
	_, writeErr := file.WriteString(text)
	syncErr := file.Sync()
	return errors.Join(writeErr, syncErr, file.Close())
}
