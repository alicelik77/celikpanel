package sitesandbox

import (
	"net"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// The listen proof (moved from serviceapps, alpha.118): which addresses the
// processes of a unit listen on.

// Listener is one listening TCP socket and the processes that hold it.
type Listener struct {
	Address string
	Port    int
	PIDs    []int
}

var ssUsersPID = regexp.MustCompile(`pid=([0-9]+)`)

// ParseSS reads `ss -Htlnp` output.
func ParseSS(output string) []Listener {
	out := []Listener{}
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 || fields[0] != "LISTEN" {
			continue
		}
		local := fields[3]
		host, portText, err := net.SplitHostPort(strings.ReplaceAll(local, "%", "%25"))
		if err != nil {
			continue
		}
		host, _, _ = strings.Cut(host, "%")
		port, err := strconv.Atoi(portText)
		if err != nil {
			continue
		}
		l := Listener{Address: host, Port: port}
		for _, m := range ssUsersPID.FindAllStringSubmatch(line, -1) {
			if pid, err := strconv.Atoi(m[1]); err == nil {
				l.PIDs = append(l.PIDs, pid)
			}
		}
		out = append(out, l)
	}
	return out
}

// ListenProof is what the Agent proves about a running install.
type ListenProof struct {
	// Serving: a process of the unit listens on 127.0.0.1:port.
	Serving bool `json:"serving"`
	// Exposed lists addresses other than loopback the unit listens on; any
	// entry is a failure (the app must be reachable only through nginx).
	Exposed []string `json:"exposed,omitempty"`
	// Squatted: another process (not this unit's) holds 127.0.0.1:port.
	Squatted bool `json:"squatted,omitempty"`
}

func loopback(host string) bool {
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// ProveListening checks the listeners against the unit's processes.
func ProveListening(listeners []Listener, unitPIDs map[int]bool, port int) ListenProof {
	proof := ListenProof{}
	for _, l := range listeners {
		ours := false
		for _, pid := range l.PIDs {
			if unitPIDs[pid] {
				ours = true
			}
		}
		if ours && !loopback(l.Address) {
			proof.Exposed = append(proof.Exposed, net.JoinHostPort(l.Address, strconv.Itoa(l.Port)))
		}
		if l.Port == port && l.Address == "127.0.0.1" {
			if ours {
				proof.Serving = true
			} else if len(l.PIDs) > 0 {
				proof.Squatted = true
			}
		}
		if l.Port == port && !ours && len(l.PIDs) > 0 && (l.Address == "0.0.0.0" || l.Address == "*" || l.Address == "::") {
			proof.Squatted = true
		}
	}
	sort.Strings(proof.Exposed)
	return proof
}
