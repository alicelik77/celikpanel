package main

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Certificate requests (alpha.128, G31). For the rare certificate bought
// from a paid authority the administrator needs a CSR made on this server.
// The private key is made here, in Go (crypto/ecdsa or crypto/rsa, no
// openssl and no shell), written once to a root-only file in the Agent's own
// private state directory (0700 directory, 0600 file, written to a new
// temporary file and linked into place, so a key file is either complete or
// absent and is never replaced), and never returned: the panel and the
// browser get only the CSR, the SHA-256 of the public key (to match the
// issued certificate later) and the file's path. Discard removes the file.
//
// The file is the owner's, plain PKCS#8 PEM: with or without CelikPanel the
// owner can use it with the issued certificate in their own configuration
// (D-022).
//
// Sertifika isteği: özel anahtar burada Go ile üretilir, kök-yalnız bir
// dosyaya bir kez yazılır ve hiç geri verilmez; panel yalnız CSR'ı ve açık
// anahtarın özetini alır.

var (
	certificateRequestKeyRoot = "/var/lib/celikpanel-agent-private/certificate-requests"
	certificateRequestToken   = regexp.MustCompile(`^[0-9a-f]{32}$`)
	certificateRequestLabel   = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
	certificateRequestMu      sync.Mutex
)

const certificateRequestMaxNames = 50

// validCertificateRequestName: a lowercase host name of at least two
// labels, or "*." and such a name.
func validCertificateRequestName(name string) bool {
	name = strings.TrimPrefix(name, "*.")
	if len(name) == 0 || len(name) > 253 || strings.Contains(name, "*") {
		return false
	}
	labels := strings.Split(name, ".")
	if len(labels) < 2 {
		return false
	}
	for _, label := range labels {
		if !certificateRequestLabel.MatchString(label) {
			return false
		}
	}
	return true
}

func validateCertificateRequestNames(commonName string, names []string) error {
	if len(names) == 0 || len(names) > certificateRequestMaxNames {
		return fmt.Errorf("a request names 1 to %d host names", certificateRequestMaxNames)
	}
	if names[0] != commonName {
		return errors.New("the common name must be the first name")
	}
	seen := map[string]bool{}
	for _, name := range names {
		if !validCertificateRequestName(name) {
			return fmt.Errorf("%q is not a host name", name)
		}
		if seen[name] {
			return fmt.Errorf("%q is named twice", name)
		}
		seen[name] = true
	}
	return nil
}

func generateCertificateRequestKey(keyType string) (crypto.Signer, error) {
	switch keyType {
	case transport.CertificateKeyECDSAP256:
		return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	case transport.CertificateKeyRSA2048:
		return rsa.GenerateKey(rand.Reader, 2048)
	case transport.CertificateKeyRSA3072:
		return rsa.GenerateKey(rand.Reader, 3072)
	case transport.CertificateKeyRSA4096:
		return rsa.GenerateKey(rand.Reader, 4096)
	}
	return nil, errors.New("unknown key type")
}

// PublicKeySHA256 is the SHA-256 of the DER SubjectPublicKeyInfo: the same
// bytes as an issued certificate's RawSubjectPublicKeyInfo.
func certificatePublicKeySHA256(public crypto.PublicKey) (string, error) {
	der, err := x509.MarshalPKIXPublicKey(public)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(der)
	return hex.EncodeToString(sum[:]), nil
}

// CertificateRequestCreateV1 makes the key and the CSR. See above.
func (a *Agent) CertificateRequestCreateV1(req *transport.CertificateRequestCreateRequest, resp *transport.CertificateRequestCreateResponse) error {
	if req == nil || resp == nil {
		return errors.New("certificate request and response are required")
	}
	*resp = transport.CertificateRequestCreateResponse{}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "making a certificate request"); err != nil {
		resp.Error = err.Error()
		return nil
	}
	if !certificateRequestToken.MatchString(req.Token) {
		resp.Error = "invalid request token"
		return nil
	}
	if !transport.ValidCertificateRequestKeyType(req.KeyType) {
		resp.Error = "unknown key type"
		return nil
	}
	if err := validateCertificateRequestNames(req.CommonName, req.Names); err != nil {
		resp.Error = err.Error()
		return nil
	}
	key, err := generateCertificateRequestKey(req.KeyType)
	if err != nil {
		resp.Error = "the key could not be made: " + err.Error()
		return nil
	}
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject:  pkix.Name{CommonName: req.CommonName},
		DNSNames: append([]string(nil), req.Names...),
	}, key)
	if err != nil {
		resp.Error = "the certificate request could not be made: " + err.Error()
		return nil
	}
	fingerprint, err := certificatePublicKeySHA256(key.Public())
	if err != nil {
		resp.Error = "the public key could not be read: " + err.Error()
		return nil
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		resp.Error = "the key could not be encoded: " + err.Error()
		return nil
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	clear(keyDER)
	path, err := writeCertificateRequestKey(req.Token, keyPEM)
	clear(keyPEM)
	if err != nil {
		resp.Error = "the key could not be saved: " + err.Error()
		return nil
	}
	resp.CSRPEM = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER}))
	resp.PublicKeySHA256 = fingerprint
	resp.KeyFile = path
	return nil
}

// CertificateRequestDiscardV1 removes a request's key file.
func (a *Agent) CertificateRequestDiscardV1(req *transport.CertificateRequestDiscardRequest, resp *transport.CertificateRequestDiscardResponse) error {
	if req == nil || resp == nil {
		return errors.New("certificate request and response are required")
	}
	*resp = transport.CertificateRequestDiscardResponse{}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "discarding a certificate request"); err != nil {
		resp.Error = err.Error()
		return nil
	}
	if !certificateRequestToken.MatchString(req.Token) {
		resp.Error = "invalid request token"
		return nil
	}
	certificateRequestMu.Lock()
	defer certificateRequestMu.Unlock()
	root, err := secureCertificateRequestRoot(false)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		resp.Error = err.Error()
		return nil
	}
	path := filepath.Join(root, req.Token+".key")
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		resp.Error = err.Error()
		return nil
	}
	if !info.Mode().IsRegular() {
		resp.Error = "the key path is not a regular file"
		return nil
	}
	if err := os.Remove(path); err != nil {
		resp.Error = err.Error()
		return nil
	}
	syncCertificateRequestDir(root)
	resp.Removed = true
	return nil
}

// secureCertificateRequestRoot makes (when create) or opens the key
// directory and proves it is a real directory, 0700, owned by this process's
// user, not a symbolic link.
func secureCertificateRequestRoot(create bool) (string, error) {
	root := filepath.Clean(certificateRequestKeyRoot)
	if !filepath.IsAbs(root) {
		return "", errors.New("the key directory is not absolute")
	}
	if create {
		if err := os.Mkdir(root, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
			return "", err
		}
	}
	info, err := os.Lstat(root)
	if err != nil {
		return "", err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("the key directory is not a directory")
	}
	if info.Mode().Perm() != 0o700 {
		return "", fmt.Errorf("the key directory has mode %o, not 700", info.Mode().Perm())
	}
	if !certificateRequestOwnedBySelf(info) {
		return "", errors.New("the key directory is not owned by the Agent's user")
	}
	return root, nil
}

func writeCertificateRequestKey(token string, content []byte) (string, error) {
	certificateRequestMu.Lock()
	defer certificateRequestMu.Unlock()
	root, err := secureCertificateRequestRoot(true)
	if err != nil {
		return "", err
	}
	final := filepath.Join(root, token+".key")
	if _, err := os.Lstat(final); err == nil {
		return "", errors.New("a key for this request already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	temporary := filepath.Join(root, "."+token+".tmp")
	_ = os.Remove(temporary)
	// O_EXCL with O_CREATE never follows a symbolic link.
	file, err := os.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", err
	}
	_, writeErr := file.Write(content)
	syncErr := file.Sync()
	closeErr := file.Close()
	if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
		_ = os.Remove(temporary)
		return "", err
	}
	// A link never replaces an existing file.
	if err := os.Link(temporary, final); err != nil {
		_ = os.Remove(temporary)
		return "", err
	}
	_ = os.Remove(temporary)
	syncCertificateRequestDir(root)
	return final, nil
}

func syncCertificateRequestDir(root string) {
	if dir, err := os.Open(root); err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
}
