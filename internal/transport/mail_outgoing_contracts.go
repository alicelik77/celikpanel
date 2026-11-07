package transport

// Outgoing mail limits (alpha.128, gap G1; owner decisions of 7 Nov 2026).
//
// The brake is postfwd, the Postfix policy daemon from the distribution's own
// repository, asked only by the submission (587) and smtps (465) services at
// the end of each message, with Postfix's default action DUNNO: when postfwd
// is stopped or removed, mail keeps flowing. The panel sends the whole wanted
// state; the Agent renders postfwd's native rule file and lists, checks them,
// reloads postfwd keeping its counters, and proves with a self-test request
// that the running daemon counts. Everything keeps working without the panel.
//
// Giden posta sınırları: postfwd yalnız 587/465'te ileti sonunda sorulur;
// Postfix'in varsayılan eylemi DUNNO'dur, postfwd durursa posta akmaya devam
// eder. Panel istenen durumun tamamını gönderir; Agent yerel kural dosyasını
// yazar, denetler, sayaçları koruyarak yeniden yükler ve öz sınamayla
// çalışan sürecin saydığını kanıtlar.

// Hard bounds a spec must stay within (a bigger server is a design change).
const (
	OutgoingMailMaxPerHour      = 1_000_000
	OutgoingMailMaxMailboxes    = 200_000
	OutgoingMailMaxDomains      = 50_000
	OutgoingMailMaxSubscription = 50_000
)

// Refusal codes of Agent.ApplyOutgoingMailLimitsV1 (stable; the screen words
// them, the code never reaches a person as is).
const (
	OutgoingMailNotInstalled   = "OUTGOING_MAIL_NOT_INSTALLED"
	OutgoingMailNoPostfix      = "OUTGOING_MAIL_NO_POSTFIX"
	OutgoingMailOwnerConfig    = "OUTGOING_MAIL_OWNER_CONFIG"
	OutgoingMailOwnerWiring    = "OUTGOING_MAIL_OWNER_WIRING"
	OutgoingMailInvalidSpec    = "OUTGOING_MAIL_INVALID_SPEC"
	OutgoingMailCheckFailed    = "OUTGOING_MAIL_CHECK_FAILED"
	OutgoingMailServiceFailed  = "OUTGOING_MAIL_SERVICE_FAILED"
	OutgoingMailPostfixFailed  = "OUTGOING_MAIL_POSTFIX_FAILED"
	OutgoingMailSelfTestFailed = "OUTGOING_MAIL_SELFTEST_FAILED"
	OutgoingMailBusy           = "OUTGOING_MAIL_BUSY"
)

// OutgoingMailLimitsSpec is the whole wanted state. Enabled false removes the
// policy check from submission and smtps (postfwd itself keeps running with
// the last rules; nothing is counted because Postfix no longer asks).
type OutgoingMailLimitsSpec struct {
	Enabled bool `json:"enabled"`
	// Paused mailboxes get a permanent refusal until their password changes.
	Paused []string `json:"paused,omitempty"`
	// MailboxLimits: recipients per hour per mailbox address (absent =
	// no mailbox limit; domain and subscription totals still apply).
	MailboxLimits map[string]int `json:"mailbox_limits,omitempty"`
	// DomainLimits: recipients per hour for all mailboxes of one domain.
	DomainLimits map[string]int `json:"domain_limits,omitempty"`
	// Subscriptions: recipients per hour for all domains of one hosting
	// account. Key is a short stable token ("s12").
	Subscriptions []OutgoingMailSubscription `json:"subscriptions,omitempty"`
}

// OutgoingMailSubscription is one hosting account's total.
type OutgoingMailSubscription struct {
	Key     string   `json:"key"`
	Domains []string `json:"domains"`
	Limit   int      `json:"limit"`
}

// OutgoingMailLimitsApplyResponse is Agent.ApplyOutgoingMailLimitsV1's answer.
type OutgoingMailLimitsApplyResponse struct {
	Applied bool `json:"applied"`
	// Changed: a file, the drop-in or master.cf was written (false: the
	// server already held this state; postfwd was still checked).
	Changed bool `json:"changed,omitempty"`
	// Verified: the running postfwd answered the self-test with its refusal,
	// so it counts.
	Verified bool   `json:"verified,omitempty"`
	Code     string `json:"code,omitempty"`
	Detail   string `json:"detail,omitempty"`
	Error    string `json:"error,omitempty"`
	// Steps: what was done, in order (real server steps for the progress
	// view; no secret).
	Steps []string `json:"steps,omitempty"`
	// Status after the call.
	Status OutgoingMailLimitsStatus `json:"status"`
}

// OutgoingMailLimitsStatus is what the server holds now (read only).
type OutgoingMailLimitsStatus struct {
	// Postfix: a Postfix is installed. Installed: postfwd's program is.
	Postfix   bool   `json:"postfix"`
	Installed bool   `json:"installed"`
	Family    string `json:"family,omitempty"`
	// ConfigPath: postfwd's rule file on this distribution.
	ConfigPath string `json:"config_path,omitempty"`
	// ConfigState: absent, package (the distribution's own file), panel
	// (CelikPanel's unchanged file) or owner (changed on the server; kept).
	ConfigState string `json:"config_state,omitempty"`
	// ServiceActive: postfwd's unit is running.
	ServiceActive bool `json:"service_active"`
	// Wiring: on (submission and smtps ask postfwd), off, partial or owner
	// (a value the server owner set is kept).
	Wiring       string `json:"wiring,omitempty"`
	WiringDetail string `json:"wiring_detail,omitempty"`
	// Verified: a self-test made now was refused by postfwd (it counts).
	Verified bool   `json:"verified,omitempty"`
	Detail   string `json:"detail,omitempty"`
	Error    string `json:"error,omitempty"`
}

// OutgoingMailActivityRequest asks for Postfix's submission lines since a
// time (RFC 3339, at most 8 days back).
type OutgoingMailActivityRequest struct {
	Since string `json:"since"`
}

// OutgoingMailActivityResponse counts what each mailbox sent per hour.
type OutgoingMailActivityResponse struct {
	Read       bool                 `json:"read"`
	Incomplete bool                 `json:"incomplete,omitempty"`
	Buckets    []OutgoingMailBucket `json:"buckets"`
	Error      string               `json:"error,omitempty"`
}

// OutgoingMailBucket is one mailbox's hour (UTC hour start, RFC 3339).
// Messages and Recipients were accepted; the refusals were not.
type OutgoingMailBucket struct {
	Hour       string `json:"hour"`
	Address    string `json:"address"`
	Messages   int    `json:"messages"`
	Recipients int    `json:"recipients"`
	// LimitRefusals: messages refused by an hourly limit; LimitKind the last
	// one seen (mailbox, domain or subscription).
	LimitRefusals int    `json:"limit_refusals,omitempty"`
	LimitKind     string `json:"limit_kind,omitempty"`
	// PausedRefusals: messages refused because the mailbox is paused.
	PausedRefusals int `json:"paused_refusals,omitempty"`
	// MismatchRefusals: messages refused because the sender address was not
	// the signed-in mailbox (reject_sender_login_mismatch).
	MismatchRefusals int `json:"mismatch_refusals,omitempty"`
}

// The refusal texts. They are postfwd action texts, so they must not hold a
// semicolon (postfwd's item separator), a slash or a comma (rate action
// fields). Words chosen by the owner's decision of 7 Nov 2026: the ordinary
// limit is temporary (the mail program retries, nothing is lost) and names
// the limit and when sending reopens; the security pause is permanent and
// says what to do.
const (
	OutgoingMailMailboxText      = "This mailbox reached its sending limit of %d recipients per hour. Sending reopens within one hour and your mail program will try again."
	OutgoingMailDomainText       = "This domain reached its sending limit of %d recipients per hour. Sending reopens within one hour and your mail program will try again."
	OutgoingMailSubscriptionText = "This hosting account reached its sending limit of %d recipients per hour. Sending reopens within one hour and your mail program will try again."
	OutgoingMailPausedText       = "Sending from this mailbox was stopped for security. Change the mailbox password to send again."
	// Stable fragments the log reader matches.
	OutgoingMailLimitMarker  = "reached its sending limit of"
	OutgoingMailPausedMarker = "Sending from this mailbox was stopped for security"
)
