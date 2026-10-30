package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Arch PowerDNS follow-up drill (2026-10-30): a record deletion eight seconds
// after an addition was refused with DNS_SERVER_REQUIRED ("no DNS engine is
// active") while the backup was still confirming the catalog. A paired primary
// that is only waiting for its backup now waits briefly and, if the backup
// still has not confirmed, says so with its own code. Component test only.
func TestDNSMutationWaitsForPairedPrimaryBackup(t *testing.T) {
	previousAttempts, previousInterval := dnsPeerCatchUpAttempts, dnsPeerCatchUpInterval
	dnsPeerCatchUpAttempts, dnsPeerCatchUpInterval = 3, time.Millisecond
	t.Cleanup(func() { dnsPeerCatchUpAttempts, dnsPeerCatchUpInterval = previousAttempts, previousInterval })

	t.Setenv("CELIKPANEL_SERVER_IP", "192.0.2.10")
	panel := newDNSPanelForTest(t)
	setDNSIdentityForTest(t, panel, "paired")
	if err := panel.setSetting(context.Background(), settingDNSPeerIP, "192.0.2.20"); err != nil {
		t.Fatal(err)
	}
	if err := panel.setSetting(context.Background(), settingDNSPeerNS, "ns2.celikhost.com"); err != nil {
		t.Fatal(err)
	}
	agent := newDNSEngineTestAgent()
	attachDNSEngineTestAgent(t, panel, agent)
	preview, recorder := requestDNSEnginePreview(t, panel, transport.DNSEngineBIND, nil, 0)
	if recorder.Code != http.StatusOK || len(preview.Blockers) != 0 {
		t.Fatalf("preview status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if commit := commitDNSEngineSwitch(t, panel, strings.Repeat("7", 32), transport.DNSEngineBIND,
		nil, 0, preview.PreviewToken, false); commit.Code != http.StatusOK {
		t.Fatalf("commit status=%d body=%s", commit.Code, commit.Body.String())
	}
	setPairReady := func(ready bool) {
		agent.mu.Lock()
		runtime := agent.runtimes[transport.DNSEngineBIND]
		runtime.PairReady = ready
		agent.runtimes[transport.DNSEngineBIND] = runtime
		agent.mu.Unlock()
	}

	// The backup catches up during the wait: the mutation proceeds.
	setPairReady(false)
	agent.mu.Lock()
	start := agent.readinessCalls
	agent.onReadiness = func(call int) {
		if call == start+2 {
			setPairReady(true)
		}
	}
	agent.mu.Unlock()
	w := httptest.NewRecorder()
	if identity, ready := panel.requireActiveDNSPublisherForMutation(w, context.Background()); !ready ||
		identity.PairRole != transport.DNSPairRolePrimary || w.Body.Len() != 0 {
		t.Fatalf("caught-up backup refused: ready=%v body=%s", ready, w.Body.String())
	}

	// It does not catch up within the bound: its own code, not "no DNS engine".
	agent.mu.Lock()
	agent.onReadiness = nil
	agent.mu.Unlock()
	setPairReady(false)
	w = httptest.NewRecorder()
	if _, ready := panel.requireActiveDNSPublisherForMutation(w, context.Background()); ready {
		t.Fatal("unconfirmed backup accepted")
	}
	body := w.Body.String()
	if w.Code != http.StatusConflict || !strings.Contains(body, errCodeDNSPeerCatchingUp) ||
		strings.Contains(body, errCodeDNSServerRequired) || !strings.Contains(body, "Nothing was changed") ||
		!strings.Contains(body, "TCP port 53") {
		t.Fatalf("refusal status=%d body=%s", w.Code, body)
	}
}
