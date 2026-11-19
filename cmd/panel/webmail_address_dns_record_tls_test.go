package main

import (
	"context"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// alpha.140: "Add" for webmail's address record against the production host
// record receiver over the real TLS transport (the alpha.135 lane replaced
// the transport and seeded no child zone, so it never met what the lab pair
// met: a customer zone beside the main zone, and a receiver held behind a
// long DNS change).

// overTLS sends this server's requests to the main server's production
// machine stack over real TLS; handler wraps it (nil: the receiver as is).
func (f *dnsAddFixture) overTLS(t *testing.T, handler func(next http.Handler) http.Handler) {
	t.Helper()
	var stack http.Handler = f.receiver.requireRemoteDNSMachineAuth(csrfProtect(f.receiver.requireAuth(http.HandlerFunc(f.receiver.handleRemoteDNSMachine))))
	if handler != nil {
		stack = handler(stack)
	}
	endpoint, deps, _ := remoteDNSTestTransport(t, stack)
	remoteDNSExchange = func(ctx context.Context, _, path, secret string, request, response any) error {
		return remoteDNSHTTPJSONWith(ctx, endpoint, path, secret, request, response, deps)
	}
}

func (f *dnsAddFixture) originAudits(t *testing.T, action string) int {
	t.Helper()
	var n int
	f.origin.db.GetDB().QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action=?`, action).Scan(&n)
	return n
}

func TestWebmailAddressDNSRecordOverTLSBesideACustomerZoneIsAdded(t *testing.T) {
	f := newDNSAddFixture(t, "")
	var publishes atomic.Int32
	remoteDNSPublishReceived = func(*Panel, context.Context, string, bool) error {
		publishes.Add(1)
		return nil
	}
	f.overTLS(t, nil)
	// The lab's situation: the main server hosts a customer zone under its
	// main domain beside the infrastructure zone.
	seedHostRecordChildZone(t, f.receiver, "shop.example.test")
	answer := f.add(t, dnsAddHost, strings.Repeat("a", 32))
	if outcomes(answer.record) != "A=added" || answer.record.Server != "dns.remote.test" {
		t.Fatalf("answer: %+v", answer)
	}
	if got := f.receiverAddresses(t, dnsAddHost, "A"); len(got) != 1 || got[0] != hostRecordPeerIP || publishes.Load() != 1 {
		t.Fatalf("receiver: %v publishes=%d", got, publishes.Load())
	}
	if f.originAudits(t, "webmail.address.dns_record.added:"+dnsAddHost) != 1 {
		t.Fatal("not audited as added")
	}
	// A name the customer zone answers for is refused there, nothing written.
	if answer := f.add(t, "webmail.shop.example.test", strings.Repeat("b", 32)); outcomes(answer.record) != "A=refused:outside_zone" {
		t.Fatalf("child zone name: %+v", answer.record)
	}
	if len(f.receiverAddresses(t, "webmail.shop.example.test", "A")) != 0 {
		t.Fatal("written into the customer's zone")
	}
}

func TestWebmailAddressDNSRecordOverTLSBusyAddsNothingAndAskingAgainAddsIt(t *testing.T) {
	f := newDNSAddFixture(t, "2001:4860::5")
	shortenRemoteDNSReceiverLockWait(t, 200*time.Millisecond)
	remoteDNSPublishReceived = func(*Panel, context.Context, string, bool) error { return nil }
	f.overTLS(t, nil)
	f.receiver.serviceMutationMu.Lock()
	answer := f.add(t, dnsAddHost, strings.Repeat("a", 32))
	f.receiver.serviceMutationMu.Unlock()
	// Busy: nothing applied, and the optional AAAA is not tried after it.
	if outcomes(answer.record) != "A=busy AAAA=not_sent:skipped" || f.fromExchanges != 0 {
		t.Fatalf("busy: %+v from=%d", answer.record, f.fromExchanges)
	}
	if len(f.receiverAddresses(t, dnsAddHost, "A")) != 0 || hostRecordLedgerCount(t, f.receiver) != 0 {
		t.Fatal("a busy answer applied something")
	}
	if f.originAudits(t, "webmail.address.dns_record.busy:"+dnsAddHost) != 1 {
		t.Fatal("busy not audited")
	}
	// A new request (new identity) after release adds it.
	if again := f.add(t, dnsAddHost, strings.Repeat("b", 32)); !strings.HasPrefix(outcomes(again.record), "A=added") {
		t.Fatalf("again: %+v", again.record)
	}
	if got := f.receiverAddresses(t, dnsAddHost, "A"); len(got) != 1 {
		t.Fatalf("records: %v", got)
	}
}

// The publication outlasts the transport's 10-second answer limit: the first
// request is "unknown: no answer"; the main server completes it; a new
// request is answered "present" and nothing is written twice.
func TestWebmailAddressDNSRecordOverTLSNoAnswerThenPresentWritesOnce(t *testing.T) {
	f := newDNSAddFixture(t, "")
	release := make(chan struct{})
	var publishes atomic.Int32
	remoteDNSPublishReceived = func(*Panel, context.Context, string, bool) error {
		if publishes.Add(1) == 1 {
			<-release
		}
		return nil
	}
	f.overTLS(t, nil)
	started := time.Now()
	first := f.add(t, dnsAddHost, strings.Repeat("a", 32))
	if outcomes(first.record) != "A=unknown:no_answer" || time.Since(started) > 20*time.Second {
		t.Fatalf("first: %+v after %s", first.record, time.Since(started))
	}
	close(release)
	second := f.add(t, dnsAddHost, strings.Repeat("b", 32))
	if outcomes(second.record) != "A=present" {
		t.Fatalf("second: %+v", second.record)
	}
	if got := f.receiverAddresses(t, dnsAddHost, "A"); len(got) != 1 || hostRecordLedgerCount(t, f.receiver) != 1 {
		t.Fatalf("records=%v ledger=%d", got, hostRecordLedgerCount(t, f.receiver))
	}
	// "present" is recorded as added on this server.
	if f.originAudits(t, "webmail.address.dns_record.unknown:"+dnsAddHost) != 1 || f.originAudits(t, "webmail.address.dns_record.added:"+dnsAddHost) != 1 {
		t.Fatal("audits")
	}
}

// An alpha.137 main server answers CONFLICT also for a reason in the zone
// itself (a customer zone under its main domain): read as "taken", whose
// sentence is true for both meanings (web test), and nothing else is tried.
func TestWebmailAddressDNSRecordOverTLSAnOlderMainServersConflict(t *testing.T) {
	f := newDNSAddFixture(t, "2001:4860::5")
	f.overTLS(t, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == remoteDNSHostRecordPath {
				writeCodedError(w, http.StatusConflict, errCodeRemoteDNSHostRecordConflict, "The infrastructure zone cannot take this record now: the infrastructure zone overlaps another managed DNS zone; review its delegation first.", "")
				return
			}
			next.ServeHTTP(w, r)
		})
	})
	if answer := f.add(t, dnsAddHost, strings.Repeat("a", 32)); outcomes(answer.record) != "A=refused:taken AAAA=not_sent:skipped" || f.fromExchanges != 0 {
		t.Fatalf("older conflict: %+v", answer.record)
	}
}
