package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"strings"
)

// Mail records of a domain whose zone is kept on the main DNS server
// (alpha.114 lab E6). A joined server publishes its domains' zones to the
// main DNS server over the authorized connection; until now only the web
// records went there, so a customer domain created on the joined server
// with mail had no MX, SPF, DMARC or DKIM anywhere and its DKIM signature
// could not be verified. The same records a domain gets in a zone this
// server keeps itself (dns_zone_template.go: MX to mail.<domain>, its
// address, SPF, DMARC; DKIM by the automatic DKIM work) are now added to
// the zone on the main DNS server through the same publication path.
//
// Additions only (D-022): a record already at one of these names (an MX,
// an address or CNAME at mail.<domain>, an SPF, DMARC or DKIM TXT) is the
// owner's and is kept, never replaced. Deleting the domain removes the zone
// with every record in it (syncRemoteDomainDNS); deleting a subdomain
// removes every record under its name.
// Ana DNS sunucusunda tutulan bolgeye posta kayitlari eklenir; sahibin
// kaydi degistirilmez. Alan adi silinince bolgeyle birlikte kalkar.

// remoteDomainMailRecords is the mail set of a top-level domain hosted
// here: MX to mail.<domain>, its addresses, the recommended SPF and DMARC.
func remoteDomainMailRecords(domain, ipv4, ipv6 string) []DNSRecord {
	records := externalMailDNSRecords(domain, ipv4, ipv6, mailAuthStatus{})
	return append(records,
		DNSRecord{Name: domain, Type: "TXT", Content: spfRecommended(), TTL: 3600},
		DNSRecord{Name: "_dmarc." + domain, Type: "TXT", Content: dmarcRecommended(domain, ""), TTL: 3600},
	)
}

func remoteDNSTXTValue(record DNSRecord) string {
	value, err := decodeDNSUserTXT(record.Content)
	if err != nil {
		return record.Content
	}
	return strings.TrimSpace(value)
}

// remoteDNSMissingMailRecords returns the desired records that have no
// counterpart yet, and the names where a different owner record was kept.
func remoteDNSMissingMailRecords(existing, desired []DNSRecord) (missing []DNSRecord, kept []string) {
	for _, want := range desired {
		present, differs := false, false
		for _, have := range existing {
			if have.Name != want.Name {
				continue
			}
			switch want.Type {
			case "TXT":
				family := remoteDNSMailTXTGroup(want)
				if have.Type == "TXT" && family != "" && remoteDNSMailTXTGroup(have) == family {
					present = true
					differs = differs || remoteDNSTXTValue(have) != remoteDNSTXTValue(want)
				}
			case "A", "AAAA":
				if have.Type == want.Type || have.Type == "CNAME" {
					present = true
					differs = differs || have.Type == "CNAME" || have.Content != want.Content
				}
			default:
				if have.Type == want.Type {
					present = true
					differs = differs || have.Content != want.Content || have.Prio != want.Prio
				}
			}
		}
		if !present {
			missing = append(missing, want)
		} else if differs {
			kept = append(kept, want.Name+" "+want.Type)
		}
	}
	return missing, kept
}

// remoteDNSAddMailRecords adds the desired mail records that are missing
// in the zone on the main DNS server and publishes it. It reports the owner
// records it kept. Nothing is sent when nothing is missing.
func (p *Panel) remoteDNSAddMailRecords(ctx context.Context, name string, desired []DNSRecord) ([]string, error) {
	canonical, err := canonicalRemoteDNSRecords(name, desired)
	if err != nil {
		return nil, err
	}
	var kept []string
	err = p.mutateRemoteDNSRecords(ctx, name, false, func(records []DNSRecord) ([]DNSRecord, error) {
		missing, differing := remoteDNSMissingMailRecords(records, canonical)
		kept = differing
		return append(records, missing...), nil
	})
	return kept, err
}

// remoteDomainIsApex: the domain owns its zone on the main DNS server (it is
// not a subdomain whose records live in its parent's zone).
func (p *Panel) remoteDomainIsApex(ctx context.Context, name string) (bool, error) {
	target, err := resolveRemoteDNSDomain(ctx, p.db.GetDB(), name)
	if err != nil {
		return false, err
	}
	return target.Domain == name, nil
}

// publishRemoteDomainMailOnCreate adds the mail set of a new top-level
// domain when this server runs mail (the same rule as the automatic DKIM
// key). It never fails the creation: a failure is logged, and the automatic
// DKIM work publishes the same set again with the DKIM record.
func (p *Panel) publishRemoteDomainMailOnCreate(ctx context.Context, domainID int, name string) {
	applies, err := domainHasProductMailRuntime(ctx, p.db.GetDB(), domainID)
	if err != nil || !applies {
		if err != nil {
			log.Printf("[remote-dns-mail] %s: read mail records: %v", name, err)
		}
		return
	}
	apex, err := p.remoteDomainIsApex(ctx, name)
	if err != nil || !apex {
		return
	}
	ipv4, ipv6 := serverPrimaryIP(), serverPrimaryIPv6()
	if ipv4 == "" && ipv6 == "" {
		log.Printf("[remote-dns-mail] %s: this server's address is not known; the mail records are not published", name)
		return
	}
	kept, err := p.remoteDNSAddMailRecords(ctx, name, remoteDomainMailRecords(name, ipv4, ipv6))
	if err != nil {
		log.Printf("[remote-dns-mail] %s: mail records not published on the main DNS server yet: %v", name, err)
		return
	}
	if len(kept) > 0 {
		log.Printf("[remote-dns-mail] %s: kept the owner's records at %s", name, strings.Join(kept, ", "))
	}
}

// automaticDKIMPublishRemote is the automatic DKIM work for a domain whose
// zone is on the main DNS server: the DKIM record, and for a top-level
// domain the rest of its mail set, are added there.
func (p *Panel) automaticDKIMPublishRemote(ctx context.Context, item dkimAutomaticItem) {
	record := item.record
	apex, err := p.remoteDomainIsApex(ctx, record.Domain)
	if errors.Is(err, sql.ErrNoRows) || p.automaticDKIMDomainGone(ctx, record) {
		p.stopAutomaticDKIMForGoneDomain(ctx, record)
		return
	}
	if err != nil {
		p.recordAutomaticDKIM(ctx, record, dkimAutoUnknown, dkimReasonDNSPublish, err.Error())
		return
	}
	name := dkimSelector + "._domainkey." + record.Domain
	desired := []DNSRecord{}
	if apex {
		if ipv4, ipv6 := serverPrimaryIP(), serverPrimaryIPv6(); ipv4 != "" || ipv6 != "" {
			desired = remoteDomainMailRecords(record.Domain, ipv4, ipv6)
		}
	}
	desired = append(desired, DNSRecord{Name: name, Type: "TXT", Content: dkimRecommended(item.pub), TTL: 3600})
	kept, err := p.remoteDNSAddMailRecords(ctx, record.Domain, desired)
	if err != nil {
		if p.automaticDKIMDomainGone(ctx, record) {
			p.stopAutomaticDKIMForGoneDomain(ctx, record)
			return
		}
		// Not published now (the main DNS server unreachable, or a saved
		// publication still being settled): the card offers Try again.
		p.recordAutomaticDKIM(ctx, record, dkimAutoUnknown, dkimReasonDNSPublish, "the record could not be published on the main DNS server now: "+err.Error())
		return
	}
	for _, item := range kept {
		if strings.HasPrefix(item, name+" ") {
			// Owner authority (D-022): a different DKIM record there is kept.
			p.finishAutomaticDKIM(ctx, record, "a different record at "+name+" on the main DNS server was kept")
			return
		}
	}
	p.finishAutomaticDKIM(ctx, record, "record published on the main DNS server")
}
