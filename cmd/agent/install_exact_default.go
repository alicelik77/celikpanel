package main

import (
	"bufio"
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
)

// With a vendor repository enabled, an install names the exact version
// (alpha.107, D-038 §1 and §7). The distribution's php-fpm and postgresql
// are "default version" packages: they depend on whichever version is the
// default today (php8.4-fpm, postgresql-17). With PGDG enabled, a later
// manual `apt upgrade` could bring a new PostgreSQL major beside the data
// (Debian opens an empty second cluster on port 5433); with Sury, a new
// default PHP line. So when the component's vendor repository is enabled, a
// plain install asks the package manager which exact package the
// distribution's own default package brings and installs that one instead
// (its record whose version is not the vendor's). Without a vendor
// repository nothing changes: the distribution keeps one version per
// release, and its default package is installed as before. A default
// package already installed is reported on the repository card, never
// removed.
//
// Üretici deposu açıkken kurulum tam sürümü adlandırır: dağıtımın varsayılan
// paketinin bugün getirdiği tam paket kurulur. Depo yoksa hiçbir şey değişmez.

const exactDefaultBound = 20 * time.Second

// exactDefaultOps is the narrow seam of the resolution.
type exactDefaultOps struct {
	repoEnabled func(repoID string) bool
	show        func(ctx context.Context, pkg string) (string, error)
	vendorOf    func(name, version string) string
}

// exactDefaultPackage returns the exact package to install in place of the
// component's distribution default package, or "" when nothing changes
// (no optional vendor repository, or it is not enabled).
func exactDefaultPackage(ctx context.Context, svc *core.ManagedService, family string, ops exactDefaultOps) (string, error) {
	if svc == nil || svc.Repo == nil || svc.Repo.Required || family != "apt" || strings.TrimSpace(svc.Repo.PackagePattern) == "" {
		return "", nil
	}
	defaults := svc.Packages["apt"]
	if len(defaults) != 1 || ops.repoEnabled == nil || !ops.repoEnabled(svc.Repo.ID) {
		return "", nil
	}
	versioned, err := regexp.Compile(svc.Repo.PackagePattern)
	if err != nil {
		return "", fmt.Errorf("invalid package pattern in catalogue")
	}
	showCtx, cancel := context.WithTimeout(ctx, exactDefaultBound)
	defer cancel()
	out, err := ops.show(showCtx, defaults[0])
	if err != nil {
		return "", fmt.Errorf("the package manager could not show what %s brings, so %s is not installed (the %s repository is enabled, so CelikPanel installs the exact version, never the moving default package): %w",
			defaults[0], svc.Name, svc.Repo.Name, err)
	}
	exact := distributionDefaultTarget(out, versioned, func(version string) bool {
		return ops.vendorOf != nil && ops.vendorOf(defaults[0], version) != ""
	})
	if exact == "" {
		return "", fmt.Errorf("could not tell which %s version the distribution's %s brings; with the %s repository enabled, choose the version to install",
			svc.Name, defaults[0], svc.Repo.Name)
	}
	return exact, nil
}

// distributionDefaultTarget reads `apt-cache show` stanzas and returns the
// first versioned dependency of the first record that is not the vendor's.
func distributionDefaultTarget(show string, versioned *regexp.Regexp, vendor func(version string) bool) string {
	scanner := bufio.NewScanner(strings.NewReader(show))
	scanner.Buffer(make([]byte, 64<<10), 4<<20)
	var version, depends string
	pick := func() string {
		if version == "" || vendor(version) {
			return ""
		}
		for _, group := range strings.Split(depends, ",") {
			for _, alternative := range strings.Split(group, "|") {
				name := strings.TrimSpace(alternative)
				if i := strings.IndexAny(name, " (:"); i >= 0 {
					name = name[:i]
				}
				if name != "" && versioned.FindString(name) == name {
					return name
				}
			}
		}
		return ""
	}
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case line == "":
			if found := pick(); found != "" {
				return found
			}
			version, depends = "", ""
		case strings.HasPrefix(line, "Version: "):
			version = strings.TrimSpace(strings.TrimPrefix(line, "Version: "))
		case strings.HasPrefix(line, "Depends: "):
			depends = strings.TrimSpace(strings.TrimPrefix(line, "Depends: "))
		}
	}
	return pick()
}

func hostExactDefaultOps(aptCache string) exactDefaultOps {
	return exactDefaultOps{
		repoEnabled: func(repoID string) bool {
			_, ok := enabledRepoLocations()[repoID]
			return ok
		},
		show: func(ctx context.Context, pkg string) (string, error) {
			if !validPackageName(pkg) {
				return "", fmt.Errorf("invalid package name")
			}
			// apt-cache show prints every version's record.
			out, err := runPackageSimulationCommand(ctx, packageCommandEnv(), aptCache, "show", pkg)
			return string(out), err
		},
		vendorOf: hostVendorVersionLookup(),
	}
}
