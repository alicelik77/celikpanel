package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/dnswire"
	"github.com/alicelik/celikpanel/internal/transport"
)

// GET /api/v1/setup/delegation?zone= (alpha.113): the registrar card's live
// check. For the setup's main domain on the owner's own DNS servers it says
// what to enter at the registrar (nameservers, ns1 = the main DNS server, and
// IP records (glue) only for nameservers inside the domain) and what the
// internet shows now: the parent zone's own servers asked without recursion
// (no cache can hide the change), each of our servers asked directly whether
// it answers the zone authoritatively, and whether they answer the imported
// records the same as before. It is a read: nothing is written, no setup
// step is started or woken, and one bounded answer is shared for a minute,
// computed apart from the caller so a closed page cannot cut it short.
// Alan adi firmasi karti: girilecekler ve internetin su anki yaniti; salt-okur.

const serverSetupDelegationPath = serverSetupPath + "/delegation"

// setupDelegationOptions are the recursive resolvers and the address map
// (port 53 in production); tests point them at loopback servers.
var setupDelegationOptions = func() dnswire.DelegationOptions {
	return dnswire.DelegationOptions{Resolvers: []string{"1.1.1.1:53", "8.8.8.8:53"}}
}

type serverSetupDelegationNS struct {
	Name       string `json:"name"`
	Server     string `json:"server"`
	IPv4       string `json:"ipv4"`
	IPv6       string `json:"ipv6,omitempty"`
	GlueNeeded bool   `json:"glue_needed"`
}

type serverSetupDelegationServer struct {
	Role   string `json:"role"`
	IP     string `json:"ip"`
	State  string `json:"state"`
	Serial uint32 `json:"serial,omitempty"`
}

type serverSetupDelegationDiff struct {
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	Expected []string `json:"expected"`
	Seen     []string `json:"seen"`
}

// serverSetupDelegationImported compares the records the setup publishes
// beside its own server names (imported and mail records, as published:
// merged SPF, MX and DMARC as the owner chose). Unknown counts records no
// server of ours answered readably (no reply, not authoritative, an error
// code): they are neither matched nor differences.
type serverSetupDelegationImported struct {
	Matched int                         `json:"matched"`
	Unknown int                         `json:"unknown"`
	Total   int                         `json:"total"`
	Diffs   []serverSetupDelegationDiff `json:"diffs"`
}

type serverSetupDelegationItem struct {
	ID    string            `json:"id"`
	State string            `json:"state"`
	Vars  map[string]string `json:"vars,omitempty"`
}

type serverSetupDelegation struct {
	Zone           string                        `json:"zone"`
	ExpectedNS     []serverSetupDelegationNS     `json:"expected_ns"`
	Parent         string                        `json:"parent,omitempty"`
	ParentNS       []string                      `json:"parent_ns"`
	ParentGlue     map[string][]string           `json:"parent_glue"`
	Servers        []serverSetupDelegationServer `json:"servers"`
	Imported       serverSetupDelegationImported `json:"imported"`
	Items          []serverSetupDelegationItem   `json:"items"`
	NowAllowed     bool                          `json:"now_allowed"`
	NowReason      string                        `json:"now_reason,omitempty"`
	CheckedAt      string                        `json:"checked_at"`
	NextCheckAfter int                           `json:"next_check_after"`
}

// serverSetupDelegationExpected: ns1 is the main DNS server at setup time.
func serverSetupDelegationExpected(d serverSetupDraft, ipv6 string) []serverSetupDelegationNS {
	mainIP, backupIP := d.LocalIP, d.PeerIP
	mainV6, backupV6 := ipv6, ""
	if serverSetupStandaloneDNS(d) {
		// One server keeping its own DNS: both names are this server.
		return []serverSetupDelegationNS{
			{Name: d.NS1, Server: "main", IPv4: d.LocalIP, IPv6: ipv6, GlueNeeded: serverSetupDNSNameWithin(d.NS1, d.MainDomain)},
			{Name: d.NS2, Server: "main", IPv4: d.LocalIP, IPv6: ipv6, GlueNeeded: serverSetupDNSNameWithin(d.NS2, d.MainDomain)},
		}
	}
	if d.DNSRole == transport.DNSPairRoleSecondary {
		mainIP, backupIP, mainV6, backupV6 = d.PeerIP, d.LocalIP, "", ipv6
	}
	return []serverSetupDelegationNS{
		{Name: d.NS1, Server: "main", IPv4: mainIP, IPv6: mainV6, GlueNeeded: serverSetupDNSNameWithin(d.NS1, d.MainDomain)},
		{Name: d.NS2, Server: "backup", IPv4: backupIP, IPv6: backupV6, GlueNeeded: serverSetupDNSNameWithin(d.NS2, d.MainDomain)},
	}
}

// serverSetupPublishedExpectation is a published record's value the way a
// DNS answer shows it: MX as "preference host", TXT as its joined text.
func serverSetupPublishedExpectation(record serverSetupInfrastructureDNSRecord) string {
	switch record.Type {
	case "MX":
		return strconv.Itoa(record.Priority) + " " + record.Content
	case "TXT":
		return joinTXTSegments(record.Content)
	}
	return record.Content
}

// serverSetupDelegationPublished lists the imported and mail records the
// setup's zone publishes (records as published, never the raw imports).
func serverSetupDelegationPublished(d serverSetupDraft) []serverSetupInfrastructureDNSRecord {
	if d.DNSRole != transport.DNSPairRolePrimary || d.InfrastructureDNS == nil || d.InfrastructureDNS.Zone != d.MainDomain {
		return nil
	}
	desired, err := serverSetupInfrastructureDNSDesired(d)
	if err != nil {
		return nil
	}
	out := []serverSetupInfrastructureDNSRecord{}
	for _, record := range desired {
		if record.Origin != "" {
			out = append(out, record)
		}
	}
	return out
}

// The parent-zone walk and our own servers' answers run side by side, each
// in its own time budget: a slow parent never turns our answers into
// mismatches, and slow servers of ours never hide the parent's answer.
var serverSetupDelegationBudget = 10 * time.Second

func (p *Panel) checkServerSetupDelegation(ctx context.Context, d serverSetupDraft, ipv6 string, options dnswire.DelegationOptions) serverSetupDelegation {
	address := func(ip string) string {
		if options.Address != nil {
			return options.Address(ip)
		}
		return net.JoinHostPort(ip, "53")
	}
	result := serverSetupDelegation{Zone: d.MainDomain, ExpectedNS: serverSetupDelegationExpected(d, ipv6), ParentNS: []string{}, ParentGlue: map[string][]string{}, Servers: []serverSetupDelegationServer{}, Items: []serverSetupDelegationItem{}, Imported: serverSetupDelegationImported{Diffs: []serverSetupDelegationDiff{}}}
	var wg sync.WaitGroup
	var delegation dnswire.DelegationResult
	var delegationErr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		walkCtx, cancel := context.WithTimeout(ctx, serverSetupDelegationBudget)
		defer cancel()
		delegation, delegationErr = dnswire.Delegation(walkCtx, d.MainDomain, options)
	}()
	asked := []serverSetupDelegationNS{}
	for _, ns := range result.ExpectedNS {
		if !slices.ContainsFunc(asked, func(other serverSetupDelegationNS) bool { return other.Server == ns.Server }) {
			asked = append(asked, ns)
		}
	}
	servers := make([]serverSetupDelegationServer, len(asked))
	var imported serverSetupDelegationImported
	importedItem := serverSetupDelegationItem{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		answerCtx, cancel := context.WithTimeout(ctx, serverSetupDelegationBudget)
		defer cancel()
		var inner sync.WaitGroup
		for index, ns := range asked {
			inner.Add(1)
			go func(index int, ns serverSetupDelegationNS) {
				defer inner.Done()
				server := serverSetupDelegationServer{Role: ns.Server, IP: ns.IPv4, State: "unknown"}
				if ns.IPv4 != "" {
					message, err := dnswire.Exchange(answerCtx, address(ns.IPv4), d.MainDomain, dnswire.TypeSOA, false)
					switch {
					case err != nil:
						server.State = "unknown"
					case message.Authoritative && message.RCode == 0 && len(message.Answer) > 0:
						server.State = "ok"
						server.Serial, _ = dnswire.SOASerial(message.Answer[0].Value)
					default:
						server.State = "not_yet"
					}
				}
				servers[index] = server
			}(index, ns)
		}
		inner.Wait()
		imported, importedItem = checkServerSetupPublishedAnswers(answerCtx, d, servers, address)
	}()
	wg.Wait()
	result.Servers = servers
	for _, server := range servers {
		state := server.State
		if server.Role == "backup" && state != "ok" {
			// The backup DNS server answers once it is set up and receives the
			// list: until then this waits, it is not a failure.
			state = "waiting"
		}
		result.Items = append(result.Items, serverSetupDelegationItem{ID: "answers_" + server.Role, State: state, Vars: map[string]string{"ip": server.IP}})
	}
	result.NowAllowed = servers[0].State == "ok"
	if !result.NowAllowed {
		result.NowReason = "server_setup_delegation_main_not_answering"
	}
	if delegationErr == nil {
		result.Parent, result.ParentNS, result.ParentGlue = delegation.Parent, delegation.NS, delegation.Glue
		if result.ParentNS == nil {
			result.ParentNS = []string{}
		}
		if result.ParentGlue == nil {
			result.ParentGlue = map[string][]string{}
		}
	}
	expectedNames := []string{}
	for _, ns := range result.ExpectedNS {
		expectedNames = append(expectedNames, ns.Name)
		if !ns.GlueNeeded {
			continue
		}
		for _, family := range []struct{ id, value string }{{"glue_v4", ns.IPv4}, {"glue_v6", ns.IPv6}} {
			if family.value == "" {
				continue
			}
			item := serverSetupDelegationItem{ID: family.id + ":" + ns.Name, Vars: map[string]string{"name": ns.Name, "ip": family.value}}
			if family.id == "glue_v6" {
				item.Vars["optional"] = "true"
			}
			switch {
			case delegationErr != nil:
				item.State = "unknown"
			case slices.Contains(result.ParentGlue[ns.Name], family.value):
				item.State = "ok"
			default:
				item.State = "not_yet"
				if seen := strings.Join(result.ParentGlue[ns.Name], ", "); seen != "" {
					item.Vars["seen"] = seen
				}
			}
			result.Items = append(result.Items, item)
		}
	}
	nsItem := serverSetupDelegationItem{ID: "nameservers", Vars: map[string]string{"expected": strings.Join(expectedNames, ", ")}}
	seen := append([]string(nil), result.ParentNS...)
	slices.Sort(seen)
	slices.Sort(expectedNames)
	switch {
	case delegationErr != nil:
		nsItem.State = "unknown"
	case !delegation.Registered:
		nsItem.State, nsItem.Vars["reason"] = "not_yet", "not_registered"
	case len(result.ParentNS) == 0:
		// The registry answers for the domain, with no nameservers for it.
		nsItem.State, nsItem.Vars["reason"] = "not_yet", "no_nameservers"
	case slices.Equal(seen, expectedNames):
		nsItem.State = "ok"
	default:
		nsItem.State = "not_yet"
		nsItem.Vars["seen"] = strings.Join(seen, ", ")
	}
	result.Items = append(result.Items, nsItem)
	if importedItem.ID != "" {
		result.Imported = imported
		result.Items = append(result.Items, importedItem)
	}
	return result
}

// checkServerSetupPublishedAnswers asks every answering server of ours each
// imported and mail record directly, as the zone publishes it: the same
// answer is what keeps the site and mail working through the nameserver
// change. Only a readable, authoritative answer (NOERROR or NXDOMAIN) that
// lacks a published value is a difference (not yet); no reply, a reply that
// is not authoritative or another response code is unknown (D-024). Values
// the owner added beside ours (a backup MX added in Domains) are allowed.
func checkServerSetupPublishedAnswers(ctx context.Context, d serverSetupDraft, servers []serverSetupDelegationServer, address func(string) string) (serverSetupDelegationImported, serverSetupDelegationItem) {
	imported := serverSetupDelegationImported{Diffs: []serverSetupDelegationDiff{}}
	type key struct{ name, kind string }
	expected := map[key][]string{}
	order := []key{}
	for _, record := range serverSetupDelegationPublished(d) {
		k := key{record.Name, record.Type}
		if _, seen := expected[k]; !seen {
			order = append(order, k)
		}
		expected[k] = append(expected[k], serverSetupPublishedExpectation(record))
	}
	if len(order) == 0 {
		return imported, serverSetupDelegationItem{}
	}
	imported.Total = len(order)
	var mu sync.Mutex
	var wg sync.WaitGroup
	limit := make(chan struct{}, 8)
	diffs := map[key]serverSetupDelegationDiff{}
	unknown := map[key]bool{}
	answered := false
	for _, server := range servers {
		if server.State != "ok" {
			continue
		}
		answered = true
		for _, k := range order {
			wg.Add(1)
			go func(ip string, k key) {
				defer wg.Done()
				limit <- struct{}{}
				defer func() { <-limit }()
				message, err := dnswire.Exchange(ctx, address(ip), k.name, serverSetupRecordTypes[k.kind], false)
				if err != nil || !message.Authoritative || (message.RCode != 0 && message.RCode != 3) {
					mu.Lock()
					unknown[k] = true
					mu.Unlock()
					return
				}
				seen := []string{}
				for _, rr := range message.Answer {
					if rr.Name == k.name && rr.Type == serverSetupRecordTypes[k.kind] {
						seen = append(seen, rr.Value)
					}
				}
				want := append([]string(nil), expected[k]...)
				slices.Sort(want)
				slices.Sort(seen)
				for _, value := range want {
					if !slices.Contains(seen, value) {
						mu.Lock()
						diffs[k] = serverSetupDelegationDiff{Name: k.name, Type: k.kind, Expected: want, Seen: seen}
						mu.Unlock()
						return
					}
				}
			}(server.IP, k)
		}
	}
	wg.Wait()
	if !answered {
		return imported, serverSetupDelegationItem{ID: "imported", State: "waiting", Vars: map[string]string{"total": strconv.Itoa(len(order))}}
	}
	for _, k := range order {
		if diff, failed := diffs[k]; failed {
			imported.Diffs = append(imported.Diffs, diff)
		} else if unknown[k] {
			imported.Unknown++
		} else {
			imported.Matched++
		}
	}
	state := "ok"
	switch {
	case len(imported.Diffs) > 0:
		state = "not_yet"
	case imported.Unknown > 0:
		state = "unknown"
	}
	return imported, serverSetupDelegationItem{ID: "imported", State: state, Vars: map[string]string{"matched": strconv.Itoa(imported.Matched), "total": strconv.Itoa(imported.Total)}}
}

type serverSetupDelegationFlight struct {
	done  chan struct{}
	value serverSetupDelegation
}

type serverSetupDelegationCache struct {
	mu       sync.Mutex
	key      string
	at       time.Time
	value    serverSetupDelegation
	inflight map[string]*serverSetupDelegationFlight
}

var setupDelegationCaches sync.Map

// serverSetupDelegation shares one check per minute. The check runs without
// the cache lock: a second caller for the same draft waits for the check in
// flight (or gives up when its own request ends) instead of starting
// another. An answer with an unknown item is not kept, so the next look asks
// the internet again.
func (p *Panel) serverSetupDelegation(ctx context.Context, d serverSetupDraft, now time.Time) serverSetupDelegation {
	ipv6 := ""
	if parsed := net.ParseIP(setupProviderRecordIPv6()); parsed != nil && parsed.To4() == nil && parsed.IsGlobalUnicast() && !parsed.IsPrivate() {
		ipv6 = parsed.String()
	}
	raw, _ := json.Marshal(struct {
		D    serverSetupDraft
		IPv6 string
	}{d, ipv6})
	key := string(raw)
	value, _ := setupDelegationCaches.LoadOrStore(p, &serverSetupDelegationCache{})
	cache := value.(*serverSetupDelegationCache)
	cache.mu.Lock()
	if cache.key == key && now.Sub(cache.at) >= 0 && now.Sub(cache.at) < serverSetupProviderRecordsTTL {
		result := cache.value
		cache.mu.Unlock()
		result.NextCheckAfter = int((serverSetupProviderRecordsTTL - now.Sub(cache.at)).Seconds())
		return result
	}
	if cache.inflight == nil {
		cache.inflight = map[string]*serverSetupDelegationFlight{}
	}
	if flight, running := cache.inflight[key]; running {
		cache.mu.Unlock()
		select {
		case <-flight.done:
			return flight.value
		case <-ctx.Done():
			// D-024: the check is running; its answer is not known yet.
			return serverSetupDelegation{Zone: d.MainDomain, ExpectedNS: []serverSetupDelegationNS{}, ParentNS: []string{}, ParentGlue: map[string][]string{}, Servers: []serverSetupDelegationServer{},
				Items: []serverSetupDelegationItem{{ID: "check_running", State: "unknown"}}, Imported: serverSetupDelegationImported{Diffs: []serverSetupDelegationDiff{}},
				NowReason: "server_setup_delegation_check_running", NextCheckAfter: 5}
		}
	}
	flight := &serverSetupDelegationFlight{done: make(chan struct{})}
	cache.inflight[key] = flight
	cache.mu.Unlock()

	result := p.checkServerSetupDelegation(context.WithoutCancel(ctx), d, ipv6, setupDelegationOptions())
	result.CheckedAt, result.NextCheckAfter = now.UTC().Format(time.RFC3339), int(serverSetupProviderRecordsTTL.Seconds())
	complete := !slices.ContainsFunc(result.Items, func(item serverSetupDelegationItem) bool { return item.State == "unknown" })
	flight.value = result

	cache.mu.Lock()
	if complete {
		cache.key, cache.at, cache.value = key, now, result
	}
	delete(cache.inflight, key)
	cache.mu.Unlock()
	close(flight.done)
	return result
}

func (p *Panel) handleServerSetupDelegation(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if !requireServerSetupAdmin(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	state, err := p.loadServerSetup(r.Context())
	if err != nil {
		writeServerError(w, err)
		return
	}
	d := state.Draft
	// Once the second server is bound (alpha.114) its address is part of the
	// run's plan, not of the draft: the card names both servers from it.
	if execution, err := p.latestServerSetupExecution(r.Context()); err == nil && execution != nil {
		if plan, err := p.loadServerSetupPlan(r.Context(), execution.PlanID); err == nil && reflect.DeepEqual(plan.Draft, d) {
			if bound, err := p.effectiveServerSetupPlan(r.Context(), plan, execution.ID); err == nil && bound.peerBound {
				d = bound.Draft
			}
		}
	}
	zone := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(r.URL.Query().Get("zone")), "."))
	if d.MainDomain == "" || zone != d.MainDomain || d.DNSMode != setupDNSModeLocal || d.NS1 == "" || d.NS2 == "" {
		writeCodedError(w, http.StatusConflict, "server_setup_delegation_not_applicable", "The registrar check applies to the setup's main domain kept on your own DNS servers.", "/setup")
		return
	}
	_ = json.NewEncoder(w).Encode(p.serverSetupDelegation(r.Context(), d, time.Now()))
}
