package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/appcatalog"
	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// The application catalog of a domain (alpha.118, docs/APP-CATALOG.md).
//
// Install is one durable operation (app_catalog_operations, migration 058):
// the row is written before any privileged step and every step's outcome is
// recorded before the next one starts, like WordPress's installer. The
// order: gates (permission, the administrator's offer, the database engine,
// limits), prerequisites read from the server (PHP version, PHP extensions
// with the package that brings each one, the empty document root, disk),
// the database and its user (counted against the limits), the verified
// files, then the application's own setup as the site's user. A setup that
// fails leaves the site closed with a "being set up" page and the operation
// in setup_failed; Retry runs the setup again (it detects a finished setup)
// and never repeats an earlier step. Reading the state never changes
// anything. The generated administrator password is sealed until it was
// shown once; it is never logged.
//
// Uygulama kataloğu: kurulum tek kalıcı işlemdir; her adımın sonucu bir
// sonraki başlamadan kaydedilir. Kurulum adımı başarısız olursa site
// "kuruluyor" sayfasıyla kapalı kalır ve Yeniden dene yalnız kurulumu yeniden
// çalıştırır. Durum okumak hiçbir şeyi değiştirmez.

// Coded refusals the web translates (err.<CODE>).
const (
	errCodeAppUnknown           = "APP_UNKNOWN"
	errCodeAppAlreadyInstalled  = "APP_ALREADY_INSTALLED"
	errCodeAppNeedsPHPSite      = "APP_NEEDS_PHP_SITE"
	errCodeAppPHPUnsupported    = "APP_PHP_UNSUPPORTED"
	errCodeAppExtensionsMissing = "APP_EXTENSIONS_MISSING"
	errCodeAppPrereqUnknown     = "APP_PREREQUISITES_UNKNOWN"
	errCodeAppDocRootNotEmpty   = "APP_DOCROOT_NOT_EMPTY"
	errCodeAppDiskRoom          = "APP_DISK_ROOM"
	errCodeAppNoDatabaseServer  = "APP_NO_DATABASE_SERVER"
	errCodeAppVerification      = "APP_VERIFICATION_FAILED"
	errCodeAppUnpack            = "APP_UNPACK_FAILED"
	errCodeAppRemoveIncomplete  = "APP_REMOVE_INCOMPLETE"
	errCodeAppDownload          = "APP_DOWNLOAD_FAILED"
	errCodeAppSetupFailed       = "APP_SETUP_FAILED"
	errCodeAppInstallFailed     = "APP_INSTALL_FAILED"
	errCodeAppOperationState    = "APP_OPERATION_STATE"
	errCodeAppRemoveConfirm     = "APP_REMOVE_CONFIRMATION"
)

// Operation states (migration 058).
const (
	appOpReserved         = "reserved"
	appOpDatabaseCreating = "database_creating"
	appOpDatabaseReady    = "database_ready"
	appOpFilesInstalling  = "files_installing"
	appOpFilesReady       = "files_ready"
	appOpSettingUp        = "setting_up"
	appOpSetupFailed      = "setup_failed"
	appOpApplied          = "applied"
	appOpRemoving         = "removing"
	appOpRemoved          = "removed"
	appOpFailed           = "failed"
	appOpNeedsReview      = "needs_review"
)

// appCatalogEntryView is one catalog application on a screen.
type appCatalogEntryView struct {
	ID          string   `json:"id"`
	Kind        string   `json:"kind"`
	OfferingID  string   `json:"offering_id"`
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	License     string   `json:"license"`
	Homepage    string   `json:"homepage"`
	Description string   `json:"description"`
	Source      string   `json:"source"`
	Pinned      bool     `json:"pinned"`
	Checked     string   `json:"checked"`
	PHP         string   `json:"php"`
	Extensions  []string `json:"extensions"`
	Databases   []string `json:"databases"`
	DiskMB      int64    `json:"disk_mb"`
	Risk        string   `json:"risk"`
	// TR is the Turkish description, check and risk note (alpha.130); the
	// English fields above stay as they were.
	TR        appCatalogTextView `json:"tr"`
	AdminUser string             `json:"admin_user,omitempty"`
	// Offered: the administrator offers it on this server.
	Offered bool `json:"offered"`
	// Legacy fields of the WordPress-only catalog (alpha.117 and earlier).
	Icon        string `json:"icon"`
	RequiresDB  bool   `json:"requires_db"`
	RequiresPHP bool   `json:"requires_php"`
}

// appCatalogTextView is one language's copy of an entry's reader texts.
type appCatalogTextView struct {
	Description string `json:"description"`
	Checked     string `json:"checked"`
	Risk        string `json:"risk"`
}

func catalogEntryView(app appcatalog.App, offered bool) appCatalogEntryView {
	return appCatalogEntryView{
		ID: app.ID, Kind: app.Kind, OfferingID: appcatalog.OfferingID(app.ID), Name: app.Name,
		Version: app.Version, License: app.License, Homepage: app.Homepage, Description: app.Summary,
		Source: app.Archive.URL, Pinned: app.Archive.SHA256 != "", Checked: app.Archive.Upstream,
		PHP: app.PHPRange(), Extensions: append([]string{}, app.Extensions...),
		Databases: append([]string{}, app.Databases...), DiskMB: app.MinDiskMB, Risk: app.Risk,
		TR:        appCatalogTextView{Description: app.TR.Summary, Checked: app.TR.Upstream, Risk: app.TR.Risk},
		AdminUser: app.AdminUser, Offered: offered,
		Icon: app.ID, RequiresDB: len(app.Databases) > 0, RequiresPHP: true,
	}
}

// handleAppCatalog: GET /api/v1/apps. The administrator sees every catalog
// application with whether it is offered; everyone else sees what the
// administrator offers (a subscription's own narrowing is on the domain's
// list).
func (p *Panel) handleAppCatalog(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, no-store")
	offered, err := p.loadOfferedApps(r.Context())
	if err != nil {
		writeServerError(w, err)
		return
	}
	hidden, err := p.loadHiddenOfferings(r.Context())
	if err != nil {
		hidden = map[string]bool{}
	}
	admin := callerIsAdministrator(r)
	out := []appCatalogEntryView{}
	for _, app := range appcatalog.All() {
		if !app.Offerable() {
			continue
		}
		on := offered[app.ID] && !hidden[appcatalog.OfferingID(app.ID)]
		if !admin && !on {
			continue
		}
		out = append(out, catalogEntryView(app, on))
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"apps": out, "administrator": admin})
}

// ---- the domain's list ----

type appRequirementView struct {
	// State: ok, unmet (a named prerequisite), unknown (could not be read).
	State          string                          `json:"state"`
	PHPOK          bool                            `json:"php_ok"`
	PHPReason      string                          `json:"php_reason,omitempty"`
	Missing        []transport.AppMissingExtension `json:"missing,omitempty"`
	DocRoot        string                          `json:"doc_root,omitempty"`
	DatabaseOK     bool                            `json:"database_ok"`
	DatabaseReason string                          `json:"database_reason,omitempty"`
	DiskOK         bool                            `json:"disk_ok"`
	DiskReason     string                          `json:"disk_reason,omitempty"`
	Family         string                          `json:"family,omitempty"`
	PHPLine        string                          `json:"php_line,omitempty"`
	// InstallPackage is the Components install (PHP-FPM of the site's PHP
	// version) whose standard extension packages bring every missing
	// extension ("" when no single Components install does).
	InstallPackage string `json:"install_package,omitempty"`
	Detail         string `json:"detail,omitempty"`
	// Reason refines "unknown" (appPrereqFailure): busy, refused, or empty
	// when the agent could not be reached.
	Reason string `json:"reason,omitempty"`
}

// Why the read-only prerequisite check had no answer (alpha.118 lab round 2).
// The check takes no host lease and changes nothing, so it is never refused
// for other work; a busy server shows as an answer that did not come in time.
const (
	appPrereqBusy    = "busy"
	appPrereqRefused = "refused"
)

// appPrereqFailure names the reason and the English message of a failed
// prerequisite check: what happened, who acts, what next, how it resumes.
func appPrereqFailure(app appcatalog.App, err error, resp transport.AppPrerequisitesResponse) (string, string) {
	switch {
	case err == nil:
		// The agent answered but refused the request itself.
		return appPrereqRefused, "the server's agent refused the check whether this site is ready for " + app.Name + " (" + boundedPrereqError(resp.Error) + "); nothing was changed. Try again; if it repeats, the administrator sends this message to CelikPanel support."
	case errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled):
		return appPrereqBusy, "the server did not answer the check for " + app.Name + " in time, most likely because other work is running on it; nothing was changed. Nobody needs to fix anything: wait a minute or two until that work has finished, then install again."
	default:
		return "", "the server could not be asked whether it is ready for " + app.Name + "; nothing was changed. Try again in a moment; if it persists, the administrator checks that the CelikPanel agent is running."
	}
}

func boundedPrereqError(text string) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) > 200 {
		return string(runes[:200]) + "…"
	}
	return string(runes)
}

// catalogExtensionsInstallPackage names the Components install that brings
// every missing extension of a site's PHP from the official packages: on
// Debian and Ubuntu, installing PHP-FPM of that version installs its standard
// extension packages (php8.4-mysql, -curl, -gd, ...), from the distribution
// when it is the distribution's own version (alpha.118). "" when one of the
// missing packages is not among them, or on Arch Linux (one PHP whose
// extensions are switched on in its configuration).
// Eksik uzantıların hepsini getiren Bileşenler kurulumu (PHP-FPM sürümü).
func catalogExtensionsInstallPackage(family, phpLine string, missing []transport.AppMissingExtension) string {
	if family != "apt" || len(missing) == 0 || !appPHPLinePattern.MatchString(phpLine) {
		return ""
	}
	php := core.GetManagedServiceByID("php-fpm")
	if php == nil || php.Repo == nil {
		return ""
	}
	companions := map[string]bool{}
	for _, template := range php.Repo.VersionCompanions {
		companions[strings.ReplaceAll(template, "{v}", "php"+phpLine)] = true
	}
	for _, m := range missing {
		if m.Package == "" || !companions[m.Package] {
			return ""
		}
	}
	return "php" + phpLine + "-fpm"
}

var appPHPLinePattern = regexp.MustCompile(`^[0-9]+\.[0-9]+$`)

type domainAppView struct {
	appCatalogEntryView
	Available    bool                `json:"available"`
	RefusedBy    string              `json:"refused_by,omitempty"`
	Grantor      string              `json:"grantor,omitempty"`
	Requirements *appRequirementView `json:"requirements,omitempty"`
}

type installedAppView struct {
	OperationID        string `json:"operation_id"`
	AppID              string `json:"app_id"`
	Name               string `json:"name"`
	Version            string `json:"version"`
	Status             string `json:"status"`
	SiteURL            string `json:"site_url"`
	AdminURL           string `json:"admin_url,omitempty"`
	AdminUser          string `json:"admin_user,omitempty"`
	DatabaseName       string `json:"database_name,omitempty"`
	DataDir            string `json:"data_dir,omitempty"`
	ErrorKind          string `json:"error_kind,omitempty"`
	LastError          string `json:"last_error,omitempty"`
	CredentialsPending bool   `json:"credentials_pending,omitempty"`
	CanRetry           bool   `json:"can_retry,omitempty"`
	CanRemove          bool   `json:"can_remove,omitempty"`
	Legacy             bool   `json:"legacy,omitempty"`
	UpdatedAt          string `json:"updated_at"`
}

// handleDomainAppCatalog: GET /api/v1/domains/{id}/apps. Read only: the
// prerequisites are asked of the server, nothing is changed.
func (p *Panel) handleDomainAppCatalog(w http.ResponseWriter, r *http.Request, domainID int) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, no-store")
	ctx := r.Context()
	target, err := p.resolveAppInstallTarget(ctx, domainID)
	if errors.Is(err, sql.ErrNoRows) {
		writeClientError(w, http.StatusNotFound, "site not found")
		return
	}
	if err != nil {
		writeServerError(w, err)
		return
	}
	e := p.loadEffectiveForRequest(w, r, target.SubscriptionID)
	if e == nil {
		return
	}
	installed, err := p.domainInstalledApps(ctx, domainID, target.Domain)
	if err != nil {
		writeServerError(w, err)
		return
	}
	site, err := p.appSiteFacts(ctx, target)
	if err != nil {
		writeServerError(w, err)
		return
	}
	occupied := false
	for _, app := range installed {
		if app.Status != appOpFailed && app.Status != appOpRemoved {
			occupied = true
		}
	}
	apps := []domainAppView{}
	var pending []appRequirementJob
	for _, app := range appcatalog.All() {
		if app.Kind != appcatalog.KindPHPSite || !app.Offerable() {
			continue
		}
		offering := appcatalog.OfferingID(app.ID)
		layer := e.offeringRefusal(offering)
		if callerIsAdministrator(r) && layer != rightsLayerAdministrator {
			layer = ""
		}
		if layer == rightsLayerAdministrator && !callerIsAdministrator(r) {
			continue // not offered on this server: not shown (P6, the server also refuses)
		}
		view := domainAppView{appCatalogEntryView: catalogEntryView(app, layer != rightsLayerAdministrator), Available: layer == ""}
		if layer != "" {
			view.RefusedBy, view.Grantor = layer, e.grantor(layer)
		} else if target.ProjectType == "php" && !occupied {
			pending = append(pending, appRequirementJob{index: len(apps), app: app})
		}
		apps = append(apps, view)
	}
	// Each application's prerequisites are asked at the same time, at most
	// appRequirementWorkers at once (alpha.127); they used to be asked one
	// after another (7 Agent calls, each starting PHP).
	// Uygulamalarin on kosullari ayni anda sorulur.
	jobs := make(chan appRequirementJob)
	var wg sync.WaitGroup
	for w := 0; w < min(appRequirementWorkers, len(pending)); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				apps[job.index].Requirements = p.appRequirements(ctx, r, e, job.app, target, site)
			}
		}()
	}
	for _, job := range pending {
		jobs <- job
	}
	close(jobs)
	wg.Wait()
	_ = json.NewEncoder(w).Encode(map[string]any{
		"apps": apps, "installed": installed,
		"site": map[string]any{"php_version": site.phpVersion, "https": site.https, "project_type": target.ProjectType, "occupied": occupied,
			"administrator": callerIsAdministrator(r)},
	})
}

// appRequirementWorkers bounds the prerequisite reads of one catalog page
// that run at the same time.
const appRequirementWorkers = 4

type appRequirementJob struct {
	index int
	app   appcatalog.App
}

type appSiteFacts struct {
	phpVersion string
	https      bool
}

func (p *Panel) appSiteFacts(ctx context.Context, target appInstallTarget) (appSiteFacts, error) {
	var facts appSiteFacts
	var php sql.NullString
	var ssl sql.NullInt64
	err := p.db.GetDB().QueryRowContext(ctx, `SELECT php_version, ssl_enabled FROM sites WHERE id = ?`, target.SiteID).Scan(&php, &ssl)
	if err != nil {
		return facts, err
	}
	facts.phpVersion, facts.https = strings.TrimSpace(php.String), ssl.Int64 == 1
	return facts, nil
}

func appSiteIdentity(target appInstallTarget, domainID int, php string) transport.AppSiteIdentity {
	return transport.AppSiteIdentity{
		SiteID: target.SiteID, SubscriptionID: target.SubscriptionID, DomainID: domainID,
		Domain: target.Domain, Username: services.SiteUsername(target.Domain), PHPVersion: php,
	}
}

// appRequirements reads one application's prerequisites (nothing changes).
func (p *Panel) appRequirements(ctx context.Context, r *http.Request, e *effectiveRights, app appcatalog.App, target appInstallTarget, site appSiteFacts) *appRequirementView {
	view := &appRequirementView{State: "ok", DatabaseOK: true, DiskOK: true}
	if !callerIsAdministrator(r) {
		if layer := e.anyVersionRefusal("component:mariadb"); layer != "" {
			view.DatabaseOK = false
			view.DatabaseReason = "MariaDB is not offered to this subscription; ask " + e.grantor(layer) + " to include it"
		}
	}
	if err := p.checkSubscriptionQuota(ctx, target.SubscriptionID, quotaDBs); err != nil {
		view.DatabaseOK = false
		view.DatabaseReason = err.Error()
	}
	if err := p.appDiskRoom(ctx, target.SubscriptionID, app.MinDiskMB); err != nil {
		view.DiskOK = false
		view.DiskReason = err.Error()
	}
	// The Agent's answer is used for 30 s and read again after any change
	// (display_checks.go, keep < 0: never shown once older). Only an answer
	// without error is kept.
	request := transport.AppPrerequisitesRequest{
		AppID: app.ID, Site: appSiteIdentity(target, target.DomainID, site.phpVersion),
	}
	key := fmt.Sprintf("%s|%+v", app.ID, request.Site)
	resp, _, err := p.displayChecks.ready().appPrereq.get(ctx, key,
		func(ctx context.Context) (transport.AppPrerequisitesResponse, error) {
			var resp transport.AppPrerequisitesResponse
			callCtx, cancel := context.WithTimeout(ctx, 40*time.Second)
			defer cancel()
			if err := p.callAgentContext(callCtx, "Agent.AppPrerequisitesV1", &request, &resp); err != nil {
				return resp, err
			}
			if resp.Error != "" {
				return resp, &appPrereqAnswerError{resp: resp}
			}
			return resp, nil
		})
	var answerErr *appPrereqAnswerError
	if errors.As(err, &answerErr) {
		resp, err = answerErr.resp, nil
	}
	if err != nil || resp.Error != "" {
		view.State = "unknown"
		view.Reason, view.Detail = appPrereqFailure(app, err, resp)
		view.PHPOK, view.PHPReason = app.PHPSupported(site.phpVersion)
		return view
	}
	if app.Legacy() {
		// WordPress's own installer keeps its document-root rules (it keeps
		// the placeholder page); its PHP and extensions are checked like
		// every other application's (alpha.118 lab: no mysqli, HTTP 500).
		resp.DocRoot = ""
	}
	view.Family, view.PHPOK, view.PHPReason, view.Missing, view.DocRoot = resp.Family, resp.PHPOK, resp.PHPReason, resp.Missing, resp.DocRoot
	view.PHPLine = resp.PHPLine
	view.InstallPackage = catalogExtensionsInstallPackage(resp.Family, resp.PHPLine, resp.Missing)
	if resp.FreeDiskMB > 0 && resp.FreeDiskMB < app.MinDiskMB*2 {
		view.DiskOK = false
		view.DiskReason = fmt.Sprintf("the server has %d MB free where sites live; %s needs about %d MB", resp.FreeDiskMB, app.Name, app.MinDiskMB*2)
	}
	switch {
	case !resp.PHPOK || len(resp.Missing) > 0 || !view.DatabaseOK || !view.DiskOK ||
		resp.DocRoot == transport.AppDocRootContent || resp.DocRoot == transport.AppDocRootUnavailable:
		view.State = "unmet"
	case resp.ExtensionsUnknown:
		view.State = "unknown"
		view.Detail = "the site's PHP could not be asked which extensions it has"
	}
	return view
}

// appPrereqAnswerError carries an Agent answer with an error out of the
// cache, which keeps only answers without one.
type appPrereqAnswerError struct {
	resp transport.AppPrerequisitesResponse
}

func (e *appPrereqAnswerError) Error() string { return e.resp.Error }

// appDiskRoom: the subscription's disk limit leaves room for needMB, against
// the same total the disk limit uses (alpha.118: the measured site files,
// mail and databases; before the first measurement the per-site numbers).
func (p *Panel) appDiskRoom(ctx context.Context, subID int, needMB int64) error {
	if err := p.checkSubscriptionQuota(ctx, subID, quotaDisk); err != nil {
		return err
	}
	if p.administratorWorkspace(ctx, subID) {
		return nil
	}
	var limit, used int64
	err := p.db.GetDB().QueryRowContext(ctx, `
		SELECT s.disk_quota_mb, `+subscriptionDiskBytesSQL+`
		FROM subscriptions s WHERE s.id = ?`, subID).Scan(&limit, &used)
	if err != nil {
		return quotaReadError(err)
	}
	if quotaUnlimited(limit) {
		return nil
	}
	usedMB := used / (1 << 20)
	if usedMB+needMB > limit {
		return &quotaExceededError{Kind: quotaDisk, Used: usedMB + needMB, Limit: limit}
	}
	return nil
}

// domainInstalledApps lists the applications of a domain: catalog
// operations that are not removed, and a WordPress CelikPanel installed.
func (p *Panel) domainInstalledApps(ctx context.Context, domainID int, domain string) ([]installedAppView, error) {
	out := []installedAppView{}
	rows, err := p.db.GetDB().QueryContext(ctx, `
		SELECT operation_id, app_id, app_version, status, COALESCE(admin_path, ''), database_name,
		       COALESCE(error_kind, ''), COALESCE(last_error, ''), admin_secret_encrypted IS NOT NULL, updated_at
		FROM app_catalog_operations
		WHERE domain_id = ? AND status <> 'removed'
		ORDER BY created_at DESC, rowid DESC LIMIT 20`, domainID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	scheme := "https://"
	for rows.Next() {
		var v installedAppView
		var adminPath string
		if err := rows.Scan(&v.OperationID, &v.AppID, &v.Version, &v.Status, &adminPath, &v.DatabaseName,
			&v.ErrorKind, &v.LastError, &v.CredentialsPending, &v.UpdatedAt); err != nil {
			return nil, err
		}
		app, _ := appcatalog.Lookup(v.AppID)
		v.Name = app.Name
		if v.Name == "" {
			v.Name = v.AppID
		}
		v.SiteURL = scheme + domain + "/"
		if v.Status == appOpApplied && adminPath != "" {
			v.AdminURL = scheme + domain + adminPath
			v.AdminUser = app.AdminUser
		}
		if app.DataDir != "" {
			v.DataDir = app.DataDir
		}
		v.CanRetry = v.Status == appOpSetupFailed || v.Status == appOpFilesReady
		v.CanRemove = appRemovable(v.Status)
		if v.Status == appOpFailed && v.UpdatedAt < time.Now().UTC().Add(-24*time.Hour).Format("2006-01-02 15:04:05") {
			continue // an old failure that left nothing behind
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var opID, status, updated string
	err = p.db.GetDB().QueryRowContext(ctx, `
		SELECT operation_id, status, updated_at FROM application_install_operations
		WHERE domain_id = ? AND app_id = 'wordpress' AND status IN ('applied', 'needs_review')
		ORDER BY created_at DESC, rowid DESC LIMIT 1`, domainID).Scan(&opID, &status, &updated)
	if err == nil {
		out = append(out, installedAppView{OperationID: opID, AppID: "wordpress", Name: "WordPress", Version: "",
			Status: status, SiteURL: scheme + domain + "/", AdminURL: scheme + domain + "/wp-admin/", Legacy: true, UpdatedAt: updated})
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	return out, nil
}

func appRemovable(status string) bool {
	switch status {
	case appOpApplied, appOpSetupFailed, appOpFilesReady, appOpNeedsReview:
		return true
	}
	return false
}

// ---- install ----

type catalogInstallRequest struct {
	App string `json:"app"`
}

// handleCatalogAppInstall installs one catalog application (not WordPress).
func (p *Panel) handleCatalogAppInstall(w http.ResponseWriter, r *http.Request, domainID int, app appcatalog.App) {
	ctx := r.Context()
	installMu := appInstallMutex(domainID)
	installMu.Lock()
	defer installMu.Unlock()

	target, err := p.resolveAppInstallTarget(ctx, domainID)
	if errors.Is(err, sql.ErrNoRows) {
		writeClientError(w, http.StatusNotFound, "site not found")
		return
	}
	if err != nil {
		writeServerError(w, err)
		return
	}
	if target.ProjectType != "php" {
		writeCodedError(w, http.StatusConflict, errCodeAppNeedsPHPSite,
			app.Name+" runs on a PHP site; this site is a "+target.ProjectType+" site. Nothing was changed. Choose a PHP site, or change this site's type first.", "")
		return
	}
	if !p.requireEntitlement(w, r, target.SubscriptionID, "app_installer") {
		return
	}
	if !p.requireAdministratorOffersApp(w, r, app) {
		return
	}
	if !p.requireSubscriptionOffering(w, r, target.SubscriptionID, appcatalog.OfferingID(app.ID), app.Name, false) {
		return
	}
	if !p.requireSubscriptionOffering(w, r, target.SubscriptionID, "component:mariadb", "MariaDB", true) {
		return
	}
	if existing, ok := p.liveAppOnDomain(ctx, domainID); ok {
		writeAppAlreadyInstalled(w, existing)
		return
	}
	if err := p.ensureInstalledDBServers(ctx, target.SubscriptionID); err != nil {
		writeServerError(w, err)
		return
	}
	if err := p.checkSubscriptionQuota(ctx, target.SubscriptionID, quotaDBs); err != nil {
		writeQuotaRefusal(w, err)
		return
	}
	if err := p.appDiskRoom(ctx, target.SubscriptionID, app.MinDiskMB); err != nil {
		var exceeded *quotaExceededError
		if errors.As(err, &exceeded) {
			writeCodedError(w, http.StatusConflict, errCodeAppDiskRoom, fmt.Sprintf(
				"%s needs about %d MB and the subscription's disk limit would be passed (%d of %d MB). Nothing was changed. Free some space, or ask your provider for a larger plan.",
				app.Name, app.MinDiskMB, exceeded.Used, exceeded.Limit), myPlanPagePath)
			return
		}
		writeQuotaRefusal(w, err)
		return
	}
	site, err := p.appSiteFacts(ctx, target)
	if err != nil {
		writeServerError(w, err)
		return
	}
	identity := appSiteIdentity(target, domainID, site.phpVersion)
	if !p.requireAppPrerequisites(w, r, app, identity) {
		return
	}

	databaseName, databaseUser, serverID, err := p.reserveAppDatabaseNames(ctx, target, app)
	if errors.Is(err, sql.ErrNoRows) {
		writeCodedError(w, http.StatusConflict, errCodeAppNoDatabaseServer,
			"no MariaDB server is installed on this server, so "+app.Name+" cannot get its database. Nothing was changed. The administrator installs MariaDB in Components; then install again.", adminInstallAction(r, "mariadb"))
		return
	}
	if err != nil {
		writeServerError(w, err)
		return
	}
	databasePassword, err := randomDBPassword()
	if err != nil {
		writeServerError(w, err)
		return
	}
	adminPassword, err := randomDBPassword()
	if err != nil {
		writeServerError(w, err)
		return
	}
	operationID, err := randomHexToken(16)
	if err != nil {
		writeServerError(w, err)
		return
	}
	cleanupToken, err := randomHexToken(32)
	if err != nil {
		writeServerError(w, err)
		return
	}
	sealedDBPassword, err := p.secrets.Encrypt(databasePassword)
	if err != nil {
		writeServerError(w, err)
		return
	}
	sealedToken, err := p.secrets.Encrypt(cleanupToken)
	if err != nil {
		writeServerError(w, err)
		return
	}
	sealedAdmin, err := p.secrets.Encrypt(adminPassword)
	if err != nil {
		writeServerError(w, err)
		return
	}
	callerID := 0
	if c := currentCaller(r); c != nil {
		callerID = c.ID
	}
	if err := p.storeCatalogAppOperation(ctx, catalogOperationRow{
		OperationID: operationID, App: app, DomainID: domainID, Target: target, ServerID: serverID,
		DatabaseName: databaseName, DatabaseUser: databaseUser, SealedPassword: sealedDBPassword,
		SealedToken: sealedToken, SealedAdmin: sealedAdmin, CreatedBy: callerID,
	}); err != nil {
		if existing, ok := p.liveAppOnDomain(ctx, domainID); ok {
			writeAppAlreadyInstalled(w, existing)
			return
		}
		writeServerError(w, err)
		return
	}
	p.auditDetailed(r, "app.install_started:"+app.ID, "domain", domainID, map[string]any{
		"operation_id": operationID, "app": app.ID, "version": app.Version, "database": databaseName})

	// Database.
	if err := p.setCatalogAppStatus(operationID, appOpDatabaseCreating, "", ""); err != nil {
		writeServerError(w, err)
		return
	}
	var createResponse transport.CreateDatabaseResponse
	createErr := p.callAgent("Agent.CreateDatabase", transport.CreateDatabaseRequest{
		Type: "mysql", Name: databaseName, User: databaseUser, Password: databasePassword,
		OperationID: operationID, CleanupToken: cleanupToken,
	}, &createResponse)
	if createErr != nil {
		_ = p.setCatalogAppStatus(operationID, appOpNeedsReview, transport.AppFailInternal,
			"the answer to creating the database was lost; the administrator checks whether "+databaseName+" exists")
		writeAgentError(w, createErr, createResponse.Error)
		return
	}
	if !createResponse.Success || createResponse.Error != "" || !createResponse.OwnedByOperation {
		status := appOpFailed
		if createResponse.CleanupIncomplete || (createResponse.Success && !createResponse.OwnedByOperation) {
			status = appOpNeedsReview
		} else if err := p.removeCatalogDatabaseLedger(ctx, operationID); err != nil {
			status = appOpNeedsReview
		}
		_ = p.setCatalogAppStatus(operationID, status, transport.AppFailInternal, "the database could not be created: "+createResponse.Error)
		writeAgentError(w, nil, firstNonEmpty(createResponse.Error, "the database could not be created"))
		return
	}
	if err := p.setCatalogAppStatus(operationID, appOpDatabaseReady, "", ""); err != nil {
		writeServerError(w, err)
		return
	}

	// Files.
	if err := p.setCatalogAppStatus(operationID, appOpFilesInstalling, "", ""); err != nil {
		writeServerError(w, err)
		return
	}
	var filesResponse transport.InstallCatalogAppResponse
	filesErr := p.callAgent("Agent.InstallCatalogAppV1", &transport.InstallCatalogAppRequest{
		ExpectedBuildCommit: strings.TrimSpace(buildCommit), OperationID: operationID,
		AppID: app.ID, Version: app.Version, Site: identity,
	}, &filesResponse)
	if filesErr != nil {
		_ = p.setCatalogAppStatus(operationID, appOpNeedsReview, transport.AppFailInternal,
			"the answer to putting the files in place was lost; the files may or may not be on the site. Remove the application to clean up, then install again")
		writeAgentError(w, filesErr, filesResponse.Error)
		return
	}
	if !filesResponse.Installed {
		status := appOpNeedsReview
		if filesResponse.CompensationSafe {
			status = appOpFailed
			if err := p.compensateAppDatabase(nil, databaseName, databaseUser, operationID, cleanupToken); err != nil {
				status = appOpNeedsReview
			} else if err := p.removeCatalogDatabaseLedger(ctx, operationID); err != nil {
				status = appOpNeedsReview
			}
		}
		_ = p.setCatalogAppStatus(operationID, status, filesResponse.ErrorKind, filesResponse.Error)
		writeCatalogAgentFailure(w, app, operationID, filesResponse.ErrorKind, filesResponse.Error)
		return
	}
	if err := p.setCatalogAppStatus(operationID, appOpFilesReady, "", ""); err != nil {
		writeServerError(w, err)
		return
	}
	p.runCatalogAppSetup(w, r, operationID, app, target, domainID, identity, site.https, databaseName, databaseUser, databasePassword, adminPassword, false)
}

// runCatalogAppSetup runs the application's own setup and answers the request.
func (p *Panel) runCatalogAppSetup(w http.ResponseWriter, r *http.Request, operationID string, app appcatalog.App,
	target appInstallTarget, domainID int, identity transport.AppSiteIdentity, https bool,
	databaseName, databaseUser, databasePassword, adminPassword string, retry bool) {
	if err := p.setCatalogAppStatus(operationID, appOpSettingUp, "", ""); err != nil {
		writeServerError(w, err)
		return
	}
	var setup transport.SetupCatalogAppResponse
	setupErr := p.callAgent("Agent.SetupCatalogAppV1", &transport.SetupCatalogAppRequest{
		ExpectedBuildCommit: strings.TrimSpace(buildCommit), OperationID: operationID, AppID: app.ID,
		Site: identity, HTTPS: https, DBName: databaseName, DBUser: databaseUser, DBPass: databasePassword,
		DBHost: "localhost", AdminPass: adminPassword, AdminEmail: p.appAdminEmail(r.Context(), target),
	}, &setup)
	if setupErr != nil {
		// A setup is safe to run again: the lost answer leaves Retry.
		_ = p.setCatalogAppStatus(operationID, appOpSetupFailed, transport.AppFailInternal,
			"the answer from the setup was lost; Retry finishes it (a finished setup is detected)")
		writeAgentError(w, setupErr, setup.Error)
		return
	}
	if !setup.Configured {
		detail := setup.Error
		if tail := installerOutputTail(setup.Output, databasePassword, adminPassword); len(tail) > 0 {
			detail += "\n" + strings.Join(tail, "\n")
		}
		_ = p.setCatalogAppStatus(operationID, appOpSetupFailed, firstNonEmpty(setup.ErrorKind, transport.AppFailSetup), detail)
		writeCatalogAgentFailure(w, app, operationID, firstNonEmpty(setup.ErrorKind, transport.AppFailSetup), setup.Error,
			installerOutputTail(setup.Output, databasePassword, adminPassword)...)
		return
	}
	if _, err := p.db.GetDB().ExecContext(r.Context(), `UPDATE app_catalog_operations SET admin_path = ?, updated_at = datetime('now') WHERE operation_id = ?`,
		setup.AdminPath, operationID); err != nil {
		writeServerError(w, err)
		return
	}
	if err := p.setCatalogAppStatus(operationID, appOpApplied, "", ""); err != nil {
		writeServerError(w, err)
		return
	}
	// alpha.128: the site's upload size follows the application, within the
	// site's limit; the site setting takes the application's line over.
	p.applyAppUploadDefault(r.Context(), domainID, app)
	action := "app.install:" + app.ID
	if retry {
		action = "app.setup_retry:" + app.ID
	}
	p.auditDetailed(r, action, "domain", domainID, map[string]any{"operation_id": operationID, "app": app.ID, "version": app.Version,
		"rules_kept": setup.RulesKept})
	siteURL := "https://" + target.Domain + "/"
	if !https {
		siteURL = "http://" + target.Domain + "/"
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true, "operation_id": operationID, "app": app.ID, "name": app.Name, "version": app.Version,
		"site_url": siteURL, "admin_url": strings.TrimSuffix(siteURL, "/") + setup.AdminPath,
		"admin_user": app.AdminUser, "admin_password": adminPassword, "detail": setup.Detail,
		"rules_kept": setup.RulesKept,
	})
}

// requireAppPrerequisites asks the server; true when the install may go on.
func (p *Panel) requireAppPrerequisites(w http.ResponseWriter, r *http.Request, app appcatalog.App, identity transport.AppSiteIdentity) bool {
	if ok, reason := app.PHPSupported(identity.PHPVersion); !ok {
		writeCodedErrorVars(w, http.StatusConflict, errCodeAppPHPUnsupported,
			reason+". Nothing was changed. Choose a supported PHP version for this site in its PHP settings (the administrator installs it in Components if it is not offered), then install again.",
			"", nil, map[string]string{"app": app.Name, "php": identity.PHPVersion, "range": app.PHPRange()})
		return false
	}
	var resp transport.AppPrerequisitesResponse
	ctx, cancel := context.WithTimeout(r.Context(), 40*time.Second)
	defer cancel()
	err := p.callAgentContext(ctx, "Agent.AppPrerequisitesV1", &transport.AppPrerequisitesRequest{AppID: app.ID, Site: identity}, &resp)
	if err != nil || resp.Error != "" {
		reason, message := appPrereqFailure(app, err, resp)
		vars := map[string]string{"app": app.Name}
		if reason == appPrereqRefused {
			vars["cause"] = boundedPrereqError(resp.Error)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(apiErrorBody{Error: message, Code: errCodeAppPrereqUnknown, Reason: reason, Vars: vars})
		return false
	}
	if !resp.PHPOK {
		writeCodedErrorVars(w, http.StatusConflict, errCodeAppPHPUnsupported,
			resp.PHPReason+". Nothing was changed. Choose a supported PHP version for this site, then install again.", "", nil,
			map[string]string{"app": app.Name, "php": identity.PHPVersion, "range": app.PHPRange()})
		return false
	}
	if len(resp.Missing) > 0 {
		details := []string{}
		packages := []string{}
		for _, m := range resp.Missing {
			switch {
			case m.Package != "":
				details = append(details, m.Extension+": "+m.Package)
				packages = append(packages, m.Package)
			default:
				details = append(details, m.Extension+": switch it on in the PHP configuration")
			}
		}
		message := "PHP " + resp.PHPLine + " on this server lacks what " + app.Name + " needs. Nothing was changed. The administrator installs these packages (Components, or the system's package manager): " + strings.Join(packages, " ") + "; then install again."
		action := ""
		install := catalogExtensionsInstallPackage(resp.Family, resp.PHPLine, resp.Missing)
		if install != "" {
			message = "PHP " + resp.PHPLine + " on this server lacks what " + app.Name + " needs (" + strings.Join(packages, " ") + "). Nothing was changed. The administrator installs PHP " + resp.PHPLine + " in Components (PHP-FPM, version " + resp.PHPLine + "): that adds its standard extensions from the official packages. Then install again."
		}
		if callerIsAdministrator(r) {
			action = "/components?component=php-fpm"
			if install != "" {
				action += "&install=" + install
			}
		}
		writeCodedErrorVars(w, http.StatusConflict, errCodeAppExtensionsMissing, message,
			action, details, map[string]string{"app": app.Name, "php": resp.PHPLine, "packages": strings.Join(packages, " ")})
		return false
	}
	if resp.ExtensionsUnknown {
		writeCodedError(w, http.StatusServiceUnavailable, errCodeAppPrereqUnknown,
			"the site's PHP could not be asked which extensions it has; nothing was changed. Check that PHP "+resp.PHPLine+" is installed and running, then try again.", "")
		return false
	}
	if app.Legacy() {
		// WordPress's own installer checks its document root and disk room
		// itself (it keeps the placeholder page); here only PHP and its
		// extensions, before anything is created.
		return true
	}
	switch resp.DocRoot {
	case transport.AppDocRootContent, transport.AppDocRootUnavailable:
		writeCodedError(w, http.StatusConflict, errCodeAppDocRootNotEmpty,
			"the site's public folder already holds files, and "+app.Name+" is only installed into an empty site so nothing of yours is overwritten. Nothing was changed. Move or delete those files with the file manager (or use a new subdomain), then install again.", "")
		return false
	}
	if resp.FreeDiskMB > 0 && resp.FreeDiskMB < app.MinDiskMB*2 {
		writeCodedError(w, http.StatusConflict, errCodeAppDiskRoom, fmt.Sprintf(
			"the server has %d MB free where sites live; %s needs about %d MB. Nothing was changed. The administrator frees disk space, then you install again.",
			resp.FreeDiskMB, app.Name, app.MinDiskMB*2), "")
		return false
	}
	return true
}

func writeCodedErrorVars(w http.ResponseWriter, status int, code, message, action string, details []string, vars map[string]string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(apiErrorBody{Error: message, Code: code, Action: action, Details: details, Vars: vars})
}

// writeCatalogAgentFailure answers a step the server refused, by kind.
func writeCatalogAgentFailure(w http.ResponseWriter, app appcatalog.App, operationID, kind, message string, details ...string) {
	vars := map[string]string{"app": app.Name, "operation_id": operationID, "kind": kind}
	if len(details) == 0 {
		details = nil
	}
	switch kind {
	case transport.AppFailVerification:
		writeCodedErrorVars(w, http.StatusBadGateway, errCodeAppVerification, message, "", details, vars)
	case transport.AppFailUnpack:
		writeCodedErrorVars(w, http.StatusBadGateway, errCodeAppUnpack, message, "", details, vars)
	case transport.AppFailDownload:
		writeCodedErrorVars(w, http.StatusBadGateway, errCodeAppDownload, message, "", details, vars)
	case transport.AppFailContent:
		writeCodedErrorVars(w, http.StatusConflict, errCodeAppDocRootNotEmpty, message, "", details, vars)
	case transport.AppFailPrerequisite:
		writeCodedErrorVars(w, http.StatusConflict, errCodeAppPHPUnsupported, message, "", details, vars)
	case transport.AppFailSetup, transport.AppFailNginx:
		writeCodedErrorVars(w, http.StatusBadGateway, errCodeAppSetupFailed, message, "", details, vars)
	case transport.AppFailRemove:
		writeCodedErrorVars(w, http.StatusBadGateway, errCodeAppRemoveIncomplete, message, "", details, vars)
	default:
		writeCodedErrorVars(w, http.StatusBadGateway, errCodeAppInstallFailed, firstNonEmpty(message, app.Name+" could not be installed"), "", details, vars)
	}
}

// installerOutputTail is the end of an application installer's own output,
// shown beside a failed setup so its own reason is visible: the last few
// non-empty lines, each bounded, with the operation's secrets removed again
// (the agent already removed them).
// Kurucunun kendi çıktısının son satırları; gizli bilgiler yeniden silinir.
func installerOutputTail(output string, secrets ...string) []string {
	for _, secret := range secrets {
		if secret != "" {
			output = strings.ReplaceAll(output, secret, "********")
		}
	}
	const maxLines, maxRunes = 8, 300
	lines := []string{}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(strings.ToValidUTF8(line, "?"))
		if line == "" {
			continue
		}
		if runes := []rune(line); len(runes) > maxRunes {
			line = string(runes[:maxRunes]) + "…"
		}
		lines = append(lines, line)
	}
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return lines
}

type liveApp struct {
	OperationID string
	AppID       string
	Status      string
}

// liveAppOnDomain finds the application a domain already holds.
func (p *Panel) liveAppOnDomain(ctx context.Context, domainID int) (liveApp, bool) {
	var live liveApp
	err := p.db.GetDB().QueryRowContext(ctx, `
		SELECT operation_id, app_id, status FROM app_catalog_operations
		WHERE domain_id = ? AND status NOT IN ('failed', 'removed') LIMIT 1`, domainID).Scan(&live.OperationID, &live.AppID, &live.Status)
	if err == nil {
		return live, true
	}
	err = p.db.GetDB().QueryRowContext(ctx, `
		SELECT operation_id, app_id, status FROM application_install_operations
		WHERE domain_id = ? AND status NOT IN ('failed') LIMIT 1`, domainID).Scan(&live.OperationID, &live.AppID, &live.Status)
	return live, err == nil
}

func writeAppAlreadyInstalled(w http.ResponseWriter, live liveApp) {
	name := live.AppID
	if app, ok := appcatalog.Lookup(live.AppID); ok {
		name = app.Name
	}
	writeCodedErrorVars(w, http.StatusConflict, errCodeAppAlreadyInstalled,
		"this site already holds "+name+" ("+live.Status+"); nothing was changed and nothing was started a second time. Open it in the list below, or remove it first to install another application.",
		"", nil, map[string]string{"app": name, "operation_id": live.OperationID, "status": live.Status})
}

// reserveAppDatabaseNames picks free names: <site>_<app> (with a number
// when an earlier attempt left that name).
func (p *Panel) reserveAppDatabaseNames(ctx context.Context, target appInstallTarget, app appcatalog.App) (string, string, int, error) {
	base := sanitizeDBIdent(services.SiteUsername(target.Domain))
	code := strings.ReplaceAll(app.ID, "-", "")
	if len(code) > 6 {
		code = code[:6]
	}
	if len(base) > 40 {
		base = base[:40]
	}
	for attempt := 0; attempt < 8; attempt++ {
		name := base + "_" + code
		if attempt > 0 {
			name = fmt.Sprintf("%s%d", name, attempt+1)
		}
		serverID, err := p.preflightAppDatabase(ctx, target.SubscriptionID, name, name)
		if errors.Is(err, errAppDatabaseExists) {
			continue
		}
		if err != nil {
			return "", "", 0, err
		}
		return name, name, serverID, nil
	}
	return "", "", 0, errors.New("no free database name was found for this application")
}

type catalogOperationRow struct {
	OperationID    string
	App            appcatalog.App
	DomainID       int
	Target         appInstallTarget
	ServerID       int
	DatabaseName   string
	DatabaseUser   string
	SealedPassword string
	SealedToken    string
	SealedAdmin    string
	CreatedBy      int
}

// storeCatalogAppOperation writes the operation and its database ledger in
// one transaction. The partial unique index refuses a second live
// application on the domain.
func (p *Panel) storeCatalogAppOperation(ctx context.Context, row catalogOperationRow) error {
	tx, err := p.db.GetDB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `INSERT INTO databases_v2 (server_id, subscription_id, domain_id, name) VALUES (?, ?, ?, ?)`,
		row.ServerID, row.Target.SubscriptionID, row.DomainID, row.DatabaseName)
	if err != nil {
		return fmt.Errorf("store application database: %w", err)
	}
	databaseID, err := result.LastInsertId()
	if err != nil {
		return err
	}
	result, err = tx.ExecContext(ctx, `INSERT INTO database_users (server_id, subscription_id, username, password) VALUES (?, ?, ?, ?)`,
		row.ServerID, row.Target.SubscriptionID, row.DatabaseUser, row.SealedPassword)
	if err != nil {
		return fmt.Errorf("store application database user: %w", err)
	}
	userID, err := result.LastInsertId()
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO database_user_grants (database_id, user_id, privileges) VALUES (?, ?, 'ALL')`, databaseID, userID); err != nil {
		return fmt.Errorf("store application database grant: %w", err)
	}
	var createdBy any
	if row.CreatedBy > 0 {
		createdBy = row.CreatedBy
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO app_catalog_operations (
			operation_id, app_id, app_version, domain_id, subscription_id, site_id,
			database_server_id, database_id, database_user_id, database_name, database_user,
			cleanup_token_encrypted, admin_secret_encrypted, status, created_by
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'reserved', ?)`,
		row.OperationID, row.App.ID, row.App.Version, row.DomainID, row.Target.SubscriptionID, row.Target.SiteID,
		row.ServerID, databaseID, userID, row.DatabaseName, row.DatabaseUser, row.SealedToken, row.SealedAdmin, createdBy); err != nil {
		return fmt.Errorf("reserve application operation: %w", err)
	}
	return tx.Commit()
}

func (p *Panel) setCatalogAppStatus(operationID, status, kind, detail string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if len(detail) > 3000 {
		detail = detail[:3000]
	}
	result, err := p.db.GetDB().ExecContext(ctx, `
		UPDATE app_catalog_operations
		SET status = ?, error_kind = NULLIF(?, ''), last_error = NULLIF(?, ''), updated_at = datetime('now')
		WHERE operation_id = ?`, status, kind, detail, operationID)
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		return fmt.Errorf("application operation %s was not found", operationID)
	}
	return nil
}

// removeCatalogDatabaseLedger removes the database rows an operation wrote
// (the engine has no such database).
func (p *Panel) removeCatalogDatabaseLedger(ctx context.Context, operationID string) error {
	var databaseID, userID sql.NullInt64
	if err := p.db.GetDB().QueryRowContext(ctx, `SELECT database_id, database_user_id FROM app_catalog_operations WHERE operation_id = ?`,
		operationID).Scan(&databaseID, &userID); err != nil {
		return err
	}
	ledger := &appDatabaseLedger{OperationID: operationID, DatabaseID: databaseID.Int64, UserID: userID.Int64}
	return p.removeAppDatabaseLedger(ledger)
}

// appAdminEmail: the subscription owner's address, else admin@<domain>.
func (p *Panel) appAdminEmail(ctx context.Context, target appInstallTarget) string {
	var email string
	_ = p.db.GetDB().QueryRowContext(ctx, `SELECT COALESCE(u.email, '') FROM subscriptions s JOIN users u ON u.id = s.owner_id WHERE s.id = ?`,
		target.SubscriptionID).Scan(&email)
	email = strings.TrimSpace(email)
	if !strings.Contains(email, "@") || strings.ContainsAny(email, " \t\r\n'\"\\") {
		email = "admin@" + target.Domain
	}
	return email
}

// ---- retry, credentials, removal ----

type catalogOperation struct {
	OperationID  string
	AppID        string
	Status       string
	DomainID     int
	DatabaseName string
	DatabaseUser string
	DatabaseUID  sql.NullInt64
	DatabaseID   sql.NullInt64
	SealedAdmin  sql.NullString
}

func (p *Panel) loadCatalogOperation(ctx context.Context, domainID int, operationID string) (catalogOperation, error) {
	var op catalogOperation
	err := p.db.GetDB().QueryRowContext(ctx, `
		SELECT operation_id, app_id, status, domain_id, database_name, database_user, database_user_id, database_id, admin_secret_encrypted
		FROM app_catalog_operations WHERE operation_id = ? AND domain_id = ?`, operationID, domainID).Scan(
		&op.OperationID, &op.AppID, &op.Status, &op.DomainID, &op.DatabaseName, &op.DatabaseUser, &op.DatabaseUID, &op.DatabaseID, &op.SealedAdmin)
	return op, err
}

// handleAppOperation serves POST apps/{op}/retry, POST apps/{op}/credentials
// and DELETE apps/{op}.
func (p *Panel) handleAppOperation(w http.ResponseWriter, r *http.Request, domainID int, operationID, action string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, no-store")
	if !validHexOperationID(operationID) {
		writeClientError(w, http.StatusNotFound, "application operation not found")
		return
	}
	installMu := appInstallMutex(domainID)
	installMu.Lock()
	defer installMu.Unlock()
	op, err := p.loadCatalogOperation(r.Context(), domainID, operationID)
	if errors.Is(err, sql.ErrNoRows) {
		writeClientError(w, http.StatusNotFound, "application operation not found")
		return
	}
	if err != nil {
		writeServerError(w, err)
		return
	}
	app, ok := appcatalog.Lookup(op.AppID)
	if !ok {
		writeCodedError(w, http.StatusConflict, errCodeAppUnknown, "this release does not know the application "+op.AppID+"; nothing was changed", "")
		return
	}
	switch action {
	case "retry":
		p.retryCatalogAppSetup(w, r, domainID, op, app)
	case "credentials":
		p.revealCatalogAppCredentials(w, r, domainID, op, app)
	case "remove":
		p.removeCatalogApp(w, r, domainID, op, app)
	default:
		writeClientError(w, http.StatusNotFound, "not found")
	}
}

func validHexOperationID(value string) bool {
	if len(value) != 32 {
		return false
	}
	for _, c := range value {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func (p *Panel) retryCatalogAppSetup(w http.ResponseWriter, r *http.Request, domainID int, op catalogOperation, app appcatalog.App) {
	if op.Status != appOpSetupFailed && op.Status != appOpFilesReady {
		writeCodedErrorVars(w, http.StatusConflict, errCodeAppOperationState,
			"only an unfinished setup can be retried; this one is "+op.Status+". Nothing was started.", "", nil,
			map[string]string{"status": op.Status})
		return
	}
	target, err := p.resolveAppInstallTarget(r.Context(), domainID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	site, err := p.appSiteFacts(r.Context(), target)
	if err != nil {
		writeServerError(w, err)
		return
	}
	var sealedDB string
	if err := p.db.GetDB().QueryRowContext(r.Context(), `SELECT password FROM database_users WHERE id = ?`, op.DatabaseUID.Int64).Scan(&sealedDB); err != nil {
		writeCodedError(w, http.StatusConflict, errCodeAppOperationState,
			"the application's database user is no longer recorded, so the setup cannot be finished. Remove the application and install it again.", "")
		return
	}
	dbPassword, err := p.secrets.Decrypt(sealedDB)
	if err != nil {
		writeServerError(w, err)
		return
	}
	adminPassword := ""
	if op.SealedAdmin.Valid {
		adminPassword, err = p.secrets.Decrypt(op.SealedAdmin.String)
		if err != nil {
			writeServerError(w, err)
			return
		}
	} else {
		adminPassword, err = randomDBPassword()
		if err != nil {
			writeServerError(w, err)
			return
		}
		sealed, err := p.secrets.Encrypt(adminPassword)
		if err != nil {
			writeServerError(w, err)
			return
		}
		if _, err := p.db.GetDB().ExecContext(r.Context(), `UPDATE app_catalog_operations SET admin_secret_encrypted = ? WHERE operation_id = ?`, sealed, op.OperationID); err != nil {
			writeServerError(w, err)
			return
		}
	}
	identity := appSiteIdentity(target, domainID, site.phpVersion)
	p.runCatalogAppSetup(w, r, op.OperationID, app, target, domainID, identity, site.https, op.DatabaseName, op.DatabaseUser, dbPassword, adminPassword, true)
}

// revealCatalogAppCredentials shows the generated administrator password
// once and forgets it.
func (p *Panel) revealCatalogAppCredentials(w http.ResponseWriter, r *http.Request, domainID int, op catalogOperation, app appcatalog.App) {
	if !op.SealedAdmin.Valid {
		writeCodedError(w, http.StatusGone, errCodeAppOperationState,
			"the administrator password was already shown once and is not kept. Use "+app.Name+"'s own \"forgot password\" to set a new one.", "")
		return
	}
	password, err := p.secrets.Decrypt(op.SealedAdmin.String)
	if err != nil {
		writeServerError(w, err)
		return
	}
	result, err := p.db.GetDB().ExecContext(r.Context(), `UPDATE app_catalog_operations SET admin_secret_encrypted = NULL, updated_at = datetime('now')
		WHERE operation_id = ? AND admin_secret_encrypted IS NOT NULL`, op.OperationID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if n, _ := result.RowsAffected(); n != 1 {
		writeCodedError(w, http.StatusGone, errCodeAppOperationState, "the administrator password was already shown once and is not kept.", "")
		return
	}
	p.auditDetailed(r, "app.credentials_shown:"+app.ID, "domain", domainID, map[string]any{"operation_id": op.OperationID})
	_ = json.NewEncoder(w).Encode(map[string]any{"admin_user": app.AdminUser, "admin_password": password})
}

type catalogRemoveRequest struct {
	// Confirm must be the domain's name.
	Confirm        string `json:"confirm"`
	RemoveDatabase bool   `json:"remove_database"`
	RemoveData     bool   `json:"remove_data"`
}

func (p *Panel) removeCatalogApp(w http.ResponseWriter, r *http.Request, domainID int, op catalogOperation, app appcatalog.App) {
	var req catalogRemoveRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		writeClientError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !appRemovable(op.Status) && op.Status != appOpRemoving {
		writeCodedErrorVars(w, http.StatusConflict, errCodeAppOperationState,
			"this application is "+op.Status+" and cannot be removed now; nothing was changed.", "", nil, map[string]string{"status": op.Status})
		return
	}
	target, err := p.resolveAppInstallTarget(r.Context(), domainID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if !strings.EqualFold(strings.TrimSpace(req.Confirm), target.Domain) {
		writeCodedError(w, http.StatusBadRequest, errCodeAppRemoveConfirm,
			"to remove "+app.Name+", type the domain name ("+target.Domain+") to confirm; nothing was changed.", "")
		return
	}
	site, err := p.appSiteFacts(r.Context(), target)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if err := p.setCatalogAppStatus(op.OperationID, appOpRemoving, "", ""); err != nil {
		writeServerError(w, err)
		return
	}
	var resp transport.RemoveCatalogAppResponse
	callErr := p.callAgent("Agent.RemoveCatalogAppV1", &transport.RemoveCatalogAppRequest{
		ExpectedBuildCommit: strings.TrimSpace(buildCommit), AppID: app.ID,
		Site: appSiteIdentity(target, domainID, site.phpVersion), RemoveData: req.RemoveData && req.RemoveDatabase,
	}, &resp)
	if callErr != nil {
		_ = p.setCatalogAppStatus(op.OperationID, appOpNeedsReview, transport.AppFailInternal,
			"the answer to the removal was lost; remove again to finish it")
		writeAgentError(w, callErr, resp.Error)
		return
	}
	if resp.Error != "" {
		_ = p.setCatalogAppStatus(op.OperationID, appOpNeedsReview, resp.ErrorKind, resp.Error)
		writeCatalogAgentFailure(w, app, op.OperationID, resp.ErrorKind, resp.Error)
		return
	}
	databaseRemoved := false
	databaseNote := ""
	if req.RemoveDatabase {
		if err := p.removeCatalogAppDatabase(r.Context(), op); err != nil {
			databaseNote = "the files were removed, but the database " + op.DatabaseName + " could not be removed (" + err.Error() + "); remove it under Databases"
		} else {
			databaseRemoved = true
		}
	}
	kept := append([]string{}, resp.KeptEntries...)
	statusNote := databaseNote
	if len(kept) > 0 {
		// Items the owner added after the removal started stay; the removal
		// still ends (no failure loop) and says what was kept.
		statusNote = strings.TrimPrefix(statusNote+"; ", "; ") + resp.Detail
	}
	if err := p.setCatalogAppStatus(op.OperationID, appOpRemoved, "", statusNote); err != nil {
		writeServerError(w, err)
		return
	}
	_, _ = p.db.GetDB().ExecContext(r.Context(), `UPDATE app_catalog_operations SET admin_secret_encrypted = NULL WHERE operation_id = ?`, op.OperationID)
	p.auditDetailed(r, "app.remove:"+app.ID, "domain", domainID, map[string]any{
		"operation_id": op.OperationID, "database_removed": databaseRemoved, "data_removed": resp.DataRemoved, "rules_kept": resp.RulesKept,
		"kept_entries": len(kept)})
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true, "database_removed": databaseRemoved, "database_kept": !databaseRemoved, "database": op.DatabaseName,
		"database_note": databaseNote, "data_removed": resp.DataRemoved, "data_kept": resp.DataKept, "rules_kept": resp.RulesKept,
		"detail": resp.Detail, "kept_entries": kept,
	})
}

// removeCatalogAppDatabase drops the application's database and user (only
// on the owner's explicit confirmation) and their records.
func (p *Panel) removeCatalogAppDatabase(ctx context.Context, op catalogOperation) error {
	var exists int
	if err := p.db.GetDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM databases_v2 WHERE id = ?`, op.DatabaseID.Int64).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return nil // already removed under Databases
	}
	var resp transport.DeleteDatabaseResponse
	if err := p.callAgent("Agent.DeleteDatabase", transport.DeleteDatabaseRequest{Type: "mysql", Name: op.DatabaseName, User: op.DatabaseUser}, &resp); err != nil {
		return err
	}
	if !resp.Success || resp.Error != "" {
		return errors.New(firstNonEmpty(resp.Error, "the database server refused"))
	}
	return p.removeAppDatabaseLedger(&appDatabaseLedger{OperationID: op.OperationID, DatabaseID: op.DatabaseID.Int64, UserID: op.DatabaseUID.Int64})
}

// recoverInterruptedCatalogAppOperations runs at start: a setup or removal
// that was running is safe to run again (Retry / Remove), every other
// unfinished step is shown for review. Nothing is guessed.
func (p *Panel) recoverInterruptedCatalogAppOperations(ctx context.Context) (int64, error) {
	result, err := p.db.GetDB().ExecContext(ctx, `
		UPDATE app_catalog_operations
		SET status = CASE status WHEN 'setting_up' THEN 'setup_failed' WHEN 'files_ready' THEN 'setup_failed' ELSE 'needs_review' END,
		    error_kind = 'internal',
		    last_error = CASE status
		        WHEN 'setting_up' THEN 'the panel restarted while the application was being set up; Retry finishes it'
		        WHEN 'files_ready' THEN 'the panel restarted before the setup started; Retry runs it'
		        WHEN 'removing' THEN 'the panel restarted during the removal; remove again to finish it'
		        ELSE 'the panel restarted before this step finished; remove the application to clean up, then install again' END,
		    updated_at = datetime('now')
		WHERE status IN ('reserved', 'database_creating', 'database_ready', 'files_installing', 'files_ready', 'setting_up', 'removing')`)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// routeAppOperation maps the request to retry, credentials or remove.
func (p *Panel) routeAppOperation(w http.ResponseWriter, r *http.Request, domainID int) {
	segments, ok := strictRouteSegments(r, "/api/v1/domains/")
	if !ok || len(segments) < 3 {
		writeClientError(w, http.StatusNotFound, "not found")
		return
	}
	operationID := segments[2]
	action := "remove"
	if r.Method == http.MethodPost && len(segments) == 4 {
		action = segments[3]
	}
	p.handleAppOperation(w, r, domainID, operationID, action)
}

// liveCatalogAppOnly: a catalog application (not WordPress) on the domain.
func (p *Panel) liveCatalogAppOnly(ctx context.Context, domainID int) (liveApp, bool) {
	var live liveApp
	err := p.db.GetDB().QueryRowContext(ctx, `
		SELECT operation_id, app_id, status FROM app_catalog_operations
		WHERE domain_id = ? AND status NOT IN ('failed', 'removed') LIMIT 1`, domainID).Scan(&live.OperationID, &live.AppID, &live.Status)
	return live, err == nil
}

// requireAdministratorOffersApp: an application is installed only once the
// administrator offered it on this server, whoever installs it (the
// administrator too): third-party software is gated at the offer step.
func (p *Panel) requireAdministratorOffersApp(w http.ResponseWriter, r *http.Request, app appcatalog.App) bool {
	offered, err := p.loadOfferedApps(r.Context())
	if err != nil {
		writeServerError(w, err)
		return false
	}
	hidden, err := p.loadHiddenOfferings(r.Context())
	if err != nil {
		hidden = map[string]bool{}
	}
	if offered[app.ID] && !hidden[appcatalog.OfferingID(app.ID)] {
		return true
	}
	writeCodedErrorVars(w, http.StatusConflict, errCodeNotOffered,
		"Nothing was changed: "+app.Name+" is not offered on this server. The administrator offers it under Plans and rights, Offered (after reading its note); then it can be installed.",
		myPlanPagePath, nil, map[string]string{"item": appcatalog.OfferingID(app.ID), "name": app.Name, "grantor": "the administrator"})
	return false
}
