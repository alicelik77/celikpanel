package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Web server settings of a site (alpha.128, PLESK-GAP G3; SITE-SETTINGS-DESIGN
// §2.4; transport/site_web_settings.go).
//
//	GET  /api/v1/domains/{id}/web-settings             the fields, the directives, who may change them
//	POST /api/v1/domains/{id}/web-settings             save the fields
//	POST /api/v1/domains/{id}/web-settings/directives  save the additional nginx directives
//	GET  /api/v1/nginx/directives-offer                (administrator) offered to resellers and customers?
//	PUT  /api/v1/nginx/directives-offer                (administrator) offer them, or stop
//
// A save is one compensated operation, like the protected folders: the row is
// saved, the site's vhost is rendered with it and applied by the Agent (the
// two files, nginx -t, reload), and when that is not applied the previous row
// is saved back and the previous files rendered again. The save's own steps
// are reported for the progress window: check → save → web_server.
//
// Three outcomes are kept apart (D-024):
//   - verified failure: nginx refused; the previous files are back; the
//     first line of nginx's answer is shown (WEB_SETTINGS_NGINX_REFUSED);
//   - unmet prerequisite: the server's nginx configuration already failed
//     its check before the change (WEB_SETTINGS_NGINX_FAILING), the site's
//     file was changed by the server owner (WEB_SETTINGS_FILE_KEPT), or the
//     permission is closed (WEB_DIRECTIVES_CLOSED);
//   - unknown result: the Agent did not answer; the screen reads the
//     settings again and never sends the save a second time.
//
// Additional directives: the administrator always; a reseller or customer
// only when the administrator offered them (the opt-in permission offer of
// rights_gate.go, off by default)
// and the subscription's rights hold nginx_directives. The audit names the
// text's SHA-256, never the text.
//
// Bir sitenin web sunucusu ayarları: kayıt, vhost üretimi ve Agent
// uygulaması tek telafili işlemdir; uygulanmazsa önceki kayıt ve dosyalar
// geri konur. Ek yönergeler: yönetici her zaman; bayi ve müşteri yalnız
// yönetici sunduysa ve planda nginx_directives varsa.

const (
	nginxDirectivesOfferPath = "/api/v1/nginx/directives-offer"

	errCodeWebSettingInvalid     = "WEB_SETTING_INVALID"
	errCodeWebDirectivesInvalid  = "WEB_DIRECTIVES_INVALID"
	errCodeWebDirectivesClosed   = "WEB_DIRECTIVES_CLOSED"
	errCodeWebSettingsKept       = "WEB_SETTINGS_FILE_KEPT"
	errCodeWebSettingsNotRead    = "WEB_SETTINGS_NOT_READ"
	errCodeWebSettingsNginx      = "WEB_SETTINGS_NGINX_REFUSED"
	errCodeWebSettingsNginxFails = "WEB_SETTINGS_NGINX_FAILING"
	errCodeWebSettingsUnavail    = "WEB_SETTINGS_UNAVAILABLE"
	errCodeWebErrorPageMissing   = "WEB_ERROR_PAGE_MISSING"
	errCodeWebPageCacheOff       = "WEB_PAGE_CACHE_OFF"
	errCodeWebPageCacheClear     = "WEB_PAGE_CACHE_CLEAR"

	maxWebSettingsRequestBytes = 64 << 10
)

// requestIdentityRouteWebSettings: a save of the web server settings writes
// nginx files and reloads nginx. Guarded when the page sends an identity, so
// it outlives a reload, never runs twice, and the progress window follows
// its steps (operation_steps.go).
var requestIdentityRouteWebSettings = requestIdentityRoute{
	pattern: "/api/v1/domains/{id}/web-settings", optional: true,
	timeout: agentRPCMutationTimeout + 2*time.Minute,
}

// siteWebSettingsApplyRender renders and applies the site's vhost with its
// saved web settings (a test seam: component tests must not reach an Agent).
// precheck asks the Agent to run nginx -t before writing.
var siteWebSettingsApplyRender = func(p *Panel, ctx context.Context, domainID int, precheck bool) (*transport.SiteFileResult, error) {
	req, err := p.buildVhostRequest(ctx, domainID, nil)
	if err != nil {
		return nil, err
	}
	if precheck && req.WebSettings != nil {
		req.WebSettings.PrecheckNginx = true
	}
	return p.applySiteFileRender(ctx, req, transport.SiteFileTriggerChange)
}

// siteWebSettingsRow is the stored row of a site.
type siteWebSettingsRow struct {
	Exists            bool
	Settings          transport.SiteWebSettings
	Directives        string
	DirectivesByAdmin bool
}

type siteWebQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func loadSiteWebSettings(ctx context.Context, db siteWebQuerier, siteID int) (siteWebSettingsRow, error) {
	var row siteWebSettingsRow
	var raw string
	var byAdmin int
	err := db.QueryRowContext(ctx, `SELECT settings_json, directives, directives_by_admin FROM site_web_settings WHERE site_id = ?`, siteID).
		Scan(&raw, &row.Directives, &byAdmin)
	if errors.Is(err, sql.ErrNoRows) {
		return siteWebSettingsRow{}, nil
	}
	if err != nil {
		return row, err
	}
	row.Exists = true
	row.DirectivesByAdmin = byAdmin == 1
	if strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &row.Settings); err != nil {
			return row, fmt.Errorf("read the site's web server settings: %w", err)
		}
	}
	return row, nil
}

func (p *Panel) saveSiteWebSettings(ctx context.Context, siteID int, row siteWebSettingsRow) error {
	db := p.db.GetDB()
	if !row.Exists {
		_, err := db.ExecContext(ctx, `DELETE FROM site_web_settings WHERE site_id = ?`, siteID)
		return err
	}
	raw, err := json.Marshal(row.Settings)
	if err != nil {
		return err
	}
	byAdmin := 0
	if row.DirectivesByAdmin {
		byAdmin = 1
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO site_web_settings (site_id, settings_json, directives, directives_by_admin, directives_sha256, updated_at)
		VALUES (?, ?, ?, ?, ?, datetime('now'))
		ON CONFLICT(site_id) DO UPDATE SET settings_json = excluded.settings_json, directives = excluded.directives,
			directives_by_admin = excluded.directives_by_admin, directives_sha256 = excluded.directives_sha256,
			updated_at = excluded.updated_at`,
		siteID, string(raw), row.Directives, byAdmin, directivesDigest(row.Directives))
	return err
}

func directivesDigest(text string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// request is the vhost render's part (nil for a site without a row).
func (row siteWebSettingsRow) request() *transport.SiteWebSettingsRequest {
	if !row.Exists {
		return nil
	}
	return &transport.SiteWebSettingsRequest{
		Settings: row.Settings, Directives: row.Directives, DirectivesTrusted: row.DirectivesByAdmin,
	}
}

// siteWebRequestFor is buildVhostRequest's part: every render of the site
// carries its web settings, so no flow that re-renders can drop them.
func (p *Panel) siteWebRequestFor(ctx context.Context, siteID int) (*transport.SiteWebSettingsRequest, error) {
	row, err := loadSiteWebSettings(ctx, p.db.GetDB(), siteID)
	if err != nil {
		return nil, err
	}
	return row.request(), nil
}

// directivesAccess says whether the caller may change the site's
// additional directives, and if not, why and who can open them.
type directivesAccess struct {
	MayEdit bool   `json:"may_edit"`
	Reason  string `json:"reason,omitempty"` // not_offered, not_in_plan
	Grantor string `json:"grantor,omitempty"`
	// Offered is the server's offer switch (the administrator only).
	Offered *bool `json:"offered,omitempty"`
}

// nginxDirectivesOffered reads the administrator's offer from the one store
// of opt-in permissions (rights_gate.go, admin_offered_permissions); an
// unreadable store is "not offered".
func (p *Panel) nginxDirectivesOffered(ctx context.Context) bool {
	offered, err := p.loadOfferedOptInPermissions(ctx)
	return err == nil && offered[rightNginxDirectives]
}

func (p *Panel) siteDirectivesAccess(ctx context.Context, r *http.Request, domainID int) (directivesAccess, error) {
	if callerIsAdministrator(r) {
		offered := p.nginxDirectivesOffered(ctx)
		return directivesAccess{MayEdit: true, Offered: &offered}, nil
	}
	if caller := currentCaller(r); caller != nil && caller.isAdditionalUser() {
		// An additional user never changes them (the route is not mapped).
		return directivesAccess{Reason: "not_in_plan", Grantor: "the subscription's owner"}, nil
	}
	if !p.nginxDirectivesOffered(ctx) {
		return directivesAccess{Reason: "not_offered", Grantor: "the administrator"}, nil
	}
	subID, err := p.domainSubscriptionID(ctx, domainID)
	if err != nil {
		return directivesAccess{}, err
	}
	e, err := p.effectiveSubscriptionRights(ctx, subID)
	if err != nil {
		return directivesAccess{}, err
	}
	if layer := e.permissionRefusal(rightNginxDirectives); layer != "" {
		return directivesAccess{Reason: "not_in_plan", Grantor: e.grantor(layer)}, nil
	}
	return directivesAccess{MayEdit: true}, nil
}

type siteWebSettingsView struct {
	DomainID    int    `json:"domain_id"`
	ProjectType string `json:"project_type"`
	// Fields that apply to the site's type.
	IndexApplies bool                      `json:"index_applies"`
	FieldsApply  bool                      `json:"fields_apply"`
	NodeApplies  bool                      `json:"node_applies"`
	DefaultIndex string                    `json:"default_index,omitempty"`
	Saved        bool                      `json:"saved"`
	Settings     transport.SiteWebSettings `json:"settings"`
	CacheChoices []int                     `json:"cache_choices"`
	ErrorCodes   []int                     `json:"error_codes"`
	// PageCacheApplies: the site's type can have a page cache (G28);
	// MayCache: the caller may turn it on or off (the administrator).
	PageCacheApplies bool                  `json:"page_cache_applies"`
	MayCache         bool                  `json:"may_cache"`
	HTTPSEnforced    bool                  `json:"https_enforced"`
	Directives       siteWebDirectivesView `json:"directives"`
}

type siteWebDirectivesView struct {
	directivesAccess
	// Set: the site has additional directives. Text only for a caller who
	// may change them.
	Set     bool   `json:"set"`
	Text    string `json:"text,omitempty"`
	ByAdmin bool   `json:"by_admin,omitempty"`
	MaxSize int    `json:"max_size"`
}

func (p *Panel) siteWebSettingsViewOf(ctx context.Context, r *http.Request, domainID int, site siteFeatureSite, row siteWebSettingsRow) (siteWebSettingsView, error) {
	index, anyField, node := services.SiteWebFieldsApply(site.ProjectType)
	view := siteWebSettingsView{
		DomainID: domainID, ProjectType: site.ProjectType,
		IndexApplies: index, FieldsApply: anyField, NodeApplies: node,
		Saved: row.Exists, Settings: row.Settings, CacheChoices: services.BrowserCacheChoices, ErrorCodes: services.SiteErrorCodes(),
		HTTPSEnforced: site.ForceHTTPS,
	}
	if index {
		view.DefaultIndex = services.DefaultIndexFiles(site.ProjectType)
	}
	view.PageCacheApplies = services.PageCacheApplies(site.ProjectType)
	view.MayCache = callerIsAdministrator(r)
	access, err := p.siteDirectivesAccess(ctx, r, domainID)
	if err != nil {
		return view, err
	}
	view.Directives = siteWebDirectivesView{
		directivesAccess: access, Set: strings.TrimSpace(row.Directives) != "",
		ByAdmin: row.DirectivesByAdmin, MaxSize: services.MaxSiteDirectivesBytes,
	}
	if access.MayEdit {
		view.Directives.Text = row.Directives
	}
	return view, nil
}

// handleDomainWebSettings serves "web-settings" and "web-settings-directives".
func (p *Panel) handleDomainWebSettings(w http.ResponseWriter, r *http.Request, domainID int, kind string) {
	ctx := r.Context()
	site, err := p.siteFeatureSite(ctx, domainID)
	if errors.Is(err, sql.ErrNoRows) {
		writeClientError(w, http.StatusNotFound, "domain site not found")
		return
	}
	if err != nil {
		writeServerError(w, err)
		return
	}
	if r.Method == http.MethodGet {
		row, err := loadSiteWebSettings(ctx, p.db.GetDB(), site.SiteID)
		if err != nil {
			writeServerError(w, err)
			return
		}
		view, err := p.siteWebSettingsViewOf(ctx, r, domainID, site, row)
		if err != nil {
			writeServerError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(view)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxWebSettingsRequestBytes)
	reportOperationPlan(ctx, "check", "save", "web_server")
	reportOperationStep(ctx, "check")
	var mutate func(row *siteWebSettingsRow) error
	audit := ""
	details := map[string]any{}
	switch kind {
	case "web-settings":
		var req struct {
			Settings transport.SiteWebSettings `json:"settings"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeClientError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		admin := callerIsAdministrator(r)
		if !admin {
			// G28: the page cache is the administrator's; anyone else's
			// save keeps what the site has.
			if current, err := loadSiteWebSettings(ctx, p.db.GetDB(), site.SiteID); err == nil {
				req.Settings.PageCache, req.Settings.PageCacheMinutes = current.Settings.PageCache, current.Settings.PageCacheMinutes
			}
		}
		prepared, err := services.PrepareSiteWebSettings(site.ProjectType, req.Settings)
		if err != nil {
			writeWebSettingsRefusal(w, err)
			return
		}
		mutate = func(row *siteWebSettingsRow) error {
			if !admin {
				// Read again under the lock: the administrator may have
				// changed it meanwhile.
				prepared.PageCache, prepared.PageCacheMinutes = row.Settings.PageCache, row.Settings.PageCacheMinutes
			}
			row.Settings = prepared
			return nil
		}
		audit = "domain.web.settings"
		details["browser_cache"] = services.BrowserCacheLabel(prepared.BrowserCacheDays)
		details["headers"] = len(prepared.Headers)
		details["access"] = prepared.AccessMode
		details["page_cache"] = prepared.PageCache
	case "web-settings-directives":
		var req struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeClientError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if _, anyField, _ := services.SiteWebFieldsApply(site.ProjectType); !anyField {
			writeCodedError(w, http.StatusConflict, errCodeWebSettingsUnavail,
				"This site serves no content of its own (forwarding), so it has no nginx directives. Nothing was changed.", "")
			return
		}
		access, err := p.siteDirectivesAccess(ctx, r, domainID)
		if err != nil {
			writeServerError(w, err)
			return
		}
		if !access.MayEdit {
			writeDirectivesClosed(w, access)
			return
		}
		text := strings.ReplaceAll(req.Text, "\r\n", "\n")
		trusted := callerIsAdministrator(r)
		var home string
		if !trusted {
			home, err = p.siteHomeOf(ctx, domainID)
			if err != nil {
				writeServerError(w, err)
				return
			}
		}
		if strings.TrimSpace(text) != "" {
			if err := services.ValidateSiteDirectives(text, trusted, home); err != nil {
				writeDirectivesRefusal(w, err)
				return
			}
		}
		mutate = func(row *siteWebSettingsRow) error {
			row.Directives = text
			row.DirectivesByAdmin = trusted && strings.TrimSpace(text) != ""
			return nil
		}
		audit = "domain.web.directives"
		details["sha256"] = directivesDigest(text)
		details["bytes"] = len(text)
	default:
		http.NotFound(w, r)
		return
	}

	unlock := lockDomainSSLOperation(domainID)
	defer unlock()
	opCtx, cancel := sslDurableContext(ctx)
	defer cancel()
	if site, err = p.siteFeatureSite(opCtx, domainID); err != nil {
		writeServerError(w, err)
		return
	}
	reportOperationStep(ctx, "save")
	if err := p.changeSiteWebSettings(opCtx, ctx, domainID, site, mutate); err != nil {
		writeWebSettingsApplyError(w, r, err)
		return
	}
	p.auditWebSettings(r, audit, domainID, details)
	row, err := loadSiteWebSettings(opCtx, p.db.GetDB(), site.SiteID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	view, err := p.siteWebSettingsViewOf(opCtx, r, domainID, site, row)
	if err != nil {
		writeServerError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(view)
}

// siteHomeOf is the site's home (the parent of its document root).
func (p *Panel) siteHomeOf(ctx context.Context, domainID int) (string, error) {
	var docroot string
	if err := p.db.GetDB().QueryRowContext(ctx, `SELECT document_root FROM sites WHERE domain_id = ?`, domainID).Scan(&docroot); err != nil {
		return "", err
	}
	return services.SiteHomeOf(docroot), nil
}

// auditWebSettings writes one audit event; every action name is spelled out
// at its call (audit_actions.go).
func (p *Panel) auditWebSettings(r *http.Request, action string, domainID int, details map[string]any) {
	switch action {
	case "domain.web.settings":
		p.auditDetailed(r, "domain.web.settings", "domain", domainID, details)
	case "domain.web.directives":
		p.auditDetailed(r, "domain.web.directives", "domain", domainID, details)
	}
}

// errWebSettingsApply carries what the Agent said about the site's files.
type errWebSettingsApply struct {
	status  int
	code    string
	message string
	cause   error
}

func (e *errWebSettingsApply) Error() string { return e.message }
func (e *errWebSettingsApply) Unwrap() error { return e.cause }

// changeSiteWebSettings is the one compensated operation.
func (p *Panel) changeSiteWebSettings(ctx, stepCtx context.Context, domainID int, site siteFeatureSite, mutate func(row *siteWebSettingsRow) error) error {
	previous, err := loadSiteWebSettings(ctx, p.db.GetDB(), site.SiteID)
	if err != nil {
		return err
	}
	next := previous
	next.Exists = true
	if err := mutate(&next); err != nil {
		return err
	}
	if err := p.saveSiteWebSettings(ctx, site.SiteID, next); err != nil {
		return err
	}
	reportOperationStep(stepCtx, "web_server")
	result, applyErr := siteWebSettingsApplyRender(p, ctx, domainID, true)
	if applyErr == nil {
		applyErr = siteWebPartsRefusal(site.DomainName, result)
	}
	if applyErr == nil {
		return nil
	}
	rollbackCtx, cancel := sslCompensationContext()
	defer cancel()
	if restoreErr := p.saveSiteWebSettings(rollbackCtx, site.SiteID, previous); restoreErr != nil {
		return fmt.Errorf("apply the site's web server settings: %v; restore the saved settings: %w", applyErr, restoreErr)
	}
	// A kept vhost or kept file was never changed, and a refused check put
	// the previous files back itself; anything else is rendered again with
	// the previous settings.
	var held *siteFileHeldError
	var refused *errWebSettingsApply
	if !errors.As(applyErr, &held) && !errors.As(applyErr, &refused) {
		_, _ = siteWebSettingsApplyRender(p, rollbackCtx, domainID, false)
	}
	return classifyWebSettingsApply(applyErr)
}

// siteWebPartsRefusal reads the Agent's outcome of the site's files.
func siteWebPartsRefusal(domain string, result *transport.SiteFileResult) error {
	if result == nil || result.SiteParts == nil {
		return nil
	}
	for _, name := range []string{services.SiteSettingsPartName, services.SiteDirectivesPartName} {
		switch result.SiteParts[name] {
		case transport.SiteFileChallengeKept:
			return &errWebSettingsApply{status: http.StatusConflict, code: errCodeWebSettingsKept,
				message: "The file " + services.SitePartPath(domain, name) + " was changed on the server outside CelikPanel, so CelikPanel kept it and did not apply this change. " +
					"The server administrator can compare it in Hosting → Configuration, or move the file aside; then save again."}
		case transport.SitePartNotRead:
			return &errWebSettingsApply{status: http.StatusConflict, code: errCodeWebSettingsNotRead,
				message: "The site's nginx file was changed by the server owner and does not have the line " + services.SiteIncludeLine(domain) +
					" , so these settings would not be read. Nothing was changed. The administrator can add that line, or take CelikPanel's file (Hosting → Configuration); then save again."}
		case transport.SiteFileChallengeFailed:
			return &errWebSettingsApply{status: http.StatusBadGateway, code: errCodeWebSettingsNginx,
				message: "The server could not write the site's web server settings, so the previous settings stay. " + strings.TrimSpace(result.Detail)}
		}
	}
	return nil
}

// classifyWebSettingsApply turns the Agent's error into the screen's code.
func classifyWebSettingsApply(err error) error {
	var known *errWebSettingsApply
	if errors.As(err, &known) {
		return err
	}
	text := err.Error()
	if rest, ok := strings.CutPrefix(text, "ERROR_PAGE_MISSING: "); ok {
		return &errWebSettingsApply{status: http.StatusConflict, code: errCodeWebErrorPageMissing, cause: err,
			message: "These error pages are not in the site's folder: " + firstLineOf(rest) +
				". Upload them (File manager or SFTP) to the site's document root first; nothing was changed."}
	}
	if rest, ok := strings.CutPrefix(text, "NGINX_FAILING_BEFORE: "); ok {
		return &errWebSettingsApply{status: http.StatusConflict, code: errCodeWebSettingsNginxFails, cause: err,
			message: "The server's nginx configuration already fails its check, so nothing was written. The server administrator must fix it first (nginx -t says: " +
				firstLineOf(rest) + "); then save again."}
	}
	var held *siteFileHeldError
	if errors.As(err, &held) && held.File.Reason == transport.SiteFileReasonNginxRefused {
		return &errWebSettingsApply{status: http.StatusBadRequest, code: errCodeWebSettingsNginx, cause: err,
			message: "nginx refused this change, so the previous settings stay and the site works as before. nginx said: " + firstLineOf(held.File.Detail)}
	}
	if strings.Contains(text, "nginx validation failed") {
		return &errWebSettingsApply{status: http.StatusBadRequest, code: errCodeWebSettingsNginx, cause: err,
			message: "nginx refused this change, so the previous settings stay and the site works as before. nginx said: " + firstLineOf(lastAfter(text, "nginx validation failed: "))}
	}
	return err
}

func firstLineOf(text string) string {
	text = strings.TrimSpace(text)
	if line, _, ok := strings.Cut(text, "\n"); ok {
		text = line
	}
	if len(text) > 300 {
		text = text[:300] + "…"
	}
	return text
}

func lastAfter(text, marker string) string {
	if i := strings.LastIndex(text, marker); i >= 0 {
		return text[i+len(marker):]
	}
	return text
}

func writeWebSettingsRefusal(w http.ResponseWriter, err error) {
	var refusal *services.SiteWebSettingsRefusal
	if errors.As(err, &refusal) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(apiErrorBody{Error: "Nothing was changed: " + refusal.Message + ".", Code: errCodeWebSettingInvalid, Reason: refusal.Field})
		return
	}
	writeCodedError(w, http.StatusBadRequest, errCodeWebSettingInvalid, "Nothing was changed: "+err.Error()+".", "")
}

func writeDirectivesRefusal(w http.ResponseWriter, err error) {
	var refusal *services.SiteDirectiveRefusal
	if errors.As(err, &refusal) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(apiErrorBody{Error: "Nothing was changed: " + refusal.Message + ".", Code: errCodeWebDirectivesInvalid, Reason: refusal.Kind})
		return
	}
	writeCodedError(w, http.StatusBadRequest, errCodeWebDirectivesInvalid, "Nothing was changed: "+err.Error()+".", "")
}

func writeDirectivesClosed(w http.ResponseWriter, access directivesAccess) {
	message := "Additional nginx directives are not offered on this server. The administrator can offer them to resellers and customers; nothing was changed."
	if access.Reason == "not_in_plan" {
		message = "Your plan does not include additional nginx directives. Ask " + access.Grantor + " to add them; nothing was changed."
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	_ = json.NewEncoder(w).Encode(apiErrorBody{Error: message, Code: errCodeWebDirectivesClosed, Reason: access.Reason})
}

func writeWebSettingsApplyError(w http.ResponseWriter, r *http.Request, err error) {
	var known *errWebSettingsApply
	if errors.As(err, &known) {
		writeCodedError(w, known.status, known.code, known.message, "")
		return
	}
	writeServerErrorFor(w, r, err)
}

// handleNginxDirectivesOffer: the administrator's switch "resellers and
// customers may be given additional nginx directives" (off by default).
func (p *Panel) handleNginxDirectivesOffer(w http.ResponseWriter, r *http.Request) {
	if !callerIsAdministrator(r) {
		writeClientError(w, http.StatusForbidden, "administrator only")
		return
	}
	switch r.Method {
	case http.MethodGet:
	case http.MethodPut:
		var req struct {
			Offered bool `json:"offered"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1<<10)
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeClientError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		// The same store as the Rights page's offer of opt-in permissions.
		offered, err := p.loadOfferedOptInPermissions(r.Context())
		if err != nil {
			writeServerError(w, err)
			return
		}
		offered[rightNginxDirectives] = req.Offered
		if err := p.saveOfferedOptInPermissions(r.Context(), offered); err != nil {
			writeServerError(w, err)
			return
		}
		p.auditDetailed(r, "nginx.directives.offer", "settings", 0, map[string]any{"offered": req.Offered})
	default:
		w.Header().Set("Allow", "GET, PUT")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"offered": p.nginxDirectivesOffered(r.Context())})
}
