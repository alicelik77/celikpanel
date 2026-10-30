package pacmanlocal

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func gz(t *testing.T, text string) []byte {
	t.Helper()
	var out bytes.Buffer
	w := gzip.NewWriter(&out)
	if _, err := w.Write([]byte(text)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// The measured shape of Arch's powerdns 5.1.5-1 local database entry
// (2026-10-30, lab guest).
const measuredDesc = "%NAME%\npowerdns\n\n%VERSION%\n5.1.5-1\n\n%BASE%\npowerdns\n\n%DESC%\nAuthoritative DNS server\n"

const measuredMtree = "#mtree\n/set type=file uid=0 gid=0 mode=644\n" +
	"./usr/bin/pdns_server time=1792587573.0 mode=755 size=5573232 sha256digest=cb331136ed552c8ef0a3de82ac90732339a6e37d6e9ffebde44346b12c44ad0c\n" +
	"./usr/lib/systemd/system/pdns.service time=1792587573.0 size=1586 sha256digest=4bc3129b997154908bb65f81c021ea965037178367b6f319c68865bb3097d420\n"

func TestParseDescAndVersion(t *testing.T) {
	name, version, err := ParseDesc([]byte(measuredDesc))
	if err != nil || name != "powerdns" || version != "5.1.5-1" {
		t.Fatalf("got %q %q %v", name, version, err)
	}
	for _, bad := range []string{"", "%NAME%\npowerdns\n", "%NAME%\npowerdns\n%NAME%\nx\n%VERSION%\n1-1\n", "%NAME%\nPowerDNS\n%VERSION%\n1-1\n", "%NAME%\npowerdns\n%VERSION%\n5.1.5\n"} {
		if _, _, err := ParseDesc([]byte(bad)); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	for version, want := range map[string]bool{"5.1.5-1": true, "5.1.5-1.1": true, "1:5.1.5-2": true, "5.1.5": false, "5.1.5-x": false, "../5-1": false, "": false} {
		if got := ValidVersion(version); got != want {
			t.Errorf("%q: got %v want %v", version, got, want)
		}
	}
}

func TestMtreeSHA256(t *testing.T) {
	digest, err := MtreeSHA256(gz(t, measuredMtree), "/usr/bin/pdns_server")
	if err != nil || digest != "cb331136ed552c8ef0a3de82ac90732339a6e37d6e9ffebde44346b12c44ad0c" {
		t.Fatalf("got %q %v", digest, err)
	}
	for name, text := range map[string]string{
		"missing":   "#mtree\n./usr/bin/other sha256digest=" + string(bytes.Repeat([]byte("a"), 64)) + "\n",
		"duplicate": measuredMtree + "./usr/bin/pdns_server sha256digest=" + string(bytes.Repeat([]byte("b"), 64)) + "\n",
		"nodigest":  "./usr/bin/pdns_server time=1 size=2\n",
		"bad":       "./usr/bin/pdns_server sha256digest=XYZ\n",
	} {
		if _, err := MtreeSHA256(gz(t, text), "/usr/bin/pdns_server"); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if _, err := MtreeSHA256([]byte("not gzip"), "/usr/bin/pdns_server"); err == nil {
		t.Error("plain bytes accepted as mtree")
	}
}

func TestInstalledAndVerifyPackageFile(t *testing.T) {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		t.Skip("the local database reader requires root-owned files on Linux")
	}
	db := t.TempDir()
	if err := os.Chmod(db, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "pdns_server")
	if err := os.WriteFile(file, []byte("daemon"), 0o755); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("daemon"))
	entry := filepath.Join(db, "powerdns-5.1.5-1")
	if err := os.Mkdir(entry, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(entry, "desc"), []byte(measuredDesc), 0o644); err != nil {
		t.Fatal(err)
	}
	mtree := "#mtree\n." + file + " sha256digest=" + hex.EncodeToString(sum[:]) + "\n"
	if err := os.WriteFile(filepath.Join(entry, "mtree"), gz(t, mtree), 0o644); err != nil {
		t.Fatal(err)
	}
	// A package whose name only starts the same is not confused with it.
	other := filepath.Join(db, "powerdns-admin-0.4-1")
	if err := os.Mkdir(other, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, "desc"), []byte("%NAME%\npowerdns-admin\n\n%VERSION%\n0.4-1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pkg, err := Installed(db, "powerdns")
	if err != nil || pkg.Version != "5.1.5-1" || pkg.Dir != entry {
		t.Fatalf("got %+v %v", pkg, err)
	}
	if err := VerifyPackageFile(pkg, file); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("changed"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := VerifyPackageFile(pkg, file); err == nil {
		t.Fatal("a changed daemon was accepted as the package's own file")
	}
	if _, err := Installed(db, "bind"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("absent package: %v", err)
	}
	if err := os.Chmod(filepath.Join(entry, "desc"), 0o666); err != nil {
		t.Fatal(err)
	}
	if _, err := Installed(db, "powerdns"); err == nil {
		t.Fatal("a world-writable package record was trusted")
	}
}
