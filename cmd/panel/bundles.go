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
//
// Where bundles are defined (owner question, 2026-10-22): here, in
// bundleDefinitions, compiled into the Panel. They are not read from a file or
// the database and cannot be changed at run time; changing one is a code
// change and a release (D-034, amendment 2026-10-22).
// Paketler burada, bundleDefinitions içinde tanımlıdır ve Panel'e derlenir;
// çalışırken değiştirilemez; değişiklik bir sürüm gerektirir.
//
// A bundle is a service together with its companion tools, never a single
// component (owner decision, 2026-10-22: PostgreSQL with phpPgAdmin is a
// bundle, MariaDB with phpMyAdmin is a bundle). A one-component "bundle" only
// repeats the component's own row; TestBundleRegistryKeepsTheOwnersRule
// refuses one.
// Paket, bir hizmet ile ona eşlik eden araçlardır; tek bileşen paket değildir.

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
	// "verify" (alpha.94-100) is no longer sent: an installed component
	// bundle that is not verified is read again by the page's own rescan.
	// The page still accepts it from an older Panel.
	bundleActionRepair = "repair"
	bundleActionNone   = "none"
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
	bundleTextMailCore      bundleTextCode = "mail_core"
	bundleTextMailWebmail   bundleTextCode = "mail_webmail"
	bundleTextMailProtected bundleTextCode = "mail_protected"
	// alpha.101: the database bundles carry their tool in their name code.
	// The page has no words for the retired one-component codes
	// ("db_postgresql", "db_mariadb"), so an older Panel's answer is named by
	// its components, and a name never claims a tool the bundle does not hold.
	// Veritabanı paketlerinin ad kodu aracını da taşır; eski kodların sözü
	// yoktur, eski yanıt bileşenleriyle adlandırılır.
	bundleTextDBPostgreSQLTool bundleTextCode = "db_postgresql_phppgadmin"
	bundleTextDBMariaDBTool    bundleTextCode = "db_mariadb_phpmyadmin"
	bundleTextWebNginxPHP      bundleTextCode = "web_nginx_php"
)

// bundleTextCodes is every code above: the list the web test reads.
var bundleTextCodes = []bundleTextCode{
	bundleTextMailCore,
	bundleTextMailWebmail,
	bundleTextMailProtected,
	bundleTextDBPostgreSQLTool,
	bundleTextDBMariaDBTool,
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
	// Prerequisites are the components a bundle holds only for the role they
	// fill (the web server and PHP-FPM a browser database tool runs on), not
	// for themselves. Where another member of that role is installed (Apache
	// instead of Nginx), it serves, and the component is "not needed"
	// instead of a conflict. A bundle is never only these.
	// Önkoşullar, pakette yalnız doldurdukları rol için bulunan bileşenlerdir;
	// rolü dolduran başka bir üye kuruluysa o kullanılır.
	Prerequisites []string
	// Parent marks a bundle that is offered only after its parent bundle is
	// installed (an add-on). No bundle has one since alpha.101 (the database
	// tools are inside the database bundles); the answer keeps the field.
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
	// PostgreSQL + phpPgAdmin and MariaDB + phpMyAdmin (alpha.101): the
	// database, the web server and PHP-FPM its browser tool runs on, then the
	// tool. The ids are those of the earlier one-component bundles, so a link
	// to them still lands on the same bundle.
	// Veritabanı, aracının üzerinde çalıştığı web sunucusu ve PHP-FPM, sonra araç.
	{
		ID: "db.postgresql", Category: "database", NameCode: bundleTextDBPostgreSQLTool, Kind: bundleKindComponents,
		Components:    []string{"postgresql", "nginx", "php-fpm", "phppgadmin"},
		Prerequisites: []string{"nginx", "php-fpm"},
	},
	{
		ID: "db.mariadb", Category: "database", NameCode: bundleTextDBMariaDBTool, Kind: bundleKindComponents,
		Components:    []string{"mariadb", "nginx", "php-fpm", "phpmyadmin"},
		Prerequisites: []string{"nginx", "php-fpm"},
	},
	{
		ID: "web.nginx_php", Category: "web", NameCode: bundleTextWebNginxPHP, Kind: bundleKindComponents,
		Components: []string{"nginx", "php-fpm"},
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
//
// unblocked are the same mail profiles derived without the host-wide refusal
// (the DNS identity, the host name): an installed mail bundle stays visible
// as installed beside that refusal instead of turning into a blocked offer.
// unblocked, sunucu geneli ret olmadan türetilen aynı posta profilleridir.
func bundlesView(
	services []ManagedServiceResponse,
	verified bool,
	packageFamily string,
	profiles []MailProfileResponse,
	unblocked []MailProfileResponse,
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
	unblockedByID := make(map[string]MailProfileResponse, len(unblocked))
	for _, profile := range unblocked {
		unblockedByID[profile.ID] = profile
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
			mailBundleState(&view, profileByID[definition.ProfileID], unblockedByID[definition.ProfileID])
		} else {
			componentBundleState(&view, definition, byID, installed, verified, packageFamily, services)
		}
		bundles = append(bundles, view)
	}
	return bundles
}

// bundleComponentPresent: a prerequisite reuses the role it fills when
// another member of that role is installed (phpMyAdmin under Apache needs no
// Nginx). Any other component is installed exactly as named (the Nginx +
// PHP-FPM bundle names Nginx itself, so beside Apache it is blocked).
// Bir önkoşul, rolü dolduran başka bir üye kuruluysa o rolü yeniden kullanır.
func bundleComponentPresent(definition bundleDefinition, id string, installed map[string]bool) bool {
	if installed[id] {
		return true
	}
	if !slices.Contains(definition.Prerequisites, id) {
		return false
	}
	managed := core.GetManagedServiceByID(id)
	return managed != nil && managed.ConflictGroup != "" && core.SeatTakenBy(managed, installed) != ""
}

// mailBundleState takes a mail bundle's state from its mail profile. A mail
// profile that is installed but not proven is repaired, not "verified": the
// only operation that proves a mail setup is the profile install, which
// completes what is missing and reconciles it without removing anything.
// Bir posta paketinin durumu profilinden gelir; kanıtlanmamış kurulu profil
// "onarılır": kanıtı veren tek işlem profil kurulumudur.
func mailBundleState(view *BundleResponse, profile, unblocked MailProfileResponse) {
	view.LatestAttemptStatus = profile.LatestAttemptStatus
	view.WarningCode = profile.WarningCode
	switch profile.Status {
	case mailProfileStatusBlocked:
		view.Status = bundleStatusBlocked
		view.BlockedCode, view.BlockedVars = profile.BlockedCode, profile.BlockedVars
		if mailProfileHostWideCode(profile.BlockedCode) && unblocked.Status == mailProfileStatusComplete {
			// Installed before the host-wide refusal: it stays installed.
			// Nothing can be started while the refusal stands.
			// Sunucu geneli retten önce kurulmuş: kurulu kalır.
			view.Status = bundleStatusInstalledUnverified
			if unblocked.Verified {
				view.Status = bundleStatusInstalledVerified
			}
			view.WarningCode = unblocked.WarningCode
		}
	case mailProfileStatusAvailable:
		view.Status, view.Action = bundleStatusAvailable, bundleActionInstall
	case mailProfileStatusPartial:
		if len(view.MissingComponents) == 0 {
			// Every component is there; what is missing is the proof (the
			// webmail runtime check). That is "installed, not verified", not
			// a half-installed bundle (owner-reported false alarm, alpha.92).
			// Her bileşen var; eksik olan kanıttır: "kurulu, doğrulanmadı".
			view.Status, view.Action = bundleStatusInstalledUnverified, bundleActionRepair
		} else {
			view.Status, view.Action = bundleStatusPartial, bundleActionContinue
		}
	case mailProfileStatusComplete:
		if profile.Verified {
			view.Status, view.Action = bundleStatusInstalledVerified, bundleActionRepair
		} else {
			view.Status, view.Action = bundleStatusInstalledUnverified, bundleActionRepair
		}
	default:
		view.Status = bundleStatusUnknown
		view.BlockedCode, view.BlockedVars = profile.BlockedCode, profile.BlockedVars
	}
}

func mailProfileHostWideCode(code mailProfileTextCode) bool {
	return code == mailProfileBlockedDNSIdentity || code == mailProfileBlockedHostnameUnknown
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
				// Installed but not running: the bundle's dialogue and the
				// rows say which one, and the page's own rescan reads them
				// again (alpha.101: the bundle has no Verify of its own).
				// Kurulu ama çalışmıyor: hangisi olduğunu pencere ve satırlar
				// söyler; sayfanın kendi taraması yeniden okur.
				view.Status, view.Action = bundleStatusInstalledUnverified, bundleActionNone
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
	// A component bundle is never "partial": the parts already present are
	// listed as kept, and Install installs only what is missing.
	// Bileşen paketi hiçbir zaman "kısmen" değildir; Kur yalnız eksikleri kurar.
	view.Status, view.Action = bundleStatusAvailable, bundleActionInstall
}

func bundleByID(id string) (bundleDefinition, bool) {
	index := slices.IndexFunc(bundleDefinitions[:], func(definition bundleDefinition) bool { return definition.ID == id })
	if index < 0 {
		return bundleDefinition{}, false
	}
	return bundleDefinitions[index], true
}
