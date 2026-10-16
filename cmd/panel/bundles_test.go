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
	"strings"
	"testing"
	"time"
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
	return bundlesView(services, true, family, profiles)
}

func bundleRunning(id string) serviceObservation {
	return serviceObservation{ID: id, IsInstalled: true, Status: "active (running)"}
}

// The registry is the owner's list of 2026-10-17 (D-034), closed and ordered:
// mail through the three mail profiles, two databases with one add-on each,
// and one web bundle. Every component is a catalogue component.
func TestBundleRegistryIsTheOwnersList(t *testing.T) {
	want := map[string]struct {
		category, kind, profile, parent string
		components, addons              []string
	}{
		"mail.core":           {"email", bundleKindMailProfile, "core-mail", "", []string{"postfix", "dovecot"}, []string{"mail.protected"}},
		"mail.webmail":        {"email", bundleKindMailProfile, "webmail", "", []string{"postfix", "dovecot", "nginx", "php-fpm", "roundcube"}, []string{"mail.protected"}},
		"mail.protected":      {"email", bundleKindMailProfile, "protected-mail", "", []string{"postfix", "dovecot", "rspamd"}, nil},
		"db.postgresql":       {"database", bundleKindComponents, "", "", []string{"postgresql"}, []string{"db.postgresql.admin"}},
		"db.mariadb":          {"database", bundleKindComponents, "", "", []string{"mariadb"}, []string{"db.mariadb.admin"}},
		"db.postgresql.admin": {"database", bundleKindComponents, "", "db.postgresql", []string{"nginx", "php-fpm", "phppgadmin"}, nil},
		"db.mariadb.admin":    {"database", bundleKindComponents, "", "db.mariadb", []string{"nginx", "php-fpm", "phpmyadmin"}, nil},
		"web.nginx_php":       {"web", bundleKindComponents, "", "", []string{"nginx", "php-fpm"}, nil},
	}
	if len(bundleDefinitions) != len(want) {
		t.Fatalf("bundle count = %d, want %d", len(bundleDefinitions), len(want))
	}
	for id, expected := range want {
		definition, ok := bundleByID(id)
		if !ok {
			t.Fatalf("bundle %s is missing", id)
		}
		if definition.Category != expected.category || definition.Kind != expected.kind ||
			definition.ProfileID != expected.profile || definition.Parent != expected.parent ||
			!reflect.DeepEqual(definition.Components, expected.components) ||
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
	for _, bundle := range bundlesView(services, false, "apt", profiles) {
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
	// An add-on of a database that is not installed is not installable: its
	// requirement (the database) is outside the plan.
	admin := bundleViewByID(t, bundles, "db.mariadb.admin")
	if admin.Status != bundleStatusBlocked || admin.BlockedCode != mailProfileBlockedRequires || admin.Parent != "db.mariadb" {
		t.Fatalf("add-on before its parent = %+v", admin)
	}
}

// Mail bundles say what the mail profiles say: verified, installed but not
// verified, half-installed.
func TestMailBundlesFollowTheMailProfiles(t *testing.T) {
	coreMail := []serviceObservation{bundleRunning("postfix"), bundleRunning("dovecot")}
	bundles := bundlesFor(coreMail, "apt", nil, false, true)
	if bundle := bundleViewByID(t, bundles, "mail.core"); bundle.Status != bundleStatusInstalledUnverified || bundle.Action != bundleActionVerify || len(bundle.MissingComponents) != 0 {
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
	if bundle.Status != bundleStatusInstalledUnverified || bundle.Action != bundleActionVerify || bundle.WarningCode != mailProfileWarningWebmailUnverified {
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

func TestComponentBundlesVerifyOnAFreshScanOnly(t *testing.T) {
	bundles := bundlesFor([]serviceObservation{bundleRunning("mariadb")}, "apt", nil, false, true)
	if bundle := bundleViewByID(t, bundles, "db.mariadb"); bundle.Status != bundleStatusInstalledVerified || bundle.Action != bundleActionNone {
		t.Fatalf("running MariaDB = %+v", bundle)
	}
	admin := bundleViewByID(t, bundles, "db.mariadb.admin")
	if admin.Status != bundleStatusAvailable || admin.Action != bundleActionInstall ||
		!reflect.DeepEqual(admin.MissingComponents, []string{"nginx", "php-fpm", "phpmyadmin"}) {
		t.Fatalf("phpMyAdmin add-on after MariaDB = %+v", admin)
	}

	stopped := bundlesFor([]serviceObservation{{ID: "mariadb", IsInstalled: true, Status: "inactive (dead)"}}, "apt", nil, false, true)
	if bundle := bundleViewByID(t, stopped, "db.mariadb"); bundle.Status != bundleStatusInstalledUnverified || bundle.Action != bundleActionVerify {
		t.Fatalf("stopped MariaDB = %+v", bundle)
	}

	// Nginx alone: the web bundle was started and continues with PHP-FPM.
	bundles = bundlesFor([]serviceObservation{bundleRunning("nginx")}, "apt", nil, false, true)
	if bundle := bundleViewByID(t, bundles, "web.nginx_php"); bundle.Status != bundleStatusPartial || bundle.Action != bundleActionContinue ||
		!reflect.DeepEqual(bundle.MissingComponents, []string{"php-fpm"}) {
		t.Fatalf("web bundle with Nginx only = %+v", bundle)
	}

	// Shared parts present (from webmail) do not make the add-on half
	// installed; they are skipped.
	bundles = bundlesFor([]serviceObservation{bundleRunning("mariadb"), bundleRunning("nginx"), bundleRunning("php-fpm")}, "apt", nil, false, true)
	if bundle := bundleViewByID(t, bundles, "db.mariadb.admin"); bundle.Status != bundleStatusAvailable ||
		!reflect.DeepEqual(bundle.MissingComponents, []string{"phpmyadmin"}) {
		t.Fatalf("add-on with shared parts present = %+v", bundle)
	}

	// Apache already fills the web server role: the add-on reuses it rather
	// than conflicting with it.
	bundles = bundlesFor([]serviceObservation{bundleRunning("mariadb"), bundleRunning("apache"), bundleRunning("php-fpm")}, "apt", nil, false, true)
	if bundle := bundleViewByID(t, bundles, "db.mariadb.admin"); bundle.Status != bundleStatusAvailable ||
		!reflect.DeepEqual(bundle.MissingComponents, []string{"phpmyadmin"}) {
		t.Fatalf("add-on under Apache = %+v", bundle)
	}
	// A top-level web bundle names Nginx itself: beside Apache it is blocked.
	if bundle := bundleViewByID(t, bundles, "web.nginx_php"); bundle.Status != bundleStatusBlocked || bundle.BlockedCode != mailProfileBlockedConflict {
		t.Fatalf("Nginx bundle beside Apache = %+v", bundle)
	}
}

// A component not offered on this package family blocks the add-on with the
// existing typed code; the parent stays installable.
func TestAnAddonThatIsNotOfferedBlocksOnlyItself(t *testing.T) {
	bundles := bundlesFor([]serviceObservation{bundleRunning("postgresql")}, "pacman", nil, false, true)
	if bundle := bundleViewByID(t, bundles, "db.postgresql"); bundle.Status != bundleStatusInstalledVerified {
		t.Fatalf("PostgreSQL on pacman = %+v", bundle)
	}
	admin := bundleViewByID(t, bundles, "db.postgresql.admin")
	if admin.Status != bundleStatusBlocked || admin.BlockedCode != mailProfileBlockedNotInstallable ||
		admin.BlockedVars["component"] != "phpPgAdmin" || admin.Action != bundleActionNone {
		t.Fatalf("phpPgAdmin add-on on pacman = %+v", admin)
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

	raw, err := json.Marshal(scanCacheDoc{Observations: []serviceObservation{bundleRunning("mariadb")}})
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
