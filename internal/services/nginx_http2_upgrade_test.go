package services

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"testing"
	"text/template"

	"github.com/alicelik/celikpanel/internal/transport"
)

// The vhost template of v0.1.0-alpha.112 (commit 265f7f8dbab7, blob
// 6832afc24e09, SHA-256 6a58192c…dc4461f), frozen as test data: the text
// installed panels wrote with `http2 on;` and the earlier TLS lines.
const alpha112TemplatePath = "testdata/vhost-v0.1.0-alpha.112.conf.tmpl"

func renderAlpha112(t *testing.T, data VhostData) string {
	t.Helper()
	source, err := os.ReadFile(alpha112TemplatePath)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(source)
	if got := hex.EncodeToString(sum[:]); got != "6a58192ce8716b3bb783dfecd761fb183dc42ca89e3c71e1384708211dc4461f" {
		t.Fatalf("the frozen alpha.112 template changed: %s", got)
	}
	parsed, err := template.New("alpha112").Parse(string(source))
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := prepareVhostData(data)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := parsed.Execute(&buf, prepared); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// A site file alpha.112 wrote is still CelikPanel's unchanged text after the
// template changed (its header seals its own body, D-031), so the first render
// of this release replaces it with the form the installed nginx accepts, not
// "edited by the owner". On Ubuntu 24.04 the alpha.112 HTTPS text was refused
// by nginx and put back, so the file there is the earlier HTTP text: the
// retried activation renders the 1.24 form, which nginx accepts.
//
// alpha.112'nin yazdığı dosya şablon değişse de CelikPanel'in değişmemiş
// metnidir; bu sürümün ilk üretimi onu kurulu nginx'in kabul ettiği biçimle
// değiştirir, "sahip düzenledi" sayılmaz.
func TestAlpha112SiteFilesAreReplacedByTheVersionForm(t *testing.T) {
	https := managedTestData("lacteria.com", "/var/run/php/php8.3-fpm-site7.sock")
	https.SSLType = "custom"
	https.SSLCert, https.SSLKey = "/etc/letsencrypt/live/lacteria.com/fullchain.pem", "/etc/letsencrypt/live/lacteria.com/privkey.pem"
	https.ForceHTTPS = true
	plain := managedTestData("lacteria.com", "/var/run/php/php8.3-fpm-site7.sock")

	for _, tc := range []struct {
		name     string
		features NginxFeatures
		onDisk   VhostData // what alpha.112 left on disk
		want     []string
		refused  []string
	}{
		{"Debian 13 (nginx 1.26): an active alpha.112 HTTPS file", nginx126, https,
			[]string{"    listen 443 ssl;\n    listen [::]:443 ssl;\n    http2 on;\n", "ssl_prefer_server_ciphers off;"},
			[]string{"ssl http2;", "HIGH:!aNULL:!MD5"}},
		{"Ubuntu 24.04 (nginx 1.24): the HTTP file alpha.112 kept after nginx refused HTTPS", nginx124, plain,
			[]string{"    listen 443 ssl http2;\n    listen [::]:443 ssl http2;\n", "ssl_session_tickets off;"},
			[]string{"http2 on;", "HIGH:!aNULL:!MD5"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newManagedVhostHarness(t)
			h.ng.SetNginxFeatures(tc.features)
			available, _ := vhostPaths(tc.onDisk.Domain)
			old := SealManagedText(renderAlpha112(t, tc.onDisk))
			writeTestFile(t, available, old, 0o644)

			results := h.apply(h.item(https, transport.SiteFileTriggerChange))
			if results[0].State != transport.SiteFileManagedUnchanged || results[0].Outcome != transport.SiteFileOutcomeWritten {
				t.Fatalf("alpha.112's file: %+v", results[0])
			}
			now := readTestFile(t, available)
			for _, want := range tc.want {
				if !strings.Contains(now, want) {
					t.Errorf("written file lacks %q:\n%s", want, now)
				}
			}
			for _, refused := range tc.refused {
				if strings.Contains(now, refused) {
					t.Errorf("written file still has %q", refused)
				}
			}
			if h.validates != 1 || h.reloads != 1 {
				t.Errorf("validates %d reloads %d", h.validates, h.reloads)
			}
		})
	}

	t.Run("an alpha.112 file the owner edited is kept", func(t *testing.T) {
		h := newManagedVhostHarness(t)
		h.ng.SetNginxFeatures(nginx124)
		available, _ := vhostPaths(https.Domain)
		edited := strings.Replace(SealManagedText(renderAlpha112(t, https)), "    http2 on;\n", "", 1)
		writeTestFile(t, available, edited, 0o644)
		results := h.apply(h.item(https, transport.SiteFileTriggerChange))
		if results[0].State != transport.SiteFileOwnerEdited || results[0].Outcome != transport.SiteFileOutcomeKept {
			t.Fatalf("%+v", results[0])
		}
		if readTestFile(t, available) != edited {
			t.Fatal("the owner's file was changed")
		}
	})
}
