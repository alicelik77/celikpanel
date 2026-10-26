package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"time"

	paneltransport "github.com/alicelik/celikpanel/internal/transport"
)

// Webmail readiness (alpha.110). The panel no longer proxies Roundcube:
// webmail has its own address served by the server's nginx
// (webmail_address_handlers.go), and /webmail/ on the panel only redirects
// there. The panel still asks Roundcube's local server, read only, whether it
// answers (its root-bound Unix socket), so screens offer webmail only while
// it runs.
//
// Panel artık Roundcube'u vekillemez; yalnız yerel soketten çalışıp
// çalışmadığını salt okur sorar.
const (
	webmailPublicPath              = "/webmail/"
	webmailInternalHost            = "celikpanel-webmail.internal"
	webmailReadinessTimeout        = time.Second
	webmailUpstreamHeaderTimeout   = 30 * time.Second
	webmailReadinessBodyDrainLimit = 8 << 10
)

var (
	webmailUpstreamTransport = newWebmailUnixTransport(paneltransport.WebmailSocketPath())
	webmailReadinessClient   = &http.Client{
		Transport: webmailUpstreamTransport,
		Timeout:   webmailReadinessTimeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	webmailRequestLimiter = newRateLimiter(240, time.Minute)
)

func newWebmailUnixTransport(socketPath string) *http.Transport {
	dialer := &net.Dialer{Timeout: webmailReadinessTimeout}
	return &http.Transport{
		// Never honor HTTP_PROXY for this privileged local boundary and never
		// fall back to TCP. Every HTTP connection is the fixed Unix socket.
		Proxy: nil,
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, "unix", socketPath)
		},
		ForceAttemptHTTP2:     false,
		MaxIdleConns:          8,
		MaxIdleConnsPerHost:   8,
		IdleConnTimeout:       30 * time.Second,
		ResponseHeaderTimeout: webmailUpstreamHeaderTimeout,
	}
}

func webmailEndpointReady(ctx context.Context, client *http.Client, endpoint string) bool {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return false
	}
	request.Header.Set("Accept", "text/html")
	response, err := client.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	if _, err := io.Copy(io.Discard, io.LimitReader(response.Body, webmailReadinessBodyDrainLimit)); err != nil {
		return false
	}
	return response.StatusCode == http.StatusOK
}

func probeWebmailReadiness(ctx context.Context) bool {
	probeContext, cancel := context.WithTimeout(ctx, webmailReadinessTimeout)
	defer cancel()
	return webmailEndpointReady(
		probeContext,
		webmailReadinessClient,
		"http://"+webmailInternalHost+webmailPublicPath,
	)
}
