package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// set11 cell A6 (2026-10-11): the probe asks the address the site's port-80
// block listens on. An address that is not this server's cannot be asked
// from here: the request goes ahead, as for a ready probe, and the answer
// says the probe did not run; the authority's own attempt is the test.

func issueOnBoundKeptFile(t *testing.T, domain string, agent *validationRestoreAgent) (*httptest.ResponseRecorder, *validationRestoreAgent, *Panel, int) {
	t.Helper()
	p, domainID := newIncludeMailFixture(t, domain)
	agent.domain = domain
	attachValidationRestoreAgent(t, p, agent)
	recorder := httptest.NewRecorder()
	p.handleIssueLetsEncrypt(recorder, httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/v1/domains/%d/ssl/letsencrypt", domainID),
		strings.NewReader(`{"email":"admin@`+domain+`"}`)))
	return recorder, agent, p, domainID
}

func TestAnAddressThisServerCannotReachGoesAheadAndSaysTheProbeDidNotRun(t *testing.T) {
	const domain = "bound-remote.example"
	recorder, agent, p, domainID := issueOnBoundKeptFile(t, domain, &validationRestoreAgent{
		keptValidation: transport.SiteFileValidationUnknown, keptName: domain,
		keptAddress: "192.0.2.7", keptUnknownReason: transport.SiteFileValidationUnknownAddressNotLocal,
	})
	if recorder.Code != http.StatusOK {
		t.Fatalf("%d %s", recorder.Code, recorder.Body.String())
	}
	var answer map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}
	if answer["validation_probe"] != "not_run" || answer["validation_unknown_reason"] != transport.SiteFileValidationUnknownAddressNotLocal ||
		answer["validation_address"] != "192.0.2.7" {
		t.Fatalf("answer = %v", answer)
	}
	note, _ := answer["validation_note"].(string)
	for _, want := range []string{"could not check beforehand", "the site listens on 192.0.2.7, which this server cannot reach from itself",
		"The request was sent anyway; if the authority cannot reach 192.0.2.7, the answer will say so."} {
		if !strings.Contains(note, want) {
			t.Fatalf("missing %q in %q", want, note)
		}
	}
	agent.mu.Lock()
	issues := len(agent.issueCalls)
	agent.mu.Unlock()
	if issues != 1 {
		t.Fatalf("issue calls = %d, want 1", issues)
	}
	// Nothing was measured: no certificate_validation reason is written.
	if reason := siteFileReason(t, p, domainID); reason == "certificate_validation" {
		t.Fatalf("ledger reason = %q", reason)
	}
}

func TestALocalReasonForAnUnknownProbeStillRefusesWithTheAddress(t *testing.T) {
	recorder, agent, _, _ := issueOnBoundKeptFile(t, "bound-down.example", &validationRestoreAgent{
		keptValidation: transport.SiteFileValidationUnknown, keptName: "bound-down.example", keptAddress: "10.0.2.15",
	})
	var body apiErrorBody
	_ = json.Unmarshal(recorder.Body.Bytes(), &body)
	if recorder.Code != http.StatusServiceUnavailable || body.Code != errCodeCertificateValidationUnknown || body.Reason != "" ||
		body.Vars["address"] != "10.0.2.15" || !strings.Contains(body.Error, "because nginx on this server did not answer") {
		t.Fatalf("%d %+v", recorder.Code, body)
	}
	agent.mu.Lock()
	issues := len(agent.issueCalls)
	agent.mu.Unlock()
	if issues != 0 {
		t.Fatalf("issue calls = %d", issues)
	}
}

func TestARefusalNamesTheAddressThatAnswered(t *testing.T) {
	const domain = "bound-missing.example"
	recorder, _, _, _ := issueOnBoundKeptFile(t, domain, &validationRestoreAgent{
		keptValidation: transport.SiteFileValidationIncludeMissing, keptName: domain,
		keptStatus: http.StatusNotFound, keptAddress: "10.0.2.15",
	})
	var body apiErrorBody
	_ = json.Unmarshal(recorder.Body.Bytes(), &body)
	if recorder.Code != http.StatusConflict || body.Detail != transport.SiteFileValidationIncludeMissing ||
		body.Vars["address"] != "10.0.2.15" || body.Vars["name"] != domain || body.Vars["status"] != "404" {
		t.Fatalf("%d %+v", recorder.Code, body)
	}
}

func TestARenewalGoesAheadWhenTheProbeCannotBeSentFromThisServer(t *testing.T) {
	p, domainID, certificateID, agent := newValidationRestoreRenewalFixture(t, "exact")
	agent.keptValidation = transport.SiteFileValidationUnknown
	agent.keptAddress, agent.keptUnknownReason = "192.0.2.7", transport.SiteFileValidationUnknownAddressNotLocal
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	p.renewLetsEncrypt(ctx, certificateID, domainID, "ssl-state.example")
	agent.mu.Lock()
	renews := agent.renewCalls
	agent.mu.Unlock()
	if renews != 1 {
		t.Fatalf("renew calls = %d, want 1", renews)
	}
}
