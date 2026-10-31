package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/dnswire"
	"github.com/alicelik/celikpanel/internal/transport"
)

// GET /api/v1/dns/overview (alpha.115): the DNS page's whole picture in one
// read. One D-024 sentence first (why, who acts, the next action, what
// happens then), the DNS servers (this one and the bound second server),
// the names with what the internet answers for each, the main domain's mail
// records and where the owner edits their own records. It is a read only:
// nothing is written, no setup step or DNS change is started or woken, and
// one bounded answer is shared for a minute (an answer with an unknown part
// for 15 seconds), computed apart from the caller so a closed page cannot
// cut it short. A lookup that failed is "unknown", never "missing" or
// "not answering". No secret (link credential, enrollment or join code, DKIM
// private key) is part of the answer.
// DNS sayfasinin tum resmi tek okumada; salt-okur, paylasilan, sinirli.

const dnsOverviewPath = "/api/v1/dns/overview"

const (
	dnsOverviewTTL        = 60 * time.Second
	dnsOverviewUnknownTTL = 15 * time.Second
)

// dnsOverviewSeams are the network edges of the check. Production leaves the
// Panel's field nil; tests set it before the first read, never while a check
// runs (a check reads the seams once, in the calling goroutine).
type dnsOverviewSeams struct {
	Lookups    []setupProviderRecordLookup
	Delegation dnswire.DelegationOptions
	IPv4       func() string
	IPv6       func() string
	// CopyProven reads whether the backup's first copy of the list is
	// proven (serverSetupBackupAnswerState); nil reads the agent.
	CopyProven func(ctx context.Context) bool
	// DKIM answers this server's public DKIM key for the main domain ("" when
	// there is none yet). Only the public half is ever read.
	DKIM func(ctx context.Context, domain string) (string, error)
}

func (p *Panel) dnsOverviewEdges() dnsOverviewSeams {
	if p.dnsOverviewSeams != nil {
		return *p.dnsOverviewSeams
	}
	lookups := []setupProviderRecordLookup{}
	resolvers := []string{"1.1.1.1:53", "8.8.8.8:53"}
	for _, address := range resolvers {
		address := address
		lookups = append(lookups, func(ctx context.Context, name string, qtype uint16) ([]string, error) {
			return dnswire.QueryRecords(ctx, address, name, qtype)
		})
	}
	return dnsOverviewSeams{
		Lookups:    lookups,
		Delegation: dnswire.DelegationOptions{Resolvers: resolvers},
		IPv4:       serverPrimaryIP,
		IPv6:       serverPrimaryIPv6,
		DKIM: func(ctx context.Context, domain string) (string, error) {
			var dkim transport.DKIMStatusResponse
			if err := p.callAgentContext(ctx, "Agent.GetDKIMStatus", &transport.DKIMStatusRequest{Domain: domain, Selector: dkimSelector}, &dkim); err != nil {
				return "", err
			}
			if dkim.Error != "" {
				return "", errors.New(dkim.Error)
			}
			if !dkim.HasKey {
				return "", nil
			}
			return dkim.PublicKeyB64, nil
		},
	}
}

// dnsOverviewSentence is the page's one status (D-024). State is "ok",
// "waiting" (nobody needs to act; a change spreads or setup runs),
// "prerequisite" (someone must act first), "failed" (a verified failure) or
// "unknown" (it could not be checked). Code is stable; the page words it in
// the owner's language from Code and Vars. Reason/Who/Next/Then are the
// English fallback.
type dnsOverviewSentence struct {
	State  string            `json:"state"`
	Code   string            `json:"code"`
	Reason string            `json:"reason"`
	Who    string            `json:"who"`
	Next   string            `json:"next"`
	Then   string            `json:"then"`
	Vars   map[string]string `json:"vars,omitempty"`
}

// dnsOverviewServer is one server of the DNS picture. Who is "this" or
// "peer". Role is "main", "backup", "standalone" or "none" (this server
// keeps no DNS list: an external provider or another CelikPanel does).
// Answers is what asking it directly for the main domain showed: "ok",
// "not_serving" (it answered, not for the domain: a verified failure),
// "unknown" (no answer could be read), "not_joined" (the second server has
// not joined yet) or "not_applicable".
type dnsOverviewServer struct {
	Who       string `json:"who"`
	Role      string `json:"role"`
	ShortName string `json:"short_name,omitempty"`
	PanelName string `json:"panel_name,omitempty"`
	Software  string `json:"software,omitempty"`
	IPv4      string `json:"ipv4,omitempty"`
	Answers   string `json:"answers"`
	Serial    uint32 `json:"serial,omitempty"`
}

// dnsOverviewName is one name with what it must point to and what the
// internet shows. Kind is "panel", "mail", "ns" (a nameserver's own
// address), "nameservers" (the registry's delegation) or "glue". Server is
// "this" or "peer" for an address. State: "ok", "missing", "different",
// "not_yet" (the registry does not show it yet), "unknown". Where says who
// changes it: "zone" (this panel's list), "main" (the main DNS server's
// panel), "provider", "remote" (another CelikPanel) or "registrar".
type dnsOverviewName struct {
	Name     string   `json:"name"`
	Kind     string   `json:"kind"`
	Type     string   `json:"type"`
	Server   string   `json:"server,omitempty"`
	Expected string   `json:"expected"`
	Seen     []string `json:"seen"`
	State    string   `json:"state"`
	Where    string   `json:"where"`
	Optional bool     `json:"optional,omitempty"`
	Reason   string   `json:"reason,omitempty"`
	// Own (a name in our own list that the internet shows missing or
	// different) is what our main DNS server itself answers: "same" (it
	// has the expected value: the change is spreading), "differs" (our
	// list has another value: the owner changed it) or "unknown".
	Own     string   `json:"own,omitempty"`
	OwnSeen []string `json:"own_seen,omitempty"`
}

// dnsOverviewMailRecord is one of the main domain's mail records: "mx",
// "spf", "dkim" or "dmarc". State adds "kept" (an old MX the owner kept)
// and "pending" (no DKIM key yet) to the name states.
type dnsOverviewMailRecord struct {
	Kind     string   `json:"kind"`
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	Expected string   `json:"expected"`
	Seen     []string `json:"seen"`
	State    string   `json:"state"`
}

// dnsOverviewMailReceiver: which server receives the main domain's mail.
type dnsOverviewMailReceiver struct {
	Server    string `json:"server"`
	Host      string `json:"host,omitempty"`
	ShortName string `json:"short_name,omitempty"`
}

// dnsOverviewMXMove is offered when the MX still points to the old mail
// server by the owner's choice and this panel keeps the list: the page
// changes that one record with the existing record editor, after the owner
// confirmed its consequence.
type dnsOverviewMXMove struct {
	Zone   string `json:"zone"`
	ToHost string `json:"to_host"`
	To     string `json:"to"`
}

// dnsOverviewOwnRecords says where the owner edits their own records:
// "here" (Path opens the existing editor, API is that editor's address),
// "main" (on the main DNS server's panel, PanelName), "provider", "remote"
// or "none" (no list yet).
type dnsOverviewOwnRecords struct {
	Where     string `json:"where"`
	Path      string `json:"path,omitempty"`
	API       string `json:"api,omitempty"`
	PanelName string `json:"panel_name,omitempty"`
}

// dnsOverviewJoin: the "Join a server" entry. State "waiting" (setup waits
// for the second server: the existing join card makes the code), "bound"
// (it joined) or "unavailable" with Reason: "setup_not_done", "standalone_bind"
// (a running BIND stays the only DNS server in this release, D-021 Q7),
// "standalone_pdns" (PowerDNS pairs through Advanced → DNS settings),
// "external" or "remote" (joining by code is for servers that keep the list
// themselves; a second server runs its own setup) or "backup" (this server is
// the backup; the main DNS server's panel holds the pair).
type dnsOverviewJoin struct {
	State  string `json:"state"`
	Reason string `json:"reason,omitempty"`
}

type dnsOverview struct {
	Domain         string                  `json:"domain"`
	Home           string                  `json:"home"`
	SetupReady     bool                    `json:"setup_ready"`
	Sentence       dnsOverviewSentence     `json:"sentence"`
	Servers        []dnsOverviewServer     `json:"servers"`
	Names          []dnsOverviewName       `json:"names"`
	Mail           []dnsOverviewMailRecord `json:"mail"`
	MailReceiver   dnsOverviewMailReceiver `json:"mail_receiver"`
	MXMove         *dnsOverviewMXMove      `json:"mx_move,omitempty"`
	OwnRecordsLink dnsOverviewOwnRecords   `json:"own_records_link"`
	Join           dnsOverviewJoin         `json:"join"`
	CheckedAt      string                  `json:"checked_at"`
	NextCheckAfter int                     `json:"next_check_after"`
	// FreshAfter is how many seconds until "Check now" starts a new check;
	// until then it returns the check in flight or the latest answer.
	FreshAfter int `json:"fresh_after"`
}

// dnsOverviewPeer is what this server knows of the other server, from the
// first server's binding or the second server's link (never their secrets).
type dnsOverviewPeer struct {
	// FromLink: read from the join document this server received (it is
	// the second server); its MainMail is the shared answer.
	FromLink  bool
	Bound     bool
	ShortName string
	PanelName string
	MailHost  string
	MainMail  bool
	MailDNS   *serverSetupMailDNS
	Software  string
	IPv4      string
}

// dnsOverviewInput is everything the check reads from this panel's own
// state, gathered before any network question is asked.
type dnsOverviewInput struct {
	Draft       serverSetupDraft
	Ready       bool
	Running     bool
	JoinWaiting bool
	Peer        dnsOverviewPeer
	Engine      string
	EngineTopo  string
	DomainID    int64
	Infra       bool
	IPv4, IPv6  string
	// EngineOwnerChanged: the DNS software card's state says the active
	// engine's published files were changed outside the panel; the overview
	// reads the same engine state and says the same notice.
	EngineOwnerChanged bool
	// EngineSeriesUntested: the active PowerDNS runs a release outside its
	// tested series after the system's own update; the same engine reading.
	EngineSeriesUntested bool
	// ZoneDigest is a digest of the main domain zone's records: any write
	// to the zone, by any path, is a new state and a new check.
	ZoneDigest string
}

func (p *Panel) dnsOverviewInputs(ctx context.Context, edges dnsOverviewSeams) (dnsOverviewInput, error) {
	in := dnsOverviewInput{}
	state, err := p.loadServerSetup(ctx)
	if err != nil {
		return in, err
	}
	in.Draft, in.Ready = state.Draft, state.Status == "ready"
	execution, err := p.latestServerSetupExecution(ctx)
	if err != nil {
		return in, err
	}
	matched := false
	if execution != nil {
		in.Running = stringIn(execution.Status, "running", "waiting")
		if plan, err := p.loadServerSetupPlan(ctx, execution.PlanID); err == nil && reflect.DeepEqual(plan.Draft, state.Draft) {
			matched = true
			if err := p.dnsOverviewApplyPlan(ctx, &in, plan, execution.ID); err != nil {
				return in, err
			}
			if !in.Peer.Bound && serverSetupPlanHasStep(plan, "peer_join") {
				in.JoinWaiting = in.Running
			}
		}
	}
	// The draft was edited after setup completed (an extension being
	// reviewed): what runs is the last succeeded plan, with its binding.
	if !matched && in.Ready {
		succeeded, err := p.latestSucceededServerSetupExecution(ctx)
		if err != nil {
			return in, err
		}
		if succeeded != nil {
			if plan, err := p.loadServerSetupPlan(ctx, succeeded.PlanID); err == nil {
				in.Draft = plan.Draft
				if err := p.dnsOverviewApplyPlan(ctx, &in, plan, succeeded.ID); err != nil {
					return in, err
				}
			}
		}
	}
	if link, err := p.readServerSetupLink(ctx); err != nil {
		return in, err
	} else if link != nil {
		first := link.Document.First
		in.Peer = dnsOverviewPeer{FromLink: true, Bound: true, ShortName: first.ShortName, PanelName: first.PanelDomain, MailHost: first.MailHostname,
			MainMail: first.MainDomainMail, MailDNS: first.MailDNS, Software: first.DNSEngine, IPv4: first.IPv4}
	}
	engine, _, err := p.readDNSEngineStateAndOperation(ctx)
	if err != nil {
		return in, err
	}
	in.Engine, in.EngineTopo = string(engine.ActiveEngine), engine.Topology
	if engine.ActiveEngine != "" {
		// The engine card's own reading; an unread state is not a change.
		if runtimes, _, hold, err := p.readDNSBackendRuntime(ctx); err == nil {
			_, entries := deriveDNSEnginePresentation(engine, runtimes, nil, hold)
			for _, entry := range entries {
				in.EngineOwnerChanged = in.EngineOwnerChanged || (entry.ID == engine.ActiveEngine && entry.DetailCode == dnsEngineOwnerChangedCode)
				in.EngineSeriesUntested = in.EngineSeriesUntested || (entry.ID == engine.ActiveEngine && entry.DetailCode == dnsEngineSeriesUntestedCode)
			}
		}
	}
	if domain := in.Draft.MainDomain; domain != "" {
		err := p.db.GetDB().QueryRowContext(ctx, `SELECT id FROM domains WHERE name=?`, domain).Scan(&in.DomainID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return in, err
		}
		if in.Infra, err = p.isInfrastructureDNSZone(ctx, domain); err != nil {
			return in, err
		}
		if in.ZoneDigest, err = p.dnsOverviewZoneDigest(ctx, domain); err != nil {
			return in, err
		}
	}
	if ip, ok := canonicalIPv4(edges.IPv4()); ok {
		in.IPv4 = ip
	}
	if parsed := net.ParseIP(edges.IPv6()); parsed != nil && parsed.To4() == nil && parsed.IsGlobalUnicast() && !parsed.IsPrivate() {
		in.IPv6 = parsed.String()
	}
	return in, nil
}

// dnsOverviewZoneDigest digests a zone's stored records (bounded), "" when
// there is no such zone.
func (p *Panel) dnsOverviewZoneDigest(ctx context.Context, zone string) (string, error) {
	rows, err := p.db.GetDB().QueryContext(ctx, `SELECT r.id,COALESCE(r.name,''),COALESCE(r.type,''),COALESCE(r.content,''),COALESCE(r.ttl,0),COALESCE(r.prio,0),COALESCE(r.disabled,0)
		FROM pdns_records r JOIN pdns_domains d ON d.id=r.domain_id WHERE d.name=? ORDER BY r.id LIMIT 4097`, zone)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	digest := sha256.New()
	count := 0
	for rows.Next() {
		var id, ttl, prio, disabled int64
		var name, recordType, content string
		if err := rows.Scan(&id, &name, &recordType, &content, &ttl, &prio, &disabled); err != nil {
			return "", err
		}
		count++
		fmt.Fprintf(digest, "%d|%q|%q|%q|%d|%d|%d;", id, name, recordType, content, ttl, prio, disabled)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if count == 0 {
		return "", nil
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

// dnsOverviewApplyPlan completes the picture with the plan's second-server
// binding, when it has one.
func (p *Panel) dnsOverviewApplyPlan(ctx context.Context, in *dnsOverviewInput, plan serverSetupPlan, executionID string) error {
	if !serverSetupPlanHasStep(plan, "peer_join") {
		return nil
	}
	binding, err := p.readServerSetupPeerBinding(ctx, plan, executionID)
	if err != nil || binding == nil {
		return err
	}
	in.Draft = serverSetupBoundPlan(plan, *binding).Draft
	in.Peer = dnsOverviewPeer{Bound: true, ShortName: binding.PeerShortName, PanelName: binding.PeerPanelDomain, MailHost: binding.PeerMailHostname,
		MainMail: binding.PeerMail != nil, Software: binding.PeerDNSEngine, IPv4: binding.PeerIPv4}
	if binding.PeerMail != nil {
		in.Peer.MailDNS = &serverSetupMailDNS{MX: binding.PeerMail.MX, DMARC: binding.PeerMail.DMARC}
	}
	return nil
}

// latestSucceededServerSetupExecution is the last setup run that finished.
func (p *Panel) latestSucceededServerSetupExecution(ctx context.Context) (*serverSetupExecution, error) {
	var raw string
	err := p.db.GetDB().QueryRowContext(ctx, `SELECT execution_json FROM server_setup_executions WHERE status='succeeded' ORDER BY created_at DESC, id DESC LIMIT 1`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var execution serverSetupExecution
	if err := json.Unmarshal([]byte(raw), &execution); err != nil {
		return nil, err
	}
	return &execution, nil
}

// thisRole is this server's DNS role in the picture.
func (in dnsOverviewInput) thisRole() string {
	d := in.Draft
	switch {
	case d.DNSMode != setupDNSModeLocal:
		return "none"
	case serverSetupStandaloneDNS(d):
		return "standalone"
	case d.DNSRole == transport.DNSPairRoleSecondary:
		return "backup"
	default:
		return "main"
	}
}

// home is where the main domain's DNS list lives: "own" (these servers),
// "provider", "remote" (another CelikPanel) or "none" (setup not chosen).
func (in dnsOverviewInput) home() string {
	switch in.Draft.DNSMode {
	case setupDNSModeLocal:
		return "own"
	case setupDNSModeExternal:
		return "provider"
	case setupDNSModeExisting:
		return "remote"
	}
	return "none"
}

func (in dnsOverviewInput) twoServers() bool {
	return in.Draft.ServerCount == 2 || in.Peer.Bound
}

// dnsOverviewAddressRows are the names that point to the servers, as A
// records (and AAAA where the provider list carries them), each labelled
// by kind and server. With the list on these servers they are the zone's
// own addresses; at a provider they are the provider records of setup.
func dnsOverviewAddressRows(in dnsOverviewInput) []dnsOverviewName {
	d := in.Draft
	where := map[string]string{"own": "zone", "provider": "provider", "remote": "remote"}[in.home()]
	if in.thisRole() == "backup" {
		where = "main"
	}
	rows := []dnsOverviewName{}
	add := func(name, kind, server, recordType, value string) {
		if name == "" || value == "" || slices.ContainsFunc(rows, func(row dnsOverviewName) bool { return row.Name == name && row.Type == recordType }) {
			return
		}
		rows = append(rows, dnsOverviewName{Name: name, Kind: kind, Type: recordType, Server: server, Expected: value, Seen: []string{}, State: "unknown", Where: where})
	}
	thisIP := d.LocalIP
	if thisIP == "" || in.home() != "own" {
		thisIP = in.IPv4
	}
	if in.home() == "provider" {
		for _, row := range serverSetupProviderRecordRows(d, thisIP, in.IPv6) {
			kind := "panel"
			if row.Name == d.MailHostname && row.Name != d.PanelDomain {
				kind = "mail"
			}
			add(row.Name, kind, "this", row.Type, row.Value)
		}
		return rows
	}
	if in.home() == "own" && d.MainDomain != "" {
		for _, ns := range serverSetupDelegationExpected(d, "") {
			if !serverSetupDNSNameWithin(ns.Name, d.MainDomain) {
				continue
			}
			server := "this"
			if (ns.Server == "backup") != (in.thisRole() == "backup") {
				server = "peer"
			}
			add(ns.Name, "ns", server, "A", ns.IPv4)
		}
	}
	add(d.PanelDomain, "panel", "this", "A", thisIP)
	if serverSetupHasComponent(d, "postfix") && d.MailHostname != "" {
		add(d.MailHostname, "mail", "this", "A", thisIP)
	}
	if in.Peer.Bound && in.Peer.IPv4 != "" {
		add(in.Peer.PanelName, "panel", "peer", "A", in.Peer.IPv4)
		add(in.Peer.MailHost, "mail", "peer", "A", in.Peer.IPv4)
	}
	return rows
}

// dnsOverviewMailInputs: the server receiving the main domain's mail, its
// mail host and the owner's MX/DMARC answers.
func dnsOverviewMailInputs(in dnsOverviewInput) (dnsOverviewMailReceiver, serverSetupMailDNS, bool) {
	d := in.Draft
	if d.MainDomain == "" {
		return dnsOverviewMailReceiver{Server: "none"}, serverSetupMailDNS{}, false
	}
	// The join's shared answer decides who receives the main domain's mail
	// (N5): on a second server whose first server receives it, that is the
	// receiver, whatever this server runs itself.
	if in.Peer.FromLink && in.Peer.MainMail && in.Peer.MailHost != "" {
		choice := serverSetupMailDNS{}
		if in.Peer.MailDNS != nil {
			choice = *in.Peer.MailDNS
		}
		return dnsOverviewMailReceiver{Server: "peer", Host: in.Peer.MailHost, ShortName: in.Peer.ShortName}, choice, true
	}
	if host, choice, ok := serverSetupMainDomainMail(d); ok {
		receiver := dnsOverviewMailReceiver{Server: "this", Host: host, ShortName: d.ShortName}
		if !serverSetupHasMail(d) {
			receiver.Server, receiver.ShortName = "peer", in.Peer.ShortName
		}
		return receiver, choice, true
	}
	if in.Peer.Bound && in.Peer.MainMail && in.Peer.MailHost != "" {
		choice := serverSetupMailDNS{}
		if in.Peer.MailDNS != nil {
			choice = *in.Peer.MailDNS
		}
		return dnsOverviewMailReceiver{Server: "peer", Host: in.Peer.MailHost, ShortName: in.Peer.ShortName}, choice, true
	}
	return dnsOverviewMailReceiver{Server: "none"}, serverSetupMailDNS{}, false
}

func dnsOverviewOwnRecordsLink(in dnsOverviewInput) dnsOverviewOwnRecords {
	switch in.home() {
	case "provider":
		return dnsOverviewOwnRecords{Where: "provider"}
	case "remote":
		return dnsOverviewOwnRecords{Where: "remote"}
	case "none":
		return dnsOverviewOwnRecords{Where: "none"}
	}
	if in.thisRole() == "backup" {
		return dnsOverviewOwnRecords{Where: "main", PanelName: in.Peer.PanelName}
	}
	domain := in.Draft.MainDomain
	switch {
	case domain != "" && in.DomainID > 0:
		return dnsOverviewOwnRecords{Where: "here", Path: "/domains/" + url.PathEscape(domain) + "?tab=dns", API: "/api/v1/domains/" + jsonInt(in.DomainID) + "/dns"}
	case domain != "" && in.Infra:
		return dnsOverviewOwnRecords{Where: "here", Path: "/domains/infrastructure/" + url.PathEscape(domain), API: infrastructureDNSZonesPath + "/" + url.PathEscape(domain)}
	}
	return dnsOverviewOwnRecords{Where: "none"}
}

func jsonInt(value int64) string {
	raw, _ := json.Marshal(value)
	return string(raw)
}

// checkDNSOverview asks the internet, bounded: the public resolvers for
// every name and mail record (4 s) and, with the list on these servers,
// the registry's delegation and each of our servers directly (the
// registrar check's own budget), side by side.
func (p *Panel) checkDNSOverview(ctx context.Context, in dnsOverviewInput, edges dnsOverviewSeams) dnsOverview {
	d := in.Draft
	result := dnsOverview{Domain: d.MainDomain, Home: in.home(), SetupReady: in.Ready, Servers: []dnsOverviewServer{}, Names: []dnsOverviewName{}, Mail: []dnsOverviewMailRecord{},
		OwnRecordsLink: dnsOverviewOwnRecordsLink(in)}
	role := in.thisRole()
	software := in.Engine
	if role == "none" {
		software = ""
	}
	result.Servers = append(result.Servers, dnsOverviewServer{Who: "this", Role: role, ShortName: d.ShortName, PanelName: d.PanelDomain, Software: software, IPv4: firstNonEmpty(d.LocalIP, in.IPv4), Answers: "not_applicable"})
	if in.twoServers() && role != "standalone" {
		peerRole := map[string]string{"main": "backup", "backup": "main"}[role]
		if peerRole == "" {
			peerRole = "none"
		}
		peer := dnsOverviewServer{Who: "peer", Role: peerRole, ShortName: in.Peer.ShortName, PanelName: in.Peer.PanelName, IPv4: in.Peer.IPv4, Answers: "not_applicable"}
		if peerRole != "none" {
			peer.Software = in.Peer.Software
		}
		if !in.Peer.Bound {
			peer.Answers, peer.IPv4 = "not_joined", firstNonEmpty(d.PeerIP)
		}
		result.Servers = append(result.Servers, peer)
	}
	result.Join = dnsOverviewJoinFor(in, role)
	result.MailReceiver = dnsOverviewMailReceiver{Server: "none"}
	// Setup neither finished nor running: nothing is asked of the internet,
	// so nothing unfinished is reported as failing; the page points to
	// Server setup.
	if !in.Ready && !in.Running {
		result.Sentence = dnsOverviewSentenceFor(in, result)
		return result
	}

	addresses := dnsOverviewAddressRows(in)
	receiver, choice, hasMail := dnsOverviewMailInputs(in)
	result.MailReceiver = receiver
	mailRows := []serverSetupProviderRecordRow{}
	// A key that could not be read is unknown, never "not made yet".
	var dkimErr error
	if hasMail {
		mailDraft := d
		mailDraft.MailHostname, mailDraft.MailDNS = receiver.Host, &choice
		dkim := ""
		peerKey := receiver.Server == "peer"
		if !peerKey && edges.DKIM != nil {
			dkim, dkimErr = edges.DKIM(ctx, d.MainDomain)
			if dkimErr != nil {
				dkim = ""
			}
		}
		mailRows = serverSetupProviderMailRows(mailDraft, dkim)
		if peerKey {
			// The other server holds its own DKIM key: the page checks that a
			// key is published, without knowing its value here.
			mailRows[3].State, mailRows[3].Kind = "unknown", "dkim-published"
		}
	}

	rows := make([]serverSetupProviderRecordRow, 0, len(addresses)+len(mailRows))
	for _, row := range addresses {
		rows = append(rows, serverSetupProviderRecordRow{Name: row.Name, Type: row.Type, Value: row.Expected, State: "unknown", Seen: []string{}})
	}
	for _, row := range mailRows {
		if row.Kind == "dkim-published" {
			row.Kind = "dkim"
			row.Value = "v=DKIM1"
		}
		rows = append(rows, row)
	}

	var wg sync.WaitGroup
	var checked []serverSetupProviderRecordRow
	var delegation serverSetupDelegation
	copyProven := false
	delegate := in.home() == "own" && d.MainDomain != "" && d.NS1 != "" && d.NS2 != "" && (role != "backup" || in.Peer.Bound || d.PeerIP != "")
	wg.Add(1)
	go func() {
		defer wg.Done()
		checked = p.checkServerSetupProviderRecords(ctx, rows, edges.Lookups)
	}()
	if delegate {
		wg.Add(1)
		go func() {
			defer wg.Done()
			delegation = p.checkServerSetupDelegation(ctx, d, in.IPv6, edges.Delegation)
			copyProven = p.dnsBackupCopyProven(ctx)
			applyServerSetupBackupCopy(&delegation, copyProven)
		}()
	}
	wg.Wait()

	for index, row := range addresses {
		row.State, row.Seen = checked[index].State, checked[index].Seen
		result.Names = append(result.Names, row)
	}
	for index, row := range mailRows {
		got := checked[len(addresses)+index]
		kind := strings.TrimSuffix(strings.TrimSuffix(row.Kind, "-later"), "-add")
		if row.Kind == "dkim-published" {
			kind = "dkim"
			// Any published DKIM key counts: its value is the other server's.
			got.State = dnsOverviewDKIMPublished(got.Seen, got.State)
			got.Value = ""
		}
		if kind == "dkim" && dkimErr != nil {
			got.State, got.Value, got.Seen = "unknown", "", []string{}
		}
		result.Mail = append(result.Mail, dnsOverviewMailRecord{Kind: kind, Name: row.Name, Type: row.Type, Expected: got.Value, Seen: got.Seen, State: got.State})
	}
	if delegate {
		for _, item := range delegation.Items {
			switch {
			case item.ID == "nameservers":
				name := dnsOverviewName{Name: d.MainDomain, Kind: "nameservers", Type: "NS", Expected: item.Vars["expected"], Seen: []string{}, State: item.State, Where: "registrar", Reason: item.Vars["reason"]}
				if seen := item.Vars["seen"]; seen != "" {
					name.Seen = strings.Split(seen, ", ")
				} else if item.State == "ok" {
					name.Seen = strings.Split(item.Vars["expected"], ", ")
				}
				result.Names = append(result.Names, name)
			case strings.HasPrefix(item.ID, "glue_v4:") || strings.HasPrefix(item.ID, "glue_v6:"):
				recordType := "A"
				if strings.HasPrefix(item.ID, "glue_v6:") {
					recordType = "AAAA"
				}
				glue := dnsOverviewName{Name: item.Vars["name"], Kind: "glue", Type: recordType, Expected: item.Vars["ip"], Seen: []string{}, State: item.State, Where: "registrar", Optional: item.Vars["optional"] == "true"}
				if seen := item.Vars["seen"]; seen != "" {
					glue.Seen = strings.Split(seen, ", ")
				}
				result.Names = append(result.Names, glue)
			}
		}
		for index := range result.Servers {
			server := &result.Servers[index]
			if server.Role == "none" || server.Answers == "not_joined" {
				continue
			}
			dnsRole := server.Role
			if dnsRole == "standalone" {
				dnsRole = "main"
			}
			for _, answered := range delegation.Servers {
				if answered.Role == dnsRole && answered.IP != "" {
					state := answered.State
					if dnsRole == "backup" {
						state = serverSetupBackupAnswerState(state, copyProven)
					}
					server.Answers = map[string]string{"ok": "ok", "not_yet": "not_serving", "waiting": "waiting"}[state]
					if server.Answers == "" {
						server.Answers = "unknown"
					}
					server.Serial = answered.Serial
				}
			}
		}
	}
	if delegate {
		p.compareDNSOverviewOwnAnswers(ctx, &result, edges)
	}
	if hasMail && result.OwnRecordsLink.Where == "here" {
		if err := p.dnsOverviewZoneMailExchange(ctx, in, receiver, &result); err != nil {
			for index := range result.Mail {
				if result.Mail[index].Kind == "mx" {
					result.Mail[index].State = "unknown"
				}
			}
		}
	}
	if mx := dnsOverviewMXMoveOffer(in, result, receiver); mx != nil {
		result.MXMove = mx
	}
	result.Sentence = dnsOverviewSentenceFor(in, result)
	return result
}

func dnsOverviewJoinFor(in dnsOverviewInput, role string) dnsOverviewJoin {
	switch {
	case in.Peer.Bound:
		return dnsOverviewJoin{State: "bound"}
	case in.JoinWaiting:
		return dnsOverviewJoin{State: "waiting"}
	case !in.Ready || in.Draft.MainDomain == "":
		return dnsOverviewJoin{State: "unavailable", Reason: "setup_not_done"}
	case in.home() == "provider":
		return dnsOverviewJoin{State: "unavailable", Reason: "external"}
	case in.home() == "remote":
		return dnsOverviewJoin{State: "unavailable", Reason: "remote"}
	case role == "backup":
		return dnsOverviewJoin{State: "unavailable", Reason: "backup"}
	case in.Engine == string(transport.DNSEnginePowerDNS):
		return dnsOverviewJoin{State: "unavailable", Reason: "standalone_pdns"}
	}
	return dnsOverviewJoin{State: "unavailable", Reason: "standalone_bind"}
}

// compareDNSOverviewOwnAnswers asks our main DNS server itself, without
// recursion, for each name of our list that the internet shows missing or
// different: a readable authoritative answer with the expected value means
// the change is spreading; one without it means our own list says something
// else (the owner changed it). Bounded; an unreadable answer is unknown.
func (p *Panel) compareDNSOverviewOwnAnswers(ctx context.Context, result *dnsOverview, edges dnsOverviewSeams) {
	mainIP := ""
	for _, server := range result.Servers {
		if (server.Role == "main" || server.Role == "standalone") && server.IPv4 != "" && server.Answers == "ok" {
			mainIP = server.IPv4
		}
	}
	address := net.JoinHostPort(mainIP, "53")
	if edges.Delegation.Address != nil {
		address = edges.Delegation.Address(mainIP)
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	for index := range result.Names {
		row := &result.Names[index]
		if (row.Where != "zone" && row.Where != "main") || (row.State != "missing" && row.State != "different") || serverSetupRecordTypes[row.Type] == 0 {
			continue
		}
		row.Own = "unknown"
		if mainIP == "" {
			continue
		}
		message, err := dnswire.Exchange(ctx, address, row.Name, serverSetupRecordTypes[row.Type], false)
		if err != nil || !message.Authoritative || (message.RCode != 0 && message.RCode != 3) {
			continue
		}
		seen := []string{}
		for _, rr := range message.Answer {
			if rr.Type == serverSetupRecordTypes[row.Type] {
				seen = append(seen, rr.Value)
			}
		}
		row.Own, row.OwnSeen = "differs", seen
		if slices.Contains(seen, row.Expected) {
			row.Own = "same"
		}
	}
}

// dnsOverviewZoneMailExchange judges the main domain's MX by this panel's
// own list, which the owner edits: it names the receiving server (the
// internet's answer then only says whether it has spread), it is still
// exactly the old MX setup imported ("kept": the owner chose to keep the old
// mail server, and moving is offered), it names another mail server the
// owner set ("owner": never called the old mail server, never offered for
// moving), or there is none ("none").
func (p *Panel) dnsOverviewZoneMailExchange(ctx context.Context, in dnsOverviewInput, receiver dnsOverviewMailReceiver, result *dnsOverview) error {
	domain := in.Draft.MainDomain
	rows, err := p.db.GetDB().QueryContext(ctx, `SELECT COALESCE(r.content,''),COALESCE(r.prio,0) FROM pdns_records r JOIN pdns_domains d ON d.id=r.domain_id
		WHERE d.name=? AND r.type='MX' AND lower(r.name)=? AND COALESCE(r.disabled,0)=0 ORDER BY r.id LIMIT 33`, domain, domain)
	if err != nil {
		return err
	}
	defer rows.Close()
	zone := []string{}
	ours := false
	for rows.Next() {
		var content string
		var priority int
		if err := rows.Scan(&content, &priority); err != nil {
			return err
		}
		host := canonicalDNSName(content)
		zone = append(zone, fmt.Sprintf("%d %s", priority, host))
		ours = ours || host == canonicalDNSName(receiver.Host)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	imported := []string{}
	for _, record := range serverSetupImported(in.Draft) {
		if record.Type == "MX" && canonicalDNSName(record.Name) == domain {
			imported = append(imported, fmt.Sprintf("%d %s", record.Priority, canonicalDNSName(record.Content)))
		}
	}
	slices.Sort(zone)
	slices.Sort(imported)
	for index := range result.Mail {
		row := &result.Mail[index]
		if row.Kind != "mx" {
			continue
		}
		switch {
		case ours:
			row.Expected = fmt.Sprintf("10 %s", canonicalDNSName(receiver.Host))
			if row.State == "kept" {
				// Our list names this mail server; the internet still shows
				// the old one: the change is spreading.
				row.State = "different"
			}
		case len(zone) == 0:
			row.State, row.Seen = "none", []string{}
		case len(imported) > 0 && slices.Equal(zone, imported):
			row.State, row.Seen = "kept", zone
		default:
			row.State, row.Seen = "owner", zone
		}
	}
	return nil
}

func dnsOverviewDKIMPublished(seen []string, state string) string {
	if len(seen) > 0 {
		return "ok"
	}
	if state == "unknown" {
		return "unknown"
	}
	return "missing"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// dnsOverviewMXMoveOffer: the MX still names another mail server because
// the owner kept it, and this panel keeps the list.
func dnsOverviewMXMoveOffer(in dnsOverviewInput, result dnsOverview, receiver dnsOverviewMailReceiver) *dnsOverviewMXMove {
	if result.OwnRecordsLink.Where != "here" || receiver.Host == "" {
		return nil
	}
	for _, row := range result.Mail {
		if row.Kind == "mx" && row.State == "kept" {
			return &dnsOverviewMXMove{Zone: in.Draft.MainDomain, ToHost: receiver.Host, To: receiver.Server}
		}
	}
	return nil
}

// The owner-change notice's English text (D-024), shared by the overview's
// sentence and the engine card's copy (web: dnsov.s.engine_owner_changed).
const (
	dnsEngineOwnerChangedReason = "This server's DNS software serves a list that was changed outside CelikPanel (for example with rndc)."
	dnsEngineOwnerChangedWho    = "You, the owner of this server."
	dnsEngineOwnerChangedNext   = "Keep your change: there is nothing to do, and CelikPanel does not overwrite it."
	dnsEngineOwnerChangedThen   = "While it stays, CelikPanel publishes no DNS change on this server (the panel's record editor cannot publish either); handing this DNS software back to CelikPanel after a native edit is not available in this release. This page checks again every minute."
)

// The untested-series notice's English text (D-024), shared by the overview's
// sentence and the engine card's copy (web: dnsov.s.engine_series_untested).
const (
	dnsEngineSeriesUntestedReason = "A system update moved PowerDNS on this server to a release CelikPanel has not tested yet. PowerDNS keeps answering with the records it has."
	dnsEngineSeriesUntestedWho    = "Nobody has to undo anything on the server; the next step is a CelikPanel update."
	dnsEngineSeriesUntestedNext   = "Update CelikPanel from its own update screen when a release names this PowerDNS version as tested."
	dnsEngineSeriesUntestedThen   = "Until then CelikPanel publishes no DNS change on this server (new domains and record edits wait); it continues by itself once the installed PowerDNS is tested. This page checks again every minute."
)

func dnsOverviewSay(state, code, reason, who, next, then string, vars map[string]string) dnsOverviewSentence {
	return dnsOverviewSentence{State: state, Code: code, Reason: reason, Who: who, Next: next, Then: then, Vars: vars}
}

// dnsOverviewSentenceFor picks the one sentence. A verified failure comes
// first, then what someone must do, then what could not be checked, then a
// change that is spreading; only when everything was read correctly is it
// "all good" (D-024: unknown is never ok and never a failure).
func dnsOverviewSentenceFor(in dnsOverviewInput, r dnsOverview) dnsOverviewSentence {
	d := in.Draft
	vars := map[string]string{"domain": d.MainDomain}
	if d.MainDomain == "" || d.DNSMode == "" || (!in.Ready && !in.Running) {
		return dnsOverviewSay("prerequisite", "dns_overview_setup_not_done", "Server setup has not finished, so there is no DNS picture to check yet.", "You, in Server setup.", "Open Server setup and finish it.", "This page then shows the names and what the internet answers.", nil)
	}
	if in.JoinWaiting {
		return dnsOverviewSay("prerequisite", "dns_overview_join_waiting", "Setup waits for the second server to join.", "You, on the second server.", "Run the second server's setup and enter this server's address and its join code (make the code below).", "Setup continues by itself once the second server has joined.", vars)
	}
	if in.EngineOwnerChanged {
		return dnsOverviewSay("prerequisite", "dns_overview_"+dnsEngineOwnerChangedCode, dnsEngineOwnerChangedReason, dnsEngineOwnerChangedWho, dnsEngineOwnerChangedNext, dnsEngineOwnerChangedThen, vars)
	}
	if in.EngineSeriesUntested {
		return dnsOverviewSay("prerequisite", "dns_overview_"+dnsEngineSeriesUntestedCode, dnsEngineSeriesUntestedReason, dnsEngineSeriesUntestedWho, dnsEngineSeriesUntestedNext, dnsEngineSeriesUntestedThen, vars)
	}
	if !in.Ready && in.Running {
		return dnsOverviewSay("waiting", "dns_overview_setup_running", "Server setup is still running; DNS is part of it.", "Follow the setup page; it says when you need to act.", "Open Server setup.", "This page shows the finished picture once setup completes.", vars)
	}
	server := func(who string) dnsOverviewServer {
		for _, s := range r.Servers {
			if s.Who == who {
				return s
			}
		}
		return dnsOverviewServer{}
	}
	byRole := func(role string) dnsOverviewServer {
		for _, s := range r.Servers {
			if s.Role == role || (role == "main" && s.Role == "standalone") {
				return s
			}
		}
		return dnsOverviewServer{}
	}
	// The short name when there is one; the page names a server without one
	// ("This server" / "Second server") from its *_who value.
	label := func(s dnsOverviewServer) string { return s.ShortName }
	mainServer, backup := byRole("main"), byRole("backup")
	if r.Home == "own" {
		vars["main"], vars["main_ip"], vars["main_who"] = label(mainServer), mainServer.IPv4, mainServer.Who
		if mainServer.Answers == "not_serving" {
			vars["who"] = mainServer.Who
			return dnsOverviewSay("failed", "dns_overview_main_not_serving", "The main DNS server answers, but not for the domain: the domain's list is not served.", "You, on the main DNS server's panel.", "Open Advanced → DNS software on that server and check its state.", "This page checks again every minute.", vars)
		}
		if backup.Role == "backup" && backup.Answers == "not_serving" {
			vars["backup"], vars["backup_ip"], vars["backup_who"] = label(backup), backup.IPv4, backup.Who
			return dnsOverviewSay("failed", "dns_overview_backup_not_serving", "The backup DNS server answers, but not for the domain; the domain answers from the main DNS server only, without a backup.", "You, on the backup DNS server's panel.", "Check its DNS software and its copy of the list there.", "When its copy is served again, this page shows it within a minute.", vars)
		}
		if peer := server("peer"); peer.Answers == "not_joined" {
			return dnsOverviewSay("prerequisite", "dns_overview_peer_not_joined", "The second server has not joined yet, so the domain has no backup DNS server.", "You, on the second server.", "Run its setup and join with this server's code.", "This page shows both servers once it joins.", vars)
		}
		for _, name := range r.Names {
			if name.Kind == "nameservers" && name.State == "not_yet" {
				vars["names"], vars["seen"], vars["reason"] = name.Expected, strings.Join(name.Seen, ", "), name.Reason
				code := "dns_overview_registrar_not_pointing"
				if name.Reason == "not_registered" {
					code = "dns_overview_domain_not_registered"
				}
				return dnsOverviewSay("prerequisite", code, "The registry does not hand out your nameservers for the domain yet.", "You, at the company where you registered the domain.", "Enter the nameservers "+name.Expected+" there.", "This page checks again every minute; the change can take a few hours to spread.", vars)
			}
		}
		for _, name := range r.Names {
			if name.Kind == "glue" && name.State == "not_yet" && !name.Optional {
				vars["name"], vars["ip"] = name.Name, name.Expected
				return dnsOverviewSay("prerequisite", "dns_overview_glue_missing", "The registry has no IP record (glue) for a nameserver inside the domain.", "You, at the company where you registered the domain.", "Add the IP record "+name.Name+" → "+name.Expected+" there.", "This page checks again every minute.", vars)
			}
		}
	}
	if r.Home == "provider" || r.Home == "remote" {
		for _, name := range r.Names {
			if name.State == "missing" || name.State == "different" {
				vars["name"], vars["value"], vars["type"] = name.Name, name.Expected, name.Type
				code := "dns_overview_provider_records"
				who := "You, in your DNS provider's panel."
				if r.Home == "remote" {
					code, who = "dns_overview_remote_records", "The other CelikPanel that keeps the list."
				}
				return dnsOverviewSay("prerequisite", code, "A record the servers need is missing or different where the list is kept.", who, "Set "+name.Name+" ("+name.Type+") to "+name.Expected+".", "This page checks again every minute.", vars)
			}
		}
		for _, mail := range r.Mail {
			if mail.State == "missing" || mail.State == "different" {
				vars["kind"], vars["name"] = mail.Kind, mail.Name
				code := "dns_overview_provider_mail"
				who := "You, in your DNS provider's panel."
				if r.Home == "remote" {
					code, who = "dns_overview_remote_mail", "The other CelikPanel that keeps the list."
				}
				return dnsOverviewSay("prerequisite", code, "A mail record of the domain is missing or different where the list is kept.", who, "Add the value shown under Mail records.", "This page checks again every minute.", vars)
			}
		}
	}
	for _, mail := range r.Mail {
		if mail.Kind == "mx" && mail.State == "none" && in.Ready {
			return dnsOverviewSay("prerequisite", "dns_overview_mail_no_mx", "The domain's list has no MX record, so no mail can be delivered to the domain.", "You, the owner of the list.", "Add an MX record in the record editor (it names the server that receives the mail).", "This page checks again every minute.", vars)
		}
	}
	// A finished setup with mail and no DKIM key yet: mail is sent unsigned
	// and may be marked as spam; that is a task, never "all good".
	for _, mail := range r.Mail {
		if mail.Kind == "dkim" && mail.State == "pending" && in.Ready {
			return dnsOverviewSay("prerequisite", "dns_overview_dkim_pending", "The mail signing key (DKIM) of the domain has not been made yet, so mail is sent unsigned.", "You, in Server setup.", "Extend this server's setup and run it again: its mail records step makes the key.", "This page shows the key and its record once it exists.", vars)
		}
	}
	// Our own list says something else for a server's name: the owner
	// changed it, which is not a delay that passes by itself.
	for _, name := range r.Names {
		if name.Own == "differs" {
			vars["name"], vars["value"], vars["seen"] = name.Name, name.Expected, strings.Join(name.OwnSeen, ", ")
			return dnsOverviewSay("prerequisite", "dns_overview_owner_changed", "The list itself has another value for one of the servers' own names.", "You, the owner of the list.", "Check "+name.Name+" in the record editor; it should be "+name.Expected+".", "This page checks again every minute.", vars)
		}
	}
	if r.Home == "own" {
		if mainServer.Answers == "unknown" || (backup.Role == "backup" && backup.Answers == "unknown") {
			who := mainServer
			if mainServer.Answers != "unknown" {
				who = backup
			}
			vars["server"], vars["ip"], vars["server_who"] = label(who), who.IPv4, who.Who
			return dnsOverviewSay("unknown", "dns_overview_server_unknown", "A DNS server could not be asked just now; this is not known to be a failure.", "Nobody yet.", "If it stays so, check that the server is running and port 53 is open.", "This page asks again in a minute.", vars)
		}
	}
	for _, name := range r.Names {
		if name.State == "unknown" {
			vars["name"] = name.Name
			return dnsOverviewSay("unknown", "dns_overview_some_unknown", "Some names could not be checked from the internet just now.", "Nobody yet.", "Nothing to do now.", "This page asks again in a minute.", vars)
		}
	}
	for _, mail := range r.Mail {
		if mail.State == "unknown" {
			vars["kind"] = mail.Kind
			return dnsOverviewSay("unknown", "dns_overview_some_unknown", "Some records could not be checked from the internet just now.", "Nobody yet.", "Nothing to do now.", "This page asks again in a minute.", vars)
		}
	}
	if backup.Role == "backup" && backup.Answers == "waiting" {
		vars["backup"], vars["backup_who"] = label(backup), backup.Who
		return dnsOverviewSay("waiting", "dns_overview_backup_first_copy", "The backup DNS server has not received its first copy of the list yet.", "Nobody: it is copied by itself.", "Nothing to do.", "It answers for the domain once it has its copy; this page checks every minute.", vars)
	}
	for _, name := range r.Names {
		if (name.State == "missing" || name.State == "different") && name.Own == "unknown" {
			vars["name"] = name.Name
			return dnsOverviewSay("unknown", "dns_overview_some_unknown", "A name does not show its address on the internet, and our own DNS server could not be asked just now.", "Nobody yet.", "Nothing to do now.", "This page asks again in a minute.", vars)
		}
	}
	for _, name := range r.Names {
		if name.State == "missing" || name.State == "different" || name.State == "not_yet" {
			vars["name"] = name.Name
			return dnsOverviewSay("waiting", "dns_overview_spreading", "A name does not show its new address everywhere on the internet yet.", "Nobody: waiting is enough.", "Nothing to do.", "This page checks again every minute; a change can take a few hours to spread.", vars)
		}
	}
	for _, mail := range r.Mail {
		if mail.State == "missing" || mail.State == "different" {
			vars["kind"] = mail.Kind
			return dnsOverviewSay("waiting", "dns_overview_mail_spreading", "A mail record does not show everywhere on the internet yet.", "Nobody: waiting is enough.", "Nothing to do.", "This page checks again every minute.", vars)
		}
	}
	for _, mail := range r.Mail {
		if mail.Kind == "mx" && mail.State == "owner" {
			vars["seen"] = strings.Join(mail.Seen, ", ")
			return dnsOverviewSay("ok", "dns_overview_ok_mail_elsewhere", "Your names look correct; the domain's MX names a mail server you set.", "Nobody.", "If mail for the domain should come to this server, change the MX in the record editor.", "This page checks again every minute.", vars)
		}
	}
	if r.MXMove != nil {
		vars["host"] = r.MXMove.ToHost
		return dnsOverviewSay("ok", "dns_overview_ok_mail_kept", "Your names look correct; the domain's mail still goes to your old mail server, as you chose.", "You, when you are ready.", "Move mail to the new server with the button under Mail records.", "Until then nothing changes for your mail.", vars)
	}
	return dnsOverviewSay("ok", "dns_overview_ok", "Your names look correct on the internet.", "Nobody.", "Nothing to do.", "This page checks again every minute.", vars)
}

type dnsOverviewFlight struct {
	done  chan struct{}
	value dnsOverview
	// ok is false when the check ended without an answer (it panicked):
	// waiting readers then say the result is not known, and never hang.
	ok bool
}

// dnsOverviewCache shares one check per panel; the zero value is ready.
// started is when the last check began (any state): a forced check ("Check
// now") is accepted at most once per dnsOverviewFreshInterval per panel.
type dnsOverviewCache struct {
	mu       sync.Mutex
	key      string
	at       time.Time
	ttl      time.Duration
	value    dnsOverview
	started  time.Time
	inflight map[string]*dnsOverviewFlight
}

// dnsOverviewFreshInterval bounds forced checks: one per panel per 15 s.
const dnsOverviewFreshInterval = 15 * time.Second

func dnsOverviewComplete(value dnsOverview) bool {
	if value.Sentence.State == "unknown" {
		return false
	}
	for _, server := range value.Servers {
		if server.Answers == "unknown" {
			return false
		}
	}
	return true
}

// dnsOverviewInFlight tells whether a check is running (tests wait on it).
func (p *Panel) dnsOverviewInFlight() bool {
	p.dnsOverviewCache.mu.Lock()
	defer p.dnsOverviewCache.mu.Unlock()
	return len(p.dnsOverviewCache.inflight) > 0
}

// dnsOverviewSeconds rounds a wait up to whole seconds, never below zero.
func dnsOverviewSeconds(d time.Duration) int {
	if d <= 0 {
		return 0
	}
	return int((d + time.Second - 1) / time.Second)
}

// timing sets when the shared answer expires and when a forced check is
// accepted again. The caller holds c.mu.
func (c *dnsOverviewCache) timing(result *dnsOverview, now time.Time) {
	result.NextCheckAfter = dnsOverviewSeconds(c.ttl - now.Sub(c.at))
	result.FreshAfter = dnsOverviewSeconds(dnsOverviewFreshInterval - now.Sub(c.started))
}

// dnsOverviewRunning: the check is running and its answer is not known yet
// (D-024).
func dnsOverviewRunning(in dnsOverviewInput, now time.Time) dnsOverview {
	return dnsOverview{Domain: in.Draft.MainDomain, Home: in.home(), SetupReady: in.Ready, Servers: []dnsOverviewServer{}, Names: []dnsOverviewName{}, Mail: []dnsOverviewMailRecord{},
		MailReceiver: dnsOverviewMailReceiver{Server: "none"}, OwnRecordsLink: dnsOverviewOwnRecordsLink(in), Join: dnsOverviewJoinFor(in, in.thisRole()),
		Sentence:  dnsOverviewSay("unknown", "dns_overview_check_running", "The check is still running.", "Nobody.", "Nothing to do.", "Its answer is shown when it arrives.", nil),
		CheckedAt: now.UTC().Format(time.RFC3339), NextCheckAfter: 5, FreshAfter: 5}
}

// dnsOverview answers the shared check. fresh asks for a new check ("Check
// now"); it is honoured at most once per panel every 15 s, and inside that
// window the check in flight or the latest answer is returned, FreshAfter
// saying when a forced check is accepted again. Checks of the same state
// are coalesced: a second caller waits for the first.
func (p *Panel) dnsOverview(ctx context.Context, now time.Time, fresh bool) (dnsOverview, error) {
	edges := p.dnsOverviewEdges()
	in, err := p.dnsOverviewInputs(ctx, edges)
	if err != nil {
		return dnsOverview{}, err
	}
	raw, _ := json.Marshal(in)
	key := string(raw)
	cache := &p.dnsOverviewCache
	cache.mu.Lock()
	if since := now.Sub(cache.started); fresh && since >= 0 && since < dnsOverviewFreshInterval {
		fresh = false
	}
	if !fresh && cache.key == key && now.Sub(cache.at) >= 0 && now.Sub(cache.at) < cache.ttl {
		result := cache.value
		cache.timing(&result, now)
		cache.mu.Unlock()
		return result, nil
	}
	if cache.inflight == nil {
		cache.inflight = map[string]*dnsOverviewFlight{}
	}
	if flight, running := cache.inflight[key]; running {
		cache.mu.Unlock()
		select {
		case <-flight.done:
			if !flight.ok {
				return dnsOverviewRunning(in, now), nil
			}
			return flight.value, nil
		case <-ctx.Done():
			return dnsOverviewRunning(in, now), nil
		}
	}
	flight := &dnsOverviewFlight{done: make(chan struct{})}
	cache.inflight[key] = flight
	cache.started = now
	cache.mu.Unlock()
	// Whatever happens to the check, its entry goes and its readers wake.
	defer func() {
		cache.mu.Lock()
		if cache.inflight[key] == flight {
			delete(cache.inflight, key)
		}
		cache.mu.Unlock()
		close(flight.done)
	}()

	result := p.checkDNSOverview(context.WithoutCancel(ctx), in, edges)
	ttl := dnsOverviewTTL
	if !dnsOverviewComplete(result) {
		ttl = dnsOverviewUnknownTTL
	}
	result.CheckedAt = now.UTC().Format(time.RFC3339)

	cache.mu.Lock()
	cache.key, cache.at, cache.ttl = key, now, ttl
	cache.timing(&result, now)
	cache.value = result
	flight.value, flight.ok = result, true
	cache.mu.Unlock()
	return result, nil
}

func (p *Panel) handleDNSOverview(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if !requireServerSetupAdmin(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		rejectRouteMethod(w, []string{http.MethodGet})
		return
	}
	value, err := p.dnsOverview(r.Context(), time.Now(), r.URL.Query().Get("fresh") == "1")
	if err != nil {
		writeServerError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(value)
}
