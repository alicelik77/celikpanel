package services

import (
	"strings"
	"testing"
)

// A site under a security emergency (D-043) serves only the maintenance
// page and the ACME challenge location. Suspended wins when both are set.
func TestMaintenanceVhostServesOnlyThePageAndValidation(t *testing.T) {
	ng, err := NewNginxGenerator()
	if err != nil {
		t.Fatal(err)
	}
	for _, projectType := range []string{"php", "node"} {
		for _, tls := range []bool{false, true} {
			d := VhostData{SiteID: 1, Domain: "ornek.com", DocumentRoot: "/var/www/ornek", ProjectType: projectType,
				PHPSocket: "/run/php/php8.5-fpm.sock", AppPort: 3000, ACMEChallengeRoot: testACMEChallengeRoot, Maintenance: true}
			if projectType == "node" {
				d.PHPSocket = ""
			}
			if tls {
				d.SSLType = "letsencrypt"
				d.SSLCert = "/etc/letsencrypt/live/ornek.com/fullchain.pem"
				d.SSLKey = "/etc/letsencrypt/live/ornek.com/privkey.pem"
				d.ForceHTTPS = true
			}
			out, err := ng.Render(d)
			if err != nil {
				t.Fatalf("%s tls=%v: %v", projectType, tls, err)
			}
			for _, banned := range []string{"root /var/www", "fastcgi_pass", "proxy_pass", "celikpanel-suspended"} {
				if strings.Contains(out, banned) {
					t.Errorf("%s tls=%v: maintenance vhost still has %q", projectType, tls, banned)
				}
			}
			for _, want := range []string{"return 503 '", "This site is temporarily under maintenance", "Bu site geçici olarak bakımdadır", "celikpanel-maintenance", "include " + PanelManagedDir(d.Domain) + "/*.conf;"} {
				if !strings.Contains(out, want) {
					t.Errorf("%s tls=%v: maintenance vhost lacks %q", projectType, tls, want)
				}
			}
			page := out[strings.Index(out, "return 503 '"):]
			page = page[:strings.Index(page, "';")]
			if strings.ContainsAny(page, "$") {
				t.Errorf("maintenance page text carries a $: %s", page)
			}
			d.Suspended = true
			both, err := ng.Render(d)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(both, "celikpanel-suspended") || strings.Contains(both, "celikpanel-maintenance") {
				t.Errorf("%s tls=%v: suspended and maintenance did not render the suspended page", projectType, tls)
			}
		}
	}
}
