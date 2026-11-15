//go:build linux

package main

import (
	"errors"
	"fmt"

	"golang.org/x/sys/unix"
)

// holdMailboxDirectory moves a deleted mailbox's stored mail, the folder
// <mail root>/<domain>/<local>, into the mail holding folder under name
// (alpha.135: kept 30 days, then removed; a mailbox made again under the same
// address starts empty). Like the domain's quarantine it is one rename on the
// same file system, with no symbolic link followed, and it returns the
// rollback the mail file transaction runs when a later step fails. A mailbox
// that never received mail has no folder: nothing is held.
func holdMailboxDirectory(domain, local, name string) (func() error, bool, error) {
	rootFD, err := openManagedMailRoot()
	if errors.Is(err, unix.ENOENT) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("open mail root to hold the mailbox: %w", err)
	}
	defer unix.Close(rootFD)
	domainFD, err := openMailDirectoryAt(rootFD, domain)
	if errors.Is(err, unix.ENOENT) {
		return nil, false, nil
	}
	if err != nil {
		if mailEntryIsLinkAt(rootFD, domain, err) {
			return nil, false, fmt.Errorf("open the domain's mail folder: %w (%s): %w", errMailPathSymlinkRefused, domain, err)
		}
		return nil, false, fmt.Errorf("open the domain's mail folder: %w", err)
	}
	defer unix.Close(domainFD)
	sourceExists, err := mailDirectoryExistsAt(domainFD, local)
	if err != nil {
		if mailEntryIsLinkAt(domainFD, local, err) {
			return nil, false, fmt.Errorf("inspect the mailbox folder: %w (%s): %w", errMailPathSymlinkRefused, local, err)
		}
		return nil, false, fmt.Errorf("inspect the mailbox folder: %w", err)
	}
	if !sourceExists {
		return nil, false, nil
	}
	holdFD, _, err := openMailQuarantineRoot(rootFD, true)
	if err != nil {
		return nil, false, err
	}
	defer unix.Close(holdFD)
	targetExists, err := mailDirectoryExistsAt(holdFD, name)
	if err != nil {
		return nil, false, fmt.Errorf("inspect the holding target: %w", err)
	}
	if targetExists {
		return nil, false, fmt.Errorf("the mailbox folder and its holding target both exist")
	}
	if err := unix.Renameat2(domainFD, local, holdFD, name, unix.RENAME_NOREPLACE); err != nil {
		return nil, false, fmt.Errorf("hold the mailbox folder: %w", err)
	}
	rollback := func() error { return restoreHeldMailbox(domain, local, name) }
	if err := unix.Fsync(domainFD); err != nil {
		return rollback, true, fmt.Errorf("sync the domain's mail folder: %w", err)
	}
	if err := unix.Fsync(holdFD); err != nil {
		return rollback, true, fmt.Errorf("sync the mail holding folder: %w", err)
	}
	return nil, true, nil
}

// restoreHeldMailbox puts a held mailbox folder back (the transaction's
// rollback). It refuses when the mailbox folder exists again.
func restoreHeldMailbox(domain, local, name string) error {
	rootFD, err := openManagedMailRoot()
	if err != nil {
		return err
	}
	defer unix.Close(rootFD)
	domainFD, err := openMailDirectoryAt(rootFD, domain)
	if err != nil {
		return err
	}
	defer unix.Close(domainFD)
	holdFD, exists, err := openMailQuarantineRoot(rootFD, false)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("mail holding folder disappeared during rollback")
	}
	defer unix.Close(holdFD)
	if err := unix.Renameat2(holdFD, name, domainFD, local, unix.RENAME_NOREPLACE); err != nil {
		return fmt.Errorf("restore the held mailbox folder: %w", err)
	}
	if err := unix.Fsync(holdFD); err != nil {
		return err
	}
	return unix.Fsync(domainFD)
}
