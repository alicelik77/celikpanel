package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/dnswire"
	"github.com/alicelik/celikpanel/internal/transport"
)

// GET /api/v1/setup/provider-records (alpha.111): the records the owner adds
// at an external DNS provider for this server, each with what the public
// internet answers for it now. It is a read: the panel writes nothing to any
// provider (no API automation in this release), changes nothing on the
// server and never starts or wakes a setup operation. The answer is shared
// for about a minute, so a page that polls it asks the internet at most once
// per minute. A lookup that failed is "unknown", never "missing" (D-024,
// D-036).
//
// Harici DNS saglayicisina eklenecek kayitlar ve internetin su anki yaniti.
// Yalniz okuma; yaklasik bir dakika paylasilir; basarisiz sorgu "bilinmiyor".

const serverSetupProviderRecordsPath = serverSetupPath + "/provider-records"

const serverSetupProviderRecordsTTL = 60 * time.Second

type serverSetupProviderRecordRow struct {
	Name  string   `json:"name"`
	Type  string   `json:"type"`
	Value string   `json:"value"`
	State string   `json:"state"`
	Seen  []string `json:"seen"`
	// Kind (alpha.112) is "" for an address record, otherwise the mail record
	// it is: "mx", "spf", "dmarc" or "dkim". Their states add "kept" (an old
	// record the owner chose to keep) and "pending" (the DKIM key is made by
	// the mail step; the value is shown once it exists).
	Kind string `json:"kind,omitempty"`
}

type serverSetupProviderRecordServer struct {
	Label string                         `json:"label"`
	IP4   string                         `json:"ip4,omitempty"`
	IP6   string                         `json:"ip6,omitempty"`
	Rows  []serverSetupProviderRecordRow `json:"rows"`
}

type serverSetupProviderRecords struct {
	Mode           string                            `json:"mode"`
	Servers        []serverSetupProviderRecordServer `json:"servers"`
	CheckedAt      string                            `json:"checked_at"`
	NextCheckAfter int                               `json:"next_check_after"`
}

// setupProviderRecordLookup asks one public resolver one question.
type setupProviderRecordLookup func(ctx context.Context, name string, qtype uint16) ([]string, error)

// setupProviderRecordResolvers are the public resolvers asked. Tests replace
// them with controlled ones; no request input can change them.
var setupProviderRecordResolvers = func() []setupProviderRecordLookup {
	lookups := []setupProviderRecordLookup{}
	for _, address := range []string{"1.1.1.1:53", "8.8.8.8:53"} {
		address := address
		lookups = append(lookups, func(ctx context.Context, name string, qtype uint16) ([]string, error) {
			return dnswire.QueryRecords(ctx, address, name, qtype)
		})
	}
	return lookups
}

type serverSetupProviderRecordsCache struct {
	mu    sync.Mutex
	key   string
	at    time.Time
	value serverSetupProviderRecords
}

var setupProviderRecordsCaches sync.Map

func (p *Panel) serverSetupProviderRecordsCache() *serverSetupProviderRecordsCache {
	value, _ := setupProviderRecordsCaches.LoadOrStore(p, &serverSetupProviderRecordsCache{})
	return value.(*serverSetupProviderRecordsCache)
}

// setupProviderRecordIPv6 is this server's global IPv6 address (tests replace
// it so the CI host's own address never leaks into a result).
var setupProviderRecordIPv6 = serverPrimaryIPv6

// serverSetupProviderRecordRows lists the address records this server needs at
// an external provider: the panel name and, with mail, the mail host name,
// each as an A record to the server's public IPv4 address and, when the server
// has a global IPv6 address, an AAAA record to it (a stale AAAA at the
// provider would break certificate issuance over IPv6).
func serverSetupProviderRecordRows(draft serverSetupDraft, ip4, ip6 string) []serverSetupProviderRecordRow {
	rows := []serverSetupProviderRecordRow{}
	if ip4 == "" {
		return rows
	}
	names := []string{draft.PanelDomain}
	if len(serverSetupMailProfileIDs(draft)) > 0 {
		names = append(names, draft.MailHostname)
	}
	for _, name := range names {
		if name == "" || slices.ContainsFunc(rows, func(row serverSetupProviderRecordRow) bool { return row.Name == name && row.Type == "A" }) {
			continue
		}
		rows = append(rows, serverSetupProviderRecordRow{Name: name, Type: "A", Value: ip4, State: "unknown", Seen: []string{}})
		if ip6 != "" {
			rows = append(rows, serverSetupProviderRecordRow{Name: name, Type: "AAAA", Value: ip6, State: "unknown", Seen: []string{}})
		}
	}
	return rows
}

// classifyServerSetupProviderRecord turns the resolvers' answers into one
// state: "different" when any answers another address, "ok" when every
// resolver answers exactly the expected one, "missing" when the answers seen
// lack it (none, or not everywhere yet), and "unknown" when a resolver could
// not be read and nothing contradicts the record.
func classifyServerSetupProviderRecord(expected string, answers [][]string, failures int) (string, []string) {
	seen := []string{}
	other := false
	matched, empty := 0, 0
	for _, values := range answers {
		if len(values) == 0 {
			empty++
			continue
		}
		hasExpected := false
		for _, value := range values {
			if !slices.Contains(seen, value) {
				seen = append(seen, value)
			}
			if value == expected {
				hasExpected = true
			} else {
				other = true
			}
		}
		if hasExpected {
			matched++
		}
	}
	slices.Sort(seen)
	switch {
	case other:
		return "different", seen
	case failures > 0:
		return "unknown", seen
	case matched > 0 && empty == 0:
		return "ok", seen
	default:
		return "missing", seen
	}
}

func (p *Panel) checkServerSetupProviderRecords(ctx context.Context, rows []serverSetupProviderRecordRow, lookups []setupProviderRecordLookup) []serverSetupProviderRecordRow {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	out := make([]serverSetupProviderRecordRow, len(rows))
	var wg sync.WaitGroup
	for index, row := range rows {
		wg.Add(1)
		go func(index int, row serverSetupProviderRecordRow) {
			defer wg.Done()
			answers := [][]string{}
			failures := 0
			if row.State == "pending" {
				out[index] = row
				return
			}
			for _, lookup := range lookups {
				qtype := serverSetupRecordTypes[row.Type]
				values, err := lookup(ctx, row.Name, qtype)
				var dnsErr *net.DNSError
				switch {
				case err == nil:
					answers = append(answers, values)
				case errors.As(err, &dnsErr) && dnsErr.IsNotFound:
					answers = append(answers, nil)
				default:
					failures++
				}
			}
			if row.Kind != "" {
				row.State, row.Seen, row.Value = classifyServerSetupMailRecord(row, answers, failures)
			} else {
				row.State, row.Seen = classifyServerSetupProviderRecord(row.Value, answers, failures)
			}
			if len(lookups) == 0 {
				row.State = "unknown"
			}
			out[index] = row
		}(index, row)
	}
	wg.Wait()
	return out
}

func (p *Panel) serverSetupProviderRecords(ctx context.Context, draft serverSetupDraft, now time.Time) serverSetupProviderRecords {
	ip4, _ := canonicalIPv4(serverPrimaryIP())
	ip6 := ""
	if parsed := net.ParseIP(setupProviderRecordIPv6()); parsed != nil && parsed.To4() == nil && parsed.IsGlobalUnicast() && !parsed.IsPrivate() {
		ip6 = parsed.String()
	}
	server := serverSetupProviderRecordServer{Label: draft.ShortName, IP4: ip4, IP6: ip6, Rows: serverSetupProviderRecordRows(draft, ip4, ip6)}
	if draft.DNSMode == setupDNSModeExternal && serverSetupHasMail(draft) {
		var dkim transport.DKIMStatusResponse
		if err := p.callAgentContext(ctx, "Agent.GetDKIMStatus", &transport.DKIMStatusRequest{Domain: draft.MainDomain, Selector: dkimSelector}, &dkim); err != nil || dkim.Error != "" || !dkim.HasKey {
			dkim.PublicKeyB64 = ""
		}
		server.Rows = append(server.Rows, serverSetupProviderMailRows(draft, dkim.PublicKeyB64)...)
	}
	key := draft.DNSMode + "|" + server.Label + "|" + ip4 + "|" + ip6
	for _, row := range server.Rows {
		key += "|" + row.Name + "/" + row.Type + "=" + row.Value
	}
	cache := p.serverSetupProviderRecordsCache()
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if cache.key == key && now.Sub(cache.at) < serverSetupProviderRecordsTTL && now.Sub(cache.at) >= 0 {
		result := cache.value
		result.NextCheckAfter = int((serverSetupProviderRecordsTTL - now.Sub(cache.at)).Seconds())
		return result
	}
	// The shared answer must not depend on one caller: a closed tab would
	// otherwise cache "unknown" for everyone. The check keeps its own bound.
	server.Rows = p.checkServerSetupProviderRecords(context.WithoutCancel(ctx), server.Rows, setupProviderRecordResolvers())
	result := serverSetupProviderRecords{Mode: draft.DNSMode, Servers: []serverSetupProviderRecordServer{server}, CheckedAt: now.UTC().Format(time.RFC3339), NextCheckAfter: int(serverSetupProviderRecordsTTL.Seconds())}
	cache.key, cache.at, cache.value = key, now, result
	return result
}

func (p *Panel) handleServerSetupProviderRecords(w http.ResponseWriter, r *http.Request) {
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
	_ = json.NewEncoder(w).Encode(p.serverSetupProviderRecords(r.Context(), state.Draft, time.Now()))
}

// serverSetupProviderMailRows are the main domain's mail records the owner
// adds at an external provider (alpha.112). The SPF value is completed with
// the provider's current policy when it is read (merged, never replaced).
func serverSetupProviderMailRows(draft serverSetupDraft, dkimPublic string) []serverSetupProviderRecordRow {
	domain := draft.MainDomain
	rows := []serverSetupProviderRecordRow{
		{Name: domain, Type: "MX", Value: "10 " + draft.MailHostname, Kind: "mx", State: "unknown", Seen: []string{}},
		{Name: domain, Type: "TXT", Value: mergeServerSetupSPF("", draft.MailHostname), Kind: "spf", State: "unknown", Seen: []string{}},
		{Name: "_dmarc." + domain, Type: "TXT", Value: dmarcRecommended(domain, "none"), Kind: "dmarc", State: "unknown", Seen: []string{}},
	}
	// At an external provider the old MX is kept unless the owner moves mail
	// now; an unanswered choice is "later", as the mail step shows.
	if draft.MailDNS == nil || draft.MailDNS.MX != "now" {
		rows[0].Kind = "mx-later"
	}
	if draft.MailDNS != nil && draft.MailDNS.DMARC == "add" {
		rows[2].Kind = "dmarc-add"
	}
	dkim := serverSetupProviderRecordRow{Name: dkimSelector + "._domainkey." + domain, Type: "TXT", Kind: "dkim", State: "pending", Seen: []string{}}
	if dkimPublic != "" {
		dkim.Value, dkim.State = dkimRecommended(dkimPublic), "unknown"
	}
	return append(rows, dkim)
}

// classifyServerSetupMailRecord: a mail record is judged by its own kind of
// value only (an apex has other TXT records). The SPF expected value is the
// provider's current policy with this mail server added.
func classifyServerSetupMailRecord(row serverSetupProviderRecordRow, answers [][]string, failures int) (string, []string, string) {
	mailHost := ""
	if _, host := serverSetupMXValue(row.Value); host != "" {
		mailHost = host
	}
	seen := []string{}
	for _, values := range answers {
		for _, value := range values {
			lower := strings.ToLower(value)
			relevant := strings.HasPrefix(row.Kind, "mx") || (row.Kind == "spf" && strings.HasPrefix(lower, "v=spf1")) ||
				(strings.HasPrefix(row.Kind, "dmarc") && strings.HasPrefix(lower, "v=dmarc1")) || (row.Kind == "dkim" && strings.HasPrefix(lower, "v=dkim1"))
			if relevant && !slices.Contains(seen, value) {
				seen = append(seen, value)
			}
		}
	}
	slices.Sort(seen)
	if len(seen) == 0 {
		if failures > 0 {
			return "unknown", seen, row.Value
		}
		return "missing", seen, row.Value
	}
	switch row.Kind {
	case "mx", "mx-later":
		for _, value := range seen {
			if _, host := serverSetupMXValue(value); host == mailHost {
				return "ok", seen, row.Value
			}
		}
		if row.Kind == "mx-later" {
			// The owner keeps the old mail server for now (alpha.112 rule:
			// MX never moves without the owner).
			return "kept", seen, row.Value
		}
		return "different", seen, row.Value
	case "spf":
		parts := strings.Fields(row.Value)
		mechanism := parts[len(parts)-2]
		merged := mergeServerSetupSPF(seen[0], strings.TrimPrefix(mechanism, "a:"))
		if merged == strings.Join(strings.Fields(seen[0]), " ") {
			return "ok", seen, merged
		}
		return "different", seen, merged
	case "dmarc":
		return "ok", seen, row.Value
	case "dmarc-add":
		if slices.Contains(seen, row.Value) {
			return "ok", seen, row.Value
		}
		return "different", seen, row.Value
	default:
		if slices.Contains(seen, row.Value) {
			return "ok", seen, row.Value
		}
		return "different", seen, row.Value
	}
}
