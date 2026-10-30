package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Automatic DKIM for every domain that can send mail from this server
// (30 Oct 2026, last mail gap before alpha.113).
//
// Until now only server setup's mail step made a DKIM key, and only for the
// main domain. A site added later, or a domain given its first mailbox, sent
// unsigned mail until the owner pressed "Generate DKIM key" and "Add to DNS".
// Now the product's own rule for "this domain has mail here"
// (domainHasProductMailRuntime: mailboxes, forwardings or a catch-all of the
// domain, or a mail service CelikPanel installed on this server) decides,
// and the same calls as those two buttons run by themselves:
//
//   - triggers: a domain created while mail is installed (domain_created),
//     a domain's first mailbox (first_mailbox), a mail profile installed from
//     Services for every existing domain (mail_profile), the owner's "Try
//     again" (retry);
//   - the key: Agent.EnsureDKIMKey, which keeps any existing key (the owner's
//     or ours) and never replaces it;
//   - signing: Agent.ConfigureDKIMSigning under its own durable lease, only
//     when the key is new or not yet in the signing table, once per pass;
//   - the TXT record: only into a zone this server keeps (local DNS), through
//     the ordinary zone publication so a paired backup receives it too. A
//     different DKIM record already at that name is the owner's and is kept;
//     the card says so. External DNS: the card shows the record to copy.
//
// Nothing here can block or fail the creation that triggered it. The work is
// a persisted record per domain (panel_settings, versioned, no schema
// change) handled by one background runner; a domain create holds its record
// until the request has published the zone. The record says what is
// happening and, when it stops, why, who acts and how it resumes (D-024):
// pending (being set up), waiting (another server change holds the host; it
// resumes by itself), blocked (an unmet prerequisite), failed (a verified
// failure), unknown (the answer was lost). A status read never starts work;
// "Try again" queues the same record once and is refused as already running
// while it is pending, so polling cannot duplicate a mutation. Records still
// pending at Panel start are resumed (startAutomaticDKIM).
//
// Otomatik DKIM: bu sunucudan posta gönderebilen her alan adı için anahtar,
// imzalama ve (DNS bu sunucudaysa) TXT kaydı kendiliğinden yapılır. Var olan
// anahtar ve sahibin kaydı korunur; iş, alan adı başına kalıcı bir kayıtla
// arka planda yürür ve oluşturmayı asla engellemez. Durum okuması iş
// başlatmaz; "Yeniden dene" bekleyen işi ikinci kez başlatmaz.

const (
	dkimAutomaticKeyPrefix = "dkim_automatic_v1:"
	dkimSigningRefreshKey  = "dkim_signing_refresh_v1"
	dkimAutomaticVersion   = 1
	dkimAutomaticLineMax   = 200

	dkimAutoPending = "pending"
	dkimAutoWaiting = "waiting"
	dkimAutoBlocked = "blocked"
	dkimAutoFailed  = "failed"
	dkimAutoUnknown = "unknown"

	dkimTriggerDomainCreated = "domain_created"
	dkimTriggerFirstMailbox  = "first_mailbox"
	dkimTriggerMailProfile   = "mail_profile"
	dkimTriggerRetry         = "retry"

	dkimReasonKeyFailed      = "key_failed"
	dkimReasonKeyUnknown     = "key_unknown"
	dkimReasonSigningFailed  = "signing_failed"
	dkimReasonSigningUnknown = "signing_unknown"
	dkimReasonHostBusy       = "host_busy"
	dkimReasonDNSNotReady    = "dns_not_ready"
	dkimReasonDNSPublish     = "dns_publish_failed"

	// How often a pass that waited for a busy host is repeated in this
	// process before the record is left for the next start or "Try again".
	dkimAutomaticBusyRetries = 20
)

var dkimAutomaticBusyDelay = 30 * time.Second

type dkimAutomaticRecord struct {
	Version    int    `json:"version"`
	DomainID   int    `json:"domain_id"`
	Domain     string `json:"domain"`
	Trigger    string `json:"trigger"`
	State      string `json:"state"`
	Reason     string `json:"reason,omitempty"`
	ErrorLine  string `json:"error_line,omitempty"`
	Hold       bool   `json:"hold,omitempty"`
	RecordedAt string `json:"recorded_at"`
}

// dkimAutomaticView is what the Mail authentication card receives.
type dkimAutomaticView struct {
	State      string `json:"state"`
	Reason     string `json:"reason,omitempty"`
	ErrorLine  string `json:"error_line,omitempty"`
	Trigger    string `json:"trigger"`
	RecordedAt string `json:"recorded_at"`
}

func dkimAutomaticKey(domainID int) string {
	return dkimAutomaticKeyPrefix + strconv.Itoa(domainID)
}

func validDKIMAutomaticState(state string) bool {
	switch state {
	case dkimAutoPending, dkimAutoWaiting, dkimAutoBlocked, dkimAutoFailed, dkimAutoUnknown:
		return true
	}
	return false
}

func decodeDKIMAutomaticRecord(raw string) (dkimAutomaticRecord, bool) {
	var record dkimAutomaticRecord
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&record) != nil || record.Version != dkimAutomaticVersion ||
		record.DomainID <= 0 || record.Domain == "" || !validDKIMAutomaticState(record.State) ||
		utf8.RuneCountInString(record.ErrorLine) > dkimAutomaticLineMax {
		return dkimAutomaticRecord{}, false
	}
	return record, true
}

// dkimAutomaticLine keeps one bounded line of an error for the card.
func dkimAutomaticLine(text string) string {
	line := boundedDeletionErrorLine(text)
	if utf8.RuneCountInString(line) > dkimAutomaticLineMax {
		line = string([]rune(line)[:dkimAutomaticLineMax-1]) + "…"
	}
	return line
}

func (p *Panel) writeDKIMAutomaticRecord(ctx context.Context, record dkimAutomaticRecord) error {
	record.Version = dkimAutomaticVersion
	record.Domain = strings.ToLower(strings.TrimSpace(record.Domain))
	record.RecordedAt = time.Now().UTC().Format(time.RFC3339)
	raw, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if err := p.setSetting(ctx, dkimAutomaticKey(record.DomainID), string(raw)); err != nil {
		return fmt.Errorf("record automatic DKIM state: %w", err)
	}
	return nil
}

func (p *Panel) clearDKIMAutomaticRecord(ctx context.Context, domainID int) error {
	if _, err := p.db.GetDB().ExecContext(ctx,
		`DELETE FROM panel_settings WHERE key = ?`, dkimAutomaticKey(domainID)); err != nil {
		return fmt.Errorf("clear automatic DKIM state: %w", err)
	}
	return nil
}

// readDKIMAutomaticRecord is read-only; a malformed or foreign record is
// reported as absent.
func (p *Panel) readDKIMAutomaticRecord(ctx context.Context, domainID int, domain string) (dkimAutomaticRecord, bool, error) {
	var raw string
	err := p.db.GetDB().QueryRowContext(ctx,
		`SELECT value FROM panel_settings WHERE key = ?`, dkimAutomaticKey(domainID)).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return dkimAutomaticRecord{}, false, nil
	}
	if err != nil {
		return dkimAutomaticRecord{}, false, err
	}
	record, ok := decodeDKIMAutomaticRecord(raw)
	if !ok || record.DomainID != domainID || record.Domain != strings.ToLower(strings.TrimSpace(domain)) {
		return dkimAutomaticRecord{}, false, nil
	}
	return record, true, nil
}

func (p *Panel) listDKIMAutomaticRecords(ctx context.Context) ([]dkimAutomaticRecord, error) {
	rows, err := p.db.GetDB().QueryContext(ctx,
		`SELECT key, value FROM panel_settings WHERE key LIKE ? ORDER BY key`, dkimAutomaticKeyPrefix+"%")
	if err != nil {
		return nil, fmt.Errorf("list automatic DKIM records: %w", err)
	}
	defer rows.Close()
	var records []dkimAutomaticRecord
	for rows.Next() {
		var key, raw string
		if err := rows.Scan(&key, &raw); err != nil {
			return nil, err
		}
		record, ok := decodeDKIMAutomaticRecord(raw)
		if !ok || key != dkimAutomaticKey(record.DomainID) {
			log.Printf("[dkim-auto] ignoring malformed record %s", key)
			continue
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

// requestAutomaticDKIM queues the automatic DKIM work for a domain. It
// returns false when the same work is already pending (nothing new is
// queued). A held record is released by releaseAutomaticDKIM.
func (p *Panel) requestAutomaticDKIM(ctx context.Context, domainID int, domain, trigger string, hold bool) (bool, error) {
	existing, ok, err := p.readDKIMAutomaticRecord(ctx, domainID, domain)
	if err != nil {
		return false, err
	}
	if ok && existing.State == dkimAutoPending && !existing.Hold && !hold {
		return false, nil
	}
	return true, p.writeDKIMAutomaticRecord(ctx, dkimAutomaticRecord{
		DomainID: domainID, Domain: domain, Trigger: trigger, State: dkimAutoPending, Hold: hold,
	})
}

// releaseAutomaticDKIM lets the runner take a held record (the request that
// created the domain has finished publishing its zone) and starts it.
func (p *Panel) releaseAutomaticDKIM(ctx context.Context, domainID int, domain string) {
	record, ok, err := p.readDKIMAutomaticRecord(ctx, domainID, domain)
	if err == nil && ok && record.Hold {
		record.Hold = false
		err = p.writeDKIMAutomaticRecord(ctx, record)
	}
	if err != nil {
		log.Printf("[dkim-auto] release %s: %v", domain, err)
	}
	p.kickAutomaticDKIM()
}

// queueAutomaticDKIMForNewDomain is the domain-create trigger. It never
// fails the creation: an error is logged and the card offers the manual
// buttons as before. The returned function must run when the request ends.
func (p *Panel) queueAutomaticDKIMForNewDomain(ctx context.Context, domainID int, domain string) func() {
	applies, err := domainHasProductMailRuntime(ctx, p.db.GetDB(), domainID)
	if err != nil {
		log.Printf("[dkim-auto] %s: read mail records: %v", domain, err)
		return func() {}
	}
	if !applies {
		return func() {}
	}
	if _, err := p.requestAutomaticDKIM(ctx, domainID, domain, dkimTriggerDomainCreated, true); err != nil {
		log.Printf("[dkim-auto] %s: queue: %v", domain, err)
		return func() {}
	}
	return func() {
		releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		p.releaseAutomaticDKIM(releaseCtx, domainID, domain)
	}
}

// queueAutomaticDKIMForMailbox is the first-mailbox trigger.
func (p *Panel) queueAutomaticDKIMForMailbox(ctx context.Context, domainID int, domain string) {
	var count int
	if err := p.db.GetDB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM email_accounts WHERE domain_id = ?`, domainID).Scan(&count); err != nil {
		log.Printf("[dkim-auto] %s: count mailboxes: %v", domain, err)
		return
	}
	if count != 1 {
		return
	}
	if _, err := p.requestAutomaticDKIM(ctx, domainID, domain, dkimTriggerFirstMailbox, false); err != nil {
		log.Printf("[dkim-auto] %s: queue: %v", domain, err)
		return
	}
	p.kickAutomaticDKIM()
}

// queueAutomaticDKIMForAllDomains is the mail-profile trigger: every active
// domain gets its key once the mail service exists.
func (p *Panel) queueAutomaticDKIMForAllDomains(ctx context.Context) {
	rows, err := p.db.GetDB().QueryContext(ctx, `
		SELECT d.id, d.name FROM domains d
		WHERE d.status = 'active'
		  AND NOT EXISTS (SELECT 1 FROM domain_deletion_operations o WHERE o.domain_id = d.id)
		ORDER BY d.id`)
	if err != nil {
		log.Printf("[dkim-auto] list domains: %v", err)
		return
	}
	type domainRow struct {
		id   int
		name string
	}
	var domains []domainRow
	for rows.Next() {
		var row domainRow
		if err := rows.Scan(&row.id, &row.name); err != nil {
			rows.Close()
			log.Printf("[dkim-auto] list domains: %v", err)
			return
		}
		domains = append(domains, row)
	}
	rows.Close()
	for _, domain := range domains {
		if _, err := p.requestAutomaticDKIM(ctx, domain.id, domain.name, dkimTriggerMailProfile, false); err != nil {
			log.Printf("[dkim-auto] %s: queue: %v", domain.name, err)
		}
	}
	if len(domains) > 0 {
		p.kickAutomaticDKIM()
	}
}

// kickAutomaticDKIM runs the pass in the background, one runner at a time; a
// kick during a pass makes the runner go round once more.
func (p *Panel) kickAutomaticDKIM() {
	if p.automaticDKIMKicked != nil {
		p.automaticDKIMKicked()
		return
	}
	p.dkimAutoMu.Lock()
	if p.dkimAutoRunning {
		p.dkimAutoAgain = true
		p.dkimAutoMu.Unlock()
		return
	}
	p.dkimAutoRunning = true
	p.dkimAutoMu.Unlock()
	go p.automaticDKIMRunner(false)
}

// startAutomaticDKIM resumes, at Panel start, every record left pending or
// waiting (held ones included: the request that held them is gone).
func (p *Panel) startAutomaticDKIM() {
	p.dkimAutoMu.Lock()
	if p.dkimAutoRunning {
		p.dkimAutoAgain = true
		p.dkimAutoMu.Unlock()
		return
	}
	p.dkimAutoRunning = true
	p.dkimAutoMu.Unlock()
	go p.automaticDKIMRunner(true)
}

func (p *Panel) automaticDKIMRunner(startup bool) {
	busyRounds := 0
	for {
		p.dkimAutoMu.Lock()
		p.dkimAutoAgain = false
		p.dkimAutoMu.Unlock()

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
		retry := p.runAutomaticDKIMPass(ctx, startup)
		cancel()
		startup = false

		p.dkimAutoMu.Lock()
		again := p.dkimAutoAgain
		if !again && retry && busyRounds < dkimAutomaticBusyRetries {
			p.dkimAutoMu.Unlock()
			busyRounds++
			time.Sleep(dkimAutomaticBusyDelay)
			continue
		}
		if !again {
			p.dkimAutoRunning = false
			p.dkimAutoMu.Unlock()
			return
		}
		p.dkimAutoMu.Unlock()
	}
}

type dkimAutomaticItem struct {
	record  dkimAutomaticRecord
	pub     string
	created bool
	signed  bool
}

func (item dkimAutomaticItem) needsSigning() bool { return item.created || !item.signed }

// runAutomaticDKIMPass handles every pending or waiting record once. It
// reports whether a record waits for a busy host (the runner repeats).
func (p *Panel) runAutomaticDKIMPass(ctx context.Context, startup bool) bool {
	records, err := p.listDKIMAutomaticRecords(ctx)
	if err != nil {
		log.Printf("[dkim-auto] %v", err)
		return false
	}
	refresh := p.setting(ctx, dkimSigningRefreshKey) != ""
	var items []dkimAutomaticItem
	for _, record := range records {
		if record.State != dkimAutoPending && record.State != dkimAutoWaiting {
			continue
		}
		if record.Hold && !startup {
			continue
		}
		if item, ok := p.automaticDKIMKey(ctx, record); ok {
			items = append(items, item)
		}
	}

	needSigning := refresh
	for _, item := range items {
		needSigning = needSigning || item.needsSigning()
	}
	retry := false
	signingOK := true
	if needSigning {
		state, reason, line := p.automaticDKIMSigning(ctx)
		if state != "" {
			signingOK = false
			retry = state == dkimAutoWaiting
			log.Printf("[dkim-auto] signing not configured (%s): %s", reason, line)
			for _, item := range items {
				if item.needsSigning() {
					p.recordAutomaticDKIM(ctx, item.record, state, reason, line)
				}
			}
		} else if refresh {
			if _, err := p.db.GetDB().ExecContext(ctx,
				`DELETE FROM panel_settings WHERE key = ?`, dkimSigningRefreshKey); err != nil {
				log.Printf("[dkim-auto] clear signing refresh: %v", err)
			}
		}
	}
	for _, item := range items {
		if !signingOK && item.needsSigning() {
			continue
		}
		p.automaticDKIMPublish(ctx, item)
	}
	return retry
}

func (p *Panel) recordAutomaticDKIM(ctx context.Context, record dkimAutomaticRecord, state, reason, line string) {
	record.State, record.Reason, record.ErrorLine, record.Hold = state, reason, dkimAutomaticLine(line), false
	if line == "" {
		record.ErrorLine = ""
	}
	if err := p.writeDKIMAutomaticRecord(ctx, record); err != nil {
		log.Printf("[dkim-auto] %s: %v", record.Domain, err)
	}
	if state != dkimAutoWaiting {
		log.Printf("[dkim-auto] %s: %s (%s) %s", record.Domain, state, reason, line)
	}
}

func (p *Panel) finishAutomaticDKIM(ctx context.Context, record dkimAutomaticRecord, note string) {
	if err := p.clearDKIMAutomaticRecord(ctx, record.DomainID); err != nil {
		log.Printf("[dkim-auto] %s: %v", record.Domain, err)
		return
	}
	log.Printf("[dkim-auto] %s: DKIM ready (%s; %s)", record.Domain, record.Trigger, note)
}

// automaticDKIMKey makes (or reuses) the key while holding the mail lock, so
// a domain deletion (which removes keys under the same lock after marking
// the domain) can never be followed by a key made for the deleted domain.
func (p *Panel) automaticDKIMKey(ctx context.Context, record dkimAutomaticRecord) (dkimAutomaticItem, bool) {
	p.mailMutationMu.Lock()
	var status string
	var marked int
	err := p.db.GetDB().QueryRowContext(ctx, `
		SELECT d.status, EXISTS(SELECT 1 FROM domain_deletion_operations o WHERE o.domain_id = d.id)
		FROM domains d WHERE d.id = ? AND lower(d.name) = ?`,
		record.DomainID, record.Domain).Scan(&status, &marked)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && marked != 0) {
		p.mailMutationMu.Unlock()
		_ = p.clearDKIMAutomaticRecord(ctx, record.DomainID)
		return dkimAutomaticItem{}, false
	}
	if err != nil || status != "active" {
		// Read again on the next pass (an import or a restore in progress).
		p.mailMutationMu.Unlock()
		return dkimAutomaticItem{}, false
	}
	applies := record.Trigger == dkimTriggerRetry
	if !applies {
		applies, err = domainHasProductMailRuntime(ctx, p.db.GetDB(), record.DomainID)
		if err != nil {
			p.mailMutationMu.Unlock()
			return dkimAutomaticItem{}, false
		}
	}
	if !applies {
		p.mailMutationMu.Unlock()
		_ = p.clearDKIMAutomaticRecord(ctx, record.DomainID)
		return dkimAutomaticItem{}, false
	}
	var key transport.DKIMEnsureResponse
	err = p.callAgentContext(ctx, "Agent.EnsureDKIMKey",
		&transport.DKIMEnsureRequest{Domain: record.Domain, Selector: dkimSelector}, &key)
	p.mailMutationMu.Unlock()
	if err != nil {
		p.recordAutomaticDKIM(ctx, record, dkimAutoUnknown, dkimReasonKeyUnknown, err.Error())
		return dkimAutomaticItem{}, false
	}
	if key.Error != "" || key.PublicKeyB64 == "" {
		p.recordAutomaticDKIM(ctx, record, dkimAutoFailed, dkimReasonKeyFailed, key.Error)
		return dkimAutomaticItem{}, false
	}
	item := dkimAutomaticItem{record: record, pub: key.PublicKeyB64, created: key.Created}
	var status2 transport.DKIMStatusResponse
	if err := p.callAgentContext(ctx, "Agent.GetDKIMStatus",
		&transport.DKIMStatusRequest{Domain: record.Domain, Selector: dkimSelector}, &status2); err == nil && status2.Error == "" {
		item.signed = status2.Signed
	}
	return item, true
}

// automaticDKIMSigning runs the "Generate DKIM key" signing step once. An
// empty state means signing is configured.
func (p *Panel) automaticDKIMSigning(ctx context.Context) (state, reason, line string) {
	p.serviceMutationMu.Lock()
	defer p.serviceMutationMu.Unlock()
	var sign transport.ConfigureDKIMSigningResponse
	err := p.withStandaloneAgentMutation(ctx, "dkim_signing_configure", "opendkim", "", func(callCtx context.Context, binding agentMutationBinding) error {
		request := transport.ServiceMutationRequest{ServiceMutationBinding: binding}
		if err := p.callAgentContext(callCtx, "Agent.ConfigureDKIMSigning", &request, &sign); err != nil {
			return err
		}
		if sign.Error != "" {
			return errors.New(sign.Error)
		}
		return nil
	})
	switch {
	case err == nil && sign.Configured:
		return "", "", ""
	case err == nil:
		return dkimAutoFailed, dkimReasonSigningFailed, "the Agent did not confirm DKIM signing"
	case sign.Error != "":
		return dkimAutoFailed, dkimReasonSigningFailed, sign.Error
	case hostMutationRefusedAsBusy(err):
		return dkimAutoWaiting, dkimReasonHostBusy, ""
	default:
		return dkimAutoUnknown, dkimReasonSigningUnknown, err.Error()
	}
}

// mailAuthZone finds the zone this server keeps that holds a domain's mail
// records: the domain's own zone, else, for a subdomain, its parent
// domain's zone (where its records live). Only the recorded parent link is
// followed: a name that merely ends in another tenant's domain never reaches
// that tenant's zone.
func (p *Panel) mailAuthZone(ctx context.Context, domain string) (int, string, bool, error) {
	name := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(domain)), ".")
	var zoneID int
	err := p.db.GetDB().QueryRowContext(ctx, `SELECT id FROM pdns_domains WHERE name = ?`, name).Scan(&zoneID)
	if err == nil {
		return zoneID, name, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, "", false, err
	}
	var parent string
	err = p.db.GetDB().QueryRowContext(ctx, `
		SELECT lower(pd.name) FROM domains d JOIN domains pd ON pd.id = d.parent_domain_id
		WHERE lower(d.name) = ?`, name).Scan(&parent)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", false, nil
	}
	if err != nil {
		return 0, "", false, err
	}
	if !strings.HasSuffix(name, "."+parent) {
		return 0, "", false, nil
	}
	err = p.db.GetDB().QueryRowContext(ctx, `SELECT id FROM pdns_domains WHERE name = ?`, parent).Scan(&zoneID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", false, nil
	}
	if err != nil {
		return 0, "", false, err
	}
	return zoneID, parent, true, nil
}

// zoneTXTValues returns every enabled TXT value at a name (segments joined).
func (p *Panel) zoneTXTValues(ctx context.Context, zoneID int, name string) ([]string, error) {
	rows, err := p.db.GetDB().QueryContext(ctx,
		`SELECT content FROM pdns_records WHERE domain_id = ? AND type = 'TXT' AND name = ? AND disabled = 0 ORDER BY id`,
		zoneID, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var values []string
	for rows.Next() {
		var content string
		if err := rows.Scan(&content); err != nil {
			return nil, err
		}
		values = append(values, joinTXTSegments(content))
	}
	return values, rows.Err()
}

// automaticDKIMPublish publishes the DKIM TXT record where this server keeps
// the zone, like "Add to DNS", without replacing a different record.
func (p *Panel) automaticDKIMPublish(ctx context.Context, item dkimAutomaticItem) {
	record := item.record
	mode, err := p.domainDNSManagementMode(ctx, record.Domain)
	if err != nil {
		p.recordAutomaticDKIM(ctx, record, dkimAutoUnknown, dkimReasonDNSPublish, err.Error())
		return
	}
	if mode != setupDNSModeLocal {
		// External DNS or a remote DNS server: the card shows the record.
		p.finishAutomaticDKIM(ctx, record, "DNS is kept elsewhere; the card shows the record")
		return
	}
	zoneID, zoneName, found, err := p.mailAuthZone(ctx, record.Domain)
	if err != nil {
		p.recordAutomaticDKIM(ctx, record, dkimAutoUnknown, dkimReasonDNSPublish, err.Error())
		return
	}
	if !found {
		p.finishAutomaticDKIM(ctx, record, "no zone on this server holds the name")
		return
	}
	name := dkimSelector + "._domainkey." + record.Domain
	want := dkimRecommended(item.pub)
	values, err := p.zoneTXTValues(ctx, zoneID, name)
	if err != nil {
		p.recordAutomaticDKIM(ctx, record, dkimAutoUnknown, dkimReasonDNSPublish, err.Error())
		return
	}
	present, other := false, false
	for _, value := range values {
		if value == want {
			present = true
		} else if value != "" {
			other = true
		}
	}
	if other && !present {
		// Owner authority (D-022): a record at this name that is not ours is
		// kept. The card reports it (dkim_owner_record).
		p.finishAutomaticDKIM(ctx, record, "a different record at "+name+" was kept")
		return
	}
	if present {
		if state, err := readDNSZoneSyncState(ctx, p.db.GetDB(), zoneName); err == nil &&
			state.Status == "applied" && state.AppliedGeneration == state.DesiredGeneration {
			p.finishAutomaticDKIM(ctx, record, "the record is already published")
			return
		}
	}
	publisher, ready, err := p.activeDNSPublisher(ctx)
	if err == nil && !ready && publisher.PairRole == transport.DNSPairRoleSecondary {
		// A backup DNS server never publishes zones itself; the card shows
		// the record for the main DNS server.
		p.finishAutomaticDKIM(ctx, record, "this is the backup DNS server; it does not publish zones")
		return
	}
	if err != nil || !ready {
		line := "the DNS engine is not ready to publish (not active, or the backup DNS server has not caught up)"
		if err != nil {
			line = err.Error()
		}
		p.recordAutomaticDKIM(ctx, record, dkimAutoBlocked, dkimReasonDNSNotReady, line)
		return
	}
	if !present {
		if _, err := p.db.GetDB().ExecContext(ctx,
			`INSERT INTO pdns_records (domain_id, name, type, content, ttl) VALUES (?, ?, 'TXT', ?, 3600)`,
			zoneID, name, splitTXTContent(want)); err != nil {
			p.recordAutomaticDKIM(ctx, record, dkimAutoFailed, dkimReasonDNSPublish, err.Error())
			return
		}
	}
	publishCtx, cancel := context.WithTimeout(ctx, dnsPublishTimeout)
	defer cancel()
	if err := p.syncZoneToDNS(publishCtx, zoneName, false); err != nil {
		// A held publication is saved and completes by itself through the
		// DNS publication reconciler (the card's record shows "In zone —
		// waiting for DNS" meanwhile); only a real failure stops here.
		var held *dnsPublicationHeldError
		if errors.As(err, &held) || strings.Contains(err.Error(), dnsAgentPendingRecoveryRefusal) {
			if held == nil {
				p.dnsPendingPublications().armRepublish()
			}
			p.finishAutomaticDKIM(ctx, record, "saved in "+zoneName+"; its publication completes by itself")
			return
		}
		p.recordAutomaticDKIM(ctx, record, dkimAutoFailed, dkimReasonDNSPublish, err.Error())
		return
	}
	p.finishAutomaticDKIM(ctx, record, "record published in "+zoneName)
}

// handleMailAuthAutomatic is "Try again": it queues the automatic work once.
// While the same work is pending it answers 202 and starts nothing.
func (p *Panel) handleMailAuthAutomatic(w http.ResponseWriter, r *http.Request, domainID int, domain string) {
	w.Header().Set("Content-Type", "application/json")
	if err := p.ensureMailDomainMutable(r.Context(), domainID); err != nil {
		writeMailDomainMutationError(w, err)
		return
	}
	queued, err := p.requestAutomaticDKIM(r.Context(), domainID, domain, dkimTriggerRetry, false)
	if err != nil {
		writeServerError(w, err)
		return
	}
	p.kickAutomaticDKIM()
	if queued {
		p.audit(r, "mail.dkim.automatic.retry:"+domain, "domain", domainID)
	}
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{"state": dkimAutoPending, "queued": queued})
}

// --- Deletion ---------------------------------------------------------------

// domainDKIMCleanup is what deleting a domain did with its DKIM key.
type domainDKIMCleanup struct {
	Removed         bool     `json:"removed"`
	KeptKey         string   `json:"kept_key,omitempty"`
	OwnerTableLines []string `json:"owner_table_lines,omitempty"`
	Error           string   `json:"error,omitempty"`
}

func (c domainDKIMCleanup) worthReporting() bool {
	return c.KeptKey != "" || len(c.OwnerTableLines) > 0 || c.Error != ""
}

// removeDomainDKIMLocked requires p.mailMutationMu (held by the deletion
// after it marked the domain, the same lock automaticDKIMKey takes). The
// key CelikPanel made goes; anything else is kept and reported. It never
// stops the deletion: a failure is reported in the deletion's answer, the
// audit trail and the log. The signing tables are refreshed by the
// automatic runner (persisted request, so a restart does not lose it).
func (p *Panel) removeDomainDKIMLocked(ctx context.Context, domainID int, domain string) domainDKIMCleanup {
	var cleanup domainDKIMCleanup
	if err := p.clearDKIMAutomaticRecord(ctx, domainID); err != nil {
		log.Printf("[dkim] %s: %v", domain, err)
	}
	var resp transport.DKIMRemoveResponse
	err := p.callAgentContext(ctx, "Agent.RemoveDKIMKey", &transport.DKIMRemoveRequest{
		ExpectedBuildCommit: strings.TrimSpace(buildCommit),
		Domain:              strings.ToLower(strings.TrimSpace(domain)),
		Selector:            dkimSelector,
	}, &resp)
	if err != nil {
		cleanup.Error = dkimAutomaticLine(err.Error())
		return cleanup
	}
	if resp.Error != "" {
		cleanup.Error = dkimAutomaticLine(resp.Error)
		return cleanup
	}
	cleanup.Removed = resp.Removed
	cleanup.KeptKey = resp.KeptKey
	cleanup.OwnerTableLines = resp.OwnerTableLines
	if resp.Removed && resp.SigningConfigured {
		if err := p.setSetting(ctx, dkimSigningRefreshKey, time.Now().UTC().Format(time.RFC3339)); err != nil {
			log.Printf("[dkim] %s: request signing refresh: %v", domain, err)
		}
	}
	return cleanup
}
