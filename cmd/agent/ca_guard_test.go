package main

import "os"

// blockRealCertificateAuthority keeps every test of this package away from a
// real CA: an ACME client started by a test (a local certbot, a Go client)
// is sent to a closed local proxy, so it cannot reach Let's Encrypt or any
// other outside address. Loopback test servers are not proxied.
func blockRealCertificateAuthority() {
	for _, name := range []string{"HTTPS_PROXY", "HTTP_PROXY", "https_proxy", "http_proxy", "ALL_PROXY", "all_proxy"} {
		_ = os.Setenv(name, "http://127.0.0.1:9")
	}
	for _, name := range []string{"NO_PROXY", "no_proxy"} {
		_ = os.Setenv(name, "localhost,127.0.0.1,::1")
	}
}
