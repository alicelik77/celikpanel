package main

import (
	"slices"

	"github.com/alicelik/celikpanel/internal/core"
)

// Ready bundles (alpha.94, owner decision of 2026-10-17, D-034) are shortcuts
// over the catalogue's own components. A bundle is not a product, not a
// component and not a page: it names a group of catalogue components that are
// installed together, and the Components page shows it at the head of the
// category it belongs to. The catalogue stays the single install address
// (D-015 item 2): a mail bundle runs the existing mail profile install, and a
// database or web bundle is the existing single-component install, once per
// missing component, in order. Nothing here changes the host; this file only
// reads the scan and the mail profiles and says what each bundle's state is.
//
// Hazır paketler, kataloğun kendi bileşenleri üzerinde kısayollardır; ürün,
// bileşen ya da sayfa değildir. Katalog tek kurulum adresi olarak kalır. Bu
// dosya sunucuyu değiştirmez; yalnız taramayı ve posta profillerini okuyup her
// paketin durumunu söyler.

const (
	bundleKindMailProfile = "mail_profile"
	bundleKindComponents  = "components"

	bundleStatusUnknown             = "unknown"
	bundleStatusAvailable           = "available"
	bundleStatusPartial             = "partial"
	bundleStatusInstalledUnverified = "installed_unverified"
	bundleStatusInstalledVerified   = "installed_verified"
	bundleStatusBlocked             = "blocked"

	bundleActionInstall  = "install"
	bundleActionContinue = "continue"
	bundleActionVerify   = "verify"
	bundleActionRepair   = "repair"
	bundleActionNone     = "none"
)

// bundleTextCode names a bundle's words: its name, its one-sentence
// description and, where it is offered after another bundle, its "add …"
// label. Every constant of this type must be in bundleTextCodes
// (TestBundleTextCodesAreListed), and web/tests/ready-bundles-ui-contract.test.mjs
// reads that list from this file and requires English and Turkish words for
// each, so a bundle cannot reach a page without a name.
// bundleTextCode bir paketin sözlerini adlandırır; her biri listede ve iki
// dilde sözü olmak zorundadır.
type bundleTextCode string

const (
	bundleTextMailCore          bundleTextCode = "mail_core"
	bundleTextMailWebmail       bundleTextCode = "mail_webmail"
	bundleTextMailProtected     bundleTextCode = "mail_protected"
	bundleTextDBPostgreSQL      bundleTextCode = "db_postgresql"
	bundleTextDBMariaDB         bundleTextCode = "db_mariadb"
	bundleTextDBPostgreSQLAdmin bundleTextCode = "db_postgresql_admin"
	bundleTextDBMariaDBAdmin    bundleTextCode = "db_mariadb_admin"
	bundleTextWebNginxPHP       bundleTextCode = "web_nginx_php"
)

// bundleTextCodes is every code above: the list the web test reads.
var bundleTextCodes = []bundleTextCode{
	bundleTextMailCore,
	bundleTextMailWebmail,
	bundleTextMailProtected,
	bundleTextDBPostgreSQL,
	bundleTextDBMariaDB,
	bundleTextDBPostgreSQLAdmin,
	bundleTextDBMariaDBAdmin,
	bundleTextWebNginxPHP,
}

type bundleDefinition struct {
	ID       string
	Category string
	NameCode bundleTextCode
	Kind     string
	// ProfileID is the mail profile a mail bundle installs through.
	ProfileID string
	// Components are the catalogue components, in install order.
	Components []string
	// Anchors are the components whose presence means this bundle was
	// started. Without one of them present the bundle is "available" even
	// when shared parts (Nginx, PHP-FPM) are already on the server; those are
	// then skipped, not counted as a half-installed bundle.
	Anchors []string
	// Parent marks a bundle that is offered only after its parent bundle is
	// installed (an add-on). A top-level bundle has none.
	Parent string
	// Addons are the bundles offered after this one is installed, in order;
	// the page shows at most one.
	Addons []string
}

var bundleDefinitions = [...]bundleDefinition{
	{
		ID: "mail.core", Category: "email", NameCode: bundleTextMailCore, Kind: bundleKindMailProfile,
		ProfileID: core.MailProfileCore, Components: mustMailProfileServiceIDs(core.MailProfileCore),
		Addons: []string{"mail.protected"},
	},
	{
		ID: "mail.webmail", Category: "email", NameCode: bundleTextMailWebmail, Kind: bundleKindMailProfile,
		ProfileID: core.MailProfileWebmail, Components: mustMailProfileServiceIDs(core.MailProfileWebmail),
		Addons: []string{"mail.protected"},
	},
	{
		ID: "mail.protected", Category: "email", NameCode: bundleTextMailProtected, Kind: bundleKindMailProfile,
		ProfileID: core.MailProfileProtected, Components: mustMailProfileServiceIDs(core.MailProfileProtected),
	},
	{
		ID: "db.postgresql", Category: "database", NameCode: bundleTextDBPostgreSQL, Kind: bundleKindComponents,
		Components: []string{"postgresql"}, Anchors: []string{"postgresql"},
		Addons: []string{"db.postgresql.admin"},
	},
	{
		ID: "db.mariadb", Category: "database", NameCode: bundleTextDBMariaDB, Kind: bundleKindComponents,
		Components: []string{"mariadb"}, Anchors: []string{"mariadb"},
		Addons: []string{"db.mariadb.admin"},
	},
	{
		ID: "db.postgresql.admin", Category: "database", NameCode: bundleTextDBPostgreSQLAdmin, Kind: bundleKindComponents,
		Components: []string{"nginx", "php-fpm", "phppgadmin"}, Anchors: []string{"phppgadmin"},
		Parent: "db.postgresql",
	},
	{
		ID: "db.mariadb.admin", Category: "database", NameCode: bundleTextDBMariaDBAdmin, Kind: bundleKindComponents,
		Components: []string{"nginx", "php-fpm", "phpmyadmin"}, Anchors: []string{"phpmyadmin"},
		Parent: "db.mariadb",
	},
	{
		ID: "web.nginx_php", Category: "web", NameCode: bundleTextWebNginxPHP, Kind: bundleKindComponents,
		Components: []string{"nginx", "php-fpm"}, Anchors: []string{"nginx", "php-fpm"},
	},
}

// BundleResponse is derived entirely on the server, as MailProfileResponse
// is: the page chooses a bundle; it never supplies its components.
// BundleResponse tamamen sunucuda türetilir; sayfa bileşen listesi veremez.
type BundleResponse struct {
	ID         string         `json:"id"`
	Category   string         `json:"category"`
	NameCode   bundleTextCode `json:"name_code"`
	Components []string       `json:"components"`
	Status     string         `json:"status"`
	Action     string         `json:"action"`
	// ActionRoute says how the action runs: the mail profile install
	// (mail_profile) or one component install per missing component
	// (components).
	ActionRoute string   `json:"action_route"`
	ProfileID   string   `json:"profile_id,omitempty"`
	Parent      string   `json:"parent,omitempty"`
	Addons      []string `json:"addons"`
	// The typed sentences are the mail profiles' own codes and values.
	BlockedCode         mailProfileTextCode `json:"blocked_code,omitempty"`
	BlockedVars         map[string]string   `json:"blocked_vars,omitempty"`
	WarningCode         mailProfileTextCode `json:"warning_code,omitempty"`
	LatestAttemptStatus string              `json:"latest_attempt_status,omitempty"`
	// MissingComponents are the components a bundle install would install
	// now, in order; already installed ones are skipped.
	MissingComponents []string `json:"missing_components"`
}

// bundlesView says each bundle's state. Mail bundles take their state from
// the mail profiles (the same scan, the same proofs); database and web
// bundles are verified when every component is ready on a fresh scan.
// bundlesView her paketin durumunu söyler.
func bundlesView(
	services []ManagedServiceResponse,
	verified bool,
	packageFamily string,
	profiles []MailProfileResponse,
) []BundleResponse {
	byID := make(map[string]ManagedServiceResponse, len(services))
	installed := make(map[string]bool, len(services))
	for _, service := range services {
		byID[service.ID] = service
		if service.Installed() {
			installed[service.ID] = true
		}
	}
	profileByID := make(map[string]MailProfileResponse, len(profiles))
	for _, profile := range profiles {
		profileByID[profile.ID] = profile
	}

	bundles := make([]BundleResponse, 0, len(bundleDefinitions))
	for _, definition := range bundleDefinitions {
		view := BundleResponse{
			ID:                definition.ID,
			Category:          definition.Category,
			NameCode:          definition.NameCode,
			Components:        append([]string(nil), definition.Components...),
			Status:            bundleStatusUnknown,
			Action:            bundleActionNone,
			ActionRoute:       definition.Kind,
			ProfileID:         definition.ProfileID,
			Parent:            definition.Parent,
			Addons:            append([]string{}, definition.Addons...),
			MissingComponents: []string{},
		}
		for _, id := range definition.Components {
			if !bundleComponentPresent(definition, id, installed) {
				view.MissingComponents = append(view.MissingComponents, id)
			}
		}
		if definition.Kind == bundleKindMailProfile {
			mailBundleState(&view, profileByID[definition.ProfileID])
		} else {
			componentBundleState(&view, definition, byID, installed, verified, packageFamily, services)
		}
		bundles = append(bundles, view)
	}
	return bundles
}

// bundleComponentPresent: an add-on reuses the role a component fills when
// another member of that role is installed (phpMyAdmin under Apache needs no
// Nginx). A top-level bundle installs exactly what it names.
// Bir ek, rolü dolduran başka bir üye kuruluysa o rolü yeniden kullanır.
func bundleComponentPresent(definition bundleDefinition, id string, installed map[string]bool) bool {
	if installed[id] {
		return true
	}
	if definition.Parent == "" {
		return false
	}
	managed := core.GetManagedServiceByID(id)
	return managed != nil && managed.ConflictGroup != "" && core.SeatTakenBy(managed, installed) != ""
}

func mailBundleState(view *BundleResponse, profile MailProfileResponse) {
	view.LatestAttemptStatus = profile.LatestAttemptStatus
	view.WarningCode = profile.WarningCode
	switch profile.Status {
	case mailProfileStatusBlocked:
		view.Status = bundleStatusBlocked
		view.BlockedCode, view.BlockedVars = profile.BlockedCode, profile.BlockedVars
	case mailProfileStatusAvailable:
		view.Status, view.Action = bundleStatusAvailable, bundleActionInstall
	case mailProfileStatusPartial:
		if len(view.MissingComponents) == 0 {
			// Every component is there; what is missing is the proof (the
			// webmail runtime check). That is "installed, not verified", not
			// a half-installed bundle (owner-reported false alarm, alpha.92).
			// Her bileşen var; eksik olan kanıttır: "kurulu, doğrulanmadı".
			view.Status, view.Action = bundleStatusInstalledUnverified, bundleActionVerify
		} else {
			view.Status, view.Action = bundleStatusPartial, bundleActionContinue
		}
	case mailProfileStatusComplete:
		if profile.Verified {
			view.Status, view.Action = bundleStatusInstalledVerified, bundleActionRepair
		} else {
			view.Status, view.Action = bundleStatusInstalledUnverified, bundleActionVerify
		}
	default:
		view.Status = bundleStatusUnknown
		view.BlockedCode, view.BlockedVars = profile.BlockedCode, profile.BlockedVars
	}
}

func componentBundleState(
	view *BundleResponse,
	definition bundleDefinition,
	byID map[string]ManagedServiceResponse,
	installed map[string]bool,
	verified bool,
	packageFamily string,
	services []ManagedServiceResponse,
) {
	if !verified {
		view.BlockedCode = mailProfileBlockedStateUnverified
		return
	}
	for _, id := range definition.Components {
		if _, ok := byID[id]; !ok {
			view.BlockedCode = mailProfileBlockedStateIncomplete
			return
		}
	}
	if len(view.MissingComponents) == 0 {
		for _, id := range definition.Components {
			if installed[id] && !componentReady(id, services) {
				// Installed but not running: the rows below say which one.
				// "Verify" reads the components again; it changes nothing.
				view.Status, view.Action = bundleStatusInstalledUnverified, bundleActionVerify
				return
			}
		}
		view.Status, view.Action = bundleStatusInstalledVerified, bundleActionNone
		return
	}

	projected := make(map[string]bool, len(installed)+len(definition.Components))
	for id := range installed {
		projected[id] = true
	}
	for _, id := range view.MissingComponents {
		projected[id] = true
	}
	if _, code, vars := componentPlanBlock(view.MissingComponents, projected, packageFamily); code != "" {
		view.Status = bundleStatusBlocked
		view.BlockedCode, view.BlockedVars = code, vars
		return
	}
	started := false
	for _, id := range definition.Anchors {
		if installed[id] {
			started = true
		}
	}
	if started {
		view.Status, view.Action = bundleStatusPartial, bundleActionContinue
		return
	}
	view.Status, view.Action = bundleStatusAvailable, bundleActionInstall
}

func bundleByID(id string) (bundleDefinition, bool) {
	index := slices.IndexFunc(bundleDefinitions[:], func(definition bundleDefinition) bool { return definition.ID == id })
	if index < 0 {
		return bundleDefinition{}, false
	}
	return bundleDefinitions[index], true
}
