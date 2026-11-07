package main

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.128 ops, G31: certificate requests.

func useCertificateRequestRoot(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "certificate-requests")
	previous := certificateRequestKeyRoot
	certificateRequestKeyRoot = root
	t.Cleanup(func() { certificateRequestKeyRoot = previous })
	return root
}

func TestCertificateRequestKeepsTheKeyAndAnswersOnlyTheCSR(t *testing.T) {
	root := useCertificateRequestRoot(t)
	agent := &Agent{}
	token := strings.Repeat("ab", 16)
	for _, keyType := range []string{transport.CertificateKeyECDSAP256, transport.CertificateKeyRSA2048} {
		token = token[:30] + map[string]string{transport.CertificateKeyECDSAP256: "01", transport.CertificateKeyRSA2048: "02"}[keyType]
		var response transport.CertificateRequestCreateResponse
		err := agent.CertificateRequestCreateV1(&transport.CertificateRequestCreateRequest{
			Token: token, CommonName: "shop.example.com", Names: []string{"shop.example.com", "*.shop.example.com"}, KeyType: keyType,
		}, &response)
		if err != nil || response.Error != "" {
			t.Fatalf("%s: %v %s", keyType, err, response.Error)
		}
		if strings.Contains(response.CSRPEM, "PRIVATE KEY") || response.KeyFile != filepath.Join(root, token+".key") {
			t.Fatalf("%s: answer %+v", keyType, response)
		}
		block, _ := pem.Decode([]byte(response.CSRPEM))
		csr, err := x509.ParseCertificateRequest(block.Bytes)
		if err != nil || csr.CheckSignature() != nil || csr.Subject.CommonName != "shop.example.com" || len(csr.DNSNames) != 2 {
			t.Fatalf("%s: csr %v %+v", keyType, err, csr)
		}
		sum := sha256.Sum256(csr.RawSubjectPublicKeyInfo)
		if hex.EncodeToString(sum[:]) != response.PublicKeySHA256 {
			t.Fatalf("%s: fingerprint", keyType)
		}
		info, err := os.Stat(response.KeyFile)
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("%s: key file %v %v", keyType, err, info)
		}
		keyPEM, _ := os.ReadFile(response.KeyFile)
		keyBlock, _ := pem.Decode(keyPEM)
		if keyBlock == nil || keyBlock.Type != "PRIVATE KEY" {
			t.Fatalf("%s: key file content", keyType)
		}
		if _, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes); err != nil {
			t.Fatalf("%s: key: %v", keyType, err)
		}
		// The same token never makes a second key.
		var again transport.CertificateRequestCreateResponse
		_ = agent.CertificateRequestCreateV1(&transport.CertificateRequestCreateRequest{
			Token: token, CommonName: "shop.example.com", Names: []string{"shop.example.com"}, KeyType: keyType,
		}, &again)
		if again.Error == "" || again.CSRPEM != "" {
			t.Fatalf("%s: second key: %+v", keyType, again)
		}
	}
	if info, err := os.Stat(root); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("root: %v %v", err, info)
	}

	// Discard removes it; a second discard finds nothing.
	var discard transport.CertificateRequestDiscardResponse
	if err := agent.CertificateRequestDiscardV1(&transport.CertificateRequestDiscardRequest{Token: token}, &discard); err != nil || !discard.Removed {
		t.Fatalf("discard: %v %+v", err, discard)
	}
	if _, err := os.Stat(filepath.Join(root, token+".key")); !os.IsNotExist(err) {
		t.Fatalf("key still there: %v", err)
	}
	discard = transport.CertificateRequestDiscardResponse{}
	if err := agent.CertificateRequestDiscardV1(&transport.CertificateRequestDiscardRequest{Token: token}, &discard); err != nil || discard.Removed || discard.Error != "" {
		t.Fatalf("second discard: %v %+v", err, discard)
	}
}

func TestCertificateRequestRefusesBadInputAndAnUnsafeDirectory(t *testing.T) {
	root := useCertificateRequestRoot(t)
	agent := &Agent{}
	token := strings.Repeat("c", 32)
	for _, request := range []transport.CertificateRequestCreateRequest{
		{Token: "../x", CommonName: "a.example", Names: []string{"a.example"}, KeyType: transport.CertificateKeyECDSAP256},
		{Token: token, CommonName: "a.example", Names: []string{"a.example"}, KeyType: "dsa-1024"},
		{Token: token, CommonName: "a.example", Names: []string{"b.example", "a.example"}, KeyType: transport.CertificateKeyECDSAP256},
		{Token: token, CommonName: "localhost", Names: []string{"localhost"}, KeyType: transport.CertificateKeyECDSAP256},
		{Token: token, CommonName: "a.example", Names: []string{"a.example", "a.example"}, KeyType: transport.CertificateKeyECDSAP256},
		{Token: token, CommonName: "a.example", Names: []string{"a.example", "x.*.example"}, KeyType: transport.CertificateKeyECDSAP256},
		{Token: token, CommonName: "a.example", Names: []string{"a.example", "bad_name.example"}, KeyType: transport.CertificateKeyECDSAP256},
	} {
		var response transport.CertificateRequestCreateResponse
		if err := agent.CertificateRequestCreateV1(&request, &response); err != nil || response.Error == "" || response.CSRPEM != "" {
			t.Fatalf("%+v: %v %+v", request, err, response)
		}
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("a refused request made the directory: %v", err)
	}
	// A directory others can read is refused, nothing is written in it.
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.Chmod(root, 0o755)
	var response transport.CertificateRequestCreateResponse
	_ = agent.CertificateRequestCreateV1(&transport.CertificateRequestCreateRequest{Token: token, CommonName: "a.example", Names: []string{"a.example"}, KeyType: transport.CertificateKeyECDSAP256}, &response)
	if response.Error == "" || response.KeyFile != "" {
		t.Fatalf("unsafe directory: %+v", response)
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 0 {
		t.Fatalf("written into an unsafe directory: %v", entries)
	}
}

func TestCertificateRequestRefusesAnotherPanelBuild(t *testing.T) {
	useCertificateRequestRoot(t)
	previous := buildCommit
	buildCommit = "agentbuild"
	t.Cleanup(func() { buildCommit = previous })
	var response transport.CertificateRequestCreateResponse
	_ = (&Agent{}).CertificateRequestCreateV1(&transport.CertificateRequestCreateRequest{ExpectedBuildCommit: "other", Token: strings.Repeat("d", 32), CommonName: "a.example", Names: []string{"a.example"}, KeyType: transport.CertificateKeyECDSAP256}, &response)
	var discard transport.CertificateRequestDiscardResponse
	_ = (&Agent{}).CertificateRequestDiscardV1(&transport.CertificateRequestDiscardRequest{ExpectedBuildCommit: "other", Token: strings.Repeat("d", 32)}, &discard)
	if response.Error == "" || response.CSRPEM != "" || discard.Error == "" {
		t.Fatalf("build gate: %+v %+v", response, discard)
	}
}
