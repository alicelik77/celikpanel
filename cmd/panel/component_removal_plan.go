package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"sort"
	"strings"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/serviceapps"
)

// The removal plan (alpha.123, D-045): the one answer to "what requires the
// thing being removed?", computed from the component dependency map
// (internal/core/component_rules.go) and the Panel's own records. The dialog
// shows it, the uninstall enforces it, the remove-together operation runs it
// and the security emergency shows it before its removal step.
//
//   - Blockers: customer data and objects, the panel's own needs and other
//     components that are themselves in use. The removal is refused while
//     there is one; each is named with where it is moved or removed.
//   - Together: leaf components nothing else uses (and settings), removed on
//     the server in the same durable operation, dependents first.
//   - Stops: what stops working, said plainly in the confirmation.
//   - Withdraws: the offering ids plans, reseller narrowings and custom sets
//     lose as part of the removal.
//
// Kaldırma planı: "kaldırılanı ne gerektiriyor?" sorusunun tek yanıtı.
// Engeller reddeder; birlikte kaldırılacaklar sunucuda aynı işlemde gider;
// durdukları onayda söylenir; sunumlar kaldırmayla geri çekilir.

const (
	removalKindComponent       = "component"
	removalKindFirewallUnknown = "panel_firewall_unknown"
	removalNoteUnitsUnknown    = "runtime_units_unknown"
)

type removalItem struct {
	Name string `json:"name"`
	Link string `json:"link,omitempty"`
}

type removalDependent struct {
	Kind      string        `json:"kind"`
	Component string        `json:"component,omitempty"`
	Count     int           `json:"count"`
	Items     []removalItem `json:"items"`
}

type removalStop struct {
	Kind  string `json:"kind"`
	Count int    `json:"count"`
}

type removalPlan struct {
	Blockers     []removalDependent `json:"blockers"`
	Together     []string           `json:"together"`
	Stops        []removalStop      `json:"stops"`
	Withdraws    []string           `json:"withdraws"`
	WithdrawSets int                `json:"withdraw_sets"`
	Notes        []string           `json:"notes"`
}

func emptyRemovalPlan() removalPlan {
	return removalPlan{Blockers: []removalDependent{}, Together: []string{}, Stops: []removalStop{},
		Withdraws: []string{}, Notes: []string{}}
}

// TogetherComponents are the plan's component members (not settings).
func (plan removalPlan) TogetherComponents() []string {
	out := []string{}
	for _, item := range plan.Together {
		if !strings.HasPrefix(item, "setting:") {
			out = append(out, item)
		}
	}
	return out
}

// BlockerLines are the plan's blockers as one refusal's detail lines.
func (plan removalPlan) BlockerLines() []string {
	lines := []string{}
	for _, blocker := range plan.Blockers {
		for _, item := range blocker.Items {
			lines = append(lines, item.Name)
		}
		if extra := blocker.Count - len(blocker.Items); extra > 0 {
			lines = append(lines, fmt.Sprintf("+%d", extra))
		}
	}
	return lines
}

func domainLink(domain, tab string) string {
	link := "/domains/" + url.PathEscape(domain)
	if tab != "" {
		link += "?tab=" + tab
	}
	return link
}

// collectItems folds (name, link) rows into a capped item list and a count.
func collectItems(ctx context.Context, db *sql.DB, query string, args ...any) (int, []removalItem, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()
	count := 0
	items := []removalItem{}
	for rows.Next() {
		var name, link string
		if err := rows.Scan(&name, &link); err != nil {
			return 0, nil, err
		}
		count++
		if count <= blockerCap {
			items = append(items, removalItem{Name: name, Link: link})
		}
	}
	return count, items, rows.Err()
}

// siteItems: sites matching where, labelled "domain (owner)", linked to the
// domain's page (where its version or type is changed, or it is removed).
func siteItems(ctx context.Context, db *sql.DB, where string, args ...any) (int, []removalItem, error) {
	rows, err := db.QueryContext(ctx, fmt.Sprintf(siteLabelQuery, where), args...)
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()
	count := 0
	items := []removalItem{}
	for rows.Next() {
		var domain, owner string
		if err := rows.Scan(&domain, &owner); err != nil {
			return 0, nil, err
		}
		count++
		if count <= blockerCap {
			items = append(items, removalItem{Name: fmt.Sprintf("%s (%s)", domain, owner), Link: domainLink(domain, "")})
		}
	}
	return count, items, rows.Err()
}

// serviceAppItems: service app installs whose app runs on runtime, optionally
// only those whose unit is in units.
func serviceAppItems(ctx context.Context, db *sql.DB, runtime string, units map[string]bool) (int, []removalItem, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT i.id, i.app_id, d.name FROM service_app_installs i JOIN domains d ON d.id = i.domain_id
		ORDER BY d.name, i.id`)
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()
	count := 0
	items := []removalItem{}
	for rows.Next() {
		var id int
		var appID, domain string
		if err := rows.Scan(&id, &appID, &domain); err != nil {
			return 0, nil, err
		}
		app, ok := serviceapps.Lookup(appID)
		if !ok || app.Runtime != runtime {
			continue
		}
		if unit := serviceapps.UnitName(id); units != nil {
			if !units[unit] {
				continue
			}
			delete(units, unit)
		}
		count++
		if count <= blockerCap {
			items = append(items, removalItem{Name: fmt.Sprintf("%s (%s)", app.Name, domain), Link: domainLink(domain, "")})
		}
	}
	return count, items, rows.Err()
}

// removalUsers counts one kind of dependent of the component (or of one of
// its versions).
func removalUsers(ctx context.Context, db *sql.DB, kind, serviceID, version string, facts removalHostFacts) (removalDependent, error) {
	dependent := removalDependent{Kind: kind, Items: []removalItem{}}
	var (
		count int
		items []removalItem
		err   error
	)
	switch kind {
	case core.UseSites:
		count, items, err = siteItems(ctx, db, `COALESCE(s.project_type, 'php') != 'dnsonly'`)
	case core.UseApacheSites:
		count, items, err = siteItems(ctx, db, `COALESCE(s.project_type, 'php') != 'dnsonly' AND s.web_server = 'apache'`)
	case core.UsePHPSites:
		if version != "" {
			count, items, err = siteItems(ctx, db, `COALESCE(s.project_type, 'php') = 'php' AND s.php_version = ?`, version)
		} else {
			count, items, err = siteItems(ctx, db, `COALESCE(s.project_type, 'php') = 'php'`)
		}
	case core.UseNodeSites:
		if version != "" {
			count, items, err = siteItems(ctx, db, `s.project_type = 'node' AND s.runtime_version = ?`, version)
		} else {
			count, items, err = siteItems(ctx, db, `s.project_type = 'node'`)
		}
	case core.UseNodeServiceApps:
		if version == "" {
			count, items, err = serviceAppItems(ctx, db, serviceapps.RuntimeNode, nil)
			break
		}
		if !facts.RuntimeUnitsKnown {
			return dependent, nil
		}
		units := map[string]bool{}
		for _, unit := range facts.RuntimeUnits {
			units[unit] = true
		}
		count, items, err = serviceAppItems(ctx, db, serviceapps.RuntimeNode, units)
		// A unit the Panel has no record of still runs on it: named as is.
		for unit := range units {
			count++
			if len(items) < blockerCap {
				items = append(items, removalItem{Name: unit})
			}
		}
	case core.UsePythonApps:
		var apps int
		apps, items, err = serviceAppItems(ctx, db, serviceapps.RuntimePython, nil)
		if err != nil {
			return dependent, err
		}
		var more int
		var programs []removalItem
		more, programs, err = collectItems(ctx, db, `
			SELECT d.name, '/domains/' || d.name FROM runtime_apps a JOIN domains d ON d.id = a.domain_id
			WHERE a.kind = 'python' ORDER BY d.name`)
		count = apps + more
		for _, item := range programs {
			if len(items) < blockerCap {
				items = append(items, item)
			}
		}
	case core.UseDatabases:
		synonym := map[string]string{"mariadb": "mysql", "postgresql": "postgres"}[serviceID]
		count, items, err = collectItems(ctx, db, `
			SELECT db.name, '/databases' FROM databases_v2 db
			JOIN database_servers ds ON db.server_id = ds.id
			JOIN database_server_types dst ON ds.type_id = dst.id
			WHERE dst.name = ?
			UNION ALL
			SELECT name, '/databases' FROM databases WHERE LOWER(db_type) IN (?, ?)
			ORDER BY 1`, serviceID, serviceID, synonym)
	case core.UseMailAddresses:
		count, items, err = collectItems(ctx, db, `
			SELECT a.address, '/domains/' || d.name || '?tab=mail' FROM email_accounts a JOIN domains d ON d.id = a.domain_id
			UNION ALL
			SELECT f.source, '/domains/' || d.name || '?tab=mail' FROM email_forwardings f JOIN domains d ON d.id = f.domain_id
			ORDER BY 1`)
	case core.UseDNSZones:
		var lines []string
		count, lines, err = serviceDependents(ctx, db, serviceID)
		for _, line := range lines {
			if !strings.HasPrefix(line, "+") {
				items = append(items, removalItem{Name: line, Link: "/dns-infrastructure"})
			}
		}
	case core.UseCertificates:
		items = []removalItem{}
		if facts.PanelCertificate != "" {
			items = append(items, removalItem{Name: facts.PanelCertificate, Link: "/settings"})
		}
		for _, host := range facts.MailHosts {
			items = append(items, removalItem{Name: host, Link: "/components"})
		}
		var sites []removalItem
		count, sites, err = collectItems(ctx, db, `
			SELECT d.name, '/domains/' || d.name || '?tab=ssl' FROM ssl_certificates sc JOIN domains d ON d.id = sc.domain_id
			WHERE sc.type = 'letsencrypt' AND sc.auto_renew = 1 AND sc.status = 'active'
			ORDER BY d.name`)
		count += len(items)
		items = append(items, sites...)
		if len(items) > blockerCap {
			items = items[:blockerCap]
		}
	case core.UsePanelFirewall:
		switch {
		case facts.Firewall == nil || !facts.Firewall.Known:
			// Unknown is not "off": the removal waits for a reading.
			dependent.Kind = removalKindFirewallUnknown
			count, items = 1, []removalItem{{Name: "panel firewall", Link: "/security"}}
		case facts.Firewall.Enabled:
			count, items = 1, []removalItem{{Name: "panel firewall", Link: "/security"}}
		}
	case core.UseVPNPeers:
		count, items, err = collectItems(ctx, db, `SELECT name, '/vpn' FROM vpn_peers WHERE desired_state = 'active' ORDER BY name`)
	case core.UseMalwareQuarantine:
		var rows *sql.Rows
		rows, err = db.QueryContext(ctx, `
			SELECT d.name, COUNT(*) FROM malware_findings f JOIN domains d ON d.id = f.domain_id
			WHERE f.state = 'quarantined' GROUP BY d.name ORDER BY d.name`)
		if err != nil {
			return dependent, err
		}
		defer rows.Close()
		items = []removalItem{}
		for rows.Next() {
			var domain string
			var files int
			if err = rows.Scan(&domain, &files); err != nil {
				return dependent, err
			}
			count += files
			if len(items) < blockerCap {
				items = append(items, removalItem{Name: fmt.Sprintf("%s (%d)", domain, files), Link: domainLink(domain, "")})
			}
		}
		err = rows.Err()
	}
	if err != nil {
		return dependent, err
	}
	dependent.Count = count
	if items != nil {
		dependent.Items = items
	}
	return dependent, nil
}

// removalStopCount says how many things a stop touches (0 when the stop is
// a whole feature, not a list).
func removalStopCount(ctx context.Context, db *sql.DB, kind string) (int, error) {
	var query string
	switch kind {
	case core.StopWebmail:
		query = `SELECT COUNT(*) FROM email_accounts`
	case core.StopSpamFiltering:
		query = `SELECT COUNT(*) FROM domains WHERE id IN (SELECT domain_id FROM email_accounts UNION SELECT domain_id FROM email_forwardings)`
	case core.StopFTP:
		// Every hosting account (a site that is not DNS-only) may use FTP; the
		// Panel does not record FTP from SFTP use (both share the account).
		query = `SELECT COUNT(*) FROM sites WHERE COALESCE(project_type, 'php') != 'dnsonly'`
	default:
		return 0, nil
	}
	var n int
	err := db.QueryRowContext(ctx, query).Scan(&n)
	return n, err
}

// removalOfferingBase is the offering id a component's versions hang under.
func removalOfferingIDs(serviceID, version string) (exact []string, base string) {
	if version != "" {
		switch serviceID {
		case "php-fpm":
			return []string{versionOfferingID("php", version)}, ""
		case "node":
			return []string{versionOfferingID("node", version)}, ""
		}
		return nil, ""
	}
	if core.IsAdminOnlyComponent(serviceID) {
		return nil, ""
	}
	return nil, "component:" + serviceID
}

// removalWithdrawals lists the offering items in rights sets the removal
// withdraws, and how many sets hold one.
func removalWithdrawals(ctx context.Context, q sqlQueryer, serviceID, version string) ([]string, int, error) {
	exact, base := removalOfferingIDs(serviceID, version)
	if len(exact) == 0 && base == "" {
		return []string{}, 0, nil
	}
	where, args := "", []any{}
	if base != "" {
		where, args = `item = ? OR item LIKE ? ESCAPE '\'`, []any{base, strings.ReplaceAll(base, "_", `\_`) + ":%"}
	} else {
		if exact[0] == "" {
			return []string{}, 0, nil
		}
		where, args = `item = ?`, []any{exact[0]}
	}
	rows, err := q.QueryContext(ctx, `SELECT DISTINCT item FROM rights_set_items WHERE `+where+` ORDER BY item`, args...)
	if err != nil {
		return nil, 0, err
	}
	items := []string{}
	for rows.Next() {
		var item string
		if err := rows.Scan(&item); err != nil {
			rows.Close()
			return nil, 0, err
		}
		items = append(items, item)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	var sets int
	if err := q.QueryRowContext(ctx, `SELECT COUNT(DISTINCT set_id) FROM rights_set_items WHERE `+where, args...).Scan(&sets); err != nil {
		return nil, 0, err
	}
	return items, sets, nil
}

// withdrawRemovedOfferings deletes the items the plan named, after the
// removal succeeded. A plan, a reseller set or a custom set that offered the
// removed component or version loses that entry; nothing that runs changes
// (a site on it already blocked the removal).
func withdrawRemovedOfferings(ctx context.Context, db *sql.DB, serviceID, version string) (int64, error) {
	exact, base := removalOfferingIDs(serviceID, version)
	switch {
	case base != "":
		result, err := db.ExecContext(ctx, `DELETE FROM rights_set_items WHERE item = ? OR item LIKE ? ESCAPE '\'`,
			base, strings.ReplaceAll(base, "_", `\_`)+":%")
		if err != nil {
			return 0, err
		}
		return result.RowsAffected()
	case len(exact) == 1 && exact[0] != "":
		result, err := db.ExecContext(ctx, `DELETE FROM rights_set_items WHERE item = ?`, exact[0])
		if err != nil {
			return 0, err
		}
		return result.RowsAffected()
	}
	return 0, nil
}

// malwareScheduleOn reads the scheduled scan setting.
func malwareScheduleOn(ctx context.Context, db *sql.DB) bool {
	var raw string
	if err := db.QueryRowContext(ctx, `SELECT value FROM panel_settings WHERE key = ?`, settingMalwareSchedule).Scan(&raw); err != nil {
		return false
	}
	var schedule malwareSchedule
	return json.Unmarshal([]byte(raw), &schedule) == nil && schedule.Enabled
}

// computeRemovalPlan answers for svc (or one version of it) with installed
// the components the stored record holds.
func computeRemovalPlan(ctx context.Context, db *sql.DB, svc *core.ManagedService, version string, installed map[string]bool, facts removalHostFacts) (removalPlan, error) {
	plan := emptyRemovalPlan()
	rule := core.RuleFor(svc.ID)
	if version == "" {
		for _, dependent := range core.RequiringComponents(svc.ID, installed) {
			if core.RuleFor(dependent).Together && len(core.RequiringComponents(dependent, installed)) == 0 {
				plan.Together = append(plan.Together, dependent)
				continue
			}
			plan.Blockers = append(plan.Blockers, removalDependent{Kind: removalKindComponent, Component: dependent, Count: 1,
				Items: []removalItem{{Name: componentDisplayName(dependent), Link: "/components"}}})
		}
		for _, setting := range rule.TogetherSettings {
			if setting == core.TogetherMalwareSchedule && malwareScheduleOn(ctx, db) {
				plan.Together = append(plan.Together, setting)
			}
		}
	}
	for _, kind := range rule.UsedBy {
		// One version of a runtime: only what runs on that version.
		if version != "" && kind != core.UsePHPSites && kind != core.UseNodeSites && kind != core.UseNodeServiceApps {
			continue
		}
		dependent, err := removalUsers(ctx, db, kind, svc.ID, version, facts)
		if err != nil {
			return removalPlan{}, fmt.Errorf("removal plan %s: %w", kind, err)
		}
		if dependent.Count > 0 {
			plan.Blockers = append(plan.Blockers, dependent)
		}
	}
	// alpha.124 (D-045 follow-up): one PHP version that sites still use is
	// refused; the refusal also names the PHP tools that run on it, each with
	// its own window to move it, so the owner sees everything that must move
	// (lab: it named the site but not phpMyAdmin). Tools alone never refuse:
	// they are moved with the removal (move_tools_to).
	// Sitelerin kullandığı PHP sürümünün reddi, üzerindeki PHP araçlarını da
	// adlandırır.
	if version != "" && svc.ID == "php-fpm" && len(plan.Blockers) > 0 {
		if tools := phpToolsOnVersion(facts.PHPTools, version); len(tools.Items) > 0 {
			plan.Blockers = append(plan.Blockers, tools)
		}
	}
	if version != "" && svc.ID == "node" && !facts.RuntimeUnitsKnown {
		plan.Notes = append(plan.Notes, removalNoteUnitsUnknown)
	}
	if version == "" {
		stops := append([]string{}, rule.Stops...)
		for _, member := range plan.TogetherComponents() {
			stops = append(stops, core.RuleFor(member).Stops...)
		}
		seen := map[string]bool{}
		for _, kind := range stops {
			if seen[kind] {
				continue
			}
			seen[kind] = true
			n, err := removalStopCount(ctx, db, kind)
			if err != nil {
				return removalPlan{}, fmt.Errorf("removal plan stop %s: %w", kind, err)
			}
			plan.Stops = append(plan.Stops, removalStop{Kind: kind, Count: n})
		}
	}
	withdraws, sets, err := removalWithdrawals(ctx, db, svc.ID, version)
	if err != nil {
		return removalPlan{}, fmt.Errorf("removal plan offerings: %w", err)
	}
	for _, member := range plan.TogetherComponents() {
		more, moreSets, err := removalWithdrawals(ctx, db, member, "")
		if err != nil {
			return removalPlan{}, fmt.Errorf("removal plan offerings: %w", err)
		}
		withdraws, sets = append(withdraws, more...), sets+moreSets
	}
	sort.Strings(withdraws)
	plan.Withdraws, plan.WithdrawSets = withdraws, sets
	return plan, nil
}

// removalKindPHPTools: PHP tools that run on the version being removed.
const removalKindPHPTools = "php_tools"

// phpToolsOnVersion lists the PHP tools on one version, in a fixed order,
// each linked to its own page (where its PHP version is chosen).
func phpToolsOnVersion(tools map[string]string, version string) removalDependent {
	dependent := removalDependent{Kind: removalKindPHPTools, Items: []removalItem{}}
	for _, id := range []string{"phpmyadmin", "phppgadmin", "roundcube"} {
		if tools[id] == version {
			dependent.Items = append(dependent.Items, removalItem{Name: componentDisplayName(id), Link: "/components/" + id})
		}
	}
	dependent.Count = len(dependent.Items)
	return dependent
}

func componentDisplayName(id string) string {
	if svc := core.GetManagedServiceByID(id); svc != nil {
		return svc.Name
	}
	return id
}

// errRemovalPlanChanged: the request's together list is not the plan's.
var errRemovalPlanChanged = errors.New("remove-together list does not match the plan")

// sameTogether compares the request's list with the plan's, as sets.
func sameTogether(plan removalPlan, requested []string) bool {
	want := core.RemoveTogetherQualifier(plan.Together)
	got := core.RemoveTogetherQualifier(requested)
	return want == got
}

// nodeLineStays: another installed Node.js version of the removed version's
// line remains, so the line's offering (component:node:<major>) stays.
func nodeLineStays(services []ManagedServiceResponse, removed string) bool {
	line := offeredVersionLine("node", removed)
	for _, service := range services {
		if service.ID != "node" {
			continue
		}
		for _, version := range service.Versions {
			if version != removed && offeredVersionLine("node", version) == line {
				return true
			}
		}
	}
	return false
}

// runtimeUnitLabels names the Agent's units as the owner knows them: a
// service app by its name and domain, anything else by its unit name.
func (p *Panel) runtimeUnitLabels(ctx context.Context, units []string) []string {
	facts := removalHostFacts{RuntimeUnits: units, RuntimeUnitsKnown: true}
	dependent, err := removalUsers(ctx, p.db.GetDB(), core.UseNodeServiceApps, "node", "x", facts)
	if err != nil {
		return units
	}
	labels := []string{}
	for _, item := range dependent.Items {
		labels = append(labels, item.Name)
	}
	return labels
}

// cachedInstalledSet reads the installed components from the stored record
// (the last scan); nil when it cannot be read.
func (p *Panel) cachedInstalledSet(ctx context.Context) map[string]bool {
	var data string
	if err := p.db.GetDB().QueryRowContext(ctx, `SELECT data FROM service_scan_cache WHERE id = 1`).Scan(&data); err != nil {
		return nil
	}
	snapshot, err := decodeScanCacheSnapshot(data)
	if err != nil {
		return nil
	}
	installed := map[string]bool{}
	for _, service := range catalogViewForHost(snapshot.Observations, p.managedServiceHostProfile()) {
		if service.Installed() {
			installed[service.ID] = true
		}
	}
	return installed
}

// emergencyDependents is the removal plan of an emergency's target, as one
// list: the blockers and the components the rule would remove together. A
// plan that cannot be read is an empty list (the step still names its sites).
func (p *Panel) emergencyDependents(ctx context.Context, component, version string) []removalDependent {
	info, ok := emergencyComponents[component]
	svc := core.GetManagedServiceByID(info.ServiceID)
	if !ok || svc == nil {
		return []removalDependent{}
	}
	facts := p.removalHostFacts(ctx, svc.ID)
	if svc.ID == "node" && version != "" {
		facts = p.removalNodeVersionFacts(ctx, version)
	}
	plan, err := computeRemovalPlan(ctx, p.db.GetDB(), svc, version, p.cachedInstalledSet(ctx), facts)
	if err != nil {
		log.Printf("emergency: removal plan of %s %s could not be read: %v", component, version, err)
		return []removalDependent{}
	}
	out := append([]removalDependent{}, plan.Blockers...)
	for _, member := range plan.TogetherComponents() {
		out = append(out, removalDependent{Kind: removalKindComponent, Component: member, Count: 1,
			Items: []removalItem{{Name: componentDisplayName(member), Link: "/components"}}})
	}
	return out
}
