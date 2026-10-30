package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Arch PowerDNS follow-up drill (2026-10-30): after a paired zone deletion
// waited for the backup's confirmation, a later record change was refused as
// a generic DNS_PUBLICATION_FAILED. A refusal caused only by the Agent's
// pending exact V3 publication now names that cause. Component test only.
func TestDNSPublicationHeldBehindPendingRecovery(t *testing.T) {
	held := &dnsPublicationError{Result: dnsSyncAllResult{Attempted: 1, Failures: []dnsZoneSyncFailure{
		{Zone: "example.test", Err: &dnsAgentPublicationError{Err: errors.New(dnsAgentPendingRecoveryRefusal)}},
	}}}
	w := httptest.NewRecorder()
	if !writeDNSPublicationConflict(w, held, "generic") {
		t.Fatal("publication error not handled")
	}
	body := w.Body.String()
	if w.Code != http.StatusConflict || !strings.Contains(body, errCodeDNSPublicationHeld) ||
		!strings.Contains(body, "not published yet") || !strings.Contains(body, "dns-peer-enroll") {
		t.Fatalf("held refusal=%d %s", w.Code, body)
	}
	mixed := &dnsPublicationError{Result: dnsSyncAllResult{Attempted: 2, Failures: []dnsZoneSyncFailure{
		held.Result.Failures[0],
		{Zone: "other.test", Err: &dnsAgentPublicationError{Err: errors.New("zone transfer refused")}},
	}}}
	w = httptest.NewRecorder()
	writeDNSPublicationConflict(w, mixed, "generic")
	if !strings.Contains(w.Body.String(), errCodeDNSPublicationFailed) || strings.Contains(w.Body.String(), errCodeDNSPublicationHeld) {
		t.Fatalf("a different failure was reported as held: %s", w.Body.String())
	}
}
