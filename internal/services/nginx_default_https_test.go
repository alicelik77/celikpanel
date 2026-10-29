package services

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var (
	nginx124 = NginxFeatures{Known: true, Version: NginxVersion{1, 24, 0}, HTTP2Module: true}
	nginx126 = NginxFeatures{Known: true, Version: NginxVersion{1, 26, 3}, HTTP2Module: true}
)

func TestDefaultHTTPSServerRender(t *testing.T) {
	policy, err := NginxTLSPolicy()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name     string
		features NginxFeatures
		listen   string
		http2On  bool
	}{
		{"nginx 1.24", nginx124, "    listen 443 ssl http2 default_server;\n    listen [::]:443 ssl http2 default_server;\n", false},
		{"nginx 1.26", nginx126, "    listen 443 ssl default_server;\n    listen [::]:443 ssl default_server;\n", true},
		{"unknown", NginxFeatures{}, "    listen 443 ssl default_server;\n    listen [::]:443 ssl default_server;\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, err := RenderDefaultHTTPSServer(tc.features)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(body, "server {\n"+tc.listen) {
				t.Errorf("listen lines:\n%s", body)
			}
			if strings.Contains(body, "http2 on;") != tc.http2On {
				t.Errorf("http2 on; = %v, want %v", !tc.http2On, tc.http2On)
			}
			for _, want := range []string{"    ssl_reject_handshake on;\n", "    server_name _;\n", "    ssl_session_cache shared:SSL:10m;\n" + policy + "\n}\n"} {
				if !strings.Contains(body, want) {
					t.Errorf("body lacks %q:\n%s", want, body)
				}
			}
			// No certificate: the handshake is refused before one is needed.
			if strings.Contains(body, "ssl_certificate") {
				t.Error("the default server names a certificate")
			}
		})
	}
}

func TestHTTPSListenFacts(t *testing.T) {
	own := "/etc/nginx/conf.d/celikpanel-default-https.conf"
	dump := func(files ...string) string { return strings.Join(files, "") }
	site := "# configuration file /etc/nginx/sites-enabled/a.conf:\nserver {\n    listen 443 ssl http2;\n    listen [::]:443 ssl http2;\n    server_name a.example;\n}\n"
	plain := "# configuration file /etc/nginx/sites-enabled/default:\nserver {\n    listen 80 default_server;\n    listen [::]:80 default_server;\n    # listen 443 ssl default_server;\n}\n"
	ours := "# configuration file " + own + ":\nserver {\n    listen 443 ssl default_server;\n    listen [::]:443 ssl default_server;\n    ssl_reject_handshake on;\n}\n"
	ownerDefault := "# configuration file /etc/nginx/conf.d/mine.conf:\nserver {\n    listen 443 ssl default_server;\n    server_name _;\n}\n"
	ownerV6Default := "# configuration file /etc/nginx/conf.d/mine6.conf:\nserver {\n    listen [::]:443 ssl default;\n}\n"
	boundDefault := "# configuration file /etc/nginx/conf.d/bound.conf:\nserver {\n    listen 192.0.2.10:443 ssl default_server;\n}\n"
	stream := "# configuration file /etc/nginx/stream.conf:\nserver {\n    listen 443;\n    proxy_pass backend;\n}\n"
	for _, tc := range []struct {
		name        string
		dump        string
		serves443   bool
		ownerDefult string
	}{
		{"distribution default site only", dump(plain), false, ""},
		{"a site with HTTPS", dump(plain, site), true, ""},
		{"only CelikPanel's own default", dump(plain, ours), false, ""},
		{"owner default on 443", dump(plain, site, ownerDefault, ours), true, "/etc/nginx/conf.d/mine.conf"},
		{"owner default on [::]:443", dump(site, ownerV6Default), true, "/etc/nginx/conf.d/mine6.conf"},
		{"a default on one address is another socket", dump(site, boundDefault), true, ""},
		{"a stream proxy on 443 is not HTTPS", dump(plain, stream), false, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			serves, owner := httpsListenFacts(tc.dump, own)
			if serves != tc.serves443 || owner != tc.ownerDefult {
				t.Fatalf("got (%v, %q), want (%v, %q)", serves, owner, tc.serves443, tc.ownerDefult)
			}
		})
	}
}

type defaultHTTPSHarness struct {
	ng        *NginxGenerator
	path      string
	dump      string
	dumpErr   error
	validate  error
	reload    error
	validates int
	reloads   int
}

func newDefaultHTTPSHarness(t *testing.T, features NginxFeatures) *defaultHTTPSHarness {
	t.Helper()
	previous := nginxDir
	nginxDir = t.TempDir()
	t.Cleanup(func() { nginxDir = previous })
	ng, err := NewNginxGenerator()
	if err != nil {
		t.Fatal(err)
	}
	h := &defaultHTTPSHarness{ng: ng, path: DefaultHTTPSServerPath()}
	ng.SetNginxFeatures(features)
	ng.nginxDump = func() (string, error) { return h.dump, h.dumpErr }
	ng.validateNginx = func() error { h.validates++; return h.validate }
	ng.reloadNginx = func() error { h.reloads++; return h.reload }
	return h
}

const dumpWithHTTPSSite = "# configuration file /etc/nginx/sites-enabled/a.conf:\nserver {\n    listen 443 ssl;\n    server_name a.example;\n}\n"

func TestReconcileDefaultHTTPSServerWritesOnceAndKeepsItsText(t *testing.T) {
	h := newDefaultHTTPSHarness(t, nginx126)
	h.dump = dumpWithHTTPSSite
	result := h.ng.ReconcileDefaultHTTPSServer()
	if result.Outcome != DefaultHTTPSWritten || h.validates != 1 || h.reloads != 1 {
		t.Fatalf("first run: %+v (validates %d reloads %d)", result, h.validates, h.reloads)
	}
	content, err := os.ReadFile(h.path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(content), managedRenderHeaderPrefix) || !strings.Contains(string(content), "ssl_reject_handshake on;") {
		t.Fatalf("written file:\n%s", content)
	}
	if info, err := os.Stat(h.path); err != nil || info.Mode().Perm() != 0o644 {
		t.Fatalf("mode %v (%v)", info.Mode().Perm(), err)
	}
	// Our own file in the dump does not count as "serves 443".
	h.dump += "# configuration file " + h.path + ":\n" + string(content)
	if result := h.ng.ReconcileDefaultHTTPSServer(); result.Outcome != DefaultHTTPSUnchanged || h.reloads != 1 {
		t.Fatalf("second run: %+v (reloads %d)", result, h.reloads)
	}
	// nginx is upgraded from 1.24 to 1.26 or back: the file follows.
	h.ng.SetNginxFeatures(nginx124)
	if result := h.ng.ReconcileDefaultHTTPSServer(); result.Outcome != DefaultHTTPSWritten || h.reloads != 2 {
		t.Fatalf("after a version change: %+v", result)
	}
	content, _ = os.ReadFile(h.path)
	if !strings.Contains(string(content), "listen 443 ssl http2 default_server;") {
		t.Fatalf("1.24 form not written:\n%s", content)
	}
}

func TestReconcileDefaultHTTPSServerLeavesTheOwnersChoice(t *testing.T) {
	t.Run("owner edited the file", func(t *testing.T) {
		h := newDefaultHTTPSHarness(t, nginx126)
		h.dump = dumpWithHTTPSSite
		if r := h.ng.ReconcileDefaultHTTPSServer(); r.Outcome != DefaultHTTPSWritten {
			t.Fatal(r)
		}
		content, _ := os.ReadFile(h.path)
		edited := strings.Replace(string(content), "server_name _;", "server_name _; # mine", 1)
		if err := os.WriteFile(h.path, []byte(edited), 0o644); err != nil {
			t.Fatal(err)
		}
		h.dump = ""
		r := h.ng.ReconcileDefaultHTTPSServer()
		after, _ := os.ReadFile(h.path)
		if r.Outcome != DefaultHTTPSKept || string(after) != edited || h.reloads != 1 {
			t.Fatalf("an edited file is kept even when not needed: %+v", r)
		}
	})
	t.Run("owner replaced it with a comment", func(t *testing.T) {
		h := newDefaultHTTPSHarness(t, nginx126)
		h.dump = dumpWithHTTPSSite
		if err := os.MkdirAll(filepath.Dir(h.path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(h.path, []byte("# off\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if r := h.ng.ReconcileDefaultHTTPSServer(); r.Outcome != DefaultHTTPSKept || h.validates != 0 {
			t.Fatalf("%+v", r)
		}
	})
	t.Run("owner has a default server of 443", func(t *testing.T) {
		h := newDefaultHTTPSHarness(t, nginx126)
		h.dump = dumpWithHTTPSSite + "# configuration file /etc/nginx/conf.d/mine.conf:\nserver {\n    listen 443 ssl default_server;\n}\n"
		if r := h.ng.ReconcileDefaultHTTPSServer(); r.Outcome != DefaultHTTPSOwnerHasOne || h.validates != 0 {
			t.Fatalf("%+v", r)
		}
		if _, err := os.Stat(h.path); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("written beside the owner's default server")
		}
	})
}

func TestReconcileDefaultHTTPSServerRemovesItsOwnWhenNoHTTPSRemains(t *testing.T) {
	h := newDefaultHTTPSHarness(t, nginx126)
	h.dump = dumpWithHTTPSSite
	if r := h.ng.ReconcileDefaultHTTPSServer(); r.Outcome != DefaultHTTPSWritten {
		t.Fatal(r)
	}
	h.dump = "# configuration file /etc/nginx/sites-enabled/default:\nserver {\n    listen 80 default_server;\n}\n"
	if r := h.ng.ReconcileDefaultHTTPSServer(); r.Outcome != DefaultHTTPSRemoved || h.reloads != 2 {
		t.Fatalf("%+v", r)
	}
	if _, err := os.Stat(h.path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("not removed")
	}
	if r := h.ng.ReconcileDefaultHTTPSServer(); r.Outcome != DefaultHTTPSNotNeeded || h.reloads != 2 {
		t.Fatalf("%+v", r)
	}
}

func TestReconcileDefaultHTTPSServerPutsBackOnRefusal(t *testing.T) {
	t.Run("nginx -t refuses a new file", func(t *testing.T) {
		h := newDefaultHTTPSHarness(t, nginx126)
		h.dump = dumpWithHTTPSSite
		h.validate = errors.New("nginx: [emerg] a duplicate default server for 0.0.0.0:443")
		r := h.ng.ReconcileDefaultHTTPSServer()
		if r.Outcome != DefaultHTTPSFailed || h.reloads != 0 || !strings.Contains(r.Detail, "duplicate default server") {
			t.Fatalf("%+v", r)
		}
		if _, err := os.Stat(h.path); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("a refused file stayed")
		}
	})
	t.Run("reload fails over an earlier text", func(t *testing.T) {
		h := newDefaultHTTPSHarness(t, nginx124)
		h.dump = dumpWithHTTPSSite
		if r := h.ng.ReconcileDefaultHTTPSServer(); r.Outcome != DefaultHTTPSWritten {
			t.Fatal(r)
		}
		before, _ := os.ReadFile(h.path)
		h.ng.SetNginxFeatures(nginx126)
		h.reload = errors.New("Job for nginx.service failed")
		r := h.ng.ReconcileDefaultHTTPSServer()
		after, _ := os.ReadFile(h.path)
		if r.Outcome != DefaultHTTPSFailed || string(after) != string(before) {
			t.Fatalf("%+v\n%s", r, after)
		}
		// The put-back configuration is tested and reloaded again.
		if h.validates != 3 || h.reloads != 3 {
			t.Fatalf("validates %d reloads %d", h.validates, h.reloads)
		}
	})
	t.Run("unreadable nginx -T changes nothing", func(t *testing.T) {
		h := newDefaultHTTPSHarness(t, nginx126)
		h.dumpErr = errors.New("nginx: [emerg] unknown directive")
		if r := h.ng.ReconcileDefaultHTTPSServer(); r.Outcome != DefaultHTTPSUnknown || h.validates != 0 {
			t.Fatalf("%+v", r)
		}
	})
	t.Run("nginx too old or unknown", func(t *testing.T) {
		for _, features := range []NginxFeatures{{}, {Known: true, Version: NginxVersion{1, 18, 0}, HTTP2Module: true}} {
			h := newDefaultHTTPSHarness(t, features)
			h.dump = dumpWithHTTPSSite
			if r := h.ng.ReconcileDefaultHTTPSServer(); r.Outcome != DefaultHTTPSUnsupported || h.validates != 0 {
				t.Fatalf("%+v", r)
			}
		}
	})
}
