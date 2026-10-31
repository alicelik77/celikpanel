package main

import (
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.114: a pair may mix a server installed before alpha.114 (port 2083)
// with a newer one (port 1923). Each server's own port comes from the
// address it declared, never from the other server's local default.

func TestServerSetupMixedPortPairAcceptsThePeersOwnPort(t *testing.T) {
	t.Setenv("CELIKPANEL_LISTEN", ":2083") // the first server, installed on alpha.113
	d, err := canonicalServerSetupDraft(joinFirstDraft())
	if err != nil {
		t.Fatal(err)
	}
	join := serverSetupJoinRecord{RequestID: strings.Repeat("a", 32), Peer: serverSetupJoinPeer{IPv4: "198.51.100.20"}}
	names := serverSetupPeerNames{RequestID: strings.Repeat("b", 32), PanelDomain: "beta.example.test", DNSEngine: "bind", Address: "https://198.51.100.20:1923", Pin: strings.Repeat("ab", 32)}
	if code, message := canonicalServerSetupPeerNames(&names, d, join); code != "" {
		t.Fatalf("a joined server on 1923: %s %s", code, message)
	}
	// Option A: the joined server is the main DNS server; its publishing
	// endpoint carries its own port.
	d.DNSRole = transport.DNSPairRoleSecondary
	names.PublisherEndpoint, names.EnrollmentCode = "https://beta.example.test:1923", strings.Repeat("c", 64)
	if code, message := canonicalServerSetupPeerNames(&names, d, join); code != "" {
		t.Fatalf("publisher on the joined server's port: %s %s", code, message)
	}
	names.PublisherEndpoint = "https://beta.example.test:2083"
	if code, _ := canonicalServerSetupPeerNames(&names, d, join); code != "server_setup_peer_names_invalid" {
		t.Fatalf("an endpoint on another port than the declared address: %q", code)
	}
}

func TestServerSetupPeerAddressNeedsItsPort(t *testing.T) {
	t.Setenv("CELIKPANEL_LISTEN", ":1923")
	for _, address := range []string{"203.0.113.10", "https://203.0.113.10", "https://203.0.113.10/"} {
		if _, ok := canonicalServerSetupPeerAddress(address); ok {
			t.Fatalf("%q: the other server's port is never guessed from this one", address)
		}
	}
	for raw, want := range map[string]string{"203.0.113.10:2083": "https://203.0.113.10:2083", "https://203.0.113.10:1923/": "https://203.0.113.10:1923"} {
		if got, ok := canonicalServerSetupPeerAddress(raw); !ok || got != want {
			t.Fatalf("%q: %q %v", raw, got, ok)
		}
	}
}
