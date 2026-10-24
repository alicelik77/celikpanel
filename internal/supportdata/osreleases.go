package supportdata

// The supported operating-system releases (alpha.107, D-038 §17): a data
// file in the repository (os-releases.json), shown on the server setup page.
// A release that is not listed has not been checked; an OS release upgrade
// is the owner's step and CelikPanel never starts it.
//
// Desteklenen işletim sistemi sürümleri: depodaki veri dosyası. Listede
// olmayan sürüm denetlenmemiştir; sürüm yükseltmesini sahibi yapar.

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

//go:embed os-releases.json
var osReleasesJSON []byte

const (
	LevelTested      = "tested"
	LevelAccepted    = "accepted"
	LevelDevelopment = "development"
	LevelPreview     = "preview"
	LevelNotListed   = "not_listed"
)

type OSReleases struct {
	Schema   int         `json:"schema"`
	Checked  string      `json:"checked"`
	About    string      `json:"about"`
	Releases []OSRelease `json:"releases"`
}

type OSRelease struct {
	ID        string   `json:"id"`
	Version   string   `json:"version"`
	Codename  string   `json:"codename"`
	Name      string   `json:"name"`
	Level     string   `json:"level"`
	DistroKey string   `json:"distro_key,omitempty"`
	Also      []string `json:"also,omitempty"`
}

// LoadOSReleases reads the embedded list and refuses a malformed one.
func LoadOSReleases() (OSReleases, error) {
	var list OSReleases
	if err := json.Unmarshal(osReleasesJSON, &list); err != nil {
		return list, fmt.Errorf("os-releases.json: %w", err)
	}
	if list.Schema != 1 || len(list.Releases) == 0 {
		return list, fmt.Errorf("os-releases.json: schema %d with %d releases", list.Schema, len(list.Releases))
	}
	for _, release := range list.Releases {
		switch release.Level {
		case LevelTested, LevelAccepted, LevelDevelopment, LevelPreview:
		default:
			return list, fmt.Errorf("os-releases.json: %s has level %q", release.Name, release.Level)
		}
	}
	return list, nil
}

// Find is this server's entry: the same ID (or one of its alias IDs) and the
// same major version (Debian and the RHEL family) or exact version (Ubuntu).
// A rolling release matches on ID alone.
func (list OSReleases) Find(id, versionID string) (OSRelease, bool) {
	id, versionID = strings.ToLower(strings.TrimSpace(id)), strings.TrimSpace(versionID)
	for _, release := range list.Releases {
		ids := append([]string{release.ID}, release.Also...)
		matched := false
		for _, candidate := range ids {
			if candidate == id {
				matched = true
			}
		}
		if !matched {
			continue
		}
		if release.Version == "" {
			return release, true
		}
		version := versionID
		if id != "ubuntu" {
			version, _, _ = strings.Cut(versionID, ".")
		}
		if version == release.Version {
			return release, true
		}
	}
	return OSRelease{}, false
}
