//go:build linux

package main

import (
	"context"
	"errors"
	"time"

	"github.com/alicelik/celikpanel/internal/dnsengineartifact"
	"github.com/alicelik/celikpanel/internal/transport"
)

// PDNSFreshPrimaryCandidateV1 answers, read-only, which PowerDNS build this
// server would install for a fresh paired primary and whether it is Debian's
// own archive build of the tested series, so the setup wizard offers
// PowerDNS as the main DNS server only when the DNS step will accept it. It
// simulates the install (apt-get -s), never refreshes the package lists and
// never waits for another package operation: busy or unreadable is unknown.
//
// PDNSFreshPrimaryCandidateV1, eşli birincil için kurulacak PowerDNS yapısını
// ve Debian arşivinden mi geldiğini salt-okur söyler.
func (a *Agent) PDNSFreshPrimaryCandidateV1(_ *transport.Empty, resp *transport.PDNSFreshPrimaryCandidateResponse) error {
	if resp == nil {
		return errors.New("PowerDNS candidate reply is required")
	}
	*resp = transport.PDNSFreshPrimaryCandidateResponse{State: "unknown"}
	profile, err := verifiedHostProfile("apt")
	if err != nil {
		return nil
	}
	aptGet, err := executableForProfile(profile, "apt", "apt-get")
	if err != nil {
		return nil
	}
	if !packageOperationMu.TryLock() {
		return nil
	}
	defer packageOperationMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	sim, err := runAptSimulation(ctx, runPackageSimulationCommand, aptGet, aptInstallArgs(dnsengineartifact.PDNSFreshPrimaryPackages))
	if err != nil {
		return nil
	}
	*resp = pdnsFreshPrimaryCandidateFrom(sim)
	return nil
}

// pdnsFreshPrimaryCandidateFrom reads a simulation of installing both
// packages: known only when both are new installs at one version.
func pdnsFreshPrimaryCandidateFrom(sim aptSimulation) transport.PDNSFreshPrimaryCandidateResponse {
	resp := transport.PDNSFreshPrimaryCandidateResponse{State: "unknown"}
	version := ""
	official := true
	for _, name := range dnsengineartifact.PDNSFreshPrimaryPackages {
		line, ok := sim.Versions[name]
		if !ok || line.To == "" || (version != "" && line.To != version) {
			return resp
		}
		version = line.To
		official = official && dnsengineartifact.PDNSOfficialDebianArchive(pdnsArchivesOf(sim, name, version))
	}
	resp.State, resp.Version, resp.Official = "known", version, official
	return resp
}
