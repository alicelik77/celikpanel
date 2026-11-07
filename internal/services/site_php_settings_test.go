package services

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.128 (docs/SITE-LIMITS.md): a site's own PHP values in its pool and
// the site's nginx settings file.

func TestSitePoolWithoutOwnValuesStaysByteIdentical(t *testing.T) {
	path, _ := withSealedPool(t)
	before := readPoolTestFile(t, path)
	// A render with limits (a plan's ceilings above the pool's values) and
	// no values of the site's own writes nothing new.
	request := sitePoolTestRequest(transport.SitePoolOperationApply)
	request.PHPLimits = transport.SitePHPLimits{Known: true, MemoryMB: 512, TimeS: 300, UploadMB: 256, InputVars: 10000, Processes: 20}
	result, err := NewPHPPoolManager().ApplySitePool(request)
	if err != nil || result.Outcome != transport.SiteFileOutcomeUnchanged {
		t.Fatalf("apply: %+v %v", result, err)
	}
	if after := readPoolTestFile(t, path); after != before {
		t.Fatalf("pool changed:\n%s", after)
	}
}

func TestSitePoolSettingsAreAdminValuesAndReadBack(t *testing.T) {
	path, reloads := withSealedPool(t)
	manager := NewPHPPoolManager()
	request := sitePoolTestRequest(transport.SitePoolOperationSettings)
	request.PHPLimits = transport.SitePHPLimits{Known: true, MemoryMB: 512, TimeS: 300, UploadMB: 256, InputVars: 10000, Processes: 20}
	request.PHPSettings = transport.SitePHPSettings{MemoryMB: 256, TimeS: 120, UploadMB: 64, InputVars: 3000, Processes: 8,
		DisplayErrors: "off", ErrorLevel: "production", ShortOpenTag: "off"}
	result, err := manager.ApplySitePool(request)
	if err != nil || result.Outcome != transport.SiteFileOutcomeWritten || *reloads != 1 {
		t.Fatalf("settings: %+v %v reloads=%d", result, err, *reloads)
	}
	content := readPoolTestFile(t, path)
	for _, want := range []string{
		"php_admin_value[memory_limit] = 256M", "php_admin_value[max_execution_time] = 120",
		"request_terminate_timeout = 130s", "php_admin_value[upload_max_filesize] = 64M",
		"php_admin_value[post_max_size] = 64M", "php_admin_value[max_input_vars] = 3000",
		"php_flag[display_errors] = off", "php_value[error_reporting] = 22527", "pm.max_children = 8",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("pool lacks %q:\n%s", want, content)
		}
	}
	own := sitePHPSettingsIn([]byte(content))
	if own.MemoryMB != 256 || own.TimeS != 120 || own.UploadMB != 64 || own.InputVars != 3000 || own.ErrorLevel != "production" || own.DisplayErrors != "off" {
		t.Fatalf("read back %+v", own)
	}
	// A disk-full render keeps the site's values (they are read back).
	full := sitePoolTestRequest(transport.SitePoolOperationApply)
	full.Uploads = transport.SitePoolUploadsOff
	if _, err := manager.ApplySitePool(full); err != nil {
		t.Fatal(err)
	}
	content = readPoolTestFile(t, path)
	if !strings.Contains(content, "php_admin_value[memory_limit] = 256M") || !strings.HasSuffix(strings.TrimSpace(content), poolUploadsOffLine) {
		t.Fatalf("disk-full render lost the values or the uploads line is not last:\n%s", content)
	}
	// A lowered ceiling clamps at the next render; the process count too.
	lowered := sitePoolTestRequest(transport.SitePoolOperationApply)
	lowered.PHPLimits = transport.SitePHPLimits{Known: true, MemoryMB: 128, Processes: 4}
	if _, err := manager.ApplySitePool(lowered); err != nil {
		t.Fatal(err)
	}
	content = readPoolTestFile(t, path)
	if !strings.Contains(content, "php_admin_value[memory_limit] = 128M") || !strings.Contains(content, "pm.max_children = 4") {
		t.Fatalf("lowered ceiling not applied:\n%s", content)
	}
	// Reset to default: every line of the site's block goes.
	reset := sitePoolTestRequest(transport.SitePoolOperationSettings)
	if _, err := manager.ApplySitePool(reset); err != nil {
		t.Fatal(err)
	}
	if content = readPoolTestFile(t, path); strings.Contains(content, "php_admin_value[memory_limit]") || strings.Contains(content, sitePHPOwnMarker) {
		t.Fatalf("reset left the block:\n%s", content)
	}
}

func TestSitePoolPlanCeilingBelowPHPIniIsWrittenAsThePlans(t *testing.T) {
	path, _ := withSealedPool(t)
	ini := phpINIPath("8.3")
	if !strings.HasPrefix(ini, phpEtcDir) {
		t.Skipf("php.ini of this layout is %s", ini)
	}
	if err := os.MkdirAll(filepath.Dir(ini), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ini, []byte("memory_limit = 1G\nmax_execution_time = 30\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	request := sitePoolTestRequest(transport.SitePoolOperationApply)
	request.PHPLimits = transport.SitePHPLimits{Known: true, MemoryMB: 256, InheritMemoryMB: 256, TimeS: 300}
	if _, err := NewPHPPoolManager().ApplySitePool(request); err != nil {
		t.Fatal(err)
	}
	content := readPoolTestFile(t, path)
	if !strings.Contains(content, sitePHPPlanMarker) || !strings.Contains(content, "php_admin_value[memory_limit] = 256M") {
		t.Fatalf("plan ceiling not written:\n%s", content)
	}
	if own := sitePHPSettingsIn([]byte(content)); own.MemoryMB != 0 {
		t.Fatalf("the plan's line was read as the site's own value: %+v", own)
	}
	if strings.Contains(content, "max_execution_time") {
		t.Fatalf("a time line was written although php.ini is under the ceiling:\n%s", content)
	}
}

func TestSitePHPSettingsValidation(t *testing.T) {
	bad := []transport.SitePHPSettings{
		{MemoryMB: 8}, {TimeS: 4000}, {UploadMB: 20000}, {Processes: 201}, {DisplayErrors: "yes"},
		{ErrorLevel: "E_ALL"}, {Timezone: "../../etc/passwd"}, {Timezone: "Europe/Istanbul\nphp_admin_value[x]"},
	}
	for _, each := range bad {
		if ValidateSitePHPSettings(each) == nil {
			t.Errorf("accepted %+v", each)
		}
	}
	if err := ValidateSitePHPSettings(transport.SitePHPSettings{MemoryMB: 256, Timezone: "UTC"}); err != nil {
		t.Fatal(err)
	}
}

func TestPHPININumber(t *testing.T) {
	for in, want := range map[string]int{"128M": 128, "1G": 1024, "512K": 1, "-1": -1, "2097152": 2} {
		if got, ok := PHPININumber(in, true); !ok || got != want {
			t.Errorf("%s = %d %v, want %d", in, got, ok, want)
		}
	}
}

const siteSettingsDomain = "settings.example"

func TestSiteSettingsIncludeWritesChecksAndRemoves(t *testing.T) {
	h := newManagedVhostHarness(t)
	response := h.ng.ApplySiteSettingsInclude(transport.SiteSettingsIncludeRequest{Domain: siteSettingsDomain, UploadMB: 64, TimeoutS: 130})
	if response.Outcome != transport.SiteSettingsIncludeWritten || h.reloads != 1 {
		t.Fatalf("write: %+v reloads=%d", response, h.reloads)
	}
	content, err := os.ReadFile(response.Path)
	if err != nil || !strings.Contains(string(content), "client_max_body_size 64m;") || !strings.Contains(string(content), "fastcgi_read_timeout 130s;") {
		t.Fatalf("file %s %v", content, err)
	}
	again := h.ng.ApplySiteSettingsInclude(transport.SiteSettingsIncludeRequest{Domain: siteSettingsDomain, UploadMB: 64, TimeoutS: 130})
	if again.Outcome != transport.SiteSettingsIncludeUnchanged || h.reloads != 1 {
		t.Fatalf("identical: %+v", again)
	}
	removed := h.ng.ApplySiteSettingsInclude(transport.SiteSettingsIncludeRequest{Domain: siteSettingsDomain})
	if removed.Outcome != transport.SiteSettingsIncludeRemoved {
		t.Fatalf("remove: %+v", removed)
	}
	if _, err := os.Stat(response.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("file still there: %v", err)
	}
}

func TestSiteSettingsIncludeLeavesAnAppsUploadLineAndKeepsOwnerEdits(t *testing.T) {
	h := newManagedVhostHarness(t)
	if err := h.ng.PublishAppInclude(siteSettingsDomain, "app-nextcloud.conf", "client_max_body_size 512m;\n"); err != nil {
		t.Fatal(err)
	}
	response := h.ng.ApplySiteSettingsInclude(transport.SiteSettingsIncludeRequest{Domain: siteSettingsDomain, UploadMB: 64, TimeoutS: 130})
	if response.Outcome != transport.SiteSettingsIncludeWritten || !strings.HasSuffix(response.UploadOwner, "app-nextcloud.conf") {
		t.Fatalf("write: %+v", response)
	}
	content, _ := os.ReadFile(response.Path)
	if strings.Contains(string(content), "client_max_body_size") {
		t.Fatalf("a second client_max_body_size was written:\n%s", content)
	}
	if err := os.WriteFile(response.Path, append(content, []byte("# mine\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	kept := h.ng.ApplySiteSettingsInclude(transport.SiteSettingsIncludeRequest{Domain: siteSettingsDomain, TimeoutS: 70})
	if kept.Outcome != transport.SiteSettingsIncludeKept {
		t.Fatalf("owner edit not kept: %+v", kept)
	}
}

func TestSiteSettingsIncludePutsBackWhenNginxRefuses(t *testing.T) {
	h := newManagedVhostHarness(t)
	calls := 0
	h.ng.validateNginx = func() error {
		calls++
		if calls == 2 { // the check after the write
			return errors.New("nginx: [emerg] invalid value")
		}
		return nil
	}
	response := h.ng.ApplySiteSettingsInclude(transport.SiteSettingsIncludeRequest{Domain: siteSettingsDomain, UploadMB: 64})
	if response.Outcome != transport.SiteSettingsIncludeRefused || !response.Restored || h.reloads != 0 {
		t.Fatalf("refusal: %+v reloads=%d", response, h.reloads)
	}
	if _, err := os.Stat(response.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("refused file stayed: %v", err)
	}
	// nginx already refusing before the change: an unmet prerequisite,
	// nothing written.
	h.ng.validateNginx = func() error { return errors.New("nginx: [emerg] elsewhere") }
	pre := h.ng.ApplySiteSettingsInclude(transport.SiteSettingsIncludeRequest{Domain: siteSettingsDomain, UploadMB: 64})
	if pre.Outcome != transport.SiteSettingsIncludeRefused || !strings.Contains(pre.Error, "already fails") {
		t.Fatalf("prerequisite: %+v", pre)
	}
}

func TestNewPoolStartsWithinThePlansProcessLimit(t *testing.T) {
	path := withPoolTree(t, "8.3", "site42", seededPool)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	stubWebServerAccount(t, "www-data")
	manager, err := NewPHPFPMManager()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.CreatePoolWithin(42, "celik_site42", "8.3", transport.SitePHPLimits{Known: true, Processes: 2}); err != nil {
		t.Fatalf("create pool: %v", err)
	}
	content := readPoolTestFile(t, path)
	if !strings.Contains(content, "pm.max_children = 2") || !strings.Contains(content, "pm.max_spare_servers = 2") {
		t.Fatalf("pool not within the limit:\n%s", content)
	}
	// Without a limit the default stays 5 (byte-identical to earlier releases).
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.CreatePool(42, "celik_site42", "8.3"); err != nil {
		t.Fatal(err)
	}
	if content := readPoolTestFile(t, path); !strings.Contains(content, "pm.max_children = 5") {
		t.Fatalf("default pool:\n%s", content)
	}
}
