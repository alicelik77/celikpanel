package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"strings"
)

// An alias answers in DNS like the domain it belongs to (alpha.144,
// alpha.143 lab item 1). Before this release no alias got a DNS record:
// an alias was a name in the site's web server file and its certificate
// only (alpha.142's lab: alias142.test REFUSED; alpha.143's lab:
// kampanya.randevu.dr.test NXDOMAIN on both servers). Now an alias that lies
// inside a zone this server keeps for the same account gets the site's
// address records in that zone, published like any change of the zone (on
// the pair, the secondary receives it by the zone's own transfer); they go
// again with the alias. Only the records CelikPanel added are removed: what
// it added is kept beside them (panel_settings "alias_dns_v1:<alias>"), so a
// record the owner made at that name is never taken away. An alias outside
// every zone of its account kept here gets nothing (its DNS is elsewhere, as
// before).
//
// Takma ad, ait olduğu alan adı gibi DNS'te yanıt verir: aynı hesabın bu
// sunucuda tutulan bir bölgesinin içindeyse o bölgeye sitenin adres kayıtları
// eklenir ve takma adla birlikte kaldırılır; yalnız CelikPanel'in eklediği
// kayıtlar silinir.

const aliasDNSKeyPrefix = "alias_dns_v1:"

// aliasDNSUnpublishedPrefix marks a zone whose alias records changed in the
// ledger while its publication did not finish (review of PR #355, findings
// 1 and 6): the hourly round publishes it again until it does
// (republishAliasZones), and the alias rows say they wait.
const aliasDNSUnpublishedPrefix = "alias_dns_unpublished_v1:"

// aliasDNSRecord is one record CelikPanel added for an alias.
type aliasDNSRecord struct {
	Type    string `json:"type"`
	Content string `json:"content"`
}

// aliasDNSAdded is what CelikPanel added for one alias: the zone and the
// records. Version 1.
type aliasDNSAdded struct {
	Version int              `json:"version"`
	Zone    string           `json:"zone"`
	Records []aliasDNSRecord `json:"records"`
	// Owner is the account that held the zone when the records were added.
	// It is only a hint for the journal: whether the records are removed is
	// decided by the current owners (withdrawAliasDNS).
	Owner int `json:"owner,omitempty"`
}

// Outcomes of publishAliasDNS (the alias answer's "dns").
const (
	// aliasDNSPublished: the alias's records are in its zone and published.
	aliasDNSPublished = "published"
	// aliasDNSPending: the records are saved in the zone but its publication
	// did not finish; the next publication of the zone carries them.
	aliasDNSPending = "pending"
	// aliasDNSElsewhere: no zone of the same account is kept on this server
	// above the alias; nothing was written.
	aliasDNSElsewhere = "elsewhere"
	// aliasDNSKept: the name already has a different address record in the
	// zone (the owner's); it was left as it is.
	aliasDNSKept = "kept"
	// aliasDNSFailed: the zone could not be changed; nothing was written.
	aliasDNSFailed = "failed"
)

func aliasDNSKey(alias string) string { return aliasDNSKeyPrefix + strings.ToLower(alias) }

// aliasDNSZone is the zone kept on this server that alias lies inside and
// that the account holding domainID holds: the longest such top-level domain
// of any of the account's subscriptions whose DNS is kept here.
func (p *Panel) aliasDNSZone(ctx context.Context, domainID int, alias string) (string, bool, error) {
	rows, err := p.db.GetDB().QueryContext(ctx, `
		SELECT d.name FROM domains d
		JOIN subscriptions s ON s.id = d.subscription_id
		WHERE d.parent_domain_id IS NULL AND d.status = 'active' AND s.owner_id = (
			SELECT owner.owner_id FROM domains alias_domain
			JOIN subscriptions owner ON owner.id = alias_domain.subscription_id
			WHERE alias_domain.id = ?)`, domainID)
	if err != nil {
		return "", false, fmt.Errorf("read the account's domains: %w", err)
	}
	defer rows.Close()
	name := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(alias)), ".")
	zone := ""
	for rows.Next() {
		var candidate string
		if err := rows.Scan(&candidate); err != nil {
			return "", false, err
		}
		candidate = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(candidate)), ".")
		if strings.HasSuffix(name, "."+candidate) && len(candidate) > len(zone) {
			zone = candidate
		}
	}
	if err := rows.Err(); err != nil {
		return "", false, err
	}
	if zone == "" {
		return "", false, nil
	}
	mode, err := p.domainDNSManagementMode(ctx, zone)
	if err != nil {
		return "", false, err
	}
	if mode != setupDNSModeLocal {
		return "", false, nil
	}
	var zoneID int
	err = p.db.GetDB().QueryRowContext(ctx, `SELECT id FROM pdns_domains WHERE name = ?`, zone).Scan(&zoneID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return zone, true, nil
}

// aliasDNSAddresses are the site addresses an alias is given: this server's
// addresses, as for a subdomain (never a guessed one).
func aliasDNSAddresses() ([]aliasDNSRecord, error) {
	var records []aliasDNSRecord
	if ipv4 := strings.TrimSpace(serverPrimaryIP()); ipv4 != "" {
		parsed := net.ParseIP(ipv4)
		if parsed == nil || parsed.To4() == nil {
			return nil, fmt.Errorf("server IPv4 address is invalid")
		}
		records = append(records, aliasDNSRecord{Type: "A", Content: parsed.To4().String()})
	}
	if ipv6 := strings.TrimSpace(serverPrimaryIPv6()); ipv6 != "" {
		parsed := net.ParseIP(ipv6)
		if parsed == nil || parsed.To4() != nil {
			return nil, fmt.Errorf("server IPv6 address is invalid")
		}
		records = append(records, aliasDNSRecord{Type: "AAAA", Content: parsed.String()})
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("server has no publishable address for the alias")
	}
	return records, nil
}

// publishAliasDNS gives alias of domainID its address records in the zone of
// its account it lies inside, and publishes the zone. It is run on a context
// the connection cannot cancel. Whatever it answers, the alias itself stands;
// the outcome is said to the person (the alias answer's "dns").
func (p *Panel) publishAliasDNS(ctx context.Context, domainID int, alias string) string {
	alias = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(alias)), ".")
	zone, ok, err := p.aliasDNSZone(ctx, domainID, alias)
	if err != nil {
		log.Printf("[alias-dns] %s: the zone to publish it in could not be read: %v", alias, err)
		return aliasDNSFailed
	}
	if !ok {
		return aliasDNSElsewhere
	}
	addresses, err := aliasDNSAddresses()
	if err != nil {
		log.Printf("[alias-dns] %s: %v", alias, err)
		return aliasDNSFailed
	}
	owner, ownerKnown := p.zoneAccount(ctx, zone)
	if !ownerKnown {
		log.Printf("[alias-dns] %s: the account of %s could not be read", alias, zone)
		return aliasDNSFailed
	}
	outcome, err := p.addAliasDNSRecords(ctx, zone, alias, addresses, owner)
	if err != nil {
		log.Printf("[alias-dns] %s in %s: %v", alias, zone, err)
		return outcome
	}
	if err := p.syncZoneToDNS(ctx, zone, false); err != nil {
		log.Printf("[alias-dns] %s: saved in %s, publication pending: %v", alias, zone, err)
		p.markAliasZoneUnpublished(ctx, zone)
		return aliasDNSPending
	}
	return aliasDNSPublished
}

// markAliasZoneUnpublished records that zone's alias records wait for their
// publication.
func (p *Panel) markAliasZoneUnpublished(ctx context.Context, zone string) {
	if err := p.setSetting(context.WithoutCancel(ctx), aliasDNSUnpublishedPrefix+strings.ToLower(zone), "1"); err != nil {
		log.Printf("[alias-dns] %s: the wait for publication could not be recorded: %v", zone, err)
	}
}

// clearAliasZoneWait (alpha.145, alpha.144 lab O1): the zone's publication
// completed (a held change published once the earlier publication
// completed), so its alias records that waited are published now; their
// rows stop saying they wait at once, not at the next hourly round.
func (p *Panel) clearAliasZoneWait(ctx context.Context, zone string) {
	result, err := p.db.GetDB().ExecContext(context.WithoutCancel(ctx), `DELETE FROM panel_settings WHERE key = ?`, aliasDNSUnpublishedPrefix+strings.ToLower(zone))
	if err != nil {
		log.Printf("[alias-dns] %s: published, but its wait could not be cleared (the hourly round clears it): %v", zone, err)
		return
	}
	if n, _ := result.RowsAffected(); n > 0 {
		log.Printf("[alias-dns] %s: the alias records that waited are published", zone)
	}
}

// aliasZoneWaiting reports whether zone's alias records wait for its
// publication (the zone's notice then says they are retried by themselves).
func (p *Panel) aliasZoneWaiting(ctx context.Context, zone string) bool {
	return p.setting(ctx, aliasDNSUnpublishedPrefix+strings.ToLower(zone)) != ""
}

// aliasZonesUnpublished lists the zones whose alias records wait.
func (p *Panel) aliasZonesUnpublished(ctx context.Context) (map[string]bool, error) {
	rows, err := p.db.GetDB().QueryContext(ctx, `SELECT key FROM panel_settings WHERE key LIKE ?`, aliasDNSUnpublishedPrefix+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	zones := map[string]bool{}
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		zones[strings.TrimPrefix(key, aliasDNSUnpublishedPrefix)] = true
	}
	return zones, rows.Err()
}

// republishAliasZones publishes every zone whose alias records wait (the
// Panel's hourly round); a zone that publishes is
// no longer marked. It never adds or removes a record.
func (p *Panel) republishAliasZones(ctx context.Context) {
	zones, err := p.aliasZonesUnpublished(ctx)
	if err != nil {
		log.Printf("[alias-dns] the zones that wait could not be read: %v", err)
		return
	}
	for marked := range zones {
		p.settleAliasZoneWait(ctx, marked, p.syncZoneToDNS)
	}
}

// settleAliasZoneWait is the hourly round's step for one marked zone: a zone
// no longer kept here loses its marker; otherwise the zone's current ledger
// is published (publish) and the marker goes only when that publication
// completes, so a record saved after an earlier publication is never left
// unpublished with its marker gone. alpha.145 (review of PR #357, finding 3):
// the same step runs for a zone whose pending publication just completed,
// with the publication locks held (publish = syncZoneToDNSLocked).
func (p *Panel) settleAliasZoneWait(ctx context.Context, marked string, publish func(context.Context, string, bool) error) {
	// The zone's own name as the ledger keeps it, compared without case
	// (the marker is kept in lower case).
	var zone string
	err := p.db.GetDB().QueryRowContext(ctx, `SELECT name FROM pdns_domains WHERE lower(name) = lower(?)`, marked).Scan(&zone)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return
	}
	mode := ""
	if err == nil {
		var modeErr error
		if mode, modeErr = p.domainDNSManagementMode(ctx, zone); modeErr != nil {
			// Not known whether the zone is still kept here: the marker
			// stays and the next round looks again (independent review
			// of the last commit, C3).
			log.Printf("[alias-dns] %s: its DNS mode could not be read (%v); its wait is kept for the next round", zone, modeErr)
			return
		}
	}
	if err != nil || mode != setupDNSModeLocal {
		// The zone went with its domain, or its DNS is no longer kept
		// here: nothing to publish, the marker goes (review of PR #355
		// fixes, finding 4).
		_, _ = p.db.GetDB().ExecContext(ctx, `DELETE FROM panel_settings WHERE key = ?`, aliasDNSUnpublishedPrefix+marked)
		log.Printf("[alias-dns] %s: no longer a zone kept here; its wait is cleared without publishing", marked)
		return
	}
	if err := publish(ctx, zone, false); err != nil {
		log.Printf("[alias-dns] %s: publication still waits: %v", zone, err)
		return
	}
	_, _ = p.db.GetDB().ExecContext(ctx, `DELETE FROM panel_settings WHERE key = ?`, aliasDNSUnpublishedPrefix+marked)
	log.Printf("[alias-dns] %s: the alias records that waited are published", zone)
}

// aliasesWaitingForPublication are the aliases among aliases whose records
// wait for their zone's publication.
func (p *Panel) aliasesWaitingForPublication(ctx context.Context, aliases []string) []string {
	zones, err := p.aliasZonesUnpublished(ctx)
	if err != nil || len(zones) == 0 {
		return nil
	}
	var waiting []string
	for _, alias := range aliases {
		raw := p.setting(ctx, aliasDNSKey(alias))
		var added aliasDNSAdded
		if raw != "" && json.Unmarshal([]byte(raw), &added) == nil && zones[strings.ToLower(added.Zone)] {
			waiting = append(waiting, alias)
		}
	}
	return waiting
}

// addAliasDNSRecords writes the records and the note of what was added in
// one transaction. A name that already has a different address record of
// the same type is the owner's: nothing is written (kept). A record that
// already exists with the same content is not CelikPanel's to note.
func (p *Panel) addAliasDNSRecords(ctx context.Context, zone, alias string, addresses []aliasDNSRecord, owner int) (string, error) {
	tx, err := p.db.GetDB().BeginTx(ctx, nil)
	if err != nil {
		return aliasDNSFailed, err
	}
	defer func() { _ = tx.Rollback() }()
	var zoneID int
	if err := tx.QueryRowContext(ctx, `SELECT id FROM pdns_domains WHERE name = ?`, zone).Scan(&zoneID); err != nil {
		return aliasDNSFailed, fmt.Errorf("find zone %s: %w", zone, err)
	}
	added := aliasDNSAdded{Version: 1, Zone: zone, Owner: owner}
	if raw, err := readSettingTx(ctx, tx, aliasDNSKey(alias)); err != nil {
		return aliasDNSFailed, err
	} else if raw != "" {
		var earlier aliasDNSAdded
		if json.Unmarshal([]byte(raw), &earlier) == nil && earlier.Version == 1 && earlier.Zone == zone {
			added.Records = earlier.Records
		}
	}
	var cname int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM pdns_records WHERE domain_id = ? AND name = ? AND type = 'CNAME'`, zoneID, alias).Scan(&cname); err != nil {
		return aliasDNSFailed, err
	}
	if cname > 0 {
		return aliasDNSKept, fmt.Errorf("%s already has a CNAME record in %s; it was left as it is", alias, zone)
	}
	// Any address record at the name that CelikPanel did not add is the
	// owner's: nothing is added beside it, of either type (review of PR
	// #355, finding 4).
	noted := map[aliasDNSRecord]bool{}
	for _, record := range added.Records {
		noted[record] = true
	}
	present := map[aliasDNSRecord]bool{}
	existing, err := tx.QueryContext(ctx, `SELECT type, content FROM pdns_records WHERE domain_id = ? AND name = ? AND type IN ('A', 'AAAA')`, zoneID, alias)
	if err != nil {
		return aliasDNSFailed, err
	}
	for existing.Next() {
		var record aliasDNSRecord
		if err := existing.Scan(&record.Type, &record.Content); err != nil {
			existing.Close()
			return aliasDNSFailed, err
		}
		if !noted[record] {
			existing.Close()
			return aliasDNSKept, fmt.Errorf("%s already has a %s record in %s that CelikPanel did not add; it was left as it is", alias, record.Type, zone)
		}
		present[record] = true
	}
	if err := existing.Close(); err != nil {
		return aliasDNSFailed, err
	}
	for _, address := range addresses {
		if present[address] {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO pdns_records (domain_id, name, type, content, ttl) VALUES (?, ?, ?, ?, 3600)`,
			zoneID, alias, address.Type, address.Content); err != nil {
			return aliasDNSFailed, fmt.Errorf("insert %s record: %w", address.Type, err)
		}
		added.Records = append(added.Records, address)
	}
	raw, err := json.Marshal(added)
	if err != nil {
		return aliasDNSFailed, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO panel_settings (key, value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP`, aliasDNSKey(alias), string(raw)); err != nil {
		return aliasDNSFailed, err
	}
	if err := tx.Commit(); err != nil {
		return aliasDNSFailed, err
	}
	return aliasDNSPublished, nil
}

// withdrawAliasDNS removes the records CelikPanel added for alias (and only
// those) and publishes their zone. skipZone, when set, is a zone that is
// being removed with its domain: its records go with it, only the note is
// removed. An error is returned only when nothing was removed (the note
// stays, so a later removal finds the records); a publication that does not
// finish after the removal is recorded as waiting (pending) and published by
// the hourly round, never an error (review of PR #355, finding 1).
// domainID is the alias's own domain (its row still exists when this runs).
func (p *Panel) withdrawAliasDNS(ctx context.Context, domainID int, alias, skipZone string) (bool, error) {
	alias = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(alias)), ".")
	var raw string
	err := p.db.GetDB().QueryRowContext(ctx, `SELECT value FROM panel_settings WHERE key = ?`, aliasDNSKey(alias)).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var added aliasDNSAdded
	if err := json.Unmarshal([]byte(raw), &added); err != nil || added.Version != 1 {
		// A note no release wrote names nothing it could remove: it goes,
		// and the records (if any) stay as the owner's.
		var partial struct {
			Zone string `json:"zone"`
		}
		_ = json.Unmarshal([]byte(raw), &partial)
		if partial.Zone == "" {
			partial.Zone = "an unknown zone"
		}
		log.Printf("[alias-dns] %s: the note of its records cannot be read and is removed; its records in %s, if any, are left (look for %s there)", alias, partial.Zone, alias)
		_, err := p.db.GetDB().ExecContext(ctx, `DELETE FROM panel_settings WHERE key = ?`, aliasDNSKey(alias))
		return false, err
	}
	if added.Zone == "" || strings.EqualFold(added.Zone, skipZone) {
		_, err := p.db.GetDB().ExecContext(ctx, `DELETE FROM panel_settings WHERE key = ?`, aliasDNSKey(alias))
		return false, err
	}
	// Only while the zone belongs, now, to the same account as the alias's
	// own domain now (independent review of the last commit, C1): after a
	// subscription transfer both moved together and the records go with
	// the alias; a zone deleted and made again by another account is that
	// account's, and nothing is removed there. The owner kept in the note
	// does not decide.
	zoneOwner, zoneKnown, err := p.accountHolding(ctx, zoneAccountQuery, added.Zone)
	if err != nil {
		return false, err
	}
	aliasOwner, aliasKnown, err := p.accountHolding(ctx, domainAccountQuery, domainID)
	if err != nil {
		return false, err
	}
	if !zoneKnown || !aliasKnown || zoneOwner != aliasOwner {
		log.Printf("[alias-dns] %s: %s is not held by the account of the alias's domain now; nothing is removed there and the note is cleared (look for %s there)", alias, added.Zone, alias)
		_, err := p.db.GetDB().ExecContext(ctx, `DELETE FROM panel_settings WHERE key = ?`, aliasDNSKey(alias))
		return false, err
	}
	tx, err := p.db.GetDB().BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	var zoneID int
	zoneErr := tx.QueryRowContext(ctx, `SELECT id FROM pdns_domains WHERE name = ?`, added.Zone).Scan(&zoneID)
	if zoneErr != nil && !errors.Is(zoneErr, sql.ErrNoRows) {
		return false, zoneErr
	}
	if zoneErr == nil {
		for _, record := range added.Records {
			if _, err := tx.ExecContext(ctx, `DELETE FROM pdns_records WHERE domain_id = ? AND name = ? AND type = ? AND content = ?`,
				zoneID, alias, record.Type, record.Content); err != nil {
				return false, err
			}
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM panel_settings WHERE key = ?`, aliasDNSKey(alias)); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	if zoneErr != nil || len(added.Records) == 0 {
		return false, nil
	}
	if err := p.syncZoneToDNS(ctx, added.Zone, false); err != nil {
		log.Printf("[alias-dns] the records of %s were removed from %s; its publication waits and is retried every hour: %v", alias, added.Zone, err)
		p.markAliasZoneUnpublished(ctx, added.Zone)
		return true, nil
	}
	return false, nil
}

// withdrawDomainAliasesDNS removes the alias records of a domain being
// deleted; ownZone is the domain's own zone, removed with it.
func (p *Panel) withdrawDomainAliasesDNS(ctx context.Context, domain, ownZone string) error {
	rows, err := p.db.GetDB().QueryContext(ctx, `
		SELECT a.domain_id, a.alias FROM domain_aliases a JOIN domains d ON d.id = a.domain_id WHERE d.name = ?`, domain)
	if err != nil {
		return err
	}
	type domainAlias struct {
		domainID int
		alias    string
	}
	var aliases []domainAlias
	for rows.Next() {
		var row domainAlias
		if err := rows.Scan(&row.domainID, &row.alias); err != nil {
			rows.Close()
			return err
		}
		aliases = append(aliases, row)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	var errs []error
	for _, row := range aliases {
		if _, err := p.withdrawAliasDNS(ctx, row.domainID, row.alias, ownZone); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// readSettingTx reads one panel setting inside tx ("" when absent).
func readSettingTx(ctx context.Context, tx *sql.Tx, key string) (string, error) {
	var value string
	err := tx.QueryRowContext(ctx, `SELECT value FROM panel_settings WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return value, err
}

// aliasDNSRemovalOutcome removes an alias's records after the alias went and
// says what became of them (the answer's "dns"): removed, pending (removed
// here, the publication waits and is retried every hour) or failed (nothing
// removed; the records stay until the next removal finds them).
func (p *Panel) aliasDNSRemovalOutcome(ctx context.Context, domainID int, alias string) string {
	pending, err := p.withdrawAliasDNS(ctx, domainID, alias, "")
	switch {
	case err != nil:
		log.Printf("[alias-dns] %s: its records could not be removed: %v", alias, err)
		return aliasDNSFailed
	case pending:
		return aliasDNSPending
	}
	return "removed"
}

// The account that holds a zone's top-level domain now (by name, without
// case), and the account that holds a domain now (by id).
const (
	zoneAccountQuery = `SELECT s.owner_id FROM domains d JOIN subscriptions s ON s.id = d.subscription_id
		WHERE lower(d.name) = lower(?) AND d.parent_domain_id IS NULL`
	domainAccountQuery = `SELECT s.owner_id FROM domains d JOIN subscriptions s ON s.id = d.subscription_id
		WHERE d.id = ?`
)

// accountHolding runs one of the queries above; known=false when no domain
// matches. A read error is returned as such (nothing is decided on it).
func (p *Panel) accountHolding(ctx context.Context, query string, arg any) (int, bool, error) {
	var owner int
	err := p.db.GetDB().QueryRowContext(ctx, query, arg).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return owner, true, nil
}

// zoneAccount is the account that holds the top-level domain whose zone is
// zone (compared without case); known=false when no domain holds it now.
func (p *Panel) zoneAccount(ctx context.Context, zone string) (int, bool) {
	owner, known, err := p.accountHolding(ctx, zoneAccountQuery, zone)
	return owner, known && err == nil
}
