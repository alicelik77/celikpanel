package main

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
)

func bundleViewByID(t *testing.T, bundles []BundleResponse, id string) BundleResponse {
	t.Helper()
	for _, bundle := range bundles {
		if bundle.ID == id {
			return bundle
		}
	}
	t.Fatalf("bundle %s is missing", id)
	return BundleResponse{}
}

func bundlesFor(observations []serviceObservation, family string, attempts map[string]mailProfileAttemptProof, webmailReady, webmailProven bool) []BundleResponse {
	services := catalogView(observations, family)
	profiles := mailProfilesView(services, true, family, "", webmailReady, webmailProven, attempts)
	return bundlesView(services, true, family, profiles, profiles)
}

func bundleRunning(id string) serviceObservation {
	return serviceObservation{ID: id, IsInstalled: true, Status: "active (running)"}
}

// The registry is the owner's list (D-034; amendment 2026-10-22), closed and
// ordered: mail through the three mail profiles, two databases each with its
// browser tool, and one web bundle. Every component is a catalogue component.
func TestBundleRegistryIsTheOwnersList(t *testing.T) {
	want := []struct {
		id, category, kind, profile string
		components, prereqs, addons []string
	}{
		{"mail.core", "email", bundleKindMailProfile, "core-mail", []string{"postfix", "dovecot"}, nil, []string{"mail.protected"}},
		{"mail.webmail", "email", bundleKindMailProfile, "webmail", []string{"postfix", "dovecot", "nginx", "php-fpm", "roundcube"}, nil, []string{"mail.protected"}},
		{"mail.protected", "email", bundleKindMailProfile, "protected-mail", []string{"postfix", "dovecot", "rspamd"}, nil, nil},
		{"db.postgresql", "database", bundleKindComponents, "", []string{"postgresql", "nginx", "php-fpm", "phppgadmin"}, []string{"nginx", "php-fpm"}, nil},
		{"db.mariadb", "database", bundleKindComponents, "", []string{"mariadb", "nginx", "php-fpm", "phpmyadmin"}, []string{"nginx", "php-fpm"}, nil},
		{"web.nginx_php", "web", bundleKindComponents, "", []string{"nginx", "php-fpm"}, nil, nil},
	}
	if len(bundleDefinitions) != len(want) {
		t.Fatalf("bundle count = %d, want %d", len(bundleDefinitions), len(want))
	}
	for index, expected := range want {
		id := expected.id
		definition, ok := bundleByID(id)
		if !ok {
			t.Fatalf("bundle %s is missing", id)
		}
		if bundleDefinitions[index].ID != id {
			t.Fatalf("bundle %d is %s, want %s (registry order)", index, bundleDefinitions[index].ID, id)
		}
		if definition.Category != expected.category || definition.Kind != expected.kind ||
			definition.ProfileID != expected.profile || definition.Parent != "" ||
			!reflect.DeepEqual(definition.Components, expected.components) ||
			len(definition.Prerequisites) != len(expected.prereqs) ||
			(len(expected.prereqs) > 0 && !reflect.DeepEqual(definition.Prerequisites, expected.prereqs)) ||
			len(definition.Addons) != len(expected.addons) ||
			(len(expected.addons) > 0 && !reflect.DeepEqual(definition.Addons, expected.addons)) {
			t.Fatalf("bundle %s = %+v", id, definition)
		}
		for _, addon := range definition.Addons {
			if _, ok := bundleByID(addon); !ok {
				t.Fatalf("bundle %s offers unknown add-on %s", id, addon)
			}
		}
		if definition.Parent != "" {
			if _, ok := bundleByID(definition.Parent); !ok {
				t.Fatalf("bundle %s has unknown parent %s", id, definition.Parent)
			}
		}
	}
}

// The owner's rule of 2026-10-22: a bundle is a service with its companion
// tools, never one component. Every components bundle has at least two
// components and at least one that is there for itself, not only for the role
// it fills; its prerequisites are among its components; a mail bundle is
// exactly its mail profile. No retired one-component or add-on bundle comes
// back.
// Sahibin kuralı: paket, bir hizmet ile ona eşlik eden araçlardır.
func TestBundleRegistryKeepsTheOwnersRule(t *testing.T) {
	for _, definition := range bundleDefinitions {
		for _, id := range definition.Components {
			if core.GetManagedServiceByID(id) == nil {
				t.Fatalf("bundle %s names %s, which is not a catalogue component", definition.ID, id)
			}
		}
		for _, id := range definition.Prerequisites {
			if !slices.Contains(definition.Components, id) {
				t.Fatalf("bundle %s has prerequisite %s outside its components", definition.ID, id)
			}
		}
		switch definition.Kind {
		case bundleKindComponents:
			own := 0
			for _, id := range definition.Components {
				if !slices.Contains(definition.Prerequisites, id) {
					own++
				}
			}
			if len(definition.Components) < 2 || own < 1 {
				t.Fatalf("bundle %s = %v (prerequisites %v): a bundle is a service with its companion tools, never one component",
					definition.ID, definition.Components, definition.Prerequisites)
			}
			if definition.ProfileID != "" {
				t.Fatalf("components bundle %s names a mail profile", definition.ID)
			}
		case bundleKindMailProfile:
			services, ok := core.MailProfileServiceIDs(definition.ProfileID)
			if !ok || !reflect.DeepEqual(definition.Components, services) || len(definition.Prerequisites) != 0 {
				t.Fatalf("mail bundle %s = %v, profile %s = %v", definition.ID, definition.Components, definition.ProfileID, services)
			}
		default:
			t.Fatalf("bundle %s has kind %q", definition.ID, definition.Kind)
		}
	}
	for _, retired := range []string{"db.postgresql.admin", "db.mariadb.admin"} {
		if _, ok := bundleByID(retired); ok {
			t.Fatalf("retired bundle %s is back; its tool is inside the database bundle", retired)
		}
	}
	for _, code := range bundleTextCodes {
		if code == "db_postgresql" || code == "db_mariadb" || strings.HasSuffix(string(code), "_admin") {
			t.Fatalf("retired name code %s is back: a page would name a different bundle by it", code)
		}
	}
}

// Every bundleTextCode constant is in bundleTextCodes, which the web test
// reads to require English and Turkish words for each.
func TestBundleTextCodesAreListed(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	declared := map[string]string{}
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		source, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		file, err := parser.ParseFile(fset, name, source, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.CONST {
				continue
			}
			for _, spec := range gen.Specs {
				value := spec.(*ast.ValueSpec)
				typed, ok := value.Type.(*ast.Ident)
				if !ok || typed.Name != "bundleTextCode" {
					continue
				}
				for i, ident := range value.Names {
					declared[ident.Name] = strings.Trim(value.Values[i].(*ast.BasicLit).Value, `"`)
				}
			}
		}
	}
	listed := map[string]bool{}
	for _, code := range bundleTextCodes {
		if code == "" || listed[string(code)] {
			t.Fatalf("code %q is empty or listed twice", code)
		}
		listed[string(code)] = true
	}
	if len(declared) != len(listed) {
		t.Fatalf("%d codes declared, %d listed", len(declared), len(listed))
	}
	for name, code := range declared {
		if !listed[code] {
			t.Fatalf("%s (%q) is not in bundleTextCodes", name, code)
		}
	}
	used := map[bundleTextCode]bool{}
	for _, definition := range bundleDefinitions {
		if used[definition.NameCode] {
			t.Fatalf("name code %s is used twice", definition.NameCode)
		}
		used[definition.NameCode] = true
	}
	if len(used) != len(bundleTextCodes) {
		t.Fatalf("%d codes used by %d listed", len(used), len(bundleTextCodes))
	}
}

func TestBundlesAreUnknownUntilAFreshScan(t *testing.T) {
	services := catalogView(nil, "apt")
	profiles := mailProfilesView(services, false, "apt", "", false, false, nil)
	for _, bundle := range bundlesView(services, false, "apt", profiles, profiles) {
		if bundle.Status != bundleStatusUnknown || bundle.Action != bundleActionNone {
			t.Fatalf("unscanned bundle %s = %+v", bundle.ID, bundle)
		}
	}
}

func TestBundlesOnAnEmptyServer(t *testing.T) {
	bundles := bundlesFor(nil, "apt", nil, false, true)
	for _, id := range []string{"mail.core", "mail.webmail", "mail.protected", "db.postgresql", "db.mariadb", "web.nginx_php"} {
		bundle := bundleViewByID(t, bundles, id)
		if bundle.Status != bundleStatusAvailable || bundle.Action != bundleActionInstall {
			t.Fatalf("%s on an empty server = %+v", id, bundle)
		}
		if !reflect.DeepEqual(bundle.MissingComponents, bundle.Components) {
			t.Fatalf("%s missing = %v, want every component", id, bundle.MissingComponents)
		}
	}
	web := bundleViewByID(t, bundles, "web.nginx_php")
	if web.ActionRoute != bundleKindComponents || web.ProfileID != "" {
		t.Fatalf("web bundle route = %+v", web)
	}
	mail := bundleViewByID(t, bundles, "mail.webmail")
	if mail.ActionRoute != bundleKindMailProfile || mail.ProfileID != "webmail" {
		t.Fatalf("mail bundle route = %+v", mail)
	}
	// A database bundle installs the database with its tool, in order; no
	// bundle is an add-on any more.
	for id, components := range map[string][]string{
		"db.postgresql": {"postgresql", "nginx", "php-fpm", "phppgadmin"},
		"db.mariadb":    {"mariadb", "nginx", "php-fpm", "phpmyadmin"},
	} {
		bundle := bundleViewByID(t, bundles, id)
		if !reflect.DeepEqual(bundle.MissingComponents, components) || bundle.Parent != "" || len(bundle.Addons) != 0 {
			t.Fatalf("%s on an empty server = %+v", id, bundle)
		}
	}
	if len(bundles) != len(bundleDefinitions) {
		t.Fatalf("bundles = %d", len(bundles))
	}
}

// The servers the owner looked at on alpha.100, as shapes.
// Sahibin alpha.100'de baktığı sunucular, biçim olarak.

// Boston: PostgreSQL and Nginx installed, no PHP-FPM, no phpPgAdmin.
// "PostgreSQL + phpPgAdmin" is offered (it is not installed and verified);
// Install installs only PHP-FPM and phpPgAdmin, in order; PostgreSQL and
// Nginx are kept.
func TestBundlesOnABostonShapedServer(t *testing.T) {
	bundles := bundlesFor([]serviceObservation{bundleRunning("postgresql"), bundleRunning("nginx")}, "apt", nil, false, true)
	pg := bundleViewByID(t, bundles, "db.postgresql")
	if pg.Status != bundleStatusAvailable || pg.Action != bundleActionInstall || pg.ActionRoute != bundleKindComponents ||
		!reflect.DeepEqual(pg.MissingComponents, []string{"php-fpm", "phppgadmin"}) {
		t.Fatalf("PostgreSQL + phpPgAdmin on Boston = %+v", pg)
	}
	maria := bundleViewByID(t, bundles, "db.mariadb")
	if maria.Status != bundleStatusAvailable || !reflect.DeepEqual(maria.MissingComponents, []string{"mariadb", "php-fpm", "phpmyadmin"}) {
		t.Fatalf("MariaDB + phpMyAdmin on Boston = %+v", maria)
	}
	web := bundleViewByID(t, bundles, "web.nginx_php")
	if web.Status != bundleStatusAvailable || !reflect.DeepEqual(web.MissingComponents, []string{"php-fpm"}) {
		t.Fatalf("Nginx + PHP-FPM on Boston = %+v", web)
	}
}

// Frankfurt: everything installed and running. Both database bundles are
// installed and verified, with nothing to do.
func TestBundlesOnAFrankfurtShapedServer(t *testing.T) {
	everything := []serviceObservation{
		bundleRunning("postgresql"), bundleRunning("mariadb"), bundleRunning("nginx"), bundleRunning("php-fpm"),
		{ID: "phppgadmin", IsInstalled: true, Status: "installed"}, {ID: "phpmyadmin", IsInstalled: true, Status: "installed"},
	}
	bundles := bundlesFor(everything, "apt", nil, false, true)
	for _, id := range []string{"db.postgresql", "db.mariadb", "web.nginx_php"} {
		if bundle := bundleViewByID(t, bundles, id); bundle.Status != bundleStatusInstalledVerified || bundle.Action != bundleActionNone || len(bundle.MissingComponents) != 0 {
			t.Fatalf("%s on Frankfurt = %+v", id, bundle)
		}
	}
	// A stopped part makes the bundle "installed, not verified" with no
	// action of its own: the dialogue shows which part, and the page's rescan
	// reads it again.
	stopped := append([]serviceObservation{}, everything...)
	stopped[1] = serviceObservation{ID: "mariadb", IsInstalled: true, Status: "inactive (dead)"}
	bundles = bundlesFor(stopped, "apt", nil, false, true)
	if bundle := bundleViewByID(t, bundles, "db.mariadb"); bundle.Status != bundleStatusInstalledUnverified || bundle.Action != bundleActionNone || len(bundle.MissingComponents) != 0 {
		t.Fatalf("MariaDB + phpMyAdmin with MariaDB stopped = %+v", bundle)
	}
	if bundle := bundleViewByID(t, bundles, "db.postgresql"); bundle.Status != bundleStatusInstalledVerified {
		t.Fatalf("PostgreSQL + phpPgAdmin beside a stopped MariaDB = %+v", bundle)
	}
	// The database alone is not the bundle: without its tool it is offered,
	// and Install adds only the tool.
	bundles = bundlesFor([]serviceObservation{bundleRunning("mariadb"), bundleRunning("nginx"), bundleRunning("php-fpm")}, "apt", nil, false, true)
	if bundle := bundleViewByID(t, bundles, "db.mariadb"); bundle.Status != bundleStatusAvailable || !reflect.DeepEqual(bundle.MissingComponents, []string{"phpmyadmin"}) {
		t.Fatalf("MariaDB without phpMyAdmin = %+v", bundle)
	}
}

// Mail bundles say what the mail profiles say: verified, installed but not
// verified, half-installed.
func TestMailBundlesFollowTheMailProfiles(t *testing.T) {
	coreMail := []serviceObservation{bundleRunning("postfix"), bundleRunning("dovecot")}
	bundles := bundlesFor(coreMail, "apt", nil, false, true)
	if bundle := bundleViewByID(t, bundles, "mail.core"); bundle.Status != bundleStatusInstalledUnverified || bundle.Action != bundleActionRepair || len(bundle.MissingComponents) != 0 {
		t.Fatalf("unproven core mail = %+v", bundle)
	}
	bundles = bundlesFor(coreMail, "apt", map[string]mailProfileAttemptProof{
		"core-mail": {Status: mailProfileAttemptSucceeded, Verified: true},
	}, false, true)
	if bundle := bundleViewByID(t, bundles, "mail.core"); bundle.Status != bundleStatusInstalledVerified || bundle.Action != bundleActionRepair {
		t.Fatalf("verified core mail = %+v", bundle)
	}
	if bundle := bundleViewByID(t, bundles, "mail.protected"); bundle.Status != bundleStatusPartial || bundle.Action != bundleActionContinue ||
		!reflect.DeepEqual(bundle.MissingComponents, []string{"rspamd"}) {
		t.Fatalf("spam protection after core mail = %+v", bundle)
	}

	// Every webmail component is running but the runtime proof is missing:
	// installed, not verified (not "partially installed").
	webmail := append(coreMail, bundleRunning("nginx"), bundleRunning("php-fpm"), serviceObservation{ID: "roundcube", IsInstalled: true, Status: "installed"})
	bundles = bundlesFor(webmail, "apt", nil, false, false)
	bundle := bundleViewByID(t, bundles, "mail.webmail")
	// Only the profile install proves a mail setup, so the action is Repair.
	if bundle.Status != bundleStatusInstalledUnverified || bundle.Action != bundleActionRepair || bundle.WarningCode != mailProfileWarningWebmailUnverified {
		t.Fatalf("webmail without its proof = %+v", bundle)
	}
	bundles = bundlesFor(webmail, "apt", map[string]mailProfileAttemptProof{
		"webmail": {Status: mailProfileAttemptSucceeded, Verified: true},
	}, true, true)
	if bundle := bundleViewByID(t, bundles, "mail.webmail"); bundle.Status != bundleStatusInstalledVerified || bundle.Action != bundleActionRepair {
		t.Fatalf("verified webmail = %+v", bundle)
	}
	// Core mail inherits the larger profile's proof, as the profiles do.
	if bundle := bundleViewByID(t, bundles, "mail.core"); bundle.Status != bundleStatusInstalledVerified {
		t.Fatalf("core mail under verified webmail = %+v", bundle)
	}

	// A conflicting SMTP server blocks with the profile's typed code.
	bundles = bundlesFor([]serviceObservation{bundleRunning("exim")}, "apt", nil, false, true)
	if bundle := bundleViewByID(t, bundles, "mail.core"); bundle.Status != bundleStatusBlocked || bundle.BlockedCode != mailProfileBlockedConflict || bundle.Action != bundleActionNone {
		t.Fatalf("core mail beside Exim = %+v", bundle)
	}
}

func TestComponentBundlesInstallOnlyWhatIsMissing(t *testing.T) {
	// Nginx alone: a component bundle is never partial; Install installs
	// only PHP-FPM, and Nginx is listed as kept.
	bundles := bundlesFor([]serviceObservation{bundleRunning("nginx")}, "apt", nil, false, true)
	if bundle := bundleViewByID(t, bundles, "web.nginx_php"); bundle.Status != bundleStatusAvailable || bundle.Action != bundleActionInstall ||
		!reflect.DeepEqual(bundle.MissingComponents, []string{"php-fpm"}) {
		t.Fatalf("web bundle with Nginx only = %+v", bundle)
	}
	stoppedWeb := bundlesFor([]serviceObservation{bundleRunning("nginx"), {ID: "php-fpm", IsInstalled: true, Status: "inactive (dead)"}}, "apt", nil, false, true)
	if bundle := bundleViewByID(t, stoppedWeb, "web.nginx_php"); bundle.Status != bundleStatusInstalledUnverified || bundle.Action != bundleActionNone {
		t.Fatalf("web bundle with PHP-FPM stopped = %+v", bundle)
	}
}

// Apache already fills the web server role: a database bundle reuses it
// (Nginx is "not needed"), and the Nginx + PHP-FPM bundle, which names Nginx
// itself, is blocked beside it.
// Apache web sunucusu rolünü doldurur: veritabanı paketi onu kullanır.
func TestBundlesOnAnApacheShapedServer(t *testing.T) {
	bundles := bundlesFor([]serviceObservation{bundleRunning("mariadb"), bundleRunning("apache"), bundleRunning("php-fpm")}, "apt", nil, false, true)
	if bundle := bundleViewByID(t, bundles, "db.mariadb"); bundle.Status != bundleStatusAvailable ||
		!reflect.DeepEqual(bundle.MissingComponents, []string{"phpmyadmin"}) {
		t.Fatalf("MariaDB + phpMyAdmin under Apache = %+v", bundle)
	}
	if bundle := bundleViewByID(t, bundles, "db.postgresql"); bundle.Status != bundleStatusAvailable ||
		!reflect.DeepEqual(bundle.MissingComponents, []string{"postgresql", "phppgadmin"}) {
		t.Fatalf("PostgreSQL + phpPgAdmin under Apache = %+v", bundle)
	}
	if bundle := bundleViewByID(t, bundles, "web.nginx_php"); bundle.Status != bundleStatusBlocked || bundle.BlockedCode != mailProfileBlockedConflict {
		t.Fatalf("Nginx bundle beside Apache = %+v", bundle)
	}
	bundles = bundlesFor([]serviceObservation{bundleRunning("mariadb"), bundleRunning("apache"), bundleRunning("php-fpm"),
		{ID: "phpmyadmin", IsInstalled: true, Status: "installed"}}, "apt", nil, false, true)
	if bundle := bundleViewByID(t, bundles, "db.mariadb"); bundle.Status != bundleStatusInstalledVerified || len(bundle.MissingComponents) != 0 {
		t.Fatalf("MariaDB + phpMyAdmin complete under Apache = %+v", bundle)
	}
	// Apache alone: nothing conflicts; Nginx is not needed.
	bundles = bundlesFor([]serviceObservation{bundleRunning("apache")}, "apt", nil, false, true)
	if bundle := bundleViewByID(t, bundles, "db.postgresql"); bundle.Status != bundleStatusAvailable ||
		!reflect.DeepEqual(bundle.MissingComponents, []string{"postgresql", "php-fpm", "phppgadmin"}) {
		t.Fatalf("PostgreSQL + phpPgAdmin beside Apache only = %+v", bundle)
	}
}

// Arch: phpPgAdmin has no pacman package. The whole bundle is blocked with
// the existing typed reason naming phpPgAdmin; a subset is never offered,
// even where PostgreSQL is installed. MariaDB + phpMyAdmin stays installable.
// Arch'ta phpPgAdmin paketi yok: paketin tamamı engellidir, alt küme sunulmaz.
func TestBundlesOnAnArchShapedServer(t *testing.T) {
	for _, observations := range [][]serviceObservation{nil, {bundleRunning("postgresql")}, {bundleRunning("postgresql"), bundleRunning("nginx"), bundleRunning("php-fpm")}} {
		bundles := bundlesFor(observations, "pacman", nil, false, true)
		pg := bundleViewByID(t, bundles, "db.postgresql")
		if pg.Status != bundleStatusBlocked || pg.BlockedCode != mailProfileBlockedNotInstallable ||
			pg.BlockedVars["component"] != "phpPgAdmin" || pg.Action != bundleActionNone {
			t.Fatalf("PostgreSQL + phpPgAdmin on pacman with %v = %+v", observations, pg)
		}
		if maria := bundleViewByID(t, bundles, "db.mariadb"); maria.Status != bundleStatusAvailable || maria.Action != bundleActionInstall {
			t.Fatalf("MariaDB + phpMyAdmin on pacman with %v = %+v", observations, maria)
		}
	}
}

// The payload carries the bundles at every fill point; the GET of a stale
// record says nothing is known.
func TestManagedServicesPayloadCarriesBundles(t *testing.T) {
	panel := newManagedServiceCachePanel(t)
	recorder := httptest.NewRecorder()
	panel.handleManagedServices(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/managed-services", nil))
	var payload managedServicesPayload
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Bundles) != len(bundleDefinitions) {
		t.Fatalf("no-scan payload bundles = %d", len(payload.Bundles))
	}
	for _, bundle := range payload.Bundles {
		if bundle.Status != bundleStatusUnknown {
			t.Fatalf("no-scan bundle %+v", bundle)
		}
	}

	raw, err := json.Marshal(scanCacheDoc{Observations: []serviceObservation{bundleRunning("mariadb"), bundleRunning("nginx"), bundleRunning("php-fpm"),
		{ID: "phpmyadmin", IsInstalled: true, Status: "installed"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := panel.db.GetDB().Exec(
		`INSERT INTO service_scan_cache (id, data, scanned_at) VALUES (1, ?, ?)`, string(raw),
		time.Now().UTC().Add(-6*time.Minute).Format(time.RFC3339),
	); err != nil {
		t.Fatal(err)
	}
	recorder = httptest.NewRecorder()
	panel.handleManagedServices(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/managed-services", nil))
	payload = managedServicesPayload{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if bundle := bundleViewByID(t, payload.Bundles, "db.mariadb"); bundle.Status != bundleStatusUnknown {
		t.Fatalf("stale record bundle = %+v", bundle)
	}

	if _, err := panel.db.GetDB().Exec(
		"UPDATE service_scan_cache SET scanned_at=? WHERE id=1",
		time.Now().UTC().Add(-time.Minute).Format(time.RFC3339),
	); err != nil {
		t.Fatal(err)
	}
	recorder = httptest.NewRecorder()
	panel.handleManagedServices(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/managed-services", nil))
	payload = managedServicesPayload{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if bundle := bundleViewByID(t, payload.Bundles, "db.mariadb"); bundle.Status != bundleStatusInstalledVerified {
		t.Fatalf("fresh record bundle = %+v", bundle)
	}
	cached, fresh, err := panel.managedServicesCacheWithin(t.Context(), 5*time.Minute)
	if err != nil || !fresh {
		t.Fatalf("cache within: fresh=%v err=%v", fresh, err)
	}
	if bundle := bundleViewByID(t, cached.Bundles, "db.mariadb"); bundle.Status != bundleStatusInstalledVerified {
		t.Fatalf("conditional scan cache bundle = %+v", bundle)
	}
}

// A mail setup installed before the DNS identity stopped being ready stays
// installed beside the host-wide refusal; nothing can be started.
func TestInstalledMailBundleStaysInstalledBesideAHostWideRefusal(t *testing.T) {
	webmail := []serviceObservation{bundleRunning("postfix"), bundleRunning("dovecot"), bundleRunning("nginx"), bundleRunning("php-fpm"),
		{ID: "roundcube", IsInstalled: true, Status: "installed"}}
	services := catalogView(webmail, "apt")
	attempts := map[string]mailProfileAttemptProof{"webmail": {Status: mailProfileAttemptSucceeded, Verified: true}}
	blocked := mailProfilesView(services, true, "apt", mailProfileDNSIdentityMessage, true, true, attempts)
	unblocked := mailProfilesView(services, true, "apt", "", true, true, attempts)
	bundles := bundlesView(services, true, "apt", blocked, unblocked)
	if bundle := bundleViewByID(t, bundles, "mail.webmail"); bundle.Status != bundleStatusInstalledVerified || bundle.Action != bundleActionNone ||
		bundle.BlockedCode != mailProfileBlockedDNSIdentity {
		t.Fatalf("installed webmail beside the DNS refusal = %+v", bundle)
	}
	if bundle := bundleViewByID(t, bundles, "mail.protected"); bundle.Status != bundleStatusBlocked || bundle.Action != bundleActionNone {
		t.Fatalf("an offer beside the DNS refusal = %+v", bundle)
	}
	// Control: without the refusal the same webmail offers Repair.
	if bundle := bundleViewByID(t, bundlesView(services, true, "apt", unblocked, unblocked), "mail.webmail"); bundle.Action != bundleActionRepair {
		t.Fatalf("webmail without the refusal = %+v", bundle)
	}
}
