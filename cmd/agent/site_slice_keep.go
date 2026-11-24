package main

import (
	"os"
	"path/filepath"

	"github.com/alicelik/celikpanel/internal/sitesandbox"
)

// Going back one release from the release that places every site's programs
// in their site's slice (S4 R2a, internal/sitesandbox/slice.go): this release
// does not make or move slices, but when it writes such a program's unit
// again it keeps the unit's Slice=celiksite_<site id>.slice line, so the
// program does not leave its site's limits at its next restart without
// anyone saying so (D-025). Nothing else of the slice is read or changed.

// unitKeepsSiteSlice reports whether a unit file's text places the unit in
// its own site's slice (its Slice= line is celiksite_<siteID>.slice) while
// that slice's file is a regular file in unitDir. Anything else - no such
// line, another site's slice, the slice's file missing - is the system slice,
// as every unit of this release.
func unitKeepsSiteSlice(text string, siteID int, unitDir string) bool {
	if siteID <= 0 {
		return false
	}
	id, ok := sitesandbox.ParseSliceName(sitesandbox.UnitFileSlice(text))
	if !ok || id != siteID {
		return false
	}
	fi, err := os.Lstat(filepath.Join(unitDir, sitesandbox.SliceName(siteID)))
	return err == nil && fi.Mode().IsRegular()
}

// unitFileKeepsSiteSlice is unitKeepsSiteSlice for the unit file at path (a
// file that is missing, not a regular file or unreadable keeps nothing).
func unitFileKeepsSiteSlice(path string, siteID int) bool {
	fi, err := os.Lstat(path)
	if err != nil || !fi.Mode().IsRegular() || fi.Size() > 1<<20 {
		return false
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return unitKeepsSiteSlice(string(raw), siteID, filepath.Dir(path))
}
