package main

import (
	"strings"
	"testing"
)

const deliverySampleLog = `2026-11-07T09:00:00+0000 mail postfix/submission/smtpd[1]: AAAAA1: client=pc[203.0.113.5], sasl_method=PLAIN, sasl_username=a@example.com
2026-11-07T09:00:01+0000 mail postfix/qmgr[2]: AAAAA1: from=<a@example.com>, size=900, nrcpt=2 (queue active)
2026-11-07T09:00:03+0000 mail postfix/smtp[3]: AAAAA1: to=<friend@remote.org>, relay=mx.remote.org[198.51.100.1]:25, delay=2, delays=0/0/1/1, dsn=2.0.0, status=sent (250 2.0.0 Ok: queued as XYZ)
2026-11-07T09:00:03+0000 mail postfix/smtp[3]: AAAAA1: to=<slow@remote.net>, relay=none, delay=2, delays=0/0/2/0, dsn=4.4.1, status=deferred (connect to mx.remote.net[198.51.100.2]:25: Connection timed out)
2026-11-07T09:30:03+0000 mail postfix/smtp[3]: AAAAA1: to=<slow@remote.net>, relay=mx.remote.net[198.51.100.2]:25, delay=1802, delays=0/1800/1/1, dsn=2.0.0, status=sent (250 Ok)
2026-11-07T10:00:00+0000 mail postfix/smtpd[4]: BBBBB2: client=mx.sender.com[192.0.2.9]
2026-11-07T10:00:01+0000 mail postfix/qmgr[2]: BBBBB2: from=<boss@sender.com>, size=500, nrcpt=2 (queue active)
2026-11-07T10:00:02+0000 mail postfix/lmtp[5]: BBBBB2: to=<a@example.com>, relay=mail.example.net[private/dovecot-lmtp], delay=1, delays=0/0/0/1, dsn=2.0.0, status=sent (250 2.0.0 <a@example.com> Saved)
2026-11-07T10:00:02+0000 mail postfix/lmtp[5]: BBBBB2: to=<z@othercustomer.com>, relay=mail.example.net[private/dovecot-lmtp], delay=1, delays=0/0/0/1, dsn=2.0.0, status=sent (250 2.0.0 Saved)
2026-11-07T11:00:00+0000 mail postfix/smtpd[4]: NOQUEUE: reject: RCPT from x[192.0.2.10]: 550 5.1.1 <nobody@example.com>: Recipient address rejected: User unknown in virtual mailbox table; from=<spam@bad.test> to=<nobody@example.com> proto=ESMTP helo=<x>
2026-11-07T11:05:00+0000 mail postfix/qmgr[2]: CCCCC3: from=<y@othercustomer.com>, size=10, nrcpt=1 (queue active)
2026-11-07T11:05:01+0000 mail postfix/smtp[3]: CCCCC3: to=<q@remote.org>, relay=mx.remote.org[198.51.100.1]:25, delay=1, delays=0/0/0/1, dsn=2.0.0, status=sent (250 Ok)
`

func TestDeliveryLookupShowsOnlyTheDomainsOwnMail(t *testing.T) {
	scan := newDeliveryScan("example.com", "")
	if err := scanDeliveryLog(strings.NewReader(deliverySampleLog), scan); err != nil {
		t.Fatal(err)
	}
	result := scan.result()
	var got []string
	for _, e := range result.Entries {
		got = append(got, e.Direction+" "+e.From+">"+e.To+" "+e.Status+" "+e.Relay)
		if strings.Contains(e.From+e.To, "othercustomer") || strings.Contains(e.Reason, "othercustomer") {
			t.Fatalf("another customer's mail left the Agent: %+v", e)
		}
	}
	want := []string{
		"in spam@bad.test>nobody@example.com refused ",
		"in boss@sender.com>a@example.com sent mailbox",
		"out a@example.com>slow@remote.net sent mx.remote.net[198.51.100.2]:25",
		"out a@example.com>friend@remote.org sent mx.remote.org[198.51.100.1]:25",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("entries:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if result.CoveredSince == "" {
		t.Fatal("no covered-since")
	}
	narrow := newDeliveryScan("example.com", "friend@remote.org")
	_ = scanDeliveryLog(strings.NewReader(deliverySampleLog), narrow)
	if r := narrow.result(); len(r.Entries) != 1 || r.Entries[0].To != "friend@remote.org" {
		t.Fatalf("address filter: %+v", r.Entries)
	}
	foreign := newDeliveryScan("example.com", "q@remote.org")
	_ = scanDeliveryLog(strings.NewReader(deliverySampleLog), foreign)
	if r := foreign.result(); len(r.Entries) != 0 {
		t.Fatalf("an address of another customer's mail was answered: %+v", r.Entries)
	}
}
