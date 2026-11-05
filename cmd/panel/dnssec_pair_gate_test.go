package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.124 (Frankfurt, 2026-11-05): PowerDNS main with a backup server in
// step through the V3 catalog. The tab offered "Sign the zone", the Agent
// refused (nothing changed) and the screen showed an internal error. The
// status now says before the click that signing is closed, with the reason.
func TestDNSSECStatusSaysSigningIsClosedForThePair(t *testing.T) {
	for _, tc := range []struct {
		unsupported string
		available   bool
	}{
		{transport.DNSSECSigningPairUnsupported, false},
		{"", true}, // an older Agent: nothing known in advance
	} {
		p := newDNSPanelForTest(t)
		domainID := seedDNSSECHandlerDomain(t, p, "pair-gate.example")
		agent := &strictDNSRPCAgent{dnssecSigningUnsupported: tc.unsupported}
		attachStrictDNSRPCAgent(t, p, agent)
		recorder := httptest.NewRecorder()
		p.handleDomainDNSSEC(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/domains/1/dnssec", nil), domainID)
		var body struct {
			Signing dnssecSigningGate `json:"signing"`
		}
		if recorder.Code != http.StatusOK || json.Unmarshal(recorder.Body.Bytes(), &body) != nil {
			t.Fatalf("status %d %s", recorder.Code, recorder.Body.String())
		}
		if body.Signing.Available != tc.available {
			t.Fatalf("%q: %+v", tc.unsupported, body.Signing)
		}
		if !tc.available && (body.Signing.Reason != errCodeDNSSECPairUnsupported || !strings.Contains(body.Signing.Message, "Nothing was changed")) {
			t.Fatalf("the reason is not said: %+v", body.Signing)
		}
	}
}

// Should the Agent still refuse, the answer is a typed 409 with the same
// guidance, never a 500 internal error.
func TestDNSSECPairRefusalIsATypedConflict(t *testing.T) {
	const domain = "pair-refusal.example"
	p := newDNSPanelForTest(t)
	setDNSIdentityForTest(t, p, "standalone")
	domainID := seedDNSSECManagedDomain(t, p, domain)
	agent := &strictDNSRPCAgent{secureResponseError: "DNSSEC signing is not available yet for a PowerDNS primary/secondary pair"}
	agent.secureHook = func(_ transport.SecureDNSZoneV2Request, resp *transport.SecureDNSZoneV2Response) error {
		resp.ErrorCode = transport.DNSSECSigningPairUnsupported
		return nil
	}
	attachStrictDNSRPCAgent(t, p, agent)
	recorder := postDNSSECV2(t, p, domainID)
	var body apiErrorBody
	if recorder.Code != http.StatusConflict || json.Unmarshal(recorder.Body.Bytes(), &body) != nil ||
		body.Code != errCodeDNSSECPairUnsupported || body.Error != dnssecPairUnsupportedMessage {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "V3") || strings.Contains(recorder.Body.String(), "authority") {
		t.Fatalf("internal words reached the screen: %s", recorder.Body.String())
	}
}
