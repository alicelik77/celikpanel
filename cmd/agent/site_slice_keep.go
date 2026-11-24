package main

import (
	"os"
	"path/filepath"

	"github.com/alicelik/celikpanel/internal/sitesandbox"
	"github.com/alicelik/celikpanel/internal/transport"
)

// A unit's Slice= line when it is written again (alpha.147 carried this for
// going back from per-site limits; S4 R2a keeps it as its fallback). With the
// site's limits in the request this release decides where the unit runs
// (siteUnitEntersSlice: into its slice when it does not run or already runs
// there, the administrator's move otherwise). Without them - a request whose
// limits are out of range, a host that cannot hold limits - a unit file that
// already names its own site's slice keeps that line while the slice's file
// exists, so a program never leaves its site's slice at its next restart
// without anyone saying so (D-025).

// siteUnitInSlice is the InSlice of a unit written now: siteUnitEntersSlice
// when the request carries the site's limits (s from siteSliceFor), else
// keep (unitKeepsSiteSlice / unitFileKeepsSiteSlice of the file written over).
//
// With the limits too, keep holds (alpha.148 fix round, review finding 1): a
// reading that cannot be made never moves a program OUT of its limits.
// siteUnitEntersSlice answers false when systemd does not answer, or a
// running unit shows an empty or unrecognised ControlGroup; the file written
// over may already name the site's slice (its file present), and writing
// Slice=system.slice then would take the program out of its limits at its
// next restart with no administrator acting and nothing shown before. Keeping
// the line the file already names changes nothing about where the program
// starts next.
func siteUnitInSlice(unit string, s *transport.SiteSlice, move bool, keep func() bool) bool {
	if s != nil {
		return siteUnitEntersSlice(unit, s, move) || (keep != nil && keep())
	}
	return keep != nil && keep()
}

// unitKeepsSiteSlice reports whether a unit file's text places the unit in
// its own site's slice (its Slice= line is celiksite_<siteID>.slice) while
// that slice's file is a regular file in unitDir. Anything else - no such
// line, another site's slice, the slice's file missing - is the system slice.
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
