package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Real Debian 13 Postfix 3.10 lines (short-iso), trimmed to the ones the
// report reads.
const passwordlessSampleLog = `2026-10-10T08:00:00+00:00 mx postfix/pickup[100]: 4A1B2C3D4E: uid=1001 from=<wordpress@shop.example>
2026-10-10T08:00:01+00:00 mx postfix/qmgr[90]: 4A1B2C3D4E: from=<wordpress@shop.example>, size=812, nrcpt=1 (queue active)
2026-10-10T08:00:02+00:00 mx postfix/smtp[101]: 4A1B2C3D4E: to=<buyer@gmail.example>, relay=gmail.example[192.0.2.5]:25, delay=1, status=sent (250 ok)
2026-10-10T08:00:02+00:00 mx postfix/qmgr[90]: 4A1B2C3D4E: removed
2026-10-11T09:00:00+00:00 mx postfix/pickup[100]: 5B1B2C3D4E: uid=0 from=<root@mx.example>
2026-10-11T09:00:00+00:00 mx postfix/pickup[100]: 6C1B2C3D4E: uid=1001 from=<>
2026-10-12T10:00:00+00:00 mx postfix/smtpd[200]: 7D1B2C3D4E: client=localhost[127.0.0.1]
2026-10-12T10:00:00+00:00 mx postfix/qmgr[90]: 7D1B2C3D4E: from=<contact@blog.example>, size=900, nrcpt=1 (queue active)
2026-10-12T10:00:01+00:00 mx postfix/smtp[201]: 7D1B2C3D4E: to=<someone@outside.example>, relay=outside.example[192.0.2.9]:25, delay=1, status=sent (250 ok)
2026-10-12T10:00:01+00:00 mx postfix/qmgr[90]: 7D1B2C3D4E: removed
2026-10-12T11:00:00+00:00 mx postfix/smtpd[200]: 8E1B2C3D4E: client=localhost[::1]
2026-10-12T11:00:00+00:00 mx postfix/qmgr[90]: 8E1B2C3D4E: from=<contact@blog.example>, size=900, nrcpt=1 (queue active)
2026-10-12T11:00:01+00:00 mx postfix/virtual[202]: 8E1B2C3D4E: to=<owner@blog.example>, relay=virtual, delay=0, status=sent (delivered to maildir)
2026-10-12T11:00:01+00:00 mx postfix/qmgr[90]: 8E1B2C3D4E: removed
2026-10-12T12:00:00+00:00 mx postfix/smtpd[200]: 9F1B2C3D4E: client=localhost[127.0.0.1], sasl_method=PLAIN, sasl_username=contact@blog.example
2026-10-12T12:00:00+00:00 mx postfix/qmgr[90]: 9F1B2C3D4E: from=<contact@blog.example>, size=900, nrcpt=1 (queue active)
2026-10-12T12:00:01+00:00 mx postfix/smtp[201]: 9F1B2C3D4E: to=<x@outside.example>, relay=outside.example[192.0.2.9]:25, delay=1, status=sent (250 ok)
2026-10-12T12:00:01+00:00 mx postfix/qmgr[90]: 9F1B2C3D4E: removed
2026-10-12T13:00:00+00:00 mx postfix/smtpd[200]: AA1B2C3D4E: client=mail.outside.example[192.0.2.9]
2026-10-12T13:00:00+00:00 mx postfix/qmgr[90]: AA1B2C3D4E: from=<x@outside.example>, size=900, nrcpt=1 (queue active)
2026-10-12T13:00:01+00:00 mx postfix/qmgr[90]: AA1B2C3D4E: removed
2026-11-02T07:00:00+00:00 mx postfix/sendmail[300]: fatal: User shop_example(1001) is not allowed to submit mail
2026-11-02T07:01:00+00:00 mx postfix/smtpd[200]: NOQUEUE: reject: RCPT from localhost[127.0.0.1]: 454 4.7.1 <x@outside.example>: Relay access denied; from=<contact@blog.example> to=<x@outside.example> proto=ESMTP helo=<blog>
2026-11-02T07:02:00+00:00 mx postfix/submission/smtpd[400]: BB1B2C3D4E: client=localhost[127.0.0.1]
2026-11-02T07:02:00+00:00 mx postfix/qmgr[90]: BB1B2C3D4E: from=<contact@blog.example>, size=900, nrcpt=1 (queue active)
2026-11-02T07:02:01+00:00 mx postfix/smtp[201]: BB1B2C3D4E: to=<x@outside.example>, relay=outside.example[192.0.2.9]:25, delay=1, status=sent (250 ok)
2026-11-02T07:02:01+00:00 mx postfix/qmgr[90]: BB1B2C3D4E: removed
`

func TestPasswordlessScanCountsBothWaysAndOnlyThem(t *testing.T) {
	scan, err := scanPasswordlessLog(strings.NewReader(passwordlessSampleLog))
	if err != nil {
		t.Fatal(err)
	}
	got := scan.result()
	if !got.Read || got.Incomplete {
		t.Fatalf("read=%v incomplete=%v", got.Read, got.Incomplete)
	}
	if got.CoveredSince != "2026-10-10T08:00:00Z" {
		t.Fatalf("covered since %q", got.CoveredSince)
	}
	if len(got.LocalSenders) != 2 {
		t.Fatalf("local senders %+v", got.LocalSenders)
	}
	root, site := got.LocalSenders[0], got.LocalSenders[1]
	if root.UID != 0 || root.Messages != 1 {
		t.Fatalf("root %+v", root)
	}
	if site.UID != 1001 || site.Messages != 2 || site.Refused != 1 || site.User != "shop_example" ||
		site.Last != "2026-11-02T07:00:00Z" || len(site.SenderDomains) != 1 || site.SenderDomains[0] != "shop.example" {
		t.Fatalf("site %+v", site)
	}
	// Only the loopback message relayed elsewhere counts; local delivery,
	// authenticated loopback, outside clients and submission do not. The
	// later relay refusal is counted as refused.
	if len(got.LoopbackSenders) != 1 {
		t.Fatalf("loopback %+v", got.LoopbackSenders)
	}
	blog := got.LoopbackSenders[0]
	if blog.Sender != "contact@blog.example" || blog.Messages != 1 || blog.Refused != 1 || blog.Last != "2026-11-02T07:01:00Z" {
		t.Fatalf("blog %+v", blog)
	}
}

func TestPasswordlessScanStillQueuedRelayCounts(t *testing.T) {
	log := `2026-10-12T10:00:00+00:00 mx postfix/smtpd[200]: CC1B2C3D4E: client=localhost[127.0.0.1]
2026-10-12T10:00:00+00:00 mx postfix/qmgr[90]: CC1B2C3D4E: from=<a@b.example>, size=9, nrcpt=1 (queue active)
2026-10-12T10:00:01+00:00 mx postfix/smtp[201]: CC1B2C3D4E: to=<x@outside.example>, relay=none, delay=1, status=deferred (connect timed out)
`
	scan, _ := scanPasswordlessLog(strings.NewReader(log))
	got := scan.result()
	if len(got.LoopbackSenders) != 1 || got.LoopbackSenders[0].Messages != 1 {
		t.Fatalf("%+v", got.LoopbackSenders)
	}
}

func TestPasswordlessScanEmptyLogIsReadNotUnknown(t *testing.T) {
	scan, _ := scanPasswordlessLog(strings.NewReader("garbage\n\n"))
	got := scan.result()
	if !got.Read || got.CoveredSince != "" || len(got.LocalSenders) != 0 || got.LocalSenders == nil || got.LoopbackSenders == nil {
		t.Fatalf("%+v", got)
	}
}

func TestPasswordlessReportUnreadableLogIsUnknown(t *testing.T) {
	saved, savedEnforcement := passwordlessReadJournal, passwordlessEnforcementNow
	t.Cleanup(func() { passwordlessReadJournal, passwordlessEnforcementNow = saved, savedEnforcement })
	passwordlessEnforcementNow = func() *transport.PasswordlessMailEnforcement {
		return &transport.PasswordlessMailEnforcement{State: passwordlessStateOpen}
	}
	passwordlessReadJournal = func(context.Context) (*passwordlessScan, error) {
		return nil, errors.New("journalctl: No journal files were found.")
	}
	var resp transport.PasswordlessMailReportResponse
	if err := (&Agent{}).PasswordlessMailReportV1(&transport.Empty{}, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Read || !strings.Contains(resp.Error, "could not be read") || resp.LocalSenders == nil ||
		resp.Enforcement == nil || resp.Enforcement.State != passwordlessStateOpen {
		t.Fatalf("%+v", resp)
	}
}

func TestPasswordlessSystemUsers(t *testing.T) {
	if !passwordlessSystemUser(0, "root") || !passwordlessSystemUser(5000, "vmail") || passwordlessSystemUser(1001, "shop_example") {
		t.Fatal("system users")
	}
}

func TestPasswordlessLineParse(t *testing.T) {
	at, ident, message, ok := parsePasswordlessLine("2026-10-10T08:00:00+0000 mx postfix/pickup[100]: X: uid=1 from=<>")
	if !ok || ident != "postfix/pickup" || message != "X: uid=1 from=<>" || at.IsZero() {
		t.Fatalf("%v %q %q %v", at, ident, message, ok)
	}
}
