package transport

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Out-of-office reply (autoresponder) for one mailbox. The reply itself is
// Dovecot Pigeonhole's Sieve `vacation` action, run by Dovecot's own LMTP
// delivery, so it keeps answering with the Panel stopped or removed (D-022).
// The Panel stores the settings and asks the Agent to write the mailbox's
// Sieve script; the Agent never overwrites a script the mailbox owner made
// (for example with a mail client's filter editor).
//
// Posta kutusu için "ofis dışında" otomatik yanıtı. Yanıtı Dovecot'un kendi
// LMTP teslimindeki Sieve `vacation` eylemi gönderir; Panel durduğunda da
// çalışır. Agent, kutu sahibinin kendi yaptığı Sieve betiğinin üzerine yazmaz.

const (
	MaxAutoresponderSubjectRunes = 200
	MaxAutoresponderMessageBytes = 8 * 1024
	MinAutoresponderIntervalDays = 1
	MaxAutoresponderIntervalDays = 30
	MaxAutoresponderAddresses    = 10
	// AutoresponderDateLayout is the calendar date the Sieve `date` test
	// compares; it is the server's local date.
	AutoresponderDateLayout = "2006-01-02"
)

// MailAutoresponder is the stored and transmitted setting.
type MailAutoresponder struct {
	Enabled      bool     `json:"enabled"`
	Subject      string   `json:"subject"`
	Message      string   `json:"message"`
	StartDate    string   `json:"start_date,omitempty"`
	EndDate      string   `json:"end_date,omitempty"`
	IntervalDays int      `json:"interval_days"`
	Addresses    []string `json:"addresses,omitempty"`
}

// SetMailAutoresponderRequest asks the Agent to bring one mailbox's reply to
// the given setting. Disabled removes CelikPanel's script (never the owner's).
type SetMailAutoresponderRequest struct {
	ExpectedBuildCommit string            `json:"expected_build_commit"`
	Email               string            `json:"email"`
	Autoresponder       MailAutoresponder `json:"autoresponder"`
	// AccountDomains (21 Nov 2026, additive) are the domains of the
	// mailbox's own account. The Agent accepts an additional address only in
	// one of them (or the mailbox's own domain) and only when the server's
	// alias map delivers it to this mailbox (ForeignAutoresponderAddresses).
	AccountDomains []string `json:"account_domains,omitempty"`
}

// MailAutoresponderStateRequest reads one mailbox's Sieve state (read only).
type MailAutoresponderStateRequest struct {
	Email string `json:"email"`
}

// Sieve states of one mailbox, as the Agent found them on disk.
const (
	// No active Sieve script: nothing answers.
	AutoresponderScriptNone = "none"
	// CelikPanel's own script is active and unchanged.
	AutoresponderScriptManaged = "managed"
	// The active script is the mailbox owner's (another script, or
	// CelikPanel's script edited by hand). CelikPanel does not touch it.
	AutoresponderScriptOwner = "owner"
)

// Why the Agent could not apply a reply. Each one has its own guidance on
// the page (D-024): who acts and what they do.
const (
	// The mailbox has its own Sieve script; nothing was changed.
	AutoresponderRefusedOwnerScript = "owner_script"
	// CelikPanel's Dovecot Sieve drop-in was edited by the administrator.
	AutoresponderRefusedOwnerDovecotConf = "owner_dovecot_conf"
	// Postfix's transport_maps names a table that is not CelikPanel's.
	AutoresponderRefusedOwnerTransport = "owner_transport_maps"
	// The Sieve package could not be installed or is not available.
	AutoresponderRefusedSievePackage = "sieve_package"
	// Dovecot refused the configuration; the previous one is in place.
	AutoresponderRefusedDovecotRejected = "dovecot_rejected"
	// Mail is not set up on this server (no Dovecot or Postfix).
	AutoresponderRefusedMailNotInstalled = "mail_not_installed"
	// The mailbox does not exist on this server.
	AutoresponderRefusedNoMailbox = "no_mailbox"
	// Dovecot is stopped, so LMTP could not be asked; nothing was changed.
	AutoresponderRefusedDovecotStopped = "dovecot_stopped"
	// Dovecot's LMTP did not accept the mailbox; the mailbox keeps (or got
	// back) Postfix's own delivery, so its mail never bounces.
	AutoresponderRefusedLMTPRejected = "lmtp_rejected"
	// An additional address is not one of the mailbox's own (21 Nov 2026):
	// nothing was changed.
	AutoresponderRefusedForeignAddress = "foreign_address"
)

// ForeignAutoresponderAddresses returns the additional addresses that are not
// the mailbox's own (21 Nov 2026, alpha.143 review finding 1). The reply's
// From: header is the additional address the incoming message named
// (Pigeonhole's vacation :addresses), and mail handed over on the server is
// signed by its From:, so an address of another account's domain would have
// the reply signed for that domain. An address is the mailbox's own when its
// domain is the mailbox's domain or one of accountDomains, and the alias map
// delivers it to this mailbox: an exact forwarding (followed through further
// forwardings), or, for an address that is neither a mailbox nor an exact
// forwarding, its domain's catch-all. forwardings are the alias map's lines
// (source "@domain" is a catch-all); isMailbox reports a mailbox address. The
// same rule runs in the Panel (from its records) and in the Agent (from the
// server's files).
func ForeignAutoresponderAddresses(mailbox string, addresses, accountDomains []string, forwardings []MailForwarding, isMailbox func(string) bool) []string {
	mailbox = strings.ToLower(strings.TrimSpace(mailbox))
	own := map[string]bool{}
	if at := strings.LastIndexByte(mailbox, '@'); at >= 0 {
		own[mailbox[at+1:]] = true
	}
	for _, domain := range accountDomains {
		own[strings.ToLower(strings.TrimSpace(domain))] = true
	}
	targets := map[string][]string{}
	for _, forwarding := range forwardings {
		source := strings.ToLower(strings.TrimSpace(forwarding.Source))
		for _, destination := range strings.Split(forwarding.Destination, ",") {
			if destination = strings.ToLower(strings.TrimSpace(destination)); destination != "" {
				targets[source] = append(targets[source], destination)
			}
		}
	}
	var deliversHere func(address string, depth int) bool
	deliversHere = func(address string, depth int) bool {
		if address == mailbox {
			return true
		}
		if depth > 8 {
			return false
		}
		destinations, exact := targets[address]
		if !exact {
			if isMailbox != nil && isMailbox(address) {
				return false
			}
			at := strings.LastIndexByte(address, '@')
			if at < 0 {
				return false
			}
			destinations = targets[address[at:]]
		}
		for _, destination := range destinations {
			if deliversHere(destination, depth+1) {
				return true
			}
		}
		return false
	}
	var foreign []string
	for _, raw := range addresses {
		address := strings.ToLower(strings.TrimSpace(raw))
		if address == "" || address == mailbox {
			continue
		}
		at := strings.LastIndexByte(address, '@')
		if at < 0 || !own[address[at+1:]] || !deliversHere(address, 0) {
			foreign = append(foreign, address)
		}
	}
	return foreign
}

// MailAutoresponderResponse is both the Set answer and the State answer.
// Applied is true only when the complete change is in place. In a State
// answer it is true when the mailbox is whole: CelikPanel's script active
// with its LMTP route, or neither of them. Refusal is one
// of the AutoresponderRefused* codes with Detail one bounded line.
type MailAutoresponderResponse struct {
	Applied        bool   `json:"applied"`
	Script         string `json:"script"`
	SieveAvailable bool   `json:"sieve_available"`
	Refusal        string `json:"refusal,omitempty"`
	Detail         string `json:"detail,omitempty"`
	// Warning names a step that did not finish although the reply is in
	// place (for example Dovecot was not reloaded).
	Warning string `json:"warning,omitempty"`
}

// NormalizeMailAutoresponder validates a setting for one mailbox and returns
// its canonical form. It is shared by the Panel (before storing) and the
// Agent (before writing), so both refuse the same input.
func NormalizeMailAutoresponder(in MailAutoresponder, mailbox string) (MailAutoresponder, error) {
	out := MailAutoresponder{Enabled: in.Enabled}
	out.Subject = strings.TrimSpace(in.Subject)
	if out.Subject == "" {
		out.Subject = "Out of office"
	}
	if utf8.RuneCountInString(out.Subject) > MaxAutoresponderSubjectRunes {
		return MailAutoresponder{}, fmt.Errorf("subject is longer than %d characters", MaxAutoresponderSubjectRunes)
	}
	if !autoresponderTextOK(out.Subject, false) {
		return MailAutoresponder{}, errors.New("subject must be one line of printable text")
	}
	out.Message = strings.ReplaceAll(strings.ReplaceAll(in.Message, "\r\n", "\n"), "\r", "\n")
	out.Message = strings.TrimRight(out.Message, " \t\n")
	if in.Enabled && strings.TrimSpace(out.Message) == "" {
		return MailAutoresponder{}, errors.New("message is required")
	}
	if len(out.Message) > MaxAutoresponderMessageBytes {
		return MailAutoresponder{}, fmt.Errorf("message is longer than %d bytes", MaxAutoresponderMessageBytes)
	}
	if !autoresponderTextOK(out.Message, true) {
		return MailAutoresponder{}, errors.New("message contains characters that cannot be sent")
	}
	out.IntervalDays = in.IntervalDays
	if out.IntervalDays == 0 {
		out.IntervalDays = 1
	}
	if out.IntervalDays < MinAutoresponderIntervalDays || out.IntervalDays > MaxAutoresponderIntervalDays {
		return MailAutoresponder{}, fmt.Errorf("reply interval must be between %d and %d days", MinAutoresponderIntervalDays, MaxAutoresponderIntervalDays)
	}
	var start, end time.Time
	var err error
	out.StartDate = strings.TrimSpace(in.StartDate)
	if out.StartDate != "" {
		if start, err = time.Parse(AutoresponderDateLayout, out.StartDate); err != nil {
			return MailAutoresponder{}, errors.New("start date must be YYYY-MM-DD")
		}
	}
	out.EndDate = strings.TrimSpace(in.EndDate)
	if out.EndDate != "" {
		if end, err = time.Parse(AutoresponderDateLayout, out.EndDate); err != nil {
			return MailAutoresponder{}, errors.New("end date must be YYYY-MM-DD")
		}
	}
	if out.StartDate != "" && out.EndDate != "" && end.Before(start) {
		return MailAutoresponder{}, errors.New("end date is before the start date")
	}
	if len(in.Addresses) > MaxAutoresponderAddresses {
		return MailAutoresponder{}, fmt.Errorf("at most %d additional addresses", MaxAutoresponderAddresses)
	}
	seen := map[string]bool{}
	if mailbox != "" {
		seen[strings.ToLower(mailbox)] = true
	}
	for _, raw := range in.Addresses {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		address, err := CanonicalMailAddress(raw)
		if err != nil {
			return MailAutoresponder{}, fmt.Errorf("additional address %q: %v", strings.TrimSpace(raw), err)
		}
		if seen[address] {
			continue
		}
		seen[address] = true
		out.Addresses = append(out.Addresses, address)
	}
	return out, nil
}

func autoresponderTextOK(value string, multiline bool) bool {
	if !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		switch {
		case r == '\n' && multiline, r == '\t':
		case unicode.IsControl(r), r == 0x2028, r == 0x2029, r == 0xFEFF:
			return false
		}
	}
	return true
}
