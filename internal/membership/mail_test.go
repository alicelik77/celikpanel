package membership

// Port of portal-membership/tests/smtp_test.py's four SMTP cases against the Go
// mailer: an isolated TLS SMTP server on the loopback with a certificate made here
// for "localhost". The password is a test value; no real mail server is reached.

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"errors"
	"io"
	"math/big"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"strings"
	"testing"
	"time"
)

const isolatedPassword = "isolated-test-password"

type smtpProbe struct {
	commands []string
	username string
	password string
	message  string
	err      error
}

func testCertificate(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "localhost"},
		DNSNames: []string{"localhost"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(parsed)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, pool
}

// smtpServer answers one connection the way smtp_test.py's server does.
func smtpServer(t *testing.T, certificate tls.Certificate, rejectAuth bool) (int, <-chan smtpProbe) {
	t.Helper()
	listener, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{certificate}})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan smtpProbe, 1)
	go func() {
		var probe smtpProbe
		defer func() { listener.Close(); done <- probe }()
		conn, err := listener.Accept()
		if err != nil {
			probe.err = err
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
		if err := conn.(*tls.Conn).Handshake(); err != nil {
			return // the client refused the certificate: nothing was sent
		}
		reader := bufio.NewReader(conn)
		reply := func(s string) { _, _ = io.WriteString(conn, s+"\r\n") }
		read := func() (string, bool) {
			line, err := reader.ReadString('\n')
			if err != nil {
				return "", false
			}
			return strings.TrimRight(line, "\r\n"), true
		}
		reply("220 localhost ESMTP test")
		line, ok := read()
		if !ok {
			return
		}
		probe.commands = append(probe.commands, line)
		reply("250-localhost\r\n250 AUTH LOGIN")
		if line, ok = read(); !ok {
			return
		}
		probe.commands = append(probe.commands, line)
		reply("334 VXNlcm5hbWU6")
		line, _ = read()
		user, _ := base64.StdEncoding.DecodeString(line)
		probe.username = string(user)
		reply("334 UGFzc3dvcmQ6")
		line, _ = read()
		pass, _ := base64.StdEncoding.DecodeString(line)
		probe.password = string(pass)
		if rejectAuth {
			reply("535 5.7.8 test password rejection")
			for {
				if line, ok = read(); !ok {
					return
				}
				if line == "QUIT" {
					reply("221 bye")
					return
				}
				reply("501 cancelled")
			}
		}
		reply("235 authenticated")
		for _, answer := range []string{"250 sender ok", "250 recipient ok", "354 end with dot"} {
			if line, ok = read(); !ok {
				return
			}
			probe.commands = append(probe.commands, line)
			reply(answer)
		}
		var lines []string
		for {
			if line, ok = read(); !ok {
				return
			}
			if line == "." {
				break
			}
			lines = append(lines, strings.TrimPrefix(line, "."))
		}
		probe.message = strings.Join(lines, "\r\n")
		reply("250 queued")
		if line, ok = read(); ok && line == "QUIT" {
			reply("221 bye")
		}
	}()
	return listener.Addr().(*net.TCPAddr).Port, done
}

func probeMailer(t *testing.T, trusted bool, host string, rejectAuth bool) (error, smtpProbe) {
	t.Helper()
	certificate, pool := testCertificate(t)
	port, done := smtpServer(t, certificate, rejectAuth)
	mailer := Mailer{Sender: "sender@example.test", SMTP: &SMTPConfig{Host: host, Port: port, Username: "sender@example.test", Password: isolatedPassword}}
	if trusted {
		mailer.RootCAs = pool
	}
	err := mailer.Send("recipient@example.test", "CelikPanel test", "Test body\n.leading dot\n")
	select {
	case probe := <-done:
		if probe.err != nil {
			t.Fatal(probe.err)
		}
		if err != nil && strings.Contains(err.Error(), isolatedPassword) {
			t.Fatal("an error carries the password")
		}
		return err, probe
	case <-time.After(20 * time.Second):
		t.Fatal("the SMTP server did not finish")
	}
	return nil, smtpProbe{}
}

func TestSMTPVerifiedTLSAuthenticationAndMessage(t *testing.T) {
	err, probe := probeMailer(t, true, "localhost", false)
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if probe.username != "sender@example.test" || probe.password != isolatedPassword {
		t.Fatalf("AUTH LOGIN carried %q", probe.username)
	}
	if !contains(probe.commands, "MAIL FROM:<sender@example.test>") || !contains(probe.commands, "RCPT TO:<recipient@example.test>") {
		t.Fatalf("commands: %q", probe.commands)
	}
	message, err := mail.ReadMessage(strings.NewReader(probe.message + "\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	subject, _ := new(mime.WordDecoder).DecodeHeader(message.Header.Get("Subject"))
	if subject != "CelikPanel test" {
		t.Fatalf("subject %q", subject)
	}
	body, _ := io.ReadAll(quotedprintable.NewReader(message.Body))
	if !strings.Contains(string(body), ".leading dot") {
		t.Fatalf("body %q", body)
	}
}

func TestSMTPUntrustedCertificateFailsBeforeAuthentication(t *testing.T) {
	err, probe := probeMailer(t, false, "localhost", false)
	if !errors.Is(err, ErrMailUnavailable) || err.Error() != "mail_unavailable" || len(probe.commands) != 0 {
		t.Fatalf("err %v, commands %q", err, probe.commands)
	}
}

func TestSMTPWrongCertificateHostnameFailsBeforeAuthentication(t *testing.T) {
	err, probe := probeMailer(t, true, "127.0.0.1", false)
	if !errors.Is(err, ErrMailUnavailable) || len(probe.commands) != 0 {
		t.Fatalf("err %v, commands %q", err, probe.commands)
	}
}

func TestSMTPAuthenticationRejectionHasNoDeliveryOrDiagnostics(t *testing.T) {
	err, probe := probeMailer(t, true, "localhost", true)
	if !errors.Is(err, ErrMailUnavailable) || probe.message != "" || strings.Contains(err.Error(), "535") {
		t.Fatalf("err %v, message %q", err, probe.message)
	}
}

func TestLocalTransportAndRefusedConfigurations(t *testing.T) {
	var got []byte
	mailer := Mailer{Sender: "sender@example.test", Sendmail: func(_ context.Context, message []byte) error { got = message; return nil }}
	if err := mailer.Send("recipient@example.test", "CelikPanel test", "body"); err != nil {
		t.Fatal(err)
	}
	for _, header := range []string{"To: recipient@example.test\r\n", "From: CelikPanel <sender@example.test>\r\n", "Content-Type: text/plain; charset=UTF-8\r\n"} {
		if !strings.Contains(string(got), header) {
			t.Fatalf("local message lacks %q:\n%s", header, got)
		}
	}
	failing := Mailer{Sender: "sender@example.test", Sendmail: func(context.Context, []byte) error { return errors.New("exit status 75") }}
	if err := failing.Send("recipient@example.test", "s", "b"); !errors.Is(err, ErrMailUnavailable) || strings.Contains(err.Error(), "75") {
		t.Fatalf("local failure: %v", err)
	}
	for name, m := range map[string]Mailer{
		"recipient with a line break": {Sender: "sender@example.test", Sendmail: mailer.Sendmail},
		"bad SMTP host":               {Sender: "sender@example.test", SMTP: &SMTPConfig{Host: "-bad", Port: 465, Username: "a@example.test", Password: "x"}},
		"SMTP port out of range":      {Sender: "sender@example.test", SMTP: &SMTPConfig{Host: "localhost", Port: 70000, Username: "a@example.test", Password: "x"}},
		"SMTP without a password":     {Sender: "sender@example.test", SMTP: &SMTPConfig{Host: "localhost", Port: 465, Username: "a@example.test"}},
	} {
		recipient := "recipient@example.test"
		if name == "recipient with a line break" {
			recipient = "recipient@example.test\r\nBcc: x@example.test"
		}
		if err := m.Send(recipient, "s", "b"); !errors.Is(err, ErrMailUnavailable) {
			t.Errorf("%s: %v", name, err)
		}
	}
	// A present but unusable smtp block never falls back to the local transport.
	config, err := ParseConfig([]byte(`{"origin":"https://example.test","sender":"s@example.test","signing_key":"` +
		base64.StdEncoding.EncodeToString(make([]byte, 64)) + `","smtp":{"host":"localhost","port":465.0,"username":"a@example.test","password":"x"}}`))
	if err != nil || !config.SMTPPresent || config.SMTP != nil {
		t.Fatalf("a float port must leave the smtp block unusable: %+v %v", config.SMTP, err)
	}
	if err := (&Handler{}).sender(config)("recipient@example.test", "verify", strings.Repeat("a", 64)); !errors.Is(err, ErrMailUnavailable) {
		t.Fatalf("unusable smtp block: %v", err)
	}
}

func contains(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}
