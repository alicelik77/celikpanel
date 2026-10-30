package main

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// alpha.113: a certificate this server's clock does not accept yet is
// reported as not yet valid, with the time it was checked, so the Panel can
// wait for a few seconds of clock difference instead of discarding it.
func TestGetCertificateInfoSaysNotYetValidAndExpiredApart(t *testing.T) {
	certPEM, _ := makeCertificatePair(t, "ecdsa", []string{"example.test"})
	path := filepath.Join(t.TempDir(), "fullchain.pem")
	if err := os.WriteFile(path, []byte(certPEM), 0o600); err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode([]byte(certPEM))
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	old := certificateClockNow
	t.Cleanup(func() { certificateClockNow = old })

	early := leaf.NotBefore.Add(-3 * time.Second)
	certificateClockNow = func() time.Time { return early }
	var resp ValidateCertResponse
	if err := (&Agent{}).GetCertificateInfo(path, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Valid || !resp.NotYetValid || resp.Expired || !resp.CheckedAt.Equal(early.UTC()) ||
		!resp.IssuedAt.Equal(leaf.NotBefore) || !strings.Contains(resp.Error, "not yet valid") {
		t.Fatalf("not yet valid reading = %+v", resp)
	}

	late := leaf.NotAfter.Add(time.Minute)
	certificateClockNow = func() time.Time { return late }
	resp = ValidateCertResponse{}
	if err := (&Agent{}).GetCertificateInfo(path, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Valid || resp.NotYetValid || !resp.Expired || !strings.Contains(resp.Error, "expired") {
		t.Fatalf("expired reading = %+v", resp)
	}

	certificateClockNow = old
	resp = ValidateCertResponse{}
	if err := (&Agent{}).GetCertificateInfo(path, &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Valid || resp.NotYetValid || resp.Expired || resp.CheckedAt.IsZero() {
		t.Fatalf("valid reading = %+v", resp)
	}
}
