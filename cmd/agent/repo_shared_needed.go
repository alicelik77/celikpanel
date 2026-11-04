package main

// A vendor-only version can need more of the vendor's shared packages than
// the ones it always takes (alpha.121, Boston, Ubuntu 24.04). PGDG's
// postgresql-18 depends on postgresql-common (>= 275~) and its client on
// postgresql-client-common; Debian 13 ships 278 and Frankfurt installed
// PostgreSQL 18 with them, Ubuntu 24.04 ships 257 and, under the confinement
// pin (the vendor at 100, the distribution at 500), apt keeps 257 and gives
// up: "postgresql-18 : Depends: postgresql-client-18 (= …) but it is not
// going to be installed". The pin is right; the request was incomplete.
//
// So the vendor's own dependency lines decide, read from its downloaded
// package lists: walking from the selected version through the packages only
// the vendor ships, every dependency that the version apt would otherwise keep
// (the installed one, or the distribution's newest) does not satisfy, and that
// the repository allows (SharedPackages, SharedWhenNeeded), is named as
// name=<the vendor's newest version that satisfies it>. Nothing else is ever
// named, the pin is never widened, nothing is removed, and where the
// distribution's version satisfies the dependency (Debian 13) nothing more is
// named than before. The named packages are notable vendor_shared items the
// owner confirms, as php-common and libpq5 already were.
//
// Üreticiye özgü bir sürümün bağımlılığını dağıtımın sürümü karşılamıyorsa,
// deponun izin verdiği ortak paket üreticinin sürümüyle adıyla istenir;
// karşılıyorsa hiçbir şey eklenmez. Sabitleme genişletilmez, paket kaldırılmaz.

import (
	"bufio"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
)

// vendorNeededCap bounds the walk (names visited).
const vendorNeededCap = 64

// aptDependency is one alternative of a dependency group.
type aptDependency struct {
	Name, Op, Version string
}

// parseAptDependencyGroups splits a Depends/Pre-Depends value into groups of
// alternatives. An unparsable alternative is kept by name only.
func parseAptDependencyGroups(value string) [][]aptDependency {
	var groups [][]aptDependency
	for _, rawGroup := range strings.Split(value, ",") {
		var group []aptDependency
		for _, raw := range strings.Split(rawGroup, "|") {
			raw = strings.TrimSpace(raw)
			if raw == "" {
				continue
			}
			// Drop an architecture restriction ("[amd64]") and a build
			// profile ("<!nocheck>"); neither is in a binary Packages file in
			// practice, but neither may become part of a name.
			if i := strings.Index(raw, "["); i >= 0 {
				raw = strings.TrimSpace(raw[:i])
			}
			tail := strings.LastIndex(raw, ")") + 1
			if i := strings.Index(raw[tail:], "<"); i >= 0 && (tail > 0 || !strings.Contains(raw, "(")) {
				raw = strings.TrimSpace(raw[:tail+i])
			}
			dep := aptDependency{}
			name := raw
			if open := strings.Index(raw, "("); open >= 0 {
				name = strings.TrimSpace(raw[:open])
				inner := strings.TrimSuffix(strings.TrimSpace(raw[open+1:]), ")")
				inner = strings.TrimSpace(inner)
				for _, op := range []string{"<<", "<=", ">=", ">>", "=", "<", ">"} {
					if strings.HasPrefix(inner, op) {
						dep.Op = op
						dep.Version = strings.TrimSpace(strings.TrimPrefix(inner, op))
						break
					}
				}
			}
			if colon := strings.Index(name, ":"); colon >= 0 {
				name = name[:colon]
			}
			dep.Name = name
			if validPackageName(dep.Name) {
				group = append(group, dep)
			}
		}
		if len(group) > 0 {
			groups = append(groups, group)
		}
	}
	return groups
}

// satisfiedBy: version satisfies the dependency's version relation (none:
// any version does).
func (d aptDependency) satisfiedBy(version string) bool {
	if version == "" {
		return false
	}
	if d.Op == "" || d.Version == "" {
		return true
	}
	c := debVersionCompare(version, d.Version)
	switch d.Op {
	case "<<":
		return c < 0
	case "<=", "<":
		return c <= 0
	case "=":
		return c == 0
	case ">=", ">":
		return c >= 0
	case ">>":
		return c > 0
	}
	return false
}

func newestVersion(versions map[string]bool, keep func(string) bool) string {
	best := ""
	for version := range versions {
		if !validDebianVersion(version) || (keep != nil && !keep(version)) {
			continue
		}
		if best == "" || debVersionCompare(version, best) > 0 {
			best = version
		}
	}
	return best
}

func newestOf(versions []string) string {
	best := ""
	for _, version := range versions {
		if best == "" || debVersionCompare(version, best) > 0 {
			best = version
		}
	}
	return best
}

// vendorNeedInputs is what the walk reads.
type vendorNeedInputs struct {
	// allowed are the packages that may be named (SharedPackages,
	// SharedWhenNeeded).
	allowed map[string]bool
	// depends: the vendor's lists, name -> version -> Depends and
	// Pre-Depends joined.
	depends map[string]map[string]string
	// distro: the distribution's lists, name -> versions.
	distro map[string]map[string]bool
	// installed: dpkg's status, name -> versions.
	installed map[string][]string
	// named: packages the request already names exactly.
	named map[string]bool
}

func (in vendorNeedInputs) vendorVersions(name string) map[string]bool {
	out := map[string]bool{}
	for version := range in.depends[name] {
		out[version] = true
	}
	return out
}

// keptVersion is the version apt keeps for name when nothing names it: the
// distribution's newest (priority 500), or the installed one when it is newer
// or the distribution has none. followsVendor: the installed version is the
// vendor's, so the confinement keeps it following the vendor.
func (in vendorNeedInputs) keptVersion(name string) (version string, followsVendor bool) {
	for _, installed := range in.installed[name] {
		if _, ok := in.depends[name][installed]; ok {
			return installed, true
		}
	}
	distro := newestVersion(in.distro[name], nil)
	installed := newestOf(in.installed[name])
	if installed != "" && (distro == "" || debVersionCompare(installed, distro) > 0) {
		return installed, false
	}
	return distro, false
}

// vendorNeededSharedArgs walks from the selected vendor-only package and
// returns the allowed shared packages it needs from the vendor, as
// name=version, sorted.
func vendorNeededSharedArgs(selected string, in vendorNeedInputs) []string {
	start := newestVersion(in.vendorVersions(selected), nil)
	if start == "" {
		return nil
	}
	named := map[string]bool{}
	for name := range in.named {
		named[name] = true
	}
	type node struct{ name, version string }
	queue := []node{{selected, start}}
	visited := map[string]bool{selected: true}
	var args []string
	for len(queue) > 0 && len(visited) <= vendorNeededCap {
		current := queue[0]
		queue = queue[1:]
		for _, group := range parseAptDependencyGroups(in.depends[current.name][current.version]) {
			// Alternatives are apt's own choice; they are never named here.
			if len(group) != 1 {
				continue
			}
			dep := group[0]
			if named[dep.Name] || visited[dep.Name] {
				continue
			}
			kept, followsVendor := in.keptVersion(dep.Name)
			if kept == "" {
				// Only the vendor ships it: apt takes the vendor's, and its
				// own dependencies are walked.
				if version := newestVersion(in.vendorVersions(dep.Name), nil); version != "" {
					visited[dep.Name] = true
					queue = append(queue, node{dep.Name, version})
				}
				continue
			}
			if followsVendor || dep.satisfiedBy(kept) || !in.allowed[dep.Name] {
				continue
			}
			version := newestVersion(in.vendorVersions(dep.Name), dep.satisfiedBy)
			if version == "" {
				continue
			}
			named[dep.Name] = true
			visited[dep.Name] = true
			args = append(args, dep.Name+"="+version)
			queue = append(queue, node{dep.Name, version})
		}
	}
	sort.Strings(args)
	return args
}

// vendorDependsCache keeps one parsed dependency index per list file, by its
// size and modification time.
var vendorDependsCache = struct {
	sync.Mutex
	entries map[string]vendorDependsEntry
}{entries: map[string]vendorDependsEntry{}}

type vendorDependsEntry struct {
	size    int64
	modTime time.Time
	depends map[string]map[string]string
}

// parseAptPackagesDepends reads Package, Version, Depends and Pre-Depends of
// every stanza (continuation lines included).
func parseAptPackagesDepends(r io.Reader) (map[string]map[string]string, error) {
	out := map[string]map[string]string{}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64<<10), 4<<20)
	var name, version, depends, field string
	flush := func() {
		if name != "" && version != "" {
			if out[name] == nil {
				out[name] = map[string]string{}
			}
			out[name][version] = depends
		}
		name, version, depends, field = "", "", "", ""
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			flush()
			continue
		}
		if line[0] == ' ' || line[0] == '\t' {
			if field == "depends" {
				depends += " " + strings.TrimSpace(line)
			}
			continue
		}
		key, value, _ := strings.Cut(line, ":")
		value = strings.TrimSpace(value)
		field = ""
		switch key {
		case "Package":
			name = value
		case "Version":
			version = value
		case "Depends", "Pre-Depends":
			field = "depends"
			if depends != "" {
				depends += ", "
			}
			depends += value
		}
	}
	flush()
	return out, scanner.Err()
}

// readVendorPackageDepends reads the repository's downloaded lists.
func readVendorPackageDepends(loc aptSourceLocation) (map[string]map[string]string, error) {
	files := loc.packagesFiles()
	if len(files) == 0 {
		return nil, os.ErrNotExist
	}
	out := map[string]map[string]string{}
	for _, file := range files {
		info, err := os.Stat(file)
		if err != nil {
			return nil, err
		}
		vendorDependsCache.Lock()
		cached, ok := vendorDependsCache.entries[file]
		vendorDependsCache.Unlock()
		depends := cached.depends
		if !ok || cached.size != info.Size() || !cached.modTime.Equal(info.ModTime()) {
			f, err := os.Open(file)
			if err != nil {
				return nil, err
			}
			depends, err = parseAptPackagesDepends(io.LimitReader(f, repoListsReadCap))
			f.Close()
			if err != nil {
				return nil, err
			}
			vendorDependsCache.Lock()
			vendorDependsCache.entries[file] = vendorDependsEntry{size: info.Size(), modTime: info.ModTime(), depends: depends}
			vendorDependsCache.Unlock()
		}
		for name, versions := range depends {
			if out[name] == nil {
				out[name] = map[string]string{}
			}
			for version, value := range versions {
				out[name][version] = value
			}
		}
	}
	return out, nil
}

// vendorWalkNames are every dependency name reachable from selected through
// the vendor's packages (bounded): the names whose distribution versions the
// walk needs.
func vendorWalkNames(selected string, depends map[string]map[string]string) map[string]bool {
	names := map[string]bool{selected: true}
	queue := []string{selected}
	for len(queue) > 0 && len(names) <= 4*vendorNeededCap {
		current := queue[0]
		queue = queue[1:]
		for _, value := range depends[current] {
			for _, group := range parseAptDependencyGroups(value) {
				for _, dep := range group {
					if names[dep.Name] {
						continue
					}
					names[dep.Name] = true
					if len(depends[dep.Name]) > 0 {
						queue = append(queue, dep.Name)
					}
				}
			}
		}
	}
	return names
}

// hostVendorNeededSharedArgs reads the host for vendorNeededSharedArgs. An
// unreadable list adds nothing (the simulation then says why).
func hostVendorNeededSharedArgs(repo *core.ManagedRepo, loc aptSourceLocation, selected string, installed map[string][]string, named []string) []string {
	if repo == nil || len(repo.SharedWhenNeeded) == 0 {
		return nil
	}
	depends, err := readVendorPackageDepends(loc)
	if err != nil || len(depends[selected]) == 0 {
		return nil
	}
	distro, err := readDistributionVersions(vendorWalkNames(selected, depends))
	if err != nil {
		return nil
	}
	in := vendorNeedInputs{
		allowed:   map[string]bool{},
		depends:   depends,
		distro:    distro,
		installed: installed,
		named:     map[string]bool{},
	}
	for _, name := range append(append([]string{}, repo.SharedPackages...), repo.SharedWhenNeeded...) {
		in.allowed[name] = true
	}
	for _, arg := range named {
		in.named[installArgName(arg)] = true
	}
	return vendorNeededSharedArgs(selected, in)
}
