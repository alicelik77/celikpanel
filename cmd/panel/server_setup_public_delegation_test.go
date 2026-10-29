package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicelik/celikpanel/internal/dnswire"
)

// The existing-records step explains an unread name by where the domain
// points. The loopback parent zone names ns1.example.test (203.0.113.10) and
// ns2.example.test (192.0.2.99).
func TestServerSetupPublicDelegationSaysWhetherTheDomainAlreadyPointsHere(t *testing.T) {
	options, _ := registrarFixture(t, false)
	read := func(own serverSetupOwnAddresses) *serverSetupPublicDelegation {
		return readServerSetupPublicDelegation(context.Background(), "example.test", own, options)
	}
	servers := func(delegation *serverSetupPublicDelegation) map[string]string {
		out := map[string]string{}
		for _, ns := range delegation.Nameservers {
			out[ns.Name] = ns.Server
		}
		return out
	}
	// This server and the pair's known address: the domain is already here.
	here := read(serverSetupOwnAddresses{This: []string{"203.0.113.10"}, Peer: []string{"192.0.2.99"}})
	if here.State != "here" || servers(here)["ns1.example.test"] != "this" || servers(here)["ns2.example.test"] != "peer" || len(here.Nameservers[0].Addresses) != 1 {
		t.Fatalf("both nameservers are ours: %+v", here)
	}
	// The second server joins later: its address is not known, and a glued
	// address that is not this server's is its pair.
	pending := read(serverSetupOwnAddresses{This: []string{"203.0.113.10"}, PairExpected: true})
	if pending.State != "here" || servers(pending)["ns2.example.test"] != "peer" {
		t.Fatalf("the pair is not joined yet: %+v", pending)
	}
	// One server with no pair: the second nameserver is somewhere else.
	alone := read(serverSetupOwnAddresses{This: []string{"203.0.113.10"}})
	if alone.State != "elsewhere" || servers(alone)["ns2.example.test"] != "other" {
		t.Fatalf("a nameserver that is not ours: %+v", alone)
	}
	// A known pair address that differs from the glue is another server.
	other := read(serverSetupOwnAddresses{This: []string{"203.0.113.10"}, Peer: []string{"198.51.100.20"}, PairExpected: true})
	if other.State != "elsewhere" {
		t.Fatalf("the glue is not the pair's address: %+v", other)
	}
	// Neither address is ours.
	away := read(serverSetupOwnAddresses{This: []string{"203.0.113.77"}, PairExpected: true})
	if away.State != "elsewhere" {
		t.Fatalf("nothing points here: %+v", away)
	}
	// Nameservers with no known address (outside the domain) are never ours.
	outside := classifyServerSetupPublicDelegation(dnswire.DelegationResult{Registered: true, NS: []string{"ns1.provider.test", "ns2.provider.test"}}, nil, serverSetupOwnAddresses{This: []string{"203.0.113.10"}, PairExpected: true})
	if outside.State != "elsewhere" || len(outside.Nameservers) != 2 || outside.Nameservers[0].Server != "other" {
		t.Fatalf("provider nameservers: %+v", outside)
	}
	// An unreadable parent, or a domain with no nameservers there, is unknown.
	if unread := classifyServerSetupPublicDelegation(dnswire.DelegationResult{}, errors.New("no answer"), serverSetupOwnAddresses{}); unread.State != "unknown" || len(unread.Nameservers) != 0 {
		t.Fatalf("unreadable: %+v", unread)
	}
	if none := classifyServerSetupPublicDelegation(dnswire.DelegationResult{Registered: true}, nil, serverSetupOwnAddresses{}); none.State != "unknown" {
		t.Fatalf("no nameservers: %+v", none)
	}
}

// The endpoint adds the delegation only when a name could not be read, and
// ?delegation=only reads it without reading any record.
func TestServerSetupPublicRecordsEndpointNamesTheDelegationForUnreadNames(t *testing.T) {
	options, _ := registrarFixture(t, false)
	previousOptions, previousResolvers := setupDelegationOptions, setupPublicRecordResolvers
	t.Cleanup(func() { setupDelegationOptions, setupPublicRecordResolvers = previousOptions, previousResolvers })
	setupDelegationOptions = func() dnswire.DelegationOptions { return options }
	failing := true
	setupPublicRecordResolvers = func() []setupProviderRecordLookup {
		return []setupProviderRecordLookup{func(context.Context, string, uint16) ([]string, error) {
			if failing {
				return nil, errors.New("name servers did not respond")
			}
			return nil, &net.DNSError{IsNotFound: true}
		}}
	}
	f, state := setupOperationFixture(t)
	state.Draft.MainDomain, state.Draft.LocalIP, state.Draft.PeerIP = "example.test", "203.0.113.10", "192.0.2.99"
	if _, err := f.panel.saveServerSetupDraft(context.Background(), state.Revision, state.Draft); err != nil {
		t.Fatal(err)
	}
	get := func(path string) serverSetupPublicRecords {
		w := httptest.NewRecorder()
		f.panel.handleServerSetupPublicRecords(w, serviceOperationAdminRequest(t, http.MethodGet, path, "", f.userID))
		var value serverSetupPublicRecords
		if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &value) != nil {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		return value
	}
	unread := get(serverSetupPublicRecordsPath)
	if unread.Delegation == nil || unread.Delegation.State != "here" || len(unread.Delegation.Nameservers) != 2 {
		t.Fatalf("unread names must come with where the domain points: %+v", unread.Delegation)
	}
	failing = false
	if answered := get(serverSetupPublicRecordsPath); answered.Delegation != nil {
		t.Fatalf("every name was readable, so the delegation is not asked: %+v", answered.Delegation)
	}
	only := get(serverSetupPublicRecordsPath + "?delegation=only")
	if only.Delegation == nil || only.Delegation.State != "here" || len(only.Records) != 0 {
		t.Fatalf("delegation only: %+v", only)
	}
}
