package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/dnswire"
)

// The IPv6 reverse DNS check (alpha.112). A server with a public IPv6 address
// may send mail over IPv6, and receiving servers then require the address's
// PTR to name the mail host, exactly as for IPv4 (mail_identity). Reverse DNS
// is set at the server provider, never by CelikPanel; a lookup that got no
// answer leaves the check unknown, not an action.
// IPv6 ters DNS kontrolu; PTR saglayicida ayarlanir, panel ayarlamaz.

// setupMailIPv6 is the server's global IPv6 address; tests replace it.
var setupMailIPv6 = serverPrimaryIPv6

// setupPTRResolvers ask the fixed public resolvers; tests replace them.
var setupPTRResolvers = func() []setupProviderRecordLookup {
	lookups := []setupProviderRecordLookup{}
	for _, address := range []string{"1.1.1.1:53", "8.8.8.8:53"} {
		address := address
		lookups = append(lookups, func(ctx context.Context, name string, qtype uint16) ([]string, error) {
			return dnswire.Query(ctx, address, name, qtype)
		})
	}
	return lookups
}

func ipv6ReverseName(ip net.IP) string {
	ip = ip.To16()
	nibbles := make([]string, 0, 32)
	for index := len(ip) - 1; index >= 0; index-- {
		nibbles = append(nibbles, fmt.Sprintf("%x", ip[index]&0x0f), fmt.Sprintf("%x", ip[index]>>4))
	}
	return strings.Join(nibbles, ".") + ".ip6.arpa"
}

// setupMailIPv6Check returns the check, or false when the server has no
// public IPv6 address (nothing to check).
func setupMailIPv6Check(ctx context.Context, mailHostname string) (serverSetupCheck, bool) {
	ip := net.ParseIP(setupMailIPv6())
	if ip == nil || ip.To4() != nil || !ip.IsGlobalUnicast() || ip.IsPrivate() {
		return serverSetupCheck{}, false
	}
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	answered, seen := 0, ""
	for _, lookup := range setupPTRResolvers() {
		values, err := lookup(ctx, ipv6ReverseName(ip), dnswire.TypePTR)
		var dnsErr *net.DNSError
		if err != nil && !(errors.As(err, &dnsErr) && dnsErr.IsNotFound) {
			continue
		}
		answered++
		for _, value := range values {
			value = strings.ToLower(strings.TrimSuffix(value, "."))
			if value == mailHostname {
				return serverSetupCheck{ID: "mail_identity_ipv6", State: "ready", Code: "ready"}, true
			}
			seen = value
		}
	}
	vars := map[string]string{"ip": ip.String(), "hostname": mailHostname}
	if answered == 0 {
		return serverSetupCheck{ID: "mail_identity_ipv6", State: "unknown", Code: "mail_identity_ipv6_unavailable", Reason: "reverse_dns6_unknown", Vars: vars}, true
	}
	reason := "reverse_dns6_missing"
	if seen != "" && setupShownDNSName(seen) {
		reason, vars["ptr"] = "reverse_dns6_mismatch", seen
	}
	return serverSetupCheck{ID: "mail_identity_ipv6", State: "action_required", Code: "mail_identity_ipv6_required", Reason: reason, Vars: vars}, true
}
