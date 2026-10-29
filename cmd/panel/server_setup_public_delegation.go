package main

import (
	"context"
	"net"
	"slices"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/dnswire"
)

// Where the main domain's nameservers point (the existing-records step). A
// name the public resolvers could not read means two different things: the
// domain's nameservers are somewhere else and did not answer, or the domain
// already points at the servers this setup is about to install, where
// nothing answers yet. The second is not a failure and "try again" does not
// help, so the answer names the delegation the parent zone itself shows
// (asked without recursion, like the registrar check) and which of those
// nameservers are this server or its pair. It only reads.
// Alan adinin ad sunuculari nereyi gosteriyor; okunamayan ad, henuz kurulmamis
// bu sunuculara giden bir delegasyon da olabilir.

type serverSetupPublicNameserver struct {
	Name      string   `json:"name"`
	Addresses []string `json:"addresses"`
	// Server: "this" (this server's address), "peer" (its pair's address) or
	// "other" (another address, or none known for the name).
	Server string `json:"server"`
}

type serverSetupPublicDelegation struct {
	// State: "here" when every nameserver is this server or its pair and at
	// least one is this server; "elsewhere" when the parent zone names other
	// nameservers; "unknown" when the delegation could not be read or the
	// domain has no nameservers there.
	State       string                        `json:"state"`
	Nameservers []serverSetupPublicNameserver `json:"nameservers"`
}

var serverSetupPublicDelegationBudget = 6 * time.Second

// serverSetupOwnAddresses are this server's addresses and, when known, its
// pair's. PairExpected is true when the setup has a second server (it joins
// by code, so its address may not be known yet): a nameserver at an address
// that is not this server's can then be that server.
type serverSetupOwnAddresses struct {
	This, Peer   []string
	PairExpected bool
}

func serverSetupOwnAddressesFor(d serverSetupDraft) serverSetupOwnAddresses {
	own := serverSetupOwnAddresses{}
	for _, value := range []string{d.LocalIP, serverPrimaryIP(), setupProviderRecordIPv6()} {
		if value = strings.TrimSpace(value); value != "" && !slices.Contains(own.This, value) {
			own.This = append(own.This, value)
		}
	}
	if peer := strings.TrimSpace(d.PeerIP); peer != "" {
		own.Peer = []string{peer}
	}
	own.PairExpected = d.DNSMode == setupDNSModeLocal && !serverSetupStandaloneDNS(d)
	return own
}

func serverSetupAddressIn(list []string, address string) bool {
	right := net.ParseIP(strings.TrimSpace(address))
	return right != nil && slices.ContainsFunc(list, func(item string) bool {
		left := net.ParseIP(strings.TrimSpace(item))
		return left != nil && left.Equal(right)
	})
}

// classifyServerSetupPublicDelegation sorts the parent zone's nameservers.
// Glue names only the addresses of nameservers inside the domain; a name with
// no known address is "other", so a delegation to a provider's nameservers is
// never taken for this server.
func classifyServerSetupPublicDelegation(result dnswire.DelegationResult, err error, own serverSetupOwnAddresses) *serverSetupPublicDelegation {
	out := &serverSetupPublicDelegation{State: "unknown", Nameservers: []serverSetupPublicNameserver{}}
	if err != nil || !result.Registered || len(result.NS) == 0 {
		return out
	}
	names := append([]string(nil), result.NS...)
	slices.Sort(names)
	here, ours := 0, true
	for _, name := range names {
		addresses := append([]string{}, result.Glue[name]...)
		slices.Sort(addresses)
		server := "other"
		switch {
		case slices.ContainsFunc(addresses, func(address string) bool { return serverSetupAddressIn(own.This, address) }):
			server = "this"
			here++
		case slices.ContainsFunc(addresses, func(address string) bool { return serverSetupAddressIn(own.Peer, address) }):
			server = "peer"
		case own.PairExpected && len(own.Peer) == 0 && len(addresses) > 0:
			// The second server joins by code later and its address is not
			// known here: another glued address is taken as that server.
			server = "peer"
		}
		if server == "other" {
			ours = false
		}
		out.Nameservers = append(out.Nameservers, serverSetupPublicNameserver{Name: name, Addresses: addresses, Server: server})
	}
	out.State = "elsewhere"
	if ours && here > 0 {
		out.State = "here"
	}
	return out
}

// readServerSetupPublicDelegation walks the parent zone for the domain.
func readServerSetupPublicDelegation(ctx context.Context, domain string, own serverSetupOwnAddresses, options dnswire.DelegationOptions) *serverSetupPublicDelegation {
	ctx, cancel := context.WithTimeout(ctx, serverSetupPublicDelegationBudget)
	defer cancel()
	result, err := dnswire.Delegation(ctx, domain, options)
	return classifyServerSetupPublicDelegation(result, err, own)
}
