package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/dnswire"
	"github.com/alicelik/celikpanel/internal/dnswire/dnswiretest"
)

// The registrar check against loopback servers only: a recursive resolver,
// the parent zone's server and this setup's main DNS server.
func registrarFixture(t *testing.T, mainAnswers bool) (dnswire.DelegationOptions, *dnswiretest.Server) {
	t.Helper()
	return registrarFixtureWith(t, mainAnswers, nil, nil)
}

// registrarFixtureWith lets a test answer the main DNS server's questions
// (answer returns false to fall back to the default) and slow the parent.
func registrarFixtureWith(t *testing.T, mainAnswers bool, answer func(name string, qtype uint16) (dnswiretest.Reply, bool), parentDelay func()) (dnswire.DelegationOptions, *dnswiretest.Server) {
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
		if parentDelay != nil {
			parentDelay()
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
		if answer != nil && qtype != dnswiretest.TypeSOA {
			if reply, ok := answer(name, qtype); ok {
				return reply
			}
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
	// Imported A and TXT plus the mail records as published (merged SPF at
	// the apex TXT, MX, DMARC): only the A record answers the same.
	if result.Imported.Total != 4 || result.Imported.Matched != 1 || len(result.Imported.Diffs) != 3 || result.Imported.Unknown != 0 || !slices.ContainsFunc(result.Imported.Diffs, func(diff serverSetupDelegationDiff) bool { return diff.Name == "example.test" && diff.Type == "TXT" }) {
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

// publishedAnswers answers the main DNS server's questions with the records
// the zone publishes (plus extra values the owner added beside them).
func publishedAnswers(d serverSetupDraft, extra map[string][]dnswiretest.RR) func(string, uint16) (dnswiretest.Reply, bool) {
	return func(name string, qtype uint16) (dnswiretest.Reply, bool) {
		reply := dnswiretest.Reply{Authoritative: true}
		for _, record := range serverSetupDelegationPublished(d) {
			if record.Name == name && serverSetupRecordTypes[record.Type] == qtype {
				reply.Answer = append(reply.Answer, dnswiretest.RR{Name: name, Type: qtype, Value: serverSetupPublishedExpectation(record)})
			}
		}
		reply.Answer = append(reply.Answer, extra[name]...)
		if len(reply.Answer) == 0 {
			reply.RCode = 3
		}
		return reply, true
	}
}

// The expectation is the zone as published: an imported SPF is merged, an
// imported MX is replaced when mail moves now, an imported DMARC replaced
// when the owner adds the recommended one. A server answering exactly that
// (and a backup MX the owner added beside it) is done.
func TestRegistrarDelegationComparesRecordsAsPublished(t *testing.T) {
	d := mailDraftForTest()
	d.ImportedRecords = []serverSetupImportedRecord{
		{Name: "example.test", Type: "TXT", Content: "v=spf1 include:_spf.oldmail.test ~all", Decision: "import"},
		{Name: "example.test", Type: "MX", Content: "mx.oldmail.test", Priority: 5, Decision: "import"},
		{Name: "_dmarc.example.test", Type: "TXT", Content: "v=DMARC1; p=reject", Decision: "import"},
	}
	d.MailDNS = &serverSetupMailDNS{MX: "now", DMARC: "add"}
	if err := canonicalServerSetupMailAnswers(&d); err != nil {
		t.Fatal(err)
	}
	for _, record := range serverSetupDelegationPublished(d) {
		value := serverSetupPublishedExpectation(record)
		if strings.Contains(value, "mx.oldmail.test") || value == "v=spf1 include:_spf.oldmail.test ~all" || value == "v=DMARC1; p=reject" {
			t.Fatalf("a raw import replaced by the mail choice is expected: %+v", record)
		}
	}
	backupMX := map[string][]dnswiretest.RR{"example.test": {{Name: "example.test", Type: dnswiretest.TypeMX, Value: "20 backup-mx.example.test"}}}
	options, _ := registrarFixtureWith(t, true, publishedAnswers(d, backupMX), nil)
	result := (&Panel{}).checkServerSetupDelegation(context.Background(), d, "", options)
	if states := itemStates(result); states["imported"] != "ok" || result.Imported.Matched != result.Imported.Total || result.Imported.Total != 3 {
		t.Fatalf("records as published must match: %v %+v", states, result.Imported)
	}
}

// No reply, a reply that is not authoritative or REFUSED/SERVFAIL is
// unknown, never a difference; such an answer is not shared for a minute.
func TestRegistrarDelegationUnreadableAnswersAreUnknown(t *testing.T) {
	d := registrarDraft()
	var mode atomic.Int32
	answer := func(name string, qtype uint16) (dnswiretest.Reply, bool) {
		if name == "example.test" && qtype == dnswiretest.TypeA {
			return dnswiretest.Reply{}, false
		}
		switch mode.Load() {
		case 0:
			return dnswiretest.Reply{RCode: 5, Authoritative: true}, true
		case 1:
			return dnswiretest.Reply{RCode: 2, Authoritative: true}, true
		}
		return publishedAnswers(d, nil)(name, qtype)
	}
	options, main := registrarFixtureWith(t, true, answer, nil)
	for _, refused := range []int32{0, 1} {
		mode.Store(refused)
		result := (&Panel{}).checkServerSetupDelegation(context.Background(), d, "", options)
		if states := itemStates(result); states["imported"] != "unknown" || len(result.Imported.Diffs) != 0 || result.Imported.Unknown != 3 || result.Imported.Matched != 1 {
			t.Fatalf("mode %d: an unreadable answer is unknown: %v %+v", refused, states, result.Imported)
		}
	}
	// A non-authoritative answer (a resolver, not our server) is unknown too.
	mode.Store(2)
	nonAA := func(name string, qtype uint16) (dnswiretest.Reply, bool) {
		reply, ok := publishedAnswers(d, nil)(name, qtype)
		reply.Authoritative = false
		return reply, ok
	}
	options2, _ := registrarFixtureWith(t, true, nonAA, nil)
	if result := (&Panel{}).checkServerSetupDelegation(context.Background(), d, "", options2); itemStates(result)["imported"] != "unknown" || len(result.Imported.Diffs) != 0 {
		t.Fatalf("non-authoritative: %+v", result.Imported)
	}
	previous, previousIPv6 := setupDelegationOptions, setupProviderRecordIPv6
	t.Cleanup(func() { setupDelegationOptions, setupProviderRecordIPv6 = previous, previousIPv6 })
	setupDelegationOptions = func() dnswire.DelegationOptions { return options }
	setupProviderRecordIPv6 = func() string { return "" }
	mode.Store(0)
	p := &Panel{}
	now := time.Now()
	p.serverSetupDelegation(context.Background(), d, now)
	asked := len(main.Questions())
	p.serverSetupDelegation(context.Background(), d, now)
	if len(main.Questions()) == asked {
		t.Fatal("an answer with an unknown item must not be shared")
	}
	mode.Store(2)
	if itemStates(p.serverSetupDelegation(context.Background(), d, now))["imported"] != "ok" {
		t.Fatal("published answers must match")
	}
	asked = len(main.Questions())
	p.serverSetupDelegation(context.Background(), d, now)
	if len(main.Questions()) != asked {
		t.Fatal("a complete answer is shared for a minute")
	}
}

// The parent walk has its own time budget: a slow parent is unknown while
// our servers' answers are still compared, and a second caller waiting for
// the check in flight can give up when its own request ends.
func TestRegistrarDelegationSlowParentKeepsOurAnswers(t *testing.T) {
	previousBudget := serverSetupDelegationBudget
	t.Cleanup(func() { serverSetupDelegationBudget = previousBudget })
	serverSetupDelegationBudget = 400 * time.Millisecond
	d := registrarDraft()
	release := make(chan struct{})
	var once atomic.Bool
	t.Cleanup(func() {
		if once.CompareAndSwap(false, true) {
			close(release)
		}
	})
	options, _ := registrarFixtureWith(t, true, publishedAnswers(d, nil), func() {
		select {
		case <-release:
		case <-time.After(2 * time.Second):
		}
	})
	result := (&Panel{}).checkServerSetupDelegation(context.Background(), d, "", options)
	states := itemStates(result)
	if states["nameservers"] != "unknown" || states["imported"] != "ok" || states["answers_main"] != "ok" {
		t.Fatalf("a slow parent must not turn our answers into mismatches: %v", states)
	}
	previous, previousIPv6 := setupDelegationOptions, setupProviderRecordIPv6
	t.Cleanup(func() { setupDelegationOptions, setupProviderRecordIPv6 = previous, previousIPv6 })
	setupDelegationOptions = func() dnswire.DelegationOptions { return options }
	setupProviderRecordIPv6 = func() string { return "" }
	p := &Panel{}
	// The first check runs in its own goroutine; the test waits for it to end
	// before its seams are restored (registered last, so it runs first).
	first := make(chan struct{})
	go func() {
		defer close(first)
		p.serverSetupDelegation(context.Background(), d, time.Now())
	}()
	t.Cleanup(func() {
		if once.CompareAndSwap(false, true) {
			close(release)
		}
		<-first
	})
	deadline := time.Now().Add(5 * time.Second)
	for !serverSetupDelegationInFlight(p) {
		if time.Now().After(deadline) {
			t.Fatal("the first check never started")
		}
		runtime.Gosched()
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	begin := time.Now()
	waited := p.serverSetupDelegation(ctx, d, time.Now())
	if time.Since(begin) > 300*time.Millisecond || itemStates(waited)["check_running"] != "unknown" {
		t.Fatalf("a waiting caller gives up when its request ends, with a state: %+v", waited.Items)
	}
}
