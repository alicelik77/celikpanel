// Package serviceapps is the catalog and the native rendering of the AI and
// automation applications a site can run as a long-running service
// (alpha.118, docs/AI-APPS.md). It is shared by the Panel (catalog, offering
// gate, risk summary) and the Agent (unit and environment rendering, install
// commands, log redaction). Nothing here touches the system.
//
// Every application is pinned to one exact version whose upstream artifact
// hash is written below; an update is an explicit "update to version X" with a
// new pin in a new CelikPanel release. Nothing updates itself.
//
// serviceapps paketi, bir sitenin uzun süre çalışan hizmet olarak
// çalıştırabileceği yapay zekâ ve otomasyon uygulamalarının kataloğu ve yerel
// yapılandırma çıktısıdır. Her uygulama tek bir tam sürüme, kaynağının özetiyle
// sabitlenir; kendi kendine güncellenen hiçbir şey yoktur.
package serviceapps

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Runtime kinds.
const (
	RuntimeNode   = "node"
	RuntimePython = "python"
)

// Artifact kinds: how the pinned hash is written.
const (
	// HashNPMIntegrity is npm's dist.integrity ("sha512-<base64>").
	HashNPMIntegrity = "npm-integrity"
	// HashSHA256 is PyPI's digests.sha256 (hex).
	HashSHA256 = "sha256"
)

// Resources are the administrator's default limits of one install.
type Resources struct {
	MemoryMB   int `json:"memory_mb"`
	CPUPercent int `json:"cpu_percent"`
	TasksMax   int `json:"tasks_max"`
	// DiskMB is the expected size of one install (application plus its
	// dependencies), shown before offering and installing; it is counted in
	// the subscription's disk usage because it lives in the site's folder.
	DiskMB int `json:"disk_mb"`
	// InstallMemoryMB bounds the install step (npm / pip).
	InstallMemoryMB int `json:"install_memory_mb"`
}

// Risk is the summary the administrator reads before offering an app and the
// customer reads before installing it. Every entry is a code; the web
// interface words it in English and Turkish (svcapps.risk.<code>).
type Risk struct {
	// CanDo: what the application can do once running.
	CanDo []string `json:"can_do"`
	// Disabled: what CelikPanel turns off with the application's own
	// supported configuration.
	Disabled []string `json:"disabled"`
	// Network: the outside connections it needs.
	Network []string `json:"network"`
	// Remains: risks that stay even inside the sandbox.
	Remains []string `json:"remains"`
}

// Secret is one value the customer may enter (an LLM provider key). It is
// written only to the application's own environment file (0600, the site's
// user) and never stored by the Panel, logged or shown again.
type Secret struct {
	Key string `json:"key"`
	// Label is a short, untranslated product name ("OpenAI API key").
	Label string `json:"label"`
}

// App is one catalog entry.
type App struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Vendor   string `json:"vendor"`
	Homepage string `json:"homepage"`
	Source   string `json:"source"`
	// Licence is the licence's name; LicenceNote a code for the web
	// interface (svcapps.licence.<code>) when the licence restricts use.
	Licence     string `json:"licence"`
	LicenceNote string `json:"licence_note,omitempty"`

	Runtime string `json:"runtime"`
	// Package is the registry name (npm or PyPI).
	Package string `json:"package"`
	Version string `json:"version"`
	// ArtifactURL is the exact upstream file; ArtifactHash its pinned hash in
	// HashKind's notation; ArtifactFile the file name it is saved as.
	ArtifactURL  string `json:"artifact_url"`
	ArtifactFile string `json:"artifact_file"`
	ArtifactHash string `json:"artifact_hash"`
	HashKind     string `json:"hash_kind"`

	// NodeMajors are the Node.js major versions the app supports (Node apps).
	NodeMajors []int `json:"node_majors,omitempty"`
	// NPMPlatformDepsRequired (Node apps): the app's own package lists
	// prebuilt binaries for other platforms as ordinary (not optional)
	// dependencies, which npm refuses on any other platform (EBADPLATFORM).
	// npm's only switch for that is --force; the install then also stores
	// those unused binaries. Every package is still checked against the
	// registry's integrity value and signature. alpha.118 lab round 2:
	// n8n-nodes-base 2.46.x requires @napi-rs/{canvas,image}-linux-*-musl.
	NPMPlatformDepsRequired bool `json:"-"`
	// PythonMinors are the Python 3 minor versions it supports (Python apps).
	PythonMinors []int `json:"python_minors,omitempty"`

	// CustomerOfferable: the administrator may offer it to resellers and
	// customers. Deferred: why it is not installable at all in this release
	// (a code, svcapps.deferred.<code>); a deferred app cannot be offered.
	CustomerOfferable bool   `json:"customer_offerable"`
	Deferred          string `json:"deferred,omitempty"`
	// ResellerClosed (alpha.118, the owner's decision of 3 Nov 2026): the
	// app's licence limits offering it to others as a hosted service, so it
	// is never available through a reseller: not in a reseller's rights or
	// plans, and not installable in a reseller's own subscription or one of
	// its customers'. The administrator may still offer it to the
	// administrator's own customers. A code (svcapps.licence.<code>); empty
	// for apps without the restriction. Stop, logs and removal stay open.
	ResellerClosed string `json:"reseller_closed,omitempty"`

	Defaults Resources `json:"defaults"`
	Risk     Risk      `json:"risk"`
	Secrets  []Secret  `json:"secrets"`
	// OwnerLogin: the install form asks for the app's first administrator
	// (e-mail and password), so no visitor can claim the app first.
	OwnerLogin bool `json:"owner_login"`
	// AuxPort: the app needs a second loopback port (n8n's task broker).
	AuxPort bool `json:"aux_port"`
}

// OfferingID is the D-042 software offering of an app.
func OfferingID(appID string) string { return "app:" + appID }

// catalog is the closed list of this release. Versions, hashes and URLs were
// read from the official registries on 2026-11-03 (docs/AI-APPS.md).
var catalog = []App{
	{
		ID: "n8n", Name: "n8n", Vendor: "n8n GmbH",
		Homepage: "https://n8n.io", Source: "https://github.com/n8n-io/n8n",
		Licence: "Sustainable Use License 1.0", LicenceNote: "sustainable_use",
		Runtime: RuntimeNode, Package: "n8n", Version: "2.46.4",
		ArtifactURL:  "https://registry.npmjs.org/n8n/-/n8n-2.46.4.tgz",
		ArtifactFile: "n8n-2.46.4.tgz",
		ArtifactHash: "sha512-Ek/AlWCxbNfULfr5Z4kzS+SwcHfVTpcVIVmughdfqM905EGw0rIGjVLHbBTvgnYsV15Ajwih8spQndc5+Xr1Wg==",
		HashKind:     HashNPMIntegrity,
		NodeMajors:   []int{24},
		// n8n-nodes-base 2.46.4 depends (not optionally) on the Alpine
		// binaries @napi-rs/canvas-linux-{x64,arm64}-musl and
		// @napi-rs/image-linux-{x64,arm64}-musl (registry, 3 Nov 2026).
		NPMPlatformDepsRequired: true,
		CustomerOfferable:       true,
		ResellerClosed:          "sustainable_use",
		Defaults:                Resources{MemoryMB: 1536, CPUPercent: 100, TasksMax: 512, DiskMB: 1600, InstallMemoryMB: 3072},
		Risk: Risk{
			CanDo:    []string{"workflows_outbound", "stores_credentials", "webhooks_public", "code_node_sandboxed"},
			Disabled: []string{"execute_command", "local_file_trigger", "community_packages", "python_code", "env_access", "file_access_outside", "git_hooks", "telemetry"},
			Network:  []string{"npm_registry_install", "llm_providers", "any_outbound"},
			Remains:  []string{"prompt_injection", "loopback_reachable", "outbound_abuse"},
		},
		Secrets:    nil, // n8n keeps credentials in its own encrypted store
		OwnerLogin: true,
		AuxPort:    true,
	},
	{
		ID: "open-webui", Name: "Open WebUI", Vendor: "Open WebUI (Timothy J. Baek)",
		Homepage: "https://openwebui.com", Source: "https://github.com/open-webui/open-webui",
		Licence: "Open WebUI License (BSD-3-Clause with a branding clause)", LicenceNote: "branding",
		Runtime: RuntimePython, Package: "open-webui", Version: "0.12.5",
		ArtifactURL:       "https://files.pythonhosted.org/packages/9d/a4/1d7f9a7ee90325ef92c7951caf95cf2105222380fcbe0891ea256841e959/open_webui-0.12.5-py3-none-any.whl",
		ArtifactFile:      "open_webui-0.12.5-py3-none-any.whl",
		ArtifactHash:      "74fafb4efeee7c894303ca5b87fa7525fbf63b39c0b9962496fb82939119d3fd",
		HashKind:          HashSHA256,
		PythonMinors:      []int{11, 12},
		CustomerOfferable: true,
		Defaults:          Resources{MemoryMB: 3072, CPUPercent: 100, TasksMax: 512, DiskMB: 7000, InstallMemoryMB: 4096},
		Risk: Risk{
			CanDo:    []string{"chat_llm", "stores_documents", "web_fetch"},
			Disabled: []string{"code_execution", "code_interpreter", "functions_safe_mode", "signup", "community_sharing", "ollama", "update_check", "telemetry"},
			Network:  []string{"pypi_install", "model_download", "llm_providers"},
			Remains:  []string{"prompt_injection", "admin_can_reenable", "loopback_reachable"},
		},
		Secrets:    []Secret{{Key: "OPENAI_API_KEY", Label: "OpenAI API key"}, {Key: "OPENAI_API_BASE_URL", Label: "OpenAI-compatible API address"}},
		OwnerLogin: true,
	},
	{
		ID: "openclaw", Name: "OpenClaw", Vendor: "OpenClaw",
		Homepage: "https://openclaw.ai", Source: "https://github.com/openclaw/openclaw",
		Licence: "MIT", Runtime: RuntimeNode, Package: "openclaw", Version: "2026.10.10",
		NodeMajors: []int{24, 26}, Deferred: "command_agent",
		Risk: Risk{CanDo: []string{"runs_host_commands"}, Remains: []string{"prompt_injection", "cross_tenant"}},
	},
	{
		ID: "hermes-agent", Name: "Hermes Agent", Vendor: "Nous Research",
		Homepage: "https://hermes-agent.nousresearch.com", Source: "https://github.com/NousResearch/hermes-agent",
		Licence: "MIT", Runtime: RuntimePython, Package: "hermes-agent", Version: "0.19.0",
		PythonMinors: []int{11, 12, 13}, Deferred: "command_agent_installer",
		Risk: Risk{CanDo: []string{"runs_host_commands"}, Remains: []string{"prompt_injection", "cross_tenant"}},
	},
	{
		ID: "paperclip", Name: "Paperclip", Vendor: "Paperclip",
		Homepage: "https://paperclip.ing", Source: "https://github.com/paperclipai/paperclip",
		Licence: "MIT", Runtime: RuntimeNode, Package: "paperclipai", Version: "2026.1023.0",
		NodeMajors: []int{24}, Deferred: "command_agent_postgres",
		Risk: Risk{CanDo: []string{"runs_host_commands"}, Remains: []string{"prompt_injection", "cross_tenant"}},
	},
}

var appIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,40}$`)

// Catalog returns a copy of every entry, in catalog order.
func Catalog() []App {
	out := make([]App, len(catalog))
	copy(out, catalog)
	return out
}

// Lookup returns one entry.
func Lookup(id string) (App, bool) {
	if !appIDPattern.MatchString(id) {
		return App{}, false
	}
	for _, app := range catalog {
		if app.ID == id {
			return app, true
		}
	}
	return App{}, false
}

// ResellerClosedOffering reports whether an offering id names an app that is
// closed to resellers and their customers, and which app.
func ResellerClosedOffering(offeringID string) (App, bool) {
	appID, ok := strings.CutPrefix(offeringID, "app:")
	if !ok {
		return App{}, false
	}
	app, found := Lookup(appID)
	if !found || app.ResellerClosed == "" {
		return App{}, false
	}
	return app, true
}

// Known reports whether id is a catalog app (offered or not).
func Known(id string) bool { _, ok := Lookup(id); return ok }

// Installable: in the catalog, not deferred, and offerable to customers.
func (a App) Installable() bool {
	return a.Deferred == "" && a.CustomerOfferable && a.ArtifactHash != ""
}

// RiskDigest is a short hash of everything the administrator agrees to when
// offering an app: version, licence, limits and risk codes. An offer records
// it; a new release that changes any of these makes the offer ask again.
func (a App) RiskDigest() string {
	parts := []string{a.ID, a.Version, a.Licence, a.LicenceNote, a.ArtifactHash,
		fmt.Sprint(a.Defaults.MemoryMB, a.Defaults.CPUPercent, a.Defaults.TasksMax, a.Defaults.DiskMB),
		strings.Join(a.Risk.CanDo, ","), strings.Join(a.Risk.Disabled, ","),
		strings.Join(a.Risk.Network, ","), strings.Join(a.Risk.Remains, ",")}
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:8])
}

// ArtifactHex is the pinned hash as lowercase hex, with its algorithm
// ("sha512" or "sha256").
func (a App) ArtifactHex() (algorithm, digest string, err error) {
	switch a.HashKind {
	case HashNPMIntegrity:
		raw, ok := strings.CutPrefix(a.ArtifactHash, "sha512-")
		if !ok {
			return "", "", errors.New("npm integrity is not sha512")
		}
		decoded, err := base64.StdEncoding.DecodeString(raw)
		if err != nil || len(decoded) != 64 {
			return "", "", errors.New("npm integrity is not a valid sha512 value")
		}
		return "sha512", hex.EncodeToString(decoded), nil
	case HashSHA256:
		if len(a.ArtifactHash) != 64 {
			return "", "", errors.New("sha256 pin has the wrong length")
		}
		if _, err := hex.DecodeString(a.ArtifactHash); err != nil {
			return "", "", errors.New("sha256 pin is not hex")
		}
		return "sha256", strings.ToLower(a.ArtifactHash), nil
	}
	return "", "", errors.New("unknown hash kind")
}

// AllowedSecretKeys is the set of environment keys a customer may set.
func (a App) AllowedSecretKeys() map[string]bool {
	out := map[string]bool{}
	for _, s := range a.Secrets {
		out[s.Key] = true
	}
	return out
}

// IDs returns the catalog ids, sorted.
func IDs() []string {
	out := []string{}
	for _, app := range catalog {
		out = append(out, app.ID)
	}
	sort.Strings(out)
	return out
}
