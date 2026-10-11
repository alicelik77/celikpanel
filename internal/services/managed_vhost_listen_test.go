package services

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// set11 cell A6 (2026-10-11): the probe asks the address the site's port-80
// block listens on, not always 127.0.0.1.

// bindSiteBlocks rewrites the kept file's port-80 listens (the owner's edit).
func bindSiteBlocks(t *testing.T, path, listens string) {
	t.Helper()
	text := readTestFile(t, path)
	bound := strings.ReplaceAll(text, "    listen 80;\n    listen [::]:80;\n", listens)
	if bound == text {
		t.Fatal("no port-80 listen to rewrite")
	}
	writeTestFile(t, path, bound, 0o644)
}

func TestTheProbeAsksTheAddressTheSiteListensOn(t *testing.T) {
	cases := []struct {
		name, listens, answerAt string
		want                    []string
	}{
		{"wildcard", "    listen 80;\n    listen [::]:80;\n", "", []string{"127.0.0.1"}},
		{"one local address", "    listen 10.0.2.15:80 default_server;\n", "10.0.2.15", []string{"10.0.2.15"}},
		{"a local address and the IPv6 wildcard", "    listen 10.0.2.15:80;\n    listen [::]:80;\n", "", []string{"10.0.2.15", "::1"}},
		{"IPv6 address", "    listen [2001:db8::15]:80;\n", "2001:db8::15", []string{"2001:db8::15"}},
		{"IPv6 wildcard only", "    listen [::]:80 ipv6only=on;\n", "::1", []string{"::1"}},
		{"address without a port", "    listen 10.0.2.15;\n", "10.0.2.15", []string{"10.0.2.15"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, nginx := probeHarness(t)
			data := probeData(nginx, "probe-bound.example")
			h.apply(h.certItem(data, transport.SiteFileTriggerCreate))
			path := SiteVhostPath(data.Domain)
			if tc.name != "wildcard" {
				bindSiteBlocks(t, path, tc.listens)
			} else {
				ownerEdit(t, path)
			}
			edited := readTestFile(t, path)
			nginx.serve[data.Domain], nginx.serve["www."+data.Domain] = true, true
			nginx.answerAt = tc.answerAt
			result := h.apply(h.probeItem(data, transport.SiteFileTriggerChange))[0]
			if result.Outcome != transport.SiteFileOutcomeKept || result.Validation != transport.SiteFileValidationReady ||
				result.ValidationAddress != strings.Join(tc.want, ",") {
				t.Fatalf("ready on %v: %+v (asked %v)", tc.want, result, nginx.addresses)
			}
			var wantAsked []string
			for range []string{data.Domain, "www." + data.Domain} {
				wantAsked = append(wantAsked, tc.want...)
			}
			if strings.Join(nginx.addresses, ",") != strings.Join(wantAsked, ",") {
				t.Fatalf("asked on %v, want %v", nginx.addresses, wantAsked)
			}
			if readTestFile(t, path) != edited {
				t.Fatal("the owner's file was touched")
			}
			if left := challengeDirEntries(t, nginx.root); len(left) != 0 {
				t.Fatalf("the probe file was left: %v", left)
			}
		})
	}
}

func TestABoundSiteThatDoesNotServeTheProbeNamesTheAddressThatAnswered(t *testing.T) {
	h, nginx := probeHarness(t)
	data := probeData(nginx, "probe-bound-missing.example")
	h.apply(h.certItem(data, transport.SiteFileTriggerCreate))
	bindSiteBlocks(t, SiteVhostPath(data.Domain), "    listen 10.0.2.15:80;\n")
	nginx.serve["www."+data.Domain] = true
	result := h.apply(h.probeItem(data, transport.SiteFileTriggerChange))[0]
	if result.Validation != transport.SiteFileValidationIncludeMissing || result.ValidationName != data.Domain ||
		result.ValidationStatus != http.StatusNotFound || result.ValidationAddress != "10.0.2.15" {
		t.Fatalf("bound, not served: %+v", result)
	}
}

func TestAnAddressThatIsNotThisServersIsUnknownAndNothingIsAsked(t *testing.T) {
	h, nginx := probeHarness(t)
	data := probeData(nginx, "probe-remote.example")
	h.apply(h.certItem(data, transport.SiteFileTriggerCreate))
	bindSiteBlocks(t, SiteVhostPath(data.Domain), "    listen 192.0.2.7:80;\n")
	nginx.serve[data.Domain], nginx.serve["www."+data.Domain] = true, true
	result := h.apply(h.probeItem(data, transport.SiteFileTriggerChange))[0]
	if result.Validation != transport.SiteFileValidationUnknown ||
		result.ValidationUnknownReason != transport.SiteFileValidationUnknownAddressNotLocal ||
		result.ValidationAddress != "192.0.2.7" || result.ValidationName != data.Domain || result.ValidationStatus != 0 {
		t.Fatalf("not local: %+v", result)
	}
	if len(nginx.asked) != 0 {
		t.Fatalf("asked although the address is not this server's: %v", nginx.asked)
	}
	if left := challengeDirEntries(t, nginx.root); len(left) != 0 {
		t.Fatalf("a probe file was written: %v", left)
	}
	// The addresses cannot be listed: unknown, but not "not local".
	h.ng.hostAddresses = func() (map[string]bool, error) { return nil, errors.New("netlink: permission denied") }
	result = h.apply(h.probeItem(data, transport.SiteFileTriggerChange))[0]
	if result.Validation != transport.SiteFileValidationUnknown || result.ValidationUnknownReason != "" ||
		!strings.Contains(result.ValidationDetail, "could not be listed") {
		t.Fatalf("addresses not listed: %+v", result)
	}
}

func TestAFileWithoutAPort80BlockIsAskedOnLoopbackAsBefore(t *testing.T) {
	h, nginx := probeHarness(t)
	data := probeData(nginx, "probe-tls-only.example")
	withMail := data
	withMail.ACMEChallengeNames = []string{"mail.probe-tls-only.example"}
	writeTestFile(t, SiteVhostPath(data.Domain),
		"server {\n    listen 10.0.2.15:443 ssl;\n    server_name probe-tls-only.example www.probe-tls-only.example;\n}\n", 0o644)
	result := h.apply(h.probeItem(withMail, transport.SiteFileTriggerChange))[0]
	if result.Validation != transport.SiteFileValidationIncludeMissing || result.ValidationAddress != "127.0.0.1" ||
		result.ValidationStatus != http.StatusNotFound {
		t.Fatalf("no port-80 block: %+v", result)
	}
	// The site's names served (by another file), mail. not: names_missing.
	nginx.addresses = nil
	nginx.serve[data.Domain], nginx.serve["www."+data.Domain] = true, true
	result = h.apply(h.probeItem(withMail, transport.SiteFileTriggerChange))[0]
	if result.Validation != transport.SiteFileValidationNamesMissing || result.ValidationName != "mail.probe-tls-only.example" ||
		result.ValidationAddress != "127.0.0.1" {
		t.Fatalf("mail not served: %+v", result)
	}
	for _, address := range nginx.addresses {
		if address != "127.0.0.1" {
			t.Fatalf("asked on %v", nginx.addresses)
		}
	}
}

func TestTheListenReaderIsConservative(t *testing.T) {
	text := `
upstream app { server 10.9.9.9:80; }
# server { listen 203.0.113.1:80; server_name commented.example; }
server {
    listen 8080;
    listen unix:/run/site.sock;
    listen 443 ssl;
    server_name other-port.example;
}
server {
    listen "10.0.2.15:80" default_server;   # quoted
    server_name Bound.Example;
    location / { listen 203.0.113.9:80; }
}
server {
    server_name nolisten.example;
}
server {
    listen *:80;
    listen [::]:80;
    server_name ~^regex\.example$ *.wild.example;
}
`
	got := probeListensByName(text, []string{"bound.example", "nolisten.example", "other-port.example", "commented.example", "x.wild.example"})
	if listens := got["bound.example"]; len(listens) != 1 || listens[0].address != "10.0.2.15" {
		t.Fatalf("bound: %+v", listens)
	}
	if listens := got["nolisten.example"]; len(listens) != 1 || !listens[0].wildcardV4 {
		t.Fatalf("no listen: %+v", listens)
	}
	for _, name := range []string{"other-port.example", "commented.example", "x.wild.example"} {
		if listens, ok := got[name]; ok {
			t.Fatalf("%s: %+v", name, listens)
		}
	}
	local := func(address string) bool { return address == "10.0.2.15" }
	if targets := probeTargetsFor(nil, local); len(targets) != 1 || targets[0].address != "127.0.0.1" {
		t.Fatalf("no listens: %+v", targets)
	}
	targets := probeTargetsFor([]probeListen{{address: "198.51.100.4"}, {wildcardV4: true}, {wildcardV6: true}}, local)
	if len(targets) != 2 || targets[0].address != "127.0.0.1" || targets[1].address != "198.51.100.4" || targets[1].local {
		t.Fatalf("mixed: %+v", targets)
	}
	for value, want := range map[string]string{
		"80": "v4", "*:80": "v4", "0.0.0.0:80": "v4", "[::]:80": "v6", "10.0.2.15": "10.0.2.15",
		"[2001:DB8::15]:80": "2001:db8::15", "host.example:80": "host.example",
	} {
		listen, ok := parsePort80Listen([]string{value})
		kind := listen.address
		if listen.wildcardV4 {
			kind = "v4"
		} else if listen.wildcardV6 {
			kind = "v6"
		}
		if !ok || kind != want {
			t.Fatalf("%s: %+v %v", value, listen, ok)
		}
	}
	for _, value := range []string{"8080", "10.0.2.15:443", "[::]:8443", "unix:/run/x.sock", "[::1]x"} {
		if listen, ok := parsePort80Listen([]string{value}); ok {
			t.Fatalf("%s counted as port 80: %+v", value, listen)
		}
	}
}
