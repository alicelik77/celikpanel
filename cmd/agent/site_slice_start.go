package main

import (
	"log"
	"os"

	"github.com/alicelik/celikpanel/internal/serviceapps"
	"github.com/alicelik/celikpanel/internal/sitesandbox"
	"github.com/alicelik/celikpanel/internal/transport"
)

// A program started from the panel while it does not run starts inside its
// site's limits (alpha.149, lab O2; docs/SITE-SANDBOX.md section 8).
//
// Before alpha.149 a Start of a stopped program (an uploaded program, a
// Node.js site, a service app) only restarted its unit, so a program stopped
// before the update kept Slice=system.slice until a deploy wrote its unit.
// Now the Start writes the unit like any write while the program does not
// run: CelikPanel's own unchanged text gets Slice=celiksite_<id>.slice (the
// move's own rewrite, siteUnitMovedBody), the site's slice file is put in
// place first, and systemd reloads; the Start's own restart then starts it
// inside. The rule of the lane stays: a RUNNING program is never moved here
// (only the administrator's move restarts a running one into its limits);
// nor is a unit whose file the server owner changed, one an owner's drop-in
// places, one of an earlier protection, one a move's record names (its
// put-back decides its place), or anything when where it runs cannot be
// read. The caller holds the program's own mutation lock.
//
// Durdurulmuş bir program panelden başlatılınca sitenin sınırlarının içinde
// başlar; çalışan bir programa yalnız yöneticinin taşıması dokunur.
func siteUnitIntoSliceBeforeStart(unit string, slice *transport.SiteSlice) bool {
	if slice == nil || siteSliceHostSupport() != "" {
		return false
	}
	s := siteSliceFor(slice, slice.SiteID)
	if s == nil {
		return false
	}
	if journal, err := readSiteMoveJournal(); err == nil {
		for _, u := range journal.Units {
			if u.Unit == unit {
				return false
			}
		}
	} else if !os.IsNotExist(err) {
		return false
	}
	text, file, mode, err := siteUnitText(unit)
	if err != nil {
		return false
	}
	body, ours := siteUnitSealedBody(unit, text)
	if !ours || serviceapps.UnitGeneration(text) < sitesandbox.Generation {
		return false
	}
	out, err := appUnitSystemctl(systemctlArgs("show", unit, "--property=ActiveState,SubState,ControlGroup,DropInPaths")...)
	if err != nil {
		return false
	}
	props := parseShow(string(out))
	state := props["ActiveState"]
	if state == "" || (siteUnitRunningStates[state] && !(state == "activating" && siteUnitAutoRestart(props))) {
		return false
	}
	if siteUnitDropInSlice(props["DropInPaths"]) != "" {
		return false
	}
	moved, already, err := siteUnitMovedBody(body, s.SiteID)
	if err != nil || already {
		return false
	}
	ensureSiteSlice(s)
	sliceFile, err := siteSliceFile(s.SiteID)
	if err != nil {
		return false
	}
	if _, err := os.Stat(sliceFile); err != nil {
		// Without its file systemd would make the slice without limits: the
		// program starts where it started before.
		return false
	}
	if err := writeUnitBytes(file, siteUnitSeal(unit, moved), mode); err != nil {
		log.Printf("site slice: %s was not written into %s before its start: %v", unit, sitesandbox.SliceName(s.SiteID), err)
		return false
	}
	if err := siteSliceDaemonReload(); err != nil {
		// The file names the slice now; the next reload or start applies it.
		log.Printf("site slice: %s was written into %s before its start, but %v", unit, sitesandbox.SliceName(s.SiteID), err)
		return true
	}
	log.Printf("site slice: %s did not run; it starts inside %s", unit, sitesandbox.SliceName(s.SiteID))
	return true
}
