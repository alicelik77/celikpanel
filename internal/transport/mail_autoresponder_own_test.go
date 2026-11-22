package transport

import (
	"strings"
	"testing"
)

// Review of alpha.143, finding 1 (21 Nov 2026): the out-of-office reply's
// From: is the additional address the incoming message named, and local mail
// is signed by its From:. Only the mailbox's own addresses may be named.
func TestForeignAutoresponderAddresses(t *testing.T) {
	forwardings := []MailForwarding{
		{Source: "info@a.test", Destination: "anna@a.test"},
		{Source: "sales@a2.test", Destination: "team@a.test"},
		{Source: "team@a.test", Destination: "anna@a.test, bob@a.test"},
		{Source: "ceo@b.test", Destination: "anna@a.test"}, // another account forwards to Anna
		{Source: "@a2.test", Destination: "anna@a.test"},   // catch-all of an own domain
		{Source: "loop@a.test", Destination: "loop2@a.test"},
		{Source: "loop2@a.test", Destination: "loop@a.test"},
	}
	mailboxes := map[string]bool{"anna@a.test": true, "bob@a.test": true, "carl@a2.test": true}
	isMailbox := func(address string) bool { return mailboxes[address] }
	cases := []struct {
		address string
		own     bool
	}{
		{"info@a.test", true},      // forwarded to the mailbox
		{"INFO@A.TEST", true},      // case does not matter
		{"sales@a2.test", true},    // through a second forwarding
		{"anything@a2.test", true}, // the own domain's catch-all
		{"carl@a2.test", false},    // a mailbox of its own, not caught
		{"bob@a.test", false},      // another mailbox
		{"ceo@b.test", false},      // another account's domain, even when forwarded here
		{"no-reply@owner.test", false},
		{"nobody@a.test", false}, // not delivered here
		{"loop@a.test", false},   // a loop never counts
	}
	for _, c := range cases {
		foreign := ForeignAutoresponderAddresses("anna@a.test", []string{c.address}, []string{"a2.test"}, forwardings, isMailbox)
		if got := len(foreign) == 0; got != c.own {
			t.Errorf("%s: own %v, want %v", c.address, got, c.own)
		}
	}
	// Without the account's other domains only the mailbox's domain counts.
	if foreign := ForeignAutoresponderAddresses("anna@a.test", []string{"sales@a2.test", "info@a.test"}, nil, forwardings, isMailbox); strings.Join(foreign, ",") != "sales@a2.test" {
		t.Fatalf("foreign without account domains: %v", foreign)
	}
}
