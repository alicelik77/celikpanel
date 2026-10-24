package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.107 review: confinement is the owner's action, shown first. The
// plan is read without changing anything; a confine without the confirmed
// text is refused before anything runs.

func TestTheConfinementPlanIsShownAndAConfineNeedsTheConfirmedText(t *testing.T) {
	panel, _, _, userID := newServiceStateAtomicPanel(t)
	restore := readRepoConfinePreview
	t.Cleanup(func() { readRepoConfinePreview = restore })
	readRepoConfinePreview = func(_ *Panel, _ context.Context, repoID string) (transport.RepoConfinePreview, error) {
		return transport.RepoConfinePreview{State: "not_written", Text: "Package: *\n", SHA256: strings.Repeat("a", 64), Kept: []string{"php8.5-fpm"},
			Changes: []transport.RepoConfineChange{{Package: "php-common", From: "2:96", Before: "2:102+sury", BeforeVendor: true}}}, nil
	}
	recorder := httptest.NewRecorder()
	panel.handleRepoConfinePreview(recorder, serviceStateAtomicRequest(http.MethodGet, repoConfinePreviewAPIPath+"?service_id=php-fpm", "", userID))
	if body := recorder.Body.String(); !strings.Contains(body, `"state":"read"`) || !strings.Contains(body, `"package":"php-common"`) {
		t.Fatalf("preview = %d %s", recorder.Code, body)
	}
	called := false
	restoreConfine := confineRepoThroughAgent
	t.Cleanup(func() { confineRepoThroughAgent = restoreConfine })
	confineRepoThroughAgent = func(*Panel, context.Context, string, string) (transport.RepoStatusResponse, error) {
		called = true
		return transport.RepoStatusResponse{}, nil
	}
	recorder = httptest.NewRecorder()
	panel.handleRepo(recorder, serviceStateAtomicRequest(http.MethodPost, "/api/v1/repo", `{"service_id":"php-fpm","action":"confine"}`, userID))
	if recorder.Code != http.StatusBadRequest || called {
		t.Fatalf("unconfirmed confine = %d %s called=%v", recorder.Code, recorder.Body.String(), called)
	}
}
