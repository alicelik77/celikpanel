package services

import (
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.128 (PLESK-GAP G26): the site's own error pages.
func TestSiteErrorPagesAreFilesOfTheSite(t *testing.T) {
	bad := []transport.SiteErrorPage{
		{Code: 200, Path: "/ok.html"},
		{Code: 404, Path: "errors/404.html"},
		{Code: 404, Path: "/../etc/passwd.html"},
		{Code: 404, Path: "/errors/404.php"},
		{Code: 404, Path: "/.hidden/404.html"},
		{Code: 404, Path: "https://example.org/404.html"},
	}
	for _, page := range bad {
		if _, err := PrepareSiteWebSettings("php", SiteWebSettings{ErrorPages: []transport.SiteErrorPage{page}}); err == nil {
			t.Errorf("%+v accepted", page)
		}
	}
	if _, err := PrepareSiteWebSettings("node", SiteWebSettings{ErrorPages: []transport.SiteErrorPage{{Code: 404, Path: "/404.html"}}}); err == nil {
		t.Error("error pages accepted for a Node.js site")
	}
	if _, err := PrepareSiteWebSettings("php", SiteWebSettings{ErrorPages: []transport.SiteErrorPage{{Code: 404, Path: "/a.html"}, {Code: 404, Path: "/b.html"}}}); err == nil {
		t.Error("a code listed twice was accepted")
	}
	got, err := PrepareSiteWebSettings("static", SiteWebSettings{ErrorPages: []transport.SiteErrorPage{{Code: 503, Path: "/errors/busy.htm"}, {Code: 404, Path: "/errors/404.html"}}})
	if err != nil {
		t.Fatal(err)
	}
	body := RenderSiteSettingsPart("ornek.com", got, true)
	if !strings.Contains(body, "error_page 404 /errors/404.html;\nerror_page 503 /errors/busy.htm;\n") {
		t.Fatalf("body:\n%s", body)
	}
	if file := SiteErrorPageFile("/var/www/x/public_html/", "/errors/404.html"); file != "/var/www/x/public_html/errors/404.html" {
		t.Fatalf("file: %s", file)
	}
}
