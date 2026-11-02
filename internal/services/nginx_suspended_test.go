package services

import (
	"strings"
	"testing"
)

// A suspended site (alpha.117) serves only the suspended page and the ACME
// challenge location; none of the site's content paths may survive.
func TestSuspendedVhostServesOnlyThePageAndValidation(t *testing.T) {
	ng, err := NewNginxGenerator()
	if err != nil {
		t.Fatal(err)
	}
	base := []VhostData{
		{SiteID: 1, Domain: "ornek.com", DocumentRoot: "/var/www/ornek", ProjectType: "php", PHPSocket: "/run/php/php8.3-fpm.sock"},
		{SiteID: 2, Domain: "ornek.com", DocumentRoot: "/var/www/ornek", ProjectType: "static"},
		{SiteID: 3, Domain: "ornek.com", DocumentRoot: "/var/www/ornek", ProjectType: "node", AppPort: 3000},
		{SiteID: 4, Domain: "ornek.com", DocumentRoot: "/var/www/ornek", ProjectType: "proxy", ForwardTo: "http://10.0.0.5:8080"},
		{SiteID: 5, Domain: "ornek.com", DocumentRoot: "/var/www/ornek", ProjectType: "forwarding", ForwardTo: "https://example.net"},
	}
	for _, data := range base {
		for _, tls := range []bool{false, true} {
			d := data
			d.ACMEChallengeRoot = testACMEChallengeRoot
			d.Suspended = true
			d.RedirectWWW = false
			if tls {
				d.SSLType = "letsencrypt"
				d.SSLCert = "/etc/letsencrypt/live/ornek.com/fullchain.pem"
				d.SSLKey = "/etc/letsencrypt/live/ornek.com/privkey.pem"
				d.ForceHTTPS = true
			}
			out, err := ng.Render(d)
			if err != nil {
				t.Fatalf("%s tls=%v: %v", d.ProjectType, tls, err)
			}
			for _, banned := range []string{"root /var/www", "fastcgi_pass", "proxy_pass", "return 301", "include " + OwnerIncludeDir(d.Domain), "10.0.0.5", "example.net"} {
				if strings.Contains(out, banned) {
					t.Errorf("%s tls=%v: suspended vhost still has %q", d.ProjectType, tls, banned)
				}
			}
			for _, want := range []string{"return 503 '", "This site is suspended", "Bu site askıya alınmıştır", "add_header Retry-After", "include " + PanelManagedDir(d.Domain) + "/*.conf;", "celikpanel-suspended"} {
				if !strings.Contains(out, want) {
					t.Errorf("%s tls=%v: suspended vhost lacks %q", d.ProjectType, tls, want)
				}
			}
			if got := strings.Count(out, "server {"); (tls && got != 2) || (!tls && got != 1) {
				t.Errorf("%s tls=%v: %d server blocks:\n%s", d.ProjectType, tls, got, out)
			}
			if tls && !strings.Contains(out, "ssl_certificate /etc/letsencrypt/live/ornek.com/fullchain.pem;") {
				t.Errorf("%s: suspended HTTPS block lost its certificate", d.ProjectType)
			}
			// nginx would expand a $ inside the page text as a variable.
			page := out[strings.Index(out, "return 503 '"):]
			page = page[:strings.Index(page, "';")]
			if strings.ContainsAny(page, "$") {
				t.Errorf("suspended page text carries a $: %s", page)
			}
		}
	}
	// Not suspended: the ordinary render is unchanged by the new branch.
	normal, err := ng.Render(VhostData{SiteID: 6, Domain: "ornek.com", DocumentRoot: "/var/www/ornek", ProjectType: "static", ACMEChallengeRoot: testACMEChallengeRoot})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(normal, "celikpanel-suspended") || !strings.Contains(normal, "root /var/www/ornek;") {
		t.Fatal("an active site rendered the suspended page")
	}
}
