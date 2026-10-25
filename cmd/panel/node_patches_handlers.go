package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Node.js patch notices (alpha.107, D-038 §9). GET
// /api/v1/runtimes/node/patches says, for every installed Node.js version,
// the newest release on its line and the sites still on that version.
// Installing the newer patch is the existing verified install, confirmed by
// the administrator. POST /api/v1/domains/{id}/app/node-version moves one
// site to another installed version on request: the application's unit is
// rewritten (which restarts it), the application must answer, and otherwise
// the site is put back on its previous version.
//
// Node.js yama bildirimleri ve bir siteyi yalnız istenince, kanıtla taşımak.

type nodePatchView struct {
	transport.NodePatch
	Sites []nodePatchSite `json:"sites"`
}

type nodePatchSite struct {
	DomainID int    `json:"domain_id"`
	Name     string `json:"name"`
}

var readNodePatchesFromAgent = func(p *Panel, ctx context.Context) (transport.NodePatchesResponse, error) {
	callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var response transport.NodePatchesResponse
	err := p.callAgentContext(callCtx, "Agent.NodePatchesV1", &transport.Empty{}, &response)
	return response, err
}

func (p *Panel) nodeVersionSites(ctx context.Context, version string) ([]nodePatchSite, error) {
	rows, err := p.db.GetDB().QueryContext(ctx, `
		SELECT d.id, d.name FROM sites s JOIN domains d ON d.id = s.domain_id
		WHERE COALESCE(s.project_type, 'php') = 'node' AND s.runtime_version = ?
		ORDER BY d.name`, version)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sites := []nodePatchSite{}
	for rows.Next() {
		var site nodePatchSite
		if err := rows.Scan(&site.DomainID, &site.Name); err != nil {
			return nil, err
		}
		sites = append(sites, site)
	}
	return sites, rows.Err()
}

func (p *Panel) handleNodePatches(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if c := currentCaller(r); c == nil || c.Role != roleAdmin {
		writeClientError(w, http.StatusForbidden, "administrator access required")
		return
	}
	response, err := readNodePatchesFromAgent(p, r.Context())
	if err != nil || response.Error != "" {
		log.Printf("node patches: %v %s", err, response.Error)
		json.NewEncoder(w).Encode(map[string]any{"state": "unknown"})
		return
	}
	views := []nodePatchView{}
	for _, patch := range response.Patches {
		sites, err := p.nodeVersionSites(r.Context(), patch.Installed)
		if err != nil {
			writeServerError(w, err)
			return
		}
		views = append(views, nodePatchView{NodePatch: patch, Sites: sites})
	}
	json.NewEncoder(w).Encode(map[string]any{"state": "read", "checked_at": response.CheckedAt, "patches": views})
}

var transitionHostingForMove = func(p *Panel, ctx context.Context, domainID int, settings hostingSettings) error {
	_, err := p.transitionHosting(ctx, domainID, settings)
	return err
}

// hostingSettingsOf is a Node.js site's current settings with another
// version.
func hostingSettingsOf(state hostingRuntimeState, version string) hostingSettings {
	settings := hostingSettings{ProjectType: "node", RuntimeVersion: version}
	if state.StartCommand.Valid {
		settings.StartCommand = state.StartCommand.String
	}
	if state.AppPort.Valid {
		settings.AppPort = int(state.AppPort.Int64)
	}
	return settings
}

func (p *Panel) handleDomainAppNodeVersion(w http.ResponseWriter, r *http.Request, domainID int) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		writeClientError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || !nodeSemverRe.MatchString(strings.TrimSpace(req.Version)) {
		writeClientError(w, http.StatusBadRequest, "a Node.js version is required")
		return
	}
	state, err := p.loadHostingRuntimeState(r.Context(), domainID)
	if errors.Is(err, errHostingSiteNotFound) {
		writeClientError(w, http.StatusNotFound, "site not found")
		return
	}
	if err != nil {
		writeServerError(w, err)
		return
	}
	if state.ProjectType != "node" || !state.AppPort.Valid || !state.RuntimeVersion.Valid {
		writeClientError(w, http.StatusConflict, "this domain is not a node project")
		return
	}
	from := state.RuntimeVersion.String
	// alpha.108 (D-038 §14): another line is a customer's new choice and
	// must be offered; another patch of the line the site runs never is
	// refused.
	if p.refuseVersionNotOffered(w, r, "node", from, req.Version) {
		return
	}
	if from == req.Version {
		json.NewEncoder(w).Encode(map[string]any{"moved": true, "from": from, "to": req.Version})
		return
	}
	ctx, cancel := hostingDurableContext(r.Context())
	defer cancel()
	failure := transitionHostingForMove(p, ctx, domainID, hostingSettingsOf(state, req.Version))
	if failure == nil {
		failure = proveAppAnswers(ctx, int(state.AppPort.Int64))
		if failure != nil {
			if backErr := transitionHostingForMove(p, ctx, domainID, hostingSettingsOf(state, from)); backErr != nil {
				failure = fmt.Errorf("%v; putting it back on %s did not complete: %v", failure, from, backErr)
			} else {
				failure = fmt.Errorf("the application did not answer on Node.js %s (%v); it was put back on %s", req.Version, failure, from)
				p.auditDetailed(r, "site.node_version.failed:"+state.DomainName+":"+from+"->"+req.Version+" — "+auditReason(failure.Error()), "domain", domainID,
					map[string]any{"site": state.DomainName, "component": "node", "from": from, "to": req.Version, "rolled_back": true})
				w.WriteHeader(http.StatusConflict)
				json.NewEncoder(w).Encode(map[string]any{"error": failure.Error(), "code": "NODE_MOVE_ROLLED_BACK", "rolled_back": true})
				return
			}
		}
	}
	if failure != nil {
		p.auditDetailed(r, "site.node_version.failed:"+state.DomainName+":"+from+"->"+req.Version+" — "+auditReason(failure.Error()), "domain", domainID,
			map[string]any{"site": state.DomainName, "component": "node", "from": from, "to": req.Version})
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]any{"error": failure.Error(), "code": "NODE_MOVE_FAILED"})
		return
	}
	details := map[string]any{"site": state.DomainName, "component": "node", "from": from, "to": req.Version, "proved": true}
	for key, value := range versionSupportDetails("node", req.Version) {
		if key != "version" {
			details[key] = value
		}
	}
	p.auditDetailed(r, "site.node_version:"+state.DomainName+":"+from+"->"+req.Version, "domain", domainID, details)
	json.NewEncoder(w).Encode(map[string]any{"moved": true, "from": from, "to": req.Version})
}
