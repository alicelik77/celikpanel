//go:build linux

package services

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/alicelik/celikpanel/internal/transport"
)

// set10 scenario g on the running kernel: an immutable file is measured as
// such without writing, and replacing it by a rename is refused with EPERM,
// the name the read reports. Needs root and a file system with attribute
// flags; skipped otherwise.
func TestMeasureSiteFileProtectionReadsTheImmutableFlag(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "locked.example.conf")
	writeTestFile(t, path, "server {}\n", 0o644)
	if protection, systemError := measureSiteFileProtection(path); protection != transport.SiteFileProtectionNone || systemError != "" {
		if protection == "" {
			t.Skip("attribute flags cannot be read here")
		}
		t.Fatalf("an ordinary file: %q %q", protection, systemError)
	}
	setFlags := func(flags int) error {
		fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if err != nil {
			return err
		}
		defer unix.Close(fd)
		return unix.IoctlSetPointerInt(fd, unix.FS_IOC_SETFLAGS, flags)
	}
	if err := setFlags(inodeFlagImmutable); err != nil {
		t.Skipf("cannot set the immutable flag here: %v", err)
	}
	t.Cleanup(func() { _ = setFlags(0) })
	protection, systemError := measureSiteFileProtection(path)
	if protection != transport.SiteFileProtectionImmutable || systemError != "EPERM" {
		t.Fatalf("immutable file: %q %q", protection, systemError)
	}
	replacement := filepath.Join(directory, ".replacement")
	writeTestFile(t, replacement, "server { listen 80; }\n", 0o644)
	err := os.Rename(replacement, path)
	if err == nil || siteFileErrnoName(err) != systemError {
		t.Fatalf("rename over the immutable file: %v (%s)", err, siteFileErrnoName(err))
	}
}
