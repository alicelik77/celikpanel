package main

import "github.com/alicelik/celikpanel/internal/services"

// Tests never measure the machine's own nginx (a CI runner may have one):
// the webmail renders use the unknown-version form unless a test sets one.
// Testler makinenin nginx'ini ölçmez.
func init() {
	webmailAddressHTTP2Form = func() string { return services.NginxFeatures{}.HTTP2Form() }
}
