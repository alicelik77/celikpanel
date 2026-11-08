package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.130 (Frankfurt and Boston, 8 Nov 2026): the panel serves its issued
// certificate for its name and the start-up self-signed one for a bare
// address. The audit's self-dial sent no name, got the start-up certificate,
// and reported the issued one as "Failed".

func sniTestPair(t *testing.T, names []string, ips []net.IP, now time.Time) (tls.Certificate, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: "test"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(60 * 24 * time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames: names, IPAddresses: ips,
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	pair, err := tls.X509KeyPair(
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}),
	)
	if err != nil {
		t.Fatal(err)
	}
	return pair, der
}

func TestLivePanelTLSProofAsksForTheCertificatesName(t *testing.T) {
	now := time.Now()
	bootstrap, _ := sniTestPair(t, []string{"host"}, []net.IP{net.IPv4(127, 0, 0, 1)}, now)
	managed, managedLeaf := sniTestPair(t, []string{"panel.example.test"}, nil, now)
	// The listener's own arrangement (configurePanelHTTPTLS): the start-up
	// certificate is the default, the managed one answers its name.
	config := &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{bootstrap},
		GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			if hello.ServerName == "" || net.ParseIP(hello.ServerName) != nil {
				return &bootstrap, nil
			}
			return &managed, nil
		},
	}
	listener, err := tls.Listen("tcp", "127.0.0.1:0", config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			if tlsConnection, ok := connection.(*tls.Conn); ok {
				_ = tlsConnection.Handshake()
			}
			_ = connection.Close()
		}
	}()
	t.Setenv("CELIKPANEL_LISTEN", listener.Addr().String())
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	name := panelTLSDialName([]string{"panel.example.test"})
	if name != "panel.example.test" {
		t.Fatalf("dial name %q", name)
	}
	if matches, err := livePanelTLSMatches(ctx, managedLeaf, name); err != nil || !matches {
		t.Fatalf("with its name the served leaf is the managed one: %v %v", matches, err)
	}
	// Without a name (the old self-dial) the start-up certificate answers.
	if matches, err := livePanelTLSMatches(ctx, managedLeaf, ""); err != nil || matches {
		t.Fatalf("without a name: %v %v", matches, err)
	}
}

func TestPanelTLSDialNameAndHostnameMatch(t *testing.T) {
	if got := panelTLSDialName([]string{"*.example.test"}); got != "panel.example.test" {
		t.Fatalf("wildcard dial name %q", got)
	}
	if got := panelTLSDialName(nil); got != "" {
		t.Fatalf("no names: %q", got)
	}
	pass := transport.SecurityAuditCheck{Status: transport.SecurityAuditStatusPass, Code: "panel_tls_certificate_valid"}
	cases := []struct {
		host, code string
	}{
		{"panel.example.test:1923", "panel_tls_hostname_match"},
		{"PANEL.example.test.", "panel_tls_hostname_match"},
		{"other.example.test", "panel_tls_hostname_mismatch"},
		{"203.0.113.7:1923", "panel_tls_hostname_address"},
		{"[2001:db8::1]:1923", "panel_tls_hostname_address"},
		{"", "panel_tls_unknown"},
	}
	for _, c := range cases {
		response := transport.SecurityAuditTLSResponse{Certificate: pass, Names: []string{"panel.example.test"}}
		annotatePanelTLSHostname(&response, c.host)
		if response.Hostname == nil || response.Hostname.Code != c.code {
			t.Errorf("%q: %+v", c.host, response.Hostname)
		}
	}
	wild := transport.SecurityAuditTLSResponse{Certificate: pass, Names: []string{"*.example.test"}}
	annotatePanelTLSHostname(&wild, "panel.example.test")
	if wild.Hostname.Code != "panel_tls_hostname_match" {
		t.Fatalf("wildcard: %+v", wild.Hostname)
	}
	annotatePanelTLSHostname(&wild, "a.b.example.test")
	if wild.Hostname.Code != "panel_tls_hostname_mismatch" {
		t.Fatalf("a wildcard covers one label only: %+v", wild.Hostname)
	}
	failed := transport.SecurityAuditTLSResponse{Certificate: transport.SecurityAuditCheck{Status: transport.SecurityAuditStatusFail, Code: "panel_tls_invalid"}}
	annotatePanelTLSHostname(&failed, "panel.example.test")
	if failed.Hostname.Status != transport.SecurityAuditStatusUnknown {
		t.Fatalf("unreadable certificate: %+v", failed.Hostname)
	}
}

func TestPanelTLSAnswerWithNamesAndHostnamePassesTheContract(t *testing.T) {
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	certificate, key := securityAuditTestTLSMaterial(t, now.Add(-time.Hour), now.Add(80*24*time.Hour))
	result := inspectPanelTLSMaterial(certificate, key, now)
	result.Names = []string{"panel.example.test"}
	annotatePanelTLSHostname(&result, "panel.example.test")
	if err := transport.ValidateSecurityAuditTLSResponse(result); err != nil {
		t.Fatalf("contract: %v", err)
	}
	result.Names = append(result.Names, "bad name")
	if err := transport.ValidateSecurityAuditTLSResponse(result); err == nil {
		t.Fatal("a name with a space passed")
	}
	bad := nonAuthoritativePanelTLSResponse(transport.SecurityAuditStatusFail, "panel_tls_live_mismatch")
	bad.Names = []string{"panel.example.test"}
	if err := transport.ValidateSecurityAuditTLSResponse(bad); err == nil {
		t.Fatal("an unreadable certificate carried names")
	}
}
