package transport

// Mail delivery lookup (alpha.128, gap G9): "did my mail arrive?" answered
// from Postfix's own log of the last days, scoped to one domain. Only
// messages from or to an address of that domain are returned, and of an
// incoming message only its recipients in that domain; no other domain's
// mail ever leaves the Agent.
//
// Posta teslim sorgusu: Postfix'in kendi günlüğünden, tek alan adıyla
// sınırlı. Yalnız o alan adının adreslerinden ya da adreslerine giden
// iletiler döner.

const (
	MailDeliveryMaxDays    = 7
	MailDeliveryMaxEntries = 200
)

// MailDeliveryLookupRequest: Domain is required; Address (optional) narrows
// to messages from or to that address.
type MailDeliveryLookupRequest struct {
	Domain  string `json:"domain"`
	Address string `json:"address,omitempty"`
	Days    int    `json:"days"`
}

// MailDeliveryEntry is one recipient of one message, newest first.
// Status: sent, deferred (waiting, Postfix retries), bounced, expired,
// refused (never accepted), queued (accepted, no outcome yet).
type MailDeliveryEntry struct {
	At     string `json:"at"`
	From   string `json:"from"`
	To     string `json:"to"`
	Status string `json:"status"`
	// Relay: where Postfix handed it (a server name, or "mailbox" for local
	// delivery). Reason: the receiving server's or Postfix's own words.
	Relay  string `json:"relay,omitempty"`
	Reason string `json:"reason,omitempty"`
	// Direction: out (sent by this domain) or in (received by it).
	Direction string `json:"direction"`
}

// MailDeliveryLookupResponse: Read false means the log could not be read
// (unknown, never "nothing found").
type MailDeliveryLookupResponse struct {
	Read         bool                `json:"read"`
	Incomplete   bool                `json:"incomplete,omitempty"`
	CoveredSince string              `json:"covered_since,omitempty"`
	Entries      []MailDeliveryEntry `json:"entries"`
	Error        string              `json:"error,omitempty"`
}
