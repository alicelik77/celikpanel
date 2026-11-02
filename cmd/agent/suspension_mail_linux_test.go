//go:build linux

package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

func configureMailSuspensionTest(t *testing.T) (users, domains string, postmaps *[]string) {
	t.Helper()
	configureMailMutationTest(t)
	hash := mailPasswordTestHash("1234567890abcdef", 'A')
	users = "a@example.com:" + hash + "::::::userdb_quota_rule=*:storage=100M\n" +
		"b@example.com:" + hash + "::::::userdb_quota_rule=*:storage=100M\n" +
		"c@other.com:" + hash + "::::::userdb_quota_rule=*:storage=100M\n"
	domains = "example.com OK\nother.com OK\n"
	writeMailTestFile(t, dovecotUsersPath, users, 0o640)
	writeMailTestFile(t, postfixDomainsPath, domains, 0o644)
	calls := []string{}
	postmaps = &calls
	mailMutationPostmap = func(_ context.Context, path string) error {
		*postmaps = append(*postmaps, path)
		return nil
	}
	return users, domains, postmaps
}

var mailSuspensionTestDomain = transport.SuspensionMailDomain{
	Domain: "example.com", Mailboxes: []string{"a@example.com", "b@example.com"},
}

func inspectMailSuspension(t *testing.T) transport.MailDomainState {
	t.Helper()
	var response transport.SuspensionInspectResponse
	if err := (&Agent{}).SuspensionInspectV1(&transport.SuspensionInspectRequest{
		MailDomains: []transport.SuspensionMailDomain{mailSuspensionTestDomain},
	}, &response); err != nil {
		t.Fatal(err)
	}
	if len(response.MailDomains) != 1 || response.MailDomains[0].Error != "" {
		t.Fatalf("inspect = %+v", response.MailDomains)
	}
	return response.MailDomains[0].State
}

func setMailSuspension(t *testing.T, suspend bool, before transport.MailDomainState) transport.SetMailDomainSuspensionResponse {
	t.Helper()
	var response transport.SetMailDomainSuspensionResponse
	if err := (&Agent{}).SetMailDomainSuspensionV1(&transport.SetMailDomainSuspensionRequest{
		Domain: mailSuspensionTestDomain, Suspend: suspend, Before: before,
	}, &response); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(response.Error, "$6$") {
		t.Fatalf("response discloses a password hash: %q", response.Error)
	}
	return response
}

func TestMailDomainSuspensionRoundTripOnFiles(t *testing.T) {
	users, domains, postmaps := configureMailSuspensionTest(t)
	before := inspectMailSuspension(t)
	if !before.DomainListed || before.DomainLine != "example.com OK" || len(before.LoginRefused) != 2 || before.LoginRefused["a@example.com"] {
		t.Fatalf("before = %+v", before)
	}

	first := setMailSuspension(t, true, before)
	if !first.Applied || !first.Changed || first.Conflict || first.Error != "" || first.State.DomainListed {
		t.Fatalf("suspend = %+v", first)
	}
	if got := readMailTestFile(t, postfixDomainsPath); got != "other.com OK\n" {
		t.Fatalf("domains after suspend = %q", got)
	}
	suspendedUsers := readMailTestFile(t, dovecotUsersPath)
	if strings.Count(suspendedUsers, "::::::nologin=y userdb_quota_rule=") != 2 || strings.Contains(suspendedUsers, "c@other.com:"+mailPasswordTestHash("1234567890abcdef", 'A')+"::::::nologin") {
		t.Fatalf("users after suspend = %q", suspendedUsers)
	}

	second := setMailSuspension(t, true, before)
	if !second.Applied || second.Changed {
		t.Fatalf("second suspend = %+v", second)
	}
	if readMailTestFile(t, dovecotUsersPath) != suspendedUsers || readMailTestFile(t, postfixDomainsPath) != "other.com OK\n" {
		t.Fatal("second suspend changed the files")
	}

	restored := setMailSuspension(t, false, before)
	if !restored.Applied || !restored.Changed || !restored.State.DomainListed {
		t.Fatalf("restore = %+v", restored)
	}
	if got := readMailTestFile(t, dovecotUsersPath); got != users {
		t.Fatalf("users after restore = %q, want %q", got, users)
	}
	if got := readMailTestFile(t, postfixDomainsPath); got != "other.com OK\nexample.com OK\n" {
		t.Fatalf("domains after restore = %q (original %q)", got, domains)
	}
	again := setMailSuspension(t, false, before)
	if !again.Applied || again.Changed {
		t.Fatalf("second restore = %+v", again)
	}
	for _, path := range *postmaps {
		if path != postfixDomainsPath {
			t.Fatalf("postmap ran on %s", path)
		}
	}
	if len(*postmaps) != 4 {
		t.Fatalf("postmap calls = %v", *postmaps)
	}
}

func TestMailDomainSuspensionConflictLeavesFilesIdentical(t *testing.T) {
	users, _, postmaps := configureMailSuspensionTest(t)
	before := inspectMailSuspension(t)
	// The owner changed the domain line after inspection.
	writeMailTestFile(t, postfixDomainsPath, "example.com REJECT\nother.com OK\n", 0o644)
	response := setMailSuspension(t, true, before)
	if !response.Conflict || response.Applied || response.Changed || !strings.Contains(response.Error, "example.com REJECT") {
		t.Fatalf("conflict = %+v", response)
	}
	if readMailTestFile(t, dovecotUsersPath) != users || readMailTestFile(t, postfixDomainsPath) != "example.com REJECT\nother.com OK\n" {
		t.Fatal("files changed on conflict")
	}
	if len(*postmaps) != 0 {
		t.Fatalf("postmap ran on conflict: %v", *postmaps)
	}
}

func TestMailDomainSuspensionRollsBackOnPostmapFailure(t *testing.T) {
	users, domains, _ := configureMailSuspensionTest(t)
	before := inspectMailSuspension(t)
	mailMutationPostmap = func(context.Context, string) error { return errors.New("postmap failed") }
	response := setMailSuspension(t, true, before)
	if response.Applied || response.Changed || response.Error == "" {
		t.Fatalf("postmap failure = %+v", response)
	}
	if readMailTestFile(t, dovecotUsersPath) != users || readMailTestFile(t, postfixDomainsPath) != domains {
		t.Fatal("files not rolled back")
	}
	if !response.State.DomainListed || response.State.LoginRefused["a@example.com"] {
		t.Fatalf("state after rollback = %+v", response.State)
	}
}

func TestMailDomainSuspensionDoesNotQueueBehindMailMutation(t *testing.T) {
	configureMailSuspensionTest(t)
	before := inspectMailSuspension(t)
	mailMutex.Lock()
	response := setMailSuspension(t, true, before)
	mailMutex.Unlock()
	if response.Error != transport.MailConfigurationBusy || response.Applied {
		t.Fatalf("busy = %+v", response)
	}
}
