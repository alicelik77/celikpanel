package sitesandbox

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// A site's slice, as the later release that introduces per-site limits
// writes it (S4 R2a, docs/SITE-SANDBOX.md): every long-running program of one
// site runs in celiksite_<site id>.slice, a native slice unit file next to
// its units. This release does not make, write or move slices. It only
// accepts a unit that a later release placed there, so that going back one
// release keeps such a program starting with the SMTP block, and keeps the
// unit's Slice= line when it writes the unit again (forward compatibility,
// D-025).
//
// The name has no "-": systemd reads a dash in a slice's name as nesting
// (celiksite-7.slice would be a child of celiksite.slice), so the slice sits
// directly below the root and a unit in it is /celiksite_<id>.slice/<unit>.
// That is depth two, as /system.slice/<unit> is, so the SMTP block's set
// (typeof socket cgroupv2 level 2) matches a unit in either place without any
// change to the table.
//
// Sitenin dilimi: sonraki sürüm bir sitenin programlarını
// celiksite_<site>.slice içinde çalıştırır. Bu sürüm dilim yazmaz; yalnız bir
// önceki sürüme dönüldüğünde o programların posta engeliyle başlamasını ve
// birim dosyasındaki Slice= satırının korunmasını sağlar.

// SystemSlice is where a unit not moved into its site's slice runs.
const SystemSlice = "system.slice"

// maxSiteID bounds a site identity in a unit name.
const maxSiteID = 1<<31 - 1

var slicePattern = regexp.MustCompile(`^celiksite_([1-9][0-9]{0,9})\.slice$`)

// SliceName is a site's slice: celiksite_<site id>.slice.
func SliceName(siteID int) string { return "celiksite_" + strconv.Itoa(siteID) + ".slice" }

// ParseSliceName reads a site's slice name back to its site identity.
func ParseSliceName(name string) (int, bool) {
	m := slicePattern.FindStringSubmatch(name)
	if m == nil {
		return 0, false
	}
	id, err := strconv.Atoi(m[1])
	if err != nil || id <= 0 || id > maxSiteID {
		return 0, false
	}
	return id, true
}

// AllowedControlGroups are the cgroups a unit of the site may run in: its
// site's slice, or the system slice where it ran before it was moved. Both
// are depth two.
func AllowedControlGroups(siteID int, unit string) []string {
	out := []string{"/" + SystemSlice + "/" + unit}
	if siteID > 0 && siteID <= maxSiteID {
		out = append(out, "/"+SliceName(siteID)+"/"+unit)
	}
	return out
}

// UnitControlGroupAllowed reports whether cgroup is /system.slice/<unit> or
// /celiksite_<id>.slice/<unit> for a sandboxed unit name, and returns the
// site identity of the slice (0 for the system slice).
func UnitControlGroupAllowed(cgroup, unit string) (int, bool) {
	if !ValidUnitName(unit) {
		return 0, false
	}
	if cgroup == "/"+SystemSlice+"/"+unit {
		return 0, true
	}
	rest, ok := strings.CutPrefix(cgroup, "/")
	if !ok {
		return 0, false
	}
	slice, leaf, ok := strings.Cut(rest, "/")
	if !ok || leaf != unit {
		return 0, false
	}
	return ParseSliceName(slice)
}

// CgroupElementFor is the SMTP set's element for a unit running in cgroup
// (the unit's ControlGroup= as systemd shows it): '{ "<slice>/<unit>" }'. A
// cgroup that is not the system slice or a site's slice is refused: the set
// matches depth two only, so a unit anywhere else would not be blocked.
func CgroupElementFor(cgroup, unit string) (string, error) {
	if _, ok := UnitControlGroupAllowed(cgroup, unit); !ok {
		return "", fmt.Errorf("%s runs in %q, not in the system slice or a site's slice", unit, cgroup)
	}
	return `{ "` + strings.TrimPrefix(cgroup, "/") + `" }`, nil
}

// UnitFileSlice reads the slice a unit file's text names (its last Slice=
// line, as systemd reads it; "" when it names none).
func UnitFileSlice(text string) string {
	slice := ""
	for _, line := range strings.Split(text, "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "Slice="); ok {
			slice = strings.TrimSpace(rest)
		}
	}
	return slice
}
