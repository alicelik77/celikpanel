package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/alicelik/celikpanel/internal/core"
)

// Rights sets (D-042, docs/PLANS-AND-RIGHTS.md, migration 052). A rights set
// has three parts: quantities, permissions and software. Quantities keep
// their existing homes (service_plans.max_*, subscriptions.max_*,
// reseller_pools); permissions and software are stored here as items, or as
// the "everything offered" flags of the built-in Unlimited plans and of the
// records migrated from before this release.
//
// Hak kümesi: sayılar, izinler, yazılımlar. Sayılar eski yerlerinde kalır;
// izin ve yazılımlar burada öğe ya da "sunulan her şey" bayrağı olarak durur.

// The closed set of permission ids (section 4.2).
const (
	rightDNSEdit       = "dns_edit"
	rightDNSSEC        = "dnssec"
	rightMail          = "mail"
	rightDatabases     = "databases"
	rightCron          = "cron"
	rightFileManager   = "file_manager"
	rightBackups       = "backups"
	rightSSLACME       = "ssl_acme"
	rightSSLCustom     = "ssl_custom"
	rightSiteProtect   = "site_protect_redirects"
	rightSiteProxy     = "site_proxy"
	rightAppInstaller  = "app_installer"
	rightVPN           = "vpn"
	rightTeamMembers   = "team_members"
	rightFileAccess    = "file_access"
	rightSSH           = "ssh"
	permissionItemHead = "permission:"
)

// rightServiceApps (alpha.118): AI and automation apps that run as the site's
// service (docs/AI-APPS.md). Each app is also its own offering, app:<id>,
// closed until the administrator offers it.
const rightServiceApps = "service_apps"

// rightPermissionIDs lists every permission in a fixed order with the name a
// sentence uses.
var rightPermissionIDs = []string{
	rightDNSEdit, rightDNSSEC, rightMail, rightDatabases, rightCron, rightFileManager,
	rightBackups, rightSSLACME, rightSSLCustom, rightSiteProtect, rightSiteProxy,
	rightAppInstaller, rightVPN, rightTeamMembers, rightFileAccess, rightSSH,
	rightRemoteBackupDestinations,
	rightServiceApps,
}

var rightPermissionNames = map[string]string{
	rightDNSEdit:      "DNS record editing",
	rightDNSSEC:       "DNSSEC",
	rightMail:         "Mail",
	rightDatabases:    "Databases",
	rightCron:         "Scheduled tasks",
	rightFileManager:  "File manager",
	rightBackups:      "Backups",
	rightSSLACME:      "Free certificates (ACME)",
	rightSSLCustom:    "Uploading your own certificate",
	rightSiteProtect:  "Protected folders and redirects",
	rightSiteProxy:    "Proxy site type",
	rightAppInstaller: "App installer",
	rightVPN:          "VPN",
	rightTeamMembers:  "Additional users",
	rightFileAccess:   "File access (FTP and SFTP)",
	rightSSH:          "SSH (command line)",

	// alpha.118 (remote_backup_model.go).
	rightRemoteBackupDestinations: "Own remote backup destinations",
	rightServiceApps:              "AI and automation apps",
}

// legacyGrantPermissions are the two product permissions that still need an
// active add-on grant (subscription_entitlements) as well: the VPN server
// serves peers by that ledger, so it stays a layer until VPN serving moves
// to rights sets.
var legacyGrantPermissions = map[string]string{rightVPN: vpnProductID, rightAppInstaller: "app_installer"}

func knownPermission(id string) bool {
	_, ok := rightPermissionNames[id]
	return ok
}

// Offering ids of D-017: component:<id>[:<version>], integration:acme:<id>,
// extension:<vendor>:<name>, app:<id> (alpha.118: one namespace for both app
// catalogs, the PHP application catalog and the AI and automation service
// apps; their ids never overlap). Lowercase, bounded.
var offeringIDPattern = regexp.MustCompile(`^(component:[a-z0-9][a-z0-9-]{0,40}(:[0-9][0-9a-z.]{0,20})?|integration:acme:[a-z0-9][a-z0-9-]{0,40}|extension:[a-z0-9][a-z0-9-]{0,40}:[a-z0-9][a-z0-9-]{0,60}|app:[a-z][a-z0-9-]{1,40})$`)

// validOfferingID: the shape of an offering id, and never an administrator-only
// component (alpha.119): the developer toolchains (Go, Rust) have no offering id,
// so no plan, reseller set, hidden-offering list or store product can name them.
// validOfferingID: bir sunum kimliğinin biçimi; yalnız yöneticinin bileşeni
// (Go, Rust araç zincirleri) asla sunum kimliği olamaz.
func validOfferingID(id string) bool {
	if !offeringIDPattern.MatchString(id) {
		return false
	}
	if rest, ok := strings.CutPrefix(id, "component:"); ok {
		component, _, _ := strings.Cut(rest, ":")
		return !core.IsAdminOnlyComponent(component)
	}
	return true
}

// validRightsItem: "permission:<known id>" or a valid offering id.
func validRightsItem(item string) bool {
	if id, ok := strings.CutPrefix(item, permissionItemHead); ok {
		return knownPermission(id)
	}
	return validOfferingID(item)
}

// versionOfferingID is the offering id of a PHP or Node.js version line.
func versionOfferingID(component, version string) string {
	line := offeredVersionLine(component, version)
	if line == "" {
		return ""
	}
	switch component {
	case "php":
		return "component:php-fpm:" + line
	case "node":
		return "component:node:" + line
	}
	return ""
}

// databaseEngineOffering: the offering a database engine is chosen by.
// PostgreSQL is offered per version; a request names the engine only, so it
// is checked with offersAnyVersion.
func databaseEngineOffering(engine string) string {
	switch engine {
	case "mysql", "mariadb":
		return "component:mariadb"
	case "postgresql", "postgres":
		return "component:postgresql"
	}
	return ""
}

// rightsSet is the permission and software part of one set.
type rightsSet struct {
	AllPermissions bool
	AllSoftware    bool
	Items          map[string]bool
}

func allOfferedRightsSet() rightsSet {
	return rightsSet{AllPermissions: true, AllSoftware: true, Items: map[string]bool{}}
}

func newRightsSet(allPermissions, allSoftware bool, items ...string) rightsSet {
	s := rightsSet{AllPermissions: allPermissions, AllSoftware: allSoftware, Items: map[string]bool{}}
	for _, item := range items {
		s.Items[item] = true
	}
	return s
}

func (s rightsSet) clone() rightsSet {
	out := rightsSet{AllPermissions: s.AllPermissions, AllSoftware: s.AllSoftware, Items: map[string]bool{}}
	for item := range s.Items {
		out.Items[item] = true
	}
	return out
}

func (s rightsSet) permits(permission string) bool {
	return s.AllPermissions || s.Items[permissionItemHead+permission]
}

// offers: the exact item, or a whole component that covers its versions.
func (s rightsSet) offers(offeringID string) bool {
	if s.AllSoftware || s.Items[offeringID] {
		return true
	}
	if strings.HasPrefix(offeringID, "component:") {
		parts := strings.SplitN(offeringID, ":", 3)
		if len(parts) == 3 && s.Items[parts[0]+":"+parts[1]] {
			return true
		}
	}
	return false
}

// offersAnyVersion: the whole component or at least one of its versions.
func (s rightsSet) offersAnyVersion(base string) bool {
	if s.AllSoftware || s.Items[base] {
		return true
	}
	for item := range s.Items {
		if strings.HasPrefix(item, base+":") {
			return true
		}
	}
	return false
}

func (s rightsSet) permissionList() []string {
	out := []string{}
	for _, id := range rightPermissionIDs {
		if s.Items[permissionItemHead+id] {
			out = append(out, id)
		}
	}
	return out
}

func (s rightsSet) softwareList() []string {
	out := []string{}
	for item := range s.Items {
		if !strings.HasPrefix(item, permissionItemHead) {
			out = append(out, item)
		}
	}
	sort.Strings(out)
	return out
}

func (s rightsSet) itemList() []string {
	out := make([]string, 0, len(s.Items))
	for item := range s.Items {
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}

// rightsPayload is the JSON form of a set's permission and software parts.
type rightsPayload struct {
	AllPermissions bool     `json:"all_permissions"`
	AllSoftware    bool     `json:"all_software"`
	Permissions    []string `json:"permissions"`
	Software       []string `json:"software"`
}

func (s rightsSet) payload() rightsPayload {
	return rightsPayload{AllPermissions: s.AllPermissions, AllSoftware: s.AllSoftware,
		Permissions: s.permissionList(), Software: s.softwareList()}
}

var errUnknownRightsItem = errors.New("unknown right")

// set validates a payload. Unknown permissions or malformed offering ids are
// refused; an "all" flag drops the listed items of that part.
func (pl rightsPayload) set() (rightsSet, error) {
	s := newRightsSet(pl.AllPermissions, pl.AllSoftware)
	if len(pl.Permissions) > 64 || len(pl.Software) > 256 {
		return s, fmt.Errorf("%w: too many items", errUnknownRightsItem)
	}
	for _, id := range pl.Permissions {
		id = strings.TrimSpace(id)
		if !knownPermission(id) {
			return s, fmt.Errorf("%w: permission %q", errUnknownRightsItem, id)
		}
		if !pl.AllPermissions {
			s.Items[permissionItemHead+id] = true
		}
	}
	for _, id := range pl.Software {
		id = strings.TrimSpace(id)
		if !validOfferingID(id) {
			return s, fmt.Errorf("%w: software %q", errUnknownRightsItem, id)
		}
		if !pl.AllSoftware {
			s.Items[id] = true
		}
	}
	return s, nil
}

// Owner kinds of rights_sets.
const (
	rightsOwnerPlan         = "plan"
	rightsOwnerResellerPlan = "reseller_plan"
	rightsOwnerSubscription = "subscription"
	rightsOwnerReseller     = "reseller"
)

// loadRightsSet reads one owner's set; found is false when it has none.
func loadRightsSet(ctx context.Context, q rowQueryer, ownerKind string, ownerID int) (rightsSet, bool, error) {
	var setID int
	var allPermissions, allSoftware int
	err := q.QueryRowContext(ctx, `SELECT id, all_permissions, all_software FROM rights_sets WHERE owner_kind = ? AND owner_id = ?`,
		ownerKind, ownerID).Scan(&setID, &allPermissions, &allSoftware)
	if errors.Is(err, sql.ErrNoRows) {
		return rightsSet{}, false, nil
	}
	if err != nil {
		return rightsSet{}, false, fmt.Errorf("read rights set: %w", err)
	}
	s := newRightsSet(allPermissions == 1, allSoftware == 1)
	rows, err := q.QueryContext(ctx, `SELECT item FROM rights_set_items WHERE set_id = ?`, setID)
	if err != nil {
		return rightsSet{}, false, fmt.Errorf("read rights set items: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var item string
		if err := rows.Scan(&item); err != nil {
			return rightsSet{}, false, err
		}
		// An item this release does not know grants nothing (fail closed).
		if validRightsItem(item) {
			s.Items[item] = true
		}
	}
	return s, true, rows.Err()
}

type rightsTx interface {
	rowQueryer
	sqlExecer
}

// saveRightsSet replaces one owner's set.
func saveRightsSet(ctx context.Context, tx rightsTx, ownerKind string, ownerID int, s rightsSet) error {
	// alpha.118: an app closed to resellers by its licence (n8n) is never
	// written into a reseller's set or a set in a reseller's tree.
	if err := refuseLicenceClosedRights(ctx, tx, ownerKind, ownerID, s); err != nil {
		return err
	}
	boolInt := func(b bool) int {
		if b {
			return 1
		}
		return 0
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO rights_sets (owner_kind, owner_id, all_permissions, all_software)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(owner_kind, owner_id) DO UPDATE SET
		  all_permissions = excluded.all_permissions, all_software = excluded.all_software,
		  updated_at = datetime('now')`,
		ownerKind, ownerID, boolInt(s.AllPermissions), boolInt(s.AllSoftware)); err != nil {
		return fmt.Errorf("save rights set: %w", err)
	}
	var setID int
	if err := tx.QueryRowContext(ctx, `SELECT id FROM rights_sets WHERE owner_kind = ? AND owner_id = ?`, ownerKind, ownerID).Scan(&setID); err != nil {
		return fmt.Errorf("read saved rights set: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM rights_set_items WHERE set_id = ?`, setID); err != nil {
		return fmt.Errorf("replace rights set items: %w", err)
	}
	for _, item := range s.itemList() {
		if s.AllPermissions && strings.HasPrefix(item, permissionItemHead) {
			continue
		}
		if s.AllSoftware && !strings.HasPrefix(item, permissionItemHead) {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO rights_set_items (set_id, item) VALUES (?, ?)`, setID, item); err != nil {
			return fmt.Errorf("save rights set item: %w", err)
		}
	}
	return nil
}

func deleteRightsSet(ctx context.Context, tx sqlExecer, ownerKind string, ownerID int) error {
	_, err := tx.ExecContext(ctx, `DELETE FROM rights_sets WHERE owner_kind = ? AND owner_id = ?`, ownerKind, ownerID)
	return err
}

// Account states (P3).
const (
	rightsStateUnlimited = "unlimited"
	rightsStatePlan      = "plan"
	rightsStateCustom    = "custom"
)

// rightsState is where one account's own layer comes from.
type rightsState struct {
	State    string
	PlanID   int
	PlanName string
	Set      rightsSet
	// Implicit: a Custom account without a stored set (made by a path that
	// does not write one); it is read as everything offered.
	Implicit bool
	// AdminWorkspace: the administrator's own subscription; unrestricted.
	AdminWorkspace bool
}

// stateLabel is "unlimited", "plan:<id>" or "custom".
func (s rightsState) stateLabel() string {
	if s.State == rightsStatePlan {
		return fmt.Sprintf("plan:%d", s.PlanID)
	}
	return s.State
}

var errRightsSubscriptionMissing = errors.New("subscription not found")

// subscriptionRightsState reads a subscription's own layer: bound to a plan
// (plan_id), Custom (its own set) or Unlimited. A subscription without a plan
// and without a set whose numbers are not all unlimited is an implicit Custom
// (a path that does not know rights sets made it).
func subscriptionRightsState(ctx context.Context, q rowQueryer, subID int) (rightsState, error) {
	var planID sql.NullInt64
	var planName sql.NullString
	var name, ownerRole string
	var domains, databases, mailboxes, disk sql.NullInt64
	err := q.QueryRowContext(ctx, `
		SELECT s.plan_id, sp.name, s.name, COALESCE(u.role, ''),
		       s.max_domains, s.max_databases, s.max_email_accounts, s.disk_quota_mb
		FROM subscriptions s
		LEFT JOIN service_plans sp ON sp.id = s.plan_id
		LEFT JOIN users u ON u.id = s.owner_id
		WHERE s.id = ?`, subID).Scan(&planID, &planName, &name, &ownerRole, &domains, &databases, &mailboxes, &disk)
	if errors.Is(err, sql.ErrNoRows) {
		return rightsState{}, errRightsSubscriptionMissing
	}
	if err != nil {
		return rightsState{}, fmt.Errorf("read subscription rights state: %w", err)
	}
	if planID.Valid && planID.Int64 > 0 {
		set, found, err := loadRightsSet(ctx, q, rightsOwnerPlan, int(planID.Int64))
		if err != nil {
			return rightsState{}, err
		}
		if !found {
			set = allOfferedRightsSet()
		}
		return rightsState{State: rightsStatePlan, PlanID: int(planID.Int64), PlanName: planName.String, Set: set}, nil
	}
	if ownerRole == roleAdmin && name == "Admin Subscription" {
		return rightsState{State: rightsStateUnlimited, Set: allOfferedRightsSet(), AdminWorkspace: true}, nil
	}
	set, found, err := loadRightsSet(ctx, q, rightsOwnerSubscription, subID)
	if err != nil {
		return rightsState{}, err
	}
	if found {
		return rightsState{State: rightsStateCustom, Set: set}, nil
	}
	if domains.Int64 <= 0 && databases.Int64 <= 0 && mailboxes.Int64 <= 0 && disk.Int64 <= 0 {
		return rightsState{State: rightsStateUnlimited, Set: allOfferedRightsSet()}, nil
	}
	return rightsState{State: rightsStateCustom, Set: allOfferedRightsSet(), Implicit: true}, nil
}

// resellerRightsState reads a reseller's own layer: bound to a reseller
// plan, Custom (its own set, or a pool without a set) or Unlimited reseller.
func resellerRightsState(ctx context.Context, q rowQueryer, resellerID int) (rightsState, error) {
	var planID int
	var planName string
	err := q.QueryRowContext(ctx, `
		SELECT b.reseller_plan_id, rp.name FROM reseller_plan_bindings b
		JOIN reseller_plans rp ON rp.id = b.reseller_plan_id
		WHERE b.reseller_id = ?`, resellerID).Scan(&planID, &planName)
	if err == nil {
		set, found, err := loadRightsSet(ctx, q, rightsOwnerResellerPlan, planID)
		if err != nil {
			return rightsState{}, err
		}
		if !found {
			set = allOfferedRightsSet()
		}
		return rightsState{State: rightsStatePlan, PlanID: planID, PlanName: planName, Set: set}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return rightsState{}, fmt.Errorf("read reseller plan binding: %w", err)
	}
	set, found, err := loadRightsSet(ctx, q, rightsOwnerReseller, resellerID)
	if err != nil {
		return rightsState{}, err
	}
	if found {
		return rightsState{State: rightsStateCustom, Set: set}, nil
	}
	_, hasPool, err := loadResellerPool(ctx, q, resellerID)
	if err != nil {
		return rightsState{}, err
	}
	if hasPool {
		return rightsState{State: rightsStateCustom, Set: allOfferedRightsSet(), Implicit: true}, nil
	}
	return rightsState{State: rightsStateUnlimited, Set: allOfferedRightsSet()}, nil
}

// subscriptionOwnerChain: the subscription's owner and the reseller whose
// tree it is in (0 when none: the administrator's own tree).
func subscriptionOwnerChain(ctx context.Context, q rowQueryer, subID int) (ownerID, resellerID int, err error) {
	if err := q.QueryRowContext(ctx, `SELECT owner_id FROM subscriptions WHERE id = ?`, subID).Scan(&ownerID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, 0, errRightsSubscriptionMissing
		}
		return 0, 0, err
	}
	resellerID, err = treeResellerID(ctx, q, ownerID)
	return ownerID, resellerID, err
}
