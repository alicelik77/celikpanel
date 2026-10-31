//go:build linux

package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

// nativeMailboxSieveStore keeps the mailbox's Sieve files inside the
// mailbox's own maildir, owned by the mail user (vmail, 0600/0700), so
// Dovecot (running as vmail) reads and compiles them. Every step opens
// relative to the managed mail root with RESOLVE_BENEATH|RESOLVE_NO_SYMLINKS:
// a symbolic link planted anywhere on the path is refused, never followed.
// The active link .dovecot.sieve is the only symbolic link CelikPanel makes;
// it is read with readlinkat and replaced atomically with renameat.
type nativeMailboxSieveStore struct{}

// autoresponderOpenMailRoot is a seam: component tests open a scratch root.
var autoresponderOpenMailRoot = openManagedMailRoot

const mailboxSieveFileLimit = 64 << 10

func openMailboxDirAt(rootFD int, domain, local string, create bool) (int, error) {
	parent := rootFD
	for index, name := range []string{domain, local} {
		if create {
			if _, err := mkdirMaildirAt(parent, name, 5000, 5000); err != nil {
				if parent != rootFD {
					unix.Close(parent)
				}
				return -1, err
			}
		}
		fd, err := unix.Openat2(parent, name, &unix.OpenHow{
			Flags:   uint64(unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC | unix.O_NOFOLLOW),
			Resolve: secureMaildirResolve,
		})
		if parent != rootFD {
			unix.Close(parent)
		}
		if err != nil {
			return -1, fmt.Errorf("open mailbox directory (%d): %w", index, err)
		}
		parent = fd
	}
	return parent, nil
}

func readFileAt(dirFD int, name string) ([]byte, bool, error) {
	fd, err := unix.Openat2(dirFD, name, &unix.OpenHow{
		Flags:   uint64(unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW),
		Resolve: secureMaildirResolve,
	})
	if errors.Is(err, unix.ENOENT) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	file := os.NewFile(uintptr(fd), name)
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, false, err
	}
	if !info.Mode().IsRegular() {
		return nil, false, fmt.Errorf("%s is not a regular file", name)
	}
	content, err := io.ReadAll(io.LimitReader(file, mailboxSieveFileLimit+1))
	if err != nil {
		return nil, false, err
	}
	if len(content) > mailboxSieveFileLimit {
		// Larger than CelikPanel ever writes: the owner's.
		content = content[:mailboxSieveFileLimit]
	}
	return content, true, nil
}

func (nativeMailboxSieveStore) Read(domain, local string) (mailboxSieveState, error) {
	state := mailboxSieveState{LinkKind: "none"}
	rootFD, err := autoresponderOpenMailRoot()
	if err != nil {
		return state, err
	}
	defer unix.Close(rootFD)
	mailboxFD, err := openMailboxDirAt(rootFD, domain, local, false)
	if errors.Is(err, unix.ENOENT) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	defer unix.Close(mailboxFD)

	buffer := make([]byte, 4096)
	n, err := unix.Readlinkat(mailboxFD, autoresponderLinkName, buffer)
	switch {
	case err == nil:
		state.LinkKind = "link"
		state.LinkTarget = string(buffer[:n])
	case errors.Is(err, unix.EINVAL):
		state.LinkKind = "file"
	case errors.Is(err, unix.ENOENT):
	default:
		return state, fmt.Errorf("read %s: %w", autoresponderLinkName, err)
	}
	sieveFD, err := unix.Openat2(mailboxFD, "sieve", &unix.OpenHow{
		Flags:   uint64(unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC | unix.O_NOFOLLOW),
		Resolve: secureMaildirResolve,
	})
	if errors.Is(err, unix.ENOENT) {
		return state, nil
	}
	if err != nil {
		return state, fmt.Errorf("open the mailbox's sieve directory: %w", err)
	}
	defer unix.Close(sieveFD)
	state.ScriptContent, state.ScriptExists, err = readFileAt(sieveFD, autoresponderScriptName)
	if err != nil {
		return state, fmt.Errorf("read %s: %w", autoresponderScriptName, err)
	}
	return state, nil
}

func (nativeMailboxSieveStore) Write(domain, local string, script []byte) error {
	rootFD, err := autoresponderOpenMailRoot()
	if err != nil {
		return err
	}
	defer unix.Close(rootFD)
	mailboxFD, err := openMailboxDirAt(rootFD, domain, local, true)
	if err != nil {
		return err
	}
	defer unix.Close(mailboxFD)
	if _, err := mkdirMaildirAt(mailboxFD, "sieve", 5000, 5000); err != nil {
		return err
	}
	sieveFD, err := unix.Openat2(mailboxFD, "sieve", &unix.OpenHow{
		Flags:   uint64(unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC | unix.O_NOFOLLOW),
		Resolve: secureMaildirResolve,
	})
	if err != nil {
		return fmt.Errorf("open the mailbox's sieve directory: %w", err)
	}
	defer unix.Close(sieveFD)

	temporary := "." + autoresponderScriptName + ".celikpanel-tmp"
	_ = unix.Unlinkat(sieveFD, temporary, 0)
	fd, err := unix.Openat2(sieveFD, temporary, &unix.OpenHow{
		Flags:   uint64(unix.O_WRONLY | unix.O_CREAT | unix.O_EXCL | unix.O_CLOEXEC | unix.O_NOFOLLOW),
		Mode:    0o600,
		Resolve: secureMaildirResolve,
	})
	if err != nil {
		return fmt.Errorf("create the Sieve script: %w", err)
	}
	file := os.NewFile(uintptr(fd), temporary)
	_, writeErr := file.Write(script)
	if writeErr == nil {
		writeErr = unix.Fchown(fd, 5000, 5000)
	}
	if writeErr == nil {
		writeErr = unix.Fchmod(fd, 0o600)
	}
	if writeErr == nil {
		writeErr = file.Sync()
	}
	closeErr := file.Close()
	if writeErr == nil {
		writeErr = closeErr
	}
	if writeErr == nil {
		writeErr = unix.Renameat(sieveFD, temporary, sieveFD, autoresponderScriptName)
	}
	if writeErr != nil {
		_ = unix.Unlinkat(sieveFD, temporary, 0)
		return fmt.Errorf("write the Sieve script: %w", writeErr)
	}
	// A stale compiled copy is recompiled by Dovecot anyway; removing it
	// keeps the directory honest.
	_ = unix.Unlinkat(sieveFD, autoresponderCompiledName, 0)

	linkTemporary := autoresponderLinkName + ".celikpanel-tmp"
	_ = unix.Unlinkat(mailboxFD, linkTemporary, 0)
	if err := unix.Symlinkat(autoresponderLinkTarget, mailboxFD, linkTemporary); err != nil {
		return fmt.Errorf("activate the Sieve script: %w", err)
	}
	if err := unix.Fchownat(mailboxFD, linkTemporary, 5000, 5000, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		_ = unix.Unlinkat(mailboxFD, linkTemporary, 0)
		return fmt.Errorf("activate the Sieve script: %w", err)
	}
	if err := unix.Renameat(mailboxFD, linkTemporary, mailboxFD, autoresponderLinkName); err != nil {
		_ = unix.Unlinkat(mailboxFD, linkTemporary, 0)
		return fmt.Errorf("activate the Sieve script: %w", err)
	}
	return unix.Fsync(mailboxFD)
}

// Remove deletes CelikPanel's active link and script only. The caller has
// classified the state first; the link is removed only while it still names
// CelikPanel's script.
func (nativeMailboxSieveStore) Remove(domain, local string) error {
	rootFD, err := autoresponderOpenMailRoot()
	if err != nil {
		return err
	}
	defer unix.Close(rootFD)
	mailboxFD, err := openMailboxDirAt(rootFD, domain, local, false)
	if errors.Is(err, unix.ENOENT) {
		return nil
	}
	if err != nil {
		return err
	}
	defer unix.Close(mailboxFD)
	buffer := make([]byte, 4096)
	if n, err := unix.Readlinkat(mailboxFD, autoresponderLinkName, buffer); err == nil && string(buffer[:n]) == autoresponderLinkTarget {
		if err := unix.Unlinkat(mailboxFD, autoresponderLinkName, 0); err != nil && !errors.Is(err, unix.ENOENT) {
			return fmt.Errorf("deactivate the Sieve script: %w", err)
		}
	}
	sieveFD, err := unix.Openat2(mailboxFD, "sieve", &unix.OpenHow{
		Flags:   uint64(unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC | unix.O_NOFOLLOW),
		Resolve: secureMaildirResolve,
	})
	if errors.Is(err, unix.ENOENT) {
		return nil
	}
	if err != nil {
		return err
	}
	defer unix.Close(sieveFD)
	for _, name := range []string{autoresponderScriptName, autoresponderCompiledName} {
		if err := unix.Unlinkat(sieveFD, name, 0); err != nil && !errors.Is(err, unix.ENOENT) {
			return fmt.Errorf("remove %s: %w", name, err)
		}
	}
	return nil
}
