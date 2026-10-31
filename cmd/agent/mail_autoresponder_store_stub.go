//go:build !linux

package main

import "errors"

// nativeMailboxSieveStore needs Linux openat2; elsewhere it refuses.
type nativeMailboxSieveStore struct{}

var errMailboxSieveUnsupported = errors.New("mailbox Sieve scripts require Linux openat2")

func (nativeMailboxSieveStore) Read(string, string) (mailboxSieveState, error) {
	return mailboxSieveState{LinkKind: "none"}, nil
}

func (nativeMailboxSieveStore) Write(string, string, []byte) error {
	return errMailboxSieveUnsupported
}

func (nativeMailboxSieveStore) Remove(string, string) error { return nil }
