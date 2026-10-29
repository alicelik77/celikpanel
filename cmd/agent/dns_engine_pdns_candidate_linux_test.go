//go:build linux

package main

import "testing"

// The wizard's read of the PowerDNS build: one version for both packages and
// whether every archive is Debian's own; anything else is unknown.
func TestPDNSFreshPrimaryCandidateFromASimulation(t *testing.T) {
	read := func(lines string) (string, string, bool) {
		got := pdnsFreshPrimaryCandidateFrom(parseAptSimulation([]byte(lines)))
		return got.State, got.Version, got.Official
	}
	debian := "Inst pdns-server (4.9.18-0+deb13u1 Debian:13.2/stable, Debian-Security:13/stable-security [amd64])\nInst pdns-backend-sqlite3 (4.9.18-0+deb13u1 Debian:13.2/stable [amd64])\n"
	if state, version, official := read(debian); state != "known" || version != "4.9.18-0+deb13u1" || !official {
		t.Fatalf("Debian's build: %s %s %v", state, version, official)
	}
	vendor := "Inst pdns-server (4.9.4-1pdns.trixie PowerDNS:trixie-auth-49 [amd64])\nInst pdns-backend-sqlite3 (4.9.4-1pdns.trixie PowerDNS:trixie-auth-49 [amd64])\n"
	if state, _, official := read(vendor); state != "known" || official {
		t.Fatalf("repo.powerdns.com build read as Debian's: %s %v", state, official)
	}
	for name, lines := range map[string]string{
		"one package":  "Inst pdns-server (4.9.18-0+deb13u1 Debian:13.2/stable [amd64])\n",
		"two versions": "Inst pdns-server (4.9.18-0+deb13u1 Debian:13.2/stable [amd64])\nInst pdns-backend-sqlite3 (4.9.17-0+deb13u1 Debian:13.2/stable [amd64])\n",
		"nothing":      "",
	} {
		if state, _, _ := read(lines); state != "unknown" {
			t.Fatalf("%s: %s", name, state)
		}
	}
}
