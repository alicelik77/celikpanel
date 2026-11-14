package main

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/alicelik/celikpanel/internal/transport"
)

// What deletions left, kept 30 days (alpha.135, owner decision 14 Nov 2026;
// internal/transport/deletion_holding.go). The Agent knows two holding
// places and nothing else:
//
//   - mail: <mail root>/.celikpanel-quarantine/<name>, the folder a domain's
//     or a mailbox's stored mail was moved into when it was deleted
//     (domain-<domain id>-<16 hex>, mailbox-<account id>-<16 hex>), or a
//     mailbox folder an earlier release left in its live domain folder
//     (left-<domain id>-<16 hex>; deletion_holding_left.go);
//   - backups: <backup root>/subscriptions/<S>/domains/<D>, a deleted
//     domain's backups where the backup store keeps them.
//
// The inventory only reads. A removal derives the item's path from its kind
// and identifier, removes exactly that one folder through an os.Root opened
// on its parent (a symbolic link inside is removed as a link, never
// followed), refuses an item that is itself a link or not a folder, and
// answers an item that is already gone as removed: repeating it is safe.
// Which items are due, and that their domain or mailbox is really gone, is
// the Panel's decision (cmd/panel/deletion_holding.go).
//
// Silmelerden kalanlar: Ajan yalnız iki yeri bilir; yolu türden ve kimlikten
// kendisi çıkarır, tek klasörü siler, bağlantı izlemez, tekrarı güvenlidir.

// heldMailName is a held mail folder's name.
var heldMailName = regexp.MustCompile(`^(domain|mailbox|left)-([1-9][0-9]{0,17})-[0-9a-f]{16}$`)

// heldMailboxName names the folder a deleted mailbox's mail is held in.
func heldMailboxName(accountID int, email string) string {
	digest := sha256.Sum256([]byte(email))
	return fmt.Sprintf("mailbox-%d-%x", accountID, digest[:8])
}

// heldMailRoot is the mail holding folder, or "" when the mail root cannot
// be named. A seam for tests.
var heldMailRoot = func() (string, error) {
	root, err := managedMailRootPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, mailDomainQuarantineDirectory), nil
}

// mailDomainQuarantineDirectory is the mail quarantine folder, under the mail root.
const mailDomainQuarantineDirectory = ".celikpanel-quarantine"

// heldBackupsRoot is the folder that holds every subscription's backups.
func heldBackupsRoot() string { return filepath.Join(backupBaseDir, "subscriptions") }

// DeletionHoldingInventoryV1 lists what is in the two holding places. Read only.
func (a *Agent) DeletionHoldingInventoryV1(_ *transport.DeletionHoldingInventoryRequest, resp *transport.DeletionHoldingInventoryResponse) error {
	resp.Items = []transport.HeldItemInfo{}
	mail, err := heldMailInventory()
	if err != nil {
		resp.MailError = boundedText(err.Error(), 200)
	}
	resp.Items = append(resp.Items, mail...)
	backups, err := heldBackupsInventory()
	if err != nil {
		resp.BackupsError = boundedText(err.Error(), 200)
	}
	resp.Items = append(resp.Items, backups...)
	return nil
}

func heldMailInventory() ([]transport.HeldItemInfo, error) {
	root, err := heldMailRoot()
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a folder", root)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var out []transport.HeldItemInfo
	for _, entry := range entries {
		match := heldMailName.FindStringSubmatch(entry.Name())
		if match == nil || entry.Type()&fs.ModeSymlink != 0 || !entry.IsDir() {
			continue
		}
		id, _ := strconv.Atoi(match[2])
		item := transport.HeldItemInfo{Kind: transport.HeldItemMail, Name: entry.Name(), Path: filepath.Join(root, entry.Name())}
		if match[1] == "domain" || match[1] == "left" {
			item.DomainID = id
		} else {
			item.AccountID = id
		}
		item.Bytes, item.Files, item.Unknown = measureHeldTree(filepath.Join(root, entry.Name()))
		out = append(out, item)
	}
	return out, nil
}

func heldBackupsInventory() ([]transport.HeldItemInfo, error) {
	root := heldBackupsRoot()
	subscriptions, err := os.ReadDir(root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []transport.HeldItemInfo
	for _, subscription := range subscriptions {
		subscriptionID, err := strconv.Atoi(subscription.Name())
		if err != nil || subscriptionID < 1 || strconv.Itoa(subscriptionID) != subscription.Name() || !subscription.IsDir() {
			continue
		}
		domainsDir := filepath.Join(root, subscription.Name(), "domains")
		domains, err := os.ReadDir(domainsDir)
		if err != nil {
			continue
		}
		for _, domain := range domains {
			domainID, err := strconv.Atoi(domain.Name())
			if err != nil || domainID < 1 || strconv.Itoa(domainID) != domain.Name() || !domain.IsDir() {
				continue
			}
			item := transport.HeldItemInfo{Kind: transport.HeldItemBackups, SubscriptionID: subscriptionID, DomainID: domainID, Path: filepath.Join(domainsDir, domain.Name())}
			item.Bytes, item.Files, item.Unknown = measureHeldTree(filepath.Join(domainsDir, domain.Name()))
			out = append(out, item)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DomainID < out[j].DomainID })
	return out, nil
}

// measureHeldTree sums the regular files under path without following links.
func measureHeldTree(path string) (bytes int64, files int, unknown bool) {
	err := filepath.WalkDir(path, func(_ string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type().IsRegular() {
			info, infoErr := entry.Info()
			if infoErr != nil {
				return infoErr
			}
			files++
			bytes += info.Size()
		}
		return nil
	})
	return bytes, files, err != nil
}

// RemoveHeldItemV1 removes exactly one held item.
func (a *Agent) RemoveHeldItemV1(req *transport.RemoveHeldItemRequest, resp *transport.RemoveHeldItemResponse) error {
	if req == nil || resp == nil {
		return fmt.Errorf("a request is required")
	}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "removing a held item"); err != nil {
		return err
	}
	var parent, name string
	switch req.Kind {
	case transport.HeldItemMail:
		if !heldMailName.MatchString(req.Name) {
			resp.Error = "not a held mail folder name"
			return nil
		}
		root, err := heldMailRoot()
		if err != nil {
			resp.Error = boundedText("name the mail holding folder: "+err.Error(), 200)
			return nil
		}
		parent, name = root, req.Name
	case transport.HeldItemBackups:
		if req.SubscriptionID < 1 || req.DomainID < 1 || req.Name != "" {
			resp.Error = "not a held backups item"
			return nil
		}
		parent = filepath.Join(heldBackupsRoot(), strconv.Itoa(req.SubscriptionID), "domains")
		name = strconv.Itoa(req.DomainID)
	default:
		resp.Error = "unknown held item kind"
		return nil
	}
	absent, err := removeHeldFolder(parent, name)
	if err != nil {
		resp.Error = boundedText(err.Error(), 200)
		return nil
	}
	if req.Kind == transport.HeldItemBackups {
		// The subscription's backup folders go too once they are empty (the
		// backup store makes them again when it needs them).
		if err := removeEmptyFolder(parent); err != nil {
			resp.Error = boundedText(err.Error(), 200)
			return nil
		}
		if err := removeEmptyFolder(filepath.Dir(parent)); err != nil {
			resp.Error = boundedText(err.Error(), 200)
			return nil
		}
	}
	resp.Removed, resp.Absent = true, absent
	return nil
}

// removeHeldFolder removes parent/name, a folder, without following any
// link: parent must be reachable without links, and name must be a folder.
// absent: it was already gone.
func removeHeldFolder(parent, name string) (absent bool, err error) {
	if _, err := os.Lstat(parent); errors.Is(err, fs.ErrNotExist) {
		return true, nil
	}
	if err := rejectSymlinkPath(parent); err != nil {
		return false, fmt.Errorf("%s: %w; nothing was removed", parent, err)
	}
	root, err := os.OpenRoot(parent)
	if err != nil {
		return false, fmt.Errorf("open %s: %w", parent, err)
	}
	defer root.Close()
	info, err := root.Lstat(name)
	if errors.Is(err, fs.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect %s: %w", filepath.Join(parent, name), err)
	}
	if !info.IsDir() || info.Mode()&fs.ModeSymlink != 0 {
		return false, fmt.Errorf("%s is not a folder; it was left as it is", filepath.Join(parent, name))
	}
	if err := root.RemoveAll(name); err != nil {
		return false, fmt.Errorf("remove %s: %s", filepath.Join(parent, name), strings.TrimSpace(err.Error()))
	}
	return false, nil
}
