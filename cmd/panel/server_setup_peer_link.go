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

// serverSetupLinkTarget: where and how to reach the peer. Pin is the peer's
// key fingerprint (10 bytes); ServerName, when set, allows the certificate
// fallback for that panel name.
type serverSetupLinkTarget struct {
	Address    string
	Pin        []byte
	ServerName string
}

func serverSetupLinkJSON(ctx context.Context, target serverSetupLinkTarget, path, bearer string, request, response any) error {
	err := serverSetupLinkJSONOnce(ctx, target, false, path, bearer, request, response)
	if errors.Is(err, errServerSetupLinkPinMismatch) && target.ServerName != "" {
		return serverSetupLinkJSONOnce(ctx, target, true, path, bearer, request, response)
	}
	return err
}

func serverSetupLinkJSONOnce(ctx context.Context, target serverSetupLinkTarget, byName bool, path, bearer string, request, response any) error {
	address, ok := canonicalServerSetupPeerAddress(target.Address)
	if !ok || address != target.Address || !serverSetupLinkMachinePath(path) || len(target.Pin) != 10 {
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
	pinMismatch := false
	config := &tls.Config{MinVersion: tls.VersionTLS12}
	if byName {
		canonical, err := canonicalRemoteDNSEndpoint("https://" + target.ServerName)
		if err != nil || canonical != "https://"+target.ServerName {
			return errors.New("invalid setup link name")
		}
		config.ServerName, config.RootCAs = target.ServerName, deps.roots
	} else {
		// The key fingerprint is the trust: no name, no authority.
		config.InsecureSkipVerify = true
		config.VerifyConnection = func(state tls.ConnectionState) error {
			if len(state.PeerCertificates) == 0 || subtle.ConstantTimeCompare(serverSetupSPKIPin(state.PeerCertificates[0]), target.Pin) != 1 {
				pinMismatch = true
				return errServerSetupLinkPinMismatch
			}
			return nil
		}
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
	wrote := false
	trace := &httptrace.ClientTrace{WroteHeaders: func() { wrote = true }}
	req, err := http.NewRequestWithContext(httptrace.WithClientTrace(ctx, trace), http.MethodPost, "https://"+host+path, bytes.NewReader(payload))
	if err != nil {
		return errors.New("setup link request could not be prepared")
	}
	if byName {
		// The certificate names the panel; the request still goes to the
		// peer's known address.
		req.Host = net.JoinHostPort(target.ServerName, u.Port())
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
		case pinMismatch:
			return errServerSetupLinkPinMismatch
		case wrote:
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
