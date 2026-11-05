package transport

// Passwordless mail (owner decision of 4 Nov 2026: "parolasız posta kesinlikle
// yasak"). Two unauthenticated ways out exist on a stock Postfix: SMTP from
// the server itself to port 25 (loopback is in mynetworks, and
// permit_mynetworks lets it relay), and the sendmail command that PHP mail(),
// cron and scripts run (postdrop → pickup). The Agent reads Postfix's own log
// lines for the last 30 days, read-only, and answers who used either way.
//
// Parolasız posta: Postfix'in kendi günlüğünden son 30 günde sunucudan
// parolasız gönderenler, salt okunur.

// PasswordlessMailWindowDays is how far back the log is read.
const PasswordlessMailWindowDays = 30

// PasswordlessMailReportResponse is Agent.PasswordlessMailReportV1's answer.
type PasswordlessMailReportResponse struct {
	// Read is true when Postfix's log could be read. A false Read means the
	// answer is unknown, never "nobody sent".
	// Read yanlışsa yanıt bilinmiyor demektir, "kimse göndermedi" değil.
	Read bool `json:"read"`
	// Source names the log that was read ("journal").
	Source string `json:"source,omitempty"`
	// CoveredSince is the time of the oldest Postfix line found (RFC 3339). A
	// log kept for less than the window covers less; the screen says so.
	CoveredSince string `json:"covered_since,omitempty"`
	// Incomplete: the read stopped early (time or size bound); counts are a
	// lower bound.
	Incomplete bool `json:"incomplete,omitempty"`
	// LocalSenders: mail handed to the sendmail command, per Unix account.
	LocalSenders []PasswordlessLocalSender `json:"local_senders"`
	// LoopbackSenders: mail sent over SMTP from this server to port 25
	// without a password and relayed to another server, per envelope sender.
	LoopbackSenders []PasswordlessLoopbackSender `json:"loopback_senders"`
	// Enforcement is what the server allows now (phase 2, alpha.125).
	Enforcement *PasswordlessMailEnforcement `json:"enforcement,omitempty"`
	// Error: why the log could not be read (Read false).
	Error string `json:"error,omitempty"`
}

// PasswordlessLocalSender is one Unix account that ran the sendmail command.
type PasswordlessLocalSender struct {
	UID  int    `json:"uid"`
	User string `json:"user,omitempty"`
	// System: root or the mail system's own account (vmail: Sieve replies).
	// These keep sending after the change.
	System bool `json:"system,omitempty"`
	// Messages accepted from this account; Refused: attempts Postfix refused
	// (after the change).
	Messages int    `json:"messages"`
	Refused  int    `json:"refused,omitempty"`
	Last     string `json:"last,omitempty"`
	// SenderDomains: the envelope sender domains seen (at most a few).
	SenderDomains []string `json:"sender_domains,omitempty"`
}

// PasswordlessLoopbackSender is one envelope sender that sent over SMTP from
// this server without a password.
type PasswordlessLoopbackSender struct {
	Sender   string `json:"sender"`
	Messages int    `json:"messages"`
	Refused  int    `json:"refused,omitempty"`
	Last     string `json:"last,omitempty"`
}

// PasswordlessMailEnforcement is the state of the two Postfix settings that
// close passwordless mail (phase 2, alpha.125).
// İki Postfix ayarının durumu (2. aşama).
type PasswordlessMailEnforcement struct {
	// State: "closed" (both settings closed), "open" (not changed yet: the
	// panel changes them on its next start), "owner" (the server owner set one
	// of them otherwise; kept), "failed" (the change was tried and rolled
	// back; Detail says why), "no_postfix".
	State string `json:"state"`
	// SubmitUsers / RelayRestrictions: the values in effect now.
	SubmitUsers       string `json:"submit_users,omitempty"`
	RelayRestrictions string `json:"relay_restrictions,omitempty"`
	// Expected*: CelikPanel's form, shown beside an owner's difference.
	ExpectedSubmitUsers       string `json:"expected_submit_users,omitempty"`
	ExpectedRelayRestrictions string `json:"expected_relay_restrictions,omitempty"`
	// ChangedAt: when CelikPanel closed it (RFC 3339), from its record.
	ChangedAt string `json:"changed_at,omitempty"`
	Detail    string `json:"detail,omitempty"`
}

// SendAccountMailRequest hands one of the panel's own account messages
// (password reset, invitation, notices) to Postfix through the sendmail
// command, run by the Agent as root (phase 2): the panel no longer relays
// through port 25 without a password. Message is the whole RFC 5322 text.
type SendAccountMailRequest struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Message []byte `json:"message"`
}

// SendAccountMailResponse: Sent is true once Postfix accepted the message
// ("handed over", not "delivered").
type SendAccountMailResponse struct {
	Sent  bool   `json:"sent"`
	Error string `json:"error,omitempty"`
}
