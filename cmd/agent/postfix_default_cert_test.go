package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPostfixDefaultServerCertificateOnlyWhenNoneIsNamed(t *testing.T) {
	dir := t.TempDir()
	oldCert, oldKey := postfixDefaultCertPath, postfixDefaultKeyPath
	t.Cleanup(func() { postfixDefaultCertPath, postfixDefaultKeyPath = oldCert, oldKey })
	postfixDefaultCertPath, postfixDefaultKeyPath = filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	settings := map[string]string{}
	var calls []string
	run := func(name string, args ...string) ([]byte, error) {
		calls = append(calls, name+" "+strings.Join(args, " "))
		switch {
		case name == "postconf" && args[0] == "-h":
			return []byte(settings[args[1]] + "\n"), nil
		case name == "postconf" && args[0] == "-e":
			key, value, _ := strings.Cut(args[1], "=")
			settings[key] = value
			return nil, nil
		case name == "openssl":
			for _, path := range []string{postfixDefaultCertPath, postfixDefaultKeyPath} {
				if err := os.WriteFile(path, []byte("pem"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			return nil, nil
		}
		t.Fatalf("unexpected command %s %v", name, args)
		return nil, nil
	}
	// Debian: the snakeoil pair is named; nothing is changed.
	settings["smtpd_tls_cert_file"] = "/etc/ssl/certs/ssl-cert-snakeoil.pem"
	if err := ensurePostfixDefaultServerCertificate(run); err != nil {
		t.Fatal(err)
	}
	for _, call := range calls {
		if !strings.HasPrefix(call, "postconf -h") {
			t.Fatalf("a named certificate was touched: %v", calls)
		}
	}
	// A chain file counts as a certificate too.
	settings = map[string]string{"smtpd_tls_chain_files": "/etc/x/chain.pem"}
	calls = nil
	if err := ensurePostfixDefaultServerCertificate(run); err != nil || len(calls) != 2 {
		t.Fatalf("chain files: err=%v calls=%v", err, calls)
	}
	// Arch: none named; the default pair is named with opportunistic TLS.
	settings = map[string]string{}
	calls = nil
	if err := ensurePostfixDefaultServerCertificate(run); err != nil {
		t.Fatal(err)
	}
	if settings["smtpd_tls_cert_file"] != postfixDefaultCertPath || settings["smtpd_tls_key_file"] != postfixDefaultKeyPath || settings["smtpd_tls_security_level"] != "may" {
		t.Fatalf("settings=%v", settings)
	}
	if info, err := os.Stat(postfixDefaultKeyPath); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("key mode: %v %v", info, err)
	}
}
