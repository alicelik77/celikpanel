package main

import (
	"bytes"
	"context"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/netip"
	"net/url"
	"strconv"
	"sync/atomic"
	"time"
)

// The setup link's transport (alpha.114). Before the peer panel has a
// certificate from a public authority it is reached at its IP address and
// trusted only by the fingerprint of its TLS key carried in the join code:
// the handshake checks the fingerprint before any request byte is written,
// so a wrong address or an interception never receives the secret. Once the
// peer panel's certificate changes (its domain certificate), the link uses
// ordinary certificate validation for the peer's panel name, still dialing
// only the peer's known address.
// Kurulum baglantisi: once anahtar parmak izi, sonra alan adi sertifikasi.

var (
	// errServerSetupLinkUnreachable: no connection or handshake; nothing was sent.
	errServerSetupLinkUnreachable = errors.New("the other server could not be reached")
	// errServerSetupLinkPinMismatch: the address answered with another key;
	// nothing was sent.
	errServerSetupLinkPinMismatch = errors.New("the other server's TLS key does not match the join code")
	// errServerSetupLinkUnknown: the request may have arrived; its answer was
	// lost or unreadable. The same request is asked again.
	errServerSetupLinkUnknown = errors.New("the other server's answer was not received")
)

// serverSetupLinkRefusal is a verified answer refusing the request.
type serverSetupLinkRefusal struct {
	Status       int
	Code         string
	AttemptsLeft *int
}

func (e *serverSetupLinkRefusal) Error() string { return "the other server refused: " + e.Code }

// serverSetupLinkDependencies are replaced only by tests (loopback servers,
// their own roots); production dials the literal address with system roots.
type serverSetupLinkDependencies struct {
	dial  func(ctx context.Context, network, address string) (net.Conn, error)
	roots *x509.CertPool
}

var serverSetupLinkDeps = func() serverSetupLinkDependencies {
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	return serverSetupLinkDependencies{dial: dialer.DialContext}
}

// serverSetupLinkTarget: where and how to reach the peer. Pin is the
// SHA-256 of the peer's TLS SubjectPublicKeyInfo: the code's 10-byte prefix
// for the join, the full 32 bytes afterwards. There is no name fallback: the
// link never accepts a certificate whose key does not match the pin (the
// panel keeps serving its first self-signed identity on its IP address
// across domain certificate changes, so the pin stays valid during setup).
// Observed, when set, receives the full fingerprint the peer presented.
type serverSetupLinkTarget struct {
	Address  string
	Pin      []byte
	Observed *[]byte
}

func serverSetupLinkJSON(ctx context.Context, target serverSetupLinkTarget, path, bearer string, request, response any) error {
	address, ok := canonicalServerSetupPeerAddress(target.Address)
	if !ok || address != target.Address || !serverSetupLinkMachinePath(path) || (len(target.Pin) != 10 && len(target.Pin) != 32) {
		return errors.New("invalid setup link exchange")
	}
	u, _ := url.Parse(address)
	ip, err := netip.ParseAddr(u.Hostname())
	if err != nil || !serverSetupPeerAddressAllowed(ip) {
		return errors.New("the other server's address is not a public server address")
	}
	payload, err := json.Marshal(request)
	if err != nil || len(payload) > remoteDNSJSONLimit {
		return errors.New("setup link request exceeds its contract")
	}
	deps := serverSetupLinkDeps()
	var pinMismatch, wrote atomic.Bool
	// The key fingerprint is the trust: no name, no authority. It is checked
	// in the handshake, before any request byte (and its bearer) is written.
	config := &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true}
	config.VerifyConnection = func(state tls.ConnectionState) error {
		if len(state.PeerCertificates) == 0 {
			pinMismatch.Store(true)
			return errServerSetupLinkPinMismatch
		}
		full := serverSetupSPKI(state.PeerCertificates[0])
		if subtle.ConstantTimeCompare(full[:len(target.Pin)], target.Pin) != 1 {
			pinMismatch.Store(true)
			return errServerSetupLinkPinMismatch
		}
		if target.Observed != nil {
			*target.Observed = full
		}
		return nil
	}
	host := u.Host
	httpTransport := &http.Transport{
		Proxy: nil, DisableKeepAlives: true, DisableCompression: true, ForceAttemptHTTP2: false,
		TLSNextProto:    map[string]func(string, *tls.Conn) http.RoundTripper{},
		TLSClientConfig: config, TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 15 * time.Second, MaxResponseHeaderBytes: 16 << 10,
		DialContext: func(dialCtx context.Context, network, dialAddress string) (net.Conn, error) {
			if network != "tcp" || dialAddress != host {
				return nil, errors.New("setup link target changed")
			}
			return deps.dial(dialCtx, "tcp", host)
		},
	}
	defer httpTransport.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	trace := &httptrace.ClientTrace{WroteHeaders: func() { wrote.Store(true) }}
	req, err := http.NewRequestWithContext(httptrace.WithClientTrace(ctx, trace), http.MethodPost, "https://"+host+path, bytes.NewReader(payload))
	if err != nil {
		return errors.New("setup link request could not be prepared")
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Transport: httpTransport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		switch {
		case pinMismatch.Load():
			return errServerSetupLinkPinMismatch
		case wrote.Load():
			return errServerSetupLinkUnknown
		default:
			return errServerSetupLinkUnreachable
		}
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, remoteDNSJSONLimit+1))
	if err != nil || len(raw) > remoteDNSJSONLimit {
		return errServerSetupLinkUnknown
	}
	if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != http.StatusRequestTimeout {
		var refusal serverSetupJoinRefusal
		if json.Unmarshal(raw, &refusal) != nil || refusal.Code == "" || len(refusal.Code) > 64 {
			return &serverSetupLinkRefusal{Status: resp.StatusCode, Code: "server_setup_link_refused"}
		}
		for _, c := range refusal.Code {
			if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_' || (c >= 'A' && c <= 'Z')) {
				return &serverSetupLinkRefusal{Status: resp.StatusCode, Code: "server_setup_link_refused"}
			}
		}
		return &serverSetupLinkRefusal{Status: resp.StatusCode, Code: refusal.Code, AttemptsLeft: refusal.AttemptsLeft}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errServerSetupLinkUnknown
	}
	mediaType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return errServerSetupLinkUnknown
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(response) != nil || decoder.Decode(new(any)) != io.EOF {
		return errServerSetupLinkUnknown
	}
	return nil
}

func serverSetupPanelPortOf(address string) int {
	u, err := url.Parse(address)
	if err != nil {
		return 0
	}
	port, _ := strconv.Atoi(u.Port())
	return port
}
