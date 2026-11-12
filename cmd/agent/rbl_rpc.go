package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// RBL (DNS blocklist) check for the server's outbound IPv4 address. A mail
// server on a blocklist has its mail silently rejected everywhere, and the
// operator usually finds out only from bounced messages — this surfaces it
// on demand. We query the well-known zones by the standard reversed-octet
// convention.
//
// An address answer is NOT by itself a listing (alpha.133, owner's server
// 12 Nov 2026): the lists also answer with addresses that mean "your
// question was refused" — Spamhaus answers 127.255.255.254 when the question
// came through a public resolver such as 1.1.1.1 or 8.8.8.8. Before alpha.133
// that refusal was shown as "this server is on a blocklist". Each list's
// answers are now read against the codes it documents: a documented listing
// code is "listed", NXDOMAIN is "not listed", and everything else (a refusal,
// an undocumented answer, a DNS error, a timeout) is "could not check" with
// its reason. Only codes we are sure of are listed below; an answer outside
// them is never guessed into a listing.
//
// Sunucunun dışa dönük IPv4 adresi için RBL (DNS kara-liste) kontrolü. Bir
// adres yanıtı tek başına listeleme değildir: listeler "sorunuz reddedildi"
// anlamına gelen adreslerle de yanıt verir (genel çözümleyiciden gelen soruya
// Spamhaus 127.255.255.254 döner). Yanıtlar her listenin belgelediği kodlarla
// okunur: belgelenmiş listeleme kodu "listede", NXDOMAIN "listede değil",
// geri kalan her şey nedeniyle birlikte "denetlenemedi"dir.

// rblZone is one list and how its answers are read.
type rblZone struct {
	zone string
	// listing maps each A answer the list documents as a listing to the
	// part of the list it comes from ("" for a list without parts).
	listing map[string]string
	// refusals maps each answer the list documents as "your question was
	// refused" to its reason. Another answer is never a listing either.
	refusals map[string]string
}

// The lists asked. dnsbl.sorbs.net was shut down in 2024 and is no longer
// asked: a dead list can only produce noise.
//
// Spamhaus ZEN (documented return codes): 127.0.0.2 SBL, 127.0.0.3 SBL CSS,
// 127.0.0.4 XBL, 127.0.0.9 SBL DROP/EDROP data, 127.0.0.10 PBL (maintained
// by the address's network operator), 127.0.0.11 PBL (maintained by
// Spamhaus). 127.0.0.5–127.0.0.8 are not read as listings: they are not in
// the current published table, so such an answer is "could not check".
// SpamCop and Barracuda answer 127.0.0.2 for a listed address.
var rblZones = []rblZone{
	{zone: "zen.spamhaus.org", listing: map[string]string{
		"127.0.0.2":  "sbl",
		"127.0.0.3":  "css",
		"127.0.0.4":  "xbl",
		"127.0.0.9":  "sbl",
		"127.0.0.10": "pbl",
		"127.0.0.11": "pbl",
	}, refusals: spamhausRefusals},
	{zone: "bl.spamcop.net", listing: map[string]string{"127.0.0.2": ""}},
	{zone: "b.barracudacentral.org", listing: map[string]string{"127.0.0.2": ""}},
}

// rblSublistOrder keeps a listing's parts in one stable order.
var rblSublistOrder = []string{"sbl", "css", "xbl", "pbl"}

// spamhausRefusals are Spamhaus's answers that mean the question was
// refused, not that the address is listed (127.255.255.252 is "typing error
// in the list's name").
var spamhausRefusals = map[string]string{
	"127.255.255.254": transport.RBLReasonPublicResolver,
	"127.255.255.255": transport.RBLReasonTooManyQueries,
	"127.255.255.252": transport.RBLReasonUnknownAnswer,
}

// rblQueryTimeout bounds one list's lookup.
const rblQueryTimeout = 5 * time.Second

// rblResolver is the part of net.Resolver the check uses (tests use a fake).
type rblResolver interface {
	LookupIP(ctx context.Context, network, host string) ([]net.IP, error)
	LookupTXT(ctx context.Context, name string) ([]string, error)
}

type RBLResult = transport.RBLResult

type CheckRBLResponse = transport.CheckRBLResponse

func (a *Agent) CheckRBL(_ *transport.Empty, resp *CheckRBLResponse) error {
	ip := outboundIPv4()
	if ip == "" {
		resp.Error = "could not determine the server's outbound IPv4 address"
		return nil
	}
	*resp = checkRBL(context.Background(), &net.Resolver{}, ip)
	return nil
}

// checkRBL asks every list about ip at the same time, each with a bounded
// timeout — a slow blocklist must not hang the whole check.
// checkRBL tüm listelere aynı anda, her biri sınırlı zaman aşımıyla sorar.
func checkRBL(ctx context.Context, resolver rblResolver, ip string) CheckRBLResponse {
	resp := CheckRBLResponse{IP: ip}
	rev, err := reverseIPv4(ip)
	if err != nil {
		resp.Error = err.Error()
		return resp
	}
	results := make([]RBLResult, len(rblZones))
	var wg sync.WaitGroup
	for i, zone := range rblZones {
		wg.Add(1)
		go func(i int, zone rblZone) {
			defer wg.Done()
			results[i] = queryRBL(ctx, resolver, rev, zone)
		}(i, zone)
	}
	wg.Wait()
	resp.Results = results
	return resp
}

// queryRBL looks up <reversed-ip>.<zone> and reads the answer; a listing
// usually carries a TXT reason we surface as the detail.
// queryRBL, <ters-ip>.<zone>'u arar ve yanıtı okur.
func queryRBL(parent context.Context, resolver rblResolver, reversedIP string, zone rblZone) RBLResult {
	ctx, cancel := context.WithTimeout(parent, rblQueryTimeout)
	defer cancel()
	host := reversedIP + "." + zone.zone
	addrs, err := resolver.LookupIP(ctx, "ip4", host)
	res := interpretRBLAnswer(zone, addrs, err)
	if res.Listed {
		if txts, err := resolver.LookupTXT(ctx, host); err == nil && len(txts) > 0 {
			res.Detail = strings.TrimSpace(txts[0])
		}
	}
	return res
}

// interpretRBLAnswer turns one list's DNS answer into listed / not listed /
// could not check. Listed is true only together with RBLListed, so an older
// Panel (which reads Listed alone) never shows a refusal as a listing.
func interpretRBLAnswer(zone rblZone, addrs []net.IP, err error) RBLResult {
	res := RBLResult{Zone: zone.zone}
	if err != nil {
		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
			// NXDOMAIN (or no address record) is the normal "not listed".
			res.Outcome = transport.RBLNotListed
			return res
		}
		res.Outcome, res.Reason = transport.RBLNotChecked, transport.RBLReasonLookupFailed
		return res
	}
	if len(addrs) == 0 {
		res.Outcome = transport.RBLNotListed
		return res
	}
	parts := map[string]bool{}
	listed := false
	refusal := ""
	for _, addr := range addrs {
		v4 := addr.To4()
		if v4 == nil {
			continue
		}
		code := v4.String()
		if part, ok := zone.listing[code]; ok {
			listed = true
			if part != "" {
				parts[part] = true
			}
			continue
		}
		if reason, ok := zone.refusals[code]; ok {
			refusal = strongerRBLReason(refusal, reason)
		}
	}
	switch {
	case listed:
		res.Outcome, res.Listed = transport.RBLListed, true
		for _, part := range rblSublistOrder {
			if parts[part] {
				res.Sublists = append(res.Sublists, part)
			}
		}
	case refusal != "":
		res.Outcome, res.Reason = transport.RBLNotChecked, refusal
	default:
		res.Outcome, res.Reason = transport.RBLNotChecked, transport.RBLReasonUnknownAnswer
	}
	return res
}

// strongerRBLReason keeps the reason that tells the administrator most.
func strongerRBLReason(current, next string) string {
	rank := map[string]int{
		transport.RBLReasonPublicResolver: 3,
		transport.RBLReasonTooManyQueries: 2,
		transport.RBLReasonUnknownAnswer:  1,
	}
	if rank[next] > rank[current] {
		return next
	}
	return current
}

// outboundIPv4 mirrors the panel's own resolution: an explicit override
// first (NAT), else the default-route source address. The UDP dial sends
// nothing; it only resolves routing.
// outboundIPv4, panelin kendi çözümünü yansıtır: önce açık geçersiz kılma
// (NAT), yoksa varsayılan rotanın kaynak adresi.
func outboundIPv4() string {
	if ip := os.Getenv("CELIKPANEL_SERVER_IP"); ip != "" {
		return ip
	}
	conn, err := net.Dial("udp4", "192.0.2.1:53")
	if err != nil {
		return ""
	}
	defer conn.Close()
	if addr, ok := conn.LocalAddr().(*net.UDPAddr); ok {
		return addr.IP.String()
	}
	return ""
}

// reverseIPv4 turns 203.0.113.7 into 7.113.0.203, the RBL query convention.
// reverseIPv4, 203.0.113.7'yi RBL sorgu kuralı olan 7.113.0.203'e çevirir.
func reverseIPv4(ip string) (string, error) {
	parsed := net.ParseIP(ip).To4()
	if parsed == nil {
		return "", fmt.Errorf("%q is not an IPv4 address", ip)
	}
	return fmt.Sprintf("%d.%d.%d.%d", parsed[3], parsed[2], parsed[1], parsed[0]), nil
}
