package dnsengineartifact

import "testing"

func TestPDNSFreshPrimaryTestedSeries(t *testing.T) {
	for version, want := range map[string]bool{
		"4.9.17-0+deb13u1": true, "4.9.18-0+deb13u1": true, "4.9.18-1": true,
		"4.9.18": false, "1:4.9.20-1": false, "4.9.4-1~bpo13+1": false, "4.9.4-1pdns.trixie": false, "4.9.18-0ubuntu1": false,
		"4.8.4-1": false, "5.0.1-1": false, "4.10.0-1": false, "4.9.19~rc1-1": false, "4.9": false, "": false, "4.9.x-1": false,
	} {
		if got := PDNSFreshPrimaryVersionInTestedSeries(version); got != want {
			t.Errorf("%q: got %v want %v", version, got, want)
		}
	}
	for name, archives := range map[string][]PDNSArchive{
		"powerdns repo": {{Origin: "PowerDNS", Suite: "trixie-auth-49"}},
		"backports":     {{Origin: "Debian Backports", Suite: "trixie-backports"}},
		"debian bpo":    {{Origin: "Debian", Suite: "stable-backports"}},
		"mixed":         {{Origin: "Debian", Suite: "stable"}, {Origin: "mirror.example", Suite: "stable"}},
		"unreadable":    nil,
	} {
		if PDNSOfficialDebianArchive(archives) {
			t.Errorf("%s accepted as Debian's archive", name)
		}
	}
	if !PDNSOfficialDebianArchive([]PDNSArchive{{Origin: "Debian", Suite: "stable"}, {Origin: "Debian-Security", Suite: "stable-security"}}) {
		t.Error("Debian's own archive refused")
	}
	sums := []byte("0123456789abcdef0123456789ABCDEF  usr/sbin/pdns_server\nffffffffffffffffffffffffffffffff  usr/bin/pdnsutil\n")
	if got, ok := PDNSPackageFileMD5(sums, "/usr/sbin/pdns_server"); !ok || got != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("digest %q %v", got, ok)
	}
	for _, bad := range []string{"", "nothex  usr/sbin/pdns_server\n", "0123456789abcdef0123456789abcdef  usr/sbin/pdns_server\n0123456789abcdef0123456789abcdef  usr/sbin/pdns_server\n"} {
		if _, ok := PDNSPackageFileMD5([]byte(bad), "/usr/sbin/pdns_server"); ok {
			t.Fatalf("accepted %q", bad)
		}
	}
}
