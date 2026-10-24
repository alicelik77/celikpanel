package main

// A version only the vendor repository ships needs some of the vendor's own
// shared packages (alpha.107, found by the real-system run): Sury's
// php8.3-common Breaks the distribution's php-common, and PGDG's
// postgresql-18 needs PGDG's libpq5. Under the confinement apt would not pick
// those, so the install of a vendor-only version names them as
// name=version, as part of the request, and they appear in the install's
// package list. After an install the confinement is refreshed: what is now
// installed from the vendor keeps following it, everything else stays with
// the distribution.
//
// Yalnız üreticinin sunduğu bir sürüm, üreticinin ortak paketlerini de
// gerektirir; kurulum bunları adıyla ister, sonra sınırlama tazelenir.

import (
	"sort"
	"strings"
	"unicode"

	"github.com/alicelik/celikpanel/internal/core"
)

// validInstallArg is a package name, or name=version for one exact version.
func validInstallArg(arg string) bool {
	name, version, exact := strings.Cut(arg, "=")
	return validPackageName(name) && (!exact || validDebianVersion(version))
}

// installArgName is the package an install argument names.
func installArgName(arg string) string {
	name, _, _ := strings.Cut(arg, "=")
	return name
}

// vendorSharedPackageArgsFrom: for each of the repository's shared packages
// the vendor offers and whose installed version is not the vendor's, the
// vendor's newest version as name=version.
func vendorSharedPackageArgsFrom(repo *core.ManagedRepo, installed map[string][]string, index map[string]map[string]bool) []string {
	if repo == nil {
		return nil
	}
	var args []string
	for _, name := range repo.SharedPackages {
		offered := index[name]
		if len(offered) == 0 {
			continue
		}
		fromVendor := false
		for _, version := range installed[name] {
			if offered[version] {
				fromVendor = true
			}
		}
		if fromVendor {
			continue
		}
		versions := make([]string, 0, len(offered))
		for version := range offered {
			if validDebianVersion(version) {
				versions = append(versions, version)
			}
		}
		if len(versions) == 0 {
			continue
		}
		sort.Slice(versions, func(i, j int) bool { return debVersionCompare(versions[i], versions[j]) > 0 })
		args = append(args, name+"="+versions[0])
	}
	return args
}

// hostVendorSharedPackageArgs reads the repository's index and the installed
// packages; an unreadable one adds nothing (the simulation then says why).
func hostVendorSharedPackageArgs(repo *core.ManagedRepo) []string {
	if repo == nil || len(repo.SharedPackages) == 0 {
		return nil
	}
	loc, ok := enabledRepoLocations()[repo.ID]
	if !ok {
		return nil
	}
	index, err := readVendorPackageIndex(loc)
	if err != nil {
		return nil
	}
	installed, err := readDpkgStatusVersions(dpkgStatusPath)
	if err != nil {
		return nil
	}
	return vendorSharedPackageArgsFrom(repo, installed, index)
}

// refreshRepoConfinement rewrites CelikPanel's own, unchanged preferences
// file when the packages installed from the repository changed. An edited
// file, the owner's rule or a missing one is left as it is.
func refreshRepoConfinement(repo *core.ManagedRepo) {
	if repo == nil {
		return
	}
	packageOperationMu.Lock()
	defer packageOperationMu.Unlock()
	loc, ok := enabledRepoLocations()[repo.ID]
	if !ok || repoConfinementOf(repo, loc).State != repoConfinementConfined {
		return
	}
	index, err := readVendorPackageIndex(loc)
	if err != nil {
		return
	}
	installed, err := readDpkgStatusVersions(dpkgStatusPath)
	if err != nil {
		return
	}
	kept := repoKeptPackages(installed, index)
	content := renderRepoPin(repo, loc.Host, kept)
	if readRepoRecord().Repos[repo.ID].PinSHA256 == sha256Hex(content) {
		return
	}
	if err := publishRepoPin(repoPinPath(repo.ID), content); err != nil {
		return
	}
	shown := kept
	if len(shown) > repoKeptCap {
		shown = shown[:repoKeptCap]
	}
	_ = updateRepoRecord(repo.ID, func(entry *repoRecordEntry, _ bool) bool {
		entry.PinSHA256 = sha256Hex(content)
		entry.Kept, entry.KeptTotal = shown, len(kept)
		return true
	})
}

// debVersionCompare is dpkg's version ordering: epoch, then upstream, then
// revision, each compared by alternating non-digit and digit runs, with "~"
// sorting before everything.
func debVersionCompare(a, b string) int {
	ea, ua, ra := splitDebVersion(a)
	eb, ub, rb := splitDebVersion(b)
	if c := compareDebNumber(ea, eb); c != 0 {
		return c
	}
	if c := compareDebPart(ua, ub); c != 0 {
		return c
	}
	return compareDebPart(ra, rb)
}

func splitDebVersion(v string) (epoch, upstream, revision string) {
	epoch = "0"
	if e, rest, ok := strings.Cut(v, ":"); ok {
		epoch, v = e, rest
	}
	if i := strings.LastIndex(v, "-"); i >= 0 {
		return epoch, v[:i], v[i+1:]
	}
	return epoch, v, ""
}

func compareDebNumber(a, b string) int {
	a, b = strings.TrimLeft(a, "0"), strings.TrimLeft(b, "0")
	if len(a) != len(b) {
		if len(a) < len(b) {
			return -1
		}
		return 1
	}
	return strings.Compare(a, b)
}

func debCharOrder(r rune) int {
	switch {
	case r == '~':
		return -1
	case unicode.IsLetter(r):
		return int(r)
	default:
		return int(r) + 256
	}
}

func compareDebPart(a, b string) int {
	for a != "" || b != "" {
		na := strings.IndexFunc(a, unicode.IsDigit)
		nb := strings.IndexFunc(b, unicode.IsDigit)
		if na < 0 {
			na = len(a)
		}
		if nb < 0 {
			nb = len(b)
		}
		sa, sb := a[:na], b[:nb]
		a, b = a[na:], b[nb:]
		for i := 0; i < len(sa) || i < len(sb); i++ {
			ca, cb := 0, 0
			if i < len(sa) {
				ca = debCharOrder(rune(sa[i]))
			}
			if i < len(sb) {
				cb = debCharOrder(rune(sb[i]))
			}
			if ca != cb {
				if ca < cb {
					return -1
				}
				return 1
			}
		}
		da := strings.IndexFunc(a, func(r rune) bool { return !unicode.IsDigit(r) })
		db := strings.IndexFunc(b, func(r rune) bool { return !unicode.IsDigit(r) })
		if da < 0 {
			da = len(a)
		}
		if db < 0 {
			db = len(b)
		}
		if c := compareDebNumber(a[:da], b[:db]); c != 0 {
			return c
		}
		a, b = a[da:], b[db:]
	}
	return 0
}
