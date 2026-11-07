package main

import (
	"context"
	"encoding/json"
	"net"
	"os/exec"
	"sort"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// The server's IP addresses (alpha.128, G14, read only): iproute2's JSON
// view, present on every supported system. Loopback and link-local
// addresses are left out. Nothing is changed; assigning a dedicated
// address to a site is a design note (docs/A128-DATA-ACCESS-DESIGN.md).
//
// Sunucunun IP adresleri, salt okunur.

var ipAddrShow = func(ctx context.Context) ([]byte, error) {
	return exec.CommandContext(ctx, "ip", "-j", "addr", "show").Output()
}

type ipAddrInterface struct {
	IfName   string `json:"ifname"`
	AddrInfo []struct {
		Family    string `json:"family"`
		Local     string `json:"local"`
		PrefixLen int    `json:"prefixlen"`
		Scope     string `json:"scope"`
	} `json:"addr_info"`
}

// parseIPAddrJSON keeps global addresses only.
func parseIPAddrJSON(raw []byte) ([]transport.ServerIPAddress, error) {
	var interfaces []ipAddrInterface
	if err := json.Unmarshal(raw, &interfaces); err != nil {
		return nil, err
	}
	out := []transport.ServerIPAddress{}
	for _, iface := range interfaces {
		for _, info := range iface.AddrInfo {
			ip := net.ParseIP(info.Local)
			if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() || info.Scope == "host" || info.Scope == "link" {
				continue
			}
			family := "ipv4"
			if ip.To4() == nil {
				family = "ipv6"
			}
			out = append(out, transport.ServerIPAddress{
				Address: ip.String(), Prefix: info.PrefixLen, Interface: iface.IfName, Family: family,
				Private: ip.IsPrivate(),
			})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Family != out[j].Family {
			return out[i].Family < out[j].Family
		}
		if out[i].Private != out[j].Private {
			return !out[i].Private
		}
		return false
	})
	return out, nil
}

// ServerIPAddressesV1 lists the server's addresses.
func (a *Agent) ServerIPAddressesV1(_ transport.Empty, resp *transport.ServerIPAddressesResponse) error {
	*resp = transport.ServerIPAddressesResponse{Addresses: []transport.ServerIPAddress{}}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	raw, err := ipAddrShow(ctx)
	if err != nil {
		resp.Error = "the addresses could not be read (ip -j addr show)"
		return nil
	}
	addresses, err := parseIPAddrJSON(raw)
	if err != nil {
		resp.Error = "the address list could not be read"
		return nil
	}
	resp.Addresses = addresses
	return nil
}
