package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/rpc"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.113 final acceptance: a just-issued certificate whose validity starts
// a few seconds after this server's time was discarded with a bare 500. Now a
// gap up to issuedCertificateClockWait is waited for; a larger one is the
// server clock's coded refusal.

type issuedClockAgent struct {
	verifiedAPTAgentRPCFixture

	mu       sync.Mutex
	readings []transport.ValidateCertResponse
	calls    int
}

func (a *issuedClockAgent) InspectInstalledCertificate(
	_ *transport.InspectCertificateRequest,
	resp *transport.ValidateCertResponse,
) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	index := a.calls
	if index >= len(a.readings) {
		index = len(a.readings) - 1
	}
	a.calls++
	*resp = a.readings[index]
	return nil
}

func attachIssuedClockAgent(t *testing.T, panel *Panel, agent *issuedClockAgent) {
	t.Helper()
	panel.pkgFamilyVal = "apt"
	server := rpc.NewServer()
	if err := server.RegisterName("Agent", agent); err != nil {
		t.Fatal(err)
	}
	connector := func(ctx context.Context) (*rpc.Client, error) {
		serverConn, clientConn := net.Pipe()
		go server.ServeConn(serverConn)
		return rpc.NewClient(clientConn), nil
	}
	client, err := connector(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	panel.agentClient = transport.NewReconnectingClientWithContextConnector(client, connector)
	t.Cleanup(func() { _ = client.Close() })
}

func stubIssuedCertificateSleep(t *testing.T) *[]time.Duration {
	t.Helper()
	old := issuedCertificateSleep
	var waits []time.Duration
	issuedCertificateSleep = func(_ context.Context, d time.Duration) error {
		waits = append(waits, d)
		return nil
	}
	t.Cleanup(func() { issuedCertificateSleep = old })
	return &waits
}

func notYetValidReading(checked time.Time, gap time.Duration) transport.ValidateCertResponse {
	return transport.ValidateCertResponse{
		Valid: false, NotYetValid: true, CheckedAt: checked, IssuedAt: checked.Add(gap),
		ExpiresAt: checked.Add(90 * 24 * time.Hour), DNSNames: []string{"shop.example"},
		Error: "certificate is not yet valid",
	}
}

func TestIssuedCertificateWaitsForASmallClockGap(t *testing.T) {
	panel, _ := newDomainAliasFixture(t)
	waits := stubIssuedCertificateSleep(t)
	checked := time.Date(2026, 10, 30, 15, 24, 20, 0, time.UTC)
	valid := transport.ValidateCertResponse{Valid: true, Trusted: true, TrustChecked: true, CheckedAt: checked.Add(3 * time.Second)}
	agent := &issuedClockAgent{readings: []transport.ValidateCertResponse{notYetValidReading(checked, 2*time.Second), valid}}
	attachIssuedClockAgent(t, panel, agent)

	info, err := panel.inspectIssuedCertificate(context.Background(), "shop.example", "/c", "/k", "/ch")
	if err != nil || !info.Valid || issuedCertificateClockProblem(info) {
		t.Fatalf("after the wait: %+v %v", info, err)
	}
	if len(*waits) != 1 || (*waits)[0] != 3*time.Second || agent.calls != 2 {
		t.Fatalf("waits = %v, inspections = %d; want one wait of 3s and two readings", *waits, agent.calls)
	}
}

func TestIssuedCertificateLargeClockGapIsTheServerClocksRefusal(t *testing.T) {
	panel, _ := newDomainAliasFixture(t)
	waits := stubIssuedCertificateSleep(t)
	checked := time.Date(2026, 10, 30, 15, 24, 20, 0, time.UTC)
	agent := &issuedClockAgent{readings: []transport.ValidateCertResponse{notYetValidReading(checked, 2*time.Hour)}}
	attachIssuedClockAgent(t, panel, agent)

	info, err := panel.inspectIssuedCertificate(context.Background(), "shop.example", "/c", "/k", "/ch")
	if err != nil || info.Valid || !issuedCertificateClockProblem(info) {
		t.Fatalf("large gap: %+v %v", info, err)
	}
	if len(*waits) != 0 || agent.calls != 1 {
		t.Fatalf("waits = %v, inspections = %d; want no wait", *waits, agent.calls)
	}

	recorder := httptest.NewRecorder()
	writeCertificateClockFailure(recorder, &Caller{Role: roleAdmin}, "shop.example", false, info)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", recorder.Code)
	}
	var body apiErrorBody
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != errCodeCertificateIssueFailed || body.Reason != transport.CertificateFailureServerClock ||
		body.Vars["valid_from"] != "2026-10-30T17:24:20Z" || body.Vars["server_time"] != "2026-10-30T15:24:20Z" ||
		body.Vars["detail"] == "" {
		t.Fatalf("body = %+v", body)
	}
	for _, want := range []string{"timedatectl", "this server's clock", "requests the certificate here again", "Nothing asks again automatically"} {
		if !strings.Contains(body.Error, want) {
			t.Fatalf("sentence lacks %q: %s", want, body.Error)
		}
	}

	customer := httptest.NewRecorder()
	writeCertificateClockFailure(customer, &Caller{Role: "customer"}, "shop.example", true, info)
	var customerBody apiErrorBody
	if err := json.Unmarshal(customer.Body.Bytes(), &customerBody); err != nil {
		t.Fatal(err)
	}
	if customerBody.Vars["detail"] != "" || customerBody.Vars["kept"] != "previous" {
		t.Fatalf("customer body = %+v", customerBody)
	}
}

func TestIssuedCertificateWaitEndsWithTheRequest(t *testing.T) {
	panel, _ := newDomainAliasFixture(t)
	checked := time.Date(2026, 10, 30, 15, 24, 20, 0, time.UTC)
	agent := &issuedClockAgent{readings: []transport.ValidateCertResponse{notYetValidReading(checked, time.Minute)}}
	attachIssuedClockAgent(t, panel, agent)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	started := time.Now()
	if _, err := panel.inspectIssuedCertificate(ctx, "shop.example", "/c", "/k", "/ch"); err == nil {
		t.Fatal("a request that ended kept waiting")
	}
	if elapsed := time.Since(started); elapsed > 10*time.Second || agent.calls != 1 {
		t.Fatalf("waited %s with %d readings; want the wait to end with the request", elapsed, agent.calls)
	}
}
