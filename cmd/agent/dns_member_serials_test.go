package main

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

// The member SOA questions of a pair proof run at the same time, at most
// dnsMemberSerialWorkers at once, and every answer lands where the
// one-by-one loop put it.
func TestProbeDNSMemberSerialsKeepsEveryAnswerInPlace(t *testing.T) {
	members := []string{}
	for i := 0; i < 12; i++ {
		members = append(members, fmt.Sprintf("zone%02d.example", i))
	}
	var inFlight, peak atomic.Int32
	soa := func(_ context.Context, _ string, address, domain string) (dnsSOAProbeResult, error) {
		now := inFlight.Add(1)
		defer inFlight.Add(-1)
		for {
			old := peak.Load()
			if now <= old || peak.CompareAndSwap(old, now) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		if address == "192.0.2.2" && domain == "zone05.example" {
			return dnsSOAProbeResult{}, errors.New("timeout")
		}
		serial := uint32(len(domain))
		if address == "192.0.2.2" {
			serial += 1000
		}
		return dnsSOAProbeResult{Authoritative: true, SOASerials: []uint32{serial}}, nil
	}
	start := time.Now()
	answers := probeDNSMemberSerials(context.Background(), []string{"192.0.2.1", "192.0.2.2"}, members, soa)
	elapsed := time.Since(start)
	for i, member := range members {
		for j, address := range []string{"192.0.2.1", "192.0.2.2"} {
			want, wantErr := exactDNSZoneSerialAtWithProbe(context.Background(), address, member, soa)
			got := answers[i][j]
			if got.serial != want || (got.err == nil) != (wantErr == nil) {
				t.Fatalf("%s at %s = %d %v, want %d %v", member, address, got.serial, got.err, want, wantErr)
			}
		}
	}
	if peak.Load() > dnsMemberSerialWorkers {
		t.Fatalf("questions at once = %d, want at most %d", peak.Load(), dnsMemberSerialWorkers)
	}
	// 24 members×addresses, UDP+TCP each (~40 ms) one by one would be ~1 s.
	if elapsed > 500*time.Millisecond {
		t.Fatalf("questions took %s; they did not run at the same time", elapsed)
	}
}
