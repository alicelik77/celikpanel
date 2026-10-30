package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// The readiness answer names a retained DNS switch journal (drill apd-e,
// 2026-10-30): the Panel must not call that operation undone and setup must
// not start an install the journal would refuse. Component test only.
func TestDNSBackendReadinessReportsRetainedSwitchJournal(t *testing.T) {
	useFakeDNSEngineBackend(t, &fakeDNSEngineBackend{
		readiness: []transport.DNSBackendRuntimeState{
			{Engine: transport.DNSEngineBIND, Unit: "named.service"},
			{Engine: transport.DNSEnginePowerDNS, Unit: "pdns.service"},
		},
	})
	previous := readDNSBackendReadinessJournal
	t.Cleanup(func() { readDNSBackendReadinessJournal = previous })
	requestID := strings.Repeat("a", 32)
	for name, tc := range map[string]struct {
		read       func() (dnsEngineSwitchJournal, bool, error)
		request    string
		unverified bool
	}{
		"absent": {read: func() (dnsEngineSwitchJournal, bool, error) { return dnsEngineSwitchJournal{}, false, nil }},
		"retained": {read: func() (dnsEngineSwitchJournal, bool, error) {
			return dnsEngineSwitchJournal{MutationRequestID: requestID}, true, nil
		}, request: requestID},
		"unreadable": {read: func() (dnsEngineSwitchJournal, bool, error) {
			return dnsEngineSwitchJournal{}, false, errors.New("/var/lib/private detail")
		}, unverified: true},
		"malformed identity": {read: func() (dnsEngineSwitchJournal, bool, error) {
			return dnsEngineSwitchJournal{MutationRequestID: "not-an-id"}, true, nil
		}, unverified: true},
	} {
		t.Run(name, func(t *testing.T) {
			readDNSBackendReadinessJournal = tc.read
			var response DNSBackendReadinessResponse
			if err := (&Agent{}).DNSBackendReadiness(&transport.Empty{}, &response); err != nil {
				t.Fatal(err)
			}
			if response.Error != "" || response.SwitchJournalRequestID != tc.request ||
				response.SwitchJournalUnverified != tc.unverified || len(response.Engines) != 2 {
				t.Fatalf("readiness=%+v", response)
			}
		})
	}
}
