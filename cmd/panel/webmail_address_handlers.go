package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/hostname"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Webmail on its own address (alpha.110; owner 2026-10-26). Roundcube must
// not run on the panel's origin: a script running in it (its past XSS CVEs)
// could call /api/v1 with the signed-in administrator's session. The owner
// sets a per-server address (any host name, for example mail.celikhost.com;
// no server prefixes required), the panel checks read only that public DNS
// answers this server's addresses, and one confirmed operation has the
// Agent obtain the name's certificate through certbot and write a native
// nginx server for it. The address is recorded in panel_settings (no schema
// change). Afterwards the panel's /webmail/ only redirects there; it serves
// none of Roundcube's content. Webmail keeps answering when the panel is
// stopped (D-022): nginx, PHP-FPM, certbot's timer and Roundcube's own
// configuration are the operating system's.
//
// Web postası kendi adresinde: sahip adı yazar, panel DNS'i salt okur denetler,
// tek onaylı işlem sertifikayı alır ve nginx sunucusunu yazar. Panelin
// /webmail/ yolu yalnız oraya yönlendirir.

const (
	webmailAddressPath       = "/api/v1/service/roundcube/address"
	webmailAddressCheckPath  = webmailAddressPath + "/check"
	webmailAddressRemovePath = webmailAddressPath + "/remove"

	settingWebmailAddress = "webmail_address"

	serviceOperationKindWebmailAddress       = "webmail_address"
	serviceOperationKindWebmailAddressRemove = "webmail_address_remove"

	// The states of the read-only check, in the screen's words.
	webmailAddressReady     = "ready"     // public DNS answers only this server
	webmailAddressNotFound  = "not_found" // no address record yet (a prerequisite)
	webmailAddressElsewhere = "elsewhere" // DNS answers another server (refused)
	webmailAddressUnknown   = "unknown"   // DNS could not be asked
	webmailAddressRefused   = "refused"   // the name cannot be webmail's address
)

type webmailAddressSetting struct {
	Host      string `json:"host"`
	EnabledAt string `json:"enabled_at"`
}

// webmailAddress is the enabled address of this server, if any.
func (p *Panel) webmailAddress(ctx context.Context) (webmailAddressSetting, bool) {
	if p == nil || p.db == nil {
		return webmailAddressSetting{}, false
	}
	raw := p.setting(ctx, settingWebmailAddress)
	if raw == "" {
		return webmailAddressSetting{}, false
	}
	var value webmailAddressSetting
	if json.Unmarshal([]byte(raw), &value) != nil {
		return webmailAddressSetting{}, false
	}
	host, err := hostname.CanonicalFQDN(value.Host)
	if err != nil || host != value.Host {
		return webmailAddressSetting{}, false
	}
	return value, true
}

func (p *Panel) saveWebmailAddress(ctx context.Context, host string) error {
	raw, err := json.Marshal(webmailAddressSetting{Host: host, EnabledAt: time.Now().UTC().Format(time.RFC3339)})
	if err != nil {
		return err
	}
	return p.setSetting(ctx, settingWebmailAddress, string(raw))
}

func (p *Panel) clearWebmailAddress(ctx context.Context) error {
	_, err := p.db.GetDB().ExecContext(ctx, `DELETE FROM panel_settings WHERE key = ?`, settingWebmailAddress)
	return err
}

func webmailAddressURL(host string) string { return "https://" + host + "/" }

// webmailLink is the address mailbox users open, when webmail has its own
// address and Roundcube answers; "" otherwise.
func (p *Panel) webmailLink(ctx context.Context) string {
	address, ok := p.webmailAddress(ctx)
	if !ok || !p.webmailAvailable(ctx) {
		return ""
	}
	return webmailAddressURL(address.Host)
}

// handleWebmailEntry is /webmail/ on the panel: a redirect to webmail's own
// address once it has one, otherwise a short page saying where it is set.
// Roundcube's content is never served on the panel's origin, and this page
// carries the panel's strict policy (no script at all).
func (p *Panel) handleWebmailEntry(w http.ResponseWriter, r *http.Request) {
	if !webmailRequestLimiter.allow(clientIP(r)) {
		w.Header().Set("Retry-After", "60")
		http.Error(w, "Too many webmail requests.", http.StatusTooManyRequests)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		rejectRouteMethod(w, []string{http.MethodGet, http.MethodHead})
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if address, ok := p.webmailAddress(r.Context()); ok {
		http.Redirect(w, r, webmailAddressURL(address.Host), http.StatusFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, webmailNoAddressPage)
}

const webmailNoAddressPage = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>Webmail</title>
<style>body{font:16px/1.5 system-ui,sans-serif;max-width:36rem;margin:3rem auto;padding:0 1rem;color:#1f2328;background:#fff}h1{font-size:1.25rem}p{margin:.5rem 0}hr{border:0;border-top:1px solid #d0d7de;margin:1.5rem 0}@media (prefers-color-scheme:dark){body{color:#e6edf3;background:#0d1117}hr{border-top-color:#30363d}}</style>
</head><body>
<h1>Webmail has no address yet</h1>
<p>Webmail opens on its own address, not inside the control panel. The server administrator sets it in CelikPanel: Components, Webmail (Roundcube), Manage, Webmail address. Then open that address, for example https://mail.example.com/.</p>
<hr>
<h1 lang="tr">Web postasının henüz adresi yok</h1>
<p lang="tr">Web postası denetim panelinin içinde değil, kendi adresinde açılır. Sunucu yöneticisi bunu CelikPanel'de ayarlar: Bileşenler, Web postası (Roundcube), Yönet, Web postası adresi. Sonra o adresi açın; örneğin https://mail.example.com/.</p>
</body></html>
`

// webmailAddressCheck is the read-only check before enabling: what public
// DNS answers, what it should answer, the record to add (and whether this
// panel hosts the zone so it can add it after confirmation), the other
// server when DNS points there, and the firewall's ports 80 and 443.
type webmailAddressCheck struct {
	Host     string                     `json:"host"`
	State    string                     `json:"state"`
	Code     string                     `json:"code,omitempty"`
	Message  string                     `json:"message"`
	Expected []string                   `json:"expected"`
	Answered []string                   `json:"answered,omitempty"`
	Peer     *webmailAddressPeer        `json:"peer,omitempty"`
	Records  []webmailAddressDNSRecord  `json:"records,omitempty"`
	Zone     *webmailAddressZone        `json:"zone,omitempty"`
	Firewall webmailAddressFirewall     `json:"firewall"`
	Current  string                     `json:"current,omitempty"`
	Ready    bool                       `json:"ready"`
	Warnings []string                   `json:"warnings,omitempty"`
	Owner    *webmailAddressOwnerAction `json:"owner_action,omitempty"`
}

type webmailAddressPeer struct {
	Name    string `json:"name,omitempty"`
	Address string `json:"address"`
}

// webmailAddressDNSRecord is one record to add, as the zone's own record
// editor takes it (Name relative to the zone, "@" for the zone itself).
type webmailAddressDNSRecord struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	TTL     int    `json:"ttl"`
	Text    string `json:"text"`
}

// webmailAddressZone: the zone is hosted by this panel and the records can
// be added through its own DNS record editor (RecordsURL), after the
// administrator confirms.
type webmailAddressZone struct {
	Name       string `json:"name"`
	RecordsURL string `json:"records_url"`
}

type webmailAddressFirewall struct {
	// State: "open" (80 and 443 are allowed), "closed" (Closed names the
	// ports), "off" (the firewall is not on; nothing filters them) or
	// "unknown" (it could not be read).
	State  string `json:"state"`
	Closed []int  `json:"closed,omitempty"`
}

// webmailAddressOwnerAction is who must act and the next step when the check
// is not ready.
type webmailAddressOwnerAction struct {
	Who  string `json:"who"`
	Next string `json:"next"`
}

var webmailAddressResolvers = func() []hostResolver { return setupDNSPublicResolvers() }

// webmailAddressExpected is this server's public addresses.
func webmailAddressExpected() (ipv4, ipv6 string) {
	ipv4, _ = canonicalIPv4(serverPrimaryIP())
	if ip := net.ParseIP(serverPrimaryIPv6()); ip != nil && ip.To4() == nil {
		ipv6 = ip.String()
	}
	return ipv4, ipv6
}

// panelOwnNames are the names this panel itself answers to; webmail never
// shares one (cookies do not separate ports, so the panel's session cookie
// would reach Roundcube there).
func (p *Panel) panelOwnNames(r *http.Request) []string {
	var names []string
	if r != nil {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		if canonical, err := hostname.CanonicalFQDN(host); err == nil {
			names = append(names, canonical)
		}
	}
	info := currentPanelCert()
	for _, raw := range append(append([]string(nil), info.DNSNames...), info.Subject) {
		if canonical, err := hostname.CanonicalFQDN(strings.TrimSpace(raw)); err == nil {
			names = append(names, canonical)
		}
	}
	return names
}

func (p *Panel) checkWebmailAddress(ctx context.Context, r *http.Request, raw string) webmailAddressCheck {
	check := webmailAddressCheck{Expected: []string{}}
	if current, ok := p.webmailAddress(ctx); ok {
		check.Current = current.Host
	}
	host, err := hostname.CanonicalFQDN(raw)
	if err != nil {
		check.Host = strings.TrimSpace(raw)
		check.State, check.Code = webmailAddressRefused, "invalid"
		check.Message = "Type a host name such as mail.example.com."
		check.Owner = &webmailAddressOwnerAction{Who: "administrator", Next: "Type the full name webmail should open at, for example mail.example.com."}
		return check
	}
	check.Host = host
	if slices.Contains(p.panelOwnNames(r), host) {
		check.State, check.Code = webmailAddressRefused, "panel_address"
		check.Message = "This is the control panel's own name. Webmail needs a name of its own, because a browser sends the panel's sign-in cookie to every port of the same name."
		check.Owner = &webmailAddressOwnerAction{Who: "administrator", Next: "Choose another name, for example mail." + parentDomain(host) + "."}
		return check
	}
	ipv4, ipv6 := webmailAddressExpected()
	if ipv4 != "" {
		check.Expected = append(check.Expected, ipv4)
		check.Records = append(check.Records, webmailAddressDNSRecord{Type: "A", Content: ipv4, TTL: 3600})
	}
	if ipv6 != "" {
		check.Expected = append(check.Expected, ipv6)
		check.Records = append(check.Records, webmailAddressDNSRecord{Type: "AAAA", Content: ipv6, TTL: 3600})
	}
	if ipv4 == "" {
		check.State, check.Code = webmailAddressUnknown, "server_address_unknown"
		check.Message = "This server's public IPv4 address could not be read, so the DNS answer cannot be compared."
		check.Owner = &webmailAddressOwnerAction{Who: "server owner", Next: "Check this server's network connection; the check runs again when you press Check."}
		return check
	}
	zone := p.webmailAddressZone(ctx, host)
	for i := range check.Records {
		name := "@"
		if zone != nil && host != zone.Name {
			name = strings.TrimSuffix(host, "."+zone.Name)
		} else if zone == nil {
			name = host + "."
		}
		check.Records[i].Name = name
		check.Records[i].Text = fmt.Sprintf("%s. %d IN %s %s", host, check.Records[i].TTL, check.Records[i].Type, check.Records[i].Content)
	}
	check.Zone = zone
	check.Firewall = p.webmailAddressFirewall()

	answered, lookupErr := webmailAddressLookup(ctx, host)
	check.Answered = answered
	var notFound *net.DNSError
	switch {
	case lookupErr != nil && errors.As(lookupErr, &notFound) && notFound.IsNotFound:
		check.State, check.Code = webmailAddressNotFound, "no_record"
		check.Message = fmt.Sprintf("Public DNS has no address record for %s yet.", host)
		if zone != nil {
			check.Owner = &webmailAddressOwnerAction{Who: "administrator", Next: "Add the record below to the zone " + zone.Name + " hosted here (it is shown before it is added), then press Check."}
		} else {
			check.Owner = &webmailAddressOwnerAction{Who: "whoever manages this name's DNS", Next: "Add the record below where this name's DNS is managed, then press Check. A new record can take a few minutes to be seen."}
		}
		return check
	case lookupErr != nil:
		check.State, check.Code = webmailAddressUnknown, "dns_unreachable"
		check.Message = fmt.Sprintf("Public DNS could not be asked about %s (%v), so whether it points here is not known.", host, boundedWebmailReason(lookupErr.Error()))
		check.Owner = &webmailAddressOwnerAction{Who: "nobody yet", Next: "Press Check again in a minute; nothing was changed."}
		return check
	}
	var foreign []string
	for _, address := range answered {
		if address != ipv4 && (ipv6 == "" || address != ipv6) {
			foreign = append(foreign, address)
		}
	}
	if len(foreign) > 0 || !slices.Contains(answered, ipv4) {
		check.State, check.Code = webmailAddressElsewhere, "points_elsewhere"
		check.Message = fmt.Sprintf("Public DNS answers %s for %s, but this server is %s. Webmail is refused here so that it answers from one server only.", strings.Join(answered, ", "), host, strings.Join(check.Expected, ", "))
		if peer := p.webmailAddressPeer(ctx, foreign); peer != nil {
			check.Peer = peer
			check.Code = "points_to_peer"
			name := peer.Address
			if peer.Name != "" {
				name = peer.Name + " (" + peer.Address + ")"
			}
			check.Message = fmt.Sprintf("Public DNS sends %s to the other server %s. The same webmail address can answer from one server only; if that server already serves webmail on this name, choose another name here.", host, name)
		}
		check.Owner = &webmailAddressOwnerAction{Who: "administrator", Next: "Change the name's record to " + strings.Join(check.Expected, ", ") + " if webmail should open on this server, or choose another name."}
		return check
	}
	check.State = webmailAddressReady
	check.Message = fmt.Sprintf("Public DNS answers %s for %s: this server.", strings.Join(answered, ", "), host)
	switch check.Firewall.State {
	case "closed":
		check.Code = "firewall_closed"
		check.Message += fmt.Sprintf(" The firewall does not allow port %s, which certbot and browsers need.", joinPorts(check.Firewall.Closed))
		check.Owner = &webmailAddressOwnerAction{Who: "administrator", Next: "Allow ports 80 and 443 on Server, Firewall (CelikPanel never opens them by itself), then press Check."}
		return check
	case "unknown":
		check.Warnings = append(check.Warnings, "The firewall's state could not be read; if ports 80 and 443 are closed, certbot's check will fail and nothing will change.")
	}
	check.Ready = true
	return check
}

func joinPorts(ports []int) string {
	parts := make([]string, 0, len(ports))
	for _, port := range ports {
		parts = append(parts, strconv.Itoa(port))
	}
	return strings.Join(parts, " and ")
}

func parentDomain(host string) string {
	if dot := strings.IndexByte(host, '.'); dot >= 0 && strings.Contains(host[dot+1:], ".") {
		return host[dot+1:]
	}
	return host
}

// webmailAddressLookup asks the fixed public resolvers; every one must
// answer the same set (a resolver that cannot be asked makes the answer
// unknown, never ready).
func webmailAddressLookup(ctx context.Context, host string) ([]string, error) {
	resolvers := webmailAddressResolvers()
	if len(resolvers) == 0 {
		return nil, errors.New("no public resolver is configured")
	}
	lookupCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	union := map[string]bool{}
	var notFound error
	for _, resolver := range resolvers {
		addresses, err := resolver.LookupHost(lookupCtx, host)
		if err != nil {
			var dnsErr *net.DNSError
			if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
				notFound = err
				continue
			}
			return nil, err
		}
		for _, address := range addresses {
			if ip := net.ParseIP(address); ip != nil {
				union[ip.String()] = true
			}
		}
	}
	if notFound != nil {
		// Not every public resolver sees the record yet: certbot's check
		// could reach one that does not, so it is not ready.
		return nil, notFound
	}
	if len(union) == 0 {
		return nil, &net.DNSError{Err: "no address records", Name: host, IsNotFound: true}
	}
	out := make([]string, 0, len(union))
	for address := range union {
		out = append(out, address)
	}
	sort.Strings(out)
	return out, nil
}

// webmailAddressZone is the zone this panel hosts for host and whose record
// editor can add the record (an infrastructure zone, or a domain whose DNS is
// managed here); nil otherwise.
func (p *Panel) webmailAddressZone(ctx context.Context, host string) *webmailAddressZone {
	rows, err := p.db.GetDB().QueryContext(ctx, `SELECT name FROM pdns_domains`)
	if err != nil {
		return nil
	}
	best := ""
	for rows.Next() {
		var name string
		if rows.Scan(&name) != nil {
			continue
		}
		name = strings.ToLower(strings.TrimSuffix(name, "."))
		if (host == name || strings.HasSuffix(host, "."+name)) && len(name) > len(best) {
			best = name
		}
	}
	rows.Close()
	if best == "" {
		return nil
	}
	if infrastructure, err := p.isInfrastructureDNSZone(ctx, best); err == nil && infrastructure {
		return &webmailAddressZone{Name: best, RecordsURL: infrastructureDNSZonesPath + "/" + best + "/records"}
	}
	var domainID int
	var mode sql.NullString
	if err := p.db.GetDB().QueryRowContext(ctx, `SELECT id, dns_management FROM domains WHERE name = ?`, best).Scan(&domainID, &mode); err != nil {
		return nil
	}
	if mode.Valid && mode.String != "" && mode.String != setupDNSModeLocal {
		return nil
	}
	return &webmailAddressZone{Name: best, RecordsURL: fmt.Sprintf("/api/v1/domains/%d/dns/records", domainID)}
}

// webmailAddressPeer names the other DNS server when DNS answers its
// address.
func (p *Panel) webmailAddressPeer(ctx context.Context, answered []string) *webmailAddressPeer {
	peerIP, _ := canonicalIPv4(p.setting(ctx, settingDNSPeerIP))
	if peerIP == "" || !slices.Contains(answered, peerIP) {
		return nil
	}
	return &webmailAddressPeer{Name: canonicalDNSName(p.setting(ctx, settingDNSPeerNS)), Address: peerIP}
}

var webmailAddressFirewallStatus = func(p *Panel) (FirewallStatusResp, error) { return p.readFirewallStatus() }

func (p *Panel) webmailAddressFirewall() webmailAddressFirewall {
	status, err := webmailAddressFirewallStatus(p)
	if err != nil {
		return webmailAddressFirewall{State: "unknown"}
	}
	if !status.EngineAvailable || !status.Enabled {
		return webmailAddressFirewall{State: "off"}
	}
	var closed []int
	for _, port := range []int{80, 443} {
		if !slices.Contains(status.TCPPorts, port) {
			closed = append(closed, port)
		}
	}
	if len(closed) > 0 {
		return webmailAddressFirewall{State: "closed", Closed: closed}
	}
	return webmailAddressFirewall{State: "open"}
}

var readWebmailAddressStatusFromAgent = func(p *Panel, ctx context.Context, host string, prove bool) (transport.WebmailAddressStatusResponse, error) {
	callCtx, cancel := context.WithTimeout(ctx, roundcubeUpdateReadBound)
	defer cancel()
	var response transport.WebmailAddressStatusResponse
	err := p.callAgentContext(callCtx, "Agent.WebmailAddressStatusV1", &transport.WebmailAddressStatusRequest{Host: host, Prove: prove}, &response)
	return response, err
}

var enableWebmailAddressThroughAgent = func(p *Panel, ctx context.Context, request transport.WebmailAddressEnableRequest) (transport.WebmailAddressResponse, error) {
	var response transport.WebmailAddressResponse
	err := p.withStandaloneAgentMutation(ctx, serviceOperationKindWebmailAddress, "roundcube", request.Host, func(callCtx context.Context, binding agentMutationBinding) error {
		request.ServiceMutationBinding = binding
		return p.callAgentContext(callCtx, "Agent.EnableWebmailAddressV1", &request, &response)
	})
	return response, err
}

var removeWebmailAddressThroughAgent = func(p *Panel, ctx context.Context, request transport.WebmailAddressRemoveRequest) (transport.WebmailAddressRemoveResponse, error) {
	var response transport.WebmailAddressRemoveResponse
	err := p.withStandaloneAgentMutation(ctx, serviceOperationKindWebmailAddressRemove, "roundcube", request.Host, func(callCtx context.Context, binding agentMutationBinding) error {
		request.ServiceMutationBinding = binding
		return p.callAgentContext(callCtx, "Agent.RemoveWebmailAddressV1", &request, &response)
	})
	return response, err
}

// certificateEmailFor is the signed-in administrator's own email, for
// certbot's account (empty: certbot keeps its existing account).
func (p *Panel) certificateEmailFor(ctx context.Context, userID int) string {
	if userID <= 0 {
		return ""
	}
	var email sql.NullString
	if err := p.db.GetDB().QueryRowContext(ctx, `SELECT email FROM users WHERE id = ?`, userID).Scan(&email); err != nil || !email.Valid {
		return ""
	}
	value := strings.TrimSpace(email.String)
	if at := strings.LastIndexByte(value, '@'); at <= 0 || !strings.Contains(value[at:], ".") || strings.ContainsAny(value, " \t\r\n") {
		return ""
	}
	return value
}

// runWebmailAddressEnable is the durable operation's work.
func (p *Panel) runWebmailAddressEnable(ctx context.Context, host, email, replace string, outcome *transport.WebmailAddressResponse) (serviceOperationResult, *serviceOperationFailure) {
	result := serviceOperationResult{"success": false, "host": host}
	if replace != "" {
		result["replace"] = replace
	}
	response, err := enableWebmailAddressThroughAgent(p, ctx, transport.WebmailAddressEnableRequest{Host: host, Email: email, Replace: replace})
	*outcome = response
	if err != nil {
		result["outcome"] = "unknown"
		if failure := platformServiceOperationFailure(err); failure != nil {
			return result, failure
		}
		return result, operationFailure("webmail_address_unknown",
			"The server agent's answer did not arrive, so whether webmail's address was set up is not known. Open this card again: it reads the server's state.", err)
	}
	if response.CertificateExpires != "" {
		result["certificate_expires"] = response.CertificateExpires
	}
	if response.Warning != "" {
		result["warning"] = boundedWebmailReason(response.Warning)
	}
	if response.RoundcubeSettings != "" {
		result["roundcube_settings"] = response.RoundcubeSettings
	}
	if !response.Enabled || !response.Proved {
		result["code"] = response.ErrorCode
		result["reason"] = boundedWebmailReason(response.Error)
		result["rolled_back"] = response.RolledBack
		result["outcome"] = "not_enabled"
		if response.ErrorCode == "ROLLBACK_FAILED" {
			result["outcome"] = "rollback_failed"
		}
		message := "Webmail's address was not set up."
		if response.Error == "" {
			response.Error = "the server agent did not confirm the address"
		}
		return result, operationFailure("webmail_address_failed", message, errors.New(response.Error))
	}
	if err := p.saveWebmailAddress(ctx, host); err != nil {
		result["outcome"] = "not_recorded"
		return result, operationFailure("webmail_address_not_recorded",
			"Webmail answers on its address, but the panel could not record it; press Set up again (it is idempotent).", err)
	}
	result["success"], result["outcome"], result["url"] = true, "enabled", webmailAddressURL(host)
	return result, nil
}

func (p *Panel) runWebmailAddressRemove(ctx context.Context, host string, removeCertificate bool, outcome *transport.WebmailAddressRemoveResponse) (serviceOperationResult, *serviceOperationFailure) {
	result := serviceOperationResult{"success": false, "host": host, "remove_certificate": removeCertificate}
	response, err := removeWebmailAddressThroughAgent(p, ctx, transport.WebmailAddressRemoveRequest{Host: host, RemoveCertificate: removeCertificate})
	*outcome = response
	if err != nil {
		result["outcome"] = "unknown"
		if failure := platformServiceOperationFailure(err); failure != nil {
			return result, failure
		}
		return result, operationFailure("webmail_address_unknown",
			"The server agent's answer did not arrive, so whether the address was removed is not known. Open this card again: it reads the server's state.", err)
	}
	if response.Warning != "" {
		result["warning"] = boundedWebmailReason(response.Warning)
	}
	if !response.Removed {
		result["code"], result["reason"], result["outcome"] = response.ErrorCode, boundedWebmailReason(response.Error), "not_removed"
		return result, operationFailure("webmail_address_remove_failed", "Webmail's address was not removed.", errors.New(response.Error))
	}
	if current, ok := p.webmailAddress(ctx); ok && current.Host == host {
		if err := p.clearWebmailAddress(ctx); err != nil {
			return result, operationFailure("webmail_address_not_recorded", "The address was removed from the web server, but the panel could not record it.", err)
		}
	}
	result["success"], result["outcome"], result["certificate_removed"] = true, "removed", response.CertificateRemoved
	return result, nil
}

// reconcileInterruptedWebmailAddress decides, read only, what an operation
// interrupted by a panel restart left: the server's own state decides and
// nothing is started again by itself.
func (p *Panel) reconcileInterruptedWebmailAddress(ctx context.Context, op serviceOperation) (serviceOperationResult, *serviceOperationFailure) {
	host := op.PackageName
	result := serviceOperationResult{"success": false, "host": host, "recovered": true}
	status, err := readWebmailAddressStatusFromAgent(p, ctx, host, op.Kind == serviceOperationKindWebmailAddress)
	if err != nil {
		result["outcome"] = "unknown"
		return result, operationFailure("webmail_address_unknown", "The panel restarted during the operation and the server's state could not be read; open the webmail address card again.", err)
	}
	if op.Kind == serviceOperationKindWebmailAddressRemove {
		if status.Present {
			result["outcome"] = "interrupted"
			return result, operationFailure("webmail_address_interrupted", "The panel restarted before the address was removed; webmail still answers there. Press Remove again.", nil)
		}
		if current, ok := p.webmailAddress(ctx); ok && current.Host == host {
			_ = p.clearWebmailAddress(ctx)
		}
		result["success"], result["outcome"] = true, "removed"
		return result, nil
	}
	if !status.Present || !status.Managed || !status.Proved {
		result["outcome"] = "interrupted"
		result["reason"] = boundedWebmailReason(status.ProofError)
		return result, operationFailure("webmail_address_interrupted", "The panel restarted before webmail's address was proved; press Set up again.", nil)
	}
	if err := p.saveWebmailAddress(ctx, host); err != nil {
		return result, operationFailure("webmail_address_not_recorded", "Webmail answers on its address, but the panel could not record it; press Set up again.", err)
	}
	// A change interrupted after the new address proved: the previous
	// address may still answer; it is said, never removed by itself.
	var data struct {
		Replace string `json:"replace"`
	}
	if json.Unmarshal([]byte(op.OperationData), &data) == nil && data.Replace != "" && data.Replace != host {
		result["replace"] = data.Replace
		if old, err := readWebmailAddressStatusFromAgent(p, ctx, data.Replace, false); err != nil || old.Present {
			result["warning"] = "the previous address " + data.Replace + " may still answer; remove it from the webmail address card"
		}
	}
	result["success"], result["outcome"], result["url"] = true, "enabled", webmailAddressURL(host)
	return result, nil
}

func (p *Panel) webmailAddressAuditWriter(remove bool, host string, details func() map[string]any) serviceOperationAuditWriter {
	return func(ctx context.Context, actor serviceOperationActor, written string) {
		operation := serviceOperationKindWebmailAddress
		if remove {
			operation = serviceOperationKindWebmailAddressRemove
		}
		values := map[string]any{"component": "roundcube", "operation": operation, "host": host}
		for key, value := range details() {
			values[key] = value
		}
		reason := ""
		if cut := strings.Index(written, " — "); cut >= 0 {
			reason = written[cut:]
		}
		failed := strings.Contains(written, ".failed")
		var action string
		switch {
		case remove && failed:
			action = "webmail.address.remove.failed:" + host + reason
		case remove:
			action = "webmail.address.remove:" + host
		case failed:
			action = "webmail.address.failed:" + host + reason
		default:
			action = "webmail.address:" + host
		}
		p.auditServiceOperation(withAuditDetails(ctx, values), actor, action)
	}
}

// lastWebmailAddressOperation is the newest enable or remove operation.
func (p *Panel) lastWebmailAddressOperation(ctx context.Context) (*serviceOperation, error) {
	op, err := scanServiceOperation(p.db.GetDB().QueryRowContext(ctx,
		serviceOperationSelect+` WHERE kind IN (?, ?) ORDER BY started_at DESC LIMIT 1`,
		serviceOperationKindWebmailAddress, serviceOperationKindWebmailAddressRemove))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &op, nil
}

// handleWebmailAddress: GET the address, its suggestion and the server's
// state; POST enable (one confirmed operation).
func (p *Panel) handleWebmailAddress(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	switch r.Method {
	case http.MethodGet:
		ctx := r.Context()
		body := map[string]any{"address": nil}
		ipv4, ipv6 := webmailAddressExpected()
		expected := []string{}
		for _, address := range []string{ipv4, ipv6} {
			if address != "" {
				expected = append(expected, address)
			}
		}
		body["expected"] = expected
		if suggestion := p.mailHostnameIdentity(ctx).Hostname; suggestion != "" && !slices.Contains(p.panelOwnNames(r), suggestion) {
			body["suggestion"] = suggestion
		}
		last, err := p.lastWebmailAddressOperation(ctx)
		if err != nil {
			log.Printf("webmail address: last operation: %v", err)
		}
		body["last_operation"] = last
		address, ok := p.webmailAddress(ctx)
		host := ""
		if ok {
			host = address.Host
			body["address"] = map[string]any{"host": address.Host, "url": webmailAddressURL(address.Host), "enabled_at": address.EnabledAt}
		}
		status, err := readWebmailAddressStatusFromAgent(p, ctx, host, ok)
		if err != nil {
			log.Printf("webmail address status: %v", err)
			body["status_unknown"] = true
		} else {
			body["status"] = status
		}
		_ = json.NewEncoder(w).Encode(body)
	case http.MethodPost:
		p.startWebmailAddressEnable(w, r)
	default:
		rejectRouteMethod(w, []string{http.MethodGet, http.MethodPost})
	}
}

func (p *Panel) handleWebmailAddressCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		rejectRouteMethod(w, []string{http.MethodPost})
		return
	}
	var req struct {
		Host string `json:"host"`
	}
	if err := decodeServiceOperationJSON(w, r, &req); err != nil {
		writeClientError(w, http.StatusBadRequest, "a host name is required")
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"check": p.checkWebmailAddress(r.Context(), r, req.Host)})
}

func writeWebmailAddressNotReady(w http.ResponseWriter, check webmailAddressCheck) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusConflict)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": check.Message, "code": "WEBMAIL_ADDRESS_NOT_READY", "check": check,
	})
}

func (p *Panel) startWebmailAddressEnable(w http.ResponseWriter, r *http.Request) {
	if !p.requireSubsystemOperational(w, degradedSubsystemServiceOperations) {
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
	// The same request again (a reload, a lost answer) is the same
	// operation; it never starts a second one.
	existing, found, err := p.idempotentServiceOperation(r.Context(), req.RequestID, serviceOperationKindWebmailAddress, "roundcube", host)
	if err != nil {
		if errors.Is(err, errServiceOperationRequestConflict) {
			writeServiceOperationRequestConflict(w)
			return
		}
		writeServerError(w, err)
		return
	}
	if found {
		writeAcceptedServiceOperation(w, existing)
		return
	}
	// The server checks again what the screen showed: DNS must answer only
	// this server and the firewall must allow 80 and 443.
	check := p.checkWebmailAddress(r.Context(), r, host)
	if !check.Ready {
		writeWebmailAddressNotReady(w, check)
		return
	}
	release, busy := p.beginServiceMutation(w, r)
	if busy {
		return
	}
	releaseInHandler := true
	defer func() {
		if releaseInHandler {
			release()
		}
	}()
	actor := captureServiceOperationActor(r)
	replace := ""
	if check.Current != "" && check.Current != host {
		replace = check.Current
	}
	data, _ := json.Marshal(map[string]string{"replace": replace})
	op, err := p.createServiceOperationRequestWithState(r.Context(), serviceOperationKindWebmailAddress, "roundcube", host, req.RequestID, actor, "queued", string(data))
	switch {
	case errors.Is(err, errServiceOperationBusy):
		writeServiceOperationBusy(w)
		return
	case errors.Is(err, errServiceOperationReplay):
		writeAcceptedServiceOperation(w, op)
		return
	case errors.Is(err, errServiceOperationRequestConflict):
		writeServiceOperationRequestConflict(w)
		return
	case err != nil:
		writeServerError(w, err)
		return
	}
	email := p.certificateEmailFor(r.Context(), actor.UserID)
	outcome := &transport.WebmailAddressResponse{}
	p.launchServiceOperationWithAudit(
		op, actor, "enabling",
		"webmail.address:"+host, "webmail.address.failed:"+host,
		release,
		func(ctx context.Context, _ func(string) error) (serviceOperationResult, *serviceOperationFailure) {
			return p.runWebmailAddressEnable(ctx, host, email, replace, outcome)
		},
		p.webmailAddressAuditWriter(false, host, func() map[string]any {
			return map[string]any{"replace": replace, "certificate_issued": outcome.CertificateIssued, "rolled_back": outcome.RolledBack, "code": outcome.ErrorCode}
		}),
	)
	releaseInHandler = false
	writeAcceptedServiceOperation(w, op)
}

func (p *Panel) handleWebmailAddressRemove(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		rejectRouteMethod(w, []string{http.MethodPost})
		return
	}
	if !p.requireSubsystemOperational(w, degradedSubsystemServiceOperations) {
		return
	}
	var req struct {
		Host              string `json:"host"`
		RemoveCertificate bool   `json:"remove_certificate"`
		RequestID         string `json:"request_id"`
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
		writeClientError(w, http.StatusBadRequest, "a host name is required")
		return
	}
	existing, found, err := p.idempotentServiceOperation(r.Context(), req.RequestID, serviceOperationKindWebmailAddressRemove, "roundcube", host)
	if err != nil {
		if errors.Is(err, errServiceOperationRequestConflict) {
			writeServiceOperationRequestConflict(w)
			return
		}
		writeServerError(w, err)
		return
	}
	if found {
		writeAcceptedServiceOperation(w, existing)
		return
	}
	release, busy := p.beginServiceMutation(w, r)
	if busy {
		return
	}
	releaseInHandler := true
	defer func() {
		if releaseInHandler {
			release()
		}
	}()
	actor := captureServiceOperationActor(r)
	data, _ := json.Marshal(map[string]bool{"remove_certificate": req.RemoveCertificate})
	op, err := p.createServiceOperationRequestWithState(r.Context(), serviceOperationKindWebmailAddressRemove, "roundcube", host, req.RequestID, actor, "queued", string(data))
	switch {
	case errors.Is(err, errServiceOperationBusy):
		writeServiceOperationBusy(w)
		return
	case errors.Is(err, errServiceOperationReplay):
		writeAcceptedServiceOperation(w, op)
		return
	case errors.Is(err, errServiceOperationRequestConflict):
		writeServiceOperationRequestConflict(w)
		return
	case err != nil:
		writeServerError(w, err)
		return
	}
	removeCertificate := req.RemoveCertificate
	outcome := &transport.WebmailAddressRemoveResponse{}
	p.launchServiceOperationWithAudit(
		op, actor, "removing",
		"webmail.address.remove:"+host, "webmail.address.remove.failed:"+host,
		release,
		func(ctx context.Context, _ func(string) error) (serviceOperationResult, *serviceOperationFailure) {
			return p.runWebmailAddressRemove(ctx, host, removeCertificate, outcome)
		},
		p.webmailAddressAuditWriter(true, host, func() map[string]any {
			return map[string]any{"remove_certificate": removeCertificate, "certificate_removed": outcome.CertificateRemoved, "code": outcome.ErrorCode}
		}),
	)
	releaseInHandler = false
	writeAcceptedServiceOperation(w, op)
}
