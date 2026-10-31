package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/hostname"
	"github.com/alicelik/celikpanel/internal/mutationpayload"
)

// Account mail (v0.1.0-alpha.114, P6): the messages the panel itself sends to
// a person about their account - a password reset link, an invitation, an
// address confirmation and a notice that the address changed.
//
// The panel has no mail system of its own and adds no outside dependency. It
// hands each message to the server's own mail system over SMTP on the
// loopback address (127.0.0.1:25), which Postfix accepts from the server
// itself (permit_mynetworks). The panel process runs with NoNewPrivileges,
// so the setgid sendmail binary is not an option. Nothing is configured,
// started or changed on the mail system: when no mail system answers there,
// or the panel has no public address for its links, or no sender domain is
// known, the feature says so (accountMailStatus.Reason) and the screens give
// the person who can act the other way: an administrator sets a temporary
// password, or the inviter copies the invitation link.
//
// A message accepted by the local mail system is "handed over", not
// "delivered": whether it reached the inbox is not known to the panel.
//
// Hesap postası: panel kendi posta sistemine sahip değildir; iletiyi
// sunucunun kendi posta sistemine 127.0.0.1:25 üzerinden SMTP ile verir.
// Posta sisteminde hiçbir şey yapılandırılmaz. Posta sistemi yanıt vermezse,
// bağlantılar için panel adresi yoksa ya da gönderen alan adı bilinmiyorsa
// özellik bunu söyler ve ekran işi yapabilecek kişiye öteki yolu gösterir.
// Yerel posta sisteminin kabul ettiği ileti "teslim edildi" değil
// "posta sistemine verildi" sayılır.

const (
	accountMailReasonNoPanelAddress = "no_panel_address"
	accountMailReasonNoSender       = "no_sender"
	accountMailReasonNoMailServer   = "no_mail_server"

	accountMailStatusTTL = time.Minute
)

// accountMailRelayAddress is the only place a message is handed to.
var accountMailRelayAddress = "127.0.0.1:25"

// Replaced by tests only. Production reads the active panel certificate's
// name; the port is the configured panel port (CELIKPANEL_LISTEN), never a
// constant.
var (
	accountLinkPanelHost = panelCertificateHostname
	accountMailProbe     = probeLocalSMTP
	accountMailSend      = sendLocalSMTP
	accountMailStatusNow = time.Now
)

// accountMailStatus is what any screen may know: whether account mail can be
// sent from this server and, when not, why. It names no address.
type accountMailStatus struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

// accountMailSetup is the server-side whole: where links point and who sends.
type accountMailSetup struct {
	accountMailStatus
	baseURL string
	from    string
	domain  string
	host    string
}

var accountMailCache struct {
	mu      sync.Mutex
	checked time.Time
	probeOK bool
}

// resetAccountMailCache forgets the last probe (tests, and after a failed send).
func resetAccountMailCache() {
	accountMailCache.mu.Lock()
	accountMailCache.checked = time.Time{}
	accountMailCache.mu.Unlock()
}

// accountMailSetup resolves the link address and the sender, then whether a
// mail system answers on the loopback address (cached for a minute so a
// public status read never becomes a probe per request).
func (p *Panel) accountMailSetup(ctx context.Context) accountMailSetup {
	setup := accountMailSetup{}
	host := strings.TrimSpace(accountLinkPanelHost())
	if canonical, err := hostname.CanonicalFQDN(host); err == nil && host != "" {
		setup.host = canonical
		setup.baseURL = "https://" + net.JoinHostPort(canonical, strconv.Itoa(panelPort()))
	}
	setup.domain = p.accountMailSenderDomain(ctx, setup.host)
	if setup.domain != "" {
		setup.from = "no-reply@" + setup.domain
		if _, err := mutationpayload.CanonicalContactEmail(setup.from); err != nil {
			setup.from, setup.domain = "", ""
		}
	}
	switch {
	case setup.baseURL == "":
		setup.Reason = accountMailReasonNoPanelAddress
		return setup
	case setup.from == "":
		setup.Reason = accountMailReasonNoSender
		return setup
	}
	accountMailCache.mu.Lock()
	defer accountMailCache.mu.Unlock()
	now := accountMailStatusNow()
	if accountMailCache.checked.IsZero() || now.Sub(accountMailCache.checked) >= accountMailStatusTTL {
		probeCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
		accountMailCache.probeOK = accountMailProbe(probeCtx) == nil
		cancel()
		accountMailCache.checked = now
	}
	if !accountMailCache.probeOK {
		setup.Reason = accountMailReasonNoMailServer
		return setup
	}
	setup.Available = true
	return setup
}

// accountMailSenderDomain: the owner's main domain from server setup (its SPF
// and DKIM name this server), else the saved mail host name, else the panel's
// own name. No address is invented when none of them is known.
func (p *Panel) accountMailSenderDomain(ctx context.Context, panelHost string) string {
	candidates := []string{}
	if p != nil && p.db != nil {
		if state, err := p.loadServerSetup(ctx); err == nil {
			candidates = append(candidates, state.Draft.MainDomain)
		}
		candidates = append(candidates, p.setting(ctx, settingMailHostname))
	}
	candidates = append(candidates, panelHost)
	for _, candidate := range candidates {
		if canonical, err := hostname.CanonicalFQDN(strings.TrimSpace(candidate)); err == nil && candidate != "" {
			return canonical
		}
	}
	return ""
}

// probeLocalSMTP reads the greeting and answers one EHLO; it sends nothing.
func probeLocalSMTP(ctx context.Context) error {
	client, conn, err := dialLocalSMTP(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	defer client.Close()
	if err := client.Hello("localhost"); err != nil {
		return err
	}
	return client.Quit()
}

func dialLocalSMTP(ctx context.Context) (*smtp.Client, net.Conn, error) {
	host, _, err := net.SplitHostPort(accountMailRelayAddress)
	if err != nil {
		return nil, nil, err
	}
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		return nil, nil, errors.New("account mail is handed only to a loopback mail system")
	}
	conn, err := (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "tcp", accountMailRelayAddress)
	if err != nil {
		return nil, nil, err
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(30 * time.Second)
	}
	if err := conn.SetDeadline(deadline); err != nil {
		conn.Close()
		return nil, nil, err
	}
	client, err := smtp.NewClient(conn, "localhost")
	if err != nil {
		conn.Close()
		return nil, nil, err
	}
	return client, conn, nil
}

// sendLocalSMTP hands one message to the loopback mail system.
func sendLocalSMTP(ctx context.Context, from, to string, message []byte) error {
	client, conn, err := dialLocalSMTP(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	defer client.Close()
	if err := client.Hello("localhost"); err != nil {
		return err
	}
	if err := client.Mail(from); err != nil {
		return err
	}
	if err := client.Rcpt(to); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := writer.Write(message); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}

// accountMailKind names one of the four messages.
type accountMailKind string

const (
	accountMailReset        accountMailKind = "password_reset"
	accountMailInvitation   accountMailKind = "invitation"
	accountMailVerify       accountMailKind = "email_verify"
	accountMailEmailChanged accountMailKind = "email_changed"
)

type accountMailContent struct {
	kind     accountMailKind
	lang     string
	username string
	inviter  string
	link     string
}

// accountMailLang keeps only the two languages the panel ships.
func accountMailLang(raw string) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(raw)), "tr") {
		return "tr"
	}
	return "en"
}

// accountMailText returns the subject and the body. Every value placed in it
// is checked by the caller: user names pass auth.ValidateUsername, the link is
// built here from the panel's own address and a URL-safe token.
func accountMailText(content accountMailContent, host string) (string, string) {
	tr := content.lang == "tr"
	user, inviter, link := content.username, content.inviter, content.link
	switch content.kind {
	case accountMailReset:
		if tr {
			return "CelikPanel parolanızı sıfırlayın", strings.Join([]string{
				host + " üzerindeki " + user + " CelikPanel hesabı için parola sıfırlama istendi.",
				"",
				"Yeni parolanızı belirlemek için bu bağlantıyı 30 dakika içinde açın:",
				link,
				"",
				"Bağlantı yalnız bir kez çalışır. Bunu siz istemediyseniz bu iletiyi yok sayın; parolanız değişmez.",
				"Bağlantının süresi dolduysa giriş sayfasından yenisini isteyin.",
			}, "\n")
		}
		return "Reset your CelikPanel password", strings.Join([]string{
			"Someone asked to reset the password of the CelikPanel account " + user + " on " + host + ".",
			"",
			"To choose a new password, open this link within 30 minutes:",
			link,
			"",
			"The link works once. If you did not ask for this, ignore this message; your password stays the same.",
			"If the link has expired, ask for a new one on the sign-in page.",
		}, "\n")
	case accountMailInvitation:
		if tr {
			return "CelikPanel hesabınız hazır", strings.Join([]string{
				inviter + ", " + host + " üzerinde sizin için bir CelikPanel hesabı açtı.",
				"Kullanıcı adınız: " + user,
				"",
				"Parolanızı belirleyip başlamak için bu bağlantıyı 7 gün içinde açın:",
				link,
				"",
				"Bağlantı yalnız bir kez çalışır. Parolanızı yalnız siz belirlersiniz; başka kimse bilmez.",
				"Bu iletiyi beklemiyorsanız yok sayın.",
			}, "\n")
		}
		return "Your CelikPanel account is ready", strings.Join([]string{
			inviter + " created a CelikPanel account for you on " + host + ".",
			"Your user name: " + user,
			"",
			"To choose your password and start, open this link within 7 days:",
			link,
			"",
			"The link works once. Only you choose your password; nobody else knows it.",
			"If you did not expect this message, ignore it.",
		}, "\n")
	case accountMailVerify:
		if tr {
			return "CelikPanel e-posta adresinizi onaylayın", strings.Join([]string{
				"Bu adresin " + host + " üzerindeki " + user + " CelikPanel hesabına ait olduğunu onaylayın.",
				"",
				"Onaylamak için bu bağlantıyı 24 saat içinde açın:",
				link,
				"",
				"Onaydan sonra parolanızı unutursanız e-postayla sıfırlayabilirsiniz.",
				"Bunu siz istemediyseniz bu iletiyi yok sayın.",
			}, "\n")
		}
		return "Confirm your email address for CelikPanel", strings.Join([]string{
			"Confirm that this address belongs to the CelikPanel account " + user + " on " + host + ".",
			"",
			"To confirm it, open this link within 24 hours:",
			link,
			"",
			"Once it is confirmed, you can reset your password by email if you forget it.",
			"If you did not ask for this, ignore this message.",
		}, "\n")
	default: // accountMailEmailChanged
		if tr {
			return "CelikPanel e-posta adresiniz değişti", strings.Join([]string{
				host + " üzerindeki " + user + " CelikPanel hesabının e-posta adresi değiştirildi.",
				"",
				"Bunu siz yaptıysanız bir şey yapmanız gerekmez.",
				"Siz yapmadıysanız hemen oturum açıp parolanızı değiştirin ya da hizmet sağlayıcınızdan yardım isteyin.",
			}, "\n")
		}
		return "Your CelikPanel email address was changed", strings.Join([]string{
			"The email address of the CelikPanel account " + user + " on " + host + " was changed.",
			"",
			"If you did this, there is nothing to do.",
			"If you did not, sign in now and change your password, or ask your provider for help.",
		}, "\n")
	}
}

// buildAccountMail writes the complete message. No header carries a value
// that could hold a line break: the recipient passed the address validator,
// the subject is encoded and fixed text.
func buildAccountMail(setup accountMailSetup, to string, content accountMailContent, now time.Time) ([]byte, error) {
	if strings.ContainsAny(to, "\r\n") || strings.ContainsAny(setup.from, "\r\n") {
		return nil, errors.New("invalid mail header value")
	}
	subject, body := accountMailText(content, setup.host)
	idBytes := make([]byte, 16)
	if _, err := rand.Read(idBytes); err != nil {
		return nil, err
	}
	var message bytes.Buffer
	header := func(name, value string) {
		message.WriteString(name + ": " + value + "\r\n")
	}
	header("From", "CelikPanel <"+setup.from+">")
	header("To", "<"+to+">")
	header("Subject", mime.QEncoding.Encode("utf-8", subject))
	header("Date", now.UTC().Format(time.RFC1123Z))
	header("Message-ID", "<"+hex.EncodeToString(idBytes)+"@"+setup.domain+">")
	header("MIME-Version", "1.0")
	header("Content-Type", "text/plain; charset=UTF-8")
	header("Content-Transfer-Encoding", "quoted-printable")
	header("Auto-Submitted", "auto-generated")
	message.WriteString("\r\n")
	encoder := quotedprintable.NewWriter(&message)
	if _, err := encoder.Write([]byte(strings.ReplaceAll(body, "\n", "\r\n") + "\r\n")); err != nil {
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return message.Bytes(), nil
}

// sendAccountMail builds and hands over one message. A refused hand-over
// forgets the cached probe so the next status read looks again.
func (p *Panel) sendAccountMail(ctx context.Context, setup accountMailSetup, to string, content accountMailContent) error {
	if !setup.Available {
		return fmt.Errorf("account mail unavailable: %s", setup.Reason)
	}
	message, err := buildAccountMail(setup, to, content, time.Now())
	if err != nil {
		return err
	}
	sendCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := accountMailSend(sendCtx, setup.from, to, message); err != nil {
		resetAccountMailCache()
		return err
	}
	return nil
}
