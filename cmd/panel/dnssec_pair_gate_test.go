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

// alpha.124 lab (PowerDNS main + backup, sec124.test): every refused click
// advanced the zone's SOA serial and republished it, because the refusal came
// after the publication snapshot. With the status saying the pair cannot sign,
// the click is refused before anything is prepared: serial and publication
// state stay exactly as they were, and the Agent is never asked to sign.
func TestDNSSECPairRefusalChangesNothingInTheZone(t *testing.T) {
	const domain = "pair-quiet.example"
	p := newDNSPanelForTest(t)
	setDNSIdentityForTest(t, p, "standalone")
	domainID := seedDNSSECManagedDomain(t, p, domain)
	agent := &strictDNSRPCAgent{dnssecSigningUnsupported: transport.DNSSECSigningPairUnsupported}
	attachStrictDNSRPCAgent(t, p, agent)
	soa := func() string {
		var content string
		if err := p.db.GetDB().QueryRow(`SELECT r.content FROM pdns_records r JOIN pdns_domains d ON d.id = r.domain_id
			WHERE d.name = ? AND r.type = 'SOA' AND r.name = d.name`, domain).Scan(&content); err != nil {
			t.Fatal(err)
		}
		return content
	}
	beforeSOA, beforeState := soa(), strictDNSState(t, p, domain)
	for i := 0; i < 2; i++ {
		recorder := postDNSSECV2(t, p, domainID)
		var body apiErrorBody
		if recorder.Code != http.StatusConflict || json.Unmarshal(recorder.Body.Bytes(), &body) != nil ||
			body.Code != errCodeDNSSECPairUnsupported || body.Error != dnssecPairUnsupportedMessage {
			t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
		}
	}
	if after := soa(); after != beforeSOA {
		t.Fatalf("the SOA changed on a refused click: %q -> %q", beforeSOA, after)
	}
	afterState := strictDNSState(t, p, domain)
	if afterState.hasLease() || afterState.DesiredGeneration != beforeState.DesiredGeneration ||
		afterState.AppliedGeneration != beforeState.AppliedGeneration {
		t.Fatalf("publication state changed: %+v -> %+v", beforeState, afterState)
	}
	agent.mu.Lock()
	defer agent.mu.Unlock()
	if agent.secureDNSCalls != 0 || len(agent.syncV3Requests) != 0 || len(agent.syncRequests) != 0 {
		t.Fatalf("secure/syncV3/sync = %d/%d/%d after a refused click",
			agent.secureDNSCalls, len(agent.syncV3Requests), len(agent.syncRequests))
	}
}
