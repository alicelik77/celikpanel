package transport

// Per-domain mail service options (alpha.128, gap G25): "incoming off, send
// only" and "refuse unknown addresses with our own message". Both are lines
// of one native Postfix recipient access map that the panel renders whole;
// mailboxes and their mail are never touched.
//
// Alan adı başına posta seçenekleri: "gelen kapalı, yalnız gönderim" ve
// "bilinmeyen adresi kendi iletimizle reddet". Tek yerel Postfix haritası.

// MailDomainOptionsEntry is one line: Key is a domain or a full address,
// Action is "DUNNO" (a known address of a domain with its own refusal text)
// or a permanent refusal "550 5.x.x <text>". Nothing else is accepted, so the
// map can never permit or relay mail.
type MailDomainOptionsEntry struct {
	Key    string `json:"key"`
	Action string `json:"action"`
}

// SetMailDomainOptionsRequest is the whole map (an empty list removes the
// map's restriction from main.cf).
type SetMailDomainOptionsRequest struct {
	ExpectedBuildCommit string                   `json:"expected_build_commit"`
	Entries             []MailDomainOptionsEntry `json:"entries"`
}

// SetMailDomainOptionsResponse: Applied when the map is in place and Postfix
// took it. Owner: the map file was changed on the server and is kept.
type SetMailDomainOptionsResponse struct {
	Applied bool   `json:"applied"`
	Changed bool   `json:"changed,omitempty"`
	Owner   bool   `json:"owner,omitempty"`
	Error   string `json:"error,omitempty"`
}

// MailDomainOptionsMaxReply bounds a refusal text.
const MailDomainOptionsMaxReply = 200
