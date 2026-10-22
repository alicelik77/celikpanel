package main

import (
	"bytes"
	"math"
	"strings"
	"testing"
)

func testSystemUpdateManifest() systemUpdateManifest {
	return systemUpdateManifest{
		Sequence: "42", Version: "v1.2.3-alpha.10", Commit: strings.Repeat("a", 40),
		PublishedAt: "2026-08-12T12:34:56Z", OS: "linux", Arch: "amd64",
		Archive:       "celikpanel-v1.2.3-alpha.10-linux-amd64.tar.gz",
		ArchiveSHA256: strings.Repeat("b", 64), ArchiveSize: "2147483648",
	}
}

func TestSystemUpdateManifestRequiresExactCanonicalV2Bytes(t *testing.T) {
	manifest := testSystemUpdateManifest()
	raw, err := canonicalSystemUpdateManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	want := "format=celikpanel-release-manifest-v2\n" +
		"sequence=42\nversion=v1.2.3-alpha.10\ncommit=" + strings.Repeat("a", 40) + "\n" +
		"published_at=2026-08-12T12:34:56Z\nos=linux\narch=amd64\n" +
		"archive=celikpanel-v1.2.3-alpha.10-linux-amd64.tar.gz\n" +
		"archive_sha256=" + strings.Repeat("b", 64) + "\narchive_size=2147483648\n"
	if !bytes.Equal(raw, []byte(want)) {
		t.Fatalf("canonical manifest = %q", raw)
	}
	parsed, err := parseCanonicalSystemUpdateManifest(raw)
	if err != nil {
		t.Fatal(err)
	}
	if parsed != manifest {
		t.Fatalf("parsed manifest = %#v, want %#v", parsed, manifest)
	}
	for name, mutate := range map[string]func([]byte) []byte{
		"missing-final-lf": func(value []byte) []byte { return value[:len(value)-1] },
		"extra-line":       func(value []byte) []byte { return append(value, '\n') },
		"crlf":             func(value []byte) []byte { return bytes.ReplaceAll(value, []byte("\n"), []byte("\r\n")) },
		"duplicate": func(value []byte) []byte {
			return bytes.Replace(value, []byte("sequence=42"), []byte("sequence=42\nsequence=42"), 1)
		},
		"leading-zero": func(value []byte) []byte {
			return bytes.Replace(value, []byte("sequence=42"), []byte("sequence=042"), 1)
		},
		"oversize": func(value []byte) []byte {
			return bytes.Replace(value, []byte("archive_size=2147483648"), []byte("archive_size=2147483649"), 1)
		},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseCanonicalSystemUpdateManifest(mutate(append([]byte(nil), raw...))); err == nil {
				t.Fatal("non-canonical manifest accepted")
			}
		})
	}
}

func TestSystemUpdateSemverUsesSemVerPrecedence(t *testing.T) {
	tests := []struct {
		left, right string
		want        int
	}{
		{"v1.0.0-alpha.9", "v1.0.0-alpha.10", -1},
		{"v1.0.0-alpha.10", "v1.0.0-alpha.9", 1},
		{"v1.0.0-alpha", "v1.0.0-alpha.1", -1},
		{"v1.0.0-1", "v1.0.0-alpha", -1},
		{"v1.0.0", "v1.0.0-rc.1", 1},
		{"v2.0.0", "v10.0.0", -1},
		{"v999999999999999999999.0.0", "v2.0.0", 1},
	}
	for _, test := range tests {
		got, err := compareSystemUpdateSemver(test.left, test.right)
		if err != nil || got != test.want {
			t.Fatalf("compare(%q,%q) = %d,%v want %d", test.left, test.right, got, err, test.want)
		}
	}
	for _, invalid := range []string{"dev", "1.2.3", "v01.2.3", "v1.02.3", "v1.2.03", "v1.2", "v1.2.3-alpha.01", "v1.2.3+build", "v1.2.3-alpha..1"} {
		if _, err := parseSystemUpdateSemver(invalid); err == nil {
			t.Errorf("accepted invalid version %q", invalid)
		}
	}
}

// v0.1.0-alpha.100 is the first three-digit prerelease number. As strings
// "alpha.100" sorts before "alpha.99"; the updater must order it by number,
// offer it over alpha.99, accept it above an alpha.99 floor, and refuse
// alpha.99 once the floor is at alpha.100.
func TestSystemUpdateThreeDigitPrereleaseOrdersAfterTwoDigit(t *testing.T) {
	for _, test := range []struct {
		left, right string
		want        int
	}{
		{"v0.1.0-alpha.100", "v0.1.0-alpha.99", 1},
		{"v0.1.0-alpha.99", "v0.1.0-alpha.100", -1},
		{"v0.1.0-alpha.100", "v0.1.0-alpha.100", 0},
		{"v0.1.0-alpha.101", "v0.1.0-alpha.100", 1},
		{"v0.1.0-alpha.1000", "v0.1.0-alpha.999", 1},
		{"v0.1.0", "v0.1.0-alpha.100", 1},
	} {
		got, err := compareSystemUpdateSemver(test.left, test.right)
		if err != nil || got != test.want {
			t.Fatalf("compare(%q,%q) = %d,%v want %d", test.left, test.right, got, err, test.want)
		}
	}
	if _, err := parseSystemUpdateSemver("v0.1.0-alpha.0100"); err == nil {
		t.Fatal("accepted a leading zero in a three-digit prerelease number")
	}

	manifest := testSystemUpdateManifest()
	manifest.Sequence, manifest.Version = "100", "v0.1.0-alpha.100"
	manifest.Archive = "celikpanel-v0.1.0-alpha.100-linux-amd64.tar.gz"
	raw, err := canonicalSystemUpdateManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if parsed, err := parseCanonicalSystemUpdateManifest(raw); err != nil || parsed != manifest {
		t.Fatalf("alpha.100 manifest round trip = %#v, %v", parsed, err)
	}
	if err := systemUpdateFloorAllows(&systemUpdateFloor{Sequence: "99", Version: "v0.1.0-alpha.99"}, manifest); err != nil {
		t.Fatalf("alpha.100 refused above the alpha.99 floor: %v", err)
	}
	if err := systemUpdateFloorAllows(&systemUpdateFloor{Sequence: "100", Version: "v0.1.0-alpha.100"}, manifest); err != nil {
		t.Fatalf("alpha.100 retry refused at its own floor: %v", err)
	}
	older := testSystemUpdateManifest()
	older.Sequence, older.Version = "99", "v0.1.0-alpha.99"
	older.Archive = "celikpanel-v0.1.0-alpha.99-linux-amd64.tar.gz"
	if err := systemUpdateFloorAllows(&systemUpdateFloor{Sequence: "100", Version: "v0.1.0-alpha.100"}, older); err == nil {
		t.Fatal("alpha.99 accepted below the alpha.100 floor")
	}
	floorRaw := canonicalSystemUpdateFloor(systemUpdateFloor{Sequence: "100", Version: "v0.1.0-alpha.100"})
	if floor, err := parseCanonicalSystemUpdateFloor(floorRaw); err != nil || floor.Sequence != "100" || floor.Version != "v0.1.0-alpha.100" {
		t.Fatalf("alpha.100 floor round trip = %#v, %v", floor, err)
	}
}

func TestSystemUpdateDecimalAndFloorFailClosed(t *testing.T) {
	for _, invalid := range []string{"", "0", "01", "+1", "-1", " 1", "1 ", "9223372036854775808"} {
		if _, err := parseCanonicalPositiveDecimal(invalid, math.MaxInt64); err == nil {
			t.Errorf("accepted invalid decimal %q", invalid)
		}
	}
	manifest := testSystemUpdateManifest()
	if err := systemUpdateFloorAllows(nil, manifest); err == nil {
		t.Fatal("missing trusted floor accepted")
	}
	if err := systemUpdateFloorAllows(&systemUpdateFloor{Sequence: "41", Version: "v1.2.2"}, manifest); err != nil {
		t.Fatal(err)
	}
	if err := systemUpdateFloorAllows(&systemUpdateFloor{Sequence: "42", Version: manifest.Version}, manifest); err != nil {
		t.Fatal(err)
	}
	if err := systemUpdateFloorAllows(&systemUpdateFloor{Sequence: "43", Version: "v1.2.4"}, manifest); err == nil {
		t.Fatal("rollback below floor accepted")
	}
	if err := systemUpdateFloorAllows(&systemUpdateFloor{Sequence: "42", Version: "v9.9.9"}, manifest); err == nil {
		t.Fatal("same-sequence conflicting identity accepted")
	}
	raw := canonicalSystemUpdateFloor(systemUpdateFloor{Sequence: "42", Version: manifest.Version})
	if _, err := parseCanonicalSystemUpdateFloor(raw); err != nil {
		t.Fatal(err)
	}
	if _, err := parseCanonicalSystemUpdateFloor(append(raw, '\n')); err == nil {
		t.Fatal("non-canonical floor accepted")
	}
}
