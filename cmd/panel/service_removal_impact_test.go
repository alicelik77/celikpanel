package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
)

// The removal dialog's facts (alpha.100). Each class of component is checked
// against the uninstall path's own conditions, against a real SQLite with the
// deletion guards' fixture, and through the handler with the Panel's real
// schema. Nothing here may change the server or the database.

func removalInstalled(ids ...string) map[string]bool {
	set := map[string]bool{}
	for _, id := range ids {
		set[id] = true
	}
	return set
}

func removalImpactFor(t *testing.T, id, family string, installed map[string]bool) serviceRemovalImpactResponse {
	t.Helper()
	return removalImpactWith(t, id, family, installed, removalHostFacts{})
}

func removalImpactWith(t *testing.T, id, family string, installed map[string]bool, facts removalHostFacts) serviceRemovalImpactResponse {
	t.Helper()
	db := newDependentsTestDB(t)
	// The certificates certbot renews (cert_renewal.go: Let's Encrypt, active,
	// auto-renew): two of them, beside a custom one, an expired one and one
	// whose automatic renewal is off. Two VPN peers (one revoked) and an FTP
	// account.
	if _, err := db.Exec(`
		CREATE TABLE ssl_certificates (id INTEGER PRIMARY KEY, domain_id INTEGER, type TEXT, status TEXT, auto_renew INTEGER);
		INSERT INTO ssl_certificates VALUES (1, 100, 'letsencrypt', 'active', 1), (2, 101, 'letsencrypt', 'active', 1),
			(3, 102, 'custom', 'active', 1), (4, 103, 'letsencrypt', 'expired', 1), (5, 104, 'letsencrypt', 'active', 0);
		CREATE TABLE vpn_peers (id INTEGER PRIMARY KEY, name TEXT, desired_state TEXT);
		INSERT INTO vpn_peers VALUES (1, 'laptop', 'active'), (2, 'phone', 'active'), (3, 'old', 'revoked');
		CREATE TABLE ftp_accounts (id INTEGER PRIMARY KEY, username TEXT, access_type TEXT NOT NULL DEFAULT 'sftp');
		INSERT INTO ftp_accounts VALUES (1, 'ali_ftp', 'ftp');
		INSERT INTO ftp_accounts VALUES (2, 'ali_sftp', 'sftp');`); err != nil {
		t.Fatal(err)
	}
	svc := core.GetManagedServiceByID(id)
	if svc == nil {
		t.Fatalf("unknown component %q", id)
	}
	impact, err := serviceRemovalImpact(context.Background(), db, svc, family, installed, facts)
	if err != nil {
		t.Fatalf("%s: %v", id, err)
	}
	return impact
}

func bundleIDs(bundles []serviceRemovalBundle) []string {
	out := []string{}
	for _, bundle := range bundles {
		out = append(out, bundle.ID)
	}
	return out
}

var removalFullServer = removalInstalled("nginx", "php-fpm", "phpmyadmin", "mariadb", "postgresql", "phppgadmin",
	"postfix", "dovecot", "rspamd", "roundcube", "certbot", "node")

func TestRemovalImpactWebServerNamesSitesRequirersAndBundles(t *testing.T) {
	impact := removalImpactFor(t, "nginx", "apt", removalFullServer)
	if impact.Removal != "packages" || !slices.Equal(impact.Packages, []string{"nginx"}) || impact.AllVersions {
		t.Fatalf("nginx removal = %+v", impact)
	}
	if !slices.Equal(impact.FirewallPorts, []string{"80/tcp", "443/tcp"}) {
		t.Fatalf("ports = %v", impact.FirewallPorts)
	}
	for _, id := range []string{"phpmyadmin", "phppgadmin", "roundcube", "node"} {
		if !slices.Contains(impact.RequiredBy, id) {
			t.Fatalf("required_by %v lacks %s", impact.RequiredBy, id)
		}
	}
	if slices.Contains(impact.RequiredBy, "php-fpm") || slices.Contains(impact.RequiredBy, "nginx") {
		t.Fatalf("required_by names a component that does not need a web server: %v", impact.RequiredBy)
	}
	bundles := bundleIDs(impact.Bundles)
	for _, id := range []string{"web.nginx_php", "db.mariadb.admin", "db.postgresql.admin", "mail.webmail"} {
		if !slices.Contains(bundles, id) {
			t.Fatalf("bundles %v lacks %s", bundles, id)
		}
	}
	for _, id := range []string{"db.mariadb", "db.postgresql", "mail.core", "mail.protected"} {
		if slices.Contains(bundles, id) {
			t.Fatalf("bundles %v names %s, which keeps every component", bundles, id)
		}
	}
	// Six sites need a web server; the DNS-only one does not.
	if impact.Hosted == nil || impact.Hosted.Kind != "sites" || impact.Hosted.Count != 6 || !impact.Hosted.BlocksRemoval {
		t.Fatalf("hosted = %+v", impact.Hosted)
	}
	if !slices.Contains(impact.Hosted.Names, "php83.example (ali)") || slices.Contains(impact.Hosted.Names, "dnsonly.example (ali)") {
		t.Fatalf("names = %v", impact.Hosted.Names)
	}
}

func TestRemovalImpactRequiredByIgnoresARequirementAnotherMemberMeets(t *testing.T) {
	// Apache keeps the web-server seat filled: nothing needs Nginx itself.
	// (Two members of a seat do not coexist on a real host; the rule is
	// RequirementsMissing's, asked with and without the component.)
	impact := removalImpactFor(t, "nginx", "apt", removalInstalled("nginx", "apache", "php-fpm", "node"))
	if len(impact.RequiredBy) != 0 {
		t.Fatalf("required_by = %v", impact.RequiredBy)
	}
}

func TestRemovalImpactRuntimesCountTheirSitesAndPurgeEveryVersionOnApt(t *testing.T) {
	php := removalImpactFor(t, "php-fpm", "apt", removalFullServer)
	if !php.AllVersions || php.Hosted == nil || php.Hosted.Kind != "php_sites" || php.Hosted.Count != 3 || !php.Hosted.BlocksRemoval {
		t.Fatalf("php-fpm = %+v / %+v", php, php.Hosted)
	}
	if pacman := removalImpactFor(t, "php-fpm", "pacman", removalFullServer); pacman.AllVersions {
		t.Fatal("pacman removes only the catalogue packages; no version discovery runs there")
	}
	node := removalImpactFor(t, "node", "apt", removalFullServer)
	if node.Hosted == nil || node.Hosted.Kind != "node_sites" || node.Hosted.Count != 2 {
		t.Fatalf("node hosted = %+v", node.Hosted)
	}
}

func TestRemovalImpactDatabaseEnginesCountTheirDatabases(t *testing.T) {
	maria := removalImpactFor(t, "mariadb", "apt", removalFullServer)
	if maria.Hosted == nil || maria.Hosted.Kind != "databases" || maria.Hosted.Count != 2 || !maria.Hosted.BlocksRemoval {
		t.Fatalf("mariadb hosted = %+v", maria.Hosted)
	}
	if !slices.Contains(maria.RequiredBy, "phpmyadmin") || !slices.Contains(bundleIDs(maria.Bundles), "db.mariadb") {
		t.Fatalf("mariadb = %+v", maria)
	}
	pg := removalImpactFor(t, "postgresql", "apt", removalFullServer)
	if pg.Hosted == nil || pg.Hosted.Kind != "databases" || pg.Hosted.Count != 0 || pg.Hosted.BlocksRemoval || len(pg.Hosted.Names) != 0 {
		t.Fatalf("postgresql hosted = %+v", pg.Hosted)
	}
	if !pg.AllVersions || !slices.Contains(pg.RequiredBy, "phppgadmin") || !slices.Contains(bundleIDs(pg.Bundles), "db.postgresql") {
		t.Fatalf("postgresql = %+v", pg)
	}
}

func TestRemovalImpactMailComponents(t *testing.T) {
	postfix := removalImpactFor(t, "postfix", "apt", removalFullServer)
	if postfix.Hosted == nil || postfix.Hosted.Kind != "mail_addresses" || postfix.Hosted.Count != 1 || !postfix.Hosted.BlocksRemoval {
		t.Fatalf("postfix hosted = %+v", postfix.Hosted)
	}
	if !slices.Contains(postfix.RequiredBy, "rspamd") || !slices.Contains(postfix.RequiredBy, "roundcube") || postfix.MailFilterRewired {
		t.Fatalf("postfix = %+v", postfix)
	}
	for _, id := range []string{"mail.core", "mail.webmail", "mail.protected"} {
		if !slices.Contains(bundleIDs(postfix.Bundles), id) {
			t.Fatalf("postfix bundles %v lack %s", bundleIDs(postfix.Bundles), id)
		}
	}
	rspamd := removalImpactFor(t, "rspamd", "apt", removalFullServer)
	if !rspamd.MailFilterRewired || rspamd.Hosted == nil || rspamd.Hosted.Kind != "mail_domains" || rspamd.Hosted.Count != 1 || rspamd.Hosted.BlocksRemoval {
		t.Fatalf("rspamd = %+v / %+v", rspamd, rspamd.Hosted)
	}
	if !slices.Equal(rspamd.Hosted.Names, []string{"php83.example"}) || !slices.Equal(bundleIDs(rspamd.Bundles), []string{"mail.protected"}) {
		t.Fatalf("rspamd = %+v", rspamd)
	}
	roundcube := removalImpactFor(t, "roundcube", "apt", removalFullServer)
	if roundcube.Removal != "webmail_tree" || len(roundcube.Packages) != 0 {
		t.Fatalf("roundcube removal = %+v", roundcube)
	}
	if roundcube.Hosted == nil || roundcube.Hosted.Kind != "mailboxes" || roundcube.Hosted.Count != 1 || roundcube.Hosted.BlocksRemoval {
		t.Fatalf("roundcube hosted = %+v", roundcube.Hosted)
	}
}

func TestRemovalImpactToolsAndTheEmptyCase(t *testing.T) {
	pma := removalImpactFor(t, "phpmyadmin", "apt", removalFullServer)
	if !pma.DatabaseToolsReconfigured || pma.Hosted != nil || len(pma.RequiredBy) != 0 {
		t.Fatalf("phpmyadmin = %+v", pma)
	}
	if pma.HasUnits || pma.SetupRequired {
		t.Fatalf("phpmyadmin has no units and is not required by setup: %+v", pma)
	}
	// A component nothing the Panel records depends on: Fail2ban.
	f2b := removalImpactFor(t, "fail2ban", "pacman", removalInstalled("fail2ban", "nginx"))
	if f2b.Hosted != nil || len(f2b.RequiredBy) != 0 || len(f2b.Bundles) != 0 || f2b.SetupRequired || !f2b.HasUnits ||
		f2b.MailFilterRewired || f2b.DatabaseToolsReconfigured || f2b.AllVersions || f2b.Removal != "packages" ||
		len(f2b.FirewallPorts) != 0 || !slices.Equal(f2b.Packages, []string{"fail2ban"}) {
		t.Fatalf("fail2ban = %+v", f2b)
	}
	// A component nothing else is installed beside: every list is empty, and
	// the wire carries empty lists, not null.
	alone := removalImpactFor(t, "nginx", "apt", removalInstalled("nginx"))
	raw, err := json.Marshal(alone)
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range []string{`"required_by":[]`, `"bundles":[]`} {
		if !strings.Contains(string(raw), part) {
			t.Fatalf("%s lacks %s", raw, part)
		}
	}
}

// certbot is never "nothing depends on it": the Panel issues and renews the
// panel's, the mail host's and the sites' certificates through it, and server
// setup requires it. Only certbot's own (Let's Encrypt, active) certificates
// count.
func TestRemovalImpactCertbotNamesTheCertificatesItRenews(t *testing.T) {
	certbot := removalImpactWith(t, "certbot", "apt", removalFullServer, removalHostFacts{PanelCertificate: "panel.example.com", MailHosts: []string{"mail.example.com"}})
	if !certbot.SetupRequired || certbot.HasUnits || certbot.Removal != "packages" {
		t.Fatalf("certbot = %+v", certbot)
	}
	if certbot.Hosted == nil || certbot.Hosted.Kind != "certificates" || certbot.Hosted.Count != 4 || certbot.Hosted.BlocksRemoval {
		t.Fatalf("certbot hosted = %+v", certbot.Hosted)
	}
	if !slices.Equal(certbot.Hosted.Names, []string{"panel.example.com", "mail.example.com", "php83.example", "php84.example"}) {
		t.Fatalf("certificates = %v", certbot.Hosted.Names)
	}
	none := removalImpactWith(t, "certbot", "apt", removalInstalled("certbot"), removalHostFacts{})
	if none.Hosted == nil || none.Hosted.Count != 2 || !none.SetupRequired {
		t.Fatalf("certbot without panel or mail certificates = %+v / %+v", none, none.Hosted)
	}
}

// nftables enforces the panel firewall; its state is said when it was read and
// "not known" otherwise. Server setup requires it.
func TestRemovalImpactFirewallEngineNamesThePanelFirewall(t *testing.T) {
	read := removalImpactWith(t, "nftables", "apt", removalInstalled("nftables"),
		removalHostFacts{Firewall: &serviceRemovalFirewall{Known: true, Enabled: true, OpenPorts: 4}})
	if !read.SetupRequired || read.HasUnits || read.Hosted == nil || read.Hosted.Kind != "firewall" || read.Hosted.BlocksRemoval ||
		read.Hosted.Firewall == nil || !read.Hosted.Firewall.Known || !read.Hosted.Firewall.Enabled || read.Hosted.Firewall.OpenPorts != 4 {
		t.Fatalf("nftables = %+v / %+v", read, read.Hosted)
	}
	unread := removalImpactFor(t, "nftables", "apt", removalInstalled("nftables"))
	if unread.Hosted == nil || unread.Hosted.Kind != "firewall" || unread.Hosted.Firewall == nil || unread.Hosted.Firewall.Known {
		t.Fatalf("an unread firewall = %+v", unread.Hosted)
	}
}

// The caches: the Panel does not record which sites use them, so the answer
// says so and never "nothing depends on it".
func TestRemovalImpactCachesAreUnrecordedNotUnused(t *testing.T) {
	for _, id := range []string{"redis", "valkey", "memcached"} {
		impact := removalImpactFor(t, id, "apt", removalInstalled(id))
		if impact.Hosted == nil || impact.Hosted.Kind != "unrecorded" || impact.Hosted.BlocksRemoval || impact.SetupRequired {
			t.Fatalf("%s = %+v", id, impact.Hosted)
		}
	}
}

// A package family without catalogue packages for the component: the Agent
// refuses ("cannot be removed automatically on this system yet"), so the
// answer says so instead of a purge.
func TestRemovalImpactWithoutPackagesForTheFamilyIsUnsupported(t *testing.T) {
	for _, id := range []string{"redis", "spamassassin", "phppgadmin"} {
		impact := removalImpactFor(t, id, "pacman", removalInstalled(id))
		if impact.Removal != "unsupported" || len(impact.Packages) != 0 {
			t.Fatalf("%s on pacman = %+v", id, impact)
		}
		if apt := removalImpactFor(t, id, "apt", removalInstalled(id)); apt.Removal != "packages" {
			t.Fatalf("%s on apt = %+v", id, apt)
		}
	}
	if unknown := removalImpactFor(t, "redis", "", removalInstalled("redis")); unknown.Removal != "packages" {
		t.Fatalf("an unknown family claims nothing: %+v", unknown)
	}
	source := readPanelSource(t, "../agent/install_rpc.go")
	if !strings.Contains(source, `cannot be removed automatically on this system yet`) {
		t.Fatal("the Agent's refusal for a family without packages changed")
	}
}

// The facts are the uninstall path's own conditions: the same helpers decide
// the mail filter rewiring and the database tools' reconfiguration in
// handleServiceUninstall, and the Agent purges every version with the same
// core.UninstallPurgesAllVersions.
func TestRemovalImpactFactsAreTheUninstallPathsConditions(t *testing.T) {
	for i := range core.ManagedServices {
		svc := &core.ManagedServices[i]
		if got, want := uninstallRewiresMailFilters(svc), svc.ConflictGroup == "spam-filter"; got != want {
			t.Fatalf("%s: mail filter rewiring = %v", svc.ID, got)
		}
		for _, family := range []string{"apt", "pacman", "dnf"} {
			want := family == "apt" && svc.Repo != nil && svc.Repo.PackagePattern != ""
			if got := core.UninstallPurgesAllVersions(svc, family); got != want {
				t.Fatalf("%s/%s: all versions = %v", svc.ID, family, got)
			}
		}
	}
	source := readPanelSource(t, "service_install_handler.go")
	for _, call := range []string{"uninstallRewiresMailFilters(svc)", "uninstallReconfiguresDatabaseTools(req.ServiceID)"} {
		if !strings.Contains(source, call) {
			t.Fatalf("handleServiceUninstall no longer asks %s", call)
		}
	}
	agent := readPanelSource(t, "../agent/install_rpc.go")
	if !strings.Contains(agent, "core.UninstallPurgesAllVersions(svc, family)") {
		t.Fatal("Agent.UninstallService no longer asks core.UninstallPurgesAllVersions")
	}
}

func seedRemovalScanCache(t *testing.T, panel *Panel, installed ...string) {
	t.Helper()
	observations := []serviceObservation{}
	for _, id := range installed {
		observations = append(observations, serviceObservation{ID: id, IsInstalled: true, Status: "active (running)"})
	}
	raw, err := json.Marshal(scanCacheDoc{Observations: observations})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := panel.db.GetDB().Exec(`INSERT INTO service_scan_cache (id, data, scanned_at) VALUES (1, ?, ?)`,
		string(raw), time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
}

func removalImpactRequest(panel *Panel, method, query string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	panel.handleServiceRemovalImpact(recorder, httptest.NewRequest(method, serviceRemovalImpactPath+query, nil))
	return recorder
}

func TestRemovalImpactHandlerReadsTheStoredRecordAndChangesNothing(t *testing.T) {
	panel := newManagedServiceCachePanel(t)
	seedRemovalScanCache(t, panel, "nginx", "php-fpm", "phpmyadmin", "mariadb")
	var before string
	if err := panel.db.GetDB().QueryRow(`SELECT data || scanned_at FROM service_scan_cache WHERE id = 1`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	countRows := func() int {
		var n int
		if err := panel.db.GetDB().QueryRow(`SELECT (SELECT COUNT(*) FROM audit_logs) + (SELECT COUNT(*) FROM service_scan_cache)`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	rowsBefore := countRows()

	recorder := removalImpactRequest(panel, http.MethodGet, "?service_id=nginx")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("the answer may be cached")
	}
	var impact serviceRemovalImpactResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &impact); err != nil {
		t.Fatal(err)
	}
	if impact.ServiceID != "nginx" || impact.PackageFamily != "apt" || !slices.Equal(impact.RequiredBy, []string{"phpmyadmin"}) {
		t.Fatalf("impact = %+v", impact)
	}
	if !slices.Equal(bundleIDs(impact.Bundles), []string{"db.mariadb.admin", "web.nginx_php"}) {
		t.Fatalf("bundles = %v", bundleIDs(impact.Bundles))
	}
	// The Panel's real schema: no sites yet, so nothing hosted blocks.
	if impact.Hosted == nil || impact.Hosted.Kind != "sites" || impact.Hosted.Count != 0 || impact.Hosted.BlocksRemoval {
		t.Fatalf("hosted = %+v", impact.Hosted)
	}
	// Every hosted query runs against the real schema too.
	restoreCert, restoreFirewall := removalPanelCertificate, removalFirewallStatus
	t.Cleanup(func() { removalPanelCertificate, removalFirewallStatus = restoreCert, restoreFirewall })
	removalPanelCertificate = func() string { return "panel.example.com" }
	removalFirewallStatus = func(*Panel) (FirewallStatusResp, error) {
		return FirewallStatusResp{Enabled: true, EngineAvailable: true, TCPPorts: []int{22, 80, 443}, UDPPorts: []int{53}}, nil
	}
	for _, id := range []string{"php-fpm", "node", "mariadb", "postgresql", "postfix", "dovecot", "rspamd", "spamassassin", "roundcube", "certbot", "nftables", "redis", "wireguard", "vsftpd"} {
		if response := removalImpactRequest(panel, http.MethodGet, "?service_id="+id); response.Code != http.StatusOK {
			t.Fatalf("%s: status = %d: %s", id, response.Code, response.Body.String())
		}
	}

	var after string
	if err := panel.db.GetDB().QueryRow(`SELECT data || scanned_at FROM service_scan_cache WHERE id = 1`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if before != after || countRows() != rowsBefore {
		t.Fatal("reading the removal impact changed the database")
	}
}

func TestRemovalImpactHandlerRefusals(t *testing.T) {
	panel := newManagedServiceCachePanel(t)
	// No stored record yet: the dialog says it could not read; removal stays allowed.
	if recorder := removalImpactRequest(panel, http.MethodGet, "?service_id=nginx"); recorder.Code != http.StatusServiceUnavailable ||
		!strings.Contains(recorder.Body.String(), errCodeServiceStateUnverified) {
		t.Fatalf("no record: %d %s", recorder.Code, recorder.Body.String())
	}
	seedRemovalScanCache(t, panel, "nginx")
	for _, tc := range []struct {
		method, query string
		status        int
	}{
		{http.MethodPost, "?service_id=nginx", http.StatusMethodNotAllowed},
		{http.MethodGet, "", http.StatusBadRequest},
		{http.MethodGet, "?service_id=nope", http.StatusBadRequest},
		{http.MethodGet, "?service_id=bind", http.StatusConflict},
		{http.MethodGet, "?service_id=pdns", http.StatusConflict},
	} {
		if recorder := removalImpactRequest(panel, tc.method, tc.query); recorder.Code != tc.status {
			t.Fatalf("%s %s: status = %d, want %d: %s", tc.method, tc.query, recorder.Code, tc.status, recorder.Body.String())
		}
	}
}

// Through the handler, certbot names the panel certificate and nftables the
// firewall's state as read; a firewall that cannot be read is "not known".
func TestRemovalImpactHandlerReadsCertificatesAndFirewall(t *testing.T) {
	panel := newManagedServiceCachePanel(t)
	seedRemovalScanCache(t, panel, "certbot", "nftables")
	restoreCert, restoreFirewall, restoreMail := removalPanelCertificate, removalFirewallStatus, removalMailHostCertificates
	t.Cleanup(func() {
		removalPanelCertificate, removalFirewallStatus, removalMailHostCertificates = restoreCert, restoreFirewall, restoreMail
	})
	removalPanelCertificate = func() string { return "panel.example.com" }
	removalMailHostCertificates = func(*Panel, context.Context) []string { return []string{"mail.example.com"} }
	removalFirewallStatus = func(*Panel) (FirewallStatusResp, error) {
		return FirewallStatusResp{Enabled: true, EngineAvailable: true, TCPPorts: []int{22, 80, 443}, UDPPorts: []int{53}}, nil
	}
	var certbot serviceRemovalImpactResponse
	if err := json.Unmarshal(removalImpactRequest(panel, http.MethodGet, "?service_id=certbot").Body.Bytes(), &certbot); err != nil {
		t.Fatal(err)
	}
	if certbot.Hosted == nil || certbot.Hosted.Kind != "certificates" || !slices.Equal(certbot.Hosted.Names, []string{"panel.example.com", "mail.example.com"}) || !certbot.SetupRequired {
		t.Fatalf("certbot = %+v / %+v", certbot, certbot.Hosted)
	}
	var nft serviceRemovalImpactResponse
	if err := json.Unmarshal(removalImpactRequest(panel, http.MethodGet, "?service_id=nftables").Body.Bytes(), &nft); err != nil {
		t.Fatal(err)
	}
	if nft.Hosted == nil || nft.Hosted.Firewall == nil || !nft.Hosted.Firewall.Known || !nft.Hosted.Firewall.Enabled || nft.Hosted.Firewall.OpenPorts != 4 {
		t.Fatalf("nftables = %+v", nft.Hosted)
	}
	// Not a known state: a failed call, an answer carrying its own error or
	// error code, an engine that cannot be read.
	for name, reader := range map[string]func(*Panel) (FirewallStatusResp, error){
		"call failed": func(*Panel) (FirewallStatusResp, error) { return FirewallStatusResp{}, errors.New("agent unavailable") },
		"answer error": func(*Panel) (FirewallStatusResp, error) {
			return FirewallStatusResp{Enabled: true, EngineAvailable: true, Error: "nft list failed"}, nil
		},
		"answer code": func(*Panel) (FirewallStatusResp, error) {
			return FirewallStatusResp{EngineAvailable: true, ErrorCode: "host_restart_required"}, nil
		},
		"engine not there": func(*Panel) (FirewallStatusResp, error) {
			return FirewallStatusResp{Enabled: false, EngineAvailable: false}, nil
		},
	} {
		removalFirewallStatus = reader
		nft = serviceRemovalImpactResponse{}
		if err := json.Unmarshal(removalImpactRequest(panel, http.MethodGet, "?service_id=nftables").Body.Bytes(), &nft); err != nil {
			t.Fatal(err)
		}
		if nft.Hosted == nil || nft.Hosted.Firewall == nil || nft.Hosted.Firewall.Known {
			t.Fatalf("%s: firewall = %+v", name, nft.Hosted)
		}
	}
}

// The firewall read for the dialog never waits for a firewall change: with
// panelFirewallMu held it answers at once, and the dialog says "not known".
func TestRemovalFirewallReadDoesNotWaitForAFirewallChange(t *testing.T) {
	panelFirewallMu.Lock()
	defer panelFirewallMu.Unlock()
	started := time.Now()
	if _, err := removalFirewallStatus(&Panel{}); !errors.Is(err, errRemovalFirewallBusy) {
		t.Fatalf("err = %v", err)
	}
	if time.Since(started) > time.Second {
		t.Fatal("the read waited for the firewall lock")
	}
}

// An owner certificate (CELIKPANEL_TLS_CERT/KEY) is not certbot's to renew.
func TestRemovalPanelCertificateLeavesAnOwnerCertificateOut(t *testing.T) {
	t.Setenv("CELIKPANEL_TLS_CERT", "/etc/owner/panel.crt")
	t.Setenv("CELIKPANEL_TLS_KEY", "/etc/owner/panel.key")
	if got := removalPanelCertificate(); got != "" {
		t.Fatalf("an owner certificate was named: %q", got)
	}
}

// The mail host certificates are the ones server setup recorded as issued;
// a failed step, another kind of step or no record names nothing.
func TestRemovalMailHostCertificatesAreTheSetupRecordsOwn(t *testing.T) {
	f, state := setupOperationFixture(t)
	if hosts := f.panel.setupMailCertificateHosts(context.Background()); len(hosts) != 0 {
		t.Fatalf("no record named %v", hosts)
	}
	plan := serverSetupPlan{Version: serverSetupPlanVersion, Revision: state.Revision, Draft: state.Draft, Purpose: "web_mail", Actor: serviceOperationActor{UserID: f.userID}, Steps: []serverSetupPlanStep{}}
	plan.ID = serverSetupPlanIdentity(plan)
	raw, _ := json.Marshal(plan)
	if _, err := f.database.GetDB().Exec(`INSERT INTO server_setup_plans(id,revision,plan_json,requested_by,created_at) VALUES(?,?,?,?,?)`, plan.ID, plan.Revision, string(raw), f.userID, "now"); err != nil {
		t.Fatal(err)
	}
	step := func(kind, target, status string) serverSetupExecutionStep {
		return serverSetupExecutionStep{serverSetupPlanStep: serverSetupPlanStep{ID: kind + target, Kind: kind, Target: target}, Status: status}
	}
	for i, execution := range []serverSetupExecution{
		{ID: strings.Repeat("7", 32), RequestID: strings.Repeat("7", 32), PlanID: plan.ID, Status: "succeeded", Steps: []serverSetupExecutionStep{
			step("panel_certificate", "panel.example.com", "succeeded"), step("mail_certificate", "mail.example.com", "succeeded")}},
		{ID: strings.Repeat("8", 32), RequestID: strings.Repeat("8", 32), PlanID: plan.ID, Status: "failed", Steps: []serverSetupExecutionStep{
			step("mail_certificate", "mail2.example.com", "failed")}},
	} {
		raw, _ := json.Marshal(execution)
		if _, err := f.database.GetDB().Exec(`INSERT INTO server_setup_executions(id,request_id,plan_id,status,execution_json,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`,
			execution.ID, execution.RequestID, plan.ID, execution.Status, string(raw), "now", fmt.Sprintf("2026-10-22T0%d:00:00Z", i)); err != nil {
			t.Fatal(err)
		}
	}
	if hosts := f.panel.setupMailCertificateHosts(context.Background()); !slices.Equal(hosts, []string{"mail.example.com"}) {
		t.Fatalf("hosts = %v", hosts)
	}
}

// WireGuard: the VPN peers the Panel keeps active. vsftpd: the FTP accounts
// it records. Neither blocks the removal.
func TestRemovalImpactVPNPeersAndFTPAccounts(t *testing.T) {
	wg := removalImpactFor(t, "wireguard", "apt", removalInstalled("wireguard"))
	if wg.Hosted == nil || wg.Hosted.Kind != "vpn_peers" || wg.Hosted.Count != 2 || wg.Hosted.BlocksRemoval || !slices.Equal(wg.Hosted.Names, []string{"laptop", "phone"}) {
		t.Fatalf("wireguard = %+v", wg.Hosted)
	}
	ftp := removalImpactFor(t, "vsftpd", "apt", removalInstalled("vsftpd"))
	if ftp.Hosted == nil || ftp.Hosted.Kind != "ftp_accounts" || ftp.Hosted.Count != 1 || ftp.Hosted.BlocksRemoval {
		t.Fatalf("vsftpd = %+v", ftp.Hosted)
	}
}
