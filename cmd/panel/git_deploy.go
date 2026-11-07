package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/nodesite"
	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Git deploy of a site (alpha.128, PLESK-GAP G32; nodesite/gitdeploy.go).
//
//	GET  /api/v1/domains/{id}/git          the repository, branch, key fingerprint, last deploy and its output
//	POST /api/v1/domains/{id}/git          save the repository address, branch and build switch
//	POST /api/v1/domains/{id}/git/key      make the deploy key; the PUBLIC key is in this answer only
//	POST /api/v1/domains/{id}/git/deploy   start one deploy (request_id; a repeated press answers the first)
//
// A deploy runs as the site's own account in the site sandbox: git init,
// fetch of the one branch, reset to it; a Node.js site may then install its
// packages and build. There is no command box. The status read never starts
// anything; the screen reads it while the deploy runs and shows the output.
//
// Git ile yayın: sitenin kendi hesabıyla, site korumasında; komut kutusu yok.

const (
	errCodeGitRefused = "GIT_DEPLOY_REFUSED"
)

var requestIdentityRouteGitKey = requestIdentityRoute{
	pattern: "/api/v1/domains/{id}/git/key", optional: true, secret: true,
	timeout: agentRPCMutationTimeout + time.Minute,
}

// gitDeployAgent is the Agent call (a test seam).
var gitDeployAgent = func(p *Panel, ctx context.Context, method string, req, resp any) error {
	return p.callAgentContext(ctx, method, req, resp)
}

type gitDeploySettings struct {
	RepoURL      string `json:"repo_url"`
	Branch       string `json:"branch"`
	Build        bool   `json:"build"`
	KeyCreatedAt string `json:"key_created_at,omitempty"`
}

func loadGitDeploySettings(ctx context.Context, db *sql.DB, siteID int) gitDeploySettings {
	s := gitDeploySettings{Branch: "main"}
	var build int
	var keyAt sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT repo_url, branch, build, key_created_at FROM git_deploy_settings WHERE site_id = ?`, siteID).
		Scan(&s.RepoURL, &s.Branch, &build, &keyAt); err == nil {
		s.Build, s.KeyCreatedAt = build == 1, keyAt.String
	}
	return s
}

func gitDeployable(projectType string) bool {
	switch projectType {
	case "php", "static", "node":
		return true
	}
	return false
}

func writeGitRefusal(w http.ResponseWriter, status int, reason, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(apiErrorBody{Error: message, Code: errCodeGitRefused, Reason: reason})
}

func (p *Panel) handleDomainGitDeploy(w http.ResponseWriter, r *http.Request, domainID int, kind string) {
	ctx := r.Context()
	w.Header().Set("Cache-Control", "no-store")
	state, err := p.loadHostingRuntimeState(ctx, domainID)
	if err != nil {
		writeClientError(w, http.StatusNotFound, "site not found")
		return
	}
	if !gitDeployable(state.ProjectType) {
		writeGitRefusal(w, http.StatusConflict, "not_deployable", "This site serves no files of its own (forwarding, proxy or an app), so it has no Git deploy. Nothing was changed.")
		return
	}
	db := p.db.GetDB()
	settings := loadGitDeploySettings(ctx, db, state.SiteID)
	canBuild := state.ProjectType == "node" && state.RuntimeVersion.Valid && state.RuntimeVersion.String != ""
	switch {
	case kind == "git" && r.Method == http.MethodGet:
		var status transport.GitDeployResponse
		statusErr := gitDeployAgent(p, ctx, "Agent.GitDeployStatusV1", &transport.GitDeployStatusRequest{SiteID: state.SiteID}, &status)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"settings": settings, "can_build": canBuild, "status": status, "status_unknown": statusErr != nil,
		})
	case kind == "git" && r.Method == http.MethodPost:
		var req gitDeploySettings
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil {
			writeClientError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		req.RepoURL, req.Branch = strings.TrimSpace(req.RepoURL), strings.TrimSpace(req.Branch)
		if _, err := nodesite.ParseGitURL(req.RepoURL); err != nil {
			writeGitRefusal(w, http.StatusBadRequest, "repo_url", "Nothing was changed: "+err.Error()+".")
			return
		}
		if !nodesite.ValidGitBranch(req.Branch) {
			writeGitRefusal(w, http.StatusBadRequest, "branch", "Nothing was changed: the branch is a plain name such as main.")
			return
		}
		if req.Build && !canBuild {
			writeGitRefusal(w, http.StatusBadRequest, "build", "Nothing was changed: only a Node.js site with its Node.js version builds after a deploy.")
			return
		}
		build := 0
		if req.Build {
			build = 1
		}
		if _, err := db.ExecContext(ctx, `
			INSERT INTO git_deploy_settings (site_id, repo_url, branch, build) VALUES (?, ?, ?, ?)
			ON CONFLICT(site_id) DO UPDATE SET repo_url = excluded.repo_url, branch = excluded.branch, build = excluded.build, updated_at = datetime('now')`,
			state.SiteID, req.RepoURL, req.Branch, build); err != nil {
			writeServerError(w, err)
			return
		}
		p.auditDetailed(r, "domain.git.settings", "domain", domainID, map[string]any{"branch": req.Branch, "build": req.Build})
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"settings": loadGitDeploySettings(ctx, db, state.SiteID)})
	case kind == "git-key":
		var req struct {
			Replace bool `json:"replace"`
		}
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&req)
		var answer transport.GitDeployKeyResponse
		if err := gitDeployAgent(p, ctx, "Agent.GitDeployKeyV1", &transport.GitDeployKeyRequest{
			ExpectedBuildCommit: strings.TrimSpace(buildCommit), SiteID: state.SiteID, Domain: state.DomainName, Replace: req.Replace,
		}, &answer); err != nil {
			writeGitRefusal(w, http.StatusBadGateway, "agent_unreachable", "The server did not answer, so it is not known whether a key was made. Read the page again; make a key again only if none is shown.")
			return
		}
		if answer.Error != "" {
			status := http.StatusConflict
			writeGitRefusal(w, status, answer.Reason, "No key was made: "+answer.Error+".")
			return
		}
		_, _ = db.ExecContext(ctx, `
			INSERT INTO git_deploy_settings (site_id, key_created_at) VALUES (?, datetime('now'))
			ON CONFLICT(site_id) DO UPDATE SET key_created_at = datetime('now'), updated_at = datetime('now')`, state.SiteID)
		p.auditDetailed(r, "domain.git.key", "domain", domainID, map[string]any{"fingerprint": answer.Fingerprint, "replace": req.Replace})
		doNotRetainRequestAnswer(ctx)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"public_key": answer.PublicKey, "fingerprint": answer.Fingerprint})
	case kind == "git-deploy":
		var body struct {
			RequestID string `json:"request_id"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&body); err != nil || !refreshRequestIDPattern.MatchString(body.RequestID) {
			writeClientError(w, http.StatusBadRequest, "a request id is required")
			return
		}
		if settings.RepoURL == "" {
			writeGitRefusal(w, http.StatusConflict, "no_repository", "Save the repository address first; nothing was started.")
			return
		}
		if p.refuseAppStartUnderEmergency(w, r, domainID, "start") {
			return
		}
		callCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
		defer cancel()
		req := transport.GitDeployRequest{
			ExpectedBuildCommit: strings.TrimSpace(buildCommit), RequestID: body.RequestID, SiteID: state.SiteID,
			Domain: state.DomainName, WorkDir: state.DocumentRoot, RunAsUser: services.SiteUsername(state.DomainName),
			RepoURL: settings.RepoURL, Branch: settings.Branch,
		}
		if settings.Build && canBuild {
			req.Build, req.NodeVersion = true, state.RuntimeVersion.String
		}
		var resp transport.GitDeployResponse
		if err := gitDeployAgent(p, callCtx, "Agent.GitDeployV1", &req, &resp); err != nil {
			writeServerError(w, err)
			return
		}
		if resp.RequestID == body.RequestID && resp.Phase != "" {
			p.auditDetailed(r, "domain.git.deploy", "domain", domainID, map[string]any{"request_id": body.RequestID, "branch": settings.Branch, "phase": resp.Phase})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	default:
		http.NotFound(w, r)
	}
}
