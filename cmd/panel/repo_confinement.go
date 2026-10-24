package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Confining a vendor repository is the owner's action from the repository
// card (alpha.107 review; D-038 §3 and §12; D-022): the card shows the plan
// (the exact preferences text and its SHA-256, the packages that keep
// following the vendor, and how `apt-get upgrade` changes), and only that
// text, confirmed by its SHA-256, is written, in its own repo_confine job,
// audited with the administrator. Nothing confines a repository by itself;
// a file the owner deleted is said, never written again unless asked.
//
// Depo sınırlaması sahibin eylemidir: plan gösterilir, yalnız onaylanan
// metin yazılır; kendiliğinden hiçbir şey yazılmaz.

const (
	repoConfinementCallBound   = 3 * time.Minute
	repoConfinePreviewBound    = 3 * time.Minute
	repoAptErrorLineCap        = 300
	repoConfinementAuditAction = "repo.confine"
	repoConfinePreviewAPIPath  = "/api/v1/repo/confine-preview"
)

// readRepoConfinePreview asks the Agent for the plan (read only).
var readRepoConfinePreview = func(p *Panel, ctx context.Context, repoID string) (transport.RepoConfinePreview, error) {
	var preview transport.RepoConfinePreview
	callCtx, cancel := context.WithTimeout(ctx, repoConfinePreviewBound)
	defer cancel()
	err := p.callAgentContext(callCtx, "Agent.RepoConfinePreviewV1", &enableRepoReq{RepoID: repoID}, &preview)
	return preview, err
}

// handleRepoConfinePreview: GET ?service_id= is what confining the
// component's repository would do. Admin only, read only.
func (p *Panel) handleRepoConfinePreview(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if c := currentCaller(r); c == nil || c.Role != roleAdmin {
		writeClientError(w, http.StatusForbidden, "admin only")
		return
	}
	if r.Method != http.MethodGet {
		writeClientError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	svc := core.GetManagedServiceByID(r.URL.Query().Get("service_id"))
	if svc == nil || svc.Repo == nil {
		writeClientError(w, http.StatusBadRequest, "service has no managed repository")
		return
	}
	preview, err := readRepoConfinePreview(p, r.Context(), svc.Repo.ID)
	if err != nil {
		log.Printf("[repo][%s][confine-preview] %v", svc.Repo.ID, err)
		json.NewEncoder(w).Encode(map[string]any{"state": "unknown"})
		return
	}
	if preview.Error != "" {
		json.NewEncoder(w).Encode(map[string]any{"state": "failed", "error": cleanAptLines([]string{preview.Error})})
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"state": "read", "preview": preview})
}

// confineRepo runs Agent.ConfineRepoV1 in its own repo_confine job.
var confineRepoThroughAgent = func(p *Panel, ctx context.Context, repoID, expectedSHA256 string) (transport.RepoStatusResponse, error) {
	var response transport.RepoStatusResponse
	callCtx, cancel := context.WithTimeout(ctx, repoConfinementCallBound)
	defer cancel()
	err := p.withStandaloneAgentMutation(callCtx, "repo_confine", repoID, "", func(callCtx context.Context, binding agentMutationBinding) error {
		request := &enableRepoReq{ServiceMutationBinding: binding, RepoID: repoID, ExpectedSHA256: expectedSHA256}
		if err := p.callAgentContext(callCtx, "Agent.ConfineRepoV1", request, &response); err != nil {
			return err
		}
		if response.Error != "" {
			return errors.New(response.Error)
		}
		if response.Confinement == nil || response.Confinement.State != "confined" {
			reason := "not confined"
			if response.Confinement != nil {
				reason = response.Confinement.State
				if response.Confinement.Reason != "" {
					reason += ": " + response.Confinement.Reason
				}
			}
			return errors.New("the repository was not confined (" + reason + ")")
		}
		return nil
	})
	return response, err
}

func (p *Panel) confineRepo(ctx context.Context, repoID, expectedSHA256 string) (transport.RepoStatusResponse, error) {
	return confineRepoThroughAgent(p, ctx, repoID, expectedSHA256)
}

// auditRepoConfinement records what the administrator's confinement did.
func (p *Panel) auditRepoConfinement(r *http.Request, repoID string, response transport.RepoStatusResponse, err error) {
	state := ""
	if response.Confinement != nil {
		state = response.Confinement.State
		if response.Confinement.Reason != "" {
			state += " (" + response.Confinement.Reason + ")"
		}
	}
	action := repoConfinementAuditAction + ":" + repoID + " — " + state
	if err != nil {
		action = repoConfinementAuditAction + ".failed:" + repoID + " — " + auditReason(err.Error())
	}
	details := map[string]any{"repository": repoID, "state": state}
	if response.Confinement != nil {
		details["kept_total"] = response.Confinement.KeptTotal
		details["pin_path"] = response.Confinement.PinPath
		details["owner_files"] = response.Confinement.OwnerFiles
	}
	p.auditDetailed(r, action, "repo", 0, details)
	log.Printf("[repo][%s][confine] %s", repoID, action)
}

// repoEnabledBy is who enabled the repository and when, from the audit log
// (the latest successful enable).
func (p *Panel) repoEnabledBy(ctx context.Context, repoID string) (string, string) {
	var who, when sql.NullString
	if p.db == nil {
		return "", ""
	}
	err := p.db.GetDB().QueryRowContext(ctx, `
		SELECT u.username, a.created_at
		FROM audit_logs a LEFT JOIN users u ON u.id = a.user_id
		WHERE a.action = ? ORDER BY a.id DESC LIMIT 1`, "repo.enable:"+repoID).Scan(&who, &when)
	if err != nil {
		return "", ""
	}
	return who.String, when.String
}

// cleanAptLines keeps apt's lines printable and bounded for the browser.
func cleanAptLines(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.Map(func(r rune) rune {
			if unicode.IsPrint(r) {
				return r
			}
			return ' '
		}, strings.TrimSpace(line))
		if len(line) > repoAptErrorLineCap {
			line = line[:repoAptErrorLineCap]
		}
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}
