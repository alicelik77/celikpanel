package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.107 (D-038 §6): a new Node.js application runs as the site's own
// user; one made before keeps the shared web user until it is moved, on
// request, with proof, and put back when it does not answer.

func seedNodeSite(t *testing.T, panel *Panel, userID int, appRunAs any) {
	t.Helper()
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO subscriptions (id, owner_id, name, max_databases, status) VALUES (9101, ?, 'Apps', 1, 'active')`, []any{userID}},
		{`INSERT INTO domains (id, subscription_id, name, status) VALUES (9102, 9101, 'app.example', 'active')`, nil},
		{`INSERT INTO sites (id, domain_id, document_root, project_type, app_port, start_command, runtime_version, status, app_run_as)
		VALUES (9103, 9102, '/var/www/celikpanel/subscriptions/9101/sites/9102/public_html', 'node', 3001, 'node server.js', '22.14.0', 'active', ?)`, []any{appRunAs}},
	} {
		if _, err := panel.db.GetDB().Exec(statement.sql, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
}

func TestAnApplicationMadeBeforeKeepsTheWebUserAndANewOneRunsAsTheSite(t *testing.T) {
	old := hostingRuntimeState{DomainName: "app.example"}
	if got := hostingAppAccount(old); got != "www-data" {
		t.Fatalf("before = %q", got)
	}
	old.AppRunAs.String, old.AppRunAs.Valid = "site", true
	if got := hostingAppAccount(old); got != "app_example" {
		t.Fatalf("site = %q", got)
	}
}

func stubAppAccountMove(t *testing.T, proof func() error) *[]string {
	t.Helper()
	restoreMove, restoreApply, restoreProve := moveAppFilesOwnerThroughAgent, applyHostingAppForAccount, proveAppAnswers
	t.Cleanup(func() {
		moveAppFilesOwnerThroughAgent, applyHostingAppForAccount, proveAppAnswers = restoreMove, restoreApply, restoreProve
	})
	var steps []string
	moveAppFilesOwnerThroughAgent = func(_ *Panel, _ context.Context, request transport.AppFilesOwnerRequest) (transport.AppFilesOwnerResponse, error) {
		if request.Restore {
			steps = append(steps, "restore files")
			return transport.AppFilesOwnerResponse{Changed: 3}, nil
		}
		steps = append(steps, "move files "+request.FromUser+"->"+request.ToUser)
		return transport.AppFilesOwnerResponse{Changed: 3}, nil
	}
	applyHostingAppForAccount = func(_ *Panel, _ context.Context, state hostingRuntimeState) error {
		steps = append(steps, "unit as "+hostingAppAccount(state))
		return nil
	}
	proveAppAnswers = func(context.Context, int) error {
		steps = append(steps, "prove")
		return proof()
	}
	return &steps
}

func TestAnApplicationIsMovedToTheSitesUserWithProof(t *testing.T) {
	panel, _, database, userID := newServiceStateAtomicPanel(t)
	seedNodeSite(t, panel, userID, nil)
	steps := stubAppAccountMove(t, func() error { return nil })
	recorder := httptest.NewRecorder()
	panel.handleDomainAppAccount(recorder, serviceStateAtomicRequest(http.MethodGet, "/api/v1/domains/9102/app/account", "", userID), 9102)
	if !strings.Contains(recorder.Body.String(), `"runs_as":"web"`) {
		t.Fatalf("before: %s", recorder.Body.String())
	}
	recorder = httptest.NewRecorder()
	panel.handleDomainAppAccount(recorder, serviceStateAtomicRequest(http.MethodPost, "/api/v1/domains/9102/app/account", "{}", userID), 9102)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"runs_as":"site"`) {
		t.Fatalf("move: %d %s", recorder.Code, recorder.Body.String())
	}
	if got := strings.Join(*steps, " | "); got != "move files www-data->app_example | unit as app_example | prove" {
		t.Fatalf("steps = %s", got)
	}
	var runAs string
	database.GetDB().QueryRow(`SELECT app_run_as FROM sites WHERE id = 9103`).Scan(&runAs)
	if runAs != "site" {
		t.Fatalf("app_run_as = %q", runAs)
	}
}

func TestAnApplicationThatDoesNotAnswerIsPutBack(t *testing.T) {
	panel, _, database, userID := newServiceStateAtomicPanel(t)
	seedNodeSite(t, panel, userID, nil)
	steps := stubAppAccountMove(t, func() error { return errors.New("connection refused") })
	recorder := httptest.NewRecorder()
	panel.handleDomainAppAccount(recorder, serviceStateAtomicRequest(http.MethodPost, "/api/v1/domains/9102/app/account", "{}", userID), 9102)
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), `"rolled_back":true`) {
		t.Fatalf("put back: %d %s", recorder.Code, recorder.Body.String())
	}
	if got := strings.Join(*steps, " | "); got != "move files www-data->app_example | unit as app_example | prove | restore files | unit as www-data" {
		t.Fatalf("steps = %s", got)
	}
	var runAs any
	database.GetDB().QueryRow(`SELECT app_run_as FROM sites WHERE id = 9103`).Scan(&runAs)
	if runAs != nil {
		t.Fatalf("app_run_as = %v", runAs)
	}
}
