package serviceapps

import (
	"net"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Hidden replaces a secret in every log line CelikPanel shows.
const Hidden = "[hidden]"

var redactPatterns = []struct {
	re   *regexp.Regexp
	with string
}{
	// A bearer token first: "Authorization: Bearer x" must lose the token,
	// not only the word "Bearer".
	{regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/=-]{8,}`), "Bearer " + Hidden},
	// key=value / "key": "value" for names that carry secrets.
	{regexp.MustCompile(`(?i)\b([A-Z0-9_]*(?:api[_-]?key|token|secret|password|passwd|encryption[_-]?key)[A-Z0-9_]*)(["']?\s*[:=]\s*["']?)([^\s"',;]+)`), "${1}${2}" + Hidden},
	{regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{16,}`), Hidden},
	{regexp.MustCompile(`\$2[aby]\$[0-9]{2}\$[./A-Za-z0-9]{53}`), Hidden},
	// user:password@ in a URL.
	{regexp.MustCompile(`(://[^/\s:@]+:)[^@\s/]+@`), "${1}" + Hidden + "@"},
}

// Redact hides every known secret value (longest first) and every value that
// looks like a key, token or password. It is applied by the Agent before a
// log line leaves the server, so the Panel never receives the secret.
func Redact(line string, secrets []string) string {
	values := make([]string, 0, len(secrets))
	for _, s := range secrets {
		if len(s) >= 6 {
			values = append(values, s)
		}
	}
	sort.Slice(values, func(i, j int) bool { return len(values[i]) > len(values[j]) })
	for _, s := range values {
		line = strings.ReplaceAll(line, s, Hidden)
	}
	for _, p := range redactPatterns {
		line = p.re.ReplaceAllString(line, p.with)
	}
	return line
}

// SecretValues lists the values of an environment file that are secret: the
// customer's keys, the generated keys and the owner's password or hash.
func SecretValues(app App, env map[string]string) []string {
	out := []string{}
	for _, s := range app.Secrets {
		if v := env[s.Key]; v != "" && !strings.HasPrefix(v, "http") {
			out = append(out, v)
		}
	}
	for _, key := range GeneratedKeys(app) {
		if v := env[key]; v != "" {
			out = append(out, v)
		}
	}
	if _, secretKey := OwnerKeys(app); secretKey != "" && env[secretKey] != "" {
		out = append(out, env[secretKey])
	}
	return out
}

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
