package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/alicelik/celikpanel/internal/hostname"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Postfix and Dovecot start for the first time in ConfigureMailStack. When
// setup already published the trusted mail host certificate for the mail
// name (IssueMailHostCertificateV1, Staged), both start with it instead of a
// self-signed default. The mail TLS step later selects the same pair through
// the same function (selectedMailHostCertificate), so nothing moves then.
// Kurulumun yayımladığı güvenilir posta sertifikası varsa Postfix ve Dovecot
// ilk kez onunla başlar; posta TLS adımı aynı çifti seçer.

// mailStackSelectHostCertificate is a test seam.
var mailStackSelectHostCertificate = selectedMailHostCertificate

// mailStackHostCertificate returns the managed pair current for name, or
// empty paths when there is none (or no name was given). A present but
// unusable pair is not an install failure here: the default stays and the
// note says why; the mail TLS step reports it as its own failure.
func mailStackHostCertificate(name string) (cert, key, note string) {
	if name == "" {
		return "", "", ""
	}
	canonical, err := hostname.CanonicalFQDN(name)
	if err != nil || canonical != name {
		return "", "", "the mail name sent for the mail host certificate is not a canonical name; the default certificate is used"
	}
	cert, key, err = mailStackSelectHostCertificate(name)
	if err != nil {
		return "", "", fmt.Sprintf("the mail host certificate could not be used (%v); the default certificate is used until the mail TLS step", err)
	}
	if cert == defaultMailCert || cert == "" || key == "" {
		return "", "", ""
	}
	return cert, key, ""
}

// mailStackHostCertificateState is the additive response field: managed only
// when every configured mail service uses the managed pair; empty when no
// mail name was given (nothing was checked).
func mailStackHostCertificateState(name, managedCert string, hasPostfix, postfixManaged, hasDovecot, dovecotManaged bool) string {
	if name == "" {
		return ""
	}
	if managedCert == "" {
		return transport.MailHostCertificateDefault
	}
	if (hasPostfix && !postfixManaged) || (hasDovecot && !dovecotManaged) {
		return transport.MailHostCertificateDefault
	}
	return transport.MailHostCertificateManaged
}

// mailTLSHostCertificateState names the server certificate a verified mail
// TLS synchronization left in use: the same selection it applied. Empty when
// the selection cannot be read now (unknown, not default).
func mailTLSHostCertificateState(myhostname string) string {
	cert, _, err := mailStackSelectHostCertificate(myhostname)
	switch {
	case err != nil:
		return ""
	case cert == defaultMailCert:
		return transport.MailHostCertificateDefault
	default:
		return transport.MailHostCertificateManaged
	}
}

// ensureDovecotHostCertificate writes CelikPanel's Dovecot TLS drop-in with
// the managed pair before Dovecot's first start. An existing drop-in belongs
// to the mail TLS step and is left as it is.
func ensureDovecotHostCertificate(name, managedCert string, run mailTLSCommandRunner) (bool, error) {
	if managedCert == "" {
		return false, nil
	}
	existing, err := os.ReadFile(dovecotTLSConf)
	if err == nil {
		return strings.Contains(string(existing), managedCert), nil
	}
	if !os.IsNotExist(err) {
		return false, fmt.Errorf("read %s: %w", dovecotTLSConf, err)
	}
	modern, err := dovecotIs24WithRunner(run)
	if err != nil {
		return false, err
	}
	if err := configureDovecotTLSForDialect(name, nil, modern, run); err != nil {
		return false, err
	}
	return true, nil
}
