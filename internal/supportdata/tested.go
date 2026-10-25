package supportdata

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
)

// Tested versions (alpha.108, D-038 §15): which component versions this
// release was tested with, per distribution release; the rest is
// "installable, not tested" — information, never a block. The data file is
// replaced with each release's real-system evidence.
//
// Denenmiş sürümler: bu sürümün hangi bileşen sürümleriyle denendiği; geri
// kalanı "kurulabilir, denenmedi" — yalnız bilgi.

//go:embed tested-versions.json
var testedJSON []byte

// TestedVersions is the parsed data file.
type TestedVersions struct {
	Schema   int                            `json:"schema"`
	Release  string                         `json:"release"`
	About    string                         `json:"about"`
	Evidence string                         `json:"evidence"`
	Tested   map[string]map[string][]string `json:"tested"`
}

var testedLine = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?$`)

// LoadTested reads the embedded file and refuses a malformed one.
func LoadTested() (TestedVersions, error) {
	var tested TestedVersions
	if err := json.Unmarshal(testedJSON, &tested); err != nil {
		return TestedVersions{}, err
	}
	if tested.Schema != 1 || tested.Release == "" {
		return TestedVersions{}, fmt.Errorf("tested versions: schema %d, release %q", tested.Schema, tested.Release)
	}
	for distro, components := range tested.Tested {
		for component, lines := range components {
			for _, line := range lines {
				if !testedLine.MatchString(line) {
					return TestedVersions{}, fmt.Errorf("tested versions: %s %s %q is not a version line", distro, component, line)
				}
			}
		}
	}
	return tested, nil
}

// For returns the tested lines per component on one distribution release
// (an empty map when nothing was tested there).
func (t TestedVersions) For(distroKey string) map[string][]string {
	out := map[string][]string{}
	for component, lines := range t.Tested[distroKey] {
		out[component] = append([]string(nil), lines...)
	}
	return out
}
