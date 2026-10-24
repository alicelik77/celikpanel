// Package supportdata is the dated support table (alpha.107, D-038 §11):
// for PHP, PostgreSQL, MariaDB and Node.js, when each version stops receiving
// security fixes from whoever provides it. The table is a data file in the
// repository (support-table.json), not code: a version installed from the
// distribution is patched for as long as that distribution release is (the
// table names which version each supported release ships, and whether in a
// component the distribution's security maintenance covers); a version from
// a vendor repository or an official release follows its upstream. Each date
// is read from the source the file names, on the date it was checked; what is
// not known stays "unknown", never guessed.
//
// Tarihli destek tablosu: bir sürümün güvenlik düzeltmelerini kimden ve ne
// zamana dek aldığı; kod değil, depodaki veri dosyası. Bilinmeyen "bilinmiyor"
// kalır.
package supportdata

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

//go:embed support-table.json
var tableJSON []byte

// SoonWithin is how close an end date is "ends soon".
const SoonWithin = 90 * 24 * time.Hour

const (
	StatusSupported   = "supported"
	StatusEndsSoon    = "ends_soon"
	StatusUnsupported = "unsupported"
	StatusUnknown     = "unknown"
)

type Table struct {
	Schema     int                  `json:"schema"`
	Checked    string               `json:"checked"`
	About      string               `json:"about"`
	Sources    map[string]string    `json:"sources"`
	Distros    map[string]Distro    `json:"distros"`
	Components map[string]Component `json:"components"`
}

type Distro struct {
	Name         string `json:"name"`
	PatchedUntil string `json:"patched_until"`
	Note         string `json:"note"`
	Source       string `json:"source"`
}

type Component struct {
	Name     string                       `json:"name"`
	Source   string                       `json:"source"`
	Versions map[string]Version           `json:"versions"`
	Distro   map[string]map[string]string `json:"distro"`
}

type Version struct {
	UpstreamUntil string `json:"upstream_until"`
}

// Result is one version's support on one server.
type Result struct {
	Status string `json:"status"`
	// Until is the last day fixes are shipped (YYYY-MM-DD), or the last month
	// (YYYY-MM) where the source gives only the month; empty when unknown.
	Until string `json:"until,omitempty"`
	// Provider: "distribution" (the server's release patches it),
	// "upstream" (its developers), or "" when unknown.
	Provider string `json:"provider,omitempty"`
	// Note says why a status is unknown (universe) or names the release.
	Note string `json:"note,omitempty"`
}

// Load parses the embedded table.
func Load() (Table, error) {
	var table Table
	if err := json.Unmarshal(tableJSON, &table); err != nil {
		return Table{}, fmt.Errorf("support table: %w", err)
	}
	if table.Schema != 1 {
		return Table{}, fmt.Errorf("support table schema %d is not 1", table.Schema)
	}
	return table, nil
}

// Raw is the file as it is in the repository.
func Raw() []byte { return append([]byte(nil), tableJSON...) }

var (
	majorMinor = regexp.MustCompile(`^([0-9]+)\.([0-9]+)`)
	majorOnly  = regexp.MustCompile(`^([0-9]+)`)
)

// Line is the table's key for a version: PHP and MariaDB "8.4"/"11.8",
// PostgreSQL and Node.js the major ("17", "22").
func Line(component, version string) string {
	version = strings.TrimSpace(strings.TrimPrefix(version, "v"))
	switch component {
	case "php", "mariadb":
		if match := majorMinor.FindStringSubmatch(version); match != nil {
			return match[1] + "." + match[2]
		}
	case "postgresql", "node":
		if match := majorOnly.FindStringSubmatch(version); match != nil {
			return match[1]
		}
	}
	return ""
}

// DistroKey is the table's key for a server: "debian-13", "ubuntu-24.04".
func DistroKey(id, versionID string) string {
	id, versionID = strings.ToLower(strings.TrimSpace(id)), strings.TrimSpace(versionID)
	if id == "" || versionID == "" {
		return ""
	}
	if id == "debian" {
		if match := majorOnly.FindStringSubmatch(versionID); match != nil {
			versionID = match[1]
		}
	}
	return id + "-" + versionID
}

// Status answers one version on one server at now.
func (t Table) Status(component, version, distroKey string, now time.Time) Result {
	entry, ok := t.Components[component]
	line := Line(component, version)
	if !ok || line == "" {
		return Result{Status: StatusUnknown}
	}
	if shipped, ok := entry.Distro[distroKey][line]; ok {
		distro := t.Distros[distroKey]
		if shipped != "main" {
			return Result{Status: StatusUnknown, Provider: "distribution", Note: distro.Name + ": " + shipped}
		}
		return statusAt(distro.PatchedUntil, "distribution", distro.Name, now)
	}
	return statusAt(entry.Versions[line].UpstreamUntil, "upstream", "", now)
}

func statusAt(until, provider, note string, now time.Time) Result {
	var end time.Time
	if day, err := time.Parse("2006-01-02", until); err == nil {
		end = day.Add(24 * time.Hour)
	} else if month, err := time.Parse("2006-01", until); err == nil {
		// A month as the source gives it: supported through its last day.
		end = month.AddDate(0, 1, 0)
	} else {
		return Result{Status: StatusUnknown, Provider: provider, Note: note}
	}
	result := Result{Until: until, Provider: provider, Note: note}
	switch {
	case !now.Before(end):
		result.Status = StatusUnsupported
	case end.Sub(now) <= SoonWithin:
		result.Status = StatusEndsSoon
	default:
		result.Status = StatusSupported
	}
	return result
}
