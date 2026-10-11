package services

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// D-031 second step (2026-10-11): the site's PHP-FPM pool file.

func sitePoolTestRequest(operation string) transport.SitePoolRequest {
	return transport.SitePoolRequest{SiteID: 42, Username: "celik_site42", PHPVersion: "8.3", Operation: operation}
}

// withSealedPool creates site42's pool the way site creation does and counts
// the reloads after it.
func withSealedPool(t *testing.T) (string, *int) {
	t.Helper()
	path := withPoolTree(t, "8.3", "site42", seededPool)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	stubWebServerAccount(t, "www-data")
	manager, err := NewPHPFPMManager()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.CreatePool(42, "celik_site42", "8.3"); err != nil {
		t.Fatalf("create pool: %v", err)
	}
	reloads := 0
	reloadPHPFPM = func(string) error { reloads++; return nil }
	return path, &reloads
}

func stubWebServerAccount(t *testing.T, account string) {
	t.Helper()
	old := phpLookupGroup
	phpLookupGroup = func(name string) bool { return name == account }
	t.Cleanup(func() { phpLookupGroup = old })
}

func readPoolTestFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func TestSitePoolIsSealedWithAnINIHeader(t *testing.T) {
	path, _ := withSealedPool(t)
	content := readPoolTestFile(t, path)
	if !strings.HasPrefix(content, ManagedHeaderPrefixINI) {
		t.Fatalf("pool does not start with the INI render header:\n%s", content)
	}
	declared, body, ok := splitManagedHeaderWith(ManagedHeaderPrefixINI, []byte(content))
	if !ok || sha256Hex(body) != declared {
		t.Fatalf("header digest does not match the body")
	}
	if _, _, ok := splitManagedHeader([]byte(content)); ok {
		t.Fatal("a pool header was read as the \"#\" form")
	}
	for _, want := range []string{"[site42]", "user = celik_site42", "listen = /var/run/php/php8.3-fpm-site42.sock", "listen.owner = www-data", "pm.max_children = 5"} {
		if !strings.Contains(content, want) {
			t.Errorf("sealed pool lacks %q", want)
		}
	}
	result, diff, _, err := NewPHPPoolManager().InspectSitePool(sitePoolTestRequest(transport.SitePoolOperationInspect))
	if err != nil || result.State != transport.SiteFileManagedUnchanged || diff != "" {
		t.Fatalf("inspect = %+v %q %v", result, diff, err)
	}
	if result.Kind != transport.SiteFileKindPHPPool {
		t.Fatalf("kind = %q", result.Kind)
	}
}

func TestSitePoolSaveRenderRule(t *testing.T) {
	path, reloads := withSealedPool(t)
	manager := NewPHPPoolManager()
	same := sitePoolTestRequest(transport.SitePoolOperationSave)
	same.Values = &transport.SitePoolValues{PM: "dynamic", PMMaxChildren: 5, PMStartServers: 2, PMMinSpareServers: 1, PMMaxSpareServers: 3, PMMaxRequests: 500}
	result, err := manager.ApplySitePool(same)
	if err != nil || result.Outcome != transport.SiteFileOutcomeUnchanged || *reloads != 0 {
		t.Fatalf("identical save: %+v %v reloads=%d", result, err, *reloads)
	}
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}
	changed := same
	changed.Values = &transport.SitePoolValues{PM: "ondemand", PMMaxChildren: 9}
	result, err = manager.ApplySitePool(changed)
	if err != nil || result.Outcome != transport.SiteFileOutcomeWritten || *reloads != 1 {
		t.Fatalf("changed save: %+v %v reloads=%d", result, err, *reloads)
	}
	content := readPoolTestFile(t, path)
	if !strings.Contains(content, "pm = ondemand") || !strings.Contains(content, "pm.max_children = 9") {
		t.Fatalf("values not written:\n%s", content)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o640 {
		t.Fatalf("mode = %o, want the file's 0640", info.Mode().Perm())
	}
}

func TestSitePoolOwnerDirectiveIsKeptOnSaveAndSwitch(t *testing.T) {
	path, reloads := withSealedPool(t)
	owner := readPoolTestFile(t, path) + "php_admin_value[memory_limit] = 512M\n"
	if err := os.WriteFile(path, []byte(owner), 0o644); err != nil {
		t.Fatal(err)
	}
	manager := NewPHPPoolManager()
	save := sitePoolTestRequest(transport.SitePoolOperationSave)
	save.Values = &transport.SitePoolValues{PM: "static", PMMaxChildren: 7}
	result, err := manager.ApplySitePool(save)
	if err != nil || result.Outcome != transport.SiteFileOutcomeKept || result.State != transport.SiteFileOwnerEdited {
		t.Fatalf("save on an owner-edited pool: %+v %v", result, err)
	}
	if readPoolTestFile(t, path) != owner || *reloads != 0 {
		t.Fatalf("owner's pool changed or reloaded (%d)", *reloads)
	}
	pending := readPoolTestFile(t, path+SiteFilePendingSuffix)
	if !strings.Contains(pending, "pm = static") || strings.Contains(pending, "memory_limit") {
		t.Fatalf("pending is not CelikPanel's text for the save:\n%s", pending)
	}
	// The page's difference names the owner's line and the held values.
	inspected, diff, _, err := manager.InspectSitePool(sitePoolTestRequest(transport.SitePoolOperationInspect))
	if err != nil || inspected.State != transport.SiteFileOwnerEdited || inspected.PendingPath == "" {
		t.Fatalf("inspect: %+v %v", inspected, err)
	}
	if !strings.Contains(diff, "-php_admin_value[memory_limit] = 512M") || !strings.Contains(diff, "+pm = static") {
		t.Fatalf("diff does not show the owner's line and the held text:\n%s", diff)
	}
	// A PHP switch on the kept pool writes nothing anywhere.
	if err := os.MkdirAll(poolDirPath("8.4"), 0o755); err != nil {
		t.Fatal(err)
	}
	switchRequest := sitePoolTestRequest(transport.SitePoolOperationSwitch)
	switchRequest.NewPHPVersion = "8.4"
	result, err = manager.ApplySitePool(switchRequest)
	if err != nil || result.Outcome != transport.SiteFileOutcomeKept || result.Path != path {
		t.Fatalf("switch on a kept pool: %+v %v", result, err)
	}
	if _, err := os.Stat(poolFilePath("8.4", "site42")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("switch wrote the target: %v", err)
	}
	if readPoolTestFile(t, path) != owner || *reloads != 0 {
		t.Fatal("switch changed the owner's pool")
	}
	// The older RPC paths answer the typed refusal too.
	err = manager.MigratePool("8.3", "8.4", "site42")
	var held *SiteFileHeldError
	if !errors.As(err, &held) || held.File.Kind != transport.SiteFileKindPHPPool {
		t.Fatalf("MigratePool on a kept pool: %v", err)
	}
	if parsed, ok := ParseSiteFileHeldError(err.Error()); !ok || parsed.Outcome != transport.SiteFileOutcomeKept || parsed.State != transport.SiteFileOwnerEdited {
		t.Fatalf("held error tokens = %+v %v", parsed, ok)
	}
}

func TestSitePoolTakeKeepsADatedCopyAndIsBoundToWhatWasShown(t *testing.T) {
	path, reloads := withSealedPool(t)
	owner := readPoolTestFile(t, path) + "php_flag[display_errors] = on\n"
	if err := os.WriteFile(path, []byte(owner), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	manager := NewPHPPoolManager()
	shown, _, _, err := manager.InspectSitePool(sitePoolTestRequest(transport.SitePoolOperationInspect))
	if err != nil {
		t.Fatal(err)
	}
	stale := sitePoolTestRequest(transport.SitePoolOperationTake)
	stale.ExpectedFileSHA256, stale.ExpectedRenderSHA256 = strings.Repeat("0", 64), shown.RenderSHA256
	if result, _ := manager.ApplySitePool(stale); result.Outcome != transport.SiteFileOutcomeRefused || result.Reason != transport.SiteFileReasonChanged {
		t.Fatalf("take on another file: %+v", result)
	}
	take := sitePoolTestRequest(transport.SitePoolOperationTake)
	take.ExpectedFileSHA256, take.ExpectedRenderSHA256 = shown.FileSHA256, shown.RenderSHA256
	result, err := manager.ApplySitePool(take)
	if err != nil || result.Outcome != transport.SiteFileOutcomeTaken || result.BackupPath == "" || *reloads != 1 {
		t.Fatalf("take: %+v %v reloads=%d", result, err, *reloads)
	}
	if readPoolTestFile(t, result.BackupPath) != owner {
		t.Fatal("dated copy is not the owner's bytes")
	}
	if info, _ := os.Stat(result.BackupPath); info.Mode().Perm() != 0o600 {
		t.Fatalf("copy mode = %o", info.Mode().Perm())
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Fatalf("taken file mode = %o, want the owner's 0600", info.Mode().Perm())
	}
	if strings.Contains(readPoolTestFile(t, path), "display_errors") {
		t.Fatal("take kept the owner's line")
	}
	if _, err := os.Lstat(path + SiteFilePendingSuffix); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("pending left after take: %v", err)
	}
}

func TestSitePoolSymlinkIsRefusedAndNotFollowed(t *testing.T) {
	path, _ := withSealedPool(t)
	target := filepath.Join(t.TempDir(), "elsewhere.conf")
	if err := os.WriteFile(target, []byte("[x]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	save := sitePoolTestRequest(transport.SitePoolOperationSave)
	save.Values = &transport.SitePoolValues{PM: "dynamic", PMMaxChildren: 8}
	result, err := NewPHPPoolManager().ApplySitePool(save)
	if err != nil || result.Outcome != transport.SiteFileOutcomeRefused || result.Reason != transport.SiteFileReasonSymlink {
		t.Fatalf("save through a link: %+v %v", result, err)
	}
	if readPoolTestFile(t, target) != "[x]\n" {
		t.Fatal("the link's target was written")
	}
}

func TestSitePoolLegacyAdoption(t *testing.T) {
	path := withPoolTree(t, "8.3", "site42", seededPool)
	stubWebServerAccount(t, "www-data")
	manager := NewPHPPoolManager()
	result, _, _, err := manager.InspectSitePool(sitePoolTestRequest(transport.SitePoolOperationInspect))
	if err != nil || result.State != transport.SiteFileManagedUnchanged || result.AdoptedFrom != legacyReleaseAlpha82 {
		t.Fatalf("frozen alpha.82 text: %+v %v", result, err)
	}
	// A saved alpha.8x text with other values is adopted too.
	saved := strings.Replace(seededPool, "pm.max_children = 5", "pm.max_children = 12", 1)
	if err := os.WriteFile(path, []byte(saved), 0o644); err != nil {
		t.Fatal(err)
	}
	result, _, _, _ = manager.InspectSitePool(sitePoolTestRequest(transport.SitePoolOperationInspect))
	if result.State != transport.SiteFileManagedUnchanged {
		t.Fatalf("saved alpha.8x text: %+v", result)
	}
	// An adopted pool is rewritten as this release's text on the next save,
	// keeping its values.
	save := sitePoolTestRequest(transport.SitePoolOperationSave)
	save.Values = &transport.SitePoolValues{PM: "dynamic", PMMaxChildren: 12, PMStartServers: 2, PMMinSpareServers: 1, PMMaxSpareServers: 3, PMMaxRequests: 500}
	if written, err := manager.ApplySitePool(save); err != nil || written.Outcome != transport.SiteFileOutcomeWritten || written.AdoptedFrom == "" {
		t.Fatalf("save of an adopted pool: %+v %v", written, err)
	}
	if !strings.HasPrefix(readPoolTestFile(t, path), ManagedHeaderPrefixINI) {
		t.Fatal("adopted pool not sealed")
	}
	// A headerless pool with a line no release wrote is unknown origin.
	if err := os.WriteFile(path, []byte(seededPool+"php_value[upload_max_filesize] = 64M\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, _, _, _ = manager.InspectSitePool(sitePoolTestRequest(transport.SitePoolOperationInspect))
	if result.State != transport.SiteFileUnknownOrigin {
		t.Fatalf("headerless pool with an owner's line: %+v", result)
	}
	// alpha.81's socket and owner (Arch, before 12 Oct) are recognised as alpha.81.
	stubWebServerAccount(t, "http")
	if err := os.WriteFile(path, []byte(seededPool), 0o644); err != nil {
		t.Fatal(err)
	}
	result, _, _, _ = manager.InspectSitePool(sitePoolTestRequest(transport.SitePoolOperationInspect))
	if result.State != transport.SiteFileManagedUnchanged || result.AdoptedFrom != legacyReleaseAlpha81 {
		t.Fatalf("alpha.81 text on a host whose web account is http: %+v", result)
	}
}

func TestSitePoolDeleteKeepsACopyOfAnEditedPool(t *testing.T) {
	path, _ := withSealedPool(t)
	owner := readPoolTestFile(t, path) + "php_admin_flag[log_errors] = on\n"
	if err := os.WriteFile(path, []byte(owner), 0o644); err != nil {
		t.Fatal(err)
	}
	save := sitePoolTestRequest(transport.SitePoolOperationSave)
	save.Values = &transport.SitePoolValues{PM: "dynamic"}
	if _, err := NewPHPPoolManager().ApplySitePool(save); err != nil {
		t.Fatal(err)
	}
	if err := NewPHPPoolManager().DeletePoolByName("8.3", "site42"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("pool remains: %v", err)
	}
	if _, err := os.Lstat(path + SiteFilePendingSuffix); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("pending remains: %v", err)
	}
	copies, _ := filepath.Glob(path + SiteFileBackupMarker + "*")
	if len(copies) != 1 || readPoolTestFile(t, copies[0]) != owner {
		t.Fatalf("copies = %v", copies)
	}
}

func TestSitePoolOneSitesKeptFileDoesNotStopAnother(t *testing.T) {
	path, _ := withSealedPool(t)
	if err := os.WriteFile(path, []byte(readPoolTestFile(t, path)+"; mine\nphp_value[x] = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manager, _ := NewPHPFPMManager()
	if _, err := manager.CreatePool(43, "celik_site43", "8.3"); err != nil {
		t.Fatal(err)
	}
	other := transport.SitePoolRequest{SiteID: 43, Username: "celik_site43", PHPVersion: "8.3", Operation: transport.SitePoolOperationSave,
		Values: &transport.SitePoolValues{PM: "ondemand"}}
	if result, err := NewPHPPoolManager().ApplySitePool(other); err != nil || result.Outcome != transport.SiteFileOutcomeWritten {
		t.Fatalf("other site's save: %+v %v", result, err)
	}
}

func TestSitePoolSwitchMovesCelikPanelsText(t *testing.T) {
	path, reloads := withSealedPool(t)
	if err := os.MkdirAll(poolDirPath("8.4"), 0o755); err != nil {
		t.Fatal(err)
	}
	request := sitePoolTestRequest(transport.SitePoolOperationSwitch)
	request.NewPHPVersion = "8.4"
	result, err := NewPHPPoolManager().ApplySitePool(request)
	if err != nil || result.Outcome != transport.SiteFileOutcomeWritten || result.Path != poolFilePath("8.4", "site42") || *reloads != 2 {
		t.Fatalf("switch: %+v %v reloads=%d", result, err, *reloads)
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("source remains: %v", err)
	}
	content := readPoolTestFile(t, result.Path)
	if !strings.HasPrefix(content, ManagedHeaderPrefixINI) || !strings.Contains(content, "php8.4-fpm-site42.sock") {
		t.Fatalf("target:\n%s", content)
	}
}

func TestLegacySiteFileTextsAreFrozen(t *testing.T) {
	pool := legacyPoolText("site42", poolIdentity{"celik_site42", "celik_site42", "/var/run/php/php8.3-fpm-site42.sock", "www-data", "www-data", "0660"}, defaultPoolValues())
	if pool != seededPool {
		t.Fatalf("frozen pool text differs from the alpha.8x pool:\n%s", pool)
	}
	units, _ := LegacyAppUnitRenders(AppUnitParams{
		Description: "example.com", User: "http", RequestedUser: "www-data", WorkDir: "/srv/x", Port: 3001,
		PathEnv: "/usr/bin", Command: "node 'app.js'", InstallTarget: "multi-user.target", DropInDir: "/etc/systemd/system/celikapp-7.service.d",
	})()
	want := map[string]string{
		legacyReleaseAlpha82: "# Managed by CelikPanel — do not edit by hand.\n# CelikPanel tarafından yönetilir — elle düzenlemeyin.\n" +
			"[Unit]\nDescription=CelikPanel app: example.com\nAfter=network.target\n\n[Service]\nUser=http\nWorkingDirectory=/srv/x\n" +
			"Environment=PORT=3001\nEnvironment=NODE_ENV=production\nEnvironment=PATH=/usr/bin\nExecStart=/bin/sh -lc 'node '\\''app.js'\\'''\n" +
			"Restart=on-failure\nRestartSec=3\n\n[Install]\nWantedBy=multi-user.target\n",
	}
	want[legacyReleaseAlpha81] = strings.Replace(want[legacyReleaseAlpha82], "User=http", "User=www-data", 1)
	for _, unit := range units {
		if unit.Text != want[unit.Release] {
			t.Errorf("%s unit text:\n%s", unit.Release, unit.Text)
		}
	}
}

func TestSiteFileWorstStateAcrossFiles(t *testing.T) {
	cases := []struct {
		states []string
		want   string
		index  int
	}{
		{[]string{transport.SiteFileManagedUnchanged, transport.SiteFileOwnerEdited}, transport.SiteFileOwnerEdited, 1},
		{[]string{transport.SiteFileOwnerEdited, transport.SiteFileForeign}, transport.SiteFileOwnerEdited, 0},
		{[]string{transport.SiteFileOwnerEdited, transport.SiteFileUnwritable, transport.SiteFileAbsent}, transport.SiteFileUnwritable, 1},
		// A file the Agent did not read is not known-good: it outranks
		// CelikPanel's unchanged text, and every observed problem outranks it.
		{[]string{transport.SiteFileStateUnknown, transport.SiteFileManagedUnchanged}, transport.SiteFileStateUnknown, 0},
		{[]string{transport.SiteFileManagedUnchanged, transport.SiteFileStateUnknown}, transport.SiteFileStateUnknown, 1},
		{[]string{transport.SiteFileManagedUnchanged, transport.SiteFileStateUnknown, transport.SiteFileOwnerEdited}, transport.SiteFileOwnerEdited, 2},
		{[]string{transport.SiteFileStateUnknown, transport.SiteFileUnknownOrigin}, transport.SiteFileUnknownOrigin, 1},
		{[]string{transport.SiteFileStateUnknown, transport.SiteFileAbsent}, transport.SiteFileAbsent, 1},
		{[]string{transport.SiteFileStateUnknown, transport.SiteFileUnwritable}, transport.SiteFileUnwritable, 1},
		{[]string{transport.SiteFileUnreadable, transport.SiteFileStateUnknown}, transport.SiteFileUnreadable, 0},
		{[]string{transport.SiteFileStateUnknown, transport.SiteFileStateUnknown}, transport.SiteFileStateUnknown, 0},
		{nil, "", -1},
	}
	for _, c := range cases {
		got, index := SiteFileWorstState(c.states...)
		if got != c.want || index != c.index {
			t.Errorf("%v = %q %d, want %q %d", c.states, got, index, c.want, c.index)
		}
	}
}

func TestAppUnitBodyNamesTheDropInDirectory(t *testing.T) {
	body := AppUnitBody(AppUnitParams{Description: "a\nb", WorkDir: "/w", Port: 3000, PathEnv: "/bin", Command: "x",
		InstallTarget: "multi-user.target", DropInDir: "/etc/systemd/system/celikapp-3.service.d"})
	if !strings.Contains(body, "/etc/systemd/system/celikapp-3.service.d/*.conf") || strings.Contains(body, "do not edit by hand") {
		t.Fatalf("unit body:\n%s", body)
	}
	if !strings.Contains(body, "Description=CelikPanel app: ab\n") {
		t.Fatal("a newline in the description reached the unit")
	}
	sealed := SealManagedTextWith(ManagedHeaderPrefixHash, body)
	if !bytes.HasPrefix([]byte(sealed), []byte("# celikpanel-render v2 sha256=")) {
		t.Fatal("unit header form")
	}
}

// A failed pool save says whether the previous file is back (2026-10-11): the
// put-back and the reload after it held ("confirmed"), or the reload after it
// failed too ("unconfirmed", with the Agent's "rollback incomplete").
func TestSitePoolFailedSaveSaysWhetherTheRestoreHeld(t *testing.T) {
	for _, c := range []struct {
		name           string
		secondReload   error
		want           string
		wantIncomplete bool
	}{
		{"restore confirmed", nil, transport.SiteFileRestoreConfirmed, false},
		{"restore unconfirmed", errors.New("reload after restore failed"), transport.SiteFileRestoreUnconfirmed, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			path, _ := withSealedPool(t)
			before := readPoolTestFile(t, path)
			calls := 0
			reloadPHPFPM = func(string) error {
				calls++
				if calls == 1 {
					return errors.New("reload failed")
				}
				return c.secondReload
			}
			save := sitePoolTestRequest(transport.SitePoolOperationSave)
			save.Values = &transport.SitePoolValues{PM: "ondemand", PMMaxChildren: 9}
			result, err := NewPHPPoolManager().ApplySitePool(save)
			if result.Outcome != transport.SiteFileOutcomeFailed || result.Restore != c.want {
				t.Fatalf("result: %+v %v", result, err)
			}
			if incomplete := err != nil && strings.Contains(err.Error(), "rollback incomplete"); incomplete != c.wantIncomplete {
				t.Fatalf("error: %v", err)
			}
			if readPoolTestFile(t, path) != before {
				t.Fatal("the previous bytes were not put back")
			}
		})
	}
}
