//go:build linux

package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/openpgp"
	"golang.org/x/crypto/openpgp/armor"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.107 (D-038 §9): Roundcube's update path. The newest release on the
// installed line is offered; an archive is installed only with a valid
// signature by the pinned key; the configuration and database are carried
// over; a webmail that does not answer on the new tree gets the previous
// tree back byte for byte.

const roundcubeReleasesSample = `[
 {"tag_name":"1.7.5","draft":false,"prerelease":false},
 {"tag_name":"1.6.20","draft":false,"prerelease":false},
 {"tag_name":"1.7.4","draft":false,"prerelease":false},
 {"tag_name":"1.6.21","draft":true,"prerelease":false},
 {"tag_name":"1.7-rc6","draft":false,"prerelease":true},
 {"tag_name":"1.6.19","draft":false,"prerelease":false},
 {"tag_name":"1.5.15","draft":false,"prerelease":false}
]`

func TestTheNewestReleaseOnTheInstalledLineIsOffered(t *testing.T) {
	onLine, overall, err := roundcubeReleaseChoice([]byte(roundcubeReleasesSample), "1.6.15")
	if err != nil || onLine != "1.6.20" || overall != "1.7.5" {
		t.Fatalf("onLine=%q overall=%q err=%v", onLine, overall, err)
	}
	if onLine, _, _ := roundcubeReleaseChoice([]byte(roundcubeReleasesSample), "1.5.15"); onLine != "1.5.15" {
		t.Fatalf("1.5 line: %q", onLine)
	}
	if !roundcubeVersionLess("1.6.9", "1.6.20") || roundcubeVersionLess("1.6.20", "1.6.20") {
		t.Fatal("version order")
	}
}

func TestTheEmbeddedSigningKeyIsThePinnedOneAndAnotherKeysSignatureIsRefused(t *testing.T) {
	keyring, err := openpgp.ReadArmoredKeyRing(strings.NewReader(roundcubeSigningKey))
	if err != nil || len(keyring) != 1 {
		t.Fatalf("keyring: %v", err)
	}
	other, err := openpgp.NewEntity("someone", "", "someone@example.test", nil)
	if err != nil {
		t.Fatal(err)
	}
	archive := []byte("not roundcube")
	var signature bytes.Buffer
	if err := openpgp.ArmoredDetachSign(&signature, other, bytes.NewReader(archive), nil); err != nil {
		t.Fatal(err)
	}
	if err := verifyRoundcubeSignature(bytes.NewReader(archive), signature.Bytes(), roundcubeSigningKey, roundcubeSigningPrimary); err == nil {
		t.Fatal("a signature by another key was accepted")
	}
	// The other key's own signature verifies against the other key, but not
	// as Roundcube's: the pinned fingerprint decides.
	var otherKey bytes.Buffer
	w, _ := armor.Encode(&otherKey, openpgp.PublicKeyType, nil)
	other.Serialize(w)
	w.Close()
	if err := verifyRoundcubeSignature(bytes.NewReader(archive), signature.Bytes(), otherKey.String(), roundcubeSigningPrimary); err == nil {
		t.Fatal("a keyring with another primary key was accepted")
	}
}

// The real 1.6.20 archive and signature, when a copy is given (the disposable
// VM run and a developer's check; the files are not in the repository).
func TestTheReal1620ArchiveVerifiesWithThePinnedKey(t *testing.T) {
	dir := os.Getenv("CELIKPANEL_RC_RELEASE_DIR")
	if dir == "" {
		t.Skip("CELIKPANEL_RC_RELEASE_DIR names a directory with rc1620.tar.gz and rc1620.asc")
	}
	archive, err := os.Open(filepath.Join(dir, "rc1620.tar.gz"))
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	signature, err := os.ReadFile(filepath.Join(dir, "rc1620.asc"))
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyRoundcubeSignature(archive, signature, roundcubeSigningKey, roundcubeSigningPrimary); err != nil {
		t.Fatal(err)
	}
	if _, err := archive.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	tampered, _ := io.ReadAll(archive)
	tampered[len(tampered)/2] ^= 0xff
	if err := verifyRoundcubeSignature(bytes.NewReader(tampered), signature, roundcubeSigningKey, roundcubeSigningPrimary); err == nil {
		t.Fatal("a changed archive verified")
	}
}

func writeRoundcubeTree(t *testing.T, root, version, marker string) {
	t.Helper()
	for _, dir := range []string{"program/include", "config", "db", "temp", "logs", "plugins/archive", "public_html", "bin", "SQL"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		"program/include/iniset.php": "<?php define('RCMAIL_VERSION', '" + version + "');\n",
		"public_html/index.php":      "<?php // " + marker + "\n",
		"config/defaults.inc.php":    "<?php // defaults " + version + "\n",
	}
	for name, text := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func roundcubeUpdateFixture(t *testing.T, prove func(int) error) (string, roundcubeUpdateOps, *int) {
	t.Helper()
	parent := t.TempDir()
	live := filepath.Join(parent, "celikpanel-webmail")
	writeRoundcubeTree(t, live, "1.6.15", "old")
	os.WriteFile(filepath.Join(live, "config", "config.inc.php"), []byte("<?php $config['des_key'] = 'owner-secret';\n"), 0o640)
	os.WriteFile(filepath.Join(live, "db", "roundcube.sqlite3"), []byte("the owner's contacts"), 0o660)
	os.MkdirAll(filepath.Join(live, "plugins", "owners_plugin"), 0o755)
	os.WriteFile(filepath.Join(live, "plugins", "owners_plugin", "owners_plugin.php"), []byte("<?php\n"), 0o644)
	proofs := 0
	ops := roundcubeUpdateOps{
		baseDir: live,
		check: func(context.Context) transport.RoundcubeUpdateCheckResponse {
			return transport.RoundcubeUpdateCheckResponse{Installed: true, Version: "1.6.15", Line: "1.6", LatestOnLine: "1.6.20", UpdateAvailable: true}
		},
		download: func(_ context.Context, _ string, _ int64, w io.Writer) error {
			_, err := w.Write([]byte("archive"))
			return err
		},
		verify: func(io.Reader, []byte) error { return nil },
		extract: func(_ context.Context, _ string, dir string) error {
			writeRoundcubeTree(t, dir, "1.6.20", "new")
			return nil
		},
		updateDB:    func(context.Context, string, string) error { return nil },
		permissions: func(context.Context, string) error { return nil },
		backup: func(context.Context, string, string) (string, error) {
			return "/var/lib/celikpanel-agent-private/roundcube-backups/x.tar.gz", nil
		},
		exchange: func(a, b string) error {
			return hostRoundcubeUpdateOps().exchange(a, b)
		},
		prove:  func(context.Context) error { proofs++; return prove(proofs) },
		retire: func(path string) error { return os.RemoveAll(path) },
	}
	return live, ops, &proofs
}

func TestARoundcubeUpdateCarriesTheOwnersFilesAndIsProved(t *testing.T) {
	live, ops, _ := roundcubeUpdateFixture(t, func(int) error { return nil })
	resp := updateRoundcubeTo(context.Background(), "1.6.20", ops)
	if !resp.Updated || !resp.Proved || resp.From != "1.6.15" || resp.Backup == "" {
		t.Fatalf("resp = %+v", resp)
	}
	if got := roundcubeTreeVersion(live); got != "1.6.20" {
		t.Fatalf("live version %q", got)
	}
	for name, want := range map[string]string{
		"config/config.inc.php":                   "owner-secret",
		"db/roundcube.sqlite3":                    "the owner's contacts",
		"plugins/owners_plugin/owners_plugin.php": "<?php",
		"public_html/index.php":                   "new",
		"config/defaults.inc.php":                 "defaults 1.6.20",
	} {
		data, err := os.ReadFile(filepath.Join(live, name))
		if err != nil || !strings.Contains(string(data), want) {
			t.Fatalf("%s = %q, %v", name, data, err)
		}
	}
	if entries, _ := os.ReadDir(filepath.Dir(live)); len(entries) != 1 {
		t.Fatalf("leftovers beside the tree: %v", entries)
	}
}

func TestAWebmailThatDoesNotAnswerGetsThePreviousTreeBackByteForByte(t *testing.T) {
	live, ops, proofs := roundcubeUpdateFixture(t, func(n int) error {
		if n == 1 {
			return errors.New("webmail answered HTTP 500")
		}
		return nil
	})
	before, _ := os.ReadFile(filepath.Join(live, "public_html", "index.php"))
	resp := updateRoundcubeTo(context.Background(), "1.6.20", ops)
	if resp.Updated || !resp.RolledBack || resp.ErrorCode != "PROOF_FAILED" || *proofs != 2 || !strings.Contains(resp.Error, "answers again") {
		t.Fatalf("resp = %+v proofs=%d", resp, *proofs)
	}
	after, _ := os.ReadFile(filepath.Join(live, "public_html", "index.php"))
	if !bytes.Equal(before, after) || roundcubeTreeVersion(live) != "1.6.15" {
		t.Fatalf("the previous tree is not back: %q", after)
	}
}

func TestOnlyTheNewestReleaseOnTheLineAndAValidSignatureAreAccepted(t *testing.T) {
	live, ops, _ := roundcubeUpdateFixture(t, func(int) error { return nil })
	if resp := updateRoundcubeTo(context.Background(), "1.7.5", ops); resp.ErrorCode != "NOT_LATEST" {
		t.Fatalf("another line: %+v", resp)
	}
	ops.verify = func(io.Reader, []byte) error { return errors.New("the archive's signature is not valid") }
	resp := updateRoundcubeTo(context.Background(), "1.6.20", ops)
	if resp.ErrorCode != "SIGNATURE_INVALID" || roundcubeTreeVersion(live) != "1.6.15" {
		t.Fatalf("bad signature: %+v", resp)
	}
}
