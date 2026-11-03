package core

import (
	"fmt"
	"regexp"
	"strings"
)

// InstallRequiresManagedRepository reports whether this exact install choice
// depends on the service's managed repository. A repository can be optional
// for the distro default package while still being mandatory for a selected
// vendor version such as postgresql-18.
func InstallRequiresManagedRepository(service *ManagedService, selectedPackage string) (bool, error) {
	if service == nil || service.Repo == nil {
		return false, nil
	}
	if service.Repo.Required {
		return true, nil
	}
	selectedPackage = strings.TrimSpace(selectedPackage)
	pattern := strings.TrimSpace(service.Repo.PackagePattern)
	if selectedPackage == "" || pattern == "" {
		return false, nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return false, fmt.Errorf("managed repository %q has an invalid package pattern: %w", service.Repo.ID, err)
	}
	match := re.FindString(selectedPackage)
	return match == selectedPackage, nil
}

// distributionPHPLines is the PHP version each supported Debian and Ubuntu
// release ships in its own archive (with its security support). Arch Linux
// ships one moving PHP and has no versioned packages.
var distributionPHPLines = map[string]string{
	"debian 12":    "8.2",
	"debian 13":    "8.4",
	"ubuntu 22.04": "8.1",
	"ubuntu 24.04": "8.3",
}

// DistributionPHPLine is the PHP major.minor the distribution release itself
// ships ("" when this release is not one CelikPanel knows).
// DistributionPHPLine dağıtımın kendi arşivindeki PHP sürümüdür.
func DistributionPHPLine(distroID, versionID string) string {
	return distributionPHPLines[strings.ToLower(strings.TrimSpace(distroID))+" "+strings.TrimSpace(versionID)]
}

var phpFPMVersionPackage = regexp.MustCompile(`^php([0-9]+\.[0-9]+)-fpm$`)

// DistributionShipsSelectedPackage: the version a PHP-FPM install picks is
// the distribution's own (php8.4-fpm on Debian 13), so it comes from the
// distribution's archive with its companions and needs no vendor repository
// (alpha.118 lab: Components asked for Sury to install Debian 13's own PHP
// 8.4 extensions). Any other version still needs the vendor repository.
// Seçilen PHP sürümü dağıtımın kendisininse üçüncü taraf depo gerekmez.
func DistributionShipsSelectedPackage(service *ManagedService, distroID, versionID, selectedPackage string) bool {
	if service == nil || service.ID != "php-fpm" {
		return false
	}
	match := phpFPMVersionPackage.FindStringSubmatch(strings.TrimSpace(selectedPackage))
	if match == nil {
		return false
	}
	line := DistributionPHPLine(distroID, versionID)
	return line != "" && line == match[1]
}
