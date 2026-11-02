package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Subscription transfer (D-042, docs/PLANS-AND-RIGHTS.md section 8, K3).
// The unit is one subscription with everything in it.
//
// Host impact of the owner change: none. Everything on the server is keyed
// by ids that do not change in a transfer: site homes are .../subscriptions/<subscription
// id>/sites/<domain id> (internal/hostingpath, services.SiteOrchestrator),
// each site runs as its own system user named from its domain
// (services.SiteUsername), database and database-user names carry the
// subscription id (services.SubscriptionDatabaseName/SubscriptionUserName),
// backups live under the subscription id (cmd/agent/backup_store.go), file
// access is the site's own system user (file_access_handlers.go), mail and
// DNS follow the domain, and VPN peers and add-on grants hold
// subscription_id. No Agent request carries the owning panel account. So a
// transfer changes Panel records only, in one transaction: the owner, the
// old owner's additional-user grants on it, and (only when a reseller's own
// plan cannot follow) the rights state, plus the durable transfer row with
// the before-image. Access is decided per request from owner_id, so the old
// owner and every "Log in as" session of the old owner lose the
// subscription with the commit.
//
// Credentials (owner decision, 2 November 2026): unless the approver unticks
// it, the same transaction also writes a rotation operation and marks the
// subscription's VPN peers revoked; after the commit the worker changes every
// password and key the old owner knew (transfer_rotation.go).
//
// Devir yalnız panel kayıtlarını değiştirir: sunucudaki her şey abonelik ve
// alan adı kimliğine bağlıdır; bu kimlikler devirde değişmez.

const (
	errCodeTransferNotAllowed      = "TRANSFER_NOT_ALLOWED"
	errCodeTransferRequestOnly     = "TRANSFER_REQUEST_ONLY"
	errCodeTransferNeedsAdmin      = "TRANSFER_NEEDS_ADMINISTRATOR"
	errCodeTransferReceiverInvalid = "TRANSFER_RECEIVER_INVALID"
	errCodeTransferSameOwner       = "TRANSFER_SAME_OWNER"
	errCodeTransferAdminWorkspace  = "TRANSFER_ADMIN_WORKSPACE"
	errCodeTransferRightsOutside   = "TRANSFER_RIGHTS_OUTSIDE_RECEIVER"
	errCodeTransferPoolExceeded    = "TRANSFER_POOL_EXCEEDED"
	errCodeTransferConfirm         = "TRANSFER_CONFIRM"
	errCodeTransferStale           = "TRANSFER_STALE"
	errCodeTransferRequestIDReused = "TRANSFER_REQUEST_ID_REUSED"
	errCodeTransferRequestID       = "TRANSFER_REQUEST_ID_INVALID"
	errCodeTransferRequestOpen     = "TRANSFER_REQUEST_OPEN"
	errCodeTransferRequestClosed   = "TRANSFER_REQUEST_CLOSED"

	transferRequested = "requested"
	transferCompleted = "completed"
	transferRejected  = "rejected"
	transferCancelled = "cancelled"
)

var transferRequestIDPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{8,80}$`)

func validTransferRequestID(id string) bool { return transferRequestIDPattern.MatchString(id) }

// transferParty is one account in a transfer.
type transferParty struct {
	ID          int    `json:"id"`
	Username    string `json:"username"`
	Role        string `json:"role"`
	Status      string `json:"-"`
	AccountType string `json:"-"`
	ParentID    int    `json:"-"`
}

func loadTransferParty(ctx context.Context, q rowQueryer, id int) (transferParty, error) {
	var party transferParty
	var parent sql.NullInt64
	var accountType, status sql.NullString
	err := q.QueryRowContext(ctx, `SELECT id, username, role, account_type, status, parent_id FROM users WHERE id = ?`, id).
		Scan(&party.ID, &party.Username, &party.Role, &accountType, &status, &parent)
	if errors.Is(err, sql.ErrNoRows) {
		return party, errNotFound
	}
	if err != nil {
		return party, err
	}
	party.AccountType = accountType.String
	if party.AccountType == "" {
		party.AccountType = "account"
	}
	party.Status = status.String
	if party.Status == "" {
		party.Status = "active"
	}
	party.ParentID = int(parent.Int64)
	return party, nil
}

// transferRefusal is a refusal with what to do about it (D-024).
type transferRefusal struct {
	Status  int      `json:"-"`
	Code    string   `json:"code"`
	Message string   `json:"message"`
	Details []string `json:"details,omitempty"`
	WaysOut []string `json:"ways_out,omitempty"`
}

func (r *transferRefusal) Error() string { return r.Code + ": " + r.Message }

func refuseTransfer(status int, code, message string) *transferRefusal {
	return &transferRefusal{Status: status, Code: code, Message: message}
}

// transferPoolLine is one quantity of the receiving reseller's pool.
type transferPoolLine struct {
	Kind  string `json:"kind"`
	Used  int64  `json:"used"`
	Adds  int64  `json:"adds"`
	Limit int64  `json:"limit"`
	Fits  bool   `json:"fits"`
}

// transferGrantLine: one additional user of the old owner who loses access.
type transferGrantLine struct {
	UserID             int    `json:"user_id"`
	Username           string `json:"username"`
	SubscriptionGrants int    `json:"subscription_grants"`
	DomainGrants       int    `json:"domain_grants"`
}

// transferContents counts what moves with the subscription.
type transferContents struct {
	Domains          int `json:"domains"`
	Sites            int `json:"sites"`
	Mailboxes        int `json:"mailboxes"`
	Databases        int `json:"databases"`
	DatabaseUsers    int `json:"database_users"`
	Certificates     int `json:"certificates"`
	VPNDevices       int `json:"vpn_devices"`
	BackupSchedules  int `json:"backup_schedules"`
	AddOns           int `json:"add_ons"`
	DiskUsedBytes    int `json:"disk_used_bytes"`
	PendingVPNIssues int `json:"pending_vpn_devices"`
}

// transferPreview is what a transfer would do. It is also the stored result.
type transferPreview struct {
	SubscriptionID   int            `json:"subscription_id"`
	SubscriptionName string         `json:"subscription_name"`
	From             transferParty  `json:"from"`
	To               transferParty  `json:"to"`
	FromReseller     *transferParty `json:"from_reseller"`
	ToReseller       *transferParty `json:"to_reseller"`
	SameTree         bool           `json:"same_tree"`
	// Rights state before and after: "unlimited", "plan:<id>" or "custom".
	RightsFrom         string `json:"rights_from"`
	RightsFromPlanName string `json:"rights_from_plan_name,omitempty"`
	RightsTo           string `json:"rights_to"`
	RightsToPlanName   string `json:"rights_to_plan_name,omitempty"`
	// PlanConverted: a reseller's own plan cannot go to another tree; the
	// subscription becomes Custom with the same items and numbers.
	PlanConverted bool `json:"plan_converted"`
	// Capped: open now, closed by the receiving reseller's rights (an
	// "everything offered" or Unlimited set stays so, within that reseller).
	Capped      []string    `json:"capped"`
	CappedInUse []inUseLine `json:"capped_in_use"`
	// Widened: closed now only by the old reseller; open after.
	Widened []string `json:"widened"`
	// Missing: items the subscription's own set names that the receiving
	// reseller does not have. Any missing item refuses the transfer.
	Missing       []string            `json:"missing"`
	Pool          []transferPoolLine  `json:"pool"`
	PoolExceeded  []string            `json:"pool_exceeded"`
	GrantsRemoved []transferGrantLine `json:"grants_removed"`
	Moves         transferContents    `json:"moves"`
	// The old owner keeps no subscription after the transfer.
	FromLeftWithout       bool   `json:"from_left_without_subscription"`
	ReceiverSubscriptions int    `json:"receiver_subscriptions_after"`
	HostChanges           string `json:"host_changes"`
	InstalledUnknown      bool   `json:"installed_unknown,omitempty"`
	Message               string `json:"message"`
	// RotateCredentials: every password and key the old owner knew is
	// changed after the owner change (default; the approver may untick it).
	RotateCredentials bool                    `json:"rotate_credentials"`
	Rotation          transferRotationPreview `json:"rotation"`

	planned *plannedTransfer
}

type plannedTransfer struct {
	fromPlanID int
	toState    string
	set        rightsSet
	limits     planLimits
	toReseller int
	pool       *resellerPool
	adds       transferUse
}

// transferUse is the real use the subscription brings into a pool.
type transferUse struct {
	Domains, Databases, Mailboxes, DiskBytes int64
}

func subscriptionRealUse(ctx context.Context, q rowQueryer, subID int) (transferUse, error) {
	var u transferUse
	err := q.QueryRowContext(ctx, `
		SELECT
		  (SELECT COUNT(*) FROM domains d WHERE d.subscription_id = ?),
		  (SELECT COUNT(*) FROM databases_v2 db WHERE db.subscription_id = ?),
		  (SELECT COUNT(*) FROM email_accounts ea JOIN domains d ON ea.domain_id = d.id WHERE d.subscription_id = ?),
		  COALESCE((SELECT SUM(si.disk_usage_bytes) FROM sites si JOIN domains d ON si.domain_id = d.id WHERE d.subscription_id = ?), 0)`,
		subID, subID, subID, subID).Scan(&u.Domains, &u.Databases, &u.Mailboxes, &u.DiskBytes)
	return u, err
}

// transferLayers loads the rights layers of a subscription through q.
func (p *Panel) transferLayers(ctx context.Context, q rowQueryer, subID int) (*effectiveRights, error) {
	own, err := subscriptionRightsState(ctx, q, subID)
	if err != nil {
		return nil, err
	}
	e := &effectiveRights{SubscriptionID: subID, Own: own, grants: map[string]bool{}}
	if own.AdminWorkspace {
		e.Unrestricted = true
		return e, nil
	}
	e.OwnerID, e.ResellerID, err = subscriptionOwnerChain(ctx, q, subID)
	if err != nil {
		return nil, err
	}
	if e.ResellerID != 0 {
		state, err := resellerRightsState(ctx, q, e.ResellerID)
		if err != nil {
			return nil, err
		}
		e.reseller = &state
	}
	if e.admin, err = p.loadAdminOfferLayer(ctx); err != nil {
		return nil, err
	}
	for permission, product := range legacyGrantPermissions {
		e.grants[permission] = p.hasEntitlement(ctx, subID, product)
	}
	return e, nil
}

// transferAuthority applies who may transfer what (section 8): the
// administrator anyone to anyone; a reseller only between its own
// customers; a customer only asks. owner is the current owner.
func transferAuthority(c *Caller, owner, receiver transferParty) *transferRefusal {
	switch {
	case c.hasAccountRole(roleAdmin):
		return nil
	case c.hasAccountRole(roleReseller):
		if owner.Role != roleCustomer || owner.ParentID != c.ID ||
			receiver.Role != roleCustomer || receiver.ParentID != c.ID || receiver.AccountType != "account" {
			r := refuseTransfer(409, errCodeTransferNeedsAdmin,
				"Nothing was changed: you can move a subscription only between your own customers. A transfer to anyone else is done by the administrator.")
			r.WaysOut = []string{"choose one of your own customers", "ask the administrator to make this transfer"}
			return r
		}
		return nil
	case c.hasAccountRole(roleCustomer):
		r := refuseTransfer(403, errCodeTransferRequestOnly,
			"Nothing was changed: a customer cannot move a subscription alone. Send a transfer request; your provider or the administrator approves it.")
		r.WaysOut = []string{"send a transfer request"}
		return r
	}
	return refuseTransfer(403, errCodeTransferNotAllowed, "only the administrator or a reseller transfers a subscription")
}

// validateReceiver: an active full account that can own a subscription.
func validateReceiver(owner, receiver transferParty) *transferRefusal {
	switch {
	case receiver.ID == owner.ID:
		return refuseTransfer(409, errCodeTransferSameOwner, "Nothing was changed: this account already owns the subscription. Choose another receiver.")
	case receiver.AccountType != "account":
		return refuseTransfer(409, errCodeTransferReceiverInvalid,
			"Nothing was changed: an additional user cannot own a subscription. Choose the customer account itself.")
	case receiver.Role != roleCustomer && receiver.Role != roleReseller:
		return refuseTransfer(409, errCodeTransferReceiverInvalid,
			"Nothing was changed: only a customer or a reseller account can receive a subscription.")
	case receiver.Status != "active":
		r := refuseTransfer(409, errCodeTransferReceiverInvalid,
			"Nothing was changed: the receiving account is suspended. Reactivate it first, then transfer again.")
		r.WaysOut = []string{"reactivate the receiving account"}
		return r
	}
	return nil
}

// planTransfer computes the transfer of subID to toID by caller c, reading
// through q (the database for a preview, the write transaction when it is
// applied). candidates are the installed offering ids (read before any
// transaction). forPreview adds the in-use counts of capped items.
func (p *Panel) planTransfer(ctx context.Context, q rowQueryer, c *Caller, subID, toID int, candidates []string, forPreview bool) (*transferPreview, *transferRefusal, error) {
	var ownerID int
	var subName string
	err := q.QueryRowContext(ctx, `SELECT owner_id, name FROM subscriptions WHERE id = ?`, subID).Scan(&ownerID, &subName)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, refuseTransfer(404, errCodeTransferNotAllowed, "subscription not found"), nil
	}
	if err != nil {
		return nil, nil, err
	}
	owner, err := loadTransferParty(ctx, q, ownerID)
	if err != nil {
		return nil, nil, err
	}
	receiver, err := loadTransferParty(ctx, q, toID)
	if errors.Is(err, errNotFound) {
		if c.hasAccountRole(roleAdmin) {
			return nil, refuseTransfer(404, errCodeTransferReceiverInvalid, "the receiving account was not found"), nil
		}
		receiver = transferParty{ID: toID}
	} else if err != nil {
		return nil, nil, err
	}
	if refusal := transferAuthority(c, owner, receiver); refusal != nil {
		return nil, refusal, nil
	}
	if refusal := validateReceiver(owner, receiver); refusal != nil {
		return nil, refusal, nil
	}
	current, err := p.transferLayers(ctx, q, subID)
	if err != nil {
		return nil, nil, err
	}
	if current.Own.AdminWorkspace {
		return nil, refuseTransfer(409, errCodeTransferAdminWorkspace,
			"Nothing was changed: the administrator's own workspace stays with the administrator. Move its domains to another subscription instead."), nil
	}
	toReseller, err := treeResellerID(ctx, q, receiver.ID)
	if err != nil {
		return nil, nil, err
	}
	preview := &transferPreview{
		SubscriptionID: subID, SubscriptionName: subName, From: owner, To: receiver,
		SameTree:   toReseller == current.ResellerID,
		RightsFrom: current.Own.stateLabel(), RightsFromPlanName: current.Own.PlanName,
		Capped: []string{}, CappedInUse: []inUseLine{}, Widened: []string{}, Missing: []string{},
		Pool: []transferPoolLine{}, PoolExceeded: []string{}, GrantsRemoved: []transferGrantLine{},
		HostChanges: "none",
	}
	if preview.FromReseller, err = optionalParty(ctx, q, current.ResellerID); err != nil {
		return nil, nil, err
	}
	if preview.ToReseller, err = optionalParty(ctx, q, toReseller); err != nil {
		return nil, nil, err
	}
	planned := &plannedTransfer{toState: current.Own.State, set: current.Own.Set, toReseller: toReseller}
	preview.RightsTo, preview.RightsToPlanName = preview.RightsFrom, preview.RightsFromPlanName

	// A reseller's own plan stays only inside that reseller's tree.
	if current.Own.State == rightsStatePlan {
		planned.fromPlanID = current.Own.PlanID
		plan, err := loadPlan(ctx, q, current.Own.PlanID)
		if err != nil {
			return nil, nil, err
		}
		if plan.OwnerID != 0 && !(receiver.Role == roleCustomer && receiver.ParentID == plan.OwnerID) {
			planned.toState = rightsStateCustom
			planned.set = current.Own.Set.clone()
			if planned.limits, err = loadSubscriptionLimits(ctx, q, subID); err != nil {
				return nil, nil, err
			}
			preview.PlanConverted = true
			preview.RightsTo, preview.RightsToPlanName = rightsStateCustom, ""
		}
	}

	// The receiving reseller's ceiling, only when the tree changes.
	after := *current
	after.Own.Set = planned.set
	after.reseller = nil
	if toReseller != 0 {
		state, err := resellerRightsState(ctx, q, toReseller)
		if err != nil {
			return nil, nil, err
		}
		after.reseller = &state
		if !preview.SameTree {
			for _, item := range planned.set.itemList() {
				if id, ok := strings.CutPrefix(item, permissionItemHead); ok {
					if !state.Set.permits(id) {
						preview.Missing = append(preview.Missing, item)
					}
					continue
				}
				if !state.Set.offers(item) {
					preview.Missing = append(preview.Missing, item)
				}
			}
			if err := transferPoolCheck(ctx, q, subID, toReseller, preview, planned); err != nil {
				return nil, nil, err
			}
		}
	}
	diff := diffEffective(current, &after, candidates)
	preview.Capped, preview.Widened = diff.Removed, diff.Added
	if forPreview && len(preview.Capped) > 0 {
		if preview.CappedInUse, err = p.removedItemsInUse(ctx, []int{subID}, preview.Capped); err != nil {
			return nil, nil, err
		}
	}
	if preview.GrantsRemoved, err = transferGrants(ctx, q, subID); err != nil {
		return nil, nil, err
	}
	if preview.Moves, err = transferMoves(ctx, q, subID); err != nil {
		return nil, nil, err
	}
	var fromCount, toCount int
	if err := q.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM subscriptions WHERE owner_id = ?),
		(SELECT COUNT(*) FROM subscriptions WHERE owner_id = ?)`, owner.ID, receiver.ID).Scan(&fromCount, &toCount); err != nil {
		return nil, nil, err
	}
	preview.FromLeftWithout = fromCount <= 1
	preview.ReceiverSubscriptions = toCount + 1
	if _, preview.Rotation, err = planTransferRotation(ctx, q, subID); err != nil {
		return nil, nil, err
	}
	preview.planned = planned
	preview.setRotation(true)

	if len(preview.Missing) > 0 {
		r := refuseTransfer(409, errCodeTransferRightsOutside,
			"Nothing was changed: the subscription has rights that "+preview.ToReseller.Username+" does not have, so it cannot move into that reseller's accounts.")
		r.Details = append([]string{}, preview.Missing...)
		r.WaysOut = []string{
			"the administrator adds these items to " + preview.ToReseller.Username + "'s rights",
			"narrow the subscription first (possible only if what it uses fits the narrower rights)",
			"keep the subscription where it is",
		}
		return preview, r, nil
	}
	if len(preview.PoolExceeded) > 0 {
		r := refuseTransfer(409, errCodeTransferPoolExceeded,
			"Nothing was changed: what the subscription really uses does not fit the free room in "+preview.ToReseller.Username+"'s pool.")
		r.Details = append([]string{}, preview.PoolExceeded...)
		r.WaysOut = []string{
			"the administrator gives " + preview.ToReseller.Username + " a larger pool",
			"remove what the subscription no longer needs, then try again",
			"keep the subscription where it is",
		}
		return preview, r, nil
	}
	return preview, nil, nil
}

func optionalParty(ctx context.Context, q rowQueryer, id int) (*transferParty, error) {
	if id == 0 {
		return nil, nil
	}
	party, err := loadTransferParty(ctx, q, id)
	if err != nil {
		return nil, err
	}
	return &party, nil
}

// transferPoolCheck: the receiving reseller's pool must hold the tree's
// real use plus what the subscription really uses (K2).
func transferPoolCheck(ctx context.Context, q rowQueryer, subID, resellerID int, preview *transferPreview, planned *plannedTransfer) error {
	pool, ok, err := loadResellerPool(ctx, q, resellerID)
	if err != nil || !ok {
		return err
	}
	planned.pool = &pool
	usage, err := resellerTreeUsage(ctx, q, resellerID)
	if err != nil {
		return err
	}
	adds, err := subscriptionRealUse(ctx, q, subID)
	if err != nil {
		return err
	}
	planned.adds = adds
	const mb = 1024 * 1024
	check := func(kind string, limit int, used, add int64, unit int64) {
		if limit <= 0 {
			return
		}
		fits := used+add <= int64(limit)*unit
		line := transferPoolLine{Kind: kind, Used: used / unit, Adds: add / unit, Limit: int64(limit), Fits: fits}
		if unit == mb && add > 0 && line.Adds == 0 {
			line.Adds = 1
		}
		preview.Pool = append(preview.Pool, line)
		if !fits {
			suffix := ""
			if unit == mb {
				suffix = " MB"
			}
			preview.PoolExceeded = append(preview.PoolExceeded, fmt.Sprintf("%s: %d%s in use + %d%s brought = more than the pool of %d%s",
				kind, line.Used, suffix, line.Adds, suffix, line.Limit, suffix))
		}
	}
	check("domains", pool.Domains, usage.Domains, adds.Domains, 1)
	check("databases", pool.Databases, usage.Databases, adds.Databases, 1)
	check("mailboxes", pool.Mailboxes, usage.Mailboxes, adds.Mailboxes, 1)
	check("disk", pool.DiskMB, usage.DiskUsedBytes, adds.DiskBytes, mb)
	return nil
}

// transferGrants lists the additional users whose grants on the
// subscription (or its domains) the transfer removes.
func transferGrants(ctx context.Context, q rowQueryer, subID int) ([]transferGrantLine, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT u.id, u.username,
		  (SELECT COUNT(*) FROM additional_user_subscription_permissions sp WHERE sp.user_id = u.id AND sp.subscription_id = ?),
		  (SELECT COUNT(*) FROM additional_user_domain_permissions dp JOIN domains d ON d.id = dp.domain_id
		     WHERE dp.user_id = u.id AND d.subscription_id = ?)
		FROM users u
		WHERE u.id IN (SELECT user_id FROM additional_user_subscription_permissions WHERE subscription_id = ?
		               UNION SELECT dp.user_id FROM additional_user_domain_permissions dp JOIN domains d ON d.id = dp.domain_id
		                     WHERE d.subscription_id = ?)
		ORDER BY u.username`, subID, subID, subID, subID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []transferGrantLine{}
	for rows.Next() {
		var line transferGrantLine
		if err := rows.Scan(&line.UserID, &line.Username, &line.SubscriptionGrants, &line.DomainGrants); err != nil {
			return nil, err
		}
		out = append(out, line)
	}
	return out, rows.Err()
}

func transferMoves(ctx context.Context, q rowQueryer, subID int) (transferContents, error) {
	var m transferContents
	err := q.QueryRowContext(ctx, `
		SELECT
		  (SELECT COUNT(*) FROM domains WHERE subscription_id = ?1),
		  (SELECT COUNT(*) FROM sites si JOIN domains d ON d.id = si.domain_id
		     WHERE d.subscription_id = ?1 AND COALESCE(si.project_type, 'php') <> 'dnsonly'),
		  (SELECT COUNT(*) FROM email_accounts ea JOIN domains d ON d.id = ea.domain_id WHERE d.subscription_id = ?1),
		  (SELECT COUNT(*) FROM databases_v2 WHERE subscription_id = ?1),
		  (SELECT COUNT(*) FROM database_users WHERE subscription_id = ?1),
		  (SELECT COUNT(*) FROM ssl_certificates c JOIN domains d ON d.id = c.domain_id
		     WHERE d.subscription_id = ?1 AND COALESCE(c.status, 'active') = 'active'),
		  (SELECT COUNT(*) FROM vpn_peers WHERE subscription_id = ?1 AND desired_state = 'active'),
		  (SELECT COUNT(*) FROM vpn_peers WHERE subscription_id = ?1 AND desired_state = 'active' AND provisioning_state = 'provisioning'),
		  (SELECT COUNT(*) FROM backup_schedules b JOIN domains d ON d.id = b.domain_id WHERE d.subscription_id = ?1),
		  (SELECT COUNT(*) FROM subscription_entitlements WHERE subscription_id = ?1 AND status = 'active'),
		  COALESCE((SELECT SUM(si.disk_usage_bytes) FROM sites si JOIN domains d ON d.id = si.domain_id WHERE d.subscription_id = ?1), 0)`,
		subID).Scan(&m.Domains, &m.Sites, &m.Mailboxes, &m.Databases, &m.DatabaseUsers, &m.Certificates,
		&m.VPNDevices, &m.PendingVPNIssues, &m.BackupSchedules, &m.AddOns, &m.DiskUsedBytes)
	return m, err
}

// setRotation records the approver's choice and what it changes.
func (p *transferPreview) setRotation(rotate bool) {
	p.RotateCredentials = rotate
	p.HostChanges = "none"
	if rotate && p.Rotation.Total > 0 {
		p.HostChanges = "credentials"
	}
	p.Message = transferMessage(p)
}

func transferMessage(p *transferPreview) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("The subscription \"%s\" moves from %s to %s with everything in it. Sites, mail, databases, certificates and scheduled tasks keep running as they are.",
		p.SubscriptionName, p.From.Username, p.To.Username))
	if p.PlanConverted {
		b.WriteString(fmt.Sprintf(" The plan \"%s\" belongs to another reseller, so the subscription becomes Custom with the same items and numbers.", p.RightsFromPlanName))
	}
	if len(p.Capped) > 0 {
		b.WriteString(" Some items close because the receiving reseller does not have them; what runs keeps running, only new use is refused.")
	}
	if len(p.GrantsRemoved) > 0 {
		b.WriteString(fmt.Sprintf(" %d additional user(s) of %s lose their access to it.", len(p.GrantsRemoved), p.From.Username))
	}
	switch {
	case p.RotateCredentials && p.Rotation.Total > 0:
		b.WriteString(fmt.Sprintf(" Right after the transfer every password and key the previous owner knew is changed (%d in all): mailboxes, database users, file access, VPN devices and protected folders. The new owner then sets new ones in the panel; mail programs and other apps that sign in need the new password.", p.Rotation.Total))
	case p.RotateCredentials:
		b.WriteString(" The subscription holds no password or key to change.")
	default:
		b.WriteString(" Warning: passwords, keys and VPN devices are NOT changed. The previous owner can keep using them until the new owner changes each one.")
	}
	b.WriteString(" Confirm to continue.")
	return b.String()
}

// transferBeforeImage is the durable state before the change.
type transferBeforeImage struct {
	OwnerID       int                  `json:"owner_id"`
	OwnerUsername string               `json:"owner_username"`
	PlanID        int                  `json:"plan_id,omitempty"`
	RightsState   string               `json:"rights_state"`
	Rights        rightsPayload        `json:"rights"`
	Limits        planLimits           `json:"limits"`
	Grants        []transferStoredRule `json:"grants"`
}

type transferStoredRule struct {
	Scope      string `json:"scope"`
	UserID     int    `json:"user_id"`
	ScopeID    int    `json:"scope_id"`
	Capability string `json:"capability"`
	Mode       string `json:"mode"`
}

func readTransferBeforeImage(ctx context.Context, q rowQueryer, subID int, preview *transferPreview) (transferBeforeImage, error) {
	img := transferBeforeImage{OwnerID: preview.From.ID, OwnerUsername: preview.From.Username,
		PlanID: preview.planned.fromPlanID, RightsState: preview.RightsFrom, Grants: []transferStoredRule{}}
	own, err := subscriptionRightsState(ctx, q, subID)
	if err != nil {
		return img, err
	}
	img.Rights = own.Set.payload()
	if img.Limits, err = loadSubscriptionLimits(ctx, q, subID); err != nil {
		return img, err
	}
	rows, err := q.QueryContext(ctx, `
		SELECT 'subscription', user_id, subscription_id, capability, mode FROM additional_user_subscription_permissions WHERE subscription_id = ?
		UNION ALL
		SELECT 'domain', dp.user_id, dp.domain_id, dp.capability, dp.mode FROM additional_user_domain_permissions dp
		JOIN domains d ON d.id = dp.domain_id WHERE d.subscription_id = ?
		ORDER BY 1, 2, 3, 4`, subID, subID)
	if err != nil {
		return img, err
	}
	defer rows.Close()
	for rows.Next() {
		var rule transferStoredRule
		if err := rows.Scan(&rule.Scope, &rule.UserID, &rule.ScopeID, &rule.Capability, &rule.Mode); err != nil {
			return img, err
		}
		img.Grants = append(img.Grants, rule)
	}
	return img, rows.Err()
}

// applyTransfer changes the records inside tx. The grants go first so the
// 029 owner guards see no conflicting grant; the owner update is a
// compare-and-set on the owner the plan was made for.
func applyTransfer(ctx context.Context, tx *sql.Tx, preview *transferPreview) (transferBeforeImage, error) {
	subID := preview.SubscriptionID
	img, err := readTransferBeforeImage(ctx, tx, subID, preview)
	if err != nil {
		return img, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM additional_user_domain_permissions
		WHERE domain_id IN (SELECT id FROM domains WHERE subscription_id = ?)`, subID); err != nil {
		return img, fmt.Errorf("remove additional-user domain grants: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM additional_user_subscription_permissions WHERE subscription_id = ?`, subID); err != nil {
		return img, fmt.Errorf("remove additional-user subscription grants: %w", err)
	}
	if preview.PlanConverted {
		if err := setSubscriptionCustom(ctx, tx, subID, preview.planned.set, preview.planned.limits); err != nil {
			return img, fmt.Errorf("keep the plan's items as Custom: %w", err)
		}
	}
	result, err := tx.ExecContext(ctx, `UPDATE subscriptions SET owner_id = ?, updated_at = datetime('now') WHERE id = ? AND owner_id = ?`,
		preview.To.ID, subID, preview.From.ID)
	if err != nil {
		return img, fmt.Errorf("change the owner: %w", err)
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		return img, errTransferStale
	}
	return img, nil
}

var errTransferStale = errors.New("the subscription changed while the transfer was prepared")

// transferWriteLockStatement takes SQLite's write lock before any read
// (the reason is in dns_write_tx.go).
const transferWriteLockStatement = `UPDATE subscriptions SET id = id WHERE 0`

func beginTransferTx(ctx context.Context, db *sql.DB) (*sql.Tx, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, transferWriteLockStatement); err != nil {
		_ = tx.Rollback()
		return nil, fmt.Errorf("wait for the panel database's write lock: %w", err)
	}
	return tx, nil
}

// transferRow is one stored transfer or request.
type transferRow struct {
	ID                 int             `json:"id"`
	RequestID          string          `json:"-"`
	SubscriptionID     int             `json:"subscription_id"`
	SubscriptionName   string          `json:"subscription_name"`
	Status             string          `json:"status"`
	From               transferParty   `json:"from"`
	To                 *transferParty  `json:"to"`
	RequestedTo        string          `json:"requested_to"`
	Note               string          `json:"note"`
	Approver           string          `json:"approver"`
	ApproverResellerID int             `json:"approver_reseller_id,omitempty"`
	RequestedBy        int             `json:"requested_by"`
	DecisionRequestID  string          `json:"-"`
	DecisionReason     string          `json:"decision_reason"`
	Result             json.RawMessage `json:"result,omitempty"`
	CreatedAt          string          `json:"created_at"`
	DecidedAt          string          `json:"decided_at,omitempty"`
	ReceiverSeen       bool            `json:"receiver_seen"`
	RequesterSeen      bool            `json:"requester_seen"`
	// IsRequest: made by a customer's request (not a direct transfer).
	IsRequest bool `json:"is_request"`
	// For the caller of a list: the row is about it, or it may decide it.
	ToYou     bool `json:"to_you,omitempty"`
	FromYou   bool `json:"from_you,omitempty"`
	CanDecide bool `json:"can_decide,omitempty"`
	// Rotation: the change of passwords and keys after a completed transfer.
	Rotation *transferRotationView `json:"rotation,omitempty"`
}

const transferRowColumns = `t.id, t.request_id, t.subscription_id, t.subscription_name, t.status,
	t.from_owner_id, COALESCE(fu.username, ''), COALESCE(t.to_owner_id, 0), COALESCE(tu.username, ''),
	t.requested_to, t.note, COALESCE(t.approver_reseller_id, 0), t.requested_by,
	COALESCE(t.decision_request_id, ''), t.decision_reason, COALESCE(t.result, ''), t.created_at, COALESCE(t.decided_at, ''),
	t.receiver_seen_at IS NOT NULL, t.requester_seen_at IS NOT NULL`

const transferRowFrom = ` FROM subscription_transfers t
	LEFT JOIN users fu ON fu.id = t.from_owner_id
	LEFT JOIN users tu ON tu.id = t.to_owner_id`

type rowScanner interface{ Scan(dest ...any) error }

func scanTransferRow(s rowScanner) (transferRow, error) {
	var row transferRow
	var toID int
	var toName, result string
	if err := s.Scan(&row.ID, &row.RequestID, &row.SubscriptionID, &row.SubscriptionName, &row.Status,
		&row.From.ID, &row.From.Username, &toID, &toName, &row.RequestedTo, &row.Note, &row.ApproverResellerID,
		&row.RequestedBy, &row.DecisionRequestID, &row.DecisionReason, &result, &row.CreatedAt, &row.DecidedAt,
		&row.ReceiverSeen, &row.RequesterSeen); err != nil {
		return row, err
	}
	if toID > 0 {
		row.To = &transferParty{ID: toID, Username: toName}
	}
	row.Approver = "administrator"
	if row.ApproverResellerID > 0 {
		row.Approver = "reseller"
	}
	if result != "" {
		row.Result = json.RawMessage(result)
	}
	row.IsRequest = row.Status != transferCompleted || row.DecisionRequestID != ""
	return row, nil
}

func loadTransferRow(ctx context.Context, q rowQueryer, where string, args ...any) (transferRow, error) {
	row, err := scanTransferRow(q.QueryRowContext(ctx, `SELECT `+transferRowColumns+transferRowFrom+` WHERE `+where, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return row, errNotFound
	}
	return row, err
}

// transferAuditDetails is what the audit row says about a transfer.
func transferAuditDetails(preview *transferPreview, transferID, requestRowID int) map[string]any {
	details := map[string]any{
		"transfer_id": transferID, "from_user_id": preview.From.ID, "from": preview.From.Username,
		"to_user_id": preview.To.ID, "to": preview.To.Username,
		"rights_from": preview.RightsFrom, "rights_to": preview.RightsTo,
		"plan_converted": preview.PlanConverted, "capped": preview.Capped,
		"grants_removed": len(preview.GrantsRemoved), "host_changes": preview.HostChanges,
		"rotate_credentials": preview.RotateCredentials,
	}
	if preview.RotateCredentials {
		details["rotation"] = map[string]int{
			"site_accounts": len(preview.Rotation.SiteAccounts), "vpn_devices": len(preview.Rotation.VPNDevices),
			"mailboxes": len(preview.Rotation.Mailboxes), "database_users": len(preview.Rotation.DatabaseUsers),
			"protected_sites": len(preview.Rotation.ProtectedUsers),
		}
	}
	if preview.FromReseller != nil {
		details["from_reseller"] = preview.FromReseller.Username
	}
	if preview.ToReseller != nil {
		details["to_reseller"] = preview.ToReseller.Username
	}
	if requestRowID > 0 {
		details["transfer_request"] = requestRowID
	}
	return details
}
