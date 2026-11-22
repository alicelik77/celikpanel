package main

import (
	"os"
	"path/filepath"
	"testing"
)

// alpha.144: what the server holds for one address, read before the
// administrator's adoption; only CelikPanel's own form is adoptable.
func TestMailboxOnServerReadsOnlyCelikPanelsOwnForm(t *testing.T) {
	users := []byte("satis@example.com:{SHA512-CRYPT}$6$abc::::::" + dovecotQuotaFields(300) + "\n" +
		"home@example.com:{SHA512-CRYPT}$6$abc::::/var/mail/other::" + dovecotQuotaFields(100) + "\n" +
		"uid@example.com:{SHA512-CRYPT}$6$abc:5000:5000::::" + dovecotQuotaFields(100) + "\n" +
		"nolimit@example.com:{SHA512-CRYPT}$6$abc::::::\n" +
		"half@example.com:{SHA512-CRYPT}$6$abc::::::" + dovecotQuotaFields(100) + "\n" +
		"twice@example.com:{SHA512-CRYPT}$6$abc::::::" + dovecotQuotaFields(100) + "\n" +
		"other@example.com:{SHA512-CRYPT}$6$abc::::::" + dovecotQuotaFields(100) + "\n")
	vboxes := []byte("satis@example.com example.com/satis/\n" +
		"home@example.com example.com/home/\n" +
		"uid@example.com example.com/uid/\n" +
		"nolimit@example.com example.com/nolimit/\n" +
		"twice@example.com example.com/twice/\n" +
		"twice@example.com other.test/bob/\n" +
		"other@example.com other.test/bob/\n")
	for email, want := range map[string]struct {
		users, inMap, own bool
		quota             int
	}{
		"satis@example.com":   {true, true, true, 300},
		"home@example.com":    {true, true, false, 100},
		"uid@example.com":     {true, true, false, 100},
		"nolimit@example.com": {true, true, true, 0},
		"half@example.com":    {true, false, false, 100},
		"twice@example.com":   {true, true, false, 100},
		"other@example.com":   {true, true, false, 100},
		"gone@example.com":    {false, false, false, 0},
	} {
		email, local, domain, err := canonicalAgentMailbox(email)
		if err != nil {
			t.Fatal(err)
		}
		got := mailboxOnServer(users, vboxes, email, local, domain)
		if got.InUsers != want.users || got.InMap != want.inMap || got.OwnFolder != want.own || got.QuotaMB != want.quota {
			t.Errorf("%s: %+v, want %+v", email, got, want)
		}
	}
}

// Review of PR #355, finding 3: adoption needs an empty folder; the count is
// the files in any cur/ or new/ folder of the mailbox (and its subfolders);
// a link makes it unknown.
func TestMailboxMessageCountReadsCurAndNewOnly(t *testing.T) {
	root := t.TempDir()
	if n, err := mailboxMessageCount(root, "example.com", "none"); err != nil || n != 0 {
		t.Fatalf("no folder: %d %v", n, err)
	}
	box := filepath.Join(root, "example.com", "satis")
	for _, dir := range []string{"cur", "new", "tmp", ".Sent/cur", ".Sent/new"} {
		if err := os.MkdirAll(filepath.Join(box, dir), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"dovecot.index", "dovecot-uidlist", "tmp/partial", "subscriptions"} {
		_ = os.WriteFile(filepath.Join(box, name), []byte("x"), 0o600)
	}
	if n, err := mailboxMessageCount(root, "example.com", "satis"); err != nil || n != 0 {
		t.Fatalf("an empty mailbox with Dovecot's own files: %d %v", n, err)
	}
	_ = os.WriteFile(filepath.Join(box, "new", "1.mail"), []byte("x"), 0o600)
	_ = os.WriteFile(filepath.Join(box, ".Sent", "cur", "2.mail:2,S"), []byte("x"), 0o600)
	if n, err := mailboxMessageCount(root, "example.com", "satis"); err != nil || n != 2 {
		t.Fatalf("two messages: %d %v", n, err)
	}
	if err := os.Symlink("/etc", filepath.Join(box, ".Trash")); err != nil {
		t.Skip("links not available")
	}
	if _, err := mailboxMessageCount(root, "example.com", "satis"); err == nil {
		t.Fatal("a link inside the mailbox was counted through")
	}
}
