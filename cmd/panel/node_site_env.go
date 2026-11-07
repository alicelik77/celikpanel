package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/siteruntime"
	"github.com/alicelik/celikpanel/internal/transport"
)

// A Node.js site's environment variables (alpha.128, PLESK-GAP G19).
//
//	GET  /api/v1/domains/{id}/app/env   the names; public values; secrets only by name
//	POST /api/v1/domains/{id}/app/env   replace them (a secret without a value keeps the server's)
//
// The values live in one root-owned EnvironmentFile on the server
// (cmd/agent/node_site_env.go); the Panel keeps the public values and the
// secret names only, so a secret is never stored here or shown back. A save
// runs the steps check → save (the file) → unit (the application unit names
// the file while it exists) → restart (a running application reads it).
//
// Node.js sitesinin ortam değişkenleri: değerler sunucudaki root dosyasında;
// Panel yalnız açık değerleri ve gizli adları saklar.

const maxNodeEnvRequestBytes = 512 << 10

var requestIdentityRouteNodeEnv = requestIdentityRoute{
	pattern: "/api/v1/domains/{id}/app/env", optional: true,
	timeout: 2*agentRPCMutationTimeout + time.Minute,
}

// Seams (tests replace them).
var (
	nodeSiteEnvCall = func(p *Panel, ctx context.Context, req *transport.NodeSiteEnvRequest) (transport.RuntimeAppStepResponse, error) {
		var resp transport.RuntimeAppStepResponse
		err := p.callAgentContext(ctx, "Agent.NodeSiteEnvV1", req, &resp)
		return resp, err
	}
	nodeSiteEnvRenderUnit = func(p *Panel, ctx context.Context, domainID int) error {
		request, _, err := p.appUnitRequest(ctx, domainID)
		if err != nil {
			return err
		}
		site, err := p.loadSiteFileSite(ctx, domainID)
		if err != nil {
			return err
		}
		// Written, never started or stopped here (the restart step decides).
		request.NoStart = true
		_, err = p.applyAppUnit(ctx, site, request)
		return err
	}
	nodeSiteEnvRestart = func(p *Panel, ctx context.Context, siteID int) (bool, error) {
		var status transport.AppStatusResponse
		if err := p.callAgentContext(ctx, "Agent.AppUnitStatus", &transport.AppControlRequest{SiteID: siteID}, &status); err != nil {
			return false, err
		}
		if status.Active != "active" {
			return false, nil
		}
		var resp transport.AppApplyResponse
		if err := p.callAgentContext(ctx, "Agent.ControlAppUnit", &transport.AppControlRequest{SiteID: siteID, Action: "restart"}, &resp); err != nil {
			return true, err
		}
		if resp.Error != "" {
			return true, errors.New(resp.Error)
		}
		return true, nil
	}
)

type nodeEnvRow struct {
	Public     map[string]string
	SecretKeys []string
}

func loadNodeEnv(ctx context.Context, db *sql.DB, siteID int) (nodeEnvRow, error) {
	row := nodeEnvRow{Public: map[string]string{}}
	var public, secrets string
	err := db.QueryRowContext(ctx, `SELECT env_public, env_secret_keys FROM node_site_env WHERE site_id = ?`, siteID).Scan(&public, &secrets)
	if errors.Is(err, sql.ErrNoRows) {
		return row, nil
	}
	if err != nil {
		return row, err
	}
	_ = json.Unmarshal([]byte(public), &row.Public)
	_ = json.Unmarshal([]byte(secrets), &row.SecretKeys)
	if row.Public == nil {
		row.Public = map[string]string{}
	}
	return row, nil
}

type nodeEnvEntryView struct {
	Key    string `json:"key"`
	Value  string `json:"value,omitempty"`
	Secret bool   `json:"secret"`
}

func (row nodeEnvRow) view() []nodeEnvEntryView {
	out := []nodeEnvEntryView{}
	for key, value := range row.Public {
		out = append(out, nodeEnvEntryView{Key: key, Value: value})
	}
	for _, key := range row.SecretKeys {
		out = append(out, nodeEnvEntryView{Key: key, Secret: true})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

func (row nodeEnvRow) keys() []string {
	keys := append([]string{}, row.SecretKeys...)
	for key := range row.Public {
		keys = append(keys, key)
	}
	return keys
}

func writeNodeEnvRefusal(w http.ResponseWriter, status int, reason, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(apiErrorBody{Error: message, Code: "NODE_ENV_REFUSED", Reason: reason})
}

func (p *Panel) handleDomainNodeEnv(w http.ResponseWriter, r *http.Request, domainID int) {
	ctx := r.Context()
	var siteID int
	var projectType string
	if err := p.db.GetDB().QueryRowContext(ctx, `SELECT id, COALESCE(project_type,'php') FROM sites WHERE domain_id = ?`, domainID).
		Scan(&siteID, &projectType); err != nil {
		writeClientError(w, http.StatusNotFound, "site not found")
		return
	}
	if projectType != "node" {
		writeNodeEnvRefusal(w, http.StatusConflict, "not_node", "This domain is not a Node.js site, so it has no application environment. Nothing was changed.")
		return
	}
	row, err := loadNodeEnv(ctx, p.db.GetDB(), siteID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"entries": row.view(), "min_secret": siteruntime.MinSecretLength})
		return
	}
	var req struct {
		Entries []transport.RuntimeAppEnvEntry `json:"entries"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxNodeEnvRequestBytes)).Decode(&req); err != nil {
		writeClientError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	reportOperationPlan(ctx, "check", "save", "unit", "restart")
	reportOperationStep(ctx, "check")
	known := map[string]bool{}
	for _, key := range row.SecretKeys {
		known[key] = true
	}
	entries := make([]siteruntime.EnvEntry, len(req.Entries))
	public, secretKeys := map[string]string{}, []string{}
	for i, e := range req.Entries {
		e.Key = strings.TrimSpace(e.Key)
		req.Entries[i].Key = e.Key
		entries[i] = siteruntime.EnvEntry{Key: e.Key, Value: e.Value, Secret: e.Secret}
		if e.Secret && e.Value == "" && !known[e.Key] {
			writeNodeEnvRefusal(w, http.StatusBadRequest, "env_values", "Nothing was changed: the secret "+e.Key+" needs a value the first time.")
			return
		}
		if e.Secret {
			secretKeys = append(secretKeys, e.Key)
		} else {
			public[e.Key] = e.Value
		}
	}
	if err := siteruntime.ValidateEnv(entries); err != nil {
		writeNodeEnvRefusal(w, http.StatusBadRequest, "env_values", "Nothing was changed: "+err.Error()+".")
		return
	}
	opCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*agentRPCMutationTimeout)
	defer cancel()
	reportOperationStep(ctx, "save")
	resp, err := nodeSiteEnvCall(p, opCtx, &transport.NodeSiteEnvRequest{
		ExpectedBuildCommit: strings.TrimSpace(buildCommit), SiteID: siteID, Entries: req.Entries, PreviousKeys: row.keys(),
	})
	if err != nil {
		writeNodeEnvRefusal(w, http.StatusBadGateway, "agent_unreachable",
			"The server did not answer, so it is not known whether the values were saved. The values are read again; enter them again only if they are missing.")
		return
	}
	if !resp.Done {
		status := http.StatusConflict
		if resp.Class == transport.RuntimeAppVerifiedFailure {
			status = http.StatusBadRequest
		}
		writeNodeEnvRefusal(w, status, resp.Reason, "Nothing was changed: "+resp.Error+".")
		return
	}
	sort.Strings(secretKeys)
	publicJSON, _ := json.Marshal(public)
	secretJSON, _ := json.Marshal(secretKeys)
	if _, err := p.db.GetDB().ExecContext(opCtx, `
		INSERT INTO node_site_env (site_id, env_public, env_secret_keys, updated_at) VALUES (?, ?, ?, datetime('now'))
		ON CONFLICT(site_id) DO UPDATE SET env_public = excluded.env_public, env_secret_keys = excluded.env_secret_keys, updated_at = excluded.updated_at`,
		siteID, string(publicJSON), string(secretJSON)); err != nil {
		writeServerError(w, err)
		return
	}
	keys := make([]string, 0, len(req.Entries))
	for _, e := range req.Entries {
		keys = append(keys, e.Key)
	}
	sort.Strings(keys)
	// The audit names the keys, never a value.
	p.auditDetailed(r, "domain.node.env", "domain", domainID, map[string]any{"keys": keys})
	reportOperationStep(ctx, "unit")
	if err := nodeSiteEnvRenderUnit(p, opCtx, domainID); err != nil {
		writeNodeEnvRefusal(w, http.StatusConflict, "unit",
			"The values are saved on the server, but the application's unit could not be written, so the application does not read them yet ("+
				firstLineOf(err.Error())+"). Hosting → Configuration shows the unit; save again after it is fixed.")
		return
	}
	reportOperationStep(ctx, "restart")
	restarted, err := nodeSiteEnvRestart(p, opCtx, siteID)
	if err != nil {
		writeNodeEnvRefusal(w, http.StatusConflict, "restart",
			"The values are saved, but the application could not be restarted, so it still runs with the previous ones. Restart it from the application controls.")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"keys": keys, "restarted": restarted})
}
