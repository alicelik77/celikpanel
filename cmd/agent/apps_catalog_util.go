package main

import (
	"context"
	"net/http"
	"strings"
	"sync"
)

// catalogUserAgent names CelikPanel honestly to the upstream download
// servers. Some of them (releases.wikimedia.org, download.phpbb.com) answer
// HTTP 403 to Go's default and to an empty User-Agent (alpha.118 lab, while
// curl's got the file from the same address).
//
// İndirme sunucularına CelikPanel'i dürüstçe adlandırır; bazı sunucular Go'nun
// varsayılan ya da boş User-Agent'ına 403 verir.
func catalogUserAgent() string {
	version := strings.TrimSpace(buildVersion)
	if version == "" || strings.ContainsAny(version, " \t\r\n()") {
		version = "dev"
	}
	return "CelikPanel/" + version + " (+https://celikpanel.net)"
}

// catalogUserAgentTransport adds the User-Agent to a request that has none,
// also to the requests of a redirect.
type catalogUserAgentTransport struct {
	base http.RoundTripper
}

func (t catalogUserAgentTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	if request.Header.Get("User-Agent") == "" {
		request = request.Clone(request.Context())
		request.Header.Set("User-Agent", catalogUserAgent())
	}
	return base.RoundTrip(request)
}

func newGetRequest(ctx context.Context, url string) (*http.Request, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", catalogUserAgent())
	return request, nil
}

// tailBuffer keeps the last limit bytes written to it.
type tailBuffer struct {
	mu    sync.Mutex
	limit int
	data  []byte
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data = append(b.data, p...)
	if over := len(b.data) - b.limit; b.limit > 0 && over > 0 {
		b.data = append([]byte(nil), b.data[over:]...)
	}
	return len(p), nil
}

func (b *tailBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.data)
}
