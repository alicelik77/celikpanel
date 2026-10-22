package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/alicelik/celikpanel/internal/core"
)

// The removal dialog says what a removal affects on THIS server, by name, and
// what it keeps (alpha.100, owner decision 2026-10-22). Everything here is
// read from what the Panel already holds: the catalogue, the stored component
// record (the last scan) and the Panel's own database. It never asks the
// Agent, never scans and never changes anything; the dialog reads it once
// when it opens and a failed read never blocks the removal itself.
//
// The facts it returns are the uninstall path's own conditions, shared with
// handleServiceUninstall and Agent.UninstallService through the helpers
// below, so the dialog cannot promise something the removal does not do.
//
// Kaldırma penceresi bu sunucuda neyin etkileneceğini adıyla ve neyin yerinde
// kaldığını söyler. Buradaki her şey Panel'in zaten tuttuğundan okunur:
// katalog, saklanan bileşen kaydı ve Panel veritabanı. Agent'a sormaz, tarama
// yapmaz, hiçbir şeyi değiştirmez; okunamaması kaldırmayı engellemez.

const serviceRemovalImpactPath = "/api/v1/service/removal-impact"

// removalHostedNameCap: how many names a hosted list carries; the count says
// the rest. / Barındırılan listenin taşıdığı ad sayısı; kalanı sayı söyler.
const removalHostedNameCap = blockerCap

type serviceRemovalHosted struct {
	// Kind names what is counted: sites, php_sites, node_sites, databases,
	// mail_addresses, mail_domains, mailboxes.
	Kind  string   `json:"kind"`
	Count int      `json:"count"`
	Names []string `json:"names"`
	// BlocksRemoval: the uninstall refuses while Count > 0
	// (SERVICE_HAS_DEPENDENTS, serviceDependents).
	BlocksRemoval bool `json:"blocks_removal"`
}

type serviceRemovalBundle struct {
	ID       string         `json:"id"`
	NameCode bundleTextCode `json:"name_code"`
}

type serviceRemovalImpactResponse struct {
	ServiceID string `json:"service_id"`
	// Removal is how the uninstall removes it: "packages" (stop, disable and
	// purge through the package manager) or "webmail_tree" (Roundcube, which
	// was never a package).
	Removal string `json:"removal"`
	// PackageFamily is the host's package manager ("apt", "pacman", …) the
	// purge runs with; empty when it is not known.
	PackageFamily string `json:"package_family"`
	// Packages are the catalogue packages the purge names on this family.
	Packages []string `json:"packages"`
	// AllVersions: on apt the purge also takes every installed versioned
	// package of this component (php8.3-fpm, postgresql-17…) and its
	// companions, as Agent.UninstallService discovers them.
	AllVersions bool `json:"all_versions"`
	// FirewallPorts are the ports this component asks the firewall for.
	FirewallPorts []string `json:"firewall_ports"`
	// MailFilterRewired: Postfix's filter chain is composed again without it.
	MailFilterRewired bool `json:"mail_filter_rewired"`
	// DatabaseToolsReconfigured: the database tools' web server is set again
	// without it.
	DatabaseToolsReconfigured bool `json:"database_tools_reconfigured"`
	// RequiredBy are installed components whose requirement this one is the
	// only thing meeting (catalogue Requires, groups included), by id.
	RequiredBy []string `json:"required_by"`
	// Bundles are ready bundles complete today that this removal leaves
	// incomplete.
	Bundles []serviceRemovalBundle `json:"bundles"`
	// Hosted is what the Panel hosts on it, or null when nothing it records
	// runs on this component.
	Hosted *serviceRemovalHosted `json:"hosted"`
}

// uninstallRewiresMailFilters is handleServiceUninstall's own condition:
// removing a spam filter recomposes Postfix's filter chain.
func uninstallRewiresMailFilters(svc *core.ManagedService) bool {
	return svc != nil && svc.ConflictGroup == "spam-filter"
}

// uninstallReconfiguresDatabaseTools is handleServiceUninstall's own
// condition for reconfiguring the database tools' server.
func uninstallReconfiguresDatabaseTools(serviceID string) bool {
	return serviceID == "phpmyadmin" || serviceID == "phppgadmin"
}

// removalRequiredBy lists the installed components whose requirements this
// removal leaves unmet — the same RequirementsMissing the install path
// enforces, asked with and without the component.
func removalRequiredBy(target *core.ManagedService, installed map[string]bool) []string {
	without := make(map[string]bool, len(installed))
	for id, ok := range installed {
		if ok && id != target.ID {
			without[id] = true
		}
	}
	out := []string{}
	for i := range core.ManagedServices {
		svc := &core.ManagedServices[i]
		if svc.ID == target.ID || !installed[svc.ID] || len(svc.Requires) == 0 {
			continue
		}
		if len(core.RequirementsMissing(svc, without)) > len(core.RequirementsMissing(svc, installed)) {
			out = append(out, svc.ID)
		}
	}
	return out
}

func bundleCompleteWith(definition bundleDefinition, installed map[string]bool) bool {
	for _, id := range definition.Components {
		if !bundleComponentPresent(definition, id, installed) {
			return false
		}
	}
	return true
}

// removalBundles lists the ready bundles whose every component is present
// today and which this removal leaves incomplete.
func removalBundles(serviceID string, installed map[string]bool) []serviceRemovalBundle {
	without := make(map[string]bool, len(installed))
	for id, ok := range installed {
		if ok && id != serviceID {
			without[id] = true
		}
	}
	out := []serviceRemovalBundle{}
	for _, definition := range bundleDefinitions {
		if bundleCompleteWith(definition, installed) && !bundleCompleteWith(definition, without) {
			out = append(out, serviceRemovalBundle{ID: definition.ID, NameCode: definition.NameCode})
		}
	}
	return out
}

// hostedLabels folds label rows into a capped name list and a count.
func hostedLabels(ctx context.Context, db *sql.DB, query string, args ...any) (int, []string, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()
	count := 0
	names := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return 0, nil, err
		}
		count++
		if count <= removalHostedNameCap {
			names = append(names, name)
		}
	}
	return count, names, rows.Err()
}

// removalHosted says what the Panel hosts on this component. The blocking
// kinds are serviceDependents' own answer (the uninstall refuses while it is
// not empty); the others are counted here and never block.
func removalHosted(ctx context.Context, db *sql.DB, svc *core.ManagedService) (*serviceRemovalHosted, error) {
	blockingKind := map[string]string{
		"nginx": "sites", "apache": "sites",
		"php-fpm": "php_sites", "node": "node_sites",
		"mariadb": "databases", "postgresql": "databases",
		"postfix": "mail_addresses", "dovecot": "mail_addresses",
	}
	if kind, ok := blockingKind[svc.ID]; ok {
		count, lines, err := serviceDependents(ctx, db, svc.ID)
		if err != nil {
			return nil, err
		}
		names := make([]string, 0, len(lines))
		for _, line := range lines {
			// serviceDependents ends a long list with "+N"; the count says it here.
			if strings.HasPrefix(line, "+") {
				continue
			}
			names = append(names, line)
		}
		return &serviceRemovalHosted{Kind: kind, Count: count, Names: names, BlocksRemoval: count > 0}, nil
	}
	switch {
	case uninstallRewiresMailFilters(svc):
		// Mail for these domains is no longer filtered.
		count, names, err := hostedLabels(ctx, db, `
			SELECT d.name FROM domains d
			WHERE d.id IN (SELECT domain_id FROM email_accounts UNION SELECT domain_id FROM email_forwardings)
			ORDER BY d.name`)
		if err != nil {
			return nil, err
		}
		return &serviceRemovalHosted{Kind: "mail_domains", Count: count, Names: names}, nil
	case svc.ID == "roundcube":
		// These mailboxes lose webmail; their mail stays.
		count, names, err := hostedLabels(ctx, db, `SELECT address FROM email_accounts ORDER BY address`)
		if err != nil {
			return nil, err
		}
		return &serviceRemovalHosted{Kind: "mailboxes", Count: count, Names: names}, nil
	}
	return nil, nil
}

// serviceRemovalImpact builds the answer from the catalogue, the installed
// set of the stored component record and the database.
func serviceRemovalImpact(ctx context.Context, db *sql.DB, svc *core.ManagedService, family string, installed map[string]bool) (serviceRemovalImpactResponse, error) {
	response := serviceRemovalImpactResponse{
		ServiceID:                 svc.ID,
		Removal:                   "packages",
		PackageFamily:             family,
		Packages:                  append([]string{}, svc.Packages[family]...),
		AllVersions:               core.UninstallPurgesAllVersions(svc, family),
		FirewallPorts:             portStrings(svc.FirewallPorts),
		MailFilterRewired:         uninstallRewiresMailFilters(svc),
		DatabaseToolsReconfigured: uninstallReconfiguresDatabaseTools(svc.ID),
		RequiredBy:                removalRequiredBy(svc, installed),
		Bundles:                   removalBundles(svc.ID, installed),
	}
	if svc.ID == "roundcube" {
		response.Removal = "webmail_tree"
		response.Packages = []string{}
	}
	if response.FirewallPorts == nil {
		response.FirewallPorts = []string{}
	}
	hosted, err := removalHosted(ctx, db, svc)
	if err != nil {
		return serviceRemovalImpactResponse{}, err
	}
	response.Hosted = hosted
	return response, nil
}

// handleServiceRemovalImpact: GET ?service_id=<id>. Admin-only through the
// /api/v1/service/ prefix. Read-only.
func (p *Panel) handleServiceRemovalImpact(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		writeClientError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	serviceID := strings.TrimSpace(r.URL.Query().Get("service_id"))
	svc := core.GetManagedServiceByID(serviceID)
	if svc == nil {
		writeClientError(w, http.StatusBadRequest, "service_id does not name a component")
		return
	}
	// The DNS engines keep their own workflow; native cron is never removed.
	if managedDNSEngineServiceID(svc.ID) {
		writeDNSEngineWorkflowRequired(w)
		return
	}
	if svc.ID == core.NativeCronServiceID {
		writeNativeCronRemovalRefused(w)
		return
	}

	var data string
	err := p.db.GetDB().QueryRowContext(r.Context(),
		`SELECT data FROM service_scan_cache WHERE id = 1`).Scan(&data)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		writeServerError(w, fmt.Errorf("read service scan cache: %w", err))
		return
	}
	if errors.Is(err, sql.ErrNoRows) {
		writeCodedError(w, http.StatusServiceUnavailable, errCodeServiceStateUnverified,
			serviceStateUnverifiedMessage, "/services")
		return
	}
	snapshot, decodeErr := decodeScanCacheSnapshot(data)
	if decodeErr != nil {
		log.Printf("removal impact: cached service state is unverified: %v", decodeErr)
		writeCodedError(w, http.StatusServiceUnavailable, errCodeServiceStateUnverified,
			serviceStateUnverifiedMessage, "/services")
		return
	}
	host := p.managedServiceHostProfile()
	installed := map[string]bool{}
	for _, service := range catalogViewForHost(snapshot.Observations, host) {
		if service.Installed() {
			installed[service.ID] = true
		}
	}
	response, err := serviceRemovalImpact(r.Context(), p.db.GetDB(), svc, host.PackageFamily, installed)
	if err != nil {
		writeServerError(w, err)
		return
	}
	json.NewEncoder(w).Encode(response)
}
