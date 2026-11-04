package siteruntime

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Arguments are tokens: the customer gives a list, never a command line, and
// each token becomes one word of ExecStart. systemd expands "$" and "%" in a
// unit's command line and splits or unquotes on spaces, quotes and
// backslashes; no escape is right for every case, so a token with any of them
// is refused, never escaped. "{port}" is the only placeholder and becomes the
// app's port.

// PortPlaceholder is the only placeholder an argument may hold.
const PortPlaceholder = "{port}"

// MaxArgs is the largest number of arguments.
const MaxArgs = 32

var argPattern = regexp.MustCompile(`^(?:[A-Za-z0-9._@:/+=,-]|\{port\})+$`)

// ValidateArgs checks the customer's arguments.
func ValidateArgs(args []string) error {
	if len(args) > MaxArgs {
		return fmt.Errorf("at most %d arguments", MaxArgs)
	}
	for i, arg := range args {
		if arg == "" || len(arg) > 256 {
			return fmt.Errorf("argument %d is empty or longer than 256 characters", i+1)
		}
		if !argPattern.MatchString(arg) {
			return fmt.Errorf("argument %d may hold only letters, digits and . _ @ : / + = , - (and {port}); spaces, quotes, $ and %% are not allowed", i+1)
		}
	}
	return nil
}

// ExpandArgs replaces the placeholder with the port.
func ExpandArgs(args []string, port int) ([]string, error) {
	if err := ValidateArgs(args); err != nil {
		return nil, err
	}
	out := make([]string, len(args))
	for i, arg := range args {
		out[i] = strings.ReplaceAll(arg, PortPlaceholder, strconv.Itoa(port))
	}
	return out, nil
}

// The environment. Keys the sandbox or CelikPanel sets itself, and keys that
// change how a program is loaded, are refused: PATH, HOME, PORT and HOST are
// CelikPanel's (the program reads PORT and HOST to know where to listen),
// LD_* would load other code into the program, PYTHONPATH and PYTHONHOME
// would do the same for Python. Values are one line of text.

var (
	envKeyPattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)
	refusedKeys   = map[string]bool{
		"PATH": true, "HOME": true, "PORT": true, "HOST": true,
		"PYTHONPATH": true, "PYTHONHOME": true, "PYTHONSTARTUP": true,
		"NOTIFY_SOCKET": true, "LISTEN_FDS": true, "LISTEN_PID": true, "LISTEN_FDNAMES": true,
		"USER": true, "LOGNAME": true, "SHELL": true,
	}
	refusedPrefixes = []string{"LD_", "CELIKPANEL_", "SYSTEMD_"}
)

// MaxEnv is the largest number of environment entries.
const MaxEnv = 64

// MinSecretLength is the shortest secret value: logs hide secret values of
// at least this length (serviceapps.Redact skips shorter ones).
const MinSecretLength = 8

// EnvOwnerKeys lists the keys of the file on the server that CelikPanel did
// not write last time (previous) and that this request does not name: the
// owner added them by hand.
func EnvOwnerKeys(existing map[string]string, previous []string, entries []EnvEntry) []string {
	known := map[string]bool{}
	for _, k := range previous {
		known[k] = true
	}
	for _, e := range entries {
		known[e.Key] = true
	}
	out := []string{}
	for k := range existing {
		if !known[k] {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// EnvKeyRefusal says why a key is refused ("" = allowed).
func EnvKeyRefusal(key string) string {
	if !envKeyPattern.MatchString(key) {
		return "a key is upper-case letters, digits and _ (starting with a letter), up to 64 characters"
	}
	if refusedKeys[key] {
		return key + " is set by CelikPanel or the sandbox and cannot be changed"
	}
	for _, prefix := range refusedPrefixes {
		if strings.HasPrefix(key, prefix) {
			return prefix + "* keys change how a program is loaded or belong to CelikPanel and cannot be set"
		}
	}
	return ""
}

// ValidEnvValue: one line of printable text, 1..4096 bytes.
func ValidEnvValue(value string) bool {
	if value == "" || len(value) > 4096 {
		return false
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

// EnvEntry is one environment value. A secret is write-only: its value is
// never shown again, never stored by the Panel and hidden in every log line.
type EnvEntry struct {
	Key    string `json:"key"`
	Value  string `json:"value,omitempty"`
	Secret bool   `json:"secret"`
}

// ValidateEnv checks a complete set of entries (a secret may come without a
// value when it is kept from the file on the server).
func ValidateEnv(entries []EnvEntry) error {
	if len(entries) > MaxEnv {
		return fmt.Errorf("at most %d environment values", MaxEnv)
	}
	seen := map[string]bool{}
	for _, e := range entries {
		if why := EnvKeyRefusal(e.Key); why != "" {
			return errors.New(why)
		}
		if seen[e.Key] {
			return fmt.Errorf("%s is given twice", e.Key)
		}
		seen[e.Key] = true
		if e.Value == "" && e.Secret {
			continue
		}
		// A secret shorter than this could not be hidden in a log line
		// without hiding ordinary words too.
		if e.Secret && len(e.Value) < MinSecretLength {
			return fmt.Errorf("a secret value is at least %d characters (%s)", MinSecretLength, e.Key)
		}
		if !ValidEnvValue(e.Value) {
			return fmt.Errorf("the value of %s is not one line of text (1 to 4096 characters)", e.Key)
		}
	}
	return nil
}

func quoteEnv(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return `"` + value + `"`
}

// RenderEnv is the environment file text. systemd expands nothing in an
// EnvironmentFile; values are double-quoted with backslash and quote escaped.
func RenderEnv(entries map[string]string) (string, error) {
	keys := make([]string, 0, len(entries))
	for key, value := range entries {
		if why := EnvKeyRefusal(key); why != "" {
			return "", errors.New(why)
		}
		if !ValidEnvValue(value) {
			return "", fmt.Errorf("the value of %s is not one line of text", key)
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("# Environment of a CelikPanel runtime app (docs/RUNTIME-APPS.md). Root's file (0600);\n")
	b.WriteString("# systemd reads it before the sandbox starts. PATH, HOME, PORT and HOST are set in the unit.\n")
	for _, key := range keys {
		b.WriteString(key + "=" + quoteEnv(entries[key]) + "\n")
	}
	return b.String(), nil
}

// ParseEnv reads a file written by RenderEnv (or edited in the same form).
// Unknown and refused lines are ignored.
func ParseEnv(text string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || EnvKeyRefusal(key) != "" {
			continue
		}
		if len(value) >= 2 && strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`) {
			inner := value[1 : len(value)-1]
			var b strings.Builder
			for i := 0; i < len(inner); i++ {
				if inner[i] == '\\' && i+1 < len(inner) {
					i++
				}
				b.WriteByte(inner[i])
			}
			value = b.String()
		}
		out[key] = value
	}
	return out
}

// MergeEnv builds the file's values: every public entry with its value, every
// secret with its new value or, without one, the value already on the server
// (a secret whose key is not listed any more is dropped).
func MergeEnv(entries []EnvEntry, existing map[string]string) (map[string]string, error) {
	if err := ValidateEnv(entries); err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, e := range entries {
		switch {
		case e.Value != "":
			out[e.Key] = e.Value
		case e.Secret && existing[e.Key] != "":
			out[e.Key] = existing[e.Key]
		}
	}
	return out, nil
}
