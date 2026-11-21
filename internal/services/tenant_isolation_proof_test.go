package services

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Tenant isolation, 20 Nov 2026 (lane S3). These tests use only what existed
// before the change, so they run unchanged on the earlier code and fail there.

const proofDocumentRoot = "/var/www/celikpanel/subscriptions/1/sites/2/public_html"

// serverBlocks splits a rendered vhost into its top-level server blocks.
func serverBlocks(vhost string) []string {
	var blocks []string
	var current []string
	inside := false
	for _, line := range strings.Split(vhost, "\n") {
		switch {
		case !inside && line == "server {":
			inside, current = true, []string{line}
		case inside && line == "}":
			blocks = append(blocks, strings.Join(append(current, line), "\n"))
			inside = false
		case inside:
			current = append(current, line)
		}
	}
	return blocks
}

// proofVhostVariants are renders of every kind of server block that serves a
// site's files: PHP, static and a Node.js site's public folder, over HTTP and
// HTTPS, with protected folders, redirects, the www redirect, the site's web
// server settings and the mail set-up answers.
func proofVhostVariants() map[string]VhostData {
	cert, key := "/etc/letsencrypt/live/ornek.com/fullchain.pem", "/etc/letsencrypt/live/ornek.com/privkey.pem"
	php := VhostData{SiteID: 2, Domain: "ornek.com", DocumentRoot: proofDocumentRoot, ProjectType: "php",
		PHPSocket: "/run/php/php8.4-fpm-site2.sock", SSLType: "none"}
	variants := map[string]VhostData{}
	add := func(name string, change func(*VhostData)) {
		data := php
		change(&data)
		variants[name] = data
	}
	add("php http", func(*VhostData) {})
	add("php https", func(d *VhostData) { d.SSLType, d.SSLCert, d.SSLKey = "custom", cert, key })
	add("php https forced, www", func(d *VhostData) {
		d.SSLType, d.SSLCert, d.SSLKey, d.ForceHTTPS = "custom", cert, key, true
		d.RedirectWWW, d.ServerNames = true, []string{"ornek.com", "www.ornek.com"}
	})
	add("php protected folder and redirect", func(d *VhostData) {
		d.ProtectedDirs = []SiteProtectedDir{{ID: 1, Path: "/private", Realm: "Private"}}
		d.Redirects = []SiteRedirect{{Match: "prefix", Source: "/old", Target: "/new", Code: 301}}
	})
	add("php site settings and mail set-up", func(d *VhostData) {
		d.SiteInclude = true
		d.MailAutoconfig = &MailAutoconfig{MailHost: "mail.ornek.com"}
	})
	add("static http", func(d *VhostData) { d.ProjectType, d.PHPSocket = "static", "" })
	add("static https, protected folder", func(d *VhostData) {
		d.ProjectType, d.PHPSocket = "static", ""
		d.SSLType, d.SSLCert, d.SSLKey = "custom", cert, key
		d.ProtectedDirs = []SiteProtectedDir{{ID: 3, Path: "/members", Realm: "Members"}}
	})
	add("node public folder", func(d *VhostData) {
		d.ProjectType, d.PHPSocket, d.AppPort = "node", "", 3001
		d.NodePublicRoot = proofDocumentRoot + "/public"
	})
	add("node public folder https", func(d *VhostData) {
		d.ProjectType, d.PHPSocket, d.AppPort = "node", "", 3001
		d.NodePublicRoot = proofDocumentRoot + "/public"
		d.SSLType, d.SSLCert, d.SSLKey = "custom", cert, key
	})
	return variants
}

// A link in one site's folder must not serve another account's file: every
// server block that serves the site's files follows a link only to a file of
// the link's own owner, over the whole path (no from=: the site's account can
// replace its own document root with a link).
func TestTenantIsolationEveryBlockThatServesFilesRefusesForeignLinks(t *testing.T) {
	for name, data := range proofVhostVariants() {
		vhost := renderTestVhost(t, data)
		serving := 0
		for _, block := range serverBlocks(vhost) {
			if strings.Contains(block, "disable_symlinks off") || strings.Contains(block, "from=") {
				t.Fatalf("%s: a block lifts or narrows the link rule:\n%s", name, block)
			}
			if !strings.Contains(block, "\n    root ") {
				continue
			}
			serving++
			if strings.Count(block, "\n    disable_symlinks if_not_owner;\n") != 1 {
				t.Errorf("%s: a block that serves the site's files follows every link:\n%s", name, block)
			}
		}
		if serving == 0 {
			t.Fatalf("%s: no block serves the site's files:\n%s", name, vhost)
		}
	}
}

// A reseller's or customer's additional directives cannot lift the link rule,
// list a folder without it, make nginx read a file as root, or change the
// PHP limits of the site's plan through a FastCGI parameter.
func TestTenantIsolationCustomerDirectivesCannotLiftTheRules(t *testing.T) {
	home := "/var/www/celikpanel/subscriptions/1/sites/2"
	for _, text := range []string{
		"location /x/ { disable_symlinks off; }",
		"disable_symlinks off;",
		"location /files/ { autoindex on; }",
		"location /r/ { random_index on; }",
		"location /x/ { xslt_stylesheet " + home + "/style.xslt; }",
		"fastcgi_param PHP_ADMIN_VALUE \"memory_limit=-1\";",
		"location /a/ { fastcgi_param PHP_VALUE \"max_execution_time=0\"; }",
		"fastcgi_param php_admin_value \"open_basedir=/\";",
	} {
		var refusal *SiteDirectiveRefusal
		if err := ValidateSiteDirectives(text, false, home); !errors.As(err, &refusal) {
			t.Errorf("%q was accepted for a customer (%v)", text, err)
		}
	}
	// What customers keep: other FastCGI parameters, their own password file
	// by its path inside the site, and everything for the administrator.
	for _, text := range []string{
		"fastcgi_param APP_ENV production;",
		"location /p { auth_basic \"x\"; auth_basic_user_file " + home + "/.htpasswd; }",
	} {
		if err := ValidateSiteDirectives(text, false, home); err != nil {
			t.Errorf("%q: %v", text, err)
		}
	}
	if err := ValidateSiteDirectives("location /x/ { autoindex on; disable_symlinks off; }", true, ""); err != nil {
		t.Fatalf("administrator: %v", err)
	}
}

// A per-site nginx file CelikPanel creates is not readable by other accounts
// (site accounts learn other sites' names, folders, ports and proxy targets
// from it).
func TestTenantIsolationNewSiteFilesAreNotWorldReadable(t *testing.T) {
	dir := t.TempDir()
	vhost := filepath.Join(dir, "sites-available", "ornek.com.conf")
	if err := os.MkdirAll(filepath.Dir(vhost), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeManagedSiteFile(vhost, []byte(SealManagedText("server {}\n")), siteFileInspection{state: transport.SiteFileAbsent}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(vhost)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o007 != 0 {
		t.Fatalf("a new vhost is %04o: other accounts read it", info.Mode().Perm())
	}
}
