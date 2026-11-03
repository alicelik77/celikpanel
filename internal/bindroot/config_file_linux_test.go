//go:build linux

package bindroot

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestReadExactBINDConfigAtOwnerModesAndSymlinks(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root-owned native config fixture requires root")
	}
	root := t.TempDir()
	if err := os.Chmod(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "etc/bind"), 0o755); err != nil {
		t.Fatal(err)
	}
	aptPath := filepath.Join(root, "etc/bind/named.conf.local")
	if err := os.WriteFile(aptPath, []byte("// managed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(aptPath, 0, 12345); err != nil {
		t.Fatal(err)
	}
	fd, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(fd)
	if raw, identity, err := ReadExactBINDConfigAt(fd, APT, 12345, "/etc/bind/named.conf.local"); err != nil || string(raw) != "// managed\n" || identity.GID != 12345 {
		t.Fatalf("APT group-owned config rejected: %q %+v %v", raw, identity, err)
	}
	if _, _, err := ReadExactBINDConfigAt(fd, APT, 12346, "/etc/bind/named.conf.local"); err == nil {
		t.Fatal("foreign APT owner accepted")
	}
	if _, _, err := ReadExactBINDConfigAt(fd, APT, 12345, "/etc/passwd"); err == nil {
		t.Fatal("arbitrary APT path accepted")
	}
	if err := os.Chmod(aptPath, 0o664); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadExactBINDConfigAt(fd, APT, 12345, "/etc/bind/named.conf.local"); err == nil {
		t.Fatal("writable APT config accepted")
	}
	if err := os.Remove(aptPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc/passwd", aptPath); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadExactBINDConfigAt(fd, APT, 12345, "/etc/bind/named.conf.local"); err == nil {
		t.Fatal("symlinked APT config accepted")
	}
	archPath := filepath.Join(root, "etc/named.conf")
	if err := os.WriteFile(archPath, []byte("// managed\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(archPath, 0, 12345); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadExactBINDConfigAt(fd, Pacman, 12345, "/etc/named.conf"); err != nil {
		t.Fatalf("pacman config rejected: %v", err)
	}
	if err := os.Chown(archPath, 0, 0); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadExactBINDConfigAt(fd, Pacman, 12345, "/etc/named.conf"); err == nil {
		t.Fatal("root:root pacman config accepted")
	}
}

func TestReadExactBINDConfigAtCertifiedDebianSetgidParent(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root-owned native config fixture requires root")
	}
	root := t.TempDir()
	if err := os.Chmod(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "etc", "bind"), 0o755); err != nil {
		t.Fatal(err)
	}
	parent := filepath.Join(root, "etc", "bind")
	file := filepath.Join(parent, "named.conf.default-zones")
	if err := os.Chown(parent, 0, 12345); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(parent, 0o2755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("// defaults\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(file, 0, 12345); err != nil {
		t.Fatal(err)
	}
	fd, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(fd)
	read := func() error {
		_, _, err := ReadExactBINDConfigAt(fd, APT, 12345, "/etc/bind/named.conf.default-zones")
		return err
	}
	if err := read(); err != nil {
		t.Fatalf("root:bind 2755 parent refused: %v", err)
	}
	if _, _, err := ReadExactRootOwnedFileAt(fd, "/etc/bind/named.conf.default-zones", "generic root file"); err == nil {
		t.Fatal("generic vendor reader accepted BIND-specific parent")
	}
	if err := os.Chown(parent, 0, 12346); err != nil {
		t.Fatal(err)
	}
	if err := read(); err == nil {
		t.Fatal("foreign parent group accepted")
	}
	if err := os.Chown(parent, 0, 12345); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(parent, 0o2775); err != nil {
		t.Fatal(err)
	}
	if err := read(); err == nil {
		t.Fatal("group-writable parent accepted")
	}
	if err := os.Chmod(parent, 0o2755); err != nil {
		t.Fatal(err)
	}
	if err := read(); err != nil {
		t.Fatalf("safe parent not restored: %v", err)
	}
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(parent); err != nil {
		t.Fatal(err)
	}
	actual := filepath.Join(root, "etc", "bind-real")
	if err := os.Mkdir(actual, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(actual, 0, 12345); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(actual, 0o2755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(actual, "named.conf.default-zones"), []byte("// defaults\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(filepath.Join(actual, "named.conf.default-zones"), 0, 12345); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(actual, parent); err != nil {
		t.Fatal(err)
	}
	if err := read(); err == nil {
		t.Fatal("symlinked BIND parent accepted")
	}
}

// Each read closes every descriptor it opened exactly once (alpha.118 lab:
// the walk closed its first descriptor number a second time at return, which
// could close a file another goroutine had just opened under that number,
// and leaked its last directory).
func TestReadExactFileAtClosesEveryDescriptorOnce(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root-owned native config fixture requires root")
	}
	root := t.TempDir()
	if err := os.Chmod(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "etc"), 0o755); err != nil {
		t.Fatal(err)
	}
	conf := filepath.Join(root, "etc/named.conf")
	if err := os.WriteFile(conf, []byte("// managed\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(conf, 0, 12345); err != nil {
		t.Fatal(err)
	}
	fd, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(fd)
	openDescriptors := func() int {
		entries, err := os.ReadDir("/proc/self/fd")
		if err != nil {
			t.Fatal(err)
		}
		return len(entries)
	}
	// A descriptor of this test that must survive every read.
	sentinel, err := os.Open(conf)
	if err != nil {
		t.Fatal(err)
	}
	defer sentinel.Close()
	before := openDescriptors()
	for i := 0; i < 20; i++ {
		if _, _, err := ReadExactBINDConfigAt(fd, Pacman, 12345, "/etc/named.conf"); err != nil {
			t.Fatalf("read %d: %v", i, err)
		}
		if _, _, err := ReadExactBINDConfigAt(fd, Pacman, 12345, "/etc/missing/named.conf"); err == nil {
			t.Fatal("an unsupported path was read")
		}
		if _, _, err := ReadExactRootOwnedFileAt(fd, "/etc/missing/unit.service", "missing unit"); err == nil {
			t.Fatal("a missing file was read")
		}
	}
	if after := openDescriptors(); after > before+2 {
		t.Fatalf("descriptors leaked: %d before, %d after 60 reads", before, after)
	}
	if _, err := sentinel.Stat(); err != nil {
		t.Fatalf("a descriptor the reads did not own was closed: %v", err)
	}
}
