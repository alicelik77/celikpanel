package remotebackup

import (
	"bytes"
	"crypto/rand"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const testPassphrase = "correct horse battery staple"

func encryptForTest(t *testing.T, plain []byte) []byte {
	t.Helper()
	var out bytes.Buffer
	w, err := Encrypt(&out, testPassphrase, "x.cpbak")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(plain); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestEncryptionRoundTrip(t *testing.T) {
	plain := make([]byte, 300<<10)
	rand.Read(plain)
	cipher := encryptForTest(t, plain)
	if bytes.Contains(cipher, plain[:64]) {
		t.Fatal("plain text visible in the object")
	}
	r, err := Decrypt(bytes.NewReader(cipher), testPassphrase)
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatal("round trip differs")
	}
}

func TestDecryptRefusesWrongPassphraseAndDamage(t *testing.T) {
	cipher := encryptForTest(t, bytes.Repeat([]byte("backup "), 5000))
	if _, err := Decrypt(bytes.NewReader(cipher), "another passphrase!"); KindOf(err) != KindDecrypt {
		t.Fatalf("wrong passphrase: %v", err)
	}
	damaged := append([]byte(nil), cipher...)
	damaged[len(damaged)/2] ^= 0x01
	r, err := Decrypt(bytes.NewReader(damaged), testPassphrase)
	if err == nil {
		_, err = io.ReadAll(r)
	}
	if KindOf(err) != KindDecrypt {
		t.Fatalf("damaged object was accepted: %v", err)
	}
}

// The object is the OpenPGP format gpg reads: a version 4 symmetric-key
// session key packet (tag 3) with AES-256 and an iterated, salted SHA-256
// S2K, followed by an integrity-protected data packet (tag 18, version 1).
func TestEncryptionFormatIsStandardOpenPGP(t *testing.T) {
	cipher := encryptForTest(t, []byte("hello"))
	tag, body, rest := readPacket(t, cipher)
	if tag != 3 {
		t.Fatalf("first packet tag %d, want 3 (SKESK)", tag)
	}
	if body[0] != 4 || body[1] != 9 || body[2] != 3 || body[3] != 8 {
		t.Fatalf("SKESK version/cipher/s2k/hash = %d/%d/%d/%d, want 4/9(AES256)/3(iterated+salted)/8(SHA256)", body[0], body[1], body[2], body[3])
	}
	tag, body, _ = readPacket(t, rest)
	if tag != 18 || body[0] != 1 {
		t.Fatalf("second packet tag %d version %d, want 18 version 1 (SEIPD with MDC)", tag, body[0])
	}
}

// readPacket reads one new-format packet header (what x/crypto writes);
// partial lengths are followed only as far as the first chunk.
func readPacket(t *testing.T, b []byte) (tag int, body, rest []byte) {
	t.Helper()
	if len(b) < 2 || b[0]&0x80 == 0 {
		t.Fatal("not an OpenPGP packet")
	}
	if b[0]&0x40 == 0 { // old format
		tag = int(b[0]>>2) & 0x0f
		switch b[0] & 3 {
		case 0:
			n := int(b[1])
			return tag, b[2 : 2+n], b[2+n:]
		case 1:
			n := int(b[1])<<8 | int(b[2])
			return tag, b[3 : 3+n], b[3+n:]
		}
		t.Fatal("unexpected old-format length")
	}
	tag = int(b[0] & 0x3f)
	l := int(b[1])
	switch {
	case l < 192:
		return tag, b[2 : 2+l], b[2+l:]
	case l < 224:
		n := (l-192)<<8 + int(b[2]) + 192
		return tag, b[3 : 3+n], b[3+n:]
	case l == 255:
		n := int(b[2])<<24 | int(b[3])<<16 | int(b[4])<<8 | int(b[5])
		return tag, b[6 : 6+n], b[6+n:]
	default: // partial body length: return the first chunk
		n := 1 << (l & 0x1f)
		return tag, b[2 : 2+n], nil
	}
}

// When the distribution's gpg is installed (it is on Debian, Ubuntu and
// Arch by default or from the official repositories), the documented command
// opens the object.
func TestEncryptionDecryptsWithGPG(t *testing.T) {
	gpg, err := exec.LookPath("gpg")
	if runtime.GOOS == "windows" {
		t.Skip("the documented command is for Linux; Git for Windows' gpg rewrites paths")
	}
	if err != nil {
		t.Skip("gpg is not installed here; the packet format is checked by TestEncryptionFormatIsStandardOpenPGP")
	}
	dir := t.TempDir()
	plain := make([]byte, 70<<10)
	rand.Read(plain)
	enc := filepath.Join(dir, "x.cpbak.gpg")
	pass := filepath.Join(dir, "pass.txt")
	out := filepath.Join(dir, "x.cpbak")
	if err := os.WriteFile(enc, encryptForTest(t, plain), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pass, []byte(testPassphrase+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(gpg, "--homedir", dir, "--batch", "--pinentry-mode", "loopback",
		"--passphrase-file", pass, "--output", out, "--decrypt", enc)
	done := make(chan struct{})
	var combined []byte
	go func() { combined, err = cmd.CombinedOutput(); close(done) }()
	select {
	case <-done:
	case <-time.After(60 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("gpg did not finish")
	}
	if err != nil {
		t.Fatalf("gpg: %v\n%s", err, combined)
	}
	got, err := os.ReadFile(out)
	if err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("gpg output differs: %v", err)
	}
}

func TestPassphraseRules(t *testing.T) {
	for _, bad := range []string{"short", " leading space pass", "two\nlines passphrase", strings.Repeat("x", 2000)} {
		if err := ValidPassphrase(bad); KindOf(err) != KindConfig || strings.Contains(err.Error(), bad) {
			t.Errorf("%q: %v", bad, err)
		}
	}
	if err := ValidPassphrase(testPassphrase); err != nil {
		t.Fatal(err)
	}
	a, b := PassphraseCheck("salt", testPassphrase), PassphraseCheck("salt", "other passphrase")
	if a == b || len(a) != 9 || strings.Contains(a, "horse") {
		t.Fatalf("checks %q %q", a, b)
	}
}
