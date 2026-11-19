package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/netip"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/dnswire"
	"github.com/alicelik/celikpanel/internal/hostname"
	"github.com/alicelik/celikpanel/internal/transport"
)

// "Add this record" where the panel safely can (alpha.135, owner 2026-11-14:
// "beside Copy there should be Add"). The webmail address check names the
// record a missing name needs; where that record can be added depends on
// where the name's DNS lives:
//
//  1. A zone this panel hosts: its own record editor (webmailAddressZone).
//  2. The infrastructure zone of the paired main DNS server, when this server
//     is the pair's DNS secondary and holds a ready authorized remote DNS
//     connection: one confirmed request asks that server, over the existing
//     host-record channel (remote_dns_host_record.go, unchanged), to write the
//     A record with the pair's verified address and, only when it accepts it,
//     the AAAA record with the address the request comes from. The record
//     reaches this server by the ordinary zone transfer; replication never
//     depends on this request (D-022: optional, authorized record-management
//     automation). The receiver refuses whatever is outside its rule or would
//     overwrite the owner's record; such a refusal is said, never worked
//     around.
//  3. Anywhere else: the record to copy, and the name servers public DNS
//     names for the closest zone, when they can be read.
//
// The request carries an identity; the same identity is answered from what
// was stored and never sent again, and a status read (the check) never sends
// anything. The last request is kept in panel_settings (one row, no schema
// change). Administrator only (the /api/v1/service/ prefix).
//
// Eklenecek kayıt, adın DNS'inin yaşadığı yere göre: bu paneldeki bölge, eşli
// ana DNS sunucusunun altyapı bölgesi (yetkili bağlantıyla tek onaylı istek)
// ya da başka yer (kopyala + ad sunucuları). Aynı istek kimliği yeniden
// gönderilmez; denetim hiçbir şey göndermez.

const settingWebmailAddressDNSRecord = "webmail_address_dns_record"

const (
	errCodeWebmailDNSRecordNotOffered      = "WEBMAIL_DNS_RECORD_NOT_OFFERED"
	errCodeWebmailDNSRecordRequestConflict = "WEBMAIL_DNS_RECORD_REQUEST_CONFLICT"
)

// webmailAddressPeerZone: the other server that holds the name's zone (its
// panel host name) and the zone, "" when this server cannot know it (an older
// setup without the joined main domain; the other server then decides).
type webmailAddressPeerZone struct {
	Server string `json:"server"`
	Zone   string `json:"zone,omitempty"`
}

// webmailAddressManagedBy: the closest zone public DNS names name servers for.
type webmailAddressManagedBy struct {
	Zone        string   `json:"zone"`
	Nameservers []string `json:"nameservers"`
}

// One record of a request and what became of it. Outcome: added | present |
// saved (written there, its publication not confirmed yet) | refused (Code
// says why) | not_sent (Code: unreachable, skipped) | unknown.
type webmailAddressDNSRecordResult struct {
	webmailAddressDNSRecord
	Outcome string `json:"outcome"`
	Code    string `json:"code,omitempty"`
}

type webmailAddressDNSRecordRequest struct {
	RequestID string                          `json:"request_id"`
	Host      string                          `json:"host"`
	Server    string                          `json:"server"`
	Zone      string                          `json:"zone,omitempty"`
	State     string                          `json:"state"` // sending | done
	At        string                          `json:"at"`
	Records   []webmailAddressDNSRecordResult `json:"records"`
}

// One request at a time on this server (each is bounded: two exchanges of at
// most 30 seconds).
var webmailAddressDNSRecordMu sync.Mutex

// The pair state is read through this boundary so tests can stand in for the
// DNS engine's own guarded tables.
var webmailAddressPairState = func(p *Panel, ctx context.Context) (dnsEngineDBState, error) {
	return readDNSEngineDBState(ctx, p.db.GetDB())
}

// webmailAddressPeerChannel decides case 2 from what this server knows
// locally, without asking anyone: it is the DNS secondary of a pair whose
// verified address is this server's public IPv4 (the only A content the
// other server accepts), the name is strictly inside the main domain the
// join gave it (the other server's infrastructure zone) when it has one, and
// an authorized connection to the other server is ready.
func (p *Panel) webmailAddressPeerChannel(ctx context.Context, host, ipv4 string) (remoteDNSConnection, *webmailAddressPeerZone, bool) {
	state, err := webmailAddressPairState(p, ctx)
	if err != nil || state.Topology != transport.DNSTopologyPaired || state.PairRole != transport.DNSPairRoleSecondary {
		return remoteDNSConnection{}, nil, false
	}
	if local, ok := canonicalIPv4(state.LocalIP); !ok || ipv4 == "" || local != ipv4 {
		return remoteDNSConnection{}, nil, false
	}
	var draft serverSetupDraft
	if setup, err := p.loadServerSetup(ctx); err == nil {
		draft = setup.Draft
	}
	zone := ""
	if draft.DNSRole == transport.DNSPairRoleSecondary && draft.MainDomain != "" {
		if host == draft.MainDomain || !serverSetupDNSNameWithin(host, draft.MainDomain) {
			return remoteDNSConnection{}, nil, false
		}
		zone = draft.MainDomain
	}
	connection, ok := p.serverSetupPeerHostRecordConnection(ctx, draft)
	if !ok {
		return remoteDNSConnection{}, nil, false
	}
	return connection, &webmailAddressPeerZone{Server: serverSetupPublisherHost(connection.Endpoint), Zone: zone}, true
}

// webmailAddressNSLookup asks a public resolver for the NS records of name.
var webmailAddressNSLookup = func(ctx context.Context, name string) ([]string, error) {
	var last error
	for _, resolver := range []string{"1.1.1.1:53", "8.8.8.8:53"} {
		values, err := dnswire.QueryRecords(ctx, resolver, name, dnswire.TypeNS)
		var dnsErr *net.DNSError
		if err == nil || (errors.As(err, &dnsErr) && dnsErr.IsNotFound) {
			return values, err
		}
		last = err
	}
	return nil, last
}

// webmailAddressManagedByLookup: the closest enclosing zone that public DNS
// names name servers for, read only and bounded (at most four names, five
// seconds, never a top-level label). Nil when it cannot be read: the screen
// then says nothing rather than guess.
func webmailAddressManagedByLookup(ctx context.Context, host string) *webmailAddressManagedBy {
	lookupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	name := host
	for i := 0; i < 4 && strings.Contains(name, "."); i++ {
		values, err := webmailAddressNSLookup(lookupCtx, name)
		var dnsErr *net.DNSError
		switch {
		case err == nil && len(values) > 0:
			seen := map[string]bool{}
			servers := []string{}
			for _, value := range values {
				server, err := hostname.CanonicalFQDN(value)
				if err != nil || seen[server] {
					continue
				}
				seen[server] = true
				servers = append(servers, server)
			}
			if len(servers) == 0 || len(servers) > 8 {
				return nil
			}
			sort.Strings(servers)
			return &webmailAddressManagedBy{Zone: name, Nameservers: servers}
		case err == nil, errors.As(err, &dnsErr) && dnsErr.IsNotFound:
			name = name[strings.IndexByte(name, '.')+1:]
		default:
			return nil
		}
	}
	return nil
}

// webmailAddressDNSRecordOutcome reads the other server's answer to one host
// record request (D-024): a verified answer, a refusal by its typed rule, a
// request that never left this server, or a result that is not known.
func webmailAddressDNSRecordOutcome(err error, answer remoteDNSHostRecordAnswer, connectionID string, want remoteDNSHostRecordRequest) (outcome, code string) {
	var httpErr *remoteDNSHTTPError
	var exchangeErr *remoteDNSExchangeError
	switch {
	case err == nil:
		if answer.Applied && answer.ClientID == connectionID && answer.Name == want.Name && answer.Type == want.Type && answer.Content == want.Content {
			switch answer.Result {
			case "present":
				return "present", ""
			case "created", "updated":
				return "added", ""
			}
		}
		return "unknown", ""
	case errors.As(err, &httpErr):
		switch httpErr.Code {
		case errCodeRemoteDNSHostRecordOutsideZone:
			return "refused", "outside_zone"
		case errCodeRemoteDNSHostRecordConflict:
			return "refused", "taken"
		case errCodeRemoteDNSHostRecordAddress:
			return "refused", "address"
		case errCodeRemoteDNSHostRecordInvalid:
			return "refused", "invalid"
		case errCodeRemoteDNSHostRecordPending:
			return "saved", ""
		case "REMOTE_DNS_AUTHORITY_NOT_READY":
			return "refused", "not_ready"
		case "REMOTE_DNS_UNAUTHORIZED":
			return "refused", "unauthorized"
		case "license_required", errCodeLicenseStatusUnavailable, errCodeLicenseVerificationUnavailable:
			return "refused", "license"
		case "":
			switch httpErr.StatusCode {
			case http.StatusUnauthorized, http.StatusForbidden:
				return "refused", "unauthorized"
			case http.StatusNotFound, http.StatusMethodNotAllowed:
				return "refused", "unsupported"
			}
		}
		return "unknown", ""
	case errors.As(err, &exchangeErr) && !exchangeErr.Delivered:
		// The request did not go out whole: it could not be applied.
		return "not_sent", "unreachable"
	default:
		return "unknown", ""
	}
}

func (p *Panel) readWebmailAddressDNSRecordRequest(ctx context.Context) (webmailAddressDNSRecordRequest, bool) {
	var request webmailAddressDNSRecordRequest
	raw := p.setting(ctx, settingWebmailAddressDNSRecord)
	if raw == "" || json.Unmarshal([]byte(raw), &request) != nil || !validServiceOperationID(request.RequestID) {
		return webmailAddressDNSRecordRequest{}, false
	}
	return request, true
}

func (p *Panel) saveWebmailAddressDNSRecordRequest(ctx context.Context, request webmailAddressDNSRecordRequest) error {
	raw, err := json.Marshal(request)
	if err != nil {
		return err
	}
	return p.setSetting(ctx, settingWebmailAddressDNSRecord, string(raw))
}

func writeWebmailAddressDNSRecord(w http.ResponseWriter, request webmailAddressDNSRecordRequest) {
	_ = json.NewEncoder(w).Encode(map[string]any{"record": request})
}

// handleWebmailAddressDNSRecord: POST {host, request_id}. The server decides
// again what the check showed; nothing the browser sends but the name and the
// identity is used.
func (p *Panel) handleWebmailAddressDNSRecord(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		rejectRouteMethod(w, []string{http.MethodPost})
		return
	}
	var req struct {
		Host      string `json:"host"`
		RequestID string `json:"request_id"`
	}
	if err := decodeServiceOperationJSON(w, r, &req); err != nil {
		writeClientError(w, http.StatusBadRequest, "a host name is required")
		return
	}
	if !validServiceOperationID(req.RequestID) {
		writeClientError(w, http.StatusBadRequest, "invalid request_id")
		return
	}
	host, err := hostname.CanonicalFQDN(req.Host)
	if err != nil {
		writeClientError(w, http.StatusBadRequest, "a host name such as mail.example.com is required")
		return
	}
	// One request at a time; a browser's own replay of the same request waits
	// here and is then answered from what the first one stored.
	webmailAddressDNSRecordMu.Lock()
	defer webmailAddressDNSRecordMu.Unlock()
	ctx := r.Context()
	// The same request again (a lost answer): what was stored, never sent again.
	if last, ok := p.readWebmailAddressDNSRecordRequest(ctx); ok && last.RequestID == req.RequestID {
		if last.Host != host {
			writeCodedError(w, http.StatusConflict, errCodeWebmailDNSRecordRequestConflict, "This request identity was used for another name.", "")
			return
		}
		if last.State != "done" {
			// Interrupted (the panel restarted while it was sent): not known.
			for i := range last.Records {
				last.Records[i].Outcome, last.Records[i].Code = "unknown", ""
			}
		}
		writeWebmailAddressDNSRecord(w, last)
		return
	}
	notOffered := func(message string) {
		writeCodedError(w, http.StatusConflict, errCodeWebmailDNSRecordNotOffered, message, "")
	}
	ipv4, ipv6 := webmailAddressExpected()
	if ipv4 == "" {
		notOffered("This server's public IPv4 address could not be read; nothing was sent.")
		return
	}
	if p.webmailAddressZone(ctx, host) != nil {
		notOffered("This name's zone is hosted on this server; add the record through its own zone. Nothing was sent.")
		return
	}
	connection, peer, ok := p.webmailAddressPeerChannel(ctx, host, ipv4)
	if !ok {
		notOffered("This server cannot ask the other server for this record now (no ready DNS connection, or the name is not in its zone); nothing was sent.")
		return
	}
	request := webmailAddressDNSRecordRequest{RequestID: req.RequestID, Host: host, Server: peer.Server, Zone: peer.Zone, State: "sending", At: time.Now().UTC().Format(time.RFC3339)}
	name := host + "."
	if peer.Zone != "" {
		name = strings.TrimSuffix(host, "."+peer.Zone)
	}
	for _, record := range []struct{ typ, content string }{{"A", ipv4}, {"AAAA", ipv6}} {
		if record.content == "" {
			continue
		}
		request.Records = append(request.Records, webmailAddressDNSRecordResult{webmailAddressDNSRecord: webmailAddressDNSRecord{
			Type: record.typ, Name: name, Content: record.content, TTL: remoteDNSHostRecordTTL,
			Text: fmt.Sprintf("%s. %d IN %s %s", host, remoteDNSHostRecordTTL, record.typ, record.content),
		}})
	}
	if err := p.saveWebmailAddressDNSRecordRequest(ctx, request); err != nil {
		writeServerError(w, err)
		return
	}
	types := []string{}
	for _, record := range request.Records {
		types = append(types, record.Type)
	}
	p.auditDetailed(r, "webmail.address.dns_record.requested:"+host, "service", 0, map[string]any{"server": peer.Server, "zone": peer.Zone, "records": types})

	// The answer is kept even when the browser has gone away.
	sendCtx := context.WithoutCancel(ctx)
	bearer := connection.ID + "." + connection.credential
	for i := range request.Records {
		record := &request.Records[i]
		want := remoteDNSHostRecordRequest{Name: host, Type: record.Type, Content: record.Content}
		if record.Type == "AAAA" {
			// Only after the A record: the other server takes an IPv6
			// address only from the request that comes from it.
			if a := request.Records[0].Outcome; a != "added" && a != "present" && a != "saved" {
				record.Outcome, record.Code = "not_sent", "skipped"
				continue
			}
		}
		var answer remoteDNSHostRecordAnswer
		callCtx, cancel := context.WithTimeout(sendCtx, 30*time.Second)
		var err error
		if record.Type == "AAAA" {
			source, parseErr := netip.ParseAddr(record.Content)
			if parseErr != nil {
				cancel()
				record.Outcome, record.Code = "not_sent", "skipped"
				continue
			}
			err = remoteDNSExchangeFrom(callCtx, source, connection.Endpoint, remoteDNSHostRecordPath, bearer, want, &answer)
		} else {
			err = remoteDNSExchange(callCtx, connection.Endpoint, remoteDNSHostRecordPath, bearer, want, &answer)
		}
		cancel()
		record.Outcome, record.Code = webmailAddressDNSRecordOutcome(err, answer, connection.ID, want)
	}
	request.State = "done"
	if err := p.saveWebmailAddressDNSRecordRequest(sendCtx, request); err != nil {
		log.Printf("webmail address dns record %s: save outcome: %v", host, err)
	}
	result := request.Records[0].Outcome
	if result == "present" || result == "saved" {
		result = "added"
	}
	details := map[string]any{"server": peer.Server, "zone": peer.Zone}
	for _, record := range request.Records {
		details[strings.ToLower(record.Type)] = strings.TrimSuffix(record.Outcome+":"+record.Code, ":")
	}
	log.Printf("[dns] webmail address record %s asked of %s: %v", host, peer.Server, details)
	p.auditDetailed(r.WithContext(sendCtx), "webmail.address.dns_record."+result+":"+host, "service", 0, details)
	writeWebmailAddressDNSRecord(w, request)
}
