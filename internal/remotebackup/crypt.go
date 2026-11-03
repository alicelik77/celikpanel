package remotebackup

import (
	"crypto"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/openpgp"
	pgperrors "golang.org/x/crypto/openpgp/errors"
	"golang.org/x/crypto/openpgp/packet"
)

// The encryption of a remote backup is OpenPGP symmetric encryption
// (RFC 4880): a passphrase stretched by the iterated and salted S2K with
// SHA-256, AES-256 in an integrity-protected packet (MDC). It is exactly what
// `gpg --symmetric --cipher-algo AES256` writes, so the distribution's own
// gpg (package "gnupg" in Debian and Ubuntu main, "gnupg" in Arch core)
// opens it without CelikPanel:
//
//	gpg --batch --pinentry-mode loopback --passphrase-file pass.txt \
//	    --output site.cpbak --decrypt site.cpbak.gpg
//
// No compression is applied: the backup package is already gzip-compressed.
//
// Uzak yedeğin şifrelemesi OpenPGP simetrik şifrelemedir (AES-256, bütünlük
// korumalı). Dağıtımın kendi gpg aracı CelikPanel olmadan açar.

// MinPassphraseLength is the shortest passphrase a destination accepts.
const MinPassphraseLength = 12

// MaxPassphraseLength bounds what is stretched and stored.
const MaxPassphraseLength = 1024

// s2kCount is the S2K iteration count (bytes hashed): the largest value the
// format encodes, the same order as gpg's own default.
const s2kCount = 65011712

// EncryptedSuffix is the file name ending of an encrypted backup object.
const EncryptedSuffix = ".gpg"

// FormatName names the object format in the side file.
const FormatName = "openpgp-symmetric-aes256-mdc"

// ValidPassphrase checks length and encoding; it never echoes the value.
func ValidPassphrase(passphrase string) error {
	if !utf8.ValidString(passphrase) || strings.ContainsRune(passphrase, 0) ||
		strings.ContainsAny(passphrase, "\r\n") {
		return newError(KindConfig, "passphrase", "the passphrase must be one line of text", nil)
	}
	if len(passphrase) < MinPassphraseLength {
		return newError(KindConfig, "passphrase", "the passphrase must be at least 12 characters long", nil)
	}
	if len(passphrase) > MaxPassphraseLength {
		return newError(KindConfig, "passphrase", "the passphrase is too long", nil)
	}
	if strings.TrimSpace(passphrase) != passphrase {
		return newError(KindConfig, "passphrase", "the passphrase must not start or end with a space", nil)
	}
	return nil
}

func pgpConfig() *packet.Config {
	return &packet.Config{
		DefaultCipher:          packet.CipherAES256,
		DefaultHash:            crypto.SHA256,
		DefaultCompressionAlgo: packet.CompressionNone,
		S2KCount:               s2kCount,
	}
}

// Encrypt returns a writer: what is written to it is encrypted into dst.
// Close finishes the message (it does not close dst).
func Encrypt(dst io.Writer, passphrase, fileName string) (io.WriteCloser, error) {
	if err := ValidPassphrase(passphrase); err != nil {
		return nil, err
	}
	hints := &openpgp.FileHints{IsBinary: true, FileName: fileName}
	w, err := openpgp.SymmetricallyEncrypt(dst, []byte(passphrase), hints, pgpConfig())
	if err != nil {
		return nil, newError(KindConfig, "encrypt", "the backup could not be encrypted", nil)
	}
	return w, nil
}

// Decrypt returns the plain content of an encrypted object. The integrity
// check (MDC) runs when the returned reader reaches the end: a caller must
// read to io.EOF and treat any error before that as "not verified". A wrong
// passphrase is reported at once as KindDecrypt.
func Decrypt(src io.Reader, passphrase string) (io.Reader, error) {
	tried := false
	prompt := func(keys []openpgp.Key, symmetric bool) ([]byte, error) {
		if !symmetric || tried {
			return nil, errWrongPassphrase
		}
		tried = true
		return []byte(passphrase), nil
	}
	md, err := openpgp.ReadMessage(src, nil, prompt, pgpConfig())
	if err != nil {
		if errors.Is(err, errWrongPassphrase) || errors.Is(err, pgperrors.ErrKeyIncorrect) {
			return nil, newError(KindDecrypt, "decrypt", "the passphrase does not open this backup", nil)
		}
		return nil, newError(KindDecrypt, "decrypt", "the object is not a readable encrypted backup", nil)
	}
	if md.IsEncrypted && !md.IsSymmetricallyEncrypted {
		return nil, newError(KindDecrypt, "decrypt", "the object is not encrypted with a passphrase", nil)
	}
	if !md.IsEncrypted {
		return nil, newError(KindDecrypt, "decrypt", "the object is not encrypted", nil)
	}
	return &decryptReader{r: md.UnverifiedBody}, nil
}

var errWrongPassphrase = errors.New("wrong passphrase")

// decryptReader maps the end-of-message integrity failure to KindDecrypt.
type decryptReader struct{ r io.Reader }

func (d *decryptReader) Read(p []byte) (int, error) {
	n, err := d.r.Read(p)
	if err != nil && !errors.Is(err, io.EOF) {
		return n, newError(KindDecrypt, "decrypt", "the backup is damaged: its integrity check failed", nil)
	}
	return n, err
}

// PassphraseCheck is a short, non-reversible tag of a passphrase under a
// destination-specific salt: shown on screen so a person can tell which
// passphrase a destination uses without the passphrase ever being shown.
func PassphraseCheck(salt, passphrase string) string {
	mac := hmac.New(sha256.New, []byte("celikpanel-remote-backup-passphrase-check/v1\x00"+salt))
	mac.Write([]byte(passphrase))
	sum := hex.EncodeToString(mac.Sum(nil))
	return strings.ToUpper(sum[:4] + "-" + sum[4:8])
}
