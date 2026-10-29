package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Claim-4 (alpha.114 review): the setup link pins the identity the listener
// actually serves on the panel's IP address, captured once at start: the
// first self-signed certificate (found by panelIPCertificate) across a
// managed domain certificate. A missing, unpaired or expired one means no
// code can be made, a verified state with the owner's next action.
func TestServerSetupIdentityIsTheListenersIPAddressIdentity(t *testing.T) {
	certPath, keyPath, bootstrap := accessTestManagedPair(t)
	server := newPanelHTTPServer("127.0.0.1:0", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	running, err := startPanelHTTP(server, certPath, keyPath)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	p := &Panel{}
	p.captureServerSetupIdentity(server.TLSConfig)
	identity, err := p.serverSetupPanelCertificate()
	if err != nil || !bytes.Equal(identity.Raw, bootstrap) {
		t.Fatalf("the bootstrap identity is pinned: %v", err)
	}
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}, "tcp", running.addr.String(), &tls.Config{InsecureSkipVerify: true}) // Inspect the served test identity.
	if err != nil {
		t.Fatal(err)
	}
	served := conn.ConnectionState().PeerCertificates[0]
	conn.Close()
	if !bytes.Equal(served.Raw, identity.Raw) {
		t.Fatal("the pinned identity is the one a peer dialing the IP address gets")
	}
	// Captured once: files changed later do not change what is pinned.
	if err := os.WriteFile(filepath.Join(tlsDir(), "panel.crt"), []byte("not a certificate"), 0o600); err != nil {
		t.Fatal(err)
	}
	if again, err := p.serverSetupPanelCertificate(); err != nil || !bytes.Equal(again.Raw, bootstrap) {
		t.Fatalf("not re-read: %v", err)
	}
	// A broken bootstrap (its key gone): the listener serves the domain
	// certificate on the IP path, which cannot be the link's identity.
	if err := os.Remove(filepath.Join(tlsDir(), "panel.key")); err != nil {
		t.Fatal(err)
	}
	broken := newPanelHTTPServer("127.0.0.1:0", http.NotFoundHandler())
	if _, err := configurePanelHTTPTLS(broken, certPath, keyPath); err != nil {
		t.Fatal(err)
	}
	f, _, _ := joinFirstFixture(t, joinFirstDraft())
	f.panel.captureServerSetupIdentity(broken.TLSConfig)
	expired := expiredSelfSignedIPCertificate(t)
	for label, capture := range map[string]func(){
		"domain certificate": func() {},
		"none":               func() { f.panel.setupLink.identity.Store(nil) },
		"expired":            func() { f.panel.setupLink.identity.Store(expired) },
	} {
		capture()
		w := httptest.NewRecorder()
		f.panel.handleServerSetupJoinCodes(w, serviceOperationAdminRequest(t, http.MethodPost, serverSetupJoinCodesPath, "{}", f.userID))
		if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "server_setup_join_certificate_unavailable") || !strings.Contains(w.Body.String(), "restarts the panel") {
			t.Fatalf("%s: %d %s", label, w.Code, w.Body.String())
		}
	}
}

func expiredSelfSignedIPCertificate(t *testing.T) *x509.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(7), Subject: pkix.Name{CommonName: "celikpanel"}, NotBefore: time.Now().Add(-48 * time.Hour), NotAfter: time.Now().Add(-time.Hour), IPAddresses: []net.IP{net.ParseIP("203.0.113.10")}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return leaf
}
