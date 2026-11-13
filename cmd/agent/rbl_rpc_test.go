package main

import (
	"context"
	"errors"
	"net"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.133: a blocklist's answer is a listing only when the list documents
// that code as one. The owner's server was shown as "on a spam blocklist"
// because Spamhaus refused a question that came through 1.1.1.1
// (127.255.255.254). No test here touches the network.

func rblZoneNamed(t *testing.T, name string) rblZone {
	t.Helper()
	for _, zone := range rblZones {
		if zone.zone == name {
			return zone
		}
	}
	t.Fatalf("zone %s is not asked", name)
	return rblZone{}
}

func ips(codes ...string) []net.IP {
	out := make([]net.IP, 0, len(codes))
	for _, code := range codes {
		out = append(out, net.ParseIP(code))
	}
	return out
}

func TestRBLInterpreterReadsOnlyDocumentedCodesAsListings(t *testing.T) {
	notFound := &net.DNSError{Err: "no such host", Name: "x", IsNotFound: true}
	timeout := &net.DNSError{Err: "i/o timeout", Name: "x", IsTimeout: true}
	cases := []struct {
		name     string
		zone     string
		answer   []net.IP
		err      error
		outcome  string
		reason   string
		sublists []string
	}{
		{"zen SBL", "zen.spamhaus.org", ips("127.0.0.2"), nil, transport.RBLListed, "", []string{"sbl"}},
		{"zen CSS", "zen.spamhaus.org", ips("127.0.0.3"), nil, transport.RBLListed, "", []string{"css"}},
		{"zen XBL", "zen.spamhaus.org", ips("127.0.0.4"), nil, transport.RBLListed, "", []string{"xbl"}},
		{"zen DROP is SBL data", "zen.spamhaus.org", ips("127.0.0.9"), nil, transport.RBLListed, "", []string{"sbl"}},
		{"zen PBL operator", "zen.spamhaus.org", ips("127.0.0.10"), nil, transport.RBLListed, "", []string{"pbl"}},
		{"zen PBL Spamhaus", "zen.spamhaus.org", ips("127.0.0.11"), nil, transport.RBLListed, "", []string{"pbl"}},
		{"zen several parts, stable order", "zen.spamhaus.org", ips("127.0.0.11", "127.0.0.4", "127.0.0.2", "127.0.0.10"), nil,
			transport.RBLListed, "", []string{"sbl", "xbl", "pbl"}},
		{"zen listing beside an unknown code", "zen.spamhaus.org", ips("127.0.0.6", "127.0.0.3"), nil, transport.RBLListed, "", []string{"css"}},
		{"zen undocumented 127.0.0.5", "zen.spamhaus.org", ips("127.0.0.5"), nil, transport.RBLNotChecked, transport.RBLReasonUnknownAnswer, nil},
		{"zen undocumented 127.0.0.8", "zen.spamhaus.org", ips("127.0.0.8"), nil, transport.RBLNotChecked, transport.RBLReasonUnknownAnswer, nil},
		{"zen public resolver refused", "zen.spamhaus.org", ips("127.255.255.254"), nil, transport.RBLNotChecked, transport.RBLReasonPublicResolver, nil},
		{"zen too many queries", "zen.spamhaus.org", ips("127.255.255.255"), nil, transport.RBLNotChecked, transport.RBLReasonTooManyQueries, nil},
		{"zen typing error", "zen.spamhaus.org", ips("127.255.255.252"), nil, transport.RBLNotChecked, transport.RBLReasonUnknownAnswer, nil},
		{"zen other refusal code", "zen.spamhaus.org", ips("127.255.255.253"), nil, transport.RBLNotChecked, transport.RBLReasonUnknownAnswer, nil},
		{"zen strongest refusal wins", "zen.spamhaus.org", ips("127.255.255.255", "127.255.255.254"), nil, transport.RBLNotChecked, transport.RBLReasonPublicResolver, nil},
		{"zen hijacked answer", "zen.spamhaus.org", ips("192.0.2.10"), nil, transport.RBLNotChecked, transport.RBLReasonUnknownAnswer, nil},
		{"zen NXDOMAIN", "zen.spamhaus.org", nil, notFound, transport.RBLNotListed, "", nil},
		{"zen timeout", "zen.spamhaus.org", nil, timeout, transport.RBLNotChecked, transport.RBLReasonLookupFailed, nil},
		{"zen deadline", "zen.spamhaus.org", nil, context.DeadlineExceeded, transport.RBLNotChecked, transport.RBLReasonLookupFailed, nil},
		{"zen server failure", "zen.spamhaus.org", nil, &net.DNSError{Err: "server misbehaving", Name: "x"}, transport.RBLNotChecked, transport.RBLReasonLookupFailed, nil},
		{"SpamCop listed", "bl.spamcop.net", ips("127.0.0.2"), nil, transport.RBLListed, "", nil},
		{"SpamCop other code", "bl.spamcop.net", ips("127.0.0.3"), nil, transport.RBLNotChecked, transport.RBLReasonUnknownAnswer, nil},
		{"SpamCop undocumented refusal range", "bl.spamcop.net", ips("127.255.255.254"), nil, transport.RBLNotChecked, transport.RBLReasonUnknownAnswer, nil},
		{"SpamCop NXDOMAIN", "bl.spamcop.net", nil, notFound, transport.RBLNotListed, "", nil},
		{"SpamCop timeout", "bl.spamcop.net", nil, timeout, transport.RBLNotChecked, transport.RBLReasonLookupFailed, nil},
		{"Barracuda listed", "b.barracudacentral.org", ips("127.0.0.2"), nil, transport.RBLListed, "", nil},
		{"Barracuda other code", "b.barracudacentral.org", ips("127.0.0.10"), nil, transport.RBLNotChecked, transport.RBLReasonUnknownAnswer, nil},
		{"Barracuda NXDOMAIN", "b.barracudacentral.org", nil, notFound, transport.RBLNotListed, "", nil},
		{"Barracuda timeout", "b.barracudacentral.org", nil, timeout, transport.RBLNotChecked, transport.RBLReasonLookupFailed, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := interpretRBLAnswer(rblZoneNamed(t, c.zone), c.answer, c.err)
			if got.Zone != c.zone || got.Outcome != c.outcome || got.Reason != c.reason || !reflect.DeepEqual(got.Sublists, c.sublists) {
				t.Fatalf("got %+v, want outcome %q reason %q sublists %v", got, c.outcome, c.reason, c.sublists)
			}
			// The old field stays truthful for an older Panel: true only for
			// a documented listing, never for a refusal or a failed lookup.
			if got.Listed != (c.outcome == transport.RBLListed) {
				t.Fatalf("Listed = %v with outcome %q", got.Listed, got.Outcome)
			}
		})
	}
}

func TestRBLDeadListIsNotAsked(t *testing.T) {
	var zones []string
	for _, zone := range rblZones {
		zones = append(zones, zone.zone)
		if len(zone.listing) == 0 {
			t.Fatalf("%s has no documented listing code", zone.zone)
		}
	}
	want := []string{"zen.spamhaus.org", "bl.spamcop.net", "b.barracudacentral.org"}
	if !reflect.DeepEqual(zones, want) {
		t.Fatalf("zones = %v, want %v (dnsbl.sorbs.net closed in 2024)", zones, want)
	}
}

type fakeRBLResolver struct {
	mu      sync.Mutex
	answers map[string][]net.IP
	errs    map[string]error
	txt     map[string][]string
	asked   []string
}

func (f *fakeRBLResolver) LookupIP(_ context.Context, network, host string) ([]net.IP, error) {
	f.mu.Lock()
	f.asked = append(f.asked, network+" "+host)
	f.mu.Unlock()
	if err, ok := f.errs[host]; ok {
		return nil, err
	}
	if answer, ok := f.answers[host]; ok {
		return answer, nil
	}
	return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
}

func (f *fakeRBLResolver) LookupTXT(_ context.Context, name string) ([]string, error) {
	if txt, ok := f.txt[name]; ok {
		return txt, nil
	}
	return nil, errors.New("no txt")
}

// The owner's case: the server asks through a public resolver. Spamhaus
// refuses, the others answer "not listed"; nothing is listed.
func TestCheckRBLPublicResolverRefusalIsNeverListed(t *testing.T) {
	resolver := &fakeRBLResolver{
		answers: map[string][]net.IP{"7.113.0.203.zen.spamhaus.org": ips("127.255.255.254")},
		txt:     map[string][]string{"7.113.0.203.zen.spamhaus.org": {"open resolver"}},
	}
	resp := checkRBL(context.Background(), resolver, "203.0.113.7")
	if resp.IP != "203.0.113.7" || resp.Error != "" || len(resp.Results) != 3 {
		t.Fatalf("resp = %+v", resp)
	}
	for _, res := range resp.Results {
		if res.Listed || res.Detail != "" {
			t.Fatalf("%s shown as listed: %+v", res.Zone, res)
		}
	}
	if got := resp.Results[0]; got.Outcome != transport.RBLNotChecked || got.Reason != transport.RBLReasonPublicResolver {
		t.Fatalf("spamhaus = %+v", got)
	}
	for _, res := range resp.Results[1:] {
		if res.Outcome != transport.RBLNotListed {
			t.Fatalf("%s = %+v", res.Zone, res)
		}
	}
	for _, asked := range resolver.asked {
		if !strings.HasPrefix(asked, "ip4 ") {
			t.Fatalf("asked %q, want IPv4 address records only", asked)
		}
	}
}

func TestCheckRBLListingCarriesItsReason(t *testing.T) {
	resolver := &fakeRBLResolver{
		answers: map[string][]net.IP{"7.113.0.203.bl.spamcop.net": ips("127.0.0.2")},
		txt:     map[string][]string{"7.113.0.203.bl.spamcop.net": {" Blocked - see https://www.spamcop.net/bl.shtml?203.0.113.7 "}},
		errs:    map[string]error{"7.113.0.203.b.barracudacentral.org": context.DeadlineExceeded},
	}
	resp := checkRBL(context.Background(), resolver, "203.0.113.7")
	spamcop := resp.Results[1]
	if !spamcop.Listed || spamcop.Outcome != transport.RBLListed || !strings.HasPrefix(spamcop.Detail, "Blocked") {
		t.Fatalf("spamcop = %+v", spamcop)
	}
	if barracuda := resp.Results[2]; barracuda.Listed || barracuda.Reason != transport.RBLReasonLookupFailed {
		t.Fatalf("barracuda = %+v", barracuda)
	}
}

func TestCheckRBLRefusesANonIPv4Address(t *testing.T) {
	resp := checkRBL(context.Background(), &fakeRBLResolver{}, "2001:db8::1")
	if resp.Error == "" || len(resp.Results) != 0 {
		t.Fatalf("resp = %+v", resp)
	}
}
