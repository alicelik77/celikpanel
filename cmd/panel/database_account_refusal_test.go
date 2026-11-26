package main

import (
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.149 security fix: an Agent refusal of an administrator account is
// one plain sentence of the panel's own - nothing was changed, the server's
// administrator acts - whatever text the Agent sent.
func TestADatabaseAccountRefusalIsOnePlainSentence(t *testing.T) {
	for _, code := range []string{transport.DatabaseAccountRefusedPrivileged, transport.DatabaseAccountRefusedMember, transport.DatabaseAccountRefusedUnreadable} {
		got := withDatabaseAccountRefusal(code, "boss", "psql: ERROR: celikpanel-refused:"+code)
		if !strings.Contains(got, "boss") || !strings.Contains(got, "Nothing was changed.") || !strings.Contains(got, "The server's administrator") ||
			!strings.HasSuffix(got, "("+code+").") || strings.Contains(got, "psql") {
			t.Errorf("%s: %q", code, got)
		}
	}
	for _, code := range []string{transport.DatabaseAccountRefusedAbsent, transport.DatabaseAccountRefusedSystemDatabase} {
		if got := withDatabaseAccountRefusal(code, "boss", "x"); !strings.Contains(got, "Nothing was changed") {
			t.Errorf("%s: %q", code, got)
		}
	}
	if got := withDatabaseAccountRefusal("", "boss", "the engine's own words"); got != "the engine's own words" {
		t.Fatalf("an answer without a refusal changed: %q", got)
	}
}
