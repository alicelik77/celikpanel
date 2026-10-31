package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPostfixDefaultServerCertificateOnlyWhenNoneIsNamed(t *testing.T) {
	dir := t.TempDir()
	oldCert, oldKey, oldCreate := postfixDefaultCertPath, postfixDefaultKeyPath, postfixDefaultPairCreate
	t.Cleanup(func() {
		postfixDefaultCertPath, postfixDefaultKeyPath, postfixDefaultPairCreate = oldCert, oldKey, oldCreate
	})
	postfixDefaultCertPath, postfixDefaultKeyPath = filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	created := 0
	postfixDefaultPairCreate = func(cert, key, _ string) error {
		created++
		for _, path := range []string{cert, key} {
			if err := os.WriteFile(path, []byte("pem"), 0o600); err != nil {
				return err
			}
		}
		return nil
	}
	settings := map[string]string{}
	var calls []string
	run := func(name string, args ...string) ([]byte, error) {
		calls = append(calls, name+" "+strings.Join(args, " "))
		switch {
		case name == "postconf" && args[0] == "-h":
			var out strings.Builder
			for _, key := range args[1:] {
				out.WriteString(settings[key] + "\n")
			}
			return []byte(out.String()), nil
		case name == "postconf" && args[0] == "-e":
			key, value, _ := strings.Cut(args[1], "=")
			settings[key] = value
			return nil, nil
		}
		t.Fatalf("unexpected command %s %v", name, args)
		return nil, nil
	}
	ctx := context.Background()
	// Debian: the snakeoil pair is named; nothing is changed.
	settings["smtpd_tls_cert_file"] = "/etc/ssl/certs/ssl-cert-snakeoil.pem"
	if err := ensurePostfixDefaultServerCertificate(ctx, run); err != nil || len(calls) != 1 || created != 0 {
		t.Fatalf("named certificate: err=%v calls=%v created=%d", err, calls, created)
	}
	// A chain file counts as a certificate too.
	settings, calls = map[string]string{"smtpd_tls_chain_files": "/etc/x/chain.pem"}, nil
	if err := ensurePostfixDefaultServerCertificate(ctx, run); err != nil || len(calls) != 1 || created != 0 {
		t.Fatalf("chain files: err=%v calls=%v", err, calls)
	}
	// A cancelled lease creates nothing.
	settings, calls = map[string]string{}, nil
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := ensurePostfixDefaultServerCertificate(cancelled, run); err == nil || !strings.Contains(err.Error(), "lease ended") || created != 0 {
		t.Fatalf("cancelled: err=%v created=%d", err, created)
	}
	// Arch: none named; the default pair is made and named with opportunistic TLS.
	if err := ensurePostfixDefaultServerCertificate(ctx, run); err != nil || created != 1 {
		t.Fatalf("arch: err=%v created=%d", err, created)
	}
	if settings["smtpd_tls_cert_file"] != postfixDefaultCertPath || settings["smtpd_tls_key_file"] != postfixDefaultKeyPath || settings["smtpd_tls_security_level"] != "may" {
		t.Fatalf("settings=%v", settings)
	}
	// An existing pair is reused, never regenerated.
	settings = map[string]string{}
	if err := ensurePostfixDefaultServerCertificate(ctx, run); err != nil || created != 1 {
		t.Fatalf("reuse: err=%v created=%d", err, created)
	}
}

func TestSelfSignedPairIsAUsableServerCertificate(t *testing.T) {
	dir := t.TempDir()
	cert, key := filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	if err := writeSelfSignedPair(cert, key, "mail.example.test"); err != nil {
		t.Skipf("secure writes unavailable here: %v", err)
	}
	if _, err := tls.LoadX509KeyPair(cert, key); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(cert)
	block, _ := pem.Decode(raw)
	parsed, err := x509.ParseCertificate(block.Bytes)
	if err != nil || parsed.Subject.CommonName != "mail.example.test" || parsed.DNSNames[0] != "mail.example.test" {
		t.Fatalf("certificate=%+v err=%v", parsed, err)
	}
	if info, err := os.Stat(key); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("key mode: %v %v", info, err)
	}
}
