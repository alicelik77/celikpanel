package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"syscall"
	"time"

	"github.com/alicelik/celikpanel/internal/dnswire"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Mail identity is an external DNS fact. The hostname operation deliberately
// puts the local FQDN in /etc/hosts; NSS and net.Resolver address lookups can
// therefore neither prove nor disprove its public forward/reverse records.
func publicMailDNSIdentity(ctx context.Context, source, hostname string) (mailDNSIdentityResult, error) {
	return mailDNSIdentityObserved(ctx, source, hostname, []string{"1.1.1.1:53", "8.8.8.8:53"})
}

// mailDNSIdentityResult is what the public resolvers said about the server's
// mail identity, and who said it: Resolver is the address of the resolver
// whose answer is reported (2026-10-15), and Disagree is true when two
// resolvers gave different reverse DNS answers, as they do while a change made
// at the server provider spreads.
// Genel cozumleyicilerin posta kimligi hakkinda soyledigi ve kimin soyledigi:
// Resolver bildirilen yaniti veren cozumleyicidir; Disagree iki cozumleyici
// farkli ters DNS yaniti verdiginde dogrudur.
type mailDNSIdentityResult struct {
	PTR      string
	Aligned  bool
	Forward  bool
	Resolver string
	Disagree bool
}

func mailDNSIdentityAt(ctx context.Context, source, hostname string, resolvers []string) (string, bool, bool, error) {
	result, err := mailDNSIdentityObserved(ctx, source, hostname, resolvers)
	return result.PTR, result.Aligned, result.Forward, err
}

// mailDNSResolverName is the resolver's address without its port.
func mailDNSResolverName(resolver string) string {
	if host, _, err := net.SplitHostPort(resolver); err == nil {
		return host
	}
	return resolver
}

// errMailDNSIdentityInvalid: the address or host name cannot be looked up, so no
// lookup was made. It is not a lookup failure.
// Adres ya da ad sorgulanamaz; sorgu yapilmadi, bu bir sorgu hatasi degildir.
var errMailDNSIdentityInvalid = errors.New("mail DNS identity is invalid")

// mailDNSLookupErrorClass names why no resolver answered, without the
// resolver's own text: timeout, no resolver reachable, refused, or other.
// Hicbir cozumleyicinin yanit vermeme nedeni, cozumleyicinin kendi metni olmadan.
func mailDNSLookupErrorClass(err error) string {
	var netErr net.Error
	switch {
	case err == nil:
		return ""
	case errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()):
		return transport.MailDNSLookupErrorTimeout
	case errors.Is(err, syscall.ECONNREFUSED) || strings.HasSuffix(err.Error(), "response code 5"):
		return transport.MailDNSLookupErrorRefused
	case errors.Is(err, errNoMailDNSResolver) || errors.Is(err, syscall.ENETUNREACH) || errors.Is(err, syscall.EHOSTUNREACH):
		return transport.MailDNSLookupErrorNoResolver
	default:
		return transport.MailDNSLookupErrorOther
	}
}

var errNoMailDNSResolver = errors.New("no public mail DNS resolver available")

// A name the resolver says does not exist (NXDOMAIN) is an answer: no PTR, or
// no address for the host name. Only a missing answer is a lookup failure.
// NXDOMAIN bir yanittir; yalniz yanitin olmamasi sorgu hatasidir.
func queryMailDNSAnswer(ctx context.Context, resolver, name string, qtype uint16) ([]string, error) {
	names, err := queryMailDNS(ctx, resolver, name, qtype)
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
		return nil, nil
	}
	return names, err
}

func mailDNSIdentityObserved(ctx context.Context, source, hostname string, resolvers []string) (mailDNSIdentityResult, error) {
	ip := net.ParseIP(source)
	hostname = strings.ToLower(strings.TrimSuffix(hostname, "."))
	if ip == nil || !serviceMutationCanonicalFQDN(hostname) {
		return mailDNSIdentityResult{}, errMailDNSIdentityInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	reverse := ""
	qtype := uint16(1)
	if v4 := ip.To4(); v4 != nil {
		reverse = fmt.Sprintf("%d.%d.%d.%d.in-addr.arpa", v4[3], v4[2], v4[1], v4[0])
	} else {
		qtype = 28
		const digits = "0123456789abcdef"
		var out strings.Builder
		for i := len(ip) - 1; i >= 0; i-- {
			out.WriteByte(digits[ip[i]&15])
			out.WriteByte('.')
			out.WriteByte(digits[ip[i]>>4])
			out.WriteByte('.')
		}
		reverse = out.String() + "ip6.arpa"
	}
	// A resolver's answer that is not the expected identity does not end the
	// lookup: the next resolver is asked too. A reverse DNS name the owner has
	// just changed at the server provider takes time to reach public DNS, and
	// each resolver may keep the old answer (or the old "no such name") cached
	// for up to the record's TTL. Any resolver that returns the expected name
	// with a matching forward A/AAAA record proves the identity. When none does,
	// the first conclusive observation is reported as before, except that a PTR
	// naming the host whose forward record does not match yet is preferred:
	// it shows the provider-side change has taken effect.
	// Sahibin saglayicida yeni degistirdigi PTR yayilirken bir cozumleyici eski
	// yaniti TTL boyunca onbellekte tutabilir; beklenen kimligi gosteren herhangi
	// bir cozumleyici yeterlidir, yoksa ilk kesin gozlem bildirilir.
	var lastErr error
	observed := false
	var observedPTR string
	var observedAligned bool
	var observedBy string
	// Every resolver's conclusive reverse DNS answer, to tell whether they
	// agree. / Her cozumleyicinin kesin ters DNS yaniti, uyusup uyusmadiklari icin.
	answers := map[string]bool{}
	observe := func(ptr string, aligned bool, resolver string) {
		answers[ptr] = true
		if !observed || (aligned && !observedAligned) {
			observed, observedPTR, observedAligned, observedBy = true, ptr, aligned, mailDNSResolverName(resolver)
		}
	}
	for _, resolver := range resolvers {
		names, err := queryMailDNSAnswer(ctx, resolver, reverse, dnsTypePTR)
		if err != nil {
			lastErr = err
			continue
		}
		if len(names) == 0 {
			observe("", false, resolver)
			continue
		}
		matched := false
		for _, name := range names {
			if name != hostname {
				continue
			}
			matched = true
			addresses, err := queryMailDNSAnswer(ctx, resolver, hostname, qtype)
			if err != nil {
				lastErr = err
				break
			}
			for _, address := range addresses {
				if ip.Equal(net.ParseIP(address)) {
					answers[hostname] = true
					return mailDNSIdentityResult{
						PTR: hostname, Aligned: true, Forward: true,
						Resolver: mailDNSResolverName(resolver), Disagree: len(answers) > 1,
					}, nil
				}
			}
			observe(hostname, true, resolver)
			break
		}
		if !matched {
			observe(names[0], false, resolver)
		}
	}
	if observed {
		return mailDNSIdentityResult{
			PTR: observedPTR, Aligned: observedAligned,
			Resolver: observedBy, Disagree: len(answers) > 1,
		}, nil
	}
	if lastErr == nil {
		lastErr = errNoMailDNSResolver
	}
	return mailDNSIdentityResult{}, lastErr
}

func queryMailDNS(ctx context.Context, resolver, name string, qtype uint16) ([]string, error) {
	return dnswire.Query(ctx, resolver, name, qtype)
}

func parseMailDNSResponse(message []byte, id uint16, name string, qtype uint16) ([]string, error) {
	return dnswire.ParseResponse(message, id, name, qtype)
}
