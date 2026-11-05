package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os/exec"
	"os/user"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Passwordless mail report (owner decision of 4 Nov 2026, phase 1, alpha.124).
//
// The owner closes the two ways a hosted site can send mail without a
// mailbox's password: SMTP from the server itself to port 25, which a stock
// Postfix relays because loopback is in mynetworks, and the sendmail command
// (PHP mail(), cron, scripts: sendmail → postdrop → pickup). Before anything
// is closed the owner sees who used either way. This file reads only
// Postfix's own log lines; it changes nothing and needs no CelikPanel state:
//
//   - postfix/pickup "QID: uid=N from=<s>": a message handed to sendmail by
//     the Unix account N. Every such message is counted, wherever it went.
//   - postfix/smtpd (port 25 only; submission and smtps log as
//     postfix/submission/smtpd and postfix/smtps/smtpd) "QID: client=n[ip]"
//     from a loopback address with no sasl_username: SMTP from this server
//     without a password. It is counted only when Postfix then handed the
//     message to another server (a postfix/smtp "to=" line for the same
//     queue ID): delivery into this server's own mailboxes is what any other
//     server on the Internet may do, and it stays open.
//   - Refusals after the change: "User x(N) is not allowed to submit mail"
//     (sendmail/postdrop, authorized_submit_users) and "Relay access denied"
//     for a loopback client.
//
// Parolasız posta raporu (sahip kararı, 4 Kas 2026, 1. aşama): yalnız
// Postfix'in kendi günlük satırları okunur; hiçbir şey değişmez. pickup uid
// satırları (sendmail/PHP mail()) her ileti için, loopback'ten parolasız SMTP
// yalnız başka sunucuya aktarıldığında sayılır; değişiklikten sonraki retler
// de sayılır.

const (
	passwordlessReadTimeout   = 45 * time.Second
	passwordlessMaxLines      = 5_000_000
	passwordlessMaxTracked    = 100_000
	passwordlessMaxGroups     = 2_000
	passwordlessMaxDomainsPer = 5
)

// passwordlessJournalArgs reads Postfix's lines of the window from the
// journal, oldest first. Every supported distribution's Postfix logs through
// syslog, which journald receives (Debian 13 and Arch keep no mail.log file).
var passwordlessJournalArgs = []string{
	"--since=-" + strconv.Itoa(transport.PasswordlessMailWindowDays) + "d",
	"--no-pager", "--quiet", "--output=short-iso",
	"-t", "postfix/pickup", "-t", "postfix/smtpd", "-t", "postfix/qmgr",
	"-t", "postfix/smtp", "-t", "postfix/sendmail", "-t", "postfix/postdrop",
}

var (
	passwordlessQueueID      = `([0-9A-Za-z]{5,20})`
	passwordlessPickupRe     = regexp.MustCompile(`^` + passwordlessQueueID + `: uid=(\d+) from=<([^>]*)>`)
	passwordlessClientRe     = regexp.MustCompile(`^` + passwordlessQueueID + `: client=[^\[\s]*\[([^\]]+)\](.*)$`)
	passwordlessQmgrFromRe   = regexp.MustCompile(`^` + passwordlessQueueID + `: from=<([^>]*)>`)
	passwordlessQmgrDoneRe   = regexp.MustCompile(`^` + passwordlessQueueID + `: removed$`)
	passwordlessSMTPToRe     = regexp.MustCompile(`^` + passwordlessQueueID + `: to=<[^>]*>,.* relay=`)
	passwordlessSubmitDenyRe = regexp.MustCompile(`fatal: User (\S*)\((\d+)\) is not allowed to submit mail`)
	passwordlessRelayDenyRe  = regexp.MustCompile(`^NOQUEUE: reject: RCPT from [^\[\s]*\[([^\]]+)\]: .*Relay access denied; from=<([^>]*)>`)
)

type passwordlessPending struct {
	sender   string
	at       time.Time
	external bool
}

// passwordlessScan accumulates one pass over the log, oldest line first.
type passwordlessScan struct {
	earliest   time.Time
	incomplete bool
	local      map[int]*transport.PasswordlessLocalSender
	localAt    map[int]time.Time
	loopback   map[string]*transport.PasswordlessLoopbackSender
	loopbackAt map[string]time.Time
	pending    map[string]*passwordlessPending
}

func newPasswordlessScan() *passwordlessScan {
	return &passwordlessScan{
		local:      map[int]*transport.PasswordlessLocalSender{},
		localAt:    map[int]time.Time{},
		loopback:   map[string]*transport.PasswordlessLoopbackSender{},
		loopbackAt: map[string]time.Time{},
		pending:    map[string]*passwordlessPending{},
	}
}

// parsePasswordlessLine splits a journalctl short-iso line into its time,
// syslog identifier (without the [pid]) and message.
func parsePasswordlessLine(line string) (at time.Time, ident, message string, ok bool) {
	fields := strings.SplitN(line, " ", 4)
	if len(fields) < 4 {
		return time.Time{}, "", "", false
	}
	for _, layout := range []string{"2006-01-02T15:04:05-0700", time.RFC3339, time.RFC3339Nano, "2006-01-02T15:04:05.999999-0700"} {
		if parsed, err := time.Parse(layout, fields[0]); err == nil {
			at = parsed
			break
		}
	}
	if at.IsZero() {
		return time.Time{}, "", "", false
	}
	ident = strings.TrimSuffix(fields[2], ":")
	if i := strings.IndexByte(ident, '['); i > 0 {
		ident = ident[:i]
	}
	return at, ident, fields[3], true
}

func passwordlessLoopbackIP(text string) bool {
	text = strings.TrimPrefix(strings.ToLower(text), "ipv6:")
	ip := net.ParseIP(text)
	return ip != nil && ip.IsLoopback()
}

func passwordlessDomain(sender string) string {
	at := strings.LastIndexByte(sender, '@')
	if at < 0 || at == len(sender)-1 {
		return ""
	}
	return strings.ToLower(sender[at+1:])
}

func (s *passwordlessScan) localFor(uid int, at time.Time) *transport.PasswordlessLocalSender {
	entry := s.local[uid]
	if entry == nil {
		if len(s.local) >= passwordlessMaxGroups {
			s.incomplete = true
			return nil
		}
		entry = &transport.PasswordlessLocalSender{UID: uid}
		s.local[uid] = entry
	}
	if at.After(s.localAt[uid]) {
		s.localAt[uid] = at
	}
	return entry
}

func (s *passwordlessScan) loopbackFor(sender string, at time.Time) *transport.PasswordlessLoopbackSender {
	sender = strings.ToLower(sender)
	entry := s.loopback[sender]
	if entry == nil {
		if len(s.loopback) >= passwordlessMaxGroups {
			s.incomplete = true
			return nil
		}
		entry = &transport.PasswordlessLoopbackSender{Sender: sender}
		s.loopback[sender] = entry
	}
	if at.After(s.loopbackAt[sender]) {
		s.loopbackAt[sender] = at
	}
	return entry
}

// finish counts a loopback message once its queue file is gone: only one
// that was handed to another server is passwordless sending.
func (s *passwordlessScan) finish(queueID string) {
	pending := s.pending[queueID]
	if pending == nil {
		return
	}
	delete(s.pending, queueID)
	if !pending.external {
		return
	}
	if entry := s.loopbackFor(pending.sender, pending.at); entry != nil {
		entry.Messages++
	}
}

func (s *passwordlessScan) add(line string) {
	at, ident, message, ok := parsePasswordlessLine(line)
	if !ok || !strings.HasPrefix(ident, "postfix/") {
		return
	}
	if s.earliest.IsZero() || at.Before(s.earliest) {
		s.earliest = at
	}
	switch ident {
	case "postfix/pickup":
		if m := passwordlessPickupRe.FindStringSubmatch(message); m != nil {
			uid, err := strconv.Atoi(m[2])
			if err != nil {
				return
			}
			if entry := s.localFor(uid, at); entry != nil {
				entry.Messages++
				if domain := passwordlessDomain(m[3]); domain != "" && len(entry.SenderDomains) < passwordlessMaxDomainsPer && !containsString(entry.SenderDomains, domain) {
					entry.SenderDomains = append(entry.SenderDomains, domain)
				}
			}
		}
	case "postfix/sendmail", "postfix/postdrop":
		if m := passwordlessSubmitDenyRe.FindStringSubmatch(message); m != nil {
			uid, err := strconv.Atoi(m[2])
			if err != nil {
				return
			}
			if entry := s.localFor(uid, at); entry != nil {
				entry.Refused++
				if entry.User == "" {
					entry.User = m[1]
				}
			}
		}
	case "postfix/smtpd":
		if m := passwordlessClientRe.FindStringSubmatch(message); m != nil {
			if !passwordlessLoopbackIP(m[2]) || strings.Contains(m[3], "sasl_username=") {
				return
			}
			if len(s.pending) >= passwordlessMaxTracked {
				s.incomplete = true
				return
			}
			s.pending[m[1]] = &passwordlessPending{at: at}
			return
		}
		if m := passwordlessRelayDenyRe.FindStringSubmatch(message); m != nil && passwordlessLoopbackIP(m[1]) {
			if entry := s.loopbackFor(m[2], at); entry != nil {
				entry.Refused++
			}
		}
	case "postfix/qmgr":
		if m := passwordlessQmgrFromRe.FindStringSubmatch(message); m != nil {
			if pending := s.pending[m[1]]; pending != nil {
				pending.sender = m[2]
			}
			return
		}
		if m := passwordlessQmgrDoneRe.FindStringSubmatch(message); m != nil {
			s.finish(m[1])
		}
	case "postfix/smtp":
		if m := passwordlessSMTPToRe.FindStringSubmatch(message); m != nil {
			if pending := s.pending[m[1]]; pending != nil {
				pending.external = true
			}
		}
	}
}

// result closes the pass: messages still queued count when they already
// went out to another server once (a deferred copy).
func (s *passwordlessScan) result() transport.PasswordlessMailReportResponse {
	for queueID := range s.pending {
		s.finish(queueID)
	}
	out := transport.PasswordlessMailReportResponse{
		Read:            true,
		Incomplete:      s.incomplete,
		LocalSenders:    []transport.PasswordlessLocalSender{},
		LoopbackSenders: []transport.PasswordlessLoopbackSender{},
	}
	if !s.earliest.IsZero() {
		out.CoveredSince = s.earliest.UTC().Format(time.RFC3339)
	}
	for uid, entry := range s.local {
		entry.Last = s.localAt[uid].UTC().Format(time.RFC3339)
		out.LocalSenders = append(out.LocalSenders, *entry)
	}
	for sender, entry := range s.loopback {
		if entry.Messages == 0 && entry.Refused == 0 {
			continue
		}
		entry.Last = s.loopbackAt[sender].UTC().Format(time.RFC3339)
		out.LoopbackSenders = append(out.LoopbackSenders, *entry)
	}
	sort.Slice(out.LocalSenders, func(i, j int) bool { return out.LocalSenders[i].UID < out.LocalSenders[j].UID })
	sort.Slice(out.LoopbackSenders, func(i, j int) bool {
		return out.LoopbackSenders[i].Sender < out.LoopbackSenders[j].Sender
	})
	return out
}

// scanPasswordlessLog reads lines until EOF or the line bound.
func scanPasswordlessLog(reader io.Reader) (*passwordlessScan, error) {
	scan := newPasswordlessScan()
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 64*1024)
	lines := 0
	for scanner.Scan() {
		lines++
		if lines > passwordlessMaxLines {
			scan.incomplete = true
			break
		}
		scan.add(scanner.Text())
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, bufio.ErrTooLong) {
		return scan, err
	}
	return scan, nil
}

// passwordlessSystemUser: root and the mail system's own account keep the
// sendmail command after the change (cron and system notices run as root;
// Dovecot's Sieve vacation replies run sendmail as vmail).
func passwordlessSystemUser(uid int, name string) bool {
	return uid == 0 || name == vmailUser
}

// passwordlessReadJournal is replaced by tests.
var passwordlessReadJournal = func(ctx context.Context) (*passwordlessScan, error) {
	if _, err := exec.LookPath("journalctl"); err != nil {
		return nil, errors.New("journalctl is not available on this server")
	}
	cmd := exec.CommandContext(ctx, "journalctl", passwordlessJournalArgs...)
	cmd.WaitDelay = 2 * time.Second
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var stderr strings.Builder
	cmd.Stderr = &limitedWriter{w: &stderr, left: 4096}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	scan, scanErr := scanPasswordlessLog(stdout)
	if scan.incomplete {
		// Stop journalctl; what was read stays a lower bound.
		_ = cmd.Process.Kill()
	}
	waitErr := cmd.Wait()
	if ctx.Err() != nil {
		scan.incomplete = true
		return scan, nil
	}
	if scanErr != nil {
		return nil, scanErr
	}
	if waitErr != nil && !scan.incomplete {
		return nil, fmt.Errorf("journalctl: %s", boundedNativeDetail(firstNonEmpty(stderr.String(), waitErr.Error())))
	}
	return scan, nil
}

type limitedWriter struct {
	w    io.Writer
	left int
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	n := len(p)
	if l.left > 0 {
		chunk := p
		if len(chunk) > l.left {
			chunk = chunk[:l.left]
		}
		l.left -= len(chunk)
		_, _ = l.w.Write(chunk)
	}
	return n, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// PasswordlessMailReportV1 answers, read-only, who sent mail from this server
// without a password in the last 30 days.
// PasswordlessMailReportV1, son 30 günde sunucudan parolasız kimin posta
// gönderdiğini salt okunur yanıtlar.
func (a *Agent) PasswordlessMailReportV1(_ *transport.Empty, resp *transport.PasswordlessMailReportResponse) error {
	*resp = transport.PasswordlessMailReportResponse{
		LocalSenders: []transport.PasswordlessLocalSender{}, LoopbackSenders: []transport.PasswordlessLoopbackSender{},
	}
	enforcement := passwordlessEnforcementNow()
	ctx, cancel := context.WithTimeout(context.Background(), passwordlessReadTimeout)
	defer cancel()
	scan, err := passwordlessReadJournal(ctx)
	if err != nil {
		resp.Error = "Postfix's log could not be read: " + boundedNativeDetail(err.Error())
		resp.Enforcement = enforcement
		return nil
	}
	*resp = scan.result()
	resp.Source = "journal"
	resp.Enforcement = enforcement
	for i := range resp.LocalSenders {
		entry := &resp.LocalSenders[i]
		if account, err := user.LookupId(strconv.Itoa(entry.UID)); err == nil {
			entry.User = account.Username
		}
		entry.System = passwordlessSystemUser(entry.UID, entry.User)
	}
	return nil
}

// passwordlessEnforcementNow is replaced by tests.
var passwordlessEnforcementNow = func() *transport.PasswordlessMailEnforcement {
	return passwordlessEnforcement(func(name string, args ...string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return exec.CommandContext(ctx, name, args...).CombinedOutput()
	})
}
