package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/hostname"
)

// The upstream of a "proxy" site (2026-11-02, security fix).
//
// nginx fetches the upstream on the server itself, for any visitor. An
// upstream on this server or its private network would publish services
// that are meant to stay internal (the panel, the agent, databases, cloud
// metadata at 169.254.169.254) under a customer's domain. So a proxy site may
// only forward to a public address of another machine. This holds for every
// caller, administrators included: an application on this server is a
// Node.js site, and the owner can still write such an nginx file by hand
// (D-022, the file is then kept as the owner's).
//
// "forwarding" sites are not checked here: they answer the visitor with an
// HTTP redirect and the server never fetches the address.
//
// Remaining limit (DNS rebinding): a host name is resolved and checked when
// the site is saved, but nginx resolves it again later (at reload, or per
// request through `resolver`). A name whose records are changed after the
// save to an internal address is not caught. Writing the checked address
// into the vhost would close this, but would also break upstreams whose
// addresses change; it is not done.
//
// Sitenin vekil adresi yalnız başka bir makinenin kamusal adresi olabilir;
// yerel, özel ağ ve bu sunucunun kendi adresleri herkes için reddedilir.
// Ad kayıtta çözülüp denetlenir; nginx adı sonra yeniden çözer (DNS
// yeniden bağlama sınırı).

const errCodeProxyTargetNotAllowed = "PROXY_TARGET_NOT_ALLOWED"

// Reasons refine PROXY_TARGET_NOT_ALLOWED.
const (
	proxyTargetReasonInvalidURL = "invalid_url"
	proxyTargetReasonInternal   = "internal_address"
	proxyTargetReasonThisServer = "this_server"
	proxyTargetReasonUnresolved = "not_resolved"
)

const proxyTargetLookupTimeout = 5 * time.Second

// Test seams: tests never resolve real names or read real interfaces.
var (
	proxyTargetLookup = func(ctx context.Context, host string) ([]net.IPAddr, error) {
		return net.DefaultResolver.LookupIPAddr(ctx, host)
	}
	proxyTargetInterfaceAddrs = net.InterfaceAddrs
)

// proxyTargetRefusal says why an upstream was refused.
type proxyTargetRefusal struct {
	Reason string
	Host   string
}

func (r *proxyTargetRefusal) Error() string {
	return "proxy target refused: " + r.Reason
}

// Message is the plain English sentence of the refusal.
func (r *proxyTargetRefusal) Message() string {
	switch r.Reason {
	case proxyTargetReasonInternal:
		return fmt.Sprintf("%s is a local or private network address, or a name that points to one. "+
			"A site may only pass visitors to a public address, so services meant to stay inside this server or its network are not exposed. "+
			"Nothing was changed. Enter the public address of the application; for an application on this server, use the Node.js site type.", r.Host)
	case proxyTargetReasonThisServer:
		return fmt.Sprintf("%s is this server's own address, or a name that points to it. "+
			"A site may not pass visitors to this server's own services. "+
			"Nothing was changed. For an application on this server, use the Node.js site type; to send visitors to another site on this server, use forwarding.", r.Host)
	case proxyTargetReasonUnresolved:
		return fmt.Sprintf("The name %s could not be looked up from this server, so the panel cannot check where it points. "+
			"Nothing was changed. Check the name and its DNS records, then save again.", r.Host)
	default:
		return "The upstream must be an http:// or https:// address with a host name and no user name or password. " +
			"Nothing was changed. Enter an address such as https://app.example.com:8080."
	}
}

// writeProxyTargetRefusal answers 400 PROXY_TARGET_NOT_ALLOWED.
func writeProxyTargetRefusal(w http.ResponseWriter, refusal *proxyTargetRefusal) {
	var vars map[string]string
	if refusal.Host != "" {
		vars = map[string]string{"host": refusal.Host}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(apiErrorBody{
		Error:  refusal.Message(),
		Code:   errCodeProxyTargetNotAllowed,
		Reason: refusal.Reason,
		Vars:   vars,
	})
}

// checkProxyTarget returns nil when raw may be a proxy site's upstream.
// lookup resolves host names; own are this server's addresses.
func checkProxyTarget(
	ctx context.Context,
	raw string,
	lookup func(context.Context, string) ([]net.IPAddr, error),
	own []netip.Addr,
) *proxyTargetRefusal {
	invalid := &proxyTargetRefusal{Reason: proxyTargetReasonInvalidURL}
	if raw == "" || raw != strings.TrimSpace(raw) || strings.ContainsAny(raw, "\\\x00\r\n\t ") {
		return invalid
	}
	target, err := url.Parse(raw)
	if err != nil || target.Opaque != "" || target.User != nil || target.Fragment != "" {
		return invalid
	}
	if scheme := strings.ToLower(target.Scheme); scheme != "http" && scheme != "https" {
		return invalid
	}
	host := strings.ToLower(strings.TrimSuffix(target.Hostname(), "."))
	if host == "" {
		return invalid
	}

	var addrs []netip.Addr
	if addr, err := netip.ParseAddr(host); err == nil {
		if addr.Zone() != "" {
			return &proxyTargetRefusal{Reason: proxyTargetReasonInternal, Host: host}
		}
		addrs = []netip.Addr{addr}
	} else {
		if hostname.Validate(host) != nil {
			return invalid
		}
		// A numeric last label is an address in another notation
		// (127.1, 2130706433) that system resolvers accept.
		labels := strings.Split(host, ".")
		if last := labels[len(labels)-1]; strings.Trim(last, "0123456789") == "" || strings.HasPrefix(last, "0x") {
			return invalid
		}
		if host == "localhost" || strings.HasSuffix(host, ".localhost") {
			return &proxyTargetRefusal{Reason: proxyTargetReasonInternal, Host: host}
		}
		lookupCtx, cancel := context.WithTimeout(ctx, proxyTargetLookupTimeout)
		resolved, err := lookup(lookupCtx, host)
		cancel()
		if err != nil || len(resolved) == 0 {
			return &proxyTargetRefusal{Reason: proxyTargetReasonUnresolved, Host: host}
		}
		for _, ip := range resolved {
			addr, ok := netip.AddrFromSlice(ip.IP)
			if !ok {
				return &proxyTargetRefusal{Reason: proxyTargetReasonUnresolved, Host: host}
			}
			addrs = append(addrs, addr)
		}
	}
	// Every address must be allowed: nginx may use any of them.
	for _, addr := range addrs {
		if proxyTargetAddressInternal(addr) {
			return &proxyTargetRefusal{Reason: proxyTargetReasonInternal, Host: host}
		}
		for _, mine := range own {
			if mine.Unmap() == addr.WithZone("").Unmap() {
				return &proxyTargetRefusal{Reason: proxyTargetReasonThisServer, Host: host}
			}
		}
	}
	return nil
}

// Address ranges refused besides those netip names (loopback, private,
// link-local, multicast, unspecified).
var proxyTargetInternalPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),      // "this network"; 0.0.0.0 reaches this host
	netip.MustParsePrefix("100.64.0.0/10"),  // carrier-grade NAT
	netip.MustParsePrefix("192.0.0.0/24"),   // IETF protocol assignments
	netip.MustParsePrefix("198.18.0.0/15"),  // benchmarking
	netip.MustParsePrefix("240.0.0.0/4"),    // reserved, incl. 255.255.255.255
	netip.MustParsePrefix("::/96"),          // IPv4-compatible IPv6 (incl. ::1, ::)
	netip.MustParsePrefix("fec0::/10"),      // old site-local
	netip.MustParsePrefix("64:ff9b:1::/48"), // local-use NAT64
}

var proxyTargetNAT64 = netip.MustParsePrefix("64:ff9b::/96")

// proxyTargetAddressInternal reports an address that is not a public address
// of another machine. IPv4-mapped (::ffff:a.b.c.d) and NAT64 (64:ff9b::/96)
// forms are judged by the IPv4 address they carry.
func proxyTargetAddressInternal(addr netip.Addr) bool {
	addr = addr.WithZone("").Unmap()
	if addr.Is6() && proxyTargetNAT64.Contains(addr) {
		raw := addr.As16()
		return proxyTargetAddressInternal(netip.AddrFrom4([4]byte{raw[12], raw[13], raw[14], raw[15]}))
	}
	if !addr.IsValid() || addr.IsLoopback() || addr.IsPrivate() || addr.IsUnspecified() ||
		addr.IsLinkLocalUnicast() || addr.IsMulticast() || addr.IsLinkLocalMulticast() ||
		addr.IsInterfaceLocalMulticast() {
		return true
	}
	for _, prefix := range proxyTargetInternalPrefixes {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

// proxyTargetOwnAddresses are this server's addresses: every interface
// address, the detected public addresses (CELIKPANEL_SERVER_IP/_IPV6 behind
// NAT) and the address server setup recorded.
func (p *Panel) proxyTargetOwnAddresses(ctx context.Context) []netip.Addr {
	var own []netip.Addr
	add := func(value string) {
		if addr, err := netip.ParseAddr(strings.TrimSpace(value)); err == nil {
			own = append(own, addr.WithZone("").Unmap())
		}
	}
	if list, err := proxyTargetInterfaceAddrs(); err == nil {
		for _, item := range list {
			if network, ok := item.(*net.IPNet); ok {
				add(network.IP.String())
			}
		}
	}
	add(serverPrimaryIP())
	add(serverPrimaryIPv6())
	if state, err := p.loadServerSetup(ctx); err == nil {
		add(state.Draft.LocalIP)
	}
	return own
}

// refuseProxyTarget checks a proxy site's upstream and answers the refusal;
// true means the request was answered.
func (p *Panel) refuseProxyTarget(ctx context.Context, w http.ResponseWriter, raw string) bool {
	refusal := checkProxyTarget(ctx, strings.TrimSpace(raw), proxyTargetLookup, p.proxyTargetOwnAddresses(ctx))
	if refusal == nil {
		return false
	}
	writeProxyTargetRefusal(w, refusal)
	return true
}
