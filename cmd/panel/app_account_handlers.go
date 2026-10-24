package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// A Node.js application runs under its site's own user (alpha.107, D-038
// §6). New applications do from this release on; one made before runs as
// the shared web user and keeps doing so until it is moved here, one at a
// time and only on request: the files the web user owns in the application's
// directory go to the site's user (exactly those, recorded), the unit is
// rewritten with the site's user (which restarts the application), and the
// application must answer on its port; otherwise the unit and those files
// are put back and the application is restarted as it was.
//
// GET /api/v1/domains/{id}/app/account says which account the application
// runs as; POST moves it to the site's own user.
//
// Bir Node.js uygulaması sitenin kendi kullanıcısıyla çalışır; eski uygulama
// yalnız istenince, kanıtla ve geri almayla taşınır.

const appAccountProofBound = 30 * time.Second

// proveAppAnswers waits, bounded, for any HTTP answer from the application's
// port: an application that cannot read its files as the new user does not
// listen.
var proveAppAnswers = func(ctx context.Context, port int) error {
	deadline := time.Now().Add(appAccountProofBound)
	client := &http.Client{Timeout: 3 * time.Second}
	var last error
	for time.Now().Before(deadline) {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/", port), nil)
		if err != nil {
			return err
		}
		response, err := client.Do(request)
		if err == nil {
			response.Body.Close()
			return nil
		}
		last = err
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return fmt.Errorf("the application did not answer on port %d within %s: %v", port, appAccountProofBound, last)
}

var moveAppFilesOwnerThroughAgent = func(p *Panel, ctx context.Context, request transport.AppFilesOwnerRequest) (transport.AppFilesOwnerResponse, error) {
	var response transport.AppFilesOwnerResponse
	if err := p.callAgentContext(ctx, "Agent.MoveAppFilesOwnerV1", &request, &response); err != nil {
		return response, err
	}
	if response.Error != "" {
		return response, errors.New(response.Error)
	}
	return response, nil
}

var applyHostingAppForAccount = func(p *Panel, ctx context.Context, state hostingRuntimeState) error {
	return p.applyHostingApp(ctx, state, transport.SiteFileTriggerChange)
}

type appAccountView struct {
	RunsAs   string `json:"runs_as"`
	SiteUser string `json:"site_user"`
	WebUser  string `json:"web_user"`
}

type appAccountFailure struct {
	Error      string `json:"error"`
	Code       string `json:"code"`
	RolledBack bool   `json:"rolled_back"`
}

func appAccountOf(state hostingRuntimeState) string {
	if state.AppRunAs.Valid && state.AppRunAs.String == "site" {
		return "site"
	}
	return "web"
}

func (p *Panel) handleDomainAppAccount(w http.ResponseWriter, r *http.Request, domainID int) {
	w.Header().Set("Content-Type", "application/json")
	state, err := p.loadHostingRuntimeState(r.Context(), domainID)
	if errors.Is(err, errHostingSiteNotFound) {
		writeClientError(w, http.StatusNotFound, "site not found")
		return
	}
	if err != nil {
		writeServerError(w, err)
		return
	}
	if state.ProjectType != "node" {
		writeClientError(w, http.StatusConflict, "this domain is not a node project")
		return
	}
	view := appAccountView{RunsAs: appAccountOf(state), SiteUser: services.SiteUsername(state.DomainName), WebUser: "www-data"}
	if r.Method == http.MethodGet {
		json.NewEncoder(w).Encode(view)
		return
	}
	if view.RunsAs == "site" {
		json.NewEncoder(w).Encode(view)
		return
	}
	if !state.AppPort.Valid {
		writeClientError(w, http.StatusConflict, "the application has no port to prove it on")
		return
	}
	release, busy := p.beginServiceMutation(w, r)
	if busy {
		return
	}
	defer release()
	rolledBack := false
	var changed int
	err = p.withStandaloneAgentMutation(r.Context(), "app_account", fmt.Sprintf("site-%d", state.SiteID), "", func(ctx context.Context, binding agentMutationBinding) error {
		moved, err := moveAppFilesOwnerThroughAgent(p, ctx, transport.AppFilesOwnerRequest{
			ServiceMutationBinding: binding, SiteID: state.SiteID, WorkDir: state.DocumentRoot, FromUser: view.WebUser, ToUser: view.SiteUser,
		})
		if err != nil {
			return fmt.Errorf("the files could not be given to %s, so nothing was changed: %w", view.SiteUser, err)
		}
		changed = moved.Changed
		target := state
		target.AppRunAs = sql.NullString{String: "site", Valid: true}
		failure := applyHostingAppForAccount(p, ctx, target)
		if failure == nil {
			failure = proveAppAnswers(ctx, int(state.AppPort.Int64))
		}
		if failure == nil {
			_, failure = p.db.GetDB().ExecContext(ctx, `UPDATE sites SET app_run_as = 'site', updated_at = datetime('now') WHERE id = ?`, state.SiteID)
		}
		if failure == nil {
			return nil
		}
		// Put exactly the moved files back (the Agent stops the application
		// first), then the unit, which starts it as before.
		_, filesErr := moveAppFilesOwnerThroughAgent(p, ctx, transport.AppFilesOwnerRequest{ServiceMutationBinding: binding, SiteID: state.SiteID, Restore: true})
		unitErr := applyHostingAppForAccount(p, ctx, state)
		if unitErr != nil || filesErr != nil {
			return fmt.Errorf("the application did not work as %s (%v), and putting it back did not complete (unit: %v; files: %v)", view.SiteUser, failure, unitErr, filesErr)
		}
		rolledBack = true
		return fmt.Errorf("the application did not work as %s (%v); it was put back and runs as the shared web user again", view.SiteUser, failure)
	})
	details := map[string]any{"site": state.DomainName, "from": view.WebUser, "to": view.SiteUser, "files_changed": changed, "rolled_back": rolledBack}
	if err != nil {
		p.auditDetailed(r, "site.app_account.failed:"+state.DomainName+" — "+auditReason(err.Error()), "domain", domainID, details)
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(appAccountFailure{Error: err.Error(), Code: "APP_ACCOUNT_MOVE_FAILED", RolledBack: rolledBack})
		return
	}
	p.auditDetailed(r, "site.app_account:"+state.DomainName+":"+view.WebUser+"->"+view.SiteUser, "domain", domainID, details)
	view.RunsAs = "site"
	json.NewEncoder(w).Encode(view)
}
