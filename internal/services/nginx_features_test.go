package services

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseNginxFeatures(t *testing.T) {
	const ubuntu = "nginx version: nginx/1.24.0 (Ubuntu)\n" +
		"built with OpenSSL 3.0.13 30 Jan 2024\nTLS SNI support enabled\n" +
		"configure arguments: --with-cc-opt='-g -O2' --prefix=/usr/share/nginx --with-http_ssl_module --with-http_v2_module --with-http_realip_module\n"
	const debian = "nginx version: nginx/1.26.3\r\nbuilt with OpenSSL 3.5.6 7 Apr 2026\r\n" +
		"configure arguments: --with-http_ssl_module --with-http_v2_module --with-http_v3_module\r\n"
	for _, tc := range []struct {
		name   string
		output string
		want   NginxFeatures
		http2  string
	}{
		{"ubuntu 24.04", ubuntu, NginxFeatures{Known: true, Version: NginxVersion{1, 24, 0}, HTTP2Module: true}, NginxHTTP2Listen},
		{"debian 13 crlf", debian, NginxFeatures{Known: true, Version: NginxVersion{1, 26, 3}, HTTP2Module: true}, NginxHTTP2Directive},
		{"nginx -v only", "nginx version: nginx/1.27.4\n", NginxFeatures{Known: true, Version: NginxVersion{1, 27, 4}, HTTP2Module: true}, NginxHTTP2Directive},
		{"no http_v2 module", "nginx version: nginx/1.26.3\nconfigure arguments: --with-http_ssl_module\n", NginxFeatures{Known: true, Version: NginxVersion{1, 26, 3}}, NginxHTTP2Off},
		{"freenginx", "nginx version: freenginx/1.27.2\n", NginxFeatures{Known: true, Version: NginxVersion{1, 27, 2}, HTTP2Module: true}, NginxHTTP2Directive},
		{"openresty", "nginx version: openresty/1.25.3.2\n", NginxFeatures{Known: true, Version: NginxVersion{1, 25, 3}, HTTP2Module: true}, NginxHTTP2Directive},
		{"angie numbers its own releases", "nginx version: Angie/1.9.0\n", NginxFeatures{}, NginxHTTP2Off},
		{"very old", "nginx version: nginx/1.8.1\n", NginxFeatures{Known: true, Version: NginxVersion{1, 8, 1}, HTTP2Module: true}, NginxHTTP2Off},
		{"empty", "", NginxFeatures{}, NginxHTTP2Off},
		{"error text", "nginx: [emerg] open() \"/etc/nginx/nginx.conf\" failed\n", NginxFeatures{}, NginxHTTP2Off},
		{"version later on a line", "warning: nginx version: nginx/1.26.3\n", NginxFeatures{}, NginxHTTP2Off},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseNginxFeatures(tc.output)
			if got != tc.want {
				t.Fatalf("ParseNginxFeatures = %+v, want %+v", got, tc.want)
			}
			if form := got.HTTP2Form(); form != tc.http2 {
				t.Errorf("HTTP2Form = %q, want %q", form, tc.http2)
			}
		})
	}
}

func TestRejectHandshakeNeedsNginx1194(t *testing.T) {
	for _, tc := range []struct {
		features NginxFeatures
		want     bool
	}{
		{NginxFeatures{Known: true, Version: NginxVersion{1, 19, 4}}, true},
		{NginxFeatures{Known: true, Version: NginxVersion{1, 24, 0}}, true},
		{NginxFeatures{Known: true, Version: NginxVersion{1, 19, 3}}, false},
		{NginxFeatures{Known: true, Version: NginxVersion{1, 18, 9}}, false},
		{NginxFeatures{}, false},
	} {
		if got := tc.features.RejectHandshake(); got != tc.want {
			t.Errorf("%+v: RejectHandshake = %v, want %v", tc.features, got, tc.want)
		}
	}
}

// The measurement is kept while the binary is the same file and taken again
// when the package replaces it; a failed measurement is not kept.
func TestHostNginxFeaturesFollowsTheBinary(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "nginx")
	if err := os.WriteFile(binary, []byte("one"), 0o755); err != nil {
		t.Fatal(err)
	}
	oldLook, oldRun := nginxFeaturesLookPath, nginxFeaturesRun
	t.Cleanup(func() {
		nginxFeaturesLookPath, nginxFeaturesRun = oldLook, oldRun
		nginxFeaturesMu.Lock()
		nginxFeaturesKey, nginxFeaturesCache = "", NginxFeatures{}
		nginxFeaturesMu.Unlock()
	})
	nginxFeaturesMu.Lock()
	nginxFeaturesKey, nginxFeaturesCache = "", NginxFeatures{}
	nginxFeaturesMu.Unlock()
	nginxFeaturesLookPath = func(string) (string, error) { return binary, nil }
	calls := 0
	answer := "nginx version: nginx/1.24.0 (Ubuntu)\n"
	var failure error
	nginxFeaturesRun = func(_ context.Context, path string) ([]byte, error) {
		calls++
		if path != binary {
			t.Errorf("ran %s, want %s", path, binary)
		}
		return []byte(answer), failure
	}

	failure = errors.New("exit status 1")
	if got := HostNginxFeatures(); got.Known {
		t.Fatalf("a failed nginx -V is unknown, got %+v", got)
	}
	failure = nil
	if got := HostNginxFeatures(); got.Version != (NginxVersion{1, 24, 0}) {
		t.Fatalf("got %+v", got)
	}
	if got := HostNginxFeatures(); got.Version != (NginxVersion{1, 24, 0}) || calls != 2 {
		t.Fatalf("the same binary is measured once (calls %d, got %+v)", calls, got)
	}
	// The package upgrade replaces the binary.
	answer = "nginx version: nginx/1.26.3\n"
	if err := os.WriteFile(binary, []byte("two, longer"), 0o755); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(time.Minute)
	if err := os.Chtimes(binary, future, future); err != nil {
		t.Fatal(err)
	}
	if got := HostNginxFeatures(); got.Version != (NginxVersion{1, 26, 3}) || calls != 3 {
		t.Fatalf("a replaced binary is measured again (calls %d, got %+v)", calls, got)
	}

	nginxFeaturesLookPath = func(string) (string, error) { return "", errors.New("not found") }
	if _, err := os.Stat("/usr/sbin/nginx"); err != nil {
		if _, err := os.Stat("/usr/bin/nginx"); err != nil {
			if got := HostNginxFeatures(); got.Known {
				t.Fatalf("no nginx is unknown, got %+v", got)
			}
		}
	}
}
