package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.118 lab findings: WordPress installed without mysqli; the installer's
// own reason was not shown; the missing-extensions refusal had no path the
// administrator could take in the panel; a half-done removal said "an answer
// was lost".

func TestWordPressRefusesMissingExtensionsBeforeChangingAnything(t *testing.T) {
	fixture := newWordPressInstallFixture(t)
	agent := &wordpressInstallAgent{
		createResponse:  transport.CreateDatabaseResponse{Success: true, OwnedByOperation: true},
		installResponse: transport.InstallWordPressResponse{Installed: true},
		prereqMissing:   []transport.AppMissingExtension{{Extension: "mysqli", Package: "php8.2-mysql"}},
	}
	attachWordPressInstallAgent(t, fixture.panel, agent)
	recorder := fixture.install(t)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var body apiErrorBody
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != errCodeAppExtensionsMissing || !strings.Contains(body.Error, "php8.2-mysql") ||
		body.Action != "/components?component=php-fpm&install=php8.2-fpm" {
		t.Fatalf("refusal %+v", body)
	}
	creates, deletes, installs := agent.snapshot()
	var operations int
	if err := fixture.panel.db.GetDB().QueryRow(`SELECT COUNT(*) FROM application_install_operations`).Scan(&operations); err != nil {
		t.Fatal(err)
	}
	if len(creates)+len(deletes)+len(installs) != 0 || operations != 0 {
		t.Fatalf("something changed: create=%d delete=%d install=%d operations=%d", len(creates), len(deletes), len(installs), operations)
	}
}

func TestCatalogExtensionsInstallPackageNamesOneComponentsInstall(t *testing.T) {
	missing := []transport.AppMissingExtension{
		{Extension: "curl", Package: "php8.4-curl"}, {Extension: "gd", Package: "php8.4-gd"},
		{Extension: "pdo_mysql", Package: "php8.4-mysql"},
	}
	if got := catalogExtensionsInstallPackage("apt", "8.4", missing); got != "php8.4-fpm" {
		t.Fatalf("Debian 13's PHP 8.4 extensions: %q", got)
	}
	if got := catalogExtensionsInstallPackage("apt", "8.4", append(missing, transport.AppMissingExtension{Extension: "imagick", Package: "php8.4-imagick"})); got != "" {
		t.Fatalf("an extension PHP-FPM's install does not bring was promised: %q", got)
	}
	if got := catalogExtensionsInstallPackage("pacman", "8.4", []transport.AppMissingExtension{{Extension: "gd", Package: "php-gd"}}); got != "" {
		t.Fatalf("Arch Linux: %q", got)
	}
	if got := catalogExtensionsInstallPackage("apt", "8.4", []transport.AppMissingExtension{{Extension: "mysqli", Enable: true}}); got != "" {
		t.Fatalf("a switched-off extension: %q", got)
	}
	if got := catalogExtensionsInstallPackage("apt", "8.4; rm", missing); got != "" {
		t.Fatalf("an invalid PHP line: %q", got)
	}
}

func TestInstallerOutputTailShowsTheEndWithoutSecrets(t *testing.T) {
	output := strings.Repeat("noise\n", 20) + "\nPHP Warning:  Failed to open stream: Permission denied\n" +
		"PHP Fatal error:  Failed opening required prepend.php\nconnect with DbSecret123 failed\n" + strings.Repeat("x", 500)
	lines := installerOutputTail(output, "DbSecret123", "")
	if len(lines) != 8 {
		t.Fatalf("lines = %d", len(lines))
	}
	joined := strings.Join(lines, "\n")
	if strings.Contains(joined, "DbSecret123") || !strings.Contains(joined, "PHP Fatal error") || len([]rune(lines[7])) > 301 {
		t.Fatalf("tail %q", lines)
	}
	if installerOutputTail("  \n \n") != nil && len(installerOutputTail("  \n \n")) != 0 {
		t.Fatal("empty output produced lines")
	}
}

func TestCatalogSetupFailureShowsTheInstallersOwnReason(t *testing.T) {
	f, agent := newCatalogFixture(t)
	agent.setup = transport.SetupCatalogAppResponse{ErrorKind: transport.AppFailSetup,
		Error:  "Joomla's own installer did not finish (Joomla CLI installer: it ended with exit code 255)",
		Output: "PHP Fatal error:  Failed opening required '/var/lib/celikpanel-app-setup/prepend.php'"}
	recorder := installCatalogApp(t, f, "joomla")
	var body apiErrorBody
	_ = json.Unmarshal(recorder.Body.Bytes(), &body)
	if recorder.Code != http.StatusBadGateway || body.Code != errCodeAppSetupFailed || len(body.Details) != 1 ||
		!strings.Contains(body.Details[0], "Failed opening required") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var lastError string
	_ = f.panel.db.GetDB().QueryRow(`SELECT last_error FROM app_catalog_operations ORDER BY rowid DESC LIMIT 1`).Scan(&lastError)
	if !strings.Contains(lastError, "Failed opening required") {
		t.Fatalf("the list does not keep the installer's reason: %q", lastError)
	}
}

func TestCatalogPartialRemovalSaysRemoveAgain(t *testing.T) {
	f, agent := newCatalogFixture(t)
	if recorder := installCatalogApp(t, f, "drupal"); recorder.Code != http.StatusOK {
		t.Fatalf("install: %d %s", recorder.Code, recorder.Body.String())
	}
	op, _ := latestCatalogOperation(t, f)
	agent.remove = transport.RemoveCatalogAppResponse{ErrorKind: transport.AppFailRemove,
		Error: "removing Drupal stopped part way; Press Remove again"}
	recorder := appOperation(t, f, http.MethodDelete, op, "remove", `{"confirm":"wordpress.example"}`)
	var body apiErrorBody
	_ = json.Unmarshal(recorder.Body.Bytes(), &body)
	if recorder.Code != http.StatusBadGateway || body.Code != errCodeAppRemoveIncomplete {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var status, kind string
	_ = f.panel.db.GetDB().QueryRow(`SELECT status, error_kind FROM app_catalog_operations WHERE operation_id = ?`, op).Scan(&status, &kind)
	if status != appOpNeedsReview || kind != transport.AppFailRemove {
		t.Fatalf("status=%s kind=%s", status, kind)
	}
	// Remove again continues it.
	agent.remove = transport.RemoveCatalogAppResponse{FilesRemoved: true, RulesRemoved: true}
	if again := appOperation(t, f, http.MethodDelete, op, "remove", `{"confirm":"wordpress.example"}`); again.Code != http.StatusOK {
		t.Fatalf("remove again: %d %s", again.Code, again.Body.String())
	}
	if _, status := latestCatalogOperation(t, f); status != appOpRemoved {
		t.Fatalf("status %s", status)
	}
}
