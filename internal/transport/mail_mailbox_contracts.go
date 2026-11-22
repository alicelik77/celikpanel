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

// MailAccountExistsOnServer is the Agent's exact answer when a mailbox is
// asked for that the server's mail files already hold (alpha.144: the Panel
// tells the person, never an internal error).
// Sunucunun posta dosyalarında zaten olan bir kutu istendiğinde Agent'ın
// tam yanıtı; Panel bunu kişiye söyler, iç hata değildir.
const MailAccountExistsOnServer = "mail account already exists"

// MailboxOnServerRequest is Agent.MailboxOnServerV1's request: what the
// server's mail files hold for one address (read only; alpha.144, adopting a
// mailbox found on the server without a record in the Panel).
type MailboxOnServerRequest struct {
	ExpectedBuildCommit string `json:"expected_build_commit"`
	Email               string `json:"email"`
}

// MailboxOnServerResponse: InUsers and InMap say whether the address has a
// line in Dovecot's users file and in Postfix's mailbox map. OwnFolder: the
// map delivers to the folder CelikPanel gives this address
// (<domain>/<local>/) and the users line names no other home, account or
// shell. QuotaMB is the size limit the users line sets in the form
// CelikPanel writes (0 when it sets none in that form).
type MailboxOnServerResponse struct {
	InUsers   bool `json:"in_users"`
	InMap     bool `json:"in_map"`
	OwnFolder bool `json:"own_folder"`
	QuotaMB   int  `json:"quota_mb"`
}
