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
	outcome, err := p.addAliasDNSRecords(ctx, zone, alias, addresses)
	if err != nil {
		log.Printf("[alias-dns] %s in %s: %v", alias, zone, err)
		return outcome
	}
	if err := p.syncZoneToDNS(ctx, zone, false); err != nil {
		log.Printf("[alias-dns] %s: saved in %s, publication pending: %v", alias, zone, err)
		return aliasDNSPending
	}
	return aliasDNSPublished
}

// addAliasDNSRecords writes the records and the note of what was added in
// one transaction. A name that already has a different address record of
// the same type is the owner's: nothing is written (kept). A record that
// already exists with the same content is not CelikPanel's to note.
func (p *Panel) addAliasDNSRecords(ctx context.Context, zone, alias string, addresses []aliasDNSRecord) (string, error) {
	tx, err := p.db.GetDB().BeginTx(ctx, nil)
	if err != nil {
		return aliasDNSFailed, err
	}
	defer func() { _ = tx.Rollback() }()
	var zoneID int
	if err := tx.QueryRowContext(ctx, `SELECT id FROM pdns_domains WHERE name = ?`, zone).Scan(&zoneID); err != nil {
		return aliasDNSFailed, fmt.Errorf("find zone %s: %w", zone, err)
	}
	added := aliasDNSAdded{Version: 1, Zone: zone}
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
	for _, address := range addresses {
		var total, exact int
		if err := tx.QueryRowContext(ctx, `
			SELECT COUNT(*), COALESCE(SUM(CASE WHEN content = ? THEN 1 ELSE 0 END), 0)
			FROM pdns_records WHERE domain_id = ? AND name = ? AND type = ?`,
			address.Content, zoneID, alias, address.Type).Scan(&total, &exact); err != nil {
			return aliasDNSFailed, err
		}
		if total > exact {
			return aliasDNSKept, fmt.Errorf("%s already has a different %s record in %s; it was left as it is", alias, address.Type, zone)
		}
		if exact > 0 {
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
// removed. An error leaves the note, so a later removal finds the records.
func (p *Panel) withdrawAliasDNS(ctx context.Context, alias, skipZone string) error {
	alias = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(alias)), ".")
	var raw string
	err := p.db.GetDB().QueryRowContext(ctx, `SELECT value FROM panel_settings WHERE key = ?`, aliasDNSKey(alias)).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var added aliasDNSAdded
	if err := json.Unmarshal([]byte(raw), &added); err != nil || added.Version != 1 {
		return fmt.Errorf("the note of %s's DNS records cannot be read", alias)
	}
	if added.Zone == "" || strings.EqualFold(added.Zone, skipZone) {
		_, err := p.db.GetDB().ExecContext(ctx, `DELETE FROM panel_settings WHERE key = ?`, aliasDNSKey(alias))
		return err
	}
	tx, err := p.db.GetDB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var zoneID int
	zoneErr := tx.QueryRowContext(ctx, `SELECT id FROM pdns_domains WHERE name = ?`, added.Zone).Scan(&zoneID)
	if zoneErr != nil && !errors.Is(zoneErr, sql.ErrNoRows) {
		return zoneErr
	}
	if zoneErr == nil {
		for _, record := range added.Records {
			if _, err := tx.ExecContext(ctx, `DELETE FROM pdns_records WHERE domain_id = ? AND name = ? AND type = ? AND content = ?`,
				zoneID, alias, record.Type, record.Content); err != nil {
				return err
			}
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM panel_settings WHERE key = ?`, aliasDNSKey(alias)); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if zoneErr != nil || len(added.Records) == 0 {
		return nil
	}
	if err := p.syncZoneToDNS(ctx, added.Zone, false); err != nil {
		return fmt.Errorf("the records of %s were removed from %s, but its publication is pending: %w", alias, added.Zone, err)
	}
	return nil
}

// withdrawDomainAliasesDNS removes the alias records of a domain being
// deleted; ownZone is the domain's own zone, removed with it.
func (p *Panel) withdrawDomainAliasesDNS(ctx context.Context, domain, ownZone string) error {
	rows, err := p.db.GetDB().QueryContext(ctx, `
		SELECT a.alias FROM domain_aliases a JOIN domains d ON d.id = a.domain_id WHERE d.name = ?`, domain)
	if err != nil {
		return err
	}
	var aliases []string
	for rows.Next() {
		var alias string
		if err := rows.Scan(&alias); err != nil {
			rows.Close()
			return err
		}
		aliases = append(aliases, alias)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	var errs []error
	for _, alias := range aliases {
		if err := p.withdrawAliasDNS(ctx, alias, ownZone); err != nil {
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
