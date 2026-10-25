package main

import (
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/alicelik/celikpanel/internal/hostplatform"
	"github.com/alicelik/celikpanel/internal/supportdata"
)

// The dated support table (alpha.107, D-038 §11, §5, §14). GET answers, for
// every version the table knows, whether it still receives security fixes on
// this server (from the distribution release the server runs, or from its
// upstream), when that ends, and the table's sources and check date. Every
// signed-in person may read it: a customer sees the mark of the PHP version
// their own site runs. The server's release is named only to an account
// holder, not to a team member (it is server-wide inventory); the statuses
// are the same for everyone.
//
// Tarihli destek tablosu: her sürümün bu sunucuda güvenlik düzeltmesi alıp
// almadığı, ne zamana dek ve kimden; kaynaklar ve denetim tarihiyle.

const supportTablePath = "/api/v1/support-table"

// osReleasePath and supportNow are seams for tests.
var (
	osReleasePath = "/etc/os-release"
	supportNow    = time.Now
)

// serverDistroKey is the table's key for this server ("debian-13"), and its
// codename.
func serverDistroKey() (key, codename string) {
	data, err := os.ReadFile(osReleasePath)
	if err != nil {
		return "", ""
	}
	release, err := hostplatform.ParseOSRelease(data)
	if err != nil {
		return "", ""
	}
	return supportdata.DistroKey(release.ID, release.Version), release.Codename
}

type supportTableView struct {
	Checked    string                                   `json:"checked"`
	Sources    map[string]string                        `json:"sources"`
	Distro     string                                   `json:"distro,omitempty"`
	DistroName string                                   `json:"distro_name,omitempty"`
	Statuses   map[string]map[string]supportdata.Result `json:"statuses"`
	// Tested / TestedRelease (alpha.108, D-038 §15): the version lines this
	// release was tested with on this server's distribution release; every
	// other line is "installable, not tested". Information, never a block.
	Tested        map[string][]string `json:"tested,omitempty"`
	TestedRelease string              `json:"tested_release,omitempty"`
}

// supportStatus is one version's status on this server (for refusals and
// audit details).
func supportStatus(component, version string) supportdata.Result {
	table, err := supportdata.Load()
	if err != nil {
		return supportdata.Result{Status: supportdata.StatusUnknown}
	}
	key, _ := serverDistroKey()
	return table.Status(component, version, key, supportNow())
}

// phpChangeSupport is a site's PHP change against the table: the chosen
// version's support, and whether the change must be refused because it no
// longer receives security fixes and was not confirmed.
func phpChangeSupport(current, requested string, confirmed bool) (supportdata.Result, bool) {
	support := supportStatus("php", requested)
	return support, requested != current && support.Status == supportdata.StatusUnsupported && !confirmed
}

func (p *Panel) handleSupportTable(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		writeClientError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	caller := currentCaller(r)
	if caller == nil {
		writeClientError(w, http.StatusUnauthorized, "sign in first")
		return
	}
	table, err := supportdata.Load()
	if err != nil {
		writeServerError(w, err)
		return
	}
	key, _ := serverDistroKey()
	now := supportNow()
	view := supportTableView{Checked: table.Checked, Sources: table.Sources, Statuses: map[string]map[string]supportdata.Result{}}
	team := caller.isAdditionalUser()
	for id, component := range table.Components {
		view.Statuses[id] = map[string]supportdata.Result{}
		for line := range component.Versions {
			result := table.Status(id, line, key, now)
			if team {
				result.Note = ""
			}
			view.Statuses[id][line] = result
		}
	}
	if !team {
		view.Distro = key
		view.DistroName = table.Distros[key].Name
		if tested, err := supportdata.LoadTested(); err == nil {
			view.Tested, view.TestedRelease = tested.For(key), tested.Release
		}
	}
	json.NewEncoder(w).Encode(view)
}
