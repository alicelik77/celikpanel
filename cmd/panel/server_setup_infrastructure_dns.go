package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/alicelik/celikpanel/internal/binddns"
	"github.com/alicelik/celikpanel/internal/hostname"
	"github.com/alicelik/celikpanel/internal/transport"
)

var errServerSetupInfrastructureDNSWaiting = errors.New("the reviewed native DNS pair must be ready before preparing access records")
var errServerSetupInfrastructureDNSUnknown = errors.New("the exact infrastructure DNS publication is being reconciled")
var errServerSetupInfrastructureDNSChanged = errors.New("infrastructure DNS ownership or records changed; review the plan again")

// These are infrastructure records only. A setup plan never creates a tenant,
// website, mailbox, MX, SPF or application apex record as a side effect.
type serverSetupInfrastructureDNSRecord struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Content string `json:"content"`
	TTL     int    `json:"ttl"`
	Action  string `json:"action"`
	// alpha.112: MX preference, and the origin of a record that is not one of
	// the setup's own server addresses: "imported" (the owner's existing
	// public record) or "mail" (the main domain's mail records). Such records
	// are the owner's in the zone and are never locked.
	Priority int    `json:"priority,omitempty"`
	Origin   string `json:"origin,omitempty"`
	// Replaces lists records the setup itself wrote earlier that this one
	// replaces on a revised plan (the merged SPF after a mail host change,
	// the old MX when mail moves now); the exact stored rows, so an owner's
	// change in between makes the plan stale instead of being overwritten.
	Replaces []serverSetupInfrastructureDNSReplaced `json:"replaces,omitempty"`
}

type serverSetupInfrastructureDNSReplaced struct {
	ID       int64  `json:"id"`
	Content  string `json:"content"`
	Priority int    `json:"priority,omitempty"`
}

// serverSetupInfrastructureDNSWritten is the setup's own record of which
// imported and mail records it wrote into the zone (panel_settings, no
// schema change): only those may be replaced on a revised plan; a different
// record the setup did not write is the owner's and is never overwritten.
// Kurulumun bolgeye kendi yazdigi kayitlarin listesi.
type serverSetupInfrastructureDNSWritten struct {
	ZoneID  int64                                       `json:"zone_id"`
	Records []serverSetupInfrastructureDNSWrittenRecord `json:"records"`
}

type serverSetupInfrastructureDNSWrittenRecord struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Content  string `json:"content"`
	Priority int    `json:"priority,omitempty"`
}

func serverSetupInfrastructureDNSWrittenKey(zone string) string {
	return "server_setup_infrastructure_written:" + zone
}

// serverSetupCanonicalStored is a stored record's content in the renderer's
// own form, so equal records compare equal however they were typed.
func serverSetupCanonicalStored(recordType, content string) string {
	if canonical, err := binddns.CanonicalRecordContent(recordType, content); err == nil {
		return canonical
	}
	return content
}

func (s serverSetupInfrastructureDNSSnapshot) setupWrote(got serverSetupInfrastructureDNSStoredRecord) bool {
	content := serverSetupCanonicalStored(got.Type, got.Content)
	for _, record := range s.Written {
		if record.Name == canonicalDNSName(got.Name) && record.Type == got.Type && record.Priority == got.Priority && serverSetupCanonicalStored(record.Type, record.Content) == content {
			return true
		}
	}
	return false
}

// setupWroteDesired: the setup wrote this exact record into the zone
// before (its own record of what it wrote).
func (s serverSetupInfrastructureDNSSnapshot) setupWroteDesired(want serverSetupInfrastructureDNSRecord) bool {
	content := serverSetupCanonicalStored(want.Type, want.Content)
	for _, record := range s.Written {
		if record.Name == want.Name && record.Type == want.Type && record.Priority == want.Priority && serverSetupCanonicalStored(record.Type, record.Content) == content {
			return true
		}
	}
	return false
}

type serverSetupInfrastructureDNSPlan struct {
	ZoneType       string                               `json:"zone_type"`
	Zone           string                               `json:"zone"`
	ExistingZoneID int64                                `json:"existing_zone_id,omitempty"`
	ExpectedDigest string                               `json:"expected_digest"`
	Records        []serverSetupInfrastructureDNSRecord `json:"records"`
}

type serverSetupInfrastructureDNSPlanError struct {
	Code    string
	Message string
}

func (e *serverSetupInfrastructureDNSPlanError) Error() string { return e.Message }
func infrastructureDNSReviewError(message string) error {
	return &serverSetupInfrastructureDNSPlanError{Code: "infrastructure_dns_conflict", Message: message}
}

type serverSetupInfrastructureDNSQuery interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

type serverSetupInfrastructureDNSOwner struct {
	Zone   string `json:"zone"`
	ZoneID int64  `json:"zone_id"`
	PlanID string `json:"plan_id"`
}

type serverSetupInfrastructureDNSReceipt struct {
	PlanID             string              `json:"plan_id"`
	StepID             string              `json:"step_id"`
	RequestID          string              `json:"request_id"`
	OwnerID            string              `json:"owner_id"`
	Zone               string              `json:"zone"`
	ZoneID             int64               `json:"zone_id"`
	Digest             string              `json:"digest"`
	Engine             transport.DNSEngine `json:"engine"`
	Epoch              int64               `json:"epoch"`
	PreparedGeneration int64               `json:"prepared_generation"`
	Failed             bool                `json:"failed,omitempty"`
}

type serverSetupInfrastructureDNSSnapshot struct {
	Zone      string
	ZoneID    int64
	ZoneType  string
	Master    string
	Ownership string
	Records   []serverSetupInfrastructureDNSStoredRecord
	Written   []serverSetupInfrastructureDNSWrittenRecord
}

type serverSetupInfrastructureDNSStoredRecord struct {
	ID        int64
	Name      string
	Type      string
	Content   string
	TTL       int
	Priority  int
	Disabled  int
	Auth      int
	OrderName string
}

func serverSetupInfrastructureDNSOwnerKey(zone string) string {
	return "server_setup_infrastructure_zone:" + zone
}
func serverSetupInfrastructureDNSReceiptKey(requestID string) string {
	return "server_setup_infrastructure_dns:" + requestID
}
func serverSetupDNSNameWithin(name, zone string) bool {
	return name == zone || strings.HasSuffix(name, "."+zone)
}

func readServerSetupInfrastructureDNSSnapshot(ctx context.Context, q serverSetupInfrastructureDNSQuery, zone string) (serverSetupInfrastructureDNSSnapshot, error) {
	s := serverSetupInfrastructureDNSSnapshot{Zone: zone, Records: []serverSetupInfrastructureDNSStoredRecord{}}
	// Creating an ancestor/child authority is a separate delegation change. This
	// bounded bootstrap helper must not infer permission to shadow another zone.
	rows, err := q.QueryContext(ctx, `SELECT name FROM pdns_domains WHERE name<>? AND (? LIKE '%.'||name OR name LIKE '%.'||?) LIMIT 1`, zone, zone, zone)
	if err != nil {
		return s, err
	}
	overlap := rows.Next()
	rowErr := rows.Err()
	rows.Close()
	if rowErr != nil {
		return s, rowErr
	}
	if overlap {
		return s, infrastructureDNSReviewError("the infrastructure zone overlaps another managed DNS zone; review its delegation first")
	}
	rows, err = q.QueryContext(ctx, `SELECT id,subscription_id,name,dns_management FROM domains WHERE name=? OR ? LIKE '%.'||name LIMIT 129`, zone, zone)
	if err != nil {
		return s, err
	}
	count := 0
	for rows.Next() {
		var name, mode string
		var domainID, subscriptionID int64
		if err := rows.Scan(&domainID, &subscriptionID, &name, &mode); err != nil {
			rows.Close()
			return s, err
		}
		count++
		if mode != setupDNSModeLocal {
			rows.Close()
			return s, infrastructureDNSReviewError("the infrastructure zone belongs to external or remote DNS; preserve that ownership")
		}
		if name == zone {
			s.Ownership = fmt.Sprintf("local-domain:%d:%d", domainID, subscriptionID)
		}
	}
	rowErr = rows.Err()
	rows.Close()
	if rowErr != nil {
		return s, rowErr
	}
	if count > 128 {
		return s, infrastructureDNSReviewError("too many overlapping DNS ownership entries")
	}
	err = q.QueryRowContext(ctx, `SELECT id,type,COALESCE(master,'') FROM pdns_domains WHERE name=?`, zone).Scan(&s.ZoneID, &s.ZoneType, &s.Master)
	if errors.Is(err, sql.ErrNoRows) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if s.Master != "" || !slices.Contains([]string{"MASTER", "NATIVE"}, s.ZoneType) {
		return s, infrastructureDNSReviewError("the infrastructure zone is not a local primary zone")
	}
	if s.Ownership == "" {
		var raw string
		err = q.QueryRowContext(ctx, `SELECT value FROM panel_settings WHERE key=?`, serverSetupInfrastructureDNSOwnerKey(zone)).Scan(&raw)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return s, err
		}
		var owner serverSetupInfrastructureDNSOwner
		if err != nil || json.Unmarshal([]byte(raw), &owner) != nil || owner.Zone != zone || owner.ZoneID != s.ZoneID || !validServiceOperationID(owner.PlanID) {
			return s, infrastructureDNSReviewError("the existing DNS zone has no verified local ownership; reconcile it before setup")
		}
		s.Ownership = raw
	}
	var signed int
	if err := q.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM pdns_cryptokeys WHERE domain_id=?) + (SELECT COUNT(*) FROM pdns_domainmetadata WHERE domain_id=? AND kind IN ('PRESIGNED','NSEC3PARAM')) + (SELECT COUNT(*) FROM pdns_records WHERE domain_id=? AND type IN ('DNSKEY','RRSIG','NSEC','NSEC3','NSEC3PARAM'))`, s.ZoneID, s.ZoneID, s.ZoneID).Scan(&signed); err != nil {
		return s, err
	}
	if signed != 0 {
		return s, infrastructureDNSReviewError("existing infrastructure DNSSEC must be managed through its signing workflow; setup will preserve it")
	}
	rows, err = q.QueryContext(ctx, `SELECT id,COALESCE(name,''),COALESCE(type,''),COALESCE(content,''),COALESCE(ttl,3600),COALESCE(prio,0),COALESCE(disabled,0),COALESCE(auth,1),COALESCE(ordername,'') FROM pdns_records WHERE domain_id=? ORDER BY id LIMIT 2049`, s.ZoneID)
	if err != nil {
		return s, err
	}
	defer rows.Close()
	for rows.Next() {
		var r serverSetupInfrastructureDNSStoredRecord
		if err := rows.Scan(&r.ID, &r.Name, &r.Type, &r.Content, &r.TTL, &r.Priority, &r.Disabled, &r.Auth, &r.OrderName); err != nil {
			return s, err
		}
		s.Records = append(s.Records, r)
	}
	if err := rows.Err(); err != nil {
		return s, err
	}
	if len(s.Records) > 2048 {
		return s, infrastructureDNSReviewError("infrastructure zone exceeds the bounded setup record limit")
	}
	var raw string
	err = q.QueryRowContext(ctx, `SELECT value FROM panel_settings WHERE key=?`, serverSetupInfrastructureDNSWrittenKey(zone)).Scan(&raw)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return s, err
	}
	var written serverSetupInfrastructureDNSWritten
	if err == nil && json.Unmarshal([]byte(raw), &written) == nil && written.ZoneID == s.ZoneID {
		s.Written = written.Records
	}
	return s, nil
}

func serverSetupInfrastructureDNSSnapshotDigest(s serverSetupInfrastructureDNSSnapshot, allowSerialAdvance bool) string {
	if allowSerialAdvance {
		s.Records = append([]serverSetupInfrastructureDNSStoredRecord(nil), s.Records...)
		for i := range s.Records {
			if s.Records[i].Type == "SOA" {
				fields := strings.Fields(s.Records[i].Content)
				if len(fields) == 7 {
					fields[2] = "0"
					s.Records[i].Content = strings.Join(fields, " ")
				}
			}
		}
	}
	raw, _ := json.Marshal(s)
	return serverSetupID(string(raw))
}

func serverSetupInfrastructureDNSDesired(d serverSetupDraft) ([]serverSetupInfrastructureDNSRecord, error) {
	if d.InfrastructureDNS == nil {
		return nil, nil
	}
	if d.InfrastructureDNS.Zone == "" {
		return nil, infrastructureDNSReviewError("choose the exact infrastructure DNS zone before reviewing setup")
	}
	if d.DNSMode != setupDNSModeLocal || d.DNSRole != transport.DNSPairRolePrimary {
		return nil, infrastructureDNSReviewError("only the reviewed local primary may prepare infrastructure DNS records")
	}
	zone, err := hostname.CanonicalFQDN(d.InfrastructureDNS.Zone)
	if err != nil || zone != d.InfrastructureDNS.Zone {
		return nil, infrastructureDNSReviewError("choose the exact fully qualified infrastructure DNS zone")
	}
	if _, _, err := setupDNSIdentity(d); err != nil {
		return nil, infrastructureDNSReviewError(err.Error())
	}
	if d.PanelDomain == "" || !serverSetupDNSNameWithin(d.PanelDomain, zone) {
		return nil, infrastructureDNSReviewError("the reviewed panel hostname must belong to the selected infrastructure zone")
	}
	records := []serverSetupInfrastructureDNSRecord{
		{Name: zone, Type: "SOA", Content: fmt.Sprintf("%s hostmaster.%s 1 3600 600 604800 300", d.NS1, zone), TTL: 3600, Action: "add"},
		{Name: zone, Type: "NS", Content: d.NS1, TTL: 3600, Action: "add"},
		{Name: zone, Type: "NS", Content: d.NS2, TTL: 3600, Action: "add"},
	}
	addAddress := func(name, ip string) error {
		canon, err := hostname.CanonicalFQDN(name)
		if err != nil || canon != name || !serverSetupDNSNameWithin(name, zone) {
			return infrastructureDNSReviewError("every prepared access hostname must belong to the reviewed infrastructure zone")
		}
		for _, r := range records {
			if r.Name == name && r.Type == "A" {
				if r.Content != ip {
					return infrastructureDNSReviewError("the same access hostname cannot point to both servers")
				}
				return nil
			}
		}
		records = append(records, serverSetupInfrastructureDNSRecord{Name: name, Type: "A", Content: ip, TTL: 300, Action: "add"})
		return nil
	}
	for _, ns := range []string{d.NS1, d.NS2} {
		if serverSetupDNSNameWithin(ns, zone) {
			ip := d.LocalIP
			if ns == d.PeerNS {
				ip = d.PeerIP
			}
			if err := addAddress(ns, ip); err != nil {
				return nil, err
			}
		}
	}
	if err := addAddress(d.PanelDomain, d.LocalIP); err != nil {
		return nil, err
	}
	if serverSetupHasComponent(d, "postfix") && d.MailHostname != "" {
		if err := addAddress(d.MailHostname, d.LocalIP); err != nil {
			return nil, err
		}
	}
	if d.InfrastructureDNS.PeerPanelDomain != "" {
		if err := addAddress(d.InfrastructureDNS.PeerPanelDomain, d.PeerIP); err != nil {
			return nil, err
		}
	}
	own := map[string]bool{zone: false}
	for _, r := range records {
		if r.Type == "A" {
			own[r.Name] = true
		}
	}
	// The peer's mail host name (2026-10-16): optional, inside the zone, never
	// one of this server's own names, and an A record to the reviewed peer
	// address like the peer's panel host name.
	// Esin posta adi: istege bagli, bolgenin icinde, bu sunucunun kendi adi
	// degil; esin panel adi gibi esin adresine A kaydi.
	if peerMail := d.InfrastructureDNS.PeerMailHostname; peerMail != "" {
		if peerMail == zone || peerMail == d.PanelDomain || peerMail == d.MailHostname || peerMail == d.NS1 || peerMail == d.NS2 {
			return nil, &serverSetupInfrastructureDNSPlanError{Code: "infrastructure_dns_peer_mail_own_name", Message: "the other server's mail host name must not be this server's own panel, mail or nameserver name or the zone apex"}
		}
		if !serverSetupDNSNameWithin(peerMail, zone) {
			return nil, &serverSetupInfrastructureDNSPlanError{Code: "infrastructure_dns_peer_mail_outside_zone", Message: "the other server's mail host name must belong to the selected infrastructure zone"}
		}
		if err := addAddress(peerMail, d.PeerIP); err != nil {
			return nil, err
		}
		own[peerMail] = true
	}
	if d.MainDomain == zone {
		extra, err := serverSetupImportedAndMailDesired(d, own)
		if err != nil {
			return nil, err
		}
		records = append(records, extra...)
	}
	// A CNAME owns its name alone (RFC 1034): any other record at the same
	// name is refused before anything is written.
	for _, alias := range records {
		if alias.Type != "CNAME" {
			continue
		}
		for _, other := range records {
			if other.Name == alias.Name && other.Type != "CNAME" {
				return nil, &serverSetupInfrastructureDNSPlanError{Code: "infrastructure_dns_imported_clash", Message: fmt.Sprintf("%s is an alias (CNAME) and cannot also have a %s record; skip one of them", alias.Name, other.Type)}
			}
		}
	}
	return records, nil
}

// serverSetupOnePerName names a record kind that may exist once at its name:
// an SPF or DMARC policy, and the mail record's MX when the owner moves
// mail now. Two of them would break mail (SPF permerror) or leave the old
// server receiving it.
func serverSetupOnePerName(record serverSetupInfrastructureDNSRecord, zone string) string {
	value := strings.ToLower(joinTXTSegments(record.Content))
	switch {
	case record.Type == "TXT" && record.Name == zone && strings.HasPrefix(value, "v=spf1"):
		return "spf"
	case record.Type == "TXT" && strings.HasPrefix(record.Name, "_dmarc.") && strings.HasPrefix(value, "v=dmarc1"):
		return "dmarc"
	case record.Type == "MX" && record.Origin == "mail":
		return "mx"
	}
	return ""
}

func buildServerSetupInfrastructureDNSAgainstSnapshot(d serverSetupDraft, s serverSetupInfrastructureDNSSnapshot) (*serverSetupInfrastructureDNSPlan, error) {
	desired, err := serverSetupInfrastructureDNSDesired(d)
	if err != nil || desired == nil {
		return nil, err
	}
	role := d.DNSRole
	if serverSetupStandaloneDNS(d) {
		role = transport.DNSTopologyStandalone
	}
	plan := &serverSetupInfrastructureDNSPlan{Zone: s.Zone, ZoneType: dnsLocalZoneType(role), ExistingZoneID: s.ZoneID, ExpectedDigest: serverSetupInfrastructureDNSSnapshotDigest(s, false), Records: desired}
	for i := range plan.Records {
		want := &plan.Records[i]
		seen := 0
		for _, got := range s.Records {
			name := canonicalDNSName(got.Name)
			// Neither an alias nor a delegated child may be overwritten by an A record.
			if got.Disabled == 0 && ((got.Type == "DNAME" && serverSetupDNSNameWithin(want.Name, name)) || (got.Type == "NS" && name != s.Zone && serverSetupDNSNameWithin(want.Name, name)) || (got.Type == "CNAME" && name == want.Name && !(want.Type == "CNAME" && canonicalDNSName(got.Content) == want.Content))) {
				return nil, infrastructureDNSReviewError(fmt.Sprintf("infrastructure hostname %s conflicts with an alias or delegated child", want.Name))
			}
			if want.Origin != "" && got.Disabled == 0 && name == want.Name && want.Type == "CNAME" && got.Type != "CNAME" {
				return nil, &serverSetupInfrastructureDNSPlanError{Code: "infrastructure_dns_imported_clash", Message: fmt.Sprintf("%s already has a %s record in the zone and cannot also be an alias (CNAME)", want.Name, got.Type)}
			}
			if name != want.Name || got.Type != want.Type {
				continue
			}
			if got.Disabled != 0 {
				return nil, infrastructureDNSReviewError(fmt.Sprintf("infrastructure record %s %s is disabled; review it first", want.Name, want.Type))
			}
			if want.Origin != "" {
				// The owner's records are a set: an identical record (in the
				// renderer's own form) is kept, anything else is added beside
				// what the zone already has. For a once-per-name record (SPF,
				// DMARC, the mail MX) a different one the setup wrote earlier
				// is replaced; one the setup did not write is the owner's: a
				// second SPF or DMARC is refused, another MX (a backup the
				// owner added in Domains) stays beside ours.
				if serverSetupCanonicalStored(got.Type, got.Content) == want.Content && got.Priority == want.Priority {
					want.TTL, want.Action = got.TTL, "keep"
					continue
				}
				kind := serverSetupOnePerName(*want, s.Zone)
				if kind == "" || (kind != "mx" && serverSetupOnePerName(serverSetupInfrastructureDNSRecord{Name: name, Type: got.Type, Content: got.Content}, s.Zone) != kind) {
					continue
				}
				if s.setupWrote(got) {
					want.Replaces = append(want.Replaces, serverSetupInfrastructureDNSReplaced{ID: got.ID, Content: got.Content, Priority: got.Priority})
					continue
				}
				if kind == "mx" {
					continue
				}
				return nil, &serverSetupInfrastructureDNSPlanError{Code: "infrastructure_dns_imported_clash", Message: fmt.Sprintf("the zone already has a different %s record for %s that setup did not write; change or remove it in Domains first", strings.ToUpper(kind), want.Name)}
			}
			if want.Type == "SOA" {
				seen++
				if len(strings.Fields(got.Content)) != 7 || seen > 1 {
					return nil, infrastructureDNSReviewError("existing infrastructure SOA is ambiguous or invalid")
				}
				want.Content = got.Content
				want.TTL = got.TTL
				want.Action = "keep"
			} else if want.Type == "A" {
				if got.Content != want.Content {
					return nil, infrastructureDNSReviewError(fmt.Sprintf("infrastructure address for %s differs from the reviewed server address", want.Name))
				}
				want.TTL = got.TTL
				want.Action = "keep"
			} else if want.Type == "NS" {
				content := canonicalDNSName(got.Content)
				if content != d.NS1 && content != d.NS2 {
					return nil, infrastructureDNSReviewError("existing nameserver records differ from the reviewed DNS pair")
				}
				if content == want.Content {
					want.TTL = got.TTL
					want.Action = "keep"
				}
			}
		}
	}
	// The owner's deletions and changes win (alpha.115): an imported or
	// mail record the setup wrote before and that is no longer in the zone
	// was removed or changed by the owner (the record editor, "Move mail
	// to ..."), and a later plan or extension never adds it back.
	kept := plan.Records[:0]
	for _, want := range plan.Records {
		if want.Origin != "" && want.Action == "add" && len(want.Replaces) == 0 && s.setupWroteDesired(want) {
			continue
		}
		kept = append(kept, want)
	}
	plan.Records = kept
	return plan, nil
}

func (p *Panel) buildServerSetupInfrastructureDNSPlan(ctx context.Context, d serverSetupDraft) (*serverSetupInfrastructureDNSPlan, error) {
	desired, err := serverSetupInfrastructureDNSDesired(d)
	if err != nil || desired == nil {
		return nil, err
	}
	tx, err := p.db.GetDB().BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	snapshot, err := readServerSetupInfrastructureDNSSnapshot(ctx, tx, d.InfrastructureDNS.Zone)
	if err != nil {
		return nil, err
	}
	return buildServerSetupInfrastructureDNSAgainstSnapshot(d, snapshot)
}

// prepareServerSetupInfrastructureDNSRecords commits additions and their owner
// receipt together. A retry reads the receipt and compares every existing record;
// it cannot turn an owner's changed address back into the old reviewed address.
func (p *Panel) prepareServerSetupInfrastructureDNSRecords(ctx context.Context, plan serverSetupPlan, step serverSetupExecutionStep, publisher dnsPublisherIdentity) (serverSetupInfrastructureDNSReceipt, error) {
	var receipt serverSetupInfrastructureDNSReceipt
	tx, err := p.db.GetDB().BeginTx(ctx, nil)
	if err != nil {
		return receipt, err
	}
	defer tx.Rollback()
	s, err := readServerSetupInfrastructureDNSSnapshot(ctx, tx, plan.InfrastructureDNS.Zone)
	if err != nil {
		return receipt, err
	}
	current, err := buildServerSetupInfrastructureDNSAgainstSnapshot(plan.Draft, s)
	if err != nil {
		return receipt, err
	}
	expectedJSON, _ := json.Marshal(plan.InfrastructureDNS)
	currentJSON, _ := json.Marshal(current)
	if string(expectedJSON) != string(currentJSON) {
		return receipt, errServerSetupInfrastructureDNSChanged
	}
	zoneID := s.ZoneID
	if zoneID == 0 {
		result, err := tx.ExecContext(ctx, `INSERT INTO pdns_domains(name,type) VALUES (?,'MASTER')`, s.Zone)
		if err != nil {
			return receipt, err
		}
		zoneID, err = result.LastInsertId()
		if err != nil {
			return receipt, err
		}
		owner, _ := json.Marshal(serverSetupInfrastructureDNSOwner{Zone: s.Zone, ZoneID: zoneID, PlanID: plan.ID})
		if _, err := tx.ExecContext(ctx, `INSERT INTO panel_settings(key,value) VALUES(?,?)`, serverSetupInfrastructureDNSOwnerKey(s.Zone), string(owner)); err != nil {
			return receipt, err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE pdns_domains SET type=? WHERE id=? AND type<>?`, current.ZoneType, zoneID, current.ZoneType); err != nil {
		return receipt, err
	}
	written := append([]serverSetupInfrastructureDNSWrittenRecord(nil), s.Written...)
	for _, r := range current.Records {
		for _, old := range r.Replaces {
			result, err := tx.ExecContext(ctx, `DELETE FROM pdns_records WHERE id=? AND domain_id=? AND content=? AND COALESCE(prio,0)=?`, old.ID, zoneID, old.Content, old.Priority)
			if err != nil {
				return receipt, err
			}
			if changed, err := result.RowsAffected(); err != nil || changed != 1 {
				return receipt, errServerSetupInfrastructureDNSChanged
			}
			content := serverSetupCanonicalStored(r.Type, old.Content)
			written = slices.DeleteFunc(written, func(record serverSetupInfrastructureDNSWrittenRecord) bool {
				return record.Name == r.Name && record.Type == r.Type && record.Priority == old.Priority && serverSetupCanonicalStored(record.Type, record.Content) == content
			})
		}
		if r.Action != "add" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO pdns_records(domain_id,name,type,content,ttl,prio) VALUES(?,?,?,?,?,?)`, zoneID, r.Name, r.Type, r.Content, r.TTL, r.Priority); err != nil {
			return receipt, err
		}
		if r.Origin != "" {
			written = append(written, serverSetupInfrastructureDNSWrittenRecord{Name: r.Name, Type: r.Type, Content: r.Content, Priority: r.Priority})
		}
	}
	if len(written) > 0 || len(s.Written) > 0 {
		raw, _ := json.Marshal(serverSetupInfrastructureDNSWritten{ZoneID: zoneID, Records: written})
		if _, err := tx.ExecContext(ctx, `INSERT INTO panel_settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, serverSetupInfrastructureDNSWrittenKey(s.Zone), string(raw)); err != nil {
			return receipt, err
		}
	}
	s, err = readServerSetupInfrastructureDNSSnapshot(ctx, tx, s.Zone)
	if err != nil {
		return receipt, err
	}
	syncState, err := readDNSZoneSyncState(ctx, tx, s.Zone)
	if err != nil {
		return receipt, err
	}
	receipt = serverSetupInfrastructureDNSReceipt{PreparedGeneration: syncState.DesiredGeneration, PlanID: plan.ID, StepID: step.ID, RequestID: step.RequestID, OwnerID: step.OwnerID, Zone: s.Zone, ZoneID: zoneID, Digest: serverSetupInfrastructureDNSSnapshotDigest(s, true), Engine: publisher.Engine, Epoch: publisher.Epoch}
	raw, _ := json.Marshal(receipt)
	if _, err := tx.ExecContext(ctx, `INSERT INTO panel_settings(key,value) VALUES(?,?)`, serverSetupInfrastructureDNSReceiptKey(step.RequestID), string(raw)); err != nil {
		return receipt, err
	}
	return receipt, tx.Commit()
}

func (p *Panel) infrastructureDNSReceiptApplied(ctx context.Context, r serverSetupInfrastructureDNSReceipt) (bool, error) {
	var count int
	err := p.db.GetDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM dns_zone_sync_state s JOIN dns_zone_engine_applications a ON a.zone_name=s.zone_name WHERE s.zone_name=? AND s.source_domain_id=? AND s.status='applied' AND s.desired_action='sync' AND s.desired_generation=s.applied_generation AND a.engine=? AND a.engine_epoch=? AND a.applied_generation=s.desired_generation AND a.applied_action='sync'`, r.Zone, r.ZoneID, r.Engine, r.Epoch).Scan(&count)
	return count == 1, err
}

func (p *Panel) runServerSetupInfrastructureDNS(ctx context.Context, plan serverSetupPlan, step serverSetupExecutionStep) (bool, error) {
	if err := p.requireServerSetupAdmission(ctx); err != nil {
		return false, err
	}
	if plan.InfrastructureDNS == nil || plan.Draft.InfrastructureDNS == nil || plan.Draft.DNSMode != setupDNSModeLocal || plan.Draft.DNSRole != transport.DNSPairRolePrimary || plan.InfrastructureDNS.Zone != plan.Draft.InfrastructureDNS.Zone || step.Kind != "infrastructure_dns" || step.Target != plan.InfrastructureDNS.Zone || (plan.ID != serverSetupPlanIdentity(plan) && !plan.peerBound) || !validServiceOperationID(step.RequestID) || !validServiceOperationID(step.OwnerID) {
		return false, errors.New("infrastructure DNS step does not match its immutable reviewed plan")
	}
	stepFound := false
	for _, candidate := range plan.Steps {
		if candidate == step.serverSetupPlanStep {
			stepFound = true
		}
	}
	if !stepFound {
		return false, errors.New("infrastructure DNS step is not in the reviewed plan")
	}
	p.serviceMutationMu.Lock()
	defer p.serviceMutationMu.Unlock()
	dnsPublicationMu.Lock()
	defer dnsPublicationMu.Unlock()
	request, local, err := setupDNSIdentity(plan.Draft)
	if err != nil {
		return false, err
	}
	state, err := readDNSEngineDBState(ctx, p.db.GetDB())
	if err != nil {
		return false, err
	}
	if !setupDNSDraftMatchesState(plan.Draft, request, local, state) {
		return false, errServerSetupInfrastructureDNSChanged
	}
	if err := p.requireNoPendingDNSClusterSaga(ctx); err != nil {
		return false, errServerSetupInfrastructureDNSWaiting
	}
	var raw string
	err = p.db.GetDB().QueryRowContext(ctx, `SELECT value FROM panel_settings WHERE key=?`, serverSetupInfrastructureDNSReceiptKey(step.RequestID)).Scan(&raw)
	var receipt serverSetupInfrastructureDNSReceipt
	if err == nil {
		if json.Unmarshal([]byte(raw), &receipt) != nil || receipt.PlanID != plan.ID || receipt.StepID != step.ID || receipt.RequestID != step.RequestID || receipt.OwnerID != step.OwnerID || receipt.Zone != plan.InfrastructureDNS.Zone || receipt.Engine != state.ActiveEngine || receipt.Epoch != state.EngineEpoch {
			return false, errServerSetupInfrastructureDNSChanged
		}
		s, err := readServerSetupInfrastructureDNSSnapshot(ctx, p.db.GetDB(), receipt.Zone)
		if err != nil {
			return false, err
		}
		if s.ZoneID != receipt.ZoneID || serverSetupInfrastructureDNSSnapshotDigest(s, true) != receipt.Digest {
			return false, errServerSetupInfrastructureDNSChanged
		}
		if applied, err := p.infrastructureDNSReceiptApplied(ctx, receipt); err != nil || applied {
			return applied, err
		}
		syncState, stateErr := readDNSZoneSyncState(ctx, p.db.GetDB(), receipt.Zone)
		if stateErr != nil {
			return false, errServerSetupInfrastructureDNSUnknown
		}
		// A publication refused only because the host was busy is not a
		// failure: it is published again below (run 5, D10).
		if receipt.Failed || (syncState.Status == "error" && syncState.DesiredGeneration > receipt.PreparedGeneration && !dnsZoneSyncStateRefusedBusy(syncState)) {
			return false, errors.New("infrastructure DNS publication failed; resolve the DNS operation and review a revised plan")
		}
	} else if errors.Is(err, sql.ErrNoRows) {
		if plan.BuildCommit != strings.TrimSpace(buildCommit) {
			return false, errServerSetupBuildChanged
		}
		// Do not prepare records behind an older unresolved publication identity.
		if previous, stateErr := readDNSZoneSyncState(ctx, p.db.GetDB(), plan.InfrastructureDNS.Zone); stateErr == nil {
			if previous.hasLease() {
				return false, errServerSetupInfrastructureDNSUnknown
			}
		} else if !errors.Is(stateErr, sql.ErrNoRows) {
			return false, stateErr
		}
		if _, err := readDNSZoneEngineLease(ctx, p.db.GetDB(), plan.InfrastructureDNS.Zone); err == nil {
			return false, errServerSetupInfrastructureDNSUnknown
		} else if !errors.Is(err, sql.ErrNoRows) {
			return false, err
		}
		publisher, ready, err := p.activeDNSPublisher(ctx)
		if err != nil {
			return false, errServerSetupInfrastructureDNSUnknown
		}
		if !ready || (publisher.PairRole != transport.DNSPairRolePrimary && !(serverSetupStandaloneDNS(plan.Draft) && publisher.PairRole == "")) {
			return false, errServerSetupInfrastructureDNSWaiting
		}
		receipt, err = p.prepareServerSetupInfrastructureDNSRecords(ctx, plan, step, publisher)
		if err != nil {
			return false, err
		}
	} else {
		return false, err
	}
	// syncZoneToDNSLocked first reconciles any exact V3 lease. It never needs a
	// remote panel API; the native pair transfers the published owner zone.
	if err := p.syncZoneToDNSLocked(ctx, receipt.Zone, false); err != nil {
		if dnsPublicationRefusedBusy(err) {
			// Refused at admission while another change held the host: the
			// records stay saved under this receipt, and the step publishes
			// them on its next pass (a bounded wait, D10).
			return false, &serverSetupZonePublishBusyError{Zone: receipt.Zone, Record: "the server's address records", Err: err}
		}
		if _, leaseErr := readDNSZoneEngineLease(ctx, p.db.GetDB(), receipt.Zone); leaseErr == nil {
			return false, errServerSetupInfrastructureDNSUnknown
		} else if !errors.Is(leaseErr, sql.ErrNoRows) {
			return false, errServerSetupInfrastructureDNSUnknown
		}
		syncState, stateErr := readDNSZoneSyncState(ctx, p.db.GetDB(), receipt.Zone)
		if stateErr != nil {
			return false, errServerSetupInfrastructureDNSUnknown
		}
		if syncState.Status == "error" && syncState.DesiredGeneration > receipt.PreparedGeneration {
			receipt.Failed = true
			next, _ := json.Marshal(receipt)
			if _, writeErr := p.db.GetDB().ExecContext(ctx, `UPDATE panel_settings SET value=? WHERE key=?`, string(next), serverSetupInfrastructureDNSReceiptKey(step.RequestID)); writeErr != nil {
				return false, errServerSetupInfrastructureDNSUnknown
			}
			return false, errors.New("infrastructure DNS publication failed; inspect Server → DNS and peer servers and review a revised plan")
		}
		_, ready, readinessErr := p.activeDNSPublisher(ctx)
		if readinessErr != nil || ready {
			return false, errServerSetupInfrastructureDNSUnknown
		}
		return false, errServerSetupInfrastructureDNSWaiting
	}
	currentSnapshot, snapshotErr := readServerSetupInfrastructureDNSSnapshot(ctx, p.db.GetDB(), receipt.Zone)
	if snapshotErr != nil {
		return false, snapshotErr
	}
	if serverSetupInfrastructureDNSSnapshotDigest(currentSnapshot, true) != receipt.Digest {
		return false, errServerSetupInfrastructureDNSChanged
	}
	applied, err := p.infrastructureDNSReceiptApplied(ctx, receipt)
	if err != nil {
		return false, errServerSetupInfrastructureDNSUnknown
	}
	if !applied {
		return false, errServerSetupInfrastructureDNSUnknown
	}
	return true, nil
}
