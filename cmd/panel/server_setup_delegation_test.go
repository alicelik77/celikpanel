package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/dnswire"
	"github.com/alicelik/celikpanel/internal/dnswire/dnswiretest"
)

// The registrar check against loopback servers only: a recursive resolver,
// the parent zone's server and this setup's main DNS server.
func registrarFixture(t *testing.T, mainAnswers bool) (dnswire.DelegationOptions, *dnswiretest.Server) {
	t.Helper()
	resolver := dnswiretest.Start(t, func(name string, qtype uint16, recursive bool) dnswiretest.Reply {
		soa := dnswiretest.RR{Name: "test", Type: dnswiretest.TypeSOA, Value: "ns.nic.test hostmaster.nic.test 1 3600 600 604800 300"}
		switch {
		case qtype == dnswiretest.TypeSOA && name == "test":
			return dnswiretest.Reply{Answer: []dnswiretest.RR{soa}}
		case qtype == dnswiretest.TypeNS && name == "test":
			return dnswiretest.Reply{Answer: []dnswiretest.RR{{Name: "test", Type: dnswiretest.TypeNS, Value: "ns.nic.test"}}}
		case qtype == dnswiretest.TypeA && name == "ns.nic.test":
			return dnswiretest.Reply{Answer: []dnswiretest.RR{{Name: name, Type: dnswiretest.TypeA, Value: "127.0.0.1"}}}
		}
		return dnswiretest.Reply{RCode: 3}
	})
	parent := dnswiretest.Start(t, func(name string, qtype uint16, recursive bool) dnswiretest.Reply {
		if recursive || name != "example.test" || qtype != dnswiretest.TypeNS {
			return dnswiretest.Reply{RCode: 5}
		}
		return dnswiretest.Reply{
			Authority:  []dnswiretest.RR{{Name: name, Type: dnswiretest.TypeNS, Value: "ns1.example.test"}, {Name: name, Type: dnswiretest.TypeNS, Value: "ns2.example.test"}},
			Additional: []dnswiretest.RR{{Name: "ns1.example.test", Type: dnswiretest.TypeA, Value: "203.0.113.10"}, {Name: "ns2.example.test", Type: dnswiretest.TypeA, Value: "192.0.2.99"}},
		}
	})
	main := dnswiretest.Start(t, func(name string, qtype uint16, recursive bool) dnswiretest.Reply {
		if recursive {
			return dnswiretest.Reply{RCode: 5}
		}
		switch {
		case name == "example.test" && qtype == dnswiretest.TypeSOA:
			return dnswiretest.Reply{Authoritative: true, Answer: []dnswiretest.RR{{Name: name, Type: dnswiretest.TypeSOA, Value: "ns1.example.test hostmaster.example.test 2026102801 3600 600 604800 300"}}}
		case name == "example.test" && qtype == dnswiretest.TypeA:
			return dnswiretest.Reply{Authoritative: true, Answer: []dnswiretest.RR{{Name: name, Type: dnswiretest.TypeA, Value: "192.0.2.80"}}}
		case name == "example.test" && qtype == dnswiretest.TypeTXT:
			return dnswiretest.Reply{Authoritative: true, Answer: []dnswiretest.RR{{Name: name, Type: dnswiretest.TypeTXT, Value: "something else"}}}
		}
		return dnswiretest.Reply{Authoritative: true, RCode: 3}
	})
	options := dnswire.DelegationOptions{Resolvers: []string{resolver.Address}, Address: func(ip string) string {
		switch {
		case ip == "127.0.0.1":
			return parent.Address
		case ip == "203.0.113.10" && mainAnswers:
			return main.Address
		}
		return "127.0.0.1:1"
	}}
	return options, main
}

func registrarDraft() serverSetupDraft {
	d := mailDraftForTest()
	d.ImportedRecords = []serverSetupImportedRecord{
		{Name: "example.test", Type: "A", Content: "192.0.2.80", Decision: "import"},
		{Name: "example.test", Type: "TXT", Content: "google-site-verification=abc", Decision: "import"},
		{Name: "old.example.test", Type: "A", Content: "192.0.2.9", Decision: "skip"},
	}
	return d
}

func itemStates(result serverSetupDelegation) map[string]string {
	states := map[string]string{}
	for _, item := range result.Items {
		states[item.ID] = item.State
	}
	return states
}

func TestRegistrarDelegationNamesWhatToEnterAndWhatTheInternetShows(t *testing.T) {
	options, _ := registrarFixture(t, true)
	p := &Panel{}
	result := p.checkServerSetupDelegation(context.Background(), registrarDraft(), "", options)
	if result.ExpectedNS[0].Name != "ns1.example.test" || result.ExpectedNS[0].Server != "main" || result.ExpectedNS[0].IPv4 != "203.0.113.10" || !result.ExpectedNS[0].GlueNeeded || result.ExpectedNS[1].IPv4 != "198.51.100.20" {
		t.Fatalf("expected nameservers: %+v", result.ExpectedNS)
	}
	states := itemStates(result)
	want := map[string]string{"answers_main": "ok", "answers_backup": "waiting", "glue_v4:ns1.example.test": "ok", "glue_v4:ns2.example.test": "not_yet", "nameservers": "ok", "imported": "not_yet"}
	for id, state := range want {
		if states[id] != state {
			t.Fatalf("%s = %q, want %q (all: %v)", id, states[id], state, states)
		}
	}
	if result.Parent != "test" || result.Servers[0].Serial != 2026102801 || !result.NowAllowed {
		t.Fatalf("parent/serial/now: %+v", result)
	}
	if result.Imported.Total != 2 || result.Imported.Matched != 1 || len(result.Imported.Diffs) != 1 || result.Imported.Diffs[0].Type != "TXT" {
		t.Fatalf("imported comparison: %+v", result.Imported)
	}
	for _, item := range result.Items {
		if item.ID == "glue_v4:ns2.example.test" && item.Vars["seen"] != "192.0.2.99" {
			t.Fatalf("the wrong glue must be named: %+v", item)
		}
	}
	// Outside the domain no glue is asked for.
	d := registrarDraft()
	d.NS1, d.NS2, d.PeerNS = "ns1.provider.test", "ns2.provider.test", "ns2.provider.test"
	if outside := serverSetupDelegationExpected(d, ""); outside[0].GlueNeeded || outside[1].GlueNeeded {
		t.Fatalf("glue only in-bailiwick: %+v", outside)
	}
	// On the backup DNS server, ns1 is the other server.
	d = registrarDraft()
	d.DNSRole = "secondary"
	if backup := serverSetupDelegationExpected(d, "2001:db8::20"); backup[0].IPv4 != "198.51.100.20" || backup[1].IPv4 != "203.0.113.10" || backup[1].IPv6 != "2001:db8::20" || backup[0].IPv6 != "" {
		t.Fatalf("roles: %+v", backup)
	}
}

func TestRegistrarDelegationRefusesNowUntilTheMainAnswers(t *testing.T) {
	options, _ := registrarFixture(t, false)
	result := (&Panel{}).checkServerSetupDelegation(context.Background(), registrarDraft(), "", options)
	if result.NowAllowed || result.NowReason != "server_setup_delegation_main_not_answering" {
		t.Fatalf("now must wait for the main DNS server: %+v", result)
	}
	if states := itemStates(result); states["answers_main"] != "unknown" || states["imported"] != "waiting" {
		t.Fatalf("states: %v", states)
	}
	options.Resolvers = []string{"127.0.0.1:1"}
	if states := itemStates((&Panel{}).checkServerSetupDelegation(context.Background(), registrarDraft(), "", options)); states["nameservers"] != "unknown" || states["glue_v4:ns1.example.test"] != "unknown" {
		t.Fatalf("an unreadable parent is unknown, never not yet: %v", states)
	}
}

func TestRegistrarDelegationEndpointIsReadOnlyAndShared(t *testing.T) {
	options, main := registrarFixture(t, true)
	previous, previousIPv6 := setupDelegationOptions, setupProviderRecordIPv6
	t.Cleanup(func() { setupDelegationOptions, setupProviderRecordIPv6 = previous, previousIPv6 })
	setupDelegationOptions = func() dnswire.DelegationOptions { return options }
	setupProviderRecordIPv6 = func() string { return "" }
	f, state := setupOperationFixture(t)
	d := registrarDraft()
	if _, err := f.panel.saveServerSetupDraft(context.Background(), state.Revision, d); err != nil {
		t.Fatal(err)
	}
	read := func(zone string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		f.panel.handleServerSetupDelegation(w, serviceOperationAdminRequest(t, http.MethodGet, serverSetupDelegationPath+"?zone="+zone, "", f.userID))
		return w
	}
	if w := read("other.test"); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "server_setup_delegation_not_applicable") {
		t.Fatalf("another zone: %d %s", w.Code, w.Body.String())
	}
	w := read("Example.TEST.")
	var value serverSetupDelegation
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &value) != nil || value.Zone != "example.test" || len(value.Items) == 0 {
		t.Fatalf("delegation: %d %s", w.Code, w.Body.String())
	}
	asked := len(main.Questions())
	read("example.test")
	if len(main.Questions()) != asked {
		t.Fatal("a read within a minute asked the servers again")
	}
	later := f.panel.serverSetupDelegation(context.Background(), d, time.Now().Add(2*time.Minute))
	if len(main.Questions()) == asked || later.CheckedAt == value.CheckedAt {
		t.Fatal("after a minute the check runs again")
	}
	if f.agent.installCalls.Load() != 0 || len(f.agent.capturedMutationEvents()) != 0 {
		t.Fatal("the registrar check mutated the host")
	}
}
