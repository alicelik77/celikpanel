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

// NFTUnit renders the boot unit. It runs only nft, before the network comes
// up, so the filter is in place before the engine can be reached, with or
// without CelikPanel.
func NFTUnit(nftProgram string) (string, error) {
	if !filepath.IsAbs(nftProgram) || strings.ContainsAny(nftProgram, " \t\n\"'\\$`;") {
		return "", errors.New("the nft program path must be absolute and plain")
	}
	return "# CelikPanel: remote database access (docs/DATABASE-REMOTE-ACCESS.md).\n" +
		"# Loads the address filter of the database ports at boot. Removed with the\n" +
		"# last remote access rule.\n" +
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
		"RemainAfterExit=yes\n" +
		"ExecStart=" + nftProgram + " -f " + NFTPath + "\n" +
		"\n" +
		"[Install]\n" +
		"WantedBy=sysinit.target\n", nil
}
