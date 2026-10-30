package services

import (
	"net"
	"net/url"
	"os"
	"strings"
)

// A reverse-proxy site whose application address is a host name
// (2026-10-30).
//
// `proxy_pass http://app.example:8080;` makes nginx look the name up when it
// reads its configuration: `nginx -t`, every reload and the start at boot. A
// name that does not resolve at that moment (DNS not up yet at boot, the
// resolver unreachable, a typo, a record removed later) is "[emerg] host not
// found in upstream", and nginx refuses the WHOLE configuration: at boot no
// site on the server is served, and every later change of any site is
// refused by `nginx -t`. One site's address must never do that.
//
// nginx looks a name up when a visitor arrives, not when it starts, when the
// address in proxy_pass is a variable; it then asks the DNS servers named by
// `resolver`. So a host-name target is written as
//
//	resolver 127.0.0.53;
//	set $celikpanel_upstream http://app.example:8080;
//	proxy_pass $celikpanel_upstream;
//
// and an unresolvable name answers that one site's visitors 502 (logged in
// the site's error log) while nginx and every other site keep running. The
// resolver is the server's own: the nameservers of /etc/resolv.conf when the
// vhost is written (127.0.0.53 on a systemd-resolved host, which also answers
// /etc/hosts names), glibc's own default 127.0.0.1 when the file names none.
// The configuration stays plain nginx the owner can read and edit; it keeps
// working without CelikPanel (D-022).
//
// Targets that need no DNS keep the plain form, which nginx resolves once
// without a DNS server: an IP address (every Node.js site is 127.0.0.1),
// "localhost", and a name listed in /etc/hosts (nginx's runtime resolver
// would not read that file).
//
// A target with a path keeps nginx's own meaning of it: in `location /`,
// `proxy_pass http://h/base/;` sends /foo to /base/foo. With a variable,
// nginx would send every request to the path itself, so the path is applied
// by a `rewrite ... break;` before proxy_pass, which gives the same request
// URI. A path whose decoded form has characters nginx would read as syntax
// ($, quotes, spaces, braces...) cannot be written into that rewrite; such a
// target keeps the plain form (and nginx's start-time lookup).
//
// Uygulama adresi bir ana makine adı olan vekil site: nginx bu adı
// başlarken çözemezse TÜM yapılandırmayı reddeder ve sunucudaki hiçbir site
// açılmaz. Adres bir değişkenle yazılınca nginx adı ziyaretçi geldiğinde
// sunucunun kendi DNS'ine (/etc/resolv.conf) sorar; çözülemeyen ad yalnız o
// sitenin ziyaretçisine 502 verir. IP adresi, localhost ve /etc/hosts'taki
// adlar DNS gerektirmez, eski düz biçimde kalır.

const (
	resolvConfPath = "/etc/resolv.conf"
	etcHostsPath   = "/etc/hosts"
	// glibc reads at most three nameservers (MAXNS).
	maxProxyResolvers = 3
)

// readHostResolutionFile reads /etc/resolv.conf and /etc/hosts (a test seam:
// tests never read the machine's own files; nginx_features_hermetic_test.go).
var readHostResolutionFile = os.ReadFile

// HostResolution is what the vhost renderer knows of the server's own name
// resolution.
type HostResolution struct {
	// Nameservers are nginx `resolver` addresses (IPv6 in brackets).
	Nameservers []string
	// HostsNames are the lower-case names of /etc/hosts.
	HostsNames map[string]bool
}

// HostNameResolution reads /etc/resolv.conf and /etc/hosts. A file that
// cannot be read contributes nothing.
func HostNameResolution() HostResolution {
	resolution := HostResolution{HostsNames: map[string]bool{}}
	if raw, err := readHostResolutionFile(resolvConfPath); err == nil {
		resolution.Nameservers = parseResolvConfNameservers(string(raw))
	}
	if raw, err := readHostResolutionFile(etcHostsPath); err == nil {
		resolution.HostsNames = parseEtcHostsNames(string(raw))
	}
	return resolution
}

// parseResolvConfNameservers returns the nameserver lines' addresses in the
// form nginx's resolver takes. A link-local IPv6 address with a zone
// ("fe80::1%eth0") is left out: nginx's resolver cannot name an interface.
func parseResolvConfNameservers(text string) []string {
	var servers []string
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != "nameserver" {
			continue
		}
		ip := net.ParseIP(fields[1])
		if ip == nil || strings.Contains(fields[1], "%") {
			continue
		}
		address := ip.String()
		if ip.To4() == nil {
			address = "[" + address + "]"
		}
		servers = append(servers, address)
		if len(servers) == maxProxyResolvers {
			break
		}
	}
	return servers
}

func parseEtcHostsNames(text string) map[string]bool {
	names := map[string]bool{}
	for _, line := range strings.Split(text, "\n") {
		if cut := strings.IndexByte(line, '#'); cut >= 0 {
			line = line[:cut]
		}
		fields := strings.Fields(line)
		if len(fields) < 2 || net.ParseIP(fields[0]) == nil {
			continue
		}
		for _, name := range fields[1:] {
			names[strings.ToLower(strings.TrimSuffix(name, "."))] = true
		}
	}
	return names
}

// proxyResolution is the generator's view of the server's name resolution (a
// test seam: hostResolution).
func (ng *NginxGenerator) proxyResolution() HostResolution {
	if ng.hostResolution != nil {
		return ng.hostResolution()
	}
	return HostNameResolution()
}

// SetHostResolution fixes the generator's view of the server's name
// resolution (tests and tools that render for another host).
func (ng *NginxGenerator) SetHostResolution(resolution HostResolution) {
	ng.hostResolution = func() HostResolution { return resolution }
}

// runtimeProxy is the variable form of a proxy target.
type runtimeProxy struct {
	Origin    string // scheme://host[:port]
	RewriteTo string // "" or the rewrite replacement that applies the path
	Resolver  string // the resolver directive's addresses
}

// runtimeResolvedProxy decides the form of a proxy target. ok is false when
// the plain form is written: the target needs no DNS, or cannot be written in
// the variable form.
func runtimeResolvedProxy(target string, resolution HostResolution) (runtimeProxy, bool) {
	parsed, err := url.Parse(target)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") {
		return runtimeProxy{}, false
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	if host == "" || net.ParseIP(host) != nil || host == "localhost" || resolution.HostsNames[host] {
		return runtimeProxy{}, false
	}
	proxy := runtimeProxy{Origin: parsed.Scheme + "://" + parsed.Host}
	if !nginxSafeProxyPart(proxy.Origin) {
		return runtimeProxy{}, false
	}
	if (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" {
		path := parsed.Path
		if path == "" {
			path = "/"
		}
		// The rewrite's result is the decoded request path (nginx escapes it
		// when it proxies), so the decoded path goes in; the query is kept
		// as written (nginx passes arguments as they are).
		if !nginxSafeProxyPart(path) || !nginxSafeProxyPart(parsed.RawQuery) || strings.Contains(path, "?") {
			return runtimeProxy{}, false
		}
		proxy.RewriteTo = path + "$1"
		if parsed.RawQuery != "" {
			proxy.RewriteTo += "?" + parsed.RawQuery
		}
	}
	servers := resolution.Nameservers
	if len(servers) == 0 {
		// glibc's own default when resolv.conf names no nameserver.
		servers = []string{"127.0.0.1"}
	}
	proxy.Resolver = strings.Join(servers, " ")
	return proxy, true
}

// nginxSafeProxyPart: only characters nginx reads literally in an unquoted
// directive argument and in a rewrite replacement (no variables, quotes,
// spaces, braces, semicolons, backslashes or control characters).
func nginxSafeProxyPart(value string) bool {
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case strings.ContainsRune("-._~/:@!&()*+,=%[]", r):
		default:
			return false
		}
	}
	return true
}
