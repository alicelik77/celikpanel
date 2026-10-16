package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/hostname"
)

// A host record for the peer in the infrastructure zone (2026-10-16, the
// owner's decision 3). The second server (the DNS secondary, for example
// Boston) asks the primary over the existing authorized remote DNS connection
// to create or update the A/AAAA record of one of its own host names (its mail
// host, its panel host) inside the primary's infrastructure zone. The primary
// applies it to its own zone and publishes it like any change of that zone;
// the secondary then receives it by the standard zone transfer. Replication
// never depends on this channel: it is the optional, authorized
// record-management automation of D-022, and without it (no connection, a
// refusal, an older primary) the secondary's guidance shows the exact record
// for the owner to add by hand.
//
// The primary accepts only: a name strictly inside one of the zones server
// setup created on it; type A with the DNS pair's verified peer address, or
// AAAA with the address the request came from; and only when no other record
// claims the name. An existing record with the same value is an idempotent
// answer; a record this channel created earlier for the same client is
// updated only while its value is still the one written (an owner's change
// is detected and refused, never overwritten). Each request is audited on
// both servers and runs under the same locks as every DNS publication.
// Ikincil sunucu, birincilin altyapi bolgesinde kendi ad kaydini ister;
// birincil kendi bolgesine uygular, standart bolge aktarimi ikincile tasir.
// Kanal yoksa ya da ret varsa elle girilecek kayit gosterilir.

const remoteDNSHostRecordPath = "/api/v1/dns/remote/receiver/host-record"

const (
	errCodeRemoteDNSHostRecordInvalid     = "REMOTE_DNS_HOST_RECORD_INVALID"
	errCodeRemoteDNSHostRecordOutsideZone = "REMOTE_DNS_HOST_RECORD_OUTSIDE_ZONE"
	errCodeRemoteDNSHostRecordAddress     = "REMOTE_DNS_HOST_RECORD_ADDRESS"
	errCodeRemoteDNSHostRecordConflict    = "REMOTE_DNS_HOST_RECORD_CONFLICT"
	errCodeRemoteDNSHostRecordPending     = "REMOTE_DNS_HOST_RECORD_PENDING"
	remoteDNSHostRecordTTL                = 300
)

type remoteDNSHostRecordRequest struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Content string `json:"content"`
}

// The answer of an applied request. Result is "created", "updated" or
// "present" (the zone already had exactly this record).
type remoteDNSHostRecordAnswer struct {
	ClientID string `json:"client_id"`
	Zone     string `json:"zone"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Content  string `json:"content"`
	Result   string `json:"result"`
	Applied  bool   `json:"applied"`
}

// One record this channel created, kept so a later request can tell its own
// record from an owner's change.
type remoteDNSHostRecordEntry struct {
	ClientID  string `json:"client_id"`
	Zone      string `json:"zone"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Content   string `json:"content"`
	RecordID  int64  `json:"record_id"`
	UpdatedAt string `json:"updated_at"`
}

func remoteDNSHostRecordKeyPrefix(zone string) string {
	return "remote_dns_host_record:" + zone + ":"
}

func remoteDNSHostRecordKey(zone, name, recordType string) string {
	return remoteDNSHostRecordKeyPrefix(zone) + name + ":" + recordType
}

func (p *Panel) remoteDNSHostRecordLedger(ctx context.Context, zone string) ([]remoteDNSHostRecordEntry, error) {
	rows, err := p.db.GetDB().QueryContext(ctx, `SELECT key,value FROM panel_settings WHERE key LIKE ? ORDER BY key LIMIT 257`, remoteDNSHostRecordKeyPrefix(zone)+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := []remoteDNSHostRecordEntry{}
	for rows.Next() {
		var key, raw string
		if err := rows.Scan(&key, &raw); err != nil {
			return nil, err
		}
		var entry remoteDNSHostRecordEntry
		if json.Unmarshal([]byte(raw), &entry) != nil || entry.Zone != zone || remoteDNSHostRecordKey(zone, entry.Name, entry.Type) != key {
			continue
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

type remoteDNSHostRecordRefusal struct {
	status  int
	code    string
	message string
}

func (e *remoteDNSHostRecordRefusal) Error() string { return e.message }

func hostRecordRefusal(status int, code, message string) error {
	return &remoteDNSHostRecordRefusal{status: status, code: code, message: message}
}

// canonicalRemoteDNSHostRecord validates the request's shape only.
func canonicalRemoteDNSHostRecord(request remoteDNSHostRecordRequest) (remoteDNSHostRecordRequest, error) {
	invalid := hostRecordRefusal(http.StatusBadRequest, errCodeRemoteDNSHostRecordInvalid, "The host record must be one fully qualified name with an A or AAAA address.")
	name, err := hostname.CanonicalFQDN(request.Name)
	if err != nil || name != request.Name || validateDNSOwner(name) != nil || strings.HasPrefix(name, "*") {
		return request, invalid
	}
	address, err := netip.ParseAddr(request.Content)
	if err != nil || address.String() != request.Content || address.Zone() != "" {
		return request, invalid
	}
	switch request.Type {
	case "A":
		if !address.Is4() {
			return request, invalid
		}
	case "AAAA":
		if !address.Is6() || address.Is4In6() {
			return request, invalid
		}
	default:
		return request, invalid
	}
	return request, nil
}

// remoteDNSHostRecordZone: the longest infrastructure zone that strictly
// contains name, or "".
func (p *Panel) remoteDNSHostRecordZone(ctx context.Context, name string) (string, error) {
	zones, err := p.infrastructureDNSZones(ctx)
	if err != nil {
		return "", err
	}
	best := ""
	for _, zone := range zones {
		if name != zone.Zone && serverSetupDNSNameWithin(name, zone.Zone) && len(zone.Zone) > len(best) {
			best = zone.Zone
		}
	}
	return best, nil
}

func (p *Panel) handleRemoteDNSHostRecord(w http.ResponseWriter, r *http.Request, claim remoteDNSMachineClaim) {
	var request remoteDNSHostRecordRequest
	if !decodeRemoteDNSBody(w, r, &request) {
		return
	}
	refuse := func(err error) {
		var refusal *remoteDNSHostRecordRefusal
		if !errors.As(err, &refusal) {
			writeServerError(w, err)
			return
		}
		log.Printf("[%d][dns] remote host record %s %s from client %s refused: %s", refusal.status, request.Name, request.Type, claim.clientID, refusal.code)
		p.audit(r, "dns.remote.host_record.refused", "dns_zone", 0)
		writeCodedError(w, refusal.status, refusal.code, refusal.message, "")
	}
	canonical, err := canonicalRemoteDNSHostRecord(request)
	if err != nil {
		refuse(err)
		return
	}
	// The same locks as a received zone publication: one DNS change at a time.
	p.serviceMutationMu.Lock()
	defer p.serviceMutationMu.Unlock()
	dnsPublicationMu.Lock()
	defer dnsPublicationMu.Unlock()
	if !p.remoteDNSClientAuthorized(r.Context(), claim.clientID, claim.credentialHash) {
		writeClientError(w, http.StatusUnauthorized, "Remote DNS authorization was revoked")
		return
	}
	proof, err := remoteDNSReadLocalAuthority(p, r.Context())
	if err != nil {
		remoteDNSWriteNotReady(w)
		return
	}
	switch canonical.Type {
	case "A":
		if peer, ok := canonicalIPv4(proof.SecondaryIP); !ok || peer != canonical.Content {
			refuse(hostRecordRefusal(http.StatusForbidden, errCodeRemoteDNSHostRecordAddress, "The address must be the verified address of this server's DNS peer."))
			return
		}
	case "AAAA":
		source, err := netip.ParseAddr(clientIP(r))
		if err != nil || source.Unmap().String() != canonical.Content || !remoteDNSPublicAddress(source) {
			refuse(hostRecordRefusal(http.StatusForbidden, errCodeRemoteDNSHostRecordAddress, "An IPv6 address must be the address this request came from."))
			return
		}
	}
	zone, err := p.remoteDNSHostRecordZone(r.Context(), canonical.Name)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if zone == "" {
		refuse(hostRecordRefusal(http.StatusConflict, errCodeRemoteDNSHostRecordOutsideZone, "The name is not inside an infrastructure DNS zone that server setup prepared on this server."))
		return
	}
	answer, err := p.applyRemoteDNSHostRecord(r.Context(), claim.clientID, zone, canonical)
	if err != nil {
		refuse(err)
		return
	}
	if answer.Result != "present" || !p.remoteDNSZoneApplied(r.Context(), zone) {
		publishCtx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), dnsPublishTimeout)
		defer cancel()
		if err := remoteDNSPublishReceived(p, publishCtx, zone, false); err != nil {
			log.Printf("[409][dns] remote host record %s saved, publication pending: %v", canonical.Name, err)
			p.audit(r, "dns.remote.host_record.pending", "dns_zone", 0)
			writeCodedError(w, http.StatusConflict, errCodeRemoteDNSHostRecordPending, "The host record is saved in the zone but its publication is not verified yet; the same request can be sent again.", "")
			return
		}
	}
	log.Printf("[dns] remote host record %s %s %s for client %s: %s", canonical.Name, canonical.Type, canonical.Content, claim.clientID, answer.Result)
	p.audit(r, "dns.remote.host_record."+answer.Result, "dns_zone", 0)
	answer.ClientID = claim.clientID
	answer.Applied = true
	_ = json.NewEncoder(w).Encode(answer)
}

func (p *Panel) remoteDNSZoneApplied(ctx context.Context, zone string) bool {
	state, err := readDNSZoneSyncState(ctx, p.db.GetDB(), zone)
	return err == nil && state.Status == "applied" && state.DesiredGeneration == state.AppliedGeneration
}

// applyRemoteDNSHostRecord changes the ledger in one transaction. Caller holds
// the DNS publication locks.
func (p *Panel) applyRemoteDNSHostRecord(ctx context.Context, clientID, zone string, request remoteDNSHostRecordRequest) (remoteDNSHostRecordAnswer, error) {
	answer := remoteDNSHostRecordAnswer{Zone: zone, Name: request.Name, Type: request.Type, Content: request.Content}
	conflict := func(message string) error {
		return hostRecordRefusal(http.StatusConflict, errCodeRemoteDNSHostRecordConflict, message)
	}
	tx, err := p.db.GetDB().BeginTx(ctx, nil)
	if err != nil {
		return answer, err
	}
	defer tx.Rollback()
	snapshot, err := readServerSetupInfrastructureDNSSnapshot(ctx, tx, zone)
	if err != nil {
		var review *serverSetupInfrastructureDNSPlanError
		if errors.As(err, &review) {
			return answer, conflict("The infrastructure zone cannot take this record now: " + review.Message + ".")
		}
		return answer, err
	}
	if snapshot.ZoneID == 0 {
		return answer, hostRecordRefusal(http.StatusConflict, errCodeRemoteDNSHostRecordOutsideZone, "The infrastructure DNS zone is not on this server.")
	}
	same := []serverSetupInfrastructureDNSStoredRecord{}
	for _, record := range snapshot.Records {
		owner := canonicalDNSName(record.Name)
		if (record.Type == "CNAME" && owner == request.Name) ||
			(record.Type == "DNAME" && serverSetupDNSNameWithin(request.Name, owner)) ||
			(record.Type == "NS" && owner != zone && serverSetupDNSNameWithin(request.Name, owner)) {
			return answer, conflict(fmt.Sprintf("Another record (%s) already claims %s on this server; the owner's record is kept.", record.Type, request.Name))
		}
		if owner == request.Name && record.Type == request.Type {
			same = append(same, record)
		}
	}
	key := remoteDNSHostRecordKey(zone, request.Name, request.Type)
	var raw string
	var entry *remoteDNSHostRecordEntry
	err = tx.QueryRowContext(ctx, `SELECT value FROM panel_settings WHERE key=?`, key).Scan(&raw)
	if err == nil {
		var saved remoteDNSHostRecordEntry
		if json.Unmarshal([]byte(raw), &saved) != nil || saved.Zone != zone || saved.Name != request.Name || saved.Type != request.Type {
			return answer, errors.New("the saved host record request is unreadable")
		}
		entry = &saved
	} else if !errors.Is(err, sql.ErrNoRows) {
		return answer, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	writeEntry := func(recordID int64) error {
		encoded, _ := json.Marshal(remoteDNSHostRecordEntry{ClientID: clientID, Zone: zone, Name: request.Name, Type: request.Type, Content: request.Content, RecordID: recordID, UpdatedAt: now})
		_, err := tx.ExecContext(ctx, `INSERT INTO panel_settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, string(encoded))
		return err
	}
	switch {
	case len(same) > 1:
		return answer, conflict(fmt.Sprintf("%s already has more than one %s record on this server; the owner's records are kept.", request.Name, request.Type))
	case len(same) == 1 && same[0].Disabled != 0:
		return answer, conflict(fmt.Sprintf("%s has a disabled %s record on this server; the owner decides about it.", request.Name, request.Type))
	case len(same) == 1 && same[0].Content == request.Content:
		answer.Result = "present"
		return answer, tx.Commit()
	case len(same) == 1:
		// Only this channel's own, unchanged record is updated.
		if entry == nil || entry.ClientID != clientID || entry.RecordID != same[0].ID || entry.Content != same[0].Content {
			return answer, conflict(fmt.Sprintf("%s already points to %s on this server; that record is kept.", request.Name, same[0].Content))
		}
		if _, err := tx.ExecContext(ctx, `UPDATE pdns_records SET content=? WHERE id=? AND domain_id=? AND content=?`, request.Content, same[0].ID, snapshot.ZoneID, same[0].Content); err != nil {
			return answer, err
		}
		if err := writeEntry(same[0].ID); err != nil {
			return answer, err
		}
		answer.Result = "updated"
	default:
		if entry != nil {
			return answer, conflict(fmt.Sprintf("The record for %s that this connection created earlier was removed on this server; it is not created again without the owner.", request.Name))
		}
		result, err := tx.ExecContext(ctx, `INSERT INTO pdns_records(domain_id,name,type,content,ttl) VALUES(?,?,?,?,?)`, snapshot.ZoneID, request.Name, request.Type, request.Content, remoteDNSHostRecordTTL)
		if err != nil {
			return answer, err
		}
		id, err := result.LastInsertId()
		if err != nil {
			return answer, err
		}
		if err := writeEntry(id); err != nil {
			return answer, err
		}
		answer.Result = "created"
	}
	return answer, tx.Commit()
}

// --- The secondary's side ---------------------------------------------------

// serverSetupPeerHostRecordOutcome is what the secondary's setup knows about
// its request for one access_dns step, read by the guidance. No credential is
// stored here.
type serverSetupPeerHostRecordOutcome struct {
	RequestID  string `json:"request_id"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	Content    string `json:"content"`
	Status     string `json:"status"` // applied | refused | unknown | unsupported | none
	Code       string `json:"code,omitempty"`
	Result     string `json:"result,omitempty"`
	Zone       string `json:"zone,omitempty"`
	Endpoint   string `json:"endpoint,omitempty"`
	Primary    string `json:"primary,omitempty"`
	At         string `json:"at"`
	Attempts   int    `json:"attempts"`
	Connection string `json:"connection_id,omitempty"`
}

func serverSetupPeerHostRecordKey(requestID string) string {
	return "server_setup_peer_host_record:" + requestID
}

const serverSetupPeerHostRecordRetry = 2 * time.Minute
const serverSetupPeerHostRecordMaxAttempts = 5

// serverSetupPeerHostRecordConnection picks the authorized connection to the
// DNS primary: the saved default, or before it is saved, the one connection
// to the endpoint reviewed in the plan. "" when there is none.
func (p *Panel) serverSetupPeerHostRecordConnection(ctx context.Context, draft serverSetupDraft) (remoteDNSConnection, bool) {
	if id, err := p.defaultRemoteDNSConnectionID(ctx); err == nil {
		if c, err := p.readRemoteDNSConnection(ctx, id); err == nil && c.Status == "ready" && remoteDNSValidSecret(c.credential) {
			return c, true
		}
	}
	if draft.DNSPublisherEndpoint == "" {
		return remoteDNSConnection{}, false
	}
	rows, err := p.db.GetDB().QueryContext(ctx, `SELECT id FROM remote_dns_connections WHERE endpoint=? AND status='ready' LIMIT 2`, draft.DNSPublisherEndpoint)
	if err != nil {
		return remoteDNSConnection{}, false
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	if len(ids) != 1 {
		return remoteDNSConnection{}, false
	}
	c, err := p.readRemoteDNSConnection(ctx, ids[0])
	if err != nil || !remoteDNSValidSecret(c.credential) {
		return remoteDNSConnection{}, false
	}
	return c, true
}

func (p *Panel) readServerSetupPeerHostRecordOutcome(ctx context.Context, requestID string) (*serverSetupPeerHostRecordOutcome, error) {
	var raw string
	err := p.db.GetDB().QueryRowContext(ctx, `SELECT value FROM panel_settings WHERE key=?`, serverSetupPeerHostRecordKey(requestID)).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var outcome serverSetupPeerHostRecordOutcome
	if json.Unmarshal([]byte(raw), &outcome) != nil || outcome.RequestID != requestID {
		return nil, errors.New("the saved host record request outcome is unreadable")
	}
	return &outcome, nil
}

// requestServerSetupPeerHostRecord runs inside the single-flight setup runner
// while an access_dns step on a DNS secondary waits for public DNS. It sends
// the request at most once per step identity after a definite answer, and an
// unknown result again no sooner than two minutes later, five times at most;
// a read of the page never sends anything.
func (p *Panel) requestServerSetupPeerHostRecord(ctx context.Context, plan serverSetupPlan, step serverSetupExecutionStep) {
	if plan.Draft.DNSMode != setupDNSModeLocal || plan.Draft.DNSRole != "secondary" || !validServiceOperationID(step.RequestID) {
		return
	}
	address, ok := canonicalIPv4(step.Qualifier)
	if !ok {
		return
	}
	previous, err := p.readServerSetupPeerHostRecordOutcome(ctx, step.RequestID)
	if err != nil {
		log.Printf("setup peer host record %s: %v", step.Target, err)
		return
	}
	if previous != nil {
		if previous.Status != "unknown" && previous.Status != "none" {
			return
		}
		at, _ := time.Parse(time.RFC3339, previous.At)
		if previous.Attempts >= serverSetupPeerHostRecordMaxAttempts || time.Since(at) < serverSetupPeerHostRecordRetry {
			return
		}
	}
	outcome := serverSetupPeerHostRecordOutcome{RequestID: step.RequestID, Name: step.Target, Type: "A", Content: address, At: time.Now().UTC().Format(time.RFC3339), Primary: plan.Draft.PeerNS}
	if previous != nil {
		outcome.Attempts = previous.Attempts
	}
	connection, ok := p.serverSetupPeerHostRecordConnection(ctx, plan.Draft)
	if !ok {
		outcome.Status = "none"
	} else {
		outcome.Attempts++
		outcome.Endpoint, outcome.Connection = connection.Endpoint, connection.ID
		var answer remoteDNSHostRecordAnswer
		callCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		err := remoteDNSExchange(callCtx, connection.Endpoint, remoteDNSHostRecordPath, connection.ID+"."+connection.credential, remoteDNSHostRecordRequest{Name: outcome.Name, Type: outcome.Type, Content: outcome.Content}, &answer)
		cancel()
		var httpErr *remoteDNSHTTPError
		switch {
		case err == nil && answer.Applied && answer.ClientID == connection.ID && answer.Name == outcome.Name && answer.Type == outcome.Type && answer.Content == outcome.Content:
			outcome.Status, outcome.Result, outcome.Zone = "applied", answer.Result, answer.Zone
		case errors.As(err, &httpErr) && httpErr.Code != "" && httpErr.Code != errCodeRemoteDNSHostRecordPending && httpErr.Code != "REMOTE_DNS_AUTHORITY_NOT_READY":
			outcome.Status, outcome.Code = "refused", httpErr.Code
		case errors.As(err, &httpErr) && httpErr.Code == "" && (httpErr.StatusCode == http.StatusNotFound || httpErr.StatusCode == http.StatusMethodNotAllowed || httpErr.StatusCode == http.StatusUnauthorized || httpErr.StatusCode == http.StatusForbidden):
			// An older primary without this endpoint, or a revoked connection.
			outcome.Status = "unsupported"
		default:
			outcome.Status = "unknown"
			if errors.As(err, &httpErr) {
				outcome.Code = httpErr.Code
			}
		}
		p.auditSetupPeerHostRecord(ctx, "server.setup.peer_host_record."+outcome.Status)
	}
	encoded, _ := json.Marshal(outcome)
	if _, err := p.db.GetDB().ExecContext(ctx, `INSERT INTO panel_settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, serverSetupPeerHostRecordKey(step.RequestID), string(encoded)); err != nil {
		log.Printf("setup peer host record %s: save outcome: %v", step.Target, err)
	}
}

func (p *Panel) auditSetupPeerHostRecord(ctx context.Context, action string) {
	if _, err := p.db.GetDB().ExecContext(ctx, `INSERT INTO audit_logs (action, resource_type) VALUES (?, 'server_setup')`, action); err != nil {
		log.Printf("audit write failed (%s): %v", action, err)
	}
}

// serverSetupPeerHostRecordContext is the additive execution context field the
// guidance reads for a waiting access_dns step on a secondary: whether this
// server has a connection that can ask the primary, and what the request got.
type serverSetupPeerHostRecordContext struct {
	Channel bool                              `json:"channel"`
	Outcome *serverSetupPeerHostRecordOutcome `json:"outcome,omitempty"`
}

func (p *Panel) serverSetupPeerHostRecordContextFor(ctx context.Context, plan serverSetupPlan, step serverSetupExecutionStep) *serverSetupPeerHostRecordContext {
	if plan.Draft.DNSMode != setupDNSModeLocal || plan.Draft.DNSRole != "secondary" || step.Kind != "access_dns" {
		return nil
	}
	_, channel := p.serverSetupPeerHostRecordConnection(ctx, plan.Draft)
	result := &serverSetupPeerHostRecordContext{Channel: channel}
	if outcome, err := p.readServerSetupPeerHostRecordOutcome(ctx, step.RequestID); err == nil && outcome != nil {
		// The endpoint and connection identity stay on the server.
		copy := *outcome
		copy.Connection = ""
		result.Outcome = &copy
	}
	return result
}
