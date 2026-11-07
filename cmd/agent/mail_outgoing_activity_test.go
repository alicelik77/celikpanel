package main

import (
	"strconv"
	"strings"
	"testing"
)

// Lines in the shape journalctl --output=short-iso prints Postfix's lines
// (Postfix 3.10 on Debian 13; submission and smtps log under the
// syslog_name CelikPanel gives them).
const outgoingSampleLog = `2026-11-07T10:05:01+0000 mail postfix/submission/smtpd[101]: 4ABCDE1: client=pc.example.net[203.0.113.5], sasl_method=PLAIN, sasl_username=a@example.com
2026-11-07T10:05:01+0000 mail postfix/qmgr[50]: 4ABCDE1: from=<a@example.com>, size=1200, nrcpt=3 (queue active)
2026-11-07T10:20:00+0000 mail postfix/qmgr[50]: 4ABCDE1: from=<a@example.com>, size=1200, nrcpt=3 (queue active)
2026-11-07T10:30:00+0000 mail postfix/smtps/smtpd[102]: 5ABCDE2: client=pc.example.net[203.0.113.5], sasl_method=LOGIN, sasl_username=A@Example.com
2026-11-07T10:30:00+0000 mail postfix/qmgr[50]: 5ABCDE2: from=<a@example.com>, size=900, nrcpt=40 (queue active)
2026-11-07T10:31:00+0000 mail postfix/submission/smtpd[101]: 6ABCDE3: client=pc.example.net[203.0.113.5], sasl_method=PLAIN, sasl_username=a@example.com
2026-11-07T10:31:00+0000 mail postfix/submission/smtpd[101]: 6ABCDE3: reject: END-OF-MESSAGE from pc.example.net[203.0.113.5]: 450 4.7.1 This mailbox reached its sending limit of 40 recipients per hour. Sending reopens within one hour and your mail program will try again.; from=<a@example.com> to=<x@else.org> proto=ESMTP helo=<pc>
2026-11-07T10:32:00+0000 mail postfix/submission/smtpd[101]: 7ABCDE4: client=pc.example.net[203.0.113.5], sasl_method=PLAIN, sasl_username=b@example.com
2026-11-07T10:32:00+0000 mail postfix/submission/smtpd[101]: 7ABCDE4: reject: END-OF-MESSAGE from pc.example.net[203.0.113.5]: 450 4.7.1 This domain reached its sending limit of 750 recipients per hour. Sending reopens within one hour and your mail program will try again.; from=<b@example.com> to=<x@else.org> proto=ESMTP helo=<pc>
2026-11-07T11:01:00+0000 mail postfix/submission/smtpd[101]: 8ABCDE5: client=pc.example.net[203.0.113.5], sasl_method=PLAIN, sasl_username=bad@example.com
2026-11-07T11:01:00+0000 mail postfix/submission/smtpd[101]: 8ABCDE5: reject: END-OF-MESSAGE from pc.example.net[203.0.113.5]: 550 5.7.1 Sending from this mailbox was stopped for security. Change the mailbox password to send again.; from=<bad@example.com> to=<x@else.org> proto=ESMTP helo=<pc>
2026-11-07T11:02:00+0000 mail postfix/submission/smtpd[101]: NOQUEUE: reject: RCPT from pc.example.net[203.0.113.5]: 553 5.7.1 <boss@example.com>: Sender address rejected: not owned by user b@example.com; from=<boss@example.com> to=<x@else.org> proto=ESMTP helo=<pc>
2026-11-07T11:03:00+0000 mail postfix/smtpd[103]: 9ABCDE6: client=other.example[198.51.100.7]
2026-11-07T11:03:00+0000 mail postfix/qmgr[50]: 9ABCDE6: from=<someone@else.org>, size=100, nrcpt=1 (queue active)
`

func TestOutgoingActivityCountsAcceptedRecipientsOnceAndRefusals(t *testing.T) {
	scan, err := scanOutgoingLog(strings.NewReader(outgoingSampleLog))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, b := range scan.result().Buckets {
		got[b.Hour+" "+b.Address] = strings.Join([]string{
			strconv.Itoa(b.Messages), strconv.Itoa(b.Recipients), strconv.Itoa(b.LimitRefusals), b.LimitKind, strconv.Itoa(b.PausedRefusals), strconv.Itoa(b.MismatchRefusals),
		}, ",")
	}
	want := map[string]string{
		// A deferred message re-entering the active queue is counted once;
		// the login's case does not make a second mailbox.
		"2026-11-07T10:00:00Z a@example.com":   "2,43,1,mailbox,0,0",
		"2026-11-07T10:00:00Z b@example.com":   "0,0,1,domain,0,0",
		"2026-11-07T11:00:00Z bad@example.com": "0,0,0,,1,0",
		"2026-11-07T11:00:00Z b@example.com":   "0,0,0,,0,1",
	}
	if len(got) != len(want) {
		t.Fatalf("buckets = %v", got)
	}
	for key, value := range want {
		if got[key] != value {
			t.Errorf("%s = %q, want %q (all %v)", key, got[key], value, got)
		}
	}
}
