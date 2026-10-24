package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.107 (D-038 §4): the operating system's own automatic updates are
// read from its own configuration and package manager; unknown stays
// unknown; the one write is a separate drop-in.

const aptDump = `APT::Periodic::Update-Package-Lists "1";
APT::Periodic::Unattended-Upgrade "1";
Unattended-Upgrade::Origins-Pattern "";
Unattended-Upgrade::Origins-Pattern:: "origin=Debian,codename=trixie,label=Debian";
Unattended-Upgrade::Origins-Pattern:: "origin=Debian,codename=trixie,label=Debian-Security";
Unattended-Upgrade::Allowed-Origins "";
Unattended-Upgrade::Allowed-Origins:: "Ubuntu:noble-security";
`

func TestTheAptConfigurationIsReadAsAptSaysIt(t *testing.T) {
	scalars, lists := parseAptConfigDump(aptDump)
	if scalars["APT::Periodic::Unattended-Upgrade"] != "1" || len(lists["Unattended-Upgrade::Origins-Pattern"]) != 2 {
		t.Fatalf("scalars %v lists %v", scalars, lists)
	}
	if !unattendedCovers(lists, "Debian") || !unattendedCovers(lists, "Ubuntu") || unattendedCovers(lists, "deb.sury.org") || unattendedCovers(lists, "") {
		t.Fatal("coverage")
	}
}

func TestTheLastRunIsReadFromTheirOwnLogs(t *testing.T) {
	log := "2026-10-23 06:10:01,201 INFO Starting unattended upgrades script\n2026-10-23 06:10:05,001 INFO No packages found that can be upgraded unattended\n2026-10-24 06:12:01,123 INFO Starting unattended upgrades script\n"
	if got := lastUnattendedRun(strings.NewReader(log)); got == "" || !strings.HasPrefix(got, "2026-10-2") {
		t.Fatalf("got %q", got)
	}
	if got := lastUnattendedRun(strings.NewReader("nothing\n")); got != "" {
		t.Fatalf("got %q", got)
	}
	pacman := "[2026-10-20T08:00:00+0000] [PACMAN] Running 'pacman -Syu'\n[2026-10-20T08:00:01+0000] [PACMAN] starting full system upgrade\n"
	if got := lastPacmanUpgrade(strings.NewReader(pacman)); got != "2026-10-20T08:00:01Z" {
		t.Fatalf("pacman %q", got)
	}
}

func TestWaitingSecurityUpdatesAreCountedFromTheSimulation(t *testing.T) {
	out := strings.Join([]string{
		"Inst libssl3t64 [3.5.1-1] (3.5.1-1+deb13u1 Debian-Security:13/stable-security [amd64])",
		"Inst openssl [3.5.1-1] (3.5.1-1+deb13u1 Debian-Security:13/stable-security [amd64])",
		"Inst tzdata [2025b-4] (2025c-0+deb13u1 Debian:13.2/stable [all])",
		"Inst libc6 [2.39-0ubuntu8] (2.39-0ubuntu8.4 Ubuntu:24.04/noble-security [amd64])",
		"Conf openssl (3.5.1-1+deb13u1 Debian-Security:13/stable-security [amd64])",
	}, "\n")
	pending := parsePendingUpgrades(out)
	if pending.Total != 4 || pending.Security != 3 || pending.SecurityNames[0] != "libssl3t64" {
		t.Fatalf("pending = %+v", pending)
	}
}

func TestTheDropInAddsOnlyConfinedUncoveredRepositoriesAndIsNeverRewrittenWhenEdited(t *testing.T) {
	vendors := []transport.OSUpdatesVendorOrigin{
		{RepoID: "sury", Origin: "deb.sury.org", Confined: true},
		{RepoID: "pgdg", Origin: "apt.postgresql.org", Confined: true, Covered: true},
		{RepoID: "netdata", Origin: "Netdata", Confined: false},
	}
	if got := strings.Join(vendorOriginsToAdd(vendors, nil), ","); got != "deb.sury.org" {
		t.Fatalf("got %q", got)
	}
	// What CelikPanel's own drop-in already adds stays in a rewrite.
	vendors[1].Covered = true
	if got := strings.Join(vendorOriginsToAdd(vendors, []string{"apt.postgresql.org"}), ","); got != "apt.postgresql.org,deb.sury.org" {
		t.Fatalf("got %q", got)
	}
	text := renderVendorOriginsDropIn([]string{"deb.sury.org"})
	if !strings.Contains(text, "Unattended-Upgrade::Origins-Pattern {\n\t\"origin=deb.sury.org\";\n};") {
		t.Fatalf("text = %s", text)
	}
	_, lists := parseAptConfigDump(`Unattended-Upgrade::Origins-Pattern:: "origin=deb.sury.org";`)
	if !unattendedCovers(lists, "deb.sury.org") {
		t.Fatal("the drop-in's own line is not read as coverage")
	}
	dir := t.TempDir()
	old := vendorOriginsDropInPath
	vendorOriginsDropInPath = filepath.Join(dir, "52celikpanel-vendor-origins")
	t.Cleanup(func() { vendorOriginsDropInPath = old })
	if state, _ := readVendorOriginsDropIn(); state != "absent" {
		t.Fatal(state)
	}
	os.WriteFile(vendorOriginsDropInPath, []byte(text), 0o644)
	if state, origins := readVendorOriginsDropIn(); state != "present" || len(origins) != 1 {
		t.Fatal(state, origins)
	}
	os.WriteFile(vendorOriginsDropInPath, []byte(text+"// mine\n"), 0o644)
	if state, _ := readVendorOriginsDropIn(); state != "edited" {
		t.Fatal(state)
	}
}

func TestUbuntusNeedrestartAutomaticModeIsRead(t *testing.T) {
	dir := t.TempDir()
	old := needrestartConfigDir
	needrestartConfigDir = dir
	t.Cleanup(func() { needrestartConfigDir = old })
	os.MkdirAll(filepath.Join(dir, "conf.d"), 0o755)
	os.WriteFile(filepath.Join(dir, "needrestart.conf"), []byte("#$nrconf{restart} = 'i';\n$nrconf{restart} = 'i';\n"), 0o644)
	if needrestartRestartsAutomatically() {
		t.Fatal("interactive read as automatic")
	}
	os.WriteFile(filepath.Join(dir, "conf.d", "50-auto.conf"), []byte("$nrconf{restart} = 'a';\n"), 0o644)
	if !needrestartRestartsAutomatically() {
		t.Fatal("automatic not read")
	}
}

func TestRoundcubesVersionIsTheTreesOwnAnswer(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "program", "include"), 0o755)
	os.WriteFile(filepath.Join(dir, "program", "include", "iniset.php"), []byte("<?php\ndefine('RCMAIL_VERSION', '1.6.15');\n"), 0o644)
	if got := roundcubeTreeVersion(dir); got != "1.6.15" {
		t.Fatalf("got %q", got)
	}
	if got := roundcubeTreeVersion(t.TempDir()); got != "" {
		t.Fatalf("got %q", got)
	}
}
