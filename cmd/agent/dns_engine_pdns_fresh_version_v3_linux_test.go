//go:build linux

package main

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/dnsengineartifact"
	"golang.org/x/sys/unix"
)

// Item 4a: the version is read before any effect. The step accepts the
// distribution's current release of the tested PowerDNS series (Beta-1 VM
// cell b: Debian 13 offered 4.9.18 while the step accepted only 4.9.17 and
// rolled back), one build for both packages, and refuses another series with
// what the owner can do.
func TestFreshPDNSVersionPreflightAcceptsTheTestedSeries(t *testing.T) {
	packages := []string{"pdns-server", "pdns-backend-sqlite3"}
	const measured, patched = "4.9.17-0+deb13u1", "4.9.18-0+deb13u1"
	debian := []dnsengineartifact.PDNSArchive{{Origin: "Debian", Suite: "stable"}, {Origin: "Debian-Security", Suite: "stable-security"}}
	archives := debian
	var archivesErr error
	ops := func(installed, candidate map[string]string) freshPDNSVersionOpsV3 {
		return freshPDNSVersionOpsV3{
			archives: func(string, string) ([]dnsengineartifact.PDNSArchive, error) { return archives, archivesErr },
			installed: func(name string) (string, error) {
				if v, ok := installed[name]; ok {
					return v, nil
				}
				return "", errors.New("not installed")
			},
			candidate: func(name string) (string, error) {
				if v, ok := candidate[name]; ok {
					return v, nil
				}
				return "", errors.New("apt-cache policy failed")
			},
		}
	}
	for _, version := range []string{measured, patched} {
		got, err := preflightFreshPDNSPackageVersionsWithOpsV3(packages, packages,
			ops(nil, map[string]string{"pdns-server": version, "pdns-backend-sqlite3": version}))
		if err != nil || got != version {
			t.Fatalf("the tested-series candidate %s was refused: %q %v", version, got, err)
		}
	}
	if got, err := preflightFreshPDNSPackageVersionsWithOpsV3(packages, []string{"pdns-backend-sqlite3"},
		ops(map[string]string{"pdns-server": patched}, map[string]string{"pdns-backend-sqlite3": patched})); err != nil || got != patched {
		t.Fatalf("an installed tested-series package was refused: %q %v", got, err)
	}
	for name, tc := range map[string]struct {
		missing   []string
		installed map[string]string
		candidate map[string]string
		want      []string
	}{
		"other series": {packages, nil, map[string]string{"pdns-server": "5.0.1-1", "pdns-backend-sqlite3": "5.0.1-1"},
			[]string{"would install pdns-server 5.0.1-1", "tested PowerDNS 4.9 series", "Choose BIND", "names 5.0.1-1 as tested", "nothing was changed"}},
		"pre-release": {packages, nil, map[string]string{"pdns-server": "4.9.19~rc1-1", "pdns-backend-sqlite3": "4.9.19~rc1-1"},
			[]string{"would install pdns-server 4.9.19~rc1-1"}},
		"two builds": {packages, nil, map[string]string{"pdns-server": patched, "pdns-backend-sqlite3": measured},
			[]string{"are not one PowerDNS build", "apt-get install --only-upgrade pdns-server pdns-backend-sqlite3"}},
		"no candidate": {packages, nil, map[string]string{"pdns-server": "", "pdns-backend-sqlite3": patched},
			[]string{"offers no installable pdns-server", "4.9 series"}},
		"unreadable candidate": {packages, nil, map[string]string{"pdns-server": patched},
			[]string{"could not read which pdns-backend-sqlite3 version", "apt-cache policy pdns-backend-sqlite3"}},
		"installed other version": {nil, map[string]string{"pdns-server": "4.8.4-1", "pdns-backend-sqlite3": patched}, nil,
			[]string{"pdns-server 4.8.4-1 is already installed", "PowerDNS 4.9 release"}},
		"powerdns repository build": {packages, nil, map[string]string{"pdns-server": "4.9.4-1pdns.trixie", "pdns-backend-sqlite3": "4.9.4-1pdns.trixie"},
			[]string{"would install pdns-server 4.9.4-1pdns.trixie"}},
		"backport": {packages, nil, map[string]string{"pdns-server": "4.9.4-1~bpo13+1", "pdns-backend-sqlite3": "4.9.4-1~bpo13+1"},
			[]string{"would install pdns-server 4.9.4-1~bpo13+1"}},
		"epoch": {packages, nil, map[string]string{"pdns-server": "1:4.9.18-0+deb13u1", "pdns-backend-sqlite3": "1:4.9.18-0+deb13u1"},
			[]string{"would install pdns-server 1:4.9.18-0+deb13u1"}},
		"installed vendor build": {nil, map[string]string{"pdns-server": "4.9.4-1pdns.trixie", "pdns-backend-sqlite3": "4.9.4-1pdns.trixie"}, nil,
			[]string{"is already installed"}},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := preflightFreshPDNSPackageVersionsWithOpsV3(packages, tc.missing, ops(tc.installed, tc.candidate))
			sentence := operatorSentenceOf(err)
			if err == nil || sentence == "" {
				t.Fatalf("mismatch was not refused with an operator sentence: %v", err)
			}
			for _, want := range tc.want {
				if !strings.Contains(sentence, want) {
					t.Fatalf("refusal lacks %q: %q", want, sentence)
				}
			}
		})
	}
	// Provenance: the Debian version from another archive, or an archive
	// that cannot be named, is refused before anything changes.
	both := map[string]string{"pdns-server": patched, "pdns-backend-sqlite3": patched}
	for name, tc := range map[string]struct {
		archives []dnsengineartifact.PDNSArchive
		err      error
		want     string
	}{
		"repo.powerdns.com": {[]dnsengineartifact.PDNSArchive{{Origin: "PowerDNS", Suite: "trixie-auth-49"}}, nil, "would come from PowerDNS trixie-auth-49"},
		"backports":         {[]dnsengineartifact.PDNSArchive{{Origin: "Debian", Suite: "stable-backports"}}, nil, "only from Debian's own archive"},
		"unreadable index":  {nil, nil, "an archive APT could not name"},
		"simulation failed": {nil, errors.New("apt-get -s failed"), "could not read which archive"},
	} {
		archives, archivesErr = tc.archives, tc.err
		_, err := preflightFreshPDNSPackageVersionsWithOpsV3(packages, packages, ops(nil, both))
		if sentence := operatorSentenceOf(err); err == nil || !strings.Contains(sentence, tc.want) || !strings.Contains(sentence, "nothing was changed") {
			t.Fatalf("%s: %v", name, err)
		}
	}
	archives, archivesErr = debian, nil
	sim := parseAptSimulation([]byte("Inst pdns-server (4.9.18-0+deb13u1 Debian:13.2/stable, Debian-Security:13/stable-security [amd64])"))
	if got := pdnsArchivesOf(sim, "pdns-server", "4.9.18-0+deb13u1"); !dnsengineartifact.PDNSOfficialDebianArchive(got) {
		t.Fatalf("archives of a simulated install: %+v", got)
	}
	if got := pdnsArchivesOf(sim, "pdns-server", "4.9.17-0+deb13u1"); got != nil {
		t.Fatalf("another version's archives: %+v", got)
	}
	if _, err := installFreshPDNSPackagesV3(context.Background(), []string{"pdns-server"}, "5.0.1-1"); err == nil {
		t.Fatal("an install outside the tested series was attempted")
	}
	// The refusal precedes every effect of the switch.
	raw, err := os.ReadFile("dns_engine_pdns_switch.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	start := strings.Index(source, "func switchToPDNSOnCertifiedProfile(")
	if start < 0 {
		t.Fatal("switchToPDNSOnCertifiedProfile is missing")
	}
	source = source[start:]
	if end := strings.Index(source[1:], "\nfunc "); end > 0 {
		source = source[:end+1]
	}
	preflight := strings.Index(source, "preflightFreshPDNSPackageVersionsV3(ctx, profile, packages, missing)")
	for _, effect := range []string{"publishDNSEngineSourceOwnership(\n", "installOwnedDNSEnginePackages(", "installFreshPDNSPackagesV3(ctx, missing, freshPDNSVersion)"} {
		if at := strings.Index(source, effect); preflight < 0 || at < preflight {
			t.Fatalf("%q is not after the V3 version preflight (%d, %d)", effect, at, preflight)
		}
	}
	if !validDebianVersion(patched) || validDebianVersion("4.9;rm") || validDebianVersion("") {
		t.Fatal("Debian version validation is wrong")
	}
}

// The post-install identity check accepts the same series: both packages at
// one release, and the daemon the package's own file (dpkg's digest).
func TestFreshPDNSNativeIdentityFollowsTheTestedSeries(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "pdns_server")
	payload := []byte("package daemon fixture\n")
	if err := os.WriteFile(binary, payload, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(binary, 0o755); err != nil {
		t.Fatal(err)
	}
	sum := md5.Sum(payload)
	sums := []byte("0123456789abcdef0123456789abcdef  usr/share/doc/pdns-server/copyright\n" + hex.EncodeToString(sum[:]) + "  " + strings.TrimPrefix(binary, "/") + "\n")
	ops := func(server, backend string, digests []byte) freshPDNSIdentityOpsV3 {
		return freshPDNSIdentityOpsV3{
			status: func(name string) (string, error) {
				if name == "pdns-server" {
					return "install ok installed\t" + server, nil
				}
				return "install ok installed\t" + backend, nil
			},
			md5sums: func(string) ([]byte, error) { return digests, nil },
			binary:  binary,
		}
	}
	for _, version := range []string{"4.9.17-0+deb13u1", "4.9.18-0+deb13u1"} {
		if err := verifyFreshPDNSNativeIdentityV3(ops(version, version, sums)); err != nil {
			t.Fatalf("%s: %v", version, err)
		}
	}
	other := md5.Sum([]byte("changed"))
	for name, o := range map[string]freshPDNSIdentityOpsV3{
		"other series": ops("5.0.1-1", "5.0.1-1", sums),
		"two builds":   ops("4.9.18-0+deb13u1", "4.9.17-0+deb13u1", sums),
		"vendor build": ops("4.9.4-1pdns.trixie", "4.9.4-1pdns.trixie", sums),
		"backport":     ops("4.9.4-1~bpo13+1", "4.9.4-1~bpo13+1", sums),
		"epoch":        ops("1:4.9.18-0+deb13u1", "1:4.9.18-0+deb13u1", sums),
		"changed file": ops("4.9.18-0+deb13u1", "4.9.18-0+deb13u1", []byte(hex.EncodeToString(other[:])+"  "+strings.TrimPrefix(binary, "/")+"\n")),
		"unrecorded":   ops("4.9.18-0+deb13u1", "4.9.18-0+deb13u1", []byte("")),
	} {
		if err := verifyFreshPDNSNativeIdentityV3(o); err == nil {
			t.Fatalf("%s was accepted", name)
		}
	}
	if _, err := readDpkgControlFileV3("/etc/passwd"); err == nil {
		t.Fatal("a control path outside dpkg's database was read")
	}
}

// Item 4b: the complete build is published under the candidate name by an
// atomic, non-replacing rename in the same directory; the inode is kept.
func TestPublishFreshPDNSCandidateBuildV3(t *testing.T) {
	dir := t.TempDir()
	build := filepath.Join(dir, ".celikpanel-switch-"+strings.Repeat("a", 32)+".building.sqlite3")
	candidate := filepath.Join(dir, ".celikpanel-switch-"+strings.Repeat("a", 32)+".sqlite3")
	if err := os.WriteFile(build, []byte("complete database"), 0o600); err != nil {
		t.Fatal(err)
	}
	var before unix.Stat_t
	if err := unix.Stat(build, &before); err != nil {
		t.Fatal(err)
	}
	if err := publishFreshPDNSCandidateBuildV3(build, candidate); err != nil {
		t.Fatal(err)
	}
	var after unix.Stat_t
	if err := unix.Stat(candidate, &after); err != nil || after.Ino != before.Ino {
		t.Fatalf("candidate inode=%d, want %d (%v)", after.Ino, before.Ino, err)
	}
	if _, err := os.Lstat(build); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("build name still exists: %v", err)
	}
	if err := os.WriteFile(build, []byte("second"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := publishFreshPDNSCandidateBuildV3(build, candidate); err == nil {
		t.Fatal("an existing candidate was replaced")
	}
	if err := publishFreshPDNSCandidateBuildV3(build, filepath.Join(t.TempDir(), "x.sqlite3")); err == nil {
		t.Fatal("a rename across directories was attempted")
	}
}
