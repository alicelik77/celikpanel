package transport

import (
	"errors"
	"net"
	"regexp"
	"strconv"
	"strings"

	"github.com/alicelik/celikpanel/internal/hostname"
)

// Sending outgoing mail through another server (alpha.130, smarthost;
// docs/MAIL-RELAY.md).
//
// Server-wide only: every message this server sends to other servers leaves
// through one relay. Postfix holds it natively: main.cf `relayhost =
// [host]:port` and `default_transport = celikpanel-relay`, a master.cf service
// celikpanel-relay (the smtp client with SASL over mandatory TLS, its own
// options only, so the distribution's smtp settings stay as they were) and a
// root-only password map /etc/postfix/celikpanel-relay-sasl. Postfix keeps
// relaying with the panel stopped or removed (D-022). Turning it off removes
// exactly these lines and files. A relayhost the server owner set is never
// overwritten.
//
// Posta başka bir sunucu üzerinden gider (sunucu geneli). Postfix bunu kendi
// ayarlarıyla tutar; panel durursa da çalışır. Kapatınca yalnız CelikPanel'in
// satırları ve dosyaları kalkar. Sahibin relayhost satırına dokunulmaz.

// Relay states (Agent.MailRelayStatusV1 and the apply answer).
const (
	// Nothing of a relay is set: mail goes straight to other servers.
	MailRelayStateOff = "off"
	// CelikPanel's relay is in place, in the form CelikPanel writes.
	MailRelayStateOn = "on"
	// The server owner set relayhost, default_transport or a sender-dependent
	// relay: CelikPanel changes nothing of it.
	MailRelayStateOwner = "owner"
	// CelikPanel's own service or password map was changed on the server.
	MailRelayStateEdited = "edited"
	// Some of CelikPanel's lines are there and some are not (an earlier run
	// stopped halfway). Saving again or turning off finishes it.
	MailRelayStatePartial = "partial"
)

// Refusal codes (stable; the screen words them).
const (
	MailRelayNoPostfix      = "MAIL_RELAY_NO_POSTFIX"
	MailRelayOwnerConfig    = "MAIL_RELAY_OWNER_CONFIG"
	MailRelayEditedConfig   = "MAIL_RELAY_EDITED_CONFIG"
	MailRelayInvalid        = "MAIL_RELAY_INVALID"
	MailRelaySASLMissing    = "MAIL_RELAY_SASL_MISSING"
	MailRelayPostfixFailed  = "MAIL_RELAY_POSTFIX_FAILED"
	MailRelayBusy           = "MAIL_RELAY_BUSY"
	MailRelayUnreadable     = "MAIL_RELAY_UNREADABLE"
	MailRelayRollbackFailed = "MAIL_RELAY_ROLLBACK_FAILED"
)

// Ports a relay may use. 465 is TLS from the first byte; the others start
// with STARTTLS, which the relay must offer (smtp_tls_security_level=encrypt).
var MailRelayPorts = []int{25, 465, 587, 2525}

// MailRelayApplyRequest is the whole wanted state. Password is sent only to
// write the root-only map; it is never logged and never sent back.
type MailRelayApplyRequest struct {
	ExpectedBuildCommit string `json:"expected_build_commit"`
	Enabled             bool   `json:"enabled"`
	Host                string `json:"host,omitempty"`
	Port                int    `json:"port,omitempty"`
	Username            string `json:"username,omitempty"`
	Password            string `json:"password,omitempty"`
}

// MailRelayStatus is what the server holds now. It never carries a password.
type MailRelayStatus struct {
	Postfix bool   `json:"postfix"`
	State   string `json:"state,omitempty"`
	// Relayhost is main.cf's relayhost as Postfix reads it ("" when unset).
	Relayhost string `json:"relayhost,omitempty"`
	// SASLReady: the Postfix SMTP client can log in with a password (Cyrus
	// SASL client support and its PLAIN/LOGIN modules are installed).
	SASLReady bool   `json:"sasl_ready"`
	Detail    string `json:"detail,omitempty"`
	Error     string `json:"error,omitempty"`
}

// MailRelayApplyResponse answers one apply.
type MailRelayApplyResponse struct {
	Applied bool     `json:"applied"`
	Changed bool     `json:"changed"`
	State   string   `json:"state,omitempty"`
	Code    string   `json:"code,omitempty"`
	Error   string   `json:"error,omitempty"`
	Steps   []string `json:"steps,omitempty"`
}

const mailRelayMaxPassword = 256

var mailRelayUsernameRe = regexp.MustCompile(`^[A-Za-z0-9._@+=-]{1,128}$`)

// MailRelayNexthop validates a relay's host and port, the same on the Panel
// and the Agent, and returns "[host]:port" (the form that turns off MX
// lookups for the relay name). Only a host name or an IPv4 address of another
// server is accepted.
func MailRelayNexthop(host string, port int) (string, error) {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	if ip := net.ParseIP(host); ip != nil {
		if ip.To4() == nil {
			return "", errors.New("the relay address must be a host name or an IPv4 address")
		}
		if ip.IsLoopback() || ip.IsUnspecified() {
			return "", errors.New("the relay must be another server, not this one")
		}
	} else {
		canonical, err := hostname.CanonicalFQDN(host)
		if err != nil {
			return "", errors.New("the relay host name is not valid")
		}
		host = canonical
		if host == "localhost" || strings.HasSuffix(host, ".localhost") {
			return "", errors.New("the relay must be another server, not this one")
		}
	}
	for _, allowed := range MailRelayPorts {
		if allowed == port {
			return "[" + host + "]:" + strconv.Itoa(port), nil
		}
	}
	return "", errors.New("the relay port must be one of 25, 465, 587 or 2525")
}

// ValidateMailRelayUsername: the sign-in name goes into a Postfix map line.
func ValidateMailRelayUsername(value string) error {
	if value == "" {
		return errors.New("the relay user name is required")
	}
	if !mailRelayUsernameRe.MatchString(value) {
		return errors.New("the relay user name may hold letters, digits and . _ @ + = - only")
	}
	return nil
}

// ValidateMailRelayPassword: one line, no control characters.
func ValidateMailRelayPassword(value string) error {
	if value == "" {
		return errors.New("the relay password is required")
	}
	if len(value) > mailRelayMaxPassword {
		return errors.New("the relay password is too long")
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return errors.New("the relay password must not contain control characters or line breaks")
		}
	}
	return nil
}
