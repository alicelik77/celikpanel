package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Owner report 2026-10-31 (Frankfurt): a directional primary whose backup had
// not yet proven the catalog served every zone publicly while the domain page
// said "no active authoritative DNS engine". The classification names why the
// DNS is not ready for a new change; only "ready" allows publication, exactly
// as activeDNSPublisher decided before.
func TestClassifyDNSPublisherNamesWhyNotReady(t *testing.T) {
	running := transport.DNSBackendRuntimeState{Installed: true, Running: true, Managed: true}
	paired := running
	paired.PairReady = true
	base := dnsEngineDBState{ActiveEngine: transport.DNSEnginePowerDNS, EngineEpoch: 3, PeerNS: "ns2.example.com", PeerIP: "192.0.2.2"}
	with := func(edit func(*dnsEngineDBState)) dnsEngineDBState { state := base; edit(&state); return state }
	cases := []struct {
		name     string
		state    dnsEngineDBState
		runtimes map[transport.DNSEngine]transport.DNSBackendRuntimeState
		port53   bool
		want     string
		identity bool
	}{
		{"no engine", with(func(s *dnsEngineDBState) { s.ActiveEngine = "" }), nil, false, dnsServerStateNone, false},
		{"switching", with(func(s *dnsEngineDBState) { s.CurrentSwitchID = "x" }), nil, false, dnsServerStateSwitching, false},
		{"port 53", base, map[transport.DNSEngine]transport.DNSBackendRuntimeState{transport.DNSEnginePowerDNS: running}, true, dnsServerStatePort53Conflict, false},
		{"not running", base, map[transport.DNSEngine]transport.DNSBackendRuntimeState{transport.DNSEnginePowerDNS: {Installed: true}}, false, dnsServerStateNotRunning, false},
		{"two engines", base, map[transport.DNSEngine]transport.DNSBackendRuntimeState{transport.DNSEnginePowerDNS: running, transport.DNSEngineBIND: running}, false, dnsServerStateEngineConflict, false},
		{"secondary", with(func(s *dnsEngineDBState) { s.PairRole = transport.DNSPairRoleSecondary }), map[transport.DNSEngine]transport.DNSBackendRuntimeState{transport.DNSEnginePowerDNS: paired}, false, dnsServerStateSecondary, true},
		{"primary waiting for its backup", with(func(s *dnsEngineDBState) { s.PairRole = transport.DNSPairRolePrimary }), map[transport.DNSEngine]transport.DNSBackendRuntimeState{transport.DNSEnginePowerDNS: running}, false, dnsServerStatePairPending, true},
		{"primary ready", with(func(s *dnsEngineDBState) { s.PairRole = transport.DNSPairRolePrimary }), map[transport.DNSEngine]transport.DNSBackendRuntimeState{transport.DNSEnginePowerDNS: paired}, false, dnsServerStateReady, true},
		{"single server", base, map[transport.DNSEngine]transport.DNSBackendRuntimeState{transport.DNSEnginePowerDNS: running}, false, dnsServerStateReady, true},
	}
	for _, tc := range cases {
		got := classifyDNSPublisher(tc.state, tc.runtimes, tc.port53)
		if got.State != tc.want {
			t.Fatalf("%s: state %q, want %q", tc.name, got.State, tc.want)
		}
		if (got.Identity.Engine != "") != tc.identity {
			t.Fatalf("%s: identity %+v, want present=%v", tc.name, got.Identity, tc.identity)
		}
		if tc.want != dnsServerStateNone && got.Engine != transport.DNSEnginePowerDNS {
			t.Fatalf("%s: engine %q, want the active engine", tc.name, got.Engine)
		}
		if got.Peer != "ns2.example.com" {
			t.Fatalf("%s: peer %q", tc.name, got.Peer)
		}
	}
	if got := classifyDNSPublisher(with(func(s *dnsEngineDBState) { s.PeerNS = "" }), nil, false).Peer; got != "192.0.2.2" {
		t.Fatalf("peer falls back to the address, got %q", got)
	}
}

// The refusal stays DNS_SERVER_REQUIRED (409, the DNS page as its fix); only
// its explanation follows the state.
func TestWriteDNSServerRequiredSaysWhy(t *testing.T) {
	read := func(state string) apiErrorBody {
		recorder := httptest.NewRecorder()
		writeDNSServerRequired(recorder, state, "pdns", "ns2.example.com", "no engine")
		if recorder.Code != http.StatusConflict {
			t.Fatalf("%s: status %d", state, recorder.Code)
		}
		var body apiErrorBody
		if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Code != errCodeDNSServerRequired || body.Action != "/dns-infrastructure" {
			t.Fatalf("%s: %+v", state, body)
		}
		return body
	}
	if body := read(dnsServerStateNone); body.Reason != "" || body.Error != "no engine" {
		t.Fatalf("none: %+v", body)
	}
	for _, state := range []string{dnsServerStatePairPending, dnsServerStateSecondary, dnsServerStateSwitching, dnsServerStatePort53Conflict, dnsServerStateNotRunning, dnsServerStateEngineConflict} {
		body := read(state)
		if body.Reason != state || body.Error == "no engine" || body.Vars["engine"] != "pdns" || body.Vars["peer"] != "ns2.example.com" {
			t.Fatalf("%s: %+v", state, body)
		}
	}
}
