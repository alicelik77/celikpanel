package dnsengineartifact

import (
	"testing"

	"github.com/alicelik/celikpanel/internal/pdnsnative"
)

func TestPDNSFreshPrimaryBuildsPerFamily(t *testing.T) {
	apt, ok := PDNSFreshPrimaryBuildFor("apt")
	if !ok || apt.Series != "4.9" || apt.Binary != "/usr/sbin/pdns_server" || apt.ServiceGroup != "pdns" ||
		apt.Kind != NativeCatalogDebian49V3 || len(apt.Packages) != 2 {
		t.Fatalf("Debian build changed: %+v", apt)
	}
	arch, ok := PDNSFreshPrimaryBuildFor("pacman")
	if !ok || arch.Series != "5.1" || arch.Binary != "/usr/bin/pdns_server" || arch.ServiceUser != "powerdns" ||
		arch.ServiceGroup != "powerdns" || arch.Kind != NativeCatalogArch51V3 ||
		len(arch.Packages) != 1 || arch.Packages[0] != "powerdns" {
		t.Fatalf("Arch build: %+v", arch)
	}
	if _, ok := PDNSFreshPrimaryBuildFor("dnf"); ok {
		t.Fatal("an unmeasured family has a tested build")
	}
	arch.Packages[0] = "mutated"
	if again, _ := PDNSFreshPrimaryBuildFor("pacman"); again.Packages[0] != "powerdns" {
		t.Fatal("the build table is shared by reference")
	}
	for marker, want := range map[string]bool{NativeCatalogDebian49V3: true, NativeCatalogArch51V3: true, "": false, "pdns-fresh-paired-primary/arch-5.2/v1": false} {
		if IsNativeCatalogV3(marker) != want {
			t.Errorf("%q: want %v", marker, want)
		}
	}
}

func TestPDNSPacmanVersionInTestedSeries(t *testing.T) {
	for version, want := range map[string]bool{
		"5.1.5-1": true, "5.1.4-2": true, "5.1.10-1": true, "5.1.5-1.1": true,
		"5.2.0-1": false, "5.0.3-1": false, "4.9.18-1": false, "1:5.1.5-1": false,
		"5.1.5": false, "5.1-1": false, "5.1.0rc1-1": false, "5.1.x-1": false, "": false,
	} {
		if got := PDNSPacmanVersionInTestedSeries(version); got != want {
			t.Errorf("%q: got %v want %v", version, got, want)
		}
		if got := PDNSFreshPrimaryVersionInTestedSeriesFor("pacman", version); got != want {
			t.Errorf("for pacman %q: got %v want %v", version, got, want)
		}
	}
	// The families do not cross: an Arch release is not Debian's build and
	// a Debian archive build is not Arch's.
	if PDNSFreshPrimaryVersionInTestedSeriesFor("apt", "5.1.5-1") || PDNSFreshPrimaryVersionInTestedSeriesFor("pacman", "4.9.18-0+deb13u1") {
		t.Fatal("a version rule crossed package families")
	}
	if PDNSPacmanSeries("5.2.0-1") != "5.2" || PDNSPacmanSeries("garbage") != "" {
		t.Fatal("series extraction")
	}
}

func TestPDNSMainConfigSnapshotFamilies(t *testing.T) {
	policy := JournalPolicy{PDNSMainPath: "/etc/powerdns/pdns.conf", PDNSManagedPath: "/etc/powerdns/pdns.d/celikpanel.conf", PDNSClusterPath: "/etc/powerdns/pdns.d/celikpanel-cluster.conf"}
	snapshot := func(mode, gid uint32) FileSnapshot {
		data := []byte("setgid=powerdns\nsetuid=powerdns\n")
		return FileSnapshot{Path: policy.PDNSMainPath, Exists: true, Mode: mode, OwnerKnown: true, UID: 0, GID: gid, Data: data, SHA256: DigestBytes(data)}
	}
	for _, accepted := range []FileSnapshot{snapshot(0o640, 0), snapshot(0o640, 997), snapshot(0o644, 0)} {
		if err := policy.ValidatePDNSConfigSnapshot(accepted); err != nil {
			t.Fatalf("mode %o gid %d: %v", accepted.Mode, accepted.GID, err)
		}
	}
	for _, refused := range []FileSnapshot{snapshot(0o644, 969), snapshot(0o666, 0), snapshot(0o600, 0)} {
		if err := policy.ValidatePDNSConfigSnapshot(refused); err == nil {
			t.Fatalf("mode %o gid %d accepted", refused.Mode, refused.GID)
		}
	}
}

// The native start-up transition measured on Arch's extra/powerdns 5.1.5-1
// (2026-10-30, lab guest, evidence arch-powerdns-20261030): the same SQL
// shape as Debian's 4.9.17 - notified_serial written, last_check left NULL,
// the producer SOA rewritten once and one CATALOG-HASH metadata row.
func TestMeasuredArch515FreshPrimaryNativeTransition(t *testing.T) {
	const catalog = "catalog-c0000346.celikpanel.invalid"
	schema := [][4]string{{"table", "domains", "domains", "CREATE TABLE domains (...)"}}
	tables := func(domains, records, metadata [][]any) map[string][][]any {
		return map[string][][]any{"domains": domains, "records": records, "domainmetadata": metadata,
			"supermasters": {}, "comments": {}, "cryptokeys": {}, "tsigkeys": {}, "celikpanel_dns_zone_sync_receipts": {}}
	}
	member := func(id float64, name, kind, content string, ttl float64) []any {
		return []any{id, 2., name, kind, content, ttl, 0., 0., nil, 1.}
	}
	common := [][]any{
		{2., 1., catalog, "NS", "invalid.", 60., 0., 0., nil, 1.},
		member(3, "s1-arch.test", "SOA", "ns1.s1-arch.test hostmaster.s1-arch.test 2026103001 10800 3600 604800 3600", 3600),
		member(4, "s1-arch.test", "NS", "ns1.s1-arch.test", 3600),
		member(5, "ns1.s1-arch.test", "A", "192.0.3.70", 300),
		member(6, "www.s1-arch.test", "A", "192.0.3.70", 300),
	}
	staged := pdnsnative.Snapshot{Schema: schema, Tables: tables(
		[][]any{
			{1., catalog, nil, nil, "PRODUCER", nil, "celikpanel-bind-catalog-v1", nil, nil},
			{2., "s1-arch.test", nil, nil, "MASTER", nil, nil, nil, catalog},
		},
		append([][]any{{1., 1., catalog, "SOA", "invalid. invalid. 1 60 30 3600 30", 60., 0., 0., nil, 1.}}, common...),
		[][]any{},
	)}
	live := pdnsnative.Snapshot{Schema: schema, Tables: tables(
		[][]any{
			{1., catalog, nil, nil, "PRODUCER", 1., "celikpanel-bind-catalog-v1", nil, nil},
			{2., "s1-arch.test", nil, nil, "MASTER", 2026103001., nil, nil, catalog},
		},
		append(append([][]any{}, common...), []any{7., 1., catalog, "SOA", "invalid invalid 1793347972 60 30 3600 30", 60., 0., 0., nil, 1.}),
		[][]any{{1., 1., "CATALOG-HASH", "Oy8PnoV/Sm1sTi9jqQLXWPgi2iO9xHebxvY0sqLiLgY="}},
	)}
	proof, err := pdnsnative.VerifyFreshPrimaryCatalogTransition(staged, live, catalog, 1)
	if err != nil {
		t.Fatal(err)
	}
	if proof.NativeSerial != 1793347972 || proof.CatalogHash != "Oy8PnoV/Sm1sTi9jqQLXWPgi2iO9xHebxvY0sqLiLgY=" {
		t.Fatalf("unexpected proof %+v", proof)
	}
}
