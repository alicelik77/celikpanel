package services

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"os"
	"strings"
	"testing"
	"text/template"

	"github.com/alicelik/celikpanel/internal/core"
)

var nginx118 = NginxFeatures{Known: true, Version: NginxVersion{1, 18, 0}, HTTP2Module: true}

// The ssl_reject_handshake form is byte for byte what it was when first
// written (testdata, frozen), so a file already in place on nginx >= 1.19.4
// stays CelikPanel's unchanged text and is not rewritten.
func TestDefaultHTTPSRejectFormIsUnchanged(t *testing.T) {
	frozen, err := os.ReadFile("testdata/default-https-reject-first.conf.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	parsed := template.Must(template.New("frozen").Parse(strings.ReplaceAll(string(frozen), "\r\n", "\n")))
	policy, err := NginxTLSPolicy()
	if err != nil {
		t.Fatal(err)
	}
	for _, features := range []NginxFeatures{nginx124, nginx126} {
		var want bytes.Buffer
		if err := parsed.Execute(&want, struct{ HTTP2, TLSPolicy string }{features.HTTP2Form(), policy}); err != nil {
			t.Fatal(err)
		}
		got, err := RenderDefaultHTTPSServer(features)
		if err != nil {
			t.Fatal(err)
		}
		if got != want.String() {
			t.Fatalf("nginx %s: the reject form changed:\n%s\nwant:\n%s", features.Version, got, want.String())
		}
	}
}

// nginx older than 1.19.4 (Ubuntu 22.04: 1.18.0), or one whose version could
// not be read: a placeholder certificate that names no site, and 444.
func TestDefaultHTTPSPlaceholderRender(t *testing.T) {
	policy, err := NginxTLSPolicy()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name     string
		features NginxFeatures
		listen   string
	}{
		{"nginx 1.18", nginx118, "    listen 443 ssl http2 default_server;\n    listen [::]:443 ssl http2 default_server;\n"},
		{"unknown", NginxFeatures{}, "    listen 443 ssl default_server;\n    listen [::]:443 ssl default_server;\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if DefaultHTTPSMode(tc.features) != DefaultHTTPSModePlaceholder {
				t.Fatal("mode")
			}
			body, err := RenderDefaultHTTPSServer(tc.features)
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{
				"server {\n" + tc.listen,
				"    server_name _;\n\n    ssl_certificate " + DefaultHTTPSPlaceholderCertPath() + ";\n    ssl_certificate_key " + DefaultHTTPSPlaceholderKeyPath() + ";\n",
				"    ssl_session_cache shared:SSL:10m;\n" + policy + "\n\n    # Close the connection without an answer.\n    return 444;\n}\n",
				DefaultHTTPSPlaceholderName,
			} {
				if !strings.Contains(body, want) {
					t.Errorf("body lacks %q:\n%s", want, body)
				}
			}
			if strings.Contains(body, "ssl_reject_handshake") || strings.Contains(body, "http2 on;") {
				t.Errorf("a directive this nginx does not know:\n%s", body)
			}
		})
	}
}

func TestReconcileDefaultHTTPSServerPlaceholder(t *testing.T) {
	h := newDefaultHTTPSHarness(t, nginx118)
	h.dump = dumpWithHTTPSSite
	r := h.ng.ReconcileDefaultHTTPSServer()
	if r.Outcome != DefaultHTTPSWritten || r.Mode != DefaultHTTPSModePlaceholder || h.validates != 1 || h.reloads != 1 {
		t.Fatalf("%+v (validates %d reloads %d)", r, h.validates, h.reloads)
	}
	certPEM, err := os.ReadFile(DefaultHTTPSPlaceholderCertPath())
	if err != nil {
		t.Fatal(err)
	}
	keyPEM, err := os.ReadFile(DefaultHTTPSPlaceholderKeyPath())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tls.X509KeyPair(certPEM, keyPEM); err != nil {
		t.Fatalf("not a pair: %v", err)
	}
	block, _ := pem.Decode(certPEM)
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if cert.Subject.CommonName != DefaultHTTPSPlaceholderName || len(cert.DNSNames) != 1 || cert.DNSNames[0] != DefaultHTTPSPlaceholderName {
		t.Fatalf("names: %q %q", cert.Subject.CommonName, cert.DNSNames)
	}
	if info, err := os.Stat(DefaultHTTPSPlaceholderKeyPath()); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("key mode %v (%v)", info.Mode().Perm(), err)
	}
	content, _ := os.ReadFile(h.path)
	if !strings.Contains(string(content), "return 444;") {
		t.Fatalf("written:\n%s", content)
	}

	// The pair is made once; the next run keeps it and the file.
	h.dump += "# configuration file " + h.path + ":\n" + string(content)
	if r := h.ng.ReconcileDefaultHTTPSServer(); r.Outcome != DefaultHTTPSUnchanged || h.reloads != 1 {
		t.Fatalf("second run: %+v", r)
	}
	again, _ := os.ReadFile(DefaultHTTPSPlaceholderCertPath())
	if !bytes.Equal(again, certPEM) {
		t.Fatal("the pair was made again")
	}

	// nginx upgraded to 1.26: the reject form replaces it.
	h.ng.SetNginxFeatures(nginx126)
	if r := h.ng.ReconcileDefaultHTTPSServer(); r.Outcome != DefaultHTTPSWritten || r.Mode != DefaultHTTPSModeReject {
		t.Fatalf("after the upgrade: %+v", r)
	}
}

func TestReconcileDefaultHTTPSServerPlaceholderPairProblems(t *testing.T) {
	t.Run("an unusable pair is reported and left", func(t *testing.T) {
		h := newDefaultHTTPSHarness(t, nginx118)
		h.dump = dumpWithHTTPSSite
		if err := os.WriteFile(DefaultHTTPSPlaceholderCertPath(), []byte("not a certificate\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		r := h.ng.ReconcileDefaultHTTPSServer()
		if r.Outcome != DefaultHTTPSFailed || h.validates != 0 || !strings.Contains(r.Detail, "remove both files") {
			t.Fatalf("%+v", r)
		}
		if _, err := os.Stat(h.path); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("the server file was written without a usable pair")
		}
		if content, _ := os.ReadFile(DefaultHTTPSPlaceholderCertPath()); string(content) != "not a certificate\n" {
			t.Fatal("the file was replaced")
		}
	})
	t.Run("a pair put there by the owner is used", func(t *testing.T) {
		h := newDefaultHTTPSHarness(t, nginx118)
		h.dump = dumpWithHTTPSSite
		certPEM, keyPEM, err := NewDefaultHTTPSPlaceholderPair()
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(DefaultHTTPSPlaceholderKeyPath(), keyPEM, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(DefaultHTTPSPlaceholderCertPath(), certPEM, 0o644); err != nil {
			t.Fatal(err)
		}
		if r := h.ng.ReconcileDefaultHTTPSServer(); r.Outcome != DefaultHTTPSWritten {
			t.Fatalf("%+v", r)
		}
		if again, _ := os.ReadFile(DefaultHTTPSPlaceholderCertPath()); !bytes.Equal(again, certPEM) {
			t.Fatal("the owner's pair was replaced")
		}
	})
	t.Run("a key left alone by an interrupted run is replaced", func(t *testing.T) {
		h := newDefaultHTTPSHarness(t, nginx118)
		h.dump = dumpWithHTTPSSite
		if err := os.WriteFile(DefaultHTTPSPlaceholderKeyPath(), []byte("half"), 0o600); err != nil {
			t.Fatal(err)
		}
		if r := h.ng.ReconcileDefaultHTTPSServer(); r.Outcome != DefaultHTTPSWritten {
			t.Fatalf("%+v", r)
		}
	})
}

// The state on the Nginx page is read from nginx -T and the file; reading
// never writes, validates or reloads.
func TestDefaultHTTPSServerStatus(t *testing.T) {
	ownerDefault := dumpWithHTTPSSite + "# configuration file /etc/nginx/conf.d/mine.conf:\nserver {\n    listen 443 ssl default_server;\n}\n"
	failed := &DefaultHTTPSServerResult{Outcome: DefaultHTTPSFailed, Detail: "nginx -t refused it"}
	for _, tc := range []struct {
		name     string
		features NginxFeatures
		file     string // "", "ours", or the file's own text
		dump     string
		dumpErr  error
		last     *DefaultHTTPSServerResult
		state    string
		mode     string
		detail   string
		owner    string
	}{
		{"no HTTPS site", nginx126, "", "", nil, nil, core.NginxDefaultHTTPSNotNeeded, DefaultHTTPSModeReject, "", ""},
		{"needed, not yet written", nginx118, "", dumpWithHTTPSSite, nil, nil, core.NginxDefaultHTTPSPending, DefaultHTTPSModePlaceholder, "", ""},
		{"needed, the last attempt failed", nginx126, "", dumpWithHTTPSSite, nil, failed, core.NginxDefaultHTTPSFailed, DefaultHTTPSModeReject, "nginx -t refused it", ""},
		{"in place", nginx126, "ours", dumpWithHTTPSSite, nil, failed, core.NginxDefaultHTTPSActive, DefaultHTTPSModeReject, "", ""},
		{"the owner's default server", nginx126, "", ownerDefault, nil, nil, core.NginxDefaultHTTPSOwnerDefault, DefaultHTTPSModeReject, "", "/etc/nginx/conf.d/mine.conf"},
		{"the owner's own text", nginx126, "# mine\n", dumpWithHTTPSSite, nil, nil, core.NginxDefaultHTTPSOwnerFile, DefaultHTTPSModeReject, "", ""},
		{"nginx -T unreadable", nginx126, "", "", errors.New("[emerg] unknown directive"), nil, core.NginxDefaultHTTPSUnknown, DefaultHTTPSModeReject, "nginx -T: [emerg] unknown directive", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newDefaultHTTPSHarness(t, tc.features)
			switch tc.file {
			case "":
			case "ours":
				body, err := RenderDefaultHTTPSServer(tc.features)
				if err != nil {
					t.Fatal(err)
				}
				writeDefaultHTTPSTestFile(t, h.path, SealManagedText(body))
			default:
				writeDefaultHTTPSTestFile(t, h.path, tc.file)
			}
			status := h.ng.DefaultHTTPSServerStatus(tc.dump, tc.dumpErr, tc.last)
			if status.State != tc.state || status.Mode != tc.mode || status.Detail != tc.detail || status.OwnerFile != tc.owner || status.Path != h.path {
				t.Fatalf("%+v", status)
			}
			if h.validates != 0 || h.reloads != 0 {
				t.Fatal("a read changed something")
			}
		})
	}
}

func writeDefaultHTTPSTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(strings.TrimSuffix(path, "/"+defaultHTTPSServerFileName), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
