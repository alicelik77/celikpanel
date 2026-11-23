package membership

// Port of portal-membership/src/Mail.php (origin/main 2584a566e).
//
// With an "smtp" block in the private configuration: implicit TLS (SMTPS) with
// certificate and host-name verification, AUTH LOGIN, no plaintext and no local
// fallback after a failure. Without one: the host's local transport, which PHP's
// mail() reaches through sendmail ("/usr/sbin/sendmail -t -i", PHP's default
// sendmail_path). Every failure is mail_unavailable; neither SMTP diagnostics nor
// credentials enter an answer or a log.

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/smtp"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// SMTPConfig is the private configuration's "smtp" object.
type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
}

// Mailer sends the service's two mails (verify, reset).
type Mailer struct {
	Sender string
	// SMTP nil: the local transport.
	SMTP *SMTPConfig
	// RootCAs replaces the system roots; tests only (PHP's openssl.cafile).
	RootCAs *x509.CertPool
	// Sendmail is the local transport's program; tests replace it.
	Sendmail func(ctx context.Context, message []byte) error
}

var (
	smtpHostPattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9.-]*[A-Za-z0-9])?$`)
	lineBreak       = regexp.MustCompile(`[\r\n]`)
)

// Send delivers one plain-text mail, or answers mail_unavailable.
func (m Mailer) Send(recipient, subject, body string) error {
	if err := m.send(recipient, subject, body); err != nil {
		return ErrMailUnavailable
	}
	return nil
}

func (m Mailer) send(recipient, subject, body string) error {
	for _, address := range []string{m.Sender, recipient} {
		if !validEmail(address) || lineBreak.MatchString(address) {
			return errors.New("invalid address")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if m.SMTP == nil {
		message := m.message(recipient, subject, body, false)
		if m.Sendmail != nil {
			return m.Sendmail(ctx, message)
		}
		return localSendmail(ctx, message)
	}
	c := m.SMTP
	if len(c.Host) > 253 || !smtpHostPattern.MatchString(c.Host) || c.Port < 1 || c.Port > 65535 ||
		!validEmail(c.Username) || c.Password == "" || len(c.Password) > 1024 {
		return errors.New("invalid SMTP configuration")
	}
	return m.smtpSend(ctx, recipient, m.message(recipient, subject, body, true))
}

// localSendmail is PHP mail()'s transport: the message on standard input of
// "sendmail -t -i", which reads the recipients from the headers.
func localSendmail(ctx context.Context, message []byte) error {
	command := exec.CommandContext(ctx, "/usr/sbin/sendmail", "-t", "-i")
	command.Stdin = bytes.NewReader(message)
	command.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin"}
	if err := command.Run(); err != nil {
		return errors.New("local transport failed")
	}
	return nil
}

// message builds the mail: UTF-8, quoted-printable body, From "CelikPanel <sender>".
func (m Mailer) message(recipient, subject, body string, smtpHeaders bool) []byte {
	var b bytes.Buffer
	header := func(name, value string) { b.WriteString(name + ": " + value + "\r\n") }
	if smtpHeaders {
		header("Date", time.Now().UTC().Format(time.RFC1123Z))
		domain := m.Sender[strings.LastIndexByte(m.Sender, '@')+1:]
		var id [16]byte
		_, _ = rand.Read(id[:])
		header("Message-ID", "<"+hex.EncodeToString(id[:])+"@"+domain+">")
	}
	header("To", recipient)
	header("From", "CelikPanel <"+m.Sender+">")
	header("Subject", mime.QEncoding.Encode("UTF-8", subject))
	header("MIME-Version", "1.0")
	header("Content-Type", "text/plain; charset=UTF-8")
	header("Content-Transfer-Encoding", "quoted-printable")
	b.WriteString("\r\n")
	w := quotedprintable.NewWriter(&b)
	_, _ = w.Write([]byte(strings.ReplaceAll(strings.ReplaceAll(body, "\r\n", "\n"), "\n", "\r\n")))
	_ = w.Close()
	return b.Bytes()
}

func (m Mailer) smtpSend(ctx context.Context, recipient string, message []byte) error {
	c := m.SMTP
	deadline, _ := ctx.Deadline()
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	config := &tls.Config{ServerName: c.Host, MinVersion: tls.VersionTLS12, RootCAs: m.RootCAs}
	conn, err := tls.DialWithDialer(dialer, "tcp", net.JoinHostPort(c.Host, itoa(c.Port)), config)
	if err != nil {
		return errors.New("tls")
	}
	defer conn.Close()
	_ = conn.SetDeadline(deadline)
	client, err := smtp.NewClient(conn, c.Host)
	if err != nil {
		return errors.New("greeting")
	}
	defer client.Close()
	if err := client.Hello("localhost"); err != nil {
		return errors.New("hello")
	}
	if ok, _ := client.Extension("AUTH"); !ok {
		return errors.New("no auth")
	}
	if err := client.Auth(loginAuth{username: c.Username, password: c.Password}); err != nil {
		_ = client.Quit()
		return errors.New("auth")
	}
	if err := client.Mail(m.Sender); err != nil {
		return errors.New("mail from")
	}
	if err := client.Rcpt(recipient); err != nil {
		return errors.New("rcpt")
	}
	w, err := client.Data()
	if err != nil {
		return errors.New("data")
	}
	if _, err := w.Write(message); err != nil {
		return errors.New("data")
	}
	if err := w.Close(); err != nil {
		return errors.New("data")
	}
	_ = client.Quit()
	return nil
}

// loginAuth is SMTP AUTH LOGIN (net/smtp has PLAIN and CRAM-MD5 only). It refuses
// to answer outside TLS.
type loginAuth struct{ username, password string }

func (a loginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	if !server.TLS {
		return "", nil, errors.New("unencrypted connection")
	}
	return "LOGIN", nil, nil
}

func (a loginAuth) Next(challenge []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	switch strings.ToLower(strings.TrimSpace(string(challenge))) {
	case "username:":
		return []byte(a.username), nil
	case "password:":
		return []byte(a.password), nil
	}
	return nil, errors.New("unexpected challenge")
}

func itoa(n int) string {
	var b [20]byte
	i := len(b)
	for {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
		if n == 0 {
			break
		}
	}
	return string(b[i:])
}
