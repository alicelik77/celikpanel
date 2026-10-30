package main

import (
	"fmt"
	"os"
	"strings"
)

// Debian's postfix main.cf names the snakeoil pair (ssl-cert), so STARTTLS on
// port 25 and the submission ports have a certificate from the first start.
// Arch Linux's main.cf names none: submission (smtpd_tls_security_level=
// encrypt) then refuses every client with "No server certs available" until
// the mail certificate is configured, and port 25 offers no STARTTLS. When
// Postfix has no server certificate at all, CelikPanel gives it a self-signed
// default pair of its own, exactly as it does for Arch's Dovecot; the mail
// host certificate replaces it later (configurePostfixTLS). A certificate the
// owner or the distribution configured is never touched.
// Arch'ın main.cf'i sertifika adlandırmaz; Postfix'in hiç sunucu sertifikası
// yoksa CelikPanel kendi kendinden imzalı varsayılan çiftini verir; sahibin
// ya da dağıtımın sertifikasına dokunulmaz.

var (
	postfixDefaultCertPath = "/etc/postfix/celikpanel-default-cert.pem"
	postfixDefaultKeyPath  = "/etc/postfix/celikpanel-default-key.pem"
)

type postfixCommandRunner func(name string, args ...string) ([]byte, error)

// postfixHasServerCertificate reads the two settings that can name one.
func postfixHasServerCertificate(run postfixCommandRunner) (bool, error) {
	for _, key := range []string{"smtpd_tls_cert_file", "smtpd_tls_chain_files"} {
		out, err := run("postconf", "-h", key)
		if err != nil {
			return false, fmt.Errorf("read postfix %s: %s", key, firstLine(string(out)))
		}
		if strings.TrimSpace(string(out)) != "" {
			return true, nil
		}
	}
	return false, nil
}

func ensurePostfixDefaultServerCertificate(run postfixCommandRunner) error {
	has, err := postfixHasServerCertificate(run)
	if err != nil || has {
		return err
	}
	if !fileExistsAgent(postfixDefaultCertPath) || !fileExistsAgent(postfixDefaultKeyPath) {
		host, _ := os.Hostname()
		if host == "" {
			host = "localhost"
		}
		if out, err := run("openssl", "req", "-x509", "-nodes", "-newkey", "rsa:2048", "-days", "3650",
			"-subj", "/CN="+host, "-keyout", postfixDefaultKeyPath, "-out", postfixDefaultCertPath); err != nil {
			return fmt.Errorf("create the default Postfix certificate: %s", firstLine(string(out)))
		}
		if err := os.Chmod(postfixDefaultKeyPath, 0o600); err != nil {
			return fmt.Errorf("protect the default Postfix key: %w", err)
		}
		_ = os.Chmod(postfixDefaultCertPath, 0o644)
	}
	for _, setting := range [][2]string{
		{"smtpd_tls_cert_file", postfixDefaultCertPath},
		{"smtpd_tls_key_file", postfixDefaultKeyPath},
		{"smtpd_tls_security_level", "may"},
	} {
		if out, err := run("postconf", "-e", setting[0]+"="+setting[1]); err != nil {
			return fmt.Errorf("postconf %s: %s", setting[0], firstLine(string(out)))
		}
	}
	return nil
}
