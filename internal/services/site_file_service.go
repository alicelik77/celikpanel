package services

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// What a site file's service does after CelikPanel put the previous file back
// (set12 cell RP1, 2026-10-11). The take on a pool was refused by `php-fpm -t`
// because of an owner's file elsewhere in the pool directory; the owner's pool
// was put back and PHP-FPM was then reloaded with a configuration its own
// check had just refused, which left it failed. Now the service's own check
// runs on the put-back configuration first; when it refuses it, the service is
// not reloaded and keeps the configuration it loaded before. A service that
// was not running before the operation is neither reloaded nor started.
//
// Önceki dosya geri konduktan sonra hizmetin kendi denetimi önce çalışır;
// reddederse hizmet yeniden yüklenmez ve önceden yüklediği yapılandırmayla
// çalışmaya devam eder. Önceden çalışmayan hizmet ne yüklenir ne başlatılır.

// ErrServiceCheckUnavailable is a check that could not be run on this host
// (systemd-analyze absent): the service is not reloaded and the answer says so.
var ErrServiceCheckUnavailable = errors.New("the service's configuration check is not available on this server")

// systemdUnitActiveTimeout bounds one `systemctl is-active`.
const systemdUnitActiveTimeout = 5 * time.Second

// systemdUnitActive answers `systemctl is-active <unit>` as "true", "false" or
// "unknown". A test seam.
var systemdUnitActive = func(args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), systemdUnitActiveTimeout)
	defer cancel()
	out, _ := exec.CommandContext(ctx, "systemctl", append([]string{"is-active"}, args...)...).Output()
	return SystemdActiveAnswer(string(out))
}

// SystemdActiveAnswer reads the first line `systemctl is-active` printed.
func SystemdActiveAnswer(output string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(output), "\n")
	switch strings.TrimSpace(line) {
	case "active", "reloading", "activating", "refreshing":
		return transport.SiteFileServiceActiveTrue
	case "inactive", "failed", "deactivating":
		return transport.SiteFileServiceActiveFalse
	}
	return transport.SiteFileServiceActiveUnknown
}

// SystemdUnitActive is systemdUnitActive for callers outside this package.
func SystemdUnitActive(args ...string) string { return systemdUnitActive(args...) }

// phpFPMActive is whether the PHP-FPM of a version runs. A test seam.
var phpFPMActive = func(version string) string { return systemdUnitActive(phpLayoutFor(version).unit()) }

// SiteFileCheckLine is the line of a check's error that names the cause:
// nginx's [emerg] line, PHP-FPM's first "ERROR:" line, else the first line;
// bounded.
func SiteFileCheckLine(err error) string {
	if err == nil {
		return ""
	}
	var refused *NginxConfigRefusedError
	if errors.As(err, &refused) {
		return boundedSiteFileDetail(refused.FirstLine())
	}
	first := ""
	for _, line := range strings.Split(err.Error(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.Contains(line, "ERROR:") || strings.Contains(line, "[emerg]") {
			return boundedSiteFileDetail(line)
		}
		if first == "" {
			first = line
		}
	}
	return boundedSiteFileDetail(first)
}
