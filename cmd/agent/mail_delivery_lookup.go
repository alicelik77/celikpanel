package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Mail delivery lookup (alpha.128, gap G9). Read only, from Postfix's own
// journal lines, like the passwordless report: per queue ID the envelope
// sender (qmgr), each recipient's outcome (smtp/lmtp/local/virtual/pipe
// "to=<r>, relay=…, dsn=…, status=…"), expiry (qmgr), and refusals before
// acceptance (smtpd "NOQUEUE: reject: RCPT … from=<s> to=<r>"). Only what
// belongs to the asked domain is returned: a message from it (every
// recipient) or to it (only its own recipients).
//
// Posta teslim sorgusu: yalnız Postfix günlüğünden, salt okunur; yalnız
// sorulan alan adına ait olan döner.

const (
	deliveryReadTimeout = 45 * time.Second
	deliveryMaxLines    = 5_000_000
	deliveryMaxTracked  = 200_000
)

var (
	deliveryFromRe    = regexp.MustCompile(`^` + passwordlessQueueID + `: from=<([^>]*)>, (?:size=\d+, nrcpt=\d+|status=expired)`)
	deliveryToRe      = regexp.MustCompile(`^` + passwordlessQueueID + `: to=<([^>]*)>,(?: orig_to=<[^>]*>,)? relay=([^,]*), .*?status=(sent|deferred|bounced|expired)(?: \((.*)\))?$`)
	deliveryExpiredRe = regexp.MustCompile(`^` + passwordlessQueueID + `: from=<([^>]*)>, status=expired`)
	deliveryRejectRe  = regexp.MustCompile(`^NOQUEUE: reject: RCPT from [^:]*: (\d{3} [0-9.]+ .*?); from=<([^>]*)> to=<([^>]*)>`)
)

type deliveryMessage struct {
	from string
	at   time.Time
}

type deliveryScan struct {
	domain, address string
	earliest        time.Time
	incomplete      bool
	messages        map[string]*deliveryMessage
	latest          map[string]int // queueID|recipient -> index in entries
	entries         []transport.MailDeliveryEntry
}

func deliveryDomainOf(address string) string {
	at := strings.LastIndexByte(address, '@')
	if at < 0 {
		return ""
	}
	return strings.ToLower(address[at+1:])
}

// visible decides what of one recipient the asked domain may see.
func (s *deliveryScan) visible(from, to string) (string, bool) {
	from, to = strings.ToLower(from), strings.ToLower(to)
	if s.address != "" && from != s.address && to != s.address {
		return "", false
	}
	switch {
	case deliveryDomainOf(from) == s.domain:
		return "out", true
	case deliveryDomainOf(to) == s.domain:
		return "in", true
	}
	return "", false
}

func deliveryReason(text string) string {
	text = strings.TrimSpace(text)
	if len(text) > 300 {
		text = text[:300] + "…"
	}
	return text
}

func (s *deliveryScan) record(queueID string, entry transport.MailDeliveryEntry) {
	key := queueID + "|" + entry.To
	if index, ok := s.latest[key]; ok && queueID != "" {
		s.entries[index] = entry // the latest outcome of the same recipient wins
		return
	}
	if len(s.entries) >= deliveryMaxTracked {
		s.incomplete = true
		return
	}
	s.entries = append(s.entries, entry)
	if queueID != "" {
		s.latest[key] = len(s.entries) - 1
	}
}

func (s *deliveryScan) add(line string) {
	at, ident, message, ok := parsePasswordlessLine(line)
	if !ok || !strings.HasPrefix(ident, "postfix") {
		return
	}
	if s.earliest.IsZero() || at.Before(s.earliest) {
		s.earliest = at
	}
	stamp := at.UTC().Format(time.RFC3339)
	if m := deliveryRejectRe.FindStringSubmatch(message); m != nil {
		if direction, ok := s.visible(m[2], m[3]); ok {
			s.record("", transport.MailDeliveryEntry{At: stamp, From: strings.ToLower(m[2]), To: strings.ToLower(m[3]), Status: "refused",
				Reason: deliveryReason(m[1]), Direction: direction})
		}
		return
	}
	if m := deliveryExpiredRe.FindStringSubmatch(message); m != nil {
		for key, index := range s.latest {
			if strings.HasPrefix(key, m[1]+"|") && s.entries[index].Status == "deferred" {
				s.entries[index].Status, s.entries[index].At = "expired", stamp
			}
		}
		return
	}
	if m := deliveryFromRe.FindStringSubmatch(message); m != nil {
		if s.messages[m[1]] == nil {
			if len(s.messages) >= deliveryMaxTracked {
				s.incomplete = true
				return
			}
			s.messages[m[1]] = &deliveryMessage{from: strings.ToLower(m[2]), at: at}
		}
		return
	}
	if m := deliveryToRe.FindStringSubmatch(message); m != nil {
		msg := s.messages[m[1]]
		if msg == nil {
			return
		}
		direction, ok := s.visible(msg.from, m[2])
		if !ok {
			return
		}
		relay := m[3]
		if strings.HasPrefix(relay, "private/dovecot-lmtp") || strings.Contains(relay, "lmtp") || relay == "local" || relay == "virtual" {
			relay = "mailbox"
		}
		s.record(m[1], transport.MailDeliveryEntry{At: stamp, From: msg.from, To: strings.ToLower(m[2]), Status: m[4], Relay: relay,
			Reason: deliveryReason(m[5]), Direction: direction})
	}
}

func (s *deliveryScan) result() transport.MailDeliveryLookupResponse {
	out := transport.MailDeliveryLookupResponse{Read: true, Incomplete: s.incomplete, Entries: append([]transport.MailDeliveryEntry{}, s.entries...)}
	if !s.earliest.IsZero() {
		out.CoveredSince = s.earliest.UTC().Format(time.RFC3339)
	}
	sort.SliceStable(out.Entries, func(i, j int) bool { return out.Entries[i].At > out.Entries[j].At })
	if len(out.Entries) > transport.MailDeliveryMaxEntries {
		out.Entries = out.Entries[:transport.MailDeliveryMaxEntries]
		out.Incomplete = true
	}
	return out
}

func newDeliveryScan(domain, address string) *deliveryScan {
	return &deliveryScan{domain: strings.ToLower(domain), address: strings.ToLower(address),
		messages: map[string]*deliveryMessage{}, latest: map[string]int{}}
}

func scanDeliveryLog(reader io.Reader, scan *deliveryScan) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 64*1024)
	lines := 0
	for scanner.Scan() {
		lines++
		if lines > deliveryMaxLines {
			scan.incomplete = true
			break
		}
		scan.add(scanner.Text())
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, bufio.ErrTooLong) {
		return err
	}
	return nil
}

// deliveryReadJournal is replaced by tests.
var deliveryReadJournal = func(ctx context.Context, days int, scan *deliveryScan) error {
	if _, err := exec.LookPath("journalctl"); err != nil {
		return errors.New("journalctl is not available on this server")
	}
	args := []string{"--since=-" + strconv.Itoa(days) + "d", "--no-pager", "--quiet", "--output=short-iso",
		"-t", "postfix/smtpd", "-t", "postfix/submission/smtpd", "-t", "postfix/smtps/smtpd", "-t", "postfix/qmgr",
		"-t", "postfix/smtp", "-t", "postfix/lmtp", "-t", "postfix/local", "-t", "postfix/virtual", "-t", "postfix/pipe"}
	cmd := exec.CommandContext(ctx, "journalctl", args...)
	cmd.WaitDelay = 2 * time.Second
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr strings.Builder
	cmd.Stderr = &limitedWriter{w: &stderr, left: 4096}
	if err := cmd.Start(); err != nil {
		return err
	}
	scanErr := scanDeliveryLog(stdout, scan)
	if scan.incomplete {
		_ = cmd.Process.Kill()
	}
	waitErr := cmd.Wait()
	if ctx.Err() != nil {
		scan.incomplete = true
		return nil
	}
	if scanErr != nil {
		return scanErr
	}
	if waitErr != nil && !scan.incomplete {
		return fmt.Errorf("journalctl: %s", boundedNativeDetail(firstNonEmpty(stderr.String(), waitErr.Error())))
	}
	return nil
}

// MailDeliveryLookupV1 answers, read only, what happened to the mail of one
// domain in the last days.
// MailDeliveryLookupV1, bir alan adının postasına son günlerde ne olduğunu
// salt okunur yanıtlar.
func (a *Agent) MailDeliveryLookupV1(req *transport.MailDeliveryLookupRequest, resp *transport.MailDeliveryLookupResponse) error {
	*resp = transport.MailDeliveryLookupResponse{Entries: []transport.MailDeliveryEntry{}}
	if req == nil {
		resp.Error = "the request is empty"
		return nil
	}
	domain := strings.ToLower(strings.TrimSpace(req.Domain))
	if _, err := transport.CanonicalMailAddress("postmaster@" + domain); err != nil || domain == "" {
		resp.Error = "the domain is not valid"
		return nil
	}
	address := strings.ToLower(strings.TrimSpace(req.Address))
	if address != "" {
		if _, err := transport.CanonicalMailAddress(address); err != nil {
			resp.Error = "the address is not valid"
			return nil
		}
	}
	days := req.Days
	if days < 1 || days > transport.MailDeliveryMaxDays {
		days = transport.MailDeliveryMaxDays
	}
	ctx, cancel := context.WithTimeout(context.Background(), deliveryReadTimeout)
	defer cancel()
	scan := newDeliveryScan(domain, address)
	if err := deliveryReadJournal(ctx, days, scan); err != nil {
		resp.Error = "Postfix's log could not be read: " + boundedNativeDetail(err.Error())
		return nil
	}
	*resp = scan.result()
	return nil
}
