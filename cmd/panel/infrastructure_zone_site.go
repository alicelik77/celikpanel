package main

import (
	"context"
	"net"
	"strings"
)

// A website for the main domain when the main domain is already the
// infrastructure zone setup made (alpha.115, the first site at the end of
// setup). The zone is adopted, and only the site's own names that have no
// address yet get one: the apex and www, an A record to this server and,
// when it has a global IPv6 address, an AAAA record. A name that already has
// an address or an alias somewhere else (an old site the owner imported, a
// record the owner set) is left exactly as it is and reported, so the page
// can say whose decision it is (D-024); nothing is ever overwritten.
// Ana alan adi altyapi bolgesiyse: yalniz eksik @ ve www adresleri eklenir;
// baska yeri gosteren kayitlar degistirilmez, bildirilir.

type infrastructureSiteRecord struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Content  string `json:"content"`
	Expected string `json:"expected,omitempty"`
}

type infrastructureSiteRecords struct {
	Added []infrastructureSiteRecord `json:"added"`
	Kept  []infrastructureSiteRecord `json:"kept"`
	IPv4  string                     `json:"ipv4"`
	IPv6  string                     `json:"ipv6,omitempty"`
}

// addInfrastructureZoneSiteRecords adds the missing site addresses to an
// infrastructure zone; nil when the zone is not one (an ordinary zone is
// left to its template).
func (p *Panel) addInfrastructureZoneSiteRecords(ctx context.Context, zone string) (*infrastructureSiteRecords, error) {
	infrastructure, err := p.isInfrastructureDNSZone(ctx, zone)
	if err != nil || !infrastructure {
		return nil, err
	}
	result := &infrastructureSiteRecords{Added: []infrastructureSiteRecord{}, Kept: []infrastructureSiteRecord{}}
	if ip, ok := canonicalIPv4(serverPrimaryIP()); ok {
		result.IPv4 = ip
	}
	if parsed := net.ParseIP(serverPrimaryIPv6()); parsed != nil && parsed.To4() == nil && parsed.IsGlobalUnicast() && !parsed.IsPrivate() {
		result.IPv6 = parsed.String()
	}
	if result.IPv4 == "" {
		// Without this server's address nothing is guessed (as the template).
		return result, nil
	}
	ours := map[string]string{"A": result.IPv4, "AAAA": result.IPv6}
	tx, err := p.db.GetDB().BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var zoneID int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM pdns_domains WHERE name=?`, zone).Scan(&zoneID); err != nil {
		return nil, err
	}
	for _, name := range []string{zone, "www." + zone} {
		rows, err := tx.QueryContext(ctx, `SELECT type,content FROM pdns_records WHERE domain_id=? AND lower(name)=? AND type IN ('A','AAAA','CNAME') AND COALESCE(disabled,0)=0 ORDER BY id`, zoneID, name)
		if err != nil {
			return nil, err
		}
		present := map[string]bool{}
		elsewhere := []infrastructureSiteRecord{}
		for rows.Next() {
			var recordType, content string
			if err := rows.Scan(&recordType, &content); err != nil {
				rows.Close()
				return nil, err
			}
			recordType = strings.ToUpper(recordType)
			present[recordType] = true
			if expected := ours[recordType]; recordType == "CNAME" || expected == "" || !sameDNSRecordContent(recordType, content, expected) {
				elsewhere = append(elsewhere, infrastructureSiteRecord{Name: name, Type: recordType, Content: content, Expected: firstNonEmpty(ours[recordType], ours["A"])})
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
		if len(elsewhere) > 0 {
			result.Kept = append(result.Kept, elsewhere...)
			continue
		}
		for _, recordType := range []string{"A", "AAAA"} {
			if ours[recordType] == "" || present[recordType] {
				continue
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO pdns_records(domain_id,name,type,content,ttl) VALUES(?,?,?,?,3600)`, zoneID, name, recordType, ours[recordType]); err != nil {
				return nil, err
			}
			result.Added = append(result.Added, infrastructureSiteRecord{Name: name, Type: recordType, Content: ours[recordType]})
		}
	}
	return result, tx.Commit()
}
