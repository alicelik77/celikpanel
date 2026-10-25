package main

import (
	"os"
	"testing"
	"time"
)

// alpha.108 (owner finding, Boston): the repository card said Sury's key
// "expired on 17 March 2021" while apt accepted the repository. The vendor
// extends its key with newer self-signatures (2019, 2021, 2024, 2025, newest
// first in the file); the newest valid one decides, as gpg and apt read it.
// The fixtures are the vendors' published public keys as downloaded on
// 2026-10-25.

func TestTheNewestSelfSignatureDecidesAVendorKeysExpiry(t *testing.T) {
	data, err := os.ReadFile("testdata/repo-keys/sury-apt.gpg")
	if err != nil {
		t.Fatal(err)
	}
	created, expires, ok := openPGPKeyValidity(data, false)
	if !ok {
		t.Fatal("Sury's key was not read")
	}
	if created.UTC().Format("2006-01-02") != "2019-03-18" {
		t.Fatalf("created = %s", created)
	}
	// 2019-03-18 plus the 2025-11-17 self-signature's lifetime.
	if expires.UTC().Before(time.Date(2028, 1, 1, 0, 0, 0, 0, time.UTC)) || expires.UTC().After(time.Date(2028, 6, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("expires = %s, want the extended expiry in early 2028 (not 2021-03-17)", expires.UTC())
	}
	if state := repoKeyState(expires, time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC)); state != "valid" {
		t.Fatalf("state on 2026-10-25 = %s", state)
	}
	if _, err := validateRepoPublicKey(data, "15058500A0235D97F5D10063B188E2B695BD4743"); err != nil {
		t.Fatalf("the fixture is not Sury's pinned key: %v", err)
	}
}

func TestAnArmouredVendorKeyIsReadToo(t *testing.T) {
	data, err := os.ReadFile("testdata/repo-keys/pgdg-ACCC4CF8.asc")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := validateRepoPublicKey(data, "B97B0AFCAA1A47F044F244A07FCC7D46ACCC4CF8"); err != nil {
		t.Fatalf("the fixture is not PGDG's pinned key: %v", err)
	}
	created, expires, ok := openPGPKeyValidity(data, true)
	if !ok || created.IsZero() {
		t.Fatalf("PGDG's key was not read: %s %s %v", created, expires, ok)
	}
	if !expires.IsZero() && expires.Before(time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("PGDG's key reads expired: %s", expires)
	}
	if _, _, ok := openPGPKeyValidity([]byte("<html>"), false); ok {
		t.Fatal("an HTML page was read as a key")
	}
}
