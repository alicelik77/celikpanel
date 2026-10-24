package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Repositories enabled before alpha.107 are confined once, after the Panel
// starts on this release (D-038 §12): in their own repo_confine job, only when
// the Agent reads them as enabled and not confined (CelikPanel's file absent
// and no rule of the owner's for the repository), and the result is audited
// as the panel's own action. A busy host is retried, bounded; a confinement
// that could not be written is not retried here: the repository card says
// why and offers it again.
//
// alpha.107 öncesi açılmış depolar, panel bu sürümle başladıktan sonra bir kez
// sınırlanır; sahibin kuralı varsa dokunulmaz, sonuç denetim kaydına yazılır.

const (
	repoConfinementStartupDelay  = 30 * time.Second
	repoConfinementRetryInterval = time.Minute
	repoConfinementRetryAttempts = 15
	repoConfinementCallBound     = 3 * time.Minute
	repoAptErrorLineCap          = 300
	repoConfinementActionUpgrade = "repo.confine"
)

// confineReposAtStartup is a test seam.
var confineReposAtStartup = func(p *Panel) { p.confineEnabledRepos(context.Background()) }

func (p *Panel) startRepoConfinement() {
	go func() {
		time.Sleep(repoConfinementStartupDelay)
		confineReposAtStartup(p)
	}()
}

func catalogueRepos() []*core.ManagedRepo {
	var repos []*core.ManagedRepo
	for i := range core.ManagedServices {
		if repo := core.ManagedServices[i].Repo; repo != nil {
			repos = append(repos, repo)
		}
	}
	return repos
}

func (p *Panel) confineEnabledRepos(ctx context.Context) {
	if p.packageFamily() != "apt" {
		return
	}
	for _, repo := range catalogueRepos() {
		var st transport.RepoStatusResponse
		if err := p.callAgentContext(ctx, "Agent.RepoStatus", &enableRepoReq{RepoID: repo.ID}, &st); err != nil {
			continue
		}
		if !st.Enabled || st.Confinement == nil || st.Confinement.State != "not_written" || st.Confinement.Reason != "" {
			continue
		}
		for attempt := 1; attempt <= repoConfinementRetryAttempts; attempt++ {
			response, err := p.confineRepo(ctx, repo.ID)
			if err != nil && startupWorkDeferredForBusyHost(err) && attempt < repoConfinementRetryAttempts {
				time.Sleep(repoConfinementRetryInterval)
				continue
			}
			p.auditRepoConfinement(ctx, nil, repo.ID, response, err)
			break
		}
	}
}

// confineRepo runs Agent.ConfineRepoV1 in its own repo_confine job.
var confineRepoThroughAgent = func(p *Panel, ctx context.Context, repoID string) (transport.RepoStatusResponse, error) {
	var response transport.RepoStatusResponse
	callCtx, cancel := context.WithTimeout(ctx, repoConfinementCallBound)
	defer cancel()
	err := p.withStandaloneAgentMutation(callCtx, "repo_confine", repoID, "", func(callCtx context.Context, binding agentMutationBinding) error {
		request := &enableRepoReq{ServiceMutationBinding: binding, RepoID: repoID}
		if err := p.callAgentContext(callCtx, "Agent.ConfineRepoV1", request, &response); err != nil {
			return err
		}
		if response.Error != "" {
			return errors.New(response.Error)
		}
		return nil
	})
	return response, err
}

func (p *Panel) confineRepo(ctx context.Context, repoID string) (transport.RepoStatusResponse, error) {
	return confineRepoThroughAgent(p, ctx, repoID)
}

// auditRepoConfinement records what the confinement did; with no request it
// is the panel's own action at its first start on this release.
func (p *Panel) auditRepoConfinement(ctx context.Context, r *http.Request, repoID string, response transport.RepoStatusResponse, err error) {
	state := ""
	if response.Confinement != nil {
		state = response.Confinement.State
		if response.Confinement.Reason != "" {
			state += " (" + response.Confinement.Reason + ")"
		}
	}
	action := repoConfinementActionUpgrade + ":" + repoID + " — " + state
	if err != nil {
		action = repoConfinementActionUpgrade + ".failed:" + repoID + " — " + auditReason(err.Error())
	}
	details := map[string]any{"repository": repoID, "state": state}
	if response.Confinement != nil {
		details["kept_total"] = response.Confinement.KeptTotal
		details["pin_path"] = response.Confinement.PinPath
		details["owner_files"] = response.Confinement.OwnerFiles
	}
	if r != nil {
		p.auditDetailed(r, action, "repo", 0, details)
		return
	}
	details["by"] = "panel_upgrade"
	p.auditSystem(ctx, action, "repo", details)
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
