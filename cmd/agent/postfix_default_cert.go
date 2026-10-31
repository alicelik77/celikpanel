package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/hostname"
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
//
// The pair is made in Go (crypto/x509), not with the openssl command: a
// server need not have openssl installed (the distro config matrix's
// containers did not), and an extra subprocess is one more thing a lease
// cancellation has to stop.
// Arch'ın main.cf'i sertifika adlandırmaz; Postfix'in hiç sunucu sertifikası
// yoksa CelikPanel kendi kendinden imzalı varsayılan çiftini Go ile üretir;
// sahibin ya da dağıtımın sertifikasına dokunulmaz.

var (
	postfixDefaultCertPath = "/etc/postfix/celikpanel-default-cert.pem"
	postfixDefaultKeyPath  = "/etc/postfix/celikpanel-default-key.pem"
	// postfixDefaultPairCreate writes the self-signed pair (a test seam).
	postfixDefaultPairCreate = writeSelfSignedPair
)

type postfixCommandRunner func(name string, args ...string) ([]byte, error)

// postfixHasServerCertificate reads the two settings that can name one, in
// one postconf call.
func postfixHasServerCertificate(run postfixCommandRunner) (bool, error) {
	out, err := run("postconf", "-h", "smtpd_tls_cert_file", "smtpd_tls_chain_files")
	if err != nil {
		return false, fmt.Errorf("read the Postfix server certificate settings: %s", firstLine(string(out)))
	}
	return strings.TrimSpace(string(out)) != "", nil
}

func ensurePostfixDefaultServerCertificate(ctx context.Context, run postfixCommandRunner) error {
	if ctx == nil {
		return fmt.Errorf("the Postfix default certificate needs a service mutation context")
	}
	has, err := postfixHasServerCertificate(run)
	if err != nil || has {
		return err
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("service mutation lease ended before the Postfix default certificate was created: %w", err)
	}
	if !fileExistsAgent(postfixDefaultCertPath) || !fileExistsAgent(postfixDefaultKeyPath) {
		host, _ := hostname.LocalFQDN()
		if host == "" {
			host = "localhost"
		}
		if err := postfixDefaultPairCreate(postfixDefaultCertPath, postfixDefaultKeyPath, host); err != nil {
			return fmt.Errorf("create the default Postfix certificate: %w", err)
		}
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

// writeSelfSignedPair writes an RSA-2048 key (root-only, 0600) and a
// ten-year self-signed certificate for commonName (0644). The key is written
// first; a certificate never names a key that is not on disk.
func writeSelfSignedPair(certPath, keyPath, commonName string) error {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("generate key: %w", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	if err != nil {
		return fmt.Errorf("generate serial: %w", err)
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: commonName},
		DNSNames:              []string{commonName},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return fmt.Errorf("create certificate: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	if err := secureWriteConfig(keyPath, keyPEM, 0o600); err != nil {
		return fmt.Errorf("write key: %w", err)
	}
	if err := secureSetMailFileMetadata(keyPath, 0o600, 0, 0); err != nil {
		return fmt.Errorf("protect key: %w", err)
	}
	if err := secureWriteConfig(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644); err != nil {
		return fmt.Errorf("write certificate: %w", err)
	}
	return nil
}
