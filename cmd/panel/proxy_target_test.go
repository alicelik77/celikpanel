package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
)

func fakeProxyLookup(records map[string][]string) func(context.Context, string) ([]net.IPAddr, error) {
	return func(_ context.Context, host string) ([]net.IPAddr, error) {
		values, ok := records[host]
		if !ok {
			return nil, errors.New("no such host")
		}
		var out []net.IPAddr
		for _, value := range values {
			out = append(out, net.IPAddr{IP: net.ParseIP(value)})
		}
		return out, nil
	}
}

func TestProxyTargetRule(t *testing.T) {
	lookup := fakeProxyLookup(map[string][]string{
		"app.example.com":      {"203.0.114.10"},
		"dual.example.com":     {"203.0.114.10", "2001:db9::10"},
		"internal.example.com": {"10.0.0.5"},
		"mixed.example.com":    {"203.0.114.10", "127.0.0.1"},
		"meta.example.com":     {"169.254.169.254"},
		"mapped.example.com":   {"::ffff:192.168.1.1"},
		"self.example.com":     {"198.51.101.7"},
		"empty.example.com":    {},
	})
	own := []netip.Addr{netip.MustParseAddr("198.51.101.7"), netip.MustParseAddr("2001:db9::7")}

	cases := []struct {
		target string
		reason string // "" allowed
	}{
		{"https://app.example.com", ""},
		{"http://app.example.com:8080/base/", ""},
		{"https://APP.example.com./", ""},
		{"https://dual.example.com", ""},
		{"http://203.0.114.10:3000", ""},
		{"http://[2001:db9::10]:8080", ""},

		{"", proxyTargetReasonInvalidURL},
		{" https://app.example.com", proxyTargetReasonInvalidURL},
		{"ftp://app.example.com", proxyTargetReasonInvalidURL},
		{"file:///etc/passwd", proxyTargetReasonInvalidURL},
		{"app.example.com:8080", proxyTargetReasonInvalidURL},
		{"http://user:pass@app.example.com", proxyTargetReasonInvalidURL},
		{"http://user@app.example.com", proxyTargetReasonInvalidURL},
		{"http://app.example.com\\@127.0.0.1", proxyTargetReasonInvalidURL},
		{"http:///path", proxyTargetReasonInvalidURL},
		{"http://app.example.com/#x", proxyTargetReasonInvalidURL},
		{"http://127.1", proxyTargetReasonInvalidURL},
		{"http://2130706433", proxyTargetReasonInvalidURL},
		{"http://0x7f.0x0.0x0.0x1", proxyTargetReasonInvalidURL},
		{"http://0177.0.0.1", proxyTargetReasonInvalidURL},
		{"http://under_score.example.com", proxyTargetReasonInvalidURL},

		{"http://127.0.0.1:8443", proxyTargetReasonInternal},
		{"http://127.5.6.7", proxyTargetReasonInternal},
		{"http://localhost:3000", proxyTargetReasonInternal},
		{"http://api.localhost", proxyTargetReasonInternal},
		{"http://0.0.0.0:22", proxyTargetReasonInternal},
		{"http://10.1.2.3", proxyTargetReasonInternal},
		{"http://172.16.0.1", proxyTargetReasonInternal},
		{"http://172.31.255.255", proxyTargetReasonInternal},
		{"http://192.168.0.1", proxyTargetReasonInternal},
		{"http://169.254.169.254/latest/meta-data/", proxyTargetReasonInternal},
		{"http://100.64.0.1", proxyTargetReasonInternal},
		{"http://224.0.0.1", proxyTargetReasonInternal},
		{"http://255.255.255.255", proxyTargetReasonInternal},
		{"http://[::1]:8080", proxyTargetReasonInternal},
		{"http://[::]", proxyTargetReasonInternal},
		{"http://[fc00::1]", proxyTargetReasonInternal},
		{"http://[fd12:3456::1]", proxyTargetReasonInternal},
		{"http://[fe80::1]", proxyTargetReasonInternal},
		{"http://[fe80::1%25eth0]", proxyTargetReasonInternal},
		{"http://[ff02::1]", proxyTargetReasonInternal},
		{"http://[::ffff:127.0.0.1]", proxyTargetReasonInternal},
		{"http://[::ffff:169.254.169.254]", proxyTargetReasonInternal},
		{"http://[::ffff:7f00:1]", proxyTargetReasonInternal},
		{"http://[::127.0.0.1]", proxyTargetReasonInternal},
		{"http://[64:ff9b::10.0.0.1]", proxyTargetReasonInternal},
		{"http://internal.example.com", proxyTargetReasonInternal},
		{"http://mixed.example.com", proxyTargetReasonInternal},
		{"http://meta.example.com", proxyTargetReasonInternal},
		{"http://mapped.example.com", proxyTargetReasonInternal},

		{"http://198.51.101.7:1923", proxyTargetReasonThisServer},
		{"http://[::ffff:198.51.101.7]", proxyTargetReasonThisServer},
		{"http://[2001:db9::7]:8080", proxyTargetReasonThisServer},
		{"https://self.example.com", proxyTargetReasonThisServer},

		{"https://missing.example.com", proxyTargetReasonUnresolved},
		{"https://empty.example.com", proxyTargetReasonUnresolved},
	}
	for _, tc := range cases {
		refusal := checkProxyTarget(context.Background(), tc.target, lookup, own)
		got := ""
		if refusal != nil {
			got = refusal.Reason
		}
		if got != tc.reason {
			t.Errorf("%q: reason %q, want %q", tc.target, got, tc.reason)
		}
	}
}

func TestProxyTargetPublicNAT64IsAllowed(t *testing.T) {
	if refusal := checkProxyTarget(context.Background(), "http://[64:ff9b::203.0.114.10]", fakeProxyLookup(nil), nil); refusal != nil {
		t.Fatalf("public NAT64 refused: %s", refusal.Reason)
	}
}

func TestProxyTargetRefusalMessagesSayWhatToDo(t *testing.T) {
	for _, reason := range []string{proxyTargetReasonInvalidURL, proxyTargetReasonInternal, proxyTargetReasonThisServer, proxyTargetReasonUnresolved} {
		message := (&proxyTargetRefusal{Reason: reason, Host: "h.example"}).Message()
		if !strings.Contains(message, "Nothing was changed.") {
			t.Errorf("%s: %q", reason, message)
		}
	}
}

func withProxyTargetSeams(t *testing.T, records map[string][]string, interfaces []string) {
	t.Helper()
	oldLookup, oldInterfaces := proxyTargetLookup, proxyTargetInterfaceAddrs
	proxyTargetLookup = fakeProxyLookup(records)
	proxyTargetInterfaceAddrs = func() ([]net.Addr, error) {
		var out []net.Addr
		for _, value := range interfaces {
			out = append(out, &net.IPNet{IP: net.ParseIP(value), Mask: net.CIDRMask(24, 32)})
		}
		return out, nil
	}
	t.Cleanup(func() { proxyTargetLookup, proxyTargetInterfaceAddrs = oldLookup, oldInterfaces })
}

func TestProxyHostingRefusesInternalTargetsForEveryCaller(t *testing.T) {
	panel, _, _, adminID := newServiceStateAtomicPanel(t)
	seedPHPSite(t, panel, adminID, "8.2")
	withProxyTargetSeams(t, map[string][]string{"rebind.example.com": {"192.168.10.4"}}, []string{"198.51.101.9"})

	cases := []struct {
		name    string
		request func(body string) *http.Request
		target  string
		reason  string
	}{
		{"customer loopback", func(b string) *http.Request {
			return customerRequest(http.MethodPut, "/api/v1/domains/9202/hosting", b, 77)
		}, "http://127.0.0.1:8443", proxyTargetReasonInternal},
		{"customer metadata", func(b string) *http.Request {
			return customerRequest(http.MethodPut, "/api/v1/domains/9202/hosting", b, 77)
		}, "http://169.254.169.254/", proxyTargetReasonInternal},
		{"customer name to private", func(b string) *http.Request {
			return customerRequest(http.MethodPut, "/api/v1/domains/9202/hosting", b, 77)
		}, "https://rebind.example.com", proxyTargetReasonInternal},
		{"customer this server", func(b string) *http.Request {
			return customerRequest(http.MethodPut, "/api/v1/domains/9202/hosting", b, 77)
		}, "http://198.51.101.9:1923", proxyTargetReasonThisServer},
		{"customer unresolved", func(b string) *http.Request {
			return customerRequest(http.MethodPut, "/api/v1/domains/9202/hosting", b, 77)
		}, "https://nowhere.example.com", proxyTargetReasonUnresolved},
		{"customer empty", func(b string) *http.Request {
			return customerRequest(http.MethodPut, "/api/v1/domains/9202/hosting", b, 77)
		}, "", proxyTargetReasonInvalidURL},
		{"administrator loopback", func(b string) *http.Request {
			return serviceStateAtomicRequest(http.MethodPut, "/api/v1/domains/9202/hosting", b, adminID)
		}, "http://[::ffff:127.0.0.1]:3306", proxyTargetReasonInternal},
	}
	for _, tc := range cases {
		body, _ := json.Marshal(hostingSettings{ProjectType: "proxy", ForwardTo: tc.target})
		recorder := httptest.NewRecorder()
		panel.handleUpdateHosting(recorder, tc.request(string(body)), 9202)
		var answer apiErrorBody
		_ = json.Unmarshal(recorder.Body.Bytes(), &answer)
		if recorder.Code != http.StatusBadRequest || answer.Code != errCodeProxyTargetNotAllowed || answer.Reason != tc.reason || answer.Error == "" {
			t.Errorf("%s: %d %s", tc.name, recorder.Code, recorder.Body.String())
		}
		var projectType string
		var forwardTo *string
		if err := panel.db.GetDB().QueryRow(`SELECT project_type, forward_to FROM sites WHERE id = 9203`).Scan(&projectType, &forwardTo); err != nil {
			t.Fatal(err)
		}
		if projectType != "php" || forwardTo != nil {
			t.Fatalf("%s: the refused change moved the site to %s %v", tc.name, projectType, forwardTo)
		}
	}
}

// A redirect ("forwarding") is answered to the visitor; the server never
// fetches it, so it is not held to the proxy rule.
func TestForwardingIsNotHeldToTheProxyRule(t *testing.T) {
	panel, _, _, adminID := newServiceStateAtomicPanel(t)
	seedPHPSite(t, panel, adminID, "8.2")
	withProxyTargetSeams(t, nil, nil)
	body, _ := json.Marshal(hostingSettings{ProjectType: "forwarding", ForwardTo: "http://127.0.0.1/"})
	recorder := httptest.NewRecorder()
	panel.handleUpdateHosting(recorder, customerRequest(http.MethodPut, "/api/v1/domains/9202/hosting", string(body), 77), 9202)
	if errorCode(t, recorder) == errCodeProxyTargetNotAllowed {
		t.Fatalf("forwarding refused by the proxy rule: %s", recorder.Body.String())
	}
}
