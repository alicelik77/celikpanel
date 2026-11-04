//go:build linux

package sandboxbind

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

// TestRealBindHoldsTheFolder mounts for real. It runs only as root inside a
// private mount namespace, with CELIKPANEL_SANDBOXBIND_MOUNT_TEST=1:
//
//	CELIKPANEL_SANDBOXBIND_MOUNT_TEST=1 unshare --mount --propagation private ./sandboxbind.test -test.run RealBind
//
// It binds a site's app folder through the helper, swaps the folder for a
// link to another site, and checks that the mount still shows the original
// folder, that a new start is refused, and that a stop removes the mount.
func TestRealBindHoldsTheFolder(t *testing.T) {
	if os.Getenv("CELIKPANEL_SANDBOXBIND_MOUNT_TEST") != "1" || os.Geteuid() != 0 {
		t.Skip("needs root in a private mount namespace (see the comment)")
	}
	for _, legacy := range []bool{false, true} {
		name := "new mount API"
		if legacy {
			name = "mount(2) fallback"
		}
		t.Run(name, func(t *testing.T) {
			tr := newTree(t)
			const site = 4242
			for _, dir := range []string{tr.home, tr.app, filepath.Join(tr.app, "config")} {
				if err := os.Chown(dir, site, site); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Chown(filepath.Join(tr.app, "config", "app.env"), site, site); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(tr.app, "marker"), []byte("own folder"), 0o644); err != nil {
				t.Fatal(err)
			}
			other := filepath.Join(t.TempDir(), "other-site")
			if err := os.MkdirAll(other, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(other, "secret.txt"), []byte("another site"), 0o644); err != nil {
				t.Fatal(err)
			}
			var mounter Mounter = KernelMounter{}
			if legacy {
				mounter = legacyMounter{}
			}
			h := tr.helper(site)
			h.Mounter = mounter
			h.RootUID = 0
			req := startReq()
			if err := h.Start(req); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(tr.mountRoot, req.Unit, "app")
			read := func() string {
				b, _ := os.ReadFile(filepath.Join(target, "marker"))
				return string(b)
			}
			if read() != "own folder" {
				t.Fatalf("the mount does not show the app folder: %q", read())
			}
			var st unix.Statfs_t
			if err := unix.Statfs(target, &st); err != nil || st.Flags&unix.ST_NOSUID == 0 || st.Flags&unix.ST_NODEV == 0 {
				t.Fatalf("the mount is not nosuid,nodev: flags=%#x %v", st.Flags, err)
			}
			// The customer swaps the folder for a link to another site.
			if err := os.Rename(tr.app, tr.app+".moved"); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(other, tr.app); err != nil {
				t.Fatal(err)
			}
			if read() != "own folder" {
				t.Fatalf("after the swap the mount shows %q", read())
			}
			if _, err := os.Stat(filepath.Join(target, "secret.txt")); !os.IsNotExist(err) {
				t.Fatalf("the other site is visible through the mount: %v", err)
			}
			// The next start is refused.
			err := h.Start(req)
			if r, ok := AsRefusal(err); !ok || r.Code != CodeNotReal {
				t.Fatalf("a start after the swap: %v", err)
			}
			if err := h.Stop(req.Unit); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Lstat(target); !os.IsNotExist(err) {
				t.Fatalf("the mount point is still there after the stop: %v", err)
			}
			// The folder put back starts again.
			os.Remove(tr.app)
			if err := os.Rename(tr.app+".moved", tr.app); err != nil {
				t.Fatal(err)
			}
			if err := h.Start(req); err != nil {
				t.Fatal(err)
			}
			if read() != "own folder" {
				t.Fatalf("after the folder came back: %q", read())
			}
			if err := h.Stop(req.Unit); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// legacyMounter forces the mount(2) fallback.
type legacyMounter struct{ KernelMounter }

func (legacyMounter) Bind(fd int, target string) error {
	if err := legacyBind(fd, target); err != nil {
		return err
	}
	if err := sameFolder(fd, target); err != nil {
		_ = unix.Unmount(target, unix.MNT_DETACH)
		return err
	}
	return nil
}
