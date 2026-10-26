package main

import (
	"os"
	"regexp"
	"strings"
)

// setupHostname is the machine's host name, read only to suggest a short
// server name. Tests replace it; nothing else may.
// Makinenin ana bilgisayar adi; yalniz kisa ad onermek icin okunur.
var setupHostname = os.Hostname

var (
	serverSetupHostnameProviderPrefix = regexp.MustCompile(`^(srv|vps|vmi?|ip|ec2|li|static|dedi|dedicated|instance|node|host|server|cloud|ubuntu|debian|arch|archlinux|fedora|centos|rocky|alma|almalinux|rhel|localhost|default|cp|s)([-_.]|[0-9])`)
	serverSetupHostnameDigitRun       = regexp.MustCompile(`[0-9]{4,}`)
	serverSetupHostnameHex            = regexp.MustCompile(`^[0-9a-f]{8,}$`)
)

// Single words a host name carries by default or that would collide with the
// names the wizard proposes itself (panel, mail, ns1…).
var serverSetupHostnameReserved = map[string]bool{
	"localhost": true, "localdomain": true, "ubuntu": true, "debian": true, "arch": true, "archlinux": true,
	"fedora": true, "centos": true, "rocky": true, "alma": true, "almalinux": true, "rhel": true, "linux": true,
	"server": true, "host": true, "vps": true, "vm": true, "node": true, "instance": true, "default": true,
	"cloud": true, "static": true, "machine": true, "srv": true, "box": true, "raspberrypi": true, "test": true, "example": true,
	"mail": true, "smtp": true, "imap": true, "www": true, "panel": true, "webmail": true, "ns": true, "ns1": true, "ns2": true,
}

// serverSetupShortNameSuggestion proposes a short server name from the
// machine's host name only when its first label reads like a name a person
// chose ("frankfurt", "boston", "web-1"); a provider-generated label
// (srv612345, vps-1a2b, ip-10-0-0-1, ubuntu-s-1vcpu-1gb-fra1-01), a
// distribution default or a digit- or hex-heavy label gives no suggestion.
// The panel never invents a name: an empty suggestion leaves the field empty
// and the screens say "this server".
// Kisa ad onerisi: ilk etiket insan secimi gibi gorunuyorsa; saglayici
// kaliplari, dagitim varsayilanlari ve rakam/hex agirlikli etiketler bos kalir.
func serverSetupShortNameSuggestion(host string) string {
	label := strings.ToLower(strings.TrimSpace(host))
	if index := strings.IndexByte(label, '.'); index >= 0 {
		label = label[:index]
	}
	if len(label) < 2 || len(label) > 24 || !serverSetupShortNamePattern.MatchString(label) {
		return ""
	}
	if serverSetupHostnameReserved[label] || serverSetupHostnameProviderPrefix.MatchString(label) ||
		serverSetupHostnameDigitRun.MatchString(label) || (serverSetupHostnameHex.MatchString(label) && strings.ContainsAny(label, "0123456789")) {
		return ""
	}
	if label[0] >= '0' && label[0] <= '9' {
		return ""
	}
	digits := 0
	for _, c := range label {
		if c >= '0' && c <= '9' {
			digits++
		}
	}
	if digits*3 > len(label) {
		return ""
	}
	return label
}

// currentServerSetupShortNameSuggestion reads the host name once per request.
func currentServerSetupShortNameSuggestion() string {
	host, err := setupHostname()
	if err != nil {
		return ""
	}
	return serverSetupShortNameSuggestion(host)
}
