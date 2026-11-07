package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.128 (G26): an error page whose file is missing is refused by a save
// and left out of every other render; a link is never a page.
func TestErrorPagesMustBeFilesOfTheDocumentRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "errors"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "errors", "404.html"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = os.Symlink("/etc/hostname", filepath.Join(root, "errors", "500.html"))
	pages := []transport.SiteErrorPage{{Code: 404, Path: "/errors/404.html"}, {Code: 500, Path: "/errors/500.html"}, {Code: 503, Path: "/errors/503.html"}}
	present, missing := presentErrorPages(root, pages)
	if len(present) != 1 || present[0].Code != 404 || strings.Join(missing, ",") != "/errors/500.html,/errors/503.html" {
		t.Fatalf("present %+v missing %v", present, missing)
	}
	a := &Agent{}
	req := &ApplyVhostRequest{DocumentRoot: root, WebSettings: &transport.SiteWebSettingsRequest{
		PrecheckNginx: true, Settings: transport.SiteWebSettings{ErrorPages: pages},
	}}
	// Without a generator the precheck is skipped entirely (development).
	if got := a.precheckSiteWebSettings(req); got != "" {
		t.Fatalf("no generator: %q", got)
	}
}
