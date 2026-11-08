//go:build linux

package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

func casTestSetup(t *testing.T) (before, oldHash, newHash string) {
	t.Helper()
	configureMailMutationTest(t)
	previous := buildCommit
	buildCommit = "mail-recovery-test"
	t.Cleanup(func() { buildCommit = previous })
	oldHash = mailPasswordTestHash("1234567890abcdef", 'B')
	newHash = mailPasswordTestHash("fedcba0987654321", 'C')
	before = "# keep\n" +
		"user@example.com:" + oldHash + "::::::userdb_quota_rule=*:storage=250M\n" +
		"other@example.com:" + oldHash + "::::::nologin=y userdb_quota_rule=*:storage=50M\n"
	writeMailTestFile(t, dovecotUsersPath, before, 0o640)
	mailHashGenerator = func(_ context.Context, password string) (string, error) {
		if password != "New-Password-1!" {
			return "", errors.New("unexpected password")
		}
		return newHash, nil
	}
	return before, oldHash, newHash
}

func casFingerprint(t *testing.T, email string) transport.MailPasswordFingerprintResponse {
	t.Helper()
	var response transport.MailPasswordFingerprintResponse
	if err := (&Agent{}).MailPasswordFingerprintV1(&transport.MailPasswordFingerprintRequest{Email: email}, &response); err != nil {
		t.Fatalf("MailPasswordFingerprintV1: %v", err)
	}
	return response
}

func casSet(t *testing.T, email, fingerprint string) transport.SetMailPasswordIfUnchangedResponse {
	t.Helper()
	var response transport.SetMailPasswordIfUnchangedResponse
	if err := (&Agent{}).SetMailPasswordIfUnchangedV1(&transport.SetMailPasswordIfUnchangedRequest{
		ExpectedBuildCommit: "mail-recovery-test", Email: email,
		NewPassword: "New-Password-1!", ExpectedFingerprint: fingerprint,
	}, &response); err != nil {
		t.Fatalf("SetMailPasswordIfUnchangedV1: %v", err)
	}
	return response
}

func TestMailPasswordFingerprintNamesTheFieldWithoutRevealingIt(t *testing.T) {
	_, oldHash, _ := casTestSetup(t)
	got := casFingerprint(t, "User@Example.com")
	if !got.Found || got.LoginDisabled || got.Error != "" {
		t.Fatalf("fingerprint = %+v", got)
	}
	if got.Fingerprint != mailPasswordFingerprint(oldHash) || strings.Contains(got.Fingerprint, oldHash) {
		t.Fatalf("fingerprint %q is not the field's", got.Fingerprint)
	}
	if missing := casFingerprint(t, "nobody@example.com"); missing.Found || missing.Fingerprint != "" || missing.Error != "" {
		t.Fatalf("missing mailbox = %+v", missing)
	}
	if disabled := casFingerprint(t, "other@example.com"); !disabled.Found || !disabled.LoginDisabled {
		t.Fatalf("nologin mailbox = %+v", disabled)
	}
}

func TestSetMailPasswordIfUnchangedReplacesOnlyTheFieldOnce(t *testing.T) {
	before, oldHash, newHash := casTestSetup(t)
	fingerprint := casFingerprint(t, "user@example.com").Fingerprint
	if got := casSet(t, "user@example.com", fingerprint); !got.Applied || got.Changed || got.Error != "" {
		t.Fatalf("first reset = %+v", got)
	}
	want := strings.Replace(before, oldHash, newHash, 1)
	if got := readMailTestFile(t, dovecotUsersPath); got != want {
		t.Fatalf("users file\ngot:  %q\nwant: %q", got, want)
	}
	// The same fingerprint again (a replayed link) changes nothing.
	if got := casSet(t, "user@example.com", fingerprint); got.Applied || !got.Changed {
		t.Fatalf("replayed reset = %+v", got)
	}
	if got := readMailTestFile(t, dovecotUsersPath); got != want {
		t.Fatalf("replay changed the file: %q", got)
	}
}

func TestSetMailPasswordIfUnchangedRefusesAfterAnOwnerEdit(t *testing.T) {
	before, oldHash, _ := casTestSetup(t)
	fingerprint := casFingerprint(t, "user@example.com").Fingerprint
	edited := strings.Replace(before, oldHash, mailPasswordTestHash("aaaaaaaaaaaaaaaa", 'D'), 1)
	writeMailTestFile(t, dovecotUsersPath, edited, 0o640)
	if got := casSet(t, "user@example.com", fingerprint); got.Applied || !got.Changed {
		t.Fatalf("reset after an owner edit = %+v", got)
	}
	if got := readMailTestFile(t, dovecotUsersPath); got != edited {
		t.Fatalf("owner's edit was overwritten: %q", got)
	}
}

func TestSetMailPasswordIfUnchangedNeverOpensALoginDisabledLine(t *testing.T) {
	before, _, _ := casTestSetup(t)
	fingerprint := casFingerprint(t, "other@example.com").Fingerprint
	if got := casSet(t, "other@example.com", fingerprint); got.Applied || !got.LoginDisabled {
		t.Fatalf("reset of a nologin line = %+v", got)
	}
	if got := readMailTestFile(t, dovecotUsersPath); got != before {
		t.Fatalf("nologin line changed: %q", got)
	}
	if got := casSet(t, "gone@example.com", fingerprint); got.Applied || !got.Changed {
		t.Fatalf("reset of a missing mailbox = %+v", got)
	}
}

func TestSetMailPasswordIfUnchangedRefusesBadInputBeforeHashing(t *testing.T) {
	before, _, _ := casTestSetup(t)
	called := false
	mailHashGenerator = func(context.Context, string) (string, error) {
		called = true
		return "", errors.New("must not be called")
	}
	for _, request := range []transport.SetMailPasswordIfUnchangedRequest{
		{ExpectedBuildCommit: "mail-recovery-test", Email: "user@example.com", NewPassword: "New-Password-1!", ExpectedFingerprint: "nope"},
		{ExpectedBuildCommit: "mail-recovery-test", Email: "bad@@example.com", NewPassword: "New-Password-1!", ExpectedFingerprint: "sha256:" + strings.Repeat("0", 64)},
		{ExpectedBuildCommit: "mail-recovery-test", Email: "user@example.com", NewPassword: "New-Pass\nword-1!", ExpectedFingerprint: "sha256:" + strings.Repeat("0", 64)},
		{ExpectedBuildCommit: "mail-recovery-test", Email: "user@example.com", NewPassword: "New-Password-1!", ExpectedFingerprint: "sha256:" + strings.Repeat("0", 64)},
	} {
		var response transport.SetMailPasswordIfUnchangedResponse
		if err := (&Agent{}).SetMailPasswordIfUnchangedV1(&request, &response); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if response.Applied {
			t.Fatalf("request %+v applied", request.Email)
		}
		if strings.Contains(response.Error, "New-Pass") {
			t.Fatal("the error carries the password")
		}
	}
	if called {
		t.Fatal("hashed a refused request")
	}
	if got := readMailTestFile(t, dovecotUsersPath); got != before {
		t.Fatalf("file changed: %q", got)
	}
}
