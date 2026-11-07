package transport

// Mailbox work (alpha.128, gap G20): emptying a mailbox removes every message
// in every folder through Dovecot itself (doveadm expunge), keeps the mailbox,
// its password, folders, aliases and settings, and recalculates its quota.
//
// Posta kutusunu boşaltma: Dovecot'un kendisiyle bütün klasörlerdeki iletiler
// silinir; kutu, parolası, klasörleri ve ayarları kalır.

// EmptyMailboxRequest is Agent.EmptyMailboxV1's request.
type EmptyMailboxRequest struct {
	ExpectedBuildCommit string `json:"expected_build_commit"`
	Email               string `json:"email"`
}

// EmptyMailboxResponse: Applied when Dovecot removed the messages. Messages
// is how many there were (-1 when Dovecot could not count them first).
// QuotaRecalculated is false when Dovecot keeps no quota for the mailbox.
type EmptyMailboxResponse struct {
	Applied           bool   `json:"applied"`
	Messages          int    `json:"messages"`
	QuotaRecalculated bool   `json:"quota_recalculated,omitempty"`
	Error             string `json:"error,omitempty"`
	// Unknown: the mailbox is not known to Dovecot (nothing was removed).
	Unknown bool `json:"unknown,omitempty"`
}
