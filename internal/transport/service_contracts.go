package transport

import (
	"time"

	"github.com/alicelik/celikpanel/internal/core"
)

// ServiceMutationJob is the durable state shared by the panel and the
// privileged agent while one host mutation is in progress. Keeping the full
// shape here prevents gob from silently dropping fields when either side is
// upgraded independently.
type ServiceMutationJob struct {
	RequestID      string    `json:"request_id"`
	OwnerID        string    `json:"owner_id"`
	Kind           string    `json:"kind"`
	Target         string    `json:"target"`
	PackageName    string    `json:"package_name,omitempty"`
	Status         string    `json:"status"`
	Phase          string    `json:"phase"`
	Attempt        int       `json:"attempt"`
	StartedAt      time.Time `json:"started_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	LeaseExpiresAt time.Time `json:"lease_expires_at"`
	DeadlineAt     time.Time `json:"deadline_at"`
	FinishedAt     time.Time `json:"finished_at,omitempty"`
	ErrorCode      string    `json:"error_code,omitempty"`
	ErrorMessage   string    `json:"error_message,omitempty"`
	WorkerPID      int       `json:"worker_pid,omitempty"`
	WorkerStarted  string    `json:"worker_started,omitempty"`
	WorkerCommand  string    `json:"worker_command,omitempty"`
}

type ServiceMutationBeginRequest struct {
	RequestID   string `json:"request_id"`
	OwnerID     string `json:"owner_id"`
	Kind        string `json:"kind"`
	Target      string `json:"target"`
	PackageName string `json:"package_name,omitempty"`
	Resume      bool   `json:"resume,omitempty"`
}

type ServiceMutationBinding struct {
	MutationRequestID string `json:"mutation_request_id,omitempty"`
	MutationOwnerID   string `json:"mutation_owner_id,omitempty"`
}

type ServiceMutationRequest struct {
	ServiceMutationBinding
}

// SetServerHostnameRequest asks the privileged agent to give this server the
// fully qualified name the mail stack will answer as. The panel sends only a
// canonical FQDN; the agent revalidates it and refuses anything else.
// SetServerHostnameRequest, ayrıcalıklı agent'tan bu sunucuya, posta yığınının
// adına yanıt vereceği tam nitelikli adı vermesini ister. Panel yalnız kanonik
// bir FQDN gönderir; agent onu yeniden doğrular ve başkasını reddeder.
type SetServerHostnameRequest struct {
	ServiceMutationBinding
	Hostname string `json:"hostname"`
}

type SetServerHostnameResponse struct {
	Hostname string `json:"hostname,omitempty"`
	Previous string `json:"previous,omitempty"`
	Changed  bool   `json:"changed"`
	Error    string `json:"error,omitempty"`
}

type ServiceMutationHeartbeatRequest struct {
	RequestID string `json:"request_id"`
	OwnerID   string `json:"owner_id"`
	Phase     string `json:"phase,omitempty"`
}

type ServiceMutationStatusRequest struct {
	RequestID string `json:"request_id,omitempty"`
}

type ServiceMutationCancelRequest struct {
	RequestID      string `json:"request_id"`
	ExpectedOwner  string `json:"expected_owner"`
	Reason         string `json:"reason,omitempty"`
	FailureCode    string `json:"failure_code,omitempty"`
	FailureMessage string `json:"failure_message,omitempty"`
}

type ServiceMutationFinishRequest struct {
	RequestID   string `json:"request_id"`
	OwnerID     string `json:"owner_id"`
	Success     bool   `json:"success"`
	FailureCode string `json:"failure_code,omitempty"`
	Message     string `json:"message,omitempty"`
}

type ServiceMutationResponse struct {
	Job       *ServiceMutationJob `json:"job,omitempty"`
	ErrorCode string              `json:"error_code,omitempty"`
	Error     string              `json:"error,omitempty"`

	// Reason refines ErrorCode with one of the stable HostMutationReason*
	// codes. The agent already worked out which of three things is blocking a
	// change - somebody else's package manager, another CelikPanel change, or a
	// lock nothing released - and used to throw that away on the way out, so
	// the operator was told a sentence covering all three.
	//
	// The three want different things. Somebody else's package manager is a
	// wait of a minute. Another CelikPanel change is a wait for that change. A
	// lock nothing released is neither: it is something to look at, and telling
	// an operator to wait for it is telling them to wait for something that
	// will not end.
	//
	// Reason, ErrorCode'u kararli HostMutationReason* kodlarindan biriyle
	// inceltir. Agent, bir degisikligi neyin engelledigini zaten hesapliyordu ve
	// bunu disari cikarken atiyordu; operatore de ucunu birden kapsayan bir
	// cumle soyleniyordu.
	Reason string `json:"reason,omitempty"`

	// MutationHold carries why the agent is refusing every durable mutation,
	// as one of the stable MutationHold* codes, or "" when it is accepting
	// them. It rides on the response rather than on the job because the
	// condition is process-wide: it is a property of the agent, not of the
	// operation being asked about.
	//
	// A caller that is waiting for an operation to reach a terminal state MUST
	// stop when this is set. A held agent cannot move any job — heartbeat,
	// finish and cancel all refuse — so the job it reports stays exactly as it
	// is for as long as anyone is willing to poll. Without this field the only
	// signal is the poller's own timeout, which is how a five-second failure
	// became a thirty-minute silence.
	//
	// MutationHold, agent'ın her kalıcı mutasyonu neden reddettiğini kararlı
	// MutationHold* kodlarından biriyle taşır; kabul ediyorsa "" olur. İş
	// üzerinde değil yanıt üzerinde durur, çünkü koşul süreç geneliidir:
	// sorulan işlemin değil agent'ın bir özelliğidir.
	//
	// Bir işlemin uç duruma ulaşmasını bekleyen çağıran, bu ayarlıysa
	// DURMALIDIR. Tutulan bir agent hiçbir işi kımıldatamaz — kalp atışı,
	// bitirme ve iptal hepsi reddeder — dolayısıyla bildirdiği iş, kim ne kadar
	// yoklarsa yoklasın olduğu gibi kalır. Bu alan olmadan tek işaret,
	// yoklayanın kendi zaman aşımıdır; beş saniyelik bir arızanın otuz dakikalık
	// bir sessizliğe dönüşmesinin sebebi tam da budur.
	MutationHold string `json:"mutation_hold,omitempty"`
}

const (
	HostMutationBusy        = "HOST_MUTATION_BUSY"
	HostMutationUnavailable = "HOST_MUTATION_UNAVAILABLE"

	HostMutationReasonPanelOperation  = "panel_operation_active"
	HostMutationReasonAgentMutation   = "agent_mutation_active"
	HostMutationReasonHostLock        = "host_lock_busy"
	HostMutationReasonPackageManager  = "package_manager_active"
	HostMutationReasonStateUnverified = "state_unverified"
)

// HostMutationReadinessResponse is an advisory, read-only snapshot for
// disabling controls that cannot currently start. It never grants a mutation;
// BeginServiceMutation remains the authoritative admission boundary.
type HostMutationReadinessResponse struct {
	Ready  bool   `json:"ready"`
	Code   string `json:"code,omitempty"`
	Reason string `json:"reason,omitempty"`
}

type ServiceMutationActionRequest struct {
	ServiceMutationBinding
	ServiceName string `json:"service_name"`
	Action      string `json:"action"`
}

type ServiceMutationServiceRequest struct {
	ServiceMutationBinding
	ServiceName string `json:"service_name"`
}

type InstallServiceRequest struct {
	ServiceMutationBinding
	ID      string `json:"id"`
	Package string `json:"package,omitempty"`
	// PHPVersion (alpha.105): the PHP version a PHP tool (phpMyAdmin,
	// phpPgAdmin) is installed for and bound to, chosen in a bundle. The
	// tool's extension packages for it are installed with the tool.
	// PHPVersion: bir PHP aracının kurulup bağlandığı PHP sürümü.
	PHPVersion string `json:"php_version,omitempty"`
	// CheckInstalls (alpha.105): the package manager's simulation of this
	// install is classified, and a notable install (another PHP version's
	// packages, Apache, another catalogue component, a database server)
	// refuses the install unless ConfirmedInstalls names it. Additive: an
	// older Panel never sets it.
	// CheckInstalls: dikkat çeken kurulum, onaylanmadıkça kurulumu reddeder.
	CheckInstalls     bool     `json:"check_installs,omitempty"`
	ConfirmedInstalls []string `json:"confirmed_installs,omitempty"`
}

// NotableInstall is one package an install would bring beyond what it
// named that the owner must see and confirm first (alpha.105). Reason:
// new_php_version (Version names it), apache, component (Component names
// the catalogue component), database_server.
// NotableInstall: kurulumun adını söylediklerinin ötesinde getireceği ve
// sahibin önce görüp onaylaması gereken paket.
type NotableInstall struct {
	Package   string `json:"package"`
	Reason    string `json:"reason"`
	Component string `json:"component,omitempty"`
	Version   string `json:"version,omitempty"`
	// From and To (alpha.107, D-038 §12): an already installed package the
	// install would upgrade although it was not asked for (Reason
	// "upgrade"), or a distribution package a vendor repository would
	// replace (Reason "vendor_replaces_distribution", Component names the
	// repository; never confirmable). Additive.
	// From/To: istenmeden yükseltilecek ya da üreticiyle değiştirilecek paket.
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
}

// ServiceInstallPreviewStep is one component install of a preview: the
// component, an optional version package, an optional PHP version for a
// PHP tool.
type ServiceInstallPreviewStep struct {
	ID         string `json:"id"`
	Package    string `json:"package,omitempty"`
	PHPVersion string `json:"php_version,omitempty"`
}

// ServiceInstallPreviewRequest asks, read-only, what installing these
// steps together would bring (alpha.105).
type ServiceInstallPreviewRequest struct {
	Steps []ServiceInstallPreviewStep `json:"steps"`
}

// InstallPreviewRefusal: a step that cannot work as chosen (Code
// php_extensions_unavailable: Packages have no installation candidate for
// Version; php_version_unavailable: PHP Version has no FPM package here;
// php_version_unsupported: the tool has no per-version extensions on this
// system). The install is refused before anything changes.
type InstallPreviewRefusal struct {
	ID       string   `json:"id"`
	Code     string   `json:"code"`
	Version  string   `json:"version,omitempty"`
	Packages []string `json:"packages,omitempty"`
}

// ServiceInstallPreviewResponse is the package manager's answer. State:
// "known" (Installs and Notable are the simulation's answer), "busy"
// (another package operation holds the lock), "unsupported" (this package
// family has no simulation here) or "failed" (Error says why).
// ServiceInstallPreviewResponse, paket yöneticisinin benzetim yanıtıdır.
type ServiceInstallPreviewResponse struct {
	State         string                  `json:"state"`
	PackageFamily string                  `json:"package_family"`
	Requested     []string                `json:"requested"`
	Installs      []string                `json:"installs"`
	Notable       []NotableInstall        `json:"notable"`
	Removes       []string                `json:"removes"`
	Refusals      []InstallPreviewRefusal `json:"refusals"`
	Error         string                  `json:"error,omitempty"`
	// ExistingData (alpha.107, D-038 §8): data an earlier install of a
	// database engine left on this server, which the install will use as
	// it is. Additive.
	ExistingData []ExistingData `json:"existing_data,omitempty"`
}

// ExistingData is one data directory a previous install left.
type ExistingData struct {
	Component string `json:"component"`
	Path      string `json:"path"`
}

type InstallServiceResponse struct {
	Installed bool   `json:"installed"`
	Detail    string `json:"detail,omitempty"`
	Unit      string `json:"unit,omitempty"`
	Error     string `json:"error,omitempty"`
	// PreservedExisting reports that the Agent found an existing
	// implementation it must not change (today: a native cron already on the
	// host) and made no package or unit change. Additive: an older Agent
	// never sets it, and an older Panel ignores it.
	// PreservedExisting, Agent'ın değiştirmemesi gereken mevcut bir
	// uygulama bulduğunu (bugün: sunucuda zaten olan yerel cron) ve hiçbir
	// paket ya da unit değişikliği yapmadığını bildirir. Eklemelidir.
	PreservedExisting bool `json:"preserved_existing,omitempty"`
	// ErrorCode types a refusal (alpha.105): INSTALL_NEEDS_CONFIRMATION
	// (Notable names what the owner has not confirmed) and
	// INSTALL_PHP_VERSION_REFUSED (the chosen PHP version cannot serve the
	// tool). Nothing was installed.
	ErrorCode string           `json:"error_code,omitempty"`
	Notable   []NotableInstall `json:"notable,omitempty"`
	// FirstInstall: the PHP tool was not on the host before this install,
	// so its proof may try another installed PHP version (alpha.105).
	FirstInstall bool `json:"first_install,omitempty"`
}

type UninstallServiceResponse struct {
	Removed         bool   `json:"removed"`
	Detail          string `json:"detail,omitempty"`
	Error           string `json:"error,omitempty"`
	PartialSuccess  bool   `json:"partial_success,omitempty"`
	MutationApplied bool   `json:"mutation_applied,omitempty"`
	// ErrorCode types a refusal (alpha.104): REMOVAL_WOULD_REMOVE_OTHERS
	// (the package manager's simulation would remove packages the removal
	// did not name; Collateral names them), REMOVAL_TOOLS_BOUND (webmail or
	// a database tool runs on the PHP being removed; BoundTools names them),
	// REMOVAL_SIMULATION_FAILED and REMOVAL_PLAN_CHANGED. Additive: an older
	// Agent never sets it.
	// ErrorCode bir reddi türler (alpha.104); eklemelidir.
	ErrorCode  string                     `json:"error_code,omitempty"`
	Collateral []PackageRemovalCollateral `json:"collateral,omitempty"`
	BoundTools []string                   `json:"bound_tools,omitempty"`
}

// PackageRemovalCollateral is one package a removal would take that it did
// not name: Reason is "component" (it belongs to a catalogue component,
// Component), "reverse_dependency" (it depends on a removed package) or
// "would_install" (the package manager would install it).
// PackageRemovalCollateral, bir kaldırmanın adını söylemeden alacağı tek
// pakettir.
type PackageRemovalCollateral struct {
	Package   string `json:"package"`
	Reason    string `json:"reason"`
	Component string `json:"component,omitempty"`
}

// ServiceRemovalPreviewRequest asks what removing a component (or, with
// Package, one version of it) would take, without changing anything.
// ServiceRemovalPreviewRequest, bir bileşeni (ya da Package ile tek sürümünü)
// kaldırmanın neyi alacağını hiçbir şeyi değiştirmeden sorar.
type ServiceRemovalPreviewRequest struct {
	ID      string `json:"id"`
	Package string `json:"package,omitempty"`
}

// ServiceRemovalPreviewResponse is the package manager's simulated answer.
// State: "checked" (Remove and Collateral are the simulation's answer),
// "not_installed" (nothing to remove), "busy" (another package operation
// holds the package lock; nothing was simulated), "unsupported" (this
// package family has no removal here) or "failed" (Error says why).
// ServiceRemovalPreviewResponse, paket yöneticisinin benzetim yanıtıdır.
type ServiceRemovalPreviewResponse struct {
	State         string                     `json:"state"`
	PackageFamily string                     `json:"package_family"`
	Requested     []string                   `json:"requested"`
	Remove        []string                   `json:"remove"`
	Collateral    []PackageRemovalCollateral `json:"collateral"`
	BoundTools    []string                   `json:"bound_tools"`
	Error         string                     `json:"error,omitempty"`
	// KeepsData (alpha.107, D-038 §8): the removal keeps the component's data
	// and configuration (apt remove, not purge); DataPaths names where it
	// stays on this server. Additive.
	KeepsData bool     `json:"keeps_data,omitempty"`
	DataPaths []string `json:"data_paths,omitempty"`
	// HoldsDatabases (alpha.108, D-038 §2): the PostgreSQL clusters of this
	// removal that still hold user databases ("17/main: shop, blog"); the
	// removal is refused while there is one. DatabasesUnknown are the
	// clusters that could not be asked (not running); the removal is refused
	// for them too.
	HoldsDatabases   []string `json:"holds_databases,omitempty"`
	DatabasesUnknown []string `json:"databases_unknown,omitempty"`
}

type ConfigureDBToolsResponse struct {
	Configured bool     `json:"configured"`
	Tools      []string `json:"tools"`
	Error      string   `json:"error,omitempty"`
	// Warning (alpha.104): the configuration was written, but a tool's PHP
	// needs attention (its bound version is gone, or its extensions could
	// not be installed); the installed tool itself is fine.
	Warning string `json:"warning,omitempty"`
	// ToolProblems are the same, per tool, for a screen to translate.
	ToolProblems []PHPToolProblem `json:"tool_problems,omitempty"`
}

// PHPToolProblem: a tool's PHP needs attention after a configuration.
// Kind: version_gone (its bound Version is no longer installed) or
// extensions_failed (its PHP extensions for Version could not be installed).
type PHPToolProblem struct {
	Tool    string `json:"tool"`
	Version string `json:"version"`
	Kind    string `json:"kind"`
}

type ConfigureDKIMSigningResponse struct {
	Configured bool   `json:"configured"`
	Domains    int    `json:"domains"`
	Detail     string `json:"detail,omitempty"`
	Error      string `json:"error,omitempty"`
}

type ConfigureMailStackResponse struct {
	Configured bool   `json:"configured"`
	Detail     string `json:"detail,omitempty"`
	Error      string `json:"error,omitempty"`
}

type WireMailFiltersResponse struct {
	Wired  bool   `json:"wired"`
	Detail string `json:"detail,omitempty"`
	Error  string `json:"error,omitempty"`
}

// MailFilterWiringStateResponse answers, read-only, whether this host has the
// mail server whose milter chain the panel composes. R-056: the startup repair
// used to find that out only after it had taken the whole-host mutation lease,
// so a machine with no mail at all recorded a failed privileged operation on
// every panel start. The question is one lookup and belongs before the ledger,
// not inside it.
//
// MailFilterWiringStateResponse, bu makinede panelin milter zincirini
// besteledigi posta sunucusunun bulunup bulunmadigini salt-okunur yanitlar.
// R-056: baslangic onarimi bunu ancak makinenin tamamini alan kirayi aldiktan
// sonra ogreniyordu.
type MailFilterWiringStateResponse struct {
	// MailServerInstalled is true when a Postfix this panel could wire is
	// present on the host.
	MailServerInstalled bool `json:"mail_server_installed"`
	// Detail says what was found, in the product's plain voice, so a caller
	// that decides not to act can say why.
	Detail string `json:"detail,omitempty"`
}

type ConfigureMailSubmissionResponse struct {
	Configured bool   `json:"configured"`
	Detail     string `json:"detail,omitempty"`
	Error      string `json:"error,omitempty"`
}

type EnsureNginxReadyResponse struct {
	Ready   bool   `json:"ready"`
	Changed bool   `json:"changed"`
	Error   string `json:"error,omitempty"`
}

type WebmailMutationRequest struct {
	ServiceMutationBinding
}

type InstallRoundcubeResponse struct {
	Installed bool   `json:"installed"`
	Version   string `json:"version"`
	Error     string `json:"error,omitempty"`
	// FirstInstall: Roundcube was not on the host before (alpha.105).
	FirstInstall bool `json:"first_install,omitempty"`
}

type RemoveRoundcubeResponse struct {
	Removed         bool   `json:"removed"`
	MutationApplied bool   `json:"mutation_applied,omitempty"`
	Error           string `json:"error,omitempty"`
}

type ConfigureWebmailResponse struct {
	Configured bool   `json:"configured"`
	Present    bool   `json:"present"`
	Error      string `json:"error,omitempty"`
}

type InstalledRepoPackagesRequest struct {
	ServiceID string `json:"service_id"`
}

type InstalledRepoPackagesResponse struct {
	Packages []string `json:"packages"`
	Error    string   `json:"error,omitempty"`
}

type ServiceInstancesRequest struct {
	ID string `json:"id"`
}

type ServiceInstancesResponse struct {
	Instances []core.ServiceInstance `json:"instances"`
	Error     string                 `json:"error,omitempty"`
}

type NodeVersionsResponse struct {
	Installed     []string `json:"installed"`
	SystemVersion string   `json:"system_version"`
}

type NodeInstallRequest struct {
	ServiceMutationBinding
	Version string `json:"version"`
}

type NodeInstallResponse struct {
	Installed bool   `json:"installed"`
	Error     string `json:"error,omitempty"`
}

type NodeRemoveRequest struct {
	ServiceMutationBinding
	Version string `json:"version"`
}

type NodeRemoveResponse struct {
	Removed bool   `json:"removed"`
	Error   string `json:"error,omitempty"`
}

type NodeLTSRelease struct {
	Version string `json:"version"`
	Name    string `json:"name"`
}

type NodeLTSResponse struct {
	Releases []NodeLTSRelease `json:"releases"`
	Error    string           `json:"error,omitempty"`
}

type ServiceJournalRequest struct {
	Unit  string `json:"unit"`
	Lines int    `json:"lines"`
}

type ServiceJournalResponse struct {
	Unit  string   `json:"unit"`
	Lines []string `json:"lines"`
	Error string   `json:"error,omitempty"`
}

// InstalledPackageVersionsResponse answers Agent.InstalledPackageVersionsV1
// (v0.1.0-alpha.97): the installed package versions of the catalogue
// components, read from the system package database in one bounded read-only
// query. State says what the read established:
//   - "read": the database answered; Components lists every component with at
//     least one installed catalogue package (a component that is absent is
//     not listed, so its version is unknown, never guessed);
//   - "busy": a package operation was running, so nothing was read and the
//     caller keeps its previous answer, marked as such;
//   - "unsupported": this package family has no version read;
//   - "failed": the query did not complete; Error says why, without output.
//
// InstalledPackageVersionsResponse, katalog bileşenlerinin kurulu paket
// sürümlerini sistemin paket veritabanından tek salt-okur sorguyla bildirir.
type InstalledPackageVersionsResponse struct {
	Family     string                     `json:"family"`
	State      string                     `json:"state"`
	Error      string                     `json:"error,omitempty"`
	Components []ComponentPackageVersions `json:"components"`
}

// Installed package version read states.
const (
	PackageVersionsRead        = "read"
	PackageVersionsBusy        = "busy"
	PackageVersionsUnsupported = "unsupported"
	PackageVersionsFailed      = "failed"
)

// ComponentPackageVersions is one catalogue component's installed packages,
// each with the version string exactly as the package database prints it
// (epoch and revision kept, for example 1:10.11.13-0+deb13u1).
type ComponentPackageVersions struct {
	ServiceID string                    `json:"service_id"`
	Packages  []InstalledPackageVersion `json:"packages"`
}

// InstalledPackageVersion is one installed package and its version.
type InstalledPackageVersion struct {
	Package string `json:"package"`
	Version string `json:"version"`
}

// PHPToolBinding is the PHP version a tool (webmail, phpmyadmin,
// phppgadmin) is served from (alpha.104). Version is empty when nothing is
// bound; SocketReady says the version's PHP-FPM socket exists right now;
// VersionInstalled that the version is still installed. Source is "config"
// (the generated nginx configuration) or "record" (the Agent's record).
// PHPToolBinding, bir aracın sunulduğu PHP sürümüdür.
type PHPToolBinding struct {
	Tool             string `json:"tool"`
	Installed        bool   `json:"installed"`
	Version          string `json:"version"`
	SocketReady      bool   `json:"socket_ready"`
	VersionInstalled bool   `json:"version_installed"`
	Source           string `json:"source,omitempty"`
	// ExtensionsPresent: the tool's PHP extension packages are installed for
	// its version (package presence, not proof that PHP loads them).
	ExtensionsPresent bool `json:"extensions_present"`
	// ProofFailed (alpha.105): the tool's last proof after its install did
	// not pass on any installed PHP version; ProofCause says why for the
	// version it runs on. Cleared by a proof that passes (a move in Manage).
	ProofFailed bool          `json:"proof_failed,omitempty"`
	ProofCause  *PHPToolCause `json:"proof_cause,omitempty"`
	// ProofTriedOthers: other installed versions were tried too.
	ProofTriedOthers bool `json:"proof_tried_others,omitempty"`
	// FPMState (alpha.108) is the version's PHP-FPM unit state read now
	// ("active", "inactive", "failed", "activating", ... or "unknown"); ""
	// when no versioned unit applies. With SocketReady it is what may say
	// "not running"; a failed proof alone never does.
	FPMState string `json:"fpm_state,omitempty"`
}

// PHPToolCause says why a tool move failed, in a form a screen translates
// (alpha.104). Kind: missing_extensions (Extensions), wrong_version
// (Running), no_answer, check_unreadable (Status; alpha.108: PHP-FPM could
// not open CelikPanel's check script), http_status (Status), served_as_file, tool_error,
// not_tool_page, not_running, version_not_installed, tool_not_installed,
// unsupported, extensions_install_failed (Extensions), fpm_reload_failed,
// config_owner_edited (Detail: the file), config_prepare_failed,
// config_read_failed, config_write_failed, nginx_rejected,
// nginx_reload_failed, other. Detail is the Agent's own words.
// PHPToolCause, bir taşımanın neden başarısız olduğunu çevrilebilir söyler.
type PHPToolCause struct {
	Kind       string   `json:"kind"`
	Version    string   `json:"version,omitempty"`
	Extensions []string `json:"extensions,omitempty"`
	Status     int      `json:"status,omitempty"`
	Running    string   `json:"running,omitempty"`
	Detail     string   `json:"detail,omitempty"`
}

// PHPToolBindingsResponse lists every tool's binding and the installed
// PHP-FPM versions, newest first.
type PHPToolBindingsResponse struct {
	Tools    []PHPToolBinding `json:"tools"`
	Versions []string         `json:"versions"`
	Error    string           `json:"error,omitempty"`
}

// ProvePHPToolRequest proves a PHP tool just installed (alpha.105): it
// must answer as itself on its PHP version; with AllowFallback (a first
// install) the other installed versions that have its extensions are tried
// in order, the distribution's default first.
type ProvePHPToolRequest struct {
	ServiceMutationBinding
	Tool          string `json:"tool"`
	AllowFallback bool   `json:"allow_fallback,omitempty"`
}

// PHPToolAttempt is one version a proof tried and why it failed.
type PHPToolAttempt struct {
	Version string        `json:"version"`
	Cause   *PHPToolCause `json:"cause,omitempty"`
}

// ProvePHPToolResponse: Proved when the tool answered on Version; Moved
// when that is not the version it was configured on first. Attempts lists
// the versions that failed, in order. A tool that was not proved stays
// installed and reads "Not ready".
type ProvePHPToolResponse struct {
	Tool     string           `json:"tool"`
	Version  string           `json:"version,omitempty"`
	Proved   bool             `json:"proved"`
	Moved    bool             `json:"moved,omitempty"`
	Attempts []PHPToolAttempt `json:"attempts,omitempty"`
	Error    string           `json:"error,omitempty"`
}

// BindPHPToolRequest moves one tool to one installed PHP version.
type BindPHPToolRequest struct {
	ServiceMutationBinding
	Tool    string `json:"tool"`
	Version string `json:"version"`
}

// BindPHPToolResponse: Bound after the tool answered on the new version.
// On failure ErrorCode types it, MutationApplied says the host changed
// (extension packages installed or nginx reloaded) and Restored that the
// previous configuration was put back. Previous is the version before.
// BindPHPToolResponse: araç yeni sürümde yanıt verdikten sonra Bound.
type BindPHPToolResponse struct {
	Bound             bool     `json:"bound"`
	Tool              string   `json:"tool"`
	Version           string   `json:"version"`
	Previous          string   `json:"previous,omitempty"`
	InstalledPackages []string `json:"installed_packages,omitempty"`
	MutationApplied   bool     `json:"mutation_applied,omitempty"`
	Restored          bool     `json:"restored,omitempty"`
	Warning           string   `json:"warning,omitempty"`
	Error             string   `json:"error,omitempty"`
	ErrorCode         string   `json:"error_code,omitempty"`
	// Detail is the cause alone, for a screen that quotes it.
	Detail string `json:"detail,omitempty"`
	// Cause is the same, structured and translatable.
	Cause *PHPToolCause `json:"cause,omitempty"`
	// ProofOnly (alpha.108): the tool was already configured on that
	// version, so nothing was rewritten or reloaded; only the proof ran.
	ProofOnly bool `json:"proof_only,omitempty"`
}

// NodePatch is one installed Node.js version and the newest release on its
// line in the official index (alpha.107, D-038 §9): Newer is true when a
// newer patch exists. LTS names the line's LTS codename, if it has one.
type NodePatch struct {
	Installed string `json:"installed"`
	Line      string `json:"line"`
	Newest    string `json:"newest,omitempty"`
	Newer     bool   `json:"newer"`
	LTS       string `json:"lts,omitempty"`
}

type NodePatchesResponse struct {
	Patches   []NodePatch `json:"patches"`
	CheckedAt string      `json:"checked_at,omitempty"`
	Error     string      `json:"error,omitempty"`
}
