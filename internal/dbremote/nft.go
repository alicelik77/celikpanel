package dbremote

import (
	"errors"
	"fmt"
	"net/netip"
	"path/filepath"
	"sort"
	"strings"
)

// The address filter of remote database access: an nftables table of its
// own, at a priority before the panel firewall (celikpanel_fw, priority 0).
// A drop is final in nftables, so packets to a database port from an address
// that is not listed never reach the engine, whatever other filter exists or
// is missing. Its accept does not pass a later drop, which is why the panel
// firewall also opens the port (firewall_handlers.go).
//
// Uzak veritabanı erişiminin adres süzgeci: kendi nftables tablosu, panel
// güvenlik duvarından önce. Listede olmayan adresten gelen paket motora
// ulaşmaz.
const (
	NFTTable    = "celikpanel_dbremote"
	NFTPath     = "/etc/celikpanel/database-remote.nft"
	NFTUnitName = "celikpanel-database-remote-firewall.service"
	NFTUnitPath = "/etc/systemd/system/" + NFTUnitName
	nftPriority = -10
)

// NFTPort is one engine port and the addresses allowed to reach it. A port
// with no address is dropped for everyone except this server: that is the
// state between the last rule's removal and the engine's restart onto the
// local address.
type NFTPort struct {
	Port int
	V4   []netip.Prefix
	V6   []netip.Prefix
}

func joinPrefixes(list []netip.Prefix) string {
	parts := make([]string, len(list))
	for i, prefix := range list {
		parts[i] = DisplayAddress(prefix)
	}
	return strings.Join(parts, ", ")
}

// NFTFile renders the file the boot unit loads with `nft -f`. Loading it
// replaces the table as one transaction (declare, delete, define). No ports:
// "" — the table is removed instead.
func NFTFile(ports []NFTPort) (string, error) {
	if len(ports) == 0 {
		return "", nil
	}
	sorted := append([]NFTPort(nil), ports...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Port < sorted[j].Port })
	var b strings.Builder
	b.WriteString("#!/usr/sbin/nft -f\n")
	b.WriteString("# CelikPanel: remote database access (docs/DATABASE-REMOTE-ACCESS.md).\n")
	b.WriteString("# Only the listed addresses reach these database ports. Loaded at boot by\n")
	b.WriteString("# " + NFTUnitName + "; rewritten from CelikPanel.\n")
	b.WriteString("table inet " + NFTTable + "\n")
	b.WriteString("delete table inet " + NFTTable + "\n")
	b.WriteString("table inet " + NFTTable + " {\n")
	b.WriteString("  chain input {\n")
	b.WriteString(fmt.Sprintf("    type filter hook input priority %d; policy accept;\n", nftPriority))
	b.WriteString("    iif \"lo\" accept\n")
	seen := map[int]bool{}
	for _, port := range sorted {
		if port.Port < 1 || port.Port > 65535 || seen[port.Port] {
			return "", fmt.Errorf("invalid or repeated port %d", port.Port)
		}
		seen[port.Port] = true
		for _, prefix := range port.V4 {
			if !prefix.Addr().Is4() || prefix.Bits() < MinIPv4Prefix {
				return "", fmt.Errorf("refused IPv4 range %s", prefix)
			}
		}
		for _, prefix := range port.V6 {
			if !prefix.Addr().Is6() || prefix.Addr().Is4In6() || prefix.Bits() < MinIPv6Prefix {
				return "", fmt.Errorf("refused IPv6 range %s", prefix)
			}
		}
		if len(port.V4) > 0 {
			b.WriteString(fmt.Sprintf("    tcp dport %d ip saddr { %s } accept\n", port.Port, joinPrefixes(port.V4)))
		}
		if len(port.V6) > 0 {
			b.WriteString(fmt.Sprintf("    tcp dport %d ip6 saddr { %s } accept\n", port.Port, joinPrefixes(port.V6)))
		}
		b.WriteString(fmt.Sprintf("    tcp dport %d drop\n", port.Port))
	}
	b.WriteString("  }\n}\n")
	return b.String(), nil
}

// The guard (alpha.129 review L2, the pattern of the site sandbox's SMTP
// block, internal/sitesandbox/egress.go): `nft flush ruleset` or a restart of
// nftables.service drops the table; the guard loads the file again when the
// table or its drop rule is gone. The unit runs it at boot before the network
// and the timer every 30 seconds, with or without CelikPanel.
const (
	NFTGuardPath = "/etc/celikpanel/database-remote-guard.sh"
	NFTTimerName = "celikpanel-database-remote-firewall.timer"
	NFTTimerPath = "/etc/systemd/system/" + NFTTimerName
	// NFTLockPath is the lock the guard and the Agent share (re-review LOW 2).
	NFTLockPath       = "/run/celikpanel-database-remote.lock"
	nftGuardIntervalS = "30s"
)

func plainProgram(path string) error {
	if !filepath.IsAbs(path) || strings.ContainsAny(path, " \t\n\"'\\$`;") {
		return errors.New("the nft program path must be absolute and plain")
	}
	return nil
}

// NFTGuardScript renders the guard's shell text.
func NFTGuardScript(nftProgram string) (string, error) {
	if err := plainProgram(nftProgram); err != nil {
		return "", err
	}
	return "# CelikPanel: remote database access (docs/DATABASE-REMOTE-ACCESS.md).\n" +
		"# Run by " + NFTUnitName + " at boot and by " + NFTTimerName + " every\n" +
		"# " + nftGuardIntervalS + ": loads the address filter again when it is missing. Works\n" +
		"# without CelikPanel. Removed with the last remote access rule.\n" +
		"set -u\n" +
		"nft=" + nftProgram + "\n" +
		"# The Agent holds the same lock while it writes or removes the filter;\n" +
		"# once the rule file is gone (closing) the guard loads nothing.\n" +
		"exec 9>" + NFTLockPath + "\n" +
		"flock 9 || exit 1\n" +
		"[ -f " + NFTPath + " ] || exit 0\n" +
		"if ! \"$nft\" list chain inet " + NFTTable + " input 2>/dev/null | grep -q ' drop'; then\n" +
		"\techo \"celikpanel-dbremote: the database port filter was missing; loading " + NFTPath + " again\"\n" +
		"\t\"$nft\" -f " + NFTPath + " || { echo \"celikpanel-dbremote: nft could not load the filter\"; exit 1; }\n" +
		"fi\n", nil
}

// NFTUnit renders the guard's unit: before the network at boot, so the
// filter is in place before an engine can be reached, and on the timer.
func NFTUnit(nftProgram string) (string, error) {
	if err := plainProgram(nftProgram); err != nil {
		return "", err
	}
	return "# CelikPanel: remote database access (docs/DATABASE-REMOTE-ACCESS.md).\n" +
		"# Loads the address filter of the database ports at boot and keeps it\n" +
		"# loaded (" + NFTTimerName + "). Removed with the last remote access rule.\n" +
		"[Unit]\n" +
		"Description=CelikPanel remote database access filter (nftables)\n" +
		"DefaultDependencies=no\n" +
		"Wants=network-pre.target\n" +
		"Before=network-pre.target shutdown.target\n" +
		"Conflicts=shutdown.target\n" +
		"ConditionPathExists=" + NFTPath + "\n" +
		"\n" +
		"[Service]\n" +
		"Type=oneshot\n" +
		"ExecStart=/bin/sh " + NFTGuardPath + "\n" +
		"\n" +
		"[Install]\n" +
		"WantedBy=sysinit.target\n", nil
}

// NFTTimer renders the guard's timer.
func NFTTimer() string {
	return "# CelikPanel: remote database access (docs/DATABASE-REMOTE-ACCESS.md).\n" +
		"# Keeps the database port filter loaded. Removed with the last rule.\n" +
		"[Unit]\n" +
		"Description=CelikPanel remote database access filter guard\n" +
		"\n" +
		"[Timer]\n" +
		"OnBootSec=" + nftGuardIntervalS + "\n" +
		"OnUnitActiveSec=" + nftGuardIntervalS + "\n" +
		"AccuracySec=5s\n" +
		"Unit=" + NFTUnitName + "\n" +
		"\n" +
		"[Install]\n" +
		"WantedBy=timers.target\n"
}
