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
)

// GET /api/v1/setup/public-records?extra=a,b (alpha.112): what the public
// internet answers now for the main domain's well-known names (the apex,
// www, mail, webmail, autodiscover, _dmarc) and up to eight names the owner
// types. The internet gives no complete list of a domain's records, so only
// these names are read, and the screen says so. Two fixed public resolvers
// are asked; a name whose lookups failed is "unknown", never "none". The
// answer is shared for a minute. It writes nothing and starts nothing.
// Ana alan adinin bilinen adlari ve yazilan adlar icin genel kayitlar.

const serverSetupPublicRecordsPath = serverSetupPath + "/public-records"

const serverSetupMaxExtraNames = 8

type serverSetupPublicRecord struct {
	Name   string   `json:"name"`
	Type   string   `json:"type"`
	Values []string `json:"values"`
	State  string   `json:"state"`
	// NotImportable maps a value to why it cannot be imported:
	// "content_invalid", "one_per_name" or "too_many".
	NotImportable map[string]string `json:"not_importable,omitempty"`
}

type serverSetupPublicRecords struct {
	Domain         string                    `json:"domain"`
	Extras         []string                  `json:"extras"`
	Records        []serverSetupPublicRecord `json:"records"`
	CheckedAt      string                    `json:"checked_at"`
	NextCheckAfter int                       `json:"next_check_after"`
}

// setupPublicRecordResolvers are the public resolvers asked. Tests replace
// them; no request input can change them.
var setupPublicRecordResolvers = func() []setupProviderRecordLookup {
	lookups := []setupProviderRecordLookup{}
	for _, address := range []string{"1.1.1.1:53", "8.8.8.8:53"} {
		address := address
		lookups = append(lookups, func(ctx context.Context, name string, qtype uint16) ([]string, error) {
			return dnswire.QueryRecords(ctx, address, name, qtype)
		})
	}
	return lookups
}

var serverSetupRecordTypes = map[string]uint16{"A": dnswire.TypeA, "AAAA": dnswire.TypeAAAA, "CNAME": dnswire.TypeCNAME, "MX": dnswire.TypeMX, "TXT": dnswire.TypeTXT, "CAA": dnswire.TypeCAA}

type serverSetupRecordQuestion struct{ Name, Type string }

func serverSetupPublicRecordQuestions(domain string, extras []string) []serverSetupRecordQuestion {
	questions := []serverSetupRecordQuestion{}
	add := func(name string, types ...string) {
		for _, kind := range types {
			questions = append(questions, serverSetupRecordQuestion{name, kind})
		}
	}
	add(domain, "A", "AAAA", "MX", "TXT", "CAA")
	for _, label := range []string{"www", "mail", "webmail", "autodiscover"} {
		add(label+"."+domain, "CNAME", "A", "AAAA")
	}
	add("_dmarc."+domain, "TXT")
	for _, name := range extras {
		add(name, "CNAME", "A", "AAAA", "MX", "TXT")
	}
	return questions
}

// serverSetupPublicExtras turns ?extra= into names under the domain: a label
// ("shop") or a full name ending with the domain; anything else is refused.
func serverSetupPublicExtras(raw, domain string) ([]string, error) {
	extras := []string{}
	for _, item := range strings.Split(raw, ",") {
		item = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(item), "."))
		if item == "" {
			continue
		}
		if !strings.HasSuffix(item, "."+domain) && item != domain {
			item += "." + domain
		}
		name, ok := serverSetupRecordName(item)
		if !ok || !serverSetupDNSNameWithin(name, domain) {
			return nil, errors.New("extra names must be names under the main domain")
		}
		if !slices.Contains(extras, name) {
			extras = append(extras, name)
		}
	}
	if len(extras) > serverSetupMaxExtraNames {
		return nil, errors.New("too many extra names")
	}
	slices.Sort(extras)
	return extras, nil
}

// readServerSetupPublicRecords asks every question of every resolver within
// one bound. A CNAME owner keeps only its CNAME (its addresses belong to the
// target), as an imported alias would.
func readServerSetupPublicRecords(ctx context.Context, questions []serverSetupRecordQuestion, lookups []setupProviderRecordLookup) []serverSetupPublicRecord {
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	out := make([]serverSetupPublicRecord, len(questions))
	limit := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for index, question := range questions {
		wg.Add(1)
		go func(index int, question serverSetupRecordQuestion) {
			defer wg.Done()
			limit <- struct{}{}
			defer func() { <-limit }()
			values := []string{}
			answered, failures := 0, 0
			for _, lookup := range lookups {
				found, err := lookup(ctx, question.Name, serverSetupRecordTypes[question.Type])
				var dnsErr *net.DNSError
				switch {
				case err == nil:
					answered++
					for _, value := range found {
						if !slices.Contains(values, value) {
							values = append(values, value)
						}
					}
				case errors.As(err, &dnsErr) && dnsErr.IsNotFound:
					answered++
				default:
					failures++
				}
			}
			slices.Sort(values)
			state := "none"
			switch {
			case len(values) > 0:
				state = "found"
			case answered == 0 || failures > 0:
				state = "unknown"
			}
			out[index] = serverSetupPublicRecord{Name: question.Name, Type: question.Type, Values: values, State: state}
		}(index, question)
	}
	wg.Wait()
	aliased := map[string]bool{}
	for _, record := range out {
		if record.Type == "CNAME" && record.State == "found" {
			aliased[record.Name] = true
		}
	}
	kept := []serverSetupPublicRecord{}
	for _, record := range out {
		if aliased[record.Name] && record.Type != "CNAME" {
			continue
		}
		kept = append(kept, record)
	}
	return kept
}

// markServerSetupImportable says, per value, why it cannot be imported, by
// the same rule the review applies (serverSetupImportableContent): content
// the zone renderer refuses (a null MX, quotes in a TXT…), a second SPF or
// DMARC policy, or a value beyond the import limit.
func markServerSetupImportable(records []serverSetupPublicRecord, domain string) {
	importable := 0
	policies := map[string]bool{}
	for index := range records {
		record := &records[index]
		if record.State != "found" {
			continue
		}
		for _, value := range record.Values {
			reason := ""
			content := value
			if record.Type == "MX" {
				_, content = serverSetupMXValue(value)
				if content == "" {
					content = "."
				}
			}
			if _, _, err := serverSetupImportableContent(record.Type, content); err != nil {
				reason = "content_invalid"
			}
			if reason == "" {
				if kind := serverSetupPolicyKind(serverSetupImportedRecord{Name: record.Name, Type: record.Type, Content: content}, domain); kind != "" {
					if policies[kind] {
						reason = "one_per_name"
					}
					policies[kind] = true
				}
			}
			if reason == "" {
				importable++
				if importable > serverSetupMaxImportedRecords {
					reason = "too_many"
				}
			}
			if reason != "" {
				if record.NotImportable == nil {
					record.NotImportable = map[string]string{}
				}
				record.NotImportable[value] = reason
			}
		}
	}
}

type serverSetupPublicRecordsFlight struct {
	done  chan struct{}
	value serverSetupPublicRecords
}

type serverSetupPublicRecordsCache struct {
	mu       sync.Mutex
	key      string
	at       time.Time
	value    serverSetupPublicRecords
	inflight map[string]*serverSetupPublicRecordsFlight
}

var setupPublicRecordsCaches sync.Map

// serverSetupPublicRecords shares one read per minute. The read itself runs
// without the cache lock: a second caller for the same names waits for the
// read in flight instead of starting another. An answer with an unreadable
// name is not kept, so "Try again" asks the internet again.
func (p *Panel) serverSetupPublicRecords(ctx context.Context, domain string, extras []string, now time.Time) serverSetupPublicRecords {
	value, _ := setupPublicRecordsCaches.LoadOrStore(p, &serverSetupPublicRecordsCache{})
	cache := value.(*serverSetupPublicRecordsCache)
	key := domain + "|" + strings.Join(extras, ",")
	cache.mu.Lock()
	if cache.key == key && now.Sub(cache.at) >= 0 && now.Sub(cache.at) < serverSetupProviderRecordsTTL {
		result := cache.value
		cache.mu.Unlock()
		result.NextCheckAfter = int((serverSetupProviderRecordsTTL - now.Sub(cache.at)).Seconds())
		return result
	}
	if cache.inflight == nil {
		cache.inflight = map[string]*serverSetupPublicRecordsFlight{}
	}
	if flight, running := cache.inflight[key]; running {
		cache.mu.Unlock()
		<-flight.done
		return flight.value
	}
	flight := &serverSetupPublicRecordsFlight{done: make(chan struct{})}
	cache.inflight[key] = flight
	cache.mu.Unlock()

	records := readServerSetupPublicRecords(context.WithoutCancel(ctx), serverSetupPublicRecordQuestions(domain, extras), setupPublicRecordResolvers())
	markServerSetupImportable(records, domain)
	flight.value = serverSetupPublicRecords{Domain: domain, Extras: extras, Records: records, CheckedAt: now.UTC().Format(time.RFC3339), NextCheckAfter: int(serverSetupProviderRecordsTTL.Seconds())}
	complete := !slices.ContainsFunc(records, func(record serverSetupPublicRecord) bool { return record.State == "unknown" })

	cache.mu.Lock()
	if complete {
		cache.key, cache.at, cache.value = key, now, flight.value
	}
	delete(cache.inflight, key)
	cache.mu.Unlock()
	close(flight.done)
	return flight.value
}

func (p *Panel) handleServerSetupPublicRecords(w http.ResponseWriter, r *http.Request) {
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
	if state.Draft.MainDomain == "" {
		writeCodedError(w, http.StatusConflict, "server_setup_main_domain_required", "Enter the main domain first; its records are read for that domain only.", "/setup")
		return
	}
	extras, err := serverSetupPublicExtras(r.URL.Query().Get("extra"), state.Draft.MainDomain)
	if err != nil {
		writeClientError(w, http.StatusBadRequest, err.Error())
		return
	}
	_ = json.NewEncoder(w).Encode(p.serverSetupPublicRecords(r.Context(), state.Draft.MainDomain, extras, time.Now()))
}
