package services

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

// NewDefaultHTTPSPlaceholderPair makes the placeholder certificate of the
// default HTTPS server on nginx older than 1.19.4: ECDSA P-256 (every TLS 1.2
// client of the sites' cipher list has ECDHE-ECDSA), self-signed for
// DefaultHTTPSPlaceholderName only, ten years. Made in Go: a server need not
// have the openssl command.
func NewDefaultHTTPSPlaceholderPair() (certPEM, keyPEM []byte, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate key: %w", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	if err != nil {
		return nil, nil, fmt.Errorf("generate serial: %w", err)
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: DefaultHTTPSPlaceholderName},
		DNSNames:              []string{DefaultHTTPSPlaceholderName},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, nil, fmt.Errorf("create certificate: %w", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, fmt.Errorf("encode key: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), nil
}

// ensureDefaultHTTPSPlaceholderPair puts the placeholder pair in place when
// its certificate is absent (the key first, root:root 0600, then the
// certificate, 0644: a certificate never names a key that is not on disk; a
// key left alone by an interrupted run is replaced with the new pair). A
// pair that is there is kept when it is a usable pair, whoever put it there;
// one that is not is reported and left as it is.
func ensureDefaultHTTPSPlaceholderPair() error {
	certPath, keyPath := DefaultHTTPSPlaceholderCertPath(), DefaultHTTPSPlaceholderKeyPath()
	certPEM, certErr := os.ReadFile(certPath)
	switch {
	case certErr == nil:
		keyPEM, err := os.ReadFile(keyPath)
		if err != nil {
			return fmt.Errorf("remove both files so CelikPanel makes a new pair, or put the key back: the key %s cannot be read (%v); the certificate is %s", keyPath, err, certPath)
		}
		if _, err := tls.X509KeyPair(certPEM, keyPEM); err != nil {
			return fmt.Errorf("remove both files so CelikPanel makes a new pair: %s and %s are not a usable certificate and key pair (%v)", certPath, keyPath, err)
		}
		return nil
	case !errors.Is(certErr, os.ErrNotExist):
		return certErr
	}
	certPEM, keyPEM, err := NewDefaultHTTPSPlaceholderPair()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(keyPath), 0o755); err != nil {
		return err
	}
	if err := atomicWriteSiteFile(keyPath, keyPEM, 0o600, rootFileOwnership()); err != nil {
		return fmt.Errorf("write %s: %w", keyPath, err)
	}
	if err := atomicWriteSiteFile(certPath, certPEM, 0o644, rootFileOwnership()); err != nil {
		return fmt.Errorf("write %s: %w", certPath, err)
	}
	return nil
}

// DefaultHTTPSServerStatus reads, without changing anything, what state
// CelikPanel's default HTTPS server is in: from the `nginx -T` dump the
// caller already has (dumpErr when it could not be read), the file itself
// and the measured nginx. The last attempt of the caller's run, when there
// was one, turns "not there yet" into a verified failure with its reason.
//
// Varsayılan HTTPS sunucusunun durumunu hiçbir şeyi değiştirmeden okur.
func (ng *NginxGenerator) DefaultHTTPSServerStatus(dump string, dumpErr error, last *DefaultHTTPSServerResult) core.NginxDefaultHTTPS {
	path := DefaultHTTPSServerPath()
	features := ng.features()
	status := core.NginxDefaultHTTPS{Path: path, Mode: DefaultHTTPSMode(features)}
	if features.Known {
		status.NginxVersion = features.Version.String()
	}
	if last != nil {
		status.LastOutcome = last.Outcome
	}

	inspected := inspectSiteFile(path, nil, "")
	switch inspected.state {
	case transport.SiteFileAbsent, transport.SiteFileManagedUnchanged:
	case transport.SiteFileUnreadable:
		status.State, status.FileState = core.NginxDefaultHTTPSUnknown, inspected.state
		status.Detail = boundedSiteFileDetail(strings.TrimSpace(inspected.reason + " " + inspected.detail))
		return status
	default:
		status.State, status.FileState = core.NginxDefaultHTTPSOwnerFile, inspected.state
		return status
	}
	if inspected.exists {
		// What the file says, not what this nginx would get now.
		if strings.Contains(string(inspected.content), "\n    ssl_reject_handshake on;\n") {
			status.Mode = DefaultHTTPSModeReject
		} else {
			status.Mode = DefaultHTTPSModePlaceholder
		}
	}
	if dumpErr != nil {
		status.State = core.NginxDefaultHTTPSUnknown
		status.Detail = boundedSiteFileDetail("nginx -T: " + dumpErr.Error())
		return status
	}
	serves443, ownerDefault := httpsListenFacts(dump, path)
	switch {
	case ownerDefault != "":
		status.State, status.OwnerFile = core.NginxDefaultHTTPSOwnerDefault, ownerDefault
	case !serves443:
		status.State = core.NginxDefaultHTTPSNotNeeded
	case inspected.exists:
		status.State = core.NginxDefaultHTTPSActive
	case last != nil && last.Outcome == DefaultHTTPSFailed:
		status.State, status.Detail = core.NginxDefaultHTTPSFailed, last.Detail
	default:
		status.State = core.NginxDefaultHTTPSPending
	}
	return status
}
