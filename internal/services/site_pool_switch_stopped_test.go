package services

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// D-043: a site moves off a PHP version stopped (and masked) in a security
// emergency. The stopped version is not reloaded (systemd would refuse a
// masked unit); the target version is reloaded as always.
func TestSitePoolSwitchOffAStoppedVersionDoesNotReloadIt(t *testing.T) {
	path, _ := withSealedPool(t)
	if err := os.MkdirAll(poolDirPath("8.4"), 0o755); err != nil {
		t.Fatal(err)
	}
	reloaded := []string{}
	reloadPHPFPM = func(version string) error {
		if version == "8.3" {
			return errors.New("Unit php8.3-fpm.service is masked")
		}
		reloaded = append(reloaded, version)
		return nil
	}
	phpFPMActive = func(version string) string {
		if version == "8.3" {
			return transport.SiteFileServiceActiveFalse
		}
		return transport.SiteFileServiceActiveTrue
	}
	request := sitePoolTestRequest(transport.SitePoolOperationSwitch)
	request.NewPHPVersion = "8.4"
	result, err := NewPHPPoolManager().ApplySitePool(request)
	if err != nil || !SiteFileApplied(result.Outcome) {
		t.Fatalf("switch off a stopped version: %+v %v", result, err)
	}
	if strings.Join(reloaded, ",") != "8.4" {
		t.Fatalf("reloads = %v, want only 8.4", reloaded)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the source pool is still there: %v", err)
	}
	if _, err := os.Stat(poolFilePath("8.4", "site42")); err != nil {
		t.Fatalf("the target pool was not written: %v", err)
	}
}
