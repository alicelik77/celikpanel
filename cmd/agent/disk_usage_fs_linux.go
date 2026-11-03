//go:build linux

package main

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// hostingFileSystemFacts: the free space of the file system that holds path
// (percent, -1 when unknown) and whether it is mounted with project quotas
// (read from /proc/self/mounts; detect only, never changed).
func hostingFileSystemFacts(path string) (int, bool) {
	free := -1
	probe := path
	for {
		if _, err := os.Stat(probe); err == nil {
			break
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			break
		}
		probe = parent
	}
	var stat syscall.Statfs_t
	if err := syscall.Statfs(probe, &stat); err == nil && stat.Blocks > 0 {
		free = int(stat.Bavail * 100 / stat.Blocks)
	}
	return free, projectQuotaMounted(probe)
}

func projectQuotaMounted(path string) bool {
	content, err := os.ReadFile("/proc/self/mounts")
	if err != nil {
		return false
	}
	best, options := "", ""
	for _, line := range strings.Split(string(content), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		mount := fields[1]
		if (path == mount || strings.HasPrefix(path, strings.TrimSuffix(mount, "/")+"/")) && len(mount) >= len(best) {
			best, options = mount, fields[3]
		}
	}
	for _, option := range strings.Split(options, ",") {
		switch option {
		case "prjquota", "pquota", "prjjquota":
			return true
		}
	}
	return false
}
