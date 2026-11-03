//go:build linux

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// fakeSuspensionPostfix stands in for postconf and the verified reload: it
// holds smtpd_recipient_restrictions and counts what was asked.
type fakeSuspensionPostfix struct {
	restrictions string
	set          bool
	writes       []string
	reloads      int
	reloadErr    error
	writeErr     error
}

func configureMailSuspensionTest(t *testing.T) (users, domains string, postmaps *[]string, postfix *fakeSuspensionPostfix) {
	t.Helper()
	dir := configureMailMutationTest(t)
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

	postfix = &fakeSuspensionPostfix{restrictions: "permit_mynetworks, permit_sasl_authenticated, reject_unauth_destination", set: true}
	oldPath, oldType := postfixSuspendedRecipientsPath, suspensionMapType
	oldLook, oldPostconf, oldReload := mailPolicyLookPath, mailPolicyPostconf, mailPolicyReload
	postfixSuspendedRecipientsPath = filepath.Join(dir, "suspended")
	suspensionMapType = func() string { return "hash" }
	mailPolicyLookPath = func(string) (string, error) { return "/usr/sbin/postconf", nil }
	mailPolicyPostconf = func(args ...string) ([]byte, error) {
		switch {
		case len(args) == 2 && args[0] == "-h" && args[1] == suspensionRestrictionsParam:
			return []byte(postfix.restrictions + "\n"), nil
		case len(args) == 2 && args[0] == "-e":
			if postfix.writeErr != nil {
				return nil, postfix.writeErr
			}
			postfix.writes = append(postfix.writes, args[1])
			postfix.restrictions, postfix.set = strings.TrimPrefix(args[1], suspensionRestrictionsParam+"="), true
			return nil, nil
		case len(args) == 2 && args[0] == "-X":
			postfix.writes = append(postfix.writes, "-X "+args[1])
			postfix.restrictions, postfix.set = "", false
			return nil, nil
		}
		t.Fatalf("unexpected postconf %v", args)
		return nil, nil
	}
	mailPolicyReload = func() (string, error) {
		postfix.reloads++
		return mailServiceReloaded, postfix.reloadErr
	}
	t.Cleanup(func() {
		postfixSuspendedRecipientsPath, suspensionMapType = oldPath, oldType
		mailPolicyLookPath, mailPolicyPostconf, mailPolicyReload = oldLook, oldPostconf, oldReload
	})
	return users, domains, postmaps, postfix
}

func mailTestFileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
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
	users, domains, postmaps, postfix := configureMailSuspensionTest(t)
	owner := postfix.restrictions
	before := inspectMailSuspension(t)
	if !before.DomainListed || before.DomainLine != "example.com OK" || before.MailDeferred || len(before.LoginRefused) != 2 || before.LoginRefused["a@example.com"] {
		t.Fatalf("before = %+v", before)
	}

	first := setMailSuspension(t, true, before)
	if !first.Applied || !first.Changed || first.Conflict || first.Error != "" || !first.State.MailDeferred || !first.State.DomainListed {
		t.Fatalf("suspend = %+v", first)
	}
	// The domain stays hosted; its mail gets a temporary 450, so senders retry.
	if got := readMailTestFile(t, postfixDomainsPath); got != domains {
		t.Fatalf("domains after suspend = %q", got)
	}
	if got := readMailTestFile(t, postfixSuspendedRecipientsPath); got != "example.com 450 4.7.1 Mailbox temporarily unavailable\n" {
		t.Fatalf("suspended map = %q", got)
	}
	prefix := "check_recipient_access hash:" + postfixSuspendedRecipientsPath
	if postfix.restrictions != prefix+", "+owner || postfix.reloads != 1 {
		t.Fatalf("main.cf after suspend = %q, reloads %d", postfix.restrictions, postfix.reloads)
	}
	suspendedUsers := readMailTestFile(t, dovecotUsersPath)
	if strings.Count(suspendedUsers, "::::::nologin=y userdb_quota_rule=") != 2 || strings.Contains(suspendedUsers, "c@other.com:"+mailPasswordTestHash("1234567890abcdef", 'A')+"::::::nologin") {
		t.Fatalf("users after suspend = %q", suspendedUsers)
	}

	second := setMailSuspension(t, true, before)
	if !second.Applied || second.Changed {
		t.Fatalf("second suspend = %+v", second)
	}
	if readMailTestFile(t, dovecotUsersPath) != suspendedUsers || len(postfix.writes) != 1 {
		t.Fatal("second suspend changed the files or main.cf")
	}

	restored := setMailSuspension(t, false, before)
	if !restored.Applied || !restored.Changed || restored.State.MailDeferred || !restored.State.DomainListed {
		t.Fatalf("restore = %+v", restored)
	}
	if got := readMailTestFile(t, dovecotUsersPath); got != users {
		t.Fatalf("users after restore = %q, want %q", got, users)
	}
	if got := readMailTestFile(t, postfixDomainsPath); got != domains {
		t.Fatalf("domains after restore = %q", got)
	}
	if got := readMailTestFile(t, postfixSuspendedRecipientsPath); got != "" {
		t.Fatalf("suspended map after restore = %q", got)
	}
	// Exact restore: main.cf holds the owner's value again.
	if postfix.restrictions != owner || !postfix.set {
		t.Fatalf("main.cf after restore = %q", postfix.restrictions)
	}
	again := setMailSuspension(t, false, before)
	if !again.Applied || again.Changed {
		t.Fatalf("second restore = %+v", again)
	}
	for _, path := range *postmaps {
		if path != postfixSuspendedRecipientsPath {
			t.Fatalf("postmap ran on %s", path)
		}
	}
	if len(*postmaps) != 4 {
		t.Fatalf("postmap calls = %v", *postmaps)
	}
}

func TestMailDomainSuspensionEmptyRestrictionsAreUnsetAgain(t *testing.T) {
	_, _, _, postfix := configureMailSuspensionTest(t)
	postfix.restrictions, postfix.set = "", false
	before := inspectMailSuspension(t)
	if r := setMailSuspension(t, true, before); !r.Applied || postfix.restrictions != "check_recipient_access hash:"+postfixSuspendedRecipientsPath {
		t.Fatalf("suspend = %+v, main.cf %q", r, postfix.restrictions)
	}
	if r := setMailSuspension(t, false, before); !r.Applied || postfix.set || postfix.restrictions != "" {
		t.Fatalf("restore = %+v, main.cf %q set=%v", r, postfix.restrictions, postfix.set)
	}
}

func TestMailDomainSuspensionConflictLeavesFilesIdentical(t *testing.T) {
	users, _, postmaps, postfix := configureMailSuspensionTest(t)
	before := inspectMailSuspension(t)
	// Someone wrote another answer for the domain after inspection.
	writeMailTestFile(t, postfixSuspendedRecipientsPath, "example.com REJECT\n", 0o644)
	response := setMailSuspension(t, true, before)
	if !response.Conflict || response.Applied || response.Changed || !strings.Contains(response.Error, "example.com REJECT") {
		t.Fatalf("conflict = %+v", response)
	}
	if readMailTestFile(t, dovecotUsersPath) != users || readMailTestFile(t, postfixSuspendedRecipientsPath) != "example.com REJECT\n" {
		t.Fatal("files changed on conflict")
	}
	if len(*postmaps) != 0 || len(postfix.writes) != 0 || postfix.reloads != 0 {
		t.Fatalf("postmap/postconf ran on conflict: %v %v", *postmaps, postfix.writes)
	}
}

func TestMailDomainSuspensionRollsBackOnPostmapFailure(t *testing.T) {
	users, _, _, postfix := configureMailSuspensionTest(t)
	before := inspectMailSuspension(t)
	mailMutationPostmap = func(context.Context, string) error { return errors.New("postmap failed") }
	response := setMailSuspension(t, true, before)
	if response.Applied || response.Changed || response.Error == "" {
		t.Fatalf("postmap failure = %+v", response)
	}
	if readMailTestFile(t, dovecotUsersPath) != users || mailTestFileExists(postfixSuspendedRecipientsPath) {
		t.Fatal("files not rolled back")
	}
	if response.State.MailDeferred || response.State.LoginRefused["a@example.com"] || len(postfix.writes) != 0 {
		t.Fatalf("state after rollback = %+v, main.cf writes %v", response.State, postfix.writes)
	}
}

func TestMailDomainSuspensionRollsBackWhenMainCfCannotBeWritten(t *testing.T) {
	users, _, _, postfix := configureMailSuspensionTest(t)
	before := inspectMailSuspension(t)
	postfix.writeErr = errors.New("postconf: fatal: open /etc/postfix/main.cf: read-only file system")
	response := setMailSuspension(t, true, before)
	if response.Applied || response.Changed || !strings.Contains(response.Error, "put back") {
		t.Fatalf("postconf failure = %+v", response)
	}
	if readMailTestFile(t, dovecotUsersPath) != users || mailTestFileExists(postfixSuspendedRecipientsPath) || postfix.reloads != 0 {
		t.Fatal("files not rolled back after a failed main.cf write")
	}
}

func TestMailDomainSuspensionReloadFailureIsNotApplied(t *testing.T) {
	_, _, _, postfix := configureMailSuspensionTest(t)
	before := inspectMailSuspension(t)
	postfix.reloadErr = errors.New("postfix check failed")
	response := setMailSuspension(t, true, before)
	if response.Applied || !strings.Contains(response.Error, "could not be reloaded") {
		t.Fatalf("reload failure = %+v", response)
	}
	// A retry reloads again even though nothing is left to write.
	postfix.reloadErr = nil
	retry := setMailSuspension(t, true, before)
	if !retry.Applied || retry.Changed || postfix.reloads != 2 {
		t.Fatalf("retry = %+v, reloads %d", retry, postfix.reloads)
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
	mailPolicyMu.Lock()
	response = setMailSuspension(t, true, before)
	mailPolicyMu.Unlock()
	if response.Error != transport.MailConfigurationBusy || response.Applied {
		t.Fatalf("busy behind the mail policy = %+v", response)
	}
}
