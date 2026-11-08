package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Outgoing mail through another server (alpha.130, smarthost;
// docs/MAIL-RELAY.md). Server-wide only, administrator only (the
// /api/v1/mail/ prefix is admin-only in the middleware):
//
//	GET  /api/v1/mail/relay        the settings (never the password), the last
//	                               apply and what the server holds now
//	POST /api/v1/mail/relay        save and apply (steps: check, save, apply)
//	POST /api/v1/mail/relay/test   sign in to the relay and log out again;
//	                               no message is ever sent
//
// The password is sealed with the panel's secret key and is never sent back.
// Turning the relay off removes CelikPanel's Postfix lines and files on the
// server and clears the sealed password.
//
// Posta başka bir sunucu üzerinden: yalnız yönetici, sunucu geneli. Parola
// mühürlü saklanır, ekrana dönmez; kapatınca sunucudaki satırlar ve dosya
// kalkar, parola silinir. Deneme yalnız oturum açıp kapatır, ileti göndermez.

const (
	mailRelayPath     = "/api/v1/mail/relay"
	mailRelayTestPath = "/api/v1/mail/relay/test"

	errCodeMailRelayPasswordRequired = "MAIL_RELAY_PASSWORD_REQUIRED"
	errCodeMailRelayInvalid          = "MAIL_RELAY_INVALID"
)

type mailRelaySettings struct {
	Enabled     bool   `json:"enabled"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	HasPassword bool   `json:"has_password"`
}

type mailRelayApply struct {
	State     string `json:"state,omitempty"`
	Code      string `json:"code,omitempty"`
	Detail    string `json:"detail,omitempty"`
	AppliedAt string `json:"applied_at,omitempty"`
}

type mailRelayView struct {
	Settings      mailRelaySettings          `json:"settings"`
	Apply         mailRelayApply             `json:"apply"`
	Server        *transport.MailRelayStatus `json:"server,omitempty"`
	ServerUnknown string                     `json:"server_unknown,omitempty"`
}

type mailRelayRow struct {
	settings mailRelaySettings
	sealed   string
	apply    mailRelayApply
}

func (p *Panel) readMailRelay(ctx context.Context) (mailRelayRow, error) {
	var row mailRelayRow
	var enabled int
	var appliedAt sql.NullString
	err := p.db.GetDB().QueryRowContext(ctx, `
		SELECT enabled, host, port, username, password_sealed, apply_state, apply_code, apply_detail, applied_at
		FROM mail_relay_settings WHERE id = 1`).Scan(&enabled, &row.settings.Host, &row.settings.Port,
		&row.settings.Username, &row.sealed, &row.apply.State, &row.apply.Code, &row.apply.Detail, &appliedAt)
	if err != nil {
		return row, err
	}
	row.settings.Enabled = enabled == 1
	row.settings.HasPassword = row.sealed != ""
	row.apply.AppliedAt = appliedAt.String
	return row, nil
}

// Swapped by tests.
var (
	mailRelayAgentStatus = func(p *Panel, ctx context.Context) (transport.MailRelayStatus, error) {
		var status transport.MailRelayStatus
		err := p.callAgentContext(ctx, "Agent.MailRelayStatusV1", &transport.Empty{}, &status)
		return status, err
	}
	mailRelayAgentApply = func(p *Panel, ctx context.Context, req *transport.MailRelayApplyRequest) (transport.MailRelayApplyResponse, error) {
		var resp transport.MailRelayApplyResponse
		err := p.callAgentContext(ctx, "Agent.MailRelayApplyV1", req, &resp)
		return resp, err
	}
)

func (p *Panel) handleMailRelay(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	switch {
	case r.URL.Path == mailRelayTestPath:
		p.handleMailRelayTest(w, r)
		return
	case r.URL.Path != mailRelayPath:
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		row, err := p.readMailRelay(r.Context())
		if err != nil {
			writeServerError(w, err)
			return
		}
		view := mailRelayView{Settings: row.settings, Apply: row.apply}
		status, err := mailRelayAgentStatus(p, r.Context())
		switch {
		case err != nil:
			view.ServerUnknown = "the server's mail settings could not be read just now"
		case status.Error != "":
			view.Server = &status
			view.ServerUnknown = status.Error
		default:
			view.Server = &status
		}
		_ = json.NewEncoder(w).Encode(view)
	case http.MethodPost:
		p.saveMailRelay(w, r)
	default:
		w.Header().Set("Allow", "GET, POST")
		writeClientError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

type mailRelayRequest struct {
	Enabled  bool   `json:"enabled"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	// Password: empty keeps the one saved for the same host and user name.
	Password string `json:"password"`
}

// relayPassword returns the password to use: the typed one, else the saved
// one when host and user name did not change. A changed relay or user needs
// its own password: a saved password is never sent to another server.
func (p *Panel) relayPassword(row mailRelayRow, host, username, typed string) (string, error) {
	if typed != "" {
		return typed, nil
	}
	if row.sealed == "" || !strings.EqualFold(strings.TrimSuffix(row.settings.Host, "."), strings.TrimSuffix(host, ".")) || row.settings.Username != username {
		return "", nil
	}
	if p.secrets == nil {
		return "", errors.New("the panel's secret key is not available")
	}
	return p.secrets.Decrypt(row.sealed)
}

func (p *Panel) saveMailRelay(w http.ResponseWriter, r *http.Request) {
	var request mailRelayRequest
	if err := decodeStrictJSON(w, r, &request); err != nil {
		writeClientError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	ctx := r.Context()
	reportOperationPlan(ctx, "check", "save", "apply")
	reportOperationStep(ctx, "check")
	row, err := p.readMailRelay(ctx)
	if err != nil {
		writeServerError(w, err)
		return
	}
	host := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(request.Host), "."))
	username := strings.TrimSpace(request.Username)
	password := ""
	if request.Enabled {
		if _, err := transport.MailRelayNexthop(host, request.Port); err != nil {
			writeCodedError(w, http.StatusBadRequest, errCodeMailRelayInvalid, err.Error(), "")
			return
		}
		if err := transport.ValidateMailRelayUsername(username); err != nil {
			writeCodedError(w, http.StatusBadRequest, errCodeMailRelayInvalid, err.Error(), "")
			return
		}
		if password, err = p.relayPassword(row, host, username, request.Password); err != nil {
			writeServerError(w, fmt.Errorf("open the saved relay password: %w", err))
			return
		}
		if password == "" {
			writeCodedError(w, http.StatusBadRequest, errCodeMailRelayPasswordRequired,
				"enter the relay password: no password is saved for this relay and user name", "")
			return
		}
		if err := transport.ValidateMailRelayPassword(password); err != nil {
			writeCodedError(w, http.StatusBadRequest, errCodeMailRelayInvalid, err.Error(), "")
			return
		}
	}

	reportOperationStep(ctx, "save")
	sealed := row.sealed
	if request.Enabled && request.Password != "" {
		if p.secrets == nil {
			writeServerError(w, errors.New("the panel's secret key is not available"))
			return
		}
		if sealed, err = p.secrets.Encrypt(password); err != nil {
			writeServerError(w, fmt.Errorf("seal the relay password: %w", err))
			return
		}
	}
	enabled := 0
	port := row.settings.Port
	if request.Enabled {
		enabled, port = 1, request.Port
	} else {
		// Turning off keeps the relay's name for the next time, never more.
		host, username = row.settings.Host, row.settings.Username
	}
	if _, err := p.db.GetDB().ExecContext(ctx, `
		UPDATE mail_relay_settings SET enabled = ?, host = ?, port = ?, username = ?, password_sealed = ?, updated_at = datetime('now')
		WHERE id = 1`, enabled, host, port, username, sealed); err != nil {
		writeServerError(w, err)
		return
	}
	p.auditDetailed(r, "mail.relay.settings", "mail_relay", 1, map[string]any{"enabled": request.Enabled, "host": host, "port": port})

	reportOperationStep(ctx, "apply")
	agentRequest := transport.MailRelayApplyRequest{ExpectedBuildCommit: strings.TrimSpace(buildCommit), Enabled: request.Enabled}
	if request.Enabled {
		agentRequest.Host, agentRequest.Port, agentRequest.Username, agentRequest.Password = host, port, username, password
	}
	resp, err := mailRelayAgentApply(p, ctx, &agentRequest)
	answer := map[string]any{"saved": true}
	if err != nil {
		log.Printf("[mail-relay] apply: the Agent did not answer: %v", err)
		p.recordMailRelayApply(ctx, "unknown", "", "the server did not answer; whether it took the change is not known. Read the state again before saving again")
		answer["unknown"] = "the server did not answer; whether it took the change is not known"
		_ = json.NewEncoder(w).Encode(answer)
		return
	}
	switch {
	case resp.Applied:
		p.recordMailRelayApply(ctx, "applied", "", "")
		if !request.Enabled {
			// Nothing on the server needs the password any more.
			if _, err := p.db.GetDB().ExecContext(ctx, `UPDATE mail_relay_settings SET password_sealed = '' WHERE id = 1`); err != nil {
				log.Printf("[mail-relay] clear the sealed password: %v", err)
			}
		}
	case resp.Code == transport.MailRelayOwnerConfig || resp.Code == transport.MailRelayEditedConfig ||
		resp.Code == transport.MailRelaySASLMissing || resp.Code == transport.MailRelayInvalid ||
		resp.Code == transport.MailRelayNoPostfix || resp.Code == transport.MailRelayBusy:
		p.recordMailRelayApply(ctx, "refused", resp.Code, resp.Error)
	default:
		p.recordMailRelayApply(ctx, "failed", resp.Code, resp.Error)
	}
	answer["result"] = resp
	_ = json.NewEncoder(w).Encode(answer)
}

func (p *Panel) recordMailRelayApply(ctx context.Context, state, code, detail string) {
	var err error
	if state == "applied" {
		_, err = p.db.GetDB().ExecContext(ctx, `
			UPDATE mail_relay_settings SET apply_state = ?, apply_code = '', apply_detail = '', applied_at = datetime('now') WHERE id = 1`, state)
	} else {
		_, err = p.db.GetDB().ExecContext(ctx, `
			UPDATE mail_relay_settings SET apply_state = ?, apply_code = ?, apply_detail = ? WHERE id = 1`, state, code, relayBounded(detail, 1000))
	}
	if err != nil {
		log.Printf("[mail-relay] record the apply: %v", err)
	}
}

func relayBounded(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}

// ---- The sign-in test ---------------------------------------------------------

// The relay test never sends a message: it connects, starts TLS the way
// Postfix will (STARTTLS, or TLS from the first byte on 465), signs in and
// says QUIT. Tests swap the dialer; nothing reaches a real server in tests.
var (
	mailRelayDial = func(ctx context.Context, address string) (net.Conn, error) {
		var dialer net.Dialer
		return dialer.DialContext(ctx, "tcp", address)
	}
	mailRelayTestRoots *x509.CertPool // nil: the system's roots
	mailRelayTestWait  = 20 * time.Second
)

type mailRelayTestAnswer struct {
	OK bool `json:"ok"`
	// Stage where it stopped: connect, greeting, tls, auth, done.
	Stage string `json:"stage"`
	// Verified: the relay's certificate is valid for its name. Postfix
	// encrypts either way (smtp_tls_security_level=encrypt); this only says
	// whether the name could also be proved.
	Verified bool   `json:"verified"`
	Detail   string `json:"detail,omitempty"`
}

func (p *Panel) handleMailRelayTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeClientError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var request mailRelayRequest
	if err := decodeStrictJSON(w, r, &request); err != nil {
		writeClientError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	host := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(request.Host), "."))
	username := strings.TrimSpace(request.Username)
	if _, err := transport.MailRelayNexthop(host, request.Port); err != nil {
		writeCodedError(w, http.StatusBadRequest, errCodeMailRelayInvalid, err.Error(), "")
		return
	}
	if err := transport.ValidateMailRelayUsername(username); err != nil {
		writeCodedError(w, http.StatusBadRequest, errCodeMailRelayInvalid, err.Error(), "")
		return
	}
	row, err := p.readMailRelay(r.Context())
	if err != nil {
		writeServerError(w, err)
		return
	}
	password, err := p.relayPassword(row, host, username, request.Password)
	if err != nil {
		writeServerError(w, fmt.Errorf("open the saved relay password: %w", err))
		return
	}
	if password == "" {
		writeCodedError(w, http.StatusBadRequest, errCodeMailRelayPasswordRequired,
			"enter the relay password: no password is saved for this relay and user name", "")
		return
	}
	if err := transport.ValidateMailRelayPassword(password); err != nil {
		writeCodedError(w, http.StatusBadRequest, errCodeMailRelayInvalid, err.Error(), "")
		return
	}
	helo, err := p.mailTLSHostname(r.Context())
	if err != nil {
		helo = "localhost"
	}
	ctx, cancel := context.WithTimeout(r.Context(), mailRelayTestWait)
	defer cancel()
	answer := testMailRelaySignIn(ctx, host, request.Port, username, password, helo)
	p.auditDetailed(r, "mail.relay.test", "mail_relay", 1, map[string]any{"host": host, "port": request.Port, "ok": answer.OK, "stage": answer.Stage})
	_ = json.NewEncoder(w).Encode(answer)
}

// smtpConn is a minimal SMTP client for the test: no message commands exist
// in it, so it cannot send mail.
type smtpConn struct {
	conn   net.Conn
	reader *bufio.Reader
}

func (c *smtpConn) reply() (int, []string, error) {
	var lines []string
	for {
		line, err := c.reader.ReadString('\n')
		if err != nil {
			return 0, lines, err
		}
		line = strings.TrimRight(line, "\r\n")
		if len(line) < 3 {
			return 0, lines, fmt.Errorf("short reply %q", line)
		}
		code, err := strconv.Atoi(line[:3])
		if err != nil {
			return 0, lines, fmt.Errorf("bad reply %q", relayBounded(line, 80))
		}
		lines = append(lines, strings.TrimSpace(line[min(4, len(line)):]))
		if len(line) == 3 || line[3] != '-' {
			return code, lines, nil
		}
		if len(lines) > 100 {
			return 0, lines, errors.New("reply too long")
		}
	}
}

func (c *smtpConn) command(line string) (int, []string, error) {
	if _, err := c.conn.Write([]byte(line + "\r\n")); err != nil {
		return 0, nil, err
	}
	return c.reply()
}

func relayReplyText(lines []string) string {
	return relayBounded(strings.Join(lines, " "), 200)
}

func testMailRelaySignIn(ctx context.Context, host string, port int, username, password, helo string) mailRelayTestAnswer {
	answer := mailRelayTestAnswer{Stage: "connect"}
	address := net.JoinHostPort(host, strconv.Itoa(port))
	raw, err := mailRelayDial(ctx, address)
	if err != nil {
		answer.Detail = "no connection to " + address + ": " + relayBounded(err.Error(), 200)
		return answer
	}
	defer raw.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = raw.SetDeadline(deadline)
	}
	verified := false
	tlsConfig := &tls.Config{
		ServerName: host,
		MinVersion: tls.VersionTLS12,
		// Postfix's encrypt level does not verify the name either; the test
		// verifies it itself and reports the result instead of refusing.
		InsecureSkipVerify: true,
		VerifyConnection: func(state tls.ConnectionState) error {
			if len(state.PeerCertificates) == 0 {
				return nil
			}
			intermediates := x509.NewCertPool()
			for _, cert := range state.PeerCertificates[1:] {
				intermediates.AddCert(cert)
			}
			_, verifyErr := state.PeerCertificates[0].Verify(x509.VerifyOptions{DNSName: host, Intermediates: intermediates, Roots: mailRelayTestRoots})
			verified = verifyErr == nil
			return nil
		},
	}
	conn := &smtpConn{conn: raw, reader: bufio.NewReader(raw)}
	if port == 465 {
		answer.Stage = "tls"
		secure := tls.Client(raw, tlsConfig)
		if err := secure.HandshakeContext(ctx); err != nil {
			answer.Detail = "TLS did not start: " + relayBounded(err.Error(), 200)
			return answer
		}
		conn = &smtpConn{conn: secure, reader: bufio.NewReader(secure)}
	}
	answer.Stage = "greeting"
	if code, lines, err := conn.reply(); err != nil || code != 220 {
		answer.Detail = "the relay did not greet: " + relayFirstText(relayErrText(err), relayReplyText(lines))
		return answer
	}
	code, lines, err := conn.command("EHLO " + helo)
	if err != nil || code != 250 {
		answer.Detail = "the relay refused EHLO: " + relayFirstText(relayErrText(err), relayReplyText(lines))
		return answer
	}
	if port != 465 {
		answer.Stage = "tls"
		if !relayOffers(lines, "STARTTLS") {
			answer.Detail = "the relay does not offer STARTTLS on port " + strconv.Itoa(port) + "; Postfix sends to it only encrypted, so mail would wait in the queue"
			return answer
		}
		if code, lines, err := conn.command("STARTTLS"); err != nil || code != 220 {
			answer.Detail = "the relay refused STARTTLS: " + relayFirstText(relayErrText(err), relayReplyText(lines))
			return answer
		}
		secure := tls.Client(raw, tlsConfig)
		if err := secure.HandshakeContext(ctx); err != nil {
			answer.Detail = "TLS did not start: " + relayBounded(err.Error(), 200)
			return answer
		}
		conn = &smtpConn{conn: secure, reader: bufio.NewReader(secure)}
		if code, lines, err = conn.command("EHLO " + helo); err != nil || code != 250 {
			answer.Detail = "the relay refused EHLO after TLS: " + relayFirstText(relayErrText(err), relayReplyText(lines))
			return answer
		}
	}
	answer.Verified = verified
	answer.Stage = "auth"
	mechanisms := relayAuthMechanisms(lines)
	switch {
	case mechanisms["PLAIN"]:
		token := base64.StdEncoding.EncodeToString([]byte("\x00" + username + "\x00" + password))
		code, lines, err = conn.command("AUTH PLAIN " + token)
	case mechanisms["LOGIN"]:
		code, lines, err = conn.command("AUTH LOGIN")
		if err == nil && code == 334 {
			code, lines, err = conn.command(base64.StdEncoding.EncodeToString([]byte(username)))
		}
		if err == nil && code == 334 {
			code, lines, err = conn.command(base64.StdEncoding.EncodeToString([]byte(password)))
		}
	default:
		answer.Detail = "the relay offers no password sign-in (AUTH PLAIN or LOGIN) after TLS"
		_, _, _ = conn.command("QUIT")
		return answer
	}
	if err != nil || code != 235 {
		answer.Detail = "the relay did not accept the user name and password: " + relayFirstText(relayErrText(err), relayReplyText(lines))
		_, _, _ = conn.command("QUIT")
		return answer
	}
	_, _, _ = conn.command("QUIT")
	answer.OK, answer.Stage = true, "done"
	return answer
}

func relayOffers(lines []string, keyword string) bool {
	for _, line := range lines {
		if strings.EqualFold(strings.Fields(line + " x")[0], keyword) {
			return true
		}
	}
	return false
}

func relayAuthMechanisms(lines []string) map[string]bool {
	out := map[string]bool{}
	for _, line := range lines {
		fields := strings.Fields(strings.ToUpper(line))
		if len(fields) > 0 && (fields[0] == "AUTH" || strings.HasPrefix(fields[0], "AUTH=")) {
			if strings.HasPrefix(fields[0], "AUTH=") {
				out[strings.TrimPrefix(fields[0], "AUTH=")] = true
			}
			for _, mechanism := range fields[1:] {
				out[mechanism] = true
			}
		}
	}
	return out
}

func relayErrText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func relayFirstText(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return "no answer"
}
