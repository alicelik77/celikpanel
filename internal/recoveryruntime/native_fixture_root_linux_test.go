//go:build linux

package recoveryruntime

import (
	"os"
	"testing"

	"golang.org/x/sys/unix"
)

// nativeFixtureRoot is the private root of one native root fixture: a fresh
// directory under /run, whose root-owned 0755 ancestry the trusted-path checks
// require, with the fixture's own small tmpfs mounted on it.
//
// The fixtures install and execute launchers, checkers and shims inside that
// root. /run itself is mounted without execute permission on some hosts
// (alpha.135: every GitHub ubuntu-24.04 job served from AWS, image
// 20261111.127.1, failed with "fork/exec /run/.../libexec/recovery:
// permission denied", while every job served from Azure passed; Debian 13 and
// Ubuntu's initramfs mount /run noexec too). The product never executes from
// /run; deploy/test-private-tmpfs.sh gives the shell contract tests the same
// private mount since alpha.122. The mount keeps every fixture independent of
// how the host mounted /run and of every other test process; it is removed
// with the fixture.
//
// Sabit /run dizini bazı makinelerde çalıştırmaya kapalıdır; her fikstür kendi
// küçük tmpfs'ini bağlar ve sonunda kaldırır. Ürün /run altından çalıştırmaz.
func nativeFixtureRoot(t *testing.T, prefix string) string {
	t.Helper()
	root, err := os.MkdirTemp("/run", prefix)
	if err != nil {
		t.Fatal(err)
	}
	mountErr := unix.Mount("celikpanel-test", root, "tmpfs", unix.MS_NOSUID|unix.MS_NODEV, "mode=0700,size=512m")
	t.Cleanup(func() {
		if mountErr == nil {
			if err := unix.Unmount(root, 0); err != nil {
				unix.Unmount(root, unix.MNT_DETACH)
			}
		}
		os.RemoveAll(root)
	})
	// Without the mount (no mount privilege) the directory serves only where
	// the host's /run already allows execution; a fixture never runs where its
	// executables would be refused for a reason unrelated to the contract.
	var fs unix.Statfs_t
	if err := unix.Statfs(root, &fs); err != nil {
		t.Fatal(err)
	}
	if fs.Flags&unix.ST_NOEXEC != 0 {
		t.Fatalf("native fixture root %s does not allow execution (private tmpfs mount: %v)", root, mountErr)
	}
	return root
}
