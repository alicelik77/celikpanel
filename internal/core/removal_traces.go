package core

import (
	"sort"
	"strings"
)

// Where local programs reach a component (alpha.108, D-038 §13). Before a
// removal the Agent looks, bounded and read-only, at the system's own traces:
// web-server configuration that points at the component's socket or local
// port, services that depend on its unit, and local programs connected to it.
// These facts say where to look, per component, in one place; the removal
// path derives everything from here and from the catalogue (FirewallPorts,
// SystemNames, HelperUnits) instead of naming sockets at its call sites.
//
// Bir bileşene yerel programların nereden ulaştığı: kaldırmadan önce sistemin
// kendi izlerine bakmak için tek kaynak.

// LocalEndpoints are a component's local sockets and ports beyond the
// catalogue's public FirewallPorts. A socket is a glob; "{v}" stands for the
// version being removed ("8.4"), and a socket without it belongs to the
// component as a whole (it is not looked at for one version's removal).
type LocalEndpoints struct {
	Sockets []string
	Ports   []int
}

var localEndpoints = map[string]LocalEndpoints{
	"php-fpm": {Sockets: []string{"/run/php/php{v}-fpm*.sock", "/var/run/php/php{v}-fpm*.sock", "/run/php-fpm/*.sock"}},
	"mariadb": {Sockets: []string{"/run/mysqld/mysqld.sock", "/var/run/mysqld/mysqld.sock"}, Ports: []int{3306}},
	"postgresql": {
		Sockets: []string{"/run/postgresql/.s.PGSQL.*", "/var/run/postgresql/.s.PGSQL.*"},
		Ports:   []int{5432},
	},
	"redis":        {Sockets: []string{"/run/redis/*.sock", "/var/run/redis/*.sock"}, Ports: []int{6379}},
	"valkey":       {Sockets: []string{"/run/valkey/*.sock", "/var/run/valkey/*.sock"}, Ports: []int{6379}},
	"memcached":    {Sockets: []string{"/run/memcached/*.sock"}, Ports: []int{11211}},
	"rspamd":       {Sockets: []string{"/run/rspamd/*.sock"}, Ports: []int{11333, 11334}},
	"spamassassin": {Ports: []int{783}},
	"clamav":       {Sockets: []string{"/run/clamav/clamd.ctl", "/var/run/clamav/clamd.ctl"}, Ports: []int{3310}},
	"netdata":      {Ports: []int{19999}},
}

// RemovalTraceFacts is what a removal looks for on the system's own traces.
type RemovalTraceFacts struct {
	// Sockets are unix socket globs (version substituted).
	Sockets []string
	// Ports are local TCP ports clients connect to.
	Ports []int
	// WebConfig: the web-server configuration is searched for the sockets
	// and ports. A web server's own configuration is not "a trace" of it.
	WebConfig bool
}

// RemovalTraceFactsFor derives the facts for a component, or one version of
// it ("" = the whole component).
func RemovalTraceFactsFor(svc *ManagedService, version string) RemovalTraceFacts {
	facts := RemovalTraceFacts{WebConfig: svc != nil && svc.ConflictGroup != "web-server"}
	if svc == nil {
		return facts
	}
	endpoints := localEndpoints[svc.ID]
	for _, socket := range endpoints.Sockets {
		switch {
		case strings.Contains(socket, "{v}") && version != "":
			facts.Sockets = append(facts.Sockets, strings.ReplaceAll(socket, "{v}", version))
		case strings.Contains(socket, "{v}"):
			facts.Sockets = append(facts.Sockets, strings.ReplaceAll(socket, "{v}", "*"))
		case version == "":
			facts.Sockets = append(facts.Sockets, socket)
		}
	}
	// A whole component's ports: its own local ones and the public ones it
	// asks the firewall for. One version's removal cannot tell its port from
	// another version's, so it looks at sockets only.
	if version == "" {
		seen := map[int]bool{}
		for _, port := range endpoints.Ports {
			seen[port] = true
		}
		for _, port := range svc.FirewallPorts {
			if port.Proto == "tcp" {
				seen[port.Port] = true
			}
		}
		for port := range seen {
			facts.Ports = append(facts.Ports, port)
		}
		sort.Ints(facts.Ports)
	}
	return facts
}
