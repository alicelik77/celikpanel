package main

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// ensureNginxDropInDirectory gives a drop-in directory CelikPanel creates
// (Arch's nginx package ships none) the shape the distribution's own
// directories have on Debian: root:root 0755. The Agent runs as root with the
// celikpanel group, so a plain MkdirAll made root:celikpanel directories, and
// the certificate renewal readiness, which trusts only root:root parents,
// then reported "renewal required" for the panel and mail certificates on
// every Arch server (Arch VM, 2026-10-30). A directory an earlier CelikPanel
// created that way (root:<agent group>, not group-writable) is given back to
// root:root; any other owner's directory is left as it is.
// CelikPanel'in oluşturduğu drop-in dizini root:root 0755 olur; daha önce
// root:celikpanel olarak oluşturduğu dizin root:root'a döner, başkasınınki kalır.
var nginxDropInChown = os.Lchown

func ensureNginxDropInDirectory(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.Mkdir(path, 0o755); err != nil && !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("mkdir %s: %w", path, err)
		}
		if err := nginxDropInChown(path, 0, 0); err != nil {
			return fmt.Errorf("own %s by root: %w", path, err)
		}
		if err := os.Chmod(path, 0o755); err != nil {
			return fmt.Errorf("chmod %s: %w", path, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect %s: %w", path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s exists but is not a directory; CelikPanel left it unchanged", path)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if ok && stat.Uid == 0 && stat.Gid != 0 && int(stat.Gid) == os.Getegid() && info.Mode().Perm()&0o022 == 0 {
		if err := nginxDropInChown(path, 0, 0); err != nil {
			return fmt.Errorf("own %s by root: %w", path, err)
		}
	}
	return nil
}
