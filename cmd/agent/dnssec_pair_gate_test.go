package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.124 (Frankfurt, 2026-11-05): PowerDNS IS the active engine, in a
// primary/secondary pair kept in step by the V3 catalog. Signing is refused
// with nothing changed; the refusal is typed and says the real reason (it
// used to say PowerDNS was not the active engine), and the status says it in
// advance.
func TestDNSSECSigningOnAV3PairIsATypedRefusalWithTheRealReason(t *testing.T) {
	manager, binding := installDNSSECV2TestLease(t, "example.test")
	t.Cleanup(func() { releasePoisonedDNSZoneSyncTestManager(manager) })
	oldMutation := legacyPowerDNSMutationAuthorityCheck
	legacyPowerDNSMutationAuthorityCheck = func(bool) error { return errDirectionalPowerDNSNeedsV3 }
	oldLookPath, oldCommand := dnssecLookPath, dnssecV2Command
	hostCalls := 0
	dnssecLookPath = func(string) (string, error) { hostCalls++; return "", errors.New("unexpected") }
	dnssecV2Command = func(context.Context, string, ...string) ([]byte, error) {
		hostCalls++
		return nil, errors.New("unexpected")
	}
	t.Cleanup(func() {
		legacyPowerDNSMutationAuthorityCheck = oldMutation
		dnssecLookPath, dnssecV2Command = oldLookPath, oldCommand
	})
	var response SecureDNSZoneV2Response
	if err := (&Agent{}).SecureDNSZoneV2(&SecureDNSZoneV2Request{ServiceMutationBinding: binding, Zone: "example.test"}, &response); err != nil {
		t.Fatal(err)
	}
	if response.ErrorCode != transport.DNSSECSigningPairUnsupported || response.Secured || hostCalls != 0 ||
		strings.Contains(response.Error, "not the active DNS engine") || !strings.Contains(response.Error, "Nothing was changed") {
		t.Fatalf("response=%+v hostCalls=%d", response, hostCalls)
	}
}

func TestDNSSECStatusSaysInAdvanceThatThePairCannotBeSigned(t *testing.T) {
	oldDurable, oldMutation := legacyPowerDNSDurableAuthorityCheck, legacyPowerDNSMutationAuthorityCheck
	oldLookPath, oldCommand := dnssecLookPath, dnssecCommand
	legacyPowerDNSDurableAuthorityCheck = func(bool) error { return nil }
	dnssecLookPath = func(string) (string, error) { return "pdnsutil", nil }
	dnssecCommand = func(string, ...string) ([]byte, error) {
		return []byte("This is a Master zone\nZone is not actively secured\n"), nil
	}
	t.Cleanup(func() {
		legacyPowerDNSDurableAuthorityCheck, legacyPowerDNSMutationAuthorityCheck = oldDurable, oldMutation
		dnssecLookPath, dnssecCommand = oldLookPath, oldCommand
	})
	for _, tc := range []struct {
		authority error
		want      string
	}{
		{errDirectionalPowerDNSNeedsV3, transport.DNSSECSigningPairUnsupported},
		{nil, ""},
		{errors.New("some other mutation refusal"), ""},
	} {
		legacyPowerDNSMutationAuthorityCheck = func(bool) error { return tc.authority }
		var response DNSSECStatusResponse
		if err := (&Agent{}).DNSSECStatus(&DNSSECRequest{Zone: "example.test"}, &response); err != nil {
			t.Fatal(err)
		}
		if response.Error != "" || response.Secured || response.SigningUnsupported != tc.want {
			t.Fatalf("%v: %+v", tc.authority, response)
		}
	}
}
