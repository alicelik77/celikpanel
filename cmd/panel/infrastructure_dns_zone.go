package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/alicelik/celikpanel/internal/hostname"
)

// The infrastructure zone on the primary (2026-10-16, the owner's decision 2):
// the zone that server setup created for the panels' own names (for example
// celikhost.com) is listed in Domains as an ordinary entry marked
// "Infrastructure zone - DNS only" and opens the existing DNS record editor.
// There is no tenant `domains` row and no schema change: the zone is
// identified by the ownership record setup already wrote
// (`server_setup_infrastructure_zone:<zone>` in panel_settings, matching the
// zone's pdns_domains id). Records that setup prepared stay managed by setup
// (the editor shows them and refuses to change or remove them; they are
// changed by extending the setup, which reviews the change first); every other
// record is the owner's and is edited like any domain record. Adding a website
// for the same name later goes through the ordinary Add domain path, which
// adopts the existing zone untouched; removing that website keeps the zone.
// Altyapi bolgesi Alan adlari listesinde "yalniz DNS" olarak gorunur ve mevcut
// kayit duzenleyicisini acar. Kiraci satiri ve sema degisikligi yoktur; kurulum
// kayitlari kurulumda yonetilir, sahibin kayitlari korunur.

const infrastructureDNSZonesPath = "/api/v1/dns/infrastructure-zones"

const (
	errCodeInfrastructureDNSZoneNotFound    = "INFRASTRUCTURE_DNS_ZONE_NOT_FOUND"
	errCodeInfrastructureDNSRecordBySetup   = "INFRASTRUCTURE_DNS_RECORD_MANAGED_BY_SETUP"
	infrastructureDNSRecordManagedBySetup   = "setup"
	infrastructureDNSRecordRequestedByPeer  = "peer"
	infrastructureDNSZoneKind               = "infrastructure"
	serverSetupInfrastructureDNSOwnerPrefix = "server_setup_infrastructure_zone:"
)

type infrastructureDNSZone struct {
	Zone string `json:"zone"`
	Kind string `json:"kind"`
	// DomainID is set once a website was added for the same name: the zone is
	// then listed in Domains as that website, and this entry only says so.
	DomainID int64 `json:"domain_id,omitempty"`
	Records  int   `json:"records"`
	// PeerRecords counts the host records the other server asked for over
	// the DNS connection (decision 3).
	PeerRecords int `json:"peer_records"`
}

// infrastructureDNSZones lists the zones server setup created, read only.
func (p *Panel) infrastructureDNSZones(ctx context.Context) ([]infrastructureDNSZone, error) {
	rows, err := p.db.GetDB().QueryContext(ctx, `SELECT key,value FROM panel_settings WHERE key LIKE ? ORDER BY key LIMIT 65`, serverSetupInfrastructureDNSOwnerPrefix+"%")
	if err != nil {
		return nil, err
	}
	type candidate struct {
		zone  string
		owner serverSetupInfrastructureDNSOwner
	}
	candidates := []candidate{}
	for rows.Next() {
		var key, raw string
		if err := rows.Scan(&key, &raw); err != nil {
			rows.Close()
			return nil, err
		}
		var owner serverSetupInfrastructureDNSOwner
		zone := strings.TrimPrefix(key, serverSetupInfrastructureDNSOwnerPrefix)
		if json.Unmarshal([]byte(raw), &owner) != nil || owner.Zone != zone {
			continue
		}
		candidates = append(candidates, candidate{zone, owner})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	if len(candidates) > 64 {
		return nil, errors.New("too many infrastructure DNS zones")
	}
	zones := []infrastructureDNSZone{}
	for _, c := range candidates {
		if canonical, err := hostname.CanonicalFQDN(c.zone); err != nil || canonical != c.zone {
			continue
		}
		var id int64
		err := p.db.GetDB().QueryRowContext(ctx, `SELECT id FROM pdns_domains WHERE name=?`, c.zone).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if id != c.owner.ZoneID {
			continue
		}
		entry := infrastructureDNSZone{Zone: c.zone, Kind: infrastructureDNSZoneKind}
		if err := p.db.GetDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM pdns_records WHERE domain_id=?`, id).Scan(&entry.Records); err != nil {
			return nil, err
		}
		if err := p.db.GetDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM panel_settings WHERE key LIKE ?`, remoteDNSHostRecordKeyPrefix(c.zone)+"%").Scan(&entry.PeerRecords); err != nil {
			return nil, err
		}
		err = p.db.GetDB().QueryRowContext(ctx, `SELECT id FROM domains WHERE name=?`, c.zone).Scan(&entry.DomainID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		zones = append(zones, entry)
	}
	return zones, nil
}

// isInfrastructureDNSZone: the zone has setup's verified ownership record.
func (p *Panel) isInfrastructureDNSZone(ctx context.Context, zone string) (bool, error) {
	var raw string
	err := p.db.GetDB().QueryRowContext(ctx, `SELECT value FROM panel_settings WHERE key=?`, serverSetupInfrastructureDNSOwnerKey(zone)).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var owner serverSetupInfrastructureDNSOwner
	if json.Unmarshal([]byte(raw), &owner) != nil || owner.Zone != zone {
		return false, nil
	}
	var id int64
	err = p.db.GetDB().QueryRowContext(ctx, `SELECT id FROM pdns_domains WHERE name=?`, zone).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil && id == owner.ZoneID, err
}

func infrastructureDNSRecordKey(name, recordType, content string) string {
	return canonicalDNSName(name) + "\x00" + strings.ToUpper(recordType) + "\x00" + strings.TrimSpace(content)
}

// infrastructureDNSManagedRecords names, for one zone, the address records
// that server setup prepares (from the current draft, the completed draft an
// extension started from and the latest run's plan, a union that protects
// more rather than less) and the host records the other server asked for.
// A zone that no setup draft names and no peer asked for has none.
func (p *Panel) infrastructureDNSManagedRecords(ctx context.Context, zone string) (map[string]string, error) {
	managed := map[string]string{}
	drafts := []serverSetupDraft{}
	state, err := p.loadServerSetup(ctx)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if err == nil {
		drafts = append(drafts, state.Draft)
		if state.ExtensionBase != nil {
			drafts = append(drafts, *state.ExtensionBase)
		}
		if execution, err := p.latestServerSetupExecution(ctx); err == nil && execution != nil {
			if plan, err := p.loadServerSetupPlan(ctx, execution.PlanID); err == nil {
				drafts = append(drafts, plan.Draft)
			}
		}
	}
	for _, d := range drafts {
		if d.InfrastructureDNS == nil || d.InfrastructureDNS.Zone != zone {
			continue
		}
		desired, err := serverSetupInfrastructureDNSDesired(d)
		if err != nil {
			continue
		}
		for _, r := range desired {
			if r.Type == "A" || r.Type == "AAAA" {
				managed[infrastructureDNSRecordKey(r.Name, r.Type, r.Content)] = infrastructureDNSRecordManagedBySetup
			}
		}
	}
	ledger, err := p.remoteDNSHostRecordLedger(ctx, zone)
	if err != nil {
		return nil, err
	}
	for _, entry := range ledger {
		key := infrastructureDNSRecordKey(entry.Name, entry.Type, entry.Content)
		if managed[key] == "" {
			managed[key] = infrastructureDNSRecordRequestedByPeer
		}
	}
	return managed, nil
}

// infrastructureDNSRecordManagedBySetup: whether a stored record is one that
// server setup prepares in an infrastructure zone. Any other zone: false.
func (p *Panel) infrastructureDNSRecordManagedBySetup(ctx context.Context, zone, name, recordType, content string) (bool, error) {
	infrastructure, err := p.isInfrastructureDNSZone(ctx, zone)
	if err != nil || !infrastructure {
		return false, err
	}
	managed, err := p.infrastructureDNSManagedRecords(ctx, zone)
	if err != nil {
		return false, err
	}
	return managed[infrastructureDNSRecordKey(name, recordType, content)] == infrastructureDNSRecordManagedBySetup, nil
}

func writeInfrastructureDNSRecordManagedBySetup(w http.ResponseWriter) {
	writeCodedError(w, http.StatusConflict, errCodeInfrastructureDNSRecordBySetup,
		"This address record is managed by server setup. Change it by extending the server setup, which shows the change for review before anything is applied; other records in this zone are yours to edit.", "/server-setup")
}

// handleInfrastructureDNSZones answers the list (GET) and, for one zone, the
// same zone and record endpoints a domain's DNS editor uses:
// /api/v1/dns/infrastructure-zones/{zone}/zone (GET) and
// /api/v1/dns/infrastructure-zones/{zone}/records (GET, POST, PUT, DELETE).
// Administrator only (the /api/v1/dns/ prefix).
func (p *Panel) handleInfrastructureDNSZones(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if r.URL.Path == infrastructureDNSZonesPath {
		if r.Method != http.MethodGet {
			rejectRouteMethod(w, []string{http.MethodGet})
			return
		}
		zones, err := p.infrastructureDNSZones(r.Context())
		if err != nil {
			writeServerError(w, fmt.Errorf("list infrastructure DNS zones: %w", err))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"zones": zones})
		return
	}
	segments, ok := strictRouteSegments(r, infrastructureDNSZonesPath+"/")
	if !ok || len(segments) != 2 {
		http.NotFound(w, r)
		return
	}
	zone, err := hostname.CanonicalFQDN(segments[0])
	if err != nil || zone != segments[0] {
		http.NotFound(w, r)
		return
	}
	allowed := map[string][]string{
		"zone":    {http.MethodGet},
		"records": {http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete},
	}[segments[1]]
	if allowed == nil {
		http.NotFound(w, r)
		return
	}
	if !routeAllows(r.Method, allowed) {
		rejectRouteMethod(w, allowed)
		return
	}
	infrastructure, err := p.isInfrastructureDNSZone(r.Context(), zone)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if !infrastructure {
		writeCodedError(w, http.StatusNotFound, errCodeInfrastructureDNSZoneNotFound, "This server has no infrastructure DNS zone with this name.", "/domains")
		return
	}
	if segments[1] == "zone" {
		p.handleGetDNSZone(w, r, zone)
		return
	}
	if r.Method != http.MethodGet {
		if _, ready := p.requireActiveDNSPublisherForMutation(w, r.Context()); !ready {
			return
		}
	}
	switch r.Method {
	case http.MethodGet:
		p.handleListDNSRecords(w, r, zone)
	case http.MethodPost:
		if p.handleAddDNSRecord(w, r, zone) {
			p.audit(r, "dns.infrastructure.record_add", "dns_zone", 0)
		}
	case http.MethodPut:
		if p.handleUpdateDNSRecord(w, r, zone) {
			p.audit(r, "dns.infrastructure.record_update", "dns_zone", 0)
		}
	case http.MethodDelete:
		if p.handleDeleteDNSRecord(w, r, zone) {
			p.audit(r, "dns.infrastructure.record_delete", "dns_zone", 0)
		}
	}
}
