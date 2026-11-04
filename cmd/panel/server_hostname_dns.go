package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/hostname"
)

// Does this server's own host name resolve in public DNS? (alpha.121,
// Frankfurt 4 Nov: frankfurt.celikhost.com, shown in the sidebar, had no A
// record in its own hosted zone and answered NXDOMAIN publicly.) The answer
// is read-only and cached: the fixed public resolvers are asked at most once
// per serverHostnameDNSTTL (a missing or unknown answer is asked again sooner,
// and "check again" at most every serverHostnameDNSRecheck), and one question
// runs at a time. Reading never changes anything. When the name is missing
// and this panel hosts its zone locally, the answer proposes the exact
// address records and names the existing DNS record path that adds them;
// the owner reviews and adds them there (the record path publishes as any
// record does). A record the zone already has is said, never added twice.
//
// Sunucunun kendi ana bilgisayar adı genel DNS'te çözülüyor mu? Salt-okur ve
// önbellekli; ad eksikse ve bölge bu panelde ise eklenecek kayıtlar önerilir,
// sahip gözden geçirip mevcut DNS kayıt yolundan ekler.

const (
	serverHostnameDNSPath    = "/api/v1/dns/server-hostname"
	serverHostnameDNSTTL     = 15 * time.Minute
	serverHostnameDNSShort   = 2 * time.Minute
	serverHostnameDNSRecheck = 30 * time.Second
	serverHostnameDNSBound   = 6 * time.Second
	serverHostnameRecordTTL  = 3600
)

const (
	serverHostnameResolves     = "resolves"
	serverHostnameMissing      = "missing"
	serverHostnameOtherAddress = "other_address"
	serverHostnameUnknown      = "unknown"
	serverHostnameNotFQDN      = "not_a_domain_name"
)

type serverHostnameRecord struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Content string `json:"content"`
	TTL     int    `json:"ttl"`
}

type serverHostnameZone struct {
	Name string `json:"name"`
	// Kind: "domain" (a website's zone) or "infrastructure" (the zone
	// server setup created).
	Kind string `json:"kind"`
	// RecordsPath is the existing record endpoint (POST) of the zone.
	RecordsPath string `json:"records_path"`
	// EditorPath is the screen that edits the zone.
	EditorPath string `json:"editor_path"`
	// Missing are the address records to add; Present the ones the zone
	// already has for the name ("A 192.0.2.10").
	Missing []serverHostnameRecord `json:"missing"`
	Present []string               `json:"present"`
}

type serverHostnameDNSAnswer struct {
	Hostname  string              `json:"hostname"`
	State     string              `json:"state"`
	Addresses []string            `json:"addresses"`
	IPv4      string              `json:"ipv4,omitempty"`
	IPv6      string              `json:"ipv6,omitempty"`
	CheckedAt string              `json:"checked_at,omitempty"`
	Zone      *serverHostnameZone `json:"zone,omitempty"`
}

var serverHostnameDNSCache = struct {
	sync.Mutex
	answer  serverHostnameDNSAnswer
	at      time.Time
	key     string
	running sync.Mutex
}{}

// serverHostnameDNSNow is a seam for tests.
var serverHostnameDNSNow = time.Now

// classifyServerHostname reads the resolvers' answers for name: resolves
// when they answer with this server's address only, other_address when an
// address is not this server's, missing when every resolver that answered
// said the name or its addresses do not exist, unknown otherwise.
func classifyServerHostname(ctx context.Context, name, ipv4, ipv6 string, resolvers []hostResolver) (string, []string) {
	type result struct {
		addresses []string
		err       error
	}
	results := make(chan result, len(resolvers))
	for _, resolver := range resolvers {
		go func(resolver hostResolver) {
			addresses, err := resolver.LookupHost(ctx, name)
			results <- result{addresses, err}
		}(resolver)
	}
	seen := map[string]bool{}
	ours, other, missing, failed := 0, 0, 0, 0
	for range resolvers {
		var answer result
		select {
		case answer = <-results:
		case <-ctx.Done():
			answer = result{err: ctx.Err()}
		}
		if answer.err != nil {
			var dnsErr *net.DNSError
			if errors.As(answer.err, &dnsErr) && dnsErr.IsNotFound {
				missing++
			} else {
				failed++
			}
			continue
		}
		mine := false
		for _, raw := range answer.addresses {
			ip := net.ParseIP(strings.TrimSpace(raw))
			if ip == nil {
				continue
			}
			if len(seen) < 8 {
				seen[ip.String()] = true
			}
			if (ipv4 != "" && ip.Equal(net.ParseIP(ipv4))) || (ipv6 != "" && ip.Equal(net.ParseIP(ipv6))) {
				mine = true
			} else {
				other++
			}
		}
		if mine {
			ours++
		}
	}
	addresses := make([]string, 0, len(seen))
	for address := range seen {
		addresses = append(addresses, address)
	}
	sort.Strings(addresses)
	switch {
	case other > 0:
		return serverHostnameOtherAddress, addresses
	case ours > 0 && failed == 0 && missing == 0:
		return serverHostnameResolves, addresses
	case missing > 0 && ours == 0 && failed == 0:
		return serverHostnameMissing, addresses
	}
	return serverHostnameUnknown, addresses
}

// serverHostnameHostedZone finds the longest zone this panel serves itself
// that holds name, with the records it has for name. nil: none.
func (p *Panel) serverHostnameHostedZone(ctx context.Context, name, ipv4, ipv6 string) (*serverHostnameZone, error) {
	labels := strings.Split(name, ".")
	for i := 0; i < len(labels)-1; i++ {
		zone := strings.Join(labels[i:], ".")
		var zoneID int64
		err := p.db.GetDB().QueryRowContext(ctx, `SELECT id FROM pdns_domains WHERE name = ?`, zone).Scan(&zoneID)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		answer := &serverHostnameZone{Name: zone, Missing: []serverHostnameRecord{}, Present: []string{}}
		infrastructure, err := p.isInfrastructureDNSZone(ctx, zone)
		if err != nil {
			return nil, err
		}
		if infrastructure {
			answer.Kind = "infrastructure"
			answer.RecordsPath = infrastructureDNSZonesPath + "/" + url.PathEscape(zone) + "/records"
			answer.EditorPath = "/domains/infrastructure/" + url.PathEscape(zone)
		} else {
			var domainID int64
			var mode string
			err := p.db.GetDB().QueryRowContext(ctx, `SELECT id, COALESCE(dns_management, '') FROM domains WHERE name = ?`, zone).Scan(&domainID, &mode)
			if errors.Is(err, sql.ErrNoRows) {
				// A zone with neither a website nor setup's ownership is
				// not one this screen offers to change.
				return nil, nil
			}
			if err != nil {
				return nil, err
			}
			if mode != "" && mode != setupDNSModeLocal {
				// The zone is served elsewhere (external or another panel).
				return nil, nil
			}
			answer.Kind = "domain"
			answer.RecordsPath = "/api/v1/domains/" + strconv.FormatInt(domainID, 10) + "/dns/records"
			answer.EditorPath = "/domains/" + url.PathEscape(zone)
		}
		rows, err := p.db.GetDB().QueryContext(ctx, `SELECT type, content FROM pdns_records WHERE domain_id = ? AND name = ? AND type IN ('A','AAAA','CNAME') LIMIT 16`, zoneID, name)
		if err != nil {
			return nil, err
		}
		has := map[string]bool{}
		for rows.Next() {
			var recordType, content string
			if err := rows.Scan(&recordType, &content); err != nil {
				rows.Close()
				return nil, err
			}
			has[recordType] = true
			answer.Present = append(answer.Present, recordType+" "+content)
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
		if !has["CNAME"] {
			if ipv4 != "" && !has["A"] {
				answer.Missing = append(answer.Missing, serverHostnameRecord{Name: name, Type: "A", Content: ipv4, TTL: serverHostnameRecordTTL})
			}
			if ipv6 != "" && !has["AAAA"] {
				answer.Missing = append(answer.Missing, serverHostnameRecord{Name: name, Type: "AAAA", Content: ipv6, TTL: serverHostnameRecordTTL})
			}
		}
		return answer, nil
	}
	return nil, nil
}

// serverHostnameDNS answers from the cache or asks once.
func (p *Panel) serverHostnameDNS(ctx context.Context, recheck bool) serverHostnameDNSAnswer {
	name, err := localServerName()
	canonical, canonicalErr := hostname.CanonicalFQDN(strings.TrimSuffix(strings.TrimSpace(name), "."))
	ipv4, _ := canonicalIPv4(serverPrimaryIP())
	ipv6 := ""
	if ip := net.ParseIP(serverPrimaryIPv6()); ip != nil && ip.To4() == nil {
		ipv6 = ip.String()
	}
	answer := serverHostnameDNSAnswer{Hostname: canonical, Addresses: []string{}, IPv4: ipv4, IPv6: ipv6}
	if err != nil || canonicalErr != nil || !strings.Contains(canonical, ".") {
		answer.Hostname = strings.TrimSpace(name)
		answer.State = serverHostnameNotFQDN
		return answer
	}
	key := canonical + "|" + ipv4 + "|" + ipv6
	cache := &serverHostnameDNSCache
	fresh := func(now time.Time) (serverHostnameDNSAnswer, bool) {
		cache.Lock()
		defer cache.Unlock()
		if cache.key != key || cache.at.IsZero() {
			return serverHostnameDNSAnswer{}, false
		}
		age := now.Sub(cache.at)
		ttl := serverHostnameDNSTTL
		if cache.answer.State != serverHostnameResolves {
			ttl = serverHostnameDNSShort
		}
		if recheck {
			ttl = serverHostnameDNSRecheck
		}
		return cache.answer, age < ttl
	}
	if cached, ok := fresh(serverHostnameDNSNow()); ok {
		return p.withServerHostnameZone(ctx, cached)
	}
	// One question at a time; a second reader takes its answer.
	cache.running.Lock()
	defer cache.running.Unlock()
	if cached, ok := fresh(serverHostnameDNSNow()); ok {
		return p.withServerHostnameZone(ctx, cached)
	}
	checkCtx, cancel := context.WithTimeout(ctx, serverHostnameDNSBound)
	answer.State, answer.Addresses = classifyServerHostname(checkCtx, canonical, ipv4, ipv6, setupDNSPublicResolvers())
	cancel()
	now := serverHostnameDNSNow()
	answer.CheckedAt = now.UTC().Format(time.RFC3339)
	cache.Lock()
	cache.answer, cache.at, cache.key = answer, now, key
	cache.Unlock()
	return p.withServerHostnameZone(ctx, answer)
}

// withServerHostnameZone adds, for a missing name, the zone this panel
// hosts for it (read from the panel's own database every time, so a record
// just added is seen at once).
func (p *Panel) withServerHostnameZone(ctx context.Context, answer serverHostnameDNSAnswer) serverHostnameDNSAnswer {
	answer.Zone = nil
	if answer.State != serverHostnameMissing {
		return answer
	}
	zone, err := p.serverHostnameHostedZone(ctx, answer.Hostname, answer.IPv4, answer.IPv6)
	if err == nil && zone != nil {
		answer.Zone = zone
	}
	return answer
}

func (p *Panel) handleServerHostnameDNS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		rejectRouteMethod(w, []string{http.MethodGet})
		return
	}
	if caller := currentCaller(r); caller == nil || !caller.hasAccountRole(roleAdmin) {
		writeCodedError(w, http.StatusForbidden, "ADMIN_ONLY", "administrator access is required", "")
		return
	}
	_ = json.NewEncoder(w).Encode(p.serverHostnameDNS(r.Context(), r.URL.Query().Get("recheck") == "1"))
}
