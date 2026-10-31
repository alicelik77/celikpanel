package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Hosting capabilities — what this server can actually host right now. The
// Add Domain dialog reads this to offer only choices that can work: a php
// site needs a web server + PHP-FPM, a static site needs a web server, a
// DNS-only domain needs nothing (its zone is served once a DNS server runs,
// or DNS stays at an external provider). The requirement follows the role
// the domain will play on this server — not a fixed service list.
//
// Barındırma yetenekleri — bu sunucunun şu anda gerçekten neyi
// barındırabildiği. Add Domain penceresi bunu okuyup yalnız çalışabilecek
// seçenekleri sunar: php sitesi web sunucusu + PHP-FPM ister, statik site web
// sunucusu ister, yalnız-DNS domain hiçbir şey istemez (zone'u bir DNS
// sunucusu koşunca sunulur ya da DNS dış sağlayıcıda kalır). Gereksinim,
// domain'in bu sunucuda üstleneceği ROLÜ izler — sabit bir servis listesini
// değil.

type hostingCapabilities struct {
	DNSManagementMode  string   `json:"dns_management_mode"`
	DNSManagementReady bool     `json:"dns_management_ready"`
	WebServer          string   `json:"web_server"`   // "nginx" or "" when no supported adapter is available
	PHPVersions        []string `json:"php_versions"` // installed FPM versions, newest first
	DNSServer          string   `json:"dns_server"`   // proven active "pdns", "bind" or ""
	// Why dns_server is "" (owner report 2026-10-31): none, switching,
	// port53_conflict, not_running, engine_conflict, secondary, pair_pending,
	// or ready. Additive; dns_server keeps deciding what is allowed.
	// dns_server neden bos: ekran bunu aciklar; izin degismez.
	DNSServerState   string `json:"dns_server_state"`
	DNSServerEngine  string `json:"dns_server_engine,omitempty"` // the active engine whenever one is active
	DNSServerPeer    string `json:"dns_server_peer,omitempty"`   // the other server of a DNS pair
	DNSIdentityReady bool   `json:"dns_identity_ready"`          // saved nameserver pair and operating mode
	MailServer       bool   `json:"mail_server"`                 // postfix present
	// All installed database engines — a server can legitimately run both
	// MariaDB and PostgreSQL side by side (no conflict group).
	// Kurulu tüm veritabanı motorları — bir sunucu MariaDB ve PostgreSQL'i
	// yan yana meşru biçimde koşturabilir (çakışma grubu yok).
	DatabaseServers []string `json:"database_servers"`
	// Installed database web tools (phpmyadmin, phppgadmin) — the Databases
	// pages show a launch button for each, reverse-proxied at /dbtool/<id>/.
	// Kurulu veritabanı web araçları (phpmyadmin, phppgadmin) — Veritabanı
	// sayfaları her biri için /dbtool/<id>/ üzerinden bir açma düğmesi gösterir.
	DBTools []string `json:"db_tools"`
}

func supportedHostingWebServer(installed []string) string {
	for _, id := range installed {
		if id == "nginx" {
			return "nginx"
		}
	}
	return ""
}

// hostingCaps computes the current capability set from installed services
// (package presence via the agent) plus the real PHP trees on disk.
// hostingCaps, mevcut yetenek kümesini kurulu servislerden (agent üzerinden
// paket varlığı) ve diskteki gerçek PHP ağaçlarından hesaplar.
func (p *Panel) hostingCaps(ctx context.Context) (hostingCapabilities, error) {
	// Slice fields start non-nil (empty, not nil) so the JSON contract is
	// always `[]`, never `null` — a nil Go slice marshals to `null`, and a
	// consumer doing `arr[0]` or `arr.length` on that throws. Caught live: on
	// a fresh server (nothing installed) the Add Domain dialog's capability
	// fetch crashed on `php_versions[0]`, and its catch handler silently reset
	// the whole capability state to null — which made every requirement check
	// evaluate as "unknown" and, for the DNS-only type, fail open. A list is
	// either populated or empty; it is never absent.
	// Dizi alanları boş-ama-nil-olmayan başlar; JSON sözleşmesi her zaman
	// `[]`dir, asla `null` değil — nil bir Go dilimi `null`a dönüşür ve bunun
	// üzerinde `arr[0]` ya da `arr.length` yapan bir tüketici çöker. Canlıda
	// yakalandı: taze bir sunucuda (hiçbir şey kurulu değilken) Add Domain
	// penceresinin yetenek çekimi `php_versions[0]`da çöktü ve catch bloğu
	// tüm yetenek durumunu sessizce null'a sıfırladı — bu da her gereksinim
	// denetimini "bilinmiyor"a çevirdi ve yalnız-DNS tipinde açık-kalmaya
	// (fail open) yol açtı. Bir liste ya doludur ya boştur; asla yok değildir.
	caps := hostingCapabilities{
		PHPVersions:     []string{},
		DatabaseServers: []string{},
		DBTools:         []string{},
	}

	var installed []string
	if err := p.callAgentContext(ctx, "Agent.InstalledServiceIDsStrict", &transport.Empty{}, &installed); err != nil {
		return hostingCapabilities{}, fmt.Errorf("discover installed hosting services: %w", err)
	}
	caps.WebServer = supportedHostingWebServer(installed)
	phpInstalled := false
	for _, id := range installed {
		switch id {
		case "php-fpm":
			phpInstalled = true
		case "postfix":
			caps.MailServer = true
		case "mariadb", "postgresql":
			caps.DatabaseServers = append(caps.DatabaseServers, id)
		case "phpmyadmin", "phppgadmin":
			caps.DBTools = append(caps.DBTools, id)
		}
	}

	mode, err := p.setupDNSManagementMode(ctx)
	if err != nil {
		return hostingCapabilities{}, err
	}
	caps.DNSManagementMode = mode
	caps.DNSManagementReady = mode == setupDNSModeExternal
	status, err := p.dnsPublisherStatus(ctx)
	if err != nil && mode == setupDNSModeLocal {
		return hostingCapabilities{}, fmt.Errorf("verify active DNS publisher: %w", err)
	}
	publisher, publisherReady := status.Identity, err == nil && status.State == dnsServerStateReady
	if err == nil {
		caps.DNSServerState = status.State
		caps.DNSServerEngine = string(status.Engine)
		caps.DNSServerPeer = status.Peer
	}
	if publisherReady {
		caps.DNSServer = string(publisher.Engine)
		identityReady, err := p.savedDNSIdentityConfiguredStrict(ctx)
		if err != nil {
			return hostingCapabilities{}, err
		}
		caps.DNSIdentityReady = identityReady
		if mode == setupDNSModeLocal {
			caps.DNSManagementReady = identityReady
		}
	}

	if mode == setupDNSModeExisting {
		_, err := p.remoteDNSConnectionForCreation(ctx, "")
		caps.DNSManagementReady = err == nil
	}

	if phpInstalled {
		versions, err := p.phpVersionsFromAgent(ctx)
		if err != nil {
			return hostingCapabilities{}, err
		}
		if len(versions) == 0 {
			return hostingCapabilities{}, fmt.Errorf("PHP-FPM is installed but no managed version could be discovered")
		}
		caps.PHPVersions = versions
	}
	return caps, nil
}

// phpVersionsFromAgent asks the one per-version authority,
// Agent.ListServiceInstances (B3b). The old source read /etc/php/<v>/fpm on
// the PANEL process's own disk — a Debian-only layout, so an Arch server
// running PHP reported php_versions: [] and Add Domain refused PHP sites with
// PHP_REQUIRED while php-fpm was actively serving. Agent failures are not
// converted into a local guess: callers must never confuse "unknown" with
// "not installed".
// phpVersionsFromAgent, sürüm-başına tek otoriteye, Agent.ListServiceInstances'a
// sorar (B3b). Eski kaynak PANEL sürecinin kendi diskinden /etc/php/<v>/fpm
// okuyordu — yalnız-Debian düzeni; PHP koşturan bir Arch sunucusu
// php_versions: [] bildiriyor ve Add Domain, php-fpm fiilen site sunarken PHP
// sitelerini PHP_REQUIRED ile geri çeviriyordu. Agent hataları yerel bir
// tahmine dönüştürülmez; çağıran "bilinmiyor" ile "kurulu değil"i
// karıştırmamalıdır.
func (p *Panel) phpVersionsFromAgent(ctx context.Context) ([]string, error) {
	var ir transport.ServiceInstancesResponse
	req := transport.ServiceInstancesRequest{ID: "php-fpm"}
	if err := p.callAgentContext(ctx, "Agent.ListServiceInstances", &req, &ir); err != nil {
		return nil, fmt.Errorf("discover installed PHP versions: %w", err)
	}
	versions := []string{}
	for _, in := range ir.Instances {
		if in.Managed && in.Version != "" {
			versions = append(versions, in.Version)
		}
	}
	return versions, nil
}

// handleHostingCapabilities: GET, any authenticated user — customers add
// domains too and deserve the same honest picture of what can work.
// handleHostingCapabilities: GET, kimliği doğrulanmış herkes — müşteriler de
// domain ekler ve neyin çalışabileceğinin aynı dürüst resmini hak eder.
func (p *Panel) handleHostingCapabilities(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	caps, err := p.hostingCaps(r.Context())
	if err != nil {
		writeAgentError(w, err, "hosting capabilities")
		return
	}
	// alpha.108 (D-038 §14): a customer is shown the PHP versions the
	// administrator offers; the administrator sees every installed one.
	if !callerIsAdministrator(r) {
		policy, err := p.loadOfferedVersions(r.Context())
		if err != nil {
			writeServerError(w, err)
			return
		}
		caps.PHPVersions = policy.filter("php", caps.PHPVersions)
	}
	if err := json.NewEncoder(w).Encode(caps); err != nil {
		writeServerError(w, err)
	}
}

// writeDNSServerRequired is the same refusal as before (DNS_SERVER_REQUIRED,
// 409, the DNS page as its fix), now saying why the DNS is not ready to take
// a new change: Reason is the dns_server_state and Vars name the engine and
// the other server of the pair (owner report 2026-10-31). What is allowed
// does not change. fallback is the sentence for no engine at all.
// Ayni ret; neden reason'da, motor ve eş sunucu vars'ta. Izin degismez.
func writeDNSServerRequired(w http.ResponseWriter, state, engine, peer, fallback string) {
	body := apiErrorBody{Error: fallback, Code: errCodeDNSServerRequired, Action: "/dns-infrastructure"}
	if message, ok := dnsServerStateMessages[state]; ok {
		body.Reason = state
		body.Error = message
	}
	vars := map[string]string{}
	if engine != "" {
		vars["engine"] = engine
	}
	if peer != "" {
		vars["peer"] = peer
	}
	if len(vars) > 0 {
		body.Vars = vars
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusConflict)
	_ = json.NewEncoder(w).Encode(body)
}

// The server's own sentence for each state that is not "no engine"; the
// screens have their own words for each reason (err.DNS_SERVER_REQUIRED.<state>).
var dnsServerStateMessages = map[string]string{
	dnsServerStatePairPending:    "the zones are served, but new changes wait until the backup DNS server confirms it holds the same zone list",
	dnsServerStateSecondary:      "this server is the backup DNS server; domains and records are changed on the main DNS server",
	dnsServerStateSwitching:      "a DNS engine switch is running; new changes wait until it ends",
	dnsServerStatePort53Conflict: "another program holds DNS port 53, so the DNS engine cannot be verified",
	dnsServerStateNotRunning:     "the active DNS engine is not running or not managed by CelikPanel",
	dnsServerStateEngineConflict: "another DNS engine is running beside the active one",
}
