package serviceapps

import (
	"regexp"
	"sort"
	"strings"

	"github.com/alicelik/celikpanel/internal/sitesandbox"
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

// The listen proof lives in sitesandbox (alpha.119); these names keep the
// alpha.118 callers.
type (
	Listener    = sitesandbox.Listener
	ListenProof = sitesandbox.ListenProof
)

// ParseSS reads `ss -Htlnp` output.
func ParseSS(output string) []Listener { return sitesandbox.ParseSS(output) }

// ProveListening checks the listeners against the unit's processes.
func ProveListening(listeners []Listener, unitPIDs map[int]bool, port int) ListenProof {
	return sitesandbox.ProveListening(listeners, unitPIDs, port)
}
