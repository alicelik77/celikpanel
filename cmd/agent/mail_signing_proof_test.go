package main

import (
	"strings"
	"testing"
)

// Mail signed only for the sender's own domain (20 Nov 2026, tenant
// isolation, lane S3 part 3). This test uses only what existed before the
// change, so it runs unchanged on the earlier code and fails there.

// Path a: over submission the key is chosen by the SASL login, not by the
// From: header, on every layout CelikPanel writes.
func TestMailSigningKeyFollowsTheLoginNotTheFromHeader(t *testing.T) {
	for _, family := range []string{"apt", "pacman"} {
		layout, ok := opendkimLayoutFor(family)
		if !ok {
			t.Fatalf("%s: no layout", family)
		}
		conf := renderOpenDKIMConf(layout)
		if !strings.Contains(conf, "\nSenderMacro\t\t{auth_authen}\n") {
			t.Errorf("%s: OpenDKIM picks the signing key from the From: header:\n%s", family, conf)
		}
	}
}
