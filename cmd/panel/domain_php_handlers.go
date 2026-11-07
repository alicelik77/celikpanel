package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/repositories"
	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// DomainPHPSettingsResponse represents PHP settings for a domain
type DomainPHPSettingsResponse struct {
	DomainID          int                 `json:"domain_id"`
	DomainName        string              `json:"domain_name"`
	PHPVersion        string              `json:"php_version"`
	AvailableVersions []string            `json:"available_versions"`
	PoolName          string              `json:"pool_name"`
	PoolConfig        *core.PHPPoolConfig `json:"pool_config,omitempty"`
}

// UpdateDomainPHPRequest represents a request to update domain PHP settings
type UpdateDomainPHPRequest struct {
	PHPVersion string `json:"php_version"`
	// ConfirmUnsupported (alpha.107, D-038 §5): the person confirmed a
	// version that no longer receives security fixes. Without it such a
	// change is refused with SUPPORT_CONFIRMATION_REQUIRED.
	ConfirmUnsupported bool `json:"confirm_unsupported,omitempty"`
}

// errCodeSiteHasNoPHP: the site's hosting type runs no PHP (static, Node.js,
// proxy, forwarding, DNS only), so it has no PHP version; nothing was
// changed. The reason is the hosting type; the action is the site's Hosting
// page, where its type is changed (D-024).
// Sitenin barındırma tipi PHP çalıştırmıyor; hiçbir şey değiştirilmedi.
const errCodeSiteHasNoPHP = "SITE_HAS_NO_PHP"

func writeSiteHasNoPHP(w http.ResponseWriter, domainName, projectType string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusConflict)
	_ = json.NewEncoder(w).Encode(apiErrorBody{
		Code:   errCodeSiteHasNoPHP,
		Reason: projectType,
		Error: domainName + " is a " + projectType + " site, so it has no PHP version and nothing was changed. " +
			"To run PHP on it, change its hosting type to PHP on the site's Hosting page; then choose the PHP version here.",
		Action: "/domains/" + url.PathEscape(domainName) + "?tab=hosting",
		Vars:   map[string]string{"domain": domainName},
	})
}

// errCodeSupportConfirmationRequired: the chosen version no longer receives
// security fixes and the change did not carry the person's confirmation.
const errCodeSupportConfirmationRequired = "SUPPORT_CONFIRMATION_REQUIRED"

// handleDomainPHPSettings handles GET/POST for domain PHP settings
func (p *Panel) handleDomainPHPSettings(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Extract domain ID from URL
	domainID, err := extractDomainID(r.URL.Path, "/api/v1/domains/", "/php")
	if err != nil {
		http.Error(w, "Invalid domain ID", http.StatusBadRequest)
		return
	}

	// Get domain
	domainRepo := repositories.NewPostgresDomainRepository(p.db.GetDB())
	domain, err := domainRepo.GetByID(context.Background(), domainID)
	if err != nil {
		http.Error(w, "Domain not found", http.StatusNotFound)
		return
	}

	// Get site info
	var siteID int
	var phpVersion, projectType string
	var phpFPMSocket *string
	query := `SELECT id, COALESCE(php_version, ''), php_fpm_socket, COALESCE(project_type, 'php') FROM sites WHERE domain_id = ? LIMIT 1`
	err = p.db.GetDB().QueryRowContext(context.Background(), query, domainID).Scan(&siteID, &phpVersion, &phpFPMSocket, &projectType)
	if err != nil {
		http.Error(w, "Site not found for domain", http.StatusNotFound)
		return
	}

	// Determine pool name from socket or site ID
	poolName := fmt.Sprintf("site%d", siteID)
	if phpFPMSocket != nil && *phpFPMSocket != "" {
		// Extract pool name from socket path
		// e.g., /var/run/php/php8.3-fpm-site12.sock -> site12
		parts := strings.Split(*phpFPMSocket, "-")
		if len(parts) >= 2 {
			poolName = strings.TrimSuffix(parts[len(parts)-1], ".sock")
		}
	}

	if r.Method == "GET" {
		availableVersions := domainPHPAvailableVersions(phpVersion, nil)
		discoveredVersions, discoveryErr := p.phpVersionsFromAgent(r.Context())
		if discoveryErr != nil {
			// The page already had a tolerant read contract for a missing pool
			// configuration. Keep that contract when discovery is temporarily
			// unavailable: the site's stored current version is known and is the
			// only honest fallback, while the internal failure remains visible in
			// the server log. Never invent another version.
			log.Printf("discover PHP versions for domain %d: %v", domainID, discoveryErr)
		} else {
			availableVersions = domainPHPAvailableVersions(phpVersion, discoveredVersions)
		}
		// alpha.108 (D-038 §14): a customer is offered what the administrator
		// offers; the site's own version stays listed (it keeps it).
		if !callerIsAdministrator(r) {
			// alpha.117 (D-042): and what the subscription's rights open.
			subID, subErr := p.domainSubscriptionID(r.Context(), domainID)
			policy, _, policyErr := p.subscriptionVersionPolicy(r.Context(), subID)
			if subErr != nil {
				policyErr = subErr
			}
			if policyErr == nil {
				kept := []string{}
				for _, version := range availableVersions {
					if version == phpVersion || policy.offered("php", version) {
						kept = append(kept, version)
					}
				}
				availableVersions = kept
			} else {
				log.Printf("versions offered to customers (domain %d): %v", domainID, policyErr)
				availableVersions = domainPHPAvailableVersions(phpVersion, nil)
			}
		}

		// Get pool config from agent
		var poolConfig core.PHPPoolConfig
		req := transport.GetPHPPoolConfigRequest{
			Version:  phpVersion,
			PoolName: poolName,
		}

		err = p.callAgent("Agent.GetPHPPoolConfig", req, &poolConfig)
		if err != nil {
			// A read must not write. This branch used to build a default pool
			// and POST it to the agent, so merely OPENING the PHP settings page
			// rewrote a root-loaded file under /etc/php — and it wrote
			// `user = <domain name>`, which is not a system user at all (the
			// real pool is created with the site's system user). php-fpm's
			// master refuses to start a pool whose user cannot be resolved, so
			// a GET could take down every site on that PHP version at the next
			// restart. Report the pool as absent instead; creating one is the
			// site-creation path's job, not a page load's.
			//
			// Bir okuma yazmamalıdır. Bu dal eskiden varsayılan bir havuz kurup
			// agent'a POST ediyordu; yani PHP ayarları sayfasını AÇMAK bile
			// /etc/php altında root'un yüklediği bir dosyayı yeniden yazıyordu —
			// üstelik `user = <alan adı>` yazıyordu ki bu hiçbir sistem
			// kullanıcısı değildir (gerçek havuz sitenin sistem kullanıcısıyla
			// oluşturulur). php-fpm master'ı, kullanıcısı çözülemeyen bir havuzu
			// başlatmayı reddeder; yani bir GET, bir sonraki yeniden başlatmada
			// o PHP sürümündeki tüm siteleri düşürebilirdi. Bunun yerine havuz
			// yok diye bildirilir; havuz oluşturmak sayfa yüklemenin değil, site
			// oluşturma yolunun işidir.
			json.NewEncoder(w).Encode(DomainPHPSettingsResponse{
				DomainID:          domainID,
				DomainName:        domain.Name,
				PHPVersion:        phpVersion,
				AvailableVersions: availableVersions,
				PoolName:          poolName,
			})
			return
		}

		json.NewEncoder(w).Encode(DomainPHPSettingsResponse{
			DomainID:          domainID,
			DomainName:        domain.Name,
			PHPVersion:        phpVersion,
			AvailableVersions: availableVersions,
			PoolName:          poolName,
			PoolConfig:        &poolConfig,
		})
		return
	}

	if r.Method == "POST" {
		var req UpdateDomainPHPRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeClientError(w, http.StatusBadRequest, "invalid request")
			return
		}
		// Only a PHP site has a PHP version to change. A static site (the
		// setup-created main site, Add Domain's default, D-013) has no pool
		// and no version: the switch below failed as 500 `invalid PHP
		// version ""` (a124 lab). The site's type is changed on its Hosting
		// type page, which writes the pool (hosting_transition.go).
		if projectType != "php" {
			writeSiteHasNoPHP(w, domain.Name, projectType)
			return
		}

		// Validate the closed version shape before asking the agent or building
		// a socket path. Then require the exact managed version reported by the
		// agent; even a no-op switch to the stored version fails closed when that
		// version is no longer discoverable.
		// alpha.127: the progress window follows these steps
		// (operation_steps.go); a save of the same version only records it.
		if req.PHPVersion != phpVersion {
			reportOperationPlan(r.Context(), "check", "pool", "save", "web_server")
		} else {
			reportOperationPlan(r.Context(), "check", "save")
		}
		reportOperationStep(r.Context(), "check")
		if err := services.ValidatePHPVersion(req.PHPVersion); err != nil {
			writeClientError(w, http.StatusBadRequest, "invalid PHP version")
			return
		}
		// alpha.108 (D-038 §14): a customer chooses among the versions the
		// administrator offers; the version the site already runs is kept.
		if p.refuseVersionNotOffered(w, r, domainID, "php", phpVersion, req.PHPVersion) {
			return
		}
		discoveredVersions, err := p.phpVersionsFromAgent(r.Context())
		if err != nil {
			writeAgentError(w, err, "PHP version discovery")
			return
		}
		if !containsDomainPHPVersion(
			domainPHPAvailableVersions("", discoveredVersions),
			req.PHPVersion,
		) {
			writeClientError(w, http.StatusBadRequest, "PHP version is not available")
			return
		}

		// alpha.107 (D-038 §5): a version that no longer receives security
		// fixes is chosen only with the person's confirmation; nothing is
		// changed without it.
		support, refuse := phpChangeSupport(phpVersion, req.PHPVersion, req.ConfirmUnsupported)
		if refuse {
			writeCodedError(w, http.StatusConflict, errCodeSupportConfirmationRequired,
				"PHP "+req.PHPVersion+" no longer receives security fixes (since "+support.Until+"); confirm the choice to use it. Nothing was changed.", "")
			return
		}

		// Only migrate if version is actually changing
		if req.PHPVersion != phpVersion {
			// alpha.128 (alpha.127 lab): with nginx stopped, the vhost
			// cannot be reloaded to the new socket; the change used to
			// answer INTERNAL after the pool had moved, and the site gave
			// 502 until the next vhost write. Nothing is changed while the
			// web server is known to be stopped; an unknown reading goes
			// on, and any failure below puts the site back (undo below).
			if p.webServerKnownStopped(r.Context()) {
				next, action := " The server administrator must start it; then choose the version again.", ""
				if callerIsAdministrator(r) {
					next, action = " Start it on the Services page, then choose the version again.", "/components"
				}
				writeCodedError(w, http.StatusConflict, errCodeWebServerStopped,
					"Nothing was changed: the web server (nginx) is stopped, so "+domain.Name+" cannot be moved to PHP "+req.PHPVersion+" safely."+next, action)
				return
			}
			// Move the pool to the new version under D-031 (second step,
			// 2026-10-11): a pool file the owner changed is kept, nothing is
			// switched, and the answer is SITE_CONFIG_OWNER_EDITED (reason
			// php_pool); the owner chooses on the Configuration file page.
			// Havuz, sahibin değiştirdiği dosya korunarak taşınır.
			site, siteErr := p.loadSiteFileSite(r.Context(), domainID)
			if siteErr != nil {
				writeServerError(w, siteErr)
				return
			}
			site.PHPVersion = phpVersion
			reportOperationStep(r.Context(), "pool")
			switchReq := p.sitePoolRequest(r.Context(), site, transport.SitePoolOperationSwitch)
			switchReq.NewPHPVersion = req.PHPVersion
			if _, err := p.applySitePool(r.Context(), site, switchReq); err != nil {
				writeServerErrorFor(w, r, err)
				return
			}

			// The socket is the one the Agent just wrote the pool with: the
			// same function on the same host (services.PHPFPMSocketPath). It
			// used to be built here as Debian's /var/run/php/..., which on a
			// host with one unversioned PHP-FPM is not where the pool listens.
			// Soket, Agent'ın havuzu az önce yazdığı sokettir: aynı sunucuda
			// aynı fonksiyon.
			reportOperationStep(r.Context(), "save")
			newSocket := services.PHPFPMSocketPath(req.PHPVersion, poolName)
			updateQuery := `UPDATE sites SET php_version = ?, php_fpm_socket = ?, updated_at = datetime('now') WHERE domain_id = ?`
			_, err = p.db.GetDB().ExecContext(context.Background(), updateQuery, req.PHPVersion, newSocket, domainID)

			// The vhost must follow the socket. Caught live (23 Jul): the
			// pool moved, the DB updated, and nginx kept proxying to the
			// DELETED old socket — the site answered 502 while everything
			// reported success. A failed regen is a failed request: the
			// operator must know the site is dark, not read "updated".
			// Vhost soketi izlemek zorundadır. Canlıda yakalandı (23 Tem):
			// havuz taşındı, DB güncellendi ve nginx SİLİNEN eski sokete
			// vekillik etmeye devam etti — her şey başarı bildirirken site
			// 502 veriyordu. Başarısız regen başarısız istektir: operatör
			// "güncellendi" değil, sitenin karanlık olduğunu okumalı.
			if err == nil {
				// D-043: a site that leaves a version under a security
				// emergency leaves its maintenance list; the render below
				// shows the site again.
				p.noteEmergencySiteMoved(r.Context(), domainID)
				reportOperationStep(r.Context(), "web_server")
				if verr := p.applySiteVhost(r.Context(), domainID); verr != nil {
					log.Printf("vhost regen after php switch (domain %d): %v", domainID, verr)
					// alpha.128: the vhost did not follow the socket; move
					// the pool and the record back so the site keeps
					// answering on its previous version.
					if p.undoPHPVersionSwitch(r.Context(), site, poolName, phpVersion, req.PHPVersion, phpFPMSocket) {
						writeCodedErrorDetails(w, http.StatusBadGateway, errCodePHPVersionUndone,
							"Nothing was changed: the web server did not take the new configuration for "+domain.Name+
								", so the site was put back on PHP "+phpVersion+" and keeps working. Check the web server on the Services page (server administrator), then try again.",
							"", []string{boundedAgentDiagnostic(verr.Error())})
						return
					}
					writeServerErrorFor(w, r, verr)
					return
				}
			}
		} else {
			// Just update version in DB (no migration needed)
			reportOperationStep(r.Context(), "save")
			updateQuery := `UPDATE sites SET php_version = ?, updated_at = datetime('now') WHERE domain_id = ?`
			_, err = p.db.GetDB().ExecContext(context.Background(), updateQuery, req.PHPVersion, domainID)
		}

		if err != nil {
			writeServerError(w, err)
			return
		}
		// alpha.107 (D-038 §16): who changed the site's PHP, from and to, and
		// whether an unsupported version was confirmed.
		if req.PHPVersion != phpVersion {
			p.auditDetailed(r, "site.php_version:"+domain.Name+":"+phpVersion+"->"+req.PHPVersion, "domain", domainID, map[string]any{
				"site": domain.Name, "component": "php-fpm", "from": phpVersion, "to": req.PHPVersion,
				"support": support.Status, "support_until": support.Until, "confirmed_unsupported": req.ConfirmUnsupported && support.Status == "unsupported",
			})
		}

		json.NewEncoder(w).Encode(map[string]string{
			"status":  "success",
			"message": "PHP version updated",
		})
	}
}

// domainPHPAvailableVersions closes the picker contract over validated
// major.minor versions, removes duplicate agent rows and sorts newest first.
// The stored current version is included only when it has the same safe shape;
// it is known domain state, not a guessed installed version.
func domainPHPAvailableVersions(current string, discovered []string) []string {
	seen := make(map[string]struct{}, len(discovered)+1)
	versions := make([]string, 0, len(discovered)+1)
	add := func(version string) {
		if services.ValidatePHPVersion(version) != nil {
			return
		}
		if _, exists := seen[version]; exists {
			return
		}
		seen[version] = struct{}{}
		versions = append(versions, version)
	}
	for _, version := range discovered {
		add(version)
	}
	add(current)
	sort.Slice(versions, func(i, j int) bool {
		leftMajor, leftMinor := domainPHPVersionParts(versions[i])
		rightMajor, rightMinor := domainPHPVersionParts(versions[j])
		if leftMajor != rightMajor {
			return leftMajor > rightMajor
		}
		if leftMinor != rightMinor {
			return leftMinor > rightMinor
		}
		return versions[i] < versions[j]
	})
	return versions
}

func domainPHPVersionParts(version string) (int, int) {
	major, minor, _ := strings.Cut(version, ".")
	majorNumber, _ := strconv.Atoi(major)
	minorNumber, _ := strconv.Atoi(minor)
	return majorNumber, minorNumber
}

func containsDomainPHPVersion(versions []string, target string) bool {
	for _, version := range versions {
		if version == target {
			return true
		}
	}
	return false
}

// handleDomainPHPPool handles GET/POST for domain-specific pool configuration
func (p *Panel) handleDomainPHPPool(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Extract domain ID from URL
	domainID, err := extractDomainID(r.URL.Path, "/api/v1/domains/", "/php/pool")
	if err != nil {
		http.Error(w, "Invalid domain ID", http.StatusBadRequest)
		return
	}

	// Get site info
	var phpVersion, projectType, domainName string
	var siteID int
	query := `SELECT s.id, COALESCE(s.php_version, ''), COALESCE(s.project_type, 'php'), d.name
		FROM sites s JOIN domains d ON d.id = s.domain_id WHERE s.domain_id = ? LIMIT 1`
	err = p.db.GetDB().QueryRowContext(context.Background(), query, domainID).Scan(&siteID, &phpVersion, &projectType, &domainName)
	if err != nil {
		http.Error(w, "Site not found for domain", http.StatusNotFound)
		return
	}
	// Only a PHP site has a pool. A static, Node.js, proxy, forwarding or
	// DNS-only site has none: reading it asked the Agent for the pool of an
	// empty PHP version and answered 500. The same typed refusal as the PHP
	// version change (SITE_HAS_NO_PHP), for reading and saving; nothing is
	// asked of the Agent and nothing is changed.
	// Yalnız PHP sitesinin havuzu vardır; diğerleri aynı 409 ile reddedilir.
	if projectType != "php" {
		writeSiteHasNoPHP(w, domainName, projectType)
		return
	}

	poolName := fmt.Sprintf("site%d", siteID)

	if r.Method == "GET" {
		var poolConfig core.PHPPoolConfig
		req := struct {
			Version  string `json:"version"`
			PoolName string `json:"pool_name"`
		}{
			Version:  phpVersion,
			PoolName: poolName,
		}

		err = p.callAgent("Agent.GetPHPPoolConfig", req, &poolConfig)
		if err != nil {
			writeServerError(w, err)
			return
		}

		json.NewEncoder(w).Encode(poolConfig)
		return
	}

	if r.Method == "POST" {
		var req core.PHPPoolConfigRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeClientError(w, http.StatusBadRequest, "invalid request")
			return
		}

		// The pool's identity is not the caller's to set, and saying so out loud
		// beats ignoring it. The agent already refuses to take identity from a
		// request (that is the layer which cannot be bypassed), but silently
		// dropping these fields would answer "success" to someone who asked for
		// something we did not do — the honesty rule forbids that, and it is
		// also how an attempt goes unnoticed. Refuse explicitly instead.
		//
		// Havuzun kimliğini belirlemek çağıranın işi değildir ve bunu açıkça
		// söylemek, yok saymaktan iyidir. Agent zaten kimliği istekten almayı
		// reddediyor (atlatılamayan katman odur), ama bu alanları sessizce
		// düşürmek, istediğini yapmadığımız birine "başarılı" demek olurdu —
		// dürüstlük kuralı bunu yasaklar ve bir deneme de böyle fark edilmez
		// kalır. Onun yerine açıkça reddet.
		if bad := callerSetPoolIdentity(&req.PoolConfig); bad != "" {
			writeCodedError(w, http.StatusBadRequest, errCodePoolIdentityFixed,
				"a pool's "+bad+" is set by the panel and cannot be changed — only the process-manager settings are adjustable",
				"")
			return
		}

		// The site's pool, saved under D-031 (second step, 2026-10-11): a pool
		// file the owner changed is kept, its directives are not dropped, and
		// the answer is SITE_CONFIG_OWNER_EDITED (reason php_pool).
		// Sahibin değiştirdiği havuz dosyası korunur; yönergeleri düşürülmez.
		site, siteErr := p.loadSiteFileSite(r.Context(), domainID)
		if siteErr != nil {
			writeServerError(w, siteErr)
			return
		}
		site.PHPVersion = phpVersion
		saveReq := p.sitePoolRequest(r.Context(), site, transport.SitePoolOperationSave)
		saveReq.Values = &transport.SitePoolValues{
			PM: req.PoolConfig.PM, PMMaxChildren: req.PoolConfig.PMMaxChildren,
			PMStartServers: req.PoolConfig.PMStartServers, PMMinSpareServers: req.PoolConfig.PMMinSpareServers,
			PMMaxSpareServers: req.PoolConfig.PMMaxSpareServers, PMMaxRequests: req.PoolConfig.PMMaxRequests,
		}
		if req.PoolConfig.PM != "" && req.PoolConfig.PM != "dynamic" && req.PoolConfig.PM != "ondemand" && req.PoolConfig.PM != "static" {
			writeClientError(w, http.StatusBadRequest, "pm must be dynamic, ondemand or static")
			return
		}
		// alpha.128 (docs/SITE-LIMITS.md): the process count stays within the
		// site's limit (a customer could ask 200 processes before), and the
		// process manager's details are the administrator's: for anyone else
		// only the count is taken, the rest stays as the pool has it.
		if !callerIsAdministrator(r) {
			var current core.PHPPoolConfig
			if err := p.callAgent("Agent.GetPHPPoolConfig", transport.GetPHPPoolConfigRequest{Version: phpVersion, PoolName: poolName}, &current); err != nil {
				writeServerError(w, err)
				return
			}
			count := saveReq.Values.PMMaxChildren
			saveReq.Values = &transport.SitePoolValues{
				PM: current.PM, PMMaxChildren: count, PMStartServers: current.PMStartServers,
				PMMinSpareServers: current.PMMinSpareServers, PMMaxSpareServers: current.PMMaxSpareServers,
				PMMaxRequests: current.PMMaxRequests,
			}
		}
		if limits, _, err := p.domainSettingLimits(r.Context(), domainID); err == nil {
			limit := limits[settingPHPProcesses]
			if saveReq.PHPLimits.Known && limit.Ceiling != nil && !limit.allows(saveReq.Values.PMMaxChildren) {
				writeSettingAboveLimit(w, []settingRefusal{{Key: settingPHPProcesses, Value: saveReq.Values.PMMaxChildren,
					Limit: *limit.Ceiling, SetBy: limit.SetBy, SetByName: limit.SetByName}})
				return
			}
		} else {
			writeServerError(w, err)
			return
		}
		if _, err := p.applySitePool(r.Context(), site, saveReq); err != nil {
			writeServerErrorFor(w, r, err)
			return
		}

		json.NewEncoder(w).Encode(map[string]string{
			"status":  "success",
			"message": "Pool configuration updated",
		})
	}
}

// extractDomainID extracts domain ID from URL path
// e.g., /api/v1/domains/123/php -> 123
func extractDomainID(path, prefix, suffix string) (int, error) {
	// Remove prefix and suffix
	path = strings.TrimPrefix(path, prefix)
	path = strings.TrimSuffix(path, suffix)

	// Extract ID
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return 0, fmt.Errorf("invalid path")
	}

	return strconv.Atoi(parts[0])
}

// callerSetPoolIdentity names the first identity field a caller tried to set,
// or "" when the request only carries tunables. These fields decide WHO the
// pool runs as and WHICH socket it answers on — the two facts that make a pool
// a security boundary between tenants rather than a performance knob.
// callerSetPoolIdentity, çağıranın ayarlamaya çalıştığı ilk kimlik alanını
// adlandırır; istek yalnız ayar taşıyorsa "" döner. Bu alanlar havuzun KİM
// olarak koşacağına ve HANGİ sokete cevap vereceğine karar verir — bir havuzu
// performans düğmesi değil, kiracılar arası bir güvenlik sınırı yapan iki gerçek.
func callerSetPoolIdentity(c *core.PHPPoolConfig) string {
	switch {
	case c.User != "":
		return "user"
	case c.Group != "":
		return "group"
	case c.Listen != "":
		return "listen socket"
	case c.ListenOwner != "":
		return "listen.owner"
	case c.ListenGroup != "":
		return "listen.group"
	case c.ListenMode != "":
		return "listen.mode"
	}
	return ""
}

// errCodeWebServerStopped: a change that must reload nginx was refused before
// anything changed, because nginx is stopped. Action: the Services page.
// errCodePHPVersionUndone: the vhost did not take the new socket; the pool
// and the record were moved back, so nothing changed.
const (
	errCodeWebServerStopped = "WEB_SERVER_STOPPED"
	errCodePHPVersionUndone = "PHP_VERSION_UNDONE"
)

// webServerKnownStopped reads nginx's unit state (read only). Only a reading
// that says inactive or failed is "stopped"; an unknown reading is not.
func (p *Panel) webServerKnownStopped(ctx context.Context) bool {
	var resp transport.ServiceHealthResponse
	if err := p.callAgentContext(ctx, "Agent.ServiceHealthV1", &transport.ServiceHealthRequest{Units: []string{"nginx"}}, &resp); err != nil {
		return false
	}
	if !resp.Batched || len(resp.Units) != 1 || resp.Units[0].LoadState != "loaded" {
		return false
	}
	switch resp.Units[0].ActiveState {
	case "inactive", "failed":
		return true
	}
	return false
}

// undoPHPVersionSwitch moves the pool back to the previous version and puts
// the site's record back. It reports whether both happened.
func (p *Panel) undoPHPVersionSwitch(ctx context.Context, site siteFileSite, poolName, previous, current string, previousSocket *string) bool {
	site.PHPVersion = current
	back := p.sitePoolRequest(ctx, site, transport.SitePoolOperationSwitch)
	back.NewPHPVersion = previous
	if _, err := p.applySitePool(ctx, site, back); err != nil {
		log.Printf("undo PHP switch of %s (%s -> %s): pool: %v", site.Domain, current, previous, err)
		return false
	}
	socket := services.PHPFPMSocketPath(previous, poolName)
	if previousSocket != nil && *previousSocket != "" {
		socket = *previousSocket
	}
	if _, err := p.db.GetDB().ExecContext(ctx, `UPDATE sites SET php_version = ?, php_fpm_socket = ?, updated_at = datetime('now') WHERE domain_id = ?`,
		previous, socket, site.DomainID); err != nil {
		log.Printf("undo PHP switch of %s: record: %v", site.Domain, err)
		return false
	}
	return true
}
