package services

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// set11 cell B4 (2026-10-11): a "take CelikPanel's" that the server refuses
// leaves no dated copy beside the owner's file — neither when the protection
// is measured before the copy, nor when it appears between the measurement
// and the replacement.

func directoryNames(t *testing.T, directory string) string {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return strings.Join(names, ",")
}

func protectOnly(t *testing.T, locked string) {
	t.Helper()
	siteFileProtectionOf = func(path string) (string, string) {
		if path == locked {
			return transport.SiteFileProtectionImmutable, "EPERM"
		}
		return transport.SiteFileProtectionNone, ""
	}
	t.Cleanup(func() { siteFileProtectionOf = measureSiteFileProtection })
}

func refuseRenameTo(t *testing.T, locked string) {
	t.Helper()
	siteFileRename = func(from, to string) error {
		if to == locked {
			return &os.LinkError{Op: "rename", Old: from, New: to, Err: syscall.EPERM}
		}
		return os.Rename(from, to)
	}
	t.Cleanup(func() { siteFileRename = os.Rename })
}

func TestARefusedVhostTakeCreatesNoFile(t *testing.T) {
	for _, when := range []string{"measured before the copy", "set after the measurement"} {
		t.Run(when, func(t *testing.T) {
			h := newManagedVhostHarness(t)
			data := managedTestData("take-locked.example", managedVhostPlatforms[0].socket)
			item := h.item(data, transport.SiteFileTriggerTake)
			path := SiteVhostPath(data.Domain)
			owner := "server { listen 80; server_name take-locked.example; }\n"
			writeTestFile(t, path, owner, 0o644)
			item.ExpectedFileSHA256, item.ExpectedRenderSHA256 = digestOf(owner), digestOf(item.Body)
			refuseRenameTo(t, path)
			if when == "measured before the copy" {
				protectOnly(t, path)
			} else {
				protectOnly(t, "")
			}
			before := directoryNames(t, filepath.Dir(path))
			result := h.apply(item)[0]
			if result.Outcome != transport.SiteFileOutcomeRefused || result.Reason != transport.SiteFileReasonWriteRefused ||
				result.BackupPath != "" {
				t.Fatalf("refused take: %+v", result)
			}
			if when == "measured before the copy" && (result.Protection != transport.SiteFileProtectionImmutable ||
				result.SystemError != "EPERM" || !strings.Contains(result.Detail, "not attempted")) {
				t.Fatalf("protection not named: %+v", result)
			}
			if after := directoryNames(t, filepath.Dir(path)); after != before {
				t.Fatalf("files before %q, after %q", before, after)
			}
			if readTestFile(t, path) != owner {
				t.Fatal("the owner's file changed")
			}
		})
	}
}

func TestARefusedPoolTakeLeavesNoDatedCopy(t *testing.T) {
	for _, when := range []string{"measured before the copy", "set after the measurement"} {
		t.Run(when, func(t *testing.T) {
			path, reloads := withSealedPool(t)
			owner := readPoolTestFile(t, path) + "php_flag[display_errors] = on\n"
			if err := os.WriteFile(path, []byte(owner), 0o644); err != nil {
				t.Fatal(err)
			}
			manager := NewPHPPoolManager()
			shown, _, _, err := manager.InspectSitePool(sitePoolTestRequest(transport.SitePoolOperationInspect))
			if err != nil {
				t.Fatal(err)
			}
			refuseRenameTo(t, path)
			if when == "measured before the copy" {
				protectOnly(t, path)
			} else {
				protectOnly(t, "")
			}
			take := sitePoolTestRequest(transport.SitePoolOperationTake)
			take.ExpectedFileSHA256, take.ExpectedRenderSHA256 = shown.FileSHA256, shown.RenderSHA256
			result, _ := manager.ApplySitePool(take)
			if result.Outcome != transport.SiteFileOutcomeRefused || result.Reason != transport.SiteFileReasonWriteRefused ||
				result.BackupPath != "" || *reloads != 0 {
				t.Fatalf("refused take: %+v reloads=%d", result, *reloads)
			}
			if names := directoryNames(t, filepath.Dir(path)); strings.Contains(names, SiteFileBackupMarker) {
				t.Fatalf("a dated copy was left: %s", names)
			}
			if readPoolTestFile(t, path) != owner {
				t.Fatal("the owner's file changed")
			}
		})
	}
}
