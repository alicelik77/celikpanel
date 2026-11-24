package main

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

// Reverse proxy for the database web tools (phpMyAdmin, phpPgAdmin). The
// agent serves them on loopback only (127.0.0.1:8306); this proxy is the sole
// way in, and it demands an authenticated panel session first — the tools are
// never exposed to the network and no firewall port opens for them. Any role
// may pass (B1 role split, Jul 17): the real authorization layer is the
// tools' own login — a tenant holds credentials only for their own database
// users, which the panel creates tenant-scoped.
//
// The panel's own credentials never reach a tool (security lane A1, 24
// November 2026): towards the tool the proxy removes the panel's sign-in
// cookies under every name they have had (the "Log in as" cookie holds the
// administrator's own session) and the Authorization header; from the tool it
// drops any cookie a tool tries to set under the panel's names or with the
// __Host- or __Secure- prefixes, which a response could otherwise use to
// overwrite the panel's sign-in, and any cookie without a name (a browser
// keeps "=celikpanel_session=x" as a nameless cookie and sends it back as
// "celikpanel_session=x"), and Clear-Site-Data, with which a tool could sign
// the person out of the panel. Informational (1xx) responses, which the
// reverse proxy relays without ModifyResponse, are filtered the same way.
//
// Veritabanı web araçları (phpMyAdmin, phpPgAdmin) için ters vekil. Agent
// onları yalnız loopback'te sunar (127.0.0.1:8306); içeri tek yol bu vekildir
// ve önce kimlik doğrulamalı bir panel oturumu ister — araçlar ağa asla
// açılmaz, onlar için güvenlik duvarında port da açılmaz. Her rol geçebilir
// (B1 rol ayrımı, 17 Tem): gerçek yetki katmanı araçların kendi girişidir —
// kiracının elinde yalnız kendi veritabanı kullanıcılarının kimlik bilgileri
// vardır; panel onları kiracı-kapsamlı oluşturur.

var dbToolProxy = newDBToolProxy(&url.URL{Scheme: "http", Host: "127.0.0.1:8306"})

func newDBToolProxy(target *url.URL) http.Handler {
	proxy := httputil.NewSingleHostReverseProxy(target)
	director := proxy.Director
	proxy.Director = func(req *http.Request) {
		director(req)
		stripPanelCredentials(req.Header)
	}
	proxy.ModifyResponse = func(resp *http.Response) error {
		filterToolResponseHeader(resp.Header)
		return nil
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxy.ServeHTTP(&dbToolResponseWriter{ResponseWriter: w, panelHeaders: panelSecurityHeadersOf(w.Header())}, r)
	})
}

// panelSecurityHeadersOf copies the panel's security headers (securityHeaders,
// set before the proxy runs) from a response's header.
func panelSecurityHeadersOf(header http.Header) http.Header {
	kept := http.Header{}
	for _, name := range panelSecurityHeaderNames {
		if values := header.Values(name); len(values) > 0 {
			kept[http.CanonicalHeaderKey(name)] = append([]string(nil), values...)
		}
	}
	return kept
}

// filterToolResponseHeader removes from a tool's response what could touch
// the panel's sign-in: Set-Cookie under the panel's names or without a name,
// and Clear-Site-Data.
func filterToolResponseHeader(header http.Header) {
	dropPanelNamedSetCookies(header)
	header.Del("Clear-Site-Data")
}

// dbToolResponseWriter filters the headers of informational (1xx) responses:
// httputil.ReverseProxy relays those from its Got1xxResponse trace straight
// to the client, without ModifyResponse. After relaying one it clears every
// header already set, the panel's security headers too, so the final
// response gets the panel's own values of those back (panelHeaders, read
// before the proxy ran), replacing whatever the tool sent under those names
// (second look at PR #368, B). Unwrap keeps flushing and protocol upgrades
// (http.ResponseController) working through it.
type dbToolResponseWriter struct {
	http.ResponseWriter
	panelHeaders http.Header
	final        bool
}

func (w *dbToolResponseWriter) WriteHeader(code int) {
	if code >= 100 && code < 200 {
		filterToolResponseHeader(w.Header())
	} else if !w.final {
		w.final = true
		header := w.Header()
		for _, name := range panelSecurityHeaderNames {
			header.Del(name)
			for _, value := range w.panelHeaders.Values(name) {
				header.Add(name, value)
			}
		}
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *dbToolResponseWriter) Write(body []byte) (int, error) {
	if !w.final {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}

func (w *dbToolResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// stripPanelCredentials removes the panel's own cookies (by exact name, old
// and new) and Authorization from a request headed for a tool. Every other
// cookie, the tools' own, passes unchanged.
func stripPanelCredentials(header http.Header) {
	header.Del("Authorization")
	values := header.Values("Cookie")
	if len(values) == 0 {
		return
	}
	var kept []string
	for _, value := range values {
		for _, part := range strings.Split(value, ";") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			name, _, _ := strings.Cut(part, "=")
			if panelCookieNames[strings.TrimSpace(name)] {
				continue
			}
			kept = append(kept, part)
		}
	}
	header.Del("Cookie")
	if len(kept) > 0 {
		header.Set("Cookie", strings.Join(kept, "; "))
	}
}

// dropPanelNamedSetCookies removes every Set-Cookie of a tool's response
// whose name starts with "celikpanel", "__Host-" or "__Secure-" (compared
// without regard to case): those names belong to the panel. A Set-Cookie
// whose name is empty is removed too, whatever its value: a browser stores
// "=celikpanel_session=x" as a nameless cookie and sends it back as
// "celikpanel_session=x", and the panel cannot remove a nameless cookie.
func dropPanelNamedSetCookies(header http.Header) {
	values := header.Values("Set-Cookie")
	if len(values) == 0 {
		return
	}
	var kept []string
	for _, value := range values {
		name, _, _ := strings.Cut(value, "=")
		name = strings.TrimSpace(name)
		if name == "" || panelReservedCookieName(name) {
			continue
		}
		kept = append(kept, value)
	}
	header.Del("Set-Cookie")
	for _, value := range kept {
		header.Add("Set-Cookie", value)
	}
}

func panelReservedCookieName(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasPrefix(lower, "celikpanel") || strings.HasPrefix(lower, "__host-") || strings.HasPrefix(lower, "__secure-")
}

func (p *Panel) handleDBToolProxy(w http.ResponseWriter, r *http.Request) {
	// requireAuth has already validated the session; this is defence in
	// depth against a future public-path regression.
	// requireAuth oturumu zaten doğruladı; bu, ileride olası bir açık-yol
	// gerilemesine karşı derinlemesine savunmadır.
	if currentCaller(r) == nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	dbToolProxy.ServeHTTP(w, r)
}
