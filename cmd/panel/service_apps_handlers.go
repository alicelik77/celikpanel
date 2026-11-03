package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/alicelik/celikpanel/internal/serviceapps"
	"github.com/alicelik/celikpanel/internal/transport"
)

// The domain's "AI & automation" screen and its actions. Every refusal says
// why (Reason, a code the screen words in English and Turkish), who acts and
// what to do next. Secrets go to the server once and are never shown again.
//
// Alan adının "Yapay zekâ ve otomasyon" ekranı ve eylemleri. Her ret nedenini,
// kimin ne yapacağını söyler; gizli değerler bir kez gider, bir daha görünmez.

var serviceAppOperationIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{7,63}$`)

// The availability of the catalog on this server changes only when the
// administrator installs a runtime; it is read at most once a minute.
var serviceAppProbeCache struct {
	sync.Mutex
	at   time.Time
	resp transport.ServiceAppProbeResponse
	err  error
}

func (p *Panel) serviceAppAvailability(ctx context.Context) (map[string]transport.ServiceAppAvailability, error) {
	serviceAppProbeCache.Lock()
	defer serviceAppProbeCache.Unlock()
	if time.Since(serviceAppProbeCache.at) > time.Minute || serviceAppProbeCache.err != nil {
		resp, err := p.serviceAppNativeOps().probe(ctx)
		if err == nil && resp.Error != "" {
			err = errors.New(resp.Error)
		}
		serviceAppProbeCache.resp, serviceAppProbeCache.err, serviceAppProbeCache.at = resp, err, time.Now()
	}
	if serviceAppProbeCache.err != nil {
		return nil, serviceAppProbeCache.err
	}
	out := map[string]transport.ServiceAppAvailability{}
	for _, a := range serviceAppProbeCache.resp.Apps {
		out[a.AppID] = a
	}
	return out, nil
}

func resetServiceAppProbeCache() {
	serviceAppProbeCache.Lock()
	serviceAppProbeCache.at = time.Time{}
	serviceAppProbeCache.Unlock()
}

func writeServiceAppRefusal(w http.ResponseWriter, status int, reason, message string, vars map[string]string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(apiErrorBody{Error: message, Code: errCodeServiceApp, Reason: reason, Vars: vars})
}

// serviceAppSite is the domain's site as an install needs it.
type serviceAppSite struct {
	SubscriptionID int
	SiteID         int
	Domain         string
	ProjectType    string
	AppPort        sql.NullInt64
	Certificate    bool
	Suspended      bool
}

func (p *Panel) serviceAppSiteFor(ctx context.Context, domainID int) (serviceAppSite, bool, error) {
	var s serviceAppSite
	var siteID sql.NullInt64
	var projectType sql.NullString
	var sslEnabled sql.NullBool
	var cert, key sql.NullString
	err := p.db.GetDB().QueryRowContext(ctx, `
		SELECT d.subscription_id, d.name, s.id, COALESCE(s.project_type, 'php'), s.app_port,
		       COALESCE(s.ssl_enabled, 0), s.ssl_cert_path, s.ssl_key_path
		FROM domains d LEFT JOIN sites s ON s.domain_id = d.id WHERE d.id = ?`, domainID).
		Scan(&s.SubscriptionID, &s.Domain, &siteID, &projectType, &s.AppPort, &sslEnabled, &cert, &key)
	if errors.Is(err, sql.ErrNoRows) {
		return s, false, nil
	}
	if err != nil {
		return s, false, err
	}
	s.SiteID = int(siteID.Int64)
	s.ProjectType = projectType.String
	s.Certificate = sslEnabled.Bool && cert.String != "" && key.String != ""
	suspended, err := p.subscriptionSuspended(ctx, s.SubscriptionID)
	if err != nil {
		return s, false, err
	}
	s.Suspended = suspended
	return s, true, nil
}

// serviceAppAppView is one catalog entry on the domain's screen.
type serviceAppAppView struct {
	serviceapps.App
	Offered   bool   `json:"offered"`
	RefusedBy string `json:"refused_by,omitempty"`
	Grantor   string `json:"grantor,omitempty"`
	Held      bool   `json:"held"`
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
	Detail    string `json:"detail,omitempty"`
}

type serviceAppInstallView struct {
	ID              int      `json:"id"`
	AppID           string   `json:"app_id"`
	Name            string   `json:"name"`
	Version         string   `json:"version"`
	TargetVersion   string   `json:"target_version"`
	State           string   `json:"state"`
	URL             string   `json:"url"`
	UpdateAvailable string   `json:"update_available,omitempty"`
	Held            bool     `json:"held"`
	Active          bool     `json:"active"`
	Enabled         bool     `json:"enabled"`
	ActiveState     string   `json:"active_state,omitempty"`
	MemoryMB        int64    `json:"memory_mb,omitempty"`
	Serving         bool     `json:"serving"`
	Exposed         []string `json:"exposed,omitempty"`
	UnitFile        string   `json:"unit_file,omitempty"`
	StatusUnknown   bool     `json:"status_unknown,omitempty"`
	Unit            string   `json:"unit"`
	Folder          string   `json:"folder"`
	CreatedAt       string   `json:"created_at"`
}

type serviceAppsDomainView struct {
	Domain            string                 `json:"domain"`
	Suspended         bool                   `json:"suspended"`
	HasSite           bool                   `json:"has_site"`
	ProjectType       string                 `json:"project_type"`
	Certificate       bool                   `json:"certificate"`
	PermissionGranted bool                   `json:"permission_granted"`
	PermissionRefused string                 `json:"permission_refused_by,omitempty"`
	PermissionGrantor string                 `json:"permission_grantor,omitempty"`
	AvailabilityKnown bool                   `json:"availability_known"`
	Apps              []serviceAppAppView    `json:"apps"`
	Install           *serviceAppInstallView `json:"install"`
	Operation         *serviceAppOperation   `json:"operation"`
}

func (p *Panel) handleDomainServiceApps(w http.ResponseWriter, r *http.Request, domainID int, kind string) {
	switch kind {
	case "service-apps":
		p.writeServiceAppsView(w, r, domainID)
	case "service-apps-install":
		p.handleServiceAppInstall(w, r, domainID)
	case "service-apps-update":
		p.handleServiceAppUpdate(w, r, domainID)
	case "service-apps-control":
		p.handleServiceAppControl(w, r, domainID)
	case "service-apps-logs":
		p.handleServiceAppLogs(w, r, domainID)
	case "service-apps-secrets":
		p.handleServiceAppSecrets(w, r, domainID)
	case "service-apps-uninstall":
		p.handleServiceAppUninstall(w, r, domainID)
	default:
		http.NotFound(w, r)
	}
}

// serviceAppRights: the permission and the layers of a subscription; an
// administrator acting for the account passes the permission and the
// reseller and subscription layers, never the administrator's own offer.
func (p *Panel) serviceAppRights(ctx context.Context, r *http.Request, subID int) (*effectiveRights, error) {
	e, err := p.effectiveSubscriptionRights(ctx, subID)
	if err != nil {
		return nil, err
	}
	if callerIsAdministrator(r) {
		e.Unrestricted = true
	}
	return e, nil
}

func (p *Panel) writeServiceAppsView(w http.ResponseWriter, r *http.Request, domainID int) {
	ctx := r.Context()
	site, found, err := p.serviceAppSiteFor(ctx, domainID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if !found {
		writeClientError(w, http.StatusNotFound, "domain not found")
		return
	}
	view := serviceAppsDomainView{Domain: site.Domain, Suspended: site.Suspended, HasSite: site.SiteID > 0 && site.ProjectType != "dnsonly",
		ProjectType: site.ProjectType, Certificate: site.Certificate, Apps: []serviceAppAppView{}}
	e, err := p.serviceAppRights(ctx, r, site.SubscriptionID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if layer := e.permissionRefusal(rightServiceApps); layer == "" {
		view.PermissionGranted = true
	} else {
		view.PermissionRefused, view.PermissionGrantor = layer, e.grantor(layer)
	}
	offerings, err := p.loadServiceAppOfferings(ctx)
	if err != nil {
		writeServerError(w, err)
		return
	}
	offered, err := p.loadOfferedServiceApps(ctx)
	if err != nil {
		writeServerError(w, err)
		return
	}
	availability, probeErr := p.serviceAppAvailability(ctx)
	view.AvailabilityKnown = probeErr == nil
	for _, app := range serviceapps.Catalog() {
		item := serviceAppAppView{App: app, Held: offerings[app.ID].Held}
		switch {
		case serviceAppClosedToReseller(app, e):
			item.RefusedBy = rightsLayerLicence
			item.Grantor = "the administrator"
		case !offered[app.ID]:
			item.RefusedBy = rightsLayerAdministrator
			item.Grantor = "the administrator"
		default:
			if layer := e.offeringRefusal(serviceapps.OfferingID(app.ID)); layer != "" {
				item.RefusedBy, item.Grantor = layer, e.grantor(layer)
			} else {
				item.Offered = true
			}
		}
		if a, ok := availability[app.ID]; ok {
			item.Available, item.Reason, item.Detail = a.Available, a.Reason, a.Detail
		} else if probeErr != nil {
			item.Reason = serviceapps.ReasonProbeFailed
		}
		if app.Deferred != "" {
			item.Available, item.Reason, item.Detail = false, serviceapps.ReasonDeferred, app.Deferred
		}
		view.Apps = append(view.Apps, item)
	}
	in, has, err := p.serviceAppInstallForDomain(ctx, domainID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if has {
		iv := &serviceAppInstallView{ID: in.ID, AppID: in.AppID, Name: in.AppID, Version: in.Version,
			TargetVersion: in.TargetVersion, State: in.State, URL: "https://" + in.Domain + "/",
			Held: offerings[in.AppID].Held, Unit: serviceapps.UnitName(in.ID), CreatedAt: in.CreatedAt}
		if app, ok := serviceapps.Lookup(in.AppID); ok {
			iv.Name = app.Name
			if in.Version != "" && in.Version != app.Version && app.Installable() {
				iv.UpdateAvailable = app.Version
			}
			if layout, err := serviceapps.LayoutFor(in.SubscriptionID, in.DomainID, in.AppID); err == nil {
				iv.Folder = layout.Root
			}
		}
		statusCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		status, err := p.serviceAppNativeOps().status(statusCtx, transport.ServiceAppStatusRequest{Target: in.target(), Version: in.Version, Port: in.Port})
		cancel()
		if err != nil || status.Error != "" {
			iv.StatusUnknown = true
		} else {
			iv.Active, iv.Enabled, iv.ActiveState = status.Unit.Active, status.Unit.Enabled, status.ActiveState
			iv.MemoryMB, iv.Serving, iv.Exposed, iv.UnitFile = status.MemoryMB, status.Serving, status.Exposed, status.UnitFile
		}
		view.Install = iv
		if op, ok, err := p.latestServiceAppOperation(ctx, in.ID); err == nil && ok {
			view.Operation = &op
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(view)
}

type serviceAppInstallRequest struct {
	App            string            `json:"app"`
	OperationID    string            `json:"operation_id"`
	OwnerEmail     string            `json:"owner_email"`
	OwnerPassword  string            `json:"owner_password"`
	Secrets        map[string]string `json:"secrets"`
	ConfirmReplace bool              `json:"confirm_replace"`
}

func decodeServiceAppBody(w http.ResponseWriter, r *http.Request, into any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := json.NewDecoder(r.Body).Decode(into); err != nil {
		writeClientError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	return true
}

// validOwnerPassword: 12 to 128 characters of one line.
func validOwnerPassword(password string) bool {
	return len(password) >= 12 && len(password) <= 128 && serviceapps.ValidSecretValue(password)
}

// ownerSecretFor is what the app's environment receives for its first
// administrator: a bcrypt hash for n8n (its owner-by-environment setting
// takes only a hash), the password itself for Open WebUI (read once on first
// start, then removed from the file).
func ownerSecretFor(app serviceapps.App, password string) (string, error) {
	if app.ID == "n8n" {
		hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
		return string(hash), err
	}
	return password, nil
}

// refuseServiceAppOffer applies the offer layers for new use (install and
// update). It writes the refusal and returns true when refused.
func (p *Panel) refuseServiceAppOffer(w http.ResponseWriter, r *http.Request, app serviceapps.App, subID int) bool {
	ctx := r.Context()
	offerings, err := p.loadServiceAppOfferings(ctx)
	if err != nil {
		writeServerError(w, err)
		return true
	}
	if offerings[app.ID].Held {
		writeServiceAppRefusal(w, http.StatusConflict, "held",
			"Nothing was changed: the administrator has put "+app.Name+" on a security hold. It can be installed or started again when the administrator lifts the hold.",
			map[string]string{"app": app.Name})
		return true
	}
	offered, err := p.loadOfferedServiceApps(ctx)
	if err != nil {
		writeServerError(w, err)
		return true
	}
	e, err := p.serviceAppRights(ctx, r, subID)
	if err != nil {
		writeServerError(w, err)
		return true
	}
	// alpha.118: closed to resellers and their customers by its licence,
	// whoever asks (an administrator acting for the account too).
	if serviceAppClosedToReseller(app, e) {
		writeServiceAppLicenceClosed(w, app)
		return true
	}
	if !offered[app.ID] {
		writeNotOffered(w, e, serviceapps.OfferingID(app.ID), app.Name, rightsLayerAdministrator)
		return true
	}
	if layer := e.permissionRefusal(rightServiceApps); layer != "" {
		writeRightNotGranted(w, e, rightServiceApps, layer)
		return true
	}
	if layer := e.offeringRefusal(serviceapps.OfferingID(app.ID)); layer != "" {
		writeNotOffered(w, e, serviceapps.OfferingID(app.ID), app.Name, layer)
		return true
	}
	return false
}

func (p *Panel) handleServiceAppInstall(w http.ResponseWriter, r *http.Request, domainID int) {
	var req serviceAppInstallRequest
	if !decodeServiceAppBody(w, r, &req) {
		return
	}
	ctx := r.Context()
	app, ok := serviceapps.Lookup(req.App)
	if !ok {
		writeClientError(w, http.StatusBadRequest, "unknown application")
		return
	}
	if !app.Installable() {
		writeServiceAppRefusal(w, http.StatusConflict, "deferred",
			"Nothing was changed: "+app.Name+" cannot be installed in this release (docs/AI-APPS.md says why).", map[string]string{"app": app.Name})
		return
	}
	if req.OperationID != "" && !serviceAppOperationIDPattern.MatchString(req.OperationID) {
		writeClientError(w, http.StatusBadRequest, "invalid operation id")
		return
	}
	// The same request again (a lost answer): the same operation is
	// answered and nothing starts twice.
	if req.OperationID != "" {
		if op, err := p.serviceAppOperationByID(ctx, req.OperationID); err == nil {
			if in, err := p.serviceAppInstallByID(ctx, op.InstallID); err == nil && in.DomainID == domainID {
				writeServiceAppAccepted(w, http.StatusOK, op)
				return
			}
			writeClientError(w, http.StatusConflict, "this operation id belongs to another request")
			return
		}
	}
	site, found, err := p.serviceAppSiteFor(ctx, domainID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if !found || site.SiteID <= 0 || site.ProjectType == "dnsonly" {
		writeServiceAppRefusal(w, http.StatusConflict, "no_site",
			"Nothing was changed: this domain has no website on this server. Add hosting to the domain first, then install the app.", nil)
		return
	}
	if site.Suspended {
		writeServiceAppRefusal(w, http.StatusConflict, "suspended",
			"Nothing was changed: this subscription is suspended. Apps can be installed again when it is resumed.", nil)
		return
	}
	if p.refuseServiceAppOffer(w, r, app, site.SubscriptionID) {
		return
	}
	// alpha.118 integration (D-044): an app's files and data live in the
	// site's home and count toward the disk limit; a subscription already
	// at or over it installs nothing new (My plan says what to do).
	if err := p.checkSubscriptionQuota(ctx, site.SubscriptionID, quotaDisk); err != nil {
		writeQuotaRefusal(w, err)
		return
	}
	if !site.Certificate {
		writeServiceAppRefusal(w, http.StatusConflict, "certificate_missing",
			"Nothing was changed: "+app.Name+" is served only over HTTPS, and "+site.Domain+" has no certificate yet. Get a free certificate on the domain's SSL page, then install.",
			map[string]string{"domain": site.Domain})
		return
	}
	if site.ProjectType == "node" || site.ProjectType == "service_app" {
		writeServiceAppRefusal(w, http.StatusConflict, "site_type",
			"Nothing was changed: this domain runs a Node.js application or another app. Use a separate domain or subdomain for "+app.Name+".", nil)
		return
	}
	if message, err := p.refuseProjectTypeWithSiteFeatures(ctx, domainID, "service_app"); err != nil {
		writeServerError(w, err)
		return
	} else if message != "" {
		writeServiceAppRefusal(w, http.StatusConflict, "site_features", message, nil)
		return
	}
	if !req.ConfirmReplace {
		writeServiceAppRefusal(w, http.StatusBadRequest, "confirm_replace",
			"Confirm that "+site.Domain+" will show "+app.Name+" instead of its current website (its files stay in place).", nil)
		return
	}
	availability, err := p.serviceAppAvailability(ctx)
	if err != nil {
		writeServiceAppRefusal(w, http.StatusServiceUnavailable, serviceapps.ReasonProbeFailed,
			"Nothing was changed: the server could not say whether "+app.Name+" can run here. Try again in a minute.", nil)
		return
	}
	if a := availability[app.ID]; !a.Available {
		writeServiceAppRefusal(w, http.StatusConflict, a.Reason,
			"Nothing was changed: "+app.Name+" cannot run on this server now ("+a.Detail+").", map[string]string{"detail": a.Detail, "app": app.Name})
		return
	}
	req.OwnerEmail = strings.TrimSpace(req.OwnerEmail)
	if app.OwnerLogin && (!serviceapps.ValidEmail(req.OwnerEmail) || !validOwnerPassword(req.OwnerPassword)) {
		writeServiceAppRefusal(w, http.StatusBadRequest, "owner_login",
			"Enter the app administrator's e-mail address and a password of 12 to 128 characters.", nil)
		return
	}
	allowed := app.AllowedSecretKeys()
	secrets := map[string]string{}
	for key, value := range req.Secrets {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if !allowed[key] || !serviceapps.ValidSecretValue(value) {
			writeServiceAppRefusal(w, http.StatusBadRequest, "secret_value", "One of the keys is not a setting of "+app.Name+" or is not one line of text.", nil)
			return
		}
		secrets[key] = value
	}
	ownerSecret := ""
	if app.OwnerLogin {
		if ownerSecret, err = ownerSecretFor(app, req.OwnerPassword); err != nil {
			writeServerError(w, err)
			return
		}
	}
	opID := req.OperationID
	if opID == "" {
		if opID, err = newServiceOperationID(); err != nil {
			writeServerError(w, err)
			return
		}
	}
	installID, err := p.createServiceAppInstall(ctx, currentCaller(r), site, domainID, app, opID)
	if err != nil {
		var refusal *serviceAppConflict
		if errors.As(err, &refusal) {
			writeServiceAppRefusal(w, http.StatusConflict, refusal.reason, refusal.message, nil)
			return
		}
		writeServerError(w, err)
		return
	}
	p.auditDetailed(r, "service_app.install", "domain", domainID, map[string]any{"app": app.ID, "version": app.Version, "install": installID, "operation": opID})
	op, err := p.serviceAppOperationByID(ctx, opID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	p.startServiceAppOperation(opID, &serviceAppSecretInput{OwnerEmail: req.OwnerEmail, OwnerSecret: ownerSecret, Secrets: secrets})
	writeServiceAppAccepted(w, http.StatusAccepted, op)
}

type serviceAppConflict struct{ reason, message string }

func (c *serviceAppConflict) Error() string { return c.message }

// createServiceAppInstall records the install (or reuses a failed first
// install of the same app) and its operation in one transaction.
func (p *Panel) createServiceAppInstall(ctx context.Context, caller *Caller, site serviceAppSite, domainID int, app serviceapps.App, opID string) (int, error) {
	tx, err := p.db.GetDB().BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	var installID int
	var existingApp, existingState, existingVersion string
	err = tx.QueryRowContext(ctx, `SELECT id, app_id, state, version FROM service_app_installs WHERE domain_id = ?`, domainID).
		Scan(&installID, &existingApp, &existingState, &existingVersion)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		port, aux, err := allocateServiceAppPorts(ctx, tx)
		if err != nil {
			return 0, &serviceAppConflict{"no_port", "Nothing was changed: " + err.Error() + "."}
		}
		if !app.AuxPort {
			aux = 0
		}
		previousType := site.ProjectType
		if previousType == "" {
			previousType = "php"
		}
		res, err := tx.ExecContext(ctx, `
			INSERT INTO service_app_installs (subscription_id, domain_id, site_id, app_id, target_version, port, aux_port,
			       state, previous_project_type, previous_app_port, created_by)
			VALUES (?, ?, ?, ?, ?, ?, ?, 'installing', ?, ?, ?)`,
			site.SubscriptionID, domainID, site.SiteID, app.ID, app.Version, port, aux, previousType, site.AppPort, nullablePositiveInt(callerID(caller)))
		if err != nil {
			return 0, err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return 0, err
		}
		installID = int(id)
	case err != nil:
		return 0, err
	case existingApp == app.ID && existingState == serviceAppFailed && existingVersion == "":
		if _, err := tx.ExecContext(ctx, `UPDATE service_app_installs SET state = 'installing', target_version = ?, updated_at = datetime('now') WHERE id = ?`,
			app.Version, installID); err != nil {
			return 0, err
		}
	default:
		return 0, &serviceAppConflict{"already_installed", "Nothing was changed: this domain already has an app. Remove it first, or use another domain or subdomain."}
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO service_app_operations (id, install_id, kind, step, actor_id) VALUES (?, ?, 'install', 'prepare', ?)`,
		opID, installID, nullablePositiveInt(callerID(caller))); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return 0, &serviceAppConflict{"busy", "Nothing was changed: another operation of this app is still running. Wait for it to finish."}
		}
		return 0, err
	}
	return installID, tx.Commit()
}

func callerID(c *Caller) int {
	if c == nil {
		return 0
	}
	return c.ID
}

func writeServiceAppAccepted(w http.ResponseWriter, status int, op serviceAppOperation) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"operation": op})
}

// serviceAppInstallForAction loads the domain's install and refuses when an
// operation of it is still running.
func (p *Panel) serviceAppInstallForAction(w http.ResponseWriter, r *http.Request, domainID int) (serviceAppInstall, bool) {
	in, has, err := p.serviceAppInstallForDomain(r.Context(), domainID)
	if err != nil {
		writeServerError(w, err)
		return in, false
	}
	if !has {
		writeServiceAppRefusal(w, http.StatusNotFound, "not_installed", "This domain has no app installed.", nil)
		return in, false
	}
	if op, ok, err := p.latestServiceAppOperation(r.Context(), in.ID); err == nil && ok && op.State == "running" {
		writeServiceAppRefusal(w, http.StatusConflict, "busy",
			"Nothing was changed: the "+op.Kind+" of this app is still running. It continues by itself; wait for it to finish.", nil)
		return in, false
	}
	return in, true
}

func (p *Panel) handleServiceAppUpdate(w http.ResponseWriter, r *http.Request, domainID int) {
	var req struct {
		OperationID string `json:"operation_id"`
	}
	if !decodeServiceAppBody(w, r, &req) {
		return
	}
	if req.OperationID != "" && !serviceAppOperationIDPattern.MatchString(req.OperationID) {
		writeClientError(w, http.StatusBadRequest, "invalid operation id")
		return
	}
	ctx := r.Context()
	if req.OperationID != "" {
		if op, err := p.serviceAppOperationByID(ctx, req.OperationID); err == nil {
			writeServiceAppAccepted(w, http.StatusOK, op)
			return
		}
	}
	in, ok := p.serviceAppInstallForAction(w, r, domainID)
	if !ok {
		return
	}
	app, known := serviceapps.Lookup(in.AppID)
	if !known || !app.Installable() || in.Version == "" || in.Version == app.Version {
		writeServiceAppRefusal(w, http.StatusConflict, "no_update", "There is no newer pinned version of this app in this release.", nil)
		return
	}
	site, _, err := p.serviceAppSiteFor(ctx, domainID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if site.Suspended {
		writeServiceAppRefusal(w, http.StatusConflict, "suspended", "Nothing was changed: this subscription is suspended.", nil)
		return
	}
	if p.refuseServiceAppOffer(w, r, app, in.SubscriptionID) {
		return
	}
	opID := req.OperationID
	if opID == "" {
		if opID, err = newServiceOperationID(); err != nil {
			writeServerError(w, err)
			return
		}
	}
	db := p.db.GetDB()
	if _, err := db.ExecContext(ctx, `UPDATE service_app_installs SET target_version = ?, updated_at = datetime('now') WHERE id = ?`, app.Version, in.ID); err != nil {
		writeServerError(w, err)
		return
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO service_app_operations (id, install_id, kind, step, actor_id) VALUES (?, ?, 'update', 'prepare', ?)`,
		opID, in.ID, nullablePositiveInt(callerID(currentCaller(r)))); err != nil {
		writeServiceAppRefusal(w, http.StatusConflict, "busy", "Nothing was changed: another operation of this app is still running.", nil)
		return
	}
	p.auditDetailed(r, "service_app.update", "domain", domainID, map[string]any{"app": app.ID, "from": in.Version, "to": app.Version, "operation": opID})
	op, _ := p.serviceAppOperationByID(ctx, opID)
	p.startServiceAppOperation(opID, &serviceAppSecretInput{})
	writeServiceAppAccepted(w, http.StatusAccepted, op)
}

func (p *Panel) handleServiceAppControl(w http.ResponseWriter, r *http.Request, domainID int) {
	var req struct {
		Action string `json:"action"`
	}
	if !decodeServiceAppBody(w, r, &req) {
		return
	}
	if req.Action != "start" && req.Action != "stop" && req.Action != "restart" {
		writeClientError(w, http.StatusBadRequest, "action must be start, stop or restart")
		return
	}
	ctx := r.Context()
	in, ok := p.serviceAppInstallForAction(w, r, domainID)
	if !ok {
		return
	}
	if in.Version == "" {
		writeServiceAppRefusal(w, http.StatusConflict, "not_installed", "The app is not installed yet; install it first.", nil)
		return
	}
	if req.Action != "stop" {
		offerings, err := p.loadServiceAppOfferings(ctx)
		if err != nil {
			writeServerError(w, err)
			return
		}
		if offerings[in.AppID].Held {
			writeServiceAppRefusal(w, http.StatusConflict, "held",
				"Nothing was changed: the administrator has put this app on a security hold. It can be started again when the hold is lifted.", nil)
			return
		}
		if suspended, err := p.subscriptionSuspended(ctx, in.SubscriptionID); err != nil {
			writeServerError(w, err)
			return
		} else if suspended {
			writeServiceAppRefusal(w, http.StatusConflict, "suspended", "Nothing was changed: this subscription is suspended.", nil)
			return
		}
	}
	resp, err := p.serviceAppNativeOps().control(ctx, transport.ServiceAppControlRequest{Target: in.target(), Action: req.Action})
	if err != nil {
		writeServiceAppRefusal(w, http.StatusBadGateway, "agent_unreachable", "The server did not answer, so the result is unknown. Read the status again before you retry.", nil)
		return
	}
	if !resp.Done {
		writeServiceAppRefusal(w, http.StatusConflict, resp.Reason, resp.Error, nil)
		return
	}
	state := serviceAppRunning
	if req.Action == "stop" {
		state = serviceAppStopped
	}
	_, _ = p.db.GetDB().ExecContext(ctx, `UPDATE service_app_installs SET state = ?, updated_at = datetime('now') WHERE id = ?`, state, in.ID)
	// Fixed audit names (the audit name list is derived from the source).
	details := map[string]any{"app": in.AppID, "install": in.ID}
	switch req.Action {
	case "start":
		p.auditDetailed(r, "service_app.start", "domain", domainID, details)
	case "stop":
		p.auditDetailed(r, "service_app.stop", "domain", domainID, details)
	default:
		p.auditDetailed(r, "service_app.restart", "domain", domainID, details)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"state": state})
}

func (p *Panel) handleServiceAppLogs(w http.ResponseWriter, r *http.Request, domainID int) {
	in, has, err := p.serviceAppInstallForDomain(r.Context(), domainID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if !has {
		writeServiceAppRefusal(w, http.StatusNotFound, "not_installed", "This domain has no app installed.", nil)
		return
	}
	lines, _ := strconv.Atoi(r.URL.Query().Get("lines"))
	if lines <= 0 || lines > 500 {
		lines = 200
	}
	resp, err := p.serviceAppNativeOps().logs(r.Context(), transport.ServiceAppLogsRequest{Target: in.target(), Lines: lines, Install: r.URL.Query().Get("install") == "1"})
	if err != nil || resp.Error != "" {
		writeServiceAppRefusal(w, http.StatusBadGateway, "logs_unavailable", "The log could not be read from the server just now. Try again.", nil)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"lines": resp.Lines})
}

func (p *Panel) handleServiceAppSecrets(w http.ResponseWriter, r *http.Request, domainID int) {
	var req struct {
		Secrets       map[string]string `json:"secrets"`
		Clear         []string          `json:"clear"`
		OwnerPassword string            `json:"owner_password"`
	}
	if !decodeServiceAppBody(w, r, &req) {
		return
	}
	in, ok := p.serviceAppInstallForAction(w, r, domainID)
	if !ok {
		return
	}
	app, known := serviceapps.Lookup(in.AppID)
	if !known || in.Version == "" {
		writeServiceAppRefusal(w, http.StatusConflict, "not_installed", "The app is not installed yet; install it first.", nil)
		return
	}
	allowed := app.AllowedSecretKeys()
	secrets := map[string]string{}
	changed := []string{}
	for key, value := range req.Secrets {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if !allowed[key] || !serviceapps.ValidSecretValue(value) {
			writeServiceAppRefusal(w, http.StatusBadRequest, "secret_value", "One of the keys is not a setting of "+app.Name+" or is not one line of text.", nil)
			return
		}
		secrets[key] = value
		changed = append(changed, key)
	}
	for _, key := range req.Clear {
		if !allowed[key] {
			writeServiceAppRefusal(w, http.StatusBadRequest, "secret_value", "One of the keys is not a setting of "+app.Name+".", nil)
			return
		}
		changed = append(changed, key)
	}
	ownerSecret := ""
	if req.OwnerPassword != "" {
		if app.ID != "n8n" {
			writeServiceAppRefusal(w, http.StatusBadRequest, "owner_in_app", "Change this app's administrator password inside the app itself.", nil)
			return
		}
		if !validOwnerPassword(req.OwnerPassword) {
			writeServiceAppRefusal(w, http.StatusBadRequest, "owner_login", "The password must be 12 to 128 characters.", nil)
			return
		}
		hash, err := ownerSecretFor(app, req.OwnerPassword)
		if err != nil {
			writeServerError(w, err)
			return
		}
		ownerSecret = hash
		changed = append(changed, "owner_password")
	}
	if len(changed) == 0 {
		writeClientError(w, http.StatusBadRequest, "nothing to change")
		return
	}
	resp, err := p.serviceAppNativeOps().secrets(r.Context(), transport.ServiceAppSecretsRequest{
		Target: in.target(), Port: in.Port, AuxPort: in.AuxPort, OwnerSecret: ownerSecret, Secrets: secrets, Clear: req.Clear})
	if err != nil {
		writeServiceAppRefusal(w, http.StatusBadGateway, "agent_unreachable", "The server did not answer, so it is unknown whether the keys were saved. Enter them again.", nil)
		return
	}
	if !resp.Done {
		writeServiceAppRefusal(w, http.StatusConflict, resp.Reason, resp.Error, nil)
		return
	}
	// The audit names which settings changed, never their values.
	p.auditDetailed(r, "service_app.keys", "domain", domainID, map[string]any{"app": in.AppID, "changed": changed})
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"changed": changed})
}

func (p *Panel) handleServiceAppUninstall(w http.ResponseWriter, r *http.Request, domainID int) {
	var req struct {
		OperationID   string `json:"operation_id"`
		RemoveData    bool   `json:"remove_data"`
		ConfirmDomain string `json:"confirm_domain"`
	}
	if !decodeServiceAppBody(w, r, &req) {
		return
	}
	if req.OperationID != "" && !serviceAppOperationIDPattern.MatchString(req.OperationID) {
		writeClientError(w, http.StatusBadRequest, "invalid operation id")
		return
	}
	ctx := r.Context()
	if req.OperationID != "" {
		if op, err := p.serviceAppOperationByID(ctx, req.OperationID); err == nil {
			writeServiceAppAccepted(w, http.StatusOK, op)
			return
		}
	}
	in, ok := p.serviceAppInstallForAction(w, r, domainID)
	if !ok {
		return
	}
	if req.RemoveData && !strings.EqualFold(strings.TrimSpace(req.ConfirmDomain), in.Domain) {
		writeServiceAppRefusal(w, http.StatusBadRequest, "confirm_data",
			"To remove the app's data too, type the domain name "+in.Domain+". Without it, the data folder is kept.", map[string]string{"domain": in.Domain})
		return
	}
	opID := req.OperationID
	var err error
	if opID == "" {
		if opID, err = newServiceOperationID(); err != nil {
			writeServerError(w, err)
			return
		}
	}
	removeData := 0
	if req.RemoveData {
		removeData = 1
	}
	db := p.db.GetDB()
	if _, err := db.ExecContext(ctx, `INSERT INTO service_app_operations (id, install_id, kind, step, remove_data, actor_id) VALUES (?, ?, 'uninstall', 'unpublish', ?, ?)`,
		opID, in.ID, removeData, nullablePositiveInt(callerID(currentCaller(r)))); err != nil {
		writeServiceAppRefusal(w, http.StatusConflict, "busy", "Nothing was changed: another operation of this app is still running.", nil)
		return
	}
	_, _ = db.ExecContext(ctx, `UPDATE service_app_installs SET state = 'removing', updated_at = datetime('now') WHERE id = ?`, in.ID)
	p.auditDetailed(r, "service_app.uninstall", "domain", domainID, map[string]any{"app": in.AppID, "install": in.ID, "remove_data": req.RemoveData, "operation": opID})
	op, _ := p.serviceAppOperationByID(ctx, opID)
	p.startServiceAppOperation(opID, nil)
	writeServiceAppAccepted(w, http.StatusAccepted, op)
}

// refuseHostingChangeWithServiceApp: a domain that serves an app keeps its
// type until the app is removed (the unit would otherwise run unreachable).
func (p *Panel) refuseHostingChangeWithServiceApp(w http.ResponseWriter, r *http.Request, domainID int) bool {
	in, has, err := p.serviceAppInstallForDomain(r.Context(), domainID)
	if err != nil {
		writeServerError(w, err)
		return true
	}
	if !has {
		return false
	}
	writeServiceAppRefusal(w, http.StatusConflict, "app_installed",
		"Nothing was changed: "+serviceAppLabel(in)+" uses this domain. Remove the app on the AI & automation tab first, then change the site type.", nil)
	return true
}

// refuseDomainDeleteWithServiceApp: the same before a domain is deleted.
func (p *Panel) refuseDomainDeleteWithServiceApp(w http.ResponseWriter, r *http.Request, domainID int) bool {
	in, has, err := p.serviceAppInstallForDomain(r.Context(), domainID)
	if err != nil {
		writeServerError(w, err)
		return true
	}
	if !has {
		return false
	}
	writeServiceAppRefusal(w, http.StatusConflict, "app_installed",
		"Nothing was deleted: "+serviceAppLabel(in)+" runs on this domain. Remove the app on the AI & automation tab first (choose whether to keep its data), then delete the domain.", nil)
	return true
}
