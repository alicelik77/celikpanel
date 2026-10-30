//go:build linux

package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Automatic DKIM (30 Oct 2026): a key CelikPanel makes carries an origin
// record; only such a key is removed with its domain. Everything else is kept
// and named (D-022, invariant 1).

func removeDKIMForTest(t *testing.T, domain string) transport.DKIMRemoveResponse {
	t.Helper()
	var resp transport.DKIMRemoveResponse
	if err := (&Agent{}).RemoveDKIMKey(&transport.DKIMRemoveRequest{Domain: domain, Selector: signingSelector}, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error != "" {
		t.Fatalf("remove: %s", resp.Error)
	}
	return resp
}

func TestEnsureDKIMKeyRecordsItsOriginAndRemovalTakesOnlyThatKey(t *testing.T) {
	base := withTestDKIMBaseDir(t)
	agent := &Agent{}
	var made DKIMEnsureResponse
	if err := agent.EnsureDKIMKey(&DKIMEnsureRequest{Domain: "site.example", Selector: signingSelector}, &made); err != nil || made.Error != "" || !made.Created {
		t.Fatalf("ensure: %v %+v", err, made)
	}
	keyPath := filepath.Join(base, "site.example", signingSelector+".private")
	origin, err := os.ReadFile(dkimOriginPath(keyPath))
	if err != nil || !strings.HasPrefix(string(origin), dkimOriginPrefix) {
		t.Fatalf("origin record = %q, %v", origin, err)
	}
	if info, err := os.Stat(dkimOriginPath(keyPath)); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("origin record mode: %v %v", info, err)
	}
	// A second ensure keeps the key and its origin record.
	var again DKIMEnsureResponse
	if err := agent.EnsureDKIMKey(&DKIMEnsureRequest{Domain: "site.example", Selector: signingSelector}, &again); err != nil || again.Created || again.PublicKeyB64 != made.PublicKeyB64 {
		t.Fatalf("idempotent ensure: %v %+v", err, again)
	}

	resp := removeDKIMForTest(t, "site.example")
	if !resp.Removed || resp.KeptKey != "" || resp.Absent {
		t.Fatalf("removal answer: %+v", resp)
	}
	for _, path := range []string{keyPath, dkimOriginPath(keyPath), filepath.Dir(keyPath)} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("%s still exists after removal: %v", path, err)
		}
	}
	// Removing again is a clean no-op.
	if resp := removeDKIMForTest(t, "site.example"); !resp.Absent || resp.Removed {
		t.Fatalf("second removal: %+v", resp)
	}
}

func writeOwnerDKIMKeyForTest(t *testing.T, base, domain string) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(base, domain)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, signingSelector+".private")
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	if err := os.WriteFile(path, pemBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRemoveDKIMKeyKeepsAKeyCelikPanelDidNotMake(t *testing.T) {
	base := withTestDKIMBaseDir(t)
	keyPath := writeOwnerDKIMKeyForTest(t, base, "owner.example")
	before, _ := os.ReadFile(keyPath)

	// The owner's key is kept by "Generate" (never replaced)...
	var ensure DKIMEnsureResponse
	if err := (&Agent{}).EnsureDKIMKey(&DKIMEnsureRequest{Domain: "owner.example", Selector: signingSelector}, &ensure); err != nil || ensure.Error != "" || ensure.Created {
		t.Fatalf("ensure over owner key: %v %+v", err, ensure)
	}
	if _, err := os.Lstat(dkimOriginPath(keyPath)); !os.IsNotExist(err) {
		t.Fatal("an origin record was written for a key CelikPanel did not make")
	}
	// ...and by the domain's deletion.
	resp := removeDKIMForTest(t, "owner.example")
	if resp.Removed || resp.KeptKey != keyPath {
		t.Fatalf("owner key removal answer: %+v", resp)
	}
	after, err := os.ReadFile(keyPath)
	if err != nil || string(after) != string(before) {
		t.Fatalf("owner key changed: %v", err)
	}
}

func TestRemoveDKIMKeyKeepsAKeyWhoseOriginNamesAnotherKey(t *testing.T) {
	base := withTestDKIMBaseDir(t)
	var made DKIMEnsureResponse
	if err := (&Agent{}).EnsureDKIMKey(&DKIMEnsureRequest{Domain: "swap.example", Selector: signingSelector}, &made); err != nil || made.Error != "" {
		t.Fatalf("ensure: %v %+v", err, made)
	}
	// The owner replaced CelikPanel's key with their own; the old origin
	// record no longer names the key on disk.
	keyPath := filepath.Join(base, "swap.example", signingSelector+".private")
	if err := os.Remove(keyPath); err != nil {
		t.Fatal(err)
	}
	writeOwnerDKIMKeyForTest(t, base, "swap.example")
	resp := removeDKIMForTest(t, "swap.example")
	if resp.Removed || resp.KeptKey != keyPath {
		t.Fatalf("swapped key answer: %+v", resp)
	}
	if _, err := os.Stat(keyPath); err != nil {
		t.Fatalf("swapped key removed: %v", err)
	}
}

func TestRemoveDKIMKeyLeavesOwnerFilesInTheDomainDirectory(t *testing.T) {
	base := withTestDKIMBaseDir(t)
	var made DKIMEnsureResponse
	if err := (&Agent{}).EnsureDKIMKey(&DKIMEnsureRequest{Domain: "notes.example", Selector: signingSelector}, &made); err != nil || made.Error != "" {
		t.Fatalf("ensure: %v %+v", err, made)
	}
	note := filepath.Join(base, "notes.example", "README.owner")
	if err := os.WriteFile(note, []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}
	if resp := removeDKIMForTest(t, "notes.example"); !resp.Removed {
		t.Fatalf("removal answer: %+v", resp)
	}
	if _, err := os.Stat(note); err != nil {
		t.Fatalf("owner file removed: %v", err)
	}
}

func TestRemoveDKIMKeyRefusesInvalidNames(t *testing.T) {
	withTestDKIMBaseDir(t)
	var resp transport.DKIMRemoveResponse
	if err := (&Agent{}).RemoveDKIMKey(&transport.DKIMRemoveRequest{Domain: "../etc", Selector: signingSelector}, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error == "" || resp.Removed {
		t.Fatalf("invalid domain accepted: %+v", resp)
	}
}

func TestRenderDKIMTablesKeepsOwnerLinesAndRegeneratesOurs(t *testing.T) {
	base := withTestDKIMBaseDir(t)
	ourKey := func(domain string) string {
		line, _, err := dkimTableLines(domain)
		if err != nil {
			t.Fatal(err)
		}
		return line
	}
	_, ourSigning, _ := dkimTableLines("gone.example")
	existingKeys := strings.Join([]string{
		ourKey("gone.example"),
		"# owner: my own selector",
		"mine._domainkey.shop.example shop.example:mine:/etc/opendkim/keys/shop.private",
		"",
	}, "\n")
	existingSigning := strings.Join([]string{
		ourSigning,
		"*@shop.example mine._domainkey.shop.example",
	}, "\n")
	kt, st, owner, err := renderDKIMTables([]byte(existingKeys), []byte(existingSigning), []string{"site.example"})
	if err != nil {
		t.Fatal(err)
	}
	if owner != 3 {
		t.Fatalf("owner lines = %d, want 3", owner)
	}
	wantKeys := ourKey("site.example") + "\n# owner: my own selector\nmine._domainkey.shop.example shop.example:mine:/etc/opendkim/keys/shop.private\n"
	if string(kt) != wantKeys {
		t.Fatalf("key table:\n%s\nwant:\n%s", kt, wantKeys)
	}
	_, siteSigning, _ := dkimTableLines("site.example")
	if string(st) != siteSigning+"\n*@shop.example mine._domainkey.shop.example\n" {
		t.Fatalf("signing table:\n%s", st)
	}
	if strings.Contains(string(kt), "gone.example") || strings.Contains(string(st), "gone.example") {
		t.Fatal("a line CelikPanel wrote for a domain without a key survived")
	}
	if !strings.Contains(ourKey("site.example"), base) {
		t.Fatal("key line does not name the key store")
	}
	// Rendering again from its own output changes nothing.
	kt2, st2, _, err := renderDKIMTables(kt, st, []string{"site.example"})
	if err != nil || string(kt2) != string(kt) || string(st2) != string(st) {
		t.Fatalf("render is not stable: %v", err)
	}
}

func TestDKIMLineNamesDomainMatchesWholeNamesOnly(t *testing.T) {
	for line, want := range map[string]bool{
		"*@shop.example mine._domainkey.shop.example":                            true,
		"mine._domainkey.shop.example shop.example:mine:/etc/k.private":          true,
		"*@myshop.example mine._domainkey.myshop.example":                        false,
		"*@sub.shop.example mine._domainkey.sub.shop.example":                    false,
		"# a comment about shop.example":                                         true,
		"other._domainkey.example.org example.org:other:/etc/opendkim/k.private": false,
	} {
		if got := dkimLineNamesDomain(line, "shop.example"); got != want {
			t.Errorf("%q names shop.example = %t, want %t", line, got, want)
		}
	}
}
