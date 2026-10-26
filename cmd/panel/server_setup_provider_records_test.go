package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// The read-only provider-records check of setup wizard v2 (alpha.111).

func TestServerSetupCatalogProviderRecordStates(t *testing.T) {
	for _, test := range []struct {
		answers  [][]string
		failures int
		want     string
	}{
		{[][]string{{"203.0.113.10"}, {"203.0.113.10"}}, 0, "ok"},
		{[][]string{nil, nil}, 0, "missing"},
		{[][]string{{"203.0.113.10"}, nil}, 0, "missing"},
		{[][]string{{"192.0.2.80"}, {"203.0.113.10"}}, 0, "different"},
		{[][]string{{"192.0.2.80"}}, 1, "different"},
		{[][]string{{"203.0.113.10"}}, 1, "unknown"},
		{nil, 2, "unknown"},
		{[][]string{nil}, 1, "unknown"},
	} {
		if got, _ := classifyServerSetupProviderRecord("203.0.113.10", test.answers, test.failures); got != test.want {
			t.Fatalf("%v failures=%d -> %s, want %s", test.answers, test.failures, got, test.want)
		}
	}
}

func TestServerSetupCatalogProviderRecordsEndpointIsBoundedAndCached(t *testing.T) {
	t.Setenv("CELIKPANEL_SERVER_IP", "203.0.113.10")
	f, state := setupOperationFixture(t)
	state.Draft.ShortName = "frankfurt"
	if _, err := f.panel.saveServerSetupDraft(context.Background(), state.Revision, state.Draft); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	answers := map[string]func() ([]string, error){
		"panel.example.test": func() ([]string, error) { return []string{"203.0.113.10"}, nil },
	}
	previous := setupProviderRecordResolvers
	t.Cleanup(func() { setupProviderRecordResolvers = previous })
	setupProviderRecordResolvers = func() []setupProviderRecordLookup {
		lookup := func(_ context.Context, name string, qtype uint16) ([]string, error) {
			calls.Add(1)
			if answer, ok := answers[name]; ok {
				return answer()
			}
			return nil, &net.DNSError{Err: "no such host", Name: name, IsNotFound: true}
		}
		return []setupProviderRecordLookup{lookup, lookup}
	}
	read := func() serverSetupProviderRecords {
		w := httptest.NewRecorder()
		f.panel.handleServerSetupProviderRecords(w, serviceOperationAdminRequest(t, http.MethodGet, serverSetupProviderRecordsPath, "", f.userID))
		var value serverSetupProviderRecords
		if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &value) != nil || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("provider records: %d %s", w.Code, w.Body.String())
		}
		return value
	}
	first := read()
	if len(first.Servers) != 1 || first.Servers[0].Label != "frankfurt" || first.Servers[0].IP4 != "203.0.113.10" || len(first.Servers[0].Rows) != 1 || first.Servers[0].Rows[0].State != "ok" || first.Mode != "external" {
		t.Fatalf("records: %+v", first)
	}
	if calls.Load() != 2 {
		t.Fatalf("expected one question per resolver, got %d", calls.Load())
	}
	read()
	if calls.Load() != 2 {
		t.Fatal("a poll within a minute asked the internet again")
	}
	// After the minute, a failing resolver makes the row unknown, never missing.
	answers["panel.example.test"] = func() ([]string, error) { return nil, errors.New("timeout") }
	later := f.panel.serverSetupProviderRecords(context.Background(), state.Draft, time.Now().Add(2*time.Minute))
	if later.Servers[0].Rows[0].State != "unknown" {
		t.Fatalf("a failed lookup must be unknown: %+v", later.Servers[0].Rows[0])
	}
	if f.agent.installCalls.Load() != 0 || len(f.agent.capturedMutationEvents()) != 0 {
		t.Fatal("provider records mutated the host")
	}
	r := httptest.NewRequest(http.MethodGet, serverSetupProviderRecordsPath, nil)
	r = r.WithContext(context.WithValue(r.Context(), callerKey, &Caller{Role: roleCustomer}))
	w := httptest.NewRecorder()
	f.panel.handleServerSetupProviderRecords(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("customer reached provider records: %d", w.Code)
	}
}
