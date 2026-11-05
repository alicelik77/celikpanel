package main

import (
	"context"
	"encoding/json"
	"testing"
)

// alpha.124 (Frankfurt, 2026-11-05): the first server's own name
// (frankfurt.<zone>, hostname -f) had no A record in the zone setup created;
// the dashboard later asked for it. The reviewed plan now has it with the
// rest; an owner's different record at that name is kept, never overwritten;
// a draft without the name (every older one) plans exactly as before.
func TestServerSetupPlanCarriesTheMachinesOwnName(t *testing.T) {
	p := newDNSPanelForTest(t)
	d := infrastructureDNSTestDraft()
	d.PanelDomain = "panel.example.test"
	d.HostName = "frankfurt.example.test"
	plan, err := p.buildServerSetupInfrastructureDNSPlan(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, r := range plan.Records {
		if r.Name == d.HostName {
			found++
			if r.Type != "A" || r.Content != d.LocalIP || r.Action != "add" || !r.Host {
				t.Fatalf("the machine's record: %+v", r)
			}
		}
	}
	if found != 1 {
		t.Fatalf("the machine's own name is not in the plan once: %d %+v", found, plan.Records)
	}

	// Older drafts (and extensions of installs set up before) have no name:
	// the plan is unchanged.
	older := d
	older.HostName = ""
	plain, err := serverSetupInfrastructureDNSDesired(older)
	if err != nil {
		t.Fatal(err)
	}
	with, err := serverSetupInfrastructureDNSDesired(d)
	if err != nil || len(with) != len(plain)+1 {
		t.Fatalf("one record more with the name: %d vs %d (%v)", len(with), len(plain), err)
	}
	raw, _ := json.Marshal(older)
	if json.Valid(raw) && containsJSONKey(raw, "host_name") {
		t.Fatal("an older draft gained host_name")
	}

	// A name outside the zone, the apex or one of the names already planned
	// adds nothing.
	for _, name := range []string{"frankfurt.other.test", "example.test", "panel.example.test", "ns1.example.test"} {
		other := d
		other.HostName = name
		records, err := serverSetupInfrastructureDNSDesired(other)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range records {
			if r.Host {
				t.Fatalf("%s added a machine record: %+v", name, r)
			}
		}
	}
}

func TestServerSetupKeepsTheOwnersRecordForTheMachinesName(t *testing.T) {
	d := infrastructureDNSTestDraft()
	d.PanelDomain = "panel.example.test"
	d.HostName = "frankfurt.example.test"
	base := []serverSetupInfrastructureDNSStoredRecord{
		{ID: 1, Name: "example.test", Type: "SOA", Content: "ns1.example.test hostmaster.example.test 2026110501 3600 600 604800 300", TTL: 3600},
		{ID: 2, Name: "example.test", Type: "NS", Content: "ns1.example.test", TTL: 3600},
		{ID: 3, Name: "example.test", Type: "NS", Content: "ns2.example.test", TTL: 3600},
	}
	for _, tc := range []struct {
		name   string
		record serverSetupInfrastructureDNSStoredRecord
		action string // "" = left out
	}{
		{"same address: kept as it is", serverSetupInfrastructureDNSStoredRecord{ID: 9, Name: "frankfurt.example.test", Type: "A", Content: d.LocalIP, TTL: 600}, "keep"},
		{"owner's other address: left out", serverSetupInfrastructureDNSStoredRecord{ID: 9, Name: "frankfurt.example.test", Type: "A", Content: "198.51.100.77", TTL: 600}, ""},
		{"owner's alias: left out", serverSetupInfrastructureDNSStoredRecord{ID: 9, Name: "frankfurt.example.test", Type: "CNAME", Content: "elsewhere.example.net", TTL: 600}, ""},
	} {
		snapshot := serverSetupInfrastructureDNSSnapshot{Zone: "example.test", ZoneID: 7, ZoneType: "MASTER", Records: append(append([]serverSetupInfrastructureDNSStoredRecord(nil), base...), tc.record)}
		plan, err := buildServerSetupInfrastructureDNSAgainstSnapshot(d, snapshot)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		action := ""
		for _, r := range plan.Records {
			if r.Name == d.HostName && r.Type == "A" {
				action = r.Action
			}
		}
		if action != tc.action {
			t.Fatalf("%s: action %q, want %q (%+v)", tc.name, action, tc.action, plan.Records)
		}
	}
}

func TestServerSetupDraftHostNameIsTheMachinesOnFirstSetupOnly(t *testing.T) {
	saved := localServerName
	t.Cleanup(func() { localServerName = saved })
	localServerName = func() (string, error) { return "Frankfurt.Example.Test.", nil }
	if got := serverSetupDraftHostName("", `{}`); got != "frankfurt.example.test" {
		t.Fatalf("first setup: %q", got)
	}
	// A completed setup keeps what its stored draft had (nothing, for an
	// install set up before alpha.124).
	if got := serverSetupDraftHostName("2026-10-29T10:00:00Z", `{"panel_domain":"panel.example.test"}`); got != "" {
		t.Fatalf("extension of an older install: %q", got)
	}
	if got := serverSetupDraftHostName("2026-11-06T10:00:00Z", `{"host_name":"frankfurt.example.test"}`); got != "frankfurt.example.test" {
		t.Fatalf("extension keeps the stored name: %q", got)
	}
	localServerName = func() (string, error) { return "localhost", nil }
	if got := serverSetupDraftHostName("", `{}`); got != "" {
		t.Fatalf("a name that is not fully qualified: %q", got)
	}
}

func containsJSONKey(raw []byte, key string) bool {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return false
	}
	_, ok := fields[key]
	return ok
}
