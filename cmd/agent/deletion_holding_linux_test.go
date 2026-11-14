//go:build linux

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.135: what deletions left is kept in two places and removed one item
// at a time; the Agent derives each path itself, follows no link and answers
// an item already gone as removed.

const heldTestDomainItem = "domain-7-0123456789abcdef"

func configureHoldingTest(t *testing.T) (mailHold, backups string) {
	t.Helper()
	configureMailMutationTest(t)
	withLifecycleTestBuild(t)
	savedBackups := backupBaseDir
	backupBaseDir = filepath.Join(t.TempDir(), "backups")
	t.Cleanup(func() { backupBaseDir = savedBackups })
	root, err := heldMailRoot()
	if err != nil {
		t.Fatal(err)
	}
	return root, backupBaseDir
}

func holdingTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func removeHeld(t *testing.T, req transport.RemoveHeldItemRequest) transport.RemoveHeldItemResponse {
	t.Helper()
	req.ExpectedBuildCommit = buildCommit
	var resp transport.RemoveHeldItemResponse
	if err := (&Agent{}).RemoveHeldItemV1(&req, &resp); err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestDeletionHoldingInventoryListsOnlyHeldItems(t *testing.T) {
	mailHold, backups := configureHoldingTest(t)
	holdingTestFile(t, filepath.Join(mailHold, heldTestDomainItem, "info", "cur", "1"), "12345")
	holdingTestFile(t, filepath.Join(mailHold, "mailbox-31-fedcba9876543210", "cur", "1"), "123")
	holdingTestFile(t, filepath.Join(mailHold, "not-a-held-folder", "x"), "1")
	if err := os.Symlink(t.TempDir(), filepath.Join(mailHold, "domain-8-0123456789abcdef")); err != nil {
		t.Fatal(err)
	}
	holdingTestFile(t, filepath.Join(backups, "subscriptions", "2", "domains", "12", "a.tar.gz"), "1234567")
	var resp transport.DeletionHoldingInventoryResponse
	if err := (&Agent{}).DeletionHoldingInventoryV1(&transport.DeletionHoldingInventoryRequest{}, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.MailError != "" || resp.BackupsError != "" {
		t.Fatalf("errors %q %q", resp.MailError, resp.BackupsError)
	}
	got := map[string]transport.HeldItemInfo{}
	for _, item := range resp.Items {
		got[item.Kind+"/"+item.Name] = item
	}
	if len(resp.Items) != 3 {
		t.Fatalf("items = %+v", resp.Items)
	}
	if item := got["mail/"+heldTestDomainItem]; item.DomainID != 7 || item.Bytes != 5 || item.Files != 1 {
		t.Fatalf("domain mail = %+v", item)
	}
	if item := got["mail/mailbox-31-fedcba9876543210"]; item.AccountID != 31 || item.DomainID != 0 || item.Bytes != 3 {
		t.Fatalf("mailbox mail = %+v", item)
	}
	if item := got["backups/"]; item.SubscriptionID != 2 || item.DomainID != 12 || item.Bytes != 7 || item.Files != 1 {
		t.Fatalf("backups = %+v", item)
	}
}

func TestRemoveHeldItemRemovesExactlyOneAndIsSafeToRepeat(t *testing.T) {
	mailHold, backups := configureHoldingTest(t)
	holdingTestFile(t, filepath.Join(mailHold, heldTestDomainItem, "info", "cur", "1"), "mail")
	holdingTestFile(t, filepath.Join(mailHold, "domain-9-0123456789abcdef", "keep"), "other")
	// A link inside the held folder is removed as a link; its target stays.
	outside := filepath.Join(t.TempDir(), "outside")
	holdingTestFile(t, outside, "not ours")
	if err := os.Symlink(outside, filepath.Join(mailHold, heldTestDomainItem, "link")); err != nil {
		t.Fatal(err)
	}
	if resp := removeHeld(t, transport.RemoveHeldItemRequest{Kind: transport.HeldItemMail, Name: heldTestDomainItem}); !resp.Removed || resp.Absent || resp.Error != "" {
		t.Fatalf("remove = %+v", resp)
	}
	if _, err := os.Lstat(filepath.Join(mailHold, heldTestDomainItem)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("the held folder stayed")
	}
	if content, err := os.ReadFile(outside); err != nil || string(content) != "not ours" {
		t.Fatal("a link's target was touched")
	}
	if _, err := os.Stat(filepath.Join(mailHold, "domain-9-0123456789abcdef", "keep")); err != nil {
		t.Fatal("another held item was touched")
	}
	// Repeating it answers removed (already gone).
	if resp := removeHeld(t, transport.RemoveHeldItemRequest{Kind: transport.HeldItemMail, Name: heldTestDomainItem}); !resp.Removed || !resp.Absent {
		t.Fatalf("repeat = %+v", resp)
	}
	// Backups: the domain's folder goes, then the subscription's folders
	// once nothing else is in them.
	holdingTestFile(t, filepath.Join(backups, "subscriptions", "2", "domains", "12", "a.tar.gz"), "1")
	holdingTestFile(t, filepath.Join(backups, "subscriptions", "2", "domains", "13", "b.tar.gz"), "1")
	if resp := removeHeld(t, transport.RemoveHeldItemRequest{Kind: transport.HeldItemBackups, SubscriptionID: 2, DomainID: 12}); !resp.Removed {
		t.Fatalf("backups = %+v", resp)
	}
	if _, err := os.Stat(filepath.Join(backups, "subscriptions", "2", "domains", "13", "b.tar.gz")); err != nil {
		t.Fatal("another domain's backups were touched")
	}
	if resp := removeHeld(t, transport.RemoveHeldItemRequest{Kind: transport.HeldItemBackups, SubscriptionID: 2, DomainID: 13}); !resp.Removed {
		t.Fatalf("backups 13 = %+v", resp)
	}
	if _, err := os.Lstat(filepath.Join(backups, "subscriptions", "2")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("the subscription's empty backup folders stayed")
	}
}

func TestRemoveHeldItemRefusesALinkAndAnythingOutsideTheTwoPlaces(t *testing.T) {
	mailHold, backups := configureHoldingTest(t)
	target := t.TempDir()
	holdingTestFile(t, filepath.Join(target, "precious"), "keep")
	if err := os.MkdirAll(mailHold, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(mailHold, heldTestDomainItem)); err != nil {
		t.Fatal(err)
	}
	if resp := removeHeld(t, transport.RemoveHeldItemRequest{Kind: transport.HeldItemMail, Name: heldTestDomainItem}); resp.Removed || !strings.Contains(resp.Error, "not a folder") {
		t.Fatalf("link = %+v", resp)
	}
	if _, err := os.Stat(filepath.Join(target, "precious")); err != nil {
		t.Fatal("the link's target was touched")
	}
	// The subscription's folder itself a link: refused before anything.
	if err := os.MkdirAll(filepath.Join(backups, "subscriptions"), 0o700); err != nil {
		t.Fatal(err)
	}
	holdingTestFile(t, filepath.Join(target, "domains", "12", "a.tar.gz"), "x")
	if err := os.Symlink(target, filepath.Join(backups, "subscriptions", "2")); err != nil {
		t.Fatal(err)
	}
	if resp := removeHeld(t, transport.RemoveHeldItemRequest{Kind: transport.HeldItemBackups, SubscriptionID: 2, DomainID: 12}); resp.Removed || resp.Error == "" {
		t.Fatalf("linked backups = %+v", resp)
	}
	if _, err := os.Stat(filepath.Join(target, "domains", "12", "a.tar.gz")); err != nil {
		t.Fatal("a file behind a link was removed")
	}
	for _, req := range []transport.RemoveHeldItemRequest{
		{Kind: transport.HeldItemMail, Name: "../vhosts"},
		{Kind: transport.HeldItemMail, Name: "example.com"},
		{Kind: transport.HeldItemBackups, SubscriptionID: 0, DomainID: 12},
		{Kind: transport.HeldItemBackups, SubscriptionID: 2, DomainID: 12, Name: "x"},
		{Kind: "files", Name: heldTestDomainItem},
	} {
		if resp := removeHeld(t, req); resp.Removed || resp.Error == "" {
			t.Fatalf("%+v accepted: %+v", req, resp)
		}
	}
}

func TestDeleteMailAccountHoldsTheMailboxSoANewOneStartsEmpty(t *testing.T) {
	configureMailMutationTest(t)
	writeMailTestFile(t, dovecotUsersPath, "info@example.com:{CRYPT}hash::::::userdb_quota_rule=*:storage=10M\n", 0o640)
	writeMailTestFile(t, postfixVBoxPath, "info@example.com example.com/info/\n", 0o644)
	if err := secureEnsureMailRoot(mailRootDir); err != nil {
		t.Fatal(err)
	}
	holdingTestFile(t, filepath.Join(mailRootDir, "example.com", "info", "cur", "1"), "hello")
	var resp transport.MailMutationResponse
	if err := (&Agent{}).DeleteMailAccount(&transport.DeleteMailAccountRequest{Email: "info@example.com", AccountID: 31}, &resp); err != nil {
		t.Fatal(err)
	}
	want := heldMailboxName(31, "info@example.com")
	if !resp.Applied || resp.HeldName != want || resp.HeldBytes != 5 {
		t.Fatalf("response = %+v", resp)
	}
	if _, err := os.Lstat(filepath.Join(mailRootDir, "example.com", "info")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("the mailbox folder stayed where a new mailbox of this address would find it")
	}
	root, _ := heldMailRoot()
	if content, err := os.ReadFile(filepath.Join(root, want, "cur", "1")); err != nil || string(content) != "hello" {
		t.Fatal("the held mail is not in the holding folder")
	}
	// A compensation (no account id) leaves the folder where it is.
	writeMailTestFile(t, dovecotUsersPath, "new@example.com:{CRYPT}hash::::::userdb_quota_rule=*:storage=10M\n", 0o640)
	writeMailTestFile(t, postfixVBoxPath, "new@example.com example.com/new/\n", 0o644)
	holdingTestFile(t, filepath.Join(mailRootDir, "example.com", "new", "cur", "1"), "x")
	resp = transport.MailMutationResponse{}
	if err := (&Agent{}).DeleteMailAccount(&transport.DeleteMailAccountRequest{Email: "new@example.com"}, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.HeldName != "" {
		t.Fatalf("a compensation held mail: %+v", resp)
	}
	if _, err := os.Stat(filepath.Join(mailRootDir, "example.com", "new", "cur", "1")); err != nil {
		t.Fatal("a compensation moved the folder")
	}
}
