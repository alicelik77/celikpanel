//go:build linux

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

func configureDomainOptionsTest(t *testing.T) (*fakeSuspensionPostfix, *[]string) {
	t.Helper()
	_, _, postmaps, postfix := configureMailSuspensionTest(t)
	old := postfixDomainOptionsPath
	postfixDomainOptionsPath = filepath.Join(filepath.Dir(postfixSuspendedRecipientsPath), "domain_options")
	t.Cleanup(func() { postfixDomainOptionsPath = old })
	return postfix, postmaps
}

func TestDomainOptionsEntriesCanOnlyRefuseOrPassOn(t *testing.T) {
	bad := []transport.MailDomainOptionsEntry{
		{Key: "example.com", Action: "OK"},
		{Key: "example.com", Action: "DUNNO"},
		{Key: "a@example.com", Action: "550 5.1.1 No such user"},
		{Key: "example.com", Action: "450 4.7.1 Later"},
		{Key: "example.com", Action: "550 5.1.1 a\nb"},
		{Key: "Example.com", Action: "550 5.7.1 Closed"},
		{Key: "example.com", Action: "REDIRECT x@evil.test"},
	}
	for _, entry := range bad {
		if validateDomainOptionsEntry(entry) == nil {
			t.Errorf("accepted %+v", entry)
		}
	}
	for _, entry := range []transport.MailDomainOptionsEntry{
		{Key: "example.com", Action: "550 5.7.1 This domain does not receive mail on this server"},
		{Key: "a@example.com", Action: "DUNNO"},
	} {
		if err := validateDomainOptionsEntry(entry); err != nil {
			t.Errorf("%+v: %v", entry, err)
		}
	}
}

func TestSetMailDomainOptionsWritesTheMapAndTheRestrictionAndTakesThemBack(t *testing.T) {
	postfix, postmaps := configureDomainOptionsTest(t)
	entries := []transport.MailDomainOptionsEntry{
		{Key: "example.com", Action: "550 5.1.1 Nobody by that name here, see example.com/contact"},
		{Key: "a@example.com", Action: "DUNNO"},
		{Key: "sendonly.test", Action: "550 5.7.1 This domain does not receive mail on this server"},
	}
	var resp transport.SetMailDomainOptionsResponse
	if err := (&Agent{}).SetMailDomainOptionsV1(&transport.SetMailDomainOptionsRequest{Entries: entries}, &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Applied || !resp.Changed {
		t.Fatalf("resp = %+v", resp)
	}
	raw, _ := os.ReadFile(postfixDomainOptionsPath)
	if !domainOptionsMapIsPanels(raw, true) || !strings.Contains(string(raw), "a@example.com DUNNO\nexample.com 550 5.1.1 Nobody") {
		t.Fatalf("map = %q", raw)
	}
	if !strings.HasPrefix(postfix.restrictions, "check_recipient_access hash:"+postfixDomainOptionsPath+", permit_mynetworks") ||
		postfix.reloads != 1 || len(*postmaps) == 0 {
		t.Fatalf("restrictions %q reloads %d postmaps %v", postfix.restrictions, postfix.reloads, *postmaps)
	}
	// The same state again changes nothing in main.cf.
	writes := len(postfix.writes)
	resp = transport.SetMailDomainOptionsResponse{}
	_ = (&Agent{}).SetMailDomainOptionsV1(&transport.SetMailDomainOptionsRequest{Entries: entries}, &resp)
	if !resp.Applied || resp.Changed || len(postfix.writes) != writes {
		t.Fatalf("second call changed something: %+v %v", resp, postfix.writes)
	}
	// No options left: the element goes and the owner's list is as before.
	resp = transport.SetMailDomainOptionsResponse{}
	_ = (&Agent{}).SetMailDomainOptionsV1(&transport.SetMailDomainOptionsRequest{}, &resp)
	if !resp.Applied || postfix.restrictions != "permit_mynetworks, permit_sasl_authenticated, reject_unauth_destination" {
		t.Fatalf("after clearing: %+v %q", resp, postfix.restrictions)
	}
}

func TestSetMailDomainOptionsKeepsTheOwnersMap(t *testing.T) {
	postfix, _ := configureDomainOptionsTest(t)
	writeMailTestFile(t, postfixDomainOptionsPath, "example.com REJECT mine\n", 0o644)
	var resp transport.SetMailDomainOptionsResponse
	_ = (&Agent{}).SetMailDomainOptionsV1(&transport.SetMailDomainOptionsRequest{Entries: []transport.MailDomainOptionsEntry{
		{Key: "example.com", Action: "550 5.7.1 Closed"},
	}}, &resp)
	if resp.Applied || !resp.Owner {
		t.Fatalf("resp = %+v", resp)
	}
	raw, _ := os.ReadFile(postfixDomainOptionsPath)
	if string(raw) != "example.com REJECT mine\n" || len(postfix.writes) != 0 || postfix.reloads != 0 {
		t.Fatalf("the owner's map or main.cf changed: %q %v", raw, postfix.writes)
	}
}
