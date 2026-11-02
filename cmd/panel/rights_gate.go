package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

// The one place that answers "may this subscription use X" (D-042 P2, P6):
//
//	installed ∩ offered by the administrator ∩ the reseller's set ∩ the subscription's set
//
// The installed layer is the Agent's: choosers filter by what it reports and
// acting endpoints keep their own installation checks; the functions here
// apply the other three layers from the Panel database. The administrator's
// own workspace is unrestricted. A right removed later never stops what
// runs (D-038): callers refuse only new use and changes.
//
// Tek karar noktası: kurulu ∩ yöneticinin sunduğu ∩ bayinin kümesi ∩
// aboneliğin kümesi. Yöneticinin kendi çalışma alanı sınırsızdır. Sonradan
// kaldırılan hak çalışanı durdurmaz; yalnız yeni kullanım reddedilir.

const (
	errCodeRightNotGranted = "RIGHT_NOT_GRANTED"
	errCodeNotOffered      = "NOT_OFFERED"

	// settingHiddenOfferings: the administrator's offered layer for items
	// other than PHP and Node.js lines (those keep customer_offered_versions):
	// a JSON list of hidden offering ids and "permission:<id>" entries.
	settingHiddenOfferings = "admin_hidden_offerings"

	// Which layer refused (the refusal's reason).
	rightsLayerAdministrator = "administrator"
	rightsLayerReseller      = "reseller"
	rightsLayerSubscription  = "subscription"
	rightsLayerAddOn         = "add_on"
)

// adminOfferLayer is what the administrator offers on this server.
type adminOfferLayer struct {
	versions offeredVersions
	hidden   map[string]bool
}

func (p *Panel) loadHiddenOfferings(ctx context.Context) (map[string]bool, error) {
	var raw string
	err := p.db.GetDB().QueryRowContext(ctx, `SELECT value FROM panel_settings WHERE key = ?`, settingHiddenOfferings).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return map[string]bool{}, nil
	}
	if err != nil {
		return nil, err
	}
	var list []string
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return nil, errOfferedVersionsUnreadable
	}
	hidden := map[string]bool{}
	for _, item := range list {
		if validRightsItem(item) {
			hidden[item] = true
		}
	}
	return hidden, nil
}

func (p *Panel) saveHiddenOfferings(ctx context.Context, hidden map[string]bool) error {
	list := []string{}
	for item := range hidden {
		list = append(list, item)
	}
	sort.Strings(list)
	raw, err := json.Marshal(list)
	if err != nil {
		return err
	}
	_, err = p.db.GetDB().ExecContext(ctx, `INSERT INTO panel_settings(key,value,updated_at) VALUES(?,?,CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=CURRENT_TIMESTAMP`, settingHiddenOfferings, string(raw))
	return err
}

func (p *Panel) loadAdminOfferLayer(ctx context.Context) (adminOfferLayer, error) {
	versions, err := p.loadOfferedVersions(ctx)
	if err != nil {
		return adminOfferLayer{}, fmt.Errorf("versions offered to customers: %w", err)
	}
	hidden, err := p.loadHiddenOfferings(ctx)
	if err != nil {
		return adminOfferLayer{}, fmt.Errorf("items offered to customers: %w", err)
	}
	return adminOfferLayer{versions: versions, hidden: hidden}, nil
}

func (a adminOfferLayer) permits(permission string) bool {
	return !a.hidden[permissionItemHead+permission]
}

func (a adminOfferLayer) offers(offeringID string) bool {
	if a.hidden[offeringID] {
		return false
	}
	parts := strings.SplitN(offeringID, ":", 3)
	if len(parts) == 3 && parts[0] == "component" {
		if a.hidden["component:"+parts[1]] {
			return false
		}
		switch parts[1] {
		case "php-fpm":
			return a.versions.offered("php", parts[2])
		case "node":
			return a.versions.offered("node", parts[2])
		}
	}
	return true
}

func (a adminOfferLayer) offersAnyVersion(base string) bool {
	return !a.hidden[base]
}

// effectiveRights holds the layers of one subscription.
type effectiveRights struct {
	SubscriptionID int
	Unrestricted   bool
	OwnerID        int
	ResellerID     int
	ResellerName   string
	admin          adminOfferLayer
	reseller       *rightsState
	Own            rightsState
	grants         map[string]bool
}

// effectiveSubscriptionRights loads every layer of a subscription once.
func (p *Panel) effectiveSubscriptionRights(ctx context.Context, subID int) (*effectiveRights, error) {
	db := p.db.GetDB()
	own, err := subscriptionRightsState(ctx, db, subID)
	if err != nil {
		return nil, err
	}
	e := &effectiveRights{SubscriptionID: subID, Own: own, grants: map[string]bool{}}
	if own.AdminWorkspace {
		e.Unrestricted = true
		return e, nil
	}
	e.OwnerID, e.ResellerID, err = subscriptionOwnerChain(ctx, db, subID)
	if err != nil {
		return nil, err
	}
	if e.ResellerID != 0 {
		state, err := resellerRightsState(ctx, db, e.ResellerID)
		if err != nil {
			return nil, err
		}
		e.reseller = &state
		_ = db.QueryRowContext(ctx, `SELECT username FROM users WHERE id = ?`, e.ResellerID).Scan(&e.ResellerName)
	}
	e.admin, err = p.loadAdminOfferLayer(ctx)
	if err != nil {
		return nil, err
	}
	for permission, product := range legacyGrantPermissions {
		e.grants[permission] = p.hasEntitlement(ctx, subID, product)
	}
	return e, nil
}

// permissionRefusal names the layer that refuses a permission ("" = granted).
func (e *effectiveRights) permissionRefusal(permission string) string {
	if e.Unrestricted {
		return ""
	}
	switch {
	case !knownPermission(permission):
		return rightsLayerAdministrator
	case !e.admin.permits(permission):
		return rightsLayerAdministrator
	case e.reseller != nil && !e.reseller.Set.permits(permission):
		return rightsLayerReseller
	case !e.Own.Set.permits(permission):
		return rightsLayerSubscription
	}
	if _, legacy := legacyGrantPermissions[permission]; legacy && !e.grants[permission] {
		return rightsLayerAddOn
	}
	return ""
}

// offeringRefusal names the layer that refuses a software item ("" = offered).
func (e *effectiveRights) offeringRefusal(offeringID string) string {
	if e.Unrestricted {
		return ""
	}
	switch {
	case !validOfferingID(offeringID) || !e.admin.offers(offeringID):
		return rightsLayerAdministrator
	case e.reseller != nil && !e.reseller.Set.offers(offeringID):
		return rightsLayerReseller
	case !e.Own.Set.offers(offeringID):
		return rightsLayerSubscription
	}
	return ""
}

// anyVersionRefusal: like offeringRefusal for "any version of" a component
// (a database engine chosen without its version).
func (e *effectiveRights) anyVersionRefusal(base string) string {
	if e.Unrestricted {
		return ""
	}
	switch {
	case !validOfferingID(base) || !e.admin.offersAnyVersion(base):
		return rightsLayerAdministrator
	case e.reseller != nil && !e.reseller.Set.offersAnyVersion(base):
		return rightsLayerReseller
	case !e.Own.Set.offersAnyVersion(base):
		return rightsLayerSubscription
	}
	return ""
}

func (e *effectiveRights) hasPermission(permission string) bool {
	return e.permissionRefusal(permission) == ""
}

func (e *effectiveRights) offers(offeringID string) bool {
	return e.offeringRefusal(offeringID) == ""
}

// grantor: who can open a refused item, in words.
func (e *effectiveRights) grantor(layer string) string {
	if layer == rightsLayerSubscription && e.ResellerID != 0 && e.OwnerID != e.ResellerID && e.ResellerName != "" {
		return "your provider (" + e.ResellerName + ")"
	}
	return "the administrator"
}

// subscriptionHasRight is the shared gate for permissions (file access, SSH
// and the other ids of rightPermissionIDs). It answers for the subscription
// alone; a caller acting as the administrator is decided by the caller.
func (p *Panel) subscriptionHasRight(ctx context.Context, subscriptionID int64, right string) (bool, error) {
	if !knownPermission(right) {
		return false, fmt.Errorf("%w: permission %q", errUnknownRightsItem, right)
	}
	e, err := p.effectiveSubscriptionRights(ctx, int(subscriptionID))
	if err != nil {
		return false, err
	}
	return e.hasPermission(right), nil
}

// subscriptionOffers answers for one offering id of D-017
// ("component:php-fpm:8.3", "integration:acme:letsencrypt"...).
func (p *Panel) subscriptionOffers(ctx context.Context, subscriptionID int64, offeringID string) (bool, error) {
	if !validOfferingID(offeringID) {
		return false, fmt.Errorf("%w: software %q", errUnknownRightsItem, offeringID)
	}
	e, err := p.effectiveSubscriptionRights(ctx, int(subscriptionID))
	if err != nil {
		return false, err
	}
	return e.offers(offeringID), nil
}

// effectiveRightsView is the answer for screens: every permission and every
// candidate software item with whether it is open and, if not, which layer
// closes it.
type effectiveRightsView struct {
	SubscriptionID int                       `json:"subscription_id"`
	State          string                    `json:"state"`
	PlanID         int                       `json:"plan_id,omitempty"`
	PlanName       string                    `json:"plan_name,omitempty"`
	Unrestricted   bool                      `json:"unrestricted"`
	Own            rightsPayload             `json:"own"`
	Permissions    []effectiveRightsItemView `json:"permissions"`
	Software       []effectiveRightsItemView `json:"software"`
	// InstalledUnknown: the installed list could not be read now; software
	// lists only what the sets name.
	InstalledUnknown bool `json:"installed_unknown,omitempty"`
}

type effectiveRightsItemView struct {
	ID        string `json:"id"`
	Name      string `json:"name,omitempty"`
	Granted   bool   `json:"granted"`
	RefusedBy string `json:"refused_by,omitempty"`
	Grantor   string `json:"grantor,omitempty"`
}

// subscriptionEffectiveRights lists the effective set for a screen.
// candidates are the installed offering ids (installedOfferingIDs).
func (p *Panel) subscriptionEffectiveRights(ctx context.Context, subscriptionID int64, candidates []string) (effectiveRightsView, error) {
	e, err := p.effectiveSubscriptionRights(ctx, int(subscriptionID))
	if err != nil {
		return effectiveRightsView{}, err
	}
	return e.view(candidates), nil
}

func (e *effectiveRights) view(candidates []string) effectiveRightsView {
	v := effectiveRightsView{
		SubscriptionID: e.SubscriptionID, State: e.Own.State, PlanID: e.Own.PlanID, PlanName: e.Own.PlanName,
		Unrestricted: e.Unrestricted, Own: e.Own.Set.payload(),
		Permissions: []effectiveRightsItemView{}, Software: []effectiveRightsItemView{},
	}
	for _, id := range rightPermissionIDs {
		layer := e.permissionRefusal(id)
		item := effectiveRightsItemView{ID: id, Name: rightPermissionNames[id], Granted: layer == "", RefusedBy: layer}
		if layer != "" {
			item.Grantor = e.grantor(layer)
		}
		v.Permissions = append(v.Permissions, item)
	}
	seen := map[string]bool{}
	ids := []string{}
	for _, id := range candidates {
		if validOfferingID(id) && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		layer := e.offeringRefusal(id)
		item := effectiveRightsItemView{ID: id, Granted: layer == "", RefusedBy: layer}
		if layer != "" {
			item.Grantor = e.grantor(layer)
		}
		v.Software = append(v.Software, item)
	}
	return v
}

// installedOfferingIDs lists the offering ids installed on this server, as
// the Agent reports them. A test sets p.rightsInstalledOfferings.
func (p *Panel) installedOfferingIDs(ctx context.Context) ([]string, error) {
	if p.rightsInstalledOfferings != nil {
		return p.rightsInstalledOfferings(ctx)
	}
	var installed []string
	if err := p.callAgentContext(ctx, "Agent.InstalledServiceIDsStrict", &transport.Empty{}, &installed); err != nil {
		return nil, fmt.Errorf("discover installed components: %w", err)
	}
	out := []string{}
	for _, id := range installed {
		if !validOfferingID("component:" + id) {
			continue
		}
		switch id {
		case "php-fpm":
			versions, err := p.phpVersionsFromAgent(ctx)
			if err != nil {
				return nil, err
			}
			for _, version := range versions {
				if offering := versionOfferingID("php", version); offering != "" {
					out = append(out, offering)
				}
			}
		default:
			out = append(out, "component:"+id)
		}
	}
	var node transport.NodeVersionsResponse
	callCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := p.callAgentContext(callCtx, "Agent.ListNodeVersions", &transport.Empty{}, &node); err == nil {
		for _, version := range node.Installed {
			if offering := versionOfferingID("node", version); offering != "" {
				out = append(out, offering)
			}
		}
	}
	for _, provider := range core.ACMEProviders {
		out = append(out, "integration:acme:"+provider.ID)
	}
	return out, nil
}

// writeRightNotGranted: the refusal of a permission, with who can open it.
func writeRightNotGranted(w http.ResponseWriter, e *effectiveRights, permission, layer string) {
	name := rightPermissionNames[permission]
	if name == "" {
		name = permission
	}
	if layer == rightsLayerAddOn {
		// The add-on ledger's own refusal, as before this release.
		writeCodedError(w, http.StatusPaymentRequired, errCodeEntitlement,
			"this feature requires the \""+name+"\" add-on", "/addons")
		return
	}
	grantor := e.grantor(layer)
	var why string
	switch layer {
	case rightsLayerAdministrator:
		why = "the administrator does not offer it on this server"
	case rightsLayerReseller:
		why = "it is not in the reseller's rights"
	case rightsLayerAddOn:
		why = "the subscription has no active \"" + name + "\" add-on"
	default:
		why = "it is not in this subscription's rights"
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	_ = json.NewEncoder(w).Encode(apiErrorBody{
		Error: "Nothing was changed: " + name + " is not available here because " + why + ". " +
			"Ask " + grantor + " to include it; it can be used as soon as it is included. What already runs keeps running.",
		Code: errCodeRightNotGranted, Reason: layer, Action: myPlanPagePath,
		Vars: map[string]string{"right": permission, "name": name, "grantor": grantor},
	})
}

// writeNotOffered: the refusal of a software item, with who can offer it.
func writeNotOffered(w http.ResponseWriter, e *effectiveRights, item, name, layer string) {
	if name == "" {
		name = item
	}
	grantor := e.grantor(layer)
	var why string
	switch layer {
	case rightsLayerAdministrator:
		why = "the administrator does not offer it on this server"
	case rightsLayerReseller:
		why = "it is not in the reseller's rights"
	default:
		why = "it is not in this subscription's rights"
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusConflict)
	_ = json.NewEncoder(w).Encode(apiErrorBody{
		Error: "Nothing was changed: " + name + " is not offered here because " + why + ". " +
			"Choose one that is offered, or ask " + grantor + " to offer it. What already runs keeps running.",
		Code: errCodeNotOffered, Reason: layer, Action: myPlanPagePath,
		Vars: map[string]string{"item": item, "name": name, "grantor": grantor},
	})
}

// loadEffectiveForRequest loads the layers or writes the failure.
func (p *Panel) loadEffectiveForRequest(w http.ResponseWriter, r *http.Request, subID int) *effectiveRights {
	e, err := p.effectiveSubscriptionRights(r.Context(), subID)
	if errors.Is(err, errRightsSubscriptionMissing) {
		writeCodedError(w, http.StatusConflict, errCodeNoSubscription,
			"this account has no subscription yet; ask your provider to assign a plan", myPlanPagePath)
		return nil
	}
	if err != nil {
		writeServerError(w, err)
		return nil
	}
	return e
}

// requireSubscriptionRight: true when the request may go on. The
// administrator acting as itself is never refused.
func (p *Panel) requireSubscriptionRight(w http.ResponseWriter, r *http.Request, subID int, permission string) bool {
	if callerIsAdministrator(r) {
		return true
	}
	e := p.loadEffectiveForRequest(w, r, subID)
	if e == nil {
		return false
	}
	if layer := e.permissionRefusal(permission); layer != "" {
		writeRightNotGranted(w, e, permission, layer)
		return false
	}
	return true
}

// requireSubscriptionOffering: the same for a software item. anyVersion
// accepts any version of the named component.
func (p *Panel) requireSubscriptionOffering(w http.ResponseWriter, r *http.Request, subID int, offeringID, name string, anyVersion bool) bool {
	if callerIsAdministrator(r) {
		return true
	}
	e := p.loadEffectiveForRequest(w, r, subID)
	if e == nil {
		return false
	}
	layer := ""
	if anyVersion {
		layer = e.anyVersionRefusal(offeringID)
	} else {
		layer = e.offeringRefusal(offeringID)
	}
	if layer != "" {
		writeNotOffered(w, e, offeringID, name, layer)
		return false
	}
	return true
}

// requireDomainRight is requireSubscriptionRight for a domain's subscription.
func (p *Panel) requireDomainRight(w http.ResponseWriter, r *http.Request, domainID int, permission string) bool {
	if callerIsAdministrator(r) {
		return true
	}
	subID, err := p.domainSubscriptionID(r.Context(), domainID)
	if err != nil {
		writeClientError(w, http.StatusNotFound, "domain not found")
		return false
	}
	return p.requireSubscriptionRight(w, r, subID, permission)
}

// domainSubrouteRight is the permission a domain sub-route family needs.
// Reads stay open (what exists is shown) unless the read is the use itself
// (files, backup downloads); a removal stays open unless it changes what is
// published (DNS records) or is the use itself (files).
type domainSubrouteRight struct {
	right       string
	gateReads   bool
	gateDeletes bool
}

var domainSubrouteRights = map[string]domainSubrouteRight{
	"dns":                {right: rightDNSEdit, gateDeletes: true},
	"dnssec":             {right: rightDNSSEC, gateDeletes: true},
	"mail":               {right: rightMail},
	"databases":          {right: rightDatabases},
	"cron":               {right: rightCron},
	"files":              {right: rightFileManager, gateReads: true, gateDeletes: true},
	"files-download":     {right: rightFileManager, gateReads: true},
	"backups":            {right: rightBackups},
	"backup-schedule":    {right: rightBackups},
	"backup-restore":     {right: rightBackups},
	"backup-download":    {right: rightBackups, gateReads: true},
	"ssl-letsencrypt":    {right: rightSSLACME},
	"ssl-upload":         {right: rightSSLCustom},
	"site-protect":       {right: rightSiteProtect},
	"site-protect-users": {right: rightSiteProtect},
	"site-redirects":     {right: rightSiteProtect},
	"app-install":        {right: rightAppInstaller},
}

// domainSubrouteRightFor: the permission a request needs, "" when none.
func domainSubrouteRightFor(kind, method string) string {
	rule, ok := domainSubrouteRights[kind]
	if !ok {
		return ""
	}
	switch method {
	case http.MethodOptions, http.MethodHead:
		return ""
	case http.MethodGet:
		if !rule.gateReads {
			return ""
		}
	case http.MethodDelete:
		if !rule.gateDeletes {
			return ""
		}
	}
	return rule.right
}

// enforceDomainSubrouteRight runs after the ownership check of a domain
// sub-route: the subscription must hold the family's permission.
func (p *Panel) enforceDomainSubrouteRight(w http.ResponseWriter, r *http.Request, domainID int, kind string) bool {
	right := domainSubrouteRightFor(kind, r.Method)
	if right == "" {
		return true
	}
	return p.requireDomainRight(w, r, domainID, right)
}

// subscriptionVersionPolicy is the administrator's version policy narrowed
// by the subscription's rights: what a customer of this subscription may
// choose for a site.
func (p *Panel) subscriptionVersionPolicy(ctx context.Context, subID int) (offeredVersions, *effectiveRights, error) {
	e, err := p.effectiveSubscriptionRights(ctx, subID)
	if err != nil {
		return offeredVersions{}, nil, err
	}
	policy := e.admin.versions
	if e.Unrestricted {
		policy, err = p.loadOfferedVersions(ctx)
		if err != nil {
			return offeredVersions{}, nil, err
		}
		return policy, e, nil
	}
	policy.allow = func(component, line string) bool {
		offering := versionOfferingID(component, line)
		return offering != "" && e.offers(offering)
	}
	return policy, e, nil
}

// writeVersionRefusal answers a version a subscription may not choose: the
// administrator's VERSION_NOT_OFFERED when the server does not offer it,
// else NOT_OFFERED naming the plan's or reseller's layer.
func writeVersionRefusal(w http.ResponseWriter, e *effectiveRights, component, version string) {
	offering := versionOfferingID(component, version)
	if e == nil || offering == "" {
		writeVersionNotOffered(w, component, version)
		return
	}
	layer := e.offeringRefusal(offering)
	if layer == "" || layer == rightsLayerAdministrator {
		writeVersionNotOffered(w, component, version)
		return
	}
	writeNotOffered(w, e, offering, offeredComponents[component]+" "+offeredVersionLine(component, version), layer)
}

// writeNoVersionRefusal: no installed version is open to the subscription.
func writeNoVersionRefusal(w http.ResponseWriter, e *effectiveRights, component string, installed []string) {
	if e != nil && !e.Unrestricted {
		admin := e.admin.versions.filter(component, installed)
		if len(admin) > 0 {
			writeVersionRefusal(w, e, component, admin[0])
			return
		}
	}
	writeNoVersionOffered(w, component)
}
