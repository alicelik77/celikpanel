//go:build linux

package main

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/dnsengineartifact"
	"github.com/alicelik/celikpanel/internal/hostplatform"
	"github.com/alicelik/celikpanel/internal/mutationpayload"
	"github.com/alicelik/celikpanel/internal/transport"
)

func archPDNSTestProfile() hostplatform.Profile {
	return hostplatform.Profile{
		ID: "arch", Arch: "amd64", DistroFamily: hostplatform.DistroFamilyArch,
		PackageManager: hostplatform.PackageManagerPacman, ServiceManager: hostplatform.ServiceManagerSystemd,
	}
}

func TestFreshPDNSPrimaryBuildPerMeasuredHostV3(t *testing.T) {
	build, err := freshPDNSPrimaryBuildV3(archPDNSTestProfile())
	if err != nil || build.Kind != dnsengineartifact.NativeCatalogArch51V3 || build.Binary != "/usr/bin/pdns_server" {
		t.Fatalf("Arch build: %+v %v", build, err)
	}
	debian := hostplatform.Profile{ID: "debian", Version: "13", Arch: "amd64", DistroFamily: hostplatform.DistroFamilyDebian,
		PackageManager: hostplatform.PackageManagerAPT, ServiceManager: hostplatform.ServiceManagerSystemd}
	if build, err := freshPDNSPrimaryBuildV3(debian); err != nil || build.Kind != dnsengineartifact.NativeCatalogDebian49V3 {
		t.Fatalf("Debian build: %+v %v", build, err)
	}
	for name, profile := range map[string]hostplatform.Profile{
		"arch arm64":   {ID: "arch", Arch: "arm64", DistroFamily: hostplatform.DistroFamilyArch, PackageManager: hostplatform.PackageManagerPacman, ServiceManager: hostplatform.ServiceManagerSystemd},
		"manjaro":      {ID: "manjaro", Arch: "amd64", DistroFamily: hostplatform.DistroFamilyArch, PackageManager: hostplatform.PackageManagerPacman, ServiceManager: hostplatform.ServiceManagerSystemd},
		"ubuntu":       {ID: "ubuntu", Version: "24.04", Arch: "amd64", DistroFamily: hostplatform.DistroFamilyDebian, PackageManager: hostplatform.PackageManagerAPT, ServiceManager: hostplatform.ServiceManagerSystemd},
		"debian 12":    {ID: "debian", Version: "12", Arch: "amd64", DistroFamily: hostplatform.DistroFamilyDebian, PackageManager: hostplatform.PackageManagerAPT, ServiceManager: hostplatform.ServiceManagerSystemd},
		"arch no apt?": {ID: "arch", Arch: "amd64", DistroFamily: hostplatform.DistroFamilyArch, PackageManager: hostplatform.PackageManagerAPT, ServiceManager: hostplatform.ServiceManagerSystemd},
	} {
		if _, err := freshPDNSPrimaryBuildV3(profile); err == nil {
			t.Errorf("%s accepted as a measured host", name)
		}
	}
}

func TestPacmanCandidateFromPrint(t *testing.T) {
	output := []byte(":: Synchronizing package databases...\n extra downloading...\nextra boost-libs 1.89.0-3\nextra powerdns 5.1.5-1\ncore sqlite 3.54.0-1\n")
	candidate, ok := pacmanCandidateFromPrint(output, "powerdns")
	if !ok || candidate != (pacmanCandidate{Repository: "extra", Name: "powerdns", Version: "5.1.5-1"}) {
		t.Fatalf("got %+v %v", candidate, ok)
	}
	for _, bad := range []string{"", "extra powerdns 5.1.5-1\nthirdparty powerdns 5.2.0-1\n", "extra powerdns garbage\n", "extra powerdns-admin 1.0-1\n"} {
		if _, ok := pacmanCandidateFromPrint([]byte(bad), "powerdns"); ok {
			t.Errorf("accepted %q", bad)
		}
	}
	if args := strings.Join(pacmanPDNSPrintArgs(true, "powerdns"), " "); args != "-Syup --needed --print-format %r %n %v extra/powerdns" {
		t.Fatalf("refresh print args %q", args)
	}
	if args := strings.Join(pacmanPDNSPrintArgs(false, "powerdns"), " "); args != "-Sp --needed --print-format %r %n %v extra/powerdns" {
		t.Fatalf("read-only print args %q", args)
	}
}

func TestFreshPDNSPacmanCandidateRefusal(t *testing.T) {
	build, _ := dnsengineartifact.PDNSFreshPrimaryBuildFor("pacman")
	if err := freshPDNSPacmanCandidateRefusal(build, pacmanCandidate{"extra", "powerdns", "5.1.6-1"}); err != nil {
		t.Fatalf("a 5.1 point release from extra was refused: %v", err)
	}
	moved := freshPDNSPacmanCandidateRefusal(build, pacmanCandidate{"extra", "powerdns", "5.2.0-1"})
	var refusal *hostOperatorRefusal
	if !errors.As(moved, &refusal) || !strings.Contains(moved.Error(), "nothing was changed") ||
		!strings.Contains(moved.Error(), "tested PowerDNS 5.1 series") || !strings.Contains(moved.Error(), "Choose BIND") {
		t.Fatalf("5.2 refusal lacks owner guidance: %v", moved)
	}
	foreign := freshPDNSPacmanCandidateRefusal(build, pacmanCandidate{"powerdns-auth", "powerdns", "5.1.6-1"})
	if !errors.As(foreign, &refusal) || !strings.Contains(foreign.Error(), "official extra repository") ||
		!strings.Contains(foreign.Error(), "/etc/pacman.conf") {
		t.Fatalf("third-party repository refusal lacks guidance: %v", foreign)
	}
}

func TestPDNSPacmanSwitchAdmission(t *testing.T) {
	arch := archPDNSTestProfile()
	fresh := func(role string) mutationpayload.DNSEngineSwitchManifestCommitment {
		return mutationpayload.DNSEngineSwitchManifestCommitment{Mode: transport.DNSEngineSwitchModeSwitch,
			TargetEngine: transport.DNSEnginePowerDNS, Topology: transport.DNSTopologyPaired, PairRole: role}
	}
	for _, role := range []string{transport.DNSPairRolePrimary, transport.DNSPairRoleSecondary} {
		if err := pdnsPacmanSwitchAdmission(arch, fresh(role), false); err != nil {
			t.Fatalf("%s refused: %v", role, err)
		}
	}
	standalone := fresh("")
	standalone.Topology = transport.DNSTopologyStandalone
	if err := pdnsPacmanSwitchAdmission(arch, standalone, false); err != nil {
		t.Fatalf("one server keeping its own DNS refused: %v", err)
	}
	fromBIND := fresh(transport.DNSPairRolePrimary)
	fromBIND.SourceEngine = transport.DNSEngineBIND
	reinstall := fresh(transport.DNSPairRoleSecondary)
	reinstall.Mode = transport.DNSEngineSwitchModeReinstall
	adopt := fresh("")
	adopt.Topology, adopt.Mode = transport.DNSTopologyStandalone, transport.DNSEngineSwitchModeAdopt
	for name, manifest := range map[string]mutationpayload.DNSEngineSwitchManifestCommitment{
		"from BIND": fromBIND, "reinstall": reinstall, "adopt": adopt,
	} {
		err := pdnsPacmanSwitchAdmission(arch, manifest, false)
		if err == nil || !strings.Contains(err.Error(), core.PowerDNSPacmanReason) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if pdnsPacmanSwitchAdmission(arch, fresh(transport.DNSPairRoleSecondary), true) == nil {
		t.Fatal("a reconfiguration of an existing Arch state was admitted")
	}
	debian := hostplatform.Profile{PackageManager: hostplatform.PackageManagerAPT}
	if err := pdnsPacmanSwitchAdmission(debian, standalone, true); err != nil {
		t.Fatalf("Debian behaviour changed: %v", err)
	}
	if strings.Contains(core.PowerDNSPacmanReason, "not offered on Arch Linux yet") {
		t.Fatal("the Arch reason still says PowerDNS is not offered on Arch at all")
	}
}

func writePacmanTestEntry(t *testing.T, db, version, binary string, content []byte) {
	t.Helper()
	entry := filepath.Join(db, "powerdns-"+version)
	if err := os.Mkdir(entry, 0o755); err != nil {
		t.Fatal(err)
	}
	desc := "%NAME%\npowerdns\n\n%VERSION%\n" + version + "\n"
	if err := os.WriteFile(filepath.Join(entry, "desc"), []byte(desc), 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	var mtree bytes.Buffer
	w := gzip.NewWriter(&mtree)
	_, _ = w.Write([]byte("#mtree\n." + binary + " mode=755 sha256digest=" + hex.EncodeToString(sum[:]) + "\n"))
	_ = w.Close()
	if err := os.WriteFile(filepath.Join(entry, "mtree"), mtree.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A pacman update inside the series changes the daemon and its record
// together and is accepted (like a Debian point release); an edited daemon or
// a release outside the series is not.
func TestVerifyFreshPDNSPacmanIdentityV3(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("the pacman local database reader requires root-owned files")
	}
	build, _ := dnsengineartifact.PDNSFreshPrimaryBuildFor("pacman")
	build.Binary = filepath.Join(t.TempDir(), "pdns_server")
	if err := os.WriteFile(build.Binary, []byte("5.1.5"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(build.Binary, 0o755); err != nil {
		t.Fatal(err)
	}
	newDB := func() string {
		db := t.TempDir()
		if err := os.Chmod(db, 0o755); err != nil {
			t.Fatal(err)
		}
		return db
	}
	db := newDB()
	writePacmanTestEntry(t, db, "5.1.5-1", build.Binary, []byte("5.1.5"))
	if err := verifyFreshPDNSPacmanIdentityV3(db, build); err != nil {
		t.Fatal(err)
	}
	// pacman -Syu inside the series: new file, new record.
	if err := os.WriteFile(build.Binary, []byte("5.1.6"), 0o755); err != nil {
		t.Fatal(err)
	}
	updated := newDB()
	writePacmanTestEntry(t, updated, "5.1.6-1", build.Binary, []byte("5.1.6"))
	if err := verifyFreshPDNSPacmanIdentityV3(updated, build); err != nil {
		t.Fatalf("an in-series package update read as tampering: %v", err)
	}
	if err := verifyFreshPDNSPacmanIdentityV3(db, build); err == nil {
		t.Fatal("a daemon that differs from its package record was accepted")
	}
	moved := newDB()
	writePacmanTestEntry(t, moved, "5.2.0-1", build.Binary, []byte("5.1.6"))
	if err := verifyFreshPDNSPacmanIdentityV3(moved, build); err == nil || !strings.Contains(err.Error(), "tested PowerDNS 5.1 series") {
		t.Fatalf("a 5.2 package was accepted: %v", err)
	}
}

func TestPDNSSeriesBuildForState(t *testing.T) {
	arch := archPDNSTestProfile()
	debian := hostplatform.Profile{PackageManager: hostplatform.PackageManagerAPT}
	native := dnsEngineStateReceipt{Engine: transport.DNSEnginePowerDNS, NativeCatalogV3: dnsengineartifact.NativeCatalogArch51V3}
	if build, ok := pdnsSeriesBuildForState(arch, native); !ok || build.Series != "5.1" {
		t.Fatalf("Arch native state: %+v %v", build, ok)
	}
	secondary := dnsEngineStateReceipt{Engine: transport.DNSEnginePowerDNS, PairRole: transport.DNSPairRoleSecondary}
	if build, ok := pdnsSeriesBuildForState(arch, secondary); !ok || build.Series != "5.1" {
		t.Fatalf("Arch secondary state: %+v %v", build, ok)
	}
	if _, ok := pdnsSeriesBuildForState(debian, secondary); ok {
		t.Fatal("a Debian secondary gained a version rule")
	}
	if _, ok := pdnsSeriesBuildForState(arch, dnsEngineStateReceipt{Engine: transport.DNSEngineBIND}); ok {
		t.Fatal("BIND gained a PowerDNS version rule")
	}
	build, _ := dnsengineartifact.PDNSFreshPrimaryBuildFor("pacman")
	err := pdnsSeriesUntestedRefusal(build, "5.2.0-1")
	if err == nil || !strings.Contains(err.Error(), "keeps answering") || !strings.Contains(err.Error(), "update CelikPanel from its own update screen") {
		t.Fatalf("held change lacks guidance: %v", err)
	}
}
