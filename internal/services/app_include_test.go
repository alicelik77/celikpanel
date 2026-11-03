package services

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const appIncludeDomain = "shop.example.test"

func TestPublishAppIncludeWritesSealedRulesAndChecksNginx(t *testing.T) {
	h := newManagedVhostHarness(t)
	if err := h.ng.PublishAppInclude(appIncludeDomain, "app-nextcloud.conf", "location = /x { return 404; }\n"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(PanelManagedDir(appIncludeDomain), "app-nextcloud.conf")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(content), managedRenderHeaderPrefix) || !strings.Contains(string(content), "location = /x") {
		t.Fatalf("the file is not CelikPanel's sealed text:\n%s", content)
	}
	if h.validates != 1 || h.reloads != 1 {
		t.Fatalf("validates=%d reloads=%d", h.validates, h.reloads)
	}
	// The same text again: nothing written, nothing reloaded.
	if err := h.ng.PublishAppInclude(appIncludeDomain, "app-nextcloud.conf", "location = /x { return 404; }\n"); err != nil {
		t.Fatal(err)
	}
	if h.validates != 1 || h.reloads != 1 {
		t.Fatalf("identical text was written again: validates=%d reloads=%d", h.validates, h.reloads)
	}
}

func TestPublishAppIncludePutsBackWhenNginxRefuses(t *testing.T) {
	h := newManagedVhostHarness(t)
	h.refuse = errors.New("nginx: [emerg] duplicate location")
	err := h.ng.PublishAppInclude(appIncludeDomain, "app-drupal.conf", "location = /x { return 404; }\n")
	if err == nil || !strings.Contains(err.Error(), "put back") {
		t.Fatalf("expected a refusal that says the file was put back, got %v", err)
	}
	if _, statErr := os.Lstat(filepath.Join(PanelManagedDir(appIncludeDomain), "app-drupal.conf")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("a refused include stayed in place: %v", statErr)
	}
	if h.reloads != 0 {
		t.Fatal("nginx was reloaded with a refused include")
	}
}

func TestAppIncludeEditedByTheOwnerIsNeverOverwrittenOrRemoved(t *testing.T) {
	h := newManagedVhostHarness(t)
	if err := h.ng.PublishAppInclude(appIncludeDomain, "app-joomla.conf", "location = /a { return 404; }\n"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(PanelManagedDir(appIncludeDomain), "app-joomla.conf")
	content, _ := os.ReadFile(path)
	edited := append(content, []byte("# the owner's line\n")...)
	if err := os.WriteFile(path, edited, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := h.ng.PublishAppInclude(appIncludeDomain, "app-joomla.conf", "location = /b { return 404; }\n"); !errors.Is(err, ErrAppIncludeKept) {
		t.Fatalf("an owner-edited include was not kept: %v", err)
	}
	if err := h.ng.RemoveAppInclude(appIncludeDomain, "app-joomla.conf"); !errors.Is(err, ErrAppIncludeKept) {
		t.Fatalf("an owner-edited include was not kept on removal: %v", err)
	}
	if now, _ := os.ReadFile(path); string(now) != string(edited) {
		t.Fatal("the owner's file changed")
	}
}

func TestRemoveAppIncludeRemovesOnlyCelikPanelsFile(t *testing.T) {
	h := newManagedVhostHarness(t)
	if err := h.ng.PublishAppInclude(appIncludeDomain, AppSetupLockFileName, RenderAppSetupLock(appIncludeDomain, "PrestaShop")); err != nil {
		t.Fatal(err)
	}
	if err := h.ng.RemoveAppInclude(appIncludeDomain, AppSetupLockFileName); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(PanelManagedDir(appIncludeDomain), AppSetupLockFileName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the setup lock stayed: %v", err)
	}
	// Absent is done.
	if err := h.ng.RemoveAppInclude(appIncludeDomain, AppSetupLockFileName); err != nil {
		t.Fatal(err)
	}
	if _, err := AppIncludePath(appIncludeDomain, "../vhost.conf"); err == nil {
		t.Fatal("an include name outside the app-*.conf form was accepted")
	}
}

func TestDeletedSiteTakesItsUnchangedAppIncludesAlong(t *testing.T) {
	h := newManagedVhostHarness(t)
	if err := h.ng.PublishAppInclude(appIncludeDomain, "app-phpbb.conf", "location = /a { return 404; }\n"); err != nil {
		t.Fatal(err)
	}
	removePanelManagedDir(appIncludeDomain)
	if _, err := os.Lstat(PanelManagedDir(appIncludeDomain)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the site's include directory stayed with CelikPanel's unchanged rules: %v", err)
	}
}

func TestSetupLockAnswersEveryAddressWithoutServingContent(t *testing.T) {
	body := RenderAppSetupLock(appIncludeDomain, "Nextcloud")
	for _, forbidden := range []string{"root ", "fastcgi_pass", "proxy_pass"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("the setup lock contains %q", forbidden)
		}
	}
	if !strings.Contains(body, "location ~ ^/ {") || !strings.Contains(body, "return 503") {
		t.Fatalf("the setup lock does not close the site:\n%s", body)
	}
}
