//go:build linux

package main

import (
	"errors"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// publishNativeSiteDeployHook writes the deploy hook root-owned 0755 into
// certbot's own renewal-hooks/deploy directory, only when no file of that
// name exists (an owner's file is never replaced).
func publishNativeSiteDeployHook() error {
	dir := filepath.Dir(nativeSiteHookPath)
	if err := ensureRootOwnedPanelCertHookDirectory(dir); err != nil {
		return err
	}
	err := publishPanelCertDeployHookAbsent(dir, filepath.Base(nativeSiteHookPath), []byte(nativeSiteDeployHook))
	if errors.Is(err, unix.EEXIST) {
		return nil
	}
	return err
}
