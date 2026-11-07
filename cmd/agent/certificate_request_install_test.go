package main

import (
	"crypto"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.128 ops, G31 follow-up: the issued certificate is validated and
// installed with the request's key, which the Agent reads from its own file.

func makeRequestForInstall(t *testing.T, token string) transport.CertificateRequestCreateResponse {
	t.Helper()
	var response transport.CertificateRequestCreateResponse
	if err := (&Agent{}).CertificateRequestCreateV1(&transport.CertificateRequestCreateRequest{
		Token: token, CommonName: "shop.example.com", Names: []string{"shop.example.com"}, KeyType: transport.CertificateKeyECDSAP256,
	}, &response); err != nil || response.Error != "" {
		t.Fatalf("create: %v %s", err, response.Error)
	}
	return response
}

// selfSignedFor issues a certificate for the request's key (as an authority
// would), reading the key the way only a test in this package can.
func selfSignedFor(t *testing.T, keyFile string) string {
	t.Helper()
	keyPEM, err := os.ReadFile(keyFile)
	if err != nil {
		t.Fatal(err)
	}
	key, err := parsePrivateKey(string(keyPEM))
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(42), Subject: pkix.Name{CommonName: "shop.example.com"}, DNSNames: []string{"shop.example.com"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(90 * 24 * time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key.(crypto.Signer))
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

func TestRequestKeyIsReadOnlyFromItsOwnSafeFile(t *testing.T) {
	root := useCertificateRequestRoot(t)
	token := strings.Repeat("cd", 16)
	made := makeRequestForInstall(t, token)

	key, err := loadCertificateRequestKey(token, made.PublicKeySHA256)
	if err != nil || !strings.Contains(key, "PRIVATE KEY") {
		t.Fatalf("load: %v", err)
	}
	// Another fingerprint: refused (the file is not this request's key).
	if _, err := loadCertificateRequestKey(token, strings.Repeat("0", 64)); err == nil {
		t.Fatal("a key with another fingerprint was used")
	}
	// A key sent together with a request is refused.
	content := "-----BEGIN PRIVATE KEY-----"
	if err := useCertificateRequestKey(token, made.PublicKeySHA256, &content); err == nil {
		t.Fatal("a sent key and a request were accepted together")
	}
	// No request: the sent key is used as before.
	content = "sent"
	if err := useCertificateRequestKey("", "", &content); err != nil || content != "sent" {
		t.Fatal("an ordinary upload changed")
	}
	// A readable-by-others file is refused.
	if err := os.Chmod(made.KeyFile, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadCertificateRequestKey(token, made.PublicKeySHA256); err == nil {
		t.Fatal("a 0644 key file was used")
	}
	_ = os.Chmod(made.KeyFile, 0o600)
	// A symbolic link in place of the key file is never followed.
	other := strings.Repeat("ef", 16)
	if err := os.Symlink(made.KeyFile, filepath.Join(root, other+".key")); err != nil {
		t.Skip("symbolic links are not available here")
	}
	if _, err := loadCertificateRequestKey(other, made.PublicKeySHA256); err == nil {
		t.Fatal("a symbolic link was followed")
	}
	// A path-like token is refused before anything is opened.
	if _, err := loadCertificateRequestKey("../"+token, made.PublicKeySHA256); err == nil {
		t.Fatal("a path-like token was accepted")
	}
}

func TestValidateCertificateUsesTheRequestKeyAndRefusesAnotherKey(t *testing.T) {
	useCertificateRequestRoot(t)
	token := strings.Repeat("12", 16)
	made := makeRequestForInstall(t, token)
	certPEM := selfSignedFor(t, made.KeyFile)

	var response ValidateCertResponse
	if err := (&Agent{}).ValidateCertificate(ValidateCertRequest{
		CertContent: certPEM, Domain: "shop.example.com", RequestKeyToken: token, RequestKeySHA256: made.PublicKeySHA256,
	}, &response); err != nil {
		t.Fatal(err)
	}
	if !response.Valid {
		t.Fatalf("the request's own certificate was refused: %s", response.Error)
	}
	if strings.Contains(response.Error, "PRIVATE KEY") {
		t.Fatal("key material in the answer")
	}

	// A certificate for another request's key: the key does not match.
	second := strings.Repeat("34", 16)
	other := makeRequestForInstall(t, second)
	response = ValidateCertResponse{}
	if err := (&Agent{}).ValidateCertificate(ValidateCertRequest{
		CertContent: selfSignedFor(t, other.KeyFile), Domain: "shop.example.com", RequestKeyToken: token, RequestKeySHA256: made.PublicKeySHA256,
	}, &response); err != nil {
		t.Fatal(err)
	}
	if response.Valid {
		t.Fatal("a certificate for another key was accepted with this request's key")
	}
}
