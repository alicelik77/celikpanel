package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// Does every name a certificate request would carry point to this server?
//
// The owner's rule (2 Nov 2026): "If the domain is not on the server, the
// certificate cannot be obtained either." A domain was created here while its
// nameservers at the registrar still pointed to another provider; the SSL/TLS
// screen offered "Get certificate" and the press could only fail at the
// certificate authority. The Domain Overview already reads the live DNS
// (domain_connection.go); this file asks the same public resolvers about each
// name of the request, so the SSL/TLS screen and the request endpoint decide
// from one answer.
//
// Three answers per name, never two: the name resolves here ("here"), the
// resolvers answered and none of them showed this server ("elsewhere"), or no
// resolver answered at all ("unknown"). Unknown never refuses: a failed lookup
// is not evidence that the domain is elsewhere (D-024).
//
// Bir sertifika isteğinin taşıyacağı her ad bu sunucuyu gösteriyor mu? Sahibin
// kuralı: "Alan adı sunucuda değilse sertifika da alınamaz." Üç yanıt vardır:
// burada ("here"), çözümleyiciler yanıtladı ama hiçbiri bu sunucuyu
// göstermedi ("elsewhere") ya da hiçbir çözümleyici yanıtlamadı ("unknown").
// Bilinmeyen asla reddettirmez.

const (
	certificateNameHere      = "here"
	certificateNameElsewhere = "elsewhere"
	certificateNameUnknown   = "unknown"

	certificateNameKindSite = "site"
	certificateNameKindMail = "mail"

	// errCodeCertificateDomainNotPointed: a certificate request for names the
	// public DNS shows somewhere else. Nothing was requested.
	errCodeCertificateDomainNotPointed = "CERTIFICATE_DOMAIN_NOT_POINTED"
	// The refusal's refinement when only mail.<domain> is the problem.
	certificateNotPointedReasonMail = "mail_name"
)

// certificateNameResolvers and certificateServerAddresses are the two inputs of
// the check. Tests replace them; nothing else does.
// İki girdi yalnız testlerde değiştirilir.
var (
	certificateNameResolvers   = func() []namedConnectionResolver { return publicConnectionResolvers }
	certificateServerAddresses = func() []string {
		return normalizeIPs([]string{serverPrimaryIP(), serverPrimaryIPv6()})
	}
)

type certificateNameCheck struct {
	Name  string   `json:"name"`
	Kind  string   `json:"kind"`
	State string   `json:"state"`
	IPs   []string `json:"ips"`
}

// checkCertificateNames asks the public resolvers, in parallel and bounded,
// where each name resolves. The order of names is kept.
func checkCertificateNames(ctx context.Context, siteNames []string, mailName string) []certificateNameCheck {
	type target struct{ name, kind string }
	targets := make([]target, 0, len(siteNames)+1)
	for _, name := range siteNames {
		targets = append(targets, target{name, certificateNameKindSite})
	}
	if mailName != "" {
		targets = append(targets, target{mailName, certificateNameKindMail})
	}
	server := certificateServerAddresses()
	resolvers := certificateNameResolvers()
	out := make([]certificateNameCheck, len(targets))
	var wg sync.WaitGroup
	for i, tgt := range targets {
		wg.Add(1)
		go func(i int, tgt target) {
			defer wg.Done()
			state, ips := certificateNameState(ctx, tgt.name, server, resolvers)
			out[i] = certificateNameCheck{Name: tgt.name, Kind: tgt.kind, State: state, IPs: ips}
		}(i, tgt)
	}
	wg.Wait()
	return out
}

func certificateNameState(ctx context.Context, name string, server []string, resolvers []namedConnectionResolver) (string, []string) {
	if len(server) == 0 || len(resolvers) == 0 {
		return certificateNameUnknown, []string{}
	}
	type answer struct {
		answered bool
		ips      []string
	}
	answers := make([]answer, len(resolvers))
	var wg sync.WaitGroup
	for i, source := range resolvers {
		wg.Add(1)
		go func(i int, source namedConnectionResolver) {
			defer wg.Done()
			queryCtx, cancel := context.WithTimeout(ctx, 2500*time.Millisecond)
			defer cancel()
			addresses, err := source.Resolver.LookupHost(queryCtx, name)
			var dnsErr *net.DNSError
			switch {
			case err == nil:
				answers[i] = answer{answered: true, ips: normalizeIPs(addresses)}
			case errors.As(err, &dnsErr) && dnsErr.IsNotFound:
				// The resolver said the name has no address: an answer.
				answers[i] = answer{answered: true}
			}
		}(i, source)
	}
	wg.Wait()

	here := make(map[string]bool, len(server))
	for _, ip := range server {
		here[ip] = true
	}
	seen := map[string]bool{}
	answered, pointsHere := false, false
	for _, a := range answers {
		if !a.answered {
			continue
		}
		answered = true
		for _, ip := range a.ips {
			seen[ip] = true
			if here[ip] {
				pointsHere = true
			}
		}
	}
	ips := make([]string, 0, len(seen))
	for ip := range seen {
		ips = append(ips, ip)
	}
	sort.Strings(ips)
	switch {
	case pointsHere:
		return certificateNameHere, ips
	case answered:
		return certificateNameElsewhere, ips
	default:
		return certificateNameUnknown, ips
	}
}

// certificateNamesNotPointed returns the names the check knows do not point
// here, and whether the mail name is the only one.
func certificateNamesNotPointed(checks []certificateNameCheck) (names []string, mailOnly bool) {
	mail, site := 0, 0
	for _, c := range checks {
		if c.State != certificateNameElsewhere {
			continue
		}
		names = append(names, c.Name)
		if c.Kind == certificateNameKindMail {
			mail++
		} else {
			site++
		}
	}
	return names, mail > 0 && site == 0
}

// certificateNameservers are the nameservers this server would be delegated to
// for the domain, only where the panel serves the zone; empty otherwise.
func (p *Panel) certificateNameservers(ctx context.Context, domainName string) []string {
	mode, err := p.domainDNSManagementMode(ctx, domainName)
	if err != nil || mode != setupDNSModeLocal {
		return nil
	}
	if ns1, ns2 := p.serverNameservers(ctx); ns1 != "" && ns2 != "" {
		return []string{ns1, ns2}
	}
	return nil
}

// refuseCertificateDomainNotPointed answers 409 CERTIFICATE_DOMAIN_NOT_POINTED
// when the check knows a requested name does not point here, and reports true.
// An unknown answer never refuses. Nothing has been requested or changed when
// this answers (D-024: the reason, who acts, the next action).
func (p *Panel) refuseCertificateDomainNotPointed(
	ctx context.Context, w http.ResponseWriter, domainName string, siteNames []string, mailName string,
) bool {
	checkCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	checks := checkCertificateNames(checkCtx, siteNames, mailName)
	cancel()
	names, mailOnly := certificateNamesNotPointed(checks)
	if len(names) == 0 {
		return false
	}
	server := certificateServerAddresses()
	ip := ""
	if len(server) > 0 {
		ip = server[0]
	}
	nameservers := []string(nil)
	if !mailOnly {
		nameservers = p.certificateNameservers(ctx, domainName)
	}
	joined := strings.Join(names, ", ")
	message := joined + " does not point to this server yet, so no certificate was requested and nothing was changed. "
	switch {
	case mailOnly:
		message += "Point the A record of " + joined + " to " + ip + ", or request the certificate without the mail name. "
	case len(nameservers) == 2:
		message += "At the company where you bought the domain, set the nameservers to " +
			strings.Join(nameservers, " and ") + " (or point the A record to " + ip + "). "
	default:
		message += "At the company that runs the domain's DNS, point the A record of " + joined + " to " + ip + ". "
	}
	message += "Request the certificate again once the change has spread; this usually takes a few hours."
	vars := map[string]string{"names": joined, "ip": ip}
	if len(nameservers) == 2 {
		vars["nameservers"] = strings.Join(nameservers, ", ")
		vars["ns1"], vars["ns2"] = nameservers[0], nameservers[1]
	}
	body := apiErrorBody{Error: message, Code: errCodeCertificateDomainNotPointed, Vars: vars}
	if mailOnly {
		body.Reason = certificateNotPointedReasonMail
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusConflict)
	_ = json.NewEncoder(w).Encode(body)
	return true
}
