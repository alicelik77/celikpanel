package main

import (
	"archive/tar"
	"errors"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/backupspec"
	"github.com/alicelik/celikpanel/internal/hostingpath"
)

// Site-home areas of a backup (docs/SITE-BACKUP-CONTENTS-DESIGN.md). A files
// or full backup holds public_html as before and, in a version-3 package, the
// areas that exist in the site home: "app" (a program or Python app) and
// "apps/<app>" (each service app). The Agent finds them itself when the backup
// is made, so scheduled, safety and pre-update backups hold the same content.
// Each area leaves out only what its rules name below, and every left-out
// item is recorded with its reason.
//
// Site ev dizini alanları: "app" (program ya da Python uygulaması) ve
// "apps/<uygulama>" (hizmet uygulamaları). Dışarıda bırakılan her şey
// gerekçesiyle kaydedilir.

// backupSiteHome is the site home of a scope (a test seam).
var backupSiteHome = hostingpath.SiteHome

// homeAreaApps is the folder under the site home that holds service apps.
const homeAreaApps = "apps"

// serviceAppAreaName is the name of one folder under apps/.
var serviceAppAreaName = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{0,63}$`)

// validAreaName reports whether name is an area a package may hold.
func validAreaName(name string) bool {
	if name == backupspec.AreaApp {
		return true
	}
	rest, ok := strings.CutPrefix(name, backupspec.AreaAppsPrefix)
	return ok && serviceAppAreaName.MatchString(rest) && rest != "." && rest != ".."
}

// restorableArea reports whether this release puts an area back. Service
// apps' areas are backed up but not restored (named in the answer).
func restorableArea(name string) bool { return name == backupspec.AreaApp }

// areaExclusion is the reason a top-level entry of an area is left out by
// rule, or "". Only names directly inside the area folder are matched.
func areaExclusion(area, name string, directory bool) string {
	if area == backupspec.AreaApp {
		switch {
		case name == ".pip-cache" && directory:
			return backupspec.LeftOutCache
		case name == ".build-tmp" && directory:
			return backupspec.LeftOutBuildTemp
		case strings.HasPrefix(name, "venv-") && strings.HasSuffix(name, ".partial") && directory:
			return backupspec.LeftOutBuildTemp
		}
		return ""
	}
	if strings.HasPrefix(area, backupspec.AreaAppsPrefix) {
		switch {
		case name == ".cache" && directory:
			return backupspec.LeftOutCache
		case (name == "dist" || strings.HasPrefix(name, "app-")) && directory:
			return backupspec.LeftOutReinstalled
		}
	}
	return ""
}

func validLeftOutReason(reason string) bool {
	switch reason {
	case backupspec.LeftOutCache, backupspec.LeftOutBuildTemp, backupspec.LeftOutReinstalled,
		backupspec.LeftOutSpecial, backupspec.LeftOutNotSiteOwned, backupspec.LeftOutNotAFolder:
		return true
	}
	return false
}

var errHomeWalkTimedOut = errors.New("the measurement reached its time limit")

// homeWalk is one walk of a site home (backup_home_linux.go): it measures
// (tw nil) or archives, with the same rules.
type homeWalk struct {
	tw       *tar.Writer
	siteUID  uint32
	deadline time.Time
	entries  int
	areas    []backupspec.Area
	left     leftOutList
	current  *backupspec.Area
}

// leftOutList collects left-out items: the first MaxLeftOutNamed by name,
// the rest counted; the bytes of all of them summed.
type leftOutList struct {
	items []backupspec.LeftOut
	more  int
	bytes int64
}

func (l *leftOutList) add(item backupspec.LeftOut) {
	l.bytes += item.Bytes
	if len(l.items) >= backupspec.MaxLeftOutNamed {
		l.more++
		return
	}
	l.items = append(l.items, item)
}

// validateManifestHome checks a version-3 manifest's home part: the payload,
// the areas (valid, distinct, sorted) and the left-out items (each inside an
// area, with a known reason, bounded).
func validateManifestHome(m backupManifest) error {
	if m.Home == nil {
		return errors.New("version 3 manifest without site-home payload")
	}
	if err := validateManifestPayload(*m.Home); err != nil {
		return err
	}
	if m.Home.Name != homePayloadName {
		return errors.New("unexpected site-home payload")
	}
	if m.Type != backupspec.TypeFiles && m.Type != backupspec.TypeFull {
		return errors.New("site-home payload in a database backup")
	}
	if len(m.Areas) == 0 && len(m.LeftOut) == 0 {
		// A package with no area exists only to name an area folder that
		// could not be taken (a link, another account's folder).
		return errors.New("site-home payload without areas")
	}
	names := make([]string, 0, len(m.Areas))
	for _, area := range m.Areas {
		if !validAreaName(area.Name) || area.Bytes < 0 || area.Files < 0 {
			return errors.New("invalid site-home area")
		}
		names = append(names, area.Name)
	}
	if !sort.StringsAreSorted(names) {
		return errors.New("site-home areas are not in order")
	}
	for i := 1; i < len(names); i++ {
		if names[i] == names[i-1] {
			return errors.New("duplicate site-home area")
		}
	}
	if len(m.LeftOut) > backupspec.MaxLeftOutNamed || m.LeftOutMore < 0 {
		return errors.New("too many left-out items")
	}
	for _, item := range m.LeftOut {
		if !validLeftOutReason(item.Reason) || item.Bytes < 0 || !validPackageEntryName(item.Path) {
			return errors.New("invalid left-out item")
		}
		if homeLevelEntry(item.Path) && (item.Reason == backupspec.LeftOutNotAFolder || item.Reason == backupspec.LeftOutNotSiteOwned) {
			// An area folder that could not be taken as an area.
			continue
		}
		if !insideAnyArea(item.Path, names) {
			return errors.New("left-out item outside the site-home areas")
		}
	}
	return nil
}

// homeLevelEntry reports whether p names an area folder or apps/ itself.
func homeLevelEntry(p string) bool {
	return p == backupspec.AreaApp || p == homeAreaApps || path.Dir(p) == homeAreaApps
}

func insideAnyArea(itemPath string, areas []string) bool {
	for _, area := range areas {
		if strings.HasPrefix(itemPath, area+"/") {
			return true
		}
	}
	return false
}
