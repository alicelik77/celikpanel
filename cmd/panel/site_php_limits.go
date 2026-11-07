package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// The site's PHP settings within limits (alpha.128, docs/SITE-LIMITS.md).
//
//	GET /api/v1/domains/{id}/php/settings   the fields, limits and effective values
//	PUT /api/v1/domains/{id}/php/settings   save the site's own values
//
// The values live in the site's native pool file (php_admin_value for the
// limits, so an application cannot raise them) and two of them in the site's
// nginx settings file (client_max_body_size, fastcgi_read_timeout). The
// screen gets, for every field, the site's own value, the server's value,
// the effective limit with the level that set it, and what PHP really uses
// with where it comes from; this answers what a phpinfo page is opened for
// without publishing one.
//
// A save runs: check the values against the limits (SETTING_ABOVE_LIMIT
// names who can raise one), write the pool (php-fpm -t, put back on refusal),
// write the nginx file (nginx -t, put back on refusal). When nginx refuses,
// the pool is written back with the values it had, so the two never disagree
// silently; the answer says what is in place.
//
// Sitenin PHP ayarları; değerler havuz dosyasında ve nginx ayar dosyasında
// durur. Kayıt: sınır denetimi, havuz, nginx; nginx reddederse havuz eski
// değerlerine döner.

// phpLimitsFor is what every render of a site's pool keeps within. The
// administrator's own workspace has no ceiling (the administrator sets them).
func phpLimitsFor(limits settingLimits) transport.SitePHPLimits {
	inherit := func(key string) int {
		limit := limits[key]
		if limit.Ceiling != nil && limit.SetBy != settingSetByServer {
			return *limit.Ceiling
		}
		return 0
	}
	return transport.SitePHPLimits{
		Known:            true,
		MemoryMB:         limits.ceilingValue(settingPHPMemoryMB),
		TimeS:            limits.ceilingValue(settingPHPTimeS),
		UploadMB:         limits.ceilingValue(settingPHPUploadMB),
		InputVars:        limits.ceilingValue(settingPHPInputVars),
		Processes:        limits.ceilingValue(settingPHPProcesses),
		InheritMemoryMB:  inherit(settingPHPMemoryMB),
		InheritTimeS:     inherit(settingPHPTimeS),
		InheritUploadMB:  inherit(settingPHPUploadMB),
		InheritInputVars: inherit(settingPHPInputVars),
	}
}

// sitePHPLimits is the pool limits of a domain's site ({} for the
// administrator's own workspace or when they cannot be read: the fixed
// bounds still apply).
func (p *Panel) sitePHPLimits(ctx context.Context, domainID int) transport.SitePHPLimits {
	subID, err := p.domainSubscriptionID(ctx, domainID)
	if err != nil {
		return transport.SitePHPLimits{}
	}
	own, err := subscriptionRightsState(ctx, p.db.GetDB(), subID)
	if err != nil || own.AdminWorkspace {
		return transport.SitePHPLimits{}
	}
	limits, err := subscriptionSettingLimits(ctx, p.db.GetDB(), subID)
	if err != nil {
		log.Printf("site setting limits (domain %d): %v", domainID, err)
		return transport.SitePHPLimits{}
	}
	return phpLimitsFor(limits)
}

// phpLimitField is one limited field of the screen.
type phpLimitField struct {
	Field   string `json:"field"`
	Setting string `json:"setting"`
	Unit    string `json:"unit"`
	// Own is the site's own value (nil: none; the default applies).
	Own *int `json:"own"`
	// Server is the server's value (php.ini, or the pool default for
	// processes); ServerText is it as written.
	Server     *int   `json:"server"`
	ServerText string `json:"server_text,omitempty"`
	// Default is what the site gets without a value of its own.
	Default   *int   `json:"default"`
	Ceiling   *int   `json:"ceiling"`
	SetBy     string `json:"set_by,omitempty"`
	SetByName string `json:"set_by_name,omitempty"`
	Presets   []int  `json:"presets"`
	Min       int    `json:"min"`
	Max       int    `json:"max"`
	Fixed     bool   `json:"fixed,omitempty"`
	// Effective is what PHP (and nginx) really use; Source says where it
	// comes from: site, server, plan (the plan's ceiling is written because
	// the server's value is above it), nginx_file (another file of the site
	// sets the web server's limit: OwnerFile).
	Effective *int   `json:"effective"`
	Source    string `json:"source"`
	OwnerFile string `json:"owner_file,omitempty"`
	// ServerAboveCeiling: the server's own value is above the server ceiling
	// (the administrator's php.ini); sites without a value of their own get
	// it, customers may choose up to the ceiling.
	ServerAboveCeiling bool `json:"server_above_ceiling,omitempty"`
	// Clamped (alpha.128): the site's own value is above its limit (the
	// limit was lowered after it was chosen); PHP runs at the limit.
	Clamped bool `json:"clamped,omitempty"`
}

// phpFreeField is one free choice of the screen.
type phpFreeField struct {
	Field  string   `json:"field"`
	Own    string   `json:"own"`
	Server string   `json:"server"`
	Values []string `json:"values,omitempty"`
}

type phpLimitsView struct {
	DomainID   int             `json:"domain_id"`
	Domain     string          `json:"domain"`
	PHPVersion string          `json:"php_version"`
	CanChange  bool            `json:"can_change"`
	ClosedBy   string          `json:"closed_by,omitempty"`
	Admin      bool            `json:"admin"`
	PoolState  string          `json:"pool_state"`
	Limits     []phpLimitField `json:"limits"`
	Free       []phpFreeField  `json:"free"`
	// MemoryAtOnceMB: processes x memory, the most the site can use at once.
	MemoryAtOnceMB int    `json:"memory_at_once_mb,omitempty"`
	NginxFile      string `json:"nginx_file_state,omitempty"`
	ReadError      string `json:"read_error,omitempty"`
}

// phpLimitsRequest is the PUT body: 0 or "" is "the default" (the line is
// removed).
type phpLimitsRequest struct {
	MemoryMB      int    `json:"memory_mb"`
	TimeS         int    `json:"time_s"`
	UploadMB      int    `json:"upload_mb"`
	InputVars     int    `json:"input_vars"`
	Processes     int    `json:"processes"`
	DisplayErrors string `json:"display_errors"`
	ErrorLevel    string `json:"error_level"`
	Timezone      string `json:"timezone"`
	ShortOpenTag  string `json:"short_open_tag"`
}

func (r phpLimitsRequest) settings() transport.SitePHPSettings {
	return transport.SitePHPSettings{MemoryMB: r.MemoryMB, TimeS: r.TimeS, UploadMB: r.UploadMB, InputVars: r.InputVars,
		Processes: r.Processes, DisplayErrors: r.DisplayErrors, ErrorLevel: r.ErrorLevel,
		Timezone: strings.TrimSpace(r.Timezone), ShortOpenTag: r.ShortOpenTag}
}

const (
	errCodePHPSettingsUnread       = "PHP_SETTINGS_UNREAD"
	errCodeSiteSettingsNginxRefuse = "SITE_SETTINGS_NGINX_REFUSED"
)

// readSitePHPSettings asks the Agent for one site's pool and server values.
func (p *Panel) readSitePHPSettings(ctx context.Context, site siteFileSite) (transport.SitePHPSettingsState, error) {
	var response transport.SitePHPSettingsResponse
	request := transport.SitePHPSettingsRequest{ExpectedBuildCommit: strings.TrimSpace(buildCommit),
		Sites: []transport.SitePHPSettingsSite{{SiteID: site.SiteID, Domain: site.Domain,
			Username: services.SiteUsername(site.Domain), PHPVersion: site.PHPVersion}}}
	if err := p.callAgentContext(ctx, "Agent.SitePHPSettingsV1", &request, &response); err != nil {
		return transport.SitePHPSettingsState{}, err
	}
	if response.Error != "" {
		return transport.SitePHPSettingsState{}, errors.New(response.Error)
	}
	if len(response.Sites) != 1 {
		return transport.SitePHPSettingsState{}, errors.New("the Agent did not answer for the site")
	}
	return response.Sites[0], nil
}

func limitIntPtr(v int) *int { return &v }

// buildPHPLimitsView joins the limits and the Agent's reading.
func buildPHPLimitsView(site siteFileSite, limits settingLimits, adminWorkspace bool, read transport.SitePHPSettingsState) phpLimitsView {
	view := phpLimitsView{DomainID: site.DomainID, Domain: site.Domain, PHPVersion: site.PHPVersion, PoolState: read.PoolState,
		Limits: []phpLimitField{}, Free: []phpFreeField{}, NginxFile: read.SettingsFileState}
	type limitSource struct {
		field, key string
		own        int
		server     string
		builtin    int
		size       bool
	}
	sources := []limitSource{
		{"memory", settingPHPMemoryMB, read.Own.MemoryMB, read.Server.MemoryLimit, 128, true},
		{"time", settingPHPTimeS, read.Own.TimeS, read.Server.MaxExecutionTime, 30, false},
		{"upload", settingPHPUploadMB, read.Own.UploadMB, read.Server.UploadMaxFilesize, 2, true},
		{"input_vars", settingPHPInputVars, read.Own.InputVars, read.Server.MaxInputVars, 1000, false},
		{"processes", settingPHPProcesses, read.Processes, "", 5, false},
	}
	for _, src := range sources {
		limit := limits[src.key]
		if adminWorkspace {
			limit.Ceiling, limit.SetBy, limit.SetByName = nil, "", ""
		}
		field := phpLimitField{Field: src.field, Setting: src.key, Unit: limit.Unit, Ceiling: limit.Ceiling, SetBy: limit.SetBy,
			SetByName: limit.SetByName, Presets: limit.Presets, Min: limit.Min, Max: limit.Max, Fixed: limit.Fixed && !adminWorkspace}
		if field.Presets == nil {
			field.Presets = []int{}
		}
		server := src.builtin
		if src.server != "" {
			field.ServerText = src.server
			if parsed, ok := services.PHPININumber(src.server, src.size); ok {
				server = parsed
			}
		}
		field.Server = limitIntPtr(server)
		if src.field == "processes" {
			// The pool always carries its own count; the default is the
			// pool's default within the ceiling.
			field.Own = limitIntPtr(src.own)
			field.Clamped = limit.Ceiling != nil && src.own > *limit.Ceiling
			def := limit.clamp(5)
			field.Default = &def
			field.Effective = limitIntPtr(limit.clamp(src.own))
			field.Source = "site"
			view.Limits = append(view.Limits, field)
			continue
		}
		unlimited := server <= 0
		def := server
		source := "server"
		if limit.Ceiling != nil && (unlimited || server > *limit.Ceiling) {
			if limit.SetBy == settingSetByServer {
				field.ServerAboveCeiling = true
			} else {
				def, source = *limit.Ceiling, "plan"
			}
		}
		field.Default = limitIntPtr(def)
		if src.own > 0 {
			field.Own = limitIntPtr(src.own)
			field.Clamped = limit.Ceiling != nil && src.own > *limit.Ceiling
			field.Effective, field.Source = limitIntPtr(limit.clamp(src.own)), "site"
		} else {
			field.Effective, field.Source = limitIntPtr(def), source
		}
		if src.field == "upload" && read.NginxUploadOwner != "" {
			field.OwnerFile = read.NginxUploadOwner
			if read.NginxUploadMB > 0 && read.NginxUploadMB < *field.Effective {
				field.Effective, field.Source = limitIntPtr(read.NginxUploadMB), "nginx_file"
			}
		}
		if src.field == "time" && read.NginxTimeoutOwner != "" {
			field.OwnerFile = read.NginxTimeoutOwner
		}
		view.Limits = append(view.Limits, field)
	}
	var memory, processes int
	for _, f := range view.Limits {
		if f.Field == "memory" && f.Effective != nil {
			memory = *f.Effective
		}
		if f.Field == "processes" && f.Effective != nil {
			processes = *f.Effective
		}
	}
	if memory > 0 && processes > 0 {
		view.MemoryAtOnceMB = memory * processes
	}
	view.Free = []phpFreeField{
		{Field: "display_errors", Own: read.Own.DisplayErrors, Server: read.Server.DisplayErrors, Values: []string{"on", "off"}},
		{Field: "error_level", Own: read.Own.ErrorLevel, Server: read.Server.ErrorReporting, Values: []string{"all", "production", "errors"}},
		{Field: "timezone", Own: read.Own.Timezone, Server: read.Server.Timezone},
		{Field: "short_open_tag", Own: read.Own.ShortOpenTag, Server: read.Server.ShortOpenTag, Values: []string{"on", "off"}},
	}
	view.ReadError = read.Error
	return view
}

func (p *Panel) handleDomainPHPLimits(w http.ResponseWriter, r *http.Request, domainID int) {
	ctx := r.Context()
	w.Header().Set("Cache-Control", "private, no-store")
	site, err := p.loadSiteFileSite(ctx, domainID)
	if err != nil {
		writeClientError(w, http.StatusNotFound, "site not found for domain")
		return
	}
	if site.ProjectType != "php" || services.ValidatePHPVersion(site.PHPVersion) != nil {
		writeSiteHasNoPHP(w, site.Domain, site.ProjectType)
		return
	}
	subID, err := p.domainSubscriptionID(ctx, domainID)
	if err != nil {
		writeClientError(w, http.StatusNotFound, "domain not found")
		return
	}
	db := p.db.GetDB()
	state, err := subscriptionRightsState(ctx, db, subID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	limits, err := subscriptionSettingLimits(ctx, db, subID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	admin := callerIsAdministrator(r)
	read, readErr := p.readSitePHPSettings(ctx, site)
	if r.Method == http.MethodGet {
		if readErr != nil {
			writeCodedError(w, http.StatusBadGateway, errCodePHPSettingsUnread,
				"The site's PHP settings could not be read from the server right now. Nothing was changed. Try again in a moment.", "")
			return
		}
		view := buildPHPLimitsView(site, limits, state.AdminWorkspace, read)
		view.Admin = admin
		view.CanChange = admin
		if !admin {
			e, err := p.effectiveSubscriptionRights(ctx, subID)
			if err != nil {
				writeServerError(w, err)
				return
			}
			view.ClosedBy = e.permissionRefusal(rightPHPSettings)
			view.CanChange = view.ClosedBy == ""
		}
		writeRightsJSON(w, http.StatusOK, view)
		return
	}

	// PUT: the right was checked by the route (php_settings).
	reportOperationPlan(ctx, "check", "pool", "web_server")
	reportOperationStep(ctx, "check")
	var req phpLimitsRequest
	if err := decodeStrictJSON(w, r, &req); err != nil {
		writeClientError(w, http.StatusBadRequest, "invalid PHP settings")
		return
	}
	if readErr != nil {
		writeCodedError(w, http.StatusBadGateway, errCodePHPSettingsUnread,
			"The site's current PHP settings could not be read, so nothing was changed. Try again in a moment.", "")
		return
	}
	settings := req.settings()
	if err := services.ValidateSitePHPSettings(settings); err != nil {
		writeCodedError(w, http.StatusBadRequest, errCodeSettingUnknown, "Nothing was changed: "+err.Error()+".", "")
		return
	}
	if !state.AdminWorkspace {
		values := map[string]int{settingPHPMemoryMB: settings.MemoryMB, settingPHPTimeS: settings.TimeS,
			settingPHPUploadMB: settings.UploadMB, settingPHPInputVars: settings.InputVars, settingPHPProcesses: settings.Processes}
		current := map[string]int{settingPHPMemoryMB: read.Own.MemoryMB, settingPHPTimeS: read.Own.TimeS,
			settingPHPUploadMB: read.Own.UploadMB, settingPHPInputVars: read.Own.InputVars, settingPHPProcesses: read.Processes}
		refusals := []settingRefusal{}
		for _, key := range []string{settingPHPMemoryMB, settingPHPTimeS, settingPHPUploadMB, settingPHPInputVars, settingPHPProcesses} {
			value := values[key]
			if value == 0 {
				continue
			}
			limit := limits[key]
			// A fixed key: only the administrator changes it.
			if limit.Fixed && !admin && value != current[key] {
				writeCodedErrorVars(w, http.StatusForbidden, errCodeSettingAboveLimit,
					"Nothing was changed: the server administrator fixed "+key+" for every site.", "", nil,
					map[string]string{"key": key, "set_by": "fixed"})
				return
			}
			if limit.Ceiling != nil && !limit.allows(value) {
				refusals = append(refusals, settingRefusal{Key: key, Value: value, Limit: *limit.Ceiling, SetBy: limit.SetBy, SetByName: limit.SetByName})
			}
		}
		if len(refusals) > 0 {
			writeSettingAboveLimit(w, refusals)
			return
		}
	}
	previous := read.Own
	previous.Processes = read.Processes

	reportOperationStep(ctx, "pool")
	poolReq := p.sitePoolRequest(ctx, site, transport.SitePoolOperationSettings)
	poolReq.PHPSettings = settings
	if _, err := p.applySitePool(ctx, site, poolReq); err != nil {
		writeServerErrorFor(w, r, err)
		return
	}

	reportOperationStep(ctx, "web_server")
	clamped := settings
	if !state.AdminWorkspace {
		lim := phpLimitsFor(limits)
		if lim.UploadMB > 0 && clamped.UploadMB > lim.UploadMB {
			clamped.UploadMB = lim.UploadMB
		}
		if lim.TimeS > 0 && clamped.TimeS > lim.TimeS {
			clamped.TimeS = lim.TimeS
		}
	}
	timeout := 0
	if clamped.TimeS > 0 {
		timeout = clamped.TimeS + 10
	}
	var nginx transport.SiteSettingsIncludeResponse
	nginxErr := p.callAgentContext(ctx, "Agent.ApplySiteSettingsIncludeV1", &transport.SiteSettingsIncludeRequest{
		ExpectedBuildCommit: strings.TrimSpace(buildCommit), Domain: site.Domain, UploadMB: clamped.UploadMB, TimeoutS: timeout,
	}, &nginx)
	details := map[string]any{"site": site.Domain, "before": previous, "after": settings, "nginx": nginx.Outcome}
	if nginxErr != nil || nginx.Outcome == transport.SiteSettingsIncludeRefused {
		// Put the pool back to what it had, so PHP and nginx agree.
		back := p.sitePoolRequest(ctx, site, transport.SitePoolOperationSettings)
		back.PHPSettings = previous
		_, backErr := p.applySitePool(ctx, site, back)
		details["pool_restored"] = backErr == nil
		p.auditDetailed(r, "site.php_settings.refused:"+site.Domain, "domain", domainID, details)
		line := nginx.Detail
		if nginxErr != nil {
			line = boundedAgentDiagnostic(nginxErr.Error())
		}
		message := "Nothing was changed: the web server refused the new settings file, so the previous file and the previous PHP values were put back."
		if backErr != nil || (nginxErr == nil && !nginx.Restored && nginx.Error != "" && !strings.Contains(nginx.Error, "already fails")) {
			message = "The web server refused the new settings file. Putting everything back could not be confirmed: open this page again to see what is in place now, and ask the server administrator if it does not show your previous values."
		}
		if strings.Contains(nginx.Error, "already fails") {
			message = "Nothing was changed: the server's web server configuration already fails its own check, so no change can be checked. The server administrator must fix it first (Server → Web server)."
		}
		writeCodedErrorVars(w, http.StatusBadGateway, errCodeSiteSettingsNginxRefuse, message, "", nil, map[string]string{"line": line})
		return
	}
	p.auditDetailed(r, "site.php_settings:"+site.Domain, "domain", domainID, details)
	writeRightsJSON(w, http.StatusOK, map[string]any{
		"status": "saved", "nginx": nginx.Outcome, "upload_owner": nginx.UploadOwner, "timeout_owner": nginx.TimeoutOwner,
		"message": fmt.Sprintf("The PHP settings of %s were saved.", site.Domain),
	})
}
