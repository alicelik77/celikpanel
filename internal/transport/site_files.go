package transport

// Site files the Panel writes, and the owner's edits to them (D-031, 10 Oct
// 2026). Every field here is additive on the gob wire: an older Agent leaves
// them empty or nil, an older Panel ignores them. A Panel that receives no
// File/Items answer from an Agent says the file's state is unknown; it never
// reads "no answer" as "unchanged".
//
// Panel'in yazdığı site dosyaları ve sahibin onlarda yaptığı değişiklikler
// (D-031). Buradaki her alan gob hattında eklemelidir: eski bir Agent boş ya
// da nil bırakır, eski bir Panel yok sayar. Yanıt alamayan Panel dosyanın
// durumunu "bilinmiyor" diye söyler; "yanıt yok"u asla "değişmedi" okumaz.

// SiteFileKindNginxVhost is the site's nginx vhost in sites-available. The
// PHP-FPM pool (php_pool) and the application unit (app_unit) are the second
// step of D-031 (2026-10-11) and use the same states under their own kinds.
const (
	SiteFileKindNginxVhost = "nginx_vhost"
	SiteFileKindPHPPool    = "php_pool"
	SiteFileKindAppUnit    = "app_unit"
)

// The classifier's verdict on the file found on disk before an operation.
const (
	// SiteFileAbsent: no file at the path.
	SiteFileAbsent = "absent"
	// SiteFileManagedUnchanged: the first line is the render header and the
	// digest it declares is the digest of the body under it; or a file with
	// no header that is byte for byte what a frozen earlier release's
	// template renders from the same data (AdoptedFrom names that release).
	SiteFileManagedUnchanged = "managed_unchanged"
	// SiteFileOwnerEdited: the header is present, the body under it differs.
	SiteFileOwnerEdited = "owner_edited"
	// SiteFileForeign: no header, and the Panel has a record of having written
	// this file under the header: it was replaced wholesale.
	SiteFileForeign = "foreign"
	// SiteFileUnreadable: a symlink, not a regular file, too large, or the
	// read failed. Never followed, never written.
	SiteFileUnreadable = "unreadable"
	// SiteFileUnknownOrigin: no header, no record, and not byte-equal to any
	// known CelikPanel text: a file from before D-031 that differs from every
	// frozen render (the owner's edit, or a render whose inputs changed).
	SiteFileUnknownOrigin = "unknown_origin"
	// SiteFileStateUnknown is the Panel's word when the Agent did not report
	// a state (an older Agent, or the Agent could not be asked).
	SiteFileStateUnknown = "unknown"
	// SiteFileMissingState is the Panel's stored state after an operation
	// left an absent file absent (the owner removed it).
	SiteFileMissingState = "missing"
	// SiteFileUnwritable is the Panel's word for a file that was read
	// (classified managed and unchanged) but whose replacement was refused
	// (SiteFileReasonWriteRefused: an immutable file, a read-only file
	// system). It is not "unreadable": the file reads fine. The ledger keeps
	// migration 044's state set and stores it as SiteFileUnreadable with the
	// reason write_refused; the domain screen and the domains list answer
	// "unwritable" (set10 scenario g, 2026-10-10).
	SiteFileUnwritable = "unwritable"
)

// What a read-only look at a file found that would refuse its replacement
// (SiteFileResult.Protection). Measured by the Agent without writing: the
// file's and its directory's attribute flags and the file system's mount
// flags. "" means not measured (another platform, or the file is not there).
const (
	SiteFileProtectionNone               = "none"
	SiteFileProtectionImmutable          = "immutable"
	SiteFileProtectionAppendOnly         = "append_only"
	SiteFileProtectionDirectoryImmutable = "directory_immutable"
	SiteFileProtectionReadOnlyFilesystem = "read_only_filesystem"
)

// What the operation did with the file.
const (
	SiteFileOutcomeWritten   = "written"
	SiteFileOutcomeUnchanged = "unchanged"
	SiteFileOutcomeKept      = "kept"
	SiteFileOutcomeMissing   = "missing"
	SiteFileOutcomeRefused   = "refused"
	SiteFileOutcomeFailed    = "failed"
	SiteFileOutcomeRecreated = "recreated"
	SiteFileOutcomeTaken     = "taken"
	SiteFileOutcomeInspected = "inspected"
)

// Why a render runs. Only create and recreate write an absent file; only take
// replaces a file the Panel did not leave unchanged.
const (
	SiteFileTriggerStartup  = "startup"
	SiteFileTriggerChange   = "change"
	SiteFileTriggerCreate   = "create"
	SiteFileTriggerRecreate = "recreate"
	SiteFileTriggerTake     = "take"
)

// Typed reasons for unreadable, refused and failed outcomes.
const (
	SiteFileReasonSymlink       = "symlink"
	SiteFileReasonNotRegular    = "not_regular"
	SiteFileReasonPermission    = "permission"
	SiteFileReasonTooLarge      = "too_large"
	SiteFileReasonReadFailed    = "read_failed"
	SiteFileReasonWriteRefused  = "write_refused"
	SiteFileReasonChanged       = "changed_since_shown"
	SiteFileReasonNginxRefused  = "nginx_refused"
	SiteFileReasonReloadFailed  = "reload_failed"
	SiteFileReasonRenderFailed  = "render_failed"
	SiteFileReasonNotApplicable = "not_applicable"
	SiteFileReasonPendingNotSet = "pending_not_written"
	// SiteFileReasonConfigRefused: the service's own check refused the text
	// (php-fpm -t for a pool; D-031 second step), or the service manager did
	// not take it; the previous file was put back.
	SiteFileReasonConfigRefused = "config_refused"
)

// SiteConfigExists: site creation found a vhost file at the site's path that
// CelikPanel did not leave unchanged; it was kept and the site was not
// created. Additive CreateSiteResponse.ErrorCode.
const SiteConfigExists = "site_config_exists"

// SiteFileResult is one site file's classification and what an operation did
// with it. Paths are this server's paths; the Panel shows them to an
// administrator. The body of a file never travels in this struct.
type SiteFileResult struct {
	Kind    string
	Path    string
	State   string
	Outcome string
	Reason  string
	// Detail is one bounded line (for nginx_refused: nginx's own line).
	Detail      string
	AdoptedFrom string
	// RenderSHA256 is the digest of the Panel's text (the body under the
	// header) for this operation's inputs.
	RenderSHA256 string
	// WrittenSHA256 is the body digest the file on disk now declares and
	// carries, when it is the Panel's text after this operation.
	WrittenSHA256 string
	// FileSHA256 is the digest of the whole file on disk after the operation
	// ("" when there is none).
	FileSHA256    string
	PendingPath   string
	PendingSHA256 string
	BackupPath    string
	// IncludeDir is the owner's include directory for the site.
	IncludeDir string
	// Enabled describes sites-enabled/<domain>.conf: "link" (the link to this
	// file), "absent", or "other" (anything else; never touched).
	Enabled string
	// Reloaded is true when this operation reloaded nginx.
	Reloaded bool

	// D-031 step 1b, additive. ManagedDir is the Panel's own include
	// directory of the site (the ACME HTTP-01 location lives there, not in
	// the vhost); ManagedInclude is the exact line a vhost needs to read it.
	ManagedDir     string
	ManagedInclude string
	// ChallengeFile is what happened to the challenge file in ManagedDir
	// (SiteFileChallenge*); "" when the operation did not look at it.
	ChallengeFile string
	// Validation is, for a kept file (owner-edited, foreign, unknown origin),
	// whether a certificate for the site can be validated without changing
	// the file (SiteFileValidation*); "" for any other file.
	Validation string
	// CertificateReferenced: the kept file has the line `ssl_certificate
	// <path>;` for the certificate path of this render.
	CertificateReferenced bool
	// The validation probe (additive; only when the operation asked for it).
	// ValidationName is the first name that did not answer the probe, and
	// ValidationStatus the HTTP status nginx gave for it (0: none); for
	// SiteFileValidationUnknown, ValidationDetail says why the probe could
	// not be made.
	ValidationName   string
	ValidationStatus int
	ValidationDetail string
	// Additive (set11 cell A6, 2026-10-11). ValidationAddress is the address
	// the probe asked: for a name that did not serve the probe, the address
	// that answered; for ready, every address asked (comma-separated); for
	// SiteFileValidationUnknown, the address that did not answer or could not
	// be asked. ValidationUnknownReason is SiteFileValidationUnknown*, ""
	// when the reason is that nginx did not answer (or an older Agent).
	ValidationAddress       string
	ValidationUnknownReason string

	// Additive (set10 scenario g). SystemError is the system's error name for
	// a refused replacement ("EPERM", "EROFS", "EACCES", …): the one the write
	// got, or, on a read, the one a write would get given Protection. "" when
	// none is known. Protection is SiteFileProtection*.
	SystemError string
	Protection  string

	// Restore, additive (2026-10-11), only with Outcome failed: whether the
	// Agent put the previous bytes back and the service took them again
	// (SiteFileRestoreConfirmed; since set12 also when the service's check
	// refused the put-back configuration and the service was deliberately not
	// reloaded, or was not running before: Service says which), or the
	// restore or the reload after it failed (SiteFileRestoreUnconfirmed; the Agent's error
	// then says "rollback incomplete"). "" from an older Agent: the Panel
	// treats it as unconfirmed.
	Restore string

	// Additive (set12 cell RP1, 2026-10-11), only with Outcome failed: what
	// happened to the service after the previous file was put back
	// (SiteFileService*). Before reloading the put-back configuration the
	// service's own check runs on it (php-fpm -t, nginx -t, systemd-analyze
	// verify); when the check refuses it, the service is NOT reloaded and keeps
	// the configuration it loaded before (Service not_reloaded, ServiceReason
	// config_refused_elsewhere, ServiceCheck the check's first line). "" from
	// an older Agent.
	Service       string
	ServiceReason string
	ServiceCheck  string
	// ServiceActive is `systemctl is-active` of the service after the
	// operation: "true", "false" or "unknown" (SiteFileServiceActive*).
	ServiceActive string
}

// What the Agent knows of a failed operation's restore (SiteFileResult.Restore).
const (
	SiteFileRestoreConfirmed   = "confirmed"
	SiteFileRestoreUnconfirmed = "unconfirmed"
)

// What happened to the service after a put-back (SiteFileResult.Service).
const (
	// SiteFileServiceReloaded: the check passed and the service was reloaded
	// with the put-back configuration (for an application unit: restarted
	// with try-restart).
	SiteFileServiceReloaded = "reloaded"
	// SiteFileServiceNotReloaded: the check refused the put-back
	// configuration (ServiceReason config_refused_elsewhere) or could not be
	// run (check_unavailable); the service was not reloaded.
	SiteFileServiceNotReloaded = "not_reloaded"
	// SiteFileServiceNotRunning: the service was not running before the
	// operation; it was neither reloaded nor started.
	SiteFileServiceNotRunning = "not_running"
	// SiteFileServiceReloadFailed: the check passed and the reload failed.
	SiteFileServiceReloadFailed = "reload_failed"
)

// Why a service was not reloaded after a put-back (SiteFileResult.ServiceReason).
const (
	SiteFileServiceReasonConfigRefusedElsewhere = "config_refused_elsewhere"
	SiteFileServiceReasonCheckUnavailable       = "check_unavailable"
)

// `systemctl is-active` of a service (SiteFileResult.ServiceActive).
const (
	SiteFileServiceActiveTrue    = "true"
	SiteFileServiceActiveFalse   = "false"
	SiteFileServiceActiveUnknown = "unknown"
)

// What happened to the Panel's challenge file (D-031 step 1b).
const (
	SiteFileChallengeWritten   = "written"
	SiteFileChallengeUnchanged = "unchanged"
	// SiteFileChallengeKept: the file in the Panel's directory is not the
	// Panel's unchanged text; it was kept and not replaced.
	SiteFileChallengeKept = "kept"
	// SiteFileChallengeFailed: it could not be written, or nginx refused the
	// configuration with it and it was put back.
	SiteFileChallengeFailed = "failed"
	// SiteFileChallengeAbsent and SiteFileChallengeDiffers are read-only
	// observations (the domain screen's inspection).
	SiteFileChallengeAbsent  = "absent"
	SiteFileChallengeDiffers = "differs"
)

// Whether a kept file lets a certificate be validated (D-031 step 1b).
const (
	// Measured by a probe (services/managed_vhost_probe.go), never read from
	// the file's text: ready means nginx served the probe under every name.
	SiteFileValidationReady = "ready"
	// SiteFileValidationIncludeMissing: nginx did not serve the probe under
	// one of the site's own names (the file's port-80 block does not read the
	// Panel's directory, or answers otherwise).
	SiteFileValidationIncludeMissing = "include_missing"
	// SiteFileValidationNamesMissing: the site's names were served, but not a
	// validation-only name such as mail.<domain>.
	SiteFileValidationNamesMissing = "names_missing"
	// SiteFileValidationChallengeKept: the challenge file in the Panel's
	// directory was changed outside CelikPanel and was kept.
	SiteFileValidationChallengeKept = "challenge_kept"
	// SiteFileValidationChallengeFailed: the challenge file could not be
	// written, or nginx refused the configuration with it.
	SiteFileValidationChallengeFailed = "challenge_failed"
	// SiteFileValidationUnknown: the probe could not be made (nginx did not
	// answer on port 80, or the probe file could not be written), so whether
	// the file lets the validation run is not known. Never "not ready".
	SiteFileValidationUnknown = "unknown"
	// SiteFileValidationUnknownAddressNotLocal (ValidationUnknownReason): the
	// site's port-80 block listens on an address (ValidationAddress) that is
	// not an address of this server, so the probe cannot be sent to it from
	// this server.
	SiteFileValidationUnknownAddressNotLocal = "address_not_local"
)

// SiteFileBatchItem is one item of a start-up batch.
type SiteFileBatchItem struct {
	DomainID int
	SiteID   int
	File     SiteFileResult
	// Error is this item's own failure (render input refused, challenge root
	// not prepared); the other items went on.
	Error string
}

// SiteFileCounts is the start line's summary.
type SiteFileCounts struct {
	Written    int
	Unchanged  int
	Kept       int
	Foreign    int
	Unknown    int
	Unreadable int
	Missing    int
	Adopted    int
	Failed     int
}

// InspectSiteFileResponse is the read-only answer for the domain screen. Diff
// is a unified diff of the file on disk against the Panel's text, bounded in
// size, with lines that look like credentials hidden.
type InspectSiteFileResponse struct {
	File          SiteFileResult
	Diff          string
	DiffTruncated bool
	Error         string
}

// The operations of the site's PHP-FPM pool file (D-031 second step,
// 2026-10-11). Inspect reads; every other one classifies the file first and
// writes only what the render rule allows.
const (
	SitePoolOperationInspect  = "inspect"
	SitePoolOperationSave     = "save"
	SitePoolOperationSwitch   = "switch"
	SitePoolOperationTake     = "take"
	SitePoolOperationRecreate = "recreate"
	SitePoolOperationApply    = "apply"
	// SitePoolOperationCreate writes the site's pool where none is (trigger
	// "create"): a site that becomes a PHP site after it was created as
	// another type (static, Node, proxy, forwarding). An existing pool is
	// classified as for every other write; one the owner changed is kept.
	SitePoolOperationCreate = "create"
)

// SitePoolValues are the process-manager settings CelikPanel renders into a
// site's pool (the pool page's form). The pool's identity (user, group,
// socket, socket owner and mode) is the site's and never the caller's.
type SitePoolValues struct {
	PM                string
	PMMaxChildren     int
	PMStartServers    int
	PMMinSpareServers int
	PMMaxSpareServers int
	PMMaxRequests     int
}

// SitePoolRequest names one hosted site's PHP-FPM pool. Username is the
// site's system user (services.SiteUsername of the domain); the Agent renders
// the identity lines from it and this host's layout.
type SitePoolRequest struct {
	ExpectedBuildCommit string
	SiteID              int
	Username            string
	PHPVersion          string
	Operation           string
	// NewPHPVersion is the target of SitePoolOperationSwitch.
	NewPHPVersion string
	// Values are the pool page's settings (SitePoolOperationSave).
	Values               *SitePoolValues
	RecordedSHA256       string
	ExpectedFileSHA256   string
	ExpectedRenderSHA256 string
}

// ApplySiteFileResponse is a pool operation's answer. File is nil only when
// Error says why nothing was classified.
type ApplySiteFileResponse struct {
	File  *SiteFileResult
	Error string
}
