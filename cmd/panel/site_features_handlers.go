package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Password-protected folders and free-form redirects of a site (alpha.114).
//
//	GET    /api/v1/domains/{id}/protected-dirs               the folders (no hashes)
//	POST   /api/v1/domains/{id}/protected-dirs               new folder + its first user
//	DELETE /api/v1/domains/{id}/protected-dirs?id=N          remove a folder
//	POST   /api/v1/domains/{id}/protected-dirs/users         add a user or change a password
//	DELETE /api/v1/domains/{id}/protected-dirs/users?id=N&user=NAME
//	GET    /api/v1/domains/{id}/redirects                    the redirects
//	POST   /api/v1/domains/{id}/redirects                    new redirect
//	DELETE /api/v1/domains/{id}/redirects?id=N               remove a redirect
//
// The whole state of a site's folders and redirects is read, changed in memory,
// validated by the same function the Agent uses (services.PrepareSiteFeatures),
// saved, and then the site's vhost is rendered and applied (write, nginx -t,
// reload). When the vhost is not applied, the previous state is saved back and
// the vhost re-applied, so the screen never shows what nginx does not serve.
// A vhost the server owner changed is kept (D-031): the change is then not
// applied, and the answer says so and what to do.
//
// Bir sitenin parolalı klasörleri ve yönlendirmeleri: durumun tamamı okunur,
// bellekte değiştirilir, Agent'ın kullandığı aynı işlevle doğrulanır,
// kaydedilir, sonra vhost yazılır, nginx -t ve yeniden yükleme yapılır. Vhost
// uygulanamazsa önceki durum geri yazılır. Sahibin değiştirdiği vhost korunur;
// değişiklik uygulanmaz ve yanıt bunu ve ne yapılacağını söyler.

const (
	maxSiteFeatureRequestBytes = 16 << 10
	minSiteProtectPasswordLen  = 8
	// bcrypt reads at most 72 bytes of a password; a longer one is refused
	// instead of being silently cut.
	maxSiteProtectPasswordLen = 72
	// A cost nginx can check at every request without stalling a worker for
	// long: about 50 ms of one CPU.
	siteProtectBcryptCost = 10

	// Codes the screen has words for (err.<CODE> in the siteFeatures part).
	errCodeSiteFeatureInvalid   = "SITE_FEATURE_INVALID"
	errCodeSiteFeatureDuplicate = "SITE_FEATURE_DUPLICATE"
	errCodeSiteFeatureLimit     = "SITE_FEATURE_LIMIT"
	errCodeSiteFeatureNotFound  = "SITE_FEATURE_NOT_FOUND"
	errCodeSiteFeatureLastUser  = "SITE_FEATURE_LAST_USER"
	errCodeSiteFeatureUnavail   = "SITE_FEATURE_UNAVAILABLE"
	errCodeSiteAuthFileKept     = "SITE_AUTH_FILE_KEPT"

	siteAuthFileKeptMessage = "A password file of this site was changed on the server outside CelikPanel, so CelikPanel kept it and did not apply this change. " +
		"The server administrator can move that file aside (it is in /etc/nginx/celikpanel-auth.d/), then the change can be made again."
)

// siteFeatureApplyVhost renders and applies the site's vhost (a test seam:
// component tests must not reach an Agent).
var siteFeatureApplyVhost = func(p *Panel, ctx context.Context, domainID int) error {
	return p.applyVhostForDomain(ctx, domainID)
}

type siteProtectedUserRow struct {
	ID        int
	Username  string
	Hash      string
	CreatedAt string
}

type siteProtectedDirRow struct {
	ID        int
	Path      string
	Realm     string
	CreatedAt string
	Users     []siteProtectedUserRow
}

type siteRedirectRow struct {
	ID        int
	Match     string
	Source    string
	Target    string
	Code      int
	KeepQuery bool
	CreatedAt string
}

// siteFeatureState is everything a site has of the two features.
type siteFeatureState struct {
	Dirs      []siteProtectedDirRow
	Redirects []siteRedirectRow
}

func (p *Panel) loadSiteFeatureState(ctx context.Context, siteID int) (siteFeatureState, error) {
	return loadSiteFeatureStateFrom(ctx, p.db.GetDB(), siteID)
}

type siteFeatureQuerier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func loadSiteFeatureStateFrom(ctx context.Context, db siteFeatureQuerier, siteID int) (siteFeatureState, error) {
	var state siteFeatureState
	rows, err := db.QueryContext(ctx, `
		SELECT id, path, realm, created_at FROM site_protected_dirs
		WHERE site_id = ? ORDER BY path`, siteID)
	if err != nil {
		return state, err
	}
	for rows.Next() {
		var dir siteProtectedDirRow
		if err := rows.Scan(&dir.ID, &dir.Path, &dir.Realm, &dir.CreatedAt); err != nil {
			rows.Close()
			return state, err
		}
		state.Dirs = append(state.Dirs, dir)
	}
	if err := rows.Close(); err != nil {
		return state, err
	}
	for index := range state.Dirs {
		userRows, err := db.QueryContext(ctx, `
			SELECT id, username, password_hash, created_at FROM site_protected_users
			WHERE dir_id = ? ORDER BY username`, state.Dirs[index].ID)
		if err != nil {
			return state, err
		}
		for userRows.Next() {
			var user siteProtectedUserRow
			if err := userRows.Scan(&user.ID, &user.Username, &user.Hash, &user.CreatedAt); err != nil {
				userRows.Close()
				return state, err
			}
			state.Dirs[index].Users = append(state.Dirs[index].Users, user)
		}
		if err := userRows.Close(); err != nil {
			return state, err
		}
	}
	redirectRows, err := db.QueryContext(ctx, `
		SELECT id, match_type, source_path, target_url, status_code, keep_query, created_at
		FROM site_redirects WHERE site_id = ? ORDER BY source_path, match_type`, siteID)
	if err != nil {
		return state, err
	}
	defer redirectRows.Close()
	for redirectRows.Next() {
		var redirect siteRedirectRow
		var keep int
		if err := redirectRows.Scan(&redirect.ID, &redirect.Match, &redirect.Source, &redirect.Target,
			&redirect.Code, &keep, &redirect.CreatedAt); err != nil {
			return state, err
		}
		redirect.KeepQuery = keep == 1
		state.Redirects = append(state.Redirects, redirect)
	}
	return state, redirectRows.Err()
}

// saveSiteFeatureState replaces the site's folders, users and redirects with
// the state, keeping the identities of rows that have one (a folder's identity
// is part of its password file's name).
func (p *Panel) saveSiteFeatureState(ctx context.Context, siteID int, state siteFeatureState) error {
	tx, err := p.db.GetDB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM site_protected_dirs WHERE site_id = ?`, siteID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM site_redirects WHERE site_id = ?`, siteID); err != nil {
		return err
	}
	for _, dir := range state.Dirs {
		var result sql.Result
		if dir.ID > 0 {
			result, err = tx.ExecContext(ctx, `
				INSERT INTO site_protected_dirs (id, site_id, path, realm, created_at)
				VALUES (?, ?, ?, ?, COALESCE(NULLIF(?, ''), datetime('now')))`,
				dir.ID, siteID, dir.Path, dir.Realm, dir.CreatedAt)
		} else {
			result, err = tx.ExecContext(ctx, `
				INSERT INTO site_protected_dirs (site_id, path, realm) VALUES (?, ?, ?)`,
				siteID, dir.Path, dir.Realm)
		}
		if err != nil {
			return err
		}
		dirID := dir.ID
		if dirID == 0 {
			newID, err := result.LastInsertId()
			if err != nil {
				return err
			}
			dirID = int(newID)
		}
		for _, user := range dir.Users {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO site_protected_users (dir_id, username, password_hash, created_at)
				VALUES (?, ?, ?, COALESCE(NULLIF(?, ''), datetime('now')))`,
				dirID, user.Username, user.Hash, user.CreatedAt); err != nil {
				return err
			}
		}
	}
	for _, redirect := range state.Redirects {
		keep := 0
		if redirect.KeepQuery {
			keep = 1
		}
		if redirect.ID > 0 {
			_, err = tx.ExecContext(ctx, `
				INSERT INTO site_redirects (id, site_id, match_type, source_path, target_url, status_code, keep_query, created_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, COALESCE(NULLIF(?, ''), datetime('now')))`,
				redirect.ID, siteID, redirect.Match, redirect.Source, redirect.Target, redirect.Code, keep, redirect.CreatedAt)
		} else {
			_, err = tx.ExecContext(ctx, `
				INSERT INTO site_redirects (site_id, match_type, source_path, target_url, status_code, keep_query)
				VALUES (?, ?, ?, ?, ?, ?)`,
				siteID, redirect.Match, redirect.Source, redirect.Target, redirect.Code, keep)
		}
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

// siteFeatureRequestFields turns the stored state into the Agent's request
// fields. Used by every render of the site's vhost, so a startup re-render or
// a certificate renewal can never drop a folder or a redirect.
func siteFeatureRequestFields(state siteFeatureState) ([]transport.SiteProtectedDirRequest, []transport.SiteRedirectRequest) {
	var dirs []transport.SiteProtectedDirRequest
	for _, dir := range state.Dirs {
		request := transport.SiteProtectedDirRequest{ID: dir.ID, Path: dir.Path, Realm: dir.Realm}
		for _, user := range dir.Users {
			request.Users = append(request.Users, transport.SiteProtectedUserRecord{Username: user.Username, PasswordHash: user.Hash})
		}
		dirs = append(dirs, request)
	}
	var redirects []transport.SiteRedirectRequest
	for _, redirect := range state.Redirects {
		redirects = append(redirects, transport.SiteRedirectRequest{
			Match: redirect.Match, Source: redirect.Source, Target: redirect.Target,
			Code: redirect.Code, KeepQuery: redirect.KeepQuery,
		})
	}
	return dirs, redirects
}

// validateSiteFeatureState runs the Agent's validation over a state, with a
// placeholder identity for a folder not saved yet.
func validateSiteFeatureState(projectType string, serverNames []string, domain string, state siteFeatureState) error {
	dirs := make([]services.SiteProtectedDir, 0, len(state.Dirs))
	for index, dir := range state.Dirs {
		id := dir.ID
		if id == 0 {
			id = 1<<20 + index
		}
		if len(dir.Users) == 0 {
			return fmt.Errorf("protected folder %q has no user who may open it", dir.Path)
		}
		if len(dir.Users) > services.MaxSiteProtectedUsers {
			return fmt.Errorf("a folder can have at most %d users", services.MaxSiteProtectedUsers)
		}
		dirs = append(dirs, services.SiteProtectedDir{ID: id, Path: dir.Path, Realm: dir.Realm})
	}
	redirects := make([]services.SiteRedirect, 0, len(state.Redirects))
	for _, redirect := range state.Redirects {
		redirects = append(redirects, services.SiteRedirect{
			Match: redirect.Match, Source: redirect.Source, Target: redirect.Target,
			Code: redirect.Code, KeepQuery: redirect.KeepQuery,
		})
	}
	_, _, err := services.PrepareSiteFeatures(projectType, serverNames, dirs, redirects,
		func(id int) string { return services.SiteAuthFilePath(domain, id) })
	return err
}

type siteFeatureSite struct {
	SiteID      int
	DomainName  string
	ProjectType string
	ForceHTTPS  bool
	HasHTTPS    bool
}

func (p *Panel) siteFeatureSite(ctx context.Context, domainID int) (siteFeatureSite, error) {
	var site siteFeatureSite
	var forceHTTPS, sslEnabled bool
	var certPath *string
	err := p.db.GetDB().QueryRowContext(ctx, `
		SELECT s.id, d.name, COALESCE(s.project_type, 'php'),
		       COALESCE(s.force_https, false), COALESCE(s.ssl_enabled, false), s.ssl_cert_path
		FROM sites s JOIN domains d ON d.id = s.domain_id
		WHERE s.domain_id = ?`, domainID).
		Scan(&site.SiteID, &site.DomainName, &site.ProjectType, &forceHTTPS, &sslEnabled, &certPath)
	if err != nil {
		return site, err
	}
	site.HasHTTPS = sslEnabled && certPath != nil && *certPath != ""
	site.ForceHTTPS = forceHTTPS && site.HasHTTPS
	return site, nil
}

type siteFeatureUserView struct {
	Username string `json:"username"`
}

type siteFeatureDirView struct {
	ID    int                   `json:"id"`
	Path  string                `json:"path"`
	Realm string                `json:"realm"`
	Users []siteFeatureUserView `json:"users"`
}

type siteFeatureRedirectView struct {
	ID        int    `json:"id"`
	Match     string `json:"match"`
	Source    string `json:"source"`
	Target    string `json:"target"`
	Code      int    `json:"code"`
	KeepQuery bool   `json:"keep_query"`
}

type siteFeatureView struct {
	DomainID int `json:"domain_id"`
	// ProtectAvailable is false for a site type that cannot carry protected
	// folders (proxy, Node.js, forwarding); RedirectsAvailable is false for a
	// forwarding site, which sends every address to its target.
	ProtectAvailable   bool `json:"protect_available"`
	RedirectsAvailable bool `json:"redirects_available"`
	// HTTPSEnforced says visitors always use HTTPS; when false a folder's
	// sign-in can also be used over plain HTTP.
	HTTPSEnforced bool                      `json:"https_enforced"`
	Dirs          []siteFeatureDirView      `json:"dirs"`
	Redirects     []siteFeatureRedirectView `json:"redirects"`
	MaxDirs       int                       `json:"max_dirs"`
	MaxUsers      int                       `json:"max_users"`
	MaxRedirects  int                       `json:"max_redirects"`
}

func siteFeatureViewOf(domainID int, site siteFeatureSite, state siteFeatureState) siteFeatureView {
	view := siteFeatureView{
		DomainID:           domainID,
		ProtectAvailable:   site.ProjectType == "php" || site.ProjectType == "static",
		RedirectsAvailable: site.ProjectType != "forwarding",
		HTTPSEnforced:      site.ForceHTTPS,
		Dirs:               []siteFeatureDirView{},
		Redirects:          []siteFeatureRedirectView{},
		MaxDirs:            services.MaxSiteProtectedDirs,
		MaxUsers:           services.MaxSiteProtectedUsers,
		MaxRedirects:       services.MaxSiteRedirects,
	}
	for _, dir := range state.Dirs {
		item := siteFeatureDirView{ID: dir.ID, Path: dir.Path, Realm: dir.Realm, Users: []siteFeatureUserView{}}
		for _, user := range dir.Users {
			item.Users = append(item.Users, siteFeatureUserView{Username: user.Username})
		}
		view.Dirs = append(view.Dirs, item)
	}
	for _, redirect := range state.Redirects {
		view.Redirects = append(view.Redirects, siteFeatureRedirectView{
			ID: redirect.ID, Match: redirect.Match, Source: redirect.Source, Target: redirect.Target,
			Code: redirect.Code, KeepQuery: redirect.KeepQuery,
		})
	}
	return view
}

// errSiteFeatureRejected carries a refusal the person can act on.
type errSiteFeatureRejected struct {
	status  int
	code    string
	message string
}

func (e *errSiteFeatureRejected) Error() string { return e.message }

func rejectSiteFeature(status int, code, message string) error {
	return &errSiteFeatureRejected{status: status, code: code, message: message}
}

// handleDomainSiteFeatures serves "protected-dirs", "protected-dirs/users" and
// "redirects" (kind: site-protect, site-protect-users, site-redirects).
func (p *Panel) handleDomainSiteFeatures(w http.ResponseWriter, r *http.Request, domainID int, kind string) {
	w.Header().Set("Content-Type", "application/json")
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
		state, err := p.loadSiteFeatureState(ctx, site.SiteID)
		if err != nil {
			writeServerError(w, err)
			return
		}
		_ = json.NewEncoder(w).Encode(siteFeatureViewOf(domainID, site, state))
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxSiteFeatureRequestBytes)
	var mutate func(state *siteFeatureState) (audit string, err error)
	switch {
	case kind == "site-protect" && r.Method == http.MethodPost:
		var req struct {
			Path     string `json:"path"`
			Realm    string `json:"realm"`
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if !decodeSiteFeatureBody(w, r, &req) {
			return
		}
		mutate = func(state *siteFeatureState) (string, error) {
			return addSiteProtectedDir(state, site, req.Path, req.Realm, req.Username, req.Password)
		}
	case kind == "site-protect" && r.Method == http.MethodDelete:
		id, ok := siteFeatureQueryID(w, r, "id")
		if !ok {
			return
		}
		mutate = func(state *siteFeatureState) (string, error) { return removeSiteProtectedDir(state, id) }
	case kind == "site-protect-users" && r.Method == http.MethodPost:
		var req struct {
			DirID    int    `json:"dir_id"`
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if !decodeSiteFeatureBody(w, r, &req) {
			return
		}
		mutate = func(state *siteFeatureState) (string, error) {
			return setSiteProtectedUser(state, req.DirID, req.Username, req.Password)
		}
	case kind == "site-protect-users" && r.Method == http.MethodDelete:
		id, ok := siteFeatureQueryID(w, r, "id")
		if !ok {
			return
		}
		username := r.URL.Query().Get("user")
		mutate = func(state *siteFeatureState) (string, error) { return removeSiteProtectedUser(state, id, username) }
	case kind == "site-redirects" && r.Method == http.MethodPost:
		var req struct {
			Match     string `json:"match"`
			Source    string `json:"source"`
			Target    string `json:"target"`
			Code      int    `json:"code"`
			KeepQuery bool   `json:"keep_query"`
		}
		if !decodeSiteFeatureBody(w, r, &req) {
			return
		}
		mutate = func(state *siteFeatureState) (string, error) {
			return addSiteRedirect(state, site, req.Match, req.Source, req.Target, req.Code, req.KeepQuery)
		}
	case kind == "site-redirects" && r.Method == http.MethodDelete:
		id, ok := siteFeatureQueryID(w, r, "id")
		if !ok {
			return
		}
		mutate = func(state *siteFeatureState) (string, error) { return removeSiteRedirect(state, id) }
	default:
		http.NotFound(w, r)
		return
	}

	unlock := lockDomainSSLOperation(domainID)
	defer unlock()
	opCtx, cancel := sslDurableContext(ctx)
	defer cancel()
	// The site is read again under the lock: a change of its type made
	// meanwhile decides what may be saved (the closures above read `site`).
	if site, err = p.siteFeatureSite(opCtx, domainID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeClientError(w, http.StatusNotFound, "domain site not found")
			return
		}
		writeServerError(w, err)
		return
	}
	auditAction, err := p.changeSiteFeatures(opCtx, domainID, site, mutate)
	if err != nil {
		writeSiteFeatureError(w, err)
		return
	}
	p.auditSiteFeature(r, auditAction, domainID)
	state, err := p.loadSiteFeatureState(opCtx, site.SiteID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(siteFeatureViewOf(domainID, site, state))
}

// auditSiteFeature writes one audit event of this file. Every action name is
// spelled out at its call, so the list of audit actions (audit_actions.go) can
// be checked against the source.
func (p *Panel) auditSiteFeature(r *http.Request, action string, domainID int) {
	switch action {
	case "domain.protect.add":
		p.audit(r, "domain.protect.add", "domain", domainID)
	case "domain.protect.remove":
		p.audit(r, "domain.protect.remove", "domain", domainID)
	case "domain.protect.user.add":
		p.audit(r, "domain.protect.user.add", "domain", domainID)
	case "domain.protect.user.remove":
		p.audit(r, "domain.protect.user.remove", "domain", domainID)
	case "domain.redirect.add":
		p.audit(r, "domain.redirect.add", "domain", domainID)
	case "domain.redirect.remove":
		p.audit(r, "domain.redirect.remove", "domain", domainID)
	}
}

func decodeSiteFeatureBody(w http.ResponseWriter, r *http.Request, into any) bool {
	if err := json.NewDecoder(r.Body).Decode(into); err != nil {
		writeClientError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	return true
}

func siteFeatureQueryID(w http.ResponseWriter, r *http.Request, name string) (int, bool) {
	id, err := strconv.Atoi(r.URL.Query().Get(name))
	if err != nil || id <= 0 {
		writeClientError(w, http.StatusBadRequest, "invalid "+name)
		return 0, false
	}
	return id, true
}

func writeSiteFeatureError(w http.ResponseWriter, err error) {
	var rejected *errSiteFeatureRejected
	if errors.As(err, &rejected) {
		writeCodedError(w, rejected.status, rejected.code, rejected.message, "")
		return
	}
	if strings.Contains(err.Error(), "is not CelikPanel's unchanged text") && strings.Contains(err.Error(), ".htpasswd") {
		writeCodedError(w, http.StatusConflict, errCodeSiteAuthFileKept, siteAuthFileKeptMessage, "")
		return
	}
	writeServerError(w, err)
}

// changeSiteFeatures is the one compensated operation: change the state, apply
// the vhost, and on failure put the previous state and the previous vhost back.
func (p *Panel) changeSiteFeatures(
	ctx context.Context,
	domainID int,
	site siteFeatureSite,
	mutate func(state *siteFeatureState) (string, error),
) (string, error) {
	previous, err := p.loadSiteFeatureState(ctx, site.SiteID)
	if err != nil {
		return "", err
	}
	next := cloneSiteFeatureState(previous)
	auditAction, err := mutate(&next)
	if err != nil {
		return "", err
	}
	serverNames, err := p.managedSiteHostnames(ctx, domainID)
	if err != nil {
		return "", err
	}
	if err := validateSiteFeatureState(site.ProjectType, serverNames, site.DomainName, next); err != nil {
		return "", rejectSiteFeature(http.StatusBadRequest, errCodeSiteFeatureInvalid, err.Error())
	}
	if err := p.saveSiteFeatureState(ctx, site.SiteID, next); err != nil {
		return "", err
	}
	applyErr := siteFeatureApplyVhost(p, ctx, domainID)
	if applyErr == nil {
		return auditAction, nil
	}
	rollbackCtx, cancel := sslCompensationContext()
	defer cancel()
	if restoreErr := p.saveSiteFeatureState(rollbackCtx, site.SiteID, previous); restoreErr != nil {
		return "", fmt.Errorf("apply the site's vhost: %v; restore the saved state: %w", applyErr, restoreErr)
	}
	var held *siteFileHeldError
	if !errors.As(applyErr, &held) && !strings.Contains(applyErr.Error(), "is not CelikPanel's unchanged text") {
		// The previous vhost is put back as well. A held file was never
		// changed, so there is nothing to put back.
		if restoreErr := siteFeatureApplyVhost(p, rollbackCtx, domainID); restoreErr != nil {
			return "", fmt.Errorf("apply the site's vhost: %v; restore the vhost: %w", applyErr, restoreErr)
		}
	}
	return "", applyErr
}

func cloneSiteFeatureState(state siteFeatureState) siteFeatureState {
	clone := siteFeatureState{Redirects: append([]siteRedirectRow(nil), state.Redirects...)}
	for _, dir := range state.Dirs {
		dir.Users = append([]siteProtectedUserRow(nil), dir.Users...)
		clone.Dirs = append(clone.Dirs, dir)
	}
	return clone
}

func hashSiteProtectPassword(password string) (string, error) {
	if len(password) < minSiteProtectPasswordLen {
		return "", rejectSiteFeature(http.StatusBadRequest, errCodeSiteFeatureInvalid,
			fmt.Sprintf("the password must have at least %d characters", minSiteProtectPasswordLen))
	}
	if len(password) > maxSiteProtectPasswordLen {
		return "", rejectSiteFeature(http.StatusBadRequest, errCodeSiteFeatureInvalid,
			fmt.Sprintf("the password must have at most %d characters", maxSiteProtectPasswordLen))
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), siteProtectBcryptCost)
	if err != nil {
		return "", err
	}
	// $2y$ is what the htpasswd tool writes and what the system's crypt()
	// reads; Go writes $2a$, the same algorithm.
	return "$2y$" + strings.TrimPrefix(string(hash), "$2a$"), nil
}

func requireProtectAvailable(site siteFeatureSite) error {
	if site.ProjectType != "php" && site.ProjectType != "static" {
		return rejectSiteFeature(http.StatusConflict, errCodeSiteFeatureUnavail,
			"Password-protected folders are available for PHP and static sites.")
	}
	return nil
}

func addSiteProtectedDir(state *siteFeatureState, site siteFeatureSite, rawPath, rawRealm, username, password string) (string, error) {
	if err := requireProtectAvailable(site); err != nil {
		return "", err
	}
	path, err := services.NormalizeSitePath(rawPath, false)
	if err != nil {
		return "", rejectSiteFeature(http.StatusBadRequest, errCodeSiteFeatureInvalid, err.Error())
	}
	realm, err := services.NormalizeSiteRealm(rawRealm)
	if err != nil {
		return "", rejectSiteFeature(http.StatusBadRequest, errCodeSiteFeatureInvalid, err.Error())
	}
	if err := services.ValidateSiteProtectedUsername(username); err != nil {
		return "", rejectSiteFeature(http.StatusBadRequest, errCodeSiteFeatureInvalid, err.Error())
	}
	for _, dir := range state.Dirs {
		if dir.Path == path {
			return "", rejectSiteFeature(http.StatusConflict, errCodeSiteFeatureDuplicate, "This folder is already protected.")
		}
	}
	if len(state.Dirs) >= services.MaxSiteProtectedDirs {
		return "", rejectSiteFeature(http.StatusConflict, errCodeSiteFeatureLimit,
			fmt.Sprintf("A site can have at most %d protected folders.", services.MaxSiteProtectedDirs))
	}
	hash, err := hashSiteProtectPassword(password)
	if err != nil {
		return "", err
	}
	state.Dirs = append(state.Dirs, siteProtectedDirRow{
		Path: path, Realm: realm, Users: []siteProtectedUserRow{{Username: username, Hash: hash}},
	})
	return "domain.protect.add", nil
}

func removeSiteProtectedDir(state *siteFeatureState, id int) (string, error) {
	for index, dir := range state.Dirs {
		if dir.ID == id {
			state.Dirs = append(state.Dirs[:index], state.Dirs[index+1:]...)
			return "domain.protect.remove", nil
		}
	}
	return "", rejectSiteFeature(http.StatusNotFound, errCodeSiteFeatureNotFound, "This folder is not protected (any more).")
}

func setSiteProtectedUser(state *siteFeatureState, dirID int, username, password string) (string, error) {
	if err := services.ValidateSiteProtectedUsername(username); err != nil {
		return "", rejectSiteFeature(http.StatusBadRequest, errCodeSiteFeatureInvalid, err.Error())
	}
	for index := range state.Dirs {
		dir := &state.Dirs[index]
		if dir.ID != dirID {
			continue
		}
		hash, err := hashSiteProtectPassword(password)
		if err != nil {
			return "", err
		}
		for userIndex := range dir.Users {
			if dir.Users[userIndex].Username == username {
				dir.Users[userIndex].Hash = hash
				return "domain.protect.user.add", nil
			}
		}
		if len(dir.Users) >= services.MaxSiteProtectedUsers {
			return "", rejectSiteFeature(http.StatusConflict, errCodeSiteFeatureLimit,
				fmt.Sprintf("A folder can have at most %d users.", services.MaxSiteProtectedUsers))
		}
		dir.Users = append(dir.Users, siteProtectedUserRow{Username: username, Hash: hash})
		return "domain.protect.user.add", nil
	}
	return "", rejectSiteFeature(http.StatusNotFound, errCodeSiteFeatureNotFound, "This folder is not protected (any more).")
}

func removeSiteProtectedUser(state *siteFeatureState, dirID int, username string) (string, error) {
	for index := range state.Dirs {
		dir := &state.Dirs[index]
		if dir.ID != dirID {
			continue
		}
		for userIndex, user := range dir.Users {
			if user.Username != username {
				continue
			}
			if len(dir.Users) == 1 {
				return "", rejectSiteFeature(http.StatusConflict, errCodeSiteFeatureLastUser,
					"This is the last user of the folder. Remove the folder's protection instead.")
			}
			dir.Users = append(dir.Users[:userIndex], dir.Users[userIndex+1:]...)
			return "domain.protect.user.remove", nil
		}
		return "", rejectSiteFeature(http.StatusNotFound, errCodeSiteFeatureNotFound, "This user does not exist (any more).")
	}
	return "", rejectSiteFeature(http.StatusNotFound, errCodeSiteFeatureNotFound, "This folder is not protected (any more).")
}

func addSiteRedirect(state *siteFeatureState, site siteFeatureSite, match, source, target string, code int, keepQuery bool) (string, error) {
	if site.ProjectType == "forwarding" {
		return "", rejectSiteFeature(http.StatusConflict, errCodeSiteFeatureUnavail,
			"A forwarding site sends every address to its target, so it has no redirects of its own.")
	}
	if len(state.Redirects) >= services.MaxSiteRedirects {
		return "", rejectSiteFeature(http.StatusConflict, errCodeSiteFeatureLimit,
			fmt.Sprintf("A site can have at most %d redirects.", services.MaxSiteRedirects))
	}
	// Validated here for the message; the same function runs again over the
	// whole state, and at the Agent.
	normalized, err := services.NormalizeRedirect(services.SiteRedirect{
		Match: match, Source: source, Target: target, Code: code, KeepQuery: keepQuery,
	}, nil)
	if err != nil {
		return "", rejectSiteFeature(http.StatusBadRequest, errCodeSiteFeatureInvalid, err.Error())
	}
	for _, redirect := range state.Redirects {
		if redirect.Match == normalized.Match && redirect.Source == normalized.Source {
			return "", rejectSiteFeature(http.StatusConflict, errCodeSiteFeatureDuplicate, "A redirect for this path already exists. Remove it first.")
		}
	}
	state.Redirects = append(state.Redirects, siteRedirectRow{
		Match: normalized.Match, Source: normalized.Source, Target: normalized.Target,
		Code: normalized.Code, KeepQuery: normalized.KeepQuery,
	})
	return "domain.redirect.add", nil
}

func removeSiteRedirect(state *siteFeatureState, id int) (string, error) {
	for index, redirect := range state.Redirects {
		if redirect.ID == id {
			state.Redirects = append(state.Redirects[:index], state.Redirects[index+1:]...)
			return "domain.redirect.remove", nil
		}
	}
	return "", rejectSiteFeature(http.StatusNotFound, errCodeSiteFeatureNotFound, "This redirect does not exist (any more).")
}
