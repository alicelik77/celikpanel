// Package panellisten is the single description of the panel's own listen
// address: its default port, how the port is read from the panel's
// configured listen address (CELIKPANEL_LISTEN in /etc/celikpanel/panel.env),
// which address the Agent dials to reach it on this server, and which ports
// the owner may choose instead.
//
// New installations listen on 1923 (owner decision 2026-10-31: 2083 is
// cPanel's port). Installations made before alpha.114 keep 2083: their
// panel.env already says so and install/update never rewrite it.
//
// panellisten, panelin kendi dinleme adresinin tek tanımıdır. Yeni kurulum
// 1923'ü dinler; alpha.114 öncesi kurulumlar panel.env'de yazan 2083'ü korur.
package panellisten

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
)

const (
	// DefaultPort is the panel port of a new installation.
	DefaultPort = 1923
	// LegacyPort is the port of installations made before alpha.114.
	LegacyPort = 2083
	// EnvKey is the panel.env key holding the listen address.
	EnvKey = "CELIKPANEL_LISTEN"
	// EnvFile is the root-only file systemd reads after the unit's own
	// Environment= lines; it is the durable source of the listen address.
	EnvFile = "/etc/celikpanel/panel.env"
	// MinChoice and MaxChoice bound a port the owner may choose. Below 1024
	// a port belongs to system services, and the panel runs unprivileged.
	MinChoice = 1024
	MaxChoice = 65535
)

// DefaultListen is the listen address of a new installation.
var DefaultListen = ":" + strconv.Itoa(DefaultPort)

// Port returns the TCP port of a listen address (":1923", "0.0.0.0:2083",
// "[::]:1923", "192.0.2.10:1923").
func Port(listen string) (int, error) {
	_, portText, err := net.SplitHostPort(strings.TrimSpace(listen))
	if err != nil {
		return 0, fmt.Errorf("panel listen address %q: %w", listen, err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("panel listen address %q has no valid port", listen)
	}
	return port, nil
}

// WithPort keeps the listen address's host and replaces its port.
func WithPort(listen string, port int) (string, error) {
	host, _, err := net.SplitHostPort(strings.TrimSpace(listen))
	if err != nil {
		return "", fmt.Errorf("panel listen address %q: %w", listen, err)
	}
	if port < 1 || port > 65535 {
		return "", fmt.Errorf("port %d is out of range", port)
	}
	return net.JoinHostPort(host, strconv.Itoa(port)), nil
}

// ProbeAddress is the address this server dials to reach a listener bound to
// listen: wildcard binds through the matching loopback address, a specific
// address as it is. It matches update.sh's panel_startup_probe_address.
func ProbeAddress(listen string) (string, error) {
	host, _, err := net.SplitHostPort(strings.TrimSpace(listen))
	if err != nil {
		return "", fmt.Errorf("panel listen address %q: %w", listen, err)
	}
	port, err := Port(listen)
	if err != nil {
		return "", err
	}
	switch host {
	case "", "0.0.0.0", "localhost":
		host = "127.0.0.1"
	case "::":
		host = "::1"
	}
	return net.JoinHostPort(host, strconv.Itoa(port)), nil
}

// ListenFromEnv returns the CELIKPANEL_LISTEN value of a panel.env body.
// panel.env is data (KEY=value lines, blank and # comment lines), never
// shell code; a repeated key is an error, as install.sh treats it.
func ListenFromEnv(raw []byte) (string, bool, error) {
	value, found := "", false
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, rest, ok := strings.Cut(line, "=")
		if !ok {
			return "", false, errors.New("panel.env has a line without '='")
		}
		if key != EnvKey {
			continue
		}
		if found {
			return "", false, errors.New("panel.env repeats " + EnvKey)
		}
		value, found = rest, true
	}
	if err := scanner.Err(); err != nil {
		return "", false, err
	}
	return value, found, nil
}

// ReplaceListen returns raw with its single CELIKPANEL_LISTEN line changed
// from "from" to "to". Every other byte is kept, so an owner's comments and
// settings survive. It refuses when the file does not say "from" exactly.
func ReplaceListen(raw []byte, from, to string) ([]byte, error) {
	current, found, err := ListenFromEnv(raw)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errors.New("panel.env has no " + EnvKey + " line")
	}
	if current != from {
		return nil, fmt.Errorf("panel.env says %s=%s, not %s", EnvKey, current, from)
	}
	if _, err := Port(to); err != nil {
		return nil, err
	}
	old := []byte(EnvKey + "=" + from)
	lines := bytes.SplitAfter(raw, []byte("\n"))
	replaced := 0
	for i, line := range lines {
		body := bytes.TrimSuffix(line, []byte("\n"))
		if bytes.Equal(body, old) {
			lines[i] = append([]byte(EnvKey+"="+to), line[len(body):]...)
			replaced++
		}
	}
	if replaced != 1 {
		return nil, errors.New("panel.env listen line could not be replaced exactly once")
	}
	return bytes.Join(lines, nil), nil
}

// reservedChoices are ports at or above 1024 that services CelikPanel
// installs or commonly finds on a server use. Mail, DNS and web ports are
// below 1024 and outside the range already.
var reservedChoices = map[int]string{
	1953:  "PowerDNS",
	3306:  "MySQL/MariaDB",
	4190:  "mail filter rules (ManageSieve)",
	5432:  "PostgreSQL",
	6379:  "Redis",
	8053:  "DNS",
	8081:  "the PowerDNS API",
	8953:  "the DNS resolver control",
	10024: "mail filtering",
	10025: "mail filtering",
	11211: "Memcached",
	11332: "Rspamd",
	11333: "Rspamd",
	11334: "Rspamd",
	51820: "WireGuard VPN",
}

// ChoiceRefusal says why port cannot be the panel port, or "" when it can.
// extraReserved adds ports other parts of the panel know are taken.
func ChoiceRefusal(port int, extraReserved map[int]string) (code, reason string) {
	switch {
	case port < MinChoice || port > MaxChoice:
		return "PANEL_PORT_RANGE", fmt.Sprintf("Choose a port from %d to %d. Ports below %d belong to system services such as web, mail and DNS.", MinChoice, MaxChoice, MinChoice)
	}
	if name, ok := extraReserved[port]; ok {
		return "PANEL_PORT_RESERVED", fmt.Sprintf("Port %d is used by %s. Choose another port.", port, name)
	}
	if name, ok := reservedChoices[port]; ok {
		return "PANEL_PORT_RESERVED", fmt.Sprintf("Port %d is used by %s. Choose another port.", port, name)
	}
	return "", ""
}
