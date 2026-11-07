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

// Outgoing mail activity (alpha.128, gap G1): what each mailbox sent per hour,
// read only from Postfix's own log lines of the submission (587) and smtps
// (465) services, the same way as the passwordless report. The panel reads the
// current and the previous hour every five minutes; it needs no CelikPanel
// state on the server and changes nothing.
//
//   - "QID: client=n[ip], sasl_method=M, sasl_username=U" (postfix/submission/smtpd
//     or postfix/smtps/smtpd): the queue ID belongs to mailbox U;
//   - "QID: from=<s>, size=N, nrcpt=K (queue active)" (postfix/qmgr): the
//     message of K recipients was accepted, counted once per queue ID;
//   - "reject: END-OF-MESSAGE ... 450 4.7.1 This mailbox reached its sending
//     limit of ...": refused by an hourly limit (or the security pause);
//   - "reject: RCPT ... Sender address rejected: not owned by user U": the
//     sender was not the signed-in mailbox (reject_sender_login_mismatch).
//
// Giden posta etkinliği: her posta kutusunun saatlik gönderimi, yalnız
// Postfix'in 587/465 günlük satırlarından, salt okunur.

const (
	outgoingActivityTimeout    = 45 * time.Second
	outgoingActivityMaxLines   = 3_000_000
	outgoingActivityMaxPending = 200_000
	outgoingActivityMaxBuckets = 200_000
	outgoingActivityMaxBack    = 8 * 24 * time.Hour
)

var (
	outgoingClientRe   = regexp.MustCompile(`^` + passwordlessQueueID + `: client=[^,]*, sasl_method=[^,]*, sasl_username=(\S+)`)
	outgoingQmgrFromRe = regexp.MustCompile(`^` + passwordlessQueueID + `: from=<[^>]*>, size=\d+, nrcpt=(\d+)`)
	outgoingRejectRe   = regexp.MustCompile(`^(` + `[0-9A-Za-z]{5,20}|NOQUEUE` + `): reject: END-OF-MESSAGE from [^:]*: [45]\d\d [45]\.\d+\.\d+ (.*?); from=<([^>]*)>`)
	outgoingMismatchRe = regexp.MustCompile(`reject: RCPT from [^:]*: 553 5\.7\.1 <[^>]*>: Sender address rejected: not owned by user (\S+?);`)
)

type outgoingBucketKey struct {
	hour    string
	address string
}

type outgoingScan struct {
	incomplete bool
	pending    map[string]string
	counted    map[string]bool
	buckets    map[outgoingBucketKey]*transport.OutgoingMailBucket
}

func newOutgoingScan() *outgoingScan {
	return &outgoingScan{pending: map[string]string{}, counted: map[string]bool{}, buckets: map[outgoingBucketKey]*transport.OutgoingMailBucket{}}
}

func (s *outgoingScan) bucket(at time.Time, address string) *transport.OutgoingMailBucket {
	address = strings.ToLower(strings.TrimSpace(address))
	if address == "" {
		return nil
	}
	key := outgoingBucketKey{hour: at.UTC().Truncate(time.Hour).Format(time.RFC3339), address: address}
	entry := s.buckets[key]
	if entry == nil {
		if len(s.buckets) >= outgoingActivityMaxBuckets {
			s.incomplete = true
			return nil
		}
		entry = &transport.OutgoingMailBucket{Hour: key.hour, Address: address}
		s.buckets[key] = entry
	}
	return entry
}

func outgoingSubmissionIdent(ident string) bool {
	return strings.HasPrefix(ident, "postfix") && strings.HasSuffix(ident, "/smtpd") && ident != "postfix/smtpd"
}

func (s *outgoingScan) add(line string) {
	at, ident, message, ok := parsePasswordlessLine(line)
	if !ok {
		return
	}
	switch {
	case outgoingSubmissionIdent(ident):
		if m := outgoingClientRe.FindStringSubmatch(message); m != nil {
			if len(s.pending) >= outgoingActivityMaxPending {
				s.incomplete = true
				return
			}
			s.pending[m[1]] = strings.ToLower(m[2])
			return
		}
		if m := outgoingRejectRe.FindStringSubmatch(message); m != nil {
			address := s.pending[m[1]]
			if address == "" {
				address = m[3]
			}
			entry := s.bucket(at, address)
			if entry == nil {
				return
			}
			text := m[2]
			switch {
			case strings.Contains(text, transport.OutgoingMailPausedMarker):
				entry.PausedRefusals++
			case strings.Contains(text, transport.OutgoingMailLimitMarker):
				entry.LimitRefusals++
				switch {
				case strings.HasPrefix(text, "This mailbox"):
					entry.LimitKind = "mailbox"
				case strings.HasPrefix(text, "This domain"):
					entry.LimitKind = "domain"
				case strings.HasPrefix(text, "This hosting account"):
					entry.LimitKind = "subscription"
				}
			}
			return
		}
		if m := outgoingMismatchRe.FindStringSubmatch(message); m != nil {
			if entry := s.bucket(at, m[1]); entry != nil {
				entry.MismatchRefusals++
			}
		}
	case ident == "postfix/qmgr":
		m := outgoingQmgrFromRe.FindStringSubmatch(message)
		if m == nil || s.counted[m[1]] {
			return
		}
		address, known := s.pending[m[1]]
		if !known {
			return
		}
		recipients, err := strconv.Atoi(m[2])
		if err != nil {
			return
		}
		s.counted[m[1]] = true
		delete(s.pending, m[1])
		if entry := s.bucket(at, address); entry != nil {
			entry.Messages++
			entry.Recipients += recipients
		}
	}
}

func (s *outgoingScan) result() transport.OutgoingMailActivityResponse {
	out := transport.OutgoingMailActivityResponse{Read: true, Incomplete: s.incomplete, Buckets: []transport.OutgoingMailBucket{}}
	for _, entry := range s.buckets {
		out.Buckets = append(out.Buckets, *entry)
	}
	sort.Slice(out.Buckets, func(i, j int) bool {
		if out.Buckets[i].Hour != out.Buckets[j].Hour {
			return out.Buckets[i].Hour < out.Buckets[j].Hour
		}
		return out.Buckets[i].Address < out.Buckets[j].Address
	})
	return out
}

func scanOutgoingLog(reader io.Reader) (*outgoingScan, error) {
	scan := newOutgoingScan()
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 64*1024)
	lines := 0
	for scanner.Scan() {
		lines++
		if lines > outgoingActivityMaxLines {
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

// outgoingReadJournal is replaced by tests.
var outgoingReadJournal = func(ctx context.Context, since time.Time) (*outgoingScan, error) {
	if _, err := exec.LookPath("journalctl"); err != nil {
		return nil, errors.New("journalctl is not available on this server")
	}
	args := []string{
		"--since=" + since.UTC().Format("2006-01-02 15:04:05 UTC"), "--no-pager", "--quiet", "--output=short-iso",
		"-t", "postfix/submission/smtpd", "-t", "postfix/smtps/smtpd", "-t", "postfix/qmgr",
	}
	cmd := exec.CommandContext(ctx, "journalctl", args...)
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
	scan, scanErr := scanOutgoingLog(stdout)
	if scan.incomplete {
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

// OutgoingMailActivityV1 answers, read only, what each mailbox sent per hour
// since the given time (at most eight days back).
// OutgoingMailActivityV1, her posta kutusunun verilen zamandan beri saatlik
// gönderimini salt okunur yanıtlar.
func (a *Agent) OutgoingMailActivityV1(req *transport.OutgoingMailActivityRequest, resp *transport.OutgoingMailActivityResponse) error {
	*resp = transport.OutgoingMailActivityResponse{Buckets: []transport.OutgoingMailBucket{}}
	since := time.Now().Add(-2 * time.Hour)
	if req != nil && strings.TrimSpace(req.Since) != "" {
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(req.Since))
		if err != nil {
			resp.Error = "the start time is not an RFC 3339 time"
			return nil
		}
		since = parsed
	}
	if oldest := time.Now().Add(-outgoingActivityMaxBack); since.Before(oldest) {
		since = oldest
	}
	ctx, cancel := context.WithTimeout(context.Background(), outgoingActivityTimeout)
	defer cancel()
	scan, err := outgoingReadJournal(ctx, since)
	if err != nil {
		resp.Error = "Postfix's log could not be read: " + boundedNativeDetail(err.Error())
		return nil
	}
	*resp = scan.result()
	return nil
}
