// Package dbremote holds the pure rules of remote database access
// (alpha.129, docs/DATABASE-REMOTE-ACCESS.md): which addresses may be
// listed, and the native text CelikPanel writes for them (a pg_hba block,
// MariaDB account statements and option file, an nftables file, a Fail2ban
// jail). It touches no file, runs no program and calls no Agent; the Agent
// and the panel share it so both judge an address the same way.
//
// Uzak veritabanı erişiminin saf kuralları: hangi adres listelenebilir ve
// CelikPanel'in yazdığı yerel metin. Dosyaya dokunmaz, program çalıştırmaz.
package dbremote

import (
	"fmt"
	"net/netip"
	"strings"
)

// Engines, as the panel and the Agent name them.
const (
	EngineMySQL      = "mysql"
	EnginePostgreSQL = "postgresql"
)

// Limits of one rule's address and of one user's list.
const (
	// MinIPv4Prefix: an IPv4 range is at most a /24 (256 addresses).
	MinIPv4Prefix = 24
	// MinIPv6Prefix: an IPv6 range is at most a /64 (one network).
	MinIPv6Prefix = 64
	// MaxAddressesPerUser: distinct addresses of one database user on one
	// engine (PostgreSQL: one cluster).
	MaxAddressesPerUser = 10
)

// AddressError is a refused address. Code is stable for the screen; Message
// is the plain sentence.
type AddressError struct {
	Code    string
	Message string
}

func (e *AddressError) Error() string { return e.Message }

// Codes of a refused address.
const (
	AddressInvalid    = "ADDRESS_INVALID"
	AddressTooWide    = "ADDRESS_TOO_WIDE"
	AddressLocal      = "ADDRESS_LOCAL"
	AddressSpecial    = "ADDRESS_SPECIAL"
	AddressIPv6Range  = "ADDRESS_IPV6_RANGE_MARIADB"
	AddressMappedIPv6 = "ADDRESS_MAPPED_IPV6"
)

func refuse(code, format string, args ...any) *AddressError {
	return &AddressError{Code: code, Message: fmt.Sprintf(format, args...)}
}

// ParseAddress reads one address the way a customer types it ("203.0.113.7",
// "198.51.100.0/24", "2001:db8::7", "2001:db8:1::/64") and answers it in
// canonical prefix form ("203.0.113.7/32"). A range with host bits set is
// moved to its network ("198.51.100.9/24" becomes "198.51.100.0/24"). The
// engine matters for IPv6 ranges: MariaDB matches an IPv6 host as text, so it
// takes single IPv6 addresses only.
//
// ParseAddress bir adresi müşterinin yazdığı gibi okur ve kanonik önek
// biçiminde döndürür.
func ParseAddress(engine, text string) (netip.Prefix, error) {
	text = strings.TrimSpace(text)
	if text == "" || len(text) > 64 || strings.ContainsAny(text, " \t\r\n%") {
		return netip.Prefix{}, refuse(AddressInvalid, "Write one IP address, for example 203.0.113.7, or a small range such as 203.0.113.0/24.")
	}
	var prefix netip.Prefix
	if strings.Contains(text, "/") {
		parsed, err := netip.ParsePrefix(text)
		if err != nil {
			return netip.Prefix{}, refuse(AddressInvalid, "%q is not an IP address or range. Write for example 203.0.113.7 or 203.0.113.0/24.", text)
		}
		prefix = parsed
	} else {
		addr, err := netip.ParseAddr(text)
		if err != nil {
			return netip.Prefix{}, refuse(AddressInvalid, "%q is not an IP address. Write for example 203.0.113.7 or 2001:db8::7.", text)
		}
		prefix = netip.PrefixFrom(addr, addr.BitLen())
	}
	addr := prefix.Addr()
	if addr.Zone() != "" {
		return netip.Prefix{}, refuse(AddressInvalid, "An address with an interface zone (%%) cannot be listed.")
	}
	if addr.Is4In6() {
		return netip.Prefix{}, refuse(AddressMappedIPv6, "Write the IPv4 address itself (%s) instead of its IPv6 form.", addr.Unmap())
	}
	prefix = prefix.Masked()
	addr = prefix.Addr()
	switch {
	case prefix.Bits() == 0:
		return netip.Prefix{}, refuse(AddressTooWide, "%s would let the whole internet try to sign in. List your own address instead.", prefix)
	case addr.Is4() && prefix.Bits() < MinIPv4Prefix:
		return netip.Prefix{}, refuse(AddressTooWide, "%s is too wide: an IPv4 range may hold at most 256 addresses (/24). List your own address or a /24 or smaller range.", prefix)
	case addr.Is6() && prefix.Bits() < MinIPv6Prefix:
		return netip.Prefix{}, refuse(AddressTooWide, "%s is too wide: an IPv6 range may be at most a /64. List your own address or network.", prefix)
	case addr.IsLoopback() || addr.IsUnspecified():
		return netip.Prefix{}, refuse(AddressLocal, "%s is this server itself; local connections are always allowed and need no rule.", prefix)
	case addr.IsMulticast() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() || addr.IsInterfaceLocalMulticast() || addr == netip.AddrFrom4([4]byte{255, 255, 255, 255}):
		return netip.Prefix{}, refuse(AddressSpecial, "%s is not an address a computer connects from. List the public address of your computer or office.", prefix)
	}
	if engine == EngineMySQL && addr.Is6() && prefix.Bits() != 128 {
		return netip.Prefix{}, refuse(AddressIPv6Range, "MariaDB takes single IPv6 addresses, not ranges. List each address on its own.")
	}
	return prefix, nil
}

// CanonicalAddress is ParseAddress' answer as text.
func CanonicalAddress(engine, text string) (string, error) {
	prefix, err := ParseAddress(engine, text)
	if err != nil {
		return "", err
	}
	return prefix.String(), nil
}

// DisplayAddress drops a single address' /32 or /128 for the screen.
func DisplayAddress(prefix netip.Prefix) string {
	if prefix.IsSingleIP() {
		return prefix.Addr().String()
	}
	return prefix.String()
}
