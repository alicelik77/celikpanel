package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// Security emergency (alpha.117, D-043). When a component or a version of it
// has a security hole and no patch, the administrator has four separate
// steps, lightest first:
//
//  1. withdraw: nobody can choose it for anything new; what runs keeps running;
//  2. stop: its units are stopped and masked, the affected sites show the
//     maintenance page;
//  3. move: the administrator moves the affected sites to another version,
//     one by one, never automatically;
//  4. remove: the package goes even though sites use it; no customer data is
//     deleted (D-038 point 8), the sites stay on the maintenance page.
//
// Re-enable puts back what the stop changed. Steps 2-4 and re-enable are
// operations with checkpoints (security_emergency_run.go), like suspension.
//
// Güvenlik acil durumu: geri çek, durdur, taşı, kaldır; veri asla silinmez.

const (
	emergencyStageWithdrawn = "withdrawn"
	emergencyStageStopped   = "stopped"
	emergencyStageRemoved   = "removed"
	emergencyStageResolved  = "resolved"

	emergencyOpStop     = "stop"
	emergencyOpMove     = "move"
	emergencyOpRemove   = "remove"
	emergencyOpReenable = "reenable"

	emergencyReasonMaxLen = 500
)

var (
	errEmergencyExists      = errors.New("an emergency for this component is already open")
	errEmergencyNotFound    = errors.New("security emergency not found")
	errEmergencyBusy        = errors.New("another step of this emergency is running")
	errEmergencyWrongStage  = errors.New("this step does not apply to the emergency now")
	errEmergencyBadTarget   = errors.New("unknown component or version")
	errEmergencyBadMoveGoal = errors.New("the target version must be another version of the same component")
)

// emergencyComponent describes one kind of target.
type emergencyComponent struct {
	Name      string // the name a sentence uses
	ServiceID string // catalogue id
	Versioned bool
}

var emergencyComponents = map[string]emergencyComponent{
	"php":        {Name: "PHP", ServiceID: "php-fpm", Versioned: true},
	"node":       {Name: "Node.js", ServiceID: "node", Versioned: true},
	"mariadb":    {Name: "MariaDB", ServiceID: "mariadb"},
	"postgresql": {Name: "PostgreSQL", ServiceID: "postgresql"},
}

var emergencyPHPVersion = regexp.MustCompile(`^[0-9]{1,2}\.[0-9]{1,2}$`)

// validEmergencyTarget checks a component and its version.
func validEmergencyTarget(component, version string) bool {
	info, ok := emergencyComponents[component]
	if !ok {
		return false
	}
	if !info.Versioned {
		return version == ""
	}
	switch component {
	case "php":
		return emergencyPHPVersion.MatchString(version)
	case "node":
		return nodeSemverRe.MatchString(version)
	}
	return false
}

// emergencyTargetName is what the administrator types to confirm.
func emergencyTargetName(component, version string) string {
	name := emergencyComponents[component].Name
	if version != "" {
		return name + " " + version
	}
	return name
}

type securityEmergency struct {
	ID          string `json:"id"`
	Component   string `json:"component"`
	Version     string `json:"version"`
	Name        string `json:"name"`
	Reason      string `json:"reason"`
	Withdrawn   bool   `json:"withdrawn"`
	Stage       string `json:"stage"`
	Recommended string `json:"recommended,omitempty"`
	CreatedBy   int    `json:"created_by,omitempty"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
	ResolvedAt  string `json:"resolved_at,omitempty"`
}

const emergencyColumns = `id, component, version, reason, withdrawn, stage, COALESCE(recommended, ''),
	COALESCE(created_by, 0), created_at, updated_at, COALESCE(resolved_at, '')`

func scanEmergency(row interface{ Scan(...any) error }) (securityEmergency, error) {
	var e securityEmergency
	err := row.Scan(&e.ID, &e.Component, &e.Version, &e.Reason, &e.Withdrawn, &e.Stage, &e.Recommended,
		&e.CreatedBy, &e.CreatedAt, &e.UpdatedAt, &e.ResolvedAt)
	e.Name = emergencyTargetName(e.Component, e.Version)
	return e, err
}

func loadEmergency(ctx context.Context, q sqlQueryer, id string) (securityEmergency, error) {
	e, err := scanEmergency(q.QueryRowContext(ctx, `SELECT `+emergencyColumns+` FROM security_emergencies WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return e, errEmergencyNotFound
	}
	return e, err
}

func listEmergencies(ctx context.Context, q sqlQueryer) ([]securityEmergency, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+emergencyColumns+` FROM security_emergencies
		WHERE stage <> 'resolved' OR resolved_at >= datetime('now', '-30 days')
		ORDER BY CASE WHEN stage = 'resolved' THEN 1 ELSE 0 END, created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []securityEmergency{}
	for rows.Next() {
		e, err := scanEmergency(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// withdrawnVersions is what the open emergencies withdraw from new choice:
// component -> exact version (PHP line or exact Node.js version). A whole
// component withdrawn is version "".
func withdrawnVersions(ctx context.Context, q sqlQueryer) (map[string]map[string]bool, error) {
	rows, err := q.QueryContext(ctx, `SELECT component, version FROM security_emergencies WHERE stage <> 'resolved' AND withdrawn = 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]map[string]bool{}
	for rows.Next() {
		var component, version string
		if err := rows.Scan(&component, &version); err != nil {
			return nil, err
		}
		if out[component] == nil {
			out[component] = map[string]bool{}
		}
		out[component][version] = true
	}
	return out, rows.Err()
}

// versionWithdrawn: whether a version is withdrawn for security.
func versionWithdrawn(withdrawn map[string]map[string]bool, component, version string) bool {
	set := withdrawn[component]
	if set == nil {
		return false
	}
	if set[""] || set[strings.TrimSpace(version)] {
		return true
	}
	if component == "php" {
		return set[offeredVersionLine("php", version)]
	}
	return false
}

// emergencySite is one affected site.
type emergencySite struct {
	DomainID       int    `json:"domain_id"`
	SiteID         int    `json:"site_id"`
	SubscriptionID int    `json:"subscription_id"`
	Domain         string `json:"domain"`
	CustomerID     int    `json:"customer_id"`
	Customer       string `json:"customer"`
	ResellerID     int    `json:"reseller_id,omitempty"`
	Reseller       string `json:"reseller,omitempty"`
	Maintenance    bool   `json:"maintenance"`
	MovedTo        string `json:"moved_to,omitempty"`
	Version        string `json:"version,omitempty"`
}

const emergencySiteSelect = `
	SELECT d.id, s.id, d.subscription_id, d.name, u.id, u.username,
	       COALESCE(CASE WHEN u.role = 'reseller' THEN u.id WHEN pr.role = 'reseller' THEN pr.id END, 0),
	       COALESCE(CASE WHEN u.role = 'reseller' THEN u.username WHEN pr.role = 'reseller' THEN pr.username END, ''),
	       CASE WHEN COALESCE(s.project_type, 'php') = 'node' THEN COALESCE(s.runtime_version, '') ELSE COALESCE(s.php_version, '') END
	FROM sites s
	JOIN domains d ON d.id = s.domain_id
	JOIN subscriptions sub ON sub.id = d.subscription_id
	JOIN users u ON u.id = sub.owner_id
	LEFT JOIN users pr ON pr.id = u.parent_id`

// emergencyAffectedSites reads the sites that use a target now.
func emergencyAffectedSites(ctx context.Context, q sqlQueryer, component, version string) ([]emergencySite, error) {
	var where string
	var args []any
	switch component {
	case "php":
		where = `COALESCE(s.project_type, 'php') = 'php' AND s.php_version = ?`
		args = []any{version}
	case "node":
		where = `s.project_type = 'node' AND s.runtime_version = ?`
		args = []any{version}
	case "mariadb", "postgresql":
		// CelikPanel does not know which site uses which database: every site
		// of a subscription with a database on the engine is affected.
		synonym := map[string]string{"mariadb": "mysql", "postgresql": "postgres"}[component]
		where = `COALESCE(s.project_type, 'php') <> 'dnsonly' AND d.subscription_id IN (
			SELECT db.subscription_id FROM databases_v2 db
			JOIN database_servers ds ON ds.id = db.server_id
			JOIN database_server_types t ON t.id = ds.type_id
			WHERE LOWER(t.name) = ? AND COALESCE(ds.host, '') IN ('', 'localhost', '127.0.0.1', '::1')
			UNION SELECT subscription_id FROM databases WHERE LOWER(db_type) IN (?, ?)
			  AND COALESCE(host, '') IN ('', 'localhost', '127.0.0.1', '::1'))`
		args = []any{component, component, synonym}
	default:
		return nil, errEmergencyBadTarget
	}
	rows, err := q.QueryContext(ctx, emergencySiteSelect+` WHERE `+where+` ORDER BY d.name`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []emergencySite{}
	for rows.Next() {
		var site emergencySite
		if err := rows.Scan(&site.DomainID, &site.SiteID, &site.SubscriptionID, &site.Domain, &site.CustomerID,
			&site.Customer, &site.ResellerID, &site.Reseller, &site.Version); err != nil {
			return nil, err
		}
		site.Maintenance = true
		out = append(out, site)
	}
	return out, rows.Err()
}

// emergencyRecordedSites reads the sites an emergency recorded at its stop.
func emergencyRecordedSites(ctx context.Context, q sqlQueryer, emergencyID string) ([]emergencySite, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT es.domain_id, es.site_id, es.subscription_id, es.domain, COALESCE(u.id, 0), COALESCE(u.username, ''),
		       COALESCE(CASE WHEN u.role = 'reseller' THEN u.id WHEN pr.role = 'reseller' THEN pr.id END, 0),
		       COALESCE(CASE WHEN u.role = 'reseller' THEN u.username WHEN pr.role = 'reseller' THEN pr.username END, ''),
		       es.maintenance, COALESCE(es.moved_to, ''),
		       COALESCE(CASE WHEN COALESCE(s.project_type, 'php') = 'node' THEN s.runtime_version ELSE s.php_version END, '')
		FROM security_emergency_sites es
		LEFT JOIN subscriptions sub ON sub.id = es.subscription_id
		LEFT JOIN users u ON u.id = sub.owner_id
		LEFT JOIN users pr ON pr.id = u.parent_id
		LEFT JOIN sites s ON s.id = es.site_id AND s.domain_id = es.domain_id
		WHERE es.emergency_id = ? ORDER BY es.domain`, emergencyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []emergencySite{}
	for rows.Next() {
		var site emergencySite
		if err := rows.Scan(&site.DomainID, &site.SiteID, &site.SubscriptionID, &site.Domain, &site.CustomerID,
			&site.Customer, &site.ResellerID, &site.Reseller, &site.Maintenance, &site.MovedTo, &site.Version); err != nil {
			return nil, err
		}
		out = append(out, site)
	}
	return out, rows.Err()
}

type emergencyParty struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Sites int    `json:"sites"`
}

// emergencyPreview is everything a step affects (read-only), shown before
// the confirmation.
type emergencyPreview struct {
	Component string           `json:"component"`
	Version   string           `json:"version"`
	Name      string           `json:"name"`
	Sites     []emergencySite  `json:"sites"`
	Customers []emergencyParty `json:"customers"`
	Resellers []emergencyParty `json:"resellers"`
	Offering  []string         `json:"offered_by"`
	Notes     []string         `json:"notes"`
}

func buildEmergencyPreview(ctx context.Context, q sqlQueryer, component, version string, sites []emergencySite) (emergencyPreview, error) {
	preview := emergencyPreview{Component: component, Version: version, Name: emergencyTargetName(component, version),
		Sites: sites, Customers: []emergencyParty{}, Resellers: []emergencyParty{}, Offering: []string{}, Notes: []string{}}
	customers := map[int]*emergencyParty{}
	resellers := map[int]*emergencyParty{}
	for _, site := range sites {
		if site.CustomerID > 0 {
			if customers[site.CustomerID] == nil {
				customers[site.CustomerID] = &emergencyParty{ID: site.CustomerID, Name: site.Customer}
			}
			customers[site.CustomerID].Sites++
		}
		if site.ResellerID > 0 {
			if resellers[site.ResellerID] == nil {
				resellers[site.ResellerID] = &emergencyParty{ID: site.ResellerID, Name: site.Reseller}
			}
			resellers[site.ResellerID].Sites++
		}
	}
	for _, party := range customers {
		preview.Customers = append(preview.Customers, *party)
	}
	for _, party := range resellers {
		preview.Resellers = append(preview.Resellers, *party)
	}
	sort.Slice(preview.Customers, func(i, j int) bool { return preview.Customers[i].Name < preview.Customers[j].Name })
	sort.Slice(preview.Resellers, func(i, j int) bool { return preview.Resellers[i].Name < preview.Resellers[j].Name })
	offering, err := emergencyOfferingSets(ctx, q, component, version)
	if err != nil {
		return preview, err
	}
	preview.Offering = offering
	switch component {
	case "mariadb", "postgresql":
		preview.Notes = append(preview.Notes, "CelikPanel does not know which site uses which database, so every site of a subscription with a database on "+emergencyComponents[component].Name+" is listed.")
	}
	preview.Notes = append(preview.Notes, "Nothing is deleted: files, databases and mailboxes stay. Removing customer data is always a separate, explicitly requested operation.")
	return preview, nil
}

// emergencyOfferingSets names the plans and rights sets that offer the
// target: every set with all software, and every set that names it.
func emergencyOfferingSets(ctx context.Context, q sqlQueryer, component, version string) ([]string, error) {
	items := []string{}
	switch component {
	case "php", "node":
		if id := versionOfferingID(component, version); id != "" {
			items = append(items, id)
		}
		items = append(items, "component:"+emergencyComponents[component].ServiceID)
	default:
		items = append(items, "component:"+component)
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(items)), ",")
	args := []any{}
	for _, item := range items {
		args = append(args, item)
	}
	rows, err := q.QueryContext(ctx, `
		SELECT rs.owner_kind,
		       COALESCE(CASE rs.owner_kind
		           WHEN 'plan' THEN (SELECT name FROM service_plans WHERE id = rs.owner_id)
		           WHEN 'reseller_plan' THEN (SELECT name FROM reseller_plans WHERE id = rs.owner_id)
		           WHEN 'subscription' THEN (SELECT name FROM subscriptions WHERE id = rs.owner_id)
		           WHEN 'reseller' THEN (SELECT username FROM users WHERE id = rs.owner_id) END, '#' || rs.owner_id)
		FROM rights_sets rs
		WHERE rs.all_software = 1
		   OR EXISTS (SELECT 1 FROM rights_set_items i WHERE i.set_id = rs.id AND i.item IN (`+placeholders+`))
		ORDER BY rs.owner_kind, 2`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	labels := map[string]string{"plan": "Plan", "reseller_plan": "Reseller plan", "subscription": "Custom subscription", "reseller": "Custom reseller"}
	out := []string{}
	for rows.Next() {
		var kind, name string
		if err := rows.Scan(&kind, &name); err != nil {
			return nil, err
		}
		out = append(out, labels[kind]+": "+name)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var unlimited int
	if err := q.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM subscriptions sub
		WHERE sub.plan_id IS NULL AND NOT EXISTS (
			SELECT 1 FROM rights_sets rs WHERE rs.owner_kind = 'subscription' AND rs.owner_id = sub.id)`).Scan(&unlimited); err != nil {
		return nil, err
	}
	if unlimited > 0 {
		out = append(out, fmt.Sprintf("Unlimited: %d subscription(s)", unlimited))
	}
	return out, nil
}

// emergencyConfirmation is what steps 2-4 require.
type emergencyConfirmation struct {
	Reason        string `json:"reason"`
	ConfirmName   string `json:"confirm_name"`
	Password      string `json:"password"`
	TargetVersion string `json:"target_version,omitempty"`
	KeepWithdrawn bool   `json:"keep_withdrawn,omitempty"`
}

type emergencyOperationRow struct {
	ID            string
	EmergencyID   string
	Kind          string
	TargetVersion string
	Reason        string
	KeepWithdrawn bool
	ActorID       int
	State         string
	Attempts      int
	ErrorKind     string
	LastError     string
	CreatedAt     string
	FinishedAt    string
}

func loadEmergencyOperation(ctx context.Context, q sqlQueryer, id string) (emergencyOperationRow, error) {
	var op emergencyOperationRow
	err := q.QueryRowContext(ctx, `
		SELECT id, emergency_id, kind, COALESCE(target_version, ''), reason, keep_withdrawn, COALESCE(actor_id, 0),
		       state, attempts, COALESCE(error_kind, ''), COALESCE(last_error, ''), created_at, COALESCE(finished_at, '')
		FROM security_emergency_operations WHERE id = ?`, id).
		Scan(&op.ID, &op.EmergencyID, &op.Kind, &op.TargetVersion, &op.Reason, &op.KeepWithdrawn, &op.ActorID,
			&op.State, &op.Attempts, &op.ErrorKind, &op.LastError, &op.CreatedAt, &op.FinishedAt)
	return op, err
}

func latestEmergencyOperation(ctx context.Context, q sqlQueryer, emergencyID string) (string, error) {
	var id string
	err := q.QueryRowContext(ctx, `SELECT id FROM security_emergency_operations WHERE emergency_id = ?
		ORDER BY CASE WHEN state = 'running' THEN 0 ELSE 1 END, created_at DESC, rowid DESC LIMIT 1`, emergencyID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return id, err
}

func runningEmergencyOperation(ctx context.Context, q sqlQueryer, emergencyID string) (string, string, error) {
	var id, kind string
	err := q.QueryRowContext(ctx, `SELECT id, kind FROM security_emergency_operations
		WHERE emergency_id = ? AND state = 'running'`, emergencyID).Scan(&id, &kind)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", nil
	}
	return id, kind, err
}

// emergencyEffectRef is the identity stored with an effect.
type emergencyEffectRef struct {
	DomainID int    `json:"domain_id,omitempty"`
	SiteID   int    `json:"site_id,omitempty"`
	Domain   string `json:"domain,omitempty"`
	Service  string `json:"service,omitempty"`
	Version  string `json:"version,omitempty"`
	From     string `json:"from,omitempty"`
	To       string `json:"to,omitempty"`
}

type emergencyPlanItem struct {
	Kind   string
	Target string
	Ref    emergencyEffectRef
}

const (
	emergencyEffectUnits   = "units"
	emergencyEffectApp     = "app"
	emergencyEffectSite    = "site"
	emergencyEffectMove    = "move"
	emergencyEffectPackage = "package"
)

// planEmergencyOperation lists the checkpoints of one operation.
func planEmergencyOperation(ctx context.Context, q sqlQueryer, e securityEmergency, kind, targetVersion string) ([]emergencyPlanItem, error) {
	sites, err := emergencyRecordedSites(ctx, q, e.ID)
	if err != nil {
		return nil, err
	}
	service := emergencyComponents[e.Component].ServiceID
	units := emergencyPlanItem{emergencyEffectUnits, service + unitsSuffix(e.Version), emergencyEffectRef{Service: service, Version: e.Version}}
	var plan []emergencyPlanItem
	switch kind {
	case emergencyOpStop:
		// Pages first, so visitors see the maintenance page and not an error,
		// then the applications and units.
		for _, site := range sites {
			plan = append(plan, emergencyPlanItem{emergencyEffectSite, site.Domain, emergencyEffectRef{DomainID: site.DomainID, SiteID: site.SiteID, Domain: site.Domain}})
		}
		if e.Component == "node" {
			for _, site := range sites {
				plan = append(plan, emergencyPlanItem{emergencyEffectApp, site.Domain, emergencyEffectRef{DomainID: site.DomainID, SiteID: site.SiteID, Domain: site.Domain}})
			}
		} else {
			plan = append(plan, units)
		}
	case emergencyOpMove:
		for _, site := range sites {
			if !site.Maintenance {
				continue
			}
			plan = append(plan, emergencyPlanItem{emergencyEffectMove, site.Domain, emergencyEffectRef{DomainID: site.DomainID, SiteID: site.SiteID, Domain: site.Domain, From: e.Version, To: targetVersion}})
		}
	case emergencyOpRemove:
		plan = append(plan, emergencyPlanItem{emergencyEffectPackage, service + unitsSuffix(e.Version), emergencyEffectRef{Service: service, Version: e.Version}})
	case emergencyOpReenable:
		if e.Component == "node" {
			for _, site := range sites {
				plan = append(plan, emergencyPlanItem{emergencyEffectApp, site.Domain, emergencyEffectRef{DomainID: site.DomainID, SiteID: site.SiteID, Domain: site.Domain}})
			}
		} else {
			plan = append(plan, units)
		}
		for _, site := range sites {
			plan = append(plan, emergencyPlanItem{emergencyEffectSite, site.Domain, emergencyEffectRef{DomainID: site.DomainID, SiteID: site.SiteID, Domain: site.Domain}})
		}
	}
	return plan, nil
}

func unitsSuffix(version string) string {
	if version == "" {
		return ""
	}
	return ":" + version
}

// emergencyAccept is the result of accepting a step.
type emergencyAccept struct {
	OperationID string `json:"operation_id,omitempty"`
	Started     bool   `json:"started"`
	Stage       string `json:"stage"`
}

// createEmergency is step 1: the record, withdrawn from new choice.
func (p *Panel) createEmergency(ctx context.Context, component, version, reason, recommended string, actorID int) (securityEmergency, error) {
	component, version = strings.TrimSpace(component), strings.TrimSpace(version)
	if !validEmergencyTarget(component, version) {
		return securityEmergency{}, errEmergencyBadTarget
	}
	recommended = strings.TrimSpace(recommended)
	if recommended != "" && (!validEmergencyTarget(component, recommended) || recommended == version) {
		return securityEmergency{}, errEmergencyBadMoveGoal
	}
	id, err := newServiceOperationID()
	if err != nil {
		return securityEmergency{}, err
	}
	db := p.db.GetDB()
	var existing string
	err = db.QueryRowContext(ctx, `SELECT id FROM security_emergencies WHERE component = ? AND version = ? AND stage <> 'resolved'`, component, version).Scan(&existing)
	if err == nil {
		e, loadErr := loadEmergency(ctx, db, existing)
		if loadErr != nil {
			return e, loadErr
		}
		return e, errEmergencyExists
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return securityEmergency{}, err
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO security_emergencies (id, component, version, reason, recommended, created_by)
		VALUES (?, ?, ?, ?, ?, ?)`, id, component, version, boundedEmergencyReason(reason), nullIfEmpty(recommended), nullablePositiveInt(actorID)); err != nil {
		return securityEmergency{}, err
	}
	return loadEmergency(ctx, db, id)
}

func boundedEmergencyReason(reason string) string {
	reason = strings.TrimSpace(reason)
	if len(reason) > emergencyReasonMaxLen {
		reason = reason[:emergencyReasonMaxLen]
	}
	return reason
}

// cancelWithdrawal resolves an emergency that never went beyond step 1.
func (p *Panel) cancelWithdrawal(ctx context.Context, id string, actorID int) (securityEmergency, error) {
	result, err := p.db.GetDB().ExecContext(ctx, `
		UPDATE security_emergencies SET stage = 'resolved', withdrawn = 0, resolved_by = ?, resolved_at = datetime('now'), updated_at = datetime('now')
		WHERE id = ? AND stage = 'withdrawn'`, nullablePositiveInt(actorID), id)
	if err != nil {
		return securityEmergency{}, err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return securityEmergency{}, errEmergencyWrongStage
	}
	return loadEmergency(ctx, p.db.GetDB(), id)
}

// acceptEmergencyOperation stores one accepted step and its checkpoints. A
// step already running or already reached answers with that operation and
// starts nothing (a repeated request never runs twice).
func (p *Panel) acceptEmergencyOperation(ctx context.Context, emergencyID, kind string, confirm emergencyConfirmation, actorID int) (emergencyAccept, error) {
	tx, err := p.db.GetDB().BeginTx(ctx, nil)
	if err != nil {
		return emergencyAccept{}, err
	}
	defer tx.Rollback()
	// Take the write lock first so the reads below cannot go stale.
	if _, err := tx.ExecContext(ctx, `UPDATE security_emergencies SET updated_at = updated_at WHERE id = ?`, emergencyID); err != nil {
		return emergencyAccept{}, err
	}
	e, err := loadEmergency(ctx, tx, emergencyID)
	if err != nil {
		return emergencyAccept{}, err
	}
	result := emergencyAccept{Stage: e.Stage}
	running, runningKind, err := runningEmergencyOperation(ctx, tx, emergencyID)
	if err != nil {
		return result, err
	}
	if running != "" {
		if runningKind == kind {
			result.OperationID = running
			return result, tx.Commit()
		}
		return result, errEmergencyBusy
	}
	if e.Stage == emergencyStageResolved {
		return result, errEmergencyWrongStage
	}
	switch kind {
	case emergencyOpStop:
		if e.Stage != emergencyStageWithdrawn {
			return result, errEmergencyWrongStage
		}
		sites, err := emergencyAffectedSites(ctx, tx, e.Component, e.Version)
		if err != nil {
			return result, err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM security_emergency_sites WHERE emergency_id = ?`, e.ID); err != nil {
			return result, err
		}
		for _, site := range sites {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO security_emergency_sites (emergency_id, domain_id, site_id, subscription_id, domain, maintenance)
				VALUES (?, ?, ?, ?, ?, 1)`, e.ID, site.DomainID, site.SiteID, site.SubscriptionID, site.Domain); err != nil {
				return result, err
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE security_emergencies SET stage = 'stopped', withdrawn = 1, updated_at = datetime('now') WHERE id = ?`, e.ID); err != nil {
			return result, err
		}
		e.Stage = emergencyStageStopped
		if err := createEmergencyNotices(ctx, tx, e, sites, "emergency"); err != nil {
			return result, err
		}
	case emergencyOpMove:
		if e.Stage != emergencyStageStopped && e.Stage != emergencyStageRemoved {
			return result, errEmergencyWrongStage
		}
		target := strings.TrimSpace(confirm.TargetVersion)
		if e.Version == "" || !validEmergencyTarget(e.Component, target) || target == e.Version {
			return result, errEmergencyBadMoveGoal
		}
		withdrawn, err := withdrawnVersions(ctx, tx)
		if err != nil {
			return result, err
		}
		if versionWithdrawn(withdrawn, e.Component, target) {
			return result, errEmergencyBadMoveGoal
		}
		confirm.TargetVersion = target
	case emergencyOpRemove:
		if e.Stage == emergencyStageRemoved {
			id, err := latestEmergencyOperation(ctx, tx, e.ID)
			result.OperationID = id
			return result, err
		}
		if e.Stage != emergencyStageStopped {
			return result, errEmergencyWrongStage
		}
	case emergencyOpReenable:
		if e.Stage != emergencyStageStopped && e.Stage != emergencyStageRemoved {
			return result, errEmergencyWrongStage
		}
	default:
		return result, errEmergencyWrongStage
	}
	id, err := newServiceOperationID()
	if err != nil {
		return result, err
	}
	plan, err := planEmergencyOperation(ctx, tx, e, kind, confirm.TargetVersion)
	if err != nil {
		return result, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO security_emergency_operations (id, emergency_id, kind, target_version, reason, keep_withdrawn, actor_id)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, id, e.ID, kind, nullIfEmpty(confirm.TargetVersion), boundedEmergencyReason(confirm.Reason),
		confirm.KeepWithdrawn, nullablePositiveInt(actorID)); err != nil {
		return result, err
	}
	for seq, item := range plan {
		ref, err := json.Marshal(item.Ref)
		if err != nil {
			return result, err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO security_emergency_effects (operation_id, seq, kind, target, ref) VALUES (?, ?, ?, ?, ?)`,
			id, seq, item.Kind, item.Target, string(ref)); err != nil {
			return result, err
		}
	}
	if err := tx.Commit(); err != nil {
		return result, err
	}
	result.OperationID, result.Started, result.Stage = id, true, e.Stage
	p.kickEmergencyWorker()
	return result, nil
}

// siteUnderEmergencyMaintenance: the site shows the maintenance page. Only
// while it still uses the affected version, so a site moved by any path
// shows itself again.
func siteUnderEmergencyMaintenance(ctx context.Context, q sqlQueryer, domainID int) (bool, error) {
	var found bool
	err := q.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM security_emergency_sites es
			JOIN security_emergencies e ON e.id = es.emergency_id
			JOIN sites s ON s.id = es.site_id AND s.domain_id = es.domain_id
			WHERE es.domain_id = ? AND es.maintenance = 1 AND e.stage IN ('stopped', 'removed')
			  AND ((e.component = 'php' AND COALESCE(s.project_type, 'php') = 'php' AND s.php_version = e.version)
			    OR (e.component = 'node' AND s.project_type = 'node' AND s.runtime_version = e.version)
			    OR e.component IN ('mariadb', 'postgresql')))`, domainID).Scan(&found)
	return found, err
}

// noteEmergencySiteMoved records that a site left an emergency's version by
// an ordinary path (the customer's one-click switch, a site settings save):
// it leaves the maintenance list. The render follows the site's version by
// itself; this keeps the record and the screens truthful.
func (p *Panel) noteEmergencySiteMoved(ctx context.Context, domainID int) {
	_, _ = p.db.GetDB().ExecContext(ctx, `
		UPDATE security_emergency_sites SET maintenance = 0, updated_at = datetime('now'),
		       moved_to = (SELECT CASE WHEN COALESCE(s.project_type, 'php') = 'node' THEN s.runtime_version ELSE s.php_version END
		                   FROM sites s WHERE s.id = security_emergency_sites.site_id)
		WHERE domain_id = ? AND maintenance = 1 AND NOT EXISTS (
			SELECT 1 FROM security_emergencies e JOIN sites s ON s.id = security_emergency_sites.site_id
			WHERE e.id = security_emergency_sites.emergency_id
			  AND ((e.component = 'php' AND COALESCE(s.project_type, 'php') = 'php' AND s.php_version = e.version)
			    OR (e.component = 'node' AND s.project_type = 'node' AND s.runtime_version = e.version)
			    OR e.component IN ('mariadb', 'postgresql')))`, domainID)
}

// emergencyWorkerState runs the operations one at a time.
type emergencyWorkerState struct {
	once sync.Once
	kick chan struct{}
	mu   sync.Mutex
}
