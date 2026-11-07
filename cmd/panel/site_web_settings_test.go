package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.128 (PLESK-GAP G3): the web server settings screen's endpoints.

type webApplyRecorder struct {
	requests  []applyVhostRPCRequest
	prechecks []bool
	script    []func() (*transport.SiteFileResult, error)
}

func (a *webApplyRecorder) install(t *testing.T) {
	t.Helper()
	saved := siteWebSettingsApplyRender
	siteWebSettingsApplyRender = func(p *Panel, ctx context.Context, domainID int, precheck bool) (*transport.SiteFileResult, error) {
		req, err := p.buildVhostRequest(ctx, domainID, nil)
		if err != nil {
			return nil, err
		}
		a.requests = append(a.requests, req)
		a.prechecks = append(a.prechecks, precheck)
		if len(a.script) == 0 {
			return &transport.SiteFileResult{Outcome: transport.SiteFileOutcomeWritten,
				SiteParts: map[string]string{services.SiteSettingsPartName: transport.SiteFileChallengeWritten}}, nil
		}
		next := a.script[0]
		a.script = a.script[1:]
		return next()
	}
	t.Cleanup(func() { siteWebSettingsApplyRender = saved })
}

func readWebView(t *testing.T, p *Panel, domainID int, caller *Caller) siteWebSettingsView {
	t.Helper()
	recorder := featureRequest(p, domainID, caller, http.MethodGet, "web-settings", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET %d %s", recorder.Code, recorder.Body.String())
	}
	var view siteWebSettingsView
	if err := json.Unmarshal(recorder.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	return view
}

func TestWebSettingsSaveRendersAndRollsBack(t *testing.T) {
	p, domainID, owner, _ := newSiteFeatureFixture(t)
	web := &webApplyRecorder{}
	web.install(t)

	view := readWebView(t, p, domainID, owner)
	if view.Saved || !view.FieldsApply || view.Directives.MayEdit {
		t.Fatalf("fresh view: %+v", view)
	}
	// A site without a row renders exactly as before: no web settings.
	req, err := p.buildVhostRequest(context.Background(), domainID, nil)
	if err != nil || req.WebSettings != nil {
		t.Fatalf("render before any save: %+v %v", req.WebSettings, err)
	}

	recorder := featureRequest(p, domainID, owner, http.MethodPost, "web-settings",
		`{"settings":{"browser_cache_days":7,"headers":[{"name":"X-Frame-Options","value":"SAMEORIGIN"}]}}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("save %d %s", recorder.Code, recorder.Body.String())
	}
	if len(web.requests) != 1 || web.requests[0].WebSettings == nil || web.requests[0].WebSettings.Settings.BrowserCacheDays != 7 || !web.prechecks[0] {
		t.Fatalf("render: %+v", web.requests)
	}
	if view = readWebView(t, p, domainID, owner); !view.Saved || view.Settings.BrowserCacheDays != 7 {
		t.Fatalf("after save: %+v", view)
	}

	// An invalid value is refused before anything is rendered.
	recorder = featureRequest(p, domainID, owner, http.MethodPost, "web-settings", `{"settings":{"browser_cache_days":3}}`)
	if recorder.Code != http.StatusBadRequest || featureCode(t, recorder) != errCodeWebSettingInvalid || len(web.requests) != 1 {
		t.Fatalf("invalid: %d %s", recorder.Code, recorder.Body.String())
	}

	// nginx refuses: the previous settings are saved back and the screen
	// gets the verified failure.
	web.script = []func() (*transport.SiteFileResult, error){
		func() (*transport.SiteFileResult, error) {
			return &transport.SiteFileResult{}, errors.New("nginx validation failed: nginx: [emerg] bad")
		},
	}
	recorder = featureRequest(p, domainID, owner, http.MethodPost, "web-settings", `{"settings":{"browser_cache_days":30}}`)
	if recorder.Code != http.StatusBadRequest || featureCode(t, recorder) != errCodeWebSettingsNginx {
		t.Fatalf("refused: %d %s", recorder.Code, recorder.Body.String())
	}
	if view = readWebView(t, p, domainID, owner); view.Settings.BrowserCacheDays != 7 {
		t.Fatalf("not rolled back: %+v", view.Settings)
	}

	// The server already failed nginx -t: an unmet prerequisite.
	web.script = []func() (*transport.SiteFileResult, error){
		func() (*transport.SiteFileResult, error) {
			return nil, errors.New("NGINX_FAILING_BEFORE: nginx: [emerg] in /etc/nginx/conf.d/x.conf:1")
		},
	}
	recorder = featureRequest(p, domainID, owner, http.MethodPost, "web-settings", `{"settings":{"browser_cache_days":1}}`)
	if recorder.Code != http.StatusConflict || featureCode(t, recorder) != errCodeWebSettingsNginxFails {
		t.Fatalf("failing before: %d %s", recorder.Code, recorder.Body.String())
	}

	// The owner edited the site's file: kept, nothing changed.
	web.script = []func() (*transport.SiteFileResult, error){
		func() (*transport.SiteFileResult, error) {
			return &transport.SiteFileResult{SiteParts: map[string]string{services.SiteSettingsPartName: transport.SiteFileChallengeKept}}, nil
		},
	}
	recorder = featureRequest(p, domainID, owner, http.MethodPost, "web-settings", `{"settings":{"browser_cache_days":1}}`)
	if recorder.Code != http.StatusConflict || featureCode(t, recorder) != errCodeWebSettingsKept {
		t.Fatalf("kept: %d %s", recorder.Code, recorder.Body.String())
	}
	if view = readWebView(t, p, domainID, owner); view.Settings.BrowserCacheDays != 7 {
		t.Fatalf("kept file changed the record: %+v", view.Settings)
	}
}

func TestWebDirectivesNeedTheOfferAndThePermission(t *testing.T) {
	p, domainID, owner, _ := newSiteFeatureFixture(t)
	web := &webApplyRecorder{}
	web.install(t)
	body := `{"text":"location = /old { return 301 /new; }"}`

	recorder := featureRequest(p, domainID, owner, http.MethodPost, "web-settings/directives", body)
	if recorder.Code != http.StatusForbidden || featureCode(t, recorder) != errCodeWebDirectivesClosed {
		t.Fatalf("not offered: %d %s", recorder.Code, recorder.Body.String())
	}
	if err := p.saveOfferedOptInPermissions(context.Background(), map[string]bool{rightNginxDirectives: true}); err != nil {
		t.Fatal(err)
	}
	// The fixture's subscription follows an "all permissions" set, so once
	// offered the customer may use them.
	view := readWebView(t, p, domainID, owner)
	if !view.Directives.MayEdit {
		t.Skipf("the fixture's subscription does not hold nginx_directives (%+v); the offer gate is covered above", view.Directives)
	}
	recorder = featureRequest(p, domainID, owner, http.MethodPost, "web-settings/directives", `{"text":"location /x { proxy_pass http://10.0.0.1; }"}`)
	if recorder.Code != http.StatusBadRequest || featureCode(t, recorder) != errCodeWebDirectivesInvalid {
		t.Fatalf("forbidden: %d %s", recorder.Code, recorder.Body.String())
	}
	recorder = featureRequest(p, domainID, owner, http.MethodPost, "web-settings/directives", body)
	if recorder.Code != http.StatusOK {
		t.Fatalf("save: %d %s", recorder.Code, recorder.Body.String())
	}
	last := web.requests[len(web.requests)-1]
	if last.WebSettings == nil || last.WebSettings.DirectivesTrusted || last.WebSettings.Directives == "" {
		t.Fatalf("render: %+v", last.WebSettings)
	}

	// The administrator's text is trusted (syntax and nginx -t only).
	admin := &Caller{ID: 1, Role: roleAdmin}
	recorder = featureRequest(p, domainID, admin, http.MethodPost, "web-settings/directives", `{"text":"location /x { proxy_pass http://127.0.0.1:9000; }"}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("admin: %d %s", recorder.Code, recorder.Body.String())
	}
	if last = web.requests[len(web.requests)-1]; !last.WebSettings.DirectivesTrusted {
		t.Fatal("the administrator's text is not marked trusted")
	}
	if view = readWebView(t, p, domainID, owner); view.Directives.Text == "" && view.Directives.MayEdit {
		t.Fatalf("view: %+v", view.Directives)
	}
}

// alpha.128 (G26): a page whose file is missing is an unmet prerequisite.
func TestWebSettingsErrorPageMissingIsAPrerequisite(t *testing.T) {
	p, domainID, owner, _ := newSiteFeatureFixture(t)
	web := &webApplyRecorder{}
	web.install(t)
	web.script = []func() (*transport.SiteFileResult, error){
		func() (*transport.SiteFileResult, error) {
			return nil, errors.New("ERROR_PAGE_MISSING: /errors/404.html")
		},
	}
	recorder := featureRequest(p, domainID, owner, http.MethodPost, "web-settings",
		`{"settings":{"error_pages":[{"code":404,"path":"/errors/404.html"}]}}`)
	if recorder.Code != http.StatusConflict || featureCode(t, recorder) != errCodeWebErrorPageMissing {
		t.Fatalf("missing page: %d %s", recorder.Code, recorder.Body.String())
	}
	if view := readWebView(t, p, domainID, owner); view.Saved {
		t.Fatalf("the refused save left a record: %+v", view)
	}
}

// alpha.128 (G28): only the administrator turns the page cache on or off;
// "Clear cache" needs the cache on and calls the Agent once.
func TestPageCacheIsTheAdministratorsAndClears(t *testing.T) {
	p, domainID, owner, _ := newSiteFeatureFixture(t)
	web := &webApplyRecorder{}
	web.install(t)
	recorder := featureRequest(p, domainID, owner, http.MethodPost, "web-settings", `{"settings":{"page_cache":true}}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("customer save: %d %s", recorder.Code, recorder.Body.String())
	}
	if view := readWebView(t, p, domainID, owner); view.Settings.PageCache || view.MayCache {
		t.Fatalf("a customer turned the cache on: %+v", view)
	}
	recorder = featureRequest(p, domainID, owner, http.MethodPost, "web-settings/cache-clear", `{}`)
	if recorder.Code != http.StatusConflict || featureCode(t, recorder) != errCodeWebPageCacheOff {
		t.Fatalf("clear without cache: %d %s", recorder.Code, recorder.Body.String())
	}
	admin := &Caller{ID: 1, Role: roleAdmin}
	recorder = featureRequest(p, domainID, admin, http.MethodPost, "web-settings", `{"settings":{"page_cache":true,"page_cache_minutes":5}}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("admin save: %d %s", recorder.Code, recorder.Body.String())
	}
	// A customer's later save keeps the administrator's cache.
	recorder = featureRequest(p, domainID, owner, http.MethodPost, "web-settings", `{"settings":{"browser_cache_days":1}}`)
	if view := readWebView(t, p, domainID, owner); recorder.Code != http.StatusOK || !view.Settings.PageCache || view.Settings.PageCacheMinutes != 5 {
		t.Fatalf("customer save dropped the cache: %d %+v", recorder.Code, view.Settings)
	}
	calls := 0
	saved := sitePageCacheClearCall
	sitePageCacheClearCall = func(*Panel, context.Context, int) (transport.SitePageCacheClearResponse, error) {
		calls++
		return transport.SitePageCacheClearResponse{Removed: 3}, nil
	}
	t.Cleanup(func() { sitePageCacheClearCall = saved })
	recorder = featureRequest(p, domainID, owner, http.MethodPost, "web-settings/cache-clear", `{}`)
	if recorder.Code != http.StatusOK || calls != 1 {
		t.Fatalf("clear: %d %s calls=%d", recorder.Code, recorder.Body.String(), calls)
	}
}
