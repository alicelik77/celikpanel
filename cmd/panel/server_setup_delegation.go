package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
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

type serverSetupDelegationImported struct {
	Matched int                         `json:"matched"`
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
	if d.DNSRole == transport.DNSPairRoleSecondary {
		mainIP, backupIP, mainV6, backupV6 = d.PeerIP, d.LocalIP, "", ipv6
	}
	return []serverSetupDelegationNS{
		{Name: d.NS1, Server: "main", IPv4: mainIP, IPv6: mainV6, GlueNeeded: serverSetupDNSNameWithin(d.NS1, d.MainDomain)},
		{Name: d.NS2, Server: "backup", IPv4: backupIP, IPv6: backupV6, GlueNeeded: serverSetupDNSNameWithin(d.NS2, d.MainDomain)},
	}
}

func serverSetupImportedExpectation(record serverSetupImportedRecord) string {
	if record.Type == "MX" {
		return strings.TrimSpace(strconv.Itoa(record.Priority) + " " + record.Content)
	}
	return record.Content
}

func (p *Panel) checkServerSetupDelegation(ctx context.Context, d serverSetupDraft, ipv6 string, options dnswire.DelegationOptions) serverSetupDelegation {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
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
		delegation, delegationErr = dnswire.Delegation(ctx, d.MainDomain, options)
	}()
	servers := make([]serverSetupDelegationServer, len(result.ExpectedNS))
	for index, ns := range result.ExpectedNS {
		wg.Add(1)
		go func(index int, ns serverSetupDelegationNS) {
			defer wg.Done()
			server := serverSetupDelegationServer{Role: ns.Server, IP: ns.IPv4, State: "unknown"}
			if ns.IPv4 != "" {
				message, err := dnswire.Exchange(ctx, address(ns.IPv4), d.MainDomain, dnswire.TypeSOA, false)
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
	case slices.Equal(seen, expectedNames):
		nsItem.State = "ok"
	default:
		nsItem.State = "not_yet"
		nsItem.Vars["seen"] = strings.Join(seen, ", ")
	}
	result.Items = append(result.Items, nsItem)
	p.checkServerSetupImportedAnswers(ctx, d, servers, address, &result)
	return result
}

// checkServerSetupImportedAnswers asks every answering server of ours each
// imported record directly: the same answer as before is what keeps the
// site and mail working through the nameserver change.
func (p *Panel) checkServerSetupImportedAnswers(ctx context.Context, d serverSetupDraft, servers []serverSetupDelegationServer, address func(string) string, result *serverSetupDelegation) {
	type key struct{ name, kind string }
	expected := map[key][]string{}
	order := []key{}
	for _, record := range serverSetupImported(d) {
		k := key{record.Name, record.Type}
		if _, seen := expected[k]; !seen {
			order = append(order, k)
		}
		expected[k] = append(expected[k], serverSetupImportedExpectation(record))
	}
	if len(order) == 0 {
		return
	}
	result.Imported.Total = len(order)
	var mu sync.Mutex
	var wg sync.WaitGroup
	limit := make(chan struct{}, 8)
	diffs := map[key]serverSetupDelegationDiff{}
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
				seen := []string{}
				if err == nil {
					for _, rr := range message.Answer {
						if rr.Name == k.name && rr.Type == serverSetupRecordTypes[k.kind] {
							seen = append(seen, rr.Value)
						}
					}
				}
				want := append([]string(nil), expected[k]...)
				slices.Sort(want)
				slices.Sort(seen)
				if err != nil || !slices.Equal(want, seen) {
					mu.Lock()
					diffs[k] = serverSetupDelegationDiff{Name: k.name, Type: k.kind, Expected: want, Seen: seen}
					mu.Unlock()
				}
			}(server.IP, k)
		}
	}
	wg.Wait()
	if !answered {
		result.Items = append(result.Items, serverSetupDelegationItem{ID: "imported", State: "waiting", Vars: map[string]string{"total": strconv.Itoa(len(order))}})
		return
	}
	for _, k := range order {
		if diff, failed := diffs[k]; failed {
			result.Imported.Diffs = append(result.Imported.Diffs, diff)
		} else {
			result.Imported.Matched++
		}
	}
	state := "ok"
	if len(result.Imported.Diffs) > 0 {
		state = "not_yet"
	}
	result.Items = append(result.Items, serverSetupDelegationItem{ID: "imported", State: state, Vars: map[string]string{"matched": strconv.Itoa(result.Imported.Matched), "total": strconv.Itoa(result.Imported.Total)}})
}

type serverSetupDelegationCache struct {
	mu    sync.Mutex
	key   string
	at    time.Time
	value serverSetupDelegation
}

var setupDelegationCaches sync.Map

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
	defer cache.mu.Unlock()
	if cache.key == key && now.Sub(cache.at) >= 0 && now.Sub(cache.at) < serverSetupProviderRecordsTTL {
		result := cache.value
		result.NextCheckAfter = int((serverSetupProviderRecordsTTL - now.Sub(cache.at)).Seconds())
		return result
	}
	result := p.checkServerSetupDelegation(context.WithoutCancel(ctx), d, ipv6, setupDelegationOptions())
	result.CheckedAt, result.NextCheckAfter = now.UTC().Format(time.RFC3339), int(serverSetupProviderRecordsTTL.Seconds())
	cache.key, cache.at, cache.value = key, now, result
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
	zone := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(r.URL.Query().Get("zone")), "."))
	if d.MainDomain == "" || zone != d.MainDomain || d.DNSMode != setupDNSModeLocal || d.NS1 == "" || d.NS2 == "" {
		writeCodedError(w, http.StatusConflict, "server_setup_delegation_not_applicable", "The registrar check applies to the setup's main domain kept on your own DNS servers.", "/setup")
		return
	}
	_ = json.NewEncoder(w).Encode(p.serverSetupDelegation(r.Context(), d, time.Now()))
}
