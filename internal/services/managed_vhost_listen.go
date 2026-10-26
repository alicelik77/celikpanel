package services

import (
	"net"
	"os"
	"sort"
	"strings"
)

// Which address the validation probe asks (set11 cell A6, 2026-10-11; D-031
// step 1b; D-025 invariant 2).
//
// The probe asked 127.0.0.1 for every name. When the site's port-80 block
// listens on one address only (`listen 10.0.2.15:80;`), nginx answers a
// request to 127.0.0.1 from the default server, and the probe reported
// include_missing for a file that would serve the validation on its own
// address. So the probe now asks the addresses the kept file's port-80 server
// blocks that carry the name listen on. The reader is small and conservative:
//
//   - only the kept file is read (an `include` is not followed);
//   - a server block counts for a name when its server_name has the name
//     exactly (case-insensitive); regular expressions and wildcards are not
//     matched;
//   - port 80 is `listen 80`, `listen *:80`, `listen <addr>:80`,
//     `listen [<v6>]:80` or `listen <addr>` (nginx's default port); a block
//     without any listen directive listens on *:80; `default_server` and the
//     other parameters are ignored; `unix:` sockets are not port 80;
//   - an IPv4 wildcard (`80`, `*:80`, `0.0.0.0:80`) is asked on 127.0.0.1, as
//     before; an IPv6 wildcard (`[::]:80`) is asked on ::1, but only when no
//     block of the name listens on the IPv4 wildcard; an explicit address is
//     asked on that address when it is an address of this server, and
//     otherwise cannot be asked from this server at all;
//   - a name no port-80 block of the file carries is asked on 127.0.0.1, as
//     before (nginx's default server answers it, and the probe says so).
//
// Hangi adresin sorulacağı: sahibin dosyasında adı taşıyan 80 numaralı
// bağlantı noktası bloklarının dinlediği adresler sorulur. Genel adres
// 127.0.0.1 (IPv6 genel adresi ::1); bu sunucunun olmayan bir adres sunucunun
// kendisinden sorulamaz ve sonuç "bilinmiyor" olur.

const (
	probeLoopbackV4 = "127.0.0.1"
	probeLoopbackV6 = "::1"
	// A kept file larger than this is not read for its listen directives:
	// every name is asked on 127.0.0.1, as before.
	probeListenFileLimit = 1 << 20
)

// probeTarget is one address a name is asked on. Local is false for an
// explicit address that is not an address of this server.
type probeTarget struct {
	address string
	local   bool
}

// probeListen is one port-80 listen of a server block.
type probeListen struct {
	wildcardV4 bool
	wildcardV6 bool
	address    string // explicit: an IP literal, or a host name as written
}

// nginxTokens splits nginx configuration text into words, `{`, `}` and `;`.
// Comments are dropped; quoted strings are one word without their quotes.
func nginxTokens(text string) []string {
	var tokens []string
	var word strings.Builder
	flush := func() {
		if word.Len() > 0 {
			tokens = append(tokens, word.String())
			word.Reset()
		}
	}
	for index := 0; index < len(text); index++ {
		c := text[index]
		switch {
		case c == '#':
			flush()
			for index < len(text) && text[index] != '\n' {
				index++
			}
		case c == '"' || c == '\'':
			flush()
			end := strings.IndexByte(text[index+1:], c)
			if end < 0 {
				tokens = append(tokens, text[index+1:])
				return tokens
			}
			tokens = append(tokens, text[index+1:index+1+end])
			index += end + 1
		case c == '{' || c == '}' || c == ';':
			flush()
			tokens = append(tokens, string(c))
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			flush()
		case c == '\\' && index+1 < len(text):
			word.WriteByte(text[index+1])
			index++
		default:
			word.WriteByte(c)
		}
	}
	flush()
	return tokens
}

// nginxServerBlock is what the reader keeps of one `server { … }` block.
type nginxServerBlock struct {
	names   []string
	listens [][]string // each listen directive's arguments
}

// nginxServerBlocks returns the server blocks of the text (a `server` word
// followed by `{`; an upstream's `server address;` is not one). Only the
// block's own directives are read, not those of a nested location.
func nginxServerBlocks(text string) []nginxServerBlock {
	tokens := nginxTokens(text)
	var blocks []nginxServerBlock
	depth := 0
	serverDepth := -1
	var current *nginxServerBlock
	var statement []string
	for index := 0; index < len(tokens); index++ {
		token := tokens[index]
		switch token {
		case "{":
			if current == nil && len(statement) == 1 && statement[0] == "server" {
				blocks = append(blocks, nginxServerBlock{})
				current = &blocks[len(blocks)-1]
				serverDepth = depth
			}
			depth++
			statement = statement[:0]
		case "}":
			depth--
			if current != nil && depth == serverDepth {
				current, serverDepth = nil, -1
			}
			statement = statement[:0]
		case ";":
			if current != nil && depth == serverDepth+1 && len(statement) > 0 {
				switch statement[0] {
				case "listen":
					current.listens = append(current.listens, append([]string(nil), statement[1:]...))
				case "server_name":
					current.names = append(current.names, statement[1:]...)
				}
			}
			statement = statement[:0]
		default:
			statement = append(statement, token)
		}
	}
	return blocks
}

// parsePort80Listen reads one listen directive's first argument; ok is false
// when it does not listen on TCP port 80.
func parsePort80Listen(arguments []string) (probeListen, bool) {
	if len(arguments) == 0 {
		return probeListen{}, false
	}
	value := arguments[0]
	if strings.HasPrefix(value, "unix:") {
		return probeListen{}, false
	}
	host, port := "", ""
	switch {
	case strings.HasPrefix(value, "["):
		end := strings.IndexByte(value, ']')
		if end < 0 {
			return probeListen{}, false
		}
		host, port = value[1:end], strings.TrimPrefix(value[end+1:], ":")
		if value[end+1:] != "" && !strings.HasPrefix(value[end+1:], ":") {
			return probeListen{}, false
		}
	case isAllDigits(value):
		port = value
	default:
		if colon := strings.LastIndexByte(value, ':'); colon >= 0 {
			host, port = value[:colon], value[colon+1:]
		} else {
			host = value
		}
	}
	if port == "" {
		port = "80"
	}
	if port != "80" {
		return probeListen{}, false
	}
	switch host {
	case "", "*", "0.0.0.0":
		return probeListen{wildcardV4: true}, true
	}
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsUnspecified() {
			if ip.To4() != nil {
				return probeListen{wildcardV4: true}, true
			}
			return probeListen{wildcardV6: true}, true
		}
		return probeListen{address: ip.String()}, true
	}
	return probeListen{address: host}, true
}

func isAllDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, c := range value {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// probeListensByName returns, for each of the names, the port-80 listens of
// the file's server blocks that carry it. A name no such block carries is
// absent from the map.
func probeListensByName(text string, names []string) map[string][]probeListen {
	out := map[string][]probeListen{}
	for _, block := range nginxServerBlocks(text) {
		var listens []probeListen
		if len(block.listens) == 0 {
			listens = []probeListen{{wildcardV4: true}}
		}
		for _, arguments := range block.listens {
			if listen, ok := parsePort80Listen(arguments); ok {
				listens = append(listens, listen)
			}
		}
		if len(listens) == 0 {
			continue
		}
		for _, name := range names {
			for _, candidate := range block.names {
				if strings.EqualFold(candidate, name) {
					out[name] = append(out[name], listens...)
					break
				}
			}
		}
	}
	return out
}

// probeTargetsFor is the addresses one name is asked on, in a fixed order:
// the IPv4 wildcard's 127.0.0.1 first, then explicit addresses sorted, then
// ::1 for an IPv6 wildcard that no IPv4 wildcard covers.
func probeTargetsFor(listens []probeListen, local func(string) bool) []probeTarget {
	if len(listens) == 0 {
		return []probeTarget{{address: probeLoopbackV4, local: true}}
	}
	wildcardV4, wildcardV6 := false, false
	explicit := map[string]bool{}
	for _, listen := range listens {
		switch {
		case listen.wildcardV4:
			wildcardV4 = true
		case listen.wildcardV6:
			wildcardV6 = true
		case listen.address != "":
			explicit[listen.address] = true
		}
	}
	var targets []probeTarget
	if wildcardV4 {
		targets = append(targets, probeTarget{address: probeLoopbackV4, local: true})
	}
	addresses := make([]string, 0, len(explicit))
	for address := range explicit {
		addresses = append(addresses, address)
	}
	sort.Strings(addresses)
	for _, address := range addresses {
		targets = append(targets, probeTarget{address: address, local: local(address)})
	}
	if wildcardV6 && !wildcardV4 {
		targets = append(targets, probeTarget{address: probeLoopbackV6, local: true})
	}
	return targets
}

// hostAddressSet lists this server's interface addresses (loopback
// included), the way the Agent lists them elsewhere (net.InterfaceAddrs).
func hostAddressSet() (map[string]bool, error) {
	addresses, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, address := range addresses {
		if network, ok := address.(*net.IPNet); ok {
			out[network.IP.String()] = true
		}
	}
	return out, nil
}

func (ng *NginxGenerator) probeHostAddresses() (map[string]bool, error) {
	if ng.hostAddresses != nil {
		return ng.hostAddresses()
	}
	return hostAddressSet()
}

// probePlan is, for every name of the probe, the addresses to ask. A kept
// file that cannot be read leaves every name on 127.0.0.1, as before.
func (ng *NginxGenerator) probePlan(domain string, names []string) (map[string][]probeTarget, error) {
	plan := map[string][]probeTarget{}
	text := ""
	available, _ := vhostPaths(domain)
	if info, err := os.Stat(available); err == nil && info.Mode().IsRegular() && info.Size() <= probeListenFileLimit {
		if content, err := os.ReadFile(available); err == nil {
			text = string(content)
		}
	}
	listens := probeListensByName(text, names)
	var hostAddresses map[string]bool
	var listErr error
	listed := false
	local := func(address string) bool {
		if !listed {
			hostAddresses, listErr = ng.probeHostAddresses()
			listed = true
		}
		if ip := net.ParseIP(address); ip != nil {
			address = ip.String()
		}
		return hostAddresses[address]
	}
	for _, name := range names {
		plan[name] = probeTargetsFor(listens[name], local)
	}
	if listErr != nil {
		return plan, listErr
	}
	return plan, nil
}

// NginxServerBlock is one `server { … }` block's own names and listen
// directives, for a reader outside this package (alpha.110: the webmail
// address refuses a name another server block already answers for).
type NginxServerBlock struct {
	Names   []string
	Listens [][]string
}

// NginxServerBlocks reads the server blocks of an nginx text (a file or one
// file's part of `nginx -T`).
func NginxServerBlocks(text string) []NginxServerBlock {
	blocks := nginxServerBlocks(text)
	out := make([]NginxServerBlock, 0, len(blocks))
	for _, block := range blocks {
		out = append(out, NginxServerBlock{Names: block.names, Listens: block.listens})
	}
	return out
}
