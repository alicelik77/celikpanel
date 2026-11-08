package services

import (
	"fmt"
	"strings"

	"github.com/alicelik/celikpanel/internal/hostname"
)

// Mail programs that set themselves up (alpha.130; docs/MAIL-AUTOCONFIG.md).
//
// Thunderbird asks https://autoconfig.<domain>/mail/config-v1.1.xml and
// https://<domain>/.well-known/autoconfig/mail/config-v1.1.xml; Outlook POSTs
// (or GETs) https://autodiscover.<domain>/autodiscover/autodiscover.xml. The
// answers are fixed per domain, so the site's own nginx file answers them
// with `return 200` and no PHP: they keep working with the panel stopped or
// removed (D-022). They hold the mail server's name, its ports and TLS, and
// never a password. The email address an Outlook request carries is never
// echoed: without LoginName Outlook uses the address the person typed, and
// Thunderbird fills %EMAILADDRESS% itself.
//
// Posta programları kendini ayarlar: yanıtlar alan adı başına sabittir;
// sitenin nginx dosyası PHP olmadan verir. Parola yoktur; istekteki adres
// geri yazılmaz.

// MailAutoconfig is the part of a site's render that answers mail programs.
// The caller sets MailHost and TLS; Render derives the rest.
type MailAutoconfig struct {
	// MailHost is the server's mail name: IMAP 993 (TLS), submission 465
	// (TLS) and 587 (STARTTLS) answer there with its certificate.
	MailHost string
	// TLS: the site's certificate covers both names, so they are also
	// served over HTTPS with it.
	TLS bool

	Names           []string
	ThunderbirdXML  string
	AutodiscoverXML string
}

// MailAutoconfigNames are the two names a domain's mail programs ask.
func MailAutoconfigNames(domain string) []string {
	domain = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(domain), "."))
	return []string{"autoconfig." + domain, "autodiscover." + domain}
}

// prepareMailAutoconfig validates the caller's part and derives the answers.
func prepareMailAutoconfig(data *VhostData) error {
	if data.MailAutoconfig == nil {
		return nil
	}
	mail := *data.MailAutoconfig
	host := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(mail.MailHost), "."))
	if err := hostname.Validate(host); err != nil || !strings.Contains(host, ".") {
		return fmt.Errorf("the mail server name for mail set-up is not a valid host name")
	}
	mail.MailHost = host
	mail.Names = MailAutoconfigNames(data.Domain)
	for _, name := range mail.Names {
		if err := hostname.Validate(name); err != nil {
			return fmt.Errorf("the mail set-up name %s is not a valid host name", name)
		}
		for _, taken := range append(append([]string{}, data.ServerNames...), data.ACMEChallengeNames...) {
			if strings.EqualFold(taken, name) {
				return fmt.Errorf("%s is already a name of the site; mail set-up cannot answer there", name)
			}
		}
	}
	if mail.TLS && (data.SSLType == "none" || data.SSLCert == "" || data.SSLKey == "") {
		mail.TLS = false
	}
	mail.ThunderbirdXML = thunderbirdAutoconfigXML(data.Domain, host)
	mail.AutodiscoverXML = outlookAutodiscoverXML(host)
	for _, text := range []string{mail.ThunderbirdXML, mail.AutodiscoverXML} {
		// The answers are written inside nginx's single quotes: nothing in
		// them may end the string or name an nginx variable.
		if strings.ContainsAny(text, "'\\$\r\n") {
			return fmt.Errorf("the mail set-up answer cannot be written into nginx")
		}
	}
	data.MailAutoconfig = &mail
	return nil
}

// thunderbirdAutoconfigXML is Mozilla's autoconfig format (clientConfig
// 1.1): IMAP over TLS, sending over TLS (465) or STARTTLS (587); the user
// name is the whole address, which Thunderbird fills in.
func thunderbirdAutoconfigXML(domain, host string) string {
	server := func(kind, tag string, port int, socket string) string {
		return fmt.Sprintf(`<%s type="%s"><hostname>%s</hostname><port>%d</port><socketType>%s</socketType><authentication>password-cleartext</authentication><username>%%EMAILADDRESS%%</username></%s>`,
			tag, kind, host, port, socket, tag)
	}
	return `<?xml version="1.0" encoding="UTF-8"?>` +
		`<clientConfig version="1.1"><emailProvider id="` + domain + `"><domain>` + domain + `</domain>` +
		`<displayName>` + domain + `</displayName><displayShortName>` + domain + `</displayShortName>` +
		server("imap", "incomingServer", 993, "SSL") +
		server("smtp", "outgoingServer", 465, "SSL") +
		server("smtp", "outgoingServer", 587, "STARTTLS") +
		`</emailProvider></clientConfig>`
}

// outlookAutodiscoverXML is the Outlook "POX" answer: IMAP and SMTP over
// TLS, sign-in required. No LoginName: Outlook then uses the address the
// person typed, so nothing of the request is written back.
func outlookAutodiscoverXML(host string) string {
	protocol := func(kind string, port int, extra string) string {
		return fmt.Sprintf(`<Protocol><Type>%s</Type><Server>%s</Server><Port>%d</Port><DomainRequired>off</DomainRequired><SPA>off</SPA><SSL>on</SSL><AuthRequired>on</AuthRequired>%s</Protocol>`,
			kind, host, port, extra)
	}
	return `<?xml version="1.0" encoding="utf-8"?>` +
		`<Autodiscover xmlns="http://schemas.microsoft.com/exchange/autodiscover/responseschema/2006">` +
		`<Response xmlns="http://schemas.microsoft.com/exchange/autodiscover/outlook/responseschema/2006a">` +
		`<Account><AccountType>email</AccountType><Action>settings</Action>` +
		protocol("IMAP", 993, "") +
		protocol("SMTP", 465, "<UsePOPAuth>off</UsePOPAuth><SMTPLast>off</SMTPLast>") +
		`</Account></Response></Autodiscover>`
}
