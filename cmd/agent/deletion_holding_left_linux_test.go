//go:build linux

package main

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.135: a mailbox folder an earlier release left in its live domain
// folder is put into the holding folder before a mailbox is made at its
// address, is listed read only, and is held one at a time when asked; the
// Agent derives the folder from the address, refuses an address that has a
// mailbox, and follows no link.

var leftHeldName = regexp.MustCompile(`^left-7-[0-9a-f]{16}$`)

func configureLeftMailTest(t *testing.T) (mailHold string) {
	t.Helper()
	mailHold, _ = configureHoldingTest(t)
	if err := secureEnsureMailRoot(mailRootDir); err != nil {
		t.Fatal(err)
	}
	return mailHold
}

func addMailbox(t *testing.T, email string, leftDomainID int) (transport.MailMutationResponse, error) {
	t.Helper()
	var resp transport.MailMutationResponse
	err := (&Agent{}).AddMailAccount(&transport.MailAccount{Email: email, Password: "Long-enough-1", QuotaMB: 10, LeftDomainID: leftDomainID}, &resp)
	return resp, err
}

func TestAddMailAccountPutsMailLeftAtTheAddressAsideFirst(t *testing.T) {
	mailHold := configureLeftMailTest(t)
	holdingTestFile(t, filepath.Join(mailRootDir, "example.com", "info", "cur", "1"), "old mail")
	resp, err := addMailbox(t, "info@example.com", 7)
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Applied || !leftHeldName.MatchString(resp.HeldName) || resp.HeldBytes != 8 {
		t.Fatalf("response = %+v", resp)
	}
	if _, err := os.Stat(filepath.Join(mailRootDir, "example.com", "info", "cur", "1")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("the new mailbox would see the old mail")
	}
	if content, err := os.ReadFile(filepath.Join(mailHold, resp.HeldName, "cur", "1")); err != nil || string(content) != "old mail" {
		t.Fatal("the old mail is not in the holding folder")
	}
	users, _ := os.ReadFile(dovecotUsersPath)
	if !strings.HasPrefix(string(users), "info@example.com:") {
		t.Fatalf("the mailbox was not made: %q", users)
	}
	// The inventory knows the held folder by its domain.
	var inventory transport.DeletionHoldingInventoryResponse
	if err := (&Agent{}).DeletionHoldingInventoryV1(&transport.DeletionHoldingInventoryRequest{}, &inventory); err != nil {
		t.Fatal(err)
	}
	if len(inventory.Items) != 1 || inventory.Items[0].Name != resp.HeldName || inventory.Items[0].DomainID != 7 || inventory.Items[0].AccountID != 0 {
		t.Fatalf("inventory = %+v", inventory.Items)
	}
	// Without the domain's record (an older Panel) the folder is kept as before.
	holdingTestFile(t, filepath.Join(mailRootDir, "example.com", "sales", "cur", "1"), "x")
	resp, err = addMailbox(t, "sales@example.com", 0)
	if err != nil || resp.HeldName != "" {
		t.Fatalf("no domain record: %+v, %v", resp, err)
	}
	if _, err := os.Stat(filepath.Join(mailRootDir, "example.com", "sales", "cur", "1")); err != nil {
		t.Fatal("the folder was moved without the domain's record")
	}
	// A mailbox that never had a folder holds nothing.
	if resp, err = addMailbox(t, "new@example.com", 7); err != nil || resp.HeldName != "" {
		t.Fatalf("nothing to hold: %+v, %v", resp, err)
	}
}

func TestAddMailAccountPutsTheOldFolderBackWhenTheMailboxIsNotMade(t *testing.T) {
	mailHold := configureLeftMailTest(t)
	holdingTestFile(t, filepath.Join(mailRootDir, "example.com", "info", "cur", "1"), "old mail")
	mailMutationWrite = func(string, []byte, os.FileMode) error { return errors.New("disk full") }
	if _, err := addMailbox(t, "info@example.com", 7); err == nil {
		t.Fatal("a failed write was answered as made")
	}
	if content, err := os.ReadFile(filepath.Join(mailRootDir, "example.com", "info", "cur", "1")); err != nil || string(content) != "old mail" {
		t.Fatal("the old folder was not put back")
	}
	entries, _ := os.ReadDir(mailHold)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "left-") {
			t.Fatalf("a held folder stayed after the failure: %s", entry.Name())
		}
	}
}

func TestLeftMailFoldersListsOnlyFoldersWithoutAMailbox(t *testing.T) {
	mailHold := configureLeftMailTest(t)
	writeMailTestFile(t, dovecotUsersPath, "info@example.com:{CRYPT}hash::::::userdb_quota_rule=*:storage=10M\n", 0o640)
	writeMailTestFile(t, postfixVBoxPath, "info@example.com example.com/info/\nmapped@example.com example.com/mapped/\n", 0o644)
	holdingTestFile(t, filepath.Join(mailRootDir, "example.com", "info", "cur", "1"), "live")
	holdingTestFile(t, filepath.Join(mailRootDir, "example.com", "mapped", "cur", "1"), "live")
	holdingTestFile(t, filepath.Join(mailRootDir, "example.com", "old", "cur", "1"), "abc")
	holdingTestFile(t, filepath.Join(mailRootDir, "example.com", ".hidden", "x"), "x")
	holdingTestFile(t, filepath.Join(mailRootDir, "other.org", "sales", "cur", "1"), "12345")
	holdingTestFile(t, filepath.Join(mailHold, "mailbox-31-0123456789abcdef", "cur", "1"), "held")
	outside := t.TempDir()
	holdingTestFile(t, filepath.Join(outside, "cur", "1"), "not ours")
	if err := os.Symlink(outside, filepath.Join(mailRootDir, "example.com", "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(mailRootDir, "other.org"), filepath.Join(mailRootDir, "linked.org")); err != nil {
		t.Fatal(err)
	}
	var resp transport.LeftMailFoldersResponse
	if err := (&Agent{}).LeftMailFoldersV1(&transport.LeftMailFoldersRequest{}, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error != "" || len(resp.Folders) != 2 {
		t.Fatalf("folders = %+v (%s)", resp.Folders, resp.Error)
	}
	if got := resp.Folders[0]; got.Address != "old@example.com" || got.Bytes != 3 || got.Path != filepath.Join(mailRootDir, "example.com", "old") {
		t.Fatalf("first = %+v", got)
	}
	if got := resp.Folders[1]; got.Address != "sales@other.org" || got.Bytes != 5 {
		t.Fatalf("second = %+v", got)
	}
}

func TestHoldLeftMailFolderMovesOneAndRefusesAMailboxOrALink(t *testing.T) {
	mailHold := configureLeftMailTest(t)
	writeMailTestFile(t, dovecotUsersPath, "info@example.com:{CRYPT}hash::::::userdb_quota_rule=*:storage=10M\n", 0o640)
	holdingTestFile(t, filepath.Join(mailRootDir, "example.com", "info", "cur", "1"), "live")
	holdingTestFile(t, filepath.Join(mailRootDir, "example.com", "old", "cur", "1"), "abc")
	outside := t.TempDir()
	holdingTestFile(t, filepath.Join(outside, "cur", "1"), "not ours")
	if err := os.Symlink(outside, filepath.Join(mailRootDir, "example.com", "link")); err != nil {
		t.Fatal(err)
	}
	hold := func(email string, domainID int) transport.HoldLeftMailFolderResponse {
		t.Helper()
		var resp transport.HoldLeftMailFolderResponse
		if err := (&Agent{}).HoldLeftMailFolderV1(&transport.HoldLeftMailFolderRequest{ExpectedBuildCommit: buildCommit, Email: email, DomainID: domainID}, &resp); err != nil {
			t.Fatal(err)
		}
		return resp
	}
	resp := hold("old@example.com", 7)
	if !resp.Held || !leftHeldName.MatchString(resp.HeldName) || resp.HeldBytes != 3 {
		t.Fatalf("hold = %+v", resp)
	}
	if _, err := os.Stat(filepath.Join(mailHold, resp.HeldName, "cur", "1")); err != nil {
		t.Fatal("the folder is not in the holding folder")
	}
	// Pressed again: nothing there, nothing moved.
	if again := hold("old@example.com", 7); again.Held || !again.Absent || again.Error != "" {
		t.Fatalf("again = %+v", again)
	}
	// An address that has a mailbox is refused; its folder stays.
	if refused := hold("info@example.com", 7); refused.Held || !strings.Contains(refused.Error, "has a mailbox") {
		t.Fatalf("a mailbox's folder = %+v", refused)
	}
	if _, err := os.Stat(filepath.Join(mailRootDir, "example.com", "info", "cur", "1")); err != nil {
		t.Fatal("a live mailbox's folder was moved")
	}
	// A link is never followed or moved.
	if link := hold("link@example.com", 7); link.Held || link.Error == "" {
		t.Fatalf("a link = %+v", link)
	}
	if _, err := os.Stat(filepath.Join(outside, "cur", "1")); err != nil {
		t.Fatal("the link's target was touched")
	}
	if info, err := os.Lstat(filepath.Join(mailRootDir, "example.com", "link")); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("the link was moved")
	}
	// Not an address, or no domain record: refused before anything.
	for _, req := range []transport.HoldLeftMailFolderRequest{
		{Email: "../vhosts@example.com", DomainID: 7},
		{Email: "old@example.com", DomainID: 0},
		{Email: "example.com", DomainID: 7},
	} {
		req.ExpectedBuildCommit = buildCommit
		var refused transport.HoldLeftMailFolderResponse
		if err := (&Agent{}).HoldLeftMailFolderV1(&req, &refused); err != nil {
			t.Fatal(err)
		}
		if refused.Held || refused.Error == "" {
			t.Fatalf("%+v accepted: %+v", req, refused)
		}
	}
	// The held folder is removed like every held item.
	if removed := removeHeld(t, transport.RemoveHeldItemRequest{Kind: transport.HeldItemMail, Name: resp.HeldName}); !removed.Removed || removed.Absent {
		t.Fatalf("remove = %+v", removed)
	}
}
