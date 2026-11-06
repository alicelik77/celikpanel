package main

import (
	"context"
	"sync"
)

// dnsMemberSerialWorkers bounds how many SOA questions of one pair proof run
// at the same time (alpha.127). The proofs used to ask every catalog member
// of both servers one after another: about 0.3 s per member between Boston
// and Frankfurt, so the readiness check took ~6 s and every screen that
// needed it waited. The questions, their per-query limit (dnsProbeTimeout)
// and the verdict are unchanged; only their order in time is.
//
// Bir es kanitinin ayni anda sorulan SOA sorulari; sorular, sinirlari ve
// karar ayni, yalniz zamanlamasi degisti.
const dnsMemberSerialWorkers = 8

type dnsMemberSerialAnswer struct {
	serial uint32
	err    error
}

// probeDNSMemberSerials asks every address for the SOA serial of every
// member. answers[i][j] is members[i] at addresses[j], as
// exactDNSZoneSerialAtWithProbe would answer it alone. The caller decides in
// the same order as before.
func probeDNSMemberSerials(
	ctx context.Context,
	addresses, members []string,
	soa dnsZoneSOAProbe,
) [][]dnsMemberSerialAnswer {
	answers := make([][]dnsMemberSerialAnswer, len(members))
	for i := range answers {
		answers[i] = make([]dnsMemberSerialAnswer, len(addresses))
	}
	type job struct{ member, address int }
	jobs := make(chan job)
	var wg sync.WaitGroup
	workers := min(dnsMemberSerialWorkers, len(members)*len(addresses))
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				serial, err := exactDNSZoneSerialAtWithProbe(ctx, addresses[j.address], members[j.member], soa)
				answers[j.member][j.address] = dnsMemberSerialAnswer{serial: serial, err: err}
			}
		}()
	}
	for i := range members {
		for j := range addresses {
			jobs <- job{member: i, address: j}
		}
	}
	close(jobs)
	wg.Wait()
	return answers
}
