package transport

// Plan limits made real (alpha.118, D-044): website traffic, subscription
// disk usage and mailbox quotas. Additive contracts; an Agent that predates
// them has no such method and the Panel reports the measurement as unknown.
//
// Plan sınırları gerçek olur: web sitesi trafiği, abonelik disk kullanımı ve
// posta kutusu kotası. Eski Agent bu yöntemleri bilmez; Panel ölçümü
// "bilinmiyor" gösterir.

// TrafficDomain names one website whose traffic the Panel wants measured.
type TrafficDomain struct {
	DomainID int    `json:"domain_id"`
	Domain   string `json:"domain"`
	// Blocked: the site shows a CelikPanel page (suspended, maintenance or
	// traffic limit), so its log is deliberately not written.
	Blocked bool `json:"blocked,omitempty"`
}

// TrafficCollectRequest asks for every batch after AfterSeq. AfterSeq
// confirms that the Panel committed every batch up to it; the Agent then
// drops those. Epoch is the Agent store the Panel's AfterSeq belongs to; a
// different store (Epoch mismatch) is answered from its own sequence start.
type TrafficCollectRequest struct {
	Epoch    string          `json:"epoch"`
	AfterSeq int64           `json:"after_seq"`
	Domains  []TrafficDomain `json:"domains"`
}

// TrafficDayBytes is one website's bytes on one day (server local date).
type TrafficDayBytes struct {
	DomainID int    `json:"domain_id"`
	Day      string `json:"day"`
	BytesOut int64  `json:"bytes_out"`
	BytesIn  int64  `json:"bytes_in"`
	Requests int64  `json:"requests"`
}

// TrafficBatch is one sealed, numbered set of totals. A batch is never
// changed after it is sealed; it is dropped only after the Panel confirmed
// it (AfterSeq >= Seq).
type TrafficBatch struct {
	Seq   int64             `json:"seq"`
	Items []TrafficDayBytes `json:"items"`
}

// Traffic source states, per website.
const (
	TrafficSourceMeasuring  = "measuring"
	TrafficSourceNotCounted = "not_counted"
	TrafficSourceUnknown    = "unknown"
)

// Reason codes of an unknown traffic source.
const (
	// The traffic log format file in nginx's conf.d is not CelikPanel's
	// unchanged text (the owner changed or removed it).
	TrafficReasonFormatKept = "format_file_kept"
	// The site's traffic log setting is not in CelikPanel's directory of the
	// site (not yet written, or the owner changed it).
	TrafficReasonIncludeMissing = "include_missing"
	// The site's vhost file is the owner's and does not read CelikPanel's
	// directory of the site.
	TrafficReasonVhostKept = "vhost_kept"
	// The log could not be read.
	TrafficReasonLogUnreadable = "log_unreadable"
	// The site's log was rotated away before it was read to the end; some
	// bytes of that period are not counted.
	TrafficReasonRotationGap = "rotation_gap"
)

// TrafficSourceState is whether one website is being measured.
type TrafficSourceState struct {
	DomainID int    `json:"domain_id"`
	State    string `json:"state"`
	Reason   string `json:"reason,omitempty"`
}

// TrafficCollectResponse is the batches the Panel has not confirmed, the
// state of every requested website and the logrotate file's state.
type TrafficCollectResponse struct {
	Epoch   string               `json:"epoch"`
	Batches []TrafficBatch       `json:"batches"`
	Sources []TrafficSourceState `json:"sources"`
	// Logrotate: "written", "unchanged", "kept" (the owner's file),
	// "missing_tool" (no logrotate on this server) or "failed".
	Logrotate string `json:"logrotate,omitempty"`
	Error     string `json:"error,omitempty"`
}

// SubscriptionDiskDatabase is one database of a subscription.
type SubscriptionDiskDatabase struct {
	// Engine: "mysql" (MariaDB/MySQL) or "postgresql".
	Engine string `json:"engine"`
	Name   string `json:"name"`
	Port   int    `json:"port,omitempty"`
}

// SubscriptionDiskUsageRequest asks for one subscription's real disk use:
// its hosting folder, the mail folders of its domains and its databases.
type SubscriptionDiskUsageRequest struct {
	SubscriptionID int                        `json:"subscription_id"`
	MailDomains    []string                   `json:"mail_domains"`
	Databases      []SubscriptionDiskDatabase `json:"databases"`
}

// SubscriptionDiskUsageResponse: bytes per part. A part that could not be
// measured is named in Unknown and its number is 0 (not "no usage").
type SubscriptionDiskUsageResponse struct {
	WebBytes  int64    `json:"web_bytes"`
	MailBytes int64    `json:"mail_bytes"`
	DBBytes   int64    `json:"db_bytes"`
	Method    string   `json:"method"`
	Unknown   []string `json:"unknown,omitempty"`
	// FreePercent is the free space of the file system that holds the
	// hosting folders (0-100), -1 when it could not be read.
	FreePercent int `json:"free_percent"`
	// ProjectQuota: the file system is mounted with project quotas. Detect
	// only; CelikPanel never changes mount options (D-044).
	ProjectQuota bool   `json:"project_quota,omitempty"`
	Error        string `json:"error,omitempty"`
}

// MailQuotaReconcileRequest makes Dovecot enforce mailbox sizes with its own
// quota plugin (native configuration; works without CelikPanel).
type MailQuotaReconcileRequest struct {
	ExpectedBuildCommit string `json:"expected_build_commit"`
}

// Mail quota states.
const (
	MailQuotaEnforced   = "enforced"
	MailQuotaKept       = "kept"        // the drop-in is the owner's; left alone
	MailQuotaNoMail     = "no_mail"     // Dovecot is not installed or not configured by CelikPanel
	MailQuotaRefused    = "refused"     // doveconf refused the change; the previous files were put back
	MailQuotaUnknown    = "unknown"     // the dialect or the files could not be read
	MailQuotaNotApplied = "not_applied" // written, but Dovecot could not be reloaded
)

// MailQuotaReconcileResponse says what was found and done.
type MailQuotaReconcileResponse struct {
	State   string `json:"state"`
	Dialect string `json:"dialect,omitempty"` // "2.3" or "2.4"
	Changed bool   `json:"changed"`
	Detail  string `json:"detail,omitempty"`
	Error   string `json:"error,omitempty"`
}
