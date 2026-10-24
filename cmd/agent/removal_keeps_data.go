package main

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Removing a component is not deleting its data (alpha.107, D-038 §8). A
// database engine (PostgreSQL, MariaDB) is removed with apt's "remove", not
// "purge", until purging is proven to keep the clusters and databases the
// owner may have made without the panel: the data directory and the
// engine's configuration stay on disk, and the removal dialogue says where.
// When an install finds such data again, its preview says so plainly: the
// engine uses it as it is.
//
// Bir bileşeni kaldırmak verisini silmek değildir: veritabanı motoru
// "remove" ile kaldırılır; veri ve yapılandırma diskte kalır.

// dataRoots is where each engine keeps its data, per package family (a
// variable for tests).
var dataRoots = map[string]map[string]string{
	"postgresql": {"apt": "/var/lib/postgresql", "pacman": "/var/lib/postgres"},
	"mariadb":    {"apt": "/var/lib/mysql", "pacman": "/var/lib/mysql"},
}

var postgresqlVersionPackage = regexp.MustCompile(`^postgresql-([0-9]+)$`)

func removalKeepsData(svc *core.ManagedService) bool {
	if svc == nil {
		return false
	}
	_, ok := dataRoots[svc.ID]
	return ok
}

// removalDataPaths are the data directories of this removal that exist on
// the server: one PostgreSQL major's, every major's, or MariaDB's.
func removalDataPaths(svc *core.ManagedService, family, pkg string) []string {
	if !removalKeepsData(svc) {
		return nil
	}
	root := dataRoots[svc.ID][family]
	if root == "" {
		return nil
	}
	if svc.ID == "postgresql" && family == "apt" {
		var paths []string
		if match := postgresqlVersionPackage.FindStringSubmatch(pkg); match != nil {
			paths = append(paths, filepath.Join(root, match[1]))
		} else if entries, err := os.ReadDir(root); err == nil {
			for _, entry := range entries {
				if entry.IsDir() && regexp.MustCompile(`^[0-9]+$`).MatchString(entry.Name()) {
					paths = append(paths, filepath.Join(root, entry.Name()))
				}
			}
		}
		out := paths[:0]
		for _, path := range paths {
			if dirHasEntries(path) {
				out = append(out, path)
			}
		}
		sort.Strings(out)
		return out
	}
	if dirHasEntries(root) {
		return []string{root}
	}
	return nil
}

func dirHasEntries(path string) bool {
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() {
		return false
	}
	entries, err := os.ReadDir(path)
	return err == nil && len(entries) > 0
}

// existingEngineData is what an install of an engine would find already on
// disk (alpha.107): the data directory a previous install left.
func existingEngineData(svc *core.ManagedService, family, pkg string) []transport.ExistingData {
	var out []transport.ExistingData
	for _, path := range removalDataPaths(svc, family, pkg) {
		out = append(out, transport.ExistingData{Component: svc.ID, Path: path})
	}
	return out
}
