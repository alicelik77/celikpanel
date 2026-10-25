package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Managed vendor repositories, panel side. A distro pins one major version of a
// database/runtime; the vendor's own repo (PGDG for PostgreSQL) carries every
// current major, so enabling it unlocks version choice at install time.
//
// The allowlist lives here: the panel resolves the repo from the catalog by
// service ID and passes only those declared values to the agent. The UI never
// sends a URL, so it can never point the agent at an arbitrary repository.
// Admin-only, and every enable/disable is audited — turning on a repo widens
// the trust boundary and should be traceable.
//
// Yönetilen vendor depoları, panel tarafı. Dağıtım bir veritabanı/çalışma
// zamanının tek major'unu sabitler; vendor'ın kendi deposu (PostgreSQL için
// PGDG) tüm güncel major'ları taşır, böylece açmak kurulumda sürüm seçimini açar.
//
// İzin listesi burada: panel depoyu katalogdan servis ID'siyle çözer ve agent'a
// yalnız bu tanımlı değerleri geçirir. UI asla URL göndermez; agent'ı asla keyfi
// bir depoya yönlendiremez. Yalnız admin ve her aç/kapat audit'lenir.

type repoInfoResp struct {
	Available  bool     `json:"available"`            // service has a managed repo at all
	Enabled    bool     `json:"enabled"`              // repo currently active on this host
	Repairable bool     `json:"repairable,omitempty"` // drift can be repaired by idempotent re-enable
	ID         string   `json:"id,omitempty"`         // repo id, e.g. "pgdg"
	Name       string   `json:"name,omitempty"`       // human name
	Detail     string   `json:"detail,omitempty"`     // one-line description
	Required   bool     `json:"required,omitempty"`   // without this repo the package does not exist here
	Packages   []string `json:"packages,omitempty"`   // available version packages (newest first)
	ErrorCode  string   `json:"error_code,omitempty"`
	// The repository card (alpha.107, D-038 §12): the confinement, the
	// moving default-version packages installed, the signing key, what apt
	// last downloaded, who enabled it and when (the audit log; the Agent's
	// own record when the log has no row), and apt's error lines that name
	// it. Absent from an older Agent: the card says it is not known.
	Confinement    *transport.RepoConfinement    `json:"confinement,omitempty"`
	MovingPackages []transport.RepoMovingPackage `json:"moving_packages,omitempty"`
	Key            *transport.RepoKeyInfo        `json:"key,omitempty"`
	Lists          *transport.RepoListsInfo      `json:"lists,omitempty"`
	EnabledAt      string                        `json:"enabled_at,omitempty"`
	EnabledBy      string                        `json:"enabled_by,omitempty"`
	AptErrors      []string                      `json:"apt_errors,omitempty"`
	AptErrorsAt    string                        `json:"apt_errors_at,omitempty"`
	// Lane / Origin (alpha.108): this repository is a vendor lane — it is
	// enabled only through the confirmed lane (repository, confinement and
	// automatic updates together); Origin is its Release origin.
	Lane   bool   `json:"lane,omitempty"`
	Origin string `json:"origin,omitempty"`
	// NoLongerUpdated / DisabledAt (alpha.108): with the repository
	// disabled, what is still installed from it; nothing updates it.
	NoLongerUpdated []string `json:"no_longer_updated,omitempty"`
	DisabledAt      string   `json:"disabled_at,omitempty"`
}

// Repository failures cross two trust boundaries: agent -> panel and panel ->
// browser. Only this allowlist may cross the second boundary. Raw diagnostics
// may contain commands, paths or package-manager output, so they stay in logs.
//
// Depo hatalari iki guven sinirini gecer: agent -> panel ve panel -> tarayici.
// Ikinci siniri yalnizca bu izin listesindeki sabit kodlar gecebilir. Ham tani
// komut, yol veya paket yoneticisi ciktisi icerebildigi icin gunlukte kalir.
const (
	errCodeRepoStatusUnavailable    = "REPO_STATUS_UNAVAILABLE"
	errCodeRepoPackagesUnavailable  = "REPO_PACKAGES_UNAVAILABLE"
	errCodeRepoAgentUnavailable     = "REPO_AGENT_UNAVAILABLE"
	errCodeRepoInvalidRequest       = "REPO_INVALID_REQUEST"
	errCodeRepoUnsupportedSystem    = "REPO_UNSUPPORTED_SYSTEM"
	errCodeRepoUnsupportedDistro    = "REPO_UNSUPPORTED_DISTRIBUTION"
	errCodeRepoKeyUntrusted         = "REPO_KEY_UNTRUSTED"
	errCodeRepoEnableFailed         = "REPO_ENABLE_FAILED"
	errCodeRepoDisableFailed        = "REPO_DISABLE_FAILED"
	errCodeRepoConfigurationInvalid = "REPO_CONFIGURATION_INVALID"
	errCodeRepoStatusFailed         = "REPO_STATUS_FAILED"
	errCodeRepoPackagesFailed       = "REPO_PACKAGES_FAILED"
)

func normalizeRepoErrorCode(code, fallback string) string {
	trimmed := strings.TrimSpace(code)
	switch trimmed {
	case errCodeRepoStatusUnavailable,
		errCodeRepoPackagesUnavailable,
		errCodeRepoAgentUnavailable,
		errCodeRepoInvalidRequest,
		errCodeRepoUnsupportedSystem,
		errCodeRepoUnsupportedDistro,
		errCodeRepoKeyUntrusted,
		errCodeRepoEnableFailed,
		errCodeRepoDisableFailed,
		errCodeRepoConfigurationInvalid,
		errCodeRepoStatusFailed,
		errCodeRepoPackagesFailed:
		return trimmed
	default:
		return fallback
	}
}

func safeRepoErrorMessage(code string) string {
	switch code {
	case errCodeRepoStatusUnavailable, errCodeRepoStatusFailed:
		return "repository status could not be verified"
	case errCodeRepoPackagesUnavailable, errCodeRepoPackagesFailed:
		return "repository package list could not be loaded"
	case errCodeRepoUnsupportedSystem, errCodeRepoUnsupportedDistro:
		return "this repository is not supported on this system"
	case errCodeRepoKeyUntrusted:
		return "repository signing key could not be verified"
	case errCodeRepoDisableFailed:
		return "repository could not be disabled"
	case errCodeRepoConfigurationInvalid:
		return "repository configuration needs repair"
	case errCodeRepoInvalidRequest:
		return "invalid repository request"
	case errCodeRepoAgentUnavailable:
		return "repository service is temporarily unavailable"
	default:
		return "repository could not be enabled"
	}
}

// handleRepo: GET ?service_id= reports the managed repo for a service and, when
// enabled, the version packages available; POST {service_id, action} enables or
// disables it. Admin-only.
// handleRepo: GET ?service_id= bir servisin yönetilen deposunu ve etkinse mevcut
// sürüm paketlerini bildirir; POST {service_id, action} açar ya da kapatır.
func (p *Panel) handleRepo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if c := currentCaller(r); c == nil || c.Role != roleAdmin {
		writeClientError(w, http.StatusForbidden, "admin only")
		return
	}

	switch r.Method {
	case http.MethodGet:
		svc := core.GetManagedServiceByID(r.URL.Query().Get("service_id"))
		if svc == nil || svc.Repo == nil {
			json.NewEncoder(w).Encode(repoInfoResp{Available: false})
			return
		}
		// Managed vendor repositories are apt-only today. On Arch the same
		// component may come from the native pacman repository, so presenting
		// an apt-only required-repo button would falsely block its install.
		// Yonetilen vendor depolari simdilik yalniz apt icindir. Arch'ta ayni
		// bilesen yerel pacman deposundan gelebilir; apt-only zorunlu-depo
		// dugmesi gostermek kurulumu yanlislikla engellerdi.
		if p.packageFamily() != "apt" {
			json.NewEncoder(w).Encode(repoInfoResp{Available: false})
			return
		}
		json.NewEncoder(w).Encode(p.repoInfoContext(r.Context(), svc.Repo))

	case http.MethodPost:
		release, busy := p.beginServiceMutation(w, r)
		if busy {
			return
		}
		defer release()
		var req struct {
			ServiceID string `json:"service_id"`
			Action    string `json:"action"` // "enable" | "disable" | "confine"
			// ExpectedSHA256: confine only, the preferences text confirmed.
			ExpectedSHA256 string `json:"expected_sha256"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeClientError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		svc := core.GetManagedServiceByID(req.ServiceID)
		if svc == nil || svc.Repo == nil {
			writeClientError(w, http.StatusBadRequest, "service has no managed repository")
			return
		}
		repo := svc.Repo

		var st RepoStatusResp
		method := ""
		switch req.Action {
		case "enable":
			// Only durable mutation identity plus the repository id travel: the
			// agent looks the URL and source line up in its OWN catalogue.
			// Yalnız kalıcı değişiklik kimliği ile depo id'si yolculuk eder: URL'i
			// ve kaynak satırını agent KENDİ kataloğunda arar.
			method = "Agent.EnableRepo"
			// alpha.108: a vendor lane is never enabled alone — only with
			// its confinement and its automatic updates, as the lane shown
			// and confirmed at /api/v1/repo/lane.
			if strings.TrimSpace(repo.ReleaseOrigin) != "" {
				writeCodedError(w, http.StatusConflict, "REPO_LANE_REQUIRED", "this repository is enabled only together with its confinement and automatic updates", "/services")
				return
			}
		case "disable":
			method = "Agent.DisableRepo"
		case "confine":
			// alpha.107: the administrator confines an enabled repository
			// with the text the card showed (its SHA-256).
			if len(strings.TrimSpace(req.ExpectedSHA256)) != 64 {
				writeClientError(w, http.StatusBadRequest, "the confirmed preferences text is missing")
				return
			}
			response, err := p.confineRepo(r.Context(), repo.ID, strings.TrimSpace(req.ExpectedSHA256))
			p.auditRepoConfinement(r, repo.ID, response, err)
			if err != nil {
				code := "REPO_CONFINE_FAILED"
				if response.Confinement != nil && response.Confinement.Reason == "plan_changed" {
					code = "REPO_CONFINE_PLAN_CHANGED"
				}
				w.WriteHeader(http.StatusConflict)
				json.NewEncoder(w).Encode(map[string]any{"error": err.Error(), "code": code})
				return
			}
			json.NewEncoder(w).Encode(p.repoInfoContext(r.Context(), repo))
			return
		default:
			writeClientError(w, http.StatusBadRequest, "action must be enable, disable or confine")
			return
		}
		err := p.withStandaloneAgentMutation(r.Context(), "repo_"+req.Action, repo.ID, "", func(callCtx context.Context, binding agentMutationBinding) error {
			request := &enableRepoReq{
				ServiceMutationBinding: binding,
				RepoID:                 repo.ID,
			}
			if err := p.callAgentContext(callCtx, method, request, &st); err != nil {
				return err
			}
			if st.Error != "" {
				return errors.New(st.Error)
			}
			return nil
		})
		if err != nil && st.Error == "" {
			log.Printf("[repo][%s][%s][transport] %v", repo.ID, req.Action, err)
			p.audit(r, "repo."+req.Action+".failed:"+repo.ID+" — "+errCodeRepoAgentUnavailable, "repo", 0)
			writeCodedError(w, http.StatusBadGateway, errCodeRepoAgentUnavailable, safeRepoErrorMessage(errCodeRepoAgentUnavailable), "/services")
			return
		}
		if st.Error != "" {
			fallback := errCodeRepoEnableFailed
			if req.Action == "disable" {
				fallback = errCodeRepoDisableFailed
			}
			code := normalizeRepoErrorCode(st.ErrorCode, fallback)
			log.Printf("[repo][%s][%s][agent][%s] %s", repo.ID, req.Action, code, st.Error)
			partial := st.PartialSuccess || st.MutationApplied
			// A rollback failure is not an ordinary refusal: disk state changed and
			// the audit/API contract must tell the operator that repair is required.
			// Rollback başarısızlığı sıradan bir ret değildir: disk durumu değişmiştir
			// ve audit/API sözleşmesi operatöre onarım gerektiğini söylemelidir.
			if partial {
				p.audit(r, "repo."+req.Action+".partial:"+repo.ID+" — "+code, "repo", 0)
				w.WriteHeader(http.StatusBadGateway)
				_ = json.NewEncoder(w).Encode(apiErrorBody{
					Error:           safeRepoErrorMessage(code),
					Code:            code,
					Action:          "/services",
					PartialSuccess:  true,
					MutationApplied: true,
				})
				return
			}
			p.audit(r, "repo."+req.Action+".failed:"+repo.ID+" — "+code, "repo", 0)
			writeCodedError(w, http.StatusConflict, code, safeRepoErrorMessage(code), "/services")
			return
		}
		details := map[string]any{"repository": repo.ID, "name": repo.Name}
		if st.Confinement != nil {
			details["confinement"] = st.Confinement.State
			details["kept_total"] = st.Confinement.KeptTotal
		}
		p.auditDetailed(r, "repo."+req.Action+":"+repo.ID, "repo", 0, details)
		json.NewEncoder(w).Encode(p.repoInfoContext(r.Context(), repo))

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// repoInfo asks the agent for the repo's current state and, when it is enabled,
// the versioned packages available right now.
// repoInfo, agent'tan deponun mevcut durumunu ve etkinse şu an mevcut sürümlü
// paketleri ister.
func (p *Panel) repoInfo(repo *core.ManagedRepo) repoInfoResp {
	return p.repoInfoContext(context.Background(), repo)
}

func (p *Panel) repoInfoContext(ctx context.Context, repo *core.ManagedRepo) repoInfoResp {
	info := repoInfoResp{Available: true, ID: repo.ID, Name: repo.Name, Detail: repo.Description, Required: repo.Required,
		Lane: strings.TrimSpace(repo.ReleaseOrigin) != "", Origin: repo.ReleaseOrigin}
	var st RepoStatusResp
	if err := p.callAgent("Agent.RepoStatus", &enableRepoReq{RepoID: repo.ID}, &st); err != nil {
		log.Printf("[repo][%s][status][transport] %v", repo.ID, err)
		info.ErrorCode = errCodeRepoStatusUnavailable
		return info
	}
	info.Enabled = st.Enabled
	info.Repairable = st.Repairable
	info.NoLongerUpdated, info.DisabledAt = st.NoLongerUpdated, st.DisabledAt
	if st.Enabled {
		info.Confinement, info.MovingPackages, info.Key, info.Lists = st.Confinement, st.MovingPackages, st.Key, st.Lists
		info.AptErrors, info.AptErrorsAt = cleanAptLines(st.AptErrors), st.AptErrorsAt
		info.EnabledBy, info.EnabledAt = p.repoEnabledBy(ctx, repo.ID)
		if info.EnabledAt == "" {
			info.EnabledAt = st.EnabledAt
		}
	}
	if st.Error != "" {
		fallback := errCodeRepoStatusFailed
		if st.Repairable {
			fallback = errCodeRepoConfigurationInvalid
		}
		info.ErrorCode = normalizeRepoErrorCode(st.ErrorCode, fallback)
		log.Printf("[repo][%s][status][agent][%s] %s", repo.ID, info.ErrorCode, st.Error)
	}
	// A repo without PackagePattern (for example Netdata) offers no version
	// menu, so never ask the agent to enumerate it with an empty search.
	// PackagePattern olmayan depo (örneğin Netdata) sürüm menüsü sunmaz; bu
	// yüzden agent'tan boş aramayla paket listelemesini asla isteme.
	if info.Enabled && strings.TrimSpace(repo.PackagePattern) != "" {
		var pkgs RepoPackagesResp
		if err := p.callAgent("Agent.RepoPackages", &repoPackagesReq{RepoID: repo.ID}, &pkgs); err != nil {
			log.Printf("[repo][%s][packages][transport] %v", repo.ID, err)
			info.ErrorCode = errCodeRepoPackagesUnavailable
			return info
		}
		info.Packages = pkgs.Packages
		if pkgs.Error != "" {
			info.ErrorCode = normalizeRepoErrorCode(pkgs.ErrorCode, errCodeRepoPackagesFailed)
			log.Printf("[repo][%s][packages][agent][%s] %s", repo.ID, info.ErrorCode, pkgs.Error)
		}
	}
	return info
}

// enableRepoReq carries an ID and nothing else. The key URL and source line
// deliberately do NOT travel over the RPC any more — the agent reads them from
// its own compiled catalogue, so a compromised panel cannot choose what apt
// trusts. Do not add fields here.
// enableRepoReq yalnız bir ID taşır. Anahtar URL'i ve kaynak satırı artık
// bilerek RPC üzerinden GİTMEZ — agent onları kendi derlenmiş kataloğundan
// okur; böylece ele geçirilmiş bir panel apt'ın neye güveneceğini seçemez.
// Buraya alan eklemeyin.
type enableRepoReq = transport.EnableRepoRequest
type RepoStatusResp = transport.RepoStatusResponse
type repoPackagesReq = transport.RepoPackagesRequest
type RepoPackagesResp = transport.RepoPackagesResponse
