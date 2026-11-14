package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Mail left in place (alpha.135; internal/transport/deletion_holding.go).
// Before alpha.135 deleting a mailbox left its folder <mail root>/<domain>/<local>
// where it was, so a mailbox made again under the same address saw the old
// mail. Three things close that, all through the one holding folder of
// deletion_holding.go:
//
//   - AddMailAccount, given the domain's record, first moves such a folder
//     into the holding folder as left-<domain id>-<16 hex>, then makes the new
//     mailbox's folder empty (mail_rpc.go);
//   - LeftMailFoldersV1 lists, read only, the mailbox folders in live domain
//     folders that no mailbox account owns (Maintenance shows them);
//   - HoldLeftMailFolderV1 moves one of them into the holding folder when the
//     administrator asks. The folder is derived from the address, never taken
//     from the caller; an address that has a mailbox account is refused; no
//     symbolic link is followed (holdMailboxDirectory).
//
// Nothing here moves anything by itself, and nothing is removed: removal is
// RemoveHeldItemV1's, after the holding period.
//
// Yerinde kalan posta: önceki sürümlerde silinen kutunun klasörü canlı alan
// adı klasöründe kalıyordu. Kutu yeniden açılırken önce saklamaya alınır;
// Bakım listesi kalanları gösterir; yönetici isterse tek tek saklamaya alır.

// leftMailListLimit bounds one answer of LeftMailFoldersV1.
const leftMailListLimit = 500

// leftMailboxName names the holding folder of a mailbox folder left in place.
// The random part keeps two such folders of the same address apart.
func leftMailboxName(domainID int, email string) (string, error) {
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("name the holding folder: %w", err)
	}
	digest := sha256.Sum256(append([]byte(email+"\x00"), nonce...))
	return fmt.Sprintf("left-%d-%s", domainID, hex.EncodeToString(digest[:8])), nil
}

// mailAccountAddresses is every address that has a mailbox account: the
// Dovecot users file and Postfix's mailbox map, lower case.
func mailAccountAddresses() (map[string]bool, error) {
	addresses := map[string]bool{}
	users, err := readMailFile(dovecotUsersPath)
	if err != nil {
		return nil, fmt.Errorf("read the mailbox accounts: %w", err)
	}
	for _, line := range splitMailFile(users) {
		if key, _, ok := strings.Cut(line, ":"); ok && key != "" {
			addresses[strings.ToLower(strings.TrimSpace(key))] = true
		}
	}
	boxes, err := readMailFile(postfixVBoxPath)
	if err != nil {
		return nil, fmt.Errorf("read the mailbox map: %w", err)
	}
	for _, line := range splitMailFile(boxes) {
		if fields := strings.Fields(line); len(fields) > 0 {
			addresses[strings.ToLower(fields[0])] = true
		}
	}
	return addresses, nil
}

// LeftMailFoldersV1 lists the mailbox folders in live domain mail folders
// that no mailbox account owns. Read only. The list is read under the mail
// lock, so a mailbox being made is never one of them; sizes are measured
// after it.
func (a *Agent) LeftMailFoldersV1(_ *transport.LeftMailFoldersRequest, resp *transport.LeftMailFoldersResponse) error {
	resp.Folders = []transport.LeftMailFolder{}
	root, err := managedMailRootPath()
	if err != nil {
		resp.Error = boundedText("name the mail root: "+err.Error(), 200)
		return nil
	}
	mailMutex.Lock()
	found, more, err := leftMailFolders(root)
	mailMutex.Unlock()
	if err != nil {
		resp.Error = boundedText(err.Error(), 200)
		return nil
	}
	for _, folder := range found {
		folder.Bytes, folder.Files, folder.Unknown = measureHeldTree(folder.Path)
		if folder.Unknown {
			folder.Bytes = -1
		}
		resp.Folders = append(resp.Folders, folder)
	}
	resp.More = more
	return nil
}

func leftMailFolders(root string) ([]transport.LeftMailFolder, bool, error) {
	domains, err := os.ReadDir(root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read the mail root: %w", err)
	}
	accounts, err := mailAccountAddresses()
	if err != nil {
		return nil, false, err
	}
	var out []transport.LeftMailFolder
	for _, domain := range domains {
		// The holding folder and anything else hidden is not a domain; a
		// link is never followed.
		if strings.HasPrefix(domain.Name(), ".") || !domain.IsDir() || domain.Type()&fs.ModeSymlink != 0 {
			continue
		}
		canonical, err := canonicalAgentDomain(domain.Name())
		if err != nil || canonical != domain.Name() {
			continue
		}
		boxes, err := os.ReadDir(filepath.Join(root, domain.Name()))
		if err != nil {
			continue
		}
		for _, box := range boxes {
			if strings.HasPrefix(box.Name(), ".") || !box.IsDir() || box.Type()&fs.ModeSymlink != 0 {
				continue
			}
			email, _, _, err := canonicalAgentMailbox(box.Name() + "@" + domain.Name())
			if err != nil || email != box.Name()+"@"+domain.Name() || accounts[strings.ToLower(email)] {
				continue
			}
			if len(out) == leftMailListLimit {
				sortLeftMailFolders(out)
				return out, true, nil
			}
			out = append(out, transport.LeftMailFolder{Address: email, Path: filepath.Join(root, domain.Name(), box.Name())})
		}
	}
	sortLeftMailFolders(out)
	return out, false, nil
}

func sortLeftMailFolders(out []transport.LeftMailFolder) {
	sort.Slice(out, func(i, j int) bool { return out[i].Address < out[j].Address })
}

// HoldLeftMailFolderV1 moves the folder of one address that has no mailbox
// account into the mail holding folder (kept 30 days by the Panel's list).
// Repeating it after the folder went answers Absent.
func (a *Agent) HoldLeftMailFolderV1(req *transport.HoldLeftMailFolderRequest, resp *transport.HoldLeftMailFolderResponse) error {
	if req == nil || resp == nil {
		return fmt.Errorf("a request is required")
	}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "holding a mail folder left in place"); err != nil {
		return err
	}
	email, local, domain, err := canonicalAgentMailbox(req.Email)
	if err != nil || req.DomainID < 1 {
		resp.Error = "not a mailbox address of a domain"
		return nil
	}
	mailMutex.Lock()
	defer mailMutex.Unlock()
	accounts, err := mailAccountAddresses()
	if err != nil {
		resp.Error = boundedText(err.Error(), 200)
		return nil
	}
	if accounts[strings.ToLower(email)] {
		resp.Error = "this address has a mailbox again, so its folder is in use; it was left as it is"
		return nil
	}
	name, err := leftMailboxName(req.DomainID, email)
	if err != nil {
		resp.Error = boundedText(err.Error(), 200)
		return nil
	}
	held, err := holdLeftMailbox(domain, local, name)
	if err != nil {
		resp.Error = boundedText(err.Error(), 200)
		return nil
	}
	if !held {
		resp.Absent = true
		return nil
	}
	resp.Held, resp.HeldName, resp.HeldBytes = true, name, heldMailBytes(name)
	return nil
}

// holdLeftMailbox moves <mail root>/<domain>/<local> into the holding folder
// as name. When the move happened but could not be made durable, it is put
// back and the failure is answered: nothing is half done.
func holdLeftMailbox(domain, local, name string) (bool, error) {
	rollback, moved, err := holdMailboxDirectory(domain, local, name)
	if err != nil {
		if rollback != nil {
			if restoreErr := rollback(); restoreErr != nil {
				return false, fmt.Errorf("%w; putting it back failed too: %v", err, restoreErr)
			}
		}
		return false, err
	}
	return moved, nil
}

// heldMailBytes is the size of a held mail folder, -1 when it is not known.
func heldMailBytes(name string) int64 {
	root, err := heldMailRoot()
	if err != nil {
		return -1
	}
	bytes, _, unknown := measureHeldTree(filepath.Join(root, name))
	if unknown {
		return -1
	}
	return bytes
}
