package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

// The removal dialog says what a removal affects on THIS server, by name, and
// what it keeps (alpha.100, owner decision 2026-10-22). Everything here is
// read from what the Panel already holds: the catalogue, the stored component
// record (the last scan), the Panel's own database and the panel certificate
// file. It adds no Agent RPC and starts no mutation and no scan: the host
// profile may come from cached Agent facts, and for the firewall engine the
// firewall's state is read through the existing read (Agent.FirewallStatus),
// bounded. The dialog reads it once when it opens and a failed read never
// blocks the removal itself.
//
// The facts it returns are the uninstall path's own conditions, shared with
// handleServiceUninstall and Agent.UninstallService through the helpers
// below, so the dialog cannot promise something the removal does not do.
//
// Kaldırma penceresi bu sunucuda neyin etkileneceğini adıyla ve neyin yerinde
// kaldığını söyler. Buradaki her şey Panel'in zaten tuttuğundan okunur:
// katalog, saklanan bileşen kaydı, Panel veritabanı ve panel sertifikası dosyası.
// Yeni Agent çağrısı eklemez, değişiklik ya da tarama başlatmaz; güvenlik duvarı
// motoru için duvarın durumu var olan okuma ile, sınırlı biçimde okunur.
// Okunamaması kaldırmayı engellemez.
//
// alpha.104 (2026-10-23 Frankfurt incident): the answer also carries the
// package manager's own simulation of the exact removal, read through the
// Agent's read-only Agent.ServiceRemovalPreviewV1 (bounded; it never waits
// for the package lock and never mutates): what it would remove, anything
// it would take that the removal did not name (the removal is then refused
// when pressed), and the PHP tools running on the PHP being removed. With
// ?package= it answers for one version of a runtime. A failed or slow
// simulation read says "unknown"; the removal is still checked by the Agent
// when pressed. / Yanıt, paket yöneticisinin benzetimini de taşır.

const serviceRemovalImpactPath = "/api/v1/service/removal-impact"

// removalHostedNameCap: how many names a hosted list carries; the count says
// the rest. / Barındırılan listenin taşıdığı ad sayısı; kalanı sayı söyler.
const removalHostedNameCap = blockerCap

type serviceRemovalHosted struct {
	// Kind names what is counted: sites, php_sites, node_sites, databases,
	// mail_addresses, mail_domains, mailboxes, certificates (certbot: the
	// certificates the Panel renews with it), firewall (nftables: the panel
	// firewall it enforces), vpn_peers (WireGuard: the VPN peers the Panel
	// records), ftp_accounts (vsftpd: the FTP accounts the Panel records; SFTP accounts do not depend on vsftpd),
	// unrecorded (caches: the Panel does not record which sites use them, so
	// nothing is claimed either way).
	Kind  string   `json:"kind"`
	Count int      `json:"count"`
	Names []string `json:"names"`
	// BlocksRemoval: the uninstall refuses while Count > 0
	// (SERVICE_HAS_DEPENDENTS, serviceDependents).
	BlocksRemoval bool `json:"blocks_removal"`
	// Firewall is the panel firewall's state for the firewall kind; Known is
	// false when it could not be read.
	Firewall *serviceRemovalFirewall `json:"firewall,omitempty"`
}

type serviceRemovalFirewall struct {
	Known     bool `json:"known"`
	Enabled   bool `json:"enabled"`
	OpenPorts int  `json:"open_ports"`
}

// removalHostFacts are the facts outside the database a few kinds need: the
// panel certificate and the mail host certificate (certbot) and the firewall
// (nftables). The handler reads them; tests pass them.
type removalHostFacts struct {
	PanelCertificate string
	MailHosts        []string
	Firewall         *serviceRemovalFirewall
}

type serviceRemovalBundle struct {
	ID       string         `json:"id"`
	NameCode bundleTextCode `json:"name_code"`
}

type serviceRemovalImpactResponse struct {
	ServiceID string `json:"service_id"`
	// Removal is how the uninstall removes it: "packages" (stop, disable and
	// purge through the package manager), "webmail_tree" (Roundcube, which
	// was never a package) or "unsupported" (this package family has no
	// catalogue packages for it, so Agent.UninstallService refuses: "cannot be
	// removed automatically on this system yet").
	Removal string `json:"removal"`
	// HasUnits: the catalogue names service units (or a unit pattern, or
	// helper units) the uninstall stops and disables first. A tool without
	// units has nothing to stop.
	HasUnits bool `json:"has_units"`
	// SetupRequired: server setup installs it on every server it prepares and
	// its readiness check expects it (serverSetupRequiredComponents).
	SetupRequired bool `json:"setup_required"`
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
	// Package and Version name the one version being removed (the PHP-FPM
	// versions panel, alpha.104); empty for the whole component.
	Package string `json:"package,omitempty"`
	Version string `json:"version,omitempty"`
	// Simulation is the package manager's own answer for the exact removal
	// (Agent.ServiceRemovalPreviewV1, alpha.104), or null when the removal
	// does not go through the package manager.
	Simulation *serviceRemovalSimulation `json:"simulation"`
}

// serviceRemovalSimulation is what the package manager says the removal
// would take on this server. State: "checked" (Remove and Collateral are its
// answer), "busy" (another package operation was running; nothing was
// simulated), "unknown" (it could not be read in time or failed),
// "unsupported" (this package family has no removal here) or
// "not_installed". Refuses: the removal will be refused when pressed —
// Collateral names packages it would also take, BoundTools the PHP tools
// running on the PHP being removed. DefaultRuntime: the component's own
// distribution package (php-fpm, postgresql) depends on this version, i.e.
// it is the system's default.
// serviceRemovalSimulation, paket yöneticisinin bu kaldırma için yanıtıdır.
type serviceRemovalSimulation struct {
	State          string                               `json:"state"`
	Remove         []string                             `json:"remove"`
	Collateral     []transport.PackageRemovalCollateral `json:"collateral"`
	BoundTools     []string                             `json:"bound_tools"`
	DefaultRuntime bool                                 `json:"default_runtime"`
	Refuses        bool                                 `json:"refuses"`
	// KeepsData (alpha.107, D-038 §8): a database engine is removed, not
	// purged; DataPaths are where its data stays on this server.
	KeepsData bool     `json:"keeps_data,omitempty"`
	DataPaths []string `json:"data_paths,omitempty"`
	// HoldsDatabases / DatabasesUnknown (alpha.108, D-038 §2): the
	// PostgreSQL clusters of this removal that still hold databases, by name,
	// or could not be asked; the removal is refused while either is not empty.
	HoldsDatabases   []string `json:"holds_databases,omitempty"`
	DatabasesUnknown []string `json:"databases_unknown,omitempty"`
	// Traces (alpha.108, D-038 §13): what the system's own traces show
	// using it outside CelikPanel; TracesState "checked", "partial" (with
	// TracesUnread) or "" (not looked at). Warnings; never Refuses.
	Traces       []transport.RemovalTrace `json:"traces"`
	TracesState  string                   `json:"traces_state,omitempty"`
	TracesUnread []string                 `json:"traces_unread,omitempty"`
}

// removalSimulationReadBound bounds the Agent's simulation for the dialog.
const removalSimulationReadBound = 30 * time.Second

// removalSimulationFrom turns the Agent's preview into the dialog's answer.
func removalSimulationFrom(serviceID string, preview transport.ServiceRemovalPreviewResponse, err error) *serviceRemovalSimulation {
	simulation := &serviceRemovalSimulation{State: "unknown", Remove: []string{}, Collateral: []transport.PackageRemovalCollateral{}, BoundTools: []string{}, Traces: []transport.RemovalTrace{}}
	if err != nil {
		return simulation
	}
	if preview.BoundTools != nil {
		simulation.BoundTools = append(simulation.BoundTools, preview.BoundTools...)
	}
	switch preview.State {
	case "checked":
		simulation.State = "checked"
		simulation.Remove = append(simulation.Remove, preview.Remove...)
		simulation.Collateral = append(simulation.Collateral, preview.Collateral...)
		for _, item := range preview.Collateral {
			if item.Reason == "component" && item.Component == serviceID {
				simulation.DefaultRuntime = true
			}
		}
	case "busy", "unsupported", "not_installed":
		simulation.State = preview.State
	}
	simulation.HoldsDatabases = append([]string{}, preview.HoldsDatabases...)
	simulation.DatabasesUnknown = append([]string{}, preview.DatabasesUnknown...)
	simulation.Refuses = len(simulation.BoundTools) > 0 || (simulation.State == "checked" && len(simulation.Collateral) > 0) ||
		len(simulation.HoldsDatabases) > 0 || len(simulation.DatabasesUnknown) > 0
	simulation.KeepsData = preview.KeepsData
	simulation.DataPaths = append([]string{}, preview.DataPaths...)
	simulation.Traces = append(simulation.Traces, preview.Traces...)
	simulation.TracesState = preview.TracesState
	simulation.TracesUnread = append([]string{}, preview.TracesUnread...)
	return simulation
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
func removalHosted(ctx context.Context, db *sql.DB, svc *core.ManagedService, facts removalHostFacts) (*serviceRemovalHosted, error) {
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
	case svc.ID == "certbot":
		// The certificates the Panel issues and renews with certbot: the
		// panel's own, the mail host's (server setup issues it) and every
		// Let's Encrypt site certificate it records.
		names := []string{}
		if facts.PanelCertificate != "" {
			names = append(names, facts.PanelCertificate)
		}
		names = append(names, facts.MailHosts...)
		count, sites, err := hostedLabels(ctx, db, `
			SELECT d.name FROM ssl_certificates sc JOIN domains d ON d.id = sc.domain_id
			WHERE sc.type = 'letsencrypt' AND sc.auto_renew = 1 AND sc.status = 'active'
			ORDER BY d.name`)
		if err != nil {
			return nil, err
		}
		count += len(names)
		names = append(names, sites...)
		if len(names) > removalHostedNameCap {
			names = names[:removalHostedNameCap]
		}
		return &serviceRemovalHosted{Kind: "certificates", Count: count, Names: names}, nil
	case svc.ID == "nftables":
		firewall := facts.Firewall
		if firewall == nil {
			firewall = &serviceRemovalFirewall{}
		}
		return &serviceRemovalHosted{Kind: "firewall", Names: []string{}, Firewall: firewall}, nil
	case svc.ID == "wireguard":
		// The VPN peers the Panel keeps active; they can no longer connect.
		count, names, err := hostedLabels(ctx, db, `SELECT name FROM vpn_peers WHERE desired_state = 'active' ORDER BY name`)
		if err != nil {
			return nil, err
		}
		return &serviceRemovalHosted{Kind: "vpn_peers", Count: count, Names: names}, nil
	case svc.ID == "vsftpd":
		count, names, err := hostedLabels(ctx, db, `SELECT username FROM ftp_accounts WHERE access_type = 'ftp' ORDER BY username`)
		if err != nil {
			return nil, err
		}
		return &serviceRemovalHosted{Kind: "ftp_accounts", Count: count, Names: names}, nil
	case svc.ID == "redis" || svc.ID == "valkey" || svc.ID == "memcached":
		return &serviceRemovalHosted{Kind: "unrecorded", Names: []string{}}, nil
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
func serviceRemovalImpact(ctx context.Context, db *sql.DB, svc *core.ManagedService, family string, installed map[string]bool, facts removalHostFacts) (serviceRemovalImpactResponse, error) {
	response := serviceRemovalImpactResponse{
		ServiceID:                 svc.ID,
		Removal:                   "packages",
		PackageFamily:             family,
		Packages:                  append([]string{}, svc.Packages[family]...),
		AllVersions:               core.UninstallPurgesAllVersions(svc, family),
		FirewallPorts:             portStrings(svc.FirewallPorts),
		MailFilterRewired:         uninstallRewiresMailFilters(svc),
		DatabaseToolsReconfigured: uninstallReconfiguresDatabaseTools(svc.ID),
		HasUnits:                  len(svc.SystemNames) > 0 || svc.SystemNamePattern != "" || len(svc.HelperUnits) > 0,
		SetupRequired:             slices.Contains(serverSetupRequiredComponents, svc.ID),
		RequiredBy:                removalRequiredBy(svc, installed),
		Bundles:                   removalBundles(svc.ID, installed),
	}
	switch {
	case svc.ID == "roundcube":
		response.Removal = "webmail_tree"
		response.Packages = []string{}
	case family != "" && len(response.Packages) == 0:
		// Agent.UninstallService: "<name> cannot be removed automatically on
		// this system yet".
		response.Removal = "unsupported"
	}
	if response.FirewallPorts == nil {
		response.FirewallPorts = []string{}
	}
	hosted, err := removalHosted(ctx, db, svc, facts)
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
	// package: one version of a runtime (the PHP-FPM versions panel).
	packageName := strings.TrimSpace(r.URL.Query().Get("package"))
	version := ""
	if packageName != "" {
		version = versionFromPackage(svc, packageName)
		if version == "" {
			writeClientError(w, http.StatusBadRequest, "package does not name a removable version of this service")
			return
		}
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
	var response serviceRemovalImpactResponse
	if packageName != "" {
		response, err = serviceVersionRemovalImpact(r.Context(), p.db.GetDB(), svc, host.PackageFamily, packageName, version)
	} else {
		response, err = serviceRemovalImpact(r.Context(), p.db.GetDB(), svc, host.PackageFamily, installed, p.removalHostFacts(r.Context(), svc.ID))
	}
	if err != nil {
		writeServerError(w, err)
		return
	}
	if response.Removal == "packages" {
		preview, previewErr := removalPackagePreview(p, r.Context(), transport.ServiceRemovalPreviewRequest{ID: svc.ID, Package: packageName})
		if previewErr != nil {
			log.Printf("removal impact: package manager simulation for %s%s could not be read: %v", svc.ID, pkgSuffix(packageName), previewErr)
		}
		response.Simulation = removalSimulationFrom(svc.ID, preview, previewErr)
		response.Simulation.Traces = p.unknownRemovalTraces(r.Context(), response.Simulation.Traces)
	}
	json.NewEncoder(w).Encode(response)
}

// serviceVersionRemovalImpact is the answer for one version: the sites the
// Panel records on it (the removal refuses while there are any,
// RUNTIME_IN_USE); the component itself stays.
func serviceVersionRemovalImpact(ctx context.Context, db *sql.DB, svc *core.ManagedService, family, packageName, version string) (serviceRemovalImpactResponse, error) {
	response := serviceRemovalImpactResponse{
		ServiceID:     svc.ID,
		Removal:       "packages",
		PackageFamily: family,
		Packages:      []string{packageName},
		FirewallPorts: []string{},
		RequiredBy:    []string{},
		Bundles:       []serviceRemovalBundle{},
		HasUnits:      true,
		Package:       packageName,
		Version:       version,
	}
	count, lines, err := runtimeVersionBlockers(ctx, db, svc.ID, version)
	if err != nil {
		return serviceRemovalImpactResponse{}, err
	}
	names := make([]string, 0, len(lines))
	for _, line := range lines {
		if !strings.HasPrefix(line, "+") {
			names = append(names, line)
		}
	}
	kind := "php_sites"
	if svc.ID == "node" {
		kind = "node_sites"
	}
	response.Hosted = &serviceRemovalHosted{Kind: kind, Count: count, Names: names, BlocksRemoval: count > 0}
	return response, nil
}

// The readers of the facts outside the database, replaceable in tests.
var (
	removalPanelCertificate = func() string {
		// An owner certificate (CELIKPANEL_TLS_CERT/KEY, or a TLS directory
		// the Panel does not manage) is not renewed with certbot.
		if _, _, blocked := panelCertificateManagementBlocker(); blocked {
			return ""
		}
		cert := currentPanelCert()
		if !cert.HTTPSEnabled || cert.SelfSigned {
			return ""
		}
		if cert.Subject != "" {
			return cert.Subject
		}
		if len(cert.DNSNames) > 0 {
			return cert.DNSNames[0]
		}
		return ""
	}
	// The firewall read for the dialog never waits for panelFirewallMu: a
	// firewall change holding it answers "not known" at once, and the Agent
	// read itself is bounded by the RPC timeout (and, for the dialog, by
	// removalFirewallReadBound).
	removalFirewallStatus = func(p *Panel) (FirewallStatusResp, error) {
		if !panelFirewallMu.TryLock() {
			return FirewallStatusResp{}, errRemovalFirewallBusy
		}
		defer panelFirewallMu.Unlock()
		var status FirewallStatusResp
		err := p.callAgent("Agent.FirewallStatus", &transport.Empty{}, &status)
		return status, err
	}
	removalMailHostCertificates = func(p *Panel, ctx context.Context) []string {
		return p.setupMailCertificateHosts(ctx)
	}
)

var errRemovalFirewallBusy = errors.New("a firewall change is in progress")

// removalPackagePreview reads the package manager's simulation through the
// Agent's read-only preview, bounded for the dialog.
var removalPackagePreview = func(p *Panel, ctx context.Context, request transport.ServiceRemovalPreviewRequest) (transport.ServiceRemovalPreviewResponse, error) {
	callCtx, cancel := context.WithTimeout(ctx, removalSimulationReadBound)
	defer cancel()
	var preview transport.ServiceRemovalPreviewResponse
	err := p.callAgentContext(callCtx, "Agent.ServiceRemovalPreviewV1", &request, &preview)
	if err == nil && preview.State == "failed" {
		err = errors.New(preview.Error)
	}
	return preview, err
}

// setupMailCertificateHosts names the mail host certificates server setup
// recorded as issued: every "mail_certificate" step that succeeded in a
// recorded execution. Nothing recorded, nothing claimed.
func (p *Panel) setupMailCertificateHosts(ctx context.Context) []string {
	rows, err := p.db.GetDB().QueryContext(ctx, `SELECT execution_json FROM server_setup_executions ORDER BY updated_at DESC`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	hosts := []string{}
	for rows.Next() {
		var raw string
		if rows.Scan(&raw) != nil {
			continue
		}
		var execution serverSetupExecution
		if json.Unmarshal([]byte(raw), &execution) != nil {
			continue
		}
		for _, step := range execution.Steps {
			if step.Kind == "mail_certificate" && step.Status == "succeeded" && step.Target != "" && !slices.Contains(hosts, step.Target) {
				hosts = append(hosts, step.Target)
			}
		}
	}
	return hosts
}

// removalFirewallReadBound bounds the firewall read for the dialog.
const removalFirewallReadBound = 5 * time.Second

// removalHostFacts reads only what the asked component needs.
func (p *Panel) removalHostFacts(ctx context.Context, serviceID string) removalHostFacts {
	facts := removalHostFacts{}
	switch serviceID {
	case "certbot":
		facts.PanelCertificate = removalPanelCertificate()
		facts.MailHosts = removalMailHostCertificates(p, ctx)
	case "nftables":
		type answer struct {
			status FirewallStatusResp
			err    error
		}
		done := make(chan answer, 1)
		go func() {
			status, err := removalFirewallStatus(p)
			done <- answer{status, err}
		}()
		select {
		case got := <-done:
			// An answer that carries its own error, or an engine that cannot
			// be read, is not a known state.
			if got.err == nil && got.status.Error == "" && got.status.ErrorCode == "" && got.status.EngineAvailable {
				facts.Firewall = &serviceRemovalFirewall{Known: true, Enabled: got.status.Enabled, OpenPorts: len(got.status.TCPPorts) + len(got.status.UDPPorts)}
			}
		case <-time.After(removalFirewallReadBound):
		case <-ctx.Done():
		}
	}
	return facts
}

// unknownRemovalTraces drops what CelikPanel itself knows of and already
// names in the dialog: its sites' own vhosts (sites-enabled/<domain>.conf,
// counted as the sites on it) and connections to its sites' own PHP pools.
// What remains is what the owner set up outside CelikPanel (D-038 §13).
// CelikPanel'in zaten bildiği ve pencerede adını söylediği izler düşülür.
func (p *Panel) unknownRemovalTraces(ctx context.Context, traces []transport.RemovalTrace) []transport.RemovalTrace {
	if len(traces) == 0 {
		return traces
	}
	domains := map[string]bool{}
	sockets := map[string]bool{}
	if rows, err := p.db.GetDB().QueryContext(ctx, `SELECT d.name, COALESCE(s.php_fpm_socket, '') FROM domains d LEFT JOIN sites s ON s.domain_id = d.id`); err == nil {
		for rows.Next() {
			var name, socket string
			if rows.Scan(&name, &socket) == nil {
				domains[strings.ToLower(name)] = true
				if socket != "" {
					sockets[strings.Replace(socket, "/var/run/", "/run/", 1)] = true
				}
			}
		}
		rows.Close()
	}
	out := []transport.RemovalTrace{}
	for _, trace := range traces {
		switch trace.Kind {
		case "web_config":
			base := strings.ToLower(strings.TrimSuffix(filepath.Base(trace.Path), ".conf"))
			if domains[base] && strings.Contains(trace.Path, "/sites-") {
				continue
			}
		case "process":
			if sockets[strings.Replace(trace.Endpoint, "/var/run/", "/run/", 1)] {
				continue
			}
		}
		out = append(out, trace)
	}
	return out
}
