package main

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAcceptsGzip(t *testing.T) {
	for header, want := range map[string]bool{
		"":                        false,
		"gzip":                    true,
		"gzip, deflate, br, zstd": true,
		"br;q=1.0, gzip;q=0.5":    true,
		"gzip;q=0":                false,
		"*":                       true,
		"*;q=0":                   false,
		"gzip;q=0, *":             false,
		"identity":                false,
		"x-gzip":                  true,
	} {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Accept-Encoding", header)
		if got := acceptsGzip(r); got != want {
			t.Errorf("Accept-Encoding %q: %v, want %v", header, got, want)
		}
	}
}

func gunzip(t *testing.T, body []byte) []byte {
	t.Helper()
	reader, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestJSONReadsAreCompressedOnlyWhenSafeAndWorthIt(t *testing.T) {
	large := `{"items":"` + strings.Repeat("component ", 400) + `"}`
	handler := compressJSONResponses(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("kind") {
		case "small":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		case "text":
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = w.Write([]byte(large))
		case "status":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(large[:600]))
			_, _ = w.Write([]byte(large[600:]))
		default:
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_, _ = w.Write([]byte(large))
		}
	}))
	serve := func(method, target, accept string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, target, nil)
		if accept != "" {
			r.Header.Set("Accept-Encoding", accept)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}

	w := serve(http.MethodGet, "/api/v1/managed-services", "gzip")
	if w.Header().Get("Content-Encoding") != "gzip" || !strings.Contains(w.Header().Get("Vary"), "Accept-Encoding") {
		t.Fatalf("a large JSON read was not compressed: %v", w.Header())
	}
	if got := gunzip(t, w.Body.Bytes()); string(got) != large || w.Body.Len() >= len(large)/4 {
		t.Fatalf("compressed body: %d bytes for %d", w.Body.Len(), len(large))
	}
	w = serve(http.MethodGet, "/api/v1/managed-services?kind=status", "gzip")
	if w.Code != http.StatusConflict || string(gunzip(t, w.Body.Bytes())) != large {
		t.Fatalf("a refusal's status or body was changed: %d", w.Code)
	}
	w = serve(http.MethodPost, "/api/v1/managed-services/scan", "gzip")
	if w.Header().Get("Content-Encoding") != "gzip" {
		t.Fatal("the scan's read-only answer was not compressed")
	}

	for name, c := range map[string]struct{ method, target, accept string }{
		"small":          {http.MethodGet, "/api/v1/managed-services?kind=small", "gzip"},
		"not JSON":       {http.MethodGet, "/api/v1/managed-services?kind=text", "gzip"},
		"no gzip":        {http.MethodGet, "/api/v1/managed-services", ""},
		"a change":       {http.MethodPost, "/api/v1/service/action", "gzip"},
		"database pass":  {http.MethodGet, "/api/v1/database-users/4/password", "gzip"},
		"2fa setup":      {http.MethodGet, "/api/v1/auth/2fa/setup", "gzip"},
		"join codes":     {http.MethodGet, "/api/v1/setup/join-codes", "gzip"},
		"app account":    {http.MethodGet, "/api/v1/domains/7/app/account", "gzip"},
		"vpn peer":       {http.MethodGet, "/api/v1/vpn/peers/3", "gzip"},
		"config file":    {http.MethodGet, "/api/v1/config?path=/etc/powerdns/pdns.conf", "gzip"},
		"licence":        {http.MethodGet, "/api/v1/license/access", "gzip"},
		"not an API":     {http.MethodGet, "/webmail/", "gzip"},
		"remote machine": {http.MethodGet, "/api/v1/dns/remote/receiver/status", "gzip"},
	} {
		w := serve(c.method, c.target, c.accept)
		if w.Header().Get("Content-Encoding") != "" {
			t.Errorf("%s: compressed", name)
		}
		if body := w.Body.String(); body != large && body != `{"ok":true}` {
			t.Errorf("%s: body changed (%d bytes)", name, len(body))
		}
	}
}

func TestPrecompressedAssetsAreServedToGzipClients(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	source := []byte("export const x = '" + strings.Repeat("panel ", 500) + "';\n")
	for name, data := range map[string][]byte{"assets/app-1.js": source, "assets/small.js": []byte("export {};\n"), "index.html": []byte("<!doctype html>")} {
		if err := os.WriteFile(filepath.Join(root, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	handler := frontendHandler(root)
	get := func(accept string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "/assets/app-1.js", nil)
		if accept != "" {
			r.Header.Set("Accept-Encoding", accept)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	w := get("gzip, br")
	if w.Code != http.StatusOK || w.Header().Get("Content-Encoding") != "gzip" ||
		!strings.HasPrefix(w.Header().Get("Content-Type"), "text/javascript") ||
		!strings.Contains(w.Header().Get("Cache-Control"), "immutable") ||
		!strings.Contains(w.Header().Get("Vary"), "Accept-Encoding") {
		t.Fatalf("gzip client: %d %v", w.Code, w.Header())
	}
	if !bytes.Equal(gunzip(t, w.Body.Bytes()), source) {
		t.Fatal("the stored copy is not the asset")
	}
	w = get("")
	if w.Header().Get("Content-Encoding") != "" || !bytes.Equal(w.Body.Bytes(), source) {
		t.Fatalf("identity client: %v", w.Header())
	}
	// The copy is made once and is the same bytes on the next request.
	first := get("gzip").Body.Bytes()
	if again := get("gzip").Body.Bytes(); !bytes.Equal(first, again) {
		t.Fatal("the compressed copy changed between requests")
	}
	// A replaced file is compressed again, not served from the old copy.
	changed := []byte("export const y = '" + strings.Repeat("other ", 600) + "';\n")
	if err := os.WriteFile(filepath.Join(root, "assets", "app-1.js"), changed, 0o644); err != nil {
		t.Fatal(err)
	}
	if w = get("gzip"); !bytes.Equal(gunzip(t, w.Body.Bytes()), changed) {
		t.Fatal("a replaced asset was served from its old compressed copy")
	}
	// A small file is served as it is.
	r := httptest.NewRequest(http.MethodGet, "/assets/small.js", nil)
	r.Header.Set("Accept-Encoding", "gzip")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Header().Get("Content-Encoding") != "" || w.Body.String() != "export {};\n" {
		t.Fatalf("small asset: %v %q", w.Header(), w.Body.String())
	}
	// A link in place of an asset is never compressed.
	if err := os.Symlink(filepath.Join(root, "assets", "app-1.js"), filepath.Join(root, "assets", "link.js")); err == nil {
		r = httptest.NewRequest(http.MethodGet, "/assets/link.js", nil)
		r.Header.Set("Accept-Encoding", "gzip")
		w = httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Header().Get("Content-Encoding") != "" {
			t.Fatal("a symbolic link was compressed")
		}
	}
}
